package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// handleAnnotNav jumps to the next or previous annotation in the flat
// cross-file list (alphabetical files, file-level first per file, then
// ascending lines — same order as the @ popup). At the boundaries the
// action is a silent no-op. Cross-file handoff goes through the existing
// tryJumpToAnnotationTarget machinery, so collapsed-hunk expansion, TOC
// sync, and pendingAnnotJump-based file load all come for free.
//
// When a target is in the store but not currently displayable (cross-file
// path filtered out of the tree, or same-file line missing from the loaded
// diff in compact mode), the walker advances past it and retries the next
// candidate in the same direction. Without that loop the cursor would not
// move on a silent-fail target and every subsequent }/{ press would
// recompute the same hidden target, trapping the user. The loop is bounded
// by the flat list length so a faulty store can never spin forever.
func (m Model) handleAnnotNav(forward bool) (tea.Model, tea.Cmd) {
	flat := m.buildAnnotListItems()
	if len(flat) == 0 {
		return m, nil
	}
	cur := m.currentAnnotKey()
	for range flat {
		target, ok := pickAdjacentAnnotation(flat, cur, forward)
		if !ok {
			return m, nil
		}
		nextModel, cmd, jumped := m.tryJumpToAnnotationTarget(&overlay.AnnotationTarget{
			File:       target.File,
			ChangeType: target.Type,
			Line:       target.Line,
		})
		if jumped {
			return nextModel, cmd
		}
		cur = cursorAnnotKey{file: target.File, line: target.Line, typ: target.Type, onAnnot: true}
	}
	return m, nil
}

// cursorAnnotKey is the cursor's position in annotation space. onAnnot is
// true when the cursor is exactly on an existing annotation row (file,
// line, AND type all match). In that case navigation steps by index in the
// flat list. Otherwise navigation uses an insertion-point fallback.
type cursorAnnotKey struct {
	file    string
	line    int
	typ     string
	onAnnot bool
}

// currentAnnotKey returns the cursor's annotation-space key. The file-level
// annotation row (diffCursor == -1) maps to (file, 0, "") which matches
// the storage form of file-level annotations. ChangeDivider rows (which
// carry OldNum=NewNum=0) and out-of-range cursors collapse to line=-1 so
// forward navigation correctly treats any same-file annotation — including
// a file-level Line=0 — as strictly after the cursor.
func (m Model) currentAnnotKey() cursorAnnotKey {
	file := m.file.name
	if m.nav.diffCursor == -1 {
		return cursorAnnotKey{file: file, line: 0, typ: "", onAnnot: m.hasFileAnnotation()}
	}
	if m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return cursorAnnotKey{file: file, line: -1, typ: "", onAnnot: false}
	}
	dl := m.file.lines[m.nav.diffCursor]
	if dl.ChangeType == diff.ChangeDivider {
		return cursorAnnotKey{file: file, line: -1, typ: "", onAnnot: false}
	}
	line := m.diffLineNum(dl)
	typ := string(dl.ChangeType)
	return cursorAnnotKey{file: file, line: line, typ: typ, onAnnot: m.store.Has(file, line, typ)}
}

// pickAdjacentAnnotation walks the flat list and selects the neighbor of
// the cursor. When the cursor is exactly on an annotation, steps by index
// in the list (so same-line different-type annotations both step in turn).
// When the cursor sits between annotations, uses an insertion-point
// fallback over the (file, line) order.
func pickAdjacentAnnotation(flat []annotation.Annotation, cur cursorAnnotKey, forward bool) (annotation.Annotation, bool) {
	if idx, ok := exactAnnotIndex(flat, cur); ok {
		return stepFlatList(flat, idx, forward)
	}
	insIdx := annotInsertionPoint(flat, cur)
	if forward {
		if insIdx >= len(flat) {
			return annotation.Annotation{}, false
		}
		return flat[insIdx], true
	}
	if insIdx == 0 {
		return annotation.Annotation{}, false
	}
	return flat[insIdx-1], true
}

// exactAnnotIndex returns the flat-list index of an annotation that exactly
// matches the cursor (file, line, type). When cur.onAnnot is false it
// short-circuits without scanning. ok=false means "use insertion-point
// fallback instead."
func exactAnnotIndex(flat []annotation.Annotation, cur cursorAnnotKey) (int, bool) {
	if !cur.onAnnot {
		return 0, false
	}
	for i, a := range flat {
		if a.File == cur.file && a.Line == cur.line && a.Type == cur.typ {
			return i, true
		}
	}
	return 0, false
}

// stepFlatList returns flat[idx+1] for forward and flat[idx-1] for backward,
// with ok=false at the boundaries.
func stepFlatList(flat []annotation.Annotation, idx int, forward bool) (annotation.Annotation, bool) {
	if forward {
		if idx+1 < len(flat) {
			return flat[idx+1], true
		}
		return annotation.Annotation{}, false
	}
	if idx > 0 {
		return flat[idx-1], true
	}
	return annotation.Annotation{}, false
}

// annotInsertionPoint returns the index where the cursor would be inserted
// in the flat list under (file, line) ordering — i.e. the index of the
// first annotation strictly after the cursor, or len(flat) if all entries
// are at or before the cursor.
func annotInsertionPoint(flat []annotation.Annotation, cur cursorAnnotKey) int {
	for i, a := range flat {
		if compareAnnotPos(a.File, a.Line, cur.file, cur.line) > 0 {
			return i
		}
	}
	return len(flat)
}

// compareAnnotPos compares two (file, line) annotation positions using the
// same ordering as the flat annotation list: alphabetical by file, then
// ascending by line within a file. Returns -1, 0, or 1.
func compareAnnotPos(aFile string, aLine int, bFile string, bLine int) int {
	if aFile != bFile {
		if aFile < bFile {
			return -1
		}
		return 1
	}
	if aLine < bLine {
		return -1
	}
	if aLine > bLine {
		return 1
	}
	return 0
}
