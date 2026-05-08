package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
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
		FileDiffFunc:     func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) { return nil, nil },
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
		FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
			return fileDiffs[file], nil
		},
	}
	res := style.PlainResolver()
	m, err := NewModel(ModelConfig{
		Renderer:         renderer,
		Store:            annotation.NewStore(),
		Highlighter:      noopHighlighter(),
		StyleResolver:    res,
		StyleRenderer:    style.NewRenderer(res),
		SGR:              style.SGR{},
		WordDiffer:       worddiff.New(),
		Overlay:          overlay.NewManager(),
		Themes:           fakeThemeCatalog{},
		TreeWidthRatio:   3,
		AnnotationMarker: "\U0001f4ac",
		NewFileTree:      testFileTreeFactory(),
		ParseTOC:         testParseTOCFactory(),
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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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

func TestModel_CompactModeFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("compact enabled and applicable via config", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			Compact: true, CompactContext: 10, CompactApplicable: true, TreeWidthRatio: 2,
		})
		assert.True(t, m.modes.compact, "compact flag must be copied into modeState")
		assert.Equal(t, 10, m.modes.compactContext, "compact context must be copied from config")
		assert.True(t, m.compact.applicable, "applicable flag must be copied into Model")
		assert.Equal(t, 10, m.currentContextLines(), "helper must return configured context when compact+applicable")
	})

	t.Run("compact requested but not applicable drops to off at construction", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			Compact: true, CompactContext: 7, CompactApplicable: false, TreeWidthRatio: 2,
		})
		assert.False(t, m.modes.compact, "compact must be off when the feature is not applicable at startup")
		assert.Equal(t, 7, m.modes.compactContext, "compact context is still carried so toggle can use it if state changes")
		assert.False(t, m.compact.applicable)
		assert.Equal(t, 0, m.currentContextLines(), "non-applicable startup must resolve to full-file context")
	})

	t.Run("compact disabled by default", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.modes.compact)
		assert.False(t, m.compact.applicable)
		assert.Equal(t, 0, m.modes.compactContext)
	})
}

func TestModel_VimMotionFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()

	t.Run("vim motion enabled via config", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{VimMotion: true, TreeWidthRatio: 2})
		assert.True(t, m.modes.vimMotion, "vim motion flag must be copied into modeState")
	})

	t.Run("vim motion disabled by default", func(t *testing.T) {
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 2})
		assert.False(t, m.modes.vimMotion)
	})
}

func TestModel_LineNumbersFromConfig(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
				FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
			FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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

func TestDispatchAction_OverlayOpen(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	require.False(t, m.overlay.Active(), "precondition: no overlay active")

	result, cmd := m.dispatchAction(keymap.ActionHelp)
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "help action should open help overlay")
	assert.Nil(t, cmd)
}

func TestDispatchAction_Resolves(t *testing.T) {
	tests := []struct {
		name   string
		action keymap.Action
		verify func(t *testing.T, m Model, cmd tea.Cmd)
	}{
		{
			name:   "ActionQuit returns tea.Quit",
			action: keymap.ActionQuit,
			verify: func(t *testing.T, _ Model, cmd tea.Cmd) {
				require.NotNil(t, cmd)
				msg := cmd()
				_, ok := msg.(tea.QuitMsg)
				assert.True(t, ok, "ActionQuit must produce tea.QuitMsg")
			},
		},
		{
			name:   "ActionTogglePane switches focus",
			action: keymap.ActionTogglePane,
			verify: func(t *testing.T, m Model, _ tea.Cmd) {
				assert.Equal(t, paneDiff, m.layout.focus, "toggle_pane with file loaded should go to diff")
			},
		},
		{
			name:   "ActionHelp opens help overlay",
			action: keymap.ActionHelp,
			verify: func(t *testing.T, m Model, _ tea.Cmd) {
				assert.True(t, m.overlay.Active(), "help must open overlay")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.tree = testNewFileTree([]string{"a.go"})
			m.file.name = "a.go" // enables togglePane to switch to diff
			result, cmd := m.dispatchAction(tc.action)
			tc.verify(t, result.(Model), cmd)
		})
	}
}

func TestDispatchAction_PaneNavFallback_Diff(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	result, _ := m.dispatchAction(keymap.ActionDown)
	model := result.(Model)
	assert.Equal(t, 1, model.nav.diffCursor, "ActionDown should route to handleDiffAction and move cursor")
}

