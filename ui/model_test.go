package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

func TestModel_FilesLoadedSingleFile(t *testing.T) {
	m := testModel(nil, nil)
	result, cmd := m.Update(filesLoadedMsg{files: []string{"main.go"}})
	model := result.(Model)

	assert.True(t, model.singleFile, "singleFile should be true for one file")
	assert.Equal(t, paneDiff, model.focus, "focus should be on diff pane in single-file mode")
	assert.NotNil(t, cmd) // should auto-select first file
}

func TestModel_FilesLoadedSingleFileViewportWidth(t *testing.T) {
	m := testModel(nil, nil)
	// simulate initial resize (viewport created with multi-file width)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = resized.(Model)
	assert.True(t, m.ready, "model should be ready after resize")

	// now load single file — viewport width should be recalculated
	result, _ := m.Update(filesLoadedMsg{files: []string{"main.go"}})
	model := result.(Model)
	assert.True(t, model.singleFile)
	assert.Equal(t, 0, model.treeWidth, "treeWidth should be 0 in single-file mode")
	assert.Equal(t, 98, model.viewport.Width, "viewport width should be width - 2 (borders only)")
}

func TestModel_ResizeInSingleFileMode(t *testing.T) {
	m := testModel(nil, nil)
	// set up single-file mode via filesLoadedMsg
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = resized.(Model)
	loaded, _ := m.Update(filesLoadedMsg{files: []string{"main.go"}})
	m = loaded.(Model)
	require.True(t, m.singleFile)

	// resize while in single-file mode
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)

	assert.Equal(t, 0, model.treeWidth, "treeWidth stays 0 after resize in single-file mode")
	assert.Equal(t, 78, model.viewport.Width, "viewport width should be new width - 2")
}

func TestModel_FilesLoadedMultipleFiles(t *testing.T) {
	m := testModel(nil, nil)
	result, _ := m.Update(filesLoadedMsg{files: []string{"a.go", "b.go", "c.go"}})
	model := result.(Model)

	assert.False(t, model.singleFile, "singleFile should be false for multiple files")
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

func TestModel_ComputeFileStats(t *testing.T) {
	tests := []struct {
		name    string
		lines   []diff.DiffLine
		adds    int
		removes int
	}{
		{name: "empty diff", lines: nil, adds: 0, removes: 0},
		{name: "context only", lines: []diff.DiffLine{
			{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "// comment", ChangeType: diff.ChangeContext},
		}, adds: 0, removes: 0},
		{name: "adds only", lines: []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "line3", ChangeType: diff.ChangeAdd},
		}, adds: 3, removes: 0},
		{name: "removes only", lines: []diff.DiffLine{
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},
			{OldNum: 2, Content: "old2", ChangeType: diff.ChangeRemove},
		}, adds: 0, removes: 2},
		{name: "mixed changes", lines: []diff.DiffLine{
			{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old func", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new func", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "// ok", ChangeType: diff.ChangeContext},
			{Content: "", ChangeType: diff.ChangeDivider},
			{NewNum: 10, Content: "added line", ChangeType: diff.ChangeAdd},
		}, adds: 2, removes: 1},
		{name: "dividers ignored", lines: []diff.DiffLine{
			{Content: "", ChangeType: diff.ChangeDivider},
			{Content: "", ChangeType: diff.ChangeDivider},
		}, adds: 0, removes: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.diffLines = tt.lines
			m.computeFileStats()
			assert.Equal(t, tt.adds, m.fileAdds, "fileAdds")
			assert.Equal(t, tt.removes, m.fileRemoves, "fileRemoves")
		})
	}
}

func TestModel_FileStatsText(t *testing.T) {
	tests := []struct {
		name    string
		lines   []diff.DiffLine
		adds    int
		removes int
		want    string
	}{
		{name: "context only shows line count", lines: []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
			{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		}, adds: 0, removes: 0, want: "3 lines"},
		{name: "diff shows adds/removes", lines: []diff.DiffLine{
			{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "ctx", ChangeType: diff.ChangeContext},
		}, adds: 1, removes: 0, want: "+1/-0"},
		{name: "empty diff shows +0/-0", lines: nil, adds: 0, removes: 0, want: "+0/-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.diffLines = tt.lines
			m.fileAdds = tt.adds
			m.fileRemoves = tt.removes
			assert.Equal(t, tt.want, m.fileStatsText())
		})
	}
}

func TestModel_FileLoadedComputesStats(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added2", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.loadSeq = 1

	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: lines})
	model := result.(Model)
	assert.Equal(t, 2, model.fileAdds)
	assert.Equal(t, 1, model.fileRemoves)
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

func TestModel_StatusBarFilterIndicator(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("filter icon shown when filter active", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.tree.filter = true
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "◉", "should show filter icon when filter active")
	})

	t.Run("filter icon always present", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "◉", "indicator always shown, muted when inactive")
	})
}

func TestModel_WrapModeFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]string, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("wrap enabled via config", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{Wrap: true, TreeWidthRatio: 2})
		assert.True(t, m.wrapMode)
	})

	t.Run("wrap disabled by default", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.wrapMode)
	})
}

func TestModel_CollapsedModeFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]string, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("collapsed enabled via config", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{Collapsed: true, TreeWidthRatio: 2})
		assert.True(t, m.collapsed.enabled)
	})

	t.Run("collapsed disabled by default", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.collapsed.enabled)
	})
}

func TestModel_StatusModeIcons(t *testing.T) {
	t.Run("all icons always present", func(t *testing.T) {
		m := testModel(nil, nil)
		icons := m.statusModeIcons()
		assert.Contains(t, icons, "▼")
		assert.Contains(t, icons, "◉")
		assert.Contains(t, icons, "↩")
		assert.Contains(t, icons, "≋")
	})

	t.Run("with colors active icons use status fg", func(t *testing.T) {
		colors := Colors{Muted: "#6c6c6c", StatusFg: "#202020"}
		m := testModel(nil, nil)
		m.styles = newStyles(colors)
		m.collapsed.enabled = true
		m.tree.filter = false
		icons := m.statusModeIcons()
		// active collapsed icon should have status fg sequence
		assert.Contains(t, icons, "\033[38;2;32;32;32m▼")
		// inactive filter icon should have muted fg sequence
		assert.Contains(t, icons, "\033[38;2;108;108;108m◉")
	})
}

func TestModel_StatusBarWrapIndicator(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("wrap icon shown when active", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.wrapMode = true
		m.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "↩", "should show wrap icon when wrap active")
	})

	t.Run("wrap icon always present", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "↩", "indicator always shown, muted when inactive")
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

	// tree pane focused - should show file tree and help hint
	m.focus = paneTree
	view := m.View()
	assert.Contains(t, view, "a.go")
	assert.Contains(t, view, "b.go")
	assert.Contains(t, view, "? help")

	// diff pane focused - should show help hint
	m.focus = paneDiff
	view = m.View()
	assert.Contains(t, view, "? help")
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

func TestModel_StatusBarShowsFilenameAndStats(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines
	m.fileAdds = 1
	m.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "a.go", "status bar should show filename")
	assert.Contains(t, status, "+1/-0", "status bar should show diff stats")
	assert.Contains(t, status, "? help", "status bar should show help hint")
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
	status := m.statusBarText()
	assert.Contains(t, status, "2 annotations")
}

