package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

func TestModel_NextPrevFile(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
		"c.go": {{NewNum: 1, Content: "c", ChangeType: diff.ChangeContext}},
	})
	m.tree = testNewFileTree(files)
	m.file.name = "a.go" // pretend first file is already loaded

	// starts on first file (a.go)
	assert.Equal(t, "a.go", m.tree.SelectedFile())

	// press n - should move to b.go
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile())
	assert.NotNil(t, cmd) // triggers file load

	// press n - should move to c.go
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, "c.go", model.tree.SelectedFile())
	assert.NotNil(t, cmd)

	// press p - back to b.go
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile())
	assert.NotNil(t, cmd)
}
func TestModel_DiffScrolling(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff

	// load file
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor)

	// j moves cursor down
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor)

	// k moves cursor back up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor)
}
func TestModel_DiffCursorSkipsDividers(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor)

	// j should skip divider and land on line10
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, 2, model.nav.diffCursor)

	// k should skip divider and go back to line1
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor)
}
func TestModel_DiffCursorAutoScrolls(t *testing.T) {
	// create more lines than viewport height
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	// move cursor past viewport height - viewport should auto-scroll
	for range 50 {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
	}
	assert.Equal(t, 50, model.nav.diffCursor)
	assert.Positive(t, model.layout.viewport.YOffset, "viewport should have scrolled")
}
func TestModel_CursorDiffLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.file.lines = lines
	m.nav.diffCursor = 0

	dl, ok := m.cursorDiffLine()
	assert.True(t, ok)
	assert.Equal(t, "line1", dl.Content)
	assert.Equal(t, diff.ChangeContext, dl.ChangeType)

	m.nav.diffCursor = 1
	dl, ok = m.cursorDiffLine()
	assert.True(t, ok)
	assert.Equal(t, "added", dl.Content)
	assert.Equal(t, diff.ChangeAdd, dl.ChangeType)

	// out of bounds
	m.nav.diffCursor = -1
	_, ok = m.cursorDiffLine()
	assert.False(t, ok)

	m.nav.diffCursor = 10
	_, ok = m.cursorDiffLine()
	assert.False(t, ok)
}
func TestModel_HunkEndLine(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added2", ChangeType: diff.ChangeAdd},
		{OldNum: 3, NewNum: 4, Content: "ctx", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.file.lines = lines

	t.Run("remove line stops at same type boundary", func(t *testing.T) {
		assert.Equal(t, 2, m.hunkEndLine(1), "single remove stays in old-file number space")
	})
	t.Run("add line walks through consecutive adds", func(t *testing.T) {
		assert.Equal(t, 3, m.hunkEndLine(2), "two adds, last NewNum=3")
	})
	t.Run("returns last line from last change line", func(t *testing.T) {
		assert.Equal(t, 3, m.hunkEndLine(3))
	})
	t.Run("returns 0 for context line", func(t *testing.T) {
		assert.Equal(t, 0, m.hunkEndLine(0))
	})
	t.Run("returns 0 for out of bounds", func(t *testing.T) {
		assert.Equal(t, 0, m.hunkEndLine(-1))
		assert.Equal(t, 0, m.hunkEndLine(99))
	})

	t.Run("mixed hunk removes followed by adds", func(t *testing.T) {
		// simulates a real diff where 3 lines are removed and 2 added
		mixedLines := []diff.DiffLine{
			{OldNum: 10, NewNum: 10, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, Content: "old line 1", ChangeType: diff.ChangeRemove},
			{OldNum: 12, Content: "old line 2", ChangeType: diff.ChangeRemove},
			{OldNum: 13, Content: "old line 3", ChangeType: diff.ChangeRemove},
			{NewNum: 11, Content: "new line 1", ChangeType: diff.ChangeAdd},
			{NewNum: 12, Content: "new line 2", ChangeType: diff.ChangeAdd},
			{OldNum: 14, NewNum: 13, Content: "ctx", ChangeType: diff.ChangeContext},
		}
		m.file.lines = mixedLines

		// cursor on first remove: end should be last remove (OldNum=13), not any add line
		assert.Equal(t, 13, m.hunkEndLine(1), "remove hunk end stays in old-file space")
		// cursor on middle remove
		assert.Equal(t, 13, m.hunkEndLine(2), "middle remove walks to last remove")
		// cursor on last remove
		assert.Equal(t, 13, m.hunkEndLine(3), "last remove returns own OldNum")

		// cursor on first add: end should be last add (NewNum=12), not a remove
		assert.Equal(t, 12, m.hunkEndLine(4), "add hunk end stays in new-file space")
		// cursor on last add
		assert.Equal(t, 12, m.hunkEndLine(5), "last add returns own NewNum")
	})
}
func TestModel_CursorViewportY(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
	}

	// no annotations, cursor at 0 -> viewport Y = 0
	m.nav.diffCursor = 0
	assert.Equal(t, 0, m.cursorViewportY())

	// no annotations, cursor at 2 -> viewport Y = 2
	m.nav.diffCursor = 2
	assert.Equal(t, 2, m.cursorViewportY())

	// add annotation on line 1 (index 0), cursor at 2 -> viewport Y = 3 (line0 + annotation + line1)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "comment"})
	m.nav.diffCursor = 2
	assert.Equal(t, 3, m.cursorViewportY())

	// empty file with non-zero cursor returns cursor value directly
	empty := testModel(nil, nil)
	empty.nav.diffCursor = 5
	assert.Equal(t, 5, empty.cursorViewportY(), "empty state returns diffCursor as-is")
}
func TestModel_NextPrevFileWrapAround(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
	})
	m.tree = testNewFileTree(files)
	m.file.name = "a.go"

	// move to last file
	m.tree.StepFile(sidepane.DirectionNext)
	assert.Equal(t, "b.go", m.tree.SelectedFile())
	m.file.name = "b.go"

	// n should wrap to first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(Model)
	assert.Equal(t, "a.go", model.tree.SelectedFile())

	// p should wrap to last
	model.file.name = "a.go"
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile())
}
func TestModel_PgDownMovesCursorByPageHeight(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize so Height is set
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff
	assert.Equal(t, 0, model.nav.diffCursor)

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight, "viewport height must be positive")

	// pgdown should move cursor by page height
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, pageHeight, model.nav.diffCursor, "PgDown should move cursor by viewport height")

	// ctrl+d should move by half page height from current position
	prevCursor := model.nav.diffCursor
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = result.(Model)
	halfPage := pageHeight / 2
	assert.Equal(t, prevCursor+halfPage, model.nav.diffCursor, "ctrl+d should move cursor by half viewport height")
}
func TestModel_PgUpMovesCursorByPageHeight(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize so Height is set
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight, "viewport height must be positive")

	// move cursor to line 80 first
	model.nav.diffCursor = 80

	// pgup should move cursor up by page height
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)
	assert.Equal(t, 80-pageHeight, model.nav.diffCursor, "PgUp should move cursor up by viewport height")

	// ctrl+u should move up by half page height
	model.nav.diffCursor = 80
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = result.(Model)
	halfPage := pageHeight / 2
	assert.Equal(t, 80-halfPage, model.nav.diffCursor, "ctrl+u should move cursor up by half viewport height")
}
func TestModel_CtrlDMovesHalfPageDown(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff
	assert.Equal(t, 0, model.nav.diffCursor)

	pageHeight := model.layout.viewport.Height
	halfPage := pageHeight / 2
	require.Positive(t, halfPage, "half page must be positive")

	// ctrl+d moves cursor and viewport by half page
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = result.(Model)
	assert.Equal(t, halfPage, model.nav.diffCursor, "ctrl+d should move cursor by half viewport height")
	assert.Equal(t, halfPage, model.layout.viewport.YOffset, "ctrl+d should scroll viewport by half page")

	// PgDn moves full page from start for comparison
	model.nav.diffCursor = 0
	model.layout.viewport.SetYOffset(0)
	model.layout.viewport.SetContent(model.renderDiff())
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, pageHeight, model.nav.diffCursor, "PgDown should move cursor by full viewport height")
}
func TestModel_CtrlUMovesHalfPageUp(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff

	pageHeight := model.layout.viewport.Height
	halfPage := pageHeight / 2
	require.Positive(t, halfPage, "half page must be positive")

	// start at line 80 with viewport scrolled to match
	model.nav.diffCursor = 80
	model.layout.viewport.SetYOffset(80)
	model.layout.viewport.SetContent(model.renderDiff())
	prevOffset := model.layout.viewport.YOffset

	// ctrl+u moves cursor and viewport by half page up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = result.(Model)
	assert.Equal(t, 80-halfPage, model.nav.diffCursor, "ctrl+u should move cursor up by half viewport height")
	assert.Equal(t, prevOffset-halfPage, model.layout.viewport.YOffset, "ctrl+u should scroll viewport up by half page")

	// PgUp moves full page up from 80 for comparison
	model.nav.diffCursor = 80
	model.layout.viewport.SetYOffset(80)
	model.layout.viewport.SetContent(model.renderDiff())
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)
	assert.Equal(t, 80-pageHeight, model.nav.diffCursor, "PgUp should move cursor up by full viewport height")
}
func TestModel_TreeCtrlDUMovesHalfPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree
	m.layout.height = 20

	pageSize := m.treePageSize()
	halfPage := max(1, pageSize/2)

	// ctrl+d from start
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", halfPage), model.tree.SelectedFile(),
		"ctrl+d should move by half page")

	// ctrl+u from end area
	m3 := testModel(files, nil)
	m3.tree = testNewFileTree(files)
	m3.layout.focus = paneTree
	m3.layout.height = 20
	// move to file 39
	m3.tree.Move(sidepane.MotionLast)
	for range 10 {
		m3.tree.Move(sidepane.MotionUp)
	}
	assert.Equal(t, "pkg/file39.go", m3.tree.SelectedFile())

	result, _ = m3.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model3 := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", 39-halfPage), model3.tree.SelectedFile(),
		"ctrl+u should move by half page")

	// PgDn from start should move full page
	m2 := testModel(files, nil)
	m2.tree = testNewFileTree(files)
	m2.layout.focus = paneTree
	m2.layout.height = 20

	result, _ = m2.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model2 := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", pageSize), model2.tree.SelectedFile(),
		"PgDn should move by full page")
}
func TestModel_HomeEndMoveCursorToBoundaries(t *testing.T) {
	lines := make([]diff.DiffLine, 50)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff

	// move to middle
	model.nav.diffCursor = 25

	// end should move to last line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = result.(Model)
	assert.Equal(t, 49, model.nav.diffCursor, "End should move cursor to last line")

	// home should move to first line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "Home should move cursor to first line")
}
func TestModel_HomeEndSkipDividers(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{Content: "...", ChangeType: diff.ChangeDivider},
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff

	// home should skip leading divider
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor, "Home should skip leading divider")

	// end should skip trailing divider
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = result.(Model)
	assert.Equal(t, 2, model.nav.diffCursor, "End should skip trailing divider")
}
func TestModel_PgDownClampsAtEnd(t *testing.T) {
	lines := make([]diff.DiffLine, 10)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file - viewport height is much larger than 10 lines
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff

	// pgdown when there are fewer lines than page height should clamp at last line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, 9, model.nav.diffCursor, "PgDown should clamp at last line")
}
func TestModel_PgUpClampsAtStart(t *testing.T) {
	lines := make([]diff.DiffLine, 10)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff
	model.nav.diffCursor = 3

	// pgup from line 3 with large page height should clamp at first line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "PgUp should clamp at first line")
}
func TestModel_PgDownAccountsForDividers(t *testing.T) {
	// create diff lines with dividers every 5 lines (simulating hunk boundaries)
	lines := make([]diff.DiffLine, 0, 71)
	for i := range 60 {
		if i > 0 && i%5 == 0 {
			lines = append(lines, diff.DiffLine{Content: "@@ hunk @@", ChangeType: diff.ChangeDivider})
		}
		lines = append(lines, diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff
	assert.Equal(t, 0, model.nav.diffCursor)

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown with dividers: cursor traverses fewer non-divider lines than viewport height
	// because divider rows consume visual space without being cursor-selectable
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Positive(t, model.nav.diffCursor, "cursor should have moved forward")

	nonDividerCount := 0
	for i := range model.nav.diffCursor {
		if lines[i].ChangeType != diff.ChangeDivider {
			nonDividerCount++
		}
	}
	assert.Less(t, nonDividerCount, pageHeight,
		"non-divider positions traversed should be fewer than viewport height")
}
func TestModel_PgDownScrollsViewportByPage(t *testing.T) {
	lines := make([]diff.DiffLine, 200)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff
	assert.Equal(t, 0, model.layout.viewport.YOffset)

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown should scroll viewport by approximately a full page (not just 1 line)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, pageHeight, model.layout.viewport.YOffset,
		"viewport should scroll to cursor position (full page), not just 1 line")

	// second pgdown should scroll another full page
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, 2*pageHeight, model.layout.viewport.YOffset,
		"viewport should advance by another full page")
}
func TestModel_PgUpScrollsViewportByPage(t *testing.T) {
	lines := make([]diff.DiffLine, 200)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight)

	// move cursor to line 100
	model.nav.diffCursor = 100
	model.syncViewportToCursor()

	// pgup should scroll viewport back by approximately a full page
	prevOffset := model.layout.viewport.YOffset
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)

	// viewport should scroll back significantly, not just 1 line
	scrolled := prevOffset - model.layout.viewport.YOffset
	assert.GreaterOrEqual(t, scrolled, pageHeight-1,
		"viewport should scroll back by approximately a full page")
}
func TestModel_TreePgDownMovesCursorByPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree
	m.layout.height = 20

	// cursor starts on first file
	assert.Equal(t, "pkg/file00.go", m.tree.SelectedFile())

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)

	// PgDown should advance cursor by page size files
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", pageSize), model.tree.SelectedFile(),
		"PgDown in tree should move cursor by page size")
}
func TestModel_TreePgUpMovesCursorByPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree
	m.layout.height = 20

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)

	// move cursor to the last file, then back 10 — lands on file39
	m.tree.Move(sidepane.MotionLast)
	for range 10 {
		m.tree.Move(sidepane.MotionUp)
	}
	assert.Equal(t, "pkg/file39.go", m.tree.SelectedFile())

	// PgUp should move back by page size files
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model := result.(Model)
	expected := fmt.Sprintf("pkg/file%02d.go", 39-pageSize)
	assert.Equal(t, expected, model.tree.SelectedFile(), "PgUp in tree should move cursor by page size")
}
func TestModel_TreeCtrlDMovesCursorByHalfPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree
	m.layout.height = 20

	assert.Equal(t, "pkg/file00.go", m.tree.SelectedFile())

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)
	halfPage := max(1, pageSize/2)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", halfPage), model.tree.SelectedFile(),
		"ctrl+d in tree should move cursor by half page size")
}
func TestModel_TreeCtrlUMovesCursorByHalfPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree
	m.layout.height = 20

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)
	halfPage := max(1, pageSize/2)

	m.tree.Move(sidepane.MotionLast)
	for range 10 {
		m.tree.Move(sidepane.MotionUp)
	}
	assert.Equal(t, "pkg/file39.go", m.tree.SelectedFile())

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model := result.(Model)
	expected := fmt.Sprintf("pkg/file%02d.go", 39-halfPage)
	assert.Equal(t, expected, model.tree.SelectedFile(), "ctrl+u in tree should move cursor by half page size")
}
func TestModel_TreeHomeEndMoveToBoundaries(t *testing.T) {
	files := []string{"cmd/main.go", "internal/a.go", "internal/b.go", "internal/c.go", "pkg/util.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree

	// move to middle
	m.tree.Move(sidepane.MotionDown)
	m.tree.Move(sidepane.MotionDown)
	assert.NotEqual(t, "cmd/main.go", m.tree.SelectedFile())

	// end should move to last file
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model := result.(Model)
	assert.Equal(t, "pkg/util.go", model.tree.SelectedFile(), "End in tree should move to last file")

	// home should move to first file
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = result.(Model)
	assert.Equal(t, "cmd/main.go", model.tree.SelectedFile(), "Home in tree should move to first file")
}
func TestModel_TreeScrollOffsetPersistsAcrossUpdates(t *testing.T) {
	// many files so tree needs scrolling at the given height
	files := make([]string, 30)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree
	m.layout.height = 10 // visible tree height = 10-3 = 7 rows

	// scroll down past the visible window via repeated Update calls
	var result tea.Model
	for range 15 {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = result.(Model)
	}
	// cursor should be well past the initial visible window
	fileAfterDown := m.tree.SelectedFile()
	assert.NotEqual(t, "pkg/file00.go", fileAfterDown, "cursor should have moved past first file")

	// move up one step, the selected file should change by one
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(Model)
	assert.NotEqual(t, fileAfterDown, m.tree.SelectedFile(),
		"selected file should change after moving up")
	assert.NotEqual(t, "pkg/file00.go", m.tree.SelectedFile(),
		"cursor should not jump back to first file after moving up within scrolled view")
}
func TestModel_FindHunks(t *testing.T) {
	tests := []struct {
		name   string
		lines  []diff.DiffLine
		expect []int
	}{
		{name: "no lines", lines: nil, expect: nil},
		{name: "all context", lines: []diff.DiffLine{
			{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "b", ChangeType: diff.ChangeContext},
		}, expect: nil},
		{name: "single chunk", lines: []diff.DiffLine{
			{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "b", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "c", ChangeType: diff.ChangeAdd},
			{NewNum: 4, Content: "d", ChangeType: diff.ChangeContext},
		}, expect: []int{1}},
		{name: "multiple chunks", lines: []diff.DiffLine{
			{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "b", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "c", ChangeType: diff.ChangeContext},
			{OldNum: 4, Content: "d", ChangeType: diff.ChangeRemove},
			{OldNum: 5, Content: "e", ChangeType: diff.ChangeRemove},
			{NewNum: 4, Content: "f", ChangeType: diff.ChangeContext},
		}, expect: []int{1, 3}},
		{name: "all changes", lines: []diff.DiffLine{
			{NewNum: 1, Content: "a", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "b", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "c", ChangeType: diff.ChangeAdd},
		}, expect: []int{0}},
		{name: "mixed add and remove in one chunk", lines: []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx", ChangeType: diff.ChangeContext},
		}, expect: []int{1}},
		{name: "chunks separated by divider", lines: []diff.DiffLine{
			{NewNum: 1, Content: "a", ChangeType: diff.ChangeAdd},
			{Content: "...", ChangeType: diff.ChangeDivider},
			{NewNum: 10, Content: "b", ChangeType: diff.ChangeAdd},
		}, expect: []int{0, 2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.file.lines = tc.lines
			assert.Equal(t, tc.expect, m.findHunks())
		})
	}
}
func TestModel_CurrentHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{NewNum: 2, Content: "add1", ChangeType: diff.ChangeAdd},     // 1 - hunk 1 start
		{NewNum: 3, Content: "add2", ChangeType: diff.ChangeAdd},     // 2
		{NewNum: 4, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
		{OldNum: 5, Content: "rem1", ChangeType: diff.ChangeRemove},  // 4 - hunk 2 start
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext}, // 5
		{NewNum: 6, Content: "add3", ChangeType: diff.ChangeAdd},     // 6 - chunk 3 start
	}

	tests := []struct {
		name      string
		cursor    int
		wantHunk  int
		wantTotal int
	}{
		{name: "file annotation line", cursor: -1, wantHunk: 0, wantTotal: 3},
		{name: "before first chunk", cursor: 0, wantHunk: 0, wantTotal: 3},
		{name: "at first chunk start", cursor: 1, wantHunk: 1, wantTotal: 3},
		{name: "inside first chunk", cursor: 2, wantHunk: 1, wantTotal: 3},
		{name: "between chunks", cursor: 3, wantHunk: 0, wantTotal: 3},
		{name: "at second chunk", cursor: 4, wantHunk: 2, wantTotal: 3},
		{name: "between hunk 2 and 3", cursor: 5, wantHunk: 0, wantTotal: 3},
		{name: "at third chunk", cursor: 6, wantHunk: 3, wantTotal: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.file.lines = lines
			m.nav.diffCursor = tc.cursor
			hunk, total := m.currentHunk()
			assert.Equal(t, tc.wantHunk, hunk)
			assert.Equal(t, tc.wantTotal, total)
		})
	}
}
func TestModel_CurrentHunkNoChanges(t *testing.T) {
	m := testModel(nil, nil)
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
	}
	hunk, total := m.currentHunk()
	assert.Equal(t, 0, hunk)
	assert.Equal(t, 0, total)
}
func TestModel_MoveToNextHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},      // 1 - hunk 1
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 2
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},     // 3 - hunk 2
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext}, // 4
	}

	m := testModel(nil, nil)
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.file.name = "a.go"
	m.layout.viewport.Height = 20

	m.moveToNextHunk()
	assert.Equal(t, 1, m.nav.diffCursor, "should jump to hunk 1")

	m.moveToNextHunk()
	assert.Equal(t, 3, m.nav.diffCursor, "should jump to hunk 2")

	// at last chunk, should not move
	m.moveToNextHunk()
	assert.Equal(t, 3, m.nav.diffCursor, "should stay at last chunk")
}
func TestModel_MoveToPrevHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},      // 1 - hunk 1
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 2
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},     // 3 - hunk 2
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext}, // 4
	}

	m := testModel(nil, nil)
	m.file.lines = lines
	m.nav.diffCursor = 4
	m.file.name = "a.go"
	m.layout.viewport.Height = 20

	m.moveToPrevHunk()
	assert.Equal(t, 3, m.nav.diffCursor, "should jump to hunk 2")

	m.moveToPrevHunk()
	assert.Equal(t, 1, m.nav.diffCursor, "should jump to hunk 1")

	// at first chunk, should not move
	m.moveToPrevHunk()
	assert.Equal(t, 1, m.nav.diffCursor, "should stay at first chunk")
}
func TestModel_CenterHunkInViewport_SmallHunk(t *testing.T) {
	// build a file with context, a small 3-line hunk, and more context
	// total: 50 lines, hunk at indices 20-22
	var lines []diff.DiffLine
	for i := 1; i <= 20; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	lines = append(lines,
		diff.DiffLine{NewNum: 21, Content: "add1", ChangeType: diff.ChangeAdd}, // idx 20
		diff.DiffLine{NewNum: 22, Content: "add2", ChangeType: diff.ChangeAdd}, // idx 21
		diff.DiffLine{NewNum: 23, Content: "add3", ChangeType: diff.ChangeAdd}, // idx 22
	)
	for i := 24; i <= 50; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff
	vpHeight := m.layout.viewport.Height
	require.Positive(t, vpHeight)

	m.nav.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 20, m.nav.diffCursor, "cursor should land on first line of hunk")

	// hunk midpoint: cursorY + hunkHeight/2 = 20 + 1 = 21
	// offset: midY - vpHeight/2
	expectedOffset := 21 - vpHeight/2
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset, "small hunk should be centered by its midpoint")
}

