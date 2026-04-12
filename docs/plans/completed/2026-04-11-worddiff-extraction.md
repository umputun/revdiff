# app/ui/worddiff/ Sub-Package Extraction

## Overview

Extract a new `app/ui/worddiff/` sub-package from `app/ui/` that owns **all intra-line word-diff algorithms and the shared text-range highlight insertion engine**. This includes the tokenizer, LCS algorithm, line pairing, similarity gate, and the ANSI-aware highlight marker insertion function used by both word-diff and search highlighting.

This is a **follow-up** to the completed `app/ui/style/` extraction (2026-04-10) and `app/ui/sidepane/` extraction (2026-04-11). It inherits the Design Philosophy section from the style plan wholesale, but is **simpler** than both predecessors because:

- The code is **pure algorithmic** — no stateful component, no consumer-side interfaces, no factory injection, no enums.
- The extraction boundary is clean — almost all functions are Model-independent today. Only `pairHunkLines`/`greedyPairLines` read `m.diffLines`, and `recomputeIntraRanges` (which stays in `ui`) is the only wiring method.
- No new dependencies needed.

**Problem it solves:**

- `app/ui/worddiff.go` (334 LOC) contains pure algorithmic code (tokenizer, LCS, range builder, similarity gate) that has zero coupling to the TUI model — yet it lives in the `ui` package alongside 20+ files of bubbletea code. Extraction makes the algorithmic boundary explicit.
- `matchRange` type in `app/ui/diffview.go:17` is shared between word-diff and search highlighting but defined as a package-private type in a rendering file. Moving it to `worddiff.Range` gives it a proper home and makes both consumers import the same named type.
- `insertHighlightMarkers` in `app/ui/diffview.go:537` is a Model method with an unused receiver — it's pure string/ANSI manipulation. Extracting it removes a fake Model dependency and makes it testable in isolation.
- `pairHunkLines`/`greedyPairLines` are Model methods that only read `m.diffLines[i].Content` and `m.diffLines[i].ChangeType` — they can be refactored to accept data directly, eliminating the Model coupling.

## Context (from discovery)

**Current state of affected code** (on master):

- `app/ui/worddiff.go` — 334 LOC. Defines `intralineToken`, `intralinePair`, `tokenPattern`, `tokenizeLineWithOffsets`, `lcsKeptTokens`, `isWhitespaceToken`, `buildChangedRanges`, `changedTokenRanges`, `maxLineLenForDiff`, `pairHunkLines` (Model method), `greedyPairLines` (Model method), `commonPrefixLen`, `commonSuffixLen`, `similarityThreshold`, `recomputeIntraRanges` (Model method), `passesSimilarityGateFromKeep`, `countNonWhitespace`.
- `app/ui/worddiff_test.go` — 608 LOC. Comprehensive coverage of all functions.
- `app/ui/diffview.go:17` — `type matchRange struct{ start, end int }`. Used by word-diff (`buildChangedRanges`, `changedTokenRanges`, `applyIntraLineHighlight`) and search (`highlightSearchMatches`).
- `app/ui/diffview.go:537-614` — `insertHighlightMarkers` (Model method, receiver unused) + `updateRestoreBg` (Model method, receiver unused). Pure ANSI text manipulation called by both `applyIntraLineHighlight` and `highlightSearchMatches`.
- `app/ui/model.go:177` — `intraRanges [][]matchRange` field on Model.

**Call-site audit** (production files only):

| File | What it references |
|---|---|
| `app/ui/worddiff.go` | Source — being extracted |
| `app/ui/diffview.go:17` | `matchRange` definition — moves to `worddiff.Range` |
| `app/ui/diffview.go:323` | `applyIntraLineHighlight` — stays, type changes |
| `app/ui/diffview.go:434-467` | `applyIntraLineHighlight` body — stays, calls `worddiff.InsertHighlightMarkers` |
| `app/ui/diffview.go:486-529` | `highlightSearchMatches` — stays, builds `[]worddiff.Range`, calls `worddiff.InsertHighlightMarkers` |
| `app/ui/diffview.go:537-614` | `insertHighlightMarkers` + `updateRestoreBg` — move to worddiff package |
| `app/ui/model.go:177` | `intraRanges [][]matchRange` field — type becomes `[][]worddiff.Range` |
| `app/ui/collapsed.go:295` | `m.pairHunkLines(start, end)` in `buildModifiedSet` — migrates to build `[]worddiff.LinePair` + call `worddiff.PairLines` |
| `app/ui/model.go:629` | `m.recomputeIntraRanges()` — stays (method stays in ui) |
| `app/ui/loaders.go:159` | `m.recomputeIntraRanges()` — stays (method stays in ui) |

