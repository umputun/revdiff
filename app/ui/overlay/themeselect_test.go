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

func themeItems() []ThemeItem {
	return []ThemeItem{
		{Name: "revdiff", Local: false, AccentColor: "#5f87ff"},
		{Name: "catppuccin-mocha", Local: false, AccentColor: "#cba6f7"},
		{Name: "dracula", Local: false, AccentColor: "#bd93f9"},
		{Name: "my-custom", Local: true, AccentColor: "#ff0000"},
	}
}

func themeSpec() ThemeSelectSpec {
	return ThemeSelectSpec{Items: themeItems(), ActiveName: "revdiff"}
}

func themeRenderCtx() RenderCtx {
	return RenderCtx{Width: 80, Height: 30, Resolver: style.PlainResolver()}
}

func TestThemeSelectOverlay_RenderShowsItems(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	result := mgr.themeSel.render(themeRenderCtx(), mgr)

	assert.Contains(t, result, "revdiff")
	assert.Contains(t, result, "catppuccin-mocha")
	assert.Contains(t, result, "dracula")
	assert.Contains(t, result, "my-custom")
}

func TestThemeSelectOverlay_RenderTitle(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	result := mgr.themeSel.render(themeRenderCtx(), mgr)
	assert.Contains(t, result, "themes (4)")
}

func TestThemeSelectOverlay_RenderFilteredTitle(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "cat"
	mgr.themeSel.applyFilter()
	result := mgr.themeSel.render(themeRenderCtx(), mgr)
	assert.Contains(t, result, "themes (1/4)")
}

func TestThemeSelectOverlay_RenderCursorHighlight(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	result := mgr.themeSel.render(themeRenderCtx(), mgr)
	assert.Contains(t, result, "> ")
}

func TestThemeSelectOverlay_RenderFilterPlaceholder(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	result := mgr.themeSel.render(themeRenderCtx(), mgr)
	assert.Contains(t, result, "type to filter...")
}

func TestThemeSelectOverlay_RenderFilterInput(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "drac"
	mgr.themeSel.applyFilter()
	result := mgr.themeSel.render(themeRenderCtx(), mgr)
	assert.Contains(t, result, "drac")
	assert.Contains(t, result, "│")
}

func TestThemeSelectOverlay_RenderNoMatches(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "nonexistent"
	mgr.themeSel.applyFilter()
	result := mgr.themeSel.render(themeRenderCtx(), mgr)
	assert.Contains(t, result, "no matches")
}

func TestThemeSelectOverlay_RenderScroll(t *testing.T) {
	items := make([]ThemeItem, 20)
	for i := range items {
		items[i] = ThemeItem{Name: strings.Repeat("t", 3) + string(rune('a'+i)), AccentColor: "#ffffff"}
	}
	mgr := NewManager()
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: items})
	mgr.themeSel.cursor = 15
	mgr.themeSel.offset = 10

	ctx := RenderCtx{Width: 80, Height: 16, Resolver: style.PlainResolver()}
	result := mgr.themeSel.render(ctx, mgr)
	assert.Contains(t, result, "tttk") // offset 10 item
	assert.NotContains(t, result, "ttta")
}

func TestThemeSelectOverlay_FormatEntryGallerySwatch(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030"}
	resolver := style.NewResolver(c)
	ts := &themeSelectOverlay{}
	item := ThemeItem{Name: "dracula", AccentColor: "#bd93f9"}
	result := ts.formatEntry(item, 40, false, resolver)
	assert.Contains(t, result, "■")
	assert.Contains(t, result, "dracula")
}

func TestThemeSelectOverlay_FormatEntryLocalSwatch(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030"}
	resolver := style.NewResolver(c)
	ts := &themeSelectOverlay{}
	item := ThemeItem{Name: "my-theme", Local: true}
	result := ts.formatEntry(item, 40, false, resolver)
	assert.Contains(t, result, "◇")
	assert.Contains(t, result, "my-theme")
}

