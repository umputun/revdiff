package ui

//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer
//go:generate moq -out mocks/syntax_highlighter.go -pkg mocks -skip-ensure -fmt goimports . SyntaxHighlighter
//go:generate moq -out mocks/blamer.go -pkg mocks -skip-ensure -fmt goimports . Blamer
//go:generate moq -out mocks/style_resolver.go -pkg mocks -skip-ensure -fmt goimports . styleResolver
//go:generate moq -out mocks/style_renderer.go -pkg mocks -skip-ensure -fmt goimports . styleRenderer
//go:generate moq -out mocks/sgr_processor.go -pkg mocks -skip-ensure -fmt goimports . sgrProcessor
//go:generate moq -out mocks/word_differ.go -pkg mocks -skip-ensure -fmt goimports . wordDiffer

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
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

// Blamer provides blame information for files.
type Blamer interface {
	FileBlame(ref, file string, staged bool) (map[int]diff.BlameLine, error)
}

// styleResolver is what Model needs for static and runtime style/color lookups.
// Implemented by style.Resolver.
type styleResolver interface {
	Color(k style.ColorKey) style.Color
	Style(k style.StyleKey) lipgloss.Style
	LineBg(change diff.ChangeType) style.Color
	LineStyle(change diff.ChangeType, highlighted bool) lipgloss.Style
	WordDiffBg(change diff.ChangeType) style.Color
	IndicatorBg(change diff.ChangeType) style.Color
}

// styleRenderer is what Model needs for compound ANSI rendering operations.
// Implemented by style.Renderer.
type styleRenderer interface {
	AnnotationInline(text string) string
	DiffCursor(noColors bool) string
	StatusBarSeparator() string
	FileStatusMark(status diff.FileStatus) string
	FileReviewedMark() string
	FileAnnotationMark() string
}

// sgrProcessor is what Model needs for ANSI SGR stream processing.
// Implemented by style.SGR.
type sgrProcessor interface {
	Reemit(lines []string) []string
}

// wordDiffer is what Model needs for intra-line word-diff and highlight insertion.
// Implemented by *worddiff.Differ.
type wordDiffer interface {
	ComputeIntraRanges(minusLine, plusLine string) ([]worddiff.Range, []worddiff.Range)
	PairLines(lines []worddiff.LinePair) []worddiff.Pair
	InsertHighlightMarkers(s string, matches []worddiff.Range, hlOn, hlOff string) string
}

// compile-time assertions — enforce that the concrete package types
// satisfy the consumer-side interfaces.
var (
	_ styleResolver = (*style.Resolver)(nil)
	_ styleRenderer = (*style.Renderer)(nil)
	_ sgrProcessor  = (*style.SGR)(nil)
	_ wordDiffer    = (*worddiff.Differ)(nil)
)

// FileTreeComponent is what Model needs from a file-tree navigation component.
// Implemented by *sidepane.FileTree. Exported so main.go can spell it in the
// factory closure's return type.
type FileTreeComponent interface {
	// SelectedFile returns the full path of the currently selected file.
	SelectedFile() string
	// TotalFiles returns the count of original file paths (before filtering).
	TotalFiles() int
	// FileStatus returns the git change status for the given file path.
	FileStatus(path string) diff.FileStatus
	// FilterActive returns true when the file tree is showing only annotated files.
	FilterActive() bool
	// ReviewedCount returns the number of files marked as reviewed.
	ReviewedCount() int
	// HasFile returns true if there is a file entry in the given direction.
	HasFile(dir sidepane.Direction) bool
	// Move navigates the cursor according to the given motion.
	Move(m sidepane.Motion, count ...int)
	// StepFile moves to the next or previous file entry, wrapping around at ends.
	StepFile(dir sidepane.Direction)
	// SelectByPath sets the cursor to the file entry matching the given path.
	SelectByPath(path string) bool
	// EnsureVisible adjusts offset so the cursor is within the visible range.
	EnsureVisible(height int)
	// Rebuild rebuilds the file tree from new entries in-place.
	Rebuild(entries []diff.FileEntry)
	// ToggleFilter toggles between showing all files and only annotated files.
	ToggleFilter(annotated map[string]bool)
	// RefreshFilter updates the filtered view with the current annotation state.
	RefreshFilter(annotated map[string]bool)
	// ToggleReviewed toggles the reviewed mark for the given file path.
	ToggleReviewed(path string)
	// Render renders the file tree into a string for display.
	Render(r sidepane.FileTreeRender) string
}

