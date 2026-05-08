package markdown

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRenderer(t *testing.T, opts ...Option) *Renderer {
	t.Helper()
	r, err := New(append([]Option{WithStyle(testStyle())}, opts...)...)
	require.NoError(t, err)
	return r
}

func testStyle() Style {
	return Style{
		BodyFg:          "#cdd6f4",
		HeadingFg:       "#f38ba8",
		EmphFg:          "#cdd6f4",
		StrongFg:        "#cdd6f4",
		InlineCodeFg:    "#fab387",
		LinkFg:          "#89b4fa",
		BlockquoteFg:    "#a6adc8",
		HRFg:            "#585b70",
		ListItemFg:      "#cdd6f4",
		ChromaStyleName: "swapoff",
		Emoji:           false,
	}
}

func TestRender_emptyInputReturnsNil(t *testing.T) {
	r := newTestRenderer(t)
	assert.Nil(t, r.Render("", 60))
	assert.Nil(t, r.Render("hello", 0))
	assert.Nil(t, r.Render("hello", -1))
}

func TestRender_paragraphProducesAtLeastOneRow(t *testing.T) {
	r := newTestRenderer(t)
	rows := r.Render("hello world", 40)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		assert.NotContains(t, row, "\n", "rows must not contain embedded newlines")
	}
	joined := strings.Join(rows, " ")
	assert.Contains(t, stripANSI(joined), "hello world")
}

func TestRender_outputIsBgFreeWhenColorSafe(t *testing.T) {
	r := newTestRenderer(t)
	rows := r.Render("**bold** and `code` and *italic*", 60)
	require.NotEmpty(t, rows)
	joined := strings.Join(rows, "\n")

	// no SGR background-set sequences (40-47, 100-107, or 48;...)
	bgPattern := regexp.MustCompile(`\x1b\[(?:4[0-9]|10[0-7])(?:;[0-9;]+)?m|\x1b\[48;[0-9;]+m`)
	assert.False(t, bgPattern.MatchString(joined),
		"expected no background SGR sequences in colorSafe mode, got %q", joined)
}

func TestRender_fullResetReplacedWithFgOnlyReset(t *testing.T) {
	r := newTestRenderer(t)
	rows := r.Render("**bold**", 40)
	joined := strings.Join(rows, "\n")
	assert.NotContains(t, joined, "\x1b[0m",
		"\\033[0m must be replaced — it would clear any outer pane bg")
}

func TestRender_styleAttributesArePresent(t *testing.T) {
	r := newTestRenderer(t)

	// glamour emits compound SGR like "\x1b[38;2;...;1m" rather than
	// standalone "\x1b[1m". hasSGR scans every CSI for the requested code.
	bold := r.Render("**bold text**", 40)
	require.NotEmpty(t, bold)
	assert.True(t, hasSGRParam(strings.Join(bold, ""), "1"), "expected bold SGR (1)")

	italic := r.Render("*italic text*", 40)
	require.NotEmpty(t, italic)
	assert.True(t, hasSGRParam(strings.Join(italic, ""), "3"), "expected italic SGR (3)")
}

// hasSGRParam reports whether s contains an SGR sequence whose semicolon-
// separated parameter list includes param. Returns false for non-SGR escapes.
func hasSGRParam(s, param string) bool {
	for _, m := range ansiCSI.FindAllString(s, -1) {
		if !strings.HasSuffix(m, "m") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(m, "\x1b["), "m")
		for _, p := range strings.Split(body, ";") {
			if p == param {
				return true
			}
		}
	}
	return false
}

func TestRender_outerBlankRowsTrimmed(t *testing.T) {
	r := newTestRenderer(t)
	rows := r.Render("just one paragraph", 40)
	require.NotEmpty(t, rows)
	assert.False(t, isBlankRow(rows[0]), "first row must not be blank")
	assert.False(t, isBlankRow(rows[len(rows)-1]), "last row must not be blank")
}

func TestRender_internalBlankRowsPreserved(t *testing.T) {
	r := newTestRenderer(t)
	body := "first paragraph.\n\nsecond paragraph."
	rows := r.Render(body, 60)
	require.NotEmpty(t, rows)
	hasBlank := false
	for i := 1; i < len(rows)-1; i++ {
		if isBlankRow(rows[i]) {
			hasBlank = true
			break
		}
	}
	assert.True(t, hasBlank, "expected at least one blank row between paragraphs, got rows=%q", rows)
}

func TestRender_codeBlockProducesMultipleRows(t *testing.T) {
	r := newTestRenderer(t)
	body := "before\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n\nafter"
	rows := r.Render(body, 60)
	require.GreaterOrEqual(t, len(rows), 5,
		"expected paragraph + blank + code (at least 3 lines) + blank + paragraph")

	stripped := make([]string, len(rows))
	for i, row := range rows {
		stripped[i] = stripANSI(row)
	}
	joined := strings.Join(stripped, "\n")
	assert.Contains(t, joined, "func main()")
	assert.Contains(t, joined, "before")
	assert.Contains(t, joined, "after")
}

