package review

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestSafeWorkDirPath(t *testing.T) {
	// real workDir is required because EvalSymlinks resolves both sides;
	// non-existent fake paths like "/repo" would short-circuit every test.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ui"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package x\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ui", "model.go"), []byte("package x\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o600))

	tests := []struct {
		name    string
		workDir string
		relPath string
		ok      bool
	}{
		{name: "empty workDir is rejected", workDir: "", relPath: "x", ok: false},
		{name: "plain relative inside workDir ok", workDir: root, relPath: "main.go", ok: true},
		{name: "subdirectory relative ok", workDir: root, relPath: "ui/model.go", ok: true},
		{name: "dot-slash prefix ok", workDir: root, relPath: "./README.md", ok: true},
		{name: "parent traversal rejected", workDir: root, relPath: "../etc/passwd", ok: false},
		{name: "deep parent traversal rejected", workDir: root, relPath: "a/b/../../../../etc/passwd", ok: false},
		{name: "absolute path rejected", workDir: root, relPath: "/etc/passwd", ok: false},
		{name: "non-existent path rejected", workDir: root, relPath: "missing.go", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realWorkDir := resolveWorkDir(tt.workDir)
			full, ok := safeWorkDirPath(tt.workDir, realWorkDir, tt.relPath)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, filepath.Join(tt.workDir, tt.relPath), full,
					"accepted result must be Join(workDir, relPath), not the symlink-resolved form")
			}
		})
	}
}

func TestSafeWorkDirPath_SymlinkEscapeRejected(t *testing.T) {
	// symlink inside workDir pointing OUT of workDir must be rejected:
	// lexical Rel comparison would accept the link, but EvalSymlinks resolves
	// the real target and the rel check then catches the escape.
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret\n"), 0o600))
	link := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), link))

	_, ok := safeWorkDirPath(root, resolveWorkDir(root), "escape")
	assert.False(t, ok, "symlink target is outside workDir; must be rejected")
}

func TestSafeWorkDirPath_SymlinkInsideWorkDirAccepted(t *testing.T) {
	// symlink whose target is still under workDir must be accepted.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "real.go"), []byte("package x\n"), 0o600))
	link := filepath.Join(root, "alias.go")
	require.NoError(t, os.Symlink(filepath.Join(root, "real.go"), link))

	_, ok := safeWorkDirPath(root, resolveWorkDir(root), "alias.go")
	assert.True(t, ok, "symlink staying inside workDir must be accepted")
}

// resolveWorkDir mirrors what ComputeStats does once at the top of its loop.
// Tests that exercise safeWorkDirPath go through this helper so the test API
// matches the production call shape.
func resolveWorkDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	r, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return ""
	}
	return r
}

// fakeDiffer is a stub FileDiffer for ComputeStats tests.
type fakeDiffer struct {
	fn func(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error)
}

func (f fakeDiffer) FileDiff(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error) {
	return f.fn(ref, file, staged, contextLines)
}

func TestComputeStats_AggregatesAddsAndRemoves(t *testing.T) {
	entries := []diff.FileEntry{
		{Path: "a.go", Status: diff.FileModified},
		{Path: "b.go", Status: diff.FileModified},
	}
	differ := fakeDiffer{fn: func(string, string, bool, int) ([]diff.DiffLine, error) {
		return []diff.DiffLine{
			{ChangeType: diff.ChangeAdd},
			{ChangeType: diff.ChangeAdd},
			{ChangeType: diff.ChangeRemove},
			{ChangeType: diff.ChangeContext},
			{ChangeType: diff.ChangeDivider},
		}, nil
	}}
	got := ComputeStats(differ, "", false, "", entries)
	assert.Equal(t, 4, got.Adds, "two files × two adds each")
	assert.Equal(t, 2, got.Removes, "two files × one remove each")
	assert.False(t, got.Partial)
	assert.NoError(t, got.Err)
}

