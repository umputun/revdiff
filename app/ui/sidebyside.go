package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// side-by-side mode shows the old text on the left and the new text on the right, one row per
// change pair (like Gerrit). it reuses the collapsed-mode "hidden line" machinery: a removed
// line that is paired with an added line is hidden (zero height, skipped by cursor, search and
// hunk navigation) and painted on the left half of its add's row instead. unpaired removes and
// adds keep their own row with the opposite half blank, context lines fill both halves.
//
// limitations by design: lines are cut (never wrapped) and scroll horizontally together, the
// blame gutter is not shown, and an annotation on a removed line that is paired with an added
// line is not painted (it stays in the store and in the annotation list, jumping to it lands
// on the paired add).

// sbsSeparatorWidth is the width of the column between the two halves.
const sbsSeparatorWidth = 1

// sideBySideActive reports whether the side-by-side layout is in effect. collapsed mode hides
// removed lines entirely, so the two are mutually exclusive and collapsed wins.
func (m Model) sideBySideActive() bool {
	return m.modes.sideBySide && !m.modes.collapsed.enabled
}

// computeSBSPartners pairs removed and added lines of every change block: the k-th add is shown beside
// the k-th of the block's last removes (or of all of them when there are fewer removes than adds). the result is parallel to file.lines and holds the partner
// index (symmetric for both members of a pair) or -1.
func (m Model) computeSBSPartners() []int {
	n := len(m.file.lines)
	partners := make([]int, n)
	for i := range partners {
		partners[i] = -1
	}
	isType := func(i int, ct diff.ChangeType) bool { return i < n && m.file.lines[i].ChangeType == ct }
	for i := 0; i < n; {
		if !isType(i, diff.ChangeRemove) && !isType(i, diff.ChangeAdd) {
			i++
			continue
		}
		removeStart := i
		for isType(i, diff.ChangeRemove) {
			i++
		}
		addStart := i
		for isType(i, diff.ChangeAdd) {
			i++
		}
		// surplus removes sit before the pairs, so the old side keeps its order: the rows of a
		// block are its unpaired removes, then the pairs, then its unpaired adds.
		nRemoves, nAdds := addStart-removeStart, i-addStart
		offset := max(0, nRemoves-nAdds)
		for k := 0; k < min(nRemoves, nAdds); k++ {
			partners[removeStart+offset+k] = addStart + k
			partners[addStart+k] = removeStart + offset + k
		}
	}
	return partners
}

// sbsPartner returns the paired line index for idx, or -1. falls back to a fresh computation when
// the cache fell out of step with file.lines (a caller that set the lines without a load).
func (m Model) sbsPartner(idx int) int {
	if idx < 0 || idx >= len(m.file.lines) {
		return -1
	}
	if len(m.file.sbsPartners) == len(m.file.lines) {
		return m.file.sbsPartners[idx]
	}
	return m.computeSBSPartners()[idx]
}

// isSBSHidden reports whether the line at idx is a removed line rendered inside its add's row.
func (m Model) isSBSHidden(idx int) bool {
	if !m.sideBySideActive() || idx < 0 || idx >= len(m.file.lines) {
		return false
	}
	return m.file.lines[idx].ChangeType == diff.ChangeRemove && m.sbsPartner(idx) >= 0
}

// sbsHalfWidths returns the display widths of the left and right halves. together with the
// separator they fill diffContentWidth.
func (m Model) sbsHalfWidths() (left, right int) {
	total := m.diffContentWidth() - sbsSeparatorWidth
	left = total / 2
	return left, total - left
}

// sbsNumWidth returns the width of the per-half line number gutter, 0 when line numbers are off.
func (m Model) sbsNumWidth() int {
	if !m.modes.lineNumbers {
		return 0
	}
	return m.file.lineNumWidth + 1
}

// sbsVisibleWidth returns the content columns available in the narrower half.
func (m Model) sbsVisibleWidth() int {
	left, right := m.sbsHalfWidths()
	return min(left, right) - m.sbsNumWidth()
}

