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

func annotListSpec(items ...AnnotationItem) AnnotListSpec {
	return AnnotListSpec{Items: items}
}

func annotItem(file string, line int, changeType, comment string) AnnotationItem {
	return AnnotationItem{
		File: file, Line: line, ChangeType: changeType, Comment: comment,
		Target: AnnotationTarget{File: file, ChangeType: changeType, Line: line},
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
