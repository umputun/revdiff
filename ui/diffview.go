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

	// build annotation lookup for current file (excludes file-level)
	annotations := m.store.Get(m.currFile)
	annotationMap := make(map[string]string, len(annotations))
	for _, a := range annotations {
		if a.Line == 0 {
			continue
		}
		annotationMap[m.annotationKey(a.Line, a.Type)] = a.Comment
	}

	var b strings.Builder

	// render file-level annotation at the top if it exists
	if fileComment := m.fileAnnotationComment(); fileComment != "" {
		line := "      " + m.styles.AnnotationLine.Render("\U0001f4ac file: "+fileComment)
		if m.diffCursor == -1 && m.focus == paneDiff {
			line = m.styles.DiffCursorLine.Render(line)
		}
		b.WriteString(line + "\n")
	} else if m.annotating && m.fileAnnotating {
		// show text input for new file-level annotation
		line := "      " + m.styles.AnnotationLine.Render("\U0001f4ac file: ") + m.annotateInput.View()
		b.WriteString(line + "\n")
	}

	for i, dl := range m.diffLines {
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
		if i == m.diffCursor && m.focus == paneDiff {
			line = m.styles.DiffCursorLine.Render(line)
		}
		b.WriteString(line + "\n")

		// inject text input if annotating on this line (non-file-level)
		if m.annotating && !m.fileAnnotating && i == m.diffCursor {
			b.WriteString("      " + m.styles.AnnotationLine.Render("\U0001f4ac ") + m.annotateInput.View() + "\n")
		} else if dl.ChangeType != diff.ChangeDivider {
			// inject existing annotation line
			key := m.annotationKey(m.diffLineNum(dl), dl.ChangeType)
			if comment, ok := annotationMap[key]; ok {
				b.WriteString("      " + m.styles.AnnotationLine.Render("\U0001f4ac "+comment) + "\n")
			}
		}
	}
	return b.String()
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
func (m *Model) moveDiffCursorDown() {
	// if on file annotation line, move to first non-divider diff line
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
func (m *Model) moveDiffCursorUp() {
	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			return
		}
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
