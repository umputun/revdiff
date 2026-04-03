package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// startSearch creates a search textinput and enters searching mode.
func (m *Model) startSearch() tea.Cmd {
	ti := textinput.New()
	ti.Placeholder = "/"
	cmd := ti.Focus()
	ti.CharLimit = 200
	ti.Width = max(10, m.width-m.treeWidth-10)
	m.searchInput = ti
	m.searching = true
	return cmd
}

// submitSearch processes the search query and finds matches in diffLines.
// empty input clears the search. otherwise stores lowercase term, scans for
// case-insensitive matches, and jumps to the first match at or after the cursor.
func (m *Model) submitSearch() {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.clearSearch()
		m.searching = false
		return
	}

	m.searchTerm = strings.ToLower(query)
	m.searchMatches = nil
	m.searchCursor = 0

	for i, dl := range m.diffLines {
		if strings.Contains(strings.ToLower(dl.Content), m.searchTerm) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}

	m.searching = false

	if len(m.searchMatches) == 0 {
		return
	}

	// find first match at or after current cursor position
	for i, idx := range m.searchMatches {
		if idx < m.diffCursor {
			continue
		}
		m.searchCursor = i
		m.diffCursor = idx
		m.cursorOnAnnotation = false
		m.syncViewportToCursor()
		return
	}

	// wrap to first match if no match found after cursor
	m.searchCursor = 0
	m.diffCursor = m.searchMatches[0]
	m.cursorOnAnnotation = false
	m.syncViewportToCursor()
}

// nextSearchMatch advances to the next search match with wrap-around.
func (m *Model) nextSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchCursor = (m.searchCursor + 1) % len(m.searchMatches)
	m.diffCursor = m.searchMatches[m.searchCursor]
	m.cursorOnAnnotation = false
	m.syncViewportToCursor()
}

// prevSearchMatch moves to the previous search match with wrap-around.
func (m *Model) prevSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchCursor--
	if m.searchCursor < 0 {
		m.searchCursor = len(m.searchMatches) - 1
	}
	m.diffCursor = m.searchMatches[m.searchCursor]
	m.cursorOnAnnotation = false
	m.syncViewportToCursor()
}

// cancelSearch exits searching mode without submitting.
func (m *Model) cancelSearch() {
	m.searching = false
}

// clearSearch resets all search state.
func (m *Model) clearSearch() {
	m.searchTerm = ""
	m.searchMatches = nil
	m.searchCursor = 0
	m.searchMatchSet = nil
}

// buildSearchMatchSet converts searchMatches slice into a map for O(1) lookup during rendering.
func (m *Model) buildSearchMatchSet() {
	if len(m.searchMatches) == 0 {
		m.searchMatchSet = nil
		return
	}
	m.searchMatchSet = make(map[int]bool, len(m.searchMatches))
	for _, idx := range m.searchMatches {
		m.searchMatchSet[idx] = true
	}
}

// handleSearchKey handles key messages during search input mode.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.submitSearch()
		return m, nil
	case tea.KeyEsc:
		m.cancelSearch()
		return m, nil
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
}
