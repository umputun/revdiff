package style

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/diff"
)

// Resolver holds pre-materialized color and lipgloss style tables
// and dispatches lookups by key or by diff.ChangeType.
// All fields are private — callers access values through methods only.
type Resolver struct {
	colors Colors
	styles map[StyleKey]lipgloss.Style
}

// NewResolver builds a Resolver from a Colors palette. Normalizes the
// input, builds all lipgloss.Style values, stores them privately.
func NewResolver(c Colors) Resolver {
	c = c.normalize()
	return Resolver{
		colors: c,
		styles: buildStyles(c),
	}
}

// PlainResolver returns a Resolver with no-color styling — used for
// --no-colors mode. Borders are preserved; all color styling is stripped.
func PlainResolver() Resolver {
	return Resolver{
		styles: buildPlainStyles(),
	}
}

// Color returns the ANSI escape sequence for the given color key.
// ColorKeyStatusFg falls back to MutedFg when Colors.StatusFg is empty.
// returns empty Color for ColorKeyUnknown.
func (r Resolver) Color(k ColorKey) Color {
	switch k {
	case ColorKeyAccentFg:
		return Color(ansiColor(r.colors.Accent, 38))
	case ColorKeyMutedFg:
		return Color(ansiColor(r.colors.Muted, 38))
	case ColorKeyAnnotationFg:
		return Color(ansiColor(r.colors.Annotation, 38))
	case ColorKeyStatusFg:
		hex := r.colors.StatusFg
		if hex == "" {
			hex = r.colors.Muted
		}
		return Color(ansiColor(hex, 38))
	case ColorKeyTreePaneBg:
		return Color(ansiColor(r.colors.TreeBg, 48))
	case ColorKeyDiffPaneBg:
		return Color(ansiColor(r.colors.DiffBg, 48))
	case ColorKeyAddLineBg:
		return Color(ansiColor(r.colors.AddBg, 48))
	case ColorKeyRemoveLineBg:
		return Color(ansiColor(r.colors.RemoveBg, 48))
	case ColorKeyModifyLineBg:
		return Color(ansiColor(r.colors.ModifyBg, 48))
	case ColorKeyWordAddBg:
		return Color(ansiColor(r.colors.WordAddBg, 48))
	case ColorKeyWordRemoveBg:
		return Color(ansiColor(r.colors.WordRemoveBg, 48))
	case ColorKeySearchBg:
		return Color(ansiColor(r.colors.SearchBg, 48))
	case ColorKeyAddLineFg:
		return Color(ansiColor(r.colors.AddFg, 38))
	case ColorKeyRemoveLineFg:
		return Color(ansiColor(r.colors.RemoveFg, 38))
	case ColorKeyModifyLineFg:
		return Color(ansiColor(r.colors.ModifyFg, 38))
	case ColorKeySearchFg:
		return Color(ansiColor(r.colors.SearchFg, 38))
	case ColorKeyNormalFg:
		return Color(ansiColor(r.colors.Normal, 38))
	case ColorKeySelectedFg:
		return Color(ansiColor(r.colors.SelectedFg, 38))
	default:
		return ""
	}
}

// Style returns the pre-built lipgloss.Style for the given style key.
// returns an empty lipgloss.Style for unknown keys.
func (r Resolver) Style(k StyleKey) lipgloss.Style {
	if s, ok := r.styles[k]; ok {
		return s
	}
	return lipgloss.NewStyle()
}

// LineBg returns the ANSI background escape sequence for a diff change type.
// ChangeAdd → AddBg, ChangeRemove → RemoveBg, everything else → empty.
func (r Resolver) LineBg(change diff.ChangeType) Color {
	switch change {
	case diff.ChangeAdd:
		return Color(ansiColor(r.colors.AddBg, 48))
	case diff.ChangeRemove:
		return Color(ansiColor(r.colors.RemoveBg, 48))
	default:
		return ""
	}
}

