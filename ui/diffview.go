package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/diff"
)

// matchRange represents a range of visible character positions for search match highlighting.
type matchRange struct{ start, end int }

// lineNumGutterWidth returns the total character width of the line number gutter.
// layout: " " + oldNum(W) + " " + newNum(W) = 2*W + 2
func (m Model) lineNumGutterWidth() int {
	return m.lineNumWidth*2 + 2
}

// lineNumGutter returns the formatted line number gutter string for a diff line.
// uses muted color via lipgloss style (m.styles.LineNumber); safe here because the gutter
// is concatenated before content, so the lipgloss reset doesn't break outer backgrounds.
// layout: " OOO NNN" where OOO is right-aligned old num, NNN is right-aligned new num.
// blank columns for adds (no old), removes (no new), and dividers (both blank).
func (m Model) lineNumGutter(dl diff.DiffLine) string {
	w := m.lineNumWidth
	blank := strings.Repeat(" ", w)

	var oldCol, newCol string
	switch dl.ChangeType {
	case diff.ChangeDivider:
		oldCol, newCol = blank, blank
	case diff.ChangeAdd:
		oldCol = blank
		newCol = fmt.Sprintf("%*d", w, dl.NewNum)
	case diff.ChangeRemove:
		oldCol = fmt.Sprintf("%*d", w, dl.OldNum)
		newCol = blank
	default: // context
		oldCol = fmt.Sprintf("%*d", w, dl.OldNum)
		newCol = fmt.Sprintf("%*d", w, dl.NewNum)
	}

	gutter := " " + oldCol + " " + newCol
	return m.styles.LineNumber.Render(gutter)
}

// renderDiff renders the current file's diff lines with styling, cursor highlight,
// and injected annotation lines.
func (m Model) renderDiff() string {
	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	if m.collapsed.enabled {
		return m.renderCollapsedDiff()
	}

	m.buildSearchMatchSet()

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
		m.renderWrappedAnnotation(b, cursor, "\U0001f4ac file: "+fileComment)
	}
}

// renderDiffLine writes a single styled diff line (with cursor highlight) to the builder.
// when wrap mode is active, long lines are broken at word boundaries with ↪ continuation markers.
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	lineContent, textContent, hasHighlight := m.prepareLineContent(idx, dl)
	isSearchMatch := m.searchMatchSet[idx]

	isCursor := idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation

	// wrap mode: break long lines at word boundaries (dividers are short, skip them)
	if m.wrapMode && dl.ChangeType != diff.ChangeDivider {
		m.renderWrappedDiffLine(b, dl, textContent, hasHighlight, isCursor, isSearchMatch)
		return
	}

	numGutter := ""
	if m.lineNumbers {
		numGutter = m.lineNumGutter(dl)
	}

	var content string
	if dl.ChangeType == diff.ChangeDivider {
		content = m.styles.LineNumber.Render(" " + lineContent)
	} else {
		content = m.styleDiffContent(dl.ChangeType, m.linePrefix(dl.ChangeType), textContent, hasHighlight, isSearchMatch)
	}

	// apply horizontal scroll to content (bar stays fixed), disabled in wrap mode
	if m.scrollX > 0 && !m.wrapMode {
		cutWidth := m.diffContentWidth()
		if m.lineNumbers {
			cutWidth -= m.lineNumGutterWidth()
		}
		if cutWidth > 0 {
			content = ansi.Cut(content, m.scrollX, m.scrollX+cutWidth)
		}
	}

	cursor := " "
	if isCursor {
		cursor = m.styles.DiffCursorLine.Render("▶")
	}
	b.WriteString(cursor + numGutter + content + "\n")
}

