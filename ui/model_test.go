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
	"github.com/umputun/revdiff/ui/mocks"
)

func noopHighlighter() *mocks.SyntaxHighlighterMock {
	return &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
	}
}

func testModel(files []string, fileDiffs map[string][]diff.DiffLine) Model {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]string, error) {
			return files, nil
		},
		FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
			return fileDiffs[file], nil
		},
	}
	store := annotation.NewStore()
	m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "needs review"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 10, Type: " ", Comment: "check this"})

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

func TestModel_EnterSwitchesToDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.focus = paneTree
	// simulate file already loaded (tree nav auto-loads)
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.focus = paneTree // reset focus after file load

	// enter should switch to diff pane
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.focus)
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test annotation"})

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
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "note"})

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
		{name: "default ratio 2 of 10", ratio: 2, termWidth: 120, wantTreeWidth: 24},
		{name: "ratio 3 of 10", ratio: 3, termWidth: 120, wantTreeWidth: 36},
		{name: "ratio 5 of 10", ratio: 5, termWidth: 120, wantTreeWidth: 60},
		{name: "min width enforced", ratio: 1, termWidth: 100, wantTreeWidth: 20},
		{name: "invalid ratio defaults to 2", ratio: 0, termWidth: 120, wantTreeWidth: 24},
		{name: "over max ratio defaults to 2", ratio: 15, termWidth: 120, wantTreeWidth: 24},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			renderer := &mocks.RendererMock{
				ChangedFilesFunc: func(string, bool) ([]string, error) { return []string{"a.go"}, nil },
				FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
			}
			m := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TreeWidthRatio: tc.ratio})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

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
	assert.Equal(t, string(diff.ChangeContext), anns[0].Type)
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
	assert.Equal(t, string(diff.ChangeAdd), anns[0].Type)
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
	assert.Equal(t, string(diff.ChangeRemove), anns[0].Type)
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "old comment"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "test comment"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "needs error handling"})

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

func TestModel_AnnotationCountInStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "other"})
	annotated := m.annotatedFiles()
	status := m.statusBarText(annotated)
	assert.Contains(t, status, "2 annotations")
}

func TestModel_NoAnnotationCountWhenEmpty(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	annotated := m.annotatedFiles()
	status := m.statusBarText(annotated)
	assert.NotContains(t, status, "annotations")
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "comment"})
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
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)

	// user navigates to b.go (seq=2)
	model.loadSeq = 2
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "initial annotation"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "annotation on a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "annotation on b"})

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
	assert.Equal(t, uint64(1), m.loadSeq, "loadSeq should be incremented to invalidate in-flight loads")
}

func TestModel_FilterDisabledWhenLastAnnotationDeleted(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "only annotation"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "addition comment"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 11, Type: "-", Comment: "removal comment"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "review this"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "some comment"})
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
		model.store.Add(annotation.Annotation{File: "a.go", Line: i + 1, Type: string(diff.ChangeAdd), Comment: "annotation"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing file note"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "this is a file note"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note to delete"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "line note"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note on a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 1, Type: " ", Comment: "line note on b"})
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
}

func TestModel_CursorLineHasAnnotationExcludesFileLevel(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "line note"})

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
	assert.Contains(t, status, "[ ] hunk 1/2")

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
	assert.NotContains(t, status, "[ ] hunk", "should not show hunk hint when no hunks")
}

func TestModel_StatusBarHunksHintInDiffPane(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.diffCursor = 0
	m.focus = paneDiff

	status := m.statusBarText(m.annotatedFiles())
	assert.Contains(t, status, "[ ] hunk 1/1")

	// tree pane should not show hunk hint
	m.focus = paneTree
	status = m.statusBarText(m.annotatedFiles())
	assert.NotContains(t, status, "[ ] hunk")
}

func TestModel_EditExistingFileAnnotationShowsInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing note"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note on a"})

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

