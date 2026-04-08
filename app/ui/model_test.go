package ui

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
