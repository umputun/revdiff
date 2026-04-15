# Structural Refactor Plan: Startup Wiring Cleanup, Theme Boundary Cleanup, and UI State Consolidation

## Overview

Refactor the project structure in three targeted areas that currently concentrate too much coordination logic:

1. **`app/main.go` is too large and mixes unrelated startup concerns** — CLI/config parsing, stdin handling, VCS renderer setup, theme command handling, theme application, Bubble Tea wiring, and history save logic all live in one 783-line file.
2. **`app/ui/` still has one large coordination hotspot** — `Model` is the right Bubble Tea boundary, but it currently carries ~70 field lines, ~20 boolean flags, 179 methods, and several implicit state clusters that are not modeled explicitly.
3. **Theme discovery/persistence leaks into `app/ui/`** — `app/ui/themeselect.go` and `app/ui/configpatch.go` make the UI package responsible for theme file loading and INI patching, which are wiring/persistence concerns and belong outside the presentation layer.

This plan is an **incremental structural refactor**, not an architecture rewrite. The single Bubble Tea `Model` stays. Existing `app/ui/style`, `app/ui/overlay`, `app/ui/sidepane`, and `app/ui/worddiff` packages stay. The goal is to reduce complexity concentration and tighten package boundaries **without changing behavior**.

## Goals

- Keep `ui.Model` as the single Bubble Tea model
- Split `package main` by concern while keeping the composition root in `app/`
- Remove theme persistence/discovery from `app/ui`
- Replace loose `Model` field sprawl with explicit grouped sub-state structs
- Replace the current diff-related parallel arrays with one explicit loaded-file state object
- Preserve all current features and keyboard behavior
- Keep all changes test-covered and converged at milestone boundaries

## Non-Goals

- No repo-level move to `cmd/` / `internal/`
- No rewrite into multiple Bubble Tea models
- No behavioral redesign of overlays, search, annotation flow, or navigation
- No new external dependencies
- No package split of `app/diff` in this plan

## Context (from discovery)

### Current structure hotspots

- `app/main.go` — **783 LOC**
- `app/ui/model.go` — **756 LOC**
- `app/ui/diffview.go` — **632 LOC**
- `app/ui/diffnav.go` — **624 LOC**
- `app/ui/` non-test code total — **~4500 LOC**
- `Model` methods across `app/ui/*.go` — **179**
- `Model` field lines — **~70**, including **~20 bool fields**

### Current layering issue around themes

`app/ui/` currently owns theme behaviors that are outside presentation:

- `app/ui/themeselect.go`
  - builds theme entries from disk via `app/theme`
  - tracks preview session state
  - applies preview/confirm/cancel logic
- `app/ui/configpatch.go`
  - patches `~/.config/revdiff/config`
  - depends on `app/fsutil`

This creates an unnecessary dependency edge:

```text
ui -> theme
ui -> fsutil
```

The UI should instead consume a narrow theme provider interface and stay focused on presentation/runtime state.

### Current `Model` state smell

The current `Model` mixes these concerns at top level:

- dependencies and immutable config
- viewport/layout state
- current file rendering state
- navigation state
- annotation input state
- search input state
- theme selector preview state
- quit/discard mode state
- pending cross-file jump state

The state is not wrong, but it is **too flat**. It is harder than necessary to reason about valid mode combinations and invariants.

## Development Approach

- **testing approach**: Regular — implementation first, then tests for each unit
- **Complete each milestone fully before moving to the next**
- Small focused changes within each task
- **CRITICAL: no behavioral changes are intended** — this is structure, state-shape, and dependency-boundary work
- **CRITICAL: update this plan file when scope changes during implementation**
- **Single-level task numbering** only

## Conventions Adopted from Prior Refactor Plans

This plan should follow the same conventions established by the completed `style`, `sidepane`, `worddiff`, `overlay`, `ui-package-split`, and `code-smells-cleanup` plans.

