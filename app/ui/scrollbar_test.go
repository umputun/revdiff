package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildPaneRender returns a synthetic lipgloss-shaped pane render with vh
// viewport rows. layout: top border, header row, vh viewport rows, bottom
// border. inner width is fixed; corners are real box-drawing chars so the
// corner-safety assertions are meaningful.
func buildPaneRender(vh, innerWidth int) string {
	pad := strings.Repeat("─", innerWidth)
	body := strings.Repeat(" ", innerWidth)
	lines := make([]string, 0, vh+3)
	lines = append(lines, "┌"+pad+"┐", "│"+body+"│") // top border + header
	for range vh {
		lines = append(lines, "│"+body+"│")
	}
	lines = append(lines, "└"+pad+"┘")
	return strings.Join(lines, "\n")
}

// countThumb counts thumb glyphs across all lines of the rendered pane.
func countThumb(s string) int {
	return strings.Count(s, scrollbarThumbRune)
}

// thumbRows returns 0-based row indices (in the full rendered output) where
// the thumb glyph appears at end-of-line position.
func thumbRows(s string) []int {
	rows := []int{}
	for i, line := range strings.Split(s, "\n") {
		if strings.Contains(line, scrollbarThumbRune) {
			rows = append(rows, i)
		}
	}
	return rows
}

func TestApplyScrollbar_NoOpWhenContentFits(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 10
	m.layout.viewport.SetContent(strings.Repeat("x\n", 5)) // 6 lines (split on \n yields trailing empty)

	in := buildPaneRender(10, 20)
	out := m.applyScrollbar(in)
	assert.Equal(t, in, out)
	assert.Zero(t, countThumb(out))
}

func TestApplyScrollbar_NoOpWhenViewportZero(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 0
	m.layout.viewport.SetContent(strings.Repeat("x\n", 100))

	in := buildPaneRender(0, 20)
	out := m.applyScrollbar(in)
	assert.Equal(t, in, out)
}

func TestApplyScrollbar_ThumbAtTop(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 10
	// 100 total lines, viewport sees 10 → thumb size = 10*10/100 = 1
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99))
	m.layout.viewport.SetYOffset(0)

	out := m.applyScrollbar(buildPaneRender(10, 20))
	rows := thumbRows(out)
	require.Len(t, rows, 1, "thumb size should be 1 row")
	// rendered layout: row 0 = top border, row 1 = header, viewport rows start at 2
	assert.Equal(t, []int{2}, rows, "thumb at YOffset=0 should be on first viewport row")
}

func TestApplyScrollbar_ThumbAtBottom(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 10
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99)) // 100 lines total
	m.layout.viewport.SetYOffset(90)                        // fully scrolled (total - vh = 90)

	out := m.applyScrollbar(buildPaneRender(10, 20))
	rows := thumbRows(out)
	require.Len(t, rows, 1)
	// last viewport row is index 2 + vh - 1 = 11
	assert.Equal(t, []int{11}, rows, "thumb at fully-scrolled should be on last viewport row")
}

func TestApplyScrollbar_ThumbProportionalSize(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 20
	// 40 total lines, viewport 20 → thumb size = 20*20/40 = 10
	m.layout.viewport.SetContent(strings.Repeat("x\n", 39))
	m.layout.viewport.SetYOffset(0)

	out := m.applyScrollbar(buildPaneRender(20, 20))
	rows := thumbRows(out)
	// thumb spans the first 10 viewport rows, contiguous, anchored at top
	want := []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	assert.Equal(t, want, rows)
}

func TestApplyScrollbar_ThumbMinimumSizeOne(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 5
	// 10000 total lines → thumb size = 5*5/10000 = 0, clamped to 1
	m.layout.viewport.SetContent(strings.Repeat("x\n", 9999))
	m.layout.viewport.SetYOffset(0)

	out := m.applyScrollbar(buildPaneRender(5, 20))
	assert.Equal(t, 1, countThumb(out), "thumb size must be at least 1")
}

func TestApplyScrollbar_ThumbMovesWithOffset(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 10
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99)) // 100 lines

	// thumb size = 10*10/100 = 1, max start = 9, denominator = 90
	tests := []struct {
		yOff     int
		wantRow0 int // index relative to viewport (0..9)
	}{
		{yOff: 0, wantRow0: 0},
		{yOff: 45, wantRow0: 4}, // 45*9/90 = 4
		{yOff: 90, wantRow0: 9}, // fully scrolled
	}
	for _, tt := range tests {
		m.layout.viewport.SetYOffset(tt.yOff)
		out := m.applyScrollbar(buildPaneRender(10, 20))
		rows := thumbRows(out)
		require.Len(t, rows, 1, "yOff=%d", tt.yOff)
		assert.Equal(t, scrollbarFirstViewportRow+tt.wantRow0, rows[0], "yOff=%d", tt.yOff)
	}
}

