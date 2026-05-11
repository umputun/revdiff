package ui

import (
	"fmt"
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

// updateWheelAndFlush dispatches a wheel event through Update, then dispatches
// the resulting wheelDebounceMsg (if any) so the test sees the final post-flush
// state. mirrors what bubbletea does after wheelRenderDelay idle: the latest
// debounce tick fires, the cursor pins and the diff re-renders. tests that
// want to observe burst-time state (pre-flush) should call Update directly
// instead.
//
// fails loudly when the wheel handler returns a cmd that is NOT a
// wheelDebounceMsg producer — a future refactor (e.g. tea.Batch) would
// otherwise silently degrade this helper into a plain Update and produce
// misleading "post-flush" assertions on pre-flush state.
func updateWheelAndFlush(t *testing.T, m Model, msg tea.MouseMsg) Model {
	t.Helper()
	result, cmd := m.Update(msg)
	model := result.(Model)
	if cmd == nil {
		return model
	}
	produced := cmd()
	debounceMsg, ok := produced.(wheelDebounceMsg)
	if !ok {
		t.Fatalf("updateWheelAndFlush: wheel handler returned non-wheelDebounceMsg cmd: %T", produced)
	}
	result, _ = model.Update(debounceMsg)
	return result.(Model)
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
	// is now above the visible range and is pinned to the new top after the
	// debounce flush (the pin is deferred so the per-event path stays O(1)).
	model := updateWheelAndFlush(t, m, wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	assert.Equal(t, wheelStep, model.layout.viewport.YOffset, "wheel-down must scroll viewport by wheelStep")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "cursor must pin to top of visible range when scrolled off-screen")

	// wheel-up scrolls the viewport back; cursor at line 3 stays in view, so it does not move.
	model = updateWheelAndFlush(t, model, wheelMsg(tea.MouseButtonWheelUp, 60, 10, false))
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
	model := updateWheelAndFlush(t, m, wheelMsg(tea.MouseButtonWheelUp, 60, 10, false))
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

	model := updateWheelAndFlush(t, m, wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
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

	model := updateWheelAndFlush(t, m, wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
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

	model := updateWheelAndFlush(t, m, wheelMsg(tea.MouseButtonWheelDown, 60, 10, true))
	assert.Equal(t, 10, model.layout.viewport.YOffset, "shift+wheel must scroll viewport by half page")
	assert.Equal(t, 10, model.nav.diffCursor, "cursor must pin to top of visible range after half-page scroll")
}

func TestModel_HandleMouse_WheelInTreeMovesTreeCursor(t *testing.T) {
	// covers the contract introduced for the tree wheel: one entry per notch,
	// direction follows wheel sign, magnitude ignored (so shift+wheel still
	// moves only one entry), and boundaries clamp. exact-match assertions
	// — not NotEqual — so a regression to multi-step (the old PageDown(3)
	// behavior) would fail this test.
	files := make([]string, 30)
	diffs := make(map[string][]diff.DiffLine, 30)
	for i := range files {
		p := string(rune('a'+(i/10))) + string(rune('a'+(i%10))) + ".go"
		files[i] = p
		diffs[p] = []diff.DiffLine{{NewNum: 1, Content: p, ChangeType: diff.ChangeContext}}
	}

	t.Run("plain wheel-down advances exactly one entry", func(t *testing.T) {
		m := mouseTestModel(t, files, diffs)
		m.layout.focus = paneDiff // wheel routing is by hit zone, not focus
		require.Equal(t, "aa.go", m.tree.SelectedFile())

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
		model := result.(Model)
		assert.Equal(t, "ab.go", model.tree.SelectedFile(), "single wheel-down notch must advance tree cursor by exactly one entry")
	})

	t.Run("plain wheel-up retreats exactly one entry", func(t *testing.T) {
		m := mouseTestModel(t, files, diffs)
		m.tree.SelectByPath("ac.go") // cursor at entry 2
		require.Equal(t, "ac.go", m.tree.SelectedFile())

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelUp, 5, 3, false))
		model := result.(Model)
		assert.Equal(t, "ab.go", model.tree.SelectedFile(), "single wheel-up notch must retreat tree cursor by exactly one entry")
	})

	t.Run("shift+wheel in tree still single-step", func(t *testing.T) {
		// shift+wheel in tree must NOT do half-page — the magnitude returned by
		// wheelStepFor is discarded in the hitTree branch, so shift has no
		// effect on the tree step size.
		m := mouseTestModel(t, files, diffs)
		require.Equal(t, "aa.go", m.tree.SelectedFile())

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, true))
		model := result.(Model)
		assert.Equal(t, "ab.go", model.tree.SelectedFile(), "shift+wheel in tree must still move exactly one entry")
	})

	t.Run("wheel-up at top is no-op", func(t *testing.T) {
		m := mouseTestModel(t, files, diffs)
		require.Equal(t, "aa.go", m.tree.SelectedFile())

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelUp, 5, 3, false))
		model := result.(Model)
		assert.Equal(t, "aa.go", model.tree.SelectedFile(), "wheel-up at first entry must clamp, not wrap")
	})

	t.Run("wheel-down at bottom is no-op", func(t *testing.T) {
		m := mouseTestModel(t, files, diffs)
		m.tree.SelectByPath(files[len(files)-1])
		require.Equal(t, files[len(files)-1], m.tree.SelectedFile())

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
		model := result.(Model)
		assert.Equal(t, files[len(files)-1], model.tree.SelectedFile(), "wheel-down at last entry must clamp, not wrap")
	})

	t.Run("tree wheel does not touch diff wheelState", func(t *testing.T) {
		// negative invariant: the hitTree branch is independent of the
		// diff-pane debounce state. a regression that accidentally bumped
		// wheel.gen or set wheel.renderPending in the tree path would
		// silently extend a later diff-pane debounce.
		m := mouseTestModel(t, files, diffs)
		require.Equal(t, 0, m.wheel.gen)
		require.False(t, m.wheel.renderPending)
		require.False(t, m.wheel.tickInFlight)

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
		model := result.(Model)
		assert.Equal(t, 0, model.wheel.gen, "tree wheel must not bump diff wheel gen")
		assert.False(t, model.wheel.renderPending, "tree wheel must not set diff renderPending")
		assert.False(t, model.wheel.tickInFlight, "tree wheel must not schedule a diff debounce tick")
	})
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

