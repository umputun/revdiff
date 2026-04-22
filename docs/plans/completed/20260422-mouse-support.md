# Mouse Support (Option B)

## Overview

Add mouse support to revdiff covering scroll-wheel in both panes and left-click on the file tree and diff lines. Today revdiff does not enable mouse tracking at all — what "works" in kitty is the terminal translating wheel events into cursor keys while alt-screen mouse reporting is off. iTerm2 and several other terminals do not translate, so users see inconsistent behavior.

This plan enables `tea.WithMouseCellMotion()` and routes `tea.MouseMsg` events through a new `handleMouse` handler. After this work:

- Scroll wheel scrolls whichever pane the cursor is over (diff or tree).
- Left-click on a tree entry focuses the tree and selects/loads that entry — same as pressing j/k to land there.
- Left-click on a diff line focuses the diff pane and moves the cursor to that line — enabling "click, then `a` to annotate".
- Shift+wheel scrolls by half-page.
- `--no-mouse` / `REVDIFF_NO_MOUSE` opts out entirely, for users who lean heavily on terminal-native text-selection.

Out of scope: clickable status-bar segments, overlay item clicks, right-click menus, drag selection, middle-click paste, click on horizontal scroll indicators, per-column semantics (blame/line-number).

## Context (from discovery)

- **Bubble Tea setup**: `app/main.go:104` currently uses `tea.WithAltScreen()` only. Mouse is opt-in via `tea.WithMouseCellMotion()` (button events + motion while pressed, no hover flood).
- **Update loop**: `app/ui/model.go:662` — `Update()` switches on `tea.KeyMsg`, `tea.WindowSizeMsg`, and the project's own `*Msg` types. No `tea.MouseMsg` case today; any mouse event is silently dropped.
- **Pane focus model already exists**: `pane` iota with `paneTree` and `paneDiff` at `app/ui/model.go:212`. `Model.layout.focus` tracks current pane. `togglePane()` (model.go:898) flips focus — re-usable by the mouse path.
- **Layout math**: `app/ui/view.go` — tree width is `m.layout.treeWidth`, diff width is `m.layout.width - m.layout.treeWidth - 4`. Tree is hidden when `m.layout.treeHidden` or `m.file.singleFile && m.file.mdTOC == nil`.
- **Tree navigation reuse**: `app/ui/diffnav.go:468` `handleTreeNav` calls `m.tree.Move(...)` then `m.loadSelectedIfChanged()` — any mouse-driven tree selection reuses the same load path.
- **FileTree API**: `app/ui/sidepane/filetree.go` exposes `Move`, `SelectByPath`, `SelectedFile`, with internal `cursor int` and `offset int`. No `SelectByVisibleRow` yet — needs adding.
- **Cursor-line math**: `app/ui/diffnav.go` has `cursorVisualRange(idx)` (forward: diff-line → visual-row range) and `hunkLineHeight(i, hunks, annSet)` (height of one diff line including wrap + annotations). Inverse mapping (visual-row → diff-line) does not exist and must be added.
- **Modal states that must swallow mouse**: `m.annot.annotating`, `m.search.active`, `m.reload.pending`, `m.inConfirmDiscard`, and any open overlay via `m.overlay.Manager`.
- **Existing no-* flags pattern**: `app/config.go:23-25` — `NoColors`, `NoStatusBar`, `NoConfirmDiscard` all use `long:"no-foo" ini-name:"no-foo" env:"REVDIFF_NO_FOO"` — `NoMouse` follows the same shape.
- **Docs sync locations** (per project memory): `README.md`, `site/docs.html`, `.claude-plugin/skills/revdiff/references/{config,usage}.md`, `plugins/codex/skills/revdiff/references/{config,usage}.md` (byte-identical copy of the claude-plugin versions).

## Development Approach

- **Testing approach**: Regular (code first, then tests). The design was already validated in brainstorm; TDD is lower-value here because the routing skeleton is nearly mechanical and the interesting invariant (visual-row ↔ diff-line round-trip) benefits from property-style tests written against the finished implementation.
- Complete each task fully before moving to the next.
- **CRITICAL: every task MUST include new/updated tests** — tests for routing, hit-testing, round-trip mapping, and modal-state swallowing are non-negotiable.
- **CRITICAL: all tests must pass before starting the next task**.
- Maintain backward compatibility — default behavior when `--no-mouse` is not set is unchanged from the user's perspective *except* that wheel now works consistently and tree/diff are clickable.
- After each change, run: `go test ./...`, `golangci-lint run`, and `~/.claude/format.sh`.

