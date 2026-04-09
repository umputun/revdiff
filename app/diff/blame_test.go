package diff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBlame(t *testing.T) {
	g := Git{}
	// simulated git blame --line-porcelain output for a 3-line file
	raw := "abc1234567890123456789012345678901234567 1 1 2\n" +
		"author Alice\n" +
		"author-mail <alice@example.com>\n" +
		"author-time 1700000000\n" +
		"author-tz +0000\n" +
		"committer Alice\n" +
		"committer-mail <alice@example.com>\n" +
		"committer-time 1700000000\n" +
		"committer-tz +0000\n" +
		"summary initial commit\n" +
		"filename main.go\n" +
		"\tpackage main\n" +
		"abc1234567890123456789012345678901234567 1 2\n" +
		"author Alice\n" +
		"author-mail <alice@example.com>\n" +
		"author-time 1700000000\n" +
		"author-tz +0000\n" +
		"committer Alice\n" +
		"committer-mail <alice@example.com>\n" +
		"committer-time 1700000000\n" +
		"committer-tz +0000\n" +
		"summary initial commit\n" +
		"filename main.go\n" +
		"\t\n" +
		"def4567890123456789012345678901234567890 3 3 1\n" +
		"author Bob\n" +
		"author-mail <bob@example.com>\n" +
		"author-time 1710000000\n" +
		"author-tz -0500\n" +
		"committer Bob\n" +
		"committer-mail <bob@example.com>\n" +
		"committer-time 1710000000\n" +
		"committer-tz -0500\n" +
		"summary add feature\n" +
		"previous abc1234567890123456789012345678901234567 main.go\n" +
		"filename main.go\n" +
		"\tfunc main() {}\n"

	result, err := g.parseBlame(raw)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	assert.Equal(t, "Alice", result[1].Author)
	assert.Equal(t, time.Unix(1700000000, 0), result[1].Time)

	assert.Equal(t, "Alice", result[2].Author)
	assert.Equal(t, time.Unix(1700000000, 0), result[2].Time)

	assert.Equal(t, "Bob", result[3].Author)
	assert.Equal(t, time.Unix(1710000000, 0), result[3].Time)
}

func TestParseBlame_empty(t *testing.T) {
	g := Git{}
	result, err := g.parseBlame("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestIsHexString(t *testing.T) {
	g := Git{}
	assert.True(t, g.isHexString("abcdef0123456789"))
	assert.True(t, g.isHexString("ABCDEF"))
	assert.False(t, g.isHexString("xyz"))
	assert.False(t, g.isHexString(""))
}

func TestRelativeAge(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "  ?"},
		{"30 seconds", now.Add(-30 * time.Second), " 1m"},
		{"5 minutes", now.Add(-5 * time.Minute), " 5m"},
		{"59 minutes", now.Add(-59 * time.Minute), "59m"},
		{"1 hour", now.Add(-1 * time.Hour), " 1h"},
		{"23 hours", now.Add(-23 * time.Hour), "23h"},
		{"1 day", now.Add(-24 * time.Hour), " 1d"},
		{"6 days", now.Add(-6 * 24 * time.Hour), " 6d"},
		{"1 week", now.Add(-7 * 24 * time.Hour), " 1w"},
		{"3 weeks", now.Add(-21 * 24 * time.Hour), " 3w"},
		{"2 months", now.Add(-60 * 24 * time.Hour), " 2M"},
		{"11 months", now.Add(-330 * 24 * time.Hour), "11M"},
		{"1 year", now.Add(-365 * 24 * time.Hour), " 1y"},
		{"3 years", now.Add(-3 * 365 * 24 * time.Hour), " 3y"},
		{"future", now.Add(1 * time.Hour), " 1m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeAge(tt.t, now)
			assert.Equal(t, tt.want, got)
			assert.Len(t, got, 3, "age string should be exactly 3 chars wide")
		})
	}
}

func TestBlameTargetRef(t *testing.T) {
	g := Git{}
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"double-dot", "HEAD~3..HEAD", "HEAD"},
		{"triple-dot", "main...feature", "feature"},
		{"single ref", "HEAD~3", ""},
		{"empty", "", ""},
		{"double-dot missing left", "..HEAD", ""},
		{"double-dot missing right", "HEAD..", ""},
		{"triple-dot missing left", "...HEAD", ""},
		{"triple-dot missing right", "HEAD...", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, g.blameTargetRef(tt.ref))
		})
	}
}

func TestGit_FileBlame_UsesWorktreeForSingleRefDiffs(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "f.txt", "one\ntwo\n")
	gitCmd(t, dir, "add", "f.txt")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "f.txt", "one\ntwo committed\n")
	gitCmd(t, dir, "add", "f.txt")
	gitCmd(t, dir, "commit", "-m", "second")

	writeFile(t, dir, "f.txt", "one\ntwo worktree\n")

	result, err := g.FileBlame("HEAD~1", "f.txt", false)
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "Test", result[1].Author)
	assert.Equal(t, "Not Committed Yet", result[2].Author)
}

func TestGit_FileBlame_UsesTargetRefForTwoRefDiffs(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "f.txt", "one\ntwo\n")
	gitCmd(t, dir, "add", "f.txt")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "f.txt", "one\ntwo target\n")
	gitCmd(t, dir, "add", "f.txt")
	gitCmd(t, dir, "commit", "-m", "target")

	writeFile(t, dir, "f.txt", "one\ntwo worktree\n")

	result, err := g.FileBlame("HEAD~1..HEAD", "f.txt", false)
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "Test", result[1].Author)
	assert.Equal(t, "Test", result[2].Author)
}

func TestGit_FileBlame_UsesIndexForStagedDiffs(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "f.txt", "one\ntwo\n")
	gitCmd(t, dir, "add", "f.txt")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "f.txt", "one\ntwo staged\n")
	gitCmd(t, dir, "add", "f.txt")

	writeFile(t, dir, "f.txt", "one\ntwo unstaged\n")

	result, err := g.FileBlame("", "f.txt", true)
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "Test", result[1].Author)
	assert.Equal(t, "External file (--contents)", result[2].Author)
}
