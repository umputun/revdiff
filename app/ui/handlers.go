package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
)

// helpLine holds a key-description pair for rendering help overlay sections.
type helpLine struct{ keys, desc string }

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

// helpColors returns the ANSI color sequences used in help overlay rendering.
// reset is fg-only to preserve background, header and key are fg sequences.
func (m Model) helpColors() (reset, header, key string) {
	return string(style.ResetFg), string(m.resolver.Color(style.ColorKeyAccentFg)), string(m.resolver.Color(style.ColorKeyAnnotationFg))
}

// helpOverlay returns a bordered help popup with keybinding sections arranged in two columns.
// sections and key bindings are rendered dynamically from the keymap.
func (m Model) helpOverlay() string {
	sections := m.keymap.HelpSections()

	// render each section into a block of lines
	type sectionBlock struct {
		lines []string
	}
	blocks := make([]sectionBlock, 0, len(sections))

	reset, headerColor, keyColor := m.helpColors()

	for _, sec := range sections {
		var block sectionBlock
		block.lines = append(block.lines, headerColor+sec.Name+reset)

		lines := make([]helpLine, 0, len(sec.Entries))
		maxW := 0
		for _, e := range sec.Entries {
			keys := m.formatKeysForHelp(e.Action)
			lines = append(lines, helpLine{keys, e.Description})
			if w := runewidth.StringWidth(keys); w > maxW {
				maxW = w
			}
		}
		for _, l := range lines {
			pad := max(maxW-runewidth.StringWidth(l.keys), 0)
			block.lines = append(block.lines, fmt.Sprintf("  %s%s%s%s  %s",
				keyColor, l.keys, reset, strings.Repeat(" ", pad), l.desc))
		}
		blocks = append(blocks, block)

		// add Markdown TOC section after Pane
		if sec.Name == keymap.SectionPane {
			var tocBuf strings.Builder
			m.writeTOCHelpSection(&tocBuf)
			tocLines := strings.Split(strings.TrimRight(tocBuf.String(), "\n"), "\n")
			blocks = append(blocks, sectionBlock{lines: tocLines})
		}
	}

	// count total lines (with blank line separators between sections)
	totalLines := 0
	for _, b := range blocks {
		totalLines += len(b.lines) + 1 // +1 for separator
	}

	// assign sections to left/right columns, keeping sections intact
	var leftBlocks, rightBlocks []sectionBlock
	leftLines := 0
	half := totalLines / 2
	for _, b := range blocks {
		blockSize := len(b.lines) + 1
		if leftLines < half {
			leftBlocks = append(leftBlocks, b)
			leftLines += blockSize
		} else {
			rightBlocks = append(rightBlocks, b)
		}
	}

	// render column from blocks
	renderColumn := func(colBlocks []sectionBlock) []string {
		var result []string
		for i, b := range colBlocks {
			if i > 0 {
				result = append(result, "")
			}
			result = append(result, b.lines...)
		}
		return result
	}

	left := renderColumn(leftBlocks)
	right := renderColumn(rightBlocks)

	// find max visible width of left column for padding (ANSI-aware)
	leftWidth := 0
	for _, line := range left {
		if w := lipgloss.Width(line); w > leftWidth {
			leftWidth = w
		}
	}

	gap := 4
	// join columns side by side
	maxRows := max(len(left), len(right))
	var buf strings.Builder
	for i := range maxRows {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		// pad left column to fixed width (ANSI-aware width)
		pad := max(leftWidth-lipgloss.Width(l), 0)
		buf.WriteString(l)
		buf.WriteString(strings.Repeat(" ", pad))

		if i < len(right) {
			buf.WriteString(strings.Repeat(" ", gap))
			buf.WriteString(right[i])
		}
		if i < maxRows-1 {
			buf.WriteString("\n")
		}
	}

	boxStyle := m.resolver.Style(style.StyleKeyHelpBox)

	return boxStyle.Render(buf.String())
}

