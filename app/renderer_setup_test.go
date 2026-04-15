package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

// mockDiffRenderer implements ui.Renderer for composition tests.
type mockDiffRenderer struct {
	files []diff.FileEntry
}

func (m *mockDiffRenderer) ChangedFiles(string, bool) ([]diff.FileEntry, error) {
	return m.files, nil
}

func (m *mockDiffRenderer) FileDiff(string, string, bool) ([]diff.DiffLine, error) {
	return nil, nil
}

func TestMakeGitRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, []string{"file.md"}, nil, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithoutOnly(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, nil, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// with no --only, returns *diff.Git directly without FallbackRenderer wrapper
	assert.IsType(t, &diff.Git{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeNoVCSRenderer_WithOnly(t *testing.T) {
	tmpDir := t.TempDir()

	renderer, workDir, err := makeNoVCSRenderer([]string{"file.md"}, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FileReader{}, renderer)
	assert.Equal(t, tmpDir, workDir)
}

func TestMakeNoVCSRenderer_NoOnly(t *testing.T) {
	renderer, workDir, err := makeNoVCSRenderer(nil, "/tmp")
	require.Error(t, err)
	assert.Nil(t, renderer)
	assert.Empty(t, workDir)
	assert.Contains(t, err.Error(), "no git or mercurial repository found")
}

func TestMakeGitRenderer_AllFiles(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, nil, nil, true, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.DirectoryReader{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_AllFilesUnsupported(t *testing.T) {
	_, _, err := makeHgRenderer(diff.NewHg(""), nil, nil, nil, true, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files is not supported in mercurial")
}

func TestMakeHgRenderer_Default(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, nil, nil, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.Hg{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, []string{"file.go"}, nil, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, nil, nil, []string{"vendor"}, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, nil, []string{"vendor"}, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_AllFilesWithExclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, nil, []string{"vendor", "mocks"}, true, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// should be ExcludeFilter wrapping DirectoryReader
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithInclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, []string{"src"}, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithInclude(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, nil, []string{"src"}, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_AllFilesWithInclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, []string{"src"}, nil, true, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// should be IncludeFilter wrapping DirectoryReader
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestIncludeExcludeComposition(t *testing.T) {
	// functional composition test: IncludeFilter + ExcludeFilter working together
	inner := &mockDiffRenderer{
		files: []diff.FileEntry{
			{Path: "src/app.go"}, {Path: "src/vendor/lib.go"}, {Path: "src/main.go"},
			{Path: "pkg/util.go"}, {Path: "vendor/dep.go"},
		},
	}

	// include narrows to src/, then exclude removes src/vendor/
	incl := diff.NewIncludeFilter(inner, []string{"src"})
	excl := diff.NewExcludeFilter(incl, []string{"src/vendor"})

	files, err := excl.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "src/app.go"}, {Path: "src/main.go"}}, files)
}

func TestDetectVCS_Git(t *testing.T) {
	// this test runs from inside the revdiff repo (which is a git repo)
	vcsType, root := diff.DetectVCS(".")
	assert.Equal(t, diff.VCSGit, vcsType)
	assert.DirExists(t, root)
	assert.NotEmpty(t, root)
}

func TestDetectVCS_None(t *testing.T) {
	t.Chdir(t.TempDir())
	vcsType, root := diff.DetectVCS(".")
	assert.Equal(t, diff.VCSNone, vcsType)
	assert.Empty(t, root)
}
