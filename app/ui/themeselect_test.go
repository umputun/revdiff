package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/theme"
	"github.com/umputun/revdiff/app/ui/mocks"
)

func TestColorsFromTheme(t *testing.T) {
	th := theme.Theme{
		ChromaStyle: "dracula",
		Colors: map[string]string{
			"color-accent":      "#bd93f9",
			"color-border":      "#6272a4",
			"color-normal":      "#f8f8f2",
			"color-muted":       "#6272a4",
			"color-selected-fg": "#f8f8f2",
			"color-selected-bg": "#44475a",
			"color-annotation":  "#f1fa8c",
			"color-cursor-fg":   "#282a36",
			"color-cursor-bg":   "#f8f8f2",
			"color-add-fg":      "#50fa7b",
			"color-add-bg":      "#2a4a2a",
			"color-remove-fg":   "#ff5555",
			"color-remove-bg":   "#4a2a2a",
			"color-modify-fg":   "#ffb86c",
			"color-modify-bg":   "#3a3a2a",
			"color-tree-bg":     "#21222c",
			"color-diff-bg":     "#282a36",
			"color-status-fg":   "#f8f8f2",
			"color-status-bg":   "#44475a",
			"color-search-fg":   "#282a36",
			"color-search-bg":   "#f1fa8c",
		},
	}

	colors := colorsFromTheme(th)
	assert.Equal(t, "#bd93f9", colors.Accent)
	assert.Equal(t, "#6272a4", colors.Border)
	assert.Equal(t, "#f8f8f2", colors.Normal)
	assert.Equal(t, "#50fa7b", colors.AddFg)
	assert.Equal(t, "#282a36", colors.DiffBg)
}

func TestBuildThemeEntries(t *testing.T) {
	m := Model{themesDir: t.TempDir()}
	entries, err := m.buildThemeEntries()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 5, "should have at least gallery themes")

	// verify bundled themes are present and not marked local
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.name] = e.local
	}
	assert.False(t, found["dracula"], "dracula should not be local")
	assert.False(t, found["nord"], "nord should not be local")
}

func TestBuildThemeEntries_ordering(t *testing.T) {
	m := Model{themesDir: t.TempDir()}
	entries, err := m.buildThemeEntries()
	require.NoError(t, err)

	// default theme should be first
	require.NotEmpty(t, entries)
	assert.Equal(t, theme.DefaultThemeName, entries[0].name, "default theme should be first")

	// remaining entries (after default) should be sorted within their groups
	// just verify default is first and all gallery entries are present
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	assert.Contains(t, names, "dracula")
	assert.Contains(t, names, "nord")
	assert.Contains(t, names, "catppuccin-latte")
}

func TestBuildThemeEntries_prefersInstalledThemeOverride(t *testing.T) {
	themesDir := t.TempDir()
	custom, err := theme.GalleryTheme("dracula")
	require.NoError(t, err)
	custom.ChromaStyle = "monokai"
	custom.Colors["color-accent"] = "#010203"

	var buf bytes.Buffer
	require.NoError(t, theme.Dump(custom, &buf))
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "dracula"), buf.Bytes(), 0o600))

	m := Model{themesDir: themesDir}
	entries, err := m.buildThemeEntries()
	require.NoError(t, err)

	var dracula themeEntry
	found := false
	for _, e := range entries {
		if e.name == "dracula" {
			dracula = e
			found = true
			break
		}
	}
	require.True(t, found, "dracula entry should be present")
	assert.False(t, dracula.local, "gallery themes with local overrides should keep their gallery classification")
	assert.Equal(t, "#010203", dracula.theme.Colors["color-accent"])
	assert.Equal(t, "monokai", dracula.theme.ChromaStyle)
}

func TestThemeSelectOverlay_renders(t *testing.T) {
	m := testModel([]string{"file.go"}, nil)
	m.width = 80
	m.height = 24
	m.themesDir = t.TempDir()
	m.openThemeSelector()

	overlay := m.themeSelectOverlay()
	assert.Contains(t, overlay, "themes")
	assert.Contains(t, overlay, "dracula")
	assert.Contains(t, overlay, "type to filter")
}

func TestApplyThemeFilter(t *testing.T) {
	m := Model{themesDir: t.TempDir()}
	entries, err := m.buildThemeEntries()
	require.NoError(t, err)
	m.themeSel.all = entries

	m.themeSel.filter = "drac"
	m.applyThemeFilter()
	require.Len(t, m.themeSel.entries, 1)
	assert.Equal(t, "dracula", m.themeSel.entries[0].name)

	m.themeSel.filter = "xyz-nonexistent"
	m.applyThemeFilter()
	assert.Empty(t, m.themeSel.entries)

	m.themeSel.filter = ""
	m.applyThemeFilter()
	assert.Equal(t, m.themeSel.all, m.themeSel.entries)
}

func TestApplyThemeFilter_caseInsensitive(t *testing.T) {
	m := Model{themesDir: t.TempDir()}
	entries, err := m.buildThemeEntries()
	require.NoError(t, err)
	m.themeSel.all = entries

	m.themeSel.filter = "NORD"
	m.applyThemeFilter()
	require.Len(t, m.themeSel.entries, 1)
	assert.Equal(t, "nord", m.themeSel.entries[0].name)
}

func TestHexColorToRGB(t *testing.T) {
	assert.Equal(t, "189;147;249", hexColorToRGB("#bd93f9"))
	assert.Equal(t, "0;0;0", hexColorToRGB("#000000"))
	assert.Equal(t, "255;255;255", hexColorToRGB("#ffffff"))
	assert.Equal(t, "255;255;255", hexColorToRGB("invalid"))
}

