package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// pickAdjacentAnnotation is a test-only wrapper that resolves the starting
// flat-list index via startingFlatIndex (the production walker entry point)
// and returns the annotation at that index. Production code walks indexes
// directly inside handleAnnotNav for O(N) navigation; this wrapper keeps
// the table-driven picking-algorithm tests intact as algorithm documentation.
func pickAdjacentAnnotation(flat []annotation.Annotation, cur cursorAnnotKey, forward bool) (annotation.Annotation, bool) {
	idx := startingFlatIndex(flat, cur, forward)
	if idx < 0 || idx >= len(flat) {
		return annotation.Annotation{}, false
	}
	return flat[idx], true
}

func TestPickAdjacentAnnotation(t *testing.T) {
	a1 := annotation.Annotation{File: "a.go", Line: 5, Type: "+"}
	a2 := annotation.Annotation{File: "a.go", Line: 10, Type: "+"}
	b1 := annotation.Annotation{File: "b.go", Line: 0, Type: ""}
	b2 := annotation.Annotation{File: "b.go", Line: 7, Type: "-"}
	flat := []annotation.Annotation{a1, a2, b1, b2}

	tests := []struct {
		name    string
		flat    []annotation.Annotation
		cur     cursorAnnotKey
		forward bool
		want    annotation.Annotation
		ok      bool
	}{
		{"empty list forward", nil, cursorAnnotKey{file: "a.go", line: 1}, true, annotation.Annotation{}, false},
		{"empty list backward", nil, cursorAnnotKey{file: "a.go", line: 1}, false, annotation.Annotation{}, false},

		{"forward from before first", flat, cursorAnnotKey{file: "a.go", line: 1}, true, a1, true},
		{"backward from before first → no-op", flat, cursorAnnotKey{file: "a.go", line: 1}, false, annotation.Annotation{}, false},

		{"forward from on first annotation", flat, cursorAnnotKey{file: "a.go", line: 5, typ: "+", onAnnot: true}, true, a2, true},
		{"backward from on first annotation → no-op", flat, cursorAnnotKey{file: "a.go", line: 5, typ: "+", onAnnot: true}, false, annotation.Annotation{}, false},

		{"forward from on middle annotation", flat, cursorAnnotKey{file: "a.go", line: 10, typ: "+", onAnnot: true}, true, b1, true},
		{"backward from on middle annotation", flat, cursorAnnotKey{file: "a.go", line: 10, typ: "+", onAnnot: true}, false, a1, true},

		{"forward crosses file boundary", flat, cursorAnnotKey{file: "a.go", line: 99}, true, b1, true},
		{"backward from b.go line 1 lands on b.go file-level", flat, cursorAnnotKey{file: "b.go", line: 1}, false, b1, true},
		{"backward from b.go line -1 crosses to a.go", flat, cursorAnnotKey{file: "b.go", line: -1}, false, a2, true},

		{"forward from on last annotation → no-op", flat, cursorAnnotKey{file: "b.go", line: 7, typ: "-", onAnnot: true}, true, annotation.Annotation{}, false},
		{"backward from on last annotation", flat, cursorAnnotKey{file: "b.go", line: 7, typ: "-", onAnnot: true}, false, b1, true},

		{"forward past last → no-op", flat, cursorAnnotKey{file: "z.go", line: 1}, true, annotation.Annotation{}, false},
		{"backward past last → last item", flat, cursorAnnotKey{file: "z.go", line: 1}, false, b2, true},

		// "az.go" sorts between "a.go" and "b.go" alphabetically:
		// "a.go" < "az.go" because '.' (0x2e) < 'z' (0x7a) at index 1, and "az.go" < "b.go" because 'a' < 'b'.
		{"forward from file with no annotations", flat, cursorAnnotKey{file: "az.go", line: 5}, true, b1, true},
		{"backward from file with no annotations", flat, cursorAnnotKey{file: "az.go", line: 5}, false, a2, true},

		// file-level annotation (Line=0)
		{"forward from before file-level lands on file-level", flat, cursorAnnotKey{file: "b.go", line: -1}, true, b1, true},
		{"forward from on file-level annotation goes to next within file",
			flat, cursorAnnotKey{file: "b.go", line: 0, typ: "", onAnnot: true}, true, b2, true},
		{"backward from on file-level annotation crosses to prev file",
			flat, cursorAnnotKey{file: "b.go", line: 0, typ: "", onAnnot: true}, false, a2, true},

		// edge case: cursor on context line at same line as a typed annotation
		// not exactly on it (different type) — strict "after" excludes same-line
		// annotations, so forward skips; backward picks it up via insertion-1.
		{"forward from context at same line as add-annotation skips same-line",
			[]annotation.Annotation{
				{File: "a.go", Line: 5, Type: "+"},
				{File: "a.go", Line: 9, Type: "+"},
			},
			cursorAnnotKey{file: "a.go", line: 5, typ: " "}, true,
			annotation.Annotation{File: "a.go", Line: 9, Type: "+"}, true,
		},
		{"backward from context at same line as add-annotation reaches it",
			[]annotation.Annotation{
				{File: "a.go", Line: 1, Type: "+"},
				{File: "a.go", Line: 5, Type: "+"},
			},
			cursorAnnotKey{file: "a.go", line: 5, typ: " "}, false,
			annotation.Annotation{File: "a.go", Line: 5, Type: "+"}, true,
		},

		// onAnnot=true but no triple match in flat — exactAnnotIndex falls
		// through to the insertion-point branch. Reaches this state when the
		// caller constructs cursorAnnotKey with onAnnot=true but the exact
		// (file, line, type) triple is not in flat (e.g. matched a different-
		// type annotation at the same line). Must NOT cause incorrect
		// stepping; must use insertion-point fallback.
		{"forward with onAnnot=true but no exact match falls through to insertion-point",
			flat,
			cursorAnnotKey{file: "a.go", line: 5, typ: "-", onAnnot: true}, true,
			a2, true,
		},
		{"backward with onAnnot=true but no exact match falls through to insertion-point",
			flat,
			cursorAnnotKey{file: "a.go", line: 10, typ: "-", onAnnot: true}, false,
			a2, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pickAdjacentAnnotation(tt.flat, tt.cur, tt.forward)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestCompareAnnotPos(t *testing.T) {
	tests := []struct {
		name               string
		aFile, bFile       string
		aLine, bLine, want int
	}{
		{"same file same line", "a.go", "a.go", 5, 5, 0},
		{"same file lower line", "a.go", "a.go", 3, 5, -1},
		{"same file higher line", "a.go", "a.go", 7, 5, 1},
		{"earlier file", "a.go", "b.go", 100, 1, -1},
		{"later file", "b.go", "a.go", 1, 100, 1},
		{"same file file-level vs line", "a.go", "a.go", 0, 5, -1},
		{"same file line vs file-level (symmetry)", "a.go", "a.go", 5, 0, 1},
		{"same file both file-level", "a.go", "a.go", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compareAnnotPos(tt.aFile, tt.aLine, tt.bFile, tt.bLine))
		})
	}
}

func TestModel_CurrentAnnotKey(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeContext, Content: "ctx", OldNum: 1, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 2},
		{ChangeType: diff.ChangeRemove, Content: "removed", OldNum: 2, NewNum: 0},
	}
	m := testModel([]string{"a.go"}, nil)
	m.file.name = "a.go"
	m.file.lines = lines

	t.Run("on context line", func(t *testing.T) {
		m.nav.diffCursor = 0
		key := m.currentAnnotKey()
		assert.Equal(t, "a.go", key.file)
		assert.Equal(t, 1, key.line)
		assert.Equal(t, " ", key.typ)
		assert.False(t, key.onAnnot)
	})

	t.Run("on add line uses NewNum", func(t *testing.T) {
		m.nav.diffCursor = 1
		key := m.currentAnnotKey()
		assert.Equal(t, 2, key.line)
		assert.Equal(t, "+", key.typ)
	})

	t.Run("on remove line uses OldNum", func(t *testing.T) {
		m.nav.diffCursor = 2
		key := m.currentAnnotKey()
		assert.Equal(t, 2, key.line)
		assert.Equal(t, "-", key.typ)
	})

	t.Run("onAnnot true when store has matching annotation", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "x"})
		m.nav.diffCursor = 1
		key := m.currentAnnotKey()
		assert.True(t, key.onAnnot)
	})

	t.Run("file-level cursor (-1) maps to file-level key", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
		m.nav.diffCursor = -1
		key := m.currentAnnotKey()
		assert.Equal(t, 0, key.line)
		assert.Empty(t, key.typ)
		assert.True(t, key.onAnnot)
	})

	t.Run("out-of-range cursor returns line -1", func(t *testing.T) {
		m.nav.diffCursor = 999
		key := m.currentAnnotKey()
		assert.Equal(t, -1, key.line)
		assert.False(t, key.onAnnot)
	})

	t.Run("leading ChangeDivider collapses to line -1 so file-level annot stays reachable", func(t *testing.T) {
		// dividers carry OldNum=NewNum=0; without the guard they would
		// alias the file-level annotation key and cause forward navigation
		// from the divider to skip the same-file file-level annotation.
		divLines := []diff.DiffLine{
			{ChangeType: diff.ChangeDivider, Content: "⋯ 5 lines ⋯", OldNum: 0, NewNum: 0},
			{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 6},
		}
		dm := testModel([]string{"a.go"}, nil)
		dm.file.name = "a.go"
		dm.file.lines = divLines
		dm.nav.diffCursor = 0
		key := dm.currentAnnotKey()
		assert.Equal(t, -1, key.line)
		assert.Empty(t, key.typ)
		assert.False(t, key.onAnnot)
	})

	t.Run("middle ChangeDivider inherits position from preceding non-divider line", func(t *testing.T) {
		// without this, a mouse-click on a middle divider would map to
		// line=-1 and forward } would jump back to the file-level/first
		// same-file annotation instead of to the next annotation after
		// the divider's logical position.
		divLines := []diff.DiffLine{
			{ChangeType: diff.ChangeAdd, Content: "first", OldNum: 0, NewNum: 5},
			{ChangeType: diff.ChangeDivider, Content: "⋯ 10 lines ⋯", OldNum: 0, NewNum: 0},
			{ChangeType: diff.ChangeAdd, Content: "later", OldNum: 0, NewNum: 16},
		}
		dm := testModel([]string{"a.go"}, nil)
		dm.file.name = "a.go"
		dm.file.lines = divLines
		dm.nav.diffCursor = 1 // on the divider
		key := dm.currentAnnotKey()
		assert.Equal(t, 5, key.line, "middle divider must inherit preceding line number")
		assert.Equal(t, "+", key.typ, "middle divider must inherit preceding type")
		assert.False(t, key.onAnnot)
	})

	t.Run("trailing ChangeDivider inherits position from preceding non-divider line", func(t *testing.T) {
		divLines := []diff.DiffLine{
			{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 5},
			{ChangeType: diff.ChangeDivider, Content: "⋯ 3 lines ⋯", OldNum: 0, NewNum: 0},
		}
		dm := testModel([]string{"a.go"}, nil)
		dm.file.name = "a.go"
		dm.file.lines = divLines
		dm.nav.diffCursor = 1 // trailing divider
		key := dm.currentAnnotKey()
		assert.Equal(t, 5, key.line)
		assert.Equal(t, "+", key.typ)
	})

	t.Run("consecutive dividers walk back through them to reach a non-divider", func(t *testing.T) {
		divLines := []diff.DiffLine{
			{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 5},
			{ChangeType: diff.ChangeDivider, Content: "⋯ 3 lines ⋯", OldNum: 0, NewNum: 0},
			{ChangeType: diff.ChangeDivider, Content: "⋯ 2 lines ⋯", OldNum: 0, NewNum: 0},
		}
		dm := testModel([]string{"a.go"}, nil)
		dm.file.name = "a.go"
		dm.file.lines = divLines
		dm.nav.diffCursor = 2 // on the second of two consecutive dividers
		key := dm.currentAnnotKey()
		assert.Equal(t, 5, key.line, "must walk past intermediate divider to reach line 5")
	})
}

