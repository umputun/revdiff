# Compact Diff Mode

## Overview

Add a `--compact` toggle that switches the underlying VCS diff from full-file context (`-U1000000`) to small-context (`-U5` by default) unified diff. Mirrors the existing `--collapsed` / `v` pattern: a CLI flag sets the initial mode and a runtime key (`C`) toggles between compact and full-file at any time.

**Problem**: reviewing small changes in large files (e.g. a one-line addition in a 5000-line `translations.json`) is painful because the cursor lands at line 1 and the user has to navigate through thousands of unchanged lines to reach the change. Full-file context is revdiff's design default (deliberately — code-in-context is load-bearing for how annotations work), but for this specific "small change, huge file" case the mental model breaks.

**Solution**: don't shrink the view at the renderer level (already rejected — that was the "simplified view" removed on 2026-03-31). Instead, shrink at the diff-generation level: pass a smaller `-U` value to git/hg/jj so the VCS itself returns only changed lines plus N lines of surrounding context. Everything else in revdiff (parser, renderer, annotations, hunk navigation) continues to work unchanged because it already handles non-full-context diffs.

**Design scope**: mode affects only VCS renderers (git, hg, jj). Context-only sources (FileReader, DirectoryReader, StdinReader) ignore the context parameter — there are no "changes" to contextualize. The `CompactApplicable` flag lets the UI no-op the toggle with a status-bar hint in those modes.

## Context (from discovery)

**Files involved**:
- `app/config.go` — options struct with go-flags tags
- `app/diff/exclude.go` — defines the diff-package `Renderer` interface (shared by all renderers in `app/diff/`)
- `app/diff/diff.go` — `fullFileContext = "-U1000000"` constant at line 25; `Git.FileDiff()` at line ~370
- `app/diff/hg.go` — `Hg.FileDiff()` at line 148
- `app/diff/jj.go` — `jjFullContext = "--context=1000000"` at line 14; `Jj.FileDiff()`
- `app/diff/fallback.go`, `directory.go`, `include.go`, `exclude.go` (ExcludeFilter impl), `stdin.go` — wrapper/context-only renderers
- `app/diff/mocks/Renderer.go` — moq-generated mock for `diff.Renderer` (regenerate via `go generate`)
- `app/ui/model.go` — **separate** UI-side `Renderer` interface at lines 38-42 (consumer-side interface, not the same as `diff.Renderer`); also `modeState` struct, `ModelConfig` struct, `CommitsApplicable` precedent
- `app/ui/mocks/renderer.go` — moq-generated mock for the UI-side `Renderer` interface (regenerate via `go generate`)
- `app/keymap/keymap.go` — `Action` constants, `validActions` map, `defaultDescriptions()`, default bindings
- `app/ui/loaders.go` — `triggerReload()` at line 135, `handleFileLoaded` at line 213, `resolveEmptyDiff` with staged-retry `FileDiff` at line 293, `skipInitialDividers()` at line 425
- `app/ui/diffnav.go` — key action dispatch
- `app/ui/view.go` — `statusModeIcons()` at line 330
- `app/main.go` — `commitsApplicable()` at line 234, `ModelConfig` wiring at line 157 (`Collapsed: opts.Collapsed` at line 163 shows the exact wiring pattern to mirror)
- `app/renderer_setup.go` — VCS renderer construction + capability detection

**Related patterns to mirror**:
- `--collapsed` flag + `v` toggle (boolean mode + keybinding) — most direct analog
- `CommitsApplicable` / capability plumbing — for `CompactApplicable`
- `R` reload behavior (resets cursor to first hunk via `skipInitialDividers()`) — for toggle re-fetch
- `ActionToggleCollapsed` — for keymap action naming convention

**Dependencies identified**:
- Parser already handles non-full-context diffs (divider lines between non-adjacent hunks work — see `diff.go:596`)
- `handleFileLoaded` already fetches fresh per navigation, so lazy re-fetch on other files is automatic — only the current file needs an explicit re-fetch on toggle
- moq regenerates from `//go:generate` comment in `app/diff/exclude.go:5`