**Test files referencing `matchRange` or worddiff functions:**

| File | Sites |
|---|---|
| `app/ui/worddiff_test.go` | All pure-function tests port; `TestModel_PairHunkLines*` and `TestModel_RecomputeIntraRanges*` adapt |
| `app/ui/diffview_test.go:682,707,729` | `recomputeIntraRanges` calls — stay in ui, type changes |
| `app/ui/diffview_test.go:742` | `matchRange` literal — changes to `worddiff.Range` |
| `app/ui/diffview_test.go:937` | `TestModel_UpdateRestoreBg` — calls `m.updateRestoreBg()` which is deleted; test must be removed (coverage moves to `highlight_test.go`) |
| `app/ui/search_test.go` | No direct `matchRange` references — only calls `highlightSearchMatches` which stays |

## Development Approach

- **testing approach**: Regular — implementation first, then tests for each unit, matching the style/sidepane plans.
- **Complete each milestone fully before moving to the next.** Individual tasks within a milestone may leave the codebase in an intermediate broken state — that's allowed and expected. See the Testing Policy section below.
- **Small focused changes within each task.** No bundling unrelated changes.
- **CRITICAL: update this plan file when scope changes during implementation.**
- **Single-level task numbering** (Task 1 through Task N, no hierarchical numbering). Ralphex executes flat numbering sequences.

## Testing Policy — CRITICAL: Milestone Gating, Not Per-Task

**This section deliberately overrides the default planning policy of "all tests must pass before starting next task".**

This refactor touches ~10 production call sites plus matching test updates. The extraction follows the same milestone-gating pattern as style/ and sidepane/:

| Milestone | What's built | Green-gate |
|---|---|---|
| **M1: Worddiff package skeleton** | New `app/ui/worddiff/` package complete with all functions, types, and tests. `ui` package is NOT yet migrated. | `go test ./app/ui/worddiff/... -race` green + `golangci-lint run app/ui/worddiff/...` clean. **Full project build may be broken** — expected. |
| **M2: Call-site migration** | All ui call sites updated to new API. Old `app/ui/worddiff.go` deleted. `matchRange` removed from `diffview.go`. `insertHighlightMarkers`/`updateRestoreBg` removed from `diffview.go`. | Full `go test ./... -race` green. `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` clean. |
| **M3: Docs + cleanup** | `app/ui/doc.go` and project `CLAUDE.md` updated. Plan moved to completed/. | Same as M2 gate + doc cross-check. |

**Non-milestone tasks do NOT have "run tests — must pass" checkboxes.** The milestone task is where convergence happens.

**Commit structure**: one git commit per milestone (3 total).

## Design Philosophy (Inherited)

All 10 design principles from `docs/plans/completed/2026-04-10-style-extraction.md` (Design Philosophy section) apply here. The ones most relevant:

1. **Sub-packages own their domain completely** — `worddiff` owns all LCS/tokenization/range-building/highlight-insertion logic. Callers express intent via the `Differ` type methods.
2. **Named types for domain values** — `Range` (not raw struct), `LinePair` (not raw slice).
3. **Methods over standalone functions** — the public API is methods on `Differ`. Internal helpers (`commonPrefixLen`, `lcsKeptTokens`, etc.) stay as package functions because they have no natural type owner.
4. **Consumer-side interface in ui** — `wordDiffer` interface in `model.go` (3 methods), consistent with `styleResolver`/`styleRenderer`/`sgrProcessor` and `FileTreeComponent`/`TOCComponent` patterns. Model holds an interface-typed field, concrete `*worddiff.Differ` injected via `ModelConfig`.
5. **Dependency injection via ModelConfig** — `ModelConfig.WordDiffer` field, required, wired in `main.go` via `worddiff.New()`.

