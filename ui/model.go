package ui

//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer
//go:generate moq -out mocks/syntax_highlighter.go -pkg mocks -skip-ensure -fmt goimports . SyntaxHighlighter

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
)

// Renderer provides methods to extract changed files and build full-file diff views.
type Renderer interface {
	ChangedFiles(ref string, staged bool) ([]string, error)
	FileDiff(ref, file string, staged bool) ([]diff.DiffLine, error)
}

// SyntaxHighlighter provides syntax highlighting for diff lines.
type SyntaxHighlighter interface {
	HighlightLines(filename string, lines []diff.DiffLine) []string
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

	ref            string
	staged         bool
	only           []string // filter to show only matching files
	workDir        string   // working directory for resolving absolute --only paths
	noStatusBar    bool
	focus          pane
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

	showHelp bool // true when help overlay is visible
	wrapMode bool // true when line wrapping is enabled

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
}

// fileLoadedMsg is sent when a file's diff has been loaded.
type fileLoadedMsg struct {
	file  string
	seq   uint64
	lines []diff.DiffLine
	err   error
}

// filesLoadedMsg is sent when the changed file list is loaded.
type filesLoadedMsg struct {
	files []string
	err   error
}

// ModelConfig holds configuration options for NewModel.
type ModelConfig struct {
	Ref              string
	Staged           bool
	TreeWidthRatio   int
	TabWidth         int      // number of spaces per tab character
	NoColors         bool     // disable all colors including syntax highlighting
	NoStatusBar      bool     // hide the status bar
	NoConfirmDiscard bool     // skip confirmation prompt when discarding annotations
	Wrap             bool     // enable line wrapping
	Collapsed        bool     // start in collapsed diff mode
	Only             []string // show only these files (match by exact path or path suffix)
	WorkDir          string   // working directory for resolving absolute --only paths
	Colors           Colors
}

