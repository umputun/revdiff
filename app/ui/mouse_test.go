package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

func TestModel_statusBarHeight(t *testing.T) {
	tests := []struct {
		name        string
		noStatusBar bool
		want        int
	}{
		{"status bar visible", false, 1},
		{"status bar hidden", true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.cfg.noStatusBar = tc.noStatusBar
			assert.Equal(t, tc.want, m.statusBarHeight())
		})
	}
}

func TestModel_diffTopRow(t *testing.T) {
	// diffTopRow is constant regardless of layout state — it always accounts
	// for the pane top border (row 0) and the diff header (row 1).
	m := testModel([]string{"a.go"}, nil)
	assert.Equal(t, 2, m.diffTopRow(), "diff viewport starts at row 2 (below border + header)")

	t.Run("single file mode", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.file.singleFile = true
		assert.Equal(t, 2, m.diffTopRow())
	})

	t.Run("no status bar", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.cfg.noStatusBar = true
		assert.Equal(t, 2, m.diffTopRow())
	})

	t.Run("tree hidden", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.layout.treeHidden = true
		assert.Equal(t, 2, m.diffTopRow())
	})
}

func TestModel_treeTopRow(t *testing.T) {
	// treeTopRow is constant — tree content begins at row 1 (below the top border).
	m := testModel([]string{"a.go"}, nil)
	assert.Equal(t, 1, m.treeTopRow(), "tree content starts at row 1 (below border)")

	t.Run("no status bar", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.cfg.noStatusBar = true
		assert.Equal(t, 1, m.treeTopRow())
	})
}

func TestModel_hitTest(t *testing.T) {
	// baseline layout: width=120, height=40, treeWidth=36, status bar visible.
	// tree block occupies x=[0, 37] (treeWidth+2 cols with borders),
	// diff block x=[38, 119].
	// rows: 0=top border, 1=diff header / tree row 0, 2..=content, 38=bottom border, 39=status.
	tests := []struct {
		name  string
		setup func(m *Model)
		x, y  int
		want  hitZone
	}{
		{name: "tree pane entry row", setup: func(m *Model) {}, x: 5, y: 3, want: hitTree},
		{name: "tree pane top border", setup: func(m *Model) {}, x: 5, y: 0, want: hitNone},
		{name: "tree pane first content row", setup: func(m *Model) {}, x: 5, y: 1, want: hitTree},
		{name: "diff pane viewport row", setup: func(m *Model) {}, x: 60, y: 10, want: hitDiff},
		{name: "diff pane header row", setup: func(m *Model) {}, x: 60, y: 1, want: hitHeader},
		{name: "diff pane top border", setup: func(m *Model) {}, x: 60, y: 0, want: hitNone},
		{name: "status bar", setup: func(m *Model) {}, x: 60, y: 39, want: hitStatus},
		{name: "status bar at x=0", setup: func(m *Model) {}, x: 0, y: 39, want: hitStatus},
		{name: "tree-diff boundary: last tree column", setup: func(m *Model) {}, x: 37, y: 10, want: hitTree},
		{name: "tree-diff boundary: first diff column", setup: func(m *Model) {}, x: 38, y: 10, want: hitDiff},
		{name: "x negative", setup: func(m *Model) {}, x: -1, y: 10, want: hitNone},
		{name: "y negative", setup: func(m *Model) {}, x: 60, y: -1, want: hitNone},
		{name: "x out of bounds (= width)", setup: func(m *Model) {}, x: 120, y: 10, want: hitNone},
		{name: "y out of bounds (= height)", setup: func(m *Model) {}, x: 60, y: 40, want: hitNone},
		{
			name: "tree hidden: click in former tree area goes to diff",
			setup: func(m *Model) {
				m.layout.treeHidden = true
				m.layout.treeWidth = 0
			},
			x: 5, y: 10, want: hitDiff,
		},
		{
			name: "tree hidden: y=1 is diff header even at x=0",
			setup: func(m *Model) {
				m.layout.treeHidden = true
				m.layout.treeWidth = 0
			},
			x: 5, y: 1, want: hitHeader,
		},
		{
			name: "single file without TOC: no tree zone",
			setup: func(m *Model) {
				m.file.singleFile = true
				m.file.mdTOC = nil
				m.layout.treeWidth = 0
			},
			x: 5, y: 10, want: hitDiff,
		},
		{
			name: "single file with TOC: tree zone active",
			setup: func(m *Model) {
				m.file.singleFile = true
				m.file.mdTOC = sidepane.ParseTOC(
					[]diff.DiffLine{{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext}},
					"README.md",
				)
			},
			x: 5, y: 10, want: hitTree,
		},
		{
			name: "no status bar: last row is pane bottom border",
			setup: func(m *Model) {
				m.cfg.noStatusBar = true
			},
			x: 60, y: 39, want: hitNone,
		},
		{
			name: "no status bar: last row in tree zone is pane bottom border",
			setup: func(m *Model) {
				m.cfg.noStatusBar = true
			},
			x: 5, y: 39, want: hitNone,
		},
		{
			name: "no status bar: second-to-last row is diff content",
			setup: func(m *Model) {
				m.cfg.noStatusBar = true
			},
			x: 60, y: 38, want: hitDiff,
		},
		{name: "diff pane bottom border", setup: func(m *Model) {}, x: 60, y: 38, want: hitNone},
		{name: "tree pane bottom border", setup: func(m *Model) {}, x: 5, y: 38, want: hitNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.layout.width = 120
			m.layout.height = 40
			m.layout.treeWidth = 36
			tc.setup(&m)
			assert.Equal(t, tc.want, m.hitTest(tc.x, tc.y),
				"hitTest(%d, %d) with setup %q", tc.x, tc.y, tc.name)
		})
	}
}

