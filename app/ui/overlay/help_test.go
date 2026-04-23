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

func helpSpec() HelpSpec {
	return HelpSpec{
		Sections: []HelpSection{
			{Title: "Navigation", Entries: []HelpEntry{
				{Keys: "j / ↓", Description: "move down"},
				{Keys: "k / ↑", Description: "move up"},
				{Keys: "PgDn", Description: "page down"},
			}},
			{Title: "Search", Entries: []HelpEntry{
				{Keys: "/", Description: "search in diff"},
				{Keys: "n", Description: "next match"},
				{Keys: "N", Description: "prev match"},
			}},
			{Title: "Quit", Entries: []HelpEntry{
				{Keys: "q", Description: "quit"},
				{Keys: "?", Description: "toggle help"},
			}},
		},
	}
}

func helpRenderCtx() RenderCtx {
	return RenderCtx{Width: 120, Height: 40, Resolver: style.PlainResolver()}
}

func makeBase(width, height int) string {
	line := strings.Repeat(" ", width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func TestHelpOverlay_RenderSections(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())
	result := mgr.help.render(helpRenderCtx(), mgr)

	assert.Contains(t, result, "Navigation")
	assert.Contains(t, result, "Search")
	assert.Contains(t, result, "Quit")
}

func TestHelpOverlay_RenderKeyNames(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())
	result := mgr.help.render(helpRenderCtx(), mgr)

	for _, k := range []string{"j / ↓", "k / ↑", "PgDn", "/", "n", "N", "q", "?"} {
		assert.Contains(t, result, k, "help should contain key: %s", k)
	}
}

func TestHelpOverlay_RenderDescriptions(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())
	result := mgr.help.render(helpRenderCtx(), mgr)

	for _, d := range []string{"move down", "move up", "page down", "search in diff", "next match", "quit"} {
		assert.Contains(t, result, d, "help should contain description: %s", d)
	}
}

func TestHelpOverlay_TwoColumnLayout(t *testing.T) {
	spec := HelpSpec{
		Sections: []HelpSection{
			{Title: "Left1", Entries: []HelpEntry{{Keys: "a", Description: "action a"}}},
			{Title: "Left2", Entries: []HelpEntry{{Keys: "b", Description: "action b"}}},
			{Title: "Right1", Entries: []HelpEntry{{Keys: "c", Description: "action c"}}},
			{Title: "Right2", Entries: []HelpEntry{{Keys: "d", Description: "action d"}}},
		},
	}
	mgr := NewManager()
	mgr.OpenHelp(spec)
	result := mgr.help.render(helpRenderCtx(), mgr)

	assert.Contains(t, result, "Left1")
	assert.Contains(t, result, "Right1")
	assert.Contains(t, result, "action a")
	assert.Contains(t, result, "action d")
}

func TestHelpOverlay_TOCSection(t *testing.T) {
	spec := HelpSpec{
		Sections: []HelpSection{
			{Title: "Navigation", Entries: []HelpEntry{{Keys: "j", Description: "down"}}},
			{Title: "Markdown TOC (single-file full-context mode)", Entries: []HelpEntry{
				{Keys: "Tab", Description: "switch between TOC and diff"},
				{Keys: "j / k", Description: "navigate TOC entries"},
				{Keys: "Enter", Description: "jump to header in diff"},
			}},
		},
	}
	mgr := NewManager()
	mgr.OpenHelp(spec)
	result := mgr.help.render(helpRenderCtx(), mgr)

	assert.Contains(t, result, "Markdown TOC")
	assert.Contains(t, result, "switch between TOC and diff")
	assert.Contains(t, result, "navigate TOC entries")
	assert.Contains(t, result, "jump to header in diff")
}

func TestHelpOverlay_CustomKeybinding(t *testing.T) {
	spec := HelpSpec{
		Sections: []HelpSection{
			{Title: "Quit", Entries: []HelpEntry{
				{Keys: "q / x", Description: "quit"},
			}},
		},
	}
	mgr := NewManager()
	mgr.OpenHelp(spec)
	result := mgr.help.render(helpRenderCtx(), mgr)

	assert.Contains(t, result, "q / x")
	assert.Contains(t, result, "quit")
}

func TestHelpOverlay_EmptySpec(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(HelpSpec{})
	result := mgr.help.render(helpRenderCtx(), mgr)
	require.NotEmpty(t, result, "empty spec should still produce a rendered box")
}

func TestHelpOverlay_ComposeOnBase(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())
	ctx := helpRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	result := mgr.Compose(base, ctx)

	assert.Contains(t, result, "Navigation")
	assert.Contains(t, result, "Search")
	assert.Contains(t, result, "quit")

	lines := strings.Split(result, "\n")
	assert.Len(t, lines, ctx.Height, "composited result should preserve base line count")
}

func TestHelpOverlay_HandleKey_ToggleClose(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())
	require.True(t, mgr.Active())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}, keymap.ActionHelp)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active(), "help should be closed after toggle")
}

func TestHelpOverlay_HandleKey_EscClose(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestHelpOverlay_HandleKey_EscHardcoded(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active(), "esc should close even without ActionDismiss")
}

func TestHelpOverlay_HandleKey_OtherKeysBlocked(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())

	keys := []struct {
		msg    tea.KeyMsg
		action keymap.Action
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, keymap.ActionNextItem},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit},
		{tea.KeyMsg{Type: tea.KeyTab}, keymap.ActionTogglePane},
		{tea.KeyMsg{Type: tea.KeyEnter}, keymap.ActionConfirm},
	}

	for _, k := range keys {
		out := mgr.HandleKey(k.msg, k.action)
		assert.Equal(t, OutcomeNone, out.Kind, "key %v should be consumed without closing", k.msg)
		assert.True(t, mgr.Active(), "key %v should not close help", k.msg)
	}
}

func TestHelpOverlay_HandleMouse_NoOp(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())

	events := []tea.MouseMsg{
		{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
		{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress},
		{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Shift: true},
		{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
	}
	for _, ev := range events {
		out := mgr.HandleMouse(ev)
		assert.Equal(t, OutcomeNone, out.Kind, "mouse event %+v should be consumed", ev)
		assert.True(t, mgr.Active(), "help overlay must stay open through mouse event")
	}
}

func TestHelpOverlay_HandleKey_DismissAction(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}
