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
	infoPopupMaxWidth    = 90  // maximum popup width
	infoPopupMinWidth    = 30  // minimum popup width
	infoPopupWidthRatio  = 0.9 // fraction of terminal width used when below max
	infoPopupChromeLines = 4   // border (2) + top/bottom padding (2)
	infoPopupBorderPad   = 6   // border (2) + padding-left/right (4)
	infoHeightMargin     = 4   // leaves breathing room around the popup
	infoBodyIndent       = "  "
	infoDateFormat       = "2006-01-02"
	infoShortHashLen     = 12
	infoErrMaxLen        = 1000 // upper bound on rendered VCS-error text; runVCS embeds raw stderr which can be megabytes
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

type infoOverlay struct {
	spec   InfoSpec
	offset int
	height int // last known terminal height, updated on render
}

// renderedRow is the post-sanitize, post-truncate label/value pair built by
// buildDetailRows and consumed by renderDetailRow. value carries any
// muted-suffix already inlined as raw ANSI.
type renderedRow struct {
	label string
	value string
}

func (c *infoOverlay) open(spec InfoSpec) {
	c.spec = spec
	c.offset = 0
}

func (c *infoOverlay) render(ctx RenderCtx, mgr *Manager) string {
	c.height = ctx.Height

	popupWidth := c.popupWidth(ctx.Width)
	innerWidth := popupWidth - infoPopupBorderPad
	viewportHeight := c.viewportHeight(ctx.Height)

	content := c.buildContent(innerWidth, ctx.Resolver)

	if maxOffset := max(len(content)-viewportHeight, 0); c.offset > maxOffset {
		c.offset = maxOffset
	}
	if c.offset < 0 {
		c.offset = 0
	}

	visible := c.applyScroll(content, viewportHeight)

	boxStyle := ctx.Resolver.Style(style.StyleKeyInfoBox).Width(popupWidth)
	box := boxStyle.Render(strings.Join(visible, "\n"))

	accentFg := string(ctx.Resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg))

	title := " info "
	if h := strings.TrimSpace(c.spec.HeaderText); h != "" {
		title = " " + c.sanitizeInfoText(c.spec.HeaderText) + " "
	}
	edge := borderEdgeText{popupWidth: popupWidth, accentFg: accentFg, paneBg: paneBg}
	box = mgr.injectBorderTitle(box, title, edge)

	if f := strings.TrimSpace(c.spec.FooterText); f != "" {
		box = mgr.injectBorderFooter(box, " "+c.sanitizeInfoText(c.spec.FooterText)+" ", edge)
	}
	return box
}

// popupWidth returns the computed popup width clamped between min/max.
func (c *infoOverlay) popupWidth(termWidth int) int {
	target := int(float64(termWidth) * infoPopupWidthRatio)
	w := min(target, infoPopupMaxWidth)
	return max(w, infoPopupMinWidth)
}

// viewportHeight returns the content height inside the popup box.
func (c *infoOverlay) viewportHeight(termHeight int) int {
	usable := termHeight - infoHeightMargin - infoPopupChromeLines
	return max(usable, 1)
}

// buildContent assembles the wrapped, padded content for the popup. Three
// optional sections render top-to-bottom: agent description, session info,
// and the commit log. Each section that has nothing to show is skipped
// entirely (no header, no padding) so the popup never has dead space; a
// blank line separates the sections that do render. The total list of lines
// is then sliced by applyScroll using the overlay offset.
func (c *infoOverlay) buildContent(innerWidth int, resolver Resolver) []string {
	var out []string
	appendSection := func(section []string) {
		if len(section) == 0 {
			return
		}
		if len(out) > 0 {
			out = append(out, c.padLine("", innerWidth))
		}
		out = append(out, section...)
	}
	appendSection(c.buildDescriptionSection(innerWidth, resolver))
	appendSection(c.buildDetailRows(innerWidth, resolver))
	appendSection(c.buildCommitsSection(innerWidth, resolver))
	if len(out) == 0 {
		// Header+footer carry the popup's signal post-redesign; an empty body
		// is fine when both are populated (e.g. working-tree mode with no
		// description, no filters, no commits applicable). A single padded
		// blank row keeps the box from collapsing visually.
		return []string{c.padLine("", innerWidth)}
	}
	return out
}