// pin the divider middle/trailing fix at the integration level: clicking on
// a middle divider then pressing } must jump to the next annotation strictly
// after the divider's logical position, not back to the file-level/first
// same-file annotation.
func TestModel_HandleAnnotNav_FromMiddleDivider(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L5", OldNum: 0, NewNum: 5},
		{ChangeType: diff.ChangeDivider, Content: "⋯ 10 lines ⋯", OldNum: 0, NewNum: 0},
		{ChangeType: diff.ChangeAdd, Content: "L16", OldNum: 0, NewNum: 16},
		{ChangeType: diff.ChangeAdd, Content: "L17", OldNum: 0, NewNum: 17},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)

	// annotations: file-level, line 5 (before divider), line 17 (after divider)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "before"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 17, Type: "+", Comment: "after"})

	t.Run("forward from middle divider lands on next annotation past divider", func(t *testing.T) {
		m.nav.diffCursor = 1 // middle divider
		result, _ := m.handleAnnotNav(true)
		model := result.(Model)
		// expected: line 17 = index 3
		assert.Equal(t, 3, model.nav.diffCursor, "} from middle divider must jump past line 5 to line 17")
	})

	t.Run("backward from middle divider lands on annotation at-or-before logical position", func(t *testing.T) {
		m.nav.diffCursor = 1 // middle divider
		result, _ := m.handleAnnotNav(false)
		model := result.(Model)
		// expected: line 5 = index 0
		assert.Equal(t, 0, model.nav.diffCursor, "{ from middle divider must reach line 5")
	})
}

