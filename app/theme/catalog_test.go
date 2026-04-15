package theme

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullThemeColors returns all 23 color key=value lines for use in test theme strings.
func fullThemeColors() string {
	return `color-accent = #bd93f9
color-border = #6272a4
color-normal = #f8f8f2
color-muted = #6272a4
color-selected-fg = #f8f8f2
color-selected-bg = #44475a
color-annotation = #f1fa8c
color-cursor-fg = #282a36
color-cursor-bg = #f8f8f2
color-add-fg = #50fa7b
color-add-bg = #2a4a2a
color-remove-fg = #ff5555
color-remove-bg = #4a2a2a
color-word-add-bg = #3a5a3a
color-word-remove-bg = #5a3a3a
color-modify-fg = #ffb86c
color-modify-bg = #3a3a2a
color-tree-bg = #21222c
color-diff-bg = #282a36
color-status-fg = #f8f8f2
color-status-bg = #44475a
color-search-fg = #282a36
color-search-bg = #f1fa8c
`
}

// fullThemeColorsMap returns all 23 color key-value pairs as a map.
func fullThemeColorsMap() map[string]string {
	return map[string]string{
		"color-accent": "#bd93f9", "color-border": "#6272a4", "color-normal": "#f8f8f2",
		"color-muted": "#6272a4", "color-selected-fg": "#f8f8f2", "color-selected-bg": "#44475a",
		"color-annotation": "#f1fa8c", "color-cursor-fg": "#282a36", "color-cursor-bg": "#f8f8f2",
		"color-add-fg": "#50fa7b", "color-add-bg": "#2a4a2a", "color-remove-fg": "#ff5555",
		"color-remove-bg": "#4a2a2a", "color-word-add-bg": "#3a5a3a", "color-word-remove-bg": "#5a3a3a",
		"color-modify-fg": "#ffb86c", "color-modify-bg": "#3a3a2a",
		"color-tree-bg": "#21222c", "color-diff-bg": "#282a36", "color-status-fg": "#f8f8f2",
		"color-status-bg": "#44475a", "color-search-fg": "#282a36", "color-search-bg": "#f1fa8c",
	}
}

func TestParse_validTheme(t *testing.T) {
	input := "# name: dracula\n# description: purple accent, vibrant colors\n\nchroma-style = dracula\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "dracula", th.Name)
	assert.Equal(t, "purple accent, vibrant colors", th.Description)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
	assert.Equal(t, "#6272a4", th.Colors["color-border"])
	assert.Equal(t, "#f8f8f2", th.Colors["color-normal"])
	assert.Len(t, th.Colors, 23)
}

func TestParse_authorAndBundled(t *testing.T) {
	input := "# name: custom\n# description: test\n# author: Jane Doe\n# bundled: true\n\nchroma-style = dracula\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe", th.Author)
	assert.True(t, th.Bundled)
}

func TestParse_bundledFalse(t *testing.T) {
	input := "# name: community\n# bundled: false\nchroma-style = dracula\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.False(t, th.Bundled)
}

func TestParse_noBundledField(t *testing.T) {
	input := "# name: community\nchroma-style = dracula\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.False(t, th.Bundled, "absent bundled field should default to false")
}

