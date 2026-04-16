package editor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditor_resolve_EditorSet(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	t.Setenv("VISUAL", "vim")
	assert.Equal(t, []string{"nano"}, Editor{}.resolve())
}

func TestEditor_resolve_VisualFallback(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "vim")
	assert.Equal(t, []string{"vim"}, Editor{}.resolve())
}

func TestEditor_resolve_ViDefault(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	assert.Equal(t, []string{"vi"}, Editor{}.resolve())
}

func TestEditor_resolve_WhitespaceOnlyFallsBack(t *testing.T) {
	t.Setenv("EDITOR", "   ")
	t.Setenv("VISUAL", "vim")
	assert.Equal(t, []string{"vim"}, Editor{}.resolve())
}

func TestEditor_resolve_SplitsOnWhitespace(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")
	assert.Equal(t, []string{"code", "--wait"}, Editor{}.resolve())
}

func TestEditor_resolve_SplitsOnMultipleSpaces(t *testing.T) {
	t.Setenv("EDITOR", "  code   --wait  --reuse-window  ")
	assert.Equal(t, []string{"code", "--wait", "--reuse-window"}, Editor{}.resolve())
}

func TestEditor_resolve_HandlesQuotedPathWithSpaces(t *testing.T) {
	t.Setenv("EDITOR", `"/Applications/My Editor.app/Contents/MacOS/edit" --wait`)
	assert.Equal(t, []string{"/Applications/My Editor.app/Contents/MacOS/edit", "--wait"}, Editor{}.resolve())
}

func TestEditor_resolve_HandlesSingleQuotedArgs(t *testing.T) {
	t.Setenv("EDITOR", `sh -c 'vim "$@"' --`)
	assert.Equal(t, []string{"sh", "-c", `vim "$@"`, "--"}, Editor{}.resolve())
}

func TestEditor_resolve_HandlesEscapedSpaceInPath(t *testing.T) {
	t.Setenv("EDITOR", `/Applications/My\ Editor/edit --wait`)
	assert.Equal(t, []string{"/Applications/My Editor/edit", "--wait"}, Editor{}.resolve())
}

func TestEditor_resolve_UnterminatedQuoteKeepsRemainder(t *testing.T) {
	// misquoted $EDITOR: unterminated double quote. tokenizer treats the rest as
	// literal rather than silently dropping tokens, so exec produces a clear
	// "no such file" error instead of a misleading parse outcome.
	t.Setenv("EDITOR", `"/path/to/edit --wait`)
	got := Editor{}.resolve()
	assert.Equal(t, []string{"/path/to/edit --wait"}, got)
}

func TestEditor_tokenize_EmptyInput(t *testing.T) {
	assert.Empty(t, Editor{}.tokenize(""))
}

func TestEditor_tokenize_PreservesEmptyQuotedArg(t *testing.T) {
	// `cmd "" arg` is valid: the empty string is a positional argument.
	assert.Equal(t, []string{"cmd", "", "arg"}, Editor{}.tokenize(`cmd "" arg`))
}

func TestEditor_tokenize_DoubleQuoteEscapes(t *testing.T) {
	// inside double quotes, \" and \\ are the only escapes interpreted.
	assert.Equal(t, []string{`a"b\c`}, Editor{}.tokenize(`"a\"b\\c"`))
}

func TestEditor_tokenize_PreservesBackslashBeforeNonShellMeta(t *testing.T) {
	// outside quotes, backslash only escapes shell-meta chars. Other characters
	// (notably Windows path separators) keep the backslash intact.
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"windows path", `C:\Program\ Files\Editor\code.exe`, []string{`C:\Program Files\Editor\code.exe`}},
		{"backslash before letter kept", `foo\abar`, []string{`foo\abar`}},
		{"backslash before digit kept", `foo\1bar`, []string{`foo\1bar`}},
		{"backslash escapes space", `foo\ bar`, []string{`foo bar`}},
		{"backslash escapes quote", `foo\"bar`, []string{`foo"bar`}},
		{"backslash escapes backslash", `foo\\bar`, []string{`foo\bar`}},
		{"backslash escapes dollar", `foo\$bar`, []string{`foo$bar`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Editor{}.tokenize(tc.in))
		})
	}
}

func TestEditor_writeTempFile_WritesContent(t *testing.T) {
	path, err := Editor{}.writeTempFile("hello world")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	assert.True(t, strings.HasSuffix(path, ".md"), "expected .md suffix, got %q", path)
	assert.Equal(t, "revdiff-annot-", filepath.Base(path)[:len("revdiff-annot-")])

	data, err := os.ReadFile(path) //nolint:gosec // test uses known temp path
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestEditor_writeTempFile_EmptyContent(t *testing.T) {
	path, err := Editor{}.writeTempFile("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	data, err := os.ReadFile(path) //nolint:gosec // test uses known temp path
	require.NoError(t, err)
	assert.Empty(t, string(data))
}

func TestEditor_writeTempFile_MultilineContent(t *testing.T) {
	path, err := Editor{}.writeTempFile("line1\nline2\nline3")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	data, err := os.ReadFile(path) //nolint:gosec // test uses known temp path
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2\nline3", string(data))
}

func TestEditor_writeTempFile_UniquePaths(t *testing.T) {
	path1, err := Editor{}.writeTempFile("one")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path1) })

	path2, err := Editor{}.writeTempFile("two")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path2) })

	assert.NotEqual(t, path1, path2)
}

