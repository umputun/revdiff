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
)

// sbsLines is a mixed diff: context, a block of two removes and one add (the first remove stays
// unpaired, the second pairs with the add), context, a pure add, and trailing context.
func sbsLines() []diff.DiffLine {
	return []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx one", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old first", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "old second", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new first", ChangeType: diff.ChangeAdd},
		{OldNum: 4, NewNum: 3, Content: "ctx two", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "pure added", ChangeType: diff.ChangeAdd},
		{OldNum: 5, NewNum: 5, Content: "ctx three", ChangeType: diff.ChangeContext},
	}
}

// loadSBS returns a model with lines loaded through the real load path, side-by-side on.
// prepare runs before the load, so it can add annotations the load has to react to.
func loadSBS(t *testing.T, lines []diff.DiffLine, prepare func(m *Model)) Model {
	t.Helper()
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.modes.sideBySide = true
	if prepare != nil {
		prepare(&m)
	}
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}}})
	m = result.(Model)
	result, _ = m.Update(m.loadFileDiff("a.go")())
	m = result.(Model)
	m.layout.focus = paneDiff
	return m
}

func rowsOf(m Model) []string {
	return strings.Split(strings.TrimSuffix(ansi.Strip(m.renderDiff()), "\n"), "\n")
}

// pressToggleHunk presses the toggle-hunk key.
func pressToggleHunk(m Model) Model {
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}})
	return result.(Model)
}

func TestModel_ComputeSBSPairing(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	partners, regions := m.computeSBSPairing()
	assert.Equal(t, []int{-1, -1, 3, 2, -1, -1, -1}, partners)
	assert.Equal(t, []int{-1, 1, 1, 1, -1, 5, -1}, regions)

	tests := []struct {
		name         string
		lines        []diff.ChangeType
		wantPartners []int
		wantRegions  []int
	}{
		{"two blocks pair independently",
			[]diff.ChangeType{diff.ChangeRemove, diff.ChangeAdd, diff.ChangeContext, diff.ChangeRemove, diff.ChangeAdd, diff.ChangeAdd},
			[]int{1, 0, -1, 4, 3, -1}, []int{0, 0, -1, 3, 3, 3}},
		{"alternating runs share a region",
			[]diff.ChangeType{diff.ChangeRemove, diff.ChangeAdd, diff.ChangeRemove, diff.ChangeAdd},
			[]int{1, 0, 3, 2}, []int{0, 0, 0, 0}},
		{"adds only", []diff.ChangeType{diff.ChangeAdd, diff.ChangeAdd}, []int{-1, -1}, []int{0, 0}},
		{"empty", nil, []int{}, []int{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var lines []diff.DiffLine
			for _, ct := range tc.lines {
				lines = append(lines, diff.DiffLine{Content: "x", ChangeType: ct})
			}
			m := testModel(nil, nil)
			m.file.lines = lines
			gotPartners, gotRegions := m.computeSBSPairing()
			assert.Equal(t, tc.wantPartners, gotPartners)
			assert.Equal(t, tc.wantRegions, gotRegions)
		})
	}
}

func TestModel_SBSHiddenOnlyForFoldedPairedRemoves(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	assert.True(t, m.isSBSHidden(2), "paired remove is folded into its add's row")
	assert.False(t, m.isSBSHidden(1), "unpaired remove keeps its own row")
	assert.False(t, m.isSBSHidden(3), "adds are never hidden")
	assert.True(t, m.isCollapsedHidden(2, nil), "navigation sees it as hidden")
	assert.False(t, m.isSBSHidden(99))
	assert.False(t, m.isSBSHidden(-1))

	m.modes.sideBySide = false
	assert.False(t, m.isSBSHidden(2), "off by default")

	m.modes.sideBySide = true
	m.modes.collapsed.enabled = true
	assert.False(t, m.sideBySideActive(), "collapsed wins over side-by-side")
}

func TestModel_SBSInactiveWithoutChanges(t *testing.T) {
	m := loadSBS(t, []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "same", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "text", ChangeType: diff.ChangeContext},
	}, nil)
	assert.False(t, m.sideBySideActive(), "a file without changes would show the same text twice")
	assert.NotContains(t, m.renderDiff(), "│")

	m.toggleSideBySide()
	assert.False(t, m.modes.sideBySide, "S turns the mode off even though it was never visible")
}

