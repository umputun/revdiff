package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// bottom-frame label layout constants.
const (
	// frameLabelLeadDashes is the number of border runes drawn between the
	// corner and the label ("└─ +x/-y …").
	frameLabelLeadDashes = 1
	// frameLabelEllipsis is appended when the label is truncated to fit.
	frameLabelEllipsis = "..."
	// frameLabelMinAvail is the smallest usable dash region (after the lead)
	// worth placing a label into; below it the border stays plain.
	frameLabelMinAvail = 10
)

// NormalBorder runes used by the diff pane; the label surgery locates and
// rebuilds the dash run between the two corners of a border line.
const (
	topLeftCorner     = "┌"
	topRightCorner    = "┐"
	bottomLeftCorner  = "└"
	bottomRightCorner = "┘"
	horizBar          = "─"
)

// frameSpan is one styled run in a frame label. color == "" renders in the
// frame's own color (the border SGR already active on the line); a non-empty
// color wraps the run and is restored to the border SGR after it.
type frameSpan struct {
	text  string
	color style.Color
}

// frameLabelsEnabled reports whether the frame-label hints should be drawn:
// the --frame-labels flag is set, a file is loaded, and it has at least one
// change. the change gate keeps pure-context views (plain file, markdown TOC)
// from showing "no modified lines below" on every screen.
func (m Model) frameLabelsEnabled() bool {
	return m.cfg.frameLabels && m.file.name != "" && len(m.file.lines) > 0 &&
		(m.file.adds > 0 || m.file.removes > 0)
}

