package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

func TestModel_AnnotatedFilesMarker(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	annotated := m.annotatedFiles()
	assert.True(t, annotated["a.go"])
	assert.False(t, annotated["b.go"])
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

func TestModel_EnterInDiffPaneStartsAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1

	// press enter in diff pane - should enter annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annotating, "enter in diff pane should start annotation mode")
	assert.NotNil(t, cmd, "should return textinput blink command")
	assert.Equal(t, paneDiff, model.focus, "focus should remain on diff pane")
}

func TestModel_EnterInDiffPaneScrollsToShowAnnotationInputAtBottom(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "line4", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 4
	m.viewport = viewport.New(100, 3)
	m.viewport.SetContent(m.renderDiff())
	m.viewport.SetYOffset(2) // cursor line (y=4) is the last visible row (2,3,4)

	require.Equal(t, m.viewport.YOffset+m.viewport.Height-1, m.cursorViewportY(),
		"cursor should start on the last visible row")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	require.True(t, model.annotating, "enter should start annotation mode")
	require.NotNil(t, cmd)

	inputY := model.cursorViewportY() + model.wrappedLineCount(model.diffCursor)
	assert.GreaterOrEqual(t, inputY, model.viewport.YOffset, "input row should be within visible viewport")
	assert.Less(t, inputY, model.viewport.YOffset+model.viewport.Height, "input row should be within visible viewport")
	assert.Equal(t, 3, model.viewport.YOffset, "viewport should scroll down by one row to reveal input")
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
	assert.Equal(t, string(diff.ChangeContext), anns[0].Type)
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

func TestModel_AnnotateHunkKeywordSetsEndLine(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx before", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old line", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new line", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added line", ChangeType: diff.ChangeAdd},
		{OldNum: 3, NewNum: 4, Content: "ctx after", ChangeType: diff.ChangeContext},
	}

	t.Run("hunk keyword populates EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 2 // on "new line" (add, NewNum=2)
		m.startAnnotation()
		m.annotateInput.SetValue("refactor this hunk")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 2, anns[0].Line)
		assert.Equal(t, 3, anns[0].EndLine, "EndLine should be last add line's NewNum")
	})

	t.Run("uppercase hunk keyword populates EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 2 // on "new line" (add, NewNum=2)
		m.startAnnotation()
		m.annotateInput.SetValue("refactor this HUNK")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 2, anns[0].Line)
		assert.Equal(t, 3, anns[0].EndLine, "case-insensitive match for HUNK")
	})

	t.Run("block is not a hunk keyword", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 1 // on "old line" (remove, OldNum=2)
		m.startAnnotation()
		m.annotateInput.SetValue("review this BLOCK carefully")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 2, anns[0].Line)
		assert.Equal(t, 0, anns[0].EndLine, "block is not a hunk keyword, no range expansion")
	})

	t.Run("no keyword does not set EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 2
		m.startAnnotation()
		m.annotateInput.SetValue("this is fine")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 0, anns[0].EndLine, "EndLine should be 0 when no hunk keyword")
	})

	t.Run("context line with keyword does not set EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 0 // context line
		m.startAnnotation()
		m.annotateInput.SetValue("rewrite this hunk")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 0, anns[0].EndLine, "EndLine should be 0 for context line even with keyword")
	})
}

func TestModel_AnnotateReAnnotateKeywordChange(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx before", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "new line", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added line", ChangeType: diff.ChangeAdd},
		{OldNum: 2, NewNum: 4, Content: "ctx after", ChangeType: diff.ChangeContext},
	}

	t.Run("add keyword to existing annotation updates EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 1 // on "new line" (add, NewNum=2)
		m.startAnnotation()
		m.annotateInput.SetValue("this is fine")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 0, anns[0].EndLine, "initially no EndLine")

		// re-annotate same line with hunk keyword
		m.startAnnotation()
		m.annotateInput.SetValue("refactor this hunk")
		m.saveAnnotation()
		anns = m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, "refactor this hunk", anns[0].Comment)
		assert.Equal(t, 3, anns[0].EndLine, "EndLine should be set after adding keyword")
	})

	t.Run("remove keyword from existing annotation clears EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = newFileTree([]string{"a.go"})
		m.focus = paneDiff
		m.currFile = "a.go"
		m.diffLines = lines
		m.diffCursor = 1
		m.startAnnotation()
		m.annotateInput.SetValue("refactor this hunk")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 3, anns[0].EndLine, "initially has EndLine")

		// re-annotate same line without keyword
		m.startAnnotation()
		m.annotateInput.SetValue("just a note")
		m.saveAnnotation()
		anns = m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, "just a note", anns[0].Comment)
		assert.Equal(t, 0, anns[0].EndLine, "EndLine should be cleared after removing keyword")
	})
}