func TestApplyScrollbar_PreservesAnsiEnvelope(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 5
	m.layout.viewport.SetContent(strings.Repeat("x\n", 19)) // 20 lines, thumb size = 5*5/20 = 1
	m.layout.viewport.SetYOffset(0)

	// synthesize a row with ANSI envelope around the right border
	const left = "\x1b[34m│\x1b[0m"
	const right = "\x1b[34m│\x1b[0m"
	body := "  body  "

	rendered := strings.Join([]string{
		"\x1b[34m┌────────┐\x1b[0m", // top border
		left + "header  " + right,   // header
		left + body + right,         // viewport row 0 (thumb here)
		left + body + right,         // viewport row 1
		left + body + right,         // viewport row 2
		left + body + right,         // viewport row 3
		left + body + right,         // viewport row 4
		"\x1b[34m└────────┘\x1b[0m", // bottom border
	}, "\n")

	out := m.applyScrollbar(rendered)
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 8)

	// row 2 should contain thumb in the same ANSI envelope; right border replaced
	assert.Equal(t, "\x1b[34m│\x1b[0m"+body+"\x1b[34m"+scrollbarThumbRune+"\x1b[0m", lines[2])

	// other viewport rows untouched
	for _, idx := range []int{3, 4, 5, 6} {
		assert.Equal(t, left+body+right, lines[idx], "row %d should be untouched", idx)
	}
}

func TestApplyScrollbar_NeverModifiesCorners(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 5
	m.layout.viewport.SetContent(strings.Repeat("x\n", 19))
	m.layout.viewport.SetYOffset(15) // fully scrolled — would put thumb on last row

	in := buildPaneRender(5, 10)
	out := m.applyScrollbar(in)
	lines := strings.Split(out, "\n")

	// top border row 0 contains corners ┌ and ┐, never replaced
	assert.Contains(t, lines[0], "┌")
	assert.Contains(t, lines[0], "┐")
	assert.NotContains(t, lines[0], scrollbarThumbRune)

	// bottom border row contains corners └ and ┘, never replaced
	last := len(lines) - 1
	assert.Contains(t, lines[last], "└")
	assert.Contains(t, lines[last], "┘")
	assert.NotContains(t, lines[last], scrollbarThumbRune)

	// header row 1 never contains thumb
	assert.NotContains(t, lines[1], scrollbarThumbRune)
}

func TestApplyScrollbar_PreservesLineCount(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 8
	m.layout.viewport.SetContent(strings.Repeat("x\n", 30))
	m.layout.viewport.SetYOffset(5)

	in := buildPaneRender(8, 30)
	out := m.applyScrollbar(in)
	assert.Len(t, strings.Split(out, "\n"), len(strings.Split(in, "\n")))
}

func TestApplyScrollbar_SafeWhenLinesShorterThanExpected(t *testing.T) {
	// defensive: if some upstream change shortens the rendered output, the
	// function must not panic.
	m := testModel(nil, nil)
	m.layout.viewport.Height = 100
	m.layout.viewport.SetContent(strings.Repeat("x\n", 999))
	m.layout.viewport.SetYOffset(0)

	short := buildPaneRender(3, 10) // 6 lines total
	out := m.applyScrollbar(short)
	// no panic; output length unchanged
	assert.Len(t, strings.Split(out, "\n"), len(strings.Split(short, "\n")))
}

// G3: lock in the bold SGR contract so a future edit that drops the
// \x1b[1m...\x1b[22m wrap in scrollbarThumbRune is caught explicitly. the
// project's CLAUDE.md gotcha entry treats the bold envelope as load-bearing
// (it brightens the accent color to make the thumb pop without resetting
// the border background) — this assertion makes that promise testable.
func TestApplyScrollbar_ThumbWrappedInBoldSGR(t *testing.T) {
	assert.True(t, strings.HasPrefix(scrollbarThumbRune, "\x1b[1m"), "thumb must start with bold SGR")
	assert.True(t, strings.HasSuffix(scrollbarThumbRune, "\x1b[22m"), "thumb must end with intensity-only reset")
	assert.Contains(t, scrollbarThumbRune, "┃", "thumb glyph must be heavy-vertical")
	assert.NotContains(t, scrollbarThumbRune, "\x1b[0m", "thumb must not use full reset (would kill BorderBackground)")
}

