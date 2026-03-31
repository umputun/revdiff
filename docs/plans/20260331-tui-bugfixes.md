# Fix TUI Bugs — Input Echo, Delete UX, Page Navigation

## Overview
- fix three TUI usability bugs discovered during manual testing
- annotation text input doesn't echo typed characters until Enter
- delete annotation shortcut is confusing — `[d]` always shows regardless of context
- page up/down keys don't work in diff viewport or file tree
- file tree lacks indentation for files under directories

## Context (from discovery)
- files involved: `ui/annotate.go`, `ui/model.go`, `ui/diffview.go`, `ui/filetree.go`
- test files: `ui/model_test.go`, `ui/filetree_test.go`
- root causes identified for all three bugs
- bubbletea viewport uses static `SetContent` — must re-render after each keystroke
- page keys are never matched in `handleDiffNav` or `handleTreeNav`

## Solution Overview
1. **input echo**: call `m.viewport.SetContent(m.renderDiff())` after every keystroke in annotation mode
2. **delete UX**: conditionally show `[d] delete` in status bar only when cursor is on an annotated line; add `cursorLineHasAnnotation()` helper method
3. **page navigation**: add `PgUp`/`PgDown`/`ctrl+u`/`ctrl+d` handling in both diff viewport and file tree

## Development Approach
- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Testing Strategy
- unit tests: test `Update()` with key messages, assert model state changes
- verify annotation input view updates on each keystroke
- verify status bar text changes based on cursor position
- verify page navigation moves cursor by page size

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with + prefix
- document issues/blockers with !! prefix

## Implementation Steps

### Task 1: Fix annotation text input not echoing characters

**Files:**
- Modify: `ui/annotate.go`
- Modify: `ui/model_test.go`

- [x] in `handleAnnotateKey` default branch (annotate.go ~line 103), add `m.viewport.SetContent(m.renderDiff())` after `m.annotateInput.Update(msg)` so viewport re-renders with live text input on each keystroke
- [x] write test: send multiple `tea.KeyMsg` characters while in annotation mode, assert `View()` output contains typed characters after each key
- [x] write test: verify text input is visible in viewport before pressing Enter
- [x] run `go test ./ui/` — must pass before task 2

### Task 2: Fix delete annotation UX — conditional status bar hint

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/annotate.go`
- Modify: `ui/model_test.go`

- [ ] add `cursorLineHasAnnotation() bool` method on Model — checks if current `diffCursor` line has an annotation in the store for current file
- [ ] in `View()` (model.go ~line 329), conditionally include `[d] delete` in diff pane status bar only when `cursorLineHasAnnotation()` returns true
- [ ] write test: status bar shows `[d] delete` when cursor is on annotated line
- [ ] write test: status bar hides `[d] delete` when cursor is on non-annotated line
- [ ] run `go test ./ui/` — must pass before task 3

### Task 3: Add page up/down navigation in diff viewport

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] in `handleDiffNav` (model.go ~line 194), add cases for `tea.KeyPgDown` and `ctrl+d` — move `diffCursor` down by `viewport.Height` lines, call `syncViewportToCursor()`
- [ ] add cases for `tea.KeyPgUp` and `ctrl+u` — move `diffCursor` up by `viewport.Height` lines, call `syncViewportToCursor()`
- [ ] add `Home`/`End` keys — move cursor to first/last diff line
- [ ] write test: PgDown moves cursor by page height
- [ ] write test: PgUp moves cursor by page height
- [ ] write test: Home/End move cursor to boundaries
- [ ] run `go test ./ui/` — must pass before task 4

### Task 4: Fix file tree indentation — files under directories need indent

**Files:**
- Modify: `ui/filetree.go`
- Modify: `ui/filetree_test.go`

- [ ] in filetree rendering, add at least one space indent for files nested under a directory entry
- [ ] write test: `View()` output shows indented file names under their directory
- [ ] run `go test ./ui/` — must pass before task 5

### Task 5: Add page up/down navigation in file tree

**Files:**
- Modify: `ui/filetree.go`
- Modify: `ui/model.go`
- Modify: `ui/filetree_test.go`

- [ ] in `handleTreeNav` (model.go ~line 180), add cases for `tea.KeyPgDown`/`tea.KeyPgUp` — move tree cursor by visible page height
- [ ] add `Home`/`End` keys — move to first/last file in tree
- [ ] write test: PgDown in tree moves cursor by page
- [ ] write test: PgUp in tree moves cursor by page
- [ ] write test: Home/End in tree move to boundaries
- [ ] run `go test ./...` — must pass before task 6

### Task 6: Verify all fixes

- [ ] manual test: type in annotation input, verify characters appear immediately
- [ ] manual test: verify `[d]` only shows on annotated lines
- [ ] manual test: file tree shows indented files under directories
- [ ] manual test: PgUp/PgDown works in diff and file tree
- [ ] manual test: ctrl+u/ctrl+d work for half-page scrolling
- [ ] run full test suite: `go test ./...`
- [ ] run linter: `golangci-lint run`