## Testing Strategy

- **Unit tests**: required for every task. Tests for `hitTest`, `visualRowToDiffLine`, `handleMouse` routing (all pane × button combinations), modal-state swallowing, and `--no-mouse` config wiring.
- **No e2e tests**: revdiff has no Playwright/headless-TUI harness. Mouse events are constructed as `tea.MouseMsg` values in-process, which is the existing test idiom for `tea.KeyMsg` cases. Manual verification against kitty, iTerm2, Ghostty, and tmux is part of Post-Completion.
- **Round-trip property test**: for a set of synthetic `diffLines` (with wraps, annotations, collapsed-mode dividers), assert `visualRowToDiffLine(cursorVisualRange(i).top) == i` for every valid `i`. This is the highest-value test — inverse mapping bugs would be the most common failure mode.

## Solution Overview

**Architecture:**

1. Program-level: `app/main.go` appends `tea.WithMouseCellMotion()` to `programOptions` unless `opts.NoMouse` is true.
2. Model-level: `Model.Update()` gains a `tea.MouseMsg` case that calls `m.handleMouse(msg)`.
3. New package-local file `app/ui/mouse.go` owns: `handleMouse`, `hitTest`, `hitZone` type, and routing helpers. Keeps `model.go` from growing (file-by-concern convention).
4. New method `m.visualRowToDiffLine(row int) int` lives in `app/ui/diffnav.go` next to `cursorVisualRange` — keeps forward and inverse mapping side-by-side so any future layout change touches both.
5. New method `ft.SelectByVisibleRow(row, topOffset int) bool` on `*sidepane.FileTree` maps a screen row to an entry index using its internal `offset`.

**Event dispatch flow inside `handleMouse`:**

```
# clear transient hints first, mirroring handleKey at model.go:704-706
m.commits.hint = ""; m.reload.hint = ""; m.compact.hint = ""

if m.inConfirmDiscard || m.reload.pending || m.annot.annotating || m.search.active:
    return m, nil                             # swallow, do nothing
if m.overlay.HasOpen():
    return m, nil                             # swallow, do not forward wheel to diff
switch msg.Button:
case wheelUp/wheelDown:
    step := wheelStep; if msg.Shift { step = viewport.Height/2 }
    route to pane under (msg.X, msg.Y)
case wheelLeft/wheelRight:
    return m, nil                             # intentionally swallowed — horizontal scroll stays keyboard-driven
case left press (msg.Action == MouseActionPress only — skip release/motion):
    route to pane under (msg.X, msg.Y)
default:                                      # right, middle, back, forward, release, motion
    swallow
```

**Wheel step constant**: `const wheelStep = 3` (unexported) in `mouse.go`. Matches standard terminal feel (3 lines per notch). Shift+wheel uses `viewport.Height/2` to mirror half-page keyboard shortcut.

**Single-click on tree loads file immediately**: confirmed by reading `diffnav.go:468-502` — `handleTreeNav` always calls `loadSelectedIfChanged()` after every cursor move. Click reuses the same path, so single-click == j-to-land behavior. No double-click state machine needed.

**Click on directory row in tree**: cursor moves there (matches j landing on a directory); `SelectedFile()` returns empty for dirs, so `loadSelectedIfChanged` becomes a no-op. Same outcome as j.

**Text-selection trade-off**: once mouse tracking is on, plain drag is captured by revdiff. Users must hold Shift (most terminals) or Option (iTerm2) for terminal-native text selection. Documented in README; `--no-mouse` is the escape hatch.

## Technical Details

### `tea.MouseMsg` shape (Bubble Tea v1)

```go
type MouseMsg MouseEvent
type MouseEvent struct {
    X, Y   int
    Shift  bool
    Alt    bool
    Ctrl   bool
    Action MouseAction   // Press / Release / Motion
    Button MouseButton   // None / Left / Middle / Right / WheelUp / WheelDown / WheelLeft / WheelRight / Back / Forward
    Type   MouseEventType // legacy
}
```

Use `msg.Button` and `msg.Action` for dispatch; use `msg.Shift` for half-page wheel; use `msg.X/Y` for hit-testing. Ignore `Alt`/`Ctrl` for this pass.