func TestModel_SBSRender(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) { m.modes.lineNumbers = true })
	m.layout.viewport.Height = 0

	rows := rowsOf(m)
	require.Len(t, rows, 6, "seven lines minus the one remove folded into a pair")
	for i, r := range rows {
		assert.Equal(t, lipgloss.Width(rows[0]), lipgloss.Width(r), "row %d keeps both columns aligned: %q", i, r)
	}

	half := func(row string) (left, right string) {
		left, right, _ = strings.Cut(row, "│")
		return left, right
	}
	left, right := half(rows[0])
	assert.Contains(t, left, "ctx one")
	assert.Contains(t, right, "ctx one")

	left, right = half(rows[1])
	assert.Contains(t, left, "old first")
	assert.Empty(t, strings.TrimSpace(right), "an unpaired remove has nothing opposite")

	left, right = half(rows[2])
	assert.Contains(t, left, "old second")
	assert.Contains(t, left, " 3 ", "the old side shows the old line number")
	assert.Contains(t, right, "new first")
	assert.Contains(t, right, " 2 ", "the new side shows the new line number")
	assert.NotContains(t, left, "new first")
	assert.NotContains(t, right, "old second")

	left, right = half(rows[4])
	assert.Empty(t, strings.TrimSpace(left), "a pure add has nothing opposite")
	assert.Contains(t, right, "pure added")

	assert.Equal(t, 1, strings.Count(ansi.Strip(m.renderDiff()), "old second"), "the folded remove is painted once")
}

func TestModel_SBSFillsViewportWithSeparator(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.layout.viewport.Height = 20

	rows := rowsOf(m)
	require.Len(t, rows, 20, "short content is padded to the pane height")
	for i, r := range rows {
		assert.Equal(t, 1, strings.Count(r, "│"), "row %d carries the separator: %q", i, r)
		assert.Equal(t, lipgloss.Width(rows[0]), lipgloss.Width(r), "row %d", i)
	}

	m.layout.viewport.Height = 3
	assert.Len(t, rowsOf(m), 6, "content taller than the pane is not padded")
}

func TestModel_SBSCursorSkipsFoldedRemove(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.nav.diffCursor = 0

	m.moveDiffCursorDown()
	assert.Equal(t, 1, m.nav.diffCursor, "the unpaired remove has its own row")
	m.moveDiffCursorDown()
	assert.Equal(t, 3, m.nav.diffCursor, "the folded remove shares the add's row")
	m.moveDiffCursorUp()
	assert.Equal(t, 1, m.nav.diffCursor, "going up skips the folded remove as well")
}

func TestModel_SBSHeightsAndNoWrap(t *testing.T) {
	lines := sbsLines()
	lines[3].Content = strings.Repeat("long ", 80)
	m := loadSBS(t, lines, func(m *Model) { m.modes.wrap = true })

	assert.Equal(t, 0, m.hunkLineHeight(2, nil, map[string]bool{}), "folded remove takes no rows")
	assert.Equal(t, 1, m.hunkLineHeight(3, nil, map[string]bool{}), "side-by-side cuts instead of wrapping")
	m.modes.sideBySide = false
	assert.Greater(t, m.wrappedLineCount(3), 1, "unified wrap mode still wraps")
}

func TestModel_ToggleSideBySide(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) { m.modes.sideBySide = false })
	m.modes.collapsed.enabled = true

	m.toggleSideBySide()
	assert.True(t, m.modes.sideBySide)
	assert.False(t, m.modes.collapsed.enabled, "turning side-by-side on turns collapsed off")

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

func TestModel_ToggleSideBySide_CursorLeavesFoldedRemove(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) { m.modes.sideBySide = false })
	m.nav.diffCursor = 2
	m.toggleSideBySide()
	assert.Equal(t, 3, m.nav.diffCursor)
}

func TestModel_ToggleSideBySide_ConfigWithCollapsed(t *testing.T) {
	// side-by-side from the config file plus --collapsed: collapsed wins until the first S
	m := loadSBS(t, sbsLines(), func(m *Model) { m.modes.collapsed.enabled = true })
	require.False(t, m.sideBySideActive())

	m.toggleSideBySide()
	assert.True(t, m.sideBySideActive(), "the first S shows side-by-side instead of doing nothing")
	assert.False(t, m.modes.collapsed.enabled)
}

