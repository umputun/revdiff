package ui

import (
	"testing"

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
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "monokai" },
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
