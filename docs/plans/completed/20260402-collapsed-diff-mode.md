# Collapsed Diff Mode

## Overview
Add a "collapsed" diff view mode that shows the final text (post-change state) with color markers on changed lines, instead of the traditional expanded remove+add diff. Users toggle between modes with `v`, and expand/collapse individual hunks with `.` to see the traditional diff inline.

- **Problem**: expanded diff view with interleaved remove/add lines can be noisy when reviewing large changes. A collapsed view shows the end result with change indicators, letting reviewers focus on what the code looks like after changes.
- **Integration**: adds a new rendering path alongside the existing expanded diff. All existing features (annotations, hunk navigation, syntax highlighting) work in both modes.

## Context (from discovery)
- **Files involved**: `ui/styles.go`, `ui/model.go`, `ui/diffview.go`, `ui/annotate.go`, `cmd/revdiff/main.go`
- **Related patterns**: existing `findHunks()` returns hunk start indices into `diffLines`; `renderDiffLine()` dispatches on `ChangeType`; `cursorViewportY()` counts visual rows including annotation sub-lines
- **Dependencies**: annotations reference lines by `(lineNum, changeType)` — collapsed mode must preserve this mapping for annotation interop
- **Test patterns**: `model_test.go` uses `testModel()` helper, drives via `m.Update(msg)`, asserts struct fields directly

## Solution Overview

**Two-mode rendering**: `collapsed bool` on Model controls which render path is used. `v` toggles it. Default is expanded (current behavior).

**Collapsed view**:
- Shows only final text: context lines + added lines. Removed lines are hidden.
- Added lines get green markers (existing `Add` colors, gutter `+`)
- Modified lines (add paired with remove in same hunk) get amber/yellow markers (new `Modify` colors, gutter `~`)
- Context lines are unchanged

**Hunk expand/collapse**: `.` key toggles inline expansion of the hunk under cursor when in collapsed mode. Expanded hunks show full remove+add lines with existing styling. Multiple hunks can be expanded independently. Tracked via `expandedHunks map[int]bool`.

**Hunk pairing**: to distinguish "modified" from "pure add", analyze each hunk's composition. If a hunk contains both removes and adds, the adds are "modified". If a hunk contains only adds, they are "pure adds".

## Technical Details

### New model fields
- `collapsed bool` — current view mode (false = expanded, true = collapsed)
- `expandedHunks map[int]bool` — which hunks are inline-expanded in collapsed mode. Key = `diffLines` start index returned by `findHunks()` (e.g., if `findHunks()` returns `[5, 20, 45]`, then `expandedHunks[20] = true` means the hunk starting at `diffLines[20]` is expanded)

### New Colors fields
- `ModifyFg string` — modified line foreground (default: `#f5c542`, warm yellow)
- `ModifyBg string` — modified line background (default: `#3D2E00`, dark amber)

### New styles
- `LineModify lipgloss.Style` — non-highlighted modified line
- `LineModifyHighlight lipgloss.Style` — syntax-highlighted modified line (background only)

### Collapsed rendering flow
1. Call `findHunks()` to get hunk start indices (into `diffLines`)
2. Build a `modifiedLines map[int]bool` via `buildModifiedSet()` — uses `findHunks()` to get contiguous change groups; within each group, if both `ChangeRemove` and `ChangeAdd` lines exist, the add-line indices are marked as "modified"
3. For each `diffLines` index, determine which hunk it belongs to (find the largest hunk start ≤ index). Check `expandedHunks[hunkStart]` for expansion state.
4. Iterate `diffLines` (using original indices, so annotation lookups remain correct):
   - `ChangeRemove` lines: skip unless the line's hunk is expanded
   - `ChangeAdd` lines: render with modify or add style based on `modifiedLines`
   - `ChangeContext` lines: render normally
   - `ChangeDivider` lines: render normally
