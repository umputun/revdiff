package ui

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
)

// hunkKeywordRe matches whole-word "hunk" (case-insensitive).
// "block" was removed as it triggers false positives in casual usage (e.g., "this code block is fine").
var hunkKeywordRe = regexp.MustCompile(`(?i)\bhunk\b`)

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
	a := annotation.Annotation{File: m.currFile, Line: lineNum, Type: string(dl.ChangeType), Comment: text}
	if hunkKeywordRe.MatchString(text) {
		if endLine := m.hunkEndLine(m.diffCursor); endLine > lineNum {
			a.EndLine = endLine
		}
	}
	m.store.Add(a)
	m.annotating = false
	m.tree.refreshFilter(m.annotatedFiles())
	m.viewport.SetContent(m.renderDiff())
}

// cancelAnnotation exits annotation input mode without saving.
func (m *Model) cancelAnnotation() {
	m.annotating = false
	m.fileAnnotating = false
	m.viewport.SetContent(m.renderDiff())
}

// deleteFileAnnotation removes the file-level annotation and adjusts cursor position.
func (m *Model) deleteFileAnnotation() tea.Cmd {
	if !m.store.Delete(m.currFile, 0, "") {
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
	if m.store.Delete(m.currFile, lineNum, string(dl.ChangeType)) {
		m.pendingAnnotJump = nil // clear before refreshFilter which may trigger file load
		m.cursorOnAnnotation = false
		m.tree.refreshFilter(m.annotatedFiles())

		// if filter moved cursor to a different file, load the new selection
		if newFile := m.tree.selectedFile(); newFile != "" && newFile != m.currFile {
			m.loadSeq++
			return m.loadFileDiff(newFile)
		}

		m.syncViewportToCursor()
	}
	return nil
}

// handleAnnotateKey handles key messages during annotation input mode.
func (m Model) handleAnnotateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.saveAnnotation()
		return m, nil
	case tea.KeyEsc:
		m.cancelAnnotation()
		return m, nil
	default:
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

// hunkEndLine returns the display line number of the last line in the change hunk
// containing diffLines[idx]. only walks forward through lines of the same change type
// as the starting line, so both start and end use the same number space (old or new).
// returns 0 if idx is not inside a change hunk.
func (m Model) hunkEndLine(idx int) int {
	if idx < 0 || idx >= len(m.diffLines) {
		return 0
	}
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
		return 0
	}

	// walk forward from idx to find the last contiguous line of the same change type
	startType := dl.ChangeType
	last := idx
	for i := idx + 1; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType != startType {
			break
		}
		last = i
	}
	return m.diffLineNum(m.diffLines[last])
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
	}
	// if cursor is on the annotation sub-line, offset by wrapped line count
	// (annotation renders after all continuation lines of the diff line)
	if m.cursorOnAnnotation {
		y += m.wrappedLineCount(m.diffCursor)
	}
	return y
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
