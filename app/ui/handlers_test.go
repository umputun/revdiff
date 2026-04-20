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
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
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

func TestModel_HandleCommitInfo_OpensOverlayWhenApplicable(t *testing.T) {
	commits := []diff.CommitInfo{{Hash: "abc123"}, {Hash: "def456"}}
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.list = commits

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.Nil(t, cmd, "commit-info open does not issue a tea.Cmd")
	assert.True(t, model.overlay.Active(), "i should open the overlay when applicable")
	assert.Equal(t, overlay.KindCommitInfo, model.overlay.Kind())
	assert.Empty(t, model.commits.hint, "applicable path does not set a hint")
}

func TestModel_HandleCommitInfo_HintWhenNotApplicable(t *testing.T) {
	fake := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
		return []diff.CommitInfo{{Hash: "abc"}}, nil
	}}
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = fake
	m.commits.applicable = false // e.g. stdin/staged/only mode

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.False(t, model.overlay.Active(), "overlay must not open in a non-applicable mode")
	assert.Equal(t, "no commits in this mode", model.commits.hint, "hint surfaces the reason")
	assert.Equal(t, 0, fake.calls, "no fetch when feature is not applicable")
	assert.False(t, model.commits.loaded)
}

func TestModel_HandleCommitInfo_HintWhenSourceNil(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = nil
	m.commits.applicable = false // applicable always collapses to false without a source

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.False(t, model.overlay.Active(), "nil source means feature is unavailable")
	assert.Equal(t, "no commits in this mode", model.commits.hint)
}

func TestModel_HandleCommitInfo_StoresErrorInSpec(t *testing.T) {
	boom := errors.New("git blew up")
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.err = boom

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay opens even on fetch failure so user sees the error")
	assert.Equal(t, overlay.KindCommitInfo, model.overlay.Kind())
	require.Error(t, model.commits.err)
	assert.Equal(t, boom, model.commits.err, "error is stored on the model and passed into the spec")
}

func TestModel_HandleCommitInfo_HintClearsOnNextKey(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = nil
	m.commits.applicable = false

	// first press sets the hint
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	require.Equal(t, "no commits in this mode", model.commits.hint)

	// next key clears the hint before dispatching the new action
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Empty(t, model.commits.hint, "any subsequent key press must clear the transient hint")
}

func TestModel_HandleCommitInfo_StatusBarShowsHint(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = nil
	m.commits.applicable = false

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)

	status := model.statusBarText()
	assert.Equal(t, "no commits in this mode", status, "status bar surfaces the hint verbatim while it is set")
}

func TestModel_HandleCommitInfo_ShowsLoadingHintBeforeLoad(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = true
	m.commits.loaded = false

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model := result.(Model)
	assert.False(t, model.overlay.Active(), "overlay must not open before commits load")
	assert.Equal(t, "loading commits…", model.commits.hint, "transient hint shown while fetch is in flight")
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

func TestModel_HandleCommitInfo_TruncatedFlagPropagates(t *testing.T) {
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
		FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) { return nil, nil },
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
		FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) { return nil, nil },
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
