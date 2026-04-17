package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

const scrollStep = 4 // horizontal scroll step in characters

// cursorDiffLine returns the DiffLine at the current cursor position, if valid.
func (m Model) cursorDiffLine() (diff.DiffLine, bool) {
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return diff.DiffLine{}, false
	}
	return m.file.lines[m.nav.diffCursor], true
}

// moveDiffCursorDown moves the diff cursor to the next non-divider line.
// if the current line has an annotation and cursor is on the diff line, stops on the annotation first.
// in collapsed mode, also skips removed lines unless their hunk is expanded.
func (m *Model) moveDiffCursorDown() {
	hunks := m.findHunks()

	// if currently on annotation sub-line, move to the next diff line
	if m.annot.cursorOnAnnotation {
		m.annot.cursorOnAnnotation = false
		for i := m.nav.diffCursor + 1; i < len(m.file.lines); i++ {
			if m.file.lines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
				m.nav.diffCursor = i
				return
			}
		}
		return
	}

	// if current line has an annotation, stop on it first.
	// skip for delete-only placeholders — their annotations are only visible when expanded.
	if m.nav.diffCursor >= 0 && m.nav.diffCursor < len(m.file.lines) {
		dl := m.file.lines[m.nav.diffCursor]
		if dl.ChangeType != diff.ChangeDivider && !m.isDeleteOnlyPlaceholder(m.nav.diffCursor, hunks) {
			lineNum := m.diffLineNum(dl)
			if m.store.Has(m.file.name, lineNum, string(dl.ChangeType)) {
				m.annot.cursorOnAnnotation = true
				return
			}
		}
	}

	// move to next non-divider diff line, skipping collapsed hidden lines
	start := m.nav.diffCursor + 1
	if m.nav.diffCursor == -1 {
		start = 0
	}
	for i := start; i < len(m.file.lines); i++ {
		if m.file.lines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.nav.diffCursor = i
			return
		}
	}
}

// moveDiffCursorUp moves the diff cursor to the previous non-divider line.
// when moving up from a diff line, if the previous line has an annotation, lands on the annotation first.
// in collapsed mode, also skips removed lines unless their hunk is expanded.
func (m *Model) moveDiffCursorUp() {
	// if currently on annotation sub-line, move up to the diff line itself
	if m.annot.cursorOnAnnotation {
		m.annot.cursorOnAnnotation = false
		return
	}

	hunks := m.findHunks()
	for i := m.nav.diffCursor - 1; i >= 0; i-- {
		if m.file.lines[i].ChangeType == diff.ChangeDivider || m.isCollapsedHidden(i, hunks) {
			continue
		}
		m.nav.diffCursor = i
		// if this line has an annotation, land on it (skip for delete-only placeholders)
		dl := m.file.lines[i]
		lineNum := m.diffLineNum(dl)
		if m.store.Has(m.file.name, lineNum, string(dl.ChangeType)) && !m.isDeleteOnlyPlaceholder(i, hunks) {
			m.annot.cursorOnAnnotation = true
		}
		return
	}
	// if we're at the first line and there's a file-level annotation, go to it
	if m.nav.diffCursor >= 0 && m.hasFileAnnotation() {
		m.nav.diffCursor = -1
	}
}

