// Package handoff prepares commands that consume flushed review annotations.
package handoff

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

// Runner prepares a user-configured shell command for annotation handoff.
type Runner struct {
	command string
}

// New creates a post-flush command runner.
func New(command string) *Runner {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	return &Runner{command: command}
}

// Prepare returns a command that reads the flushed annotation snapshot from stdin.
func (r *Runner) Prepare(content string) *exec.Cmd {
	//nolint:gosec // the command is explicitly user-configured
	cmd := exec.CommandContext(context.Background(), "sh", "-c", r.command)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = io.Discard
	return cmd
}
