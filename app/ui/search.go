package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// searchHistoryMax bounds the retained per-session search-query history.
// when exceeded, oldest entries are dropped.
const searchHistoryMax = 50

// startSearch creates a search textinput and enters searching mode.
func (m *Model) startSearch() tea.Cmd {
	m.clearPendingInputState()
	ti := textinput.New()
	ti.Placeholder = "/"
	cmd := ti.Focus()
	ti.CharLimit = 200
	ti.Width = max(10, m.layout.width-m.layout.treeWidth-10)
	m.search.input = ti
	m.search.active = true
	m.search.historyIdx = len(m.search.history)
	return cmd
}

// submitSearch processes the search query and finds matches in diffLines.
// empty input clears the search. otherwise stores lowercase term, scans for
// case-insensitive matches, and jumps to the first match at or after the cursor.
func (m *Model) submitSearch() {
	query := m.search.input.Value()
	if strings.TrimSpace(query) == "" {
		m.clearSearch()
		m.search.active = false
		return
	}

	m.search.term = strings.ToLower(query)
	m.appendSearchHistory(query)
	m.search.matches = nil
	m.search.cursor = 0

	for i, dl := range m.file.lines {
		if strings.Contains(strings.ToLower(dl.Content), m.search.term) {
			m.search.matches = append(m.search.matches, i)
		}
	}

	m.search.active = false

	if len(m.search.matches) == 0 {
		return
	}

	idx := m.findFirstVisibleMatch(m.nav.diffCursor)
	if idx < 0 {
		return
	}
	m.search.cursor = idx
	m.nav.diffCursor = m.search.matches[idx]
	m.annot.cursorOnAnnotation = false
	m.syncTOCActiveSection()
	m.centerViewportOnCursor()
}

// nextSearchMatch advances to the next search match with wrap-around.
// in collapsed mode, hidden removed lines are skipped.
func (m *Model) nextSearchMatch() {
	if len(m.search.matches) == 0 {
		return
	}
	hunks := m.findHunks()
	start := m.search.cursor
	for {
		m.search.cursor = (m.search.cursor + 1) % len(m.search.matches)
		if !m.isCollapsedHidden(m.search.matches[m.search.cursor], hunks) {
			break
		}
		if m.search.cursor == start {
			return // all matches are hidden
		}
	}
	m.nav.diffCursor = m.search.matches[m.search.cursor]
	m.annot.cursorOnAnnotation = false
	m.centerViewportOnCursor()
}

// prevSearchMatch moves to the previous search match with wrap-around.
// in collapsed mode, hidden removed lines are skipped.
func (m *Model) prevSearchMatch() {
	if len(m.search.matches) == 0 {
		return
	}
	hunks := m.findHunks()
	start := m.search.cursor
	for {
		m.search.cursor--
		if m.search.cursor < 0 {
			m.search.cursor = len(m.search.matches) - 1
		}
		if !m.isCollapsedHidden(m.search.matches[m.search.cursor], hunks) {
			break
		}
		if m.search.cursor == start {
			return // all matches are hidden
		}
	}
	m.nav.diffCursor = m.search.matches[m.search.cursor]
	m.annot.cursorOnAnnotation = false
	m.centerViewportOnCursor()
}

// cancelSearch exits searching mode without submitting.
func (m *Model) cancelSearch() {
	m.search.active = false
}

// appendSearchHistory records query in the in-session history. consecutive
// duplicates are skipped (less-style dedup). when the cap is exceeded, the
// oldest entries are dropped. historyIdx is reset to the draft slot so the
// next recall starts from "no recall active".
func (m *Model) appendSearchHistory(query string) {
	if n := len(m.search.history); n == 0 || m.search.history[n-1] != query {
		m.search.history = append(m.search.history, query)
		if len(m.search.history) > searchHistoryMax {
			m.search.history = m.search.history[len(m.search.history)-searchHistoryMax:]
		}
	}
	m.search.historyIdx = len(m.search.history)
}

// recallHistory walks the in-session search history. direction -1 walks toward
// older queries, +1 walks toward newer. clamps to [0, len(history)]; the
// upper bound represents the "draft" slot (input cleared, no recall active).
// no-op when history is empty.
func (m *Model) recallHistory(direction int) {
	if len(m.search.history) == 0 {
		return
	}
	idx := max(m.search.historyIdx+direction, 0)
	idx = min(idx, len(m.search.history))
	if idx == len(m.search.history) {
		m.search.input.SetValue("")
	} else {
		m.search.input.SetValue(m.search.history[idx])
	}
	m.search.input.CursorEnd()
	m.search.historyIdx = idx
}

// realignSearchCursor updates searchCursor to the nearest visible match at or after diffCursor.
// called after adjustCursorIfHidden moves diffCursor so the [X/Y] display stays accurate
// and n/N navigation starts from the correct position.
func (m *Model) realignSearchCursor() {
	if len(m.search.matches) == 0 {
		return
	}
	if idx := m.findFirstVisibleMatch(m.nav.diffCursor); idx >= 0 {
		m.search.cursor = idx
	}
}

// findFirstVisibleMatch returns the searchMatches index of the first visible match
// at or after startIdx, with wrap-around. returns -1 if no visible match exists.
func (m *Model) findFirstVisibleMatch(startIdx int) int {
	hunks := m.findHunks()
	// scan forward from startIdx
	for i, idx := range m.search.matches {
		if idx >= startIdx && !m.isCollapsedHidden(idx, hunks) {
			return i
		}
	}
	// wrap: scan from beginning
	for i, idx := range m.search.matches {
		if !m.isCollapsedHidden(idx, hunks) {
			return i
		}
	}
	return -1
}

// clearSearch resets per-query search state (term, matches, cursor, matchSet).
// session-scoped history fields are intentionally preserved.
func (m *Model) clearSearch() {
	m.search.term = ""
	m.search.matches = nil
	m.search.cursor = 0
	m.search.matchSet = nil
}

// buildSearchMatchSet converts searchMatches slice into a map for O(1) lookup during rendering.
// returns nil when there are no matches. callers assign the result to m.search.matchSet,
// which is read by render sub-methods (renderDiffLine, renderCollapsedAddLine, renderDeletePlaceholder)
// on the same value-receiver copy — the field acts as a render-pass local shared via the copy.
func (m Model) buildSearchMatchSet() map[int]bool {
	if len(m.search.matches) == 0 {
		return nil
	}
	result := make(map[int]bool, len(m.search.matches))
	for _, idx := range m.search.matches {
		result[idx] = true
	}
	return result
}

// handleSearchKey handles key messages during search input mode.
func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.submitSearch()
		m.layout.viewport.SetContent(m.renderDiff()) // refresh viewport to clear/update highlights
		return m, nil
	case tea.KeyEsc:
		m.cancelSearch()
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		m.recallHistory(-1)
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		m.recallHistory(+1)
		return m, nil
	default:
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(msg)
		return m, cmd
	}
}
