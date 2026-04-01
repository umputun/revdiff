package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
	"github.com/umputun/revdiff/diff/mocks"
)

func testModel(files []string, fileDiffs map[string][]diff.DiffLine) Model {
	renderer := &mocks.DiffRendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]string, error) {
			return files, nil
		},
		FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
			return fileDiffs[file], nil
		},
	}
	store := annotation.NewStore()
	m := NewModel(renderer, store, "", false, 3)
	// simulate window size
	m.width = 120
	m.height = 40
	m.treeWidth = m.width * m.treeWidthRatio / 10
	m.ready = true
	return m
}

func TestModel_Init(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	cmd := m.Init()
	require.NotNil(t, cmd)

	// execute the command - should produce filesLoadedMsg
	msg := cmd()
	flm, ok := msg.(filesLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, []string{"a.go", "b.go"}, flm.files)
	assert.NoError(t, flm.err)
}

func TestModel_FilesLoaded(t *testing.T) {
	m := testModel(nil, nil)

	result, cmd := m.Update(filesLoadedMsg{files: []string{"internal/handler.go", "internal/store.go", "main.go"}})
	model := result.(Model)

	// tree should be populated
	assert.Len(t, model.tree.entries, 5) // 2 dirs + 3 files
	assert.NotNil(t, cmd)                // should auto-select first file
}

func TestModel_FilesLoadedError(t *testing.T) {
	m := testModel(nil, nil)
	m.ready = true

	result, cmd := m.Update(filesLoadedMsg{err: assert.AnError})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Empty(t, model.tree.entries)
}

func TestModel_FileLoaded(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})

	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func main() {}", ChangeType: diff.ChangeAdd},
	}

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.Equal(t, "a.go", model.currFile)
	assert.Len(t, model.diffLines, 2)
}

func TestModel_QuitKey(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)

	// cmd should be tea.Quit
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}

func TestModel_QuitPreservesAnnotations(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.store.Add("a.go", 5, "+", "needs review")
	m.store.Add("b.go", 10, " ", "check this")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)

	// verify annotations survive the quit
	model := result.(Model)
	output := model.Store().FormatOutput()
	assert.Contains(t, output, "a.go:5")
	assert.Contains(t, output, "needs review")
	assert.Contains(t, output, "b.go:10")
	assert.Contains(t, output, "check this")
}

func TestModel_QuitNoAnnotationsEmptyOutput(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.Empty(t, model.Store().FormatOutput())
}

func TestModel_EnterSelectsFile(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}},
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.focus = paneTree

	// enter on selected file should trigger file load
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	flm, ok := msg.(fileLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, "a.go", flm.file)
}

func TestModel_TabPaneSwitching(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})

	t.Run("tree to diff when file loaded", func(t *testing.T) {
		m.focus = paneTree
		m.currFile = "a.go"
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus)
	})

	t.Run("diff to tree", func(t *testing.T) {
		m.focus = paneDiff
		m.currFile = "a.go"
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.focus)
	})

	t.Run("stays on tree when no file loaded", func(t *testing.T) {
		m.focus = paneTree
		m.currFile = ""
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.focus)
	})
}

func TestModel_FKeyFilterToggle(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.store.Add("a.go", 1, "+", "test annotation")

	t.Run("toggle filter on and off from tree pane", func(t *testing.T) {
		m.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.True(t, model.tree.filter)

		// only a.go should be visible (1 dir + 1 file)
		fileCount := 0
		for _, e := range model.tree.entries {
			if !e.isDir {
				fileCount++
			}
		}
		assert.Equal(t, 1, fileCount)

		// toggle filter off
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model = result.(Model)
		assert.False(t, model.tree.filter)
	})

	t.Run("works from diff pane", func(t *testing.T) {
		m.focus = paneDiff
		m.tree.filter = false
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.True(t, model.tree.filter)
	})

	t.Run("no-op when no annotations", func(t *testing.T) {
		m2 := testModel([]string{"a.go", "b.go"}, nil)
		m2.tree = newFileTree([]string{"a.go", "b.go"})
		// no annotations added
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.False(t, model.tree.filter, "filter should not toggle when no annotated files")
	})
}

func TestModel_StatusBarFilterHint(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("filter hint shown when annotations exist", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.store.Add("a.go", 1, " ", "note")

		m.focus = paneTree
		view := m.View()
		assert.Contains(t, view, "[f] filter", "tree pane should show filter hint when annotations exist")

		m.focus = paneDiff
		view = m.View()
		assert.Contains(t, view, "[f] filter", "diff pane should show filter hint when annotations exist")
	})

	t.Run("filter hint hidden when no annotations", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines

		m.focus = paneTree
		view := m.View()
		assert.NotContains(t, view, "[f] filter", "tree pane should not show filter hint without annotations")

		m.focus = paneDiff
		view = m.View()
		assert.NotContains(t, view, "[f] filter", "diff pane should not show filter hint without annotations")
	})
}

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

