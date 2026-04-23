package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

const (
	commitInfoPopupMaxWidth    = 90  // maximum popup width
	commitInfoPopupMinWidth    = 30  // minimum popup width
	commitInfoPopupWidthRatio  = 0.9 // fraction of terminal width used when below max
	commitInfoPopupChromeLines = 4   // border (2) + top/bottom padding (2)
	commitInfoPopupBorderPad   = 6   // border (2) + padding-left/right (4)
	commitInfoHeightMargin     = 4   // leaves breathing room around the popup
	commitInfoBodyIndent       = "  "
	commitInfoDateFormat       = "2006-01-02"
	commitInfoShortHashLen     = 12
)

// ANSI inline sequences kept local to avoid lipgloss full-reset leaking into the
// box background. buildStyles emits raw ANSI for colors and bold/italic toggles
// so continuation lines after wrap can re-emit the active attribute cleanly.
const (
	ansiBoldOn    = "\x1b[1m"
	ansiBoldOff   = "\x1b[22m"
	ansiItalicOn  = "\x1b[3m"
	ansiItalicOff = "\x1b[23m"
)

type commitInfoOverlay struct {
	spec   CommitInfoSpec
	offset int
	height int // last known terminal height, updated on render
}

func (c *commitInfoOverlay) open(spec CommitInfoSpec) {
	c.spec = spec
	c.offset = 0
}

func (c *commitInfoOverlay) render(ctx RenderCtx, mgr *Manager) string {
	c.height = ctx.Height

	popupWidth := c.popupWidth(ctx.Width)
	innerWidth := popupWidth - commitInfoPopupBorderPad
	viewportHeight := c.viewportHeight(ctx.Height)

	content := c.buildContent(innerWidth, ctx.Resolver)

	if maxOffset := max(len(content)-viewportHeight, 0); c.offset > maxOffset {
		c.offset = maxOffset
	}
	if c.offset < 0 {
		c.offset = 0
	}

	visible := c.applyScroll(content, viewportHeight)

	boxStyle := ctx.Resolver.Style(style.StyleKeyCommitInfoBox).Width(popupWidth)
	box := boxStyle.Render(strings.Join(visible, "\n"))

	title := c.titleText()
	accentFg := string(ctx.Resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg))
	return mgr.injectBorderTitle(box, title, popupWidth, accentFg, paneBg)
}

// popupWidth returns the computed popup width clamped between min/max.
func (c *commitInfoOverlay) popupWidth(termWidth int) int {
	target := int(float64(termWidth) * commitInfoPopupWidthRatio)
	w := min(target, commitInfoPopupMaxWidth)
	return max(w, commitInfoPopupMinWidth)
}

// viewportHeight returns the content height inside the popup box.
func (c *commitInfoOverlay) viewportHeight(termHeight int) int {
	usable := termHeight - commitInfoHeightMargin - commitInfoPopupChromeLines
	return max(usable, 1)
}

// titleText returns the centered title string injected into the top border.
func (c *commitInfoOverlay) titleText() string {
	n := len(c.spec.Commits)
	if c.spec.Truncated {
		return fmt.Sprintf(" commits (%d, truncated) ", n)
	}
	return fmt.Sprintf(" commits (%d) ", n)
}

// buildContent returns the wrapped content lines ready for scroll/render.
func (c *commitInfoOverlay) buildContent(innerWidth int, resolver Resolver) []string {
	switch {
	case !c.spec.Applicable:
		return c.centeredMessage("no commits in this mode", innerWidth, true)
	case c.spec.Err != nil:
		// runVCS embeds raw stderr in errors which may contain newlines/tabs;
		// Fields collapses all whitespace runs into single spaces so the
		// centered-message single-line layout assumption holds.
		msg := strings.Join(strings.Fields(c.spec.Err.Error()), " ")
		return c.centeredMessage(msg, innerWidth, true)
	case len(c.spec.Commits) == 0:
		return c.centeredMessage("no commits in range", innerWidth, false)
	}

	var out []string
	for i, commit := range c.spec.Commits {
		if i > 0 {
			out = append(out, c.padLine("", innerWidth))
		}
		out = append(out, c.renderCommit(commit, innerWidth, resolver)...)
	}
	return out
}

// renderCommit produces all wrapped, padded lines for a single commit.
func (c *commitInfoOverlay) renderCommit(commit diff.CommitInfo, innerWidth int, resolver Resolver) []string {
	var out []string

	accent := string(resolver.Color(style.ColorKeyAccentFg))
	muted := string(resolver.Color(style.ColorKeyMutedFg))
	reset := string(style.ResetFg)

	hash := c.shortHash(commit.Hash)
	date := commit.Date.Format(commitInfoDateFormat)
	meta := accent + hash + reset
	if commit.Author != "" {
		meta += " " + muted + commit.Author + reset
	}
	if !commit.Date.IsZero() {
		meta += " " + muted + date + reset
	}
	for _, line := range c.wrapLine(meta, innerWidth) {
		out = append(out, c.padLine(line, innerWidth))
	}

	if subject := strings.TrimSpace(commit.Subject); subject != "" {
		// wrap plain subject first, then re-emit bold on each continuation line
		// since ansi.Wrap does not preserve SGR attributes across inserted newlines.
		for _, line := range c.wrapLine(subject, innerWidth) {
			out = append(out, c.padLine(ansiBoldOn+line+ansiBoldOff, innerWidth))
		}
	}

	if body := strings.TrimSpace(commit.Body); body != "" {
		out = append(out, c.padLine("", innerWidth))
		bodyInner := max(innerWidth-len(commitInfoBodyIndent), 1)
		for _, rawLine := range c.splitBodyLines(body) {
			if rawLine == "" {
				out = append(out, c.padLine("", innerWidth))
				continue
			}
			for _, wrapped := range c.wrapLine(rawLine, bodyInner) {
				out = append(out, c.padLine(commitInfoBodyIndent+wrapped, innerWidth))
			}
		}
	}

	return out
}

