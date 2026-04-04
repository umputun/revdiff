# Annotation List Popup

## Overview
Add a popup overlay (`@` key) listing all annotations across all files. Users can navigate the list with arrow keys and jump to any annotation with Enter. This provides a quick index of all review notes without scrolling through files.

## Context
- **Overlay pattern:** `ui/model.go` — `showHelp`, `helpOverlay()`, `overlayCenter()` already implement ANSI-aware compositing
- **Annotation store:** `annotation/store.go` — `All()` returns `map[string][]Annotation` sorted by line, `Files()` returns sorted file list
- **File loading:** `ui/model.go` — `loadSelectedIfChanged()` loads a file when tree selection changes, `handleFileLoaded` resets cursor/viewport
- **Tree sync:** `ui/filetree.go` — cursor manipulation + `loadSelectedIfChanged()` (need to create `selectByPath` method)

## Technical Details

### Popup state
New fields on Model:
- `showAnnotList bool` — popup visible
- `annotListCursor int` — selected item in the flat list
- `annotListItems []annotation.Annotation` — built on open from `store.All()`, sorted by file then line

### Popup appearance
```
┌──────────── annotations (3) ────────────────┐
│                                              │
│ > handler.go:43 (+)  use errors.Is() ins...  │
│   handler.go:87 (+)  add context to error    │
│   store.go:18 (-)    keep this validation    │
│                                              │
└──────────────────────────────────────────────┘
```
- Bordered box using `lipgloss.NormalBorder()` with `Accent` color (same as help overlay)
- Selected line highlighted with `>` prefix and `SelectedFg`/`SelectedBg` styles
- File-level annotations show as `filename (file-level)  comment...`
- Comment truncated to fit popup width
- No help hints at the bottom (keys are obvious)
- Empty state: "no annotations" centered text

### Key handling
- `@` toggles popup (open/close)
- `j`/`k` or `↑`/`↓` navigate the list
- `Enter` jumps to the annotation: loads file if needed, moves diff cursor to the line, centers viewport, closes popup
- `Esc` closes without jumping
- All other keys consumed (blocked)
- Priority in `handleKey()`: place `showAnnotList` check after annotating/searching checks, parallel to `showHelp` (pattern: `if msg.String() == "@" || m.showAnnotList { return m.handleAnnotListKey(msg) }`)
- `inConfirmDiscard` is already handled at the `Update()` level, no special handling needed

### Styling
- Border: `lipgloss.NormalBorder()` with `s.colors.Accent` foreground (same as help overlay)
- Selected item: `s.FileSelected` style (reuse existing selected-file highlight)
- Normal items: `s.FileEntry` style
- Change type prefix: `s.colors.AddFg` for `+`, `s.colors.RemoveFg` for `-`, muted for context

### Jump-to logic
On Enter:
1. Get selected `Annotation` from `annotListItems[annotListCursor]`
2. If `annotation.Line == 0` (file-level): load file if needed, set `diffCursor = -1` (virtual file annotation line)
3. If `annotation.File != m.currFile`: select file in tree via `m.tree.selectByPath(path)`, set `pendingAnnotJump`, trigger file load
4. If same file: use `findDiffLineIndex()` to locate the line, set `m.diffCursor`, call `centerViewportOnCursor()`
5. Close popup

### findDiffLineIndex semantics
Must match `diffLineNum()` logic from `annotate.go`: for removes (`changeType == "-"`), compare against `dl.OldNum`; for adds/context, compare against `dl.NewNum`. Also verify `string(dl.ChangeType) == changeType`. Returns -1 if not found.

### Cross-file jump challenge
File loading is async (returns a `tea.Cmd`). When jumping to an annotation in a different file:
- Store a "pending jump" target: `pendingAnnotJump *annotation.Annotation`
- In `handleFileLoaded`, check if `pendingAnnotJump` is set AND `pendingAnnotJump.File == msg.file` (guard against stale jumps from rapid file switching), find the line, position cursor, clear the pending jump
- Clear `pendingAnnotJump` when a new non-annotation file load is triggered (e.g., via n/p file navigation) to prevent stale jumps
- In single-file mode, tree selection is unnecessary — position cursor directly

### Popup width/height
- Width: `min(m.width - 10, 70)` — reasonable max, leaves margin
- Height: `min(len(items) + 4, m.height - 6)` — fit all items if possible, leave room for borders and padding
- If items exceed height: scroll the list (track offset like the tree pane does)

