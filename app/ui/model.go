package ui

//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer
//go:generate moq -out mocks/syntax_highlighter.go -pkg mocks -skip-ensure -fmt goimports . SyntaxHighlighter
//go:generate moq -out mocks/blamer.go -pkg mocks -skip-ensure -fmt goimports . Blamer
//go:generate moq -out mocks/style_resolver.go -pkg mocks -skip-ensure -fmt goimports . styleResolver
//go:generate moq -out mocks/style_renderer.go -pkg mocks -skip-ensure -fmt goimports . styleRenderer
//go:generate moq -out mocks/sgr_processor.go -pkg mocks -skip-ensure -fmt goimports . sgrProcessor
//go:generate moq -out mocks/word_differ.go -pkg mocks -skip-ensure -fmt goimports . wordDiffer
//go:generate moq -out mocks/external_editor.go -pkg mocks -skip-ensure -fmt goimports . ExternalEditor
//go:generate moq -out mocks/commit_log_source.go -pkg mocks -skip-ensure -fmt goimports . commitLogSource

// note: ThemeCatalog is not moq-generated because ThemeEntry/ThemeSpec are defined in this package,
// creating an import cycle (ui -> mocks -> ui). Tests use manual fakes instead.

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/editor"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

// Renderer provides methods to extract changed files and build diff views.
// contextLines controls surrounding context on FileDiff: 0 or >= 1000000 requests
// full-file context (the revdiff default); positive values < 1000000 request that
// many lines on each side of a hunk. Context-only sources ignore this parameter.
type Renderer interface {
	ChangedFiles(ref string, staged bool) ([]diff.FileEntry, error)
	FileDiff(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error)
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

// overlayManager is what Model needs for overlay popup coordination.
// Implemented by *overlay.Manager.
type overlayManager interface {
	Active() bool
	Kind() overlay.Kind
	OpenHelp(spec overlay.HelpSpec)
	OpenAnnotList(spec overlay.AnnotListSpec)
	OpenThemeSelect(spec overlay.ThemeSelectSpec)
	OpenCommitInfo(spec overlay.CommitInfoSpec)
	Close()
	HandleKey(msg tea.KeyMsg, action keymap.Action) overlay.Outcome
	HandleMouse(msg tea.MouseMsg) overlay.Outcome
	Compose(base string, ctx overlay.RenderCtx) string
}

// commitLogSource is what Model needs to enumerate commits in the current ref range
// for the commit-info overlay. Implemented by diff.Git, diff.Hg, and diff.Jj via
// the diff.CommitLogger capability interface; nil means the feature is unavailable
// (e.g. stdin mode, FileReader, DirectoryReader, or any wrapper that hides the
// underlying VCS). Defined on the consumer side per Go convention.
type commitLogSource interface {
	CommitLog(ref string) ([]diff.CommitInfo, error)
}

// ThemeCatalog is what Model needs for theme discovery and persistence.
// The UI calls Entries() to populate the theme selector overlay, Resolve() to
// preview or apply a chosen theme, and Persist() to save the user's choice.
// Implemented by a concrete type in app/theme, wired through ModelConfig.
type ThemeCatalog interface {
	Entries() ([]ThemeEntry, error)
	Resolve(name string) (ThemeSpec, bool)
	Persist(name string) error
}

// ThemeEntry is minimal list-view data for one theme in the selector overlay.
type ThemeEntry struct {
	Name        string
	Local       bool
	AccentColor string
}

// ThemeSpec holds the runtime-ready representation of a theme for preview/apply.
// UI should not import app/theme — this struct carries everything needed to
// rebuild style.Resolver / style.Renderer / chroma style from a theme choice.
type ThemeSpec struct {
	Colors      style.Colors
	ChromaStyle string
}

// compile-time assertions — enforce that the concrete package types
// satisfy the consumer-side interfaces.
var (
	_ styleResolver  = (*style.Resolver)(nil)
	_ styleRenderer  = (*style.Renderer)(nil)
	_ sgrProcessor   = (*style.SGR)(nil)
	_ wordDiffer     = (*worddiff.Differ)(nil)
	_ overlayManager = (*overlay.Manager)(nil)
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
	// SelectByVisibleRow sets the cursor to the entry at the given visible row
	// (0-based, relative to the first visible tree line). Returns true when the
	// row maps to a valid entry; the cursor is unchanged when false.
	SelectByVisibleRow(row int) bool
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
	// ScrollState reports the file tree's visible window after rendering.
	ScrollState() sidepane.ScrollState
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
	// SelectByVisibleRow sets the cursor to the entry at the given visible row
	// (0-based, relative to the first visible TOC line). Returns true when the
	// row maps to a valid entry; the cursor is unchanged when false.
	SelectByVisibleRow(row int) bool
	// EnsureVisible adjusts offset so the cursor is within the visible range.
	EnsureVisible(height int)
	// UpdateActiveSection sets the active section based on the diff cursor position.
	UpdateActiveSection(diffCursor int)
	// SyncCursorToActiveSection sets cursor to activeSection when activeSection >= 0.
	SyncCursorToActiveSection()
	// ScrollState reports the TOC's visible window after rendering.
	ScrollState() sidepane.ScrollState
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

// loadedFileState holds all state related to the currently loaded file.
// it groups parallel arrays (lines, highlighted, intraRanges) and derived
// metadata (adds/removes, blame, line numbering) into a single coherent
// object, making the synchronization invariant explicit.
type loadedFileState struct {
	name             string                 // currently displayed file path
	lines            []diff.DiffLine        // parsed diff lines
	highlighted      []string               // pre-computed highlighted content, parallel to lines
	intraRanges      [][]worddiff.Range     // per-line intra-line word-diff ranges, parallel to lines
	adds             int                    // cached count of added lines
	removes          int                    // cached count of removed lines
	blameData        map[int]diff.BlameLine // blame info keyed by 1-based new line number
	blameAuthorLen   int                    // max author display width for blame gutter
	lineNumWidth     int                    // digit width for line number columns
	singleColLineNum bool                   // true for full-context files: one line-number column
	loadSeq          uint64                 // monotonic counter to identify the latest load request
	mdTOC            TOCComponent           // markdown table-of-contents (nil when not applicable)
	singleFile       bool                   // true when diff contains exactly one file
}

// modelConfigState holds immutable or near-immutable session configuration.
// these values are set once at startup and not changed during runtime.
type modelConfigState struct {
	ref              string   // git ref for diff
	staged           bool     // show staged changes
	only             []string // filter to show only matching files
	workDir          string   // working directory for resolving absolute --only paths
	noColors         bool     // keep monochrome output when previewing or applying themes
	noStatusBar      bool     // hide the status bar
	noConfirmDiscard bool     // skip confirmation prompt on discard quit
	crossFileHunks   bool     // allow [ and ] to jump across file boundaries
	treeWidthRatio   int      // 1-10 units for file tree panel
	tabSpaces        string   // spaces to replace tabs with
}

// layoutState holds viewport and layout concerns that change on resize and pane toggles.
type layoutState struct {
	viewport   viewport.Model // scrollable diff viewport
	focus      pane           // which pane has focus
	treeHidden bool           // user toggled tree/TOC pane off
	width      int            // terminal width
	height     int            // terminal height
	treeWidth  int            // current tree pane width in columns
	scrollX    int            // horizontal scroll offset for diff pane
}

// modeState holds user-togglable view-mode flags.
// these are display modes that the user can switch at runtime via keybindings.
type modeState struct {
	wrap           bool           // true when line wrapping is enabled
	collapsed      collapsedState // collapsed diff view state
	lineNumbers    bool           // true when line numbers are shown in gutter
	wordDiff       bool           // true when intra-line word-diff highlighting is enabled
	showBlame      bool           // true when blame gutter is shown
	showUntracked  bool           // true when untracked files are shown in tree
	compact        bool           // true when diffs are fetched with small context around changes
	compactContext int            // number of context lines around changes when compact is enabled
	vimMotion      bool           // true when the --vim-motion preset is active (gates the vim-motion interceptor in handleKey)
}

// navigationState holds cursor and navigation-adjacent state.
type navigationState struct {
	diffCursor      int   // index into file.lines for current cursor line
	pendingHunkJump *bool // pending hunk jump after cross-file hunk navigation (true=first, false=last)
}

// searchState holds all search lifecycle state.
type searchState struct {
	active   bool            // true when search textinput is active (typing)
	term     string          // last submitted search query
	matches  []int           // indices into file.lines that match
	cursor   int             // current position in matches (0-based)
	input    textinput.Model // dedicated textinput for search
	matchSet map[int]bool    // set of file.lines indices that match, computed per render
}

// commitsState holds the commit log for the commit-info overlay, fetched
// eagerly at startup (and on R reload) via loadCommits under tea.Batch. loaded
// flips to true once the first commitsLoadedMsg lands (success or failure) so
// handleCommitInfo only reads cached state — pressing `i` before the fetch
// resolves shows a transient "loading commits…" hint instead of triggering a
// fetch. loadSeq is bumped before each new load; handleCommitsLoaded drops
// messages whose seq no longer matches, discarding stale in-flight results
// after a reload. applicable mirrors ModelConfig.CommitsApplicable, copied at
// construction so the handler can short-circuit without consulting CLI flags.
// hint holds a transient status-bar message shown once (until the next key
// press) — set when the user triggers ActionCommitInfo in a mode where the
// feature is unavailable, so the key press has visible feedback instead of
// appearing inert.
type commitsState struct {
	source     commitLogSource   // VCS-backed log source; nil disables the feature
	applicable bool              // true when current mode supports a commit list
	loaded     bool              // true once a fetch attempt has populated the cache
	list       []diff.CommitInfo // cached commits (may be empty after a successful empty-range fetch)
	truncated  bool              // true when the list was capped at diff.MaxCommits
	err        error             // last fetch error; surfaces in the overlay
	hint       string            // transient status-bar message; cleared on next key press
	loadSeq    uint64            // bumped before each new commit-log load; stale commitsLoadedMsg (seq mismatch) is dropped
}

// reloadState holds the pending-confirmation state for the R reload feature.
// hint is a transient status-bar message cleared on the next key press.
// applicable is false in stdin mode (stream consumed; reload impossible).
type reloadState struct {
	pending    bool   // true when waiting for y/other-key confirmation
	hint       string // transient status-bar message; cleared on next key press
	applicable bool   // false when reload is unavailable (e.g. --stdin)
}

// compactState holds runtime state for the compact diff mode feature.
// applicable mirrors ModelConfig.CompactApplicable, copied at construction so
// the toggle handler can short-circuit without consulting CLI flags: false
// when the underlying source is context-only (stdin, all-files, standalone
// FileReader) and shrinking context makes no sense. hint is a transient
// status-bar message set when the toggle fires in an unavailable mode, so
// the key press has visible feedback; cleared on the next key press, matching
// the commits.hint / reload.hint lifecycle. The user-controlled toggle state
// (on/off, context size) lives on modeState alongside the other view toggles.
type compactState struct {
	applicable bool   // true when current mode supports compact diffs
	hint       string // transient status-bar message; cleared on next key press
}

// keyState holds transient key-dispatch state for the leader-chord feature.
// It lives separate from navigationState because chord state is a key-dispatch
// concern, not a cursor/scroll concern — keeping the two split prevents
// navigationState from growing into a grab-bag. chordPending holds the leader
// key while waiting for the second-stage key ("" otherwise); hint is a
// transient status-bar message ("Pending: …" while waiting, "Unknown chord: …"
// on a miss) cleared on the next key press.
type keyState struct {
	chordPending string // leader key while waiting for the second-stage key; "" otherwise
	hint         string // transient status-bar message; cleared on next key press
}

// vimState holds vim-motion preset state: count prefix accumulator and
// pending letter leader. Distinct from keyState (ctrl/alt chord dispatch);
// the two are orthogonal and run in different guards of handleKey. The vim
// interceptor runs only when modes.vimMotion is true; when off, all fields
// stay at their zero values. Invariant: count > 0 and leader != "" never
// coexist (enforced in interceptor code, not types).
type vimState struct {
	count  int    // accumulated count prefix; 0 = none pending
	leader string // pending letter leader: "g", "z", "Z", or ""
	hint   string // transient status-bar message; cleared on next key press
}

// annotationState holds annotation input lifecycle state.
type annotationState struct {
	annotating         bool            // true when annotation text input is active
	fileAnnotating     bool            // true when annotating at file level (Line=0)
	cursorOnAnnotation bool            // true when cursor is on the annotation sub-line (not the diff line)
	input              textinput.Model // text input for annotations
	// existingMultiline holds the original multi-line comment of an annotation
	// being re-edited. textinput's sanitizer collapses \n to space, so pre-filling
	// via SetValue would silently flatten the stored content. When set, the
	// textinput is left empty with a hint placeholder; Ctrl+E seeds the editor
	// from this field, and Enter with empty input preserves the existing content
	// unchanged. Cleared on every annotation-mode exit path.
	existingMultiline string
}

// Model is the top-level bubbletea model for revdiff.
type Model struct {
	// injected dependencies
	resolver     styleResolver
	renderer     styleRenderer
	sgr          sgrProcessor
	differ       wordDiffer
	overlay      overlayManager
	tree         FileTreeComponent // never nil after NewModel; starts empty, gets Rebuilt on filesLoadedMsg
	parseTOC     func(lines []diff.DiffLine, filename string) TOCComponent
	store        *annotation.Store
	diffRenderer Renderer
	keymap       *keymap.Keymap
	themes       ThemeCatalog   // theme catalog for discovery, resolve, and persistence
	editor       ExternalEditor // launches $EDITOR for multi-line annotation editing

	// grouped state
	cfg    modelConfigState // immutable session config
	layout layoutState      // viewport and layout
	modes  modeState        // user-togglable view modes
	nav    navigationState  // cursor and navigation

	highlighter SyntaxHighlighter // syntax highlighter
	file        loadedFileState   // current file's loaded state (lines, highlights, blame, etc.)
	search      searchState       // search lifecycle state
	annot       annotationState   // annotation input lifecycle state
	commits     commitsState      // eagerly loaded commit log for the commit-info overlay
	reload      reloadState       // pending-confirmation state and applicability for R reload
	compact     compactState      // applicability + transient hint for compact diff mode
	keys        keyState          // chord-pending state and transient hint for leader-chord keybindings
	vim         vimState          // count accumulator, pending letter leader, and transient hint for vim-motion preset

	ready        bool   // true after first WindowSizeMsg
	filesLoaded  bool   // true after the first filesLoadedMsg is handled (keeps the loading view pinned until real data arrives)
	filesLoadSeq uint64 // bumped before each new file-list load; stale filesLoadedMsg (seq mismatch) is dropped

	blamer        Blamer                   // optional blame provider (nil when git unavailable)
	loadUntracked func() ([]string, error) // fetches untracked files; nil when unavailable
	blameNow      time.Time                // snapshot of time.Now() set once per render pass for blame age

	discarded        bool // true when user chose to discard annotations and quit
	inConfirmDiscard bool // true when showing discard confirmation prompt

	pendingAnnotJump *annotation.Annotation // pending jump target after cross-file annotation list jump

	activeThemeName string               // name of currently applied theme (for cursor positioning)
	themePreview    *themePreviewSession // non-nil while theme selector is open
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
	seq      uint64 // matches m.filesLoadSeq at the time the load was issued; mismatched messages are dropped
	entries  []diff.FileEntry
	err      error
	warnings []string // non-fatal issues (staged/untracked fetch failures)
}

// commitsLoadedMsg is sent when the commit log for the current ref range is loaded.
type commitsLoadedMsg struct {
	seq       uint64 // matches m.commits.loadSeq at the time the load was issued; mismatched messages are dropped
	list      []diff.CommitInfo
	err       error
	truncated bool
}

// ModelConfig holds all dependencies and configuration for NewModel.
// All dependencies (Renderer, Store, Highlighter, StyleResolver, StyleRenderer, SGR, WordDiffer, Overlay,
// NewFileTree, ParseTOC, Themes) are required and must be constructed by the caller.
// Blamer, LoadUntracked, and Keymap are optional.
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

	// --- Overlay dependency (required, caller-constructed) ---
	Overlay overlayManager // overlay popup coordinator

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

	// --- Theme catalog (required, wired from main.go) ---
	Themes ThemeCatalog // theme discovery, resolve, and persistence

	// --- Optional dependencies ---
	Blamer        Blamer                   // optional blame provider (nil when git unavailable)
	LoadUntracked func() ([]string, error) // optional untracked-files fetcher (nil when unavailable)
	Keymap        *keymap.Keymap           // custom key bindings (nil uses defaults)
	Editor        ExternalEditor           // external-editor driver (nil uses app/editor.Editor{})
	// CommitLog enumerates commits in the current ref range for the commit-info
	// overlay. When nil, NewModel attempts to derive the source by type-asserting
	// the Renderer against diff.CommitLogger; if the assertion fails, the feature
	// is unavailable and the `i` hotkey acts as a no-op. Pass a typed-nil
	// (e.g. var c *Foo; cfg.CommitLog = c) and the typed-nil is collapsed to
	// nil before the type-assertion fallback runs (mirrors the Editor guard).
	CommitLog commitLogSource

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
	ActiveThemeName  string   // name of theme currently applied (for theme selector cursor positioning)
	// CommitsApplicable is the composition-root verdict on whether the current
	// invocation supports a commit-info popup. Computed once in main.go from the
	// full option set (stdin, staged, only, all-files, ref) and copied into Model
	// state. Model does not re-derive from CLI flags because modelConfigState
	// today does not carry stdin/all-files; keeping the computation in the
	// composition root avoids scope creep.
	CommitsApplicable bool
	// ReloadApplicable is false when --stdin is active (stream already consumed;
	// reload is impossible). Computed at composition root in main.go and copied
	// into Model state. Follows the same pattern as CommitsApplicable.
	ReloadApplicable bool
	// Compact is the initial value for the compact diff mode toggle. When true
	// the UI starts with small-context diffs (CompactContext lines around each
	// change) instead of the full-file default. Runtime-toggleable via C.
	Compact bool
	// CompactContext is the number of context lines requested from the VCS
	// when compact mode is active. Zero or negative values (and values at or
	// above the full-file sentinel) are treated as full-file context.
	CompactContext int
	// CompactApplicable is the composition-root verdict on whether the current
	// invocation can shrink the diff. false in --stdin and --all-files modes
	// and when the underlying source is a context-only reader (no changes to
	// contextualize). Computed once in main.go and copied into Model state.
	// Follows the same pattern as CommitsApplicable.
	CompactApplicable bool
	// VimMotion enables the vim-style motion preset (counts, gg, G, zz/zt/zb,
	// ZZ/ZQ). When true, the vim-motion interceptor in handleKey runs between
	// the modal-key handler and keymap.Resolve. Copied into modes.vimMotion at
	// construction; the feature is gated on that field everywhere.
	VimMotion bool
}

// NewModel creates a new Model from the given configuration. All dependencies
// must be provided by the caller — there is no fallback construction.
// Returns an error if any required dependency is missing from the config.
// isNilValue reports whether v is a typed-nil interface value (e.g. a (*T)(nil)
// wrapped in an interface). Used to guard interface fields where "nil means
// default" must survive a caller passing a typed-nil pointer.
func isNilValue(v any) bool {
	rv := reflect.ValueOf(v)
	k := rv.Kind()
	if k == reflect.Ptr || k == reflect.Interface || k == reflect.Chan ||
		k == reflect.Func || k == reflect.Map || k == reflect.Slice {
		return rv.IsNil()
	}
	return false
}

// validateRequired checks every non-optional ModelConfig field is populated.
// returns a single "<field> is required" error for the first missing dependency.
func (cfg ModelConfig) validateRequired() error {
	required := []struct {
		name string
		ok   bool
	}{
		{"Renderer", cfg.Renderer != nil},
		{"Store", cfg.Store != nil},
		{"Highlighter", cfg.Highlighter != nil},
		{"StyleResolver", cfg.StyleResolver != nil},
		{"StyleRenderer", cfg.StyleRenderer != nil},
		{"SGR", cfg.SGR != nil},
		{"WordDiffer", cfg.WordDiffer != nil},
		{"Overlay", cfg.Overlay != nil},
		{"NewFileTree", cfg.NewFileTree != nil},
		{"ParseTOC", cfg.ParseTOC != nil},
		{"Themes", cfg.Themes != nil},
	}
	for _, r := range required {
		if !r.ok {
			return fmt.Errorf("ui.NewModel: cfg.%s is required", r.name)
		}
	}
	return nil
}

func NewModel(cfg ModelConfig) (Model, error) {
	if err := cfg.validateRequired(); err != nil {
		return Model{}, err
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
	ed := cfg.Editor
	if ed == nil || isNilValue(ed) {
		ed = editor.Editor{}
	}
	cls := resolveCommitLogSource(cfg.CommitLog, cfg.Renderer)

	return Model{
		resolver:     cfg.StyleResolver,
		renderer:     cfg.StyleRenderer,
		sgr:          cfg.SGR,
		differ:       cfg.WordDiffer,
		overlay:      cfg.Overlay,
		keymap:       km,
		store:        cfg.Store,
		diffRenderer: cfg.Renderer,
		highlighter:  cfg.Highlighter,
		blamer:       cfg.Blamer,
		tree:         cfg.NewFileTree(nil), // empty tree for nil-safety before first filesLoadedMsg
		parseTOC:     cfg.ParseTOC,
		themes:       cfg.Themes,
		editor:       ed,
		cfg: modelConfigState{
			ref:              cfg.Ref,
			staged:           cfg.Staged,
			only:             cfg.Only,
			workDir:          cfg.WorkDir,
			noColors:         cfg.NoColors,
			noStatusBar:      cfg.NoStatusBar,
			noConfirmDiscard: cfg.NoConfirmDiscard,
			crossFileHunks:   cfg.CrossFileHunks,
			treeWidthRatio:   cfg.TreeWidthRatio,
			tabSpaces:        strings.Repeat(" ", cfg.TabWidth),
		},
		layout: layoutState{
			focus: paneTree,
		},
		modes: modeState{
			wrap:           cfg.Wrap,
			lineNumbers:    cfg.LineNumbers,
			collapsed:      collapsedState{enabled: cfg.Collapsed},
			wordDiff:       cfg.WordDiff,
			showBlame:      cfg.ShowBlame && cfg.Blamer != nil,
			showUntracked:  false,
			compact:        cfg.Compact && cfg.CompactApplicable,
			compactContext: cfg.CompactContext,
			vimMotion:      cfg.VimMotion,
		},
		commits: commitsState{
			source:     cls,
			applicable: cfg.CommitsApplicable && cls != nil,
		},
		reload:          reloadState{applicable: cfg.ReloadApplicable},
		compact:         compactState{applicable: cfg.CompactApplicable},
		loadUntracked:   cfg.LoadUntracked,
		activeThemeName: cfg.ActiveThemeName,
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

// Init initializes the model by loading changed files and the commit log
// in parallel. loadCommits returns nil when the feature is not applicable
// (e.g. --stdin, standalone file, working-tree review), so tea.Batch harmlessly
// drops it in those cases.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadFiles(), m.loadCommits())
}

// Update handles messages and updates the model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inConfirmDiscard {
			return m.handleConfirmDiscardKey(msg)
		}
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg)
	case filesLoadedMsg:
		return m.handleFilesLoaded(msg)
	case commitsLoadedMsg:
		return m.handleCommitsLoaded(msg)
	case fileLoadedMsg:
		return m.handleFileLoaded(msg)
	case blameLoadedMsg:
		return m.handleBlameLoaded(msg)
	case editorFinishedMsg:
		return m.handleEditorFinished(msg)
	}

	// forward other messages to textinput when annotating (e.g. cursor blink)
	if m.annot.annotating {
		var cmd tea.Cmd
		m.annot.input, cmd = m.annot.input.Update(msg)
		m.layout.viewport.SetContent(m.renderDiff()) // re-render so cursor blink updates are visible
		return m, cmd
	}

	// forward other messages to search textinput when searching (e.g. cursor blink)
	if m.search.active {
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// transient hints persist for exactly one render cycle; any key that reaches
	// this point dismisses the last hint before the new action runs.
	m.commits.hint = ""
	m.reload.hint = ""
	m.compact.hint = ""
	m.keys.hint = ""
	m.vim.hint = ""

	// pending-reload intercept: y confirms, any other key cancels
	if m.reload.pending {
		return m.handlePendingReload(msg)
	}

	// chord-second guard: a second key arriving while a chord is pending must
	// be consumed as the chord's second stage, regardless of any modal that
	// would otherwise eat it. Modal-entry paths clear chord state explicitly,
	// so coexistence should not occur in normal flow — this guard is
	// defense-in-depth.
	if m.keys.chordPending != "" {
		return m.handleChordSecond(msg.String())
	}

	if handled, model, cmd := m.handleModalKey(msg); handled {
		return model, cmd
	}

	// vim-motion interceptor: runs AFTER handleModalKey so modals consume keys
	// first (digits and letters belong to the modal's textinput when active),
	// and BEFORE keymap.Resolve so vim chords/counts preempt normal bindings.
	// propagate the interceptor's model on fall-through so state cleared inside
	// the interceptor (e.g., count dropped after an unrelated key like "5q")
	// is visible to the standard keymap path that runs next.
	if m.modes.vimMotion {
		model, cmd, handled := m.interceptVimMotion(msg)
		if handled {
			return model, cmd
		}
		m = model.(Model)
	}

	action := m.keymap.Resolve(msg.String())

	// chord-first guard: an unresolved key that is a registered chord leader
	// enters pending state. Load-time conflict resolution guarantees no key is
	// bound both as a standalone action and a chord prefix, so action is empty
	// whenever IsChordLeader returns true; the guard stays purely additive.
	if action == "" && m.keymap.IsChordLeader(msg.String()) {
		m.keys.chordPending = msg.String()
		m.keys.hint = "Pending: " + msg.String() + ", esc to cancel"
		return m, nil
	}

	return m.dispatchAction(action)
}

