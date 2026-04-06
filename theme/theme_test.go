package theme

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullThemeColors returns all 21 color key=value lines for use in test theme strings.
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

// fullThemeColorsMap returns all 21 color key-value pairs as a map.
func fullThemeColorsMap() map[string]string {
	return map[string]string{
		"color-accent": "#bd93f9", "color-border": "#6272a4", "color-normal": "#f8f8f2",
		"color-muted": "#6272a4", "color-selected-fg": "#f8f8f2", "color-selected-bg": "#44475a",
		"color-annotation": "#f1fa8c", "color-cursor-fg": "#282a36", "color-cursor-bg": "#f8f8f2",
		"color-add-fg": "#50fa7b", "color-add-bg": "#2a4a2a", "color-remove-fg": "#ff5555",
		"color-remove-bg": "#4a2a2a", "color-modify-fg": "#ffb86c", "color-modify-bg": "#3a3a2a",
		"color-tree-bg": "#21222c", "color-diff-bg": "#282a36", "color-status-fg": "#f8f8f2",
		"color-status-bg": "#44475a", "color-search-fg": "#282a36", "color-search-bg": "#f1fa8c",
	}
}

func TestParse_validTheme(t *testing.T) {
	input := "# name: dracula\n# description: purple accent, vibrant colors\n\nchroma-style = dracula\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "dracula", th.Name)
	assert.Equal(t, "purple accent, vibrant colors", th.Description)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
	assert.Equal(t, "#6272a4", th.Colors["color-border"])
	assert.Equal(t, "#f8f8f2", th.Colors["color-normal"])
	assert.Len(t, th.Colors, 21)
}

func TestParse_missingMetadata(t *testing.T) {
	input := "chroma-style = nord\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, th.Name)
	assert.Empty(t, th.Description)
	assert.Equal(t, "nord", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
}

func TestParse_emptyInput(t *testing.T) {
	_, err := Parse(strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required key: chroma-style")
}

func TestParse_commentsOnly(t *testing.T) {
	input := `# name: test
# description: just comments
# random comment
`
	_, err := Parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required key: chroma-style")
}

func TestParse_malformedLine(t *testing.T) {
	input := `color-accent = #bd93f9
this is not valid
`
	_, err := Parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed line")
}

func TestParse_blankLinesAndSpacing(t *testing.T) {
	input := "\n  # name: spaced\n\n  chroma-style = monokai\n\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "spaced", th.Name)
	assert.Equal(t, "monokai", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
}

func TestParse_missingChromaStyle(t *testing.T) {
	input := fullThemeColors()
	_, err := Parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required key: chroma-style")
}

func TestParse_ignoresNonColorKeys(t *testing.T) {
	input := "something-else = value\nchroma-style = monokai\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
	_, exists := th.Colors["something-else"]
	assert.False(t, exists, "non-color keys should be ignored")
}

func TestDump_withMetadata(t *testing.T) {
	colors := map[string]string{
		"color-accent": "#bd93f9",
		"color-border": "#6272a4",
	}
	var buf bytes.Buffer
	err := Dump(Theme{Name: "dracula", Description: "purple accent theme", ChromaStyle: "dracula", Colors: colors}, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "# name: dracula")
	assert.Contains(t, output, "# description: purple accent theme")
	assert.Contains(t, output, "chroma-style = dracula")
	assert.Contains(t, output, "color-accent = #bd93f9")
	assert.Contains(t, output, "color-border = #6272a4")
}

func TestDump_withoutMetadata(t *testing.T) {
	colors := map[string]string{"color-accent": "#aaa"}
	var buf bytes.Buffer
	err := Dump(Theme{Colors: colors}, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "# name:")
	assert.NotContains(t, output, "# description:")
	assert.NotContains(t, output, "chroma-style")
	assert.Contains(t, output, "color-accent = #aaa")
}

func TestDump_canonicalOrder(t *testing.T) {
	colors := map[string]string{
		"color-search-bg": "#111",
		"color-accent":    "#222",
		"color-border":    "#333",
	}
	var buf bytes.Buffer
	err := Dump(Theme{Colors: colors}, &buf)
	require.NoError(t, err)

	output := buf.String()
	accentIdx := strings.Index(output, "color-accent")
	borderIdx := strings.Index(output, "color-border")
	searchIdx := strings.Index(output, "color-search-bg")
	assert.Less(t, accentIdx, borderIdx, "accent should come before border")
	assert.Less(t, borderIdx, searchIdx, "border should come before search-bg")
}

func TestDump_Parse_roundtrip(t *testing.T) {
	colors := fullThemeColorsMap()
	var buf bytes.Buffer
	err := Dump(Theme{Name: "my-theme", Description: "test roundtrip", ChromaStyle: "dracula", Colors: colors}, &buf)
	require.NoError(t, err)

	th, err := Parse(strings.NewReader(buf.String()))
	require.NoError(t, err)
	assert.Equal(t, "my-theme", th.Name)
	assert.Equal(t, "test roundtrip", th.Description)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Equal(t, colors, th.Colors)
}

func TestLoad_validFile(t *testing.T) {
	dir := t.TempDir()
	content := "# name: test-theme\n# description: a test\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test-theme"), []byte(content), 0o600))

	th, err := Load("test-theme", dir)
	require.NoError(t, err)
	assert.Equal(t, "test-theme", th.Name)
	assert.Equal(t, "a test", th.Description)
	assert.Equal(t, "monokai", th.ChromaStyle)
	assert.Equal(t, "#bd93f9", th.Colors["color-accent"])
	assert.Len(t, th.Colors, 21)
}

