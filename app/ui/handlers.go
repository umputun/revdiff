package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

// helpKeyDisplay maps bubbletea key names to user-friendly display names.
var helpKeyDisplay = map[string]string{
	"pgdown": "PgDn",
	"pgup":   "PgUp",
	"left":   "←",
	"right":  "→",
	"home":   "Home",
	"end":    "End",
	"enter":  "Enter",
	"esc":    "Esc",
	"tab":    "Tab",
	"up":     "↑",
	"down":   "↓",
	" ":      "Space",
}

// displayKeyName returns a user-friendly display name for a bubbletea key.
func (m Model) displayKeyName(key string) string {
	if d, ok := helpKeyDisplay[key]; ok {
		return d
	}
	if strings.HasPrefix(key, "ctrl+") {
		return "Ctrl+" + key[5:]
	}
	return key
}

// formatKeysForHelp returns a formatted key string for a given action using display names.
func (m Model) formatKeysForHelp(action keymap.Action) string {
	keys := m.keymap.KeysFor(action)
	display := make([]string, len(keys))
	for i, k := range keys {
		display[i] = m.displayKeyName(k)
	}
	return strings.Join(display, " / ")
}

// buildHelpSpec builds an overlay.HelpSpec from the keymap's help sections,
// converting raw key names to display names and inserting the TOC section.
// When the vim-motion preset is active, appends a synthetic "Vim motion"
// section listing the 8 preset bindings (which have no entries in the base
// keymap since they're only reachable through the interceptor).
func (m Model) buildHelpSpec() overlay.HelpSpec {
	sections := m.keymap.HelpSections()
	var result []overlay.HelpSection
	for _, sec := range sections {
		var entries []overlay.HelpEntry
		for _, e := range sec.Entries {
			entries = append(entries, overlay.HelpEntry{
				Keys:        m.formatKeysForHelp(e.Action),
				Description: e.Description,
			})
		}
		if sec.Name == "Search" {
			entries = append(entries,
				overlay.HelpEntry{Keys: "↑ / Ctrl+P", Description: "recall previous search query (in search prompt)"},
				overlay.HelpEntry{Keys: "↓ / Ctrl+N", Description: "recall next search query / clear (in search prompt)"},
			)
		}
		result = append(result, overlay.HelpSection{Title: sec.Name, Entries: entries})

		if sec.Name == keymap.SectionPane {
			result = append(result, m.buildTOCHelpSection())
		}
	}
	if m.modes.vimMotion {
		result = append(result, m.buildVimMotionHelpSection())
	}
	return overlay.HelpSpec{Sections: result}
}

// buildVimMotionHelpSection returns the synthetic help section for the
// vim-motion preset. Keys are hardcoded because these bindings are not part
// of the configurable keymap — they are driven by the interceptor in
// vimmotion.go, not by defaultBindings.
func (m Model) buildVimMotionHelpSection() overlay.HelpSection {
	return overlay.HelpSection{
		Title: "Vim motion",
		Entries: []overlay.HelpEntry{
			{Keys: "N j / N k", Description: "move cursor N lines down/up"},
			{Keys: "gg", Description: "jump to first line"},
			{Keys: "G / N G", Description: "jump to last line / goto line N"},
			{Keys: "zz", Description: "center viewport on cursor"},
			{Keys: "zt", Description: "align viewport top"},
			{Keys: "zb", Description: "align viewport bottom"},
			{Keys: "ZZ", Description: "quit"},
			{Keys: "ZQ", Description: "discard and quit"},
		},
	}
}

// buildTOCHelpSection returns the Markdown TOC contextual help section.
func (m Model) buildTOCHelpSection() overlay.HelpSection {
	mergedKeys := func(actions ...keymap.Action) string {
		var all []string
		seen := map[string]bool{}
		for _, a := range actions {
			for _, k := range m.keymap.KeysFor(a) {
				dk := m.displayKeyName(k)
				if !seen[dk] {
					all = append(all, dk)
					seen[dk] = true
				}
			}
		}
		return strings.Join(all, " / ")
	}

	return overlay.HelpSection{
		Title: "Markdown TOC (single-file full-context mode)",
		Entries: []overlay.HelpEntry{
			{Keys: mergedKeys(keymap.ActionTogglePane), Description: "switch between TOC and diff"},
			{Keys: mergedKeys(keymap.ActionDown, keymap.ActionUp), Description: "navigate TOC entries"},
			{Keys: mergedKeys(keymap.ActionNextItem, keymap.ActionPrevItem), Description: "next / prev header"},
			{Keys: mergedKeys(keymap.ActionConfirm), Description: "jump to header in diff"},
		},
	}
}

