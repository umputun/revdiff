package overlay

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

const (
	filePickerMaxWidth    = 80
	filePickerMinWidth    = 24
	filePickerMargin      = 10
	filePickerBorderPad   = 4
	filePickerChromeLines = 10
)

type filePickerOverlay struct {
	all        []string
	entries    []string
	cursor     int
	offset     int
	filter     string
	height     int
	popupWidth int
}

func (f *filePickerOverlay) open(spec FilePickerSpec) {
	f.all = slices.Clone(spec.Paths)
	f.filter = ""
	f.entries = slices.Clone(f.all)
	f.cursor = 0
	f.offset = 0
	for i, path := range f.entries {
		if path == spec.ActivePath {
			f.cursor = i
			break
		}
	}
}

func (f *filePickerOverlay) applyFilter() {
	if f.filter == "" {
		f.entries = slices.Clone(f.all)
	} else {
		needle := strings.ToLower(f.filter)
		f.entries = f.entries[:0]
		for _, path := range f.all {
			if strings.Contains(strings.ToLower(path), needle) {
				f.entries = append(f.entries, path)
			}
		}
	}
	f.cursor = 0
	f.offset = 0
}

func (f *filePickerOverlay) render(ctx RenderCtx, mgr *Manager) string {
	f.height = ctx.Height
	f.popupWidth = max(min(ctx.Width-filePickerMargin, filePickerMaxWidth), filePickerMinWidth)
	maxVisible := f.maxVisible()

	if len(f.entries) == 0 {
		f.cursor = 0
		f.offset = 0
	} else {
		f.cursor = min(max(f.cursor, 0), len(f.entries)-1)
		f.offset = min(f.offset, max(len(f.entries)-maxVisible, 0))
		if f.cursor >= f.offset+maxVisible {
			f.offset = f.cursor - maxVisible + 1
		}
		if f.cursor < f.offset {
			f.offset = f.cursor
		}
	}

	contentWidth := f.popupWidth - filePickerBorderPad
	parts := []string{f.renderFilter(ctx.Resolver), ""}
	if len(f.entries) == 0 {
		muted := ctx.Resolver.Color(style.ColorKeyMutedFg)
		parts = append(parts, string(muted)+"  no matches"+string(style.ResetFg))
	} else {
		end := min(len(f.entries), f.offset+maxVisible)
		for i := f.offset; i < end; i++ {
			parts = append(parts, f.formatEntry(f.entries[i], contentWidth, i == f.cursor, ctx.Resolver))
		}
	}

	title := fmt.Sprintf(" files (%d) ", len(f.all))
	if f.filter != "" {
		title = fmt.Sprintf(" files (%d/%d) ", len(f.entries), len(f.all))
	}
	box := ctx.Resolver.Style(style.StyleKeyThemeSelectBox).Width(f.popupWidth).Render(strings.Join(parts, "\n"))
	box = mgr.injectBorderTitle(box, title, borderEdgeText{
		popupWidth: f.popupWidth,
		accentFg:   string(ctx.Resolver.Color(style.ColorKeyAccentFg)),
		paneBg:     string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg)),
	})
	return box
}

func (f *filePickerOverlay) renderFilter(resolver Resolver) string {
	if f.filter == "" {
		return "  " + string(resolver.Color(style.ColorKeyMutedFg)) + "type to filter..." + string(style.ResetFg)
	}
	return "  " + f.filter + string(resolver.Color(style.ColorKeyAccentFg)) + "│" + string(style.ResetFg)
}

func (f *filePickerOverlay) formatEntry(path string, width int, selected bool, resolver Resolver) string {
	clean := (style.Resolver{}).SanitizeFilenameForDisplay(path)
	display := f.truncateFilePath(clean, width-2)
	if selected {
		entryStyle := resolver.Style(style.StyleKeyFileSelected)
		styled := entryStyle.Render("> " + display)
		if w := lipgloss.Width(styled); w < width {
			styled += entryStyle.Render(strings.Repeat(" ", width-w))
		}
		return styled
	}
	normal := string(resolver.Color(style.ColorKeyNormalFg))
	if normal == "" {
		return "  " + display
	}
	return "  " + normal + display + string(style.ResetFg)
}

