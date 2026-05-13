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
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

func TestModel_AnnotatedFilesMarker(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
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
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	// press 'a' - should enter annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.True(t, model.annot.annotating)
	assert.NotNil(t, cmd) // textinput blink command
}

func TestModel_EnterInDiffPaneStartsAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1

	// press enter in diff pane - should enter annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annot.annotating, "enter in diff pane should start annotation mode")
	assert.NotNil(t, cmd, "should return textinput blink command")
	assert.Equal(t, paneDiff, model.layout.focus, "focus should remain on diff pane")
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
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 4
	m.layout.viewport = viewport.New(100, 3)
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(2) // cursor line (y=4) is the last visible row (2,3,4)

	require.Equal(t, m.layout.viewport.YOffset+m.layout.viewport.Height-1, m.cursorViewportY(),
		"cursor should start on the last visible row")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	require.True(t, model.annot.annotating, "enter should start annotation mode")
	require.NotNil(t, cmd)

	inputY := model.cursorViewportY() + model.wrappedLineCount(model.nav.diffCursor)
	assert.GreaterOrEqual(t, inputY, model.layout.viewport.YOffset, "input row should be within visible viewport")
	assert.Less(t, inputY, model.layout.viewport.YOffset+model.layout.viewport.Height, "input row should be within visible viewport")
	assert.Equal(t, 3, model.layout.viewport.YOffset, "viewport should scroll down by one row to reveal input")
}

func TestModel_AnnotateEnterSaves(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	// enter annotation mode and set text
	m.startAnnotation()
	m.annot.input.SetValue("test comment")

	// press Enter - should save
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annot.annotating)

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
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	// don't set any text, press Enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annot.annotating)
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
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 2 // on "new line" (add, NewNum=2)
		m.startAnnotation()
		m.annot.input.SetValue("refactor this hunk")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 2, anns[0].Line)
		assert.Equal(t, 3, anns[0].EndLine, "EndLine should be last add line's NewNum")
	})

	t.Run("uppercase hunk keyword populates EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 2 // on "new line" (add, NewNum=2)
		m.startAnnotation()
		m.annot.input.SetValue("refactor this HUNK")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 2, anns[0].Line)
		assert.Equal(t, 3, anns[0].EndLine, "case-insensitive match for HUNK")
	})

	t.Run("block is not a hunk keyword", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 1 // on "old line" (remove, OldNum=2)
		m.startAnnotation()
		m.annot.input.SetValue("review this BLOCK carefully")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 2, anns[0].Line)
		assert.Equal(t, 0, anns[0].EndLine, "block is not a hunk keyword, no range expansion")
	})

	t.Run("no keyword does not set EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 2
		m.startAnnotation()
		m.annot.input.SetValue("this is fine")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 0, anns[0].EndLine, "EndLine should be 0 when no hunk keyword")
	})

	t.Run("context line with keyword does not set EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 0 // context line
		m.startAnnotation()
		m.annot.input.SetValue("rewrite this hunk")
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
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 1 // on "new line" (add, NewNum=2)
		m.startAnnotation()
		m.annot.input.SetValue("this is fine")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 0, anns[0].EndLine, "initially no EndLine")

		// re-annotate same line with hunk keyword
		m.startAnnotation()
		m.annot.input.SetValue("refactor this hunk")
		m.saveAnnotation()
		anns = m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, "refactor this hunk", anns[0].Comment)
		assert.Equal(t, 3, anns[0].EndLine, "EndLine should be set after adding keyword")
	})

	t.Run("remove keyword from existing annotation clears EndLine", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.layout.focus = paneDiff
		m.file.name = "a.go"
		m.file.lines = lines
		m.nav.diffCursor = 1
		m.startAnnotation()
		m.annot.input.SetValue("refactor this hunk")
		m.saveAnnotation()
		anns := m.store.Get("a.go")
		require.Len(t, anns, 1)
		assert.Equal(t, 3, anns[0].EndLine, "initially has EndLine")

		// re-annotate same line without keyword
		m.startAnnotation()
		m.annot.input.SetValue("just a note")
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
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1 // on "single add" (add, NewNum=2)
	m.startAnnotation()
	m.annot.input.SetValue("refactor this hunk")
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
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1 // on first remove (OldNum=11)
	m.startAnnotation()
	m.annot.input.SetValue("fix this hunk")
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
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	m.annot.input.SetValue("should not be saved")

	// press Esc - should cancel without saving
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.annot.annotating)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_AnnotateOnDividerIgnored(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	// press 'a' on divider - should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.False(t, model.annot.annotating)
}

func TestModel_AnnotateOnAddLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 3, Content: "new line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	m.annot.input.SetValue("needs review")

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
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	m.annot.input.SetValue("why removed?")

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
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "old comment"})

	m.startAnnotation()
	assert.Equal(t, "old comment", m.annot.input.Value())
}

func TestModel_DeleteAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.annot.cursorOnAnnotation = true // cursor on the annotation sub-line
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
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	// press 'd' on divider - should not panic
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	_ = result.(Model)
}

func TestModel_DeleteAnnotationNoAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	// no annotation exists, 'd' should be harmless
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"))
}

func TestModel_RenderDiffWithAnnotations(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "needs error handling"})

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "needs error handling")
	assert.Contains(t, rendered, "\U0001f4ac")
}

func TestModel_CustomAnnotationMarker(t *testing.T) {
	res := style.PlainResolver()
	m, err := NewModel(ModelConfig{
		Renderer:         plainRenderer(),
		Store:            annotation.NewStore(),
		Highlighter:      noopHighlighter(),
		StyleResolver:    res,
		StyleRenderer:    style.NewRenderer(res),
		SGR:              style.SGR{},
		WordDiffer:       worddiff.New(),
		Overlay:          overlay.NewManager(),
		Themes:           fakeThemeCatalog{},
		TreeWidthRatio:   3,
		AnnotationMarker: "▸",
		NewFileTree:      testFileTreeFactory(),
		ParseTOC:         testParseTOCFactory(),
	})
	require.NoError(t, err)
	m.layout.width = 120
	m.layout.height = 40
	m.layout.treeWidth = m.layout.width * m.cfg.treeWidthRatio / 10
	m.ready = true
	m.filesLoaded = true

	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "▸ note", "line annotation should use custom marker")
	assert.Contains(t, rendered, "▸ file: file note", "file annotation should use custom marker")
	assert.NotContains(t, rendered, "\U0001f4ac", "default emoji should not appear with custom marker")
}

func TestModel_EmptyAnnotationMarkerExplicit(t *testing.T) {
	res := style.PlainResolver()
	m, err := NewModel(ModelConfig{
		Renderer:         plainRenderer(),
		Store:            annotation.NewStore(),
		Highlighter:      noopHighlighter(),
		StyleResolver:    res,
		StyleRenderer:    style.NewRenderer(res),
		SGR:              style.SGR{},
		WordDiffer:       worddiff.New(),
		Overlay:          overlay.NewManager(),
		Themes:           fakeThemeCatalog{},
		TreeWidthRatio:   3,
		AnnotationMarker: "",
		NewFileTree:      testFileTreeFactory(),
		ParseTOC:         testParseTOCFactory(),
	})
	require.NoError(t, err)
	m.layout.width = 120
	m.layout.height = 40
	m.layout.treeWidth = m.layout.width * m.cfg.treeWidthRatio / 10
	m.ready = true
	m.filesLoaded = true

	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "bare note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	rendered := m.renderDiff()
	// verify empty marker produces bare space prefix, not emoji fallback
	assert.NotContains(t, rendered, "\U0001f4ac", "empty marker should not produce emoji")
	assert.NotContains(t, rendered, "\U0001f4ac bare note", "should not have emoji before line annotation")
	assert.NotContains(t, rendered, "\U0001f4ac file:", "should not have emoji before file annotation")
	assert.Contains(t, rendered, " bare note", "empty marker should render bare prefix for line annotation")
	assert.Contains(t, rendered, " file: file note", "empty marker should render ' file: ' prefix for file annotation")
}

func TestModel_RenderDiffAnnotationInput(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.nav.diffCursor = 0
	m.layout.focus = paneDiff
	m.startAnnotation()
	m.annot.input.SetValue("typing...")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "typing...")
	assert.Contains(t, rendered, "\U0001f4ac")
}

func TestModel_AnnotationCountInStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.layout.width = 120
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "other"})
	status := m.statusBarText()
	assert.Contains(t, status, "2 annotations")
}

func TestModel_NoAnnotationCountWhenEmpty(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.layout.width = 120
	status := m.statusBarText()
	assert.NotContains(t, status, "annotations")
}

func TestModel_AnnotateStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.ready = true
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.layout.focus = paneDiff
	m.annot.annotating = true

	view := m.View()
	assert.Contains(t, view, "save")
	assert.Contains(t, view, "cancel")
	assert.NotContains(t, view, "annotate")
}

func TestModel_AnnotateKeysBlockedInTreePane(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneTree
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}

	// 'a' in tree pane should not enter annotation mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model := result.(Model)
	assert.False(t, model.annot.annotating)
}

func TestModel_FileLoadedDiscardsStaleResponse(t *testing.T) {
	// simulate rapid n/n where second load completes first, then stale first response arrives
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)

	// user presses n twice: first for b.go (seq=1), then for c.go (seq=2)
	m.file.loadSeq = 2
	m.tree.StepFile(sidepane.DirectionNext) // -> b.go
	m.tree.StepFile(sidepane.DirectionNext) // -> c.go

	// c.go response arrives first with latest seq - accepted
	cLines := []diff.DiffLine{{NewNum: 1, Content: "package c", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "c.go", seq: 2, lines: cLines})
	model := result.(Model)
	assert.Equal(t, "c.go", model.file.name)

	// stale b.go response arrives later with old seq - should be discarded
	bLines := []diff.DiffLine{{NewNum: 1, Content: "package b", ChangeType: diff.ChangeContext}}
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: 1, lines: bLines})
	model = result.(Model)
	assert.Equal(t, "c.go", model.file.name, "stale response should not overwrite current file")
	assert.Equal(t, cLines, model.file.lines, "stale response should not overwrite diff lines")
}

func TestModel_FileLoadedStaleErrorDiscarded(t *testing.T) {
	// stale error responses should also be discarded, not overwrite the current diff
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)

	// load a.go successfully (seq=1)
	m.file.loadSeq = 1
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.file.name)

	// user navigates to b.go (seq=2)
	model.file.loadSeq = 2
	model.tree.StepFile(sidepane.DirectionNext)

	// stale error for a.go arrives with old seq - should be discarded
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, err: errors.New("stale error")})
	model = result.(Model)
	assert.Equal(t, "a.go", model.file.name, "stale error should not change current file")
	assert.Equal(t, aLines, model.file.lines, "stale error should not clear diff lines")
}