// buildDescriptionSection renders the optional agent-supplied prose. Empty
// description returns nil so the section is skipped entirely. The text is
// wrapped to inner width; newlines in the source are honored as paragraph
// breaks. Caller is responsible for sanitizing the input before it reaches
// the spec — this code does not strip control bytes because chroma-style
// ANSI is preserved verbatim for highlighted output.
func (c *infoOverlay) buildDescriptionSection(innerWidth int, resolver Resolver) []string {
	if strings.TrimSpace(c.spec.Description) == "" {
		return nil
	}
	out := []string{c.sectionHeader("description", innerWidth, resolver)}
	for _, raw := range c.splitLines(c.spec.Description) {
		if raw == "" {
			out = append(out, c.padLine("", innerWidth))
			continue
		}
		for _, line := range c.wrapLine(raw, innerWidth) {
			out = append(out, c.padLine(line, innerWidth))
		}
	}
	return out
}

// buildDetailRows renders the trimmed session/detail block. Most session
// info now lives in the popup's top/bottom border labels (mode summary in
// the title, aggregate stats in the footer); this section only carries
// non-redundant rows passed in by the caller — typically the active
// --only/--include/--exclude filters, --compact options, and the working-
// directory root.
//
// The section starts with a "details" header that visually matches the
// description and commits headers — keeps the three sections symmetric
// even when only one or two are populated.
//
// Returns nil when no row has content, so the section (and its header) are
// fully hidden in the common case (no filters, no compact mode, no workDir).
func (c *infoOverlay) buildDetailRows(innerWidth int, resolver Resolver) []string {
	muted := string(resolver.Color(style.ColorKeyMutedFg))
	reset := string(style.ResetFg)

	visible := make([]renderedRow, 0, len(c.spec.Rows))
	for _, row := range c.spec.Rows {
		label := c.sanitizeInfoText(row.Label)
		// row values can carry upstream-supplied text (raw VCS-stderr in the
		// stats-unavailable row, --only/--include/--exclude flag values), so
		// cap them at the same limit as CommitsErr to keep popup rendering
		// bounded. Truncation runs after sanitize so the cap counts visible
		// runes, not stripped escapes.
		value := c.truncateForDisplay(c.sanitizeInfoText(row.Value), infoErrMaxLen)
		if label == "" || value == "" {
			continue
		}
		if suffix := c.sanitizeInfoText(row.MutedSuffix); suffix != "" {
			value += "  " + muted + c.truncateForDisplay(suffix, infoErrMaxLen) + reset
		}
		visible = append(visible, renderedRow{label: label, value: value})
	}
	if len(visible) == 0 {
		return nil
	}

	labelW := 0
	for _, row := range visible {
		if w := lipgloss.Width(row.label); w > labelW {
			labelW = w
		}
	}
	labelW = min(labelW, max(innerWidth/3, 1))

	out := []string{c.sectionHeader("details", innerWidth, resolver)}
	for _, row := range visible {
		out = append(out, c.renderDetailRow(row, detailRowLayout{
			labelW:     labelW,
			innerWidth: innerWidth,
			muted:      muted,
			reset:      reset,
		})...)
	}
	return out
}

// detailRowLayout carries the shared per-row layout values used by
// renderDetailRow. Bundled into a struct because the prior 6-positional shape
// (label, value, labelW, innerWidth, muted, reset) made silent reorder bugs
// possible (label/value and muted/reset are same-typed pairs).
type detailRowLayout struct {
	labelW     int
	innerWidth int
	muted      string
	reset      string
}