// moveDiffCursorPageDown moves the diff cursor down by one visual page.
// accounts for divider lines and annotation rows that occupy rendered space.
// scrolls the viewport so cursor appears near the top of the new page.
func (m *Model) moveDiffCursorPageDown() {
	startY := m.cursorViewportY()
	for {
		prev := m.nav.diffCursor
		m.moveDiffCursorDown()
		if m.nav.diffCursor == prev {
			break
		}
		if m.cursorViewportY()-startY >= m.layout.viewport.Height {
			break
		}
	}
	// place cursor at the top of the viewport for a true page-scroll feel
	m.layout.viewport.SetYOffset(m.cursorViewportY())
	m.layout.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorPageUp moves the diff cursor up by one visual page.
// accounts for divider lines and annotation rows that occupy rendered space.
// scrolls the viewport so cursor appears near the bottom of the new page.
func (m *Model) moveDiffCursorPageUp() {
	startY := m.cursorViewportY()
	for {
		prev := m.nav.diffCursor
		m.moveDiffCursorUp()
		if m.nav.diffCursor == prev {
			break
		}
		if startY-m.cursorViewportY() >= m.layout.viewport.Height {
			break
		}
	}
	// place cursor at the bottom of the viewport for a true page-scroll feel
	m.layout.viewport.SetYOffset(max(0, m.cursorViewportY()-m.layout.viewport.Height+1))
	m.layout.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorHalfPageDown moves the diff cursor down by half a visual page.
// scrolls viewport by half page explicitly, matching vim/less ctrl+d behavior.
func (m *Model) moveDiffCursorHalfPageDown() {
	halfPage := max(1, m.layout.viewport.Height/2)
	startY := m.cursorViewportY()
	for {
		prev := m.nav.diffCursor
		m.moveDiffCursorDown()
		if m.nav.diffCursor == prev {
			break
		}
		if m.cursorViewportY()-startY >= halfPage {
			break
		}
	}
	maxOffset := max(0, m.layout.viewport.TotalLineCount()-m.layout.viewport.Height)
	m.layout.viewport.SetYOffset(min(m.layout.viewport.YOffset+halfPage, maxOffset))
	m.layout.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorHalfPageUp moves the diff cursor up by half a visual page.
// scrolls viewport by half page explicitly, matching vim/less ctrl+u behavior.
func (m *Model) moveDiffCursorHalfPageUp() {
	halfPage := max(1, m.layout.viewport.Height/2)
	startY := m.cursorViewportY()
	for {
		prev := m.nav.diffCursor
		m.moveDiffCursorUp()
		if m.nav.diffCursor == prev {
			break
		}
		if startY-m.cursorViewportY() >= halfPage {
			break
		}
	}
	m.layout.viewport.SetYOffset(max(0, m.layout.viewport.YOffset-halfPage))
	m.layout.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorToStart moves the diff cursor to the first selectable position.
// if a file-level annotation exists, the cursor goes to -1 (file annotation line).
func (m *Model) moveDiffCursorToStart() {
	m.annot.cursorOnAnnotation = false
	if m.hasFileAnnotation() {
		m.nav.diffCursor = -1
		m.syncViewportToCursor()
		return
	}

	m.skipInitialDividers()
	m.syncViewportToCursor()
}

// moveDiffCursorToEnd moves the diff cursor to the last visible non-divider line.
// in collapsed mode, skips hidden removed lines.
func (m *Model) moveDiffCursorToEnd() {
	m.annot.cursorOnAnnotation = false
	hunks := m.findHunks()
	for i := len(m.file.lines) - 1; i >= 0; i-- {
		if m.file.lines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.nav.diffCursor = i
			break
		}
	}
	m.syncViewportToCursor()
}

// syncViewportToCursor adjusts viewport scroll so the cursor's logical line
// is visible — its full visual extent when it fits, otherwise the cursor's
// top row (trailing wrap/annotation rows then overflow by necessity).
// re-renders content in the process. the down-scroll branch compares against
// the cursor's visual bottom row so wrap-continuation rows and injected
// annotation rows below the diff line are not clipped at the viewport edge.
// SetContent runs before SetYOffset because viewport.SetYOffset clamps to
// maxYOffset() (derived from current content length); rendering first lets
// the clamp accept a larger target after content grows (e.g. a freshly saved
// multi-row annotation extending past the prior content end).
func (m *Model) syncViewportToCursor() {
	cursorTop, cursorBottom := m.cursorVisualRange()
	m.layout.viewport.SetContent(m.renderDiff())
	switch {
	case cursorTop < m.layout.viewport.YOffset:
		m.layout.viewport.SetYOffset(cursorTop)
	case cursorBottom >= m.layout.viewport.YOffset+m.layout.viewport.Height:
		m.layout.viewport.SetYOffset(min(cursorBottom-m.layout.viewport.Height+1, cursorTop))
	}
}

// centerViewportOnCursor scrolls the viewport to place the cursor in the middle of the page.
func (m *Model) centerViewportOnCursor() {
	cursorY := m.cursorViewportY()
	offset := max(0, cursorY-m.layout.viewport.Height/2)
	m.layout.viewport.SetYOffset(offset)
	m.layout.viewport.SetContent(m.renderDiff())
}

// centerHunkInViewport centers the current hunk in the viewport.
// For small hunks, the entire hunk is centered. For large hunks that exceed
// the viewport height, the first line is placed near the top with a small context margin.
// No-op if the cursor is not on a changed line (ChangeAdd/ChangeRemove).
func (m *Model) centerHunkInViewport() {
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return
	}
	ct := m.file.lines[m.nav.diffCursor].ChangeType
	if ct != diff.ChangeAdd && ct != diff.ChangeRemove {
		return
	}

	// build shared context once for both cursor Y and hunk height
	var hunks []int
	if m.modes.collapsed.enabled {
		hunks = m.findHunks()
	}
	annotationSet := m.buildAnnotationSet()

	cursorY := m.cursorViewportYUsing(hunks, annotationSet)

	// find hunk end: scan forward from cursor while lines are changed.
	// hidden ChangeRemove lines (collapsed mode) are included in the range because
	// hunkLineHeight returns 0 for them, so over-extending hunkEnd is harmless.
	hunkEnd := m.nav.diffCursor
	for i := m.nav.diffCursor + 1; i < len(m.file.lines); i++ {
		ct := m.file.lines[i].ChangeType
		if ct != diff.ChangeAdd && ct != diff.ChangeRemove {
			break
		}
		hunkEnd = i
	}

	// calculate visual height of the hunk
	hunkVisualHeight := 0
	for i := m.nav.diffCursor; i <= hunkEnd; i++ {
		hunkVisualHeight += m.hunkLineHeight(i, hunks, annotationSet)
	}

	var offset int
	if hunkVisualHeight >= m.layout.viewport.Height {
		// hunk taller than viewport: place first line near top with small context margin
		offset = max(0, cursorY-2)
	} else {
		// center the entire hunk by centering its midpoint
		hunkMidY := cursorY + hunkVisualHeight/2
		offset = max(0, hunkMidY-m.layout.viewport.Height/2)
	}
	m.layout.viewport.SetYOffset(offset)
	m.layout.viewport.SetContent(m.renderDiff())
}

// topAlignViewportOnCursor scrolls the viewport to place the cursor at the top of the page.
func (m *Model) topAlignViewportOnCursor() {
	cursorY := m.cursorViewportY()
	m.layout.viewport.SetYOffset(max(0, cursorY))
	m.layout.viewport.SetContent(m.renderDiff())
}

// findHunks scans diffLines and returns a slice of hunk start indices.
// a hunk is a contiguous group of added/removed lines. the returned index
// is the first line of each such group.
func (m Model) findHunks() []int {
	var hunks []int
	inHunk := false
	for i, dl := range m.file.lines {
		isChange := dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove
		switch {
		case isChange && !inHunk:
			hunks = append(hunks, i)
			inHunk = true
		case !isChange:
			inHunk = false
		}
	}
	return hunks
}

// currentHunk returns the 1-based hunk index and total hunk count.
// returns non-zero hunk index only when the cursor is on a changed line (add/remove).
// returns (0, total) when cursor is not inside any hunk.
func (m Model) currentHunk() (int, int) {
	hunks := m.findHunks()
	if len(hunks) == 0 {
		return 0, 0
	}
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return 0, len(hunks)
	}
	dl := m.file.lines[m.nav.diffCursor]
	if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
		return 0, len(hunks)
	}
	// cursor is on a changed line, find which hunk
	cur := 0
	for i, start := range hunks {
		if m.nav.diffCursor >= start {
			cur = i + 1
		}
	}
	return cur, len(hunks)
}

