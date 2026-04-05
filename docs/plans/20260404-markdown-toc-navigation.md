# Markdown TOC Navigation Pane

## Overview
Add a table-of-contents navigation pane for markdown files in single-file full-context mode.
Currently, single-file mode hides the left tree pane entirely. When the file is markdown (`.md`/`.markdown`)
and displayed as full context (all lines are `ChangeContext` — no git diff, just file content), show a
TOC pane instead. The TOC lists markdown headers with indentation by level, supports cursor navigation
and jumping to header lines, and auto-tracks the current section as the user scrolls the diff.

## Context (from discovery)
- **Trigger conditions**: `singleFile == true` + markdown extension + all `diffLines` are `ChangeContext`
- **Pane system**: `pane` type with `paneTree`/`paneDiff` constants, focus switching via `togglePane()`
- **Single-file mode**: sets `treeWidth=0`, `focus=paneDiff`, disables tree rendering in `View()`
- **File tree template**: `fileTree` struct in `ui/filetree.go` — cursor/offset/render pattern to mirror
- **Key handling**: `handleKey` dispatches to `handleTreeNav`/`handleDiffNav` based on `m.focus`
- **Viewport centering**: `centerViewportOnCursor()` centers diff on `m.diffCursor` — used for hunk nav
- **`handleFileLoaded`**: insertion point after `m.highlightedLines` assignment, before `skipInitialDividers`
- **`View()`**: single-file branch (lines 597-603) renders diff only; multi-file branch (604-628) is the template for two-pane layout

## Development Approach
- **Testing approach**: regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy
- **Unit tests**: `ui/mdtoc_test.go` for TOC parsing, cursor movement, render, section tracking
- **Integration tests**: `ui/model_test.go` for trigger detection, pane layout, key handling, focus switching

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Solution Overview
New `mdTOC` component (`ui/mdtoc.go`) mirrors the `fileTree` pattern: flat list of entries, cursor/offset
for scrolling, render method. When conditions match (single-file + markdown + full-context), the Model
populates `mdTOC` in `handleFileLoaded`, sets `treeWidth` to ratio-based width, and renders the TOC in
the left pane. Reuse `paneTree` constant for TOC focus (no new pane constant needed — the TOC replaces
the tree in this mode). `togglePane()` and key dispatch work as-is since TOC uses the same pane slot.

## Technical Details
- **`tocEntry`**: `{title string, level int, lineIdx int}` — title is header text without `#` prefix,
  level is 1-6, lineIdx is index into `diffLines`
- **Header parsing**: scan `diffLines` for `Content` matching `^#{1,6} ` (space required after last `#`)
- **Active section tracking**: on each cursor move in diff pane, find nearest `tocEntry` with
  `lineIdx <= m.diffCursor` — update `mdTOC.activeSection` for highlight in render
- **Layout**: when `mdTOC` is populated, `View()` renders two-pane layout (same as multi-file) with
  TOC content in left pane instead of file tree
- **Width**: reuse `treeWidthRatio` for TOC pane width, same as multi-file tree width calculation

## Implementation Steps

### Task 1: mdTOC component — parsing and data structure

**Files:**
- Create: `ui/mdtoc.go`
- Create: `ui/mdtoc_test.go`