// G4: cover the idx<0 branch — a viewport row that does not contain the
// track rune is silently skipped instead of mutating the wrong byte. happens
// in practice when a future caller passes content shorter or differently
// shaped than the lipgloss-rendered pane assumed by applyScrollbar.
func TestApplyScrollbar_RowWithoutTrackRune(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 3
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99)) // forces thumb
	m.layout.viewport.SetYOffset(0)

	// row 2 (first viewport row) has no │ — must be skipped without panic
	rendered := strings.Join([]string{
		"┌────────┐",
		"│header  │",
		"plain row no border",
		"│body    │",
		"│body    │",
		"└────────┘",
	}, "\n")

	out := m.applyScrollbar(rendered)
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 6)
	assert.Equal(t, "plain row no border", lines[2], "row without │ is left intact")
	assert.NotContains(t, lines[2], scrollbarThumbRune)
}

// T2/G5: a viewport row whose content contains literal │ (e.g., reviewing a
// markdown table or box-drawing source) must not have its body │ touched —
// only the rightmost (border) │ is replaced. locks in the assumption
// documented in scrollbar.go's godoc.
func TestApplyScrollbar_BodyContainingTrackRune(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 3
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99))
	m.layout.viewport.SetYOffset(0)

	rendered := strings.Join([]string{
		"┌──────────────┐",
		"│header        │",
		"│ a │ b │ c    │", // body with multiple │
		"│ a │ b │ c    │",
		"│ a │ b │ c    │",
		"└──────────────┘",
	}, "\n")

	out := m.applyScrollbar(rendered)
	lines := strings.Split(out, "\n")
	// row 2 has 4 track runes total: 1 left border + 2 body separators + 1 right border
	// applyScrollbar should replace only the rightmost (border) one
	assert.Equal(t, 3, strings.Count(lines[2], "│"), "body │ separators must be intact, only right border replaced")
	assert.True(t, strings.HasSuffix(lines[2], scrollbarThumbRune), "thumb sits where the right border was")
}

// T1: integration test that runs applyScrollbar against the real lipgloss
// pane render path (with Border + BorderForeground + BorderBackground), so a
// future lipgloss change to the right-border emission shape is caught here.
func TestApplyScrollbar_AgainstRealLipglossOutput(t *testing.T) {
	m := testModel(nil, nil)
	const innerW = 20
	const innerH = 6
	m.layout.viewport.Height = innerH - 1 // 1 row reserved for the header
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99))
	m.layout.viewport.SetYOffset(0)

	header := lipgloss.NewStyle().Render(" header.go")
	body := strings.Repeat("body\n", innerH-1)
	content := lipgloss.JoinVertical(lipgloss.Left, header, body)

	rendered := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#ff8800")).
		BorderBackground(lipgloss.Color("#202020")).
		Width(innerW).
		Height(innerH).
		Render(content)

	out := m.applyScrollbar(rendered)
	require.NotEqual(t, rendered, out, "thumb must be applied")
	assert.Contains(t, out, scrollbarThumbRune, "real lipgloss output must accept the thumb substitution")

	// line count and per-line display width unchanged
	inLines := strings.Split(rendered, "\n")
	outLines := strings.Split(out, "\n")
	require.Len(t, outLines, len(inLines))
	for i := range inLines {
		assert.Equal(t, lipgloss.Width(inLines[i]), lipgloss.Width(outLines[i]), "row %d width must be unchanged", i)
	}
}

// G8: smallest non-trivial viewport. with vh=1 the thumb has nowhere to
// move; this test pins down "no movement, single-row thumb" so a future
// off-by-one in the maxStart math can't sneak in.
func TestApplyScrollbar_ViewportHeightOne(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.viewport.Height = 1
	m.layout.viewport.SetContent(strings.Repeat("x\n", 99)) // 100 lines

	for _, yOff := range []int{0, 50, 99} {
		m.layout.viewport.SetYOffset(yOff)
		out := m.applyScrollbar(buildPaneRender(1, 10))
		rows := thumbRows(out)
		require.Len(t, rows, 1, "yOff=%d must have exactly one thumb row", yOff)
		assert.Equal(t, scrollbarFirstViewportRow, rows[0], "yOff=%d thumb must stay on the only viewport row", yOff)
	}
}
