package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/style"
)

// testThemeCatalog is a manual ThemeCatalog fake with configurable entries and themes.
// moq can't be used here because ThemeEntry/ThemeSpec are defined in this package (import cycle).
type testThemeCatalog struct {
	entries   []ThemeEntry
	themes    map[string]ThemeSpec
	persisted []string
}

func (tc *testThemeCatalog) Entries() ([]ThemeEntry, error) { return tc.entries, nil }

func (tc *testThemeCatalog) Resolve(name string) (ThemeSpec, bool) {
	spec, ok := tc.themes[name]
	return spec, ok
}

func (tc *testThemeCatalog) Persist(name string) error {
	tc.persisted = append(tc.persisted, name)
	return nil
}

// newTestThemeCatalog returns a catalog pre-populated with revdiff, dracula, and nord themes.
func newTestThemeCatalog() *testThemeCatalog {
	entries := []ThemeEntry{
		{Name: "revdiff", Local: false, AccentColor: "#61afef"},
		{Name: "dracula", Local: false, AccentColor: "#bd93f9"},
		{Name: "nord", Local: false, AccentColor: "#88c0d0"},
	}
	themes := map[string]ThemeSpec{
		"revdiff": {
			Colors: style.Colors{
				Accent: "#61afef", Border: "#3e4451", Normal: "#abb2bf", Muted: "#5c6370",
				SelectedFg: "#282c34", SelectedBg: "#61afef", Annotation: "#e5c07b",
				CursorFg: "#282c34", CursorBg: "#abb2bf",
				AddFg: "#98c379", AddBg: "#2a4a2a", RemoveFg: "#e06c75", RemoveBg: "#4a2a2a",
				WordAddBg: "#3a5a3a", WordRemoveBg: "#5a3a3a",
				ModifyFg: "#d19a66", ModifyBg: "#3a3a2a",
				TreeBg: "#21252b", DiffBg: "#282c34",
				StatusFg: "#abb2bf", StatusBg: "#3e4451",
				SearchFg: "#282c34", SearchBg: "#e5c07b",
			},
			ChromaStyle: "onedark",
		},
		"dracula": {
			Colors: style.Colors{
				Accent: "#bd93f9", Border: "#6272a4", Normal: "#f8f8f2", Muted: "#6272a4",
				SelectedFg: "#f8f8f2", SelectedBg: "#44475a", Annotation: "#f1fa8c",
				CursorFg: "#282a36", CursorBg: "#f8f8f2",
				AddFg: "#50fa7b", AddBg: "#2a4a2a", RemoveFg: "#ff5555", RemoveBg: "#4a2a2a",
				WordAddBg: "#3a5a3a", WordRemoveBg: "#5a3a3a",
				ModifyFg: "#ffb86c", ModifyBg: "#3a3a2a",
				TreeBg: "#21222c", DiffBg: "#282a36",
				StatusFg: "#f8f8f2", StatusBg: "#44475a",
				SearchFg: "#282a36", SearchBg: "#f1fa8c",
			},
			ChromaStyle: "dracula",
		},
		"nord": {
			Colors: style.Colors{
				Accent: "#88c0d0", Border: "#4c566a", Normal: "#d8dee9", Muted: "#4c566a",
				SelectedFg: "#2e3440", SelectedBg: "#88c0d0", Annotation: "#ebcb8b",
				CursorFg: "#2e3440", CursorBg: "#d8dee9",
				AddFg: "#a3be8c", AddBg: "#2a4a2a", RemoveFg: "#bf616a", RemoveBg: "#4a2a2a",
				WordAddBg: "#3a5a3a", WordRemoveBg: "#5a3a3a",
				ModifyFg: "#d08770", ModifyBg: "#3a3a2a",
				TreeBg: "#242933", DiffBg: "#2e3440",
				StatusFg: "#d8dee9", StatusBg: "#4c566a",
				SearchFg: "#2e3440", SearchBg: "#ebcb8b",
			},
			ChromaStyle: "nord",
		},
	}
	return &testThemeCatalog{entries: entries, themes: themes}
}

func TestOpenThemeSelector_savesOriginalState(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: newTestThemeCatalog(),
	})
	m.layout.width = 80
	m.layout.height = 24
	m.ready = true

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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
		TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: newTestThemeCatalog(),
	})
	m.layout.width = 80
	m.layout.height = 24
	m.ready = true

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

