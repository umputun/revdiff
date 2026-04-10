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
// when content overflows the viewport, shows double-angle overflow indicators («/») at the edges so
// the user can see more content exists in that direction. the left indicator replaces the first
// visible column; the right indicator reserves the last content column for a space separator and
// extends one column beyond cutWidth into the pane's right padding so the glyph sits flush against
// the right border. when the viewport is too narrow to fit both indicators plus inner content
// (cutWidth ≤ 2 with dual overflow, cutWidth ≤ 1 with single-side overflow), falls back to a plain
// cut without indicators. indicatorBg is the line background used so the indicator blends with the
// line (add/remove/modify bg, or DiffBg for context/divider). the returned width may equal cutWidth+1
// when right overflow is present, which extendLineBg treats as a no-op (current > target).
// truncates whenever the viewport has room (cutWidth > 0), even when scroll offset is zero, to
// prevent long lines from overflowing the right padding. when cutWidth ≤ 0 (pathologically narrow
// terminal with wide gutters), returns the content unchanged.
func (m Model) applyHorizontalScroll(content, indicatorBg string) string {
	cutWidth := m.diffContentWidth() - m.gutterExtra()
	if cutWidth <= 0 {
		return content
	}
	origWidth := lipgloss.Width(content)
	start := m.scrollX
	end := m.scrollX + cutWidth

	hasLeftOverflow := start > 0 && origWidth > start
	hasRightOverflow := origWidth > end

	if !hasLeftOverflow && !hasRightOverflow {
		return ansi.Cut(content, start, end)
	}

	// reserve columns for indicators: left takes 1 visible col (replaces first col),
	// right reserves 1 col for a space separator and then extends 1 col beyond cutWidth
	// into the pane's right padding so the arrow sits flush against the border.
	innerStart := start
	innerEnd := end
	if hasLeftOverflow {
		innerStart++
	}
	if hasRightOverflow {
		innerEnd--
	}
	if innerEnd <= innerStart {
		// viewport too narrow to fit inner content plus indicators; fall back to plain cut
		return ansi.Cut(content, start, end)
	}

	var b strings.Builder
	if hasLeftOverflow {
		b.WriteString(m.leftScrollIndicator(indicatorBg))
	}
	b.WriteString(ansi.Cut(content, innerStart, innerEnd))
	if hasRightOverflow {
		b.WriteString(m.rightScrollIndicator(indicatorBg))
	}
	return b.String()
}

// plainHorizontalCut truncates content to the diff content width and applies horizontal scroll
// offset without emitting any overflow indicators. used for wrap-mode divider lines where
// indicators would contradict the "unwrapped mode only" design intent.
func (m Model) plainHorizontalCut(content string) string {
	cutWidth := m.diffContentWidth() - m.gutterExtra()
	if cutWidth <= 0 {
		return content
	}
	return ansi.Cut(content, m.scrollX, m.scrollX+cutWidth)
}

// leftScrollIndicator renders the left-side scroll overflow glyph («) using raw ANSI sequences
// so it doesn't break outer lipgloss backgrounds. lineBg is the line background the glyph should
// blend with; empty string emits foreground only. in no-colors mode, falls back to reverse video.
func (m Model) leftScrollIndicator(lineBg string) string {
	return m.scrollIndicatorANSI("«", lineBg, false)
}

// rightScrollIndicator renders the right-side scroll overflow glyph (») prefixed with a space
// separator so the glyph doesn't touch the last content character. uses raw ANSI sequences so it
// doesn't break outer lipgloss backgrounds. lineBg is the line background the glyph should blend
// with; empty string emits foreground only. in no-colors mode, falls back to reverse video.
func (m Model) rightScrollIndicator(lineBg string) string {
	return m.scrollIndicatorANSI("»", lineBg, true)
}

