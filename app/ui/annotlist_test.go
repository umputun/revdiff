package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
)

func TestModel_BuildAnnotListItems(t *testing.T) {
	t.Run("multiple files and annotations sorted by file then line", func(t *testing.T) {
		m := testModel([]string{"b.go", "a.go"}, nil)
		m.store.Add(annotation.Annotation{File: "b.go", Line: 10, Type: "+", Comment: "fix this"})
		m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: "-", Comment: "remove"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "+", Comment: "add here"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

		items := m.buildAnnotListItems()
		require.Len(t, items, 4)

		// a.go comes first (alphabetical), file-level (line 0) before line 3
		assert.Equal(t, "a.go", items[0].File)
		assert.Equal(t, 0, items[0].Line)
		assert.Equal(t, "a.go", items[1].File)
		assert.Equal(t, 3, items[1].Line)

		// b.go second, line 5 before line 10
		assert.Equal(t, "b.go", items[2].File)
		assert.Equal(t, 5, items[2].Line)
		assert.Equal(t, "b.go", items[3].File)
		assert.Equal(t, 10, items[3].Line)
	})

	t.Run("empty store returns empty slice", func(t *testing.T) {
		m := testModel(nil, nil)
		items := m.buildAnnotListItems()
		assert.Empty(t, items)
	})

	t.Run("single annotation", func(t *testing.T) {
		m := testModel([]string{"x.go"}, nil)
		m.store.Add(annotation.Annotation{File: "x.go", Line: 42, Type: "+", Comment: "check"})

		items := m.buildAnnotListItems()
		require.Len(t, items, 1)
		assert.Equal(t, "x.go", items[0].File)
		assert.Equal(t, 42, items[0].Line)
	})
}

func TestModel_AnnotListOverlay(t *testing.T) {
	t.Run("empty state shows no annotations", func(t *testing.T) {
		m := testModel(nil, nil)
		m.showAnnotList = true
		m.width = 80
		m.height = 30

		overlay := m.annotListOverlay()
		assert.Contains(t, overlay, "no annotations")
		assert.Contains(t, overlay, "annotations (0)")
	})

	t.Run("shows annotations with title count", func(t *testing.T) {
		m := testModel([]string{"handler.go"}, nil)
		m.width = 80
		m.height = 30
		m.showAnnotList = true
		m.store.Add(annotation.Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is()"})
		m.store.Add(annotation.Annotation{File: "handler.go", Line: 87, Type: "+", Comment: "add context"})
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0

		overlay := m.annotListOverlay()
		assert.Contains(t, overlay, "annotations (2)")
		assert.Contains(t, overlay, "handler.go:43")
		assert.Contains(t, overlay, "handler.go:87")
	})

	t.Run("selected item has cursor marker", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.width = 80
		m.height = 30
		m.showAnnotList = true
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "first"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "second"})
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0

		overlay := m.annotListOverlay()
		assert.Contains(t, overlay, "> ")
	})

	t.Run("file-level annotation shows file-level label", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.width = 80
		m.height = 30
		m.showAnnotList = true
		m.store.Add(annotation.Annotation{File: "main.go", Line: 0, Type: "", Comment: "review this file"})
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0

		overlay := m.annotListOverlay()
		assert.Contains(t, overlay, "main.go (file-level)")
	})

	t.Run("long comments are truncated", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.width = 60
		m.height = 30
		m.showAnnotList = true
		longComment := strings.Repeat("x", 200)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: longComment})
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0

		overlay := m.annotListOverlay()
		assert.Contains(t, overlay, "...")
		// should not contain the full comment
		assert.NotContains(t, overlay, longComment)
	})

	t.Run("scrolling with offset", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.width = 80
		m.height = 12 // small height to force scrolling
		m.showAnnotList = true

		// add many annotations
		for i := 1; i <= 20; i++ {
			m.store.Add(annotation.Annotation{File: "a.go", Line: i, Type: "+", Comment: "note"})
		}
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 15
		m.annotListOffset = 10

		overlay := m.annotListOverlay()
		// should show items starting from offset, not from the beginning
		assert.Contains(t, overlay, "a.go:11")    // offset 10 means item index 10 (line 11)
		assert.NotContains(t, overlay, "a.go:1 ") // first item should be scrolled out
	})
}