func TestModel_TreeNavigation(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree

	// cursor starts on first file (a.go)
	assert.Equal(t, "a.go", m.tree.selectedFile())

	// j moves down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())

	// k moves up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, "a.go", model.tree.selectedFile())
}

func TestModel_FocusSwitching(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go" // pretend a file is loaded
	m.focus = paneTree

	// l switches to diff pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.focus)

	// h switches back to tree
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = result.(Model)
	assert.Equal(t, paneTree, model.focus)
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

func TestModel_WindowResize(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.ready = false

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	model := result.(Model)

	assert.True(t, model.ready)
	assert.Equal(t, 100, model.width)
	assert.Equal(t, 50, model.height)
	assert.Equal(t, 30, model.treeWidth) // 100 * 3 / 10 = 30
}

func TestModel_TreeWidthRatio(t *testing.T) {
	tests := []struct {
		name          string
		ratio         int
		termWidth     int
		wantTreeWidth int
	}{
		{name: "default ratio 3 of 10", ratio: 3, termWidth: 120, wantTreeWidth: 36},
		{name: "ratio 2 of 10", ratio: 2, termWidth: 120, wantTreeWidth: 24},
		{name: "ratio 5 of 10", ratio: 5, termWidth: 120, wantTreeWidth: 60},
		{name: "min width enforced", ratio: 1, termWidth: 100, wantTreeWidth: 20},
		{name: "invalid ratio defaults to 3", ratio: 0, termWidth: 120, wantTreeWidth: 36},
		{name: "over max ratio defaults to 3", ratio: 15, termWidth: 120, wantTreeWidth: 36},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			renderer := &mocks.DiffRendererMock{
				ChangedFilesFunc: func(string, bool) ([]string, error) { return []string{"a.go"}, nil },
				FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
			}
			m := NewModel(renderer, annotation.NewStore(), "", false, tc.ratio)
			result, _ := m.Update(tea.WindowSizeMsg{Width: tc.termWidth, Height: 40})
			model := result.(Model)
			assert.Equal(t, tc.wantTreeWidth, model.treeWidth)
		})
	}
}

func TestModel_ViewOutput(t *testing.T) {
	m := testModel([]string{"internal/a.go", "internal/b.go"}, nil)
	m.tree = newFileTree([]string{"internal/a.go", "internal/b.go"})
	m.ready = true

	// tree pane focused - should show tree navigation hints
	m.focus = paneTree
	view := m.View()
	assert.Contains(t, view, "a.go")
	assert.Contains(t, view, "b.go")
	assert.Contains(t, view, "quit")
	assert.Contains(t, view, "navigate")

	// diff pane focused - should show diff hints
	m.focus = paneDiff
	view = m.View()
	assert.Contains(t, view, "annotate")
	assert.Contains(t, view, "scroll")
}

func TestModel_ViewNotReady(t *testing.T) {
	m := testModel(nil, nil)
	m.ready = false

	assert.Equal(t, "loading...", m.View())
}

func TestModel_AnnotatedFilesMarker(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.store.Add("a.go", 1, "+", "test")

	annotated := m.annotatedFiles()
	assert.True(t, annotated["a.go"])
	assert.False(t, annotated["b.go"])
}

func TestModel_RenderDiffEmpty(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = nil
	assert.Contains(t, m.renderDiff(), "no changes")
}

func TestModel_RenderDiffLines(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
		{OldNum: 3, Content: "func bar() {}", ChangeType: diff.ChangeRemove},
		{Content: "~~~", ChangeType: diff.ChangeDivider},
	}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "package main")
	assert.Contains(t, rendered, "func foo()")
	assert.Contains(t, rendered, "func bar()")
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

func TestModel_FileLoadedResetsCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.diffCursor = 5 // simulate cursor was elsewhere

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor) // cursor reset to first line
}

func TestModel_AnnotateKey(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// press 'a' - should enter annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.True(t, model.annotating)
	assert.NotNil(t, cmd) // textinput blink command
}

func TestModel_EnterInDiffPaneStartsAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1

	// press enter in diff pane - should enter annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annotating, "enter in diff pane should start annotation mode")
	assert.NotNil(t, cmd, "should return textinput blink command")
	assert.Equal(t, paneDiff, model.focus, "focus should remain on diff pane")
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

func TestModel_StatusBarShowsEnterAnnotateHint(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff

	view := m.View()
	assert.Contains(t, view, "[enter/a] annotate", "diff pane status bar should show enter/a annotate hint")
}

func TestModel_AnnotateEnterSaves(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// enter annotation mode and set text
	m.startAnnotation()
	m.annotateInput.SetValue("test comment")

	// press Enter - should save
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "test comment", anns[0].Comment)
	assert.Equal(t, 5, anns[0].Line)
	assert.Equal(t, diff.ChangeContext, anns[0].Type)
}

func TestModel_AnnotateEnterEmptyTextCancels(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	// don't set any text, press Enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_AnnotateEscCancels(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	m.annotateInput.SetValue("should not be saved")

	// press Esc - should cancel without saving
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.annotating)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_AnnotateOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// press 'a' on divider - should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.False(t, model.annotating)
}