func TestParse_missingMetadata(t *testing.T) {
	input := "chroma-style = nord\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, th.Name)
	assert.Empty(t, th.Description)
	assert.Equal(t, "nord", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
}

func TestParse_emptyInput(t *testing.T) {
	_, err := NewCatalog("").parse(strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required key: chroma-style")
}

func TestParse_commentsOnly(t *testing.T) {
	input := `# name: test
# description: just comments
# random comment
`
	_, err := NewCatalog("").parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required key: chroma-style")
}

func TestParse_malformedLine(t *testing.T) {
	input := `color-accent = #bd93f9
this is not valid
`
	_, err := NewCatalog("").parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed line")
}

func TestParse_blankLinesAndSpacing(t *testing.T) {
	input := "\n  # name: spaced\n\n  chroma-style = monokai\n\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "spaced", th.Name)
	assert.Equal(t, "monokai", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
}

func TestParse_missingChromaStyle(t *testing.T) {
	input := fullThemeColors()
	_, err := NewCatalog("").parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required key: chroma-style")
}

func TestParse_ignoresNonColorKeys(t *testing.T) {
	input := "something-else = value\nchroma-style = monokai\n" + fullThemeColors()
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
	_, exists := th.Colors["something-else"]
	assert.False(t, exists, "non-color keys should be ignored")
}

func TestParse_missingColorKeys(t *testing.T) {
	input := "chroma-style = dracula\ncolor-accent = #bd93f9\ncolor-border = #6272a4\n"
	_, err := NewCatalog("").parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required keys")
	assert.Contains(t, err.Error(), "color-normal")
}

func TestParse_invalidHexColor(t *testing.T) {
	tests := []struct {
		name  string
		color string
	}{
		{name: "no hash prefix", color: "bd93f9"},
		{name: "three digit shorthand", color: "#abc"},
		{name: "non-hex characters", color: "#zzzzzz"},
		{name: "empty value", color: ""},
		{name: "named color word", color: "purple"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := "chroma-style = dracula\ncolor-accent = " + tc.color + "\n"
			_, err := NewCatalog("").parse(strings.NewReader(input))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid hex color")
		})
	}
}

func TestParse_optionalKeysOmitted(t *testing.T) {
	// theme without color-cursor-bg, color-tree-bg, color-diff-bg should parse successfully
	input := `chroma-style = dracula
color-accent = #bd93f9
color-border = #6272a4
color-normal = #f8f8f2
color-muted = #6272a4
color-selected-fg = #f8f8f2
color-selected-bg = #44475a
color-annotation = #f1fa8c
color-cursor-fg = #282a36
color-add-fg = #50fa7b
color-add-bg = #2a4a2a
color-remove-fg = #ff5555
color-remove-bg = #4a2a2a
color-modify-fg = #ffb86c
color-modify-bg = #3a3a2a
color-status-fg = #f8f8f2
color-status-bg = #44475a
color-search-fg = #282a36
color-search-bg = #f1fa8c
`
	th, err := NewCatalog("").parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Len(t, th.Colors, 18) // 23 - 5 optional
	assert.Empty(t, th.Colors["color-cursor-bg"])
	assert.Empty(t, th.Colors["color-tree-bg"])
	assert.Empty(t, th.Colors["color-diff-bg"])
	assert.Empty(t, th.Colors["color-word-add-bg"])
	assert.Empty(t, th.Colors["color-word-remove-bg"])
}

func TestDump_Parse_roundtrip(t *testing.T) {
	colors := fullThemeColorsMap()
	var buf bytes.Buffer
	err := (Theme{Name: "my-theme", Description: "test roundtrip", ChromaStyle: "dracula", Colors: colors}).Dump(&buf)
	require.NoError(t, err)

	th, err := NewCatalog("").parse(strings.NewReader(buf.String()))
	require.NoError(t, err)
	assert.Equal(t, "my-theme", th.Name)
	assert.Equal(t, "test roundtrip", th.Description)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Equal(t, colors, th.Colors)
}

func TestDump_Parse_roundtrip_withoutOptionalKeys(t *testing.T) {
	// dump a theme without optional keys, then parse it back
	colors := fullThemeColorsMap()
	delete(colors, "color-cursor-bg")
	delete(colors, "color-tree-bg")
	delete(colors, "color-diff-bg")
	delete(colors, "color-word-add-bg")
	delete(colors, "color-word-remove-bg")

	var buf bytes.Buffer
	err := (Theme{Name: "minimal", ChromaStyle: "dracula", Colors: colors}).Dump(&buf)
	require.NoError(t, err)

	th, err := NewCatalog("").parse(strings.NewReader(buf.String()))
	require.NoError(t, err)
	assert.Equal(t, "minimal", th.Name)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Len(t, th.Colors, 18)
	assert.Equal(t, colors, th.Colors)
}

func TestValidateHexColor(t *testing.T) {
	tests := []struct {
		name    string
		color   string
		wantErr bool
	}{
		{name: "valid lowercase", color: "#aabbcc", wantErr: false},
		{name: "valid uppercase", color: "#AABBCC", wantErr: false},
		{name: "valid mixed case", color: "#aAbBcC", wantErr: false},
		{name: "valid digits", color: "#123456", wantErr: false},
		{name: "valid black", color: "#000000", wantErr: false},
		{name: "valid white", color: "#ffffff", wantErr: false},
		{name: "missing hash", color: "aabbcc", wantErr: true},
		{name: "too short", color: "#abc", wantErr: true},
		{name: "too long", color: "#aabbccdd", wantErr: true},
		{name: "non-hex chars", color: "#gghhii", wantErr: true},
		{name: "empty string", color: "", wantErr: true},
		{name: "hash only", color: "#", wantErr: true},
		{name: "spaces", color: "# aabb", wantErr: true},
		{name: "rgb notation", color: "rgb(0,0,0)", wantErr: true},
		{name: "named color", color: "red", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := NewCatalog("").validateHexColor(tc.color)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid hex color")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOptionalColorKeys(t *testing.T) {
	opt := NewCatalog("").OptionalColorKeys()
	assert.Len(t, opt, 5)
	assert.True(t, opt["color-cursor-bg"])
	assert.True(t, opt["color-tree-bg"])
	assert.True(t, opt["color-diff-bg"])
	assert.True(t, opt["color-word-add-bg"])
	assert.True(t, opt["color-word-remove-bg"])

	// verify it returns a copy
	opt["color-accent"] = true
	assert.False(t, NewCatalog("").OptionalColorKeys()["color-accent"])
}

func Test_load_validFile(t *testing.T) {
	dir := t.TempDir()
	content := "# name: test-theme\n# description: a test\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test-theme"), []byte(content), 0o600))

	th, err := NewCatalog(dir).Load("test-theme")
	require.NoError(t, err)
	assert.Equal(t, "test-theme", th.Name)
	assert.Equal(t, "a test", th.Description)
	assert.Equal(t, "monokai", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
	assert.Len(t, th.Colors, 23)
}

func Test_load_missingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := NewCatalog(dir).Load("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func Test_load_fallbackToGallery(t *testing.T) {
	dir := t.TempDir() // empty dir, no local files
	th, err := NewCatalog(dir).Load("dracula")
	require.NoError(t, err, "should fall back to embedded gallery")
	assert.Equal(t, "dracula", th.Name)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.GreaterOrEqual(t, len(th.Colors), 18)
}

func Test_load_localOverridesGallery(t *testing.T) {
	dir := t.TempDir()
	// write a customized "dracula" with different accent
	content := "# name: dracula\n# description: customized\nchroma-style = dracula\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dracula"), []byte(content), 0o600))

	th, err := NewCatalog(dir).Load("dracula")
	require.NoError(t, err)
	assert.Equal(t, "customized", th.Description, "local file should override gallery")
}

