package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

// wheelStep is the number of lines one wheel notch scrolls by. Shift+wheel
// uses half the viewport height instead. Shared with overlay.WheelStep so
// overlay popup scroll feels the same as diff-pane scroll.
const wheelStep = overlay.WheelStep

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
	// viewport beneath. hints are preserved here so the modal prompt (e.g.
	// reload's "press y to confirm") stays visible while the event is
	// discarded. in the keyboard path the prompt is also replaced by a new
	// hint from handlePendingReload, but mouse events don't transition the
	// modal, so dropping the hint would leave an invisible modal.
	if m.inConfirmDiscard || m.reload.pending || m.annot.annotating || m.search.active {
		return m, nil
	}
	if m.overlay.Active() {
		return m.handleOverlayMouse(msg)
	}

	// transient hints persist for exactly one render cycle; any mouse event
	// that reaches this point dismisses the last hint, mirroring handleKey.
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

// handleOverlayMouse routes a mouse event to the active overlay. wheel events
// drive the overlay's own scroll/cursor navigation; clicks and other buttons
// are consumed so they don't leak through to the panes underneath. outcomes
// that need model-side side effects (annotation jump, theme preview/confirm)
// are dispatched through the same helpers as the keyboard path. Canceled and
// Closed branches mirror the keyboard dispatch for symmetry but the current
// overlay mouse handlers never emit them — a mouse click either confirms
// (themeselect) or is a no-op.
func (m Model) handleOverlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	out := m.overlay.HandleMouse(msg)
	switch out.Kind {
	case overlay.OutcomeAnnotationChosen:
		return m.jumpToAnnotationTarget(out.AnnotationTarget)
	case overlay.OutcomeThemePreview:
		m.previewThemeByName(out.ThemeChoice.Name)
	case overlay.OutcomeThemeConfirmed:
		m.confirmThemeByName(out.ThemeChoice.Name)
	case overlay.OutcomeThemeCanceled:
		m.cancelThemeSelect()
	case overlay.OutcomeClosed, overlay.OutcomeNone:
	}
	return m, nil
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
//
// diff-pane wheel scrolls the viewport only; the diff cursor stays on its
// current logical line unless the line is scrolled out of view, in which
// case the cursor is pinned to the topmost or bottommost visible line so
// the highlight stays on screen. this matches less/vim mouse behavior and
// keeps the cursor from being yanked along with the wheel.
func (m Model) handleWheel(zone hitZone, delta int) (tea.Model, tea.Cmd) {
	switch zone {
	case hitDiff:
		m.scrollDiffViewportBy(delta)
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

// scrollDiffViewportBy shifts the diff viewport's YOffset by delta and pins
// the diff cursor to the visible range when the cursor would otherwise leave
// the viewport. delta > 0 scrolls down, delta < 0 scrolls up. no-op when no
// file is loaded or the clamped target equals the current offset.
// the content is re-rendered only when the cursor moves — a pure viewport
// shift does not need a re-render since the rendered content is unchanged.
func (m *Model) scrollDiffViewportBy(delta int) {
	if m.file.name == "" {
		return
	}
	maxOffset := max(0, m.layout.viewport.TotalLineCount()-m.layout.viewport.Height)
	current := m.layout.viewport.YOffset
	target := max(0, min(current+delta, maxOffset))
	if target == current {
		return
	}
	cursorMoved := m.pinDiffCursorTo(target)
	if cursorMoved {
		m.layout.viewport.SetContent(m.renderDiff())
	}
	m.layout.viewport.SetYOffset(target)
}

// pinDiffCursorTo moves the diff cursor onto the visible viewport range when it
// would otherwise be off-screen at newOffset; when the cursor marker is already
// within the viewport, it is left alone. when the cursor sits above the viewport,
// it is pinned to the topmost visible row; when below, to the bottommost visible
// row. returns true when the cursor actually changed position (idx or annotation
// flag), so callers know whether a re-render is required.
//
// the in-view check uses cursorTop (first visual row, where the cursor marker is
// rendered) rather than cursorBottom. in wrap mode a diff line spans multiple rows;
// using cursorBottom would incorrectly treat the cursor as visible when only its
// tail wrap-continuation rows are in the viewport while the marker row is above it.
//
// for the top-pin case, if the cursor line straddles the viewport top boundary
// (cursorTop < viewTop <= cursorBottom) the target row is advanced past the
// cursor line's last visual row so that the next line — whose marker is inside
// the viewport — is selected. when the entire wrapped span exceeds the viewport
// height the advance is clamped to viewBottom and the function returns false
// (no visible alternative exists).
func (m *Model) pinDiffCursorTo(newOffset int) bool {
	if len(m.file.lines) == 0 {
		return false
	}
	cursorTop, cursorBottom := m.cursorVisualRange()
	viewTop := newOffset
	viewBottom := newOffset + m.layout.viewport.Height - 1
	if cursorTop >= viewTop && cursorTop <= viewBottom {
		return false // cursor marker already visible
	}
	targetRow := viewBottom
	if cursorTop < viewTop {
		// cursor is above the viewport; in wrap mode the cursor line may have
		// continuation rows visible at viewTop — advance past them.
		if cursorBottom >= viewTop {
			targetRow = min(cursorBottom+1, viewBottom)
		} else {
			targetRow = viewTop
		}
	}
	idx, onAnnot := m.visualRowToDiffLine(targetRow)
	if idx == m.nav.diffCursor && onAnnot == m.annot.cursorOnAnnotation {
		return false
	}
	m.nav.diffCursor = idx
	m.annot.cursorOnAnnotation = onAnnot
	return true
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
