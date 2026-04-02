# Vim-Style Search in Diff Pane

## Overview
Add `/` search in the diff pane following vim/less conventions. Users can search for text within the current file's diff, navigate between matches with `n`/`N`, and see all matches highlighted with configurable colors. This addresses the need to quickly find specific code within large diffs.

## Context
- **Key files:** `ui/model.go`, `ui/diffview.go`, `ui/styles.go`, `ui/model_test.go`, `ui/annotate.go`, `cmd/revdiff/main.go`
- **Reuse pattern:** annotation textinput (`textinput.Model`, `newAnnotationInput`, `handleAnnotateKey`) provides the exact input/dismiss pattern needed
- **Key constraint:** `n` is globally bound to next file — when search is active, `n` overrides to next-match instead. `p` (prev file) is never overridden. `N` (shift-n) is a new key exclusively for prev-match
- **Rendering:** `renderDiffLine` (diffview.go:68) applies per-line styling; search highlighting needs to mark entire matching lines
- **Colors:** `Colors` struct (styles.go:6) has 19 fields with CLI flags, env vars, config file support via go-flags

## Solution Overview
- `/` opens a `textinput` replacing the status bar (same pattern as annotation input)
- Case-insensitive substring match against `DiffLine.Content`, current-file-only scope
- `enter` submits search, jumps to first match forward from cursor; `esc` cancels
- All matching lines highlighted with configurable `SearchBg`/`SearchFg` colors
- Current match line uses cursor line style (existing `DiffCursorLine`)
- `n` = next match, `N` = prev match when search is active (overrides next-file `n`)
- `n`/`p` revert to next/prev file when search is cleared
- Status line shows `[X/Y]` match position when search is active
- Search clears on: new file load, empty `/` submit
- Help overlay updated with `/`, `n`, `N` keys

## Technical Details

### New fields in Model (model.go)
- `searching bool` — true when search textinput is active (typing)
- `searchTerm string` — last submitted search query
- `searchMatches []int` — indices into `m.diffLines` that match
- `searchCursor int` — current position in `searchMatches` (0-based)
- `searchInput textinput.Model` — dedicated textinput for search (separate from annotateInput)

### New color fields in Colors (styles.go)
- `SearchFg string` — foreground for matched lines (default: dark, e.g. `#1a1a1a`)
- `SearchBg string` — background for matched lines (default: yellow, e.g. `#d7d700`)
- New style in `styles` struct: `SearchMatch lipgloss.Style`

### New CLI flags (main.go)
- `--color-search-fg` with `ini-name:"color-search-fg"` and `env:"REVDIFF_COLOR_SEARCH_FG"`
- `--color-search-bg` with `ini-name:"color-search-bg"` and `env:"REVDIFF_COLOR_SEARCH_BG"`

### Search flow
1. User presses `/` in diff pane → `startSearch()` creates textinput, sets `m.searching = true`
2. Status bar shows search input with `[enter] search  [esc] cancel`
3. User types query and presses enter → `submitSearch()`:
   - Stores `m.searchTerm = strings.ToLower(input)`
   - Scans `m.diffLines` for case-insensitive substring matches, stores indices in `m.searchMatches`
   - Sets `m.searchCursor` to first match at or after current `m.diffCursor`
   - Moves `m.diffCursor` to that match and syncs viewport
   - Sets `m.searching = false`, re-renders diff
4. User presses `esc` → `cancelSearch()` clears input, sets `m.searching = false`

### Match navigation
- `n` key: when `len(m.searchMatches) > 0`, advance `m.searchCursor` (wrap around), move `m.diffCursor` to that line, sync viewport. Otherwise fall through to next-file.
- `N` key: same but backwards (prev match, wrap around)

