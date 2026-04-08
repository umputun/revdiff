package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/mocks"
)

func TestModel_HelpOverlaySections(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	// verify section headers are present
	assert.Contains(t, help, "Navigation")
	assert.Contains(t, help, "Search")
	assert.Contains(t, help, "Annotations")
	assert.Contains(t, help, "View")
	assert.Contains(t, help, "Quit")
}

func TestModel_HelpOverlayKeyListings(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	// verify key listings are present (dynamic rendering uses display names)
	keys := []string{
		"Tab", "PgDn", "PgUp", "Ctrl+d", "Ctrl+u", "Home", "End",
		"j", "k", "n", "N", "h", "l",
		"/", "Enter", "A", "d", "@", "f", "v", "w", ".", "L", "t",
		"q", "Q", "?", "Esc",
	}
	for _, k := range keys {
		assert.Contains(t, help, k, "help overlay should contain key: %s", k)
	}
}

func TestModel_HelpOverlayInView(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.width = 100
	m.height = 80

	// without help, view should not contain help sections
	m.showHelp = false
	view := m.View()
	assert.NotContains(t, view, "Navigation")
	assert.NotContains(t, view, "Annotations")

	// with help, view should contain help sections overlaid on top of content
	m.showHelp = true
	view = m.View()
	assert.Contains(t, view, "Navigation")
	assert.Contains(t, view, "Annotations")
	assert.Contains(t, view, "View")
	assert.Contains(t, view, "Quit")
	// overlay should preserve background content (tree pane visible on edges)
	assert.Contains(t, view, "a.go", "tree pane should be visible behind help overlay")
}

func TestModel_HelpOverlayContainsWordWrap(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()
	assert.Contains(t, help, "toggle word wrap")
	assert.Contains(t, help, "w")
}

func TestModel_HelpOverlayContainsSearchKeys(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	assert.Contains(t, help, "Search")
	assert.Contains(t, help, "search in diff")
	// n/N for next/prev search match is shown via File/Hunk section's "next file / search match"
	assert.Contains(t, help, "next file / search match")
	assert.Contains(t, help, "prev file / search match")
}

func TestModel_HelpOverlayContainsTOCSection(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	assert.Contains(t, help, "Markdown TOC")
	assert.Contains(t, help, "switch between TOC and diff")
	assert.Contains(t, help, "navigate TOC entries")
	assert.Contains(t, help, "jump to header in diff")
}

func TestModel_HelpOverlayContainsLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 120
	m.height = 40
	help := m.helpOverlay()
	assert.Contains(t, help, "L")
	assert.Contains(t, help, "line numbers")
}

func TestModel_HelpOverlayCustomBinding(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	m.keymap.Bind("x", keymap.ActionQuit)
	help := m.helpOverlay()

	// custom binding should appear alongside default
	assert.Contains(t, help, "x")
	assert.Contains(t, help, "q")
	assert.Contains(t, help, "quit")
}

func TestModel_HelpOverlayUnmappedAction(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	// unbind all keys for search action
	m.keymap.Unbind("/")
	help := m.helpOverlay()

	// search section should still exist but "search in diff" description should be gone
	assert.NotContains(t, help, "search in diff")
}

func TestModel_HelpToggle(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": {{ChangeType: diff.ChangeContext, Content: "x"}}})
	m.currFile = "a.go"
	m.focus = paneDiff
	assert.False(t, m.showHelp)

	// press ? to open help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := result.(Model)
	assert.True(t, model.showHelp)

	// press ? again to close help
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = result.(Model)
	assert.False(t, model.showHelp)
}

func TestModel_HelpCloseWithEsc(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": {{ChangeType: diff.ChangeContext, Content: "x"}}})
	m.currFile = "a.go"
	m.showHelp = true

	// press esc to close help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.showHelp)
}

func TestModel_HelpBlocksOtherKeys(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeContext, Content: "x"}},
		"b.go": {{ChangeType: diff.ChangeContext, Content: "y"}},
	})
	m.currFile = "a.go"
	m.focus = paneDiff
	m.showHelp = true

	// navigation keys should be blocked
	for _, key := range []rune{'n', 'p', 'v', 'f', 'q', 'Q', 'j', 'k'} {
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		model := result.(Model)
		assert.True(t, model.showHelp, "key %q should not close help", string(key))
		assert.Nil(t, cmd, "key %q should produce no command", string(key))
	}

	// tab should also be blocked
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := result.(Model)
	assert.True(t, model.showHelp, "tab should not close help")
	assert.Nil(t, cmd, "tab should produce no command")

	// enter should be blocked
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.True(t, model.showHelp, "enter should not close help")
	assert.Nil(t, cmd, "enter should produce no command")
}

func TestModel_MarkReviewedFromTreePane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	m.focus = paneTree

	// space bar toggles reviewed
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.True(t, model.tree.reviewed["a.go"], "space should mark current file as reviewed")
	assert.Equal(t, 1, model.tree.reviewedCount())

	// space again toggles off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model = result.(Model)
	assert.False(t, model.tree.reviewed["a.go"], "space should unmark reviewed file")
	assert.Equal(t, 0, model.tree.reviewedCount())
}

