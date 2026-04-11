package ui

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
			got := parseTOC(tt.lines, "test.md")
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
	got := parseTOC(lines, "docs/plans/guide.md")
	require.NotNil(t, got)
	assert.Equal(t, "guide.md", got.entries[0].title, "top entry should use base filename, not full path")
	assert.Equal(t, "Title", got.entries[1].title)
}

func TestMdTOC_MoveUpDown(t *testing.T) {
	toc := &mdTOC{entries: []tocEntry{
		{title: "A", level: 1, lineIdx: 0},
		{title: "B", level: 2, lineIdx: 5},
		{title: "C", level: 2, lineIdx: 10},
	}, cursor: 0, activeSection: -1}

	t.Run("move down from first", func(t *testing.T) {
		toc.cursor = 0
		toc.moveDown()
		assert.Equal(t, 1, toc.cursor)
	})

	t.Run("move down to last", func(t *testing.T) {
		toc.cursor = 1
		toc.moveDown()
		assert.Equal(t, 2, toc.cursor)
	})

	t.Run("move down clamped at last", func(t *testing.T) {
		toc.cursor = 2
		toc.moveDown()
		assert.Equal(t, 2, toc.cursor)
	})

	t.Run("move up from last", func(t *testing.T) {
		toc.cursor = 2
		toc.moveUp()
		assert.Equal(t, 1, toc.cursor)
	})

	t.Run("move up clamped at first", func(t *testing.T) {
		toc.cursor = 0
		toc.moveUp()
		assert.Equal(t, 0, toc.cursor)
	})

	t.Run("single entry no movement", func(t *testing.T) {
		single := &mdTOC{entries: []tocEntry{{title: "Only", level: 1, lineIdx: 0}}, cursor: 0}
		single.moveUp()
		assert.Equal(t, 0, single.cursor)
		single.moveDown()
		assert.Equal(t, 0, single.cursor)
	})
}

func TestMdTOC_EnsureVisible(t *testing.T) {
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
			toc := &mdTOC{entries: entries, cursor: tt.cursor, offset: tt.offset}
			toc.ensureVisible(tt.height)
			assert.Equal(t, tt.wantOffset, toc.offset)
		})
	}
}

func TestMdTOC_UpdateActiveSection(t *testing.T) {
	toc := &mdTOC{entries: []tocEntry{
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
			toc.updateActiveSection(tt.diffCursor)
			assert.Equal(t, tt.wantActive, toc.activeSection)
		})
	}

	t.Run("empty entries", func(t *testing.T) {
		empty := &mdTOC{entries: nil, activeSection: 5}
		empty.updateActiveSection(10)
		assert.Equal(t, -1, empty.activeSection)
	})
}

