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
// $EDITOR → $VISUAL → "vi". Split on whitespace so values like "code --wait"
// work as expected.
func (Editor) resolve() []string {
	for _, env := range []string{"EDITOR", "VISUAL"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return strings.Fields(v)
		}
	}
	return []string{"vi"}
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
