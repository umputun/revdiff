package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
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

// blameGutterWidth returns the total character width of the blame gutter.
// layout: " " + author(W) + " " + age(3) = W + 5
func (m Model) blameGutterWidth() int {
	return m.blameAuthorLen + 5
}

// blameGutter returns the formatted blame gutter string for a diff line.
// shows author name (truncated) and relative age for lines with NewNum; blank for removed lines and dividers.
// now is the reference time for computing relative age, passed from the render entry point.
func (m Model) blameGutter(dl diff.DiffLine, now time.Time) string {
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

	age := diff.RelativeAge(bl.Time, now)
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
		blameGutter = m.blameGutter(dl, m.blameNow)
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

// applyHorizontalScroll truncates content to the diff content width and applies horizontal scroll offset.
// always truncates even when scroll offset is zero to prevent long lines from overflowing the right padding.
func (m Model) applyHorizontalScroll(content string) string {
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

	m.searchMatchSet = m.buildSearchMatchSet()

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
		line := " " + m.annotationInline("\U0001f4ac file: ") + m.annotateInput.View()
		// strip textinput's unstyled trailing padding so extendLineBg can re-pad with DiffBg
		line = strings.TrimRight(line, " ")
		b.WriteString(m.extendLineBg(line, m.styles.colors.DiffBg) + "\n")
		return
	}

	if fileComment != "" {
		cursor := " "
		if m.diffCursor == -1 && m.focus == paneDiff {
			cursor = m.diffCursorCell()
		}
		m.renderWrappedAnnotation(b, cursor, "\U0001f4ac file: "+fileComment)
	}
}

// renderDiffLine writes a single styled diff line (with cursor highlight) to the builder.
// when wrap mode is active, long lines are broken at word boundaries with ↪ continuation markers.
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	lineContent, textContent, hasHighlight := m.prepareLineContent(idx, dl)
	isSearchMatch := m.searchMatchSet[idx]

	isCursor := m.isCursorLine(idx)

	// wrap mode: break long lines at word boundaries (dividers are short, skip them)
	if m.wrapMode && dl.ChangeType != diff.ChangeDivider {
		m.renderWrappedDiffLine(b, dl, textContent, hasHighlight, isCursor, isSearchMatch)
		return
	}

	numGutter, blGutter := m.lineGutters(dl)

	var content string
	if dl.ChangeType == diff.ChangeDivider {
		content = m.styles.LineNumber.Render(" " + lineContent)
	} else {
		content = m.styleDiffContent(dl.ChangeType, m.linePrefix(dl.ChangeType), textContent, hasHighlight, isSearchMatch)
	}

	content = m.applyHorizontalScroll(content)
	content = m.extendLineBg(content, m.changeBgColor(dl.ChangeType))

	cursor := " "
	if isCursor {
		cursor = m.diffCursorCell()
	}
	b.WriteString(cursor + numGutter + blGutter + content + "\n")
}

