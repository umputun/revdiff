package diff_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/diff/mocks"
)

func TestExcludeFilter_ChangedFiles(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{
				{Path: "cmd/main.go"}, {Path: "vendor/lib.go"}, {Path: "diff/diff.go"},
				{Path: "vendor/pkg/x.go"}, {Path: "ui/mocks/m.go"},
			}, nil
		},
	}
	ef := diff.NewExcludeFilter(inner, []string{"vendor", "ui/mocks"})

	files, err := ef.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "cmd/main.go"}, {Path: "diff/diff.go"}}, files)
}

func TestExcludeFilter_ChangedFiles_noExcludes(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}, nil
		},
	}
	ef := diff.NewExcludeFilter(inner, nil)

	files, err := ef.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}, files)
}

func TestExcludeFilter_ChangedFiles_allExcluded(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			return []diff.FileEntry{{Path: "vendor/a.go"}, {Path: "vendor/b.go"}}, nil
		},
	}
	ef := diff.NewExcludeFilter(inner, []string{"vendor"})

	files, err := ef.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestExcludeFilter_ChangedFiles_innerError(t *testing.T) {
	inner := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, errors.New("git failed") },
	}
	ef := diff.NewExcludeFilter(inner, []string{"vendor"})

	_, err := ef.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git failed")
}

func TestExcludeFilter_FileDiff_passthrough(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	inner := &mocks.RendererMock{
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) { return lines, nil },
	}
	ef := diff.NewExcludeFilter(inner, []string{"vendor"})

	// even a file matching exclude prefix is passed through — filtering is only at file list level
	result, err := ef.FileDiff("", "vendor/foo.go", false, 0)
	require.NoError(t, err)
	assert.Equal(t, lines, result)
}

func TestExcludeFilter_FileDiff_innerError(t *testing.T) {
	inner := &mocks.RendererMock{
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, errors.New("read failed") },
	}
	ef := diff.NewExcludeFilter(inner, []string{"vendor"})

	_, err := ef.FileDiff("", "foo.go", false, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read failed")
}

func TestExcludeFilter_FileDiff_passesContextLinesThrough(t *testing.T) {
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
			ef := diff.NewExcludeFilter(inner, []string{"vendor"})
			_, err := ef.FileDiff("", "foo.go", false, tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.context, gotContext)
		})
	}
}