1. **Be explicit about mechanical split vs. actual redesign.** Milestone 1 is primarily a decomposition of the existing `app/main.go`, not a redesign of startup behavior.
2. **Put domain logic in the true owning package.** Theme catalog/repository behavior belongs in `app/theme`; `package main` remains wiring/composition only.
3. **Use consumer-side interfaces.** Any new UI-facing dependency is defined in `app/ui/model.go`, with concrete implementations living in the provider package.
4. **Keep DI strict.** Required runtime dependencies should be wired explicitly through `ModelConfig`; production code should not fabricate silent fallbacks.
5. **Avoid speculative abstractions.** New types/interfaces/helpers must correspond to real ownership boundaries or real call sites, not just aesthetic cleanup.
6. **Document why, not just what.** When a new boundary or type is introduced, the plan should state why that shape is preferred over the obvious alternatives.
7. **Keep state refactoring intentional.** The broad `ui.Model` state consolidation is in scope here, but it should still result in cohesive state clusters, not a grab-bag of arbitrary wrappers.

## Testing Policy

Unlike the extraction plans, this refactor can and should stay green at milestone boundaries.
Intermediate tasks inside a milestone may temporarily break build/tests while state and call sites are being migrated, but each milestone must converge cleanly.

| Milestone | Tasks | Scope | Green gate |
|---|---|---|---|
| **M1: Split `package main`** | 1–2 | file split only; no package boundary changes | `go test ./...` green + `make lint` green |
| **M2: Theme boundary cleanup** | 3–5 | `ui` stops owning theme discovery/persistence | `go test ./...` green + `make lint` green + theme selector manual smoke test |
| **M3: `loadedFileState`** | 6–8 | loaded-file parallel arrays → single struct | `go test ./...` green + `make lint` green |
| **M4: Remaining state grouping** | 9–12 | config/layout/mode/search/annotation sub-structs | `go test ./...` green + `make lint` green + manual smoke test for search/annotate/navigation |
| **M5: Docs/cleanup** | 13–14 | docs updated, plan moved to completed | same as M4 |

**Commit structure**: one commit per milestone.

## Solution Overview

## Part A — Split `package main` by concern

Keep `package main`, but split the current `app/main.go` into multiple files with stable ownership boundaries.

### Target file layout

```text
app/
├── main.go              — main(), early-exit command flow, run() call
├── config.go            — options struct, parseArgs, dumpConfig, config-path helpers
├── stdin.go             — stdin validation, /dev/tty reopen, stdin renderer prep
├── renderer_setup.go    — DetectVCS wiring, makeGitRenderer/makeHgRenderer/makeNoVCSRenderer
├── themes.go            — theme command handling, theme apply helpers, thin adapter/wiring for ui
└── history_save.go      — histReq and saveHistory
```

### Rules

- `main.go` becomes a thin flow coordinator
- `config.go` owns all config/flag parsing helpers
- `themes.go` owns startup-level theme command flow and thin theme-related wiring used by `package main`
- `history_save.go` owns history-save policy instead of leaving it buried in startup wiring

**Clarification:** these files are intended to be **primarily a decomposition of the current `app/main.go` monolith**. For Milestone 1, this should be as close to move-only as practical: existing helpers are regrouped by concern, not redesigned. The only likely non-trivial addition is in `themes.go`, where later milestones may need a thin adapter for the UI-facing theme catalog/persistence boundary. Even there, the goal is still to keep `package main` as wiring/composition code, not to grow new domain logic.

This keeps the composition root in one package without forcing a bigger package-level redesign.

---

## Part B — Move theme persistence/discovery out of `app/ui`

### Problem

`ui` currently knows:
- where themes live on disk
- how to list/load them
- how to patch the config file to persist selection

That is wiring and persistence logic.

### Decision

Introduce a **consumer-side interface in `app/ui/model.go`** for theme data and persistence, but keep the actual theme-domain implementation in **`app/theme`**, not in `package main`. `package main` should only wire thin adapters where needed. This follows the same provider/consumer split used in the completed extraction plans: UI consumes an interface, the domain package owns the real implementation, and `main` only composes them.

### Proposed UI-side contract

```go
type ThemeCatalog interface {
    Entries() ([]ThemeEntry, error)
    Resolve(name string) (ThemeSpec, bool)
    Persist(name string) error
}

type ThemeEntry struct {
    Name        string
    Local       bool
    AccentColor string
}

type ThemeSpec struct {
    Colors      style.Colors
    ChromaStyle string
}
```

