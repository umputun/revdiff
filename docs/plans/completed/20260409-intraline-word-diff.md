# Intra-line word-diff highlighting

## Overview

Add always-on intra-line highlighting to revdiff. Within each diff hunk, paired remove/add lines run through a token-level diff and only the actually changed spans get a brighter background overlay — the existing whole-line add/remove styling is preserved.

This combines ideas from two contributor PRs:
- **PR #74 (daulet)**: zero-dependency token-level algorithm (regex tokenizer + LCS)
- **PR #73 (rashpile)**: production integration (pairing, similarity gate, colors, `insertBgMarkers`, collapsed mode refactoring)

The feature is always on — no toggle flag, no keybinding, no status icon.

## Context (from discovery)

**Key decisions from brainstorm:**
1. **Algorithm**: PR #74's regex tokenizer + LCS (token-level, no `go-diff` dependency)
2. **Pairing**: PR #73's `pairHunkLines` with `2*commonPrefix + 2*commonSuffix` scoring
3. **Similarity gate**: 30% threshold — pairs with <30% common content get no intra-line overlay
4. **Colors**: Dedicated `WordAddBg`/`WordRemoveBg` with HSL auto-derivation from add/remove bg
5. **Always on**: no toggle, no `--word-diff` flag, no `W` keybinding, no `⇄` status icon
6. **Integration**: reuse existing `insertHighlightMarkers` (`diffview.go:458`) with different ANSI on/off
7. **Collapsed mode**: shared `pairHunkLines` refactored from `buildModifiedSet`

**Existing integration points:**
- `matchRange` type at `diffview.go:16` — reuse for intra-line ranges
- `insertHighlightMarkers` at `diffview.go:458` — reuse for ANSI insertion
- `prepareLineContent` at `diffview.go:390` — hook point for intra-line markup
- `handleFileLoaded` at `loaders.go:158` — compute ranges after `HighlightLines`
- `buildModifiedSet` at `collapsed.go:270` — extract shared pairing logic
- `colorFieldPtrs` at `main.go:644` — register new color keys
- `colorKeys` at `theme.go:44` + `optionalColorKeys` at `theme.go:58` — theme registration

## Development Approach
- **testing approach**: TDD where practical — write tests for algorithm functions first, integration tests after wiring
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- run tests after each change
- maintain backward compatibility

## Testing Strategy
- **unit tests**: table-driven tests for tokenizer, LCS, pairing, similarity gate, range computation, color derivation
- **integration tests**: render pipeline tests verifying ANSI markers appear in output
- **edge cases**: empty lines, pure add/remove blocks (no pairs), unicode/multibyte, very long lines, identical paired lines

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix

## Solution Overview

**Data flow:**
```
handleFileLoaded → diffLines set
  → recomputeIntraRanges():
    1. walk diffLines finding contiguous add/remove blocks
    2. pairHunkLines() pairs remove/add lines via prefix/suffix scoring
    3. for each pair: tokenize both lines (regex), run LCS, extract changed byte ranges
    4. similarity gate: skip pairs with <30% common tokens
    5. store [][]matchRange parallel to diffLines (nil for unpaired lines)
  → renderDiff → renderDiffLine → prepareLineContent
    → apply intra-line markers via insertHighlightMarkers before styleDiffContent
```

**Color derivation (inside `normalizeColors` in `styles.go`):**
```
if WordAddBg empty → shiftLightness(AddBg, 0.15)
if WordRemoveBg empty → shiftLightness(RemoveBg, 0.15)
```
Both `shiftLightness` and `normalizeColors` live in package `ui` — no cross-package call needed.

**Tab handling:** `recomputeIntraRanges` works on tab-replaced content (`strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)`) so byte ranges align with tab-replaced `lineContent`/`textContent` from `prepareLineContent`. No render pipeline restructuring needed.

**Byte offsets:** all token offsets and `matchRange` values are byte positions (not rune positions), consistent with `insertHighlightMarkers` which advances byte-by-byte.

**Wrap mode:** `reemitANSIState` currently tracks fg/bold/italic only — no background state. Word-diff bg markers would be lost on continuation lines after wrap. Task 5 adds bg tracking to `reemitANSIState`.

**Method vs function rule:** `recomputeIntraRanges` and `pairHunkLines` are methods on Model (called exclusively from Model methods). Pure algorithm functions (`tokenizeLineWithOffsets`, `lcsKeptTokens`, `buildChangedRanges`, `changedTokenRanges`) are standalone package-level utilities — they operate on strings/tokens with no Model dependency.

**Collapsed mode:** no intra-line word-diff in collapsed view — collapsed mode shows which lines are modified (amber styling) but not which words. Adding word-level detail to a collapsed view is noisy and can be added later if needed.

## Implementation Steps

### Task 1: Color utility and theme wiring

