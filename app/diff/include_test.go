package diff_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/diff/mocks"
)

func TestIncludeFilter_ChangedFiles(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{
				{Path: "cmd/main.go"}, {Path: "src/app.go"}, {Path: "src/lib/util.go"},
				{Path: "vendor/lib.go"}, {Path: "ui/mocks/m.go"},
			}, nil
		},
	}
	f := diff.NewIncludeFilter(inner, []string{"src"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "src/app.go"}, {Path: "src/lib/util.go"}}, files)
}

func TestIncludeFilter_ChangedFiles_multiplePrefixes(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{
				{Path: "cmd/main.go"}, {Path: "src/app.go"}, {Path: "pkg/util.go"},
				{Path: "vendor/lib.go"},
			}, nil
		},
	}
	f := diff.NewIncludeFilter(inner, []string{"src", "pkg"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "src/app.go"}, {Path: "pkg/util.go"}}, files)
}

func TestIncludeFilter_ChangedFiles_noneMatch(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "vendor/a.go"}, {Path: "vendor/b.go"}}, nil
		},
	}
	f := diff.NewIncludeFilter(inner, []string{"src"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestIncludeFilter_ChangedFiles_exactMatch(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "Makefile"}, {Path: "src/app.go"}}, nil
		},
	}
	f := diff.NewIncludeFilter(inner, []string{"Makefile"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "Makefile"}}, files)
}

func TestIncludeFilter_ChangedFiles_innerError(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, errors.New("git failed") },
	}
	f := diff.NewIncludeFilter(inner, []string{"src"})

	_, err := f.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include filter, changed files")
	assert.Contains(t, err.Error(), "git failed")
}

func TestIncludeFilter_ChangedFiles_prefixNormalization(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "src/app.go"}, {Path: "vendor/lib.go"}}, nil
		},
	}
	f := diff.NewIncludeFilter(inner, []string{" src/ ", "", "  "})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "src/app.go"}}, files)
}

func TestIncludeFilter_ChangedFiles_allPrefixesEmpty(t *testing.T) {
	expected := []diff.FileEntry{{Path: "src/app.go"}, {Path: "vendor/lib.go"}}
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return expected, nil },
	}
	f := diff.NewIncludeFilter(inner, []string{"", " ", "  "})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, expected, files, "all prefixes normalized to empty should be a no-op")
}

func TestIncludeFilter_FileDiff_passthrough(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	inner := &mocks.RendererMock{
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) { return lines, nil },
	}
	f := diff.NewIncludeFilter(inner, []string{"src"})

	// even a file NOT matching include prefix is passed through — filtering is only at file list level
	result, err := f.FileDiff("", "vendor/foo.go", false, 0)
	require.NoError(t, err)
	assert.Equal(t, lines, result)
}

func TestIncludeFilter_FileDiff_innerError(t *testing.T) {
	inner := &mocks.RendererMock{
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, errors.New("read failed") },
	}
	f := diff.NewIncludeFilter(inner, []string{"src"})

	_, err := f.FileDiff("", "foo.go", false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include filter, file diff foo.go")
	assert.Contains(t, err.Error(), "read failed")
}

func TestIncludeFilter_FileDiff_passesContextLinesThrough(t *testing.T) {
	tests := []struct {
		name    string
		context int
	}{
		{name: "full context (zero)", context: 0},
		{name: "small context", context: 5},
		{name: "full-file sentinel", context: 1000000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotContext int
			inner := &mocks.RendererMock{
				FileDiffFunc: func(_, _ string, _ bool, c int) ([]diff.DiffLine, error) {
					gotContext = c
					return nil, nil
				},
			}
			f := diff.NewIncludeFilter(inner, []string{"src"})
			_, err := f.FileDiff("", "foo.go", false, tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.context, gotContext)
		})
	}
}