// dispatchAction routes a resolved keymap action through overlay-open, the
// global action switch, and the pane-specific nav fallback. It is the unified
// dispatch path shared by keymap-resolved single keys (handleKey) and by
// chord-resolved actions (handleChordSecond).
func (m Model) dispatchAction(action keymap.Action) (tea.Model, tea.Cmd) {
	if model, ok := m.handleOverlayOpen(action); ok {
		return model, nil
	}

	switch action {
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
	case keymap.ActionToggleCollapsed, keymap.ActionToggleCompact, keymap.ActionToggleWrap, keymap.ActionToggleTree,
		keymap.ActionToggleLineNums, keymap.ActionToggleBlame, keymap.ActionToggleWordDiff, keymap.ActionToggleUntracked:
		return m.handleViewToggle(action)
	case keymap.ActionNextHunk, keymap.ActionPrevHunk:
		return m.handleHunkNav(action == keymap.ActionNextHunk)
	case keymap.ActionReload:
		return m.handleReload()
	default: // remaining actions (navigation, search, etc.) handled by pane-specific handlers below
	}

	// pane-specific navigation
	switch m.layout.focus {
	case paneTree:
		return m.handleTreeAction(action)
	case paneDiff:
		return m.handleDiffAction(action)
	}
	return m, nil
}

