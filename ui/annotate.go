package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
)

// newAnnotationInput creates and focuses a text input for annotation editing.
// prefixWidth accounts for the visible prefix characters (cursor col + emoji + label + margin).
func (m *Model) newAnnotationInput(placeholder string, prefixWidth int) (textinput.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = placeholder
	cmd := ti.Focus()
	ti.CharLimit = 500
	ti.Width = max(10, m.diffContentWidth()-prefixWidth)
	return ti, cmd
}

// startSelection enters visual range selection mode with the current cursor as anchor.
func (m *Model) startSelection() {
	if m.focus != paneDiff || m.annotating || m.showHelp || m.showAnnotList ||
		m.searching || m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return
	}
	dl := m.diffLines[m.diffCursor]
	if dl.ChangeType == diff.ChangeDivider {
		return
	}
	m.selecting = true
	m.selectAnchor = m.diffCursor
	m.cursorOnAnnotation = false
	m.cursorOnRangeAnnotation = false
	m.viewport.SetContent(m.renderDiff())
}

// selectionBounds returns the ordered (start, end) diffLines indices of the current selection.
func (m *Model) selectionBounds() (int, int) {
	start, end := m.selectAnchor, m.diffCursor
	if start > end {
		start, end = end, start
	}
	return start, end
}

// annotateHunk auto-selects the current hunk and opens range annotation input.
// uses hunk boundary detection to determine the range, then calls startRangeAnnotation.
// no-op when cursor is on a context/divider line (not inside a hunk).
func (m *Model) annotateHunk() tea.Cmd {
	if m.annotating || m.showHelp || m.showAnnotList || m.searching {
		return nil
	}
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return nil
	}

	hunks := m.findHunks()
	hunkStart := m.hunkStartFor(m.diffCursor, hunks)
	if hunkStart < 0 {
		return nil // cursor not on a changed line
	}

	hunkEnd := m.hunkEndFor(hunkStart)
	startLine := m.diffLineNum(m.diffLines[hunkStart])
	endLine := m.diffLineNum(m.diffLines[hunkEnd])

	if hunkStart == hunkEnd {
		// single diff line in hunk: use point annotation.
		// check diff index count, not line numbers — replacement hunks may have
		// startLine == endLine (same old/new number) but span multiple diff lines.
		m.diffCursor = hunkStart
		return m.startAnnotation()
	}

	// position cursor at hunk end where annotation input renders.
	// in collapsed mode, if hunk end is hidden (e.g., delete-only hunk),
	// anchor to the visible placeholder at hunkStart instead.
	visibleEnd := hunkEnd
	if m.collapsed.enabled && m.isCollapsedHidden(hunkEnd, hunks) {
		visibleEnd = hunkStart
	}
	m.diffCursor = visibleEnd
	return m.startRangeAnnotation(startLine, endLine, hunkStart, visibleEnd)
}

// hunkEndFor returns the last diffLines index of the hunk starting at startIdx.
func (m *Model) hunkEndFor(startIdx int) int {
	end := startIdx
	for i := startIdx; i < len(m.diffLines); i++ {
		dl := m.diffLines[i]
		if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
			break
		}
		end = i
	}
	return end
}

// editRangeAnnotation opens annotation input for the range annotation at the current cursor.
func (m *Model) editRangeAnnotation() tea.Cmd {
	dl, ok := m.cursorDiffLine()
	if !ok {
		return nil
	}
	lineNum := m.diffLineNum(dl)
	rangeAnn, ok := m.store.GetRangeCovering(m.currFile, lineNum)
	if !ok {
		return nil
	}
	startIdx, endIdx := m.visibleRangeIndices(rangeAnn.Line, rangeAnn.EndLine)
	return m.startRangeAnnotation(rangeAnn.Line, rangeAnn.EndLine, startIdx, endIdx)
}

// startAnnotation enters annotation input mode for the current cursor line.
func (m *Model) startAnnotation() tea.Cmd {
	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}
	// prevent annotating hidden or placeholder removed lines in collapsed mode
	hunks := m.findHunks()
	if m.isCollapsedHidden(m.diffCursor, hunks) {
		return nil
	}
	if m.isDeleteOnlyPlaceholder(m.diffCursor, hunks) {
		return nil
	}

	ti, cmd := m.newAnnotationInput("annotation...", 6) // cursor col + emoji prefix "💬 " + border margin

	// pre-fill with existing annotation if one exists
	lineNum := m.diffLineNum(dl)
	for _, a := range m.store.Get(m.currFile) {
		if a.Line == lineNum && a.Type == string(dl.ChangeType) {
			ti.SetValue(a.Comment)
			break
		}
	}

	m.annotateInput = ti
	m.annotating = true
	m.fileAnnotating = false
	return cmd
}

