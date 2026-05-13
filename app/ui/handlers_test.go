package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

func TestModel_MarkReviewedFromTreePane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.file.name = "a.go"
	m.layout.focus = paneTree

	// space bar toggles reviewed
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.Equal(t, 1, model.tree.ReviewedCount(), "space should mark current file as reviewed")

	// space again toggles off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model = result.(Model)
	assert.Equal(t, 0, model.tree.ReviewedCount(), "space should unmark reviewed file")
}

func TestModel_MarkReviewedFromTreePaneUsesSelectedFile(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.file.name = "a.go"
	m.layout.focus = paneTree
	m.tree.Move(sidepane.MotionDown) // cursor -> b.go while the diff pane still shows a.go

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.Equal(t, 1, model.tree.ReviewedCount(), "space in tree pane should mark selected file (b.go)")
	// verify it's b.go that's reviewed by toggling it off and checking count drops
	model.tree.ToggleReviewed("b.go")
	assert.Equal(t, 0, model.tree.ReviewedCount(), "b.go was the reviewed file")
}

func TestModel_MarkReviewedFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.file.name = "b.go"
	m.layout.focus = paneDiff

	// space from diff pane marks currFile
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.Equal(t, 1, model.tree.ReviewedCount(), "space in diff pane should mark currFile as reviewed")
}

func TestModel_FKeyFilterToggle(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test annotation"})

	t.Run("toggle filter on and off from tree pane", func(t *testing.T) {
		m.layout.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.True(t, model.tree.FilterActive())

		// only a.go should be visible after filter
		assert.Equal(t, "a.go", model.tree.SelectedFile(), "only annotated file should be selected")
		assert.False(t, model.tree.HasFile(sidepane.DirectionNext), "no next file when only one annotated")

		// toggle filter off
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model = result.(Model)
		assert.False(t, model.tree.FilterActive())
	})

	t.Run("works from diff pane", func(t *testing.T) {
		m.layout.focus = paneDiff
		// filter should be off after previous subtest toggled it off
		require.False(t, m.tree.FilterActive(), "precondition: filter must be off")
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.True(t, model.tree.FilterActive())
	})

	t.Run("no-op when no annotations", func(t *testing.T) {
		m2 := testModel([]string{"a.go", "b.go"}, nil)
		m2.tree = testNewFileTree([]string{"a.go", "b.go"})
		// no annotations added
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.False(t, model.tree.FilterActive(), "filter should not toggle when no annotated files")
	})
}

func TestModel_FilterToggleLoadsDiffForNewSelection(t *testing.T) {
	lines := map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a-line", ChangeType: diff.ChangeAdd}},
		"b.go": {{NewNum: 1, Content: "b-line", ChangeType: diff.ChangeAdd}},
	}
	m := testModel([]string{"a.go", "b.go"}, lines)
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.file.name = "b.go"
	m.file.lines = lines["b.go"]
	m.layout.focus = paneTree

	// annotate only a.go
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note on a"})

	// toggle filter on — should select a.go (the only annotated file)
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model := result.(Model)
	assert.True(t, model.tree.FilterActive())

	// since b.go was current and a.go is now selected, a load command should be returned
	if cmd != nil {
		msg := cmd()
		flm, ok := msg.(fileLoadedMsg)
		assert.True(t, ok, "filter toggle should trigger file load for new selection")
		assert.Equal(t, "a.go", flm.file)
	} else {
		t.Fatal("expected a load command after filter toggle changed selection")
	}
}

func TestModel_QuitKey(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)

	// cmd should be tea.Quit
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}

func TestModel_QuitPreservesAnnotations(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 5, Type: "+", Comment: "needs review"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 10, Type: " ", Comment: "check this"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)

	// verify annotations survive the quit
	model := result.(Model)
	output := model.Store().FormatOutput()
	assert.Contains(t, output, "a.go:5")
	assert.Contains(t, output, "needs review")
	assert.Contains(t, output, "b.go:10")
	assert.Contains(t, output, "check this")
}

func TestModel_QuitNoAnnotationsEmptyOutput(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.Empty(t, model.Store().FormatOutput())
}

func TestModel_NoConfirmDiscardWired(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	store := annotation.NewStore()
	m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{NoConfirmDiscard: true, TreeWidthRatio: 3})
	assert.True(t, m.cfg.noConfirmDiscard, "noConfirmDiscard should be wired from ModelConfig")
}

func TestModel_ConfirmDiscardY(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "y should confirm discard")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit after y")
}

