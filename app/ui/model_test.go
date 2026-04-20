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
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

func noopHighlighter() *mocks.SyntaxHighlighterMock {
	return &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "monokai" },
	}
}

// plainRenderer returns a RendererMock that returns an empty file list and no diff lines.
func plainRenderer() *mocks.RendererMock {
	return &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(ref, file string, staged bool) ([]diff.DiffLine, error) { return nil, nil },
	}
}

// testFileTreeFactory returns the sidepane factory closures for NewFileTree and ParseTOC,
// suitable for injection into ModelConfig in tests.
func testFileTreeFactory() func(entries []diff.FileEntry) FileTreeComponent {
	return func(entries []diff.FileEntry) FileTreeComponent {
		return sidepane.NewFileTree(entries)
	}
}

func testParseTOCFactory() func(lines []diff.DiffLine, filename string) TOCComponent {
	return func(lines []diff.DiffLine, filename string) TOCComponent {
		toc := sidepane.ParseTOC(lines, filename)
		if toc == nil {
			return nil
		}
		return toc
	}
}

// moveTOCTo positions the TOC cursor at the given entry index via the production Move API.
func moveTOCTo(toc TOCComponent, idx int) {
	toc.Move(sidepane.MotionFirst)
	if idx > 0 {
		toc.Move(sidepane.MotionPageDown, idx)
	}
}

// tocLineIdx returns the current TOC cursor's diff line index for assertions.
func tocLineIdx(t *testing.T, toc TOCComponent) int {
	t.Helper()
	idx, ok := toc.CurrentLineIdx()
	require.True(t, ok, "expected valid TOC cursor position")
	return idx
}

// tocActiveLineIdx calls SyncCursorToActiveSection and returns the resulting line index.
// this mutates cursor state — use as the last assertion in a test.
func tocActiveLineIdx(t *testing.T, toc TOCComponent) int {
	t.Helper()
	toc.SyncCursorToActiveSection()
	return tocLineIdx(t, toc)
}

// testNewFileTree creates a sidepane.FileTree from a list of file paths,
// replacing the old testNewFileTree([]string{...}) pattern in tests.
func testNewFileTree(files []string) *sidepane.FileTree {
	entries := make([]diff.FileEntry, len(files))
	for i, f := range files {
		entries[i] = diff.FileEntry{Path: f}
	}
	return sidepane.NewFileTree(entries)
}

// fakeThemeCatalog is a no-op ThemeCatalog for tests that don't exercise theme selection.
// moq can't be used here because ThemeEntry/ThemeSpec are defined in this package (import cycle).
type fakeThemeCatalog struct{}

func (fakeThemeCatalog) Entries() ([]ThemeEntry, error)   { return nil, nil }
func (fakeThemeCatalog) Resolve(string) (ThemeSpec, bool) { return ThemeSpec{}, false }
func (fakeThemeCatalog) Persist(string) error             { return nil }

// testNewModel is a test-only helper that preserves the old 4-arg NewModel
// shape while the production NewModel takes a single ModelConfig. It accepts
// the diff renderer, store, and highlighter as explicit args (so tests can
// pass mocks/fakes directly) and fills in default style dependencies
// (PlainResolver + its derived Renderer + a zero SGR) and sidepane factories
// when the cfg doesn't set them explicitly.
func testNewModel(t *testing.T, renderer Renderer, store *annotation.Store, highlighter SyntaxHighlighter, cfg ModelConfig) Model {
	t.Helper()
	cfg.Renderer = renderer
	cfg.Store = store
	cfg.Highlighter = highlighter
	if cfg.StyleResolver == nil {
		res := style.PlainResolver()
		cfg.StyleResolver = res
		cfg.StyleRenderer = style.NewRenderer(res)
		cfg.SGR = style.SGR{}
	}
	if cfg.WordDiffer == nil {
		cfg.WordDiffer = worddiff.New()
	}
	if cfg.NewFileTree == nil {
		cfg.NewFileTree = testFileTreeFactory()
	}
	if cfg.ParseTOC == nil {
		cfg.ParseTOC = testParseTOCFactory()
	}
	if cfg.Overlay == nil {
		cfg.Overlay = overlay.NewManager()
	}
	if cfg.Themes == nil {
		cfg.Themes = fakeThemeCatalog{}
	}
	m, err := NewModel(cfg)
	require.NoError(t, err)
	return m
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
	res := style.PlainResolver()
	m, err := NewModel(ModelConfig{
		Renderer:       renderer,
		Store:          annotation.NewStore(),
		Highlighter:    noopHighlighter(),
		StyleResolver:  res,
		StyleRenderer:  style.NewRenderer(res),
		SGR:            style.SGR{},
		WordDiffer:     worddiff.New(),
		Overlay:        overlay.NewManager(),
		Themes:         fakeThemeCatalog{},
		TreeWidthRatio: 3,
		NewFileTree:    testFileTreeFactory(),
		ParseTOC:       testParseTOCFactory(),
	})
	if err != nil {
		// testModel supplies all required deps — an error here is a bug in the helper, not the test.
		panic("testModel: " + err.Error())
	}
	// simulate window size
	m.layout.width = 120
	m.layout.height = 40
	m.layout.treeWidth = m.layout.width * m.cfg.treeWidthRatio / 10
	m.ready = true
	m.filesLoaded = true
	return m
}

