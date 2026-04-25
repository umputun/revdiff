package ui

import "strings"

// scrollbar glyphs. track stays the lipgloss right-border default (│),
// thumb replaces it on rows mapped to the visible viewport portion. the
// thumb is heavy-vertical (U+2503): same line geometry and centering as
// the track, just visually thicker — block-style alternatives like ▐ paint
// only one half of the cell and visually misalign with the frame. bold SGR
// is wrapped around the rune to brighten the accent color on the thumb;
// \x1b[22m resets only intensity so the surrounding border bg/fg envelope
// is preserved (vs \x1b[0m which would kill BorderBackground). bold is
// emitted unconditionally including in --no-colors mode: it is an SGR
// attribute, not a color, and most terminals render it as a weight change
// even without color support, which keeps the indicator visible in plain
// mode. this is an intentional deviation from the reverse-video pattern
// other helpers (scrollIndicatorANSI, search highlight) use for no-colors.
const (
	scrollbarTrackRune = "│"
	scrollbarThumbRune = "\x1b[1m┃\x1b[22m"

	// the rendered diff pane has 1 top border row + 1 header row before the
	// viewport rows begin. the single-line header invariant is enforced by
	// truncateHeaderTitle in view.go — if the diff pane's pre-viewport row
	// count ever changes (multi-line header, status pill above viewport,
	// etc.), this offset must be updated in lockstep.
	scrollbarFirstViewportRow = 2
)

// applyScrollbar replaces the right-border rune of viewport rows with a
// thicker thumb glyph (heavy-vertical, bold) to indicate scroll position.
// no-op when the diff content fits the viewport (nothing to scroll) or the
// viewport has zero height. preserves the ANSI envelope around the replaced
// rune: lipgloss renders the right border as the line's last │ rune, so
// strings.LastIndex finds the rune position and the slice operation swaps
// only that rune — the prefix/suffix bytes (border fg/bg ANSI) stay intact
// regardless of the thumb's added SGR wrap. glyphs ┃ and │ are both 1 cell
// wide so display geometry is unchanged.
//
// also no-ops when the rendered pane's line count differs from the expected
// shape (top border + header + vh viewport rows + bottom border). this is a
// safety net for cases where lipgloss soft-wraps content unexpectedly —
// e.g., narrow terminals where applyHorizontalScroll cannot truncate body
// rows because gutters consume the full content width. better to show no
// thumb than a misplaced one.
func (m Model) applyScrollbar(rendered string) string {
	total := m.layout.viewport.TotalLineCount()
	vh := m.layout.viewport.Height
	if total <= vh || vh <= 0 {
		return rendered
	}

	lines := strings.Split(rendered, "\n")
	// shape check: rendered pane must have at least 2+vh+1 rows (top + header
	// + vh viewport rows + bottom) and at most paneHeight()+2 rows (lipgloss
	// outer height with padding). more than the upper bound means content
	// soft-wrapped somewhere (header or body) and the thumb would land on
	// non-viewport rows; bail. less than the lower bound means the caller
	// passed a truncated render; bail. tests using synthetic input hit the
	// minimum bound; production hits the maximum (vh = paneHeight()-1).
	minRows := scrollbarFirstViewportRow + vh + 1
	maxRows := max(minRows, m.paneHeight()+2)
	if len(lines) < minRows || len(lines) > maxRows {
		return rendered
	}

	thumbSize := min(vh, max(1, vh*vh/total))

	// the divisor below (total - vh) is guaranteed > 0 by the early-return
	// above; no zero-divisor guard needed. when vh == 1, thumbSize == 1 and
	// maxStart == 0, so thumbStart resolves to 0 regardless of yOff and the
	// single thumb row anchors at the only viewport row.
	maxStart := vh - thumbSize
	yOff := m.layout.viewport.YOffset
	thumbStart := min(maxStart, yOff*maxStart/(total-vh))

	for i := range vh {
		if i < thumbStart || i >= thumbStart+thumbSize {
			continue
		}
		rowIdx := scrollbarFirstViewportRow + i
		if rowIdx >= len(lines) {
			break
		}
		idx := strings.LastIndex(lines[rowIdx], scrollbarTrackRune)
		if idx < 0 {
			continue
		}
		lines[rowIdx] = lines[rowIdx][:idx] + scrollbarThumbRune + lines[rowIdx][idx+len(scrollbarTrackRune):]
	}
	return strings.Join(lines, "\n")
}