// moveToNextHunk moves the diff cursor to the start of the next change hunk.
// in collapsed mode, advances past hidden removed lines to the first visible line in the hunk.
func (m *Model) moveToNextHunk() {
	m.annot.cursorOnAnnotation = false
	hunks := m.findHunks()
	for _, start := range hunks {
		if start <= m.nav.diffCursor {
			continue
		}
		target := m.firstVisibleInHunk(start, hunks)
		if target < 0 {
			continue // skip delete-only hunks in collapsed mode
		}
		m.nav.diffCursor = target
		m.centerHunkInViewport()
		return
	}
}

// moveToPrevHunk moves the diff cursor to the start of the previous change hunk.
// in collapsed mode, advances past hidden removed lines to the first visible line in the hunk.
func (m *Model) moveToPrevHunk() {
	m.annot.cursorOnAnnotation = false
	hunks := m.findHunks()
	for i := len(hunks) - 1; i >= 0; i-- {
		target := m.firstVisibleInHunk(hunks[i], hunks)
		if target < 0 {
			continue // skip delete-only hunks in collapsed mode
		}
		if target < m.nav.diffCursor {
			m.nav.diffCursor = target
			m.centerHunkInViewport()
			return
		}
	}
}

// handleHunkNav moves to the next or previous hunk, crossing file boundaries when needed.
// when cross-file hunk navigation is enabled, forward at the last hunk navigates to the next file
// and lands on its first hunk, and backward at the first hunk navigates to the previous file and
// lands on its last hunk.
// always shifts focus to the diff pane. no-op when no file is loaded.
func (m Model) handleHunkNav(forward bool) (tea.Model, tea.Cmd) {
	if m.file.name == "" {
		return m, nil
	}
	m.layout.focus = paneDiff
	prevCursor := m.nav.diffCursor
	if forward {
		m.moveToNextHunk()
	} else {
		m.moveToPrevHunk()
	}
	if m.nav.diffCursor != prevCursor || m.file.singleFile || !m.cfg.crossFileHunks {
		m.syncTOCActiveSection()
		return m, nil
	}
	// cursor did not move — we are at the boundary; try to cross to adjacent file
	if forward {
		if m.tree.HasFile(sidepane.DirectionNext) {
			fwd := true
			m.nav.pendingHunkJump = &fwd
			m.tree.StepFile(sidepane.DirectionNext)
			return m.loadSelectedIfChanged()
		}
	} else {
		if m.tree.HasFile(sidepane.DirectionPrev) {
			bwd := false
			m.nav.pendingHunkJump = &bwd
			m.tree.StepFile(sidepane.DirectionPrev)
			return m.loadSelectedIfChanged()
		}
	}
	m.syncTOCActiveSection()
	return m, nil
}