func Test_load_emptyThemesDir(t *testing.T) {
	// with empty themesDir, should still fall back to gallery
	th, err := NewCatalog("").Load("nord")
	require.NoError(t, err)
	assert.Equal(t, "nord", th.Name)
}

func Test_load_invalidContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad"), []byte("not a valid line\n"), 0o600))

	_, err := NewCatalog(dir).Load("bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing theme")
}

func Test_load_pathTraversal(t *testing.T) {
	dir := t.TempDir()
	tests := []struct{ name string }{
		{name: "../etc/passwd"}, {name: "../../secret"}, {name: "/etc/passwd"}, {name: "."}, {name: ".."},
		{name: "sub/theme"}, {name: "a/../b"},
	}
	for _, tc := range tests {
		_, err := NewCatalog(dir).Load(tc.name)
		require.Error(t, err, "name=%q should be rejected", tc.name)
		assert.Contains(t, err.Error(), "invalid theme name", "name=%q", tc.name)
	}
}

func Test_load_permissionError(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "secret-theme")
	require.NoError(t, os.WriteFile(fpath, []byte("chroma-style = dracula\n"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(fpath, 0o600) }) // restore so TempDir cleanup works

	_, err := NewCatalog(dir).Load("secret-theme")
	require.Error(t, err, "should not silently fall back to gallery on permission error")
	assert.Contains(t, err.Error(), "opening theme")
}

