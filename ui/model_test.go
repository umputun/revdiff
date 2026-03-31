package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
	"github.com/umputun/revdiff/diff/mocks"
)

func testModel(files []string, fileDiffs map[string][]diff.DiffLine) Model {
	renderer := &mocks.DiffRendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]string, error) {
			return files, nil
		},
		FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
			return fileDiffs[file], nil
		},
	}
	store := annotation.NewStore()
	m := NewModel(renderer, store, "", false)
	// simulate window size
	m.width = 120
	m.height = 40
	m.treeWidth = defaultTreeWidth
	m.ready = true
	return m
}

func TestModel_Init(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	cmd := m.Init()
	require.NotNil(t, cmd)

	// execute the command - should produce filesLoadedMsg
	msg := cmd()
	flm, ok := msg.(filesLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, []string{"a.go", "b.go"}, flm.files)
	assert.NoError(t, flm.err)
}

func TestModel_FilesLoaded(t *testing.T) {
	m := testModel(nil, nil)

	result, cmd := m.Update(filesLoadedMsg{files: []string{"internal/handler.go", "internal/store.go", "main.go"}})
	model := result.(Model)

	// tree should be populated
	assert.Len(t, model.tree.entries, 5) // 2 dirs + 3 files
	assert.NotNil(t, cmd)                // should auto-select first file
}

func TestModel_FilesLoadedError(t *testing.T) {
	m := testModel(nil, nil)
	m.ready = true

	result, cmd := m.Update(filesLoadedMsg{err: assert.AnError})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Empty(t, model.tree.entries)
}

func TestModel_FileLoaded(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})

	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func main() {}", ChangeType: diff.ChangeAdd},
	}

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.Equal(t, "a.go", model.currFile)
	assert.Len(t, model.diffLines, 2)
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
	m.store.Add("a.go", 5, "+", "needs review")
	m.store.Add("b.go", 10, " ", "check this")

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

func TestModel_EnterSelectsFile(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}},
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.focus = paneTree

	// enter on selected file should trigger file load
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	flm, ok := msg.(fileLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, "a.go", flm.file)
}

func TestModel_TabToggleFilter(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.store.Add("a.go", 1, "+", "test annotation")

	// toggle filter - should show only annotated files
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
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

	// toggle back
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.False(t, model.tree.filter)
}

func TestModel_NextPrevFile(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
		"c.go": {{NewNum: 1, Content: "c", ChangeType: diff.ChangeContext}},
	})
	m.tree = newFileTree(files)
	m.currFile = "a.go" // pretend first file is already loaded

	// starts on first file (a.go)
	assert.Equal(t, "a.go", m.tree.selectedFile())

	// press n - should move to b.go
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())
	assert.NotNil(t, cmd) // triggers file load

	// press n - should move to c.go
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, "c.go", model.tree.selectedFile())
	assert.NotNil(t, cmd)

	// press p - back to b.go
	result, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())
	assert.NotNil(t, cmd)
}

func TestModel_TreeNavigation(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.focus = paneTree

	// cursor starts on first file (a.go)
	assert.Equal(t, "a.go", m.tree.selectedFile())

	// j moves down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())

	// k moves up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, "a.go", model.tree.selectedFile())
}

func TestModel_FocusSwitching(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go" // pretend a file is loaded
	m.focus = paneTree

	// l switches to diff pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model := result.(Model)
	assert.Equal(t, paneDiff, model.focus)

	// h switches back to tree
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = result.(Model)
	assert.Equal(t, paneTree, model.focus)
}

func TestModel_DiffScrolling(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	// load file
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor)

	// j moves cursor down
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, 1, model.diffCursor)

	// k moves cursor back up
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor)
}

func TestModel_DiffCursorSkipsDividers(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor)

	// j should skip divider and land on line10
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = result.(Model)
	assert.Equal(t, 2, model.diffCursor)

	// k should skip divider and go back to line1
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = result.(Model)
	assert.Equal(t, 0, model.diffCursor)
}

