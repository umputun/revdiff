package overlay

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

func filePickerSpec() FilePickerSpec {
	return FilePickerSpec{
		Paths:      []string{"README.md", "app/ui/model.go", "app/ui/view.go", "docs/ARCHITECTURE.md"},
		ActivePath: "app/ui/model.go",
	}
}

func filePickerRenderCtx(height int) RenderCtx {
	return RenderCtx{Width: 100, Height: height, Resolver: style.PlainResolver()}
}

func TestFilePickerOpenHighlightsCurrentFileAndCopiesInput(t *testing.T) {
	spec := filePickerSpec()
	mgr := NewManager()
	mgr.OpenFilePicker(spec)

	assert.Equal(t, KindFilePicker, mgr.Kind())
	assert.Equal(t, 1, mgr.filePick.cursor)
	spec.Paths[1] = "mutated"
	assert.Equal(t, "app/ui/model.go", mgr.filePick.entries[1])
}

func TestFilePickerFilterFullPathCaseInsensitive(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())

	for _, r := range "UI/VIEW" {
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, "")
		assert.Equal(t, OutcomeNone, out.Kind)
	}
	assert.Equal(t, []string{"app/ui/view.go"}, mgr.filePick.entries)
	assert.Equal(t, 0, mgr.filePick.cursor)

	rendered := mgr.filePick.render(filePickerRenderCtx(30), mgr)
	assert.Contains(t, rendered, "files (1/4)")
	assert.Contains(t, rendered, "app/ui/view.go")
}

func TestFilePickerKeyboardNavigationUsesConfiguredActions(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, keymap.ActionDown)
	assert.Equal(t, 2, mgr.filePick.cursor)
	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, keymap.ActionUp)
	assert.Equal(t, 1, mgr.filePick.cursor)
}

func TestFilePickerEnterSelectsAndCloses(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())
	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, keymap.ActionDown)

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, keymap.ActionConfirm)
	require.Equal(t, OutcomeFileChosen, out.Kind)
	require.NotNil(t, out.FileChoice)
	assert.Equal(t, "app/ui/view.go", out.FileChoice.Path)
	assert.False(t, mgr.Active())
}

func TestFilePickerEmptyResultsStayOpenOnEnter(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())
	mgr.filePick.filter = "missing"
	mgr.filePick.applyFilter()

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, "")
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.True(t, mgr.Active())
	assert.Contains(t, mgr.filePick.render(filePickerRenderCtx(30), mgr), "no matches")
}

func TestFilePickerBackspaceAndEscapeBehavior(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())
	for _, r := range "模型" {
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, "")
	}

	mgr.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace}, "")
	assert.Equal(t, "模", mgr.filePick.filter)
	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeNone, out.Kind, "first Esc clears a non-empty filter")
	assert.Empty(t, mgr.filePick.filter)
	assert.Len(t, mgr.filePick.entries, 4)
	assert.True(t, mgr.Active())

	out = mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestFilePickerJumpActionCloses(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())

	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlP}, keymap.ActionJumpFile)
	assert.Equal(t, OutcomeClosed, out.Kind)
	assert.False(t, mgr.Active())
}

func TestFilePickerMouseWheelAndLeftClick(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())
	_ = mgr.filePick.render(filePickerRenderCtx(30), mgr)

	out := mgr.filePick.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.Equal(t, 2, mgr.filePick.cursor)

	out = mgr.filePick.handleMouse(tea.MouseMsg{X: 2, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	require.Equal(t, OutcomeFileChosen, out.Kind)
	assert.Equal(t, "README.md", out.FileChoice.Path)
}

func TestFilePickerMouseClickIgnoresChromeAndBlankRows(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())
	_ = mgr.filePick.render(filePickerRenderCtx(30), mgr)

	tests := []tea.MouseMsg{
		{X: 1, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{X: 2, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{X: 2, Y: 20, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
	}
	for _, msg := range tests {
		assert.Equal(t, OutcomeNone, mgr.filePick.handleMouse(msg).Kind)
	}
}

func TestFilePickerScrollingAndResizeClamp(t *testing.T) {
	paths := make([]string, 12)
	for i := range paths {
		paths[i] = fmt.Sprintf("dir/file-%02d.go", i)
	}
	mgr := NewManager()
	mgr.OpenFilePicker(FilePickerSpec{Paths: paths})
	_ = mgr.filePick.render(filePickerRenderCtx(15), mgr)
	for range 8 {
		mgr.HandleKey(tea.KeyMsg{Type: tea.KeyDown}, keymap.ActionDown)
	}
	assert.Equal(t, 8, mgr.filePick.cursor)
	assert.Positive(t, mgr.filePick.offset)

	_ = mgr.filePick.render(filePickerRenderCtx(40), mgr)
	assert.Equal(t, 0, mgr.filePick.offset, "growing the terminal should clamp stale scroll offset")
}

func TestFilePickerTruncationPreservesBasename(t *testing.T) {
	assert.Equal(t, "…/file.go", truncateFilePath("very/long/directory/file.go", 9))
	truncated := truncateFilePath("very/long/directory/exceptionally-long-file.go", 10)
	assert.True(t, strings.HasPrefix(truncated, "…"))
	assert.True(t, strings.HasSuffix(truncated, "-file.go"))
	assert.LessOrEqual(t, lipgloss.Width(truncated), 10)
}

func TestFilePickerRenderSelectedPathAndWidth(t *testing.T) {
	mgr := NewManager()
	mgr.OpenFilePicker(filePickerSpec())
	rendered := mgr.filePick.render(RenderCtx{Width: 42, Height: 20, Resolver: style.PlainResolver()}, mgr)

	assert.Contains(t, rendered, "> app/ui/model.go")
	assert.Contains(t, rendered, "files (4)")
	assert.Equal(t, 32, mgr.filePick.popupWidth)
}