func TestModel_HandleAnnotNav_WithinFile(t *testing.T) {
	// layout: ctx (line 1) | + (line 2) | + (line 3) | + (line 4)
	// annotations at lines 2 and 4 — context line 1 lets us test "before first"
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeContext, Content: "ctx", OldNum: 1, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "a", OldNum: 0, NewNum: 2},
		{ChangeType: diff.ChangeAdd, Content: "b", OldNum: 0, NewNum: 3},
		{ChangeType: diff.ChangeAdd, Content: "c", OldNum: 0, NewNum: 4},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "first"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 4, Type: "+", Comment: "second"})

	t.Run("forward from before first lands on first", func(t *testing.T) {
		m.nav.diffCursor = 0 // ctx line 1, before any annotation
		result, _ := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, 1, model.nav.diffCursor) // line 2 = index 1
	})

	t.Run("forward from on first lands on second", func(t *testing.T) {
		m.nav.diffCursor = 1 // line 2, on first annotation
		result, _ := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, 3, model.nav.diffCursor) // line 4 = index 3
	})

	t.Run("forward from on second is no-op", func(t *testing.T) {
		m.nav.diffCursor = 3 // line 4, on second annotation
		result, _ := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, 3, model.nav.diffCursor) // unchanged
	})

	t.Run("backward from on second lands on first", func(t *testing.T) {
		m.nav.diffCursor = 3
		result, _ := m.handleAnnotNav(false)
		model := result.(Model)
		assert.Equal(t, 1, model.nav.diffCursor)
	})

	t.Run("backward from on first is no-op", func(t *testing.T) {
		m.nav.diffCursor = 1
		result, _ := m.handleAnnotNav(false)
		model := result.(Model)
		assert.Equal(t, 1, model.nav.diffCursor)
	})

	t.Run("empty store is no-op", func(t *testing.T) {
		m.store.Clear()
		m.nav.diffCursor = 1
		before := m.nav.diffCursor
		result, cmd := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, before, model.nav.diffCursor)
		assert.Nil(t, cmd)
	})
}

