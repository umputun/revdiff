package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

// lineNumGutterWidth returns the total character width of the line number gutter.
// two-column layout: " " + oldNum(W) + " " + newNum(W) = 2*W + 2
// single-column layout: " " + num(W) = W + 1
func (m Model) lineNumGutterWidth() int {
	if m.file.singleColLineNum {
		return m.file.lineNumWidth + 1
	}
	return m.file.lineNumWidth*2 + 2
}

// lineNumGutter returns the formatted line number gutter string for a diff line.
// uses muted color via lipgloss style (StyleKeyLineNumber); safe here because the gutter
// is concatenated before content, so the lipgloss reset doesn't break outer backgrounds.
// two-column layout: " OOO NNN" where OOO is right-aligned old num, NNN is right-aligned new num.
// blank columns for adds (no old), removes (no new), and dividers (both blank).
// single-column layout: " NNN" — used for full-context files where OldNum == NewNum.
func (m Model) lineNumGutter(dl diff.DiffLine) string {
	w := m.file.lineNumWidth
	blank := strings.Repeat(" ", w)

	if m.file.singleColLineNum {
		var col string
		if dl.ChangeType == diff.ChangeDivider {
			col = blank
		} else {
			col = fmt.Sprintf("%*d", w, dl.NewNum)
		}
		return m.resolver.Style(style.StyleKeyLineNumber).Render(" " + col)
	}

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
	return m.resolver.Style(style.StyleKeyLineNumber).Render(gutter)
}

// blameGutterWidth returns the total character width of the blame gutter.
// layout: " " + author(W) + " " + age(3) = W + 5
func (m Model) blameGutterWidth() int {
	return m.file.blameAuthorLen + 5
}

// blameGutter returns the formatted blame gutter string for a diff line.
// shows author name (truncated) and relative age for lines with NewNum; blank for removed lines and dividers.
// now is the reference time for computing relative age, passed from the render entry point.
func (m Model) blameGutter(dl diff.DiffLine, now time.Time) string {
	w := m.file.blameAuthorLen
	totalW := m.blameGutterWidth()
	blank := strings.Repeat(" ", totalW)

	lineNum := dl.NewNum
	if lineNum == 0 || dl.ChangeType == diff.ChangeDivider {
		return m.resolver.Style(style.StyleKeyLineNumber).Render(blank)
	}

	bl, ok := m.file.blameData[lineNum]
	if !ok {
		return m.resolver.Style(style.StyleKeyLineNumber).Render(blank)
	}

	author := runewidth.Truncate(bl.Author, w, "…")
	pad := w - runewidth.StringWidth(author)
	if pad > 0 {
		author += strings.Repeat(" ", pad)
	}

	age := diff.RelativeAge(bl.Time, now)
	gutter := " " + author + " " + age
	return m.resolver.Style(style.StyleKeyLineNumber).Render(gutter)
}

// hasBlameGutter returns true when the blame gutter should be rendered.
func (m Model) hasBlameGutter() bool {
	return m.modes.showBlame && len(m.file.blameData) > 0
}