// renderSideBySideLine writes the row for the diff line at idx. dividers span the full width.
func (m Model) renderSideBySideLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	if dl.ChangeType == diff.ChangeDivider {
		m.renderDiffLine(b, idx, dl)
		return
	}
	leftW, rightW := m.sbsHalfWidths()
	match := m.search.matchSet[idx]

	var left, right string
	switch dl.ChangeType { //nolint:exhaustive // dividers returned above
	case diff.ChangeRemove:
		left = m.sbsHalf(idx, dl, leftW, true, match)
		right = m.sbsBlank(rightW)
	case diff.ChangeAdd:
		if p := m.sbsPartner(idx); p >= 0 {
			// search matches on a paired remove are not navigable (it is hidden), so they are not painted either
			left = m.sbsHalf(p, m.file.lines[p], leftW, true, false)
		} else {
			left = m.sbsBlank(leftW)
		}
		right = m.sbsHalf(idx, dl, rightW, false, match)
	default: // context
		left = m.sbsHalf(idx, dl, leftW, true, match)
		right = m.sbsHalf(idx, dl, rightW, false, match)
	}

	cursor := " "
	if m.isCursorLine(idx) {
		cursor = m.renderer.DiffCursor(m.cfg.noColors)
	}
	sep := m.resolver.Style(style.StyleKeyLineNumber).Render("│")
	b.WriteString(cursor + left + sep + right + "\n")
}

// sbsHalf renders one half of a row: optional line number, then the styled and horizontally cut
// content, padded to exactly width columns. left selects the old line number, otherwise the new one.
func (m Model) sbsHalf(idx int, dl diff.DiffLine, width int, left, searchMatch bool) string {
	_, text, hasHighlight := m.prepareLineContent(idx, dl)
	text = m.applyIntraLineHighlight(idx, dl.ChangeType, text)

	num := ""
	if m.modes.lineNumbers {
		n := dl.NewNum
		if left {
			n = dl.OldNum
		}
		col := strings.Repeat(" ", m.file.lineNumWidth)
		if n > 0 {
			col = fmt.Sprintf("%*d", m.file.lineNumWidth, n)
		}
		num = m.resolver.Style(style.StyleKeyLineNumber).Render(" " + col)
	}

	avail := max(0, width-m.sbsNumWidth())
	content := m.styleDiffContent(dl.ChangeType, m.linePrefix(dl.ChangeType), text, hasHighlight, searchMatch)
	content = ansi.Cut(content, m.layout.scrollX, m.layout.scrollX+avail)
	return num + m.sbsPad(content, avail, m.resolver.LineBg(dl.ChangeType))
}

// sbsBlank returns an empty half of the given width, used opposite an unpaired add or remove.
func (m Model) sbsBlank(width int) string {
	return m.sbsPad("", width, "")
}

// sbsPad pads styled to width columns with bg (the pane background when bg is empty). unlike
// extendLineBg it always pads, since a short half would shift the other half out of its column.
func (m Model) sbsPad(styled string, width int, bg style.Color) string {
	pad := width - lipgloss.Width(styled)
	if pad <= 0 {
		return styled
	}
	if bg == "" {
		bg = m.resolver.Color(style.ColorKeyDiffPaneBg)
	}
	if bg == "" {
		return styled + strings.Repeat(" ", pad)
	}
	return styled + string(bg) + strings.Repeat(" ", pad) + "\033[49m"
}

// toggleSideBySide switches between the unified and the side-by-side layout. turning it on
// turns collapsed mode off, since the two cannot be shown together.
func (m *Model) toggleSideBySide() {
	if m.layout.focus != paneDiff || m.file.name == "" {
		return
	}
	m.modes.sideBySide = !m.modes.sideBySide
	if m.modes.sideBySide {
		m.modes.collapsed.enabled = false
		m.modes.collapsed.expandedHunks = make(map[int]bool)
	}
	m.annot.cursorOnAnnotation = false // visible rows change, reset annotation cursor state
	m.adjustCursorIfHidden()
	m.realignSearchCursor()
	m.clampHorizontalScroll() // the visible width per half differs from the unified width
	m.syncViewportToCursor()
}
