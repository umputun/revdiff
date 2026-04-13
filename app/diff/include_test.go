package diff

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncludeFilter_ChangedFiles(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{
			{Path: "cmd/main.go"}, {Path: "src/app.go"}, {Path: "src/lib/util.go"},
			{Path: "vendor/lib.go"}, {Path: "ui/mocks/m.go"},
		},
	}
	f := NewIncludeFilter(inner, []string{"src"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "src/app.go"}, {Path: "src/lib/util.go"}}, files)
}

func TestIncludeFilter_ChangedFiles_multiplePrefixes(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{
			{Path: "cmd/main.go"}, {Path: "src/app.go"}, {Path: "pkg/util.go"},
			{Path: "vendor/lib.go"},
		},
	}
	f := NewIncludeFilter(inner, []string{"src", "pkg"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "src/app.go"}, {Path: "pkg/util.go"}}, files)
}

func TestIncludeFilter_ChangedFiles_noneMatch(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{{Path: "vendor/a.go"}, {Path: "vendor/b.go"}},
	}
	f := NewIncludeFilter(inner, []string{"src"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestIncludeFilter_ChangedFiles_exactMatch(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{{Path: "Makefile"}, {Path: "src/app.go"}},
	}
	f := NewIncludeFilter(inner, []string{"Makefile"})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "Makefile"}}, files)
}

func TestIncludeFilter_ChangedFiles_innerError(t *testing.T) {
	inner := &mockRenderer{changedErr: errors.New("git failed")}
	f := NewIncludeFilter(inner, []string{"src"})

	_, err := f.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include filter, changed files")
	assert.Contains(t, err.Error(), "git failed")
}

func TestIncludeFilter_ChangedFiles_prefixNormalization(t *testing.T) {
	inner := &mockRenderer{
		changedFiles: []FileEntry{{Path: "src/app.go"}, {Path: "vendor/lib.go"}},
	}
	f := NewIncludeFilter(inner, []string{" src/ ", "", "  "})

	files, err := f.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "src/app.go"}}, files)
	assert.Len(t, f.prefixes, 1, "empty/whitespace prefixes should be skipped")
}

func TestIncludeFilter_FileDiff_passthrough(t *testing.T) {
	lines := []DiffLine{
		{OldNum: 1, NewNum: 1, Content: "line1", ChangeType: ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "line2", ChangeType: ChangeContext},
	}
	inner := &mockRenderer{fileDiff: lines}
	f := NewIncludeFilter(inner, []string{"src"})

	// even a file NOT matching include prefix is passed through — filtering is only at file list level
	result, err := f.FileDiff("", "vendor/foo.go", false)
	require.NoError(t, err)
	assert.Equal(t, lines, result)
}

func TestIncludeFilter_FileDiff_innerError(t *testing.T) {
	inner := &mockRenderer{fileDiffErr: errors.New("read failed")}
	f := NewIncludeFilter(inner, []string{"src"})

	_, err := f.FileDiff("", "foo.go", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include filter, file diff foo.go")
	assert.Contains(t, err.Error(), "read failed")
}

func TestMatchesPrefix(t *testing.T) {
	prefixes := []string{"src", "pkg/util"}

	tests := []struct {
		file string
		want bool
	}{
		{"src/app.go", true},
		{"src/lib/deep.go", true},
		{"src", true},
		{"pkg/util/helper.go", true},
		{"pkg/util", true},
		{"pkg/other.go", false},
		{"srcutil/foo.go", false},
		{"other/src/foo.go", false},
		{"vendor/lib.go", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesPrefix(tt.file, prefixes))
		})
	}
}

func TestNormalizePrefixes(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		want   []string
		wantN  int
	}{
		{"trailing slashes", []string{"src/", "vendor/"}, []string{"src", "vendor"}, 2},
		{"whitespace", []string{" src ", " vendor"}, []string{"src", "vendor"}, 2},
		{"empty strings", []string{"src", "", "  ", "vendor"}, []string{"src", "vendor"}, 2},
		{"nil input", nil, []string{}, 0},
		{"all empty", []string{"", " ", "  "}, []string{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePrefixes(tt.input)
			assert.Equal(t, tt.want, got)
			assert.Len(t, got, tt.wantN)
		})
	}
}