func TestModel_HandleMouse_ShiftWheelInDiffUsesHalfPage(t *testing.T) {
	// shift+wheel in the diff pane must step by viewport.Height/2 to match
	// the keyboard half-page shortcut. tree/TOC wheel paths ignore the
	// magnitude entirely (single-step cursor nav) so they are not exercised
	// here.
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "x", ChangeType: diff.ChangeContext}},
	})
	m.layout.viewport.Height = 20

	assert.Equal(t, max(1, m.layout.viewport.Height/2), m.wheelStepFor(true),
		"shift+wheel must step by viewport.Height/2")
	assert.Equal(t, wheelStep, m.wheelStepFor(false),
		"plain wheel must step by the wheelStep constant")
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
	// mirrors the file-tree wheel test contract for the TOC branch: one entry
	// per notch, direction follows wheel sign, magnitude ignored (shift still
	// single-step), boundaries clamp. uses a non-heading first line so the
	// first heading lands at lineIdx=1 — distinct from the synthetic filename
	// entry ParseTOC prepends at lineIdx=0 — letting a single notch change
	// CurrentLineIdx.
	files := []string{"README.md"}
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "intro paragraph", ChangeType: diff.ChangeContext}, // lineIdx=0, not a heading
		{NewNum: 2, Content: "# A", ChangeType: diff.ChangeContext},             // lineIdx=1, first heading
		{NewNum: 3, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "## B", ChangeType: diff.ChangeContext}, // lineIdx=3
		{NewNum: 5, Content: "body", ChangeType: diff.ChangeContext},
		{NewNum: 6, Content: "## C", ChangeType: diff.ChangeContext}, // lineIdx=5
	}

	// helper: build a fresh model with TOC ready.
	build := func(t *testing.T) Model {
		t.Helper()
		m := mouseTestModel(t, files, map[string][]diff.DiffLine{"README.md": lines})
		m.file.lines = lines
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(lines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		return m
	}

	t.Run("plain wheel-down advances one entry and syncs diff cursor", func(t *testing.T) {
		m := build(t)
		before, ok := m.file.mdTOC.CurrentLineIdx()
		require.True(t, ok, "TOC must have a valid cursor as a precondition")
		require.Equal(t, 0, before)

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
		model := result.(Model)
		after, ok := model.file.mdTOC.CurrentLineIdx()
		require.True(t, ok)
		assert.Equal(t, 1, after, "single wheel-down notch must advance TOC cursor by exactly one entry (to # A at lineIdx=1)")
		assert.Equal(t, 1, model.nav.diffCursor, "TOC wheel must sync diff cursor to the selected entry's lineIdx")
	})

	t.Run("plain wheel-up retreats one entry", func(t *testing.T) {
		m := build(t)
		// position TOC cursor on entry 2 (## B at lineIdx=3) via two wheel-downs.
		for range 2 {
			r, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
			m = r.(Model)
		}
		mid, ok := m.file.mdTOC.CurrentLineIdx()
		require.True(t, ok, "TOC cursor must remain valid after the two-step setup")
		require.Equal(t, 3, mid)

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelUp, 5, 3, false))
		model := result.(Model)
		after, ok := model.file.mdTOC.CurrentLineIdx()
		require.True(t, ok)
		assert.Equal(t, 1, after, "single wheel-up notch must retreat TOC cursor by exactly one entry")
		assert.Equal(t, 1, model.nav.diffCursor, "TOC wheel-up must sync diff cursor to the previous entry's lineIdx")
	})

	t.Run("shift+wheel in TOC still single-step", func(t *testing.T) {
		m := build(t)
		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, true))
		model := result.(Model)
		after, ok := model.file.mdTOC.CurrentLineIdx()
		require.True(t, ok)
		assert.Equal(t, 1, after, "shift+wheel in TOC must still move exactly one entry (magnitude discarded)")
	})

	t.Run("wheel-up at top entry is no-op", func(t *testing.T) {
		m := build(t)
		before, ok := m.file.mdTOC.CurrentLineIdx()
		require.True(t, ok)
		require.Equal(t, 0, before)

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelUp, 5, 3, false))
		model := result.(Model)
		after, ok := model.file.mdTOC.CurrentLineIdx()
		require.True(t, ok, "wheel-up at top must leave cursor in a valid position (not invalidated)")
		assert.Equal(t, 0, after, "wheel-up at first TOC entry must clamp, not wrap")
	})

	t.Run("wheel-down at last entry is no-op", func(t *testing.T) {
		m := build(t)
		// advance to the last entry by wheel-down notches; entries are
		// [README.md, # A, ## B, ## C] = 4 total, so 3 notches lands on # C.
		for range 3 {
			r, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
			m = r.(Model)
		}
		end, ok := m.file.mdTOC.CurrentLineIdx()
		require.True(t, ok)
		require.Equal(t, 5, end, "should now be on ## C at lineIdx=5")

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
		model := result.(Model)
		after, ok := model.file.mdTOC.CurrentLineIdx()
		require.True(t, ok, "wheel-down at bottom must leave cursor in a valid position (not invalidated)")
		assert.Equal(t, 5, after, "wheel-down at last TOC entry must clamp, not wrap")
	})

	t.Run("TOC wheel does not touch diff wheelState", func(t *testing.T) {
		m := build(t)
		require.Equal(t, 0, m.wheel.gen)
		require.False(t, m.wheel.renderPending)
		require.False(t, m.wheel.tickInFlight)

		result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 5, 3, false))
		model := result.(Model)
		assert.Equal(t, 0, model.wheel.gen, "TOC wheel must not bump diff wheel gen")
		assert.False(t, model.wheel.renderPending, "TOC wheel must not set diff renderPending")
		assert.False(t, model.wheel.tickInFlight, "TOC wheel must not schedule a diff debounce tick")
	})
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
	// "## Second" header. TOC active section must follow once the debounce
	// flushes the deferred pin + TOC sync.
	model := updateWheelAndFlush(t, m, wheelMsg(tea.MouseButtonWheelDown, 60, 5, true))

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

