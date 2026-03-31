package ui

import (
	"fmt"
	"strings"

	"github.com/umputun/revdiff/diff"
)

// renderDiff renders the current file's diff lines with styling and cursor highlight.
func (m Model) renderDiff() string {
	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	var b strings.Builder
	for i, dl := range m.diffLines {
		lineNum := m.styles.LineNumber.Render(formatLineNum(dl))
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
	}
	return b.String()
}

// formatLineNum formats old/new line numbers for display.
func formatLineNum(dl diff.DiffLine) string {
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

// syncViewportToCursor adjusts viewport scroll to keep cursor visible and re-renders content.
func (m *Model) syncViewportToCursor() {
	if m.diffCursor < m.viewport.YOffset {
		m.viewport.SetYOffset(m.diffCursor)
	} else if m.diffCursor >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.diffCursor - m.viewport.Height + 1)
	}
	m.viewport.SetContent(m.renderDiff())
}