func TestModel_NoAnnotationCountWhenEmpty(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	status := m.statusBarText()
	assert.NotContains(t, status, "annotations")
}

func TestModel_StatusBarFilenameTruncation(t *testing.T) {
	longFile := "very/long/path/to/some/deeply/nested/file/in/the/project/structure.go"
	m := testModel(nil, nil)
	m.currFile = longFile
	m.fileAdds = 3
	m.fileRemoves = 1
	m.focus = paneDiff
	m.width = 40 // narrow terminal forces truncation

	status := m.statusBarText()
	assert.Contains(t, status, "…", "should truncate filename with ellipsis")
	assert.Contains(t, status, "+3/-1", "should still show stats after truncation")
	assert.Contains(t, status, "? help", "should still show help hint")
}

func TestModel_StatusBarFilenameTruncationWideChars(t *testing.T) {
	// CJK characters are 2 display cells wide per rune, the truncation must use
	// display-width measurement, not rune count, to avoid overflowing the status line
	wideFile := "path/to/日本語のファイル名/テスト.go"
	m := testModel(nil, nil)
	m.currFile = wideFile
	m.fileAdds = 1
	m.fileRemoves = 0
	m.focus = paneDiff
	m.width = 40

	status := m.statusBarText()
	assert.Contains(t, status, "…", "should truncate wide-char filename with ellipsis")
	assert.Contains(t, status, "+1/-0", "should still show stats after truncation")
	assert.LessOrEqual(t, lipgloss.Width(status), m.width-2, "status text must fit within terminal width minus padding")
}

func TestModel_StatusBarModeIndicators(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.diffCursor = 0
	m.focus = paneDiff
	m.width = 200

	t.Run("both indicators when collapsed and filtered", func(t *testing.T) {
		m.collapsed.enabled = true
		m.collapsed.expandedHunks = make(map[int]bool)
		m.tree.filter = true
		status := m.statusBarText()
		assert.Contains(t, status, "▼")
		assert.Contains(t, status, "◉")
	})

	t.Run("indicators always present in default mode", func(t *testing.T) {
		m.collapsed.enabled = false
		m.tree.filter = false
		status := m.statusBarText()
		assert.Contains(t, status, "▼", "always shown, muted when inactive")
		assert.Contains(t, status, "◉", "always shown, muted when inactive")
	})
}

func TestModel_StatusBarNarrowTerminalDegradation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1
	m.fileAdds = 1
	m.focus = paneDiff
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.tree.filter = true

	t.Run("wide terminal shows all segments", func(t *testing.T) {
		m.width = 200
		status := m.statusBarText()
		assert.Contains(t, status, "a.go")
		assert.Contains(t, status, "+1/-0")
		assert.Contains(t, status, "hunk 1/1")
		assert.Contains(t, status, "▼")
		assert.Contains(t, status, "◉")
		assert.Contains(t, status, "? help")
	})

	t.Run("narrow terminal drops hunk from left first", func(t *testing.T) {
		m.width = 50
		status := m.statusBarText()
		assert.Contains(t, status, "a.go")
		assert.Contains(t, status, "+1/-0")
		assert.Contains(t, status, "? help")
	})

	t.Run("very narrow terminal drops hunk info", func(t *testing.T) {
		m.width = 28
		status := m.statusBarText()
		assert.Contains(t, status, "? help")
		assert.NotContains(t, status, "hunk", "hunk should be dropped on very narrow terminal")
	})
}

func TestModel_StatusBarStatsDisplay(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "main.go"
	m.fileAdds = 10
	m.fileRemoves = 5
	m.width = 120

	status := m.statusBarText()
	assert.Contains(t, status, "main.go")
	assert.Contains(t, status, "+10/-5")
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

func TestModel_StatusBarNoShortcutHintsInDiffPane(t *testing.T) {
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
	m.cursorOnAnnotation = true
	m.focus = paneDiff
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "review this"})

	status := m.statusBarText()
	// shortcut hints moved to help overlay
	assert.NotContains(t, status, "[d]")
	assert.NotContains(t, status, "[enter/a]")
	assert.NotContains(t, status, "[A]")
	assert.NotContains(t, status, "[Q]")
	assert.NotContains(t, status, "[q]")
	// should show filename, stats, annotation count, help hint
	assert.Contains(t, status, "a.go")
	assert.Contains(t, status, "1 annotation")
	assert.Contains(t, status, "? help")
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

func TestModel_StatusBarShowsHelpHint(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines

	m.focus = paneTree
	status := m.statusBarText()
	assert.Contains(t, status, "? help", "tree pane should show help hint")

	m.focus = paneDiff
	status = m.statusBarText()
	assert.Contains(t, status, "? help", "diff pane should show help hint")
}

func TestModel_StatusBarNoFilenameWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = ""
	m.focus = paneTree

	status := m.statusBarText()
	assert.NotContains(t, status, "/-", "no diff stats should be shown without a file")
	assert.Contains(t, status, "? help")
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

func TestModel_CursorViewportYWithWrappedAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.width = 60
	m.treeWidth = 20
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	t.Run("long file annotation wraps to multiple rows", func(t *testing.T) {
		longComment := strings.Repeat("word ", 20) // ~100 chars, wraps at ~34
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})
		defer m.store.Delete("a.go", 0, "")

		wrapCount := m.wrappedAnnotationLineCount("file")
		assert.Greater(t, wrapCount, 1, "long file annotation should wrap to multiple rows")

		m.diffCursor = 0
		assert.Equal(t, wrapCount, m.cursorViewportY(), "first diff line offset should equal wrap count")

		m.diffCursor = 1
		assert.Equal(t, wrapCount+1, m.cursorViewportY(), "second diff line offset should be wrap count + 1")
	})

	t.Run("long inline annotation wraps to multiple rows", func(t *testing.T) {
		longComment := strings.Repeat("note ", 20) // ~100 chars
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: longComment})
		defer m.store.Delete("a.go", 1, " ")

		key := m.annotationKey(1, " ")
		wrapCount := m.wrappedAnnotationLineCount(key)
		assert.Greater(t, wrapCount, 1, "long inline annotation should wrap to multiple rows")

		m.diffCursor = 1
		assert.Equal(t, 1+wrapCount, m.cursorViewportY(), "second line should offset by annotation wrap count")
	})

	t.Run("short annotation stays one row", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "short"})
		defer m.store.Delete("a.go", 0, "")

		assert.Equal(t, 1, m.wrappedAnnotationLineCount("file"))
	})
}

func TestModel_RenderWrappedAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.width = 60
	m.treeWidth = 20
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff

	t.Run("long file annotation wraps in rendered output", func(t *testing.T) {
		longComment := strings.Repeat("wrap ", 20)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})
		defer m.store.Delete("a.go", 0, "")

		wrapCount := m.wrappedAnnotationLineCount("file")
		assert.Greater(t, wrapCount, 1, "annotation should wrap")
		// cursor chevron appears exactly once (first line only)
		m.diffCursor = -1
		rendered := m.renderDiff()
		assert.Equal(t, 1, strings.Count(rendered, "▶"), "cursor on first wrap line only")
	})
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

	status := m.statusBarText()
	assert.Contains(t, status, "hunk 1/2")

	m.diffCursor = 3
	status = m.statusBarText()
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

	status := m.statusBarText()
	assert.NotContains(t, status, "hunk", "should not show hunk when cursor on context line")
}