func TestModel_RenderDiffLineHighlighted(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
		{OldNum: 2, Content: "func bar() {}", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.highlightedLines = []string{"hl-context", "hl-add", "hl-remove"}
	m.focus = paneDiff
	output := m.renderDiff()

	assert.Contains(t, output, "hl-context", "highlighted context line should appear")
	assert.Contains(t, output, "hl-add", "highlighted add line should appear")
	assert.Contains(t, output, "hl-remove", "highlighted remove line should appear")
}

func TestModel_RenderDiffLineCursorHighlight(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "line two", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.focus = paneDiff
	m.diffCursor = 0
	output := m.renderDiff()
	assert.Contains(t, output, "▶", "cursor indicator should appear on active line")
	assert.Contains(t, output, "line one", "cursor line content should appear")
}

func TestModel_RenderDiffLineTabReplacement(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "\tfoo", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.tabSpaces = "    " // 4 spaces
	output := m.renderDiff()
	assert.Contains(t, output, "    foo", "tabs should be replaced with spaces")
	assert.NotContains(t, output, "\t", "no raw tabs should remain")
}

func TestModel_PlainStyles(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]string, error) { return []string{"a.go"}, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{NoColors: true, TreeWidthRatio: 3})
	m.width = 120
	m.height = 40
	m.treeWidth = 36
	m.ready = true
	// plain styles should not panic and should render
	output := m.View()
	assert.NotEmpty(t, output)
}

func TestModel_TabWidthDefault(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]string, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TabWidth: 0})
	assert.Equal(t, "    ", m.tabSpaces, "tab width 0 should default to 4 spaces")

	m2 := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TabWidth: 2})
	assert.Equal(t, "  ", m2.tabSpaces, "tab width 2 should produce 2 spaces")
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

func TestModel_ViewNoStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.noStatusBar = true
	m.ready = true
	m.width = 120
	m.height = 40
	m.treeWidth = 24
	view := m.View()
	assert.NotContains(t, view, "quit", "status bar should be hidden")
	assert.Contains(t, view, "a.go", "tree content should still appear")
}

func TestModel_DiscardedAccessor(t *testing.T) {
	t.Run("default is false", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		assert.False(t, m.Discarded())
	})

	t.Run("true when set", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.discarded = true
		assert.True(t, m.Discarded())
	})
}

func TestModel_NoConfirmDiscardWired(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]string, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()
	m := NewModel(renderer, store, noopHighlighter(), ModelConfig{NoConfirmDiscard: true, TreeWidthRatio: 3})
	assert.True(t, m.noConfirmDiscard, "noConfirmDiscard should be wired from ModelConfig")
}

func TestModel_QKeyDiscardNoAnnotations(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "should be discarded when no annotations")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit")
}

func TestModel_QKeyWithAnnotationsEntersConfirming(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	assert.Nil(t, cmd, "should not quit yet")

	model := result.(Model)
	assert.True(t, model.inConfirmDiscard, "should enter confirming state")
	assert.False(t, model.Discarded(), "should not be discarded yet")
}

func TestModel_ConfirmDiscardY(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "y should confirm discard")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit after y")
}

func TestModel_ConfirmDiscardN(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	assert.Nil(t, cmd, "n should not quit")

	model := result.(Model)
	assert.False(t, model.inConfirmDiscard, "n should cancel confirmation")
	assert.False(t, model.Discarded(), "should not be discarded")
}

func TestModel_ConfirmDiscardEsc(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.Nil(t, cmd, "esc should not quit")

	model := result.(Model)
	assert.False(t, model.inConfirmDiscard, "esc should cancel confirmation")
	assert.False(t, model.Discarded())
}

func TestModel_ConfirmDiscardSecondQ(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "second Q should confirm discard")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit after second Q")
}

func TestModel_QKeyDuringAnnotationIgnored(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// load file
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// press Q - should be handled as text input, not discard
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	model = result.(Model)
	assert.True(t, model.annotating, "should still be annotating")
	assert.False(t, model.Discarded(), "should not be discarded")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming")
	assert.Contains(t, model.annotateInput.Value(), "Q", "Q should be typed into input")
}

