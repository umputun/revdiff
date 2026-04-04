package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFileAsContext_NormalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("line one\nline two\nline three\n"), 0o600))

	lines, err := readFileAsContext(path)
	require.NoError(t, err)
	require.Len(t, lines, 3)

	assert.Equal(t, DiffLine{OldNum: 1, NewNum: 1, Content: "line one", ChangeType: ChangeContext}, lines[0])
	assert.Equal(t, DiffLine{OldNum: 2, NewNum: 2, Content: "line two", ChangeType: ChangeContext}, lines[1])
	assert.Equal(t, DiffLine{OldNum: 3, NewNum: 3, Content: "line three", ChangeType: ChangeContext}, lines[2])
}

func TestReadFileAsContext_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	lines, err := readFileAsContext(path)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestReadFileAsContext_NonexistentFile(t *testing.T) {
	_, err := readFileAsContext("/nonexistent/file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read file")
}

func TestReadFileAsContext_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noterminal.txt")
	require.NoError(t, os.WriteFile(path, []byte("alpha\nbeta"), 0o600))

	lines, err := readFileAsContext(path)
	require.NoError(t, err)
	require.Len(t, lines, 2)

	assert.Equal(t, "alpha", lines[0].Content)
	assert.Equal(t, "beta", lines[1].Content)
	assert.Equal(t, 1, lines[0].OldNum)
	assert.Equal(t, 2, lines[1].NewNum)
}

func TestReadFileAsContext_AllContextType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o600))

	lines, err := readFileAsContext(path)
	require.NoError(t, err)

	for _, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType, "all lines must be context type")
		assert.Positive(t, l.OldNum, "OldNum must be positive")
		assert.Positive(t, l.NewNum, "NewNum must be positive")
		assert.Equal(t, l.OldNum, l.NewNum, "OldNum and NewNum must be equal")
	}
}

func TestFallbackRenderer_ChangedFiles_FileInDiff(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create and commit a file, then modify it so it appears in diff
	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "hello.go", "package main\n\nvar x = 1\n")

	fr := NewFallbackRenderer(g, []string{"hello.go"}, dir)
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	// hello.go is already in the diff, should not be duplicated
	assert.Equal(t, []string{"hello.go"}, files)
}

func TestFallbackRenderer_ChangedFiles_FileNotInDiffButOnDisk(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// commit a file so repo is clean
	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// create another file on disk that is not in the diff (committed, unchanged)
	writeFile(t, dir, "readme.md", "# readme\n")
	gitCmd(t, dir, "add", "readme.md")
	gitCmd(t, dir, "commit", "-m", "add readme")

	fr := NewFallbackRenderer(g, []string{"readme.md"}, dir)
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	// no files in diff, but readme.md exists on disk and is in --only
	assert.Equal(t, []string{"readme.md"}, files)
}

func TestFallbackRenderer_ChangedFiles_FileNotOnDisk(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	fr := NewFallbackRenderer(g, []string{"nonexistent.go"}, dir)
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	// nonexistent file should not appear
	assert.Empty(t, files)
}

func TestFallbackRenderer_ChangedFiles_SuffixMatchDedup(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create a file in a subdirectory, modify it so it's in the diff
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "plans"), 0o750))
	writeFileAt(t, filepath.Join(dir, "docs", "plans", "plan.md"), "# plan\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFileAt(t, filepath.Join(dir, "docs", "plans", "plan.md"), "# plan\n\nupdated\n")

	// --only "plan.md" should suffix-match "docs/plans/plan.md" from the diff
	fr := NewFallbackRenderer(g, []string{"plan.md"}, dir)
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	// should not duplicate - plan.md suffix-matches docs/plans/plan.md
	assert.Equal(t, []string{"docs/plans/plan.md"}, files)
}

func TestFallbackRenderer_ChangedFiles_AbsolutePath(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// create a file with absolute path outside the repo
	tmpFile := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte("# outside\n"), 0o600))

	fr := NewFallbackRenderer(g, []string{tmpFile}, dir)
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	// absolute path outside workDir should be preserved as-is so filterOnly can match it
	assert.Equal(t, tmpFile, files[0])
}

func TestFallbackRenderer_FileDiff_FileHasDiff(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "main.go")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n")

	fr := NewFallbackRenderer(g, []string{"main.go"}, dir)
	lines, err := fr.FileDiff("", "main.go", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	// should contain actual diff lines (additions), not just context
	var hasAdd bool
	for _, l := range lines {
		if l.ChangeType == ChangeAdd {
			hasAdd = true
			break
		}
	}
	assert.True(t, hasAdd, "should use inner renderer's diff with actual changes")
}

func TestFallbackRenderer_FileDiff_NoDiffFallbackToDisk(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "readme.md", "# readme\nline two\nline three\n")
	gitCmd(t, dir, "add", "readme.md")
	gitCmd(t, dir, "commit", "-m", "initial")

	// readme.md is committed and unchanged, so inner FileDiff returns empty
	fr := NewFallbackRenderer(g, []string{"readme.md"}, dir)
	lines, err := fr.FileDiff("", "readme.md", false)
	require.NoError(t, err)
	require.Len(t, lines, 3)

	// all lines should be context type (fallback to disk read)
	for _, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType)
	}
	assert.Equal(t, "# readme", lines[0].Content)
	assert.Equal(t, "line two", lines[1].Content)
	assert.Equal(t, "line three", lines[2].Content)
}