**Consistency with prior extractions**: style/ uses interfaces for 3 types (Resolver/Renderer/SGR). sidepane/ uses interfaces for 2 types (FileTreeComponent/TOCComponent). worddiff/ uses an interface for 1 type (Differ). The pattern scales down cleanly — even a stateless function set benefits from the interface boundary for testability and decoupling.

## Solution Overview

**Package layout:**

```
app/ui/worddiff/
├── worddiff.go       — Range type, LinePair type, tokenizer, LCS, range builder,
│                       similarity gate, line pairing, constants
├── highlight.go      — InsertHighlightMarkers function, updateRestoreBg helper
├── worddiff_test.go  — tests for tokenizer, LCS, ranges, similarity, pairing
├── highlight_test.go — tests for InsertHighlightMarkers
```

Two source files + two test files. `worddiff.go` holds the core diff algorithms (~300 LOC). `highlight.go` holds the ANSI highlight insertion engine (~90 LOC). Split is by concern: diff computation vs ANSI rendering.

**Public API — `Differ` type with methods:**

```go
package worddiff

// Range represents a byte-offset range in a text line.
// used for both intra-line word-diff highlighting and search match highlighting.
type Range struct {
    Start int // byte offset of range start
    End   int // byte offset past last byte
}

// LinePair represents a line of content with its change direction for pairing.
type LinePair struct {
    Content  string
    IsRemove bool
}

// Pair represents a matched pair of remove/add line indices for intra-line diffing.
type Pair struct {
    RemoveIdx int
    AddIdx    int
}

// Differ provides intra-line word-diff algorithms and highlight marker insertion.
// stateless — the receiver carries no mutable state. exists to group related
// methods under a type for consumer-side interface wrapping (same pattern as style.SGR).
type Differ struct{}

// New returns a Differ. the constructor exists for consistency with the DI pattern
// (main.go calls worddiff.New() and injects into ModelConfig).
func New() *Differ

// ComputeIntraRanges computes changed byte-offset ranges for a pair of minus/plus lines.
// returns ranges for the minus line and plus line respectively.
// returns nil ranges if either line is empty, exceeds MaxLineLenForDiff,
// or fails the similarity gate (< 30% common non-whitespace tokens).
func (d *Differ) ComputeIntraRanges(minusLine, plusLine string) ([]Range, []Range)

// PairLines pairs remove and add lines within a contiguous change block.
// equal-length runs pair 1:1 in order. unequal runs use greedy best-match scoring.
// the indices in the returned Pair values are indices into the input lines slice.
func (d *Differ) PairLines(lines []LinePair) []Pair

// InsertHighlightMarkers walks the string inserting hlOn/hlOff ANSI sequences
// at Range positions, skipping over existing ANSI escape sequences to preserve them.
// tracks background ANSI state so that match-end restores to the correct bg
// rather than always using the static hlOff.
func (d *Differ) InsertHighlightMarkers(s string, matches []Range, hlOn, hlOff string) string

// maxLineLenForDiff caps intra-line diff to lines up to this many bytes.
// unexported: no cross-package consumers need these values.
const maxLineLenForDiff = 500

// similarityThreshold is the minimum percentage of common tokens for highlighting.
const similarityThreshold = 30
```

**Consumer-side interface in ui (`model.go`):**

```go
// wordDiffer is what Model needs for intra-line word-diff and highlight insertion.
// implemented by *worddiff.Differ.
type wordDiffer interface {
    ComputeIntraRanges(minusLine, plusLine string) ([]worddiff.Range, []worddiff.Range)
    PairLines(lines []worddiff.LinePair) []worddiff.Pair
    InsertHighlightMarkers(s string, matches []worddiff.Range, hlOn, hlOff string) string
}
```

Model field: `differ wordDiffer`. ModelConfig field: `WordDiffer wordDiffer` (required).

**main.go wiring:**
```go
cfg := ui.ModelConfig{
    WordDiffer: worddiff.New(),
    // ...
}
```

**Compile-time assertion in model.go:**
```go
var _ wordDiffer = (*worddiff.Differ)(nil)
```

**Note on `Differ` being stateless**: the receiver is unused in all methods — `Differ` exists purely to provide a method namespace for the consumer-side interface. This is the same pattern as `style.SGR` (stateless, zero-value usable, exists for interface wrapping). `New()` returns `&Differ{}` for consistency with the DI pattern, but `&worddiff.Differ{}` works too.