// renderWrappedDiffLine renders a diff line with word wrapping, producing continuation lines with ↪ markers.
func (m Model) renderWrappedDiffLine(b *strings.Builder, dl diff.DiffLine, textContent string, hasHighlight, isCursor, isSearchMatch bool) {
	numGutter, blGutter := m.lineGutters(dl)
	numBlank, blBlank := m.gutterBlanks()

	visualLines := m.wrapContent(textContent, m.wrapWidth())
	for i, vl := range visualLines {
		prefix := " ↪ "
		ng := numBlank
		bg := blBlank
		if i == 0 {
			prefix = m.linePrefix(dl.ChangeType)
			ng = numGutter
			bg = blGutter
		}

		styled := m.styleDiffContent(dl.ChangeType, prefix, vl, hasHighlight, isSearchMatch)
		styled = m.extendLineBg(styled, m.changeBgColor(dl.ChangeType))

		cursor := " "
		if i == 0 && isCursor {
			cursor = m.diffCursorCell()
		}
		b.WriteString(cursor + ng + bg + styled + "\n")
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
	return len(m.wrapContent(textContent, m.wrapWidth()))
}

// wrapContent wraps text content at the given width using word boundaries.
// returns a slice of visual lines (at least one). handles ANSI escape sequences.
// re-emits active SGR state (foreground, bold, italic) at the start of each continuation line
// because ansi.Wrap does not preserve ANSI state across inserted newlines.
func (m Model) wrapContent(content string, width int) []string {
	if width <= 0 {
		return []string{content}
	}
	wrapped := ansi.Wrap(content, width, "")
	lines := strings.Split(wrapped, "\n")
	if len(lines) <= 1 {
		return lines
	}
	return m.reemitANSIState(lines)
}

// reemitANSIState scans each line for active SGR attributes (foreground color, bold, italic)
// and prepends the accumulated state to the next line. this fixes the issue where ansi.Wrap
// splits a line mid-token, causing continuation lines to lose their foreground color.
func (m Model) reemitANSIState(lines []string) []string {
	var activeFg string // e.g. "\033[38;2;100;200;50m" or "\033[32m"
	var bold, italic bool

	for i, line := range lines {
		if i > 0 {
			// prepend accumulated state from previous lines
			var prefix strings.Builder
			if activeFg != "" {
				prefix.WriteString(activeFg)
			}
			if bold {
				prefix.WriteString("\033[1m")
			}
			if italic {
				prefix.WriteString("\033[3m")
			}
			if prefix.Len() > 0 {
				lines[i] = prefix.String() + line
			}
		}

		// scan this line to update active state
		activeFg, bold, italic = m.scanANSIState(lines[i], activeFg, bold, italic)
	}
	return lines
}

// scanANSIState scans a string for SGR sequences and returns the updated state.
// tracks foreground color (38;2;r;g;b or 3x), bold (1), italic (3) and their resets.
func (m Model) scanANSIState(s, fg string, bold, italic bool) (string, bool, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] != '\033' || i+1 >= len(s) || s[i+1] != '[' {
			continue
		}
		seq, params, end := m.parseSGR(s, i)
		if end < 0 {
			break
		}
		i = end
		if seq == "" { // not an SGR sequence
			continue
		}
		fg, bold, italic = m.applySGR(params, seq, fg, bold, italic)
	}
	return fg, bold, italic
}

// parseSGR extracts an SGR sequence starting at position i in s.
// returns the full sequence, the parameter string, and the end index.
// returns end=-1 if the sequence is unterminated.
// returns seq="" if the CSI sequence is not SGR (not terminated by 'm').
func (m Model) parseSGR(s string, i int) (seq, params string, end int) {
	j := i + 2
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3F {
		j++
	}
	if j >= len(s) {
		return "", "", -1
	}
	if s[j] != 'm' {
		return "", "", j
	}
	return s[i : j+1], s[i+2 : j], j
}

// applySGR updates the active SGR state based on a parameter string.
func (m Model) applySGR(params, seq, fg string, bold, italic bool) (string, bool, bool) {
	switch params {
	case "", "0": // full reset (\033[m and \033[0m)
		return "", false, false
	case "1": // bold on
		return fg, true, italic
	case "3": // italic on
		return fg, bold, true
	case "22": // bold off
		return fg, false, italic
	case "23": // italic off
		return fg, bold, false
	case "39": // fg reset
		return "", bold, italic
	}
	if m.isFgColor(params) {
		return seq, bold, italic
	}
	return fg, bold, italic
}

// isFgColor returns true if the SGR params represent a foreground color (24-bit or basic).
func (m Model) isFgColor(params string) bool {
	return strings.HasPrefix(params, "38;2;") ||
		(len(params) == 2 && params[0] == '3' && params[1] >= '0' && params[1] <= '7')
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
// does NOT extend backgrounds — callers must apply extendLineBg after applyHorizontalScroll
// (non-wrap paths) or directly after styling (wrap paths where scroll is not used).
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
			return m.styles.LineContextHighlight.Render(prefix + content)
		}
		return m.styles.LineContext.Render(prefix + content)
	}
}

