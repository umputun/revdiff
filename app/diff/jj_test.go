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

// setupJjRepo creates a temp dir, writes a minimal jj user config there and
// exports JJ_CONFIG so every jj invocation (test setup + production methods under
// test) sees the same deterministic identity without depending on the host jj config.
// It then initializes a colocated jj/git repo and returns the repo path.
// Cannot be used with t.Parallel() because it relies on t.Setenv.
func setupJjRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not available")
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	const body = "[user]\nname = \"Test User\"\nemail = \"test@test.com\"\n"
	require.NoError(t, os.WriteFile(cfg, []byte(body), 0o600))
	t.Setenv("JJ_CONFIG", cfg)
	dir := t.TempDir()
	jjCmd(t, dir, "git", "init", "--quiet")
	return dir
}

func jjCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("jj", args...) //nolint:gosec // args constructed internally
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "jj %v failed: %s", args, string(out))
}

func TestJj_TranslateRef(t *testing.T) {
	j := &Jj{}
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
			assert.Equal(t, tt.want, j.translateRef(tt.ref))
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

func TestJj_ExpandRename(t *testing.T) {
	j := &Jj{}
	tests := []struct {
		name    string
		target  string
		oldPath string
		newPath string
		ok      bool
	}{
		{name: "brace plain", target: "{old.txt => new.txt}", oldPath: "old.txt", newPath: "new.txt", ok: true},
		{name: "brace with prefix and suffix", target: "dir/{old => new}/file.txt", oldPath: "dir/old/file.txt", newPath: "dir/new/file.txt", ok: true},
		{name: "brace with prefix only", target: "dir/{a.txt => b.txt}", oldPath: "dir/a.txt", newPath: "dir/b.txt", ok: true},
		{name: "fallback arrow separator", target: "old.txt => new.txt", oldPath: "old.txt", newPath: "new.txt", ok: true},
		{name: "no separator returns false", target: "just a path", ok: false},
		{name: "empty returns false", target: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldPath, newPath, ok := j.expandRename(tt.target)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.oldPath, oldPath)
				assert.Equal(t, tt.newPath, newPath)
			}
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

	j := &Jj{}
	got := j.synthesizeBinaryDiff(raw)
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
	j := &Jj{}
	assert.Equal(t, raw, j.synthesizeBinaryDiff(raw), "non-binary diff should be returned unchanged")
}

func TestJj_CommitLogRevset(t *testing.T) {
	j := &Jj{}
	tests := []struct {
		name, ref, want string
	}{
		{"single ref maps to X..@", "feature", "feature..@"},
		{"HEAD alias translates to @-", "HEAD", "@-..@"},
		{"HEAD~N translates", "HEAD~3", "@----..@"},
		{"explicit range X..Y", "main..feature", "main..feature"},
		{"range with empty left defaults to root()", "..feature", "root()..feature"},
		{"range with empty right defaults to @", "main..", "main..@"},
		{"HEAD range translates both sides", "HEAD~3..HEAD", "@----..@-"},
		{"triple dot produces symmetric difference", "main...feature", "(main..feature) | (feature..main)"},
		{"triple dot with HEAD", "HEAD~2...HEAD", "(@---..@-) | (@-..@---)"},
		{"tag-like ref with dots not a range", "v1.2.3", "v1.2.3..@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, j.commitLogRevset(tt.ref))
		})
	}
}