### Match highlighting in renderDiffLine (diffview.go) and renderCollapsedAddLine (collapsed.go)
- Store search match set (`map[int]bool`) as a Model field, computed once per render in `renderDiff()`/`renderCollapsedDiff()`
- In `renderDiffLine`: after building line content (before horizontal scroll), check if index is in match set; if so, apply `m.styles.SearchMatch` background
- In `renderCollapsedAddLine`: same check and highlight — collapsed mode uses a separate rendering path that bypasses `renderDiffLine`
- The cursor line (`idx == m.diffCursor`) already gets the cursor indicator; no extra styling needed since the cursor bar `▶` is sufficient to identify current match
- Search highlight and syntax highlight coexist: search bg replaces the change-type bg for matched lines
- Matches remain visible while typing new search input; cleared only on submit or explicit clear

### Search clearing
- `handleFileLoaded`: reset `m.searchTerm`, `m.searchMatches`, `m.searchCursor`
- Empty `/` submit: same reset
- Starting a new `/` search: previous matches cleared when new search submitted

### Status line integration
- When `m.searching` is true: show search input (replaces status bar)
- When search matches exist (`len(m.searchMatches) > 0`): append `[X/Y]` to status line segments, where X = `m.searchCursor+1`, Y = `len(m.searchMatches)`
- The `[X/Y]` segment goes between hunk info and mode icons

## Development Approach
- **Testing approach:** regular (code first, then tests)
- Complete each task fully before moving to the next
- Run tests after each change
- Maintain backward compatibility (existing keys, config, styles all still work)

## Implementation Steps

### Task 1: Add search colors and styles

**Files:**
- Modify: `ui/styles.go`
- Modify: `cmd/revdiff/main.go`
- Modify: `ui/model_test.go`

- [ ] add `SearchFg` and `SearchBg` string fields to `Colors` struct
- [ ] add `SearchMatch lipgloss.Style` to `styles` struct
- [ ] initialize `SearchMatch` style in `newStyles()` with foreground and background from colors
- [ ] update `normalizeColors()` to normalize `SearchFg` and `SearchBg`
- [ ] add CLI flags `--color-search-fg` (default `#1a1a1a`) and `--color-search-bg` (default `#d7d700`) in main.go opts
- [ ] wire new color fields in `run()` function's `ui.Colors{}` literal
- [ ] update `plainStyles()` to include `SearchMatch` for tests
- [ ] run `make test` — must pass before task 2

### Task 2: Add search model fields and input handling

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] add `searching bool`, `searchTerm string`, `searchMatches []int`, `searchCursor int`, `searchInput textinput.Model` fields to `Model`
- [ ] add `startSearch()` method — creates textinput with `/` placeholder, sets `m.searching = true`
- [ ] add `handleSearchKey(msg)` method — enter calls `submitSearch`, esc calls `cancelSearch`, default forwards to `searchInput.Update(msg)`
- [ ] add `submitSearch()` method — if input empty, call `clearSearch()` and return; otherwise store lowercase search term, scan `diffLines` for case-insensitive matches, populate `searchMatches`, move cursor to first match forward, set `searching = false`
- [ ] add `cancelSearch()` method — clears input, sets `searching = false`
- [ ] forward non-key messages to `searchInput` in `Update()` when `m.searching` (parallel to annotating path)
- [ ] handle `/` key in `handleKey()` — calls `startSearch()` (only from diff pane)
- [ ] handle `searching` priority in `handleKey()` — check before help overlay, after annotation
- [ ] write tests for `startSearch`, `submitSearch`, `cancelSearch` behavior
- [ ] write tests for search input key handling (enter submits, esc cancels)
- [ ] run `make test` — must pass before task 3

