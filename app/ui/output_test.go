package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/ui/mocks"
)

type postFlushHookStub struct {
	content string
}

func (s *postFlushHookStub) Prepare(content string) *exec.Cmd {
	s.content = content
	return exec.Command("sh", "-c", "exit 0")
}

func TestNewModel_OutputPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "present", path: "/tmp/review.md"},
		{name: "empty", path: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testNewModel(t, &mocks.RendererMock{}, annotation.NewStore(), noopHighlighter(), ModelConfig{OutputPath: tc.path})
			assert.Equal(t, tc.path, m.cfg.outputPath)
			assert.Empty(t, m.output.hint)
		})
	}
}

func TestModel_HandleFlushOutput_EmptyPath(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{OutputPath: ""})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "Output flush requires -o/--output", model.output.hint)
	assert.Nil(t, cmd)
}

func TestModel_HandleFlushOutput_EmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{OutputPath: path})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "No annotations to flush", model.output.hint)
	assert.Nil(t, cmd)
	assert.NoFileExists(t, path, "empty store must not create the output file")
}

func TestModel_HandleFlushOutput_Success(t *testing.T) {
	tests := []struct {
		name     string
		anns     []annotation.Annotation
		wantHint string
	}{
		{
			name:     "single",
			anns:     []annotation.Annotation{{File: "a.go", Line: 1, Type: "+", Comment: "note"}},
			wantHint: "Wrote 1 annotation to output file",
		},
		{
			name: "multiple",
			anns: []annotation.Annotation{
				{File: "a.go", Line: 1, Type: "+", Comment: "note"},
				{File: "b.go", Line: 5, Type: " ", Comment: "check"},
			},
			wantHint: "Wrote 2 annotations to output file",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := annotation.NewStore()
			for _, a := range tc.anns {
				store.Add(a)
			}
			path := filepath.Join(t.TempDir(), "out.md")
			m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{OutputPath: path})

			result, cmd := m.handleFlushOutput()
			model := result.(Model)
			assert.Equal(t, tc.wantHint, model.output.hint)
			assert.Nil(t, cmd)

			got, err := os.ReadFile(path) //nolint:gosec // path is a t.TempDir() file
			require.NoError(t, err)
			assert.Equal(t, store.FormatOutput(), string(got), "written file must match FormatOutput")
			assert.Equal(t, len(tc.anns), store.Count(), "flush must not mutate the store")
		})
	}
}

func TestModel_HandleFlushOutput_WriteError(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	path := filepath.Join(t.TempDir(), "missing-dir", "out.md")
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{OutputPath: path})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "Flush failed", model.output.hint)
	assert.Nil(t, cmd)
	assert.NoFileExists(t, path)
}

func TestModel_HandleFlushOutput_PostFlushHook(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	path := filepath.Join(t.TempDir(), "out.md")
	hook := &postFlushHookStub{}
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{
		OutputPath:    path,
		PostFlushHook: hook,
		MouseTracking: true,
	})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	require.NotNil(t, cmd)
	assert.Equal(t, store.FormatOutput(), hook.content)
	assert.Equal(t, "Wrote 1 annotation to output file; running post-flush command", model.output.hint)
	assert.FileExists(t, path)
}

func TestModel_HandlePostFlushFinished(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		result, cmd := m.handlePostFlushFinished(postFlushFinishedMsg{
			writtenHint:  "Wrote 2 annotations to output file",
			restoreMouse: true,
		})
		model := result.(Model)
		assert.Equal(t, "Wrote 2 annotations to output file and ran post-flush command", model.output.hint)
		assert.NotNil(t, cmd)
	})

	t.Run("failure", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		result, cmd := m.handlePostFlushFinished(postFlushFinishedMsg{
			err:         errors.New("exit status 1"),
			writtenHint: "Wrote 1 annotation to output file",
		})
		model := result.(Model)
		assert.Equal(t, "Wrote 1 annotation to output file; post-flush command failed", model.output.hint)
		assert.Nil(t, cmd)
	})
}

func TestNewModel_TypedNilPostFlushHook(t *testing.T) {
	var hook *postFlushHookStub
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{PostFlushHook: hook})
	assert.Nil(t, m.postFlushHook)
}

func TestModel_ActionFlushOutput_Dispatch(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	path := filepath.Join(t.TempDir(), "out.md")
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{OutputPath: path})

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	model := result.(Model)
	assert.Equal(t, "Wrote 1 annotation to output file", model.output.hint)
	assert.FileExists(t, path, "O key must flush annotations to the output file")
}

func TestModel_OutputHint_ShownInStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.output.hint = "test output hint"
	assert.Equal(t, "test output hint", m.transientHint())
}

func TestModel_OutputHint_ClearsOnNextKey(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.output.hint = "some hint"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Empty(t, model.output.hint, "any key press must clear the output hint")
}
