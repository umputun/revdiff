package ui

import (
	"fmt"
	"strings"

	"github.com/umputun/revdiff/diff"
)

// renderDiff renders the current file's diff lines with styling, cursor highlight,
// and injected annotation lines. dispatches to renderSimplifiedDiff when simplified view is active.
func (m Model) renderDiff() string {
	if m.simplifiedView {
		return m.renderSimplifiedDiff()
	}

	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	annotationMap := m.buildAnnotationMap()
	var b strings.Builder
	m.renderFileAnnotationHeader(&b)

	for i, dl := range m.diffLines {
		m.renderDiffLine(&b, i, dl)
		m.renderAnnotationOrInput(&b, i, dl, annotationMap)
	}
	return b.String()
}

// buildAnnotationMap creates a lookup map of line annotations for the current file.
// excludes file-level annotations (Line=0).
func (m Model) buildAnnotationMap() map[string]string {
	annotations := m.store.Get(m.currFile)
	annotationMap := make(map[string]string, len(annotations))
	for _, a := range annotations {
		if a.Line == 0 {
			continue
		}
		annotationMap[m.annotationKey(a.Line, a.Type)] = a.Comment
	}
	return annotationMap
}

// renderFileAnnotationHeader writes the file-level annotation or input to the builder.
func (m Model) renderFileAnnotationHeader(b *strings.Builder) {
	// when actively editing a file-level annotation, always show the input widget
	if m.annotating && m.fileAnnotating {
		line := "      " + m.styles.AnnotationLine.Render("\U0001f4ac file: ") + m.annotateInput.View()
		b.WriteString(line + "\n")
		return
	}

	if fileComment := m.fileAnnotationComment(); fileComment != "" {
		line := "      " + m.styles.AnnotationLine.Render("\U0001f4ac file: "+fileComment)
		if m.diffCursor == -1 && m.focus == paneDiff {
			line = m.styles.DiffCursorLine.Render(line)
		}
		b.WriteString(line + "\n")
	}
}

// renderDiffLine writes a single styled diff line (with cursor highlight) to the builder.
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	lineNum := m.styles.LineNumber.Render(m.formatLineNum(dl))
	var content string
	switch dl.ChangeType {
	case diff.ChangeAdd:
		content = m.styles.LineAdd.Render(" +" + dl.Content)
	case diff.ChangeRemove:
		content = m.styles.LineRemove.Render(" -" + dl.Content)
	case diff.ChangeDivider:
		content = m.styles.LineNumber.Render(" " + dl.Content)
	default:
		content = m.styles.LineContext.Render("  " + dl.Content)
	}

	line := lineNum + content
	if idx == m.diffCursor && m.focus == paneDiff {
		line = m.styles.DiffCursorLine.Render(line)
	}
	b.WriteString(line + "\n")
}

// renderAnnotationOrInput writes the annotation input or existing annotation below a diff line.
func (m Model) renderAnnotationOrInput(b *strings.Builder, idx int, dl diff.DiffLine, annotationMap map[string]string) {
	if m.annotating && !m.fileAnnotating && idx == m.diffCursor {
		b.WriteString("      " + m.styles.AnnotationLine.Render("\U0001f4ac ") + m.annotateInput.View() + "\n")
		return
	}
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), dl.ChangeType)
		if comment, ok := annotationMap[key]; ok {
			b.WriteString("      " + m.styles.AnnotationLine.Render("\U0001f4ac "+comment) + "\n")
		}
	}
}

// formatLineNum formats old/new line numbers for display.
func (m Model) formatLineNum(dl diff.DiffLine) string {
	switch dl.ChangeType {
	case diff.ChangeAdd:
		return fmt.Sprintf("%4d ", dl.NewNum)
	case diff.ChangeRemove:
		return fmt.Sprintf("%4d ", dl.OldNum)
	case diff.ChangeDivider:
		return "     "
	default:
		return fmt.Sprintf("%4d ", dl.NewNum)
	}
}

// cursorDiffLine returns the DiffLine at the current cursor position, if valid.
func (m Model) cursorDiffLine() (diff.DiffLine, bool) {
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return diff.DiffLine{}, false
	}
	return m.diffLines[m.diffCursor], true
}

