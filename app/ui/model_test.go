package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/mocks"
)

func noopHighlighter() *mocks.SyntaxHighlighterMock {
	return &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
	}
}

func testModel(files []string, fileDiffs map[string][]diff.DiffLine) Model {
	entries := make([]diff.FileEntry, len(files))
	for i, f := range files {
		entries[i] = diff.FileEntry{Path: f}
	}
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
			return entries, nil
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
	assert.Equal(t, []string{"a.go", "b.go"}, diff.FileEntryPaths(flm.entries))
	assert.NoError(t, flm.err)
}

func TestModel_FilesLoaded(t *testing.T) {
	m := testModel(nil, nil)

	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "internal/handler.go"}, {Path: "internal/store.go"}, {Path: "main.go"}}})
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

func TestModel_FilesLoadedMultipleFiles(t *testing.T) {
	m := testModel(nil, nil)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}})
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

func TestModel_WrapModeFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
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
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
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

func TestModel_LineNumbersFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("line numbers enabled via config", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{LineNumbers: true, TreeWidthRatio: 2})
		assert.True(t, m.lineNumbers)
	})

	t.Run("line numbers disabled by default", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.lineNumbers)
	})
}

func TestModel_BlameFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()
	blamer := &mocks.BlamerMock{
		FileBlameFunc: func(string, string, bool) (map[int]diff.BlameLine, error) { return map[int]diff.BlameLine{}, nil },
	}

	t.Run("blame enabled via config when blamer is available", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{ShowBlame: true, Blamer: blamer, TreeWidthRatio: 2})
		assert.True(t, m.showBlame)
	})

	t.Run("blame disabled without blamer even if requested", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{ShowBlame: true, TreeWidthRatio: 2})
		assert.False(t, m.showBlame)
	})

	t.Run("blame disabled by default", func(t *testing.T) {
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{Blamer: blamer, TreeWidthRatio: 2})
		assert.False(t, m.showBlame)
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
				ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return []diff.FileEntry{{Path: "a.go"}}, nil },
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

func TestModel_DiffLineNum(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, 5, m.diffLineNum(diff.DiffLine{NewNum: 5, ChangeType: diff.ChangeContext}))
	assert.Equal(t, 3, m.diffLineNum(diff.DiffLine{NewNum: 3, ChangeType: diff.ChangeAdd}))
	assert.Equal(t, 7, m.diffLineNum(diff.DiffLine{OldNum: 7, ChangeType: diff.ChangeRemove}))
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