**Files:**
- Create: `app/ui/colorutil.go`
- Create: `app/ui/colorutil_test.go`
- Modify: `app/ui/styles.go` — add `WordAddBg`, `WordRemoveBg` to `Colors` struct, auto-derive in `normalizeColors`
- Modify: `app/theme/theme.go` — add to `colorKeys` and `optionalColorKeys`
- Modify: `app/theme/theme_test.go` — update tests for new keys
- Modify: `app/main.go` — add to `colorFieldPtrs`, `run()` color mapping

- [x] create `colorutil.go` with `shiftLightness(hexColor string, amount float64) string` (unexported — same package as caller)
- [x] implement hex parsing, RGB→HSL→shift lightness toward 0.5 by `amount`→HSL→RGB→hex
- [x] add `WordAddBg`, `WordRemoveBg` fields to `Colors` struct in `styles.go` (after `RemoveBg`)
- [x] add auto-derivation in `normalizeColors`: if `WordAddBg` empty, set to `shiftLightness(AddBg, 0.15)`, same for remove
- [x] add `normalizeColor` calls for both new fields (after auto-derivation)
- [x] add `"color-word-add-bg"`, `"color-word-remove-bg"` to `colorKeys` slice in `theme.go`
- [x] add both to `optionalColorKeys` map in `theme.go`
- [x] add CLI flag definitions to `options.Colors` struct in `main.go` (with `long`, `ini-name`, `env` tags)
- [x] add 2 entries to `colorFieldPtrs` map in `main.go`
- [x] add `WordAddBg`/`WordRemoveBg` to `ui.Colors{...}` in `run()` function
- [x] write table-driven tests for `shiftLightness`: dark colors get lighter, light colors get darker, edge cases (black, white, empty)
- [x] write test for auto-derivation in `normalizeColors`: verify derivation when empty, no-op when explicitly set
- [x] update theme tests to account for new keys count
- [x] run tests — must pass before task 2

### Task 2: Core algorithm — tokenizer, LCS, range computation

**Files:**
- Create: `app/ui/worddiff.go`
- Create: `app/ui/worddiff_test.go`

All functions below are standalone package-level utilities (pure algorithm, no Model dependency). All offsets and `matchRange` values are byte positions (not rune), matching `insertHighlightMarkers` semantics.

- [x] create `worddiff.go` with `tokenizeLineWithOffsets(line string) []intralineToken` — regex tokenizer (`[\pL\pN_]+|\s+|[^\pL\pN_\s]+`), returns byte-offset tokens
- [x] implement `lcsKeptTokens(minusTokens, plusTokens []intralineToken) ([]bool, []bool)` — O(m×n) LCS DP + backtrace
- [x] implement `buildChangedRanges(tokens []intralineToken, keep []bool) []matchRange` — merge adjacent changed non-whitespace tokens into byte-offset ranges
- [x] implement `changedTokenRanges(minusLine, plusLine string) ([]matchRange, []matchRange)` — orchestrator calling tokenize → LCS → build ranges
- [x] write table-driven tests for tokenizer: words, punctuation, whitespace, unicode/multibyte, empty string
- [x] write table-driven tests for LCS: identical lines (no ranges), single rename, multiple changes, fully different
- [x] write tests for buildChangedRanges: adjacent merging, whitespace exclusion
- [x] write tests for changedTokenRanges: end-to-end with exact byte positions, including multibyte characters
- [x] run tests — must pass before task 3

### Task 3: Line pairing and intra-range computation

**Files:**
- Modify: `app/ui/worddiff.go`
- Modify: `app/ui/worddiff_test.go`
- Modify: `app/ui/model.go` — add `intraRanges` field

`pairHunkLines` and `recomputeIntraRanges` are methods on Model (called exclusively from Model methods). `intraRanges` field added here so `recomputeIntraRanges` compiles and is testable.

- [x] add `intraRanges [][]matchRange` field to `Model` struct in `model.go` (after `highlightedLines`)
- [x] implement `pairHunkLines(start, end int) []intralinePair` as method on Model — walk contiguous change block in `m.diffLines`, collect remove/add indices
- [x] implement scoring: equal-length runs pair 1:1, unequal runs use greedy best-match with `2*commonPrefixLen + 2*commonSuffixLen`
- [x] implement `recomputeIntraRanges()` as method on Model — walk `m.diffLines`, find change blocks, call `m.pairHunkLines` + `changedTokenRanges` on tab-replaced content (`strings.ReplaceAll(dl.Content, "\t", m.tabSpaces)`), store results in `m.intraRanges`
- [x] implement 30% similarity gate inside `recomputeIntraRanges`: after `changedTokenRanges`, if `totalEqual*100 < shorter*30`, discard ranges for that pair
- [x] write tests for pairing: equal-count, unequal-count, pure-add (no pairs), pure-remove (no pairs)
- [x] write tests for similarity gate: similar lines get ranges, dissimilar lines get nil
- [x] write test for `recomputeIntraRanges` with a realistic hunk
- [x] run tests — must pass before task 4