func TestModel_HandleAnnotNav_CrossFile(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "a1", OldNum: 0, NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "b1", OldNum: 0, NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "in a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 1, Type: "+", Comment: "in b"})

	t.Run("forward from on a.go's annotation crosses to b.go", func(t *testing.T) {
		m.nav.diffCursor = 0 // line 1, on a.go annotation
		result, cmd := m.handleAnnotNav(true)
		model := result.(Model)
		require.NotNil(t, model.pendingAnnotJump, "must set pendingAnnotJump for cross-file target")
		assert.Equal(t, "b.go", model.pendingAnnotJump.File)
		require.NotNil(t, cmd)

		// simulate file load completing
		loadMsg := cmd()
		result, _ = model.Update(loadMsg)
		model = result.(Model)
		assert.Equal(t, "b.go", model.file.name)
		assert.Nil(t, model.pendingAnnotJump)
	})

	t.Run("forward from on b.go's annotation is no-op (last in flat list)", func(t *testing.T) {
		// load b.go and position on its annotation
		loadMsg := m.loadFileDiff("b.go")()
		result, _ := m.Update(loadMsg)
		model := result.(Model)
		model.nav.diffCursor = 0
		result, cmd := model.handleAnnotNav(true)
		model = result.(Model)
		assert.Nil(t, model.pendingAnnotJump, "no boundary crossing at last annotation")
		assert.Nil(t, cmd)
		assert.Equal(t, "b.go", model.file.name)
	})

	t.Run("backward from on a.go's annotation is no-op (first in flat list)", func(t *testing.T) {
		m.nav.diffCursor = 0
		result, cmd := m.handleAnnotNav(false)
		model := result.(Model)
		assert.Nil(t, model.pendingAnnotJump)
		assert.Nil(t, cmd)
		assert.Equal(t, "a.go", model.file.name)
	})
}

