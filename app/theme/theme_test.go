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

func TestParse_authorAndBundled(t *testing.T) {
	input := "# name: custom\n# description: test\n# author: Jane Doe\n# bundled: true\n\nchroma-style = dracula\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe", th.Author)
	assert.True(t, th.Bundled)
}

func TestParse_bundledFalse(t *testing.T) {
	input := "# name: community\n# bundled: false\nchroma-style = dracula\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.False(t, th.Bundled)
}

func TestParse_noBundledField(t *testing.T) {
	input := "# name: community\nchroma-style = dracula\n" + fullThemeColors()
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.False(t, th.Bundled, "absent bundled field should default to false")
}

func TestDump_authorAndBundled(t *testing.T) {
	colors := map[string]string{"color-accent": "#bd93f9"}
	var buf bytes.Buffer
	err := Dump(Theme{Name: "test", Author: "Jane Doe", Bundled: true, Colors: colors}, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "# author: Jane Doe")
	assert.Contains(t, output, "# bundled: true")
}

func TestDump_noBundledWhenFalse(t *testing.T) {
	colors := map[string]string{"color-accent": "#bd93f9"}
	var buf bytes.Buffer
	err := Dump(Theme{Name: "test", Colors: colors}, &buf)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "bundled")
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
	assert.Contains(t, err.Error(), "not found")
}

func TestLoad_fallbackToGallery(t *testing.T) {
	dir := t.TempDir() // empty dir, no local files
	th, err := Load("dracula", dir)
	require.NoError(t, err, "should fall back to embedded gallery")
	assert.Equal(t, "dracula", th.Name)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.GreaterOrEqual(t, len(th.Colors), 18)
}

func TestLoad_localOverridesGallery(t *testing.T) {
	dir := t.TempDir()
	// write a customized "dracula" with different accent
	content := "# name: dracula\n# description: customized\nchroma-style = dracula\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dracula"), []byte(content), 0o600))

	th, err := Load("dracula", dir)
	require.NoError(t, err)
	assert.Equal(t, "customized", th.Description, "local file should override gallery")
}

func TestLoad_emptyThemesDir(t *testing.T) {
	// with empty themesDir, should still fall back to gallery
	th, err := Load("nord", "")
	require.NoError(t, err)
	assert.Equal(t, "nord", th.Name)
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
	assert.Equal(t, []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)

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
	assert.Equal(t, []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)
}

func TestBundledThemes_parseCorrectly(t *testing.T) {
	gallery, err := Gallery()
	require.NoError(t, err)

	for _, name := range BundledNames() {
		t.Run(name, func(t *testing.T) {
			th, ok := gallery[name]
			require.True(t, ok)
			assert.True(t, th.Bundled, "bundled theme %q should have bundled: true", name)
			assert.Equal(t, name, th.Name)
			assert.NotEmpty(t, th.Description)
			assert.NotEmpty(t, th.ChromaStyle)

			// verify all required color keys are present (optional keys may be omitted)
			optional := OptionalColorKeys()
			for _, key := range ColorKeys() {
				if optional[key] {
					continue
				}
				assert.NotEmpty(t, th.Colors[key], "missing required color key %s in theme %s", key, name)
			}
		})
	}
}

func TestInstallFile(t *testing.T) {
	// create a valid theme file
	srcDir := t.TempDir()
	content := "# name: my-custom\n# description: a custom theme\n# author: Test User\nchroma-style = monokai\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "my-custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	name, err := InstallFile(destDir, srcPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-custom", name)

	// verify file was written
	names, err := List(destDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"my-custom"}, names)

	// verify it's a valid theme
	th, err := Load("my-custom", destDir)
	require.NoError(t, err)
	assert.Equal(t, "my-custom", th.Name)
	assert.Equal(t, "Test User", th.Author)
}

func TestInstallFile_invalidTheme(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "bad-theme")
	require.NoError(t, os.WriteFile(srcPath, []byte("not valid\n"), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	_, err := InstallFile(destDir, srcPath, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing theme file")
}

func TestInstallFile_notFound(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "themes")
	_, err := InstallFile(destDir, "/nonexistent/path/theme", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening theme file")
}

func TestInstallFile_dotPaths(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "themes")
	for _, path := range []string{".", ".."} {
		_, err := InstallFile(destDir, path, nil)
		require.Error(t, err, "path=%q should be rejected", path)
		assert.Contains(t, err.Error(), "invalid theme file name", "path=%q", path)
	}
}

func TestInstallFile_invalidChromaStyle(t *testing.T) {
	srcDir := t.TempDir()
	content := "# name: bad-chroma\n# description: theme with bad chroma\nchroma-style = nonexistent-style\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "bad-chroma")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")

	// with validator that rejects the style
	rejectAll := func(string) bool { return false }
	_, err := InstallFile(destDir, srcPath, rejectAll)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown chroma style")

	// without validator (nil) — should succeed
	name, err := InstallFile(destDir, srcPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "bad-chroma", name)
}

