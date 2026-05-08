package main

import (
	"fmt"
	"os"

	"github.com/umputun/revdiff/app/markdown"
	"github.com/umputun/revdiff/app/ui"
	"github.com/umputun/revdiff/app/ui/style"
)

// buildMarkdownRenderer constructs the initial AnnotationMarkdown passed to
// ModelConfig at startup. Returns nil — which the UI interprets as "fall back
// to the legacy italic plain-text wrap path" — when the user disables glamour
// rendering via --plain-annotations / --no-colors. A failed glamour init
// logs once to stderr and degrades silently rather than killing startup; the
// legacy path is always available.
func buildMarkdownRenderer(opts options) ui.AnnotationMarkdown {
	if opts.NoColors || opts.PlainAnnotations {
		return nil
	}
	return makeMarkdownRenderer(optsToStyleColors(opts), opts.ChromaStyle)
}

// markdownRebuilder returns the closure that ModelConfig.AnnotationMarkdownBuilder
// expects: a function the UI calls on theme apply with the new resolved
// colors and chroma style, returning a freshly-styled renderer. The closure
// honors the same opt-out flags as buildMarkdownRenderer (so a user with
// --plain-annotations stays on the legacy path through theme changes too).
//
// This is the runtime mirror of buildMarkdownRenderer: both produce a
// markdown.Renderer from the same color sources. The split exists because
// startup colors live on opts (parsed flags) while runtime colors live on
// the resolved spec.Colors / spec.ChromaStyle handed to applyTheme — same
// shape, different lifecycle.
func markdownRebuilder(opts options) func(style.Colors, string) ui.AnnotationMarkdown {
	if opts.NoColors || opts.PlainAnnotations {
		return nil
	}
	return func(colors style.Colors, chromaStyle string) ui.AnnotationMarkdown {
		return makeMarkdownRenderer(colors, chromaStyle)
	}
}

// makeMarkdownRenderer is the shared body of buildMarkdownRenderer and
// markdownRebuilder: derive the Style and construct a markdown.Renderer.
// Returns nil on init failure with a warning to stderr; nil propagates as
// "use legacy path" through the UI.
func makeMarkdownRenderer(colors style.Colors, chromaStyle string) ui.AnnotationMarkdown {
	r, err := markdown.New(
		markdown.WithStyle(deriveMarkdownStyle(colors, chromaStyle)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] markdown renderer init failed: %v — falling back to plain annotations\n", err)
		return nil
	}
	return r
}

// deriveMarkdownStyle maps revdiff's existing theme color slots to the
// markdown.Style fields. v0 reuses the Annotation slot for body / heading /
// emphasis / strong / blockquote so markdown blends with the existing
// italic-prose look; Accent drives inline code and links (it's the active-
// border color, guaranteed by every theme to be foreground-readable against
// the diff pane bg, and naturally contrasts the body color); the chroma
// style for fenced code blocks tracks --chroma-style so fenced code matches
// the diff pane's syntax highlighting.
//
// Why NOT SearchFg: every bundled dark theme deliberately sets SearchFg
// equal to DiffBg (search highlights work via the bright SearchBg with text
// punched out); using SearchFg as a foreground here would render inline
// code in the same color as the pane background — invisible. Accent is the
// closest existing slot designed to read as foreground.
//
// Adding dedicated markdown_* theme keys is a v1 concern.
func deriveMarkdownStyle(c style.Colors, chromaStyle string) markdown.Style {
	return markdown.Style{
		BodyFg:          c.Annotation,
		HeadingFg:       c.Annotation,
		EmphFg:          c.Annotation,
		StrongFg:        c.Annotation,
		InlineCodeFg:    c.Accent,
		LinkFg:          c.Accent,
		BlockquoteFg:    c.Annotation,
		HRFg:            c.Border,
		ListItemFg:      c.Annotation,
		ChromaStyleName: chromaStyle,
		Emoji:           true,
	}
}
