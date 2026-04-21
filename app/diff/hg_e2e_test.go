package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHg_E2E_FullPipeline tests the complete flow: detect VCS → create renderer → list files → diff → blame.
// mirrors what main.go does for a real hg repo.
func TestHg_E2E_FullPipeline(t *testing.T) {
	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg not available")
	}

	dir := setupHgRepo(t)

	// create initial content
	writeFile(t, dir, "hello.txt", "line one\nline two\nline three\n")
	writeFile(t, dir, "readme.md", "# readme\n")
	hgCmd(t, dir, "add", "hello.txt", "readme.md")
	hgCmd(t, dir, "commit", "-m", "initial commit")

	// make changes
	writeFile(t, dir, "hello.txt", "line one modified\nline two\nline three\nline four\n")
	writeFile(t, dir, "new.txt", "new file content\n")
	hgCmd(t, dir, "add", "new.txt")
	writeFile(t, dir, "untracked.txt", "untracked\n")

	// step 1: detect VCS from a subdirectory
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	vcsType, root := DetectVCS(sub)
	assert.Equal(t, VCSHg, vcsType)
	assert.Equal(t, dir, root)

	// step 2: create Hg renderer (same as main.go)
	h := NewHg(root)

	// step 3: list changed files
	entries, err := h.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 2, "expected hello.txt (modified) and new.txt (added)")

	paths := FileEntryPaths(entries)
	assert.Contains(t, paths, "hello.txt")
	assert.Contains(t, paths, "new.txt")

	// verify statuses
	for _, e := range entries {
		switch e.Path {
		case "hello.txt":
			assert.Equal(t, FileModified, e.Status)
		case "new.txt":
			assert.Equal(t, FileAdded, e.Status)
		}
	}

	// step 4: get file diff (goes through parseUnifiedDiff)
	lines, err := h.FileDiff("", "hello.txt", false, 0)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	// verify diff content
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
	assert.Equal(t, 1, removes, "expected 1 removed line (line one)")
	assert.Equal(t, 2, adds, "expected 2 added lines (line one modified + line four)")
	assert.Equal(t, 2, ctx, "expected 2 context lines (line two + line three)")

	// step 5: new file diff should be all additions
	newLines, err := h.FileDiff("", "new.txt", false, 0)
	require.NoError(t, err)
	require.NotEmpty(t, newLines)
	for _, l := range newLines {
		if l.ChangeType == ChangeDivider {
			continue
		}
		assert.Equal(t, ChangeAdd, l.ChangeType)
	}

	// step 6: blame (uses hg annotate)
	blame, err := h.FileBlame("", "hello.txt", false)
	require.NoError(t, err)
	// blame only covers committed lines (3 from initial commit)
	assert.Len(t, blame, 3)
	for _, bl := range blame {
		assert.Equal(t, "Test User", bl.Author)
		assert.False(t, bl.Time.IsZero())
	}

	// step 7: untracked files
	untracked, err := h.UntrackedFiles()
	require.NoError(t, err)
	assert.Contains(t, untracked, "untracked.txt")
	assert.NotContains(t, untracked, "hello.txt")
	assert.NotContains(t, untracked, "new.txt")
}

// TestHg_E2E_RefDiff tests diffs between committed revisions.
func TestHg_E2E_RefDiff(t *testing.T) {
	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg not available")
	}

	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "original\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "rev 0")

	writeFile(t, dir, "hello.txt", "modified\n")
	hgCmd(t, dir, "commit", "-m", "rev 1")

	writeFile(t, dir, "hello.txt", "final\n")
	hgCmd(t, dir, "commit", "-m", "rev 2")

	// diff rev 0 to rev 2
	entries, err := h.ChangedFiles("0..2", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Path)

	lines, err := h.FileDiff("0..2", "hello.txt", false, 0)
	require.NoError(t, err)

	var adds, removes int
	for _, l := range lines {
		switch l.ChangeType { //nolint:exhaustive // only counting relevant types
		case ChangeAdd:
			adds++
		case ChangeRemove:
			removes++
		}
	}
	assert.Equal(t, 1, adds, "expected 'final' added")
	assert.Equal(t, 1, removes, "expected 'original' removed")

	// blame at specific revision
	blame, err := h.FileBlame("0..2", "hello.txt", false)
	require.NoError(t, err)
	assert.Len(t, blame, 1) // "final" is one line
	assert.Equal(t, "Test User", blame[1].Author)
}

