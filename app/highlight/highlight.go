package highlight

import (
	"fmt"
	"log"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/umputun/revdiff/app/diff"
)

// chromaFallbackStyle is the name of the Chroma style that doubles as styles.Fallback.
// styles.Get returns Fallback for unknown names, but "swapoff" is a real built-in style
// whose registry entry IS the Fallback sentinel, so we must special-case it.
const chromaFallbackStyle = "swapoff"

// Highlighter applies syntax highlighting to source code lines using Chroma.
type Highlighter struct {
	styleName string
	enabled   bool
}

// New creates a Highlighter with the given Chroma style name and enabled state.
// if styleName is empty, defaults to "monokai". Logs a warning if the style name is unknown.
func New(styleName string, enabled bool) *Highlighter {
	if styleName == "" {
		styleName = "monokai"
	}
	if styles.Get(styleName) == styles.Fallback && styleName != chromaFallbackStyle {
		log.Printf("[WARN] unknown chroma style %q, using monokai", styleName)
		styleName = "monokai"
	}
	return &Highlighter{styleName: styleName, enabled: enabled}
}

// SetStyle changes the Chroma style used for subsequent HighlightLines calls.
// Returns false if the style name is unknown.
func (h *Highlighter) SetStyle(styleName string) bool {
	if styles.Get(styleName) == styles.Fallback && styleName != chromaFallbackStyle {
		return false
	}
	h.styleName = styleName
	return true
}

// StyleName returns the current Chroma style name.
func (h *Highlighter) StyleName() string {
	return h.styleName
}

// IsValidStyle reports whether styleName is a known Chroma style.
func IsValidStyle(styleName string) bool {
	return styles.Get(styleName) != styles.Fallback || styleName == chromaFallbackStyle
}

// HighlightLines takes a filename (for lexer detection) and a slice of diff.DiffLine,
// reconstructs the file content, tokenizes it with Chroma, and returns a parallel []string
// where each entry contains the ANSI-formatted (foreground-only) version of that line's content.
// returns nil if highlighting is disabled or no lexer matches the filename.
func (h *Highlighter) HighlightLines(filename string, lines []diff.DiffLine) []string {
	if !h.enabled || len(lines) == 0 {
		return nil
	}

	lexer := lexers.Match(filename)
	if lexer == nil {
		return nil
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get(h.styleName)

	newContent, oldContent := reconstructFiles(lines)

	newHL := highlightFile(lexer, style, newContent)
	oldHL := highlightFile(lexer, style, oldContent)

	return mapHighlightedLines(lines, newHL, oldHL)
}

// reconstructFiles builds old and new file content from diff lines.
// new file = context + added lines, old file = context + removed lines.
func reconstructFiles(lines []diff.DiffLine) (newFile, oldFile string) {
	var newB, oldB strings.Builder
	for _, dl := range lines {
		switch dl.ChangeType {
		case diff.ChangeAdd:
			newB.WriteString(dl.Content)
			newB.WriteByte('\n')
		case diff.ChangeRemove:
			oldB.WriteString(dl.Content)
			oldB.WriteByte('\n')
		case diff.ChangeDivider:
			// skip dividers
		default: // context
			newB.WriteString(dl.Content)
			newB.WriteByte('\n')
			oldB.WriteString(dl.Content)
			oldB.WriteByte('\n')
		}
	}
	return newB.String(), oldB.String()
}

// highlightFile tokenizes source code and returns per-line ANSI strings with foreground-only colors.
func highlightFile(lexer chroma.Lexer, style *chroma.Style, source string) []string {
	if source == "" {
		return nil
	}

	iter, err := lexer.Tokenise(nil, source)
	if err != nil {
		log.Printf("[WARN] syntax highlighting tokenization failed: %v", err)
		return nil
	}

	var lines []string
	var cur strings.Builder

	for _, tok := range iter.Tokens() {
		// tokens may contain embedded newlines, split them
		parts := strings.Split(tok.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				// newline boundary: flush current line
				lines = append(lines, cur.String())
				cur.Reset()
			}
			if part == "" {
				continue
			}
			writeTokenANSI(&cur, tok.Type, part, style)
		}
	}
	// flush last line
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}

	return lines
}

// writeTokenANSI writes a token value with foreground-only ANSI escape codes.
// uses specific attribute resets instead of full reset (\033[0m) to preserve
// any outer background color set by lipgloss.
func writeTokenANSI(b *strings.Builder, tokenType chroma.TokenType, value string, style *chroma.Style) {
	entry := style.Get(tokenType)
	hasFg, hasBold, hasItalic := false, false, false

	if entry.Colour.IsSet() { //nolint:misspell // chroma API uses British spelling
		r, g, bb := entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue() //nolint:misspell // chroma API
		fmt.Fprintf(b, "\033[38;2;%d;%d;%dm", r, g, bb)
		hasFg = true
	}
	if entry.Bold == chroma.Yes {
		b.WriteString("\033[1m")
		hasBold = true
	}
	if entry.Italic == chroma.Yes {
		b.WriteString("\033[3m")
		hasItalic = true
	}

	b.WriteString(value)

	// reset only the attributes we set, preserving outer background
	if hasFg {
		b.WriteString("\033[39m") // reset foreground to default
	}
	if hasBold {
		b.WriteString("\033[22m") // reset bold
	}
	if hasItalic {
		b.WriteString("\033[23m") // reset italic
	}
}

// mapHighlightedLines maps highlighted old/new file lines back to the original diff line order.
func mapHighlightedLines(lines []diff.DiffLine, newHL, oldHL []string) []string {
	result := make([]string, len(lines))
	var newIdx, oldIdx int

	for i, dl := range lines {
		switch dl.ChangeType {
		case diff.ChangeAdd:
			if newIdx < len(newHL) {
				result[i] = newHL[newIdx]
			}
			newIdx++
		case diff.ChangeRemove:
			if oldIdx < len(oldHL) {
				result[i] = oldHL[oldIdx]
			}
			oldIdx++
		case diff.ChangeDivider:
			// no highlighted content for dividers
		default: // context
			if newIdx < len(newHL) {
				result[i] = newHL[newIdx]
			}
			newIdx++
			oldIdx++
		}
	}
	return result
}
