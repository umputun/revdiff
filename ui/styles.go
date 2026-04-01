package ui

import "github.com/charmbracelet/lipgloss"

// Colors holds hex color values (#rrggbb) for TUI rendering.
type Colors struct {
	Accent     string // active pane borders, dir names
	Border     string // inactive pane borders
	Normal     string // file entries, context lines
	Muted      string // line numbers, status bar
	SelectedFg string // selected file text
	SelectedBg string // selected file background
	Annotation string // annotation text and markers
	CursorBg   string // diff cursor line background
	AddFg      string // added line foreground
	AddBg      string // added line background
	RemoveFg   string // removed line foreground
	RemoveBg   string // removed line background
}

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

func newStyles(c Colors) styles {
	border := lipgloss.NormalBorder()

	return styles{
		TreePane: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color(c.Border)),
		TreePaneActive: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color(c.Accent)),
		DirEntry: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Accent)).
			Bold(true),
		FileEntry: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Normal)),
		FileSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.SelectedFg)).
			Background(lipgloss.Color(c.SelectedBg)),
		AnnotationMark: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Annotation)),

		DiffPane: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color(c.Border)),
		DiffPaneActive: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color(c.Accent)),
		LineAdd: lipgloss.NewStyle().
			Background(lipgloss.Color(c.AddBg)).
			Foreground(lipgloss.Color(c.AddFg)),
		LineRemove: lipgloss.NewStyle().
			Background(lipgloss.Color(c.RemoveBg)).
			Foreground(lipgloss.Color(c.RemoveFg)),
		LineContext: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Normal)),
		LineNumber: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Muted)).
			Width(6).
			Align(lipgloss.Right),

		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Muted)).
			Padding(0, 1),

		DiffCursorLine: lipgloss.NewStyle().
			Background(lipgloss.Color(c.CursorBg)),

		AnnotationLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Annotation)).
			Italic(true),
	}
}
