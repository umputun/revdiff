# Rename-aware diffs and rename-origin display (issue #222)

## Overview
When reviewing a renamed file in git modes (`--staged`, single-ref `X`, two-ref `X..Y`), revdiff currently:
1. Discards the rename origin path, so the diff pane header shows only the new filename (no `old → new`).
2. Renders the file as fully added/changed instead of showing only the real line changes.

Root cause (confirmed via reproduction): `(*Git).ChangedFiles` parses git's `R<score> old new` rename pair but keeps only the new path; `(*Git).FileDiff` then runs `git diff [--cached] [ref] -- <newpath>`, and the single-path pathspec strips the old side before git can pair the rename, so the file renders as brand-new.

Fix: preserve the old path on `FileEntry`, thread it through a new `FileDiffRequest` value into `FileDiff`, make `(*Git).FileDiff` pass `-M` + both paths so git emits the rename-aware minimal diff, and render `old → new` in the diff pane header.

## Context (from discovery)
- `app/diff/diff.go` — `FileEntry{Path, Status}` (no old-path field); `(*Git).ChangedFiles` discards the old path at the rename branch; `(*Git).FileDiff(ref, file string, staged bool, contextLines int)` (already 4 params); private helpers `totalOldLines(ref, file, staged)` and `binarySizeDesc(ref, file, staged)` called only from `FileDiff`.
- `FileDiff` is declared in THREE interfaces: `diff.Renderer` (`app/diff/exclude.go:13`), `ui.Renderer` (`app/ui/model.go:43`), `review.FileDiffer` (`app/review/stats.go:26`). Implemented by Git, Hg, Jj, FileReader, DirectoryReader, MultiFileStdinReader, StdinReader, CompareReader; wrapped by ExcludeFilter, IncludeFilter, FallbackRenderer.
- Moq mocks exist for `diff.Renderer` (`app/diff/mocks/Renderer.go`) and `ui.Renderer` (`app/ui/mocks/renderer.go`) — regenerate with `go generate ./...`. `app/review/stats_test.go` uses a hand-written `fakeDiffer` (update its signature in place).
- `sidepane.FileTree` already owns per-file metadata: it stores `fileStatuses map[string]diff.FileStatus` (populated in both `NewFileTree` and `Rebuild`) and exposes `FileStatus(path)` via the `FileTreeComponent` interface (`app/ui/model.go:160`). `FileTreeComponent` is NOT mocked (tests use the real `NewFileTree`), so adding an accessor needs no mock regen. Diff header renders `m.file.name` (`app/ui/view.go:48`); `loadedFileState` (`app/ui/model.go:238`); `loadFileDiff`/`handleFileLoaded`/`fileLoadedMsg`/`resolveEmptyDiff` in `app/ui/loaders.go` (two `FileDiff` callsites: `loadFileDiff` ~98 and `resolveEmptyDiff` ~337).
- `app/review/stats.go` `ComputeStats` loop has `e.OldPath` in hand; `app/annotations_load.go` preloader builds a path→status map from `ChangedFiles`.
- hg `status` reports "R" as *removed* (not renamed) and needs `-C` for copies; jj `ChangedFiles` already decomposes renames into delete+add pairs. Neither reports a rename origin today, so the rename-aware path is git-only. `FileEntry.OldPath` stays VCS-generic but only Git populates it here.
- Precedent for the request-struct shape: `app/review/stats.go` already uses `StatsRequest` for the same "4+ params → option struct" rule.

## Development Approach
- **testing approach**: Regular (code first, then tests) — per user choice.
- complete each task fully before moving to the next.
- make small, focused changes.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task — success and error/edge scenarios, as separate checklist items.
- **CRITICAL: all tests must pass before starting next task** — no exceptions.
- **CRITICAL: update this plan file when scope changes during implementation.**
- run `go test ./... -race` after each change; maintain backward compatibility for non-git modes (no behavior change there).

## Code-Quality Rules (HARD — verify against every task before marking complete)

These rules supplement project CLAUDE.md and are NOT optional. They are the gate for marking any task complete. If a rule is violated, the task is not done — refactor, re-test, then mark complete.

**Signatures (hard limits):**
- No function or method has 4+ parameters. `ctx context.Context` does not count toward the budget. If you need 4+, use an option struct (e.g., `type fooOpts struct { ... }`).
- No function or method has 4+ return values. Split the function into two single-purpose ones, or return a struct.
- Multiple adjacent same-type parameters (`oldLine, newLine int`) are a swap hazard — review whether they belong on a struct.