Notes:
- `ThemeEntry` is minimal list-view data
- `ThemeSpec` is already in UI/runtime terms; `ui` should not import `app/theme`
- `Persist(name)` is the only persistence operation UI needs
- `Resolve(name)` gives UI what it needs to preview/apply a theme without knowing how that theme was loaded
- `Persist` is bundled into `ThemeCatalog` deliberately: from UI's perspective, theme confirm is one interaction ("save this choice"), and splitting catalog vs persistence into two injected interfaces adds dependency count for no consumer-side benefit. The *concrete implementation* in `package main` composes `app/theme` catalog with config-patching persistence — that's where the separation lives, not in the UI-facing contract

### Ownership after refactor

**`app/ui` owns:**
- opening the theme selector overlay
- holding preview-session state
- applying `style.Resolver`/`style.Renderer`/`style.SGR` replacements to the live model
- restoring previous style state on cancel

**`app/theme` owns:**
- theme list discovery
- loading themes from disk/gallery
- translating stored themes into runtime theme data suitable for UI preview/apply

**`package main` owns:**
- wiring the concrete theme services into `ui.ModelConfig`
- theme-related CLI command flow (`--list-themes`, `--init-themes`, etc.)
- config patching/persistence adapter, unless a dedicated config package is introduced later

This means the "theme switcher" is split by concern:
- the **selector UI** stays in `app/ui` / `app/ui/overlay`
- the **theme catalog/repository** belongs in `app/theme`
- the **wiring** stays in `package main`
- the **selected-theme persistence** stays in startup/config territory unless later extracted to a config package

### Files affected

- Modify: `app/ui/model.go`
- Modify: `app/ui/themeselect.go`
- Delete: `app/ui/configpatch.go`
- Delete: `app/ui/configpatch_test.go`
- Modify: `app/main.go` / split-out `app/themes.go`
- Add tests in `app/` for theme catalog/persist helpers
- Update UI tests to use injected theme catalog mocks/fakes

### Expected dependency cleanup

After this refactor, `app/ui` should no longer import:
- `app/theme`
- `app/fsutil`

That is the main boundary win.

---

## Part C — Consolidate `ui.Model` state into explicit sub-structs

### Decision

Keep one `Model`, but group its mutable state into named sub-structs. This preserves Bubble Tea ergonomics while making state ownership explicit.

### Target state layout

```go
type Model struct {
    // injected deps
    resolver     styleResolver
    renderer     styleRenderer
    sgr          sgrProcessor
    differ       wordDiffer
    overlay      overlayManager
    diffRenderer Renderer
    highlighter  SyntaxHighlighter
    blamer       Blamer
    keymap       *keymap.Keymap
    tree         FileTreeComponent
    parseTOC     func([]diff.DiffLine, string) TOCComponent
    store        *annotation.Store
    themes       ThemeCatalog

    // immutable/session config
    cfg modelConfigState

    // grouped mutable runtime state
    layout layoutState
    nav    navigationState
    file   loadedFileState
    modes  modeState
    annot  annotationState
    search searchState

    // thin top-level state (not worth dedicated structs)
    ready              bool
    discarded          bool
    inConfirmDiscard   bool
    themeActiveName    string
    themePreview       *themePreviewSession
    pendingAnnotJump   *annotation.Annotation
}
```

### Proposed sub-state structs

#### `modelConfigState`
Holds immutable or near-immutable startup config that is currently spread across top-level fields:
- `ref`, `staged`, `only`, `workDir`
- `noColors`, `noStatusBar`, `noConfirmDiscard`
- `crossFileHunks`, `treeWidthRatio`
- possibly `tabSpaces`

#### `layoutState`
Pure layout/viewport concerns:
- `viewport`
- `focus`
- `treeHidden`
- `width`, `height`, `treeWidth`
- `scrollX`

#### `navigationState`
Cursor and navigation-adjacent state:
- `diffCursor`
- `pendingHunkJump`

#### `loadedFileState`
Replaces the current flat diff-related fields and removes the parallel-array smell:
- `name`
- `lines []diff.DiffLine`
- `highlighted []string`
- `intraRanges [][]worddiff.Range`
- `adds`, `removes`
- `blameData map[int]diff.BlameLine`
- `blameAuthorLen int`
- `lineNumWidth int`
- `singleColLineNum bool`
- `loadSeq uint64`
- `mdTOC TOCComponent`
- `singleFile bool`

This state object becomes the single source of truth for the currently loaded file.

#### `modeState`
View-mode toggles only — cohesive set of user-togglable display modes:
- `wrap`
- `collapsed`
- `lineNumbers`
- `wordDiff`
- `showBlame`
- `showUntracked`