### Task 4: Integrate into render pipeline

**Files:**
- Modify: `app/ui/loaders.go` — call `recomputeIntraRanges` in `handleFileLoaded`
- Modify: `app/ui/diffview.go` — apply intra-line markers in render path, add bg tracking to `reemitANSIState`
- Modify: `app/ui/diffview_test.go` — integration tests

Ranges are computed on tab-replaced content (Task 3), so byte offsets already align with `prepareLineContent` output — no render pipeline restructuring needed.

- [x] call `m.recomputeIntraRanges()` in `handleFileLoaded` after `m.highlightedLines` is set (after line 158)
- [x] in `renderDiffLine`, after `prepareLineContent`: when `m.intraRanges[idx]` is non-nil and line is add/remove, call `m.insertHighlightMarkers(textContent, ranges, hlOn, hlOff)` with `WordAddBg`/`WordRemoveBg` ANSI sequences
- [x] handle no-color mode fallback: use reverse-video markers matching search highlight fallback pattern
- [x] wire same logic into `renderWrappedDiffLine` for wrap mode compatibility
- [x] add background state tracking to `reemitANSIState` — track `\033[48;2;r;g;bm` and `\033[49m` alongside existing fg/bold/italic, so word-diff bg survives across wrapped continuation lines
- [x] update `scanANSIState`/`applySGR` to handle bg codes (48;2;r;g;b set, 49 reset, 0 full reset)
- [x] write integration test: render a hunk with paired add/remove, verify ANSI bg markers appear in output
- [x] write test: unpaired lines (pure add block) produce no intra-line markers
- [x] write test: no-color mode uses reverse-video
- [x] write test: wrapped lines preserve word-diff bg on continuation lines
- [x] write test: tab-containing lines have correctly positioned highlights
- [x] run tests — must pass before task 5

### Task 5: Refactor collapsed mode to share pairHunkLines

**Files:**
- Modify: `app/ui/collapsed.go`
- Modify: `app/ui/collapsed_test.go`

Note: current `buildModifiedSet` already strips trailing non-change lines (lines 281-284). No bug fix needed — verify during refactoring that behavior is preserved.

- [x] refactor `buildModifiedSet` to call `m.pairHunkLines` for pairing logic instead of inline loop
- [x] verify `buildModifiedSet` produces identical results (same `map[int]bool` output) — run existing collapsed tests
- [x] add test case verifying trailing context lines are excluded from modified set
- [x] run tests — must pass before task 6

### Task 6: Update bundled themes

**Files:**
- Modify: `themes/gallery/revdiff`
- Modify: `themes/gallery/catppuccin-mocha`
- Modify: `themes/gallery/catppuccin-latte`
- Modify: `themes/gallery/dracula`
- Modify: `themes/gallery/gruvbox`
- Modify: `themes/gallery/nord`
- Modify: `themes/gallery/solarized-dark`

Auto-derivation via `shiftLightness` may produce acceptable results for all themes. Only add explicit values where the auto-derived color doesn't look right for the theme's palette.

- [x] test each bundled theme without explicit word-diff keys — verify auto-derived colors are visually acceptable
- [x] for themes where auto-derivation looks wrong, add explicit `color-word-add-bg`/`color-word-remove-bg`
- [x] verify all themes load without errors
- [x] run tests — must pass before task 7

### Task 7: Verify acceptance criteria

- [x] verify intra-line highlighting works on expanded diff mode
- [x] verify intra-line highlighting works with wrap mode enabled (continuation lines preserve bg)
- [x] verify intra-line highlighting works with search active (no visual collision — different bg colors)
- [x] verify intra-line highlighting works with line numbers and blame gutter
- [x] verify collapsed mode still works correctly with shared pairing (no intra-line in collapsed — intentional)
- [x] verify no-color mode fallback (reverse-video)
- [x] verify custom themes without word-diff keys auto-derive colors
- [x] verify horizontal scroll doesn't break intra-line markers
- [x] run full test suite: `make test`
- [x] run linter: `make lint`

### Task 8: [Final] Update documentation

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `CLAUDE.md`

- [x] add intra-line highlighting to README.md features section
- [x] add `color-word-add-bg`/`color-word-remove-bg` to README color configuration table
- [x] update site/docs.html with feature description and color keys
- [x] update CLAUDE.md data flow documentation to include intra-line highlighting
- [x] update CLAUDE.md with worddiff.go and colorutil.go in project structure

## Post-Completion

**PR communication:**
- comment on PR #73 and #74 explaining the combined approach and crediting both contributors
- reference this plan in the implementation PR

**Manual verification:**
- test on real-world diffs: large refactors, indentation-only changes, single-character edits
- verify no visual regression on existing themes
- test with custom user themes that don't define word-diff colors