func TestNewModel_RequiredDependencies(t *testing.T) {
	// build a fully valid config, then zero out one required dep at a time
	validCfg := func() ModelConfig {
		return ModelConfig{
			Renderer:      &mocks.RendererMock{},
			Store:         annotation.NewStore(),
			Highlighter:   noopHighlighter(),
			StyleResolver: style.PlainResolver(),
			StyleRenderer: style.NewRenderer(style.PlainResolver()),
			SGR:           style.SGR{},
			WordDiffer:    worddiff.New(),
			Overlay:       overlay.NewManager(),
			NewFileTree:   testFileTreeFactory(),
			ParseTOC:      testParseTOCFactory(),
			Themes:        fakeThemeCatalog{},
		}
	}

	tests := []struct {
		name  string
		patch func(c *ModelConfig)
	}{
		{name: "nil Renderer", patch: func(c *ModelConfig) { c.Renderer = nil }},
		{name: "nil Store", patch: func(c *ModelConfig) { c.Store = nil }},
		{name: "nil Highlighter", patch: func(c *ModelConfig) { c.Highlighter = nil }},
		{name: "nil StyleResolver", patch: func(c *ModelConfig) { c.StyleResolver = nil }},
		{name: "nil StyleRenderer", patch: func(c *ModelConfig) { c.StyleRenderer = nil }},
		{name: "nil SGR", patch: func(c *ModelConfig) { c.SGR = nil }},
		{name: "nil WordDiffer", patch: func(c *ModelConfig) { c.WordDiffer = nil }},
		{name: "nil Overlay", patch: func(c *ModelConfig) { c.Overlay = nil }},
		{name: "nil NewFileTree", patch: func(c *ModelConfig) { c.NewFileTree = nil }},
		{name: "nil ParseTOC", patch: func(c *ModelConfig) { c.ParseTOC = nil }},
		{name: "nil Themes", patch: func(c *ModelConfig) { c.Themes = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validCfg()
			tc.patch(&cfg)
			_, err := NewModel(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "is required")
		})
	}

	t.Run("all required deps present succeeds", func(t *testing.T) {
		cfg := validCfg()
		_, err := NewModel(cfg)
		require.NoError(t, err)
	})
}

func TestNewModel_OptionalDefaults(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}

	t.Run("nil keymap defaults to keymap.Default()", func(t *testing.T) {
		m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{})
		require.NotNil(t, m.keymap)
		// verify a known default binding works
		action := m.keymap.Resolve("q")
		assert.Equal(t, keymap.ActionQuit, action)
	})

	t.Run("custom keymap is used when provided", func(t *testing.T) {
		km := keymap.Default()
		km.Unbind("q")
		m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{Keymap: km})
		action := m.keymap.Resolve("q")
		assert.Equal(t, keymap.Action(""), action)
	})

	t.Run("TreeWidthRatio below range defaults to 2", func(t *testing.T) {
		m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TreeWidthRatio: 0})
		assert.Equal(t, 2, m.cfg.treeWidthRatio)
	})

	t.Run("TreeWidthRatio above range defaults to 2", func(t *testing.T) {
		m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TreeWidthRatio: 15})
		assert.Equal(t, 2, m.cfg.treeWidthRatio)
	})

	t.Run("TreeWidthRatio in range is kept", func(t *testing.T) {
		m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TreeWidthRatio: 5})
		assert.Equal(t, 5, m.cfg.treeWidthRatio)
	})
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

