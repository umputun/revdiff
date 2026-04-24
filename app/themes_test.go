package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jessevdk/go-flags"
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

// testListThemeFiles returns sorted names of non-directory entries in dir.
func testListThemeFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		require.NoError(t, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
}

func TestApplyTheme(t *testing.T) {
	optional := theme.NewCatalog("").OptionalColorKeys()

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
		applyTheme(&opts, th, optional)

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
		applyTheme(&opts, th, optional)
		assert.Equal(t, "new-style", opts.ChromaStyle)
	})

	t.Run("partial theme overwrites only present keys", func(t *testing.T) {
		opts := options{}
		opts.Colors.Accent = "#original-accent"
		opts.Colors.Border = "#original-border"
		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-accent": "#new-accent"}}
		applyTheme(&opts, th, optional)
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
		applyTheme(&opts, th, optional)

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
		applyTheme(&opts, th, optional)

		assert.Equal(t, "#new-tree-bg", opts.Colors.TreeBg, "optional key present in theme should overwrite")
	})
}

func TestDumpThemeOutput(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)

	colors := collectColors(opts)
	th := theme.Theme{Colors: colors, ChromaStyle: opts.ChromaStyle}
	var buf bytes.Buffer
	require.NoError(t, th.Dump(&buf))
	output := buf.String()
	assert.Contains(t, output, "chroma-style = catppuccin-macchiato")
	assert.Contains(t, output, "color-accent = #D5895F")
	assert.Contains(t, output, "color-add-fg = #87d787")

	// verify dump output can be loaded back (roundtrip via temp file)
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test-roundtrip"), []byte(output), 0o600))
	parsed, err := theme.NewCatalog(tmpDir).Load("test-roundtrip")
	require.NoError(t, err, "dump-theme output must be loadable")
	assert.Equal(t, opts.ChromaStyle, parsed.ChromaStyle)
	assert.Equal(t, colors["color-accent"], parsed.Colors["color-accent"])
}

func TestListThemesOutput(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.NewCatalog(themesDir).InitBundled())

	names := testListThemeFiles(t, themesDir)
	assert.Equal(t, []string{"basic", "catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)
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
	t.Run("returns 23 entries", func(t *testing.T) {
		opts := options{}
		ptrs := colorFieldPtrs(&opts)
		assert.Len(t, ptrs, 23)
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
		ptrs := colorFieldPtrs(&opts)
		th := theme.Theme{ChromaStyle: "test-style", Colors: map[string]string{}}
		keys := make([]string, 0, len(ptrs))
		for key := range ptrs {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for i, key := range keys {
			th.Colors[key] = fmt.Sprintf("#%02x%02x%02x", i*10, i*11, i*12)
		}
		applyTheme(&opts, th, theme.NewCatalog("").OptionalColorKeys())
		collected := collectColors(opts)
		for _, key := range keys {
			assert.Equal(t, th.Colors[key], collected[key], "round-trip mismatch for %q", key)
		}
	})
}

func TestThemeOverridesColorFlags(t *testing.T) {
	// simulate the full flow: parseArgs sets --color-accent from CLI,
	// then applyTheme overwrites it with theme value
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	require.NoError(t, cat.InitBundled())

	// parse args with explicit --color-accent flag
	opts, err := parseArgs(append(noConfigArgs(t), "--color-accent=#ffffff"))
	require.NoError(t, err)
	assert.Equal(t, "#ffffff", opts.Colors.Accent, "CLI flag sets accent before theme")

	// load theme and overwrite — theme wins unconditionally
	th, err := cat.Load("dracula")
	require.NoError(t, err)
	applyTheme(&opts, th, cat.OptionalColorKeys())
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme should override CLI --color-accent")
	assert.Equal(t, "dracula", opts.ChromaStyle, "theme should set chroma-style")
}

func TestHandleThemes_InitThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InitThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "init-themes should signal exit")
	assert.Contains(t, stdout.String(), "bundled themes written to")
	assert.Contains(t, stdout.String(), themesDir)
}

func TestHandleThemes_InitThemesEmptyDir(t *testing.T) {
	cat := theme.NewCatalog("")
	var stdout, stderr bytes.Buffer
	opts := options{InitThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_ListThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	require.NoError(t, cat.InitBundled())
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
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
	cat := theme.NewCatalog(themesDir)
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-custom"), []byte("custom\n"), 0o600))
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "my-custom")
	assert.NotContains(t, stdout.String(), "◇")
}

