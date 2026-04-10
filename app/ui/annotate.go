package ui

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
)

// annotKeyFile is the lookup key for file-level annotations in wrappedAnnotationLineCount.
const annotKeyFile = "file"

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

	// set DiffBg on all textinput sub-styles so View() output inherits the pane background.
	// wrapping View() externally doesn't work because lipgloss Render emits \033[0m resets.
	// text uses Normal fg (context line color) so active input is readable on any theme
	// and visually distinct from saved annotations (which use Annotation color + italic).
	inputStyle := lipgloss.NewStyle()
	if fg := m.styles.colors.Normal; fg != "" {
		inputStyle = inputStyle.Foreground(lipgloss.Color(fg))
	}
	if bg := m.styles.colors.DiffBg; bg != "" {
		inputStyle = inputStyle.Background(lipgloss.Color(bg))
	}
	ti.PromptStyle = inputStyle
	ti.TextStyle = inputStyle
	ti.Cursor.TextStyle = inputStyle
	ti.Cursor.Style = inputStyle

	muted := inputStyle
	if fg := m.styles.colors.Muted; fg != "" {
		muted = muted.Foreground(lipgloss.Color(fg))
	}
	ti.PlaceholderStyle = muted

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
	m.ensureLineAnnotationInputVisible()
	return cmd
}

// ensureLineAnnotationInputVisible scrolls the viewport so the line-annotation
// input row is visible. the input is rendered below the diff line, so keeping
// the cursor line visible is not always sufficient when cursor is on the last
// visible row.
func (m *Model) ensureLineAnnotationInputVisible() {
	if !m.annotating || m.fileAnnotating || m.viewport.Height <= 0 {
		return
	}
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return
	}

	inputY := m.cursorViewportY() + m.wrappedLineCount(m.diffCursor)
	switch {
	case inputY < m.viewport.YOffset:
		m.viewport.SetYOffset(inputY)
	case inputY >= m.viewport.YOffset+m.viewport.Height:
		m.viewport.SetYOffset(inputY - m.viewport.Height + 1)
	}
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
	m.pendingHunkJump = nil  // clear before refreshFilter which may trigger file load
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
		m.pendingHunkJump = nil  // clear before refreshFilter which may trigger file load
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
	return m.cursorOnFileAnnotationLine() || m.cursorOnAnnotation
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
	var comment string
	for _, a := range m.store.Get(m.currFile) {
		if key == annotKeyFile && a.Line == 0 {
			comment = "\U0001f4ac file: " + a.Comment
			break
		}
		if key != annotKeyFile && m.annotationKey(a.Line, a.Type) == key {
			comment = "\U0001f4ac " + a.Comment
			break
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

// hunkLineHeight returns the visual row count for a single diff line,
// including collapsed visibility, wrap, and inline annotation.
func (m Model) hunkLineHeight(idx int, hunks []int, annotationSet map[string]bool) int {
	if m.isCollapsedHidden(idx, hunks) {
		return 0
	}
	if m.isDeleteOnlyPlaceholder(idx, hunks) {
		return m.deletePlaceholderVisualHeight(idx)
	}
	h := m.wrappedLineCount(idx)
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
		if annotationSet[key] {
			h += m.wrappedAnnotationLineCount(key)
		}
	}
	return h
}

// cursorViewportY computes the actual viewport Y position of the cursor,
// accounting for injected annotation lines and the file-level annotation line.
// in collapsed mode, hidden removed lines (those in non-expanded hunks) are not counted.
func (m Model) cursorViewportY() int {
	var hunks []int
	if m.collapsed.enabled {
		hunks = m.findHunks()
	}
	return m.cursorViewportYUsing(hunks, m.buildAnnotationSet())
}

// cursorViewportYUsing is the same as cursorViewportY but accepts pre-built
// hunks and annotationSet to avoid redundant computation when the caller
// already has them (e.g. centerHunkInViewport).
func (m Model) cursorViewportYUsing(hunks []int, annotationSet map[string]bool) int {
	if m.currFile == "" || len(m.diffLines) == 0 {
		return max(0, m.diffCursor)
	}

	fileAnnotationOffset := 0
	if m.hasFileAnnotation() {
		fileAnnotationOffset = m.wrappedAnnotationLineCount(annotKeyFile)
	}

	if m.diffCursor == -1 {
		return 0
	}

	y := fileAnnotationOffset
	for i := 0; i < m.diffCursor && i < len(m.diffLines); i++ {
		y += m.hunkLineHeight(i, hunks, annotationSet)
	}
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