func TestDispatchAction_PaneNavFallback_Tree(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.layout.focus = paneTree
	require.Equal(t, "a.go", m.tree.SelectedFile(), "precondition: first file selected")

	result, _ := m.dispatchAction(keymap.ActionDown)
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.SelectedFile(), "ActionDown should route to handleTreeAction and move selection")
}

func TestHandleChordSecond_ResolvedDispatches(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, cmd := m.handleChordSecond("x")
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "chordPending must clear after dispatch")
	assert.Empty(t, model.keys.hint, "hint must clear after successful dispatch")
	require.NotNil(t, cmd, "resolved ActionQuit must produce a tea.Cmd")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "resolved chord must dispatch ActionQuit through dispatchAction")
}

func TestHandleChordSecond_UnboundShowsHint(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, cmd := m.handleChordSecond("q")
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "chordPending must clear after unbound second")
	assert.Equal(t, "Unknown chord: ctrl+w>q", model.keys.hint, "unbound second key must set Unknown chord hint")
	assert.Nil(t, cmd, "unbound chord must not produce a tea.Cmd")
}

func TestHandleChordSecond_EscCancels(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, cmd := m.handleChordSecond("esc")
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "esc must clear chordPending")
	assert.Empty(t, model.keys.hint, "esc must clear hint silently (no Unknown chord message)")
	assert.Nil(t, cmd, "esc must not dispatch any action")
}

func TestHandleChordSecond_LayoutFallback(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km
	m.keys.chordPending = "ctrl+w"

	// Cyrillic 'ч' sits on the same physical key as Latin 'x' on a QWERTY layout;
	// ResolveChord's layout-resolve fallback must translate it and find the binding.
	result, cmd := m.handleChordSecond("ч")
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "chordPending must clear even when resolved via layout fallback")
	assert.Empty(t, model.keys.hint, "hint must clear on successful layout-fallback dispatch")
	require.NotNil(t, cmd, "layout-fallback match must dispatch the bound action")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "Cyrillic ч on ctrl+w pending must resolve ctrl+w>x chord")
}

func TestHandleChordSecond_DispatchesToTOCWhenFocused(t *testing.T) {
	// regression: when TOC is focused, chord-resolved actions must flow through
	// the pre-resolved action path and NOT be re-resolved from the synthesized
	// second-stage key (which would turn `x` into whatever `x` is bound to
	// standalone, losing the chord action). `x` is unbound by default, so the
	// buggy path would be a no-op; the correct path honors the pre-resolved
	// ActionDown and advances the TOC cursor.
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# First", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Second", ChangeType: diff.ChangeContext},
	}
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionDown)

	m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
	m.keymap = km
	m.file.singleFile = true
	m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
	require.NotNil(t, m.file.mdTOC)
	m.file.name = "README.md"
	m.file.lines = mdLines
	m.layout.focus = paneTree
	// entries: [0]=README.md(lineIdx=0), [1]=First(lineIdx=0), [2]=Second(lineIdx=2).
	// seed cursor at [1] so ActionDown advances to [2] with an observably different lineIdx.
	m.file.mdTOC.Move(sidepane.MotionDown)
	m.keys.chordPending = "ctrl+w"

	before, _ := m.file.mdTOC.CurrentLineIdx()
	require.Equal(t, 0, before, "precondition: cursor at First entry (lineIdx=0)")

	result, _ := m.handleChordSecond("x")
	model := result.(Model)

	after, _ := model.file.mdTOC.CurrentLineIdx()
	assert.Equal(t, 2, after, "chord-resolved ActionDown must advance TOC cursor to Second entry (lineIdx=2)")
	assert.Empty(t, model.keys.chordPending, "chordPending must clear after TOC dispatch")
}

func TestTransientHint_ChordHintLowestPriority(t *testing.T) {
	tests := []struct {
		name    string
		setHint func(m *Model)
		want    string
	}{
		{name: "keys hint alone", setHint: func(m *Model) { m.keys.hint = "Unknown chord: ctrl+w>q" }, want: "Unknown chord: ctrl+w>q"},
		{name: "reload beats keys", setHint: func(m *Model) { m.reload.hint = "Reloaded"; m.keys.hint = "chord hint" }, want: "Reloaded"},
		{name: "compact beats keys", setHint: func(m *Model) { m.compact.hint = "compact off"; m.keys.hint = "chord hint" }, want: "compact off"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			tc.setHint(&m)
			assert.Equal(t, tc.want, m.transientHint())
		})
	}
}

