package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// buildAnnotListItems builds a flat list of all annotations across all files.
// items are ordered by file name then line number, as returned by the store.
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
			File: a.File, ChangeType: a.Type, Comment: a.Comment, Line: a.Line,
			Target: overlay.AnnotationTarget{File: a.File, ChangeType: a.Type, Line: a.Line},
		}
	}
	return overlay.AnnotListSpec{Items: items}
}

// jumpToAnnotationTarget jumps to an annotation target returned by the overlay manager.
func (m Model) jumpToAnnotationTarget(target *overlay.AnnotationTarget) (tea.Model, tea.Cmd) {
	if target == nil {
		return m, nil
	}
	a := annotation.Annotation{File: target.File, Line: target.Line, Type: target.ChangeType}

	if a.File == m.currFile {
		m.positionOnAnnotation(a)
		return m, nil
	}

	m.pendingAnnotJump = &a
	if !m.singleFile {
		if !m.tree.SelectByPath(a.File) {
			m.pendingAnnotJump = nil
			return m, nil
		}
	}
	return m.loadSelectedIfChanged()
}

// positionOnAnnotation moves the cursor to the given annotation's line, re-renders, and centers the viewport.
// in collapsed mode, expands the hunk containing the target line so removed lines are visible.
func (m *Model) positionOnAnnotation(a annotation.Annotation) {
	if a.Line == 0 {
		m.diffCursor = -1
	} else {
		idx := m.findDiffLineIndex(a.Line, a.Type)
		if idx >= 0 {
			m.diffCursor = idx
			m.ensureHunkExpanded(idx)
		}
	}
	m.focus = paneDiff
	m.syncTOCActiveSection()
	m.viewport.SetContent(m.renderDiff())
	m.centerViewportOnCursor()
}

// ensureHunkExpanded expands the hunk containing diffLines[idx] when collapsed mode is active.
// this ensures the target line is visible after a jump (e.g., annotation on a removed line).
// also expands delete-only placeholder hunks where the first line is "visible" as a synthetic
// placeholder but annotations are not rendered (renderCollapsedDiff skips them).
func (m *Model) ensureHunkExpanded(idx int) {
	if !m.collapsed.enabled {
		return
	}
	hunks := m.findHunks()
	if m.isCollapsedHidden(idx, hunks) || m.isDeleteOnlyPlaceholder(idx, hunks) {
		hunkStart := m.hunkStartFor(idx, hunks)
		if hunkStart >= 0 {
			m.collapsed.expandedHunks[hunkStart] = true
		}
	}
}

// findDiffLineIndex finds the index into diffLines matching the given line number and change type.
// uses diffLineNum() semantics: compares against OldNum for removes, NewNum for adds/context.
// returns -1 if not found.
func (m Model) findDiffLineIndex(line int, changeType string) int {
	for i, dl := range m.diffLines {
		if string(dl.ChangeType) != changeType {
			continue
		}
		if m.diffLineNum(dl) == line {
			return i
		}
	}
	return -1
}
