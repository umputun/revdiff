package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestModel_ApplyHorizontalScrollTruncatesLongLines(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0

	// content wider than diffContentWidth should be truncated
	longContent := strings.Repeat("x", 200)
	result := m.applyHorizontalScroll(longContent)
	maxWidth := m.diffContentWidth() - m.gutterExtra()
	resultWidth := lipgloss.Width(result)
	assert.LessOrEqual(t, resultWidth, maxWidth, "long line should be truncated to content width even with scrollX=0")
}

func TestModel_ExtendLineBgAfterScrollFillsWidth(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 10
	m.styles = plainStyles()

	// simulate a styled add line longer than content width
	longContent := strings.Repeat("x", 200)
	scrolled := m.applyHorizontalScroll(longContent)
	extended := m.extendLineBg(scrolled, "#2e3440")

	expectedWidth := m.diffContentWidth() - m.gutterExtra()
	resultWidth := lipgloss.Width(extended)
	assert.Equal(t, expectedWidth, resultWidth, "bg should fill to full content width after scroll")
}

func TestModel_ChangeBgColor(t *testing.T) {
	m := testModel(nil, nil)
	assert.Equal(t, m.styles.colors.AddBg, m.changeBgColor(diff.ChangeAdd))
	assert.Equal(t, m.styles.colors.RemoveBg, m.changeBgColor(diff.ChangeRemove))
	assert.Empty(t, m.changeBgColor(diff.ChangeContext))
	assert.Empty(t, m.changeBgColor(diff.ChangeDivider))
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

func TestModel_WrapContent_ANSIStatePreservation(t *testing.T) {
	m := testModel(nil, nil)

	t.Run("fg color carries across wrap boundary", func(t *testing.T) {
		// simulate chroma-highlighted long token: fg set, long text, fg reset
		content := "\033[38;2;100;200;50mthis is a very long green token that must wrap\033[39m"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1, "should wrap into multiple lines")
		// continuation lines must start with the active fg sequence
		for i := 1; i < len(lines); i++ {
			assert.Contains(t, lines[i], "\033[38;2;100;200;50m",
				"continuation line %d should have fg color re-emitted", i)
		}
	})

	t.Run("bold carries across wrap boundary", func(t *testing.T) {
		content := "\033[1mthis is a bold token that should wrap at boundary\033[22m"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		for i := 1; i < len(lines); i++ {
			assert.Contains(t, lines[i], "\033[1m", "continuation line %d should have bold re-emitted", i)
		}
	})

	t.Run("italic carries across wrap boundary", func(t *testing.T) {
		content := "\033[3mthis is italic text that should wrap properly\033[23m"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		for i := 1; i < len(lines); i++ {
			assert.Contains(t, lines[i], "\033[3m", "continuation line %d should have italic re-emitted", i)
		}
	})

	t.Run("fg reset before wrap means no carry", func(t *testing.T) {
		// fg is set and reset on the first segment, second segment should have no fg
		content := "\033[32mshort\033[39m and then some more plain text that wraps here"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		// first line has the color, continuation should NOT re-emit it (already reset)
		assert.NotContains(t, lines[len(lines)-1], "\033[32m", "reset fg should not carry")
	})

	t.Run("multiple fg changes across wrap", func(t *testing.T) {
		// first token green, then red token that wraps
		content := "\033[32mhi\033[39m \033[31mthis red token is long enough to wrap over\033[39m"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		// the last line should carry the red fg, not green
		assert.Contains(t, lines[len(lines)-1], "\033[31m", "should carry the last active fg color")
		assert.NotContains(t, lines[len(lines)-1], "\033[32m", "should not carry the first fg color")
	})

	t.Run("full reset clears all state before wrap", func(t *testing.T) {
		content := "\033[38;2;100;200;50m\033[1m\033[3mstyled text\033[0m and then plain text long enough to wrap here"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		// after \033[0m, no state should carry to continuation lines
		last := lines[len(lines)-1]
		assert.NotContains(t, last, "\033[38;2;100;200;50m", "fg should not carry after full reset")
		assert.NotContains(t, last, "\033[1m", "bold should not carry after full reset")
		assert.NotContains(t, last, "\033[3m", "italic should not carry after full reset")
	})

	t.Run("bare reset ESC[m clears all state", func(t *testing.T) {
		content := "\033[38;2;100;200;50m\033[1mstyled text\033[m and then plain wrapping text here"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		last := lines[len(lines)-1]
		assert.NotContains(t, last, "\033[38;2;100;200;50m", "fg should not carry after bare reset")
		assert.NotContains(t, last, "\033[1m", "bold should not carry after bare reset")
	})

	t.Run("no ANSI content unchanged", func(t *testing.T) {
		content := "plain text that is long enough to wrap at the boundary"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		// no ANSI codes should appear
		for _, line := range lines {
			assert.NotContains(t, line, "\033[", "plain text should have no ANSI injected")
		}
	})
}