### Hit-zone classification

```go
type hitZone int
const (
    hitNone   hitZone = iota   // outside any interactive area
    hitTree                    // in tree pane (or TOC pane when mdTOC active)
    hitDiff                    // in diff pane (body, not header)
    hitStatus                  // in status bar rows
    hitHeader                  // in diff header (file path row) — currently no-op
)

func (m Model) hitTest(x, y int) hitZone {
    if y >= m.layout.height - m.statusBarHeight() { return hitStatus }
    if y < m.diffTopRow() { return hitHeader }

    treeVisible := !m.layout.treeHidden && (!m.file.singleFile || m.file.mdTOC != nil)
    if treeVisible && x < m.layout.treeWidth { return hitTree }
    if x < m.layout.width { return hitDiff }
    return hitNone
}
```

Three layout helpers on `Model` (all unexported, pure reads of `m.layout.*`):

- `statusBarHeight() int` — 0 when `m.NoStatusBar`, 1 otherwise. Check if a similar helper already exists — reuse if present.
- `diffTopRow() int` — first row of diff viewport content (accounts for diff header row).
- `treeTopRow() int` — first row of tree content (accounts for any pane border from `renderTwoPaneLayout`). Derived from the same layout math that produces the rendered output, not hardcoded — verified against `view.go` rendering.

Unit-testable in isolation — pure arithmetic, no I/O.

### Inverse mapping `visualRowToDiffLine`

Returns both the diff-line index and whether the target row is an annotation sub-row (so the caller can set `m.annot.cursorOnAnnotation` consistently with keyboard navigation at `diffnav.go:28-46`).

```go
// visualRowToDiffLine returns the diff-line index whose rendered rows contain
// the given visual row (0 = first visible row of viewport content), and whether
// the row falls on an annotation sub-line rather than the diff line itself.
// Returns the current cursor if row is out of bounds.
func (m Model) visualRowToDiffLine(row int) (idx int, onAnnotation bool) {
    if len(m.file.lines) == 0 { return m.nav.diffCursor, false }

    hunks := m.findHunks()
    annSet := m.buildAnnotationSet()
    running := 0

    // file-level annotation occupies visual row 0 at logical index -1
    // (see moveDiffCursorToStart at diffnav.go:172 and hasFileAnnotation at annotate.go:331)
    if m.hasFileAnnotation() {
        if row < 1 { return -1, false }
        running = 1
    }

    for i := 0; i < len(m.file.lines); i++ {
        h := m.hunkLineHeight(i, hunks, annSet)   // total visual rows (diff line + injected annotation rows)
        if row < running + h {
            // if row falls on an annotation sub-row (row >= running + diffLineHeight)
            // we need to distinguish. hunkLineHeight = diffLineRows + annotationRows.
            diffRows := h - m.annotationRowsFor(i, annSet)  // helper: 0 if no annotation, else N rows
            if row >= running + diffRows {
                return i, true
            }
            return i, false
        }
        running += h
    }
    return len(m.file.lines) - 1, false
}
```

Called with `row = (y - m.diffTopRow()) + m.layout.viewport.YOffset` where `diffTopRow()` is a new helper returning the first row of the diff viewport content (header offset).

**Round-trip invariant** (tested in Task 2):

```go
// for each valid cursor position i, cursorVisualRange must round-trip through
// the inverse mapping. cursorVisualRange takes no args; it reads m.nav.diffCursor.
m.nav.diffCursor = i
top, _ := m.cursorVisualRange()
idx, _ := m.visualRowToDiffLine(top)
assert.Equal(t, i, idx)
```

Test cases must cover: `i=-1` (file-level annotation), `i=0`, wrapped long lines, lines with injected annotations (annotation sub-row round-trip), collapsed-mode dividers, viewport with `YOffset > 0`.

### Tree row → entry index

```go
// SelectByVisibleRow sets the cursor to the entry at the given visible row.
// row is 0-based relative to the first visible tree line. Returns true if the
// row maps to a valid entry.
func (ft *FileTree) SelectByVisibleRow(row int) bool {
    idx := ft.offset + row
    if idx < 0 || idx >= len(ft.entries) { return false }
    ft.cursor = idx
    return true
}
```

Click wrapper on `Model`:

```go
func (m Model) clickTree(x, y int) (tea.Model, tea.Cmd) {
    row := y - m.treeTopRow()
    m.layout.focus = paneTree
    if m.file.mdTOC != nil {
        if !m.file.mdTOC.SelectByVisibleRow(row) { return m, nil }
    } else if !m.tree.SelectByVisibleRow(row) {
        return m, nil
    }
    m.pendingAnnotJump = nil
    m.nav.pendingHunkJump = nil
    return m.loadSelectedIfChanged()
}

func (m Model) clickDiff(x, y int) (tea.Model, tea.Cmd) {
    row := (y - m.diffTopRow()) + m.layout.viewport.YOffset
    idx, onAnnot := m.visualRowToDiffLine(row)
    m.layout.focus = paneDiff
    m.nav.diffCursor = idx
    m.annot.cursorOnAnnotation = onAnnot
    m.syncViewportToCursor()
    return m, nil
}
```

**Wheel-by-N in tree**: use the existing page-motion mechanism — `m.tree.Move(sidepane.MotionPageDown, wheelStep)` / `MotionPageUp` already move the cursor by N entries (see `pageDown(n)` at `filetree.go:488`). No new motion needed, no loop required. Shift+wheel passes `m.treePageSize()/2` for half-page feel consistent with half-page-down keybinding.

### `--no-mouse` flag

```go
// app/config.go
NoMouse bool `long:"no-mouse" ini-name:"no-mouse" env:"REVDIFF_NO_MOUSE" description:"disable mouse support (scroll wheel, click)"`
```

```go
// app/main.go
programOptions := []tea.ProgramOption{tea.WithAltScreen()}
if !opts.NoMouse {
    programOptions = append(programOptions, tea.WithMouseCellMotion())
}
```

`--dump-config` will include the field automatically via the existing INI serialization.

### `--no-mouse` is honored at the program-option layer

Rationale: if mouse tracking is never enabled, the terminal never emits mouse escape sequences, so `handleMouse` is never called. No runtime branch needed inside `handleMouse`. Simpler and makes text-selection-for-copy work without Shift.

### TOC pane handling

When `m.file.mdTOC != nil`, the tree pane slot renders the TOC instead. Click in that slot should route to the TOC (analogous `mdTOC.SelectByVisibleRow` — add to `app/ui/sidepane/toc.go`). Otherwise routes to the file tree. This is a direct mirror of `handleTreeNav`'s `if m.file.mdTOC != nil { return m.handleTOCNav(msg) }` branch.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code, tests, docs sync. Everything that can be verified locally.
- **Post-Completion** (no checkboxes): manual terminal verification (kitty, iTerm2, Ghostty, tmux, Zellij), since we cannot test mouse behavior in CI.

## Implementation Steps

### Task 1: Add `NoMouse` config flag + wire program option

**Files:**
- Modify: `app/config.go`
- Modify: `app/main.go`
- Modify: `app/config_test.go` (add flag-parsing assertion)

- [x] add `NoMouse bool` field to `Options` in `app/config.go` following `NoColors`/`NoStatusBar` convention (CLI flag, ini-name, env var, description)
- [x] in `app/main.go` conditionally append `tea.WithMouseCellMotion()` to `programOptions` when `!opts.NoMouse`
- [x] add flag-parsing test: `--no-mouse` sets `Options.NoMouse=true`; env var `REVDIFF_NO_MOUSE=true` sets it; default is false
- [x] verify `--dump-config` output includes `no-mouse = false` by default (spot-check via existing dump-config test pattern)
- [x] run `go test ./...` — must pass before task 2

*Note: bubbletea `ProgramOption` is an opaque function value — testing its presence in a `[]tea.ProgramOption` slice is impractical. Program wiring is covered by the manual terminal verification in Post-Completion.*

### Task 2: Add `visualRowToDiffLine` inverse mapping

**Files:**
- Modify: `app/ui/diffnav.go` (or `app/ui/annotate.go` — put it next to `cursorVisualRange` wherever that lives; currently `annotate.go:477`)
- Modify: `app/ui/diffnav_test.go` or corresponding test file (match location of implementation)