// mouseTestModel builds a Model with known layout dimensions, a diff loaded
// into the viewport, and a multi-file tree. suitable for exercising
// handleMouse routing end-to-end. caller may tweak any grouped state after
// the call, e.g. m.layout.focus, m.file.mdTOC.
func mouseTestModel(t *testing.T, files []string, diffs map[string][]diff.DiffLine) Model {
	t.Helper()
	m := testModel(files, diffs)
	m.tree = testNewFileTree(files)
	m.layout.width = 120
	m.layout.height = 40
	m.layout.treeWidth = 36
	m.layout.viewport = viewport.New(80, 30)
	if len(files) > 0 {
		m.file.name = files[0]
		if d, ok := diffs[files[0]]; ok {
			m.file.lines = d
		}
	}
	return m
}

// wheelMsg builds a wheel-up or wheel-down MouseMsg at (x, y) with an
// optional shift modifier.
func wheelMsg(button tea.MouseButton, x, y int, shift bool) tea.MouseMsg {
	return tea.MouseMsg(tea.MouseEvent{
		X: x, Y: y, Shift: shift, Button: button, Action: tea.MouseActionPress,
	})
}

// leftPressAt builds a left-click press MouseMsg at (x, y).
func leftPressAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg(tea.MouseEvent{
		X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
}

func TestModel_HandleMouse_WheelInDiff(t *testing.T) {
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())

	// wheel-down scrolls the viewport by wheelStep; the cursor at line 0
	// is now above the visible range and is pinned to the new top.
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, wheelStep, model.layout.viewport.YOffset, "wheel-down must scroll viewport by wheelStep")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "cursor must pin to top of visible range when scrolled off-screen")

	// wheel-up scrolls the viewport back; cursor at line 3 stays in view, so it does not move.
	result, _ = model.Update(wheelMsg(tea.MouseButtonWheelUp, 60, 10, false))
	model = result.(Model)
	assert.Equal(t, 0, model.layout.viewport.YOffset, "wheel-up must scroll viewport back to the top")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "cursor stays put when its visual range still overlaps the viewport")
}

func TestModel_HandleMouse_WheelInDiff_CursorStaysWhenInView(t *testing.T) {
	// when the cursor is well inside the viewport, plain wheel scrolling that
	// does not push the cursor out must leave the cursor on its original line.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())
	m.nav.diffCursor = 20 // cursor sits comfortably inside the 30-row viewport

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, wheelStep, model.layout.viewport.YOffset)
	assert.Equal(t, 20, model.nav.diffCursor, "cursor must not move while still inside the viewport")
}

func TestModel_HandleMouse_WheelInDiff_NoopWhenContentFits(t *testing.T) {
	// when the entire diff fits in the viewport (TotalLineCount <= Height),
	// there is no room to scroll and wheel events become no-ops. cursor and
	// YOffset both stay put.
	lines := make([]diff.DiffLine, 5)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, 0, model.layout.viewport.YOffset, "wheel must not change YOffset when content fits")
	assert.Equal(t, 0, model.nav.diffCursor, "wheel must not change cursor when content fits")
}

func TestModel_HandleMouse_WheelUp_PinsCursorToBottom(t *testing.T) {
	// wheel-up when cursor is below the new viewport range must pin the cursor
	// to the bottommost visible row. exercises the targetRow=viewBottom branch.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())
	// start with viewport scrolled down and cursor at the bottom of the file.
	m.layout.viewport.SetYOffset(30)
	m.nav.diffCursor = 59
	require.Equal(t, 30, m.layout.viewport.YOffset)

	// wheel-up: viewport scrolls up by wheelStep, cursor at 59 is below new view.
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelUp, 60, 10, false))
	model := result.(Model)
	newOffset := 30 - wheelStep
	assert.Equal(t, newOffset, model.layout.viewport.YOffset, "wheel-up must scroll viewport up by wheelStep")
	wantCursor := newOffset + model.layout.viewport.Height - 1
	assert.Equal(t, wantCursor, model.nav.diffCursor, "cursor must pin to bottom of new visible range")
}

func TestModel_HandleMouse_WheelDown_NoopAtMaxOffset(t *testing.T) {
	// when the viewport is already at maximum scroll offset, wheel-down must be
	// a no-op: YOffset and cursor both stay unchanged.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())
	maxOffset := m.layout.viewport.TotalLineCount() - m.layout.viewport.Height
	require.Positive(t, maxOffset, "test requires a scrollable diff")
	m.layout.viewport.SetYOffset(maxOffset)
	m.nav.diffCursor = 30

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, maxOffset, model.layout.viewport.YOffset, "wheel-down at max offset must not change YOffset")
	assert.Equal(t, 30, model.nav.diffCursor, "wheel-down at max offset must not change cursor")
}

