package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	result := m.applyHorizontalScroll(longContent, "")
	// when right overflow is present, output extends 1 col beyond cutWidth into the pane's right
	// padding column so the indicator sits flush against the border
	maxWidth := m.diffContentWidth() - m.gutterExtra() + 1
	resultWidth := lipgloss.Width(result)
	assert.LessOrEqual(t, resultWidth, maxWidth, "long line should be truncated to content width (+1 for flush indicator)")
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
	scrolled := m.applyHorizontalScroll(longContent, "#2e3440")
	extended := m.extendLineBg(scrolled, "#2e3440")

	// scrollX > 0 and overflow on both sides: right indicator extends by 1 col into pane padding
	expectedWidth := m.diffContentWidth() - m.gutterExtra() + 1
	resultWidth := lipgloss.Width(extended)
	assert.Equal(t, expectedWidth, resultWidth, "scroll output should fill content width plus the flush right indicator col")
}

func TestModel_ExtendLineBgWithoutOverflowFillsWidth(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0
	m.styles = plainStyles()

	// short content with no overflow gets padded by extendLineBg to full cut width (no indicator extension)
	shortContent := "hello"
	scrolled := m.applyHorizontalScroll(shortContent, "#2e3440")
	extended := m.extendLineBg(scrolled, "#2e3440")

	expectedWidth := m.diffContentWidth() - m.gutterExtra()
	resultWidth := lipgloss.Width(extended)
	assert.Equal(t, expectedWidth, resultWidth, "without overflow, bg should fill exactly to cut width")
}

func TestModel_ApplyHorizontalScrollShowsRightIndicator(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0

	// content wider than viewport should get a right-pointing indicator with a leading space
	longContent := strings.Repeat("x", 200)
	result := m.applyHorizontalScroll(longContent, "#2e3440")
	plain := ansi.Strip(result)
	assert.Contains(t, plain, "»", "right indicator should appear when content overflows right")
	assert.NotContains(t, plain, "«", "left indicator should not appear when scrollX is 0")
	assert.True(t, strings.HasSuffix(plain, " »"), "right indicator should have a leading space separator from content")

	// result extends exactly 1 col beyond cut width to place the arrow flush against the right border
	expectedWidth := m.diffContentWidth() - m.gutterExtra() + 1
	resultWidth := lipgloss.Width(result)
	assert.Equal(t, expectedWidth, resultWidth, "result width should equal cutWidth+1 when right overflow is present")
}

func TestModel_ApplyHorizontalScrollShowsBothIndicators(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 50

	// scrolling right with content longer than scrollX+cutWidth triggers both overflows
	longContent := strings.Repeat("x", 200)
	result := m.applyHorizontalScroll(longContent, "#2e3440")
	plain := ansi.Strip(result)
	assert.Contains(t, plain, "«", "left indicator should appear when scrolled right with hidden content on the left")
	assert.Contains(t, plain, "»", "right indicator should still appear when content also overflows right")
}

func TestModel_ApplyHorizontalScrollLeftOnlyOverflow(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 50

	// content of exactly scrollX+cutWidth (126 chars) at scrollX=50: end=126, origWidth=126
	// hasLeftOverflow: 126 > 50 = true; hasRightOverflow: 126 > 126 = false
	// left-only path: total visible width should equal cutWidth (no +1 extension)
	cutWidth := m.diffContentWidth() - m.gutterExtra()
	content := strings.Repeat("x", m.scrollX+cutWidth)
	result := m.applyHorizontalScroll(content, "#2e3440")
	plain := ansi.Strip(result)
	assert.Contains(t, plain, "«", "left indicator should appear when scrolled past hidden content on the left")
	assert.NotContains(t, plain, "»", "right indicator should not appear when content fits within viewport end")
	assert.True(t, strings.HasPrefix(plain, "«"), "left indicator should be the first visible char")

	// total width should equal cutWidth exactly (no +1 extension since no right overflow)
	assert.Equal(t, cutWidth, lipgloss.Width(result), "left-only overflow should not trigger the +1 right padding extension")
}

