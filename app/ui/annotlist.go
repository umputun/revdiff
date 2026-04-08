package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/keymap"
)

// buildAnnotListItems builds a flat list of all annotations across all files.
// items are ordered by file name then line number, as returned by the store.
func (m *Model) buildAnnotListItems() []annotation.Annotation {
	files := m.store.Files()
	items := make([]annotation.Annotation, 0, m.store.Count())
	for _, f := range files {
		items = append(items, m.store.Get(f)...)
	}
	return items
}

// annotListOverlay renders the annotation list popup as a bordered box.
func (m Model) annotListOverlay() string {
	popupWidth := max(min(m.width-10, 70), 20)

	if len(m.annotListItems) == 0 {
		return m.annotListEmptyOverlay(popupWidth)
	}

	// calculate visible height for items (excluding border and title padding)
	maxVisibleItems := m.annotListMaxVisible()

	// content width inside the box (minus padding)
	contentWidth := popupWidth - 4 // 2 for border + 2 for padding

	var lines []string
	for i := m.annotListOffset; i < len(m.annotListItems) && i < m.annotListOffset+maxVisibleItems; i++ {
		a := m.annotListItems[i]
		line := m.formatAnnotListItem(a, contentWidth, i == m.annotListCursor)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	title := fmt.Sprintf(" annotations (%d) ", len(m.annotListItems))

	border := lipgloss.NormalBorder()
	boxStyle := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(m.styles.colors.Accent)).
		Padding(1, 1).
		Width(popupWidth)

	box := boxStyle.Render(content)

	// inject title into top border
	box = m.injectBorderTitle(box, title, popupWidth)

	return box
}

// annotListEmptyOverlay renders the empty-state annotation list popup.
func (m Model) annotListEmptyOverlay(popupWidth int) string {
	border := lipgloss.NormalBorder()
	boxStyle := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(m.styles.colors.Accent)).
		Padding(1, 1).
		Width(popupWidth)

	// center "no annotations" text
	text := "no annotations"
	innerWidth := popupWidth - 4 // border + padding
	pad := max((innerWidth-len(text))/2, 0)
	centered := strings.Repeat(" ", pad) + text

	box := boxStyle.Render(centered)
	title := " annotations (0) "
	box = m.injectBorderTitle(box, title, popupWidth)
	return box
}

// formatAnnotListItem formats a single annotation list item for display.
func (m Model) formatAnnotListItem(a annotation.Annotation, width int, selected bool) string {
	// build prefix: "filename:line (type)" or "filename (file-level)"
	var prefix string
	if a.Line == 0 {
		prefix = filepath.Base(a.File) + " (file-level)"
	} else {
		prefix = fmt.Sprintf("%s:%d (%s)", filepath.Base(a.File), a.Line, a.Type)
	}

	// build the full line: "prefix  comment"
	prefixWidth := lipgloss.Width(prefix)
	commentSpace := width - prefixWidth - 4 // 2 for cursor prefix, 2 for gap between prefix and comment

	var comment string
	if commentSpace > 3 && a.Comment != "" {
		comment = a.Comment
		if lipgloss.Width(comment) > commentSpace {
			comment = ansi.Truncate(comment, commentSpace-3, "...")
		}
	}

	var line string
	if comment != "" {
		line = prefix + "  " + comment
	} else {
		line = prefix
	}

	// apply styling
	if selected {
		cursor := "> "
		styled := m.styles.FileSelected.Render(cursor + line)
		// pad to full width
		w := lipgloss.Width(styled)
		if w < width {
			styled += m.styles.FileSelected.Render(strings.Repeat(" ", width-w))
		}
		return styled
	}

	// style the prefix with change type color
	var styledPrefix string
	switch a.Type {
	case "+":
		styledPrefix = m.ansiFg(m.styles.colors.AddFg) + prefix + "\033[39m"
	case "-":
		styledPrefix = m.ansiFg(m.styles.colors.RemoveFg) + prefix + "\033[39m"
	default:
		styledPrefix = m.ansiFg(m.styles.colors.Muted) + prefix + "\033[39m"
	}

	if comment != "" {
		return "  " + styledPrefix + "  " + comment
	}
	return "  " + styledPrefix
}

// injectBorderTitle replaces part of the top border line with a centered title.
func (m Model) injectBorderTitle(box, title string, popupWidth int) string {
	boxLines := strings.Split(box, "\n")
	if len(boxLines) == 0 {
		return box
	}

	// build new top border with title centered
	topLine := boxLines[0]
	topWidth := lipgloss.Width(topLine)
	titleWidth := lipgloss.Width(title)

	if titleWidth >= topWidth-4 {
		return box // title too wide, skip injection
	}

	titleStart := max((topWidth-titleWidth)/2, 2)

	borderColor := m.styles.colors.Accent
	border := lipgloss.NormalBorder()

	// rebuild top border: corner + left segment + title + right segment + corner
	leftLen := titleStart - 1
	rightLen := max(popupWidth-titleStart-titleWidth+1, 0)

	// use raw ANSI sequences instead of lipgloss.Render() to avoid
	// \033[0m full reset which would break the popup's background color
	bgSeq := ""
	bgReset := ""
	if m.styles.colors.DiffBg != "" {
		bgSeq = m.ansiBg(m.styles.colors.DiffBg)
		bgReset = "\033[49m"
	}
	newTop := bgSeq + m.ansiFg(borderColor) +
		border.TopLeft +
		strings.Repeat(border.Top, leftLen) +
		title +
		strings.Repeat(border.Top, rightLen) +
		border.TopRight +
		"\033[39m" + bgReset

	boxLines[0] = newTop
	return strings.Join(boxLines, "\n")
}

