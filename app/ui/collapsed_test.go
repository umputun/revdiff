package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
)

func TestModel_VKeyTogglesCollapsedMode(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.focus = paneDiff
	m.viewport.Height = 20

	t.Run("toggle on", func(t *testing.T) {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.True(t, model.collapsed.enabled, "v should enable collapsed mode")
		assert.NotNil(t, model.collapsed.expandedHunks)
		assert.Empty(t, model.collapsed.expandedHunks, "expandedHunks should be reset on toggle")
	})

	t.Run("toggle off", func(t *testing.T) {
		m.collapsed.enabled = true
		m.collapsed.expandedHunks = map[int]bool{1: true}
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.False(t, model.collapsed.enabled, "v should disable collapsed mode")
		assert.Empty(t, model.collapsed.expandedHunks, "expandedHunks should be reset on toggle")
	})

	t.Run("no-op in tree pane", func(t *testing.T) {
		m.collapsed.enabled = false
		m.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.False(t, model.collapsed.enabled, "v should be no-op in tree pane")
	})

	t.Run("no-op when no file loaded", func(t *testing.T) {
		m.focus = paneDiff
		m.currFile = ""
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.False(t, model.collapsed.enabled, "v should be no-op when no file loaded")
	})
}

func TestModel_DotKeyExpandsHunkInCollapsedMode(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},   // 1 - hunk start
		{NewNum: 2, Content: "new", ChangeType: diff.ChangeAdd},      // 2
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},     // 4 - hunk 2 start
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.focus = paneDiff
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.viewport.Height = 20

	t.Run("expand hunk at cursor", func(t *testing.T) {
		m.diffCursor = 2 // on add line in hunk 1 (start=1)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.True(t, model.collapsed.expandedHunks[1], "hunk at index 1 should be expanded")
	})

	t.Run("collapse expanded hunk", func(t *testing.T) {
		m.collapsed.expandedHunks = map[int]bool{1: true}
		m.diffCursor = 1 // on remove line in hunk 1
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.False(t, model.collapsed.expandedHunks[1], "hunk should be collapsed after second dot")
	})

	t.Run("expand second hunk independently", func(t *testing.T) {
		m.collapsed.expandedHunks = map[int]bool{1: true}
		m.diffCursor = 4 // on add line in hunk 2 (start=4)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.True(t, model.collapsed.expandedHunks[4], "hunk 2 should be expanded")
		assert.True(t, model.collapsed.expandedHunks[1], "hunk 1 should remain expanded")
	})

	t.Run("no-op on context line", func(t *testing.T) {
		m.collapsed.expandedHunks = make(map[int]bool)
		m.diffCursor = 0 // on context line
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.Empty(t, model.collapsed.expandedHunks, "dot on context line should be no-op")
	})
}

func TestModel_DotKeyNoOpInExpandedMode(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.focus = paneDiff
	m.collapsed.enabled = false
	m.collapsed.expandedHunks = make(map[int]bool)
	m.diffCursor = 0
	m.viewport.Height = 20

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
	model := result.(Model)
	assert.Empty(t, model.collapsed.expandedHunks, "dot should be no-op in expanded mode")
}

func TestModel_FileSwitchResetsExpandedHunksPreservesCollapsed(t *testing.T) {
	linesA := []diff.DiffLine{
		{NewNum: 1, Content: "a-ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "a-add", ChangeType: diff.ChangeAdd},
	}
	linesB := []diff.DiffLine{
		{NewNum: 1, Content: "b-ctx", ChangeType: diff.ChangeContext},
	}
	fileDiffs := map[string][]diff.DiffLine{"a.go": linesA, "b.go": linesB}
	m := testModel([]string{"a.go", "b.go"}, fileDiffs)
	m.tree = newFileTree([]string{"a.go", "b.go"})

	// simulate loading first file
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: linesA})
	model := result.(Model)

	// set collapsed mode and expand a hunk
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = map[int]bool{1: true}

	// load second file
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: model.loadSeq, lines: linesB})
	model = result.(Model)

	assert.True(t, model.collapsed.enabled, "collapsed should persist across file switches")
	assert.Empty(t, model.collapsed.expandedHunks, "expandedHunks should be reset on file switch")
	assert.Equal(t, "b.go", model.currFile)
}

