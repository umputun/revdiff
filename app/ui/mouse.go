package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/ui/sidepane"
)

// wheelStep is the number of lines one wheel notch scrolls by. matches the
// typical terminal feel (3 lines per notch). Shift+wheel uses half the
// viewport height instead.
const wheelStep = 3

// hitZone identifies which interactive area a mouse event targets.
type hitZone int

const (
	hitNone   hitZone = iota // outside any interactive area (borders, gaps, out-of-bounds)
	hitTree                  // tree pane (or TOC pane when mdTOC is active)
	hitDiff                  // diff pane body (below the diff header)
	hitStatus                // status bar row(s)
	hitHeader                // diff header row (file path) — currently a no-op zone
)

// statusBarHeight returns the number of rows occupied by the status bar.
// 0 when the status bar is hidden, otherwise 1.
func (m Model) statusBarHeight() int {
	if m.cfg.noStatusBar {
		return 0
	}
	return 1
}

// diffTopRow returns the first screen row (0-based y) of diff viewport content.
// accounts for the pane top border (row 0) and the diff header row (row 1),
// so the viewport always starts at row 2 regardless of whether the tree pane
// is visible.
func (m Model) diffTopRow() int {
	return 2
}

// treeTopRow returns the first screen row (0-based y) of tree pane content.
// accounts for the pane top border only — unlike diff, the tree pane has no
// internal header row, so content starts at row 1.
func (m Model) treeTopRow() int {
	return 1
}

// hitTest classifies a screen coordinate into a hitZone for mouse-event routing.
// the classification is pure arithmetic over m.layout state and does not
// inspect any dynamic UI content. ordering matters: status bar is checked
// first (y at bottom), then x is used to split tree vs diff columns, and
// finally y is used within each column to reject the diff header row or tree
// top border.
func (m Model) hitTest(x, y int) hitZone {
	if x < 0 || y < 0 || x >= m.layout.width || y >= m.layout.height {
		return hitNone
	}
	if sbh := m.statusBarHeight(); sbh > 0 && y >= m.layout.height-sbh {
		return hitStatus
	}
	// pane bottom border row sits just above the status bar (or the last
	// row when the status bar is hidden). clicks on the border must not
	// map into the viewport — without this guard, clickDiff would compute
	// a row one past the visible content.
	if y == m.layout.height-m.statusBarHeight()-1 {
		return hitNone
	}

	// tree block spans columns [0, treeWidth+1] when visible: left border +
	// treeWidth content columns + right border = treeWidth+2 columns total.
	// diff block picks up at column treeWidth+2.
	if !m.treePaneHidden() && x < m.layout.treeWidth+2 {
		if y < m.treeTopRow() {
			return hitNone
		}
		return hitTree
	}

	if y == 0 {
		return hitNone // diff pane top border — mirror of treeTopRow() guard above
	}
	if y < m.diffTopRow() {
		return hitHeader
	}
	return hitDiff
}

// handleMouse routes a tea.MouseMsg through the modal-state checks and into
// per-button dispatch. mouse events are only generated when
// tea.WithMouseCellMotion is enabled (i.e. --no-mouse is off), so this
// handler never runs in the opted-out path. wheel routing is by pointer
// position, not by current focus — this matches terminal conventions where
// scrolling follows the cursor.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// swallow during modal states — input belongs to the modal, not the
	// viewport beneath. overlay must also swallow so wheel does not scroll
	// through a popup. hints are preserved here so the modal prompt (e.g.
	// reload's "press y to confirm") stays visible while the event is
	// discarded. in the keyboard path the prompt is also replaced by a new
	// hint from handlePendingReload, but mouse events don't transition the
	// modal, so dropping the hint would leave an invisible modal.
	if m.inConfirmDiscard || m.reload.pending || m.annot.annotating || m.search.active {
		return m, nil
	}
	if m.overlay.Active() {
		return m, nil
	}

	// transient hints persist for exactly one render cycle; any mouse event
	// that reaches this point dismisses the last hint, mirroring handleKey.
	m.commits.hint = ""
	m.reload.hint = ""
	m.compact.hint = ""

	zone := m.hitTest(msg.X, msg.Y)

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if msg.Action != tea.MouseActionPress {
			return m, nil // guard against non-press wheel emissions for symmetry with left-click
		}
		return m.handleWheel(zone, -m.wheelStepFor(zone, msg.Shift))
	case tea.MouseButtonWheelDown:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		return m.handleWheel(zone, m.wheelStepFor(zone, msg.Shift))
	case tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight:
		// horizontal wheel is intentionally swallowed — horizontal scroll
		// stays keyboard-driven so users keep a single mental model.
		return m, nil
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil // ignore release and motion while holding
		}
		switch zone {
		case hitTree:
			return m.clickTree(msg.Y)
		case hitDiff:
			return m.clickDiff(msg.Y)
		case hitNone, hitStatus, hitHeader:
			return m, nil
		}
		return m, nil
	default:
		// right, middle, back, forward, none — no-op for this pass.
		return m, nil
	}
}

