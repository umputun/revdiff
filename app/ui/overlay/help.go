package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

const (
	helpHeightMargin     = 4 // leaves breathing room above and below the popup
	helpWidthMargin      = 4 // leaves breathing room left and right of the popup
	helpPopupChromeLines = 4 // border (2) + top/bottom padding (2)
	helpPopupBorderPad   = 6 // border (2) + padding-left/right (4)
	helpColumnGap        = 4 // spaces between the two columns
)

type helpOverlay struct {
	spec   HelpSpec
	offset int
	height int // last known terminal height, updated on render
	// viewport is the body-row count of the last render. It is not always
	// usableHeight — an overflowing body spends one row on the scroll hint —
	// and paging must step by exactly this, or every page boundary skips the
	// row the hint displaced.
	viewport int
}

// sectionBlock is one laid-out help section: a header line followed by
// key/description lines with the keys column padded to a common width.
type sectionBlock struct {
	lines []string
}

func (h *helpOverlay) open(spec HelpSpec) {
	h.spec = spec
	h.offset = 0
}

func (h *helpOverlay) render(ctx RenderCtx, _ *Manager) string {
	h.height = ctx.Height

	innerWidth := max(ctx.Width-helpWidthMargin-helpPopupBorderPad, 1)
	content := h.layout(h.buildBlocks(ctx.Resolver), innerWidth)

	// the hint costs a body row, so it is only affordable when there are at
	// least two: on a terminal down to the box's 5-row floor the bindings
	// themselves matter more than the cue to scroll.
	usable := h.usableHeight(ctx.Height)
	showHint := len(content) > usable && usable > 1
	viewportHeight := usable
	if showHint {
		viewportHeight = usable - 1
	}
	h.viewport = viewportHeight

	if maxOffset := max(len(content)-viewportHeight, 0); h.offset > maxOffset {
		h.offset = maxOffset
	}
	if h.offset < 0 {
		h.offset = 0
	}

	// pad to the width of the whole body, not of the visible slice, so the box
	// keeps one size while scrolling instead of resizing under the cursor.
	bodyWidth := min(h.widest(content), innerWidth)

	end := min(h.offset+viewportHeight, len(content))
	visible := make([]string, 0, viewportHeight+1)
	for _, row := range content[h.offset:end] {
		visible = append(visible, h.padLine(row, bodyWidth))
	}
	if showHint {
		visible = append(visible, h.scrollHint(len(content), end, bodyWidth, ctx.Resolver))
	}

	boxStyle := ctx.Resolver.Style(style.StyleKeyHelpBox)
	return boxStyle.Render(strings.Join(visible, "\n"))
}