func TestModel_ToggleSideBySide_OffWithWrapResetsScroll(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) { m.modes.wrap = true })
	m.layout.scrollX = 12

	m.toggleSideBySide()
	assert.False(t, m.modes.sideBySide)
	assert.Zero(t, m.layout.scrollX, "wrap mode cannot scroll back, so divider labels would stay cut")
}

func TestModel_SBSCacheInvalidatedByToggle(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) { m.modes.sideBySide = false })
	unified := m.renderDiff()
	m.modes.sideBySide = true
	assert.NotEqual(t, unified, m.renderDiff())
	m.modes.sideBySide = false
	assert.Equal(t, unified, m.renderDiff(), "returning to unified must not serve side-by-side rows from the cache")
}

func TestModel_SBSHorizontalScroll(t *testing.T) {
	lines := sbsLines()
	lines[3].Content = strings.Repeat("x", 300)
	m := loadSBS(t, lines, nil)
	m.layout.viewport.Height = 0

	left, right := m.sbsHalfWidths()
	assert.Equal(t, m.diffContentWidth(), left+right+sbsSeparatorWidth)

	m.setScrollX(10000)
	assert.Equal(t, m.maxRenderedContentWidth()-m.sbsVisibleWidth(), m.layout.scrollX)

	m.setScrollX(20)
	rows := rowsOf(m)
	for i, r := range rows {
		assert.Equal(t, lipgloss.Width(rows[0]), lipgloss.Width(r), "row %d stays aligned while scrolled", i)
	}
	l, r, _ := strings.Cut(rows[2], "│")
	assert.Contains(t, r, "«", "a half scrolled past its start shows the left marker")
	assert.Contains(t, r, "»", "a half with more content to the right shows the right marker")
	assert.NotContains(t, l, "»", "the short old line fits its half and has no marker")
}

func TestModel_SBSHorizontalWheelWorksWithWrap(t *testing.T) {
	lines := sbsLines()
	lines[3].Content = strings.Repeat("x", 300)
	m := loadSBS(t, lines, func(m *Model) { m.modes.wrap = true })

	result, _ := m.handleHorizontalWheel(8)
	assert.Equal(t, 8, result.(Model).layout.scrollX, "wrap is ignored in side-by-side mode, so the wheel scrolls")

	m.modes.sideBySide = false
	result, _ = m.handleHorizontalWheel(8)
	assert.Zero(t, result.(Model).layout.scrollX, "unified wrap mode still ignores it")
}

func TestModel_SBSExpandRegionGivesOldSideARow(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.layout.viewport.Height = 0
	m.nav.diffCursor = 3
	require.Len(t, rowsOf(m), 6)

	m = pressToggleHunk(m)
	assert.True(t, m.sbsExpanded(3))
	assert.False(t, m.isSBSHidden(2), "the remove now has a row of its own")
	rows := rowsOf(m)
	require.Len(t, rows, 7, "the folded remove is a row again")
	assert.Contains(t, rows[2], "old second")
	assert.Contains(t, rows[3], "new first")
	l, _, _ := strings.Cut(rows[3], "│")
	assert.Empty(t, strings.Trim(l, "▶ "), "an expanded add has nothing opposite")

	m.nav.diffCursor = 2
	m.startAnnotation()
	require.True(t, m.annot.annotating, "the old side can be annotated once its region is expanded")
	m.annot.input.SetValue("old note")
	m.saveAnnotation()
	got := m.store.Get("a.go")
	require.Len(t, got, 1)
	assert.Equal(t, "-", got[0].Type)
	assert.Equal(t, 3, got[0].Line)

	m = pressToggleHunk(m)
	assert.True(t, m.sbsExpanded(2), "a region with a note on a removed line stays expanded")
	assert.Contains(t, m.compact.hint, "note")
}

func TestModel_SBSExpandToggleFoldsAgain(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.nav.diffCursor = 3
	m = pressToggleHunk(m)
	require.True(t, m.sbsExpanded(3))
	m.nav.diffCursor = 2
	m = pressToggleHunk(m)
	assert.False(t, m.sbsExpanded(2), "a second press folds the region back")
	assert.Equal(t, 3, m.nav.diffCursor, "the cursor leaves the remove that just folded away")
}

