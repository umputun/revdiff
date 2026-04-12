package ui

import (
	"fmt"
	"log"

	"github.com/umputun/revdiff/app/theme"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/style"
)

// themeEntry represents a single entry in the theme list with parsed theme data.
type themeEntry struct {
	name  string
	local bool
	theme theme.Theme
}

// themePreviewSession holds app-side state for an active theme selector session.
// created on open, consumed on confirm/cancel, nil when selector is not active.
type themePreviewSession struct {
	entries      []themeEntry
	origResolver styleResolver
	origRenderer styleRenderer
	origSGR      sgrProcessor
	origChroma   string
}

// openThemeSelector builds the theme list, saves original state, and opens the overlay.
func (m *Model) openThemeSelector() {
	entries, err := m.buildThemeEntries()
	if err != nil {
		log.Printf("[WARN] theme selector: %v", err)
		return
	}

	m.themePreview = &themePreviewSession{
		entries:      entries,
		origResolver: m.resolver,
		origRenderer: m.renderer,
		origSGR:      m.sgr,
		origChroma:   m.highlighter.StyleName(),
	}

	items := make([]overlay.ThemeItem, len(entries))
	for i, e := range entries {
		items[i] = overlay.ThemeItem{
			Name:        e.name,
			Local:       e.local,
			AccentColor: e.theme.Colors["color-accent"],
		}
	}

	m.overlay.OpenThemeSelect(overlay.ThemeSelectSpec{
		Items:      items,
		ActiveName: m.activeThemeName,
	})
}

// buildThemeEntries merges gallery + local themes into a sorted list.
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

// previewThemeByName looks up a theme by name in the preview session and applies it.
func (m *Model) previewThemeByName(name string) {
	if m.themePreview == nil {
		return
	}
	for _, e := range m.themePreview.entries {
		if e.name == name {
			m.applyTheme(e.theme)
			return
		}
	}
}

// confirmThemeByName applies the named theme, persists to config, and clears the preview session.
func (m *Model) confirmThemeByName(name string) {
	if m.themePreview == nil {
		return
	}
	for _, e := range m.themePreview.entries {
		if e.name == name {
			m.activeThemeName = name
			m.applyTheme(e.theme)
			break
		}
	}
	if m.configPath != "" {
		if err := patchConfigTheme(m.configPath, name); err != nil {
			log.Printf("[WARN] failed to persist theme %q to %s: %v", name, m.configPath, err)
		}
	}
	m.themePreview = nil
}

// cancelThemeSelect restores the original theme and clears the preview session.
func (m *Model) cancelThemeSelect() {
	if m.themePreview == nil {
		return
	}
	m.resolver = m.themePreview.origResolver
	m.renderer = m.themePreview.origRenderer
	m.sgr = m.themePreview.origSGR
	if !m.highlighter.SetStyle(m.themePreview.origChroma) {
		log.Printf("[WARN] failed to restore chroma style %q", m.themePreview.origChroma)
	}
	m.themePreview = nil
	m.refreshDiff()
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
		m.viewport.SetContent(m.renderDiff())
	}
}

// refreshDiff re-highlights and re-renders the current diff if one is loaded.
func (m *Model) refreshDiff() {
	if m.currFile != "" && len(m.diffLines) > 0 {
		m.highlightedLines = m.highlighter.HighlightLines(m.currFile, m.diffLines)
		m.viewport.SetContent(m.renderDiff())
	}
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
