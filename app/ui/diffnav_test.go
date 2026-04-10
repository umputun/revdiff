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
)

func TestModel_NextPrevFile(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
		"c.go": {{NewNum: 1, Content: "c", ChangeType: diff.ChangeContext}},
	})
	m.tree = newFileTree(files)
	m.currFile = "a.go" // pretend first file is already loaded

	// starts on first file (a.go)
	assert.Equal(t, "a.go", m.tree.selectedFile())

	// press n - should move to b.go
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())
	assert.NotNil(t, cmd) // triggers file load

	// press n - should move to c.go
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, "c.go", model.tree.selectedFile())
	assert.NotNil(t, cmd)

	// press p - back to b.go
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())
	assert.NotNil(t, cmd)
}
func TestModel_DiffScrolling(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	// load file
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor)

	// j moves cursor down
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, 1, model.diffCursor)

	// k moves cursor back up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor)
}
func TestModel_DiffCursorSkipsDividers(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor)

	// j should skip divider and land on line10
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, 2, model.diffCursor)

	// k should skip divider and go back to line1
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor)
}
func TestModel_DiffCursorAutoScrolls(t *testing.T) {
	// create more lines than viewport height
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	// move cursor past viewport height - viewport should auto-scroll
	for range 50 {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
	}
	assert.Equal(t, 50, model.diffCursor)
	assert.Positive(t, model.viewport.YOffset, "viewport should have scrolled")
}
func TestModel_CursorDiffLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.diffCursor = 0

	dl, ok := m.cursorDiffLine()
	assert.True(t, ok)
	assert.Equal(t, "line1", dl.Content)
	assert.Equal(t, diff.ChangeContext, dl.ChangeType)

	m.diffCursor = 1
	dl, ok = m.cursorDiffLine()
	assert.True(t, ok)
	assert.Equal(t, "added", dl.Content)
	assert.Equal(t, diff.ChangeAdd, dl.ChangeType)

	// out of bounds
	m.diffCursor = -1
	_, ok = m.cursorDiffLine()
	assert.False(t, ok)

	m.diffCursor = 10
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
	m.diffLines = lines

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
		m.diffLines = mixedLines

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
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
	}

	// no annotations, cursor at 0 -> viewport Y = 0
	m.diffCursor = 0
	assert.Equal(t, 0, m.cursorViewportY())

	// no annotations, cursor at 2 -> viewport Y = 2
	m.diffCursor = 2
	assert.Equal(t, 2, m.cursorViewportY())

	// add annotation on line 1 (index 0), cursor at 2 -> viewport Y = 3 (line0 + annotation + line1)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "comment"})
	m.diffCursor = 2
	assert.Equal(t, 3, m.cursorViewportY())

	// empty file with non-zero cursor returns cursor value directly
	empty := testModel(nil, nil)
	empty.diffCursor = 5
	assert.Equal(t, 5, empty.cursorViewportY(), "empty state returns diffCursor as-is")
}
func TestModel_NextPrevFileWrapAround(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
	})
	m.tree = newFileTree(files)
	m.currFile = "a.go"

	// move to last file
	m.tree.nextFile()
	assert.Equal(t, "b.go", m.tree.selectedFile())
	m.currFile = "b.go"

	// n should wrap to first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(Model)
	assert.Equal(t, "a.go", model.tree.selectedFile())

	// p should wrap to last
	model.currFile = "a.go"
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())
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
	model.focus = paneDiff
	assert.Equal(t, 0, model.diffCursor)

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight, "viewport height must be positive")

	// pgdown should move cursor by page height
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, pageHeight, model.diffCursor, "PgDown should move cursor by viewport height")

	// ctrl+d should move by half page height from current position
	prevCursor := model.diffCursor
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = result.(Model)
	halfPage := pageHeight / 2
	assert.Equal(t, prevCursor+halfPage, model.diffCursor, "ctrl+d should move cursor by half viewport height")
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
	model.focus = paneDiff

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight, "viewport height must be positive")

	// move cursor to line 80 first
	model.diffCursor = 80

	// pgup should move cursor up by page height
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)
	assert.Equal(t, 80-pageHeight, model.diffCursor, "PgUp should move cursor up by viewport height")

	// ctrl+u should move up by half page height
	model.diffCursor = 80
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = result.(Model)
	halfPage := pageHeight / 2
	assert.Equal(t, 80-halfPage, model.diffCursor, "ctrl+u should move cursor up by half viewport height")
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
	model.focus = paneDiff
	assert.Equal(t, 0, model.diffCursor)

	pageHeight := model.viewport.Height
	halfPage := pageHeight / 2
	require.Positive(t, halfPage, "half page must be positive")

	// ctrl+d moves cursor and viewport by half page
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = result.(Model)
	assert.Equal(t, halfPage, model.diffCursor, "ctrl+d should move cursor by half viewport height")
	assert.Equal(t, halfPage, model.viewport.YOffset, "ctrl+d should scroll viewport by half page")

	// PgDn moves full page from start for comparison
	model.diffCursor = 0
	model.viewport.SetYOffset(0)
	model.viewport.SetContent(model.renderDiff())
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, pageHeight, model.diffCursor, "PgDown should move cursor by full viewport height")
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
	model.focus = paneDiff

	pageHeight := model.viewport.Height
	halfPage := pageHeight / 2
	require.Positive(t, halfPage, "half page must be positive")

	// start at line 80 with viewport scrolled to match
	model.diffCursor = 80
	model.viewport.SetYOffset(80)
	model.viewport.SetContent(model.renderDiff())
	prevOffset := model.viewport.YOffset

	// ctrl+u moves cursor and viewport by half page up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = result.(Model)
	assert.Equal(t, 80-halfPage, model.diffCursor, "ctrl+u should move cursor up by half viewport height")
	assert.Equal(t, prevOffset-halfPage, model.viewport.YOffset, "ctrl+u should scroll viewport up by half page")

	// PgUp moves full page up from 80 for comparison
	model.diffCursor = 80
	model.viewport.SetYOffset(80)
	model.viewport.SetContent(model.renderDiff())
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)
	assert.Equal(t, 80-pageHeight, model.diffCursor, "PgUp should move cursor up by full viewport height")
}
func TestModel_TreeCtrlDUMovesHalfPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree
	m.height = 20

	pageSize := m.treePageSize()
	halfPage := max(1, pageSize/2)

	// ctrl+d from start
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", halfPage), model.tree.selectedFile(),
		"ctrl+d should move by half page")

	// ctrl+u from end area
	m3 := testModel(files, nil)
	m3.tree = newFileTree(files)
	m3.focus = paneTree
	m3.height = 20
	// move to file 39
	m3.tree.moveToLast()
	for range 10 {
		m3.tree.moveUp()
	}
	assert.Equal(t, "pkg/file39.go", m3.tree.selectedFile())

	result, _ = m3.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model3 := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", 39-halfPage), model3.tree.selectedFile(),
		"ctrl+u should move by half page")

	// PgDn from start should move full page
	m2 := testModel(files, nil)
	m2.tree = newFileTree(files)
	m2.focus = paneTree
	m2.height = 20

	result, _ = m2.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model2 := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", pageSize), model2.tree.selectedFile(),
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
	model.focus = paneDiff

	// move to middle
	model.diffCursor = 25

	// end should move to last line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = result.(Model)
	assert.Equal(t, 49, model.diffCursor, "End should move cursor to last line")

	// home should move to first line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor, "Home should move cursor to first line")
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
	model.focus = paneDiff

	// home should skip leading divider
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = result.(Model)
	assert.Equal(t, 1, model.diffCursor, "Home should skip leading divider")

	// end should skip trailing divider
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = result.(Model)
	assert.Equal(t, 2, model.diffCursor, "End should skip trailing divider")
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
	model.focus = paneDiff

	// pgdown when there are fewer lines than page height should clamp at last line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, 9, model.diffCursor, "PgDown should clamp at last line")
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
	model.focus = paneDiff
	model.diffCursor = 3

	// pgup from line 3 with large page height should clamp at first line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor, "PgUp should clamp at first line")
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
	model.focus = paneDiff
	assert.Equal(t, 0, model.diffCursor)

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown with dividers: cursor traverses fewer non-divider lines than viewport height
	// because divider rows consume visual space without being cursor-selectable
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Positive(t, model.diffCursor, "cursor should have moved forward")

	nonDividerCount := 0
	for i := range model.diffCursor {
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
	model.focus = paneDiff
	assert.Equal(t, 0, model.viewport.YOffset)

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown should scroll viewport by approximately a full page (not just 1 line)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, pageHeight, model.viewport.YOffset,
		"viewport should scroll to cursor position (full page), not just 1 line")

	// second pgdown should scroll another full page
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = result.(Model)
	assert.Equal(t, 2*pageHeight, model.viewport.YOffset,
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
	model.focus = paneDiff

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight)

	// move cursor to line 100
	model.diffCursor = 100
	model.syncViewportToCursor()

	// pgup should scroll viewport back by approximately a full page
	prevOffset := model.viewport.YOffset
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = result.(Model)

	// viewport should scroll back significantly, not just 1 line
	scrolled := prevOffset - model.viewport.YOffset
	assert.GreaterOrEqual(t, scrolled, pageHeight-1,
		"viewport should scroll back by approximately a full page")
}
func TestModel_TreePgDownMovesCursorByPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree
	m.height = 20

	// cursor starts on first file
	assert.Equal(t, "pkg/file00.go", m.tree.selectedFile())

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)

	// PgDown should advance cursor by page size files
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", pageSize), model.tree.selectedFile(),
		"PgDown in tree should move cursor by page size")
}
func TestModel_TreePgUpMovesCursorByPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree
	m.height = 20

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)

	// move cursor to the last file, then back 10 — lands on file39
	m.tree.moveToLast()
	for range 10 {
		m.tree.moveUp()
	}
	assert.Equal(t, "pkg/file39.go", m.tree.selectedFile())

	// PgUp should move back by page size files
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model := result.(Model)
	expected := fmt.Sprintf("pkg/file%02d.go", 39-pageSize)
	assert.Equal(t, expected, model.tree.selectedFile(), "PgUp in tree should move cursor by page size")
}
func TestModel_TreeCtrlDMovesCursorByHalfPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree
	m.height = 20

	assert.Equal(t, "pkg/file00.go", m.tree.selectedFile())

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)
	halfPage := max(1, pageSize/2)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", halfPage), model.tree.selectedFile(),
		"ctrl+d in tree should move cursor by half page size")
}
func TestModel_TreeCtrlUMovesCursorByHalfPage(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree
	m.height = 20

	pageSize := m.treePageSize()
	require.Positive(t, pageSize)
	halfPage := max(1, pageSize/2)

	m.tree.moveToLast()
	for range 10 {
		m.tree.moveUp()
	}
	assert.Equal(t, "pkg/file39.go", m.tree.selectedFile())

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model := result.(Model)
	expected := fmt.Sprintf("pkg/file%02d.go", 39-halfPage)
	assert.Equal(t, expected, model.tree.selectedFile(), "ctrl+u in tree should move cursor by half page size")
}
func TestModel_TreeHomeEndMoveToBoundaries(t *testing.T) {
	files := []string{"cmd/main.go", "internal/a.go", "internal/b.go", "internal/c.go", "pkg/util.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree

	// move to middle
	m.tree.moveDown()
	m.tree.moveDown()
	assert.NotEqual(t, "cmd/main.go", m.tree.selectedFile())

	// end should move to last file
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model := result.(Model)
	assert.Equal(t, "pkg/util.go", model.tree.selectedFile(), "End in tree should move to last file")

	// home should move to first file
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = result.(Model)
	assert.Equal(t, "cmd/main.go", model.tree.selectedFile(), "Home in tree should move to first file")
}
func TestModel_TreeScrollOffsetPersistsAcrossUpdates(t *testing.T) {
	// many files so tree needs scrolling at the given height
	files := make([]string, 30)
	for i := range files {
		files[i] = fmt.Sprintf("pkg/file%02d.go", i)
	}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree
	m.height = 10 // visible tree height = 10-3 = 7 rows

	// scroll down past the visible window via repeated Update calls
	var result tea.Model
	for range 15 {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = result.(Model)
	}
	// cursor should be well past the initial visible window
	offsetAfterDown := m.tree.offset
	assert.Positive(t, offsetAfterDown, "offset should be non-zero after scrolling past visible area")

	// move up one step, offset should stay stable (not jump to 0)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(Model)
	assert.Equal(t, offsetAfterDown, m.tree.offset,
		"offset should remain stable when moving cursor up within the visible window")
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
			m.diffLines = tc.lines
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
			m.diffLines = lines
			m.diffCursor = tc.cursor
			hunk, total := m.currentHunk()
			assert.Equal(t, tc.wantHunk, hunk)
			assert.Equal(t, tc.wantTotal, total)
		})
	}
}
func TestModel_CurrentHunkNoChanges(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = []diff.DiffLine{
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
	m.diffLines = lines
	m.diffCursor = 0
	m.currFile = "a.go"
	m.viewport.Height = 20

	m.moveToNextHunk()
	assert.Equal(t, 1, m.diffCursor, "should jump to hunk 1")

	m.moveToNextHunk()
	assert.Equal(t, 3, m.diffCursor, "should jump to hunk 2")

	// at last chunk, should not move
	m.moveToNextHunk()
	assert.Equal(t, 3, m.diffCursor, "should stay at last chunk")
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
	m.diffLines = lines
	m.diffCursor = 4
	m.currFile = "a.go"
	m.viewport.Height = 20

	m.moveToPrevHunk()
	assert.Equal(t, 3, m.diffCursor, "should jump to hunk 2")

	m.moveToPrevHunk()
	assert.Equal(t, 1, m.diffCursor, "should jump to hunk 1")

	// at first chunk, should not move
	m.moveToPrevHunk()
	assert.Equal(t, 1, m.diffCursor, "should stay at first chunk")
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
	m.focus = paneDiff
	vpHeight := m.viewport.Height
	require.Positive(t, vpHeight)

	m.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 20, m.diffCursor, "cursor should land on first line of hunk")

	// hunk midpoint: cursorY + hunkHeight/2 = 20 + 1 = 21
	// offset: midY - vpHeight/2
	expectedOffset := 21 - vpHeight/2
	assert.Equal(t, expectedOffset, m.viewport.YOffset, "small hunk should be centered by its midpoint")
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
	m.focus = paneDiff

	m.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 10, m.diffCursor, "cursor should land on first line of hunk")

	// hunk is 100 lines >> viewport, so offset = cursorY - 2 = 10 - 2 = 8
	assert.Equal(t, 8, m.viewport.YOffset, "large hunk should place first line near top with context margin")
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
	m.focus = paneDiff

	m.diffCursor = 10
	m.moveToPrevHunk()
	assert.Equal(t, 0, m.diffCursor, "cursor should land on first line")
	assert.Equal(t, 0, m.viewport.YOffset, "offset should be clamped to 0 when hunk is near top")
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
	m.focus = paneDiff
	vpHeight := m.viewport.Height

	m.diffCursor = 40
	m.syncViewportToCursor()
	m.moveToPrevHunk()
	assert.Equal(t, 25, m.diffCursor, "cursor should land on first line of hunk")

	// hunk midpoint: 25 + 2/2 = 26, offset: 26 - vpHeight/2
	expectedOffset := 26 - vpHeight/2
	assert.Equal(t, expectedOffset, m.viewport.YOffset, "prevHunk should center the hunk by its midpoint")
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
	m.focus = paneDiff
	vpHeight := m.viewport.Height

	m.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 25, m.diffCursor)

	// single-line hunk: midY = 25 + 0 = 25, offset = 25 - vpHeight/2
	expectedOffset := 25 - vpHeight/2
	assert.Equal(t, expectedOffset, m.viewport.YOffset, "single-line hunk midpoint centered in viewport")
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
		diff.DiffLine{NewNum: 21, Content: "short add", ChangeType: diff.ChangeAdd},                      // idx 20
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
	m.focus = paneDiff
	m.wrapMode = true
	vpHeight := m.viewport.Height
	require.Positive(t, vpHeight)

	m.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 20, m.diffCursor, "cursor should land on first line of hunk")

	// compute expected offset using the same helpers the production code uses
	wrapCount0 := m.wrappedLineCount(20)
	wrapCount1 := m.wrappedLineCount(21)
	require.Greater(t, wrapCount1, 1, "long line should wrap to multiple rows")
	hunkVisualHeight := wrapCount0 + wrapCount1
	cursorY := m.cursorViewportY()
	hunkMidY := cursorY + hunkVisualHeight/2
	expectedOffset := max(0, hunkMidY-vpHeight/2)
	assert.Equal(t, expectedOffset, m.viewport.YOffset, "wrap mode: offset should account for wrapped line height")
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
	m.focus = paneDiff
	vpHeight := m.viewport.Height

	// add an annotation on the first changed line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 26, Type: "+", Comment: "review note"})
	defer m.store.Delete("a.go", 26, "+")

	m.diffCursor = 0
	m.moveToNextHunk()
	assert.Equal(t, 25, m.diffCursor, "cursor should land on first line of hunk")

	// expected: hunk height = 2 changed lines + annotation visual rows
	annotKey := m.annotationKey(26, "+")
	annotHeight := m.wrappedAnnotationLineCount(annotKey)
	require.Positive(t, annotHeight, "annotation should contribute visual rows")
	hunkVisualHeight := 2 + annotHeight // 2 changed lines + annotation
	cursorY := m.cursorViewportY()
	hunkMidY := cursorY + hunkVisualHeight/2
	expectedOffset := max(0, hunkMidY-vpHeight/2)
	assert.Equal(t, expectedOffset, m.viewport.YOffset, "annotation: offset should include annotation visual rows")
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
	m.focus = paneDiff

	// position cursor on a context line and set a known viewport offset
	m.diffCursor = 10
	m.viewport.SetYOffset(5)
	m.viewport.SetContent(m.renderDiff())
	beforeOffset := m.viewport.YOffset

	m.centerHunkInViewport()
	assert.Equal(t, beforeOffset, m.viewport.YOffset, "should be no-op when cursor is on context line")
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
	m.focus = paneDiff
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	vpHeight := m.viewport.Height

	m.diffCursor = 0
	m.moveToNextHunk()
	// in collapsed mode, cursor lands on first visible line (first add, idx 23)
	assert.Equal(t, 23, m.diffCursor, "cursor should skip hidden removes and land on first add")

	// hunk visual height = 3 visible add lines (removes are hidden)
	cursorY := m.cursorViewportY()
	hunkMidY := cursorY + 3/2
	expectedOffset := max(0, hunkMidY-vpHeight/2)
	assert.Equal(t, expectedOffset, m.viewport.YOffset, "collapsed mode: offset should only count visible lines")
}

