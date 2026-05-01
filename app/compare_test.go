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
		assert.NotEmpty(t, opts.compareAbsOld)
		assert.NotEmpty(t, opts.compareAbsNew)
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
	common := []string{"--compare-old=/tmp/a", "--compare-new=/tmp/b"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "refs base", args: append(common, "HEAD~1"), want: "--compare-old/--compare-new cannot be used with refs"},
		{name: "refs two", args: append(common, "main", "feature"), want: "--compare-old/--compare-new cannot be used with refs"},
		{name: "staged", args: append(common, "--staged"), want: "--compare-old/--compare-new cannot be used with --staged"},
		{name: "only", args: append(common, "--only", "main.go"), want: "--compare-old/--compare-new cannot be used with --only"},
		{name: "all-files", args: append(common, "--all-files"), want: "--compare-old/--compare-new cannot be used with --all-files"},
		{name: "stdin", args: append(common, "--stdin"), want: "--compare-old/--compare-new cannot be used with --stdin"},
		{name: "include", args: append(common, "--include", "src"), want: "--compare-old/--compare-new cannot be used with --include"},
		{name: "exclude", args: append(common, "--exclude", "vendor"), want: "--compare-old/--compare-new cannot be used with --exclude"},
		{name: "annotations", args: append(common, "--annotations", "/tmp/a.md"), want: "--compare-old/--compare-new cannot be used with --annotations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(append(noConfigArgs(t), tt.args...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
