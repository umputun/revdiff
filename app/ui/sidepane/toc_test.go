package sidepane

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

func TestParseTOC(t *testing.T) {
	tests := []struct {
		name    string
		lines   []diff.DiffLine
		wantNil bool
		want    []tocEntry
	}{
		{name: "empty input", lines: nil, wantNil: true},
		{name: "no headers", lines: []diff.DiffLine{
			{Content: "some text", ChangeType: diff.ChangeContext},
			{Content: "more text", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "single header", lines: []diff.DiffLine{
			{Content: "# Title", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{{title: "Title", level: 1, lineIdx: 0}}},
		{name: "nested headers h1-h6", lines: []diff.DiffLine{
			{Content: "# H1", ChangeType: diff.ChangeContext},
			{Content: "## H2", ChangeType: diff.ChangeContext},
			{Content: "### H3", ChangeType: diff.ChangeContext},
			{Content: "#### H4", ChangeType: diff.ChangeContext},
			{Content: "##### H5", ChangeType: diff.ChangeContext},
			{Content: "###### H6", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "H1", level: 1, lineIdx: 0},
			{title: "H2", level: 2, lineIdx: 1},
			{title: "H3", level: 3, lineIdx: 2},
			{title: "H4", level: 4, lineIdx: 3},
			{title: "H5", level: 5, lineIdx: 4},
			{title: "H6", level: 6, lineIdx: 5},
		}},
		{name: "headers mixed with non-header lines", lines: []diff.DiffLine{
			{Content: "intro text", ChangeType: diff.ChangeContext},
			{Content: "# First", ChangeType: diff.ChangeContext},
			{Content: "body text", ChangeType: diff.ChangeContext},
			{Content: "## Second", ChangeType: diff.ChangeContext},
			{Content: "more body", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "First", level: 1, lineIdx: 1},
			{title: "Second", level: 2, lineIdx: 3},
		}},
		{name: "headers inside fenced code block excluded", lines: []diff.DiffLine{
			{Content: "# Real Header", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "# Not A Header", ChangeType: diff.ChangeContext},
			{Content: "## Also Not", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "## After Code Block", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Real Header", level: 1, lineIdx: 0},
			{title: "After Code Block", level: 2, lineIdx: 5},
		}},
		{name: "fenced code block with language", lines: []diff.DiffLine{
			{Content: "# Header", ChangeType: diff.ChangeContext},
			{Content: "```go", ChangeType: diff.ChangeContext},
			{Content: "# comment in code", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{{title: "Header", level: 1, lineIdx: 0}}},
		{name: "tilde fenced code block excluded", lines: []diff.DiffLine{
			{Content: "# Before", ChangeType: diff.ChangeContext},
			{Content: "~~~", ChangeType: diff.ChangeContext},
			{Content: "# Inside Tilde Fence", ChangeType: diff.ChangeContext},
			{Content: "~~~", ChangeType: diff.ChangeContext},
			{Content: "# After", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Before", level: 1, lineIdx: 0},
			{title: "After", level: 1, lineIdx: 4},
		}},
		{name: "mixed fences do not cross-close", lines: []diff.DiffLine{
			{Content: "# Before", ChangeType: diff.ChangeContext},
			{Content: "~~~", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "# Leaked Header", ChangeType: diff.ChangeContext},
			{Content: "~~~", ChangeType: diff.ChangeContext},
			{Content: "# After", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Before", level: 1, lineIdx: 0},
			{title: "After", level: 1, lineIdx: 5},
		}},
		{name: "backtick fence ignores tilde inside", lines: []diff.DiffLine{
			{Content: "# Before", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "~~~", ChangeType: diff.ChangeContext},
			{Content: "# Inside Backtick", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "# After", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Before", level: 1, lineIdx: 0},
			{title: "After", level: 1, lineIdx: 5},
		}},
		{name: "4-backtick fence not closed by 3 backticks", lines: []diff.DiffLine{
			{Content: "# Before", ChangeType: diff.ChangeContext},
			{Content: "````", ChangeType: diff.ChangeContext},
			{Content: "```go", ChangeType: diff.ChangeContext},
			{Content: "# Inside Nested", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "# Still Inside", ChangeType: diff.ChangeContext},
			{Content: "````", ChangeType: diff.ChangeContext},
			{Content: "# After", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Before", level: 1, lineIdx: 0},
			{title: "After", level: 1, lineIdx: 7},
		}},
		{name: "closing fence with trailing text does not close", lines: []diff.DiffLine{
			{Content: "# Before", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "```not a close", ChangeType: diff.ChangeContext},
			{Content: "# Still Inside", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "# After", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Before", level: 1, lineIdx: 0},
			{title: "After", level: 1, lineIdx: 5},
		}},
		{name: "5-tilde fence requires 5+ tildes to close", lines: []diff.DiffLine{
			{Content: "# Before", ChangeType: diff.ChangeContext},
			{Content: "~~~~~", ChangeType: diff.ChangeContext},
			{Content: "~~~", ChangeType: diff.ChangeContext},
			{Content: "# Leaked?", ChangeType: diff.ChangeContext},
			{Content: "~~~~~", ChangeType: diff.ChangeContext},
			{Content: "# After", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Before", level: 1, lineIdx: 0},
			{title: "After", level: 1, lineIdx: 5},
		}},
		{name: "no space after hash is not a header", lines: []diff.DiffLine{
			{Content: "#nospace", ChangeType: diff.ChangeContext},
			{Content: "##also-no", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "hash only without title", lines: []diff.DiffLine{
			{Content: "# ", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "more than 6 hashes", lines: []diff.DiffLine{
			{Content: "####### Too Deep", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "divider lines skipped", lines: []diff.DiffLine{
			{Content: "", ChangeType: diff.ChangeDivider},
			{Content: "# Title", ChangeType: diff.ChangeContext},
			{Content: "", ChangeType: diff.ChangeDivider},
		}, want: []tocEntry{{title: "Title", level: 1, lineIdx: 1}}},
	}

	topEntry := tocEntry{title: "test.md", level: 1, lineIdx: 0}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTOC(tt.lines, "test.md")
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			// first entry is always the synthetic "top" entry
			wantWithTop := append([]tocEntry{topEntry}, tt.want...)
			assert.Equal(t, wantWithTop, got.entries)
			assert.Equal(t, 0, got.cursor)
			assert.Equal(t, 0, got.offset)
			assert.Equal(t, -1, got.activeSection)
		})
	}
}

func TestParseTOC_FilenamePathStripping(t *testing.T) {
	lines := []diff.DiffLine{{Content: "# Title", ChangeType: diff.ChangeContext}}
	got := ParseTOC(lines, "docs/plans/guide.md")
	require.NotNil(t, got)
	assert.Equal(t, "guide.md", got.entries[0].title, "top entry should use base filename, not full path")
	assert.Equal(t, "Title", got.entries[1].title)
}

func TestParseTOC_NoHeaders(t *testing.T) {
	// returns nil (not a typed-nil wrapped in a valid *TOC), so main.go factory guard works
	got := ParseTOC([]diff.DiffLine{
		{Content: "just text", ChangeType: diff.ChangeContext},
		{Content: "more text", ChangeType: diff.ChangeContext},
	}, "readme.md")
	assert.Nil(t, got, "ParseTOC with no headers must return nil")

	// also verify nil for empty input
	got = ParseTOC(nil, "empty.md")
	assert.Nil(t, got, "ParseTOC with nil lines must return nil")
}

func TestParseTOC_InitialActiveSection(t *testing.T) {
	// newly returned *TOC must have activeSection == -1
	lines := []diff.DiffLine{
		{Content: "# Header One", ChangeType: diff.ChangeContext},
		{Content: "## Header Two", ChangeType: diff.ChangeContext},
	}
	got := ParseTOC(lines, "test.md")
	require.NotNil(t, got)
	assert.Equal(t, -1, got.activeSection, "initial activeSection must be -1 sentinel")
}

func TestTOC_MoveUpDown(t *testing.T) {
	toc := &TOC{entries: []tocEntry{
		{title: "A", level: 1, lineIdx: 0},
		{title: "B", level: 2, lineIdx: 5},
		{title: "C", level: 2, lineIdx: 10},
	}, cursor: 0, activeSection: -1}

	t.Run("move down from first", func(t *testing.T) {
		toc.cursor = 0
		toc.Move(MotionDown)
		assert.Equal(t, 1, toc.cursor)
	})

	t.Run("move down to last", func(t *testing.T) {
		toc.cursor = 1
		toc.Move(MotionDown)
		assert.Equal(t, 2, toc.cursor)
	})

	t.Run("move down clamped at last", func(t *testing.T) {
		toc.cursor = 2
		toc.Move(MotionDown)
		assert.Equal(t, 2, toc.cursor)
	})

	t.Run("move up from last", func(t *testing.T) {
		toc.cursor = 2
		toc.Move(MotionUp)
		assert.Equal(t, 1, toc.cursor)
	})

	t.Run("move up clamped at first", func(t *testing.T) {
		toc.cursor = 0
		toc.Move(MotionUp)
		assert.Equal(t, 0, toc.cursor)
	})

	t.Run("single entry no movement", func(t *testing.T) {
		single := &TOC{entries: []tocEntry{{title: "Only", level: 1, lineIdx: 0}}, cursor: 0}
		single.Move(MotionUp)
		assert.Equal(t, 0, single.cursor)
		single.Move(MotionDown)
		assert.Equal(t, 0, single.cursor)
	})
}

func TestTOC_EnsureVisible(t *testing.T) {
	tests := []struct {
		name       string
		entries    int
		cursor     int
		offset     int
		height     int
		wantOffset int
	}{
		{name: "cursor already visible", entries: 10, cursor: 3, offset: 0, height: 5, wantOffset: 0},
		{name: "cursor above viewport", entries: 10, cursor: 1, offset: 3, height: 5, wantOffset: 1},
		{name: "cursor below viewport", entries: 10, cursor: 8, offset: 0, height: 5, wantOffset: 4},
		{name: "cursor at last with small height", entries: 10, cursor: 9, offset: 0, height: 3, wantOffset: 7},
		{name: "zero height", entries: 10, cursor: 5, offset: 0, height: 0, wantOffset: 0},
		{name: "height larger than entries", entries: 3, cursor: 2, offset: 0, height: 10, wantOffset: 0},
		{name: "offset clamped to max", entries: 5, cursor: 2, offset: 10, height: 3, wantOffset: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := make([]tocEntry, tt.entries)
			for i := range entries {
				entries[i] = tocEntry{title: "H", level: 1, lineIdx: i * 10}
			}
			toc := &TOC{entries: entries, cursor: tt.cursor, offset: tt.offset}
			toc.EnsureVisible(tt.height)
			assert.Equal(t, tt.wantOffset, toc.offset)
		})
	}
}

func TestTOC_UpdateActiveSection(t *testing.T) {
	toc := &TOC{entries: []tocEntry{
		{title: "Intro", level: 1, lineIdx: 0},
		{title: "Setup", level: 2, lineIdx: 10},
		{title: "Usage", level: 2, lineIdx: 25},
		{title: "API", level: 2, lineIdx: 50},
	}, activeSection: -1}

	tests := []struct {
		name       string
		diffCursor int
		wantActive int
	}{
		{name: "before first header", diffCursor: -1, wantActive: -1},
		{name: "at first header", diffCursor: 0, wantActive: 0},
		{name: "between first and second", diffCursor: 5, wantActive: 0},
		{name: "at second header", diffCursor: 10, wantActive: 1},
		{name: "between second and third", diffCursor: 20, wantActive: 1},
		{name: "at last header", diffCursor: 50, wantActive: 3},
		{name: "after last header", diffCursor: 100, wantActive: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toc.UpdateActiveSection(tt.diffCursor)
			assert.Equal(t, tt.wantActive, toc.activeSection)
		})
	}

	t.Run("empty entries", func(t *testing.T) {
		empty := &TOC{entries: nil, activeSection: 5}
		empty.UpdateActiveSection(10)
		assert.Equal(t, -1, empty.activeSection)
	})
}

func TestTOC_Render(t *testing.T) {
	res := style.PlainResolver()

	t.Run("empty TOC", func(t *testing.T) {
		toc := &TOC{entries: nil}
		got := toc.Render(TOCRender{Width: 40, Height: 10, Focused: true, Resolver: res})
		assert.Equal(t, "  no headers", got)
	})

	t.Run("single h1 entry, tree focused", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{{title: "Title", level: 1, lineIdx: 0}}, cursor: 0, activeSection: -1}
		got := toc.Render(TOCRender{Width: 40, Height: 10, Focused: true, Resolver: res})
		assert.Contains(t, got, "Title")
		// cursor should be highlighted with FileSelected style (reverse in plain)
		assert.NotEqual(t, "  Title", got) // styled, not plain
	})

	t.Run("indentation by level", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "H1", level: 1, lineIdx: 0},
			{title: "H2", level: 2, lineIdx: 5},
			{title: "H3", level: 3, lineIdx: 10},
		}, cursor: 0, activeSection: -1}
		got := toc.Render(TOCRender{Width: 40, Height: 10, Focused: false, Resolver: res})
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 3)
		// h1 has no indent (level-1=0), h2 has 2 spaces, h3 has 4 spaces
		assert.True(t, strings.HasPrefix(lines[0], "  H1"), "h1 should have prefix spaces only: %q", lines[0])
		assert.Contains(t, lines[1], "  H2", "h2 should have 2 extra spaces indent: %q", lines[1])
		assert.Contains(t, lines[2], "    H3", "h3 should have 4 extra spaces indent: %q", lines[2])
	})

	t.Run("active section highlighted in diff focus", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "First", level: 1, lineIdx: 0},
			{title: "Second", level: 1, lineIdx: 10},
		}, cursor: 0, activeSection: 1}
		got := toc.Render(TOCRender{Width: 40, Height: 10, Focused: false, Resolver: res})
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 2)
		// active section (Second) should be highlighted, first should not
		assert.Contains(t, lines[0], "First")
		assert.Contains(t, lines[1], "Second")
		// both lines contain their text, active section uses FileSelected (same as cursor)
		assert.Greater(t, len(lines[1]), len(lines[0]), "highlighted line should have style sequences")
	})

	t.Run("cursor highlighted in tree focus", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "First", level: 1, lineIdx: 0},
			{title: "Second", level: 1, lineIdx: 10},
		}, cursor: 1, activeSection: 0}
		got := toc.Render(TOCRender{Width: 40, Height: 10, Focused: true, Resolver: res})
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 2)
		// cursor (Second) should be highlighted, not active section
		assert.Greater(t, len(lines[1]), len(lines[0]), "cursor line should have style sequences")
	})

	t.Run("truncation of long title", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "This is a very long title that should be truncated", level: 1, lineIdx: 0},
		}, cursor: 0, activeSection: -1}
		got := toc.Render(TOCRender{Width: 20, Height: 10, Focused: false, Resolver: res})
		assert.Contains(t, got, "…")
	})

	t.Run("scrolling offset", func(t *testing.T) {
		entries := make([]tocEntry, 20)
		for i := range entries {
			entries[i] = tocEntry{title: fmt.Sprintf("Header %d", i), level: 1, lineIdx: i * 10}
		}
		toc := &TOC{entries: entries, cursor: 15, offset: 0, activeSection: -1}
		got := toc.Render(TOCRender{Width: 40, Height: 5, Focused: true, Resolver: res})
		lines := strings.Split(got, "\n")
		assert.Len(t, lines, 5)
		assert.Contains(t, got, "Header 15")   // cursor entry should be visible
		assert.NotContains(t, got, "Header 0") // first entry should not be visible
	})

	t.Run("tree focus highlights cursor, diff focus highlights active section", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 10},
			{title: "C", level: 1, lineIdx: 20},
		}, cursor: 1, activeSection: 2}
		// in tree focus, cursor (B at idx 1) is highlighted
		gotTree := toc.Render(TOCRender{Width: 40, Height: 10, Focused: true, Resolver: res})
		treeLines := strings.Split(gotTree, "\n")
		require.Len(t, treeLines, 3)
		assert.Greater(t, len(treeLines[1]), len(treeLines[0]), "cursor B should be highlighted in tree focus")
		assert.Less(t, len(treeLines[2]), len(treeLines[1]), "C should not be highlighted in tree focus")

		// in diff focus, active section (C at idx 2) is highlighted
		gotDiff := toc.Render(TOCRender{Width: 40, Height: 10, Focused: false, Resolver: res})
		diffLines := strings.Split(gotDiff, "\n")
		require.Len(t, diffLines, 3)
		assert.Greater(t, len(diffLines[2]), len(diffLines[0]), "active section C should be highlighted in diff focus")
		assert.Less(t, len(diffLines[1]), len(diffLines[2]), "B should not be highlighted in diff focus")
	})
}