func TestModel_ConfirmDiscardN(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	assert.Nil(t, cmd, "n should not quit")

	model := result.(Model)
	assert.False(t, model.inConfirmDiscard, "n should cancel confirmation")
	assert.False(t, model.Discarded(), "should not be discarded")
}

func TestModel_ConfirmDiscardEsc(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.Nil(t, cmd, "esc should not quit")

	model := result.(Model)
	assert.False(t, model.inConfirmDiscard, "esc should cancel confirmation")
	assert.False(t, model.Discarded())
}

func TestModel_ConfirmDiscardSecondQ(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "second Q should confirm discard")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit after second Q")
}

func TestModel_QKeyNoConfirmDiscardWithAnnotations(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.cfg.noConfirmDiscard = true
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "should immediately discard with noConfirmDiscard")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming state")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit immediately")
}

func TestModel_ConfirmDiscardBlocksOtherKeys(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	// pressing j (navigation) should be blocked
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Nil(t, cmd, "j should be blocked during confirmation")
	model := result.(Model)
	assert.True(t, model.inConfirmDiscard, "should still be confirming")

	// pressing q should be blocked too
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.Nil(t, cmd, "q should be blocked during confirmation")
	model = result.(Model)
	assert.True(t, model.inConfirmDiscard, "should still be confirming")
}

func TestModel_ConfirmDiscardAllowsNonKeyMessages(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})
	m.inConfirmDiscard = true

	// WindowSizeMsg should still be handled
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := result.(Model)
	assert.Equal(t, 100, model.layout.width, "resize should be handled during confirmation")
	assert.True(t, model.inConfirmDiscard, "should still be confirming after resize")
}

func TestModel_HandleEscKeyClearsSearch(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "hello world"}},
	})
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{ChangeType: diff.ChangeAdd, Content: "hello world"}}
	m.search.term = "hello"
	m.search.matches = []int{0}
	m.search.cursor = 0
	m.layout.focus = paneDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.Empty(t, model.search.term, "esc should clear search term")
	assert.Nil(t, model.search.matches, "esc should clear search matches")
}

func TestModel_HandleEscKeyNoopWithoutSearch(t *testing.T) {
	m := testModel(nil, nil)
	m.layout.focus = paneDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.Empty(t, model.search.term)
	assert.Nil(t, model.search.matches)
}

func TestModel_HandleFileAnnotateKey(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("starts annotation when focus is diff and file is set", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.file.name = "a.go"
		m.file.lines = lines
		m.layout.focus = paneDiff

		result, cmd := m.handleFileAnnotateKey()
		model := result.(Model)
		assert.True(t, model.annot.annotating, "should start annotation mode")
		assert.NotNil(t, cmd, "should return a command")
	})

	t.Run("no-op when focus is tree pane", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.file.name = "a.go"
		m.file.lines = lines
		m.layout.focus = paneTree

		result, cmd := m.handleFileAnnotateKey()
		model := result.(Model)
		assert.False(t, model.annot.annotating, "should not start annotation")
		assert.Nil(t, cmd, "should return nil command")
	})

	t.Run("no-op when currFile is empty", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.file.name = ""
		m.layout.focus = paneDiff

		result, cmd := m.handleFileAnnotateKey()
		model := result.(Model)
		assert.False(t, model.annot.annotating, "should not start annotation")
		assert.Nil(t, cmd, "should return nil command")
	})
}

func TestModel_HandleInfo_OpensOverlayWhenApplicable(t *testing.T) {
	commits := []diff.CommitInfo{{Hash: "abc123"}, {Hash: "def456"}}
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.list = commits

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "i should open the overlay when applicable")
	assert.Equal(t, overlay.KindInfo, model.overlay.Kind())
}

func TestModel_HandleInfo_OpensOverlayEvenWhenCommitsHidden(t *testing.T) {
	// regression for the unified info popup: the overlay must open in modes
	// where the commits section is hidden (stdin / staged / no-VCS), so the
	// session info section is reachable. The pre-merge behavior was a
	// "no commits in this mode" hint with no overlay — now removed.
	fake := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
		return []diff.CommitInfo{{Hash: "abc"}}, nil
	}}
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = fake
	m.commits.applicable = false // e.g. stdin/staged/only mode

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay must open in every mode — commits section hides itself")
	assert.Equal(t, overlay.KindInfo, model.overlay.Kind())
	assert.Equal(t, 0, fake.calls, "no commits fetch when feature is not applicable")
}