### Task 3: Add search match navigation (n/N keys)

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] add `nextSearchMatch()` method — advances `searchCursor` with wrap, moves `diffCursor`, syncs viewport
- [ ] add `prevSearchMatch()` method — same but backwards
- [ ] modify `n` key handling: when `len(m.searchMatches) > 0`, call `nextSearchMatch()` instead of `tree.nextFile()`
- [ ] add `N` key handling in `handleKey()`: when `len(m.searchMatches) > 0`, call `prevSearchMatch()`
- [ ] write tests for next/prev match navigation including wrap-around
- [ ] write tests that `n` falls through to next-file when no search active
- [ ] write test that `N` does prev match when search active
- [ ] run `make test` — must pass before task 4

### Task 4: Highlight matching lines in diff rendering

**Files:**
- Modify: `ui/diffview.go`
- Modify: `ui/collapsed.go`
- Modify: `ui/model_test.go`

- [ ] add `searchMatchSet map[int]bool` field to Model, computed in `renderDiff()` and `renderCollapsedDiff()` before line iteration
- [ ] in `renderDiffLine`: when line index is in `m.searchMatchSet`, apply `m.styles.SearchMatch` background to content (after syntax highlight, before horizontal scroll/wrap — highlight propagates to all wrapped continuation rows automatically)
- [ ] in `renderCollapsedAddLine`: same search match highlight check and styling (before wrap if word wrap is active)
- [ ] ensure cursor line styling (`▶`) coexists with search highlight
- [ ] verify search highlight works correctly with word wrap active (all continuation rows highlighted)
- [ ] write tests verifying matched lines contain search highlight styling
- [ ] write tests verifying non-matched lines are unchanged
- [ ] run `make test` — must pass before task 5

### Task 5: Status line search indicator and search input display

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] add `if m.searching` branch in `statusBarText()` returning search input view with hints (before `inConfirmDiscard` check)
- [ ] add `[X/Y]` search position segment to status line when `len(m.searchMatches) > 0` (between hunk and mode icons)
- [ ] handle `searchMatches` in `statusSegmentsNoIcons()` and `statusSegmentsMinimal()` for narrow terminal degradation
- [ ] write tests for status bar during active search input
- [ ] write tests for `[X/Y]` display with various match counts
- [ ] run `make test` — must pass before task 6

### Task 6: Clear search on file change

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] add `clearSearch()` method that resets `searchTerm`, `searchMatches`, `searchCursor`, `searchMatchSet`
- [ ] call `clearSearch()` in `handleFileLoaded` after setting `m.diffLines`
- [ ] write tests for search clearing on file load
- [ ] write tests for empty search submit clearing matches (already wired in Task 2's `submitSearch`)
- [ ] run `make test` — must pass before task 7

### Task 7: Update help overlay

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] add search entries to help overlay — either new "Search" section or under "Navigation"
- [ ] entries: `/` = search, `n` = next match, `N` = prev match
- [ ] update `n / p` entry to note search-active override
- [ ] write test verifying help overlay contains search key listings
- [ ] run `make test` — must pass before task 8

### Task 8: Verify acceptance criteria
- [ ] verify `/` opens search input in diff pane
- [ ] verify enter submits and jumps to first match
- [ ] verify esc cancels without searching
- [ ] verify `n`/`N` navigate matches with wrap-around
- [ ] verify `n` reverts to next-file when no search active
- [ ] verify all matches highlighted with search colors
- [ ] verify `[X/Y]` shown in status line
- [ ] verify search clears on file change
- [ ] verify `--color-search-fg`/`--color-search-bg` flags work
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`

### Task 9: [Final] Update documentation
- [ ] update README.md with `/` search, `n`/`N` navigation keybindings
- [ ] update `.claude-plugin/skills/revdiff/references/usage.md` with search keybindings
- [ ] update `.claude-plugin/skills/revdiff/references/config.md` with search color options
- [ ] update CLAUDE.md if new patterns discovered
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test search with various diff sizes (small file, large file with many hunks)
- test search with special regex characters in search term (should be literal match, not regex)
- test narrow terminal widths to verify `[X/Y]` segment degrades gracefully
- test with different color themes to verify search highlight visibility
- test collapsed mode with search active