// writeTOCHelpSection writes the Markdown TOC contextual help section.
// keys are resolved dynamically from the keymap.
func (m Model) writeTOCHelpSection(buf *strings.Builder) {
	// collect display keys for multiple actions combined
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

	lines := []helpLine{
		{mergedKeys(keymap.ActionTogglePane), "switch between TOC and diff"},
		{mergedKeys(keymap.ActionDown, keymap.ActionUp), "navigate TOC entries"},
		{mergedKeys(keymap.ActionNextItem, keymap.ActionPrevItem), "next / prev header"},
		{mergedKeys(keymap.ActionConfirm), "jump to header in diff"},
	}

	maxW := 0
	for _, l := range lines {
		if w := runewidth.StringWidth(l.keys); w > maxW {
			maxW = w
		}
	}

	reset, headerColor, keyColor := m.helpColors()

	buf.WriteString(headerColor + "Markdown TOC (single-file full-context mode)" + reset + "\n")
	for _, l := range lines {
		pad := max(maxW-runewidth.StringWidth(l.keys), 0)
		fmt.Fprintf(buf, "  %s%s%s%s  %s\n", keyColor, l.keys, reset, strings.Repeat(" ", pad), l.desc)
	}
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
	if m.focus != paneDiff || m.currFile == "" {
		return m, nil
	}
	cmd := m.startFileAnnotation()
	m.viewport.SetContent(m.renderDiff())
	return m, cmd
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
		if m.mdTOC != nil {
			if idx, ok := m.mdTOC.CurrentLineIdx(); ok {
				// jump to selected header in diff
				m.diffCursor = idx
				m.cursorOnAnnotation = false
				m.mdTOC.UpdateActiveSection(m.diffCursor)
				m.focus = paneDiff
				m.topAlignViewportOnCursor()
				return m, nil
			}
		}
		if m.currFile != "" {
			m.focus = paneDiff
		}
		return m, nil
	case paneDiff:
		var cmd tea.Cmd
		if m.cursorOnFileAnnotationLine() {
			cmd = m.startFileAnnotation()
		} else {
			cmd = m.startAnnotation()
		}
		m.viewport.SetContent(m.renderDiff())
		return m, cmd
	}
	return m, nil
}

// handleHelpKey handles help overlay keys.
// help action toggles the overlay, dismiss/esc closes it, all other keys are blocked while showing.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.keymap.Resolve(msg.String())
	if action == keymap.ActionHelp {
		m.showHelp = !m.showHelp
		return m, nil
	}
	if action == keymap.ActionDismiss || msg.Type == tea.KeyEsc {
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
		m.pendingAnnotJump = nil // clear pending annotation jump on manual navigation
		m.pendingHunkJump = nil  // clear pending hunk jump on manual navigation
		m.tree.ToggleFilter(annotated)
		m.tree.EnsureVisible(m.treePageSize())
		return m.loadSelectedIfChanged()
	}
	return m, nil
}

// handleMarkReviewed toggles the reviewed state of the focused file.
// tree focus uses the selected row; diff/TOC focus uses the displayed file.
func (m Model) handleMarkReviewed() (tea.Model, tea.Cmd) {
	file := m.currFile
	if m.focus == paneTree && m.mdTOC == nil {
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
	if len(m.searchMatches) > 0 {
		if forward {
			m.nextSearchMatch()
		} else {
			m.prevSearchMatch()
		}
		m.syncTOCActiveSection()
		m.viewport.SetContent(m.renderDiff())
		return m, nil
	}
	dir := 1
	if !forward {
		dir = -1
	}
	if m.singleFile && m.mdTOC != nil {
		return m.jumpTOCEntry(dir)
	}
	if !m.singleFile {
		m.pendingAnnotJump = nil // clear pending annotation jump on manual navigation
		m.pendingHunkJump = nil  // clear pending hunk jump on manual navigation
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