// NewModel creates a new Model with the given renderer, store, highlighter and configuration.
func NewModel(renderer Renderer, store *annotation.Store, highlighter SyntaxHighlighter, cfg ModelConfig) Model {
	if cfg.TreeWidthRatio < 1 || cfg.TreeWidthRatio > 10 {
		cfg.TreeWidthRatio = 2
	}
	if cfg.TabWidth < 1 {
		cfg.TabWidth = 4
	}
	s := newStyles(cfg.Colors)
	if cfg.NoColors {
		s = plainStyles()
	}
	return Model{
		styles:           s,
		store:            store,
		renderer:         renderer,
		highlighter:      highlighter,
		ref:              cfg.Ref,
		staged:           cfg.Staged,
		only:             cfg.Only,
		workDir:          cfg.WorkDir,
		noStatusBar:      cfg.NoStatusBar,
		noConfirmDiscard: cfg.NoConfirmDiscard,
		wrapMode:         cfg.Wrap,
		collapsed:        collapsedState{enabled: cfg.Collapsed},
		focus:            paneTree,
		treeWidthRatio:   cfg.TreeWidthRatio,
		tabSpaces:        strings.Repeat(" ", cfg.TabWidth),
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

func (m Model) loadFiles() tea.Cmd {
	return func() tea.Msg {
		files, err := m.renderer.ChangedFiles(m.ref, m.staged)
		return filesLoadedMsg{files: files, err: err}
	}
}

func (m Model) loadFileDiff(file string) tea.Cmd {
	seq := m.loadSeq
	return func() tea.Msg {
		lines, err := m.renderer.FileDiff(m.ref, file, m.staged)
		return fileLoadedMsg{file: file, seq: seq, lines: lines, err: err}
	}
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
	// annotation input mode takes priority
	if m.annotating {
		return m.handleAnnotateKey(msg)
	}

	// search input mode takes priority after annotation
	if m.searching {
		return m.handleSearchKey(msg)
	}

	// help overlay: toggle with ?, dismiss with esc, block everything else
	if msg.String() == "?" || m.showHelp {
		return m.handleHelpKey(msg)
	}

	switch {
	case msg.Type == tea.KeyEsc:
		return m.handleEscKey()

	case msg.String() == "Q":
		return m.handleDiscardQuit()

	case msg.String() == "q":
		return m, tea.Quit

	case msg.String() == "tab":
		m.togglePane()
		return m, nil

	case msg.String() == "f":
		return m.handleFilterToggle()

	case msg.String() == "n" || msg.String() == "N":
		return m.handleFileOrSearchNav(msg.String())

	case msg.String() == "p":
		return m.handlePrevFile()

	case msg.String() == "enter":
		return m.handleEnterKey()

	case msg.String() == "A":
		return m.handleFileAnnotateKey()

	case msg.String() == "v":
		m.toggleCollapsedMode()
		return m, nil

	case msg.String() == "w":
		m.toggleWrapMode()
		return m, nil
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

// togglePane switches focus between tree and diff panes.
// only switches to diff pane when a file is loaded.
// no-op in single-file mode (tree pane is hidden).
func (m *Model) togglePane() {
	if m.singleFile {
		return
	}
	if m.focus != paneTree {
		m.focus = paneTree
		return
	}
	if m.currFile != "" {
		m.focus = paneDiff
	}
}

// handleSwitchToTree switches focus to tree pane from diff.
// no-op in single-file mode (tree pane is hidden).
func (m Model) handleSwitchToTree() (tea.Model, tea.Cmd) {
	if !m.singleFile {
		m.focus = paneTree
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

// loadSelectedIfChanged ensures the tree is visible and loads the selected file if it changed.
func (m Model) loadSelectedIfChanged() (tea.Model, tea.Cmd) {
	m.tree.ensureVisible(m.treePageSize())
	if f := m.tree.selectedFile(); f != "" && f != m.currFile {
		m.loadSeq++
		return m, m.loadFileDiff(f)
	}
	return m, nil
}

func (m Model) handleTreeNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "j" || msg.String() == "down":
		m.tree.moveDown()
	case msg.String() == "k" || msg.String() == "up":
		m.tree.moveUp()
	case msg.Type == tea.KeyPgDown || msg.String() == "ctrl+d":
		m.tree.pageDown(m.treePageSize())
	case msg.Type == tea.KeyPgUp || msg.String() == "ctrl+u":
		m.tree.pageUp(m.treePageSize())
	case msg.Type == tea.KeyHome:
		m.tree.moveToFirst()
	case msg.Type == tea.KeyEnd:
		m.tree.moveToLast()
	case msg.String() == "l" || msg.String() == "right":
		if m.currFile != "" {
			m.focus = paneDiff
		}
	}
	m.tree.ensureVisible(m.treePageSize())
	return m.loadSelectedIfChanged()
}

// treePageSize returns the number of visible lines in the tree pane.
func (m Model) treePageSize() int {
	return max(1, m.paneHeight())
}

// paneHeight returns the content height for panes (total minus borders and status bar).
func (m Model) paneHeight() int {
	h := m.height - 2 // borders
	if !m.noStatusBar {
		h-- // status bar
	}
	return max(1, h)
}

func (m Model) handleDiffNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "h":
		return m.handleSwitchToTree()
	case msg.String() == "left":
		m.handleHorizontalScroll(-1)
		return m, nil
	case msg.String() == "right":
		m.handleHorizontalScroll(1)
		return m, nil
	case msg.String() == "j" || msg.String() == "down":
		m.moveDiffCursorDown()
		m.syncViewportToCursor()
	case msg.String() == "k" || msg.String() == "up":
		m.moveDiffCursorUp()
		m.syncViewportToCursor()
	case msg.Type == tea.KeyPgDown || msg.String() == "ctrl+d":
		m.moveDiffCursorPageDown()
	case msg.Type == tea.KeyPgUp || msg.String() == "ctrl+u":
		m.moveDiffCursorPageUp()
	case msg.Type == tea.KeyHome:
		m.moveDiffCursorToStart()
	case msg.Type == tea.KeyEnd:
		m.moveDiffCursorToEnd()
	case msg.String() == "]":
		m.moveToNextHunk()
	case msg.String() == "[":
		m.moveToPrevHunk()
	case msg.String() == "a":
		cmd := m.startAnnotation()
		m.viewport.SetContent(m.renderDiff())
		return m, cmd
	case msg.String() == "d":
		cmd := m.deleteAnnotation()
		return m, cmd
	case msg.String() == ".":
		m.toggleHunkExpansion()
		return m, nil
	case msg.String() == "/":
		cmd := m.startSearch()
		return m, cmd
	}
	return m, nil
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	var diffWidth int
	if m.singleFile {
		m.treeWidth = 0
		diffWidth = m.width - 2 // diff pane borders only
	} else {
		// adjust tree width based on ratio (N out of 10 units)
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

// filterOnly returns only files matching the --only patterns, or all files if no filter is set.
// matches by exact path or path suffix (e.g. "model.go" matches "ui/model.go").
// when a pattern is an absolute path, it is also resolved relative to workDir for matching
// (e.g. "/repo/README.md" with workDir="/repo" matches "README.md").
func (m Model) filterOnly(files []string) []string {
	if len(m.only) == 0 {
		return files
	}
	var filtered []string
	for _, f := range files {
		for _, pattern := range m.only {
			if f == pattern || strings.HasSuffix(f, "/"+pattern) {
				filtered = append(filtered, f)
				break
			}
			// resolve absolute pattern relative to workDir for matching against repo-relative files
			if m.workDir != "" && filepath.IsAbs(pattern) {
				rel, err := filepath.Rel(m.workDir, pattern)
				if err == nil && !strings.HasPrefix(rel, "..") && (f == rel || strings.HasSuffix(f, "/"+rel)) {
					filtered = append(filtered, f)
					break
				}
			}
		}
	}
	return filtered
}

func (m Model) handleFilesLoaded(msg filesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.viewport.SetContent(fmt.Sprintf("error loading files: %v", msg.err))
		return m, nil
	}
	files := m.filterOnly(msg.files)
	if len(files) == 0 && len(m.only) > 0 {
		m.viewport.SetContent("no files match --only filter")
		return m, nil
	}
	m.tree = newFileTree(files)
	m.singleFile = len(files) == 1
	if m.singleFile {
		m.focus = paneDiff
		m.treeWidth = 0
		if m.ready {
			m.viewport.Width = m.width - 2
		}
	}

	// auto-select first file
	if f := m.tree.selectedFile(); f != "" {
		m.loadSeq++
		return m, m.loadFileDiff(f)
	}
	return m, nil
}

func (m Model) handleFileLoaded(msg fileLoadedMsg) (tea.Model, tea.Cmd) {
	// discard stale responses; only the latest load request (by sequence) is accepted
	if msg.seq != m.loadSeq {
		return m, nil
	}
	if msg.err != nil {
		m.viewport.SetContent(fmt.Sprintf("error loading diff: %v", msg.err))
		return m, nil
	}
	m.currFile = msg.file
	m.diffLines = msg.lines
	m.clearSearch()
	m.computeFileStats()
	m.highlightedLines = m.highlighter.HighlightLines(msg.file, msg.lines)
	m.cursorOnAnnotation = false
	m.scrollX = 0
	m.collapsed.expandedHunks = make(map[int]bool)
	m.skipInitialDividers()
	m.viewport.SetContent(m.renderDiff())
	m.viewport.GotoTop()
	return m, nil
}

// computeFileStats counts added and removed lines in the current diffLines.
func (m *Model) computeFileStats() {
	m.fileAdds, m.fileRemoves = 0, 0
	for _, dl := range m.diffLines {
		switch dl.ChangeType {
		case diff.ChangeAdd:
			m.fileAdds++
		case diff.ChangeRemove:
			m.fileRemoves++
		case diff.ChangeContext, diff.ChangeDivider:
			// not counted in stats
		}
	}
}

// fileStatsText returns the stats segment for the status bar.
// shows total line count for context-only files, or +adds/-removes for diffs.
func (m Model) fileStatsText() string {
	if m.fileAdds == 0 && m.fileRemoves == 0 && len(m.diffLines) > 0 {
		return fmt.Sprintf("%d lines", len(m.diffLines))
	}
	return fmt.Sprintf("+%d/-%d", m.fileAdds, m.fileRemoves)
}

// skipInitialDividers positions diffCursor on the first visible line.
// skips divider lines, and in collapsed mode also skips removed lines
// unless their hunk is expanded.
func (m *Model) skipInitialDividers() {
	m.diffCursor = 0
	hunks := m.findHunks()
	for i, dl := range m.diffLines {
		if dl.ChangeType == diff.ChangeDivider || m.isCollapsedHidden(i, hunks) {
			continue
		}
		m.diffCursor = i
		return
	}
}

// View renders the full TUI.
func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	ph := m.paneHeight()

	// diff pane title
	diffTitle := "no file selected"
	if m.currFile != "" {
		diffTitle = m.currFile
	}
	diffHeader := m.styles.DirEntry.Render(" " + diffTitle)
	diffContent := lipgloss.JoinVertical(lipgloss.Left, diffHeader, m.viewport.View())

	var mainView string
	if m.singleFile {
		// single-file mode: no tree pane, diff uses full width
		diffPane := m.styles.DiffPaneActive.
			Width(m.width - 2).
			Height(ph).
			Render(diffContent)
		mainView = diffPane
	} else {
		annotated := m.annotatedFiles()
		treeContent := m.tree.render(m.treeWidth, ph, annotated, m.styles)

		// apply pane borders based on focus
		treeStyle := m.styles.TreePane
		diffStyle := m.styles.DiffPane
		if m.focus == paneTree {
			treeStyle = m.styles.TreePaneActive
		} else {
			diffStyle = m.styles.DiffPaneActive
		}

		treePane := treeStyle.
			Width(m.treeWidth).
			Height(ph).
			Render(treeContent)

		diffPane := diffStyle.
			Width(m.width - m.treeWidth - 4).
			Height(ph).
			Render(diffContent)

		mainView = lipgloss.JoinHorizontal(lipgloss.Top, treePane, diffPane)
	}

	if m.showHelp {
		// overlay help popup on top of current content
		helpBox := m.helpOverlay()
		mainView = m.overlayCenter(mainView, helpBox)
	}

	if m.noStatusBar {
		return mainView
	}

	status := m.styles.StatusBar.Width(m.width).Render(m.statusBarText())
	return lipgloss.JoinVertical(lipgloss.Left, mainView, status)
}

// statusBarText returns context-sensitive status line content.
// shows search input (when typing), or filename, diff stats, hunk position,
// search match position, mode indicators, and right-aligned annotation count + help hint.
func (m Model) statusBarText() string {
	if m.searching {
		return m.searchBarText()
	}

	if m.inConfirmDiscard {
		return fmt.Sprintf("discard %d annotations? [y/n]", m.store.Count())
	}

	if m.annotating {
		return "[enter] save  [esc] cancel"
	}

	// build left-side segments
	var segments []string

	// filename and diff stats segments
	if m.currFile != "" {
		segments = append(segments, m.currFile, m.fileStatsText())
	}

	// hunk position (always shown in diff pane when there are hunks)
	if hs := m.hunkSegment(); hs != "" {
		segments = append(segments, hs)
	}

	// search match position
	if ss := m.searchSegment(); ss != "" {
		segments = append(segments, ss)
	}

	// build right-side segments
	var rightParts []string
	if cnt := m.store.Count(); cnt > 0 {
		suffix := "annotations"
		if cnt == 1 {
			suffix = "annotation"
		}
		rightParts = append(rightParts, fmt.Sprintf("%d %s", cnt, suffix))
	}
	rightParts = append(rightParts, m.statusModeIcons(), "? help")

	// build separator with muted foreground using raw ANSI (not lipgloss.Render)
	// to avoid full reset that would break the status bar background
	statusFg := m.styles.colors.Muted
	if m.styles.colors.StatusFg != "" {
		statusFg = m.styles.colors.StatusFg
	}
	sep := " " + m.ansiFg(m.styles.colors.Muted) + "|" + m.ansiFg(statusFg) + " "
	left := strings.Join(segments, sep)
	right := strings.Join(rightParts, sep)

	// truncate filename from left with … if status line is too wide
	minRight := lipgloss.Width(right) + 5 // 2 for status bar padding + 3 for separator
	available := max(m.width-minRight, 0)

	// graceful degradation: drop left segments when too narrow
	if lipgloss.Width(left) > available {
		// rebuild without search position
		segments = m.statusSegmentsNoSearch()
		left = strings.Join(segments, sep)
	}
	if lipgloss.Width(left) > available {
		// rebuild without hunk info
		segments = m.statusSegmentsMinimal()
		left = strings.Join(segments, sep)
	}
	if lipgloss.Width(left) > available && m.currFile != "" {
		// truncate filename from left, keeping end of path.
		// uses display-width measurement to handle wide characters (CJK, emoji)
		statsStr := m.fileStatsText()
		nameMax := max(available-lipgloss.Width(statsStr)-lipgloss.Width(sep), 4) // reserve separator between name and stats
		name := m.currFile
		if lipgloss.Width(name) > nameMax {
			budget := nameMax - 1 // reserve 1 cell for "…"
			runes := []rune(name)
			w, cutIdx := 0, len(runes)
			for i := len(runes) - 1; i >= 0; i-- {
				rw := runewidth.RuneWidth(runes[i])
				if w+rw > budget {
					break
				}
				w += rw
				cutIdx = i
			}
			name = "…" + string(runes[cutIdx:])
		}
		left = name + sep + statsStr
	}

	return m.joinStatusSections(left, right, sep)
}

// hunkSegment returns a formatted hunk position string for the status line.
// returns "hunk X/Y" when cursor is on a changed line, "N hunks"/"1 hunk" otherwise, or empty if not in diff pane.
func (m Model) hunkSegment() string {
	if m.focus != paneDiff {
		return ""
	}
	cur, total := m.currentHunk()
	if total == 0 {
		return ""
	}
	if cur > 0 {
		return fmt.Sprintf("hunk %d/%d", cur, total)
	}
	if total == 1 {
		return "1 hunk"
	}
	return fmt.Sprintf("%d hunks", total)
}

// joinStatusSections joins left and right status sections with padding and separators.
func (m Model) joinStatusSections(left, right, sep string) string {
	sepWidth := lipgloss.Width(sep)
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2 // 2 for status bar padding
	if left != "" && padding > sepWidth {
		return left + sep + strings.Repeat(" ", padding-sepWidth) + right
	}
	if padding > 0 {
		return left + strings.Repeat(" ", padding) + right
	}
	if left != "" {
		return left + sep + right
	}
	return right
}

// searchBarText returns the status bar content during search input mode.
func (m Model) searchBarText() string {
	return "/" + m.searchInput.Value()
}

// searchSegment returns a formatted search position string like "X/Y" for the status line.
// returns empty string when no search matches exist. shows 0/N when all matches are hidden
// in collapsed mode (e.g. matches only on removed lines).
func (m Model) searchSegment() string {
	if len(m.searchMatches) == 0 {
		return ""
	}
	pos := m.searchCursor + 1
	if m.collapsed.enabled && m.searchCursor < len(m.searchMatches) {
		hunks := m.findHunks()
		if m.isCollapsedHidden(m.searchMatches[m.searchCursor], hunks) {
			pos = 0
		}
	}
	return fmt.Sprintf("%d/%d", pos, len(m.searchMatches))
}

// ansiColor returns an ANSI 24-bit color escape sequence for a hex color.
// code 38 = foreground, 48 = background. uses raw ANSI instead of lipgloss.Render
// to avoid full reset that breaks outer backgrounds.
func (m Model) ansiColor(hex string, code int) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return ""
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return fmt.Sprintf("\033[%d;2;%d;%d;%dm", code, r, g, b)
}

// ansiFg returns an ANSI 24-bit foreground escape sequence for a hex color.
func (m Model) ansiFg(hex string) string { return m.ansiColor(hex, 38) }

// ansiBg returns an ANSI 24-bit background escape sequence for a hex color.
func (m Model) ansiBg(hex string) string { return m.ansiColor(hex, 48) }

// statusModeIcons returns combined mode indicator icons (▼ collapsed, ◉ filter, ↩ wrap, ≋ search).
// all icons are always shown; active modes use status foreground, inactive use muted color.
func (m Model) statusModeIcons() string {
	type indicator struct {
		icon   string
		active bool
	}
	indicators := []indicator{
		{"▼", m.collapsed.enabled},
		{"◉", m.tree.filter},
		{"↩", m.wrapMode},
		{"≋", len(m.searchMatches) > 0},
	}

	statusFg := m.styles.colors.Muted
	if m.styles.colors.StatusFg != "" {
		statusFg = m.styles.colors.StatusFg
	}
	mutedSeq := m.ansiFg(m.styles.colors.Muted)
	activeSeq := m.ansiFg(statusFg)

	var icons []string
	for _, ind := range indicators {
		if ind.active {
			icons = append(icons, activeSeq+ind.icon)
		} else {
			icons = append(icons, mutedSeq+ind.icon)
		}
	}
	return strings.Join(icons, " ") + activeSeq
}

// statusSegmentsNoSearch returns left segments without search position (for narrow terminals).
func (m Model) statusSegmentsNoSearch() []string {
	var segments []string
	if m.currFile != "" {
		segments = append(segments, m.currFile, m.fileStatsText())
	}
	if hs := m.hunkSegment(); hs != "" {
		segments = append(segments, hs)
	}
	return segments
}

// statusSegmentsMinimal returns left segments with only filename and stats.
func (m Model) statusSegmentsMinimal() []string {
	var segments []string
	if m.currFile != "" {
		segments = append(segments, m.currFile, m.fileStatsText())
	}
	return segments
}

// helpOverlay returns a bordered help popup with keybinding sections.
func (m Model) helpOverlay() string {
	help := "" +
		"Navigation\n" +
		"  tab          switch pane\n" +
		"  n / p        next / prev file (n = next match when searching)\n" +
		"  j / k        scroll down / up\n" +
		"  PgDn/PgUp    page down / up\n" +
		"  Ctrl+d/u     half-page down / up\n" +
		"  Home/End     top / bottom\n" +
		"  h / l        focus tree / diff pane\n" +
		"  \u2190 / \u2192        scroll left / right (diff)\n" +
		"  [ / ]        prev / next hunk\n" +
		"  enter        focus diff pane\n" +
		"\n" +
		"Search\n" +
		"  /            search in diff\n" +
		"  n            next match (overrides next file)\n" +
		"  N            prev match\n" +
		"\n" +
		"Annotations\n" +
		"  a / enter    annotate line (diff pane)\n" +
		"  A            annotate file\n" +
		"  d            delete annotation\n" +
		"\n" +
		"View\n" +
		"  v            toggle collapsed mode\n" +
		"  w            toggle word wrap\n" +
		"  .            expand/collapse hunk\n" +
		"  f            filter annotated files\n" +
		"\n" +
		"Quit\n" +
		"  q            quit\n" +
		"  Q            discard annotations & quit\n" +
		"  ? / esc      close help"

	border := lipgloss.NormalBorder()
	boxStyle := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(m.styles.colors.Accent)).
		Padding(1, 2)

	return boxStyle.Render(help)
}