// TestHg_E2E_CommitLog exercises (*Hg).CommitLog against a real hg binary, covering
// single-ref, range, and triple-dot revset translations plus round-trip of hash,
// author, date, subject, and body.
func TestHg_E2E_CommitLog(t *testing.T) {
	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg not available")
	}

	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "a.txt", "a\n")
	hgCmd(t, dir, "add", "a.txt")
	hgCmd(t, dir, "commit", "-m", "first subject\n\nbody of first commit")

	writeFile(t, dir, "a.txt", "a\nb\n")
	hgCmd(t, dir, "commit", "-m", "second subject")

	writeFile(t, dir, "a.txt", "a\nb\nc\n")
	hgCmd(t, dir, "commit", "-m", "third subject\n\nthird body line 1\nthird body line 2")

	t.Run("single ref X excludes X for parity with git/jj", func(t *testing.T) {
		// rev 0 → X::. - X includes revs 1, 2 (X itself excluded to match
		// git's X..HEAD and jj's X..@ semantics)
		commits, err := h.CommitLog("0")
		require.NoError(t, err)
		require.Len(t, commits, 2)
		subjects := map[string]string{}
		bodies := map[string]string{}
		for _, c := range commits {
			subjects[c.Subject] = c.Subject
			bodies[c.Subject] = c.Body
		}
		assert.NotContains(t, subjects, "first subject", "X::. - X must exclude X")
		assert.Contains(t, subjects, "second subject")
		assert.Contains(t, subjects, "third subject")
		assert.Empty(t, bodies["second subject"])
		assert.Equal(t, "third body line 1\nthird body line 2", bodies["third subject"])

		// all dates parsed, all authors populated
		for _, c := range commits {
			assert.False(t, c.Date.IsZero(), "date should be populated")
			assert.Contains(t, c.Author, "Test User")
			assert.NotEmpty(t, c.Hash)
		}
	})

	t.Run("explicit range X..Y only includes descendants of X through Y", func(t *testing.T) {
		// rev 0..2 — excludes rev 0, includes rev 1 and rev 2
		commits, err := h.CommitLog("0..2")
		require.NoError(t, err)
		require.Len(t, commits, 2)
		got := map[string]bool{commits[0].Subject: true, commits[1].Subject: true}
		assert.True(t, got["second subject"])
		assert.True(t, got["third subject"])
		assert.False(t, got["first subject"], "X..Y must exclude X")
	})

	t.Run("range excluding tip returns earlier commits", func(t *testing.T) {
		// 0..1 — excludes rev 0, includes rev 1 only
		commits, err := h.CommitLog("0..1")
		require.NoError(t, err)
		require.Len(t, commits, 1)
		assert.Equal(t, "second subject", commits[0].Subject)
	})

	t.Run("invalid ref returns wrapped error", func(t *testing.T) {
		_, err := h.CommitLog("not-a-real-ref")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commit log")
	})
}

// TestGit_E2E_StillWorks verifies git repos still work after the refactoring.
func TestGit_E2E_StillWorks(t *testing.T) {
	dir := setupTestRepo(t)

	writeFile(t, dir, "hello.txt", "line one\n")
	gitCmd(t, dir, "add", "hello.txt")
	gitCmd(t, dir, "commit", "-m", "init")

	writeFile(t, dir, "hello.txt", "line one modified\n")

	// detect VCS
	vcsType, root := DetectVCS(dir)
	assert.Equal(t, VCSGit, vcsType)
	assert.Equal(t, dir, root)

	// use Git renderer
	g := NewGit(root)

	entries, err := g.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Path)
	assert.Equal(t, FileModified, entries[0].Status)

	lines, err := g.FileDiff("", "hello.txt", false, 0)
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

	// blame on worktree — uncommitted line shows "Not Committed Yet"
	blame, err := g.FileBlame("", "hello.txt", false)
	require.NoError(t, err)
	assert.Len(t, blame, 1)
	assert.NotEmpty(t, blame[1].Author)
}