// centeredMessage renders a one-line centered message, optionally italicized.
func (c *commitInfoOverlay) centeredMessage(text string, innerWidth int, italic bool) []string {
	if italic {
		text = ansiItalicOn + text + ansiItalicOff
	}
	visual := lipgloss.Width(text)
	pad := max((innerWidth-visual)/2, 0)
	line := strings.Repeat(" ", pad) + text
	return []string{c.padLine(line, innerWidth)}
}

// wrapLine wraps s to width using ANSI-aware soft wrap and re-emits active SGR
// state on each continuation line. ansi.Wrap can split between an SGR opening
// and its paired reset (e.g. inside a muted author/date span in meta); without
// re-emit, continuation lines would lose the active color. style.SGR.Reemit
// scans each line for active attributes and prepends them to the next.
func (c *commitInfoOverlay) wrapLine(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	wrapped := ansi.Wrap(s, width, "")
	lines := strings.Split(wrapped, "\n")
	return style.SGR{}.Reemit(lines)
}

// padLine right-pads line with spaces up to width so an optional box background
// fills the whole line. no-op when line already meets or exceeds width.
func (c *commitInfoOverlay) padLine(line string, width int) string {
	w := lipgloss.Width(line)
	if w >= width {
		return line
	}
	return line + strings.Repeat(" ", width-w)
}

// applyScroll returns the slice of lines currently visible given the viewport
// height and the overlay's offset. returns content as-is when it fits; the
// outer lipgloss box sizes to content so no padding is needed.
func (c *commitInfoOverlay) applyScroll(content []string, viewportHeight int) []string {
	if viewportHeight <= 0 {
		return nil
	}
	if len(content) <= viewportHeight {
		return content
	}
	end := min(c.offset+viewportHeight, len(content))
	return content[c.offset:end]
}

// handleKey dispatches overlay keys: navigation updates offset, dismissal keys
// close the overlay. offset is clamped on the next render.
func (c *commitInfoOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionCommitInfo ||
		action == keymap.ActionDismiss ||
		action == keymap.ActionQuit ||
		msg.Type == tea.KeyEsc {
		return Outcome{Kind: OutcomeClosed}
	}

	switch action { //nolint:exhaustive // navigation subset; other actions fall through to rune handling below
	case keymap.ActionDown:
		c.offset++
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionUp:
		if c.offset > 0 {
			c.offset--
		}
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageDown, keymap.ActionHalfPageDown:
		c.offset += c.viewportHeight(c.height)
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageUp, keymap.ActionHalfPageUp:
		c.offset -= c.viewportHeight(c.height)
		if c.offset < 0 {
			c.offset = 0
		}
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHome:
		c.offset = 0
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionEnd:
		c.offset = scrollEndSentinel // clamped in render
		return Outcome{Kind: OutcomeNone}
	}

	// vim-style g / G accepted without requiring a keymap binding.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'g':
			c.offset = 0
			return Outcome{Kind: OutcomeNone}
		case 'G':
			c.offset = scrollEndSentinel
			return Outcome{Kind: OutcomeNone}
		}
	}
	return Outcome{Kind: OutcomeNone}
}

// scrollEndSentinel is a large offset value that render clamps to the last page.
const scrollEndSentinel = 1 << 30

// WheelStep is the offset delta applied per plain wheel notch inside overlay
// popups. exported so app/ui can reuse the same constant for diff-pane wheel
// scrolling — keeps the feel consistent across overlay and non-overlay panes.
const WheelStep = 3

// handleMouse scrolls the popup in response to wheel events. plain wheel moves
// by WheelStep lines; shift+wheel moves by half the viewport height.
// non-wheel buttons and non-press actions are ignored — clicks outside the box
// do not dismiss the overlay (symmetric with the other overlays). render
// clamps the resulting offset, so only the lower bound is enforced here.
func (c *commitInfoOverlay) handleMouse(msg tea.MouseMsg) Outcome {
	if msg.Action != tea.MouseActionPress {
		return Outcome{Kind: OutcomeNone}
	}
	step := WheelStep
	if msg.Shift {
		step = max(c.viewportHeight(c.height)/2, 1)
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		c.offset += step
	case tea.MouseButtonWheelUp:
		c.offset -= step
		if c.offset < 0 {
			c.offset = 0
		}
	default:
		return Outcome{Kind: OutcomeNone}
	}
	return Outcome{Kind: OutcomeNone}
}

// splitBodyLines splits the commit body into individual lines, normalizing CR/LF
// endings. trailing blank lines are dropped so we don't emit dangling padding.
func (c *commitInfoOverlay) splitBodyLines(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// shortHash returns the first commitInfoShortHashLen runes of hash, or the
// full hash when shorter.
func (c *commitInfoOverlay) shortHash(hash string) string {
	runes := []rune(hash)
	if len(runes) <= commitInfoShortHashLen {
		return string(runes)
	}
	return string(runes[:commitInfoShortHashLen])
}
