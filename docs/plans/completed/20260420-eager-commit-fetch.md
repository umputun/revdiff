# Eager Commit-Info Fetch

## Overview

Replace the lazy commit-info fetch (triggered on first `i` press) with an eager parallel async fetch that runs alongside `loadFiles()` at startup and on `R` reload. This dramatically narrows the window in which the commit overlay can disagree with the displayed diff — from "time until first `i` press" (could be minutes) to "skew between two parallel goroutine starts" (milliseconds).

**Problem** (issue #122): files are loaded eagerly at startup against `HEAD@T0`. Commits are fetched lazily on first `i` press at `T1`. If HEAD advances between T0 and T1 (e.g. during an iterative workflow), the overlay and the diff reflect inconsistent VCS states.

**Fix**: fire `loadCommits()` in parallel with `loadFiles()` via `tea.Batch` in `Init()` and from `triggerReload()`. Both commands run as independent goroutines and land as separate messages. No ordering coupling, no blocking VCS call on the `i` key press, and the practical race window shrinks to the time between two subprocess starts. `R` reload extends the same invariant — both caches get invalidated and re-fetched in parallel. Note: this is not a strict snapshot guarantee — the two subprocesses each resolve `HEAD` independently, so a theoretical race remains. A stricter fix would resolve `HEAD` to a concrete SHA once and pass it to both loaders, but the milliseconds-wide window does not justify the added VCS capability.

**UX change**: previously, the first `i` press blocked for the duration of the VCS call and then opened the overlay. Now, if the user presses `i` before `commitsLoadedMsg` arrives, the status bar shows a transient `loading commits…` hint and the overlay does not open; a second press (after load) opens it. Acceptable for typical startup timing (tens of ms); the loading window only widens on very large repos.

Closes #122.

## Context (from discovery)

- Lazy fetch lives in `app/ui/model.go:611-626` (`ensureCommitsLoaded`), called synchronously from `handleCommitInfo` at `model.go:758`
- Files use an established two-phase async pattern: `loadFiles() tea.Cmd` → `filesLoadedMsg` → `handleFilesLoaded` (see `app/ui/loaders.go:21-67`)
- Stale-result guard for files: `m.filesLoadSeq` bumped before each new load, each `filesLoadedMsg` tagged with its seq, mismatches dropped
- `triggerReload()` (loaders.go:107) currently invalidates `m.commits.loaded = false` / `m.commits.list = nil` and returns `m.loadFiles()`
- `commits.applicable` + `commits.source` are populated at composition root (`main.go`), UI does not derive them
- Tests follow one-file-per-source convention: `loaders.go` ↔ `loaders_test.go`, `model.go` ↔ `model_test.go`

## Development Approach

- **testing approach**: Regular (code first, then tests) — matches project convention; each task writes its own tests before the next task begins
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- run `go test ./...` and `golangci-lint run` after each task
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy

- **unit tests**: required for every task — one test file per source file (project convention)
- mocks via moq — `commits.source` is already an interface (`diff.CommitLogger`), use existing test fixtures in `app/ui/model_test.go`
- no e2e tests needed — this is pure internal async wiring with no user-visible surface change beyond the startup timing

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

Mirror the existing files async pattern for commits:

```
Init() → tea.Batch(loadFiles, loadCommits)
              │              │
              ▼              ▼
        filesLoadedMsg   commitsLoadedMsg
              │              │
              ▼              ▼
      handleFilesLoaded  handleCommitsLoaded
              │              │
              └──────┬───────┘
                     ▼
              m.commits.{list, err, loaded, truncated}
                     ▲
                     │ read
              handleCommitInfo (overlay open)
```

Reload extends naturally: `triggerReload()` bumps both `filesLoadSeq` and `commits.loadSeq`, clears both caches, returns `tea.Batch(loadFiles, loadCommits)`.

Key design decisions:
- **parallel, not sequential** — no ordering dependency between files and commits; each arrives independently
- **same seq-guard pattern as files** — prevents stale commit results from landing after rapid `R` presses
- **delete `ensureCommitsLoaded` entirely** — no hybrid lazy/eager state machine; one pattern only
- **loading-hint path** — if user presses `i` before `commitsLoadedMsg` arrives, show a transient "loading…" hint (mirrors other not-yet-available hints)

## Technical Details

**New types and state:**

```go
// loaders.go
type commitsLoadedMsg struct {
    seq       uint64
    list      []diff.CommitInfo
    err       error
    truncated bool
}

// model.go — inside commitsState
type commitsState struct {
    // ... existing fields ...
    loadSeq uint64  // NEW: bumped on each new commit load; stale messages dropped
}
```

**New command:**

```go
// loaders.go
func (m Model) loadCommits() tea.Cmd {
    if !m.commits.applicable || m.commits.source == nil {
        return nil  // no-op in Batch
    }
    seq := m.commits.loadSeq
    ref := m.cfg.ref
    src := m.commits.source
    return func() tea.Msg {
        list, err := src.CommitLog(ref)
        return commitsLoadedMsg{
            seq:       seq,
            list:      list,
            err:       err,
            truncated: len(list) >= diff.MaxCommits,
        }
    }
}
```

**New handler:**

```go
// loaders.go (next to handleFilesLoaded)
func (m Model) handleCommitsLoaded(msg commitsLoadedMsg) (tea.Model, tea.Cmd) {
    if msg.seq != m.commits.loadSeq {
        return m, nil  // stale, drop
    }
    m.commits.list = msg.list
    m.commits.err = msg.err
    m.commits.truncated = msg.truncated
    m.commits.loaded = true
    return m, nil
}
```

**Modified `triggerReload()` (in loaders.go):**

```go
func (m Model) triggerReload() tea.Cmd {
    m.filesLoadSeq++
    m.file.loadSeq++
    m.commits.loadSeq++       // NEW
    m.commits.loaded = false
    m.commits.list = nil
    // ... existing reset logic ...
    return tea.Batch(m.loadFiles(), m.loadCommits())
}
```

**Modified `Init()` (in model.go):**

```go
func (m Model) Init() tea.Cmd {
    return tea.Batch(m.loadFiles(), m.loadCommits())
}
```

**Modified `Update()` (in model.go):**

```go
case commitsLoadedMsg:
    return m.handleCommitsLoaded(msg)
```

**Simplified `handleCommitInfo()` (in model.go):**

```go
func (m *Model) handleCommitInfo() {
    if !m.commits.applicable || m.commits.source == nil {
        m.commits.hint = "no commits in this mode"
        return
    }
    if !m.commits.loaded {
        m.commits.hint = "loading commits…"
        return
    }
    m.overlay.OpenCommitInfo(overlay.CommitInfoSpec{
        Commits:    m.commits.list,
        Applicable: true,
        Truncated:  m.commits.truncated,
        Err:        m.commits.err,
    })
}
```

## What Goes Where

- **Implementation Steps**: all code and tests within the revdiff repo
- **Post-Completion**: close issue #122 with reference to the merge commit

## Implementation Steps

### Task 1: Add `loadCommits()` cmd and `commitsLoadedMsg` type

**Files:**
- Modify: `app/ui/loaders.go`
- Modify: `app/ui/model.go`
- Modify: `app/ui/loaders_test.go`

- [x] add `commitsLoadedMsg` struct to `app/ui/model.go` (next to `filesLoadedMsg` at line 393)
- [x] add `loadSeq uint64` field to `commitsState` struct in `app/ui/model.go`
- [x] add `loadCommits() tea.Cmd` method in `app/ui/loaders.go` — returns nil when `!applicable` or `source == nil`; captures seq + ref + source at invocation time; calls `CommitLog(ref)` inside the closure
- [x] add godoc on `loadCommits()` mirroring `loadFiles()` (loaders.go:16-20): explain the seq-tag contract — caller must bump `m.commits.loadSeq` before invoking when issuing a re-fetch
- [x] write test `TestModel_LoadCommits_ReturnsNilWhenNotApplicable` — asserts nil cmd when `applicable=false`
- [x] write test `TestModel_LoadCommits_ReturnsNilWhenSourceIsNil` — asserts nil cmd when `source=nil`
- [x] write test `TestModel_LoadCommits_ReturnsCmdWhenApplicable` — asserts non-nil cmd returns correct `commitsLoadedMsg`
- [x] write test `TestModel_LoadCommits_PropagatesError` — fake source returns error, asserts `msg.err` populated
- [x] run `go test ./app/ui/...` — must pass before task 2

### Task 2: Add `handleCommitsLoaded` and wire into `Update()`

**Files:**
- Modify: `app/ui/loaders.go`
- Modify: `app/ui/model.go`
- Modify: `app/ui/loaders_test.go`

- [x] add `handleCommitsLoaded(msg commitsLoadedMsg)` method in `app/ui/loaders.go` next to `handleFilesLoaded`
- [x] implement seq-guard: return `(m, nil)` when `msg.seq != m.commits.loadSeq`
- [x] populate `m.commits.list`, `.err`, `.truncated`, `.loaded = true` on seq match
- [x] add `case commitsLoadedMsg: return m.handleCommitsLoaded(msg)` in `Update()` switch (`app/ui/model.go:629`)
- [x] write test `TestModel_HandleCommitsLoaded_PopulatesState` — assert all four fields set
- [x] write test `TestModel_HandleCommitsLoaded_DropsStaleResult` — bump `loadSeq` after msg constructed, assert fields NOT populated
- [x] write test `TestModel_HandleCommitsLoaded_SetsLoadedOnError` — err message still marks `loaded=true` (caches the error, same as files)
- [x] run `go test ./app/ui/...` — must pass before task 3

### Task 3: Wire `loadCommits` into `Init()` and `triggerReload()`

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/loaders.go`
- Modify: `app/ui/loaders_test.go`

**Receiver note**: `triggerReload()` stays `func (m *Model)` — the seq bump and cache resets must mutate the live Model, not a copy. `Init()` stays value-receiver.

- [x] change `Init()` in `app/ui/model.go` to `return tea.Batch(m.loadFiles(), m.loadCommits())`
- [x] in `triggerReload()` (`app/ui/loaders.go`), add `m.commits.loadSeq++` alongside the existing `m.filesLoadSeq++` / `m.file.loadSeq++` bumps
- [x] **keep** the existing `m.commits.loaded = false` and `m.commits.list = nil` resets — the seq guard only catches stale results; the resets are what ensure the overlay shows the loading hint (not stale data) while the new fetch is in flight
- [x] change `triggerReload()` return from `m.loadFiles()` to `tea.Batch(m.loadFiles(), m.loadCommits())`
- [x] write test `TestModel_TriggerReload_RefetchesCommits` — seed `commits.loaded=true, commits.list=[...]`, call `triggerReload()`, assert (a) `commits.loaded` flipped to false, (b) `commits.list` is nil, (c) `commits.loadSeq` incremented, (d) returned cmd is non-nil batch
- [x] run `go test ./app/ui/...` — must pass before task 4

### Task 4: Delete `ensureCommitsLoaded` and simplify `handleCommitInfo`

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/model_test.go`
- Modify: `app/ui/handlers_test.go`
- Modify: `app/ui/loaders_test.go`

- [x] delete `ensureCommitsLoaded()` method at `app/ui/model.go:611-626` entirely
- [x] remove `m.ensureCommitsLoaded()` call inside `handleCommitInfo` (`app/ui/model.go:758`)
- [x] add `if !m.commits.loaded { m.commits.hint = "loading commits…"; return }` guard in `handleCommitInfo` before the overlay open
- [x] delete `TestModel_ensureCommitsLoaded_*` subtests in `app/ui/model_test.go` (6 subtests, the block starting around line 762)
- [x] delete `TestModel_TriggerReload_InvalidatesCommitCache` in `app/ui/loaders_test.go` — superseded by `TestModel_TriggerReload_RefetchesCommits`
- [x] delete `TestModel_HandleCommitInfo_CachesBetweenOpens` in `app/ui/handlers_test.go` — cache-on-press semantics no longer exist
- [x] update `TestModel_HandleCommitInfo_OpensOverlayWhenApplicable` — seed `m.commits.loaded=true` and `m.commits.list=[...]` directly; drop any assertion that the first press triggered the fetch
- [x] update `TestModel_HandleCommitInfo_StoresErrorInSpec` — seed `m.commits.loaded=true, m.commits.err=<someErr>` directly instead of relying on fetch-on-press
- [x] update `TestModel_HandleCommitInfo_TruncatedFlagPropagates` — seed `m.commits.loaded=true, m.commits.truncated=true` directly
- [x] verify `TestModel_HandleCommitInfo_HintWhenNotApplicable`, `TestModel_HandleCommitInfo_HintWhenSourceNil`, `TestModel_HandleCommitInfo_HintClearsOnNextKey`, `TestModel_HandleCommitInfo_StatusBarShowsHint` still pass without changes — they do not depend on fetch-on-press
- [x] write test `TestModel_HandleCommitInfo_ShowsLoadingHintBeforeLoad` — seed `m.commits.applicable=true, m.commits.source=<fake>, m.commits.loaded=false`, call `handleCommitInfo`, assert hint set to `loading commits…` and overlay NOT opened
- [x] run `go test ./app/ui/...` — must pass before task 5

### Task 5: Full validation and acceptance check

**Files:** none

- [x] run `go test ./...` from repo root — all packages must pass
- [x] run `go test -race ./...` — no races
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — zero issues
- [x] run `make build` — binary builds clean (output at `./.bin/revdiff`)
- [x] manual smoke test: launch `./.bin/revdiff HEAD~3..HEAD`, press `i` immediately (before overlay has data) → loading hint shown; wait → press `i` again → overlay with commits; press `R` → overlay closes, press `i` → overlay shows refreshed commits
- [x] grep sweep: `rg 'ensureCommitsLoaded' app/` must return zero matches
- [x] verify `handleCommitInfo` no longer performs any fetch — only reads cached state
- [x] verify `Init()` and `triggerReload()` both return `tea.Batch` containing `loadCommits`
- [x] verify #122 scenario is fixed: files and commits fetched against same HEAD snapshot on startup; both re-fetched together on `R`

### Task 6: [Final] Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/ARCHITECTURE.md`

- [x] update `CLAUDE.md` Gotchas section: change the "Data is fetched lazily on first press and cached for the session" sentence in the commit-info overlay bullet to reflect eager parallel fetch from Init/triggerReload, with seq-guard
- [x] update `docs/ARCHITECTURE.md` line 322 reference to `ensureCommitsLoaded()` — replace with description of `loadCommits()` / `handleCommitsLoaded` pair
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Issue #122 closure:**
- close #122 with a comment linking to the merge commit
- note that the fix is eager parallel fetch at startup + re-fetch on `R`, consistent with the files load pattern

**Manual verification (smoke test covered in Task 5):**
- verify the `loading commits…` hint fires correctly on fast `i` press during startup on a large repo where `CommitLog` takes noticeable time
