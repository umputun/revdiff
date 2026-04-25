package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
)

// View renders the full TUI.
func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}
	// ready but the first filesLoadedMsg hasn't landed yet — the file tree is still
	// nil-populated and the diff pane has no file selected. Showing the empty two-pane
	// layout here would flash a misleading "no changes" state for as long as ChangedFiles
	// takes to return (can be 100-500ms on large repos).
	if !m.filesLoaded {
		return "loading files..."
	}

	ph := m.paneHeight()

	// determine diff pane inner width up front so the header can be truncated
	// to a single visual row before lipgloss renders. without truncation, a
	// long filename causes lipgloss to soft-wrap the header onto multiple
	// rows, which would push viewport rows past applyScrollbar's hardcoded
	// scrollbarFirstViewportRow offset.
	var diffPaneW int
	if m.treePaneHidden() {
		diffPaneW = m.layout.width - 2
	} else {
		diffPaneW = m.layout.width - m.layout.treeWidth - 4
	}

	// diff pane title
	diffTitle := "no file selected"
	if m.file.name != "" {
		diffTitle = m.file.name
	}
	diffHeader := m.resolver.Style(style.StyleKeyDirEntry).Render(m.truncateHeaderTitle(diffTitle, diffPaneW))
	diffContent := lipgloss.JoinVertical(lipgloss.Left, diffHeader, m.layout.viewport.View())

	var mainView string
	switch {
	case m.treePaneHidden():
		// tree pane hidden (user toggle or single-file without TOC): diff uses full width
		diffContent = m.padContentBg(diffContent, diffPaneW, m.resolver.Color(style.ColorKeyDiffPaneBg))
		diffPane := m.resolver.Style(style.StyleKeyDiffPaneActive).
			Width(diffPaneW).
			Height(ph).
			Render(diffContent)
		mainView = m.applyScrollbar(diffPane)

	case m.file.singleFile && m.file.mdTOC != nil:
		// single-file markdown with TOC: two-pane layout with TOC in left pane
		tocContent := m.file.mdTOC.Render(sidepane.TOCRender{Width: m.layout.treeWidth, Height: ph, Focused: m.layout.focus == paneTree, Resolver: m.resolver})
		mainView = m.renderTwoPaneLayout(tocContent, diffContent, ph, diffPaneW)

	default:
		annotated := m.annotatedFiles()
		treeContent := m.tree.Render(sidepane.FileTreeRender{Width: m.layout.treeWidth, Height: ph, Annotated: annotated, Resolver: m.resolver, Renderer: m.renderer})
		mainView = m.renderTwoPaneLayout(treeContent, diffContent, ph, diffPaneW)
	}

	mainView = m.overlay.Compose(mainView, overlay.RenderCtx{Width: m.layout.width, Height: m.layout.height, Resolver: m.resolver})

	if m.cfg.noStatusBar {
		return mainView
	}

	status := m.resolver.Style(style.StyleKeyStatusBar).Width(m.layout.width).Render(m.statusBarText())
	return lipgloss.JoinVertical(lipgloss.Left, mainView, status)
}

// renderTwoPaneLayout renders a two-pane layout with left (tree/TOC) and right (diff) content.
// applies focus-based pane styles, background padding, and joins horizontally.
// diffPaneW is the inner width caller passed to truncateHeaderTitle and must
// match the lipgloss Width() applied here — single source of truth for the
// scrollbar's single-line-header invariant.
func (m Model) renderTwoPaneLayout(leftContent, diffContent string, ph, diffPaneW int) string {
	treeStyle := m.resolver.Style(style.StyleKeyTreePane)
	diffStyle := m.resolver.Style(style.StyleKeyDiffPane)
	if m.layout.focus == paneTree {
		treeStyle = m.resolver.Style(style.StyleKeyTreePaneActive)
	} else {
		diffStyle = m.resolver.Style(style.StyleKeyDiffPaneActive)
	}

	leftContent = m.padContentBg(leftContent, m.layout.treeWidth, m.resolver.Color(style.ColorKeyTreePaneBg))
	diffContent = m.padContentBg(diffContent, diffPaneW, m.resolver.Color(style.ColorKeyDiffPaneBg))

	leftPane := treeStyle.
		Width(m.layout.treeWidth).
		Height(ph).
		Render(leftContent)

	diffPane := diffStyle.
		Width(diffPaneW).
		Height(ph).
		Render(diffContent)
	diffPane = m.applyScrollbar(diffPane)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, diffPane)
}

