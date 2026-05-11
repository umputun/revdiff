package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

// wheelStep is the number of lines one wheel notch scrolls by. Shift+wheel
// uses half the viewport height instead. Shared with overlay.WheelStep so
// overlay popup scroll feels the same as diff-pane scroll.
const wheelStep = overlay.WheelStep

// wheelRenderDelay is the idle window after the last diff-pane wheel event
// before the deferred cursor pin + SetContent(renderDiff()) runs. Each wheel
// event updates the viewport YOffset synchronously and defers both the cursor
// pin and the diff render to a single tea.Tick so a burst of wheel events
// produces one pin + render at burst-end instead of per-event work. Short
// enough that the cursor highlight reappears quickly after the user stops
// scrolling, long enough to coalesce a typical trackpad flick into one render.
const wheelRenderDelay = 30 * time.Millisecond

// wheelState tracks the coalescing state for diff-pane wheel events.
//
// gen is bumped on every wheel event that actually shifts YOffset (i.e.
// scrollDiffViewportBy returned true — at-edge no-ops do NOT bump). The
// in-flight tea.Tick captures the gen at scheduling time so the resulting
// debounce msg can tell whether the burst has advanced past it.
//
// renderPending stays true while a render is owed (set by wheel events,
// cleared by flushWheelPending).
//
// tickInFlight gates tea.Tick scheduling: only ONE tick is alive at a time,
// regardless of how many wheel events arrive. The first wheel of a burst
// schedules a tick; subsequent wheels in the same burst just bump gen. If the
// tick fires with stale gen (burst still going), handleWheelDebounce
// reschedules a new tick for the current gen. This keeps the debounce-msg
// count proportional to burst duration (one per wheelRenderDelay), not to
// wheel event count — drops thousands of redundant Update+View cycles on
// trackpad/free-spin bursts and stops the wheel-up from queueing behind them.
type wheelState struct {
	gen           int
	renderPending bool
	tickInFlight  bool
}

// wheelDebounceMsg is the deferred flush trigger for diff-pane wheel events.
// gen captures the wheel generation at scheduling time so handleWheelDebounce
// can distinguish a settled burst (gen matches → flush) from an in-progress
// burst (gen advanced → reschedule a fresh tick for the current gen).
type wheelDebounceMsg struct {
	gen int
}

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
		return m.handleWheel(zone, -m.wheelStepFor(msg.Shift))
	case tea.MouseButtonWheelDown:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		return m.handleWheel(zone, m.wheelStepFor(msg.Shift))
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

