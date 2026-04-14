package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchConfigTheme_existingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("wrap = true\ntheme = dracula\nblame = false\n"), 0o600))

	err := patchConfigTheme(path, "nord")
	require.NoError(t, err)

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

	err := patchConfigTheme(path, "nord")
	require.NoError(t, err)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "theme = nord")
	assert.Contains(t, string(data), "wrap = true")
}

func TestPatchConfigTheme_createsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	err := patchConfigTheme(path, "dracula")
	require.NoError(t, err)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Equal(t, "theme = dracula\n", string(data))
}

func TestPatchConfigTheme_createsParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config")

	err := patchConfigTheme(path, "dracula")
	require.NoError(t, err)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Equal(t, "theme = dracula\n", string(data))
}

func TestPatchConfigTheme_skipsCommentedOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("# theme = dracula\nwrap = true\n"), 0o600))

	err := patchConfigTheme(path, "nord")
	require.NoError(t, err)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "# theme = dracula", "commented line should be preserved")
	assert.Contains(t, string(data), "theme = nord", "new theme line should be appended")
}

func TestPatchConfigTheme_semicolonComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte("; theme = dracula\n"), 0o600))

	err := patchConfigTheme(path, "nord")
	require.NoError(t, err)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "; theme = dracula")
	assert.Contains(t, string(data), "theme = nord")
}

func TestPatchConfigTheme_rejectsNewlines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	tests := []struct {
		name      string
		themeName string
	}{
		{name: "newline", themeName: "bad\ntheme"},
		{name: "carriage return", themeName: "bad\rtheme"},
		{name: "crlf", themeName: "bad\r\ntheme"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := patchConfigTheme(path, tc.themeName)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must not contain newlines")
		})
	}
}

func TestPatchConfigTheme_preservesFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := "wrap = true\n\n# Colors\ncolor-accent = #ff0000\ntheme = old\n\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	err := patchConfigTheme(path, "new")
	require.NoError(t, err)

	data, err := os.ReadFile(path) //nolint:gosec // test
	require.NoError(t, err)
	assert.Contains(t, string(data), "theme = new")
	assert.Contains(t, string(data), "# Colors")
	assert.Contains(t, string(data), "color-accent = #ff0000")
}