// sanitizeFilenameForDisplay strips characters that would break or spoof
// header/status-bar layout: C0 controls (< 0x20), DEL (0x7F), C1 controls
// (0x80–0x9F), the Unicode replacement character (U+FFFD), and Unicode
// format/bidi controls (RTL/LTR overrides U+202A–U+202E, isolates
// U+2066–U+2069, ZWJ/ZWNJ U+200D/U+200C, ZWSP U+200B, BOM U+FEFF).
// POSIX permits the C0/C1 bytes in paths; ingesting them raw lets crafted
// paths re-wrap the diff header (and re-break the scrollbar's single-line
// invariant) or inject terminal escape sequences. The bidi/format strip
// is a defense-in-depth measure against filename spoofing — the chars
// are zero-width so they do not affect width math, but they can make a
// path render as something the user did not actually approve.
func (m Model) sanitizeFilenameForDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7F, r >= 0x80 && r <= 0x9F:
			return -1
		case r == utf8.RuneError:
			return -1
		case r >= 0x200B && r <= 0x200F: // ZWSP, ZWNJ, ZWJ, LRM, RLM
			return -1
		case r >= 0x202A && r <= 0x202E: // bidi overrides + embeddings
			return -1
		case r >= 0x2066 && r <= 0x2069: // bidi isolates
			return -1
		case r == 0xFEFF: // BOM / ZWNBSP
			return -1
		}
		return r
	}, s)
}

// truncateLeftToWidth left-truncates s with a leading "…" so it fits in
// budget visual columns, preserving the meaningful end. returns s unchanged
// when it already fits, "" when budget <= 0, "…" when budget == 1.
func (m Model) truncateLeftToWidth(s string, budget int) string {
	if lipgloss.Width(s) <= budget {
		return s
	}
	if budget <= 0 {
		return ""
	}
	if budget == 1 {
		return "…"
	}
	tailBudget := budget - 1 // 1 cell for the leading "…"
	runes := []rune(s)
	w, cutIdx := 0, len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if w+rw > tailBudget {
			break
		}
		w += rw
		cutIdx = i
	}
	return "…" + string(runes[cutIdx:])
}

// truncateHeaderTitle returns the diff pane header text shortened to fit
// in exactly one visual row of width paneW, prefixed with the leading
// single-cell space the header always renders with. control characters
// (newline, ESC, etc.) are stripped first so crafted filenames cannot
// re-wrap the header. left-truncation keeps the meaningful end of long
// paths. extreme-narrow paneW values (≤ 1) delegate to truncateLeftToWidth
// without the leading space; these are not produced by any realistic
// terminal layout but the helper must not overflow.
func (m Model) truncateHeaderTitle(title string, paneW int) string {
	clean := m.sanitizeFilenameForDisplay(title)
	full := " " + clean
	if lipgloss.Width(full) <= paneW {
		return full
	}
	if paneW <= 1 {
		return m.truncateLeftToWidth(clean, paneW)
	}
	// paneW >= 2: " " + truncateLeftToWidth(clean, paneW-1) fits in paneW
	return " " + m.truncateLeftToWidth(clean, paneW-1)
}

// transientHint returns the first non-empty transient status-bar hint. hints
// are cleared on the next key press (see handleKey). priority matches the
// display order: commits > reload > compact > keys > vim. returns "" when no
// hint is set. chord (keys) hints and vim-motion hints are lowest priority —
// an in-flight reload or a compact-mode toggle wins, since those hints are
// user-driven and recoverable. vim hints sit below keys since vim-motion
// feedback is the most recoverable of the group.
func (m Model) transientHint() string {
	switch {
	case m.commits.hint != "":
		return m.commits.hint
	case m.reload.hint != "":
		return m.reload.hint
	case m.compact.hint != "":
		return m.compact.hint
	case m.keys.hint != "":
		return m.keys.hint
	case m.vim.hint != "":
		return m.vim.hint
	}
	return ""
}