**Not included** (kept at `Model` top level):
- `ready` — singleton lifecycle flag, set once after first `WindowSizeMsg`
- `discarded`, `inConfirmDiscard` — quit-flow flags, not view modes

#### `annotationState`
Annotation input lifecycle:
- `annotating`
- `fileAnnotating`
- `cursorOnAnnotation`
- `input textinput.Model`

#### `searchState`
Search lifecycle:
- `active`
- `term`
- `matches`
- `cursor`
- `input textinput.Model`
- `matchSet`

**Kept at `Model` top level** (too thin for dedicated structs):
- `themeActiveName string` — current theme name
- `themePreview *themePreviewSession` — preview session state (2 fields not worth a struct)
- `pendingAnnotJump` — single cross-file deferred jump field

### Important constraint

This is **state grouping**, not a move toward mini-models. Methods remain on `Model`. The grouped structs are there to make invariants explicit and keep field access local to concern-specific code.

---

## Part D — Introduce a dedicated current-file state object first

Before broad field grouping, introduce `loadedFileState` as the first explicit cluster. This is the lowest-risk and highest-value state cleanup because it removes several synchronization invariants that are currently implicit.

### Current implicit invariant

These values must stay aligned by index and lifecycle:
- `diffLines`
- `highlightedLines`
- `intraRanges`

### Proposed replacement

```go
type loadedFileState struct {
    name           string
    lines          []diff.DiffLine
    highlighted    []string
    intraRanges    [][]worddiff.Range
    adds           int
    removes        int
    blameData      map[int]diff.BlameLine
    blameAuthorLen int
    lineNumWidth   int
    singleColLineNum bool
    mdTOC          TOCComponent
    singleFile     bool
}
```

All render/load/search/annotation code should read through `m.file` rather than independent top-level fields.

This should be done before the broader mode/layout grouping because it shrinks the highest-risk shared state surface early.

## Design Decisions (Locked)

### D1: Keep one Bubble Tea `Model`
This refactor does not split the app into multiple Bubble Tea models.

### D2: `package main` stays the composition root
We split files, not packages, for startup/wiring logic.

### D3: Theme persistence/discovery leaves `ui`
`ui` may still own live preview/apply/cancel behavior, but it must not know about config patching or theme file loading. Theme discovery/loading should move to `app/theme`; config persistence should stay in startup/config territory unless a dedicated config package is introduced.

### D4: Consumer-side interfaces remain in `app/ui/model.go`
Any new UI dependency (`ThemeCatalog`) is defined where it is consumed.

### D5: Grouped sub-state structs are private implementation details
No new exported state types unless a test helper or config boundary genuinely needs them. Single-field clusters (`pendingAnnotJump`) stay at `Model` top level. Two-field clusters should be evaluated case-by-case — a struct is justified when the fields represent a coherent concept, not just by count alone.

### D6: Introduce `loadedFileState` before broader `Model` reshaping
This removes the most fragile implicit invariant first.

### D7: No opportunistic behavior changes
Do not combine this refactor with UX tweaks, keybinding changes, search semantics changes, or theme behavior redesign.

## Implementation Steps

### Task 1: Create the structural refactor branch and baseline checks

- [x] create a dedicated branch for the structural refactor
- [x] run `go test ./...`
- [x] run `make lint`
- [x] capture baseline file sizes for `app/main.go` and `app/ui/*.go`

### Task 2: Split `app/main.go` into startup concern files

**Files:**
- Create: `app/config.go`
- Create: `app/stdin.go`
- Create: `app/renderer_setup.go`
- Create: `app/themes.go`
- Create: `app/history_save.go`
- Modify: `app/main.go`

- [x] move `options`, `parseArgs`, `dumpConfig`, `loadConfigFile`, `resolveFlagPath`, `defaultConfigPath`, `defaultKeysPath` into `config.go`
- [x] move stdin helpers into `stdin.go`
- [x] move VCS/renderer selection helpers into `renderer_setup.go`
- [x] move history-save code into `history_save.go`
- [x] move theme command handling, color mapping helpers, and `defaultThemesDir` into `themes.go`
- [x] leave `main.go` with only top-level command flow and `run()` orchestration
- [x] verify `app/main.go` is reduced to a thin entrypoint file
- [x] split `app/main_test.go` to match source files: `config_test.go`, `stdin_test.go`, `renderer_setup_test.go`, `themes_test.go`, `history_save_test.go`
- [x] run `go test ./...`
- [x] run `make lint`