func TestModel_HandleMouse_WheelDeferredRender_SchedulesDebounceWhenYOffsetChanges(t *testing.T) {
	// when a wheel event shifts YOffset, both the cursor pin AND the render are
	// deferred to handleWheelDebounce. the wheel handler returns a tea.Tick
	// command carrying the current gen; the per-event path is O(1) so a burst
	// of events doesn't queue behind expensive per-event work.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())

	result, cmd := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)

	assert.Equal(t, wheelStep, model.layout.viewport.YOffset, "YOffset must advance synchronously")
	assert.Equal(t, 0, model.nav.diffCursor, "cursor pin is deferred — diffCursor stays at the pre-burst position")
	assert.True(t, model.wheel.renderPending, "render must be marked pending after YOffset shift")
	assert.Equal(t, 1, model.wheel.gen, "wheel gen must bump on each YOffset-shifting event")
	require.NotNil(t, cmd, "wheel that shifts YOffset must return a debounce command")

	msg := cmd()
	debounce, ok := msg.(wheelDebounceMsg)
	require.True(t, ok, "debounce command must produce wheelDebounceMsg, got %T", msg)
	assert.Equal(t, 1, debounce.gen, "debounce msg gen must match wheel gen at scheduling")
}

func TestModel_HandleMouse_WheelDeferredRender_NoDebounceWhenYOffsetCannotChange(t *testing.T) {
	// when YOffset cannot advance (already at max or content fits), the wheel
	// path returns early without scheduling a debounce — nothing to flush.
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

	result, cmd := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)

	assert.False(t, model.wheel.renderPending, "render must not be pending when YOffset cannot change")
	assert.Equal(t, 0, model.wheel.gen, "wheel gen must not bump when YOffset cannot change")
	assert.Nil(t, cmd, "wheel without YOffset change must not schedule a debounce")
}

