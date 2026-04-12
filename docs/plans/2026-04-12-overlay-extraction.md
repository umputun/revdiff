# app/ui/overlay/ Sub-Package Extraction

## Overview

Extract a new `app/ui/overlay/` sub-package from `app/ui/` that owns **all layered popup UI** — help popup, annotation list popup, theme selector popup, active-overlay coordination, and shared popup chrome/composition.

This is a **follow-up** to the completed `app/ui/sidepane/` and `app/ui/worddiff/` extractions. All three overlays currently live as methods on `Model` across `app/ui/handlers.go`, `app/ui/annotlist.go`, `app/ui/themeselect.go`, and `app/ui/view.go`, with state fields scattered across the `Model` struct.

**Problem it solves:**

- Popup UI is split across four files with no clear ownership boundary. Help state is a single `showHelp bool` on Model, annotation list has 4 fields (`showAnnotList`, `annotListCursor`, `annotListOffset`, `annotListItems`), theme selector has an embedded `themeSelectState` struct with 10 fields. All are managed directly by Model methods.
- Overlay rendering dispatch is a bare `switch` in `View()` (lines 54-61) that picks one of three overlay renderers — a coordination concern that belongs in a dedicated type.
- Shared helpers (`overlayCenter`, `injectBorderTitle`) are Model methods despite having no conceptual dependency on Model — they operate on strings and dimensions.
- Key dispatch for overlays is buried inside `handleModalKey` (model.go:517-543) interleaved with annotation input and search input, making the modal priority order hard to follow.

**Why one package, not three:**

- Overlays are mutually exclusive — only one is visible at a time. A `Manager` coordinator naturally enforces this and owns the shared `Compose` step.
- All three overlays share `overlayCenter` (centered compositing) and `injectBorderTitle` (border chrome). These are overlay-domain concerns, not general UI utilities.
- Splitting into `overlay/help/`, `overlay/annotlist/`, `overlay/themeselect/` would fragment a small, cohesive domain (combined ~820 LOC of rendering + key handling) into three tiny packages that still need a coordinator.

## Context (from discovery)

**Current state of affected code** (on master):

- `app/ui/handlers.go` (410 LOC) — `helpOverlay()` (65-174, 110 lines), `writeTOCHelpSection()` (178-216), `overlayCenter()` (220-249, 30 lines), `handleHelpKey()` (314-324, 11 lines), plus `helpColors()`, `displayKeyName()`, `formatKeysForHelp()` helpers.
- `app/ui/annotlist.go` (310 LOC) — `annotListOverlay()` (36-65), `formatAnnotListItem()` (82-136), `injectBorderTitle()` (139-181), `handleAnnotListKey()` (185-232), `jumpToAnnotation()` (241-260), `positionOnAnnotation()` (264-278), `ensureHunkExpanded()` (284-295), `findDiffLineIndex()` (300-310). Also `buildAnnotListItems()` (19-26), `annotListBoxStyle()` (29-33), `annotListMaxVisible()` (235-237).
- `app/ui/themeselect.go` (403 LOC) — `themeSelectState` struct (26-37), `themeEntry` struct (40-44), `openThemeSelector()` (47-76), `buildThemeEntries()` (80-99), `applyThemeFilter()` (102-119), `themeSelectOverlay()` (122-158), `renderThemeFilter()` (161-169), `formatThemeEntry()` (172-209), `handleThemeSelectKey()` (213-274), `previewTheme()` (277-283), `applyTheme()` (286-312), `confirmThemeSelect()` (315-333), `cancelThemeSelect()` (336-346), `refreshDiff()` (349-354), `themeSelectMaxVisible()` (358-361), `colorsFromTheme()` (364-390), `swatchText()` (394-403).
- `app/ui/view.go` (375 LOC) — overlay switch block (54-61): `switch { case m.themeSel.active / m.showAnnotList / m.showHelp }`.
- `app/ui/model.go` (725 LOC) — overlay state fields at lines 205 (`showHelp`), 231-235 (`showAnnotList`, `annotListCursor`, `annotListOffset`, `annotListItems`, `pendingAnnotJump`), 243-244 (`themeSel`, `activeThemeName`). `handleModalKey` (517-543) dispatches to overlay handlers.

**Test files:**
- `app/ui/handlers_test.go` (548 LOC) — 12 help overlay tests (lines 18-208)
- `app/ui/annotlist_test.go` (707 LOC) — full annotation list coverage
- `app/ui/themeselect_test.go` (377 LOC) — full theme selector coverage

**Call-site audit** (what moves vs. what stays):