func TestModel_ApplyHorizontalScrollWithLineNumberGutter(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0
	m.lineNumbers = true
	m.lineNumWidth = 3 // gutter width = 2*3 + 2 = 8

	// with gutters enabled, cutWidth = diffContentWidth - gutterExtra = 76 - 8 = 68
	// right-overflow extends by 1 col into pane padding: total = cutWidth + 1 = 69
	longContent := strings.Repeat("x", 200)
	result := m.applyHorizontalScroll(longContent, "#2e3440")
	plain := ansi.Strip(result)
	assert.Contains(t, plain, "»", "right indicator should appear with gutters enabled")

	expectedWidth := m.diffContentWidth() - m.gutterExtra() + 1
	assert.Equal(t, expectedWidth, lipgloss.Width(result), "gutter-adjusted cut width + 1 for flush right indicator")
	assert.Equal(t, 8, m.gutterExtra(), "sanity check: gutterExtra computed from lineNumWidth")
}

func TestModel_ApplyHorizontalScrollNarrowViewportFallback(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 14
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 10
	m.lineNumbers = true
	m.lineNumWidth = 3 // gutter width = 8, cutWidth = max(10, 14-4) - 8 = 2

	// cutWidth=2 with both overflows: innerStart = start+1 = 11, innerEnd = end-1 = 11
	// innerEnd <= innerStart -> fallback to plain cut (no indicators)
	require.Equal(t, 2, m.diffContentWidth()-m.gutterExtra(), "test precondition: cutWidth=2")
	longContent := strings.Repeat("x", 200)
	assert.NotPanics(t, func() {
		result := m.applyHorizontalScroll(longContent, "#2e3440")
		plain := ansi.Strip(result)
		// fallback path returns plain cut; no indicators present
		assert.NotContains(t, plain, "«", "narrow viewport fallback should drop indicators")
		assert.NotContains(t, plain, "»", "narrow viewport fallback should drop indicators")
	})
}

func TestModel_ApplyHorizontalScrollNoIndicatorForShortLines(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0

	// short content that fits entirely within the viewport should have no indicators
	result := m.applyHorizontalScroll("short", "#2e3440")
	plain := ansi.Strip(result)
	assert.NotContains(t, plain, "»", "no right indicator for short lines")
	assert.NotContains(t, plain, "«", "no left indicator for short lines")
}

func TestModel_ApplyHorizontalScrollNoLeftIndicatorWhenScrolledPastContent(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 100

	// content shorter than scrollX should not show a left indicator (nothing to the left is visible)
	result := m.applyHorizontalScroll("short", "#2e3440")
	plain := ansi.Strip(result)
	assert.NotContains(t, plain, "«", "left indicator should not appear when content ends before viewport start")
	assert.NotContains(t, plain, "»", "right indicator should not appear when content ends before viewport")
}

func TestModel_ApplyHorizontalScrollIndicatorEmptyLineBgSkipsBgReset(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0

	// empty indicatorBg: scrollIndicatorANSI should not emit \033[49m because no bg was set,
	// so the caller's inherited bg is preserved
	longContent := strings.Repeat("x", 200)
	result := m.applyHorizontalScroll(longContent, "")
	// verify no bg reset sequence is emitted when we didn't set a bg
	assert.NotContains(t, result, "\033[49m", "empty lineBg should not emit \\033[49m bg reset")
	// glyph must still be present
	assert.Contains(t, ansi.Strip(result), "»", "indicator glyph should still render with empty lineBg")
}

