package ui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// annotMaxInputHeight caps the visible height of the in-progress annotation
// textarea so a runaway paste cannot push the entire diff content off the
// pane. Beyond this row count the textarea scrolls internally.
const annotMaxInputHeight = 20

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// annotKeyFile is the lookup key for file-level annotations in wrappedAnnotationLineCount.
const annotKeyFile = "file"

// annotCharLimit caps annotation text length. sized for multi-item lists and
// small pasted data slices, not for full-document content.
const annotCharLimit = 8000

// hunkKeywordRe matches whole-word "hunk" (case-insensitive).
// "block" was removed as it triggers false positives in casual usage (e.g., "this code block is fine").
var hunkKeywordRe = regexp.MustCompile(`(?i)\bhunk\b`)

// newAnnotationInput creates and focuses a multi-line textarea for annotation
// editing. prefixWidth accounts for the visible prefix characters (cursor col
// + emoji + label + margin).
//
// Keymap follows the gum write convention (Charm's own multi-line text capture
// tool): Enter saves (handled in handleAnnotateKey, NOT bound to InsertNewline
// here), Ctrl+J and Alt+Enter insert newlines. Shift+Enter is terminal-
// dependent — bubbletea v1's KeyMsg has no Shift modifier, so it cannot be
// distinguished from plain Enter. iTerm2/ghostty/kitty users can configure
// their terminal to send Alt+Enter when Shift+Enter is pressed; xterm/tmux
// users should use Ctrl+J or Alt+Enter directly.
func (m *Model) newAnnotationInput(placeholder string, prefixWidth int) (textarea.Model, tea.Cmd) {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.CharLimit = annotCharLimit
	// hide the textarea's left-edge prompt ("┃ ") and line-number gutter so
	// the in-progress input visually matches the saved annotation rendering.
	// MUST be set BEFORE SetWidth — textarea's SetWidth subtracts the prompt
	// width and a 4-cell line-number reserve from the input width, so calling
	// SetWidth with the defaults still active leaves only ~width-6 cells.
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	cmd := ta.Focus()
	ta.SetWidth(max(10, m.diffContentWidth()-prefixWidth))
	ta.SetHeight(1)

	// match the legacy textinput styling: input text uses Normal fg (context-
	// line color) so active input is readable on any theme and visually
	// distinct from saved annotations (which use Annotation color + italic).
	inputStyle := m.resolver.Style(style.StyleKeyAnnotInputText)
	cursorStyle := m.resolver.Style(style.StyleKeyAnnotInputCursor)
	placeholderStyle := m.resolver.Style(style.StyleKeyAnnotInputPlaceholder)
	ta.FocusedStyle.Text = inputStyle
	ta.FocusedStyle.Prompt = inputStyle
	ta.FocusedStyle.CursorLine = inputStyle
	ta.FocusedStyle.Placeholder = placeholderStyle
	ta.Cursor.TextStyle = cursorStyle
	ta.Cursor.Style = cursorStyle

	// rebind keys: Enter is reserved for save (handled in handleAnnotateKey),
	// so InsertNewline keeps only ctrl+j and alt+enter — matching gum write.
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+j", "alt+enter"),
		key.WithHelp("ctrl+j/alt+enter", "newline"),
	)

	return ta, cmd
}

// startAnnotation enters annotation input mode for the current cursor line.
func (m *Model) startAnnotation() tea.Cmd {
	m.clearPendingInputState()
	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}
	// prevent annotating hidden or placeholder removed lines in collapsed mode
	hunks := m.findHunks()
	if m.isCollapsedHidden(m.nav.diffCursor, hunks) {
		return nil
	}
	if m.isDeleteOnlyPlaceholder(m.nav.diffCursor, hunks) {
		return nil
	}

	placeholder := "annotation... (Ctrl+J newline · Ctrl+E editor · Enter save)"

	// pre-fill with existing annotation if one exists. textarea preserves \n
	// in SetValue (unlike the legacy textinput, which sanitized it to space),
	// so multi-line bodies seed cleanly without the existingMultiline detour.
	lineNum := m.diffLineNum(dl)
	var preFill string
	for _, a := range m.store.Get(m.file.name) {
		if a.Line == lineNum && a.Type == string(dl.ChangeType) {
			preFill = a.Comment
			break
		}
	}

	ta, cmd := m.newAnnotationInput(placeholder, 6) // cursor col + emoji prefix "💬 " + border margin
	if preFill != "" {
		ta.SetValue(preFill)
		ta.SetHeight(clamp(ta.LineCount(), 1, annotMaxInputHeight))
	}

	m.annot.input = ta
	m.annot.annotating = true
	m.annot.fileAnnotating = false
	m.ensureLineAnnotationInputVisible()
	return cmd
}

