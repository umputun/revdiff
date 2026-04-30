package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompareReader_ChangedFiles(t *testing.T) {
	dir := t.TempDir()
	r := NewCompareReader(filepath.Join(dir, "old.md"), filepath.Join(dir, "new.md"))
	entries, err := r.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "new.md", entries[0].Path)
}

func TestCompareReader_FileDiff_Normal(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("line1\nline2\nline3\n"), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte("line1\nchanged\nline3\n"), 0o600))

	r := NewCompareReader(oldPath, newPath)
	lines, err := r.FileDiff("", "", false, 0)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	var adds, removes int
	for _, l := range lines {
		switch l.ChangeType {
		case ChangeAdd:
			adds++
		case ChangeRemove:
			removes++
		case ChangeContext, ChangeDivider:
		}
	}
	assert.Equal(t, 1, adds, "expected 1 added line")
	assert.Equal(t, 1, removes, "expected 1 removed line")
}

func TestCompareReader_FileDiff_Identical(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "a.txt")
	newPath := filepath.Join(dir, "b.txt")
	content := "hello\nworld\n"
	require.NoError(t, os.WriteFile(oldPath, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte(content), 0o600))

	r := NewCompareReader(oldPath, newPath)
	lines, err := r.FileDiff("", "", false, 0)
	require.NoError(t, err)
	// identical files produce empty diff output
	assert.Empty(t, lines)
}

func TestCompareReader_FileDiff_CompactMode(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")

	// 10-line file; change only line 5 so compact mode (contextLines=3) produces dividers
	oldContent := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	newContent := "1\n2\n3\n4\nFIVE\n6\n7\n8\n9\n10\n"
	require.NoError(t, os.WriteFile(oldPath, []byte(oldContent), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte(newContent), 0o600))

	r := NewCompareReader(oldPath, newPath)
	lines, err := r.FileDiff("", "", false, 3)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	var adds, removes int
	var dividers []string
	for _, l := range lines {
		switch l.ChangeType {
		case ChangeAdd:
			adds++
		case ChangeRemove:
			removes++
		case ChangeDivider:
			dividers = append(dividers, l.Content)
		case ChangeContext:
		}
	}
	assert.Equal(t, 1, adds)
	assert.Equal(t, 1, removes)
	// with contextLines=3 and a 10-line file, expect leading and/or trailing dividers
	assert.NotEmpty(t, dividers, "compact mode should produce divider lines")
}

func TestCompareReader_FileDiff_FullFileMode(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("aaa\nbbb\n"), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte("aaa\nccc\n"), 0o600))

	r := NewCompareReader(oldPath, newPath)
	// contextLines=0 means full-file mode (no compact dividers)
	lines, err := r.FileDiff("", "", false, 0)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	for _, l := range lines {
		assert.NotEqual(t, ChangeDivider, l.ChangeType, "full-file mode should have no dividers")
	}
}

func TestCompareReader_FileDiff_ErrorMissingFile(t *testing.T) {
	r := NewCompareReader("/nonexistent/old.txt", "/nonexistent/new.txt")
	_, err := r.FileDiff("", "", false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git diff --no-index")
}

func TestCountFileLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty", "", 0},
		{"one line with newline", "hello\n", 1},
		{"one line without newline", "hello", 1},
		{"three lines", "a\nb\nc\n", 3},
		{"three lines no trailing newline", "a\nb\nc", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*.txt")
			require.NoError(t, err)
			_, err = f.WriteString(tc.content)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			got := countFileLines(f.Name())
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCountFileLines_MissingFile(t *testing.T) {
	assert.Equal(t, 0, countFileLines("/nonexistent/file.txt"))
}