## Development Approach

- **testing approach**: Regular (code first, then tests) — matches project default; each task writes implementation then tests immediately before moving on
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - write unit tests for new/modified functions
  - add test cases for new code paths (compact flag on/off, context=0/5/N)
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change with `go test ./...`
- maintain backward compatibility (adding a 4th parameter to `FileDiff` is a breaking API change for internal consumers only; external API is not exposed)

## Testing Strategy

- **unit tests**: required for every task
  - `app/diff/*_test.go` — verify each renderer passes the right `-U` arg to the VCS based on `contextLines`
  - `app/config_test.go` — verify `--compact` and `--compact-context=N` parse correctly from CLI, env, and config file
  - `app/keymap/keymap_test.go` — verify `ActionToggleCompact` registered, default binding `C`
  - `app/ui/*_test.go` — verify toggle handler, re-fetch, cursor reset, applicable-flag no-op behavior
  - `app/renderer_setup_test.go` — verify `compactApplicable()` returns expected values for each source type
- **moq regeneration**: after interface change, `go generate ./...` must be run; the regenerated `Renderer.go` mock in `app/diff/mocks/` is a required artifact
- **e2e tests**: not applicable (no browser UI tests in this project)

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

**Architecture**:
- Context size is a per-call parameter on `Renderer.FileDiff()`. The VCS renderers (git, hg, jj) construct their `-U` / `--context=` arg from it; wrappers pass it through to their inner renderer; context-only sources ignore it.
- The UI holds a single `compact` boolean on `modes` and a `contextLines int` on config; when the user presses `C`, the boolean flips and the current file is re-fetched with the new value. Other files re-fetch naturally on next navigation.
- The `CompactApplicable` capability flag is computed once at the composition root (in `main.go`) based on whether the underlying renderer is a VCS type. The UI consults this flag on toggle; if `!applicable`, the key is a no-op with a transient status-bar hint.

**Key design decisions**:
- **Parameter vs setter**: chose parameter (`FileDiff(ref, file string, staged bool, contextLines int)`). Consistent with existing `staged bool` parameter; no hidden mutable state on renderers; compiler catches every callsite on change.
- **Two flags not one**: `--compact` (boolean) + `--compact-context=N` (default 5). Mirrors `--collapsed` idiom exactly. A combined `--context=N` flag was considered and rejected — blurs the toggle semantics and requires magic values (e.g. 0 = full) that don't read clearly.
- **Mode stacking with collapsed**: compact shrinks the VCS diff before parsing; collapsed hides removed lines during rendering. They compose without special handling because they operate at different layers.
- **Status glyph**: `⊂` (subset) — reads as "this is a subset of the full diff".

## Technical Details

**New config fields** (`app/config.go`):
```go
Compact        bool `long:"compact" ini-name:"compact" env:"REVDIFF_COMPACT" description:"start in compact diff mode (small context around changes)"`
CompactContext int  `long:"compact-context" ini-name:"compact-context" env:"REVDIFF_COMPACT_CONTEXT" default:"5" description:"number of context lines around changes when in compact mode"`
```

**Renderer interface change** — two separate interfaces must be updated in lockstep:

1. `app/diff/exclude.go` (the diff-package interface, shared by all `app/diff/` implementations):
```go
type Renderer interface {
    ChangedFiles(ref string, staged bool) ([]FileEntry, error)
    FileDiff(ref, file string, staged bool, contextLines int) ([]DiffLine, error)
}
```

2. `app/ui/model.go` (the UI-package consumer-side interface, lines 38-42):
```go
type Renderer interface {
    ChangedFiles(ref string, staged bool) ([]diff.FileEntry, error)
    FileDiff(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error)
}
```

Both interfaces must be updated together as one atomic API change so `go build ./...` remains green after Task 1. Both moq mocks (`app/diff/mocks/Renderer.go` and `app/ui/mocks/renderer.go`) must be regenerated.