func TestModel_AnnotateOnAddLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 3, Content: "new line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	m.annotateInput.SetValue("needs review")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 3, anns[0].Line)
	assert.Equal(t, diff.ChangeAdd, anns[0].Type)
}

func TestModel_AnnotateOnRemoveLine(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 7, Content: "removed line", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	m.annotateInput.SetValue("why removed?")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 7, anns[0].Line)
	assert.Equal(t, diff.ChangeRemove, anns[0].Type)
}

func TestModel_AnnotatePreFillsExisting(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.store.Add("a.go", 1, " ", "old comment")

	m.startAnnotation()
	assert.Equal(t, "old comment", m.annotateInput.Value())
}

func TestModel_DeleteAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.cursorOnAnnotation = true // cursor on the annotation sub-line
	m.store.Add("a.go", 1, " ", "test comment")

	// press 'd' - should delete annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_DeleteAnnotationOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// press 'd' on divider - should not panic
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	_ = result.(Model)
}

func TestModel_DeleteAnnotationNoAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// no annotation exists, 'd' should be harmless
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_RenderDiffWithAnnotations(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
	}
	m.store.Add("a.go", 2, "+", "needs error handling")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "needs error handling")
	assert.Contains(t, rendered, "\U0001f4ac")
}

func TestModel_RenderDiffAnnotationInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.diffCursor = 0
	m.focus = paneDiff
	m.startAnnotation()
	m.annotateInput.SetValue("typing...")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "typing...")
	assert.Contains(t, rendered, "\U0001f4ac")
}

func TestModel_AnnotationSummary(t *testing.T) {
	m := testModel(nil, nil)
	m.store.Add("internal/a.go", 1, "+", "first comment")
	m.store.Add("b.go", 5, " ", "second comment")

	summary := m.renderAnnotationSummary(40)
	assert.Contains(t, summary, "annotations")
	assert.Contains(t, summary, "a.go:1")
	assert.Contains(t, summary, "b.go:5")
	assert.Contains(t, summary, "first comment")
	assert.Contains(t, summary, "second comment")
}

func TestModel_AnnotationSummaryFileLevelAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.store.Add("handler.go", 0, "", "file-level note")
	m.store.Add("handler.go", 5, "+", "line note")

	summary := m.renderAnnotationSummary(40)
	assert.Contains(t, summary, "handler.go (file)", "file-level annotation should show (file) instead of :0")
	assert.Contains(t, summary, "handler.go:5", "line annotation should show normal line number")
	assert.NotContains(t, summary, "handler.go:0", "should not show :0 for file-level annotations")
}

func TestModel_AnnotationSummaryEmpty(t *testing.T) {
	m := testModel(nil, nil)
	summary := m.renderAnnotationSummary(30)
	assert.Empty(t, summary)
}

func TestModel_AnnotationSummaryTruncatesLongComments(t *testing.T) {
	m := testModel(nil, nil)
	m.store.Add("a.go", 1, "+", "this is a very long comment that should be truncated because it exceeds the width limit")

	summary := m.renderAnnotationSummary(20)
	assert.Contains(t, summary, "...")
}

func TestModel_AnnotateStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.focus = paneDiff
	m.annotating = true

	view := m.View()
	assert.Contains(t, view, "save")
	assert.Contains(t, view, "cancel")
	assert.NotContains(t, view, "annotate")
}

func TestModel_AnnotateKeysBlockedInTreePane(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneTree
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}

	// 'a' in tree pane should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.False(t, model.annotating)
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
	m.store.Add("a.go", 1, " ", "comment")
	m.diffCursor = 2
	assert.Equal(t, 3, m.cursorViewportY())

	// empty file with non-zero cursor returns cursor value directly
	empty := testModel(nil, nil)
	empty.diffCursor = 5
	assert.Equal(t, 5, empty.cursorViewportY(), "empty state returns diffCursor as-is")
}

func TestModel_DiffLineNum(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, 5, m.diffLineNum(diff.DiffLine{NewNum: 5, ChangeType: diff.ChangeContext}))
	assert.Equal(t, 3, m.diffLineNum(diff.DiffLine{NewNum: 3, ChangeType: diff.ChangeAdd}))
	assert.Equal(t, 7, m.diffLineNum(diff.DiffLine{OldNum: 7, ChangeType: diff.ChangeRemove}))
}

func TestModel_FileLoadedDiscardsStaleResponse(t *testing.T) {
	// simulate rapid n/n where second load completes first, then stale first response arrives
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// user presses n twice: first for b.go (seq=1), then for c.go (seq=2)
	m.loadSeq = 2
	m.pendingFile = "c.go"
	m.tree.nextFile() // -> b.go
	m.tree.nextFile() // -> c.go

	// c.go response arrives first with latest seq - accepted
	cLines := []diff.DiffLine{{NewNum: 1, Content: "package c", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "c.go", seq: 2, lines: cLines})
	model := result.(Model)
	assert.Equal(t, "c.go", model.currFile)

	// stale b.go response arrives later with old seq - should be discarded
	bLines := []diff.DiffLine{{NewNum: 1, Content: "package b", ChangeType: diff.ChangeContext}}
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: 1, lines: bLines})
	model = result.(Model)
	assert.Equal(t, "c.go", model.currFile, "stale response should not overwrite current file")
	assert.Equal(t, cLines, model.diffLines, "stale response should not overwrite diff lines")
}

