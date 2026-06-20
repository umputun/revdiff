package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/editor"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

// mockEditor returns a configured ExternalEditorMock whose Command returns a
// no-op exec.Cmd and a complete func that reports the given content. cmdErr
// short-circuits Command (returns nil cmd + cmdErr); the complete closure
// preserves any runErr the caller passes.
func mockEditor(content string, cmdErr error) *mocks.ExternalEditorMock {
	return &mocks.ExternalEditorMock{
		CommandFunc: func(string) (*exec.Cmd, func(error) (string, error), error) {
			if cmdErr != nil {
				return nil, nil, cmdErr
			}
			cmd := exec.Command("/bin/true")
			complete := func(runErr error) (string, error) {
				return content, runErr
			}
			return cmd, complete, nil
		},
		SourceCommandFunc: func(editor.SourceTarget) (*exec.Cmd, error) {
			return exec.Command("/bin/true"), nil
		},
	}
}

func mockSourceEditor(err error) *mocks.ExternalEditorMock {
	return &mocks.ExternalEditorMock{
		CommandFunc: func(string) (*exec.Cmd, func(error) (string, error), error) {
			return exec.Command("/bin/true"), func(error) (string, error) { return "", nil }, nil
		},
		SourceCommandFunc: func(editor.SourceTarget) (*exec.Cmd, error) {
			if err != nil {
				return nil, err
			}
			return exec.Command("/bin/true"), nil
		},
	}
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

	fake := mockEditor("", nil)
	m.editor = fake

	m.startAnnotation()
	m.annot.input.SetValue("pre-edit content")

	cmd := m.openEditor()
	require.NotNil(t, cmd, "openEditor should return a non-nil tea.Cmd")
	require.Len(t, fake.CommandCalls(), 1)
	assert.Equal(t, "pre-edit content", fake.CommandCalls()[0].Content, "editor should receive the current input value")
}

func TestOpenEditor_FileLevelCapturesTarget(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"

	fake := mockEditor("", nil)
	m.editor = fake

	cmd := m.startFileAnnotation()
	require.NotNil(t, cmd)
	m.annot.input.SetValue("file-level seed")

	editorCmd := m.openEditor()
	require.NotNil(t, editorCmd)
	require.Len(t, fake.CommandCalls(), 1, "editor.Command must be invoked exactly once on file-level path")
	assert.Equal(t, "file-level seed", fake.CommandCalls()[0].Content, "editor must receive the current input value")
}

func TestOpenEditor_LineLevelReturnsNilWhenNoCursorLine(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	// no lines loaded and diffCursor default 0 -> cursorDiffLine returns false

	fake := mockEditor("", nil)
	m.editor = fake
	m.annot.annotating = true
	m.annot.fileAnnotating = false

	assert.Nil(t, m.openEditor(), "line-level openEditor must return nil when cursor has no diff line")
	assert.Empty(t, fake.CommandCalls(), "editor.Command must NOT be invoked when no cursor line exists")
	assert.True(t, m.annot.annotating, "annotating must remain true so the user can Esc out normally")
	assert.False(t, m.annot.fileAnnotating, "fileAnnotating state must not be flipped by this no-op path")
}

func TestIsNilValue(t *testing.T) {
	var typedNil *mocks.ExternalEditorMock
	var untypedNil ExternalEditor
	assert.True(t, isNilValue(typedNil), "typed-nil pointer must be detected")
	assert.False(t, isNilValue(untypedNil), "untyped-nil interface is handled by the != nil check, not isNilValue")
	assert.False(t, isNilValue(42), "non-pointer value is not nil")
	assert.False(t, isNilValue(&mocks.ExternalEditorMock{}), "non-nil pointer is not nil")
}