func (m Model) handleOverlayOpen(action keymap.Action) (tea.Model, bool) {
	// clear pending input state on any overlay-opening action so a pending chord
	// or vim-motion count/leader never coexists with an active overlay. the
	// non-overlay default case short-circuits below without touching state.
	switch action {
	case keymap.ActionHelp:
		m.clearPendingInputState()
		m.overlay.OpenHelp(m.buildHelpSpec())
		return m, true
	case keymap.ActionAnnotList:
		m.clearPendingInputState()
		m.overlay.OpenAnnotList(m.buildAnnotListSpec())
		return m, true
	case keymap.ActionThemeSelect:
		m.clearPendingInputState()
		m.openThemeSelector()
		return m, true
	case keymap.ActionCommitInfo:
		m.clearPendingInputState()
		m.handleCommitInfo()
		return m, true
	default:
		return m, false
	}
}

// clearPendingInputState clears all pending key-dispatch state: chord-pending,
// chord hint, and vim-motion (count, leader, hint). Enforces the invariant
// that these fields never coexist with an active modal. Called by modal-entry
// paths (startSearch, startAnnotation, handleOverlayOpen) so a pending chord
// or vim count never survives into a modal session — the early chord-second
// guard in handleKey is defense-in-depth against accidental coexistence.
func (m *Model) clearPendingInputState() {
	m.keys.chordPending = ""
	m.keys.hint = ""
	m.vim = vimState{}
}

