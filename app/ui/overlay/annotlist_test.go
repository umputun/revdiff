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

func annotListSpec(items ...AnnotationItem) AnnotListSpec {
	return AnnotListSpec{Items: items}
}

func annotItem(file string, line int, changeType, comment string) AnnotationItem {
	return AnnotationItem{
		AnnotationTarget: AnnotationTarget{File: file, ChangeType: changeType, Line: line},
		Comment:          comment,
	}
}

func annotRenderCtx() RenderCtx {
	return RenderCtx{Width: 80, Height: 30, Resolver: style.PlainResolver()}
}

func TestAnnotListOverlay_RenderEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec())
	result := mgr.annotLst.render(annotRenderCtx(), mgr)
	assert.Contains(t, result, "no annotations")
	assert.Contains(t, result, "annotations (0)")
}

func TestAnnotListOverlay_RenderWithItems(t *testing.T) {
	items := []AnnotationItem{
		annotItem("handler.go", 43, "+", "use errors.Is()"),
		annotItem("handler.go", 87, "+", "add context"),
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	result := mgr.annotLst.render(annotRenderCtx(), mgr)
	assert.Contains(t, result, "annotations (2)")
	assert.Contains(t, result, "handler.go:43")
	assert.Contains(t, result, "handler.go:87")
}

func TestAnnotListOverlay_RenderCursorHighlight(t *testing.T) {
	items := []AnnotationItem{
		annotItem("a.go", 1, "+", "first"),
		annotItem("a.go", 2, "-", "second"),
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	result := mgr.annotLst.render(annotRenderCtx(), mgr)
	assert.Contains(t, result, "> ")
}

func TestAnnotListOverlay_RenderFileLevelAnnotation(t *testing.T) {
	items := []AnnotationItem{annotItem("main.go", 0, "", "review this file")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	result := mgr.annotLst.render(annotRenderCtx(), mgr)
	assert.Contains(t, result, "main.go (file-level)")
}

func TestAnnotListOverlay_RenderTruncation(t *testing.T) {
	longComment := strings.Repeat("x", 200)
	items := []AnnotationItem{annotItem("a.go", 5, "+", longComment)}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	ctx := RenderCtx{Width: 60, Height: 30, Resolver: style.PlainResolver()}
	result := mgr.annotLst.render(ctx, mgr)
	assert.Contains(t, result, "...")
	assert.NotContains(t, result, longComment)
}

func TestAnnotListOverlay_RenderPopupWidthScaling(t *testing.T) {
	items := []AnnotationItem{annotItem("a.go", 1, "+", "note")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))

	// rendered width = popupWidth + 2 (1-col border on each side; padding lives inside lipgloss Width)
	const borderCols = 2
	tests := []struct {
		name      string
		ctxW      int
		wantPopup int
		comment   string
	}{
		{"narrow terminal floors at min cap", 25, annotPopupMinWidth, "min cap protects readability"},
		{"medium terminal scales by ctx width", 60, 50, "ctx.Width-margin below the upper cap"},
		{"upper cap kicks in just past max", 150, annotPopupMaxWidth, "popup capped to avoid dominating wide screens"},
		{"wide terminal stays at max cap", 220, annotPopupMaxWidth, "extra columns past max cap ignored"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := RenderCtx{Width: tt.ctxW, Height: 30, Resolver: style.PlainResolver()}
			result := mgr.annotLst.render(ctx, mgr)
			var maxLine int
			for ln := range strings.SplitSeq(result, "\n") {
				if w := lipgloss.Width(ln); w > maxLine {
					maxLine = w
				}
			}
			assert.Equal(t, tt.wantPopup+borderCols, maxLine, tt.comment)
		})
	}
}

func TestAnnotListOverlay_RenderWidePopupFitsMoreCommentText(t *testing.T) {
	// the user-visible payoff of the wider cap: long comments must fit more chars
	// before truncation on a wide terminal than on a narrow one. locks the
	// behavior the cap bump was meant to deliver, not just the popup geometry.
	longComment := strings.Repeat("ABCDEFGHIJ ", 20) // 220 chars
	items := []AnnotationItem{annotItem("a.go", 1, "+", longComment)}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))

	render := func(ctxW int) string {
		ctx := RenderCtx{Width: ctxW, Height: 30, Resolver: style.PlainResolver()}
		return mgr.annotLst.render(ctx, mgr)
	}
	visibleAlpha := func(s string) int {
		var n int
		for _, r := range s {
			if r >= 'A' && r <= 'J' {
				n++
			}
		}
		return n
	}

	narrow := visibleAlpha(render(80)) // popup capped to 70
	wide := visibleAlpha(render(200))  // popup uses up to annotPopupMaxWidth
	assert.Greater(t, wide, narrow, "wide terminal must show more comment chars before truncation")
}

