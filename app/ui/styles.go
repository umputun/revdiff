package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/diff"
)

// Colors holds hex color values (#rrggbb) for TUI rendering.
type Colors struct {
	Accent     string // active pane borders, dir names
	Border     string // inactive pane borders
	Normal     string // file entries, context lines
	Muted      string // divider lines, status bar
	SelectedFg string // selected file text
	SelectedBg string // selected file background
	Annotation string // annotation text and markers
	CursorFg   string // diff cursor indicator foreground
	CursorBg   string // diff cursor line background
	AddFg      string // added line foreground
	AddBg      string // added line background
	RemoveFg   string // removed line foreground
	RemoveBg   string // removed line background
	ModifyFg   string // modified line foreground (collapsed mode)
	ModifyBg   string // modified line background (collapsed mode)
	TreeBg     string // file tree pane background
	DiffBg     string // diff pane background
	StatusFg   string // status bar foreground
	StatusBg   string // status bar background
	SearchFg   string // search match foreground
	SearchBg   string // search match background
}

// styles holds all lipgloss styles used in the TUI.
type styles struct {
	// file tree pane
	TreePane        lipgloss.Style
	TreePaneActive  lipgloss.Style
	DirEntry        lipgloss.Style
	FileEntry       lipgloss.Style
	FileSelected    lipgloss.Style
	AnnotationMark  lipgloss.Style
	ReviewedMark    lipgloss.Style
	StatusAdded     lipgloss.Style
	StatusDeleted   lipgloss.Style
	StatusUntracked lipgloss.Style
	StatusDefault   lipgloss.Style // M, R, and other statuses

	// diff pane
	DiffPane       lipgloss.Style
	DiffPaneActive lipgloss.Style
	LineAdd        lipgloss.Style
	LineRemove     lipgloss.Style
	LineContext    lipgloss.Style
	LineNumber     lipgloss.Style

	// status bar
	StatusBar lipgloss.Style

	// syntax-highlighted lines (background only, chroma owns foreground)
	LineAddHighlight     lipgloss.Style
	LineRemoveHighlight  lipgloss.Style
	LineContextHighlight lipgloss.Style // context line with syntax highlighting (DiffBg only)
	LineModify           lipgloss.Style // modified line (collapsed mode, non-highlighted)
	LineModifyHighlight  lipgloss.Style // modified line (collapsed mode, syntax-highlighted)

	// diff cursor
	DiffCursorLine lipgloss.Style
	// annotation
	AnnotationLine lipgloss.Style
	// search
	SearchMatch lipgloss.Style

	colors Colors // original color values for dynamic style construction
}

// fileStatusFg returns the foreground hex color for a git file status letter.
// Returns a raw hex string (not a lipgloss.Style) because status marks are rendered
// inside a lipgloss parent container — using lipgloss.Render() would emit \033[0m
// (full reset) and break the outer pane background.
func (s styles) fileStatusFg(status diff.FileStatus) string {
	switch status {
	case diff.FileAdded, diff.FileUntracked:
		return s.colors.AddFg
	case diff.FileDeleted:
		return s.colors.RemoveFg
	default:
		return s.colors.Muted
	}
}

// coloredText wraps text in ANSI fg-only escape sequences (no \033[0m reset).
// safe to use inside a lipgloss-rendered parent without breaking outer backgrounds.
func coloredText(hex, text string) string {
	return coloredTextWithReset(hex, text, "")
}

// coloredTextWithReset wraps text in ANSI fg-only escape sequences and restores
// the provided foreground color instead of resetting to the terminal default.
func coloredTextWithReset(hex, text, resetHex string) string {
	if hex == "" {
		return text
	}
	reset := "\033[39m"
	if resetHex != "" {
		reset = fmt.Sprintf("\033[38;2;%sm", hexColorToRGB(resetHex))
	}
	return fmt.Sprintf("\033[38;2;%sm%s%s", hexColorToRGB(hex), text, reset)
}

// hexColorToRGB converts "#RRGGBB" to "R;G;B" for ANSI escape sequences.
func hexColorToRGB(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return "255;255;255"
	}
	r := hexVal(hex[1])<<4 | hexVal(hex[2])
	g := hexVal(hex[3])<<4 | hexVal(hex[4])
	b := hexVal(hex[5])<<4 | hexVal(hex[6])
	return fmt.Sprintf("%d;%d;%d", r, g, b)
}

