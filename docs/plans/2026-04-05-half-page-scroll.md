# Fix ctrl+d/u to scroll half-page (vim convention)

## Overview
- `ctrl+d`/`ctrl+u` currently behave identically to `PgDn`/`PgUp` (full page scroll)
- Help overlay says "half-page down / up" but implementation is full page
- Fix: make `ctrl+d`/`ctrl+u` scroll by half a page in all three panes (diff, tree, TOC)
- `PgDn`/`PgUp` remain unchanged (full page)

## Context (from discovery)
- Files/components involved:
  - `ui/model.go:413-416` — tree pane: `ctrl+d`/`ctrl+u` shares case with `PgDn`/`PgUp`
  - `ui/model.go:438-445` — TOC pane: same shared dispatch
  - `ui/model.go:491-494` — diff pane: same shared dispatch, calls `moveDiffCursorPageDown()`/`moveDiffCursorPageUp()`
  - `ui/diffview.go:418-453` — `moveDiffCursorPageDown()`/`moveDiffCursorPageUp()` use `m.viewport.Height` as step
  - `ui/filetree.go:132-158` — `fileTree.pageDown(n)`/`pageUp(n)` accept a row count
  - `ui/model.go:1068-1069` — help overlay text (already says "half-page", which is correct after fix)
- Related patterns: `moveDiffCursorPageDown` loops calling `moveDiffCursorDown()` until visual distance >= `viewport.Height`
- Dependencies: none

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task

## Progress Tracking
- Mark completed items with `[x]` immediately when done

## Implementation Steps

### Task 1: Split ctrl+d/u from PgDn/PgUp in all panes
- [x] in `ui/model.go` `handleTreeNav` (line 413-416): split into separate cases — `PgDn`/`PgUp` call `pageDown(m.treePageSize())`, `ctrl+d`/`ctrl+u` call `pageDown(m.treePageSize()/2)`
- [x] in `ui/model.go` `handleTOCNav` (line 438-445): split into separate cases — `PgDn`/`PgUp` loop `treePageSize()` times, `ctrl+d`/`ctrl+u` loop `treePageSize()/2` times
- [x] in `ui/model.go` `handleDiffNav` (line 491-494): split into separate cases — `PgDn`/`PgUp` call `moveDiffCursorPageDown()`, `ctrl+d`/`ctrl+u` call new `moveDiffCursorHalfPageDown()`/`moveDiffCursorHalfPageUp()`
- [x] in `ui/diffview.go`: add `moveDiffCursorHalfPageDown()` and `moveDiffCursorHalfPageUp()` — same logic as page versions but using `m.viewport.Height/2` as the step
- [x] write tests for ctrl+d moving half page in diff pane (compare distance to full page)
- [x] write tests for ctrl+u moving half page in diff pane
- [x] write tests for ctrl+d/u in tree pane moving half page size
- [x] run tests — must pass before next task

### Task 2: Verify acceptance criteria
- [ ] verify ctrl+d/u moves half page in diff pane
- [ ] verify PgDn/PgUp still moves full page in diff pane
- [ ] verify ctrl+d/u moves half page in tree and TOC panes
- [ ] run full test suite (unit tests)
- [ ] run linter — all issues must be fixed

### Task 3: [Final] Update documentation
- [ ] verify help overlay text is correct (already says "half-page" — no change needed)
- [ ] update CLAUDE.md if new patterns discovered

## Technical Details

**Diff pane half-page methods** — clone `moveDiffCursorPageDown`/`Up` but replace `m.viewport.Height` with `m.viewport.Height/2`:

```go
func (m *Model) moveDiffCursorHalfPageDown() {
    halfPage := max(1, m.viewport.Height/2)
    startY := m.cursorViewportY()
    for {
        prev := m.diffCursor
        m.moveDiffCursorDown()
        if m.diffCursor == prev { break }
        if m.cursorViewportY()-startY >= halfPage { break }
    }
    m.syncViewportToCursor()
}
```

Note: half-page uses `syncViewportToCursor()` (keep cursor visible) not `SetYOffset` (page-scroll jump), since moving half a page should keep context visible rather than jumping.

**Tree/TOC panes** — just pass half the page size to existing `pageDown(n)`/`pageUp(n)` or loop half count.

## Post-Completion
**Manual verification**:
- Open revdiff on a long diff, press ctrl+d — should move ~half the viewport
- Press PgDn — should move a full viewport
- Compare the two visually