func TestModel_DiffCursorAutoScrolls(t *testing.T) {
	// create more lines than viewport height
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	// move cursor past viewport height - viewport should auto-scroll
	for range 50 {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = result.(Model)
	}
	assert.Equal(t, 50, model.diffCursor)
	assert.Positive(t, model.viewport.YOffset, "viewport should have scrolled")
}

func TestModel_WindowResize(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.ready = false

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	model := result.(Model)

	assert.True(t, model.ready)
	assert.Equal(t, 100, model.width)
	assert.Equal(t, 50, model.height)
}

func TestModel_ViewOutput(t *testing.T) {
	m := testModel([]string{"internal/a.go", "internal/b.go"}, nil)
	m.tree = newFileTree([]string{"internal/a.go", "internal/b.go"})
	m.ready = true

	// tree pane focused - should show tree navigation hints
	m.focus = paneTree
	view := m.View()
	assert.Contains(t, view, "a.go")
	assert.Contains(t, view, "b.go")
	assert.Contains(t, view, "quit")
	assert.Contains(t, view, "navigate")

	// diff pane focused - should show diff hints
	m.focus = paneDiff
	view = m.View()
	assert.Contains(t, view, "annotate")
	assert.Contains(t, view, "scroll")
}

func TestModel_ViewNotReady(t *testing.T) {
	m := testModel(nil, nil)
	m.ready = false

	assert.Equal(t, "loading...", m.View())
}

func TestModel_AnnotatedFilesMarker(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.store.Add("a.go", 1, "+", "test")

	annotated := m.annotatedFiles()
	assert.True(t, annotated["a.go"])
	assert.False(t, annotated["b.go"])
}

func TestModel_RenderDiffEmpty(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = nil
	assert.Contains(t, m.renderDiff(), "no changes")
}

func TestModel_RenderDiffLines(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
		{OldNum: 3, Content: "func bar() {}", ChangeType: diff.ChangeRemove},
		{Content: "~~~", ChangeType: diff.ChangeDivider},
	}

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "package main")
	assert.Contains(t, rendered, "func foo()")
	assert.Contains(t, rendered, "func bar()")
}

func TestModel_CursorDiffLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.diffLines = lines
	m.diffCursor = 0

	dl, ok := m.cursorDiffLine()
	assert.True(t, ok)
	assert.Equal(t, "line1", dl.Content)
	assert.Equal(t, diff.ChangeContext, dl.ChangeType)

	m.diffCursor = 1
	dl, ok = m.cursorDiffLine()
	assert.True(t, ok)
	assert.Equal(t, "added", dl.Content)
	assert.Equal(t, diff.ChangeAdd, dl.ChangeType)

	// out of bounds
	m.diffCursor = -1
	_, ok = m.cursorDiffLine()
	assert.False(t, ok)

	m.diffCursor = 10
	_, ok = m.cursorDiffLine()
	assert.False(t, ok)
}

func TestModel_FileLoadedResetsCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.diffCursor = 5 // simulate cursor was elsewhere

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor) // cursor reset to first line
}

func TestModel_AnnotateKey(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// press 'a' - should enter annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.True(t, model.annotating)
	assert.NotNil(t, cmd) // textinput blink command
}

func TestModel_AnnotateEnterSaves(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// enter annotation mode and set text
	m.startAnnotation()
	m.annotateInput.SetValue("test comment")

	// press Enter - should save
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "test comment", anns[0].Comment)
	assert.Equal(t, 5, anns[0].Line)
	assert.Equal(t, diff.ChangeContext, anns[0].Type)
}

func TestModel_AnnotateEnterEmptyTextCancels(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	// don't set any text, press Enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_AnnotateEscCancels(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	m.annotateInput.SetValue("should not be saved")

	// press Esc - should cancel without saving
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.annotating)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_AnnotateOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// press 'a' on divider - should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.False(t, model.annotating)
}

func TestModel_AnnotateOnAddLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 3, Content: "new line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	m.annotateInput.SetValue("needs review")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 3, anns[0].Line)
	assert.Equal(t, diff.ChangeAdd, anns[0].Type)
}

func TestModel_AnnotateOnRemoveLine(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 7, Content: "removed line", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	m.startAnnotation()
	m.annotateInput.SetValue("why removed?")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 7, anns[0].Line)
	assert.Equal(t, diff.ChangeRemove, anns[0].Type)
}

