package ui

//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
)

// Renderer provides methods to extract changed files and build full-file diff views.
type Renderer interface {
	ChangedFiles(ref string, staged bool) ([]string, error)
	FileDiff(ref, file string, staged bool) ([]diff.DiffLine, error)
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
	focus          pane
	width          int
	height         int
	treeWidth      int
	treeWidthRatio int // 1-10 units for file tree panel
	diffCursor     int // index into diffLines for current cursor line

	diffLines          []diff.DiffLine // current file's parsed diff lines
	currFile           string          // currently displayed file
	loadSeq            uint64          // monotonic counter to identify the latest load request
	ready              bool            // true after first WindowSizeMsg
	annotating         bool            // true when annotation text input is active
	fileAnnotating     bool            // true when annotating at file level (Line=0)
	cursorOnAnnotation bool            // true when cursor is on the annotation sub-line (not the diff line)
	annotateInput      textinput.Model // text input for annotations
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
	Ref            string
	Staged         bool
	TreeWidthRatio int
	Colors         Colors
}

// NewModel creates a new Model with the given renderer, store, and configuration.
func NewModel(renderer Renderer, store *annotation.Store, cfg ModelConfig) Model {
	if cfg.TreeWidthRatio < 1 || cfg.TreeWidthRatio > 10 {
		cfg.TreeWidthRatio = 3
	}
	return Model{
		styles:         newStyles(cfg.Colors),
		store:          store,
		renderer:       renderer,
		ref:            cfg.Ref,
		staged:         cfg.Staged,
		focus:          paneTree,
		treeWidthRatio: cfg.TreeWidthRatio,
	}
}

// Store returns the annotation store for reading results after quit.
func (m Model) Store() *annotation.Store {
	return m.store
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

	switch {
	case msg.String() == "q":
		return m, tea.Quit

	case msg.String() == "tab":
		// switch panes: tree <-> diff (only switch to diff if a file is loaded)
		if m.focus != paneTree {
			m.focus = paneTree
			return m, nil
		}
		if m.currFile != "" {
			m.focus = paneDiff
		}
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
		switch m.focus {
		case paneTree:
			if f := m.tree.selectedFile(); f != "" {
				m.loadSeq++
				return m, m.loadFileDiff(f)
			}
		case paneDiff:
			cmd := m.startAnnotation()
			m.viewport.SetContent(m.renderDiff())
			return m, cmd
		}
		return m, nil

	case msg.String() == "A":
		// file-level annotation only from diff pane to avoid annotating the wrong file
		// when tree selection differs from the currently displayed file.
		if m.focus == paneDiff && m.currFile != "" {
			cmd := m.startFileAnnotation()
			m.viewport.SetContent(m.renderDiff())
			return m, cmd
		}
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
	return m, nil
}

// treePageSize returns the number of visible lines in the tree pane,
// accounting for annotation summary lines when present.
func (m Model) treePageSize() int {
	size := m.height - 3 // content height matching Height(m.height-3) in View()
	if summary := m.renderAnnotationSummary(m.treeWidth); summary != "" {
		size -= strings.Count(summary, "\n") + 1
	}
	return max(1, size)
}

func (m Model) handleDiffNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "h" || msg.String() == "left":
		m.focus = paneTree
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
	}
	return m, nil
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// adjust tree width based on ratio (N out of 10 units)
	m.treeWidth = max(minTreeWidth, m.width*m.treeWidthRatio/10)

	diffWidth := m.width - m.treeWidth - 4 // borders
	diffHeight := m.height - 4             // borders + status

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
	m.cursorOnAnnotation = false
	m.skipInitialDividers()
	m.viewport.SetContent(m.renderDiff())
	m.viewport.GotoTop()
	return m, nil
}

// skipInitialDividers positions diffCursor on the first non-divider line.
func (m *Model) skipInitialDividers() {
	m.diffCursor = 0
	for i, dl := range m.diffLines {
		if dl.ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			break
		}
	}
}

// View renders the full TUI.
func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	treeHeight := m.height - 3 // content height matching treeStyle.Height(m.height-3)
	summary := m.renderAnnotationSummary(m.treeWidth)
	if summary != "" {
		treeHeight -= strings.Count(summary, "\n") + 1
	}
	treeHeight = max(1, treeHeight) // ensure at least one row for file list
	annotated := m.annotatedFiles()
	treeContent := m.tree.render(m.treeWidth, treeHeight, annotated, m.styles)
	if summary != "" {
		treeContent += "\n" + summary
	}

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
		Height(m.height - 3).
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
		Height(m.height - 3).
		Render(diffContent)

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, treePane, diffPane)

	// status bar with context-sensitive hints
	status := m.styles.StatusBar.Render(m.statusBarText(annotated))

	return lipgloss.JoinVertical(lipgloss.Left, mainView, status)
}

// statusBarText returns context-sensitive status bar hints.
func (m Model) statusBarText(annotated map[string]bool) string {
	if m.annotating {
		return "[enter] save  [esc] cancel"
	}

	filterHint := ""
	if len(annotated) > 0 {
		filterHint = "  [f] filter"
	}
	fileNoteHint := ""
	if m.currFile != "" {
		fileNoteHint = "  [A] file note"
	}

	switch m.focus {
	case paneTree:
		return "[j/k] navigate  [enter] select  [l/tab] diff" + filterHint + "  [n/p] next/prev  [q] quit"
	case paneDiff:
		deleteHint := ""
		if m.cursorLineHasAnnotation() {
			deleteHint = "  [d] delete"
		}
		hunkHint := ""
		if cur, total := m.currentHunk(); total > 0 {
			hunkHint = fmt.Sprintf("  [/] hunk %d/%d", cur, total)
		}
		return "[j/k] scroll  [h/tab] files  [enter/a] annotate" + deleteHint + hunkHint + filterHint + fileNoteHint + "  [n/p] next/prev  [q] quit"
	default:
		return ""
	}
}

// annotatedFiles returns a set of files that have annotations.
func (m Model) annotatedFiles() map[string]bool {
	result := make(map[string]bool)
	for _, f := range m.store.Files() {
		result[f] = true
	}
	return result
}
