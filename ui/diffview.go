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

	// build annotation lookup for current file
	annotations := m.store.Get(m.currFile)
	annotationMap := make(map[string]string, len(annotations))
	for _, a := range annotations {
		annotationMap[m.annotationKey(a.Line, a.Type)] = a.Comment
	}

	var b strings.Builder
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

		// inject text input if annotating on this line
		if m.annotating && i == m.diffCursor {
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
	for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
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
}

// moveDiffCursorPageDown moves the diff cursor down by one page (viewport height).
func (m *Model) moveDiffCursorPageDown() {
	for range m.viewport.Height {
		m.moveDiffCursorDown()
	}
	m.syncViewportToCursor()
}

// moveDiffCursorPageUp moves the diff cursor up by one page (viewport height).
func (m *Model) moveDiffCursorPageUp() {
	for range m.viewport.Height {
		m.moveDiffCursorUp()
	}
	m.syncViewportToCursor()
}

// moveDiffCursorToStart moves the diff cursor to the first non-divider line.
func (m *Model) moveDiffCursorToStart() {
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