func TestModel_SameFileDuplicateLoadDiscarded(t *testing.T) {
	// pressing enter twice on the same file issues two loads for a.go.
	// the older response (seq=1) must be discarded even though it's for the same file.
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)

	// first enter on a.go (seq=1), then another enter on a.go (seq=2)
	m.file.loadSeq = 2

	// newer response arrives first (seq=2)
	aLines := []diff.DiffLine{{NewNum: 1, Content: "package a", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 2, lines: aLines})
	model := result.(Model)
	assert.Equal(t, "a.go", model.file.name)
	assert.Equal(t, aLines, model.file.lines)

	// stale response for the same file arrives later with old seq - must be discarded
	staleLines := []diff.DiffLine{{NewNum: 1, Content: "stale data", ChangeType: diff.ChangeContext}}
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: staleLines})
	model = result.(Model)
	assert.Equal(t, aLines, model.file.lines, "stale same-file response should not overwrite newer data")

	// stale error for the same file should also be discarded
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: 1, err: errors.New("stale error")})
	model = result.(Model)
	assert.Equal(t, aLines, model.file.lines, "stale same-file error should not overwrite newer data")
}

func TestModel_FilterRefreshedAfterAnnotationSave(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "initial annotation"})

	// enable filter - should show only a.go
	annotated := m.annotatedFiles()
	m.tree.ToggleFilter(annotated)
	assert.True(t, m.tree.FilterActive())

	// only a.go should be visible
	assert.Equal(t, "a.go", m.tree.SelectedFile())
	assert.False(t, m.tree.HasFile(sidepane.DirectionNext), "only a.go should be visible")

	// add annotation to b.go via saveAnnotation
	m.file.name = "b.go"
	m.file.lines = []diff.DiffLine{{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext}}
	m.nav.diffCursor = 0
	m.startAnnotation()
	m.annot.input.SetValue("new annotation")
	m.saveAnnotation()

	// after save, filter should be refreshed and b.go should be visible
	assert.True(t, m.tree.HasFile(sidepane.DirectionNext) || m.tree.HasFile(sidepane.DirectionPrev),
		"both a.go and b.go should be visible after adding annotation")
}

func TestModel_FilterRefreshedAfterAnnotationDelete(t *testing.T) {
	files := []string{"a.go", "b.go"}
	diffs := map[string][]diff.DiffLine{
		"b.go": {{NewNum: 5, Content: "line5", ChangeType: diff.ChangeContext}},
	}
	m := testModel(files, diffs)
	m.tree = testNewFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "annotation on a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 5, Type: " ", Comment: "annotation on b"})

	// enable filter - should show both annotated files
	annotated := m.annotatedFiles()
	m.tree.ToggleFilter(annotated)
	assert.True(t, m.tree.FilterActive())

	// delete the annotation on a.go via deleteAnnotation
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, OldNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.nav.diffCursor = 0
	m.annot.cursorOnAnnotation = true
	cmd := m.deleteAnnotation()

	// after delete, filter should be refreshed and only b.go should be visible
	assert.Equal(t, "b.go", m.tree.SelectedFile(), "only b.go should be visible after deleting a.go annotation")
	assert.False(t, m.tree.HasFile(sidepane.DirectionNext), "only one file should be visible")

	// should return a command to load the new selection (b.go)
	require.NotNil(t, cmd, "should trigger file load for new tree selection")
	assert.Equal(t, uint64(1), m.file.loadSeq, "loadSeq should be incremented to invalidate in-flight loads")
}

func TestModel_FilterDisabledWhenLastAnnotationDeleted(t *testing.T) {
	files := []string{"a.go", "b.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "only annotation"})

	// enable filter
	annotated := m.annotatedFiles()
	m.tree.ToggleFilter(annotated)
	assert.True(t, m.tree.FilterActive())

	// delete the last annotation
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, OldNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.nav.diffCursor = 0
	m.annot.cursorOnAnnotation = true
	cmd := m.deleteAnnotation()

	// filter should be disabled since no annotated files remain
	assert.False(t, m.tree.FilterActive(), "filter should be disabled when no annotated files remain")

	// all files should be visible
	assert.Equal(t, 2, m.tree.TotalFiles(), "all files should be visible")

	// when filter switches back to all-files, cursor lands on a.go (first file) which matches currFile,
	// so no file load command is needed
	assert.Nil(t, cmd, "no file load needed when filter switches back to all-files and cursor stays on same file")
}

func TestModel_AnnotationsPersistAcrossFileSwitch(t *testing.T) {
	linesA := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}, {NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd}}
	linesB := []diff.DiffLine{{NewNum: 1, Content: "b-line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": linesA, "b.go": linesB})
	m.tree = testNewFileTree([]string{"a.go", "b.go"})

	// load file a.go
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: linesA})
	model := result.(Model)
	assert.Equal(t, "a.go", model.file.name)

	// add annotation on a.go
	model.layout.focus = paneDiff
	model.nav.diffCursor = 1
	model.startAnnotation()
	model.annot.input.SetValue("fix this in a.go")
	model.saveAnnotation()

	// navigate tree to b.go and load it
	model.tree.StepFile(sidepane.DirectionNext)
	assert.Equal(t, "b.go", model.tree.SelectedFile())
	result, _ = model.Update(fileLoadedMsg{file: "b.go", lines: linesB})
	model = result.(Model)
	assert.Equal(t, "b.go", model.file.name)

	// add annotation on b.go
	model.layout.focus = paneDiff
	model.nav.diffCursor = 0
	model.startAnnotation()
	model.annot.input.SetValue("check b.go")
	model.saveAnnotation()

	// navigate tree back to a.go and load it
	model.tree.StepFile(sidepane.DirectionPrev)
	assert.Equal(t, "a.go", model.tree.SelectedFile())
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
	model.layout.focus = paneDiff
	model.nav.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annot.annotating)

	// type characters one at a time and verify each appears in viewport
	for _, ch := range []rune{'h', 'e', 'l', 'l', 'o'} {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)

		// verify the typed text so far is visible in the viewport content
		vpContent := model.layout.viewport.View()
		typed := model.annot.input.Value()
		assert.Contains(t, vpContent, typed, "viewport should contain typed text %q after keystroke %q", typed, string(ch))
	}

	// final check: all characters are visible
	assert.Equal(t, "hello", model.annot.input.Value())
	assert.Contains(t, model.layout.viewport.View(), "hello")
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
	model.layout.focus = paneDiff
	model.nav.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annot.annotating)

	// type some text
	for _, ch := range []rune{'t', 'e', 's', 't'} {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	// verify text is in viewport content before pressing Enter
	assert.True(t, model.annot.annotating, "should still be in annotation mode")
	vpContent := model.layout.viewport.View()
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
	model.layout.focus = paneDiff

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annot.annotating)

	// send a non-key message (e.g. a custom struct); should not panic and model stays annotating
	type customMsg struct{}
	result, _ = model.Update(customMsg{})
	model = result.(Model)
	assert.True(t, model.annot.annotating, "annotating should remain true after non-key message")
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
	m.file.name = "a.go"
	m.file.lines = lines

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
	model.layout.focus = paneDiff

	// add annotations on several lines - each takes an extra visual row
	for i := range 10 {
		model.store.Add(annotation.Annotation{File: "a.go", Line: i + 1, Type: string(diff.ChangeAdd), Comment: "annotation"})
	}

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight)

	// pgdown with annotations should move fewer cursor positions than viewport height
	// because annotation rows and annotation sub-lines take visual space
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := result.(Model)
	// cursor position or annotation flag must have changed
	assert.True(t, m2.nav.diffCursor > 0 || m2.annot.cursorOnAnnotation,
		"cursor should have moved forward (position or onto annotation)")
	assert.Less(t, m2.nav.diffCursor, pageHeight,
		"with annotations, cursor should move fewer positions than viewport height")
}

func TestModel_ShiftAStartsFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.True(t, model.annot.annotating, "A should start annotation mode")
	assert.True(t, model.annot.fileAnnotating, "A should set fileAnnotating=true")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_AnnotationInputCharLimit(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	longInput := strings.Repeat("x", annotCharLimit)
	overflowInput := longInput + strings.Repeat("y", 100)

	tests := []struct {
		name          string
		fileLevel     bool
		input         string
		wantStoredLen int
	}{
		{"line: long input preserved", false, longInput, annotCharLimit},
		{"line: overflow truncated to limit", false, overflowInput, annotCharLimit},
		{"file: long input preserved", true, longInput, annotCharLimit},
		{"file: overflow truncated to limit", true, overflowInput, annotCharLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.tree = testNewFileTree([]string{"a.go"})
			m.file.name = "a.go"
			m.file.lines = lines
			m.nav.diffCursor = 0
			m.layout.focus = paneDiff
			m.layout.width = 120
			m.layout.treeWidth = 30

			if tt.fileLevel {
				require.NotNil(t, m.startFileAnnotation())
				require.True(t, m.annot.fileAnnotating)
			} else {
				require.NotNil(t, m.startAnnotation())
				require.True(t, m.annot.annotating)
			}

			m.annot.input, _ = m.annot.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.input)})
			m.saveAnnotation()

			stored := m.store.Get("a.go")
			require.Len(t, stored, 1)
			assert.Len(t, stored[0].Comment, tt.wantStoredLen)
		})
	}
}

func TestModel_AnnotationInputWidthNarrowTerminal(t *testing.T) {
	// very narrow terminal should not produce negative textinput width
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.layout.focus = paneDiff
	m.layout.width = 20
	m.layout.treeWidth = 20 // width - treeWidth - 10 = -10 without the guard

	// line-level annotation
	cmd := m.startAnnotation()
	assert.NotNil(t, cmd)
	assert.True(t, m.annot.annotating)
	assert.GreaterOrEqual(t, m.annot.input.Width, 10, "text input width should be at least 10")

	// file-level annotation
	m.annot.annotating = false
	cmd = m.startFileAnnotation()
	assert.NotNil(t, cmd)
	assert.True(t, m.annot.fileAnnotating)
	assert.GreaterOrEqual(t, m.annot.input.Width, 10, "file text input width should be at least 10")
}

func TestModel_FileAnnotationInputWidthNarrowerThanLineLevel(t *testing.T) {
	// file-level annotation has wider prefix ("💬 file: " vs "💬 "), so input should be narrower
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.layout.focus = paneDiff
	m.layout.width = 120
	m.layout.treeWidth = 30
	m.layout.treeHidden = false

	// line-level annotation
	m.startAnnotation()
	lineWidth := m.annot.input.Width

	// file-level annotation
	m.annot.annotating = false
	m.startFileAnnotation()
	fileWidth := m.annot.input.Width

	assert.Greater(t, lineWidth, fileWidth, "file-level input should be narrower than line-level due to wider prefix")
	assert.Equal(t, 6, lineWidth-fileWidth, "width difference should match prefix width difference")
}