func TestInstallFile_setsNameFromFilenameWhenMissing(t *testing.T) {
	srcDir := t.TempDir()
	content := "# description: no explicit name\nchroma-style = monokai\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "my-custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	name, err := InstallFile(destDir, srcPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-custom", name)

	th, err := Load("my-custom", destDir)
	require.NoError(t, err)
	assert.Equal(t, "my-custom", th.Name)
}

func TestInstallFile_rejectsMetadataNameMismatch(t *testing.T) {
	srcDir := t.TempDir()
	content := "# name: other-name\n# description: mismatch\nchroma-style = monokai\n" + fullThemeColors()
	srcPath := filepath.Join(srcDir, "my-custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	_, err := InstallFile(destDir, srcPath, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `theme metadata name "other-name" does not match file name "my-custom"`)
}

func TestInstall_validatesGalleryNamesBeforeWriting(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "my-custom")
	content := "# name: my-custom\n# description: a custom theme\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	destDir := filepath.Join(t.TempDir(), "themes")
	var out bytes.Buffer

	err := Install([]string{srcPath, "nonexistent"}, destDir, nil, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `theme "nonexistent" not found in gallery`)

	names, listErr := List(destDir)
	require.NoError(t, listErr)
	assert.Empty(t, names, "validation failure should prevent partial installs")
	assert.Empty(t, out.String())
}

func TestPrintList(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, InitBundled(themesDir))

	localContent := "# name: my-local\n# description: local theme\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-local"), []byte(localContent), 0o600))

	var out bytes.Buffer
	err := PrintList(themesDir, &out)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.NotEmpty(t, lines)
	assert.Equal(t, DefaultThemeName, lines[0])
	assert.Contains(t, lines, "my-local")
	assert.Contains(t, lines, "dracula")
}

func TestActiveName(t *testing.T) {
	assert.Equal(t, DefaultThemeName, ActiveName(""))
	assert.Equal(t, "dracula", ActiveName("dracula"))
}

func TestIsLocalPath(t *testing.T) {
	assert.True(t, IsLocalPath("themes/custom"))
	assert.True(t, IsLocalPath("./custom"))
	assert.False(t, IsLocalPath("dracula"))
}

func TestListOrdered(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, InitBundled(themesDir))

	// add a local-only theme
	localContent := "# name: my-local\n# description: local theme\nchroma-style = monokai\n" + fullThemeColors()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-local"), []byte(localContent), 0o600))

	infos, err := ListOrdered(themesDir)
	require.NoError(t, err)
	require.NotEmpty(t, infos)

	// default theme must be first
	assert.Equal(t, DefaultThemeName, infos[0].Name)
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

func TestListOrdered_emptyDir(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	infos, err := ListOrdered(themesDir)
	require.NoError(t, err)
	// should still return gallery themes
	require.NotEmpty(t, infos)
	assert.Equal(t, DefaultThemeName, infos[0].Name)
}

func TestLoad_permissionError(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "secret-theme")
	require.NoError(t, os.WriteFile(fpath, []byte("chroma-style = dracula\n"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(fpath, 0o600) }) // restore so TempDir cleanup works

	_, err := Load("secret-theme", dir)
	require.Error(t, err, "should not silently fall back to gallery on permission error")
	assert.Contains(t, err.Error(), "opening theme")
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
			err := validateHexColor(tc.color)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid hex color")
			} else {
				require.NoError(t, err)
			}
		})
	}
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
			_, err := Parse(strings.NewReader(input))
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
	th, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Len(t, th.Colors, 18) // 21 - 3 optional
	assert.Empty(t, th.Colors["color-cursor-bg"])
	assert.Empty(t, th.Colors["color-tree-bg"])
	assert.Empty(t, th.Colors["color-diff-bg"])
}