// renderWrappedDiffLine renders a diff line with word wrapping, producing continuation lines with ↪ markers.
func (m Model) renderWrappedDiffLine(b *strings.Builder, dl diff.DiffLine, textContent string, hasHighlight, isCursor, isSearchMatch bool) {
	gutterExtra := 0
	numGutter := ""
	numBlank := ""
	if m.lineNumbers {
		gutterExtra = m.lineNumGutterWidth()
		numGutter = m.lineNumGutter(dl)
		numBlank = strings.Repeat(" ", gutterExtra)
	}

	wrapWidth := m.diffContentWidth() - wrapGutterWidth - gutterExtra

	visualLines := m.wrapContent(textContent, wrapWidth)
	for i, vl := range visualLines {
		prefix := " ↪ "
		ng := numBlank
		if i == 0 {
			prefix = m.linePrefix(dl.ChangeType)
			ng = numGutter
		}

		styled := m.styleDiffContent(dl.ChangeType, prefix, vl, hasHighlight, isSearchMatch)

		cursor := " "
		if i == 0 && isCursor {
			cursor = m.styles.DiffCursorLine.Render("▶")
		}
		b.WriteString(cursor + ng + styled + "\n")
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

	_, textContent, _ := m.prepareLineContent(idx, dl)
	gutterExtra := 0
	if m.lineNumbers {
		gutterExtra = m.lineNumGutterWidth()
	}
	wrapWidth := m.diffContentWidth() - wrapGutterWidth - gutterExtra
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

// prepareLineContent returns the display-ready content for a diff line with tabs replaced.
// returns the raw line content, the best available content (highlighted if available), and whether highlight was used.
func (m Model) prepareLineContent(idx int, dl diff.DiffLine) (lineContent, textContent string, hasHighlight bool) {
	lineContent = strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)
	hasHighlight = idx < len(m.highlightedLines)
	textContent = lineContent
	if hasHighlight {
		textContent = strings.ReplaceAll(m.highlightedLines[idx], "\t", m.tabSpaces)
	}
	return lineContent, textContent, hasHighlight
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

// highlightSearchMatches wraps each occurrence of the search term in the visible text
// with ANSI background color sequence (preserving syntax foreground within matches).
// works with both plain text and ANSI-coded content by stripping ANSI to find match positions.
func (m Model) highlightSearchMatches(s string) string {
	if m.searchTerm == "" {
		return s
	}

	// find match positions in visible (ANSI-stripped) text
	plain := ansi.Strip(s)
	plainLower := strings.ToLower(plain)
	term := strings.ToLower(m.searchTerm)
	if !strings.Contains(plainLower, term) {
		return s
	}

	// collect all match ranges in visible-character positions
	var matches []matchRange
	offset := 0
	for {
		idx := strings.Index(plainLower[offset:], term)
		if idx < 0 {
			break
		}
		start := offset + idx
		matches = append(matches, matchRange{start, start + len(term)})
		offset = start + len(term)
	}
	if len(matches) == 0 {
		return s
	}

	// background-only highlight preserves syntax foreground colors within matches
	hlOn := m.ansiBg(m.styles.colors.SearchBg)
	hlOff := "\033[49m"
	if hlOn == "" {
		// no-colors mode: fall back to reverse video so matches remain visible
		hlOn = "\033[7m"
		hlOff = "\033[27m"
	}

	return m.insertHighlightMarkers(s, matches, hlOn, hlOff)
}

// insertHighlightMarkers walks the string inserting hlOn/hlOff ANSI sequences at match positions,
// skipping over existing ANSI escape sequences to preserve them.
func (m Model) insertHighlightMarkers(s string, matches []matchRange, hlOn, hlOff string) string {
	var b strings.Builder
	visPos := 0   // current position in visible text
	matchIdx := 0 // current match we're processing
	i := 0

	for i < len(s) {
		// skip ANSI escape sequences (copy them as-is)
		if s[i] == '\033' {
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // include the 'm'
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}

		// insert highlight start/end at match boundaries
		if matchIdx < len(matches) && visPos == matches[matchIdx].start {
			b.WriteString(hlOn)
		}
		if matchIdx < len(matches) && visPos == matches[matchIdx].end {
			b.WriteString(hlOff)
			matchIdx++
			if matchIdx < len(matches) && visPos == matches[matchIdx].start {
				b.WriteString(hlOn)
			}
		}

		b.WriteByte(s[i])
		visPos++
		i++
	}

	// close any unclosed highlight
	if matchIdx < len(matches) && visPos >= matches[matchIdx].start && visPos <= matches[matchIdx].end {
		b.WriteString(hlOff)
	}

	return b.String()
}

// styleDiffContent applies the appropriate line style based on change type.
func (m Model) styleDiffContent(changeType diff.ChangeType, prefix, content string, hasHighlight, isSearchMatch bool) string {
	if isSearchMatch && m.searchTerm != "" {
		content = m.highlightSearchMatches(content)
	}

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
			m.renderWrappedAnnotation(b, cursor, "\U0001f4ac "+comment)
		}
	}
}

