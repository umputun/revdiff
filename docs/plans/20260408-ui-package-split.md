# Split app/ui/ Package Large Files

## Overview
- Refactor `app/ui/` to keep code files under 500 lines and test files under 1000 lines
- Pure file reorganization — all methods stay on `Model` struct, no new types or interfaces
- No behavioral changes; tests must pass identically before and after each step

## Context
- `model.go` (1809 lines) — god file with struct, init, update, view, status bar, help overlay, key dispatch, loaders, toggles, ANSI helpers
- `model_test.go` (8252 lines) — ~307 test functions covering all concerns
- `diffview.go` (849 lines) — diff rendering mixed with cursor movement/navigation and cross-file hunk nav
- `collapsed.go` (456 lines) — fine, stays as-is
- `collapsed_test.go` (1716 lines) — needs split into two logical halves
- `filetree.go` (460 lines) — fine, stays as-is
- Other files (`annotate.go`, `annotlist.go`, `mdtoc.go`, `search.go`, `styles.go`) stay untouched

## Development Approach
- **testing approach**: Regular — this is pure file moves, no new code
- complete each task fully before moving to the next
- run `make test` and `make lint` after every task
- **CRITICAL: no behavioral changes** — only move methods between files
- **CRITICAL: all tests must pass before starting next task**

## Solution Overview
Split `Model` methods across files by concern area. Each file contains related methods on the same struct.

**CRITICAL: This is a strict move-only operation.** Every method must be moved verbatim — no renaming, no refactoring, no signature changes, no logic tweaks. After each move, verify the code at the destination is byte-identical to the source. Run `make test && make lint` after every individual task to catch any breakage immediately. If any test fails, the move introduced a problem — revert and investigate before proceeding.

**Code files after refactor:**
| File | Content | Target |
|---|---|---|
| `model.go` | struct, NewModel, Init, Update, handleKey, toggles | ~500 |
| `loaders.go` | loadFiles, loadFileDiff, loadBlame, handleFilesLoaded, handleFileLoaded, handleBlameLoaded, computeBlameAuthorLen, filterOnly, computeFileStats, fileStatsText, skipInitialDividers | ~300 |
| `view.go` | View(), status bar, help overlay, overlay compositor, ANSI helpers, modal handlers | ~500 |
| `diffview.go` | diff rendering, gutters, line styling, annotations in diff, search highlights | ~450 |
| `diffnav.go` | nav dispatchers (handleDiffNav, handleTreeNav, handleTOCNav), cursor movement, hunk nav (incl. cross-file handleHunkNav, applyPendingHunkJump), viewport sync, horizontal scroll | ~500 |
| `collapsed.go` | collapsed mode logic (unchanged) | ~456 |
| _unchanged:_ | `filetree.go`, `annotate.go`, `annotlist.go`, `mdtoc.go`, `search.go`, `styles.go` | |

**Test files after refactor:**
| File | Content | Target |
|---|---|---|
| `model_test.go` | core/init, layout, tree nav, blame, filter, loaders, keymap, config | ~900 |
| `view_test.go` | status bar, help overlay, mode icons, single-file mode, TOC | ~1000 |
| `model_annotate_test.go` | annotation CRUD + discard/quit (named to avoid `annotate.go` slot collision) | ~1000 |
| `model_search_test.go` | search input, navigation, highlighting, edge cases (named to avoid `search.go` slot collision) | ~1000 |
| `diffnav_test.go` | cursor/scroll/hunk/wrap navigation, cross-file hunk nav, pending hunk jump | ~900 |
| `collapsed_test.go` | collapsed mode logic tests | ~900 |
| `collapsed_render_test.go` | collapsed rendering, wrap, line numbers | ~800 |
| `diffview_test.go` | existing rendering tests (unchanged) | ~116 |

## Implementation Steps

### Task 1: Extract diffnav.go from diffview.go and model.go

Split cursor movement, navigation methods, and nav dispatchers into new `diffnav.go`.

**Files:**
- Create: `app/ui/diffnav.go`
- Modify: `app/ui/diffview.go`
- Modify: `app/ui/model.go`