// startFileAnnotation enters annotation input mode for a file-level annotation (Line=0).
func (m *Model) startFileAnnotation() tea.Cmd {
	if m.currFile == "" {
		return nil
	}

	ti, cmd := m.newAnnotationInput("file-level annotation...", 12) // cursor col + "💬 file: " prefix + border margin

	// pre-fill with existing file-level annotation if one exists
	for _, a := range m.store.Get(m.currFile) {
		if a.Line == 0 {
			ti.SetValue(a.Comment)
			break
		}
	}

	m.annotateInput = ti
	m.annotating = true
	m.fileAnnotating = true
	m.diffCursor = -1 // position cursor on the file annotation line
	m.viewport.GotoTop()
	return cmd
}

// saveAnnotation saves the current text input as an annotation on the cursor line.
func (m *Model) saveAnnotation() {
	text := m.annotateInput.Value()
	if text == "" {
		m.cancelAnnotation()
		return
	}

	if m.fileAnnotating {
		m.store.Add(annotation.Annotation{File: m.currFile, Line: 0, Type: "", Comment: text})
		m.annotating = false
		m.fileAnnotating = false
		m.diffCursor = -1 // position cursor on the file annotation line
		m.tree.refreshFilter(m.annotatedFiles())
		m.viewport.SetContent(m.renderDiff())
		m.viewport.GotoTop()
		return
	}

	dl, ok := m.cursorDiffLine()
	if !ok {
		m.cancelAnnotation()
		return
	}

	lineNum := m.diffLineNum(dl)
	m.store.Add(annotation.Annotation{File: m.currFile, Line: lineNum, Type: string(dl.ChangeType), Comment: text})
	m.annotating = false
	m.tree.refreshFilter(m.annotatedFiles())
	m.viewport.SetContent(m.renderDiff())
}

// confirmSelection finalizes the visual selection and opens range annotation input.
// single-line selections (anchor == cursor) collapse to a regular point annotation.
func (m *Model) confirmSelection() tea.Cmd {
	startIdx, endIdx := m.selectionBounds()

	// single-line selection: collapse to point annotation
	if startIdx == endIdx {
		m.selecting = false
		m.selectAnchor = 0
		m.diffCursor = startIdx
		return m.startAnnotation()
	}

	startDL := m.diffLines[startIdx]
	endDL := m.diffLines[endIdx]
	startLine := m.diffLineNum(startDL)
	endLine := m.diffLineNum(endDL)

	// keep cursor at end of selection where the annotation input renders
	m.diffCursor = endIdx
	return m.startRangeAnnotation(startLine, endLine, startIdx, endIdx)
}

// startRangeAnnotation enters annotation input mode for a range of lines.
func (m *Model) startRangeAnnotation(startLine, endLine, startIdx, endIdx int) tea.Cmd {
	prefix := rangeAnnotationPrefix(startLine, endLine)
	ti, cmd := m.newAnnotationInput("range annotation...", len(prefix)+2)

	// pre-fill with existing range annotation if one matches
	for _, a := range m.store.Get(m.currFile) {
		if a.Line == startLine && a.EndLine == endLine {
			ti.SetValue(a.Comment)
			break
		}
	}

	m.annotateInput = ti
	m.annotating = true
	m.fileAnnotating = false
	m.rangeStartLine = startLine
	m.rangeEndLine = endLine
	m.rangeStartIdx = startIdx
	m.rangeEndIdx = endIdx
	return cmd
}

// saveRangeAnnotation saves the current text input as a range annotation.
func (m *Model) saveRangeAnnotation() {
	text := m.annotateInput.Value()
	if text == "" {
		m.cancelAnnotation()
		return
	}

	if !m.store.Add(annotation.Annotation{
		File:    m.currFile,
		Line:    m.rangeStartLine,
		EndLine: m.rangeEndLine,
		Type:    "",
		Comment: text,
	}) {
		m.statusFlash = "overlaps existing range — adjust or [esc] cancel"
		return
	}
	m.annotating = false
	m.selecting = false
	m.selectAnchor = 0
	m.rangeStartLine = 0
	m.rangeEndLine = 0
	m.rangeStartIdx = 0
	m.rangeEndIdx = 0
	m.tree.refreshFilter(m.annotatedFiles())
	m.viewport.SetContent(m.renderDiff())
}

// cancelAnnotation exits annotation input mode without saving.
func (m *Model) cancelAnnotation() {
	m.annotating = false
	m.fileAnnotating = false
	m.selecting = false
	m.selectAnchor = 0
	m.rangeStartLine = 0
	m.rangeEndLine = 0
	m.rangeStartIdx = 0
	m.rangeEndIdx = 0
	m.statusFlash = ""
	m.viewport.SetContent(m.renderDiff())
}

