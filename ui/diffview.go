package ui

import (
	"fmt"
	"strings"

	"github.com/umputun/revdiff/diff"
)

// renderDiff renders the current file's diff lines with styling, cursor highlight,
// and injected annotation lines.
func (m Model) renderDiff() string {
	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	annotationMap, fileComment := m.buildAnnotationMap()
	var b strings.Builder
	m.renderFileAnnotationHeader(&b, fileComment)

	for i, dl := range m.diffLines {
		m.renderDiffLine(&b, i, dl)
		m.renderAnnotationOrInput(&b, i, annotationMap)
	}
	return b.String()
}

// buildAnnotationMap creates a lookup map of line annotations for the current file.
// returns the annotation map and the file-level comment (empty if none).
func (m Model) buildAnnotationMap() (annotations map[string]string, fileComment string) {
	all := m.store.Get(m.currFile)
	annotations = make(map[string]string, len(all))
	for _, a := range all {
		if a.Line == 0 {
			fileComment = a.Comment
			continue
		}
		annotations[m.annotationKey(a.Line, a.Type)] = a.Comment
	}
	return annotations, fileComment
}

// renderFileAnnotationHeader writes the file-level annotation or input to the builder.
func (m Model) renderFileAnnotationHeader(b *strings.Builder, fileComment string) {
	// when actively editing a file-level annotation, always show the input widget
	if m.annotating && m.fileAnnotating {
		line := "      " + m.styles.AnnotationLine.Render("\U0001f4ac file: ") + m.annotateInput.View()
		b.WriteString(line + "\n")
		return
	}

	if fileComment != "" {
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
	if idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation {
		line = m.styles.DiffCursorLine.Render(line)
	}
	b.WriteString(line + "\n")
}

// renderAnnotationOrInput writes the annotation input or existing annotation below a diff line.
func (m Model) renderAnnotationOrInput(b *strings.Builder, idx int, annotationMap map[string]string) {
	if m.annotating && !m.fileAnnotating && idx == m.diffCursor {
		b.WriteString("      " + m.styles.AnnotationLine.Render("\U0001f4ac ") + m.annotateInput.View() + "\n")
		return
	}
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), dl.ChangeType)
		if comment, ok := annotationMap[key]; ok {
			line := "      " + m.styles.AnnotationLine.Render("\U0001f4ac "+comment)
			if idx == m.diffCursor && m.cursorOnAnnotation && m.focus == paneDiff {
				line = m.styles.DiffCursorLine.Render(line)
			}
			b.WriteString(line + "\n")
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
// if the current line has an annotation and cursor is on the diff line, stops on the annotation first.
func (m *Model) moveDiffCursorDown() {
	// if currently on annotation sub-line, move to the next diff line
	if m.cursorOnAnnotation {
		m.cursorOnAnnotation = false
		for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
			if m.diffLines[i].ChangeType != diff.ChangeDivider {
				m.diffCursor = i
				return
			}
		}
		return
	}

	// if current line has an annotation, stop on it first
	if m.diffCursor >= 0 && m.diffCursor < len(m.diffLines) {
		dl := m.diffLines[m.diffCursor]
		if dl.ChangeType != diff.ChangeDivider {
			lineNum := m.diffLineNum(dl)
			if m.store.Has(m.currFile, lineNum, dl.ChangeType) {
				m.cursorOnAnnotation = true
				return
			}
		}
	}

	// move to next non-divider diff line
	start := m.diffCursor + 1
	if m.diffCursor == -1 {
		start = 0
	}
	for i := start; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			return
		}
	}
}

// moveDiffCursorUp moves the diff cursor to the previous non-divider line.
// when moving up from a diff line, if the previous line has an annotation, lands on the annotation first.
func (m *Model) moveDiffCursorUp() {
	// if currently on annotation sub-line, move up to the diff line itself
	if m.cursorOnAnnotation {
		m.cursorOnAnnotation = false
		return
	}

	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType == diff.ChangeDivider {
			continue
		}
		m.diffCursor = i
		// if this line has an annotation, land on it
		dl := m.diffLines[i]
		lineNum := m.diffLineNum(dl)
		if m.store.Has(m.currFile, lineNum, dl.ChangeType) {
			m.cursorOnAnnotation = true
		}
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
func (m *Model) moveDiffCursorToStart() {
	m.cursorOnAnnotation = false
	if m.hasFileAnnotation() {
		m.diffCursor = -1
		m.syncViewportToCursor()
		return
	}

	m.diffCursor = 0
	for i, dl := range m.diffLines {
		if dl.ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			break
		}
	}
	m.syncViewportToCursor()
}

// moveDiffCursorToEnd moves the diff cursor to the last non-divider line.
func (m *Model) moveDiffCursorToEnd() {
	m.cursorOnAnnotation = false
	for i := len(m.diffLines) - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			break
		}
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

// findHunks scans diffLines and returns a slice of chunk start indices.
// a chunk is a contiguous group of added/removed lines. the returned index
// is the first line of each such group.
func (m Model) findHunks() []int {
	var hunks []int
	inHunk := false
	for i, dl := range m.diffLines {
		isChange := dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove
		if isChange && !inHunk {
			hunks = append(hunks, i)
			inHunk = true
		} else if !isChange {
			inHunk = false
		}
	}
	return hunks
}

// currentHunk returns the 1-based chunk index and total chunk count.
// returns non-zero chunk index only when the cursor is on a changed line (add/remove).
// returns (0, total) when cursor is not inside any chunk.
func (m Model) currentHunk() (int, int) {
	hunks := m.findHunks()
	if len(hunks) == 0 {
		return 0, 0
	}
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return 0, len(hunks)
	}
	dl := m.diffLines[m.diffCursor]
	if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
		return 0, len(hunks)
	}
	// cursor is on a changed line, find which chunk
	cur := 0
	for i, start := range hunks {
		if m.diffCursor >= start {
			cur = i + 1
		}
	}
	return cur, len(hunks)
}

// moveToNextHunk moves the diff cursor to the start of the next change chunk.
func (m *Model) moveToNextHunk() {
	m.cursorOnAnnotation = false
	hunks := m.findHunks()
	for _, start := range hunks {
		if start > m.diffCursor {
			m.diffCursor = start
			m.centerViewportOnCursor()
			return
		}
	}
}

// moveToPrevHunk moves the diff cursor to the start of the previous change chunk.
func (m *Model) moveToPrevHunk() {
	m.cursorOnAnnotation = false
	hunks := m.findHunks()
	for i := len(hunks) - 1; i >= 0; i-- {
		if hunks[i] < m.diffCursor {
			m.diffCursor = hunks[i]
			m.centerViewportOnCursor()
			return
		}
	}
}

// centerViewportOnCursor scrolls the viewport to place the cursor in the middle of the page.
func (m *Model) centerViewportOnCursor() {
	cursorY := m.cursorViewportY()
	offset := max(0, cursorY-m.viewport.Height/2)
	m.viewport.SetYOffset(offset)
	m.viewport.SetContent(m.renderDiff())
}
