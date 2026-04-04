package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/diff"
)

// tocEntry represents a single markdown header in the table of contents.
type tocEntry struct {
	title   string // header text without # prefix
	level   int    // header level 1-6
	lineIdx int    // index into diffLines
}

// mdTOC manages the markdown table-of-contents navigation pane.
type mdTOC struct {
	entries       []tocEntry
	cursor        int // currently highlighted entry index
	offset        int // first visible entry index for viewport scrolling
	activeSection int // index of entry matching current diff cursor position (-1 if none)
}

// parseTOC scans diff lines for markdown headers and builds a TOC.
// headers inside fenced code blocks (```) are excluded.
func parseTOC(lines []diff.DiffLine) *mdTOC {
	var entries []tocEntry
	inCodeBlock := false

	for i, line := range lines {
		if line.ChangeType == diff.ChangeDivider {
			continue
		}

		content := line.Content

		// track fenced code block state
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		// check for markdown header: ^#{1,6} (space required after last #)
		if !strings.HasPrefix(content, "#") {
			continue
		}

		level := 0
		for _, ch := range content {
			if ch != '#' {
				break
			}
			level++
		}
		if level < 1 || level > 6 {
			continue
		}
		if len(content) <= level || content[level] != ' ' {
			continue
		}

		title := strings.TrimSpace(content[level+1:])
		if title == "" {
			continue
		}

		entries = append(entries, tocEntry{title: title, level: level, lineIdx: i})
	}

	if len(entries) == 0 {
		return nil
	}

	return &mdTOC{entries: entries, activeSection: -1}
}

// moveUp moves cursor to the previous entry, clamped to first entry.
func (toc *mdTOC) moveUp() {
	if toc.cursor > 0 {
		toc.cursor--
	}
}

// moveDown moves cursor to the next entry, clamped to last entry.
func (toc *mdTOC) moveDown() {
	if toc.cursor < len(toc.entries)-1 {
		toc.cursor++
	}
}

// ensureVisible adjusts offset so the cursor is within the visible range of given height.
func (toc *mdTOC) ensureVisible(height int) {
	if height <= 0 {
		return
	}
	if toc.cursor < toc.offset {
		toc.offset = toc.cursor
	} else if toc.cursor >= toc.offset+height {
		toc.offset = toc.cursor - height + 1
	}
	if toc.offset < 0 {
		toc.offset = 0
	}
	if maxOff := max(len(toc.entries)-height, 0); toc.offset > maxOff {
		toc.offset = maxOff
	}
}

// updateActiveSection finds the nearest entry with lineIdx <= diffCursor and sets activeSection.
func (toc *mdTOC) updateActiveSection(diffCursor int) {
	toc.activeSection = -1
	for i, e := range toc.entries {
		if e.lineIdx > diffCursor {
			break
		}
		toc.activeSection = i
	}
}

// render produces the TOC display string with indentation by level, cursor highlight, and active section marker.
// when focusedPane is paneTree, the cursor entry gets FileSelected style.
// when focusedPane is paneDiff, the active section entry gets a marker prefix.
func (toc *mdTOC) render(width, height int, focusedPane pane, s styles) string {
	if len(toc.entries) == 0 {
		return "  no headers"
	}

	toc.ensureVisible(height)
	end := min(toc.offset+height, len(toc.entries))

	var b strings.Builder
	for idx := toc.offset; idx < end; idx++ {
		e := toc.entries[idx]
		indent := strings.Repeat("  ", e.level-1) // h1=0 indent, h2=2, h3=4, etc.
		prefix := "  "
		if idx == toc.activeSection && focusedPane == paneDiff {
			prefix = "▸ "
		}

		title := toc.truncateTitle(e.title, width-len(indent)-len(prefix)-1)
		line := fmt.Sprintf("%s%s%s", prefix, indent, title)

		if focusedPane == paneTree && idx == toc.cursor {
			line = s.FileSelected.Width(max(width-2, 1)).Render(line)
		} else if idx == toc.activeSection && focusedPane == paneDiff {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		b.WriteString(line)
		if idx < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// truncateTitle trims a title to fit maxWidth, appending ellipsis when truncated.
func (toc *mdTOC) truncateTitle(title string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	runes := []rune(title)
	if len(runes) <= maxWidth {
		return title
	}
	if maxWidth <= 1 {
		return "…"
	}
	return string(runes[:maxWidth-1]) + "…"
}

// isFullContext returns true when all lines are ChangeContext (skips ChangeDivider).
func (m *Model) isFullContext(lines []diff.DiffLine) bool {
	hasContext := false
	for _, line := range lines {
		if line.ChangeType == diff.ChangeDivider {
			continue
		}
		if line.ChangeType != diff.ChangeContext {
			return false
		}
		hasContext = true
	}
	return hasContext
}

// isMarkdownFile checks if the filename has a markdown extension (.md or .markdown).
func (m *Model) isMarkdownFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown"
}