func TestModel_AnnotationInputWidthUsesMarkerWidth(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd}}
	tests := []struct {
		name          string
		marker        string
		wantLineWidth int
		wantFileWidth int
	}{
		{name: "default emoji marker", marker: "\U0001f4ac", wantLineWidth: 78, wantFileWidth: 72},
		{name: "wide emoji marker", marker: "\U0001f4ac\U0001f4ac", wantLineWidth: 76, wantFileWidth: 70},
		{name: "empty marker", marker: "", wantLineWidth: 80, wantFileWidth: 74},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.tree = testNewFileTree([]string{"a.go"})
			m.file.name = "a.go"
			m.file.lines = lines
			m.nav.diffCursor = 0
			m.layout.focus = paneDiff
			m.layout.width = 120
			m.layout.treeWidth = 30
			m.layout.treeHidden = false
			m.cfg.annotPrefix = tt.marker + " "
			m.cfg.annotFilePrefix = tt.marker + " file: "

			require.Equal(t, 84, m.diffContentWidth(), "test fixture pins absolute width")

			m.startAnnotation()
			assert.Equal(t, tt.wantLineWidth, m.annot.input.Width, "line annotation input width")

			m.annot.annotating = false
			m.startFileAnnotation()
			assert.Equal(t, tt.wantFileWidth, m.annot.input.Width, "file annotation input width")
		})
	}
}

func TestModel_FileAnnotationSavesWithLineZero(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff

	// start file-level annotation and set text
	m.startFileAnnotation()
	m.annot.input.SetValue("file-level comment")

	// save via Enter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.False(t, model.annot.annotating)
	assert.False(t, model.annot.fileAnnotating)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 0, anns[0].Line, "file-level annotation should have Line=0")
	assert.Empty(t, anns[0].Type, "file-level annotation should have empty Type")
	assert.Equal(t, "file-level comment", anns[0].Comment)
}

func TestModel_FileAnnotationPreFillsExisting(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing file note"})

	m.startFileAnnotation()
	assert.Equal(t, "existing file note", m.annot.input.Value(), "should pre-fill with existing file-level annotation")
}

func TestModel_FileAnnotationCancelResetsFlags(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.layout.focus = paneDiff

	m.startFileAnnotation()
	assert.True(t, m.annot.annotating)
	assert.True(t, m.annot.fileAnnotating)

	// press Esc to cancel
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := result.(Model)
	assert.False(t, model.annot.annotating, "cancel should reset annotating")
	assert.False(t, model.annot.fileAnnotating, "cancel should reset fileAnnotating")
}

func TestModel_FileAnnotationRenderedAtTopOfDiff(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
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
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.layout.focus = paneDiff
	m.nav.diffCursor = -1 // on file annotation line

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "file: file note", "file annotation should be rendered")
}

func TestModel_EnterOnFileAnnotationLineTriggersFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff
	m.nav.diffCursor = -1 // on file annotation line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing note"})

	// press enter on file annotation line - should start file annotation mode
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annot.annotating, "enter on file annotation line should start annotation mode")
	assert.True(t, model.annot.fileAnnotating, "enter on file annotation line should set fileAnnotating")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_EnterOnFileAnnotationLinePreFillsText(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff
	m.nav.diffCursor = -1 // on file annotation line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "pre-existing comment"})

	// press enter - should pre-fill with existing annotation text
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.Equal(t, "pre-existing comment", model.annot.input.Value(), "should pre-fill with existing file annotation")
}

func TestModel_EnterOnRegularDiffLineStillTriggersLineAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff
	m.nav.diffCursor = 1 // on regular diff line, not file annotation
	// add a file annotation to ensure it doesn't interfere
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	// press enter on regular line - should start line annotation, not file annotation
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	assert.True(t, model.annot.annotating, "enter on regular line should start annotation mode")
	assert.False(t, model.annot.fileAnnotating, "enter on regular line should not set fileAnnotating")
	assert.NotNil(t, cmd, "should return textinput blink command")
}

func TestModel_DeleteFileAnnotationViaD(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note to delete"})
	m.nav.diffCursor = -1 // on file annotation line

	// press 'd' to delete file-level annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)
	assert.Empty(t, model.store.Get("a.go"), "file-level annotation should be deleted")
	assert.GreaterOrEqual(t, model.nav.diffCursor, 0, "cursor should move to first valid diff line after deletion")
}

func TestModel_DeleteFileAnnotationCursorNotOnFileLine(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "line note"})
	m.nav.diffCursor = 0              // on regular line
	m.annot.cursorOnAnnotation = true // on the annotation sub-line for line 1

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
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note on a"})
	m.store.Add(annotation.Annotation{File: "b.go", Line: 1, Type: " ", Comment: "line note on b"})
	m.nav.diffCursor = -1 // on file annotation line

	// enable filter to show only annotated files
	m.tree.ToggleFilter(m.annotatedFiles())
	require.True(t, m.tree.FilterActive())

	// press 'd' to delete file-level annotation on a.go
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := result.(Model)

	// a.go no longer has annotations, filter should shift selection to b.go
	assert.Empty(t, model.store.Get("a.go"), "file-level annotation should be deleted")
	assert.NotNil(t, cmd, "should return a command to load the new file")
}

func TestModel_CursorLineHasAnnotationExcludesFileLevel(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.nav.diffCursor = 0 // on line 1, not on file annotation
	m.layout.focus = paneDiff

	// line 1 has no line-level annotation, only file-level exists
	assert.False(t, m.cursorLineHasAnnotation(), "should not report file-level annotation as line annotation")
}

func TestModel_CursorOnFileAnnotationLineReportsAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.nav.diffCursor = -1

	assert.True(t, m.cursorLineHasAnnotation(), "cursor on file annotation line should report annotation")
}

func TestModel_CursorNavigatesToFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	// move up from first line should go to file annotation
	m.moveDiffCursorUp()
	assert.Equal(t, -1, m.nav.diffCursor, "cursor should move to file annotation line (-1)")

	// move down from file annotation should go to first non-divider line
	m.moveDiffCursorDown()
	assert.Equal(t, 0, m.nav.diffCursor, "cursor should move from file annotation to first line")
}

func TestModel_HomeGoesToFileAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneDiff
	m.nav.diffCursor = 1
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	m.ready = true

	m.moveDiffCursorToStart()
	assert.Equal(t, -1, m.nav.diffCursor, "Home should move to file annotation line when it exists")
}

func TestModel_CursorViewportYWithFileAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	// cursor on file annotation line
	m.nav.diffCursor = -1
	assert.Equal(t, 0, m.cursorViewportY(), "file annotation line should be at viewport Y=0")

	// cursor on first diff line should be at Y=1 (file annotation occupies Y=0)
	m.nav.diffCursor = 0
	assert.Equal(t, 1, m.cursorViewportY(), "first diff line should be at Y=1 when file annotation exists")

	// cursor on second diff line
	m.nav.diffCursor = 1
	assert.Equal(t, 2, m.cursorViewportY(), "second diff line should be at Y=2 when file annotation exists")
}

func TestModel_CursorViewportYWithWrappedAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 60
	m.layout.treeWidth = 20
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	t.Run("long file annotation wraps to multiple rows", func(t *testing.T) {
		longComment := strings.Repeat("word ", 20) // ~100 chars, wraps at ~34
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})
		defer m.store.Delete("a.go", 0, "")

		wrapCount := m.wrappedAnnotationLineCount(annotKeyFile)
		assert.Greater(t, wrapCount, 1, "long file annotation should wrap to multiple rows")

		m.nav.diffCursor = 0
		assert.Equal(t, wrapCount, m.cursorViewportY(), "first diff line offset should equal wrap count")

		m.nav.diffCursor = 1
		assert.Equal(t, wrapCount+1, m.cursorViewportY(), "second diff line offset should be wrap count + 1")
	})

	t.Run("long inline annotation wraps to multiple rows", func(t *testing.T) {
		longComment := strings.Repeat("note ", 20) // ~100 chars
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: longComment})
		defer m.store.Delete("a.go", 1, " ")

		key := m.annotationKey(1, " ")
		wrapCount := m.wrappedAnnotationLineCount(key)
		assert.Greater(t, wrapCount, 1, "long inline annotation should wrap to multiple rows")

		m.nav.diffCursor = 1
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
	m.file.name = "a.go"
	m.layout.width = 60
	m.layout.treeWidth = 20
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.layout.focus = paneDiff

	t.Run("long file annotation wraps in rendered output", func(t *testing.T) {
		longComment := strings.Repeat("wrap ", 20)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})
		defer m.store.Delete("a.go", 0, "")

		wrapCount := m.wrappedAnnotationLineCount(annotKeyFile)
		assert.Greater(t, wrapCount, 1, "annotation should wrap")
		// cursor chevron appears exactly once (first line only)
		m.nav.diffCursor = -1
		rendered := m.renderDiff()
		assert.Equal(t, 1, strings.Count(rendered, "▶"), "cursor on first wrap line only")
	})
}

func TestModel_RenderDiffFileAnnotationInput(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.layout.focus = paneDiff

	// start file annotation and set text
	m.startFileAnnotation()
	m.annot.input.SetValue("typing file note...")

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "typing file note...", "file annotation input should be visible in rendered diff")
	assert.Contains(t, rendered, "file:", "should show file: prefix during input")
}

func TestModel_FileAnnotationExcludedFromRegularAnnotationMap(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
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
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m.nav.diffCursor = 0
	m.layout.focus = paneDiff

	// starting a regular annotation should set fileAnnotating to false
	m.startAnnotation()
	assert.True(t, m.annot.annotating)
	assert.False(t, m.annot.fileAnnotating, "startAnnotation should set fileAnnotating=false")
}

func TestModel_EditExistingFileAnnotationShowsInput(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.layout.focus = paneDiff
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "existing note"})

	// start editing the existing file-level annotation
	m.startFileAnnotation()
	assert.True(t, m.annot.annotating)
	assert.True(t, m.annot.fileAnnotating)
	assert.Equal(t, "existing note", m.annot.input.Value(), "input should be pre-filled")

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
	model.layout.focus = paneDiff
	model.nav.diffCursor = 0

	// enter annotation mode
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = result.(Model)
	require.True(t, model.annot.annotating)

	// press Q - should be handled as text input, not discard
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	model = result.(Model)
	assert.True(t, model.annot.annotating, "should still be annotating")
	assert.False(t, model.Discarded(), "should not be discarded")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming")
	assert.Contains(t, model.annot.input.Value(), "Q", "Q should be typed into input")
}