func TestTOC_Render_ActiveSectionViewportVisibility(t *testing.T) {
	// when diff pane is focused and activeSection is far from cursor,
	// the TOC viewport should scroll to keep activeSection visible
	res := style.PlainResolver()
	entries := make([]tocEntry, 30)
	for i := range entries {
		entries[i] = tocEntry{title: fmt.Sprintf("Header %d", i), level: 1, lineIdx: i * 10}
	}
	toc := &TOC{entries: entries, cursor: 0, offset: 0, activeSection: 25}
	got := toc.Render(TOCRender{Width: 40, Height: 5, Focused: false, Resolver: res})
	assert.Contains(t, got, "Header 25", "active section entry should be visible in viewport")
	assert.NotContains(t, got, "Header 0", "cursor entry should scroll out of viewport")
}

func TestTOC_TruncateTitle(t *testing.T) {
	toc := &TOC{}
	tests := []struct {
		name     string
		title    string
		maxWidth int
		want     string
	}{
		{name: "fits", title: "Hello", maxWidth: 10, want: "Hello"},
		{name: "exact fit", title: "Hello", maxWidth: 5, want: "Hello"},
		{name: "truncated", title: "Hello World", maxWidth: 8, want: "Hello W…"},
		{name: "very narrow", title: "Hello", maxWidth: 1, want: "…"},
		{name: "zero width", title: "Hello", maxWidth: 0, want: ""},
		{name: "negative width", title: "Hello", maxWidth: -1, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toc.truncateTitle(tt.title, tt.maxWidth))
		})
	}
}