func TestAnnotListOverlay_RenderScrollOffset(t *testing.T) {
	items := make([]AnnotationItem, 20)
	for i := range items {
		items[i] = annotItem("a.go", i+1, "+", "note")
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	mgr.annotLst.cursor = 15
	mgr.annotLst.offset = 10

	ctx := RenderCtx{Width: 80, Height: 12, Resolver: style.PlainResolver()}
	result := mgr.annotLst.render(ctx, mgr)
	assert.Contains(t, result, "a.go:11")
	assert.NotContains(t, result, "a.go:1 ")
}

func TestAnnotListOverlay_FormatItemAddType(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", AddFg: "#87d787", RemoveFg: "#ff8787"}
	resolver := style.NewResolver(c)
	a := &annotListOverlay{}
	item := annotItem("handler.go", 43, "+", "fix this")
	result := a.formatItem(item, 60, false, resolver)
	assert.Contains(t, result, "handler.go:43 (+)")
	assert.Contains(t, result, "fix this")
}

func TestAnnotListOverlay_FormatItemRemoveType(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", AddFg: "#87d787", RemoveFg: "#ff8787"}
	resolver := style.NewResolver(c)
	a := &annotListOverlay{}
	item := annotItem("store.go", 18, "-", "keep this")
	result := a.formatItem(item, 60, false, resolver)
	assert.Contains(t, result, "store.go:18 (-)")
}

func TestAnnotListOverlay_FormatItemContextType(t *testing.T) {
	resolver := style.PlainResolver()
	a := &annotListOverlay{}
	item := annotItem("file.go", 5, " ", "context note")
	result := a.formatItem(item, 60, false, resolver)
	assert.Contains(t, result, "file.go:5 ( )")
}

func TestAnnotListOverlay_FormatItemSelected(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", AddFg: "#87d787", RemoveFg: "#ff8787"}
	resolver := style.NewResolver(c)
	a := &annotListOverlay{}
	item := annotItem("a.go", 1, "+", "note")
	result := a.formatItem(item, 60, true, resolver)
	assert.Contains(t, result, "> ")
}

func TestAnnotListOverlay_FormatItemMultiLineComment(t *testing.T) {
	resolver := style.PlainResolver()
	a := &annotListOverlay{}
	item := annotItem("a.go", 5, "+", "first line\nsecond line")
	result := a.formatItem(item, 60, false, resolver)
	// no literal newlines in the rendered row
	assert.NotContains(t, result, "\n", "multi-line comment must render as a single row")
	// both logical lines are present, joined by the newline glyph
	assert.Contains(t, result, "first line")
	assert.Contains(t, result, "second line")
	assert.Contains(t, result, "⏎", "newline glyph should separate flattened lines")
}

func TestAnnotListOverlay_FormatItemMultiLineSelected(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", AddFg: "#87d787", RemoveFg: "#ff8787"}
	resolver := style.NewResolver(c)
	a := &annotListOverlay{}
	item := annotItem("a.go", 5, "+", "first line\nsecond line")
	result := a.formatItem(item, 60, true, resolver)
	assert.NotContains(t, result, "\n", "selected multi-line comment must render as a single row")
	assert.Contains(t, result, "> ")
}

func TestAnnotListOverlay_FormatItemMultiLineTruncation(t *testing.T) {
	resolver := style.PlainResolver()
	a := &annotListOverlay{}
	// long multi-line comment must still truncate
	body := strings.Repeat("very long first line ", 10) + "\n" + strings.Repeat("very long second line ", 10)
	item := annotItem("a.go", 5, "+", body)
	result := a.formatItem(item, 60, false, resolver)
	assert.NotContains(t, result, "\n")
	assert.Contains(t, result, "...", "long multi-line comment should still be truncated")
}

func TestAnnotListOverlay_FormatItemFileLevel(t *testing.T) {
	resolver := style.PlainResolver()
	a := &annotListOverlay{}
	item := annotItem("path/to/file.go", 0, "", "review")
	result := a.formatItem(item, 60, false, resolver)
	assert.Contains(t, result, "file.go (file-level)")
}

func TestAnnotListOverlay_HandleKey_ToggleClose(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(annotItem("a.go", 1, "+", "note")))
	require.True(t, mgr.Active())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}}, keymap.ActionAnnotList)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestAnnotListOverlay_HandleKey_EscClose(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(annotItem("a.go", 1, "+", "note")))

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestAnnotListOverlay_HandleKey_EscHardcoded(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(annotItem("a.go", 1, "+", "note")))

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active(), "esc should close even without ActionDismiss")
}