// handleCommitInfo opens the commit-info overlay when the feature is available
// in the current mode, otherwise sets a transient status-bar hint so the key
// press produces visible feedback instead of appearing inert. Reads from the
// cache populated eagerly by loadCommits at startup / reload; if the fetch has
// not yet landed (commits.loaded=false), shows a transient "loading commits…"
// hint instead of opening the overlay.
func (m *Model) handleCommitInfo() {
	if !m.commits.applicable || m.commits.source == nil {
		m.commits.hint = "no commits in this mode"
		return
	}
	if !m.commits.loaded {
		m.commits.hint = "loading commits…"
		return
	}
	m.overlay.OpenCommitInfo(overlay.CommitInfoSpec{
		Commits:    m.commits.list,
		Applicable: true,
		Truncated:  m.commits.truncated,
		Err:        m.commits.err,
	})
}

// applyReloadCleanup clears annotations and turns off the annotated-only
// filter if it was active. Value receiver matches handleReload and
// handlePendingReload; store and tree are reference-holding so mutations
// propagate without a pointer receiver.
func (m Model) applyReloadCleanup() {
	m.store.Clear()
	if m.tree.FilterActive() {
		m.tree.ToggleFilter(nil)
	}
}

func (m Model) handlePendingReload(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.reload.pending = false
	if msg.String() == "y" {
		m.applyReloadCleanup()
		m.reload.hint = "Reloaded"
		cmd := m.triggerReload()
		return m, cmd
	}
	m.reload.hint = "Reload canceled"
	return m, nil
}