func Test_list_multipleThemes(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"nord", "dracula", "solarized-dark"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("color-accent = #000\n"), 0o600))
	}

	names, err := NewCatalog(dir).list()
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord", "solarized-dark"}, names)
}

func Test_list_emptyDir(t *testing.T) {
	dir := t.TempDir()
	names, err := NewCatalog(dir).list()
	require.NoError(t, err)
	assert.Empty(t, names)
}

func Test_list_nonExistentDir(t *testing.T) {
	names, err := NewCatalog(filepath.Join(t.TempDir(), "nonexistent")).list()
	require.NoError(t, err)
	assert.Nil(t, names)
}

func Test_list_skipsDirectories(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mytheme"), []byte("color-accent = #000\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o750))

	names, err := NewCatalog(dir).list()
	require.NoError(t, err)
	assert.Equal(t, []string{"mytheme"}, names)
}

func Test_initBundled_createsDirAndFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := NewCatalog(dir).InitBundled()
	require.NoError(t, err)

	names, err := NewCatalog(dir).list()
	require.NoError(t, err)
	assert.Equal(t, []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)

	// verify files are non-empty
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test uses temp dir
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	}
}

func Test_initBundled_preservesUserThemes(t *testing.T) {
	dir := t.TempDir()
	userTheme := filepath.Join(dir, "my-custom")
	require.NoError(t, os.WriteFile(userTheme, []byte("color-accent = #111111\n"), 0o600))

	err := NewCatalog(dir).InitBundled()
	require.NoError(t, err)

	// user theme should be untouched
	data, err := os.ReadFile(userTheme) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Equal(t, "color-accent = #111111\n", string(data))

	// bundled themes should exist
	names, err := NewCatalog(dir).list()
	require.NoError(t, err)
	assert.Contains(t, names, "dracula")
	assert.Contains(t, names, "my-custom")
}

func Test_initBundled_overwritesBundledThemes(t *testing.T) {
	dir := t.TempDir()
	// write a fake "dracula" file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dracula"), []byte("old content\n"), 0o600))

	err := NewCatalog(dir).InitBundled()
	require.NoError(t, err)

	// dracula should be overwritten with bundled content
	data, err := os.ReadFile(filepath.Join(dir, "dracula")) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Contains(t, string(data), "# name: dracula")
	assert.NotContains(t, string(data), "old content")
}

func Test_initAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := NewCatalog(dir).InitAll()
	require.NoError(t, err)

	names, err := NewCatalog(dir).list()
	require.NoError(t, err)

	galleryNames, err := NewCatalog("").galleryNames()
	require.NoError(t, err)
	assert.Equal(t, galleryNames, names, "InitAll should write all gallery themes")
}

func Test_initAll_preservesUserThemes(t *testing.T) {
	dir := t.TempDir()
	userTheme := filepath.Join(dir, "my-custom")
	require.NoError(t, os.WriteFile(userTheme, []byte("user content\n"), 0o600))

	err := NewCatalog(dir).InitAll()
	require.NoError(t, err)

	data, err := os.ReadFile(userTheme) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Equal(t, "user content\n", string(data))
}

func Test_initNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := NewCatalog(dir).initNames([]string{"dracula", "nord"})
	require.NoError(t, err)

	names, err := NewCatalog(dir).list()
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord"}, names)
}

func Test_initNames_notInGallery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := NewCatalog(dir).initNames([]string{"nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in gallery")
}

func Test_bundledNames(t *testing.T) {
	names, err := NewCatalog("").bundledNames()
	require.NoError(t, err)
	assert.Equal(t, []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)
}

