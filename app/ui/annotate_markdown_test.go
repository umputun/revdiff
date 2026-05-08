package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/style"
)

// modelWithMarkdown returns a testModel that delegates annotation rendering
// to the supplied AnnotationMarkdown. Pre-loads a single line annotation on
// "a.go" line 1 of changeType "+".
func modelWithMarkdown(t *testing.T, md AnnotationMarkdown, comment string) Model {
	t.Helper()
	m := testModel([]string{"a.go"}, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeAdd},
	}
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: comment})
	m.annot.markdown = md
	m.annot.rowCache = make(map[annotCacheKey][]string)
	return m
}

func TestAnnotationVisualRows_legacyFallbackWhenNilMarkdown(t *testing.T) {
	m := modelWithMarkdown(t, nil, "hello")
	rows := m.annotationVisualRows("\U0001f4ac ", "hello")
	require.NotEmpty(t, rows)
	assert.Contains(t, rows[0], "\U0001f4ac ", "legacy row 0 must include the prefix")
	assert.Contains(t, rows[0], "hello")
}

func TestAnnotationVisualRows_markdownPathUsedWhenWired(t *testing.T) {
	called := 0
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string {
			called++
			return []string{"\x1b[1mhello\x1b[22m"}
		},
	}
	m := modelWithMarkdown(t, md, "hello")
	rows := m.annotationVisualRows("\U0001f4ac ", "hello")
	require.Len(t, rows, 1)
	assert.Equal(t, 1, called, "markdown renderer must be called when wired")
	assert.Contains(t, rows[0], "\U0001f4ac ", "row 0 still gets the styled prefix")
	assert.Contains(t, rows[0], "\x1b[1mhello\x1b[22m", "glamour styling preserved verbatim")
}

func TestAnnotationVisualRows_markdownEmptyRowsFallsBackToLegacy(t *testing.T) {
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return nil },
	}
	m := modelWithMarkdown(t, md, "hello")
	rows := m.annotationVisualRows("\U0001f4ac ", "hello")
	require.NotEmpty(t, rows, "empty markdown output must trigger legacy fallback")
	assert.Contains(t, rows[0], "\U0001f4ac hello")
}

func TestAnnotationVisualRows_caches(t *testing.T) {
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{body} },
	}
	m := modelWithMarkdown(t, md, "hello")

	m.annotationVisualRows("\U0001f4ac ", "hello")
	m.annotationVisualRows("\U0001f4ac ", "hello")
	m.annotationVisualRows("\U0001f4ac ", "hello")

	assert.Equal(t, 1, len(md.RenderCalls()),
		"identical (body, prefix, width) must hit the cache after the first call")
}

func TestAnnotationVisualRows_cacheKeyDistinguishesPrefix(t *testing.T) {
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{body} },
	}
	m := modelWithMarkdown(t, md, "x")

	m.annotationVisualRows("\U0001f4ac ", "x")
	m.annotationVisualRows("\U0001f4ac file: ", "x")

	assert.Equal(t, 2, len(md.RenderCalls()),
		"different prefixes (line vs file-level) must produce distinct cache entries")
}

func TestAnnotationVisualRows_cacheKeyDistinguishesBody(t *testing.T) {
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{body} },
	}
	m := modelWithMarkdown(t, md, "first")
	m.annotationVisualRows("\U0001f4ac ", "first")
	m.annotationVisualRows("\U0001f4ac ", "second")
	assert.Equal(t, 2, len(md.RenderCalls()))
}

func TestAnnotationVisualRows_emptyBodyReturnsNil(t *testing.T) {
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{body} },
	}
	m := modelWithMarkdown(t, md, "")
	assert.Nil(t, m.annotationVisualRows("\U0001f4ac ", ""))
	assert.Equal(t, 0, len(md.RenderCalls()), "empty body must short-circuit before hitting markdown")
}

func TestAnnotationVisualRows_invalidateClearsCache(t *testing.T) {
	called := 0
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string {
			called++
			return []string{body}
		},
	}
	m := modelWithMarkdown(t, md, "hello")
	m.annotationVisualRows("\U0001f4ac ", "hello")
	require.Equal(t, 1, called)
	m.annotationVisualRows("\U0001f4ac ", "hello")
	require.Equal(t, 1, called, "second identical call must hit cache")

	m.invalidateAnnotationRows()
	m.annotationVisualRows("\U0001f4ac ", "hello")
	assert.Equal(t, 2, called, "invalidate must force re-render")
}

