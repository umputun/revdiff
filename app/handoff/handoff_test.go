package handoff

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerPrepare(t *testing.T) {
	out := filepath.Join(t.TempDir(), "captured.md")
	t.Setenv("REVDIFF_HANDOFF_TEST_OUTPUT", out)
	runner := New(`cat > "$REVDIFF_HANDOFF_TEST_OUTPUT"`)

	cmd := runner.Prepare("## a.go:1 (+)\nchange this\n")
	assert.Equal(t, io.Discard, cmd.Stdout)
	require.NoError(t, cmd.Run())

	data, err := os.ReadFile(out) //nolint:gosec // path is created under t.TempDir
	require.NoError(t, err)
	assert.Equal(t, "## a.go:1 (+)\nchange this\n", string(data))
}

func TestNewEmptyCommand(t *testing.T) {
	assert.Nil(t, New(" \t "))
}
