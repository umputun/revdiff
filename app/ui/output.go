package ui

import (
	"errors"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// outputState holds transient feedback for the O in-session output flush.
// hint is a status-bar message cleared on the next key press, mirroring
// reloadState.hint.
type outputState struct {
	path         string // effective output destination; initialized from --output, mutable after the O prompt
	hint         string // transient status-bar message; cleared on next key press
	checkpointed bool   // true after this session successfully writes a file snapshot
}

type postFlushFinishedMsg struct {
	err         error
	successHint string
	failureHint string
}

// handleFlushOutput exports the current annotations through the configured
// output file and/or post-flush command without exiting. The store is never
// mutated, so annotations persist in-session and can be re-flushed. Feedback
// is reported through output.hint.
func (m Model) handleFlushOutput() (tea.Model, tea.Cmd) {
	n := m.store.Count()
	if n == 0 {
		m.output.hint = "No annotations to flush"
		return m, nil
	}
	if m.output.path == "" && m.postFlushHook == nil {
		m.clearPendingInputState()
		m.overlay.OpenOutputPath(overlay.OutputPathSpec{})
		return m, nil
	}
	noun := annotationNoun(n)

	var content, writtenHint string
	if m.output.path != "" {
		var err error
		content, err = m.store.WriteFile(m.output.path)
		if err != nil {
			log.Printf("[WARN] flush annotations to output: %v", err)
			m.output.hint = "Flush failed"
			return m, nil
		}
		m.output.checkpointed = true
		writtenHint = fmt.Sprintf("Wrote %d %s to output file", n, noun)
	} else {
		content = m.store.FormatOutput()
	}

	if m.postFlushHook == nil {
		m.output.hint = writtenHint
		return m, nil
	}

	runningHint := fmt.Sprintf("Running post-flush command with %d %s", n, noun)
	successHint := fmt.Sprintf("Ran post-flush command with %d %s", n, noun)
	failureHint := "Post-flush command failed"
	if writtenHint != "" {
		runningHint = writtenHint + "; running post-flush command"
		successHint = writtenHint + " and ran post-flush command"
		failureHint = writtenHint + "; post-flush command failed"
	}
	cmd := m.postFlushHook.Prepare(content)
	m.output.hint = runningHint
	return m, tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		return postFlushFinishedMsg{
			err:         runErr,
			successHint: successHint,
			failureHint: failureHint,
		}
	})
}

// handleOutputPathChoice handles a path submitted by the output-path overlay.
// The first attempt refuses existing targets; confirmed retries overwrite via
// the same atomic Store.WriteFile path used by --output.
func (m Model) handleOutputPathChoice(choice *overlay.OutputPathChoice) (tea.Model, tea.Cmd) {
	if choice == nil {
		return m, nil
	}

	var err error
	if choice.Overwrite {
		_, err = m.store.WriteFile(choice.Path)
	} else {
		_, err = m.store.WriteFileIfAbsent(choice.Path)
	}
	if errors.Is(err, annotation.ErrOutputExists) {
		m.overlay.OpenOutputPath(overlay.OutputPathSpec{Path: choice.Path, ConfirmOverwrite: true})
		return m, nil
	}
	if err != nil {
		log.Printf("[WARN] flush annotations to selected output: %v", err)
		m.overlay.OpenOutputPath(overlay.OutputPathSpec{Path: choice.Path, Error: "Save failed: " + err.Error()})
		return m, nil
	}

	m.output.path = choice.Path
	m.output.checkpointed = true
	m.output.hint = fmt.Sprintf("Wrote %d %s to output file", m.store.Count(), annotationNoun(m.store.Count()))
	return m, nil
}

func annotationNoun(n int) string {
	if n == 1 {
		return "annotation"
	}
	return "annotations"
}

func (m Model) handlePostFlushFinished(msg postFlushFinishedMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	// restore the current tracking state, not the startup flag (see handleEditorFinished)
	if m.mouseTracking {
		cmd = tea.EnableMouseCellMotion
	}
	if msg.err != nil {
		log.Printf("[WARN] post-flush command failed: %v", msg.err)
		m.output.hint = msg.failureHint
		return m, cmd
	}
	m.output.hint = msg.successHint
	return m, cmd
}
