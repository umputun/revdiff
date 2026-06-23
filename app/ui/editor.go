package ui

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/editor"
)

// ExternalEditor abstracts editor processes used by the UI.
// Annotation editing round-trips through a temp file,
// while source-file opening launches an existing worktree file
// without reading it back.
// The default wiring is app/editor.Editor; tests may inject a stub.
// Defined on the consumer side per Go convention.
type ExternalEditor interface {
	// Command prepares the editor invocation for content. Returns a *exec.Cmd
	// ready to hand to tea.ExecProcess plus a complete function that the caller
	// invokes from the completion callback to read the edited content back. The
	// complete function also handles any temp-file cleanup.
	Command(content string) (*exec.Cmd, func(error) (string, error), error)
	// SourceCommand prepares the editor invocation for an existing source file.
	SourceCommand(path string, line int) (*exec.Cmd, error)
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

	// restoreMouse requests mouse tracking after the editor returns.
	// Bubble Tea disables mouse modes while the child process owns the terminal;
	// editor setup failures never release the terminal, so they do not need this.
	restoreMouse bool
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
		// Earlier setup failures return before tea.ExecProcess releases the
		// terminal, so restoreMouse is set only on this completion path and only
		// when the session originally enabled mouse tracking.
		return editorFinishedMsg{
			content:      text,
			seed:         seed,
			err:          finalErr,
			restoreMouse: m.cfg.mouseTracking,
			fileName:     fileName,
			fileLevel:    fileLevel,
			line:         line,
			changeType:   changeType,
		}
	})
}

// sourceEditorFinishedMsg is dispatched after opening a worktree source file
// in the external editor.
type sourceEditorFinishedMsg struct {
	err                  error
	fileName             string
	reloadAfterCleanExit bool
}

// sourceEditorTargetResult is the UI-side decision for one source-editor
// request.
type sourceEditorTargetResult struct {
	// fileName is the displayed diff file captured when the editor launches.
	fileName string

	// sourcePath is the source file passed to ExternalEditor.
	sourcePath string

	// sourceLine is the optional one-based line passed to ExternalEditor.
	sourceLine int

	// reloadAfterCleanExit controls whether a clean editor exit reloads the
	// displayed diff file.
	reloadAfterCleanExit bool
}

func (m *Model) openSourceEditor() tea.Cmd {
	result, err := m.sourceEditorTarget()
	if err != nil {
		m.editorState.hint = fmt.Sprintf("Editor unavailable: %v", err)
		return nil
	}
	cmd, err := m.editor.SourceCommand(result.sourcePath, result.sourceLine)
	if err != nil {
		switch {
		case errors.Is(err, editor.ErrSourceMissing):
			m.editorState.hint = "Editor unavailable: file is missing"
			return nil
		case errors.Is(err, editor.ErrSourceNotRegular):
			m.editorState.hint = "Editor unavailable: file is not regular"
			return nil
		}
		return func() tea.Msg {
			return sourceEditorFinishedMsg{err: err, fileName: result.fileName, reloadAfterCleanExit: result.reloadAfterCleanExit}
		}
	}
	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		return sourceEditorFinishedMsg{err: runErr, fileName: result.fileName, reloadAfterCleanExit: result.reloadAfterCleanExit}
	})
}

// sourceEditorTarget resolves the current UI selection to the worktree source
// file that should be opened in the external editor.
//
// Selection errors describe states where launching the editor is not valid and
// are suitable for display after the "Editor unavailable: " prefix. Filesystem
// validation remains with ExternalEditor.SourceCommand so missing and
// non-regular files keep the same command-boundary handling as other source
// editor launches.
func (m Model) sourceEditorTarget() (sourceEditorTargetResult, error) {
	policy := m.cfg.sourceEditorPolicy
	if !policy.Available {
		return sourceEditorTargetResult{}, errors.New("source editor disabled")
	}
	if m.file.name == "" {
		return sourceEditorTargetResult{}, errors.New("no file loaded")
	}
	if m.tree.FileStatus(m.file.name) == diff.FileDeleted {
		return sourceEditorTargetResult{}, errors.New("file was deleted")
	}
	targetLine, ok, err := m.sourceEditorLine()
	if err != nil {
		return sourceEditorTargetResult{}, err
	}
	if !ok {
		targetLine = 0
	}
	if policy.DisallowAnnotatedFileEditing && m.hasCurrentFileLineAnnotations() {
		return sourceEditorTargetResult{}, errors.New("file has line annotations")
	}
	targetPath := cmp.Or(policy.ExactPath, m.file.name)
	if !filepath.IsAbs(targetPath) {
		// Relative displayed paths come from VCS diff data,
		// so confine them to the source root.
		// Absolute paths are explicit user input
		// (e.g. --only=/path/to/file)
		// and are therefore trusted for now.
		if policy.Root == "" {
			return sourceEditorTargetResult{}, errors.New("no source root")
		}
		if !filepath.IsLocal(targetPath) {
			return sourceEditorTargetResult{}, errors.New("file path escapes worktree")
		}
		root, err := os.OpenRoot(policy.Root)
		if err != nil {
			return sourceEditorTargetResult{}, fmt.Errorf("open worktree root: %w", err)
		}
		defer root.Close()
		if _, err := root.Stat(targetPath); err != nil && !os.IsNotExist(err) {
			return sourceEditorTargetResult{}, errors.New("file path escapes worktree")
		}
		targetPath = filepath.Join(policy.Root, targetPath)
	}
	return sourceEditorTargetResult{
		fileName:             m.file.name,
		sourcePath:           targetPath,
		sourceLine:           targetLine,
		reloadAfterCleanExit: policy.ReloadAfterCleanExit,
	}, nil
}