// ensureLineAnnotationInputVisible scrolls the viewport so the line-annotation
// input row is visible. the input is rendered below the diff line, so keeping
// the cursor line visible is not always sufficient when cursor is on the last
// visible row.
func (m *Model) ensureLineAnnotationInputVisible() {
	if !m.annot.annotating || m.annot.fileAnnotating || m.layout.viewport.Height <= 0 {
		return
	}
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return
	}

	inputY := m.cursorViewportY() + m.wrappedLineCount(m.nav.diffCursor)
	switch {
	case inputY < m.layout.viewport.YOffset:
		m.layout.viewport.SetYOffset(inputY)
	case inputY >= m.layout.viewport.YOffset+m.layout.viewport.Height:
		m.layout.viewport.SetYOffset(inputY - m.layout.viewport.Height + 1)
	}
}

// startFileAnnotation enters annotation input mode for a file-level annotation (Line=0).
func (m *Model) startFileAnnotation() tea.Cmd {
	m.clearPendingInputState()
	if m.file.name == "" {
		return nil
	}

	placeholder := "file-level annotation... (Ctrl+J newline · Ctrl+E editor · Enter save)"

	// pre-fill with existing file-level annotation if one exists. textarea
	// preserves \n in SetValue (unlike the legacy textinput).
	var preFill string
	for _, a := range m.store.Get(m.file.name) {
		if a.Line == 0 {
			preFill = a.Comment
			break
		}
	}

	ta, cmd := m.newAnnotationInput(placeholder, 12) // cursor col + "💬 file: " prefix + border margin
	if preFill != "" {
		ta.SetValue(preFill)
		ta.SetHeight(clamp(ta.LineCount(), 1, annotMaxInputHeight))
	}

	m.annot.input = ta
	m.annot.annotating = true
	m.annot.fileAnnotating = true
	m.nav.diffCursor = -1 // position cursor on the file annotation line
	m.layout.viewport.GotoTop()
	return cmd
}

// saveAnnotation saves the current text input as an annotation on the cursor line.
// Thin wrapper around saveComment that reads model state for the current target.
func (m *Model) saveAnnotation() {
	// strip leading/trailing whitespace so a stray Ctrl+J at the start or
	// trailing newlines from "type, Ctrl+J, Enter" don't pollute the saved
	// annotation. The legacy textinput sanitizer flattened \n entirely; the
	// new textarea preserves them, so explicit trimming is needed.
	text := strings.TrimSpace(m.annot.input.Value())
	if text == "" {
		m.cancelAnnotation()
		return
	}

	if m.annot.fileAnnotating {
		m.saveComment(text, m.file.name, true, 0, "")
		return
	}

	dl, ok := m.cursorDiffLine()
	if !ok {
		m.cancelAnnotation()
		return
	}
	m.saveComment(text, m.file.name, false, m.diffLineNum(dl), string(dl.ChangeType))
}

// saveComment persists the annotation text for the explicitly provided target.
// Target fields are taken as arguments (not read from model state) so the
// Enter-key path and the editor-finished path can both use it without
// temporal coupling on cursor position or current file. Hunk-end detection for
// line-level saves re-derives the diffLines index from the (line, changeType)
// pair so cursor movement during an external editor session does not skew the
// range; when fileName matches the currently loaded file, m.file.lines is
// scanned, otherwise EndLine expansion is skipped (no hunk context available).
func (m *Model) saveComment(text, fileName string, fileLevel bool, line int, changeType string) {
	if text == "" {
		m.cancelAnnotation()
		return
	}

	if fileLevel {
		m.store.Add(annotation.Annotation{File: fileName, Line: 0, Type: "", Comment: text})
		m.annot.annotating = false
		m.annot.fileAnnotating = false
	
		m.nav.diffCursor = -1 // position cursor on the file annotation line
		m.tree.RefreshFilter(m.annotatedFiles())
		m.layout.viewport.SetContent(m.renderDiff())
		m.layout.viewport.GotoTop()
		return
	}

	a := annotation.Annotation{File: fileName, Line: line, Type: changeType, Comment: text}
	if hunkKeywordRe.MatchString(text) && fileName == m.file.name {
		// re-derive the diff-line index from (line, changeType) so hunk-end
		// detection survives cursor drift during an external editor session.
		// only scan when the captured file still matches the loaded one —
		// otherwise m.file.lines describes a different file and would mislead.
		for i, dl := range m.file.lines {
			if string(dl.ChangeType) != changeType {
				continue
			}
			if m.diffLineNum(dl) != line {
				continue
			}
			if endLine := m.hunkEndLine(i); endLine > line {
				a.EndLine = endLine
			}
			break
		}
	}
	m.store.Add(a)
	m.annot.annotating = false
	m.annot.fileAnnotating = false // defensive hygiene: parity with file-level branch

	m.tree.RefreshFilter(m.annotatedFiles())
	// sync scroll so a newly added multi-row annotation stays visible when the
	// cursor sits near the bottom of the viewport.
	m.syncViewportToCursor()
}