func TestRenderWrappedAnnotation_rowCountMatchesWrappedAnnotationLineCount(t *testing.T) {
	// the cursor-math invariant: wrappedAnnotationLineCount and
	// renderWrappedAnnotation MUST agree on the number of rows produced for
	// any (body, prefix, width). desync here breaks scroll math.
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string {
			return []string{"row a", "row b", "row c", "row d"}
		},
	}
	m := modelWithMarkdown(t, md, "anything")

	count := m.wrappedAnnotationLineCount(m.annotationKey(1, "+"))

	var b strings.Builder
	cursor := m.renderer.DiffCursor(m.cfg.noColors)
	m.renderWrappedAnnotation(&b, cursor, "\U0001f4ac ", "anything")

	emittedRows := strings.Count(b.String(), "\n")
	assert.Equal(t, count, emittedRows,
		"wrappedAnnotationLineCount(%d) must equal painted row count (%d)", count, emittedRows)
}

func TestRenderWrappedAnnotation_glamourMultilineEachRowExtended(t *testing.T) {
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string {
			return []string{"first", "second", "third"}
		},
	}
	m := modelWithMarkdown(t, md, "x")

	var b strings.Builder
	cursor := m.renderer.DiffCursor(m.cfg.noColors)
	m.renderWrappedAnnotation(&b, cursor, "\U0001f4ac ", "x")

	out := b.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 3)

	assert.Contains(t, lines[0], "\U0001f4ac ", "first row carries the prefix")
	assert.NotContains(t, lines[1], "\U0001f4ac ", "continuation rows must NOT repeat the prefix")
	assert.True(t, strings.HasPrefix(lines[1], " "), "continuation row 1 starts with a cursor-cell space")
	assert.True(t, strings.HasPrefix(lines[2], " "), "continuation row 2 starts with a cursor-cell space")
}

func TestApplyTheme_rebuildsMarkdownRenderer(t *testing.T) {
	// regression: today's legacy italic-prose path tracks AnnotationFg
	// changes via the live resolver, so theme switching takes effect
	// immediately. The markdown path bakes Style into the renderer at
	// construction time — it MUST be rebuilt on theme apply or annotations
	// will stay stuck on the old theme's colors.
	first := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{"FIRST:" + body} },
	}
	second := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{"SECOND:" + body} },
	}
	rebuildCalls := 0
	builder := func(colors style.Colors, chromaStyle string) AnnotationMarkdown {
		rebuildCalls++
		return second
	}

	m := modelWithMarkdown(t, first, "x")
	m.annot.markdownBuilder = builder

	rows := m.annotationVisualRows("\U0001f4ac ", "x")
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "FIRST:x", "before theme apply, first renderer is used")

	m.applyTheme(ThemeSpec{
		Colors:      style.Colors{Annotation: "#abcdef"},
		ChromaStyle: "swapoff",
	})
	require.Equal(t, 1, rebuildCalls, "applyTheme must call the markdown builder once")

	rows = m.annotationVisualRows("\U0001f4ac ", "x")
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "SECOND:x", "after theme apply, rebuilt renderer is used")
}

func TestApplyTheme_noRebuilderLeavesRendererUnchanged(t *testing.T) {
	// when no builder is wired (--plain-annotations or tests), applyTheme
	// must NOT touch annot.markdown — leaving a stale renderer is better
	// than nilling out a working one.
	first := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string { return []string{"x"} },
	}
	m := modelWithMarkdown(t, first, "x")
	m.annot.markdownBuilder = nil

	m.applyTheme(ThemeSpec{Colors: style.Colors{Annotation: "#000"}, ChromaStyle: "swapoff"})

	assert.Same(t, first, m.annot.markdown, "no builder → renderer must be left as-is")
}

func TestApplyTheme_invalidatesStaleRowCacheEntries(t *testing.T) {
	// the legacy fallback bakes the live AnnotationFg into cached rows via
	// AnnotationInline — stale entries from before the theme apply MUST be
	// gone, otherwise the next paint shows stale colors. (applyTheme also
	// re-renders the diff at the end which re-populates the cache with
	// fresh entries — that's expected; we only care that the old key is
	// invalidated.)
	m := modelWithMarkdown(t, nil, "hello")
	staleKey := annotCacheKey{body: "stale-body-not-in-store", prefix: "\U0001f4ac ", width: 50}
	m.annot.rowCache[staleKey] = []string{"stale"}

	m.applyTheme(ThemeSpec{Colors: style.Colors{Annotation: "#000"}, ChromaStyle: "swapoff"})

	_, present := m.annot.rowCache[staleKey]
	assert.False(t, present, "applyTheme must invalidate the stale cache entry")
}

func TestRenderWrappedAnnotation_widthForwardedToMarkdown(t *testing.T) {
	var seen int
	md := &mocks.AnnotationMarkdownMock{
		RenderFunc: func(body string, width int) []string {
			seen = width
			return []string{body}
		},
	}
	m := modelWithMarkdown(t, md, "text")
	prefix := "\U0001f4ac "
	m.annotationVisualRows(prefix, "text")
	expected := m.diffContentWidth() - 1 - lipgloss.Width(prefix)
	assert.Equal(t, expected, seen,
		"markdown renderer must receive contentWidth - 1 - prefix-width")
}