**VCS renderer behavior**:
- `contextLines == 0` or `contextLines >= 1000000` → use existing `fullFileContext = "-U1000000"` (full file)
- `contextLines > 0 && contextLines < 1000000` → use `-U<contextLines>`
- For jj: `--context=<contextLines>` (adjust `jjFullContext` usage accordingly)

**Wrapper behavior** (Fallback, Exclude, Include, Directory, Stdin):
- Each wrapper's `FileDiff` signature gets the new parameter
- Wrappers that delegate to an inner Renderer pass `contextLines` through unchanged
- Context-only wrappers (Directory, Stdin, FileReader within Fallback's fallback path) ignore it — they read files as all-context lines regardless

**Keymap** (`app/keymap/keymap.go`):
- New constant: `ActionToggleCompact Action = "toggle_compact"`
- Add to `validActions` map
- Default binding: `"C": ActionToggleCompact`
- Help entry: `{ActionToggleCompact, "toggle compact diff view", "View"}`

**Model state** (`app/ui/model.go`):
- Add `compact bool` field to `modeState` struct (alongside existing `collapsed`, `wrap`, etc.)
- Add `CompactApplicable bool` field to `ModelConfig`
- Add `compactContext int` field to `modeState` (stores the configured N)
- Initial value of `compact` comes from `opts.Compact`; `compactContext` from `opts.CompactContext`

**Toggle handler** (`app/ui/diffnav.go` or wherever key dispatch lives):
- On `ActionToggleCompact`:
  - If `!CompactApplicable`, set transient status hint and no-op
  - Else flip `m.modes.compact`, re-fetch current file via existing load machinery, reset cursor to first hunk via `skipInitialDividers()` after reload

**Capability detection** (`app/main.go`, `app/renderer_setup.go`):
- Add `compactApplicable(opts, renderer)` function — returns false for `--stdin`, `--all-files` (no diff), and when the concrete renderer is a context-only type
- Alternatively, add a `SupportsContext()` method or interface check — decide during Task 6
- Wire `CompactApplicable: compactApplicable(opts, r)` into `ModelConfig` construction at line ~157

**Status bar glyph** (`app/ui/view.go:330`):
- In `statusModeIcons()`, append `⊂` when `m.modes.compact` is true
- Follow existing pattern used for other mode icons

**FileDiff call sites** (all in UI):
- Every call to `m.renderer.FileDiff(...)` / `m.diffRenderer.FileDiff(...)` needs the new parameter
- Pass `m.currentContextLines()` — a helper that returns `m.modes.compactContext` if `compact` is on, else `0` (meaning "use full")
- Known callsites: `loaders.go:loadFileDiff` (primary), `loaders.go:293` in `resolveEmptyDiff` (staged retry for new files — uses the same context as the primary call, since it's re-fetching the same file with `staged=true`)
- Compiler will catch any additional missed callsites

**Capability detection approach** (`CompactApplicable`):
- Use **type assertion**, following the exact precedent set by `CommitLogger`. No new `SupportsContext()` interface. The wrapper chain (ExcludeFilter, IncludeFilter, FallbackRenderer) doesn't need to opt in — `compactApplicable()` checks `opts.Stdin`, `opts.AllFiles`, and type-asserts the renderer (or the innermost VCS it wraps) against the concrete context-only types (`*DirectoryReader`, `*StdinReader`, and the FileReader case inside Fallback).

**ModelConfig wiring** (follow `CommitsApplicable` + `Collapsed` precedent at `main.go:163`):
- Add `Compact bool` and `CompactContext int` fields to `ModelConfig` (alongside `Collapsed`)
- Add `CompactApplicable bool` to `ModelConfig` (alongside `CommitsApplicable`)
- In `main.go` `ModelConfig` construction: set `Compact: opts.Compact`, `CompactContext: opts.CompactContext`, `CompactApplicable: compactApplicable(opts, renderer)`
- In Model constructor: `m.modes.compact = cfg.Compact`, `m.modes.compactContext = cfg.CompactContext`, `m.compactApplicable = cfg.CompactApplicable`

## What Goes Where

- **Implementation Steps**: all code changes, tests, docs sync, moq regeneration — all achievable within this repo
- **Post-Completion**: manual sanity check on large translations file (reporter's scenario); announce in the discussion thread #134 after landing

## Implementation Steps

### Task 1: Add `contextLines` parameter to both Renderer interfaces and wire through all wrappers and UI callsites (atomic API change)

This is the full API change — both the `diff.Renderer` interface and the UI-side `ui.Renderer` interface get the new parameter in a single task so `go build ./...` remains green. All existing UI callsites pass `0` for now (preserves current full-file behavior); the actual use of `contextLines` at call-time is wired in Task 5.

**Files:**
- Modify: `app/diff/exclude.go` (diff-package Renderer interface + ExcludeFilter impl)
- Modify: `app/diff/fallback.go` (FallbackRenderer.FileDiff pass-through)
- Modify: `app/diff/directory.go` (DirectoryReader.FileDiff — accept & ignore)
- Modify: `app/diff/include.go` (IncludeFilter.FileDiff pass-through)
- Modify: `app/diff/stdin.go` (StdinReader.FileDiff — accept & ignore)
- Modify: `app/diff/diff.go` (Git.FileDiff signature only; arg construction in Task 2)
- Modify: `app/diff/hg.go` (Hg.FileDiff signature only; arg construction in Task 2)
- Modify: `app/diff/jj.go` (Jj.FileDiff signature only; arg construction in Task 2)
- Modify: `app/diff/fallback_test.go`
- Modify: `app/diff/exclude_test.go`
- Modify: `app/diff/include_test.go`
- Modify: `app/diff/directory_test.go` (if exists)
- Modify: `app/diff/stdin_test.go` (if exists)
- Modify: `app/ui/model.go` (UI-side Renderer interface at lines 38-42)
- Modify: `app/ui/loaders.go` (update all `FileDiff` callsites to pass `0`, including `resolveEmptyDiff:293`)
- Modify: `app/ui/loaders_test.go` and any other UI tests that construct mock renderers
- Regenerate: `app/diff/mocks/Renderer.go` via `go generate ./app/diff/...`
- Regenerate: `app/ui/mocks/renderer.go` via `go generate ./app/ui/...`

- [x] change `diff.Renderer.FileDiff` signature in `app/diff/exclude.go` to `FileDiff(ref, file string, staged bool, contextLines int) ([]DiffLine, error)`
- [x] change `ui.Renderer.FileDiff` signature in `app/ui/model.go:38-42` to `FileDiff(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error)`
- [x] update `FallbackRenderer.FileDiff` to accept and pass `contextLines` to inner
- [x] update `ExcludeFilter.FileDiff` to accept and pass `contextLines` to inner
- [x] update `IncludeFilter.FileDiff` to accept and pass `contextLines` to inner
- [x] update `DirectoryReader.FileDiff` to accept `contextLines` (unused — context-only source; document in comment)
- [x] update `StdinReader.FileDiff` to accept `contextLines` (unused — context-only source; document in comment)
- [x] update `Git.FileDiff`, `Hg.FileDiff`, `Jj.FileDiff` signatures to accept `contextLines int` — still use the hardcoded full-file arg in this task; arg construction changes in Task 2
- [x] run `go generate ./app/diff/...` to regenerate `app/diff/mocks/Renderer.go`
- [x] run `go generate ./app/ui/...` to regenerate `app/ui/mocks/renderer.go`
- [x] update every `FileDiff(...)` callsite in `app/ui/loaders.go` to pass `0` as the 4th argument (including `resolveEmptyDiff:293` staged retry)
- [x] update existing tests in `app/diff/` wrappers to pass `0` for `contextLines` (existing behavior preserved)
- [x] update existing tests in `app/ui/` that construct `RendererMock` or call `FileDiff` to pass `0` for `contextLines`
- [x] add test case to `fallback_test.go` verifying `contextLines` is passed through unchanged to inner (table-driven: 0, 5, 1000000)
- [x] add test case to `exclude_test.go` verifying `contextLines` is passed through unchanged to inner
- [x] add test case to `include_test.go` verifying `contextLines` is passed through unchanged to inner
- [x] run `go build ./...` — must succeed (confirms atomic API change covered all sites)
- [x] run `go test ./...` — must pass before Task 2 (behavior unchanged since everyone passes 0 and VCS renderers still use hardcoded full-context)

### Task 2: Use `contextLines` in Git/Hg/Jj `FileDiff` — replace hardcoded full-file arg with computed one

Task 1 already added the `contextLines` parameter to the three VCS implementations. This task swaps the hardcoded full-file arg for a computed one via small helpers.

**Files:**
- Modify: `app/diff/diff.go` (Git.FileDiff — line ~370 where `fullFileContext` is currently passed)
- Modify: `app/diff/hg.go` (Hg.FileDiff — line 153 where `fullFileContext` is currently passed)
- Modify: `app/diff/jj.go` (Jj.FileDiff — currently uses `jjFullContext = "--context=1000000"` at line 14)
- Modify: `app/diff/diff_test.go`
- Modify: `app/diff/hg_test.go`
- Modify: `app/diff/jj_test.go`

- [x] in `diff.go`: add helper `gitContextArg(contextLines int) string` returning `"-U1000000"` if `contextLines <= 0 || contextLines >= 1000000`, else `fmt.Sprintf("-U%d", contextLines)`. Use it in `Git.FileDiff` instead of hardcoded `fullFileContext`
- [x] in `hg.go`: add helper `hgContextArg(contextLines int) string` with identical semantics (also produces `"-U<N>"`). Use it in `Hg.FileDiff`
- [x] in `jj.go`: add helper `jjContextArg(contextLines int) string` producing `"--context=1000000"` for full, else `fmt.Sprintf("--context=%d", contextLines)`. Use it in `Jj.FileDiff`
- [x] keep the existing `fullFileContext` / `jjFullContext` constants but update their doc comments to "full-file sentinel value; use the per-renderer *ContextArg helpers for call-site context selection"
- [x] write table-driven tests for `gitContextArg`: input 0 → `-U1000000`, 5 → `-U5`, 1000000 → `-U1000000`, 1000001 → `-U1000000`, negative → `-U1000000` (defensive)
- [x] write analogous tests for `hgContextArg`
- [x] write analogous tests for `jjContextArg` (note `--context=` form)
- [x] in `diff_test.go`: add a test case for `Git.FileDiff` that creates a real tiny git repo (following existing pattern in this file) with a small change, invokes `FileDiff` with `contextLines=2`, and asserts the returned `[]DiffLine` contains only changed lines + 2 context lines per side (i.e. verifies end-to-end that the `-U2` arg was actually passed to git). Keeps fidelity high — no exec-stub gymnastics needed
- [x] add analogous end-to-end test in `hg_test.go` (follows the existing hg_e2e test pattern)
- [x] add analogous end-to-end test in `jj_test.go`
- [x] run `go test ./app/diff/...` — must pass before Task 3

### Task 3: Add `Compact` and `CompactContext` config fields

**Files:**
- Modify: `app/config.go` (add two fields to options struct)
- Modify: `app/config_test.go` (parse tests)

- [x] add `Compact bool` with long/ini-name/env tags and description matching `--collapsed` precedent
- [x] add `CompactContext int` with long/ini-name/env tags, `default:"5"`, description
- [x] add test `TestParseArgs_Compact` following `TestParseArgs_Collapsed` pattern — verify CLI flag, env var, and config file variants all parse correctly
- [x] add test `TestParseArgs_CompactContext` verifying default is 5 and custom values parse
- [x] verify `TestParseArgs` default-values test (around line 31) includes the new fields with expected defaults
- [x] run `go test ./app -run TestParseArgs` — must pass before Task 4

### Task 4: Add `ActionToggleCompact` to keymap with default binding `C`

**Files:**
- Modify: `app/keymap/keymap.go` (action constant, validActions entry, default binding, help entry)
- Modify: `app/keymap/keymap_test.go`

- [x] add `ActionToggleCompact Action = "toggle_compact"` to action constants
- [x] add `ActionToggleCompact: true` to `validActions` map
- [x] add default binding `"C": ActionToggleCompact` to the defaults map (around line 201)
- [x] add help entry `{ActionToggleCompact, "toggle compact diff view", "View"}` in `defaultDescriptions()` near the other view toggles (after `ActionToggleCollapsed`)
- [x] write test verifying default binding resolves `C` → `ActionToggleCompact` (follow `ActionToggleCollapsed` test precedent at keymap_test.go:580)
- [x] write test verifying `IsValidAction(ActionToggleCompact)` returns true
- [x] write test verifying help entry is present in `defaultDescriptions()`
- [x] run `go test ./app/keymap/...` — must pass before Task 5

### Task 5: Add `compact` mode state, ModelConfig wiring, toggle handler, and re-fetch on toggle

**Files:**
- Modify: `app/ui/model.go` (modeState, ModelConfig, construction — wire Compact/CompactContext/CompactApplicable)
- Modify: `app/ui/diffnav.go` (key action dispatch for `ActionToggleCompact`)
- Modify: `app/ui/loaders.go` (add `currentContextLines()` helper; update `FileDiff` callsites to pass it; add current-file-only re-fetch helper)
- Modify: `app/ui/model_test.go`
- Modify: `app/ui/loaders_test.go`
- Modify: `app/ui/diffnav_test.go` (if exists, otherwise add toggle test to suitable existing file)

- [x] add `compact bool` and `compactContext int` fields to `modeState` struct (grouped with `collapsed`)
- [x] add `Compact bool`, `CompactContext int`, `CompactApplicable bool` fields to `ModelConfig` — mirror the `Collapsed` and `CommitsApplicable` precedent exactly (model.go:482 for doc comment pattern)
- [x] in Model constructor (around model.go:601), copy all three into Model state: `m.modes.compact = cfg.Compact`, `m.modes.compactContext = cfg.CompactContext`, `m.compactApplicable = cfg.CompactApplicable`
- [x] add helper method `(m Model) currentContextLines() int` returning `m.modes.compactContext` if `m.modes.compact && m.compactApplicable`, else `0`
- [x] update every `FileDiff(...)` callsite in `app/ui/loaders.go` to pass `m.currentContextLines()` as the 4th argument (replacing the `0` placeholders added in Task 1). Known sites: the primary `loadFileDiff` body, and `resolveEmptyDiff:293` (staged retry for new files — same context as the primary call, since it's re-fetching the same file)
- [x] add a current-file-only re-fetch helper on Model (e.g. `reloadCurrentFile() tea.Cmd`) — distinct from `triggerReload()` which batches files+commits; this helper only bumps `file.loadSeq` and returns `loadFileDiff(m.file.name)`. Used by the compact toggle so other files and commit log aren't re-fetched unnecessarily
- [x] in `diffnav.go` (verify exact location by grepping for `ActionToggleCollapsed` dispatch), add case for `ActionToggleCompact`:
  - if `!m.compactApplicable`, set transient hint `"compact not applicable"` on status bar and return no-op command
  - else flip `m.modes.compact`, call the new `reloadCurrentFile()` helper, return its cmd. Cursor reset to first hunk happens naturally via existing `skipInitialDividers()` in `handleFileLoaded` after reload completes — verify by reading `handleFileLoaded` flow (loaders.go:213 onwards)
- [x] write test verifying `C` toggles `m.modes.compact` when applicable
- [x] write test verifying `C` is a no-op when `!CompactApplicable` (status hint set, mode unchanged, no re-fetch issued)
- [x] write test verifying `currentContextLines()` returns `compactContext` when compact on AND applicable, `0` when compact off, `0` when compact on but not applicable
- [x] write test verifying compact toggle triggers a current-file re-fetch (observe `file.loadSeq` bump or `loadFileDiff` invocation via mock) and does NOT trigger files-list or commits reload
- [x] write test verifying cursor lands on first hunk after compact-toggle re-fetch completes (simulate `fileLoadedMsg` and assert `m.nav.diffCursor` via `skipInitialDividers` outcome)
- [x] write wiring test verifying `ModelConfig{Compact: true, CompactContext: 10}` at Model construction → `m.modes.compact == true` and `m.modes.compactContext == 10` and `m.currentContextLines() == 10` (when applicable)
- [x] run `go test ./app/ui/...` — must pass before Task 6

### Task 6: Wire `Compact`, `CompactContext`, `CompactApplicable` at the composition root

**Files:**
- Modify: `app/main.go` (add `compactApplicable()` function; wire all three into ModelConfig)
- Modify: `app/renderer_setup.go` (if capability detection needs to peek at the renderer constructed there; otherwise not touched)
- Modify: `app/renderer_setup_test.go` or `app/main_test.go`

- [x] add `compactApplicable(opts options, r diff.Renderer) bool` function following `commitsApplicable()` pattern (main.go:234)
- [x] definition (commit to type-assertion — same approach as `CommitLogger`, no new capability interface):
  - return `false` when `opts.Stdin` (stream-only source, no changes to contextualize)
  - return `false` when `opts.AllFiles` (all-files browse mode reads files as all-context, no diffs)
  - return `false` when `r` (or the innermost renderer it wraps) type-asserts to `*diff.StdinReader`, `*diff.DirectoryReader`, or the pure-FileReader branch of Fallback (follow the existing pattern for unwrapping Include/Exclude/Fallback to reach the underlying VCS — mirror how `setupVCSRenderer` detects VCS capability)
  - return `true` when the underlying source is a VCS type (Git, Hg, Jj), including when wrapped in ExcludeFilter/IncludeFilter/FallbackRenderer
- [x] wire the three config fields into `ModelConfig` construction (main.go around line 157, alongside `Collapsed: opts.Collapsed` at line 163):
  - `Compact: opts.Compact`
  - `CompactContext: opts.CompactContext`
  - `CompactApplicable: compactApplicable(opts, renderer)`
- [x] write tests for `compactApplicable` covering: plain git ref → true, --stdin → false, --all-files → false, `--only path/to/file` without VCS → false (FileReader / pure-fallback path), `--only` in a VCS repo → true (Fallback wrapping VCS reaches a VCS inner), ExcludeFilter wrapping Git → true, IncludeFilter wrapping Git → true, hg → true, jj → true
- [x] run `go test ./app/...` — must pass before Task 7

### Task 7: Add `⊂` glyph to status bar mode icons

**Files:**
- Modify: `app/ui/view.go` (statusModeIcons function at line 330)
- Modify: `app/ui/view_test.go`

- [x] in `statusModeIcons()`, append `⊂` (and a separator matching the existing pattern) when `m.modes.compact` is true
- [x] verify placement: icons should render in a consistent order; place compact near `v`/collapsed since they're related view-mode toggles
- [x] verify graceful degradation on narrow terminals (per CLAUDE.md: "Graceful degradation drops segments on narrow terminals") — follow existing code pattern
- [x] write test `TestModel_StatusModeIconsCompact` following `TestModel_StatusModeIconsWordDiff` pattern (view_test.go:1286): assert glyph present when compact on, absent when off
- [x] write test verifying compact + collapsed together produces both glyphs
- [x] run `go test ./app/ui -run StatusMode` — must pass before Task 8

### Task 8: Documentation sync — README, site, plugin references, ARCHITECTURE

**Files:**
- Modify: `README.md` (Options table, Key Bindings table, Features if relevant)
- Modify: `site/docs.html` (mirror README changes)
- Modify: `site/index.html` (features grid — optional, only if this feature is prominent enough)
- Modify: `.claude-plugin/skills/revdiff/references/config.md` (mirror Options table)
- Modify: `.claude-plugin/skills/revdiff/references/usage.md` (mirror Key Bindings)
- Modify: `plugins/codex/skills/revdiff/references/config.md` (byte-identical except source header)
- Modify: `plugins/codex/skills/revdiff/references/usage.md` (byte-identical except source header)
- Modify: `docs/ARCHITECTURE.md` (add one line about the new `compact` mode on `modeState`, alongside the existing mode documentation)

- [x] add `--compact` and `--compact-context` rows to README.md Options table (near `--collapsed`)
- [x] add `C` row to README.md Key Bindings table (near `v` collapsed toggle) with description "toggle compact diff view (small context around changes)"
- [x] mirror the README changes in `site/docs.html` (find the equivalent tables)
- [x] decide whether to mention in `site/index.html` features grid — skip unless it's prominent; this is a refinement, not a flagship feature
- [x] mirror the Options table changes in `.claude-plugin/skills/revdiff/references/config.md`
- [x] mirror the Key Bindings changes in `.claude-plugin/skills/revdiff/references/usage.md`
- [x] copy the updated `.claude-plugin/` reference files to `plugins/codex/skills/revdiff/references/` (preserving the source-tracking header comment at the top of each file; per CLAUDE.md they must stay byte-identical except the header)
- [x] verify doc sync by diffing the .claude-plugin vs plugins/codex copies — only the source header line should differ
- [x] add one line to `docs/ARCHITECTURE.md` about the `compact` field on `modeState` in the section that documents mode flags (mirror how `collapsed`, `wrap`, etc. are mentioned)
- [x] no tests for this task (documentation only)
- [x] run `make lint` to confirm no markdown/link issues flagged

### Task 9: Verify acceptance criteria

- [x] verify the reporter's scenario end-to-end: open a large file with a one-line change in a git repo, start with `--compact`, confirm the cursor lands on the change and surrounding context is ~5 lines each side
- [x] verify runtime toggle: open without `--compact`, press `C`, confirm file re-fetches and cursor repositions
- [x] verify `--compact-context=10` produces 10 lines of context instead of 5
- [x] verify `C` is a no-op in `--stdin`, `--all-files`, and `--only path/to/file.md` (no VCS) modes, with a status-bar hint shown
- [x] verify compact + collapsed together work (both glyphs in status bar, both behaviors apply)
- [x] verify `--dump-config` includes the new fields
- [x] verify `--dump-keys` lists the new binding
- [x] run full test suite: `go test ./...`
- [x] run race detector: `go test -race ./...`
- [x] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [x] run formatters: `~/.claude/format.sh`
- [x] verify test coverage did not regress: `go test -cover ./...`

### Task 10: [Final] Update project documentation and move plan

- [x] update CLAUDE.md with any new patterns discovered (likely: mention compact mode alongside the existing collapsed/wrap toggle pattern in the project structure section)
- [x] move this plan to `docs/plans/completed/` — `mv docs/plans/20260421-compact-diff-mode.md docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification**:
- test the reporter's original scenario: a multi-thousand-line JSON file with a single one-line change, opened via `revdiff --compact` — confirms the feature addresses the reported friction
- quick smoke test on hg and jj repos (not just git) to confirm context plumbing works for all three VCS backends

**Announce resolution**:
- after this plan is landed in a PR and merged, post a follow-up comment on discussion #134 (https://github.com/umputun/revdiff/discussions/134) linking to the PR and noting the new `--compact` / `C` toggle. Keep it brief per existing project style
