# app/ui/sidepane/ Sub-Package Extraction

## Overview

Extract a new `app/ui/sidepane/` sub-package from `app/ui/` that owns **all left-pane navigation components** — the file tree and the markdown table-of-contents. Both currently live in `app/ui/` as package-private types (`fileTree`, `mdTOC`) that share the `paneTree` layout slot and a common `ensureVisibleInList` helper.

This is a **follow-up** to the completed `app/ui/style/` extraction (see `docs/plans/completed/2026-04-10-style-extraction.md`) and inherits its Design Philosophy section wholesale. The style plan explicitly lists `filetree` and `mdtoc` as follow-up candidates; this plan bundles them into a single `sidepane` package because they are semantic siblings, not arbitrary "what's easy to move" groupings.

**Problem it solves:**

- `app/ui/filetree.go` (500 LOC) and `app/ui/mdtoc.go` (227 LOC) plus their test files bloat the `ui` package. Both are self-contained navigation components that expose methods invoked from Model handlers/view/loaders — a clean seam that the current flat package structure obscures.
- Model holds direct access to private fields (`m.tree.reviewed`, `m.tree.allFiles`, `m.tree.fileStatuses`, `m.tree.filter`, `m.mdTOC.cursor`, `m.mdTOC.entries`, `m.mdTOC.activeSection`) in ~10 call sites, which couples Model to implementation details of both types. Extraction forces all access through exported methods.
- The `ensureVisibleInList` helper sits in `filetree.go` but is called by both `fileTree.ensureVisible` and `mdTOC.ensureVisible` — a latent abstraction that naturally lifts into a shared sub-package.
- Both types have method-explosion patterns (`moveUp`/`moveDown`/`pageUp`/`pageDown`/`moveToFirst`/`moveToLast`) that are prime candidates for Design Philosophy #3 (parameterized accessors via enum).

**Why bundle into one package instead of two:**

- `fileTree` and `mdTOC` are semantic siblings — they occupy the same `paneTree` layout slot. `view.go:renderTwoPaneLayout` treats them as alternates. `togglePane()` and `handleTreeNav()` route to one or the other.
- They share the `ensureVisibleInList` helper, which would need relocation once regardless.
- Doing both at once establishes the "navigation pane" template firmly while context is fresh, and keeps ui-side migration churn in one coherent commit per milestone rather than spread across two plans.
- They are distinct enough that merging into a single `NavPane` type would force a fake abstraction — they stay as separate `FileTree` and `TOC` types inside the `sidepane` package, not a single aggregate.

## Context (from discovery)

**Current state of affected code** (on master):