func TestModel_FileLoadedAcceptedAfterCursorMove(t *testing.T) {
	// simulate: user presses n to load b.go (seq=1), then j/k moves cursor to c.go before response arrives.
	// the response for b.go should still be accepted because it carries the latest sequence number.
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// user presses n to load b.go
	m.loadSeq = 1
	m.pendingFile = "b.go"
	m.tree.nextFile() // cursor -> b.go

	// then j/k moves cursor to c.go (without triggering a load)
	m.tree.moveDown() // cursor -> c.go
	assert.Equal(t, "c.go", m.tree.selectedFile(), "cursor moved to c.go")

	// b.go response arrives with matching seq - should be accepted
	bLines := []diff.DiffLine{{NewNum: 1, Content: "package b", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "b.go", seq: 1, lines: bLines})
	model := result.(Model)
	assert.Equal(t, "b.go", model.currFile, "response should be accepted despite cursor being on c.go")
	assert.Equal(t, bLines, model.diffLines)
}

func TestModel_FileLoadedStaleErrorDiscarded(t *testing.T) {
	// stale error responses should also be discarded, not overwrite the current diff
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// load a.go successfully (seq=1)
	m.loadSeq = 1
	m.pendingFile = "a.go"
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)

	// user navigates to b.go (seq=2)
	model.loadSeq = 2
	model.pendingFile = "b.go"
	model.tree.nextFile()

	// stale error for a.go arrives with old seq - should be discarded
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, err: errors.New("stale error")})
	model = result.(Model)
	assert.Equal(t, "a.go", model.currFile, "stale error should not change current file")
	assert.Equal(t, aLines, model.diffLines, "stale error should not clear diff lines")
}

func TestModel_SameFileDuplicateLoadDiscarded(t *testing.T) {
	// pressing enter twice on the same file issues two loads for a.go.
	// the older response (seq=1) must be discarded even though it's for the same file.
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// first enter on a.go (seq=1), then another enter on a.go (seq=2)
	m.loadSeq = 2
	m.pendingFile = "a.go"

	// newer response arrives first (seq=2)
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 2, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)
	assert.Equal(t, aLines, model.diffLines)

	// stale response for the same file arrives later with old seq - must be discarded
	staleLines := []diff.DiffLine{{NewNum: 1, Content: "stale data", ChangeType: diff.ChangeContext}}
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: staleLines})
	model = result.(Model)
	assert.Equal(t, aLines, model.diffLines, "stale same-file response should not overwrite newer data")

	// stale error for the same file should also be discarded
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, err: errors.New("stale error")})
	model = result.(Model)
	assert.Equal(t, aLines, model.diffLines, "stale same-file error should not overwrite newer data")
}

func TestModel_FilterRefreshedAfterAnnotationSave(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.store.Add("a.go", 1, "+", "initial annotation")

	// enable filter - should show only a.go
	annotated := m.annotatedFiles()
	m.tree.toggleFilter(annotated)
	assert.True(t, m.tree.filter)

	fileCount := 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 1, fileCount, "only a.go should be visible")

	// add annotation to b.go via saveAnnotation
	m.currFile = "b.go"
	m.diffLines = []diff.DiffLine{{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext}}
	m.diffCursor = 0
	m.startAnnotation()
	m.annotateInput.SetValue("new annotation")
	m.saveAnnotation()

	// after save, filter should be refreshed and b.go should be visible
	fileCount = 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount, "both a.go and b.go should be visible after adding annotation")
}

func TestModel_FilterRefreshedAfterAnnotationDelete(t *testing.T) {
	files := []string{"a.go", "b.go"}
	diffs := map[string][]diff.DiffLine{
		"b.go": {{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext}},
	}
	m := testModel(files, diffs)
	m.tree = newFileTree(files)
	m.store.Add("a.go", 1, " ", "annotation on a")
	m.store.Add("b.go", 5, " ", "annotation on b")

	// enable filter - should show both annotated files
	annotated := m.annotatedFiles()
	m.tree.toggleFilter(annotated)
	assert.True(t, m.tree.filter)

	// delete the annotation on a.go via deleteAnnotation
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, OldNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.diffCursor = 0
	m.cursorOnAnnotation = true
	cmd := m.deleteAnnotation()

	// after delete, filter should be refreshed and only b.go should be visible
	fileCount := 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 1, fileCount, "only b.go should be visible after deleting a.go annotation")

	// should return a command to load the new selection (b.go)
	require.NotNil(t, cmd, "should trigger file load for new tree selection")
	assert.Equal(t, "b.go", m.pendingFile, "pendingFile should be set to new tree selection")
	assert.Equal(t, uint64(1), m.loadSeq, "loadSeq should be incremented to invalidate in-flight loads")
}