func TestModel_HandleAnnotNav_DeletionRobustness(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "a", OldNum: 0, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "b", OldNum: 0, NewNum: 2},
		{ChangeType: diff.ChangeAdd, Content: "c", OldNum: 0, NewNum: 3},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)

	t.Run("delete current annotation then forward lands on next", func(t *testing.T) {
		m.store.Clear()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "x"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "+", Comment: "y"})
		m.nav.diffCursor = 0 // on line 1
		m.store.Delete("a.go", 1, "+")
		result, _ := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, 2, model.nav.diffCursor) // jumped to line 3 even though cursor still at index 0
	})

	t.Run("delete current annotation then backward lands on prev", func(t *testing.T) {
		m.store.Clear()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "x"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "+", Comment: "y"})
		m.nav.diffCursor = 2 // on line 3
		m.store.Delete("a.go", 3, "+")
		result, _ := m.handleAnnotNav(false)
		model := result.(Model)
		assert.Equal(t, 0, model.nav.diffCursor) // jumped to line 1
	})

	t.Run("delete only annotation then either direction is no-op", func(t *testing.T) {
		m.store.Clear()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "x"})
		m.nav.diffCursor = 0
		m.store.Delete("a.go", 1, "+")

		result, cmd := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, 0, model.nav.diffCursor)
		assert.Nil(t, cmd)

		result, cmd = m.handleAnnotNav(false)
		model = result.(Model)
		assert.Equal(t, 0, model.nav.diffCursor)
		assert.Nil(t, cmd)
	})
}

func TestModel_HandleAnnotNav_KeymapDispatch(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "a", OldNum: 0, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "b", OldNum: 0, NewNum: 2},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "x"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "y"})

	t.Run("} key triggers next annotation", func(t *testing.T) {
		m.nav.diffCursor = 0
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'}'}})
		model := result.(Model)
		assert.Equal(t, 1, model.nav.diffCursor)
	})

	t.Run("{ key triggers prev annotation", func(t *testing.T) {
		m.nav.diffCursor = 1
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'{'}})
		model := result.(Model)
		assert.Equal(t, 0, model.nav.diffCursor)
	})

	// pin the always-on cross-file invariant: the actions sit in the global
	// dispatch switch (model.go), not in handleDiffAction, so } / { must fire
	// regardless of which pane is focused. A future refactor that demoted the
	// case into the pane-specific handler would silently regress tree-focus
	// behavior.
	t.Run("} fires from tree pane focus (always-on invariant)", func(t *testing.T) {
		m.nav.diffCursor = 0
		m.layout.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'}'}})
		model := result.(Model)
		assert.Equal(t, 1, model.nav.diffCursor, "} must fire from tree-pane focus")
	})

	t.Run("{ fires from tree pane focus (always-on invariant)", func(t *testing.T) {
		m.nav.diffCursor = 1
		m.layout.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'{'}})
		model := result.(Model)
		assert.Equal(t, 0, model.nav.diffCursor, "{ must fire from tree-pane focus")
	})
}

