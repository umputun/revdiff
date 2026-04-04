# No-Git File Review Mode

## Overview
When `--only` is given and the specified file isn't in any git diff (or no git repo exists), revdiff falls back to showing all lines as context (no +/- gutter) with full annotation support. This enables reviewing arbitrary files without git context — the primary use case is plan review hooks in cc-thingz, where the plan text is written to a temp file and needs structured annotation via revdiff.

## Context
- `ui.Renderer` interface: `ChangedFiles(ref, staged)` and `FileDiff(ref, file, staged)` — defined in `ui/model.go:22-26`, consumer-side (stays here)
- `diff.Git` is the only `Renderer` implementation, always requires a git repo
- `filterOnly` in `ui/model.go:445-458` filters `ChangedFiles` result using suffix matching (`f == pattern || strings.HasSuffix(f, "/"+pattern)`); if no match → "no files match" error
- `annotation.Store` works fine with `ChangeContext` lines — `Type: " "`, keyed by `NewNum`
- `highlight.HighlightLines` works fine with all-context lines — no changes needed
- `cmd/revdiff/main.go:run()` calls `gitTopLevel()` first, fails immediately outside a git repo

## Solution Overview
Two new types in `diff/`:
1. **`FallbackRenderer`** — wraps `*diff.Git`, knows `--only` file paths. Delegates to inner renderer, falls back to disk read for `--only` files not in the git diff.
2. **`FileReader`** — standalone `Renderer` for no-git usage. Reads `--only` files directly from disk. Used when `gitTopLevel()` fails.

Both types satisfy `ui.Renderer` implicitly via duck typing. The `Renderer` interface stays in `ui/model.go` (consumer-side, per Go convention). `FallbackRenderer.inner` is typed as `*diff.Git` since that's the only concrete inner renderer and both live in the same package.

In `run()`: if git is available, wrap `diff.NewGit` with `FallbackRenderer`. If git is unavailable and `--only` is set, use `FileReader` directly. If git is unavailable and `--only` is NOT set, error as before. The renderer-selection logic is extracted into a testable `makeRenderer()` function.

A shared helper `readFileAsContext(path)` reads a file from disk and returns `[]DiffLine` with `ChangeContext` for every line.

## Technical Details
- `readFileAsContext(path string) ([]DiffLine, error)` — reads file, splits by newline, returns `DiffLine{OldNum: i+1, NewNum: i+1, Content: line, ChangeType: ChangeContext}` for each line
- `FallbackRenderer` stores `inner *diff.Git`, `only []string`, `workDir string`. `ChangedFiles` calls inner, then appends `--only` files not already present (using the same suffix-matching logic as `filterOnly`: exact match or `HasSuffix(f, "/"+pattern)`) if they exist on disk resolved against workDir. `FileDiff` calls inner first; if empty result for an `--only` file, falls back to `readFileAsContext`
- `FileReader` stores `files []string` and `workDir string`. `ChangedFiles` returns the file list (resolved against workDir, only those that exist on disk). `FileDiff` calls `readFileAsContext`
- **Path resolution**: relative `--only` values are resolved against workDir (git root or cwd). Absolute `--only` paths are used as-is. For no-git `FileReader`, files appear with their resolved path in the tree. For git `FallbackRenderer`, fallback files appear with their workDir-relative path when possible

## Development Approach
- **testing approach**: TDD — write tests first for new types
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy
- **unit tests**: `diff/fallback_test.go` — test `readFileAsContext`, `FallbackRenderer`, and `FileReader` (all in one file since they share context)
- **unit tests**: `cmd/revdiff/main_test.go` — test `makeRenderer()` for the three decision branches
- **integration**: manual test with `revdiff --only /tmp/somefile.md` outside a git repo

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add `readFileAsContext` helper in `diff/fallback.go`

**Files:**
- Create: `diff/fallback.go`
- Create: `diff/fallback_test.go`

