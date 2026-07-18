package ui

import (
	"fmt"
	"log"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// PostFlushHook prepares the external command run after an in-session output flush.
type PostFlushHook interface {
	Prepare(content string) *exec.Cmd
}

// outputState holds transient feedback for the O in-session output flush.
// hint is a status-bar message cleared on the next key press, mirroring
// reloadState.hint.
type outputState struct {
	hint string // transient status-bar message; cleared on next key press
}

type postFlushFinishedMsg struct {
	err          error
	writtenHint  string
	restoreMouse bool
}

// handleFlushOutput writes the current annotations to the configured --output
// file without exiting. It is a pure export — the store is never mutated, so
// annotations persist in-session and can be re-flushed. Feedback is reported
// through output.hint.
func (m Model) handleFlushOutput() (tea.Model, tea.Cmd) {
	if m.cfg.outputPath == "" {
		m.output.hint = "Output flush requires -o/--output"
		return m, nil
	}
	content := m.store.FormatOutput()
	if content == "" {
		m.output.hint = "No annotations to flush"
		return m, nil
	}
	if err := m.store.WriteFile(m.cfg.outputPath); err != nil {
		log.Printf("[WARN] flush annotations to output: %v", err)
		m.output.hint = "Flush failed"
		return m, nil
	}
	n := m.store.Count()
	noun := "annotations"
	if n == 1 {
		noun = "annotation"
	}
	writtenHint := fmt.Sprintf("Wrote %d %s to output file", n, noun)
	if m.postFlushHook == nil {
		m.output.hint = writtenHint
		return m, nil
	}

	cmd := m.postFlushHook.Prepare(content)
	m.output.hint = writtenHint + "; running post-flush command"
	return m, tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		return postFlushFinishedMsg{err: runErr, writtenHint: writtenHint, restoreMouse: m.cfg.mouseTracking}
	})
}

func (m Model) handlePostFlushFinished(msg postFlushFinishedMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.restoreMouse {
		cmd = tea.EnableMouseCellMotion
	}
	if msg.err != nil {
		log.Printf("[WARN] post-flush command failed: %v", msg.err)
		m.output.hint = msg.writtenHint + "; post-flush command failed"
		return m, cmd
	}
	m.output.hint = msg.writtenHint + " and ran post-flush command"
	return m, cmd
}
