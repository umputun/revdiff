package theme

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_Entries(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, InitBundled(themesDir))

	cat := NewCatalog(themesDir)
	entries, err := cat.Entries()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 5, "should have at least gallery themes")

	// default theme should be first
	require.NotEmpty(t, entries)
	assert.Equal(t, DefaultThemeName, entries[0].Name)
}

func TestCatalog_Entries_includesLocalThemes(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, InitBundled(themesDir))

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
	require.NoError(t, InitBundled(themesDir))

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
	require.NoError(t, InitBundled(themesDir))

	// override dracula with custom colors
	orig, err := GalleryTheme("dracula")
	require.NoError(t, err)
	orig.ChromaStyle = "monokai"
	orig.Colors["color-accent"] = "#010203"

	var buf bytes.Buffer
	require.NoError(t, Dump(orig, &buf))
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "dracula"), buf.Bytes(), 0o600))

	cat := NewCatalog(themesDir)
	th, ok := cat.Resolve("dracula")
	require.True(t, ok)
	assert.Equal(t, "monokai", th.ChromaStyle)
	assert.Equal(t, "#010203", th.Colors["color-accent"])
}