func TestModel_AnnotatePreFillsExisting(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.store.Add("a.go", 1, " ", "old comment")

	m.startAnnotation()
	assert.Equal(t, "old comment", m.annotateInput.Value())
}

func TestModel_DeleteAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.store.Add("a.go", 1, " ", "test comment")

	// press 'd' - should delete annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_DeleteAnnotationOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// press 'd' on divider - should not panic
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	_ = result.(Model)
}

func TestModel_DeleteAnnotationNoAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0

	// no annotation exists, 'd' should be harmless
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_RenderDiffWithAnnotations(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
	}
	m.store.Add("a.go", 2, "+", "needs error handling")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "needs error handling")
	assert.Contains(t, rendered, "\U0001f4ac")
}

func TestModel_RenderDiffAnnotationInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.diffCursor = 0
	m.focus = paneDiff
	m.startAnnotation()
	m.annotateInput.SetValue("typing...")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "typing...")
	assert.Contains(t, rendered, "\U0001f4ac")
}

func TestModel_AnnotationSummary(t *testing.T) {
	m := testModel(nil, nil)
	m.store.Add("internal/a.go", 1, "+", "first comment")
	m.store.Add("b.go", 5, " ", "second comment")

	summary := m.renderAnnotationSummary(40)
	assert.Contains(t, summary, "annotations")
	assert.Contains(t, summary, "a.go:1")
	assert.Contains(t, summary, "b.go:5")
	assert.Contains(t, summary, "first comment")
	assert.Contains(t, summary, "second comment")
}

func TestModel_AnnotationSummaryEmpty(t *testing.T) {
	m := testModel(nil, nil)
	summary := m.renderAnnotationSummary(30)
	assert.Empty(t, summary)
}

func TestModel_AnnotationSummaryTruncatesLongComments(t *testing.T) {
	m := testModel(nil, nil)
	m.store.Add("a.go", 1, "+", "this is a very long comment that should be truncated because it exceeds the width limit")

	summary := m.renderAnnotationSummary(20)
	assert.Contains(t, summary, "...")
}

func TestModel_AnnotateStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.focus = paneDiff
	m.annotating = true

	view := m.View()
	assert.Contains(t, view, "save")
	assert.Contains(t, view, "cancel")
	assert.NotContains(t, view, "annotate")
}

func TestModel_AnnotateKeysBlockedInTreePane(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneTree
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}

	// 'a' in tree pane should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.False(t, model.annotating)
}

func TestModel_CursorViewportY(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
	}

	// no annotations, cursor at 0 -> viewport Y = 0
	m.diffCursor = 0
	assert.Equal(t, 0, m.cursorViewportY())

	// no annotations, cursor at 2 -> viewport Y = 2
	m.diffCursor = 2
	assert.Equal(t, 2, m.cursorViewportY())

	// add annotation on line 1 (index 0), cursor at 2 -> viewport Y = 3 (line0 + annotation + line1)
	m.store.Add("a.go", 1, " ", "comment")
	m.diffCursor = 2
	assert.Equal(t, 3, m.cursorViewportY())
}

func TestModel_DiffLineNum(t *testing.T) {
	assert.Equal(t, 5, diffLineNum(diff.DiffLine{NewNum: 5, ChangeType: diff.ChangeContext}))
	assert.Equal(t, 3, diffLineNum(diff.DiffLine{NewNum: 3, ChangeType: diff.ChangeAdd}))
	assert.Equal(t, 7, diffLineNum(diff.DiffLine{OldNum: 7, ChangeType: diff.ChangeRemove}))
}

func TestModel_NextPrevFileWrapAround(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
	})
	m.tree = newFileTree(files)
	m.currFile = "a.go"

	// move to last file
	m.tree.nextFile()
	assert.Equal(t, "b.go", m.tree.selectedFile())
	m.currFile = "b.go"

	// n should wrap to first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model := result.(Model)
	assert.Equal(t, "a.go", model.tree.selectedFile())

	// p should wrap to last
	model.currFile = "a.go"
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile())
}
