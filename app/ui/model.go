package ui

//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer
//go:generate moq -out mocks/syntax_highlighter.go -pkg mocks -skip-ensure -fmt goimports . SyntaxHighlighter
//go:generate moq -out mocks/blamer.go -pkg mocks -skip-ensure -fmt goimports . Blamer

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
)

// Renderer provides methods to extract changed files and build full-file diff views.
type Renderer interface {
	ChangedFiles(ref string, staged bool) ([]diff.FileEntry, error)
	FileDiff(ref, file string, staged bool) ([]diff.DiffLine, error)
}

// SyntaxHighlighter provides syntax highlighting for diff lines.
type SyntaxHighlighter interface {
	HighlightLines(filename string, lines []diff.DiffLine) []string
	SetStyle(styleName string) bool
	StyleName() string
}

// Blamer provides git blame information for files.
type Blamer interface {
	FileBlame(ref, file string, staged bool) (map[int]diff.BlameLine, error)
}

// pane identifies which pane has focus.
type pane int

const (
	paneTree pane = iota
	paneDiff

	minTreeWidth = 20
)

// Model is the top-level bubbletea model for revdiff.
type Model struct {
	styles   styles
	tree     fileTree
	viewport viewport.Model
	store    *annotation.Store
	renderer Renderer
	keymap   *keymap.Keymap

	ref            string
	staged         bool
	only           []string // filter to show only matching files
	workDir        string   // working directory for resolving absolute --only paths
	noColors       bool     // keep monochrome output when previewing or applying themes
	noStatusBar    bool
	focus          pane
	treeHidden     bool // user toggled tree/TOC pane off
	width          int
	height         int
	treeWidth      int
	treeWidthRatio int    // 1-10 units for file tree panel
	tabSpaces      string // spaces to replace tabs with
	diffCursor     int    // index into diffLines for current cursor line
	scrollX        int    // horizontal scroll offset for diff pane

	highlighter        SyntaxHighlighter // syntax highlighter
	highlightedLines   []string          // pre-computed highlighted content, parallel to diffLines
	diffLines          []diff.DiffLine   // current file's parsed diff lines
	currFile           string            // currently displayed file
	loadSeq            uint64            // monotonic counter to identify the latest load request
	ready              bool              // true after first WindowSizeMsg
	annotating         bool              // true when annotation text input is active
	fileAnnotating     bool              // true when annotating at file level (Line=0)
	cursorOnAnnotation bool              // true when cursor is on the annotation sub-line (not the diff line)
	annotateInput      textinput.Model   // text input for annotations

	collapsed collapsedState // collapsed diff view state

	fileAdds    int // cached count of added lines in current file
	fileRemoves int // cached count of removed lines in current file

	showHelp       bool // true when help overlay is visible
	wrapMode       bool // true when line wrapping is enabled
	crossFileHunks bool // allow [ and ] to jump across file boundaries
	lineNumbers    bool // true when line numbers are shown in gutter
	lineNumWidth   int  // digit width for line number columns (max digits across old/new nums)

	blamer         Blamer                   // optional blame provider (nil when git unavailable)
	showBlame      bool                     // true when blame gutter is shown
	blameData      map[int]diff.BlameLine   // blame info keyed by 1-based new line number
	showUntracked  bool                     // true when untracked files are shown in tree
	loadUntracked  func() ([]string, error) // fetches untracked files; nil when unavailable
	blameAuthorLen int                      // max author display width for blame gutter
	blameNow       time.Time                // snapshot of time.Now() set once per render pass for blame age

	searching      bool            // true when search textinput is active (typing)
	searchTerm     string          // last submitted search query
	searchMatches  []int           // indices into diffLines that match
	searchCursor   int             // current position in searchMatches (0-based)
	searchInput    textinput.Model // dedicated textinput for search
	searchMatchSet map[int]bool    // set of diffLines indices that match search, computed per render

	discarded        bool // true when user chose to discard annotations and quit
	inConfirmDiscard bool // true when showing discard confirmation prompt
	noConfirmDiscard bool // skip confirmation prompt on discard quit
	singleFile       bool // true when diff contains exactly one file, hides tree pane

	showAnnotList    bool                    // true when annotation list popup is visible
	annotListCursor  int                     // selected item in the flat list
	annotListOffset  int                     // scroll offset for the annotation list
	annotListItems   []annotation.Annotation // flat sorted list of all annotations
	pendingAnnotJump *annotation.Annotation  // pending jump target after cross-file annotation list jump
	pendingHunkJump  *bool                   // pending hunk jump after cross-file hunk navigation (true=first, false=last)

	mdTOC *mdTOC // markdown table-of-contents for single-file full-context markdown mode (nil when not applicable)

	themesDir  string // path to themes directory for theme selector
	configPath string // path to config file for persisting theme choice

	themeSel        themeSelectState // theme selector overlay state
	activeThemeName string           // name of currently applied theme (for cursor positioning)
}