func TestModel_CenterHunkInViewport_LargeHunk(t *testing.T) {
	// hunk larger than viewport
	var lines []diff.DiffLine
	for i := 1; i <= 10; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	for i := 11; i <= 110; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "add", ChangeType: diff.ChangeAdd})
	}
	lines = append(lines, diff.DiffLine{NewNum: 111, Content: "ctx", ChangeType: diff.ChangeContext})

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff

	m.nav.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 10, m.nav.diffCursor, "cursor should land on first line of hunk")

	// hunk is 100 lines >> viewport, so offset = cursorY - 2 = 10 - 2 = 8
	assert.Equal(t, 8, m.layout.viewport.YOffset, "large hunk should place first line near top with context margin")
}

func TestModel_CenterHunkInViewport_HunkAtStart(t *testing.T) {
	// hunk starts at line 0 — offset should be clamped to 0
	var lines []diff.DiffLine
	lines = append(lines,
		diff.DiffLine{NewNum: 1, Content: "add1", ChangeType: diff.ChangeAdd},
		diff.DiffLine{NewNum: 2, Content: "add2", ChangeType: diff.ChangeAdd},
	)
	for i := 3; i <= 50; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff

	m.nav.diffCursor = 10
	m.moveToPrevHunk()
	assert.Equal(t, 0, m.nav.diffCursor, "cursor should land on first line")
	assert.Equal(t, 0, m.layout.viewport.YOffset, "offset should be clamped to 0 when hunk is near top")
}

