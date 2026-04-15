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

const (
	themePopupMaxWidth    = 50 // maximum popup width
	themePopupMinWidth    = 20 // minimum popup width
	themePopupMargin      = 10 // horizontal margin from terminal edges
	themePopupBorderPad   = 4  // border (2) + padding (2) for content width
	themePopupChromeLines = 10 // border + padding + filter + separator lines
)

type themeSelectOverlay struct {
	all               []ThemeItem
	entries           []ThemeItem
	cursor            int
	offset            int
	filter            string
	lastPreviewedName string
	height            int // last known terminal height, updated on render
}

func (t *themeSelectOverlay) open(spec ThemeSelectSpec) {
	t.all = spec.Items
	t.filter = ""
	t.lastPreviewedName = ""
	t.applyFilter()

	for i, item := range t.entries {
		if item.Name == spec.ActiveName {
			t.cursor = i
			maxVis := t.maxVisible()
			if i >= maxVis {
				t.offset = i - maxVis + 1
			}
			break
		}
	}
}

func (t *themeSelectOverlay) applyFilter() {
	if t.filter == "" {
		t.entries = t.all
		t.cursor = 0
		t.offset = 0
		return
	}
	lower := strings.ToLower(t.filter)
	filtered := make([]ThemeItem, 0, len(t.all))
	for _, item := range t.all {
		if strings.Contains(strings.ToLower(item.Name), lower) {
			filtered = append(filtered, item)
		}
	}
	t.entries = filtered
	t.cursor = 0
	t.offset = 0
}

func (t *themeSelectOverlay) render(ctx RenderCtx, mgr *Manager) string {
	t.height = ctx.Height
	popupWidth := max(min(ctx.Width-themePopupMargin, themePopupMaxWidth), themePopupMinWidth)
	maxVisible := t.maxVisible()

	// clamp offset after height refresh so cursor stays visible on terminal resize
	if maxOffset := max(len(t.entries)-maxVisible, 0); t.offset > maxOffset {
		t.offset = maxOffset
	}
	if t.cursor >= t.offset+maxVisible {
		t.offset = t.cursor - maxVisible + 1
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}

	contentWidth := popupWidth - themePopupBorderPad

	var parts []string

	filterLine := t.renderFilter(ctx.Resolver)
	parts = append(parts, filterLine, "")

	if len(t.entries) == 0 {
		muted := ctx.Resolver.Color(style.ColorKeyMutedFg)
		parts = append(parts, string(muted)+"  no matches"+string(style.ResetFg))
	} else {
		for i := t.offset; i < len(t.entries) && i < t.offset+maxVisible; i++ {
			line := t.formatEntry(t.entries[i], contentWidth, i == t.cursor, ctx.Resolver)
			parts = append(parts, line)
		}
	}

	content := strings.Join(parts, "\n")

	total := len(t.all)
	showing := len(t.entries)
	title := fmt.Sprintf(" themes (%d) ", total)
	if t.filter != "" {
		title = fmt.Sprintf(" themes (%d/%d) ", showing, total)
	}

	boxStyle := ctx.Resolver.Style(style.StyleKeyThemeSelectBox).Width(popupWidth)
	box := boxStyle.Render(content)

	accentFg := string(ctx.Resolver.Color(style.ColorKeyAccentFg))
	paneBg := string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg))
	box = mgr.injectBorderTitle(box, title, popupWidth, accentFg, paneBg)

	return box
}

func (t *themeSelectOverlay) renderFilter(resolver Resolver) string {
	accent := resolver.Color(style.ColorKeyAccentFg)
	muted := resolver.Color(style.ColorKeyMutedFg)

	if t.filter == "" {
		return "  " + string(muted) + "type to filter..." + string(style.ResetFg)
	}
	return "  " + t.filter + string(accent) + "│" + string(style.ResetFg)
}

