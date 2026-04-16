// Package editor spawns the user's external $EDITOR on seeded content and
// reads the result back. It is independent of the TUI — the caller wraps the
// returned *exec.Cmd with bubbletea's tea.ExecProcess (or equivalent).
package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Editor bundles external-editor operations. It is stateless; the type exists
// to group related behavior as methods per project convention.
type Editor struct{}

// Command prepares the editor invocation for content. It writes content to a
// freshly-created temp file and resolves the editor command, returning a ready
// *exec.Cmd plus a complete function. The caller hands the Cmd to
// tea.ExecProcess (or runs it directly) and invokes complete(runErr) from the
// completion callback — complete reads the file back, removes it, and returns
// the content (trailing newlines trimmed) plus any error. A non-nil runErr is
// preserved on the returned err even if the file reads successfully.
func (e Editor) Command(content string) (*exec.Cmd, func(error) (string, error), error) {
	tempPath, err := e.writeTempFile(content)
	if err != nil {
		return nil, nil, err
	}

	argv := e.resolve()
	//nolint:gosec // user-controlled editor binary by design (resolved from $EDITOR/$VISUAL)
	cmd := exec.CommandContext(context.Background(), argv[0], append(argv[1:], tempPath)...)

	complete := func(runErr error) (string, error) {
		return e.readResult(tempPath, runErr)
	}
	return cmd, complete, nil
}

// resolve returns the editor command and its arguments. Lookup order is
// $EDITOR → $VISUAL → "vi". Tokenization respects POSIX-style quoting so
// values like `code --wait`, `"/Applications/My Editor.app/Contents/MacOS/Edit" --wait`,
// or `sh -c 'vim "$@"' --` work as expected. Falls back to whitespace splitting
// when the input contains no quote characters.
func (e Editor) resolve() []string {
	for _, env := range []string{"EDITOR", "VISUAL"} {
		v := strings.TrimSpace(os.Getenv(env))
		if v == "" {
			continue
		}
		if tokens := e.tokenize(v); len(tokens) > 0 {
			return tokens
		}
	}
	return []string{"vi"}
}

// shellMetaChars are the runes whose shell significance backslash can escape
// outside quotes (POSIX-style). Other characters — notably Windows-style path
// separators in `C:\foo` — preserve the backslash literally.
var shellMetaChars = map[rune]bool{
	' ': true, '\t': true, '\n': true,
	'\'': true, '"': true, '\\': true, '$': true, '`': true,
}

// tokenize splits s into shell-style tokens honoring single and double quotes
// and backslash escapes. Outside quotes, backslash only escapes shell-meta
// characters — other characters preserve the backslash intact so paths like
// `C:\Program Files` survive. Unterminated quotes are treated as literal from
// the opening quote to end-of-string so a misquoted $EDITOR never drops user
// input. Not a full shell parser — variable expansion, subshells, and
// redirection are not interpreted; those are caller-supplied values handled
// by exec directly.
func (e Editor) tokenize(s string) []string {
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
		r := runes[i]
		consumed, open := e.tokenizeStep(runes, i, r, &cur, &hasToken, inSingle, inDouble)
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
func (Editor) tokenizeStep(runes []rune, i int, r rune, cur *strings.Builder, hasToken *bool, inSingle, inDouble bool) (int, quoteState) {
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

// writeTempFile creates a new temp file with a .md suffix and writes content
// into it. The returned path is paired with readResult which removes the file.
func (Editor) writeTempFile(content string) (string, error) {
	f, err := os.CreateTemp("", "revdiff-annot-*.md")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("close temp file: %w", err)
	}
	return f.Name(), nil
}

// readResult reads the temp file and returns its content (trailing newlines
// trimmed) plus any error, removing the file regardless of outcome. A non-nil
// runErr takes precedence — read errors only surface when runErr is nil.
func (Editor) readResult(tempPath string, runErr error) (string, error) {
	data, readErr := os.ReadFile(tempPath) //nolint:gosec // tempPath produced by writeTempFile
	_ = os.Remove(tempPath)
	if runErr != nil {
		return strings.TrimRight(string(data), "\n"), runErr
	}
	if readErr != nil {
		return "", fmt.Errorf("read editor output: %w", readErr)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
