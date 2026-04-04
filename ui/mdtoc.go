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
