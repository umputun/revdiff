package ui

import (
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

	// pointer in diff pane (x=60 is past tree columns 0..37)
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Equal(t, wheelStep, model.nav.diffCursor, "wheel-down should advance cursor by wheelStep")

	// wheel-up returns cursor to top
	result, _ = model.Update(wheelMsg(tea.MouseButtonWheelUp, 60, 10, false))
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "wheel-up should move cursor back up by wheelStep")
}

func TestModel_HandleMouse_ShiftWheelHalfPage(t *testing.T) {
	lines := make([]diff.DiffLine, 100)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.file.lines = lines
	m.layout.viewport = viewport.New(80, 20) // Height=20 so half-page = 10

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, true))
	model := result.(Model)
	assert.Equal(t, 10, model.nav.diffCursor, "shift+wheel should move cursor by viewport.Height/2")
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

	// wheel must not close the overlay nor move cursor
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.True(t, model.overlay.Active(), "overlay must stay open after wheel event")
	assert.Equal(t, 0, model.nav.diffCursor)

	// click must also be swallowed
	result, _ = m.Update(leftPressAt(60, 12))
	model = result.(Model)
	assert.True(t, model.overlay.Active(), "overlay must stay open after click")
	assert.Equal(t, 0, model.nav.diffCursor)
}

func TestModel_HandleMouse_ClearsTransientHints(t *testing.T) {
	m := mouseTestModel(t, []string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {{NewNum: 1, Content: "a", ChangeType: diff.ChangeContext}},
	})
	m.commits.hint = "loading commits..."
	m.reload.hint = "press y to reload"
	m.compact.hint = "not applicable"

	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 10, false))
	model := result.(Model)
	assert.Empty(t, model.commits.hint)
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
	// moving the diff cursor via wheel must keep mdTOC.activeSection aligned
	// with the new cursor position, otherwise switching focus back to the TOC
	// highlights the stale section.
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
	m.layout.focus = paneDiff
	m.nav.diffCursor = 0

	// wheel-down in diff pane (x=60 lands past treeWidth, y=5 > diffTopRow)
	result, _ := m.Update(wheelMsg(tea.MouseButtonWheelDown, 60, 5, false))
	model := result.(Model)

	// after wheel, diffCursor advanced; TOC active-section must reflect it
	require.Positive(t, model.nav.diffCursor, "wheel-down must advance diffCursor")
	model.file.mdTOC.SyncCursorToActiveSection()
	idx, ok := model.file.mdTOC.CurrentLineIdx()
	assert.True(t, ok, "TOC active section must be set after wheel in diff")
	// cursor moved past line 2 (## Second at lineIdx=2); active section must be Second or Third
	assert.GreaterOrEqual(t, idx, 2, "TOC active section must match diff cursor region after wheel")
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