func TestModel_AnnotateSingleLineHunkWithKeyword(t *testing.T) {
	// single change line: hunkEndLine returns the same lineNum, so endLine > lineNum is false
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx before", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "single add", ChangeType: diff.ChangeAdd},
		{OldNum: 2, NewNum: 3, Content: "ctx after", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1 // on "single add" (add, NewNum=2)
	m.startAnnotation()
	m.annotateInput.SetValue("refactor this hunk")
	m.saveAnnotation()

	anns := m.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 2, anns[0].Line)
	assert.Equal(t, 0, anns[0].EndLine, "single-line hunk should not set EndLine (avoids 2-2 range)")
}

func TestModel_AnnotateRemovedLineInMixedHunk(t *testing.T) {
	// mixed hunk: 2 removes followed by 2 adds — annotating a remove line with hunk keyword
	// should produce EndLine in the old-file number space, not crossing into the add lines
	lines := []diff.DiffLine{
		{OldNum: 10, NewNum: 10, Content: "ctx before", ChangeType: diff.ChangeContext},
		{OldNum: 11, Content: "removed 1", ChangeType: diff.ChangeRemove},
		{OldNum: 12, Content: "removed 2", ChangeType: diff.ChangeRemove},
		{NewNum: 11, Content: "added 1", ChangeType: diff.ChangeAdd},
		{NewNum: 12, Content: "added 2", ChangeType: diff.ChangeAdd},
		{OldNum: 13, NewNum: 13, Content: "ctx after", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1 // on first remove (OldNum=11)
	m.startAnnotation()
	m.annotateInput.SetValue("fix this hunk")
	m.saveAnnotation()

	anns := m.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 11, anns[0].Line, "start line is OldNum of first remove")
	assert.Equal(t, 12, anns[0].EndLine, "end line is OldNum of last remove, not NewNum of an add")
	assert.Equal(t, string(diff.ChangeRemove), anns[0].Type)
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
	assert.Equal(t, string(diff.ChangeAdd), anns[0].Type)
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
	assert.Equal(t, string(diff.ChangeRemove), anns[0].Type)
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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "old comment"})

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
	m.cursorOnAnnotation = true // cursor on the annotation sub-line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "test comment"})

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
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "needs error handling"})

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

func TestModel_AnnotationCountInStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "other"})
	status := m.statusBarText()
	assert.Contains(t, status, "2 annotations")
}

func TestModel_NoAnnotationCountWhenEmpty(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	status := m.statusBarText()
	assert.NotContains(t, status, "annotations")
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

func TestModel_FileLoadedDiscardsStaleResponse(t *testing.T) {
	// simulate rapid n/n where second load completes first, then stale first response arrives
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// user presses n twice: first for b.go (seq=1), then for c.go (seq=2)
	m.loadSeq = 2
	m.tree.nextFile() // -> b.go
	m.tree.nextFile() // -> c.go

	// c.go response arrives first with latest seq - accepted
	cLines := []diff.DiffLine{{NewNum: 1, Content: "package c", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "c.go", seq: 2, lines: cLines})
	model := result.(Model)
	assert.Equal(t, "c.go", model.currFile)

	// stale b.go response arrives later with old seq - should be discarded
	bLines := []diff.DiffLine{{NewNum: 1, Content: "package b", ChangeType: diff.ChangeContext}}
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: 1, lines: bLines})
	model = result.(Model)
	assert.Equal(t, "c.go", model.currFile, "stale response should not overwrite current file")
	assert.Equal(t, cLines, model.diffLines, "stale response should not overwrite diff lines")
}