func TestModel_HandleMouse_WheelInWrapMode_CursorAboveViewport(t *testing.T) {
	// in wrap mode, when wheel-down scrolls the viewport so that the cursor's first
	// visual row (marker row) is entirely above the new viewTop, the cursor must pin
	// to the first line whose marker row is within the new viewport.
	// this exercises the cursorTop < viewTop && cursorBottom < viewTop path.
	lines := make([]diff.DiffLine, 40)
	lines[0] = diff.DiffLine{NewNum: 1, Content: strings.Repeat("X", 90), ChangeType: diff.ChangeContext}
	for i := 1; i < len(lines); i++ {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "short", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.modes.wrap = true
	m.layout.viewport.SetContent(m.renderDiff())

	cursorTop, cursorBottom := m.cursorVisualRange()
	require.Equal(t, 0, cursorTop)
	require.Greater(t, cursorBottom, cursorTop, "line 0 must wrap for this test to be meaningful")
	// with wheelStep=3 past cursorBottom, cursor is entirely above the new viewport.
	require.Less(t, cursorBottom, wheelStep, "cursorBottom must be < wheelStep to exercise the above-viewport path")

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	require.Equal(t, wheelStep, model.layout.viewport.YOffset)
	assert.Positive(t, model.nav.diffCursor, "cursor must advance to a line whose marker row is in the new viewport")
}

func TestModel_HandleMouse_WheelInWrapMode_StraddlePinsToNextLine(t *testing.T) {
	// in wrap mode, when wheel-down scrolls the viewport so that viewTop lands
	// INSIDE the cursor line's wrapped span (cursorTop < viewTop <= cursorBottom),
	// the cursor must advance past the continuation rows to the next line.
	// this exercises the cursorBottom+1 straddle branch in pinDiffCursorTo.
	//
	// mouseTestModel layout: width=120, treeWidth=36 → wrapWidth≈77 chars/row.
	// a 350-char line wraps to 5 rows (rows 0-4), so cursorBottom=4 > wheelStep=3.
	// after wheel-down, viewTop=3 lands inside rows 0-4 → straddle fires.
	lines := make([]diff.DiffLine, 40)
	lines[0] = diff.DiffLine{NewNum: 1, Content: strings.Repeat("X", 350), ChangeType: diff.ChangeContext}
	for i := 1; i < len(lines); i++ {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "short", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.modes.wrap = true
	m.layout.viewport.SetContent(m.renderDiff())

	cursorTop, cursorBottom := m.cursorVisualRange()
	require.Equal(t, 0, cursorTop)
	require.Greater(t, cursorBottom, wheelStep,
		"cursorBottom (%d) must exceed wheelStep (%d) to exercise the straddle branch", cursorBottom, wheelStep)

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	require.Equal(t, wheelStep, model.layout.viewport.YOffset,
		"viewport must scroll by wheelStep")
	// viewTop=wheelStep is inside line 0's span [0, cursorBottom]; cursor must advance to line 1.
	assert.Equal(t, 1, model.nav.diffCursor,
		"cursor must advance to line 1 (first line after the straddling wrapped span)")
}

func TestModel_HandleMouse_ShiftWheelHalfPage(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport = viewport.New(80, 20) // Height=20 so half-page = 10
	m.layout.viewport.SetContent(m.renderDiff())

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, true))
	model := result.(Model)
	assert.Equal(t, 10, model.layout.viewport.YOffset, "shift+wheel must scroll viewport by half page")
	assert.Equal(t, 10, model.nav.diffCursor, "cursor must pin to top of visible range after half-page scroll")
}

func TestModel_HandleMouse_WheelInTreeMovesTreeCursor(t *testing.T) {
	files := make([]string, 30)
	diffs := make(map[string][]diff.DiffLine, 30)
	for i := range files {
		p := string(rune('a'+(i/10))) + string(rune('a'+(i%10))) + ".go"
		files[i] = p
		diffs[p] = []diff.DiffLine{{NewNum: 1, Content: p, ChangeType: diff.ChangeContext}}
	}
	m := mouseTestModel(t, files, diffs)
	m.layout.focus = paneDiff // wheel routing is by hit zone, not focus
	before := m.tree.SelectedFile()

	// wheel-down inside tree columns (x=5, y=3 → hitTree)
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
	model := result.(Model)
	assert.NotEqual(t, before, model.tree.SelectedFile(), "wheel in tree should change tree selection even when focus is diff")
}

func TestModel_HandleMouse_WheelNonPressActionIgnored(t *testing.T) {
	// bubbletea viewport guards wheel on MouseActionPress to avoid double-firing
	// if a terminal emits non-press wheel events (motion/release). handleMouse
	// matches that pattern for symmetry with the left-click guard.
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0

	for _, action := range []tea.MouseAction{tea.MouseActionRelease, tea.MouseActionMotion} {
		for _, btn := range []tea.MouseButton{tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
			msg := tea.MouseMsg(tea.MouseEvent{X: 60, Y: 10, Button: btn, Action: action})
			result, _ := m.Update(msg)
			model := result.(Model)
			assert.Equal(t, 0, model.nav.diffCursor, "wheel with Action=%v must be ignored", action)
		}
	}
}