func TestModel_CenterHunkInViewport_PrevHunkCenters(t *testing.T) {
	// verify moveToPrevHunk also centers the hunk
	var lines []diff.DiffLine
	for i := 1; i <= 25; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	lines = append(lines,
		diff.DiffLine{NewNum: 26, Content: "add1", ChangeType: diff.ChangeAdd}, // idx 25
		diff.DiffLine{NewNum: 27, Content: "add2", ChangeType: diff.ChangeAdd}, // idx 26
	)
	for i := 28; i <= 60; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff
	vpHeight := m.layout.viewport.Height

	m.nav.diffCursor = 40
	m.syncViewportToCursor()
	m.moveToPrevHunk()
	assert.Equal(t, 25, m.nav.diffCursor, "cursor should land on first line of hunk")

	// hunk midpoint: 25 + 2/2 = 26, offset: 26 - vpHeight/2
	expectedOffset := 26 - vpHeight/2
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset, "prevHunk should center the hunk by its midpoint")
}

func TestModel_CenterHunkInViewport_SingleLineHunk(t *testing.T) {
	// single-line hunk should still be centered
	var lines []diff.DiffLine
	for i := 1; i <= 25; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	lines = append(lines, diff.DiffLine{NewNum: 26, Content: "add", ChangeType: diff.ChangeAdd}) // idx 25
	for i := 27; i <= 60; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff
	vpHeight := m.layout.viewport.Height

	m.nav.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 25, m.nav.diffCursor)

	// single-line hunk: midY = 25 + 0 = 25, offset = 25 - vpHeight/2
	expectedOffset := 25 - vpHeight/2
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset, "single-line hunk midpoint centered in viewport")
}