func TestModel_StatusBarDiscardConfirmation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.layout.width = 120
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
	model.file.singleFile = true
	model.layout.focus = paneDiff
	model.nav.diffCursor = 1 // on the add line
	res := style.PlainResolver()
	model.resolver = res
	model.renderer = style.NewRenderer(res)

	// press enter to start annotation
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)
	assert.True(t, model.annot.annotating, "enter should start annotation in single-file mode")

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
	assert.False(t, model.annot.annotating, "annotation should be saved")
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
		m.file.singleFile = true
		m.layout.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.file.mdTOC)

		model.layout.focus = paneDiff
		model.nav.diffCursor = 1 // on "some text" line

		// press 'a' to start annotation
		result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		model = result.(Model)
		assert.True(t, model.annot.annotating, "should enter annotation mode in diff pane with TOC")
		assert.NotNil(t, cmd)
	})

	t.Run("file annotation with TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.layout.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.file.mdTOC)

		model.layout.focus = paneDiff

		// press 'A' for file annotation
		result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
		model = result.(Model)
		assert.True(t, model.annot.annotating, "should enter file annotation mode with TOC")
		assert.True(t, model.annot.fileAnnotating, "should be file-level annotation")
		assert.NotNil(t, cmd)
	})

	t.Run("annotation list with TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.layout.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.file.mdTOC)

		model.layout.focus = paneDiff
		model.nav.diffCursor = 1

		// add an annotation first
		model.store.Add(annotation.Annotation{File: "README.md", Line: 2, Type: " ", Comment: "test annotation"})

		// press '@' to open annotation list
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
		model = result.(Model)
		assert.True(t, model.overlay.Active(), "annotation list should open with TOC active")
		assert.Equal(t, overlay.KindAnnotList, model.overlay.Kind())
	})

	t.Run("annotation keys blocked in TOC pane", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.layout.treeWidth = 0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)
		require.NotNil(t, model.file.mdTOC)

		model.layout.focus = paneTree // TOC pane

		// press 'a' in TOC pane - should not start annotation
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		model = result.(Model)
		assert.False(t, model.annot.annotating, "annotation should not start from TOC pane")
	})
}

func TestModel_ShiftAIgnoredWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = ""
	m.layout.focus = paneTree

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.False(t, model.annot.annotating, "A without currFile should not start annotation")
	assert.Nil(t, cmd)
}

func TestModel_ShiftAOnlyWorksFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}

	// from tree pane — should be ignored to avoid annotating wrong file
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneTree
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model := result.(Model)
	assert.False(t, model.annot.annotating, "A from tree pane should not start annotation")
	assert.False(t, model.annot.fileAnnotating)
	assert.Nil(t, cmd)

	// from diff pane — should work
	m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m2.tree = testNewFileTree([]string{"a.go"})
	m2.file.name = "a.go"
	m2.file.lines = lines
	m2.layout.focus = paneDiff
	result, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	model = result.(Model)
	assert.True(t, model.annot.annotating, "A should work from diff pane")
	assert.True(t, model.annot.fileAnnotating)
	assert.NotNil(t, cmd)
}

func TestModel_WrappedAnnotationLineCount_MultiLine(t *testing.T) {
	newModel := func() Model {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.layout.width = 120
		m.layout.treeWidth = 20
		m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
		return m
	}

	t.Run("single-line short comment is one row", func(t *testing.T) {
		m := newModel()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "hi"})
		defer m.store.Delete("a.go", 1, " ")

		assert.Equal(t, 1, m.wrappedAnnotationLineCount(m.annotationKey(1, " ")))
	})

	t.Run("multi-line without wrap counts each logical line", func(t *testing.T) {
		m := newModel()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "one\ntwo\nthree"})
		defer m.store.Delete("a.go", 1, " ")

		assert.Equal(t, 3, m.wrappedAnnotationLineCount(m.annotationKey(1, " ")))
	})

	t.Run("multi-line with inner-line wrap sums wrapped rows", func(t *testing.T) {
		m := newModel()
		m.layout.width = 40
		m.layout.treeWidth = 8
		long := strings.Repeat("alpha ", 20) // forces wrap
		short := "brief"
		m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: long + "\n" + short})
		defer m.store.Delete("a.go", 1, " ")

		count := m.wrappedAnnotationLineCount(m.annotationKey(1, " "))
		assert.Greater(t, count, 2, "long-first-line + short-second-line must exceed two rows")
	})

	t.Run("file-level multi-line annotation", func(t *testing.T) {
		m := newModel()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "line1\nline2"})
		defer m.store.Delete("a.go", 0, "")

		assert.Equal(t, 2, m.wrappedAnnotationLineCount(annotKeyFile))
	})

	t.Run("file-level multi-line with continuation-wrap", func(t *testing.T) {
		m := newModel()
		m.layout.width = 40
		m.layout.treeWidth = 8
		// second logical line is long AND receives 9-space file-level indent, so wraps
		long := strings.Repeat("beta ", 20)
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "short\n" + long})
		defer m.store.Delete("a.go", 0, "")

		count := m.wrappedAnnotationLineCount(annotKeyFile)
		assert.Greater(t, count, 2, "second line wraps due to length, total exceeds 2")
	})

	t.Run("empty annotation yields one row", func(t *testing.T) {
		m := newModel()
		// no annotation stored — lookup returns empty, falls through to baseline
		assert.Equal(t, 1, m.wrappedAnnotationLineCount(m.annotationKey(42, "+")))
	})
}

func TestModel_AnnotateCtrlEOpensEditor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1

	fake := mockEditor("edited result", nil)
	m.editor = fake

	m.startAnnotation()
	m.annot.input.SetValue("seeded text")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := result.(Model)
	require.NotNil(t, cmd, "Ctrl+E should return a tea.Cmd for ExecProcess")
	require.Len(t, fake.CommandCalls(), 1, "editor.Command should be called once")
	assert.Equal(t, "seeded text", fake.CommandCalls()[0].Content, "editor must receive current input value")
	assert.True(t, model.annot.annotating, "annotation mode should remain active so editorFinishedMsg routes back correctly")

	// tea.ExecProcess wraps the cmd so that when invoked outside the runtime the
	// returned msg is nil; the assertions above exercise the synchronous
	// spawn-time contract. editorFinishedMsg delivery is covered by
	// TestModel_EditorFinishedSavesMultiLineContent which feeds the msg directly.
}

func TestModel_AnnotateCtrlEOpensEditorFileLevel(t *testing.T) {
	// mirrors TestModel_AnnotateCtrlEOpensEditor but starts from the file-level
	// annotation entry point — asserts Ctrl+E also dispatches the editor on
	// startFileAnnotation() flow.
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"

	fake := mockEditor("edited file note", nil)
	m.editor = fake

	m.startFileAnnotation()
	m.annot.input.SetValue("file seed")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := result.(Model)
	require.NotNil(t, cmd, "Ctrl+E should return a tea.Cmd for ExecProcess on file-level path")
	require.Len(t, fake.CommandCalls(), 1, "editor.Command should be called once on file-level path")
	assert.Equal(t, "file seed", fake.CommandCalls()[0].Content, "editor must receive current file-level input value")
	assert.True(t, model.annot.annotating, "annotation mode should remain active so editorFinishedMsg routes back correctly")
	assert.True(t, model.annot.fileAnnotating, "fileAnnotating must remain true so editor result targets file-level save")
}

func TestModel_EditorFinishedSavesMultiLineContent(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 2

	m.startAnnotation()
	m.annot.input.SetValue("stale")

	msg := editorFinishedMsg{content: "line1\nline2\nline3", fileName: "a.go", fileLevel: false, line: 3, changeType: "+"}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "multi-line content should be saved as a single annotation")
	assert.Equal(t, "line1\nline2\nline3", anns[0].Comment, "embedded newlines must be preserved")
	assert.Equal(t, 3, anns[0].Line)
	assert.Equal(t, "+", anns[0].Type)
	assert.False(t, model.annot.annotating, "annotation mode should be cleared after successful save")
}

func TestModel_EditorFinishedErrorPreservesState(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	m.annot.input.SetValue("in-progress note")

	msg := editorFinishedMsg{err: errors.New("editor exit 1"), fileName: "a.go", fileLevel: false, line: 1, changeType: "+"}
	result, _ := m.Update(msg)
	model := result.(Model)

	assert.Empty(t, model.store.Get("a.go"), "editor error must not touch the store")
	assert.True(t, model.annot.annotating, "annotation mode should stay open so user can retry or Esc")
	assert.Equal(t, "in-progress note", model.annot.input.Value(), "input value should be preserved on editor error")

	// feed a subsequent keystroke to prove annotation mode is fully live — not
	// just value-aliased state. A default-branch key in handleAnnotateKey must
	// append to the textinput, which would silently fail if the model weren't
	// actually in annotation mode.
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	model = result.(Model)
	assert.Equal(t, "in-progress note!", model.annot.input.Value(), "annotation mode must accept typed keys after editor error path")
}

func TestModel_EditorFinishedErrorPreservesStateFileLevel(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"

	m.startFileAnnotation()
	m.annot.input.SetValue("file-level draft")

	msg := editorFinishedMsg{err: errors.New("editor exit 1"), fileName: "a.go", fileLevel: true, line: 0, changeType: ""}
	result, _ := m.Update(msg)
	model := result.(Model)

	assert.Empty(t, model.store.Get("a.go"), "editor error must not touch the store on file-level path")
	assert.True(t, model.annot.annotating, "annotation mode should stay open so user can retry or Esc")
	assert.True(t, model.annot.fileAnnotating, "fileAnnotating must remain true on error")
	assert.Equal(t, "file-level draft", model.annot.input.Value(), "input value must be preserved on editor error")
}

func TestModel_EditorFinishedRetryAfterErrorReSeedsWithPreservedInput(t *testing.T) {
	// after an editor error, pressing Ctrl+E again must re-seed the editor with
	// the preserved input content so the user can resume without losing work.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	fake := mockEditor("", nil)
	m.editor = fake

	m.startAnnotation()
	m.annot.input.SetValue("retry content")

	// first Ctrl+E
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := result.(Model)
	require.NotNil(t, cmd)
	require.Len(t, fake.CommandCalls(), 1)
	assert.Equal(t, "retry content", fake.CommandCalls()[0].Content)

	// simulate editor failure
	errMsg := editorFinishedMsg{err: errors.New("failed"), fileName: "a.go", fileLevel: false, line: 1, changeType: "+"}
	result, _ = model.Update(errMsg)
	model = result.(Model)
	assert.True(t, model.annot.annotating, "annotating must remain true after error")
	assert.Equal(t, "retry content", model.annot.input.Value(), "input preserved after error")

	// second Ctrl+E — editor must be re-invoked with the preserved content
	result, cmd2 := model.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model = result.(Model)
	require.NotNil(t, cmd2)
	require.Len(t, fake.CommandCalls(), 2, "editor must be invoked again on retry")
	assert.Equal(t, "retry content", fake.CommandCalls()[1].Content, "retry must re-seed with preserved input content")
	assert.True(t, model.annot.annotating)
}

func TestModel_EditorFinishedEmptyContentPreservesExistingAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 2

	// pre-existing annotation on the target line
	m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "+", Comment: "old comment"})

	m.startAnnotation()

	msg := editorFinishedMsg{content: "", fileName: "a.go", fileLevel: false, line: 3, changeType: "+"}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "existing annotation must remain in the store")
	assert.Equal(t, "old comment", anns[0].Comment, "empty editor result must not overwrite or delete the existing annotation")
	assert.False(t, model.annot.annotating, "annotation mode should be cleared after cancel")
	assert.False(t, model.annot.fileAnnotating, "fileAnnotating must also be cleared on cancel path")
}