// deleteFileAnnotation removes the file-level annotation and adjusts cursor position.
func (m *Model) deleteFileAnnotation() tea.Cmd {
	if !m.store.Delete(m.currFile, 0, 0, "") {
		return nil
	}
	m.pendingAnnotJump = nil // clear before refreshFilter which may trigger file load
	m.skipInitialDividers()

	m.tree.refreshFilter(m.annotatedFiles())

	if newFile := m.tree.selectedFile(); newFile != "" && newFile != m.currFile {
		m.loadSeq++
		return m.loadFileDiff(newFile)
	}

	m.syncViewportToCursor()
	return nil
}

// deleteAnnotation removes the annotation on the current cursor line if one exists.
// handles both file-level annotations (cursor at -1) and regular line annotations.
// if the current line has no annotation, checks the previous line (since annotations
// only works when cursor is on the annotation sub-line (cursorOnAnnotation=true) or file annotation line.
// returns a command to load the new file if the tree selection changed after filter refresh.
func (m *Model) deleteAnnotation() tea.Cmd {
	if m.cursorOnFileAnnotationLine() {
		return m.deleteFileAnnotation()
	}

	if !m.cursorOnAnnotation {
		return nil
	}

	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}

	lineNum := m.diffLineNum(dl)

	// when on range sub-row, delete the covering range directly
	if m.cursorOnRangeAnnotation {
		if rangeAnn, ok := m.store.GetRangeCovering(m.currFile, lineNum); ok {
			m.store.Delete(m.currFile, rangeAnn.Line, rangeAnn.EndLine, rangeAnn.Type)
			return m.afterAnnotationDelete()
		}
		return nil
	}

	// on point sub-row, delete the point annotation
	if m.store.Delete(m.currFile, lineNum, 0, string(dl.ChangeType)) {
		return m.afterAnnotationDelete()
	}

	// fall back to a covering range when no point annotation exists
	if rangeAnn, ok := m.store.GetRangeCovering(m.currFile, lineNum); ok {
		m.store.Delete(m.currFile, rangeAnn.Line, rangeAnn.EndLine, rangeAnn.Type)
		return m.afterAnnotationDelete()
	}
	return nil
}

// afterAnnotationDelete handles shared cleanup after removing any annotation.
func (m *Model) afterAnnotationDelete() tea.Cmd {
	m.pendingAnnotJump = nil
	m.cursorOnAnnotation = false
	m.cursorOnRangeAnnotation = false
	m.tree.refreshFilter(m.annotatedFiles())

	if newFile := m.tree.selectedFile(); newFile != "" && newFile != m.currFile {
		m.loadSeq++
		return m.loadFileDiff(newFile)
	}

	m.syncViewportToCursor()
	return nil
}

// handleAnnotateKey handles key messages during annotation input mode.
func (m Model) handleAnnotateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if m.rangeStartLine > 0 {
			m.saveRangeAnnotation()
		} else {
			m.saveAnnotation()
		}
		return m, nil
	case tea.KeyEsc:
		m.cancelAnnotation()
		return m, nil
	default:
		m.statusFlash = ""
		var cmd tea.Cmd
		m.annotateInput, cmd = m.annotateInput.Update(msg)
		m.viewport.SetContent(m.renderDiff()) // re-render so typed characters are visible immediately
		return m, cmd
	}
}

// cursorLineHasAnnotation checks if the cursor is on a deletable annotation line.
// returns true only when cursor is on the file annotation line or on an annotation sub-line.
func (m Model) cursorLineHasAnnotation() bool {
	if m.cursorOnFileAnnotationLine() {
		return true
	}
	return m.cursorOnAnnotation
}

// isRangeAnnotationEnd returns true if the given diffLines index is the end of a saved range annotation.
func (m Model) isRangeAnnotationEnd(idx int) bool {
	for _, r := range m.buildRangeAnnotations() {
		if r.endIdx == idx {
			return true
		}
	}
	return false
}

// hasFileAnnotation checks if the current file has a file-level annotation (Line=0).
func (m Model) hasFileAnnotation() bool {
	for _, a := range m.store.Get(m.currFile) {
		if a.Line == 0 {
			return true
		}
	}
	return false
}

// cursorOnFileAnnotationLine returns true if the diff cursor is on the file-level annotation line.
func (m Model) cursorOnFileAnnotationLine() bool {
	return m.diffCursor == -1 && m.hasFileAnnotation()
}

// diffLineNum returns the display line number for a diff line.
func (m Model) diffLineNum(dl diff.DiffLine) int {
	if dl.ChangeType == diff.ChangeRemove {
		return dl.OldNum
	}
	return dl.NewNum
}