// handleChordSecond dispatches the second-stage key of a pending chord. esc
// cancels silently, an unbound second key surfaces an "Unknown chord" hint,
// and a resolved action flows through dispatchAction so chord-bound actions
// reach the same handlers as keymap-resolved single keys. The local copy of
// Model carries the cleared chord state back to bubbletea.
func (m Model) handleChordSecond(keyStr string) (tea.Model, tea.Cmd) {
	prefix := m.keys.chordPending
	m.keys.chordPending = ""
	m.keys.hint = ""
	if keyStr == "esc" {
		return m, nil
	}
	action := m.keymap.ResolveChord(prefix, keyStr)
	if action == "" {
		m.keys.hint = "Unknown chord: " + prefix + ">" + keyStr
		return m, nil
	}
	return m.dispatchAction(action)
}

// handleReload handles the ActionReload key. In stdin mode the feature is
// unavailable. If no annotations exist, reloads immediately. If annotations
// exist, enters pending-confirmation state (waiting for y/other key in
// handlePendingReload).
func (m Model) handleReload() (tea.Model, tea.Cmd) {
	if !m.reload.applicable {
		m.reload.hint = "Reload not available in stdin mode"
		return m, nil
	}
	if m.store.Count() > 0 && !m.cfg.noStatusBar {
		m.reload.pending = true
		m.reload.hint = "Annotations will be dropped — press y to confirm, any other key to cancel"
		return m, nil
	}
	if m.store.Count() > 0 {
		m.applyReloadCleanup()
	}
	m.reload.hint = "Reloaded"
	cmd := m.triggerReload()
	return m, cmd
}

