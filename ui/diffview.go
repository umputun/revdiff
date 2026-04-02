package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/diff"
)

// renderDiff renders the current file's diff lines with styling, cursor highlight,
// and injected annotation lines.
func (m Model) renderDiff() string {
	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	if m.collapsed {
		return m.renderCollapsedDiff()
	}

	annotationMap, fileComment := m.buildAnnotationMap()
	var b strings.Builder
	m.renderFileAnnotationHeader(&b, fileComment)

	for i, dl := range m.diffLines {
		m.renderDiffLine(&b, i, dl)
		m.renderAnnotationOrInput(&b, i, annotationMap)
	}
	return b.String()
}

// renderCollapsedDiff renders the collapsed diff view showing only final text.
// removed lines are hidden unless their hunk is expanded. added lines are styled
// as "modified" (amber ~) when paired with removes, or "pure add" (green +) otherwise.
func (m Model) renderCollapsedDiff() string {
	annotationMap, fileComment := m.buildAnnotationMap()
	hunks := m.findHunks()
	modifiedSet := m.buildModifiedSet(hunks)

	var b strings.Builder
	m.renderFileAnnotationHeader(&b, fileComment)

	hasVisibleContent := false
	for i, dl := range m.diffLines {
		hunkStart := m.hunkStartFor(i, hunks)
		expanded := hunkStart >= 0 && m.expandedHunks[hunkStart]

		switch dl.ChangeType {
		case diff.ChangeRemove:
			switch {
			case expanded:
				m.renderDiffLine(&b, i, dl)
			case i == hunkStart && hunkStart >= 0 && m.isDeleteOnlyHunk(hunkStart):
				m.renderDeletePlaceholder(&b, i, hunkStart)
				hasVisibleContent = true
				continue // placeholder is synthetic, skip annotation rendering
			default:
				continue // hide removed lines in collapsed mode
			}

		case diff.ChangeAdd:
			if expanded {
				m.renderDiffLine(&b, i, dl) // use standard add styling when hunk is expanded
			} else {
				m.renderCollapsedAddLine(&b, i, dl, modifiedSet[i])
			}

		default: // context and divider lines render normally
			m.renderDiffLine(&b, i, dl)
		}
		hasVisibleContent = true

		m.renderAnnotationOrInput(&b, i, annotationMap)
	}

	if !hasVisibleContent {
		b.WriteString("  (file deleted)\n")
	}
	return b.String()
}

// renderCollapsedAddLine renders an add line in collapsed mode with modify or add styling.
func (m Model) renderCollapsedAddLine(b *strings.Builder, idx int, dl diff.DiffLine, modified bool) {
	hasHighlight := idx < len(m.highlightedLines)
	hlContent := ""
	if hasHighlight {
		hlContent = strings.ReplaceAll(m.highlightedLines[idx], "\t", m.tabSpaces)
	}
	lineContent := strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)

	style, hlStyle, gutter := m.styles.LineAdd, m.styles.LineAddHighlight, " + "
	if modified {
		style, hlStyle, gutter = m.styles.LineModify, m.styles.LineModifyHighlight, " ~ "
	}

	content := style.Render(gutter + lineContent)
	if hasHighlight {
		content = hlStyle.Render(gutter + hlContent)
	}

	// apply horizontal scroll
	if m.scrollX > 0 {
		content = ansi.Cut(content, m.scrollX, m.scrollX+m.diffContentWidth())
	}

	isCursor := idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation
	cursor := " "
	if isCursor {
		cursor = m.styles.DiffCursorLine.Render("▶")
	}
	b.WriteString(cursor + content + "\n")
}