// wheelStepFor returns the wheel scroll step for the diff pane. Plain wheel
// scrolls by the wheelStep constant; Shift+wheel scrolls by half the
// viewport height to match the keyboard half-page shortcut. The tree/TOC
// path in handleWheel ignores the magnitude and uses single-step cursor
// navigation regardless, so this only governs the diff-pane delta.
func (m Model) wheelStepFor(shift bool) int {
	if !shift {
		return wheelStep
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
//
// when the diff pane scrolls, both the cursor pin and the SetContent
// (renderDiff()) call are deferred via a tea.Tick debounce — see
// wheelRenderDelay. the only per-event work is SetYOffset (and the modal/zone
// checks that were already free), so each event is O(1). this coalesces a
// burst of wheel events (a trackpad flick or free-spin wheel) into a single
// pin+render at burst-end, so an opposite-direction wheel event isn't blocked
// behind a backlog of expensive per-event operations on large diffs.
// fixes #179.
func (m Model) handleWheel(zone hitZone, delta int) (tea.Model, tea.Cmd) {
	switch zone {
	case hitDiff:
		if !m.scrollDiffViewportBy(delta) {
			return m, nil
		}
		m.wheel.renderPending = true
		m.wheel.gen++
		gen := m.wheel.gen

		// schedule a tick only when none is in flight; subsequent wheels in the
		// same burst just bump gen. an in-flight tick fires at its own
		// wallclock deadline and reschedules itself if it lands on a stale gen
		// (see handleWheelDebounce). this keeps message count proportional to
		// burst duration, not wheel event count — without this guard each
		// wheel event spawns a tea.Tick goroutine and a debounce Msg, every
		// one of which forces another Update + View cycle (~2ms each).
		if m.wheel.tickInFlight {
			return m, nil
		}
		m.wheel.tickInFlight = true
		return m, tea.Tick(wheelRenderDelay, func(time.Time) tea.Msg {
			return wheelDebounceMsg{gen: gen}
		})
	case hitTree:
		// tree/TOC wheel = direct cursor navigation, one entry per notch.
		// no debounce, no shift-half-page tricks (those are diff-pane things);
		// the tree is small and cheap so single-step matches j/k semantics.
		motion := sidepane.MotionDown
		if delta < 0 {
			motion = sidepane.MotionUp
		}
		if m.file.mdTOC != nil {
			m.file.mdTOC.Move(motion)
			m.file.mdTOC.EnsureVisible(m.treePageSize())
			m.syncDiffToTOCCursor()
			return m, nil
		}
		m.tree.Move(motion)
		m.pendingAnnotJump = nil
		m.nav.pendingHunkJump = nil
		return m.loadSelectedIfChanged()
	case hitNone, hitStatus, hitHeader:
		// no-op zones — wheel outside the interactive panes is ignored.
	}
	return m, nil
}

// handleWheelDebounce processes a deferred tick for a wheel burst. branches:
//   - renderPending is false: an external path (handleKey, handleResize,
//     handleBlameLoaded) already flushed; clear tickInFlight, no-op.
//   - msg.gen lags m.wheel.gen: the burst is still going (new wheels bumped
//     gen since this tick was scheduled). reschedule a fresh tick for the
//     current gen and keep tickInFlight=true so the next wheel doesn't double-
//     schedule.
//   - msg.gen matches m.wheel.gen: the burst has been idle for at least
//     wheelRenderDelay. flush the deferred work (pin + diff render) and
//     clear tickInFlight so the next burst's first wheel schedules fresh.
func (m Model) handleWheelDebounce(msg wheelDebounceMsg) (tea.Model, tea.Cmd) {
	// renderPending cleared by some other path (handleKey flushed, resize
	// flushed, etc.): the in-flight tick is done, no more rescheduling.
	if !m.wheel.renderPending {
		m.wheel.tickInFlight = false
		return m, nil
	}
	// gen has advanced past msg.gen: the burst is still going. reschedule a
	// new tick for the current gen and stay tickInFlight=true. without this
	// reschedule the burst would never flush after the original tick fires
	// stale.
	if msg.gen != m.wheel.gen {
		curGen := m.wheel.gen
		return m, tea.Tick(wheelRenderDelay, func(time.Time) tea.Msg {
			return wheelDebounceMsg{gen: curGen}
		})
	}
	// gen matches: burst has been idle for wheelRenderDelay. flushWheelPending
	// clears both renderPending and tickInFlight, so the next burst's first
	// wheel reschedules a fresh tick.
	m.flushWheelPending()
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

// scrollDiffViewportBy shifts the diff viewport's YOffset by delta. delta > 0
// scrolls down, delta < 0 scrolls up. returns true when YOffset changed (the
// caller should schedule a deferred cursor pin + render); returns false when
// no file is loaded or the clamped target equals the current offset.
//
// the expensive cursor pin (pinDiffCursorTo loops O(cursor_idx) via
// cursorVisualRange) and the full-diff render (SetContent(renderDiff()))
// are both deferred to handleWheelDebounce so each wheel event is O(1).
// during a wheel burst the cursor index in m.nav.diffCursor stays stale and
// the rendered string still highlights the old cursor row, matching less/vim
// behavior; the deferred work runs at burst-end (wheelRenderDelay of wheel
// idle) or when handleKey, handleResize, or handleBlameLoaded flushes early.
func (m *Model) scrollDiffViewportBy(delta int) bool {
	if m.file.name == "" {
		return false
	}
	maxOffset := max(0, m.layout.viewport.TotalLineCount()-m.layout.viewport.Height)
	current := m.layout.viewport.YOffset
	target := max(0, min(current+delta, maxOffset))
	if target == current {
		return false
	}
	m.layout.viewport.SetYOffset(target)
	return true
}

// flushWheelPending applies the deferred cursor pin + diff re-render owed by
// an in-flight wheel burst, then clears renderPending AND tickInFlight.
// callers invoke this before any action that reads m.nav.diffCursor or
// relies on a fresh diff content string. Production call sites:
//   - handleKey (model.go) — cursor-relative key actions (j/k, search,
//     annotate) must see the pinned cursor before they read m.nav.diffCursor
//   - handleResize (model.go) — syncViewportToCursor must anchor at the
//     wheeled-to position, not the pre-burst cursor
//   - handleBlameLoaded (loaders.go) — same syncViewportToCursor rationale
//   - handleWheelDebounce (mouse.go) — the matching-gen tick path
//
// no-op when no render is pending. when pinDiffCursorTo returns false (the
// cursor stayed in view through the burst, no pin needed), syncTOCActiveSection
// and SetContent are skipped — the existing rendered string already has the
// correct cursor highlight and the TOC active section is keyed off the
// (unchanged) cursor index, so a re-render would be redundant. Only the
// state flags are always cleared.
//
// clearing tickInFlight here means a new wheel event arriving after an
// external flush can schedule a fresh tick immediately (rather than waiting
// for the previously-scheduled tick to drain through the !renderPending
// branch of handleWheelDebounce); any already-scheduled tick that fires
// after the flush hits the !renderPending or stale-gen branches and is
// harmless.
func (m *Model) flushWheelPending() {
	if !m.wheel.renderPending {
		return
	}
	if m.pinDiffCursorTo(m.layout.viewport.YOffset) {
		m.syncTOCActiveSection()
		m.layout.viewport.SetContent(m.renderDiff())
	}
	m.wheel.renderPending = false
	m.wheel.tickInFlight = false
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