func TestModel_HandleInfo_OpensOverlayWhenSourceNil(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = nil
	m.commits.applicable = false // applicable always collapses to false without a source

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay opens even with no commit source — session section still useful")
}

func TestModel_HandleInfo_StoresErrorInSpec(t *testing.T) {
	boom := errors.New("git blew up")
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.err = boom

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay opens even on fetch failure so user sees the error")
	assert.Equal(t, overlay.KindInfo, model.overlay.Kind())
	require.Error(t, model.commits.err)
	assert.Equal(t, boom, model.commits.err, "error is stored on the model and passed into the spec")
}

func TestModel_HandleInfo_OpensImmediatelyDuringLoad(t *testing.T) {
	// regression: pressing `i` before commits finish loading must open the
	// popup immediately. The commits section renders "loading commits…"
	// inline; the previous behavior of refusing to open is gone.
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = false

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay opens immediately even while commits are loading")
}

func TestModel_ReloadHint_ShownInStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.reload.hint = "test reload hint"
	assert.Equal(t, "test reload hint", m.statusBarText())
}

func TestModel_ReloadHint_ClearsOnNextKey(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.reload.hint = "some hint"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Empty(t, model.reload.hint, "any key press must clear the reload hint")
}

func TestModel_HandleInfo_TruncatedFlagPropagates(t *testing.T) {
	full := make([]diff.CommitInfo, diff.MaxCommits)
	for i := range full {
		full[i] = diff.CommitInfo{Hash: "h"}
	}
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.list = full
	m.commits.truncated = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.commits.truncated, "MaxCommits result sets the truncated flag")
	assert.True(t, model.overlay.Active())
}

func TestModel_ActionReload_StdinGuard(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.reload.applicable = false // stdin mode

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := result.(Model)
	assert.Equal(t, "Reload not available in stdin mode", model.reload.hint)
	assert.False(t, model.reload.pending)
}

func TestModel_ActionReload_NoAnnotations_DirectReload(t *testing.T) {
	callCount := 0
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
			callCount++
			return []diff.FileEntry{{Path: "a.go"}}, nil
		},
		FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(),
		ModelConfig{ReloadApplicable: true})
	initialSeq := m.filesLoadSeq

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := result.(Model)
	assert.False(t, model.reload.pending, "no confirmation needed without annotations")
	assert.Equal(t, "Reloaded", model.reload.hint)
	assert.Equal(t, initialSeq+1, model.filesLoadSeq, "filesLoadSeq must be bumped")
	assert.NotNil(t, cmd, "reload command must be returned")
}

func TestModel_ActionReload_WithAnnotations_SetsPending(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(),
		ModelConfig{ReloadApplicable: true})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := result.(Model)
	assert.True(t, model.reload.pending, "confirmation must be requested when annotations exist")
	assert.Equal(t, "Annotations will be dropped — press y to confirm, any other key to cancel", model.reload.hint)
	assert.Nil(t, cmd, "no reload command before confirmation")
	assert.Equal(t, 1, store.Count(), "annotations must not be cleared yet")
}

func TestModel_ActionReload_YConfirms(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	callCount := 0
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
			callCount++
			return []diff.FileEntry{{Path: "a.go"}}, nil
		},
		FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := testNewModel(t, renderer, store, noopHighlighter(),
		ModelConfig{ReloadApplicable: true})

	// first R: sets pending
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := result.(Model)
	require.True(t, model.reload.pending)

	// y: confirms
	result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = result.(Model)
	assert.False(t, model.reload.pending, "pending must be cleared after confirmation")
	assert.Equal(t, 0, store.Count(), "annotations must be cleared after confirmation")
	assert.Equal(t, "Reloaded", model.reload.hint)
	assert.NotNil(t, cmd, "reload command must be returned after confirmation")
}

func TestModel_ActionReload_OtherKeyCancels(t *testing.T) {
	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m := testNewModel(t, plainRenderer(), store, noopHighlighter(),
		ModelConfig{ReloadApplicable: true})

	// first R: sets pending
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model := result.(Model)
	require.True(t, model.reload.pending)

	// j: cancels
	result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.False(t, model.reload.pending, "pending must be cleared on cancel")
	assert.Equal(t, "Reload canceled", model.reload.hint)
	assert.Equal(t, 1, store.Count(), "annotations must not be cleared on cancel")
	assert.Nil(t, cmd, "no reload command on cancel")
}

