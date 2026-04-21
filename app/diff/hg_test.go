package diff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	lines, err := h.FileDiff("", "hello.txt", false, 0)
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

	lines, err := h.FileDiff("", "new.txt", false, 0)
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
	lines, err := h.FileDiff("0..1", "hello.txt", false, 0)
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

func TestHg_CommitLogRevset(t *testing.T) {
	h := &Hg{}
	tests := []struct {
		name, ref, want string
	}{
		{"single ref X excludes X for parity with git/jj", "feature", "feature::. - feature"},
		{"HEAD alias translates to .", "HEAD", ".::. - ."},
		{"HEAD~N translates", "HEAD~3", ".~3::. - .~3"},
		{"explicit range X..Y", "default..feature", "default::feature - default"},
		{"range with empty left defaults to 0", "..feature", "0::feature - 0"},
		{"range with empty right defaults to .", "default..", "default::. - default"},
		{"HEAD range translates both sides", "HEAD~3..HEAD", ".~3::. - .~3"},
		{"triple dot produces symmetric difference", "default...feature", "only(default,feature) + only(feature,default)"},
		{"triple dot with HEAD", "HEAD~2...HEAD", "only(.~2,.) + only(.,.~2)"},
		{"tag-like ref with dots not a range", "v1.2.3", "v1.2.3::. - v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, h.commitLogRevset(tt.ref))
		})
	}
}

func TestHg_ParseCommitLog(t *testing.T) {
	h := &Hg{}

	t.Run("empty output returns nil", func(t *testing.T) {
		assert.Nil(t, h.parseCommitLog(readFixture(t, "hglog_empty.txt")))
	})

	t.Run("single record", func(t *testing.T) {
		got := h.parseCommitLog(readFixture(t, "hglog_single.txt"))
		require.Len(t, got, 1)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", got[0].Hash)
		assert.Equal(t, "Eugene <umputun@gmail.com>", got[0].Author)
		assert.Equal(t, "Add commit info popup", got[0].Subject)
		assert.Equal(t, "This popup shows the list of commits\nin the current ref-based diff range.\nUseful for PR reviews.", got[0].Body)
		assert.Equal(t, 2026, got[0].Date.Year())
		assert.Equal(t, time.April, got[0].Date.Month())
		assert.Equal(t, 10, got[0].Date.Day())
	})

	t.Run("many records preserve order and separate bodies", func(t *testing.T) {
		got := h.parseCommitLog(readFixture(t, "hglog_many.txt"))
		require.Len(t, got, 3)
		assert.Equal(t, "First commit", got[0].Subject)
		assert.Equal(t, "Body of first commit.", got[0].Body)
		assert.Equal(t, "Second commit", got[1].Subject)
		assert.Empty(t, got[1].Body)
		assert.Equal(t, "Third commit", got[2].Subject)
		assert.Equal(t, "Multi-line\nbody here.", got[2].Body)
	})

	t.Run("ANSI escape bytes stripped from author, subject and body", func(t *testing.T) {
		raw := "hash\x1fEvil\x1b[31mRed\x1b[0m <e@x>\x1f2026-04-10T12:00:00-04:00\x1ftest\x1b[31mred\x1b[0m\nbody\x1b[32mgreen\x1b[0m end\x1e"
		got := h.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "EvilRed <e@x>", got[0].Author)
		assert.Equal(t, "testred", got[0].Subject)
		assert.Equal(t, "bodygreen end", got[0].Body)
		assert.NotContains(t, got[0].Author, "\x1b")
		assert.NotContains(t, got[0].Subject, "\x1b")
		assert.NotContains(t, got[0].Body, "\x1b")
	})

	t.Run("BEL and C1 control bytes stripped from all fields", func(t *testing.T) {
		raw := "hash\x1fAlice\x07 <a@x>\x1f2026-04-10T12:00:00-04:00\x1fsub\u009b31mject\nbo\u009ddy\x1e"
		got := h.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.NotContains(t, got[0].Author, "\x07")
		assert.NotContains(t, got[0].Subject, "\u009b")
		assert.NotContains(t, got[0].Body, "\u009d")
	})

	t.Run("malformed record (fewer than 4 US-separated fields) skipped", func(t *testing.T) {
		raw := "hash\x1fauthor\x1f2026-04-10T12:00:00-04:00\x1e"
		// fields are [hash, author, date] — only 3, not 4 → skipped
		assert.Empty(t, h.parseCommitLog(raw))
	})

	t.Run("records cap at MaxCommits", func(t *testing.T) {
		var b strings.Builder
		for range MaxCommits + 50 {
			b.WriteString("hash\x1fauthor\x1f2026-04-10T12:00:00-04:00\x1fsubject\x1e")
		}
		got := h.parseCommitLog(b.String())
		assert.Len(t, got, MaxCommits)
	})

	t.Run("malformed date leaves zero-value Date", func(t *testing.T) {
		raw := "hash\x1fauthor\x1fnot-a-date\x1fsubject\nbody\x1e"
		got := h.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.True(t, got[0].Date.IsZero())
	})

	t.Run("CJK content preserved as UTF-8", func(t *testing.T) {
		raw := "hash\x1f李雷 <lilei@example.com>\x1f2026-04-10T12:00:00-04:00\x1f添加中文支持\n修复 CJK 字符宽度。\x1e"
		got := h.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "李雷 <lilei@example.com>", got[0].Author)
		assert.Equal(t, "添加中文支持", got[0].Subject)
		assert.Equal(t, "修复 CJK 字符宽度。", got[0].Body)
	})
}