- [x] add `readFileAsContext(path string) ([]DiffLine, error)` function to `diff/fallback.go`
- [x] write tests in `diff/fallback_test.go` for `readFileAsContext` — normal file, empty file, nonexistent file
- [x] run `go test ./...` — must pass before next task

### Task 2: Add `FallbackRenderer` wrapper

**Files:**
- Modify: `diff/fallback.go`
- Modify: `diff/fallback_test.go`

- [x] create `FallbackRenderer` struct with `inner *diff.Git`, `only []string`, `workDir string`
- [x] implement `ChangedFiles` — call inner, then for each `--only` pattern not matching any returned file (using suffix logic: `f == pattern || strings.HasSuffix(f, "/"+pattern)`), resolve against workDir and append if it exists on disk
- [x] implement `FileDiff` — call inner; if empty result and file is in `only` list, call `readFileAsContext` with the resolved path
- [x] add `NewFallbackRenderer(inner *diff.Git, only []string, workDir string) *FallbackRenderer` constructor
- [x] write tests for `FallbackRenderer.ChangedFiles` — file in diff, file not in diff but on disk, file not on disk, suffix match deduplication (e.g. `--only plan.md` when diff has `docs/plans/plan.md`)
- [x] write tests for `FallbackRenderer.FileDiff` — file has diff (use inner), file has no diff but on disk (fallback), file not on disk
- [x] run `go test ./...` — must pass before next task

### Task 3: Add `FileReader` standalone renderer

**Files:**
- Modify: `diff/fallback.go`
- Modify: `diff/fallback_test.go`

- [x] create `FileReader` struct with `files []string`, `workDir string`
- [x] implement `ChangedFiles` — resolve files against workDir, return only those that exist on disk
- [x] implement `FileDiff` — call `readFileAsContext` for the resolved file path
- [x] add `NewFileReader(files []string, workDir string) *FileReader` constructor
- [x] write tests for `FileReader.ChangedFiles` and `FileReader.FileDiff`
- [x] run `go test ./...` — must pass before next task

### Task 4: Wire up in `run()` with extracted `makeRenderer()`

**Files:**
- Modify: `cmd/revdiff/main.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] extract renderer-selection logic into `makeRenderer(only []string, gitRoot string, gitErr error) (ui.Renderer, error)` — simplified to two returns since workDir is internal to the renderer. Three branches: git available → FallbackRenderer, no-git+only → FileReader with cwd, no-git+no-only → error
- [x] refactor `run()` to use `makeRenderer()` instead of inline git check
- [x] write tests in `cmd/revdiff/main_test.go` for `makeRenderer` — all four cases (git+only, git+no-only, no-git+only, no-git+no-only)
- [x] run `go test ./...` — must pass before next task

### Task 5: Verify acceptance criteria
- [x] verify `revdiff --only existing-file.md` works inside a git repo when file has no changes (context-only view)
- [x] verify `revdiff --only /tmp/somefile.md` works outside a git repo (context-only view)
- [x] verify annotations work on context-only lines and output correctly via `--output`
- [x] verify syntax highlighting works on context-only files
- [x] verify normal git diff mode is unaffected (no regressions)
- [x] run full test suite: `go test ./...`
- [x] run linter: `golangci-lint run`

### Task 6: [Final] Update documentation
- [x] update README.md with new fallback behavior description
- [x] update CLAUDE.md if needed
- [x] update `.claude-plugin/skills/revdiff/references/usage.md` with context-only usage example
- [x] run `go test ./...` and `golangci-lint run` — final check
- [x] move this plan to `docs/plans/completed/`

## Post-Completion
**cc-thingz integration:**
- replace `plan-annotate.py` hook with a new script that uses `revdiff --only /tmp/plan.md --output /tmp/annotations.txt`
- shadow git repo approach for iteration 2+ (revdiff shows real diff of Claude's revisions)
- this is a separate task in cc-thingz repo, not part of this plan