// renderDetailRow wraps a single label/value pair to innerWidth, returning
// padded lines. The first line carries a muted, label-width-padded label
// followed by two spaces of gutter; continuation lines align under the value
// with whitespace of the same total width. Caller is responsible for upstream
// label-width computation across the rendered rows.
func (c *infoOverlay) renderDetailRow(row renderedRow, layout detailRowLayout) []string {
	prefix := layout.muted + c.padPlain(row.label, layout.labelW) + layout.reset + "  "
	contPrefix := strings.Repeat(" ", layout.labelW+2)
	valueWidth := max(layout.innerWidth-layout.labelW-2, 1)
	wrapped := c.wrapLine(row.value, valueWidth)
	out := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		if i == 0 {
			out = append(out, c.padLine(prefix+line, layout.innerWidth))
			continue
		}
		out = append(out, c.padLine(contPrefix+line, layout.innerWidth))
	}
	return out
}

// buildCommitsSection renders the commit-log section. Entirely hidden when
// CommitsApplicable is false — the mode reason already lives in the session
// section, so a "no commits in this mode" stub would be redundant.
// CommitsErr is rendered inside the section (one centered line) and takes
// precedence over the empty-list path. An empty list with no error renders
// "no commits in range" centered inside the section.
func (c *infoOverlay) buildCommitsSection(innerWidth int, resolver Resolver) []string {
	if !c.spec.CommitsApplicable {
		return nil
	}
	out := []string{c.sectionHeader(c.commitsHeaderText(), innerWidth, resolver)}
	switch {
	case !c.spec.CommitsLoaded:
		out = append(out, c.centeredMessage("loading commits…", innerWidth, true)...)
		return out
	case c.spec.CommitsErr != nil:
		// runVCS embeds raw stderr in errors which may contain newlines/tabs
		// and may be megabytes for verbose VCSes; Fields collapses all
		// whitespace runs into single spaces so the centered-message
		// single-line layout assumption holds, and truncate caps total length
		// so a hostile stderr cannot blow up popup rendering.
		msg := c.truncateForDisplay(strings.Join(strings.Fields(c.spec.CommitsErr.Error()), " "), infoErrMaxLen)
		out = append(out, c.centeredMessage(msg, innerWidth, true)...)
		return out
	case len(c.spec.Commits) == 0:
		out = append(out, c.centeredMessage("no commits in range", innerWidth, false)...)
		return out
	}
	for i, commit := range c.spec.Commits {
		if i > 0 {
			out = append(out, c.padLine("", innerWidth))
		}
		out = append(out, c.renderCommit(commit, innerWidth, resolver)...)
	}
	return out
}

// commitsHeaderText returns the label for the commits section header. Returns
// a bare "commits" while the list is still loading (the count would display
// as "0" otherwise, which would mislead users into thinking there are no
// commits in the range). Includes the count and a "truncated" marker once
// loaded.
func (c *infoOverlay) commitsHeaderText() string {
	if !c.spec.CommitsLoaded {
		return "commits"
	}
	n := len(c.spec.Commits)
	if c.spec.Truncated {
		return fmt.Sprintf("commits (%d, truncated)", n)
	}
	return fmt.Sprintf("commits (%d)", n)
}

// sectionHeader produces a single bold accent-colored label line that marks
// the start of a section. No surrounding rule lines — the popup border and
// blank-line separators between sections give enough visual structure.
func (c *infoOverlay) sectionHeader(label string, innerWidth int, resolver Resolver) string {
	accent := string(resolver.Color(style.ColorKeyAccentFg))
	reset := string(style.ResetFg)
	return c.padLine(accent+ansiBoldOn+label+ansiBoldOff+reset, innerWidth)
}

// renderCommit produces all wrapped, padded lines for a single commit.
func (c *infoOverlay) renderCommit(commit diff.CommitInfo, innerWidth int, resolver Resolver) []string {
	var out []string

	accent := string(resolver.Color(style.ColorKeyAccentFg))
	muted := string(resolver.Color(style.ColorKeyMutedFg))
	reset := string(style.ResetFg)

	hash := c.shortHash(commit.Hash)
	date := commit.Date.Format(infoDateFormat)
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
		bodyInner := max(innerWidth-len(infoBodyIndent), 1)
		for _, rawLine := range c.splitBodyLines(body) {
			if rawLine == "" {
				out = append(out, c.padLine("", innerWidth))
				continue
			}
			for _, wrapped := range c.wrapLine(rawLine, bodyInner) {
				out = append(out, c.padLine(infoBodyIndent+wrapped, innerWidth))
			}
		}
	}

	return out
}