// renderDeletePlaceholder renders a placeholder line for a delete-only hunk in collapsed mode.
// shows "⋯ N lines deleted" with remove styling so users know deletions exist and can expand with '.'.
func (m Model) renderDeletePlaceholder(b *strings.Builder, idx, hunkStart int) {
	count := 0
	for i := hunkStart; i < len(m.diffLines); i++ {
		ct := m.diffLines[i].ChangeType
		if ct == diff.ChangeContext || ct == diff.ChangeDivider {
			break
		}
		if ct == diff.ChangeRemove {
			count++
		}
	}

	text := fmt.Sprintf("⋯ %d lines deleted", count)
	if count == 1 {
		text = "⋯ 1 line deleted"
	}
	content := m.styles.LineRemove.Render(" - " + text)

	// apply horizontal scroll
	if m.scrollX > 0 {
		content = ansi.Cut(content, m.scrollX, m.scrollX+m.diffContentWidth())
	}

	isCursor := idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation
	cursor := " "
	if isCursor {
		cursor = m.styles.DiffCursorLine.Render("▶")
	}
	b.WriteString(cursor + content + "\n")
}

// hunkStartFor returns the findHunks() start index for the hunk containing diffLines[idx].
// returns -1 if the index is not inside any hunk (context or divider line).
func (m Model) hunkStartFor(idx int, hunks []int) int {
	if len(hunks) == 0 || idx < 0 || idx >= len(m.diffLines) {
		return -1
	}
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
		return -1
	}
	best := -1
	for _, start := range hunks {
		if start <= idx {
			best = start
		}
	}
	return best
}

// buildAnnotationMap creates a lookup map of line annotations for the current file.
// returns the annotation map and the file-level comment (empty if none).
func (m Model) buildAnnotationMap() (annotations map[string]string, fileComment string) {
	all := m.store.Get(m.currFile)
	annotations = make(map[string]string, len(all))
	for _, a := range all {
		if a.Line == 0 {
			fileComment = a.Comment
			continue
		}
		annotations[m.annotationKey(a.Line, a.Type)] = a.Comment
	}
	return annotations, fileComment
}

// renderFileAnnotationHeader writes the file-level annotation or input to the builder.
func (m Model) renderFileAnnotationHeader(b *strings.Builder, fileComment string) {
	// when actively editing a file-level annotation, always show the input widget
	if m.annotating && m.fileAnnotating {
		line := " " + m.styles.AnnotationLine.Render("\U0001f4ac file: ") + m.annotateInput.View()
		b.WriteString(line + "\n")
		return
	}

	if fileComment != "" {
		cursor := " "
		if m.diffCursor == -1 && m.focus == paneDiff {
			cursor = m.styles.DiffCursorLine.Render("▶")
		}
		line := cursor + m.styles.AnnotationLine.Render("\U0001f4ac file: "+fileComment)
		b.WriteString(line + "\n")
	}
}

// renderDiffLine writes a single styled diff line (with cursor highlight) to the builder.
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	// check for pre-computed syntax-highlighted content
	hasHighlight := idx < len(m.highlightedLines)
	hlContent := ""
	if hasHighlight {
		hlContent = strings.ReplaceAll(m.highlightedLines[idx], "\t", m.tabSpaces)
	}
	lineContent := strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)

	var content string
	switch dl.ChangeType {
	case diff.ChangeAdd:
		if hasHighlight {
			content = m.styles.LineAddHighlight.Render(" + " + hlContent)
		} else {
			content = m.styles.LineAdd.Render(" + " + lineContent)
		}
	case diff.ChangeRemove:
		if hasHighlight {
			content = m.styles.LineRemoveHighlight.Render(" - " + hlContent)
		} else {
			content = m.styles.LineRemove.Render(" - " + lineContent)
		}
	case diff.ChangeDivider:
		content = m.styles.LineNumber.Render(" " + lineContent)
	default:
		if hasHighlight {
			content = "   " + hlContent
		} else {
			content = m.styles.LineContext.Render("   " + lineContent)
		}
	}

	// apply horizontal scroll to content (bar stays fixed)
	if m.scrollX > 0 {
		content = ansi.Cut(content, m.scrollX, m.scrollX+m.diffContentWidth())
	}

	isCursor := idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation
	cursor := " "
	if isCursor {
		cursor = m.styles.DiffCursorLine.Render("▶")
	}
	b.WriteString(cursor + content + "\n")
}