func TestModel_HandleMouse_ShiftWheelInTreeUsesTreePageSize(t *testing.T) {
	// shift+wheel over the tree must move by treePageSize()/2 entries,
	// matching the keyboard half-page shortcut for that pane — not by
	// viewport.Height/2 which is the diff-pane half-page step.
	// force viewport.Height != treePageSize so the test fails under the old
	// formula.
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}},
	})
	m.layout.viewport.Height = 4 // small diff viewport
	// treePageSize derives from layout.height - 2 (- 1 if status bar present);
	// with the default mouseTestModel height it is larger than 4.
	assert.NotEqual(t, m.layout.viewport.Height/2, m.treePageSize()/2,
		"test precondition: viewport and tree half-pages must differ")

	// shift+wheel in tree zone (x < treeWidth+2, y >= treeTopRow)
	got := m.wheelStepFor(hitTree, true)
	assert.Equal(t, max(1, m.treePageSize()/2), got,
		"shift+wheel in tree must step by treePageSize/2, not viewport.Height/2")

	// sanity: shift+wheel in diff still uses viewport half-page
	got = m.wheelStepFor(hitDiff, true)
	assert.Equal(t, max(1, m.layout.viewport.Height/2), got,
		"shift+wheel in diff must step by viewport.Height/2")
}

func TestModel_HandleMouse_HorizontalWheelNoop(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0

	for _, btn := range []tea.MouseButton{tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight} {
		result, cmd := m.Update(wheelMsg(btn, 60, 10, false))
		model := result.(Model)
		assert.Nil(t, cmd)
		assert.Equal(t, 0, model.nav.diffCursor, "horizontal wheel must not move cursor")
	}
}

func TestModel_HandleMouse_LeftClickInDiff(t *testing.T) {
	lines := make([]diff.DiffLine, 40)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.focus = paneTree // click should flip focus to diff

	// click on diff row at y=12, diffTopRow=2, YOffset=0 → row 10
	result, _ := m.Update(leftPressAt(60, 12))
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus, "click in diff must set focus to paneDiff")
	assert.Equal(t, 10, model.nav.diffCursor, "click at y=12 with diffTopRow=2 must land on diff line 10")
	assert.False(t, model.annot.cursorOnAnnotation, "plain diff click must not set cursorOnAnnotation")
}

func TestModel_HandleMouse_LeftClickInDiffWithScrolledViewport(t *testing.T) {
	// verifies the full clickDiff formula: row = (y - diffTopRow()) + YOffset.
	// plan requires a YOffset > 0 case to guard against regressions in the
	// scroll-adjusted click mapping; without this, clicks on a scrolled
	// viewport could silently drop the YOffset term and land on the wrong
	// diff line.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.SetYOffset(5)
	m.layout.focus = paneTree

	// click at y=12 with diffTopRow=2, YOffset=5 → row 15
	result, _ := m.Update(leftPressAt(60, 12))
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus)
	assert.Equal(t, 15, model.nav.diffCursor, "click in scrolled viewport must add YOffset to logical row")
	assert.False(t, model.annot.cursorOnAnnotation)
}

func TestModel_HandleMouse_LeftClickInDiffNoopWhenNoFileLoaded(t *testing.T) {
	// mirrors the togglePane invariant: focus must not switch to paneDiff
	// when no file is loaded (e.g. clicks received before filesLoadedMsg
	// arrives or in an empty --only result).
	m := mouseTestModel(t, []string{"a.go"}, nil)
	m.file.name = ""
	m.file.lines = nil
	m.layout.focus = paneTree

	result, cmd := m.Update(leftPressAt(60, 12))
	model := result.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, paneTree, model.layout.focus, "click in diff with no file loaded must not flip focus")
	assert.Equal(t, 0, model.nav.diffCursor, "click with no file loaded must not move the diff cursor")
	assert.False(t, model.annot.cursorOnAnnotation)
}

func TestModel_HandleMouse_LeftClickSetsCursorOnAnnotation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	// store an annotation on line 1 so it renders an annotation sub-row
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	m.layout.focus = paneTree

	// diff line 0 has 1 diff row + 1 annotation row. diffTopRow=2, so y=3 → row 1 (annotation sub-row)
	result, _ := m.Update(leftPressAt(60, 3))
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus)
	assert.Equal(t, 0, model.nav.diffCursor, "click on annotation sub-row should keep logical index pointing at the diff line")
	assert.True(t, model.annot.cursorOnAnnotation, "click on annotation sub-row must set cursorOnAnnotation")
}

