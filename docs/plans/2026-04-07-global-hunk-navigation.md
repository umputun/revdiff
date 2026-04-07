# Global Hunk Navigation with Cross-File Jump for [ and ]

## Overview
Make `[` / `]` (prev/next hunk) work regardless of which pane has focus. When pressing `]` past the last hunk of a file, automatically navigate to the next file and land on its first hunk. When pressing `[` before the first hunk, navigate to the previous file and land on its last hunk. In single-file mode these remain intra-file only.

## Context
- Files involved: `app/ui/model.go`, `app/ui/diffview.go`, `app/ui/filetree.go`, `app/ui/model_test.go`
- Related patterns: `pendingAnnotJump` field for deferred post-load cursor positioning; `handleFileOrSearchNav` for file tree navigation; `loadSelectedIfChanged` for triggering async file load
- Dependencies: none

## Development Approach
- **Testing approach**: TDD — write failing tests first, then implement
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Implementation Steps

### Task 1: Add `hasNextFile` / `hasPrevFile` helpers to fileTree

**Files:**
- Modify: `app/ui/filetree.go`

- [x] Add `hasNextFile() bool` that returns true if a file entry exists with index > current cursor (no wrap)
- [x] Add `hasPrevFile() bool` that returns true if a file entry exists with index < current cursor (no wrap)
- [x] Write unit tests for both helpers covering: first file, middle file, last file, single file
- [x] Run `make test` — must pass before task 2

### Task 2: Add `pendingHunkJump` field and update Model

**Files:**
- Modify: `app/ui/model.go`

- [x] Add `pendingHunkJump *bool` field to Model struct (true = land on first hunk, false = land on last hunk)
- [x] In `handleFileLoaded`, after `skipInitialDividers()`, check `pendingHunkJump`: if true, set `m.diffCursor = -1` then call `m.moveToNextHunk()`; if false, set `m.diffCursor = len(m.diffLines)` then call `m.moveToPrevHunk()`; clear the field in either branch then return
- [x] Clear `pendingHunkJump` in all places where `pendingAnnotJump` is cleared (tree nav, filter, file nav, annotate)
- [x] Write tests for `handleFileLoaded` with `pendingHunkJump` set: verify cursor lands on first/last hunk of the loaded file
- [x] Run `make test` — must pass before task 3

### Task 3: Implement `handleHunkNav` and wire global key dispatch

**Files:**
- Modify: `app/ui/diffview.go`, `app/ui/model.go`

- [ ] In `diffview.go`, add `handleHunkNav(forward bool) (tea.Model, tea.Cmd)` method on Model:
  - If `m.currFile == ""`: return no-op
  - Set `m.focus = paneDiff`
  - Record `prevCursor := m.diffCursor`
  - Call `m.moveToNextHunk()` (forward) or `m.moveToPrevHunk()` (backward)
  - If cursor did not move and `!m.singleFile`:
    - forward: if `m.tree.hasNextFile()`, set `pendingHunkJump = &true`, call `m.tree.nextFile()`, return `m.loadSelectedIfChanged()`
    - backward: if `m.tree.hasPrevFile()`, set `pendingHunkJump = &false`, call `m.tree.prevFile()`, return `m.loadSelectedIfChanged()`
  - Call `m.syncTOCActiveSection()` and return
- [ ] In `model.go` `handleKey`, move `ActionNextHunk` and `ActionPrevHunk` cases from `handleDiffNav` to the global action switch (before pane-specific dispatch), delegating to `m.handleHunkNav(true/false)`
- [ ] Remove `ActionNextHunk` / `ActionPrevHunk` from `handleDiffNav` switch
- [ ] Write tests:
  - `]` from tree pane switches focus to diff and jumps to next hunk
  - `[` from tree pane switches focus to diff and jumps to prev hunk
  - `]` at last hunk navigates to next file and lands on its first hunk
  - `[` at first hunk navigates to prev file and lands on its last hunk
  - `]` at last hunk with no next file: no-op
  - `[` at first hunk with no prev file: no-op
  - single-file mode: `]`/`[` do not cross to other files
- [ ] Run `make test` — must pass before task 4

### Task 4: Verify acceptance criteria

- [ ] Run `make test` (full test suite with race detector)
- [ ] Run `make lint`
- [ ] Manually verify: navigate a multi-file diff using only `[`/`]`, confirm seamless traversal through all hunks across all files from either pane

### Task 5: Update documentation

- [ ] Update `site/docs.html` and `README.md` to document the cross-file hunk navigation behavior for `[` and `]`
- [ ] Move this plan to `docs/plans/completed/`