func TestModel_MarkReviewedFromTreePaneUsesSelectedFile(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	m.focus = paneTree
	m.tree.moveDown() // cursor -> b.go while the diff pane still shows a.go

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.True(t, model.tree.reviewed["b.go"], "space in tree pane should mark selected file")
	assert.False(t, model.tree.reviewed["a.go"], "space in tree pane should not mark stale currFile")
}

func TestModel_MarkReviewedFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "b.go"
	m.focus = paneDiff

	// space from diff pane marks currFile
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model := result.(Model)
	assert.True(t, model.tree.reviewed["b.go"], "space in diff pane should mark currFile as reviewed")
}

func TestModel_FKeyFilterToggle(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test annotation"})

	t.Run("toggle filter on and off from tree pane", func(t *testing.T) {
		m.focus = paneTree
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.True(t, model.tree.filter)

		// only a.go should be visible (1 dir + 1 file)
		fileCount := 0
		for _, e := range model.tree.entries {
			if !e.isDir {
				fileCount++
			}
		}
		assert.Equal(t, 1, fileCount)

		// toggle filter off
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model = result.(Model)
		assert.False(t, model.tree.filter)
	})

	t.Run("works from diff pane", func(t *testing.T) {
		m.focus = paneDiff
		m.tree.filter = false
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.True(t, model.tree.filter)
	})

	t.Run("no-op when no annotations", func(t *testing.T) {
		m2 := testModel([]string{"a.go", "b.go"}, nil)
		m2.tree = newFileTree([]string{"a.go", "b.go"})
		// no annotations added
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.False(t, model.tree.filter, "filter should not toggle when no annotated files")
	})
}

func TestModel_FilterToggleLoadsDiffForNewSelection(t *testing.T) {
	lines := map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a-line", ChangeType: diff.ChangeAdd}},
		"b.go": {{NewNum: 1, Content: "b-line", ChangeType: diff.ChangeAdd}},
	}
	m := testModel([]string{"a.go", "b.go"}, lines)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "b.go"
	m.diffLines = lines["b.go"]
	m.focus = paneTree

	// annotate only a.go
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note on a"})

	// toggle filter on — should select a.go (the only annotated file)
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model := result.(Model)
	assert.True(t, model.tree.filter)

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
	m := NewModel(renderer, store, noopHighlighter(), ModelConfig{NoConfirmDiscard: true, TreeWidthRatio: 3})
	assert.True(t, m.noConfirmDiscard, "noConfirmDiscard should be wired from ModelConfig")
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
	m.noConfirmDiscard = true
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
	assert.Equal(t, 100, model.width, "resize should be handled during confirmation")
	assert.True(t, model.inConfirmDiscard, "should still be confirming after resize")
}

func TestModel_HandleEscKeyClearsSearch(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{ChangeType: diff.ChangeAdd, Content: "hello world"}},
	})
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{ChangeType: diff.ChangeAdd, Content: "hello world"}}
	m.searchTerm = "hello"
	m.searchMatches = []int{0}
	m.searchCursor = 0
	m.focus = paneDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.Empty(t, model.searchTerm, "esc should clear search term")
	assert.Nil(t, model.searchMatches, "esc should clear search matches")
}

func TestModel_HandleEscKeyNoopWithoutSearch(t *testing.T) {
	m := testModel(nil, nil)
	m.focus = paneDiff

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.Empty(t, model.searchTerm)
	assert.Nil(t, model.searchMatches)
}

func TestModel_HandleFileAnnotateKey(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("starts annotation when focus is diff and file is set", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.currFile = "a.go"
		m.diffLines = lines
		m.focus = paneDiff

		result, cmd := m.handleFileAnnotateKey()
		model := result.(Model)
		assert.True(t, model.annotating, "should start annotation mode")
		assert.NotNil(t, cmd, "should return a command")
	})

	t.Run("no-op when focus is tree pane", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.currFile = "a.go"
		m.diffLines = lines
		m.focus = paneTree

		result, cmd := m.handleFileAnnotateKey()
		model := result.(Model)
		assert.False(t, model.annotating, "should not start annotation")
		assert.Nil(t, cmd, "should return nil command")
	})

	t.Run("no-op when currFile is empty", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.currFile = ""
		m.focus = paneDiff

		result, cmd := m.handleFileAnnotateKey()
		model := result.(Model)
		assert.False(t, model.annotating, "should not start annotation")
		assert.Nil(t, cmd, "should return nil command")
	})
}

func TestModel_HandleViewToggle_WordDiff(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 0, Content: "hello world", ChangeType: diff.ChangeRemove},
		{OldNum: 0, NewNum: 1, Content: "hello brave world", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines

	assert.False(t, m.wordDiff)
	result, _ := m.handleViewToggle(keymap.ActionToggleWordDiff)
	m = result.(Model)
	assert.True(t, m.wordDiff)
	require.Len(t, m.intraRanges, len(lines))
	assert.NotNil(t, m.intraRanges[1], "paired add line should have ranges")

	result, _ = m.handleViewToggle(keymap.ActionToggleWordDiff)
	m = result.(Model)
	assert.False(t, m.wordDiff)
	assert.Nil(t, m.intraRanges, "ranges cleared when toggled off")
}