// statusBarText returns context-sensitive status line content.
// shows search input (when typing), or filename, diff stats, hunk position,
// search match position, mode indicators, and right-aligned annotation count + help hint.
func (m Model) statusBarText() string {
	if m.search.active {
		return m.searchBarText()
	}

	if m.inConfirmDiscard {
		return fmt.Sprintf("discard %d annotations? [y/n]", m.store.Count())
	}

	if m.annot.annotating {
		return "[enter] save  [esc] cancel"
	}

	if hint := m.transientHint(); hint != "" {
		return hint
	}

	// build left-side segments
	var segments []string

	// filename and diff stats segments. sanitize the filename so crafted
	// paths (newline/ESC/bidi controls) cannot break or spoof status-bar
	// layout — same defense-in-depth applied to the diff header.
	cleanName := m.sanitizeFilenameForDisplay(m.file.name)
	if cleanName != "" {
		segments = append(segments, cleanName, m.fileStatsText())
	}

	// hunk position (always shown in diff pane when there are hunks)
	if hs := m.hunkSegment(); hs != "" {
		segments = append(segments, hs)
	}

	// line number position
	if ls := m.lineNumberSegment(); ls != "" {
		segments = append(segments, ls)
	}

	// search match position
	if ss := m.searchSegment(); ss != "" {
		segments = append(segments, ss)
	}

	// build right-side segments
	var rightParts []string
	if rc := m.tree.ReviewedCount(); rc > 0 {
		rightParts = append(rightParts, fmt.Sprintf("✓ %d/%d", rc, m.tree.TotalFiles()))
	}
	if cnt := m.store.Count(); cnt > 0 {
		suffix := "annotations"
		if cnt == 1 {
			suffix = "annotation"
		}
		rightParts = append(rightParts, fmt.Sprintf("%d %s", cnt, suffix))
	}
	rightParts = append(rightParts, m.statusModeIcons(), "? help")

	// build separator with muted foreground using raw ANSI (not lipgloss.Render)
	// to avoid full reset that would break the status bar background
	sep := m.renderer.StatusBarSeparator()
	left := strings.Join(segments, sep)
	right := strings.Join(rightParts, sep)

	// truncate filename from left with … if status line is too wide
	minRight := lipgloss.Width(right) + 5 // 2 for status bar padding + 3 for separator
	available := max(m.layout.width-minRight, 0)

	// graceful degradation: drop left segments when too narrow
	if lipgloss.Width(left) > available {
		// rebuild without search position
		segments = m.statusSegmentsNoSearch()
		left = strings.Join(segments, sep)
	}
	if lipgloss.Width(left) > available {
		// rebuild without hunk info and line number
		segments = m.statusSegmentsMinimal()
		left = strings.Join(segments, sep)
	}
	if lipgloss.Width(left) > available && cleanName != "" {
		// truncate filename from left, keeping end of path. uses display-width
		// measurement to handle wide characters (CJK, emoji)
		statsStr := m.fileStatsText()
		nameMax := max(available-lipgloss.Width(statsStr)-lipgloss.Width(sep), 4) // reserve separator between name and stats
		left = m.truncateLeftToWidth(cleanName, nameMax) + sep + statsStr
	}

	return m.joinStatusSections(left, right, sep)
}

// hunkSegment returns a formatted hunk position string for the status line.
// returns "hunk X/Y" when cursor is on a changed line, "N hunks"/"1 hunk" otherwise, or empty if not in diff pane.
func (m Model) hunkSegment() string {
	if m.layout.focus != paneDiff {
		return ""
	}
	cur, total := m.currentHunk()
	if total == 0 {
		return ""
	}
	if cur > 0 {
		return fmt.Sprintf("hunk %d/%d", cur, total)
	}
	if total == 1 {
		return "1 hunk"
	}
	return fmt.Sprintf("%d hunks", total)
}

// lineNumberSegment returns a formatted line number string like "L:42/380" for the status line.
// The denominator is dynamic: on removed lines it shows the old file's max line number,
// on context/added lines it shows the new file's max line number.
// Returns empty string when focus is not on diff pane, cursor is out of range, or on a divider line.
func (m Model) lineNumberSegment() string {
	if m.layout.focus != paneDiff {
		return ""
	}
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return ""
	}
	dl := m.file.lines[m.nav.diffCursor]
	if dl.ChangeType == diff.ChangeDivider {
		return ""
	}
	lineNum := m.diffLineNum(dl)
	if lineNum == 0 {
		return ""
	}
	var maxOld, maxNew int
	for _, l := range m.file.lines {
		if l.OldNum > maxOld {
			maxOld = l.OldNum
		}
		if l.NewNum > maxNew {
			maxNew = l.NewNum
		}
	}
	total := maxNew
	if dl.ChangeType == diff.ChangeRemove {
		total = maxOld
	}
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("L:%d/%d", lineNum, total)
}