func TestFallbackRenderer_FileDiff_FileNotInOnly(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// file is not in the --only list, so no fallback
	fr := NewFallbackRenderer(g, []string{"other.go"}, dir)
	lines, err := fr.FileDiff("", "hello.go", false)
	require.NoError(t, err)
	assert.Empty(t, lines) // no diff and not in --only, so empty
}

func TestFallbackRenderer_FileDiff_FileNotOnDisk(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// --only points to a file that doesn't exist on disk
	fr := NewFallbackRenderer(g, []string{"missing.go"}, dir)
	lines, err := fr.FileDiff("", "missing.go", false)
	require.NoError(t, err)
	assert.Empty(t, lines) // file doesn't exist, return empty
}

func TestFallbackRenderer_FileDiff_OutsideRepoAbsPath(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// create a file outside the repo with absolute path
	tmpFile := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(tmpFile, []byte("# outside\nline two\n"), 0o600))

	fr := NewFallbackRenderer(g, []string{tmpFile}, dir)
	// should read from disk without calling inner git (which would fail with "outside repository")
	lines, err := fr.FileDiff("", tmpFile, false)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, ChangeContext, lines[0].ChangeType)
	assert.Equal(t, "# outside", lines[0].Content)
	assert.Equal(t, "line two", lines[1].Content)
}

func TestFallbackRenderer_FileDiff_OutsideRepoNonexistent(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// absolute path outside the repo that doesn't exist on disk
	missingFile := filepath.Join(t.TempDir(), "no-such-file.md")
	fr := NewFallbackRenderer(g, []string{missingFile}, dir)
	_, err := fr.FileDiff("", missingFile, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestFallbackRenderer_ChangedFiles_AbsoluteInsideRepo(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create and commit a file, leave it unchanged
	writeFile(t, dir, "readme.md", "# readme\n")
	gitCmd(t, dir, "add", "readme.md")
	gitCmd(t, dir, "commit", "-m", "initial")

	// use absolute path pointing inside the repo
	absPath := filepath.Join(dir, "readme.md")
	fr := NewFallbackRenderer(g, []string{absPath}, dir)
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	// should use original absolute pattern so filterOnly can match against it
	assert.Equal(t, absPath, files[0])
}

func TestFallbackRenderer_FileDiff_InnerErrorWithOnlyFile(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "hello.go", "package main\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// create a file that exists on disk but request diff with an invalid ref
	// to trigger an inner error; the error should propagate, not be masked by fallback
	writeFile(t, dir, "readme.md", "# readme\n")
	gitCmd(t, dir, "add", "readme.md")
	gitCmd(t, dir, "commit", "-m", "add readme")

	fr := NewFallbackRenderer(g, []string{"readme.md"}, dir)
	// use a ref that will cause inner.FileDiff to return an error
	_, err := fr.FileDiff("nonexistent-ref-abc123", "readme.md", false)
	require.Error(t, err, "git errors should propagate, not be swallowed by fallback")
	assert.Contains(t, err.Error(), "get file diff")
}

func TestFallbackRenderer_MatchesAny(t *testing.T) {
	fr := &FallbackRenderer{} // workDir is empty, so resolvePath/relativePath are no-ops for these cases

	tests := []struct {
		name    string
		files   []string
		pattern string
		want    bool
	}{
		{name: "exact match", files: []string{"main.go", "util.go"}, pattern: "main.go", want: true},
		{name: "suffix match", files: []string{"docs/plans/plan.md"}, pattern: "plan.md", want: true},
		{name: "no match", files: []string{"main.go", "util.go"}, pattern: "other.go", want: false},
		{name: "empty files", files: nil, pattern: "any.go", want: false},
		{name: "partial name no match", files: []string{"main.go"}, pattern: "ain.go", want: false},
		{name: "exact path match", files: []string{"docs/readme.md"}, pattern: "docs/readme.md", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fr.matchesAny(tt.files, tt.pattern))
		})
	}
}

func TestFallbackRenderer_PathMatches(t *testing.T) {
	fr := &FallbackRenderer{workDir: "/repo"}

	tests := []struct {
		name    string
		file    string
		pattern string
		want    bool
	}{
		{name: "exact match", file: "main.go", pattern: "main.go", want: true},
		{name: "suffix match", file: "docs/plans/plan.md", pattern: "plan.md", want: true},
		{name: "no match", file: "main.go", pattern: "other.go", want: false},
		{name: "partial name no match", file: "main.go", pattern: "ain.go", want: false},
		{name: "exact path match", file: "docs/readme.md", pattern: "docs/readme.md", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fr.pathMatches(tt.file, tt.pattern))
		})
	}
}