// moveDiffCursorDown moves the diff cursor to the next non-divider line.
// in simplified view, also skips lines that are not visible.
func (m *Model) moveDiffCursorDown() {
	// if on file annotation line, move to first non-divider diff line
	start := m.diffCursor + 1
	if m.diffCursor == -1 {
		start = 0
	}

	var visible []bool
	if m.simplifiedView {
		visible = m.visibleInSimplified()
	}

	for i := start; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType == diff.ChangeDivider {
			continue
		}
		if len(visible) > 0 && !visible[i] {
			continue
		}
		m.diffCursor = i
		return
	}
}

// moveDiffCursorUp moves the diff cursor to the previous non-divider line.
// in simplified view, also skips lines that are not visible.
func (m *Model) moveDiffCursorUp() {
	var visible []bool
	if m.simplifiedView {
		visible = m.visibleInSimplified()
	}

	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType == diff.ChangeDivider {
			continue
		}
		if len(visible) > 0 && !visible[i] {
			continue
		}
		m.diffCursor = i
		return
	}
	// if we're at the first line and there's a file-level annotation, go to it
	if m.diffCursor >= 0 && m.hasFileAnnotation() {
		m.diffCursor = -1
	}
}

// moveDiffCursorPageDown moves the diff cursor down by one visual page.
// accounts for divider lines and annotation rows that occupy rendered space.
// scrolls the viewport so cursor appears near the top of the new page.
func (m *Model) moveDiffCursorPageDown() {
	startY := m.cursorViewportY()
	for {
		prev := m.diffCursor
		m.moveDiffCursorDown()
		if m.diffCursor == prev {
			break
		}
		if m.cursorViewportY()-startY >= m.viewport.Height {
			break
		}
	}
	// place cursor at the top of the viewport for a true page-scroll feel
	m.viewport.SetYOffset(m.cursorViewportY())
	m.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorPageUp moves the diff cursor up by one visual page.
// accounts for divider lines and annotation rows that occupy rendered space.
// scrolls the viewport so cursor appears near the bottom of the new page.
func (m *Model) moveDiffCursorPageUp() {
	startY := m.cursorViewportY()
	for {
		prev := m.diffCursor
		m.moveDiffCursorUp()
		if m.diffCursor == prev {
			break
		}
		if startY-m.cursorViewportY() >= m.viewport.Height {
			break
		}
	}
	// place cursor at the bottom of the viewport for a true page-scroll feel
	m.viewport.SetYOffset(max(0, m.cursorViewportY()-m.viewport.Height+1))
	m.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorToStart moves the diff cursor to the first selectable position.
// if a file-level annotation exists, the cursor goes to -1 (file annotation line).
// in simplified view, finds the first visible non-divider line.
func (m *Model) moveDiffCursorToStart() {
	if m.hasFileAnnotation() {
		m.diffCursor = -1
		m.syncViewportToCursor()
		return
	}

	var visible []bool
	if m.simplifiedView {
		visible = m.visibleInSimplified()
	}

	m.diffCursor = 0
	for i, dl := range m.diffLines {
		if dl.ChangeType == diff.ChangeDivider {
			continue
		}
		if len(visible) > 0 && !visible[i] {
			continue
		}
		m.diffCursor = i
		break
	}
	m.syncViewportToCursor()
}

// moveDiffCursorToEnd moves the diff cursor to the last non-divider line.
// in simplified view, finds the last visible non-divider line.
func (m *Model) moveDiffCursorToEnd() {
	var visible []bool
	if m.simplifiedView {
		visible = m.visibleInSimplified()
	}

	for i := len(m.diffLines) - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType == diff.ChangeDivider {
			continue
		}
		if len(visible) > 0 && !visible[i] {
			continue
		}
		m.diffCursor = i
		break
	}
	m.syncViewportToCursor()
}

// syncViewportToCursor adjusts viewport scroll to keep cursor visible and re-renders content.
// accounts for annotation lines injected between diff lines.
func (m *Model) syncViewportToCursor() {
	cursorY := m.cursorViewportY()
	if cursorY < m.viewport.YOffset {
		m.viewport.SetYOffset(cursorY)
	} else if cursorY >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(cursorY - m.viewport.Height + 1)
	}
	m.viewport.SetContent(m.renderDiff())
}

