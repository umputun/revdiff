package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// diffLinesForLabel builds a file with a leading context block followed by
// removes then adds, so change lines sit below a short viewport.
func diffLinesForLabel(adds, removes int) []diff.DiffLine {
	var lines []diff.DiffLine
	for i := 0; i < 20; i++ {
		lines = append(lines, diff.DiffLine{ChangeType: diff.ChangeContext, OldNum: i + 1, NewNum: i + 1, Content: fmt.Sprintf("ctx %d", i)})
	}
	for i := 0; i < removes; i++ {
		lines = append(lines, diff.DiffLine{ChangeType: diff.ChangeRemove, OldNum: 21 + i, Content: fmt.Sprintf("old %d", i)})
	}
	for i := 0; i < adds; i++ {
		lines = append(lines, diff.DiffLine{ChangeType: diff.ChangeAdd, NewNum: 21 + i, Content: fmt.Sprintf("new %d", i)})
	}
	return lines
}

func labelModel(t *testing.T, adds, removes int) Model {
	t.Helper()
	res := style.NewResolver(style.Colors{
		Border: "#888888", Accent: "#00aaff",
		AddFg: "#00ff00", RemoveFg: "#ff0000",
		Normal: "#cccccc", Muted: "#777777",
	})
	m := testModel([]string{"a.go"}, nil)
	m.resolver = res
	m.cfg.frameLabels = true
	m.file.name = "a.go"
	m.file.lines = diffLinesForLabel(adds, removes)
	m.file.adds, m.file.removes = adds, removes
	m.layout.viewport.Height = 10
	return m
}

func TestModel_ModifiedBelow(t *testing.T) {
	m := labelModel(t, 3, 2) // 20 ctx, then 2 removes, 3 adds

	t.Run("top of file counts all changes below", func(t *testing.T) {
		m.layout.viewport.YOffset = 0
		a, r := m.modifiedBelow()
		assert.Equal(t, 3, a)
		assert.Equal(t, 2, r)
	})

	t.Run("scrolled to end counts nothing below", func(t *testing.T) {
		m.layout.viewport.YOffset = 100
		a, r := m.modifiedBelow()
		assert.Equal(t, 0, a)
		assert.Equal(t, 0, r)
	})

	t.Run("mid-scroll counts only remaining changes below", func(t *testing.T) {
		// lines: 0..19 context, 20 remove#0, 21 remove#1, 22..24 adds.
		// bottom visible row = 20 (remove#0) → below remain 1 remove + 3 adds.
		m.layout.viewport.YOffset = 11 // bottom row = 11+10-1 = 20
		a, r := m.modifiedBelow()
		assert.Equal(t, 3, a)
		assert.Equal(t, 1, r)
	})
}

func TestModel_ModifiedAbove(t *testing.T) {
	m := labelModel(t, 3, 2)

	t.Run("top of file counts nothing above", func(t *testing.T) {
		m.layout.viewport.YOffset = 0
		a, r := m.modifiedAbove()
		assert.Equal(t, 0, a)
		assert.Equal(t, 0, r)
	})

	t.Run("mid-scroll counts changes scrolled past", func(t *testing.T) {
		// first visible row = 22 (add#0) → above are 2 removes + 0 adds.
		m.layout.viewport.YOffset = 22
		a, r := m.modifiedAbove()
		assert.Equal(t, 0, a)
		assert.Equal(t, 2, r)
	})

	t.Run("scrolled to end counts every change above the last visible line", func(t *testing.T) {
		// the last diff line (add#2, idx 24) is always on screen, so it is never
		// "above": above = 2 removes + 2 adds (idx 22,23).
		m.layout.viewport.YOffset = 100 // clamps to the last line index
		a, r := m.modifiedAbove()
		assert.Equal(t, 2, a)
		assert.Equal(t, 2, r)
	})
}

// bottomLine renders a diff pane and returns its bottom border line.
func bottomLine(m Model, width int) string {
	pane := m.resolver.Style(style.StyleKeyDiffPane).Width(width).Height(4).Render("body")
	pl := strings.Split(pane, "\n")
	return pl[len(pl)-1]
}

func topLine(m Model, width int) string {
	pane := m.resolver.Style(style.StyleKeyDiffPane).Width(width).Height(4).Render("body")
	return strings.Split(pane, "\n")[0]
}