## Development Approach
- **Testing approach:** regular (code first, then tests)
- Complete each task fully before moving to the next
- Run tests after each change

## Implementation Steps

### Task 1: Build annotation list data, rendering, and overlay integration

**Files:**
- Create: `ui/annotlist.go`
- Create: `ui/annotlist_test.go`
- Modify: `ui/model.go`

- [x] add `showAnnotList bool`, `annotListCursor int`, `annotListOffset int`, `annotListItems []annotation.Annotation`, `pendingAnnotJump *annotation.Annotation` fields to Model
- [x] create `ui/annotlist.go` with `buildAnnotListItems()` method: iterates `store.Files()`, then `store.Get(file)` for each, builds flat sorted list
- [x] add `annotListOverlay()` method in `annotlist.go`: renders bordered popup with item list, highlighted cursor, scrolling, handles empty state
- [x] truncate comments to fit popup width, show file:line prefix with change type styling
- [x] render annotation list popup in `View()` using `overlayCenter()` when `showAnnotList` is true (takes priority over help overlay)
- [x] write tests in `ui/annotlist_test.go` for `buildAnnotListItems()` with multiple files and annotations
- [x] write tests for `annotListOverlay()` verifying content, highlighting, empty state, scrolling
- [x] run `make test` — must pass before task 2

### Task 2: Wire up key handling and popup toggle

**Files:**
- Modify: `ui/annotlist.go`
- Modify: `ui/model.go`

- [ ] add `handleAnnotListKey(msg)` method in `annotlist.go`: `j`/`k`/`↑`/`↓` for navigation with scroll offset, `Enter` for jump, `Esc`/`@` for close, consume all other keys
- [ ] add `showAnnotList` check in `handleKey()` after annotating/searching, parallel to `showHelp`: `if msg.String() == "@" || m.showAnnotList { return m.handleAnnotListKey(msg) }`
- [ ] on `@` open: rebuild items via `buildAnnotListItems()`, reset cursor and offset
- [ ] write tests for popup toggle, list navigation, scroll offset, close with esc
- [ ] run `make test` — must pass before task 3

### Task 3: Implement jump-to-annotation logic

**Files:**
- Modify: `ui/annotlist.go`
- Modify: `ui/model.go`
- Modify: `ui/filetree.go`

- [ ] create `selectByPath(path string)` method on `fileTree` — sets cursor to matching file entry
- [ ] add `findDiffLineIndex(line int, changeType string) int` method in `annotlist.go` — uses `diffLineNum()` semantics: compares against `dl.OldNum` for removes, `dl.NewNum` for adds/context; returns -1 if not found
- [ ] on Enter: for file-level annotations (`Line == 0`), set `diffCursor = -1`; for same-file, use `findDiffLineIndex` + `centerViewportOnCursor()`; for different file, set `pendingAnnotJump` and trigger file load via `selectByPath` + `loadSelectedIfChanged()`
- [ ] in `handleFileLoaded`: check `pendingAnnotJump` and verify `pendingAnnotJump.File == msg.file` (guard against stale jumps), position cursor, center viewport, clear pending
- [ ] clear `pendingAnnotJump` when non-annotation file navigation triggers a load (n/p keys)
- [ ] write tests for same-file jump, cross-file jump (pending mechanism), file-level annotation jump, stale pending guard, line matching with OldNum/NewNum semantics
- [ ] run `make test` — must pass before next task

### Task 4: Verify acceptance criteria
- [ ] verify `@` opens and closes annotation list popup
- [ ] verify list shows all annotations sorted by file then line
- [ ] verify arrow keys navigate the list with visual highlight
- [ ] verify Enter jumps to same-file annotation correctly
- [ ] verify Enter jumps to cross-file annotation correctly (loads file, positions cursor)
- [ ] verify Esc closes without jumping
- [ ] verify empty state ("no annotations") when no annotations exist
- [ ] verify popup scrolls with long annotation lists
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`

### Task 5: [Final] Update documentation
- [ ] update README.md with `@` annotation list keybinding
- [ ] update `.claude-plugin/skills/revdiff/references/usage.md` with annotation list feature
- [ ] update CLAUDE.md if new patterns discovered
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test with no annotations (empty popup)
- test with 1 annotation (single item)
- test with many annotations across multiple files (scrolling, cross-file jump)
- test with file-level annotations in the list
- test jump to annotation in collapsed mode
- test jump to annotation with wrap mode enabled