func TestModel_FormatAnnotListItem(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = newStyles(Colors{Accent: "#5f87ff", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", AddFg: "#87d787", RemoveFg: "#ff8787"})

	t.Run("add type item", func(t *testing.T) {
		a := annotation.Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "fix this"}
		result := m.formatAnnotListItem(a, 60, false)
		assert.Contains(t, result, "handler.go:43 (+)")
		assert.Contains(t, result, "fix this")
	})

	t.Run("remove type item", func(t *testing.T) {
		a := annotation.Annotation{File: "store.go", Line: 18, Type: "-", Comment: "keep this"}
		result := m.formatAnnotListItem(a, 60, false)
		assert.Contains(t, result, "store.go:18 (-)")
	})

	t.Run("selected item", func(t *testing.T) {
		a := annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"}
		result := m.formatAnnotListItem(a, 60, true)
		assert.Contains(t, result, "> ")
	})

	t.Run("file-level item", func(t *testing.T) {
		a := annotation.Annotation{File: "path/to/file.go", Line: 0, Type: "", Comment: "review"}
		result := m.formatAnnotListItem(a, 60, false)
		assert.Contains(t, result, "file.go (file-level)")
	})
}

func TestModel_ViewWithAnnotList(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
	}
	m := testModel([]string{"a.go"}, diffs)

	// simulate file load
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// open annotation list
	m.showAnnotList = true
	m.annotListItems = m.buildAnnotListItems()

	view := m.View()
	// should show empty annotation list overlay
	assert.Contains(t, view, "no annotations")
}

func TestModel_HandleAnnotListKey(t *testing.T) {
	t.Run("@ opens popup and rebuilds items", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "note"})

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
		model := result.(Model)
		assert.True(t, model.showAnnotList)
		assert.Len(t, model.annotListItems, 1)
		assert.Equal(t, 0, model.annotListCursor)
		assert.Equal(t, 0, model.annotListOffset)
	})

	t.Run("@ closes popup when already open", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
		model := result.(Model)
		assert.False(t, model.showAnnotList)
	})

	t.Run("esc closes popup", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model := result.(Model)
		assert.False(t, model.showAnnotList)
	})

	t.Run("j moves cursor down", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{
			{File: "a.go", Line: 1, Type: "+"},
			{File: "a.go", Line: 2, Type: "+"},
			{File: "a.go", Line: 3, Type: "+"},
		}
		m.annotListCursor = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 1, model.annotListCursor)
	})

	t.Run("k moves cursor up", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{
			{File: "a.go", Line: 1, Type: "+"},
			{File: "a.go", Line: 2, Type: "+"},
		}
		m.annotListCursor = 1

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 0, model.annotListCursor)
	})

	t.Run("j does not go past last item", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}
		m.annotListCursor = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 0, model.annotListCursor)
	})

	t.Run("k does not go above first item", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}
		m.annotListCursor = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 0, model.annotListCursor)
	})

	t.Run("scroll offset adjusts when cursor moves below visible area", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.height = 10 // maxVisible = max(min(N, 10-6), 1) = max(min(N, 4), 1) = 4

		items := make([]annotation.Annotation, 10)
		for i := range items {
			items[i] = annotation.Annotation{File: "a.go", Line: i + 1, Type: "+"}
		}
		m.annotListItems = items
		m.annotListCursor = 3 // last visible item at offset 0
		m.annotListOffset = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 4, model.annotListCursor)
		assert.Equal(t, 1, model.annotListOffset) // scrolled down
	})

	t.Run("scroll offset adjusts when cursor moves above visible area", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.height = 10

		items := make([]annotation.Annotation, 10)
		for i := range items {
			items[i] = annotation.Annotation{File: "a.go", Line: i + 1, Type: "+"}
		}
		m.annotListItems = items
		m.annotListCursor = 3
		m.annotListOffset = 3 // cursor is at top of visible area

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 2, model.annotListCursor)
		assert.Equal(t, 2, model.annotListOffset) // scrolled up to follow cursor
	})

	t.Run("enter closes popup", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}
		m.annotListCursor = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)
		assert.False(t, model.showAnnotList)
	})

	t.Run("other keys are consumed and do nothing", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		model := result.(Model)
		assert.True(t, model.showAnnotList) // still open, key consumed
	})

	t.Run("arrow keys work for navigation", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{
			{File: "a.go", Line: 1, Type: "+"},
			{File: "a.go", Line: 2, Type: "+"},
		}
		m.annotListCursor = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		model := result.(Model)
		assert.Equal(t, 1, model.annotListCursor)

		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
		model = result.(Model)
		assert.Equal(t, 0, model.annotListCursor)
	})

	t.Run("remapped key navigates down via keymap", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		// remap: unbind j, bind x to ActionDown
		km := keymap.Default()
		km.Unbind("j")
		km.Bind("x", keymap.ActionDown)
		m.keymap = km
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{
			{File: "a.go", Line: 1, Type: "+"},
			{File: "a.go", Line: 2, Type: "+"},
			{File: "a.go", Line: 3, Type: "+"},
		}
		m.annotListCursor = 0

		// x should navigate down (remapped)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		model := result.(Model)
		assert.Equal(t, 1, model.annotListCursor)

		// j should no longer navigate (unbound)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		assert.Equal(t, 1, model.annotListCursor, "j should be consumed as no-op after unbind")
	})

	t.Run("enter and esc work regardless of keymap remapping", func(t *testing.T) {
		km := keymap.Default()
		km.Unbind("esc") // unbind esc from ActionDismiss

		// esc still closes popup via tea.KeyEsc fallback
		m := testModel([]string{"a.go"}, nil)
		m.keymap = km
		m.showAnnotList = true
		m.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model := result.(Model)
		assert.False(t, model.showAnnotList, "esc should close popup even when unbound from keymap")

		// enter still works regardless of keymap
		m2 := testModel([]string{"a.go"}, nil)
		m2.keymap = km
		m2.showAnnotList = true
		m2.annotListItems = []annotation.Annotation{{File: "a.go", Line: 1, Type: "+"}}
		m2.annotListCursor = 0

		result, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = result.(Model)
		assert.False(t, model.showAnnotList, "enter should close popup regardless of keymap")
	})
}

