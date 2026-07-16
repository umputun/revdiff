package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner_Send_DeliversStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	out := filepath.Join(t.TempDir(), "got.txt")
	err := Runner{Command: "cat > " + out}.Send("hello annotations")
	require.NoError(t, err)

	data, err := os.ReadFile(out) //nolint:gosec // path is a t.TempDir() file
	require.NoError(t, err)
	assert.Equal(t, "hello annotations", string(data))
}

func TestRunner_Send_EmptyCommand(t *testing.T) {
	err := Runner{Command: "   "}.Send("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestRunner_Send_NonZeroExitFoldsStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	err := Runner{Command: "echo boom >&2; exit 3"}.Send("payload")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom", "stderr should be folded into the error")
}

func TestRunner_Send_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	err := Runner{Command: "sleep 5", Timeout: 50 * time.Millisecond}.Send("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", truncate("short"))

	long := strings.Repeat("x", stderrCap+50)
	got := truncate(long)
	assert.Len(t, []rune(got), stderrCap+1, "truncated to cap plus a single ellipsis rune")
	assert.True(t, strings.HasSuffix(got, "…"))
}
