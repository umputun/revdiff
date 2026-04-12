package ui

import (
	"fmt"
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/theme"
	"github.com/umputun/revdiff/app/ui/style"
)

const (
	themePopupMaxWidth    = 50 // maximum popup width
	themePopupMinWidth    = 20 // minimum popup width
	themePopupMargin      = 10 // horizontal margin from terminal edges
	themePopupBorderPad   = 4  // border (2) + padding (2) for content width
	themePopupChromeLines = 10 // border + padding + filter + separator lines
)

// themeSelectState holds all state for the theme selector overlay.
type themeSelectState struct {
	active       bool          // true when overlay is visible
	all          []themeEntry  // all available themes (unfiltered)
	entries      []themeEntry  // filtered view into all
	cursor       int           // selected item in filtered list
	offset       int           // scroll offset
	filter       string        // current filter text
	origResolver styleResolver // saved resolver for cancel/restore
	origRenderer styleRenderer // saved renderer for cancel/restore
	origSGR      sgrProcessor  // saved SGR for cancel/restore
	origChroma   string        // saved chroma style name for cancel/restore
}

// themeEntry represents a single entry in the theme selector list.
type themeEntry struct {
	name  string
	local bool        // true for user-created themes not in the gallery
	theme theme.Theme // parsed theme data
}

// openThemeSelector builds the theme list and activates the overlay.
func (m *Model) openThemeSelector() {
	entries, err := m.buildThemeEntries()
	if err != nil {
		log.Printf("[WARN] theme selector: %v", err)
		return
	}
	m.themeSel.all = entries
	m.themeSel.filter = ""
	m.themeSel.active = true

	// save current state so Esc can restore
	m.themeSel.origResolver = m.resolver
	m.themeSel.origRenderer = m.renderer
	m.themeSel.origSGR = m.sgr
	m.themeSel.origChroma = m.highlighter.StyleName()

	m.applyThemeFilter()

	// position cursor on the active theme (match by name)
	for i, e := range m.themeSel.entries {
		if e.name == m.activeThemeName {
			m.themeSel.cursor = i
			maxVis := m.themeSelectMaxVisible()
			if i >= maxVis {
				m.themeSel.offset = i - maxVis + 1
			}
			break
		}
	}
}

// buildThemeEntries merges gallery + local themes into a sorted list.
// Order: default theme first, then local, then bundled, then other gallery — sorted within each group.
func (m *Model) buildThemeEntries() ([]themeEntry, error) {
	infos, err := theme.ListOrdered(m.themesDir)
	if err != nil {
		return nil, fmt.Errorf("listing themes: %w", err)
	}

	entries := make([]themeEntry, 0, len(infos))
	for _, info := range infos {
		loaded, loadErr := theme.Load(info.Name, m.themesDir)
		if loadErr != nil {
			continue
		}
		entries = append(entries, themeEntry{
			name:  info.Name,
			local: info.Local,
			theme: loaded,
		})
	}
	return entries, nil
}

// applyThemeFilter updates themeSel.entries from themeSel.all based on the current filter.
func (m *Model) applyThemeFilter() {
	if m.themeSel.filter == "" {
		m.themeSel.entries = m.themeSel.all
		m.themeSel.cursor = 0
		m.themeSel.offset = 0
		return
	}
	lower := strings.ToLower(m.themeSel.filter)
	filtered := make([]themeEntry, 0, len(m.themeSel.all))
	for _, e := range m.themeSel.all {
		if strings.Contains(strings.ToLower(e.name), lower) {
			filtered = append(filtered, e)
		}
	}
	m.themeSel.entries = filtered
	m.themeSel.cursor = 0
	m.themeSel.offset = 0
}

// themeSelectOverlay renders the theme selector popup.
func (m Model) themeSelectOverlay() string {
	popupWidth := max(min(m.width-themePopupMargin, themePopupMaxWidth), themePopupMinWidth)
	maxVisible := m.themeSelectMaxVisible()
	contentWidth := popupWidth - themePopupBorderPad

	var parts []string

	// filter input line
	filterLine := m.renderThemeFilter()
	parts = append(parts, filterLine, "")

	if len(m.themeSel.entries) == 0 {
		muted := m.resolver.Color(style.ColorKeyMutedFg)
		parts = append(parts, string(muted)+"  no matches"+string(style.ResetFg))
	} else {
		for i := m.themeSel.offset; i < len(m.themeSel.entries) && i < m.themeSel.offset+maxVisible; i++ {
			e := m.themeSel.entries[i]
			line := m.formatThemeEntry(e, contentWidth, i == m.themeSel.cursor)
			parts = append(parts, line)
		}
	}

	content := strings.Join(parts, "\n")

	total := len(m.themeSel.all)
	showing := len(m.themeSel.entries)
	title := fmt.Sprintf(" themes (%d) ", total)
	if m.themeSel.filter != "" {
		title = fmt.Sprintf(" themes (%d/%d) ", showing, total)
	}

	boxStyle := m.resolver.Style(style.StyleKeyThemeSelectBox).Width(popupWidth)

	box := boxStyle.Render(content)
	box = m.injectBorderTitle(box, title, popupWidth)
	return box
}