| Code | Moves to overlay/ | Stays in ui |
|---|---|---|
| `helpOverlay`, `writeTOCHelpSection`, `helpColors` | rendering + layout | `formatKeysForHelp`, `displayKeyName`, `helpKeyDisplay`, `helpLine` (spec-building, needs keymap) |
| `handleHelpKey` | key dispatch | — |
| `overlayCenter` | compositing | — |
| `injectBorderTitle` | border chrome | — |
| `annotListOverlay`, `formatAnnotListItem`, `annotListBoxStyle`, `annotListEmptyOverlay` | rendering | — |
| `handleAnnotListKey` | key dispatch → Outcome | jump logic (`jumpToAnnotation`, `positionOnAnnotation`, `ensureHunkExpanded`, `findDiffLineIndex`) |
| `annotListMaxVisible` | height calc | — |
| `buildAnnotListItems` | — | spec building from store |
| `themeSelectOverlay`, `renderThemeFilter`, `formatThemeEntry`, `swatchText` | rendering | — |
| `handleThemeSelectKey` | key dispatch → Outcome | `previewTheme`, `applyTheme`, `confirmThemeSelect`, `cancelThemeSelect`, `refreshDiff`, `colorsFromTheme` |
| `applyThemeFilter`, `themeSelectMaxVisible` | filter + height calc | — |
| `openThemeSelector`, `buildThemeEntries` | — | spec building |

**Dependencies from overlay/:**
- `app/ui/style` — `StyleKey`, `ColorKey`, `Color` types for `Resolver` interface
- `app/keymap` — `Action` type and constants for key dispatch
- `github.com/charmbracelet/bubbletea` — `tea.KeyMsg`
- `github.com/charmbracelet/lipgloss` — `lipgloss.Style` return from `Resolver.Style()`
- `github.com/charmbracelet/x/ansi` — `ansi.Cut`, `ansi.StringWidth`, `ansi.Truncate` for compositing

No dependency on `annotation`, `theme`, `diff`, `highlight`, or `ui.Model`.

**Note on `go-runewidth`:** Both `helpOverlay` and `formatThemeEntry` use `runewidth.StringWidth` for column width calculation and `runewidth.Truncate` for name truncation. The `github.com/mattn/go-runewidth` package is already in `go.mod` and must be imported by `overlay/`.

## Development Approach

- **testing approach**: Regular — implementation first, then tests for each unit.
- **Complete each milestone fully before moving to the next.** Individual tasks within a milestone may leave the codebase in an intermediate broken state — that's allowed and expected. See the "Testing Policy" section below.
- **Small focused changes within each task**. No bundling unrelated changes.
- **CRITICAL: update this plan file when scope changes during implementation**.
- **Single-level task numbering** (Task 1 through Task N, no hierarchical Task 1.1/1.2 etc). Ralphex executes flat numbering sequences.

## Testing Policy — CRITICAL: Milestone Gating, Not Per-Task

**This section deliberately overrides the default planning policy of "all tests must pass before starting next task".**

This refactor is a single coordinated extraction touching rendering, key dispatch, and state management across 4 production files plus 3 test files. It is structurally impossible for every intermediate task to leave `go test ./... -race` green, because:

- The new `overlay.Manager` must exist before any call site can delegate to it.
- Old overlay methods on Model must be removed before the new type can fully replace them.
- Between those two points, some call sites use the new API and some still use the old — neither state is consistent.

**The correct gating is per-milestone.** Three milestones, each with a specific green-gate:

| Milestone | What's built | Green-gate |
|---|---|---|
| **M1: Overlay package skeleton** | New `app/ui/overlay/` package complete and self-contained: types, Manager, popup helpers, help/annotlist/themeselect state+rendering+key handling, tests. `ui` package is NOT yet migrated. | `go test ./app/ui/overlay/... -race` green + `golangci-lint run app/ui/overlay/...` clean. **Full project build may be broken** — expected. |
| **M2: Call-site migration** | All ui call sites updated to new API. Old overlay code removed from `handlers.go`, `annotlist.go`, `themeselect.go`. Model fields replaced with `overlay overlayManager` interface. | Full `go test ./... -race` green. `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` clean. Manual smoke test: help overlay (`?`), annotation list (`@`), theme selector (`T`). |
| **M3: Docs + cleanup** | `app/ui/doc.go`, `CLAUDE.md`, and plugin reference docs updated. Orphan references audited. Plan moved to completed/. | Same as M2 gate + doc cross-check. |

**Non-milestone tasks do NOT have "run tests — must pass" checkboxes.** They may note "may leave N tests failing until milestone gate". The milestone task is where convergence happens.

**Commit structure**: one git commit per milestone (3 total). Each milestone commit is a coherent unit that leaves the tree green (M2/M3) or at least leaves `overlay/` green (M1).

**If hit failing tests inside a non-milestone task — that is expected. Proceed to the next task. Only escalate if a MILESTONE task leaves failures.**

## Design Philosophy (Inherited from Style Plan)