func TestModel_FileLoadedStaleErrorDiscarded(t *testing.T) {
	// stale error responses should also be discarded, not overwrite the current diff
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// load a.go successfully (seq=1)
	m.loadSeq = 1
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)

	// user navigates to b.go (seq=2)
	model.loadSeq = 2
	model.tree.nextFile()

	// stale error for a.go arrives with old seq - should be discarded
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, err: errors.New("stale error")})
	model = result.(Model)
	assert.Equal(t, "a.go", model.currFile, "stale error should not change current file")
	assert.Equal(t, aLines, model.diffLines, "stale error should not clear diff lines")
}

func TestModel_SameFileDuplicateLoadDiscarded(t *testing.T) {
	// pressing enter twice on the same file issues two loads for a.go.
	// the older response (seq=1) must be discarded even though it's for the same file.
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)

	// first enter on a.go (seq=1), then another enter on a.go (seq=2)
	m.loadSeq = 2

	// newer response arrives first (seq=2)
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 2, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)
	assert.Equal(t, aLines, model.diffLines)

	// stale response for the same file arrives later with old seq - must be discarded
	staleLines := []diff.DiffLine{{NewNum: 1, Content: "stale data", ChangeType: diff.ChangeContext}}
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: staleLines})
	model = result.(Model)
	assert.Equal(t, aLines, model.diffLines, "stale same-file response should not overwrite newer data")

	// stale error for the same file should also be discarded
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, err: errors.New("stale error")})
	model = result.(Model)
	assert.Equal(t, aLines, model.diffLines, "stale same-file error should not overwrite newer data")
}

func TestModel_FilterRefreshedAfterAnnotationSave(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "initial annotation"})

	// enable filter - should show only a.go
	annotated := m.annotatedFiles()
	m.tree.toggleFilter(annotated)
	assert.True(t, m.tree.filter)

	fileCount := 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 1, fileCount, "only a.go should be visible")

	// add annotation to b.go via saveAnnotation
	m.currFile = "b.go"
	m.diffLines = []diff.DiffLine{{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext}}
	m.diffCursor = 0
	m.startAnnotation()
	m.annotateInput.SetValue("new annotation")
	m.saveAnnotation()

	// after save, filter should be refreshed and b.go should be visible
	fileCount = 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount, "both a.go and b.go should be visible after adding annotation")
}

func TestModel_FilterRefreshedAfterAnnotationDelete(t *testing.T) {
	files := []string{"a.go", "b.go"}
	diffs := map[string][]diff.DiffLine{
		"b.go": {{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext}},
	}
	m := testModel(files, diffs)
	m.tree = newFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "annotation on a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "annotation on b"})

	// enable filter - should show both annotated files
	annotated := m.annotatedFiles()
	m.tree.toggleFilter(annotated)
	assert.True(t, m.tree.filter)

	// delete the annotation on a.go via deleteAnnotation
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, OldNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.diffCursor = 0
	m.cursorOnAnnotation = true
	cmd := m.deleteAnnotation()

	// after delete, filter should be refreshed and only b.go should be visible
	fileCount := 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 1, fileCount, "only b.go should be visible after deleting a.go annotation")

	// should return a command to load the new selection (b.go)
	require.NotNil(t, cmd, "should trigger file load for new tree selection")
	assert.Equal(t, uint64(1), m.loadSeq, "loadSeq should be incremented to invalidate in-flight loads")
}

func TestModel_FilterDisabledWhenLastAnnotationDeleted(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = newFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "only annotation"})

	// enable filter
	annotated := m.annotatedFiles()
	m.tree.toggleFilter(annotated)
	assert.True(t, m.tree.filter)

	// delete the last annotation
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, OldNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.diffCursor = 0
	m.cursorOnAnnotation = true
	cmd := m.deleteAnnotation()

	// filter should be disabled since no annotated files remain
	assert.False(t, m.tree.filter, "filter should be disabled when no annotated files remain")

	fileCount := 0
	for _, e := range m.tree.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount, "all files should be visible")

	// when filter switches back to all-files, cursor lands on a.go (first file) which matches currFile,
	// so no file load command is needed
	assert.Nil(t, cmd, "no file load needed when filter switches back to all-files and cursor stays on same file")
}