// renderThemeFilter renders the filter input line, aligned with list item names.
func (m Model) renderThemeFilter() string {
	accent := m.resolver.Color(style.ColorKeyAccentFg)
	muted := m.resolver.Color(style.ColorKeyMutedFg)

	if m.themeSel.filter == "" {
		return "  " + string(muted) + "type to filter..." + string(style.ResetFg)
	}
	return "  " + m.themeSel.filter + string(accent) + "│" + string(style.ResetFg)
}

// formatThemeEntry formats a single theme entry with accent color swatch.
func (m Model) formatThemeEntry(e themeEntry, width int, selected bool) string {
	accentColor := e.theme.Colors["color-accent"]

	// swatch: ■ colored with theme's accent for gallery, ◇ for local
	var swatch string
	var resetFg string
	if selected {
		resetFg = string(m.resolver.Color(style.ColorKeySelectedFg))
	}
	switch {
	case e.local:
		swatch = swatchText(string(m.resolver.Color(style.ColorKeyMutedFg)), "◇", resetFg)
	case accentColor != "":
		swatch = swatchText(style.AnsiFg(accentColor), "■", resetFg)
	default:
		swatch = "■"
	}

	// "  ■ name" or "> ■ name" when selected
	nameMaxWidth := width - 6 // "  " + swatch + " " + padding
	name := e.name
	if runewidth.StringWidth(name) > nameMaxWidth {
		name = runewidth.Truncate(name, nameMaxWidth, "…")
	}

	fileSelected := m.resolver.Style(style.StyleKeyFileSelected)
	if selected {
		line := "> " + swatch + " " + name
		styled := fileSelected.Render(line)
		w := lipgloss.Width(styled)
		if w < width {
			styled += fileSelected.Render(strings.Repeat(" ", width-w))
		}
		return styled
	}

	return "  " + swatch + " " + name
}

// handleThemeSelectKey handles keys when the theme selector is visible.
// Uses fzf-style input: printable chars go to filter, arrows navigate.
func (m Model) handleThemeSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.keymap.Resolve(msg.String())
	if action == keymap.ActionThemeSelect {
		m.cancelThemeSelect()
		return m, nil
	}

	switch msg.Type { //nolint:exhaustive // only handling relevant key types
	case tea.KeyEnter:
		return m.confirmThemeSelect()

	case tea.KeyEsc:
		if m.themeSel.filter != "" {
			// first Esc clears filter
			m.themeSel.filter = ""
			m.applyThemeFilter()
			m.previewTheme()
			return m, nil
		}
		m.cancelThemeSelect()
		return m, nil

	case tea.KeyBackspace:
		if m.themeSel.filter != "" {
			runes := []rune(m.themeSel.filter)
			m.themeSel.filter = string(runes[:len(runes)-1])
			m.applyThemeFilter()
			m.previewTheme()
		}
		return m, nil

	case tea.KeyUp:
		if m.themeSel.cursor > 0 {
			m.themeSel.cursor--
			if m.themeSel.cursor < m.themeSel.offset {
				m.themeSel.offset = m.themeSel.cursor
			}
			m.previewTheme()
		}
		return m, nil

	case tea.KeyDown:
		if m.themeSel.cursor < len(m.themeSel.entries)-1 {
			m.themeSel.cursor++
			maxVisible := m.themeSelectMaxVisible()
			if m.themeSel.cursor >= m.themeSel.offset+maxVisible {
				m.themeSel.offset = m.themeSel.cursor - maxVisible + 1
			}
			m.previewTheme()
		}
		return m, nil

	case tea.KeyRunes:
		// printable characters go to filter
		m.themeSel.filter += string(msg.Runes)
		m.applyThemeFilter()
		m.previewTheme()
		return m, nil
	}

	return m, nil
}

// previewTheme applies the currently highlighted theme without persisting.
func (m *Model) previewTheme() {
	if len(m.themeSel.entries) == 0 || m.themeSel.cursor >= len(m.themeSel.entries) {
		return
	}
	e := m.themeSel.entries[m.themeSel.cursor]
	m.applyTheme(e.theme)
}

// applyTheme rebuilds styles and re-highlights the current file.
func (m *Model) applyTheme(th theme.Theme) {
	sc := colorsFromTheme(th)
	var res style.Resolver
	if m.noColors {
		res = style.PlainResolver()
	} else {
		res = style.NewResolver(sc)
	}
	m.resolver = res
	m.renderer = style.NewRenderer(res)
	m.sgr = style.SGR{}
	prevStyle := m.highlighter.StyleName()
	chromaChanged := false
	if th.ChromaStyle != prevStyle {
		if m.highlighter.SetStyle(th.ChromaStyle) {
			chromaChanged = true
		} else {
			log.Printf("[WARN] failed to apply chroma style %q, keeping %q", th.ChromaStyle, prevStyle)
		}
	}
	if m.currFile != "" && len(m.diffLines) > 0 {
		if chromaChanged {
			m.highlightedLines = m.highlighter.HighlightLines(m.currFile, m.diffLines)
		}
		m.viewport.SetContent(m.renderDiff()) // always re-render since colors changed
	}
}

