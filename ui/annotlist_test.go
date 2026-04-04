package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
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
	result, _ := m.Update(filesLoadedMsg{files: []string{"a.go"}})
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