func TestAnnotListOverlay_HandleKey_EnterSelection(t *testing.T) {
	items := []AnnotationItem{
		annotItem("a.go", 10, "+", "first"),
		annotItem("b.go", 20, "-", "second"),
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	mgr.annotLst.cursor = 1

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")
	assert.Equal(t, OutcomeAnnotationChosen, out.Kind)
	require.NotNil(t, out.AnnotationTarget)
	assert.Equal(t, "b.go", out.AnnotationTarget.File)
	assert.Equal(t, 20, out.AnnotationTarget.Line)
	assert.Equal(t, "-", out.AnnotationTarget.ChangeType)
	assert.False(t, mgr.Active())
}

func TestAnnotListOverlay_HandleKey_EnterEmptyList(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestAnnotListOverlay_HandleKey_NavigateDown(t *testing.T) {
	items := []AnnotationItem{
		annotItem("a.go", 1, "+", "first"),
		annotItem("a.go", 2, "+", "second"),
		annotItem("a.go", 3, "+", "third"),
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	assert.Equal(t, 0, mgr.annotLst.cursor)

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.Equal(t, 1, mgr.annotLst.cursor)
	assert.True(t, mgr.Active())
}

func TestAnnotListOverlay_HandleKey_NavigateUp(t *testing.T) {
	items := []AnnotationItem{
		annotItem("a.go", 1, "+", "first"),
		annotItem("a.go", 2, "+", "second"),
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	mgr.annotLst.cursor = 1

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.Equal(t, 0, mgr.annotLst.cursor)
}

func TestAnnotListOverlay_HandleKey_DownBounds(t *testing.T) {
	items := []AnnotationItem{annotItem("a.go", 1, "+", "only")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	assert.Equal(t, 0, mgr.annotLst.cursor)

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, 0, mgr.annotLst.cursor, "cursor should not go past last item")
}

func TestAnnotListOverlay_HandleKey_UpBounds(t *testing.T) {
	items := []AnnotationItem{annotItem("a.go", 1, "+", "only")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	assert.Equal(t, 0, mgr.annotLst.cursor)

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 0, mgr.annotLst.cursor, "cursor should not go above first item")
}

func TestAnnotListOverlay_HandleKey_ScrollDown(t *testing.T) {
	items := make([]AnnotationItem, 10)
	for i := range items {
		items[i] = annotItem("a.go", i+1, "+", "note")
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	mgr.annotLst.height = 10 // maxVisible = max(min(10, 10-6), 1) = 4

	// move cursor to end of visible area
	for range 4 {
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	}
	assert.Equal(t, 4, mgr.annotLst.cursor)
	assert.Equal(t, 1, mgr.annotLst.offset, "offset should scroll down")
}

func TestAnnotListOverlay_HandleKey_ScrollUp(t *testing.T) {
	items := make([]AnnotationItem, 10)
	for i := range items {
		items[i] = annotItem("a.go", i+1, "+", "note")
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	mgr.annotLst.cursor = 3
	mgr.annotLst.offset = 3
	mgr.annotLst.height = 10

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 2, mgr.annotLst.cursor)
	assert.Equal(t, 2, mgr.annotLst.offset, "offset should scroll up to follow cursor")
}

func TestAnnotListOverlay_HandleMouse_WheelMovesCursor(t *testing.T) {
	items := make([]AnnotationItem, 10)
	for i := range items {
		items[i] = annotItem("a.go", i+1, "+", "note")
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	mgr.annotLst.height = 10 // maxVisible ends up = max(min(10, 10-6), 1) = 4

	t.Run("wheel down advances cursor by one", func(t *testing.T) {
		mgr.annotLst.cursor = 0
		mgr.annotLst.offset = 0
		out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.Equal(t, OutcomeNone, out.Kind)
		assert.Equal(t, 1, mgr.annotLst.cursor)
	})

	t.Run("wheel up retreats cursor by one", func(t *testing.T) {
		mgr.annotLst.cursor = 3
		mgr.annotLst.offset = 0
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
		assert.Equal(t, 2, mgr.annotLst.cursor)
	})

	t.Run("wheel clamps at boundaries", func(t *testing.T) {
		mgr.annotLst.cursor = 0
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
		assert.Equal(t, 0, mgr.annotLst.cursor, "cursor already at 0 does not go negative")

		mgr.annotLst.cursor = 9
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.Equal(t, 9, mgr.annotLst.cursor, "cursor at last item does not exceed bounds")
	})

	t.Run("wheel down past visible window scrolls offset", func(t *testing.T) {
		mgr.annotLst.cursor = 0
		mgr.annotLst.offset = 0
		for range 4 {
			mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		}
		assert.Equal(t, 4, mgr.annotLst.cursor)
		assert.Equal(t, 1, mgr.annotLst.offset, "offset follows cursor past visible window")
	})

	t.Run("shift+wheel uses half visible step", func(t *testing.T) {
		mgr.annotLst.cursor = 0
		mgr.annotLst.offset = 0
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Shift: true})
		want := max(mgr.annotLst.maxVisible(mgr.annotLst.height)/2, 1)
		assert.Equal(t, want, mgr.annotLst.cursor)
	})

	t.Run("non-press wheel ignored", func(t *testing.T) {
		mgr.annotLst.cursor = 2
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})
		assert.Equal(t, 2, mgr.annotLst.cursor)
	})

	t.Run("non-wheel button ignored", func(t *testing.T) {
		mgr.annotLst.cursor = 2
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		assert.Equal(t, 2, mgr.annotLst.cursor)
	})
}

func TestAnnotListOverlay_HandleMouse_EmptyList(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec())
	mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.True(t, mgr.Active(), "overlay stays open")
	assert.Equal(t, 0, mgr.annotLst.cursor)
}

func TestAnnotListOverlay_HandleLeftClick(t *testing.T) {
	items := []AnnotationItem{
		annotItem("a.go", 10, "+", "first"),
		annotItem("b.go", 20, "-", "second"),
		annotItem("c.go", 30, "+", "third"),
	}
	const popupWidth = 40
	// prime layout state that render() would normally set
	setup := func() *Manager {
		mgr := NewManager()
		mgr.OpenAnnotList(annotListSpec(items...))
		mgr.annotLst.height = 20 // maxVisible = max(min(3, 20-6), 1) = 3
		mgr.annotLst.popupWidth = popupWidth
		return mgr
	}

	t.Run("click on first item row", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 2})
		assert.Equal(t, OutcomeAnnotationChosen, out.Kind)
		require.NotNil(t, out.AnnotationTarget)
		assert.Equal(t, "a.go", out.AnnotationTarget.File)
		assert.Equal(t, 10, out.AnnotationTarget.Line)
		assert.Equal(t, 0, mgr.annotLst.cursor)
	})

	t.Run("click on third item row", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 4})
		assert.Equal(t, OutcomeAnnotationChosen, out.Kind)
		assert.Equal(t, "c.go", out.AnnotationTarget.File)
		assert.Equal(t, 2, mgr.annotLst.cursor)
	})

	t.Run("click on top border is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 0})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click on top padding is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 1})
		assert.Equal(t, OutcomeNone, out.Kind)
	})

	t.Run("click past visible items is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 5})
		assert.Equal(t, OutcomeNone, out.Kind, "row past the 3 visible items")
	})

	t.Run("click on left border column is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 0, Y: 2})
		assert.Equal(t, OutcomeNone, out.Kind, "x=0 is the left border")
	})

	t.Run("click on left padding column is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 1, Y: 2})
		assert.Equal(t, OutcomeNone, out.Kind, "x=1 is the left padding")
	})

	t.Run("click on right padding column is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: popupWidth - 2, Y: 2})
		assert.Equal(t, OutcomeNone, out.Kind, "x=popupWidth-2 is the right padding")
	})

	t.Run("click on right border column is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: popupWidth - 1, Y: 2})
		assert.Equal(t, OutcomeNone, out.Kind, "x=popupWidth-1 is the right border")
	})

	t.Run("click at first content column (x=2) selects", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 2, Y: 2})
		assert.Equal(t, OutcomeAnnotationChosen, out.Kind, "x=2 is the first content column")
	})

	t.Run("click respects scroll offset", func(t *testing.T) {
		mgr := NewManager()
		many := make([]AnnotationItem, 10)
		for i := range many {
			many[i] = annotItem("a.go", i+1, "+", "note")
		}
		mgr.OpenAnnotList(annotListSpec(many...))
		mgr.annotLst.height = 10 // maxVisible = 4
		mgr.annotLst.popupWidth = popupWidth
		mgr.annotLst.offset = 3

		// clicking the first visible row (localY=2) selects item at offset
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 2})
		assert.Equal(t, OutcomeAnnotationChosen, out.Kind)
		assert.Equal(t, 4, out.AnnotationTarget.Line, "row 2 with offset 3 selects item index 3 (Line=4)")
	})

	t.Run("non-press action is no-op", func(t *testing.T) {
		mgr := setup()
		out := mgr.annotLst.handleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: 5, Y: 2})
		assert.Equal(t, OutcomeNone, out.Kind)
	})
}