// renderWrappedAnnotation writes an annotation line with word wrapping.
// annotations always wrap regardless of wrapMode since they contain prose.
func (m Model) renderWrappedAnnotation(b *strings.Builder, cursor, text string) {
	wrapWidth := m.diffContentWidth() - 1 // 1 for cursor column

	if wrapWidth > 10 && lipgloss.Width(text) > wrapWidth {
		lines := m.wrapContent(text, wrapWidth)
		for i, line := range lines {
			c := " " // continuation lines get space instead of cursor
			if i == 0 {
				c = cursor
			}
			b.WriteString(c + m.styles.AnnotationLine.Render(line) + "\n")
		}
		return
	}

	b.WriteString(cursor + m.styles.AnnotationLine.Render(text) + "\n")
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

// moveDiffCursorHalfPageDown moves the diff cursor down by half a visual page.
// scrolls viewport by half page explicitly, matching vim/less ctrl+d behavior.
func (m *Model) moveDiffCursorHalfPageDown() {
	halfPage := max(1, m.viewport.Height/2)
	startY := m.cursorViewportY()
	for {
		prev := m.diffCursor
		m.moveDiffCursorDown()
		if m.diffCursor == prev {
			break
		}
		if m.cursorViewportY()-startY >= halfPage {
			break
		}
	}
	maxOffset := max(0, m.viewport.TotalLineCount()-m.viewport.Height)
	m.viewport.SetYOffset(min(m.viewport.YOffset+halfPage, maxOffset))
	m.viewport.SetContent(m.renderDiff())
}

// moveDiffCursorHalfPageUp moves the diff cursor up by half a visual page.
// scrolls viewport by half page explicitly, matching vim/less ctrl+u behavior.
func (m *Model) moveDiffCursorHalfPageUp() {
	halfPage := max(1, m.viewport.Height/2)
	startY := m.cursorViewportY()
	for {
		prev := m.diffCursor
		m.moveDiffCursorUp()
		if m.diffCursor == prev {
			break
		}
		if startY-m.cursorViewportY() >= halfPage {
			break
		}
	}
	m.viewport.SetYOffset(max(0, m.viewport.YOffset-halfPage))
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

// topAlignViewportOnCursor scrolls the viewport to place the cursor at the top of the page.
func (m *Model) topAlignViewportOnCursor() {
	cursorY := m.cursorViewportY()
	m.viewport.SetYOffset(max(0, cursorY))
	m.viewport.SetContent(m.renderDiff())
}

const wrapGutterWidth = 3 // wrap gutter prefix width: " + ", " - ", "   ", " ↪ "
const scrollStep = 4      // horizontal scroll step in characters

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
	if m.treeHidden || (m.singleFile && m.mdTOC == nil) {
		// tree hidden or single-file without TOC: diff pane borders (2) + cursor bar (1)
		return max(10, m.width-3)
	}
	// multi-file or single-file with TOC: diff pane width minus borders (4) minus tree width, minus bar (1)
	return max(10, m.width-m.treeWidth-4-1)
}
