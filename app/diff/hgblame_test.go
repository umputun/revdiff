package diff

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHg_ParseAnnotate(t *testing.T) {
	h := &Hg{}

	raw := "0\t6c394a888c81\tTest User\t1775860468 -7200\tline one\n" +
		"1\t3721fe584e35\tAnother Dev\t1775860469 -7200\tline two\n"

	result, err := h.parseAnnotate(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "Test User", result[1].Author)
	assert.Equal(t, int64(1775860468), result[1].Time.Unix())

	assert.Equal(t, "Another Dev", result[2].Author)
	assert.Equal(t, int64(1775860469), result[2].Time.Unix())
}

func TestHg_ParseAnnotate_Empty(t *testing.T) {
	h := &Hg{}
	result, err := h.parseAnnotate("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestHg_ParseAnnotate_BlankLineContent(t *testing.T) {
	h := &Hg{}

	// line with empty content (5th field is empty or missing)
	raw := "0\t6c394a888c81\tTest User\t1775860468 -7200\t\n" +
		"0\t6c394a888c81\tTest User\t1775860468 -7200\thello\n"

	result, err := h.parseAnnotate(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "Test User", result[1].Author)
	assert.Equal(t, "Test User", result[2].Author)
}

func TestHg_ParseAnnotate_MalformedLines(t *testing.T) {
	h := &Hg{}

	// malformed line with fewer than 4 fields is skipped but counted
	raw := "bad\tdata\n" +
		"0\t6c394a888c81\tTest User\t1775860468 -7200\tline one\n"

	result, err := h.parseAnnotate(raw)
	require.NoError(t, err)
	// line 1 is malformed (skipped), line 2 is valid at lineNum=2
	require.Len(t, result, 1)
	assert.Equal(t, "Test User", result[2].Author)
}

func TestHg_BlameTargetRef(t *testing.T) {
	h := &Hg{}

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "empty", ref: "", want: ""},
		{name: "single ref", ref: "HEAD", want: "."},
		{name: "single hash", ref: "abc123", want: "abc123"},
		{name: "double dot", ref: "main..feature", want: "feature"},
		{name: "triple dot", ref: "main...feature", want: "feature"},
		{name: "HEAD range", ref: "HEAD~3..HEAD", want: "."},
		{name: "left empty double dot", ref: "..feature", want: ""},
		{name: "right empty double dot", ref: "main..", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, h.blameTargetRef(tt.ref))
		})
	}
}

func TestHg_FileBlame_Integration(t *testing.T) {
	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg not available")
	}

	dir := setupHgRepo(t)
	h := NewHg(dir)

	writeFile(t, dir, "hello.txt", "line one\n")
	hgCmd(t, dir, "add", "hello.txt")
	hgCmd(t, dir, "commit", "-m", "first")

	writeFile(t, dir, "hello.txt", "line one\nline two\n")
	hgCmd(t, dir, "commit", "-m", "second")

	blame, err := h.FileBlame("", "hello.txt", false)
	require.NoError(t, err)
	require.Len(t, blame, 2)

	// both lines should have "Test User" as author
	assert.Equal(t, "Test User", blame[1].Author)
	assert.Equal(t, "Test User", blame[2].Author)

	// second line should have a later or equal timestamp
	assert.False(t, blame[2].Time.Before(blame[1].Time))
}
