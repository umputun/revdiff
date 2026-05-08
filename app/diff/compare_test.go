package diff

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

			r := NewCompareReader(f.Name(), f.Name())
			assert.Equal(t, tc.want, r.countFileLines())
		})
	}
}

func TestCountFileLines_MissingFile(t *testing.T) {
	r := NewCompareReader("/nonexistent/file.txt", "/nonexistent/file.txt")
	assert.Equal(t, 0, r.countFileLines())
}

// TestCompareReader_FileDiff_PathsWithColons confirms paths containing ':'
// (e.g. ISO timestamps) round-trip cleanly under the two-flag form. The OG
// --compare=old:new bug is killed by the flag shape change; this test pins it.
func TestCompareReader_FileDiff_PathsWithColons(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "2026-05-01T12:30:00.md")
	newPath := filepath.Join(dir, "2026-05-01T13:00:00.md")
	require.NoError(t, os.WriteFile(oldPath, []byte("alpha\nbeta\n"), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte("alpha\ngamma\n"), 0o600))

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
	assert.Equal(t, 1, adds)
	assert.Equal(t, 1, removes)
}

func TestCompareReader_FileDiff_BinaryFiles(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.bin")
	newPath := filepath.Join(dir, "new.bin")
	// embed NUL bytes so git classifies the files as binary
	require.NoError(t, os.WriteFile(oldPath, []byte{0x00, 0x01, 0x02, 0x03, 0x04}, 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte{0x00, 0x01, 0xFF, 0x03, 0x04}, 0o600))

	r := NewCompareReader(oldPath, newPath)
	lines, err := r.FileDiff("", "", false, 0)
	// git emits "Binary files X and Y differ" on stdout with exit 1; diffError
	// treats this as success and parseUnifiedDiff produces a single
	// "(binary file)" placeholder row (Partial=true) — no add/remove rows,
	// no @@ hunks. Contract under test: no error, exactly one placeholder.
	require.NoError(t, err)
	require.Len(t, lines, 1, "binary diff should produce exactly one placeholder row")
	assert.Contains(t, strings.ToLower(lines[0].Content), "binary",
		"placeholder content should mark the row as binary")
	for _, l := range lines {
		assert.NotEqual(t, ChangeAdd, l.ChangeType)
		assert.NotEqual(t, ChangeRemove, l.ChangeType)
	}
}

func TestCompareReader_FileDiff_NoTrailingNewline(t *testing.T) {
	cases := []struct {
		name       string
		oldContent string
		newContent string
		wantAdd    int
		wantRemove int
	}{
		{"both with newline", "a\nb\n", "a\nc\n", 1, 1},
		{"both without newline", "a\nb", "a\nc", 1, 1},
		{"old without, new with", "a\nb", "a\nc\n", 1, 1},
		{"old with, new without", "a\nb\n", "a\nc", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			oldPath := filepath.Join(dir, "old.txt")
			newPath := filepath.Join(dir, "new.txt")
			require.NoError(t, os.WriteFile(oldPath, []byte(tc.oldContent), 0o600))
			require.NoError(t, os.WriteFile(newPath, []byte(tc.newContent), 0o600))

			r := NewCompareReader(oldPath, newPath)
			lines, err := r.FileDiff("", "", false, 0)
			require.NoError(t, err)

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
			assert.Equal(t, tc.wantAdd, adds)
			assert.Equal(t, tc.wantRemove, removes)
		})
	}
}

func TestCompareReader_FileDiff_GitErrorSurfacesStderr(t *testing.T) {
	// missing files trigger git diff --no-index to fail with a fatal error on
	// stderr; diffError must surface that text rather than fall back to the
	// bare "exit N" branch.
	r := NewCompareReader("/nonexistent/old-only-here.txt", "/nonexistent/new-only-here.txt")
	_, err := r.FileDiff("", "", false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git diff --no-index")
	// the error must carry stderr content. accept either git's path-echo or a
	// recognizable fatal/error token; explicitly NOT the bare exit-N fallback.
	msg := err.Error()
	hasStderr := strings.Contains(msg, "old-only-here") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "fatal:") ||
		strings.Contains(msg, "error:")
	assert.True(t, hasStderr, "git error should surface stderr content, got: %s", msg)
}

func TestCompareReader_FileDiff_Symlinks(t *testing.T) {
	dir := t.TempDir()
	realOld := filepath.Join(dir, "real-old.txt")
	realNew := filepath.Join(dir, "real-new.txt")
	require.NoError(t, os.WriteFile(realOld, []byte("alpha\n"), 0o600))
	require.NoError(t, os.WriteFile(realNew, []byte("beta\n"), 0o600))

	linkOld := filepath.Join(dir, "link-old.txt")
	linkNew := filepath.Join(dir, "link-new.txt")
	require.NoError(t, os.Symlink(realOld, linkOld))
	require.NoError(t, os.Symlink(realNew, linkNew))

	countAddRemove := func(t *testing.T, lines []DiffLine) (adds, removes int) {
		t.Helper()
		for _, l := range lines {
			switch l.ChangeType {
			case ChangeAdd:
				adds++
			case ChangeRemove:
				removes++
			case ChangeContext, ChangeDivider:
			}
		}
		return adds, removes
	}

	// git --no-index treats symlinks as symlink objects (mode 120000),
	// not as their targets. Both-symlinks case diffs target paths
	// (real-old.txt → real-new.txt). Mixed case yields two file diffs
	// (delete the symlink object + add the regular file). Both produce
	// at least one add and one remove; assert that, not exact counts.
	t.Run("both symlinks", func(t *testing.T) {
		r := NewCompareReader(linkOld, linkNew)
		lines, err := r.FileDiff("", "", false, 0)
		require.NoError(t, err)
		adds, removes := countAddRemove(t, lines)
		assert.GreaterOrEqual(t, adds, 1, "should produce at least one add")
		assert.GreaterOrEqual(t, removes, 1, "should produce at least one remove")
	})

	t.Run("mixed symlink and regular", func(t *testing.T) {
		r := NewCompareReader(linkOld, realNew)
		lines, err := r.FileDiff("", "", false, 0)
		require.NoError(t, err)
		adds, removes := countAddRemove(t, lines)
		assert.GreaterOrEqual(t, adds, 1)
		assert.GreaterOrEqual(t, removes, 1)
	})
}

func TestCompareReader_CountFileLines_LargeFileStreaming(t *testing.T) {
	// Build a >5MB file. countFileLines uses a 32KB buffer, so HeapAlloc
	// should not grow by anywhere near the file size.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	f, err := os.Create(path) //nolint:gosec // test path from t.TempDir()
	require.NoError(t, err)
	const targetLines = 200_000 // ~5MB at "line N\n" sizing
	var line [24]byte
	copy(line[:], "0123456789abcdefghijklmn")
	line[23] = '\n'
	for range targetLines {
		_, werr := f.Write(line[:])
		require.NoError(t, werr)
	}
	require.NoError(t, f.Close())

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(4_000_000), "file should be >4MB to make the streaming test meaningful")

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	r := NewCompareReader(path, path)
	got := r.countFileLines()

	runtime.ReadMemStats(&after)

	assert.Equal(t, targetLines, got)

	// HeapAlloc growth should be far below file size — the read buffer is 32KB
	// plus modest GC noise. Cap at file size / 4 as a generous smoke threshold.
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	maxGrowth := uint64(info.Size()) / 4 //nolint:gosec // info.Size() guarded > 4_000_000 above
	assert.Less(t, growth, maxGrowth,
		"countFileLines should stream; heap grew by %d bytes for a %d byte file", growth, info.Size())
}