func TestMdTOC_Render(t *testing.T) {
	res := style.PlainResolver()

	t.Run("empty TOC", func(t *testing.T) {
		toc := &mdTOC{entries: nil}
		got := toc.render(40, 10, paneTree, res)
		assert.Equal(t, "  no headers", got)
	})

	t.Run("single h1 entry, tree focused", func(t *testing.T) {
		toc := &mdTOC{entries: []tocEntry{{title: "Title", level: 1, lineIdx: 0}}, cursor: 0, activeSection: -1}
		got := toc.render(40, 10, paneTree, res)
		assert.Contains(t, got, "Title")
		// cursor should be highlighted with FileSelected style (reverse in plain)
		assert.NotEqual(t, "  Title", got) // styled, not plain
	})

	t.Run("indentation by level", func(t *testing.T) {
		toc := &mdTOC{entries: []tocEntry{
			{title: "H1", level: 1, lineIdx: 0},
			{title: "H2", level: 2, lineIdx: 5},
			{title: "H3", level: 3, lineIdx: 10},
		}, cursor: 0, activeSection: -1}
		got := toc.render(40, 10, paneDiff, res)
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 3)
		// h1 has no indent (level-1=0), h2 has 2 spaces, h3 has 4 spaces
		assert.True(t, strings.HasPrefix(lines[0], "  H1"), "h1 should have prefix spaces only: %q", lines[0])
		assert.Contains(t, lines[1], "  H2", "h2 should have 2 extra spaces indent: %q", lines[1])
		assert.Contains(t, lines[2], "    H3", "h3 should have 4 extra spaces indent: %q", lines[2])
	})

	t.Run("active section highlighted in diff focus", func(t *testing.T) {
		toc := &mdTOC{entries: []tocEntry{
			{title: "First", level: 1, lineIdx: 0},
			{title: "Second", level: 1, lineIdx: 10},
		}, cursor: 0, activeSection: 1}
		got := toc.render(40, 10, paneDiff, res)
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 2)
		// active section (Second) should be highlighted, first should not
		assert.Contains(t, lines[0], "First")
		assert.Contains(t, lines[1], "Second")
		// both lines contain their text, active section uses FileSelected (same as cursor)
		assert.Greater(t, len(lines[1]), len(lines[0]), "highlighted line should have style sequences")
	})

	t.Run("cursor highlighted in tree focus", func(t *testing.T) {
		toc := &mdTOC{entries: []tocEntry{
			{title: "First", level: 1, lineIdx: 0},
			{title: "Second", level: 1, lineIdx: 10},
		}, cursor: 1, activeSection: 0}
		got := toc.render(40, 10, paneTree, res)
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 2)
		// cursor (Second) should be highlighted, not active section
		assert.Greater(t, len(lines[1]), len(lines[0]), "cursor line should have style sequences")
	})

	t.Run("truncation of long title", func(t *testing.T) {
		toc := &mdTOC{entries: []tocEntry{
			{title: "This is a very long title that should be truncated", level: 1, lineIdx: 0},
		}, cursor: 0, activeSection: -1}
		got := toc.render(20, 10, paneDiff, res)
		assert.Contains(t, got, "…")
	})

	t.Run("scrolling offset", func(t *testing.T) {
		entries := make([]tocEntry, 20)
		for i := range entries {
			entries[i] = tocEntry{title: fmt.Sprintf("Header %d", i), level: 1, lineIdx: i * 10}
		}
		toc := &mdTOC{entries: entries, cursor: 15, offset: 0, activeSection: -1}
		got := toc.render(40, 5, paneTree, res)
		lines := strings.Split(got, "\n")
		assert.Len(t, lines, 5)
		assert.Contains(t, got, "Header 15")   // cursor entry should be visible
		assert.NotContains(t, got, "Header 0") // first entry should not be visible
	})

	t.Run("tree focus highlights cursor, diff focus highlights active section", func(t *testing.T) {
		toc := &mdTOC{entries: []tocEntry{
			{title: "A", level: 1, lineIdx: 0},
			{title: "B", level: 1, lineIdx: 10},
			{title: "C", level: 1, lineIdx: 20},
		}, cursor: 1, activeSection: 2}
		// in tree focus, cursor (B at idx 1) is highlighted
		gotTree := toc.render(40, 10, paneTree, res)
		treeLines := strings.Split(gotTree, "\n")
		require.Len(t, treeLines, 3)
		assert.Greater(t, len(treeLines[1]), len(treeLines[0]), "cursor B should be highlighted in tree focus")
		assert.Less(t, len(treeLines[2]), len(treeLines[1]), "C should not be highlighted in tree focus")

		// in diff focus, active section (C at idx 2) is highlighted
		gotDiff := toc.render(40, 10, paneDiff, res)
		diffLines := strings.Split(gotDiff, "\n")
		require.Len(t, diffLines, 3)
		assert.Greater(t, len(diffLines[2]), len(diffLines[0]), "active section C should be highlighted in diff focus")
		assert.Less(t, len(diffLines[1]), len(diffLines[2]), "B should not be highlighted in diff focus")
	})
}

func TestMdTOC_Render_ActiveSectionViewportVisibility(t *testing.T) {
	// when diff pane is focused and activeSection is far from cursor,
	// the TOC viewport should scroll to keep activeSection visible
	res := style.PlainResolver()
	entries := make([]tocEntry, 30)
	for i := range entries {
		entries[i] = tocEntry{title: fmt.Sprintf("Header %d", i), level: 1, lineIdx: i * 10}
	}
	toc := &mdTOC{entries: entries, cursor: 0, offset: 0, activeSection: 25}
	got := toc.render(40, 5, paneDiff, res)
	assert.Contains(t, got, "Header 25", "active section entry should be visible in viewport")
	assert.NotContains(t, got, "Header 0", "cursor entry should scroll out of viewport")
}

func TestMdTOC_TruncateTitle(t *testing.T) {
	toc := &mdTOC{}
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

func TestModel_IsFullContext(t *testing.T) {
	m := &Model{}
	tests := []struct {
		name  string
		lines []diff.DiffLine
		want  bool
	}{
		{name: "all context", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeContext},
		}, want: true},
		{name: "context with dividers", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeDivider}, {ChangeType: diff.ChangeContext},
		}, want: true},
		{name: "mixed with add", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeAdd},
		}, want: false},
		{name: "mixed with remove", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeRemove},
		}, want: false},
		{name: "empty", lines: nil, want: false},
		{name: "divider only", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeDivider},
		}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, m.isFullContext(tt.lines))
		})
	}
}

func TestModel_IsMarkdownFile(t *testing.T) {
	m := &Model{}
	tests := []struct {
		name string
		file string
		want bool
	}{
		{name: ".md", file: "README.md", want: true},
		{name: ".markdown", file: "notes.markdown", want: true},
		{name: ".go", file: "main.go", want: false},
		{name: ".MD uppercase", file: "README.MD", want: true},
		{name: ".MARKDOWN uppercase", file: "DOC.MARKDOWN", want: true},
		{name: "no extension", file: "Makefile", want: false},
		{name: "path with .md", file: "docs/guide.md", want: true},
		{name: "empty", file: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, m.isMarkdownFile(tt.file))
		})
	}
}