func TestModel_CenterHunkInViewport_WrapMode(t *testing.T) {
	// when wrap mode is on, long changed lines occupy multiple visual rows;
	// the hunk visual height (and thus the centering offset) must account for them
	var lines []diff.DiffLine
	for i := 1; i <= 20; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	// two changed lines: one short, one long enough to wrap
	lines = append(lines,
		diff.DiffLine{NewNum: 21, Content: "short add", ChangeType: diff.ChangeAdd},              // idx 20
		diff.DiffLine{NewNum: 22, Content: strings.Repeat("a", 200), ChangeType: diff.ChangeAdd}, // idx 21, wraps
	)
	for i := 23; i <= 60; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff
	m.modes.wrap = true
	// force single-file mode so tree pane is hidden and wrap width is deterministic:
	// diffContentWidth = 80 - 4 = 76, wrapWidth = 76 - 3 (wrap gutter) = 73
	m.file.singleFile = true
	m.layout.treeWidth = 0
	vpHeight := m.layout.viewport.Height
	require.Positive(t, vpHeight)

	m.nav.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 20, m.nav.diffCursor, "cursor should land on first line of hunk")

	// pinned literals independent of production helpers:
	// - 20 context lines above hunk, each 1 visual row → cursorY = 20
	// - first add line "short add" → 1 row
	// - second add line is 200 'a' chars at wrap width 73 → 3 rows
	// - total hunk visual height = 1 + 3 = 4
	const cursorY = 20
	const hunkVisualHeight = 4
	expectedOffset := max(0, cursorY+hunkVisualHeight/2-vpHeight/2)
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset, "wrap mode: offset should account for wrapped line height")
}

func TestModel_CenterHunkInViewport_WithAnnotation(t *testing.T) {
	// an inline annotation on a hunk line adds visual rows that must be
	// included when computing the hunk midpoint for centering
	var lines []diff.DiffLine
	for i := 1; i <= 25; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	lines = append(lines,
		diff.DiffLine{NewNum: 26, Content: "add1", ChangeType: diff.ChangeAdd}, // idx 25
		diff.DiffLine{NewNum: 27, Content: "add2", ChangeType: diff.ChangeAdd}, // idx 26
	)
	for i := 28; i <= 60; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff
	vpHeight := m.layout.viewport.Height

	// add an annotation on the first changed line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 26, Type: "+", Comment: "review note"})
	defer m.store.Delete("a.go", 26, "+")

	m.nav.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 25, m.nav.diffCursor, "cursor should land on first line of hunk")

	// pinned literals independent of production helpers:
	// - 25 context lines above hunk, each 1 visual row → cursorY = 25
	// - 2 short add lines, each 1 row
	// - "review note" annotation (short, no wrap) → 1 row
	// - total hunk visual height = 2 + 1 = 3
	const cursorY = 25
	const hunkVisualHeight = 3
	expectedOffset := max(0, cursorY+hunkVisualHeight/2-vpHeight/2)
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset, "annotation: offset should include annotation visual rows")
}

