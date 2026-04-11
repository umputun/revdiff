package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

//go:generate moq -out mocks/style_resolver.go -pkg mocks -skip-ensure -fmt goimports . styleResolver
//go:generate moq -out mocks/style_renderer.go -pkg mocks -skip-ensure -fmt goimports . styleRenderer
//go:generate moq -out mocks/sgr_processor.go -pkg mocks -skip-ensure -fmt goimports . sgrProcessor

// styleResolver is what Model needs for static and runtime style/color lookups.
// Implemented by style.Resolver.
type styleResolver interface {
	Color(k style.ColorKey) style.Color
	Style(k style.StyleKey) lipgloss.Style
	LineBg(change diff.ChangeType) style.Color
	LineStyle(change diff.ChangeType, highlighted bool) lipgloss.Style
	WordDiffBg(change diff.ChangeType) style.Color
	IndicatorBg(change diff.ChangeType) style.Color
}

// styleRenderer is what Model needs for compound ANSI rendering operations.
// Implemented by style.Renderer.
type styleRenderer interface {
	AnnotationInline(text string) string
	DiffCursor(noColors bool) string
	StatusBarSeparator() string
	FileStatusMark(status diff.FileStatus) string
	FileReviewedMark() string
	FileAnnotationMark() string
}

// sgrProcessor is what Model needs for ANSI SGR stream processing.
// Implemented by style.SGR.
type sgrProcessor interface {
	Reemit(lines []string) []string
}

// compile-time assertions — enforce that the concrete style package types
// satisfy the consumer-side interfaces. These are the entire point of the
// interfaces: a contract boundary, caught at compile time.
var (
	_ styleResolver = (*style.Resolver)(nil)
	_ styleRenderer = (*style.Renderer)(nil)
	_ sgrProcessor  = (*style.SGR)(nil)
)