func (m Model) handleModalKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// annotation input mode takes priority
	if m.annot.annotating {
		model, cmd := m.handleAnnotateKey(msg)
		return true, model, cmd
	}

	// search input mode takes priority after annotation
	if m.search.active {
		model, cmd := m.handleSearchKey(msg)
		return true, model, cmd
	}

	// overlay popup dispatch (help, annotation list, theme selector)
	if m.overlay.Active() {
		action := m.keymap.Resolve(msg.String())
		out := m.overlay.HandleKey(msg, action)
		switch out.Kind {
		case overlay.OutcomeAnnotationChosen:
			model, cmd := m.jumpToAnnotationTarget(out.AnnotationTarget)
			return true, model, cmd
		case overlay.OutcomeThemePreview:
			m.previewThemeByName(out.ThemeChoice.Name)
		case overlay.OutcomeThemeConfirmed:
			m.confirmThemeByName(out.ThemeChoice.Name)
		case overlay.OutcomeThemeCanceled:
			m.cancelThemeSelect()
		case overlay.OutcomeClosed, overlay.OutcomeNone:
		}
		return true, m, nil
	}

	return false, m, nil
}

// treePaneHidden returns true when the tree/TOC pane should be hidden.
// true when user toggled it off, or in single-file mode without a markdown TOC.
func (m Model) treePaneHidden() bool {
	return m.layout.treeHidden || (m.file.singleFile && m.file.mdTOC == nil)
}

