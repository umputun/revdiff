package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/diff"
)

// matchRange represents a range of visible character positions for search match highlighting.
type matchRange struct{ start, end int }

// rangeRenderInfo maps a range annotation to its diffLines indices for rendering.
type rangeRenderInfo struct {
	startIdx  int    // first visible diffLines index in the range
	endIdx    int    // last visible diffLines index in the range
	startLine int    // annotation start line number
	endLine   int    // annotation end line number
	comment   string // annotation comment
}

// rangeGutterWidth is the character width of the range annotation gutter indicator.
const rangeGutterWidth = 2

// rangeAnnotationPrefix returns the formatted prefix for range annotation display.
func rangeAnnotationPrefix(startLine, endLine int) string {
	return fmt.Sprintf("\U0001f4ac [lines %d-%d] ", startLine, endLine)
}

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

// blameGutterWidth returns the total character width of the blame gutter.
// layout: " " + author(W) + " " + age(3) = W + 5
func (m Model) blameGutterWidth() int {
	return m.blameAuthorLen + 5
}

// blameGutter returns the formatted blame gutter string for a diff line.
// shows author name (truncated) and relative age for lines with NewNum; blank for removed lines and dividers.
func (m Model) blameGutter(dl diff.DiffLine) string {
	w := m.blameAuthorLen
	totalW := m.blameGutterWidth()
	blank := strings.Repeat(" ", totalW)

	lineNum := dl.NewNum
	if lineNum == 0 || dl.ChangeType == diff.ChangeDivider {
		return m.styles.LineNumber.Render(blank)
	}

	bl, ok := m.blameData[lineNum]
	if !ok {
		return m.styles.LineNumber.Render(blank)
	}

	author := runewidth.Truncate(bl.Author, w, "…")
	pad := w - runewidth.StringWidth(author)
	if pad > 0 {
		author += strings.Repeat(" ", pad)
	}

	age := diff.RelativeAge(bl.Time, m.blameNow)
	gutter := " " + author + " " + age
	return m.styles.LineNumber.Render(gutter)
}

// hasBlameGutter returns true when the blame gutter should be rendered.
func (m Model) hasBlameGutter() bool {
	return m.showBlame && len(m.blameData) > 0
}

// lineGutters returns the formatted line number and blame gutter strings for a diff line.
// returns empty strings for disabled gutters.
func (m Model) lineGutters(dl diff.DiffLine) (numGutter, blameGutter string) {
	if m.lineNumbers {
		numGutter = m.lineNumGutter(dl)
	}
	if m.hasBlameGutter() {
		blameGutter = m.blameGutter(dl)
	}
	return numGutter, blameGutter
}

// gutterExtra returns the total character width consumed by enabled gutters (line numbers + blame).
func (m Model) gutterExtra() int {
	w := 0
	if m.lineNumbers {
		w += m.lineNumGutterWidth()
	}
	if m.hasBlameGutter() {
		w += m.blameGutterWidth()
	}
	return w
}

// gutterBlanks returns blank strings matching the widths of enabled gutters,
// used as padding for wrap continuation lines.
func (m Model) gutterBlanks() (numBlank, blameBlank string) {
	if m.lineNumbers {
		numBlank = strings.Repeat(" ", m.lineNumGutterWidth())
	}
	if m.hasBlameGutter() {
		blameBlank = strings.Repeat(" ", m.blameGutterWidth())
	}
	return numBlank, blameBlank
}

// applyHorizontalScroll applies horizontal scroll offset to content, subtracting gutter widths.
// no-op when scroll offset is zero.
func (m Model) applyHorizontalScroll(content string) string {
	if m.scrollX <= 0 {
		return content
	}
	cutWidth := m.diffContentWidth() - m.gutterExtra()
	if cutWidth > 0 {
		return ansi.Cut(content, m.scrollX, m.scrollX+cutWidth)
	}
	return content
}