func TestModel_FindDiffLineIndex(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeContext, Content: "ctx1", OldNum: 1, NewNum: 1},
			{ChangeType: diff.ChangeRemove, Content: "old", OldNum: 2, NewNum: 0},
			{ChangeType: diff.ChangeAdd, Content: "new", OldNum: 0, NewNum: 2},
			{ChangeType: diff.ChangeContext, Content: "ctx2", OldNum: 3, NewNum: 3},
		},
	}
	m := testModel([]string{"a.go"}, diffs)
	m.diffLines = diffs["a.go"]

	t.Run("find add line by NewNum", func(t *testing.T) {
		idx := m.findDiffLineIndex(2, "+")
		assert.Equal(t, 2, idx)
	})

	t.Run("find remove line by OldNum", func(t *testing.T) {
		idx := m.findDiffLineIndex(2, "-")
		assert.Equal(t, 1, idx)
	})

	t.Run("find context line by NewNum", func(t *testing.T) {
		idx := m.findDiffLineIndex(3, " ")
		assert.Equal(t, 3, idx)
	})

	t.Run("not found returns -1", func(t *testing.T) {
		idx := m.findDiffLineIndex(99, "+")
		assert.Equal(t, -1, idx)
	})

	t.Run("wrong change type returns -1", func(t *testing.T) {
		idx := m.findDiffLineIndex(2, " ")
		assert.Equal(t, -1, idx)
	})
}

func TestModel_JumpToAnnotation_SameFile(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1},
			{ChangeType: diff.ChangeAdd, Content: "new", OldNum: 0, NewNum: 2},
			{ChangeType: diff.ChangeContext, Content: "line3", OldNum: 2, NewNum: 3},
		},
	}
	m := testModel([]string{"a.go"}, diffs)

	// simulate file load
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "check this"})

	t.Run("same-file jump positions cursor", func(t *testing.T) {
		m.showAnnotList = true
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)
		assert.False(t, model.showAnnotList)
		assert.Equal(t, 1, model.diffCursor) // index 1 is the add line
		assert.Equal(t, paneDiff, model.focus)
	})

	t.Run("file-level annotation sets cursor to -1", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
		m.showAnnotList = true
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0 // file-level comes first (line 0)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)
		assert.False(t, model.showAnnotList)
		assert.Equal(t, -1, model.diffCursor)
	})
}

func TestModel_JumpToAnnotation_CrossFile(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
		"b.go": {
			{ChangeType: diff.ChangeContext, Content: "ctx", OldNum: 1, NewNum: 1},
			{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 2},
		},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)

	// load files and first file
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)
	assert.Equal(t, "a.go", m.currFile)

	// add annotation in b.go
	m.store.Add(annotation.Annotation{File: "b.go", Line: 2, Type: "+", Comment: "check"})

	t.Run("cross-file jump sets pending and triggers load", func(t *testing.T) {
		m.showAnnotList = true
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0

		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)
		assert.False(t, model.showAnnotList)
		assert.NotNil(t, model.pendingAnnotJump)
		assert.Equal(t, "b.go", model.pendingAnnotJump.File)
		require.NotNil(t, cmd)

		// simulate file loaded
		loadMsg := cmd()
		result, _ = model.Update(loadMsg)
		model = result.(Model)
		assert.Equal(t, "b.go", model.currFile)
		assert.Nil(t, model.pendingAnnotJump)
		assert.Equal(t, 1, model.diffCursor) // index 1 is the add line
		assert.Equal(t, paneDiff, model.focus)
	})
}

