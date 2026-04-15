package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJjRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not available")
	}
	dir := t.TempDir()
	jjCmd(t, dir, "git", "init", "--quiet")
	// user config for commits (stored in per-repo config path after recent jj versions)
	// we pass --config inline instead, which avoids the config migration warning
	return dir
}

func jjCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := make([]string, 0, 2+len(args))
	full = append(full,
		"--config=user.name=Test User",
		"--config=user.email=test@test.com",
	)
	full = append(full, args...)
	cmd := exec.Command("jj", full...) //nolint:gosec // args constructed internally
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "jj %v failed: %s", args, string(out))
}

// newJjForTest returns a Jj renderer with test-only config overrides so commits
// get a deterministic author and don't depend on the host jj config.
func newJjForTest(dir string) *Jj {
	j := NewJj(dir)
	j.extraArgs = []string{
		"--config=user.name=Test User",
		"--config=user.email=test@test.com",
	}
	return j
}

func TestTranslateJjRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "empty", ref: "", want: ""},
		{name: "HEAD", ref: "HEAD", want: "@-"},
		{name: "HEAD~1", ref: "HEAD~1", want: "@--"},
		{name: "HEAD~3", ref: "HEAD~3", want: "@----"},
		{name: "HEAD^", ref: "HEAD^", want: "@--"},
		{name: "HEAD^1", ref: "HEAD^1", want: "@--"},
		{name: "HEAD^2", ref: "HEAD^2", want: "parents(@-)"},
		{name: "HEAD^3", ref: "HEAD^3", want: "parents(@-)"},
		{name: "bare hash", ref: "abc123", want: "abc123"},
		{name: "bookmark", ref: "main", want: "main"},
		{name: "jj working copy", ref: "@", want: "@"},
		{name: "jj parent", ref: "@-", want: "@-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, translateJjRef(tt.ref))
		})
	}
}

func TestJj_DiffRangeFlags(t *testing.T) {
	j := &Jj{}

	tests := []struct {
		name string
		ref  string
		want []string
	}{
		{name: "empty", ref: "", want: nil},
		{name: "single ref HEAD", ref: "HEAD", want: []string{"--from", "@-", "--to", "@"}},
		{name: "single ref bookmark", ref: "main", want: []string{"--from", "main", "--to", "@"}},
		{name: "range", ref: "main..feature", want: []string{"--from", "main", "--to", "feature"}},
		{name: "HEAD range", ref: "HEAD~3..HEAD", want: []string{"--from", "@----", "--to", "@-"}},
		{name: "left empty", ref: "..HEAD", want: []string{"--from", "root()", "--to", "@-"}},
		{name: "right empty", ref: "main..", want: []string{"--from", "main", "--to", "@"}},
		{name: "triple dot", ref: "main...feature", want: []string{"--from", "ancestors(main) & ancestors(feature) & ~root()", "--to", "feature"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, j.diffRangeFlags(tt.ref))
		})
	}
}

func TestJj_ParseStatus(t *testing.T) {
	j := &Jj{}
	tests := []struct {
		name  string
		input string
		want  []FileEntry
	}{
		{
			name:  "modified file",
			input: "M hello.txt\n",
			want:  []FileEntry{{Path: "hello.txt", Status: FileModified}},
		},
		{
			name:  "multiple statuses",
			input: "M a.txt\nA b.txt\nD c.txt\n",
			want: []FileEntry{
				{Path: "a.txt", Status: FileModified},
				{Path: "b.txt", Status: FileAdded},
				{Path: "c.txt", Status: FileDeleted},
			},
		},
		{
			name:  "rename expands to delete + add",
			input: "R {old.txt => new.txt}\n",
			want: []FileEntry{
				{Path: "old.txt", Status: FileDeleted},
				{Path: "new.txt", Status: FileAdded},
			},
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, j.parseSummary(tt.input))
		})
	}
}