func TestModel_HandleMouse_LeftClickOnDeleteOnlyPlaceholderDoesNotLandOnAnnotation(t *testing.T) {
	// in collapsed mode, a delete-only hunk renders a single "⋯ N lines deleted"
	// placeholder. annotations on the underlying removed lines are NOT rendered
	// (see renderCollapsedDiff). clicking the placeholder must not set
	// cursorOnAnnotation, mirroring keyboard navigation guarded by
	// TestModel_CollapsedCursorDownSkipsPlaceholderAnnotation.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx1", ChangeType: diff.ChangeContext}, // 0
		{OldNum: 2, Content: "del1", ChangeType: diff.ChangeRemove},  // 1 - placeholder
		{OldNum: 3, Content: "del2", ChangeType: diff.ChangeRemove},  // 2 - hidden
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext}, // 3
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	// annotation on the hidden removed line — its sub-row is NOT rendered for a placeholder
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "hidden note"})

	// collapsed layout: row 0 = ctx1, row 1 = placeholder (idx 1), row 2 = ctx2 (idx 3).
	// diffTopRow=2 so y=3 targets the placeholder row.
	result, _ := m.Update(leftPressAt(60, 3))
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus)
	assert.Equal(t, 1, model.nav.diffCursor, "click on placeholder lands on placeholder line index")
	assert.False(t, model.annot.cursorOnAnnotation,
		"click on placeholder must not set cursorOnAnnotation — annotation is not rendered")
}

func TestModel_HandleMouse_LeftClickInTreeSelectsAndLoads(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go"}
	diffs := map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
		"c.go": {{NewNum: 1, Content: "c", ChangeType: diff.ChangeContext}},
	}
	m := mouseTestModel(t, files, diffs)
	m.layout.focus = paneDiff

	// treeTopRow=1; entries render at rows 1,2,3 → click y=3 hits entry at visible row 2 (c.go or b.go depending on layout)
	result, cmd := m.Update(leftPressAt(5, 3))
	model := result.(Model)
	assert.Equal(t, paneTree, model.layout.focus, "click in tree must set focus to paneTree")
	// whatever file gets selected must not be the same as the starting file and must trigger a load
	assert.NotEqual(t, "a.go", model.tree.SelectedFile(), "tree click should move cursor off first file")
	assert.NotNil(t, cmd, "tree click on a file entry should trigger file load")
}

func TestModel_HandleMouse_LeftClickOnStatusBarNoop(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0
	m.layout.focus = paneTree

	result, cmd := m.Update(leftPressAt(60, 39)) // last row = status
	model := result.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, paneTree, model.layout.focus, "status-bar click must not change focus")
	assert.Equal(t, 0, model.nav.diffCursor)
}

func TestModel_HandleMouse_LeftClickOnHeaderNoop(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0
	m.layout.focus = paneTree

	result, cmd := m.Update(leftPressAt(60, 1)) // diff header row
	model := result.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, paneTree, model.layout.focus, "diff header click must not change focus")
	assert.Equal(t, 0, model.nav.diffCursor)
}

func TestModel_HandleMouse_LeftClickOutOfBoundsNoop(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0
	m.layout.focus = paneTree

	result, cmd := m.Update(leftPressAt(999, 999))
	model := result.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, paneTree, model.layout.focus)
	assert.Equal(t, 0, model.nav.diffCursor)
}

func TestModel_HandleMouse_NonLeftButtonsNoop(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0
	m.layout.focus = paneTree

	buttons := []tea.MouseButton{
		tea.MouseButtonRight,
		tea.MouseButtonMiddle,
		tea.MouseButtonBackward,
		tea.MouseButtonForward,
	}
	for _, btn := range buttons {
		msg := tea.MouseMsg(tea.MouseEvent{X: 60, Y: 10, Button: btn, Action: tea.MouseActionPress})
		result, cmd := m.Update(msg)
		model := result.(Model)
		assert.Nil(t, cmd, "button %v must be no-op", btn)
		assert.Equal(t, paneTree, model.layout.focus, "button %v must not change focus", btn)
	}
}

func TestModel_HandleMouse_LeftReleaseAndMotionNoop(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.nav.diffCursor = 0
	m.layout.focus = paneTree

	for _, action := range []tea.MouseAction{tea.MouseActionRelease, tea.MouseActionMotion} {
		msg := tea.MouseMsg(tea.MouseEvent{X: 60, Y: 12, Button: tea.MouseButtonLeft, Action: action})
		result, cmd := m.Update(msg)
		model := result.(Model)
		assert.Nil(t, cmd, "action %v on left button must be no-op", action)
		assert.Equal(t, paneTree, model.layout.focus, "action %v must not change focus", action)
	}
}

func TestModel_HandleMouse_SwallowedWhileAnnotating(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.annot.annotating = true
	m.annot.input = textinput.New()
	m.annot.input.SetValue("draft")
	m.nav.diffCursor = 0

	// wheel must be swallowed: cursor unchanged, textinput value preserved
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "wheel must not move cursor while annotating")
	assert.True(t, model.annot.annotating, "annotating state must be preserved")

	// click must be swallowed too
	result, _ = m.Update(leftPressAt(60, 12))
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "click must not move cursor while annotating")
	assert.Equal(t, "draft", model.annot.input.Value(), "textinput value must be unchanged by mouse event")
}