// confirmThemeSelect applies the selected theme and persists to config.
func (m Model) confirmThemeSelect() (tea.Model, tea.Cmd) {
	if len(m.themeSel.entries) == 0 {
		m.themeSel.filter = ""
		m.cancelThemeSelect()
		return m, nil
	}
	e := m.themeSel.entries[m.themeSel.cursor]
	m.themeSel.active = false
	m.activeThemeName = e.name
	m.applyTheme(e.theme)

	// persist to config file
	if m.configPath != "" {
		if err := patchConfigTheme(m.configPath, e.name); err != nil {
			log.Printf("[WARN] failed to persist theme %q to %s: %v", e.name, m.configPath, err)
		}
	}
	return m, nil
}

// cancelThemeSelect restores the original theme and closes the overlay.
func (m *Model) cancelThemeSelect() {
	m.themeSel.active = false
	m.themeSel.filter = ""
	m.resolver = m.themeSel.origResolver
	m.renderer = m.themeSel.origRenderer
	m.sgr = m.themeSel.origSGR
	if !m.highlighter.SetStyle(m.themeSel.origChroma) {
		log.Printf("[WARN] failed to restore chroma style %q", m.themeSel.origChroma)
	}
	m.refreshDiff()
}

// refreshDiff re-highlights and re-renders the current diff if one is loaded.
func (m *Model) refreshDiff() {
	if m.currFile != "" && len(m.diffLines) > 0 {
		m.highlightedLines = m.highlighter.HighlightLines(m.currFile, m.diffLines)
		m.viewport.SetContent(m.renderDiff())
	}
}

// themeSelectMaxVisible returns the maximum number of visible items in the theme selector.
// accounts for filter input line (2 lines: input + blank separator).
func (m Model) themeSelectMaxVisible() int {
	available := m.height - themePopupChromeLines
	return max(min(len(m.themeSel.entries), available), 1)
}

// colorsFromTheme converts a theme.Theme to a style.Colors struct.
func colorsFromTheme(th theme.Theme) style.Colors {
	return style.Colors{
		Accent:       th.Colors["color-accent"],
		Border:       th.Colors["color-border"],
		Normal:       th.Colors["color-normal"],
		Muted:        th.Colors["color-muted"],
		SelectedFg:   th.Colors["color-selected-fg"],
		SelectedBg:   th.Colors["color-selected-bg"],
		Annotation:   th.Colors["color-annotation"],
		CursorFg:     th.Colors["color-cursor-fg"],
		CursorBg:     th.Colors["color-cursor-bg"],
		AddFg:        th.Colors["color-add-fg"],
		AddBg:        th.Colors["color-add-bg"],
		RemoveFg:     th.Colors["color-remove-fg"],
		RemoveBg:     th.Colors["color-remove-bg"],
		WordAddBg:    th.Colors["color-word-add-bg"],
		WordRemoveBg: th.Colors["color-word-remove-bg"],
		ModifyFg:     th.Colors["color-modify-fg"],
		ModifyBg:     th.Colors["color-modify-bg"],
		TreeBg:       th.Colors["color-tree-bg"],
		DiffBg:       th.Colors["color-diff-bg"],
		StatusFg:     th.Colors["color-status-fg"],
		StatusBg:     th.Colors["color-status-bg"],
		SearchFg:     th.Colors["color-search-fg"],
		SearchBg:     th.Colors["color-search-bg"],
	}
}

// previewThemeByName looks up a theme by name and applies it as a preview.
// stub for Task 10 migration — will use themePreviewSession entries.
func (m *Model) previewThemeByName(name string) {
	th, err := theme.Load(m.themesDir, name)
	if err != nil {
		return
	}
	m.applyTheme(th)
}

// confirmThemeByName applies the named theme and persists to config.
// stub for Task 10 migration — will use themePreviewSession entries.
func (m *Model) confirmThemeByName(name string) {
	th, err := theme.Load(m.themesDir, name)
	if err != nil {
		return
	}
	m.activeThemeName = name
	m.applyTheme(th)
	if m.configPath != "" {
		if err := patchConfigTheme(m.configPath, name); err != nil {
			log.Printf("[WARN] failed to persist theme %q to %s: %v", name, m.configPath, err)
		}
	}
}

// swatchText renders text with the given ANSI fg sequence and resets afterward.
// if fg is empty, returns text unchanged. if resetFg is empty, uses style.ResetFg.
func swatchText(fg, text, resetFg string) string {
	if fg == "" {
		return text
	}
	reset := string(style.ResetFg)
	if resetFg != "" {
		reset = resetFg
	}
	return fg + text + reset
}
