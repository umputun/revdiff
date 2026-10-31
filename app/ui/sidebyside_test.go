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
	"github.com/umputun/revdiff/app/ui/style"
)

// sbsTestModel returns a plain-styled model with a mixed diff: context, a 2-remove/1-add block
// (one unpaired remove), a pure add, and trailing context.
func sbsTestModel(t *testing.T) Model {
	t.Helper()
	m := testModel(nil, nil)
	res := style.PlainResolver()
	m.resolver = res
	m.renderer = style.NewRenderer(res)
	m.sgr = style.SGR{}
	m.layout.width = 120
	m.layout.treeWidth = 0
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx one", ChangeType: diff.ChangeContext},   // 0
		{OldNum: 2, Content: "old first", ChangeType: diff.ChangeRemove},             // 1 unpaired
		{OldNum: 3, Content: "old second", ChangeType: diff.ChangeRemove},            // 2 paired with 3
		{NewNum: 2, Content: "new first", ChangeType: diff.ChangeAdd},                // 3
		{OldNum: 4, NewNum: 3, Content: "ctx two", ChangeType: diff.ChangeContext},   // 4
		{NewNum: 4, Content: "pure added", ChangeType: diff.ChangeAdd},               // 5
		{OldNum: 5, NewNum: 5, Content: "ctx three", ChangeType: diff.ChangeContext}, // 6
	}
	m.file.sbsPartners = m.computeSBSPartners()
	m.file.lineWidths = m.computeLineWidths()
	return m
}

func TestModel_ComputeSBSPartners(t *testing.T) {
	m := sbsTestModel(t)
	assert.Equal(t, []int{-1, -1, 3, 2, -1, -1, -1}, m.computeSBSPartners())

	t.Run("two blocks pair independently", func(t *testing.T) {
		m.file.lines = []diff.DiffLine{
			{Content: "r1", ChangeType: diff.ChangeRemove},
			{Content: "a1", ChangeType: diff.ChangeAdd},
			{Content: "c", ChangeType: diff.ChangeContext},
			{Content: "r2", ChangeType: diff.ChangeRemove},
			{Content: "a2", ChangeType: diff.ChangeAdd},
			{Content: "a3", ChangeType: diff.ChangeAdd},
		}
		assert.Equal(t, []int{1, 0, -1, 4, 3, -1}, m.computeSBSPartners())
	})
	t.Run("empty", func(t *testing.T) {
		m.file.lines = nil
		assert.Empty(t, m.computeSBSPartners())
	})
}

func TestModel_SBSHiddenOnlyForPairedRemoves(t *testing.T) {
	m := sbsTestModel(t)
	assert.False(t, m.isSBSHidden(1), "off by default")

	m.modes.sideBySide = true
	assert.True(t, m.isSBSHidden(2), "paired remove is folded into its add's row")
	assert.False(t, m.isSBSHidden(1), "unpaired remove keeps its own row")
	assert.False(t, m.isSBSHidden(3), "adds are never hidden")
	assert.True(t, m.isCollapsedHidden(2, nil), "navigation sees it as hidden")
	assert.False(t, m.isSBSHidden(99))
	assert.False(t, m.isSBSHidden(-1))

	m.modes.collapsed.enabled = true
	assert.False(t, m.sideBySideActive(), "collapsed wins over side-by-side")
}

func TestModel_SBSRender(t *testing.T) {
	m := sbsTestModel(t)
	m.modes.sideBySide = true
	m.modes.lineNumbers = true
	m.file.lineNumWidth = 2

	rows := strings.Split(strings.TrimSuffix(ansi.Strip(m.renderDiff()), "\n"), "\n")
	require.Len(t, rows, 6, "7 lines minus the one remove folded into a pair")

	wantWidth := lipgloss.Width(rows[0])
	for i, r := range rows {
		assert.Equal(t, wantWidth, lipgloss.Width(r), "row %d must keep both columns aligned: %q", i, r)
	}

	sep := func(r string) int { return strings.Index(r, "│") }
	// context: both sides
	assert.Contains(t, rows[0][:sep(rows[0])], "ctx one")
	assert.Contains(t, rows[0][sep(rows[0])+len("│"):], "ctx one")
	// unpaired remove comes first, right side empty
	left, right, _ := strings.Cut(rows[1], "│")
	assert.Contains(t, left, "old first")
	assert.Empty(t, strings.TrimSpace(right))
	// pair: old on the left with its old number, new on the right with its new number
	left, right, _ = strings.Cut(rows[2], "│")
	assert.Contains(t, left, "old second")
	assert.Contains(t, left, " 3 ")
	assert.Contains(t, right, "new first")
	assert.Contains(t, right, " 2 ")
	assert.NotContains(t, left, "new first")
	assert.NotContains(t, right, "old second")
	// pure add: left side empty
	left, right, _ = strings.Cut(rows[4], "│")
	assert.Empty(t, strings.TrimSpace(strings.TrimPrefix(left, " ")))
	assert.Contains(t, right, "pure added")
	// the folded remove appears exactly once
	assert.Equal(t, 1, strings.Count(ansi.Strip(m.renderDiff()), "old second"))
}