func TestTransientHint_VimLowestPriority(t *testing.T) {
	tests := []struct {
		name    string
		setHint func(m *Model)
		want    string
	}{
		{name: "vim hint alone", setHint: func(m *Model) { m.vim.hint = "5" }, want: "5"},
		{name: "reload beats vim", setHint: func(m *Model) { m.reload.hint = "Reloaded"; m.vim.hint = "5" }, want: "Reloaded"},
		{name: "compact beats vim", setHint: func(m *Model) { m.compact.hint = "compact off"; m.vim.hint = "5" }, want: "compact off"},
		{name: "keys beats vim", setHint: func(m *Model) { m.keys.hint = "Pending: ctrl+w"; m.vim.hint = "5" }, want: "Pending: ctrl+w"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			tc.setHint(&m)
			assert.Equal(t, tc.want, m.transientHint())
		})
	}
}

func TestHandleKey_EntersChordPending(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	model := result.(Model)

	assert.Equal(t, "ctrl+w", model.keys.chordPending, "leader key must set chordPending")
	assert.Equal(t, "Pending: ctrl+w, esc to cancel", model.keys.hint, "status hint must announce pending chord")
	assert.Nil(t, cmd, "entering pending state must not produce a tea.Cmd")
}

func TestHandleKey_ChordSecondCoexistenceGuard(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km

	// simulate buggy coexistence: chord pending AND search active at the same time.
	// the chord-second guard runs BEFORE handleModalKey, so the second key must
	// resolve as chord-second and must NOT leak into the search textinput.
	m.keys.chordPending = "ctrl+w"
	m.search.active = true
	m.search.input = textinput.New()

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "chord-second guard must clear chordPending")
	assert.Empty(t, model.search.input.Value(), "search textinput must NOT receive the chord-second key")
	require.NotNil(t, cmd, "resolved chord must dispatch ActionQuit")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "chord-second must dispatch the bound ActionQuit even with search active")
}

func TestHandleKey_ChordIgnoredWhenPendingReload(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km
	// simulate the state produced by pressing R with annotations present
	m.reload.applicable = true
	m.reload.pending = true
	m.reload.hint = "Annotations will be dropped — press y to confirm, any other key to cancel"

	// send the chord leader; handlePendingReload must intercept first, so the
	// chord-first guard never runs and chordPending stays empty.
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "chord-first guard must not fire while reload is pending")
	assert.False(t, model.reload.pending, "non-y key cancels pending reload")
	assert.Equal(t, "Reload canceled", model.reload.hint, "reload-cancel hint must be set")
}

func TestHandleKey_LeaderWithStandaloneActionDoesNotEnterChord(t *testing.T) {
	km := keymap.Default()
	// bind ctrl+w as a standalone action (no chord binding for ctrl+w>*)
	km.Bind("ctrl+w", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "standalone-bound leader must not enter chord-pending state")
	assert.Empty(t, model.keys.hint, "no chord hint when the key resolves to a standalone action")
	require.NotNil(t, cmd, "standalone ActionQuit must dispatch")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "ctrl+w must fire the standalone ActionQuit")
}

func TestClearPendingInputState_ClearsAllFields(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"
	m.vim.count = 42
	m.vim.leader = "g"
	m.vim.hint = "g…"

	m.clearPendingInputState()

	assert.Empty(t, m.keys.chordPending, "chordPending must be cleared")
	assert.Empty(t, m.keys.hint, "keys.hint must be cleared alongside chordPending")
	assert.Zero(t, m.vim.count, "vim.count must be cleared")
	assert.Empty(t, m.vim.leader, "vim.leader must be cleared")
	assert.Empty(t, m.vim.hint, "vim.hint must be cleared")
}

func TestVimState_ZeroValue(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	assert.Equal(t, 0, m.vim.count, "vim.count must default to 0")
	assert.Empty(t, m.vim.leader, "vim.leader must default to empty")
	assert.Empty(t, m.vim.hint, "vim.hint must default to empty")
}

func TestHandleKey_ClearsVimHint(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.vim.hint = "5"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model := result.(Model)

	assert.Empty(t, model.vim.hint, "any key press must clear vim.hint alongside other transient hints")
}

