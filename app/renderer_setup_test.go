package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui"
	"github.com/umputun/revdiff/app/ui/mocks"
)

func TestMakeGitRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{Only: []string{"file.md"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithoutOnly(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{}, dir)
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
	assert.Contains(t, err.Error(), "no git, mercurial, or jujutsu repository found")
}

func TestMakeGitRenderer_AllFiles(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{AllFiles: true}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.DirectoryReader{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_AllFilesUnsupported(t *testing.T) {
	_, _, err := makeHgRenderer(diff.NewHg(""), options{AllFiles: true}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files is not supported in mercurial")
}

func TestMakeHgRenderer_Default(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, options{}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.Hg{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, options{Only: []string{"file.go"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, options{Exclude: []string{"vendor"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{Exclude: []string{"vendor"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_AllFilesWithExclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{AllFiles: true, Exclude: []string{"vendor", "mocks"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// should be ExcludeFilter wrapping DirectoryReader
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithInclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{Include: []string{"src"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithInclude(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, options{Include: []string{"src"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeJjRenderer_Default(t *testing.T) {
	dir := t.TempDir()
	j := diff.NewJj(dir)
	renderer, workDir, err := makeJjRenderer(j, options{}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.Jj{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeJjRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	j := diff.NewJj(dir)
	renderer, workDir, err := makeJjRenderer(j, options{Only: []string{"file.go"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeJjRenderer_AllFiles(t *testing.T) {
	dir := t.TempDir()
	j := diff.NewJj(dir)
	renderer, workDir, err := makeJjRenderer(j, options{AllFiles: true}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.DirectoryReader{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeJjRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	j := diff.NewJj(dir)
	renderer, workDir, err := makeJjRenderer(j, options{Exclude: []string{"vendor"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeJjRenderer_WithInclude(t *testing.T) {
	dir := t.TempDir()
	j := diff.NewJj(dir)
	renderer, workDir, err := makeJjRenderer(j, options{Include: []string{"src"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_AllFilesWithInclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, options{AllFiles: true, Include: []string{"src"}}, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// should be IncludeFilter wrapping DirectoryReader
	assert.IsType(t, &diff.IncludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestIncludeExcludeComposition(t *testing.T) {
	// functional composition test: IncludeFilter + ExcludeFilter working together
	files := []diff.FileEntry{
		{Path: "src/app.go"}, {Path: "src/vendor/lib.go"}, {Path: "src/main.go"},
		{Path: "pkg/util.go"}, {Path: "vendor/dep.go"},
	}
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return files, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
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

func TestCommitsApplicable(t *testing.T) {
	refOpts := options{}
	refOpts.Refs.Base = "HEAD~1"
	g := diff.NewGit(t.TempDir())

	tests := []struct {
		name string
		opts options
		cl   diff.CommitLogger
		want bool
	}{
		{name: "nil commit logger", opts: refOpts, cl: nil, want: false},
		{name: "stdin mode", opts: options{Stdin: true}, cl: g, want: false},
		{name: "staged mode", opts: options{Staged: true}, cl: g, want: false},
		{name: "all-files mode", opts: options{AllFiles: true}, cl: g, want: false},
		{name: "empty ref", opts: options{}, cl: g, want: false},
		{name: "ref + logger applicable", opts: refOpts, cl: g, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, commitsApplicable(tc.opts, tc.cl))
		})
	}
}

func TestCompactApplicable(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	h := diff.NewHg(dir)
	j := diff.NewJj(dir)
	fr := diff.NewFileReader([]string{"file.md"}, dir)
	fallback := diff.NewFallbackRenderer(g, []string{"file.md"}, dir)
	incl := diff.NewIncludeFilter(g, []string{"src"})
	excl := diff.NewExcludeFilter(g, []string{"vendor"})
	cr := diff.NewCompareReader(dir+"/old.md", dir+"/new.md")

	tests := []struct {
		name     string
		opts     options
		renderer ui.Renderer
		want     bool
	}{
		{name: "plain git ref", opts: options{}, renderer: g, want: true},
		{name: "plain hg", opts: options{}, renderer: h, want: true},
		{name: "plain jj", opts: options{}, renderer: j, want: true},
		{name: "stdin disqualifies", opts: options{Stdin: true}, renderer: g, want: false},
		{name: "all-files disqualifies", opts: options{AllFiles: true}, renderer: g, want: false},
		{name: "only without VCS (FileReader)", opts: options{Only: []string{"file.md"}}, renderer: fr, want: false},
		{name: "only in VCS repo (Fallback wrapping Git)", opts: options{Only: []string{"file.md"}}, renderer: fallback, want: true},
		{name: "include wrapping git", opts: options{Include: []string{"src"}}, renderer: incl, want: true},
		{name: "exclude wrapping git", opts: options{Exclude: []string{"vendor"}}, renderer: excl, want: true},
		// compare-mode renderer (not *FileReader) qualifies; pins that the
		// FileReader bypass does not over-fire on CompareReader.
		{name: "compare reader (not FileReader)", opts: options{CompareOld: dir + "/old.md", CompareNew: dir + "/new.md"}, renderer: cr, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, compactApplicable(tc.opts, tc.renderer))
		})
	}
}

func TestGitHgJj_ImplementCommitLogger(t *testing.T) {
	assert.Implements(t, (*diff.CommitLogger)(nil), diff.NewGit(t.TempDir()))
	assert.Implements(t, (*diff.CommitLogger)(nil), diff.NewHg(t.TempDir()))
	assert.Implements(t, (*diff.CommitLogger)(nil), diff.NewJj(t.TempDir()))
}

func TestReloadApplicable(t *testing.T) {
	tests := []struct {
		name string
		opts options
		want bool
	}{
		{name: "stdin mode", opts: options{Stdin: true}, want: false},
		{name: "normal mode", opts: options{}, want: true},
		{name: "staged mode", opts: options{Staged: true}, want: true},
		{name: "all-files mode", opts: options{AllFiles: true}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reloadApplicable(tc.opts))
		})
	}
}