func TestTOCMove_Exhaustive(t *testing.T) {
	// iterating all MotionValues, asserting no panic with or without count argument
	toc := &TOC{entries: []tocEntry{
		{title: "A", level: 1, lineIdx: 0},
		{title: "B", level: 2, lineIdx: 5},
		{title: "C", level: 2, lineIdx: 10},
		{title: "D", level: 3, lineIdx: 15},
		{title: "E", level: 3, lineIdx: 20},
	}, cursor: 2, activeSection: -1}

	for _, m := range MotionValues {
		t.Run(m.String()+"_no_count", func(t *testing.T) {
			assert.NotPanics(t, func() { toc.Move(m) })
		})
		t.Run(m.String()+"_with_count", func(t *testing.T) {
			assert.NotPanics(t, func() { toc.Move(m, 3) })
		})
	}
}

func TestTOCMove_PageUpDown(t *testing.T) {
	toc := &TOC{entries: []tocEntry{
		{title: "A", level: 1, lineIdx: 0},
		{title: "B", level: 1, lineIdx: 5},
		{title: "C", level: 1, lineIdx: 10},
		{title: "D", level: 1, lineIdx: 15},
		{title: "E", level: 1, lineIdx: 20},
		{title: "F", level: 1, lineIdx: 25},
		{title: "G", level: 1, lineIdx: 30},
		{title: "H", level: 1, lineIdx: 35},
		{title: "I", level: 1, lineIdx: 40},
		{title: "J", level: 1, lineIdx: 45},
	}, cursor: 0, activeSection: -1}

	t.Run("page down by 3", func(t *testing.T) {
		toc.cursor = 0
		toc.Move(MotionPageDown, 3)
		assert.Equal(t, 3, toc.cursor)
	})

	t.Run("page down clamped at end", func(t *testing.T) {
		toc.cursor = 8
		toc.Move(MotionPageDown, 5)
		assert.Equal(t, 9, toc.cursor)
	})

	t.Run("page up by 3", func(t *testing.T) {
		toc.cursor = 5
		toc.Move(MotionPageUp, 3)
		assert.Equal(t, 2, toc.cursor)
	})

	t.Run("page up clamped at start", func(t *testing.T) {
		toc.cursor = 1
		toc.Move(MotionPageUp, 5)
		assert.Equal(t, 0, toc.cursor)
	})

	t.Run("page down no count defaults to 1", func(t *testing.T) {
		toc.cursor = 3
		toc.Move(MotionPageDown)
		assert.Equal(t, 4, toc.cursor)
	})

	t.Run("page up no count defaults to 1", func(t *testing.T) {
		toc.cursor = 3
		toc.Move(MotionPageUp)
		assert.Equal(t, 2, toc.cursor)
	})
}