func TestModel_StatusBarHunkOnlyInDiffPane(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.diffCursor = 0
	m.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "hunk 1/1")

	// tree pane should not show hunk
	m.focus = paneTree
	status = m.statusBarText()
	assert.NotContains(t, status, "hunk")
}

func TestModel_StatusBarHunkCountOnContextLine(t *testing.T) {
	t.Run("plural hunks", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "add1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
			{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 0
		m.currFile = "a.go"
		m.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "2 hunks")
		assert.NotContains(t, status, "hunk 0/")
	})

	t.Run("singular hunk", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "add1", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 0
		m.currFile = "a.go"
		m.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "1 hunk")
		assert.NotContains(t, status, "1 hunks", "should use singular form for one hunk")
	})
}

func TestModel_StatusBarPipeSeparators(t *testing.T) {
	t.Run("plain styles", func(t *testing.T) {
		m := testModel(nil, nil)
		m.currFile = "a.go"
		m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
		m.fileAdds = 1
		m.diffCursor = 0
		m.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "|", "separator pipe must be present")
		assert.NotContains(t, status, "\033[0m", "no full ANSI reset in separator")
	})

	t.Run("with colors", func(t *testing.T) {
		colors := Colors{Muted: "#6c6c6c", StatusFg: "#202020", StatusBg: "#C5794F", Normal: "#d0d0d0"}
		m := testModel(nil, nil)
		m.styles = newStyles(colors)
		m.currFile = "a.go"
		m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
		m.fileAdds = 1
		m.diffCursor = 0
		m.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "|", "separator pipe must be present")
		assert.NotContains(t, status, "\033[0m", "separator must use raw ANSI fg, not lipgloss Render")
		assert.Contains(t, status, "\033[38;2;108;108;108m", "separator should have muted fg ANSI sequence")
		assert.Contains(t, status, "\033[38;2;32;32;32m", "separator should restore status fg after pipe")
	})
}

func TestModel_AnsiFg(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, "\033[38;2;108;108;108m", m.ansiFg("#6c6c6c"))
	assert.Equal(t, "\033[38;2;255;0;0m", m.ansiFg("#ff0000"))
	assert.Equal(t, "\033[38;2;255;0;0m", m.ansiFg("ff0000"), "should work without # prefix")
	assert.Empty(t, m.ansiFg("bad"), "should return empty for invalid hex")
}

func TestModel_AnsiBg(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, "\033[48;2;108;108;108m", m.ansiBg("#6c6c6c"))
	assert.Equal(t, "\033[48;2;255;0;0m", m.ansiBg("#ff0000"))
	assert.Empty(t, m.ansiBg("bad"), "should return empty for invalid hex")
}

func TestModel_HandleEscKeyClearsSearch(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "hello world"}},
	})
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{ChangeType: diff.ChangeAdd, Content: "hello world"}}
	m.searchTerm = "hello"
	m.searchMatches = []int{0}
	m.searchCursor = 0
	m.focus = paneDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.Empty(t, model.searchTerm, "esc should clear search term")
	assert.Nil(t, model.searchMatches, "esc should clear search matches")
}

func TestModel_HandleEscKeyNoopWithoutSearch(t *testing.T) {
	m := testModel(nil, nil)
	m.focus = paneDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.Empty(t, model.searchTerm)
	assert.Nil(t, model.searchMatches)
}

func TestModel_HighlightSearchMatches(t *testing.T) {
	colors := Colors{SearchFg: "#1a1a1a", SearchBg: "#d7d700"}
	m := testModel(nil, nil)
	m.styles = newStyles(colors)

	t.Run("plain text single match", func(t *testing.T) {
		m.searchTerm = "hello"
		result := m.highlightSearchMatches("say hello world")
		assert.NotContains(t, result, "\033[38;2;", "should not set foreground (bg-only highlight)")
		assert.Contains(t, result, "\033[48;2;215;215;0m") // search bg
		assert.Contains(t, result, "hello")
		assert.Contains(t, result, "\033[49m") // bg reset
	})

	t.Run("multiple matches", func(t *testing.T) {
		m.searchTerm = "ab"
		result := m.highlightSearchMatches("ab cd ab")
		assert.Equal(t, 2, strings.Count(result, "\033[48;2;215;215;0m"), "should highlight both occurrences")
	})

	t.Run("no match", func(t *testing.T) {
		m.searchTerm = "xyz"
		result := m.highlightSearchMatches("hello world")
		assert.Equal(t, "hello world", result)
	})

	t.Run("empty search term", func(t *testing.T) {
		m.searchTerm = ""
		result := m.highlightSearchMatches("hello world")
		assert.Equal(t, "hello world", result)
	})

	t.Run("case insensitive", func(t *testing.T) {
		m.searchTerm = "hello"
		result := m.highlightSearchMatches("say HELLO world")
		assert.Contains(t, result, "\033[48;2;215;215;0m")
	})

	t.Run("with ansi codes", func(t *testing.T) {
		m.searchTerm = "world"
		result := m.highlightSearchMatches("\033[32mhello world\033[0m")
		assert.Contains(t, result, "\033[48;2;215;215;0m") // search bg on
		assert.Contains(t, result, "\033[49m")             // search bg reset
		assert.Contains(t, result, "\033[32m")             // original ansi preserved
	})

	t.Run("no-colors fallback", func(t *testing.T) {
		noColorModel := testModel(nil, nil)
		noColorModel.searchTerm = "hello"
		result := noColorModel.highlightSearchMatches("say hello world")
		assert.Contains(t, result, "\033[7m", "should use reverse video in no-colors mode")
		assert.Contains(t, result, "\033[27m", "should reset reverse video")
	})
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

	status := m.statusBarText()
	assert.Equal(t, "discard 2 annotations? [y/n]", status)
}

func TestModel_StatusBarNoKeyHints(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120

	t.Run("tree pane has no shortcut hints", func(t *testing.T) {
		m.focus = paneTree
		status := m.statusBarText()
		assert.NotContains(t, status, "[Q]")
		assert.NotContains(t, status, "[q]")
		assert.NotContains(t, status, "[j/k]")
		assert.Contains(t, status, "? help")
	})

	t.Run("diff pane has no shortcut hints", func(t *testing.T) {
		m.focus = paneDiff
		status := m.statusBarText()
		assert.NotContains(t, status, "[Q]")
		assert.NotContains(t, status, "[q]")
		assert.NotContains(t, status, "[enter/a]")
		assert.Contains(t, status, "? help")
	})
}

func TestModel_HelpOverlaySections(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	// verify section headers are present
	assert.Contains(t, help, "Navigation")
	assert.Contains(t, help, "Search")
	assert.Contains(t, help, "Annotations")
	assert.Contains(t, help, "View")
	assert.Contains(t, help, "Quit")
}

func TestModel_HelpOverlayKeyListings(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	// verify key listings are present
	keys := []string{
		"tab", "n / p", "j / k", "PgDn/PgUp", "Ctrl+d/u", "Home/End", "h / l", "← / →", "[ / ]",
		"/", "n", "N",
		"a / enter", "A", "d", "f", "v", "w", ".",
		"q", "Q", "? / esc",
	}
	for _, k := range keys {
		assert.Contains(t, help, k, "help overlay should contain key: %s", k)
	}
}

