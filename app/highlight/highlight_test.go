package highlight

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestHighlighter_HighlightLines(t *testing.T) {
	h := New("monokai", true)

	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "", ChangeType: diff.ChangeContext},
		{OldNum: 3, NewNum: 3, Content: `import "fmt"`, ChangeType: diff.ChangeContext},
		{OldNum: 4, NewNum: 4, Content: "", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "func main() {", ChangeType: diff.ChangeAdd},
		{NewNum: 6, Content: `	fmt.Println("hello")`, ChangeType: diff.ChangeAdd},
		{NewNum: 7, Content: "}", ChangeType: diff.ChangeAdd},
	}

	result := h.HighlightLines("main.go", lines)
	require.NotNil(t, result)
	assert.Len(t, result, len(lines))

	// context lines should have ANSI codes
	assert.Contains(t, result[0], "\033[", "context line should contain ANSI escape")
	assert.Contains(t, result[0], "package")

	// added lines should have ANSI codes
	assert.Contains(t, result[4], "\033[", "added line should contain ANSI escape")
	assert.Contains(t, result[4], "func")
}

func TestHighlighter_HighlightLinesWithRemoved(t *testing.T) {
	h := New("monokai", true)

	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: `var x = "old"`, ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: `var x = "new"`, ChangeType: diff.ChangeAdd},
		{OldNum: 3, NewNum: 3, Content: "", ChangeType: diff.ChangeContext},
	}

	result := h.HighlightLines("main.go", lines)
	require.NotNil(t, result)
	assert.Len(t, result, len(lines))

	// removed line should have ANSI codes
	assert.Contains(t, result[1], "\033[", "removed line should contain ANSI escape")
	assert.Contains(t, result[1], "old")

	// added line should have ANSI codes
	assert.Contains(t, result[2], "\033[", "added line should contain ANSI escape")
	assert.Contains(t, result[2], "new")
}

func TestHighlighter_HighlightLinesWithDivider(t *testing.T) {
	h := New("monokai", true)

	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{Content: "~~~", ChangeType: diff.ChangeDivider},
		{OldNum: 10, NewNum: 10, Content: `func foo() {}`, ChangeType: diff.ChangeContext},
	}

	result := h.HighlightLines("main.go", lines)
	require.NotNil(t, result)
	assert.Len(t, result, len(lines))
	assert.Empty(t, result[1], "divider should have empty highlighted content")
}

func TestHighlighter_DisabledReturnsNil(t *testing.T) {
	h := New("monokai", false)
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
	}
	assert.Nil(t, h.HighlightLines("main.go", lines))
}

func TestHighlighter_UnknownFileReturnsNil(t *testing.T) {
	h := New("monokai", true)
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "some data", ChangeType: diff.ChangeContext},
	}
	assert.Nil(t, h.HighlightLines("data.unknownext12345", lines))
}

func TestHighlighter_EmptyLinesReturnsNil(t *testing.T) {
	h := New("monokai", true)
	assert.Nil(t, h.HighlightLines("main.go", nil))
	assert.Nil(t, h.HighlightLines("main.go", []diff.DiffLine{}))
}

func TestHighlighter_DefaultStyle(t *testing.T) {
	h := New("", true)
	assert.Equal(t, "monokai", h.styleName)
}

func TestHighlighter_EnabledState(t *testing.T) {
	h := New("monokai", false)
	assert.False(t, h.enabled)
	h2 := New("monokai", true)
	assert.True(t, h2.enabled)
}

// verifies that syntax highlighting works correctly on files where all lines
// are ChangeContext (no-git file review mode).
func TestHighlighter_ContextOnlyFile(t *testing.T) {
	h := New("monokai", true)

	// simulate a Go file viewed in context-only mode (all lines are ChangeContext)
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "", ChangeType: diff.ChangeContext},
		{OldNum: 3, NewNum: 3, Content: `import "fmt"`, ChangeType: diff.ChangeContext},
		{OldNum: 4, NewNum: 4, Content: "", ChangeType: diff.ChangeContext},
		{OldNum: 5, NewNum: 5, Content: "func main() {", ChangeType: diff.ChangeContext},
		{OldNum: 6, NewNum: 6, Content: `	fmt.Println("hello")`, ChangeType: diff.ChangeContext},
		{OldNum: 7, NewNum: 7, Content: "}", ChangeType: diff.ChangeContext},
	}

	result := h.HighlightLines("main.go", lines)
	require.NotNil(t, result, "should produce highlighted output for context-only file")
	assert.Len(t, result, len(lines))

	// all non-empty lines should contain ANSI escape codes for syntax highlighting
	for i, hl := range result {
		if lines[i].Content != "" {
			assert.Contains(t, hl, "\033[", "line %d (%q) should have ANSI highlighting", i+1, lines[i].Content)
		}
	}

	// verify keywords are highlighted
	assert.Contains(t, result[0], "package", "should preserve keyword text")
	assert.Contains(t, result[4], "func", "should preserve keyword text")
}

func TestReconstructFiles(t *testing.T) {
	lines := []diff.DiffLine{
		{Content: "line1", ChangeType: diff.ChangeContext},
		{Content: "removed", ChangeType: diff.ChangeRemove},
		{Content: "added", ChangeType: diff.ChangeAdd},
		{Content: "~~~", ChangeType: diff.ChangeDivider},
		{Content: "line2", ChangeType: diff.ChangeContext},
	}

	newFile, oldFile := reconstructFiles(lines)

	assert.Equal(t, "line1\nadded\nline2\n", newFile)
	assert.Equal(t, "line1\nremoved\nline2\n", oldFile)
}

func TestWriteTokenANSI_PlainText(t *testing.T) {
	var b strings.Builder
	style := styles.Get("monokai")
	writeTokenANSI(&b, chroma.None, "hello", style)
	assert.Contains(t, b.String(), "hello")
}

func TestWriteTokenANSI_WithAttributes(t *testing.T) {
	style := styles.Get("monokai")

	t.Run("foreground color", func(t *testing.T) {
		var b strings.Builder
		// keyword tokens typically have foreground color in monokai
		writeTokenANSI(&b, chroma.Keyword, "func", style)
		result := b.String()
		assert.Contains(t, result, "\033[38;2;", "should contain RGB foreground")
		assert.Contains(t, result, "func")
		assert.Contains(t, result, "\033[39m", "should reset foreground")
	})

	t.Run("bold attribute", func(t *testing.T) {
		var b strings.Builder
		// use NameBuiltin which is typically bold in monokai
		writeTokenANSI(&b, chroma.GenericStrong, "strong", style)
		result := b.String()
		assert.Contains(t, result, "strong")
		// GenericStrong should be bold
		if strings.Contains(result, "\033[1m") {
			assert.Contains(t, result, "\033[22m", "bold should be reset")
		}
	})
}

func TestSetStyle(t *testing.T) {
	h := New("monokai", true)
	assert.Equal(t, "monokai", h.StyleName())

	ok := h.SetStyle("dracula")
	assert.True(t, ok)
	assert.Equal(t, "dracula", h.StyleName())
}

func TestSetStyle_unknownStyle(t *testing.T) {
	h := New("monokai", true)
	ok := h.SetStyle("nonexistent-style-xyz")
	assert.False(t, ok)
	assert.Equal(t, "monokai", h.StyleName(), "style should not change on failure")
}