func TestModel_BuildModifiedSet(t *testing.T) {
	tests := []struct {
		name   string
		lines  []diff.DiffLine
		expect map[int]bool
	}{
		{name: "empty lines", lines: nil, expect: map[int]bool{}},
		{name: "all context", lines: []diff.DiffLine{
			{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "b", ChangeType: diff.ChangeContext},
		}, expect: map[int]bool{}},
		{name: "pure adds only", lines: []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "new2", ChangeType: diff.ChangeAdd},
			{NewNum: 4, Content: "ctx", ChangeType: diff.ChangeContext},
		}, expect: map[int]bool{}},
		{name: "pure removes only", lines: []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "ctx", ChangeType: diff.ChangeContext},
		}, expect: map[int]bool{}},
		{name: "mixed hunk marks adds as modified", lines: []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx", ChangeType: diff.ChangeContext},
		}, expect: map[int]bool{2: true}},
		{name: "mixed hunk multiple adds", lines: []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "new2", ChangeType: diff.ChangeAdd},
			{NewNum: 4, Content: "new3", ChangeType: diff.ChangeAdd},
			{NewNum: 5, Content: "ctx", ChangeType: diff.ChangeContext},
		}, expect: map[int]bool{3: true, 4: true, 5: true}},
		{name: "two hunks one mixed one pure add", lines: []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},  // 1
			{NewNum: 2, Content: "new", ChangeType: diff.ChangeAdd},     // 2 - modified
			{NewNum: 3, Content: "ctx", ChangeType: diff.ChangeContext}, // 3
			{NewNum: 4, Content: "added", ChangeType: diff.ChangeAdd},   // 4 - pure add
			{NewNum: 5, Content: "ctx", ChangeType: diff.ChangeContext}, // 5
		}, expect: map[int]bool{2: true}},
		{name: "two hunks both mixed", lines: []diff.DiffLine{
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},  // 0
			{NewNum: 1, Content: "new1", ChangeType: diff.ChangeAdd},     // 1 - modified
			{NewNum: 2, Content: "ctx", ChangeType: diff.ChangeContext},  // 2
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},  // 3
			{NewNum: 3, Content: "new2", ChangeType: diff.ChangeAdd},     // 4 - modified
			{NewNum: 4, Content: "ctx2", ChangeType: diff.ChangeContext}, // 5
		}, expect: map[int]bool{1: true, 4: true}},
		{name: "hunks separated by divider", lines: []diff.DiffLine{
			{OldNum: 1, Content: "old", ChangeType: diff.ChangeRemove}, // 0 - hunk 1
			{NewNum: 1, Content: "new", ChangeType: diff.ChangeAdd},    // 1 - modified
			{Content: "...", ChangeType: diff.ChangeDivider},           // 2
			{NewNum: 10, Content: "added", ChangeType: diff.ChangeAdd}, // 3 - pure add (hunk 2)
		}, expect: map[int]bool{1: true}},
		{name: "trailing context lines excluded from modified set", lines: []diff.DiffLine{
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},  // 0
			{NewNum: 1, Content: "new1", ChangeType: diff.ChangeAdd},     // 1 - modified
			{NewNum: 2, Content: "ctx1", ChangeType: diff.ChangeContext}, // 2 - trailing context
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3 - trailing context
			{NewNum: 4, Content: "ctx3", ChangeType: diff.ChangeContext}, // 4 - trailing context
			{OldNum: 5, Content: "old2", ChangeType: diff.ChangeRemove},  // 5
			{NewNum: 5, Content: "new2", ChangeType: diff.ChangeAdd},     // 6 - modified
			{NewNum: 6, Content: "ctx4", ChangeType: diff.ChangeContext}, // 7 - trailing context
		}, expect: map[int]bool{1: true, 6: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.diffLines = tc.lines
			assert.Equal(t, tc.expect, m.buildModifiedSet(m.findHunks()))
		})
	}
}

func TestModel_CollapsedDeleteOnlyPlaceholderBlocksAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m.diffCursor = 1 // on placeholder

	cmd := m.startAnnotation()
	assert.Nil(t, cmd, "should not allow annotating delete-only placeholder")
	assert.False(t, m.annotating, "annotating mode should not be active")
}