// lineGutters returns the formatted line number and blame gutter strings for a diff line.
// returns empty strings for disabled gutters.
func (m Model) lineGutters(dl diff.DiffLine) (numGutter, blameGutter string) {
	if m.modes.lineNumbers {
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
	if m.modes.lineNumbers {
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
	if m.modes.lineNumbers {
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
// cut without indicators. indicatorBg is the line background used for the left glyph and for the
// right indicator's leading space separator (so the colored line extends naturally through the
// content area); the right glyph itself is always drawn on DiffBg by rightScrollIndicator so it
// reads as pane chrome, not as part of the line. the returned width may equal cutWidth+1 when
// right overflow is present, which extendLineBg treats as a no-op (current > target).
// truncates whenever the viewport has room (cutWidth > 0), even when scroll offset is zero, to
// prevent long lines from overflowing the right padding. when cutWidth ≤ 0 (pathologically narrow
// terminal with wide gutters), returns the content unchanged.
func (m Model) applyHorizontalScroll(content string, indicatorBg style.Color) string {
	cutWidth := m.diffContentWidth() - m.gutterExtra()
	if cutWidth <= 0 {
		return content
	}
	origWidth := lipgloss.Width(content)
	start := m.layout.scrollX
	end := m.layout.scrollX + cutWidth

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
	return ansi.Cut(content, m.layout.scrollX, m.layout.scrollX+cutWidth)
}

// leftScrollIndicator renders the left-side scroll overflow glyph («) using raw ANSI sequences
// so it doesn't break outer lipgloss backgrounds. the « replaces the first visible content column,
// so it belongs to the line and uses lineBg for its background; empty string emits foreground only.
// in no-colors mode, falls back to reverse video.
func (m Model) leftScrollIndicator(lineBg style.Color) string {
	return m.scrollIndicatorANSI("«", lineBg, lineBg, false)
}

// rightScrollIndicator renders the right-side scroll overflow glyph (») prefixed with a space
// separator so the glyph doesn't touch the last content character. uses raw ANSI sequences so it
// doesn't break outer lipgloss backgrounds. the leading space carries lineBg so the colored line
// extends contiguously through the content area, while the » glyph itself is drawn on DiffBg so it
// visually sits on pane chrome (matching the surrounding right-padding column) rather than on the
// line's colored bg. in no-colors mode, falls back to reverse video.
func (m Model) rightScrollIndicator(lineBg style.Color) string {
	return m.scrollIndicatorANSI("»", lineBg, m.resolver.Color(style.ColorKeyDiffPaneBg), true)
}

// scrollIndicatorANSI builds the ANSI-encoded indicator string shared by left and right variants.
// leadingSpace controls whether a separator space is emitted before the glyph (drawn on spaceBg);
// glyphBg is the background for the glyph itself. the two can differ so the right indicator can
// keep its separator on the line bg (letting the colored content area extend naturally) while
// drawing its glyph on DiffBg to read as pane chrome. only emits the fg/bg reset sequences we
// actually set, so callers that pass an empty bg (or a theme with empty Muted) don't accidentally
// trash inherited pane background or foreground.
func (m Model) scrollIndicatorANSI(glyph string, spaceBg, glyphBg style.Color, leadingSpace bool) string {
	if m.cfg.noColors {
		prefix := ""
		if leadingSpace {
			prefix = " "
		}
		return prefix + "\033[7m" + glyph + "\033[27m"
	}
	var b strings.Builder
	if leadingSpace {
		if spaceBg != "" {
			b.WriteString(string(spaceBg))
		}
		b.WriteString(" ")
		if spaceBg != "" {
			b.WriteString("\033[49m")
		}
	}
	if glyphBg != "" {
		b.WriteString(string(glyphBg))
	}
	fg := m.resolver.Color(style.ColorKeyMutedFg)
	if fg != "" {
		b.WriteString(string(fg))
	}
	b.WriteString(glyph)
	if fg != "" {
		b.WriteString("\033[39m")
	}
	if glyphBg != "" {
		b.WriteString("\033[49m")
	}
	return b.String()
}

// renderDiff renders the current file's diff lines with styling, cursor highlight,
// and injected annotation lines.
func (m Model) renderDiff() string {
	if len(m.file.lines) == 0 {
		return "  no changes"
	}

	m.blameNow = time.Now()

	if m.modes.collapsed.enabled {
		return m.renderCollapsedDiff()
	}

	m.search.matchSet = m.buildSearchMatchSet()

	annotationMap, fileComment := m.buildAnnotationMap()
	var b strings.Builder
	m.renderFileAnnotationHeader(&b, fileComment)

	for i, dl := range m.file.lines {
		m.renderDiffLine(&b, i, dl)
		m.renderAnnotationOrInput(&b, i, annotationMap)
	}
	return b.String()
}

// buildAnnotationMap creates a lookup map of line annotations for the current file.
// returns the annotation map and the file-level comment (empty if none).
func (m Model) buildAnnotationMap() (annotations map[string]string, fileComment string) {
	all := m.store.Get(m.file.name)
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
	if m.annot.annotating && m.annot.fileAnnotating {
		line := " " + m.renderer.AnnotationInline("\U0001f4ac file: ") + m.annot.input.View()
		// strip textinput's unstyled trailing padding so extendLineBg can re-pad with DiffBg
		line = strings.TrimRight(line, " ")
		b.WriteString(m.extendLineBg(line, m.resolver.Color(style.ColorKeyDiffPaneBg)) + "\n")
		return
	}

	if fileComment != "" {
		cursor := " "
		if m.nav.diffCursor == -1 && m.layout.focus == paneDiff {
			cursor = m.renderer.DiffCursor(m.cfg.noColors)
		}
		m.renderWrappedAnnotation(b, cursor, "\U0001f4ac file: "+fileComment)
	}
}

// renderDiffLine writes a single styled diff line (with cursor highlight) to the builder.
// when wrap mode is active, long lines are broken at word boundaries with ↪ continuation markers.
func (m Model) renderDiffLine(b *strings.Builder, idx int, dl diff.DiffLine) {
	lineContent, textContent, hasHighlight := m.prepareLineContent(idx, dl)
	textContent = m.applyIntraLineHighlight(idx, dl.ChangeType, textContent)
	isSearchMatch := m.search.matchSet[idx]

	isCursor := m.isCursorLine(idx)

	// wrap mode: break long lines at word boundaries (dividers are short, skip them)
	if m.modes.wrap && dl.ChangeType != diff.ChangeDivider {
		m.renderWrappedDiffLine(b, dl, textContent, hasHighlight, isCursor, isSearchMatch)
		return
	}

	numGutter, blGutter := m.lineGutters(dl)

	var content string
	if dl.ChangeType == diff.ChangeDivider {
		content = m.resolver.Style(style.StyleKeyLineNumber).Render(" " + lineContent)
	} else {
		content = m.styleDiffContent(dl.ChangeType, m.linePrefix(dl.ChangeType), textContent, hasHighlight, isSearchMatch)
	}

	lineBg := m.resolver.LineBg(dl.ChangeType)
	// wrap mode divider fallthrough: dividers are unwrapped even in wrap mode, but indicators
	// only belong in unwrapped mode globally, so skip them for this edge case.
	if m.modes.wrap && dl.ChangeType == diff.ChangeDivider {
		content = m.plainHorizontalCut(content)
	} else {
		content = m.applyHorizontalScroll(content, m.resolver.IndicatorBg(dl.ChangeType))
	}
	content = m.extendLineBg(content, lineBg)

	cursor := " "
	if isCursor {
		cursor = m.renderer.DiffCursor(m.cfg.noColors)
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
		styled = m.extendLineBg(styled, m.resolver.LineBg(dl.ChangeType))

		cursor := " "
		if i == 0 && isCursor {
			cursor = m.renderer.DiffCursor(m.cfg.noColors)
		}
		b.WriteString(cursor + ng + bg + styled + "\n")
	}
}

// wrappedLineCount returns the number of visual rows a diff line occupies.
// returns 1 when wrap mode is off or for divider lines.
// stays in sync with renderWrappedDiffLine by using the same wrapContent method and width calculation.
func (m Model) wrappedLineCount(idx int) int {
	if !m.modes.wrap || idx < 0 || idx >= len(m.file.lines) {
		return 1
	}
	dl := m.file.lines[idx]
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
	return m.sgr.Reemit(lines)
}

// prepareLineContent returns the display-ready content for a diff line with tabs replaced.
// returns the raw line content, the best available content (highlighted if available), and whether highlight was used.
func (m Model) prepareLineContent(idx int, dl diff.DiffLine) (lineContent, textContent string, hasHighlight bool) {
	lineContent = strings.ReplaceAll(dl.Content, "\t", m.cfg.tabSpaces)
	hasHighlight = idx < len(m.file.highlighted)
	textContent = lineContent
	if hasHighlight {
		textContent = strings.ReplaceAll(m.file.highlighted[idx], "\t", m.cfg.tabSpaces)
	}
	return lineContent, textContent, hasHighlight
}

// applyIntraLineHighlight inserts ANSI background markers for intra-line word-diff ranges.
// returns textContent unchanged when no intra-line ranges are available for the given line.
// uses WordAddBg/WordRemoveBg for color mode and reverse-video for no-color mode.
func (m Model) applyIntraLineHighlight(idx int, changeType diff.ChangeType, textContent string) string {
	if idx >= len(m.file.intraRanges) || m.file.intraRanges[idx] == nil {
		return textContent
	}
	if changeType != diff.ChangeAdd && changeType != diff.ChangeRemove {
		return textContent
	}

	ranges := m.file.intraRanges[idx]
	if len(ranges) == 0 {
		return textContent
	}

	var hlOn, hlOff string
	if m.cfg.noColors {
		hlOn = "\033[7m"   // reverse video
		hlOff = "\033[27m" // reverse video off
	} else {
		switch changeType { //nolint:exhaustive // only add/remove relevant
		case diff.ChangeAdd:
			hlOn = string(m.resolver.WordDiffBg(diff.ChangeAdd))
			hlOff = string(m.resolver.LineBg(diff.ChangeAdd)) // restore line bg
		case diff.ChangeRemove:
			hlOn = string(m.resolver.WordDiffBg(diff.ChangeRemove))
			hlOff = string(m.resolver.LineBg(diff.ChangeRemove)) // restore line bg
		}
	}

	if hlOn == "" {
		return textContent
	}

	return m.differ.InsertHighlightMarkers(textContent, ranges, hlOn, hlOff)
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

// wrapPrefixForHighlight wraps the +/-/~ prefix in explicit raw ANSI fg when
// chroma highlighting is on. Highlighted line styles intentionally set only
// background (chroma owns per-token fg for content), so the prefix would
// otherwise inherit the terminal default fg and may render invisibly on
// light theme backgrounds.
func (m Model) wrapPrefixForHighlight(prefix string, fg style.Color, hasHighlight bool) string {
	if !hasHighlight || fg == "" {
		return prefix
	}
	return string(fg) + prefix + string(style.ResetFg)
}

// highlightSearchMatches wraps each occurrence of the search term in the visible text
// with ANSI background color sequence (preserving syntax foreground within matches).
// works with both plain text and ANSI-coded content by stripping ANSI to find match positions.
// changeType is used to restore the correct line background after each match (add/remove bg)
// instead of resetting to terminal default, which would break word-diff and line bg overlays.
func (m Model) highlightSearchMatches(s string, changeType diff.ChangeType) string {
	if m.search.term == "" {
		return s
	}

	// find match positions in visible (ANSI-stripped) text
	plain := ansi.Strip(s)
	plainLower := strings.ToLower(plain)
	term := strings.ToLower(m.search.term)
	if !strings.Contains(plainLower, term) {
		return s
	}

	// collect all match ranges as byte offsets in ANSI-stripped text
	var matches []worddiff.Range
	offset := 0
	for {
		idx := strings.Index(plainLower[offset:], term)
		if idx < 0 {
			break
		}
		start := offset + idx
		matches = append(matches, worddiff.Range{Start: start, End: start + len(term)})
		offset = start + len(term)
	}
	if len(matches) == 0 {
		return s
	}

	// background-only highlight preserves syntax foreground colors within matches.
	// restore to line bg (add/remove) after each match instead of terminal default (\033[49m]),
	// so word-diff and line bg overlays are not broken by search highlights.
	searchBg := m.resolver.Color(style.ColorKeySearchBg)
	hlOn := string(searchBg)
	hlOff := "\033[49m"
	if hlOn == "" {
		// no-colors mode: fall back to reverse video so matches remain visible
		hlOn = "\033[7m"
		hlOff = "\033[27m"
	} else if bg := m.resolver.LineBg(changeType); bg != "" {
		hlOff = string(bg)
	}

	return m.differ.InsertHighlightMarkers(s, matches, hlOn, hlOff)
}

// styleDiffContent applies the appropriate line style based on change type.
// does NOT extend backgrounds — callers must apply extendLineBg after applyHorizontalScroll
// (non-wrap paths) or directly after styling (wrap paths where scroll is not used).
func (m Model) styleDiffContent(changeType diff.ChangeType, prefix, content string, hasHighlight, isSearchMatch bool) string {
	if isSearchMatch && m.search.term != "" {
		content = m.highlightSearchMatches(content, changeType)
	}
	prefix = m.wrapPrefixForHighlight(prefix, m.resolver.LineFg(changeType), hasHighlight)
	return m.resolver.LineStyle(changeType, hasHighlight).Render(prefix + content)
}

// extendLineBg extends a styled line's background to the full diff content width
// using raw ANSI sequences. this ensures add/remove/modify backgrounds fill the entire line.
// subtracts line number gutter width when line numbers are enabled.
func (m Model) extendLineBg(styled string, bg style.Color) string {
	if bg == "" {
		return styled
	}
	// target = content area minus cursor bar (1) minus gutters (if on)
	// diffContentWidth() already excludes cursor bar; subtract gutters if enabled
	// diffContentWidth() already includes right padding, just subtract gutters
	targetWidth := m.diffContentWidth() - m.gutterExtra()
	currentWidth := lipgloss.Width(styled)
	if pad := targetWidth - currentWidth; pad > 0 {
		return styled + string(bg) + strings.Repeat(" ", pad) + "\033[49m"
	}
	return styled
}

// renderAnnotationOrInput writes the annotation input or existing annotation below a diff line.
func (m Model) renderAnnotationOrInput(b *strings.Builder, idx int, annotationMap map[string]string) {
	if m.annot.annotating && !m.annot.fileAnnotating && idx == m.nav.diffCursor {
		line := " " + m.renderer.AnnotationInline("\U0001f4ac ") + m.annot.input.View()
		// strip textinput's unstyled trailing padding so extendLineBg can re-pad with DiffBg
		line = strings.TrimRight(line, " ")
		b.WriteString(m.extendLineBg(line, m.resolver.Color(style.ColorKeyDiffPaneBg)) + "\n")
		return
	}
	dl := m.file.lines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
		if comment, ok := annotationMap[key]; ok {
			cursor := " "
			if idx == m.nav.diffCursor && m.annot.cursorOnAnnotation && m.layout.focus == paneDiff {
				cursor = m.renderer.DiffCursor(m.cfg.noColors)
			}
			m.renderWrappedAnnotation(b, cursor, "\U0001f4ac "+comment)
		}
	}
}

// renderWrappedAnnotation writes an annotation line with word wrapping.
// annotations always wrap regardless of wrapMode since they contain prose.
// embedded "\n" in text splits into logical lines; the first logical line carries
// the emoji prefix baked into text, continuation logical lines receive an indent
// sized to the emoji prefix so body columns line up.
// each visual row is padded with DiffPaneBg via extendLineBg so themed pane
// backgrounds extend across the full width rather than falling back to terminal
// default on the right portion.
func (m Model) renderWrappedAnnotation(b *strings.Builder, cursor, text string) {
	wrapWidth := m.diffContentWidth() - 1 // 1 for cursor column
	paneBg := m.resolver.Color(style.ColorKeyDiffPaneBg)

	logical := strings.Split(text, "\n")
	indent := m.annotationContinuationIndent(logical[0])

	first := true
	for i, segment := range logical {
		if i > 0 {
			segment = indent + segment
		}
		var lines []string
		if wrapWidth > 10 && lipgloss.Width(segment) > wrapWidth {
			lines = m.wrapContent(segment, wrapWidth)
		} else {
			lines = []string{segment}
		}
		for _, line := range lines {
			c := " "
			if first {
				c = cursor
				first = false
			}
			styled := c + m.renderer.AnnotationInline(line)
			b.WriteString(m.extendLineBg(styled, paneBg) + "\n")
		}
	}
}

// annotationContinuationIndent returns leading whitespace sized to match the emoji
// prefix on the first logical line of an annotation so continuation logical lines
// align under the body. Uses lipgloss.Width because the emoji is double-width.
func (m Model) annotationContinuationIndent(firstLogicalLine string) string {
	switch {
	case strings.HasPrefix(firstLogicalLine, "\U0001f4ac file: "):
		return strings.Repeat(" ", lipgloss.Width("\U0001f4ac file: "))
	case strings.HasPrefix(firstLogicalLine, "\U0001f4ac "):
		return strings.Repeat(" ", lipgloss.Width("\U0001f4ac "))
	default:
		return ""
	}
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
		return max(10, m.layout.width-4)
	}
	// multi-file or single-file with TOC: diff pane width minus borders (4) minus tree width, minus bar (1), minus right padding (1)
	return max(10, m.layout.width-m.layout.treeWidth-4-2)
}
