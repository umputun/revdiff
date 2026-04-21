package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

func descriptionRenderCtx() RenderCtx {
	return RenderCtx{Width: 100, Height: 30, Resolver: style.PlainResolver()}
}

func TestDescriptionOverlay_RenderEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{})
	out := mgr.description.render(descriptionRenderCtx(), mgr)
	assert.Contains(t, out, "no description")
	assert.Contains(t, out, "description")
}

func TestDescriptionOverlay_RenderPlainText(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: "first line\nsecond line"})
	out := mgr.description.render(descriptionRenderCtx(), mgr)
	assert.Contains(t, out, "first line")
	assert.Contains(t, out, "second line")
}

func TestDescriptionOverlay_RenderHighlightedMarkdown(t *testing.T) {
	mgr := NewManager()
	hl := stubHighlighter{result: []string{"\033[1mhello\033[22m", "world"}}
	mgr.OpenDescription(DescriptionSpec{Text: "# Hello\n\nworld", Highlighter: hl})
	out := mgr.description.render(descriptionRenderCtx(), mgr)
	assert.Contains(t, out, "hello", "highlighted content passed through")
	assert.Contains(t, out, "world")
}

func TestDescriptionOverlay_RenderLongContentWraps(t *testing.T) {
	long := strings.Repeat("word ", 80)
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: long})
	out := mgr.description.render(descriptionRenderCtx(), mgr)
	// wraps into multiple visible rows
	assert.Greater(t, strings.Count(out, "word"), 4)
}

func TestDescriptionOverlay_ScrollJK(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: body})
	// force a render so height is known
	_ = mgr.description.render(descriptionRenderCtx(), mgr)

	mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, 1, mgr.description.offset)

	mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 0, mgr.description.offset)

	// k at 0 must stay at 0
	mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 0, mgr.description.offset)
}

func TestDescriptionOverlay_ScrollGG(t *testing.T) {
	body := strings.Repeat("line\n", 200)
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: body})
	_ = mgr.description.render(descriptionRenderCtx(), mgr)

	mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}, "")
	assert.Equal(t, scrollEndSentinel, mgr.description.offset)

	mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}, "")
	assert.Equal(t, 0, mgr.description.offset)
}

func TestDescriptionOverlay_DClosesOverlay(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: "hi"})
	out := mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}}, keymap.ActionDescription)
	assert.Equal(t, OutcomeClosed, out.Kind)
}

func TestDescriptionOverlay_QClosesOverlay(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: "hi"})
	out := mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit)
	assert.Equal(t, OutcomeClosed, out.Kind)
}

func TestDescriptionOverlay_EscClosesOverlay(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: "hi"})
	out := mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeClosed, out.Kind)
}

func TestDescriptionOverlay_TitleShowsDescription(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{Text: "body"})
	out := mgr.description.render(descriptionRenderCtx(), mgr)
	assert.Contains(t, out, "description")
}

// stubHighlighter is used only by description overlay tests to simulate
// chroma-formatted markdown without requiring the real highlighter.
type stubHighlighter struct {
	result []string
}

func (s stubHighlighter) HighlightText(_, _ string) []string { return s.result }

// ensure description overlay offsets clamp within bounds when Text is empty.
func TestDescriptionOverlay_EmptyScrollClamp(t *testing.T) {
	mgr := NewManager()
	mgr.OpenDescription(DescriptionSpec{})
	_ = mgr.description.render(descriptionRenderCtx(), mgr)
	// scrolling on empty should not panic or move offset below zero
	mgr.description.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.GreaterOrEqual(t, mgr.description.offset, 0)
	require.NotPanics(t, func() {
		_ = mgr.description.render(descriptionRenderCtx(), mgr)
	})
}
