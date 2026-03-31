package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
)

// pane identifies which pane has focus.
type pane int

const (
	paneTree pane = iota
	paneDiff

	defaultTreeWidth = 30
	minTreeWidth     = 20
)

// Model is the top-level bubbletea model for revdiff.
type Model struct {
	styles   styles
	tree     fileTree
	viewport viewport.Model
	store    *annotation.Store
	renderer diff.DiffRenderer

	ref        string
	staged     bool
	focus      pane
	width      int
	height     int
	treeWidth  int
	diffCursor int // index into diffLines for current cursor line

	diffLines     []diff.DiffLine // current file's parsed diff lines
	currFile      string          // currently displayed file
	pendingFile   string          // file currently being loaded (async request identity)
	loadSeq       uint64          // monotonic counter to identify the latest load request
	ready         bool            // true after first WindowSizeMsg
	annotating    bool            // true when annotation text input is active
	annotateInput textinput.Model // text input for annotations
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

// NewModel creates a new Model with the given renderer, store, ref, and staged flag.
func NewModel(renderer diff.DiffRenderer, store *annotation.Store, ref string, staged bool) Model {
	return Model{
		styles:    defaultStyles(),
		store:     store,
		renderer:  renderer,
		ref:       ref,
		staged:    staged,
		focus:     paneTree,
		treeWidth: defaultTreeWidth,
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
		annotated := m.annotatedFiles()
		m.tree.toggleFilter(annotated)
		return m, nil

	case msg.String() == "n":
		m.tree.nextFile()
		if f := m.tree.selectedFile(); f != "" && f != m.currFile {
			m.loadSeq++
			m.pendingFile = f
			return m, m.loadFileDiff(f)
		}
		return m, nil

	case msg.String() == "p":
		m.tree.prevFile()
		if f := m.tree.selectedFile(); f != "" && f != m.currFile {
			m.loadSeq++
			m.pendingFile = f
			return m, m.loadFileDiff(f)
		}
		return m, nil

	case msg.String() == "enter":
		if m.focus == paneTree {
			if f := m.tree.selectedFile(); f != "" {
				m.loadSeq++
				m.pendingFile = f
				return m, m.loadFileDiff(f)
			}
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

func (m Model) handleTreeNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "j" || msg.String() == "down":
		m.tree.moveDown()
	case msg.String() == "k" || msg.String() == "up":
		m.tree.moveUp()
	case msg.String() == "l" || msg.String() == "right":
		if m.currFile != "" {
			m.focus = paneDiff
		}
	}
	return m, nil
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

	// adjust tree width
	m.treeWidth = max(minTreeWidth, min(defaultTreeWidth, m.width/3))

	diffWidth := m.width - m.treeWidth - 4 // borders
	diffHeight := m.height - 4             // borders + status

	if !m.ready {
		m.viewport = viewport.New(diffWidth, diffHeight)
		m.ready = true
	} else {
		m.viewport.Width = diffWidth
		m.viewport.Height = diffHeight
	}

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
		m.pendingFile = f
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
	m.diffCursor = 0
	// skip divider lines at the start
	for i, dl := range m.diffLines {
		if dl.ChangeType != diff.ChangeDivider {
			m.diffCursor = i
			break
		}
	}
	m.viewport.SetContent(m.renderDiff())
	m.viewport.GotoTop()
	return m, nil
}

// View renders the full TUI.
func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	treeContent := m.tree.render(m.treeWidth, m.annotatedFiles(), m.styles)
	if summary := m.renderAnnotationSummary(m.treeWidth); summary != "" {
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
	var statusText string
	if m.annotating {
		statusText = "[enter] save  [esc] cancel"
	} else {
		switch m.focus {
		case paneTree:
			statusText = "[j/k] navigate  [enter] select  [l] diff  [tab] filter  [n/p] next/prev  [q] quit"
		case paneDiff:
			statusText = "[j/k] scroll  [h] files  [a] annotate  [d] delete  [n/p] next/prev  [q] quit"
		}
	}
	status := m.styles.StatusBar.Render(statusText)

	return lipgloss.JoinVertical(lipgloss.Left, mainView, status)
}

// annotatedFiles returns a set of files that have annotations.
func (m Model) annotatedFiles() map[string]bool {
	result := make(map[string]bool)
	for _, f := range m.store.Files() {
		result[f] = true
	}
	return result
}
