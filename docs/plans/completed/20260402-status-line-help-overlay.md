# Split Status Bar into Status Line + Help Overlay

## Overview
Replace the current single status bar (which mixes status info and shortcut hints) with two separate concerns:
1. **Status line** — shows file info, diff stats, hunk position, mode indicators, annotation count
2. **Help overlay** — a modal popup triggered by `?` showing all keybindings organized by section

The current `statusBarText()` in `ui/model.go:488-551` is crowded and hard to scan. Splitting status from help makes both more useful: the status line becomes a clean info strip, and the help overlay provides comprehensive reference without cluttering the screen.

## Context
- **Primary file:** `ui/model.go` — `statusBarText()` (lines 488-551), `View()` (lines 442-486), `handleKey()` (lines 188-259)
- **Styles:** `ui/styles.go` — `StatusBar` style, `Colors` struct
- **Diff navigation:** `ui/diffview.go` — `currentHunk()`, `findHunks()`
- **Collapsed mode:** `ui/collapsed.go` — `collapsedState`
- **Model fields:** `ui/model.go` — `currFile`, `diffLines`, `store`, `collapsed`, etc.

## Solution Overview

### Status line layout (left to right)
```
filename  +N/-N  hunk X/Y  ▼ ◉                    3 annotations  ? help
```
- **filename** — current file path (truncated from left with `…` if too long)
- **+N/-N** — additions/deletions count for the current file
- **hunk X/Y** — current hunk position (only when cursor is on a changed line)
- **▼** — collapsed mode indicator (only when active)
- **◉** — filter active indicator (only when active)
- **right-aligned:** annotation count + `? help` hint

### Help overlay
- Triggered by `?` key, dismissed by `?` or `esc`
- Centered bordered box rendered on top of the main view
- Sections: Navigation, Annotations, View, Quit
- Uses lipgloss border styling consistent with existing pane borders

## Technical Details

### New fields in Model
- `showHelp bool` — true when help overlay is visible
- No new files needed — help rendering goes in a new `helpOverlay()` method in `model.go`

### File stats computation
- Count adds/removes from `m.diffLines` on file load (in `handleFileLoaded`)
- Cache as `fileAdds int`, `fileRemoves int` fields on Model
- Reset on file change

### Status line segments
Each segment is a small string. Segments are joined with double-space separators. Right-aligned section uses padding like current implementation.

### Help overlay rendering
- Build help text as a lipgloss-bordered box
- When `m.showHelp` is true, `View()` replaces the main content area with the centered help popup (standard bubbletea modal pattern — no true compositing, the help box replaces tree+diff content)
- Use `lipgloss.Place(m.width, paneHeight, lipgloss.Center, lipgloss.Center, helpBox)` + status bar below
- Note: bubbletea reports `?` key correctly via `msg.String()` (shifted `/` key)

### Narrow terminal handling
- Truncate filename from left with `…` prefix when space is tight
- Drop lower-priority segments (hunk, mode icons) if width is insufficient

## Development Approach
- **Testing approach:** regular (code first, then tests)
- Complete each task fully before moving to the next
- Run tests after each change
- Maintain backward compatibility (existing CLI flags, config, styles all still work)

## Implementation Steps

### Task 1: Compute and cache file diff stats

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] add `fileAdds` and `fileRemoves` int fields to `Model` struct
- [x] add `computeFileStats()` method that counts add/remove lines from `m.diffLines`
- [x] call `computeFileStats()` in `handleFileLoaded` after setting `m.diffLines`
- [x] write tests for `computeFileStats()` with various diff line combinations
- [x] run `make test` — must pass before task 2

### Task 2: Rewrite status line and update tests

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] rewrite `statusBarText()` to show: filename, +N/-N stats, hunk X/Y, mode icons (▼ ◉), right-aligned annotation count + `? help`
- [x] keep special cases for `inConfirmDiscard` and `annotating` modes unchanged
- [x] implement filename truncation with `…` prefix for narrow terminals
- [x] drop hunk and mode icons gracefully when terminal is too narrow
- [x] update existing `statusBarText` tests to match new format (no shortcut hints, has filename/stats)
- [x] add test cases for: filename truncation, mode indicators present/absent, stats display
- [x] add test cases for narrow terminal width graceful degradation
- [x] run `make test` — must pass before task 3

### Task 3: Add help overlay rendering

**Files:**
- Modify: `ui/model.go`

- [x] add `showHelp bool` field to Model
- [x] add `helpOverlay()` method returning the bordered help text with sections (Navigation, Annotations, View, Quit)
- [x] modify `View()` to overlay help popup using `lipgloss.Place()` when `m.showHelp` is true
- [x] write tests for `helpOverlay()` verifying section headers and key listings are present
- [x] run `make test` — must pass before task 4

### Task 4: Wire up `?` key handling

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] handle `?` key in `handleKey()` to toggle `m.showHelp`
- [x] handle `esc` key to close help when `m.showHelp` is true
- [x] block all other key handling when help overlay is showing (except `?` and `esc`)
- [x] write tests for help toggle behavior (open, close with ?, close with esc)
- [x] write test that other keys are blocked when help is showing
- [x] run `make test` — must pass before task 5

### Task 5: Verify acceptance criteria
- [x] verify status line shows filename, stats, hunk, mode icons, annotations, help hint
- [x] verify help overlay opens with `?` and closes with `?` or `esc`
- [x] verify no shortcut hints in status bar anymore (all moved to help overlay)
- [x] verify special modes (annotation input, discard confirm) still work in status bar
- [x] run full test suite: `make test`
- [x] run linter: `make lint`

### Task 6: [Final] Update documentation
- [x] update README.md with new `?` help shortcut and status line description
- [x] update `.claude-plugin/skills/revdiff/references/usage.md` with `?` help keybinding
- [x] update CLAUDE.md if any new patterns discovered
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test with narrow terminal widths (< 80 cols) to verify truncation
- test with large diffs (many hunks) to verify hunk counter
- test collapsed mode + filter active to verify both icons show
- verify help overlay looks correct with different color themes