func TestHandleThemes_ListThemesEmptyDir(t *testing.T) {
	cat := theme.NewCatalog("")
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_LoadTheme(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	require.NoError(t, cat.InitBundled())
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula"}
	opts.Colors.Accent = "#original"

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done, "load theme should not signal exit")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme should override accent color")
	assert.Equal(t, "dracula", opts.ChromaStyle)
}

func TestHandleThemes_LoadThemeNotFound(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	require.NoError(t, cat.InitBundled())
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "nonexistent"}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
}

func TestHandleThemes_LoadThemeEmptyDir(t *testing.T) {
	cat := theme.NewCatalog("")
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula"}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_NoColorsWarning(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	require.NoError(t, cat.InitBundled())
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula", NoColors: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Contains(t, stderr.String(), "warning: --no-colors ignored when --theme is set")
	assert.False(t, opts.NoColors, "--no-colors should be cleared")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme colors should be applied")
}

func TestHandleThemes_InitAllThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InitAllThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "init-all-themes should signal exit")
	assert.Contains(t, stdout.String(), "all gallery themes written to")

	names := testListThemeFiles(t, themesDir)
	assert.GreaterOrEqual(t, len(names), 7, "should have at least 7 gallery themes")
}

func TestHandleThemes_InitAllThemesEmptyDir(t *testing.T) {
	cat := theme.NewCatalog("")
	var stdout, stderr bytes.Buffer
	opts := options{InitAllThemes: true}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_InstallTheme(t *testing.T) {
	themesDir := t.TempDir() // pre-existing dir prevents auto-init
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula", "nord"}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "install-theme should signal exit")
	assert.Contains(t, stdout.String(), "2 theme(s) installed")

	names := testListThemeFiles(t, themesDir)
	assert.Equal(t, []string{"dracula", "nord"}, names)
}

func TestHandleThemes_InstallThemeSkipsAutoInitOnFirstRun(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula"}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)

	names := testListThemeFiles(t, themesDir)
	assert.Equal(t, []string{"dracula"}, names)
}

func TestHandleThemes_InstallThemeNotFound(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"nonexistent"}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "not found in gallery")
}

func TestHandleThemes_InstallThemeNotFoundDoesNotWriteBundledThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"nonexistent"}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "not found in gallery")

	names := testListThemeFiles(t, themesDir)
	assert.Empty(t, names)
}

func TestHandleThemes_InstallThemeEmptyDir(t *testing.T) {
	cat := theme.NewCatalog("")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula"}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
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
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{srcPath}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "installed my-local from")
	assert.Contains(t, stdout.String(), "1 theme(s) installed")

	// verify the theme was installed
	names := testListThemeFiles(t, themesDir)
	assert.Contains(t, names, "my-local")
}

func TestHandleThemes_InstallThemeMixed(t *testing.T) {
	// mix a gallery name and a local path in one invocation
	srcDir := t.TempDir()
	content := testThemeContent("custom")
	srcPath := filepath.Join(srcDir, "custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	themesDir := t.TempDir()
	cat := theme.NewCatalog(themesDir)
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{srcPath, "dracula"}}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "2 theme(s) installed")

	names := testListThemeFiles(t, themesDir)
	assert.Contains(t, names, "custom")
	assert.Contains(t, names, "dracula")
}

func TestHandleThemes_NoOp(t *testing.T) {
	cat := theme.NewCatalog(t.TempDir())
	var stdout, stderr bytes.Buffer
	opts := options{}

	done, err := handleThemes(&opts, cat, &stdout, &stderr)
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

func TestThemeCatalog_Entries(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.NewCatalog(themesDir).InitBundled())

	tc := &themeCatalog{catalog: theme.NewCatalog(themesDir), configPath: ""}
	entries, err := tc.Entries()
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	assert.Equal(t, "revdiff", entries[0].Name, "default theme should be first")
	for _, e := range entries {
		assert.NotEmpty(t, e.AccentColor, "every entry should have an accent color")
	}
}

func TestThemeCatalog_Resolve(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.NewCatalog(themesDir).InitBundled())

	tc := &themeCatalog{catalog: theme.NewCatalog(themesDir), configPath: ""}

	t.Run("found", func(t *testing.T) {
		spec, ok := tc.Resolve("dracula")
		assert.True(t, ok)
		assert.Equal(t, "dracula", spec.ChromaStyle)
		assert.Equal(t, "#bd93f9", spec.Colors.Accent)
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := tc.Resolve("no-such-theme-xyz")
		assert.False(t, ok)
	})
}

func TestThemeCatalog_Persist(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config")
	tc := &themeCatalog{catalog: theme.NewCatalog(t.TempDir()), configPath: configPath}

	require.NoError(t, tc.Persist("dracula"))

	data, err := os.ReadFile(configPath) //nolint:gosec // test-only path from t.TempDir()
	require.NoError(t, err)
	assert.Contains(t, string(data), "theme = dracula")
}