func TestComputeStats_FirstErrorStopsAndReturns(t *testing.T) {
	entries := []diff.FileEntry{
		{Path: "good.go", Status: diff.FileModified},
		{Path: "bad.go", Status: diff.FileModified},
	}
	wantErr := errors.New("vcs blew up")
	calls := 0
	differ := fakeDiffer{fn: func(_, file string, _ bool, _ int) ([]diff.DiffLine, error) {
		calls++
		if file == "bad.go" {
			return nil, wantErr
		}
		return []diff.DiffLine{{ChangeType: diff.ChangeAdd}}, nil
	}}
	got := ComputeStats(differ, "", false, "", entries)
	require.ErrorIs(t, got.Err, wantErr)
	assert.Equal(t, 2, calls, "should stop after the failing call, not iterate further")
}

func TestComputeStats_StagedFallbackForAddedFile(t *testing.T) {
	entries := []diff.FileEntry{{Path: "new.go", Status: diff.FileAdded}}
	differ := fakeDiffer{fn: func(_, _ string, staged bool, _ int) ([]diff.DiffLine, error) {
		if !staged {
			return nil, nil
		}
		return []diff.DiffLine{{ChangeType: diff.ChangeAdd}, {ChangeType: diff.ChangeAdd}}, nil
	}}
	got := ComputeStats(differ, "", false, "", entries)
	assert.Equal(t, 2, got.Adds, "must fall back to staged content for empty primary diff on FileAdded")
	assert.False(t, got.Partial)
	assert.NoError(t, got.Err)
}

func TestComputeStats_StagedFallbackErrorMarksPartial(t *testing.T) {
	entries := []diff.FileEntry{{Path: "new.go", Status: diff.FileAdded}}
	differ := fakeDiffer{fn: func(_, _ string, staged bool, _ int) ([]diff.DiffLine, error) {
		if staged {
			return nil, errors.New("staged blew up")
		}
		return nil, nil
	}}
	got := ComputeStats(differ, "", false, "", entries)
	assert.True(t, got.Partial, "staged fallback failure must mark stats partial")
	assert.NoError(t, got.Err, "fallback failures are non-fatal")
}

func TestComputeStats_UntrackedReadOutsideWorkDirMarksPartial(t *testing.T) {
	// untracked file paths must not escape workDir even if they contain "..".
	// the safeWorkDirPath guard rejects, partial flag is set, and the file
	// contributes zero lines instead of being read off-tree.
	entries := []diff.FileEntry{{Path: "../../etc/passwd", Status: diff.FileUntracked}}
	differ := fakeDiffer{fn: func(string, string, bool, int) ([]diff.DiffLine, error) {
		return nil, nil
	}}
	got := ComputeStats(differ, "", false, t.TempDir(), entries)
	assert.True(t, got.Partial)
	assert.Equal(t, 0, got.Adds)
	assert.Equal(t, 0, got.Removes)
}

func TestComputeStats_OversizedUntrackedFileSkipped(t *testing.T) {
	// untracked files larger than maxUntrackedBytes are skipped to keep stats
	// computation bounded; the file is excluded from totals and stats are
	// marked partial.
	root := t.TempDir()
	huge := filepath.Join(root, "huge.bin")
	// Sparse file: Truncate grows the file's size without writing maxUntrackedBytes
	// of zeros to disk, so the test is fast and uses no real space.
	require.NoError(t, os.WriteFile(huge, []byte{}, 0o600))
	require.NoError(t, os.Truncate(huge, maxUntrackedBytes+1))
	entries := []diff.FileEntry{{Path: "huge.bin", Status: diff.FileUntracked}}
	differ := fakeDiffer{fn: func(string, string, bool, int) ([]diff.DiffLine, error) {
		return nil, nil
	}}
	got := ComputeStats(differ, "", false, root, entries)
	assert.True(t, got.Partial, "oversized untracked file must mark stats partial")
	assert.Equal(t, 0, got.Adds, "oversized untracked file must not contribute to totals")
}
