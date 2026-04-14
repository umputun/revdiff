package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWriteFile(t *testing.T) {
	readFile := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path) //nolint:gosec // test helper reads from temp dir
		require.NoError(t, err)
		return string(data)
	}

	t.Run("writes new file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, AtomicWriteFile(path, []byte("hello")))
		assert.Equal(t, "hello", readFile(t, path))
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
		require.NoError(t, AtomicWriteFile(path, []byte("new")))
		assert.Equal(t, "new", readFile(t, path))
	})

	t.Run("writes empty file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")
		require.NoError(t, AtomicWriteFile(path, []byte{}))
		assert.Empty(t, readFile(t, path))
	})

	t.Run("fails on nonexistent directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "no-such-dir", "file.txt")
		err := AtomicWriteFile(path, []byte("data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating temp file")
	})

	t.Run("no temp file left on success", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "clean.txt")
		require.NoError(t, AtomicWriteFile(path, []byte("data")))
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Len(t, entries, 1, "only the target file should remain")
		assert.Equal(t, "clean.txt", entries[0].Name())
	})

	t.Run("cleans path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sub", "..", "clean.txt")
		require.NoError(t, AtomicWriteFile(path, []byte("ok")))
		assert.Equal(t, "ok", readFile(t, filepath.Join(dir, "clean.txt")))
	})

	t.Run("fails when target is a directory", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "occupied")
		require.NoError(t, os.Mkdir(path, 0o750))
		err := AtomicWriteFile(path, []byte("data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "renaming temp file")
		// verify no temp files leaked
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Len(t, entries, 1, "only the original directory should remain")
	})

	t.Run("fails when directory becomes read-only before write", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "ro")
		require.NoError(t, os.Mkdir(sub, 0o750))
		// pre-create a file so rename target exists
		path := filepath.Join(sub, "target.txt")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
		// make directory read-only so CreateTemp fails
		require.NoError(t, os.Chmod(sub, 0o555))       //nolint:gosec // intentional read-only for test
		t.Cleanup(func() { _ = os.Chmod(sub, 0o750) }) //nolint:gosec // restore perms for cleanup
		err := AtomicWriteFile(path, []byte("new"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating temp file")
		// original file untouched
		assert.Equal(t, "old", readFile(t, path))
	})
}