func TestModel_FilterDisabledWhenLastAnnotationDeleted(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.store.Add("a.go", 1, " ", "only annotation")

	// enable filter
	annotated := m.annotatedFiles()
	m.tree.toggleFilter(annotated)
	assert.True(t, m.tree.filter)

	// delete the last annotation
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, OldNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.diffCursor = 0
	m.cursorOnAnnotation = true
	cmd := m.deleteAnnotation()

	// filter should be disabled since no annotated files remain
	assert.False(t, m.tree.filter, "filter should be disabled when no annotated files remain")

	fileCount := 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount, "all files should be visible")

	// when filter switches back to all-files, cursor lands on a.go (first file) which matches currFile,
	// so no file load command is needed
	assert.Nil(t, cmd, "no file load needed when filter switches back to all-files and cursor stays on same file")
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

func TestModel_AnnotationsPersistAcrossFileSwitch(t *testing.T) {
	linesA := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}, {NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd}}
	linesB := []diff.DiffLine{{NewNum: 1, Content: "b-line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": linesA, "b.go": linesB})
	m.tree = newFileTree([]string{"a.go", "b.go"})

	// load file a.go
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: linesA})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)

	// add annotation on a.go
	model.focus = paneDiff
	model.diffCursor = 1
	model.startAnnotation()
	model.annotateInput.SetValue("fix this in a.go")
	model.saveAnnotation()

	// navigate tree to b.go and load it
	model.tree.nextFile()
	assert.Equal(t, "b.go", model.tree.selectedFile())
	result, _ = model.Update(fileLoadedMsg{file: "b.go", lines: linesB})
	model = result.(Model)
	assert.Equal(t, "b.go", model.currFile)

	// add annotation on b.go
	model.focus = paneDiff
	model.diffCursor = 0
	model.startAnnotation()
	model.annotateInput.SetValue("check b.go")
	model.saveAnnotation()

	// navigate tree back to a.go and load it
	model.tree.prevFile()
	assert.Equal(t, "a.go", model.tree.selectedFile())
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: linesA})
	model = result.(Model)

	// both annotations should still exist
	annsA := model.store.Get("a.go")
	require.Len(t, annsA, 1)
	assert.Equal(t, "fix this in a.go", annsA[0].Comment)

	annsB := model.store.Get("b.go")
	require.Len(t, annsB, 1)
	assert.Equal(t, "check b.go", annsB[0].Comment)

	// rendered diff for a.go should show annotation
	rendered := model.renderDiff()
	assert.Contains(t, rendered, "fix this in a.go")
}

func TestModel_AnnotateInputEchoesCharacters(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// type characters one at a time and verify each appears in viewport
	for _, ch := range []rune{'h', 'e', 'l', 'l', 'o'} {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)

		// verify the typed text so far is visible in the viewport content
		vpContent := model.viewport.View()
		typed := model.annotateInput.Value()
		assert.Contains(t, vpContent, typed, "viewport should contain typed text %q after keystroke %q", typed, string(ch))
	}

	// final check: all characters are visible
	assert.Equal(t, "hello", model.annotateInput.Value())
	assert.Contains(t, model.viewport.View(), "hello")
}

func TestModel_AnnotateInputVisibleBeforeEnter(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// type some text
	for _, ch := range []rune{'t', 'e', 's', 't'} {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	// verify text is in viewport content before pressing Enter
	assert.True(t, model.annotating, "should still be in annotation mode")
	vpContent := model.viewport.View()
	assert.Contains(t, vpContent, "test", "typed text should be visible in viewport before Enter")

	// also verify via renderDiff (which is what SetContent uses)
	rendered := model.renderDiff()
	assert.Contains(t, rendered, "test", "typed text should be in rendered diff before Enter")

	// also verify the annotation input emoji marker is visible
	assert.Contains(t, rendered, "\U0001f4ac", "annotation marker should be visible before Enter")
}

func TestModel_UpdateForwardsNonKeyMsgWhileAnnotating(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// send a non-key message (e.g. a custom struct); should not panic and model stays annotating
	type customMsg struct{}
	result, _ = model.Update(customMsg{})
	model = result.(Model)
	assert.True(t, model.annotating, "annotating should remain true after non-key message")
}

func TestModel_AnnotateRenderWithDividers(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
		{OldNum: 11, Content: "removed", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, nil)
	m.currFile = "a.go"
	m.diffLines = lines

	// add annotations on different change types
	m.store.Add("a.go", 2, "+", "addition comment")
	m.store.Add("a.go", 11, "-", "removal comment")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "addition comment")
	assert.Contains(t, rendered, "removal comment")
	assert.Contains(t, rendered, "...")
}