// renderAnnotationOrInput writes the annotation input or existing annotation below a diff line.
func (m Model) renderAnnotationOrInput(b *strings.Builder, idx int, annotationMap map[string]string) {
	if m.annotating && !m.fileAnnotating && idx == m.diffCursor {
		b.WriteString(" " + m.styles.AnnotationLine.Render("\U0001f4ac ") + m.annotateInput.View() + "\n")
		return
	}
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
		if comment, ok := annotationMap[key]; ok {
			cursor := " "
			if idx == m.diffCursor && m.cursorOnAnnotation && m.focus == paneDiff {
				cursor = m.styles.DiffCursorLine.Render("▶")
			}
			line := cursor + m.styles.AnnotationLine.Render("\U0001f4ac "+comment)
			b.WriteString(line + "\n")
		}
	}
}

// cursorDiffLine returns the DiffLine at the current cursor position, if valid.
func (m Model) cursorDiffLine() (diff.DiffLine, bool) {
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return diff.DiffLine{}, false
	}
	return m.diffLines[m.diffCursor], true
}

// moveDiffCursorDown moves the diff cursor to the next non-divider line.
// if the current line has an annotation and cursor is on the diff line, stops on the annotation first.
// in collapsed mode, also skips removed lines unless their hunk is expanded.
func (m *Model) moveDiffCursorDown() {
	hunks := m.findHunks()

	// if currently on annotation sub-line, move to the next diff line
	if m.cursorOnAnnotation {
		m.cursorOnAnnotation = false
		for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
			if m.diffLines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
				m.diffCursor = i
				return
			}
		}
		return
	}

	// if current line has an annotation, stop on it first.
	// skip for delete-only placeholders — their annotations are only visible when expanded.
	if m.diffCursor >= 0 && m.diffCursor < len(m.diffLines) {
		dl := m.diffLines[m.diffCursor]
		if dl.ChangeType != diff.ChangeDivider && !m.isDeleteOnlyPlaceholder(m.diffCursor, hunks) {
			lineNum := m.diffLineNum(dl)
			if m.store.Has(m.currFile, lineNum, string(dl.ChangeType)) {
				m.cursorOnAnnotation = true
				return
			}
		}
	}

	// move to next non-divider diff line, skipping collapsed hidden lines
	start := m.diffCursor + 1
	if m.diffCursor == -1 {
		start = 0
	}
	for i := start; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.diffCursor = i
			return
		}
	}
}

// moveDiffCursorUp moves the diff cursor to the previous non-divider line.
// when moving up from a diff line, if the previous line has an annotation, lands on the annotation first.
// in collapsed mode, also skips removed lines unless their hunk is expanded.
func (m *Model) moveDiffCursorUp() {
	// if currently on annotation sub-line, move up to the diff line itself
	if m.cursorOnAnnotation {
		m.cursorOnAnnotation = false
		return
	}

	hunks := m.findHunks()
	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType == diff.ChangeDivider || m.isCollapsedHidden(i, hunks) {
			continue
		}
		m.diffCursor = i
		// if this line has an annotation, land on it (skip for delete-only placeholders)
		dl := m.diffLines[i]
		lineNum := m.diffLineNum(dl)
		if m.store.Has(m.currFile, lineNum, string(dl.ChangeType)) && !m.isDeleteOnlyPlaceholder(i, hunks) {
			m.cursorOnAnnotation = true
		}
		return
	}
	// if we're at the first line and there's a file-level annotation, go to it
	if m.diffCursor >= 0 && m.hasFileAnnotation() {
		m.diffCursor = -1
	}
}