func TestModel_EditorFinishedFileLevelSavesMultiLine(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"

	// start file-level annotation so saveComment's fileLevel branch is exercised
	m.startFileAnnotation()

	msg := editorFinishedMsg{content: "note1\nnote2", fileName: "a.go", fileLevel: true, line: 0, changeType: ""}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 0, anns[0].Line, "file-level annotation stored on Line=0")
	assert.Equal(t, "note1\nnote2", anns[0].Comment)
	assert.False(t, model.annot.annotating)
	assert.False(t, model.annot.fileAnnotating)
}

func TestModel_EditorFinishedFileLevelEmptyContentPreservesExistingAnnotation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"

	// pre-existing file-level annotation the user is re-editing
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "original file note"})

	m.startFileAnnotation()

	msg := editorFinishedMsg{content: "", fileName: "a.go", fileLevel: true, line: 0, changeType: ""}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "pre-existing file-level annotation must survive empty editor result")
	assert.Equal(t, "original file note", anns[0].Comment, "empty editor result must not overwrite existing file-level annotation")
	assert.False(t, model.annot.annotating, "annotation mode cleared via cancelAnnotation")
	assert.False(t, model.annot.fileAnnotating, "fileAnnotating cleared via cancelAnnotation")
}

func TestModel_EditorFinishedHunkKeywordSetsEndLine(t *testing.T) {
	// verifies saveComment re-derives the hunk index from (line, changeType)
	// so hunk range expansion still works even though the editor path no
	// longer references m.nav.diffCursor.
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx before", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "new line", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added line", ChangeType: diff.ChangeAdd},
		{OldNum: 2, NewNum: 4, Content: "ctx after", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1
	m.startAnnotation()

	// simulate cursor drifting during editor session
	m.nav.diffCursor = 0

	msg := editorFinishedMsg{content: "rewrite this hunk", fileName: "a.go", fileLevel: false, line: 2, changeType: "+"}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 2, anns[0].Line)
	assert.Equal(t, 3, anns[0].EndLine, "hunk-end detection must resolve from (line, type) not cursor")
	assert.False(t, model.annot.annotating, "annotation mode must be cleared after successful save via editor path")
}

func TestModel_ReAnnotateMultiLineKeepsInputEmptyAndStashesOriginal(t *testing.T) {
	// re-annotating a line that already has a multi-line comment must NOT call
	// ti.SetValue(comment) because textinput's sanitizer collapses \n to a
	// single space — that would silently flatten user work on the first re-open
	// and, if the user then hits Enter, overwrite the stored multi-line version.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "first line\nsecond line\nthird"})

	m.startAnnotation()
	assert.Empty(t, m.annot.input.Value(), "input must stay empty so the sanitizer cannot flatten \\n into space")
	assert.Equal(t, "first line\nsecond line\nthird", m.annot.existingMultiline, "original multi-line content stashed verbatim")
	assert.Contains(t, m.annot.input.Placeholder, "existing multi-line", "placeholder should hint that content is stored")

	// Enter with empty input must not touch the stored annotation
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "first line\nsecond line\nthird", anns[0].Comment, "Enter on empty input must preserve existing multi-line annotation unchanged")
	assert.False(t, model.annot.annotating, "annotation mode cleared")
	assert.Empty(t, model.annot.existingMultiline, "existingMultiline cleared on annotation exit")
}

func TestModel_ReAnnotateMultiLineCtrlESeedsFromStash(t *testing.T) {
	// Ctrl+E after re-opening a multi-line annotation must seed the editor
	// with the full stored content, not the empty textinput value.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "top\nmiddle\nbottom"})

	fake := mockEditor("", nil)
	m.editor = fake

	m.startAnnotation()
	require.Empty(t, m.annot.input.Value())
	require.Equal(t, "top\nmiddle\nbottom", m.annot.existingMultiline)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := result.(Model)
	require.NotNil(t, cmd)
	require.Len(t, fake.CommandCalls(), 1)
	assert.Equal(t, "top\nmiddle\nbottom", fake.CommandCalls()[0].Content, "editor must be seeded from existingMultiline when input is empty")
	assert.True(t, model.annot.annotating, "annotation mode remains open while editor runs")
	assert.Equal(t, "top\nmiddle\nbottom", model.annot.existingMultiline, "stash preserved across Ctrl+E")
}

func TestModel_ReAnnotateMultiLineTypedOverwriteWins(t *testing.T) {
	// if user re-opens multi-line and explicitly types a new value, that value
	// overwrites the stored annotation (not silently blended with the original).
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "old\nmulti\nline"})

	m.startAnnotation()
	m.annot.input.SetValue("new one-liner")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := result.(Model)
	anns := model.store.Get("a.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "new one-liner", anns[0].Comment, "explicit typed value overwrites stored multi-line")
	assert.Empty(t, model.annot.existingMultiline, "existingMultiline cleared after save")
}

func TestModel_ReAnnotateSingleLinePreFillsAsBefore(t *testing.T) {
	// single-line existing annotations keep the pre-fill-via-SetValue path —
	// only multi-line comments go through the stash workaround.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "plain note"})

	m.startAnnotation()
	assert.Equal(t, "plain note", m.annot.input.Value(), "single-line annotation still pre-fills the textinput")
	assert.Empty(t, m.annot.existingMultiline, "single-line path does not populate existingMultiline")
	assert.Contains(t, m.annot.input.Placeholder, "Ctrl+E", "placeholder unchanged for single-line re-annotation")
}

func TestModel_ReAnnotateFileLevelMultiLineStashedNotFlattened(t *testing.T) {
	// file-level path has the same sanitizer hazard as line-level; verify it
	// also stashes multi-line content instead of flattening via SetValue.
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file\nnote\nspans"})

	m.startFileAnnotation()
	assert.Empty(t, m.annot.input.Value(), "file-level input empty when existing is multi-line")
	assert.Equal(t, "file\nnote\nspans", m.annot.existingMultiline, "file-level stash holds full content")

	fake := mockEditor("", nil)
	m.editor = fake
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	model := result.(Model)
	require.NotNil(t, cmd)
	require.Len(t, fake.CommandCalls(), 1)
	assert.Equal(t, "file\nnote\nspans", fake.CommandCalls()[0].Content, "file-level Ctrl+E seeds from stash")

	// Esc must clear the stash so it doesn't leak to a later annotation on a different line
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = result.(Model)
	assert.False(t, model.annot.annotating)
	assert.Empty(t, model.annot.existingMultiline, "Esc clears existingMultiline")
}

func TestModel_EditorFinishedErrorWithContentStillSavesRecoveredText(t *testing.T) {
	// readResult's documented contract: on soft editor error (tty restore,
	// non-zero exit after save) content is populated alongside runErr so callers
	// can preserve user work. handleEditorFinished must save that content and
	// log the error rather than dropping the edited text.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	m.annot.input.SetValue("stale")

	msg := editorFinishedMsg{
		err: errors.New("tty restore failed"), content: "recovered\nmulti-line text", seed: "stale",
		fileName: "a.go", fileLevel: false, line: 1, changeType: "+",
	}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "content must be saved even though editor reported an error")
	assert.Equal(t, "recovered\nmulti-line text", anns[0].Comment, "recovered content survives")
	assert.Equal(t, 1, anns[0].Line)
	assert.Equal(t, "+", anns[0].Type)
	assert.False(t, model.annot.annotating, "annotation mode cleared after save")
}

func TestModel_EditorFinishedErrorWithSeedUnchangedDoesNotSave(t *testing.T) {
	// launch-time failure scenario: editor binary missing / tty release failure /
	// cmd.Run start error — temp file still contains the ORIGINAL seed content,
	// readResult returns the seed verbatim alongside the error. handleEditorFinished
	// must NOT treat this as "user edited to produce this content" — the user's
	// intent was to keep editing the in-progress draft, not to commit the seed
	// as a finished annotation.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	m.annot.input.SetValue("my rough note")

	msg := editorFinishedMsg{
		err:     errors.New("exec: \"no-such-editor\": executable file not found in $PATH"),
		content: "my rough note", seed: "my rough note",
		fileName: "a.go", fileLevel: false, line: 1, changeType: "+",
	}
	result, _ := m.Update(msg)
	model := result.(Model)

	assert.Empty(t, model.store.Get("a.go"), "seed-equals-content on error must NOT commit the seed as an annotation")
	assert.True(t, model.annot.annotating, "annotation mode must stay open so user can retry or continue typing")
	assert.Equal(t, "my rough note", model.annot.input.Value(), "input value must be preserved verbatim")
}

func TestModel_EditorFinishedScrollsToShowMultilineAnnotation(t *testing.T) {
	// regression: saving a multi-row annotation via the external-editor path on
	// a line sitting at the bottom of the viewport used to leave the appended
	// annotation rows clipped below the viewport, because saveComment only
	// re-rendered content without syncing scroll.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "line4", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 3
	// viewport height of 4 fits 1-row diff line + 3 annotation rows only if sync scrolls.
	m.layout.viewport = viewport.New(80, 4)
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(0)

	require.Equal(t, m.layout.viewport.YOffset+m.layout.viewport.Height-1, m.cursorViewportY(),
		"cursor should start on the last visible row")

	msg := editorFinishedMsg{
		content:  "note line 1\nnote line 2\nnote line 3",
		fileName: "a.go", fileLevel: false, line: 4, changeType: " ",
	}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "annotation must be saved")

	cursorTop := model.cursorViewportY()
	annotRows := model.wrappedAnnotationLineCount(model.annotationKey(4, " "))
	cursorBottom := cursorTop + 1 + annotRows - 1 // 1 diff row + annotation rows
	expectedOffset := cursorBottom - model.layout.viewport.Height + 1
	assert.Equal(t, expectedOffset, model.layout.viewport.YOffset,
		"YOffset should pin the annotation's visual bottom to the viewport bottom after editor save")
	assert.GreaterOrEqual(t, cursorTop, model.layout.viewport.YOffset, "cursor top must be visible")
	assert.Less(t, cursorBottom, model.layout.viewport.YOffset+model.layout.viewport.Height,
		"last annotation row must be visible after editor save")
}

func TestModel_EditorFinishedFileLevelGoesToTop(t *testing.T) {
	// file-level annotation save via editor path goes to the top of the viewport
	// (GotoTop), distinct from the line-level path which uses syncViewportToCursor.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "line4", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 3
	m.layout.viewport = viewport.New(80, 3)
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(2)

	msg := editorFinishedMsg{
		content:  "file note 1\nfile note 2",
		fileName: "a.go", fileLevel: true, line: 0, changeType: "",
	}
	result, _ := m.Update(msg)
	model := result.(Model)

	anns := model.store.Get("a.go")
	require.Len(t, anns, 1, "file-level annotation must be saved")
	assert.Equal(t, 0, anns[0].Line, "saved annotation should be file-level (Line=0)")
	assert.Equal(t, 0, model.layout.viewport.YOffset,
		"viewport should be at top after file-level save (GotoTop path)")
	assert.Equal(t, -1, model.nav.diffCursor,
		"cursor should park on file annotation line (-1)")
}