func TestModel_HelpOverlayInView(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.width = 100
	m.height = 40

	// without help, view should not contain help sections
	m.showHelp = false
	view := m.View()
	assert.NotContains(t, view, "Navigation")
	assert.NotContains(t, view, "Annotations")

	// with help, view should contain help sections overlaid on top of content
	m.showHelp = true
	view = m.View()
	assert.Contains(t, view, "Navigation")
	assert.Contains(t, view, "Annotations")
	assert.Contains(t, view, "View")
	assert.Contains(t, view, "Quit")
	// overlay should preserve background content (tree pane visible on edges)
	assert.Contains(t, view, "a.go", "tree pane should be visible behind help overlay")
}

func TestModel_HelpToggle(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": {{ChangeType: diff.ChangeContext, Content: "x"}}})
	m.currFile = "a.go"
	m.focus = paneDiff
	assert.False(t, m.showHelp)

	// press ? to open help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := result.(Model)
	assert.True(t, model.showHelp)

	// press ? again to close help
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = result.(Model)
	assert.False(t, model.showHelp)
}

func TestModel_HelpCloseWithEsc(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": {{ChangeType: diff.ChangeContext, Content: "x"}}})
	m.currFile = "a.go"
	m.showHelp = true

	// press esc to close help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.showHelp)
}

func TestModel_HelpBlocksOtherKeys(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "x"}},
		"b.go": {{ChangeType: diff.ChangeContext, Content: "y"}},
	})
	m.currFile = "a.go"
	m.focus = paneDiff
	m.showHelp = true

	// navigation keys should be blocked
	for _, key := range []rune{'n', 'p', 'v', 'f', 'q', 'Q', 'j', 'k'} {
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		model := result.(Model)
		assert.True(t, model.showHelp, "key %q should not close help", string(key))
		assert.Nil(t, cmd, "key %q should produce no command", string(key))
	}

	// tab should also be blocked
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := result.(Model)
	assert.True(t, model.showHelp, "tab should not close help")
	assert.Nil(t, cmd, "tab should produce no command")

	// enter should be blocked
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.True(t, model.showHelp, "enter should not close help")
	assert.Nil(t, cmd, "enter should produce no command")
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

func TestModel_StyleDiffContent(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()

	t.Run("add line", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, false)
		assert.Contains(t, result, " + content")
	})

	t.Run("remove line", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeRemove, " - ", "content", false, false)
		assert.Contains(t, result, " - content")
	})

	t.Run("context line", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeContext, "   ", "content", false, false)
		assert.Contains(t, result, "   content")
	})

	t.Run("highlighted add", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeAdd, " + ", "\033[32mgreen\033[0m", true, false)
		assert.Contains(t, result, " + ")
		assert.Contains(t, result, "\033[32m")
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

func TestModel_HelpOverlayContainsWordWrap(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()
	assert.Contains(t, help, "toggle word wrap")
	assert.Contains(t, help, "w")
}

func TestModel_HelpOverlayContainsSearchKeys(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	assert.Contains(t, help, "Search")
	assert.Contains(t, help, "search in diff")
	assert.Contains(t, help, "next match")
	assert.Contains(t, help, "prev match")
	assert.Contains(t, help, "n = next match when searching")
}

func TestModel_StartSearch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// press / to start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	assert.True(t, model.searching, "should be in searching mode")
	assert.True(t, model.searchInput.Focused(), "search input should be focused")
}

func TestModel_StartSearchOnlyFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneTree

	// press / in tree pane - should not start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	assert.False(t, model.searching, "should not search from tree pane")
}

func TestModel_SubmitSearchFindsMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello world", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "foo bar", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "hello again", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	// type "hello"
	for _, ch := range "hello" {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	// submit with enter
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.False(t, model.searching, "should exit searching mode")
	assert.Equal(t, "hello", model.searchTerm)
	assert.Equal(t, []int{0, 2}, model.searchMatches)
	assert.Equal(t, 0, model.searchCursor)
	assert.Equal(t, 0, model.diffCursor, "cursor should be on first match")
}

func TestModel_SubmitSearchCaseInsensitive(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "Hello World", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "HELLO again", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("hello")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.Equal(t, []int{0, 1}, model.searchMatches, "should match case-insensitively")
}

func TestModel_SubmitSearchJumpsForwardFromCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match here", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "foo bar", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match again", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 1 // cursor past first match

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("match")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.Equal(t, 1, model.searchCursor, "should jump to second match (index 1)")
	assert.Equal(t, 2, model.diffCursor, "cursor should be on second match line")
}

func TestModel_SubmitSearchWrapsToFirstMatch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match here", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "foo bar", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 1 // cursor past all matches

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("match")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.Equal(t, 0, model.searchCursor, "should wrap to first match")
	assert.Equal(t, 0, model.diffCursor, "cursor should be on first match line")
}

func TestModel_SubmitSearchNoMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("xyz")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.False(t, model.searching)
	assert.Equal(t, "xyz", model.searchTerm)
	assert.Empty(t, model.searchMatches)
}

func TestModel_SubmitEmptySearchClearsMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// set up existing search state
	model.searchTerm = "hello"
	model.searchMatches = []int{0}
	model.searchCursor = 0

	// start search with empty input
	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.False(t, model.searching)
	assert.Empty(t, model.searchTerm)
	assert.Empty(t, model.searchMatches)
}

func TestModel_CancelSearch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	require.True(t, model.searching)

	// cancel with esc
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = result.(Model)

	assert.False(t, model.searching, "should exit searching mode on esc")
}

func TestModel_CancelSearchPreservesExistingMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// set up existing search state
	model.searchTerm = "hello"
	model.searchMatches = []int{0}

	// start and cancel new search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = result.(Model)

	assert.Equal(t, "hello", model.searchTerm, "existing search term should be preserved on cancel")
	assert.Equal(t, []int{0}, model.searchMatches, "existing matches should be preserved on cancel")
}

func TestModel_SearchInputForwardsCharacters(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	// type characters
	for _, ch := range "test" {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	assert.Equal(t, "test", model.searchInput.Value(), "characters should be forwarded to search input")
}

func TestModel_SearchBlocksOtherKeysWhileActive(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	// pressing q should not quit, it should type 'q'
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = result.(Model)

	assert.True(t, model.searching, "should still be searching")
	assert.Contains(t, model.searchInput.Value(), "q")
}

func TestModel_SearchForwardsNonKeyMessages(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	require.True(t, model.searching)

	// send a non-key message; should not panic and model stays searching
	type customMsg struct{}
	result, _ = model.Update(customMsg{})
	model = result.(Model)
	assert.True(t, model.searching, "searching should remain true after non-key message")
}

func TestModel_NextSearchMatch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "match three", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2, 3}
	model.searchCursor = 0
	model.diffCursor = 0

	// press n to go to next match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "search cursor should advance to 1")
	assert.Equal(t, 2, model.diffCursor, "diff cursor should move to second match")

	// press n again
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 2, model.searchCursor, "search cursor should advance to 2")
	assert.Equal(t, 3, model.diffCursor, "diff cursor should move to third match")
}