func TestModel_CenterHunkInViewport_NoOpOnContextLine(t *testing.T) {
	// calling centerHunkInViewport when the cursor is on a context line
	// should be a no-op — viewport offset must not change
	var lines []diff.DiffLine
	for i := 1; i <= 30; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	lines = append(lines, diff.DiffLine{NewNum: 31, Content: "add", ChangeType: diff.ChangeAdd})
	for i := 32; i <= 60; i++ {
		lines = append(lines, diff.DiffLine{NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff

	// position cursor on a context line and set a known viewport offset
	m.nav.diffCursor = 10
	m.layout.viewport.SetYOffset(5)
	m.layout.viewport.SetContent(m.renderDiff())
	beforeOffset := m.layout.viewport.YOffset

	m.centerHunkInViewport()
	assert.Equal(t, beforeOffset, m.layout.viewport.YOffset, "should be no-op when cursor is on context line")
}

func TestModel_CenterHunkInViewport_CollapsedMode(t *testing.T) {
	// in collapsed mode, removed lines in a mixed add/remove hunk are hidden;
	// the visual height should only count visible (add) lines
	var lines []diff.DiffLine
	for i := 1; i <= 20; i++ {
		lines = append(lines, diff.DiffLine{OldNum: i, NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}
	// mixed hunk: 3 removes then 3 adds (indices 20-25)
	lines = append(lines,
		diff.DiffLine{OldNum: 21, Content: "old1", ChangeType: diff.ChangeRemove}, // idx 20, hidden in collapsed
		diff.DiffLine{OldNum: 22, Content: "old2", ChangeType: diff.ChangeRemove}, // idx 21, hidden
		diff.DiffLine{OldNum: 23, Content: "old3", ChangeType: diff.ChangeRemove}, // idx 22, hidden
		diff.DiffLine{NewNum: 21, Content: "new1", ChangeType: diff.ChangeAdd},    // idx 23, visible
		diff.DiffLine{NewNum: 22, Content: "new2", ChangeType: diff.ChangeAdd},    // idx 24, visible
		diff.DiffLine{NewNum: 23, Content: "new3", ChangeType: diff.ChangeAdd},    // idx 25, visible
	)
	for i := 24; i <= 60; i++ {
		lines = append(lines, diff.DiffLine{OldNum: i, NewNum: i, Content: "ctx", ChangeType: diff.ChangeContext})
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	vpHeight := m.layout.viewport.Height

	m.nav.diffCursor = 0
	m.moveToNextHunk()
	// in collapsed mode, cursor lands on first visible line (first add, idx 23)
	assert.Equal(t, 23, m.nav.diffCursor, "cursor should skip hidden removes and land on first add")

	// pinned literals independent of production helpers:
	// - 20 context lines above hunk, each 1 visual row
	// - in collapsed mode, 3 hidden removes contribute 0 rows each → cursorY = 20
	// - hunk visual height = 3 visible add lines (removes contribute 0)
	const cursorY = 20
	const hunkVisualHeight = 3
	expectedOffset := max(0, cursorY+hunkVisualHeight/2-vpHeight/2)
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset, "collapsed mode: offset should only count visible lines")
}

func TestModel_HunkNavigationViaKeys(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},      // 1 - hunk 1
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 2
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},     // 3 - hunk 2
	}

	m := testModel(nil, nil)
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.file.name = "a.go"
	m.layout.focus = paneDiff
	m.layout.viewport.Height = 20

	// press ] to go to next chunk
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor, "] should jump to first chunk")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model = result.(Model)
	assert.Equal(t, 3, model.nav.diffCursor, "] should jump to second chunk")

	// press [ to go to previous chunk
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model = result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor, "[ should jump back to first chunk")
}
func TestModel_HorizontalScroll(t *testing.T) {
	longLine := "package " + strings.Repeat("x", 200)
	lines := []diff.DiffLine{{OldNum: 1, NewNum: 1, Content: longLine, ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneDiff

	assert.Equal(t, 0, m.layout.scrollX)

	// scroll right
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = result.(Model)
	assert.Equal(t, scrollStep, m.layout.scrollX)

	// scroll left back to 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, 0, m.layout.scrollX)

	// scroll left at 0 stays at 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, 0, m.layout.scrollX)
}
func TestModel_HorizontalScrollResetsOnFileLoad(t *testing.T) {
	lines := []diff.DiffLine{{OldNum: 1, NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.scrollX = 20

	// loading new file should reset scrollX
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	assert.Equal(t, 0, m.layout.scrollX)
}
func TestModel_PaneHeight(t *testing.T) {
	tests := []struct {
		name         string
		noStatusBar  bool
		height, want int
	}{
		{name: "with status bar", noStatusBar: false, height: 40, want: 37},
		{name: "without status bar", noStatusBar: true, height: 40, want: 38},
		{name: "min clamp with status", noStatusBar: false, height: 3, want: 1},
		{name: "min clamp without status", noStatusBar: true, height: 2, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.layout.height = tc.height
			m.cfg.noStatusBar = tc.noStatusBar
			assert.Equal(t, tc.want, m.paneHeight())
		})
	}
}
func TestModel_WrapContent(t *testing.T) {
	m := testModel(nil, nil)

	t.Run("short line unchanged", func(t *testing.T) {
		lines := m.wrapContent("hello world", 40)
		assert.Equal(t, []string{"hello world"}, lines)
	})

	t.Run("long line wraps at word boundary", func(t *testing.T) {
		lines := m.wrapContent("the quick brown fox jumps over the lazy dog", 20)
		assert.Greater(t, len(lines), 1, "should produce multiple lines")
		for _, line := range lines {
			assert.LessOrEqual(t, len(line), 20, "each line should fit within width")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		lines := m.wrapContent("", 40)
		assert.Equal(t, []string{""}, lines)
	})

	t.Run("zero width returns content as-is", func(t *testing.T) {
		lines := m.wrapContent("hello", 0)
		assert.Equal(t, []string{"hello"}, lines)
	})

	t.Run("negative width returns content as-is", func(t *testing.T) {
		lines := m.wrapContent("hello", -5)
		assert.Equal(t, []string{"hello"}, lines)
	})

	t.Run("single long word", func(t *testing.T) {
		lines := m.wrapContent("abcdefghijklmnopqrstuvwxyz", 10)
		require.NotEmpty(t, lines)
		// ansi.Wrap hard-wraps words that exceed the limit
		for _, line := range lines {
			assert.LessOrEqual(t, len(line), 10+1, "long words should be hard-wrapped") // +1 for potential breakpoint
		}
	})

	t.Run("content with ANSI codes", func(t *testing.T) {
		ansiContent := "\033[32mgreen text\033[0m and normal"
		lines := m.wrapContent(ansiContent, 15)
		require.NotEmpty(t, lines)
		// the wrapped output should still contain ANSI codes
		joined := strings.Join(lines, "")
		assert.Contains(t, joined, "\033[32m", "ANSI codes should be preserved")
	})

	t.Run("multi-byte characters", func(t *testing.T) {
		lines := m.wrapContent("日本語テスト hello world", 10)
		require.NotEmpty(t, lines)
		assert.Greater(t, len(lines), 1, "CJK text should wrap")
	})
}
func TestModel_RenderDiffLineWithWrap(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wrap = true
	m.layout.width = 60
	m.layout.treeWidth = 12

	t.Run("short line no continuation", func(t *testing.T) {
		var b strings.Builder
		dl := diff.DiffLine{Content: "short", ChangeType: diff.ChangeAdd, NewNum: 1}
		m.renderDiffLine(&b, 0, dl)
		output := b.String()
		assert.Contains(t, output, " + short")
		assert.NotContains(t, output, "↪", "short line should not have continuation")
		assert.Equal(t, 1, strings.Count(output, "\n"), "should produce exactly one line")
	})

	t.Run("long add line wraps with continuation markers", func(t *testing.T) {
		var b strings.Builder
		longContent := "this is a very long line that should definitely be wrapped at word boundaries to fit the viewport"
		dl := diff.DiffLine{Content: longContent, ChangeType: diff.ChangeAdd, NewNum: 1}
		m.renderDiffLine(&b, 0, dl)
		output := b.String()

		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		require.Greater(t, len(lines), 1, "long line should wrap into multiple lines")

		// first line should have " + " prefix
		assert.Contains(t, lines[0], " + ", "first line should have add prefix")

		// continuation lines should have " ↪ " prefix
		for _, line := range lines[1:] {
			assert.Contains(t, line, " ↪ ", "continuation lines should have ↪ marker")
		}
	})

	t.Run("long remove line wraps with continuation markers", func(t *testing.T) {
		var b strings.Builder
		longContent := "this is a removed line that is very long and should be wrapped at word boundaries to fit the viewport width"
		dl := diff.DiffLine{Content: longContent, ChangeType: diff.ChangeRemove, OldNum: 5}
		m.renderDiffLine(&b, 0, dl)
		output := b.String()

		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		require.Greater(t, len(lines), 1, "long line should wrap")
		assert.Contains(t, lines[0], " - ", "first line should have remove prefix")
		for _, line := range lines[1:] {
			assert.Contains(t, line, " ↪ ", "continuation lines should have ↪ marker")
		}
	})

	t.Run("long context line wraps with continuation markers", func(t *testing.T) {
		var b strings.Builder
		longContent := "this is a context line that is very long and should be wrapped at word boundaries for readability"
		dl := diff.DiffLine{Content: longContent, ChangeType: diff.ChangeContext, NewNum: 10}
		m.renderDiffLine(&b, 0, dl)
		output := b.String()

		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		require.Greater(t, len(lines), 1, "long context line should wrap")
		for _, line := range lines[1:] {
			assert.Contains(t, line, " ↪ ", "continuation lines should have ↪ marker")
		}
	})

	t.Run("divider lines are not wrapped", func(t *testing.T) {
		var b strings.Builder
		dl := diff.DiffLine{Content: "@@ -1,5 +1,7 @@", ChangeType: diff.ChangeDivider}
		m.renderDiffLine(&b, 0, dl)
		output := b.String()
		assert.NotContains(t, output, "↪", "dividers should not be wrapped")
		assert.Equal(t, 1, strings.Count(output, "\n"), "divider should be a single line")
	})

	t.Run("cursor only on first visual line", func(t *testing.T) {
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff
		m.annot.cursorOnAnnotation = false

		var b strings.Builder
		longContent := "this is a very long line that should definitely be wrapped at word boundaries to test cursor placement"
		dl := diff.DiffLine{Content: longContent, ChangeType: diff.ChangeAdd, NewNum: 1}
		m.renderDiffLine(&b, 0, dl)
		output := b.String()

		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		require.Greater(t, len(lines), 1, "should have continuation lines")
		assert.Contains(t, lines[0], "▶", "first line should have cursor")
		for _, line := range lines[1:] {
			assert.NotContains(t, line, "▶", "continuation lines should not have cursor")
		}
	})

	t.Run("no horizontal scroll in wrap mode", func(t *testing.T) {
		m.layout.scrollX = 10
		m.modes.wrap = true

		var b strings.Builder
		dl := diff.DiffLine{Content: "@@ -1,3 +1,3 @@", ChangeType: diff.ChangeDivider}
		m.renderDiffLine(&b, 0, dl)

		// divider falls through to non-wrap path but ansi.Cut should be skipped
		output := b.String()
		assert.Contains(t, output, "@@", "divider content should not be scrolled in wrap mode")

		m.layout.scrollX = 0 // reset
	})
}
func TestModel_WrappedLineCount(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "short", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: strings.Repeat("x", 200), ChangeType: diff.ChangeAdd},
		{Content: "@@ -1,3 +1,3 @@", ChangeType: diff.ChangeDivider},
		{OldNum: 3, Content: strings.Repeat("y", 200), ChangeType: diff.ChangeRemove},
	}

	t.Run("wrap off returns 1 for all lines", func(t *testing.T) {
		m.modes.wrap = false
		assert.Equal(t, 1, m.wrappedLineCount(0))
		assert.Equal(t, 1, m.wrappedLineCount(1))
		assert.Equal(t, 1, m.wrappedLineCount(2))
		assert.Equal(t, 1, m.wrappedLineCount(3))
	})

	t.Run("wrap on, short line returns 1", func(t *testing.T) {
		m.modes.wrap = true
		assert.Equal(t, 1, m.wrappedLineCount(0))
	})

	t.Run("wrap on, long line returns more than 1", func(t *testing.T) {
		m.modes.wrap = true
		count := m.wrappedLineCount(1)
		assert.Greater(t, count, 1, "long add line should wrap to multiple visual rows")
	})

	t.Run("wrap on, divider always returns 1", func(t *testing.T) {
		m.modes.wrap = true
		assert.Equal(t, 1, m.wrappedLineCount(2))
	})

	t.Run("wrap on, long remove line wraps", func(t *testing.T) {
		m.modes.wrap = true
		count := m.wrappedLineCount(3)
		assert.Greater(t, count, 1, "long remove line should wrap to multiple visual rows")
	})

	t.Run("out of bounds returns 1", func(t *testing.T) {
		m.modes.wrap = true
		assert.Equal(t, 1, m.wrappedLineCount(-1))
		assert.Equal(t, 1, m.wrappedLineCount(100))
	})
}
func TestModel_CursorViewportYWithWrap(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.modes.wrap = true
	// use a narrow width so wrapping is predictable
	m.layout.width = 60
	m.layout.treeWidth = 20

	// diffContentWidth = 60 - 20 - 4 - 1 = 35
	// wrapWidth = 35 - 3 (gutter) = 32

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "short line", ChangeType: diff.ChangeContext},                                             // idx 0, fits in 1 row
		{NewNum: 2, Content: strings.Repeat("a", 60), ChangeType: diff.ChangeAdd},                                      // idx 1, wraps to ~2 rows
		{NewNum: 3, Content: "another short line", ChangeType: diff.ChangeContext},                                     // idx 2, fits in 1 row
		{NewNum: 4, Content: "this is a really long line that " + strings.Repeat("z", 60), ChangeType: diff.ChangeAdd}, // idx 3, wraps to ~3 rows
	}

	// verify wrapping counts are consistent
	count0 := m.wrappedLineCount(0)
	count1 := m.wrappedLineCount(1)
	count2 := m.wrappedLineCount(2)
	assert.Equal(t, 1, count0, "short context line should be 1 row")
	assert.Greater(t, count1, 1, "long add line should wrap")
	assert.Equal(t, 1, count2, "short context line should be 1 row")

	t.Run("cursor at 0, no wrapping before it", func(t *testing.T) {
		m.nav.diffCursor = 0
		m.annot.cursorOnAnnotation = false
		assert.Equal(t, 0, m.cursorViewportY())
	})

	t.Run("cursor at 1, after short line 0", func(t *testing.T) {
		m.nav.diffCursor = 1
		m.annot.cursorOnAnnotation = false
		assert.Equal(t, count0, m.cursorViewportY())
	})

	t.Run("cursor at 2, after wrapped line 1", func(t *testing.T) {
		m.nav.diffCursor = 2
		m.annot.cursorOnAnnotation = false
		assert.Equal(t, count0+count1, m.cursorViewportY())
	})

	t.Run("cursor at 3, after lines 0+1+2", func(t *testing.T) {
		m.nav.diffCursor = 3
		m.annot.cursorOnAnnotation = false
		assert.Equal(t, count0+count1+count2, m.cursorViewportY())
	})

	t.Run("cursor on annotation after wrapped line", func(t *testing.T) {
		// add annotation on line 2 (idx 1, the long wrapped add line)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "note"})
		defer func() { m.store.Delete("a.go", 2, "+") }()

		m.nav.diffCursor = 2
		m.annot.cursorOnAnnotation = false
		// cursor at line 2: count0 + count1 + 1 (annotation row after line 1)
		assert.Equal(t, count0+count1+1, m.cursorViewportY())
	})

	t.Run("cursor on annotation sub-line of wrapped line", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: " ", Comment: "note on ctx"})
		defer func() { m.store.Delete("a.go", 3, " ") }()

		m.nav.diffCursor = 2
		m.annot.cursorOnAnnotation = true
		// on annotation sub-line of line 2: offset is line0 + line1 rows + wrappedLineCount(2)
		assert.Equal(t, count0+count1+count2, m.cursorViewportY())
	})
}

