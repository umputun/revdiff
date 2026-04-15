package diff

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJj_ParseAnnotate(t *testing.T) {
	j := &Jj{}

	raw := "abc12345\tTest User\t1775860468\tline one\n" +
		"def67890\tAnother Dev\t1775860469\tline two\n"

	result, err := j.parseAnnotate(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "Test User", result[1].Author)
	assert.Equal(t, int64(1775860468), result[1].Time.Unix())

	assert.Equal(t, "Another Dev", result[2].Author)
	assert.Equal(t, int64(1775860469), result[2].Time.Unix())
}

func TestJj_ParseAnnotate_Empty(t *testing.T) {
	j := &Jj{}
	result, err := j.parseAnnotate("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestJj_ParseAnnotate_LastLineNoNewline(t *testing.T) {
	j := &Jj{}

	raw := "abc12345\tTest User\t1775860468\tline one\n" +
		"abc12345\tTest User\t1775860468\tline two"

	result, err := j.parseAnnotate(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "Test User", result[2].Author)
}

func TestJj_ParseAnnotate_BlankContent(t *testing.T) {
	j := &Jj{}

	// blank source line produces empty content field
	raw := "abc12345\tTest User\t1775860468\t\n" +
		"abc12345\tTest User\t1775860468\thello\n"

	result, err := j.parseAnnotate(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "Test User", result[1].Author)
	assert.Equal(t, "Test User", result[2].Author)
}

func TestJj_ParseAnnotate_MalformedLine(t *testing.T) {
	j := &Jj{}

	raw := "bad\tdata\n" +
		"abc12345\tTest User\t1775860468\tline one\n"

	result, err := j.parseAnnotate(raw)
	require.NoError(t, err)
	// line 1 is malformed (skipped), line 2 is valid at lineNum=2
	require.Len(t, result, 1)
	assert.Equal(t, "Test User", result[2].Author)
}

func TestJj_BlameTargetRef(t *testing.T) {
	j := &Jj{}

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "empty", ref: "", want: ""},
		{name: "single ref", ref: "HEAD", want: "@-"},
		{name: "single hash", ref: "abc123", want: "abc123"},
		{name: "bookmark", ref: "main", want: "main"},
		{name: "double dot", ref: "main..feature", want: "feature"},
		{name: "triple dot", ref: "main...feature", want: "feature"},
		{name: "HEAD range", ref: "HEAD~3..HEAD", want: "@-"},
		{name: "left empty double dot", ref: "..feature", want: ""},
		{name: "right empty double dot", ref: "main..", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, j.blameTargetRef(tt.ref))
		})
	}
}

func TestJj_FileBlame_Integration(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not available")
	}

	dir := setupJjRepo(t)
	j := newJjForTest(dir)

	writeFile(t, dir, "hello.txt", "line one\n")
	jjCmd(t, dir, "describe", "-m", "first", "--quiet")
	jjCmd(t, dir, "new", "-m", "second", "--quiet")
	writeFile(t, dir, "hello.txt", "line one\nline two\n")

	blame, err := j.FileBlame("", "hello.txt", false)
	require.NoError(t, err)
	require.Len(t, blame, 2)

	assert.Equal(t, "Test User", blame[1].Author)
	assert.Equal(t, "Test User", blame[2].Author)

	assert.False(t, blame[2].Time.Before(blame[1].Time))
}