func TestNewModel_TypedNilEditorDefaultsToConcrete(t *testing.T) {
	// passing (*mocks.ExternalEditorMock)(nil) as Editor would normally bypass a
	// plain `cfg.Editor == nil` check and panic on later use. Verify the
	// isNilValue guard in NewModel replaces it with the default editor.Editor{}.
	var typedNil *mocks.ExternalEditorMock
	res := style.PlainResolver()
	m, err := NewModel(ModelConfig{
		Renderer:       &mocks.RendererMock{},
		Store:          annotation.NewStore(),
		Highlighter:    noopHighlighter(),
		StyleResolver:  res,
		StyleRenderer:  style.NewRenderer(res),
		SGR:            style.SGR{},
		WordDiffer:     worddiff.New(),
		Overlay:        overlay.NewManager(),
		Themes:         fakeThemeCatalog{},
		TreeWidthRatio: 3,
		NewFileTree:    testFileTreeFactory(),
		ParseTOC:       testParseTOCFactory(),
		Editor:         typedNil,
	})
	require.NoError(t, err)
	require.NotNil(t, m.editor, "editor must not be a typed-nil interface")
	// prove Command is callable (would panic on typed-nil route)
	t.Setenv("EDITOR", "/bin/true")
	cmd, complete, err := m.editor.Command("seed")
	require.NoError(t, err)
	require.NotNil(t, cmd)
	_, _ = complete(nil)
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
	m.editor = mockEditor("", cmdErr)
	m.startAnnotation()
	m.annot.input.SetValue("in-progress note")

	cmd := m.openEditor()
	require.NotNil(t, cmd)
	msg := cmd()
	finished, ok := msg.(editorFinishedMsg)
	require.True(t, ok, "msg should be editorFinishedMsg, got %T", msg)
	assert.Equal(t, cmdErr, finished.err)
	assert.False(t, finished.fileLevel)
	assert.Equal(t, 1, finished.line)
	assert.Equal(t, "+", finished.changeType)

	// round-trip the msg through Update so handleEditorFinished executes and we
	// verify its contract: error path keeps annotation mode open with input intact.
	result, _ := m.Update(finished)
	model := result.(Model)
	assert.True(t, model.annot.annotating, "annotation mode must stay open after editor error so user can retry")
	assert.Equal(t, "in-progress note", model.annot.input.Value(), "input value must be preserved after error")
	assert.Empty(t, model.store.Get("a.go"), "no annotation should be saved when editor errored")
}

func TestSourceEditorTarget_LineResolution(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("one\ntwo\nthree\nfour\n"), 0o600))
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
		{OldNum: 2, NewNum: 2, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 3, NewNum: 3, Content: "added", ChangeType: diff.ChangeAdd},
		{OldNum: 4, NewNum: 0, Content: "removed tail", ChangeType: diff.ChangeRemove},
		{OldNum: 5, NewNum: 5, Content: "later context", ChangeType: diff.ChangeContext},
	}
	tests := []struct {
		name     string
		cursor   int
		wantLine int
	}{
		{"remove uses next current line", 0, 2},
		{"context uses new line", 1, 2},
		{"add uses new line", 2, 3},
		{"remove with equal previous and next distance uses previous current line", 3, 3},
		{"file annotation opens file without line", -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
			m.cfg.workDir = workDir
			m.tree = testNewFileTree([]string{"a.go"})
			m.file.name = "a.go"
			m.file.lines = lines
			m.nav.diffCursor = tt.cursor

			got, err := m.sourceEditorTarget()

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(workDir, "a.go"), got.Target.Path)
			assert.Equal(t, tt.wantLine, got.Target.Line)
		})
	}
}

func TestSourceEditorTarget_RemoveOnlyFileOpensWithoutLine(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("remaining\n"), 0o600))
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
		{OldNum: 2, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	got, err := m.sourceEditorTarget()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workDir, "a.go"), got.Target.Path)
	assert.Equal(t, 0, got.Target.Line)
}

func TestSourceEditorTarget_CollapsedDeleteOnlyPlaceholderHasNoSourceLine(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("ctx1\nctx2\n"), 0o600))
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
		{OldNum: 3, NewNum: 0, Content: "removed too", ChangeType: diff.ChangeRemove},
		{OldNum: 4, NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	m.nav.diffCursor = 1

	got, err := m.sourceEditorTarget()

	assert.Equal(t, editor.SourceTarget{}, got.Target)
	assert.EqualError(t, err, "no source line")
}

func TestSourceEditorTarget_CollapsedHiddenRemovedLineHasNoSourceLine(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("ctx1\nctx2\n"), 0o600))
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
		{OldNum: 3, NewNum: 0, Content: "hidden removed", ChangeType: diff.ChangeRemove},
		{OldNum: 4, NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	m.nav.diffCursor = 2

	got, err := m.sourceEditorTarget()

	assert.Equal(t, editor.SourceTarget{}, got.Target)
	assert.EqualError(t, err, "no source line")
}

func TestSourceEditorTarget_ExpandedDeleteOnlyLineUsesNearestCurrentLine(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("ctx1\nctx2\n"), 0o600))
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
		{OldNum: 3, NewNum: 0, Content: "removed too", ChangeType: diff.ChangeRemove},
		{OldNum: 4, NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = map[int]bool{1: true}
	m.nav.diffCursor = 1

	got, err := m.sourceEditorTarget()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workDir, "a.go"), got.Target.Path)
	assert.Equal(t, 1, got.Target.Line)
}