// changeBgColor returns the background color for a given change type.
func (m Model) changeBgColor(changeType diff.ChangeType) string {
	switch changeType {
	case diff.ChangeAdd:
		return m.styles.colors.AddBg
	case diff.ChangeRemove:
		return m.styles.colors.RemoveBg
	default:
		return ""
	}
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
	// diffContentWidth() already includes right padding, just subtract gutters
	targetWidth := m.diffContentWidth() - m.gutterExtra()
	currentWidth := lipgloss.Width(styled)
	if pad := targetWidth - currentWidth; pad > 0 {
		return styled + m.ansiBg(bgColor) + strings.Repeat(" ", pad) + "\033[49m"
	}
	return styled
}

// renderAnnotationOrInput writes the annotation input or existing annotation below a diff line.
func (m Model) renderAnnotationOrInput(b *strings.Builder, idx int, annotationMap map[string]string) {
	if m.annotating && !m.fileAnnotating && idx == m.diffCursor {
		line := " " + m.annotationInline("\U0001f4ac ") + m.annotateInput.View()
		// strip textinput's unstyled trailing padding so extendLineBg can re-pad with DiffBg
		line = strings.TrimRight(line, " ")
		b.WriteString(m.extendLineBg(line, m.styles.colors.DiffBg) + "\n")
		return
	}
	dl := m.diffLines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
		if comment, ok := annotationMap[key]; ok {
			cursor := " "
			if idx == m.diffCursor && m.cursorOnAnnotation && m.focus == paneDiff {
				cursor = m.diffCursorCell()
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
			b.WriteString(c + m.annotationInline(line) + "\n")
		}
		return
	}

	b.WriteString(cursor + m.annotationInline(text) + "\n")
}

// annotationInline renders annotation text using raw ANSI sequences instead of lipgloss.Render()
// to avoid \033[0m full reset that kills the outer DiffBg background.
func (m Model) annotationInline(text string) string {
	fg := m.styles.colors.Annotation
	bg := m.styles.colors.DiffBg
	var b strings.Builder
	if fg != "" {
		b.WriteString(m.ansiFg(fg))
	}
	if bg != "" {
		b.WriteString(m.ansiBg(bg))
	}
	b.WriteString("\033[3m") // italic on
	b.WriteString(text)
	b.WriteString("\033[23m") // italic off
	if fg != "" {
		b.WriteString("\033[39m")
	}
	if bg != "" {
		b.WriteString("\033[49m")
	}
	return b.String()
}

// diffCursorCell renders the ▶ cursor using raw ANSI sequences instead of lipgloss.Render()
// to avoid \033[0m full reset that kills the outer DiffBg background.
func (m Model) diffCursorCell() string {
	if m.noColors {
		return "\033[7m▶\033[27m" // reverse video
	}
	fg := m.styles.colors.CursorFg
	bg := m.styles.colors.CursorBg
	if bg == "" {
		bg = m.styles.colors.DiffBg
	}
	var b strings.Builder
	if fg != "" {
		b.WriteString(m.ansiFg(fg))
	}
	if bg != "" {
		b.WriteString(m.ansiBg(bg))
	}
	b.WriteString("▶")
	if fg != "" {
		b.WriteString("\033[39m")
	}
	if bg != "" {
		b.WriteString("\033[49m")
	}
	return b.String()
}

const wrapGutterWidth = 3 // wrap gutter prefix width: " + ", " - ", "   ", " ↪ "

// wrapWidth returns the available width for wrapped content (diff content minus gutter prefix and extra gutters).
func (m Model) wrapWidth() int {
	return m.diffContentWidth() - wrapGutterWidth - m.gutterExtra()
}

// diffContentWidth returns the available width for diff line content.
// accounts for borders, cursor bar, and 1 char right padding to prevent text from touching the pane border.
func (m Model) diffContentWidth() int {
	if m.treePaneHidden() {
		// tree hidden or single-file without TOC: diff pane borders (2) + cursor bar (1) + right padding (1)
		return max(10, m.width-4)
	}
	// multi-file or single-file with TOC: diff pane width minus borders (4) minus tree width, minus bar (1), minus right padding (1)
	return max(10, m.width-m.treeWidth-4-2)
}