// cancelAnnotation exits annotation input mode without saving.
func (m *Model) cancelAnnotation() {
	m.annot.annotating = false
	m.annot.fileAnnotating = false

	m.layout.viewport.SetContent(m.renderDiff())
}

// deleteFileAnnotation removes the file-level annotation and adjusts cursor position.
func (m *Model) deleteFileAnnotation() tea.Cmd {
	if !m.store.Delete(m.file.name, 0, "") {
		return nil
	}
	m.pendingAnnotJump = nil    // clear before refreshFilter which may trigger file load
	m.nav.pendingHunkJump = nil // clear before refreshFilter which may trigger file load
	m.skipInitialDividers()

	m.tree.RefreshFilter(m.annotatedFiles())

	if newFile := m.tree.SelectedFile(); newFile != "" && newFile != m.file.name {
		m.file.loadSeq++
		return m.loadFileDiff(newFile)
	}

	m.syncViewportToCursor()
	return nil
}

// deleteAnnotation removes the annotation on the current cursor line if one exists.
// handles both file-level annotations (cursor at -1) and regular line annotations.
// only works when cursor is on the annotation sub-line (cursorOnAnnotation=true) or file annotation line.
// returns a command to load the new file if the tree selection changed after filter refresh.
func (m *Model) deleteAnnotation() tea.Cmd {
	if m.cursorOnFileAnnotationLine() {
		return m.deleteFileAnnotation()
	}

	if !m.annot.cursorOnAnnotation {
		return nil
	}

	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}

	lineNum := m.diffLineNum(dl)
	if m.store.Delete(m.file.name, lineNum, string(dl.ChangeType)) {
		m.pendingAnnotJump = nil    // clear before refreshFilter which may trigger file load
		m.nav.pendingHunkJump = nil // clear before refreshFilter which may trigger file load
		m.annot.cursorOnAnnotation = false
		m.tree.RefreshFilter(m.annotatedFiles())

		// if filter moved cursor to a different file, load the new selection
		if newFile := m.tree.SelectedFile(); newFile != "" && newFile != m.file.name {
			m.file.loadSeq++
			return m.loadFileDiff(newFile)
		}

		m.syncViewportToCursor()
	}
	return nil
}

// handleAnnotateKey handles key messages during annotation input mode.
//
// Key plan (mirrors gum write):
//   - Enter (no modifier)     → save the annotation
//   - Alt+Enter, Ctrl+J       → insert newline (handled by textarea's InsertNewline binding)
//   - Esc                     → cancel
//   - Ctrl+E                  → hand off to $EDITOR for editing
//   - everything else         → forwarded to the textarea
//
// Shift+Enter is terminal-dependent: bubbletea v1's KeyMsg has no Shift
// modifier, so it cannot natively distinguish Shift+Enter from Enter. Users
// can configure their terminal to send Alt+Enter on Shift+Enter (iTerm2,
// ghostty, kitty all support this) — at which point Shift+Enter routes
// through the InsertNewline binding.
func (m Model) handleAnnotateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEnter && !msg.Alt:
		// plain Enter saves; Alt+Enter falls through to textarea's
		// InsertNewline binding (configured in newAnnotationInput).
		m.saveAnnotation()
		return m, nil
	case msg.Type == tea.KeyEsc:
		m.cancelAnnotation()
		return m, nil
	case msg.Type == tea.KeyCtrlE:
		// hand off to $EDITOR for multi-line annotation input.
		// keep annotating=true so editorFinishedMsg routes back through the annotation flow.
		cmd := m.openEditor()
		return m, cmd
	default:
		var cmd tea.Cmd
		m.annot.input, cmd = m.annot.input.Update(msg)
		// grow the textarea's visible height to match its content so the
		// in-progress annotation occupies the right number of diff-pane rows
		// (capped at annotMaxInputHeight to bound layout impact).
		m.annot.input.SetHeight(clamp(m.annot.input.LineCount(), 1, annotMaxInputHeight))
		m.layout.viewport.SetContent(m.renderDiff()) // re-render so typed characters are visible immediately
		return m, cmd
	}
}