// overlayCenter composites fg on top of bg, centered horizontally and vertically.
// uses ANSI-aware string cutting to preserve styling in both layers.
func (m Model) overlayCenter(bg, fg string) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	fgWidth := lipgloss.Width(fg)
	fgHeight := len(fgLines)
	bgHeight := len(bgLines)

	startY := (bgHeight - fgHeight) / 2
	startX := max((m.width-fgWidth)/2, 0)

	for i, fgLine := range fgLines {
		bgIdx := startY + i
		if bgIdx < 0 || bgIdx >= bgHeight {
			continue
		}
		bgLine := bgLines[bgIdx]
		// pad bg line to full width so right part is always available
		bgW := lipgloss.Width(bgLine)
		if bgW < m.width {
			bgLine += strings.Repeat(" ", m.width-bgW)
		}

		left := ansi.Cut(bgLine, 0, startX)
		right := ansi.Cut(bgLine, startX+fgWidth, m.width)
		bgLines[bgIdx] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

// handleDiscardQuit handles the Q key press for discard-and-quit.
func (m Model) handleDiscardQuit() (tea.Model, tea.Cmd) {
	if m.store.Count() == 0 || m.noConfirmDiscard || m.noStatusBar {
		m.discarded = true
		return m, tea.Quit
	}
	m.inConfirmDiscard = true
	return m, nil
}

// handleFileAnnotateKey starts file-level annotation from diff pane only.
func (m Model) handleFileAnnotateKey() (tea.Model, tea.Cmd) {
	if m.focus == paneDiff && m.currFile != "" {
		cmd := m.startFileAnnotation()
		m.viewport.SetContent(m.renderDiff())
		return m, cmd
	}
	return m, nil
}

// handleEscKey clears active search results on esc.
func (m Model) handleEscKey() (tea.Model, tea.Cmd) {
	if len(m.searchMatches) > 0 {
		m.clearSearch()
		m.viewport.SetContent(m.renderDiff())
	}
	return m, nil
}

// handleEnterKey handles enter key based on current pane focus.
func (m Model) handleEnterKey() (tea.Model, tea.Cmd) {
	switch m.focus {
	case paneTree:
		if m.currFile != "" {
			m.focus = paneDiff
		}
		return m, nil
	case paneDiff:
		cmd := m.startAnnotation()
		m.viewport.SetContent(m.renderDiff())
		return m, cmd
	}
	return m, nil
}

// handleHelpKey handles help overlay keys.
// ? toggles the overlay, esc closes it, all other keys are blocked while showing.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "?" {
		m.showHelp = !m.showHelp
		return m, nil
	}
	if msg.Type == tea.KeyEsc {
		m.showHelp = false
	}
	return m, nil
}