**Methods vs standalone helpers (project rule, hard):**
- If a function is called only from methods of a single struct, it MUST be a method on that struct. Calling pattern decides, not field access.
- Standalone helpers are reserved for: (a) constructors and entry points (`Parse...`, `New...`, `Decorate...`), (b) utilities shared by multiple unrelated types or by both standalone functions AND methods, (c) tiny cross-cutting helpers.
- Before adding any standalone helper, mentally walk its callers. If every caller is a method of one type, make the helper a method on that type.

**Visibility (private by default, hard):**
- Lowercase identifiers by default. Only export when an out-of-package caller exists.
- Exception (per CLAUDE.md): methods called by other structs in the same package CAN be exported for inter-component API clarity. This is the only exception. It does not extend to types, functions, constants, or variables.
- Before exporting any new identifier, grep for cross-package callers. If none, lowercase it.

**Comments (default: none, hard):**
- Default to writing no comments. Add one only when the WHY is non-obvious (a hidden invariant, a workaround, behavior that would surprise a reader).
- Exported items get godoc comments starting with the name. Unexported items get lowercase non-godoc comments — or no comment at all.
- Never describe WHAT the code does when the code itself is self-evident. Never write multi-paragraph comments on routine helpers.

**Per-task gate (before marking ANY checkbox complete):**
1. Formatter runs clean (`~/.claude/format.sh` or `gofmt -s -w` + `goimports -w`).
2. `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` reports zero issues.
3. `go test ./... -race` passes.
4. Scan the new code for the four rule classes above. Specifically:
   - Grep new function signatures: `grep -nE '^func.*\(.*,.*,.*,.*\)' app/<path>/*.go` — any hit with 4+ comma-separated params (excluding `ctx`) is a violation. Same for the return-value side.
   - For every new standalone helper, `grep -rn 'helperName(' --include='*.go'` and confirm at least one caller is NOT a method of a single type. If all callers are methods of one type, convert.
   - For every new exported identifier, grep cross-package. If no out-of-package hit, lowercase it.
5. Only after 1–4 pass: mark the task complete.

If a previous task shipped a violation (spotted later by user, reviewer, or yourself): fix it in the next commit BEFORE starting the next task. Do not let violations accumulate.

## Testing Strategy
- **unit tests**: required for every task (see Development Approach).
- VCS behavior tests use the existing git test harness in `app/diff/diff_test.go` (`t.TempDir()` + `exec.Command("git", ...)`, see `TestGit_ChangedFiles` ~line 415 and the helper ~line 1095). Rename fixtures use `git mv old new` + an edit + `git add`.
- UI header/loader tests use the moq `ui.Renderer` mock and `loadFileDiff`/`handleFileLoaded` round-trips (see `app/ui/diffnav_test.go`).
- **e2e tests**: project has no Playwright/UI e2e harness; VCS-level integration tests in `app/diff/diff_test.go` are the closest equivalent and stand in for the issue's reproduction.

## Progress Tracking
- mark completed items with `[x]` immediately when done.
- add newly discovered tasks with ➕ prefix; document blockers with ⚠️ prefix.
- keep this plan in sync with actual work done.

## Solution Overview
1. `FileEntry` gains `OldPath` (rename origin, empty for non-renames); `(*Git).ChangedFiles` populates it.
2. A new `FileDiffRequest` value type replaces the four positional `FileDiff` parameters across all three interfaces and every implementation/wrapper (collapses the already-4-param signature to one cohesive value AND carries `OldPath`).
3. `(*Git).FileDiff` becomes rename-aware: when `OldPath` is set, pass `-M` + both paths so git pairs the rename; old-side probes (`totalOldLines`, `binarySizeDesc`) use the old path.
4. The file tree gains an `OldPath(path)` accessor (parallel to `FileStatus(path)`); the UI looks up the selected file's `OldPath` from the tree, threads it into the request, and renders `old → new` in the diff-pane header.
5. Stats and the annotations preloader pass `OldPath` so their per-file diffs match the displayed rename-aware diff.

Non-git impls ignore `OldPath` — no behavior change for hg/jj/file/stdin/compare modes.

## Technical Details