func TestModel_QKeyNoConfirmDiscardWithAnnotations(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.noConfirmDiscard = true
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "should immediately discard with noConfirmDiscard")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming state")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit immediately")
}

func TestModel_QKeyNoStatusBarSkipsConfirmation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.noStatusBar = true
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "should immediately discard when status bar is hidden")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming state without status bar")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit immediately")
}

func TestModel_ConfirmDiscardBlocksOtherKeys(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	// pressing j (navigation) should be blocked
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Nil(t, cmd, "j should be blocked during confirmation")
	model := result.(Model)
	assert.True(t, model.inConfirmDiscard, "should still be confirming")

	// pressing q should be blocked too
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.Nil(t, cmd, "q should be blocked during confirmation")
	model = result.(Model)
	assert.True(t, model.inConfirmDiscard, "should still be confirming")
}

func TestModel_ConfirmDiscardAllowsNonKeyMessages(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	// WindowSizeMsg should still be handled
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := result.(Model)
	assert.Equal(t, 100, model.width, "resize should be handled during confirmation")
	assert.True(t, model.inConfirmDiscard, "should still be confirming after resize")
}

func TestModel_StatusBarDiscardConfirmation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "other"})
	m.inConfirmDiscard = true

	annotated := m.annotatedFiles()
	status := m.statusBarText(annotated)
	assert.Equal(t, "discard 2 annotations? [y/n]", status)
}

func TestModel_StatusBarShowsDiscardHint(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120

	t.Run("tree pane", func(t *testing.T) {
		m.focus = paneTree
		status := m.statusBarText(m.annotatedFiles())
		assert.Contains(t, status, "[Q] discard")
		assert.Contains(t, status, "[q] quit")
	})

	t.Run("diff pane", func(t *testing.T) {
		m.focus = paneDiff
		status := m.statusBarText(m.annotatedFiles())
		assert.Contains(t, status, "[Q] discard")
		assert.Contains(t, status, "[q] quit")
	})
}

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
		assert.True(t, model.collapsed, "v should enable collapsed mode")
		assert.NotNil(t, model.expandedHunks)
		assert.Empty(t, model.expandedHunks, "expandedHunks should be reset on toggle")
	})

	t.Run("toggle off", func(t *testing.T) {
		m.collapsed = true
		m.expandedHunks = map[int]bool{1: true}
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.False(t, model.collapsed, "v should disable collapsed mode")
		assert.Empty(t, model.expandedHunks, "expandedHunks should be reset on toggle")
	})

	t.Run("no-op in tree pane", func(t *testing.T) {
		m.collapsed = false
		m.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.False(t, model.collapsed, "v should be no-op in tree pane")
	})

	t.Run("no-op when no file loaded", func(t *testing.T) {
		m.focus = paneDiff
		m.currFile = ""
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		model := result.(Model)
		assert.False(t, model.collapsed, "v should be no-op when no file loaded")
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
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.viewport.Height = 20

	t.Run("expand hunk at cursor", func(t *testing.T) {
		m.diffCursor = 2 // on add line in hunk 1 (start=1)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.True(t, model.expandedHunks[1], "hunk at index 1 should be expanded")
	})

	t.Run("collapse expanded hunk", func(t *testing.T) {
		m.expandedHunks = map[int]bool{1: true}
		m.diffCursor = 1 // on remove line in hunk 1
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.False(t, model.expandedHunks[1], "hunk should be collapsed after second dot")
	})

	t.Run("expand second hunk independently", func(t *testing.T) {
		m.expandedHunks = map[int]bool{1: true}
		m.diffCursor = 4 // on add line in hunk 2 (start=4)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.True(t, model.expandedHunks[4], "hunk 2 should be expanded")
		assert.True(t, model.expandedHunks[1], "hunk 1 should remain expanded")
	})

	t.Run("no-op on context line", func(t *testing.T) {
		m.expandedHunks = make(map[int]bool)
		m.diffCursor = 0 // on context line
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
		model := result.(Model)
		assert.Empty(t, model.expandedHunks, "dot on context line should be no-op")
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
	m.collapsed = false
	m.expandedHunks = make(map[int]bool)
	m.diffCursor = 0
	m.viewport.Height = 20

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
	model := result.(Model)
	assert.Empty(t, model.expandedHunks, "dot should be no-op in expanded mode")
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
	model.collapsed = true
	model.expandedHunks = map[int]bool{1: true}

	// load second file
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: model.loadSeq, lines: linesB})
	model = result.(Model)

	assert.True(t, model.collapsed, "collapsed should persist across file switches")
	assert.Empty(t, model.expandedHunks, "expandedHunks should be reset on file switch")
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.diffLines = tc.lines
			assert.Equal(t, tc.expect, m.buildModifiedSet(m.findHunks()))
		})
	}
}

