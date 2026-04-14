package history

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSave_WithAnnotationsAndGit(t *testing.T) {
	// set up a temp git repo so gitCommitHash and gitDiff work
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot)

	histDir := t.TempDir()
	p := Params{
		Annotations:    "## app/model.go:42 (+)\nneeds refactoring\n",
		Path:           gitRoot,
		Ref:            "HEAD~1",
		GitRoot:        gitRoot,
		AnnotatedFiles: []string{"hello.txt"},
	}
	New(histDir).Save(p)

	// find the created file
	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)

	content := entries[0]
	assert.Contains(t, content, "# Review: ")
	assert.Contains(t, content, "path: "+gitRoot)
	assert.Contains(t, content, "refs: HEAD~1")
	assert.Contains(t, content, "commit: ")
	assert.Contains(t, content, "## Annotations")
	assert.Contains(t, content, "needs refactoring")
	assert.Contains(t, content, "## Diff")
	assert.Contains(t, content, "diff --git")
}

func TestSave_WithoutGit(t *testing.T) {
	histDir := t.TempDir()
	p := Params{
		Annotations: "## readme.md:10 (+)\nfix typo\n",
		Path:        "/some/project",
	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)

	content := entries[0]
	assert.Contains(t, content, "# Review: ")
	assert.Contains(t, content, "path: /some/project")
	assert.NotContains(t, content, "refs:")
	assert.NotContains(t, content, "commit:")
	assert.Contains(t, content, "## Annotations")
	assert.Contains(t, content, "fix typo")
	assert.NotContains(t, content, "## Diff")
}

func TestSave_StdinMode(t *testing.T) {
	histDir := t.TempDir()
	p := Params{
		Annotations: "## stdin:5 (+)\nnote\n",
		Path:        "stdin",
	}
	New(histDir).Save(p)

	// verify subdir is "stdin"
	entries, err := os.ReadDir(histDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "stdin", entries[0].Name())
}

func TestSave_EmptyAnnotations(t *testing.T) {
	histDir := t.TempDir()
	p := Params{Annotations: "", Path: "/some/project"}
	New(histDir).Save(p)

	// no files should be created
	entries, err := os.ReadDir(histDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSave_UnwritableDirectory(t *testing.T) {
	// use a path that cannot be created
	p := Params{
		Annotations: "## file.go:1 (+)\ncomment\n",
		Path:        "/some/project",
	}
	// should not panic
	New("/dev/null/impossible").Save(p)
}

func TestSave_StagedFlag(t *testing.T) {
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot)

	// stage a change
	err := os.WriteFile(filepath.Join(gitRoot, "hello.txt"), []byte("modified\n"), 0o600)
	require.NoError(t, err)
	runGit(t, gitRoot, "add", "hello.txt")

	histDir := t.TempDir()
	p := Params{
		Annotations:    "## hello.txt:1 (+)\nstaged note\n",
		Path:           gitRoot,
		Staged:         true,
		GitRoot:        gitRoot,
		AnnotatedFiles: []string{"hello.txt"},
	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0], "## Diff")
	assert.Contains(t, entries[0], "diff --git")
}

func TestHistoryDir_Default(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	svc := New("")
	p := Params{Path: "/Users/joe/myrepo"}
	got := svc.historyDir(p)
	assert.Equal(t, filepath.Join(home, ".config", "revdiff", "history", "myrepo"), got)
}

func TestHistoryDir_CustomDir(t *testing.T) {
	svc := New("/tmp/hist")
	p := Params{Path: "/Users/joe/myrepo"}
	got := svc.historyDir(p)
	assert.Equal(t, "/tmp/hist/myrepo", got)
}

func TestHistoryDir_StdinPath(t *testing.T) {
	svc := New("/tmp/hist")
	p := Params{Path: "stdin"}
	got := svc.historyDir(p)
	assert.Equal(t, "/tmp/hist/stdin", got)
}

func TestHistoryDir_SubDirOverride(t *testing.T) {
	// non-git single-file mode: Path is the full file path, SubDir overrides the directory name
	svc := New("/tmp/hist")
	p := Params{Path: "/tmp/note.md", SubDir: "tmp"}
	got := svc.historyDir(p)
	assert.Equal(t, "/tmp/hist/tmp", got)
}

func TestHistoryDir_SubDirTakesPrecedence(t *testing.T) {
	// SubDir should take precedence over deriving from Path
	svc := New("/tmp/hist")
	p := Params{Path: "/home/user/docs/readme.md", SubDir: "docs"}
	got := svc.historyDir(p)
	assert.Equal(t, "/tmp/hist/docs", got)
}

func TestHistoryDir_EmptyPath(t *testing.T) {
	svc := New("/tmp/hist")
	p := Params{Path: ""}
	got := svc.historyDir(p)
	assert.Equal(t, "/tmp/hist/unknown", got)
}