func TestModel_HunkNavigationViaKeys(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},  // 0
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},      // 1 - hunk 1
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // 2
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},     // 3 - hunk 2
	}

	m := testModel(nil, nil)
	m.diffLines = lines
	m.diffCursor = 0
	m.currFile = "a.go"
	m.focus = paneDiff
	m.viewport.Height = 20

	// press ] to go to next chunk
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)
	assert.Equal(t, 1, model.diffCursor, "] should jump to first chunk")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model = result.(Model)
	assert.Equal(t, 3, model.diffCursor, "] should jump to second chunk")

	// press [ to go to previous chunk
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model = result.(Model)
	assert.Equal(t, 1, model.diffCursor, "[ should jump back to first chunk")
}
func TestModel_HorizontalScroll(t *testing.T) {
	longLine := "package " + strings.Repeat("x", 200)
	lines := []diff.DiffLine{{OldNum: 1, NewNum: 1, Content: longLine, ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.focus = paneDiff

	assert.Equal(t, 0, m.scrollX)

	// scroll right
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = result.(Model)
	assert.Equal(t, scrollStep, m.scrollX)

	// scroll left back to 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, 0, m.scrollX)

	// scroll left at 0 stays at 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, 0, m.scrollX)
}
func TestModel_HorizontalScrollResetsOnFileLoad(t *testing.T) {
	lines := []diff.DiffLine{{OldNum: 1, NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.scrollX = 20

	// loading new file should reset scrollX
	result, _ = m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	assert.Equal(t, 0, m.scrollX)
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
			m.height = tc.height
			m.noStatusBar = tc.noStatusBar
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
	m.wrapMode = true
	m.width = 60
	m.treeWidth = 12
	m.styles = plainStyles()

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
		m.diffCursor = 0
		m.focus = paneDiff
		m.cursorOnAnnotation = false

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
		m.scrollX = 10
		m.wrapMode = true

		var b strings.Builder
		dl := diff.DiffLine{Content: "@@ -1,3 +1,3 @@", ChangeType: diff.ChangeDivider}
		m.renderDiffLine(&b, 0, dl)

		// divider falls through to non-wrap path but ansi.Cut should be skipped
		output := b.String()
		assert.Contains(t, output, "@@", "divider content should not be scrolled in wrap mode")

		m.scrollX = 0 // reset
	})
}
func TestModel_WrappedLineCount(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "short", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: strings.Repeat("x", 200), ChangeType: diff.ChangeAdd},
		{Content: "@@ -1,3 +1,3 @@", ChangeType: diff.ChangeDivider},
		{OldNum: 3, Content: strings.Repeat("y", 200), ChangeType: diff.ChangeRemove},
	}

	t.Run("wrap off returns 1 for all lines", func(t *testing.T) {
		m.wrapMode = false
		assert.Equal(t, 1, m.wrappedLineCount(0))
		assert.Equal(t, 1, m.wrappedLineCount(1))
		assert.Equal(t, 1, m.wrappedLineCount(2))
		assert.Equal(t, 1, m.wrappedLineCount(3))
	})

	t.Run("wrap on, short line returns 1", func(t *testing.T) {
		m.wrapMode = true
		assert.Equal(t, 1, m.wrappedLineCount(0))
	})

	t.Run("wrap on, long line returns more than 1", func(t *testing.T) {
		m.wrapMode = true
		count := m.wrappedLineCount(1)
		assert.Greater(t, count, 1, "long add line should wrap to multiple visual rows")
	})

	t.Run("wrap on, divider always returns 1", func(t *testing.T) {
		m.wrapMode = true
		assert.Equal(t, 1, m.wrappedLineCount(2))
	})

	t.Run("wrap on, long remove line wraps", func(t *testing.T) {
		m.wrapMode = true
		count := m.wrappedLineCount(3)
		assert.Greater(t, count, 1, "long remove line should wrap to multiple visual rows")
	})

	t.Run("out of bounds returns 1", func(t *testing.T) {
		m.wrapMode = true
		assert.Equal(t, 1, m.wrappedLineCount(-1))
		assert.Equal(t, 1, m.wrappedLineCount(100))
	})
}
func TestModel_CursorViewportYWithWrap(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.wrapMode = true
	// use a narrow width so wrapping is predictable
	m.width = 60
	m.treeWidth = 20

	// diffContentWidth = 60 - 20 - 4 - 1 = 35
	// wrapWidth = 35 - 3 (gutter) = 32

	m.diffLines = []diff.DiffLine{
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
		m.diffCursor = 0
		m.cursorOnAnnotation = false
		assert.Equal(t, 0, m.cursorViewportY())
	})

	t.Run("cursor at 1, after short line 0", func(t *testing.T) {
		m.diffCursor = 1
		m.cursorOnAnnotation = false
		assert.Equal(t, count0, m.cursorViewportY())
	})

	t.Run("cursor at 2, after wrapped line 1", func(t *testing.T) {
		m.diffCursor = 2
		m.cursorOnAnnotation = false
		assert.Equal(t, count0+count1, m.cursorViewportY())
	})

	t.Run("cursor at 3, after lines 0+1+2", func(t *testing.T) {
		m.diffCursor = 3
		m.cursorOnAnnotation = false
		assert.Equal(t, count0+count1+count2, m.cursorViewportY())
	})

	t.Run("cursor on annotation after wrapped line", func(t *testing.T) {
		// add annotation on line 2 (idx 1, the long wrapped add line)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "note"})
		defer func() { m.store.Delete("a.go", 2, "+") }()

		m.diffCursor = 2
		m.cursorOnAnnotation = false
		// cursor at line 2: count0 + count1 + 1 (annotation row after line 1)
		assert.Equal(t, count0+count1+1, m.cursorViewportY())
	})

	t.Run("cursor on annotation sub-line of wrapped line", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: " ", Comment: "note on ctx"})
		defer func() { m.store.Delete("a.go", 3, " ") }()

		m.diffCursor = 2
		m.cursorOnAnnotation = true
		// on annotation sub-line of line 2: offset is line0 + line1 rows + wrappedLineCount(2)
		assert.Equal(t, count0+count1+count2, m.cursorViewportY())
	})
}
func TestModel_CursorViewportYWithWrapDeletePlaceholder(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.wrapMode = true
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.width = 60
	m.treeWidth = 20

	// diffContentWidth = 60 - 20 - 4 - 1 = 35, wrapWidth = 35 - 3 = 32
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "context line", ChangeType: diff.ChangeContext},
		{OldNum: 1, Content: strings.Repeat("x", 80), ChangeType: diff.ChangeRemove}, // long remove, hunk start
		{OldNum: 2, Content: strings.Repeat("y", 80), ChangeType: diff.ChangeRemove}, // long remove
		{OldNum: 3, Content: strings.Repeat("z", 80), ChangeType: diff.ChangeRemove}, // long remove
		{NewNum: 2, Content: "after context", ChangeType: diff.ChangeContext},
	}

	// placeholder text "⋯ 3 lines deleted" is short (~17 chars), fits in 1 row at wrapWidth=32.
	// the original removed lines are 80 chars each and would wrap to ~3 rows.
	// cursorViewportY must use placeholder text (1 row), not original content (~3 rows).

	m.diffCursor = 4 // cursor on "after context" line
	m.cursorOnAnnotation = false
	m.focus = paneDiff

	y := m.cursorViewportY()
	// expected: 1 (context) + 1 (placeholder = 1 visual row) = 2
	assert.Equal(t, 2, y, "viewport Y should count placeholder as 1 row, not original line content")
}
func TestModel_CursorViewportYWithWrapDeletePlaceholderAndBlame(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.wrapMode = true
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.showBlame = true
	m.blameData = map[int]diff.BlameLine{
		1: {Author: "LongName", Time: time.Now()},
	}
	m.blameAuthorLen = m.computeBlameAuthorLen()
	m.width = 25
	m.treeHidden = true

	m.diffLines = []diff.DiffLine{
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

	m.diffCursor = 4
	m.cursorOnAnnotation = false

	assert.Equal(t, contextRows+placeholderRows, m.cursorViewportY())
}
func TestModel_WrapToggle(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = []string{"x"}
	m.focus = paneDiff
	m.viewport.Width = 80
	m.viewport.Height = 20
	assert.False(t, m.wrapMode)

	// press w to enable wrap
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.True(t, model.wrapMode)

	// press w again to disable wrap
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.wrapMode)
}
func TestModel_WrapToggleResetsScrollX(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = []string{"x"}
	m.focus = paneDiff
	m.viewport.Width = 80
	m.viewport.Height = 20
	m.scrollX = 10

	// enable wrap: scrollX should reset to 0
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.True(t, model.wrapMode)
	assert.Equal(t, 0, model.scrollX)
}
func TestModel_WrapToggleNoOpWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = ""
	assert.False(t, m.wrapMode)

	// w should be no-op without a loaded file
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.False(t, model.wrapMode)
}
func TestModel_WrapToggleNoOpInTreePane(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneTree
	assert.False(t, m.wrapMode)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model := result.(Model)
	assert.False(t, model.wrapMode)
}
func TestModel_ScrollBlockedInWrapMode(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = []string{"x"}
	m.focus = paneDiff
	m.viewport.Width = 80
	m.viewport.Height = 20
	m.wrapMode = true
	m.scrollX = 0

	// right key should not change scrollX in wrap mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := result.(Model)
	assert.Equal(t, 0, model.scrollX)

	// left key should not change scrollX in wrap mode
	model.scrollX = 0
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = result.(Model)
	assert.Equal(t, 0, model.scrollX)
}
func TestModel_ScrollWorksWithoutWrapMode(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = []string{"x"}
	m.focus = paneDiff
	m.viewport.Width = 80
	m.viewport.Height = 20
	m.wrapMode = false
	m.scrollX = 0

	// right key should scroll in non-wrap mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := result.(Model)
	assert.Positive(t, model.scrollX)
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
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)
		m.currFile = "README.md"
		m.diffLines = mdLines
		m.highlightedLines = make([]string, len(mdLines))
		m.focus = paneDiff
		m.diffCursor = 0

		// TOC entries: [0]=README.md(0), [1]=First(0), [2]=Second(2), [3]=Third(4)
		// move down one line at a time and check active section
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 1, model.mdTOC.activeSection, "cursor at line 1 should be in First section")

		// move to line 2 (## Second)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		assert.Equal(t, 2, model.mdTOC.activeSection, "cursor at line 2 should be in Second section")

		// move to line 4 (### Third)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
		assert.Equal(t, 3, model.mdTOC.activeSection, "cursor at line 4 should be in Third section")
	})

	t.Run("no TOC does not crash on scroll", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.mdTOC = nil
		m.currFile = "main.go"
		m.diffLines = []diff.DiffLine{{Content: "package main", ChangeType: diff.ChangeContext}}
		m.highlightedLines = []string{"package main"}
		m.focus = paneDiff
		m.diffCursor = 0

		// should not panic
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Nil(t, model.mdTOC)
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
	m.diffLines = lines
	m.diffCursor = 0
	m.currFile = "a.go"
	m.focus = paneDiff
	m.viewport.Height = 20

	// "x" should jump to next hunk
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)
	assert.Equal(t, 1, model.diffCursor, "x should jump to first hunk")

	// "]" should not jump (unbound)
	model.diffCursor = 0
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor, "] should not jump when unbound")
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
	m.pendingHunkJump = &fwd
	m.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.loadSeq, lines: bLines}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "pendingHunkJump should be cleared after file load")
	assert.Equal(t, 2, model.diffCursor, "cursor should land on first hunk (index 2, the add line)")
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
	m.pendingHunkJump = &bwd
	m.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.loadSeq, lines: bLines}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "pendingHunkJump should be cleared after file load")
	assert.Equal(t, 4, model.diffCursor, "cursor should land on last hunk (index 4, the second add line)")
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
	m.pendingHunkJump = &fwd
	m.focus = paneTree
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Nil(t, model.pendingHunkJump, "pendingHunkJump should be cleared on manual tree navigation")
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
	m.viewport.Height = 20
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
	m.focus = paneTree
	m.diffCursor = 0 // on context line

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Equal(t, paneDiff, model.focus, "] from tree pane should switch focus to diff")
	assert.Equal(t, 1, model.diffCursor, "] should land on the add line (hunk start)")
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
	m.focus = paneTree
	m.diffCursor = 2 // on second add line

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := result.(Model)

	assert.Equal(t, paneDiff, model.focus, "[ from tree pane should switch focus to diff")
	assert.Equal(t, 0, model.diffCursor, "[ should land on first hunk start (index 0)")
}
func TestModel_HunkNav_NextCrossesFileForward(t *testing.T) {
	// pressing ] at the last hunk of a.go should navigate to b.go and set pendingHunkJump=true
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.crossFileHunks = true
	m.focus = paneDiff
	m.diffCursor = 0 // at the only (last) hunk

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	require.NotNil(t, model.pendingHunkJump, "pendingHunkJump should be set for cross-file forward jump")
	assert.True(t, *model.pendingHunkJump, "pendingHunkJump should be true (land on first hunk)")
	assert.Equal(t, "b.go", model.tree.selectedFile(), "tree should have advanced to b.go")
	assert.NotNil(t, cmd, "a load command should be returned")
}
func TestModel_HunkNav_PrevCrossesFileBackward(t *testing.T) {
	// pressing [ at the first hunk of b.go should navigate to a.go and set pendingHunkJump=false
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.crossFileHunks = true
	// tree: index 0 = dir entry, index 1 = a.go, index 2 = b.go
	m.tree.cursor = 2
	m.loadSeq++
	bLoad := m.loadFileDiff("b.go")()
	result, _ := m.Update(bLoad)
	m = result.(Model)
	m.viewport.Height = 20
	m.focus = paneDiff
	m.diffCursor = 0 // at the first (and only) hunk

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := result.(Model)

	require.NotNil(t, model.pendingHunkJump, "pendingHunkJump should be set for cross-file backward jump")
	assert.False(t, *model.pendingHunkJump, "pendingHunkJump should be false (land on last hunk)")
	assert.Equal(t, "a.go", model.tree.selectedFile(), "tree should have moved back to a.go")
	assert.NotNil(t, cmd, "a load command should be returned")
}
func TestModel_HunkNav_NextAtLastFileNoOp(t *testing.T) {
	// pressing ] at the last hunk of the last file: no-op
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.focus = paneDiff
	m.diffCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "no pendingHunkJump when no next file")
	assert.Equal(t, 0, model.diffCursor, "cursor should stay at last hunk")
	assert.Equal(t, "a.go", model.currFile, "should remain on a.go")
}
func TestModel_HunkNav_PrevAtFirstFileNoOp(t *testing.T) {
	// pressing [ at the first hunk of the first file: no-op
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.focus = paneDiff
	m.diffCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "no pendingHunkJump when no prev file")
	assert.Equal(t, 0, model.diffCursor, "cursor should stay at first hunk")
	assert.Equal(t, "a.go", model.currFile, "should remain on a.go")
}
func TestModel_HunkNav_SingleFileNoCrossFile(t *testing.T) {
	// in single-file mode, ] at last hunk should not cross to other files
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go"}, diffs)
	m.crossFileHunks = true
	m.singleFile = true
	m.treeWidth = 0
	m.focus = paneDiff
	m.diffCursor = 0

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "single-file mode should not set pendingHunkJump")
	assert.Equal(t, 0, model.diffCursor, "cursor should not move in single-file mode at last hunk")
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
	m.crossFileHunks = true
	m.focus = paneDiff
	m.diffCursor = 0 // at the only hunk of a.go

	// press ] to trigger cross-file jump
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = result.(Model)
	require.NotNil(t, cmd, "should have a load command")

	// execute the load command and process the result
	loadMsg := cmd()
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "pendingHunkJump should be cleared after landing")
	assert.Equal(t, "b.go", model.currFile, "should have navigated to b.go")
	assert.Equal(t, 2, model.diffCursor, "should land on first hunk of b.go (index 2)")
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
	m.crossFileHunks = true
	// tree: index 0 = dir entry, index 1 = a.go, index 2 = b.go; navigate to b.go
	m.tree.cursor = 2
	m.loadSeq++
	loadMsg := m.loadFileDiff("b.go")()
	result, _ := m.Update(loadMsg)
	m = result.(Model)
	m.viewport.Height = 20
	m.focus = paneDiff
	m.diffCursor = 0 // at first (only) hunk of b.go

	// press [ to trigger cross-file backward jump
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m = result.(Model)
	require.NotNil(t, cmd, "should have a load command")

	// execute the load command and process the result
	loadMsg = cmd()
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump, "pendingHunkJump should be cleared after landing")
	assert.Equal(t, "a.go", model.currFile, "should have navigated to a.go")
	assert.Equal(t, 2, model.diffCursor, "should land on last hunk of a.go (index 2)")
}
func TestModel_HunkNav_DefaultDoesNotCrossFiles(t *testing.T) {
	diffs := map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
		"b.go": {{ChangeType: diff.ChangeAdd, Content: "add", NewNum: 1}},
	}
	m := loadFileIntoModel(t, []string{"a.go", "b.go"}, diffs)
	m.focus = paneDiff
	m.diffCursor = 0

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Nil(t, model.pendingHunkJump)
	assert.Equal(t, "a.go", model.currFile)
	assert.Equal(t, "a.go", model.tree.selectedFile())
	assert.Equal(t, 0, model.diffCursor)
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
	m.pendingHunkJump = &fwd
	m.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.loadSeq, lines: lines}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingHunkJump)
	assert.Equal(t, 1, model.diffCursor, "should fall back to first visible context line")
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
	m.pendingHunkJump = &fwd
	m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1, Type: "+", Comment: "note"}
	m.loadSeq++
	loadMsg := fileLoadedMsg{file: "b.go", seq: m.loadSeq, lines: diffs["b.go"]}
	result, _ = m.Update(loadMsg)
	model := result.(Model)

	assert.Nil(t, model.pendingAnnotJump)
	assert.Nil(t, model.pendingHunkJump)
	assert.Equal(t, 0, model.diffCursor)
}

func TestModel_EnterInDiffPaneOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "--- a/file.go", ChangeType: diff.ChangeDivider},
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0 // on divider line

	// press enter on divider - should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating, "enter on divider should not start annotation")
}

func TestModel_DiffLineNum(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, 5, m.diffLineNum(diff.DiffLine{NewNum: 5, ChangeType: diff.ChangeContext}))
	assert.Equal(t, 3, m.diffLineNum(diff.DiffLine{NewNum: 3, ChangeType: diff.ChangeAdd}))
	assert.Equal(t, 7, m.diffLineNum(diff.DiffLine{OldNum: 7, ChangeType: diff.ChangeRemove}))
}