func TestSourceEditorTarget_AbsoluteFilePathUsesOriginalPath(t *testing.T) {
	workDir := t.TempDir()
	standaloneDir := t.TempDir()
	standaloneFile := filepath.Join(standaloneDir, "standalone.md")
	require.NoError(t, os.WriteFile(standaloneFile, []byte("one\n"), 0o600))
	lines := []diff.DiffLine{{NewNum: 1, Content: "one", ChangeType: diff.ChangeContext}}
	m := testModel([]string{standaloneFile}, map[string][]diff.DiffLine{standaloneFile: lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{standaloneFile})
	m.file.name = standaloneFile
	m.file.lines = lines
	m.nav.diffCursor = 0

	got, err := m.sourceEditorTarget()

	require.NoError(t, err)
	assert.Equal(t, standaloneFile, got.Target.Path)
	assert.Equal(t, 1, got.Target.Line)
}

func TestSourceEditorTarget_RelativeSymlinkEscapeRejected(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.go")
	require.NoError(t, os.WriteFile(outsideFile, []byte("one\n"), 0o600))
	require.NoError(t, os.Symlink(outsideFile, filepath.Join(workDir, "escape.go")))
	lines := []diff.DiffLine{{NewNum: 1, Content: "one", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"escape.go"}, map[string][]diff.DiffLine{"escape.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"escape.go"})
	m.file.name = "escape.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	got, err := m.sourceEditorTarget()

	assert.Equal(t, editor.SourceTarget{}, got.Target)
	assert.EqualError(t, err, "file path escapes worktree")
}

func TestSourceEditorTarget_StagedReviewOpensWithFocusedLine(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("worktree\ncontents\n"), 0o600))
	lines := []diff.DiffLine{{OldNum: 0, NewNum: 2, Content: "staged", ChangeType: diff.ChangeAdd}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.cfg.staged = true
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	got, err := m.sourceEditorTarget()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workDir, "a.go"), got.Target.Path)
	assert.Equal(t, 2, got.Target.Line)
	assert.Equal(t, sourceEditorRefreshNever, got.RefreshPolicy)
}

func TestSourceEditorTarget_RefReviewOpensWithFocusedLine(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("worktree\ncontents\n"), 0o600))
	lines := []diff.DiffLine{{OldNum: 1, NewNum: 2, Content: "range", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.cfg.ref = "HEAD~1"
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	got, err := m.sourceEditorTarget()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workDir, "a.go"), got.Target.Path)
	assert.Equal(t, 2, got.Target.Line)
	assert.Equal(t, sourceEditorRefreshNever, got.RefreshPolicy)
}

func TestSourceEditorTarget_StagedReviewStillRejectsRowsWithoutSource(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("worktree\ncontents\n"), 0o600))
	lines := []diff.DiffLine{{NewNum: 1, Content: "(binary file)", ChangeType: diff.ChangeContext, IsBinary: true}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.cfg.staged = true
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	got, err := m.sourceEditorTarget()

	assert.Equal(t, editor.SourceTarget{}, got.Target)
	assert.EqualError(t, err, "no source line")
}