// handleHorizontalScroll processes left/right scroll keys.
// direction < 0 scrolls left, direction > 0 scrolls right.
// no-op when wrap mode is active (content is already fully visible).
func (m *Model) handleHorizontalScroll(direction int) {
	if m.modes.wrap {
		return
	}
	if direction < 0 {
		m.layout.scrollX = max(0, m.layout.scrollX-scrollStep)
	} else {
		m.layout.scrollX += scrollStep
	}
	m.layout.viewport.SetContent(m.renderDiff())
}

// handleDiffNav handles navigation keys when the diff pane is focused.
func (m Model) handleDiffNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.keymap.Resolve(msg.String())
	switch action {
	case keymap.ActionFocusTree:
		return m.handleSwitchToTree()
	case keymap.ActionScrollLeft:
		m.handleHorizontalScroll(-1)
		return m, nil
	case keymap.ActionScrollRight:
		m.handleHorizontalScroll(1)
		return m, nil
	case keymap.ActionDown:
		m.moveDiffCursorDown()
		m.syncViewportToCursor()
	case keymap.ActionUp:
		m.moveDiffCursorUp()
		m.syncViewportToCursor()
	case keymap.ActionPageDown:
		m.moveDiffCursorPageDown()
	case keymap.ActionHalfPageDown:
		m.moveDiffCursorHalfPageDown()
	case keymap.ActionPageUp:
		m.moveDiffCursorPageUp()
	case keymap.ActionHalfPageUp:
		m.moveDiffCursorHalfPageUp()
	case keymap.ActionHome:
		m.moveDiffCursorToStart()
	case keymap.ActionEnd:
		m.moveDiffCursorToEnd()
	case keymap.ActionDeleteAnnotation:
		cmd := m.deleteAnnotation()
		return m, cmd
	case keymap.ActionToggleHunk:
		m.toggleHunkExpansion()
		return m, nil
	case keymap.ActionSearch:
		cmd := m.startSearch()
		return m, cmd
	default: // actions handled by handleKey (quit, toggle_pane, filter, etc.) — not repeated here
	}
	m.syncTOCActiveSection()
	return m, nil
}

