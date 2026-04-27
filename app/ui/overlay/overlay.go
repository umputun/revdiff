// Package overlay owns all layered popup UI for revdiff — help, annotation list,
// and theme selector overlays. It provides a Manager coordinator that enforces
// mutual exclusivity (only one overlay visible at a time), routes key dispatch
// to the active overlay, and composes the overlay on top of the base view via
// ANSI-aware centered compositing.
//
// Callers supply fully populated spec structs (HelpSpec, AnnotListSpec, ThemeSelectSpec)
// when opening an overlay and handle side effects by switching on the returned Outcome
// from HandleKey. The overlay package has no dependency on ui.Model, annotation store,
// theme loading, or any filesystem operation.
package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

// Kind identifies which overlay is currently active.
type Kind int

const (
	KindNone        Kind = iota
	KindHelp             // help overlay
	KindAnnotList        // annotation list popup
	KindThemeSelect      // theme selector popup
	KindInfo             // unified info popup (description + session + commits)
)

// OutcomeKind describes what happened after a key press in an overlay.
type OutcomeKind int

const (
	OutcomeNone             OutcomeKind = iota // key consumed, no side effect
	OutcomeClosed                              // overlay was closed
	OutcomeAnnotationChosen                    // user picked an annotation (target in Outcome.AnnotationTarget)
	OutcomeThemePreview                        // cursor moved to a new theme (name in Outcome.ThemeChoice)
	OutcomeThemeConfirmed                      // user confirmed a theme (name in Outcome.ThemeChoice)
	OutcomeThemeCanceled                       // user canceled theme selection
)

// Outcome is the return value from HandleKey. Callers switch on Kind and
// read AnnotationTarget or ThemeChoice for the relevant outcome.
type Outcome struct {
	Kind             OutcomeKind
	AnnotationTarget *AnnotationTarget
	ThemeChoice      *ThemeChoice
}

// RenderCtx carries per-render parameters passed to Compose.
type RenderCtx struct {
	Width    int
	Height   int
	Resolver Resolver
}

// Resolver is a narrow view of style.Resolver consumed by overlay rendering.
type Resolver interface {
	Style(k style.StyleKey) lipgloss.Style
	Color(k style.ColorKey) style.Color
}

// HelpSpec describes the help overlay content.
type HelpSpec struct {
	Sections []HelpSection
}

// HelpSection is a titled group of key bindings inside the help overlay.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// HelpEntry is a single key-description pair in a help section.
type HelpEntry struct {
	Keys        string
	Description string
}

// AnnotListSpec describes the annotation list popup content.
type AnnotListSpec struct {
	Items []AnnotationItem
}

// AnnotationItem is one entry in the annotation list popup.
// embeds AnnotationTarget for the jump destination; Comment is the display text.
type AnnotationItem struct {
	AnnotationTarget
	Comment string
}

// AnnotationTarget identifies the jump destination for an annotation list selection.
type AnnotationTarget struct {
	File       string
	ChangeType string
	Line       int
}

// ThemeSelectSpec describes the theme selector popup content.
type ThemeSelectSpec struct {
	Items      []ThemeItem
	ActiveName string
}

// ThemeItem is one entry in the theme selector list.
type ThemeItem struct {
	Name        string
	Local       bool
	AccentColor string
}

// ThemeChoice carries the selected theme name.
type ThemeChoice struct {
	Name string
}

// InfoSpec describes the unified info popup, composed of three optional
// sections rendered top-to-bottom: an agent-supplied prose description (#130
// — empty hides the section), invocation/session info from --description-less
// review-info plumbing (Rows; always shown when non-empty), and the commit log
// for the current ref range (Commits; gated by CommitsApplicable so modes
// without a meaningful range — stdin/staged/all-files/file-only — hide the
// commits section entirely instead of rendering "no commits in this mode").
//
// CommitsApplicable=false hides the commits section so the popup is never a
// dead-end: the session section explains the mode, and there's no need to
// duplicate that as a "no commits" message.
//
// Truncated marks the commit list as capped at diff.MaxCommits entries.
// CommitsErr surfaces a CommitLog fetch error inside the commits section,
// taking precedence over the empty-list message; it does not gate the popup.
type InfoSpec struct {
	// HeaderText is rendered as the centered top-border label. Typically a
	// compact mode summary ("vs HEAD~3", "stdin patch.diff", "working tree").
	// Empty falls back to the literal " info " title.
	HeaderText string
	// FooterText is rendered as a centered bottom-border label, used to
	// surface aggregate stats (files, +/-, status, vcs) without spending
	// body rows on them. Empty leaves the bottom border bare. Both
	// header and footer gracefully no-op when too wide for the popup.
	FooterText        string
	Description       string
	Rows              []InfoRow
	Commits           []diff.CommitInfo
	CommitsApplicable bool
	CommitsLoaded     bool
	Truncated         bool
	CommitsErr        error
}