// TestModel_HandleFilterToggle_TurnsOffWhenNoAnnotations is a regression test
// for the || m.tree.FilterActive() branch at handlers.go:181.
// The filter must be toggle-able off even when all annotations have been deleted
// (annotatedFiles() returns empty), so that the user is not stuck with an empty
// filtered view after a reload or manual annotation deletion.
func TestModel_HandleFilterToggle_TurnsOffWhenNoAnnotations(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go", "b.go"})

	// add annotation and turn filter on
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model := result.(Model)
	require.True(t, model.tree.FilterActive(), "precondition: filter must be active after 'f' with annotation")

	// delete the annotation so annotatedFiles() becomes empty
	model.store.Delete("a.go", 1, "+")
	require.Equal(t, 0, model.store.Count(), "precondition: store must be empty after delete")
	require.Empty(t, model.annotatedFiles(), "precondition: annotatedFiles() must return empty")

	// pressing f again must toggle the filter off via the FilterActive() branch
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = result.(Model)
	assert.False(t, model.tree.FilterActive(),
		"filter must toggle off even when no annotations remain — guards the || m.tree.FilterActive() branch")
}

func TestModel_ToggleCompactMode_FlipsModeAndRefetches(t *testing.T) {
	var calls int
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			calls++
			return nil, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.compact.applicable = true
	m.modes.compactContext = 5
	m.file.name = "a.go"

	// pressing C flips compact on and issues a re-fetch of the current file
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	model := result.(Model)
	assert.True(t, model.modes.compact, "C should flip compact mode on when applicable")
	require.NotNil(t, cmd, "C should issue a re-fetch command for the current file")
	cmd()
	assert.Equal(t, 1, calls, "toggle must trigger exactly one FileDiff call for the current file")
	assert.Empty(t, model.compact.hint, "applicable path must not set a hint")

	// pressing C again flips compact off and re-fetches
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	model = result.(Model)
	assert.False(t, model.modes.compact, "C should flip compact mode off on second press")
	require.NotNil(t, cmd, "second press must also issue a re-fetch")
	cmd()
	assert.Equal(t, 2, calls)
}

func TestModel_ToggleCompactMode_NoOpWhenNotApplicable(t *testing.T) {
	var calls int
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			calls++
			return nil, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.compact.applicable = false
	m.file.name = "a.go"
	beforeSeq := m.file.loadSeq

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	model := result.(Model)
	assert.False(t, model.modes.compact, "non-applicable toggle must leave mode unchanged")
	assert.Nil(t, cmd, "no-op path must not issue a command")
	assert.Equal(t, beforeSeq, model.file.loadSeq, "no-op path must not bump loadSeq")
	assert.Equal(t, 0, calls, "no-op path must not invoke FileDiff")
	assert.Equal(t, "compact not applicable in this mode", model.compact.hint, "hint must surface the reason")
}

func TestModel_CompactHint_ShownInStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.compact.hint = "test compact hint"
	assert.Equal(t, "test compact hint", m.statusBarText())
}

func TestModel_CompactHint_ClearsOnNextKey(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.compact.hint = "some hint"

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Empty(t, model.compact.hint, "any key press must clear the compact hint")
}

func TestModel_TransientHint_Priority(t *testing.T) {
	t.Run("reload wins over compact", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.reload.hint = "reload msg"
		m.compact.hint = "compact msg"
		assert.Equal(t, "reload msg", m.transientHint())
	})

	t.Run("compact when only one set", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.compact.hint = "compact msg"
		assert.Equal(t, "compact msg", m.transientHint())
	})

	t.Run("empty when none set", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		assert.Empty(t, m.transientHint())
	})
}

func TestModel_ToggleCompactMode_DoesNotReloadFilesOrCommits(t *testing.T) {
	var fileDiffCalls, changedFilesCalls int
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) {
			changedFilesCalls++
			return nil, nil
		},
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			fileDiffCalls++
			return nil, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.compact.applicable = true
	m.modes.compactContext = 5
	m.file.name = "a.go"
	beforeFilesSeq := m.filesLoadSeq
	beforeCommitsSeq := m.commits.loadSeq

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	model := result.(Model)
	require.NotNil(t, cmd)
	cmd()

	assert.Equal(t, beforeFilesSeq, model.filesLoadSeq, "toggle must not bump filesLoadSeq (no file-list reload)")
	assert.Equal(t, beforeCommitsSeq, model.commits.loadSeq, "toggle must not bump commits.loadSeq (no commit reload)")
	assert.Equal(t, 0, changedFilesCalls, "toggle must not call ChangedFiles")
	assert.Equal(t, 1, fileDiffCalls, "toggle must trigger exactly one FileDiff call (current file only)")
}