func TestJj_ParseCommitLog(t *testing.T) {
	j := &Jj{}

	t.Run("empty output returns nil", func(t *testing.T) {
		assert.Nil(t, j.parseCommitLog(readFixture(t, "jjlog_empty.txt")))
	})

	t.Run("single record", func(t *testing.T) {
		got := j.parseCommitLog(readFixture(t, "jjlog_single.txt"))
		require.Len(t, got, 1)
		assert.Equal(t, "fb56480c709943c74d65ded4e13c94b2a7978ced", got[0].Hash)
		assert.Equal(t, "Eugene <umputun@gmail.com>", got[0].Author)
		assert.Equal(t, "Add commit info popup", got[0].Subject)
		assert.Equal(t, "This popup shows the list of commits\nin the current ref-based diff range.\nUseful for PR reviews.", got[0].Body)
		assert.Equal(t, 2026, got[0].Date.Year())
		assert.Equal(t, time.April, got[0].Date.Month())
		assert.Equal(t, 10, got[0].Date.Day())
	})

	t.Run("many records preserve order and separate bodies", func(t *testing.T) {
		got := j.parseCommitLog(readFixture(t, "jjlog_many.txt"))
		require.Len(t, got, 3)
		assert.Equal(t, "First commit", got[0].Subject)
		assert.Equal(t, "Body of first commit.", got[0].Body)
		assert.Equal(t, "Second commit", got[1].Subject)
		assert.Empty(t, got[1].Body)
		assert.Equal(t, "Third commit", got[2].Subject)
		assert.Equal(t, "Multi-line\nbody here.", got[2].Body)
	})

	t.Run("ANSI escape bytes stripped from author, subject and body", func(t *testing.T) {
		raw := "hash\x00Evil\x1b[31mRed\x1b[0m <e@x>\x002026-04-10T12:00:00-04:00\x000\x00test\x1b[31mred\x1b[0m\nbody\x1b[32mgreen\x1b[0m end\n\x00\x01"
		got := j.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "EvilRed <e@x>", got[0].Author)
		assert.Equal(t, "testred", got[0].Subject)
		assert.Equal(t, "bodygreen end", got[0].Body)
		assert.NotContains(t, got[0].Author, "\x1b")
		assert.NotContains(t, got[0].Subject, "\x1b")
		assert.NotContains(t, got[0].Body, "\x1b")
	})

	t.Run("BEL and C1 control bytes stripped from all fields", func(t *testing.T) {
		raw := "hash\x00Alice\x07 <a@x>\x002026-04-10T12:00:00-04:00\x000\x00sub\u009b31mject\nbo\u009ddy\n\x00\x01"
		got := j.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.NotContains(t, got[0].Author, "\x07")
		assert.NotContains(t, got[0].Subject, "\u009b")
		assert.NotContains(t, got[0].Body, "\u009d")
	})

	t.Run("malformed record (fewer than 5 NUL-separated fields) skipped", func(t *testing.T) {
		raw := "hash\x00author\x002026-04-10T12:00:00-04:00\x000\x00\x01"
		assert.Empty(t, j.parseCommitLog(raw))
	})

	t.Run("records cap at MaxCommits", func(t *testing.T) {
		var b strings.Builder
		for range MaxCommits + 50 {
			b.WriteString("hash\x00author\x002026-04-10T12:00:00-04:00\x000\x00subject\n\x00\x01")
		}
		got := j.parseCommitLog(b.String())
		assert.Len(t, got, MaxCommits)
	})

	t.Run("working-copy placeholder does not shrink MaxCommits cap", func(t *testing.T) {
		// jj is queried with `-n MaxCommits+1` so that the synthetic working-copy @
		// placeholder does not steal one of the MaxCommits real-commit slots.
		// When MaxCommits+1 records arrive with a leading placeholder and MaxCommits
		// real commits, the parser must drop the placeholder and return exactly
		// MaxCommits real entries so the caller's truncation signal still fires.
		var b strings.Builder
		b.WriteString(strings.Repeat("0", 40) + "\x00\x002026-04-10T13:00:00-04:00\x001\x00\n\x00\x01")
		for range MaxCommits {
			b.WriteString("hash\x00author\x002026-04-10T12:00:00-04:00\x000\x00subject\n\x00\x01")
		}
		got := j.parseCommitLog(b.String())
		assert.Len(t, got, MaxCommits)
	})

	t.Run("malformed date leaves zero-value Date", func(t *testing.T) {
		raw := "hash\x00author\x00not-a-date\x000\x00subject\nbody\n\x00\x01"
		got := j.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.True(t, got[0].Date.IsZero())
	})

	t.Run("CJK content preserved as UTF-8", func(t *testing.T) {
		raw := "hash\x00李雷 <lilei@example.com>\x002026-04-10T12:00:00-04:00\x000\x00添加中文支持\n修复 CJK 字符宽度。\n\x00\x01"
		got := j.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "李雷 <lilei@example.com>", got[0].Author)
		assert.Equal(t, "添加中文支持", got[0].Subject)
		assert.Equal(t, "修复 CJK 字符宽度。", got[0].Body)
	})

	t.Run("empty-description working-copy @ commit is filtered out", func(t *testing.T) {
		// jj auto-creates an empty working-copy commit at @ after every `jj new`;
		// range queries like X..@ include it. The parser must drop these — identified
		// by the current_working_copy flag combined with an empty description — so
		// the popup does not show a blank/hash-only row. Real commits with empty
		// descriptions (flag=0) are kept.
		raw := "hash\x00Alice\x002026-04-10T12:00:00-04:00\x000\x00real subject\nreal body\n\x00\x01" +
			strings.Repeat("0", 40) + "\x00\x002026-04-10T13:00:00-04:00\x001\x00\n\x00\x01"
		got := j.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "hash", got[0].Hash)
		assert.Equal(t, "real subject", got[0].Subject)
		assert.Equal(t, "real body", got[0].Body)
	})

	t.Run("non-working-copy commit with empty description is kept", func(t *testing.T) {
		// jj allows real commits with no description; they must not be dropped.
		raw := "realhash\x00Bob\x002026-04-10T14:00:00-04:00\x000\x00\x00\x01"
		got := j.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "realhash", got[0].Hash)
		assert.Empty(t, got[0].Subject)
		assert.Empty(t, got[0].Body)
	})

	t.Run("working-copy flag with empty description and no trailing newline is filtered", func(t *testing.T) {
		// some jj template paths emit a bare empty description field (no newline).
		raw := "emptyhash\x00Bob\x002026-04-10T14:00:00-04:00\x001\x00\x00\x01"
		assert.Empty(t, j.parseCommitLog(raw))
	})
}