### Task 3: Introduce a UI-side `ThemeCatalog` dependency

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/themeselect.go`
- Modify: `app/ui/doc.go`
- Modify: `app/themes.go`
- Modify: `app/theme/theme.go` (or add a new file in `app/theme/`)

- [x] add `ThemeCatalog`, `ThemeEntry`, and `ThemeSpec` types to `app/ui/model.go`
- [x] add `ThemeCatalog` to `ui.ModelConfig` as a required dependency
- [x] add constructor validation for missing `ThemeCatalog`
- [x] add a concrete catalog/repository type to `app/theme` backed by the existing list/load functions
- [x] convert theme loading from `theme.Theme` to UI-facing `ThemeSpec`
- [x] keep `package main` adapter/wiring thin — compose `app/theme` catalog + selected-theme persistence
- [x] wire the catalog into `ui.NewModel(...)` in `run()`
- [x] add compile-time satisfaction checks where appropriate
- [x] add tests for the concrete `ThemeCatalog` implementation in `app/theme/`
- [x] verify the resulting ownership split is still obvious at a glance: `ui` consumes, `theme` implements, `main` wires

### Task 4: Move theme discovery out of `app/ui/themeselect.go`

**Files:**
- Modify: `app/ui/themeselect.go`
- Modify: `app/ui/themeselect_test.go`

- [x] replace `buildThemeEntries()` disk-loading logic with `m.themes.Entries()`
- [x] replace preview/confirm lookup against `theme.Theme` entries with `ThemeCatalog.Resolve(name)`
- [x] keep preview-session state in UI, but make it store `ThemeSpec` instead of `theme.Theme`
- [x] keep `applyTheme(...)` in UI, but make it accept `ThemeSpec`
- [x] update `themeselect_test.go` to use a fake/mock catalog instead of filesystem/theme package coupling
- [x] verify theme selector behavior is unchanged

### Task 5: Move theme persistence out of `app/ui`

**Files:**
- Delete: `app/ui/configpatch.go`
- Delete: `app/ui/configpatch_test.go`
- Modify: `app/ui/themeselect.go`
- Modify: `app/themes.go`
- Add: tests in `app/themes_test.go` or equivalent

- [x] remove `patchConfigTheme()` from `app/ui`
- [x] move config-patching helper out of `ui` and keep it as a thin startup/config persistence adapter
- [x] replace direct config file patching in `confirmThemeByName()` with `m.themes.Persist(name)`
- [x] add focused tests for theme persistence in `app/`
- [x] confirm `app/ui` no longer imports `app/fsutil`
- [x] confirm `app/ui` no longer imports `app/theme`
- [x] run `go test ./...`
- [x] run `make lint`

### Task 6: Introduce `loadedFileState` struct and migrate loaders

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/loaders.go`

- [ ] add private `loadedFileState` to `app/ui/model.go` with all fields listed in Part D (including `lineNumWidth`, `singleColLineNum`)
- [ ] replace old top-level fields with `file loadedFileState` in `Model`
- [ ] migrate `loaders.go` to populate `m.file` instead of top-level fields

### Task 7: Migrate rendering and navigation to `loadedFileState`

**Files:**
- Modify: `app/ui/diffview.go`
- Modify: `app/ui/diffnav.go`
- Modify: `app/ui/view.go`
- Modify: `app/ui/collapsed.go`

- [ ] migrate `diffview.go` to read `m.file.lines`, `m.file.highlighted`, `m.file.intraRanges`, etc.
- [ ] migrate `diffnav.go` to use `m.file`
- [ ] migrate `view.go` to use `m.file`
- [ ] migrate `collapsed.go` to use `m.file`

### Task 8: Migrate search, annotation, and remaining callers to `loadedFileState`

**Files:**
- Modify: `app/ui/search.go`
- Modify: `app/ui/annotate.go`
- Modify: `app/ui/handlers.go`
- Modify: remaining `app/ui/*.go` with stale references