func TestModel_CollapsedRenderHidesRemovedLines(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "context line", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed line", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added line", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "another context", ChangeType: diff.ChangeContext},
	}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "context line")
	assert.NotContains(t, rendered, "removed line", "removed lines should be hidden in collapsed mode")
	assert.Contains(t, rendered, "added line")
	assert.Contains(t, rendered, "another context")
}

func TestModel_CollapsedRenderModifiedVsPureAdd(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},        // hunk 1: mixed
		{NewNum: 2, Content: "modified line", ChangeType: diff.ChangeAdd}, // modified (paired with remove)
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "pure add line", ChangeType: diff.ChangeAdd}, // hunk 2: pure add
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext},
	}

	rendered := m.renderDiff()
	// modified lines get ~ gutter
	assert.Contains(t, rendered, " ~ modified line", "modified add should have ~ gutter")
	// pure adds get + gutter
	assert.Contains(t, rendered, " + pure add line", "pure add should have + gutter")
	// removed lines are hidden
	assert.NotContains(t, rendered, "old", "removed lines should be hidden")
}

func TestModel_CollapsedRenderExpandedHunkShowsAllLines(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	// expand the hunk at index 1
	m.expandedHunks = map[int]bool{1: true}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "removed", "removed line should be visible in expanded hunk")
	assert.Contains(t, rendered, "added", "added line should be visible in expanded hunk")
	// expanded hunk uses standard styling: + for add, - for remove
	assert.Contains(t, rendered, " - removed", "expanded hunk should use - gutter for removes")
	assert.Contains(t, rendered, " + added", "expanded hunk should use + gutter for adds")
}

func TestModel_CollapsedRenderAnnotationsOnRemovedLinesHidden(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "annotation on removed"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "annotation on added"})

	rendered := m.renderDiff()
	assert.NotContains(t, rendered, "annotation on removed", "annotation on removed line should be hidden in collapsed mode")
	assert.Contains(t, rendered, "annotation on added", "annotation on added line should be visible")
}

func TestModel_CollapsedRenderAnnotationsVisibleWhenHunkExpanded(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m.expandedHunks = map[int]bool{1: true}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "annotation on removed"})

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "annotation on removed", "annotation on removed line should be visible when hunk expanded")
}

func TestModel_CollapsedRenderEmptyDiffLines(t *testing.T) {
	m := testModel(nil, nil)
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = nil

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "no changes")
}

func TestModel_CollapsedRenderDividerOnlyLines(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
		{Content: "~~~", ChangeType: diff.ChangeDivider},
	}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "...")
	assert.Contains(t, rendered, "~~~")
}

func TestModel_CollapsedRenderAllRemovesShowsPlaceholder(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},
		{OldNum: 2, Content: "old2", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "old3", ChangeType: diff.ChangeRemove},
	}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "3 lines deleted", "all-removes file should show delete placeholder in collapsed mode")
	assert.NotContains(t, rendered, "old1", "removed lines content should be hidden")
}