func TestThemeSelectOverlay_FormatEntrySelected(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030"}
	resolver := style.NewResolver(c)
	ts := &themeSelectOverlay{}
	item := ThemeItem{Name: "nord", AccentColor: "#88c0d0"}
	result := ts.formatEntry(item, 40, true, resolver)
	assert.Contains(t, result, "> ")
	assert.Contains(t, result, "nord")
}

func TestThemeSelectOverlay_FormatEntryTruncation(t *testing.T) {
	resolver := style.PlainResolver()
	ts := &themeSelectOverlay{}
	item := ThemeItem{Name: strings.Repeat("x", 100), AccentColor: "#ffffff"}
	result := ts.formatEntry(item, 20, false, resolver)
	assert.Contains(t, result, "…")
}

func TestThemeSelectOverlay_HandleKey_NavigateDown(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	assert.Equal(t, 0, mgr.themeSel.cursor)

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, "")
	assert.Equal(t, OutcomeThemePreview, out.Kind)
	require.NotNil(t, out.ThemeChoice)
	assert.Equal(t, "catppuccin-mocha", out.ThemeChoice.Name)
	assert.Equal(t, 1, mgr.themeSel.cursor)
	assert.True(t, mgr.Active())
}

func TestThemeSelectOverlay_HandleKey_NavigateUp(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.cursor = 2
	mgr.themeSel.lastPreviewedName = "dracula"

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, "")
	assert.Equal(t, OutcomeThemePreview, out.Kind)
	require.NotNil(t, out.ThemeChoice)
	assert.Equal(t, "catppuccin-mocha", out.ThemeChoice.Name)
	assert.Equal(t, 1, mgr.themeSel.cursor)
}

func TestThemeSelectOverlay_HandleKey_DownBounds(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.cursor = len(themeItems()) - 1

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, "")
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.Equal(t, len(themeItems())-1, mgr.themeSel.cursor)
}

func TestThemeSelectOverlay_HandleKey_UpBounds(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, "")
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.Equal(t, 0, mgr.themeSel.cursor)
}

func TestThemeSelectOverlay_HandleKey_EnterConfirm(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.cursor = 2

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")
	assert.Equal(t, OutcomeThemeConfirmed, out.Kind)
	require.NotNil(t, out.ThemeChoice)
	assert.Equal(t, "dracula", out.ThemeChoice.Name)
	assert.False(t, mgr.Active())
}

func TestThemeSelectOverlay_HandleKey_EnterEmptyList(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: themeItems()})
	mgr.themeSel.filter = "nonexistent"
	mgr.themeSel.applyFilter()

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")
	assert.Equal(t, OutcomeThemeCanceled, out.Kind)
	assert.False(t, mgr.Active())
}

func TestThemeSelectOverlay_HandleKey_EscCancelNoFilter(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.Equal(t, OutcomeThemeCanceled, out.Kind)
	assert.False(t, mgr.Active())
}

func TestThemeSelectOverlay_HandleKey_EscClearsFilterFirst(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "drac"
	mgr.themeSel.applyFilter()
	mgr.themeSel.lastPreviewedName = ""

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.NotEqual(t, OutcomeThemeCanceled, out.Kind, "first esc should clear filter, not cancel")
	assert.True(t, mgr.Active(), "overlay should stay open after clearing filter")
	assert.Empty(t, mgr.themeSel.filter)
	assert.Len(t, mgr.themeSel.entries, 4, "entries should be unfiltered")
}

func TestThemeSelectOverlay_HandleKey_EscTwoPress(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "x"
	mgr.themeSel.applyFilter()

	// first esc clears filter
	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.True(t, mgr.Active())
	assert.Empty(t, mgr.themeSel.filter)

	// second esc cancels
	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.Equal(t, OutcomeThemeCanceled, out.Kind)
	assert.False(t, mgr.Active())
}

func TestThemeSelectOverlay_HandleKey_FilterInput(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, "")
	assert.True(t, mgr.Active())
	assert.Equal(t, "d", mgr.themeSel.filter)
	assert.Len(t, mgr.themeSel.entries, 2) // dracula + revdiff (contains 'd')
	assert.Equal(t, OutcomeThemePreview, out.Kind)
}

