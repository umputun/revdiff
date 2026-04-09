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
	query := m.searchInput.Value()
	if strings.TrimSpace(query) == "" {
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

	idx := m.findFirstVisibleMatch(m.diffCursor)
	if idx < 0 {
		return
	}
	m.searchCursor = idx
	m.diffCursor = m.searchMatches[idx]
	m.cursorOnAnnotation = false
	m.syncTOCActiveSection()
	m.centerViewportOnCursor()
}

// nextSearchMatch advances to the next search match with wrap-around.
// in collapsed mode, hidden removed lines are skipped.
func (m *Model) nextSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	hunks := m.findHunks()
	start := m.searchCursor
	for {
		m.searchCursor = (m.searchCursor + 1) % len(m.searchMatches)
		if !m.isCollapsedHidden(m.searchMatches[m.searchCursor], hunks) {
			break
		}
		if m.searchCursor == start {
			return // all matches are hidden
		}
	}
	m.diffCursor = m.searchMatches[m.searchCursor]
	m.cursorOnAnnotation = false
	m.centerViewportOnCursor()
}

// prevSearchMatch moves to the previous search match with wrap-around.
// in collapsed mode, hidden removed lines are skipped.
func (m *Model) prevSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	hunks := m.findHunks()
	start := m.searchCursor
	for {
		m.searchCursor--
		if m.searchCursor < 0 {
			m.searchCursor = len(m.searchMatches) - 1
		}
		if !m.isCollapsedHidden(m.searchMatches[m.searchCursor], hunks) {
			break
		}
		if m.searchCursor == start {
			return // all matches are hidden
		}
	}
	m.diffCursor = m.searchMatches[m.searchCursor]
	m.cursorOnAnnotation = false
	m.centerViewportOnCursor()
}

// cancelSearch exits searching mode without submitting.
func (m *Model) cancelSearch() {
	m.searching = false
}

// realignSearchCursor updates searchCursor to the nearest visible match at or after diffCursor.
// called after adjustCursorIfHidden moves diffCursor so the [X/Y] display stays accurate
// and n/N navigation starts from the correct position.
func (m *Model) realignSearchCursor() {
	if len(m.searchMatches) == 0 {
		return
	}
	if idx := m.findFirstVisibleMatch(m.diffCursor); idx >= 0 {
		m.searchCursor = idx
	}
}

// findFirstVisibleMatch returns the searchMatches index of the first visible match
// at or after startIdx, with wrap-around. returns -1 if no visible match exists.
func (m *Model) findFirstVisibleMatch(startIdx int) int {
	hunks := m.findHunks()
	// scan forward from startIdx
	for i, idx := range m.searchMatches {
		if idx >= startIdx && !m.isCollapsedHidden(idx, hunks) {
			return i
		}
	}
	// wrap: scan from beginning
	for i, idx := range m.searchMatches {
		if !m.isCollapsedHidden(idx, hunks) {
			return i
		}
	}
	return -1
}

// clearSearch resets all search state.
func (m *Model) clearSearch() {
	m.searchTerm = ""
	m.searchMatches = nil
	m.searchCursor = 0
	m.searchMatchSet = nil
}

// buildSearchMatchSet converts searchMatches slice into a map for O(1) lookup during rendering.
// returns nil when there are no matches. callers assign the result to m.searchMatchSet,
// which is read by render sub-methods (renderDiffLine, renderCollapsedAddLine, renderDeletePlaceholder)
// on the same value-receiver copy — the field acts as a render-pass local shared via the copy.
func (m Model) buildSearchMatchSet() map[int]bool {
	if len(m.searchMatches) == 0 {
		return nil
	}
	result := make(map[int]bool, len(m.searchMatches))
	for _, idx := range m.searchMatches {
		result[idx] = true
	}
	return result
}

// handleSearchKey handles key messages during search input mode.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.submitSearch()
		m.viewport.SetContent(m.renderDiff()) // refresh viewport to clear/update highlights
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
