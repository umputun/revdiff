package overlay

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/keymap"
)

type themeSelectOverlay struct {
	active            bool
	all               []ThemeItem
	entries           []ThemeItem
	cursor            int
	offset            int
	filter            string
	lastPreviewedName string
}

func (t *themeSelectOverlay) open(spec ThemeSelectSpec) {
	t.active = true
	t.all = spec.Items
	t.entries = spec.Items
	t.cursor = 0
	t.offset = 0
	t.filter = ""
	t.lastPreviewedName = ""

	for i, item := range t.entries {
		if item.Name == spec.ActiveName {
			t.cursor = i
			break
		}
	}
}

func (t *themeSelectOverlay) render(_ RenderCtx, _ *Manager) string {
	return "" // implemented in task 4
}

func (t *themeSelectOverlay) handleKey(_ tea.KeyMsg, _ keymap.Action) Outcome {
	return Outcome{} // implemented in task 4
}