func TestModel_JumpToAnnotation_StalePendingGuard(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)

	// load files and first file
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	t.Run("stale pending jump is ignored when file does not match", func(t *testing.T) {
		// set pending for b.go but simulate a.go being loaded
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		m.loadSeq++
		loadMsg := fileLoadedMsg{file: "a.go", seq: m.loadSeq, lines: diffs["a.go"]}
		result, _ := m.Update(loadMsg)
		model := result.(Model)
		// pending should not be cleared (file mismatch)
		assert.NotNil(t, model.pendingAnnotJump)
		assert.Equal(t, "b.go", model.pendingAnnotJump.File)
	})

	t.Run("n key clears pending jump", func(t *testing.T) {
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		m.focus = paneDiff
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Nil(t, model.pendingAnnotJump)
	})

	t.Run("p key clears pending jump", func(t *testing.T) {
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Nil(t, model.pendingAnnotJump)
	})
}

func TestModel_PendingAnnotJump_ClearedByTreeNav(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)
	m.focus = paneTree

	t.Run("tree j clears pending jump", func(t *testing.T) {
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Nil(t, model.pendingAnnotJump)
	})

	t.Run("tree k clears pending jump", func(t *testing.T) {
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Nil(t, model.pendingAnnotJump)
	})
}

func TestModel_PendingAnnotJump_ClearedByFilterToggle(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// add annotation so filter has something to toggle
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
	m.focus = paneDiff
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model := result.(Model)
	assert.Nil(t, model.pendingAnnotJump)
}

func TestModel_PositionOnAnnotation_CollapsedMode(t *testing.T) {
	// set up a file with a remove line inside a hunk
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeDivider, Content: "@@ -1,3 +1,2 @@"},
			{ChangeType: diff.ChangeContext, Content: "ctx", OldNum: 1, NewNum: 1},
			{ChangeType: diff.ChangeRemove, Content: "old", OldNum: 2, NewNum: 0},
			{ChangeType: diff.ChangeAdd, Content: "new", OldNum: 0, NewNum: 2},
		},
	}
	m := testModel([]string{"a.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// enable collapsed mode - remove line at index 2 should be hidden
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)

	a := annotation.Annotation{File: "a.go", Line: 2, Type: "-"}
	m.positionOnAnnotation(a)

	// hunk should be expanded so the remove line is visible
	assert.Equal(t, 2, m.diffCursor)
	hunks := m.findHunks()
	assert.False(t, m.isCollapsedHidden(m.diffCursor, hunks), "target line should be visible after jump")
}

func TestModel_PositionOnAnnotation_DeleteOnlyHunk(t *testing.T) {
	// delete-only hunk: first line is a placeholder in collapsed mode,
	// ensureHunkExpanded must expand it so the annotation is visible
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeDivider, Content: "@@ -1,2 +1,0 @@"},
			{ChangeType: diff.ChangeRemove, Content: "deleted1", OldNum: 1, NewNum: 0},
			{ChangeType: diff.ChangeRemove, Content: "deleted2", OldNum: 2, NewNum: 0},
		},
	}
	m := testModel([]string{"a.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// enable collapsed mode - first remove line is a delete-only placeholder
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)

	a := annotation.Annotation{File: "a.go", Line: 1, Type: "-"}
	m.positionOnAnnotation(a)

	// hunk must be expanded so the actual line and annotation are visible
	assert.Equal(t, 1, m.diffCursor)
	hunks := m.findHunks()
	hunkStart := m.hunkStartFor(m.diffCursor, hunks)
	assert.True(t, m.collapsed.expandedHunks[hunkStart], "delete-only hunk should be expanded after jump")
	assert.False(t, m.isDeleteOnlyPlaceholder(m.diffCursor, hunks), "line should not be a placeholder after expansion")
}

func TestModel_JumpToAnnotation_EmptyList(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.showAnnotList = true
	m.annotListItems = nil // empty
	m.annotListCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.showAnnotList)
}