// hexVal converts a single hex character to its value.
func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

// normalizeColor ensures hex color values have a # prefix.
// returns empty string unchanged (used for optional colors).
func normalizeColor(s string) string {
	if s == "" || s[0] == '#' {
		return s
	}
	return "#" + s
}

// normalizeColors ensures all color values have # prefix where needed.
func normalizeColors(c Colors) Colors {
	c.Accent = normalizeColor(c.Accent)
	c.Border = normalizeColor(c.Border)
	c.Normal = normalizeColor(c.Normal)
	c.Muted = normalizeColor(c.Muted)
	c.SelectedFg = normalizeColor(c.SelectedFg)
	c.SelectedBg = normalizeColor(c.SelectedBg)
	c.Annotation = normalizeColor(c.Annotation)
	c.CursorFg = normalizeColor(c.CursorFg)
	c.CursorBg = normalizeColor(c.CursorBg)
	c.AddFg = normalizeColor(c.AddFg)
	c.AddBg = normalizeColor(c.AddBg)
	c.RemoveFg = normalizeColor(c.RemoveFg)
	c.RemoveBg = normalizeColor(c.RemoveBg)
	c.ModifyFg = normalizeColor(c.ModifyFg)
	c.ModifyBg = normalizeColor(c.ModifyBg)
	c.TreeBg = normalizeColor(c.TreeBg)
	c.DiffBg = normalizeColor(c.DiffBg)
	c.StatusFg = normalizeColor(c.StatusFg)
	c.StatusBg = normalizeColor(c.StatusBg)
	c.SearchFg = normalizeColor(c.SearchFg)
	c.SearchBg = normalizeColor(c.SearchBg)
	return c
}

func newStyles(c Colors) styles {
	c = normalizeColors(c)
	border := lipgloss.NormalBorder()

	treePane := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Border))
	treePaneActive := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Accent))
	if c.TreeBg != "" {
		treeBg := lipgloss.Color(c.TreeBg)
		treePane = treePane.Background(treeBg).BorderBackground(treeBg)
		treePaneActive = treePaneActive.Background(treeBg).BorderBackground(treeBg)
	}

	diffPane := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Border))
	diffPaneActive := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Accent))
	if c.DiffBg != "" {
		diffBg := lipgloss.Color(c.DiffBg)
		diffPane = diffPane.Background(diffBg).BorderBackground(diffBg)
		diffPaneActive = diffPaneActive.Background(diffBg).BorderBackground(diffBg)
	}

	statusFg := c.Muted
	if c.StatusFg != "" {
		statusFg = c.StatusFg
	}
	statusBar := lipgloss.NewStyle().
		Foreground(lipgloss.Color(statusFg)).
		Padding(0, 1)
	if c.StatusBg != "" {
		statusBar = statusBar.Background(lipgloss.Color(c.StatusBg))
	}

	return styles{
		TreePane:       treePane,
		TreePaneActive: treePaneActive,
		DirEntry:       dirEntryStyle(c),
		FileEntry:      fileEntryStyle(c),
		FileSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.SelectedFg)).
			Background(lipgloss.Color(c.SelectedBg)),
		AnnotationMark:  treeItemStyle(c, c.Annotation),
		ReviewedMark:    treeItemStyle(c, c.AddFg),
		StatusAdded:     treeItemStyle(c, c.AddFg),
		StatusDeleted:   treeItemStyle(c, c.RemoveFg),
		StatusUntracked: treeItemStyle(c, c.AddFg),
		StatusDefault:   treeItemStyle(c, c.Muted),

		DiffPane:       diffPane,
		DiffPaneActive: diffPaneActive,
		LineAdd: lipgloss.NewStyle().
			Background(lipgloss.Color(c.AddBg)).
			Foreground(lipgloss.Color(c.AddFg)),
		LineRemove: lipgloss.NewStyle().
			Background(lipgloss.Color(c.RemoveBg)).
			Foreground(lipgloss.Color(c.RemoveFg)),
		LineContext: contextStyle(c),
		LineNumber:  lineNumberStyle(c),

		StatusBar: statusBar,

		LineAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(c.AddBg)),
		LineRemoveHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(c.RemoveBg)),
		LineContextHighlight: contextHighlightStyle(c),
		LineModify: lipgloss.NewStyle().
			Background(lipgloss.Color(c.ModifyBg)).
			Foreground(lipgloss.Color(c.ModifyFg)),
		LineModifyHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(c.ModifyBg)),

		DiffCursorLine: cursorLineStyle(c),
		AnnotationLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Annotation)).
			Italic(true),
		SearchMatch: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.SearchFg)).
			Background(lipgloss.Color(c.SearchBg)),

		colors: c,
	}
}

