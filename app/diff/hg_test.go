package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupHgRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg not available")
	}
	dir := t.TempDir()
	hgCmd(t, dir, "init")
	err := os.WriteFile(filepath.Join(dir, ".hg", "hgrc"), []byte("[ui]\nusername = Test User <test@test.com>\n"), 0o600)
	require.NoError(t, err)
	return dir
}

func hgCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("hg", args...) //nolint:gosec // args constructed internally
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "hg %v failed: %s", args, string(out))
}

func TestTranslateRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "HEAD", ref: "HEAD", want: "."},
		{name: "HEAD~1", ref: "HEAD~1", want: ".~1"},
		{name: "HEAD~3", ref: "HEAD~3", want: ".~3"},
		{name: "HEAD^", ref: "HEAD^", want: ".^"},
		{name: "HEAD^1", ref: "HEAD^1", want: ".^"},
		{name: "HEAD^2", ref: "HEAD^2", want: "p2(.)"},
		{name: "HEAD^3", ref: "HEAD^3", want: "p3(.)"},
		{name: "bare hash", ref: "abc123", want: "abc123"},
		{name: "bookmark", ref: "main", want: "main"},
		{name: "empty", ref: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, translateRef(tt.ref))
		})
	}
}

func TestHg_RevFlag(t *testing.T) {
	h := &Hg{}

	tests := []struct {
		name string
		flag string
		ref  string
		want []string
	}{
		{name: "empty", flag: "-r", ref: "", want: nil},
		{name: "single ref -r", flag: "-r", ref: "HEAD", want: []string{"-r", "."}},
		{name: "range -r", flag: "-r", ref: "main..feature", want: []string{"-r", "main", "-r", "feature"}},
		{name: "HEAD range -r", flag: "-r", ref: "HEAD~3..HEAD", want: []string{"-r", ".~3", "-r", "."}},
		{name: "single ref --rev", flag: "--rev", ref: "HEAD", want: []string{"--rev", "."}},
		{name: "range --rev", flag: "--rev", ref: "main..feature", want: []string{"--rev", "main", "--rev", "feature"}},
		{name: "left empty", flag: "-r", ref: "..HEAD", want: []string{"-r", "0", "-r", "."}},
		{name: "right empty", flag: "-r", ref: "main..", want: []string{"-r", "main", "-r", "."}},
		{name: "triple dot", flag: "-r", ref: "main...feature", want: []string{"-r", "ancestor(main,feature)", "-r", "feature"}},
		{name: "triple dot HEAD", flag: "-r", ref: "HEAD~3...HEAD", want: []string{"-r", "ancestor(.~3,.)", "-r", "."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, h.revFlag(tt.flag, tt.ref))
		})
	}
}

func TestHg_ChangedFiles_Uncommitted(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	// create and commit a file
	writeFile(t, dir, "hello.txt", "hello\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	// modify the file
	writeFile(t, dir, "hello.txt", "hello world\n")

	entries, err := h.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Path)
	assert.Equal(t, FileModified, entries[0].Status)
}

func TestHg_ChangedFiles_Added(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	writeFile(t, dir, "new.txt", "new file\n")
	hgCmd(t, dir, "add", "new.txt")

	entries, err := h.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "new.txt", entries[0].Path)
	assert.Equal(t, FileAdded, entries[0].Status)
}

func TestHg_ChangedFiles_NoChanges(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	entries, err := h.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestHg_ChangedFiles_Deleted(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	hgCmd(t, dir, "remove", "hello.txt")

	entries, err := h.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Path)
	assert.Equal(t, FileDeleted, entries[0].Status)
}

func TestHg_FileDiff_Uncommitted(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "line one\nline two\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	writeFile(t, dir, "hello.txt", "line one\nline modified\nline three\n")

	lines, err := h.FileDiff("", "hello.txt", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	// should have mix of context, add, and remove lines
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

func TestHg_FileDiff_NewFile(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "line one\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	writeFile(t, dir, "new.txt", "new content\n")
	hgCmd(t, dir, "add", "new.txt")

	lines, err := h.FileDiff("", "new.txt", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	// all lines should be additions
	for _, l := range lines {
		if l.ChangeType == ChangeDivider {
			continue
		}
		assert.Equal(t, ChangeAdd, l.ChangeType, "new file lines should be additions")
	}
}

func TestHg_FileDiff_WithRef(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "original\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "first")

	writeFile(t, dir, "hello.txt", "modified\n")
	hgCmd(t, dir, "commit", "-m", "second")

	// diff between revisions
	lines, err := h.FileDiff("0..1", "hello.txt", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	var adds, removes int
	for _, l := range lines {
		switch l.ChangeType { //nolint:exhaustive // only counting relevant types
		case ChangeAdd:
			adds++
		case ChangeRemove:
			removes++
		}
	}
	assert.Equal(t, 1, adds)
	assert.Equal(t, 1, removes)
}

func TestHg_UntrackedFiles(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "tracked.txt", "tracked\n")
	hgCmd(t, dir, "add", "tracked.txt")
	hgCmd(t, dir, "commit", "-m", "init")

	writeFile(t, dir, "untracked.txt", "untracked\n")

	files, err := h.UntrackedFiles()
	require.NoError(t, err)
	assert.Contains(t, files, "untracked.txt")
	assert.NotContains(t, files, "tracked.txt")
}

func TestHg_ParseStatus(t *testing.T) {
	h := &Hg{}
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
			input: "M a.txt\nA b.txt\nR c.txt\n",
			want: []FileEntry{
				{Path: "a.txt", Status: FileModified},
				{Path: "b.txt", Status: FileAdded},
				{Path: "c.txt", Status: FileDeleted},
			},
		},
		{
			name:  "skips untracked",
			input: "M a.txt\n? untracked.txt\n",
			want:  []FileEntry{{Path: "a.txt", Status: FileModified}},
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, h.parseStatus(tt.input))
		})
	}
}

func TestHg_StatusToFileStatus(t *testing.T) {
	h := &Hg{}
	tests := []struct {
		status string
		want   FileStatus
	}{
		{"M", FileModified},
		{"A", FileAdded},
		{"R", FileDeleted},
		{"?", ""},
		{"!", ""},
		{"I", ""},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, h.hgStatusToFileStatus(tt.status))
		})
	}
}
