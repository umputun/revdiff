package overlay

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

type annotListOverlay struct {
	items  []AnnotationItem
	cursor int
	offset int
	height int // last known terminal height, updated on render
}

func (a *annotListOverlay) open(spec AnnotListSpec) {
	a.items = spec.Items
	a.cursor = 0
	a.offset = 0
}

func (a *annotListOverlay) render(ctx RenderCtx, mgr *Manager) string {
	a.height = ctx.Height
	popupWidth := max(min(ctx.Width-10, 70), 20)

	if len(a.items) == 0 {
		return a.emptyOverlay(popupWidth, ctx.Resolver, mgr)
	}

	maxVisibleItems := a.maxVisible(ctx.Height)
	contentWidth := popupWidth - 4 // 2 for border + 2 for padding

	var lines []string
	for i := a.offset; i < len(a.items) && i < a.offset+maxVisibleItems; i++ {
		line := a.formatItem(a.items[i], contentWidth, i == a.cursor, ctx.Resolver)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	title := fmt.Sprintf(" annotations (%d) ", len(a.items))

	box := a.boxStyle(popupWidth, ctx.Resolver).Render(content)

	accentFg := string(ctx.Resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg))
	box = mgr.injectBorderTitle(box, title, popupWidth, accentFg, paneBg)

	return box
}

func (a *annotListOverlay) emptyOverlay(popupWidth int, resolver Resolver, mgr *Manager) string {
	text := "no annotations"
	innerWidth := popupWidth - 4
	pad := max((innerWidth-lipgloss.Width(text))/2, 0)
	centered := strings.Repeat(" ", pad) + text

	box := a.boxStyle(popupWidth, resolver).Render(centered)
	title := " annotations (0) "
	accentFg := string(resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(resolver.Color(style.ColorKeyDiffPaneBg))
	box = mgr.injectBorderTitle(box, title, popupWidth, accentFg, paneBg)
	return box
}

func (a *annotListOverlay) boxStyle(width int, resolver Resolver) lipgloss.Style {
	return resolver.Style(style.StyleKeyAnnotListBorder).
		Padding(1, 1).
		Width(width)
}

func (a *annotListOverlay) maxVisible(height int) int {
	return max(min(len(a.items), height-6), 1)
}

func (a *annotListOverlay) formatItem(item AnnotationItem, width int, selected bool, resolver Resolver) string {
	var prefix string
	if item.Line == 0 {
		prefix = filepath.Base(item.File) + " (file-level)"
	} else {
		prefix = fmt.Sprintf("%s:%d (%s)", filepath.Base(item.File), item.Line, item.ChangeType)
	}

	prefixWidth := lipgloss.Width(prefix)
	commentSpace := width - prefixWidth - 4 // 2 for cursor prefix, 2 for gap

	var comment string
	if commentSpace > 3 && item.Comment != "" {
		comment = item.Comment
		if lipgloss.Width(comment) > commentSpace {
			comment = ansi.Truncate(comment, commentSpace-3, "...")
		}
	}

	line := prefix
	if comment != "" {
		line = prefix + "  " + comment
	}

	if selected {
		cursor := "> "
		selStyle := resolver.Style(style.StyleKeyFileSelected)
		styled := selStyle.Render(cursor + line)
		w := lipgloss.Width(styled)
		if w < width {
			styled += selStyle.Render(strings.Repeat(" ", width-w))
		}
		return styled
	}

	var styledPrefix string
	switch item.ChangeType {
	case "+":
		styledPrefix = string(resolver.Color(style.ColorKeyAddLineFg)) + prefix + string(style.ResetFg)
	case "-":
		styledPrefix = string(resolver.Color(style.ColorKeyRemoveLineFg)) + prefix + string(style.ResetFg)
	default:
		styledPrefix = string(resolver.Color(style.ColorKeyMutedFg)) + prefix + string(style.ResetFg)
	}

	if comment != "" {
		return "  " + styledPrefix + "  " + comment
	}
	return "  " + styledPrefix
}

func (a *annotListOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionAnnotList {
		return Outcome{Kind: OutcomeClosed}
	}

	switch {
	case msg.Type == tea.KeyEnter:
		if len(a.items) == 0 {
			return Outcome{Kind: OutcomeClosed}
		}
		item := a.items[a.cursor]
		return Outcome{
			Kind:             OutcomeAnnotationChosen,
			AnnotationTarget: &AnnotationTarget{File: item.Target.File, ChangeType: item.Target.ChangeType, Line: item.Target.Line},
		}

	case action == keymap.ActionDismiss || msg.Type == tea.KeyEsc:
		return Outcome{Kind: OutcomeClosed}
	}

	switch action { //nolint:exhaustive // only navigation actions relevant
	case keymap.ActionUp:
		if a.cursor > 0 {
			a.cursor--
			if a.cursor < a.offset {
				a.offset = a.cursor
			}
		}
		return Outcome{Kind: OutcomeNone}

	case keymap.ActionDown:
		if a.cursor < len(a.items)-1 {
			a.cursor++
			maxVis := a.maxVisible(a.height)
			if a.cursor >= a.offset+maxVis {
				a.offset = a.cursor - maxVis + 1
			}
		}
		return Outcome{Kind: OutcomeNone}
	}

	return Outcome{Kind: OutcomeNone}
}