func TestModel_HandleMouse_SwallowedWhileSearching(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.search.active = true
	m.search.input = textinput.New()
	m.nav.diffCursor = 0

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "wheel must not move cursor while search is active")
	assert.True(t, model.search.active)
}

func TestModel_HandleMouse_SwallowedWhileReloadPending(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.reload.pending = true
	m.reload.hint = "Annotations will be dropped — press y to confirm, any other key to cancel"
	m.nav.diffCursor = 0
	m.layout.focus = paneTree

	result, _ := m.Update(leftPressAt(60, 12))
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "click must be swallowed while reload confirmation is pending")
	assert.Equal(t, paneTree, model.layout.focus)
	assert.True(t, model.reload.pending)
	assert.Equal(t, "Annotations will be dropped — press y to confirm, any other key to cancel", model.reload.hint,
		"reload hint must stay visible while pending — otherwise the modal prompt vanishes but the modal remains")

	// wheel must also preserve the hint
	result, _ = m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model = result.(Model)
	assert.True(t, model.reload.pending)
	assert.NotEmpty(t, model.reload.hint, "wheel must not erase the reload prompt while pending")
}

func TestModel_HandleMouse_SwallowedWhileConfirmDiscard(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.inConfirmDiscard = true
	m.nav.diffCursor = 0

	result, _ := m.Update(leftPressAt(60, 12))
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor)
	assert.True(t, model.inConfirmDiscard)
}

func TestModel_HandleMouse_SwallowedWhileOverlayOpen(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	mgr, ok := m.overlay.(*overlay.Manager)
	require.True(t, ok, "test expects the real overlay.Manager")
	mgr.OpenHelp(overlay.HelpSpec{})
	require.True(t, m.overlay.Active())
	m.nav.diffCursor = 0

	// wheel over help is a no-op — overlay stays open and diff cursor is unchanged
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay must stay open after wheel event")
	assert.Equal(t, 0, model.nav.diffCursor, "wheel must not leak through to diff pane")

	// click while overlay active is swallowed — no focus change or cursor move
	result, _ = m.Update(leftPressAt(60, 12))
	model = result.(Model)
	assert.True(t, model.overlay.Active(), "overlay must stay open after click")
	assert.Equal(t, 0, model.nav.diffCursor)
}

func TestModel_HandleMouse_WheelScrollsInfoOverlay(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	mgr, ok := m.overlay.(*overlay.Manager)
	require.True(t, ok, "test expects the real overlay.Manager")

	commits := make([]diff.CommitInfo, 0, 20)
	for range 20 {
		commits = append(commits, diff.CommitInfo{Hash: "abc", Author: "a", Subject: "subject", Body: "body"})
	}
	mgr.OpenInfo(overlay.InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	ctx := overlay.RenderCtx{Width: m.layout.width, Height: m.layout.height, Resolver: m.resolver}
	_ = mgr.Compose(makeOverlayBase(m.layout.width, m.layout.height), ctx)
	require.True(t, m.overlay.Active())
	// park diff cursor mid-stream: if wheel leaks through, it would change
	// this value; overlay consuming the wheel leaves it intact.
	m.nav.diffCursor = 5

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "info overlay stays open after wheel")
	assert.Equal(t, 5, model.nav.diffCursor, "diff cursor must stay put — wheel is consumed by overlay, not the pane beneath")
	// exact offset advancement is verified by TestInfoOverlay_HandleMouse_WheelScrollsOffset
	// in the overlay package (has access to the private offset field).
}

func TestModel_HandleMouse_WheelScrollsAnnotListOverlay(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	mgr, ok := m.overlay.(*overlay.Manager)
	require.True(t, ok)

	items := []overlay.AnnotationItem{
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 1, ChangeType: "+"}, Comment: "one"},
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 2, ChangeType: "+"}, Comment: "two"},
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 3, ChangeType: "+"}, Comment: "three"},
	}
	mgr.OpenAnnotList(overlay.AnnotListSpec{Items: items})
	m.nav.diffCursor = 0

	// wheel-down must be consumed by the overlay; diff cursor must not move
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "annotlist overlay stays open after wheel")
	assert.Equal(t, 0, model.nav.diffCursor, "diff cursor must not move")
}

func TestModel_HandleMouse_WheelScrollsThemeSelectOverlay(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	mgr, ok := m.overlay.(*overlay.Manager)
	require.True(t, ok)

	mgr.OpenThemeSelect(overlay.ThemeSelectSpec{Items: []overlay.ThemeItem{
		{Name: "a"}, {Name: "b"},
	}, ActiveName: "a"})
	ctx := overlay.RenderCtx{Width: m.layout.width, Height: m.layout.height, Resolver: m.resolver}
	_ = mgr.Compose(makeOverlayBase(m.layout.width, m.layout.height), ctx)
	m.nav.diffCursor = 0

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "themeselect overlay stays open after wheel")
	assert.Equal(t, 0, model.nav.diffCursor)
}

