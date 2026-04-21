package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

const (
	descriptionPopupMaxWidth    = 90
	descriptionPopupMinWidth    = 30
	descriptionPopupWidthRatio  = 0.9
	descriptionPopupChromeLines = 4 // border (2) + top/bottom padding (2)
	descriptionPopupBorderPad   = 6 // border (2) + padding-left/right (4)
	descriptionHeightMargin     = 4
	descriptionLexerName        = "description.md"
)

// DescriptionHighlighter renders a markdown source string as per-line ANSI
// strings. The overlay calls it with the description text and a fake markdown
// filename so the caller can route to chroma's markdown lexer. Implementations
// return nil to signal "render plain text instead".
type DescriptionHighlighter interface {
	HighlightText(filename, text string) []string
}

// DescriptionSpec carries the description content and an optional highlighter.
// Text is the raw markdown source; when Highlighter is non-nil and returns a
// non-nil slice the overlay renders chroma-highlighted lines, otherwise it
// falls back to the raw text.
type DescriptionSpec struct {
	Text        string
	Highlighter DescriptionHighlighter
}

type descriptionOverlay struct {
	spec   DescriptionSpec
	offset int
	height int
}

func (d *descriptionOverlay) open(spec DescriptionSpec) {
	d.spec = spec
	d.offset = 0
}

func (d *descriptionOverlay) render(ctx RenderCtx, mgr *Manager) string {
	d.height = ctx.Height

	popupWidth := d.popupWidth(ctx.Width)
	innerWidth := popupWidth - descriptionPopupBorderPad
	viewportHeight := d.viewportHeight(ctx.Height)

	content := d.buildContent(innerWidth)

	if maxOffset := max(len(content)-viewportHeight, 0); d.offset > maxOffset {
		d.offset = maxOffset
	}
	if d.offset < 0 {
		d.offset = 0
	}

	visible := d.applyScroll(content, viewportHeight)

	boxStyle := ctx.Resolver.Style(style.StyleKeyCommitInfoBox).Width(popupWidth)
	box := boxStyle.Render(strings.Join(visible, "\n"))

	accentFg := string(ctx.Resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg))
	return mgr.injectBorderTitle(box, " description ", popupWidth, accentFg, paneBg)
}

func (d *descriptionOverlay) popupWidth(termWidth int) int {
	target := int(float64(termWidth) * descriptionPopupWidthRatio)
	w := min(target, descriptionPopupMaxWidth)
	return max(w, descriptionPopupMinWidth)
}

func (d *descriptionOverlay) viewportHeight(termHeight int) int {
	usable := termHeight - descriptionHeightMargin - descriptionPopupChromeLines
	return max(usable, 1)
}

// buildContent returns padded, wrapped content lines ready for scroll/render.
// when Text is empty, renders a centered "no description" hint instead.
func (d *descriptionOverlay) buildContent(innerWidth int) []string {
	if strings.TrimSpace(d.spec.Text) == "" {
		return d.centeredMessage("no description", innerWidth, true)
	}

	rawLines := d.sourceLines(d.spec.Text)

	var out []string
	for _, raw := range rawLines {
		if raw == "" {
			out = append(out, d.padLine("", innerWidth))
			continue
		}
		for _, wrapped := range d.wrapLine(raw, innerWidth) {
			out = append(out, d.padLine(wrapped, innerWidth))
		}
	}
	return out
}

// sourceLines returns chroma-highlighted lines when a highlighter is attached
// and produces output, otherwise it splits the raw text on newlines.
func (d *descriptionOverlay) sourceLines(text string) []string {
	if d.spec.Highlighter != nil {
		if hl := d.spec.Highlighter.HighlightText(descriptionLexerName, text); hl != nil {
			return hl
		}
	}
	// preserve trailing blank lines as visible rows inside the popup.
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

// wrapLine wraps s to width using ANSI-aware soft wrap and re-emits active
// SGR state on each continuation line so chroma tokens that span wraps keep
// their color.
func (d *descriptionOverlay) wrapLine(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	wrapped := ansi.Wrap(s, width, "")
	lines := strings.Split(wrapped, "\n")
	return style.SGR{}.Reemit(lines)
}

// padLine right-pads line with spaces up to width so the overlay's background
// fills the full line — otherwise the terminal's default bg bleeds through.
func (d *descriptionOverlay) padLine(line string, width int) string {
	w := lipgloss.Width(line)
	if w >= width {
		return line
	}
	return line + strings.Repeat(" ", width-w)
}

// centeredMessage renders a one-line centered message, optionally italicized.
func (d *descriptionOverlay) centeredMessage(text string, innerWidth int, italic bool) []string {
	if italic {
		text = ansiItalicOn + text + ansiItalicOff
	}
	visual := lipgloss.Width(text)
	pad := max((innerWidth-visual)/2, 0)
	line := strings.Repeat(" ", pad) + text
	return []string{d.padLine(line, innerWidth)}
}

// applyScroll returns the slice of lines currently visible given the viewport
// height and the overlay's offset.
func (d *descriptionOverlay) applyScroll(content []string, viewportHeight int) []string {
	if viewportHeight <= 0 {
		return nil
	}
	if len(content) <= viewportHeight {
		return content
	}
	end := min(d.offset+viewportHeight, len(content))
	return content[d.offset:end]
}

func (d *descriptionOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionDescription ||
		action == keymap.ActionDismiss ||
		action == keymap.ActionQuit ||
		msg.Type == tea.KeyEsc {
		return Outcome{Kind: OutcomeClosed}
	}

	switch action { //nolint:exhaustive // navigation subset; other actions fall through
	case keymap.ActionDown:
		d.offset++
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionUp:
		if d.offset > 0 {
			d.offset--
		}
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageDown, keymap.ActionHalfPageDown:
		d.offset += d.viewportHeight(d.height)
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageUp, keymap.ActionHalfPageUp:
		d.offset -= d.viewportHeight(d.height)
		if d.offset < 0 {
			d.offset = 0
		}
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHome:
		d.offset = 0
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionEnd:
		d.offset = scrollEndSentinel
		return Outcome{Kind: OutcomeNone}
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'g':
			d.offset = 0
			return Outcome{Kind: OutcomeNone}
		case 'G':
			d.offset = scrollEndSentinel
			return Outcome{Kind: OutcomeNone}
		}
	}
	return Outcome{Kind: OutcomeNone}
}
