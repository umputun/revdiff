package style

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// SanitizeFilenameForDisplay strips characters that would break or spoof
// header and status-bar layout: C0/C1 controls, DEL, invalid UTF-8 replacement
// runes, and Unicode format/bidi controls. POSIX permits control bytes in file
// names, so every path-rendering surface must apply this before measuring or
// rendering a filename to preserve the single-row header invariant.
func SanitizeFilenameForDisplay(s string) string {
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

// TruncateLeftToWidth left-truncates s with a leading "…" so it fits in
// budget visual columns, preserving the meaningful end.
func TruncateLeftToWidth(s string, budget int) string {
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
	width, cutIdx := 0, len(runes)
	for i, r := range slices.Backward(runes) {
		runeWidth := runewidth.RuneWidth(r)
		if width+runeWidth > tailBudget {
			break
		}
		width += runeWidth
		cutIdx = i
	}
	return "…" + string(runes[cutIdx:])
}