func TestJj_SynthesizeBinaryDiff(t *testing.T) {
	// when jj emits raw bytes for a binary file, swap the hunk with the
	// git-style "Binary files ... differ" marker so parseUnifiedDiff picks it up.
	raw := "diff --git a/bin.dat b/bin.dat\n" +
		"new file mode 100644\n" +
		"index 0000000000..b78dd4eaf7\n" +
		"--- /dev/null\n" +
		"+++ b/bin.dat\n" +
		"@@ -0,0 +1,1 @@\n" +
		"+\x00\x01\x02binary\n"

	got := jjSynthesizeBinaryDiff(raw)
	assert.Contains(t, got, "Binary files /dev/null and b/bin.dat differ")
	assert.NotContains(t, got, "\x00", "null bytes should be stripped")
	assert.NotContains(t, got, "+\x01\x02binary", "binary hunk body should be removed")
}

func TestJj_SynthesizeBinaryDiff_NoBinary(t *testing.T) {
	raw := "diff --git a/a.txt b/a.txt\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-old\n" +
		"+new\n"
	assert.Equal(t, raw, jjSynthesizeBinaryDiff(raw), "non-binary diff should be returned unchanged")
}

// e2e tests below

func TestJj_ChangedFiles_Uncommitted(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "modify", "--quiet")
	writeFile(t, dir, "hello.txt", "hello world\n")

	entries, err := j.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Path)
	assert.Equal(t, FileModified, entries[0].Status)
}

func TestJj_ChangedFiles_Added(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "second", "--quiet")
	writeFile(t, dir, "new.txt", "new file\n")

	entries, err := j.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "new.txt", entries[0].Path)
	assert.Equal(t, FileAdded, entries[0].Status)
}

func TestJj_ChangedFiles_NoChanges(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "empty", "--quiet")

	entries, err := j.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestJj_ChangedFiles_Deleted(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "rm", "--quiet")
	require.NoError(t, os.Remove(filepath.Join(dir, "hello.txt")))

	entries, err := j.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Path)
	assert.Equal(t, FileDeleted, entries[0].Status)
}

func TestJj_FileDiff_Uncommitted(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "line one\nline two\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "modify", "--quiet")
	writeFile(t, dir, "hello.txt", "line one\nline modified\nline three\n")

	lines, err := j.FileDiff("", "hello.txt", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	var adds, removes, ctx int
	for _, l := range lines {
		switch l.ChangeType { //nolint:exhaustive // only counting relevant types
		case ChangeAdd:
			adds++
		case ChangeRemove:
			removes++
		case ChangeContext:
			ctx++
		}
	}
	assert.Positive(t, adds, "expected some added lines")
	assert.Positive(t, removes, "expected some removed lines")
	assert.Positive(t, ctx, "expected some context lines")
}

func TestJj_FileDiff_NewFile(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "line one\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "add", "--quiet")
	writeFile(t, dir, "new.txt", "new content\n")

	lines, err := j.FileDiff("", "new.txt", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	for _, l := range lines {
		if l.ChangeType == ChangeDivider {
			continue
		}
		assert.Equal(t, ChangeAdd, l.ChangeType, "new file lines should be additions")
	}
}

func TestJj_FileDiff_Binary(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "placeholder.txt", "x\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "add binary", "--quiet")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0x00, 0x01, 0x02, 0xff}, 0o600))

	lines, err := j.FileDiff("", "bin.dat", false)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.True(t, lines[0].IsBinary)
}

func TestJj_DirectoryReader_ChangedFiles(t *testing.T) {
	dir := setupJjRepo(t)

	writeFile(t, dir, "a.go", "package a\n")
	writeFile(t, dir, "z.go", "package z\n")
	writeFile(t, dir, "m.go", "package m\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")

	dr := NewJjDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "a.go"}, {Path: "m.go"}, {Path: "z.go"}}, files)
}

func TestJj_DirectoryReader_FileDiff(t *testing.T) {
	dir := setupJjRepo(t)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")

	dr := NewJjDirectoryReader(dir)
	lines, err := dr.FileDiff("", "main.go", false)
	require.NoError(t, err)
	require.Len(t, lines, 3)
	for _, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType)
	}
}

func TestJj_UntrackedFiles(t *testing.T) {
	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	// jj auto-tracks everything in the working copy, so there are no "untracked" files
	// in the git sense. We expect UntrackedFiles to always return an empty result.
	writeFile(t, dir, "a.txt", "a\n")
	files, err := j.UntrackedFiles()
	require.NoError(t, err)
	assert.Empty(t, files)
}