func TestThemeSelectOverlay_HandleKey_Backspace(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "dra"
	mgr.themeSel.applyFilter()
	mgr.themeSel.lastPreviewedName = ""

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace}, "")
	assert.Equal(t, "dr", mgr.themeSel.filter)
	assert.Equal(t, OutcomeThemePreview, out.Kind)
}

func TestThemeSelectOverlay_HandleKey_BackspaceEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace}, "")
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.Empty(t, mgr.themeSel.filter)
}

func TestThemeSelectOverlay_HandleKey_ActionThemeSelectCancels(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}}, keymap.ActionThemeSelect)
	assert.Equal(t, OutcomeThemeCanceled, out.Kind)
	assert.False(t, mgr.Active())
}

func TestThemeSelectOverlay_HandleMouse_WheelMovesCursor(t *testing.T) {
	t.Run("wheel down previews next theme", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr) // ensure height is set for maxVisible
		require.Equal(t, 0, mgr.themeSel.cursor)

		out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.Equal(t, OutcomeThemePreview, out.Kind)
		require.NotNil(t, out.ThemeChoice)
		assert.Equal(t, "catppuccin-mocha", out.ThemeChoice.Name)
		assert.Equal(t, 1, mgr.themeSel.cursor)
		assert.True(t, mgr.Active())
	})

	t.Run("wheel up previews previous theme", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		mgr.themeSel.cursor = 2
		mgr.themeSel.lastPreviewedName = "dracula"

		out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
		assert.Equal(t, OutcomeThemePreview, out.Kind)
		require.NotNil(t, out.ThemeChoice)
		assert.Equal(t, "catppuccin-mocha", out.ThemeChoice.Name)
		assert.Equal(t, 1, mgr.themeSel.cursor)
	})

	t.Run("wheel at last entry is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		mgr.themeSel.cursor = len(themeItems()) - 1

		out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.Equal(t, OutcomeNone, out.Kind)
		assert.Equal(t, len(themeItems())-1, mgr.themeSel.cursor)
	})

	t.Run("shift+wheel uses half-page step", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		start := mgr.themeSel.cursor
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Shift: true})
		step := max(mgr.themeSel.maxVisible()/2, 1)
		want := min(start+step, len(mgr.themeSel.entries)-1)
		assert.Equal(t, want, mgr.themeSel.cursor)
	})

	t.Run("non-press wheel ignored", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})
		assert.Equal(t, 0, mgr.themeSel.cursor)
	})

	t.Run("non-wheel button ignored", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		assert.Equal(t, 0, mgr.themeSel.cursor)
	})
}