- [x] create `tocEntry` struct with `title`, `level`, `lineIdx` fields
- [x] create `mdTOC` struct with `entries []tocEntry`, `cursor`, `offset`, `activeSection` fields
- [x] implement `parseTOC(lines []diff.DiffLine) mdTOC` — scans for markdown headers in context lines, skips headers inside fenced code blocks (track ``` state)
- [x] implement `(m Model) isFullContext(lines []diff.DiffLine) bool` method — returns true when all lines are `ChangeContext` (skips `ChangeDivider`)
- [x] implement `(m Model) isMarkdownFile(filename string) bool` method — checks `.md`/`.markdown` extension
- [x] write tests for `parseTOC` — empty input, single header, nested headers (h1-h6), non-header lines, headers inside fenced code blocks are excluded
- [x] write tests for `isFullContext` — all context, mixed types, empty, divider-only
- [x] write tests for `isMarkdownFile` — `.md`, `.markdown`, `.go`, `.MD` (case sensitivity), no extension
- [x] run tests — must pass before task 2

### Task 2: mdTOC cursor movement and scrolling

**Files:**
- Modify: `ui/mdtoc.go`
- Modify: `ui/mdtoc_test.go`

- [x] implement `moveUp()` / `moveDown()` — move cursor between entries, clamp to bounds
- [x] implement `ensureVisible(height int)` — adjust offset to keep cursor in visible range (mirror `fileTree.ensureVisible`)
- [x] implement `updateActiveSection(diffCursor int)` — find nearest entry with `lineIdx <= diffCursor`, set `activeSection`
- [x] write tests for cursor movement — boundaries, wrap behavior (or clamp)
- [x] write tests for `ensureVisible` — cursor above/below viewport, already visible
- [x] write tests for `updateActiveSection` — cursor before first header, between headers, after last header, no entries
- [x] run tests — must pass before task 3

### Task 3: mdTOC rendering

**Files:**
- Modify: `ui/mdtoc.go`
- Modify: `ui/mdtoc_test.go`

- [x] implement `render(width, height int, focusedPane pane, s styles) string` — renders TOC entries with indentation by level, highlights cursor entry when TOC is focused, highlights active section when diff is focused
- [x] indent entries: 2 spaces per level (h1=0, h2=2, h3=4, etc.), truncate long titles with `…`
- [x] use `styles.FileSelected` for cursor highlight (same as file tree), different style for active section marker
- [x] write tests for render — empty TOC, various header levels, cursor highlight, active section highlight, truncation, scrolling offset
- [x] run tests — must pass before task 4

### Task 4: integrate mdTOC into Model

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] add `mdTOC *mdTOC` field to `Model` struct (nil when not applicable)
- [x] in `handleFileLoaded`: after `highlightedLines` assignment, check `singleFile && m.isMarkdownFile(msg.file) && m.isFullContext(msg.lines)` — if true, parse TOC, set `treeWidth` to ratio-based width (overrides the `treeWidth=0` set earlier in `handleFilesLoaded`), adjust viewport width
- [x] in `handleFileLoaded`: if conditions don't match (or TOC has no entries), set `mdTOC = nil`, keep `treeWidth = 0`
- [x] in `handleResize`: modify existing `if m.singleFile` branch — only set `treeWidth=0` when `m.mdTOC == nil`; when `m.mdTOC != nil`, compute `treeWidth` from ratio (same as multi-file)
- [x] write tests for TOC detection in `handleFileLoaded` — markdown full-context triggers TOC, non-markdown doesn't, markdown with diff changes doesn't, markdown with no headers produces nil TOC
- [x] write tests for resize with TOC active — treeWidth computed correctly
- [x] run tests — must pass before task 5

### Task 5: TOC pane rendering in View

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] in `View()`: replace single-file branch — when `mdTOC != nil`, render two-pane layout with TOC in left pane (reuse multi-file layout pattern), otherwise keep current single-pane layout
- [x] TOC pane uses `TreePane`/`TreePaneActive` styles based on focus (same as file tree)
- [x] diff pane width = `m.width - m.treeWidth - 4` when TOC is shown
- [x] in `diffContentWidth()`: narrow existing `if m.singleFile` condition to `m.singleFile && m.mdTOC == nil`; when `mdTOC != nil`, falls through to the existing multi-file formula
- [x] write tests for `View()` output — TOC pane appears for markdown full-context, doesn't appear for non-markdown single-file
- [x] write tests for `diffContentWidth` with TOC active
- [x] run tests — must pass before task 6

### Task 6: focus switching and key handling

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] modify `togglePane()`: when `singleFile && mdTOC != nil`, toggle between `paneTree` and `paneDiff` (reuse existing pane constants — TOC uses `paneTree` slot)
- [x] modify `handleSwitchToTree()`: allow switching to tree pane when `mdTOC != nil` (currently blocked by `!m.singleFile` guard) — needed for `h` key in diff pane
- [x] in `handleTreeNav`: when `mdTOC != nil`, handle `j`/`k`/`pgdn`/`pgup`/`home`/`end` as TOC cursor movement instead of file tree movement
- [x] in `handleTreeNav`: when `mdTOC != nil`, handle `l`/`right` to switch to diff pane (same as file tree)
- [x] in `handleEnterKey`: when `focus == paneTree && mdTOC != nil`, jump to selected header — set `m.diffCursor = entry.lineIdx`, call `centerViewportOnCursor()`
- [x] in `handleDiffNav`: on cursor movement (`j`/`k`/pgdn/pgup/home/end), call `mdTOC.updateActiveSection(m.diffCursor)` when TOC is active
- [x] write tests for Tab toggling with TOC active — cycles between TOC and diff
- [x] write tests for `h` key in diff pane switching to TOC
- [x] write tests for j/k/pgdn/pgup/home/end in TOC pane — cursor moves between headers
- [x] write tests for Enter in TOC pane — diffCursor jumps to header line
- [x] write tests for active section tracking — scrolling diff updates TOC highlight
- [x] run tests — must pass before task 7

### Task 7: help overlay and edge cases

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [x] update help overlay text to mention TOC navigation for markdown files (Tab to switch, j/k to navigate, Enter to jump)
- [x] handle edge case: markdown file with no headers — `mdTOC` is nil, falls back to normal single-file mode
- [x] verify annotation workflow still works with TOC pane (annotating in diff pane, file-level annotation)
- [x] verify search still works with TOC pane (search input width, match navigation)
- [x] write tests for annotations and search with TOC active
- [x] run tests — must pass before task 8

### Task 8: verify acceptance criteria

- [ ] verify: single-file markdown in full-context mode shows TOC pane
- [ ] verify: non-markdown single-file still hides tree pane
- [ ] verify: markdown with actual diff changes (adds/removes) does not show TOC
- [ ] verify: Tab switches focus, j/k navigates, Enter jumps
- [ ] verify: active section tracks cursor position in diff
- [ ] verify: multi-file mode completely unaffected
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] run formatters: `make fmt`

### Task 9: [Final] Update documentation

- [ ] update README.md with markdown TOC navigation feature
- [ ] update CLAUDE.md if new patterns discovered
- [ ] update `.claude-plugin/skills/revdiff/references/usage.md` with TOC keybindings
- [ ] ask about bumping plugin version in `plugin.json` and `marketplace.json`
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test with a real markdown file: `revdiff --only README.md`
- verify TOC renders correctly with various header depths
- verify jumping to headers centers the viewport properly
- verify active section tracking works while scrolling
- test with a very long markdown file to check scrolling in TOC pane
- test with a markdown file that has no headers — should fall back to hidden pane