// LineFg returns the ANSI foreground escape sequence for a diff change type.
// Used to color the +/-/~ prefix when chroma highlighting is active and the
// highlighted line style intentionally omits foreground (chroma owns content fg).
// ChangeAdd → AddFg, ChangeRemove → RemoveFg, everything else → empty.
// Modify lines (collapsed mode) are synthesized in the UI layer and not a
// diff change type — call Color(ColorKeyModifyLineFg) directly for those.
func (r Resolver) LineFg(change diff.ChangeType) Color {
	switch change {
	case diff.ChangeAdd:
		return Color(ansiColor(r.colors.AddFg, 38))
	case diff.ChangeRemove:
		return Color(ansiColor(r.colors.RemoveFg, 38))
	default:
		return ""
	}
}

// LineStyle returns the lipgloss.Style for a diff line based on change type
// and whether syntax highlighting is active.
func (r Resolver) LineStyle(change diff.ChangeType, highlighted bool) lipgloss.Style {
	switch {
	case change == diff.ChangeAdd && highlighted:
		return r.Style(StyleKeyLineAddHighlight)
	case change == diff.ChangeAdd:
		return r.Style(StyleKeyLineAdd)
	case change == diff.ChangeRemove && highlighted:
		return r.Style(StyleKeyLineRemoveHighlight)
	case change == diff.ChangeRemove:
		return r.Style(StyleKeyLineRemove)
	case highlighted:
		return r.Style(StyleKeyLineContextHighlight)
	default:
		return r.Style(StyleKeyLineContext)
	}
}

// WordDiffBg returns the ANSI background escape for intra-line word-diff highlighting.
// ChangeAdd → WordAddBg, ChangeRemove → WordRemoveBg, everything else → empty.
func (r Resolver) WordDiffBg(change diff.ChangeType) Color {
	switch change {
	case diff.ChangeAdd:
		return Color(ansiColor(r.colors.WordAddBg, 48))
	case diff.ChangeRemove:
		return Color(ansiColor(r.colors.WordRemoveBg, 48))
	default:
		return ""
	}
}

// IndicatorBg returns the background color for a horizontal scroll indicator,
// falling back to the diff pane background when the line has no explicit bg.
func (r Resolver) IndicatorBg(change diff.ChangeType) Color {
	bg := r.LineBg(change)
	if bg != "" {
		return bg
	}
	return Color(ansiColor(r.colors.DiffBg, 48))
}