func TestModel_ApplyHorizontalScrollIndicatorInNoColorsMode(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 80
	m.singleFile = true
	m.treeWidth = 0
	m.scrollX = 0
	m.noColors = true

	longContent := strings.Repeat("x", 200)
	result := m.applyHorizontalScroll(longContent, "#2e3440")
	assert.Contains(t, result, "\033[7m", "no-colors mode should use reverse video for indicator")
	assert.Contains(t, ansi.Strip(result), "»", "indicator glyph should still be visible in no-colors mode")
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

	t.Run("bg color carries across wrap boundary", func(t *testing.T) {
		content := "\033[48;2;80;40;40mthis is text with a background color that must wrap\033[49m"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1, "should wrap into multiple lines")
		for i := 1; i < len(lines); i++ {
			assert.Contains(t, lines[i], "\033[48;2;80;40;40m",
				"continuation line %d should have bg color re-emitted", i)
		}
	})

	t.Run("bg reset clears bg state before wrap", func(t *testing.T) {
		content := "\033[48;2;80;40;40mhighlighted\033[49m and then plain text that is long enough to wrap"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		last := lines[len(lines)-1]
		assert.NotContains(t, last, "\033[48;2;80;40;40m", "bg should not carry after bg reset")
	})

	t.Run("full reset clears bg state", func(t *testing.T) {
		content := "\033[48;2;80;40;40m\033[38;2;100;200;50mstyled\033[0m plain text long enough to wrap here"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		last := lines[len(lines)-1]
		assert.NotContains(t, last, "\033[48;2;80;40;40m", "bg should not carry after full reset")
		assert.NotContains(t, last, "\033[38;2;100;200;50m", "fg should not carry after full reset")
	})

	t.Run("reverse video carries across wrap boundary", func(t *testing.T) {
		content := "\033[7mthis is reverse video text that should wrap at boundary\033[27m"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		for i := 1; i < len(lines); i++ {
			assert.Contains(t, lines[i], "\033[7m", "continuation line %d should have reverse video re-emitted", i)
		}
	})

	t.Run("reverse video off clears state before wrap", func(t *testing.T) {
		content := "\033[7mhighlighted\033[27m and then plain text that is long enough to wrap"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		last := lines[len(lines)-1]
		assert.NotContains(t, last, "\033[7m", "reverse video should not carry after reset")
	})

	t.Run("full reset clears reverse video", func(t *testing.T) {
		content := "\033[7m\033[1mreverse bold\033[0m plain text that is long enough to wrap here"
		lines := m.wrapContent(content, 20)
		require.Greater(t, len(lines), 1)
		last := lines[len(lines)-1]
		assert.NotContains(t, last, "\033[7m", "reverse should not carry after full reset")
		assert.NotContains(t, last, "\033[1m", "bold should not carry after full reset")
	})
}

func TestModel_ApplyIntraLineHighlight(t *testing.T) {
	t.Run("paired add/remove lines get bg markers", func(t *testing.T) {
		m := testModel(nil, nil)
		m.styles = newStyles(Colors{AddBg: "#1a3320", RemoveBg: "#331a1a", WordAddBg: "#2d5a3a", WordRemoveBg: "#5a2d2d"})
		m.wordDiff = true
		m.diffLines = []diff.DiffLine{
			{OldNum: 1, Content: "hello world", ChangeType: diff.ChangeRemove},
			{NewNum: 1, Content: "hello earth", ChangeType: diff.ChangeAdd},
		}
		m.tabSpaces = "    "
		m.recomputeIntraRanges()

		// remove line should have word-diff ranges for "world"
		require.NotNil(t, m.intraRanges[0], "remove line should have intra-line ranges")
		result := m.applyIntraLineHighlight(0, diff.ChangeRemove, "hello world")
		assert.Contains(t, result, "\033[48;2;", "should contain bg ANSI sequence")

		// add line should have word-diff ranges for "earth"
		require.NotNil(t, m.intraRanges[1], "add line should have intra-line ranges")
		result = m.applyIntraLineHighlight(1, diff.ChangeAdd, "hello earth")
		assert.Contains(t, result, "\033[48;2;", "should contain bg ANSI sequence")
	})

	t.Run("pure add block produces no markers", func(t *testing.T) {
		m := testModel(nil, nil)
		m.styles = newStyles(Colors{AddBg: "#1a3320", WordAddBg: "#2d5a3a"})
		m.wordDiff = true
		m.diffLines = []diff.DiffLine{
			{NewNum: 1, Content: "new line one", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "new line two", ChangeType: diff.ChangeAdd},
		}
		m.tabSpaces = "    "
		m.recomputeIntraRanges()

		assert.Nil(t, m.intraRanges[0], "pure add should have no intra-line ranges")
		assert.Nil(t, m.intraRanges[1], "pure add should have no intra-line ranges")

		result := m.applyIntraLineHighlight(0, diff.ChangeAdd, "new line one")
		assert.Equal(t, "new line one", result, "should return unchanged content")
	})

	t.Run("no-color mode uses reverse-video", func(t *testing.T) {
		m := testModel(nil, nil)
		m.noColors = true
		m.styles = plainStyles()
		m.wordDiff = true
		m.diffLines = []diff.DiffLine{
			{OldNum: 1, Content: "hello world", ChangeType: diff.ChangeRemove},
			{NewNum: 1, Content: "hello earth", ChangeType: diff.ChangeAdd},
		}
		m.tabSpaces = "    "
		m.recomputeIntraRanges()

		require.NotNil(t, m.intraRanges[0])
		result := m.applyIntraLineHighlight(0, diff.ChangeRemove, "hello world")
		assert.Contains(t, result, "\033[7m", "no-color should use reverse video on")
		assert.Contains(t, result, "\033[27m", "no-color should use reverse video off")
	})

	t.Run("context lines are not highlighted", func(t *testing.T) {
		m := testModel(nil, nil)
		m.diffLines = []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "context", ChangeType: diff.ChangeContext},
		}
		m.intraRanges = [][]matchRange{{matchRange{0, 3}}} // fake ranges

		result := m.applyIntraLineHighlight(0, diff.ChangeContext, "context")
		assert.Equal(t, "context", result, "context lines should not get intra-line markers")
	})

	t.Run("out of range idx returns unchanged", func(t *testing.T) {
		m := testModel(nil, nil)
		m.intraRanges = nil
		result := m.applyIntraLineHighlight(5, diff.ChangeAdd, "text")
		assert.Equal(t, "text", result)
	})
}