// applyDiffTopLabel embeds a "+x/-y modified lines above" / "no modified lines
// above" hint into the diff pane's top border line (the changes the reader has
// scrolled past). see applyDiffBottomLabel for the shared surgery contract.
func (m Model) applyDiffTopLabel(rendered string) string {
	if !m.frameLabelsEnabled() {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	newTop, ok := m.buildFrameLabelLine(lines[0], topLeftCorner, topRightCorner, m.aboveSpans())
	if !ok {
		return rendered
	}
	lines[0] = newTop
	return strings.Join(lines, "\n")
}

// applyDiffBottomLabel embeds a "+x/-y modified lines below" / "no modified
// lines below" hint into the diff pane's bottom border line. rendered is the
// fully styled diff pane (post-scrollbar); the transform rebuilds only the
// border line, preserving its display width and ANSI envelope so horizontal
// joins and the frame color stay intact. no-op when disabled or when the
// border line can't be located / is too narrow.
func (m Model) applyDiffBottomLabel(rendered string) string {
	if !m.frameLabelsEnabled() {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	last := len(lines) - 1
	if last < 0 {
		return rendered
	}
	newBottom, ok := m.buildFrameLabelLine(lines[last], bottomLeftCorner, bottomRightCorner, m.belowSpans())
	if !ok {
		return rendered
	}
	lines[last] = newBottom
	return strings.Join(lines, "\n")
}

// buildFrameLabelLine rewrites a single border line with the given spans
// embedded between leftCorner and rightCorner. returns (line, false) when the
// line is not a recognizable border of that shape or the dash region is too
// narrow to hold a label.
func (m Model) buildFrameLabelLine(line, leftCorner, rightCorner string, spans []frameSpan) (string, bool) {
	li := strings.Index(line, leftCorner)
	ri := strings.LastIndex(line, rightCorner)
	if li < 0 || ri < 0 || ri <= li {
		return "", false
	}
	leadingSGR := line[:li]               // border fg/bg SGR active for the whole line
	suffix := line[ri+len(rightCorner):]  // trailing reset after the corner
	mid := line[li+len(leftCorner) : ri]  // the "─" run between the corners
	width := strings.Count(mid, horizBar) // dash count == inner pane width
	avail := width - frameLabelLeadDashes
	if width <= 0 || avail < frameLabelMinAvail {
		return "", false
	}

	label, used := renderFrameLabel(spans, avail, leadingSGR)
	fill := width - frameLabelLeadDashes - used
	if fill < 0 {
		fill = 0
	}
	var b strings.Builder
	b.WriteString(leadingSGR)
	b.WriteString(leftCorner)
	b.WriteString(strings.Repeat(horizBar, frameLabelLeadDashes))
	b.WriteString(label)
	b.WriteString(strings.Repeat(horizBar, fill))
	b.WriteString(rightCorner)
	b.WriteString(suffix)
	return b.String(), true
}

// belowSpans returns the label runs for the bottom border: changes below the
// viewport's last visible row.
func (m Model) belowSpans() []frameSpan {
	adds, removes := m.modifiedBelow()
	return m.changeLabelSpans(adds, removes, "modified lines below", "no modified lines below")
}

// aboveSpans returns the label runs for the top border: changes above the
// viewport's first visible row.
func (m Model) aboveSpans() []frameSpan {
	adds, removes := m.modifiedAbove()
	return m.changeLabelSpans(adds, removes, "modified lines above", "no modified lines above")
}

// changeLabelSpans builds the styled runs: "+x" in the add color, "-y" in the
// remove color, " <suffix>" in the frame color; the all-frame-color <none>
// text when there are no changes on that side.
func (m Model) changeLabelSpans(adds, removes int, suffix, none string) []frameSpan {
	if adds == 0 && removes == 0 {
		return []frameSpan{{text: none}}
	}
	return []frameSpan{
		{text: fmt.Sprintf("+%d", adds), color: m.resolver.Color(style.ColorKeyAddLineFg)},
		{text: "/"},
		{text: fmt.Sprintf("-%d", removes), color: m.resolver.Color(style.ColorKeyRemoveLineFg)},
		{text: " " + suffix},
	}
}

// renderFrameLabel wraps spans in surround spaces (frame-colored) and renders
// them capped at avail display cells, returning the ANSI string and its width.
func renderFrameLabel(spans []frameSpan, avail int, borderSGR string) (out string, used int) {
	surrounded := make([]frameSpan, 0, len(spans)+2)
	surrounded = append(surrounded, frameSpan{text: " "})
	surrounded = append(surrounded, spans...)
	surrounded = append(surrounded, frameSpan{text: " "})
	return renderFrameSpans(surrounded, avail, borderSGR)
}

// renderFrameSpans emits spans into at most avail display cells. when the total
// plain width exceeds avail the tail is cut and frameLabelEllipsis appended
// (in the frame color). colored spans are wrapped with their color and closed
// by re-emitting borderSGR so subsequent runs return to the frame color.
func renderFrameSpans(spans []frameSpan, avail int, borderSGR string) (out string, used int) {
	total := 0
	for _, s := range spans {
		total += runewidth.StringWidth(s.text)
	}
	budget := avail
	ellipsis := ""
	if total > avail {
		ellipsis = frameLabelEllipsis
		budget = avail - runewidth.StringWidth(frameLabelEllipsis)
		if budget < 0 {
			budget = 0
		}
	}

	var b strings.Builder
	for _, s := range spans {
		if used >= budget {
			break
		}
		text := s.text
		w := runewidth.StringWidth(text)
		if used+w > budget {
			text = cutRightToWidth(text, budget-used)
			w = runewidth.StringWidth(text)
		}
		if s.color != "" {
			b.WriteString(string(s.color))
			b.WriteString(text)
			b.WriteString(borderSGR)
		} else {
			b.WriteString(text)
		}
		used += w
	}
	if ellipsis != "" {
		b.WriteString(ellipsis)
		used += runewidth.StringWidth(ellipsis)
	}
	return b.String(), used
}

// modifiedBelow counts added and removed lines below the diff viewport's last
// visible visual row, i.e. the changes the reader has not scrolled to yet.
// returns (0, 0) when the viewport already reaches the end of the file.
func (m Model) modifiedBelow() (adds, removes int) {
	if len(m.file.lines) == 0 {
		return 0, 0
	}
	bottomRow := m.layout.viewport.YOffset + m.layout.viewport.Height - 1
	idx, _ := m.visualRowToDiffLine(bottomRow)
	from := idx + 1 // idx == -1 (file-annotation region) → count the whole file
	if from < 0 {
		from = 0
	}
	if from >= len(m.file.lines) {
		return 0, 0
	}
	return diff.CountChanges(m.file.lines[from:])
}

// modifiedAbove counts added and removed lines above the diff viewport's first
// visible visual row, i.e. the changes the reader has scrolled past. returns
// (0, 0) when the viewport is at the top of the file.
func (m Model) modifiedAbove() (adds, removes int) {
	if len(m.file.lines) == 0 {
		return 0, 0
	}
	idx, _ := m.visualRowToDiffLine(m.layout.viewport.YOffset)
	upto := idx // lines strictly above the first visible line
	if upto < 0 {
		upto = 0
	}
	if upto > len(m.file.lines) {
		upto = len(m.file.lines)
	}
	return diff.CountChanges(m.file.lines[:upto])
}

// cutRightToWidth truncates s to at most w display cells, keeping the start.
// returns "" for w <= 0. wide runes that would straddle the boundary are dropped.
func cutRightToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > w {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}
