package ui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines all keybindings for the TUI.
type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Tab        key.Binding
	NextFile   key.Binding
	PrevFile   key.Binding
	Annotate   key.Binding
	Delete     key.Binding
	Cancel     key.Binding
	Quit       key.Binding
	ScrollUp   key.Binding
	ScrollDown key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/↓", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "toggle filter"),
		),
		NextFile: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next file"),
		),
		PrevFile: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "prev file"),
		),
		Annotate: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "annotate"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete annotation"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/↑", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/↓", "scroll down"),
		),
	}
}