**What stays private (all as methods on `Differ` per CLAUDE.md rule):**

Per the rule "Functions called only from methods MUST be methods", every private function in the call chain of `Differ`'s public methods becomes a private method on `Differ`. The receiver is unused (Differ is stateless), but structural consistency requires it — field access is irrelevant to this decision, the calling pattern determines the structure.

- `(d *Differ) tokenizeLineWithOffsets(line string) []intralineToken` — called from `ComputeIntraRanges`
- `(d *Differ) lcsKeptTokens(minus, plus []intralineToken) ([]bool, []bool)` — called from `ComputeIntraRanges`
- `(d *Differ) buildChangedRanges(tokens []intralineToken, keep []bool) []Range` — called from `ComputeIntraRanges`
- `(d *Differ) passesSimilarityGateFromKeep(minus, plus []intralineToken, keepMinus []bool) bool` — called from `ComputeIntraRanges`
- `(d *Differ) isWhitespaceToken(t intralineToken) bool` — called from `buildChangedRanges`, `countNonWhitespace`
- `(d *Differ) countNonWhitespace(tokens []intralineToken) int` — called from `passesSimilarityGateFromKeep`
- `(d *Differ) greedyPair(lines []LinePair, removes, adds []int) []Pair` — called from `PairLines`
- `(d *Differ) commonPrefixLen(a, b string) int` — called from `greedyPair`
- `(d *Differ) commonSuffixLen(a, b string) int` — called from `greedyPair`
- `(d *Differ) updateRestoreBg(seq, current, hlOff string) string` — called from `InsertHighlightMarkers`

**Standalone (not methods) — justified exceptions:**
- `intralineToken` — data struct (type, not behavior — types cannot be methods)
- `tokenPattern` — `var` of type `*regexp.Regexp`, compiled once at package init. Cannot be a method because it's a variable, not a function. Standard Go pattern for package-level compiled regex. Used by `Differ.tokenizeLineWithOffsets`.

**Rationale for private vs public**: export only what crosses the interface boundary (`Differ` constructor + 3 methods + data types). All internal helpers are private methods on `Differ` for structural consistency. No exported functions exist solely for testability — tests live in `package worddiff` (same package) and can call private methods directly.

**`ComputeIntraRanges` as primary entry point**: combines tokenize → LCS → ranges → similarity gate in one pass. Returns nil ranges when similarity fails. The logic is inlined directly in `ComputeIntraRanges` (no separate `changedRanges` method). Internal helpers are testable via same-package tests.

**`PairLines` signature detail**: accepts `[]LinePair` where each entry has `Content` and `IsRemove`. Internally partitions into remove/add index lists and runs the same equal-count or greedy logic. Returns `[]Pair` with indices into the input slice. The caller (`recomputeIntraRanges`) builds the `[]LinePair` slice from `m.diffLines[blockStart:blockEnd]`.

## Technical Details

**No vendoring needed**: this extraction creates an internal sub-package only. No new external dependencies, so `go mod vendor` is not required.

**Minor behavioral improvement**: the current `recomputeIntraRanges` does NOT enforce `maxLineLenForDiff` — it tokenizes and LCS-diffs lines of any length. After extraction, `ComputeIntraRanges` inherits the `MaxLineLenForDiff` cap (500 bytes), adding the missing O(m*n) safety guard that was only enforced in the standalone `changedTokenRanges` path. Lines over 500 bytes that previously got intra-line highlighting will no longer get it — this is correct behavior (the cap was always intended to apply).

**`recomputeIntraRanges` migration** (stays as a Model method, moves to loaders.go):