func (m Model) hasCurrentFileLineAnnotations() bool {
	for _, a := range m.store.Get(m.file.name) {
		if a.Line > 0 {
			return true
		}
	}
	return false
}

// sourceEditorLine maps the focused diff row to the current worktree line for
// editor positioning.
//
// A false ok result means the file can still be opened without a line target,
// either because no diff row is focused or because a removed row has no nearby
// current-file anchor. An error means the focused row represents content that
// cannot be opened as a source location, such as binary, placeholder,
// collapsed context, collapsed-mode delete-only placeholders, or hidden
// removed rows.
func (m Model) sourceEditorLine() (line int, ok bool, err error) {
	dl, ok := m.cursorDiffLine()
	if !ok {
		return 0, false, nil
	}
	if dl.IsBinary || dl.IsPlaceholder {
		return 0, false, errors.New("no source line")
	}
	if dl.ChangeType == diff.ChangeRemove && m.modes.collapsed.enabled {
		hunks := m.findHunks()
		if m.isDeleteOnlyPlaceholder(m.nav.diffCursor, hunks) || m.isCollapsedHidden(m.nav.diffCursor, hunks) {
			return 0, false, errors.New("no source line")
		}
	}
	switch dl.ChangeType {
	case diff.ChangeAdd, diff.ChangeContext:
		if dl.NewNum > 0 {
			return dl.NewNum, true, nil
		}
	case diff.ChangeRemove:
		if line := m.nearestCurrentLine(); line > 0 {
			return line, true, nil
		}
		return 0, false, nil
	case diff.ChangeDivider:
		return 0, false, errors.New("skipped context")
	}
	return 0, false, nil
}

// nearestCurrentLine finds the closest line that exists in the current file.
// When previous and next rows are equally close, the previous row wins so a
// deletion between two live lines lands where the removed content was anchored.
func (m Model) nearestCurrentLine() int {
	for distance := 1; m.nav.diffCursor-distance >= 0 || m.nav.diffCursor+distance < len(m.file.lines); distance++ {
		if previous := m.nav.diffCursor - distance; previous >= 0 {
			dl := m.file.lines[previous]
			if !dl.IsBinary && !dl.IsPlaceholder && dl.ChangeType != diff.ChangeDivider && dl.ChangeType != diff.ChangeRemove {
				if dl.NewNum > 0 {
					return dl.NewNum
				}
			}
		}
		if next := m.nav.diffCursor + distance; next < len(m.file.lines) {
			dl := m.file.lines[next]
			if !dl.IsBinary && !dl.IsPlaceholder && dl.ChangeType != diff.ChangeDivider && dl.ChangeType != diff.ChangeRemove {
				if dl.NewNum > 0 {
					return dl.NewNum
				}
			}
		}
	}
	return 0
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
	var cmd tea.Cmd
	if msg.restoreMouse {
		cmd = tea.EnableMouseCellMotion
	}
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
			return m, cmd
		}
		// fall through to save: user wrote content before the error, preserve it
	}
	if msg.content == "" {
		m.cancelAnnotation()
		return m, cmd
	}
	m.saveComment(msg.content, msg.fileName, msg.fileLevel, msg.line, msg.changeType)
	return m, cmd
}

func (m Model) handleSourceEditorFinished(msg sourceEditorFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Printf("[WARN] source editor session error: %v", msg.err)
		m.editorState.hint = "Editor failed"
		return m, nil
	}
	m.editorState.hint = "Returned from editor"
	if !msg.reloadAfterCleanExit {
		return m, nil
	}
	// Cross-file hunk navigation can advance the tree selection before the
	// selected file load finishes. In that window, the editor returned for a
	// stale displayed file, so leave the queued load in control.
	if msg.fileName != m.tree.SelectedFile() {
		m.nav.pendingHunkJump = nil
		return m, nil
	}
	if msg.fileName != m.file.name {
		return m, nil
	}
	cmd := m.reloadCurrentFile()
	return m, cmd
}