// InfoRow is a label/value line rendered in the session section of the info
// popup. MutedSuffix is an optional secondary text rendered after Value in
// the muted foreground color, used for "name (path)"-style rows where the
// primary token (project name, file basename) deserves prominence and the
// secondary token (full path) is contextual. Empty MutedSuffix renders as
// a plain Value row.
type InfoRow struct {
	Label       string
	Value       string
	MutedSuffix string
}

// Manager coordinates overlay lifecycle: open/close, key routing, and render composition.
// Only one overlay can be active at a time.
type Manager struct {
	kind     Kind
	help     helpOverlay
	annotLst annotListOverlay
	themeSel themeSelectOverlay
	info     infoOverlay
	// bounds is the popup rectangle on screen as of the last Compose call;
	// used by HandleMouse to hit-test clicks and translate to popup-local coords.
	bounds popupBounds
}

// popupBounds holds the screen rectangle of the last-composed popup.
// zero-valued when no overlay has been rendered yet (kind == KindNone).
type popupBounds struct {
	x, y, w, h int
}

// contains reports whether (x, y) falls inside the popup rectangle.
func (b popupBounds) contains(x, y int) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

// NewManager creates a Manager with no active overlay.
func NewManager() *Manager { return &Manager{} }

// Active reports whether any overlay is currently visible.
func (m *Manager) Active() bool { return m.kind != KindNone }

// Kind returns the currently active overlay kind.
func (m *Manager) Kind() Kind { return m.kind }

// Close dismisses whatever overlay is active.
func (m *Manager) Close() {
	m.kind = KindNone
	m.bounds = popupBounds{}
}

// OpenHelp activates the help overlay with the given spec.
func (m *Manager) OpenHelp(spec HelpSpec) {
	m.Close()
	m.kind = KindHelp
	m.help.open(spec)
}

// OpenAnnotList activates the annotation list popup with the given spec.
func (m *Manager) OpenAnnotList(spec AnnotListSpec) {
	m.Close()
	m.kind = KindAnnotList
	m.annotLst.open(spec)
}

// OpenThemeSelect activates the theme selector popup with the given spec.
func (m *Manager) OpenThemeSelect(spec ThemeSelectSpec) {
	m.Close()
	m.kind = KindThemeSelect
	m.themeSel.open(spec)
}

// OpenInfo activates the unified info popup with the given spec.
func (m *Manager) OpenInfo(spec InfoSpec) {
	m.Close()
	m.kind = KindInfo
	m.info.open(spec)
}

// UpdateInfo replaces the active info popup's spec without resetting
// the user's scroll position. Used when async data (review-stats fetch,
// commit-log fetch) lands while the popup is open — the popup re-reads
// the new spec on the next render and the user sees the loading state
// flip to loaded inline. No-op when the active overlay is not the info popup,
// so callers can fire it unconditionally without checking Kind() first.
func (m *Manager) UpdateInfo(spec InfoSpec) {
	if m.kind != KindInfo {
		return
	}
	m.info.spec = spec
}

// HandleKey routes a key press to the active overlay and returns the outcome.
// auto-closes the overlay for outcomes that imply dismissal.
// returns Outcome{Kind: OutcomeNone} when no overlay is active.
func (m *Manager) HandleKey(msg tea.KeyMsg, action keymap.Action) Outcome {
	var out Outcome
	switch m.kind {
	case KindNone:
		return Outcome{}
	case KindHelp:
		out = m.help.handleKey(msg, action)
	case KindAnnotList:
		out = m.annotLst.handleKey(msg, action)
	case KindThemeSelect:
		out = m.themeSel.handleKey(msg, action)
	case KindInfo:
		out = m.info.handleKey(msg, action)
	default:
		return Outcome{}
	}

	switch out.Kind {
	case OutcomeClosed, OutcomeAnnotationChosen, OutcomeThemeConfirmed, OutcomeThemeCanceled:
		m.Close()
	case OutcomeNone, OutcomeThemePreview: // no state change
	}

	return out
}

// HandleMouse routes a mouse event to the active overlay. wheel events drive
// per-overlay scroll/cursor navigation; left-clicks inside the popup hit-test
// an item row and can produce selection outcomes (jump/confirm); clicks
// outside the popup and other buttons are consumed without side effects.
// returns Outcome{Kind: OutcomeNone} when no overlay is active. mirrors
// HandleKey: outcomes that imply dismissal auto-close.
//
// left-click coords are translated to popup-local coords before dispatch so
// each overlay can reason about its own layout (border + padding + content
// rows) without knowing screen geometry. clicks outside the popup bounds are
// swallowed rather than dismissing the overlay — intentionally conservative
// to avoid accidental closes.
func (m *Manager) HandleMouse(msg tea.MouseMsg) Outcome {
	if m.kind == KindNone {
		return Outcome{}
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		if !m.bounds.contains(msg.X, msg.Y) {
			return Outcome{Kind: OutcomeNone}
		}
		msg.X -= m.bounds.x
		msg.Y -= m.bounds.y
	}
	var out Outcome
	switch m.kind {
	case KindHelp:
		out = m.help.handleMouse(msg)
	case KindAnnotList:
		out = m.annotLst.handleMouse(msg)
	case KindThemeSelect:
		out = m.themeSel.handleMouse(msg)
	case KindInfo:
		out = m.info.handleMouse(msg)
	default: // KindNone handled by the early return above
		return Outcome{}
	}

	switch out.Kind {
	case OutcomeClosed, OutcomeAnnotationChosen, OutcomeThemeConfirmed, OutcomeThemeCanceled:
		m.Close()
	case OutcomeNone, OutcomeThemePreview: // no state change
	}

	return out
}

