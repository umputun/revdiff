package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/overlay"
)

func filePickerModel(files []string) Model {
	diffs := make(map[string][]diff.DiffLine, len(files))
	for _, file := range files {
		diffs[file] = []diff.DiffLine{{NewNum: 1, Content: file, ChangeType: diff.ChangeContext}}
	}
	m := testModel(files, diffs)
	m.tree = testNewFileTree(files)
	if len(files) > 0 {
		m.file.name = files[0]
	}
	return m
}

func TestModel_JumpFileOpensPickerAndLoadsSelection(t *testing.T) {
	m := filePickerModel([]string{"a.go", "b.go", "c.go"})
	m.layout.focus = paneTree
	m.pendingAnnotJump = &annotation.Annotation{File: "c.go", Line: 1}
	pendingHunk := true
	m.nav.pendingHunkJump = &pendingHunk

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Nil(t, cmd)
	m = result.(Model)
	assert.True(t, m.overlay.Active())
	assert.Equal(t, overlay.KindFilePicker, m.overlay.Kind())

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	result, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	assert.False(t, m.overlay.Active())
	assert.Equal(t, "b.go", m.tree.SelectedFile())
	assert.Equal(t, paneDiff, m.layout.focus)
	assert.Nil(t, m.pendingAnnotJump)
	assert.Nil(t, m.nav.pendingHunkJump)
	require.NotNil(t, cmd, "cross-file choice should use the normal async file loader")
	loaded, ok := cmd().(fileLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, "b.go", loaded.file)
}

func TestModel_JumpFileCurrentSelectionFocusesDiffWithoutReload(t *testing.T) {
	m := filePickerModel([]string{"a.go", "b.go"})
	m.layout.focus = paneTree
	m.pendingAnnotJump = &annotation.Annotation{File: "b.go", Line: 1}
	pendingHunk := false
	m.nav.pendingHunkJump = &pendingHunk
	m.openFilePicker()

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	assert.Nil(t, cmd, "choosing the loaded file should not reload it")
	assert.Equal(t, "a.go", m.tree.SelectedFile())
	assert.Equal(t, paneDiff, m.layout.focus)
	assert.Nil(t, m.pendingAnnotJump)
	assert.Nil(t, m.nav.pendingHunkJump)
}

func TestModel_JumpFilePreservesSidebarFilters(t *testing.T) {
	t.Run("annotated-only", func(t *testing.T) {
		m := filePickerModel([]string{"a.go", "b.go", "c.go"})
		m.tree.ToggleFilter(map[string]bool{"a.go": true, "b.go": true})
		require.True(t, m.tree.FilterActive())
		m.openFilePicker()

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		result, _ = result.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = result.(Model)

		assert.True(t, m.tree.FilterActive())
		assert.False(t, m.tree.UnreviewedFilterActive())
		assert.Equal(t, "b.go", m.tree.SelectedFile())
	})

	t.Run("unreviewed-only", func(t *testing.T) {
		m := filePickerModel([]string{"a.go", "b.go", "c.go"})
		m.tree.SetReviewed("c.go", "fingerprint")
		m.tree.ToggleUnreviewedFilter()
		require.True(t, m.tree.UnreviewedFilterActive())
		m.openFilePicker()

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		result, _ = result.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = result.(Model)

		assert.True(t, m.tree.UnreviewedFilterActive())
		assert.False(t, m.tree.FilterActive())
		assert.Equal(t, "b.go", m.tree.SelectedFile())
	})
}

func TestModel_JumpFileRevealsDistantPathInTree(t *testing.T) {
	files := make([]string, 60)
	for i := range files {
		files[i] = fmt.Sprintf("dir/file-%02d.go", i)
	}
	m := filePickerModel(files)
	m.openFilePicker()

	for _, r := range "59" {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = result.(Model)
	}
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	assert.Equal(t, "dir/file-59.go", m.tree.SelectedFile())
	assert.Positive(t, m.tree.ScrollState().Offset, "selected path should be scrolled into the sidebar viewport")
	assert.Equal(t, paneDiff, m.layout.focus)
	assert.NotNil(t, cmd)
}

func TestModel_JumpFileToggleClosesPicker(t *testing.T) {
	m := filePickerModel([]string{"a.go"})
	m.openFilePicker()
	require.True(t, m.overlay.Active())

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = result.(Model)
	assert.Nil(t, cmd)
	assert.False(t, m.overlay.Active())
}

func TestModel_JumpFileMouseSelectionLoadsFile(t *testing.T) {
	m := filePickerModel([]string{"a.go", "b.go"})
	m.openFilePicker()
	mgr := m.overlay.(*overlay.Manager)
	base := makeOverlayBase(m.layout.width, m.layout.height)
	_ = mgr.Compose(base, overlay.RenderCtx{Width: m.layout.width, Height: m.layout.height, Resolver: m.resolver})

	// The 80-column, 8-row popup starts at (20,16). Local row 5 is b.go.
	result, cmd := m.Update(tea.MouseMsg{X: 22, Y: 21, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = result.(Model)

	assert.False(t, m.overlay.Active())
	assert.Equal(t, "b.go", m.tree.SelectedFile())
	assert.Equal(t, paneDiff, m.layout.focus)
	assert.NotNil(t, cmd)
}
