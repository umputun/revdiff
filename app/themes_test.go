package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/theme"
)

// testThemeContent returns a valid theme file string with the given name.
func testThemeContent(name string) string {
	return "# name: " + name + "\n# description: test theme\n# author: Test\nchroma-style = monokai\n" +
		"color-accent = #bd93f9\ncolor-border = #6272a4\ncolor-normal = #f8f8f2\ncolor-muted = #6272a4\n" +
		"color-selected-fg = #f8f8f2\ncolor-selected-bg = #44475a\ncolor-annotation = #f1fa8c\n" +
		"color-cursor-fg = #282a36\ncolor-cursor-bg = #f8f8f2\ncolor-add-fg = #50fa7b\ncolor-add-bg = #2a4a2a\n" +
		"color-remove-fg = #ff5555\ncolor-remove-bg = #4a2a2a\ncolor-modify-fg = #ffb86c\ncolor-modify-bg = #3a3a2a\n" +
		"color-tree-bg = #21222c\ncolor-diff-bg = #282a36\ncolor-status-fg = #f8f8f2\ncolor-status-bg = #44475a\n" +
		"color-search-fg = #282a36\ncolor-search-bg = #f1fa8c\n"
}

func TestApplyTheme(t *testing.T) {
	t.Run("overwrites all 23 fields and chroma-style", func(t *testing.T) {
		opts := options{}
		opts.Colors.Accent = "#original"
		opts.ChromaStyle = "original-style"

		th := theme.Theme{
			ChromaStyle: "dracula",
			Colors: map[string]string{
				"color-accent": "#bd93f9", "color-border": "#6272a4", "color-normal": "#f8f8f2",
				"color-muted": "#6272a4", "color-selected-fg": "#f8f8f2", "color-selected-bg": "#44475a",
				"color-annotation": "#f1fa8c", "color-cursor-fg": "#f8f8f2", "color-cursor-bg": "#44475a",
				"color-add-fg": "#50fa7b", "color-add-bg": "#1a3a1a", "color-remove-fg": "#ff5555",
				"color-remove-bg": "#3a1a1a", "color-word-add-bg": "#2a4a2a", "color-word-remove-bg": "#4a2a2a",
				"color-modify-fg": "#ffb86c", "color-modify-bg": "#3a2a1a",
				"color-tree-bg": "#282a36", "color-diff-bg": "#282a36", "color-status-fg": "#282a36",
				"color-status-bg": "#bd93f9", "color-search-fg": "#282a36", "color-search-bg": "#f1fa8c",
			},
		}
		applyTheme(&opts, th)

		// verify chroma-style
		assert.Equal(t, "dracula", opts.ChromaStyle)

		// verify all 23 color fields
		assert.Equal(t, "#bd93f9", opts.Colors.Accent)
		assert.Equal(t, "#6272a4", opts.Colors.Border)
		assert.Equal(t, "#f8f8f2", opts.Colors.Normal)
		assert.Equal(t, "#6272a4", opts.Colors.Muted)
		assert.Equal(t, "#f8f8f2", opts.Colors.SelectedFg)
		assert.Equal(t, "#44475a", opts.Colors.SelectedBg)
		assert.Equal(t, "#f1fa8c", opts.Colors.Annotation)
		assert.Equal(t, "#f8f8f2", opts.Colors.CursorFg)
		assert.Equal(t, "#44475a", opts.Colors.CursorBg)
		assert.Equal(t, "#50fa7b", opts.Colors.AddFg)
		assert.Equal(t, "#1a3a1a", opts.Colors.AddBg)
		assert.Equal(t, "#ff5555", opts.Colors.RemoveFg)
		assert.Equal(t, "#3a1a1a", opts.Colors.RemoveBg)
		assert.Equal(t, "#2a4a2a", opts.Colors.WordAddBg)
		assert.Equal(t, "#4a2a2a", opts.Colors.WordRemoveBg)
		assert.Equal(t, "#ffb86c", opts.Colors.ModifyFg)
		assert.Equal(t, "#3a2a1a", opts.Colors.ModifyBg)
		assert.Equal(t, "#282a36", opts.Colors.TreeBg)
		assert.Equal(t, "#282a36", opts.Colors.DiffBg)
		assert.Equal(t, "#282a36", opts.Colors.StatusFg)
		assert.Equal(t, "#bd93f9", opts.Colors.StatusBg)
		assert.Equal(t, "#282a36", opts.Colors.SearchFg)
		assert.Equal(t, "#f1fa8c", opts.Colors.SearchBg)
	})

	t.Run("chroma-style always overwritten", func(t *testing.T) {
		opts := options{}
		opts.ChromaStyle = "original-style"
		th := theme.Theme{ChromaStyle: "new-style", Colors: map[string]string{}}
		applyTheme(&opts, th)
		assert.Equal(t, "new-style", opts.ChromaStyle)
	})

	t.Run("partial theme overwrites only present keys", func(t *testing.T) {
		opts := options{}
		opts.Colors.Accent = "#original-accent"
		opts.Colors.Border = "#original-border"
		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-accent": "#new-accent"}}
		applyTheme(&opts, th)
		assert.Equal(t, "#new-accent", opts.Colors.Accent)
		assert.Equal(t, "#original-border", opts.Colors.Border, "unset required key should not change opts")
	})

	t.Run("optional keys cleared when absent from theme", func(t *testing.T) {
		opts := options{}
		opts.Colors.CursorBg = "#111111"
		opts.Colors.TreeBg = "#222222"
		opts.Colors.DiffBg = "#333333"
		opts.Colors.WordAddBg = "#444444"
		opts.Colors.WordRemoveBg = "#555555"
		opts.Colors.Accent = "#original-accent"

		// theme has accent but omits all optional keys
		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-accent": "#new-accent"}}
		applyTheme(&opts, th)

		assert.Equal(t, "#new-accent", opts.Colors.Accent)
		assert.Empty(t, opts.Colors.CursorBg, "optional cursor-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.TreeBg, "optional tree-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.DiffBg, "optional diff-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.WordAddBg, "optional word-add-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.WordRemoveBg, "optional word-remove-bg should be cleared when theme omits it")
	})

	t.Run("optional keys preserved when present in theme", func(t *testing.T) {
		opts := options{}
		opts.Colors.TreeBg = "#old-value"

		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-tree-bg": "#new-tree-bg"}}
		applyTheme(&opts, th)

		assert.Equal(t, "#new-tree-bg", opts.Colors.TreeBg, "optional key present in theme should overwrite")
	})
}