// fileLoadedMsg is sent when a file's diff has been loaded.
type fileLoadedMsg struct {
	file  string
	seq   uint64
	lines []diff.DiffLine
	err   error
}

// blameLoadedMsg is sent when blame data for a file has been loaded.
type blameLoadedMsg struct {
	file string
	seq  uint64
	data map[int]diff.BlameLine
	err  error
}

// filesLoadedMsg is sent when the changed file list is loaded.
type filesLoadedMsg struct {
	entries  []diff.FileEntry
	err      error
	warnings []string // non-fatal issues (staged/untracked fetch failures)
}

// ModelConfig holds configuration options for NewModel.
type ModelConfig struct {
	Ref              string
	Staged           bool
	TreeWidthRatio   int
	TabWidth         int                      // number of spaces per tab character
	NoColors         bool                     // disable all colors including syntax highlighting
	NoStatusBar      bool                     // hide the status bar
	NoConfirmDiscard bool                     // skip confirmation prompt when discarding annotations
	Wrap             bool                     // enable line wrapping
	Collapsed        bool                     // start in collapsed diff mode
	CrossFileHunks   bool                     // allow [ and ] to jump across file boundaries
	LineNumbers      bool                     // show line numbers in diff gutter
	ShowBlame        bool                     // show blame gutter on startup when available
	Only             []string                 // show only these files (match by exact path or path suffix)
	WorkDir          string                   // working directory for resolving absolute --only paths
	Keymap           *keymap.Keymap           // custom key bindings (nil uses defaults)
	LoadUntracked    func() ([]string, error) // fetches untracked files; nil when unavailable
	Blamer           Blamer                   // optional blame provider (nil when git unavailable)
	Colors           Colors
	ThemesDir        string // path to themes directory for theme selector
	ConfigPath       string // path to config file for persisting theme choice
	ActiveThemeName  string // name of theme currently applied (for theme selector cursor positioning)
}

// NewModel creates a new Model with the given renderer, store, highlighter and configuration.
func NewModel(renderer Renderer, store *annotation.Store, highlighter SyntaxHighlighter, cfg ModelConfig) Model {
	if cfg.TreeWidthRatio < 1 || cfg.TreeWidthRatio > 10 {
		cfg.TreeWidthRatio = 2
	}
	if cfg.TabWidth < 1 {
		cfg.TabWidth = 4
	}
	km := cfg.Keymap
	if km == nil {
		km = keymap.Default()
	}
	s := newStyles(cfg.Colors)
	if cfg.NoColors {
		s = plainStyles()
	}
	return Model{
		styles:           s,
		keymap:           km,
		store:            store,
		renderer:         renderer,
		highlighter:      highlighter,
		blamer:           cfg.Blamer,
		ref:              cfg.Ref,
		staged:           cfg.Staged,
		only:             cfg.Only,
		workDir:          cfg.WorkDir,
		noColors:         cfg.NoColors,
		noStatusBar:      cfg.NoStatusBar,
		noConfirmDiscard: cfg.NoConfirmDiscard,
		wrapMode:         cfg.Wrap,
		crossFileHunks:   cfg.CrossFileHunks,
		lineNumbers:      cfg.LineNumbers,
		collapsed:        collapsedState{enabled: cfg.Collapsed},
		showBlame:        cfg.ShowBlame && cfg.Blamer != nil,
		showUntracked:    false,
		loadUntracked:    cfg.LoadUntracked,
		focus:            paneTree,
		treeWidthRatio:   cfg.TreeWidthRatio,
		tabSpaces:        strings.Repeat(" ", cfg.TabWidth),
		themesDir:        cfg.ThemesDir,
		configPath:       cfg.ConfigPath,
		activeThemeName:  cfg.ActiveThemeName,
	}
}

