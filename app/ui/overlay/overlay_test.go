package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	assert.False(t, mgr.Active(), "new manager should have no active overlay")
	assert.Equal(t, KindNone, mgr.Kind())
}

func TestManager_HandleMouse_NoActiveOverlay(t *testing.T) {
	mgr := NewManager()
	out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.Equal(t, Outcome{}, out, "no active overlay returns zero Outcome")
}

func TestManager_Close_ClearsBounds(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(HelpSpec{})

	ctx := RenderCtx{Width: 120, Height: 40, Resolver: style.PlainResolver()}
	base := strings.Repeat(strings.Repeat(" ", ctx.Width)+"\n", ctx.Height)
	_ = mgr.Compose(base, ctx)
	require.NotZero(t, mgr.bounds.w, "Compose should have populated bounds")

	mgr.Close()
	assert.Equal(t, popupBounds{}, mgr.bounds, "Close must reset bounds to zero value")
}

func TestManager_OpenClose(t *testing.T) {
	mgr := NewManager()

	t.Run("help", func(t *testing.T) {
		mgr.OpenHelp(HelpSpec{})
		assert.True(t, mgr.Active())
		assert.Equal(t, KindHelp, mgr.Kind())
		mgr.Close()
		assert.False(t, mgr.Active())
		assert.Equal(t, KindNone, mgr.Kind())
	})

	t.Run("annotList", func(t *testing.T) {
		mgr.OpenAnnotList(AnnotListSpec{Items: []AnnotationItem{
			{AnnotationTarget: AnnotationTarget{File: "a.go", Line: 1}},
		}})
		assert.True(t, mgr.Active())
		assert.Equal(t, KindAnnotList, mgr.Kind())
		mgr.Close()
		assert.False(t, mgr.Active())
	})

	t.Run("themeSelect", func(t *testing.T) {
		mgr.OpenThemeSelect(ThemeSelectSpec{Items: []ThemeItem{{Name: "dark"}}})
		assert.True(t, mgr.Active())
		assert.Equal(t, KindThemeSelect, mgr.Kind())
		mgr.Close()
		assert.False(t, mgr.Active())
	})

	t.Run("commitInfo", func(t *testing.T) {
		mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
		assert.True(t, mgr.Active())
		assert.Equal(t, KindCommitInfo, mgr.Kind())
		mgr.Close()
		assert.False(t, mgr.Active())
	})
}

func TestManager_OpenClosesExisting(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(HelpSpec{})
	assert.Equal(t, KindHelp, mgr.Kind())
	mgr.OpenAnnotList(AnnotListSpec{})
	assert.Equal(t, KindAnnotList, mgr.Kind(), "opening annot list should close help first")
	assert.NotEqual(t, KindHelp, mgr.Kind(), "help should be deactivated")
}

func TestManager_HandleKeyNoOverlay(t *testing.T) {
	mgr := NewManager()
	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit)
	assert.Equal(t, OutcomeNone, out.Kind, "no overlay active should return OutcomeNone")
}

func TestManager_ComposeNoOverlay(t *testing.T) {
	mgr := NewManager()
	base := "line1\nline2\nline3"
	result := mgr.Compose(base, RenderCtx{Width: 20, Height: 3})
	assert.Equal(t, base, result, "compose with no overlay should return base unchanged")
}

func TestOverlayCenter_EmptyBg(t *testing.T) {
	mgr := NewManager()
	result := mgr.overlayCenter("", "fg", 10)
	assert.Contains(t, result, "fg", "fg composites onto single-line empty bg")
}

func TestOverlayCenter_EmptyFg(t *testing.T) {
	mgr := NewManager()
	bg := "aaaa\nbbbb\ncccc"
	result := mgr.overlayCenter(bg, "", 4)
	assert.Equal(t, bg, result, "empty fg returns bg unchanged")
}

func TestOverlayCenter_FgSmallerThanBg(t *testing.T) {
	mgr := NewManager()
	bg := strings.Repeat(".", 10) + "\n" +
		strings.Repeat(".", 10) + "\n" +
		strings.Repeat(".", 10) + "\n" +
		strings.Repeat(".", 10) + "\n" +
		strings.Repeat(".", 10)
	fg := "XX\nYY"

	result := mgr.overlayCenter(bg, fg, 10)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 5, "should preserve bg line count")

	assert.Equal(t, "..........", lines[0], "first bg line unchanged")
	assert.Contains(t, lines[1], "XX", "fg line 1 centered in bg")
	assert.Contains(t, lines[2], "YY", "fg line 2 centered in bg")
	assert.Equal(t, "..........", lines[3], "fourth bg line unchanged")
}

