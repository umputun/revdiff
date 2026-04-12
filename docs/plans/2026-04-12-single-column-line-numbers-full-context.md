# Single-Column Line Numbers for Full-Context Files

## Overview
- When line numbers are enabled (`L` toggle) in full-context modes (`--only`, `--stdin`, `--all-files`), the gutter shows two identical columns (`" OOO NNN"`) because there's no "previous version" — both `OldNum` and `NewNum` are set to the same value
- Fix: detect full-context files per-file and render a single column (`" NNN"`) instead, saving horizontal space
- Applies to all rendering paths: expanded, wrapped, collapsed, and collapsed-wrapped

## Context (from discovery)
- `lineNumGutter()` at `app/ui/diffview.go:28` always renders two columns for context lines
- `lineNumGutterWidth()` at `app/ui/diffview.go:19` always computes `2*W + 2`
- `readReaderAsContext()` at `app/diff/fallback.go:193` sets `OldNum == NewNum` for all lines
- `isFullContext()` at `app/ui/loaders.go:343` already detects all-context files (used for markdown TOC)
- `FallbackRenderer` (`app/diff/fallback.go`) can return real diffs for `--only` files that are in VCS diff — detection must be per-file, not global
- Affected rendering functions: `lineNumGutter()`, `lineNumGutterWidth()`, `renderDiffLine()`, `renderWrappedDiffLine()`, `renderCollapsedAddLine()`, `renderWrappedCollapsedLine()`, `gutterBlanks()`

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task (see Development Approach above)
- Existing tests in `app/ui/diffview_test.go`, `app/ui/collapsed_test.go` cover gutter rendering

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Add `singleColLineNum` flag and detection in `handleFileLoaded`
<!-- [conventions] Keep the cached field — matches project pattern (lineNumWidth, blameAuthorLen are cached in handleFileLoaded). isFullContext() is O(n) and lineNumGutterWidth() is called per rendered line in the hot path -->
<!-- [completionist] isFullContext() is already called in handleFileLoaded for markdown TOC detection. Capture the result in a local variable and reuse for both TOC check and singleColLineNum to avoid scanning diffLines twice -->
<!-- [completionist] Add a one-line code comment: "Cached because isFullContext() is O(n) and lineNumGutterWidth() is called per rendered line" -->
- [x] add `singleColLineNum bool` field to `Model` struct in `app/ui/model.go`
- [x] in `handleFileLoaded()` (`app/ui/loaders.go:145`), after computing `lineNumWidth`, set `m.singleColLineNum = m.isFullContext(msg.lines)` — this reuses the existing `isFullContext()` helper and evaluates per-file on every file load. Reuse the `isFullContext()` result from the existing markdown TOC check to avoid a second O(n) scan
- [x] write test for `singleColLineNum` set to true when file is full-context (all `ChangeContext` lines)
- [x] write test for `singleColLineNum` set to false when file has add/remove lines (real diff)
- [x] run tests - must pass before next task

### Task 2: Audit callers, then update `lineNumGutterWidth()` for single column
<!-- [architect] Audit must happen BEFORE implementation to discover full surface area. Fold Task 4 audit here -->
<!-- [go_idioms] Verify gutterBlanks() delegates to lineNumGutterWidth() rather than computing its own width -->
<!-- [go_idioms] Verify blameGutterWidth() is independent of lineNumGutterWidth() (almost certain — blame W is author width, not line-num width) -->
<!-- [conventions] Add matching layout comment: // layout: " " + num(W) = W + 1 to parallel existing // layout: " " + oldNum(W) + " " + newNum(W) = 2*W + 2 -->
- [ ] audit: grep all callers of `lineNumGutterWidth()` and `lineNumGutter()` — confirm `renderDiffLine()`, `renderWrappedDiffLine()`, `renderCollapsedAddLine()`, `renderWrappedCollapsedLine()`, `gutterBlanks()`, `applyHorizontalScroll()` all go through shared functions (no inline gutter formatting)
- [ ] audit: confirm `blameGutterWidth()` is independent of `lineNumGutterWidth()`
- [ ] modify `lineNumGutterWidth()` in `app/ui/diffview.go:19`: when `m.singleColLineNum`, return `m.lineNumWidth + 1` (layout: `" NNN"`) instead of `m.lineNumWidth*2 + 2`
- [ ] write tests for `lineNumGutterWidth()` returning correct width in single-column mode
- [ ] write tests for `lineNumGutterWidth()` still returning two-column width when `singleColLineNum` is false
- [ ] run tests - must pass before next task