func TestModel_SyncViewportToCursor_WrappedLineBottomVisible(t *testing.T) {
	// regression: cursor sitting on a wrapped line at the bottom of the viewport
	// used to keep only the first visual row visible. scroll must shift by the
	// extra wrap rows so the line's visual bottom stays within the viewport.
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.modes.wrap = true
	m.layout.width = 60
	m.layout.treeWidth = 20
	m.layout.viewport.Width = 40
	m.layout.viewport.Height = 4

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "short1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "short2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "short3", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: strings.Repeat("a", 120), ChangeType: diff.ChangeAdd}, // wraps to 3+ rows
	}

	wrapRows := m.wrappedLineCount(3)
	require.GreaterOrEqual(t, wrapRows, 3, "long line should wrap to at least 3 rows")

	m.nav.diffCursor = 3
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(0)

	m.syncViewportToCursor()

	cursorTop := m.cursorViewportY()
	cursorBottom := cursorTop + wrapRows - 1
	expectedOffset := cursorBottom - m.layout.viewport.Height + 1
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset,
		"YOffset should pin the wrapped line's visual bottom to the viewport bottom")
	assert.GreaterOrEqual(t, cursorTop, m.layout.viewport.YOffset, "cursor top must be visible")
	assert.Less(t, cursorBottom, m.layout.viewport.YOffset+m.layout.viewport.Height,
		"last wrap row must be visible (was clipped below viewport before fix)")
}

func TestModel_SyncViewportToCursor_AnnotationRowsVisible(t *testing.T) {
	// regression: multi-row annotation injected below a diff line at the bottom
	// of the viewport used to render past the bottom edge because the down-scroll
	// branch only compared the cursor's top row.
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 80
	m.layout.treeWidth = 20

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "line4", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 4, Type: " ",
		Comment: "line-a\nline-b\nline-c"})

	annotRows := m.wrappedAnnotationLineCount(m.annotationKey(4, " "))
	require.Equal(t, 3, annotRows, "multi-line annotation should occupy 3 rows")

	// viewport height of 4 comfortably fits the 1-row diff line + 3 annotation rows
	m.layout.viewport.Width = 50
	m.layout.viewport.Height = 4
	m.nav.diffCursor = 3
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(0)

	m.syncViewportToCursor()

	cursorTop := m.cursorViewportY()
	cursorBottom := cursorTop + 1 + annotRows - 1 // 1 diff row + annotation rows
	expectedOffset := cursorBottom - m.layout.viewport.Height + 1
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset,
		"YOffset should pin the annotation's visual bottom to the viewport bottom")
	assert.GreaterOrEqual(t, cursorTop, m.layout.viewport.YOffset, "cursor top must be visible")
	assert.Less(t, cursorBottom, m.layout.viewport.YOffset+m.layout.viewport.Height,
		"last annotation row must be visible (was clipped below viewport before fix)")
}

func TestModel_SyncViewportToCursor_LineTallerThanViewport(t *testing.T) {
	// when the cursor's logical line is taller than the viewport, the floor
	// min(cursorBottom-Height+1, cursorTop) anchors scroll at cursorTop so the
	// cursor itself stays visible; trailing rows overflow by necessity.
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.modes.wrap = true
	m.layout.width = 60
	m.layout.treeWidth = 20
	m.layout.viewport.Width = 40
	m.layout.viewport.Height = 2

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "short1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: strings.Repeat("b", 200), ChangeType: diff.ChangeAdd}, // wraps to 5+ rows
	}

	wrapRows := m.wrappedLineCount(1)
	require.Greater(t, wrapRows, m.layout.viewport.Height, "wrapped line must exceed viewport height")

	m.nav.diffCursor = 1
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(0)

	m.syncViewportToCursor()

	cursorTop := m.cursorViewportY()
	assert.Equal(t, cursorTop, m.layout.viewport.YOffset,
		"YOffset should snap to cursorTop when logical line is taller than viewport")
}

func TestModel_SyncViewportToCursor_FileAnnotationOverflow(t *testing.T) {
	// file annotation on a narrow viewport: cursor at -1, multi-line file
	// annotation that exceeds viewport height; syncViewportToCursor should
	// keep the annotation's top row visible (the file-annotation branch of
	// cursorVisualHeight returns the file annotation's row count).
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 80
	m.layout.treeWidth = 20
	m.layout.viewport.Width = 50
	m.layout.viewport.Height = 2

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0,
		Comment: "file-a\nfile-b\nfile-c"})

	fileRows := m.wrappedAnnotationLineCount(annotKeyFile)
	require.Greater(t, fileRows, m.layout.viewport.Height, "file annotation must exceed viewport height")

	m.nav.diffCursor = -1 // cursor on file annotation line
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(5)

	m.syncViewportToCursor()

	// with cursor at top (Y=0) and prior offset=5, first branch fires:
	// SetYOffset(0) so the file annotation's top row is visible.
	assert.Equal(t, 0, m.layout.viewport.YOffset,
		"YOffset should scroll to show the file annotation top row")
}

func TestModel_SyncViewportToCursor_OnAnnotationSubLineOverflow(t *testing.T) {
	// cursor parked on annotation sub-line of a tall multi-row annotation:
	// cursorVisualHeight branches on cursorOnAnnotation and returns only the
	// annotation's row count, so scroll keeps the annotation's bottom visible.
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 80
	m.layout.treeWidth = 20
	m.layout.viewport.Width = 50
	m.layout.viewport.Height = 3

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: " ",
		Comment: "note-a\nnote-b\nnote-c"})

	annotRows := m.wrappedAnnotationLineCount(m.annotationKey(2, " "))
	require.Equal(t, 3, annotRows, "annotation should occupy 3 rows")

	m.nav.diffCursor = 1
	m.annot.cursorOnAnnotation = true
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(0)

	m.syncViewportToCursor()

	cursorTop := m.cursorViewportY()
	cursorBottom := cursorTop + annotRows - 1
	expectedOffset := cursorBottom - m.layout.viewport.Height + 1
	assert.Equal(t, expectedOffset, m.layout.viewport.YOffset,
		"YOffset should pin the annotation sub-line's visual bottom to the viewport bottom")
	assert.Less(t, cursorBottom, m.layout.viewport.YOffset+m.layout.viewport.Height,
		"last annotation row must be visible")
}