// cursorLineHasAnnotation checks if the cursor is on a deletable annotation line.
// returns true only when cursor is on the file annotation line or on an annotation sub-line.
func (m Model) cursorLineHasAnnotation() bool {
	return m.cursorOnFileAnnotationLine() || m.annot.cursorOnAnnotation
}

// hasFileAnnotation checks if the current file has visible file-level
// annotation content — either a saved annotation in the store, or an
// in-progress textarea whose value will be saved as a file-level annotation.
// Both cases produce rows that need to be counted by the cursor-math layer.
func (m Model) hasFileAnnotation() bool {
	if m.annot.annotating && m.annot.fileAnnotating {
		return true
	}
	for _, a := range m.store.Get(m.file.name) {
		if a.Line == 0 {
			return true
		}
	}
	return false
}

// cursorOnFileAnnotationLine returns true if the diff cursor is on the file-level annotation line.
func (m Model) cursorOnFileAnnotationLine() bool {
	return m.nav.diffCursor == -1 && m.hasFileAnnotation()
}

// diffLineNum returns the display line number for a diff line.
func (m Model) diffLineNum(dl diff.DiffLine) int {
	if dl.ChangeType == diff.ChangeRemove {
		return dl.OldNum
	}
	return dl.NewNum
}

// hunkEndLine returns the display line number of the last line in the change hunk
// containing diffLines[idx]. only walks forward through lines of the same change type
// as the starting line, so both start and end use the same number space (old or new).
// returns 0 if idx is not inside a change hunk.
func (m Model) hunkEndLine(idx int) int {
	if idx < 0 || idx >= len(m.file.lines) {
		return 0
	}
	dl := m.file.lines[idx]
	if dl.ChangeType != diff.ChangeAdd && dl.ChangeType != diff.ChangeRemove {
		return 0
	}

	// walk forward from idx to find the last contiguous line of the same change type
	startType := dl.ChangeType
	last := idx
	for i := idx + 1; i < len(m.file.lines); i++ {
		if m.file.lines[i].ChangeType != startType {
			break
		}
		last = i
	}
	return m.diffLineNum(m.file.lines[last])
}

// wrappedAnnotationLineCount returns the number of visual rows an annotation
// occupies in the diff pane. Delegates to annotationVisualRows so cursor math
// (this function) and paint math (renderWrappedAnnotation) read from the same
// cached row slice — see CLAUDE.md "Annotation visual-row invariant".
//
// While the user is actively typing into the annotation textarea (no saved
// annotation exists yet for this line), returns the textarea's current
// visible height instead, so the diff content below the input scrolls
// correctly as newlines are inserted.
func (m Model) wrappedAnnotationLineCount(key string) int {
	if rows := m.activeInputRowCount(key); rows > 0 {
		return rows
	}
	prefix, body := m.annotationPrefixBody(key)
	if body == "" {
		return 1
	}
	rows := m.annotationVisualRows(prefix, body)
	if len(rows) == 0 {
		return 1
	}
	return len(rows)
}

// activeInputRowCount returns the visible row count of the in-progress
// annotation textarea when key identifies the line currently being annotated,
// or 0 otherwise. The textarea's Height is kept in sync with its LineCount in
// handleAnnotateKey, so this is exactly what renderInProgressAnnotation will
// emit. Long unwrapped logical lines may exceed Height — for those, prefer
// Ctrl+J to insert a deliberate newline.
func (m Model) activeInputRowCount(key string) int {
	if !m.annot.annotating {
		return 0
	}
	if m.annot.fileAnnotating {
		if key != annotKeyFile {
			return 0
		}
	} else {
		if key == annotKeyFile {
			return 0
		}
		if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
			return 0
		}
		dl := m.file.lines[m.nav.diffCursor]
		if m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType)) != key {
			return 0
		}
	}
	return clamp(m.annot.input.LineCount(), 1, annotMaxInputHeight)
}