The Model method `recomputeIntraRanges` stays in `app/ui/`. It currently lives in `worddiff.go` which is being deleted. It should move to `loaders.go` (where it's called from `handleFileLoaded`) or stay as a small method in a file alongside `toggleWordDiff`. Since `toggleWordDiff` is in `model.go:624`, and `recomputeIntraRanges` is called from both `loaders.go:159` and `model.go:629`, it fits best in `loaders.go` (data preparation concern).

After migration, `recomputeIntraRanges` body becomes:
```go
func (m *Model) recomputeIntraRanges() {
    if !m.wordDiff {
        m.intraRanges = nil
        return
    }
    n := len(m.diffLines)
    m.intraRanges = make([][]worddiff.Range, n)

    i := 0
    for i < n {
        if m.diffLines[i].ChangeType != diff.ChangeAdd && m.diffLines[i].ChangeType != diff.ChangeRemove {
            i++
            continue
        }
        blockStart := i
        for i < n && (m.diffLines[i].ChangeType == diff.ChangeAdd || m.diffLines[i].ChangeType == diff.ChangeRemove) {
            i++
        }

        // build LinePair slice for the block
        block := make([]worddiff.LinePair, i-blockStart)
        for j := blockStart; j < i; j++ {
            block[j-blockStart] = worddiff.LinePair{
                Content:  strings.ReplaceAll(m.diffLines[j].Content, "\t", m.tabSpaces),
                IsRemove: m.diffLines[j].ChangeType == diff.ChangeRemove,
            }
        }

        pairs := m.differ.PairLines(block)
        for _, p := range pairs {
            minusRanges, plusRanges := m.differ.ComputeIntraRanges(block[p.RemoveIdx].Content, block[p.AddIdx].Content)
            m.intraRanges[blockStart+p.RemoveIdx] = minusRanges
            m.intraRanges[blockStart+p.AddIdx] = plusRanges
        }
    }
}
```

**`applyIntraLineHighlight` migration** (stays in `diffview.go`):

Type changes from `matchRange` to `worddiff.Range`. Calls `worddiff.InsertHighlightMarkers` instead of `m.insertHighlightMarkers`. Field access changes from `.start`/`.end` to `.Start`/`.End`.

**`highlightSearchMatches` migration** (stays in `diffview.go`):

Local variable type changes from `[]matchRange` to `[]worddiff.Range`. Field construction changes from `matchRange{start, end}` to `worddiff.Range{Start: start, End: start + len(term)}`. Calls `worddiff.InsertHighlightMarkers` instead of `m.insertHighlightMarkers`.

**`model.go` field change**: `intraRanges [][]matchRange` → `intraRanges [][]worddiff.Range`.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): package creation, function implementation, test porting, call-site migration, doc updates.
- **Post-Completion** (no checkboxes): manual smoke test scenarios.

## Implementation Steps

### Task 1: Create worddiff package with Differ type and core algorithms

Create the package with `Differ` type, `Range`, `LinePair`, `Pair` types, `New()` constructor, tokenizer, LCS, range builder, similarity gate, and `ComputeIntraRanges` entry point. All private helpers as methods on `Differ`.

**Files:**
- Create: `app/ui/worddiff/worddiff.go`

- [x] create `app/ui/worddiff/worddiff.go` with package doc comment describing the sub-package
- [x] define `Differ` struct (empty — stateless, exists for method namespace and interface wrapping) with godoc noting the pattern matches `style.SGR`
- [x] implement `New() *Differ` constructor — returns `&Differ{}`
- [x] define `Range` struct with exported `Start`, `End int` fields and godoc
- [x] define `maxLineLenForDiff` and `similarityThreshold` unexported constants (no cross-package consumers)
- [x] port `tokenPattern` regex var, `intralineToken` type (private data struct)
- [x] port as private methods on `*Differ` (receiver unused, structural consistency per CLAUDE.md rule): `tokenizeLineWithOffsets`, `lcsKeptTokens`, `isWhitespaceToken`, `countNonWhitespace`, `buildChangedRanges` (change return type from `[]matchRange` to `[]Range`, `.start`/`.end` to `.Start`/`.End`), `passesSimilarityGateFromKeep`
- [x] implement `(d *Differ) ComputeIntraRanges(minusLine, plusLine string) ([]Range, []Range)` — method on Differ, inlines tokenize + LCS + ranges + similarity gate in one pass (no separate `changedRanges` method). Returns nil ranges when similarity fails. This is the primary entry point called via the `wordDiffer` interface.

### Task 2: Add line pairing to worddiff package

Port `pairHunkLines` and `greedyPairLines` as methods on `Differ` operating on `[]LinePair` input.

**Files:**
- Modify: `app/ui/worddiff/worddiff.go`