// wrappedAnnotationLineCount returns the number of visual rows an annotation occupies.
// annotations always wrap at the pane width regardless of wrapMode.
func (m Model) wrappedAnnotationLineCount(key string) int {
	comment := ""
	if key == "file" {
		for _, a := range m.store.Get(m.currFile) {
			if a.Line == 0 {
				comment = "\U0001f4ac file: " + a.Comment
				break
			}
		}
	} else {
		for _, a := range m.store.Get(m.currFile) {
			aKey := m.annotationKey(a.Line, a.Type)
			if aKey == key {
				comment = "\U0001f4ac " + a.Comment
				break
			}
		}
	}
	if comment == "" {
		return 1
	}
	wrapWidth := m.diffContentWidth() - 1 // 1 for cursor column
	if wrapWidth > 10 && lipgloss.Width(comment) > wrapWidth {
		return len(m.wrapContent(comment, wrapWidth))
	}
	return 1
}

// cursorViewportY computes the actual viewport Y position of the cursor,
// accounting for injected annotation lines and the file-level annotation line.
// in collapsed mode, hidden removed lines (those in non-expanded hunks) are not counted.
func (m Model) cursorViewportY() int {
	if m.currFile == "" || len(m.diffLines) == 0 {
		return max(0, m.diffCursor)
	}

	// file-level annotation line at the top (may wrap to multiple rows)
	fileAnnotationOffset := 0
	if m.hasFileAnnotation() {
		fileAnnotationOffset = m.wrappedAnnotationLineCount("file")
	}

	// cursor is on the file annotation line
	if m.diffCursor == -1 {
		return 0
	}

	annotationSet := m.buildAnnotationSet()
	ranges := m.buildRangeAnnotations()
	rangeEndSet := make(map[int]rangeRenderInfo, len(ranges))
	for _, r := range ranges {
		rangeEndSet[r.endIdx] = r
	}
	var hunks []int
	if m.collapsed.enabled {
		hunks = m.findHunks()
	}

	y := fileAnnotationOffset
	for i := 0; i < m.diffCursor && i < len(m.diffLines); i++ {
		// skip hidden removed lines in collapsed mode
		if m.isCollapsedHidden(i, hunks) {
			continue
		}
		// delete-only placeholders render synthetic text ("⋯ N lines deleted"), not original content.
		// use placeholder text for wrapping to stay in sync with renderDeletePlaceholder.
		if m.isDeleteOnlyPlaceholder(i, hunks) {
			y += m.deletePlaceholderVisualHeight(i)
			continue
		}
		y += m.wrappedLineCount(i) // the diff line (may occupy multiple visual rows when wrapping)
		dl := m.diffLines[i]
		if dl.ChangeType != diff.ChangeDivider {
			key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
			if annotationSet[key] {
				y += m.wrappedAnnotationLineCount(key)
			}
		}
		// range annotation row below range end
		if r, ok := rangeEndSet[i]; ok {
			y += m.wrappedRangeAnnotationLineCount(r)
		}
	}
	// if cursor is on an annotation sub-line, offset by wrapped line count of the diff line
	// (annotations render after all continuation lines of the diff line)
	if m.cursorOnAnnotation {
		y += m.wrappedLineCount(m.diffCursor)
		// on range sub-row: also add point annotation height between diff line and range annotation
		if m.cursorOnRangeAnnotation && m.diffCursor >= 0 && m.diffCursor < len(m.diffLines) {
			dl := m.diffLines[m.diffCursor]
			key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
			if annotationSet[key] {
				y += m.wrappedAnnotationLineCount(key)
			}
		}
	}
	return y
}

// wrappedRangeAnnotationLineCount returns the visual row count for a range annotation comment.
func (m Model) wrappedRangeAnnotationLineCount(r rangeRenderInfo) int {
	comment := rangeAnnotationPrefix(r.startLine, r.endLine) + r.comment
	wrapWidth := m.diffContentWidth() - 1
	if wrapWidth > 10 && lipgloss.Width(comment) > wrapWidth {
		return len(m.wrapContent(comment, wrapWidth))
	}
	return 1
}

// buildAnnotationSet returns a set of annotation keys for the current file.
// excludes file-level annotations (Line=0) since they are rendered separately.
func (m Model) buildAnnotationSet() map[string]bool {
	annotations := m.store.Get(m.currFile)
	set := make(map[string]bool, len(annotations))
	for _, a := range annotations {
		if a.Line == 0 {
			continue
		}
		set[m.annotationKey(a.Line, a.Type)] = true
	}
	return set
}

// annotationKey creates a lookup key from line number and change type.
func (m Model) annotationKey(line int, changeType string) string {
	return fmt.Sprintf("%d:%s", line, changeType)
}