### Task 3: Update `lineNumGutter()` to render single column when `singleColLineNum` is true
<!-- [architect][simplifier][conventions][completionist][go_idioms] UNANIMOUS: Drop the two-column fallback guard for add/remove lines. It creates a width mismatch (lineNumGutter outputs 2*W+2 while lineNumGutterWidth reports W+1), causing visual corruption in horizontal scroll, extendLineBg, and gutterBlanks. The invariant is structural — isFullContext() guarantees no add/remove lines exist. Always render single-column when singleColLineNum is true -->
<!-- [completionist] Add a width-consistency test: assert runewidth.StringWidth(lineNumGutter(dl)) == lineNumGutterWidth() for representative DiffLine values in both single-column and two-column modes -->
- [ ] modify `lineNumGutter()` in `app/ui/diffview.go:28`: when `m.singleColLineNum`, render `" NNN"` (single column using `dl.NewNum`) for all line types — no fallback to two-column logic (width must always match `lineNumGutterWidth()`)
- [ ] write tests for single-column gutter output format (`" NNN"`) for context lines
- [ ] write tests for divider lines rendering blank single column
- [ ] write width-consistency test: `runewidth.StringWidth(stripANSI(lineNumGutter(dl))) == lineNumGutterWidth()` for both modes
- [ ] write test that two-column format is unchanged when `singleColLineNum` is false
- [ ] run tests - must pass before next task

<!-- [architect][simplifier][go_idioms] Task 4 (audit) folded into Task 2 — audit callers before making changes, not after -->

### Task 4: Verify acceptance criteria
<!-- [completionist] Add horizontal scroll test with single-column mode — scroll indicators shift when gutter is narrower -->
<!-- [completionist] Add manual test for blame + line numbers together in full-context mode -->
- [ ] verify single-column gutter renders for full-context files (all ChangeContext)
- [ ] verify two-column gutter still renders for files with real diffs
- [ ] verify per-file detection: switching between full-context and diff files updates the gutter correctly
- [ ] run full test suite (`make test`)
- [ ] run linter (`make lint`) — all issues must be fixed

<!-- [simplifier][conventions][go_idioms] Task 6 (documentation) dropped — no user-facing behavior change, no new flags/keybindings/modes. Gutter column count is an internal rendering detail not documented anywhere -->

## Technical Details
- **Detection**: `singleColLineNum` is set per-file in `handleFileLoaded` via `isFullContext(msg.lines)`. This means switching files in the tree recalculates correctly
- **Single-column layout**: `" NNN"` — leading space + right-aligned number, width = `W + 1`
- **Two-column layout** (unchanged): `" OOO NNN"` — leading space + old + space + new, width = `2*W + 2`
- **No fallback guard**: when `singleColLineNum` is true, always render single-column for all line types. The invariant is structural (set by the parser). A two-column fallback would create width mismatch with `lineNumGutterWidth()`

## Post-Completion
**Manual verification:**
- Run `revdiff --only README.md` with `L` toggle — should show single column
- Run `revdiff` on a branch with changes, toggle `L` — diff files should still show two columns
- Run `echo "hello" | revdiff --stdin` with `L` toggle — single column
- Run `revdiff --all-files` and browse a file with no changes — single column
- Test with `--only` on a file that IS in the diff (has changes) — should show two columns
- Run `revdiff --only README.md` with both `L` and `B` toggles active — verify blame + single-column line numbers don't misalign
- Run `revdiff --only` on a file with long lines, enable `L`, scroll right — verify `«`/`»` indicators at correct positions
- Revert path: set `singleColLineNum = false` unconditionally to restore previous behavior
