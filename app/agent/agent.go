// Package agent runs an external command that receives the current annotation
// output on its stdin. It is independent of the TUI: the caller invokes Send
// from a background goroutine (a tea.Cmd) so a slow or hung command never
// blocks the event loop. The command is run through the system shell so users
// can supply ordinary shell in --agent-cmd (quoting, arguments, pipes).
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// defaultTimeout bounds a single Send so a hung relay command cannot leak a
// goroutine for the life of the review session.
const defaultTimeout = 30 * time.Second

// stderrCap limits how many bytes of a failing command's stderr are folded
// into the returned error, so a megabyte of output cannot bloat a status hint.
const stderrCap = 200

// Runner sends annotation output to an external command via stdin. It is a
// value type carrying only the command line; the type exists to group the
// behavior as a method and to satisfy the consumer-side sink interface.
type Runner struct {
	Command string        // shell command line; receives annotations on stdin
	Timeout time.Duration // per-send timeout; <= 0 uses defaultTimeout
}

// Send runs the command with payload written to its stdin and waits for it to
// exit. A non-zero exit or spawn failure is returned as an error with a short,
// bounded slice of stderr folded in so the caller can surface a reason.
func (r Runner) Send(payload string) error {
	if strings.TrimSpace(r.Command) == "" {
		return errors.New("agent command is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	name, args := shellCommand(r.Command)
	//nolint:gosec // the command is user-supplied by design (--agent-cmd / REVDIFF_AGENT_CMD)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("agent command timed out after %s", timeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, truncate(msg))
		}
		return fmt.Errorf("run agent command: %w", err)
	}
	return nil
}

// shellCommand returns the shell binary and arguments used to run command as a
// single shell line on the current platform.
func shellCommand(command string) (name string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
}

// truncate shortens s to stderrCap runes, appending an ellipsis when it had to
// cut. Operating on runes avoids splitting a multi-byte character.
func truncate(s string) string {
	r := []rune(s)
	if len(r) <= stderrCap {
		return s
	}
	return string(r[:stderrCap]) + "…"
}