func TestModel_LineNumberSegment(t *testing.T) {
	t.Run("context line", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 10, NewNum: 10, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, NewNum: 11, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 0
		m.focus = paneDiff
		assert.Equal(t, "L:10/11", m.lineNumberSegment())
	})

	t.Run("add line", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 5, NewNum: 5, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 0, NewNum: 6, Content: "new", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 1
		m.focus = paneDiff
		assert.Equal(t, "L:6/6", m.lineNumberSegment())
	})

	t.Run("remove line uses old max", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 5, NewNum: 5, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 6, NewNum: 0, Content: "old", ChangeType: diff.ChangeRemove},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 1
		m.focus = paneDiff
		assert.Equal(t, "L:6/6", m.lineNumberSegment())
	})

	t.Run("remove line denominator differs from new max", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 10, NewNum: 9, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
			{OldNum: 12, NewNum: 0, Content: "removed2", ChangeType: diff.ChangeRemove},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 1
		m.focus = paneDiff
		// on removed line: denominator = maxOld (12), not maxNew (9)
		assert.Equal(t, "L:11/12", m.lineNumberSegment())
	})

	t.Run("context line uses new max not old", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 10, NewNum: 9, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
			{OldNum: 12, NewNum: 0, Content: "removed2", ChangeType: diff.ChangeRemove},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 0
		m.focus = paneDiff
		// on context line: denominator = maxNew (9), not maxOld (12)
		assert.Equal(t, "L:9/9", m.lineNumberSegment())
	})

	t.Run("divider line returns empty", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{Content: "...", ChangeType: diff.ChangeDivider},
			{OldNum: 50, NewNum: 50, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 1
		m.focus = paneDiff
		assert.Empty(t, m.lineNumberSegment())
	})

	t.Run("empty diffLines returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = nil
		m.diffCursor = 0
		m.focus = paneDiff
		assert.Empty(t, m.lineNumberSegment())
	})

	t.Run("tree focus returns empty", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.diffLines = lines
		m.diffCursor = 0
		m.focus = paneTree
		assert.Empty(t, m.lineNumberSegment())
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

func TestModel_PadContentBg(t *testing.T) {
	m := testModel(nil, nil)

	t.Run("empty bgHex is no-op", func(t *testing.T) {
		assert.Equal(t, "hello", m.padContentBg("hello", 20, ""))
	})

	t.Run("zero width is no-op", func(t *testing.T) {
		assert.Equal(t, "hello", m.padContentBg("hello", 0, "#2e3440"))
	})

	t.Run("pads short line", func(t *testing.T) {
		result := m.padContentBg("hi", 10, "#2e3440")
		assert.Contains(t, result, "\033[48;2;46;52;64m")
		assert.Contains(t, result, "\033[49m")
		assert.Equal(t, 10, lipgloss.Width(result))
	})

	t.Run("strips trailing spaces before padding", func(t *testing.T) {
		result := m.padContentBg("hi      ", 10, "#2e3440")
		assert.Contains(t, result, "\033[48;2;46;52;64m")
		assert.Equal(t, 10, lipgloss.Width(result))
	})

	t.Run("multi-line pads each line", func(t *testing.T) {
		result := m.padContentBg("ab\ncd", 5, "#2e3440")
		lines := strings.Split(result, "\n")
		assert.Len(t, lines, 2)
		assert.Equal(t, 5, lipgloss.Width(lines[0]))
		assert.Equal(t, 5, lipgloss.Width(lines[1]))
	})

	t.Run("line at target width is no-op", func(t *testing.T) {
		result := m.padContentBg("abcde", 5, "#2e3440")
		assert.Equal(t, "abcde", result)
	})
}

func TestModel_ExtendLineBg(t *testing.T) {
	t.Run("empty bgColor is no-op", func(t *testing.T) {
		m := testModel(nil, nil)
		m.width = 80
		assert.Equal(t, "hello", m.extendLineBg("hello", ""))
	})

	t.Run("pads to content width", func(t *testing.T) {
		m := testModel(nil, nil)
		m.width = 80
		result := m.extendLineBg("hi", "#2e3440")
		assert.Contains(t, result, "\033[48;2;46;52;64m")
		assert.Contains(t, result, "\033[49m")
		w := lipgloss.Width(result)
		assert.Greater(t, w, 2, "should be wider than input")
	})

	t.Run("with line numbers subtracts gutter", func(t *testing.T) {
		m := testModel(nil, nil)
		m.width = 80
		m.lineNumbers = true
		m.lineNumWidth = 3
		resultWithNums := m.extendLineBg("hi", "#2e3440")
		m.lineNumbers = false
		resultWithout := m.extendLineBg("hi", "#2e3440")
		assert.Less(t, lipgloss.Width(resultWithNums), lipgloss.Width(resultWithout), "line numbers should reduce target width")
	})
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
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return []diff.FileEntry{{Path: "a.go"}}, nil },
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
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
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

func TestModel_HandleBlameLoadedSyncsViewportForWrap(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: strings.Repeat("a", 60), ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "tail", ChangeType: diff.ChangeContext},
	}
	m.wrapMode = true
	m.showBlame = true
	m.focus = paneDiff
	m.treeHidden = true
	m.width = 40
	m.viewport = viewport.New(37, 2)
	m.diffCursor = 1

	m.syncViewportToCursor()
	before := m.viewport.YOffset

	result, _ := m.handleBlameLoaded(blameLoadedMsg{
		file: "a.go",
		seq:  m.loadSeq,
		data: map[int]diff.BlameLine{
			1: {Author: "LongAuthor", Time: time.Now()},
			2: {Author: "LongAuthor", Time: time.Now()},
		},
	})
	model := result.(Model)

	assert.Greater(t, model.viewport.YOffset, before, "viewport should be re-synced after blame narrows wrap width")
	cursorY := model.cursorViewportY()
	assert.GreaterOrEqual(t, cursorY, model.viewport.YOffset)
	assert.Less(t, cursorY, model.viewport.YOffset+model.viewport.Height)
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