// handleAnnotListKey handles keys when the annotation list popup is visible.
// j/k/arrows navigate, Enter jumps to annotation, Esc/annot_list key closes, all other keys consumed.
func (m Model) handleAnnotListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// resolve action to allow closing popup with the same key that opens it (remappable)
	action := m.keymap.Resolve(msg.String())
	if action == keymap.ActionAnnotList {
		m.showAnnotList = false
		return m, nil
	}

	// handle modal keys first so they always work regardless of keymap remapping
	switch {
	case msg.Type == tea.KeyEnter:
		if len(m.annotListItems) == 0 {
			m.showAnnotList = false
			return m, nil
		}
		return m.jumpToAnnotation()

	case action == keymap.ActionDismiss || msg.Type == tea.KeyEsc:
		m.showAnnotList = false
		return m, nil
	}

	switch action { //nolint:exhaustive // only navigation actions are relevant in annotation list overlay
	case keymap.ActionUp:
		if m.annotListCursor > 0 {
			m.annotListCursor--
			// scroll up if cursor is above visible area
			if m.annotListCursor < m.annotListOffset {
				m.annotListOffset = m.annotListCursor
			}
		}
		return m, nil

	case keymap.ActionDown:
		if m.annotListCursor < len(m.annotListItems)-1 {
			m.annotListCursor++
			// scroll down if cursor is below visible area
			maxVisible := m.annotListMaxVisible()
			if m.annotListCursor >= m.annotListOffset+maxVisible {
				m.annotListOffset = m.annotListCursor - maxVisible + 1
			}
		}
		return m, nil
	}

	// consume all other keys while popup is open
	return m, nil
}

// annotListMaxVisible returns the maximum number of visible items in the annotation list popup.
func (m Model) annotListMaxVisible() int {
	return max(min(len(m.annotListItems), m.height-6), 1)
}

// jumpToAnnotation handles Enter in the annotation list popup.
// jumps to the selected annotation, loading a different file if needed.
func (m Model) jumpToAnnotation() (tea.Model, tea.Cmd) {
	a := m.annotListItems[m.annotListCursor]
	m.showAnnotList = false

	if a.File == m.currFile {
		// same file: position cursor directly
		m.positionOnAnnotation(a)
		return m, nil
	}

	// different file: set pending jump and trigger file load
	m.pendingAnnotJump = &a
	if !m.singleFile {
		if !m.tree.selectByPath(a.File) {
			m.pendingAnnotJump = nil // file not in tree, cancel jump
			return m, nil
		}
	}
	return m.loadSelectedIfChanged()
}

// positionOnAnnotation moves the cursor to the given annotation's line, re-renders, and centers the viewport.
// in collapsed mode, expands the hunk containing the target line so removed lines are visible.
func (m *Model) positionOnAnnotation(a annotation.Annotation) {
	if a.Line == 0 {
		m.diffCursor = -1
	} else {
		idx := m.findDiffLineIndex(a.Line, a.Type)
		if idx >= 0 {
			m.diffCursor = idx
			m.ensureHunkExpanded(idx)
		}
	}
	m.focus = paneDiff
	m.syncTOCActiveSection()
	m.viewport.SetContent(m.renderDiff())
	m.centerViewportOnCursor()
}

// ensureHunkExpanded expands the hunk containing diffLines[idx] when collapsed mode is active.
// this ensures the target line is visible after a jump (e.g., annotation on a removed line).
// also expands delete-only placeholder hunks where the first line is "visible" as a synthetic
// placeholder but annotations are not rendered (renderCollapsedDiff skips them).
func (m *Model) ensureHunkExpanded(idx int) {
	if !m.collapsed.enabled {
		return
	}
	hunks := m.findHunks()
	if m.isCollapsedHidden(idx, hunks) || m.isDeleteOnlyPlaceholder(idx, hunks) {
		hunkStart := m.hunkStartFor(idx, hunks)
		if hunkStart >= 0 {
			m.collapsed.expandedHunks[hunkStart] = true
		}
	}
}

// findDiffLineIndex finds the index into diffLines matching the given line number and change type.
// uses diffLineNum() semantics: compares against OldNum for removes, NewNum for adds/context.
// returns -1 if not found.
func (m Model) findDiffLineIndex(line int, changeType string) int {
	for i, dl := range m.diffLines {
		if string(dl.ChangeType) != changeType {
			continue
		}
		if m.diffLineNum(dl) == line {
			return i
		}
	}
	return -1
}