func TestModel_IsDeleteOnlyPlaceholder(t *testing.T) {
	m := testModel(nil, nil)
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove}, // idx 1
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove}, // idx 2
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	hunks := m.findHunks()

	assert.True(t, m.isDeleteOnlyPlaceholder(1, hunks), "first line of delete-only hunk should be placeholder")
	assert.False(t, m.isDeleteOnlyPlaceholder(2, hunks), "second line of delete-only hunk is not placeholder")
	assert.False(t, m.isDeleteOnlyPlaceholder(0, hunks), "context line is not placeholder")

	// expanded hunk is not a placeholder
	m.collapsed.expandedHunks[1] = true
	assert.False(t, m.isDeleteOnlyPlaceholder(1, hunks), "expanded hunk should not be placeholder")

	// not collapsed mode
	m.collapsed.enabled = false
	m.collapsed.expandedHunks = make(map[int]bool)
	assert.False(t, m.isDeleteOnlyPlaceholder(1, hunks), "should return false when not in collapsed mode")
}

func TestModel_CollapsedCursorMovementIncludesDeleteOnlyPlaceholder(t *testing.T) {
	m := testModel(nil, nil)
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},  // 1 - placeholder
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},  // 2 - hidden
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
	}
	m.diffCursor = 0

	// move down should land on placeholder (idx 1), not skip to ctx2 (idx 3)
	m.moveDiffCursorDown()
	assert.Equal(t, 1, m.diffCursor, "should land on delete-only hunk placeholder")

	// move down again should skip hidden idx 2 and land on ctx2 (idx 3)
	m.moveDiffCursorDown()
	assert.Equal(t, 3, m.diffCursor, "should skip hidden remove and land on context")

	// move up should go back to placeholder
	m.moveDiffCursorUp()
	assert.Equal(t, 1, m.diffCursor, "should go back to placeholder")
}

func TestModel_IsDeleteOnlyHunk(t *testing.T) {
	m := testModel(nil, nil)

	t.Run("delete-only hunk", func(t *testing.T) {
		m.diffLines = []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "ctx", ChangeType: diff.ChangeContext},
		}
		hunks := m.findHunks()
		assert.True(t, m.isDeleteOnlyHunk(hunks[0]))
	})

	t.Run("mixed hunk", func(t *testing.T) {
		m.diffLines = []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "del", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx", ChangeType: diff.ChangeContext},
		}
		hunks := m.findHunks()
		assert.False(t, m.isDeleteOnlyHunk(hunks[0]))
	})

	t.Run("add-only hunk", func(t *testing.T) {
		m.diffLines = []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx", ChangeType: diff.ChangeContext},
		}
		hunks := m.findHunks()
		assert.False(t, m.isDeleteOnlyHunk(hunks[0]))
	})
}

func TestModel_HunkStartFor(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},   // 1
		{NewNum: 2, Content: "new", ChangeType: diff.ChangeAdd},      // 2
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
		{NewNum: 4, Content: "added", ChangeType: diff.ChangeAdd},    // 4
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext}, // 5
	}
	hunks := m.findHunks() // should be [1, 4]
	assert.Equal(t, []int{1, 4}, hunks)

	// context line returns -1
	assert.Equal(t, -1, m.hunkStartFor(0, hunks))
	// first hunk lines
	assert.Equal(t, 1, m.hunkStartFor(1, hunks))
	assert.Equal(t, 1, m.hunkStartFor(2, hunks))
	// context between hunks
	assert.Equal(t, -1, m.hunkStartFor(3, hunks))
	// second hunk
	assert.Equal(t, 4, m.hunkStartFor(4, hunks))
	// trailing context
	assert.Equal(t, -1, m.hunkStartFor(5, hunks))
	// out of bounds
	assert.Equal(t, -1, m.hunkStartFor(-1, hunks))
	assert.Equal(t, -1, m.hunkStartFor(10, hunks))
	// empty hunks
	assert.Equal(t, -1, m.hunkStartFor(0, nil))
}

func TestModel_CollapsedCursorDownSkipsRemovedLines(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	model.collapsed.enabled = true
	assert.Equal(t, 0, model.diffCursor, "starts on ctx1")

	// move down should skip removed lines (indices 1,2) and land on add line (index 3)
	model.moveDiffCursorDown()
	assert.Equal(t, 3, model.diffCursor, "should skip removed lines and land on add line")

	// move down again lands on ctx2
	model.moveDiffCursorDown()
	assert.Equal(t, 4, model.diffCursor, "should land on ctx2")
}

