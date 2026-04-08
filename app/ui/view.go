package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
)

// View renders the full TUI.
func (m Model) View() string {
	if !m.ready {
		return "loading..."
	}

	ph := m.paneHeight()

	// diff pane title
	diffTitle := "no file selected"
	if m.currFile != "" {
		diffTitle = m.currFile
	}
	diffHeader := m.styles.DirEntry.Render(" " + diffTitle)
	diffContent := lipgloss.JoinVertical(lipgloss.Left, diffHeader, m.viewport.View())

	var mainView string
	switch {
	case m.treeHidden || (m.singleFile && m.mdTOC == nil):
		// tree pane hidden (user toggle or single-file without TOC): diff uses full width
		paneW := m.width - 2
		diffContent = m.padContentBg(diffContent, paneW, m.styles.colors.DiffBg)
		diffPane := m.styles.DiffPaneActive.
			Width(paneW).
			Height(ph).
			Render(diffContent)
		mainView = diffPane

	case m.singleFile && m.mdTOC != nil:
		// single-file markdown with TOC: two-pane layout with TOC in left pane
		tocContent := m.mdTOC.render(m.treeWidth, ph, m.focus, m.styles)

		treeStyle := m.styles.TreePane
		diffStyle := m.styles.DiffPane
		if m.focus == paneTree {
			treeStyle = m.styles.TreePaneActive
		} else {
			diffStyle = m.styles.DiffPaneActive
		}

		diffW := m.width - m.treeWidth - 4
		tocContent = m.padContentBg(tocContent, m.treeWidth, m.styles.colors.TreeBg)
		diffContent = m.padContentBg(diffContent, diffW, m.styles.colors.DiffBg)

		tocPane := treeStyle.
			Width(m.treeWidth).
			Height(ph).
			Render(tocContent)

		diffPane := diffStyle.
			Width(diffW).
			Height(ph).
			Render(diffContent)

		mainView = lipgloss.JoinHorizontal(lipgloss.Top, tocPane, diffPane)

	default:
		annotated := m.annotatedFiles()
		treeContent := m.tree.render(m.treeWidth, ph, annotated, m.styles)

		// apply pane borders based on focus
		treeStyle := m.styles.TreePane
		diffStyle := m.styles.DiffPane
		if m.focus == paneTree {
			treeStyle = m.styles.TreePaneActive
		} else {
			diffStyle = m.styles.DiffPaneActive
		}

		diffW := m.width - m.treeWidth - 4
		treeContent = m.padContentBg(treeContent, m.treeWidth, m.styles.colors.TreeBg)
		diffContent = m.padContentBg(diffContent, diffW, m.styles.colors.DiffBg)

		treePane := treeStyle.
			Width(m.treeWidth).
			Height(ph).
			Render(treeContent)

		diffPane := diffStyle.
			Width(diffW).
			Height(ph).
			Render(diffContent)

		mainView = lipgloss.JoinHorizontal(lipgloss.Top, treePane, diffPane)
	}

	switch {
	case m.themeSel.active:
		mainView = m.overlayCenter(mainView, m.themeSelectOverlay())
	case m.showAnnotList:
		mainView = m.overlayCenter(mainView, m.annotListOverlay())
	case m.showHelp:
		mainView = m.overlayCenter(mainView, m.helpOverlay())
	}

	if m.noStatusBar {
		return mainView
	}

	status := m.styles.StatusBar.Width(m.width).Render(m.statusBarText())
	return lipgloss.JoinVertical(lipgloss.Left, mainView, status)
}