func TestThemeSelectOverlay_HandleLeftClick(t *testing.T) {
	t.Run("click on first entry confirms it", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		// entries start at localY=4 (border + padding + filter + blank)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 4})
		assert.Equal(t, OutcomeThemeConfirmed, out.Kind)
		require.NotNil(t, out.ThemeChoice)
		assert.Equal(t, "revdiff", out.ThemeChoice.Name)
		assert.Equal(t, 0, mgr.themeSel.cursor)
	})

	t.Run("click on third entry confirms it", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 6})
		assert.Equal(t, OutcomeThemeConfirmed, out.Kind)
		assert.Equal(t, "dracula", out.ThemeChoice.Name)
		assert.Equal(t, 2, mgr.themeSel.cursor)
	})

	t.Run("click on filter row is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 2})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click on blank separator row is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 3})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click on top border is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 0})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click past visible entries is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		farRow := 4 + len(themeItems()) + 5
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: farRow})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click respects scroll offset", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		mgr.themeSel.offset = 1

		// click on first visible row (localY=4) selects entry at offset
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 4})
		assert.Equal(t, OutcomeThemeConfirmed, out.Kind)
		assert.Equal(t, "catppuccin-mocha", out.ThemeChoice.Name)
	})

	t.Run("non-press action is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: 5, Y: 4})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click on left border column is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 0, Y: 4})
		assert.Equal(t, OutcomeNone, out.Kind, "x=0 is the left border")
	})

	t.Run("click on left padding column is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 1, Y: 4})
		assert.Equal(t, OutcomeNone, out.Kind, "x=1 is the left padding")
	})

	t.Run("click on right padding column is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		w := mgr.themeSel.popupWidth
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: w - 2, Y: 4})
		assert.Equal(t, OutcomeNone, out.Kind, "x=popupWidth-2 is the right padding")
	})

	t.Run("click on right border column is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		w := mgr.themeSel.popupWidth
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: w - 1, Y: 4})
		assert.Equal(t, OutcomeNone, out.Kind, "x=popupWidth-1 is the right border")
	})

	t.Run("click at first content column (x=2) confirms", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 2, Y: 4})
		assert.Equal(t, OutcomeThemeConfirmed, out.Kind, "x=2 is the first content column")
	})

	t.Run("click updates lastPreviewedName to confirmed theme", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 6})
		require.Equal(t, OutcomeThemeConfirmed, out.Kind)
		assert.Equal(t, "dracula", mgr.themeSel.lastPreviewedName, "so a subsequent arrow-key back to the same entry does not emit redundant preview")
	})

	t.Run("click with empty entries (filter rejects all) is no-op", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenThemeSelect(themeSpec())
		_ = mgr.themeSel.render(themeRenderCtx(), mgr)
		mgr.themeSel.filter = "no-match-xyz"
		mgr.themeSel.applyFilter()
		require.Empty(t, mgr.themeSel.entries)

		out := mgr.themeSel.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 4})
		assert.Equal(t, OutcomeNone, out.Kind, "no entries to confirm, guard must protect against index panic")
	})
}

func TestThemeSelectOverlay_HandleMouse_ClickOutsideSwallowed(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	ctx := themeRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	_ = mgr.Compose(base, ctx)

	require.NotZero(t, mgr.bounds.w)

	out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 0, Y: 0})
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.True(t, mgr.Active(), "click outside popup must not close or confirm")
}

func TestThemeSelectOverlay_HandleMouse_ClickInsideConfirms(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	ctx := themeRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	_ = mgr.Compose(base, ctx)

	// click on first entry row: screen Y = bounds.y + 4
	out := mgr.HandleMouse(tea.MouseMsg{
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
		X:      mgr.bounds.x + 5,
		Y:      mgr.bounds.y + 4,
	})
	assert.Equal(t, OutcomeThemeConfirmed, out.Kind)
	require.NotNil(t, out.ThemeChoice)
	assert.False(t, mgr.Active(), "OutcomeThemeConfirmed auto-closes the overlay")
}

func TestThemeSelectOverlay_HandleKey_PreviewDedup(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	// first move down — preview emitted
	out1 := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, "")
	assert.Equal(t, OutcomeThemePreview, out1.Kind)

	// move back up to same item — preview emitted (different name)
	out2 := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, "")
	assert.Equal(t, OutcomeThemePreview, out2.Kind)

	// move down again to same name as out1 — should be deduped
	_ = mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, "")
	// cursor is on "catppuccin-mocha" again, lastPreviewedName is "catppuccin-mocha"
	// stay on same position — another down would go to next
	assert.Equal(t, "catppuccin-mocha", mgr.themeSel.lastPreviewedName)
}

func TestThemeSelectOverlay_HandleKey_PreviewDedupSameName(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.cursor = 1
	mgr.themeSel.lastPreviewedName = "catppuccin-mocha"

	// try to move down, which triggers preview — but then move back
	// the point: if lastPreviewedName already matches, OutcomeNone
	out := mgr.themeSel.handleKey(tea.KeyMsg{Type: tea.KeyDown}, "")
	assert.Equal(t, OutcomeThemePreview, out.Kind) // dracula != catppuccin-mocha

	mgr.themeSel.cursor = 1
	mgr.themeSel.lastPreviewedName = "catppuccin-mocha"
	// simulate: cursor doesn't move because already at position, but we call previewOutcome
	outDedup := mgr.themeSel.previewOutcome()
	assert.Equal(t, OutcomeNone, outDedup.Kind, "same name should be deduped")
}

