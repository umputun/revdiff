package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

type helpOverlay struct {
	spec HelpSpec
}

func (h *helpOverlay) open(spec HelpSpec) {
	h.spec = spec
}

func (h *helpOverlay) render(ctx RenderCtx, _ *Manager) string {
	sections := h.spec.Sections

	type sectionBlock struct {
		lines []string
	}
	blocks := make([]sectionBlock, 0, len(sections))

	reset, headerColor, keyColor := helpColors(ctx.Resolver)

	for _, sec := range sections {
		var block sectionBlock
		block.lines = append(block.lines, headerColor+sec.Title+reset)

		type entry struct{ keys, desc string }
		entries := make([]entry, 0, len(sec.Entries))
		maxW := 0
		for _, e := range sec.Entries {
			entries = append(entries, entry{e.Keys, e.Description})
			if w := runewidth.StringWidth(e.Keys); w > maxW {
				maxW = w
			}
		}
		for _, e := range entries {
			pad := max(maxW-runewidth.StringWidth(e.keys), 0)
			block.lines = append(block.lines, fmt.Sprintf("  %s%s%s%s  %s",
				keyColor, e.keys, reset, strings.Repeat(" ", pad), e.desc))
		}
		blocks = append(blocks, block)
	}

	totalLines := 0
	for _, b := range blocks {
		totalLines += len(b.lines) + 1
	}

	var leftBlocks, rightBlocks []sectionBlock
	leftLines := 0
	half := totalLines / 2
	for _, b := range blocks {
		blockSize := len(b.lines) + 1
		if leftLines < half {
			leftBlocks = append(leftBlocks, b)
			leftLines += blockSize
		} else {
			rightBlocks = append(rightBlocks, b)
		}
	}

	renderColumn := func(colBlocks []sectionBlock) []string {
		var result []string
		for i, b := range colBlocks {
			if i > 0 {
				result = append(result, "")
			}
			result = append(result, b.lines...)
		}
		return result
	}

	left := renderColumn(leftBlocks)
	right := renderColumn(rightBlocks)

	leftWidth := 0
	for _, line := range left {
		if w := lipgloss.Width(line); w > leftWidth {
			leftWidth = w
		}
	}

	const gap = 4
	maxRows := max(len(left), len(right))
	var buf strings.Builder
	for i := range maxRows {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		pad := max(leftWidth-lipgloss.Width(l), 0)
		buf.WriteString(l)
		buf.WriteString(strings.Repeat(" ", pad))

		if i < len(right) {
			buf.WriteString(strings.Repeat(" ", gap))
			buf.WriteString(right[i])
		}
		if i < maxRows-1 {
			buf.WriteString("\n")
		}
	}

	boxStyle := ctx.Resolver.Style(style.StyleKeyHelpBox)
	return boxStyle.Render(buf.String())
}

func (h *helpOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionHelp || action == keymap.ActionDismiss || msg.Type == tea.KeyEsc {
		return Outcome{Kind: OutcomeClosed}
	}
	return Outcome{Kind: OutcomeNone}
}

// handleMouse is a no-op — the help overlay has no scrollable state. wheel
// and click events are simply consumed so they do not leak through to the
// diff/tree panes underneath.
func (h *helpOverlay) handleMouse(_ tea.MouseMsg) Outcome {
	return Outcome{Kind: OutcomeNone}
}

// helpColors returns ANSI color sequences for help overlay rendering.
func helpColors(resolver Resolver) (reset, header, key string) {
	return string(style.ResetFg), string(resolver.Color(style.ColorKeyAccentFg)),
		string(resolver.Color(style.ColorKeyAnnotationFg))
}
