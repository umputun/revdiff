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
	"github.com/umputun/revdiff/app/ui/overlay"
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
			assert.Equal(t, tc.path, m.OutputPath())
			assert.Empty(t, m.output.hint)
			assert.False(t, m.OutputCheckpointed())
		})
	}
}

func TestModel_HandleFlushOutput_EmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "No annotations to flush", model.output.hint)
	assert.Nil(t, cmd)
	assert.NoFileExists(t, path, "empty store must not create the output file")
}

func TestModel_HandleFlushOutput_Modes(t *testing.T) {
	tests := []struct {
		name       string
		withPath   bool
		withHook   bool
		wantHint   string
		wantCmd    bool
		wantOutput bool
		wantPrompt bool
	}{
		{name: "neither", wantPrompt: true},
		{name: "output only", withPath: true, wantHint: "Wrote 1 annotation to output file", wantOutput: true},
		{name: "hook only", withHook: true, wantHint: "Running post-flush command with 1 annotation", wantCmd: true},
		{name: "output and hook", withPath: true, withHook: true, wantHint: "Wrote 1 annotation to output file; running post-flush command", wantCmd: true, wantOutput: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := annotation.NewStore()
			store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
			path := filepath.Join(t.TempDir(), "out.md")
			cfg := ModelConfig{}
			if tc.withPath {
				cfg.OutputPath = path
			}
			var hook *postFlushHookStub
			if tc.withHook {
				hook = &postFlushHookStub{}
				cfg.PostFlushHook = hook
			}
			m := testNewModel(t, plainRenderer(), store, noopHighlighter(), cfg)

			result, cmd := m.handleFlushOutput()
			model := result.(Model)
			assert.Equal(t, tc.wantHint, model.output.hint)
			assert.Equal(t, tc.wantPrompt, model.overlay.Kind() == overlay.KindOutputPath)
			if tc.wantCmd {
				require.NotNil(t, cmd)
			} else {
				assert.Nil(t, cmd)
			}
			if tc.wantOutput {
				assert.FileExists(t, path)
				assert.True(t, model.OutputCheckpointed())
			} else {
				assert.NoFileExists(t, path)
			}
			if tc.withHook {
				assert.Equal(t, store.FormatOutput(), hook.content)
			}
		})
	}
}