func TestModel_AnnotationsPersistAcrossFileSwitch(t *testing.T) {
	linesA := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}, {NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd}}
	linesB := []diff.DiffLine{{NewNum: 1, Content: "b-line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": linesA, "b.go": linesB})
	m.tree = newFileTree([]string{"a.go", "b.go"})

	// load file a.go
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: linesA})
	model := result.(Model)
	assert.Equal(t, "a.go", model.currFile)

	// add annotation on a.go
	model.focus = paneDiff
	model.diffCursor = 1
	model.startAnnotation()
	model.annotateInput.SetValue("fix this in a.go")
	model.saveAnnotation()

	// navigate tree to b.go and load it
	model.tree.nextFile()
	assert.Equal(t, "b.go", model.tree.selectedFile())
	result, _ = model.Update(fileLoadedMsg{file: "b.go", lines: linesB})
	model = result.(Model)
	assert.Equal(t, "b.go", model.currFile)

	// add annotation on b.go
	model.focus = paneDiff
	model.diffCursor = 0
	model.startAnnotation()
	model.annotateInput.SetValue("check b.go")
	model.saveAnnotation()

	// navigate tree back to a.go and load it
	model.tree.prevFile()
	assert.Equal(t, "a.go", model.tree.selectedFile())
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: linesA})
	model = result.(Model)

	// both annotations should still exist
	annsA := model.store.Get("a.go")
	require.Len(t, annsA, 1)
	assert.Equal(t, "fix this in a.go", annsA[0].Comment)

	annsB := model.store.Get("b.go")
	require.Len(t, annsB, 1)
	assert.Equal(t, "check b.go", annsB[0].Comment)

	// rendered diff for a.go should show annotation
	rendered := model.renderDiff()
	assert.Contains(t, rendered, "fix this in a.go")
}

func TestModel_AnnotateInputEchoesCharacters(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// type characters one at a time and verify each appears in viewport
	for _, ch := range []rune{'h', 'e', 'l', 'l', 'o'} {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)

		// verify the typed text so far is visible in the viewport content
		vpContent := model.viewport.View()
		typed := model.annotateInput.Value()
		assert.Contains(t, vpContent, typed, "viewport should contain typed text %q after keystroke %q", typed, string(ch))
	}

	// final check: all characters are visible
	assert.Equal(t, "hello", model.annotateInput.Value())
	assert.Contains(t, model.viewport.View(), "hello")
}

func TestModel_AnnotateInputVisibleBeforeEnter(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// initialize viewport via resize
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// load file
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// type some text
	for _, ch := range []rune{'t', 'e', 's', 't'} {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	// verify text is in viewport content before pressing Enter
	assert.True(t, model.annotating, "should still be in annotation mode")
	vpContent := model.viewport.View()
	assert.Contains(t, vpContent, "test", "typed text should be visible in viewport before Enter")

	// also verify via renderDiff (which is what SetContent uses)
	rendered := model.renderDiff()
	assert.Contains(t, rendered, "test", "typed text should be in rendered diff before Enter")

	// also verify the annotation input emoji marker is visible
	assert.Contains(t, rendered, "\U0001f4ac", "annotation marker should be visible before Enter")
}

func TestModel_UpdateForwardsNonKeyMsgWhileAnnotating(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// send a non-key message (e.g. a custom struct); should not panic and model stays annotating
	type customMsg struct{}
	result, _ = model.Update(customMsg{})
	model = result.(Model)
	assert.True(t, model.annotating, "annotating should remain true after non-key message")
}

func TestModel_AnnotateRenderWithDividers(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
		{OldNum: 11, Content: "removed", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, nil)
	m.currFile = "a.go"
	m.diffLines = lines

	// add annotations on different change types
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "addition comment"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 11, Type: "-", Comment: "removal comment"})

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "addition comment")
	assert.Contains(t, rendered, "removal comment")
	assert.Contains(t, rendered, "...")
}

