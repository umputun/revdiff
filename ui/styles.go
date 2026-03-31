package ui

import "github.com/charmbracelet/lipgloss"

// styles holds all lipgloss styles used in the TUI.
type styles struct {
	// file tree pane
	TreePane       lipgloss.Style
	TreePaneActive lipgloss.Style
	DirEntry       lipgloss.Style
	FileEntry      lipgloss.Style
	FileSelected   lipgloss.Style
	AnnotationMark lipgloss.Style

	// diff pane
	DiffPane       lipgloss.Style
	DiffPaneActive lipgloss.Style
	LineAdd        lipgloss.Style
	LineRemove     lipgloss.Style
	LineContext    lipgloss.Style
	LineNumber     lipgloss.Style

	// status bar
	StatusBar lipgloss.Style

	// diff cursor
	DiffCursorLine lipgloss.Style

	// annotation
	AnnotationLine lipgloss.Style
}

func defaultStyles() styles {
	border := lipgloss.NormalBorder()

	return styles{
		TreePane: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color("240")),
		TreePaneActive: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color("69")),
		DirEntry: lipgloss.NewStyle().
			Foreground(lipgloss.Color("69")).
			Bold(true),
		FileEntry: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
		FileSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("236")),
		AnnotationMark: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")),

		DiffPane: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color("240")),
		DiffPaneActive: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color("69")),
		LineAdd: lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("114")),
		LineRemove: lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("210")),
		LineContext: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
		LineNumber: lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Width(6).
			Align(lipgloss.Right),

		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Padding(0, 1),

		DiffCursorLine: lipgloss.NewStyle().
			Background(lipgloss.Color("237")),

		AnnotationLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Italic(true),
	}
}