func TestBundledThemes_parseCorrectly(t *testing.T) {
	gallery, err := NewCatalog("").gallery()
	require.NoError(t, err)

	names, err := NewCatalog("").bundledNames()
	require.NoError(t, err)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			th, ok := gallery[name]
			require.True(t, ok)
			assert.True(t, th.Bundled, "bundled theme %q should have bundled: true", name)
			assert.Equal(t, name, th.Name)
			assert.NotEmpty(t, th.Description)
			assert.NotEmpty(t, th.ChromaStyle)

			// verify all required color keys are present (optional keys may be omitted)
			optional := NewCatalog("").OptionalColorKeys()
			for _, key := range colorKeys {
				if optional[key] {
					continue
				}
				assert.NotEmpty(t, th.Colors[key], "missing required color key %s in theme %s", key, name)
			}
		})
	}
}

func Test_installFile(t *testing.T) {
	// create a valid theme file
	srcDir := t.TempDir()
	content := "# name: my-custom\n# description: a custom theme\n# author: Test User\nchroma-style = monokai\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "my-custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	name, err := NewCatalog(destDir).installFile(srcPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-custom", name)

	// verify file was written
	names, err := NewCatalog(destDir).list()
	require.NoError(t, err)
	assert.Equal(t, []string{"my-custom"}, names)

	// verify it's a valid theme
	th, err := NewCatalog(destDir).Load("my-custom")
	require.NoError(t, err)
	assert.Equal(t, "my-custom", th.Name)
	assert.Equal(t, "Test User", th.Author)
}

func Test_installFile_invalidTheme(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "bad-theme")
	require.NoError(t, os.WriteFile(srcPath, []byte("not valid\n"), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	_, err := NewCatalog(destDir).installFile(srcPath, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing theme file")
}

func Test_installFile_notFound(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "themes")
	_, err := NewCatalog(destDir).installFile("/nonexistent/path/theme", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening theme file")
}

func Test_installFile_dotPaths(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "themes")
	for _, path := range []string{".", ".."} {
		_, err := NewCatalog(destDir).installFile(path, nil)
		require.Error(t, err, "path=%q should be rejected", path)
		assert.Contains(t, err.Error(), "invalid theme file name", "path=%q", path)
	}
}

func Test_installFile_invalidChromaStyle(t *testing.T) {
	srcDir := t.TempDir()
	content := "# name: bad-chroma\n# description: theme with bad chroma\nchroma-style = nonexistent-style\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "bad-chroma")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")

	// with validator that rejects the style
	rejectAll := func(string) bool { return false }
	_, err := NewCatalog(destDir).installFile(srcPath, rejectAll)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown chroma style")

	// without validator (nil) — should succeed
	name, err := NewCatalog(destDir).installFile(srcPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "bad-chroma", name)
}

func Test_installFile_setsNameFromFilenameWhenMissing(t *testing.T) {
	srcDir := t.TempDir()
	content := "# description: no explicit name\nchroma-style = monokai\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "my-custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	name, err := NewCatalog(destDir).installFile(srcPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-custom", name)

	th, err := NewCatalog(destDir).Load("my-custom")
	require.NoError(t, err)
	assert.Equal(t, "my-custom", th.Name)
}

func Test_installFile_rejectsMetadataNameMismatch(t *testing.T) {
	srcDir := t.TempDir()
	content := "# name: other-name\n# description: mismatch\nchroma-style = monokai\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "my-custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	_, err := NewCatalog(destDir).installFile(srcPath, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `theme metadata name "other-name" does not match file name "my-custom"`)
}

func Test_install_validatesGalleryNamesBeforeWriting(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "my-custom")
	content := "# name: my-custom\n# description: a custom theme\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	var out bytes.Buffer

	err := NewCatalog(destDir).Install([]string{srcPath, "nonexistent"}, nil, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `theme "nonexistent" not found in gallery`)

	names, listErr := NewCatalog(destDir).list()
	require.NoError(t, listErr)
	assert.Empty(t, names, "validation failure should prevent partial installs")
	assert.Empty(t, out.String())
}

func Test_isLocalPath(t *testing.T) {
	cat := NewCatalog("")
	assert.True(t, cat.isLocalPath("themes/custom"))
	assert.True(t, cat.isLocalPath("./custom"))
	assert.False(t, cat.isLocalPath("dracula"))
}

func Test_printList(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, NewCatalog(themesDir).InitBundled())

	localContent := "# name: my-local\n# description: local theme\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-local"), []byte(localContent), 0o600))

	var out bytes.Buffer
	err := NewCatalog(themesDir).PrintList(&out)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.NotEmpty(t, lines)
	assert.Equal(t, defaultThemeName, lines[0])
	assert.Contains(t, lines, "my-local")
	assert.Contains(t, lines, "dracula")
}