// modal-active suppression: }/ { must NOT navigate while a textinput modal
// (annotation input or search input) is consuming keystrokes. handleModalKey
// short-circuits before dispatch so the literal characters reach the input.
func TestModel_HandleAnnotNav_ModalSuppression(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "a", OldNum: 0, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "b", OldNum: 0, NewNum: 2},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "x"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "y"})

	t.Run("} does not navigate while annotation input is active", func(t *testing.T) {
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff
		// open the annotation modal on the current line
		cmd := m.startAnnotation()
		require.NotNil(t, cmd)
		require.True(t, m.annot.annotating, "annotation modal must be active for this test")
		before := m.nav.diffCursor
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'}'}})
		model := result.(Model)
		assert.Equal(t, before, model.nav.diffCursor, "annotation modal must swallow }")
		assert.True(t, model.annot.annotating, "annotation modal must remain active")
	})

	t.Run("{ does not navigate while search input is active", func(t *testing.T) {
		m.nav.diffCursor = 1
		m.annot.annotating = false
		m.layout.focus = paneDiff
		cmd := m.startSearch()
		require.NotNil(t, cmd)
		require.True(t, m.search.active, "search modal must be active for this test")
		before := m.nav.diffCursor
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'{'}})
		model := result.(Model)
		assert.Equal(t, before, model.nav.diffCursor, "search modal must swallow {")
		assert.True(t, model.search.active, "search modal must remain active")
	})
}

// godoc on handleAnnotNav claims "collapsed-hunk expansion comes for free" via
// jumpToAnnotationTarget → ensureHunkExpanded. This test pins that contract:
// jumping to an annotation inside a collapsed hunk must expand the hunk.
func TestModel_HandleAnnotNav_CollapsedModeExpandsHunk(t *testing.T) {
	// build a diff with a context block then a change hunk so collapsed mode
	// has something to collapse. annotation lives on the add line inside the
	// hunk; expansion is detectable via expandedHunks map.
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeContext, Content: "ctx1", OldNum: 1, NewNum: 1},
		{ChangeType: diff.ChangeContext, Content: "ctx2", OldNum: 2, NewNum: 2},
		{ChangeType: diff.ChangeRemove, Content: "old", OldNum: 3, NewNum: 0},
		{ChangeType: diff.ChangeAdd, Content: "new", OldNum: 0, NewNum: 3},
		{ChangeType: diff.ChangeContext, Content: "ctx3", OldNum: 4, NewNum: 4},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)

	// enable collapsed mode and place annotation on the remove line (which
	// is hidden by collapsed mode by default)
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = map[int]bool{}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "-", Comment: "removed"})

	// position cursor at the top of the file so } walks forward to the annotation
	m.nav.diffCursor = 0
	result, _ = m.handleAnnotNav(true)
	model := result.(Model)
	hunks := model.findHunks()
	require.NotEmpty(t, hunks, "test setup must produce at least one hunk")
	assert.Truef(t, model.modes.collapsed.expandedHunks[hunks[0]],
		"jumping to annotation inside collapsed hunk must expand the hunk; expandedHunks=%v", model.modes.collapsed.expandedHunks)
}

// pin backward cross-file integration: } symmetric coverage exists, but {
// crossing a file boundary triggers the same pendingAnnotJump handoff and
// loadSelectedIfChanged path. Asymmetric coverage would let direction-handling
// regressions slip through.
func TestModel_HandleAnnotNav_BackwardCrossFile(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "a1", OldNum: 0, NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "b1", OldNum: 0, NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	// load b.go and position on its annotation
	loadMsg := m.loadFileDiff("b.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "in a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 1, Type: "+", Comment: "in b"})
	m.nav.diffCursor = 0 // on b.go's annotation

	result, cmd := m.handleAnnotNav(false)
	model := result.(Model)
	require.NotNil(t, model.pendingAnnotJump, "{ across files must set pendingAnnotJump")
	assert.Equal(t, "a.go", model.pendingAnnotJump.File)
	require.NotNil(t, cmd)

	loadCmd := cmd()
	result, _ = model.Update(loadCmd)
	model = result.(Model)
	assert.Equal(t, "a.go", model.file.name)
	assert.Nil(t, model.pendingAnnotJump)
}