5. For expanded hunks: render all lines (remove + add) with existing expanded styling
6. Annotations on removed lines are only visible when their hunk is expanded (collapsed mode hides removed lines and their annotations together). Annotations on added/context lines render normally via `renderAnnotationOrInput` using the original `diffLines` index.

### Cursor and viewport in collapsed mode
- `diffCursor` still indexes into `diffLines`, but removed lines are skipped during cursor movement when not in an expanded hunk
- `cursorViewportY()` must account for hidden removed lines in collapsed mode
- Hunk navigation (`]`/`[`) works unchanged — hunks exist in both modes

### State reset on file switch
- `handleFileLoaded` resets `expandedHunks` to empty map (same as existing `scrollX` reset)
- `collapsed` persists across file switches (mode is a user preference, not per-file)

## Development Approach
- **Testing approach**: regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- Run tests after each change
- Maintain backward compatibility — expanded mode must remain unchanged

## Testing Strategy
- **Unit tests**: required for every task — test both expanded and collapsed rendering paths
- Key test scenarios: collapsed rendering (removes hidden, adds shown, modified vs pure-add distinction), hunk expand/collapse, cursor skip logic, viewport Y calculation in collapsed mode, `v` and `.` key handling, state reset on file switch

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add ModifyFg/ModifyBg colors and styles

**Files:**
- Modify: `ui/styles.go`
- Modify: `cmd/revdiff/main.go`
- Modify: `ui/styles_test.go`

- [x] add `ModifyFg` and `ModifyBg` fields to `Colors` struct in `ui/styles.go`
- [x] add `LineModify` and `LineModifyHighlight` fields to `styles` struct
- [x] wire `LineModify` and `LineModifyHighlight` in `newStyles()` (parallel to add/remove pattern)
- [x] wire them in `plainStyles()` as no-op styles
- [x] add `ModifyFg`/`ModifyBg` to `normalizeColors()`
- [x] add default values in `cmd/revdiff/main.go` options struct (`ModifyFg: #f5c542`, `ModifyBg: #3D2E00`)
- [x] write tests in `ui/styles_test.go` for normalize and style creation with modify colors
- [x] run `go test ./ui/...` — must pass before task 2

### Task 2: Add collapsed mode state and key handling

**Files:**
- Modify: `ui/model.go`

- [x] add `collapsed bool` field to Model struct
- [x] add `expandedHunks map[int]bool` field to Model struct
- [x] handle `v` key in `handleKey()` — toggle `collapsed`, reset `expandedHunks`, re-render
- [x] handle `.` key in `handleDiffNav()` — toggle current hunk in `expandedHunks` when collapsed (no-op in expanded mode), re-render. Key is `findHunks()` start index for the hunk containing cursor.
- [x] reset `expandedHunks` in `handleFileLoaded()` (collapsed persists across files)
- [x] write tests for `v` key toggling collapsed mode
- [x] write tests for `.` key expanding/collapsing hunks in collapsed mode
- [x] write tests for `.` key as no-op in expanded mode
- [x] write tests for file switch resetting expandedHunks but preserving collapsed
- [x] run `go test ./ui/...` — must pass before task 3

### Task 3: Hunk pairing — identify modified vs pure-add lines

**Files:**
- Modify: `ui/diffview.go`

- [x] add `buildModifiedSet() map[int]bool` method — uses `findHunks()` to get contiguous change groups, then for each group scans from start to next context/divider; if both `ChangeRemove` and `ChangeAdd` exist in the group, marks all add-line `diffLines` indices as modified
- [x] write tests for `buildModifiedSet()` with various hunk compositions: pure adds, pure removes, mixed, multiple hunks
- [x] run `go test ./ui/...` — must pass before task 4

### Task 4: Collapsed diff rendering

**Files:**
- Modify: `ui/diffview.go`