func TestHandleOverlayOpen_HelpClearsChord(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, _, handled := m.handleOverlayOpen(keymap.ActionHelp)
	model := result.(Model)

	assert.True(t, handled, "ActionHelp must be handled by handleOverlayOpen")
	assert.Empty(t, model.keys.chordPending, "help overlay entry must clear chordPending")
	assert.Empty(t, model.keys.hint, "help overlay entry must clear chord hint")
	assert.True(t, model.overlay.Active(), "help overlay must be open")
	assert.Equal(t, overlay.KindHelp, model.overlay.Kind())
}

func TestHandleOverlayOpen_AnnotListClearsChord(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, _, handled := m.handleOverlayOpen(keymap.ActionAnnotList)
	model := result.(Model)

	assert.True(t, handled, "ActionAnnotList must be handled by handleOverlayOpen")
	assert.Empty(t, model.keys.chordPending, "annot-list overlay entry must clear chordPending")
	assert.Empty(t, model.keys.hint, "annot-list overlay entry must clear chord hint")
	assert.True(t, model.overlay.Active(), "annot-list overlay must be open")
	assert.Equal(t, overlay.KindAnnotList, model.overlay.Kind())
}

func TestHandleOverlayOpen_ThemeSelectClearsChord(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.themes = newTestThemeCatalog()
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, _, handled := m.handleOverlayOpen(keymap.ActionThemeSelect)
	model := result.(Model)

	assert.True(t, handled, "ActionThemeSelect must be handled by handleOverlayOpen")
	assert.Empty(t, model.keys.chordPending, "theme-select overlay entry must clear chordPending")
	assert.Empty(t, model.keys.hint, "theme-select overlay entry must clear chord hint")
	assert.True(t, model.overlay.Active(), "theme-select overlay must be open")
	assert.Equal(t, overlay.KindThemeSelect, model.overlay.Kind())
}

func TestHandleOverlayOpen_InfoClearsChord(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.list = []diff.CommitInfo{{Hash: "abc"}}
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	result, _, handled := m.handleOverlayOpen(keymap.ActionInfo)
	model := result.(Model)

	assert.True(t, handled, "ActionInfo must be handled by handleOverlayOpen")
	assert.Empty(t, model.keys.chordPending, "info overlay entry must clear chordPending")
	assert.Empty(t, model.keys.hint, "info overlay entry must clear chord hint")
	assert.True(t, model.overlay.Active(), "info overlay must be open")
	assert.Equal(t, overlay.KindInfo, model.overlay.Kind())
}

func TestStartSearch_ClearsChord(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	m.startSearch()

	assert.Empty(t, m.keys.chordPending, "startSearch must clear chordPending")
	assert.Empty(t, m.keys.hint, "startSearch must clear chord hint")
	assert.True(t, m.search.active, "startSearch must enter searching mode")
}

func TestStartAnnotation_ClearsChord(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.keys.chordPending = "ctrl+w"
	m.keys.hint = "Pending: ctrl+w, esc to cancel"

	m.startAnnotation()

	assert.Empty(t, m.keys.chordPending, "startAnnotation must clear chordPending")
	assert.Empty(t, m.keys.hint, "startAnnotation must clear chord hint")
	assert.True(t, m.annot.annotating, "startAnnotation must enter annotating mode")
}

func TestStartSearch_ClearsVimState(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.vim.count = 5
	m.vim.leader = "g"
	m.vim.hint = "5"

	m.startSearch()

	assert.Zero(t, m.vim.count, "startSearch must clear vim.count")
	assert.Empty(t, m.vim.leader, "startSearch must clear vim.leader")
	assert.Empty(t, m.vim.hint, "startSearch must clear vim.hint")
	assert.True(t, m.search.active, "startSearch must enter searching mode")
}

func TestStartAnnotation_ClearsVimState(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.vim.count = 5
	m.vim.leader = "z"
	m.vim.hint = "z…"

	m.startAnnotation()

	assert.Zero(t, m.vim.count, "startAnnotation must clear vim.count")
	assert.Empty(t, m.vim.leader, "startAnnotation must clear vim.leader")
	assert.Empty(t, m.vim.hint, "startAnnotation must clear vim.hint")
	assert.True(t, m.annot.annotating, "startAnnotation must enter annotating mode")
}