// handleDiscardQuit handles the Q key press for discard-and-quit.
func (m Model) handleDiscardQuit() (tea.Model, tea.Cmd) {
	if m.store.Count() == 0 || m.cfg.noConfirmDiscard || m.cfg.noStatusBar {
		m.discarded = true
		return m, tea.Quit
	}
	m.inConfirmDiscard = true
	return m, nil
}

// handleFileAnnotateKey starts file-level annotation from diff pane only.
func (m Model) handleFileAnnotateKey() (tea.Model, tea.Cmd) {
	if m.layout.focus != paneDiff || m.file.name == "" {
		return m, nil
	}
	cmd := m.startFileAnnotation()
	m.layout.viewport.SetContent(m.renderDiff())
	return m, cmd
}

// handleEscKey clears active search results on esc.
func (m Model) handleEscKey() (tea.Model, tea.Cmd) {
	if len(m.search.matches) > 0 {
		m.clearSearch()
		m.layout.viewport.SetContent(m.renderDiff())
	}
	return m, nil
}

// handleEnterKey handles enter key based on current pane focus.
func (m Model) handleEnterKey() (tea.Model, tea.Cmd) {
	switch m.layout.focus {
	case paneTree:
		if m.file.mdTOC != nil {
			if idx, ok := m.file.mdTOC.CurrentLineIdx(); ok {
				// jump to selected header in diff
				m.nav.diffCursor = idx
				m.annot.cursorOnAnnotation = false
				m.file.mdTOC.UpdateActiveSection(m.nav.diffCursor)
				m.layout.focus = paneDiff
				m.topAlignViewportOnCursor()
				return m, nil
			}
		}
		if m.file.name != "" {
			m.layout.focus = paneDiff
		}
		return m, nil
	case paneDiff:
		var cmd tea.Cmd
		if m.cursorOnFileAnnotationLine() {
			cmd = m.startFileAnnotation()
		} else {
			cmd = m.startAnnotation()
		}
		m.layout.viewport.SetContent(m.renderDiff())
		return m, cmd
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
	if m.file.singleFile {
		return m, nil
	}
	annotated := m.annotatedFiles()
	if len(annotated) > 0 || m.tree.FilterActive() {
		m.pendingAnnotJump = nil    // clear pending annotation jump on manual navigation
		m.nav.pendingHunkJump = nil // clear pending hunk jump on manual navigation
		m.tree.ToggleFilter(annotated)
		m.tree.EnsureVisible(m.treePageSize())
		return m.loadSelectedIfChanged()
	}
	return m, nil
}

// handleMarkReviewed toggles the reviewed state of the focused file.
// tree focus uses the selected row; diff/TOC focus uses the displayed file.
func (m Model) handleMarkReviewed() (tea.Model, tea.Cmd) {
	file := m.file.name
	if m.layout.focus == paneTree && m.file.mdTOC == nil {
		file = m.tree.SelectedFile()
	}
	if file == "" {
		file = m.tree.SelectedFile()
	}
	m.tree.ToggleReviewed(file)
	return m, nil
}

// handleFileOrSearchNav handles next/prev item navigation: navigates search matches when a search
// is active, otherwise navigates files or TOC entries (no-op in single-file mode without TOC).
func (m Model) handleFileOrSearchNav(forward bool) (tea.Model, tea.Cmd) {
	if len(m.search.matches) > 0 {
		if forward {
			m.nextSearchMatch()
		} else {
			m.prevSearchMatch()
		}
		m.syncTOCActiveSection()
		m.layout.viewport.SetContent(m.renderDiff())
		return m, nil
	}
	dir := 1
	if !forward {
		dir = -1
	}
	if m.file.singleFile && m.file.mdTOC != nil {
		return m.jumpTOCEntry(dir)
	}
	if !m.file.singleFile {
		m.pendingAnnotJump = nil    // clear pending annotation jump on manual navigation
		m.nav.pendingHunkJump = nil // clear pending hunk jump on manual navigation
		if forward {
			m.tree.StepFile(sidepane.DirectionNext)
		} else {
			m.tree.StepFile(sidepane.DirectionPrev)
		}
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