func TestModel_CollapsedCursorUpSkipsRemovedLines(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	model.collapsed.enabled = true
	model.diffCursor = 4 // start on ctx2

	// move up should skip removed lines (indices 2,1) and land on add line (index 3)
	model.moveDiffCursorUp()
	assert.Equal(t, 3, model.diffCursor, "should land on add line")

	// move up again skips removed lines and lands on ctx1
	model.moveDiffCursorUp()
	assert.Equal(t, 0, model.diffCursor, "should skip removed lines and land on ctx1")
}

func TestModel_CollapsedCursorMovementInExpandedHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = map[int]bool{1: true} // expand the hunk starting at index 1

	// cursor on ctx1, move down should land on removed line since hunk is expanded
	model.moveDiffCursorDown()
	assert.Equal(t, 1, model.diffCursor, "should land on removed line in expanded hunk")

	// move down lands on add line
	model.moveDiffCursorDown()
	assert.Equal(t, 2, model.diffCursor, "should land on add line")

	// move down lands on ctx2
	model.moveDiffCursorDown()
	assert.Equal(t, 3, model.diffCursor, "should land on ctx2")

	// now move up through the expanded hunk
	model.moveDiffCursorUp()
	assert.Equal(t, 2, model.diffCursor, "should land on add line")

	model.moveDiffCursorUp()
	assert.Equal(t, 1, model.diffCursor, "should land on removed line in expanded hunk")

	model.moveDiffCursorUp()
	assert.Equal(t, 0, model.diffCursor, "should land on ctx1")
}

func TestModel_CollapsedSkipInitialDividers(t *testing.T) {
	t.Run("skips divider and removed lines", func(t *testing.T) {
		lines := []diff.DiffLine{
			{Content: "@@...", ChangeType: diff.ChangeDivider},
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},
			{NewNum: 1, Content: "new1", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "ctx1", ChangeType: diff.ChangeContext},
		}
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.collapsed.enabled = true
		m.diffLines = lines
		m.skipInitialDividers()
		assert.Equal(t, 2, m.diffCursor, "should skip divider and removed line, land on add")
	})

	t.Run("expanded mode skips only dividers", func(t *testing.T) {
		lines := []diff.DiffLine{
			{Content: "@@...", ChangeType: diff.ChangeDivider},
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},
			{NewNum: 1, Content: "new1", ChangeType: diff.ChangeAdd},
		}
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.diffLines = lines
		m.skipInitialDividers()
		assert.Equal(t, 1, m.diffCursor, "expanded mode should land on removed line after divider")
	})

	t.Run("collapsed with expanded hunk allows removed lines", func(t *testing.T) {
		lines := []diff.DiffLine{
			{Content: "@@...", ChangeType: diff.ChangeDivider},
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},
			{NewNum: 1, Content: "new1", ChangeType: diff.ChangeAdd},
		}
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.collapsed.enabled = true
		m.collapsed.expandedHunks = map[int]bool{1: true} // hunk starts at index 1
		m.diffLines = lines
		m.skipInitialDividers()
		assert.Equal(t, 1, m.diffCursor, "expanded hunk should allow landing on removed line")
	})
}

func TestModel_CollapsedCursorDownMultipleHunks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove}, // hunk 1 at idx 1
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{OldNum: 4, Content: "old2", ChangeType: diff.ChangeRemove}, // hunk 2 at idx 4
		{OldNum: 5, Content: "old3", ChangeType: diff.ChangeRemove},
		{NewNum: 4, Content: "new2", ChangeType: diff.ChangeAdd},
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	model.collapsed.enabled = true

	// traverse all lines with cursor down
	positions := []int{model.diffCursor}
	for range 10 {
		prev := model.diffCursor
		model.moveDiffCursorDown()
		if model.diffCursor == prev {
			break
		}
		positions = append(positions, model.diffCursor)
	}
	// should visit: ctx1(0), new1(2), ctx2(3), new2(6), ctx3(7)
	assert.Equal(t, []int{0, 2, 3, 6, 7}, positions, "cursor should skip all removed lines across hunks")
}