// TOCComponent is what Model needs from a table-of-contents navigation component.
// Implemented by *sidepane.TOC. Exported so main.go can spell it in the factory closure.
type TOCComponent interface {
	// CurrentLineIdx returns the diff line index for the current TOC cursor entry.
	CurrentLineIdx() (int, bool)
	// NumEntries returns the number of TOC entries.
	NumEntries() int
	// Move navigates the cursor according to the given motion.
	Move(m sidepane.Motion, count ...int)
	// EnsureVisible adjusts offset so the cursor is within the visible range.
	EnsureVisible(height int)
	// UpdateActiveSection sets the active section based on the diff cursor position.
	UpdateActiveSection(diffCursor int)
	// SyncCursorToActiveSection sets cursor to activeSection when activeSection >= 0.
	SyncCursorToActiveSection()
	// Render renders the TOC into a string for display.
	Render(r sidepane.TOCRender) string
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
	resolver     styleResolver
	renderer     styleRenderer
	sgr          sgrProcessor
	differ       wordDiffer
	tree         FileTreeComponent // never nil after NewModel; starts empty, gets Rebuilt on filesLoadedMsg
	viewport     viewport.Model
	parseTOC     func(lines []diff.DiffLine, filename string) TOCComponent
	store        *annotation.Store
	diffRenderer Renderer
	keymap       *keymap.Keymap

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

	highlighter        SyntaxHighlighter  // syntax highlighter
	highlightedLines   []string           // pre-computed highlighted content, parallel to diffLines
	intraRanges        [][]worddiff.Range // per-line intra-line word-diff ranges, parallel to diffLines (nil for unpaired lines)
	diffLines          []diff.DiffLine    // current file's parsed diff lines
	currFile           string             // currently displayed file
	loadSeq            uint64             // monotonic counter to identify the latest load request
	ready              bool               // true after first WindowSizeMsg
	annotating         bool               // true when annotation text input is active
	fileAnnotating     bool               // true when annotating at file level (Line=0)
	cursorOnAnnotation bool               // true when cursor is on the annotation sub-line (not the diff line)
	annotateInput      textinput.Model    // text input for annotations

	collapsed collapsedState // collapsed diff view state

	fileAdds    int // cached count of added lines in current file
	fileRemoves int // cached count of removed lines in current file

	showHelp         bool // true when help overlay is visible
	wrapMode         bool // true when line wrapping is enabled
	crossFileHunks   bool // allow [ and ] to jump across file boundaries
	lineNumbers      bool // true when line numbers are shown in gutter
	lineNumWidth     int  // digit width for line number columns (max digits across old/new nums)
	singleColLineNum bool // true for full-context files: render one line-number column instead of two

	wordDiff       bool                     // true when intra-line word-diff highlighting is enabled
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

	mdTOC TOCComponent // markdown table-of-contents for single-file full-context markdown mode (nil when not applicable)

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

// ModelConfig holds all dependencies and configuration for NewModel.
// All dependencies (Renderer, Store, Highlighter, StyleResolver, StyleRenderer, SGR, WordDiffer)
// are required and must be constructed by the caller. Blamer is optional.
type ModelConfig struct {
	// --- UI dependencies (required, caller-constructed) ---
	Renderer    Renderer          // diff renderer: ChangedFiles, FileDiff
	Store       *annotation.Store // annotation store
	Highlighter SyntaxHighlighter // syntax highlighter

	// --- Style dependencies (required, caller-constructed) ---
	StyleResolver styleResolver // color/style lookups
	StyleRenderer styleRenderer // compound ANSI rendering
	SGR           sgrProcessor  // SGR stream reemit

	// --- Word-diff dependency (required, caller-constructed) ---
	WordDiffer wordDiffer // intra-line diff and highlight insertion

	// --- Sidepane factories (required, wired from main.go) ---

	// NewFileTree constructs a fresh FileTreeComponent from the file list.
	// Injected by main.go (typically a closure wrapping sidepane.NewFileTree).
	// Required — NewModel returns an error when nil.
	NewFileTree func(entries []diff.FileEntry) FileTreeComponent

	// ParseTOC parses markdown headers from diff lines into a TOCComponent.
	// Returns nil when no headers are found. The closure must collapse typed-nil
	// to interface-nil for empty TOCs to avoid the typed-nil trap.
	// Injected by main.go (typically a closure wrapping sidepane.ParseTOC).
	// Required — NewModel returns an error when nil.
	ParseTOC func(lines []diff.DiffLine, filename string) TOCComponent

	// --- Optional dependencies ---
	Blamer        Blamer                   // optional blame provider (nil when git unavailable)
	LoadUntracked func() ([]string, error) // optional untracked-files fetcher (nil when unavailable)
	Keymap        *keymap.Keymap           // custom key bindings (nil uses defaults)

	// --- Configuration values ---
	Ref              string
	Staged           bool
	TreeWidthRatio   int
	TabWidth         int      // number of spaces per tab character
	NoColors         bool     // disable all colors including syntax highlighting
	NoStatusBar      bool     // hide the status bar
	NoConfirmDiscard bool     // skip confirmation prompt when discarding annotations
	Wrap             bool     // enable line wrapping
	Collapsed        bool     // start in collapsed diff mode
	CrossFileHunks   bool     // allow [ and ] to jump across file boundaries
	LineNumbers      bool     // show line numbers in diff gutter
	ShowBlame        bool     // show blame gutter on startup when available
	WordDiff         bool     // enable intra-line word-diff highlighting on startup
	Only             []string // show only these files (match by exact path or path suffix)
	WorkDir          string   // working directory for resolving absolute --only paths
	ThemesDir        string   // path to themes directory for theme selector
	ConfigPath       string   // path to config file for persisting theme choice
	ActiveThemeName  string   // name of theme currently applied (for theme selector cursor positioning)
}

// NewModel creates a new Model from the given configuration. All dependencies
// must be provided by the caller — there is no fallback construction.
// Returns an error if any required dependency is missing from the config.
func NewModel(cfg ModelConfig) (Model, error) {
	if cfg.Renderer == nil {
		return Model{}, errors.New("ui.NewModel: cfg.Renderer is required")
	}
	if cfg.Store == nil {
		return Model{}, errors.New("ui.NewModel: cfg.Store is required")
	}
	if cfg.Highlighter == nil {
		return Model{}, errors.New("ui.NewModel: cfg.Highlighter is required")
	}
	if cfg.StyleResolver == nil {
		return Model{}, errors.New("ui.NewModel: cfg.StyleResolver is required")
	}
	if cfg.StyleRenderer == nil {
		return Model{}, errors.New("ui.NewModel: cfg.StyleRenderer is required")
	}
	if cfg.SGR == nil {
		return Model{}, errors.New("ui.NewModel: cfg.SGR is required")
	}
	if cfg.WordDiffer == nil {
		return Model{}, errors.New("ui.NewModel: cfg.WordDiffer is required")
	}
	if cfg.NewFileTree == nil {
		return Model{}, errors.New("ui.NewModel: cfg.NewFileTree is required")
	}
	if cfg.ParseTOC == nil {
		return Model{}, errors.New("ui.NewModel: cfg.ParseTOC is required")
	}
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

	return Model{
		resolver:         cfg.StyleResolver,
		renderer:         cfg.StyleRenderer,
		sgr:              cfg.SGR,
		differ:           cfg.WordDiffer,
		keymap:           km,
		store:            cfg.Store,
		diffRenderer:     cfg.Renderer,
		highlighter:      cfg.Highlighter,
		blamer:           cfg.Blamer,
		tree:             cfg.NewFileTree(nil), // empty tree for nil-safety before first filesLoadedMsg
		parseTOC:         cfg.ParseTOC,
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
		wordDiff:         cfg.WordDiff,
		showBlame:        cfg.ShowBlame && cfg.Blamer != nil,
		showUntracked:    false,
		loadUntracked:    cfg.LoadUntracked,
		focus:            paneTree,
		treeWidthRatio:   cfg.TreeWidthRatio,
		tabSpaces:        strings.Repeat(" ", cfg.TabWidth),
		themesDir:        cfg.ThemesDir,
		configPath:       cfg.ConfigPath,
		activeThemeName:  cfg.ActiveThemeName,
	}, nil
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
	case keymap.ActionToggleCollapsed, keymap.ActionToggleWrap, keymap.ActionToggleTree, keymap.ActionToggleLineNums,
		keymap.ActionToggleBlame, keymap.ActionToggleWordDiff, keymap.ActionToggleUntracked:
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

// treePaneHidden returns true when the tree/TOC pane should be hidden.
// true when user toggled it off, or in single-file mode without a markdown TOC.
func (m Model) treePaneHidden() bool {
	return m.treeHidden || (m.singleFile && m.mdTOC == nil)
}

// isCursorLine returns true when the diff line at idx is the active cursor line.
func (m Model) isCursorLine(idx int) bool {
	return idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation
}

// togglePane switches focus between tree and diff panes.
// only switches to diff pane when a file is loaded.
// no-op in single-file mode unless mdTOC is active (TOC uses paneTree slot).
func (m *Model) togglePane() {
	if m.treePaneHidden() {
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

// toggleWordDiff toggles intra-line word-diff highlighting on/off.
// recomputeIntraRanges honors the new wordDiff state: it populates ranges
// when enabling and clears them when disabling.
// no-op when the diff pane is not focused or no file is loaded.
func (m *Model) toggleWordDiff() {
	if m.focus != paneDiff || m.currFile == "" {
		return
	}
	m.wordDiff = !m.wordDiff
	m.recomputeIntraRanges()
	m.viewport.SetContent(m.renderDiff())
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
	case keymap.ActionToggleWordDiff:
		m.toggleWordDiff()
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
	if m.treePaneHidden() {
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

	m.tree.EnsureVisible(m.treePageSize())

	if m.currFile != "" {
		m.syncViewportToCursor()
	}

	return m, nil
}
