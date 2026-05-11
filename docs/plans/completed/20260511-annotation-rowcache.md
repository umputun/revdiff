# Annotation Visual-Row Chokepoint + Cache

## Overview
Extract `Model.annotationVisualRows(prefix, body) []string` as the single source of truth for "how many rows + what content does this annotation paint as." Today the height query (`wrappedAnnotationLineCount`, `app/ui/annotate.go:385`) and the painter (`renderWrappedAnnotation`, `app/ui/diffview.go:680`) each fork the wrap-and-style work. They currently agree, but only because the test suite happens to enforce it — the CLAUDE.md "Annotation visual-row invariant" gotcha exists precisely because of this risk. Memoize results on `annot.rowCache` keyed by `(body, prefix, width)`. Invalidate on file load and theme apply; width self-invalidates via cache key.

Pure structural refactor. No UX change, no new flag, no new dependency, no visual difference in rendered annotations. After this PR, the height-vs-paint invariant is self-enforced by the type system (one method, one cache) instead of being defended by docs and a test suite.

Performance side-effect: the cache turns repeat `wrappedAnnotationLineCount` and `renderWrappedAnnotation` calls into O(1) map lookups. Composes with the wheel-coalescing work shipped in v1.2.0 (#180) but is not a user-perceptible win on its own.

Background: extracted from PR #174 (rlch, closed). The original PR bundled this with glamour markdown rendering which was declined; the chokepoint+cache stood on its own merits. Textarea swap (B) was considered and dropped — `Ctrl+E → $EDITOR` already covers multi-line editing.

## Context (from discovery)
- `app/ui/annotate.go:385-420` — current `wrappedAnnotationLineCount` (forked wrap walk #1)
- `app/ui/diffview.go:680-708` — current `renderWrappedAnnotation` (forked wrap walk #2)
- `app/ui/diffview.go:328, 667` — two `renderWrappedAnnotation` call sites (file-level + line-level)
- `app/ui/model.go` — `annotationState` struct, needs `rowCache` field
- `app/ui/loaders.go:256` — `handleFileLoaded` (one invalidation hook)
- `app/ui/themeselect.go:93` — `applyTheme` (other invalidation hook)
- `CLAUDE.md` — "Annotation visual-row invariant" gotcha note (update to reflect new self-enforced invariant)

Patterns observed:
- The project already uses per-source-file test files (`annotate_test.go`, `diffview_test.go`). New tests go in the matching file, not a separate `annotate_markdown_test.go` like the PR did.
- Cache fields on substate structs are common pattern (`m.annot.*`, `m.commits.*`, `m.wheel.*`).
- Invalidation helpers are unexported methods on `*Model` (`clearPendingInputState`, etc.).

## Development Approach
- **testing approach**: Regular — preserve existing tests as the behavior safety net, refactor production code, then add new invariant + cache + snapshot tests
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional — they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - tests cover both success and edge scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: byte-identical annotation rendering vs current master** — this is the success criterion of the refactor; pin it with a snapshot test
- run tests after each change
- maintain backward compatibility (this is a refactor, callers and output stay the same)

## Testing Strategy
- **unit tests**: required for every task
  - existing `wrappedAnnotationLineCount` tests in `annotate_test.go` must pass unchanged (this is the behavior anchor)
  - existing `renderWrappedAnnotation` tests in `diffview_test.go` must pass unchanged
  - new tests for `annotationVisualRows`: empty body, single-line body, multi-line body, wide vs narrow pane, prefix width variations
  - new invariant test: for a representative set of inputs, `len(annotationVisualRows(p, b)) == wrappedAnnotationLineCount(key)` AND the rendered rows from the painter match the chokepoint output byte-for-byte
  - new cache tests: repeat call with same key returns same slice (cache hit), different width creates new entry, `invalidateAnnotationRows` clears the cache
  - new invalidation tests: cache empty after `handleFileLoaded`, cache empty after `applyTheme`
- **snapshot test**: capture current master's rendered annotation output for a representative input set (single-line, multi-line, wrap-needed, file-level, narrow pane), assert byte-equivalence after refactor. Living in `annotate_test.go`.
- **e2e tests**: project has no UI-based e2e suite, so unit tests are the full safety net here.

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview
Single chokepoint method on `Model`:

```go
type annotCacheKey struct {
    body, prefix string
    width        int
}

func (m Model) annotationVisualRows(prefix, body string) []string {
    if body == "" {
        return nil
    }
    wrapW := m.diffContentWidth() - 1
    if wrapW < wrapMinContent {
        wrapW = wrapMinContent
    }
    key := annotCacheKey{body: body, prefix: prefix, width: wrapW}
    if rows, ok := m.annot.rowCache[key]; ok {
        return rows
    }
    rows := m.composeAnnotationRows(prefix, body, wrapW)
    m.annot.rowCache[key] = rows
    return rows
}
```

`composeAnnotationRows` is the wrap-and-style logic factored out from today's `renderWrappedAnnotation`: split on `\n`, prefix on row 0, plain-space indent on rows 1+, wrap each segment at `wrapW`, wrap each visual row in `AnnotationInline`. Returns fully styled `[]string`.

`wrappedAnnotationLineCount` collapses to: resolve `(prefix, body)` via `annotationPrefixBody(key)`, call chokepoint, return `len(rows)`. `renderWrappedAnnotation` collapses to: call chokepoint, iterate rows, prepend cursor cell on row 0, call `extendLineBg`. Painter does no wrap logic anymore.

Cache key uses the full `wrapW = diffContentWidth() - 1` (the actual wrap width applied to each segment), not the body-only width. Same wrap width regardless of prefix; prefix affects content of row 0 only. Width changes self-invalidate.

## Technical Details
- **Data structure**: `rowCache map[annotCacheKey][]string` on `annotationState` in `app/ui/model.go`
- **Cache key**: `annotCacheKey{body string, prefix string, width int}` — three-field comparable struct, native map key
- **Unbounded**: cache size grows until next invalidation. Memory ceiling per session is small (`unique_bodies × widths_seen` ≈ tens of entries, kB each). LRU cap is a trivial follow-up if it ever matters; not needed v0.
- **Invalidation**: explicit `invalidateAnnotationRows()` helper called from `handleFileLoaded` (annotation set may change per file) and `applyTheme` (styling colors baked into rows change). Width changes do NOT need explicit invalidation — they self-invalidate via the cache key.
- **Concurrency**: cache lives on Model, accessed only from the Bubble Tea Update/View cycle (single goroutine). No locking needed.
- **What's baked into cached rows**: prefix bytes (row 0) or matching-width plain-space indent (row 1+), `AnnotationInline` styling envelope from `m.renderer`, AND the `DiffPaneBg` color applied via `extendLineBg` lives in the painter (NOT the chokepoint) — so `paneBg` mutations would NOT desync cached rows on their own. What DOES bake in is the resolver's `AnnotationInline` colors, which `applyTheme` rebuilds; that's why the `applyTheme` invalidation hook is load-bearing. `noColors` is set at construction in `NewModel` and never mutated at runtime — does not need cache participation. Future contributors adding a runtime bg or color toggle MUST also invalidate the cache.
- **Signature change**: `renderWrappedAnnotation(b, cursor, text)` becomes `renderWrappedAnnotation(b, cursor, prefix, body)`. Both call sites (`diffview.go:328` file-level, `diffview.go:667` line-level) updated.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): all code, tests, doc updates inside this repo
- **Post-Completion**: none — no external systems, no consumer projects, no deployment changes

## Implementation Steps

### Task 1: Add cache infrastructure and chokepoint method

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/annotate.go`
- Modify: `app/ui/annotate_test.go`

- [x] add `annotCacheKey` struct (private, in `annotate.go`) with `body, prefix string; width int` fields
- [x] add `rowCache map[annotCacheKey][]string` to `annotationState` in `model.go`
- [x] initialize `rowCache: make(map[annotCacheKey][]string)` in `NewModel`
- [x] add `annotationPrefixBody(key string) (prefix, body string)` helper on `Model` in `annotate.go` — extracts the duplicated lookup loop currently inside `wrappedAnnotationLineCount`
- [x] add `annotationVisualRows(prefix, body string) []string` method on `Model` in `annotate.go` — returns cached slice or computes fresh via `composeAnnotationRows`
- [x] add `composeAnnotationRows(prefix, body string, wrapW int) []string` method (unexported, on `Model`) — wrap+style logic copied verbatim from today's `renderWrappedAnnotation` (split on `\n`, indent continuation, wrap each segment, wrap in `AnnotationInline`). Godoc on this method MUST note: rows bake in `AnnotationInline` resolver styling — `applyTheme` invalidation is load-bearing; anyone adding a runtime color toggle must also invalidate.
- [x] add `invalidateAnnotationRows()` helper on `*Model` — clears `rowCache` via `for k := range; delete`
- [x] write tests in `annotate_test.go`: `annotationVisualRows` empty body returns nil, single-line returns one row, multi-line splits on \n, wrap-needed produces multiple rows, prefix affects row-0 content
- [x] write tests in `annotate_test.go`: cache hit on repeat call returns same slice, different width creates new entry, `invalidateAnnotationRows` empties the cache
- [x] write tests in `annotate_test.go`: `annotationPrefixBody` returns correct prefix for file-level vs line-level, returns ("", "") when no annotation matches
- [x] run `make test` — all tests must pass before next task

### Task 2: Refactor wrappedAnnotationLineCount through the chokepoint

**Files:**
- Modify: `app/ui/annotate.go`
- Modify: `app/ui/annotate_test.go`

- [x] reshape `wrappedAnnotationLineCount(key string) int` to: resolve `(prefix, body)` via `annotationPrefixBody`, call `annotationVisualRows`, return `len(rows)` clamped to min 1
- [x] remove the now-dead inline wrap walk (lines ~395-419 of current `annotate.go`)
- [x] verify existing `wrappedAnnotationLineCount` tests in `annotate_test.go` still pass unchanged (behavior preserved)
- [x] add invariant test: for a representative set of (key, width) pairs, `wrappedAnnotationLineCount(key) == len(annotationVisualRows(prefix, body))`
- [x] run `make test` — all tests must pass before next task

### Task 3: Refactor renderWrappedAnnotation through the chokepoint

**Files:**
- Modify: `app/ui/diffview.go`
- Modify: `app/ui/diffview_test.go`

- [x] change `renderWrappedAnnotation` signature from `(b, cursor, text)` to `(b, cursor, prefix, body)`
- [x] reshape body to: call `annotationVisualRows(prefix, body)`, iterate rows, prepend cursor cell on row 0, call `extendLineBg(c+row, paneBg)`. Painter has no wrap logic.
- [x] update call site at `diffview.go:328` (file-level): change `m.renderWrappedAnnotation(b, cursor, "\U0001f4ac file: "+fileComment)` to `m.renderWrappedAnnotation(b, cursor, "\U0001f4ac file: ", fileComment)`
- [x] update call site at `diffview.go:667` (line-level): change `m.renderWrappedAnnotation(b, cursor, "\U0001f4ac "+comment)` to `m.renderWrappedAnnotation(b, cursor, "\U0001f4ac ", comment)`
- [x] update any `renderWrappedAnnotation` tests in `diffview_test.go` to use the new signature
- [x] verify existing rendered-output assertions still pass byte-for-byte (the success criterion)
- [x] run `make test` — all tests must pass before next task

### Task 4: Wire invalidation on file load and theme apply

**Files:**
- Modify: `app/ui/loaders.go`
- Modify: `app/ui/themeselect.go`
- Modify: `app/ui/annotate_test.go`

- [x] add `m.invalidateAnnotationRows()` call inside `handleFileLoaded` (`loaders.go:256`) after the file state is set
- [x] add `m.invalidateAnnotationRows()` call inside `applyTheme` (`themeselect.go:93`) after the theme is applied
- [x] write test: pre-populate cache, call `handleFileLoaded` with a fileLoadedMsg, assert cache is empty
- [x] write test: pre-populate cache, call `applyTheme` with a ThemeSpec, assert cache is empty
- [x] run `make test` — all tests must pass before next task

### Task 5: Snapshot test for byte-equivalent rendering

**Files:**
- Modify: `app/ui/annotate_test.go`

- [x] **before Task 1 begins** (or by checking out `master` in a worktree mid-task if Task 1 already ran), capture rendered annotation output for a fixed input matrix from the **pre-refactor master code path** — never regenerate the expected bytes by running the refactored chokepoint, that would make the test self-confirming and defeat the regression check
- [x] input matrix MUST include: single-line body, multi-line body (embedded `\n`), wrap-needed body in narrow pane, multi-segment wrap on a continuation line (`\n` followed by a long segment that itself wraps — this is the path where indent affects both wrap threshold and `lipgloss.Width`, the most likely drift point), file-level prefix (`💬 file: `), line-level prefix (`💬 `)
- [x] paste the captured raw bytes (or hex-escape if cleaner) into the test as expected strings; this is a one-time freeze
- [x] run `annotationVisualRows` for each, assert the joined output matches the expected captured strings byte-for-byte
- [x] this is the regression safety net for the whole refactor — if it passes, no visual change happened
- [x] run `make test` — all tests must pass before next task

### Task 6: Verify acceptance criteria
- [x] verify chokepoint exists and is used by both height query and painter (grep for direct wrap walks in `annotate.go`/`diffview.go` should find none outside the chokepoint)
- [x] verify cache is wired (rowCache field, NewModel init, invalidate calls in handleFileLoaded and applyTheme)
- [x] run full test suite: `make test`
- [x] run linter: `make lint`
- [x] manual visual check (skipped - not automatable, snapshot test in Task 5 provides byte-equivalent guarantee)

### Task 7: [Final] Update documentation
- [x] update CLAUDE.md "Annotation visual-row invariant" gotcha to reflect that the invariant is now self-enforced (single chokepoint), with a brief note on the cache and its invalidation policy
- [x] move this plan to `docs/plans/completed/` (run `mkdir -p docs/plans/completed && mv docs/plans/20260511-annotation-rowcache.md docs/plans/completed/`)

## Post-Completion
*No external systems involved — pure code refactor inside this repo. No consuming projects, no deployment changes, no manual verification beyond the unit-test + visual check covered in Task 6.*