func TestModel_PgDownAccountsForAnnotations(t *testing.T) {
	lines := make([]diff.DiffLine, 50)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeAdd}
	}

	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// add annotations on several lines - each takes an extra visual row
	for i := range 10 {
		model.store.Add(annotation.Annotation{File: "a.go", Line: i + 1, Type: string(diff.ChangeAdd), Comment: "annotation"})
	}

	pageHeight := model.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown with annotations should move fewer cursor positions than viewport height
	// because annotation rows and annotation sub-lines take visual space
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := result.(Model)
	// cursor position or annotation flag must have changed
	assert.True(t, m2.diffCursor > 0 || m2.cursorOnAnnotation,
		"cursor should have moved forward (position or onto annotation)")
	assert.Less(t, m2.diffCursor, pageHeight,
		"with annotations, cursor should move fewer positions than viewport height")
}

func TestModel_ShiftAStartsFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.True(t, model.annotating, "A should start annotation mode")
	assert.True(t, model.fileAnnotating, "A should set fileAnnotating=true")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_AnnotationInputWidthNarrowTerminal(t *testing.T) {
	// very narrow terminal should not produce negative textinput width
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.focus = paneDiff
	m.width = 20
	m.treeWidth = 20 // width - treeWidth - 10 = -10 without the guard

	// line-level annotation
	cmd := m.startAnnotation()
	assert.NotNil(t, cmd)
	assert.True(t, m.annotating)
	assert.GreaterOrEqual(t, m.annotateInput.Width, 10, "text input width should be at least 10")

	// file-level annotation
	m.annotating = false
	cmd = m.startFileAnnotation()
	assert.NotNil(t, cmd)
	assert.True(t, m.fileAnnotating)
	assert.GreaterOrEqual(t, m.annotateInput.Width, 10, "file text input width should be at least 10")
}

func TestModel_FileAnnotationInputWidthNarrowerThanLineLevel(t *testing.T) {
	// file-level annotation has wider prefix ("💬 file: " vs "💬 "), so input should be narrower
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.focus = paneDiff
	m.width = 120
	m.treeWidth = 30
	m.treeHidden = false

	// line-level annotation
	m.startAnnotation()
	lineWidth := m.annotateInput.Width

	// file-level annotation
	m.annotating = false
	m.startFileAnnotation()
	fileWidth := m.annotateInput.Width

	assert.Greater(t, lineWidth, fileWidth, "file-level input should be narrower than line-level due to wider prefix")
	assert.Equal(t, 6, lineWidth-fileWidth, "width difference should match prefix width difference (12-6=6)")
}

func TestModel_FileAnnotationSavesWithLineZero(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff

	// start file-level annotation and set text
	m.startFileAnnotation()
	m.annotateInput.SetValue("file-level comment")

	// save via Enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annotating)
	assert.False(t, model.fileAnnotating)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 0, anns[0].Line, "file-level annotation should have Line=0")
	assert.Empty(t, anns[0].Type, "file-level annotation should have empty Type")
	assert.Equal(t, "file-level comment", anns[0].Comment)
}

func TestModel_FileAnnotationPreFillsExisting(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing file note"})

	m.startFileAnnotation()
	assert.Equal(t, "existing file note", m.annotateInput.Value(), "should pre-fill with existing file-level annotation")
}

func TestModel_FileAnnotationCancelResetsFlags(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff

	m.startFileAnnotation()
	assert.True(t, m.annotating)
	assert.True(t, m.fileAnnotating)

	// press Esc to cancel
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.annotating, "cancel should reset annotating")
	assert.False(t, model.fileAnnotating, "cancel should reset fileAnnotating")
}

func TestModel_FileAnnotationRenderedAtTopOfDiff(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "this is a file note"})

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "file: this is a file note", "file-level annotation should appear in rendered diff")
	assert.Contains(t, rendered, "\U0001f4ac", "file-level annotation should have speech bubble emoji")

	// file annotation should appear before any line content
	fileIdx := strings.Index(rendered, "file: this is a file note")
	lineIdx := strings.Index(rendered, "package main")
	assert.Less(t, fileIdx, lineIdx, "file-level annotation should appear before diff lines")
}

