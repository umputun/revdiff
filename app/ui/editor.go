package ui

import (
	"log"
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
// exits. The target fields (fileName, fileLevel, line, changeType) are
// captured when the editor is opened so subsequent cursor movement or file
// navigation during editing does not misroute the saved annotation. The seed
// is the content written into the temp file before the editor started; it is
// used to distinguish a launch-time failure (where the temp file is untouched
// and the read-back returns the seed verbatim) from a post-save soft failure
// (where the read-back returns the user's actual edits).
type editorFinishedMsg struct {
	content    string
	seed       string
	err        error
	fileName   string
	fileLevel  bool
	line       int
	changeType string
}

// openEditor returns a tea.Cmd that suspends the program, launches the user's
// editor on a temp file seeded with the current annotation input value, and
// emits an editorFinishedMsg on exit. Target annotation fields (fileName,
// fileLevel, line, changeType) are captured now so subsequent cursor movement
// or file navigation during editing does not misroute the saved annotation.
func (m *Model) openEditor() tea.Cmd {
	content := m.annot.input.Value()
	// when re-editing an existing multi-line annotation, the textinput is kept
	// empty (sanitizer would flatten \n). seed the editor from the stashed
	// original so Ctrl+E resumes with the full content, not a blank file.
	if content == "" && m.annot.existingMultiline != "" {
		content = m.annot.existingMultiline
	}
	fileName := m.file.name
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
			return editorFinishedMsg{err: err, seed: content, fileName: fileName, fileLevel: fileLevel, line: line, changeType: changeType}
		}
	}

	seed := content
	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		text, finalErr := complete(runErr)
		return editorFinishedMsg{
			content:    text,
			seed:       seed,
			err:        finalErr,
			fileName:   fileName,
			fileLevel:  fileLevel,
			line:       line,
			changeType: changeType,
		}
	})
}

// handleEditorFinished processes the result of an external editor session.
// On success with non-empty content, the captured target fields drive
// saveComment — this bypasses the single-line textinput so embedded newlines
// survive. Empty content routes through cancelAnnotation, preserving any
// pre-existing annotation on that line. On editor error, the seed (content
// written into the temp file before the editor started) distinguishes
// launch-time failures from post-save soft failures: if the read-back content
// equals the seed verbatim, the editor never changed the file (binary missing,
// tty release failure before spawn, user opened-and-quit without saving), so
// the annotation mode is left open with the prior input untouched. If the
// content differs from the seed, the user edited successfully before a soft
// failure (tty restore error post-save, non-zero exit after save), so the
// content is saved and the error logged — preserving user work.
func (m Model) handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// covers editor spawn failure, non-zero exit, tty release/restore errors,
		// temp-file creation/write failures, and post-exit file read errors.
		log.Printf("[WARN] external editor session error: %v", msg.err)
		if msg.content == "" || msg.content == msg.seed {
			// launch-time failure (temp file untouched, read-back returns seed) or
			// nothing to save — preserve annotation mode with prior input intact.
			// known edge case: seed comparison is a heuristic for "editor did not
			// produce new content." false negative when user saves content identical
			// to the seed AND RestoreTerminal fails afterward — treated as no-edit
			// and drop. trade-off: preserving input state on ambiguous errors lets
			// users retry. the alternative (save on any non-empty content) caused
			// the iter-2 regression where launch-time failures silently saved seed.
			return m, nil
		}
		// fall through to save: user wrote content before the error, preserve it
	}
	if msg.content == "" {
		m.cancelAnnotation()
		return m, nil
	}
	m.saveComment(msg.content, msg.fileName, msg.fileLevel, msg.line, msg.changeType)
	return m, nil
}