// visibleInSimplified returns a boolean slice marking which diffLines indices
// are visible in simplified view. visible lines are: changed lines (add/remove),
// up to 3 context lines around each change group, lines with annotations,
// and divider lines between groups.
func (m Model) visibleInSimplified() []bool {
	n := len(m.diffLines)
	if n == 0 {
		return nil
	}

	visible := make([]bool, n)
	const contextLines = 3

	// mark changed lines and their context
	for i, dl := range m.diffLines {
		if dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove {
			// mark context before
			for j := max(0, i-contextLines); j < i; j++ {
				visible[j] = true
			}
			visible[i] = true
			// mark context after
			for j := i + 1; j < n && j <= i+contextLines; j++ {
				visible[j] = true
			}
		}
	}

	// mark annotated lines as visible so annotations are never hidden
	annotationMap := m.buildAnnotationMap()
	for i, dl := range m.diffLines {
		if dl.ChangeType == diff.ChangeDivider {
			continue
		}
		key := m.annotationKey(m.diffLineNum(dl), dl.ChangeType)
		if annotationMap[key] != "" {
			visible[i] = true
		}
	}

	return visible
}

// renderSimplifiedDiff renders only visible lines (changes + context) with
// dividers inserted between non-adjacent visible groups.
func (m Model) renderSimplifiedDiff() string {
	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	visible := m.visibleInSimplified()
	annotationMap := m.buildAnnotationMap()

	var b strings.Builder
	m.renderFileAnnotationHeader(&b)

	lastVisibleIdx := -1
	for i, dl := range m.diffLines {
		if !visible[i] {
			continue
		}

		// insert divider between non-adjacent visible groups
		if lastVisibleIdx >= 0 && i-lastVisibleIdx > 1 {
			b.WriteString(m.styles.LineNumber.Render("     ") + m.styles.LineNumber.Render(" ···") + "\n")
		}
		lastVisibleIdx = i

		m.renderDiffLine(&b, i, dl)
		m.renderAnnotationOrInput(&b, i, dl, annotationMap)
	}
	return b.String()
}

// ensureCursorVisible adjusts the diff cursor to the nearest visible line
// when switching to simplified view.
func (m *Model) ensureCursorVisible() {
	if !m.simplifiedView || len(m.diffLines) == 0 {
		return
	}
	visible := m.visibleInSimplified()

	// cursor is on file annotation line or already visible
	if m.diffCursor == -1 {
		return
	}
	if m.diffCursor >= 0 && m.diffCursor < len(visible) && visible[m.diffCursor] {
		return
	}

	// search forward for nearest visible non-divider line
	for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
		if visible[i] && m.diffLines[i].ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			return
		}
	}
	// search backward
	for i := m.diffCursor - 1; i >= 0; i-- {
		if visible[i] && m.diffLines[i].ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			return
		}
	}
}

// findChunks scans diffLines and returns a slice of chunk start indices.
// a chunk is a contiguous group of added/removed lines. the returned index
// is the first line of each such group.
func (m Model) findChunks() []int {
	var chunks []int
	inChunk := false
	for i, dl := range m.diffLines {
		isChange := dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove
		if isChange && !inChunk {
			chunks = append(chunks, i)
			inChunk = true
		} else if !isChange {
			inChunk = false
		}
	}
	return chunks
}

// currentChunk returns the 1-based chunk index and total chunk count
// based on the current diffCursor position. returns (0, 0) when there are no chunks.
func (m Model) currentChunk() (int, int) {
	chunks := m.findChunks()
	if len(chunks) == 0 {
		return 0, 0
	}
	if m.diffCursor < 0 {
		return 0, 0
	}
	cur := 0
	for i, start := range chunks {
		if m.diffCursor >= start {
			cur = i + 1
		}
	}
	if cur == 0 {
		cur = 1
	}
	return cur, len(chunks)
}

// moveToNextChunk moves the diff cursor to the start of the next change chunk.
func (m *Model) moveToNextChunk() {
	chunks := m.findChunks()
	for _, start := range chunks {
		if start > m.diffCursor {
			m.diffCursor = start
			m.syncViewportToCursor()
			return
		}
	}
}

// moveToPrevChunk moves the diff cursor to the start of the previous change chunk.
func (m *Model) moveToPrevChunk() {
	chunks := m.findChunks()
	for i := len(chunks) - 1; i >= 0; i-- {
		if chunks[i] < m.diffCursor {
			m.diffCursor = chunks[i]
			m.syncViewportToCursor()
			return
		}
	}
}