// centeredMessage renders a one-line centered message, optionally italicized.
func (c *infoOverlay) centeredMessage(text string, innerWidth int, italic bool) []string {
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
func (c *infoOverlay) wrapLine(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	wrapped := ansi.Wrap(s, width, "")
	lines := strings.Split(wrapped, "\n")
	return style.SGR{}.Reemit(lines)
}

// padLine right-pads line with spaces up to width so an optional box background
// fills the whole line. no-op when line already meets or exceeds width.
func (c *infoOverlay) padLine(line string, width int) string {
	w := lipgloss.Width(line)
	if w >= width {
		return line
	}
	return line + strings.Repeat(" ", width-w)
}

// applyScroll returns the slice of lines currently visible given the viewport
// height and the overlay's offset. returns content as-is when it fits; the
// outer lipgloss box sizes to content so no padding is needed.
func (c *infoOverlay) applyScroll(content []string, viewportHeight int) []string {
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
func (c *infoOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionInfo ||
		action == keymap.ActionDismiss ||
		action == keymap.ActionQuit ||
		msg.Type == tea.KeyEsc {
		return Outcome{Kind: OutcomeClosed}
	}

	full := c.viewportHeight(c.height)
	half := max(full/2, 1)
	switch action { //nolint:exhaustive // navigation subset; other actions fall through to rune handling
	case keymap.ActionDown:
		c.offset++
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionUp:
		if c.offset > 0 {
			c.offset--
		}
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageDown:
		c.offset += full
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageUp:
		c.offset -= full
		if c.offset < 0 {
			c.offset = 0
		}
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHalfPageDown:
		c.offset += half
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHalfPageUp:
		c.offset -= half
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
func (c *infoOverlay) handleMouse(msg tea.MouseMsg) Outcome {
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
func (c *infoOverlay) splitBodyLines(body string) []string {
	lines := strings.Split(normalizeNewlines(body), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// shortHash returns the first infoShortHashLen runes of hash, or the
// full hash when shorter.
func (c *infoOverlay) shortHash(hash string) string {
	runes := []rune(hash)
	if len(runes) <= infoShortHashLen {
		return string(runes)
	}
	return string(runes[:infoShortHashLen])
}

// splitLines normalizes CR/LF endings and splits on \n, preserving blank lines
// as empty entries (so paragraph breaks render). Trailing blanks are kept;
// the section builder turns them into padded empty lines, which are visually
// indistinguishable from the section's bottom margin.
func (c *infoOverlay) splitLines(s string) []string {
	return strings.Split(normalizeNewlines(s), "\n")
}

// normalizeNewlines converts CRLF and lone CR to LF.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// padPlain right-pads s with spaces up to width using lipgloss.Width for the
// measurement so wide-rune labels (CJK etc.) line up. no-op when s already
// meets or exceeds width.
func (c *infoOverlay) padPlain(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// truncateForDisplay caps s at limit runes, appending an ellipsis sentinel
// when truncation occurs. Operates on runes (not bytes) so multi-byte UTF-8
// stays well-formed. Used for upstream-supplied error text whose length is
// not otherwise bounded (raw VCS stderr can be megabytes).
func (c *infoOverlay) truncateForDisplay(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "… (truncated)"
}

// sanitizeInfoText neutralizes terminal-unsafe content before rendering values
// inside single-line info rows. Sanitization itself is delegated to
// diff.SanitizeCommitText so all untrusted text in the overlay routes through
// one strip path with identical semantics (full ANSI CSI sequences, ESC,
// C0/DEL/C1 controls, VCS framing delimiters, invalid UTF-8). After stripping,
// each surviving LF or TAB is mapped to a single space so multi-line values
// (e.g. workdir paths copied with embedded newlines) render on one row;
// runs of LF/TAB therefore expand to runs of spaces, and repeated normal
// spaces inside paths or patterns are preserved as-is.
// Trailing/leading whitespace is stripped at the boundary.
func (c *infoOverlay) sanitizeInfoText(s string) string {
	s = diff.SanitizeCommitText(s)
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
