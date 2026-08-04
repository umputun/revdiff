package overlay

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestHelpOverlay_HandleMouse_ConsumedWithoutClosing(t *testing.T) {
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

// helpTallSpec builds a spec whose two-column form is both taller and wider
// than any ordinary terminal, so size clamping and scrolling are exercised.
func helpTallSpec() HelpSpec {
	sections := make([]HelpSection, 0, 8)
	for s := range 8 {
		entries := make([]HelpEntry, 0, 9)
		for e := range 9 {
			entries = append(entries, HelpEntry{
				Keys:        fmt.Sprintf("k%d%d", s, e),
				Description: fmt.Sprintf("section %d entry %d does something reasonably wordy", s, e),
			})
		}
		sections = append(sections, HelpSection{Title: fmt.Sprintf("Section%d", s), Entries: entries})
	}
	return HelpSpec{Sections: sections}
}

func TestHelpOverlay_FitsTerminal(t *testing.T) {
	sizes := []struct{ w, h int }{{100, 40}, {80, 24}, {60, 20}, {40, 12}, {200, 60}, {30, 8}, {20, 6}, {20, 5}}
	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			mgr := NewManager()
			mgr.OpenHelp(helpTallSpec())
			box := mgr.help.render(RenderCtx{Width: sz.w, Height: sz.h, Resolver: style.PlainResolver()}, mgr)

			assert.LessOrEqual(t, lipgloss.Height(box), sz.h, "popup must not be taller than the terminal")
			assert.LessOrEqual(t, lipgloss.Width(box), sz.w, "popup must not be wider than the terminal")
		})
	}
}

func TestHelpOverlay_SingleColumnWhenNarrow(t *testing.T) {
	spec := HelpSpec{
		Sections: []HelpSection{
			{Title: "Left1", Entries: []HelpEntry{{Keys: "a", Description: "a fairly long description of action a"}}},
			{Title: "Left2", Entries: []HelpEntry{{Keys: "b", Description: "a fairly long description of action b"}}},
			{Title: "Right1", Entries: []HelpEntry{{Keys: "c", Description: "a fairly long description of action c"}}},
			{Title: "Right2", Entries: []HelpEntry{{Keys: "d", Description: "a fairly long description of action d"}}},
		},
	}

	pairedOnOneRow := func(box string) bool {
		for line := range strings.SplitSeq(box, "\n") {
			if strings.Contains(line, "Left1") && strings.Contains(line, "Right1") {
				return true
			}
		}
		return false
	}

	mgr := NewManager()
	mgr.OpenHelp(spec)
	wide := mgr.help.render(RenderCtx{Width: 200, Height: 40, Resolver: style.PlainResolver()}, mgr)
	assert.True(t, pairedOnOneRow(wide), "wide terminal should keep the two-column layout")

	narrow := mgr.help.render(RenderCtx{Width: 60, Height: 40, Resolver: style.PlainResolver()}, mgr)
	assert.False(t, pairedOnOneRow(narrow), "narrow terminal should fall back to a single column")
	assert.Contains(t, narrow, "Right2", "single column must still render every section")
}

func TestHelpOverlay_ScrollKeys(t *testing.T) {
	ctx := RenderCtx{Width: 80, Height: 20, Resolver: style.PlainResolver()}
	mgr := NewManager()
	mgr.OpenHelp(helpTallSpec())
	require.NotContains(t, mgr.help.render(ctx, mgr), "Section7", "last section must start below the fold")

	tests := []struct {
		name   string
		msg    tea.KeyMsg
		action keymap.Action
		want   int
	}{
		{"down", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown, 1},
		{"page down", tea.KeyMsg{Type: tea.KeyPgDown}, keymap.ActionPageDown, 11},
		{"half page down", tea.KeyMsg{Type: tea.KeyCtrlD}, keymap.ActionHalfPageDown, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr.help.offset = 0
			mgr.HandleKey(tt.msg, tt.action)
			assert.Equal(t, tt.want, mgr.help.offset)
		})
	}

	mgr.help.offset = 0
	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, keymap.ActionEnd)
	assert.Contains(t, mgr.help.render(ctx, mgr), "Section7", "End must reach the last section")

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyHome}, keymap.ActionHome)
	assert.Equal(t, 0, mgr.help.offset)
	assert.Contains(t, mgr.help.render(ctx, mgr), "Section0", "Home must return to the top")

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}, "")
	assert.Contains(t, mgr.help.render(ctx, mgr), "Section7", "G must reach the last section")

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}, "")
	assert.Equal(t, 0, mgr.help.offset)
}

func TestHelpOverlay_ScrollKeysClampAtTop(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpTallSpec())
	mgr.help.render(RenderCtx{Width: 80, Height: 20, Resolver: style.PlainResolver()}, mgr)

	for _, action := range []keymap.Action{keymap.ActionUp, keymap.ActionPageUp, keymap.ActionHalfPageUp} {
		mgr.help.offset = 0
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, action)
		assert.Equal(t, 0, mgr.help.offset, "%s must not scroll above the first row", action)
	}
}