func TestModel_HandleMouse_WheelDebounceMsg_RendersOnMatchingGen(t *testing.T) {
	// wheelDebounceMsg with matching gen and renderPending=true flushes the
	// deferred cursor pin AND diff render, then clears renderPending +
	// tickInFlight. all four side effects must be observable: state flags
	// clear, cursor pins, AND the viewport content reflects the post-pin
	// render (asserting against viewport.View() catches a regression where
	// SetContent(renderDiff()) is removed from flushWheelPending).
	// per-line unique content so a re-render is observable in the rendered
	// string (the cursor highlight uses default colors in tests, so a plain
	// "all lines look the same" fixture wouldn't catch a missing SetContent).
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: fmt.Sprintf("line %d", i), ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	// initial render with cursor at 0, YOffset=0 — establishes the baseline
	// content string the test asserts against later.
	m.layout.viewport.SetContent(m.renderDiff())
	preContent := m.layout.viewport.View()

	// simulate a wheel burst that scrolled YOffset past the cursor: cursor at 0
	// is now above the viewport top, pinDiffCursorTo will pin to row wheelStep.
	// YOffset shifts but viewport.SetContent is NOT called — this matches the
	// real wheel path that defers SetContent to the debounce flush.
	m.layout.viewport.SetYOffset(wheelStep)
	m.nav.diffCursor = 0
	m.wheel.gen = 5
	m.wheel.renderPending = true
	m.wheel.tickInFlight = true

	result, cmd := m.Update(wheelDebounceMsg{gen: 5})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.False(t, model.wheel.renderPending, "matching debounce msg must clear renderPending")
	assert.False(t, model.wheel.tickInFlight, "matching debounce msg must clear tickInFlight")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "matching debounce msg must pin cursor to viewport top")

	// the deferred SetContent(renderDiff()) ran during flushWheelPending, so
	// viewport.View() must now reflect the post-pin state — at minimum,
	// scrolled lines (YOffset+wheelStep) are visible where pre-burst lines
	// used to be. without the assertion, removing SetContent from
	// flushWheelPending would still pass the flag/cursor checks above.
	postContent := model.layout.viewport.View()
	assert.NotEqual(t, preContent, postContent, "matching debounce msg must re-render viewport content via SetContent(renderDiff())")
	assert.Contains(t, postContent, fmt.Sprintf("line %d", wheelStep), "post-flush viewport must show the wheelStep-offset content")
}