**`FileDiffRequest` (new, `app/diff/diff.go`):**
```go
// FileDiffRequest carries the inputs FileDiff needs to render one file's diff.
// Bundled into a struct because the prior 4-positional shape (ref, file, staged,
// contextLines) hit the project's "4+ params → option struct" rule, and adding
// OldPath for rename-aware diffs would push it to five. OldPath is the rename
// origin (empty for non-renames); only the Git renderer consumes it.
type FileDiffRequest struct {
    Ref          string
    Path         string // new/current path
    OldPath      string // rename origin; empty when not a rename
    Staged       bool
    ContextLines int
}
```

**Interface change (all three):**
```go
FileDiff(req FileDiffRequest) ([]DiffLine, error)   // diff.Renderer, ui.Renderer (with diff. qualifier), review.FileDiffer
```

**Git rename-aware diff:**
```go
func (g *Git) FileDiff(req FileDiffRequest) ([]DiffLine, error) {
    args := g.diffArgs(req.Ref, req.Staged)
    args = append(args, unifiedContextArg(req.ContextLines))
    if req.OldPath != "" && req.OldPath != req.Path {
        args = append(args, "-M", "--", req.OldPath, req.Path)
    } else {
        args = append(args, "--", req.Path)
    }
    // ... totalOldLines/binarySizeDesc now take req and use req.OldPath for old-side lookups
}
```

**Header (`app/ui/view.go`):**
```go
diffTitle := "no file selected"
if m.file.name != "" {
    diffTitle = m.file.name
    if m.file.oldName != "" && m.file.oldName != m.file.name {
        diffTitle = m.file.oldName + " → " + m.file.name
    }
}
```
(still passed through `truncateHeaderTitle` for single-row truncation; arrow glyph is 1 cell, no scrollbar-offset impact.)

## What Goes Where
- **Implementation Steps** (`[ ]`): code, tests, docs in this repo.
- **Post-Completion** (no checkboxes): hg/jj rename support (separate design), manual TUI smoke test of the reproduction.

## Implementation Steps

### Task 1: Add `FileEntry.OldPath` and preserve rename origin in `(*Git).ChangedFiles`

**Files:**
- Modify: `app/diff/diff.go`
- Modify: `app/diff/diff_test.go`

- [x] add `OldPath string` field to `FileEntry` (godoc: rename origin, empty for non-renames)
- [x] in `(*Git).ChangedFiles`, capture the consumed old path into `OldPath` for `R`/`C` entries (currently discarded)
- [x] confirm `FileEntryPaths` and other `FileEntry` consumers are unaffected (additive field)
- [x] write test: staged rename (`git mv` + edit + `git add`) → entry has `Status==FileRenamed`, `Path==new`, `OldPath==old`
- [x] write test: non-rename modify/add/delete entries have empty `OldPath` (regression guard)
- [x] run `go test ./app/diff/... -race` — must pass before next task

### Task 2: Introduce `FileDiffRequest` and migrate the `FileDiff` signature module-wide (mechanical, no behavior change)

**Design Contract:**

Type:
- `FileDiffRequest` (EXPORTED — out-of-package callers construct it: `app/ui/loaders.go`, `app/annotations_load.go`, `app/review/stats.go`; it is part of the `diff.Renderer` interface surface)

Fields (no methods — plain DTO):
- `Ref string`
- `Path string`
- `OldPath string`
- `Staged bool`
- `ContextLines int`

Methods (full signatures): none — `FileDiffRequest` is a pure data carrier.

Standalone helpers planned: none.

Exports (justification per item): `FileDiffRequest` and its fields — referenced by the `diff.Renderer` / `ui.Renderer` / `review.FileDiffer` interface method and constructed in packages `ui`, `main` (annotations_load), and `review`.