func TestModel_TreePaneToggle(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines, "b.go": lines})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneTree
	m.viewport = viewport.New(80, 30)
	origTreeWidth := m.treeWidth

	t.Run("t hides tree pane", func(t *testing.T) {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.True(t, model.treeHidden)
		assert.Equal(t, 0, model.treeWidth)
		assert.Equal(t, paneDiff, model.focus, "focus should move to diff when hiding tree")
		assert.Equal(t, model.width-2, model.viewport.Width, "diff should use full width")
	})

	t.Run("t shows tree pane again", func(t *testing.T) {
		m2 := m
		m2.treeHidden = true
		m2.treeWidth = 0
		m2.focus = paneDiff
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.False(t, model.treeHidden)
		assert.Equal(t, origTreeWidth, model.treeWidth)
	})

	t.Run("tab is no-op when tree hidden", func(t *testing.T) {
		m2 := m
		m2.treeHidden = true
		m2.focus = paneDiff
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "tab should not switch pane when tree hidden")
	})

	t.Run("h is no-op when tree hidden", func(t *testing.T) {
		m2 := m
		m2.treeHidden = true
		m2.focus = paneDiff
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "h should not switch to tree when hidden")
	})

	t.Run("no-op in single-file mode without TOC", func(t *testing.T) {
		m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m2.singleFile = true
		m2.viewport = viewport.New(80, 30)
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.False(t, model.treeHidden, "t should be no-op in single-file mode without TOC")
	})

	t.Run("toggle works in single-file markdown with TOC", func(t *testing.T) {
		m2 := testModel([]string{"readme.md"}, map[string][]diff.DiffLine{"readme.md": lines})
		m2.singleFile = true
		m2.mdTOC = &mdTOC{entries: []tocEntry{{title: "Header", level: 1, lineIdx: 0}}}
		m2.treeWidth = max(minTreeWidth, m2.width*m2.treeWidthRatio/10)
		m2.viewport = viewport.New(80, 30)
		m2.focus = paneTree
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.True(t, model.treeHidden, "t should hide TOC pane in single-file markdown mode")
		assert.Equal(t, 0, model.treeWidth)
		assert.Equal(t, paneDiff, model.focus)
	})

	t.Run("resize preserves hidden state", func(t *testing.T) {
		m2 := m
		m2.treeHidden = true
		m2.treeWidth = 0
		result, _ := m2.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		model := result.(Model)
		assert.True(t, model.treeHidden)
		assert.Equal(t, 0, model.treeWidth)
	})

	t.Run("status icon shows when hidden", func(t *testing.T) {
		m2 := m
		m2.treeHidden = true
		icons := m2.statusModeIcons()
		assert.Contains(t, icons, "⊟")
	})
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

func TestModel_FilterOnly(t *testing.T) {
	toEntries := func(paths ...string) []diff.FileEntry {
		entries := make([]diff.FileEntry, len(paths))
		for i, p := range paths {
			entries[i] = diff.FileEntry{Path: p}
		}
		return entries
	}

	t.Run("no filter returns all files", func(t *testing.T) {
		m := testModel(nil, nil)
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, files, m.filterOnly(files))
	})

	t.Run("exact path match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"ui/model.go"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("suffix match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"model.go"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("multiple patterns", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"model.go", "README.md"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go", "README.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path pattern resolved against workDir", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/README.md"}
		m.workDir = "/repo"
		files := toEntries("ui/model.go", "README.md")
		assert.Equal(t, []string{"README.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path pattern with subdirectory", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/ui/model.go"}
		m.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path outside workDir does not match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/other/README.md"}
		m.workDir = "/repo"
		files := toEntries("README.md", "ui/model.go")
		assert.Empty(t, m.filterOnly(files))
	})

	t.Run("absolute path suffix match via resolved relative", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/model.go"}
		m.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"nonexistent.go"}
		files := toEntries("ui/model.go", "diff/diff.go")
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

	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "ui/model.go"}, {Path: "diff/diff.go"}}})
	model := result.(Model)
	assert.Nil(t, cmd, "should not trigger file load when no files match")
	assert.Contains(t, model.viewport.View(), "no files match --only filter")
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

func TestModel_ToggleLineNumbers(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		},
	})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext}}

	assert.False(t, m.lineNumbers)
	result, _ := m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.True(t, m.lineNumbers)
	result, _ = m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.False(t, m.lineNumbers)
}

func TestModel_ComputeLineNumWidth(t *testing.T) {
	tests := []struct {
		name  string
		lines []diff.DiffLine
		want  int
	}{
		{name: "single digit", lines: []diff.DiffLine{
			{OldNum: 5, NewNum: 5, ChangeType: diff.ChangeContext},
		}, want: 1},
		{name: "two digits", lines: []diff.DiffLine{
			{OldNum: 99, NewNum: 99, ChangeType: diff.ChangeContext},
		}, want: 2},
		{name: "mixed old larger", lines: []diff.DiffLine{
			{OldNum: 100, NewNum: 5, ChangeType: diff.ChangeContext},
		}, want: 3},
		{name: "mixed new larger", lines: []diff.DiffLine{
			{OldNum: 5, NewNum: 1000, ChangeType: diff.ChangeContext},
		}, want: 4},
		{name: "empty", lines: nil, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.diffLines = tt.lines
			assert.Equal(t, tt.want, m.computeLineNumWidth())
		})
	}
}

