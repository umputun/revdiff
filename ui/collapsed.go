package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/diff"
)

// collapsedState holds the state for collapsed diff view mode.
// collapsed mode shows the final text with color markers on changed lines,
// hiding removed lines unless their hunk is explicitly expanded.
type collapsedState struct {
	enabled       bool         // true when viewing collapsed diff (final text only)
	expandedHunks map[int]bool // hunks expanded inline, key = diffLines start index from findHunks()
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
	hunkIdx := 0
	for i, dl := range m.diffLines {
		// advance hunk tracker to the last hunk that starts at or before i
		for hunkIdx+1 < len(hunks) && hunks[hunkIdx+1] <= i {
			hunkIdx++
		}
		hunkStart := -1
		isChange := dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove
		if isChange && len(hunks) > 0 && hunks[hunkIdx] <= i {
			hunkStart = hunks[hunkIdx]
		}
		expanded := hunkStart >= 0 && m.collapsed.expandedHunks[hunkStart]

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
	m.collapsed.enabled = !m.collapsed.enabled
	m.collapsed.expandedHunks = make(map[int]bool)
	m.cursorOnAnnotation = false // visible lines change, reset annotation cursor state
	m.adjustCursorIfHidden()
	m.viewport.SetContent(m.renderDiff())
}

// toggleHunkExpansion toggles the expansion state of the hunk under the cursor.
// only operates in collapsed mode; no-op in expanded mode or when cursor is not on a hunk.
func (m *Model) toggleHunkExpansion() {
	if !m.collapsed.enabled {
		return
	}
	hunkStart, ok := m.cursorHunkStart()
	if !ok {
		return
	}
	if m.collapsed.expandedHunks[hunkStart] {
		delete(m.collapsed.expandedHunks, hunkStart)
		m.cursorOnAnnotation = false // annotations on removed lines become invisible
		m.adjustCursorIfHidden()
	} else {
		m.collapsed.expandedHunks[hunkStart] = true
	}
	m.viewport.SetContent(m.renderDiff())
}

// isCollapsedHidden returns true if the line at idx is hidden in collapsed mode.
// a line is hidden when collapsed mode is active, the line is a remove line,
// and its hunk is not expanded. the first line of a delete-only hunk is kept
// visible as a placeholder so users can navigate to it and expand with '.'.
func (m Model) isCollapsedHidden(idx int, hunks []int) bool {
	if !m.collapsed.enabled || idx < 0 || idx >= len(m.diffLines) {
		return false
	}
	if m.diffLines[idx].ChangeType != diff.ChangeRemove {
		return false
	}
	hunkStart := m.hunkStartFor(idx, hunks)
	if hunkStart < 0 {
		return true
	}
	if m.collapsed.expandedHunks[hunkStart] {
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
	if !m.collapsed.enabled {
		return false
	}
	if idx < 0 || idx >= len(m.diffLines) || m.diffLines[idx].ChangeType != diff.ChangeRemove {
		return false
	}
	hunkStart := m.hunkStartFor(idx, hunks)
	return hunkStart >= 0 && idx == hunkStart && !m.collapsed.expandedHunks[hunkStart] && m.isDeleteOnlyHunk(hunkStart)
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
	if !m.collapsed.enabled || m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
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