// annotationPrefixBody returns the leading prefix ("💬 " or "💬 file: ") and
// the raw comment body for the annotation identified by key, or ("", "") if
// no annotation matches. The prefix is what the painter prepends to row 0 of
// the rendered body; rows 1+ get a matching-width plain-space indent.
func (m Model) annotationPrefixBody(key string) (prefix, body string) {
	for _, a := range m.store.Get(m.file.name) {
		if key == annotKeyFile && a.Line == 0 {
			return "\U0001f4ac file: ", a.Comment
		}
		if key != annotKeyFile && m.annotationKey(a.Line, a.Type) == key {
			return "\U0001f4ac ", a.Comment
		}
	}
	return "", ""
}

// annotationVisualRows is the single source of truth for "what does this
// annotation paint as." Both wrappedAnnotationLineCount and
// renderWrappedAnnotation read from the same memoized []string here — every
// caller that needs to count or paint an annotation MUST go through this
// method or cursor scroll math will desync from rendered output.
//
// Returned rows are fully styled and include their leading prefix (row 0) or
// matching-width indent (row 1+); the painter just prepends the cursor cell
// and right-pads with extendLineBg. The legacy and markdown paths produce
// rows in the same shape but with different styling: legacy wraps prefix+body
// in a single AnnotationInline envelope (compact ANSI, matches pre-glamour
// output exactly); markdown emits a styled prefix followed by glamour's per-
// element styled body.
func (m Model) annotationVisualRows(prefix, body string) []string {
	if body == "" {
		return nil
	}
	contentW := m.diffContentWidth() - 1 - lipgloss.Width(prefix)
	if contentW < wrapMinContent {
		contentW = wrapMinContent
	}
	key := annotCacheKey{body: body, prefix: prefix, width: contentW}
	if rows, ok := m.annot.rowCache[key]; ok {
		return rows
	}

	var rows []string
	if m.annot.markdown != nil {
		bodyRows := m.annot.markdown.Render(body, contentW)
		if len(bodyRows) > 0 {
			rows = m.composeMarkdownRows(prefix, bodyRows)
		}
	}
	if len(rows) == 0 {
		rows = m.composeLegacyRows(prefix, body, contentW)
	}
	if rows == nil {
		rows = []string{}
	}
	m.annot.rowCache[key] = rows
	return rows
}

// composeLegacyRows returns the pre-glamour annotation rows, preserving the
// exact shape of the original italic-prose path: each row is an
// AnnotationInline-styled string spanning either prefix+body (row 0) or
// indent+body (row 1+), so the entire visible content lives inside a single
// SGR envelope per row. Embedded "\n" splits into logical lines; wrapping
// happens at full pane-content width, ignoring indent for wrap-row
// continuations of a single logical line (matching old-loop behavior).
func (m Model) composeLegacyRows(prefix, body string, contentW int) []string {
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	wrapW := contentW + lipgloss.Width(prefix) // = pane-content - 1
	var rows []string
	for li, logical := range strings.Split(body, "\n") {
		head := indent
		if li == 0 {
			head = prefix
		}
		segment := head + logical
		var wrapped []string
		if wrapW > wrapMinContent && lipgloss.Width(segment) > wrapW {
			wrapped = m.wrapContent(segment, wrapW)
		} else {
			wrapped = []string{segment}
		}
		for _, w := range wrapped {
			rows = append(rows, m.renderer.AnnotationInline(w))
		}
	}
	return rows
}

// composeMarkdownRows takes pre-styled body rows from the markdown renderer
// and prepends a styled prefix on row 0, plain-space indent on rows 1+. The
// prefix is wrapped in its own AnnotationInline envelope (separate from the
// glamour-styled body) because glamour's per-element styling does not extend
// across externally-injected text — composing them in a single envelope
// would either re-style the body or de-style the prefix.
func (m Model) composeMarkdownRows(prefix string, bodyRows []string) []string {
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	prefixStyled := m.renderer.AnnotationInline(prefix)
	out := make([]string, len(bodyRows))
	out[0] = prefixStyled + bodyRows[0]
	for i := 1; i < len(bodyRows); i++ {
		out[i] = indent + bodyRows[i]
	}
	return out
}