// Compose renders the active overlay on top of base using centered compositing.
// returns base unchanged when no overlay is active.
func (m *Manager) Compose(base string, ctx RenderCtx) string {
	var fg string
	switch m.kind {
	case KindNone:
		return base
	case KindHelp:
		fg = m.help.render(ctx, m)
	case KindAnnotList:
		fg = m.annotLst.render(ctx, m)
	case KindThemeSelect:
		fg = m.themeSel.render(ctx, m)
	case KindInfo:
		fg = m.info.render(ctx, m)
	}
	return m.overlayCenter(base, fg, ctx.Width)
}

// overlayCenter composites fg on top of bg, centered horizontally and vertically.
// uses ANSI-aware string cutting to preserve styling in both layers. records the
// composed popup rectangle on the Manager so HandleMouse can hit-test clicks.
func (m *Manager) overlayCenter(bg, fg string, width int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	fgWidth := lipgloss.Width(fg)
	fgHeight := len(fgLines)
	bgHeight := len(bgLines)

	startY := (bgHeight - fgHeight) / 2
	startX := max((width-fgWidth)/2, 0)

	m.bounds = popupBounds{x: startX, y: startY, w: fgWidth, h: fgHeight}

	for i, fgLine := range fgLines {
		bgIdx := startY + i
		if bgIdx < 0 || bgIdx >= bgHeight {
			continue
		}
		bgLine := bgLines[bgIdx]
		bgW := lipgloss.Width(bgLine)
		if bgW < width {
			bgLine += strings.Repeat(" ", width-bgW)
		}

		left := ansi.Cut(bgLine, 0, startX)
		right := ansi.Cut(bgLine, startX+fgWidth, width)
		bgLines[bgIdx] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

// injectBorderTitle replaces part of the top border line with a centered title.
// accentFg is an ANSI fg escape for border characters, paneBg is an ANSI bg escape
// for the border background (both from Resolver.Color lookups). Either may be empty.
func (m *Manager) injectBorderTitle(box, title string, popupWidth int, accentFg, paneBg string) string {
	return injectBorderEdgeText(box, title, popupWidth, accentFg, paneBg, true)
}

// injectBorderFooter replaces part of the bottom border line with a centered
// label. Mirrors injectBorderTitle but operates on the last row of the rendered
// box. Used by the unified info popup to surface aggregate stats (file count,
// line totals, status histogram) without occupying body rows. Gracefully
// no-ops when the footer text would not fit (popupWidth too small).
func (m *Manager) injectBorderFooter(box, footer string, popupWidth int, accentFg, paneBg string) string {
	return injectBorderEdgeText(box, footer, popupWidth, accentFg, paneBg, false)
}

// injectBorderEdgeText is the shared body for top/bottom border-label injection.
// isTop=true rewrites the first line using the top-border glyphs, false rewrites
// the last line using the bottom-border glyphs. The geometry math (centering,
// left/right pad lengths, fit check) is identical between the two — only the
// target row index and the corner/line glyphs differ, so collapsing this lets
// changes to the centering rule (e.g. minimum margin) stay in one place.
func injectBorderEdgeText(box, text string, popupWidth int, accentFg, paneBg string, isTop bool) string {
	boxLines := strings.Split(box, "\n")
	if len(boxLines) == 0 {
		return box
	}
	idx := 0
	if !isTop {
		idx = len(boxLines) - 1
	}

	line := boxLines[idx]
	lineWidth := lipgloss.Width(line)
	textWidth := lipgloss.Width(text)
	if textWidth >= lineWidth-4 {
		return box
	}

	textStart := max((lineWidth-textWidth)/2, 2)

	border := lipgloss.NormalBorder()
	leftCorner := border.TopLeft
	rightCorner := border.TopRight
	mid := border.Top
	if !isTop {
		leftCorner = border.BottomLeft
		rightCorner = border.BottomRight
		mid = border.Bottom
	}

	leftLen := textStart - 1
	rightLen := max(popupWidth-textStart-textWidth+1, 0)

	bgSeq := ""
	bgReset := ""
	if paneBg != "" {
		bgSeq = paneBg
		bgReset = string(style.ResetBg)
	}
	fgSeq := ""
	fgReset := ""
	if accentFg != "" {
		fgSeq = accentFg
		fgReset = resetFg
	}
	newLine := bgSeq + fgSeq +
		leftCorner +
		strings.Repeat(mid, leftLen) +
		text +
		strings.Repeat(mid, rightLen) +
		rightCorner +
		fgReset + bgReset

	boxLines[idx] = newLine
	return strings.Join(boxLines, "\n")
}

const resetFg = "\033[39m"