func TestModel_HandleMouse_WheelDebounceMsg_SkipsRenderWhenCursorStaysInView(t *testing.T) {
	// when the burst left the cursor inside the viewport (pinDiffCursorTo
	// returns false), the deferred flush must skip the expensive
	// SetContent(renderDiff()) — the existing viewport content already
	// has the correct cursor highlight. only the state flags clear.
	// without this gate, every wheel-burst-then-flush forces a redundant
	// full-diff render even on short bursts where nothing visually changed.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: fmt.Sprintf("line %d", i), ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())
	// cursor at row 20 sits comfortably inside the 30-row viewport after a
	// wheelStep-sized scroll (YOffset=3 → visible [3,32], cursor 20 still in view).
	m.layout.viewport.SetYOffset(wheelStep)
	m.nav.diffCursor = 20
	preCursor := m.nav.diffCursor
	preContent := m.layout.viewport.View()
	m.wheel.gen = 5
	m.wheel.renderPending = true
	m.wheel.tickInFlight = true

	result, cmd := m.Update(wheelDebounceMsg{gen: 5})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.False(t, model.wheel.renderPending, "matching debounce msg must clear renderPending even when no render runs")
	assert.False(t, model.wheel.tickInFlight, "matching debounce msg must clear tickInFlight even when no render runs")
	assert.Equal(t, preCursor, model.nav.diffCursor, "cursor must not move when already in view")
	assert.Equal(t, preContent, model.layout.viewport.View(), "no-pin flush must not re-render the diff")
}

func TestModel_HandleMouse_WheelDebounceMsg_StaleGenReschedules(t *testing.T) {
	// a debounce msg whose gen lags the current wheel gen represents an older
	// burst tick that arrived while the burst is still going. it must NOT
	// render or clear state, but it MUST reschedule a new tick for the
	// current gen — otherwise the burst would never get a flush after the
	// initial tick fires stale (only one tick is in flight at a time).
	m := mouseTestModel(t, []string{"a.go"}, nil)
	m.wheel.gen = 5
	m.wheel.renderPending = true

	result, cmd := m.Update(wheelDebounceMsg{gen: 3})
	model := result.(Model)

	require.NotNil(t, cmd, "stale debounce must reschedule a fresh tick for the current gen")
	assert.True(t, model.wheel.renderPending, "stale debounce msg must not clear renderPending")
	assert.Equal(t, 5, model.wheel.gen, "stale debounce msg must not change wheel gen")

	rescheduled, ok := cmd().(wheelDebounceMsg)
	require.True(t, ok, "rescheduled cmd must produce wheelDebounceMsg")
	assert.Equal(t, 5, rescheduled.gen, "rescheduled tick must target the current gen")
}