// TestModel_CancelThemeSelect_InvalidatesAnnotationRows mirrors
// TestModel_ApplyTheme_InvalidatesAnnotationRows. previewing a theme applies it
// (which invalidates and then re-fills rowCache with preview-styled rows), but
// cancel restores resolver/renderer to the original. without explicit
// invalidation, cached rows would carry the preview theme's AnnotationInline
// bytes after restore, producing stale themed annotation output on next render.
func TestModel_CancelThemeSelect_InvalidatesAnnotationRows(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}
	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: newTestThemeCatalog(),
	})
	m.file.name = "a.go"
	m.layout.width = 120
	m.layout.treeWidth = 20
	m.ready = true

	m.openThemeSelector()
	m.previewThemeByName("dracula")
	// populate cache AFTER the preview so cached rows carry dracula's
	// AnnotationInline styling — exactly the stale-bytes scenario the fix targets.
	m.annotationVisualRows("\U0001f4ac ", "one")
	m.annotationVisualRows("\U0001f4ac ", "two")
	require.Len(t, m.annot.rowCache, 2)

	m.cancelThemeSelect()

	assert.Empty(t, m.annot.rowCache, "cache must be cleared after cancelThemeSelect to drop preview-themed rows")
}

func TestPreviewThemeByName_appliesTheme(t *testing.T) {
	currentStyle := "orig-style"
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
		TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: newTestThemeCatalog(),
	})
	m.layout.width = 80
	m.layout.height = 24
	m.ready = true

	m.openThemeSelector()
	m.previewThemeByName("dracula")

	assert.Equal(t, "dracula", currentStyle, "should apply dracula chroma style")
}

func TestPreviewThemeByName_nilSessionNoOp(t *testing.T) {
	m := testModel(nil, nil)
	m.previewThemeByName("dracula") // should not panic
}

func TestPreviewThemeByName_unknownThemeNoOp(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: newTestThemeCatalog(),
	})
	m.layout.width = 80
	m.layout.height = 24
	m.ready = true

	m.openThemeSelector()
	origResolver := m.resolver
	m.previewThemeByName("nonexistent")

	assert.Equal(t, origResolver, m.resolver, "unknown theme should not change resolver")
}

func TestConfirmThemeByName_appliesAndPersists(t *testing.T) {
	currentStyle := "orig-style"
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc: func(s string) bool {
			currentStyle = s
			return true
		},
		StyleNameFunc: func() string { return currentStyle },
	}

	catalog := newTestThemeCatalog()
	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: catalog,
	})
	m.layout.width = 80
	m.layout.height = 24
	m.ready = true

	m.openThemeSelector()
	m.confirmThemeByName("dracula")

	assert.Equal(t, "dracula", m.activeThemeName)
	assert.Equal(t, "dracula", currentStyle)
	assert.Nil(t, m.themePreview, "preview session should be cleared after confirm")
	assert.Equal(t, []string{"dracula"}, catalog.persisted, "theme should be persisted via catalog")
}

func TestThemeSelectPreviewAndConfirmPreserveNoColors(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}

	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		NoColors: true, TreeWidthRatio: 3, Overlay: overlay.NewManager(), Themes: newTestThemeCatalog(),
	})
	m.layout.width = 80
	m.layout.height = 24
	m.ready = true
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
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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
	m.file.name = "main.go"
	m.file.lines = []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "package main"}}
	m.file.highlighted = []string{"existing"}

	m.applyTheme(ThemeSpec{
		Colors: style.Colors{
			Accent: "#bd93f9", Border: "#6272a4", Normal: "#f8f8f2", Muted: "#6272a4",
			SelectedFg: "#f8f8f2", SelectedBg: "#44475a", Annotation: "#f1fa8c",
			CursorFg: "#282a36", CursorBg: "#f8f8f2",
			AddFg: "#50fa7b", AddBg: "#2a4a2a", RemoveFg: "#ff5555", RemoveBg: "#4a2a2a",
			ModifyFg: "#ffb86c", ModifyBg: "#3a3a2a",
			TreeBg: "#21222c", DiffBg: "#282a36",
			StatusFg: "#f8f8f2", StatusBg: "#44475a",
			SearchFg: "#282a36", SearchBg: "#f1fa8c",
		},
		ChromaStyle: "bad-style",
	})

	assert.Equal(t, "orig-style", currentStyle)
	assert.Equal(t, 0, highlightCalls, "invalid chroma style should not trigger re-highlighting")
	assert.Equal(t, []string{"existing"}, m.file.highlighted)
}
