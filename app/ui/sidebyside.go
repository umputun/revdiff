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
// line that is paired with an added line is hidden (zero height, skipped by cursor and hunk
// navigation) and painted on the left half of its add's row instead. unpaired removes and adds
// keep their own row with the opposite half blank, context lines fill both halves.
//
// the old side of a folded pair has no cursor stop of its own, so, as in collapsed mode, a change
// region can be expanded with the toggle-hunk key: its removes then get their own rows and can be
// annotated like any line. regions that hold an annotation on a removed line, and the target of a
// jump or search match on a folded remove, expand on their own.
//
// lines are cut (never wrapped) and scroll horizontally together; the blame gutter is not shown.

// sbsSeparatorWidth is the width of the column between the two halves.
const sbsSeparatorWidth = 1

// sideBySideActive reports whether the side-by-side layout is in effect. collapsed mode hides
// removed lines entirely, so the two are mutually exclusive and collapsed wins. a file without
// changes (--only, --all-files) would show the same text twice, so it stays unified.
func (m Model) sideBySideActive() bool {
	return m.modes.sideBySide && !m.modes.collapsed.enabled && m.file.adds+m.file.removes > 0
}

// computeSBSPairing pairs removed and added lines of every change block and numbers the change
// regions. partners is parallel to file.lines and holds the partner index (symmetric for both
// members of a pair) or -1: the k-th add is shown beside the k-th of the block's last removes (or
// of all of them when there are fewer removes than adds), so surplus removes sit before the pairs
// and the old side keeps its order. regions holds, for every added or removed line, the index of
// the first line of its contiguous run of changes (-1 for other lines); a region is what the
// toggle-hunk key expands.
func (m Model) computeSBSPairing() (partners, regions []int) {
	n := len(m.file.lines)
	partners = make([]int, n)
	regions = make([]int, n)
	for i := range partners {
		partners[i], regions[i] = -1, -1
	}
	isType := func(i int, ct diff.ChangeType) bool { return i < n && m.file.lines[i].ChangeType == ct }
	for i := 0; i < n; {
		if !isType(i, diff.ChangeRemove) && !isType(i, diff.ChangeAdd) {
			i++
			continue
		}
		regionStart := i
		for i < n && (isType(i, diff.ChangeRemove) || isType(i, diff.ChangeAdd)) {
			regions[i] = regionStart
			removeStart := i
			for isType(i, diff.ChangeRemove) {
				regions[i] = regionStart
				i++
			}
			addStart := i
			for isType(i, diff.ChangeAdd) {
				regions[i] = regionStart
				i++
			}
			// surplus removes sit before the pairs, so the rows of a block are its unpaired removes,
			// then the pairs, then its unpaired adds
			nRemoves, nAdds := addStart-removeStart, i-addStart
			offset := max(0, nRemoves-nAdds)
			for k := range min(nRemoves, nAdds) {
				partners[removeStart+offset+k] = addStart + k
				partners[addStart+k] = removeStart + offset + k
			}
		}
	}
	return partners, regions
}

// sbsPair returns the line shown beside idx on the other half of its row, or -1. an expanded
// region shows nothing beside its lines: every line has a row of its own.
func (m Model) sbsPair(idx int) int {
	if idx < 0 || idx >= len(m.file.sbsPartners) || m.sbsExpanded(idx) {
		return -1
	}
	return m.file.sbsPartners[idx]
}

// sbsExpanded reports whether the change region holding idx was expanded.
func (m Model) sbsExpanded(idx int) bool {
	if idx < 0 || idx >= len(m.file.sbsRegions) {
		return false
	}
	region := m.file.sbsRegions[idx]
	return region >= 0 && m.modes.sbsExpanded[region]
}

// isSBSHidden reports whether the line at idx is a removed line rendered inside its add's row.
func (m Model) isSBSHidden(idx int) bool {
	if !m.sideBySideActive() || idx < 0 || idx >= len(m.file.lines) {
		return false
	}
	return m.file.lines[idx].ChangeType == diff.ChangeRemove && m.sbsPair(idx) >= 0
}

// expandSBSRegion expands the change region holding idx.
// the intra-line ranges depend on which lines share a row, so they are recomputed with the caches.
func (m *Model) expandSBSRegion(idx int) {
	if idx < 0 || idx >= len(m.file.sbsRegions) || m.file.sbsRegions[idx] < 0 || m.sbsExpanded(idx) {
		return
	}
	if m.modes.sbsExpanded == nil {
		m.modes.sbsExpanded = make(map[int]bool)
	}
	m.modes.sbsExpanded[m.file.sbsRegions[idx]] = true
	m.refreshSBSLayout()
}

// refreshSBSLayout recomputes what depends on which lines share a row after the fold state changed.
func (m *Model) refreshSBSLayout() {
	m.modes.sbsEpoch++
	m.recomputeIntraRanges()
	m.invalidateRenderCaches()
}

// expandAnnotatedSBSRegions expands every region that holds an annotation on a removed line that
// would otherwise be folded away: such a note is only painted on a row of its own.
func (m *Model) expandAnnotatedSBSRegions() {
	if !m.modes.sideBySide {
		return
	}
	for _, a := range m.store.Get(m.file.name) {
		if a.Line == 0 || a.Type != string(diff.ChangeRemove) {
			continue
		}
		if idx := m.findDiffLineIndex(a.Line, a.Type); idx >= 0 && m.file.sbsPartners[idx] >= 0 {
			m.expandSBSRegion(idx)
		}
	}
}

