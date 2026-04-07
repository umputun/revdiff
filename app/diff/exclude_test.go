package diff

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRenderer is a minimal test double for the renderer interface.
type mockRenderer struct {
	changedFiles []FileEntry
	changedErr   error
	fileDiff     []DiffLine
	fileDiffErr  error
}

func (m *mockRenderer) ChangedFiles(string, bool) ([]FileEntry, error) {
	return m.changedFiles, m.changedErr
}

func (m *mockRenderer) FileDiff(string, string, bool) ([]DiffLine, error) {
	return m.fileDiff, m.fileDiffErr
}

func TestExcludeFilter_matchesExclude(t *testing.T) {
	ef := NewExcludeFilter(&mockRenderer{}, []string{"vendor", "ui/mocks"})

	tests := []struct {
		file string
		want bool
	}{
		{"vendor/foo.go", true},
		{"vendor/pkg/bar.go", true},
		{"vendor", true},
		{"ui/mocks/mock.go", true},
		{"ui/mocks", true},
		{"ui/model.go", false},
		{"vendorutil/foo.go", false},
		{"src/vendor/foo.go", false},
		{"diff/diff.go", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Equal(t, tt.want, ef.matchesExclude(tt.file))
		})
	}
}

func TestExcludeFilter_matchesExclude_trailingSlash(t *testing.T) {
	// prefixes with trailing slashes should be normalized
	ef := NewExcludeFilter(&mockRenderer{}, []string{"vendor/", "mocks/"})
	assert.True(t, ef.matchesExclude("vendor/foo.go"))
	assert.True(t, ef.matchesExclude("mocks/mock.go"))
	assert.False(t, ef.matchesExclude("src/vendor/foo.go"))
}

func TestExcludeFilter_matchesExclude_whitespaceAndEmpty(t *testing.T) {
	// prefixes with whitespace (e.g., from env var "vendor, mocks") should be trimmed
	ef := NewExcludeFilter(&mockRenderer{}, []string{" vendor ", " mocks", ""})
	assert.True(t, ef.matchesExclude("vendor/foo.go"))
	assert.True(t, ef.matchesExclude("mocks/mock.go"))
	assert.Len(t, ef.prefixes, 2, "empty prefix should be skipped")
}

func TestExcludeFilter_ChangedFiles(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{
			{Path: "cmd/main.go"}, {Path: "vendor/lib.go"}, {Path: "diff/diff.go"},
			{Path: "vendor/pkg/x.go"}, {Path: "ui/mocks/m.go"},
		},
	}
	ef := NewExcludeFilter(inner, []string{"vendor", "ui/mocks"})

	files, err := ef.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "cmd/main.go"}, {Path: "diff/diff.go"}}, files)
}

func TestExcludeFilter_ChangedFiles_noExcludes(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{{Path: "a.go"}, {Path: "b.go"}},
	}
	ef := NewExcludeFilter(inner, nil)

	files, err := ef.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "a.go"}, {Path: "b.go"}}, files)
}

func TestExcludeFilter_ChangedFiles_allExcluded(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{{Path: "vendor/a.go"}, {Path: "vendor/b.go"}},
	}
	ef := NewExcludeFilter(inner, []string{"vendor"})

	files, err := ef.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestExcludeFilter_ChangedFiles_innerError(t *testing.T) {
	inner := &mockRenderer{changedErr: errors.New("git failed")}
	ef := NewExcludeFilter(inner, []string{"vendor"})

	_, err := ef.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git failed")
}

func TestExcludeFilter_FileDiff_passthrough(t *testing.T) {
	lines := []DiffLine{
		{OldNum: 1, NewNum: 1, Content: "line1", ChangeType: ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "line2", ChangeType: ChangeContext},
	}
	inner := &mockRenderer{fileDiff: lines}
	ef := NewExcludeFilter(inner, []string{"vendor"})

	// even a file matching exclude prefix is passed through — filtering is only at file list level
	result, err := ef.FileDiff("", "vendor/foo.go", false)
	require.NoError(t, err)
	assert.Equal(t, lines, result)
}

func TestExcludeFilter_FileDiff_innerError(t *testing.T) {
	inner := &mockRenderer{fileDiffErr: errors.New("read failed")}
	ef := NewExcludeFilter(inner, []string{"vendor"})

	_, err := ef.FileDiff("", "foo.go", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read failed")
}
