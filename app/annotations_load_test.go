package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/mocks"
)

func writeTempAnnotations(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notes.md")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestParseArgs_AnnotationsFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--annotations", "/tmp/notes.md"))
	require.NoError(t, err)
	assert.Equal(t, "/tmp/notes.md", opts.Annotations)
}

func TestParseArgs_AnnotationsDefaultEmpty(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Empty(t, opts.Annotations)
}

func TestPreloadAnnotations_FileNotFound(t *testing.T) {
	store := annotation.NewStore()
	r := &mocks.RendererMock{}
	err := preloadAnnotations(filepath.Join(t.TempDir(), "missing.md"), store, r, "", false, nil, "", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open annotations file")
	assert.Equal(t, 0, store.Count())
}

func TestPreloadAnnotations_ParseError(t *testing.T) {
	path := writeTempAnnotations(t, "## bad header without parens\nbody\n")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
	}
	err := preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse annotations")
}

func TestPreloadAnnotations_ChangedFilesError(t *testing.T) {
	path := writeTempAnnotations(t, "## a.go (file-level)\nhi\n")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, errors.New("boom") },
	}
	err := preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve diff")
}

func TestPreloadAnnotations_DropsOrphans(t *testing.T) {
	body := "## a.go (file-level)\nfile-level note\n\n" +
		"## a.go:5 (+)\nline-add note\n\n" +
		"## a.go:99 (+)\notherwise valid file but bad line\n\n" +
		"## ghost.go (file-level)\nghost\n\n" +
		"## ghost.go:1 (+)\nghost line\n"
	path := writeTempAnnotations(t, body)

	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}, nil
		},
		FileDiffFunc: func(_, file string, _ bool, _ int) ([]diff.DiffLine, error) {
			assert.Equal(t, "a.go", file)
			return []diff.DiffLine{
				{OldNum: 0, NewNum: 5, Content: "added", ChangeType: diff.ChangeAdd},
				{OldNum: 4, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
				{OldNum: 6, NewNum: 6, Content: "ctx", ChangeType: diff.ChangeContext},
			}, nil
		},
	}
	warn := &bytes.Buffer{}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", warn))

	assert.Equal(t, 2, store.Count())
	all := store.All()
	require.Len(t, all["a.go"], 2)
	assert.Equal(t, 0, all["a.go"][0].Line) // file-level sorted first
	assert.Equal(t, 5, all["a.go"][1].Line)

	out := warn.String()
	assert.Contains(t, out, "a.go:99")
	assert.Contains(t, out, "ghost.go")
}

func TestPreloadAnnotations_RemovedLineMatchesOldNum(t *testing.T) {
	path := writeTempAnnotations(t, "## a.go:4 (-)\nremoved-line note\n")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}, nil
		},
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			return []diff.DiffLine{
				{OldNum: 4, NewNum: 0, Content: "gone", ChangeType: diff.ChangeRemove},
			}, nil
		},
	}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{}))
	assert.Equal(t, 1, store.Count())
	assert.True(t, store.Has("a.go", 4, "-"))
}

func TestPreloadAnnotations_ContextMatchesNewNum(t *testing.T) {
	path := writeTempAnnotations(t, "## a.go:7 ( )\ncontext note\n")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}, nil
		},
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			return []diff.DiffLine{
				{OldNum: 7, NewNum: 7, Content: "ctx", ChangeType: diff.ChangeContext},
			}, nil
		},
	}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{}))
	assert.True(t, store.Has("a.go", 7, " "))
}

func TestPreloadAnnotations_DuplicateLastWriteWins(t *testing.T) {
	path := writeTempAnnotations(t, "## a.go:5 (+)\nfirst\n\n## a.go:5 (+)\nsecond\n")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}, nil
		},
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			return []diff.DiffLine{{NewNum: 5, ChangeType: diff.ChangeAdd}}, nil
		},
	}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{}))
	assert.Equal(t, 1, store.Count())
	got := store.Get("a.go")
	require.Len(t, got, 1)
	assert.Equal(t, "second", got[0].Comment)
}