// Store returns the annotation store for reading results after quit.
func (m Model) Store() *annotation.Store {
	return m.store
}

// Discarded returns true when the user chose to discard annotations and quit.
func (m Model) Discarded() bool {
	return m.discarded
}

// Init initializes the model by loading changed files.
func (m Model) Init() tea.Cmd {
	return m.loadFiles()
}

// Update handles messages and updates the model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inConfirmDiscard {
			return m.handleConfirmDiscardKey(msg)
		}
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg)
	case filesLoadedMsg:
		return m.handleFilesLoaded(msg)
	case fileLoadedMsg:
		return m.handleFileLoaded(msg)
	case blameLoadedMsg:
		return m.handleBlameLoaded(msg)
	}

	// forward other messages to textinput when annotating (e.g. cursor blink)
	if m.annotating {
		var cmd tea.Cmd
		m.annotateInput, cmd = m.annotateInput.Update(msg)
		m.viewport.SetContent(m.renderDiff()) // re-render so cursor blink updates are visible
		return m, cmd
	}

	// forward other messages to search textinput when searching (e.g. cursor blink)
	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, model, cmd := m.handleModalKey(msg); handled {
		return model, cmd
	}

	action := m.keymap.Resolve(msg.String())

	// help overlay: toggle with help action, dismiss with esc, block everything else
	if action == keymap.ActionHelp || m.showHelp {
		return m.handleHelpKey(msg)
	}

	switch action {
	case keymap.ActionAnnotList:
		m.annotListItems = m.buildAnnotListItems()
		m.annotListCursor = 0
		m.annotListOffset = 0
		m.showAnnotList = true
		return m, nil
	case keymap.ActionThemeSelect:
		m.openThemeSelector()
		return m, nil
	case keymap.ActionDismiss:
		return m.handleEscKey()
	case keymap.ActionDiscardQuit:
		return m.handleDiscardQuit()
	case keymap.ActionQuit:
		return m, tea.Quit
	case keymap.ActionTogglePane:
		m.togglePane()
		return m, nil
	case keymap.ActionFilter:
		return m.handleFilterToggle()
	case keymap.ActionNextItem:
		return m.handleFileOrSearchNav(true)
	case keymap.ActionPrevItem:
		return m.handleFileOrSearchNav(false)
	case keymap.ActionConfirm:
		return m.handleEnterKey()
	case keymap.ActionAnnotateFile:
		return m.handleFileAnnotateKey()
	case keymap.ActionMarkReviewed:
		return m.handleMarkReviewed()
	case keymap.ActionToggleCollapsed, keymap.ActionToggleWrap, keymap.ActionToggleTree, keymap.ActionToggleLineNums, keymap.ActionToggleBlame, keymap.ActionToggleUntracked:
		return m.handleViewToggle(action)
	case keymap.ActionNextHunk, keymap.ActionPrevHunk:
		return m.handleHunkNav(action == keymap.ActionNextHunk)
	default: // remaining actions (navigation, search, etc.) handled by pane-specific handlers below
	}

	// pane-specific navigation
	switch m.focus {
	case paneTree:
		return m.handleTreeNav(msg)
	case paneDiff:
		return m.handleDiffNav(msg)
	}
	return m, nil
}

func (m Model) handleModalKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// annotation input mode takes priority
	if m.annotating {
		model, cmd := m.handleAnnotateKey(msg)
		return true, model, cmd
	}

	// search input mode takes priority after annotation
	if m.searching {
		model, cmd := m.handleSearchKey(msg)
		return true, model, cmd
	}

	// annotation list popup: handle keys when already open
	if m.showAnnotList {
		model, cmd := m.handleAnnotListKey(msg)
		return true, model, cmd
	}

	// theme selector: handle keys when already open
	if m.themeSel.active {
		model, cmd := m.handleThemeSelectKey(msg)
		return true, model, cmd
	}

	return false, m, nil
}

// togglePane switches focus between tree and diff panes.
// only switches to diff pane when a file is loaded.
// no-op in single-file mode unless mdTOC is active (TOC uses paneTree slot).
func (m *Model) togglePane() {
	if m.treeHidden || (m.singleFile && m.mdTOC == nil) {
		return
	}
	if m.focus != paneTree {
		m.focus = paneTree
		m.syncTOCCursorToActive()
		return
	}
	if m.currFile != "" {
		m.focus = paneDiff
	}
}