// moveDiffCursorPageDown moves the diff cursor down by one visual page.
// accounts for divider lines and annotation rows that occupy rendered space.
// scrolls the viewport so cursor appears near the top of the new page.
func (m *Model) moveDiffCursorPageDown() {
	startY := m.cursorViewportY()
	for {
		prev := m.diffCursor
		m.moveDiffCursorDown()
		if m.diffCursor == prev {
			break
		}
		if m.cursorViewportY()-startY >= m.viewport.Height {
			break
		}
	}
	// place cursor at the top of the viewport for a true page-scroll feel
	m.viewport.SetYOffset(m.cursorViewportY())
	m.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorPageUp moves the diff cursor up by one visual page.
// accounts for divider lines and annotation rows that occupy rendered space.
// scrolls the viewport so cursor appears near the bottom of the new page.
func (m *Model) moveDiffCursorPageUp() {
	startY := m.cursorViewportY()
	for {
		prev := m.diffCursor
		m.moveDiffCursorUp()
		if m.diffCursor == prev {
			break
		}
		if startY-m.cursorViewportY() >= m.viewport.Height {
			break
		}
	}
	// place cursor at the bottom of the viewport for a true page-scroll feel
	m.viewport.SetYOffset(max(0, m.cursorViewportY()-m.viewport.Height+1))
	m.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorToStart moves the diff cursor to the first selectable position.
// if a file-level annotation exists, the cursor goes to -1 (file annotation line).
func (m *Model) moveDiffCursorToStart() {
	m.cursorOnAnnotation = false
	if m.hasFileAnnotation() {
		m.diffCursor = -1
		m.syncViewportToCursor()
		return
	}

	m.skipInitialDividers()
	m.syncViewportToCursor()
}

// moveDiffCursorToEnd moves the diff cursor to the last visible non-divider line.
// in collapsed mode, skips hidden removed lines.
func (m *Model) moveDiffCursorToEnd() {
	m.cursorOnAnnotation = false
	hunks := m.findHunks()
	for i := len(m.diffLines) - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.diffCursor = i
			break
		}
	}
	m.syncViewportToCursor()
}

// syncViewportToCursor adjusts viewport scroll to keep cursor visible and re-renders content.
// accounts for annotation lines injected between diff lines.
func (m *Model) syncViewportToCursor() {
	cursorY := m.cursorViewportY()
	if cursorY < m.viewport.YOffset {
		m.viewport.SetYOffset(cursorY)
	} else if cursorY >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(cursorY - m.viewport.Height + 1)
	}
	m.viewport.SetContent(m.renderDiff())
}

// findHunks scans diffLines and returns a slice of hunk start indices.
// a hunk is a contiguous group of added/removed lines. the returned index
// is the first line of each such group.
func (m Model) findHunks() []int {
	var hunks []int
	inHunk := false
	for i, dl := range m.diffLines {
		isChange := dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove
		if isChange && !inHunk {
			hunks = append(hunks, i)
			inHunk = true
		} else if !isChange {
			inHunk = false
		}
	}
	return hunks
}

// buildModifiedSet returns a set of diffLines indices for add lines that are "modified"
// (paired with removes in the same hunk). pure-add lines (hunk has no removes) are not included.
func (m Model) buildModifiedSet(hunks []int) map[int]bool {
	result := make(map[int]bool)
	n := len(m.diffLines)

	for hi, start := range hunks {
		// find the end of this hunk: next hunk start or first non-change line
		end := n
		if hi+1 < len(hunks) {
			end = hunks[hi+1]
		}
		// scan only contiguous change lines from start
		for end > start && (m.diffLines[end-1].ChangeType != diff.ChangeAdd &&
			m.diffLines[end-1].ChangeType != diff.ChangeRemove) {
			end--
		}

		// check if hunk has both removes and adds
		hasRemove, hasAdd := false, false
		var addIndices []int
		for i := start; i < end; i++ {
			switch m.diffLines[i].ChangeType {
			case diff.ChangeRemove:
				hasRemove = true
			case diff.ChangeAdd:
				hasAdd = true
				addIndices = append(addIndices, i)
			case diff.ChangeContext, diff.ChangeDivider:
				// context and divider lines are not part of the hunk's change set
			}
		}

		if hasRemove && hasAdd {
			for _, idx := range addIndices {
				result[idx] = true
			}
		}
	}
	return result
}