func TestModel_FileAnnotationCursorHighlighted(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.focus = paneDiff
	m.diffCursor = -1 // on file annotation line

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "file: file note", "file annotation should be rendered")
}

func TestModel_EnterOnFileAnnotationLineTriggersFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = -1 // on file annotation line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing note"})

	// press enter on file annotation line - should start file annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annotating, "enter on file annotation line should start annotation mode")
	assert.True(t, model.fileAnnotating, "enter on file annotation line should set fileAnnotating")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_EnterOnFileAnnotationLinePreFillsText(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = -1 // on file annotation line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "pre-existing comment"})

	// press enter - should pre-fill with existing annotation text
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.Equal(t, "pre-existing comment", model.annotateInput.Value(), "should pre-fill with existing file annotation")
}

func TestModel_EnterOnRegularDiffLineStillTriggersLineAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = 1 // on regular diff line, not file annotation
	// add a file annotation to ensure it doesn't interfere
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	// press enter on regular line - should start line annotation, not file annotation
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annotating, "enter on regular line should start annotation mode")
	assert.False(t, model.fileAnnotating, "enter on regular line should not set fileAnnotating")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_DeleteFileAnnotationViaD(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note to delete"})
	m.diffCursor = -1 // on file annotation line

	// press 'd' to delete file-level annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"), "file-level annotation should be deleted")
	assert.GreaterOrEqual(t, model.diffCursor, 0, "cursor should move to first valid diff line after deletion")
}

func TestModel_DeleteFileAnnotationCursorNotOnFileLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "line note"})
	m.diffCursor = 0            // on regular line
	m.cursorOnAnnotation = true // on the annotation sub-line for line 1

	// press 'd' should delete the line annotation, not the file annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "should only delete the line annotation")
	assert.Equal(t, 0, anns[0].Line, "file-level annotation should remain")
}

func TestModel_DeleteFileAnnotationFilterShiftsSelection(t *testing.T) {
	// when filter is active and deleting a file-level annotation removes the only annotation
	// for that file, the filter rebuilds entries and tree selects a different file
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines, "b.go": lines})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.focus = paneDiff
	m.currFile = "a.go"
	m.diffLines = lines
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note on a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 1, Type: " ", Comment: "line note on b"})
	m.diffCursor = -1 // on file annotation line

	// enable filter to show only annotated files
	m.tree.toggleFilter(m.annotatedFiles())
	require.True(t, m.tree.filter)

	// press 'd' to delete file-level annotation on a.go
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)

	// a.go no longer has annotations, filter should shift selection to b.go
	assert.Empty(t, model.store.Get("a.go"), "file-level annotation should be deleted")
	assert.NotNil(t, cmd, "should return a command to load the new file")
}

func TestModel_CursorLineHasAnnotationExcludesFileLevel(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.diffCursor = 0 // on line 1, not on file annotation
	m.focus = paneDiff

	// line 1 has no line-level annotation, only file-level exists
	assert.False(t, m.cursorLineHasAnnotation(), "should not report file-level annotation as line annotation")
}

func TestModel_CursorOnFileAnnotationLineReportsAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.diffCursor = -1

	assert.True(t, m.cursorLineHasAnnotation(), "cursor on file annotation line should report annotation")
}

func TestModel_CursorNavigatesToFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	// move up from first line should go to file annotation
	m.moveDiffCursorUp()
	assert.Equal(t, -1, m.diffCursor, "cursor should move to file annotation line (-1)")

	// move down from file annotation should go to first non-divider line
	m.moveDiffCursorDown()
	assert.Equal(t, 0, m.diffCursor, "cursor should move from file annotation to first line")
}

func TestModel_HomeGoesToFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneDiff
	m.diffCursor = 1
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.ready = true

	m.moveDiffCursorToStart()
	assert.Equal(t, -1, m.diffCursor, "Home should move to file annotation line when it exists")
}