func TestModel_StatusBarShowsDeleteOnAnnotatedLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.cursorOnAnnotation = true // cursor on the annotation sub-line
	m.focus = paneDiff
	m.store.Add("a.go", 1, " ", "review this")

	view := m.View()
	assert.Contains(t, view, "[d] delete", "status bar should show delete hint on annotation sub-line")
	assert.Contains(t, view, "annotate")
}

func TestModel_StatusBarHidesDeleteOnNonAnnotatedLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.focus = paneDiff

	// no annotations exist - delete hint should not appear
	view := m.View()
	assert.NotContains(t, view, "[d] delete", "status bar should not show delete hint on non-annotated line")
	assert.Contains(t, view, "annotate")

	// add annotation on line 2 (index 1), but cursor is on line 1 (index 0) - still no delete
	m.store.Add("a.go", 2, "+", "some comment")
	view = m.View()
	assert.NotContains(t, view, "[d] delete", "status bar should not show delete hint when cursor is on a different line")
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

	// ctrl+d should also move by page height from current position
	prevCursor := model.diffCursor
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = result.(Model)
	assert.Equal(t, prevCursor+pageHeight, model.diffCursor, "ctrl+d should move cursor by viewport height")
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

	// ctrl+u should also move up by page height
	model.diffCursor = 80
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = result.(Model)
	assert.Equal(t, 80-pageHeight, model.diffCursor, "ctrl+u should move cursor up by viewport height")
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
	var lines []diff.DiffLine
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

func TestModel_PgDownAccountsForAnnotations(t *testing.T) {
	lines := make([]diff.DiffLine, 50)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeAdd}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// add annotations on several lines - each takes an extra visual row
	for i := range 10 {
		model.store.Add("a.go", i+1, diff.ChangeAdd, "annotation")
	}

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown with annotations should move fewer cursor positions than viewport height
	// because annotation rows and annotation sub-lines take visual space
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := result.(Model)
	// cursor position or annotation flag must have changed
	assert.True(t, m2.diffCursor > 0 || m2.cursorOnAnnotation,
		"cursor should have moved forward (position or onto annotation)")
	assert.Less(t, m2.diffCursor, pageHeight,
		"with annotations, cursor should move fewer positions than viewport height")
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

func TestModel_TreeCtrlDMovesCursorByPage(t *testing.T) {
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

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model := result.(Model)
	assert.Equal(t, fmt.Sprintf("pkg/file%02d.go", pageSize), model.tree.selectedFile(),
		"ctrl+d in tree should move cursor by page size")
}

func TestModel_TreeCtrlUMovesCursorByPage(t *testing.T) {
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

	m.tree.moveToLast()
	for range 10 {
		m.tree.moveUp()
	}
	assert.Equal(t, "pkg/file39.go", m.tree.selectedFile())

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model := result.(Model)
	expected := fmt.Sprintf("pkg/file%02d.go", 39-pageSize)
	assert.Equal(t, expected, model.tree.selectedFile(), "ctrl+u in tree should move cursor by page size")
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

func TestModel_ShiftAStartsFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.True(t, model.annotating, "A should start annotation mode")
	assert.True(t, model.fileAnnotating, "A should set fileAnnotating=true")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_ShiftAIgnoredWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = ""
	m.focus = paneTree

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.False(t, model.annotating, "A without currFile should not start annotation")
	assert.Nil(t, cmd)
}

func TestModel_ShiftAOnlyWorksFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}

	// from tree pane — should be ignored to avoid annotating wrong file
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneTree
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.False(t, model.annotating, "A from tree pane should not start annotation")
	assert.False(t, model.fileAnnotating)
	assert.Nil(t, cmd)

	// from diff pane — should work
	m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m2.tree = newFileTree([]string{"a.go"})
	m2.currFile = "a.go"
	m2.diffLines = lines
	m2.focus = paneDiff
	result, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model = result.(Model)
	assert.True(t, model.annotating, "A should work from diff pane")
	assert.True(t, model.fileAnnotating)
	assert.NotNil(t, cmd)
}

func TestModel_AnnotationInputWidthNarrowTerminal(t *testing.T) {
	// very narrow terminal should not produce negative textinput width
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.focus = paneDiff
	m.width = 20
	m.treeWidth = 20 // width - treeWidth - 10 = -10 without the guard

	// line-level annotation
	cmd := m.startAnnotation()
	assert.NotNil(t, cmd)
	assert.True(t, m.annotating)
	assert.GreaterOrEqual(t, m.annotateInput.Width, 10, "text input width should be at least 10")

	// file-level annotation
	m.annotating = false
	cmd = m.startFileAnnotation()
	assert.NotNil(t, cmd)
	assert.True(t, m.fileAnnotating)
	assert.GreaterOrEqual(t, m.annotateInput.Width, 10, "file text input width should be at least 10")
}

func TestModel_FileAnnotationSavesWithLineZero(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff

	// start file-level annotation and set text
	m.startFileAnnotation()
	m.annotateInput.SetValue("file-level comment")

	// save via Enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating)
	assert.False(t, model.fileAnnotating)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 0, anns[0].Line, "file-level annotation should have Line=0")
	assert.Empty(t, anns[0].Type, "file-level annotation should have empty Type")
	assert.Equal(t, "file-level comment", anns[0].Comment)
}