// currentHunk returns the 1-based hunk index and total hunk count.
// returns non-zero hunk index only when the cursor is on a changed line (add/remove).
// returns (0, total) when cursor is not inside any hunk.
func (m Model) currentHunk() (int, int) {
	hunks := m.findHunks()
	if len(hunks) == 0 {
		return 0, 0
	}
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return 0, len(hunks)
	}
	dl := m.diffLines[m.diffCursor]
	if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
		return 0, len(hunks)
	}
	// cursor is on a changed line, find which hunk
	cur := 0
	for i, start := range hunks {
		if m.diffCursor >= start {
			cur = i + 1
		}
	}
	return cur, len(hunks)
}

// moveToNextHunk moves the diff cursor to the start of the next change hunk.
// in collapsed mode, advances past hidden removed lines to the first visible line in the hunk.
func (m *Model) moveToNextHunk() {
	m.cursorOnAnnotation = false
	hunks := m.findHunks()
	for _, start := range hunks {
		if start <= m.diffCursor {
			continue
		}
		target := m.firstVisibleInHunk(start, hunks)
		if target < 0 {
			continue // skip delete-only hunks in collapsed mode
		}
		m.diffCursor = target
		m.centerViewportOnCursor()
		return
	}
}

// moveToPrevHunk moves the diff cursor to the start of the previous change hunk.
// in collapsed mode, advances past hidden removed lines to the first visible line in the hunk.
func (m *Model) moveToPrevHunk() {
	m.cursorOnAnnotation = false
	hunks := m.findHunks()
	for i := len(hunks) - 1; i >= 0; i-- {
		target := m.firstVisibleInHunk(hunks[i], hunks)
		if target < 0 {
			continue // skip delete-only hunks in collapsed mode
		}
		if target < m.diffCursor {
			m.diffCursor = target
			m.centerViewportOnCursor()
			return
		}
	}
}

// cursorHunkStart returns the findHunks() start index for the hunk containing the cursor.
// returns false if the cursor is not inside any hunk.
func (m Model) cursorHunkStart() (int, bool) {
	hunks := m.findHunks()
	best := m.hunkStartFor(m.diffCursor, hunks)
	if best < 0 {
		return 0, false
	}
	return best, true
}

// toggleCollapsedMode switches between collapsed and expanded diff view.
// only operates when the diff pane is focused and a file is loaded.
func (m *Model) toggleCollapsedMode() {
	if m.focus != paneDiff || m.currFile == "" {
		return
	}
	m.collapsed = !m.collapsed
	m.expandedHunks = make(map[int]bool)
	m.cursorOnAnnotation = false // visible lines change, reset annotation cursor state
	m.adjustCursorIfHidden()
	m.viewport.SetContent(m.renderDiff())
}

// toggleHunkExpansion toggles the expansion state of the hunk under the cursor.
// only operates in collapsed mode; no-op in expanded mode or when cursor is not on a hunk.
func (m *Model) toggleHunkExpansion() {
	if !m.collapsed {
		return
	}
	hunkStart, ok := m.cursorHunkStart()
	if !ok {
		return
	}
	if m.expandedHunks[hunkStart] {
		delete(m.expandedHunks, hunkStart)
		m.cursorOnAnnotation = false // annotations on removed lines become invisible
		m.adjustCursorIfHidden()
	} else {
		m.expandedHunks[hunkStart] = true
	}
	m.viewport.SetContent(m.renderDiff())
}

// isCollapsedHidden returns true if the line at idx is hidden in collapsed mode.
// a line is hidden when collapsed mode is active, the line is a remove line,
// and its hunk is not expanded. the first line of a delete-only hunk is kept
// visible as a placeholder so users can navigate to it and expand with '.'.
func (m Model) isCollapsedHidden(idx int, hunks []int) bool {
	if !m.collapsed || idx < 0 || idx >= len(m.diffLines) {
		return false
	}
	if m.diffLines[idx].ChangeType != diff.ChangeRemove {
		return false
	}
	hunkStart := m.hunkStartFor(idx, hunks)
	if hunkStart < 0 {
		return true
	}
	if m.expandedHunks[hunkStart] {
		return false
	}
	// first line of a delete-only hunk serves as the visible placeholder
	if idx == hunkStart && m.isDeleteOnlyHunk(hunkStart) {
		return false
	}
	return true
}