func TestModel_InitialLoadingState_NoEmptyFlash(t *testing.T) {
	// regression: before filesLoadedMsg is handled, View() must not paint the two-pane
	// layout with an empty tree and "no file selected". That made users see a 100-500 ms
	// flash of a "no changes" screen on launch. See app/ui/view.go and handleFilesLoaded.
	m := testModel([]string{"a.go", "b.go"}, nil)
	// undo the testModel shortcut to simulate a real program start
	m.ready = false
	m.filesLoaded = false

	// before WindowSizeMsg: generic loading string
	assert.Equal(t, "loading...", m.View())

	// WindowSizeMsg arrives; filesLoadedMsg has not
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)
	assert.Equal(t, "loading files...", m.View())

	// filesLoadedMsg arrives; loading state must end
	result, _ = m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	m = result.(Model)
	assert.True(t, m.filesLoaded)
	got := m.View()
	assert.NotEqual(t, "loading...", got)
	assert.NotEqual(t, "loading files...", got)
}

func TestModel_EnterSwitchesToDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.layout.focus = paneTree
	// simulate file already loaded (tree nav auto-loads)
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.layout.focus = paneTree // reset focus after file load

	// enter should switch to diff pane
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus)
}

func TestModel_TabPaneSwitching(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})

	t.Run("tree to diff when file loaded", func(t *testing.T) {
		m.layout.focus = paneTree
		m.file.name = "a.go"
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus)
	})

	t.Run("diff to tree", func(t *testing.T) {
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.layout.focus)
	})

	t.Run("stays on tree when no file loaded", func(t *testing.T) {
		m.layout.focus = paneTree
		m.file.name = ""
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.layout.focus)
	})
}

func TestModel_WrapModeFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("wrap enabled via config", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{Wrap: true, TreeWidthRatio: 2})
		assert.True(t, m.modes.wrap)
	})

	t.Run("wrap disabled by default", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.modes.wrap)
	})
}

func TestModel_CollapsedModeFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("collapsed enabled via config", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{Collapsed: true, TreeWidthRatio: 2})
		assert.True(t, m.modes.collapsed.enabled)
	})

	t.Run("collapsed disabled by default", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.modes.collapsed.enabled)
	})
}

func TestModel_LineNumbersFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("line numbers enabled via config", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{LineNumbers: true, TreeWidthRatio: 2})
		assert.True(t, m.modes.lineNumbers)
	})

	t.Run("line numbers disabled by default", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.modes.lineNumbers)
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
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{ShowBlame: true, Blamer: blamer, TreeWidthRatio: 2})
		assert.True(t, m.modes.showBlame)
	})

	t.Run("blame disabled without blamer even if requested", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{ShowBlame: true, TreeWidthRatio: 2})
		assert.False(t, m.modes.showBlame)
	})

	t.Run("blame disabled by default", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{Blamer: blamer, TreeWidthRatio: 2})
		assert.False(t, m.modes.showBlame)
	})
}

func TestModel_TreeNavigation(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree

	// cursor starts on first file (a.go)
	assert.Equal(t, "a.go", m.tree.SelectedFile())

	// j moves down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile())

	// k moves up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, "a.go", model.tree.SelectedFile())
}

func TestModel_FocusSwitching(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go" // pretend a file is loaded
	m.layout.focus = paneTree

	// l switches to diff pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus)

	// h switches back to tree
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = result.(Model)
	assert.Equal(t, paneTree, model.layout.focus)
}