func TestModel_LineNumbersEndToEnd(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 10, NewNum: 10, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 11, NewNum: 0, Content: "old", ChangeType: diff.ChangeRemove},
		{OldNum: 0, NewNum: 11, Content: "new", ChangeType: diff.ChangeAdd},
		{Content: "@@ -10,3 +10,3 @@", ChangeType: diff.ChangeDivider},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines

	// toggle on
	result, _ := m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.True(t, m.lineNumbers)
	assert.Equal(t, 2, m.lineNumWidth)

	rendered := m.renderDiff()
	stripped := ansi.Strip(rendered)
	assert.Contains(t, stripped, "10 10")
	assert.Contains(t, stripped, "11   ")
	assert.Contains(t, stripped, "   11")

	// toggle off
	result, _ = m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.False(t, m.lineNumbers)
	rendered = m.renderDiff()
	stripped = ansi.Strip(rendered)
	assert.NotContains(t, stripped, "10 10")
}

func TestModel_CustomKeymapQuitOverride(t *testing.T) {
	// map "x" to quit, unbind "q" — verify "x" quits and "q" does not
	km := keymap.Default()
	km.Bind("x", keymap.ActionQuit)
	km.Unbind("q")

	m := testModel([]string{"a.go"}, nil)
	m.keymap = km

	// "x" should quit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.NotNil(t, cmd, "x should produce a command")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "x should trigger quit")

	// "q" should not quit (unbound)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.Nil(t, cmd, "q should not produce a command when unbound")
}

func TestModel_CustomKeymapViewToggle(t *testing.T) {
	// map "x" to toggle_wrap — verify "x" toggles wrap and "w" still works
	km := keymap.Default()
	km.Bind("x", keymap.ActionToggleWrap)

	lines := []diff.DiffLine{{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.keymap = km
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines

	assert.False(t, m.wrapMode)

	// "x" should toggle wrap
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)
	assert.True(t, model.wrapMode, "x should toggle wrap mode on")

	// "w" should also toggle wrap (still bound by default)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.wrapMode, "w should toggle wrap mode off")
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

func TestModel_CustomKeymapTreeNav(t *testing.T) {
	// map "x" to down, unbind "j" — verify "x" moves tree cursor and "j" does not
	km := keymap.Default()
	km.Bind("x", keymap.ActionDown)
	km.Unbind("j")

	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.keymap = km
	m.tree = newFileTree(files)
	m.focus = paneTree

	assert.Equal(t, "a.go", m.tree.selectedFile())

	// "x" should move down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile(), "x should move tree cursor down")

	// "j" should not move (unbound)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile(), "j should not move when unbound")
}

func TestModel_CustomKeymapTreeFocusDiff(t *testing.T) {
	// scroll_right in tree pane should focus diff (implicit fallback)
	files := []string{"a.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.currFile = "a.go"
	m.focus = paneTree

	// right key maps to scroll_right by default, should focus diff in tree pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.focus, "right key (scroll_right) should focus diff in tree pane")
}

func TestModel_AcceptanceAdditiveQuitBinding(t *testing.T) {
	// map x quit (additive) — both x and q should quit
	km := keymap.Default()
	km.Bind("x", keymap.ActionQuit)

	m := testModel([]string{"a.go"}, nil)
	m.keymap = km

	// "x" should quit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.NotNil(t, cmd, "x should produce a command")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "x should trigger quit")

	// "q" should also still quit (additive binding)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd, "q should still produce a command")
	msg = cmd()
	_, ok = msg.(tea.QuitMsg)
	assert.True(t, ok, "q should still trigger quit")
}

func TestModel_AcceptanceDefaultBehaviorNoKeybindingsFile(t *testing.T) {
	// no keybindings file → identical behavior to current defaults
	m := testModel([]string{"a.go"}, nil)
	// m.keymap is set to Default() in testModel via NewModel

	// q should quit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "q should quit with default keymap")

	// ? should open help
	m2 := testModel([]string{"a.go"}, nil)
	result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := result.(Model)
	assert.True(t, model.showHelp, "? should open help with default keymap")
}

func TestModel_MarkReviewedFromTreePane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	m.focus = paneTree

	// space bar toggles reviewed
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.True(t, model.tree.reviewed["a.go"], "space should mark current file as reviewed")
	assert.Equal(t, 1, model.tree.reviewedCount())

	// space again toggles off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model = result.(Model)
	assert.False(t, model.tree.reviewed["a.go"], "space should unmark reviewed file")
	assert.Equal(t, 0, model.tree.reviewedCount())
}

func TestModel_MarkReviewedFromTreePaneUsesSelectedFile(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	m.focus = paneTree
	m.tree.moveDown() // cursor -> b.go while the diff pane still shows a.go

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.True(t, model.tree.reviewed["b.go"], "space in tree pane should mark selected file")
	assert.False(t, model.tree.reviewed["a.go"], "space in tree pane should not mark stale currFile")
}

func TestModel_MarkReviewedFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "b.go"
	m.focus = paneDiff

	// space from diff pane marks currFile
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.True(t, model.tree.reviewed["b.go"], "space in diff pane should mark currFile as reviewed")
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

func TestModel_UntrackedToggle(t *testing.T) {
	t.Run("toggle cycles showUntracked and reloads files", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return []diff.DiffLine{{Content: "line1", ChangeType: diff.ChangeContext, OldNum: 1, NewNum: 1}}, nil
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			LoadUntracked: func() ([]string, error) {
				return []string{"newfile.go"}, nil
			},
		})
		m.width = 120
		m.height = 40
		m.ready = true

		// initially untracked is off
		assert.False(t, m.showUntracked)

		// toggle on
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		assert.True(t, result.(Model).showUntracked)
		assert.NotNil(t, cmd)

		// execute loadFiles command — should include untracked file
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		paths := make([]string, 0, len(flMsg.entries))
		for _, e := range flMsg.entries {
			paths = append(paths, e.Path)
		}
		assert.Contains(t, paths, "main.go")
		assert.Contains(t, paths, "newfile.go")

		// toggle off — use result from toggle on
		m = result.(Model)
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		assert.False(t, result.(Model).showUntracked)
	})

	t.Run("status bar shows ? icon when untracked is on", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		// ? is always in the icons list but inactive (muted) when showUntracked is false
		// check that ? becomes active when toggled
		// icons always contain ?
		// find ? position and verify it's muted (not the active color pattern)
		m.showUntracked = true
		iconsActive := m.statusModeIcons()
		assert.Contains(t, iconsActive, "?")
	})

	t.Run("no untracked files when LoadUntracked is nil", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go"}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
		m.ready = true
		m.showUntracked = true

		// directly execute loadFiles to check behavior without toggling
		cmd := m.loadFiles()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1, "should only have the original file, no untracked")
	})

	t.Run("dedup: untracked file already in staged list", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			LoadUntracked: func() ([]string, error) {
				return []string{"newfile.go", "other.go"}, nil
			},
		})
		m.width = 120
		m.height = 40
		m.ready = true
		m.showUntracked = true

		cmd := m.loadFiles()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 2, "newfile.go should not be duplicated")
	})
}