func TestHg_CommitLog_EmptyRefReturnsNil(t *testing.T) {
	h := NewHg("/nonexistent/dir")
	commits, err := h.CommitLog("")
	require.NoError(t, err)
	assert.Nil(t, commits)

	commits, err = h.CommitLog("   ")
	require.NoError(t, err)
	assert.Nil(t, commits)
}

func TestHg_CommitLog_InvalidDir(t *testing.T) {
	h := NewHg("/nonexistent/dir")
	_, err := h.CommitLog("HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit log")
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

func TestHgContextArg(t *testing.T) {
	tests := []struct {
		name         string
		contextLines int
		want         string
	}{
		{name: "zero requests full file", contextLines: 0, want: "-U1000000"},
		{name: "five", contextLines: 5, want: "-U5"},
		{name: "one", contextLines: 1, want: "-U1"},
		{name: "just below sentinel", contextLines: 999999, want: "-U999999"},
		{name: "exact sentinel returns full file", contextLines: 1000000, want: "-U1000000"},
		{name: "above sentinel returns full file", contextLines: 1000001, want: "-U1000000"},
		{name: "negative returns full file", contextLines: -1, want: "-U1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hgContextArg(tt.contextLines))
		})
	}
}

func TestHg_FileDiff_SmallContext(t *testing.T) {
	dir := setupHgRepo(t)
	h := NewHg(dir)

	// build a 20-line file, commit, then change line 10.
	// with contextLines=2 the diff should contain 1 removed, 1 added, 4 context.
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	writeFile(t, dir, "big.txt", sb.String())
	hgCmd(t, dir, "add", "big.txt")
	hgCmd(t, dir, "commit", "-m", "initial")

	sb.Reset()
	for i := 1; i <= 20; i++ {
		if i == 10 {
			fmt.Fprintf(&sb, "line %d CHANGED\n", i)
			continue
		}
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	writeFile(t, dir, "big.txt", sb.String())

	lines, err := h.FileDiff("", "big.txt", false, 2)
	require.NoError(t, err)

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
	assert.Equal(t, 1, removes, "expected exactly 1 removed line at contextLines=2")
	assert.Equal(t, 1, adds, "expected exactly 1 added line at contextLines=2")
	assert.Equal(t, 4, ctx, "expected 4 context lines (2 above + 2 below) at contextLines=2")

	// with contextLines=0 (full file) the diff should contain all 19 unchanged
	// lines as context, proving the parameter is actually in effect.
	fullLines, err := h.FileDiff("", "big.txt", false, 0)
	require.NoError(t, err)
	var fullCtx int
	for _, l := range fullLines {
		if l.ChangeType == ChangeContext {
			fullCtx++
		}
	}
	assert.Equal(t, 19, fullCtx, "expected 19 context lines with full-file context")
}