func TestTOCMove_FirstLast(t *testing.T) {
	toc := &TOC{entries: []tocEntry{
		{title: "A", level: 1, lineIdx: 0},
		{title: "B", level: 1, lineIdx: 5},
		{title: "C", level: 1, lineIdx: 10},
	}, cursor: 1, activeSection: -1}

	toc.Move(MotionFirst)
	assert.Equal(t, 0, toc.cursor)

	toc.Move(MotionLast)
	assert.Equal(t, 2, toc.cursor)
}

func TestTOCCurrentLineIdx(t *testing.T) {
	t.Run("valid cursor returns idx and true", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 2, lineIdx: 15},
			{title: "C", level: 2, lineIdx: 30},
		}, cursor: 1}
		idx, ok := toc.CurrentLineIdx()
		assert.True(t, ok)
		assert.Equal(t, 15, idx)
	})

	t.Run("empty entries returns false", func(t *testing.T) {
		toc := &TOC{entries: nil, cursor: 0}
		idx, ok := toc.CurrentLineIdx()
		assert.False(t, ok)
		assert.Equal(t, 0, idx)
	})

	t.Run("cursor past end returns false", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
		}, cursor: 5}
		idx, ok := toc.CurrentLineIdx()
		assert.False(t, ok)
		assert.Equal(t, 0, idx)
	})

	t.Run("negative cursor returns false", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
		}, cursor: -1}
		idx, ok := toc.CurrentLineIdx()
		assert.False(t, ok)
		assert.Equal(t, 0, idx)
	})

	t.Run("first entry", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 42},
		}, cursor: 0}
		idx, ok := toc.CurrentLineIdx()
		assert.True(t, ok)
		assert.Equal(t, 42, idx)
	})
}

