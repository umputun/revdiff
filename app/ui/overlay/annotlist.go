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

const (
	annotPopupMaxWidth  = 140 // maximum popup width
	annotPopupMinWidth  = 20  // minimum popup width
	annotPopupMargin    = 10  // horizontal margin from terminal edges
	annotPopupBorderPad = 4   // border (2) + padding (2) for content width
)

type annotListOverlay struct {
	items      []AnnotationItem
	cursor     int
	offset     int
	height     int // last known terminal height, updated on render
	popupWidth int // last known popup width, updated on render; used by handleLeftClick
}

func (a *annotListOverlay) open(spec AnnotListSpec) {
	a.items = spec.Items
	a.cursor = 0
	a.offset = 0
}

func (a *annotListOverlay) render(ctx RenderCtx, mgr *Manager) string {
	a.height = ctx.Height
	popupWidth := max(min(ctx.Width-annotPopupMargin, annotPopupMaxWidth), annotPopupMinWidth)
	a.popupWidth = popupWidth

	if len(a.items) == 0 {
		return a.emptyOverlay(popupWidth, ctx.Resolver, mgr)
	}

	maxVisibleItems := a.maxVisible(ctx.Height)
	contentWidth := popupWidth - annotPopupBorderPad

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
	box = mgr.injectBorderTitle(box, title, borderEdgeText{popupWidth: popupWidth, accentFg: accentFg, paneBg: paneBg})

	return box
}

func (a *annotListOverlay) emptyOverlay(popupWidth int, resolver Resolver, mgr *Manager) string {
	text := "no annotations"
	innerWidth := popupWidth - annotPopupBorderPad
	pad := max((innerWidth-lipgloss.Width(text))/2, 0)
	centered := strings.Repeat(" ", pad) + text

	box := a.boxStyle(popupWidth, resolver).Render(centered)
	title := " annotations (0) "
	accentFg := string(resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(resolver.Color(style.ColorKeyDiffPaneBg))
	box = mgr.injectBorderTitle(box, title, borderEdgeText{popupWidth: popupWidth, accentFg: accentFg, paneBg: paneBg})
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
		// flatten newlines so multi-line comments render as a single row
		comment = strings.ReplaceAll(item.Comment, "\n", " ⏎ ")
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
		target := item.AnnotationTarget
		return Outcome{
			Kind:             OutcomeAnnotationChosen,
			AnnotationTarget: &target,
		}

	case action == keymap.ActionDismiss || msg.Type == tea.KeyEsc:
		return Outcome{Kind: OutcomeClosed}
	}

	switch action {
	case keymap.ActionUp:
		a.moveCursorBy(-1)
		return Outcome{Kind: OutcomeNone}

	case keymap.ActionDown:
		a.moveCursorBy(1)
		return Outcome{Kind: OutcomeNone}

	default:
		return Outcome{Kind: OutcomeNone}
	}
}

// moveCursorBy shifts the cursor by delta (positive = down, negative = up),
// clamped to [0, len(items)-1]. offset follows the cursor using the same
// "scroll by one when cursor leaves the visible window" policy as keyboard
// navigation.
func (a *annotListOverlay) moveCursorBy(delta int) {
	if len(a.items) == 0 {
		return
	}
	target := min(max(a.cursor+delta, 0), len(a.items)-1)
	a.cursor = target
	if a.cursor < a.offset {
		a.offset = a.cursor
	}
	maxVis := a.maxVisible(a.height)
	if a.cursor >= a.offset+maxVis {
		a.offset = a.cursor - maxVis + 1
	}
}

// handleMouse drives the overlay in response to mouse events. plain wheel steps
// the cursor by one entry (single-notch feel); shift+wheel steps by half the
// visible page. left-click inside the popup maps the clicked row to an item
// and returns OutcomeAnnotationChosen for the main model to jump to (same as
// pressing Enter). coords are popup-local (Manager.HandleMouse translates).
// non-press actions and other buttons are ignored so the overlay is not
// dismissed by accidental drag or release events.
func (a *annotListOverlay) handleMouse(msg tea.MouseMsg) Outcome {
	if msg.Action != tea.MouseActionPress {
		return Outcome{Kind: OutcomeNone}
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		a.moveCursorBy(a.wheelStep(msg.Shift))
		return Outcome{Kind: OutcomeNone}
	case tea.MouseButtonWheelUp:
		a.moveCursorBy(-a.wheelStep(msg.Shift))
		return Outcome{Kind: OutcomeNone}
	case tea.MouseButtonLeft:
		return a.handleLeftClick(msg.X, msg.Y)
	default:
		return Outcome{Kind: OutcomeNone}
	}
}

// wheelStep returns the cursor step for a wheel notch: 1 by default,
// half the visible page when shift is held.
func (a *annotListOverlay) wheelStep(shift bool) int {
	if !shift {
		return 1
	}
	return max(a.maxVisible(a.height)/2, 1)
}

// handleLeftClick maps popup-local (x, y) to an item row and returns a jump
// outcome. the popup has a 1-row border + 1-row top padding, and 1-col border
// + 1-col left/right padding — so the content rectangle is y in [2, h-2) and
// x in [2, popupWidth-2). clicks outside that rectangle (including the
// vertical borders and horizontal padding on an item row) are no-ops so
// users cannot accidentally select by clicking chrome.
func (a *annotListOverlay) handleLeftClick(localX, localY int) Outcome {
	const contentTop = 2      // border (1) + top padding (1)
	const horizChromeCols = 2 // border (1) + side padding (1) on each side
	if localX < horizChromeCols || localX >= a.popupWidth-horizChromeCols {
		return Outcome{Kind: OutcomeNone}
	}
	relRow := localY - contentTop
	maxVis := a.maxVisible(a.height)
	if relRow < 0 || relRow >= maxVis {
		return Outcome{Kind: OutcomeNone}
	}
	itemIdx := a.offset + relRow
	if itemIdx < 0 || itemIdx >= len(a.items) {
		return Outcome{Kind: OutcomeNone}
	}
	a.cursor = itemIdx
	target := a.items[itemIdx].AnnotationTarget
	return Outcome{Kind: OutcomeAnnotationChosen, AnnotationTarget: &target}
}