func TestThemeCatalog_PersistEmptyPath(t *testing.T) {
	tc := &themeCatalog{catalog: theme.NewCatalog(t.TempDir()), configPath: ""}
	require.NoError(t, tc.Persist("dracula"), "empty config path should be a no-op")
}

func TestColorsFromTheme(t *testing.T) {
	th := theme.Theme{
		ChromaStyle: "dracula",
		Colors: map[string]string{
			"color-accent": "#bd93f9", "color-border": "#6272a4", "color-normal": "#f8f8f2",
			"color-muted": "#6272a4", "color-selected-fg": "#f8f8f2", "color-selected-bg": "#44475a",
			"color-annotation": "#f1fa8c", "color-cursor-fg": "#282a36", "color-cursor-bg": "#f8f8f2",
			"color-add-fg": "#50fa7b", "color-add-bg": "#2a4a2a", "color-remove-fg": "#ff5555",
			"color-remove-bg": "#4a2a2a", "color-word-add-bg": "#3a5a3a", "color-word-remove-bg": "#5a3a3a",
			"color-modify-fg": "#ffb86c", "color-modify-bg": "#3a3a2a", "color-tree-bg": "#21222c",
			"color-diff-bg": "#282a36", "color-status-fg": "#f8f8f2", "color-status-bg": "#44475a",
			"color-search-fg": "#282a36", "color-search-bg": "#f1fa8c",
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

func TestPatchConfigTheme_existingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("wrap = true\ntheme = dracula\nblame = false\n"), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "theme = nord")
	assert.NotContains(t, string(data), "dracula")
	assert.Contains(t, string(data), "wrap = true")
	assert.Contains(t, string(data), "blame = false")
}

func TestPatchConfigTheme_noExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("wrap = true\nblame = false\n"), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "theme = nord")
	assert.Contains(t, string(data), "wrap = true")
}

func TestPatchConfigTheme_createsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("dracula"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Equal(t, "theme = dracula\n", string(data))
}

func TestPatchConfigTheme_createsParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config")

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("dracula"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Equal(t, "theme = dracula\n", string(data))
}

func TestPatchConfigTheme_skipsCommentedOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("# theme = dracula\nwrap = true\n"), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "# theme = dracula", "commented line should be preserved")
	assert.Contains(t, string(data), "theme = nord", "new theme line should be appended")
}

func TestPatchConfigTheme_semicolonComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("; theme = dracula\n"), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "; theme = dracula")
	assert.Contains(t, string(data), "theme = nord")
}

func TestPatchConfigTheme_rejectsNewlines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	tests := []struct {
		name, themeName string
	}{
		{name: "newline", themeName: "bad\ntheme"},
		{name: "carriage return", themeName: "bad\rtheme"},
		{name: "crlf", themeName: "bad\r\ntheme"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := (&themeCatalog{configPath: path}).patchConfigTheme(tc.themeName)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must not contain newlines")
		})
	}
}

func TestPatchConfigTheme_preservesFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := "wrap = true\n\n# Colors\ncolor-accent = #ff0000\ntheme = old\n\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("new"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "theme = new")
	assert.Contains(t, string(data), "# Colors")
	assert.Contains(t, string(data), "color-accent = #ff0000")
}

// reproduces issue #148: appending theme = ... after a trailing [color options] section
// must land inside [Application Options], not inside [color options].
func TestPatchConfigTheme_insertsBeforeNamedSection(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "application options then color options",
			content: "[Application Options]\nwrap = true\ncompact = true\n\n[color options]\n;color-word-add-bg = #1a8f00\n",
		},
		{
			name:    "only color options section",
			content: "[color options]\n;color-word-add-bg = #1a8f00\n",
		},
		{
			name:    "application options with trailing blank lines before named section",
			content: "[Application Options]\nwrap = true\n\n\n[color options]\n",
		},
		{
			name:    "case-insensitive application options header",
			content: "[application options]\nwrap = true\n\n[color options]\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))

			require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

			data, err := os.ReadFile(path) //nolint:gosec // test
			require.NoError(t, err)
			result := string(data)

			themeIdx := strings.Index(result, "theme = nord")
			colorIdx := strings.Index(result, "[color options]")
			require.NotEqual(t, -1, themeIdx, "theme = nord must be present: %q", result)
			require.NotEqual(t, -1, colorIdx, "[color options] header must be preserved: %q", result)
			assert.Less(t, themeIdx, colorIdx, "theme = nord must precede [color options], got: %q", result)
		})
	}
}