func TestModel_HandleOutputPathChoice(t *testing.T) {
	newStore := func() *annotation.Store {
		store := annotation.NewStore()
		store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
		return store
	}

	t.Run("writes new path and remembers it", func(t *testing.T) {
		store := newStore()
		path := filepath.Join(t.TempDir(), "review.md")
		m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{})

		result, cmd := m.handleOutputPathChoice(&overlay.OutputPathChoice{Path: path})
		model := result.(Model)

		assert.Nil(t, cmd)
		assert.FileExists(t, path)
		assert.Equal(t, path, model.OutputPath())
		assert.True(t, model.OutputCheckpointed())
		assert.Equal(t, "Wrote 1 annotation to output file", model.output.hint)
	})

	t.Run("existing path asks before overwrite", func(t *testing.T) {
		store := newStore()
		path := filepath.Join(t.TempDir(), "review.md")
		require.NoError(t, os.WriteFile(path, []byte("keep me"), 0o600))
		m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{})

		result, _ := m.handleOutputPathChoice(&overlay.OutputPathChoice{Path: path})
		model := result.(Model)

		assert.Equal(t, overlay.KindOutputPath, model.overlay.Kind())
		assert.Empty(t, model.OutputPath())
		assert.False(t, model.OutputCheckpointed())
		got, err := os.ReadFile(path) //nolint:gosec // test-owned path
		require.NoError(t, err)
		assert.Equal(t, "keep me", string(got))
	})

	t.Run("confirmed overwrite activates path", func(t *testing.T) {
		store := newStore()
		path := filepath.Join(t.TempDir(), "review.md")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
		m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{})

		result, _ := m.handleOutputPathChoice(&overlay.OutputPathChoice{Path: path, Overwrite: true})
		model := result.(Model)

		assert.Equal(t, path, model.OutputPath())
		assert.True(t, model.OutputCheckpointed())
		got, err := os.ReadFile(path) //nolint:gosec // test-owned path
		require.NoError(t, err)
		assert.Equal(t, store.FormatOutput(), string(got))
	})

	t.Run("write error keeps prompt open", func(t *testing.T) {
		store := newStore()
		dir := t.TempDir()
		blocker := filepath.Join(dir, "not-a-directory")
		require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0o600))
		path := filepath.Join(blocker, "review.md")
		m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{})

		result, _ := m.handleOutputPathChoice(&overlay.OutputPathChoice{Path: path})
		model := result.(Model)

		assert.Equal(t, overlay.KindOutputPath, model.overlay.Kind())
		assert.Empty(t, model.OutputPath())
		assert.False(t, model.OutputCheckpointed())
		assert.NoFileExists(t, path)
	})

	t.Run("later flush reuses selected path", func(t *testing.T) {
		store := newStore()
		path := filepath.Join(t.TempDir(), "review.md")
		m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{})
		result, _ := m.handleOutputPathChoice(&overlay.OutputPathChoice{Path: path})
		m = result.(Model)
		store.Add(annotation.Annotation{File: "b.go", Line: 2, Type: "-", Comment: "second"})

		result, cmd := m.handleFlushOutput()
		model := result.(Model)

		assert.Nil(t, cmd)
		assert.False(t, model.overlay.Active())
		got, err := os.ReadFile(path) //nolint:gosec // test-owned path
		require.NoError(t, err)
		assert.Equal(t, store.FormatOutput(), string(got))
	})
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
		tests := []struct {
			name        string
			trackingOn  bool
			toggle      bool
			wantRestore bool
		}{
			{name: "tracking on, untouched", trackingOn: true, wantRestore: true},
			{name: "tracking on, toggled off", trackingOn: true, toggle: true, wantRestore: false},
			{name: "no-mouse start, toggled on", toggle: true, wantRestore: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				m := testModel([]string{"a.go"}, nil)
				m.mouseTracking = tt.trackingOn
				if tt.toggle {
					toggled, cmd := m.handleMouseToggle()
					require.NotNil(t, cmd)
					_ = cmd()
					m = toggled.(Model)
				}

				result, cmd := m.handlePostFlushFinished(postFlushFinishedMsg{
					successHint: "Wrote 2 annotations to output file and ran post-flush command",
					failureHint: "Wrote 2 annotations to output file; post-flush command failed",
				})
				model := result.(Model)
				assert.Equal(t, "Wrote 2 annotations to output file and ran post-flush command", model.output.hint)
				if tt.wantRestore {
					require.NotNil(t, cmd)
					assert.IsType(t, tea.EnableMouseCellMotion(), cmd())
				} else {
					assert.Nil(t, cmd)
				}
			})
		}
	})

	t.Run("failure", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		result, cmd := m.handlePostFlushFinished(postFlushFinishedMsg{
			err:         errors.New("exit status 1"),
			successHint: "Wrote 1 annotation to output file and ran post-flush command",
			failureHint: "Wrote 1 annotation to output file; post-flush command failed",
		})
		model := result.(Model)
		assert.Equal(t, "Wrote 1 annotation to output file; post-flush command failed", model.output.hint)
		assert.Nil(t, cmd)
	})

	t.Run("hook only", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		result, cmd := m.handlePostFlushFinished(postFlushFinishedMsg{
			successHint: "Ran post-flush command with 1 annotation",
			failureHint: "Post-flush command failed",
		})
		model := result.(Model)
		assert.Equal(t, "Ran post-flush command with 1 annotation", model.output.hint)
		assert.Nil(t, cmd)
	})

	t.Run("hook only failure", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		result, cmd := m.handlePostFlushFinished(postFlushFinishedMsg{
			err:         errors.New("exit status 1"),
			successHint: "Ran post-flush command with 1 annotation",
			failureHint: "Post-flush command failed",
		})
		model := result.(Model)
		assert.Equal(t, "Post-flush command failed", model.output.hint)
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