func TestModel_HandleMouse_WheelDebounceMsg_NoopWhenRenderNotPending(t *testing.T) {
	// defensive guard: handleWheelDebounce must degrade gracefully when it
	// finds renderPending=false but tickInFlight=true. In production this
	// combination is no longer reachable since flushWheelPending clears both
	// flags atomically, but the handler must still no-op cleanly and clear
	// tickInFlight if state ever lands in this combination (e.g. a future
	// path that touches one flag without the other).
	m := mouseTestModel(t, []string{"a.go"}, nil)
	m.wheel.gen = 2
	m.wheel.renderPending = false
	m.wheel.tickInFlight = true

	result, cmd := m.Update(wheelDebounceMsg{gen: 2})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.False(t, model.wheel.renderPending)
	assert.False(t, model.wheel.tickInFlight, "no-pending tick must clear tickInFlight defensively")
	assert.Equal(t, 2, model.wheel.gen)
}

func TestModel_HandleMouse_WheelBurst_OppositeDirectionProcessesImmediately(t *testing.T) {
	// the core fix for issue #179: a wheel-up event arriving during a wheel-down
	// burst must NOT wait for buffered events' renders to complete. each wheel
	// event shifts YOffset synchronously and defers cursor pin + renderDiff to
	// a single debounce tick; only the first wheel of a burst schedules a tea
	// Tick (subsequent wheels just bump gen) so per-event message count is
	// O(1) regardless of burst length.
	lines := make([]diff.DiffLine, 200)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())

	// rapid wheel-down burst — first event schedules a tick, subsequent events
	// just bump gen (no new tick) so the debounce-message count stays bounded.
	model := m
	for i := 1; i <= 5; i++ {
		result, cmd := model.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
		model = result.(Model)
		assert.Equal(t, i, model.wheel.gen, "wheel gen must bump on each YOffset-shifting event")
		assert.True(t, model.wheel.renderPending, "render must stay pending across the burst")
		if i == 1 {
			require.NotNil(t, cmd, "first wheel of a burst must schedule a debounce tick")
		} else {
			assert.Nil(t, cmd, "wheel #%d must not schedule a new tick while one is already in flight", i)
		}
	}
	require.Equal(t, 5*wheelStep, model.layout.viewport.YOffset, "five wheel-down events must scroll by 5*wheelStep")
	assert.True(t, model.wheel.tickInFlight, "tickInFlight must stay true while the burst is alive")

	// wheel-up arrives mid-burst (gen=1 tick is still in flight). the up event
	// shifts YOffset immediately and bumps gen to 6 — no new tick scheduled
	// because tickInFlight is still true. the in-flight gen=1 tick will fire
	// stale and reschedule itself for gen 6.
	result, cmd := model.Update(wheelMsg(tea.MouseButtonWheelUp, 60, 10, false))
	model = result.(Model)
	assert.Nil(t, cmd, "wheel-up mid-burst must not schedule a new tick — the existing one will reschedule itself")
	assert.Equal(t, 4*wheelStep, model.layout.viewport.YOffset, "wheel-up must shift YOffset by -wheelStep relative to last down")
	assert.Equal(t, 6, model.wheel.gen, "wheel-up shifting YOffset must bump gen past the down events")

	// the original gen=1 tick fires stale; handleWheelDebounce reschedules for
	// gen=6 (the current burst tip). pending state stays true.
	result, cmd = model.Update(wheelDebounceMsg{gen: 1})
	model = result.(Model)
	require.NotNil(t, cmd, "stale tick must reschedule a fresh tick targeting the current gen")
	assert.True(t, model.wheel.renderPending, "stale debounce must not clear renderPending")
	rescheduled, ok := cmd().(wheelDebounceMsg)
	require.True(t, ok)
	assert.Equal(t, 6, rescheduled.gen, "rescheduled tick must target current gen")

	// rescheduled gen=6 tick fires — burst has settled. flushes pin + render
	// and clears tickInFlight so the next burst's first wheel can schedule
	// fresh.
	result, _ = model.Update(wheelDebounceMsg{gen: 6})
	model = result.(Model)
	assert.False(t, model.wheel.renderPending, "matching debounce gen must flush the deferred render")
	assert.False(t, model.wheel.tickInFlight, "tickInFlight must clear once the flush completes")
}

