package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/ui/style"
)

func outputPathRender(mgr *Manager) string {
	base := strings.Repeat(strings.Repeat(" ", 100)+"\n", 30)
	return mgr.Compose(base, RenderCtx{Width: 100, Height: 30, Resolver: style.PlainResolver()})
}

func TestOutputPathOverlay_Submit(t *testing.T) {
	mgr := NewManager()
	mgr.OpenOutputPath(OutputPathSpec{})
	assert.Contains(t, outputPathRender(mgr), "Save Annotations")

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("review.md")}, "")
	assert.Equal(t, OutcomeNone, out.Kind)
	out = mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")

	require.Equal(t, OutcomeOutputPathSubmitted, out.Kind)
	require.NotNil(t, out.OutputPathChoice)
	assert.Equal(t, "review.md", out.OutputPathChoice.Path)
	assert.False(t, out.OutputPathChoice.Overwrite)
	assert.False(t, mgr.Active(), "submitting a path closes the overlay")
}

func TestOutputPathOverlay_EmptyPathStaysOpen(t *testing.T) {
	mgr := NewManager()
	mgr.OpenOutputPath(OutputPathSpec{})

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.True(t, mgr.Active())
	assert.Contains(t, outputPathRender(mgr), "Output path is required")
}

func TestOutputPathOverlay_Cancel(t *testing.T) {
	mgr := NewManager()
	mgr.OpenOutputPath(OutputPathSpec{Path: "review.md"})

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, "")
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestOutputPathOverlay_OverwriteConfirmation(t *testing.T) {
	t.Run("yes submits overwrite", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenOutputPath(OutputPathSpec{Path: "review.md", ConfirmOverwrite: true})

		assert.Contains(t, outputPathRender(mgr), "Overwrite review.md?")
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, "")

		require.Equal(t, OutcomeOutputPathSubmitted, out.Kind)
		require.NotNil(t, out.OutputPathChoice)
		assert.Equal(t, "review.md", out.OutputPathChoice.Path)
		assert.True(t, out.OutputPathChoice.Overwrite)
		assert.False(t, mgr.Active())
	})

	t.Run("no returns to editing", func(t *testing.T) {
		mgr := NewManager()
		mgr.OpenOutputPath(OutputPathSpec{Path: "review.md", ConfirmOverwrite: true})

		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, "")
		assert.Equal(t, OutcomeNone, out.Kind)
		assert.True(t, mgr.Active())
		view := outputPathRender(mgr)
		assert.Contains(t, view, "Path:")
		assert.Contains(t, view, "review.md")
		assert.NotContains(t, view, "Overwrite review.md?")
	})
}

func TestOutputPathOverlay_ErrorPreservesPath(t *testing.T) {
	mgr := NewManager()
	mgr.OpenOutputPath(OutputPathSpec{Path: "missing/review.md", Error: "Save failed: missing directory"})

	view := outputPathRender(mgr)
	assert.Contains(t, view, "missing/review.md")
	assert.Contains(t, view, "Save failed: missing directory")
}