// truncateFilePath trims from the directory side so the basename remains
// visible. If the basename alone is too wide, its rightmost cells are kept.
func (f *filePickerOverlay) truncateFilePath(path string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(path) <= width {
		return path
	}
	base := filepath.Base(path)
	candidate := "…/" + base
	if runewidth.StringWidth(candidate) <= width {
		return candidate
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(base)
	used := 0
	start := len(runes)
	for i, r := range slices.Backward(runes) {
		runeWidth := runewidth.RuneWidth(r)
		if used+runeWidth > width-1 {
			break
		}
		used += runeWidth
		start = i
	}
	return "…" + string(runes[start:])
}

func (f *filePickerOverlay) maxVisible() int {
	return max(min(len(f.entries), f.height-filePickerChromeLines), 1)
}

func (f *filePickerOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	if f.appendPrintableRunes(msg) {
		return Outcome{Kind: OutcomeNone}
	}
	if action == keymap.ActionJumpFile {
		return Outcome{Kind: OutcomeClosed}
	}
	if action == keymap.ActionUp {
		f.moveCursorBy(-1)
		return Outcome{Kind: OutcomeNone}
	}
	if action == keymap.ActionDown {
		f.moveCursorBy(1)
		return Outcome{Kind: OutcomeNone}
	}

	switch msg.Type {
	case tea.KeyEnter:
		return f.chooseCurrent()
	case tea.KeyEsc:
		if f.filter == "" {
			return Outcome{Kind: OutcomeClosed}
		}
		f.filter = ""
		f.applyFilter()
		return Outcome{Kind: OutcomeNone}
	case tea.KeyBackspace:
		if f.filter != "" {
			runes := []rune(f.filter)
			f.filter = string(runes[:len(runes)-1])
			f.applyFilter()
		}
		return Outcome{Kind: OutcomeNone}
	default:
		return Outcome{Kind: OutcomeNone}
	}
}

// appendPrintableRunes gives unmodified text input priority over configured
// single-rune actions such as the default j/k navigation bindings. Modified
// runes and non-printable keys remain available for configured actions.
func (f *filePickerOverlay) appendPrintableRunes(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || msg.Alt || len(msg.Runes) == 0 {
		return false
	}
	for _, r := range msg.Runes {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	f.filter += string(msg.Runes)
	f.applyFilter()
	return true
}

func (f *filePickerOverlay) chooseCurrent() Outcome {
	if len(f.entries) == 0 || f.cursor < 0 || f.cursor >= len(f.entries) {
		return Outcome{Kind: OutcomeNone}
	}
	return Outcome{Kind: OutcomeFileChosen, FileChoice: &FileChoice{Path: f.entries[f.cursor]}}
}

func (f *filePickerOverlay) moveCursorBy(delta int) {
	if len(f.entries) == 0 {
		return
	}
	target := min(max(f.cursor+delta, 0), len(f.entries)-1)
	if target == f.cursor {
		return
	}
	f.cursor = target
	if f.cursor < f.offset {
		f.offset = f.cursor
	}
	if maxVisible := f.maxVisible(); f.cursor >= f.offset+maxVisible {
		f.offset = f.cursor - maxVisible + 1
	}
}

func (f *filePickerOverlay) handleMouse(msg tea.MouseMsg) Outcome {
	if msg.Action != tea.MouseActionPress {
		return Outcome{Kind: OutcomeNone}
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		f.moveCursorBy(f.wheelStep(msg.Shift))
	case tea.MouseButtonWheelUp:
		f.moveCursorBy(-f.wheelStep(msg.Shift))
	case tea.MouseButtonLeft:
		return f.handleLeftClick(msg.X, msg.Y)
	default:
		// Other mouse buttons and horizontal wheels are intentionally ignored.
	}
	return Outcome{Kind: OutcomeNone}
}

func (f *filePickerOverlay) wheelStep(shift bool) int {
	if !shift {
		return 1
	}
	return max(f.maxVisible()/2, 1)
}

func (f *filePickerOverlay) handleLeftClick(localX, localY int) Outcome {
	const entriesTop = 4
	const horizontalChrome = 2
	if localX < horizontalChrome || localX >= f.popupWidth-horizontalChrome {
		return Outcome{Kind: OutcomeNone}
	}
	relativeRow := localY - entriesTop
	if relativeRow < 0 || relativeRow >= f.maxVisible() {
		return Outcome{Kind: OutcomeNone}
	}
	entryIdx := f.offset + relativeRow
	if entryIdx < 0 || entryIdx >= len(f.entries) {
		return Outcome{Kind: OutcomeNone}
	}
	f.cursor = entryIdx
	return f.chooseCurrent()
}