func TestModel_CursorViewportYWithFileAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	// cursor on file annotation line
	m.diffCursor = -1
	assert.Equal(t, 0, m.cursorViewportY(), "file annotation line should be at viewport Y=0")

	// cursor on first diff line should be at Y=1 (file annotation occupies Y=0)
	m.diffCursor = 0
	assert.Equal(t, 1, m.cursorViewportY(), "first diff line should be at Y=1 when file annotation exists")

	// cursor on second diff line
	m.diffCursor = 1
	assert.Equal(t, 2, m.cursorViewportY(), "second diff line should be at Y=2 when file annotation exists")
}

func TestModel_CursorViewportYWithWrappedAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.width = 60
	m.treeWidth = 20
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	t.Run("long file annotation wraps to multiple rows", func(t *testing.T) {
		longComment := strings.Repeat("word ", 20) // ~100 chars, wraps at ~34
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})
		defer m.store.Delete("a.go", 0, "")

		wrapCount := m.wrappedAnnotationLineCount(annotKeyFile)
		assert.Greater(t, wrapCount, 1, "long file annotation should wrap to multiple rows")

		m.diffCursor = 0
		assert.Equal(t, wrapCount, m.cursorViewportY(), "first diff line offset should equal wrap count")

		m.diffCursor = 1
		assert.Equal(t, wrapCount+1, m.cursorViewportY(), "second diff line offset should be wrap count + 1")
	})

	t.Run("long inline annotation wraps to multiple rows", func(t *testing.T) {
		longComment := strings.Repeat("note ", 20) // ~100 chars
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: longComment})
		defer m.store.Delete("a.go", 1, " ")

		key := m.annotationKey(1, " ")
		wrapCount := m.wrappedAnnotationLineCount(key)
		assert.Greater(t, wrapCount, 1, "long inline annotation should wrap to multiple rows")

		m.diffCursor = 1
		assert.Equal(t, 1+wrapCount, m.cursorViewportY(), "second line should offset by annotation wrap count")
	})

	t.Run("short annotation stays one row", func(t *testing.T) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "short"})
		defer m.store.Delete("a.go", 0, "")

		assert.Equal(t, 1, m.wrappedAnnotationLineCount(annotKeyFile))
	})
}

func TestModel_RenderWrappedAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.width = 60
	m.treeWidth = 20
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff

	t.Run("long file annotation wraps in rendered output", func(t *testing.T) {
		longComment := strings.Repeat("wrap ", 20)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})
		defer m.store.Delete("a.go", 0, "")

		wrapCount := m.wrappedAnnotationLineCount(annotKeyFile)
		assert.Greater(t, wrapCount, 1, "annotation should wrap")
		// cursor chevron appears exactly once (first line only)
		m.diffCursor = -1
		rendered := m.renderDiff()
		assert.Equal(t, 1, strings.Count(rendered, "▶"), "cursor on first wrap line only")
	})
}

func TestModel_RenderDiffFileAnnotationInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.focus = paneDiff

	// start file annotation and set text
	m.startFileAnnotation()
	m.annotateInput.SetValue("typing file note...")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "typing file note...", "file annotation input should be visible in rendered diff")
	assert.Contains(t, rendered, "file:", "should show file: prefix during input")
}

func TestModel_FileAnnotationExcludedFromRegularAnnotationMap(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "line note"})

	set := m.buildAnnotationSet()
	assert.Len(t, set, 1, "buildAnnotationSet should exclude file-level annotations")
	assert.True(t, set["1: "], "line annotation should be in set")
}

func TestModel_StartAnnotationResetsFileAnnotating(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.diffCursor = 0
	m.focus = paneDiff

	// starting a regular annotation should set fileAnnotating to false
	m.startAnnotation()
	assert.True(t, m.annotating)
	assert.False(t, m.fileAnnotating, "startAnnotation should set fileAnnotating=false")
}

func TestModel_EditExistingFileAnnotationShowsInput(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.focus = paneDiff
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing note"})

	// start editing the existing file-level annotation
	m.startFileAnnotation()
	assert.True(t, m.annotating)
	assert.True(t, m.fileAnnotating)
	assert.Equal(t, "existing note", m.annotateInput.Value(), "input should be pre-filled")

	// render should show the text input, not the static annotation
	rendered := m.renderDiff()
	assert.Contains(t, rendered, "existing note", "input with pre-filled text should be visible")
	assert.Contains(t, rendered, "file:", "should show file: prefix during input")
}

