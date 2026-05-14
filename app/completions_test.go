package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintCompletionScript_Bash(t *testing.T) {
	var buf bytes.Buffer
	err := printCompletionScript(&buf, "bash", "/usr/local/bin/revdiff")
	require.NoError(t, err)
	body := buf.String()
	assert.Contains(t, body, "_revdiff")
	assert.Contains(t, body, "GO_FLAGS_COMPLETION=1 /usr/local/bin/revdiff")
	assert.Contains(t, body, "complete -F _revdiff /usr/local/bin/revdiff")
	// bare <TAB> injects "--" to avoid positional fallback
	assert.Contains(t, body, `if [ ${#args[@]} -eq 0 ] || ([ ${#args[@]} -eq 1 ] && [ -z "${args[0]}" ]); then`)
	assert.Contains(t, body, `args=("--")`)
}

func TestPrintCompletionScript_Zsh(t *testing.T) {
	var buf bytes.Buffer
	err := printCompletionScript(&buf, "zsh", "revdiff")
	require.NoError(t, err)
	body := buf.String()
	assert.Contains(t, body, "#compdef revdiff")
	assert.Contains(t, body, "GO_FLAGS_COMPLETION=1 revdiff")
	// bare <TAB> injects "--" to avoid positional fallback
	assert.Contains(t, body, `if [ ${#args[@]} -eq 0 ] || ([ ${#args[@]} -eq 1 ] && [ -z "${args[1]}" ]); then`)
	assert.Contains(t, body, `args=("--")`)
}

func TestPrintCompletionScript_Fish(t *testing.T) {
	var buf bytes.Buffer
	err := printCompletionScript(&buf, "fish", "revdiff")
	require.NoError(t, err)
	body := buf.String()
	assert.Contains(t, body, "function __revdiff_complete")
	assert.Contains(t, body, "commandline -opc")
	assert.Contains(t, body, "commandline -ct")
	assert.Contains(t, body, "complete -c revdiff")
	assert.Contains(t, body, "GO_FLAGS_COMPLETION=1 revdiff")
}

func TestPrintCompletionScript_SanitizeName(t *testing.T) {
	var buf bytes.Buffer
	err := printCompletionScript(&buf, "bash", "./my-tool.bin")
	require.NoError(t, err)
	body := buf.String()
	// identifier-safe name should be used in function name, full path in invocation
	assert.Contains(t, body, "_my_tool_bin")
	assert.Contains(t, body, "GO_FLAGS_COMPLETION=1 ./my-tool.bin")
	assert.Contains(t, body, "complete -F _my_tool_bin ./my-tool.bin")
}

func TestCompletionShellComplete(t *testing.T) {
	var cs completionShell

	t.Run("empty prefix returns all", func(t *testing.T) {
		res := cs.Complete("")
		items := make([]string, len(res))
		for i, c := range res {
			items[i] = c.Item
		}
		assert.Equal(t, []string{"bash", "zsh", "fish"}, items)
	})

	t.Run("partial match", func(t *testing.T) {
		res := cs.Complete("ba")
		require.Len(t, res, 1)
		assert.Equal(t, "bash", res[0].Item)
	})

	t.Run("no match", func(t *testing.T) {
		res := cs.Complete("xyz")
		assert.Empty(t, res)
	})

	t.Run("prefix z returns zsh", func(t *testing.T) {
		res := cs.Complete("z")
		require.Len(t, res, 1)
		assert.Equal(t, "zsh", res[0].Item)
	})
}

func TestSanitizeShellIdent(t *testing.T) {
	assert.Equal(t, "revdiff", sanitizeShellIdent("revdiff"))
	assert.Equal(t, "my_tool", sanitizeShellIdent("my-tool"))
	assert.Equal(t, "my_tool_bin", sanitizeShellIdent("my-tool.bin"))
	assert.Equal(t, "_23", sanitizeShellIdent("123"))
	assert.Equal(t, "___", sanitizeShellIdent("!@#"))
}
