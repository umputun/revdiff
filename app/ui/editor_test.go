package ui

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

// fakeExternalEditor captures the content passed to Command and returns a
// configurable Cmd, complete function, and error. Used to drive openEditor
// tests without spawning a real editor.
type fakeExternalEditor struct {
	seenContent    string
	commandCallCnt int
	cmd            *exec.Cmd
	content        string
	completeErr    error
	commandErr     error
}

func (f *fakeExternalEditor) Command(content string) (*exec.Cmd, func(error) (string, error), error) {
	f.seenContent = content
	f.commandCallCnt++
	if f.commandErr != nil {
		return nil, nil, f.commandErr
	}
	cmd := f.cmd
	if cmd == nil {
		cmd = exec.Command("/bin/true")
	}
	complete := func(runErr error) (string, error) {
		if runErr != nil {
			return f.content, runErr
		}
		return f.content, f.completeErr
	}
	return cmd, complete, nil
}

func TestOpenEditor_LineLevelCapturesTargetAndSeedsContent(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1

	fake := &fakeExternalEditor{}
	m.editor = fake

	m.startAnnotation()
	m.annot.input.SetValue("pre-edit content")

	cmd := m.openEditor()
	require.NotNil(t, cmd, "openEditor should return a non-nil tea.Cmd")
	assert.Equal(t, 1, fake.commandCallCnt)
	assert.Equal(t, "pre-edit content", fake.seenContent, "editor should receive the current input value")
}

func TestOpenEditor_FileLevelCapturesTarget(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"

	fake := &fakeExternalEditor{}
	m.editor = fake

	cmd := m.startFileAnnotation()
	require.NotNil(t, cmd)
	m.annot.input.SetValue("file-level seed")

	editorCmd := m.openEditor()
	require.NotNil(t, editorCmd)
	assert.Equal(t, "file-level seed", fake.seenContent)
}

func TestOpenEditor_LineLevelReturnsNilWhenNoCursorLine(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	// no lines loaded and diffCursor default 0 -> cursorDiffLine returns false

	m.editor = &fakeExternalEditor{}
	m.annot.annotating = true
	m.annot.fileAnnotating = false

	assert.Nil(t, m.openEditor(), "line-level openEditor must return nil when cursor has no diff line")
}

func TestOpenEditor_CommandErrorProducesEditorFinishedMsg(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeAdd}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	cmdErr := errors.New("temp file unavailable")
	m.editor = &fakeExternalEditor{commandErr: cmdErr}
	m.startAnnotation()

	cmd := m.openEditor()
	require.NotNil(t, cmd)
	msg := cmd()
	finished, ok := msg.(editorFinishedMsg)
	require.True(t, ok, "msg should be editorFinishedMsg, got %T", msg)
	assert.Equal(t, cmdErr, finished.err)
	assert.False(t, finished.fileLevel)
	assert.Equal(t, 1, finished.line)
	assert.Equal(t, "+", finished.changeType)
}