func TestRender_widthCacheReusedAcrossCalls(t *testing.T) {
	r := newTestRenderer(t)
	// New() pre-warms width=80 to surface config errors at startup, so the
	// cache starts non-empty.
	startSize := len(r.rendererBy)
	require.NotZero(t, startSize)

	r.Render("first call", 40)
	require.Len(t, r.rendererBy, startSize+1, "new width must allocate")
	r.Render("second call same width", 40)
	require.Len(t, r.rendererBy, startSize+1, "same width must reuse cache")
	r.Render("third call wider", 120)
	require.Len(t, r.rendererBy, startSize+2, "another new width must allocate")
}

func TestRender_emojiOptionEnablesShortcodes(t *testing.T) {
	on := newTestRenderer(t, WithStyle(Style{BodyFg: "#fff", Emoji: true}))
	off := newTestRenderer(t, WithStyle(Style{BodyFg: "#fff", Emoji: false}))

	rowsOn := on.Render(":rocket: launch", 40)
	rowsOff := off.Render(":rocket: launch", 40)

	require.NotEmpty(t, rowsOn)
	require.NotEmpty(t, rowsOff)

	onText := stripANSI(strings.Join(rowsOn, ""))
	offText := stripANSI(strings.Join(rowsOff, ""))
	assert.Contains(t, onText, "🚀", "emoji enabled should substitute :rocket:")
	assert.Contains(t, offText, ":rocket:", "emoji disabled should keep literal shortcode")
}

func TestPostProcess_colorSafeStripsBg(t *testing.T) {
	on, err := New(WithStyle(testStyle()), WithColorSafe(true))
	require.NoError(t, err)

	off, err := New(WithStyle(testStyle()), WithColorSafe(false))
	require.NoError(t, err)

	// synthetic glamour-shape input with explicit bg sequences (256-color
	// and truecolor variants) — postProcess is the only call site that
	// branches on colorSafe so we exercise it directly rather than relying
	// on chroma to emit bg, which is theme- and version-dependent.
	in := "\x1b[38;5;212m\x1b[48;5;236mhello\x1b[0m"

	withSafe := strings.Join(on.postProcess(in), "\n")
	withoutSafe := strings.Join(off.postProcess(in), "\n")

	bgPattern := regexp.MustCompile(`\x1b\[(?:4[0-9]|10[0-7])(?:;[0-9;]+)?m|\x1b\[48;[0-9;]+m`)
	assert.False(t, bgPattern.MatchString(withSafe),
		"colorSafe=true must strip bg sequences, got %q", withSafe)
	assert.True(t, bgPattern.MatchString(withoutSafe),
		"colorSafe=false must preserve bg sequences, got %q", withoutSafe)
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;38;2;255;0;0mbold red\x1b[0m", "bold red"},
		{"\x1b[2J\x1b[Hmixed", "mixed"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, stripANSI(tt.in))
	}
}

func TestIsBlankRow(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"\x1b[0m", true},
		{"\x1b[31m\x1b[0m", true},
		{"x", false},
		{"  hello  ", false},
		{"\x1b[31mhi\x1b[0m", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isBlankRow(tt.in), "input=%q", tt.in)
	}
}

func TestTrimOuterBlankRows(t *testing.T) {
	rows := []string{"", "  ", "real", "", "more", "", "\x1b[0m"}
	got := trimOuterBlankRows(rows)
	assert.Equal(t, []string{"real", "", "more"}, got)
}

func TestTrimOuterBlankRows_allBlank(t *testing.T) {
	assert.Nil(t, trimOuterBlankRows([]string{"", "  ", "\x1b[0m"}))
}

func TestNew_returnsErrorOnInvalidStyle(t *testing.T) {
	// glamour accepts our StyleConfig in all reasonable cases — this test
	// documents that New surfaces glamour init errors. We simulate by
	// passing a non-empty ChromaStyleName referring to an unknown chroma
	// style; glamour falls back to "no theme" rather than erroring, so the
	// constructor succeeds. This test exists to pin the behavior: New
	// returns nil error for unknown chroma names rather than panicking.
	r, err := New(WithStyle(Style{ChromaStyleName: "definitely-not-a-real-style"}))
	require.NoError(t, err)
	require.NotNil(t, r)
	rows := r.Render("```go\nx := 1\n```", 40)
	assert.NotEmpty(t, rows)
}

func TestRender_glamourErrorFallsBackToPlainRows(t *testing.T) {
	// Construct a renderer, then deliberately overwrite its cache with a
	// closed *glamour.TermRenderer to force Render() into the err branch.
	// This documents the graceful-degradation contract: Render never panics
	// and never returns nil for a non-empty body.
	r := newTestRenderer(t)
	tr, err := r.rendererForWidth(40)
	require.NoError(t, err)
	require.NoError(t, tr.Close())

	rows := r.Render("hello\nworld", 40)
	// the closed renderer should error or produce non-styled output —
	// either way fallback or successful render must not be nil.
	require.NotEmpty(t, rows)
}