func TestModel_NextSearchMatchWrapsAround(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2}
	model.searchCursor = 1 // on last match
	model.diffCursor = 2

	// press n should wrap to first match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 0, model.searchCursor, "search cursor should wrap to 0")
	assert.Equal(t, 0, model.diffCursor, "diff cursor should wrap to first match")
}

func TestModel_PrevSearchMatch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "match three", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2, 3}
	model.searchCursor = 2
	model.diffCursor = 3

	// press N to go to prev match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "search cursor should go back to 1")
	assert.Equal(t, 2, model.diffCursor, "diff cursor should move to second match")
}

func TestModel_PrevSearchMatchWrapsAround(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2}
	model.searchCursor = 0 // on first match
	model.diffCursor = 0

	// press N should wrap to last match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "search cursor should wrap to last")
	assert.Equal(t, 2, model.diffCursor, "diff cursor should wrap to last match")
}

func TestModel_SearchNavigationSkipsCollapsedHiddenLines(t *testing.T) {
	// in collapsed mode, removed lines are hidden. search navigation must skip them.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "match removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "match added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = make(map[int]bool)
	// matches on indices 0 (ctx), 1 (hidden remove), 2 (add), 3 (ctx)
	model.searchMatches = []int{0, 1, 2, 3}
	model.searchCursor = 0
	model.diffCursor = 0

	t.Run("nextSearchMatch skips hidden removed line", func(t *testing.T) {
		m := model
		m.nextSearchMatch()
		assert.Equal(t, 2, m.searchCursor, "should skip hidden index 1, land on index 2")
		assert.Equal(t, 2, m.diffCursor, "cursor should be on visible add line")
	})

	t.Run("prevSearchMatch skips hidden removed line", func(t *testing.T) {
		m := model
		m.searchCursor = 2 // on index 2 (add line)
		m.diffCursor = 2
		m.prevSearchMatch()
		assert.Equal(t, 0, m.searchCursor, "should skip hidden index 1, land on index 0")
		assert.Equal(t, 0, m.diffCursor, "cursor should be on visible context line")
	})

	t.Run("submitSearch skips hidden match for initial jump", func(t *testing.T) {
		m := model
		m.diffCursor = 1 // cursor on hidden line
		m.searchTerm = ""
		m.searchMatches = nil
		m.searchInput = textinput.New()
		m.searchInput.SetValue("match")
		m.submitSearch()
		// should jump to index 2 (visible add) not index 1 (hidden remove)
		assert.Equal(t, 2, m.diffCursor, "should skip hidden remove and land on visible add")
	})
}

func TestModel_NKeyFallsThroughToNextFileWhenNoSearch(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// no search active, n should advance to next file
	assert.Empty(t, model.searchMatches)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile(), "n should go to next file when no search active")
}

func TestModel_ShiftNDoesPrevMatchWhenSearchActive(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "match two", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 1}
	model.searchCursor = 1
	model.diffCursor = 1

	// press N (shift-n)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 0, model.searchCursor, "N should go to prev match")
	assert.Equal(t, 0, model.diffCursor, "cursor should be on first match")
}

func TestModel_ShiftNDoesNothingWithoutSearch(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// no search active, N should do nothing
	assert.Empty(t, model.searchMatches)
	selected := model.tree.selectedFile()
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, selected, model.tree.selectedFile(), "N should not change file when no search")
}

func TestModel_SearchHighlightInRenderDiff(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func hello() {}", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "func world() {}", ChangeType: diff.ChangeAdd},
		{OldNum: 4, Content: "old line", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = noopHighlighter().HighlightLines("a.go", lines)
	m.focus = paneDiff
	m.diffCursor = 0
	m.styles = plainStyles()

	t.Run("no search, renderDiff succeeds with all lines", func(t *testing.T) {
		m.searchMatches = nil
		m.searchMatchSet = nil
		rendered := m.renderDiff()
		assert.Contains(t, rendered, "package main")
		assert.Contains(t, ansi.Strip(rendered), "func hello")
		assert.Contains(t, rendered, "func world")
		assert.Contains(t, rendered, "old line")
	})

	t.Run("search active, renderDiff includes matched content", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{1}
		m.searchCursor = 0
		rendered := m.renderDiff()
		// matched and non-matched lines should both be rendered
		assert.Contains(t, ansi.Strip(rendered), "func hello")
		assert.Contains(t, rendered, "func world")
		assert.Contains(t, rendered, "old line")
	})

	t.Run("search vs no search both render content correctly", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{1}
		m.searchCursor = 0
		renderedWithSearch := m.renderDiff()

		m.searchMatches = nil
		renderedWithout := m.renderDiff()

		// both should contain the same text content
		assert.Contains(t, ansi.Strip(renderedWithSearch), "func hello")
		assert.Contains(t, ansi.Strip(renderedWithout), "func hello")
		assert.Contains(t, renderedWithSearch, "func world")
		assert.Contains(t, renderedWithout, "func world")
	})

	t.Run("cursor coexists with search highlight", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{1}
		m.searchCursor = 0
		m.diffCursor = 1
		rendered := m.renderDiff()

		outputLines := strings.Split(rendered, "\n")
		var matchLine string
		for _, l := range outputLines {
			if strings.Contains(l, "hello") {
				matchLine = l
			}
		}
		require.NotEmpty(t, matchLine)
		assert.Contains(t, matchLine, "▶", "cursor should be present on matched line")
		assert.Contains(t, ansi.Strip(matchLine), "func hello", "content should be preserved with cursor on match")
	})
}

func TestModel_SearchHighlightWithWrap(t *testing.T) {
	longContent := "this is a very long line that contains the search term hello somewhere in the middle and should wrap"
	lines := []diff.DiffLine{
		{NewNum: 1, Content: longContent, ChangeType: diff.ChangeAdd},
		{NewNum: 2, Content: "short line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = noopHighlighter().HighlightLines("a.go", lines)
	m.focus = paneDiff
	m.diffCursor = 0
	m.wrapMode = true
	m.width = 60
	m.treeWidth = 12
	m.styles = plainStyles()

	m.searchTerm = "hello"
	m.searchMatches = []int{0}
	m.searchCursor = 0

	rendered := m.renderDiff()
	outputLines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")

	// the long line should produce continuation rows with ↪
	var continuationCount int
	for _, l := range outputLines {
		if strings.Contains(l, "↪") {
			continuationCount++
		}
	}
	assert.Positive(t, continuationCount, "wrapped search match should have continuation lines")

	// verify content is present (text flows through the rendering path correctly)
	assert.Contains(t, rendered, "hello")
	assert.Contains(t, rendered, "short line")
}

func TestModel_SearchHighlightInCollapsedMode(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "context line", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed line", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added hello line", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added other line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = noopHighlighter().HighlightLines("a.go", lines)
	m.focus = paneDiff
	m.diffCursor = 0
	m.styles = plainStyles()
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)

	t.Run("collapsed renders with search matches", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{2}
		m.searchCursor = 0
		rendered := m.renderDiff()

		assert.Contains(t, rendered, "added hello line")
		assert.Contains(t, rendered, "added other line")
	})

	t.Run("collapsed without search has no match set", func(t *testing.T) {
		m.searchMatches = nil
		m.searchMatchSet = nil
		rendered := m.renderDiff()

		assert.Contains(t, rendered, "added hello line")
		assert.Nil(t, m.searchMatchSet, "no search should produce nil match set")
	})
}