- [x] create `app/ui/diffnav.go` with package header and imports
- [x] move from `diffview.go` — cursor movement: `moveDiffCursorDown`, `moveDiffCursorUp`, `moveDiffCursorPageDown`, `moveDiffCursorPageUp`, `moveDiffCursorHalfPageDown`, `moveDiffCursorHalfPageUp`, `moveDiffCursorToStart`, `moveDiffCursorToEnd`
- [x] move from `diffview.go` — viewport sync: `syncViewportToCursor`, `centerViewportOnCursor`, `topAlignViewportOnCursor`
- [x] move from `diffview.go` — hunk navigation: `findHunks`, `currentHunk`, `moveToNextHunk`, `moveToPrevHunk`, `handleHunkNav`
- [x] move from `diffview.go` — `handleHorizontalScroll`, `cursorDiffLine`
- [x] move from `model.go` — `applyPendingHunkJump`
- [x] move from `model.go` — nav dispatchers: `handleDiffNav`, `handleTreeNav`, `handleTOCNav`, `handleSwitchToTree`, `treePageSize`, `paneHeight`
- [x] move from `model.go` — TOC nav helpers: `jumpTOCEntry`, `syncTOCCursorToActive`, `syncDiffToTOCCursor`, `syncTOCActiveSection`
- [x] verify `diffview.go` is under 500 lines and `diffnav.go` is under 500 lines
- [x] run `make test` — must pass
- [x] run `make lint` — must pass

### Task 2: Extract view.go from model.go

Move View(), status bar, help overlay, ANSI helpers, and modal handlers out of `model.go` into new `view.go`.

**Files:**
- Create: `app/ui/view.go`
- Modify: `app/ui/model.go`

- [x] create `app/ui/view.go` with package header and imports
- [x] move `View()` method
- [x] move status bar methods: `statusBarText`, `hunkSegment`, `lineNumberSegment`, `searchSegment`, `searchBarText`, `joinStatusSections`, `statusModeIcons`, `statusSegmentsNoSearch`, `statusSegmentsMinimal`
- [x] move help overlay methods: `helpOverlay`, `writeTOCHelpSection`, `overlayCenter`, `formatKeysForHelp`, `displayKeyName`, `handleHelpKey`
- [x] move ANSI helpers: `padContentBg`, `ansiColor`, `ansiFg`, `ansiBg`
- [x] move modal handlers: `handleEnterKey`, `handleEscKey`, `handleConfirmDiscardKey`, `handleDiscardQuit`, `handleFileAnnotateKey`, `handleFilterToggle`, `handleMarkReviewed`, `handleFileOrSearchNav`, `annotatedFiles`
- [x] verify `view.go` is under 500 lines (if over, split modal handlers into separate `handlers.go`)
- [x] run `make test` — must pass
- [x] run `make lint` — must pass

### Task 3: Extract loaders.go from model.go

Move file/blame loading and data preparation methods into new `loaders.go`.

**Files:**
- Create: `app/ui/loaders.go`
- Modify: `app/ui/model.go`

- [x] create `app/ui/loaders.go` with package header and imports
- [x] move async loaders: `loadFiles`, `loadFileDiff`, `loadBlame`, `loadSelectedIfChanged`
- [x] move loaded-message handlers: `handleFilesLoaded`, `handleFileLoaded`, `handleBlameLoaded`
- [x] move data helpers: `computeBlameAuthorLen`, `filterOnly`, `computeFileStats`, `fileStatsText`, `skipInitialDividers`
- [x] verify `model.go` is under 500 lines and `loaders.go` is under 500 lines
- [x] run `make test` — must pass
- [x] run `make lint` — must pass

### Task 4: Split model_test.go — extract model_search_test.go

Move all search-related tests (~46 functions) to dedicated file.

**Files:**
- Create: `app/ui/model_search_test.go`
- Modify: `app/ui/model_test.go`

- [x] create `app/ui/model_search_test.go` with package header, imports, and shared test helpers if needed
- [x] move all `TestModel_Search*` and `TestModel_*Search*` test functions
- [x] move search-related test helpers if any exist
- [x] verify no duplicate imports or missing helpers
- [x] run `make test` — must pass
- [x] run `make lint` — must pass