func TestDumpThemeOutput(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)

	colors := collectColors(opts)
	var buf bytes.Buffer
	require.NoError(t, theme.Dump(theme.Theme{Colors: colors, ChromaStyle: opts.ChromaStyle}, &buf))
	output := buf.String()
	assert.Contains(t, output, "chroma-style = catppuccin-macchiato")
	assert.Contains(t, output, "color-accent = #D5895F")
	assert.Contains(t, output, "color-add-fg = #87d787")

	// verify dump output can be parsed back (roundtrip)
	th, err := theme.Parse(strings.NewReader(output))
	require.NoError(t, err, "dump-theme output must be parseable")
	assert.Equal(t, opts.ChromaStyle, th.ChromaStyle)
	assert.Equal(t, colors["color-accent"], th.Colors["color-accent"])
}

func TestListThemesOutput(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)
}

func TestCollectColors(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	colors := collectColors(opts)
	assert.Equal(t, "#D5895F", colors["color-accent"])
	assert.Equal(t, "#585858", colors["color-border"])
	assert.Equal(t, "#87d787", colors["color-add-fg"])
	// 5 optional keys (cursor-bg, tree-bg, diff-bg, word-add-bg, word-remove-bg) have no default and are omitted
	assert.Len(t, colors, 18)
	assert.Empty(t, colors["color-cursor-bg"])
	assert.Empty(t, colors["color-tree-bg"])
	assert.Empty(t, colors["color-diff-bg"])
}

func TestColorFieldPtrs(t *testing.T) {
	t.Run("returns 23 entries matching theme.ColorKeys", func(t *testing.T) {
		opts := options{}
		ptrs := colorFieldPtrs(&opts)
		assert.Len(t, ptrs, 23)
		for _, key := range theme.ColorKeys() {
			_, ok := ptrs[key]
			assert.True(t, ok, "missing key %q in colorFieldPtrs", key)
		}
	})

	t.Run("pointers write to correct fields", func(t *testing.T) {
		opts := options{}
		ptrs := colorFieldPtrs(&opts)
		*ptrs["color-accent"] = "#aaa"
		*ptrs["color-search-bg"] = "#bbb"
		assert.Equal(t, "#aaa", opts.Colors.Accent)
		assert.Equal(t, "#bbb", opts.Colors.SearchBg)
	})

	t.Run("applyTheme and collectColors round-trip", func(t *testing.T) {
		// set all colors via applyTheme, then collectColors should return same values
		opts := options{}
		th := theme.Theme{ChromaStyle: "test-style", Colors: map[string]string{}}
		for i, key := range theme.ColorKeys() {
			th.Colors[key] = fmt.Sprintf("#%02x%02x%02x", i*10, i*11, i*12)
		}
		applyTheme(&opts, th)
		collected := collectColors(opts)
		for _, key := range theme.ColorKeys() {
			assert.Equal(t, th.Colors[key], collected[key], "round-trip mismatch for %q", key)
		}
	})
}