func TestModel_AnnotationPlaceholderMentionsEditor(t *testing.T) {
	// placeholder is an affordance surfacing Ctrl+E binding without a help overlay.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "x", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.startAnnotation()
	assert.Contains(t, m.annot.input.Placeholder, "Ctrl+E", "line-level placeholder must mention Ctrl+E")

	m2 := testModel([]string{"a.go"}, nil)
	m2.tree = testNewFileTree([]string{"a.go"})
	m2.layout.focus = paneDiff
	m2.file.name = "a.go"
	m2.startFileAnnotation()
	assert.Contains(t, m2.annot.input.Placeholder, "Ctrl+E", "file-level placeholder must mention Ctrl+E")
}

func TestModel_AnnotationPlaceholderRemappedEditor(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeAdd}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.keymap.Unbind("ctrl+e")
	m.keymap.Bind("ctrl+g", keymap.ActionOpenEditor)

	m.startAnnotation()
	assert.Contains(t, m.annot.input.Placeholder, "Ctrl+G", "placeholder must reflect remapped key")
	assert.NotContains(t, m.annot.input.Placeholder, "Ctrl+E", "placeholder must not mention old key")
}

func TestModel_AnnotationPlaceholderUnboundEditor(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "x", ChangeType: diff.ChangeAdd}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.keymap.Unbind("ctrl+e")

	m.startAnnotation()
	assert.NotContains(t, m.annot.input.Placeholder, "Ctrl+E", "placeholder must not mention editor when unbound")
	assert.NotContains(t, m.annot.input.Placeholder, "for editor", "placeholder must not mention editor when unbound")
}

func TestModel_RemappedEditorKeyOpensEditor(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.keymap.Unbind("ctrl+e")
	m.keymap.Bind("ctrl+g", keymap.ActionOpenEditor)

	fake := mockEditor("edited", nil)
	m.editor = fake

	m.startAnnotation()
	m.annot.input.SetValue("seed")

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	model := result.(Model)
	require.NotNil(t, cmd, "remapped ctrl+g should trigger editor")
	require.Len(t, fake.CommandCalls(), 1, "editor.Command called once")
	assert.Equal(t, "seed", fake.CommandCalls()[0].Content)
	assert.True(t, model.annot.annotating)
}

func TestModel_UnboundEditorKeyFallsThrough(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0

	m.keymap.Unbind("ctrl+e")

	fake := mockEditor("edited", nil)
	m.editor = fake

	m.startAnnotation()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	assert.Empty(t, fake.CommandCalls(), "unbound ctrl+e must not open editor")
}

func TestModel_VisualRowToDiffLine_EmptyFile(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.nav.diffCursor = 5

	idx, onAnn := m.visualRowToDiffLine(0)
	assert.Equal(t, 5, idx, "empty file returns current diffCursor")
	assert.False(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(10)
	assert.Equal(t, 5, idx)
	assert.False(t, onAnn)
}

func TestModel_VisualRowToDiffLine_EmptyFileWithFileAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	idx, onAnn := m.visualRowToDiffLine(0)
	assert.Equal(t, -1, idx, "row 0 within empty-file file-annotation returns -1")
	assert.False(t, onAnn)
}

func TestModel_VisualRowToDiffLine_SimpleLines(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeAdd},
	}

	tests := []struct {
		name    string
		row     int
		wantIdx int
		wantAnn bool
	}{
		{name: "row 0 maps to line 0", row: 0, wantIdx: 0, wantAnn: false},
		{name: "row 1 maps to line 1", row: 1, wantIdx: 1, wantAnn: false},
		{name: "row 2 maps to line 2", row: 2, wantIdx: 2, wantAnn: false},
		{name: "row beyond end clamps to last", row: 10, wantIdx: 2, wantAnn: false},
		{name: "negative row falls back to first visible line", row: -1, wantIdx: 0, wantAnn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, onAnn := m.visualRowToDiffLine(tt.row)
			assert.Equal(t, tt.wantIdx, idx)
			assert.Equal(t, tt.wantAnn, onAnn)
		})
	}
}

func TestModel_VisualRowToDiffLine_FileAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})

	idx, onAnn := m.visualRowToDiffLine(0)
	assert.Equal(t, -1, idx, "row 0 maps to file annotation line")
	assert.False(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(1)
	assert.Equal(t, 0, idx, "row 1 maps to first diff line")
	assert.False(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(2)
	assert.Equal(t, 1, idx, "row 2 maps to second diff line")
	assert.False(t, onAnn)
}

func TestModel_VisualRowToDiffLine_WrappedFileAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 60
	m.layout.treeWidth = 20
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	longComment := strings.Repeat("word ", 20) // ~100 chars, wraps
	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: longComment})

	wrapCount := m.wrappedAnnotationLineCount(annotKeyFile)
	require.Greater(t, wrapCount, 1, "test precondition: file annotation must wrap")

	// every row within the wrapped file annotation maps to idx=-1
	for r := range wrapCount {
		idx, onAnn := m.visualRowToDiffLine(r)
		assert.Equal(t, -1, idx, "row %d inside wrapped file annotation", r)
		assert.False(t, onAnn, "file annotation does not distinguish sub-rows")
	}
	// row right after the wrapped annotation is the first diff line
	idx, onAnn := m.visualRowToDiffLine(wrapCount)
	assert.Equal(t, 0, idx)
	assert.False(t, onAnn)
	idx, onAnn = m.visualRowToDiffLine(wrapCount + 1)
	assert.Equal(t, 1, idx)
	assert.False(t, onAnn)
}

func TestModel_VisualRowToDiffLine_LineAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "inline note"})

	// line 0 occupies row 0, line 1 occupies rows 1 (diff) + 2 (annotation),
	// line 2 starts at row 3
	idx, onAnn := m.visualRowToDiffLine(0)
	assert.Equal(t, 0, idx)
	assert.False(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(1)
	assert.Equal(t, 1, idx, "row 1 on diff row of annotated line")
	assert.False(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(2)
	assert.Equal(t, 1, idx, "row 2 on annotation sub-row of line 1")
	assert.True(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(3)
	assert.Equal(t, 2, idx, "row 3 on the following diff line")
	assert.False(t, onAnn)
}

func TestModel_VisualRowToDiffLine_WrappedLineAnnotation(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 60
	m.layout.treeWidth = 20
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
	}
	longComment := strings.Repeat("note ", 20)
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: longComment})

	key := m.annotationKey(2, "+")
	annCount := m.wrappedAnnotationLineCount(key)
	require.Greater(t, annCount, 1, "test precondition: annotation must wrap")

	// line 0 at row 0, line 1 starts at row 1 (1 diff row + annCount annotation rows)
	idx, onAnn := m.visualRowToDiffLine(0)
	assert.Equal(t, 0, idx)
	assert.False(t, onAnn)

	idx, onAnn = m.visualRowToDiffLine(1)
	assert.Equal(t, 1, idx, "row 1 is the diff row of annotated line 1")
	assert.False(t, onAnn)

	// each subsequent row within annotation maps to line 1 with onAnn=true
	for r := 2; r < 1+1+annCount; r++ {
		idx, onAnn = m.visualRowToDiffLine(r)
		assert.Equal(t, 1, idx, "row %d inside wrapped annotation", r)
		assert.True(t, onAnn, "row %d must be flagged as annotation sub-row", r)
	}
}

func TestModel_VisualRowToDiffLine_Dividers(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{Content: "...", ChangeType: diff.ChangeDivider},
		{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
	}

	// dividers occupy 1 row each (they are not hidden unless in collapsed mode)
	idx, _ := m.visualRowToDiffLine(0)
	assert.Equal(t, 0, idx)
	idx, _ = m.visualRowToDiffLine(1)
	assert.Equal(t, 1, idx, "divider row maps to its line index")
	idx, _ = m.visualRowToDiffLine(2)
	assert.Equal(t, 2, idx)
}

func TestModel_VisualRowToDiffLine_CollapsedHidden(t *testing.T) {
	t.Run("mixed hunk hides all removes", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		// a hunk with context, then remove+add+context — not a delete-only hunk,
		// so collapsed mode hides every remove line entirely (no placeholder).
		m.file.lines = []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "removed1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "removed2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "added1", ChangeType: diff.ChangeAdd},
			{OldNum: 4, NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m.modes.collapsed.enabled = true
		m.modes.collapsed.expandedHunks = map[int]bool{}

		// visible rows: 0->ctx(0), 1->added1(3), 2->ctx2(4)
		idx, onAnn := m.visualRowToDiffLine(0)
		assert.Equal(t, 0, idx)
		assert.False(t, onAnn)

		idx, onAnn = m.visualRowToDiffLine(1)
		assert.Equal(t, 3, idx, "row 1 skips hidden removed lines and lands on added1")
		assert.False(t, onAnn)

		idx, onAnn = m.visualRowToDiffLine(2)
		assert.Equal(t, 4, idx, "row 2 must land on the trailing context line")
		assert.False(t, onAnn)
	})

	t.Run("delete-only hunk keeps placeholder visible", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		// delete-only hunk: placeholder at hunkStart stays visible, subsequent
		// removes hidden
		m.file.lines = []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "removed1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "removed2", ChangeType: diff.ChangeRemove},
			{OldNum: 4, NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m.modes.collapsed.enabled = true
		m.modes.collapsed.expandedHunks = map[int]bool{}

		// visible rows: 0->ctx, 1->placeholder(idx 1), 2->ctx2
		idx, onAnn := m.visualRowToDiffLine(1)
		assert.Equal(t, 1, idx, "delete-only hunk placeholder maps to hunkStart")
		assert.False(t, onAnn)
	})
}

func TestModel_VisualRowToDiffLine_LargeRow(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "b", ChangeType: diff.ChangeContext},
	}
	// simulates a scrolled viewport: caller passes (y + YOffset) — arbitrary big row
	idx, onAnn := m.visualRowToDiffLine(9999)
	assert.Equal(t, 1, idx, "rows past end clamp to last line")
	assert.False(t, onAnn)
}