func TestStartFileAnnotation_ClearsVimState(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.vim.count = 5
	m.vim.leader = "Z"
	m.vim.hint = "Z…"

	m.startFileAnnotation()

	assert.Zero(t, m.vim.count, "startFileAnnotation must clear vim.count")
	assert.Empty(t, m.vim.leader, "startFileAnnotation must clear vim.leader")
	assert.Empty(t, m.vim.hint, "startFileAnnotation must clear vim.hint")
	assert.True(t, m.annot.fileAnnotating, "startFileAnnotation must enter file-level annotating mode")
}

func TestHandleOverlayOpen_ClearsVimState_Help(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.vim.count = 7
	m.vim.leader = "g"
	m.vim.hint = "g…"

	result, _, handled := m.handleOverlayOpen(keymap.ActionHelp)
	model := result.(Model)

	assert.True(t, handled, "ActionHelp must be handled by handleOverlayOpen")
	assert.Zero(t, model.vim.count, "help overlay entry must clear vim.count")
	assert.Empty(t, model.vim.leader, "help overlay entry must clear vim.leader")
	assert.Empty(t, model.vim.hint, "help overlay entry must clear vim.hint")
	assert.True(t, model.overlay.Active(), "help overlay must be open")
}

func TestHandleOverlayOpen_ClearsVimState_ThemeSelect(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.themes = newTestThemeCatalog()
	m.vim.count = 9
	m.vim.leader = "Z"
	m.vim.hint = "Z…"

	result, _, handled := m.handleOverlayOpen(keymap.ActionThemeSelect)
	model := result.(Model)

	assert.True(t, handled, "ActionThemeSelect must be handled by handleOverlayOpen")
	assert.Zero(t, model.vim.count, "theme-select overlay entry must clear vim.count")
	assert.Empty(t, model.vim.leader, "theme-select overlay entry must clear vim.leader")
	assert.Empty(t, model.vim.hint, "theme-select overlay entry must clear vim.hint")
	assert.True(t, model.overlay.Active(), "theme-select overlay must be open")
}