// invalidateAnnotationRows clears the visual-row cache. Call on file load
// (annotation set may have changed) and theme apply (styling colors may have
// changed). Width-only changes self-invalidate via the cache key, so resize
// does not need to clear.
func (m *Model) invalidateAnnotationRows() {
	for k := range m.annot.rowCache {
		delete(m.annot.rowCache, k)
	}
}

// hunkLineHeight returns the visual row count for a single diff line,
// including collapsed visibility, wrap, and inline annotation.
func (m Model) hunkLineHeight(idx int, hunks []int, annotationSet map[string]bool) int {
	if m.isCollapsedHidden(idx, hunks) {
		return 0
	}
	if m.isDeleteOnlyPlaceholder(idx, hunks) {
		return m.deletePlaceholderVisualHeight(idx)
	}
	h := m.wrappedLineCount(idx)
	dl := m.file.lines[idx]
	if dl.ChangeType != diff.ChangeDivider {
		key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
		// Count saved annotations OR an in-progress textarea on this line:
		// renderAnnotationOrInput emits rows for either case, so the height
		// math must agree or cursor scroll desyncs (Ctrl+J in a brand-new
		// annotation grows the textarea but the desync hides the new rows
		// until the user scrolls).
		if annotationSet[key] || m.activeInputRowCount(key) > 0 {
			h += m.wrappedAnnotationLineCount(key)
		}
	}
	return h
}

// cursorViewportY computes the actual viewport Y position of the cursor,
// accounting for injected annotation lines and the file-level annotation line.
// in collapsed mode, hidden removed lines (those in non-expanded hunks) are not counted.
func (m Model) cursorViewportY() int {
	var hunks []int
	if m.modes.collapsed.enabled {
		hunks = m.findHunks()
	}
	return m.cursorViewportYUsing(hunks, m.buildAnnotationSet())
}

// cursorViewportYUsing is the same as cursorViewportY but accepts pre-built
// hunks and annotationSet to avoid redundant computation when the caller
// already has them (e.g. centerHunkInViewport).
func (m Model) cursorViewportYUsing(hunks []int, annotationSet map[string]bool) int {
	if m.file.name == "" || len(m.file.lines) == 0 {
		return max(0, m.nav.diffCursor)
	}

	fileAnnotationOffset := 0
	if m.hasFileAnnotation() {
		fileAnnotationOffset = m.wrappedAnnotationLineCount(annotKeyFile)
	}

	if m.nav.diffCursor == -1 {
		return 0
	}

	y := fileAnnotationOffset
	for i := 0; i < m.nav.diffCursor && i < len(m.file.lines); i++ {
		y += m.hunkLineHeight(i, hunks, annotationSet)
	}
	if m.annot.cursorOnAnnotation {
		y += m.wrappedLineCount(m.nav.diffCursor)
	}
	return y
}

// cursorVisualRange returns the top and bottom viewport Y coordinates the
// cursor currently occupies. when the cursor is on a diff row, bottom spans
// any wrap-continuation rows plus any injected annotation rows below it;
// when the cursor is on an annotation sub-line, bottom spans only the
// annotation rows (the diff row sits above top). callers keeping the cursor
// "visible" use this range to preserve the full logical extent, not just the
// top row.
func (m Model) cursorVisualRange() (top, bottom int) {
	var hunks []int
	if m.modes.collapsed.enabled {
		hunks = m.findHunks()
	}
	annotationSet := m.buildAnnotationSet()
	top = m.cursorViewportYUsing(hunks, annotationSet)
	h := max(m.cursorVisualHeight(hunks, annotationSet), 1)
	return top, top + h - 1
}

// rowOnAnnotationSubLine reports whether relRow (0-based, relative to the first
// visual row of the diff line at idx) targets the injected annotation sub-line
// below that diff line. h is the total visual height of the diff line (wrap rows
// plus any injected annotation rows). delete-only placeholders always return
// false because renderCollapsedDiff skips annotation rendering for them, even
// when the underlying removed line has an annotation.
func (m Model) rowOnAnnotationSubLine(idx, relRow, h int, hunks []int, annSet map[string]bool) bool {
	if m.isDeleteOnlyPlaceholder(idx, hunks) {
		return false
	}
	dl := m.file.lines[idx]
	if dl.ChangeType == diff.ChangeDivider {
		return false
	}
	key := m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType))
	if !annSet[key] {
		return false
	}
	annRows := m.wrappedAnnotationLineCount(key)
	return annRows > 0 && relRow >= h-annRows
}