func TestModel_VisualRowToDiffLine_RoundTrip(t *testing.T) {
	// for every reachable cursor position, cursorVisualRange top must round-trip
	// back through visualRowToDiffLine to the same index. this is the core
	// invariant — inverse mapping bugs would surface here.
	t.Run("plain diff", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.file.lines = []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
			{Content: "...", ChangeType: diff.ChangeDivider},
			{NewNum: 10, Content: "line10", ChangeType: diff.ChangeContext},
		}

		for i := range m.file.lines {
			m.nav.diffCursor = i
			m.annot.cursorOnAnnotation = false
			top, _ := m.cursorVisualRange()
			idx, onAnn := m.visualRowToDiffLine(top)
			assert.Equal(t, i, idx, "round-trip diff index for cursor %d", i)
			assert.False(t, onAnn, "diff row (not annotation) for cursor %d", i)
		}
	})

	t.Run("with file annotation", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.file.lines = []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
		}
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "file"})

		// file-annotation cursor (-1)
		m.nav.diffCursor = -1
		m.annot.cursorOnAnnotation = false
		top, _ := m.cursorVisualRange()
		idx, onAnn := m.visualRowToDiffLine(top)
		assert.Equal(t, -1, idx, "file annotation cursor round-trips to -1")
		assert.False(t, onAnn)

		// regular cursor positions still round-trip after the file annotation offset
		for i := range m.file.lines {
			m.nav.diffCursor = i
			m.annot.cursorOnAnnotation = false
			rowTop, _ := m.cursorVisualRange()
			gotIdx, gotAnn := m.visualRowToDiffLine(rowTop)
			assert.Equal(t, i, gotIdx, "round-trip with file annotation, cursor %d", i)
			assert.False(t, gotAnn)
		}
	})

	t.Run("with inline annotation and cursor on annotation sub-row", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.file.lines = []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		}
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: "inline"})

		// cursor on the diff row of annotated line
		m.nav.diffCursor = 1
		m.annot.cursorOnAnnotation = false
		top, _ := m.cursorVisualRange()
		idx, onAnn := m.visualRowToDiffLine(top)
		assert.Equal(t, 1, idx)
		assert.False(t, onAnn)

		// cursor on the annotation sub-row — top now points to the annotation row
		m.nav.diffCursor = 1
		m.annot.cursorOnAnnotation = true
		top, _ = m.cursorVisualRange()
		idx, onAnn = m.visualRowToDiffLine(top)
		assert.Equal(t, 1, idx, "annotation sub-row round-trips to same diff-line index")
		assert.True(t, onAnn, "annotation sub-row round-trips with onAnnotation=true")
	})

	t.Run("with wrapped file and line annotations", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.layout.width = 60
		m.layout.treeWidth = 20
		m.file.lines = []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		}
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: strings.Repeat("word ", 20)})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Comment: strings.Repeat("note ", 20)})

		// file-annotation cursor
		m.nav.diffCursor = -1
		m.annot.cursorOnAnnotation = false
		top, _ := m.cursorVisualRange()
		idx, onAnn := m.visualRowToDiffLine(top)
		assert.Equal(t, -1, idx)
		assert.False(t, onAnn)

		for i := range m.file.lines {
			m.nav.diffCursor = i
			m.annot.cursorOnAnnotation = false
			rowTop, _ := m.cursorVisualRange()
			gotIdx, gotAnn := m.visualRowToDiffLine(rowTop)
			assert.Equal(t, i, gotIdx, "wrapped diff round-trip cursor %d", i)
			assert.False(t, gotAnn, "cursor %d should not be on annotation sub-row", i)
		}

		// cursor on annotation sub-row of the annotated line
		m.nav.diffCursor = 1
		m.annot.cursorOnAnnotation = true
		top, _ = m.cursorVisualRange()
		idx, onAnn = m.visualRowToDiffLine(top)
		assert.Equal(t, 1, idx)
		assert.True(t, onAnn)
	})

	t.Run("in collapsed mode", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.file.lines = []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "rem1", ChangeType: diff.ChangeRemove},
			{OldNum: 3, Content: "rem2", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "add1", ChangeType: diff.ChangeAdd},
			{OldNum: 4, NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m.modes.collapsed.enabled = true
		m.modes.collapsed.expandedHunks = map[int]bool{}

		// visible indices are 0 (ctx), 3 (add), 4 (ctx2). indices 1 and 2 are
		// hidden removes (mixed hunk, not delete-only) and can't be cursor targets.
		for _, i := range []int{0, 3, 4} {
			m.nav.diffCursor = i
			m.annot.cursorOnAnnotation = false
			top, _ := m.cursorVisualRange()
			idx, onAnn := m.visualRowToDiffLine(top)
			assert.Equal(t, i, idx, "collapsed-mode round-trip for visible cursor %d", i)
			assert.False(t, onAnn)
		}
	})
}

func TestModel_AnnotationPrefixBody(t *testing.T) {
	newModel := func() Model {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.layout.width = 120
		m.layout.treeWidth = 20
		m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
		return m
	}

	t.Run("file-level annotation returns file prefix", func(t *testing.T) {
		m := newModel()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: "header note"})
		defer m.store.Delete("a.go", 0, "")

		prefix, body := m.annotationPrefixBody(annotKeyFile)
		assert.Equal(t, "\U0001f4ac file: ", prefix)
		assert.Equal(t, "header note", body)
	})

	t.Run("line-level annotation returns line prefix", func(t *testing.T) {
		m := newModel()
		m.store.Add(annotation.Annotation{File: "a.go", Line: 7, Type: "+", Comment: "added"})
		defer m.store.Delete("a.go", 7, "+")

		prefix, body := m.annotationPrefixBody(m.annotationKey(7, "+"))
		assert.Equal(t, "\U0001f4ac ", prefix)
		assert.Equal(t, "added", body)
	})

	t.Run("no matching annotation returns empty pair", func(t *testing.T) {
		m := newModel()
		prefix, body := m.annotationPrefixBody(m.annotationKey(99, "+"))
		assert.Empty(t, prefix)
		assert.Empty(t, body)
	})

	t.Run("file key with no file annotation returns empty", func(t *testing.T) {
		m := newModel()
		// store has only a line-level annotation, no file-level
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: " ", Comment: "ctx"})
		defer m.store.Delete("a.go", 3, " ")

		prefix, body := m.annotationPrefixBody(annotKeyFile)
		assert.Empty(t, prefix)
		assert.Empty(t, body)
	})
}

func TestModel_AnnotationVisualRows(t *testing.T) {
	newModel := func() Model {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.layout.width = 120
		m.layout.treeWidth = 20
		m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
		return m
	}

	t.Run("empty body emits prefix-only row", func(t *testing.T) {
		// preload via --annotations may produce empty-body records; the chokepoint
		// must emit one styled row containing the prefix alone so paint and
		// height (wrappedAnnotationLineCount) stay in sync. matches master's
		// behavior where comment = prefix+body is non-empty for any real prefix.
		m := newModel()
		rows := m.annotationVisualRows("\U0001f4ac ", "")
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0], "\U0001f4ac")
	})

	t.Run("single-line body returns one row", func(t *testing.T) {
		m := newModel()
		rows := m.annotationVisualRows("\U0001f4ac ", "hello")
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0], "hello")
		assert.Contains(t, rows[0], "\U0001f4ac")
	})

	t.Run("multi-line body splits on newline", func(t *testing.T) {
		m := newModel()
		rows := m.annotationVisualRows("\U0001f4ac ", "one\ntwo\nthree")
		require.Len(t, rows, 3)
		assert.Contains(t, rows[0], "one")
		assert.Contains(t, rows[1], "two")
		assert.Contains(t, rows[2], "three")
	})

	t.Run("wrap-needed body produces multiple rows", func(t *testing.T) {
		m := newModel()
		m.layout.width = 40
		m.layout.treeWidth = 8
		long := strings.Repeat("alpha ", 20)
		rows := m.annotationVisualRows("\U0001f4ac ", long)
		assert.Greater(t, len(rows), 1, "long body must wrap to more than one row")
	})

	t.Run("prefix affects row-0 content", func(t *testing.T) {
		m := newModel()
		fileRows := m.annotationVisualRows("\U0001f4ac file: ", "x")
		lineRows := m.annotationVisualRows("\U0001f4ac ", "x")
		require.Len(t, fileRows, 1)
		require.Len(t, lineRows, 1)
		assert.Contains(t, fileRows[0], "file:")
		assert.NotContains(t, lineRows[0], "file:")
	})

	t.Run("cache hit returns identical slice on repeat call", func(t *testing.T) {
		m := newModel()
		first := m.annotationVisualRows("\U0001f4ac ", "cached body")
		second := m.annotationVisualRows("\U0001f4ac ", "cached body")
		// identical backing array means a real cache hit; value-equality would
		// pass even if the cache short-circuit was removed (recompute returns
		// equal bytes), so compare backing-array addresses via &slice[0].
		require.Len(t, first, len(second))
		require.NotEmpty(t, first)
		assert.Same(t, &first[0], &second[0],
			"second call must return the same backing array (true cache hit)")
		assert.Len(t, m.annot.rowCache, 1, "exactly one cache entry")
	})

	t.Run("different width creates new cache entry", func(t *testing.T) {
		m := newModel()
		// first call at wider pane
		m.annotationVisualRows("\U0001f4ac ", "body")
		assert.Len(t, m.annot.rowCache, 1)
		// shrink pane → new wrap width → new cache key
		m.layout.width = 40
		m.layout.treeWidth = 8
		m.annotationVisualRows("\U0001f4ac ", "body")
		assert.Len(t, m.annot.rowCache, 2)
	})

	t.Run("different prefix creates new cache entry", func(t *testing.T) {
		m := newModel()
		m.annotationVisualRows("\U0001f4ac ", "body")
		m.annotationVisualRows("\U0001f4ac file: ", "body")
		assert.Len(t, m.annot.rowCache, 2)
	})

	t.Run("different body creates new cache entry", func(t *testing.T) {
		m := newModel()
		m.annotationVisualRows("\U0001f4ac ", "first")
		m.annotationVisualRows("\U0001f4ac ", "second")
		assert.Len(t, m.annot.rowCache, 2)
	})
}

func TestModel_InvalidateAnnotationRows(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 120
	m.layout.treeWidth = 20

	m.annotationVisualRows("\U0001f4ac ", "one")
	m.annotationVisualRows("\U0001f4ac ", "two")
	require.Len(t, m.annot.rowCache, 2)

	m.invalidateAnnotationRows()
	assert.Empty(t, m.annot.rowCache)

	// cache must be usable after invalidation (not nil-mapped into a no-op)
	m.annotationVisualRows("\U0001f4ac ", "after")
	assert.Len(t, m.annot.rowCache, 1)
}