func TestAnnotListOverlay_HandleMouse_ClickOutsideSwallowed(t *testing.T) {
	// Manager.HandleMouse clicks outside popup bounds must not reach the overlay —
	// verified via an integration-style test that primes bounds via Compose.
	items := []AnnotationItem{annotItem("a.go", 1, "+", "note")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))

	ctx := annotRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	_ = mgr.Compose(base, ctx) // primes bounds

	require.NotZero(t, mgr.bounds.w)
	require.NotZero(t, mgr.bounds.h)

	// click far outside the popup rectangle
	out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 0, Y: 0})
	assert.Equal(t, OutcomeNone, out.Kind, "click outside popup must not select")
	assert.True(t, mgr.Active(), "click outside must not close the overlay")
}

func TestAnnotListOverlay_HandleMouse_ClickInsideTranslatesCoords(t *testing.T) {
	items := []AnnotationItem{
		annotItem("a.go", 10, "+", "first"),
		annotItem("b.go", 20, "-", "second"),
	}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))

	ctx := annotRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	_ = mgr.Compose(base, ctx) // primes bounds

	// click on first item row — in screen coords that's bounds.y + 2
	out := mgr.HandleMouse(tea.MouseMsg{
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
		X:      mgr.bounds.x + 5,
		Y:      mgr.bounds.y + 2,
	})
	assert.Equal(t, OutcomeAnnotationChosen, out.Kind, "click on first item row should select")
	require.NotNil(t, out.AnnotationTarget)
	assert.Equal(t, "a.go", out.AnnotationTarget.File)
	assert.False(t, mgr.Active(), "OutcomeAnnotationChosen auto-closes the overlay")
}