func TestModel_SBSCursorSkipsFoldedRemove(t *testing.T) {
	m := sbsTestModel(t)
	m.modes.sideBySide = true
	m.nav.diffCursor = 0

	m.moveDiffCursorDown()
	assert.Equal(t, 1, m.nav.diffCursor, "unpaired remove has its own row")
	m.moveDiffCursorDown()
	assert.Equal(t, 3, m.nav.diffCursor, "line 2 is folded into the add's row")
	m.moveDiffCursorUp()
	assert.Equal(t, 1, m.nav.diffCursor, "going up skips the folded remove as well")
}

func TestModel_SBSHeightsAndNoWrap(t *testing.T) {
	m := sbsTestModel(t)
	m.modes.sideBySide = true
	m.modes.wrap = true
	m.file.lines[3].Content = strings.Repeat("long ", 80)
	m.file.sbsPartners = m.computeSBSPartners()

	assert.Equal(t, 0, m.hunkLineHeight(2, nil, map[string]bool{}), "folded remove takes no rows")
	assert.Equal(t, 1, m.hunkLineHeight(3, nil, map[string]bool{}), "side-by-side cuts instead of wrapping")
	assert.Equal(t, 1, m.wrappedLineCount(3))
	m.modes.sideBySide = false
	assert.Greater(t, m.wrappedLineCount(3), 1, "unified wrap mode still wraps")
}

func TestModel_ToggleSideBySide(t *testing.T) {
	m := sbsTestModel(t)
	m.nav.diffCursor = 2 // folded once side-by-side turns on
	m.modes.collapsed.enabled = true

	m.toggleSideBySide()
	assert.True(t, m.modes.sideBySide)
	assert.False(t, m.modes.collapsed.enabled, "turning side-by-side on turns collapsed off")
	assert.Equal(t, 3, m.nav.diffCursor, "cursor leaves the folded remove")

	m.toggleCollapsedMode()
	assert.True(t, m.modes.collapsed.enabled)
	assert.False(t, m.modes.sideBySide, "turning collapsed on turns side-by-side off")

	m.toggleSideBySide()
	m.toggleSideBySide()
	assert.False(t, m.modes.sideBySide)

	m.layout.focus = paneTree
	m.toggleSideBySide()
	assert.False(t, m.modes.sideBySide, "ignored unless the diff pane is focused")
}

func TestModel_SBSCacheInvalidatedByToggle(t *testing.T) {
	m := sbsTestModel(t)
	unified := m.renderDiff()
	m.modes.sideBySide = true
	sbs := m.renderDiff()
	assert.NotEqual(t, unified, sbs)
	m.modes.sideBySide = false
	assert.Equal(t, unified, m.renderDiff(), "returning to unified must not serve side-by-side rows from the cache")
}

func TestModel_SBSHorizontalScrollBound(t *testing.T) {
	m := sbsTestModel(t)
	m.file.lines[3].Content = strings.Repeat("x", 300)
	m.file.lineWidths = m.computeLineWidths()
	m.modes.sideBySide = true

	left, right := m.sbsHalfWidths()
	assert.Equal(t, m.diffContentWidth(), left+right+sbsSeparatorWidth)
	m.setScrollX(10000)
	assert.Equal(t, m.maxRenderedContentWidth()-m.sbsVisibleWidth(), m.layout.scrollX)

	rows := strings.Split(strings.TrimSuffix(ansi.Strip(m.renderDiff()), "\n"), "\n")
	for i, r := range rows {
		assert.Equal(t, lipgloss.Width(rows[0]), lipgloss.Width(r), "row %d stays aligned while scrolled", i)
	}
}

func TestModel_SBSAnnotationJumpToFoldedRemove(t *testing.T) {
	m := sbsTestModel(t)
	m.modes.sideBySide = true
	m.positionOnAnnotation(annotation.Annotation{File: "a.go", Line: 3, Type: "-", Comment: "c"})
	assert.Equal(t, 3, m.nav.diffCursor, "lands on the paired add")
	assert.False(t, m.annot.cursorOnAnnotation)
}