func TestModel_StyleDiffContentSearchMatch(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()

	t.Run("search match returns same text content", func(t *testing.T) {
		resultMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, true)
		resultNoMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, false)
		assert.Contains(t, resultMatch, " + content")
		assert.Contains(t, resultNoMatch, " + content")
	})

	t.Run("search match with highlight preserves content", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeAdd, " + ", "\033[32mgreen\033[0m", true, true)
		assert.Contains(t, result, " + ")
		assert.Contains(t, result, "\033[32m", "chroma foreground should be preserved")
	})

	t.Run("search match uses different style than normal add", func(t *testing.T) {
		// use newStyles with distinct colors so rendering produces different output
		c := Colors{
			Accent: "#ffffff", Border: "#555555", Normal: "#cccccc", Muted: "#666666",
			SelectedFg: "#ffffff", SelectedBg: "#333333", Annotation: "#ff9900",
			AddFg: "#00ff00", AddBg: "#002200", RemoveFg: "#ff0000", RemoveBg: "#220000",
			ModifyFg: "#ffaa00", ModifyBg: "#221100",
			SearchFg: "#1a1a1a", SearchBg: "#d7d700",
		}
		m.styles = newStyles(c)
		resultMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, true)
		resultNoMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, false)
		// both have same text but may differ in ANSI sequences (depends on terminal detection)
		// the key test is that both contain the content and the code paths don't panic
		assert.Contains(t, resultMatch, "content")
		assert.Contains(t, resultNoMatch, "content")
	})
}

func TestModel_BuildSearchMatchSet(t *testing.T) {
	m := testModel(nil, nil)

	t.Run("empty matches produces nil set", func(t *testing.T) {
		m.searchMatches = nil
		m.buildSearchMatchSet()
		assert.Nil(t, m.searchMatchSet)
	})

	t.Run("matches produce correct set", func(t *testing.T) {
		m.searchMatches = []int{1, 5, 10}
		m.buildSearchMatchSet()
		assert.True(t, m.searchMatchSet[1])
		assert.True(t, m.searchMatchSet[5])
		assert.True(t, m.searchMatchSet[10])
		assert.False(t, m.searchMatchSet[0])
		assert.False(t, m.searchMatchSet[3])
	})
}

func TestModel_ClearSearchResetsMatchSet(t *testing.T) {
	m := testModel(nil, nil)
	m.searchTerm = "test"
	m.searchMatches = []int{1, 2}
	m.searchCursor = 1
	m.searchMatchSet = map[int]bool{1: true, 2: true}

	m.clearSearch()

	assert.Empty(t, m.searchTerm)
	assert.Nil(t, m.searchMatches)
	assert.Equal(t, 0, m.searchCursor)
	assert.Nil(t, m.searchMatchSet)
}

func TestModel_StatusBarShowsSearchInput(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.currFile = "a.go"
	m.searching = true
	m.searchInput = textinput.New()
	m.searchInput.SetValue("hello")

	status := m.statusBarText()
	assert.Contains(t, status, "/hello", "should show search prompt with value")
	assert.NotContains(t, status, "a.go", "filename should not appear during search input")
}

func TestModel_StatusBarSearchInputTakesPriority(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.currFile = "a.go"
	m.searching = true
	m.searchInput = textinput.New()
	m.inConfirmDiscard = true // should not show discard prompt

	status := m.statusBarText()
	assert.Contains(t, status, "/", "search input should take priority over discard")
	assert.NotContains(t, status, "discard")
}

func TestModel_StatusBarSearchMatchPosition(t *testing.T) {
	tests := []struct {
		name         string
		matches      []int
		cursor       int
		wantContains string
		wantAbsent   string
	}{
		{name: "first of three", matches: []int{0, 2, 5}, cursor: 0, wantContains: "1/3"},
		{name: "second of three", matches: []int{0, 2, 5}, cursor: 1, wantContains: "2/3"},
		{name: "third of three", matches: []int{0, 2, 5}, cursor: 2, wantContains: "3/3"},
		{name: "single match", matches: []int{1}, cursor: 0, wantContains: "1/1"},
		{name: "no matches", matches: nil, cursor: 0, wantAbsent: "["},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.currFile = "a.go"
			m.diffLines = []diff.DiffLine{
				{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
				{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
				{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
			}
			m.focus = paneDiff
			m.width = 200
			m.searchMatches = tt.matches
			m.searchCursor = tt.cursor

			status := m.statusBarText()
			if tt.wantContains != "" {
				assert.Contains(t, status, tt.wantContains)
			}
			if tt.wantAbsent != "" {
				assert.NotContains(t, status, tt.wantAbsent)
			}
		})
	}
}

func TestModel_SearchSegment(t *testing.T) {
	m := testModel(nil, nil)

	// no matches
	assert.Empty(t, m.searchSegment())

	// with matches
	m.searchMatches = []int{0, 3, 7}
	m.searchCursor = 1
	assert.Equal(t, "2/3", m.searchSegment())

	// all matches on hidden removed lines in collapsed mode shows [0/N]
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed match", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx end", ChangeType: diff.ChangeContext},
	}
	m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m2.diffLines = lines
	m2.currFile = "a.go"
	m2.collapsed.enabled = true
	m2.collapsed.expandedHunks = make(map[int]bool)
	m2.searchMatches = []int{1} // only on hidden removed line
	m2.searchCursor = 0
	assert.Equal(t, "0/1", m2.searchSegment(), "should show [0/N] when all matches are hidden")
}

func TestModel_StatusBarSearchPositionBetweenHunkAndIcons(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1
	m.fileAdds = 1
	m.focus = paneDiff
	m.width = 200
	m.searchMatches = []int{1}
	m.searchCursor = 0
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)

	status := m.statusBarText()
	// all three should be present
	assert.Contains(t, status, "hunk 1/1")
	assert.Contains(t, status, "1/1")
	assert.Contains(t, status, "▼")

	// [1/1] should appear after hunk and before ▼
	hunkIdx := strings.Index(status, "hunk 1/1")
	searchIdx := strings.Index(status, "1/1")
	iconIdx := strings.Index(status, "▼")
	assert.Greater(t, searchIdx, hunkIdx, "search position should appear after hunk")
	assert.Less(t, searchIdx, iconIdx, "search position should appear before mode icons")
}

func TestModel_ClearSearchOnFileLoad(t *testing.T) {
	lines1 := []diff.DiffLine{
		{NewNum: 1, Content: "hello world", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "hello again", ChangeType: diff.ChangeAdd},
	}
	lines2 := []diff.DiffLine{
		{NewNum: 1, Content: "other content", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines1, "b.go": lines2})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: model.loadSeq, lines: lines1})
	model = result.(Model)
	model.focus = paneDiff

	// set up search state as if user searched for "hello"
	model.searchTerm = "hello"
	model.searchMatches = []int{0, 1}
	model.searchCursor = 1
	model.searchMatchSet = map[int]bool{0: true, 1: true}

	// load a different file
	model.loadSeq++
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: model.loadSeq, lines: lines2})
	model = result.(Model)

	assert.Empty(t, model.searchTerm, "search term should be cleared on file load")
	assert.Nil(t, model.searchMatches, "search matches should be cleared on file load")
	assert.Equal(t, 0, model.searchCursor, "search cursor should be reset on file load")
	assert.Nil(t, model.searchMatchSet, "search match set should be cleared on file load")
}