- `app/ui/filetree.go` — 500 LOC. Defines `fileTree`, `treeEntry`, `renderCtx`, `newFileTree`, `newFileTreeFromEntries`, `ensureVisibleInList` (shared helper), plus ~22 methods: `selectedFile`, `ensureVisible`, `moveDown`, `moveUp`, `pageDown`, `pageUp`, `moveToFirst`, `moveToLast`, `nextFile`, `prevFile`, `hasNextFile`, `hasPrevFile`, `fileIndices`, `render`, `renderFileEntry`, `filterFiles`, `toggleFilter`, `selectByPath`, `restoreReviewed`, `truncateDirName`, `refreshFilter`, `toggleReviewed`, `reviewedCount`.
- `app/ui/filetree_test.go` — 664 LOC. Comprehensive coverage of the type.
- `app/ui/mdtoc.go` — 227 LOC. Defines `tocEntry`, `mdTOC`, `parseTOC` (factory), plus ~7 methods: `moveUp`, `moveDown`, `ensureVisible`, `updateActiveSection`, `render`, `truncateTitle`. Plus `fencePrefix` (package function), and two Model receivers `isFullContext`/`isMarkdownFile` which are pure (don't touch Model state) and would logically move into sidepane's caller-side guard logic or stay in ui as helpers.
- `app/ui/mdtoc_test.go` — 489 LOC.
- `app/ui/model.go:96-176` — Model struct fields: `tree fileTree` (value type), `mdTOC *mdTOC` (pointer, nil when not markdown-full-context).

**Call-site audit** (production files only, tests separately):

| File | Sites touching `m.tree.*` or `m.mdTOC.*` |
|---|---:|
| `app/ui/diffnav.go` | 24 (8 fileTree nav, 4 file step/check, 8 mdTOC cursor/entries, 4 mdTOC method) |
| `app/ui/loaders.go` | 9 (tree rebuild, field reads, ParseTOC) |
| `app/ui/handlers.go` | 9 (filter/reviewed/nextFile/prevFile + mdTOC cursor access) |
| `app/ui/view.go` | 6 (render, FilterActive/TotalFiles/ReviewedCount) |
| `app/ui/annotate.go` | 5 (refreshFilter, selectedFile) |
| `app/ui/annotlist.go` | 1 (selectByPath) |
| `app/ui/model.go` | 1 (ensureVisible) |

Field accesses that need to disappear behind new getter methods:
- `loaders.go:113` — `m.tree.reviewed` (eliminated entirely by new `Rebuild` method)
- `loaders.go:120`, `view.go:140` — `len(m.tree.allFiles)` → `TotalFiles()`
- `loaders.go:155` — `m.tree.fileStatuses[file]` → `FileStatus(path)`
- `view.go:327` — `m.tree.filter` → `FilterActive()`
- `diffnav.go:525,527,573,580,581,587,590,599` — `m.mdTOC.cursor`, `m.mdTOC.entries`, `m.mdTOC.activeSection`
- `handlers.go:283,285` — `m.mdTOC.cursor`, `m.mdTOC.entries`

**Dependencies:**

- `github.com/go-pkgz/enum` — already added to `go.mod` from style extraction. No new deps.

**Reference material** (from style extraction):

- `docs/plans/completed/2026-04-10-style-extraction.md` — Design Philosophy section (lines 83–170) is inherited wholesale. Read it before starting.
- `app/ui/style/enums.go`, `app/ui/style/color_key_enum.go`, `app/ui/style/style_key_enum.go` — enum generator pattern to replicate for `Motion` and `Direction`.
- `app/ui/model.go:45-79` — consumer-side interface pattern (`styleResolver`, `styleRenderer`, `sgrProcessor`) to follow for `fileTreeComponent` / `tocComponent`.

## Development Approach

- **testing approach**: Regular — implementation first, then tests for each unit, matching the style plan.
- **Complete each milestone fully before moving to the next.** Individual tasks within a milestone may leave the codebase in an intermediate broken state — that's allowed and expected. See the "Testing Policy" section below.
- **Small focused changes within each task**. No bundling unrelated changes.
- **CRITICAL: update this plan file when scope changes during implementation**.
- **Single-level task numbering** (Task 1 through Task N, no hierarchical Task 1.1/1.2 etc). Ralphex executes flat numbering sequences.

## Testing Policy — CRITICAL: Milestone Gating, Not Per-Task

**This section deliberately overrides the default planning policy of "all tests must pass before starting next task".**

This refactor is a single coordinated extraction touching ~55 production call sites plus matching test updates. It is structurally impossible for every intermediate task to leave `go test ./... -race` green, because:

- The new `sidepane.FileTree` and `sidepane.TOC` types must exist before any call site can migrate to them.
- Old `app/ui/filetree.go` and `app/ui/mdtoc.go` must be deleted before the new types can fully replace them.
- Between those two points, some call sites call the new API and some still call the old — neither state is consistent.

**The correct gating is per-milestone.** Three milestones, each with a specific green-gate:

| Milestone | What's built | Green-gate |
|---|---|---|
| **M1: Sidepane package skeleton** | New `app/ui/sidepane/` package complete and self-contained: types, enums, methods, tests. `ui` package is NOT yet migrated. | `go test ./app/ui/sidepane/... -race` green + `golangci-lint run app/ui/sidepane/...` clean. **Full project build may be broken** — expected. |
| **M2: Call-site migration** | All ui call sites updated to new API. Old `app/ui/filetree.go`, `filetree_test.go`, `mdtoc.go`, `mdtoc_test.go` deleted. | Full `go test ./... -race` green. `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` clean. Manual smoke test on a git repo (file tree navigation + markdown TOC navigation both work). |
| **M3: Docs + cleanup** | `app/ui/doc.go` and project `CLAUDE.md` updated. Orphan references audited. Plan moved to completed/. | Same as M2 gate + doc cross-check. |

**Non-milestone tasks do NOT have "run tests — must pass" checkboxes.** They may note "may leave N tests failing until milestone gate". The milestone task is where convergence happens.

**Commit structure**: one git commit per milestone (3 total). Each milestone commit is a coherent unit that leaves the tree green (M2/M3) or at least leaves `sidepane/` green (M1).

**If hit failing tests inside a non-milestone task — that is expected. Proceed to the next task. Only escalate if a MILESTONE task leaves failures.**

## Design Philosophy (Inherited from Style Plan)

All 10 design principles from `docs/plans/completed/2026-04-10-style-extraction.md` (Design Philosophy section) apply here unchanged. The ones most relevant to this extraction:

1. **Sub-packages own their domain completely** — `sidepane` owns all navigation cursor state, viewport offset management, entry parsing, and rendering logic. Callers express intent ("move to next file") and never touch cursor indices or entry slices.
2. **Named types for domain values** — `Motion`, `Direction`, `FileTreeRender`, `TOCRender` — all explicit types, not raw primitives or positional params.
3. **Parameterized accessors over method explosion** — `Move(Motion, count)` collapses 6 nav methods into 1. `StepFile(Direction)` and `HasFile(Direction)` collapse 4 file-nav methods into 2 while keeping query/mutation separated (Go idiom — don't unify query and mutation under one method just to save API surface).
4. **Methods over standalone functions** — the one narrow exception is `ensureVisible(cursor, offset *int, count, height int)` which operates on primitive pointers and is genuinely shared by both `FileTree.EnsureVisible` and `TOC.EnsureVisible`. No natural type owner, so package function is justified.
5. **Consumer-side interfaces** — two levels:
    - `sidepane` package defines its own `Resolver` and `Renderer` interfaces as narrow views of `style` (consumer-side from sidepane's perspective, loose-coupling from style's full surface, enables sidepane to test in isolation).
    - `ui/model.go` defines `fileTreeComponent` and `tocComponent` interfaces (consumer-side from ui's perspective, loose-coupling from sidepane concrete types). Model holds them as interface-typed fields.
6. **Avoid fragmenting related data/behavior** — core types, shared helpers, and request structs stay in `sidepane.go`. FileTree-specific code in `filetree.go`, TOC-specific in `toc.go`. Don't split further.
7. **Milestone-based test gating** — see Testing Policy above.
8. **Enum generator for type-safe keys** — `Motion` and `Direction` both use `go-pkgz/enum` for `Values`/`String`/exhaustiveness tests.
9. **Comprehensive tests from day one** — no file without its `_test.go` counterpart.
10. **Dependency injection without fallbacks** — sidepane constructors (`NewFileTree`, `ParseTOC`) take their required inputs directly; no fallback construction.

**Additional decision for this extraction:**

**No compile-time assertions** (`var _ fileTreeComponent = (*sidepane.FileTree)(nil)`). Rationale: the assignment sites `m.tree = sidepane.NewFileTree(entries)` and `m.mdTOC = toc` already serve as compile-time checks — if `*sidepane.FileTree` or `*sidepane.TOC` ever stops satisfying the interface, the compiler flags it there with a clear error. The redundant assertion adds ceremony without benefit. The style package has them but that was pre-discovery; dropping them for sidepane reflects the lesson.

**No moq mocks for the consumer-side interfaces in `ui`.** Rationale: interfaces exist for decoupling, not for test injection. Model tests currently construct real `fileTree` and `mdTOC` values and assert on real state — that still works. Mocking a 14-method data container via moq adds test maintenance burden for zero gain.

## Solution Overview

**Package layout:**

```
app/ui/sidepane/
├── sidepane.go           — package types: Motion/Direction enums, ensureVisible helper,
│                           Resolver/Renderer interfaces, FileTreeRender/TOCRender structs
├── filetree.go           — FileTree type + methods + NewFileTree factory
├── toc.go                — TOC type + methods + ParseTOC factory
├── sidepane_test.go      — enum exhaustiveness, ensureVisible table-driven
├── filetree_test.go      — ported from app/ui/filetree_test.go, adapted to new API
├── toc_test.go           — ported from app/ui/mdtoc_test.go, adapted to new API
├── motion_enum.go        — generated via go-pkgz/enum
└── direction_enum.go     — generated via go-pkgz/enum
```

**FileTree public API** (14 methods + constructor + Rebuild):

```go
// Constructor
func NewFileTree(entries []diff.FileEntry) *FileTree

// Query
func (ft *FileTree) SelectedFile() string
func (ft *FileTree) TotalFiles() int
func (ft *FileTree) FileStatus(path string) diff.FileStatus
func (ft *FileTree) FilterActive() bool
func (ft *FileTree) ReviewedCount() int
func (ft *FileTree) HasFile(dir Direction) bool  // no-wrap check

// Navigation
func (ft *FileTree) Move(m Motion, count ...int) // count is variadic: page motions use count[0], others ignore it
func (ft *FileTree) StepFile(dir Direction)      // wraps at ends
func (ft *FileTree) SelectByPath(path string) bool
func (ft *FileTree) EnsureVisible(height int)

// Mutation
func (ft *FileTree) Rebuild(entries []diff.FileEntry)  // preserves reviewed map (filtered to entries that still exist), resets cursor/offset
func (ft *FileTree) ToggleFilter(annotated map[string]bool)
func (ft *FileTree) RefreshFilter(annotated map[string]bool)
func (ft *FileTree) ToggleReviewed(path string)

// Render
func (ft *FileTree) Render(r FileTreeRender) string
```

**TOC public API** (10 methods + constructor):

```go
// Constructor
func ParseTOC(lines []diff.DiffLine, filename string) *TOC  // returns nil when no headers

// Query
func (t *TOC) CurrentLineIdx() (int, bool)  // hides entries[cursor].lineIdx
func (t *TOC) NumEntries() int

// Navigation
func (t *TOC) Move(m Motion, count ...int) // variadic count; page motions use count[0], others ignore
func (t *TOC) EnsureVisible(height int)

// Active section
func (t *TOC) UpdateActiveSection(diffCursor int)
func (t *TOC) SyncCursorToActiveSection()  // sets cursor = activeSection when activeSection >= 0

// Render
func (t *TOC) Render(r TOCRender) string
```

**Enums:**

```go
type Motion int
const (
    MotionUnknown Motion = iota
    MotionUp
    MotionDown
    MotionPageUp     // uses count[0] from variadic
    MotionPageDown   // uses count[0] from variadic
    MotionFirst
    MotionLast
)

type Direction int
const (
    DirUnknown Direction = iota
    DirNext
    DirPrev
)
```

**Note on `Move(m Motion, count ...int)` signature**: `count` is variadic specifically to make call sites read naturally — page motions that need a count write `Move(MotionPageDown, m.treePageSize())`, while single-step and jump motions write `Move(MotionDown)` without a trailing `0`. Inside `Move`, page cases take `count[0]` (panic-safe guard: treat missing/empty count as 1 for page motions, which is "page of size 1" = single step and is harmless). Document this contract in the method godoc so future maintainers don't misuse it. This is a narrow, intentional use of variadic to simulate an optional parameter; Go linters may flag it but the readability win is concrete (8 diffnav.go call sites become terser).

**Render option structs:**

```go
type FileTreeRender struct {
    Width     int
    Height    int
    Annotated map[string]bool
    Resolver  Resolver
    Renderer  Renderer
}

type TOCRender struct {
    Width    int
    Height   int
    Focused  bool
    Resolver Resolver
}
```

**Consumer-side interfaces in sidepane** (narrow view of style):

```go
// Resolver is what sidepane needs for style lookups.
// Satisfied by ui's styleResolver interface (which is satisfied by *style.Resolver).
type Resolver interface {
    Style(k style.StyleKey) lipgloss.Style
    Color(k style.ColorKey) style.Color  // if needed by render paths
}

// Renderer is what sidepane needs for compound ANSI rendering (FileTree only — TOC doesn't use it).
type Renderer interface {
    FileStatusMark(status diff.FileStatus) string
    FileReviewedMark() string
    FileAnnotationMark() string
}
```

**Consumer-side interfaces in ui (exported, in model.go or a new deps.go):**

Interfaces must be **exported** because main.go needs to spell the return type in the factory closures it injects into `ModelConfig`. The interfaces are consumer-side (defined by ui, the consumer), not provider-side — sidepane does not know they exist.

```go
// FileTreeComponent is what Model needs from a file-tree navigation component.
// Implemented by *sidepane.FileTree. Exported so main.go can spell it in the
// factory closure's return type.
type FileTreeComponent interface {
    SelectedFile() string
    TotalFiles() int
    FileStatus(path string) diff.FileStatus
    FilterActive() bool
    ReviewedCount() int
    HasFile(dir sidepane.Direction) bool
    Move(m sidepane.Motion, count ...int)
    StepFile(dir sidepane.Direction)
    SelectByPath(path string) bool
    EnsureVisible(height int)
    Rebuild(entries []diff.FileEntry)
    ToggleFilter(annotated map[string]bool)
    RefreshFilter(annotated map[string]bool)
    ToggleReviewed(path string)
    Render(r sidepane.FileTreeRender) string
}

// TOCComponent is what Model needs from a table-of-contents navigation component.
// Implemented by *sidepane.TOC. Exported so main.go can spell it in the factory closure.
type TOCComponent interface {
    CurrentLineIdx() (int, bool)
    NumEntries() int
    Move(m sidepane.Motion, count ...int)
    EnsureVisible(height int)
    UpdateActiveSection(diffCursor int)
    SyncCursorToActiveSection()
    Render(r sidepane.TOCRender) string
}
```

**Rationale recap on data-object vs behavior imports**: `ui/model.go` importing `sidepane` for type names (`sidepane.Motion`, `sidepane.Direction`, `sidepane.FileTreeRender`, `sidepane.TOCRender`) is a **data-object import** — Model references stable value types in interface signatures. It does NOT call any sidepane constructor or stateful function. `sidepane.NewFileTree` and `sidepane.ParseTOC` — the **concrete behavior imports** — live only in `app/main.go`, wired via `ModelConfig` factory closures. The "loose coupling" goal is met: changing `sidepane.NewFileTree`'s internal implementation or signature does NOT touch Model code (only main.go). Adding a new motion value requires updating the `Motion` enum and the `Move` switch in sidepane — Model signatures don't change. Data-object imports are cheap and safe; behavior imports are the actual coupling risk this plan avoids.

**Note on exports:** the interfaces reference `sidepane.Direction`, `sidepane.Motion`, `sidepane.FileTreeRender`, `sidepane.TOCRender` in their method signatures. This means the `ui` package DOES import `sidepane` — but only for type names in interface method signatures, never for constructor calls. Concrete construction happens via injected factories (see below), so Model never calls `sidepane.NewFileTree(...)` or `sidepane.ParseTOC(...)` directly. The "loose coupling" goal is met: Model can switch to a different sidepane implementation by swapping the factory, without the Model code changing.

Model fields:
```go
type Model struct {
    tree  FileTreeComponent  // holds *sidepane.FileTree at runtime
    mdTOC TOCComponent       // holds *sidepane.TOC, nil when not markdown-full-context
    // factory closures injected via ModelConfig, invoked from loaders.go
    newFileTree func(entries []diff.FileEntry) FileTreeComponent
    parseTOC    func(lines []diff.DiffLine, filename string) TOCComponent
    ...
}
```

**Factory injection via ModelConfig:**

```go
type ModelConfig struct {
    ...
    // NewFileTree constructs a fresh FileTreeComponent from the file list.
    // Injected by main.go (typically a closure wrapping sidepane.NewFileTree).
    // Required — NewModel returns an error when nil.
    NewFileTree func(entries []diff.FileEntry) FileTreeComponent

    // ParseTOC parses markdown headers from diff lines into a TOCComponent.
    // Returns nil when no headers are found (typed-nil trap handled by the closure).
    // Injected by main.go (typically a closure wrapping sidepane.ParseTOC).
    // Required — NewModel returns an error when nil.
    ParseTOC func(lines []diff.DiffLine, filename string) TOCComponent
}
```

**main.go wiring:**

```go
import (
    "github.com/umputun/revdiff/app/ui"
    "github.com/umputun/revdiff/app/ui/sidepane"
)

cfg := ui.ModelConfig{
    ...
    NewFileTree: func(entries []diff.FileEntry) ui.FileTreeComponent {
        return sidepane.NewFileTree(entries)
    },
    ParseTOC: func(lines []diff.DiffLine, filename string) ui.TOCComponent {
        toc := sidepane.ParseTOC(lines, filename)
        if toc == nil {
            return nil // explicit nil interface — avoids typed-nil-vs-interface-nil trap
        }
        return toc
    },
}
```

The explicit `if toc == nil { return nil }` guard in the `ParseTOC` closure is critical: returning a typed-nil `*sidepane.TOC` from a closure with return type `ui.TOCComponent` creates an interface value that is not-nil (carries type info + nil pointer). Explicit nil return collapses it to a truly nil interface, so `m.mdTOC != nil` checks inside Model work correctly.

## Technical Details

**`Rebuild` semantics:**

Old pattern in `loaders.go:handleFilesLoaded`:
```go
oldReviewed := m.tree.reviewed
m.tree = newFileTreeFromEntries(entries)
m.tree.restoreReviewed(oldReviewed)
if m.currFile != "" {
    m.tree.selectByPath(m.currFile)
}
```

New pattern (uses injected factory only in NewModel for init, uses Rebuild thereafter):
```go
// in NewModel (Task 7):
m.tree = cfg.NewFileTree(nil) // empty tree for nil-safety before first load

// in handleFilesLoaded (Task 8):
m.tree.Rebuild(entries)
if m.currFile != "" {
    m.tree.SelectByPath(m.currFile)
}
```

`m.tree` is always non-nil after `NewModel`, so `loaders.go` never needs to check. `Rebuild` on an empty tree produces the same result as `NewFileTree(entries)` — the two paths converge.

Inside `Rebuild`:
1. Build new entries from `entries` (same logic as `buildEntries`).
2. Prune `reviewed` map: drop keys that are no longer in `entries`.
3. Reset `cursor` and `offset` to zero and position cursor on first file entry.
4. Preserve `fileStatuses` from new entries; drop stale keys.
5. Leave `filter` state as-is (if user had filter on, keep it on after rebuild — matches current behavior of refreshFilter).

**Typed-nil trap for `mdTOC`:**

`sidepane.ParseTOC` returns `*sidepane.TOC` (typed, nil when no headers found). The factory closure in main.go is responsible for collapsing a typed-nil `*TOC` into a truly nil interface before it reaches Model:

```go
// main.go — closure wiring
ParseTOC: func(lines []diff.DiffLine, filename string) ui.TOCComponent {
    toc := sidepane.ParseTOC(lines, filename)
    if toc == nil {
        return nil // explicit nil interface
    }
    return toc
},
```

Inside Model (`loaders.go:handleFileLoaded`), the call site then works the obvious way:

```go
m.mdTOC = m.parseTOC(msg.lines, msg.file) // nil when no headers, non-nil interface otherwise
```

All `m.mdTOC != nil` checks elsewhere in ui code continue to work because `m.mdTOC` either holds a non-nil interface wrapping a valid `*TOC`, or is a truly nil interface.

**`isMarkdownFile` / `isFullContext` helpers:**

These are Model methods today but don't touch Model state. They're called only from `handleFileLoaded` before `ParseTOC`. Two options:
- Keep them as Model methods in ui package (they're ui-side pre-checks, not sidepane concerns)
- Move them into sidepane as package functions

**Decision:** keep them in ui. They guard the `ParseTOC` call site and are pure string/slice predicates with no sidepane dependency. Moving them would pull Model-independent helpers into sidepane for no gain.

**`ensureVisible` helper scope:**

```go
// ensureVisible adjusts offset so cursor is within the visible range of given height.
// Shared by FileTree.EnsureVisible and TOC.EnsureVisible.
func ensureVisible(cursor, offset *int, count, height int) {
    if height <= 0 {
        return
    }
    switch {
    case *cursor < *offset:
        *offset = *cursor
    case *cursor >= *offset+height:
        *offset = *cursor - height + 1
    }
    if *offset < 0 {
        *offset = 0
    }
    if maxOff := max(count-height, 0); *offset > maxOff {
        *offset = maxOff
    }
}
```

Lives in `sidepane.go`. Unexported. Called only by methods inside the same package.

**Migration checklist for diffnav.go (largest migration):**

| Line | Old | New |
|---|---|---|
| 385 | `m.tree.hasNextFile()` | `m.tree.HasFile(sidepane.DirNext)` |
| 388 | `m.tree.nextFile()` | `m.tree.StepFile(sidepane.DirNext)` |
| 392 | `m.tree.hasPrevFile()` | `m.tree.HasFile(sidepane.DirPrev)` |
| 395 | `m.tree.prevFile()` | `m.tree.StepFile(sidepane.DirPrev)` |
| 473 | `m.tree.moveDown()` | `m.tree.Move(sidepane.MotionDown)` |
| 475 | `m.tree.moveUp()` | `m.tree.Move(sidepane.MotionUp)` |
| 477 | `m.tree.pageDown(m.treePageSize())` | `m.tree.Move(sidepane.MotionPageDown, m.treePageSize())` |
| 479 | `m.tree.pageDown(max(1, m.treePageSize()/2))` | `m.tree.Move(sidepane.MotionPageDown, max(1, m.treePageSize()/2))` |
| 481 | `m.tree.pageUp(m.treePageSize())` | `m.tree.Move(sidepane.MotionPageUp, m.treePageSize())` |
| 483 | `m.tree.pageUp(max(1, m.treePageSize()/2))` | `m.tree.Move(sidepane.MotionPageUp, max(1, m.treePageSize()/2))` |
| 485 | `m.tree.moveToFirst()` | `m.tree.Move(sidepane.MotionFirst)` |
| 487 | `m.tree.moveToLast()` | `m.tree.Move(sidepane.MotionLast)` |
| 496 | `m.tree.ensureVisible(m.treePageSize())` | `m.tree.EnsureVisible(m.treePageSize())` |
| 505 | `m.mdTOC.moveDown()` | `m.mdTOC.Move(sidepane.MotionDown)` |
| 507 | `m.mdTOC.moveUp()` | `m.mdTOC.Move(sidepane.MotionUp)` |
| 509-515 | `for range ...  { m.mdTOC.moveDown() }` | `m.mdTOC.Move(sidepane.MotionPageDown, m.treePageSize())` / `MotionPageDown, max(1, m.treePageSize()/2)` |
| 516-523 | `for range ...  { m.mdTOC.moveUp() }` | `m.mdTOC.Move(sidepane.MotionPageUp, ...)` |
| 525 | `m.mdTOC.cursor = 0` | `m.mdTOC.Move(sidepane.MotionFirst)` |
| 527 | `m.mdTOC.cursor = max(0, len(m.mdTOC.entries)-1)` | `m.mdTOC.Move(sidepane.MotionLast)` |
| 535 | `m.mdTOC.ensureVisible(...)` | `m.mdTOC.EnsureVisible(...)` |
| 573 | `m.mdTOC.cursor = max(0, min(m.mdTOC.cursor+delta, len(m.mdTOC.entries)-1))` | `m.mdTOC.Move(MotionUp)` or `Move(MotionDown)` by sign — add inline code comment `// jumpTOCEntry always passes delta ±1; if that ever changes, extend here` to lock the contract |
| 580-582 | guarded `m.mdTOC.cursor = m.mdTOC.activeSection` | `m.mdTOC.SyncCursorToActiveSection()` |
| 587 | `m.mdTOC.cursor >= len(m.mdTOC.entries)` | handled inside `CurrentLineIdx()` (returns ok=false) |
| 590 | `m.mdTOC.entries[m.mdTOC.cursor].lineIdx` | `idx, ok := m.mdTOC.CurrentLineIdx(); if !ok { return }` |
| 592, 599 | `m.mdTOC.updateActiveSection(...)` | `m.mdTOC.UpdateActiveSection(...)` |

Similar mechanical list for handlers.go (lines 283-288, 347-348, 359-364, 392-394), loaders.go (lines 90-120, 137, 155), view.go (lines 44, 49, 139-140, 327, 334), annotate.go (lines 138, 159, 179, 181, 213, 216), annotlist.go (line 254), model.go (line 620).

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): package creation, type implementation, test porting, call-site migration, doc updates.
- **Post-Completion** (no checkboxes): manual smoke test scenarios for M2 milestone gate.

## Implementation Steps

### Task 1: Bootstrap sidepane package

Create the package shell, generator directives, shared helper, interfaces, and request structs. No FileTree or TOC implementation yet.

**Files:**
- Create: `app/ui/sidepane/sidepane.go`
- Create: `app/ui/sidepane/sidepane_test.go`
- Create: `app/ui/sidepane/motion_enum.go` (after go generate)
- Create: `app/ui/sidepane/direction_enum.go` (after go generate)

- [x] create `app/ui/sidepane/sidepane.go` with package doc comment describing the sub-package
- [x] verify the `go-pkgz/enum` version pin used in `app/ui/style/enums.go` (check both `styleKey` and `colorKey` directives) and use the exact same version in sidepane directives — avoids drift between the two sub-packages
- [x] define `motion` and `direction` unexported enum types in `sidepane.go` with matching `//go:generate go run github.com/go-pkgz/enum@<version> -type motion` and `-type direction` directives (mirrors `app/ui/style/enums.go` pattern)
- [x] run `go generate ./app/ui/sidepane/...` to produce `motion_enum.go` and `direction_enum.go` (generates `Motion`/`Direction` exported types with `Values`/`String`/parse helpers)
- [x] add shared `ensureVisible(cursor, offset *int, count, height int)` unexported helper (copied from `app/ui/filetree.go:ensureVisibleInList`)
- [x] define consumer-side `Resolver` interface (narrow view of style — only the methods sidepane render paths actually call)
- [x] define consumer-side `Renderer` interface (FileStatusMark, FileReviewedMark, FileAnnotationMark — FileTree only)
- [x] define `FileTreeRender` struct with Width/Height/Annotated/Resolver/Renderer fields
- [x] define `TOCRender` struct with Width/Height/Focused/Resolver fields
- [x] write table-driven test for `ensureVisible` covering: cursor-above-viewport, cursor-below-viewport, cursor-in-range, empty count, count-smaller-than-height, negative height
- [x] write exhaustiveness test iterating `MotionValues` and `DirectionValues` asserting `String()` returns non-empty for every value
- [x] verify `go test ./app/ui/sidepane/... -race` green for the scaffold (only exhaustiveness + ensureVisible tests should be running so far)

### Task 2: Implement FileTree type and all methods

Port `app/ui/filetree.go` logic into the new package, adapting method names, and add `Rebuild`. Tests for this type live in Task 3 (separate task). This is a deliberate deviation from Design Philosophy #9 ("tests alongside source") because the port copies existing well-covered tests wholesale — the split is organizational, not a gap in coverage. Task 6 (M1 gate) verifies coverage ≥ 80%.

**Files:**
- Create: `app/ui/sidepane/filetree.go`

- [x] before writing code: grep `app/ui/` for all callers of `newFileTree(` and `newFileTreeFromEntries(` — verify no production caller passes `[]string`; confirm all production call sites already use `[]diff.FileEntry` (the `[]string` form is only used internally by `newFileTreeFromEntries` → `newFileTree`). If any production caller needs the `[]string` form, document and extend `NewFileTree` signature before proceeding.
- [x] create `app/ui/sidepane/filetree.go` with package `sidepane` header
- [x] define `FileTree` struct (fields: entries, cursor, offset, allFiles, filter, reviewed, fileStatuses) and `treeEntry`/`renderCtx` unexported types (copied from old file)
- [x] implement `NewFileTree(entries []diff.FileEntry) *FileTree` (merges old `newFileTree` + `newFileTreeFromEntries`, takes `diff.FileEntry` directly; must handle `entries == nil` gracefully — returns a valid empty `*FileTree` so `NewModel` can construct an initial empty tree for nil-safety)
- [x] implement `buildEntries` as method on `*FileTree` (same logic as old)
- [x] implement query methods: `SelectedFile`, `TotalFiles`, `FileStatus`, `FilterActive`, `ReviewedCount`, `HasFile(Direction)` (HasFile merges old `hasNextFile`/`hasPrevFile` via switch on Direction) — each with godoc starting with the method name
- [x] implement `Move(m Motion, count ...int)` with exhaustive switch — merges `moveUp`/`moveDown`/`pageUp`/`pageDown`/`moveToFirst`/`moveToLast`. Page cases read `count[0]` (with safe fallback if `len(count) == 0`); non-page cases ignore count entirely. Document the variadic contract in godoc.
- [x] implement `StepFile(dir Direction)` — merges `nextFile`/`prevFile`, preserves wrap-around semantic
- [x] implement `SelectByPath(path string) bool`
- [x] implement `EnsureVisible(height int)` calling shared `ensureVisible(&ft.cursor, &ft.offset, len(ft.entries), height)`
- [x] implement `Rebuild(entries []diff.FileEntry)` — NEW method that rebuilds entries in-place, prunes stale keys from `reviewed` and `fileStatuses`, resets cursor/offset, positions cursor on first file entry, preserves `filter` state
- [x] implement mutation methods: `ToggleFilter`, `RefreshFilter`, `ToggleReviewed`
- [x] implement `Render(r FileTreeRender) string` — port `render` + `renderFileEntry` + `truncateDirName` + `filterFiles` logic, using `r.Resolver` and `r.Renderer` instead of direct style field access
- [x] implement `fileIndices` as unexported helper method (used by StepFile)

### Task 3: Port FileTree tests

**Files:**
- Create: `app/ui/sidepane/filetree_test.go`

- [x] copy `app/ui/filetree_test.go` to `app/ui/sidepane/filetree_test.go`, change package header to `package sidepane` (**NOT** `package sidepane_test`) — tests need to touch private fields like `cursor`, `offset`, `reviewed`, `filter`, `entries` for setup and assertions, so they must live in the same package
- [x] adapt test constructors: `newFileTree(...)` / `newFileTreeFromEntries(...)` → `NewFileTree(...)`, changing `[]string` inputs to `[]diff.FileEntry{}`
- [x] rename method calls: `moveDown/moveUp` → `Move(MotionDown)` / `Move(MotionUp)`; `pageDown(n)/pageUp(n)` → `Move(MotionPageDown, n)` / `Move(MotionPageUp, n)`; `moveToFirst/moveToLast` → `Move(MotionFirst)` / `Move(MotionLast)`
- [x] replace `hasNextFile/nextFile/hasPrevFile/prevFile` → `HasFile(DirNext/DirPrev)` / `StepFile(DirNext/DirPrev)`
- [x] replace `selectedFile` → `SelectedFile`, `selectByPath` → `SelectByPath`, `toggleFilter` → `ToggleFilter`, `refreshFilter` → `RefreshFilter`, `toggleReviewed` → `ToggleReviewed`, `reviewedCount` → `ReviewedCount`
- [x] update any direct field-access assertions to use exported methods where possible (`.allFiles` → `.TotalFiles()`, `.filter` → `.FilterActive()`, etc.) — remaining field-access stays OK because tests live in `package sidepane`
- [x] add new test `TestFileTreeRebuild` covering: reviewed map preservation (keys kept for files still present), reviewed map pruning (stale keys dropped for removed files), cursor reset to first file entry, fileStatuses refresh from new entries, filter state preservation
- [x] add new test `TestFileTreeRebuildThenSelectByPath` covering the contract: after `Rebuild(entries)` + `SelectByPath(existingFile)`, cursor lands on that file; after `Rebuild(entries)` + `SelectByPath(deletedFile)`, cursor stays on first file (SelectByPath returns false)
- [x] add test `TestFileTreeMove_Exhaustive` iterating all `MotionValues`, ensuring `Move` doesn't panic with any enum value (both with and without a count argument)
- [x] add test `TestFileTreeMove_VariadicCount` covering: `Move(MotionPageDown)` with no count argument (fallback behavior), `Move(MotionPageDown, 5)`, `Move(MotionDown, 99)` (count ignored for non-page motions)
- [x] add test `TestNewFileTree_Nil` covering `NewFileTree(nil)` returns a valid empty `*FileTree` (needed for NewModel nil-safe initialization path)
- [x] adapt render tests — replace direct style field access with `FileTreeRender{Resolver: ..., Renderer: ...}`
- [x] for render tests, use `style.NewResolver(...)` / `style.NewRenderer(...)` from `app/ui/style/` — tests importing style is fine (coupling is test→style, not sidepane→style; sidepane's production code uses only `style.StyleKey`/`ColorKey` type names)

### Task 4: Implement TOC type and all methods

Port `app/ui/mdtoc.go` into the new package. Tests in Task 5 (same DP #9 deviation rationale as Task 2/3).

**Files:**
- Create: `app/ui/sidepane/toc.go`

- [x] create `app/ui/sidepane/toc.go`
- [x] define `TOC` struct (fields: entries, cursor, offset, activeSection) and unexported `tocEntry` struct
- [x] implement `ParseTOC(lines []diff.DiffLine, filename string) *TOC` — port logic from old `parseTOC`, return `nil` when no headers found, include synthetic filename prepend entry
- [x] **CRITICAL: `ParseTOC` must initialize `activeSection: -1`** in the returned `*TOC` literal — this is the sentinel value that `UpdateActiveSection` and `SyncCursorToActiveSection` depend on. Zero-value `activeSection=0` would misbehave (pointing at the synthetic filename entry instead of "no active section"). See `mdtoc.go:100` for the original initialization.
- [x] add godoc on `TOC.activeSection` field documenting the `-1` sentinel meaning "no active section"
- [x] implement `fencePrefix` as unexported package function (pure string helper — not a method because neither `TOC` nor `string` is the natural owner, and it's called from `ParseTOC` during construction before any `*TOC` exists)
- [x] implement `NumEntries() int`
- [x] implement `CurrentLineIdx() (int, bool)` — returns lineIdx of entry at cursor, ok=false when empty or cursor out of range
- [x] implement `Move(m Motion, count ...int)` — exhaustive switch, covers all motions (MotionPageUp/Down loop internally since TOC allows full page, reusing the same pattern as FileTree). Variadic contract documented in godoc.
- [x] implement `EnsureVisible(height int)` via shared `ensureVisible` helper
- [x] implement `UpdateActiveSection(diffCursor int)` (port from old — sets `activeSection` back to `-1` when no entry matches, preserving the sentinel contract)
- [x] implement `SyncCursorToActiveSection()` — NEW method; sets `cursor = activeSection` when `activeSection >= 0` (no-op when `activeSection == -1`, replaces the guarded block in diffnav.go:580)
- [x] implement `Render(r TOCRender) string` — port old `render`, replace `focusedPane pane` with `r.Focused bool`, use `r.Resolver`
- [x] implement `truncateTitle` as unexported method on `*TOC`

### Task 5: Port TOC tests

**Files:**
- Create: `app/ui/sidepane/toc_test.go`

- [x] copy `app/ui/mdtoc_test.go` to `app/ui/sidepane/toc_test.go`, change package header to `package sidepane` (**NOT** `package sidepane_test`) — tests need access to private fields like `entries`, `cursor`, `activeSection` for setup and assertions
- [x] rename `parseTOC` → `ParseTOC` in tests
- [x] rename `moveUp/moveDown` → `Move(MotionUp)` / `Move(MotionDown)`
- [x] rename `updateActiveSection` → `UpdateActiveSection`, `ensureVisible` → `EnsureVisible`, `render` → `Render`
- [x] add `TestTOCMove_Exhaustive` iterating all `MotionValues`, asserting no panic with or without count argument
- [x] add `TestTOCCurrentLineIdx` covering: valid cursor returns (idx, true), empty entries returns (0, false), cursor past end returns (0, false), nil TOC... wait — methods on nil receiver are not valid, so callers must nil-check first; confirm this by removing any test that calls methods on a nil *TOC
- [x] add `TestTOCSyncCursorToActiveSection` covering: activeSection=-1 is no-op (cursor unchanged), activeSection >= 0 moves cursor to match
- [x] add `TestParseTOC_NoHeaders` covering: returns nil (not a typed-nil wrapped in a valid *TOC), so main.go factory guard works
- [x] add `TestParseTOC_InitialActiveSection` covering: newly returned *TOC has `activeSection == -1`
- [x] adapt `render` test calls to use `TOCRender{Resolver: ..., Focused: true/false}` instead of positional args
- [x] verify `go test ./app/ui/sidepane/... -race` green and coverage is comparable to the pre-extraction tests

### Task 6: M1 MILESTONE — Sidepane package green

This is the milestone gate for M1. No new code, just verification and commit.

- [x] run `go test ./app/ui/sidepane/... -race` — must be green
- [x] run `go vet ./app/ui/sidepane/...` — must be clean
- [x] run `golangci-lint run app/ui/sidepane/...` — must be clean
- [x] verify test coverage is ≥ 80% for sidepane: `go test ./app/ui/sidepane/... -cover`
- [x] acknowledge that full project build is still broken — expected
- [x] commit M1: `git add app/ui/sidepane/ && git commit -m "feat(sidepane): add filetree and mdtoc sub-package"` (message expands to cover Motion/Direction enums + new Rebuild API + consumer-side interfaces)

### Task 7: Add ui consumer-side interfaces, factory fields, and change Model field types

Add the exported `FileTreeComponent` and `TOCComponent` interfaces to `ui/model.go`, add the `NewFileTree` and `ParseTOC` factory function fields to `ModelConfig`, change `Model.tree` and `Model.mdTOC` field types, store factories on Model, and initialize `m.tree` with an empty tree for nil-safety.

**Files:**
- Modify: `app/ui/model.go`

- [x] add `FileTreeComponent` exported interface definition with all 15 methods; each method gets a godoc comment starting with the method name (CLAUDE.md `rules/comments.md` requirement for exported types)
- [x] add `TOCComponent` exported interface definition with all 7 methods; same godoc requirement
- [x] add `import "github.com/umputun/revdiff/app/ui/sidepane"` to model.go — **data-object import only** (for `sidepane.Motion`, `sidepane.Direction`, `sidepane.FileTreeRender`, `sidepane.TOCRender` type names in interface signatures). Model code MUST NOT call `sidepane.NewFileTree(...)` or `sidepane.ParseTOC(...)` — all construction goes through injected factories.
- [x] change `Model.tree fileTree` → `Model.tree FileTreeComponent`
- [x] change `Model.mdTOC *mdTOC` → `Model.mdTOC TOCComponent`
- [x] add `newFileTree func(entries []diff.FileEntry) FileTreeComponent` field to `Model` struct
- [x] add `parseTOC func(lines []diff.DiffLine, filename string) TOCComponent` field to `Model` struct
- [x] add `NewFileTree func(entries []diff.FileEntry) FileTreeComponent` field to `ModelConfig` with godoc noting it's required and typically wraps `sidepane.NewFileTree`
- [x] add `ParseTOC func(lines []diff.DiffLine, filename string) TOCComponent` field to `ModelConfig` with godoc noting it's required, must collapse typed-nil to interface-nil for empty TOCs
- [x] update `NewModel` to validate both factories are non-nil (return `errors.New("ui.NewModel: cfg.NewFileTree is required")` / `cfg.ParseTOC is required`) matching existing validation pattern for other required deps
- [x] copy factories from `cfg.NewFileTree` / `cfg.ParseTOC` into the `Model{newFileTree: ..., parseTOC: ...}` struct literal in `NewModel`
- [x] **CRITICAL nil-safety**: in `NewModel`, initialize `m.tree` to an empty tree via `cfg.NewFileTree(nil)` — the interface field would otherwise be a nil interface, and `loaders.go:90 loadSelectedIfChanged` calls `m.tree.EnsureVisible(...)` before any `handleFilesLoaded`. Current code is zero-value-safe because `fileTree` is a value type; switching to interface breaks that safety unless we pre-initialize. Add godoc on the field noting this invariant: "tree is never nil after NewModel; starts empty, gets Rebuilt on filesLoadedMsg".
- [x] leave `m.mdTOC` as nil-interface by default (nil-check guards already exist at every call site — `if m.mdTOC == nil` in diffnav.go, handlers.go, view.go, loaders.go)
- [x] NOTE: many call sites will be broken at this point — expected, they get fixed in Tasks 8–12

### Task 8: Migrate loaders.go call sites

**Files:**
- Modify: `app/ui/loaders.go`

- [x] replace `m.tree = newFileTreeFromEntries(entries)` with `m.tree.Rebuild(entries)` — no nil-check needed because Task 7 initializes `m.tree` to an empty tree in `NewModel`, so `m.tree` is never nil. Rebuild handles both empty→populated and populated→populated transitions.
- [x] remove the `oldReviewed := m.tree.reviewed` / `m.tree.restoreReviewed(oldReviewed)` lines — `Rebuild` handles it
- [x] replace `m.tree.selectByPath(m.currFile)` → `m.tree.SelectByPath(m.currFile)`
- [x] replace `len(m.tree.allFiles) == 1` → `m.tree.TotalFiles() == 1`
- [x] replace `m.tree.fileStatuses[msg.file]` → `m.tree.FileStatus(msg.file)`
- [x] replace `m.tree.selectedFile()` → `m.tree.SelectedFile()` (two sites)
- [x] replace `m.tree.ensureVisible(m.treePageSize())` → `m.tree.EnsureVisible(m.treePageSize())`
- [x] replace `parseTOC(msg.lines, msg.file)` call with injected factory: `m.mdTOC = m.parseTOC(msg.lines, msg.file)` — the factory closure in main.go handles the typed-nil collapse, so Model just assigns directly
- [x] confirm loaders.go does NOT import `app/ui/sidepane` — Model stays decoupled (only `ui/model.go` imports sidepane, and only for type names in interface signatures)
- [x] verify loaders.go compiles when viewed in isolation (other ui files may still error — expected)

### Task 9: Migrate view.go call sites

**Files:**
- Modify: `app/ui/view.go`

- [x] replace `m.mdTOC.render(m.treeWidth, ph, m.focus, m.resolver)` with `m.mdTOC.Render(sidepane.TOCRender{Width: m.treeWidth, Height: ph, Focused: m.focus == paneTree, Resolver: m.resolver})`
- [x] replace `m.tree.render(m.treeWidth, ph, annotated, m.resolver, m.renderer)` with `m.tree.Render(sidepane.FileTreeRender{Width: m.treeWidth, Height: ph, Annotated: annotated, Resolver: m.resolver, Renderer: m.renderer})`
- [x] replace `m.tree.reviewedCount()` → `m.tree.ReviewedCount()` (two sites)
- [x] replace `len(m.tree.allFiles)` → `m.tree.TotalFiles()`
- [x] replace `m.tree.filter` → `m.tree.FilterActive()`

### Task 10: Migrate diffnav.go call sites

Largest migration file — 24 call sites. Use the mechanical table from Technical Details.

**Files:**
- Modify: `app/ui/diffnav.go`

- [ ] migrate handleHunkNav cross-file path (lines 385-398): `hasNextFile`/`nextFile`/`hasPrevFile`/`prevFile` → `HasFile`/`StepFile` with `Direction`
- [ ] migrate handleTreeNav (lines 471-497): 8 tree nav cases → `Move(Motion, count)`, `ensureVisible` → `EnsureVisible`
- [ ] migrate handleTOCNav (lines 501-538): mdTOC nav methods → `Move(Motion, count)`, delete the `for range` page loops and replace with direct `Move(MotionPageDown/Up, count)` calls
- [ ] migrate TOC MoveToFirst/Last: `m.mdTOC.cursor = 0` / `= max(0, len(...)-1)` → `m.mdTOC.Move(MotionFirst/Last, 0)`
- [ ] migrate `m.mdTOC.ensureVisible(...)` → `m.mdTOC.EnsureVisible(...)`
- [ ] migrate `jumpTOCEntry` (line 569-576) — `m.mdTOC.cursor + delta` clamp → `m.mdTOC.Move(MotionUp/Down, 0)` based on delta sign
- [ ] migrate `syncTOCCursorToActive` (lines 579-583) → delegate to `m.mdTOC.SyncCursorToActiveSection()`
- [ ] migrate `syncDiffToTOCCursor` (lines 586-594) — replace `m.mdTOC.cursor >= len(m.mdTOC.entries)` and `m.mdTOC.entries[m.mdTOC.cursor].lineIdx` with `idx, ok := m.mdTOC.CurrentLineIdx(); if !ok { return }; m.diffCursor = idx`
- [ ] migrate `syncTOCActiveSection` (line 598-600) → `m.mdTOC.UpdateActiveSection(m.diffCursor)`
- [ ] verify diffnav.go compiles

### Task 11: Migrate handlers.go, annotate.go, annotlist.go, model.go call sites

**Files:**
- Modify: `app/ui/handlers.go`
- Modify: `app/ui/annotate.go`
- Modify: `app/ui/annotlist.go`
- Modify: `app/ui/model.go`

- [ ] handlers.go: replace the mdTOC access block (lines 283-288) — use `CurrentLineIdx()` with the ok guard, then `UpdateActiveSection`
- [ ] handlers.go: `m.tree.toggleFilter(annotated)` → `m.tree.ToggleFilter(annotated)`, `m.tree.ensureVisible(...)` → `m.tree.EnsureVisible(...)`
- [ ] handlers.go: `m.tree.selectedFile()` → `m.tree.SelectedFile()` (two sites)
- [ ] handlers.go: `m.tree.toggleReviewed(file)` → `m.tree.ToggleReviewed(file)`
- [ ] handlers.go: `m.tree.nextFile()` / `m.tree.prevFile()` → `m.tree.StepFile(sidepane.DirNext/DirPrev)`
- [ ] annotate.go: `m.tree.refreshFilter(...)` → `m.tree.RefreshFilter(...)` (four sites)
- [ ] annotate.go: `m.tree.selectedFile()` → `m.tree.SelectedFile()` (two sites)
- [ ] annotlist.go: `m.tree.selectByPath(a.File)` → `m.tree.SelectByPath(a.File)`
- [ ] model.go handleResize: `m.tree.ensureVisible(m.treePageSize())` → `m.tree.EnsureVisible(m.treePageSize())`
- [ ] verify all modified files compile individually

### Task 12: Wire sidepane factories in main.go

Update `main.go` to construct the `NewFileTree` and `ParseTOC` factory closures and inject them into `ModelConfig`. This is where `app/main` learns about the `sidepane` package — Model and other ui files remain untouched.

**Files:**
- Modify: `app/main.go`

- [ ] add `import "github.com/umputun/revdiff/app/ui/sidepane"` to main.go
- [ ] in the `ModelConfig{...}` literal that main.go currently builds, add `NewFileTree: func(entries []diff.FileEntry) ui.FileTreeComponent { return sidepane.NewFileTree(entries) }`
- [ ] add `ParseTOC: func(lines []diff.DiffLine, filename string) ui.TOCComponent { toc := sidepane.ParseTOC(lines, filename); if toc == nil { return nil }; return toc }` — the explicit nil guard is mandatory, not cosmetic
- [ ] verify main.go compiles and can construct a Model

### Task 13: Delete old filetree.go, mdtoc.go, and their tests

**Files:**
- Delete: `app/ui/filetree.go`
- Delete: `app/ui/filetree_test.go`
- Delete: `app/ui/mdtoc.go`
- Delete: `app/ui/mdtoc_test.go`

- [ ] `git rm app/ui/filetree.go app/ui/filetree_test.go app/ui/mdtoc.go app/ui/mdtoc_test.go`
- [ ] confirm no ui file still references `fileTree` (unexported type), `newFileTree`, `newFileTreeFromEntries`, `mdTOC` (type), `parseTOC` (function), `ensureVisibleInList` — grep for each, should find zero hits
- [ ] `go build ./...` — should compile now; if it doesn't, fix remaining errors

### Task 14: Fix ui package tests

Tests in ui that touched `fileTree` or `mdTOC` directly need mechanical updates. Test helpers that construct Models also need to inject sidepane factories.

**Files:**
- Modify: `app/ui/model_test.go`
- Modify: `app/ui/view_test.go`
- Modify: `app/ui/diffnav_test.go`
- Modify: `app/ui/loaders_test.go`
- Modify: `app/ui/annotate_test.go`
- Modify: `app/ui/handlers_test.go`

- [ ] pre-task scoping: run `grep -rn 'ui.ModelConfig{' app/ui/*_test.go app/main/*_test.go 2>/dev/null | wc -l` to count how many `ui.ModelConfig{...}` literals need updating. Document the count in this checkbox before proceeding so the executor knows scope. If count > 20, consider adding a helper constructor `testModelConfig(t *testing.T, overrides ...func(*ui.ModelConfig)) ui.ModelConfig` that sets defaults including the factories, and have tests override only what they need.
- [ ] add a shared test helper `testSidepaneFactories()` (or include inside existing `testNewModel`-style helper if one exists) that returns valid `NewFileTree func(...)` and `ParseTOC func(...)` closures wrapping `sidepane.NewFileTree` and `sidepane.ParseTOC`. Test files import `app/ui/sidepane` directly — tests are not Model, so the import is fine.
- [ ] update every `ui.ModelConfig{...}` literal in test files to include the new `NewFileTree` and `ParseTOC` fields via the helper (or directly via inline closures for one-off tests)
- [ ] grep each test file for `fileTree{`, `.tree.`, `.mdTOC.`, `.tree =`, `.mdTOC =` — fix each hit by calling the new API
- [ ] replace any test setup that constructs a `fileTree` directly with `sidepane.NewFileTree(...)`
- [ ] replace any test that assigned a `*mdTOC` to `m.mdTOC` with `sidepane.ParseTOC(...)` + explicit nil guard in the test helper closure (matches main.go production pattern)
- [ ] update assertions reading `.cursor`, `.offset`, `.reviewed`, `.filter`, `.entries`, `.allFiles`, `.fileStatuses` — first try the new getter (`.ReviewedCount()`, `.TotalFiles()`, `.FilterActive()`, `.SelectedFile()`, etc.). If the test needs to inspect internal state not exposed via getters, either add a test-only getter to sidepane OR move that specific assertion into `sidepane/filetree_test.go` (where private field access is free)
- [ ] grep for `m.mdTOC = ` assignments in tests (beyond the construction replacement) — verify none remain that would bypass the typed-nil collapse pattern
- [ ] run `go test ./app/ui/... -race` iteratively, fixing errors file-by-file

### Task 15: M2 MILESTONE — full test suite green

- [ ] run `go test ./... -race` — must be green end-to-end
- [ ] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — must be clean
- [ ] run `go test ./app/ui/... -cover` and verify coverage has not regressed meaningfully compared to pre-extraction (target: within 2 percentage points of pre-extraction baseline)
- [ ] build the binary: `make build`
- [ ] manual smoke test 1: launch `.bin/revdiff` in a git repo with multiple files — verify file tree navigates with j/k, page up/down, home/end, n/p, cross-file hunk nav, filter toggle, reviewed toggle
- [ ] manual smoke test 2: launch `.bin/revdiff --only README.md` in a repo with README.md — verify markdown TOC appears in side pane, navigates with j/k/n/p/Enter, active section tracks diff cursor
- [ ] manual smoke test 3: launch with `--no-colors` — verify tree and TOC render without styling issues
- [ ] manual smoke test 4: launch with `--all-files` — verify untracked toggle works, tree rebuild preserves reviewed marks
- [ ] commit M2: `git add -A && git commit -m "refactor(ui): migrate to sidepane sub-package"` (full call-site migration + main.go wiring + old files deleted)

### Task 16: Update app/ui/doc.go and CLAUDE.md

**Files:**
- Modify: `app/ui/doc.go`
- Modify: `CLAUDE.md`

- [ ] first read `app/ui/doc.go` end-to-end to confirm the style sub-package paragraph exists and understand its structure — the sidepane paragraph must match its form
- [ ] remove `filetree.go` and `mdtoc.go` bullet points from `app/ui/doc.go` file map
- [ ] add a paragraph in `app/ui/doc.go` describing the `sidepane` sub-package, mirroring the existing `style` sub-package paragraph's structure, and noting that sidepane components are injected via `ModelConfig.NewFileTree` / `ModelConfig.ParseTOC` factory closures (wired in `app/main.go`)
- [ ] in CLAUDE.md Project Structure section, remove individual `filetree.go` / `mdtoc.go` descriptions, add new `app/ui/sidepane/` description listing FileTree and TOC types and noting that concrete construction lives in main.go via factory closures
- [ ] in CLAUDE.md Key Interfaces section, add mention of exported `ui.FileTreeComponent` / `ui.TOCComponent` interfaces and the factory-injection pattern
- [ ] in CLAUDE.md "Gotchas" section, add an entry about the typed-nil trap: "`ParseTOC` factory closure in main.go MUST guard `if toc == nil { return nil }` to collapse typed-nil `*sidepane.TOC` into a truly nil interface"

### Task 17: Verify acceptance criteria

- [ ] grep for orphan references using word-boundary matching: `grep -rwE 'fileTree|newFileTree|newFileTreeFromEntries|mdTOC|parseTOC|ensureVisibleInList' app/ui/*.go` — should find zero hits in the `ui` package (excluding `app/ui/sidepane/`). Word-boundary `-w` avoids false negatives from substrings like `fileTreeSomething`.
- [ ] grep `app/ui/` for `"github.com/umputun/revdiff/app/ui/sidepane"` in production files (exclude `*_test.go`): `grep -l '"github.com/umputun/revdiff/app/ui/sidepane"' app/ui/*.go | grep -v _test.go` — should return only `app/ui/model.go` (the single data-object-import site). Model handlers (loaders, view, diffnav, handlers, annotate, annotlist) must not appear.
- [ ] run `go test ./... -race` once more
- [ ] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [ ] verify `app/ui/sidepane` test coverage is ≥ 80%
- [ ] verify no `var _ FileTreeComponent = ...` or `var _ TOCComponent = ...` assertions exist (they were intentionally omitted per Design Philosophy decision)
- [ ] verify all Motion/Direction enum values are used in at least one call site or test (no dead enum values)

### Task 18: M3 MILESTONE — move plan to completed

- [ ] `git mv docs/plans/2026-04-11-sidepane-extraction.md docs/plans/completed/`
- [ ] commit M3: `git add -A && git commit -m "docs: complete sidepane extraction plan"` (docs updates + plan moved)

## Post-Completion

*Items requiring manual intervention or external verification — no checkboxes.*

**Manual smoke tests** (part of M2 gate, documented here for reference):
- Multi-file git repo: file tree shows directories + files, cursor navigation works, filter toggle (`f`), reviewed toggle (Space), cross-file hunk nav (`[` / `]`)
- Single-file markdown with full context (`--only README.md` on a README-only diff or on clean checkout): TOC pane appears, headers listed with indentation by level, Enter jumps to header, `n`/`p` cycles headers, active section highlights
- `--no-colors` mode: tree and TOC render without ANSI sequences
- `--all-files` + untracked toggle (`u`): tree rebuild preserves reviewed marks

**Follow-up candidates** (explicitly out of scope for this plan, same list as style plan's tail):
- Extract `app/ui/worddiff/` sub-package (pure LCS algorithm — no interface needed, simpler)
- Evaluate whether `configpatch.go` should move into its own package
- Evaluate whether overlays (`annotlist.go`, `themeselect.go`) should become sub-packages with result-type event patterns