func TestFormatThemeEntry_accentSwatch(t *testing.T) {
	m := testModel(nil, nil)
	e := themeEntry{
		name:  "dracula",
		local: false,
		theme: theme.Theme{Colors: map[string]string{"color-accent": "#bd93f9"}},
	}
	line := m.formatThemeEntry(e, 40, false)
	assert.Contains(t, line, "■")
	assert.Contains(t, line, "dracula")
}

func TestFormatThemeEntry_localDiamond(t *testing.T) {
	m := testModel(nil, nil)
	e := themeEntry{
		name:  "custom",
		local: true,
		theme: theme.Theme{Colors: map[string]string{"color-accent": "#ff0000"}},
	}
	line := m.formatThemeEntry(e, 40, false)
	assert.Contains(t, line, "◇")
	assert.Contains(t, line, "custom")
}

func TestFormatThemeEntry_selectedRestoresSelectedForegroundAfterSwatch(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = newStyles(Colors{
		Normal:     "#d0d0d0",
		Muted:      "#6272a4",
		SelectedFg: "#f8f8f2",
		SelectedBg: "#44475a",
	})

	e := themeEntry{
		name:  "dracula",
		local: false,
		theme: theme.Theme{Colors: map[string]string{"color-accent": "#bd93f9"}},
	}

	line := m.formatThemeEntry(e, 40, true)
	assert.Contains(t, line, "\033[38;2;189;147;249m■\033[38;2;248;248;242m dracula")
}

func TestConfirmThemeSelect_noMatchesRestoresOriginalTheme(t *testing.T) {
	currentStyle := "orig-style"
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc: func(style string) bool {
			currentStyle = style
			return true
		},
		StyleNameFunc: func() string { return currentStyle },
	}

	m := NewModel(renderer, annotation.NewStore(), highlighter, ModelConfig{TreeWidthRatio: 3})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()

	origAccent := m.styles.colors.Accent
	m.openThemeSelector()

	draculaIdx := -1
	for i, e := range m.themeSel.entries {
		if e.name == "dracula" {
			draculaIdx = i
			break
		}
	}
	require.NotEqual(t, -1, draculaIdx, "dracula entry should be present")

	m.themeSel.cursor = draculaIdx
	m.previewTheme()
	require.NotEqual(t, origAccent, m.styles.colors.Accent, "preview should apply the highlighted theme")
	require.Equal(t, "dracula", currentStyle)

	m.themeSel.filter = "no-match"
	m.applyThemeFilter()

	result, _ := m.confirmThemeSelect()
	updated := result.(Model)

	assert.False(t, updated.themeSel.active)
	assert.Empty(t, updated.themeSel.filter)
	assert.Equal(t, origAccent, updated.styles.colors.Accent)
	assert.Equal(t, "orig-style", currentStyle)
}

func TestThemeSelectPreviewAndConfirmPreserveNoColors(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}

	m := NewModel(renderer, annotation.NewStore(), highlighter, ModelConfig{NoColors: true, TreeWidthRatio: 3})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()
	m.openThemeSelector()

	draculaIdx := -1
	for i, e := range m.themeSel.entries {
		if e.name == "dracula" {
			draculaIdx = i
			break
		}
	}
	require.NotEqual(t, -1, draculaIdx, "dracula entry should be present")

	m.themeSel.cursor = draculaIdx
	m.previewTheme()
	assert.Equal(t, plainStyles().FileSelected.Render("x"), m.styles.FileSelected.Render("x"))
	assert.Empty(t, m.styles.colors.Accent, "preview should stay monochrome")

	result, _ := m.confirmThemeSelect()
	updated := result.(Model)

	assert.Equal(t, plainStyles().FileSelected.Render("x"), updated.styles.FileSelected.Render("x"))
	assert.Empty(t, updated.styles.colors.Accent, "confirm should stay monochrome")
	assert.Equal(t, "dracula", updated.activeThemeName)
}

func TestApplyTheme_invalidChromaStyleKeepsPreviousHighlightingStyle(t *testing.T) {
	currentStyle := "orig-style"
	highlightCalls := 0
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string {
			highlightCalls++
			return []string{"highlighted"}
		},
		SetStyleFunc: func(style string) bool {
			return false
		},
		StyleNameFunc: func() string { return currentStyle },
	}

	m := NewModel(renderer, annotation.NewStore(), highlighter, ModelConfig{TreeWidthRatio: 3})
	m.currFile = "main.go"
	m.diffLines = []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "package main"}}
	m.highlightedLines = []string{"existing"}

	m.applyTheme(theme.Theme{
		ChromaStyle: "bad-style",
		Colors: map[string]string{
			"color-accent":      "#bd93f9",
			"color-border":      "#6272a4",
			"color-normal":      "#f8f8f2",
			"color-muted":       "#6272a4",
			"color-selected-fg": "#f8f8f2",
			"color-selected-bg": "#44475a",
			"color-annotation":  "#f1fa8c",
			"color-cursor-fg":   "#282a36",
			"color-cursor-bg":   "#f8f8f2",
			"color-add-fg":      "#50fa7b",
			"color-add-bg":      "#2a4a2a",
			"color-remove-fg":   "#ff5555",
			"color-remove-bg":   "#4a2a2a",
			"color-modify-fg":   "#ffb86c",
			"color-modify-bg":   "#3a3a2a",
			"color-tree-bg":     "#21222c",
			"color-diff-bg":     "#282a36",
			"color-status-fg":   "#f8f8f2",
			"color-status-bg":   "#44475a",
			"color-search-fg":   "#282a36",
			"color-search-bg":   "#f1fa8c",
		},
	})

	assert.Equal(t, "orig-style", currentStyle)
	assert.Equal(t, 0, highlightCalls, "invalid chroma style should not trigger re-highlighting")
	assert.Equal(t, []string{"existing"}, m.highlightedLines)
}