// renderDiff renders the current file's diff lines with styling, cursor highlight,
// and injected annotation lines.
func (m Model) renderDiff() string {
	if len(m.diffLines) == 0 {
		return "  no changes"
	}

	m.blameNow = time.Now()

	if m.collapsed.enabled {
		return m.renderCollapsedDiff()
	}

	m.buildSearchMatchSet()

	annotationMap, fileComment := m.buildAnnotationMap()
	ranges := m.buildRangeAnnotations()
	ranges = m.appendSelectionRange(ranges)
	var b strings.Builder
	m.renderFileAnnotationHeader(&b, fileComment)

	for i, dl := range m.diffLines {
		m.renderDiffLine(&b, i, dl, ranges)
		m.renderAnnotationOrInput(&b, i, annotationMap)
		m.renderRangeAnnotation(&b, i, ranges)
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
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine, ranges []rangeRenderInfo) {
	lineContent, textContent, hasHighlight := m.prepareLineContent(idx, dl)
	isSearchMatch := m.searchMatchSet[idx]

	isCursor := idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation
	// wrap mode: break long lines at word boundaries (dividers are short, skip them)
	if m.wrapMode && dl.ChangeType != diff.ChangeDivider {
		m.renderWrappedDiffLine(b, dl, textContent, hasHighlight, isCursor, isSearchMatch, ranges, idx)
		return
	}

	numGutter, blGutter := m.lineGutters(dl)
	rg := m.styledRangeGutter(idx, ranges)

	var content string
	if dl.ChangeType == diff.ChangeDivider {
		content = m.styles.LineNumber.Render(" " + lineContent)
	} else {
		content = m.styleDiffContent(dl.ChangeType, m.linePrefix(dl.ChangeType), textContent, hasHighlight, isSearchMatch)
	}

	content = m.applyHorizontalScroll(content)

	cursor := " "
	if isCursor {
		cursor = m.styles.DiffCursorLine.Render("▶")
	}
	b.WriteString(cursor + rg + numGutter + blGutter + content + "\n")
}

// styledRangeGutter returns the styled range gutter indicator for a diffLines index.
func (m Model) styledRangeGutter(idx int, ranges []rangeRenderInfo) string {
	gutter := rangeGutterFor(idx, ranges)
	if gutter == "" {
		if len(ranges) > 0 {
			return strings.Repeat(" ", rangeGutterWidth)
		}
		return ""
	}
	return m.styles.AnnotationLine.Render(gutter)
}

// renderRangeAnnotation writes the range annotation comment or input below the last line of a range.
func (m Model) renderRangeAnnotation(b *strings.Builder, idx int, ranges []rangeRenderInfo) {
	// render range annotation input below the end line
	if m.annotating && m.rangeStartLine > 0 && idx == m.diffCursor {
		prefix := rangeAnnotationPrefix(m.rangeStartLine, m.rangeEndLine)
		b.WriteString(" " + m.styles.AnnotationLine.Render(prefix) + m.annotateInput.View() + "\n")
		return
	}

	for _, r := range ranges {
		if r.endIdx != idx || r.comment == "" {
			continue
		}
		prefix := rangeAnnotationPrefix(r.startLine, r.endLine)
		cursor := " "
		if idx == m.diffCursor && m.cursorOnAnnotation && m.cursorOnRangeAnnotation && m.focus == paneDiff {
			cursor = m.styles.DiffCursorLine.Render("▶")
		}
		m.renderWrappedAnnotation(b, cursor, prefix+r.comment)
		return
	}
}

// hasRangeGutter returns true if range gutter indicators are currently shown.
func (m Model) hasRangeGutter() bool {
	if m.selecting || (m.annotating && m.rangeStartLine > 0) {
		return true
	}
	for _, a := range m.store.Get(m.currFile) {
		if a.IsRange() {
			return true
		}
	}
	return false
}

// appendSelectionRange adds a live selection indicator to the ranges slice.
// shown during active selection and while typing a range annotation input.
func (m Model) appendSelectionRange(ranges []rangeRenderInfo) []rangeRenderInfo {
	if m.selecting {
		start, end := m.selectionBounds()
		if start != end {
			return append(ranges, rangeRenderInfo{
				startIdx:  start,
				endIdx:    end,
				startLine: m.diffLineNum(m.diffLines[start]),
				endLine:   m.diffLineNum(m.diffLines[end]),
			})
		}
		return ranges
	}
	// keep gutter visible while editing range annotation (diffCursor is at range end)
	if m.annotating && m.rangeStartLine > 0 {
		if m.rangeStartIdx >= 0 && m.rangeEndIdx >= m.rangeStartIdx {
			return append(ranges, rangeRenderInfo{
				startIdx:  m.rangeStartIdx,
				endIdx:    m.rangeEndIdx,
				startLine: m.rangeStartLine,
				endLine:   m.rangeEndLine,
			})
		}
	}
	return ranges
}

// renderWrappedDiffLine renders a diff line with word wrapping, producing continuation lines with ↪ markers.
func (m Model) renderWrappedDiffLine(b *strings.Builder, dl diff.DiffLine, textContent string, hasHighlight, isCursor, isSearchMatch bool, ranges []rangeRenderInfo, idx int) {
	numGutter, blGutter := m.lineGutters(dl)
	numBlank, blBlank := m.gutterBlanks()
	rg := m.styledRangeGutter(idx, ranges)
	rgBlank := ""
	if len(ranges) > 0 {
		rgBlank = strings.Repeat(" ", rangeGutterWidth)
	}
	wrapWidth := m.diffContentWidth() - wrapGutterWidth - m.gutterExtra()
	if len(ranges) > 0 {
		wrapWidth -= rangeGutterWidth
	}

	visualLines := m.wrapContent(textContent, wrapWidth)
	for i, vl := range visualLines {
		prefix := " ↪ "
		ng := numBlank
		bg := blBlank
		rug := rgBlank
		if i == 0 {
			prefix = m.linePrefix(dl.ChangeType)
			ng = numGutter
			bg = blGutter
			rug = rg
		}

		styled := m.styleDiffContent(dl.ChangeType, prefix, vl, hasHighlight, isSearchMatch)

		cursor := " "
		if i == 0 && isCursor {
			cursor = m.styles.DiffCursorLine.Render("▶")
		}
		b.WriteString(cursor + rug + ng + bg + styled + "\n")
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
	wrapWidth := m.diffContentWidth() - wrapGutterWidth - m.gutterExtra()
	if m.hasRangeGutter() {
		wrapWidth -= rangeGutterWidth
	}
	return len(m.wrapContent(textContent, wrapWidth))
}

// visibleRangeIndices maps an annotation line range to visible diff line indices.
func (m Model) visibleRangeIndices(startLine, endLine int) (int, int) {
	startIdx := -1
	endIdx := -1
	hunks := m.findHunks()
	for i, dl := range m.diffLines {
		if dl.ChangeType == diff.ChangeDivider {
			if startIdx != -1 {
				break // stop at hunk boundary once range has started
			}
			continue
		}
		if m.collapsed.enabled && m.isCollapsedHidden(i, hunks) {
			continue
		}

		lineNum := m.diffLineNum(dl)
		inRange := lineNum >= startLine && lineNum <= endLine
		if startIdx == -1 {
			if !inRange {
				continue
			}
			startIdx = i
			endIdx = i
			continue
		}

		if !inRange {
			break
		}

		// in collapsed mode, stop if hidden lines exist between last match and current line.
		// this prevents a range on hidden removed lines from spilling into context lines
		// that happen to share the same display line numbers (OldNum vs NewNum aliasing).
		if m.collapsed.enabled {
			for j := endIdx + 1; j < i; j++ {
				if m.isCollapsedHidden(j, hunks) {
					return startIdx, endIdx
				}
			}
		}

		endIdx = i
	}
	return startIdx, endIdx
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
// for add/remove lines, extends the background to the full content width so padContentBg
// at the View level doesn't replace it with DiffBg.
func (m Model) styleDiffContent(changeType diff.ChangeType, prefix, content string, hasHighlight, isSearchMatch bool) string {
	if isSearchMatch && m.searchTerm != "" {
		content = m.highlightSearchMatches(content)
	}

	switch changeType {
	case diff.ChangeAdd:
		if hasHighlight {
			return m.extendLineBg(m.styles.LineAddHighlight.Render(prefix+content), m.styles.colors.AddBg)
		}
		return m.extendLineBg(m.styles.LineAdd.Render(prefix+content), m.styles.colors.AddBg)
	case diff.ChangeRemove:
		if hasHighlight {
			return m.extendLineBg(m.styles.LineRemoveHighlight.Render(prefix+content), m.styles.colors.RemoveBg)
		}
		return m.extendLineBg(m.styles.LineRemove.Render(prefix+content), m.styles.colors.RemoveBg)
	default:
		if hasHighlight {
			return m.styles.LineContextHighlight.Render(prefix + content)
		}
		return m.styles.LineContext.Render(prefix + content)
	}
}

// buildRangeAnnotations maps range annotations to their diffLines indices.
func (m Model) buildRangeAnnotations() []rangeRenderInfo {
	var ranges []rangeRenderInfo
	for _, a := range m.store.Get(m.currFile) {
		if !a.IsRange() {
			continue
		}
		startIdx, endIdx := m.visibleRangeIndices(a.Line, a.EndLine)
		if startIdx >= 0 && endIdx >= 0 {
			ranges = append(ranges, rangeRenderInfo{
				startIdx:  startIdx,
				endIdx:    endIdx,
				startLine: a.Line,
				endLine:   a.EndLine,
				comment:   a.Comment,
			})
		}
	}
	return ranges
}

// rangeGutterFor returns the gutter indicator for a diffLines index within range annotations.
// returns "┌ ", "│ ", "└ ", or "" depending on position within the range.
func rangeGutterFor(idx int, ranges []rangeRenderInfo) string {
	for _, r := range ranges {
		if r.startIdx == r.endIdx && idx == r.startIdx {
			return "│ "
		}
		if idx == r.startIdx {
			return "┌ "
		}
		if idx == r.endIdx {
			return "└ "
		}
		if idx > r.startIdx && idx < r.endIdx {
			return "│ "
		}
	}
	return ""
}

// extendLineBg extends a styled line's background to the full diff content width
// using raw ANSI sequences. this ensures add/remove/modify backgrounds fill the entire line.
// subtracts line number gutter width when line numbers are enabled.
func (m Model) extendLineBg(styled, bgColor string) string {
	if bgColor == "" {
		return styled
	}
	// target = content area minus cursor bar (1) minus gutters (if on)
	// diffContentWidth() already excludes cursor bar; subtract gutters if enabled
	targetWidth := m.diffContentWidth() - m.gutterExtra()
	// leave 1 char gap before right border
	targetWidth--
	currentWidth := lipgloss.Width(styled)
	if pad := targetWidth - currentWidth; pad > 0 {
		return styled + m.ansiBg(bgColor) + strings.Repeat(" ", pad) + "\033[49m"
	}
	return styled
}

// renderAnnotationOrInput writes the annotation input or existing annotation below a diff line.
func (m Model) renderAnnotationOrInput(b *strings.Builder, idx int, annotationMap map[string]string) {
	if m.annotating && !m.fileAnnotating && m.rangeStartLine == 0 && idx == m.diffCursor {
		b.WriteString(" " + m.styles.AnnotationLine.Render("\U0001f4ac ") + m.annotateInput.View() + "\n")
		return
	}
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
		if comment, ok := annotationMap[key]; ok {
			cursor := " "
			if idx == m.diffCursor && m.cursorOnAnnotation && !m.cursorOnRangeAnnotation && m.focus == paneDiff {
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

	// if currently on annotation sub-line, advance to the next sub-row or diff line
	if m.cursorOnAnnotation {
		// on point sub-row: if a range-end also exists here, advance to range sub-row
		if !m.cursorOnRangeAnnotation && m.isRangeAnnotationEnd(m.diffCursor) {
			dl := m.diffLines[m.diffCursor]
			lineNum := m.diffLineNum(dl)
			if m.store.Has(m.currFile, lineNum, string(dl.ChangeType)) {
				m.cursorOnRangeAnnotation = true
				return
			}
		}
		// otherwise move to the next diff line
		m.cursorOnAnnotation = false
		m.cursorOnRangeAnnotation = false
		for i := m.diffCursor + 1; i < len(m.diffLines); i++ {
			if m.diffLines[i].ChangeType != diff.ChangeDivider && !m.isCollapsedHidden(i, hunks) {
				m.diffCursor = i
				return
			}
		}
		return
	}

	// if current line has a point or range-end annotation, stop on it first (disabled during selection).
	// skip for delete-only placeholders — their annotations are only visible when expanded.
	if !m.selecting && m.diffCursor >= 0 && m.diffCursor < len(m.diffLines) {
		dl := m.diffLines[m.diffCursor]
		if dl.ChangeType != diff.ChangeDivider && !m.isDeleteOnlyPlaceholder(m.diffCursor, hunks) {
			lineNum := m.diffLineNum(dl)
			hasPoint := m.store.Has(m.currFile, lineNum, string(dl.ChangeType))
			hasRangeEnd := m.isRangeAnnotationEnd(m.diffCursor)
			if hasPoint || hasRangeEnd {
				m.cursorOnAnnotation = true
				m.cursorOnRangeAnnotation = !hasPoint && hasRangeEnd
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
	// if currently on annotation sub-line, move up to previous sub-row or diff line
	if m.cursorOnAnnotation {
		// on range sub-row: if a point annotation also exists, move up to point sub-row
		if m.cursorOnRangeAnnotation {
			dl := m.diffLines[m.diffCursor]
			lineNum := m.diffLineNum(dl)
			if m.store.Has(m.currFile, lineNum, string(dl.ChangeType)) {
				m.cursorOnRangeAnnotation = false
				return
			}
		}
		m.cursorOnAnnotation = false
		m.cursorOnRangeAnnotation = false
		return
	}

	hunks := m.findHunks()
	for i := m.diffCursor - 1; i >= 0; i-- {
		if m.diffLines[i].ChangeType == diff.ChangeDivider || m.isCollapsedHidden(i, hunks) {
			continue
		}
		m.diffCursor = i
		// if this line has annotations, land on the last sub-row (skip for delete-only placeholders; disabled during selection)
		if !m.selecting && !m.isDeleteOnlyPlaceholder(i, hunks) {
			dl := m.diffLines[i]
			lineNum := m.diffLineNum(dl)
			hasRangeEnd := m.isRangeAnnotationEnd(i)
			hasPoint := m.store.Has(m.currFile, lineNum, string(dl.ChangeType))
			if hasPoint || hasRangeEnd {
				m.cursorOnAnnotation = true
				m.cursorOnRangeAnnotation = hasRangeEnd
			}
		}
		return
	}
	// if we're at the first line and there's a file-level annotation, go to it (disabled during selection)
	if !m.selecting && m.diffCursor >= 0 && m.hasFileAnnotation() {
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
	m.cursorOnRangeAnnotation = false
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
	m.cursorOnRangeAnnotation = false
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
	m.cursorOnRangeAnnotation = false
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
	m.cursorOnRangeAnnotation = false
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