// makeOverlayBase builds a base screen string for priming Manager.bounds via
// Compose. Joins blank lines with "\n" (no trailing newline) so lipgloss.Width
// and len(Split) match production View() output; Repeat("line\n", N) would add
// a phantom trailing row and shift centering math.
func makeOverlayBase(width, height int) string {
	line := strings.Repeat(" ", width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func TestModel_HandleMouse_ClickConfirmsThemeSelect(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	// real catalog so openThemeSelector builds entries and confirmThemeByName
	// can Resolve + Persist; otherwise themePreview stays nil and the dispatch
	// is a silent no-op (which the assertion must not hide).
	m.themes = newTestThemeCatalog()
	m.activeThemeName = "revdiff"
	m.openThemeSelector()
	require.NotNil(t, m.themePreview, "openThemeSelector must initialize the preview session")

	mgr, ok := m.overlay.(*overlay.Manager)
	require.True(t, ok)
	ctx := overlay.RenderCtx{Width: m.layout.width, Height: m.layout.height, Resolver: m.resolver}
	_ = mgr.Compose(makeOverlayBase(m.layout.width, m.layout.height), ctx)

	// themeselect layout with 3 entries on a 120x40 viewport: popup spans rows
	// 15-23 (9 rows total = top border, top pad, filter, blank, 3 entries, bot
	// pad, bot border). entries are at screen y=19, 20, 21. click the second
	// entry ("dracula") so confirmThemeByName changes activeThemeName from its
	// initial "revdiff".
	clickX, clickY := 60, 20
	result, _ := m.Update(leftPressAt(clickX, clickY))
	model := result.(Model)

	assert.False(t, model.overlay.Active(), "confirm outcome closes the overlay")
	// real Model-side effects of confirmThemeByName — not just the overlay-close
	// that Manager auto-fires. themePreview is cleared and activeThemeName is set
	// to the confirmed entry ("dracula", not the initial "revdiff").
	assert.Nil(t, model.themePreview, "confirmThemeByName must clear the preview session")
	assert.Equal(t, "dracula", model.activeThemeName, "confirmThemeByName must set activeThemeName to the clicked entry")
}

func TestModel_HandleMouse_ClickJumpsAnnotList(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
		"b.go": {{NewNum: 1, Content: "b", ChangeType: diff.ChangeContext}},
	})
	mgr, ok := m.overlay.(*overlay.Manager)
	require.True(t, ok)

	// multiple items so the popup is tall enough that a deterministic click
	// coordinate lands on an item row regardless of centering rounding.
	items := []overlay.AnnotationItem{
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 1, ChangeType: "+"}, Comment: "filler-0"},
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 2, ChangeType: "+"}, Comment: "filler-1"},
		{AnnotationTarget: overlay.AnnotationTarget{File: "b.go", Line: 1, ChangeType: "+"}, Comment: "target"},
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 3, ChangeType: "+"}, Comment: "filler-3"},
		{AnnotationTarget: overlay.AnnotationTarget{File: "a.go", Line: 4, ChangeType: "+"}, Comment: "filler-4"},
	}
	mgr.OpenAnnotList(overlay.AnnotListSpec{Items: items})
	ctx := overlay.RenderCtx{Width: m.layout.width, Height: m.layout.height, Resolver: m.resolver}
	_ = mgr.Compose(makeOverlayBase(m.layout.width, m.layout.height), ctx)

	// annotlist layout with 5 items on a 120x40 viewport: popup width = 70,
	// popup height = 5 items + 4 chrome = 9 rows. centered at startX=(120-70)/2=25,
	// startY=(40-9)/2=15. items[2] (b.go, the cross-file target) is at screen
	// y = startY + 2 + 2 = 19.
	clickX, clickY := 60, 19
	result, _ := m.Update(leftPressAt(clickX, clickY))
	model := result.(Model)

	assert.False(t, model.overlay.Active(), "jump outcome closes the overlay")
	// cross-file target (b.go != current a.go) — jumpToAnnotationTarget must
	// set pendingAnnotJump before loadSelectedIfChanged fires. this is the
	// real Model-side effect; Manager.HandleMouse alone would not set it.
	require.NotNil(t, model.pendingAnnotJump, "jumpToAnnotationTarget must set pendingAnnotJump for cross-file target")
	assert.Equal(t, "b.go", model.pendingAnnotJump.File)
	assert.Equal(t, 1, model.pendingAnnotJump.Line)
}

func TestModel_HandleMouse_ClearsTransientHints(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.reload.hint = "press y to reload"
	m.compact.hint = "not applicable"

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Empty(t, model.reload.hint)
	assert.Empty(t, model.compact.hint)
}

func TestModel_HandleMouse_StdinModeTreeHiddenClickLandsOnDiff(t *testing.T) {
	// stdin mode sets treeHidden = true with singleFile = true; mdTOC = nil
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
	}
	m := mouseTestModel(t, []string{"stdin"}, map[string][]diff.DiffLine{"stdin": lines})
	m.file.lines = lines
	m.layout.treeHidden = true
	m.file.singleFile = true
	m.layout.treeWidth = 0
	m.layout.focus = paneDiff

	// click at (x=0, y=4) — tree is hidden, so this should land on diff row 2
	result, _ := m.Update(leftPressAt(0, 4))
	model := result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus)
	assert.Equal(t, 2, model.nav.diffCursor, "with tree hidden, click at y=4 must land on diff row 2")
}