func TestThemeOverridesColorFlags(t *testing.T) {
	// simulate the full flow: parseArgs sets --color-accent from CLI,
	// then applyTheme overwrites it with theme value
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))

	// parse args with explicit --color-accent flag
	opts, err := parseArgs(append(noConfigArgs(t), "--color-accent=#ffffff"))
	require.NoError(t, err)
	assert.Equal(t, "#ffffff", opts.Colors.Accent, "CLI flag sets accent before theme")

	// load theme and overwrite — theme wins unconditionally
	th, err := theme.Load("dracula", themesDir)
	require.NoError(t, err)
	applyTheme(&opts, th)
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme should override CLI --color-accent")
	assert.Equal(t, "dracula", opts.ChromaStyle, "theme should set chroma-style")
}

func TestResolveThemeConflicts(t *testing.T) {
	t.Run("theme with no-colors clears no-colors", func(t *testing.T) {
		opts := options{Theme: "dracula", NoColors: true}
		resolveThemeConflicts(&opts)
		assert.False(t, opts.NoColors, "--no-colors should be cleared when --theme is set")
	})

	t.Run("no theme keeps no-colors", func(t *testing.T) {
		opts := options{NoColors: true}
		resolveThemeConflicts(&opts)
		assert.True(t, opts.NoColors, "--no-colors should remain when no theme is set")
	})

	t.Run("theme without no-colors is noop", func(t *testing.T) {
		opts := options{Theme: "dracula"}
		resolveThemeConflicts(&opts)
		assert.False(t, opts.NoColors)
	})
}

func TestHandleThemes_InitThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InitThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "init-themes should signal exit")
	assert.Contains(t, stdout.String(), "bundled themes written to")
	assert.Contains(t, stdout.String(), themesDir)
}

func TestHandleThemes_InitThemesEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{InitThemes: true}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_ListThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "list-themes should signal exit")
	output := stdout.String()
	assert.Contains(t, output, "dracula")
	assert.Contains(t, output, "nord")
	assert.NotContains(t, output, "■")
	assert.NotContains(t, output, "\u2713")
}

func TestHandleThemes_ListThemes_localOnly(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-custom"), []byte("custom\n"), 0o600))
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "my-custom")
	assert.NotContains(t, stdout.String(), "◇")
}

func TestHandleThemes_ListThemesEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_LoadTheme(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula"}
	opts.Colors.Accent = "#original"

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done, "load theme should not signal exit")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme should override accent color")
	assert.Equal(t, "dracula", opts.ChromaStyle)
}

func TestHandleThemes_LoadThemeNotFound(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "nonexistent"}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
}

func TestHandleThemes_LoadThemeEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula"}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_NoColorsWarning(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula", NoColors: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Contains(t, stderr.String(), "warning: --no-colors ignored when --theme is set")
	assert.False(t, opts.NoColors, "--no-colors should be cleared")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme colors should be applied")
}

func TestHandleThemes_InitAllThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InitAllThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "init-all-themes should signal exit")
	assert.Contains(t, stdout.String(), "all themes written to")

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	galleryNames, err := theme.GalleryNames()
	require.NoError(t, err)
	assert.Equal(t, galleryNames, names)
}

func TestHandleThemes_InitAllThemesEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{InitAllThemes: true}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_InstallTheme(t *testing.T) {
	themesDir := t.TempDir() // pre-existing dir prevents auto-init
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula", "nord"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "install-theme should signal exit")
	assert.Contains(t, stdout.String(), "2 theme(s) installed")

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord"}, names)
}

func TestHandleThemes_InstallThemeSkipsAutoInitOnFirstRun(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula"}, names)
}

func TestHandleThemes_InstallThemeNotFound(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"nonexistent"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "not found in gallery")
}

func TestHandleThemes_InstallThemeNotFoundDoesNotWriteBundledThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"nonexistent"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "not found in gallery")

	names, listErr := theme.List(themesDir)
	require.NoError(t, listErr)
	assert.Empty(t, names)
}

func TestHandleThemes_InstallThemeEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula"}}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_InstallThemeLocalFile(t *testing.T) {
	// create a valid theme file at a local path
	srcDir := t.TempDir()
	content := testThemeContent("my-local")
	srcPath := filepath.Join(srcDir, "my-local")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	themesDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{srcPath}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "installed my-local from")
	assert.Contains(t, stdout.String(), "1 theme(s) installed")

	// verify the theme was installed
	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Contains(t, names, "my-local")
}

func TestHandleThemes_InstallThemeMixed(t *testing.T) {
	// mix a gallery name and a local path in one invocation
	srcDir := t.TempDir()
	content := testThemeContent("custom")
	srcPath := filepath.Join(srcDir, "custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	themesDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{srcPath, "dracula"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "2 theme(s) installed")

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Contains(t, names, "custom")
	assert.Contains(t, names, "dracula")
}

func TestHandleThemes_NoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{}

	done, err := handleThemes(&opts, t.TempDir(), &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDefaultThemesDir(t *testing.T) {
	dir := defaultThemesDir()
	assert.Contains(t, dir, ".config")
	assert.Contains(t, dir, "revdiff")
	assert.Contains(t, dir, "themes")
}