// contextStyle builds the context line style, applying DiffBg as background when set.
func contextStyle(c Colors) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Normal))
	if c.DiffBg != "" {
		s = s.Background(lipgloss.Color(c.DiffBg))
	}
	return s
}

// dirEntryStyle builds the directory entry style, applying TreeBg as background when set.
func dirEntryStyle(c Colors) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Accent)).Bold(true)
	if c.TreeBg != "" {
		s = s.Background(lipgloss.Color(c.TreeBg))
	}
	return s
}

// fileEntryStyle builds the file entry style, applying TreeBg as background when set.
func fileEntryStyle(c Colors) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Normal))
	if c.TreeBg != "" {
		s = s.Background(lipgloss.Color(c.TreeBg))
	}
	return s
}

// treeItemStyle builds a tree item style with the given foreground, applying TreeBg when set.
func treeItemStyle(c Colors, fg string) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	if c.TreeBg != "" {
		s = s.Background(lipgloss.Color(c.TreeBg))
	}
	return s
}

// lineNumberStyle builds the line number style, applying DiffBg as background when set.
func lineNumberStyle(c Colors) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Muted))
	if c.DiffBg != "" {
		s = s.Background(lipgloss.Color(c.DiffBg))
	}
	return s
}

// contextHighlightStyle builds the syntax-highlighted context line style (DiffBg only).
func contextHighlightStyle(c Colors) lipgloss.Style {
	s := lipgloss.NewStyle()
	if c.DiffBg != "" {
		s = s.Background(lipgloss.Color(c.DiffBg))
	}
	return s
}

// cursorLineStyle builds the diff cursor style with optional foreground and background.
func cursorLineStyle(c Colors) lipgloss.Style {
	s := lipgloss.NewStyle()
	if c.CursorFg != "" {
		s = s.Foreground(lipgloss.Color(c.CursorFg))
	}
	if c.CursorBg != "" {
		s = s.Background(lipgloss.Color(c.CursorBg))
	}
	return s
}

// plainStyles returns styles with no colors for --no-colors mode.
// borders are preserved for layout but all color styling is removed.
func plainStyles() styles {
	border := lipgloss.NormalBorder()

	return styles{
		TreePane:        lipgloss.NewStyle().Border(border),
		TreePaneActive:  lipgloss.NewStyle().Border(border),
		DirEntry:        lipgloss.NewStyle().Bold(true),
		FileEntry:       lipgloss.NewStyle(),
		FileSelected:    lipgloss.NewStyle().Reverse(true),
		AnnotationMark:  lipgloss.NewStyle(),
		ReviewedMark:    lipgloss.NewStyle(),
		StatusAdded:     lipgloss.NewStyle(),
		StatusDeleted:   lipgloss.NewStyle(),
		StatusUntracked: lipgloss.NewStyle(),
		StatusDefault:   lipgloss.NewStyle(),

		DiffPane:       lipgloss.NewStyle().Border(border),
		DiffPaneActive: lipgloss.NewStyle().Border(border),
		LineAdd:        lipgloss.NewStyle(),
		LineRemove:     lipgloss.NewStyle(),
		LineContext:    lipgloss.NewStyle(),
		LineNumber:     lipgloss.NewStyle(),

		StatusBar: lipgloss.NewStyle().Padding(0, 1),

		LineAddHighlight:     lipgloss.NewStyle(),
		LineRemoveHighlight:  lipgloss.NewStyle(),
		LineContextHighlight: lipgloss.NewStyle(),
		LineModify:           lipgloss.NewStyle(),
		LineModifyHighlight:  lipgloss.NewStyle(),

		DiffCursorLine: lipgloss.NewStyle().Reverse(true),
		AnnotationLine: lipgloss.NewStyle().Italic(true),
		SearchMatch:    lipgloss.NewStyle().Reverse(true),
	}
}
