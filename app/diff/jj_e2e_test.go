package diff

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJj_E2E_CommitLog exercises (*Jj).CommitLog against a real jj binary, covering
// single-ref and range revset translations plus round-trip of commit_id, author,
// date, subject, and body.
func TestJj_E2E_CommitLog(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not available")
	}

	dir := setupJjRepo(t)
	j := NewJj(dir)

	// build three commits by describing @ and creating new children.
	writeFile(t, dir, "a.txt", "a\n")
	jjCmd(t, dir, "describe", "-m", "first subject\n\nbody of first commit", "--quiet")
	jjCmd(t, dir, "new", "-m", "second subject", "--quiet")
	writeFile(t, dir, "a.txt", "a\nb\n")
	jjCmd(t, dir, "new", "-m", "third subject\n\nthird body line 1\nthird body line 2", "--quiet")
	writeFile(t, dir, "a.txt", "a\nb\nc\n")

	// capture the commit_id of the first and second described commits so we can
	// build explicit ranges without depending on change_id prefix instability.
	// Layout: @-- = first subject, @- = second subject, @ = third subject (plus
	// working-copy edits).
	firstCommitID := jjCommitID(t, dir, "@--")
	secondCommitID := jjCommitID(t, dir, "@-")

	t.Run("single ref X selects X..@ and filters empty @", func(t *testing.T) {
		// firstCommitID..@ — excludes firstCommitID, includes second + third.
		// jj auto-creates an empty working-copy commit at @ after each `jj new`;
		// the parser drops it (current_working_copy && empty description) so only
		// real commits show up.
		commits, err := j.CommitLog(firstCommitID)
		require.NoError(t, err)
		subjects := map[string]string{}
		bodies := map[string]string{}
		for _, c := range commits {
			assert.NotEmptyf(t, c.Subject, "synthetic working-copy @ must be filtered; got %+v", c)
			subjects[c.Subject] = c.Subject
			bodies[c.Subject] = c.Body
		}
		assert.Contains(t, subjects, "second subject")
		assert.Contains(t, subjects, "third subject")
		assert.NotContains(t, subjects, "first subject", "X..@ must exclude X")
		assert.Empty(t, bodies["second subject"])
		assert.Equal(t, "third body line 1\nthird body line 2", bodies["third subject"])

		for _, c := range commits {
			assert.False(t, c.Date.IsZero(), "date should be populated")
			assert.Contains(t, c.Author, "Test User")
			assert.Regexp(t, `^[0-9a-f]{40}$`, c.Hash)
		}
	})

	t.Run("non-working-copy commit with empty description is kept", func(t *testing.T) {
		// jj permits real commits without descriptions; the parser must keep them
		// since only the synthetic working-copy @ placeholder should be filtered.
		// Sequence: base (described) → @ (no desc) → @ (described) → @ (working copy).
		// The middle commit has no description but is not the working copy, so it
		// must show up in the popup.
		dir2 := setupJjRepo(t)
		j2 := NewJj(dir2)
		writeFile(t, dir2, "a.txt", "a\n")
		jjCmd(t, dir2, "describe", "-m", "base", "--quiet")
		baseID := jjCommitID(t, dir2, "@")
		jjCmd(t, dir2, "new", "--quiet") // empty-description child becomes @
		writeFile(t, dir2, "a.txt", "a\nb\n")
		jjCmd(t, dir2, "new", "-m", "tip", "--quiet") // @ now points at a described child

		commits, err := j2.CommitLog(baseID)
		require.NoError(t, err)
		hasEmpty := false
		hasTip := false
		for _, c := range commits {
			switch {
			case c.Subject == "" && c.Body == "":
				hasEmpty = true
				assert.Regexp(t, `^[0-9a-f]{40}$`, c.Hash)
			case c.Subject == "tip":
				hasTip = true
			}
		}
		assert.True(t, hasEmpty, "commits with empty descriptions that are not the working-copy @ must be kept; got %+v", commits)
		assert.True(t, hasTip, "described commits must still appear; got %+v", commits)
	})

	t.Run("explicit range X..Y only includes descendants of X through Y", func(t *testing.T) {
		// first..second — excludes first, includes second only.
		commits, err := j.CommitLog(firstCommitID + ".." + secondCommitID)
		require.NoError(t, err)
		require.Len(t, commits, 1)
		assert.Equal(t, "second subject", commits[0].Subject)
		assert.Empty(t, commits[0].Body)
	})

	t.Run("invalid ref returns wrapped error", func(t *testing.T) {
		_, err := j.CommitLog("not-a-real-ref")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commit log")
	})
}

// jjCommitID resolves a revset to a full 40-char commit id via
// `jj log --no-graph -T 'commit_id' -r <rev>`.
func jjCommitID(t *testing.T, dir, rev string) string {
	t.Helper()
	cmd := exec.Command("jj", "log", "--no-graph", "--no-pager", "--color=never", "-T", "commit_id", "-r", rev) //nolint:gosec // args constructed internally
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "jj log for %q failed: %s", rev, string(out))
	return strings.TrimSpace(string(out))
}