func TestModel_ToggleCompactMode_CursorResetsAfterReload(t *testing.T) {
	// simulates the full toggle flow: press C, then process the fileLoadedMsg
	// from the triggered re-fetch. verifies skipInitialDividers ran and the
	// cursor landed on the first non-divider visible line.
	compactDiff := []diff.DiffLine{
		{ChangeType: diff.ChangeDivider},
		{NewNum: 40, Content: "context before", ChangeType: diff.ChangeContext},
		{NewNum: 41, Content: "added line", ChangeType: diff.ChangeAdd},
		{NewNum: 42, Content: "context after", ChangeType: diff.ChangeContext},
	}
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			return compactDiff, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.compact.applicable = true
	m.modes.compactContext = 5
	m.file.name = "a.go"
	m.nav.diffCursor = 999 // pretend cursor was somewhere deep in a full-file view

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	model := result.(Model)
	require.NotNil(t, cmd)
	msg := cmd()
	result, _ = model.Update(msg)
	model = result.(Model)

	// after skipInitialDividers, cursor should skip index 0 (divider) and land on 1
	assert.Equal(t, 1, model.nav.diffCursor, "cursor must reset to first non-divider line after compact re-fetch")
}

func TestDisplayKeyName(t *testing.T) {
	m := testModel(nil, nil)
	tests := []struct{ input, want string }{
		{"ctrl+e", "Ctrl+E"},
		{"ctrl+d", "Ctrl+D"},
		{"ctrl+u", "Ctrl+U"},
		{"ctrl+w>x", "Ctrl+W>x"},
		{"ctrl+", "Ctrl+"},
		{"j", "j"},
		{"pgdown", "PgDn"},
		{"enter", "Enter"},
		{" ", "Space"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, m.displayKeyName(tt.input), "displayKeyName(%q)", tt.input)
	}
}

func TestBuildHelpSpec_SearchPromptHistoryEntries(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	spec := m.buildHelpSpec()
	var searchSection *overlay.HelpSection
	for i := range spec.Sections {
		if spec.Sections[i].Title == "Search" {
			searchSection = &spec.Sections[i]
			break
		}
	}
	require.NotNil(t, searchSection, "help overlay must include a Search section")

	var upEntry, downEntry *overlay.HelpEntry
	for i := range searchSection.Entries {
		switch searchSection.Entries[i].Keys {
		case "↑ / Ctrl+P":
			upEntry = &searchSection.Entries[i]
		case "↓ / Ctrl+N":
			downEntry = &searchSection.Entries[i]
		}
	}
	require.NotNil(t, upEntry, "Search section must list the Up / Ctrl+P recall binding")
	require.NotNil(t, downEntry, "Search section must list the Down / Ctrl+N recall binding")
	assert.Contains(t, upEntry.Description, "previous", "Up entry description must mention previous query")
	assert.Contains(t, downEntry.Description, "next", "Down entry description must mention next query")
}

func TestBuildHelpSpec_VimMotionSectionOff(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = false

	spec := m.buildHelpSpec()
	for _, sec := range spec.Sections {
		assert.NotEqual(t, "Vim motion", sec.Title,
			"help overlay must not include a Vim motion section when --vim-motion is off")
	}
}

func TestBuildHelpSpec_VimMotionSectionOn(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.modes.vimMotion = true

	spec := m.buildHelpSpec()
	var vimSection *overlay.HelpSection
	for i := range spec.Sections {
		if spec.Sections[i].Title == "Vim motion" {
			vimSection = &spec.Sections[i]
			break
		}
	}
	require.NotNil(t, vimSection, "help overlay must include a Vim motion section when --vim-motion is on")
	require.Len(t, vimSection.Entries, 8, "Vim motion section must list all 8 preset bindings")

	// verify each expected binding is present by key string
	wantKeys := []string{"N j / N k", "gg", "G / N G", "zz", "zt", "zb", "ZZ", "ZQ"}
	for i, want := range wantKeys {
		assert.Equal(t, want, vimSection.Entries[i].Keys,
			"entry %d key string mismatch", i)
		assert.NotEmpty(t, vimSection.Entries[i].Description,
			"entry %d must have a description", i)
	}
}