func TestActiveName(t *testing.T) {
	cat := NewCatalog("")
	assert.Equal(t, defaultThemeName, cat.ActiveName(""))
	assert.Equal(t, "dracula", cat.ActiveName("dracula"))
}

func TestWriteThemeFile_atomic(t *testing.T) {
	dir := t.TempDir()
	th, err := NewCatalog("").galleryTheme("dracula")
	require.NoError(t, err)

	require.NoError(t, NewCatalog(dir).writeThemeFile("dracula", th))

	data, err := os.ReadFile(filepath.Join(dir, "dracula")) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Contains(t, string(data), "# name: dracula")

	matches, err := filepath.Glob(filepath.Join(dir, "dracula.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestCatalog_Entries(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, NewCatalog(themesDir).InitBundled())

	cat := NewCatalog(themesDir)
	entries, err := cat.Entries()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 5, "should have at least gallery themes")

	// default theme should be first
	require.NotEmpty(t, entries)
	assert.Equal(t, defaultThemeName, entries[0].Name)
}

func TestCatalog_Entries_includesLocalThemes(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, NewCatalog(themesDir).InitBundled())

	// write a custom theme
	customContent := "# name: my-custom\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-custom"), []byte(customContent), 0o600))

	cat := NewCatalog(themesDir)
	entries, err := cat.Entries()
	require.NoError(t, err)

	found := false
	for _, e := range entries {
		if e.Name == "my-custom" {
			found = true
			assert.True(t, e.Local, "custom theme should be marked local")
			break
		}
	}
	assert.True(t, found, "custom theme should appear in entries")
}

func TestCatalog_Resolve_found(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, NewCatalog(themesDir).InitBundled())

	cat := NewCatalog(themesDir)
	th, ok := cat.Resolve("dracula")
	assert.True(t, ok)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.NotEmpty(t, th.Colors)
}

func TestCatalog_Resolve_notFound(t *testing.T) {
	cat := NewCatalog(t.TempDir())
	_, ok := cat.Resolve("nonexistent-theme-name-xyz")
	assert.False(t, ok)
}

func TestCatalog_Resolve_prefersLocalOverride(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, NewCatalog(themesDir).InitBundled())

	// override dracula with custom colors
	orig, err := NewCatalog("").galleryTheme("dracula")
	require.NoError(t, err)
	orig.ChromaStyle = "monokai"
	orig.Colors["color-accent"] = "#010203"

	var buf bytes.Buffer
	require.NoError(t, orig.Dump(&buf))
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "dracula"), buf.Bytes(), 0o600))

	cat := NewCatalog(themesDir)
	th, ok := cat.Resolve("dracula")
	require.True(t, ok)
	assert.Equal(t, "monokai", th.ChromaStyle)
	assert.Equal(t, "#010203", th.Colors["color-accent"])
}

func Test_listOrdered(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, NewCatalog(themesDir).InitBundled())

	// add a local-only theme
	localContent := "# name: my-local\n# description: local theme\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-local"), []byte(localContent), 0o600))

	infos, err := NewCatalog(themesDir).Entries()
	require.NoError(t, err)
	require.NotEmpty(t, infos)

	// default theme must be first
	assert.Equal(t, defaultThemeName, infos[0].Name)
	assert.True(t, infos[0].InGallery)

	// local theme should appear before bundled themes
	var localIdx, firstBundledIdx int
	for i, info := range infos {
		if info.Name == "my-local" {
			localIdx = i
			assert.True(t, info.Local)
			assert.False(t, info.InGallery)
		}
		if info.Bundled && firstBundledIdx == 0 {
			firstBundledIdx = i
		}
	}
	assert.Positive(t, localIdx, "local theme should not be first (default is first)")
	assert.Less(t, localIdx, firstBundledIdx, "local themes should appear before bundled")
}