func TestHandleKey_ChordPrecedence(t *testing.T) {
	leader := tea.KeyMsg{Type: tea.KeyCtrlW}
	boundSecond := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	unboundSecond := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	escKey := tea.KeyMsg{Type: tea.KeyEsc}

	tests := []struct {
		name  string
		setup func(t *testing.T, m *Model)
		send  tea.KeyMsg
		check func(t *testing.T, after Model, cmd tea.Cmd)
	}{
		{
			name:  "clean state, leader sets pending",
			setup: func(t *testing.T, m *Model) {},
			send:  leader,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Equal(t, "ctrl+w", after.keys.chordPending, "leader must set chordPending")
				assert.Equal(t, "Pending: ctrl+w, esc to cancel", after.keys.hint, "pending hint must be set")
				assert.Nil(t, cmd, "entering pending state must not produce a tea.Cmd")
			},
		},
		{
			name: "chord pending, bound second dispatches",
			setup: func(t *testing.T, m *Model) {
				m.keys.chordPending = "ctrl+w"
				m.keys.hint = "Pending: ctrl+w, esc to cancel"
			},
			send: boundSecond,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chordPending must clear after dispatch")
				assert.Empty(t, after.keys.hint, "hint must clear after successful dispatch")
				require.NotNil(t, cmd, "resolved chord must dispatch ActionQuit")
				_, ok := cmd().(tea.QuitMsg)
				assert.True(t, ok, "second-stage 'x' must dispatch ctrl+w>x → ActionQuit")
			},
		},
		{
			name: "chord pending, unbound second sets Unknown chord hint",
			setup: func(t *testing.T, m *Model) {
				m.keys.chordPending = "ctrl+w"
				m.keys.hint = "Pending: ctrl+w, esc to cancel"
			},
			send: unboundSecond,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chordPending must clear after unbound second")
				assert.Equal(t, "Unknown chord: ctrl+w>q", after.keys.hint, "unbound second must set Unknown chord hint")
				assert.Nil(t, cmd, "unbound chord must not dispatch any action")
			},
		},
		{
			name: "chord pending, esc cancels silently",
			setup: func(t *testing.T, m *Model) {
				m.keys.chordPending = "ctrl+w"
				m.keys.hint = "Pending: ctrl+w, esc to cancel"
			},
			send: escKey,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "esc must clear chordPending")
				assert.Empty(t, after.keys.hint, "esc must clear hint silently (no Unknown chord message)")
				assert.Nil(t, cmd, "esc must not dispatch any action")
			},
		},
		{
			name: "chord pending, leader again consumed as second-stage (Unknown chord)",
			setup: func(t *testing.T, m *Model) {
				m.keys.chordPending = "ctrl+w"
				m.keys.hint = "Pending: ctrl+w, esc to cancel"
			},
			send: leader,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chord-second must consume the leader and clear pending")
				assert.Equal(t, "Unknown chord: ctrl+w>ctrl+w", after.keys.hint, "leader>leader is unbound, must surface Unknown chord")
				assert.Nil(t, cmd, "unbound chord must not dispatch any action")
			},
		},
		{
			name: "annotate active, leader does not enter chord",
			setup: func(t *testing.T, m *Model) {
				m.annot.annotating = true
				m.annot.input = textinput.New()
			},
			send: leader,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chord-first guard must not fire while annotating (modal eats the key)")
				assert.Empty(t, after.keys.hint, "no chord hint when modal owns the key")
				assert.True(t, after.annot.annotating, "annotation mode stays active")
			},
		},
		{
			name: "search active, leader does not enter chord",
			setup: func(t *testing.T, m *Model) {
				m.search.active = true
				m.search.input = textinput.New()
			},
			send: leader,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chord-first guard must not fire while searching (modal eats the key)")
				assert.Empty(t, after.keys.hint, "no chord hint when modal owns the key")
				assert.True(t, after.search.active, "search mode stays active")
			},
		},
		{
			name: "overlay active, leader does not enter chord",
			setup: func(t *testing.T, m *Model) {
				m.overlay.OpenHelp(m.buildHelpSpec())
			},
			send: leader,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chord-first guard must not fire while overlay is active (handleModalKey routes to overlay)")
				assert.Empty(t, after.keys.hint, "no chord hint when overlay owns the key")
				assert.True(t, after.overlay.Active(), "overlay stays active")
			},
		},
		{
			name: "pending reload, leader cancels reload (chord not entered)",
			setup: func(t *testing.T, m *Model) {
				m.reload.applicable = true
				m.reload.pending = true
				m.reload.hint = "Annotations will be dropped — press y to confirm, any other key to cancel"
			},
			send: leader,
			check: func(t *testing.T, after Model, cmd tea.Cmd) {
				assert.Empty(t, after.keys.chordPending, "chord-first guard must not fire while reload is pending")
				assert.False(t, after.reload.pending, "non-y key cancels pending reload")
				assert.Equal(t, "Reload canceled", after.reload.hint, "reload-cancel hint must replace pending prompt")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			km := keymap.Default()
			km.Bind("ctrl+w>x", keymap.ActionQuit)
			m := testModel([]string{"a.go"}, nil)
			m.keymap = km
			tc.setup(t, &m)

			result, cmd := m.Update(tc.send)
			after := result.(Model)
			tc.check(t, after, cmd)
		})
	}
}

func TestHandleKey_VimMotionOff_InterceptorSkipped(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = false

	// digit key with vim-motion off must not touch vim state
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	model := result.(Model)

	assert.Equal(t, 0, model.vim.count, "vim.count must stay 0 when vim-motion is off")
	assert.Empty(t, model.vim.hint, "vim.hint must stay empty when vim-motion is off")
	assert.Empty(t, model.vim.leader, "vim.leader must stay empty when vim-motion is off")
}

func TestHandleKey_VimMotionOn_DigitAccumulates(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	model := result.(Model)

	assert.Equal(t, 5, model.vim.count, "digit must accumulate into vim.count")
	assert.Equal(t, "5", model.vim.hint, "vim.hint must reflect accumulated count")
	assert.Nil(t, cmd, "digit accumulation must not produce a tea.Cmd")
}

func TestHandleKey_VimMotionOn_ChordSecondWins(t *testing.T) {
	km := keymap.Default()
	km.Bind("ctrl+w>x", keymap.ActionQuit)
	m := testModel([]string{"a.go"}, nil)
	m.keymap = km
	m.modes.vimMotion = true
	// coexistence: chord pending + vim-motion on. chord-second guard must
	// preempt the vim-motion interceptor (runs earlier in handleKey).
	m.keys.chordPending = "ctrl+w"

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	model := result.(Model)

	assert.Empty(t, model.keys.chordPending, "chord-second guard must clear chordPending")
	assert.Equal(t, 0, model.vim.count, "vim interceptor must NOT see the key when chord-second consumed it")
	assert.Empty(t, model.vim.hint, "vim.hint must stay empty when chord-second wins")
	assert.Nil(t, cmd, "ctrl+w>5 is unbound — chord-second surfaces an Unknown hint without dispatch")
	assert.Equal(t, "Unknown chord: ctrl+w>5", model.keys.hint, "chord-second sets Unknown chord hint")
}