// walker-stuck retry: when the immediate adjacent target is non-jumpable
// (cross-file path missing from tree, or same-file line missing from loaded
// diff in compact mode), the walker must skip past it and find the next
// jumpable candidate instead of getting trapped.
func TestModel_HandleAnnotNav_SkipsNonJumpableSameFileTarget(t *testing.T) {
	// loaded diff only has line 5 (compact mode could shrink context like this).
	// Annotations exist at lines 3 (NOT in loaded diff → non-jumpable) and 7
	// (NOT in loaded diff → non-jumpable) and 5 (jumpable).
	// Cursor sits BEFORE line 5; forward walker must skip line-3 candidate
	// (wait, line 3 < 5 so forward never sees it), then skip nothing, then
	// land on line 5 — for clearer test, put one non-jumpable AFTER line 5
	// and one jumpable further after.
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L5", OldNum: 0, NewNum: 5},
		{ChangeType: diff.ChangeAdd, Content: "L9", OldNum: 0, NewNum: 9},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)

	// annotations: line 5 (jumpable), line 7 (NOT in loaded diff → non-jumpable),
	// line 9 (jumpable). Cursor on line 5.
	m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "a"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 7, Type: "+", Comment: "stuck"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 9, Type: "+", Comment: "c"})
	m.nav.diffCursor = 0 // line 5

	result, _ = m.handleAnnotNav(true)
	model := result.(Model)
	// must skip line-7 (non-jumpable) and land on line 9 (jumpable, index 1)
	assert.Equal(t, 1, model.nav.diffCursor, "walker must skip non-jumpable line-7 annotation and reach line-9")
}

// stuck-loop bound check: when ALL forward annotations are non-jumpable,
// the walker must terminate (bounded by len(flat)) and leave the cursor
// unchanged rather than spinning.
func TestModel_HandleAnnotNav_AllNonJumpableTerminates(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L5", OldNum: 0, NewNum: 5},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)

	// only annotation is at line 7, which is not in the loaded diff
	m.store.Add(annotation.Annotation{File: "a.go", Line: 7, Type: "+", Comment: "stuck"})
	m.nav.diffCursor = 0 // line 5

	before := m.nav.diffCursor
	result, cmd := m.handleAnnotNav(true)
	model := result.(Model)
	assert.Equal(t, before, model.nav.diffCursor, "walker must not move when no jumpable target exists")
	assert.Nil(t, cmd)
}

// jumping to a line-level annotation must land the cursor ON the annotation
// comment sub-row (cursorOnAnnotation=true), not on the diff line above it.
// The diff line and the comment sub-row render as separate visual rows; without
// cursorOnAnnotation the highlight sits one row above the comment, which is
// what users perceive as "cursor lands one line above the annotation."
// Both }/{ and the @ popup go through positionOnAnnotation, so this single
// invariant covers both navigation paths.
func TestModel_HandleAnnotNav_CursorLandsOnAnnotationRow(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L1", OldNum: 0, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "L2", OldNum: 0, NewNum: 2},
		{ChangeType: diff.ChangeAdd, Content: "L3", OldNum: 0, NewNum: 3},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "+", Comment: "target"})

	t.Run("} sets cursorOnAnnotation when landing on a line-level annotation", func(t *testing.T) {
		m.nav.diffCursor = 0
		m.annot.cursorOnAnnotation = false
		result, _ := m.handleAnnotNav(true)
		model := result.(Model)
		assert.Equal(t, 2, model.nav.diffCursor, "cursor must be on the annotated diff line")
		assert.True(t, model.annot.cursorOnAnnotation, "cursor must render on the annotation comment sub-row, not the diff line above it")
	})

	t.Run("{ sets cursorOnAnnotation when landing on a line-level annotation", func(t *testing.T) {
		m.nav.diffCursor = 2
		m.annot.cursorOnAnnotation = false
		// add a second annotation so { has somewhere to go
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "earlier"})
		result, _ := m.handleAnnotNav(false)
		model := result.(Model)
		assert.Equal(t, 0, model.nav.diffCursor)
		assert.True(t, model.annot.cursorOnAnnotation, "cursor must render on the annotation comment sub-row")
	})
}