func TestJj_CommitLog_EmptyRefReturnsNil(t *testing.T) {
	j := NewJj("/nonexistent/dir")
	commits, err := j.CommitLog("")
	require.NoError(t, err)
	assert.Nil(t, commits)

	commits, err = j.CommitLog("   ")
	require.NoError(t, err)
	assert.Nil(t, commits)
}

func TestJj_CommitLog_InvalidDir(t *testing.T) {
	j := NewJj("/nonexistent/dir")
	_, err := j.CommitLog("HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit log")
}

// e2e tests below

func TestJj_ChangedFiles_Uncommitted(t *testing.T) {
	dir := setupJjRepo(t)
	j := NewJj(dir)

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
	j := NewJj(dir)

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
	j := NewJj(dir)

	writeFile(t, dir, "hello.txt", "hello\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "empty", "--quiet")

	entries, err := j.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestJj_ChangedFiles_Deleted(t *testing.T) {
	dir := setupJjRepo(t)
	j := NewJj(dir)

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
	j := NewJj(dir)

	writeFile(t, dir, "hello.txt", "line one\nline two\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "modify", "--quiet")
	writeFile(t, dir, "hello.txt", "line one\nline modified\nline three\n")

	lines, err := j.FileDiff("", "hello.txt", false, 0)
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
	j := NewJj(dir)

	writeFile(t, dir, "hello.txt", "line one\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "add", "--quiet")
	writeFile(t, dir, "new.txt", "new content\n")

	lines, err := j.FileDiff("", "new.txt", false, 0)
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
	j := NewJj(dir)

	writeFile(t, dir, "placeholder.txt", "x\n")
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "add binary", "--quiet")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0x00, 0x01, 0x02, 0xff}, 0o600))

	lines, err := j.FileDiff("", "bin.dat", false, 0)
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
	lines, err := dr.FileDiff("", "main.go", false, 0)
	require.NoError(t, err)
	require.Len(t, lines, 3)
	for _, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType)
	}
}

func TestJj_UntrackedFiles(t *testing.T) {
	dir := setupJjRepo(t)
	j := NewJj(dir)

	// jj auto-tracks everything in the working copy, so there are no "untracked" files
	// in the git sense. We expect UntrackedFiles to always return an empty result.
	writeFile(t, dir, "a.txt", "a\n")
	files, err := j.UntrackedFiles()
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestJjContextArg(t *testing.T) {
	tests := []struct {
		name         string
		contextLines int
		want         string
	}{
		{name: "zero requests full file", contextLines: 0, want: "--context=1000000"},
		{name: "five", contextLines: 5, want: "--context=5"},
		{name: "one", contextLines: 1, want: "--context=1"},
		{name: "just below sentinel", contextLines: 999999, want: "--context=999999"},
		{name: "exact sentinel returns full file", contextLines: 1000000, want: "--context=1000000"},
		{name: "above sentinel returns full file", contextLines: 1000001, want: "--context=1000000"},
		{name: "negative returns full file", contextLines: -1, want: "--context=1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jjContextArg(tt.contextLines))
		})
	}
}

func TestJj_FileDiff_SmallContext(t *testing.T) {
	dir := setupJjRepo(t)
	j := NewJj(dir)

	// build a 20-line file in an initial commit, then modify line 10 in a new change.
	// with contextLines=2 the diff should contain 1 removed, 1 added, 4 context.
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	writeFile(t, dir, "big.txt", sb.String())
	jjCmd(t, dir, "describe", "-m", "init", "--quiet")
	jjCmd(t, dir, "new", "-m", "modify", "--quiet")

	sb.Reset()
	for i := 1; i <= 20; i++ {
		if i == 10 {
			fmt.Fprintf(&sb, "line %d CHANGED\n", i)
			continue
		}
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	writeFile(t, dir, "big.txt", sb.String())

	lines, err := j.FileDiff("", "big.txt", false, 2)
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
	fullLines, err := j.FileDiff("", "big.txt", false, 0)
	require.NoError(t, err)
	var fullCtx int
	for _, l := range fullLines {
		if l.ChangeType == ChangeContext {
			fullCtx++
		}
	}
	assert.Equal(t, 19, fullCtx, "expected 19 context lines with full-file context")
}