func TestPreloadAnnotations_StagedOnlyAddedFile(t *testing.T) {
	// Mirrors ui.loadFiles + ui.resolveEmptyDiff: when ref=="" and !staged
	// and the unstaged set is empty, staged-only FileAdded entries surface in
	// the UI. Annotations saved from that startup path must round-trip.
	body := "## new.go (file-level)\nnote\n\n## new.go:2 (+)\nadd note\n"
	path := writeTempAnnotations(t, body)
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(_ string, staged bool) ([]diff.FileEntry, error) {
			if staged {
				return []diff.FileEntry{{Path: "new.go", Status: diff.FileAdded}}, nil
			}
			return nil, nil
		},
		FileDiffFunc: func(_, file string, staged bool, _ int) ([]diff.DiffLine, error) {
			assert.Equal(t, "new.go", file)
			assert.True(t, staged, "FileAdded with empty unstaged set must retry with --cached")
			return []diff.DiffLine{
				{NewNum: 1, Content: "first", ChangeType: diff.ChangeAdd},
				{NewNum: 2, Content: "second", ChangeType: diff.ChangeAdd},
			}, nil
		},
	}
	warn := &bytes.Buffer{}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", warn))
	assert.Equal(t, 2, store.Count(), "warnings: %s", warn.String())
	assert.True(t, store.Has("new.go", 2, "+"))
}

func TestPreloadAnnotations_UntrackedFile(t *testing.T) {
	// Untracked files surface in the UI via the show-untracked toggle, not
	// renderer.ChangedFiles. Annotations saved against them must round-trip:
	// the file is folded into the known set via untrackedFn and its line set
	// is read from disk as all-added lines.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "new.go")
	require.NoError(t, os.WriteFile(filePath, []byte("first\nsecond\nthird\n"), 0o600))

	body := "## new.go (file-level)\nfile note\n\n" +
		"## new.go:2 (+)\nline note\n\n" +
		"## new.go:99 (+)\nout-of-range\n"
	path := writeTempAnnotations(t, body)

	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
	}
	untracked := func() ([]string, error) { return []string{"new.go"}, nil }

	warn := &bytes.Buffer{}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, untracked, dir, warn))

	assert.Equal(t, 2, store.Count(), "warnings: %s", warn.String())
	assert.True(t, store.Has("new.go", 0, ""), "file-level annotation must round-trip")
	assert.True(t, store.Has("new.go", 2, "+"), "line-2 annotation must match disk content")
	assert.Contains(t, warn.String(), "new.go:99")
}

func TestPreloadAnnotations_UntrackedListError(t *testing.T) {
	path := writeTempAnnotations(t, "## a.go (file-level)\nnote\n")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}, nil
		},
	}
	untracked := func() ([]string, error) { return nil, errors.New("git boom") }

	warn := &bytes.Buffer{}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, untracked, "", warn))
	assert.Equal(t, 1, store.Count(), "tracked annotations survive when untracked listing fails")
	assert.Contains(t, warn.String(), "list untracked files")
}

func TestPreloadAnnotations_RejectsNonRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFO not portable")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	store := annotation.NewStore()
	r := &mocks.RendererMock{}
	err := preloadAnnotations(fifo, store, r, "", false, nil, "", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestPreloadAnnotations_RejectsOversizeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.md")
	// header + body that scrolls past the 1 MiB cap
	body := "## a.go:1 (+)\n" + strings.Repeat("x", maxAnnotationsFileSize+10) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	store := annotation.NewStore()
	r := &mocks.RendererMock{}
	err := preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestPreloadAnnotations_SanitizesCommentText(t *testing.T) {
	// Stray ANSI / CR-overwrite bytes in the source file must be stripped
	// before reaching Store.Add — the TUI renderer wraps comment text in
	// ANSI italic without a second pass.
	body := "## a.go:5 (+)\nrogue \x1b[31mred\x1b[0m and \rcr-overwrite\n"
	path := writeTempAnnotations(t, body)
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}, nil
		},
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			return []diff.DiffLine{{NewNum: 5, ChangeType: diff.ChangeAdd}}, nil
		},
	}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{}))
	got := store.Get("a.go")
	require.Len(t, got, 1)
	assert.NotContains(t, got[0].Comment, "\x1b")
	assert.NotContains(t, got[0].Comment, "\r")
	assert.Contains(t, got[0].Comment, "rogue red")
	assert.Contains(t, got[0].Comment, "cr-overwrite")
}

func TestValidateStdinFlags_RejectsAnnotations(t *testing.T) {
	opts := options{Stdin: true, Annotations: "/tmp/n.md"}
	err := validateStdinFlags(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--annotations")
}

func TestPreloadAnnotations_EmptyFile(t *testing.T) {
	path := writeTempAnnotations(t, "")
	store := annotation.NewStore()
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
	}
	require.NoError(t, preloadAnnotations(path, store, r, "", false, nil, "", &bytes.Buffer{}))
	assert.Equal(t, 0, store.Count())
}