// isCursorLine returns true when the diff line at idx is the active cursor line.
func (m Model) isCursorLine(idx int) bool {
	return idx == m.nav.diffCursor && m.layout.focus == paneDiff && !m.annot.cursorOnAnnotation
}

// togglePane switches focus between tree and diff panes.
// only switches to diff pane when a file is loaded.
// no-op in single-file mode unless mdTOC is active (TOC uses paneTree slot).
func (m *Model) togglePane() {
	if m.treePaneHidden() {
		return
	}
	if m.layout.focus != paneTree {
		m.layout.focus = paneTree
		m.syncTOCCursorToActive()
		return
	}
	if m.file.name != "" {
		m.layout.focus = paneDiff
	}
}

// toggleTreePane hides or shows the tree/TOC pane.
// no-op in single-file mode without TOC (already no tree).
func (m *Model) toggleTreePane() {
	if m.file.singleFile && m.file.mdTOC == nil {
		return
	}
	m.layout.treeHidden = !m.layout.treeHidden
	if m.layout.treeHidden {
		m.layout.treeWidth = 0
		m.layout.focus = paneDiff
		m.layout.viewport.Width = m.layout.width - 2
	} else {
		m.layout.treeWidth = max(minTreeWidth, m.layout.width*m.cfg.treeWidthRatio/10)
		m.layout.viewport.Width = m.layout.width - m.layout.treeWidth - 4
	}
	m.layout.viewport.Height = m.paneHeight() - 1
	m.syncViewportToCursor()
}

