package ui

import (
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

	if m.collapsed.enabled {
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
// when wrap mode is active, long lines are broken at word boundaries with ↪ continuation markers.
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	hasHighlight := idx < len(m.highlightedLines)
	hlContent := ""
	if hasHighlight {
		hlContent = strings.ReplaceAll(m.highlightedLines[idx], "\t", m.tabSpaces)
	}
	lineContent := strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)

	isCursor := idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation

	// wrap mode: break long lines at word boundaries (dividers are short, skip them)
	if m.wrapMode && dl.ChangeType != diff.ChangeDivider {
		textContent := lineContent
		if hasHighlight {
			textContent = hlContent
		}
		m.renderWrappedDiffLine(b, dl, textContent, hasHighlight, isCursor)
		return
	}

	var content string
	if dl.ChangeType == diff.ChangeDivider {
		content = m.styles.LineNumber.Render(" " + lineContent)
	} else {
		textContent := lineContent
		if hasHighlight {
			textContent = hlContent
		}
		content = m.styleDiffContent(dl.ChangeType, m.linePrefix(dl.ChangeType), textContent, hasHighlight)
	}

	// apply horizontal scroll to content (bar stays fixed), disabled in wrap mode
	if m.scrollX > 0 && !m.wrapMode {
		content = ansi.Cut(content, m.scrollX, m.scrollX+m.diffContentWidth())
	}

	cursor := " "
	if isCursor {
		cursor = m.styles.DiffCursorLine.Render("▶")
	}
	b.WriteString(cursor + content + "\n")
}

// renderWrappedDiffLine renders a diff line with word wrapping, producing continuation lines with ↪ markers.
func (m Model) renderWrappedDiffLine(b *strings.Builder, dl diff.DiffLine, textContent string, hasHighlight, isCursor bool) {
	const gutterWidth = 3 // prefix width: " + ", " - ", "   ", " ↪ "
	wrapWidth := m.diffContentWidth() - gutterWidth

	visualLines := m.wrapContent(textContent, wrapWidth)
	for i, vl := range visualLines {
		prefix := " ↪ "
		if i == 0 {
			prefix = m.linePrefix(dl.ChangeType)
		}

		styled := m.styleDiffContent(dl.ChangeType, prefix, vl, hasHighlight)

		cursor := " "
		if i == 0 && isCursor {
			cursor = m.styles.DiffCursorLine.Render("▶")
		}
		b.WriteString(cursor + styled + "\n")
	}
}

// wrappedLineCount returns the number of visual rows a diff line occupies.
// returns 1 when wrap mode is off or for divider lines.
// stays in sync with renderWrappedDiffLine by using the same wrapContent method and width calculation.
func (m Model) wrappedLineCount(idx int) int {
	if !m.wrapMode || idx < 0 || idx >= len(m.diffLines) {
		return 1
	}
	dl := m.diffLines[idx]
	if dl.ChangeType == diff.ChangeDivider {
		return 1
	}

	// use highlighted content when available, matching renderDiffLine behavior
	textContent := strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)
	if idx < len(m.highlightedLines) {
		textContent = strings.ReplaceAll(m.highlightedLines[idx], "\t", m.tabSpaces)
	}

	const gutterWidth = 3 // same as renderWrappedDiffLine
	wrapWidth := m.diffContentWidth() - gutterWidth
	return len(m.wrapContent(textContent, wrapWidth))
}

// wrapContent wraps text content at the given width using word boundaries.
// returns a slice of visual lines (at least one). handles ANSI escape sequences.
func (m Model) wrapContent(content string, width int) []string {
	if width <= 0 {
		return []string{content}
	}
	wrapped := ansi.Wrap(content, width, "")
	return strings.Split(wrapped, "\n")
}

// linePrefix returns the 3-character gutter prefix for a given change type.
func (m Model) linePrefix(changeType diff.ChangeType) string {
	switch changeType {
	case diff.ChangeAdd:
		return " + "
	case diff.ChangeRemove:
		return " - "
	default:
		return "   "
	}
}

// styleDiffContent applies the appropriate line style based on change type.
func (m Model) styleDiffContent(changeType diff.ChangeType, prefix, content string, hasHighlight bool) string {
	switch changeType {
	case diff.ChangeAdd:
		if hasHighlight {
			return m.styles.LineAddHighlight.Render(prefix + content)
		}
		return m.styles.LineAdd.Render(prefix + content)
	case diff.ChangeRemove:
		if hasHighlight {
			return m.styles.LineRemoveHighlight.Render(prefix + content)
		}
		return m.styles.LineRemove.Render(prefix + content)
	default:
		if hasHighlight {
			return prefix + content
		}
		return m.styles.LineContext.Render(prefix + content)
	}
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

// centerViewportOnCursor scrolls the viewport to place the cursor in the middle of the page.
func (m *Model) centerViewportOnCursor() {
	cursorY := m.cursorViewportY()
	offset := max(0, cursorY-m.viewport.Height/2)
	m.viewport.SetYOffset(offset)
	m.viewport.SetContent(m.renderDiff())
}

const scrollStep = 4 // horizontal scroll step in characters

// handleHorizontalScroll processes left/right scroll keys.
// direction < 0 scrolls left, direction > 0 scrolls right.
// no-op when wrap mode is active (content is already fully visible).
func (m *Model) handleHorizontalScroll(direction int) {
	if m.wrapMode {
		return
	}
	if direction < 0 {
		m.scrollX = max(0, m.scrollX-scrollStep)
	} else {
		m.scrollX += scrollStep
	}
	m.viewport.SetContent(m.renderDiff())
}

// diffContentWidth returns the available width for diff line content (excluding cursor bar).
func (m Model) diffContentWidth() int {
	// diff pane width minus borders (4) minus tree width, minus bar (1)
	return max(10, m.width-m.treeWidth-4-1)
}
