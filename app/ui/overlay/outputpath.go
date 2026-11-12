package overlay

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/ui/style"
)

const (
	outputPathMaxWidth  = 80
	outputPathMinWidth  = 28
	outputPathMargin    = 10
	outputPathBorderPad = 4
	outputPathCharLimit = 4096
)

type outputPathOverlay struct {
	input            textinput.Model
	confirmOverwrite bool
	errText          string
	popupWidth       int
}

func (o *outputPathOverlay) open(spec OutputPathSpec) {
	ti := textinput.New()
	ti.Cursor.SetMode(cursor.CursorStatic)
	_ = ti.Focus()
	ti.CharLimit = outputPathCharLimit
	ti.Prompt = "Path: "
	ti.SetValue(spec.Path)
	ti.CursorEnd()

	o.input = ti
	o.confirmOverwrite = spec.ConfirmOverwrite
	o.errText = spec.Error
}

func (o *outputPathOverlay) render(ctx RenderCtx, mgr *Manager) string {
	o.popupWidth = max(min(ctx.Width-outputPathMargin, outputPathMaxWidth), outputPathMinWidth)
	contentWidth := max(o.popupWidth-outputPathBorderPad, 1)

	var content string
	if o.confirmOverwrite {
		path := style.SanitizeFilenameForDisplay(o.input.Value())
		path = style.TruncateLeftToWidth(path, contentWidth)
		content = fmt.Sprintf("Overwrite %s?\n\n[y] overwrite  [n] edit  [esc] cancel", path)
	} else {
		o.input.Width = max(contentWidth-len(o.input.Prompt), 1)
		o.input.PromptStyle = ctx.Resolver.Style(style.StyleKeyAnnotInputText)
		o.input.TextStyle = ctx.Resolver.Style(style.StyleKeyAnnotInputText)
		o.input.Cursor.Style = ctx.Resolver.Style(style.StyleKeyAnnotInputCursor)
		o.input.Cursor.TextStyle = ctx.Resolver.Style(style.StyleKeyAnnotInputCursor)
		o.input.PlaceholderStyle = ctx.Resolver.Style(style.StyleKeyAnnotInputPlaceholder)
		parts := []string{o.input.View()}
		if o.errText != "" {
			errText := style.SanitizeFilenameForDisplay(o.errText)
			parts = append(parts, "", style.TruncateLeftToWidth(errText, contentWidth))
		}
		parts = append(parts, "", "[enter] save  [esc] cancel")
		content = strings.Join(parts, "\n")
	}

	box := ctx.Resolver.Style(style.StyleKeyFilePickerBox).Width(o.popupWidth).Render(content)
	return mgr.injectBorderTitle(box, " Save Annotations ", borderEdgeText{
		popupWidth: o.popupWidth,
		accentFg:   string(ctx.Resolver.Color(style.ColorKeyAccentFg)),
		paneBg:     string(ctx.Resolver.Color(style.ColorKeyDiffPaneBg)),
	})
}

func (o *outputPathOverlay) handleKey(msg tea.KeyMsg) Outcome {
	if o.confirmOverwrite {
		switch msg.String() {
		case "y", "Y":
			return Outcome{Kind: OutcomeOutputPathSubmitted, OutputPathChoice: &OutputPathChoice{
				Path:      o.input.Value(),
				Overwrite: true,
			}}
		case "n", "N":
			o.confirmOverwrite = false
			o.errText = ""
			return Outcome{Kind: OutcomeNone}
		case "esc":
			return Outcome{Kind: OutcomeClosed}
		default:
			return Outcome{Kind: OutcomeNone}
		}
	}

	switch msg.Type {
	case tea.KeyEnter:
		if strings.TrimSpace(o.input.Value()) == "" {
			o.errText = "Output path is required"
			return Outcome{Kind: OutcomeNone}
		}
		return Outcome{Kind: OutcomeOutputPathSubmitted, OutputPathChoice: &OutputPathChoice{Path: o.input.Value()}}
	case tea.KeyEsc:
		return Outcome{Kind: OutcomeClosed}
	default:
		o.errText = ""
		var cmd tea.Cmd
		o.input, cmd = o.input.Update(msg)
		_ = cmd // static cursor: key handling updates synchronously; no follow-up command is needed
		return Outcome{Kind: OutcomeNone}
	}
}
