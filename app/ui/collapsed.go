package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
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
	m.search.matchSet = m.buildSearchMatchSet()

	annotationMap, fileComment := m.buildAnnotationMap()
	hunks := m.findHunks()
	modifiedSet := m.buildModifiedSet(hunks)

	var b strings.Builder
	m.renderFileAnnotationHeader(&b, fileComment)

	hasVisibleContent := false
	hunkIdx := 0
	for i, dl := range m.file.lines {
		// advance hunk tracker to the last hunk that starts at or before i
		for hunkIdx+1 < len(hunks) && hunks[hunkIdx+1] <= i {
			hunkIdx++
		}
		hunkStart := -1
		isChange := dl.ChangeType == diff.ChangeAdd || dl.ChangeType == diff.ChangeRemove
		if isChange && len(hunks) > 0 && hunks[hunkIdx] <= i {
			hunkStart = hunks[hunkIdx]
		}
		expanded := hunkStart >= 0 && m.modes.collapsed.expandedHunks[hunkStart]

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
// when search is active, matching lines use search highlight instead of add/modify styling.
func (m Model) renderCollapsedAddLine(b *strings.Builder, idx int, dl diff.DiffLine, modified bool) {
	lineContent, textContent, hasHighlight := m.prepareLineContent(idx, dl)
	isSearchMatch := m.search.matchSet[idx]

	lineStyle := m.resolver.Style(style.StyleKeyLineAdd)
	lineHlStyle := m.resolver.Style(style.StyleKeyLineAddHighlight)
	gutter := " + "
	if modified {
		lineStyle = m.resolver.Style(style.StyleKeyLineModify)
		lineHlStyle = m.resolver.Style(style.StyleKeyLineModifyHighlight)
		gutter = " ~ "
	}
	if isSearchMatch {
		sm := m.resolver.Style(style.StyleKeySearchMatch)
		lineStyle = sm
		lineHlStyle = sm.UnsetForeground()
	}

	isCursor := m.isCursorLine(idx)

	numGutter, blGutter := m.lineGutters(dl)

	bgColor := m.resolver.Color(style.ColorKeyAddLineBg)
	prefixFg := m.resolver.LineFg(diff.ChangeAdd)
	if modified {
		bgColor = m.resolver.Color(style.ColorKeyModifyLineBg)
		prefixFg = m.resolver.Color(style.ColorKeyModifyLineFg)
	}

	// wrap mode: break long lines at word boundaries with continuation markers
	if m.modes.wrap {
		m.renderWrappedCollapsedLine(b, textContent, wrappedLineCtx{
			gutter: gutter, numGutter: numGutter, blGutter: blGutter,
			isCursor: isCursor, hasHighlight: hasHighlight,
			lineStyle: lineStyle, hlStyle: lineHlStyle, bgColor: bgColor,
			prefixFg: prefixFg,
		})
		return
	}

	content := lineStyle.Render(gutter + lineContent)
	if hasHighlight {
		content = lineHlStyle.Render(m.wrapPrefixForHighlight(gutter, prefixFg, true) + textContent)
	}
	content = m.applyHorizontalScroll(content, bgColor)
	content = m.extendLineBg(content, bgColor)

	cursor := " "
	if isCursor {
		cursor = m.renderer.DiffCursor(m.cfg.noColors)
	}
	b.WriteString(cursor + numGutter + blGutter + content + "\n")
}

// wrappedLineCtx holds rendering context for a wrapped collapsed line,
// reducing the parameter count of renderWrappedCollapsedLine.
type wrappedLineCtx struct {
	gutter, numGutter, blGutter string
	isCursor, hasHighlight      bool
	lineStyle, hlStyle          lipgloss.Style
	bgColor, prefixFg           style.Color
}

// renderWrappedCollapsedLine renders a collapsed add line with word wrapping, producing continuation lines with ↪ markers.
func (m Model) renderWrappedCollapsedLine(b *strings.Builder, textContent string, ctx wrappedLineCtx) {
	numBlank, blBlank := m.gutterBlanks()
	visualLines := m.wrapContent(textContent, m.wrapWidth())
	for i, vl := range visualLines {
		prefix := " ↪ "
		ng := numBlank
		bg := blBlank
		if i == 0 {
			prefix = ctx.gutter
			ng = ctx.numGutter
			bg = ctx.blGutter
		}

		var styled string
		if ctx.hasHighlight {
			styled = ctx.hlStyle.Render(m.wrapPrefixForHighlight(prefix, ctx.prefixFg, true) + vl)
		} else {
			styled = ctx.lineStyle.Render(prefix + vl)
		}
		styled = m.extendLineBg(styled, ctx.bgColor)

		cursor := " "
		if i == 0 && ctx.isCursor {
			cursor = m.renderer.DiffCursor(m.cfg.noColors)
		}
		b.WriteString(cursor + ng + bg + styled + "\n")
	}
}

// deletePlaceholderText returns the text shown for a delete-only hunk placeholder starting at hunkStart.
// used by both renderDeletePlaceholder and cursorViewportY to stay in sync.
func (m Model) deletePlaceholderText(hunkStart int) string {
	count := 0
	for i := hunkStart; i < len(m.file.lines); i++ {
		ct := m.file.lines[i].ChangeType
		if ct == diff.ChangeContext || ct == diff.ChangeDivider {
			break
		}
		if ct == diff.ChangeRemove {
			count++
		}
	}
	if count == 1 {
		return "⋯ 1 line deleted"
	}
	return fmt.Sprintf("⋯ %d lines deleted", count)
}

// deletePlaceholderVisualHeight returns the number of visual rows a delete-only placeholder
// occupies, accounting for wrap mode and gutter widths.
func (m Model) deletePlaceholderVisualHeight(hunkStart int) int {
	if !m.modes.wrap {
		return 1
	}
	text := m.deletePlaceholderText(hunkStart)
	return len(m.wrapContent(text, m.wrapWidth()))
}

// renderDeletePlaceholder renders a placeholder line for a delete-only hunk in collapsed mode.
// shows "⋯ N lines deleted" with remove styling so users know deletions exist and can expand with '.'.
// when search is active, matching placeholders use search highlight instead of remove styling.
func (m Model) renderDeletePlaceholder(b *strings.Builder, idx, hunkStart int) {
	text := m.deletePlaceholderText(hunkStart)

	lineStyle := m.resolver.Style(style.StyleKeyLineRemove)
	if m.search.matchSet[idx] {
		lineStyle = m.resolver.Style(style.StyleKeySearchMatch)
	}
	removeBg := m.resolver.Color(style.ColorKeyRemoveLineBg)

	isCursor := m.isCursorLine(idx)

	divider := diff.DiffLine{ChangeType: diff.ChangeDivider}
	numGutter, blGutter := m.lineGutters(divider)

	// wrap mode: break long placeholder at word boundaries
	if m.modes.wrap {
		numBlank, blBlank := m.gutterBlanks()
		visualLines := m.wrapContent(text, m.wrapWidth())
		for i, vl := range visualLines {
			prefix := " ↪ "
			ng := numBlank
			bg := blBlank
			if i == 0 {
				prefix = " - "
				ng = numGutter
				bg = blGutter
			}
			styled := lineStyle.Render(prefix + vl)
			styled = m.extendLineBg(styled, removeBg)

			cursor := " "
			if i == 0 && isCursor {
				cursor = m.renderer.DiffCursor(m.cfg.noColors)
			}
			b.WriteString(cursor + ng + bg + styled + "\n")
		}
		return
	}

	content := lineStyle.Render(" - " + text)
	content = m.applyHorizontalScroll(content, removeBg)
	content = m.extendLineBg(content, removeBg)

	cursor := " "
	if isCursor {
		cursor = m.renderer.DiffCursor(m.cfg.noColors)
	}
	b.WriteString(cursor + numGutter + blGutter + content + "\n")
}

// hunkStartFor returns the findHunks() start index for the hunk containing diffLines[idx].
// returns -1 if the index is not inside any hunk (context or divider line).
func (m Model) hunkStartFor(idx int, hunks []int) int {
	if len(hunks) == 0 || idx < 0 || idx >= len(m.file.lines) {
		return -1
	}
	dl := m.file.lines[idx]
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
	n := len(m.file.lines)

	for hi, start := range hunks {
		// find the end of this hunk: next hunk start or first non-change line
		end := n
		if hi+1 < len(hunks) {
			end = hunks[hi+1]
		}
		// scan only contiguous change lines from start
		for end > start && (m.file.lines[end-1].ChangeType != diff.ChangeAdd &&
			m.file.lines[end-1].ChangeType != diff.ChangeRemove) {
			end--
		}

		// build LinePair slice and use PairLines to detect mixed hunks.
		// if pairs exist, mark all add lines in the block as modified.
		block := make([]worddiff.LinePair, end-start)
		for j := start; j < end; j++ {
			block[j-start] = worddiff.LinePair{
				Content:  m.file.lines[j].Content,
				IsRemove: m.file.lines[j].ChangeType == diff.ChangeRemove,
			}
		}
		pairs := m.differ.PairLines(block)
		if len(pairs) > 0 {
			for i := start; i < end; i++ {
				if m.file.lines[i].ChangeType == diff.ChangeAdd {
					result[i] = true
				}
			}
		}
	}
	return result
}

// cursorHunkStart returns the findHunks() start index for the hunk containing the cursor.
// returns false if the cursor is not inside any hunk.
func (m Model) cursorHunkStart() (int, bool) {
	hunks := m.findHunks()
	best := m.hunkStartFor(m.nav.diffCursor, hunks)
	if best < 0 {
		return 0, false
	}
	return best, true
}

// toggleCollapsedMode switches between collapsed and expanded diff view.
// only operates when the diff pane is focused and a file is loaded.
func (m *Model) toggleCollapsedMode() {
	if m.layout.focus != paneDiff || m.file.name == "" {
		return
	}
	m.modes.collapsed.enabled = !m.modes.collapsed.enabled
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	m.annot.cursorOnAnnotation = false // visible lines change, reset annotation cursor state
	m.adjustCursorIfHidden()
	m.realignSearchCursor()
	m.layout.viewport.SetContent(m.renderDiff())
}

// toggleHunkExpansion toggles the expansion state of the hunk under the cursor.
// only operates in collapsed mode; no-op in expanded mode or when cursor is not on a hunk.
func (m *Model) toggleHunkExpansion() {
	if !m.modes.collapsed.enabled {
		return
	}
	hunkStart, ok := m.cursorHunkStart()
	if !ok {
		return
	}
	if m.modes.collapsed.expandedHunks[hunkStart] {
		delete(m.modes.collapsed.expandedHunks, hunkStart)
		m.annot.cursorOnAnnotation = false // annotations on removed lines become invisible
		m.adjustCursorIfHidden()
		m.realignSearchCursor()
	} else {
		m.modes.collapsed.expandedHunks[hunkStart] = true
	}
	m.layout.viewport.SetContent(m.renderDiff())
}

// isCollapsedHidden returns true if the line at idx is hidden in collapsed mode.
// a line is hidden when collapsed mode is active, the line is a remove line,
// and its hunk is not expanded. the first line of a delete-only hunk is kept
// visible as a placeholder so users can navigate to it and expand with '.'.
func (m Model) isCollapsedHidden(idx int, hunks []int) bool {
	if !m.modes.collapsed.enabled || idx < 0 || idx >= len(m.file.lines) {
		return false
	}
	if m.file.lines[idx].ChangeType != diff.ChangeRemove {
		return false
	}
	hunkStart := m.hunkStartFor(idx, hunks)
	if hunkStart < 0 {
		return true
	}
	if m.modes.collapsed.expandedHunks[hunkStart] {
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
	if !m.modes.collapsed.enabled {
		return false
	}
	if idx < 0 || idx >= len(m.file.lines) || m.file.lines[idx].ChangeType != diff.ChangeRemove {
		return false
	}
	hunkStart := m.hunkStartFor(idx, hunks)
	return hunkStart >= 0 && idx == hunkStart && !m.modes.collapsed.expandedHunks[hunkStart] && m.isDeleteOnlyHunk(hunkStart)
}

// isDeleteOnlyHunk returns true if the hunk starting at hunkStart contains only remove lines.
func (m Model) isDeleteOnlyHunk(hunkStart int) bool {
	for i := hunkStart; i < len(m.file.lines); i++ {
		ct := m.file.lines[i].ChangeType
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
	for i := hunkStart + 1; i < len(m.file.lines); i++ {
		if m.file.lines[i].ChangeType == diff.ChangeDivider || m.file.lines[i].ChangeType == diff.ChangeContext {
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
	if !m.modes.collapsed.enabled || m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return
	}
	hunks := m.findHunks()
	if !m.isCollapsedHidden(m.nav.diffCursor, hunks) {
		return
	}
	// search forward for nearest visible non-divider line
	for i := m.nav.diffCursor + 1; i < len(m.file.lines); i++ {
		if m.file.lines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.nav.diffCursor = i
			return
		}
	}
	// search backward for nearest visible non-divider line
	for i := m.nav.diffCursor - 1; i >= 0; i-- {
		if m.file.lines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
			m.nav.diffCursor = i
			return
		}
	}
	// no visible content line found (delete-only file); fall back to nearest divider
	for i := m.nav.diffCursor + 1; i < len(m.file.lines); i++ {
		if m.file.lines[i].ChangeType == diff.ChangeDivider {
			m.nav.diffCursor = i
			return
		}
	}
	for i := m.nav.diffCursor - 1; i >= 0; i-- {
		if m.file.lines[i].ChangeType == diff.ChangeDivider {
			m.nav.diffCursor = i
			return
		}
	}
}
