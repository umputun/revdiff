package ui

import "strings"

// scrollbar glyphs. track stays the lipgloss right-border default (│),
// thumb replaces it on rows mapped to the visible viewport portion. the
// thumb is heavy-vertical (U+2503): same line geometry and centering as
// the track, just visually thicker — block-style alternatives like ▐ paint
// only one half of the cell and visually misalign with the frame. bold SGR
// is wrapped around the rune to brighten the accent color on the thumb;
// \x1b[22m resets only intensity so the surrounding border bg/fg envelope
// is preserved (vs \x1b[0m which would kill BorderBackground).
const (
	scrollbarTrackRune = "│"
	scrollbarThumbRune = "\x1b[1m┃\x1b[22m"

	// the rendered diff pane has 1 top border row + 1 header row before
	// the viewport rows begin. corners and bottom border are skipped.
	scrollbarFirstViewportRow = 2
)

// applyScrollbar replaces the right-border rune of viewport rows with a
// half-block thumb glyph to indicate scroll position. no-op when the diff
// content fits the viewport (nothing to scroll). preserves ANSI envelope
// around the replaced rune since both glyphs are 3-byte UTF-8 box-drawing
// chars and lipgloss renders the right border as the line's last │ rune.
func (m Model) applyScrollbar(rendered string) string {
	total := m.layout.viewport.TotalLineCount()
	vh := m.layout.viewport.Height
	if total <= vh || vh <= 0 {
		return rendered
	}

	thumbSize := min(vh, max(1, vh*vh/total))

	maxStart := vh - thumbSize
	yOff := m.layout.viewport.YOffset
	var thumbStart int
	if maxStart > 0 {
		thumbStart = min(maxStart, yOff*maxStart/(total-vh))
	}

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