func TestModel_HandleKey_FlushesPendingWheelBeforeAction(t *testing.T) {
	// handleKey calls flushWheelPending at the top so cursor-relative key
	// actions (j/k, save annotation, search) read a freshly pinned cursor
	// rather than the stale pre-burst value. without this flush the deferred
	// debounce would arrive after the key action, potentially yanking the
	// cursor back to a viewport edge.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())

	// simulate a wheel burst that left state deferred: YOffset advanced, cursor
	// stale at the pre-burst position (0), renderPending+tickInFlight true.
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	require.True(t, model.wheel.renderPending)
	require.True(t, model.wheel.tickInFlight)
	require.Equal(t, 0, model.nav.diffCursor, "cursor pin is deferred — stays at pre-burst position")

	// arbitrary keypress — even one with no resolved action — triggers the
	// flush at the top of handleKey. assertions don't depend on the key's
	// semantics, only on the flush happening before any handler runs.
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = result.(Model)

	assert.False(t, model.wheel.renderPending, "handleKey must flush pending wheel work before processing the key")
	assert.False(t, model.wheel.tickInFlight, "handleKey flush must also clear tickInFlight")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "cursor must be pinned to viewport top by the pre-key flush")
}

func TestModel_HandleResize_FlushesPendingWheelBeforeSync(t *testing.T) {
	// handleResize calls flushWheelPending before syncViewportToCursor so the
	// post-resize viewport anchors at the wheeled-to position rather than the
	// pre-burst cursor. without this flush syncViewportToCursor would scroll
	// back to where the cursor used to live and lose the wheel scroll.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport.SetContent(m.renderDiff())

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	require.True(t, model.wheel.renderPending)
	wheeledOffset := model.layout.viewport.YOffset
	require.Positive(t, wheeledOffset, "wheel must advance YOffset for the test to be meaningful")

	// a window resize at the same dimensions still routes through handleResize
	// and exercises the flush path; the values match the testModel defaults so
	// no layout state actually changes — only the flush effect is observable.
	result, _ = model.Update(tea.WindowSizeMsg{Width: model.layout.width, Height: model.layout.height})
	model = result.(Model)

	assert.False(t, model.wheel.renderPending, "handleResize must flush pending wheel work before syncViewportToCursor")
	assert.False(t, model.wheel.tickInFlight, "handleResize flush must also clear tickInFlight")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "cursor must be pinned to viewport top by the pre-resize flush")
}

func TestModel_HandleBlameLoaded_FlushesPendingWheelBeforeSync(t *testing.T) {
	// handleBlameLoaded calls flushWheelPending before syncViewportToCursor
	// for the same reason as handleResize — without the flush the stale
	// pre-burst cursor would anchor the post-blame viewport and snap the
	// scroll back, losing the user's wheel position.
	lines := make([]diff.DiffLine, 60)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.modes.showBlame = true
	m.layout.viewport.SetContent(m.renderDiff())

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	require.True(t, model.wheel.renderPending)
	require.Equal(t, 0, model.nav.diffCursor)

	result, _ = model.Update(blameLoadedMsg{
		file: "a.go",
		seq:  model.file.loadSeq,
		data: map[int]diff.BlameLine{1: {Author: "alice"}},
	})
	model = result.(Model)

	assert.False(t, model.wheel.renderPending, "handleBlameLoaded must flush pending wheel work before syncViewportToCursor")
	assert.False(t, model.wheel.tickInFlight, "handleBlameLoaded flush must also clear tickInFlight")
	assert.Equal(t, wheelStep, model.nav.diffCursor, "cursor must be pinned to viewport top by the pre-blame flush")
}
