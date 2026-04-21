package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDescription_Empty(t *testing.T) {
	got, err := resolveDescription(options{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolveDescription_InlineOverridesFile(t *testing.T) {
	// --description wins when both are set; validation in parseArgs already
	// rejects that combination upstream, but the resolver should still be robust.
	got, err := resolveDescription(options{Description: "inline", DescriptionFile: "/nonexistent"})
	require.NoError(t, err)
	assert.Equal(t, "inline", got)
}

func TestResolveDescription_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "desc.md")
	content := "# Agent notes\n\n- point one\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	got, err := resolveDescription(options{DescriptionFile: path})
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestResolveDescription_FromFileMissing(t *testing.T) {
	_, err := resolveDescription(options{DescriptionFile: filepath.Join(t.TempDir(), "missing.md")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read --description-file")
}

func TestResolveDescription_FromFileDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveDescription(options{DescriptionFile: dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}

func TestResolveDescription_FromFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.md")
	// write a file just over the 1 MiB cap
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("x"), descriptionMaxBytes+1), 0o600))

	_, err := resolveDescription(options{DescriptionFile: path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max")
}

func TestResolveDescription_FromFileRelativePath(t *testing.T) {
	// create a file in CWD so a relative path resolves
	dir := t.TempDir()
	name := "rel-desc.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("hello"), 0o600))
	t.Chdir(dir)

	got, err := resolveDescription(options{DescriptionFile: name})
	require.NoError(t, err)
	assert.Equal(t, "hello", got)
}