**Files:**
- Modify: `app/diff/diff.go` (struct + `(*Git).FileDiff` signature; internal callsites pass req fields)
- Modify: `app/diff/exclude.go` (`diff.Renderer` interface + `ExcludeFilter.FileDiff`)
- Modify: `app/diff/include.go` (`IncludeFilter.FileDiff`)
- Modify: `app/diff/fallback.go` (`FallbackRenderer.FileDiff`, `FileReader.FileDiff`)
- Modify: `app/diff/directory.go`, `app/diff/stdin.go`, `app/diff/multistdin.go`, `app/diff/hg.go`, `app/diff/jj.go`, `app/diff/compare.go` (impl signatures)
- Modify: `app/ui/model.go` (`ui.Renderer` interface), `app/ui/loaders.go` (BOTH callsites: `loadFileDiff` ~98 and `resolveEmptyDiff` ~337 build req, OldPath empty)
- Modify: `app/review/stats.go` (`FileDiffer` interface + both `ComputeStats` callsites ~111/~117, OldPath empty), `app/review/stats_test.go` (`fakeDiffer` signature)
- Modify: `app/annotations_load.go` (callsite ~208 builds req, OldPath empty)
- Modify: mocks via `go generate ./...` (`app/diff/mocks/Renderer.go`, `app/ui/mocks/renderer.go`)
- Modify: test interface-implementers + positional callers — `app/diff/fallback_test.go` (`stubInnerRenderer` and any stub implementing `Renderer`), and all `*_test.go` calling `FileDiff(...)` positionally

