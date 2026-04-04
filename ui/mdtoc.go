package ui

import (
	"path/filepath"
	"strings"

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