// joinStatusSections joins left and right status sections with padding and separators.
func (m Model) joinStatusSections(left, right, sep string) string {
	sepWidth := lipgloss.Width(sep)
	padding := m.layout.width - lipgloss.Width(left) - lipgloss.Width(right) - 2 // 2 for status bar padding
	if left != "" && padding > sepWidth {
		return left + sep + strings.Repeat(" ", padding-sepWidth) + right
	}
	if padding > 0 {
		return left + strings.Repeat(" ", padding) + right
	}
	if left != "" {
		return left + sep + right
	}
	return right
}

// searchBarText returns the status bar content during search input mode.
func (m Model) searchBarText() string {
	return "/" + m.search.input.Value()
}

// searchSegment returns a formatted search position string like "X/Y" for the status line.
// returns empty string when no search matches exist. shows 0/N when all matches are hidden
// in collapsed mode (e.g. matches only on removed lines).
func (m Model) searchSegment() string {
	if len(m.search.matches) == 0 {
		return ""
	}
	pos := m.search.cursor + 1
	if m.modes.collapsed.enabled && m.search.cursor < len(m.search.matches) {
		hunks := m.findHunks()
		if m.isCollapsedHidden(m.search.matches[m.search.cursor], hunks) {
			pos = 0
		}
	}
	return fmt.Sprintf("%d/%d", pos, len(m.search.matches))
}

// padContentBg pads every line in content to targetWidth using the given ANSI background color.
// strips trailing plain spaces first (left by viewport/lipgloss padding after \033[0m reset),
// then re-pads with bg-colored spaces. this ensures the background fills the entire pane
// interior, working around lipgloss full-reset that kills outer pane backgrounds.
// no-op when bg is empty.
func (m Model) padContentBg(content string, targetWidth int, bg style.Color) string {
	if bg == "" || targetWidth <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// strip trailing plain spaces left by viewport/lipgloss padding after ANSI reset
		trimmed := strings.TrimRight(line, " ")
		w := lipgloss.Width(trimmed)
		pad := targetWidth - w
		if pad > 0 {
			lines[i] = trimmed + string(bg) + strings.Repeat(" ", pad) + "\033[49m"
		} else {
			lines[i] = trimmed
		}
	}
	return strings.Join(lines, "\n")
}

// statusModeIcons returns combined mode indicator icons (one per view toggle).
// all icons are always shown; active modes use status foreground, inactive use muted color.
func (m Model) statusModeIcons() string {
	type indicator struct {
		icon   string
		active bool
	}
	indicators := []indicator{
		{"▼", m.modes.collapsed.enabled},
		{"⊂", m.modes.compact},
		{"◉", m.tree.FilterActive()},
		{"↩", m.modes.wrap},
		{"≋", len(m.search.matches) > 0},
		{"⊟", m.layout.treeHidden},
		{"#", m.modes.lineNumbers},
		{"b", m.modes.showBlame},
		{"±", m.modes.wordDiff},
		{"✓", m.tree.ReviewedCount() > 0},
		{"∅", m.modes.showUntracked},
	}

	mutedSeq := string(m.resolver.Color(style.ColorKeyMutedFg))
	activeSeq := string(m.resolver.Color(style.ColorKeyStatusFg))

	icons := make([]string, 0, len(indicators))
	for _, ind := range indicators {
		if ind.active {
			icons = append(icons, activeSeq+ind.icon)
		} else {
			icons = append(icons, mutedSeq+ind.icon)
		}
	}
	return strings.Join(icons, " ") + activeSeq
}

// statusSegmentsNoSearch returns left segments without search position (for narrow terminals).
func (m Model) statusSegmentsNoSearch() []string {
	var segments []string
	if name := m.sanitizeFilenameForDisplay(m.file.name); name != "" {
		segments = append(segments, name, m.fileStatsText())
	}
	if hs := m.hunkSegment(); hs != "" {
		segments = append(segments, hs)
	}
	if ls := m.lineNumberSegment(); ls != "" {
		segments = append(segments, ls)
	}
	return segments
}

// statusSegmentsMinimal returns left segments with only filename and stats.
func (m Model) statusSegmentsMinimal() []string {
	var segments []string
	if name := m.sanitizeFilenameForDisplay(m.file.name); name != "" {
		segments = append(segments, name, m.fileStatsText())
	}
	return segments
}