- [x] add `FileDiffRequest` struct to `app/diff/diff.go` per the Design Contract
- [x] change `(*Git).FileDiff` to accept `req FileDiffRequest`; pass `req.Ref/req.Path/req.Staged/req.ContextLines` to existing internal calls (no `-M`, OldPath ignored this task)
- [x] update the other diff implementations + the three wrappers to the new signature (delegate `req` straight through)
- [x] update `diff.Renderer`, `ui.Renderer`, `review.FileDiffer` interface declarations
- [x] update every PRODUCTION callsite to construct `FileDiffRequest{...}` with `OldPath: ""`: `loaders.go` `loadFileDiff` AND `resolveEmptyDiff`, `stats.go` both `ComputeStats` calls, `annotations_load.go`
- [x] migrate test interface-implementers (`stubInnerRenderer` in `fallback_test.go`, `fakeDiffer` in `stats_test.go`) to the new method signature — distinct failure mode from positional callers (won't satisfy the interface vs. wrong arg count)
- [x] regenerate mocks (`go generate ./...`); update remaining positional `FileDiff(...)` test calls
- [x] run `go test ./... -race` — full suite must pass with zero behavior change before next task

### Task 3: Make `(*Git).FileDiff` rename-aware

**Files:**
- Modify: `app/diff/diff.go`
- Modify: `app/diff/diff_test.go`

- [x] in `(*Git).FileDiff`, when `req.OldPath != "" && req.OldPath != req.Path`, append `-M -- <OldPath> <Path>`; else keep `-- <Path>`
- [x] make `totalOldLines` rename-aware: take `req` (or old path) and resolve old-side line count from `OldPath` when set (so compact-mode trailing divider is correct for renames); update the direct test caller at `diff_test.go` ~line 1163 to the new shape
- [x] make `binarySizeDesc` pass both paths when `OldPath` is set (renamed binary edge); keep single-path otherwise
- [x] verify these helpers remain methods on `*Git` and stay ≤1 param after the change (pass `req`, not multiple positionals)
- [x] write test: staged rename + edit (`one/two/three` → `two`→`TWO`) → diff is the minimal `-two/+TWO` set, NOT all-added; assert add/remove counts
- [x] write test: two-ref committed rename (`HEAD~1..HEAD`) → same minimal rename-aware diff
- [x] write test: pure rename, no content change → empty/near-empty diff (not all-added); and non-rename file unchanged behavior (regression)
- [x] run `go test ./app/diff/... -race` — must pass before next task

### Task 4: Render `old → new` in the diff-pane header (UI wiring)

**Files:**
- Modify: `app/ui/sidepane/filetree.go` (`oldPaths` map + `OldPath(path)` accessor, populated in `NewFileTree` and `Rebuild`)
- Modify: `app/ui/sidepane/filetree_test.go`
- Modify: `app/ui/model.go` (`FileTreeComponent` interface gains `OldPath(path string) string`; `loadedFileState.oldName`; `fileLoadedMsg.oldName`)
- Modify: `app/ui/loaders.go` (`loadFileDiff` sets `req.OldPath = m.tree.OldPath(file)`; `resolveEmptyDiff` likewise; `handleFileLoaded` sets `m.file.oldName`)
- Modify: `app/ui/view.go` (header title shows `old → new`)
- Modify: `app/ui/loaders_test.go` (or matching `_test.go`), `app/ui/view_test.go`

- [x] in `sidepane.FileTree`, add `oldPaths map[string]string`; populate from `entry.OldPath` in BOTH `NewFileTree` and `Rebuild` (mirror the existing `fileStatuses` handling); add method `(ft *FileTree) OldPath(path string) string` returning `ft.oldPaths[path]`
- [x] add `OldPath(path string) string` to the `FileTreeComponent` interface (`app/ui/model.go:160`); no mock regen (interface is unmocked — tests use real `NewFileTree`)
- [x] add `oldName string` to `loadedFileState` and to `fileLoadedMsg`
- [x] in `loadFileDiff` (and `resolveEmptyDiff`), set `req.OldPath = m.tree.OldPath(file)`; carry `oldName` on `fileLoadedMsg`; in `handleFileLoaded` set `m.file.oldName` (and clear it on the no-file path at loaders.go:213)
- [x] in `view.go`, build `diffTitle` as `oldName + " → " + name` when `oldName` is set and differs from `name`; keep `truncateHeaderTitle`
- [x] write test (`filetree_test.go`): `OldPath` returns the origin for a renamed entry and `""` for non-rename/unknown paths; survives `Rebuild`
- [x] write test (ui): renamed file selected → header string contains `old → new`; non-rename file → header is just the name
- [x] run `go test ./app/ui/... -race` — must pass before next task

### Task 5: Make review stats rename-aware

**Files:**
- Modify: `app/review/stats.go` (`ComputeStats` passes `e.OldPath`)
- Modify: `app/review/stats_test.go`

- [x] in `ComputeStats`, set `OldPath: e.OldPath` on both the primary and the staged-fallback `FileDiff` requests
- [x] write test: renamed+edited entry contributes only the real `+/-` counts, not all-lines-added
- [x] write test: non-rename entries unchanged (regression)
- [x] run `go test ./app/review/... -race` — must pass before next task

### Task 6: Make the annotations preloader rename-aware

**Files:**
- Modify: `app/annotations_load.go` (preloader carries new→old map; `lookupLineSet` passes OldPath)
- Modify: `app/annotations_load_test.go`

- [x] build a `renames map[string]string` (new path → old path) on the preloader from `ChangedFiles` entries (alongside the existing `known` status map)
- [x] in `lookupLineSet`, set `OldPath` on the `FileDiffRequest` from that map so the annotation line-set matches the displayed rename-aware diff
- [x] write test: an annotation on a renamed+edited file maps to the rename-aware diff's line keys (not the all-added line keys)
- [x] run `go test ./... -race` (annotations live in `package main`) — must pass before next task

### Task 7: Verify acceptance criteria
- [x] reproduce the issue's exact scenario as an integration test in `app/diff/diff_test.go` (init repo → `git mv old.txt new.txt` → edit `two`→`TWO` → `git add` → `--staged` flow): `ChangedFiles` yields `OldPath`, `FileDiff` yields minimal rename-aware diff
- [x] verify all Overview requirements (origin visible in header, rename-aware diff) are implemented across `--staged`, single-ref, two-ref
- [x] run full suite: `go test ./... -race`
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — zero issues
- [x] verify coverage did not regress for `app/diff`, `app/ui`, `app/review`

### Task 8: Documentation
- [x] update `README.md` if rename display is user-facing behavior worth a note (and `site/docs.html` in sync if so)
- [x] update `CLAUDE.md` Gotchas: `FileEntry.OldPath` + rename-aware `FileDiff` via `FileDiffRequest`; git-only; header shows `old → new`
- [x] update `docs/ARCHITECTURE.md` if the `FileDiff` request-struct change warrants it
- [x] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification:**
- run `revdiff --staged` against a real staged rename and confirm the header shows `old → new` and the diff shows only real changes.

**Out of scope (separate work):**
- hg rename detection (`hg status -C`) and jj rename-pair UX — neither reports a rename origin today; `FileEntry.OldPath` is left VCS-generic for a future change.

---
Smells pre-check: 0 signature violations; 3 plan defects fixed before save — (1) `oldPathFor`/`m.entries` replaced with a `FileTree.OldPath(path)` tree accessor (the model has no `entries` field), (2) added the missing `resolveEmptyDiff` `FileDiff` callsite to Task 2, (3) called out `stubInnerRenderer` as a test interface-implementer distinct from positional callers.