- [x] add `renderCollapsedDiff() string` method on Model
- [x] modify `renderDiff()` to dispatch: if `m.collapsed`, call `renderCollapsedDiff()`
- [x] in collapsed rendering: skip `ChangeRemove` lines (unless hunk is expanded)
- [x] render `ChangeAdd` lines with modify or add style based on `buildModifiedSet()`
- [x] use gutter markers: `+` for pure adds, `~` for modified lines
- [x] for expanded hunks: render all lines with existing expanded styling (reuse `renderDiffLine`)
- [x] preserve annotation rendering in collapsed mode (reuse `renderAnnotationOrInput`)
- [x] write tests for collapsed rendering: removes hidden, adds shown with correct style
- [x] write tests for modified vs pure-add distinction in rendered output
- [x] write tests for expanded hunk inline rendering in collapsed mode (all lines visible)
- [x] write tests for annotations on removed lines hidden in collapsed mode, visible when hunk expanded
- [x] write tests for empty diffLines and divider-only diffLines in collapsed mode
- [x] write tests verifying expanded mode is unchanged (regression)
- [x] run `go test ./ui/...` — must pass before task 5

### Task 5: Cursor movement in collapsed mode

**Files:**
- Modify: `ui/diffview.go`

- [x] modify `moveDiffCursorDown()` to skip removed lines in collapsed mode (unless in expanded hunk)
- [x] modify `moveDiffCursorUp()` to skip removed lines in collapsed mode (unless in expanded hunk)
- [x] modify `skipInitialDividers()` to also skip initial removed lines in collapsed mode
- [x] write tests for cursor down skipping removed lines in collapsed mode
- [x] write tests for cursor up skipping removed lines in collapsed mode
- [x] write tests for cursor movement within expanded hunks (should not skip)
- [x] write tests for cursor movement in expanded mode (unchanged behavior)
- [x] run `go test ./ui/...` — must pass before task 6

### Task 6: Viewport Y calculation in collapsed mode

**Files:**
- Modify: `ui/annotate.go` (where `cursorViewportY` lives)

- [x] modify `cursorViewportY()` to skip hidden removed lines when counting visual rows in collapsed mode
- [x] account for expanded hunks showing all lines in Y calculation
- [x] write tests for `cursorViewportY` in collapsed mode (removed lines not counted)
- [x] write tests for `cursorViewportY` with expanded hunks (all lines counted)
- [x] write tests for page-up/page-down behavior in collapsed mode (implicitly uses fixed `cursorViewportY` + cursor skip from Task 5)
- [x] run `go test ./ui/...` — must pass before task 7

### Task 7: Status bar and help text updates

**Files:**
- Modify: `ui/model.go`

- [x] update `statusBarText()` to show `[v] expand` hint in collapsed mode (or `[v] collapse` in expanded mode)
- [x] show `[.] expand hunk` / `[.] collapse hunk` hint in collapsed diff pane
- [x] write tests for status bar text in both modes
- [x] run `go test ./ui/...` — must pass before task 8

### Task 8: Verify acceptance criteria

- [x] verify `v` toggles between expanded and collapsed views
- [x] verify `.` expands/collapses individual hunks in collapsed mode
- [x] verify modified lines (amber) are distinguished from pure adds (green)
- [x] verify removed lines are hidden in collapsed mode
- [x] verify annotations work in both modes
- [x] verify hunk navigation (`]`/`[`) works in both modes
- [x] verify file switching resets expanded hunks
- [x] run full test suite: `go test ./...`
- [x] run linter: `golangci-lint run`
- [x] verify test coverage for new code meets 80%+

### Task 9: [Final] Update documentation

- [x] update README.md with collapsed mode documentation (keybindings, description)
- [x] update CLAUDE.md if new patterns discovered
- [x] update `.claude-plugin/skills/revdiff/references/usage.md` with new keybindings
- [x] update `.claude-plugin/skills/revdiff/references/config.md` with ModifyFg/ModifyBg color options
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test with real git diffs of various sizes (small edits, large refactors, pure additions, pure deletions)
- test with syntax highlighting enabled and disabled
- test with `--no-colors` flag
- verify annotation export format is unaffected by view mode