func TestModel_SBSPreloadedRemoveNoteExpandsRegion(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "-", Comment: "old side note"})
	})
	assert.True(t, m.sbsExpanded(2), "a note on a folded remove would otherwise be invisible")
	assert.Contains(t, ansi.Strip(m.renderDiff()), "old side note")
}

func TestModel_SBSAnnotNavNotStuckOnOldSideNote(t *testing.T) {
	m := loadSBS(t, sbsLines(), func(m *Model) {
		m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "-", Comment: "old"})
		m.store.Add(annotation.Annotation{File: "a.go", Line: 4, Type: "+", Comment: "new"})
	})
	m.nav.diffCursor = 0

	step := func(forward bool) Model {
		result, _ := m.handleAnnotNav(forward)
		return result.(Model)
	}
	m = step(true)
	assert.Equal(t, 2, m.nav.diffCursor)
	assert.True(t, m.annot.cursorOnAnnotation, "} lands on the old-side note")
	m = step(true)
	assert.Equal(t, 5, m.nav.diffCursor, "a second } moves past it")
	m = step(false)
	assert.Equal(t, 2, m.nav.diffCursor, "{ comes back to the old-side note")
	m = step(false)
	assert.Equal(t, 2, m.nav.diffCursor, "and a second { has nothing before it")
	assert.True(t, m.annot.cursorOnAnnotation)

	m.nav.diffCursor, m.annot.cursorOnAnnotation = 2, true
	m.deleteAnnotation()
	assert.Len(t, m.store.Get("a.go"), 1, "the old-side note can be deleted")
}

func TestModel_SBSSearchReachesOldSide(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.nav.diffCursor = 0
	m.search.input.SetValue("old second")
	m.submitSearch()

	assert.Equal(t, 2, m.nav.diffCursor, "a match on a folded remove moves the cursor onto it")
	assert.True(t, m.sbsExpanded(2), "which expands its region so the cursor has a row")
	assert.Equal(t, "1/1", m.searchSegment())

	m = loadSBS(t, sbsLines(), nil)
	m.nav.diffCursor = 0
	m.search.input.SetValue("old")
	m.submitSearch()
	m.nextSearchMatch()
	assert.Equal(t, 2, m.nav.diffCursor, "n steps through matches on both removes")
	m.prevSearchMatch()
	assert.Equal(t, 1, m.nav.diffCursor)
}

func TestModel_SBSSearchHighlightsOldSide(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.search.term = "old second"
	m.search.matches = []int{2}
	plain := m.renderDiff()

	m.search.term = "old"
	m.search.matches = []int{1, 2}
	assert.NotEqual(t, plain, m.renderDiff(), "the paired remove's match changes how the row paints")
}

func TestModel_SBSIntraRangesFollowShownPairs(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, Content: "foo(a)", ChangeType: diff.ChangeRemove},
		{OldNum: 2, Content: "bar()", ChangeType: diff.ChangeRemove},
		{NewNum: 1, Content: "foo(b)", ChangeType: diff.ChangeAdd},
	}
	m := loadSBS(t, lines, func(m *Model) { m.modes.wordDiff = true })

	_, plusShown := m.differ.ComputeIntraRanges("bar()", "foo(b)")
	assert.Equal(t, plusShown, m.file.intraRanges[2], "the add is highlighted against the remove drawn beside it")

	m.modes.sideBySide = false
	m.recomputeIntraRanges()
	_, plusUnified := m.differ.ComputeIntraRanges("foo(a)", "foo(b)")
	assert.Equal(t, plusUnified, m.file.intraRanges[2], "the unified view keeps its content-matched pairing")
}

func TestModel_SBSStatusIconsDegradeOnNarrowTerminal(t *testing.T) {
	m := loadSBS(t, sbsLines(), nil)
	m.file.name = "very/long/path/to/some/deeply/nested/file/in/the/project/structure.go"

	m.layout.width = 200
	wide := m.statusBarText()
	assert.Contains(t, wide, "║")
	assert.Contains(t, wide, "⊟", "a wide terminal shows every icon")

	m.layout.width = 45
	narrow := m.statusBarText()
	assert.Contains(t, narrow, "║", "an active mode stays visible")
	assert.NotContains(t, narrow, "⊟", "inactive icons are dropped to leave the filename room")
	assert.LessOrEqual(t, lipgloss.Width(narrow), m.layout.width-2)
}