func TestModel_WindowResize(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.ready = false

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	model := result.(Model)

	assert.True(t, model.ready)
	assert.Equal(t, 100, model.layout.width)
	assert.Equal(t, 50, model.layout.height)
	assert.Equal(t, 30, model.layout.treeWidth) // 100 * 3 / 10 = 30
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
			m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TreeWidthRatio: tc.ratio})
			result, _ := m.Update(tea.WindowSizeMsg{Width: tc.termWidth, Height: 40})
			model := result.(Model)
			assert.Equal(t, tc.wantTreeWidth, model.layout.treeWidth)
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
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines

	assert.False(t, m.modes.wrap)

	// "x" should toggle wrap
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)
	assert.True(t, model.modes.wrap, "x should toggle wrap mode on")

	// "w" should also toggle wrap (still bound by default)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.modes.wrap, "w should toggle wrap mode off")
}

func TestModel_CustomKeymapTreeNav(t *testing.T) {
	// map "x" to down, unbind "j" — verify "x" moves tree cursor and "j" does not
	km := keymap.Default()
	km.Bind("x", keymap.ActionDown)
	km.Unbind("j")

	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.keymap = km
	m.tree = testNewFileTree(files)
	m.layout.focus = paneTree

	assert.Equal(t, "a.go", m.tree.SelectedFile())

	// "x" should move down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile(), "x should move tree cursor down")

	// "j" should not move (unbound)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile(), "j should not move when unbound")
}

func TestModel_CustomKeymapTreeFocusDiff(t *testing.T) {
	// scroll_right in tree pane should focus diff (implicit fallback)
	files := []string{"a.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.file.name = "a.go"
	m.layout.focus = paneTree

	// right key maps to scroll_right by default, should focus diff in tree pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus, "right key (scroll_right) should focus diff in tree pane")
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
	assert.True(t, model.overlay.Active(), "? should open help with default keymap")
	assert.Equal(t, overlay.KindHelp, model.overlay.Kind())
}

// fakeCommitLog is a tiny test fake satisfying both diff.CommitLogger and the
// unexported commitLogSource. mirrors the fakeThemeCatalog pattern used for
// interfaces whose moq-generated mocks land in another package and would be
// unreachable from package-internal tests.
type fakeCommitLog struct {
	fn      func(ref string) ([]diff.CommitInfo, error)
	calls   int
	lastRef string
	allRefs []string
}

func (f *fakeCommitLog) CommitLog(ref string) ([]diff.CommitInfo, error) {
	f.calls++
	f.lastRef = ref
	f.allRefs = append(f.allRefs, ref)
	if f.fn == nil {
		return nil, nil
	}
	return f.fn(ref)
}

// rendererWithCommitLog wraps RendererMock and adds a CommitLog method so the
// renderer satisfies diff.CommitLogger. Used to verify the type-assertion
// fallback path in NewModel.
type rendererWithCommitLog struct {
	*mocks.RendererMock
	commit *fakeCommitLog
}

func (r rendererWithCommitLog) CommitLog(ref string) ([]diff.CommitInfo, error) {
	return r.commit.CommitLog(ref)
}

func TestNewModel_CommitLogResolution(t *testing.T) {
	plainRenderer := func() *mocks.RendererMock {
		return &mocks.RendererMock{
			ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
			FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
		}
	}

	t.Run("explicit CommitLog wins over renderer capability", func(t *testing.T) {
		explicit := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
			return []diff.CommitInfo{{Hash: "explicit"}}, nil
		}}
		viaRenderer := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
			return []diff.CommitInfo{{Hash: "renderer"}}, nil
		}}
		m := testNewModel(t, rendererWithCommitLog{RendererMock: plainRenderer(), commit: viaRenderer},
			annotation.NewStore(), noopHighlighter(), ModelConfig{
				CommitLog:         explicit,
				CommitsApplicable: true,
			})
		require.NotNil(t, m.commits.source)
		got, err := m.commits.source.CommitLog("ref")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "explicit", got[0].Hash, "explicit source must take precedence")
		assert.Equal(t, 0, viaRenderer.calls, "renderer source must be ignored when explicit is provided")
	})

	t.Run("renderer capability is used when explicit is nil", func(t *testing.T) {
		viaRenderer := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
			return []diff.CommitInfo{{Hash: "from-renderer"}}, nil
		}}
		m := testNewModel(t, rendererWithCommitLog{RendererMock: plainRenderer(), commit: viaRenderer},
			annotation.NewStore(), noopHighlighter(), ModelConfig{CommitsApplicable: true})
		require.NotNil(t, m.commits.source, "fallback to renderer capability must populate source")
		got, err := m.commits.source.CommitLog("HEAD~1")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "from-renderer", got[0].Hash)
		assert.Equal(t, 1, viaRenderer.calls)
	})

	t.Run("typed-nil explicit collapses to renderer fallback", func(t *testing.T) {
		var typedNil *fakeCommitLog
		viaRenderer := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
			return []diff.CommitInfo{{Hash: "fallback"}}, nil
		}}
		m := testNewModel(t, rendererWithCommitLog{RendererMock: plainRenderer(), commit: viaRenderer},
			annotation.NewStore(), noopHighlighter(), ModelConfig{
				CommitLog:         typedNil,
				CommitsApplicable: true,
			})
		require.NotNil(t, m.commits.source, "typed-nil must collapse to interface-nil and trigger fallback")
		_, err := m.commits.source.CommitLog("X..Y")
		require.NoError(t, err)
		assert.Equal(t, 1, viaRenderer.calls, "fallback source must be used after typed-nil collapses")
	})

	t.Run("nil source when renderer lacks capability", func(t *testing.T) {
		m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(),
			ModelConfig{CommitsApplicable: true})
		assert.Nil(t, m.commits.source, "no source must remain nil when renderer is not a CommitLogger")
		assert.False(t, m.commits.applicable, "applicable must collapse to false when source is nil")
	})

	t.Run("applicable mirrors ModelConfig and source presence", func(t *testing.T) {
		viaRenderer := &fakeCommitLog{}
		t.Run("applicable+source -> true", func(t *testing.T) {
			m := testNewModel(t, rendererWithCommitLog{RendererMock: plainRenderer(), commit: viaRenderer},
				annotation.NewStore(), noopHighlighter(), ModelConfig{CommitsApplicable: true})
			assert.True(t, m.commits.applicable)
		})
		t.Run("applicable but no source -> false", func(t *testing.T) {
			m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(),
				ModelConfig{CommitsApplicable: true})
			assert.False(t, m.commits.applicable)
		})
		t.Run("not applicable -> false", func(t *testing.T) {
			m := testNewModel(t, rendererWithCommitLog{RendererMock: plainRenderer(), commit: viaRenderer},
				annotation.NewStore(), noopHighlighter(), ModelConfig{CommitsApplicable: false})
			assert.False(t, m.commits.applicable)
		})
	})
}