// toggleTreePane hides or shows the tree/TOC pane.
// no-op in single-file mode without TOC (already no tree).
func (m *Model) toggleTreePane() {
	if m.singleFile && m.mdTOC == nil {
		return
	}
	m.treeHidden = !m.treeHidden
	if m.treeHidden {
		m.treeWidth = 0
		m.focus = paneDiff
		m.viewport.Width = m.width - 2
	} else {
		m.treeWidth = max(minTreeWidth, m.width*m.treeWidthRatio/10)
		m.viewport.Width = m.width - m.treeWidth - 4
	}
	m.viewport.Height = m.paneHeight() - 1
	m.syncViewportToCursor()
}

// toggleLineNumbers toggles line number display on/off and recomputes gutter width.
func (m *Model) toggleLineNumbers() {
	if m.focus != paneDiff || m.currFile == "" {
		return
	}
	m.lineNumbers = !m.lineNumbers
	if m.lineNumbers {
		m.lineNumWidth = m.computeLineNumWidth()
	}
	m.syncViewportToCursor()
}

// computeLineNumWidth returns the digit width needed for line number columns.
// scans all diffLines to find the maximum old or new line number.
func (m Model) computeLineNumWidth() int {
	maxNum := 0
	for _, dl := range m.diffLines {
		if dl.OldNum > maxNum {
			maxNum = dl.OldNum
		}
		if dl.NewNum > maxNum {
			maxNum = dl.NewNum
		}
	}
	if maxNum == 0 {
		return 1
	}
	return len(strconv.Itoa(maxNum))
}

// toggleBlame toggles the blame gutter on/off. returns a tea.Cmd to load blame data async.
func (m *Model) toggleBlame() tea.Cmd {
	if m.focus != paneDiff || m.currFile == "" || m.blamer == nil {
		return nil
	}
	m.showBlame = !m.showBlame
	if m.showBlame {
		m.blameData = nil
		m.blameAuthorLen = 0
		return m.loadBlame(m.currFile)
	}
	m.blameData = nil
	m.blameAuthorLen = 0
	m.syncViewportToCursor()
	return nil
}

// toggleUntracked toggles visibility of untracked files in the tree.
func (m *Model) toggleUntracked() tea.Cmd {
	m.showUntracked = !m.showUntracked
	return m.loadFiles()
}

// handleViewToggle dispatches view mode toggle actions.
func (m Model) handleViewToggle(action keymap.Action) (tea.Model, tea.Cmd) {
	switch action { //nolint:exhaustive // only toggle actions are dispatched here
	case keymap.ActionToggleCollapsed:
		m.toggleCollapsedMode()
	case keymap.ActionToggleWrap:
		m.toggleWrapMode()
	case keymap.ActionToggleTree:
		m.toggleTreePane()
	case keymap.ActionToggleLineNums:
		m.toggleLineNumbers()
	case keymap.ActionToggleBlame:
		cmd := m.toggleBlame()
		return m, cmd
	case keymap.ActionToggleUntracked:
		cmd := m.toggleUntracked()
		return m, cmd
	}
	return m, nil
}

// toggleWrapMode toggles line wrapping on/off.
// resets horizontal scroll when enabling wrap and re-renders the diff.
func (m *Model) toggleWrapMode() {
	if m.focus != paneDiff || m.currFile == "" {
		return
	}
	m.wrapMode = !m.wrapMode
	if m.wrapMode {
		m.scrollX = 0
	}
	m.syncViewportToCursor()
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	var diffWidth int
	if m.treeHidden || (m.singleFile && m.mdTOC == nil) {
		m.treeWidth = 0
		diffWidth = m.width - 2 // diff pane borders only
	} else {
		// adjust tree width based on ratio (N out of 10 units);
		// applies to multi-file mode and single-file markdown with TOC
		m.treeWidth = max(minTreeWidth, m.width*m.treeWidthRatio/10)
		diffWidth = m.width - m.treeWidth - 4 // borders
	}
	diffHeight := m.paneHeight() - 1 // pane height minus diff header

	if !m.ready {
		m.viewport = viewport.New(diffWidth, diffHeight)
		m.ready = true
	} else {
		m.viewport.Width = diffWidth
		m.viewport.Height = diffHeight
	}

	m.tree.ensureVisible(m.treePageSize())

	if m.currFile != "" {
		m.syncViewportToCursor()
	}

	return m, nil
}

