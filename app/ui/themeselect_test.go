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
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/style"
)

func TestColorsFromTheme(t *testing.T) {
	th := theme.Theme{
		ChromaStyle: "dracula",
		Colors: map[string]string{
			"color-accent":         "#bd93f9",
			"color-border":         "#6272a4",
			"color-normal":         "#f8f8f2",
			"color-muted":          "#6272a4",
			"color-selected-fg":    "#f8f8f2",
			"color-selected-bg":    "#44475a",
			"color-annotation":     "#f1fa8c",
			"color-cursor-fg":      "#282a36",
			"color-cursor-bg":      "#f8f8f2",
			"color-add-fg":         "#50fa7b",
			"color-add-bg":         "#2a4a2a",
			"color-remove-fg":      "#ff5555",
			"color-remove-bg":      "#4a2a2a",
			"color-word-add-bg":    "#3a5a3a",
			"color-word-remove-bg": "#5a3a3a",
			"color-modify-fg":      "#ffb86c",
			"color-modify-bg":      "#3a3a2a",
			"color-tree-bg":        "#21222c",
			"color-diff-bg":        "#282a36",
			"color-status-fg":      "#f8f8f2",
			"color-status-bg":      "#44475a",
			"color-search-fg":      "#282a36",
			"color-search-bg":      "#f1fa8c",
		},
	}

	colors := colorsFromTheme(th)
	assert.Equal(t, "#bd93f9", colors.Accent)
	assert.Equal(t, "#6272a4", colors.Border)
	assert.Equal(t, "#f8f8f2", colors.Normal)
	assert.Equal(t, "#50fa7b", colors.AddFg)
	assert.Equal(t, "#282a36", colors.DiffBg)
	assert.Equal(t, "#3a5a3a", colors.WordAddBg)
	assert.Equal(t, "#5a3a3a", colors.WordRemoveBg)
}

func TestBuildThemeEntries(t *testing.T) {
	m := Model{themesDir: t.TempDir()}
	entries, err := m.buildThemeEntries()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 5, "should have at least gallery themes")

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

	require.NotEmpty(t, entries)
	assert.Equal(t, theme.DefaultThemeName, entries[0].name, "default theme should be first")

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

func TestOpenThemeSelector_savesOriginalState(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()

	origResolver := m.resolver
	m.openThemeSelector()

	require.NotNil(t, m.themePreview)
	assert.Equal(t, origResolver, m.themePreview.origResolver)
	assert.Equal(t, "orig-style", m.themePreview.origChroma)
	assert.True(t, m.overlay.Active())
	assert.Equal(t, overlay.KindThemeSelect, m.overlay.Kind())
}

func TestCancelThemeSelect_restoresOriginalTheme(t *testing.T) {
	currentStyle := "orig-style"
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc: func(s string) bool {
			currentStyle = s
			return true
		},
		StyleNameFunc: func() string { return currentStyle },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()

	origAccent := m.resolver.Color(style.ColorKeyAccentFg)
	m.openThemeSelector()

	m.previewThemeByName("dracula")
	require.NotEqual(t, origAccent, m.resolver.Color(style.ColorKeyAccentFg), "preview should change accent")
	require.Equal(t, "dracula", currentStyle)

	m.cancelThemeSelect()

	assert.Equal(t, origAccent, m.resolver.Color(style.ColorKeyAccentFg), "cancel should restore original accent")
	assert.Equal(t, "orig-style", currentStyle)
	assert.Nil(t, m.themePreview, "preview session should be cleared")
}

func TestPreviewThemeByName_appliesTheme(t *testing.T) {
	currentStyle := "orig-style"
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc: func(s string) bool {
			currentStyle = s
			return true
		},
		StyleNameFunc: func() string { return currentStyle },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()

	m.openThemeSelector()
	m.previewThemeByName("dracula")

	assert.Equal(t, "dracula", currentStyle, "should apply dracula chroma style")
}

func TestPreviewThemeByName_nilSessionNoOp(t *testing.T) {
	m := testModel(nil, nil)
	m.previewThemeByName("dracula") // should not panic
}

func TestConfirmThemeByName_appliesAndPersists(t *testing.T) {
	currentStyle := "orig-style"
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc: func(s string) bool {
			currentStyle = s
			return true
		},
		StyleNameFunc: func() string { return currentStyle },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()

	m.openThemeSelector()
	m.confirmThemeByName("dracula")

	assert.Equal(t, "dracula", m.activeThemeName)
	assert.Equal(t, "dracula", currentStyle)
	assert.Nil(t, m.themePreview, "preview session should be cleared after confirm")
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

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		NoColors: true, TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
	m.width = 80
	m.height = 24
	m.ready = true
	m.themesDir = t.TempDir()
	m.openThemeSelector()

	m.previewThemeByName("dracula")
	plainRes := style.PlainResolver()
	assert.Equal(t, plainRes.Style(style.StyleKeyFileSelected).Render("x"),
		m.resolver.Style(style.StyleKeyFileSelected).Render("x"))
	assert.Empty(t, string(m.resolver.Color(style.ColorKeyAccentFg)), "preview should stay monochrome")

	m.confirmThemeByName("dracula")

	assert.Equal(t, plainRes.Style(style.StyleKeyFileSelected).Render("x"),
		m.resolver.Style(style.StyleKeyFileSelected).Render("x"))
	assert.Empty(t, string(m.resolver.Color(style.ColorKeyAccentFg)), "confirm should stay monochrome")
	assert.Equal(t, "dracula", m.activeThemeName)
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
		SetStyleFunc:  func(string) bool { return false },
		StyleNameFunc: func() string { return currentStyle },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
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