// toggleLineNumbers toggles line number display on/off and recomputes gutter width.
func (m *Model) toggleLineNumbers() {
	if m.layout.focus != paneDiff || m.file.name == "" {
		return
	}
	m.modes.lineNumbers = !m.modes.lineNumbers
	if m.modes.lineNumbers {
		m.file.lineNumWidth = m.computeLineNumWidth()
	}
	m.syncViewportToCursor()
}

// computeLineNumWidth returns the digit width needed for line number columns.
// scans all file lines to find the maximum old or new line number.
func (m Model) computeLineNumWidth() int {
	maxNum := 0
	for _, dl := range m.file.lines {
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
	if m.layout.focus != paneDiff || m.file.name == "" || m.blamer == nil {
		return nil
	}
	m.modes.showBlame = !m.modes.showBlame
	if m.modes.showBlame {
		m.file.blameData = nil
		m.file.blameAuthorLen = 0
		return m.loadBlame(m.file.name)
	}
	m.file.blameData = nil
	m.file.blameAuthorLen = 0
	m.syncViewportToCursor()
	return nil
}

// toggleWordDiff toggles intra-line word-diff highlighting on/off.
// recomputeIntraRanges honors the new wordDiff state: it populates ranges
// when enabling and clears them when disabling.
// no-op when the diff pane is not focused or no file is loaded.
func (m *Model) toggleWordDiff() {
	if m.layout.focus != paneDiff || m.file.name == "" {
		return
	}
	m.modes.wordDiff = !m.modes.wordDiff
	m.recomputeIntraRanges()
	m.layout.viewport.SetContent(m.renderDiff())
}

// toggleUntracked toggles visibility of untracked files in the tree.
func (m *Model) toggleUntracked() tea.Cmd {
	m.modes.showUntracked = !m.modes.showUntracked
	m.filesLoadSeq++
	return m.loadFiles()
}

// toggleCompactMode switches between compact (small-context) and full-file
// diff mode and re-fetches the currently displayed file so the new context
// size takes effect. Other files re-fetch naturally on next navigation. When
// the feature is not applicable in the current mode (e.g. --stdin, --all-files,
// standalone FileReader), sets a transient status-bar hint and returns nil —
// mode stays unchanged and no re-fetch is issued.
func (m *Model) toggleCompactMode() tea.Cmd {
	if !m.compact.applicable {
		m.compact.hint = "compact not applicable in this mode"
		return nil
	}
	m.modes.compact = !m.modes.compact
	return m.reloadCurrentFile()
}

// handleViewToggle dispatches view mode toggle actions.
func (m Model) handleViewToggle(action keymap.Action) (tea.Model, tea.Cmd) {
	switch action {
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
	case keymap.ActionToggleCompact:
		cmd := m.toggleCompactMode()
		return m, cmd
	default:
		return m, nil
	}
	return m, nil
}

// toggleWrapMode toggles line wrapping on/off.
// resets horizontal scroll when enabling wrap and re-renders the diff.
func (m *Model) toggleWrapMode() {
	if m.layout.focus != paneDiff || m.file.name == "" {
		return
	}
	m.modes.wrap = !m.modes.wrap
	if m.modes.wrap {
		m.layout.scrollX = 0
	}
	m.syncViewportToCursor()
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.layout.width = msg.Width
	m.layout.height = msg.Height

	var diffWidth int
	if m.treePaneHidden() {
		m.layout.treeWidth = 0
		diffWidth = m.layout.width - 2 // diff pane borders only
	} else {
		// adjust tree width based on ratio (N out of 10 units);
		// applies to multi-file mode and single-file markdown with TOC
		m.layout.treeWidth = max(minTreeWidth, m.layout.width*m.cfg.treeWidthRatio/10)
		diffWidth = m.layout.width - m.layout.treeWidth - 4 // borders
	}
	diffHeight := m.paneHeight() - 1 // pane height minus diff header

	if !m.ready {
		m.layout.viewport = viewport.New(diffWidth, diffHeight)
		m.ready = true
	} else {
		m.layout.viewport.Width = diffWidth
		m.layout.viewport.Height = diffHeight
	}

	m.tree.EnsureVisible(m.treePageSize())

	if m.file.name != "" {
		m.syncViewportToCursor()
	}

	return m, nil
}

// resolveCommitLogSource picks the commit-log source for the model from an
// explicit ModelConfig.CommitLog field (taking precedence) or, when that is
// nil or a typed-nil interface, falls back to the renderer's optional
// diff.CommitLogger capability. Returns nil when neither path produces a
// usable source — the caller treats nil as "feature unavailable".
func resolveCommitLogSource(explicit commitLogSource, renderer Renderer) commitLogSource {
	if explicit != nil && !isNilValue(explicit) {
		return explicit
	}
	if cl, ok := renderer.(diff.CommitLogger); ok {
		return cl
	}
	return nil
}