func TestAnnotListOverlay_HandleKey_OtherKeysConsumed(t *testing.T) {
	items := []AnnotationItem{annotItem("a.go", 1, "+", "note")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))

	keys := []struct {
		msg    tea.KeyMsg
		action keymap.Action
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, ""},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit},
		{tea.KeyMsg{Type: tea.KeyTab}, keymap.ActionTogglePane},
	}

	for _, k := range keys {
		out := mgr.HandleKey(k.msg, k.action)
		assert.Equal(t, OutcomeNone, out.Kind, "key %v should be consumed", k.msg)
		assert.True(t, mgr.Active(), "key %v should not close overlay", k.msg)
	}
}

func TestAnnotListOverlay_ComposeOnBase(t *testing.T) {
	items := []AnnotationItem{annotItem("a.go", 1, "+", "note")}
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(items...))
	ctx := annotRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	result := mgr.Compose(base, ctx)

	assert.Contains(t, result, "annotations (1)")
	assert.Contains(t, result, "a.go:1")

	lines := strings.Split(result, "\n")
	assert.Len(t, lines, ctx.Height, "composited result should preserve base line count")
}

func TestAnnotListOverlay_ComposeEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec())
	ctx := annotRenderCtx()
	base := makeBase(ctx.Width, ctx.Height)
	result := mgr.Compose(base, ctx)
	assert.Contains(t, result, "no annotations")
}

func TestAnnotListOverlay_DismissAction(t *testing.T) {
	mgr := NewManager()
	mgr.OpenAnnotList(annotListSpec(annotItem("a.go", 1, "+", "x")))

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestAnnotListOverlay_OpenResetsState(t *testing.T) {
	mgr := NewManager()
	items1 := []AnnotationItem{annotItem("a.go", 1, "+", "first"), annotItem("a.go", 2, "+", "second")}
	mgr.OpenAnnotList(annotListSpec(items1...))
	mgr.annotLst.cursor = 1
	mgr.annotLst.offset = 1

	items2 := []AnnotationItem{annotItem("b.go", 5, "-", "new")}
	mgr.OpenAnnotList(annotListSpec(items2...))
	assert.Equal(t, 0, mgr.annotLst.cursor, "cursor should reset on reopen")
	assert.Equal(t, 0, mgr.annotLst.offset, "offset should reset on reopen")
	require.Len(t, mgr.annotLst.items, 1)
	assert.Equal(t, "b.go", mgr.annotLst.items[0].File)
}