func TestThemeSelectOverlay_HandleKey_OtherKeysConsumed(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	keys := []struct {
		msg    tea.KeyMsg
		action keymap.Action
	}{
		{tea.KeyMsg{Type: tea.KeyTab}, keymap.ActionTogglePane},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit},
	}

	for _, k := range keys {
		out := mgr.HandleKey(k.msg, k.action)
		// q goes to filter, tab is consumed
		assert.NotEqual(t, OutcomeThemeCanceled, out.Kind, "key %v should not cancel", k.msg)
		assert.NotEqual(t, OutcomeThemeConfirmed, out.Kind, "key %v should not confirm", k.msg)
		assert.True(t, mgr.Active(), "key %v should not close overlay", k.msg)
	}
}

func TestThemeSelectOverlay_ComposeOnBase(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	ctx := themeRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	result := mgr.Compose(base, ctx)

	assert.Contains(t, result, "themes (4)")
	assert.Contains(t, result, "revdiff")

	lines := strings.Split(result, "\n")
	assert.Len(t, lines, ctx.Height, "composited result should preserve base line count")
}

func TestThemeSelectOverlay_OpenCursorOnActiveName(t *testing.T) {
	spec := ThemeSelectSpec{Items: themeItems(), ActiveName: "dracula"}
	mgr := NewManager()
	mgr.OpenThemeSelect(spec)
	assert.Equal(t, 2, mgr.themeSel.cursor, "cursor should be on dracula (index 2)")
}

func TestThemeSelectOverlay_OpenCursorOnActiveNameNotFound(t *testing.T) {
	spec := ThemeSelectSpec{Items: themeItems(), ActiveName: "nonexistent"}
	mgr := NewManager()
	mgr.OpenThemeSelect(spec)
	assert.Equal(t, 0, mgr.themeSel.cursor, "cursor should default to 0 when active name not found")
}

func TestThemeSelectOverlay_OpenResetsState(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.cursor = 3
	mgr.themeSel.offset = 2
	mgr.themeSel.filter = "abc"
	mgr.themeSel.lastPreviewedName = "dracula"

	mgr.OpenThemeSelect(themeSpec())
	assert.Equal(t, 0, mgr.themeSel.cursor, "cursor should reset on reopen")
	assert.Equal(t, 0, mgr.themeSel.offset, "offset should reset on reopen")
	assert.Empty(t, mgr.themeSel.filter, "filter should reset on reopen")
	assert.Empty(t, mgr.themeSel.lastPreviewedName, "lastPreviewedName should reset on reopen")
}

func TestThemeSelectOverlay_ApplyFilter(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	mgr.themeSel.filter = "cat"
	mgr.themeSel.applyFilter()
	require.Len(t, mgr.themeSel.entries, 1)
	assert.Equal(t, "catppuccin-mocha", mgr.themeSel.entries[0].Name)
	assert.Equal(t, 0, mgr.themeSel.cursor)
}

func TestThemeSelectOverlay_ApplyFilterCaseInsensitive(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())

	mgr.themeSel.filter = "DRAC"
	mgr.themeSel.applyFilter()
	require.Len(t, mgr.themeSel.entries, 1)
	assert.Equal(t, "dracula", mgr.themeSel.entries[0].Name)
}

func TestThemeSelectOverlay_ApplyFilterEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.OpenThemeSelect(themeSpec())
	mgr.themeSel.filter = "x"
	mgr.themeSel.applyFilter()

	mgr.themeSel.filter = ""
	mgr.themeSel.applyFilter()
	assert.Len(t, mgr.themeSel.entries, 4, "empty filter should show all items")
}

func TestThemeSelectOverlay_ScrollDown(t *testing.T) {
	items := make([]ThemeItem, 20)
	for i := range items {
		items[i] = ThemeItem{Name: string(rune('a' + i)), AccentColor: "#ffffff"}
	}
	mgr := NewManager()
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: items})
	mgr.themeSel.height = 14 // maxVisible = max(min(20, 14-10), 1) = 4

	for range 4 {
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, "")
	}
	assert.Equal(t, 4, mgr.themeSel.cursor)
	assert.Equal(t, 1, mgr.themeSel.offset, "offset should scroll down")
}