// regionHasRemoveAnnotation reports whether any removed line of the region starting at start carries an annotation.
func (m Model) regionHasRemoveAnnotation(start int) bool {
	for i := start; i < len(m.file.lines) && m.file.sbsRegions[i] == start; i++ {
		dl := m.file.lines[i]
		if dl.ChangeType != diff.ChangeRemove {
			continue
		}
		for _, a := range m.store.Get(m.file.name) {
			if a.Line == dl.OldNum && a.Type == string(diff.ChangeRemove) {
				return true
			}
		}
	}
	return false
}

// toggleSBSRegion expands the change region under the cursor, or folds it back. a region with a
// note on a removed line stays expanded, since the note is only painted on a row of its own.
func (m *Model) toggleSBSRegion() {
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.sbsRegions) {
		return
	}
	region := m.file.sbsRegions[m.nav.diffCursor]
	if region < 0 {
		return
	}
	if !m.modes.sbsExpanded[region] {
		m.expandSBSRegion(m.nav.diffCursor)
	} else {
		if m.regionHasRemoveAnnotation(region) {
			m.compact.hint = "region has a note on a removed line"
			return
		}
		delete(m.modes.sbsExpanded, region)
		m.annot.cursorOnAnnotation = false
		m.refreshSBSLayout()
		m.adjustCursorIfHidden()
	}
	m.realignSearchCursor()
	m.syncViewportToCursor()
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

// sbsSide selects which half of a row a line is painted on.
type sbsSide int

const (
	sbsOld sbsSide = iota // left half, shows the old line number
	sbsNew                // right half, shows the new line number
)

// renderSideBySideLine writes the row for the diff line at idx. dividers span the full width.
func (m Model) renderSideBySideLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	if dl.ChangeType == diff.ChangeDivider {
		m.renderDiffLine(b, idx, dl)
		return
	}
	leftW, rightW := m.sbsHalfWidths()
	match := m.search.matchSet[idx]

	var left, right string
	switch dl.ChangeType {
	case diff.ChangeRemove:
		left = m.sbsHalf(idx, dl, leftW, sbsOld, match)
		right = m.sbsBlank(rightW)
	case diff.ChangeAdd:
		if p := m.sbsPair(idx); p >= 0 {
			left = m.sbsHalf(p, m.file.lines[p], leftW, sbsOld, m.search.matchSet[p])
		} else {
			left = m.sbsBlank(leftW)
		}
		right = m.sbsHalf(idx, dl, rightW, sbsNew, match)
	default: // context
		left = m.sbsHalf(idx, dl, leftW, sbsOld, match)
		right = m.sbsHalf(idx, dl, rightW, sbsNew, match)
	}

	cursor := " "
	if m.isCursorLine(idx) {
		cursor = m.renderer.DiffCursor(m.cfg.noColors)
	}
	b.WriteString(cursor + left + m.sbsSeparator() + right + "\n")
}

// sbsSeparator returns the column between the two halves.
func (m Model) sbsSeparator() string {
	return m.resolver.Style(style.StyleKeyLineNumber).Render("│")
}

// sbsHalf renders one half of a row: optional line number, then the styled and horizontally
// scrolled content with the «/» overflow markers, padded to exactly width columns.
func (m Model) sbsHalf(idx int, dl diff.DiffLine, width int, side sbsSide, searchMatch bool) string {
	_, text, hasHighlight := m.prepareLineContent(idx, dl)
	text = m.applyIntraLineHighlight(idx, dl.ChangeType, text)

	num := ""
	if m.modes.lineNumbers {
		n := dl.NewNum
		if side == sbsOld {
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
	// the right marker reaches one column past its cut width, so cut one short to stay inside the half
	content = m.scrollHorizontally(content, m.resolver.IndicatorBg(dl.ChangeType), avail-1)
	if lipgloss.Width(content) > avail {
		content = ansi.Cut(content, 0, avail) // a half too narrow for markers
	}
	return num + m.sbsPad(content, avail, m.resolver.LineBg(dl.ChangeType))
}

// padSideBySideRows appends empty rows, separator included, until the content fills the viewport,
// so the two columns run to the bottom of the pane instead of ending with the last diff line.
// the padding is not cached: it depends on the viewport height, which is not part of the cache key.
func (m Model) padSideBySideRows(b *strings.Builder) {
	rows := strings.Count(b.String(), "\n")
	if rows >= m.layout.viewport.Height {
		return
	}
	leftW, rightW := m.sbsHalfWidths()
	row := " " + m.sbsBlank(leftW) + m.sbsSeparator() + m.sbsBlank(rightW) + "\n"
	b.WriteString(strings.Repeat(row, m.layout.viewport.Height-rows))
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
	m.modes.sideBySide = !m.modes.sideBySide || m.modes.collapsed.enabled // collapsed wins until this press turns it off
	if m.modes.sideBySide {
		m.modes.collapsed.enabled = false
		m.modes.collapsed.expandedHunks = make(map[int]bool)
	} else if m.modes.wrap {
		m.layout.scrollX = 0 // wrap mode cannot scroll back
	}
	m.annot.cursorOnAnnotation = false // visible rows change, reset annotation cursor state
	m.expandAnnotatedSBSRegions()
	m.refreshSBSLayout()
	m.adjustCursorIfHidden()
	m.realignSearchCursor()
	m.clampHorizontalScroll() // the visible width per half differs from the unified width
	m.syncViewportToCursor()
}