- [x] add `func (m Model) visualRowToDiffLine(row int) (idx int, onAnnotation bool)` next to `cursorVisualRange` — walks `m.file.lines` summing `hunkLineHeight` (using `m.findHunks()` and `m.buildAnnotationSet()`) until the running sum crosses `row`; distinguishes annotation sub-rows from diff rows within each line
- [x] handle file-level annotation: when `m.hasFileAnnotation()` is true, visual row 0 maps to `idx=-1, onAnnotation=false`
- [x] handle edge cases: empty `m.file.lines` → return `(m.nav.diffCursor, false)`; row < 0 → return `(0, false)` (or `(-1, false)` if file-level annotation present); row exceeds total → return last valid index
- [x] add round-trip table test: for each cursor index `i` in a synthetic set, set `m.nav.diffCursor = i`, call `top, _ := m.cursorVisualRange()`, assert `visualRowToDiffLine(top) == (i, false)`
- [x] add round-trip test for annotation sub-rows: set cursor on a line with annotation, set `cursorOnAnnotation=true`, round-trip must return `(i, true)`
- [x] add table cases covering: file-level annotation row `i=-1`, `i=0` baseline, wrapped long lines, lines with injected annotations, collapsed-mode dividers, viewport with `YOffset > 0`
- [x] add direct-input tests: specific `row` values → expected `(idx, onAnnotation)` pairs in a known fixture
- [x] run `go test ./app/ui/...` — must pass before task 3

### Task 3: Add `SelectByVisibleRow` on FileTree and TOC

**Files:**
- Modify: `app/ui/sidepane/filetree.go`
- Modify: `app/ui/sidepane/filetree_test.go`
- Modify: `app/ui/sidepane/toc.go`
- Modify: `app/ui/sidepane/toc_test.go`

- [x] add `func (ft *FileTree) SelectByVisibleRow(row int) bool` — returns true if row maps to a valid entry, updates `ft.cursor` to `ft.offset + row`
- [x] add equivalent `SelectByVisibleRow` on `*TOC` (same signature)
- [x] tests for `FileTree`: click on first visible row when `offset=0` selects entry 0; click when scrolled (`offset=5`) selects `offset+row`; click past end returns false and does not modify cursor; click on directory row succeeds (cursor moves to dir entry); click on negative row returns false
- [x] tests for `TOC`: mirror the above
- [x] run `go test ./app/ui/sidepane/...` — must pass before task 4

### Task 4: Add `hitTest` and `hitZone` classification + layout helpers