func TestModel_CollapsedPageDownSkipsRemovedLines(t *testing.T) {
	// create enough lines so page movement is meaningful
	var lines []diff.DiffLine
	for i := 1; i <= 50; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
		// add a remove+add hunk every 5 lines
		if i%5 == 0 {
			lines = append(lines,
				diff.DiffLine{OldNum: i + 100, Content: "old", ChangeType: diff.ChangeRemove},
				diff.DiffLine{NewNum: i + 1, Content: "new", ChangeType: diff.ChangeAdd},
			)
		}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.collapsed.enabled = true

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight)

	startCursor := model.diffCursor
	startY := model.cursorViewportY()

	// page down
	model.moveDiffCursorPageDown()

	assert.Greater(t, model.diffCursor, startCursor, "cursor should advance")
	assert.GreaterOrEqual(t, model.cursorViewportY()-startY, pageHeight, "should move at least one page")

	// verify cursor did not land on a hidden removed line
	dl := model.diffLines[model.diffCursor]
	assert.NotEqual(t, diff.ChangeRemove, dl.ChangeType, "cursor should not land on hidden removed line")
}

func TestModel_CollapsedPageUpSkipsRemovedLines(t *testing.T) {
	var lines []diff.DiffLine
	for i := 1; i <= 50; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
		if i%5 == 0 {
			lines = append(lines,
				diff.DiffLine{OldNum: i + 100, Content: "old", ChangeType: diff.ChangeRemove},
				diff.DiffLine{NewNum: i + 1, Content: "new", ChangeType: diff.ChangeAdd},
			)
		}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.collapsed.enabled = true

	// move cursor to near the end
	model.diffCursor = len(lines) - 1
	startY := model.cursorViewportY()

	// page up
	model.moveDiffCursorPageUp()

	assert.Less(t, model.diffCursor, len(lines)-1, "cursor should move back")
	assert.GreaterOrEqual(t, startY-model.cursorViewportY(), model.viewport.Height, "should move at least one page up")

	// verify cursor did not land on a hidden removed line
	dl := model.diffLines[model.diffCursor]
	assert.NotEqual(t, diff.ChangeRemove, dl.ChangeType, "cursor should not land on hidden removed line")
}

func TestModel_CollapsedCursorToEndSkipsRemovedLines(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove}, // last lines are removes
		{OldNum: 4, Content: "old3", ChangeType: diff.ChangeRemove},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.diffCursor = 0

	m.moveDiffCursorToEnd()
	assert.Equal(t, 2, m.diffCursor, "should land on add line, not hidden removed lines")
}

func TestModel_CollapsedHunkNavigationSkipsRemovedLines(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // 1 - hunk 1 start (remove)
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},  // 2
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},     // 3 - first visible in hunk 1
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 4
		{OldNum: 5, Content: "old3", ChangeType: diff.ChangeRemove},  // 5 - hunk 2 start (remove)
		{NewNum: 4, Content: "new2", ChangeType: diff.ChangeAdd},     // 6 - first visible in hunk 2
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext}, // 7
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.diffCursor = 0
	m.viewport.Height = 20

	// next hunk should skip hidden removes and land on add line
	m.moveToNextHunk()
	assert.Equal(t, 3, m.diffCursor, "should land on first visible line in hunk 1")

	m.moveToNextHunk()
	assert.Equal(t, 6, m.diffCursor, "should land on first visible line in hunk 2")

	// prev hunk back
	m.moveToPrevHunk()
	assert.Equal(t, 3, m.diffCursor, "should land on first visible line in hunk 1")
}

func TestModel_CollapsedHunkNavigationExpandedHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // 1 - hunk 1 start
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},     // 2
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = map[int]bool{1: true} // hunk at index 1 is expanded
	m.diffCursor = 0
	m.viewport.Height = 20

	// expanded hunk: should land on hunk start (remove line is visible)
	m.moveToNextHunk()
	assert.Equal(t, 1, m.diffCursor, "expanded hunk should land on remove line")
}

func TestModel_CollapsedHunkNavigationDeleteOnly(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},  // 1 - hunk 1 (delete-only)
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},  // 2
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
		{OldNum: 5, Content: "old3", ChangeType: diff.ChangeRemove},  // 4 - hunk 2 (mixed)
		{NewNum: 3, Content: "new3", ChangeType: diff.ChangeAdd},     // 5
		{NewNum: 4, Content: "ctx3", ChangeType: diff.ChangeContext}, // 6
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.diffCursor = 0
	m.viewport.Height = 20

	// next hunk lands on delete-only hunk 1's placeholder (first remove line)
	m.moveToNextHunk()
	assert.Equal(t, 1, m.diffCursor, "should land on delete-only hunk placeholder")

	// next hunk from hunk 1 lands on hunk 2's visible add line
	m.moveToNextHunk()
	assert.Equal(t, 5, m.diffCursor, "should land on mixed hunk's add line")

	// prev hunk from hunk 2 goes back to delete-only hunk 1's placeholder
	m.moveToPrevHunk()
	assert.Equal(t, 1, m.diffCursor, "should go back to delete-only hunk placeholder")
}

