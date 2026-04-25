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
// rune since both track and thumb share the same UTF-8 byte width and
// lipgloss renders the right border as the line's last │ rune.
func (m Model) applyScrollbar(rendered string) string {
	total := m.layout.viewport.TotalLineCount()
	vh := m.layout.viewport.Height
	if total <= vh || vh <= 0 {
		return rendered
	}

	thumbSize := min(vh, max(1, vh*vh/total))

	// total > vh (early-return above) and thumbSize is clamped to vh-1 max
	// for any total > vh (since vh*vh/total < vh by integer division), so
	// maxStart >= 1 always reaches here. no zero-divisor guard needed.
	maxStart := vh - thumbSize
	yOff := m.layout.viewport.YOffset
	thumbStart := min(maxStart, yOff*maxStart/(total-vh))

	lines := strings.Split(rendered, "\n")
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
