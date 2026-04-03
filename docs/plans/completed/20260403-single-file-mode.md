# Single-File Mode

## Overview
When a diff contains exactly one file, automatically hide the file tree pane and give the full terminal width to the diff view. This eliminates the unnecessary tree panel and pane-switching overhead for single-file reviews.

## Context
- **Key files:** `ui/model.go` (Model struct, handleFilesLoaded, View, handleKey, togglePane, handleDiffNav, handleResize), `ui/diffview.go` (diffContentWidth), `ui/model_test.go`
- **Detection point:** `handleFilesLoaded` at line 423 — where the file list arrives
- **Rendering:** `View()` at lines 492-528 — tree pane rendered at lines 499-513, joined with diff at line 528
- **Width calculation:** `diffContentWidth()` in diffview.go:602 uses `m.width - m.treeWidth - 4 - 1`
- **Pane switching:** `togglePane()` at line 280, `h` key in `handleDiffNav` at line 352

## Solution Overview
- Add `singleFile bool` field to Model, set in `handleFilesLoaded` when `len(files) == 1`
- In single-file mode: skip tree pane rendering, diff pane uses full width (`m.width - 2`), focus stays on `paneDiff`
- Key no-ops in single-file mode: `tab`, `h`, `l`, `n`/`p` (file nav), `f` (filter)
- `n`/`N` still work for search navigation when search is active
- `diffContentWidth()` returns `m.width - 3` (diff borders + cursor bar only)
- No CLI flag — purely automatic based on file count

## Technical Details

### Width calculations in single-file mode
- Tree pane: not rendered, `treeWidth = 0`
- Diff pane in `View()`: `Width(m.width - 2)` (only diff pane borders, 1 left + 1 right)
- `diffContentWidth()`: `m.width - 2 - 1 = m.width - 3` (diff borders + cursor bar)
- Viewport in `handleResize`: `diffWidth = m.width - 2`

### Key handling
- `tab` → no-op (guard in `togglePane`)
- `h` in diff pane → no-op (guard in `handleDiffNav`)
- `n`/`p` → file nav no-op, but `n`/`N` search nav still works via `handleFileOrSearchNav`
- `f` (filter) → no-op
- All other keys work normally

## Development Approach
- **Testing approach:** regular (code first, then tests)
- Complete each task fully before moving to the next
- Run tests after each change
- Maintain backward compatibility (multi-file mode unchanged)

## Implementation Steps

### Task 1: Add singleFile field and detection

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] add `singleFile bool` field to Model struct
- [x] in `handleFilesLoaded`: set `m.singleFile = len(msg.files) == 1` and `m.focus = paneDiff` when single file
- [x] write test: single file sets `singleFile = true` and `focus = paneDiff`
- [x] write test: multiple files keeps `singleFile = false`
- [x] run `make test` — must pass before task 2

### Task 2: Adjust View rendering for single-file mode

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/diffview.go`
- Modify: `ui/model_test.go`

- [x] in `View()`: when `m.singleFile`, skip tree pane rendering and set diff pane `Width(m.width - 2)`
- [x] in `handleResize`: when `m.singleFile`, set `m.treeWidth = 0` and `diffWidth = m.width - 2`
- [x] in `diffContentWidth()`: when `m.singleFile`, return `max(10, m.width-3)`
- [x] write test: `View()` output in single-file mode does not contain tree pane content
- [x] write test: `diffContentWidth()` returns correct width in single-file mode
- [x] run `make test` — must pass before task 3

### Task 3: Disable pane-switching keys in single-file mode

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] in `togglePane()`: early return when `m.singleFile`
- [x] in `handleDiffNav`: skip `h` key (switch to tree) when `m.singleFile`
- [x] in `handleKey`: skip `f` (filter) when `m.singleFile`
- [x] in `handleFileOrSearchNav` or `handleKey`: `n`/`p` file nav no-op when `m.singleFile` (search nav still works)
- [x] write tests: tab, h, f keys are no-ops in single-file mode
- [x] write test: `n` still navigates search matches in single-file mode
- [x] run `make test` — must pass before task 4

### Task 4: Verify acceptance criteria
- [x] verify single-file diff shows no tree pane
- [x] verify diff pane uses full terminal width
- [x] verify focus starts on diff pane
- [x] verify pane-switching keys are no-ops
- [x] verify search, annotations, wrap, collapsed mode all work normally
- [x] verify multi-file mode is unchanged
- [x] run full test suite: `make test`
- [x] run linter: `make lint`

### Task 5: [Final] Update documentation
- [x] update README.md to mention single-file auto-detection
- [x] update CLAUDE.md if new patterns discovered
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test with `revdiff HEAD~1` on a commit that changes exactly 1 file
- test with `revdiff HEAD~1` on a commit that changes multiple files
- test resizing terminal in single-file mode
- test all keyboard shortcuts in single-file mode
