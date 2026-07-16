package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/ui/mocks"
)

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

func TestModel_HandleFlushOutput_NoSink(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{OutputPath: ""})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "Output flush requires -o/--output or --agent-cmd", model.output.hint)
	assert.Nil(t, cmd)
}

// fakeAgentSink records the payload passed to Send and returns a configured error.
type fakeAgentSink struct {
	called  bool
	payload string
	err     error
}

func (f *fakeAgentSink) Send(payload string) error {
	f.called = true
	f.payload = payload
	return f.err
}

func TestNewModel_AgentNilByDefault(t *testing.T) {
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{})
	assert.Nil(t, m.agent, "no --agent-cmd means the sink is disabled")
}

func TestNewModel_AgentTypedNilCollapsed(t *testing.T) {
	var sink *fakeAgentSink // typed-nil pointer wrapped in the interface
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{Agent: sink})
	assert.Nil(t, m.agent, "a typed-nil sink must collapse to interface-nil so guards see it disabled")
}

func TestModel_HandleFlushOutput_AgentOnly(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	sink := &fakeAgentSink{}
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{Agent: sink})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "Sending 1 annotation to agent…", model.output.hint)
	require.NotNil(t, cmd, "agent flush must return an async command")

	msg, ok := cmd().(agentFlushedMsg)
	require.True(t, ok)
	assert.True(t, sink.called)
	assert.Equal(t, store.FormatOutput(), sink.payload, "the exact FormatOutput must be piped to the agent")
	require.NoError(t, msg.err)

	done, _ := model.handleAgentFlushed(msg)
	assert.Equal(t, "Sent 1 annotation to agent", done.(Model).output.hint)
	assert.Equal(t, 1, store.Count(), "agent flush must not mutate the store")
}

func TestModel_HandleFlushOutput_FileAndAgent(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	store.Add(annotation.Annotation{File: "b.go", Line: 2, Type: " ", Comment: "check"})
	path := filepath.Join(t.TempDir(), "out.md")
	sink := &fakeAgentSink{}
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{OutputPath: path, Agent: sink})

	result, cmd := m.handleFlushOutput()
	model := result.(Model)
	assert.Equal(t, "Wrote 2 annotations; sending to agent…", model.output.hint)
	require.NotNil(t, cmd)
	assert.FileExists(t, path, "file sink must be written even when an agent sink is present")

	msg, ok := cmd().(agentFlushedMsg)
	require.True(t, ok)
	assert.True(t, sink.called)
	require.NoError(t, msg.err)
}

func TestModel_HandleFlushOutput_AgentError(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	sink := &fakeAgentSink{err: assert.AnError}
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(), ModelConfig{Agent: sink})

	_, cmd := m.handleFlushOutput()
	require.NotNil(t, cmd)
	msg, ok := cmd().(agentFlushedMsg)
	require.True(t, ok)
	require.Error(t, msg.err)

	done, _ := m.handleAgentFlushed(msg)
	assert.Equal(t, "Agent flush failed", done.(Model).output.hint)
}

func TestModel_HandleFlushOutput_EmptyStoreWithAgent(t *testing.T) {
	sink := &fakeAgentSink{}
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{Agent: sink})

	result, cmd := m.handleFlushOutput()
	assert.Equal(t, "No annotations to flush", result.(Model).output.hint)
	assert.Nil(t, cmd)
	assert.False(t, sink.called, "empty store must not invoke the agent")
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
