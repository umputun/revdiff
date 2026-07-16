package ui

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

// AgentSink hands the current annotation output to an external agent command.
// Implemented by agent.Runner and wired through ModelConfig.Agent; a nil sink
// disables the agent flush. Defined on the consumer side per Go convention.
type AgentSink interface {
	Send(payload string) error
}

// outputState holds transient feedback for the O in-session output flush.
// hint is a status-bar message cleared on the next key press, mirroring
// reloadState.hint.
type outputState struct {
	hint string // transient status-bar message; cleared on next key press
}

// agentFlushedMsg reports the result of an asynchronous agent send started by
// handleFlushOutput. n is the annotation count at flush time (for the success
// hint); err is non-nil when the command failed to run or exited non-zero.
type agentFlushedMsg struct {
	n   int
	err error
}

// handleFlushOutput hands the current annotations off without exiting: it writes
// them to the configured --output file and/or pipes them to the --agent-cmd
// command. It is a pure export — the store is never mutated, so annotations
// persist in-session and can be re-flushed. The file write is synchronous; the
// agent send runs asynchronously (returned tea.Cmd) so a slow command never
// blocks the event loop. Feedback is reported through output.hint.
func (m Model) handleFlushOutput() (tea.Model, tea.Cmd) {
	if m.cfg.outputPath == "" && m.agent == nil {
		m.output.hint = "Output flush requires -o/--output or --agent-cmd"
		return m, nil
	}
	n := m.store.Count()
	if n == 0 {
		m.output.hint = "No annotations to flush"
		return m, nil
	}
	payload := m.store.FormatOutput()

	wroteFile := false
	if m.cfg.outputPath != "" {
		if err := m.store.WriteFile(m.cfg.outputPath); err != nil {
			log.Printf("[WARN] flush annotations to output: %v", err)
			m.output.hint = "Flush failed"
			return m, nil
		}
		wroteFile = true
	}

	if m.agent != nil {
		m.output.hint = flushSendingHint(n, wroteFile)
		return m, m.sendToAgent(payload, n)
	}

	m.output.hint = fmt.Sprintf("Wrote %d %s to output file", n, pluralAnnotation(n))
	return m, nil
}

// sendToAgent returns a tea.Cmd that runs the agent command with payload on its
// stdin off the main goroutine, reporting completion via agentFlushedMsg.
func (m Model) sendToAgent(payload string, n int) tea.Cmd {
	sink := m.agent
	return func() tea.Msg {
		return agentFlushedMsg{n: n, err: sink.Send(payload)}
	}
}

// handleAgentFlushed updates the transient hint with the outcome of an agent
// send. A failure is logged and reported without disturbing the store.
func (m Model) handleAgentFlushed(msg agentFlushedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Printf("[WARN] flush annotations to agent: %v", msg.err)
		m.output.hint = "Agent flush failed"
		return m, nil
	}
	m.output.hint = fmt.Sprintf("Sent %d %s to agent", msg.n, pluralAnnotation(msg.n))
	return m, nil
}

// flushSendingHint is the interim hint shown while an agent send is in flight,
// noting whether the output file was also written in the same flush.
func flushSendingHint(n int, wroteFile bool) string {
	if wroteFile {
		return fmt.Sprintf("Wrote %d %s; sending to agent…", n, pluralAnnotation(n))
	}
	return fmt.Sprintf("Sending %d %s to agent…", n, pluralAnnotation(n))
}

// pluralAnnotation returns the singular or plural noun for n annotations.
func pluralAnnotation(n int) string {
	if n == 1 {
		return "annotation"
	}
	return "annotations"
}
