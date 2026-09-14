package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgs_CompareFlag(t *testing.T) {
	t.Run("valid flag", func(t *testing.T) {
		dir := t.TempDir()
		oldFile := filepath.Join(dir, "a.md")
		newFile := filepath.Join(dir, "b.md")
		require.NoError(t, os.WriteFile(oldFile, []byte("old"), 0o600))
		require.NoError(t, os.WriteFile(newFile, []byte("new"), 0o600))
		opts, err := parseArgs(append(noConfigArgs(t), "--compare-old="+oldFile, "--compare-new="+newFile))
		require.NoError(t, err)
		assert.Equal(t, oldFile, opts.CompareOld)
		assert.Equal(t, newFile, opts.CompareNew)
		// compareAbsOld/New should equal filepath.Abs of the input paths —
		// the contract validateCompareFlag actually produces. NotEmpty would
		// confirm population but not correctness (e.g. swapped fields would
		// pass a NotEmpty check silently).
		expectedOldAbs, err := filepath.Abs(oldFile)
		require.NoError(t, err)
		expectedNewAbs, err := filepath.Abs(newFile)
		require.NoError(t, err)
		assert.Equal(t, expectedOldAbs, opts.compareAbsOld)
		assert.Equal(t, expectedNewAbs, opts.compareAbsNew)
	})

	t.Run("same paths allowed", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "x.md")
		require.NoError(t, os.WriteFile(f, []byte("same"), 0o600))
		opts, err := parseArgs(append(noConfigArgs(t), "--compare-old="+f, "--compare-new="+f))
		require.NoError(t, err)
		assert.Equal(t, f, opts.CompareOld)
		assert.Equal(t, f, opts.CompareNew)
	})

	t.Run("rejects directory old path", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "b.md")
		require.NoError(t, os.WriteFile(f, []byte("new"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare-old="+dir, "--compare-new="+f))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare-old must be a regular file")
	})

	t.Run("rejects directory new path", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "a.md")
		require.NoError(t, os.WriteFile(f, []byte("old"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare-old="+f, "--compare-new="+dir))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare-new must be a regular file")
	})

	t.Run("rejects nonexistent old path", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "b.md")
		require.NoError(t, os.WriteFile(f, []byte("new"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare-old="+dir+"/nonexistent.md", "--compare-new="+f))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare-old:")
	})

	t.Run("only old set", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "a.md")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare-old="+f))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare-old and --compare-new must be used together")
	})

	t.Run("only new set", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "b.md")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare-new="+f))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare-old and --compare-new must be used together")
	})
}

func TestParseArgs_CompareConflicts(t *testing.T) {
	// real files so a future stat-first reordering would not silently green
	// these tests — the assertion under test is the conflict, not the stat.
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "a.md")
	newFile := filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(oldFile, []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(newFile, []byte("b"), 0o600))
	common := []string{"--compare-old=" + oldFile, "--compare-new=" + newFile}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "refs base", args: append(append([]string{}, common...), "HEAD~1"), want: "--compare-old/--compare-new cannot be used with refs"},
		{name: "refs two", args: append(append([]string{}, common...), "main", "feature"), want: "--compare-old/--compare-new cannot be used with refs"},
		{name: "staged", args: append(append([]string{}, common...), "--staged"), want: "--compare-old/--compare-new cannot be used with --staged"},
		{name: "only", args: append(append([]string{}, common...), "--only", "main.go"), want: "--compare-old/--compare-new cannot be used with --only"},
		{name: "all-files", args: append(append([]string{}, common...), "--all-files"), want: "--compare-old/--compare-new cannot be used with --all-files"},
		{name: "stdin", args: append(append([]string{}, common...), "--stdin"), want: "--compare-old/--compare-new cannot be used with --stdin"},
		{name: "include", args: append(append([]string{}, common...), "--include", "src"), want: "--compare-old/--compare-new cannot be used with --include"},
		{name: "exclude", args: append(append([]string{}, common...), "--exclude", "vendor"), want: "--compare-old/--compare-new cannot be used with --exclude"},
		{name: "annotations", args: append(append([]string{}, common...), "--annotations", "/tmp/a.md"), want: "--compare-old/--compare-new cannot be used with --annotations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(append(noConfigArgs(t), tt.args...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