func TestTOCSyncCursorToActiveSection(t *testing.T) {
	t.Run("activeSection -1 is no-op", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 10},
		}, cursor: 0, activeSection: -1}
		toc.SyncCursorToActiveSection()
		assert.Equal(t, 0, toc.cursor, "cursor should not change when activeSection is -1")
	})

	t.Run("activeSection >= 0 moves cursor to match", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 10},
			{title: "C", level: 1, lineIdx: 20},
		}, cursor: 0, activeSection: 2}
		toc.SyncCursorToActiveSection()
		assert.Equal(t, 2, toc.cursor, "cursor should move to activeSection")
	})

	t.Run("activeSection 0 moves cursor", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 10},
		}, cursor: 1, activeSection: 0}
		toc.SyncCursorToActiveSection()
		assert.Equal(t, 0, toc.cursor, "cursor should move to activeSection 0")
	})
}

func TestTOC_NumEntries(t *testing.T) {
	t.Run("with entries", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 5},
		}}
		assert.Equal(t, 2, toc.NumEntries())
	})

	t.Run("empty", func(t *testing.T) {
		toc := &TOC{}
		assert.Equal(t, 0, toc.NumEntries())
	})
}

func TestTOC_SelectByVisibleRow(t *testing.T) {
	t.Run("first row at offset zero selects first entry", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 5},
		}, cursor: 1}
		ok := toc.SelectByVisibleRow(0)
		assert.True(t, ok)
		assert.Equal(t, 0, toc.cursor)
	})

	t.Run("row within visible range selects matching entry", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 5},
			{title: "C", level: 1, lineIdx: 10},
		}, cursor: 0}
		ok := toc.SelectByVisibleRow(2)
		assert.True(t, ok)
		assert.Equal(t, 2, toc.cursor)
	})

	t.Run("row with non-zero offset adds offset", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 5},
			{title: "C", level: 1, lineIdx: 10},
			{title: "D", level: 1, lineIdx: 15},
			{title: "E", level: 1, lineIdx: 20},
		}, cursor: 0, offset: 2}
		ok := toc.SelectByVisibleRow(1)
		assert.True(t, ok)
		assert.Equal(t, 3, toc.cursor) // offset(2) + row(1)
	})

	t.Run("click past end returns false and does not modify cursor", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 5},
		}, cursor: 1}
		ok := toc.SelectByVisibleRow(10)
		assert.False(t, ok)
		assert.Equal(t, 1, toc.cursor)
	})

	t.Run("negative row returns false and does not modify cursor", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
		}, cursor: 0}
		ok := toc.SelectByVisibleRow(-1)
		assert.False(t, ok)
		assert.Equal(t, 0, toc.cursor)
	})

	t.Run("empty TOC returns false for any row", func(t *testing.T) {
		toc := &TOC{}
		ok := toc.SelectByVisibleRow(0)
		assert.False(t, ok)
	})

	t.Run("row past end with offset returns false", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 5},
			{title: "C", level: 1, lineIdx: 10},
		}, cursor: 0, offset: 1}
		ok := toc.SelectByVisibleRow(5)
		assert.False(t, ok)
		assert.Equal(t, 0, toc.cursor)
	})
}

