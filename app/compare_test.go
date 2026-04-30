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
		opts, err := parseArgs(append(noConfigArgs(t), "--compare="+oldFile+":"+newFile))
		require.NoError(t, err)
		assert.Equal(t, oldFile+":"+newFile, opts.Compare)
	})

	t.Run("same paths allowed", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "x.md")
		require.NoError(t, os.WriteFile(f, []byte("same"), 0o600))
		opts, err := parseArgs(append(noConfigArgs(t), "--compare="+f+":"+f))
		require.NoError(t, err)
		assert.Equal(t, f+":"+f, opts.Compare)
	})

	t.Run("rejects directory old path", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "b.md")
		require.NoError(t, os.WriteFile(f, []byte("new"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare="+dir+":"+f))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare old path must be a regular file")
	})

	t.Run("rejects directory new path", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "a.md")
		require.NoError(t, os.WriteFile(f, []byte("old"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare="+f+":"+dir))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare new path must be a regular file")
	})

	t.Run("rejects nonexistent old path", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "b.md")
		require.NoError(t, os.WriteFile(f, []byte("new"), 0o600))
		_, err := parseArgs(append(noConfigArgs(t), "--compare="+dir+"/nonexistent.md:"+f))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare old path:")
	})

	t.Run("missing colon", func(t *testing.T) {
		_, err := parseArgs(append(noConfigArgs(t), "--compare=/tmp/a.md"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare requires old:new format")
	})

	t.Run("empty old path", func(t *testing.T) {
		_, err := parseArgs(append(noConfigArgs(t), "--compare=:/tmp/b.md"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare requires old:new format")
	})

	t.Run("empty new path", func(t *testing.T) {
		_, err := parseArgs(append(noConfigArgs(t), "--compare=/tmp/a.md:"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--compare requires old:new format")
	})
}

func TestParseArgs_CompareConflicts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "refs base", args: []string{"--compare=/tmp/a:/tmp/b", "HEAD~1"}, want: "--compare cannot be used with refs"},
		{name: "refs two", args: []string{"--compare=/tmp/a:/tmp/b", "main", "feature"}, want: "--compare cannot be used with refs"},
		{name: "staged", args: []string{"--compare=/tmp/a:/tmp/b", "--staged"}, want: "--compare cannot be used with --staged"},
		{name: "only", args: []string{"--compare=/tmp/a:/tmp/b", "--only", "main.go"}, want: "--compare cannot be used with --only"},
		{name: "all-files", args: []string{"--compare=/tmp/a:/tmp/b", "--all-files"}, want: "--compare cannot be used with --all-files"},
		{name: "stdin", args: []string{"--compare=/tmp/a:/tmp/b", "--stdin"}, want: "--compare cannot be used with --stdin"},
		{name: "include", args: []string{"--compare=/tmp/a:/tmp/b", "--include", "src"}, want: "--compare cannot be used with --include"},
		{name: "exclude", args: []string{"--compare=/tmp/a:/tmp/b", "--exclude", "vendor"}, want: "--compare cannot be used with --exclude"},
		{name: "annotations", args: []string{"--compare=/tmp/a:/tmp/b", "--annotations", "/tmp/a.md"}, want: "--compare cannot be used with --annotations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(append(noConfigArgs(t), tt.args...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