func TestEditor_writeTempFile_CreateFailure(t *testing.T) {
	// point TMPDIR at a path that cannot exist so CreateTemp fails.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nonexistent", "subdir"))

	path, err := Editor{}.writeTempFile("content")
	require.Error(t, err, "CreateTemp must fail when TMPDIR points to a missing parent")
	assert.Empty(t, path, "no path should be returned on create failure")
	assert.Contains(t, err.Error(), "create temp file", "error should be wrapped with context")
}

func TestEditor_Command_TempFileCreateFailurePropagates(t *testing.T) {
	// Command delegates to writeTempFile; verify the error path is surfaced rather than swallowed.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nonexistent", "subdir"))
	t.Setenv("EDITOR", "/bin/true")

	cmd, complete, err := Editor{}.Command("seed")
	require.Error(t, err)
	assert.Nil(t, cmd, "no cmd returned when temp file creation fails")
	assert.Nil(t, complete, "no complete fn returned when temp file creation fails")
}

func TestEditor_readResult_Success(t *testing.T) {
	path, err := Editor{}.writeTempFile("line1\nline2\n")
	require.NoError(t, err)

	content, resultErr := Editor{}.readResult(path, nil)

	require.NoError(t, resultErr)
	assert.Equal(t, "line1\nline2", content, "trailing newline should be trimmed")

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "temp file should be removed after read")
}

func TestEditor_readResult_EmptyFile(t *testing.T) {
	path, err := Editor{}.writeTempFile("")
	require.NoError(t, err)

	content, resultErr := Editor{}.readResult(path, nil)

	require.NoError(t, resultErr)
	assert.Empty(t, content)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestEditor_readResult_PreservesRunErr(t *testing.T) {
	// Documented contract: when runErr is non-nil, readResult surfaces runErr as
	// the returned error AND returns any content read from the temp file so
	// callers can preserve user work in the editorFinishedMsg payload. Losing
	// content on a soft editor error would surprise users.
	path, err := Editor{}.writeTempFile("unused content")
	require.NoError(t, err)

	runErr := errors.New("editor crashed")
	content, resultErr := Editor{}.readResult(path, runErr)

	assert.Equal(t, runErr, resultErr, "runErr must be preserved even when file reads successfully")
	assert.Equal(t, "unused content", content, "documented contract: content is populated alongside runErr so callers can keep user work")

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "temp file must be removed even on runErr")
}

func TestEditor_readResult_MissingTempFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.md")

	content, resultErr := Editor{}.readResult(path, nil)

	require.Error(t, resultErr, "missing file must surface as err")
	assert.Contains(t, resultErr.Error(), "read editor output")
	assert.Empty(t, content)
}

func TestEditor_readResult_MissingTempFilePreservesRunErr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.md")

	runErr := errors.New("editor exited non-zero")
	_, resultErr := Editor{}.readResult(path, runErr)

	assert.Equal(t, runErr, resultErr, "runErr takes precedence over read error")
}

func TestEditor_readResult_TrimsTrailingNewlinesOnly(t *testing.T) {
	path, err := Editor{}.writeTempFile("line1\n\nline2\n\n\n")
	require.NoError(t, err)

	content, resultErr := Editor{}.readResult(path, nil)
	require.NoError(t, resultErr)
	assert.Equal(t, "line1\n\nline2", content, "internal blank lines preserved, only trailing \\n trimmed")
}

func TestEditor_Command_WritesSeedAndBuildsCmd(t *testing.T) {
	t.Setenv("EDITOR", "/bin/true")

	cmd, complete, err := Editor{}.Command("seeded content")
	require.NoError(t, err)
	require.NotNil(t, cmd)
	require.NotNil(t, complete)

	require.NotEmpty(t, cmd.Args)
	assert.Equal(t, "/bin/true", cmd.Args[0])
	tempPath := cmd.Args[len(cmd.Args)-1]
	assert.True(t, strings.HasPrefix(filepath.Base(tempPath), "revdiff-annot-"))
	assert.True(t, strings.HasSuffix(tempPath, ".md"))

	data, err := os.ReadFile(tempPath) //nolint:gosec // test uses known temp path
	require.NoError(t, err)
	assert.Equal(t, "seeded content", string(data))

	content, completeErr := complete(nil)
	require.NoError(t, completeErr)
	assert.Equal(t, "seeded content", content)
	_, statErr := os.Stat(tempPath)
	assert.True(t, os.IsNotExist(statErr), "complete must remove the temp file")
}

func TestEditor_Command_WithEditorArgs(t *testing.T) {
	t.Setenv("EDITOR", "/bin/true --flag")

	cmd, complete, err := Editor{}.Command("")
	require.NoError(t, err)
	require.NotNil(t, cmd)

	require.Len(t, cmd.Args, 3, "args: [bin, --flag, tempPath]")
	assert.Equal(t, "/bin/true", cmd.Args[0])
	assert.Equal(t, "--flag", cmd.Args[1])

	tempPath := cmd.Args[2]
	_, _ = complete(nil)
	_, statErr := os.Stat(tempPath)
	assert.True(t, os.IsNotExist(statErr))
}