func TestTOC_ScrollState(t *testing.T) {
	t.Run("empty TOC reports zero total and zero offset", func(t *testing.T) {
		toc := &TOC{}
		s := toc.ScrollState()
		assert.Equal(t, 0, s.Total)
		assert.Equal(t, 0, s.Offset)
	})

	t.Run("fresh TOC starts at offset zero", func(t *testing.T) {
		toc := &TOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 10},
			{title: "C", level: 1, lineIdx: 20},
		}}
		s := toc.ScrollState()
		assert.Equal(t, 3, s.Total)
		assert.Equal(t, 0, s.Offset)
	})

	t.Run("offset reflects post-EnsureVisible state when cursor moves", func(t *testing.T) {
		entries := make([]tocEntry, 30)
		for i := range entries {
			entries[i] = tocEntry{title: fmt.Sprintf("S%02d", i), level: 1, lineIdx: i * 10}
		}
		toc := &TOC{entries: entries, cursor: 0}
		toc.Move(MotionLast)
		assert.Equal(t, 0, toc.ScrollState().Offset, "offset is stale before EnsureVisible")

		toc.EnsureVisible(10)
		s := toc.ScrollState()
		assert.Equal(t, 30, s.Total)
		assert.Positive(t, s.Offset, "EnsureVisible scrolls to keep cursor visible")
		assert.LessOrEqual(t, s.Offset, 30-10)
	})

	t.Run("offset reflects active-section visibility when diff has focus", func(t *testing.T) {
		// when not focused, Render aligns offset to activeSection rather than cursor.
		// ScrollState read AFTER Render must report the activeSection-driven offset.
		entries := make([]tocEntry, 30)
		for i := range entries {
			entries[i] = tocEntry{title: fmt.Sprintf("S%02d", i), level: 1, lineIdx: i * 10}
		}
		toc := &TOC{entries: entries, cursor: 0}
		toc.UpdateActiveSection(280) // resolves to last entry (lineIdx 290 > 280, so previous index 28)
		require.GreaterOrEqual(t, toc.activeSection, 20, "active section should land near the end")

		res := style.NewResolver(style.Colors{Normal: "#d0d0d0", Muted: "#6c6c6c"})
		_ = toc.Render(TOCRender{Width: 30, Height: 10, Focused: false, Resolver: res})

		s := toc.ScrollState()
		assert.Positive(t, s.Offset, "Render must scroll the viewport to keep activeSection visible when diff has focus")
		assert.Equal(t, 0, toc.cursor, "cursor must be restored after the focus-driven render")
	})
}

func TestFencePrefix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantCh  rune
		wantLen int
	}{
		{name: "empty", input: "", wantCh: 0, wantLen: 0},
		{name: "no fence", input: "hello", wantCh: 0, wantLen: 0},
		{name: "3 backticks", input: "```", wantCh: '`', wantLen: 3},
		{name: "3 tildes", input: "~~~", wantCh: '~', wantLen: 3},
		{name: "4 backticks with lang", input: "````go", wantCh: '`', wantLen: 4},
		{name: "5 tildes", input: "~~~~~", wantCh: '~', wantLen: 5},
		{name: "single backtick", input: "`", wantCh: '`', wantLen: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, n := fencePrefix(tt.input)
			assert.Equal(t, tt.wantCh, ch)
			assert.Equal(t, tt.wantLen, n)
		})
	}
}