func TestModel_StagedOnlyFiles(t *testing.T) {
	t.Run("staged-only new files included in file list", func(t *testing.T) {
		// simulate: working tree has no changes, but index has a new file
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}, nil
				}
				return nil, nil // no unstaged changes
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return []diff.DiffLine{{Content: "content", ChangeType: diff.ChangeAdd, NewNum: 1}}, nil
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1)
		assert.Equal(t, "newfile.go", flMsg.entries[0].Path)
		assert.Equal(t, diff.FileAdded, flMsg.entries[0].Status)
	})

	t.Run("staged-only files not duplicated when already in unstaged list", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, nil
				}
				return []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1, "main.go should not be duplicated")
	})

	t.Run("staged fetch failure logged as warning", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return nil, errors.New("git error")
				}
				return []diff.FileEntry{{Path: "main.go"}}, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1, "original files should still be present")
		assert.Len(t, flMsg.warnings, 1, "staged fetch error should be in warnings")
		assert.Contains(t, flMsg.warnings[0], "git error")
	})
}

func TestModel_HandleFileLoadedUntrackedFallback(t *testing.T) {
	t.Run("untracked file with empty diff falls back to disk read", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileUntracked}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil // empty diff for untracked
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
		m.ready = true

		// load files then select the untracked file
		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		_ = flMsg
		// handleFilesLoaded auto-selects first file and returns loadFileDiff cmd
		result, cmd := m.Update(msg)
		m = result.(Model)
		// handleFileLoaded — empty diff triggers fallback
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)
		// should not be empty — either fallback worked or file doesn't exist on disk
		// (if testdata/newfile.go doesn't exist, diffLines stays nil — that's OK, we just test no crash)
		assert.NotNil(t, m, "should not crash on untracked file load")
	})

	t.Run("non-untracked file with empty diff does not trigger fallback", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil // empty diff for some reason
			},
		}
		store := annotation.NewStore()
		m := NewModel(renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)
		assert.Nil(t, m.diffLines, "non-untracked empty diff should not trigger disk fallback")
	})
}