func TestModel_FileAnnotationPreFillsExisting(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.store.Add("a.go", 0, "", "existing file note")

	m.startFileAnnotation()
	assert.Equal(t, "existing file note", m.annotateInput.Value(), "should pre-fill with existing file-level annotation")
}

func TestModel_FileAnnotationCancelResetsFlags(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff

	m.startFileAnnotation()
	assert.True(t, m.annotating)
	assert.True(t, m.fileAnnotating)

	// press Esc to cancel
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.annotating, "cancel should reset annotating")
	assert.False(t, model.fileAnnotating, "cancel should reset fileAnnotating")
}

func TestModel_FileAnnotationRenderedAtTopOfDiff(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
	}
	m.store.Add("a.go", 0, "", "this is a file note")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "file: this is a file note", "file-level annotation should appear in rendered diff")
	assert.Contains(t, rendered, "\U0001f4ac", "file-level annotation should have speech bubble emoji")

	// file annotation should appear before any line content
	fileIdx := strings.Index(rendered, "file: this is a file note")
	lineIdx := strings.Index(rendered, "package main")
	assert.Less(t, fileIdx, lineIdx, "file-level annotation should appear before diff lines")
}

func TestModel_FileAnnotationCursorHighlighted(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add("a.go", 0, "", "file note")
	m.focus = paneDiff
	m.diffCursor = -1 // on file annotation line

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "file: file note", "file annotation should be rendered")
}

func TestModel_DeleteFileAnnotationViaD(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.store.Add("a.go", 0, "", "file note to delete")
	m.diffCursor = -1 // on file annotation line

	// press 'd' to delete file-level annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"), "file-level annotation should be deleted")
	assert.GreaterOrEqual(t, model.diffCursor, 0, "cursor should move to first valid diff line after deletion")
}

func TestModel_DeleteFileAnnotationCursorNotOnFileLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.store.Add("a.go", 0, "", "file note")
	m.store.Add("a.go", 1, " ", "line note")
	m.diffCursor = 0            // on regular line
	m.cursorOnAnnotation = true // on the annotation sub-line for line 1

	// press 'd' should delete the line annotation, not the file annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "should only delete the line annotation")
	assert.Equal(t, 0, anns[0].Line, "file-level annotation should remain")
}

func TestModel_DeleteFileAnnotationFilterShiftsSelection(t *testing.T) {
	// when filter is active and deleting a file-level annotation removes the only annotation
	// for that file, the filter rebuilds entries and tree selects a different file
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines, "b.go": lines})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.store.Add("a.go", 0, "", "file note on a")
	m.store.Add("b.go", 1, " ", "line note on b")
	m.diffCursor = -1 // on file annotation line

	// enable filter to show only annotated files
	m.tree.toggleFilter(m.annotatedFiles())
	require.True(t, m.tree.filter)

	// press 'd' to delete file-level annotation on a.go
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)

	// a.go no longer has annotations, filter should shift selection to b.go
	assert.Empty(t, model.store.Get("a.go"), "file-level annotation should be deleted")
	assert.NotNil(t, cmd, "should return a command to load the new file")
	assert.Equal(t, "b.go", model.pendingFile, "should load b.go after filter shift")
}

func TestModel_CursorLineHasAnnotationExcludesFileLevel(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add("a.go", 0, "", "file note")
	m.diffCursor = 0 // on line 1, not on file annotation
	m.focus = paneDiff

	// line 1 has no line-level annotation, only file-level exists
	assert.False(t, m.cursorLineHasAnnotation(), "should not report file-level annotation as line annotation")
}

func TestModel_CursorOnFileAnnotationLineReportsAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add("a.go", 0, "", "file note")
	m.diffCursor = -1

	assert.True(t, m.cursorLineHasAnnotation(), "cursor on file annotation line should report annotation")
}

func TestModel_StatusBarShowsFileNoteHint(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines

	// tree pane should not show file note hint (A only works from diff pane)
	m.focus = paneTree
	view := m.View()
	assert.NotContains(t, view, "[A] file note", "tree pane should not show file note hint")

	// diff pane should show file note hint
	m.focus = paneDiff
	view = m.View()
	assert.Contains(t, view, "[A] file note", "diff pane should show file note hint when file is loaded")
}

func TestModel_StatusBarHidesFileNoteHintWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = ""
	m.focus = paneTree

	view := m.View()
	assert.NotContains(t, view, "[A] file note", "should not show file note hint when no file is loaded")
}

func TestModel_CursorNavigatesToFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = 0
	m.store.Add("a.go", 0, "", "file note")

	// move up from first line should go to file annotation
	m.moveDiffCursorUp()
	assert.Equal(t, -1, m.diffCursor, "cursor should move to file annotation line (-1)")

	// move down from file annotation should go to first non-divider line
	m.moveDiffCursorDown()
	assert.Equal(t, 0, m.diffCursor, "cursor should move from file annotation to first line")
}