func TestModel_FirstVisibleInHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	hunks := m.findHunks() // [1]

	// expanded mode: returns start unchanged
	m.collapsed.enabled = false
	assert.Equal(t, 1, m.firstVisibleInHunk(1, hunks))

	// collapsed mode: skips hidden removes, lands on add
	m.collapsed.enabled = true
	assert.Equal(t, 3, m.firstVisibleInHunk(1, hunks))

	// collapsed mode with expanded hunk: returns start
	m.collapsed.expandedHunks = map[int]bool{1: true}
	assert.Equal(t, 1, m.firstVisibleInHunk(1, hunks))
}

func TestModel_FirstVisibleInHunk_AllRemoves(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // idx 1
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},  // idx 2
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext}, // idx 3
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.collapsed.enabled = true
	hunks := m.findHunks() // [1]

	// all-removes hunk: placeholder line is visible, returns hunkStart
	assert.Equal(t, 1, m.firstVisibleInHunk(1, hunks))

	// expanded hunk: also returns hunkStart
	m.collapsed.expandedHunks = map[int]bool{1: true}
	assert.Equal(t, 1, m.firstVisibleInHunk(1, hunks))
}

func TestModel_AdjustCursorIfHidden(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // idx 0
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // idx 1 - hidden
		{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},  // idx 2 - hidden
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},     // idx 3
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // idx 4
	}

	t.Run("cursor on hidden line moves forward", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = lines
		m.collapsed.enabled = true
		m.diffCursor = 1 // on hidden removed line
		m.adjustCursorIfHidden()
		assert.Equal(t, 3, m.diffCursor, "should move forward to add line")
	})

	t.Run("cursor on visible line stays put", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = lines
		m.collapsed.enabled = true
		m.diffCursor = 0 // on context line
		m.adjustCursorIfHidden()
		assert.Equal(t, 0, m.diffCursor, "should stay on context line")
	})

	t.Run("not collapsed mode is no-op", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = lines
		m.collapsed.enabled = false
		m.diffCursor = 1
		m.adjustCursorIfHidden()
		assert.Equal(t, 1, m.diffCursor, "should not adjust in expanded mode")
	})

	t.Run("cursor on hidden line moves backward to placeholder", func(t *testing.T) {
		onlyRemoves := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // idx 0
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // idx 1 - placeholder (visible)
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},  // idx 2 - hidden
		}
		m := testModel(nil, nil)
		m.diffLines = onlyRemoves
		m.collapsed.enabled = true
		m.diffCursor = 2 // on hidden removed line (not placeholder)
		m.adjustCursorIfHidden()
		assert.Equal(t, 1, m.diffCursor, "should move backward to delete-only hunk placeholder")
	})

	t.Run("cursor on delete-only hunk placeholder stays put", func(t *testing.T) {
		// cursor on delete-only hunk's first line (placeholder) is already visible
		deleteOnly := []diff.DiffLine{
			{Content: "...", ChangeType: diff.ChangeDivider},            // idx 0 - divider
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove}, // idx 1 - placeholder (visible)
			{OldNum: 2, Content: "old2", ChangeType: diff.ChangeRemove}, // idx 2 - hidden
			{OldNum: 3, Content: "old3", ChangeType: diff.ChangeRemove}, // idx 3 - hidden
		}
		m := testModel(nil, nil)
		m.diffLines = deleteOnly
		m.collapsed.enabled = true
		m.diffCursor = 1 // on placeholder (not hidden)
		m.adjustCursorIfHidden()
		assert.Equal(t, 1, m.diffCursor, "placeholder line is visible, cursor should stay")
	})

	t.Run("single hunk all removes placeholder at start", func(t *testing.T) {
		// real single-hunk deleted file: first line is the visible placeholder
		allRemoves := []diff.DiffLine{
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove}, // idx 0 - placeholder (visible)
			{OldNum: 2, Content: "old2", ChangeType: diff.ChangeRemove}, // idx 1 - hidden
			{OldNum: 3, Content: "old3", ChangeType: diff.ChangeRemove}, // idx 2 - hidden
		}
		m := testModel(nil, nil)
		m.diffLines = allRemoves
		m.collapsed.enabled = true
		m.diffCursor = 0 // on placeholder, not hidden
		m.adjustCursorIfHidden()
		assert.Equal(t, 0, m.diffCursor, "placeholder is visible, cursor stays")
	})
}