func TestModel_CollapsedDeleteOnlyPlaceholderHidesAnnotations(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove}, // placeholder line
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "note on deleted line"})

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "2 lines deleted", "placeholder should be shown")
	assert.NotContains(t, rendered, "note on deleted line", "annotation on placeholder should be hidden")

	// expand hunk, annotation should appear
	m.expandedHunks[1] = true
	rendered = m.renderDiff()
	assert.Contains(t, rendered, "note on deleted line", "annotation should be visible when hunk is expanded")
}

func TestModel_CollapsedDeleteOnlyPlaceholderBlocksAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	m.expandedHunks[1] = true
	assert.False(t, m.isDeleteOnlyPlaceholder(1, hunks), "expanded hunk should not be placeholder")

	// not collapsed mode
	m.collapsed = false
	m.expandedHunks = make(map[int]bool)
	assert.False(t, m.isDeleteOnlyPlaceholder(1, hunks), "should return false when not in collapsed mode")
}

func TestModel_CollapsedRenderDeleteOnlyHunkInMixedFile(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove}, // delete-only hunk
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
		{OldNum: 5, Content: "old", ChangeType: diff.ChangeRemove}, // mixed hunk
		{NewNum: 3, Content: "new", ChangeType: diff.ChangeAdd},
	}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "2 lines deleted", "delete-only hunk should show placeholder")
	assert.NotContains(t, rendered, "del1", "removed line content should be hidden")
	assert.NotContains(t, rendered, "del2", "removed line content should be hidden")
	assert.Contains(t, rendered, "new", "add line from mixed hunk should be visible")
}

func TestModel_CollapsedExpandDeleteOnlyHunkWithDot(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove}, // delete-only hunk start
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m.diffCursor = 1 // on placeholder

	// verify placeholder is shown and content is hidden
	rendered := m.renderDiff()
	assert.Contains(t, rendered, "2 lines deleted")
	assert.NotContains(t, rendered, "del1")

	// expand the hunk with '.'
	m.toggleHunkExpansion()
	assert.True(t, m.expandedHunks[1], "hunk should be expanded")

	// after expansion, removed lines should be visible
	rendered = m.renderDiff()
	assert.Contains(t, rendered, "del1", "expanded hunk should show removed lines")
	assert.Contains(t, rendered, "del2", "expanded hunk should show all removed lines")
	assert.NotContains(t, rendered, "lines deleted", "placeholder should not appear when expanded")
}

func TestModel_CollapsedCursorMovementIncludesDeleteOnlyPlaceholder(t *testing.T) {
	m := testModel(nil, nil)
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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

func TestModel_ExpandedModeUnchangedRegression(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = false
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}

	rendered := m.renderDiff()
	// in expanded mode, all lines are visible
	assert.Contains(t, rendered, "old", "removed lines should be visible in expanded mode")
	assert.Contains(t, rendered, "new", "added lines should be visible in expanded mode")
	assert.Contains(t, rendered, " - old", "expanded mode should use - gutter for removes")
	assert.Contains(t, rendered, " + new", "expanded mode should use + gutter for adds")
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

func TestModel_CollapsedRenderMultipleExpandedHunks(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove}, // hunk at 1
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{OldNum: 4, Content: "old2", ChangeType: diff.ChangeRemove}, // hunk at 4
		{NewNum: 4, Content: "new2", ChangeType: diff.ChangeAdd},
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext},
	}
	// expand both hunks
	m.expandedHunks = map[int]bool{1: true, 4: true}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "old1", "first expanded hunk should show removed line")
	assert.Contains(t, rendered, "old2", "second expanded hunk should show removed line")
	assert.Contains(t, rendered, "new1")
	assert.Contains(t, rendered, "new2")
}