func TestParse_missingColorKeys(t *testing.T) {
	input := "chroma-style = dracula\ncolor-accent = #bd93f9\ncolor-border = #6272a4\n"
	_, err := Parse(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "theme missing required keys")
	assert.Contains(t, err.Error(), "color-normal")
}

func TestLoad_missingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load("nonexistent", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening theme")
}

func TestLoad_invalidContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad"), []byte("not a valid line\n"), 0o600))

	_, err := Load("bad", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing theme")
}

func TestLoad_pathTraversal(t *testing.T) {
	dir := t.TempDir()
	tests := []struct{ name string }{
		{name: "../etc/passwd"}, {name: "../../secret"}, {name: "/etc/passwd"}, {name: "."}, {name: ".."},
		{name: "sub/theme"}, {name: "a/../b"},
	}
	for _, tc := range tests {
		_, err := Load(tc.name, dir)
		require.Error(t, err, "name=%q should be rejected", tc.name)
		assert.Contains(t, err.Error(), "invalid theme name", "name=%q", tc.name)
	}
}

func TestList_multipleThemes(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"nord", "dracula", "solarized-dark"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("color-accent = #000\n"), 0o600))
	}

	names, err := List(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord", "solarized-dark"}, names)
}

func TestList_emptyDir(t *testing.T) {
	dir := t.TempDir()
	names, err := List(dir)
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestList_nonExistentDir(t *testing.T) {
	names, err := List(filepath.Join(t.TempDir(), "nonexistent"))
	require.NoError(t, err)
	assert.Nil(t, names)
}

func TestList_skipsDirectories(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mytheme"), []byte("color-accent = #000\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o750))

	names, err := List(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"mytheme"}, names)
}

func TestInitBundled_createsDirAndFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := InitBundled(dir)
	require.NoError(t, err)

	names, err := List(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"catppuccin-mocha", "dracula", "gruvbox", "nord", "solarized-dark"}, names)

	// verify files are non-empty
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test uses temp dir
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	}
}

func TestInitBundled_preservesUserThemes(t *testing.T) {
	dir := t.TempDir()
	userTheme := filepath.Join(dir, "my-custom")
	require.NoError(t, os.WriteFile(userTheme, []byte("color-accent = #111111\n"), 0o600))

	err := InitBundled(dir)
	require.NoError(t, err)

	// user theme should be untouched
	data, err := os.ReadFile(userTheme) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Equal(t, "color-accent = #111111\n", string(data))

	// bundled themes should exist
	names, err := List(dir)
	require.NoError(t, err)
	assert.Contains(t, names, "dracula")
	assert.Contains(t, names, "my-custom")
}

func TestInitBundled_overwritesBundledThemes(t *testing.T) {
	dir := t.TempDir()
	// write a fake "dracula" file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dracula"), []byte("old content\n"), 0o600))

	err := InitBundled(dir)
	require.NoError(t, err)

	// dracula should be overwritten with bundled content
	data, err := os.ReadFile(filepath.Join(dir, "dracula")) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Contains(t, string(data), "# name: dracula")
	assert.NotContains(t, string(data), "old content")
}

func TestBundledNames(t *testing.T) {
	names := BundledNames()
	assert.Equal(t, []string{"catppuccin-mocha", "dracula", "gruvbox", "nord", "solarized-dark"}, names)
}

func TestBundledThemes_parseCorrectly(t *testing.T) {
	for _, name := range BundledNames() {
		t.Run(name, func(t *testing.T) {
			content, ok := bundledThemes[name]
			require.True(t, ok)

			th, err := Parse(strings.NewReader(content))
			require.NoError(t, err)
			assert.Equal(t, name, th.Name)
			assert.NotEmpty(t, th.Description)
			assert.NotEmpty(t, th.ChromaStyle)

			// verify all 21 color keys are present
			for _, key := range ColorKeys() {
				assert.NotEmpty(t, th.Colors[key], "missing color key %s in theme %s", key, name)
			}
		})
	}
}

func TestColorKeys(t *testing.T) {
	keys := ColorKeys()
	assert.Len(t, keys, 21)
	assert.Equal(t, "color-accent", keys[0])
	assert.Equal(t, "color-search-bg", keys[len(keys)-1])

	// verify it returns a copy
	keys[0] = "modified"
	assert.Equal(t, "color-accent", ColorKeys()[0])
}