func TestModel_RenderDiffWithIntraLine(t *testing.T) {
	t.Run("render hunk with paired lines includes bg markers", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "context line", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old value", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new value", ChangeType: diff.ChangeAdd},
		}
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.styles = newStyles(Colors{
			AddBg: "#1a3320", RemoveBg: "#331a1a",
			WordAddBg: "#2d5a3a", WordRemoveBg: "#5a2d2d",
			DiffBg: "#1e1e1e",
		})
		m.wordDiff = true
		result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
		m = result.(Model)

		// intraRanges should be computed by handleFileLoaded
		require.NotNil(t, m.intraRanges, "intra-line ranges should be computed")

		output := m.renderDiff()
		// "old" vs "new" are the changed words — the bg markers should appear
		assert.Contains(t, output, "\033[48;2;", "rendered output should contain bg color sequences")
	})

	t.Run("tab-containing lines have correct highlights", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 1, Content: "\treturn old", ChangeType: diff.ChangeRemove},
			{NewNum: 1, Content: "\treturn new", ChangeType: diff.ChangeAdd},
		}
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.styles = newStyles(Colors{
			AddBg: "#1a3320", RemoveBg: "#331a1a",
			WordAddBg: "#2d5a3a", WordRemoveBg: "#5a2d2d",
		})
		m.tabSpaces = "    "
		m.wordDiff = true
		result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
		m = result.(Model)

		require.NotNil(t, m.intraRanges)
		output := m.renderDiff()
		stripped := ansi.Strip(output)
		// tab should be replaced, and content should be present
		assert.Contains(t, stripped, "    return", "tabs should be replaced with spaces")
		// the word "old"/"new" should be highlighted differently
		assert.Contains(t, output, "\033[48;2;", "tab lines should have word-diff bg markers")
	})
}

func TestModel_WrapModeWithIntraLine(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, Content: "this is a long line with old word in it that needs to wrap because it is very long", ChangeType: diff.ChangeRemove},
		{NewNum: 1, Content: "this is a long line with new word in it that needs to wrap because it is very long", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.styles = newStyles(Colors{
		AddBg: "#1a3320", RemoveBg: "#331a1a",
		WordAddBg: "#2d5a3a", WordRemoveBg: "#5a2d2d",
	})
	m.wrapMode = true
	m.width = 50
	m.treeWidth = 0
	m.singleFile = true
	m.wordDiff = true

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	m = result.(Model)

	require.NotNil(t, m.intraRanges)
	output := m.renderDiff()
	// verify word-diff markers are present in wrapped output
	assert.Contains(t, output, "\033[48;2;", "wrapped output should contain word-diff bg markers")
}