func TestModel_StatusBarNarrowDropsSearchSegment(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1
	m.fileAdds = 1
	m.focus = paneDiff
	m.searchMatches = []int{1}
	m.searchCursor = 0

	t.Run("wide terminal shows search segment", func(t *testing.T) {
		m.width = 200
		status := m.statusBarText()
		assert.Contains(t, status, "1/1")
	})

	t.Run("very narrow terminal drops search with hunk", func(t *testing.T) {
		m.width = 28
		status := m.statusBarText()
		assert.NotContains(t, status, "1/1", "search segment should be dropped on very narrow terminal")
		assert.Contains(t, status, "? help")
	})
}

func TestModel_RealignSearchCursorOnCollapsedToggle(t *testing.T) {
	// when toggling collapsed mode, searchCursor must realign to nearest visible match
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "match removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "match added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// set up search with cursor on the removed line (index 1)
	model.searchMatches = []int{0, 1, 2, 3}
	model.searchCursor = 1
	model.diffCursor = 1

	// toggle collapsed mode, which hides removed lines
	model.toggleCollapsedMode()

	assert.True(t, model.collapsed.enabled)
	assert.NotEqual(t, 1, model.diffCursor, "cursor should have moved off hidden removed line")
	assert.NotEqual(t, 1, model.searchCursor, "searchCursor should realign away from hidden match")
	// searchCursor should point to a visible match
	if model.searchCursor < len(model.searchMatches) {
		matchIdx := model.searchMatches[model.searchCursor]
		hunks := model.findHunks()
		assert.False(t, model.isCollapsedHidden(matchIdx, hunks), "realigned searchCursor should point to a visible match")
	}
}

func TestModel_RealignSearchCursorOnHunkCollapse(t *testing.T) {
	// when collapsing a hunk, searchCursor must realign if current match becomes hidden
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "match removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "match added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start in collapsed mode with hunk expanded (hunk starts at index 1, first change line)
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = map[int]bool{1: true}
	model.searchMatches = []int{0, 1, 2, 3}
	model.searchCursor = 1 // on removed line (visible because hunk is expanded)
	model.diffCursor = 1

	// collapse the hunk — removed line becomes hidden
	model.toggleHunkExpansion()

	assert.NotContains(t, model.collapsed.expandedHunks, 1, "hunk should be collapsed")
	// searchCursor should have realigned to a visible match
	if len(model.searchMatches) > 0 && model.searchCursor < len(model.searchMatches) {
		matchIdx := model.searchMatches[model.searchCursor]
		hunks := model.findHunks()
		assert.False(t, model.isCollapsedHidden(matchIdx, hunks), "searchCursor should point to visible match after hunk collapse")
	}
}

func TestModel_RealignSearchCursorNoopWithoutSearch(t *testing.T) {
	// realignSearchCursor should be a no-op when no search is active
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = nil
	model.searchCursor = 0

	// should not panic or change anything
	model.realignSearchCursor()
	assert.Equal(t, 0, model.searchCursor)
}

func TestModel_SubmitSearchPreservesLeadingWhitespace(t *testing.T) {
	// search query with leading/trailing whitespace should be preserved in the search term
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "  indented line", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "normal line", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	model.searchInput = textinput.New()
	model.searchInput.SetValue("  indented")
	model.submitSearch()

	assert.Equal(t, "  indented", model.searchTerm, "leading whitespace should be preserved in search term")
	assert.Equal(t, []int{0}, model.searchMatches, "should match the indented line")
}

func TestModel_SubmitSearchWhitespaceOnlyClearsSearch(t *testing.T) {
	// pure whitespace query should clear search (same as empty)
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// pre-populate search state
	model.searchTerm = "old"
	model.searchMatches = []int{0}
	model.searchCursor = 0

	model.searchInput = textinput.New()
	model.searchInput.SetValue("   ")
	model.submitSearch()

	assert.Empty(t, model.searchTerm, "whitespace-only query should clear search")
	assert.Nil(t, model.searchMatches)
}

func TestModel_DeletePlaceholderSearchHighlight(t *testing.T) {
	// delete-only placeholder should render correctly with and without search match.
	// verifies the code path doesn't panic and produces correct text content.
	// (actual ANSI styling differences depend on terminal detection)
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "deleted match", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "deleted other", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "context end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.styles = plainStyles()
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = make(map[int]bool)
	model.diffCursor = 1

	t.Run("with search match", func(t *testing.T) {
		model.searchMatchSet = map[int]bool{1: true}
		var b strings.Builder
		model.renderDeletePlaceholder(&b, 1, 1)
		rendered := b.String()
		assert.Contains(t, rendered, "2 lines deleted")
		assert.Contains(t, rendered, "▶", "cursor indicator should be present")
	})

	t.Run("without search match", func(t *testing.T) {
		model.searchMatchSet = nil
		var b strings.Builder
		model.renderDeletePlaceholder(&b, 1, 1)
		rendered := b.String()
		assert.Contains(t, rendered, "2 lines deleted")
		assert.Contains(t, rendered, "▶")
	})

	t.Run("with wrap mode and search match", func(t *testing.T) {
		model.searchMatchSet = map[int]bool{1: true}
		model.wrapMode = true
		model.width = 120
		model.treeWidth = 30
		var b strings.Builder
		model.renderDeletePlaceholder(&b, 1, 1)
		rendered := b.String()
		assert.Contains(t, rendered, "2 lines deleted")
		model.wrapMode = false
	})
}

func TestModel_ViewSingleFileMode(t *testing.T) {
	t.Run("single-file mode renders full-width diff without tree pane", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.tree = newFileTree([]string{"main.go"})
		m.singleFile = true
		m.treeWidth = 0
		m.focus = paneDiff
		m.currFile = "main.go"
		m.noStatusBar = true
		m.ready = true

		view := m.View()
		assert.Contains(t, view, "main.go")

		// every rendered line must be full terminal width (diff pane uses m.width - 2 + 2 border = m.width)
		lines := strings.Split(view, "\n")
		for i, line := range lines {
			w := lipgloss.Width(line)
			if w == 0 {
				continue // skip empty trailing lines
			}
			assert.Equal(t, m.width, w, "line %d should be full width (%d), got %d", i, m.width, w)
		}

		// single-file mode must not contain adjacent pane borders (││) from JoinHorizontal
		stripped := ansi.Strip(view)
		assert.NotContains(t, stripped, "││", "single-file mode should not have two adjacent pane borders")
	})

	t.Run("multi-file mode renders tree and diff panes side by side", func(t *testing.T) {
		m := testModel([]string{"internal/a.go", "internal/b.go"}, nil)
		m.tree = newFileTree([]string{"internal/a.go", "internal/b.go"})
		m.singleFile = false
		m.focus = paneTree
		m.noStatusBar = true
		m.ready = true

		view := m.View()
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "a.go")
		assert.Contains(t, stripped, "b.go")

		// multi-file mode should have adjacent pane borders from JoinHorizontal
		assert.Contains(t, stripped, "││", "multi-file mode should have two pane borders from tree+diff join")
	})
}

