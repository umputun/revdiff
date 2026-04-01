# TUI Improvements - Navigation, Views, and Annotations

## Overview

A set of UX improvements to the revdiff TUI covering key bindings, navigation, views, and annotation capabilities:

1. Tab switches panes (l/h stay as alternatives)
2. `f` toggles annotation filter (replaces tab), shown only when annotations exist
3. `[` / `]` jump between change chunks, status shows "chunk 2/5"
4. `v` toggles simplified/unified diff view with annotation support
5. `·` dot prefix for directories instead of `▾`
6. `A` for file-level annotations (stored with Line: 0), displayed at top of diff
7. Enter in diff pane starts annotation (same as `a`)

## Context

- Key handling: `ui/model.go` handleKey, handleTreeNav, handleDiffNav
- Diff rendering: `ui/diffview.go` renderDiff, cursor movement
- Annotations: `ui/annotate.go` startAnnotation, saveAnnotation, deleteAnnotation
- File tree: `ui/filetree.go` render, buildEntries
- Store: `annotation/store.go` Add/Delete/Get/FormatOutput
- Status bar: `ui/model.go` View()
- Styles: `ui/styles.go` defaultStyles

## Solution Overview

- Task 1 changes the folder icon in filetree.go render()
- Tasks 2-3 are key rebinding changes in model.go handleKey (tab→pane switch, f→filter, enter in diff→annotate)
- Tasks 4a/4b extend the annotation store and UI for file-level (Line: 0) annotations
- Task 5 adds chunk detection and navigation in diffview.go
- Task 6 adds simplified diff view toggle with cursor-skipping approach (no index mapping)
- Status bar updates are spread across tasks as each feature adds its hints

**Key design decisions:**
- Simplified view keeps `diffCursor` on original `diffLines` indices; rendering skips non-visible lines and cursor movement skips hidden lines. This avoids index mapping complexity.
- `findChunks()` always operates on original `diffLines` regardless of view mode.
- File-level annotations render as a special line at diff top; `d` on that line deletes them.

## Development Approach

- **testing approach**: regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- run tests after each change

## Testing Strategy

- **unit tests**: required for every task
- test key bindings via Update() with tea.KeyMsg
- test rendering output for expected content
- test navigation state changes
- test annotation store for new file-level annotation type

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with + prefix
- document issues/blockers with warning prefix

## Implementation Steps

### Task 1: Replace folder icon with dot prefix

**Files:**
- Modify: `ui/filetree.go`
- Modify: `ui/filetree_test.go`

- [x] change `"▾ "` to `"· "` in filetree.go render()
- [x] update tests in filetree_test.go that assert on `▾` to use `·`
- [x] run `go test ./ui/` - must pass before next task

### Task 2: Remap Tab to pane switching, f for filter

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] in handleKey global keys: change `tab` from toggleFilter to pane switching (toggle focus between paneTree/paneDiff, only switch to paneDiff if currFile != "")
- [x] add `f` key in global keys section to call tree.toggleFilter(annotatedFiles()) + ensureVisible
- [x] update status bar in View(): replace `[tab] filter` with `[tab] switch` in tree pane hints
- [x] add `[tab] files` hint to diff pane status bar
- [x] add `[f] filter` to status bar hints (both panes), but only when len(annotatedFiles()) > 0
- [x] update existing tests that check tab behavior to expect pane switching
- [x] write tests for tab pane switching (tree->diff, diff->tree, tree->tree when no file loaded)
- [x] write tests for f filter toggle (global, works from both panes)
- [x] write tests for conditional filter hint in status bar (with/without annotations)
- [x] run `go test ./ui/` - must pass before next task

### Task 3: Enter in diff pane starts annotation

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

NOTE: the global `enter` handler in handleKey returns before reaching handleDiffNav. The fix must be in the global section: when focus == paneDiff, call startAnnotation() instead of returning nil.

- [x] in handleKey global `enter` case: add branch for `m.focus == paneDiff` that calls startAnnotation() and re-renders viewport
- [x] update diff pane status bar hints to show `[enter/a] annotate`
- [x] write tests for Enter key in diff pane triggering annotation mode
- [x] run `go test ./ui/` - must pass before next task

### Task 4a: Store support for file-level annotations

**Files:**
- Modify: `annotation/store.go`
- Modify: `annotation/store_test.go`

- [ ] verify Add/Delete/Get work with Line=0 and Type="" for file-level annotations (no special-casing blocks this)
- [ ] in FormatOutput: render file-level annotations (Line=0) as `## file (file-level)` before line-specific annotations for each file
- [ ] write tests for store Add/Delete/Get with Line=0 file-level annotations
- [ ] write tests for FormatOutput with mixed file-level and line-level annotations
- [ ] run `go test ./annotation/` - must pass before next task

### Task 4b: File-level annotation UI

**Files:**
- Modify: `ui/annotate.go`
- Modify: `ui/model.go`
- Modify: `ui/diffview.go`
- Modify: `ui/model_test.go`