// buildBlocks turns each spec section into a block of rendered lines: a colored
// header followed by one line per binding, keys padded to the section's widest
// key string so descriptions line up.
func (h *helpOverlay) buildBlocks(resolver Resolver) []sectionBlock {
	reset, headerColor, keyColor := h.colors(resolver)

	blocks := make([]sectionBlock, 0, len(h.spec.Sections))
	for _, sec := range h.spec.Sections {
		var block sectionBlock
		block.lines = append(block.lines, headerColor+sec.Title+reset)

		maxW := 0
		for _, e := range sec.Entries {
			if w := runewidth.StringWidth(e.Keys); w > maxW {
				maxW = w
			}
		}
		for _, e := range sec.Entries {
			pad := max(maxW-runewidth.StringWidth(e.Keys), 0)
			block.lines = append(block.lines, fmt.Sprintf("  %s%s%s%s  %s",
				keyColor, e.Keys, reset, strings.Repeat(" ", pad), e.Description))
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// layout arranges the blocks into popup body rows. Two columns are preferred
// because they roughly halve the height, but they need close to twice the
// width, so a terminal too narrow for them falls back to a single column —
// which is taller and scrolls rather than being cut off at the right edge.
// Rows still wider than innerWidth after that (a very narrow terminal) are
// truncated, since the popup must never be wider than the screen.
func (h *helpOverlay) layout(blocks []sectionBlock, innerWidth int) []string {
	rows := h.twoColumnRows(blocks)
	if h.widest(rows) > innerWidth {
		rows = h.singleColumnRows(blocks)
	}
	for i, row := range rows {
		if lipgloss.Width(row) > innerWidth {
			rows[i] = ansi.Truncate(row, innerWidth, "")
		}
	}
	return rows
}

// singleColumnRows stacks the blocks vertically, separated by a blank line.
func (h *helpOverlay) singleColumnRows(blocks []sectionBlock) []string {
	var out []string
	for i, b := range blocks {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, b.lines...)
	}
	return out
}

// twoColumnRows splits the blocks into two side-by-side columns at the halfway
// point of the total line count, padding the left column to a common width so
// the right column starts on the same screen column for every row.
func (h *helpOverlay) twoColumnRows(blocks []sectionBlock) []string {
	totalLines := 0
	for _, b := range blocks {
		totalLines += len(b.lines) + 1
	}

	var leftBlocks, rightBlocks []sectionBlock
	leftLines, half := 0, totalLines/2
	for _, b := range blocks {
		if leftLines < half {
			leftBlocks = append(leftBlocks, b)
			leftLines += len(b.lines) + 1
			continue
		}
		rightBlocks = append(rightBlocks, b)
	}

	left := h.singleColumnRows(leftBlocks)
	right := h.singleColumnRows(rightBlocks)
	leftWidth := h.widest(left)

	maxRows := max(len(left), len(right))
	rows := make([]string, 0, maxRows)
	for i := range maxRows {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		row := l + strings.Repeat(" ", max(leftWidth-lipgloss.Width(l), 0))
		if i < len(right) {
			row += strings.Repeat(" ", helpColumnGap) + right[i]
		}
		rows = append(rows, row)
	}
	return rows
}

// usableHeight returns how many body rows fit inside the popup box.
func (h *helpOverlay) usableHeight(termHeight int) int {
	return max(termHeight-helpHeightMargin-helpPopupChromeLines, 1)
}

// pageSize returns the scroll step for a full page: the last rendered viewport
// height, falling back to the usable height before the first render.
func (h *helpOverlay) pageSize() int {
	if h.viewport > 0 {
		return h.viewport
	}
	return h.usableHeight(h.height)
}

// widest returns the visual width of the longest line.
func (h *helpOverlay) widest(lines []string) int {
	w := 0
	for _, line := range lines {
		if x := lipgloss.Width(line); x > w {
			w = x
		}
	}
	return w
}

// scrollHint renders the muted footer row shown when the body does not fit.
// It is the only cue that more bindings exist past the fold, so it carries the
// position as well as the keys — without it a clipped section reads as missing.
func (h *helpOverlay) scrollHint(total, end, bodyWidth int, resolver Resolver) string {
	text := fmt.Sprintf("↑/↓ scroll · %d-%d of %d", h.offset+1, end, total)
	pad := max((bodyWidth-runewidth.StringWidth(text))/2, 0)
	hint := strings.Repeat(" ", pad) + string(resolver.Color(style.ColorKeyMutedFg)) + text + string(style.ResetFg)
	if lipgloss.Width(hint) > bodyWidth {
		return ansi.Truncate(hint, bodyWidth, "")
	}
	return h.padLine(hint, bodyWidth)
}

// padLine right-pads line with spaces up to width so the box background fills
// the whole row. no-op when line already meets or exceeds width.
func (h *helpOverlay) padLine(line string, width int) string {
	w := lipgloss.Width(line)
	if w >= width {
		return line
	}
	return line + strings.Repeat(" ", width-w)
}

// handleKey dispatches overlay keys: navigation updates offset, dismissal keys
// close the overlay. offset is clamped on the next render.
func (h *helpOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionHelp || action == keymap.ActionDismiss || msg.Type == tea.KeyEsc {
		return Outcome{Kind: OutcomeClosed}
	}

	full := h.pageSize()
	half := max(full/2, 1)
	switch action { //nolint:exhaustive // navigation subset; other actions fall through to rune handling
	case keymap.ActionDown:
		h.offset++
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionUp:
		h.offset = max(h.offset-1, 0)
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageDown:
		h.offset += full
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionPageUp:
		h.offset = max(h.offset-full, 0)
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHalfPageDown:
		h.offset += half
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHalfPageUp:
		h.offset = max(h.offset-half, 0)
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionHome:
		h.offset = 0
		return Outcome{Kind: OutcomeNone}
	case keymap.ActionEnd:
		h.offset = scrollEndSentinel // clamped in render
		return Outcome{Kind: OutcomeNone}
	}

	// vim-style g / G accepted without requiring a keymap binding.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'g':
			h.offset = 0
		case 'G':
			h.offset = scrollEndSentinel
		}
	}
	return Outcome{Kind: OutcomeNone}
}

// handleMouse scrolls the help body in response to wheel events. plain wheel
// moves by WheelStep rows, shift+wheel by half a page. clicks and other buttons
// are consumed so they do not leak through to the diff/tree panes underneath.
func (h *helpOverlay) handleMouse(msg tea.MouseMsg) Outcome {
	if msg.Action != tea.MouseActionPress {
		return Outcome{Kind: OutcomeNone}
	}
	step := WheelStep
	if msg.Shift {
		step = max(h.pageSize()/2, 1)
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		h.offset += step
	case tea.MouseButtonWheelUp:
		h.offset = max(h.offset-step, 0)
	default:
		return Outcome{Kind: OutcomeNone}
	}
	return Outcome{Kind: OutcomeNone}
}

// colors returns ANSI color sequences for help overlay rendering.
func (h *helpOverlay) colors(resolver Resolver) (reset, header, key string) {
	return string(style.ResetFg), string(resolver.Color(style.ColorKeyAccentFg)),
		string(resolver.Color(style.ColorKeyAnnotationFg))
}