func TestModel_HandleMouse_WheelInTOCJumpsViewport(t *testing.T) {
	// markdown single-file with TOC: tree pane slot shows TOC; wheel must route to TOC.
	files := []string{"README.md"}
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "# A", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "# B", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "# C", ChangeType: diff.ChangeContext},
	}
	m := mouseTestModel(t, files, map[string][]diff.DiffLine{"README.md": lines})
	m.file.lines = lines
	m.file.singleFile = true
	m.file.mdTOC = sidepane.ParseTOC(lines, "README.md")
	require.NotNil(t, m.file.mdTOC)

	before, _ := m.file.mdTOC.CurrentLineIdx()

	// wheel-down at (x=5, y=3) hits the tree slot; with mdTOC active, route to TOC
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
	model := result.(Model)
	after, _ := model.file.mdTOC.CurrentLineIdx()
	assert.NotEqual(t, before, after, "TOC cursor must advance when wheel is used in TOC pane")
}

func TestModel_HandleMouse_LeftClickInTOCJumpsViewport(t *testing.T) {
	files := []string{"README.md"}
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "# A", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "# B", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "body", ChangeType: diff.ChangeContext},
	}
	m := mouseTestModel(t, files, map[string][]diff.DiffLine{"README.md": lines})
	m.file.lines = lines
	m.file.singleFile = true
	m.file.mdTOC = sidepane.ParseTOC(lines, "README.md")
	require.NotNil(t, m.file.mdTOC)
	m.layout.focus = paneDiff

	// click on row 1 of TOC (y=2, treeTopRow=1 → row 1)
	result, _ := m.Update(leftPressAt(5, 2))
	model := result.(Model)
	assert.Equal(t, paneTree, model.layout.focus, "TOC click must focus tree pane slot")
}

func TestModel_HandleMouse_WheelInDiffSyncsTOCActiveSection(t *testing.T) {
	// mirrors the keyboard-navigation TOC-sync guarantee (diffnav.go:464):
	// when wheel scrolling pushes the cursor off the top and pins it to the
	// new top visible line, mdTOC.activeSection must follow so switching focus
	// back to the TOC highlights the correct section. requires a diff long
	// enough for the viewport to actually scroll.
	files := []string{"README.md"}
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "body", ChangeType: diff.ChangeContext}
	}
	lines[0] = diff.DiffLine{NewNum: 1, Content: "# First", ChangeType: diff.ChangeContext}
	lines[10] = diff.DiffLine{NewNum: 11, Content: "## Second", ChangeType: diff.ChangeContext}
	lines[20] = diff.DiffLine{NewNum: 21, Content: "### Third", ChangeType: diff.ChangeContext}
	m := mouseTestModel(t, files, map[string][]diff.DiffLine{"README.md": lines})
	m.file.lines = lines
	m.file.singleFile = true
	m.file.mdTOC = sidepane.ParseTOC(lines, "README.md")
	require.NotNil(t, m.file.mdTOC)
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0
	m.layout.viewport.SetContent(m.renderDiff())

	// shift+wheel-down advances the viewport by half a page (15 rows);
	// cursor at line 0 is now off the top and pins to line 15, past the
	// "## Second" header. TOC active section must follow.
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 5, true))
	model := result.(Model)

	require.GreaterOrEqual(t, model.nav.diffCursor, 10, "wheel-down must scroll past the second section header")
	model.file.mdTOC.SyncCursorToActiveSection()
	idx, ok := model.file.mdTOC.CurrentLineIdx()
	assert.True(t, ok, "TOC active section must be set after wheel-driven cursor pin")
	assert.GreaterOrEqual(t, idx, 10, "TOC active section must match diff cursor region after wheel")
}

func TestModel_HandleMouse_ClickInDiffSyncsTOCActiveSection(t *testing.T) {
	// same guarantee as the wheel case, for click-to-set-cursor.
	files := []string{"README.md"}
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "# First", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Second", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "### Third", ChangeType: diff.ChangeContext},
		{NewNum: 6, Content: "body", ChangeType: diff.ChangeContext},
	}
	m := mouseTestModel(t, files, map[string][]diff.DiffLine{"README.md": lines})
	m.file.lines = lines
	m.file.singleFile = true
	m.file.mdTOC = sidepane.ParseTOC(lines, "README.md")
	require.NotNil(t, m.file.mdTOC)
	m.layout.focus = paneTree
	m.nav.diffCursor = 0

	// click at y=6 with diffTopRow=2, YOffset=0 → row 4 (### Third)
	result, _ := m.Update(leftPressAt(60, 6))
	model := result.(Model)

	assert.Equal(t, paneDiff, model.layout.focus)
	assert.Equal(t, 4, model.nav.diffCursor, "click must land on diff line 4 (### Third)")
	model.file.mdTOC.SyncCursorToActiveSection()
	idx, ok := model.file.mdTOC.CurrentLineIdx()
	assert.True(t, ok, "TOC active section must be set after click in diff")
	assert.Equal(t, 4, idx, "TOC active section must match clicked diff line (Third section at lineIdx=4)")
}
