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
		t.Skip("uses the POSIX `tee` command")
	}
	out := filepath.Join(t.TempDir(), "got.txt")
	// tee copies stdin to the given file (argv-only, no shell redirection).
	err := Runner{Command: "tee " + out}.Send("hello annotations")
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
		t.Skip("uses the POSIX `ls` command")
	}
	// ls of a missing path writes to stderr and exits non-zero, no shell needed.
	err := Runner{Command: "ls /no/such/revdiff/agent/path/zzzz"}.Send("payload")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No such file", "stderr should be folded into the error")
}

func TestRunner_Send_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the POSIX `sleep` command")
	}
	err := Runner{Command: "sleep 5", Timeout: 50 * time.Millisecond}.Send("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestRunner_tokenize(t *testing.T) {
	r := Runner{}
	assert.Empty(t, r.tokenize(""))
	assert.Empty(t, r.tokenize("   "))
	assert.Equal(t, []string{"relay", "a", "b"}, r.tokenize("relay a b"))
	assert.Equal(t, []string{"relay", "a b", "c"}, r.tokenize(`relay "a b" c`), "double quotes keep spaces in one arg")
	assert.Equal(t, []string{"relay", "a b"}, r.tokenize(`relay 'a b'`), "single quotes keep spaces in one arg")
	assert.Equal(t, []string{"curl", "-fsS", "--data-binary", "@-"}, r.tokenize("curl -fsS --data-binary @-"))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "short", truncate("short"))

	long := strings.Repeat("x", stderrCap+50)
	got := truncate(long)
	assert.Len(t, []rune(got), stderrCap+1, "truncated to cap plus a single ellipsis rune")
	assert.True(t, strings.HasSuffix(got, "…"))
}
