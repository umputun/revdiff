package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/overlay"
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

func TestModel_BuildAnnotListSpec(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	spec := m.buildAnnotListSpec()
	require.Len(t, spec.Items, 2)
	assert.Equal(t, "a.go", spec.Items[0].File)
	assert.Equal(t, 0, spec.Items[0].Line)
	assert.Equal(t, "a.go", spec.Items[1].File)
	assert.Equal(t, 5, spec.Items[1].Line)
	assert.Equal(t, "+", spec.Items[1].ChangeType)
}

func TestModel_AnnotListOpenClose(t *testing.T) {
	t.Run("@ opens annotation list overlay", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "note"})

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
		model := result.(Model)
		assert.True(t, model.overlay.Active())
		assert.Equal(t, overlay.KindAnnotList, model.overlay.Kind())
	})

	t.Run("@ closes annotation list when already open", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.overlay.OpenAnnotList(m.buildAnnotListSpec())

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
		model := result.(Model)
		assert.False(t, model.overlay.Active())
	})

	t.Run("esc closes annotation list", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.overlay.OpenAnnotList(m.buildAnnotListSpec())

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model := result.(Model)
		assert.False(t, model.overlay.Active())
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
	m.file.lines = diffs["a.go"]

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

func TestModel_JumpToAnnotationTarget_SameFile(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1},
			{ChangeType: diff.ChangeAdd, Content: "new", OldNum: 0, NewNum: 2},
			{ChangeType: diff.ChangeContext, Content: "line3", OldNum: 2, NewNum: 3},
		},
	}
	m := testModel([]string{"a.go"}, diffs)

	// simulate file load
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	t.Run("same-file jump positions cursor", func(t *testing.T) {
		target := &overlay.AnnotationTarget{File: "a.go", ChangeType: "+", Line: 2}
		result, _ := m.jumpToAnnotationTarget(target)
		model := result.(Model)
		assert.Equal(t, 1, model.nav.diffCursor)
		assert.Equal(t, paneDiff, model.layout.focus)
	})

	t.Run("file-level annotation sets cursor to -1", func(t *testing.T) {
		target := &overlay.AnnotationTarget{File: "a.go", ChangeType: "", Line: 0}
		result, _ := m.jumpToAnnotationTarget(target)
		model := result.(Model)
		assert.Equal(t, -1, model.nav.diffCursor)
	})

	t.Run("nil target is no-op", func(t *testing.T) {
		result, _ := m.jumpToAnnotationTarget(nil)
		model := result.(Model)
		assert.Equal(t, m.nav.diffCursor, model.nav.diffCursor)
	})
}

func TestModel_JumpToAnnotationTarget_CrossFile(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
		"b.go": {
			{ChangeType: diff.ChangeContext, Content: "ctx", OldNum: 1, NewNum: 1},
			{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 2},
		},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)

	// load files and first file
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}, diff.FileEntry{Path: "b.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)
	assert.Equal(t, "a.go", m.file.name)

	t.Run("cross-file jump sets pending and triggers load", func(t *testing.T) {
		target := &overlay.AnnotationTarget{File: "b.go", ChangeType: "+", Line: 2}
		result, cmd := m.jumpToAnnotationTarget(target)
		model := result.(Model)
		assert.NotNil(t, model.pendingAnnotJump)
		assert.Equal(t, "b.go", model.pendingAnnotJump.File)
		require.NotNil(t, cmd)

		// simulate file loaded
		loadMsg := cmd()
		result, _ = model.Update(loadMsg)
		model = result.(Model)
		assert.Equal(t, "b.go", model.file.name)
		assert.Nil(t, model.pendingAnnotJump)
		assert.Equal(t, 1, model.nav.diffCursor)
		assert.Equal(t, paneDiff, model.layout.focus)
	})
}

func TestModel_JumpToAnnotation_StalePendingGuard(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "line1", OldNum: 1, NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "added", OldNum: 0, NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)

	// load files and first file
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}, diff.FileEntry{Path: "b.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	t.Run("stale pending jump is ignored when file does not match", func(t *testing.T) {
		// set pending for b.go but simulate a.go being loaded
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		m.file.loadSeq++
		loadMsg := fileLoadedMsg{file: "a.go", seq: m.file.loadSeq, lines: diffs["a.go"]}
		result, _ := m.Update(loadMsg)
		model := result.(Model)
		// pending should not be cleared (file mismatch)
		assert.NotNil(t, model.pendingAnnotJump)
		assert.Equal(t, "b.go", model.pendingAnnotJump.File)
	})

	t.Run("n key clears pending jump", func(t *testing.T) {
		m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
		m.layout.focus = paneDiff
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
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}, diff.FileEntry{Path: "b.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)
	m.layout.focus = paneTree

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
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}, diff.FileEntry{Path: "b.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// add annotation so filter has something to toggle
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+"}
	m.layout.focus = paneDiff
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
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// enable collapsed mode - remove line at index 2 should be hidden
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)

	a := annotation.Annotation{File: "a.go", Line: 2, Type: "-"}
	m.positionOnAnnotation(a)

	// hunk should be expanded so the remove line is visible
	assert.Equal(t, 2, m.nav.diffCursor)
	hunks := m.findHunks()
	assert.False(t, m.isCollapsedHidden(m.nav.diffCursor, hunks), "target line should be visible after jump")
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
	result, _ := m.Update(testFilesLoadedMsg(diff.FileEntry{Path: "a.go"}))
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// enable collapsed mode - first remove line is a delete-only placeholder
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)

	a := annotation.Annotation{File: "a.go", Line: 1, Type: "-"}
	m.positionOnAnnotation(a)

	// hunk must be expanded so the actual line and annotation are visible
	assert.Equal(t, 1, m.nav.diffCursor)
	hunks := m.findHunks()
	hunkStart := m.hunkStartFor(m.nav.diffCursor, hunks)
	assert.True(t, m.modes.collapsed.expandedHunks[hunkStart], "delete-only hunk should be expanded after jump")
	assert.False(t, m.isDeleteOnlyPlaceholder(m.nav.diffCursor, hunks), "line should not be a placeholder after expansion")
}
