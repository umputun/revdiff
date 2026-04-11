package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
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
	diffHeader := m.resolver.Style(style.StyleKeyDirEntry).Render(" " + diffTitle)
	diffContent := lipgloss.JoinVertical(lipgloss.Left, diffHeader, m.viewport.View())

	var mainView string
	switch {
	case m.treePaneHidden():
		// tree pane hidden (user toggle or single-file without TOC): diff uses full width
		paneW := m.width - 2
		diffContent = m.padContentBg(diffContent, paneW, m.resolver.Color(style.ColorKeyDiffPaneBg))
		diffPane := m.resolver.Style(style.StyleKeyDiffPaneActive).
			Width(paneW).
			Height(ph).
			Render(diffContent)
		mainView = diffPane

	case m.singleFile && m.mdTOC != nil:
		// single-file markdown with TOC: two-pane layout with TOC in left pane
		tocContent := m.mdTOC.Render(sidepane.TOCRender{Width: m.treeWidth, Height: ph, Focused: m.focus == paneTree, Resolver: m.resolver})
		mainView = m.renderTwoPaneLayout(tocContent, diffContent, ph)

	default:
		annotated := m.annotatedFiles()
		treeContent := m.tree.Render(sidepane.FileTreeRender{Width: m.treeWidth, Height: ph, Annotated: annotated, Resolver: m.resolver, Renderer: m.renderer})
		mainView = m.renderTwoPaneLayout(treeContent, diffContent, ph)
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

	status := m.resolver.Style(style.StyleKeyStatusBar).Width(m.width).Render(m.statusBarText())
	return lipgloss.JoinVertical(lipgloss.Left, mainView, status)
}

// renderTwoPaneLayout renders a two-pane layout with left (tree/TOC) and right (diff) content.
// applies focus-based pane styles, background padding, and joins horizontally.
func (m Model) renderTwoPaneLayout(leftContent, diffContent string, ph int) string {
	treeStyle := m.resolver.Style(style.StyleKeyTreePane)
	diffStyle := m.resolver.Style(style.StyleKeyDiffPane)
	if m.focus == paneTree {
		treeStyle = m.resolver.Style(style.StyleKeyTreePaneActive)
	} else {
		diffStyle = m.resolver.Style(style.StyleKeyDiffPaneActive)
	}

	diffW := m.width - m.treeWidth - 4
	leftContent = m.padContentBg(leftContent, m.treeWidth, m.resolver.Color(style.ColorKeyTreePaneBg))
	diffContent = m.padContentBg(diffContent, diffW, m.resolver.Color(style.ColorKeyDiffPaneBg))

	leftPane := treeStyle.
		Width(m.treeWidth).
		Height(ph).
		Render(leftContent)

	diffPane := diffStyle.
		Width(diffW).
		Height(ph).
		Render(diffContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, diffPane)
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
		{"▼", m.collapsed.enabled},
		{"◉", m.tree.FilterActive()},
		{"↩", m.wrapMode},
		{"≋", len(m.searchMatches) > 0},
		{"⊟", m.treeHidden},
		{"#", m.lineNumbers},
		{"b", m.showBlame},
		{"±", m.wordDiff},
		{"✓", m.tree.ReviewedCount() > 0},
		{"∅", m.showUntracked},
	}

	mutedSeq := string(m.resolver.Color(style.ColorKeyMutedFg))
	activeSeq := string(m.resolver.Color(style.ColorKeyStatusFg))

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