// buildStyles creates all lipgloss.Style values from a normalized Colors palette.
func buildStyles(c Colors) map[StyleKey]lipgloss.Style {
	m := make(map[StyleKey]lipgloss.Style, len(StyleKeyValues))
	border := lipgloss.NormalBorder()

	// tree pane styles
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
	m[StyleKeyTreePane] = treePane
	m[StyleKeyTreePaneActive] = treePaneActive

	// diff pane styles
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
	m[StyleKeyDiffPane] = diffPane
	m[StyleKeyDiffPaneActive] = diffPaneActive

	// diff line styles
	m[StyleKeyLineAdd] = lipgloss.NewStyle().
		Background(lipgloss.Color(c.AddBg)).
		Foreground(lipgloss.Color(c.AddFg))
	m[StyleKeyLineRemove] = lipgloss.NewStyle().
		Background(lipgloss.Color(c.RemoveBg)).
		Foreground(lipgloss.Color(c.RemoveFg))
	m[StyleKeyLineContext] = c.contextStyle()
	m[StyleKeyLineModify] = lipgloss.NewStyle().
		Background(lipgloss.Color(c.ModifyBg)).
		Foreground(lipgloss.Color(c.ModifyFg))

	// syntax-highlighted line styles (background only, chroma owns foreground)
	m[StyleKeyLineAddHighlight] = lipgloss.NewStyle().
		Background(lipgloss.Color(c.AddBg))
	m[StyleKeyLineRemoveHighlight] = lipgloss.NewStyle().
		Background(lipgloss.Color(c.RemoveBg))
	m[StyleKeyLineContextHighlight] = c.contextHighlightStyle()
	m[StyleKeyLineModifyHighlight] = lipgloss.NewStyle().
		Background(lipgloss.Color(c.ModifyBg))

	// line number style
	m[StyleKeyLineNumber] = c.lineNumberStyle()

	// file tree entry styles
	m[StyleKeyDirEntry] = c.dirEntryStyle()
	m[StyleKeyFileEntry] = c.fileEntryStyle()
	m[StyleKeyFileSelected] = lipgloss.NewStyle().
		Foreground(lipgloss.Color(c.SelectedFg)).
		Background(lipgloss.Color(c.SelectedBg))
	m[StyleKeyAnnotationMark] = c.treeItemStyle(c.Annotation)
	m[StyleKeyReviewedMark] = c.treeItemStyle(c.AddFg)

	// file status styles
	m[StyleKeyStatusAdded] = c.treeItemStyle(c.AddFg)
	m[StyleKeyStatusDeleted] = c.treeItemStyle(c.RemoveFg)
	m[StyleKeyStatusUntracked] = c.treeItemStyle(c.AddFg)
	m[StyleKeyStatusDefault] = c.treeItemStyle(c.Muted)

	// status bar
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
	m[StyleKeyStatusBar] = statusBar

	// search match
	m[StyleKeySearchMatch] = lipgloss.NewStyle().
		Foreground(lipgloss.Color(c.SearchFg)).
		Background(lipgloss.Color(c.SearchBg))

	// annotation input styles (D14 — all inline lipgloss construction from annotate.go)
	inputStyle := lipgloss.NewStyle()
	if c.Normal != "" {
		inputStyle = inputStyle.Foreground(lipgloss.Color(c.Normal))
	}
	if c.DiffBg != "" {
		inputStyle = inputStyle.Background(lipgloss.Color(c.DiffBg))
	}
	m[StyleKeyAnnotInputText] = inputStyle

	placeholderStyle := lipgloss.NewStyle()
	if c.Muted != "" {
		placeholderStyle = placeholderStyle.Foreground(lipgloss.Color(c.Muted))
	}
	if c.DiffBg != "" {
		placeholderStyle = placeholderStyle.Background(lipgloss.Color(c.DiffBg))
	}
	m[StyleKeyAnnotInputPlaceholder] = placeholderStyle

	// cursor uses same style as input text
	m[StyleKeyAnnotInputCursor] = inputStyle

	// annotation list popup border (D14 — from annotlist.go)
	annotListBorder := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(c.Accent)).
		Padding(1, 1)
	m[StyleKeyAnnotListBorder] = annotListBorder

	// help overlay box (D14 — from handlers.go)
	helpBox := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Accent)).
		Padding(1, 2)
	if c.DiffBg != "" {
		bg := lipgloss.Color(c.DiffBg)
		helpBox = helpBox.Background(bg).BorderBackground(bg)
	}
	m[StyleKeyHelpBox] = helpBox

	// theme selector box (D14 — from themeselect.go)
	themeBox := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Accent)).
		Padding(1, 1)
	if c.DiffBg != "" {
		bg := lipgloss.Color(c.DiffBg)
		themeBox = themeBox.Background(bg).BorderBackground(bg)
	}
	m[StyleKeyThemeSelectBox] = themeBox

	// theme selector focused state — same as default for now;
	// differentiation happens at the call site via width/focus logic
	m[StyleKeyThemeSelectBoxFocused] = themeBox

	// info overlay box (accent border matches help/annot/theme overlays, optional DiffBg background)
	infoBox := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(c.Accent)).
		Padding(1, 2)
	if c.DiffBg != "" {
		bg := lipgloss.Color(c.DiffBg)
		infoBox = infoBox.Background(bg).BorderBackground(bg)
	}
	m[StyleKeyInfoBox] = infoBox

	return m
}