func TestModel_CollapsedRenderMixedExpandedAndCollapsedHunks(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()
	m.collapsed = true
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove}, // hunk at 1
		{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{OldNum: 4, Content: "old2", ChangeType: diff.ChangeRemove}, // hunk at 4
		{NewNum: 4, Content: "new2", ChangeType: diff.ChangeAdd},
		{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext},
	}
	// expand only first hunk
	m.expandedHunks = map[int]bool{1: true}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "old1", "expanded hunk should show removed line")
	assert.NotContains(t, rendered, "old2", "collapsed hunk should hide removed line")
	assert.Contains(t, rendered, " ~ new2", "collapsed mixed hunk should use ~ gutter")
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
	model.collapsed = true
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
	model.collapsed = true
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
	model.collapsed = true
	model.expandedHunks = map[int]bool{1: true} // expand the hunk starting at index 1

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

func TestModel_ExpandedModeCursorMovementUnchanged(t *testing.T) {
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
	assert.False(t, model.collapsed, "should be in expanded mode by default")
	assert.Equal(t, 0, model.diffCursor)

	// move down lands on removed line in expanded mode
	model.moveDiffCursorDown()
	assert.Equal(t, 1, model.diffCursor, "expanded mode should visit removed line")

	model.moveDiffCursorDown()
	assert.Equal(t, 2, model.diffCursor, "expanded mode should visit add line")

	model.moveDiffCursorDown()
	assert.Equal(t, 3, model.diffCursor, "expanded mode should visit ctx2")

	// move back up visits all lines
	model.moveDiffCursorUp()
	assert.Equal(t, 2, model.diffCursor)

	model.moveDiffCursorUp()
	assert.Equal(t, 1, model.diffCursor)

	model.moveDiffCursorUp()
	assert.Equal(t, 0, model.diffCursor)
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
		m.collapsed = true
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
		m.collapsed = true
		m.expandedHunks = map[int]bool{1: true} // hunk starts at index 1
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
	model.collapsed = true

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

func TestModel_CursorViewportYCollapsedMode(t *testing.T) {
	t.Run("removed lines not counted", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // idx 0
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // idx 1 - hidden
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},  // idx 2 - hidden
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},     // idx 3
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // idx 4
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines
		m.collapsed = true

		m.diffCursor = 0
		assert.Equal(t, 0, m.cursorViewportY(), "ctx1 at Y=0")

		// cursor at idx 3 (add line), but removed lines at 1,2 are hidden, so Y=1
		m.diffCursor = 3
		assert.Equal(t, 1, m.cursorViewportY(), "add line should be at Y=1, removed lines skipped")

		m.diffCursor = 4
		assert.Equal(t, 2, m.cursorViewportY(), "ctx2 should be at Y=2, removed lines skipped")
	})

	t.Run("expanded mode counts all lines", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines

		// expanded mode (default) counts all lines
		m.diffCursor = 3
		assert.Equal(t, 3, m.cursorViewportY(), "expanded mode should count all lines including removes")

		m.diffCursor = 4
		assert.Equal(t, 4, m.cursorViewportY(), "expanded mode Y=4 for idx 4")
	})

	t.Run("collapsed with annotations on visible lines", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines
		m.collapsed = true

		// add annotation on ctx1 (line 1, context type)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "note"})

		// cursor at idx 2 (add): ctx1(1 row) + annotation(1 row) = 2 preceding visual rows
		m.diffCursor = 2
		assert.Equal(t, 2, m.cursorViewportY(), "annotation on ctx1 adds a visual row")

		// cursor at idx 3 (ctx2): ctx1(1) + annotation(1) + add(1) = 3
		m.diffCursor = 3
		assert.Equal(t, 3, m.cursorViewportY(), "ctx2 after annotated ctx1 and add line")
	})

	t.Run("collapsed with annotation on removed line hidden", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines
		m.collapsed = true

		// annotation on the removed line - both line and annotation are hidden
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: string(diff.ChangeRemove), Comment: "old note"})

		// cursor at idx 2 (add): only ctx1 visible before it, removed line+annotation skipped
		m.diffCursor = 2
		assert.Equal(t, 1, m.cursorViewportY(), "removed line and its annotation should not count")
	})
}