All 10 design principles from `docs/plans/completed/2026-04-10-style-extraction.md` (Design Philosophy section) apply here unchanged. The ones most relevant to this extraction:

1. **Sub-packages own their domain completely** — `overlay` owns all popup state (cursor, offset, filter text, items, active kind), rendering (box layout, item formatting, title injection, centering), and key dispatch (navigation, confirm, cancel, filter input). Callers supply specs and handle outcomes.
2. **Named types for domain values** — `Kind`, `OutcomeKind`, `RenderCtx`, `HelpSpec`, `AnnotListSpec`, `ThemeSelectSpec`, `AnnotationTarget`, `ThemeChoice` — all explicit types.
3. **Methods over standalone functions, per-type not god-object** — each overlay type (`helpOverlay`, `annotListOverlay`, `themeSelectOverlay`) owns its own methods. `Manager` is a thin coordinator that delegates to the active overlay. Shared composition helpers (`overlayCenter`, `injectBorderTitle`) are Manager methods since only Manager calls them. Overlay-specific rendering/formatting helpers are methods on the respective overlay type. This avoids turning Manager into a god object.
4. **Consumer-side interfaces** — two levels:
    - `overlay` package defines its own `Resolver` interface as a narrow view of `style` (consumer-side from overlay's perspective): `Style(style.StyleKey) lipgloss.Style` + `Color(style.ColorKey) style.Color`.
    - `ui/model.go` defines `overlayManager` interface (consumer-side from ui's perspective). Model holds it as an interface-typed field wired via `ModelConfig`.
5. **Outcome-based side effects** — `Manager.HandleKey()` returns an `Outcome` value. Model's dispatch code switches on `OutcomeKind` and performs side effects (file jump, theme apply/persist, viewport centering). No callbacks, no interface injection from overlay into Model.
6. **Avoid fragmenting related data/behavior** — core types (`Manager`, `Kind`, `Outcome`, `RenderCtx`, `Resolver`) stay in `overlay.go`. Each overlay type and its methods live in its own file (`help.go`, `annotlist.go`, `themeselect.go`) — the file split follows the type boundary, not arbitrary size limits.
7. **Milestone-based test gating** — see Testing Policy above.
8. **Render-time dependency injection** — `Resolver` is passed via `RenderCtx` at render time, not stored on Manager. This handles theme preview changing the resolver mid-session without sync issues.
9. **Comprehensive tests from day one** — no file without its `_test.go` counterpart.
10. **Dependency injection without fallbacks** — `NewManager()` takes no arguments (Manager is stateless until opened). Open methods take fully-populated spec structs.

**Additional decisions for this extraction:**

**Compile-time assertion** (`var _ overlayManager = (*overlay.Manager)(nil)`) in `model.go`. Following the same pattern as `styleResolver`, `wordDiffer`, etc. The assertion documents the contract explicitly at the interface definition site.

**No moq mocks for `overlayManager` in `ui`.** Model tests can construct a real `overlay.Manager` and call real methods — the Manager is lightweight with no external dependencies. Mocking 8 methods via moq adds test maintenance for zero gain.

**`lastPreviewedName` dedup** inside `themeSelectOverlay` — the overlay type tracks the last theme name for which it emitted `OutcomeThemePreview`. On cursor movement, if the selected theme name equals `lastPreviewedName`, `handleKey` returns `OutcomeNone` instead of `OutcomeThemePreview`. This avoids redundant re-renders when navigating away and back to the same theme entry.

## Solution Overview

**Package layout:**

```
app/ui/overlay/
├── doc.go               — package-level godoc
├── overlay.go           — Manager struct (thin coordinator), Kind/OutcomeKind enums, Outcome,
│                          RenderCtx, Resolver interface, NewManager(), Active(), Kind(), Close(),
│                          HandleKey() (routes to active overlay), Compose() (routes + overlayCenter),
│                          overlayCenter(), injectBorderTitle() (Manager-only composition helpers)
├── help.go              — helpOverlay struct + methods: render, handleKey.
│                          Exported spec types: HelpSpec, HelpSection, HelpEntry
├── annotlist.go         — annotListOverlay struct + methods: render, handleKey, formatItem, boxStyle, etc.
│                          Exported spec types: AnnotListSpec, AnnotationItem, AnnotationTarget
├── themeselect.go       — themeSelectOverlay struct + methods: render, handleKey, formatEntry,
│                          swatchText, applyFilter, renderFilter, etc.
│                          Exported spec types: ThemeSelectSpec, ThemeItem, ThemeChoice
├── overlay_test.go      — tests for overlay.go (Manager coordination, overlayCenter, injectBorderTitle)
├── help_test.go         — tests for help.go (helpOverlay rendering + key handling)
├── annotlist_test.go    — tests for annotlist.go (annotListOverlay rendering + key handling)
└── themeselect_test.go  — tests for themeselect.go (themeSelectOverlay rendering + filtering + keys)

File rules:
- Each file owns one type — file boundaries follow type boundaries, not size limits.
- Test files: 1:1 with source files. Split only past ~1000 lines (soft limit).
```

**Manager (thin coordinator) public API:**

```go
type Manager struct {
    kind     Kind
    help     helpOverlay        // owns help state + methods
    annotLst annotListOverlay   // owns annotation list state + methods
    themeSel themeSelectOverlay // owns theme selector state + methods
}
func NewManager() *Manager

func (m *Manager) Active() bool
func (m *Manager) Kind() Kind
func (m *Manager) OpenHelp(spec HelpSpec)
func (m *Manager) OpenAnnotList(spec AnnotListSpec)
func (m *Manager) OpenThemeSelect(spec ThemeSelectSpec)
func (m *Manager) Close()
func (m *Manager) HandleKey(msg tea.KeyMsg, action keymap.Action) Outcome  // routes to active overlay
func (m *Manager) Compose(base string, ctx RenderCtx) string              // routes + overlayCenter
// unexported Manager methods: overlayCenter, injectBorderTitle (shared composition)
```

**Per-overlay types (unexported, each owns its methods):**

```go
// help.go
type helpOverlay struct { active bool; spec HelpSpec }
func (h *helpOverlay) render(ctx RenderCtx, mgr *Manager) string     // mgr for injectBorderTitle
func (h *helpOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome

// annotlist.go
type annotListOverlay struct { active bool; items []AnnotationItem; cursor, offset int }
func (a *annotListOverlay) render(ctx RenderCtx, mgr *Manager) string
func (a *annotListOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome
// + formatItem, boxStyle, maxVisible, emptyOverlay

// themeselect.go
type themeSelectOverlay struct { active bool; all, entries []ThemeItem; cursor, offset int; filter string; lastPreviewedName string }
func (t *themeSelectOverlay) render(ctx RenderCtx, mgr *Manager) string
func (t *themeSelectOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome
// + formatEntry, swatchText, applyFilter, renderFilter, maxVisible
```

**Type hierarchy (exported):**

```go
// coordination
type Kind int           // KindNone, KindHelp, KindAnnotList, KindThemeSelect
type OutcomeKind int    // OutcomeNone, OutcomeClosed, OutcomeAnnotationChosen,
                        // OutcomeThemePreview, OutcomeThemeConfirmed, OutcomeThemeCanceled

type Outcome struct {
    Kind             OutcomeKind
    AnnotationTarget *AnnotationTarget
    ThemeChoice      *ThemeChoice
}

type RenderCtx struct {
    Width    int
    Height   int
    Resolver Resolver
}

type Resolver interface {
    Style(k style.StyleKey) lipgloss.Style
    Color(k style.ColorKey) style.Color
}

// help
type HelpSpec struct{ Sections []HelpSection }
type HelpSection struct{ Title string; Entries []HelpEntry }
type HelpEntry struct{ Keys string; Description string }

// annotation list
type AnnotListSpec struct{ Items []AnnotationItem }
type AnnotationItem struct {
    File, ChangeType, Comment string
    Line                      int
    Target                    AnnotationTarget
}
type AnnotationTarget struct {
    File, ChangeType string
    Line             int
}

// theme selector
type ThemeSelectSpec struct {
    Items      []ThemeItem
    ActiveName string
}
type ThemeItem struct {
    Name        string
    Local       bool
    AccentColor string  // hex color for swatch
}
type ThemeChoice struct{ Name string }
```

**Consumer-side interface in `model.go`:**

```go
type overlayManager interface {
    Active() bool
    Kind() overlay.Kind
    OpenHelp(spec overlay.HelpSpec)
    OpenAnnotList(spec overlay.AnnotListSpec)
    OpenThemeSelect(spec overlay.ThemeSelectSpec)
    Close()
    HandleKey(msg tea.KeyMsg, action keymap.Action) overlay.Outcome
    Compose(base string, ctx overlay.RenderCtx) string
}
```

## What Stays in ui

These methods remain on Model because they perform side effects that depend on Model state:

**Annotation list side effects** (stay in `annotlist.go` as slimmed-down handlers):
- `jumpToAnnotation()` — cross-file load via `pendingAnnotJump` + `selectByPath` + `loadSelectedIfChanged`
- `positionOnAnnotation()` — sets `diffCursor`, calls `ensureHunkExpanded`, `syncTOCActiveSection`, `centerViewportOnCursor`
- `ensureHunkExpanded()` — expands collapsed hunks for annotation targets
- `findDiffLineIndex()` — maps annotation line+type to diffLines index
- `buildAnnotListItems()` — builds spec from `m.store` snapshot

**Theme selector side effects** (stay in `themeselect.go` as slimmed-down handlers):
- `openThemeSelector()` — becomes spec builder: calls `buildThemeEntries`, saves orig state, builds `ThemeSelectSpec`, calls `m.overlay.OpenThemeSelect(spec)`
- `buildThemeEntries()` — loads themes from disk via `theme.ListOrdered` + `theme.Load`
- `previewTheme()` — applies theme to resolver/renderer/sgr/highlighter (no persist)
- `applyTheme()` — builds new resolver/renderer from theme colors, re-highlights, re-renders
- `confirmThemeSelect()` — applies theme + persists to config file
- `cancelThemeSelect()` — restores orig resolver/renderer/sgr/chroma
- `refreshDiff()` — re-highlights + re-renders after theme change
- `colorsFromTheme()` — converts `theme.Theme` → `style.Colors`

**Help spec-building helpers** (stay in `handlers.go`):
- `formatKeysForHelp(action)` — calls `m.keymap.KeysFor(action)`, needs keymap access; pre-formats `HelpEntry.Keys` strings
- `displayKeyName(key)` — key name display map (`helpKeyDisplay`, `helpLine` struct at lines 18-34); called by `formatKeysForHelp`
- Help open action builds the full `HelpSpec` (sections + entries with pre-formatted key strings) and passes it to `m.overlay.OpenHelp(spec)`

**Theme preview session state** remains on Model:
- `origResolver`, `origRenderer`, `origSGR`, `origChroma` — saved on open, restored on cancel
- `activeThemeName` — tracks confirmed theme
- `themesDir`, `configPath` — filesystem paths

## Implementation Steps

### Task 1: Bootstrap overlay package with types, Manager skeleton, and popup composition

**Files:**
- Create: `app/ui/overlay/doc.go`
- Create: `app/ui/overlay/overlay.go`
- Create: `app/ui/overlay/overlay_test.go`

- [x] create `app/ui/overlay/doc.go` with package-level godoc describing the overlay sub-package purpose
- [x] create `app/ui/overlay/overlay.go` with all exported types: `Kind` (iota enum), `OutcomeKind` (iota enum), `Outcome` struct, `RenderCtx` struct, `Resolver` interface
- [x] add `Manager` struct — thin coordinator with fields: `kind Kind`, `help helpOverlay`, `annotLst annotListOverlay`, `themeSel themeSelectOverlay` (each overlay is its own type defined in its own file)
- [x] implement `NewManager() *Manager`, `Active() bool`, `Kind() Kind`, `Close()`
- [x] implement `HandleKey` that routes to the active overlay's `handleKey` method based on `m.kind` and returns the `Outcome`
- [x] implement `Compose` that routes to the active overlay's `render` method and wraps with `overlayCenter`; returns base unchanged when `m.kind == KindNone`
- [x] implement `(m *Manager) overlayCenter(bg, fg string, width int) string` — port from `handlers.go:220-249`, replace `m.width` with explicit param, use `ansi.Cut`/`ansi.StringWidth` for ANSI-aware line splicing
- [x] implement `(m *Manager) injectBorderTitle(box, title string, popupWidth int, bgColor string) string` — port from `annotlist.go:139-181`, replace `m.resolver.Color(style.ColorKeyDiffPaneBg)` with explicit `bgColor` param
- [x] write tests for `overlayCenter`: empty bg, empty fg, fg smaller than bg, fg wider than bg, ANSI content in both
- [x] write tests for `injectBorderTitle`: title injection, empty title, title with ANSI, empty bgColor fallback

### Task 2: Implement help overlay

**Files:**
- Create: `app/ui/overlay/help.go`
- Create: `app/ui/overlay/help_test.go` (1:1 with source file)

- [x] define `helpOverlay` struct (unexported): `active bool`, `spec HelpSpec`
- [x] add `OpenHelp(spec HelpSpec)` to Manager — sets `m.kind = KindHelp`, delegates to `m.help.open(spec)`
- [x] implement `(h *helpOverlay) render(ctx RenderCtx, mgr *Manager) string` — port two-column layout from `handlers.go:65-174`, `helpColors` (59-61), and `writeTOCHelpSection` (178-216). Replace `m.resolver`, `m.width`, `m.height` with `RenderCtx` + spec data. `mgr` param is for `injectBorderTitle` call. Note: `formatKeysForHelp` (48-55), `displayKeyName` (37-45), `helpKeyDisplay` map (21-34), and `helpLine` struct (18) stay in `handlers.go` — they need `m.keymap` access and pre-format the `HelpEntry.Keys` strings at spec-building time. TOC section: include in `HelpSpec.Sections` at build time instead of a separate method
- [x] implement `(h *helpOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome` — port `handleHelpKey` (314-324): `ActionHelp` toggles, `ActionDismiss`/Esc closes, all other keys consumed. Return `OutcomeClosed` on close, `OutcomeNone` otherwise
- [x] wire help into Manager's `HandleKey` and `Compose` — `Compose` calls `m.overlayCenter(base, m.help.render(ctx, m), ctx.Width)` when help is active
- [x] write tests for help rendering: section headers present, key names present, two-column layout, TOC section, custom keybinding reflected
- [x] write tests for help key handling: toggle on/off, Esc close, other keys blocked

### Task 3: Implement annotation list overlay

**Files:**
- Create: `app/ui/overlay/annotlist.go`
- Create: `app/ui/overlay/annotlist_test.go` (1:1 with source file)

- [x] define `annotListOverlay` struct (unexported): `active bool`, `items []AnnotationItem`, `cursor int`, `offset int`
- [x] add `OpenAnnotList(spec AnnotListSpec)` to Manager — sets `m.kind = KindAnnotList`, delegates to `m.annotLst.open(spec)`
- [x] implement `(a *annotListOverlay) render(ctx RenderCtx, mgr *Manager) string` — port `annotListOverlay` (36-65), `annotListBoxStyle` (29-33), `annotListEmptyOverlay` (68-79), `annotListMaxVisible` (235-237). `mgr` param for `injectBorderTitle`. Replace `m.resolver` with `RenderCtx.Resolver`, `m.width`/`m.height` with `RenderCtx` dimensions
- [x] implement `(a *annotListOverlay) formatItem(item AnnotationItem, width int, selected bool, resolver Resolver) string` — port `formatAnnotListItem` (82-136). Color resolution: map `ChangeType` ("+"/"-"/" ") to style keys via Resolver
- [x] implement `(a *annotListOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome` — port `handleAnnotListKey` (185-232): ActionAnnotList toggles off, Enter returns `OutcomeAnnotationChosen` with `AnnotationTarget`, Esc/ActionDismiss returns `OutcomeClosed`, Up/Down navigate cursor with scroll offset. All other keys consumed
- [x] wire annotation list into Manager's `HandleKey` and `Compose`
- [x] write tests for annotation list rendering: item formatting (add/remove/context types), cursor highlight, scroll offset, empty state, truncation
- [x] write tests for annotation list key handling: navigation (up/down bounds, scroll), enter selection, toggle close, esc close

### Task 4: Implement theme selector overlay

**Files:**
- Create: `app/ui/overlay/themeselect.go`
- Create: `app/ui/overlay/themeselect_test.go` (1:1 with source file)

- [x] define `themeSelectOverlay` struct (unexported): `active bool`, `all []ThemeItem`, `entries []ThemeItem` (filtered), `cursor int`, `offset int`, `filter string`, `lastPreviewedName string`
- [x] add `OpenThemeSelect(spec ThemeSelectSpec)` to Manager — sets `m.kind = KindThemeSelect`, delegates to `m.themeSel.open(spec)`
- [x] implement `(t *themeSelectOverlay) applyFilter()` — port `applyThemeFilter` (102-119): case-insensitive name match, reset cursor/offset on filter change
- [x] implement `(t *themeSelectOverlay) render(ctx RenderCtx, mgr *Manager) string` — port `themeSelectOverlay` (122-158), and popup dimension constants (`themePopupMaxWidth`, `themePopupMinWidth`, `themePopupMargin`, `themePopupBorderPad`, `themePopupChromeLines` at 18-23). `mgr` param for `injectBorderTitle`. Replace `m.resolver` with `RenderCtx.Resolver`
- [x] implement `(t *themeSelectOverlay) formatEntry(...)`, `renderFilter(...)`, `swatchText(...)`, `maxVisible(...)` — port from themeselect.go (161-209, 358-361, 394-403)
- [x] implement `(t *themeSelectOverlay) handleKey(msg tea.KeyMsg, action keymap.Action) Outcome` — port `handleThemeSelectKey` (213-274): Enter returns `OutcomeThemeConfirmed`, Esc first clears filter (+ returns `OutcomeThemePreview` to re-preview unfiltered selection), Esc second returns `OutcomeThemeCanceled`. Up/Down navigate + return `OutcomeThemePreview` (with dedup via `lastPreviewedName`). Rune input appends to filter + returns `OutcomeThemePreview`. Backspace pops filter. ActionThemeSelect returns `OutcomeThemeCanceled`. Note: theme selector uses `msg.Type` for navigation/filter input (not keymap actions) — fzf-style behavior where all printable chars go to filter. Only `ActionThemeSelect` uses the keymap action
- [x] wire theme selector into Manager's `HandleKey` and `Compose`
- [x] write tests for theme selector rendering: item formatting (local vs gallery swatch), filter input display, filtered count, cursor highlight, scroll
- [x] write tests for theme selector key handling: navigation, filter input/clear, enter confirm, esc cancel (two-press), preview dedup (same name = OutcomeNone)
- [x] write tests for `OpenThemeSelect` cursor positioning on `ActiveName`

### Task 5: M1 milestone gate — overlay package green

- [x] run `go test ./app/ui/overlay/... -race` — must pass
- [x] run `golangci-lint run app/ui/overlay/...` — must be clean
- [x] verify all overlay types and Manager methods are tested
- [x] verify no dependency on `annotation`, `theme`, `diff`, `highlight`, or `ui` packages (check imports)

### Task 6: Add consumer-side interface and ModelConfig wiring

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/main.go`

- [x] add `overlayManager` interface in `model.go` with 8 methods (Active, Kind, OpenHelp, OpenAnnotList, OpenThemeSelect, Close, HandleKey, Compose)
- [x] add compile-time assertion: `var _ overlayManager = (*overlay.Manager)(nil)`
- [x] add `overlay overlayManager` field to `Model` struct, remove old overlay fields: `showHelp`, `showAnnotList`, `annotListCursor`, `annotListOffset`, `annotListItems`, `themeSel` (the `themeSelectState` struct). Keep `activeThemeName`, `themesDir`, `configPath`, `pendingAnnotJump`
- [x] add `Overlay overlayManager` to `ModelConfig` struct
- [x] wire `overlay` field in `NewModel` from `ModelConfig.Overlay`, add nil check matching existing pattern for required deps (model.go:331-356)
- [x] add `Overlay: overlay.NewManager()` to ModelConfig in `app/main.go` (around line 415)

### Task 7: Migrate View() overlay dispatch

**Files:**
- Modify: `app/ui/view.go`

- [x] replace the `switch { case m.themeSel.active / m.showAnnotList / m.showHelp }` block (lines 54-61) with single call: `mainView = m.overlay.Compose(mainView, overlay.RenderCtx{Width: m.width, Height: m.height, Resolver: m.resolver})`
- [x] remove `overlayCenter` method from `handlers.go` (lines 220-249) — now a Manager method in `overlay/overlay.go`

### Task 8: Migrate help overlay dispatch and consolidate overlay key routing

**Files:**
- Modify: `app/ui/handlers.go`
- Modify: `app/ui/model.go`

- [x] consolidate all overlay dispatch into `handleModalKey`: add a single `if m.overlay.Active()` guard after annotation-input and search-input checks (model.go:525). This replaces the separate help check in `handleKey` (462-465) and the annotList/themeSelect checks in `handleModalKey` (530-539) with one unified block that delegates to `m.overlay.HandleKey(msg, action)` and switches on `Outcome.Kind`
- [x] update help open action in `handleKey`: replace direct `m.showHelp = !m.showHelp` with: if overlay active and kind is help → `m.overlay.Close()`, else build `HelpSpec` from `m.keymap.HelpSections()` + `m.overlay.OpenHelp(spec)`
- [x] remove `handleHelpKey`, `helpOverlay`, `writeTOCHelpSection`, `helpColors` methods from `handlers.go`. Keep `formatKeysForHelp`, `displayKeyName`, `helpKeyDisplay`, `helpLine` — they are spec-building helpers used when constructing `HelpSpec`
- [x] remove help-related tests from `handlers_test.go` (lines 18-208) — covered by `overlay/help_test.go` now

### Task 9: Migrate annotation list overlay dispatch

**Files:**
- Modify: `app/ui/annotlist.go`
- Modify: `app/ui/model.go`
- Modify: `app/ui/annotlist_test.go`

- [x] add annotation list outcome handling to the unified overlay dispatch in `handleModalKey` (wired in Task 8):
  - `OutcomeAnnotationChosen` → call `m.jumpToAnnotationTarget(outcome.AnnotationTarget)`
  - `OutcomeClosed` → no-op (overlay already closed itself)
- [x] update annotation list open action in `handleKey` (model.go:469-474): build `AnnotListSpec` from `buildAnnotListItems()` → convert `[]annotation.Annotation` to `[]overlay.AnnotationItem`, call `m.overlay.OpenAnnotList(spec)`
- [x] add `jumpToAnnotationTarget(target *overlay.AnnotationTarget)` method — constructs `annotation.Annotation{File: target.File, Line: target.Line, Type: target.ChangeType}` and delegates to existing jump logic. `pendingAnnotJump` type stays as `*annotation.Annotation` (no type change) — `positionOnAnnotation` and `loaders.go:194` consume it unchanged
- [x] remove from `annotlist.go`: `annotListOverlay`, `annotListEmptyOverlay`, `formatAnnotListItem`, `annotListBoxStyle`, `injectBorderTitle`, `handleAnnotListKey`, `annotListMaxVisible`. Keep: `buildAnnotListItems`, `jumpToAnnotation` (refactored), `positionOnAnnotation`, `ensureHunkExpanded`, `findDiffLineIndex`
- [x] update `annotlist_test.go` — remove rendering/navigation tests (now in `overlay/annotlist_test.go`), keep jump/position tests, update remaining tests for new interface field

### Task 10: Migrate theme selector overlay dispatch

**Files:**
- Modify: `app/ui/themeselect.go`
- Modify: `app/ui/model.go`
- Modify: `app/ui/themeselect_test.go`

- [ ] add theme selector outcome handling to the unified overlay dispatch in `handleModalKey` (wired in Task 8):
  - `OutcomeThemePreview` → call `m.previewThemeByName(outcome.ThemeChoice.Name)` (new method: looks up theme in saved entries, calls `applyTheme`)
  - `OutcomeThemeConfirmed` → call `m.confirmThemeByName(outcome.ThemeChoice.Name)` (new method: applies + persists)
  - `OutcomeThemeCanceled` → call `m.cancelThemeSelect()` (existing, restores originals)
- [ ] refactor `openThemeSelector()` — build `ThemeSelectSpec` from `buildThemeEntries`, save orig state (resolver/renderer/sgr/chroma), call `m.overlay.OpenThemeSelect(spec)`
- [ ] add `previewThemeByName(name string)` and `confirmThemeByName(name string)` methods — look up theme from saved entries list (stored on Model during open), delegate to existing `applyTheme`/`patchConfigTheme`
- [ ] remove `themeSelectState` struct and `themeEntry` struct from `themeselect.go` — replaced by `overlay.ThemeSelectSpec`/`overlay.ThemeItem`. Add `themePreviewSession` struct to hold: `entries []themeEntry`, `origResolver`, `origRenderer`, `origSGR`, `origChroma` (the app-side preview state)
- [ ] remove from `themeselect.go`: `themeSelectOverlay`, `renderThemeFilter`, `formatThemeEntry`, `handleThemeSelectKey`, `applyThemeFilter`, `themeSelectMaxVisible`, `swatchText`. Keep: `openThemeSelector` (refactored), `buildThemeEntries`, `previewTheme` (refactored), `applyTheme`, `confirmThemeSelect` (refactored), `cancelThemeSelect`, `refreshDiff`, `colorsFromTheme`
- [ ] update `themeselect_test.go` — remove rendering/navigation/filter tests (now in `overlay/themeselect_test.go`), keep apply/confirm/cancel/preview tests, update for new interface field and `themePreviewSession` struct

### Task 11: Update test helpers and fix Model construction in tests

**Files:**
- Modify: `app/ui/model.go` (if needed for test helper)
- Modify: various `app/ui/*_test.go` files

- [ ] update all test helpers that construct `Model` or `ModelConfig` to include `Overlay: overlay.NewManager()` field
- [ ] verify no test references removed fields (`showHelp`, `showAnnotList`, `annotListCursor`, `annotListOffset`, `annotListItems`, `themeSel`)
- [ ] fix any compilation errors in test files from the field removal

### Task 12: M2 milestone gate — full project green

- [ ] run `go test ./... -race` — must pass
- [ ] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — must be clean
- [ ] run `~/.claude/format.sh` — must produce no changes
- [ ] manual smoke test: open revdiff on a git repo, verify `?` (help), `@` (annotation list), `T` (theme selector) all work correctly
- [ ] verify theme preview on cursor move, confirm with Enter, cancel with Esc (two-press), filter typing

### Task 13: Update documentation

**Files:**
- Modify: `app/ui/doc.go`
- Modify: `app/ui/overlay/doc.go` (if needed)
- Modify: `CLAUDE.md`

- [ ] update `app/ui/doc.go` to reference new `overlay` sub-package and `overlayManager` interface
- [ ] update `CLAUDE.md` project structure section: add `app/ui/overlay/` entry with file descriptions, add `overlayManager` to Key Interfaces section, update Data Flow section if needed
- [ ] audit `handlers.go`, `annotlist.go`, `themeselect.go` for stale comments referencing removed code

### Task 14: M3 milestone gate — docs and cleanup

- [ ] verify `CLAUDE.md` overlay section matches actual package contents
- [ ] verify no orphan references to `showHelp`, `showAnnotList`, `annotListCursor`, `annotListOffset`, `annotListItems`, `themeSel`, `themeSelectState` in any `.go` file
- [ ] run full test suite: `go test ./... -race`
- [ ] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test all three overlays on a multi-file diff repo
- test theme selector with both bundled and local custom themes
- test annotation list with cross-file annotation jumps
- test help overlay with custom keybindings

**Plugin/docs sync:**
- if CLAUDE.md changes affect `.claude-plugin/skills/revdiff/references/`, sync those files
- if `plugins/codex/skills/` references overlay internals, update accordingly