func TestThemeSelectOverlay_ScrollUp(t *testing.T) {
	items := make([]ThemeItem, 20)
	for i := range items {
		items[i] = ThemeItem{Name: string(rune('a' + i)), AccentColor: "#ffffff"}
	}
	mgr := NewManager()
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: items})
	mgr.themeSel.cursor = 5
	mgr.themeSel.offset = 5
	mgr.themeSel.height = 14

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyUp}, "")
	assert.Equal(t, 4, mgr.themeSel.cursor)
	assert.Equal(t, 4, mgr.themeSel.offset, "offset should scroll up to follow cursor")
}

func TestThemeSelectOverlay_RenderClampsOffsetAfterResize(t *testing.T) {
	items := make([]ThemeItem, 20)
	for i := range items {
		items[i] = ThemeItem{Name: string(rune('a' + i)), AccentColor: "#ffffff"}
	}
	mgr := NewManager()

	// simulate stale state: cursor far down, offset 0, height from a tall terminal
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: items})
	mgr.themeSel.cursor = 15
	mgr.themeSel.offset = 0
	mgr.themeSel.height = 50 // stale height from previous tall render

	// render with a short terminal — clamp in render must adjust offset
	shortCtx := RenderCtx{Width: 80, Height: 16, Resolver: style.PlainResolver()}
	result := mgr.themeSel.render(shortCtx, mgr)
	maxVis := mgr.themeSel.maxVisible()
	assert.GreaterOrEqual(t, mgr.themeSel.cursor, mgr.themeSel.offset, "cursor must be >= offset")
	assert.Less(t, mgr.themeSel.cursor, mgr.themeSel.offset+maxVis, "cursor must be < offset+maxVisible")
	assert.Contains(t, result, string(rune('a'+15)), "active theme must be visible")
}

func TestThemeSelectOverlay_RenderClampsStaleOffsetOnFirstRender(t *testing.T) {
	items := make([]ThemeItem, 20)
	for i := range items {
		items[i] = ThemeItem{Name: string(rune('a' + i)), AccentColor: "#ffffff"}
	}
	mgr := NewManager()

	// open with zero height (fresh manager) — active theme at index 15 sets offset too high
	spec := ThemeSelectSpec{Items: items, ActiveName: string(rune('a' + 15))}
	mgr.OpenThemeSelect(spec)
	assert.Equal(t, 15, mgr.themeSel.cursor, "cursor should be on active theme")
	assert.Positive(t, mgr.themeSel.offset, "offset should be non-zero from stale maxVisible")

	// first render with a tall terminal where all 20 items fit — offset must be reduced
	tallCtx := RenderCtx{Width: 80, Height: 40, Resolver: style.PlainResolver()}
	result := mgr.themeSel.render(tallCtx, mgr)
	assert.Equal(t, 0, mgr.themeSel.offset, "offset should be 0 when all items fit")
	assert.Contains(t, result, string(rune('a')), "first item must be visible")
	assert.Contains(t, result, string(rune('a'+15)), "active item must be visible")
}

func TestSwatchText_WithFg(t *testing.T) {
	result := swatchText("\033[38;2;255;0;0m", "■", "")
	assert.Contains(t, result, "\033[38;2;255;0;0m")
	assert.Contains(t, result, "■")
	assert.Contains(t, result, "\033[39m")
}

func TestSwatchText_EmptyFg(t *testing.T) {
	result := swatchText("", "■", "")
	assert.Equal(t, "■", result)
}

func TestSwatchText_CustomReset(t *testing.T) {
	result := swatchText("\033[38;2;255;0;0m", "■", "\033[38;2;0;255;0m")
	assert.Contains(t, result, "\033[38;2;0;255;0m")
	assert.NotContains(t, result, "\033[39m")
}