func TestModel_BuildBottomLabel(t *testing.T) {
	m := labelModel(t, 5, 3)
	m.layout.viewport.YOffset = 0
	orig := bottomLine(m, 60)

	got, ok := m.buildFrameLabelLine(orig, bottomLeftCorner, bottomRightCorner, m.belowSpans())
	require.True(t, ok)

	assert.Equal(t, lipgloss.Width(orig), lipgloss.Width(got), "label must not change border width")
	plain := ansi.Strip(got)
	assert.Contains(t, plain, "+5/-3 modified lines below")
	assert.True(t, strings.HasPrefix(plain, "└─ +5/-3"), "label starts just after the corner: %q", plain)
	assert.True(t, strings.HasSuffix(plain, "┘"))
	assert.Contains(t, got, string(m.resolver.Color(style.ColorKeyAddLineFg))+"+5")
	assert.Contains(t, got, string(m.resolver.Color(style.ColorKeyRemoveLineFg))+"-3")
}

func TestModel_BuildTopLabel(t *testing.T) {
	m := labelModel(t, 5, 3)
	m.layout.viewport.YOffset = 100 // scrolled to end (last add line stays on screen)
	orig := topLine(m, 60)

	got, ok := m.buildFrameLabelLine(orig, topLeftCorner, topRightCorner, m.aboveSpans())
	require.True(t, ok)

	assert.Equal(t, lipgloss.Width(orig), lipgloss.Width(got))
	plain := ansi.Strip(got)
	assert.Contains(t, plain, "+4/-3 modified lines above")
	assert.True(t, strings.HasPrefix(plain, "┌─ +4/-3"), "label starts just after the corner: %q", plain)
	assert.True(t, strings.HasSuffix(plain, "┐"))
}

func TestModel_BottomLabel_NoChangesBelow(t *testing.T) {
	m := labelModel(t, 2, 1)
	m.layout.viewport.YOffset = 500 // past the end
	got, ok := m.buildFrameLabelLine(bottomLine(m, 50), bottomLeftCorner, bottomRightCorner, m.belowSpans())
	require.True(t, ok)
	assert.Contains(t, ansi.Strip(got), "no modified lines below")
}

func TestModel_TopLabel_NoChangesAbove(t *testing.T) {
	m := labelModel(t, 2, 1)
	m.layout.viewport.YOffset = 0 // at the top
	got, ok := m.buildFrameLabelLine(topLine(m, 50), topLeftCorner, topRightCorner, m.aboveSpans())
	require.True(t, ok)
	assert.Contains(t, ansi.Strip(got), "no modified lines above")
}

func TestModel_BuildLabel_Truncation(t *testing.T) {
	m := labelModel(t, 5, 3)
	m.layout.viewport.YOffset = 0
	orig := bottomLine(m, 20)
	got, ok := m.buildFrameLabelLine(orig, bottomLeftCorner, bottomRightCorner, m.belowSpans())
	require.True(t, ok)
	plain := ansi.Strip(got)
	assert.Equal(t, lipgloss.Width(orig), lipgloss.Width(got))
	assert.Contains(t, plain, "...", "narrow border truncates with ellipsis: %q", plain)
	assert.True(t, strings.HasPrefix(plain, "└─ +5/-3"), "numeric prefix survives truncation: %q", plain)
}

func TestModel_ApplyLabels_FlagOff(t *testing.T) {
	m := labelModel(t, 5, 3)
	m.cfg.frameLabels = false // flag hides the feature
	pane := m.resolver.Style(style.StyleKeyDiffPane).Width(50).Height(4).Render("body")
	assert.Equal(t, pane, m.applyDiffTopLabel(pane), "top label off when flag disabled")
	assert.Equal(t, pane, m.applyDiffBottomLabel(pane), "bottom label off when flag disabled")
}

func TestModel_ApplyLabels_SkipsPureContext(t *testing.T) {
	m := labelModel(t, 0, 0) // flag on but no changes at all
	pane := m.resolver.Style(style.StyleKeyDiffPane).Width(50).Height(4).Render("body")
	assert.Equal(t, pane, m.applyDiffTopLabel(pane), "pure-context file keeps a plain top border")
	assert.Equal(t, pane, m.applyDiffBottomLabel(pane), "pure-context file keeps a plain bottom border")
}

// TestModel_FrameLabelDemo prints the rendered borders at several scroll
// positions so the visual result can be eyeballed with `go test -run Demo -v`.
func TestModel_FrameLabelDemo(t *testing.T) {
	m := labelModel(t, 5, 3)
	m.layout.viewport.Height = 8
	for _, off := range []int{0, 12, 100} {
		m.layout.viewport.YOffset = off
		top, _ := m.buildFrameLabelLine(topLine(m, 60), topLeftCorner, topRightCorner, m.aboveSpans())
		bot, _ := m.buildFrameLabelLine(bottomLine(m, 60), bottomLeftCorner, bottomRightCorner, m.belowSpans())
		t.Logf("off=%d\n  top: %q\n  bot: %q", off, ansi.Strip(top), ansi.Strip(bot))
	}
}