func TestModel_CursorViewportYWithWrapDeletePlaceholder(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.modes.wrap = true
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	m.layout.width = 60
	m.layout.treeWidth = 20

	// diffContentWidth = 60 - 20 - 4 - 1 = 35, wrapWidth = 35 - 3 = 32
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "context line", ChangeType: diff.ChangeContext},
		{OldNum: 1, Content: strings.Repeat("x", 80), ChangeType: diff.ChangeRemove}, // long remove, hunk start
		{OldNum: 2, Content: strings.Repeat("y", 80), ChangeType: diff.ChangeRemove}, // long remove
		{OldNum: 3, Content: strings.Repeat("z", 80), ChangeType: diff.ChangeRemove}, // long remove
		{NewNum: 2, Content: "after context", ChangeType: diff.ChangeContext},
	}

	// placeholder text "⋯ 3 lines deleted" is short (~17 chars), fits in 1 row at wrapWidth=32.
	// the original removed lines are 80 chars each and would wrap to ~3 rows.
	// cursorViewportY must use placeholder text (1 row), not original content (~3 rows).

	m.nav.diffCursor = 4 // cursor on "after context" line
	m.annot.cursorOnAnnotation = false
	m.layout.focus = paneDiff

	y := m.cursorViewportY()
	// expected: 1 (context) + 1 (placeholder = 1 visual row) = 2
	assert.Equal(t, 2, y, "viewport Y should count placeholder as 1 row, not original line content")
}
func TestModel_CursorViewportYWithWrapDeletePlaceholderAndBlame(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.modes.wrap = true
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	m.modes.showBlame = true
	m.file.blameData = map[int]diff.BlameLine{
		1: {Author: "LongName", Time: time.Now()},
	}
	m.file.blameAuthorLen = m.computeBlameAuthorLen()
	m.layout.width = 25
	m.layout.treeHidden = true

	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 1, Content: strings.Repeat("x", 40), ChangeType: diff.ChangeRemove},
		{OldNum: 2, Content: strings.Repeat("y", 40), ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: strings.Repeat("z", 40), ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "after context", ChangeType: diff.ChangeContext},
	}

	wrapWidth := m.diffContentWidth() - wrapGutterWidth - m.blameGutterWidth()
	placeholderRows := len(m.wrapContent(m.deletePlaceholderText(1), wrapWidth))
	require.Greater(t, placeholderRows, 1, "blame gutter should force the placeholder to wrap")
	contextRows := m.wrappedLineCount(0)

	m.nav.diffCursor = 4
	m.annot.cursorOnAnnotation = false

	assert.Equal(t, contextRows+placeholderRows, m.cursorViewportY())
}
func TestModel_WrapToggle(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.name = "a.go"
	m.file.lines = lines
	m.file.highlighted = []string{"x"}
	m.layout.focus = paneDiff
	m.layout.viewport.Width = 80
	m.layout.viewport.Height = 20
	assert.False(t, m.modes.wrap)

	// press w to enable wrap
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.True(t, model.modes.wrap)

	// press w again to disable wrap
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.modes.wrap)
}
func TestModel_WrapToggleResetsScrollX(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.name = "a.go"
	m.file.lines = lines
	m.file.highlighted = []string{"x"}
	m.layout.focus = paneDiff
	m.layout.viewport.Width = 80
	m.layout.viewport.Height = 20
	m.layout.scrollX = 10

	// enable wrap: scrollX should reset to 0
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.True(t, model.modes.wrap)
	assert.Equal(t, 0, model.layout.scrollX)
}
func TestModel_WrapToggleNoOpWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.layout.focus = paneDiff
	m.file.name = ""
	assert.False(t, m.modes.wrap)

	// w should be no-op without a loaded file
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.False(t, model.modes.wrap)
}
func TestModel_WrapToggleNoOpInTreePane(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneTree
	assert.False(t, m.modes.wrap)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.False(t, model.modes.wrap)
}
func TestModel_ScrollBlockedInWrapMode(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.name = "a.go"
	m.file.lines = lines
	m.file.highlighted = []string{"x"}
	m.layout.focus = paneDiff
	m.layout.viewport.Width = 80
	m.layout.viewport.Height = 20
	m.modes.wrap = true
	m.layout.scrollX = 0

	// right key should not change scrollX in wrap mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := result.(Model)
	assert.Equal(t, 0, model.layout.scrollX)

	// left key should not change scrollX in wrap mode
	model.layout.scrollX = 0
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = result.(Model)
	assert.Equal(t, 0, model.layout.scrollX)
}
func TestModel_ScrollWorksWithoutWrapMode(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.name = "a.go"
	m.file.lines = lines
	m.file.highlighted = []string{"x"}
	m.layout.focus = paneDiff
	m.layout.viewport.Width = 80
	m.layout.viewport.Height = 20
	m.modes.wrap = false
	m.layout.scrollX = 0

	// right key should scroll in non-wrap mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := result.(Model)
	assert.Positive(t, model.layout.scrollX)
}
func TestModel_ActiveSectionTrackingOnScroll(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# First", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Second", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "text", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "### Third", ChangeType: diff.ChangeContext},
		{NewNum: 6, Content: "text", ChangeType: diff.ChangeContext},
	}

	t.Run("scrolling diff updates TOC active section", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "README.md"
		m.file.lines = mdLines
		m.file.highlighted = make([]string, len(mdLines))
		m.layout.focus = paneDiff
		m.nav.diffCursor = 0

		// TOC entries: [0]=README.md(0), [1]=First(0), [2]=Second(2), [3]=Third(4)
		// move down one line at a time and verify active section via SyncCursorToActiveSection + CurrentLineIdx
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		// after scrolling to line 1 (under # First), sync cursor and check
		model.file.mdTOC.SyncCursorToActiveSection()
		idx, ok := model.file.mdTOC.CurrentLineIdx()
		assert.True(t, ok, "active section should be set")
		assert.Equal(t, 0, idx, "cursor at line 1 should be in First section (lineIdx=0)")

		// move to line 2 (## Second)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		model.file.mdTOC.SyncCursorToActiveSection()
		idx, ok = model.file.mdTOC.CurrentLineIdx()
		assert.True(t, ok)
		assert.Equal(t, 2, idx, "cursor at line 2 should be in Second section (lineIdx=2)")

		// move to line 4 (### Third)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		model.file.mdTOC.SyncCursorToActiveSection()
		idx, ok = model.file.mdTOC.CurrentLineIdx()
		assert.True(t, ok)
		assert.Equal(t, 4, idx, "cursor at line 4 should be in Third section (lineIdx=4)")
	})

	t.Run("no TOC does not crash on scroll", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.file.mdTOC = nil
		m.file.name = "main.go"
		m.file.lines = []diff.DiffLine{{Content: "package main", ChangeType: diff.ChangeContext}}
		m.file.highlighted = []string{"package main"}
		m.layout.focus = paneDiff
		m.nav.diffCursor = 0

		// should not panic
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Nil(t, model.file.mdTOC)
	})
}
func TestModel_CustomKeymapDiffNavNextHunk(t *testing.T) {
	// map "x" to next_hunk, unbind "]" — verify "x" jumps to next hunk and "]" does not
	km := keymap.Default()
	km.Bind("x", keymap.ActionNextHunk)
	km.Unbind("]")

	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},
	}

	m := testModel(nil, nil)
	m.keymap = km
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.file.name = "a.go"
	m.layout.focus = paneDiff
	m.layout.viewport.Height = 20

	// "x" should jump to next hunk
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor, "x should jump to first hunk")

	// "]" should not jump (unbound)
	model.nav.diffCursor = 0
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "] should not jump when unbound")
}
func TestModel_PendingHunkJump_FirstHunk(t *testing.T) {
	// File b.go has two hunks: divider, context, add, context, add
	bLines := []diff.DiffLine{
		{ChangeType: diff.ChangeDivider},
		{ChangeType: diff.ChangeContext, Content: "ctx1", NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "add1", NewNum: 2},
		{ChangeType: diff.ChangeContext, Content: "ctx2", NewNum: 3},
		{ChangeType: diff.ChangeAdd, Content: "add2", NewNum: 4},
	}
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 1}},
		"b.go": bLines,
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// set pendingHunkJump = true (first hunk)
	fwd := true
	m.nav.pendingHunkJump = &fwd
	m.file.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.file.loadSeq, lines: bLines}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "pendingHunkJump should be cleared after file load")
	assert.Equal(t, 2, model.nav.diffCursor, "cursor should land on first hunk (index 2, the add line)")
}
func TestModel_PendingHunkJump_LastHunk(t *testing.T) {
	// File b.go has two hunks
	bLines := []diff.DiffLine{
		{ChangeType: diff.ChangeDivider},
		{ChangeType: diff.ChangeContext, Content: "ctx1", NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "add1", NewNum: 2},
		{ChangeType: diff.ChangeContext, Content: "ctx2", NewNum: 3},
		{ChangeType: diff.ChangeAdd, Content: "add2", NewNum: 4},
	}
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 1}},
		"b.go": bLines,
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// set pendingHunkJump = false (last hunk)
	bwd := false
	m.nav.pendingHunkJump = &bwd
	m.file.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.file.loadSeq, lines: bLines}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "pendingHunkJump should be cleared after file load")
	assert.Equal(t, 4, model.nav.diffCursor, "cursor should land on last hunk (index 4, the second add line)")
}
func TestModel_PendingHunkJump_ClearedOnManualNav(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	// set pendingHunkJump then trigger manual tree navigation (j key from tree pane)
	fwd := true
	m.nav.pendingHunkJump = &fwd
	m.layout.focus = paneTree
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Nil(t, model.nav.pendingHunkJump, "pendingHunkJump should be cleared on manual tree navigation")
}