**Files:**
- Create: `app/ui/mouse.go`
- Create: `app/ui/mouse_test.go`
- Potentially Modify: `app/ui/model.go` or `app/ui/view.go` (for layout helpers, if they don't already exist)

- [x] check existing code for `statusBarHeight`, `diffTopRow`, `treeTopRow` helpers — reuse if present, otherwise add as methods on `Model` near other layout methods
- [x] `statusBarHeight() int` returns 0 when `m.NoStatusBar`, otherwise the actual number of rows (typically 1)
- [x] `diffTopRow() int` returns the first row of diff viewport content (after diff header). Derive from the same math that `view.go` uses to render
- [x] `treeTopRow() int` returns the first row of tree content (after any pane border). Derive from the same math that `renderTwoPaneLayout` uses
- [x] declare `hitZone` type and constants (`hitNone`, `hitTree`, `hitDiff`, `hitStatus`, `hitHeader`) in `app/ui/mouse.go`
- [x] implement `func (m Model) hitTest(x, y int) hitZone` using the three helpers above — pure arithmetic, no I/O
- [x] unit test table for `hitTest`: tree visible/hidden, single-file without TOC, single-file with TOC, no-status-bar, stdin-mode (tree hidden by default), `(x=treeWidth-1, y)` → `hitTree`, `(x=treeWidth, y)` → `hitDiff`, `y=0` → `hitHeader`, `y=last` → `hitStatus`, `x=width` → `hitNone` (out of bounds)
- [x] unit tests for `statusBarHeight`, `diffTopRow`, `treeTopRow` covering status bar on/off and single-file vs two-pane layout
- [x] run `go test ./app/ui/...` — must pass before task 5

### Task 5: Implement `handleMouse` routing with wheel + left-click

**Files:**
- Modify: `app/ui/mouse.go`
- Modify: `app/ui/model.go` (add `tea.MouseMsg` case in `Update`)
- Modify: `app/ui/mouse_test.go`

- [x] add `tea.MouseMsg` case in `Model.Update` at `app/ui/model.go:662` → delegates to `m.handleMouse(msg)`
- [x] at top of `handleMouse`, clear transient hints mirroring `handleKey` at `model.go:704-706`: `m.commits.hint = ""`, `m.reload.hint = ""`, `m.compact.hint = ""`
- [x] implement swallow checks in the order specified in Solution Overview (`inConfirmDiscard`, `reload.pending`, `annot.annotating`, `search.active`, overlay-open via `m.overlay` check) — all return `m, nil`
- [x] dispatch by `msg.Button`:
  - wheel up/down: compute `step := wheelStep` (const 3); when `msg.Shift`, use `m.layout.viewport.Height/2`; hit-test `(msg.X, msg.Y)`; route to `m.moveDiffCursorDownBy(step)`/`UpBy(step)` for `hitDiff`, `m.tree.Move(sidepane.MotionPageDown, step)` / `MotionPageUp` for `hitTree`, analogous `m.file.mdTOC.Move(MotionPageDown/Up, step)` when TOC active, no-op for `hitStatus`/`hitHeader`/`hitNone`
  - wheel left/right: return `m, nil` (intentionally swallowed)
  - left press (only `msg.Action == tea.MouseActionPress` — skip release/motion): invoke `clickTree` or `clickDiff` based on hit-zone (helpers set focus themselves)
  - any other button or action: return `m, nil`
- [x] implement `clickDiff(x, y int)` helper as shown in Technical Details: uses `visualRowToDiffLine` two-return, sets `m.annot.cursorOnAnnotation` to the `onAnnotation` flag, calls `syncViewportToCursor`, sets `m.layout.focus = paneDiff`
- [x] implement `clickTree(x, y int)` helper as shown in Technical Details: routes to `m.file.mdTOC.SelectByVisibleRow` when mdTOC active, else `m.tree.SelectByVisibleRow`; clears pending jumps; calls `loadSelectedIfChanged`; sets focus
- [x] wheel tests: wheel in diff pane → cursor moves 3 lines; shift+wheel in diff → cursor moves `viewport.Height/2`; wheel in tree pane → tree cursor moves 3; wheel in tree with focus on diff → tree cursor still moves (routing is by cursor-position, not by current focus); wheel in TOC pane (markdown file, full-context) → TOC cursor moves; wheel-left / wheel-right → no-op
- [x] click tests: click in diff at row N → `diffCursor` lands on matching diff-line, `cursorOnAnnotation` set correctly; click on annotation sub-row sets `cursorOnAnnotation=true`; click in tree → tree selection changes + file load triggered; click on directory entry in tree → cursor moves but no file load; click in TOC pane (mdTOC active) → TOC selection changes; click in status zone → no-op; click in diff header → no-op; click in out-of-bounds zone → no-op
- [x] button filter tests: right-click → no-op; middle-click → no-op; left release (no press) → no-op; mouse motion → no-op; back/forward buttons → no-op
- [x] modal swallow tests: wheel while annotating → no-op; click while annotating → no-op, annotation input unchanged; wheel while overlay open → no-op, overlay stays open; click while overlay open → no-op; wheel while search active → no-op; click while `reload.pending` → no-op; click while `inConfirmDiscard` → no-op
- [x] hint-clearing test: wheel event with `m.commits.hint="loading commits..."` — after handler, hint is empty
- [x] focus side-effect tests: click in tree while focused on diff → `layout.focus == paneTree`; click in diff while focused on tree → `layout.focus == paneDiff`
- [x] stdin-mode test: tree is hidden in stdin mode; click at `(x=0, y=1)` → `hitDiff`, not `hitTree`; wheel and click behave as diff-only
- [x] run `go test ./app/ui/...` — must pass before task 6

### Task 6: Documentation sync (README + site + ARCHITECTURE + plugin references)

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `.claude-plugin/skills/revdiff/references/config.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff/references/config.md`
- Modify: `plugins/codex/skills/revdiff/references/usage.md`

- [x] README: add a "Mouse support" subsection covering wheel, click-to-select in tree, click-to-set-cursor in diff, Shift+wheel half-page, `--no-mouse` opt-out
- [x] README: add explicit text-selection note in that subsection — something like: "Text selection requires Shift+drag (most terminals) or Option+drag (iTerm2). Because the tree pane is rendered alongside the diff on the same rows, multi-line Shift+drag will include tree content. For clean copies of diff text, use your terminal's block-select mode: Option+drag in iTerm2, Ctrl+Shift+drag in kitty, or run with `--no-mouse` to disable mouse capture entirely."
- [x] README: add `--no-mouse` to the flags table (placement consistent with `--no-colors`, `--no-status-bar`)
- [x] `site/docs.html`: mirror README Mouse section and flag entry (including the text-selection note)
- [x] `docs/ARCHITECTURE.md`: add one paragraph under the `app/ui/` description noting the new `mouse.go` file and its role (hit-testing + event routing), matching the file-by-file breakdown style
- [x] `.claude-plugin/skills/revdiff/references/config.md`: add `--no-mouse` / `REVDIFF_NO_MOUSE` to config options
- [x] `.claude-plugin/skills/revdiff/references/usage.md`: add mouse interactions (wheel, click) to the interactions list, plus the Shift+drag / block-select note
- [x] `plugins/codex/skills/revdiff/references/config.md`: byte-identical copy of the claude-plugin version
- [x] `plugins/codex/skills/revdiff/references/usage.md`: byte-identical copy of the claude-plugin version
- [x] verify byte-identity: `diff -q .claude-plugin/skills/revdiff/references/config.md plugins/codex/skills/revdiff/references/config.md` and same for usage.md — both must produce no output
- [x] no test changes needed (documentation task)

*Note: CHANGELOG.md is updated at release-tag time (see commit pattern `c564016 docs: update changelog, site, and plugin versions for v0.22.0` — changelog updates land with version bumps, not with feature PRs). Do not modify CHANGELOG.md in this task.*

### Task 7: Plugin version bumps (ask user first)

**Files:**
- Potentially modify: `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `package.json`

- [x] per project memory "plugin-version-bump": ask user whether to bump Claude plugin version (both `plugin.json` and `marketplace.json`) after modifying `.claude-plugin/skills/revdiff/references/*.md`
- [x] ask user whether to bump pi plugin version in `package.json` after modifying `plugins/pi/` (only if any pi files changed — this task does not modify pi by default, so likely no-op)
- [x] no test changes needed

### Task 8: Verify acceptance criteria

- [x] verify scroll wheel works in both panes (automated test via `tea.MouseMsg` in Task 5 — re-run here)
- [x] verify click in tree selects + loads file
- [x] verify click in diff sets cursor, enabling "click then `a`" annotation flow
- [x] verify modal-state swallow: no mouse interference during annotation, search, pending-reload, confirm-discard, or any open overlay
- [x] verify `--no-mouse` compiles, runs, and disables all mouse behavior (unit test from Task 1)
- [x] run full test suite: `go test ./...`
- [x] run `go test -race ./...`
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [x] run `~/.claude/format.sh`
- [x] verify test coverage did not regress (`make test`)

### Task 9: Final documentation wrap-up

**Files:**
- Potentially modify: `CLAUDE.md`

- [x] update `CLAUDE.md` Gotchas section ONLY if implementation revealed a non-obvious nesting or coordinate issue (e.g., mouse X/Y vs wrapped-ANSI rendering quirk). Otherwise skip — the code is discoverable and a gotcha note would be noise. *(skipped: coordinate math is encapsulated in `diffTopRow`/`treeTopRow`/`hitTest` with comments explaining the magic numbers; no quirk worth a gotcha)*
- [x] move this plan to `docs/plans/completed/`
- [x] no test changes needed

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational only.*

**Manual terminal verification** (CI cannot test mouse escape sequences):

- **kitty**: wheel in both panes, click in tree, click in diff, Shift+wheel half-page, `--no-mouse` text-selection
- **iTerm2**: same scenarios (this is the terminal the bug report mentioned — verify scroll now works consistently)
- **Ghostty**: same scenarios
- **Alacritty**: same scenarios
- **tmux with `mouse on`** and with `mouse off`: verify behavior in both, confirm the README note about `set -g mouse on`
- **Zellij floating pane**: same scenarios
- **SSH into a remote box** (iTerm2 → ssh → revdiff): verify mouse tracking still works — some SSH + tmux combos drop the escape sequences. If broken, recommend `--no-mouse` via env var.

**Text-selection trade-off communication**: the README change documents the Shift-select requirement. If user reports friction, consider whether the default should flip to `mouse off` — but the memory of "text selection captured by default" across other TUIs (vim, htop, lazygit) argues against that.

**Release notes**: when cutting the next revdiff release, mention mouse support as a headline feature plus the `--no-mouse` opt-out in CHANGELOG.md and the site hero badge.
