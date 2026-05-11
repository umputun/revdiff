package ui

import (
	"log"

	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/style"
)

// themePreviewSession holds app-side state for an active theme selector session.
// created on open, consumed on confirm/cancel, nil when selector is not active.
type themePreviewSession struct {
	origResolver styleResolver
	origRenderer styleRenderer
	origSGR      sgrProcessor
	origChroma   string
}

// openThemeSelector builds the theme list, saves original state, and opens the overlay.
func (m *Model) openThemeSelector() {
	entries, err := m.themes.Entries()
	if err != nil {
		log.Printf("[WARN] theme selector: %v", err)
		return
	}

	m.themePreview = &themePreviewSession{
		origResolver: m.resolver,
		origRenderer: m.renderer,
		origSGR:      m.sgr,
		origChroma:   m.highlighter.StyleName(),
	}

	items := make([]overlay.ThemeItem, len(entries))
	for i, e := range entries {
		items[i] = overlay.ThemeItem{
			Name:        e.Name,
			Local:       e.Local,
			AccentColor: e.AccentColor,
		}
	}

	m.overlay.OpenThemeSelect(overlay.ThemeSelectSpec{
		Items:      items,
		ActiveName: m.activeThemeName,
	})
}

// previewThemeByName looks up a theme by name via the catalog and applies it.
func (m *Model) previewThemeByName(name string) {
	if m.themePreview == nil {
		return
	}
	spec, ok := m.themes.Resolve(name)
	if !ok {
		return
	}
	m.applyTheme(spec)
}

// confirmThemeByName applies the named theme, persists to config, and clears the preview session.
func (m *Model) confirmThemeByName(name string) {
	if m.themePreview == nil {
		return
	}
	spec, ok := m.themes.Resolve(name)
	if ok {
		m.activeThemeName = name
		m.applyTheme(spec)
	}
	if err := m.themes.Persist(name); err != nil {
		log.Printf("[WARN] failed to persist theme %q: %v", name, err)
	}
	m.themePreview = nil
}

// cancelThemeSelect restores the original theme and clears the preview session.
// invalidates the annotation row cache because preview applied a theme via
// applyTheme (which re-populated the cache with preview-styled rows) — without
// invalidation here, cached rows would carry the preview theme's AnnotationInline
// bytes after the resolver is restored to the original.
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
	m.invalidateAnnotationRows()
	m.refreshDiff()
}

// applyTheme rebuilds styles from ThemeSpec and re-highlights the current file.
func (m *Model) applyTheme(spec ThemeSpec) {
	var res style.Resolver
	if m.cfg.noColors {
		res = style.PlainResolver()
	} else {
		res = style.NewResolver(spec.Colors)
	}
	m.resolver = res
	m.renderer = style.NewRenderer(res)
	m.sgr = style.SGR{}
	prevStyle := m.highlighter.StyleName()
	chromaChanged := false
	if spec.ChromaStyle != prevStyle {
		if m.highlighter.SetStyle(spec.ChromaStyle) {
			chromaChanged = true
		} else {
			log.Printf("[WARN] failed to apply chroma style %q, keeping %q", spec.ChromaStyle, prevStyle)
		}
	}
	m.invalidateAnnotationRows()
	if m.file.name != "" && len(m.file.lines) > 0 {
		if chromaChanged {
			m.file.highlighted = m.highlighter.HighlightLines(m.file.name, m.file.lines)
		}
		m.layout.viewport.SetContent(m.renderDiff())
	}
}

// refreshDiff re-highlights and re-renders the current diff if one is loaded.
func (m *Model) refreshDiff() {
	if m.file.name != "" && len(m.file.lines) > 0 {
		m.file.highlighted = m.highlighter.HighlightLines(m.file.name, m.file.lines)
		m.layout.viewport.SetContent(m.renderDiff())
	}
}