func TestModel_ApplyReloadCleanup(t *testing.T) {
	t.Run("annotations cleared", func(t *testing.T) {
		m := testModel([]string{"a.go", "b.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go", "b.go"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
		require.Equal(t, 1, m.store.Count(), "precondition: store has annotation")

		m.applyReloadCleanup()

		assert.Equal(t, 0, m.store.Count(), "applyReloadCleanup must clear annotations")
	})

	t.Run("filter turned off when active", func(t *testing.T) {
		m := testModel([]string{"a.go", "b.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go", "b.go"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
		m.tree.ToggleFilter(m.annotatedFiles()) // activate filter
		require.True(t, m.tree.FilterActive(), "precondition: filter must be active")

		m.applyReloadCleanup()

		assert.False(t, m.tree.FilterActive(), "applyReloadCleanup must turn off active filter")
	})

	t.Run("filter left alone when not active", func(t *testing.T) {
		m := testModel([]string{"a.go", "b.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go", "b.go"})
		require.False(t, m.tree.FilterActive(), "precondition: filter must be inactive")

		m.applyReloadCleanup() // no-op on filter when not active

		assert.False(t, m.tree.FilterActive(), "applyReloadCleanup must not flip inactive filter")
	})
}

func TestModel_NewModel_ReloadApplicable(t *testing.T) {
	plainRend := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	t.Run("true when ReloadApplicable is true", func(t *testing.T) {
		m := testNewModel(t, plainRend, annotation.NewStore(), noopHighlighter(),
			ModelConfig{ReloadApplicable: true})
		assert.True(t, m.reload.applicable)
	})
	t.Run("false when ReloadApplicable is false", func(t *testing.T) {
		m := testNewModel(t, plainRend, annotation.NewStore(), noopHighlighter(),
			ModelConfig{ReloadApplicable: false})
		assert.False(t, m.reload.applicable)
	})
}