func TestSourceEditorTarget_SelectionErrorCases(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("one\n"), 0o600))
	tests := []struct {
		name    string
		workDir string
		file    string
		tree    FileTreeComponent
		lines   []diff.DiffLine
		cursor  int
		wantErr string
	}{
		{
			name:    "no worktree",
			workDir: "",
			file:    "a.go",
			tree:    testNewFileTree([]string{"a.go"}),
			lines:   []diff.DiffLine{{NewNum: 1, ChangeType: diff.ChangeContext}},
			cursor:  0,
			wantErr: "no worktree",
		},
		{
			name:    "deleted file",
			workDir: workDir,
			file:    "a.go",
			tree:    sidepane.NewFileTree([]diff.FileEntry{{Path: "a.go", Status: diff.FileDeleted}}),
			lines:   []diff.DiffLine{{OldNum: 1, ChangeType: diff.ChangeRemove}},
			cursor:  0,
			wantErr: "file was deleted",
		},
		{
			name:    "divider",
			workDir: workDir,
			file:    "a.go",
			tree:    testNewFileTree([]string{"a.go"}),
			lines:   []diff.DiffLine{{ChangeType: diff.ChangeDivider}},
			cursor:  0,
			wantErr: "skipped context",
		},
		{
			name:    "binary",
			workDir: workDir,
			file:    "a.go",
			tree:    testNewFileTree([]string{"a.go"}),
			lines:   []diff.DiffLine{{ChangeType: diff.ChangeContext, IsBinary: true}},
			cursor:  0,
			wantErr: "no source line",
		},
		{
			name:    "placeholder",
			workDir: workDir,
			file:    "a.go",
			tree:    testNewFileTree([]string{"a.go"}),
			lines:   []diff.DiffLine{{ChangeType: diff.ChangeContext, IsPlaceholder: true}},
			cursor:  0,
			wantErr: "no source line",
		},
		{
			name:    "relative path escapes worktree",
			workDir: workDir,
			file:    "a/../../../outside.go",
			tree:    testNewFileTree([]string{"a/../../../outside.go"}),
			lines:   []diff.DiffLine{{NewNum: 1, ChangeType: diff.ChangeContext}},
			cursor:  0,
			wantErr: "file path escapes worktree",
		},
		{
			name:    "no loaded file",
			workDir: workDir,
			file:    "",
			tree:    testNewFileTree([]string{"a.go"}),
			lines:   nil,
			cursor:  0,
			wantErr: "no file loaded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.cfg.workDir = tt.workDir
			m.tree = tt.tree
			m.file.name = tt.file
			m.file.lines = tt.lines
			m.nav.diffCursor = tt.cursor

			got, err := m.sourceEditorTarget()

			assert.Equal(t, editor.SourceTarget{}, got.Target)
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestOpenSourceEditor_SourceValidationErrorKeepsHint(t *testing.T) {
	workDir := t.TempDir()
	lines := []diff.DiffLine{{NewNum: 1, Content: "one", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"missing.go"}, map[string][]diff.DiffLine{"missing.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"missing.go"})
	m.layout.focus = paneDiff
	m.file.name = "missing.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.editor = mockSourceEditor(editor.ErrSourceMissing)

	cmd := m.openSourceEditor()

	assert.Nil(t, cmd)
	assert.Equal(t, "Editor unavailable: file is missing", m.editorState.hint)
}

func TestOpenSourceEditor_SourceNotRegularErrorKeepsHint(t *testing.T) {
	workDir := t.TempDir()
	lines := []diff.DiffLine{{NewNum: 1, Content: "one", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"dir"}, map[string][]diff.DiffLine{"dir": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"dir"})
	m.layout.focus = paneDiff
	m.file.name = "dir"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.editor = mockSourceEditor(editor.ErrSourceNotRegular)

	cmd := m.openSourceEditor()

	assert.Nil(t, cmd)
	assert.Equal(t, "Editor unavailable: file is not regular", m.editorState.hint)
}

func TestHandleSourceEditorFinished_RefreshPolicy(t *testing.T) {
	tests := []struct {
		name       string
		policy     sourceEditorRefreshPolicy
		wantReload bool
	}{
		{name: "worktree clean exit reloads displayed file", policy: sourceEditorRefreshWorktree, wantReload: true},
		{name: "staged or ref clean exit skips reload", policy: sourceEditorRefreshNever, wantReload: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.file.name = "a.go"
			beforeSeq := m.file.loadSeq

			result, cmd := m.handleSourceEditorFinished(sourceEditorFinishedMsg{refreshPolicy: tt.policy})
			model := result.(Model)

			assert.Equal(t, "Returned from editor", model.editorState.hint)
			if tt.wantReload {
				require.NotNil(t, cmd)
				assert.Greater(t, model.file.loadSeq, beforeSeq)
			} else {
				assert.Nil(t, cmd)
				assert.Equal(t, beforeSeq, model.file.loadSeq)
			}
		})
	}
}

func TestHandleDiffAction_OpenFileInEditor(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "a.go"), []byte("one\ntwo\n"), 0o600))
	lines := []diff.DiffLine{{NewNum: 2, Content: "two", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = workDir
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	fake := mockSourceEditor(nil)
	m.editor = fake

	model, cmd := m.handleDiffAction(keymap.ActionOpenFileInEditor)

	require.NotNil(t, cmd)
	assert.IsType(t, Model{}, model)
	require.Len(t, fake.SourceCommandCalls(), 1)
	assert.Equal(t, editor.SourceTarget{Path: filepath.Join(workDir, "a.go"), Line: 2}, fake.SourceCommandCalls()[0].Target)
}

func TestHandleDiffAction_OpenFileInEditorNoopKeepsHint(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "one", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.cfg.workDir = ""
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	fake := mockSourceEditor(nil)
	m.editor = fake

	result, cmd := m.handleDiffAction(keymap.ActionOpenFileInEditor)
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Empty(t, fake.SourceCommandCalls())
	assert.Equal(t, "Editor unavailable: no worktree", model.editorState.hint)
}