// loadFileIntoModel sets up a model with files loaded and a.go as the current file.
func loadFileIntoModel(t *testing.T, files []string, diffs map[string][]diff.DiffLine) Model {
	t.Helper()
	m := testModel(files, diffs)
	entries := make([]diff.FileEntry, len(files))
	for i, f := range files {
		entries[i] = diff.FileEntry{Path: f}
	}
	result, _ := m.Update(filesLoadedMsg{entries: entries})
	m = result.(Model)
	loadMsg := m.loadFileDiff(files[0])()
	result, _ = m.Update(loadMsg)
	m = result.(Model)
	m.layout.viewport.Height = 20
	return m
}
func TestModel_HunkNav_FromTreePane_SwitchesFocusToDiff(t *testing.T) {
	// pressing ] from tree pane should switch focus to diff and jump to first hunk
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 1},
			{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 2},
		},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.layout.focus = paneTree
	m.nav.diffCursor = 0 // on context line

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Equal(t, paneDiff, model.layout.focus, "] from tree pane should switch focus to diff")
	assert.Equal(t, 1, model.nav.diffCursor, "] should land on the add line (hunk start)")
}
func TestModel_HunkNav_PrevFromTreePane_SwitchesFocusToDiff(t *testing.T) {
	// pressing [ from tree pane should switch focus to diff and jump to prev hunk
	diffs := map[string][]diff.DiffLine{
		"a.go": {
			{ChangeType: diff.ChangeAdd, Content: "add1", NewNum: 1},
			{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 2},
			{ChangeType: diff.ChangeAdd, Content: "add2", NewNum: 3},
		},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.layout.focus = paneTree
	m.nav.diffCursor = 2 // on second add line

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := result.(Model)

	assert.Equal(t, paneDiff, model.layout.focus, "[ from tree pane should switch focus to diff")
	assert.Equal(t, 0, model.nav.diffCursor, "[ should land on first hunk start (index 0)")
}
func TestModel_HunkNav_NextCrossesFileForward(t *testing.T) {
	// pressing ] at the last hunk of a.go should navigate to b.go and set pendingHunkJump=true
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.cfg.crossFileHunks = true
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0 // at the only (last) hunk

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	require.NotNil(t, model.nav.pendingHunkJump, "pendingHunkJump should be set for cross-file forward jump")
	assert.True(t, *model.nav.pendingHunkJump, "pendingHunkJump should be true (land on first hunk)")
	assert.Equal(t, "b.go", model.tree.SelectedFile(), "tree should have advanced to b.go")
	assert.NotNil(t, cmd, "a load command should be returned")
}
func TestModel_HunkNav_PrevCrossesFileBackward(t *testing.T) {
	// pressing [ at the first hunk of b.go should navigate to a.go and set pendingHunkJump=false
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.cfg.crossFileHunks = true
	// select b.go in tree (index 0 = dir entry, index 1 = a.go, index 2 = b.go)
	m.tree.SelectByPath("b.go")
	m.file.loadSeq++
	bLoad := m.loadFileDiff("b.go")()
	result, _ := m.Update(bLoad)
	m = result.(Model)
	m.layout.viewport.Height = 20
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0 // at the first (and only) hunk

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := result.(Model)

	require.NotNil(t, model.nav.pendingHunkJump, "pendingHunkJump should be set for cross-file backward jump")
	assert.False(t, *model.nav.pendingHunkJump, "pendingHunkJump should be false (land on last hunk)")
	assert.Equal(t, "a.go", model.tree.SelectedFile(), "tree should have moved back to a.go")
	assert.NotNil(t, cmd, "a load command should be returned")
}
func TestModel_HunkNav_NextAtLastFileNoOp(t *testing.T) {
	// pressing ] at the last hunk of the last file: no-op
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "no pendingHunkJump when no next file")
	assert.Equal(t, 0, model.nav.diffCursor, "cursor should stay at last hunk")
	assert.Equal(t, "a.go", model.file.name, "should remain on a.go")
}
func TestModel_HunkNav_PrevAtFirstFileNoOp(t *testing.T) {
	// pressing [ at the first hunk of the first file: no-op
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "no pendingHunkJump when no prev file")
	assert.Equal(t, 0, model.nav.diffCursor, "cursor should stay at first hunk")
	assert.Equal(t, "a.go", model.file.name, "should remain on a.go")
}
func TestModel_HunkNav_SingleFileNoCrossFile(t *testing.T) {
	// in single-file mode, ] at last hunk should not cross to other files
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.cfg.crossFileHunks = true
	m.file.singleFile = true
	m.layout.treeWidth = 0
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "single-file mode should not set pendingHunkJump")
	assert.Equal(t, 0, model.nav.diffCursor, "cursor should not move in single-file mode at last hunk")
}
func TestModel_HunkNav_CrossFile_LandsOnFirstHunk(t *testing.T) {
	// end-to-end: ] from last hunk of a.go loads b.go and lands on its first hunk
	bLines := []diff.DiffLine{
		{ChangeType: diff.ChangeDivider},
		{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 1},
		{ChangeType: diff.ChangeAdd, Content: "add1", NewNum: 2},
		{ChangeType: diff.ChangeContext, Content: "ctx2", NewNum: 3},
		{ChangeType: diff.ChangeAdd, Content: "add2", NewNum: 4},
	}
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": bLines,
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.cfg.crossFileHunks = true
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0 // at the only hunk of a.go

	// press ] to trigger cross-file jump
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = result.(Model)
	require.NotNil(t, cmd, "should have a load command")

	// execute the load command and process the result
	loadMsg := cmd()
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "pendingHunkJump should be cleared after landing")
	assert.Equal(t, "b.go", model.file.name, "should have navigated to b.go")
	assert.Equal(t, 2, model.nav.diffCursor, "should land on first hunk of b.go (index 2)")
}
func TestModel_HunkNav_CrossFile_LandsOnLastHunk(t *testing.T) {
	// end-to-end: [ from first hunk of b.go loads a.go and lands on its last hunk
	aLines := []diff.DiffLine{
		{ChangeType: diff.ChangeAdd, Content: "add1", NewNum: 1},
		{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 2},
		{ChangeType: diff.ChangeAdd, Content: "add2", NewNum: 3},
	}
	diffs := map[string][]diff.DiffLine{
		"a.go": aLines,
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.cfg.crossFileHunks = true
	// select b.go in tree (index 0 = dir entry, index 1 = a.go, index 2 = b.go)
	m.tree.SelectByPath("b.go")
	m.file.loadSeq++
	loadMsg := m.loadFileDiff("b.go")()
	result, _ := m.Update(loadMsg)
	m = result.(Model)
	m.layout.viewport.Height = 20
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0 // at first (only) hunk of b.go

	// press [ to trigger cross-file backward jump
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m = result.(Model)
	require.NotNil(t, cmd, "should have a load command")

	// execute the load command and process the result
	loadMsg = cmd()
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump, "pendingHunkJump should be cleared after landing")
	assert.Equal(t, "a.go", model.file.name, "should have navigated to a.go")
	assert.Equal(t, 2, model.nav.diffCursor, "should land on last hunk of a.go (index 2)")
}
func TestModel_HunkNav_DefaultDoesNotCrossFiles(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Nil(t, model.nav.pendingHunkJump)
	assert.Equal(t, "a.go", model.file.name)
	assert.Equal(t, "a.go", model.tree.SelectedFile())
	assert.Equal(t, 0, model.nav.diffCursor)
}
func TestModel_PendingHunkJump_FallsBackToFirstVisibleLineWithoutHunks(t *testing.T) {
	lines := []diff.DiffLine{
		{ChangeType: diff.ChangeDivider},
		{ChangeType: diff.ChangeContext, Content: "ctx1", NewNum: 1},
		{ChangeType: diff.ChangeContext, Content: "ctx2", NewNum: 2},
	}
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": lines,
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	fwd := true
	m.nav.pendingHunkJump = &fwd
	m.file.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.file.loadSeq, lines: lines}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.nav.pendingHunkJump)
	assert.Equal(t, 1, model.nav.diffCursor, "should fall back to first visible context line")
}
func TestModel_PendingHunkJump_ClearedWhenPendingAnnotJumpLands(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "ctx", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := testModel([]string{"a.go", "b.go"}, diffs)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	msg := m.loadFileDiff("a.go")()
	result, _ = m.Update(msg)
	m = result.(Model)

	fwd := true
	m.nav.pendingHunkJump = &fwd
	m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+", Comment: "note"}
	m.file.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.file.loadSeq, lines: diffs["b.go"]}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingAnnotJump)
	assert.Nil(t, model.nav.pendingHunkJump)
	assert.Equal(t, 0, model.nav.diffCursor)
}

func TestModel_EnterInDiffPaneOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "--- a/file.go", ChangeType: diff.ChangeDivider},
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0 // on divider line

	// press enter on divider - should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annot.annotating, "enter on divider should not start annotation")
}

func TestModel_DiffLineNum(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, 5, m.diffLineNum(diff.DiffLine{NewNum: 5, ChangeType: diff.ChangeContext}))
	assert.Equal(t, 3, m.diffLineNum(diff.DiffLine{NewNum: 3, ChangeType: diff.ChangeAdd}))
	assert.Equal(t, 7, m.diffLineNum(diff.DiffLine{OldNum: 7, ChangeType: diff.ChangeRemove}))
}