func TestSave_NonGitSingleFile(t *testing.T) {
	// non-git --only mode: Path is the full file path, SubDir overrides the directory name.
	// the path: header should show the full file path, not just the parent directory.
	histDir := t.TempDir()
	p := Params{
		Annotations: "## note.md:10 (+)\nfix typo\n",
		Path:        "/tmp/note.md",
		SubDir:      "tmp",
	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)

	content := entries[0]
	assert.Contains(t, content, "path: /tmp/note.md", "header should show full file path")

	// verify subdir is the parent directory name, not the filename
	subdirs, err := os.ReadDir(histDir)
	require.NoError(t, err)
	require.Len(t, subdirs, 1)
	assert.Equal(t, "tmp", subdirs[0].Name(), "subdir should be parent directory name")
}

func TestSave_DiffOnlyAnnotatedFiles(t *testing.T) {
	// set up a git repo with two files but only annotate one — diff should contain only the annotated file
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot) // creates hello.txt with two commits

	// add a second file with changes
	require.NoError(t, os.WriteFile(filepath.Join(gitRoot, "other.txt"), []byte("other\n"), 0o600))
	runGit(t, gitRoot, "add", "other.txt")
	runGit(t, gitRoot, "commit", "-m", "add other")

	// modify both files
	require.NoError(t, os.WriteFile(filepath.Join(gitRoot, "hello.txt"), []byte("hello changed\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(gitRoot, "other.txt"), []byte("other changed\n"), 0o600))

	histDir := t.TempDir()
	p := Params{
		Annotations:    "## hello.txt:1 (+)\nonly this file annotated\n",
		Path:           gitRoot,
		GitRoot:        gitRoot,
		AnnotatedFiles: []string{"hello.txt"}, // only hello.txt, not other.txt

	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)
	content := entries[0]
	assert.Contains(t, content, "## Diff")
	assert.Contains(t, content, "hello.txt")
	assert.NotContains(t, content, "other.txt", "diff should only contain annotated files")
}

func TestSave_FileLocationAndFormat(t *testing.T) {
	histDir := t.TempDir()
	p := Params{
		Annotations: "## file.go:1 (+)\nnote\n",
		Path:        "/Users/joe/myproject",
	}
	New(histDir).Save(p)

	// verify subdir is repo basename
	subdirs, err := os.ReadDir(histDir)
	require.NoError(t, err)
	require.Len(t, subdirs, 1)
	assert.Equal(t, "myproject", subdirs[0].Name())

	// verify file is .md with timestamp-like name
	files, err := os.ReadDir(filepath.Join(histDir, "myproject"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.True(t, strings.HasSuffix(files[0].Name(), ".md"))
	assert.Contains(t, files[0].Name(), "T") // timestamp format: 2006-01-02T15-04-05.md

	// verify header format precisely
	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)
	lines := strings.Split(entries[0], "\n")
	assert.True(t, strings.HasPrefix(lines[0], "# Review: "), "first line should be review header")
	assert.Equal(t, "path: /Users/joe/myproject", lines[1])
}

func TestSave_HeaderWithAllMetadata(t *testing.T) {
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot)

	histDir := t.TempDir()
	p := Params{
		Annotations:    "## hello.txt:1 (+)\nnote\n",
		Path:           gitRoot,
		Ref:            "master..feature",
		GitRoot:        gitRoot,
		AnnotatedFiles: []string{"hello.txt"},
	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)
	content := entries[0]

	// verify all header fields present and in order
	reviewIdx := strings.Index(content, "# Review: ")
	pathIdx := strings.Index(content, "path: ")
	refsIdx := strings.Index(content, "refs: master..feature")
	commitIdx := strings.Index(content, "commit: ")
	annotIdx := strings.Index(content, "## Annotations")

	assert.Greater(t, pathIdx, reviewIdx, "path should come after review header")
	assert.Greater(t, refsIdx, pathIdx, "refs should come after path")
	assert.Greater(t, commitIdx, refsIdx, "commit should come after refs")
	assert.Greater(t, annotIdx, commitIdx, "annotations should come after commit")
}

func TestSave_NoHistoryOnEmptyAnnotations(t *testing.T) {
	histDir := t.TempDir()
	// save with empty annotations, both calls pass empty string
	svc := New(histDir)
	svc.Save(Params{Annotations: "", Path: "/some/project"})
	svc.Save(Params{Annotations: "", Path: "/other/project"})

	entries, err := os.ReadDir(histDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no history files should be created for empty annotations")
}

func TestGitDiff_NoGitRoot(t *testing.T) {
	svc := New("")
	p := Params{AnnotatedFiles: []string{"file.go"}}
	assert.Empty(t, svc.gitDiff(p))
}

func TestGitDiff_NoFiles(t *testing.T) {
	svc := New("")
	p := Params{GitRoot: "/tmp"}
	assert.Empty(t, svc.gitDiff(p))
}

func TestGitCommitHash_NoGitRoot(t *testing.T) {
	svc := New("")
	assert.Empty(t, svc.gitCommitHash(""))
}

func TestGitCommitHash_InvalidDir(t *testing.T) {
	svc := New("")
	assert.Empty(t, svc.gitCommitHash("/nonexistent"))
}

func TestGitCommitHash_ValidRepo(t *testing.T) {
	svc := New("")
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot)
	hash := svc.gitCommitHash(gitRoot)
	assert.NotEmpty(t, hash)
	assert.Regexp(t, `^[0-9a-f]+$`, hash) // valid hex short hash
	assert.LessOrEqual(t, len(hash), 40)  // at most full SHA
}

func TestFilterRepoFiles(t *testing.T) {
	svc := New("")
	tests := []struct {
		name     string
		gitRoot  string
		files    []string
		expected []string
	}{
		{name: "relative files inside repo", gitRoot: "/repo", files: []string{"main.go", "pkg/util.go"}, expected: []string{"main.go", "pkg/util.go"}},
		{name: "absolute path outside repo", gitRoot: "/repo", files: []string{"main.go", "/tmp/note.md"}, expected: []string{"main.go"}},
		{name: "all outside repo", gitRoot: "/repo", files: []string{"/tmp/a.md", "/other/b.md"}, expected: []string{}},
		{name: "absolute path inside repo", gitRoot: "/repo", files: []string{"/repo/main.go"}, expected: []string{"main.go"}},
		{name: "absolute path nested", gitRoot: "/repo", files: []string{"/repo/pkg/util.go"}, expected: []string{"pkg/util.go"}},
		{name: "dotdot resolves inside repo", gitRoot: "/repo", files: []string{"sub/../main.go"}, expected: []string{"main.go"}},
		{name: "empty list", gitRoot: "/repo", files: []string{}, expected: []string{}},
		{name: "dotdot path outside", gitRoot: "/repo", files: []string{"../outside.go"}, expected: []string{}},
		{name: "dotdot-prefixed filename inside repo", gitRoot: "/repo", files: []string{"/repo/..foo"}, expected: []string{"..foo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.filterRepoFiles(tt.gitRoot, tt.files)
			if len(tt.expected) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestSave_ExternalFilesFilteredFromDiff(t *testing.T) {
	// set up a git repo, annotate both an in-repo file and an external file.
	// the external file should be filtered out, and the in-repo diff should still be captured.
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot)

	// modify the in-repo file so there's a diff to capture
	require.NoError(t, os.WriteFile(filepath.Join(gitRoot, "hello.txt"), []byte("modified\n"), 0o600))

	histDir := t.TempDir()
	p := Params{
		Annotations:    "## hello.txt:1 (+)\nin-repo note\n## /tmp/ext.md:1 (+)\nexternal note\n",
		Path:           gitRoot,
		GitRoot:        gitRoot,
		AnnotatedFiles: []string{"hello.txt", "/tmp/ext.md"}, // /tmp/ext.md is outside repo

	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)
	content := entries[0]
	assert.Contains(t, content, "## Diff", "diff section should be present for in-repo file")
	assert.Contains(t, content, "hello.txt", "diff should contain the in-repo file")
}

func TestSave_AllExternalFilesNoDiff(t *testing.T) {
	// when all annotated files are external, no diff section should appear
	gitRoot := t.TempDir()
	setupGitRepo(t, gitRoot)

	histDir := t.TempDir()
	p := Params{
		Annotations:    "## /tmp/ext.md:1 (+)\nnote\n",
		Path:           gitRoot,
		GitRoot:        gitRoot,
		AnnotatedFiles: []string{"/tmp/ext.md"},
	}
	New(histDir).Save(p)

	entries := readHistoryFiles(t, histDir)
	require.Len(t, entries, 1)
	assert.NotContains(t, entries[0], "## Diff", "no diff section when all files are external")
	assert.Contains(t, entries[0], "## Annotations", "annotations should still be present")
}

func TestSave_FilenameHasMilliseconds(t *testing.T) {
	histDir := t.TempDir()
	p := Params{Annotations: "## file.go:1 (+)\nnote\n", Path: "/myrepo"}
	New(histDir).Save(p)

	files, err := os.ReadDir(filepath.Join(histDir, "myrepo"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	name := files[0].Name()
	// filename format: 2006-01-02T15-04-05.000.md — the dot before milliseconds
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3}\.md$`, name)
}

// setupGitRepo creates a temp git repo with one commit containing hello.txt.
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o600)
	require.NoError(t, err)
	runGit(t, dir, "add", "hello.txt")
	runGit(t, dir, "commit", "-m", "initial")

	// add a second commit so HEAD~1 ref works
	err = os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0o600)
	require.NoError(t, err)
	runGit(t, dir, "add", "hello.txt")
	runGit(t, dir, "commit", "-m", "update")
}

// runGit runs a git command in the given directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := newGitCmd(dir, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
}

// newGitCmd creates an exec.Cmd for git in the given directory.
func newGitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...) //nolint:gosec // test helper with controlled inputs
	cmd.Dir = dir
	return cmd
}

// readHistoryFiles walks the history directory and returns file contents.
func readHistoryFiles(t *testing.T, histDir string) []string {
	t.Helper()
	var contents []string
	err := filepath.Walk(histDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			data, readErr := os.ReadFile(path) //nolint:gosec // test helper reading from temp dir
			require.NoError(t, readErr)
			contents = append(contents, string(data))
		}
		return nil
	})
	require.NoError(t, err)
	return contents
}
