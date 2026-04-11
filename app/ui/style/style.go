package style

// Colors holds hex color values (#rrggbb) for TUI rendering.
// This is the construction input for NewResolver — it is not part of
// any type's runtime API.
type Colors struct {
	Accent       string // active pane borders, dir names
	Border       string // inactive pane borders
	Normal       string // file entries, context lines
	Muted        string // divider lines, status bar
	SelectedFg   string // selected file text
	SelectedBg   string // selected file background
	Annotation   string // annotation text and markers
	CursorFg     string // diff cursor indicator foreground
	CursorBg     string // diff cursor line background
	AddFg        string // added line foreground
	AddBg        string // added line background
	RemoveFg     string // removed line foreground
	RemoveBg     string // removed line background
	WordAddBg    string // intra-line word-diff add background (auto-derived if empty)
	WordRemoveBg string // intra-line word-diff remove background (auto-derived if empty)
	ModifyFg     string // modified line foreground (collapsed mode)
	ModifyBg     string // modified line background (collapsed mode)
	TreeBg       string // file tree pane background
	DiffBg       string // diff pane background
	StatusFg     string // status bar foreground
	StatusBg     string // status bar background
	SearchFg     string // search match foreground
	SearchBg     string // search match background
}

// normalizeColors ensures all color values have # prefix where needed
// and auto-derives WordAddBg/WordRemoveBg from AddBg/RemoveBg via
// shiftLightness when those fields are empty.
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
	// auto-derive word-diff backgrounds from add/remove bg when not explicitly set.
	// the shift amount is intentionally small (0.08 in HSL lightness, not 0.15): the
	// default palette uses very dark add/remove bgs (L~0.11-0.15), so a larger shift
	// would roughly double the lightness and crush contrast against syntax-highlighted
	// text on top. 0.08 keeps the changed-range span visibly distinct without making
	// the bg read as a different color entirely.
	if c.WordAddBg == "" && c.AddBg != "" {
		c.WordAddBg = shiftLightness(c.AddBg, 0.08)
	}
	if c.WordRemoveBg == "" && c.RemoveBg != "" {
		c.WordRemoveBg = shiftLightness(c.RemoveBg, 0.08)
	}
	c.WordAddBg = normalizeColor(c.WordAddBg)
	c.WordRemoveBg = normalizeColor(c.WordRemoveBg)
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

// normalizeColor ensures a hex color value has a # prefix.
// returns empty string unchanged (used for optional colors).
func normalizeColor(s string) string {
	if s == "" || s[0] == '#' {
		return s
	}
	return "#" + s
}