- [x] define `LinePair` struct with exported `Content string`, `IsRemove bool` fields and godoc
- [x] define `Pair` struct with exported `RemoveIdx`, `AddIdx int` fields and godoc
- [x] implement `(d *Differ) PairLines(lines []LinePair) []Pair` — method on Differ. Partitions input into remove/add index lists, uses equal-count 1:1 or greedy path. Indices are into the input `lines` slice.
- [x] port `greedyPairLines` as private method `(d *Differ) greedyPair(lines []LinePair, removes, adds []int) []Pair` — called only from `PairLines`, must be method per rule
- [x] port `commonPrefixLen`, `commonSuffixLen` as private methods on `*Differ` — called only from `greedyPair`

### Task 3: Add highlight marker engine

Port `insertHighlightMarkers` and `updateRestoreBg` from `diffview.go`.

**Files:**
- Create: `app/ui/worddiff/highlight.go`

- [x] create `app/ui/worddiff/highlight.go` with package header
- [x] implement `(d *Differ) InsertHighlightMarkers(s string, matches []Range, hlOn, hlOff string) string` — method on Differ, port from `diffview.go:537-594`. Remove Model receiver. Change `matchRange` → `Range`, `.start`/`.end` → `.Start`/`.End`. Call `d.updateRestoreBg` (private method).
- [x] port `updateRestoreBg` as private method `(d *Differ) updateRestoreBg(seq, current, hlOff string) string` — called only from `InsertHighlightMarkers`, must be method per rule

### Task 4: Write worddiff package tests

Port tests from `app/ui/worddiff_test.go` and add tests for `InsertHighlightMarkers`. Tests live in `package worddiff` (same package) for private function access.

**Files:**
- Create: `app/ui/worddiff/worddiff_test.go`
- Create: `app/ui/worddiff/highlight_test.go`

- [x] create `worddiff_test.go` with package `worddiff` header
- [x] port `TestTokenizeLineWithOffsets` — same test, call `tokenizeLineWithOffsets` directly
- [x] port `TestLCSKeptTokens` — same test
- [x] port `TestBuildChangedRanges` — change `matchRange` → `Range` in expected values
- [x] port `TestChangedTokenRanges` as `TestChangedRanges` — change function name and types
- [x] port `TestChangedTokenRanges_MultibytePrecision` as `TestChangedRanges_MultibytePrecision`
- [x] port `TestChangedTokenRanges_SkipsVeryLongLines` as `TestChangedRanges_SkipsVeryLongLines`
- [x] port `TestPassesSimilarityGateFromKeep` — adapt to test internal `passesSimilarityGateFromKeep` directly
- [x] port `TestPassesSimilarityGateFromKeep_WhitespaceOnly`
- [x] add `TestComputeIntraRanges` — verify that similarity gate is applied: similar pair returns ranges, dissimilar pair returns nil
- [x] port `TestCommonPrefixLen`, `TestCommonSuffixLen`
- [x] port `TestModel_PairHunkLines` as `TestPairLines` — construct `[]LinePair` input instead of `testModel` with diffLines. Adapt all table cases.
- [x] port `TestModel_PairHunkLines_BestMatchScoring` as `TestPairLines_BestMatchScoring`
- [x] create `highlight_test.go` with package `worddiff` header
- [x] add `TestInsertHighlightMarkers` — port relevant assertions from `search_test.go` that exercise the marker insertion (match at start, middle, end, overlapping ANSI, no matches, empty input)
- [x] add `TestInsertHighlightMarkers_BgStateTracking` — test that bg-changing ANSI sequences inside a match cause hlOn re-emission and match-end restores to the correct bg
- [x] add `TestUpdateRestoreBg` — table-driven: 24-bit bg, basic bg, reverse video, bg reset, full reset, non-bg sequence

### Task 5: M1 MILESTONE — Worddiff package green

This is the milestone gate for M1. No new code, just verification.

- [x] run `go test ./app/ui/worddiff/... -race` — must be green
- [x] run `go vet ./app/ui/worddiff/...` — must be clean
- [x] run `golangci-lint run app/ui/worddiff/...` — must be clean
- [x] verify test coverage: `go test ./app/ui/worddiff/... -cover` — target ≥ 80%
- [x] acknowledge that full project build is still broken — expected
- [x] commit M1: `feat(worddiff): add intra-line diff and highlight sub-package`

### Task 6: Add consumer-side interface and ModelConfig wiring, migrate call sites

