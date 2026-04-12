package overlay

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/keymap"
)

type helpOverlay struct {
	active bool
	spec   HelpSpec
}

func (h *helpOverlay) open(spec HelpSpec) {
	h.active = true
	h.spec = spec
}

func (h *helpOverlay) render(_ RenderCtx, _ *Manager) string {
	return "" // implemented in task 2
}

func (h *helpOverlay) handleKey(_ tea.KeyMsg, _ keymap.Action) Outcome {
	return Outcome{} // implemented in task 2
}