// when file already has [Application Options] but no other named sections,
// appending at EOF is correct (stays inside [Application Options]).
func TestPatchConfigTheme_appendsInsideApplicationOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := "[Application Options]\nwrap = true\ncompact = true\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	result := string(data)
	assert.Contains(t, result, "[Application Options]")
	assert.Contains(t, result, "theme = nord")
	// theme line must be after the [Application Options] header
	assert.Less(t, strings.Index(result, "[Application Options]"), strings.Index(result, "theme = nord"))
	// existing keys must be preserved
	assert.Contains(t, result, "wrap = true")
	assert.Contains(t, result, "compact = true")
}

// reproduces the upgrade path from issue #148: a config already corrupted by
// the pre-fix persist (theme = X sitting inside a trailing named section) must
// be healed, not just replaced in place.
func TestPatchConfigTheme_healsMisplacedThemeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := "[Application Options]\nwrap = true\n\n[color options]\n;color-word-add-bg = #1a8f00\ntheme = dracula\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	result := string(data)

	themeIdx := strings.Index(result, "theme = nord")
	colorIdx := strings.Index(result, "[color options]")
	require.NotEqual(t, -1, themeIdx, "new theme line must be present: %q", result)
	require.NotEqual(t, -1, colorIdx, "[color options] header must be preserved: %q", result)
	assert.Less(t, themeIdx, colorIdx, "theme = nord must precede [color options]: %q", result)
	assert.NotContains(t, result, "theme = dracula", "stale theme line must be removed: %q", result)
	assert.Contains(t, result, "wrap = true")
	assert.Contains(t, result, ";color-word-add-bg = #1a8f00")
}

// reproduces the hand-repair duplicate scenario flagged in post-merge review:
// a user whose config was originally corrupted (theme = X in [color options])
// adds a correct theme = Y in [Application Options] manually without deleting
// the stray. the next patch must update the default line AND remove the stray,
// otherwise go-flags keeps erroring on "unknown option: theme".
func TestPatchConfigTheme_removesStrayDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := "[Application Options]\nwrap = true\ntheme = dracula\n\n[color options]\ntheme = solarized-dark\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

	opts, parseErr := parsePatchedConfig(t, path)
	require.NoError(t, parseErr, "patched file must parse without go-flags error even with prior duplicates")
	assert.Equal(t, "nord", opts.Theme)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	result := string(data)
	assert.NotContains(t, result, "theme = dracula", "original default-scope value must be replaced")
	assert.NotContains(t, result, "theme = solarized-dark", "stray line inside [color options] must be removed")
	assert.Contains(t, result, "wrap = true")
	// exactly one theme line must remain
	assert.Equal(t, 1, strings.Count(result, "\ntheme = "), "exactly one theme entry must remain: %q", result)
}

// parsePatchedConfig parses a patched INI file through go-flags the same way the
// production code does (see loadConfigFile in config.go), so tests can assert the
// patched file not only looks right textually but actually resolves opts.Theme
// with no "unknown option" warning from go-flags.
func parsePatchedConfig(t *testing.T, path string) (options, error) {
	t.Helper()
	var opts options
	p := flags.NewParser(&opts, flags.Default)
	iniParser := flags.NewIniParser(p)
	if err := iniParser.ParseFile(path); err != nil {
		return opts, fmt.Errorf("parse %s: %w", path, err)
	}
	return opts, nil
}

// testdata fixtures round-trip through patchConfigTheme + flags.NewIniParser.
// this is the assertion that would have caught issue #148 in the first place.
func TestPatchConfigTheme_testdataRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		fixture string // path under app/testdata/themes/
	}{
		{name: "good config (theme already in [Application Options])", fixture: "good.ini"},
		{name: "no theme line, trailing [color options]", fixture: "no_theme.ini"},
		{name: "corrupted config (theme inside [color options])", fixture: "corrupted.ini"},
		{name: "duplicate (one valid, one stray in [color options])", fixture: "duplicate.ini"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "themes", tc.fixture))
			require.NoError(t, err)

			path := filepath.Join(t.TempDir(), "config")
			require.NoError(t, os.WriteFile(path, raw, 0o600)) //nolint:gosec // test fixture roundtrip

			require.NoError(t, (&themeCatalog{configPath: path}).patchConfigTheme("nord"))

			opts, parseErr := parsePatchedConfig(t, path)
			require.NoError(t, parseErr, "patched file must parse without go-flags error (this is the #148 invariant)")
			assert.Equal(t, "nord", opts.Theme, "Theme must be populated from the default section after patch")
		})
	}
}