- [ ] migrate `search.go` to use `m.file`
- [ ] migrate `annotate.go` to use `m.file`
- [ ] migrate any remaining files with direct references to old field names
- [ ] grep for old field names (`m.diffLines`, `m.highlightedLines`, `m.intraRanges`, etc.) to verify no remaining references
- [ ] run `go test ./...`
- [ ] run `make lint`

### Task 9: Group `modelConfigState` and `layoutState`

**CRITICAL:** this is an intentional redesign task, not a move-only cleanup. Each grouping must pull its weight — reject speculative wrappers. Methods stay on `Model`; do not extract mini-models.

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/view.go`, `app/ui/handlers.go`, `app/ui/diffview.go`, and other files referencing layout/config fields

- [ ] add `modelConfigState` struct and move `ref`, `staged`, `only`, `workDir`, `noColors`, `noStatusBar`, `noConfirmDiscard`, `crossFileHunks`, `treeWidthRatio`, `tabSpaces` into `m.cfg`
- [ ] add `layoutState` struct and move `viewport`, `focus`, `treeHidden`, `width`, `height`, `treeWidth`, `scrollX` into `m.layout`
- [ ] update all call sites for config and layout fields

### Task 10: Group `modeState` and `navigationState`

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/*.go` as needed

- [ ] add `modeState` struct with view-mode toggles only: `wrap`, `collapsed`, `lineNumbers`, `wordDiff`, `showBlame`, `showUntracked`
- [ ] keep `ready`, `discarded`, `inConfirmDiscard` at `Model` top level (lifecycle/quit flags, not view modes)
- [ ] add `navigationState` struct with `diffCursor`, `pendingHunkJump`
- [ ] update all call sites

### Task 11: Group `searchState` and `annotationState`

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/search.go`, `app/ui/annotate.go`, and other referencing files

- [ ] add `searchState` struct with `active`, `term`, `matches`, `cursor`, `input`, `matchSet`
- [ ] add `annotationState` struct with `annotating`, `fileAnnotating`, `cursorOnAnnotation`, `input`
- [ ] update all call sites
- [ ] remove old top-level fields
- [ ] verify `Model` field list is materially shorter and visually grouped
- [ ] run `go test ./...`
- [ ] run `make lint`

### Task 12: Tighten constructor and test helpers around grouped state

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/*_test.go`

- [ ] keep `NewModel` strict on required dependencies
- [ ] move any test-only defaulting into test helpers, not production constructor logic
- [ ] update tests to construct models through helpers that match the new `ModelConfig`
- [ ] ensure no production fallback logic was introduced just to ease tests
- [ ] run `go test ./...`
- [ ] run `make lint`

### Task 13: Documentation updates

**Files:**
- Modify: `docs/ARCHITECTURE.md`
- Modify: `app/ui/doc.go`
- Modify: `CLAUDE.md`

- [ ] update startup/composition-root description to reflect split `package main` files
- [ ] update `app/ui/doc.go` to describe `loadedFileState`, `layoutState`, `modeState`, `searchState`, `annotationState` sub-structs and their purpose
- [ ] update project structure notes in `CLAUDE.md`
- [ ] confirm architecture docs still describe actual theme ownership boundaries

### Task 14: Final verification and plan move

- [ ] run full test suite: `go test ./...`
- [ ] run `make lint`
- [ ] build the binary: `make build`
- [ ] manual smoke test:
  - [ ] default git diff startup
  - [ ] `--stdin`
  - [ ] `--only`
  - [ ] theme selector preview / confirm / cancel
  - [ ] search mode
  - [ ] annotation add/delete flow
  - [ ] hunk navigation across files
- [ ] move this plan to `docs/plans/completed/`

## Expected End State

After this plan:

- `app/main.go` is a thin entrypoint instead of a startup god file
- `package main` remains the composition root, but its concerns are split cleanly across files
- `app/ui` no longer owns theme discovery or config persistence
- `Model` remains single and idiomatic for Bubble Tea, but its state is explicitly grouped
- current-file rendering state is held in one dedicated object instead of parallel top-level arrays
- the project keeps its current architecture, but with lower structural risk and clearer boundaries

## Post-Completion Review Checklist

- Is `ui.Model` easier to scan than before?
- Can a new contributor find startup/config/theme wiring without reading a 700+ line `main.go`?
- Can theme selector behavior be understood without knowing config file patch details?
- Are current-file render invariants explicit in one place?
- Did we reduce complexity concentration without adding needless package churn?