func TestModel_CursorViewportYCollapsedExpandedHunks(t *testing.T) {
	t.Run("expanded hunk shows all lines in Y calculation", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "old2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines
		m.collapsed = true
		m.expandedHunks = map[int]bool{1: true} // hunk starts at index 1

		// all lines are now visible because the hunk is expanded
		m.diffCursor = 3
		assert.Equal(t, 3, m.cursorViewportY(), "expanded hunk: Y=3 counting all lines")

		m.diffCursor = 4
		assert.Equal(t, 4, m.cursorViewportY(), "expanded hunk: Y=4 for ctx2")
	})

	t.Run("mixed expanded and collapsed hunks", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // idx 0
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},  // idx 1 - hunk1 (expanded)
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},     // idx 2
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext}, // idx 3
			{OldNum: 4, Content: "old2", ChangeType: diff.ChangeRemove},  // idx 4 - hunk2 (collapsed)
			{NewNum: 4, Content: "new2", ChangeType: diff.ChangeAdd},     // idx 5
			{NewNum: 5, Content: "ctx3", ChangeType: diff.ChangeContext}, // idx 6
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines
		m.collapsed = true
		m.expandedHunks = map[int]bool{1: true} // only hunk1 expanded

		// hunk1 expanded: ctx1(0), old1(1), new1(2), ctx2(3) all visible
		m.diffCursor = 3
		assert.Equal(t, 3, m.cursorViewportY(), "hunk1 expanded: ctx2 at Y=3")

		// hunk2 collapsed: old2 at idx 4 hidden, so idx 5 (new2) is at Y=4
		m.diffCursor = 5
		assert.Equal(t, 4, m.cursorViewportY(), "hunk2 collapsed: new2 at Y=4, old2 hidden")

		// ctx3 at idx 6: Y=5
		m.diffCursor = 6
		assert.Equal(t, 5, m.cursorViewportY(), "ctx3 at Y=5")
	})

	t.Run("expanded hunk with annotation on removed line", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old1", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new1", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = lines
		m.collapsed = true
		m.expandedHunks = map[int]bool{1: true}

		// annotation on the removed line - visible because hunk is expanded
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: string(diff.ChangeRemove), Comment: "old note"})

		// cursor at idx 2 (add): ctx1(1) + old1(1) + annotation(1) = 3
		m.diffCursor = 2
		assert.Equal(t, 3, m.cursorViewportY(), "expanded hunk: annotation on removed line is counted")
	})
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
	model.collapsed = true

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
	model.collapsed = true

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

func TestModel_StatusBarViewModeHint(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.focus = paneDiff
	m.width = 200

	t.Run("expanded mode shows collapse hint", func(t *testing.T) {
		m.collapsed = false
		status := m.statusBarText(m.annotatedFiles())
		assert.Contains(t, status, "[v] collapse")
		assert.NotContains(t, status, "[v] expand")
	})

	t.Run("collapsed mode shows expand hint", func(t *testing.T) {
		m.collapsed = true
		m.expandedHunks = make(map[int]bool)
		status := m.statusBarText(m.annotatedFiles())
		assert.Contains(t, status, "[v] expand")
		assert.NotContains(t, status, "[v] collapse")
	})

	t.Run("tree pane does not show view mode hint", func(t *testing.T) {
		m.focus = paneTree
		status := m.statusBarText(m.annotatedFiles())
		assert.NotContains(t, status, "[v]")
	})
}