// isDeleteOnlyPlaceholder returns true if the line at idx is rendered as a synthetic
// delete-only placeholder (⋯ N lines deleted) in collapsed mode. these lines should not
// display or accept annotations — annotations become visible when the hunk is expanded.
func (m Model) isDeleteOnlyPlaceholder(idx int, hunks []int) bool {
	if !m.collapsed {
		return false
	}
	if idx < 0 || idx >= len(m.diffLines) || m.diffLines[idx].ChangeType != diff.ChangeRemove {
		return false
	}
	hunkStart := m.hunkStartFor(idx, hunks)
	return hunkStart >= 0 && idx == hunkStart && !m.expandedHunks[hunkStart] && m.isDeleteOnlyHunk(hunkStart)
}

// isDeleteOnlyHunk returns true if the hunk starting at hunkStart contains only remove lines.
func (m Model) isDeleteOnlyHunk(hunkStart int) bool {
	for i := hunkStart; i < len(m.diffLines); i++ {
		ct := m.diffLines[i].ChangeType
		if ct == diff.ChangeContext || ct == diff.ChangeDivider {
			break
		}
		if ct == diff.ChangeAdd {
			return false
		}
	}
	return true
}

// firstVisibleInHunk returns the first visible line index starting from hunkStart.
// in collapsed mode, this skips hidden removed lines. in expanded mode, returns hunkStart unchanged.
// returns -1 if the hunk has no visible lines (delete-only hunk in collapsed mode).
func (m Model) firstVisibleInHunk(hunkStart int, hunks []int) int {
	if !m.isCollapsedHidden(hunkStart, hunks) {
		return hunkStart
	}
	for i := hunkStart + 1; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType == diff.ChangeDivider || m.diffLines[i].ChangeType == diff.ChangeContext {
			break // past the hunk boundary
		}
		if !m.isCollapsedHidden(i, hunks) {
			return i
		}
	}
	return -1 // no visible lines in this hunk (delete-only, not expanded)
}

// adjustCursorIfHidden moves the cursor to the nearest visible line if it is currently
// on a hidden removed line in collapsed mode. searches forward first, then backward.
// falls back to nearest divider if no content line is visible (delete-only file).
func (m *Model) adjustCursorIfHidden() {
	if !m.collapsed || m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return
	}
	hunks := m.findHunks()
	if !m.isCollapsedHidden(m.diffCursor, hunks) {
		return
	}
	// search forward for nearest visible non-divider line
	for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.diffCursor = i
			return
		}
	}
	// search backward for nearest visible non-divider line
	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.diffCursor = i
			return
		}
	}
	// no visible content line found (delete-only file); fall back to nearest divider
	for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
		if m.diffLines[i].ChangeType == diff.ChangeDivider {
			m.diffCursor = i
			return
		}
	}
	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType == diff.ChangeDivider {
			m.diffCursor = i
			return
		}
	}
}

// centerViewportOnCursor scrolls the viewport to place the cursor in the middle of the page.
func (m *Model) centerViewportOnCursor() {
	cursorY := m.cursorViewportY()
	offset := max(0, cursorY-m.viewport.Height/2)
	m.viewport.SetYOffset(offset)
	m.viewport.SetContent(m.renderDiff())
}

const scrollStep = 4 // horizontal scroll step in characters

// scrollRight moves the horizontal scroll offset to the right.
func (m *Model) scrollRight() {
	m.scrollX += scrollStep
	m.viewport.SetContent(m.renderDiff())
}

// scrollLeft moves the horizontal scroll offset to the left.
func (m *Model) scrollLeft() {
	m.scrollX = max(0, m.scrollX-scrollStep)
	m.viewport.SetContent(m.renderDiff())
}

// diffContentWidth returns the available width for diff line content (excluding cursor bar).
func (m Model) diffContentWidth() int {
	// diff pane width minus borders (4) minus tree width, minus bar (1)
	return max(10, m.width-m.treeWidth-4-1)
}
