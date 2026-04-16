package ui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// ExternalEditor abstracts launching the user's editor on seeded content and
// reading the result back. The default wiring is app/editor.Editor; tests may
// inject a stub. Defined on the consumer side per Go convention.
type ExternalEditor interface {
	// Command prepares the editor invocation for content. Returns a *exec.Cmd
	// ready to hand to tea.ExecProcess plus a complete function that the caller
	// invokes from the completion callback to read the edited content back. The
	// complete function also handles any temp-file cleanup.
	Command(content string) (*exec.Cmd, func(error) (string, error), error)
}

// editorFinishedMsg is dispatched after the external editor spawned via Ctrl+E
// exits. The target fields (fileLevel, line, changeType) are captured when the
// editor is opened so subsequent cursor movement during editing does not
// misroute the saved annotation.
type editorFinishedMsg struct {
	content    string
	err        error
	fileLevel  bool
	line       int
	changeType string
}

// openEditor returns a tea.Cmd that suspends the program, launches the user's
// editor on a temp file seeded with the current annotation input value, and
// emits an editorFinishedMsg on exit. Target annotation fields (fileLevel,
// line, changeType) are captured now so subsequent cursor movement during
// editing does not misroute the saved annotation.
func (m *Model) openEditor() tea.Cmd {
	content := m.annot.input.Value()
	fileLevel := m.annot.fileAnnotating

	var line int
	var changeType string
	if !fileLevel {
		dl, ok := m.cursorDiffLine()
		if !ok {
			return nil
		}
		line = m.diffLineNum(dl)
		changeType = string(dl.ChangeType)
	}

	cmd, complete, err := m.editor.Command(content)
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: err, fileLevel: fileLevel, line: line, changeType: changeType}
		}
	}

	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		text, finalErr := complete(runErr)
		return editorFinishedMsg{
			content:    text,
			err:        finalErr,
			fileLevel:  fileLevel,
			line:       line,
			changeType: changeType,
		}
	})
}