func TestModel_StatusBarDotHint(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.currFile = "a.go"
	m.focus = paneDiff
	m.width = 200

	t.Run("collapsed mode on hunk shows expand hunk hint", func(t *testing.T) {
		m.collapsed = true
		m.expandedHunks = make(map[int]bool)
		m.diffCursor = 2 // on add line in hunk
		status := m.statusBarText(m.annotatedFiles())
		assert.Contains(t, status, "[.] expand hunk")
		assert.NotContains(t, status, "[.] collapse hunk")
	})

	t.Run("collapsed mode on expanded hunk shows collapse hunk hint", func(t *testing.T) {
		m.collapsed = true
		m.expandedHunks = map[int]bool{1: true} // hunk starts at index 1
		m.diffCursor = 2                        // on add line in expanded hunk
		status := m.statusBarText(m.annotatedFiles())
		assert.Contains(t, status, "[.] collapse hunk")
		assert.NotContains(t, status, "[.] expand hunk")
	})

	t.Run("collapsed mode on context line hides dot hint", func(t *testing.T) {
		m.collapsed = true
		m.expandedHunks = make(map[int]bool)
		m.diffCursor = 0 // on context line
		status := m.statusBarText(m.annotatedFiles())
		assert.NotContains(t, status, "[.]")
	})

	t.Run("expanded mode hides dot hint", func(t *testing.T) {
		m.collapsed = false
		m.diffCursor = 2 // on changed line, but not collapsed
		status := m.statusBarText(m.annotatedFiles())
		assert.NotContains(t, status, "[.]")
	})
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
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	m.collapsed = true
	m.expandedHunks = map[int]bool{1: true} // hunk at index 1 is expanded
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
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	m.collapsed = false
	assert.Equal(t, 1, m.firstVisibleInHunk(1, hunks))

	// collapsed mode: skips hidden removes, lands on add
	m.collapsed = true
	assert.Equal(t, 3, m.firstVisibleInHunk(1, hunks))

	// collapsed mode with expanded hunk: returns start
	m.expandedHunks = map[int]bool{1: true}
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
	m.collapsed = true
	hunks := m.findHunks() // [1]

	// all-removes hunk: placeholder line is visible, returns hunkStart
	assert.Equal(t, 1, m.firstVisibleInHunk(1, hunks))

	// expanded hunk: also returns hunkStart
	m.expandedHunks = map[int]bool{1: true}
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
		m.collapsed = true
		m.diffCursor = 1 // on hidden removed line
		m.adjustCursorIfHidden()
		assert.Equal(t, 3, m.diffCursor, "should move forward to add line")
	})

	t.Run("cursor on visible line stays put", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = lines
		m.collapsed = true
		m.diffCursor = 0 // on context line
		m.adjustCursorIfHidden()
		assert.Equal(t, 0, m.diffCursor, "should stay on context line")
	})

	t.Run("not collapsed mode is no-op", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = lines
		m.collapsed = false
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
		m.collapsed = true
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
		m.collapsed = true
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
		m.collapsed = true
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
	assert.True(t, m.collapsed)
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
	m.collapsed = true
	m.expandedHunks = map[int]bool{1: true} // hunk expanded
	m.diffCursor = 1                        // on removed line (visible because expanded)

	// collapse the hunk - cursor on removed line should move
	m.toggleHunkExpansion()
	assert.False(t, m.expandedHunks[1], "hunk should be collapsed")
	assert.Equal(t, 2, m.diffCursor, "cursor should move to add line after hunk collapse")
}

func TestModel_CollapsedCursorDownSkipsPlaceholderAnnotation(t *testing.T) {
	// cursor moving down through a delete-only placeholder with an annotation should NOT
	// stop on the invisible annotation sub-line
	m := testModel(nil, nil)
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
	assert.True(t, m.collapsed)
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
	m.collapsed = true
	m.expandedHunks = map[int]bool{1: true}
	m.diffCursor = 1
	m.cursorOnAnnotation = true // on annotation of expanded remove line

	m.toggleHunkExpansion()
	assert.False(t, m.cursorOnAnnotation, "cursorOnAnnotation should be cleared when hunk collapses")
}

func TestModel_CollapsedDeleteAnnotationBlockedOnPlaceholder(t *testing.T) {
	// pressing 'd' on a delete-only placeholder should not delete the invisible annotation
	m := testModel(nil, nil)
	m.collapsed = true
	m.expandedHunks = make(map[int]bool)
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
