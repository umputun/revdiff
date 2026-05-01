package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// buildAnnotListItems builds a flat list of all annotations across all files.
// Items are ordered by file name then line number, as returned by the store
// (alphabetical via Store.Files, line-ascending via Store.Get with file-level
// Line=0 first within each file). This combined ordering is load-bearing: both
// the @ popup and the }/{ walker in annotnav.go iterate this exact sequence,
// so changes to Store.Files / Store.Get ordering must keep the two consumers
// in sync.
func (m *Model) buildAnnotListItems() []annotation.Annotation {
	files := m.store.Files()
	items := make([]annotation.Annotation, 0, m.store.Count())
	for _, f := range files {
		items = append(items, m.store.Get(f)...)
	}
	return items
}

// buildAnnotListSpec builds an overlay.AnnotListSpec from the annotation store.
func (m Model) buildAnnotListSpec() overlay.AnnotListSpec {
	annots := m.buildAnnotListItems()
	items := make([]overlay.AnnotationItem, len(annots))
	for i, a := range annots {
		items[i] = overlay.AnnotationItem{
			AnnotationTarget: overlay.AnnotationTarget{File: a.File, ChangeType: a.Type, Line: a.Line},
			Comment:          a.Comment,
		}
	}
	return overlay.AnnotListSpec{Items: items}
}

// jumpToAnnotationTarget jumps to an annotation target returned by the overlay
// manager. Existing entry point used by the @ popup and the mouse handler;
// preserves the original silent-fail-on-unreachable behavior.
func (m Model) jumpToAnnotationTarget(target *overlay.AnnotationTarget) (tea.Model, tea.Cmd) {
	model, cmd, _ := m.tryJumpToAnnotationTarget(target)
	return model, cmd
}

// tryJumpToAnnotationTarget attempts to jump and reports whether the jump
// was actually issued. ok=false is returned when the target cannot be reached:
// cross-file path is not present in the file tree (filtered, hidden by
// --include/--exclude, or untracked-with-toggle-off), or same-file
// non-file-level line is not in the loaded diff (e.g. compact mode shrank
// context away). Used by the }/{ navigator to skip non-jumpable targets and
// keep walking instead of getting trapped on a target that silently no-ops.
// Single-file mode always rejects cross-file targets — there is nowhere to go.
func (m Model) tryJumpToAnnotationTarget(target *overlay.AnnotationTarget) (tea.Model, tea.Cmd, bool) {
	if target == nil {
		return m, nil, false
	}
	a := annotation.Annotation{File: target.File, Line: target.Line, Type: target.ChangeType}

	if a.File == m.file.name {
		if a.Line != 0 && m.findDiffLineIndex(a.Line, a.Type) < 0 {
			return m, nil, false
		}
		m.positionOnAnnotation(a)
		return m, nil, true
	}

	if m.file.singleFile {
		return m, nil, false
	}
	if !m.tree.SelectByPath(a.File) {
		return m, nil, false
	}
	m.pendingAnnotJump = &a
	model, cmd := m.loadSelectedIfChanged()
	return model, cmd, true
}

// positionOnAnnotation moves the cursor to the given annotation's line, re-renders, and centers the viewport.
// In collapsed mode, expands the hunk containing the target line so removed lines are visible.
// For line-level annotations the cursor lands on the annotation comment sub-row (cursorOnAnnotation=true),
// matching what `j`/`k` navigation produces when stepping onto an annotated line. File-level annotations
// (Line=0) use diffCursor=-1 which already represents the annotation row directly. Without this flag the
// cursor would land on the diff line above the comment, leaving navigation visually one row off the target.
func (m *Model) positionOnAnnotation(a annotation.Annotation) {
	m.annot.cursorOnAnnotation = false
	if a.Line == 0 {
		m.nav.diffCursor = -1
	} else {
		idx := m.findDiffLineIndex(a.Line, a.Type)
		if idx >= 0 {
			m.nav.diffCursor = idx
			m.ensureHunkExpanded(idx)
			hunks := m.findHunks()
			if !m.isCollapsedHidden(idx, hunks) && !m.isDeleteOnlyPlaceholder(idx, hunks) {
				m.annot.cursorOnAnnotation = true
			}
		}
	}
	m.layout.focus = paneDiff
	m.syncTOCActiveSection()
	m.layout.viewport.SetContent(m.renderDiff())
	m.centerViewportOnCursor()
}

// ensureHunkExpanded expands the hunk containing diffLines[idx] when collapsed mode is active.
// this ensures the target line is visible after a jump (e.g., annotation on a removed line).
// also expands delete-only placeholder hunks where the first line is "visible" as a synthetic
// placeholder but annotations are not rendered (renderCollapsedDiff skips them).
func (m *Model) ensureHunkExpanded(idx int) {
	if !m.modes.collapsed.enabled {
		return
	}
	hunks := m.findHunks()
	if m.isCollapsedHidden(idx, hunks) || m.isDeleteOnlyPlaceholder(idx, hunks) {
		hunkStart := m.hunkStartFor(idx, hunks)
		if hunkStart >= 0 {
			m.modes.collapsed.expandedHunks[hunkStart] = true
		}
	}
}

// findDiffLineIndex finds the index into diffLines matching the given line number and change type.
// uses diffLineNum() semantics: compares against OldNum for removes, NewNum for adds/context.
// returns -1 if not found.
func (m Model) findDiffLineIndex(line int, changeType string) int {
	for i, dl := range m.file.lines {
		if string(dl.ChangeType) != changeType {
			continue
		}
		if m.diffLineNum(dl) == line {
			return i
		}
	}
	return -1
}