func TestModel_HomeGoesToFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = 1
	m.store.Add("a.go", 0, "", "file note")
	m.ready = true

	m.moveDiffCursorToStart()
	assert.Equal(t, -1, m.diffCursor, "Home should move to file annotation line when it exists")
}

func TestModel_CursorViewportYWithFileAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m.store.Add("a.go", 0, "", "file note")

	// cursor on file annotation line
	m.diffCursor = -1
	assert.Equal(t, 0, m.cursorViewportY(), "file annotation line should be at viewport Y=0")

	// cursor on first diff line should be at Y=1 (file annotation occupies Y=0)
	m.diffCursor = 0
	assert.Equal(t, 1, m.cursorViewportY(), "first diff line should be at Y=1 when file annotation exists")

	// cursor on second diff line
	m.diffCursor = 1
	assert.Equal(t, 2, m.cursorViewportY(), "second diff line should be at Y=2 when file annotation exists")
}

func TestModel_RenderDiffFileAnnotationInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.focus = paneDiff

	// start file annotation and set text
	m.startFileAnnotation()
	m.annotateInput.SetValue("typing file note...")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "typing file note...", "file annotation input should be visible in rendered diff")
	assert.Contains(t, rendered, "file:", "should show file: prefix during input")
}

func TestModel_FileAnnotationExcludedFromRegularAnnotationMap(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add("a.go", 0, "", "file note")
	m.store.Add("a.go", 1, " ", "line note")

	set := m.buildAnnotationSet()
	assert.Len(t, set, 1, "buildAnnotationSet should exclude file-level annotations")
	assert.True(t, set["1: "], "line annotation should be in set")
}

func TestModel_StartAnnotationResetsFileAnnotating(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.diffCursor = 0
	m.focus = paneDiff

	// starting a regular annotation should set fileAnnotating to false
	m.startAnnotation()
	assert.True(t, m.annotating)
	assert.False(t, m.fileAnnotating, "startAnnotation should set fileAnnotating=false")
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

func TestModel_StatusBarShowsHunkIndicator(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},
	}

	m := testModel(nil, nil)
	m.diffLines = lines
	m.diffCursor = 1
	m.currFile = "a.go"
	m.focus = paneDiff

	status := m.statusBarText(m.annotatedFiles())
	assert.Contains(t, status, "[/] hunk 1/2")

	m.diffCursor = 3
	status = m.statusBarText(m.annotatedFiles())
	assert.Contains(t, status, "hunk 2/2")
}

func TestModel_StatusBarNoHunkIndicatorWithoutChanges(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}

	m := testModel(nil, nil)
	m.diffLines = lines
	m.diffCursor = 0
	m.currFile = "a.go"
	m.focus = paneDiff

	status := m.statusBarText(m.annotatedFiles())
	assert.NotContains(t, status, "[/] hunk", "should not show hunk hint when no hunks")
}

func TestModel_StatusBarHunksHintInDiffPane(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.diffCursor = 0
	m.focus = paneDiff

	status := m.statusBarText(m.annotatedFiles())
	assert.Contains(t, status, "[/] hunk 1/1")

	// tree pane should not show hunk hint
	m.focus = paneTree
	status = m.statusBarText(m.annotatedFiles())
	assert.NotContains(t, status, "[/] hunk")
}

func TestModel_EditExistingFileAnnotationShowsInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff
	m.store.Add("a.go", 0, "", "existing note")

	// start editing the existing file-level annotation
	m.startFileAnnotation()
	assert.True(t, m.annotating)
	assert.True(t, m.fileAnnotating)
	assert.Equal(t, "existing note", m.annotateInput.Value(), "input should be pre-filled")

	// render should show the text input, not the static annotation
	rendered := m.renderDiff()
	assert.Contains(t, rendered, "existing note", "input with pre-filled text should be visible")
	assert.Contains(t, rendered, "file:", "should show file: prefix during input")
}

func TestModel_FilterToggleLoadsDiffForNewSelection(t *testing.T) {
	lines := map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a-line", ChangeType: diff.ChangeAdd}},
		"b.go": {{NewNum: 1, Content: "b-line", ChangeType: diff.ChangeAdd}},
	}
	m := testModel([]string{"a.go", "b.go"}, lines)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "b.go"
	m.diffLines = lines["b.go"]
	m.focus = paneTree

	// annotate only a.go
	m.store.Add("a.go", 1, "+", "note on a")

	// toggle filter on — should select a.go (the only annotated file)
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model := result.(Model)
	assert.True(t, model.tree.filter)

	// since b.go was current and a.go is now selected, a load command should be returned
	if cmd != nil {
		msg := cmd()
		flm, ok := msg.(fileLoadedMsg)
		assert.True(t, ok, "filter toggle should trigger file load for new selection")
		assert.Equal(t, "a.go", flm.file)
	} else {
		t.Fatal("expected a load command after filter toggle changed selection")
	}
}
