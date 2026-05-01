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
// diff in compact mode), the walker advances past it and tries the next
// candidate in the same direction. Without that walk-past the cursor would
// not move on a silent-fail target and every subsequent }/{ press would
// recompute the same hidden target, trapping the user.
//
// The walk is index-based: starting position is computed once via
// startingFlatIndex, then the loop steps by ±1 per attempt. Each candidate
// is examined at most once, so the worst case is O(N) over flat — even
// when every annotation is non-jumpable.
func (m Model) handleAnnotNav(forward bool) (tea.Model, tea.Cmd) {
	flat := m.buildAnnotListItems()
	if len(flat) == 0 {
		return m, nil
	}
	cur := m.currentAnnotKey()
	step := 1
	if !forward {
		step = -1
	}
	for idx := startingFlatIndex(flat, cur, forward); idx >= 0 && idx < len(flat); idx += step {
		target := flat[idx]
		nextModel, cmd, jumped := m.tryJumpToAnnotationTarget(&overlay.AnnotationTarget{
			File:       target.File,
			ChangeType: target.Type,
			Line:       target.Line,
		})
		if jumped {
			return nextModel, cmd
		}
	}
	return m, nil
}

// cursorAnnotKey is the cursor's position in annotation space. onAnnot is
// true when the cursor's diff position matches an annotation in the store
// (file, line, AND type), regardless of whether the cursor visually sits
// on the diff line or on the annotation comment sub-row — both point to
// the same annotation. In that case navigation steps by index in the flat
// list; otherwise it uses an insertion-point fallback.
type cursorAnnotKey struct {
	file    string
	line    int
	typ     string
	onAnnot bool
}

// currentAnnotKey returns the cursor's annotation-space key.
//
//   - File-level annotation row (diffCursor == -1) maps to (file, 0, "")
//     which matches the storage form of file-level annotations.
//   - Out-of-range cursor collapses to line=-1 so forward navigation treats
//     any same-file annotation as strictly after.
//   - ChangeDivider rows (which carry OldNum=NewNum=0) inherit the position
//     of the nearest preceding non-divider line in the same file. This keeps
//     middle/trailing dividers (reachable via mouse-click in the diff) from
//     short-circuiting forward navigation back to the file-level annotation.
//     A leading divider with no prior non-divider line falls back to line=-1
//     so the file-level annotation remains reachable from the top.
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
		return m.dividerAnnotKey(file)
	}
	line := m.diffLineNum(dl)
	typ := string(dl.ChangeType)
	return cursorAnnotKey{file: file, line: line, typ: typ, onAnnot: m.store.Has(file, line, typ)}
}

// dividerAnnotKey returns the cursor's annotation-space key when the cursor
// sits on a ChangeDivider row. Walks back to the nearest preceding
// non-divider line and uses its line number, so forward navigation from a
// middle/trailing divider reaches the next annotation strictly after the
// divider's logical position rather than re-entering at the file-level
// annotation. A leading divider (no prior non-divider line) falls back to
// line=-1 so the file-level annotation stays reachable.
func (m Model) dividerAnnotKey(file string) cursorAnnotKey {
	for i := m.nav.diffCursor - 1; i >= 0; i-- {
		prev := m.file.lines[i]
		if prev.ChangeType == diff.ChangeDivider {
			continue
		}
		return cursorAnnotKey{file: file, line: m.diffLineNum(prev), typ: string(prev.ChangeType), onAnnot: false}
	}
	return cursorAnnotKey{file: file, line: -1, typ: "", onAnnot: false}
}

// startingFlatIndex returns the index in flat from which the walker should
// begin attempting jumps. Forward: first index strictly after the cursor,
// or one past an exact-match index. Backward: mirror. Returns an
// out-of-range index (-1 or len(flat)) when the cursor is at the
// corresponding boundary — the loop exits immediately in that case.
func startingFlatIndex(flat []annotation.Annotation, cur cursorAnnotKey, forward bool) int {
	if idx, ok := exactAnnotIndex(flat, cur); ok {
		if forward {
			return idx + 1
		}
		return idx - 1
	}
	insIdx := annotInsertionPoint(flat, cur)
	if forward {
		return insIdx
	}
	return insIdx - 1
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