func (t *themeSelectOverlay) formatEntry(item ThemeItem, width int, selected bool, resolver Resolver) string {
	var swatch string
	var resetAfterSwatch string
	if selected {
		resetAfterSwatch = string(resolver.Color(style.ColorKeySelectedFg))
	}
	switch {
	case item.Local:
		swatch = swatchText(string(resolver.Color(style.ColorKeyMutedFg)), "◇", resetAfterSwatch)
	case item.AccentColor != "":
		swatch = swatchText(style.AnsiFg(item.AccentColor), "■", resetAfterSwatch)
	default:
		swatch = "■"
	}

	nameMaxWidth := width - 6
	name := item.Name
	if runewidth.StringWidth(name) > nameMaxWidth {
		name = runewidth.Truncate(name, nameMaxWidth, "…")
	}

	fileSelected := resolver.Style(style.StyleKeyFileSelected)
	if selected {
		line := "> " + swatch + " " + name
		styled := fileSelected.Render(line)
		w := lipgloss.Width(styled)
		if w < width {
			styled += fileSelected.Render(strings.Repeat(" ", width-w))
		}
		return styled
	}

	return "  " + swatch + " " + name
}

func (t *themeSelectOverlay) maxVisible() int {
	available := t.height - themePopupChromeLines
	return max(min(len(t.entries), available), 1)
}

func (t *themeSelectOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if action == keymap.ActionThemeSelect {
		return Outcome{Kind: OutcomeThemeCanceled}
	}

	switch msg.Type {
	case tea.KeyEnter:
		if len(t.entries) == 0 {
			t.filter = ""
			return Outcome{Kind: OutcomeThemeCanceled}
		}
		return Outcome{Kind: OutcomeThemeConfirmed, ThemeChoice: &ThemeChoice{Name: t.entries[t.cursor].Name}}

	case tea.KeyEsc:
		if t.filter != "" {
			t.filter = ""
			t.applyFilter()
			return t.previewOutcome()
		}
		return Outcome{Kind: OutcomeThemeCanceled}

	case tea.KeyBackspace:
		if t.filter != "" {
			runes := []rune(t.filter)
			t.filter = string(runes[:len(runes)-1])
			t.applyFilter()
			return t.previewOutcome()
		}
		return Outcome{Kind: OutcomeNone}

	case tea.KeyUp:
		if t.cursor > 0 {
			t.cursor--
			if t.cursor < t.offset {
				t.offset = t.cursor
			}
			return t.previewOutcome()
		}
		return Outcome{Kind: OutcomeNone}

	case tea.KeyDown:
		if t.cursor < len(t.entries)-1 {
			t.cursor++
			maxVis := t.maxVisible()
			if t.cursor >= t.offset+maxVis {
				t.offset = t.cursor - maxVis + 1
			}
			return t.previewOutcome()
		}
		return Outcome{Kind: OutcomeNone}

	case tea.KeyRunes:
		t.filter += string(msg.Runes)
		t.applyFilter()
		return t.previewOutcome()

	default:
		return Outcome{Kind: OutcomeNone}
	}
}

// previewOutcome returns a theme preview outcome with dedup — if the current
// entry name matches lastPreviewedName, returns OutcomeNone instead.
func (t *themeSelectOverlay) previewOutcome() Outcome {
	if len(t.entries) == 0 {
		t.lastPreviewedName = ""
		return Outcome{Kind: OutcomeNone}
	}
	name := t.entries[t.cursor].Name
	if name == t.lastPreviewedName {
		return Outcome{Kind: OutcomeNone}
	}
	t.lastPreviewedName = name
	return Outcome{Kind: OutcomeThemePreview, ThemeChoice: &ThemeChoice{Name: name}}
}

// swatchText renders text with the given ANSI fg sequence and resets afterward.
func swatchText(fg, text, resetAfter string) string {
	if fg == "" {
		return text
	}
	reset := string(style.ResetFg)
	if resetAfter != "" {
		reset = resetAfter
	}
	return fg + text + reset
}