func TestModel_DiscardedAccessor(t *testing.T) {
	t.Run("default is false", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		assert.False(t, m.Discarded())
	})

	t.Run("true when set", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.discarded = true
		assert.True(t, m.Discarded())
	})
}

func TestModel_QKeyDiscardNoAnnotations(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "should be discarded when no annotations")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit")
}

func TestModel_QKeyWithAnnotationsEntersConfirming(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	assert.Nil(t, cmd, "should not quit yet")

	model := result.(Model)
	assert.True(t, model.inConfirmDiscard, "should enter confirming state")
	assert.False(t, model.Discarded(), "should not be discarded yet")
}

func TestModel_QKeyDuringAnnotationIgnored(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})

	// load file
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annotating)

	// press Q - should be handled as text input, not discard
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	model = result.(Model)
	assert.True(t, model.annotating, "should still be annotating")
	assert.False(t, model.Discarded(), "should not be discarded")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming")
	assert.Contains(t, model.annotateInput.Value(), "Q", "Q should be typed into input")
}

func TestModel_StatusBarDiscardConfirmation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "other"})
	m.inConfirmDiscard = true

	status := m.statusBarText()
	assert.Equal(t, "discard 2 annotations? [y/n]", status)
}

func TestModel_SingleFileAnnotationWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.singleFile = true
	model.focus = paneDiff
	model.diffCursor = 1 // on the add line
	res := style.PlainResolver()
	model.resolver = res
	model.renderer = style.NewRenderer(res)

	// press enter to start annotation
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.True(t, model.annotating, "enter should start annotation in single-file mode")

	// type annotation text
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = result.(Model)

	// press enter to save
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.False(t, model.annotating, "annotation should be saved")
	assert.Equal(t, 1, model.store.Count(), "annotation should be stored")
}

func TestModel_AnnotationsWithTOCActive(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "some text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Section", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "more text", ChangeType: diff.ChangeContext},
	}

	t.Run("annotate line in diff pane with TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneDiff
		model.diffCursor = 1 // on "some text" line

		// press 'a' to start annotation
		result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		model = result.(Model)
		assert.True(t, model.annotating, "should enter annotation mode in diff pane with TOC")
		assert.NotNil(t, cmd)
	})

	t.Run("file annotation with TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneDiff

		// press 'A' for file annotation
		result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
		model = result.(Model)
		assert.True(t, model.annotating, "should enter file annotation mode with TOC")
		assert.True(t, model.fileAnnotating, "should be file-level annotation")
		assert.NotNil(t, cmd)
	})

	t.Run("annotation list with TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneDiff
		model.diffCursor = 1

		// add an annotation first
		model.store.Add(annotation.Annotation{File: "README.md", Line: 2, Type: " ", Comment: "test annotation"})

		// press '@' to open annotation list
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
		model = result.(Model)
		assert.True(t, model.showAnnotList, "annotation list should open with TOC active")
	})

	t.Run("annotation keys blocked in TOC pane", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneTree // TOC pane

		// press 'a' in TOC pane - should not start annotation
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		model = result.(Model)
		assert.False(t, model.annotating, "annotation should not start from TOC pane")
	})
}

func TestModel_ShiftAIgnoredWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = ""
	m.focus = paneTree

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.False(t, model.annotating, "A without currFile should not start annotation")
	assert.Nil(t, cmd)
}

func TestModel_ShiftAOnlyWorksFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}

	// from tree pane — should be ignored to avoid annotating wrong file
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.focus = paneTree
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.False(t, model.annotating, "A from tree pane should not start annotation")
	assert.False(t, model.fileAnnotating)
	assert.Nil(t, cmd)

	// from diff pane — should work
	m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m2.tree = newFileTree([]string{"a.go"})
	m2.currFile = "a.go"
	m2.diffLines = lines
	m2.focus = paneDiff
	result, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model = result.(Model)
	assert.True(t, model.annotating, "A should work from diff pane")
	assert.True(t, model.fileAnnotating)
	assert.NotNil(t, cmd)
}