func Test_listOrdered_emptyDir(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	infos, err := NewCatalog(themesDir).Entries()
	require.NoError(t, err)
	// should still return gallery themes
	require.NotEmpty(t, infos)
	assert.Equal(t, defaultThemeName, infos[0].Name)
}

func Test_gallery(t *testing.T) {
	gallery, err := NewCatalog("").gallery()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(gallery), 5, "gallery should have at least 5 bundled themes")

	// verify all bundled themes are present and marked
	for _, name := range []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"} {
		th, ok := gallery[name]
		require.True(t, ok, "gallery should contain %q", name)
		assert.True(t, th.Bundled, "%q should be marked bundled", name)
		assert.NotEmpty(t, th.ChromaStyle)
		assert.GreaterOrEqual(t, len(th.Colors), 18, "should have at least 18 required color keys")
	}
}

func TestGalleryNames(t *testing.T) {
	names, err := NewCatalog("").galleryNames()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(names), 5)
	// verify sorted
	for i := 1; i < len(names); i++ {
		assert.Less(t, names[i-1], names[i], "gallery names should be sorted")
	}
}

func Test_galleryTheme(t *testing.T) {
	th, err := NewCatalog("").galleryTheme("dracula")
	require.NoError(t, err)
	assert.Equal(t, "dracula", th.Name)
	assert.True(t, th.Bundled)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.GreaterOrEqual(t, len(th.Colors), 18)
}

func Test_galleryTheme_notFound(t *testing.T) {
	_, err := NewCatalog("").galleryTheme("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func Test_galleryTheme_returnsClone(t *testing.T) {
	first, err := NewCatalog("").galleryTheme("dracula")
	require.NoError(t, err)
	first.Colors["color-accent"] = "#010203"

	second, err := NewCatalog("").galleryTheme("dracula")
	require.NoError(t, err)
	assert.NotEqual(t, "#010203", second.Colors["color-accent"])
}

func TestGallery_returnsDeepCopy(t *testing.T) {
	first, err := NewCatalog("").gallery()
	require.NoError(t, err)

	th := first["dracula"]
	th.Colors["color-accent"] = "#010203"
	first["dracula"] = th
	delete(first, "nord")

	second, err := NewCatalog("").gallery()
	require.NoError(t, err)
	assert.Contains(t, second, "nord")
	assert.NotEqual(t, "#010203", second["dracula"].Colors["color-accent"])
}

// TestGalleryThemes_validate validates that all gallery themes are well-formed.
// This serves as the CI validation test for community theme contributions.
func TestGalleryThemes_validate(t *testing.T) {
	gallery, err := NewCatalog("").gallery()
	require.NoError(t, err)

	for name, th := range gallery {
		t.Run(name, func(t *testing.T) {
			assert.NotEmpty(t, th.Name, "theme must have a name")
			assert.NotEmpty(t, th.Description, "theme must have a description")
			assert.NotEmpty(t, th.ChromaStyle, "theme must have a chroma-style")
			// verify chroma-style is a real chroma style, not the fallback for unknown names
			assert.NotEqual(t, styles.Fallback, styles.Get(th.ChromaStyle),
				"unknown chroma style %q in theme %s", th.ChromaStyle, name)
			assert.Equal(t, name, th.Name, "theme name must match filename")

			// all required color keys must be present (optional keys may be omitted)
			optional := NewCatalog("").OptionalColorKeys()
			for _, key := range colorKeys {
				if optional[key] {
					continue
				}
				assert.NotEmpty(t, th.Colors[key], "missing required color key %s", key)
			}
		})
	}
}
