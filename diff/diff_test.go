package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUnifiedDiff_SimpleAdd(t *testing.T) {
	raw := readFixture(t, "simple_add.diff")
	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err)

	// expected: context, blank, context, add, add, add, context, context
	require.Len(t, lines, 8)

	assert.Equal(t, ChangeContext, lines[0].ChangeType)
	assert.Equal(t, "package main", lines[0].Content)
	assert.Equal(t, 1, lines[0].OldNum)
	assert.Equal(t, 1, lines[0].NewNum)

	// blank line (empty context)
	assert.Equal(t, ChangeContext, lines[1].ChangeType)
	assert.Empty(t, lines[1].Content)

	assert.Equal(t, ChangeContext, lines[2].ChangeType)
	assert.Equal(t, "func handle() {", lines[2].Content)

	// three added lines
	assert.Equal(t, ChangeAdd, lines[3].ChangeType)
	assert.Equal(t, "    if err != nil {", lines[3].Content)
	assert.Equal(t, 0, lines[3].OldNum)
	assert.Equal(t, 4, lines[3].NewNum)

	assert.Equal(t, ChangeAdd, lines[4].ChangeType)
	assert.Equal(t, "        return", lines[4].Content)

	assert.Equal(t, ChangeAdd, lines[5].ChangeType)
	assert.Equal(t, "    }", lines[5].Content)

	assert.Equal(t, ChangeContext, lines[6].ChangeType)
	assert.Equal(t, "    fmt.Println(\"done\")", lines[6].Content)
	assert.Equal(t, 4, lines[6].OldNum)
	assert.Equal(t, 7, lines[6].NewNum)

	assert.Equal(t, ChangeContext, lines[7].ChangeType)
	assert.Equal(t, "}", lines[7].Content)
}

func TestParseUnifiedDiff_SimpleRemove(t *testing.T) {
	raw := readFixture(t, "simple_remove.diff")
	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err)
	require.NotEmpty(t, lines, "expected non-empty result")

	// verify we have removed lines
	var removals, additions int
	for _, l := range lines {
		switch l.ChangeType {
		case ChangeRemove:
			removals++
		case ChangeAdd:
			additions++
		}
	}
	assert.Equal(t, 3, removals, "expected 3 removed lines")
	assert.Equal(t, 1, additions, "expected 1 added line")
}

func TestParseUnifiedDiff_MultiHunk(t *testing.T) {
	raw := readFixture(t, "multi_hunk.diff")
	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err)

	// verify divider exists between hunks
	var dividers int
	for _, l := range lines {
		if l.ChangeType == ChangeDivider {
			dividers++
		}
	}
	assert.Equal(t, 1, dividers, "expected 1 divider between two hunks")

	// verify additions in both hunks
	var additions []string
	for _, l := range lines {
		if l.ChangeType == ChangeAdd {
			additions = append(additions, l.Content)
		}
	}
	assert.Equal(t, []string{`import "os"`, "    os.Exit(0)"}, additions)
}

func TestParseUnifiedDiff_MixedChanges(t *testing.T) {
	raw := readFixture(t, "mixed_changes.diff")
	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err)

	types := make([]string, 0, len(lines))
	for _, l := range lines {
		types = append(types, l.ChangeType)
	}

	// expected sequence: ctx, blank, ctx, remove, remove, add, add, ctx, ctx, blank
	assert.Equal(t, []string{
		ChangeContext,
		ChangeContext, // blank
		ChangeContext,
		ChangeRemove,
		ChangeRemove,
		ChangeAdd,
		ChangeAdd,
		ChangeContext,
		ChangeContext,
		ChangeContext, // blank trailing
	}, types)
}