func TestDump_Parse_roundtrip_withoutOptionalKeys(t *testing.T) {
	// dump a theme without optional keys, then parse it back
	colors := fullThemeColorsMap()
	delete(colors, "color-cursor-bg")
	delete(colors, "color-tree-bg")
	delete(colors, "color-diff-bg")

	var buf bytes.Buffer
	err := Dump(Theme{Name: "minimal", ChromaStyle: "dracula", Colors: colors}, &buf)
	require.NoError(t, err)

	th, err := Parse(strings.NewReader(buf.String()))
	require.NoError(t, err)
	assert.Equal(t, "minimal", th.Name)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.Len(t, th.Colors, 18)
	assert.Equal(t, colors, th.Colors)
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

func TestOptionalColorKeys(t *testing.T) {
	opt := OptionalColorKeys()
	assert.Len(t, opt, 3)
	assert.True(t, opt["color-cursor-bg"])
	assert.True(t, opt["color-tree-bg"])
	assert.True(t, opt["color-diff-bg"])

	// verify it returns a copy
	opt["color-accent"] = true
	assert.False(t, OptionalColorKeys()["color-accent"])
}

func TestGallery(t *testing.T) {
	gallery, err := Gallery()
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
	names, err := GalleryNames()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(names), 5)
	// verify sorted
	for i := 1; i < len(names); i++ {
		assert.Less(t, names[i-1], names[i], "gallery names should be sorted")
	}
}

func TestGalleryTheme(t *testing.T) {
	th, err := GalleryTheme("dracula")
	require.NoError(t, err)
	assert.Equal(t, "dracula", th.Name)
	assert.True(t, th.Bundled)
	assert.Equal(t, "dracula", th.ChromaStyle)
	assert.GreaterOrEqual(t, len(th.Colors), 18)
}

func TestGalleryTheme_notFound(t *testing.T) {
	_, err := GalleryTheme("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGalleryTheme_returnsClone(t *testing.T) {
	first, err := GalleryTheme("dracula")
	require.NoError(t, err)
	first.Colors["color-accent"] = "#010203"

	second, err := GalleryTheme("dracula")
	require.NoError(t, err)
	assert.NotEqual(t, "#010203", second.Colors["color-accent"])
}

func TestGallery_returnsDeepCopy(t *testing.T) {
	first, err := Gallery()
	require.NoError(t, err)

	th := first["dracula"]
	th.Colors["color-accent"] = "#010203"
	first["dracula"] = th
	delete(first, "nord")

	second, err := Gallery()
	require.NoError(t, err)
	assert.Contains(t, second, "nord")
	assert.NotEqual(t, "#010203", second["dracula"].Colors["color-accent"])
}

func TestInitAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := InitAll(dir)
	require.NoError(t, err)

	names, err := List(dir)
	require.NoError(t, err)

	galleryNames, err := GalleryNames()
	require.NoError(t, err)
	assert.Equal(t, galleryNames, names, "InitAll should write all gallery themes")
}

func TestInitNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := InitNames(dir, []string{"dracula", "nord"})
	require.NoError(t, err)

	names, err := List(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord"}, names)
}

func TestInitNames_notInGallery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	err := InitNames(dir, []string{"nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in gallery")
}

func TestInitAll_preservesUserThemes(t *testing.T) {
	dir := t.TempDir()
	userTheme := filepath.Join(dir, "my-custom")
	require.NoError(t, os.WriteFile(userTheme, []byte("user content\n"), 0o600))

	err := InitAll(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(userTheme) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Equal(t, "user content\n", string(data))
}

func TestWriteThemeFile_atomic(t *testing.T) {
	dir := t.TempDir()
	th, err := GalleryTheme("dracula")
	require.NoError(t, err)

	require.NoError(t, writeThemeFile(dir, "dracula", th))

	data, err := os.ReadFile(filepath.Join(dir, "dracula")) //nolint:gosec // test uses temp dir
	require.NoError(t, err)
	assert.Contains(t, string(data), "# name: dracula")

	matches, err := filepath.Glob(filepath.Join(dir, "dracula.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

// TestGalleryThemes_validate validates that all gallery themes are well-formed.
// This serves as the CI validation test for community theme contributions.
func TestGalleryThemes_validate(t *testing.T) {
	gallery, err := Gallery()
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
			optional := OptionalColorKeys()
			for _, key := range ColorKeys() {
				if optional[key] {
					continue
				}
				assert.NotEmpty(t, th.Colors[key], "missing required color key %s", key)
			}
		})
	}
}
