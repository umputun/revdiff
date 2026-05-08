package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
)

// Renderer turns markdown bodies into ANSI-styled visual rows. It wraps
// glamour with a per-width cache of *glamour.TermRenderer instances (glamour
// bakes the wrap width into the renderer at construction time, so callers
// that want the same body re-rendered at a different width need a different
// renderer). Renderer is not safe for concurrent use; revdiff's UI runs on a
// single goroutine and constructs one Renderer at startup.
type Renderer struct {
	style       Style
	cfgEmoji    bool
	rendererBy  map[int]*glamour.TermRenderer
	colorSafe   bool
	cleanResets *regexp.Regexp
	cleanBg     *regexp.Regexp
}

// Option configures a Renderer.
type Option func(*Renderer)

// WithStyle applies the given Style to all rendered output.
func WithStyle(s Style) Option {
	return func(r *Renderer) {
		r.style = s
		r.cfgEmoji = s.Emoji
	}
}

// WithColorSafe forces foreground-only output by stripping every SGR
// background-color sequence from rendered rows. Defaults to true: revdiff's
// pane backgrounds are supplied by the caller via extendLineBg, and any
// background bytes inside row content would clash with the lipgloss-bg
// invariants documented in CLAUDE.md.
func WithColorSafe(on bool) Option {
	return func(r *Renderer) { r.colorSafe = on }
}

// New constructs a Renderer with the supplied options.
func New(opts ...Option) (*Renderer, error) {
	r := &Renderer{
		rendererBy: make(map[int]*glamour.TermRenderer),
		colorSafe:  true,
		// matches \033[0m (full SGR reset) — replaced with a fg+attrs reset
		// that leaves background untouched so an outer pane bg survives.
		cleanResets: regexp.MustCompile(`\x1b\[0m`),
		// matches \033[<bg>m where <bg> is 40-47, 48;..., 49, 100-107.
		// Used when colorSafe is true to strip every background-set sequence.
		cleanBg: regexp.MustCompile(`\x1b\[(?:4[0-9]|10[0-7])(?:;[0-9;]+)?m|\x1b\[48;[0-9;]+m`),
	}
	for _, opt := range opts {
		opt(r)
	}
	// Construct one renderer at default width up front so we surface
	// glamour configuration errors at startup rather than on first render.
	if _, err := r.rendererForWidth(80); err != nil {
		return nil, fmt.Errorf("markdown renderer: %w", err)
	}
	return r, nil
}

// Render renders body to a slice of visual rows of the given width. Each row
// is an ANSI-styled string (no embedded newlines). Outer blank rows (glamour
// always emits leading/trailing block margins) are trimmed. An empty body or
// non-positive width returns nil.
//
// On any glamour error, Render falls back to returning body split by line so
// the caller still sees the prose — the failure becomes a visual regression
// (no markdown styling) rather than an empty annotation.
func (r *Renderer) Render(body string, width int) []string {
	if body == "" || width <= 0 {
		return nil
	}
	tr, err := r.rendererForWidth(width)
	if err != nil {
		return fallbackRows(body)
	}
	out, err := tr.Render(body)
	if err != nil {
		return fallbackRows(body)
	}
	return r.postProcess(out)
}

// rendererForWidth returns a cached *glamour.TermRenderer configured for the
// given wrap width, constructing one on first use. width=0 disables wrapping.
func (r *Renderer) rendererForWidth(width int) (*glamour.TermRenderer, error) {
	if tr, ok := r.rendererBy[width]; ok {
		return tr, nil
	}
	opts := []glamour.TermRendererOption{
		glamour.WithStyles(r.style.toGlamourConfig()),
		glamour.WithWordWrap(width),
		glamour.WithColorProfile(termenv.TrueColor),
	}
	if r.cfgEmoji {
		opts = append(opts, glamour.WithEmoji())
	}
	tr, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return nil, err
	}
	r.rendererBy[width] = tr
	return tr, nil
}

// postProcess normalizes glamour output for safe composition with revdiff's
// pane backgrounds: replaces full-reset SGR with a fg+attrs-only reset,
// optionally strips background-set sequences, splits on \n, and drops outer
// blank rows produced by glamour's block margins.
func (r *Renderer) postProcess(s string) []string {
	// \033[0m clears every SGR attribute, including the background that
	// app/highlight is careful never to touch. Replace with a compound that
	// resets intensity, italic, underline, inverse, strike, fg — but leaves
	// bg alone (matching the pattern documented in CLAUDE.md "ANSI nesting
	// with lipgloss" / app/highlight's chroma fg-only emission).
	s = r.cleanResets.ReplaceAllString(s, "\x1b[22;23;24;27;29;39m")

	if r.colorSafe {
		s = r.cleanBg.ReplaceAllString(s, "")
	}

	// Drop leading/trailing fully-empty rows (glamour wraps every render
	// in a Document block whose default margin emits a blank line above
	// and below). Internal blanks (between paragraphs, around code blocks)
	// are preserved.
	rows := strings.Split(s, "\n")
	rows = trimOuterBlankRows(rows)
	return rows
}

// fallbackRows returns body split by line so callers always get something
// renderable when glamour fails. No styling is applied — the caller's outer
// AnnotationInline wrapping (in app/ui) supplies the prose color.
func fallbackRows(body string) []string {
	rows := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(rows) == 1 && rows[0] == "" {
		return nil
	}
	return rows
}

// trimOuterBlankRows drops rows from the head and tail of rows whose visible
// content is empty (after ANSI is stripped). Internal blank rows are kept.
func trimOuterBlankRows(rows []string) []string {
	first, last := 0, len(rows)-1
	for first <= last && isBlankRow(rows[first]) {
		first++
	}
	for last >= first && isBlankRow(rows[last]) {
		last--
	}
	if first > last {
		return nil
	}
	return rows[first : last+1]
}

// isBlankRow reports whether the row contains no visible cells. ANSI escape
// sequences are stripped before checking so a row of "\x1b[0m" or pure
// whitespace counts as blank.
func isBlankRow(s string) bool {
	stripped := stripANSI(s)
	return strings.TrimSpace(stripped) == ""
}

// stripANSI removes every CSI sequence from s. We don't need a full ANSI
// parser here — the only sequences glamour emits are SGR (`\033[...m`) and
// occasionally cursor-position resets that don't appear in rendered text.
var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiCSI.ReplaceAllString(s, "") }
