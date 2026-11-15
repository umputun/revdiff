package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// diffViewport is the scrollable window over the rendered diff. It keeps the
// subset of bubbles/viewport that revdiff uses (the field names match, so call
// sites read the same) and drops the part that made every keystroke O(file):
// bubbles' SetContent measures the display width of every line, grapheme by
// grapheme, to support its own horizontal scroll. revdiff never uses that
// scroll (layout.scrollX and applyHorizontalScroll cut lines before they get
// here), yet the measurement was 60% of a keystroke on a 50k-line diff.
// Here SetContent only splits, and View measures just the rows it shows.
type diffViewport struct {
	Width   int // visible columns
	Height  int // visible rows
	YOffset int // index of the first visible content row

	lines []string
}

// newDiffViewport returns a viewport of the given size with no content.
func newDiffViewport(width, height int) diffViewport {
	return diffViewport{Width: width, Height: height}
}

// SetContent replaces the content, one row per line. CRLF is normalized to LF
// as bubbles/viewport did. An offset past the new last row moves to the bottom.
func (v *diffViewport) SetContent(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	v.lines = strings.Split(s, "\n")
	if v.YOffset > len(v.lines)-1 {
		v.YOffset = v.maxYOffset()
	}
}

// SetYOffset scrolls so row n is the first visible one, clamped to the content.
func (v *diffViewport) SetYOffset(n int) {
	v.YOffset = min(max(n, 0), v.maxYOffset())
}

// GotoTop scrolls to the first row.
func (v *diffViewport) GotoTop() {
	v.YOffset = 0
}

// TotalLineCount returns the number of content rows.
func (v diffViewport) TotalLineCount() int {
	return len(v.lines)
}

// View renders the visible rows padded to Width x Height. A row wider than
// Width is cut to it first: lipgloss would otherwise soft-wrap it into extra
// rows and push the bottom of the window out of view.
func (v diffViewport) View() string {
	return lipgloss.NewStyle().
		Width(v.Width).
		Height(v.Height).
		MaxHeight(v.Height).
		MaxWidth(v.Width).
		Render(strings.Join(v.visibleLines(), "\n"))
}

// visibleLines returns the rows inside the window, each cut to Width when wider.
// It allocates a copy only when a row actually needs cutting.
func (v diffViewport) visibleLines() []string {
	if len(v.lines) == 0 {
		return nil
	}
	top := max(v.YOffset, 0)
	bottom := min(max(v.YOffset+v.Height, top), len(v.lines))
	lines := v.lines[top:bottom]
	if v.Width == 0 {
		return lines
	}
	var cut []string
	for i, l := range lines {
		if ansi.StringWidth(l) <= v.Width {
			continue
		}
		if cut == nil {
			cut = make([]string, len(lines))
			copy(cut, lines)
		}
		cut[i] = ansi.Cut(l, 0, v.Width)
	}
	if cut == nil {
		return lines
	}
	return cut
}

func (v diffViewport) maxYOffset() int {
	return max(0, len(v.lines)-v.Height)
}