// statusBarText returns context-sensitive status line content.
// shows search input (when typing), or filename, diff stats, hunk position,
// search match position, mode indicators, and right-aligned annotation count + help hint.
func (m Model) statusBarText() string {
	if m.searching {
		return m.searchBarText()
	}

	if m.inConfirmDiscard {
		return fmt.Sprintf("discard %d annotations? [y/n]", m.store.Count())
	}

	if m.annotating {
		return "[enter] save  [esc] cancel"
	}

	// build left-side segments
	var segments []string

	// filename and diff stats segments
	if m.currFile != "" {
		segments = append(segments, m.currFile, m.fileStatsText())
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
	if rc := m.tree.reviewedCount(); rc > 0 {
		rightParts = append(rightParts, fmt.Sprintf("✓ %d/%d", rc, len(m.tree.allFiles)))
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
	statusFg := m.styles.colors.Muted
	if m.styles.colors.StatusFg != "" {
		statusFg = m.styles.colors.StatusFg
	}
	sep := " " + m.ansiFg(m.styles.colors.Muted) + "|" + m.ansiFg(statusFg) + " "
	left := strings.Join(segments, sep)
	right := strings.Join(rightParts, sep)

	// truncate filename from left with … if status line is too wide
	minRight := lipgloss.Width(right) + 5 // 2 for status bar padding + 3 for separator
	available := max(m.width-minRight, 0)

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
	if lipgloss.Width(left) > available && m.currFile != "" {
		// truncate filename from left, keeping end of path.
		// uses display-width measurement to handle wide characters (CJK, emoji)
		statsStr := m.fileStatsText()
		nameMax := max(available-lipgloss.Width(statsStr)-lipgloss.Width(sep), 4) // reserve separator between name and stats
		name := m.currFile
		if lipgloss.Width(name) > nameMax {
			budget := nameMax - 1 // reserve 1 cell for "…"
			runes := []rune(name)
			w, cutIdx := 0, len(runes)
			for i := len(runes) - 1; i >= 0; i-- {
				rw := runewidth.RuneWidth(runes[i])
				if w+rw > budget {
					break
				}
				w += rw
				cutIdx = i
			}
			name = "…" + string(runes[cutIdx:])
		}
		left = name + sep + statsStr
	}

	return m.joinStatusSections(left, right, sep)
}

// hunkSegment returns a formatted hunk position string for the status line.
// returns "hunk X/Y" when cursor is on a changed line, "N hunks"/"1 hunk" otherwise, or empty if not in diff pane.
func (m Model) hunkSegment() string {
	if m.focus != paneDiff {
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
	if m.focus != paneDiff {
		return ""
	}
	if m.diffCursor < 0 || m.diffCursor >= len(m.diffLines) {
		return ""
	}
	dl := m.diffLines[m.diffCursor]
	if dl.ChangeType == diff.ChangeDivider {
		return ""
	}
	lineNum := m.diffLineNum(dl)
	if lineNum == 0 {
		return ""
	}
	var maxOld, maxNew int
	for _, l := range m.diffLines {
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
	padding := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2 // 2 for status bar padding
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
	return "/" + m.searchInput.Value()
}

// searchSegment returns a formatted search position string like "X/Y" for the status line.
// returns empty string when no search matches exist. shows 0/N when all matches are hidden
// in collapsed mode (e.g. matches only on removed lines).
func (m Model) searchSegment() string {
	if len(m.searchMatches) == 0 {
		return ""
	}
	pos := m.searchCursor + 1
	if m.collapsed.enabled && m.searchCursor < len(m.searchMatches) {
		hunks := m.findHunks()
		if m.isCollapsedHidden(m.searchMatches[m.searchCursor], hunks) {
			pos = 0
		}
	}
	return fmt.Sprintf("%d/%d", pos, len(m.searchMatches))
}

// padContentBg pads every line in content to targetWidth using raw ANSI background.
// strips trailing plain spaces first (left by viewport/lipgloss padding after \033[0m reset),
// then re-pads with bg-colored spaces. this ensures the background fills the entire pane
// interior, working around lipgloss full-reset that kills outer pane backgrounds.
// no-op when bgHex is empty.
func (m Model) padContentBg(content string, targetWidth int, bgHex string) string {
	if bgHex == "" || targetWidth <= 0 {
		return content
	}
	bg := m.ansiBg(bgHex)
	if bg == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// strip trailing plain spaces left by viewport/lipgloss padding after ANSI reset
		trimmed := strings.TrimRight(line, " ")
		w := lipgloss.Width(trimmed)
		pad := targetWidth - w
		if pad > 0 {
			lines[i] = trimmed + bg + strings.Repeat(" ", pad) + "\033[49m"
		} else {
			lines[i] = trimmed
		}
	}
	return strings.Join(lines, "\n")
}

// ansiColor returns an ANSI 24-bit color escape sequence for a hex color.
// code 38 = foreground, 48 = background. uses raw ANSI instead of lipgloss.Render
// to avoid full reset that breaks outer backgrounds.
func (m Model) ansiColor(hex string, code int) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return ""
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return fmt.Sprintf("\033[%d;2;%d;%d;%dm", code, r, g, b)
}

// ansiFg returns an ANSI 24-bit foreground escape sequence for a hex color.
func (m Model) ansiFg(hex string) string { return m.ansiColor(hex, 38) }

// ansiBg returns an ANSI 24-bit background escape sequence for a hex color.
func (m Model) ansiBg(hex string) string { return m.ansiColor(hex, 48) }

// statusModeIcons returns combined mode indicator icons (▼ collapsed, ◉ filter, ↩ wrap, ≋ search).
// all icons are always shown; active modes use status foreground, inactive use muted color.
func (m Model) statusModeIcons() string {
	type indicator struct {
		icon   string
		active bool
	}
	indicators := []indicator{
		{"▼", m.collapsed.enabled},
		{"◉", m.tree.filter},
		{"↩", m.wrapMode},
		{"≋", len(m.searchMatches) > 0},
		{"⊟", m.treeHidden},
		{"#", m.lineNumbers},
		{"b", m.showBlame},
		{"✓", m.tree.reviewedCount() > 0},
		{"?", m.showUntracked},
	}

	statusFg := m.styles.colors.Muted
	if m.styles.colors.StatusFg != "" {
		statusFg = m.styles.colors.StatusFg
	}
	mutedSeq := m.ansiFg(m.styles.colors.Muted)
	activeSeq := m.ansiFg(statusFg)

	var icons []string
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
	if m.currFile != "" {
		segments = append(segments, m.currFile, m.fileStatsText())
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
	if m.currFile != "" {
		segments = append(segments, m.currFile, m.fileStatsText())
	}
	return segments
}