// TestModel_WrappedAnnotationLineCount_MatchesChokepoint pins the height-vs-paint
// invariant after the chokepoint refactor: wrappedAnnotationLineCount(key) must
// equal len(annotationVisualRows(prefix, body)) for every annotation shape. this
// is the type-system replacement for the docs-and-test-suite invariant guard
// the old forked implementation relied on.
func TestModel_WrappedAnnotationLineCount_MatchesChokepoint(t *testing.T) {
	newModel := func(width int) Model {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.layout.width = width
		m.layout.treeWidth = 8
		m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
		return m
	}

	long := strings.Repeat("alpha ", 20)

	tests := []struct {
		name      string
		width     int
		line      int
		typ       string
		body      string
		fileLevel bool
	}{
		{"line-level single short", 120, 1, " ", "hi", false},
		{"line-level multi-line plain", 120, 1, " ", "one\ntwo\nthree", false},
		{"line-level wrap-needed", 40, 1, " ", long, false},
		{"line-level multi-line with wrap", 40, 1, " ", long + "\nbrief", false},
		{"line-level empty body", 120, 1, " ", "", false},
		{"file-level single line", 120, 0, "", "header", true},
		{"file-level multi-line", 120, 0, "", "line1\nline2", true},
		{"file-level multi-line wrap continuation", 40, 0, "", "short\n" + long, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(tc.width)
			m.store.Add(annotation.Annotation{File: "a.go", Line: tc.line, Type: tc.typ, Comment: tc.body})
			defer m.store.Delete("a.go", tc.line, tc.typ)

			var key string
			if tc.fileLevel {
				key = annotKeyFile
			} else {
				key = m.annotationKey(tc.line, tc.typ)
			}
			prefix, body := m.annotationPrefixBody(key)
			rows := m.annotationVisualRows(prefix, body)
			count := m.wrappedAnnotationLineCount(key)
			assert.Equal(t, len(rows), count, "wrappedAnnotationLineCount must equal len(annotationVisualRows)")
		})
	}
}

// TestModel_HandleFileLoaded_InvalidatesAnnotationRows pins the file-load
// invalidation hook. handleFileLoaded must call invalidateAnnotationRows so
// per-file annotation sets don't leak cached rows across files.
func TestModel_HandleFileLoaded_InvalidatesAnnotationRows(t *testing.T) {
	m := testModel([]string{"a.go", "b.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.file.name = "a.go"
	m.layout.width = 120
	m.layout.treeWidth = 20

	// populate the cache from the "current file" state
	m.annotationVisualRows("\U0001f4ac ", "one")
	m.annotationVisualRows("\U0001f4ac ", "two")
	require.Len(t, m.annot.rowCache, 2)

	lines := []diff.DiffLine{{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "b.go", lines: lines})
	model := result.(Model)

	assert.Empty(t, model.annot.rowCache, "cache must be cleared after file load")
}

// TestModel_ApplyTheme_InvalidatesAnnotationRows pins the theme-apply
// invalidation hook. cached rows bake in AnnotationInline resolver styling, so
// applyTheme must clear the cache or stale colors persist.
func TestModel_ApplyTheme_InvalidatesAnnotationRows(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	highlighter := &mocks.SyntaxHighlighterMock{
		HighlightLinesFunc: func(string, []diff.DiffLine) []string { return nil },
		SetStyleFunc:       func(string) bool { return true },
		StyleNameFunc:      func() string { return "orig-style" },
	}
	m := testNewModel(t, renderer, annotation.NewStore(), highlighter, ModelConfig{
		TreeWidthRatio: 3, Overlay: overlay.NewManager(),
	})
	m.file.name = "a.go"
	m.layout.width = 120
	m.layout.treeWidth = 20

	m.annotationVisualRows("\U0001f4ac ", "one")
	m.annotationVisualRows("\U0001f4ac ", "two")
	require.Len(t, m.annot.rowCache, 2)

	m.applyTheme(ThemeSpec{
		Colors: style.Colors{
			Accent: "#bd93f9", Border: "#6272a4", Normal: "#f8f8f2", Muted: "#6272a4",
			SelectedFg: "#f8f8f2", SelectedBg: "#44475a", Annotation: "#f1fa8c",
			CursorFg: "#282a36", CursorBg: "#f8f8f2",
			AddFg: "#50fa7b", AddBg: "#2a4a2a", RemoveFg: "#ff5555", RemoveBg: "#4a2a2a",
			ModifyFg: "#ffb86c", ModifyBg: "#3a3a2a",
			TreeBg: "#21222c", DiffBg: "#282a36",
			StatusFg: "#f8f8f2", StatusBg: "#44475a",
			SearchFg: "#282a36", SearchBg: "#f1fa8c",
		},
		ChromaStyle: "dracula",
	})

	assert.Empty(t, m.annot.rowCache, "cache must be cleared after applyTheme")
}

// TestModel_AnnotationVisualRows_EmptyBodyEmitsPrefixRow pins the regression caught
// by codex (PR review round 3): --annotations preload accepts empty bodies, and
// master rendered a visible prefix-only row for them. an earlier version of the
// chokepoint short-circuited on body == "" and returned nil, which desynced the
// height query (wrappedAnnotationLineCount returned 1) from paint (zero rows).
// the chokepoint must emit exactly one styled row containing the prefix alone.
func TestModel_AnnotationVisualRows_EmptyBodyEmitsPrefixRow(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 120
	m.layout.treeWidth = 20
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	// line-level prefix with empty body emits exactly one row (the styled prefix).
	rows := m.annotationVisualRows("\U0001f4ac ", "")
	require.Len(t, rows, 1, "empty body must emit one prefix-only row, not zero")
	assert.Contains(t, rows[0], "\U0001f4ac")

	// wrappedAnnotationLineCount agrees with paint when annotation is in the store.
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: ""})
	count := m.wrappedAnnotationLineCount(m.annotationKey(1, " "))
	assert.Equal(t, 1, count, "height query must match paint (1 row) for empty body")
}

// TestModel_FileAnnotation_EmptyBody_Renders pins the regression caught by codex
// (PR review round 3, file-level parallel of the line-level fix): a file-level
// annotation with an empty body loaded via --annotations was reserving a viewport
// row via hasFileAnnotation+wrappedAnnotationLineCount, but renderFileAnnotationHeader
// short-circuited on fileComment != "" and emitted zero rows. paint must match
// the height query: emit one prefix-only row for the file annotation header.
func TestModel_FileAnnotation_EmptyBody_Renders(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.layout.width = 120
	m.layout.treeWidth = 20
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m.layout.focus = paneDiff

	m.store.Add(annotation.Annotation{File: "a.go", Line: 0, Type: "", Comment: ""})
	defer m.store.Delete("a.go", 0, "")

	assert.True(t, m.hasFileAnnotation(), "empty-body file annotation must register as present")
	assert.Equal(t, 1, m.wrappedAnnotationLineCount(annotKeyFile), "height query must return 1 row for empty body")

	rendered := m.renderDiff()
	// the file annotation header must emit a row containing the file prefix even with empty body
	assert.Contains(t, rendered, "\U0001f4ac file:", "empty-body file annotation must paint the prefix-only row")
}

// TestModel_AnnotationVisualRows_ByteEquivalentToMaster is the regression safety net
// for the chokepoint refactor. expected bytes were captured from the pre-refactor
// master code path (commit 28e6d8f) using a one-off capture program that
// replicated the wrap+style inner loop of renderWrappedAnnotation BEFORE cursor
// prepend and BEFORE extendLineBg padding — i.e. exactly what the chokepoint
// annotationVisualRows must return. if this test ever fails after a code change,
// DO NOT regenerate the expected bytes from the current code; investigate why
// the refactored chokepoint diverges from master.
//
// model setup matches the capture program: testModel defaults + explicit
// width/treeWidth overrides per case, so diffContentWidth() returns the same
// value (wrapWidth = diffContentWidth() - 1 = 93 wide, 25 narrow).
func TestModel_AnnotationVisualRows_ByteEquivalentToMaster(t *testing.T) {
	type caseDef struct {
		name      string
		width     int
		treeWidth int
		prefix    string
		body      string
		expected  string
	}

	cases := []caseDef{
		{
			name:      "single-line line-level",
			width:     120,
			treeWidth: 20,
			prefix:    "\U0001f4ac ",
			body:      "hello",
			expected:  "\x1b[3m\U0001f4ac hello\x1b[23m",
		},
		{
			name:      "multi-line line-level",
			width:     120,
			treeWidth: 20,
			prefix:    "\U0001f4ac ",
			body:      "one\ntwo\nthree",
			expected:  "\x1b[3m\U0001f4ac one\x1b[23m\n\x1b[3m   two\x1b[23m\n\x1b[3m   three\x1b[23m",
		},
		{
			name:      "wrap-needed line-level narrow",
			width:     40,
			treeWidth: 8,
			prefix:    "\U0001f4ac ",
			body:      strings.Repeat("alpha ", 20),
			expected: "\x1b[3m\U0001f4ac alpha alpha alpha\x1b[23m\n" +
				"\x1b[3malpha alpha alpha alpha\x1b[23m\n" +
				"\x1b[3malpha alpha alpha alpha\x1b[23m\n" +
				"\x1b[3malpha alpha alpha alpha\x1b[23m\n" +
				"\x1b[3malpha alpha alpha alpha\x1b[23m\n" +
				"\x1b[3malpha \x1b[23m",
		},
		{
			name:      "multi-segment wrap on continuation narrow",
			width:     40,
			treeWidth: 8,
			prefix:    "\U0001f4ac ",
			body:      "short\n" + strings.Repeat("beta ", 20),
			expected: "\x1b[3m\U0001f4ac short\x1b[23m\n" +
				"\x1b[3m   beta beta beta beta\x1b[23m\n" +
				"\x1b[3mbeta beta beta beta beta\x1b[23m\n" +
				"\x1b[3mbeta beta beta beta beta\x1b[23m\n" +
				"\x1b[3mbeta beta beta beta beta\x1b[23m\n" +
				"\x1b[3mbeta \x1b[23m",
		},
		{
			name:      "single-line file-level",
			width:     120,
			treeWidth: 20,
			prefix:    "\U0001f4ac file: ",
			body:      "header",
			expected:  "\x1b[3m\U0001f4ac file: header\x1b[23m",
		},
		{
			name:      "multi-line file-level",
			width:     120,
			treeWidth: 20,
			prefix:    "\U0001f4ac file: ",
			body:      "line1\nline2",
			expected:  "\x1b[3m\U0001f4ac file: line1\x1b[23m\n\x1b[3m         line2\x1b[23m",
		},
		{
			// empty-body annotations are produced by --annotations preload when
			// the source file has a header followed by no body lines. master
			// rendered a visible prefix-only row in this case; the chokepoint
			// must too, so paint and wrappedAnnotationLineCount stay in sync.
			name:      "empty-body line-level",
			width:     120,
			treeWidth: 20,
			prefix:    "\U0001f4ac ",
			body:      "",
			expected:  "\x1b[3m\U0001f4ac \x1b[23m",
		},
		{
			// file-level parallel of empty-body line-level: same regression
			// caught by codex round 3. paint must emit one prefix-only row.
			name:      "empty-body file-level",
			width:     120,
			treeWidth: 20,
			prefix:    "\U0001f4ac file: ",
			body:      "",
			expected:  "\x1b[3m\U0001f4ac file: \x1b[23m",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.file.name = "a.go"
			m.layout.width = tc.width
			m.layout.treeWidth = tc.treeWidth
			m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

			rows := m.annotationVisualRows(tc.prefix, tc.body)
			got := strings.Join(rows, "\n")
			assert.Equal(t, tc.expected, got, "annotationVisualRows must match pre-refactor master byte-for-byte")
		})
	}
}
