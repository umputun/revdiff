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
	path, err := Editor{}.writeTempFile("unused content")
	require.NoError(t, err)

	runErr := errors.New("editor crashed")
	content, resultErr := Editor{}.readResult(path, runErr)

	assert.Equal(t, runErr, resultErr, "runErr must be preserved even when file reads successfully")
	assert.Equal(t, "unused content", content, "content should still be populated")

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
