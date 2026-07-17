// Package agent runs an external command that receives the current annotation
// output on its stdin. It is independent of the TUI: the caller invokes Send
// from a background goroutine (a tea.Cmd) so a slow or hung command never
// blocks the event loop.
//
// The command is split into argv with a POSIX-style, quote-aware tokenizer and
// executed directly — there is no shell. This mirrors app/editor, which is
// deliberately shell-free (a value like `code --wait` or `relay "with space"`
// works, but pipes, redirection, and variable expansion are not interpreted).
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
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
	Command string        // command line, argv-split (no shell); receives annotations on stdin
	Timeout time.Duration // per-send timeout; <= 0 uses defaultTimeout
}

// Send runs the command with payload written to its stdin and waits for it to
// exit. A non-zero exit or spawn failure is returned as an error with a short,
// bounded slice of stderr folded in so the caller can surface a reason.
func (r Runner) Send(payload string) error {
	argv := r.tokenize(r.Command)
	if len(argv) == 0 {
		return errors.New("agent command is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	//nolint:gosec // command is user-supplied by design (--agent-cmd / REVDIFF_AGENT_CMD); argv split, no shell (mirrors app/editor)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
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

// shellMetaChars are the runes whose shell significance backslash can escape
// outside quotes (POSIX-style). Other characters — notably Windows-style path
// separators in `C:\foo` — preserve the backslash literally.
var shellMetaChars = map[rune]bool{
	' ': true, '\t': true, '\n': true,
	'\'': true, '"': true, '\\': true, '$': true, '`': true,
}

// tokenize splits s into shell-style tokens honoring single and double quotes
// and backslash escapes, without invoking a shell. Mirrors app/editor.tokenize
// so --agent-cmd parses command lines the same way $EDITOR does: variable
// expansion, subshells, pipes, and redirection are NOT interpreted — those are
// caller-supplied values handled by exec directly. Unterminated quotes are
// treated as literal to end-of-string so a misquoted command never drops input.
func (r Runner) tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inSingle, inDouble, hasToken := false, false, false
	flush := func() {
		if hasToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			hasToken = false
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		consumed, open := r.tokenizeStep(runes, i, runes[i], &cur, &hasToken, inSingle, inDouble)
		i += consumed
		inSingle, inDouble = open.single, open.double
		if open.flush {
			flush()
		}
	}
	flush()
	return tokens
}

type quoteState struct {
	single, double, flush bool
}

// tokenizeStep processes one input rune. Returns the additional runes consumed
// (for escape sequences) and the updated quote state plus a flush signal.
func (Runner) tokenizeStep(runes []rune, i int, r rune, cur *strings.Builder, hasToken *bool, inSingle, inDouble bool) (int, quoteState) {
	state := quoteState{single: inSingle, double: inDouble}
	switch {
	case inSingle:
		if r == '\'' {
			state.single = false
			return 0, state
		}
		cur.WriteRune(r)
		*hasToken = true
	case inDouble:
		if r == '"' {
			state.double = false
			return 0, state
		}
		if r == '\\' && i+1 < len(runes) {
			next := runes[i+1]
			if next == '"' || next == '\\' || next == '$' || next == '`' {
				cur.WriteRune(next)
				*hasToken = true
				return 1, state
			}
		}
		cur.WriteRune(r)
		*hasToken = true
	case r == '\'':
		state.single = true
		*hasToken = true
	case r == '"':
		state.double = true
		*hasToken = true
	case r == '\\' && i+1 < len(runes) && shellMetaChars[runes[i+1]]:
		cur.WriteRune(runes[i+1])
		*hasToken = true
		return 1, state
	case r == ' ' || r == '\t' || r == '\n':
		state.flush = true
	default:
		cur.WriteRune(r)
		*hasToken = true
	}
	return 0, state
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