func TestModel_WordDiffOptIn(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, Content: "old value here", ChangeType: diff.ChangeRemove},
		{NewNum: 1, Content: "new value here", ChangeType: diff.ChangeAdd},
	}
	colors := Colors{AddBg: "#1a3320", RemoveBg: "#331a1a", WordAddBg: "#2d5a3a", WordRemoveBg: "#5a2d2d"}

	t.Run("default off: no ranges computed", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.styles = newStyles(colors)
		result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
		m = result.(Model)

		assert.False(t, m.wordDiff, "wordDiff should default to false")
		assert.Nil(t, m.intraRanges, "intraRanges should be nil when wordDiff is off")
	})

	t.Run("enabled: ranges computed on file load and bg markers in render", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.styles = newStyles(colors)
		m.wordDiff = true
		result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
		m = result.(Model)

		require.NotNil(t, m.intraRanges, "intraRanges should be computed when wordDiff is on")
		assert.Contains(t, m.renderDiff(), "\033[48;2;", "rendered output should contain word-diff bg markers")
	})

	t.Run("toggleWordDiff flips state and recomputes", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.styles = newStyles(colors)
		m.ready = true
		m.width = 200
		m.height = 30
		m.viewport.Width = 196
		m.viewport.Height = 28
		result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
		m = result.(Model)
		m.focus = paneDiff

		assert.Nil(t, m.intraRanges, "initial state: no ranges")

		m.toggleWordDiff()
		assert.True(t, m.wordDiff, "should be enabled after toggle")
		assert.NotNil(t, m.intraRanges, "ranges computed after enabling")

		m.toggleWordDiff()
		assert.False(t, m.wordDiff, "should be disabled after second toggle")
		assert.Nil(t, m.intraRanges, "ranges cleared after disabling")
	})

	t.Run("toggleWordDiff is no-op when no file loaded", func(t *testing.T) {
		m := testModel(nil, nil)
		m.focus = paneDiff
		m.toggleWordDiff()
		assert.False(t, m.wordDiff, "should stay off with no file")
	})

	t.Run("W key flips wordDiff through Update dispatch", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.styles = newStyles(colors)
		m.ready = true
		m.width = 200
		m.height = 30
		m.viewport.Width = 196
		m.viewport.Height = 28
		result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
		m = result.(Model)
		m.focus = paneDiff

		require.False(t, m.wordDiff, "initial state: off")
		require.Nil(t, m.intraRanges, "initial state: no ranges")

		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}})
		m = result.(Model)
		assert.True(t, m.wordDiff, "W key should enable wordDiff")
		assert.NotNil(t, m.intraRanges, "ranges should be computed after W")

		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}})
		m = result.(Model)
		assert.False(t, m.wordDiff, "second W should disable wordDiff")
		assert.Nil(t, m.intraRanges, "ranges should be cleared after second W")
	})
}

func TestModel_UpdateRestoreBg(t *testing.T) {
	m := testModel(nil, nil)
	hlOff := "\033[48;2;0;34;0m" // line bg (AddBg)

	tests := []struct {
		name, seq, current, want string
	}{
		{name: "24-bit bg sets restore", seq: "\033[48;2;45;90;58m", current: hlOff, want: "\033[48;2;45;90;58m"},
		{name: "basic bg sets restore", seq: "\033[42m", current: hlOff, want: "\033[42m"},
		{name: "reverse on sets restore", seq: "\033[7m", current: hlOff, want: "\033[7m"},
		{name: "bg reset returns hlOff", seq: "\033[49m", current: "\033[48;2;45;90;58m", want: hlOff},
		{name: "reverse off returns hlOff", seq: "\033[27m", current: "\033[7m", want: hlOff},
		{name: "full reset returns hlOff", seq: "\033[0m", current: "\033[48;2;45;90;58m", want: hlOff},
		{name: "bare reset returns hlOff", seq: "\033[m", current: "\033[7m", want: hlOff},
		{name: "fg color unchanged", seq: "\033[38;2;255;0;0m", current: "\033[48;2;45;90;58m", want: "\033[48;2;45;90;58m"},
		{name: "bold unchanged", seq: "\033[1m", current: hlOff, want: hlOff},
		{name: "too short unchanged", seq: "\033[", current: hlOff, want: hlOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.updateRestoreBg(tt.seq, tt.current, hlOff)
			assert.Equal(t, tt.want, got)
		})
	}
}