func TestParseUnifiedDiff_Empty(t *testing.T) {
	lines, err := ParseUnifiedDiff("")
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestParseUnifiedDiff_LineNumbers(t *testing.T) {
	raw := readFixture(t, "simple_add.diff")
	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err)

	// additions should have OldNum=0
	for _, l := range lines {
		if l.ChangeType == ChangeAdd {
			assert.Equal(t, 0, l.OldNum, "added lines should have OldNum=0")
			assert.Positive(t, l.NewNum, "added lines should have positive NewNum")
		}
	}

	// context lines should have both nums > 0
	for _, l := range lines {
		if l.ChangeType == ChangeContext {
			assert.Positive(t, l.OldNum, "context lines should have positive OldNum")
			assert.Positive(t, l.NewNum, "context lines should have positive NewNum")
		}
	}
}

func TestParseUnifiedDiff_RemoveLineNumbers(t *testing.T) {
	raw := readFixture(t, "simple_remove.diff")
	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err)

	for _, l := range lines {
		if l.ChangeType == ChangeRemove {
			assert.Positive(t, l.OldNum, "removed lines should have positive OldNum")
			assert.Equal(t, 0, l.NewNum, "removed lines should have NewNum=0")
		}
	}
}

func TestGit_ChangedFiles(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create and modify a file
	writeFile(t, dir, "hello.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n")

	files, err := g.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"hello.go"}, files)
}

func TestGit_ChangedFiles_Staged(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "a.go", "package a\n")
	gitCmd(t, dir, "add", "a.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "a.go", "package a\n\nvar x = 1\n")
	gitCmd(t, dir, "add", "a.go")

	files, err := g.ChangedFiles("", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.go"}, files)
}

func TestGit_ChangedFiles_WithRef(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "b.go", "package b\n")
	gitCmd(t, dir, "add", "b.go")
	gitCmd(t, dir, "commit", "-m", "first")

	writeFile(t, dir, "b.go", "package b\n\nvar y = 2\n")
	gitCmd(t, dir, "add", "b.go")
	gitCmd(t, dir, "commit", "-m", "second")

	files, err := g.ChangedFiles("HEAD~1", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"b.go"}, files)
}

func TestGit_ChangedFiles_NoChanges(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "c.go", "package c\n")
	gitCmd(t, dir, "add", "c.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	files, err := g.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestGit_ChangedFiles_Error(t *testing.T) {
	g := NewGit("/nonexistent/repo")
	_, err := g.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get changed files")
}

func TestGit_FileDiff(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {\n}\n")
	gitCmd(t, dir, "add", "main.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")

	lines, err := g.FileDiff("", "main.go", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines, "expected non-empty diff lines")

	// verify we have both additions and context
	var hasAdd, hasCtx bool
	for _, l := range lines {
		switch l.ChangeType {
		case ChangeAdd:
			hasAdd = true
		case ChangeContext:
			hasCtx = true
		}
	}
	assert.True(t, hasAdd, "expected addition lines")
	assert.True(t, hasCtx, "expected context lines")
}

func TestGit_FileDiff_Error(t *testing.T) {
	g := NewGit("/nonexistent/repo")
	_, err := g.FileDiff("", "main.go", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get file diff")
}

func TestParseUnifiedDiff_LongLine(t *testing.T) {
	// build a diff with a line exceeding default scanner buffer (64KB)
	longContent := strings.Repeat("x", 100_000)
	raw := "diff --git a/big.js b/big.js\n--- a/big.js\n+++ b/big.js\n@@ -1,1 +1,2 @@\n context\n+" + longContent + "\n"

	lines, err := ParseUnifiedDiff(raw)
	require.NoError(t, err, "should handle lines up to 1MB without error")

	var hasAdd bool
	for _, l := range lines {
		if l.ChangeType == ChangeAdd && len(l.Content) == 100_000 {
			hasAdd = true
		}
	}
	assert.True(t, hasAdd, "should parse the long added line")
}

func TestGit_FileDiff_NoChanges(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "x.go", "package x\n")
	gitCmd(t, dir, "add", "x.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// no modifications, diff should be empty
	lines, err := g.FileDiff("", "x.go", false)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

// helpers

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // test fixture path
	require.NoError(t, err)
	return string(data)
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	require.NoError(t, err)
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test helper
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}