func TestHelpOverlay_ScrollWheel(t *testing.T) {
	ctx := RenderCtx{Width: 80, Height: 20, Resolver: style.PlainResolver()}
	mgr := NewManager()
	mgr.OpenHelp(helpTallSpec())
	mgr.help.render(ctx, mgr)

	mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.Equal(t, WheelStep, mgr.help.offset)

	mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	assert.Equal(t, 0, mgr.help.offset)

	mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Shift: true})
	assert.Equal(t, 5, mgr.help.offset, "shift+wheel moves by half a page")

	mgr.help.offset = 4
	mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})
	assert.Equal(t, 4, mgr.help.offset, "non-press wheel events must not scroll")
}

func TestHelpOverlay_ScrollHint(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpTallSpec())
	overflowing := mgr.help.render(RenderCtx{Width: 80, Height: 20, Resolver: style.PlainResolver()}, mgr)
	assert.Contains(t, overflowing, "↑/↓ scroll · 1-", "overflowing help must advertise scrolling")

	mgr.OpenHelp(helpSpec())
	fitting := mgr.help.render(RenderCtx{Width: 120, Height: 40, Resolver: style.PlainResolver()}, mgr)
	assert.NotContains(t, fitting, "↑/↓ scroll", "help that fits must not show a scroll hint")
}

func TestHelpOverlay_OpenResetsScroll(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(helpTallSpec())
	mgr.help.render(RenderCtx{Width: 80, Height: 20, Resolver: style.PlainResolver()}, mgr)
	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnd}, keymap.ActionEnd)
	require.NotZero(t, mgr.help.offset)

	mgr.OpenHelp(helpTallSpec())
	assert.Equal(t, 0, mgr.help.offset, "reopening help must start at the top")
}

func TestHelpOverlay_BoxWidthStableWhileScrolling(t *testing.T) {
	ctx := RenderCtx{Width: 100, Height: 24, Resolver: style.PlainResolver()}
	mgr := NewManager()
	mgr.OpenHelp(helpTallSpec())

	want := lipgloss.Width(mgr.help.render(ctx, mgr))
	for range 6 {
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown}, keymap.ActionPageDown)
		assert.Equal(t, want, lipgloss.Width(mgr.help.render(ctx, mgr)),
			"popup width must not change as the body scrolls")
	}
}

// pins issue #304: on a ~100x40 terminal the popup used to render at 41x122 and
// get clipped on all four sides, hiding whole sections with no way to reach them.
func TestHelpOverlay_AllSectionsReachableOnSmallTerminal(t *testing.T) {
	ctx := RenderCtx{Width: 100, Height: 40, Resolver: style.PlainResolver()}
	spec := helpTallSpec()
	mgr := NewManager()
	mgr.OpenHelp(spec)

	seen := map[string]bool{}
	for range 40 {
		box := mgr.help.render(ctx, mgr)
		require.LessOrEqual(t, lipgloss.Height(box), ctx.Height)
		require.LessOrEqual(t, lipgloss.Width(box), ctx.Width)
		for _, sec := range spec.Sections {
			if strings.Contains(box, sec.Title) {
				seen[sec.Title] = true
			}
			for _, e := range sec.Entries {
				if strings.Contains(box, e.Description) {
					seen[e.Description] = true
				}
			}
		}
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown}, keymap.ActionPageDown)
	}

	for _, sec := range spec.Sections {
		assert.True(t, seen[sec.Title], "section %q must be reachable by scrolling", sec.Title)
		for _, e := range sec.Entries {
			assert.True(t, seen[e.Description], "entry %q must be reachable by scrolling", e.Description)
		}
	}
}

func TestHelpOverlay_TruncationMarksCutRows(t *testing.T) {
	spec := HelpSpec{
		Sections: []HelpSection{
			{Title: "Search", Entries: []HelpEntry{
				{Keys: "↓ / Ctrl+N", Description: "recall next search query / clear (in search prompt)"},
			}},
		},
	}
	mgr := NewManager()
	mgr.OpenHelp(spec)

	narrow := mgr.help.render(RenderCtx{Width: 60, Height: 40, Resolver: style.PlainResolver()}, mgr)
	assert.Contains(t, narrow, helpTruncTail, "a row cut by a narrow terminal must say it was cut")
	assert.LessOrEqual(t, lipgloss.Width(narrow), 60, "the marker must not push the popup past the terminal")

	wide := mgr.help.render(RenderCtx{Width: 200, Height: 40, Resolver: style.PlainResolver()}, mgr)
	assert.NotContains(t, wide, helpTruncTail, "a row that fits must not be marked")
	assert.Contains(t, wide, "recall next search query / clear (in search prompt)")
}
