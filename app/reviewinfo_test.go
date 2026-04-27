package main

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestReviewInfoFromOptions(t *testing.T) {
	t.Run("stdin overrides VCS as 'stdin'", func(t *testing.T) {
		opts := options{Stdin: true, StdinName: "buf"}
		info := reviewInfoFromOptions(opts, "/repo", diff.VCSGit, "")
		assert.Equal(t, "stdin", info.VCS)
		assert.Equal(t, "buf", info.StdinName)
		assert.True(t, info.Stdin)
	})

	t.Run("missing VCS becomes 'none'", func(t *testing.T) {
		info := reviewInfoFromOptions(options{}, "/tmp", "", "")
		assert.Equal(t, "none", info.VCS)
	})

	t.Run("VCS string is propagated", func(t *testing.T) {
		info := reviewInfoFromOptions(options{}, "/repo", diff.VCSJJ, "")
		assert.Equal(t, string(diff.VCSJJ), info.VCS)
	})

	t.Run("staged is ignored for VCSes without staging area", func(t *testing.T) {
		info := reviewInfoFromOptions(options{Staged: true}, "/repo", diff.VCSHg, "")
		assert.False(t, info.Staged)
	})

	t.Run("staged is preserved for git", func(t *testing.T) {
		info := reviewInfoFromOptions(options{Staged: true}, "/repo", diff.VCSGit, "")
		assert.True(t, info.Staged)
	})

	t.Run("list slices are decoupled from caller", func(t *testing.T) {
		opts := options{Only: []string{"a", "b"}}
		info := reviewInfoFromOptions(opts, "", "", "")
		// mutating the original must not affect the captured slice
		opts.Only[0] = "x"
		assert.Equal(t, []string{"a", "b"}, info.Only, "Only must be defensively copied")
	})

	t.Run("description is propagated", func(t *testing.T) {
		info := reviewInfoFromOptions(options{}, "", "", "agent says hi\n\nrefactor done")
		assert.Equal(t, "agent says hi\n\nrefactor done", info.Description)
	})

	t.Run("enabled is always true from constructor", func(t *testing.T) {
		// reviewInfoFromOptions is the production constructor; it must always
		// return Enabled=true so the review-info subsystem activates. Only
		// focused tests using the zero-value config rely on Enabled=false as
		// the off-switch.
		info := reviewInfoFromOptions(options{}, "", "", "")
		assert.True(t, info.Enabled)
	})
}

func TestResolveDescription(t *testing.T) {
	t.Run("no flags returns empty", func(t *testing.T) {
		got, err := resolveDescription(options{})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("inline description passes through verbatim", func(t *testing.T) {
		got, err := resolveDescription(options{Description: "agent prose"})
		require.NoError(t, err)
		assert.Equal(t, "agent prose", got)
	})

	t.Run("description-file reads file contents", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "desc.md")
		body := "# heading\n\nbody text\n"
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

		got, err := resolveDescription(options{DescriptionFile: path})
		require.NoError(t, err)
		assert.Equal(t, body, got)
	})

	t.Run("description-file missing returns wrapped error", func(t *testing.T) {
		_, err := resolveDescription(options{DescriptionFile: "/nonexistent/path.md"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--description-file")
	})

	t.Run("description-file exceeding size cap is rejected", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "huge.md")
		oversized := make([]byte, maxDescriptionFileSize+1)
		require.NoError(t, os.WriteFile(path, oversized, 0o600))

		_, err := resolveDescription(options{DescriptionFile: path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})

	t.Run("description-file must be regular", func(t *testing.T) {
		dir := t.TempDir()
		_, err := resolveDescription(options{DescriptionFile: dir})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "regular file")
	})

	t.Run("description-file at exact cap is accepted", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "boundary.md")
		atCap := make([]byte, maxDescriptionFileSize)
		require.NoError(t, os.WriteFile(path, atCap, 0o600))

		got, err := resolveDescription(options{DescriptionFile: path})
		require.NoError(t, err)
		assert.Len(t, got, maxDescriptionFileSize)
	})

	t.Run("description and description-file together return error", func(t *testing.T) {
		// parseArgs rejects this at the CLI; resolveDescription enforces the
		// same invariant for any direct programmatic call so the helper's
		// contract matches the CLI rather than silently picking a winner.
		dir := t.TempDir()
		path := filepath.Join(dir, "desc.md")
		require.NoError(t, os.WriteFile(path, []byte("from file"), 0o600))

		_, err := resolveDescription(options{Description: "from flag", DescriptionFile: path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("description-file pointing at a FIFO is rejected", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("FIFOs not supported on Windows")
		}
		dir := t.TempDir()
		fifo := filepath.Join(dir, "pipe")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Skipf("mkfifo unsupported: %v", err)
		}
		_, err := resolveDescription(options{DescriptionFile: fifo})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "regular file")
	})
}

func TestParseArgs_DescriptionMutuallyExclusive(t *testing.T) {
	args := append(noConfigArgs(t), "--description=foo", "--description-file=bar")
	_, err := parseArgs(args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}
