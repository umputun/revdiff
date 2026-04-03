package ui

//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer
//go:generate moq -out mocks/syntax_highlighter.go -pkg mocks -skip-ensure -fmt goimports . SyntaxHighlighter

import (
	"fmt"
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

	discarded        bool // true when user chose to discard annotations and quit
	inConfirmDiscard bool // true when showing discard confirmation prompt
	noConfirmDiscard bool // skip confirmation prompt on discard quit
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
	TabWidth         int  // number of spaces per tab character
	NoColors         bool // disable all colors including syntax highlighting
	NoStatusBar      bool // hide the status bar
	NoConfirmDiscard bool // skip confirmation prompt when discarding annotations
	Wrap             bool // enable line wrapping
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
		noStatusBar:      cfg.NoStatusBar,
		noConfirmDiscard: cfg.NoConfirmDiscard,
		wrapMode:         cfg.Wrap,
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

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// annotation input mode takes priority
	if m.annotating {
		return m.handleAnnotateKey(msg)
	}

	// help overlay: toggle with ?, dismiss with esc, block everything else
	if msg.String() == "?" || m.showHelp {
		return m.handleHelpKey(msg)
	}

	switch {
	case msg.String() == "Q":
		return m.handleDiscardQuit()

	case msg.String() == "q":
		return m, tea.Quit

	case msg.String() == "tab":
		m.togglePane()
		return m, nil

	case msg.String() == "f":
		annotated := m.annotatedFiles()
		if len(annotated) > 0 {
			m.tree.toggleFilter(annotated)
			m.tree.ensureVisible(m.treePageSize())
			return m.loadSelectedIfChanged()
		}
		return m, nil

	case msg.String() == "n":
		m.tree.nextFile()
		return m.loadSelectedIfChanged()

	case msg.String() == "p":
		m.tree.prevFile()
		return m.loadSelectedIfChanged()

	case msg.String() == "enter":
		return m.handleEnterKey()

	case msg.String() == "A":
		// file-level annotation only from diff pane to avoid annotating the wrong file
		// when tree selection differs from the currently displayed file.
		if m.focus == paneDiff && m.currFile != "" {
			cmd := m.startFileAnnotation()
			m.viewport.SetContent(m.renderDiff())
			return m, cmd
		}
		return m, nil

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
func (m *Model) togglePane() {
	if m.focus != paneTree {
		m.focus = paneTree
		return
	}
	if m.currFile != "" {
		m.focus = paneDiff
	}
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
	m.viewport.SetContent(m.renderDiff())
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
		m.focus = paneTree
		return m, nil
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
	}
	return m, nil
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// adjust tree width based on ratio (N out of 10 units)
	m.treeWidth = max(minTreeWidth, m.width*m.treeWidthRatio/10)

	diffWidth := m.width - m.treeWidth - 4 // borders
	diffHeight := m.paneHeight() - 1       // pane height minus diff header

	if !m.ready {
		m.viewport = viewport.New(diffWidth, diffHeight)
		m.ready = true
	} else {
		m.viewport.Width = diffWidth
		m.viewport.Height = diffHeight
	}

	m.tree.ensureVisible(m.treePageSize())

	if m.currFile != "" {
		m.viewport.SetContent(m.renderDiff())
	}

	return m, nil
}

func (m Model) handleFilesLoaded(msg filesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.viewport.SetContent(fmt.Sprintf("error loading files: %v", msg.err))
		return m, nil
	}
	m.tree = newFileTree(msg.files)

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

	// diff pane title
	diffTitle := "no file selected"
	if m.currFile != "" {
		diffTitle = m.currFile
	}
	diffHeader := m.styles.DirEntry.Render(" " + diffTitle)
	diffContent := lipgloss.JoinVertical(lipgloss.Left, diffHeader, m.viewport.View())

	diffPane := diffStyle.
		Width(m.width - m.treeWidth - 4).
		Height(ph).
		Render(diffContent)

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, treePane, diffPane)

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
// shows filename, diff stats, hunk position, mode indicators, and right-aligned annotation count + help hint.
func (m Model) statusBarText() string {
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
		segments = append(segments, m.currFile, fmt.Sprintf("+%d/-%d", m.fileAdds, m.fileRemoves))
	}

	// hunk position (always shown in diff pane when there are hunks)
	if hs := m.hunkSegment(); hs != "" {
		segments = append(segments, hs)
	}

	// mode indicators (combined into one segment)
	if modeIcons := m.statusModeIcons(); modeIcons != "" {
		segments = append(segments, modeIcons)
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
	rightParts = append(rightParts, "? help")

	const sep = " | "
	left := strings.Join(segments, sep)
	right := strings.Join(rightParts, sep)

	// truncate filename from left with … if status line is too wide
	minRight := lipgloss.Width(right) + 5 // 2 for status bar padding + 3 for separator
	available := max(m.width-minRight, 0)

	// graceful degradation: drop segments from right to left when too narrow
	if lipgloss.Width(left) > available {
		// rebuild without mode icons first
		segments = m.statusSegmentsNoIcons()
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
		statsStr := fmt.Sprintf("+%d/-%d", m.fileAdds, m.fileRemoves)
		nameMax := max(available-lipgloss.Width(statsStr)-len(sep), 4) // reserve separator between name and stats
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

	// pad left to push right section to the end
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2 // 2 for status bar padding
	if padding > 0 {
		return left + strings.Repeat(" ", padding) + right
	}
	if left != "" {
		return left + sep + right
	}
	return right
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

// statusModeIcons returns combined mode indicator icons (▼ for collapsed, ◉ for filter, ↩ for wrap).
func (m Model) statusModeIcons() string {
	var icons []string
	if m.collapsed.enabled {
		icons = append(icons, "▼")
	}
	if m.tree.filter {
		icons = append(icons, "◉")
	}
	if m.wrapMode {
		icons = append(icons, "↩")
	}
	return strings.Join(icons, " ")
}

// statusSegmentsNoIcons returns left segments without mode indicators (▼ ◉).
func (m Model) statusSegmentsNoIcons() []string {
	var segments []string
	if m.currFile != "" {
		segments = append(segments, m.currFile, fmt.Sprintf("+%d/-%d", m.fileAdds, m.fileRemoves))
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
		segments = append(segments, m.currFile, fmt.Sprintf("+%d/-%d", m.fileAdds, m.fileRemoves))
	}
	return segments
}

// helpOverlay returns a bordered help popup with keybinding sections.
func (m Model) helpOverlay() string {
	help := "" +
		"Navigation\n" +
		"  tab          switch pane\n" +
		"  n / p        next / prev file\n" +
		"  j / k        scroll down / up\n" +
		"  PgDn/PgUp    page down / up\n" +
		"  Ctrl+d/u     half-page down / up\n" +
		"  Home/End     top / bottom\n" +
		"  h / l        focus tree / diff pane\n" +
		"  \u2190 / \u2192        scroll left / right (diff)\n" +
		"  [ / ]        prev / next hunk\n" +
		"  enter        focus diff pane\n" +
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

// annotatedFiles returns a set of files that have annotations.
func (m Model) annotatedFiles() map[string]bool {
	result := make(map[string]bool)
	for _, f := range m.store.Files() {
		result[f] = true
	}
	return result
}