### Task 5: Split model_test.go — extract model_annotate_test.go

Move all annotation and discard/quit tests (~67+14 functions) to dedicated file.
Named `model_annotate_test.go` (not `annotate_test.go`) to avoid collision with `annotate.go`'s test slot.

**Files:**
- Create: `app/ui/model_annotate_test.go`
- Modify: `app/ui/model_test.go`

- [x] create `app/ui/model_annotate_test.go` with package header, imports, and shared test helpers
- [x] move all `TestModel_Annotate*`, `TestModel_DeleteAnnotation*`, `TestModel_RenderDiffWithAnnot*` test functions
- [x] move all `TestModel_*Discard*`, `TestModel_Quit*` test functions
- [x] move annotation/discard-related test helpers if any exist
- [x] verify no duplicate imports or missing helpers
- [x] run `make test` — must pass
- [x] run `make lint` — must pass

### Task 6: Split model_test.go — extract view_test.go

Move status bar, help overlay, single-file, TOC, and mode icon tests to dedicated file.

**Files:**
- Create: `app/ui/view_test.go`
- Modify: `app/ui/model_test.go`

- [ ] create `app/ui/view_test.go` with package header, imports, and shared test helpers
- [ ] move all `TestModel_StatusBar*`, `TestModel_StatusMode*`, `TestModel_ReviewedStatus*`, `TestModel_ReviewedModeIcon*` test functions
- [ ] move all `TestModel_HelpOverlay*` test functions
- [ ] move all `TestModel_SingleFile*` test functions
- [ ] move all `TestModel_*TOC*`, `TestModel_*Markdown*`, `TestModel_FileLoaded*TOC*` test functions
- [ ] verify no duplicate imports or missing helpers
- [ ] run `make test` — must pass
- [ ] run `make lint` — must pass

### Task 7: Split model_test.go — extract diffnav_test.go

Move cursor, scroll, hunk, and wrap navigation tests to dedicated file.

**Files:**
- Create: `app/ui/diffnav_test.go`
- Modify: `app/ui/model_test.go`

- [ ] create `app/ui/diffnav_test.go` with package header, imports, and shared test helpers
- [ ] move all `TestModel_DiffScroll*`, `TestModel_DiffCursor*` test functions
- [ ] move all `TestModel_*Hunk*` navigation test functions (not collapsed hunk tests), including `TestModel_HunkNav_*`
- [ ] move all `TestModel_PendingHunkJump_*` test functions
- [ ] move all `TestModel_Wrap*` test functions (not collapsed wrap tests)
- [ ] move `TestModel_NextPrev*`, `TestModel_PageDown*`, `TestModel_PageUp*` if present
- [ ] verify remaining `model_test.go` is under 1000 lines
- [ ] run `make test` — must pass
- [ ] run `make lint` — must pass

### Task 8: Split collapsed_test.go

Split into logic tests and rendering/UI tests.

**Files:**
- Create: `app/ui/collapsed_render_test.go`
- Modify: `app/ui/collapsed_test.go`

- [ ] create `app/ui/collapsed_render_test.go` with package header and imports
- [ ] move rendering tests: `TestModel_CollapsedRender*`, `TestModel_CollapsedWrap*`, `TestModel_CollapsedRenderWithLineNumbers*`
- [ ] keep logic tests in `collapsed_test.go`: toggle, cursor movement, hunk expansion, modified set, delete-only, etc.
- [ ] verify both files are under 1000 lines
- [ ] run `make test` — must pass
- [ ] run `make lint` — must pass

### Task 9: Verify final state

- [ ] verify all code files under 500 lines: `wc -l app/ui/*.go | grep -v _test | sort -rn`
- [ ] verify all test files under 1000 lines: `wc -l app/ui/*_test.go | sort -rn`
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] verify no behavioral changes: confirm identical test count and 0 failures vs baseline

### Task 10: [Final] Update documentation

- [ ] update CLAUDE.md project structure section to reflect new file layout
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- browse `app/ui/` directory to confirm logical grouping feels natural
- spot-check that IDE navigation (go-to-definition) still works correctly across split files