// handleTreeNav handles navigation keys when the tree pane is focused.
func (m Model) handleTreeNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// when mdTOC is active, route navigation to TOC instead of file tree
	if m.file.mdTOC != nil {
		return m.handleTOCNav(msg)
	}

	action := m.keymap.Resolve(msg.String())
	switch action {
	case keymap.ActionDown:
		m.tree.Move(sidepane.MotionDown)
	case keymap.ActionUp:
		m.tree.Move(sidepane.MotionUp)
	case keymap.ActionPageDown:
		m.tree.Move(sidepane.MotionPageDown, m.treePageSize())
	case keymap.ActionHalfPageDown:
		m.tree.Move(sidepane.MotionPageDown, max(1, m.treePageSize()/2))
	case keymap.ActionPageUp:
		m.tree.Move(sidepane.MotionPageUp, m.treePageSize())
	case keymap.ActionHalfPageUp:
		m.tree.Move(sidepane.MotionPageUp, max(1, m.treePageSize()/2))
	case keymap.ActionHome:
		m.tree.Move(sidepane.MotionFirst)
	case keymap.ActionEnd:
		m.tree.Move(sidepane.MotionLast)
	case keymap.ActionFocusDiff, keymap.ActionScrollRight:
		if m.file.name != "" {
			m.layout.focus = paneDiff
		}
	default: // actions handled by handleKey (quit, toggle_pane, filter, etc.) — not repeated here
	}
	m.pendingAnnotJump = nil    // clear pending annotation jump on manual navigation
	m.nav.pendingHunkJump = nil // clear pending hunk jump on manual navigation
	m.tree.EnsureVisible(m.treePageSize())
	return m.loadSelectedIfChanged()
}

// handleTOCNav handles navigation keys when the TOC pane is focused.
func (m Model) handleTOCNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.keymap.Resolve(msg.String())
	switch action {
	case keymap.ActionDown:
		m.file.mdTOC.Move(sidepane.MotionDown)
	case keymap.ActionUp:
		m.file.mdTOC.Move(sidepane.MotionUp)
	case keymap.ActionPageDown:
		m.file.mdTOC.Move(sidepane.MotionPageDown, m.treePageSize())
	case keymap.ActionHalfPageDown:
		m.file.mdTOC.Move(sidepane.MotionPageDown, max(1, m.treePageSize()/2))
	case keymap.ActionPageUp:
		m.file.mdTOC.Move(sidepane.MotionPageUp, m.treePageSize())
	case keymap.ActionHalfPageUp:
		m.file.mdTOC.Move(sidepane.MotionPageUp, max(1, m.treePageSize()/2))
	case keymap.ActionHome:
		m.file.mdTOC.Move(sidepane.MotionFirst)
	case keymap.ActionEnd:
		m.file.mdTOC.Move(sidepane.MotionLast)
	case keymap.ActionFocusDiff, keymap.ActionScrollRight:
		if m.file.name != "" {
			m.layout.focus = paneDiff
		}
		return m, nil // switch pane without re-jumping viewport
	default: // actions handled by handleKey (quit, toggle_pane, filter, etc.) — not repeated here
	}
	m.file.mdTOC.EnsureVisible(m.treePageSize())
	m.syncDiffToTOCCursor()
	return m, nil
}