// scrollIndicatorANSI builds the ANSI-encoded indicator string shared by left and right variants.
// leadingSpace controls whether a separator space is emitted before the glyph (carrying lineBg).
// only emits the fg/bg reset sequences we actually set, so callers that pass an empty lineBg (or
// a theme with empty Muted) don't accidentally trash inherited pane background or foreground.
func (m Model) scrollIndicatorANSI(glyph, lineBg string, leadingSpace bool) string {
	if m.noColors {
		prefix := ""
		if leadingSpace {
			prefix = " "
		}
		return prefix + "\033[7m" + glyph + "\033[27m"
	}
	var b strings.Builder
	bg := m.ansiBg(lineBg)
	if bg != "" {
		b.WriteString(bg)
	}
	if leadingSpace {
		b.WriteString(" ")
	}
	fg := m.ansiFg(m.styles.colors.Muted)
	if fg != "" {
		b.WriteString(fg)
	}
	b.WriteString(glyph)
	if fg != "" {
		b.WriteString("\033[39m")
	}
	if bg != "" {
		b.WriteString("\033[49m")
	}
	return b.String()
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
	textContent = m.applyIntraLineHighlight(idx, dl.ChangeType, textContent)
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

	lineBg := m.changeBgColor(dl.ChangeType)
	// wrap mode divider fallthrough: dividers are unwrapped even in wrap mode, but indicators
	// only belong in unwrapped mode globally, so skip them for this edge case.
	if m.wrapMode && dl.ChangeType == diff.ChangeDivider {
		content = m.plainHorizontalCut(content)
	} else {
		content = m.applyHorizontalScroll(content, m.indicatorBg(lineBg))
	}
	content = m.extendLineBg(content, lineBg)

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

// reemitANSIState scans each line for active SGR attributes (foreground color, background color,
// bold, italic, reverse video) and prepends the accumulated state to the next line. this fixes
// the issue where ansi.Wrap splits a line mid-token, causing continuation lines to lose their styling.
func (m Model) reemitANSIState(lines []string) []string {
	var activeFg, activeBg string // e.g. "\033[38;2;100;200;50m" or "\033[48;2;...]m"
	var bold, italic, reverse bool

	for i, line := range lines {
		if i > 0 {
			if prefix := m.buildSGRPrefix(activeFg, activeBg, bold, italic, reverse); prefix != "" {
				lines[i] = prefix + line
			}
		}
		activeFg, activeBg, bold, italic, reverse = m.scanANSIState(lines[i], activeFg, activeBg, bold, italic, reverse)
	}
	return lines
}

// buildSGRPrefix assembles an ANSI prefix string from the accumulated SGR state.
func (m Model) buildSGRPrefix(fg, bg string, bold, italic, reverse bool) string {
	var b strings.Builder
	if fg != "" {
		b.WriteString(fg)
	}
	if bg != "" {
		b.WriteString(bg)
	}
	if bold {
		b.WriteString("\033[1m")
	}
	if italic {
		b.WriteString("\033[3m")
	}
	if reverse {
		b.WriteString("\033[7m")
	}
	return b.String()
}

// scanANSIState scans a string for SGR sequences and returns the updated state.
// tracks foreground color (38;2;r;g;b or 3x), background color (48;2;r;g;b or 4x),
// bold (1), italic (3), reverse video (7) and their resets.
func (m Model) scanANSIState(s, fg, bg string, bold, italic, reverse bool) (string, string, bool, bool, bool) {
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
		fg, bg, bold, italic, reverse = m.applySGR(params, seq, fg, bg, bold, italic, reverse)
	}
	return fg, bg, bold, italic, reverse
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
func (m Model) applySGR(params, seq, fg, bg string, bold, italic, reverse bool) (string, string, bool, bool, bool) {
	switch params {
	case "", "0": // full reset (\033[m and \033[0m)
		return "", "", false, false, false
	case "1": // bold on
		return fg, bg, true, italic, reverse
	case "3": // italic on
		return fg, bg, bold, true, reverse
	case "7": // reverse video on
		return fg, bg, bold, italic, true
	case "22": // bold off
		return fg, bg, false, italic, reverse
	case "23": // italic off
		return fg, bg, bold, false, reverse
	case "27": // reverse video off
		return fg, bg, bold, italic, false
	case "39": // fg reset
		return "", bg, bold, italic, reverse
	case "49": // bg reset
		return fg, "", bold, italic, reverse
	}
	if m.isFgColor(params) {
		return seq, bg, bold, italic, reverse
	}
	if m.isBgColor(params) {
		return fg, seq, bold, italic, reverse
	}
	return fg, bg, bold, italic, reverse
}

// isFgColor returns true if the SGR params represent a foreground color (24-bit or basic).
func (m Model) isFgColor(params string) bool {
	return strings.HasPrefix(params, "38;2;") ||
		(len(params) == 2 && params[0] == '3' && params[1] >= '0' && params[1] <= '7')
}

// isBgColor returns true if the SGR params represent a background color (24-bit or basic).
func (m Model) isBgColor(params string) bool {
	return strings.HasPrefix(params, "48;2;") ||
		(len(params) == 2 && params[0] == '4' && params[1] >= '0' && params[1] <= '7')
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

// applyIntraLineHighlight inserts ANSI background markers for intra-line word-diff ranges.
// returns textContent unchanged when no intra-line ranges are available for the given line.
// uses WordAddBg/WordRemoveBg for color mode and reverse-video for no-color mode.
func (m Model) applyIntraLineHighlight(idx int, changeType diff.ChangeType, textContent string) string {
	if idx >= len(m.intraRanges) || m.intraRanges[idx] == nil {
		return textContent
	}
	if changeType != diff.ChangeAdd && changeType != diff.ChangeRemove {
		return textContent
	}

	ranges := m.intraRanges[idx]
	if len(ranges) == 0 {
		return textContent
	}

	var hlOn, hlOff string
	if m.noColors {
		hlOn = "\033[7m"   // reverse video
		hlOff = "\033[27m" // reverse video off
	} else {
		switch changeType { //nolint:exhaustive // only add/remove relevant
		case diff.ChangeAdd:
			hlOn = m.ansiBg(m.styles.colors.WordAddBg)
			hlOff = m.ansiBg(m.styles.colors.AddBg) // restore line bg
		case diff.ChangeRemove:
			hlOn = m.ansiBg(m.styles.colors.WordRemoveBg)
			hlOff = m.ansiBg(m.styles.colors.RemoveBg) // restore line bg
		}
	}

	if hlOn == "" {
		return textContent
	}

	return m.insertHighlightMarkers(textContent, ranges, hlOn, hlOff)
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
// changeType is used to restore the correct line background after each match (add/remove bg)
// instead of resetting to terminal default, which would break word-diff and line bg overlays.
func (m Model) highlightSearchMatches(s string, changeType diff.ChangeType) string {
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

	// background-only highlight preserves syntax foreground colors within matches.
	// restore to line bg (add/remove) after each match instead of terminal default (\033[49m]),
	// so word-diff and line bg overlays are not broken by search highlights.
	hlOn := m.ansiBg(m.styles.colors.SearchBg)
	hlOff := "\033[49m"
	if hlOn == "" {
		// no-colors mode: fall back to reverse video so matches remain visible
		hlOn = "\033[7m"
		hlOff = "\033[27m"
	} else if bg := m.ansiBg(m.changeBgColor(changeType)); bg != "" {
		hlOff = bg
	}

	return m.insertHighlightMarkers(s, matches, hlOn, hlOff)
}

// insertHighlightMarkers walks the string inserting hlOn/hlOff ANSI sequences at match positions,
// skipping over existing ANSI escape sequences to preserve them.
// tracks background ANSI state so that match-end restores to the correct bg (e.g. word-diff bg)
// rather than always using the static hlOff (line bg). when the input has no bg sequences
// (word-diff caller), restoreBg stays at hlOff and behavior is unchanged.
func (m Model) insertHighlightMarkers(s string, matches []matchRange, hlOn, hlOff string) string {
	var b strings.Builder
	visPos := 0   // current position in visible text
	matchIdx := 0 // current match we're processing
	i := 0
	restoreBg := hlOff // tracks the active bg to restore after each match
	inMatch := false   // whether we're inside an active highlight span

	for i < len(s) {
		// skip ANSI escape sequences (copy them as-is, track bg state)
		if s[i] == '\033' {
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // include the 'm'
			}
			seq := s[i:j]
			b.WriteString(seq)
			prev := restoreBg
			restoreBg = m.updateRestoreBg(seq, restoreBg, hlOff)
			// if a bg-changing sequence appeared inside a match, re-emit hlOn
			// so the terminal doesn't switch away from the highlight
			if inMatch && restoreBg != prev {
				b.WriteString(hlOn)
			}
			i = j
			continue
		}

		// insert highlight start/end at match boundaries
		if matchIdx < len(matches) && visPos == matches[matchIdx].start {
			b.WriteString(hlOn)
			inMatch = true
		}
		if matchIdx < len(matches) && visPos == matches[matchIdx].end {
			b.WriteString(restoreBg)
			inMatch = false
			matchIdx++
			if matchIdx < len(matches) && visPos == matches[matchIdx].start {
				b.WriteString(hlOn)
				inMatch = true
			}
		}

		b.WriteByte(s[i])
		visPos++
		i++
	}

	// close any unclosed highlight
	if matchIdx < len(matches) && visPos >= matches[matchIdx].start && visPos <= matches[matchIdx].end {
		b.WriteString(restoreBg)
	}

	return b.String()
}

// updateRestoreBg checks if an ANSI escape sequence changes the background or reverse-video state,
// returning the updated restore sequence. resets to hlOff on bg-reset, reverse-off, or full-reset.
func (m Model) updateRestoreBg(seq, current, hlOff string) string {
	if len(seq) < 3 || seq[0] != '\033' || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return current
	}
	params := seq[2 : len(seq)-1]
	switch {
	case strings.HasPrefix(params, "48;2;"): // 24-bit bg
		return seq
	case len(params) == 2 && params[0] == '4' && params[1] >= '0' && params[1] <= '7': // basic bg
		return seq
	case params == "7": // reverse video on
		return seq
	case params == "49" || params == "27" || params == "0" || params == "": // bg/reverse/full reset
		return hlOff
	}
	return current
}

// styleDiffContent applies the appropriate line style based on change type.
// does NOT extend backgrounds — callers must apply extendLineBg after applyHorizontalScroll
// (non-wrap paths) or directly after styling (wrap paths where scroll is not used).
func (m Model) styleDiffContent(changeType diff.ChangeType, prefix, content string, hasHighlight, isSearchMatch bool) string {
	if isSearchMatch && m.searchTerm != "" {
		content = m.highlightSearchMatches(content, changeType)
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

// indicatorBg returns the background color to use for a horizontal scroll indicator,
// falling back to the diff pane background when the line has no explicit bg (context/divider).
// this keeps the indicator visible and consistent with the surrounding line.
func (m Model) indicatorBg(lineBg string) string {
	if lineBg != "" {
		return lineBg
	}
	return m.styles.colors.DiffBg
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