- [ ] add `fileAnnotating bool` field to Model to distinguish file-level vs line-level annotation mode
- [ ] add `startFileAnnotation()` method in annotate.go: creates textinput, sets fileAnnotating=true and annotating=true, pre-fills if existing file-level annotation exists
- [ ] in saveAnnotation: when fileAnnotating, store with Line=0, Type=""
- [ ] in cancelAnnotation: reset fileAnnotating
- [ ] in handleKey global keys: add `A` (shift+a) to call startFileAnnotation() when currFile != ""
- [ ] in renderDiff: render file-level annotations at the top of the diff view as a special selectable line, using `"💬 file: "` prefix with AnnotationLine style
- [ ] enable `d` to delete file-level annotation when cursor is on the file-level annotation line at the top
- [ ] in cursorLineHasAnnotation: exclude file-level annotations from regular per-line checks
- [ ] update status bar: show `[A] file note` hint when currFile != "" (both panes)
- [ ] write tests for A key triggering file-level annotation mode
- [ ] write tests for file-level annotation rendering at top of diff
- [ ] write tests for deleting file-level annotation via d on the special line
- [ ] run `go test ./ui/` - must pass before next task

### Task 5: Chunk navigation with [ / ]

**Files:**
- Modify: `ui/diffview.go`
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

NOTE: `findChunks()` always operates on original `diffLines` regardless of view mode (chunks are properties of the diff, not the view).

- [ ] add `findChunks()` method that scans diffLines and returns a slice of chunk start indices (first line of each contiguous +/- group)
- [ ] add `currentChunk()` method that returns (chunkIndex, totalChunks) based on diffCursor position relative to chunk ranges
- [ ] add `moveToNextChunk()` method: finds next chunk start after current cursor, moves cursor there, syncs viewport
- [ ] add `moveToPrevChunk()` method: finds previous chunk start before current cursor, moves cursor there, syncs viewport
- [ ] in handleDiffNav: add `]` to call moveToNextChunk(), `[` to call moveToPrevChunk()
- [ ] in View() status bar for diff pane: append chunk indicator "chunk 2/5" when chunks > 0
- [ ] update diff pane status hints to include `[/] chunks`
- [ ] write tests for findChunks with various diff patterns (single chunk, multiple chunks, no changes, all changes)
- [ ] write tests for currentChunk returning correct index
- [ ] write tests for moveToNextChunk/moveToPrevChunk navigation
- [ ] write tests for chunk indicator in status bar
- [ ] run `go test ./ui/` - must pass before next task

### Task 6: Simplified/unified diff view with v toggle

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/diffview.go`
- Modify: `ui/model_test.go`

**Approach**: keep `diffCursor` as index into original `diffLines`. The simplified view only changes what is rendered and how cursor movement skips non-visible lines. No index mapping needed.

- [ ] add `simplifiedView bool` field to Model struct
- [ ] add `visibleInSimplified()` method: returns a `[]bool` (or set) marking which diffLines indices are visible — changed lines plus ~3 context lines around each change group, plus divider lines between groups
- [ ] add `renderSimplifiedDiff()` method: iterates diffLines, skips non-visible lines, renders visible ones with same styling as renderDiff, inserts dividers between non-adjacent visible groups
- [ ] adapt `moveDiffCursorDown/Up` to skip non-visible lines when simplifiedView is true
- [ ] adapt `moveDiffCursorPageDown/Up` to account for simplified view (page size based on visible lines)
- [ ] adapt `moveDiffCursorToStart/End` to find first/last visible non-divider line
- [ ] adapt `cursorViewportY` to count only visible lines when computing cursor position
- [ ] in handleDiffNav: add `v` key to toggle simplifiedView, reset viewport position, re-render
- [ ] in renderDiff callers (handleFileLoaded, syncViewportToCursor, etc.): dispatch to renderSimplifiedDiff() when simplifiedView is true
- [ ] annotations continue to work unchanged — annotationMap lookup uses original line numbers, store operations use original indices
- [ ] chunk navigation (Task 5) continues to work — findChunks uses original diffLines, cursor jumps to original indices which are visible in simplified view
- [ ] update status bar: show `[v] simple` / `[v] full` toggle hint depending on current mode
- [ ] write tests for visibleInSimplified with various change patterns
- [ ] write tests for renderSimplifiedDiff output (correct context, dividers)
- [ ] write tests for v key toggling between views
- [ ] write tests for cursor navigation in simplified view (skip hidden lines)
- [ ] write tests for annotation creation/display in simplified view
- [ ] write test for toggling view with existing annotations (annotations preserved)
- [ ] run `go test ./ui/` - must pass before next task

### Task 7: Verify acceptance criteria

- [ ] verify all 7 improvements are implemented and working together
- [ ] verify tab switches panes, l/h still work as alternatives
- [ ] verify f toggles filter, hint hidden when no annotations
- [ ] verify [ / ] navigate chunks with correct status indicator
- [ ] verify v toggles simplified view with working annotations
- [ ] verify · dot prefix on directories
- [ ] verify A creates file-level annotations displayed at diff top
- [ ] verify Enter in diff pane starts annotation
- [ ] run full test suite: `go test ./...`
- [ ] run linter: `golangci-lint run`
- [ ] verify test coverage meets 80%+

### Task 8: Update documentation

- [ ] update README.md key bindings section with new keys (tab, f, [, ], v, A, Enter)
- [ ] update CLAUDE.md if new patterns discovered
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test all key bindings in a real git repo with changes
- verify simplified view renders correctly with various diff sizes
- verify file-level annotations appear in stdout output
- test chunk navigation with files that have many scattered changes