// handleConfirmDiscardKey handles keys during discard confirmation prompt.
func (m Model) handleConfirmDiscardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Q":
		m.discarded = true
		return m, tea.Quit
	case "n", "esc":
		m.inConfirmDiscard = false
		return m, nil
	}
	return m, nil
}

// handleFilterToggle toggles the annotated files filter.
// no-op in single-file mode (tree pane is hidden).
func (m Model) handleFilterToggle() (tea.Model, tea.Cmd) {
	if m.singleFile {
		return m, nil
	}
	annotated := m.annotatedFiles()
	if len(annotated) > 0 {
		m.tree.toggleFilter(annotated)
		m.tree.ensureVisible(m.treePageSize())
		return m.loadSelectedIfChanged()
	}
	return m, nil
}

// handlePrevFile navigates to previous file.
// no-op in single-file mode (tree pane is hidden).
func (m Model) handlePrevFile() (tea.Model, tea.Cmd) {
	if m.singleFile {
		return m, nil
	}
	m.tree.prevFile()
	return m.loadSelectedIfChanged()
}

// handleFileOrSearchNav handles n/N keys: navigates search matches when a search is active,
// otherwise n falls through to next-file navigation (no-op in single-file mode).
// N does nothing without search.
func (m Model) handleFileOrSearchNav(key string) (tea.Model, tea.Cmd) {
	if len(m.searchMatches) > 0 {
		if key == "n" {
			m.nextSearchMatch()
		} else {
			m.prevSearchMatch()
		}
		m.viewport.SetContent(m.renderDiff())
		return m, nil
	}
	if key == "n" && !m.singleFile {
		m.tree.nextFile()
		return m.loadSelectedIfChanged()
	}
	return m, nil
}

// annotatedFiles returns a set of files that have annotations.
func (m Model) annotatedFiles() map[string]bool {
	result := make(map[string]bool)
	for _, f := range m.store.Files() {
		result[f] = true
	}
	return result
}