func TestModel_ToggleCollapsedModeAdjustsCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1 // on removed line

	// toggle to collapsed mode
	m.toggleCollapsedMode()
	assert.True(t, m.collapsed.enabled)
	assert.Equal(t, 2, m.diffCursor, "cursor should move to add line, not stay on hidden removed line")
}

func TestModel_ToggleHunkExpansionAdjustsCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove}, // idx 1
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},    // idx 2
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = map[int]bool{1: true} // hunk expanded
	m.diffCursor = 1                                  // on removed line (visible because expanded)

	// collapse the hunk - cursor on removed line should move
	m.toggleHunkExpansion()
	assert.False(t, m.collapsed.expandedHunks[1], "hunk should be collapsed")
	assert.Equal(t, 2, m.diffCursor, "cursor should move to add line after hunk collapse")
}

func TestModel_CollapsedCursorDownSkipsPlaceholderAnnotation(t *testing.T) {
	// cursor moving down through a delete-only placeholder with an annotation should NOT
	// stop on the invisible annotation sub-line
	m := testModel(nil, nil)
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},  // 1 - placeholder
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},  // 2 - hidden
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "hidden note"})
	m.diffCursor = 0
	m.focus = paneDiff

	// move down lands on placeholder (idx 1)
	m.moveDiffCursorDown()
	assert.Equal(t, 1, m.diffCursor)
	assert.False(t, m.cursorOnAnnotation, "should not stop on invisible annotation of placeholder")

	// move down again goes to ctx2 (idx 3), skipping the annotation
	m.moveDiffCursorDown()
	assert.Equal(t, 3, m.diffCursor)
	assert.False(t, m.cursorOnAnnotation)
}

func TestModel_CollapsedCursorUpSkipsPlaceholderAnnotation(t *testing.T) {
	// cursor moving up onto a delete-only placeholder with an annotation should NOT
	// land on the annotation sub-line
	m := testModel(nil, nil)
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},  // 1 - placeholder
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},  // 2 - hidden
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "hidden note"})
	m.diffCursor = 3
	m.focus = paneDiff

	// move up should land on placeholder (idx 1), NOT on its annotation
	m.moveDiffCursorUp()
	assert.Equal(t, 1, m.diffCursor)
	assert.False(t, m.cursorOnAnnotation, "should not land on invisible annotation of placeholder")
}

func TestModel_CollapsedToggleClearsAnnotationState(t *testing.T) {
	// toggling collapsed mode should clear cursorOnAnnotation
	m := testModel(nil, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "some note"})
	m.diffCursor = 1
	m.cursorOnAnnotation = true // simulating cursor on annotation in expanded mode

	m.toggleCollapsedMode()
	assert.True(t, m.collapsed.enabled)
	assert.False(t, m.cursorOnAnnotation, "cursorOnAnnotation should be cleared when toggling mode")
}

func TestModel_CollapsedHunkCollapseClearsAnnotationState(t *testing.T) {
	// collapsing a hunk should clear cursorOnAnnotation for annotations on removed lines
	m := testModel(nil, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "note"})
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = map[int]bool{1: true}
	m.diffCursor = 1
	m.cursorOnAnnotation = true // on annotation of expanded remove line

	m.toggleHunkExpansion()
	assert.False(t, m.cursorOnAnnotation, "cursorOnAnnotation should be cleared when hunk collapses")
}

func TestModel_CollapsedDeleteAnnotationBlockedOnPlaceholder(t *testing.T) {
	// pressing 'd' on a delete-only placeholder should not delete the invisible annotation
	m := testModel(nil, nil)
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "keep this"})
	m.diffCursor = 1
	m.focus = paneDiff

	// cursor should not be on annotation (placeholder)
	assert.False(t, m.cursorOnAnnotation)

	// attempt delete - should be no-op since cursorOnAnnotation is false
	m.deleteAnnotation()
	assert.True(t, m.store.Has("a.go", 2, "-"), "annotation should not be deleted from placeholder")
}