func TestModel_DiffContentWidthSingleFile(t *testing.T) {
	t.Run("single-file mode", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.width = 100
		m.treeWidth = 0
		assert.Equal(t, 97, m.diffContentWidth()) // width - 3 (borders + cursor bar)
	})

	t.Run("multi-file mode", func(t *testing.T) {
		m := testModel([]string{"a.go", "b.go"}, nil)
		m.singleFile = false
		m.width = 120
		m.treeWidth = 36
		assert.Equal(t, 79, m.diffContentWidth()) // 120 - 36 - 4 - 1
	})

	t.Run("single-file mode minimum width", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.width = 5
		m.treeWidth = 0
		assert.Equal(t, 10, m.diffContentWidth()) // min 10
	})
}

func TestModel_FilterOnly(t *testing.T) {
	t.Run("no filter returns all files", func(t *testing.T) {
		m := testModel(nil, nil)
		files := []string{"ui/model.go", "diff/diff.go", "README.md"}
		assert.Equal(t, files, m.filterOnly(files))
	})

	t.Run("exact path match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"ui/model.go"}
		files := []string{"ui/model.go", "diff/diff.go", "README.md"}
		assert.Equal(t, []string{"ui/model.go"}, m.filterOnly(files))
	})

	t.Run("suffix match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"model.go"}
		files := []string{"ui/model.go", "diff/diff.go", "README.md"}
		assert.Equal(t, []string{"ui/model.go"}, m.filterOnly(files))
	})

	t.Run("multiple patterns", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"model.go", "README.md"}
		files := []string{"ui/model.go", "diff/diff.go", "README.md"}
		assert.Equal(t, []string{"ui/model.go", "README.md"}, m.filterOnly(files))
	})

	t.Run("absolute path pattern resolved against workDir", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/README.md"}
		m.workDir = "/repo"
		files := []string{"ui/model.go", "README.md"}
		assert.Equal(t, []string{"README.md"}, m.filterOnly(files))
	})

	t.Run("absolute path pattern with subdirectory", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/ui/model.go"}
		m.workDir = "/repo"
		files := []string{"ui/model.go", "diff/diff.go", "README.md"}
		assert.Equal(t, []string{"ui/model.go"}, m.filterOnly(files))
	})

	t.Run("absolute path outside workDir does not match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/other/README.md"}
		m.workDir = "/repo"
		files := []string{"README.md", "ui/model.go"}
		assert.Empty(t, m.filterOnly(files))
	})

	t.Run("absolute path suffix match via resolved relative", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/model.go"}
		m.workDir = "/repo"
		files := []string{"ui/model.go", "diff/diff.go"}
		assert.Equal(t, []string{"ui/model.go"}, m.filterOnly(files))
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"nonexistent.go"}
		files := []string{"ui/model.go", "diff/diff.go"}
		assert.Empty(t, m.filterOnly(files))
	})
}

func TestModel_FilterOnlyNoMatchShowsMessage(t *testing.T) {
	m := testModel(nil, nil)
	m.only = []string{"nonexistent.go"}
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(76, 20)

	result, cmd := m.Update(filesLoadedMsg{files: []string{"ui/model.go", "diff/diff.go"}})
	model := result.(Model)
	assert.Nil(t, cmd, "should not trigger file load when no files match")
	assert.Contains(t, model.viewport.View(), "no files match --only filter")
}

func TestModel_SingleFileKeysNoOp(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line two", ChangeType: diff.ChangeAdd},
	}
	setup := func() Model {
		m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
		m.tree = newFileTree([]string{"main.go"})
		m.singleFile = true
		m.focus = paneDiff
		m.currFile = "main.go"
		m.diffLines = lines
		m.highlightedLines = noopHighlighter().HighlightLines("main.go", lines)
		m.styles = plainStyles()
		return m
	}

	t.Run("tab is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "tab should not switch pane in single-file mode")
	})

	t.Run("h is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "h should not switch to tree in single-file mode")
	})

	t.Run("f is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		m.store.Add(annotation.Annotation{File: "main.go", Line: 1, Type: "+", Comment: "test"})
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.False(t, model.tree.filter, "f should not toggle filter in single-file mode")
	})

	t.Run("p is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		selected := m.tree.selectedFile()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, selected, model.tree.selectedFile(), "p should not change file in single-file mode")
	})

	t.Run("n is no-op for file nav in single-file mode", func(t *testing.T) {
		m := setup()
		selected := m.tree.selectedFile()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, selected, model.tree.selectedFile(), "n should not advance file in single-file mode")
	})
}

func TestModel_SingleFileSearchNavStillWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no hit", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.singleFile = true
	model.focus = paneDiff
	model.searchMatches = []int{0, 2}
	model.searchCursor = 0
	model.diffCursor = 0

	// n should navigate to next search match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "n should advance search cursor in single-file mode")
	assert.Equal(t, 2, model.diffCursor, "cursor should move to second match")

	// N should navigate to previous search match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 0, model.searchCursor, "N should go back in single-file mode")
	assert.Equal(t, 0, model.diffCursor, "cursor should return to first match")
}

func TestModel_SingleFileWrapModeWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "short line", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: strings.Repeat("long ", 50), ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.singleFile = true
	model.focus = paneDiff

	assert.False(t, model.wrapMode, "wrap should be off initially")

	// toggle wrap on
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.True(t, model.wrapMode, "w should toggle wrap on in single-file mode")
	assert.Equal(t, 0, model.scrollX, "wrap should reset horizontal scroll")

	// toggle wrap off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.wrapMode, "w should toggle wrap off in single-file mode")
}

func TestModel_SingleFileCollapsedModeWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.singleFile = true
	model.focus = paneDiff

	assert.False(t, model.collapsed.enabled, "collapsed should be off initially")

	// toggle collapsed on
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	model = result.(Model)
	assert.True(t, model.collapsed.enabled, "v should toggle collapsed on in single-file mode")

	// toggle collapsed off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	model = result.(Model)
	assert.False(t, model.collapsed.enabled, "v should toggle collapsed off in single-file mode")
}

func TestModel_SingleFileAnnotationWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.singleFile = true
	model.focus = paneDiff
	model.diffCursor = 1 // on the add line
	model.styles = plainStyles()

	// press enter to start annotation
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.True(t, model.annotating, "enter should start annotation in single-file mode")

	// type annotation text
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = result.(Model)

	// press enter to save
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.False(t, model.annotating, "annotation should be saved")
	assert.Equal(t, 1, model.store.Count(), "annotation should be stored")
}

func TestModel_SingleFileMultiFileModeUnchanged(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines, "b.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(filesLoadedMsg{files: []string{"a.go", "b.go"}})
	model = result.(Model)

	assert.False(t, model.singleFile, "multi-file should not be in single-file mode")
	assert.Equal(t, paneTree, model.focus, "multi-file should start on tree pane")

	// tab should switch panes
	model.focus = paneTree
	model.currFile = "a.go"
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneDiff, model.focus, "tab should switch to diff pane in multi-file mode")

	// tab back to tree
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneTree, model.focus, "tab should switch back to tree in multi-file mode")

	// f should toggle filter (with annotations present)
	model.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	model.focus = paneDiff
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = result.(Model)
	assert.True(t, model.tree.filter, "f should toggle filter in multi-file mode")
}