func TestHandleKey_VimMotionOn_PendingReloadWins(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true
	m.reload.applicable = true
	m.reload.pending = true
	m.reload.hint = "Annotations will be dropped — press y to confirm, any other key to cancel"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model := result.(Model)

	assert.False(t, model.reload.pending, "pending-reload guard must consume y before vim interceptor")
	assert.Equal(t, 0, model.vim.count, "vim state must be untouched when reload preempts")
	assert.Empty(t, model.vim.hint, "vim.hint must stay empty when reload preempts")
}

func TestHandleKey_VimMotionOn_SearchActiveModalWins(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true
	m.search.active = true
	m.search.input = textinput.New()
	m.search.input.Focus()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	model := result.(Model)

	assert.True(t, model.search.active, "search mode stays active")
	assert.Equal(t, "5", model.search.input.Value(), "search textinput must receive the digit key")
	assert.Equal(t, 0, model.vim.count, "vim interceptor must not run while search modal is active")
	assert.Empty(t, model.vim.hint, "vim.hint must stay empty when modal consumes the key")
}

func TestHandleKey_VimMotionOn_AnnotateActiveModalWins(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true
	m.annot.annotating = true
	m.annot.input = textinput.New()
	m.annot.input.Focus()

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	model := result.(Model)

	assert.True(t, model.annot.annotating, "annotation mode stays active")
	assert.Equal(t, "5", model.annot.input.Value(), "annotation textinput must receive the digit key")
	assert.Equal(t, 0, model.vim.count, "vim interceptor must not run while annotate modal is active")
	assert.Empty(t, model.vim.hint, "vim.hint must stay empty when modal consumes the key")
}

func TestHandleKey_VimMotionOn_OverlayActiveModalWins(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true
	m.overlay.OpenHelp(m.buildHelpSpec())
	require.True(t, m.overlay.Active(), "help overlay must be open for this test")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	model := result.(Model)

	assert.Equal(t, 0, model.vim.count, "vim interceptor must not run while overlay is active")
	assert.Empty(t, model.vim.hint, "vim.hint must stay empty when overlay consumes the key")
}

func TestHandleKey_VimMotionOn_NonVimKeyFallsThrough(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true

	// 'q' is not a vim key and has no pending vim state — interceptor returns
	// handled=false, keymap.Resolve routes it to ActionQuit.
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := result.(Model)

	assert.Equal(t, 0, model.vim.count, "non-vim key must not set vim.count")
	assert.Empty(t, model.vim.leader, "non-vim key must not set vim.leader")
	assert.Empty(t, model.vim.hint, "non-vim key must not set vim.hint")
	require.NotNil(t, cmd, "q must dispatch ActionQuit through normal keymap resolution")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "q must fire ActionQuit even when vim-motion is on")
}

func TestHandleKey_NonKeyMessagesPreserveChordState(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "WindowSizeMsg", msg: tea.WindowSizeMsg{Width: 100, Height: 40}},
		// stale seq makes the handlers short-circuit without touching unrelated state
		{name: "filesLoadedMsg", msg: filesLoadedMsg{seq: 99999}},
		{name: "blameLoadedMsg", msg: blameLoadedMsg{file: "a.go", seq: 99999}},
		{name: "commitsLoadedMsg", msg: commitsLoadedMsg{seq: 99999}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			km := keymap.Default()
			km.Bind("ctrl+w>x", keymap.ActionQuit)
			m := testModel([]string{"a.go"}, nil)
			m.keymap = km
			m.keys.chordPending = "ctrl+w"
			m.keys.hint = "Pending: ctrl+w, esc to cancel"

			result, _ := m.Update(tc.msg)
			after := result.(Model)

			assert.Equal(t, "ctrl+w", after.keys.chordPending, "chordPending must survive non-key messages (route through Update, not handleKey)")
			assert.Equal(t, "Pending: ctrl+w, esc to cancel", after.keys.hint, "chord hint must survive non-key messages")
		})
	}
}