// buildPlainStyles returns a style map with no colors for --no-colors mode.
// borders are preserved for layout but all color styling is removed.
func buildPlainStyles() map[StyleKey]lipgloss.Style {
	m := make(map[StyleKey]lipgloss.Style, len(StyleKeyValues))
	border := lipgloss.NormalBorder()

	m[StyleKeyTreePane] = lipgloss.NewStyle().Border(border)
	m[StyleKeyTreePaneActive] = lipgloss.NewStyle().Border(border)
	m[StyleKeyDiffPane] = lipgloss.NewStyle().Border(border)
	m[StyleKeyDiffPaneActive] = lipgloss.NewStyle().Border(border)

	m[StyleKeyDirEntry] = lipgloss.NewStyle().Bold(true)
	m[StyleKeyFileEntry] = lipgloss.NewStyle()
	m[StyleKeyFileSelected] = lipgloss.NewStyle().Reverse(true)
	m[StyleKeyAnnotationMark] = lipgloss.NewStyle()
	m[StyleKeyReviewedMark] = lipgloss.NewStyle()

	m[StyleKeyStatusAdded] = lipgloss.NewStyle()
	m[StyleKeyStatusDeleted] = lipgloss.NewStyle()
	m[StyleKeyStatusUntracked] = lipgloss.NewStyle()
	m[StyleKeyStatusDefault] = lipgloss.NewStyle()

	m[StyleKeyLineAdd] = lipgloss.NewStyle()
	m[StyleKeyLineRemove] = lipgloss.NewStyle()
	m[StyleKeyLineContext] = lipgloss.NewStyle()
	m[StyleKeyLineModify] = lipgloss.NewStyle()
	m[StyleKeyLineNumber] = lipgloss.NewStyle()

	m[StyleKeyLineAddHighlight] = lipgloss.NewStyle()
	m[StyleKeyLineRemoveHighlight] = lipgloss.NewStyle()
	m[StyleKeyLineContextHighlight] = lipgloss.NewStyle()
	m[StyleKeyLineModifyHighlight] = lipgloss.NewStyle()

	m[StyleKeyStatusBar] = lipgloss.NewStyle().Padding(0, 1)
	m[StyleKeySearchMatch] = lipgloss.NewStyle().Reverse(true)

	// overlay / input styles — plain (no colors)
	m[StyleKeyAnnotInputText] = lipgloss.NewStyle()
	m[StyleKeyAnnotInputPlaceholder] = lipgloss.NewStyle()
	m[StyleKeyAnnotInputCursor] = lipgloss.NewStyle()
	m[StyleKeyAnnotListBorder] = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).Padding(1, 1)
	m[StyleKeyHelpBox] = lipgloss.NewStyle().
		Border(border).Padding(1, 2)
	m[StyleKeyThemeSelectBox] = lipgloss.NewStyle().
		Border(border).Padding(1, 1)
	m[StyleKeyThemeSelectBoxFocused] = lipgloss.NewStyle().
		Border(border).Padding(1, 1)
	m[StyleKeyInfoBox] = lipgloss.NewStyle().
		Border(border).Padding(1, 2)

	return m
}

// contextStyle builds the context line style, applying DiffBg as background when set.
func (c Colors) contextStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Normal))
	if c.DiffBg != "" {
		s = s.Background(lipgloss.Color(c.DiffBg))
	}
	return s
}

// dirEntryStyle builds the directory entry style, applying TreeBg as background when set.
func (c Colors) dirEntryStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Accent)).Bold(true)
	if c.TreeBg != "" {
		s = s.Background(lipgloss.Color(c.TreeBg))
	}
	return s
}

// fileEntryStyle builds the file entry style, applying TreeBg as background when set.
func (c Colors) fileEntryStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Normal))
	if c.TreeBg != "" {
		s = s.Background(lipgloss.Color(c.TreeBg))
	}
	return s
}

// treeItemStyle builds a tree item style with the given foreground, applying TreeBg when set.
func (c Colors) treeItemStyle(fg string) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	if c.TreeBg != "" {
		s = s.Background(lipgloss.Color(c.TreeBg))
	}
	return s
}

// lineNumberStyle builds the line number style, applying DiffBg as background when set.
func (c Colors) lineNumberStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Muted))
	if c.DiffBg != "" {
		s = s.Background(lipgloss.Color(c.DiffBg))
	}
	return s
}

// contextHighlightStyle builds the syntax-highlighted context line style (DiffBg only).
func (c Colors) contextHighlightStyle() lipgloss.Style {
	s := lipgloss.NewStyle()
	if c.DiffBg != "" {
		s = s.Background(lipgloss.Color(c.DiffBg))
	}
	return s
}