// visualRowToDiffLine maps a visual row within the diff viewport content back
// to a diff-line index. row is 0-based relative to the first visible content
// row of the viewport (caller must add YOffset and subtract any header rows).
// when the file has a file-level annotation, rows covered by that annotation
// map to idx=-1. the onAnnotation return value mirrors the semantics of
// m.annot.cursorOnAnnotation: true when the row falls on an injected
// annotation sub-line rather than the diff line (or its wrap-continuation)
// itself. this is the inverse of cursorVisualRange + cursorViewportYUsing.
//
// edge cases: empty m.file.lines returns (m.nav.diffCursor, false); row < 0
// returns (-1, false) when a file-level annotation is present, else
// (firstVisibleIdx, false); rows past the last line return the last valid
// index.
func (m Model) visualRowToDiffLine(row int) (idx int, onAnnotation bool) {
	if len(m.file.lines) == 0 {
		if m.hasFileAnnotation() && row >= 0 && row < m.wrappedAnnotationLineCount(annotKeyFile) {
			return -1, false
		}
		return m.nav.diffCursor, false
	}

	var hunks []int
	if m.modes.collapsed.enabled {
		hunks = m.findHunks()
	}
	annSet := m.buildAnnotationSet()

	running := 0
	if m.hasFileAnnotation() {
		fileRows := m.wrappedAnnotationLineCount(annotKeyFile)
		if row < fileRows {
			return -1, false
		}
		running = fileRows
	} else if row < 0 {
		// no file annotation, row above the top: pick the first visible line
		for i := range m.file.lines {
			if m.hunkLineHeight(i, hunks, annSet) > 0 {
				return i, false
			}
		}
		return 0, false
	}

	for i := range m.file.lines {
		h := m.hunkLineHeight(i, hunks, annSet)
		if h == 0 {
			continue
		}
		if row < running+h {
			return i, m.rowOnAnnotationSubLine(i, row-running, h, hunks, annSet)
		}
		running += h
	}
	// row past the last visible line: return the last visible line index
	for i := range slices.Backward(m.file.lines) {
		if m.hunkLineHeight(i, hunks, annSet) > 0 {
			return i, false
		}
	}
	return len(m.file.lines) - 1, false
}

// cursorVisualHeight returns the number of visual rows occupied by the cursor.
// branches, in order of evaluation:
//   - cursor on the file-level annotation line → file annotation's wrapped row count
//   - diffCursor out of range (empty or not loaded) → 1
//   - cursor on annotation sub-line of a divider (defensive; unreachable today) → 1
//   - cursor on annotation sub-line of a regular line → annotation's own wrapped row count
//   - cursor on the diff row → hunkLineHeight (wrap + injected annotation rows)
//
// hunks and annotationSet are supplied by the caller to avoid redundant
// computation when they are already built; both must describe the current file.
func (m Model) cursorVisualHeight(hunks []int, annotationSet map[string]bool) int {
	if m.cursorOnFileAnnotationLine() {
		return m.wrappedAnnotationLineCount(annotKeyFile)
	}
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return 1
	}
	dl := m.file.lines[m.nav.diffCursor]
	if m.annot.cursorOnAnnotation {
		if dl.ChangeType == diff.ChangeDivider {
			return 1
		}
		return m.wrappedAnnotationLineCount(m.annotationKey(m.diffLineNum(dl), string(dl.ChangeType)))
	}
	return m.hunkLineHeight(m.nav.diffCursor, hunks, annotationSet)
}

// buildAnnotationSet returns a set of annotation keys for the current file.
// excludes file-level annotations (Line=0) since they are rendered separately.
func (m Model) buildAnnotationSet() map[string]bool {
	annotations := m.store.Get(m.file.name)
	set := make(map[string]bool, len(annotations))
	for _, a := range annotations {
		if a.Line == 0 {
			continue
		}
		set[m.annotationKey(a.Line, a.Type)] = true
	}
	return set
}

// annotationKey creates a lookup key from line number and change type.
func (m Model) annotationKey(line int, changeType string) string {
	return fmt.Sprintf("%d:%s", line, changeType)
}
