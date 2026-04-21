package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryReader_ChangedFiles(t *testing.T) {
	dir := setupTestRepo(t)

	// create and commit several files
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg"), 0o750))
	writeFile(t, dir, "main.go", "package main\n")
	writeFile(t, dir, "readme.md", "# readme\n")
	writeFileAt(t, filepath.Join(dir, "pkg", "lib.go"), "package pkg\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "main.go"}, {Path: "pkg/lib.go"}, {Path: "readme.md"}}, files)
}

func TestDirectoryReader_ChangedFiles_sorted(t *testing.T) {
	dir := setupTestRepo(t)

	// create files that would be unsorted alphabetically if not sorted
	writeFile(t, dir, "z.go", "package z\n")
	writeFile(t, dir, "a.go", "package a\n")
	writeFile(t, dir, "m.go", "package m\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "a.go"}, {Path: "m.go"}, {Path: "z.go"}}, files)
}

func TestDirectoryReader_ChangedFiles_emptyRepo(t *testing.T) {
	dir := setupTestRepo(t)

	// empty repo with no committed files — need at least one commit for ls-files to work
	writeFile(t, dir, "dummy", "x\n")
	gitCmd(t, dir, "add", "dummy")
	gitCmd(t, dir, "commit", "-m", "initial")
	// remove the file
	require.NoError(t, os.Remove(filepath.Join(dir, "dummy")))
	gitCmd(t, dir, "rm", "dummy")
	gitCmd(t, dir, "commit", "-m", "remove all")

	dr := NewDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestDirectoryReader_ChangedFiles_ignoresRefAndStaged(t *testing.T) {
	dir := setupTestRepo(t)

	writeFile(t, dir, "file.go", "package main\n")
	gitCmd(t, dir, "add", "file.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)

	// passing ref and staged=true should not affect the result
	files, err := dr.ChangedFiles("HEAD~1", true)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "file.go"}}, files)
}

func TestDirectoryReader_ChangedFiles_binaryFiles(t *testing.T) {
	dir := setupTestRepo(t)

	// create a text file and a binary file
	writeFile(t, dir, "code.go", "package main\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image.png"), []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}, 0o600))
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	// binary files should be listed too — git ls-files lists all tracked files
	assert.Equal(t, []FileEntry{{Path: "code.go"}, {Path: "image.png"}}, files)
}

func TestDirectoryReader_ChangedFiles_deletedFile(t *testing.T) {
	dir := setupTestRepo(t)

	// create and commit two files
	writeFile(t, dir, "keep.go", "package main\n")
	writeFile(t, dir, "deleted.go", "package main\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	// delete one file locally without staging the deletion
	require.NoError(t, os.Remove(filepath.Join(dir, "deleted.go")))

	dr := NewDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	// deleted file should not appear in the list
	assert.Equal(t, []FileEntry{{Path: "keep.go"}}, files)
}

func TestDirectoryReader_ChangedFiles_brokenSymlink(t *testing.T) {
	dir := setupTestRepo(t)

	// create a regular file and a symlink pointing to a nonexistent target
	writeFile(t, dir, "real.go", "package main\n")
	require.NoError(t, os.Symlink("/nonexistent/target", filepath.Join(dir, "broken.link")))
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	// broken symlink should still appear — it's a valid tracked entry
	assert.Equal(t, []FileEntry{{Path: "broken.link"}, {Path: "real.go"}}, files)
}

func TestDirectoryReader_FileDiff_brokenSymlink(t *testing.T) {
	dir := setupTestRepo(t)

	// create a regular file and a broken symlink, commit both
	writeFile(t, dir, "real.go", "package main\n")
	require.NoError(t, os.Symlink("/nonexistent/target", filepath.Join(dir, "broken.link")))
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)

	// broken symlink should be in the file list
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Contains(t, files, FileEntry{Path: "broken.link"})

	// FileDiff should return a placeholder, not an error
	lines, err := dr.FileDiff("", "broken.link", false, 0)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "(broken symlink)", lines[0].Content)
	assert.Equal(t, ChangeContext, lines[0].ChangeType)
}

func TestDirectoryReader_ChangedFiles_notGitRepo(t *testing.T) {
	dir := t.TempDir() // not a git repo
	dr := NewDirectoryReader(dir)
	_, err := dr.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git ls-files")
}

func TestDirectoryReader_FileDiff(t *testing.T) {
	dir := setupTestRepo(t)

	content := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	writeFile(t, dir, "main.go", content)
	gitCmd(t, dir, "add", "main.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	lines, err := dr.FileDiff("", "main.go", false, 0)
	require.NoError(t, err)
	require.Len(t, lines, 7)

	// all lines should be context type with matching line numbers
	for i, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType, "line %d should be context", i+1)
		assert.Equal(t, i+1, l.OldNum)
		assert.Equal(t, i+1, l.NewNum)
	}
	assert.Equal(t, "package main", lines[0].Content)
	assert.Equal(t, `	fmt.Println("hello")`, lines[5].Content)
}

func TestDirectoryReader_FileDiff_emptyFile(t *testing.T) {
	dir := setupTestRepo(t)

	writeFile(t, dir, "empty.go", "")
	gitCmd(t, dir, "add", "empty.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	lines, err := dr.FileDiff("", "empty.go", false, 0)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestDirectoryReader_FileDiff_binaryFile(t *testing.T) {
	dir := setupTestRepo(t)

	// create and commit a binary file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image.png"),
		[]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, 0o600))
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	lines, err := dr.FileDiff("", "image.png", false, 0)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "(binary file)", lines[0].Content)
	assert.Equal(t, ChangeContext, lines[0].ChangeType)
}

func TestDirectoryReader_FileDiff_nonexistentFile(t *testing.T) {
	dir := setupTestRepo(t)

	writeFile(t, dir, "exists.go", "package main\n")
	gitCmd(t, dir, "add", "exists.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	_, err := dr.FileDiff("", "missing.go", false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat file")
}

func TestDirectoryReader_FileDiff_subdirectory(t *testing.T) {
	dir := setupTestRepo(t)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "sub"), 0o750))
	writeFileAt(t, filepath.Join(dir, "pkg", "sub", "lib.go"), "package sub\n\nvar x = 1\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)
	lines, err := dr.FileDiff("", "pkg/sub/lib.go", false, 0)
	require.NoError(t, err)
	require.Len(t, lines, 3)
	assert.Equal(t, "package sub", lines[0].Content)
	assert.Equal(t, "var x = 1", lines[2].Content)
}

func TestDirectoryReader_FullPipeline(t *testing.T) {
	dir := setupTestRepo(t)

	// create a small project structure
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd"), 0o750))
	writeFile(t, dir, "go.mod", "module example\n\ngo 1.21\n")
	writeFileAt(t, filepath.Join(dir, "cmd", "main.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "lib.go", "package example\n\nvar Version = \"1.0\"\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")

	dr := NewDirectoryReader(dir)

	// list all files
	files, err := dr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "cmd/main.go"}, {Path: "go.mod"}, {Path: "lib.go"}}, files)

	// read each file
	for _, f := range files {
		lines, readErr := dr.FileDiff("", f.Path, false, 0)
		require.NoError(t, readErr, "failed to read %s", f.Path)
		require.NotEmpty(t, lines, "file %s should not be empty", f.Path)
		for _, l := range lines {
			assert.Equal(t, ChangeContext, l.ChangeType, "all lines in %s should be context", f.Path)
		}
	}
}
