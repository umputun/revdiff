package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/mocks"
)

func TestModel_LineNumGutter(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumbers = true
	m.lineNumWidth = 3

	tests := []struct {
		name string
		dl   diff.DiffLine
		want string // plain text content (ANSI stripped)
	}{
		{
			name: "context line",
			dl:   diff.DiffLine{OldNum: 25, NewNum: 32, ChangeType: diff.ChangeContext},
			want: "  25  32", // " " + " 25" + " " + " 32"
		},
		{
			name: "add line",
			dl:   diff.DiffLine{OldNum: 0, NewNum: 40, ChangeType: diff.ChangeAdd},
			want: "      40", // " " + "   " + " " + " 40"
		},
		{
			name: "remove line",
			dl:   diff.DiffLine{OldNum: 40, NewNum: 0, ChangeType: diff.ChangeRemove},
			want: "  40    ", // " " + " 40" + " " + "   "
		},
		{
			name: "divider",
			dl:   diff.DiffLine{ChangeType: diff.ChangeDivider},
			want: "        ", // " " + "   " + " " + "   "
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.lineNumGutter(tt.dl)
			stripped := ansi.Strip(got)
			assert.Equal(t, tt.want, stripped)
		})
	}
}

func TestModel_RenderDiffLineWithLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumbers = true
	m.lineNumWidth = 2
	m.focus = paneDiff
	m.diffLines = []diff.DiffLine{
		{OldNum: 5, NewNum: 5, Content: "hello", ChangeType: diff.ChangeContext},
		{OldNum: 6, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
		{OldNum: 0, NewNum: 6, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m.highlightedLines = nil

	rendered := m.renderDiff()
	stripped := ansi.Strip(rendered)

	assert.Contains(t, stripped, " 5  5")
	assert.Contains(t, stripped, " 6    ")
	assert.Contains(t, stripped, "    6")
}

func TestModel_RenderDiffLineWithoutLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumbers = false
	m.diffLines = []diff.DiffLine{
		{OldNum: 5, NewNum: 5, Content: "hello", ChangeType: diff.ChangeContext},
	}

	rendered := m.renderDiff()
	stripped := ansi.Strip(rendered)

	// should NOT contain number gutter, just the prefix
	assert.NotContains(t, stripped, " 5  5")
	assert.Contains(t, stripped, "hello")
}

func TestModel_RenderWrappedDiffLineWithLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumbers = true
	m.lineNumWidth = 2
	m.wrapMode = true
	m.focus = paneDiff
	m.width = 50
	m.treeWidth = 0
	m.singleFile = true
	m.diffLines = []diff.DiffLine{
		{OldNum: 5, NewNum: 5, Content: "short", ChangeType: diff.ChangeContext},
	}

	rendered := m.renderDiff()
	stripped := ansi.Strip(rendered)

	// first line should have numbers
	assert.Contains(t, stripped, " 5  5")
}

func TestModel_LineNumGutterWidth(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumWidth = 3
	// width = 1 (leading space) + 3 (old) + 1 (space) + 3 (new) = 8
	assert.Equal(t, 8, m.lineNumGutterWidth())

	m.lineNumWidth = 1
	// width = 1 + 1 + 1 + 1 = 4
	assert.Equal(t, 4, m.lineNumGutterWidth())
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

func TestModel_ExtendLineBg(t *testing.T) {
	t.Run("empty bgColor is no-op", func(t *testing.T) {
		m := testModel(nil, nil)
		m.width = 80
		assert.Equal(t, "hello", m.extendLineBg("hello", ""))
	})

	t.Run("pads to content width", func(t *testing.T) {
		m := testModel(nil, nil)
		m.width = 80
		result := m.extendLineBg("hi", "#2e3440")
		assert.Contains(t, result, "\033[48;2;46;52;64m")
		assert.Contains(t, result, "\033[49m")
		w := lipgloss.Width(result)
		assert.Greater(t, w, 2, "should be wider than input")
	})

	t.Run("with line numbers subtracts gutter", func(t *testing.T) {
		m := testModel(nil, nil)
		m.width = 80
		m.lineNumbers = true
		m.lineNumWidth = 3
		resultWithNums := m.extendLineBg("hi", "#2e3440")
		m.lineNumbers = false
		resultWithout := m.extendLineBg("hi", "#2e3440")
		assert.Less(t, lipgloss.Width(resultWithNums), lipgloss.Width(resultWithout), "line numbers should reduce target width")
	})
}

func TestModel_RenderDiffLineHighlighted(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func foo() {}", ChangeType: diff.ChangeAdd},
		{OldNum: 2, Content: "func bar() {}", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.highlightedLines = []string{"hl-context", "hl-add", "hl-remove"}
	m.focus = paneDiff
	output := m.renderDiff()

	assert.Contains(t, output, "hl-context", "highlighted context line should appear")
	assert.Contains(t, output, "hl-add", "highlighted add line should appear")
	assert.Contains(t, output, "hl-remove", "highlighted remove line should appear")
}

func TestModel_RenderDiffLineCursorHighlight(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "line two", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.focus = paneDiff
	m.diffCursor = 0
	output := m.renderDiff()
	assert.Contains(t, output, "▶", "cursor indicator should appear on active line")
	assert.Contains(t, output, "line one", "cursor line content should appear")
}

func TestModel_RenderDiffLineTabReplacement(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "\tfoo", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)
	m.tabSpaces = "    " // 4 spaces
	output := m.renderDiff()
	assert.Contains(t, output, "    foo", "tabs should be replaced with spaces")
	assert.NotContains(t, output, "\t", "no raw tabs should remain")
}

func TestModel_PlainStyles(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return []diff.FileEntry{{Path: "a.go"}}, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{NoColors: true, TreeWidthRatio: 3})
	m.width = 120
	m.height = 40
	m.treeWidth = 36
	m.ready = true
	// plain styles should not panic and should render
	output := m.View()
	assert.NotEmpty(t, output)
}

func TestModel_TabWidthDefault(t *testing.T) {
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc:     func(string, string, bool) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TabWidth: 0})
	assert.Equal(t, "    ", m.tabSpaces, "tab width 0 should default to 4 spaces")

	m2 := NewModel(renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{TabWidth: 2})
	assert.Equal(t, "  ", m2.tabSpaces, "tab width 2 should produce 2 spaces")
}

func TestModel_StyleDiffContent(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()

	t.Run("add line", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, false)
		assert.Contains(t, result, " + content")
	})

	t.Run("remove line", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeRemove, " - ", "content", false, false)
		assert.Contains(t, result, " - content")
	})

	t.Run("context line", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeContext, "   ", "content", false, false)
		assert.Contains(t, result, "   content")
	})

	t.Run("highlighted add", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeAdd, " + ", "\033[32mgreen\033[0m", true, false)
		assert.Contains(t, result, " + ")
		assert.Contains(t, result, "\033[32m")
	})
}