func TestOverlayCenter_FgWiderThanBg(t *testing.T) {
	mgr := NewManager()
	bg := "ab\ncd"
	fg := "XXXX"

	result := mgr.overlayCenter(bg, fg, 2)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "XXXX", "fg wider than bg should still composite")
}

func TestOverlayCenter_ANSIContent(t *testing.T) {
	mgr := NewManager()
	bg := "\033[31mred text\033[0m  \n" +
		"\033[32mgreen txt\033[0m \n" +
		"\033[34mblue text\033[0m "
	fg := "\033[33myellow\033[0m"

	result := mgr.overlayCenter(bg, fg, 11)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], "yellow", "ANSI fg composited onto ANSI bg")
}

func TestInjectBorderTitle_Basic(t *testing.T) {
	mgr := NewManager()
	border := lipgloss.NormalBorder()
	topLine := border.TopLeft + strings.Repeat(border.Top, 18) + border.TopRight
	box := topLine + "\n│ content          │\n" + border.BottomLeft + strings.Repeat(border.Bottom, 18) + border.BottomRight

	result := mgr.injectBorderTitle(box, " Title ", 20, "", "")
	lines := strings.Split(result, "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	assert.Contains(t, lines[0], "Title", "title should be injected into top border")
}

func TestInjectBorderTitle_EmptyTitle(t *testing.T) {
	mgr := NewManager()
	border := lipgloss.NormalBorder()
	topLine := border.TopLeft + strings.Repeat(border.Top, 18) + border.TopRight
	box := topLine + "\n│ content          │\n" + border.BottomLeft + strings.Repeat(border.Bottom, 18) + border.BottomRight

	result := mgr.injectBorderTitle(box, "", 20, "", "")
	lines := strings.Split(result, "\n")
	assert.Contains(t, lines[0], border.TopLeft, "empty title still produces valid border")
}

func TestInjectBorderTitle_TitleTooWide(t *testing.T) {
	mgr := NewManager()
	border := lipgloss.NormalBorder()
	topLine := border.TopLeft + strings.Repeat(border.Top, 4) + border.TopRight
	box := topLine + "\n│ ok │"

	result := mgr.injectBorderTitle(box, " very long title text ", 6, "", "")
	assert.Equal(t, box, result, "too-wide title should leave box unchanged")
}

func TestInjectBorderTitle_WithANSIColors(t *testing.T) {
	mgr := NewManager()
	border := lipgloss.NormalBorder()
	topLine := border.TopLeft + strings.Repeat(border.Top, 28) + border.TopRight
	box := topLine + "\n│ content                      │"

	accentFg := "\033[38;2;100;200;255m"
	paneBg := "\033[48;2;30;30;50m"

	result := mgr.injectBorderTitle(box, " Test ", 30, accentFg, paneBg)
	lines := strings.Split(result, "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	assert.Contains(t, lines[0], "Test", "title present")
	assert.Contains(t, lines[0], paneBg, "bg escape present")
	assert.Contains(t, lines[0], accentFg, "fg escape present")
	assert.Contains(t, lines[0], "\033[39m", "fg reset present")
	assert.Contains(t, lines[0], "\033[49m", "bg reset present")
}

func TestInjectBorderTitle_EmptyBgFallback(t *testing.T) {
	mgr := NewManager()
	border := lipgloss.NormalBorder()
	topLine := border.TopLeft + strings.Repeat(border.Top, 18) + border.TopRight
	box := topLine + "\n│ content          │"

	result := mgr.injectBorderTitle(box, " Title ", 20, "", "")
	lines := strings.Split(result, "\n")
	assert.NotContains(t, lines[0], "\033[49m", "no bg reset when no bg color")
	assert.NotContains(t, lines[0], "\033[48", "no bg escape when no bg color")
}

func TestInjectBorderTitle_EmptyBox(t *testing.T) {
	mgr := NewManager()
	result := mgr.injectBorderTitle("", " Title ", 20, "", "")
	assert.Empty(t, result, "empty box returns empty")
}

func TestThemeSelectOverlay_OpenPositionsCursor(t *testing.T) {
	mgr := NewManager()
	items := []ThemeItem{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: items, ActiveName: "b"})
	assert.Equal(t, 1, mgr.themeSel.cursor, "cursor should be on 'b' (index 1)")
}

func TestThemeSelectOverlay_OpenNoMatch(t *testing.T) {
	mgr := NewManager()
	items := []ThemeItem{{Name: "a"}, {Name: "b"}}
	mgr.OpenThemeSelect(ThemeSelectSpec{Items: items, ActiveName: "missing"})
	assert.Equal(t, 0, mgr.themeSel.cursor, "cursor defaults to 0 when active name not found")
}
