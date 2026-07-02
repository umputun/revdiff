package ui

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

// outputState holds transient feedback for the O in-session output flush.
// hint is a status-bar message cleared on the next key press, mirroring
// reloadState.hint.
type outputState struct {
	hint string // transient status-bar message; cleared on next key press
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
	n := m.store.Count()
	if n == 0 {
		m.output.hint = "No annotations to flush"
		return m, nil
	}
	if err := m.store.WriteFile(m.cfg.outputPath); err != nil {
		log.Printf("[WARN] flush annotations to output: %v", err)
		m.output.hint = "Flush failed"
		return m, nil
	}
	noun := "annotations"
	if n == 1 {
		noun = "annotation"
	}
	m.output.hint = fmt.Sprintf("Wrote %d %s to output file", n, noun)
	return m, nil
}
