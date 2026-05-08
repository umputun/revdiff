// Package markdown renders annotation prose as glow-style ANSI rows. It wraps
// charmbracelet/glamour with a foreground-only style so the output composes
// with revdiff's pane backgrounds (the same constraint app/highlight already
// honors for chroma — see CLAUDE.md "ANSI nesting with lipgloss" / "Background
// fill for themed panes").
package markdown

import (
	"github.com/charmbracelet/glamour/ansi"
)

// Style is the consumer-facing color configuration for markdown rendering.
// Every field is a hex color string ("#rrggbb"); empty strings fall back to
// terminal default. The struct deliberately avoids any background-color
// fields: revdiff's diff pane is foreground-only at the cell level (the pane
// background is supplied by extendLineBg as right-padding) so a markdown
// renderer that emitted backgrounds inside row content would conflict with
// the lipgloss-bg invariants documented in CLAUDE.md.
type Style struct {
	BodyFg       string // text body, paragraphs
	HeadingFg    string // h1-h6 (bold attribute always applied on top)
	EmphFg       string // *italic* (italic attribute always applied)
	StrongFg     string // **bold** (bold attribute always applied)
	InlineCodeFg string // `code` spans
	LinkFg       string // [text](url)
	BlockquoteFg string // > quoted text
	HRFg         string // ---
	ListItemFg   string // bullet / number prefix

	// ChromaStyleName names the chroma style used inside fenced code blocks
	// (e.g. "monokai", "dracula", "swapoff"). Empty string disables chroma
	// styling — code blocks render as plain text using BodyFg.
	ChromaStyleName string

	// Emoji enables :rocket: → 🚀 substitution.
	Emoji bool
}

// toGlamourConfig converts a Style into a glamour ansi.StyleConfig optimized
// for inline rendering inside revdiff's diff pane: zero block margins (so
// annotation rows stay flush with the diff line they hang off), no background
// colors anywhere, and no block prefixes/suffixes that would inject blank
// lines or decoration glyphs around content.
func (s Style) toGlamourConfig() ansi.StyleConfig {
	zero := uintPtr(0)
	bold := boolPtr(true)
	italic := boolPtr(true)
	underline := boolPtr(true)

	cfg := ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: optPtr(s.BodyFg),
			},
			Margin: zero,
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: optPtr(s.BodyFg)},
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  optPtr(s.BlockquoteFg),
				Italic: italic,
			},
			// glamour applies IndentToken to every wrapped line of the block,
			// which is what users expect from a "leading bar" — Prefix would
			// only fire once at the top of the block. Indent is the number
			// of times glamour REPEATS the token (per ansi/margin.go), not
			// a cell count — Indent: 1 emits "│ " once per line; Indent: 2
			// would emit "│ │ ", giving the doubled-bar look glamour's own
			// dark style avoids by also using Indent: 1.
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		List: ansi.StyleList{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: optPtr(s.BodyFg)},
			},
			LevelIndent: 2,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: optPtr(s.HeadingFg),
				Bold:  bold,
			},
			Margin: zero,
		},
		H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
		H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
		H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
		H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
		H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
		H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
		Strong: ansi.StylePrimitive{
			Color: optPtr(s.StrongFg),
			Bold:  bold,
		},
		Emph: ansi.StylePrimitive{
			Color:  optPtr(s.EmphFg),
			Italic: italic,
		},
		Link: ansi.StylePrimitive{
			Color:     optPtr(s.LinkFg),
			Underline: underline,
		},
		LinkText: ansi.StylePrimitive{
			Color: optPtr(s.LinkFg),
			Bold:  bold,
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  optPtr(s.InlineCodeFg),
				Prefix: "`",
				Suffix: "`",
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: optPtr(s.BodyFg)},
				Margin:         zero,
			},
			Theme: s.ChromaStyleName,
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
			Color:       optPtr(s.ListItemFg),
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
			Color:       optPtr(s.ListItemFg),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  optPtr(s.HRFg),
			Format: "\n──────\n",
		},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: boolPtr(true),
		},
	}
	return cfg
}

// optPtr returns a *string for non-empty s, nil otherwise. Glamour's StyleConfig
// uses nil to mean "don't set a color"; an explicit empty string would be
// passed to termenv as a literal style and produce malformed SGR.
func optPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func boolPtr(b bool) *bool       { return &b }
func uintPtr(u uint) *uint       { return &u }
func stringPtr(s string) *string { return &s }