// wheelStepFor returns the wheel scroll step. Plain wheel scrolls by the
// wheelStep constant in every zone. Shift+wheel scrolls by half the pane
// under the pointer — viewport half for the diff, treePageSize half for
// the tree/TOC — to match the keyboard half-page shortcuts for that pane.
func (m Model) wheelStepFor(zone hitZone, shift bool) int {
	if !shift {
		return wheelStep
	}
	if zone == hitTree {
		return max(1, m.treePageSize()/2)
	}
	return max(1, m.layout.viewport.Height/2)
}

// handleWheel routes a vertical wheel event to the pane under the pointer.
// delta is positive for wheel-down, negative for wheel-up. the pane is
// selected by the hit zone, not the current pane focus: users expect the
// wheel to act on whichever pane the pointer is over.
func (m Model) handleWheel(zone hitZone, delta int) (tea.Model, tea.Cmd) {
	switch zone {
	case hitDiff:
		switch {
		case delta > 0:
			m.moveDiffCursorDownBy(delta)
		case delta < 0:
			m.moveDiffCursorUpBy(-delta)
		}
		m.syncTOCActiveSection()
	case hitTree:
		motion := sidepane.MotionPageDown
		step := delta
		if delta < 0 {
			motion = sidepane.MotionPageUp
			step = -delta
		}
		if m.file.mdTOC != nil {
			m.file.mdTOC.Move(motion, step)
			m.file.mdTOC.EnsureVisible(m.treePageSize())
			m.syncDiffToTOCCursor()
			return m, nil
		}
		m.tree.Move(motion, step)
		m.pendingAnnotJump = nil
		m.nav.pendingHunkJump = nil
		return m.loadSelectedIfChanged()
	case hitNone, hitStatus, hitHeader:
		// no-op zones — wheel outside the interactive panes is ignored.
	}
	return m, nil
}

// clickTree handles a left-click press in the tree (or TOC) pane. the click
// both focuses the pane and selects the entry under the pointer — same as
// pressing j/k to land on the entry. when the entry is a file, the diff
// load is triggered via loadSelectedIfChanged; on a directory row or an
// out-of-range row the click just moves the cursor with no load (mirrors
// j-landing semantics).
func (m Model) clickTree(y int) (tea.Model, tea.Cmd) {
	row := y - m.treeTopRow()
	m.layout.focus = paneTree
	if m.file.mdTOC != nil {
		if !m.file.mdTOC.SelectByVisibleRow(row) {
			return m, nil
		}
		m.file.mdTOC.EnsureVisible(m.treePageSize())
		m.syncDiffToTOCCursor()
		return m, nil
	}
	if !m.tree.SelectByVisibleRow(row) {
		return m, nil
	}
	m.pendingAnnotJump = nil
	m.nav.pendingHunkJump = nil
	return m.loadSelectedIfChanged()
}

// clickDiff handles a left-click press in the diff viewport. the click
// focuses the diff pane and moves the diff cursor to the logical line
// under the pointer. when the click lands on an injected annotation
// sub-row, cursorOnAnnotation is set so subsequent navigation treats the
// cursor as being on the annotation rather than the diff line above it.
func (m Model) clickDiff(y int) (tea.Model, tea.Cmd) {
	if m.file.name == "" {
		return m, nil // no file loaded — nothing to focus or point at
	}
	row := (y - m.diffTopRow()) + m.layout.viewport.YOffset
	idx, onAnnot := m.visualRowToDiffLine(row)
	m.layout.focus = paneDiff
	m.nav.diffCursor = idx
	m.annot.cursorOnAnnotation = onAnnot
	m.syncViewportToCursor()
	m.syncTOCActiveSection()
	return m, nil
}