Add `wordDiffer` interface to `model.go`, add `differ` field to Model, add `WordDiffer` to ModelConfig, wire in `main.go`. Change `matchRange` → `worddiff.Range`, migrate all call sites to use `m.differ.*`.

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/diffview.go`
- Modify: `app/ui/diffview_test.go`
- Modify: `app/ui/collapsed.go`
- Modify: `app/main.go`

- [x] add `import "github.com/umputun/revdiff/app/ui/worddiff"` to `model.go`
- [x] add `wordDiffer` interface to `model.go` (alongside existing `styleResolver`, `styleRenderer`, `sgrProcessor`, `FileTreeComponent`, `TOCComponent` interfaces): 3 methods — `ComputeIntraRanges`, `PairLines`, `InsertHighlightMarkers`
- [x] add compile-time assertion: `var _ wordDiffer = (*worddiff.Differ)(nil)`
- [x] add `differ wordDiffer` field to `Model` struct
- [x] add `WordDiffer wordDiffer` field to `ModelConfig` with godoc noting it's required
- [x] update `NewModel` to validate `cfg.WordDiffer != nil` (return error) and copy into `m.differ`
- [x] change `model.go:177` — `intraRanges [][]matchRange` → `intraRanges [][]worddiff.Range`
- [x] add `import "github.com/umputun/revdiff/app/ui/worddiff"` to `diffview.go`
- [x] delete `matchRange` type definition at `diffview.go:17`
- [x] in `applyIntraLineHighlight`: replace `m.insertHighlightMarkers(textContent, ranges, hlOn, hlOff)` → `m.differ.InsertHighlightMarkers(textContent, ranges, hlOn, hlOff)`
- [x] in `highlightSearchMatches` (diffview.go:486): change local `var matches []matchRange` → `var matches []worddiff.Range`
- [x] in `highlightSearchMatches`: change `matches = append(matches, matchRange{start, start + len(term)})` → `matches = append(matches, worddiff.Range{Start: start, End: start + len(term)})`
- [x] in `highlightSearchMatches`: replace `m.insertHighlightMarkers(s, matches, hlOn, hlOff)` → `m.differ.InsertHighlightMarkers(s, matches, hlOn, hlOff)`
- [x] delete `insertHighlightMarkers` method (diffview.go:537-594)
- [x] delete `updateRestoreBg` method (diffview.go:598-614)
- [x] delete `TestModel_UpdateRestoreBg` from `diffview_test.go:937` — coverage moves to `highlight_test.go:TestUpdateRestoreBg`
- [x] migrate `collapsed.go:295` — `buildModifiedSet` calls `m.pairHunkLines(start, end)`. Replace with: build `[]worddiff.LinePair` from `m.diffLines[start:end]`, call `m.differ.PairLines(block)`, check `len(pairs) > 0` (only the pair count matters here, indices are not used)
- [x] add `import "github.com/umputun/revdiff/app/ui/worddiff"` to `collapsed.go`
- [x] wire in `app/main.go`: add `import "github.com/umputun/revdiff/app/ui/worddiff"`, set `WordDiffer: worddiff.New()` in the `ModelConfig{...}` literal

### Task 7: Migrate recomputeIntraRanges to use worddiff package

Move `recomputeIntraRanges` from `app/ui/worddiff.go` to `app/ui/loaders.go` and rewrite to call worddiff package functions.

**Files:**
- Modify: `app/ui/loaders.go`

- [x] add `import "github.com/umputun/revdiff/app/ui/worddiff"` to `loaders.go` (for `worddiff.LinePair` and `worddiff.Range` type names)
- [x] move `recomputeIntraRanges` method from `app/ui/worddiff.go` to `app/ui/loaders.go`
- [x] rewrite the method body to: build `[]worddiff.LinePair` from `m.diffLines` block, call `m.differ.PairLines`, call `m.differ.ComputeIntraRanges` for each pair, store results in `m.intraRanges` (see Technical Details for the exact code)
- [x] verify the method is called from `loaders.go:159` (handleFileLoaded) and `model.go:629` (toggleWordDiff) — both call sites unchanged since the method signature is identical

### Task 8: Delete old worddiff.go and migrate remaining tests

Delete the source file and update test files that reference old types.

**Files:**
- Delete: `app/ui/worddiff.go`
- Delete: `app/ui/worddiff_test.go`
- Modify: `app/ui/loaders_test.go`
- Modify: `app/ui/diffview_test.go`

- [x] delete `app/ui/worddiff.go` — all functions either moved to worddiff package or to loaders.go
- [x] move `TestModel_RecomputeIntraRanges*` tests from `app/ui/worddiff_test.go` into `app/ui/loaders_test.go` — follows "one test file per source file" convention since `recomputeIntraRanges` now lives in `loaders.go`. Combined size: ~824 + ~118 = ~942 lines, within the ~1000 soft target.
- [x] delete `app/ui/worddiff_test.go` — all tests either ported to `worddiff/worddiff_test.go` (pure functions) or merged into `loaders_test.go` (Model method tests)
- [x] update `TestModel_RecomputeIntraRanges*` tests in `loaders_test.go`: change `matchRange` → `worddiff.Range`, update field access if needed
- [x] update `diffview_test.go:742`: change `matchRange{0, 3}` → `worddiff.Range{Start: 0, End: 3}`
- [x] update `diffview_test.go` imports to include `worddiff`
- [x] update test helper (`testModel` or `testNewModel` or `ModelConfig` literals in test files) to include `WordDiffer: worddiff.New()` — every `ModelConfig{...}` in tests needs this field or `NewModel` returns error
- [x] confirm no ui file still references `matchRange` (type), `changedTokenRanges` (function), `pairHunkLines` (method), `greedyPairLines` (method), `insertHighlightMarkers` (method), `updateRestoreBg` (method): `grep -rn 'matchRange\|changedTokenRanges\|pairHunkLines\|greedyPairLines\|insertHighlightMarkers\|updateRestoreBg' app/ui/*.go` — should find zero hits
- [x] `go build ./...` — should compile now

### Task 9: M2 MILESTONE — full test suite green

- [x] run `go test ./... -race` — must be green end-to-end
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — must be clean
- [x] run `go test ./app/ui/... -cover` and verify coverage has not regressed
- [x] build the binary: `make build`
- [x] manual smoke test: launch `.bin/revdiff` on a git repo with word-diff enabled (`W` key) — verify intra-line highlighting works on add/remove lines
- [x] manual smoke test: search (`/`) for a term — verify search highlighting works
- [x] manual smoke test: `--no-colors` mode — verify reverse-video fallback works for both word-diff and search
- [x] commit M2: `refactor(ui): extract worddiff sub-package`

### Task 10: Update documentation

**Files:**
- Modify: `app/ui/doc.go`
- Modify: `CLAUDE.md`

- [x] read `app/ui/doc.go` to understand its current structure
- [x] update `app/ui/doc.go`: remove `worddiff.go` description, add `worddiff/` sub-package description noting it contains intra-line word-diff algorithms and the shared highlight marker insertion engine
- [x] update `CLAUDE.md` Project Structure section: add `app/ui/worddiff/` entry describing the sub-package
- [x] update `CLAUDE.md` Data Flow section: update the word-diff paragraph to note that `worddiff.ComputeIntraRanges()` and `worddiff.PairLines()` are in the sub-package, and `worddiff.InsertHighlightMarkers()` is used by both word-diff and search highlighting

### Task 11: M3 MILESTONE — docs complete, plan moved

- [x] run `go test ./... -race` — must pass
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — must be clean
- [x] verify `app/ui/worddiff/` test coverage ≥ 80%
- [x] move this plan: `git mv docs/plans/2026-04-11-worddiff-extraction.md docs/plans/completed/`
- [x] commit M3: `docs: complete worddiff extraction plan`

## Post-Completion

*Items requiring manual intervention — no checkboxes, informational only*

**Manual smoke tests** (part of M2 gate, documented here for reference):
- Multi-file diff with word-diff on (`W` key): verify intra-line highlighting on add/remove pairs
- Search (`/` + term): verify search highlights appear, `n`/`N` navigate correctly
- Word-diff + search combined: verify both highlight layers coexist
- `--no-colors` mode: verify reverse-video fallback for both word-diff and search
- Collapsed mode with word-diff: verify collapsed add lines still show word-diff

**Follow-up candidates** (explicitly out of scope):
- Evaluate whether `configpatch.go` should move into its own package (66 LOC, single function — likely too small)
- Evaluate whether overlays (`annotlist.go`, `themeselect.go`) should become sub-packages with result-type event patterns