// handleSwitchToTree switches focus to the tree/TOC pane when available.
// no-op when tree is hidden, or in single-file mode without TOC.
// syncs TOC cursor to active section when focus is switched.
func (m Model) handleSwitchToTree() (tea.Model, tea.Cmd) {
	if m.layout.treeHidden {
		return m, nil
	}
	if !m.file.singleFile || m.file.mdTOC != nil {
		m.layout.focus = paneTree
		m.syncTOCCursorToActive()
	}
	return m, nil
}

// treePageSize returns the number of visible lines in the tree pane.
func (m Model) treePageSize() int {
	return max(1, m.paneHeight())
}

// paneHeight returns the content height for panes (total minus borders and status bar).
func (m Model) paneHeight() int {
	h := m.layout.height - 2 // borders
	if !m.cfg.noStatusBar {
		h-- // status bar
	}
	return max(1, h)
}

// jumpTOCEntry moves the TOC cursor by delta (+1 next, -1 prev) and jumps the diff viewport.
// jumpTOCEntry always passes delta +/-1; if that ever changes, extend here
func (m Model) jumpTOCEntry(delta int) (tea.Model, tea.Cmd) {
	if m.file.mdTOC == nil {
		return m, nil
	}
	if delta > 0 {
		m.file.mdTOC.Move(sidepane.MotionDown)
	} else {
		m.file.mdTOC.Move(sidepane.MotionUp)
	}
	m.syncDiffToTOCCursor()
	return m, nil
}

// syncTOCCursorToActive sets the TOC cursor to the current active section.
func (m *Model) syncTOCCursorToActive() {
	if m.file.mdTOC != nil {
		m.file.mdTOC.SyncCursorToActiveSection()
	}
}

// syncDiffToTOCCursor jumps the diff viewport to the TOC entry at the current cursor position.
func (m *Model) syncDiffToTOCCursor() {
	if m.file.mdTOC == nil {
		return
	}
	idx, ok := m.file.mdTOC.CurrentLineIdx()
	if !ok {
		return
	}
	m.nav.diffCursor = idx
	m.annot.cursorOnAnnotation = false
	m.file.mdTOC.UpdateActiveSection(m.nav.diffCursor)
	m.topAlignViewportOnCursor()
}

// syncTOCActiveSection updates the TOC active section to match the current diff cursor position.
func (m *Model) syncTOCActiveSection() {
	if m.file.mdTOC != nil {
		m.file.mdTOC.UpdateActiveSection(m.nav.diffCursor)
	}
}

// applyPendingHunkJump moves the cursor to the first or last hunk after a cross-file navigation.
func (m *Model) applyPendingHunkJump() {
	forward := *m.nav.pendingHunkJump
	m.nav.pendingHunkJump = nil
	if forward {
		m.nav.diffCursor = -1
		m.moveToNextHunk()
		if m.nav.diffCursor != -1 {
			return
		}
		m.skipInitialDividers()
		return
	}

	m.nav.diffCursor = len(m.file.lines)
	m.moveToPrevHunk()
	if m.nav.diffCursor == len(m.file.lines) {
		m.skipInitialDividers()
	}
}
