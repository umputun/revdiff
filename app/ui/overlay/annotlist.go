package overlay

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/keymap"
)

type annotListOverlay struct {
	active bool
	items  []AnnotationItem
	cursor int
	offset int
}

func (a *annotListOverlay) open(spec AnnotListSpec) {
	a.active = true
	a.items = spec.Items
	a.cursor = 0
	a.offset = 0
}

func (a *annotListOverlay) render(_ RenderCtx, _ *Manager) string {
	return "" // implemented in task 3
}

func (a *annotListOverlay) handleKey(_ tea.KeyMsg, _ keymap.Action) Outcome {
	return Outcome{} // implemented in task 3
}