// jumpToAnnotationTarget is the shared entry point for the @ popup's
// selection callback and for the }/{ walker. The same cursor-on-annotation
// invariant must apply to popup-driven jumps.
func TestModel_JumpToAnnotationTarget_CursorLandsOnAnnotationRow(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L1", OldNum: 0, NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "L2", OldNum: 0, NewNum: 2},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "popup target"})

	m.annot.cursorOnAnnotation = false
	result, _ = m.jumpToAnnotationTarget(&overlay.AnnotationTarget{File: "a.go", ChangeType: "+", Line: 2})
	model := result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor)
	assert.True(t, model.annot.cursorOnAnnotation, "@ popup selection must land cursor on annotation comment row")
}

// file-level annotations use diffCursor=-1 which already represents the
// annotation row directly. cursorOnAnnotation must NOT be set in that case
// (it applies only to line-level annotation sub-rows, not the file-level row).
func TestModel_HandleAnnotNav_FileLevelDoesNotSetCursorOnAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L1", OldNum: 0, NewNum: 1},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file-level"})

	m.nav.diffCursor = 0
	m.annot.cursorOnAnnotation = false
	result, _ = m.handleAnnotNav(false) // backward to file-level
	model := result.(Model)
	assert.Equal(t, -1, model.nav.diffCursor, "file-level position is diffCursor=-1")
	assert.False(t, model.annot.cursorOnAnnotation, "file-level annotation row uses diffCursor=-1, not the cursorOnAnnotation flag")
}

func TestModel_HandleAnnotNav_DefaultBindings(t *testing.T) {
	km := keymap.Default()
	assert.Equal(t, keymap.ActionNextAnnotation, km.Resolve("}"))
	assert.Equal(t, keymap.ActionPrevAnnotation, km.Resolve("{"))
}

// tryJumpToAnnotationTarget returns false on unreachable targets so the
// walker can skip past them. This test pins the contract for each rejection
// path: nil target, same-file line not in loaded diff, single-file mode
// cross-file (which has nowhere to go).
func TestModel_TryJumpToAnnotationTarget_RejectsNonJumpable(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "L5", OldNum: 0, NewNum: 5},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	loadMsg := m.loadFileDiff("a.go")()
	result, _ = m.Update(loadMsg)
	m = result.(Model)

	t.Run("nil target", func(t *testing.T) {
		_, _, ok := m.tryJumpToAnnotationTarget(nil)
		assert.False(t, ok)
	})

	t.Run("same-file line missing from loaded diff", func(t *testing.T) {
		// line 99 is not in m.file.lines
		_, _, ok := m.tryJumpToAnnotationTarget(&overlay.AnnotationTarget{
			File: "a.go", ChangeType: "+", Line: 99,
		})
		assert.False(t, ok, "line not in loaded diff must reject")
	})

	t.Run("file-level annotation always jumpable", func(t *testing.T) {
		_, _, ok := m.tryJumpToAnnotationTarget(&overlay.AnnotationTarget{
			File: "a.go", ChangeType: "", Line: 0,
		})
		assert.True(t, ok, "file-level (Line=0) target must be jumpable in current file")
	})

	t.Run("single-file mode rejects cross-file target", func(t *testing.T) {
		mSingle := m
		mSingle.file.singleFile = true
		_, _, ok := mSingle.tryJumpToAnnotationTarget(&overlay.AnnotationTarget{
			File: "other.go", ChangeType: "+", Line: 1,
		})
		assert.False(t, ok, "single-file mode must reject cross-file targets")
	})

	t.Run("same-file jumpable line returns ok", func(t *testing.T) {
		_, _, ok := m.tryJumpToAnnotationTarget(&overlay.AnnotationTarget{
			File: "a.go", ChangeType: "+", Line: 5,
		})
		assert.True(t, ok)
	})
}