func TestFileReader_ChangedFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exists.md"), []byte("# exists\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "also.txt"), []byte("hello\n"), 0o600))

	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{name: "all exist", files: []string{"exists.md", "also.txt"},
			want: []string{filepath.Join(dir, "exists.md"), filepath.Join(dir, "also.txt")}},
		{name: "one missing", files: []string{"exists.md", "nope.go"},
			want: []string{filepath.Join(dir, "exists.md")}},
		{name: "all missing", files: []string{"missing1.go", "missing2.go"}, want: nil},
		{name: "empty list", files: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewFileReader(tt.files, dir)
			got, err := r.ChangedFiles("", false)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFileReader_ChangedFiles_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absFile := filepath.Join(dir, "abs.md")
	require.NoError(t, os.WriteFile(absFile, []byte("# abs\n"), 0o600))

	r := NewFileReader([]string{absFile}, "/some/other/workdir")
	got, err := r.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, absFile, got[0])
}

func TestFileReader_FileDiff(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("line one\nline two\nline three\n"), 0o600))

	r := NewFileReader([]string{"readme.md"}, dir)
	lines, err := r.FileDiff("", filepath.Join(dir, "readme.md"), false)
	require.NoError(t, err)
	require.Len(t, lines, 3)

	for _, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType)
		assert.Equal(t, l.OldNum, l.NewNum)
	}
	assert.Equal(t, "line one", lines[0].Content)
	assert.Equal(t, "line two", lines[1].Content)
	assert.Equal(t, "line three", lines[2].Content)
}

func TestFileReader_FileDiff_RelativePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.txt"), []byte("alpha\nbeta\n"), 0o600))

	r := NewFileReader([]string{"doc.txt"}, dir)
	// pass relative path, should resolve against workDir
	lines, err := r.FileDiff("", "doc.txt", false)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "alpha", lines[0].Content)
	assert.Equal(t, "beta", lines[1].Content)
}

func TestFileReader_FileDiff_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	r := NewFileReader([]string{"missing.go"}, dir)
	_, err := r.FileDiff("", "missing.go", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read file")
}

// acceptance criteria verification tests

// verifies that --only with a file that has no git changes
// produces a context-only view (all lines are ChangeContext) inside a git repo.
func TestFallbackRenderer_ContextOnlyView(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create a Go file, commit it, and leave it unchanged
	content := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	writeFile(t, dir, "main.go", content)
	gitCmd(t, dir, "add", "main.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	fr := NewFallbackRenderer(g, []string{"main.go"}, dir)

	// ChangedFiles should include main.go even though it has no git changes
	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"main.go"}, files)

	// FileDiff should return context-only lines (fallback to disk read)
	lines, err := fr.FileDiff("", "main.go", false)
	require.NoError(t, err)
	require.Len(t, lines, 7, "should have 7 lines from the file")

	for i, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType, "line %d should be context", i+1)
		assert.Equal(t, i+1, l.OldNum, "OldNum should be 1-based line number")
		assert.Equal(t, i+1, l.NewNum, "NewNum should be 1-based line number")
	}
	assert.Equal(t, "package main", lines[0].Content)
	assert.Equal(t, `	fmt.Println("hello")`, lines[5].Content)
}

// verifies that regular git diff mode still works when FallbackRenderer wraps
// a Git renderer — files with actual changes show real diffs.
func TestFallbackRenderer_NormalDiffUnaffected(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "main.go")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n")

	// wrap with FallbackRenderer (no --only, acts as passthrough)
	fr := NewFallbackRenderer(g, nil, dir)

	files, err := fr.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"main.go"}, files)

	lines, err := fr.FileDiff("", "main.go", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines)

	// should have actual diff lines (adds and removes), not just context
	var hasAdd, hasCtx bool
	for _, l := range lines {
		if l.ChangeType == ChangeAdd {
			hasAdd = true
		}
		if l.ChangeType == ChangeContext {
			hasCtx = true
		}
	}
	assert.True(t, hasAdd, "normal diff should have addition lines")
	assert.True(t, hasCtx, "normal diff should have context lines")
}

// verifies FileReader works as a standalone renderer without any git repo —
// simulating the no-git use case.
func TestFileReader_FullPipeline(t *testing.T) {
	dir := t.TempDir()
	content := "# Plan\n\n## Step 1\n- do something\n- do another thing\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.md"), []byte(content), 0o600))

	r := NewFileReader([]string{"plan.md"}, dir)

	// ChangedFiles should return the resolved path
	files, err := r.ChangedFiles("", false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, filepath.Join(dir, "plan.md"), files[0])

	// FileDiff should return all lines as context
	lines, err := r.FileDiff("", files[0], false)
	require.NoError(t, err)
	require.Len(t, lines, 5)

	for _, l := range lines {
		assert.Equal(t, ChangeContext, l.ChangeType)
		assert.Equal(t, l.OldNum, l.NewNum)
	}
	assert.Equal(t, "# Plan", lines[0].Content)
	assert.Equal(t, "- do another thing", lines[4].Content)
}

// writeFileAt writes content to an absolute path, creating directories as needed.
func writeFileAt(t *testing.T, absPath, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o750))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o600))
}
