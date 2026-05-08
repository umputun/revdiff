# In-Session Search History

## Overview
- adds in-session recall of previously submitted `/`-search queries inside the search prompt.
- pressing `Up` / `Ctrl+P` walks backward through past queries; `Down` / `Ctrl+N` walks forward; `Down` past the newest entry clears the input.
- closes [issue #170](https://github.com/umputun/revdiff/issues/170): "Search history is missing".
- history is in-memory only — not persisted across revdiff invocations. No CLI flags, no new config keys, no new files.

## Context (from discovery)
- files/components involved:
  - `app/ui/model.go` — `searchState` struct (lines 301-309) holds search lifecycle fields; two new fields go here.
  - `app/ui/search.go` — `startSearch`, `submitSearch`, `clearSearch`, `handleSearchKey` are the touchpoints; one new helper method is added.
  - `app/ui/search_test.go` — paired test file per the project's "one test file per source file" rule.
- related patterns found:
  - `searchState` already groups all search lifecycle state (`active`, `term`, `matches`, `cursor`, `input`, `matchSet`); history fits naturally as more state on the same struct.
  - `handleSearchKey` already intercepts specific `tea.KeyType` values (Enter, Esc) before forwarding to the embedded `textinput.Model`; the same pattern works for Up/Down/Ctrl+P/Ctrl+N.
  - other state-grouping structs in the file follow the same shape (`navigationState`, `commitsState`, `reloadState`), so adding fields stays consistent with the convention.
- dependencies identified: none. `bubbles/textinput` already provides `SetValue` and `CursorEnd`; `bubbletea` already exposes `tea.KeyUp`, `tea.KeyDown`, `tea.KeyCtrlP`, `tea.KeyCtrlN`.

## Development Approach
- **testing approach**: Regular (code first, then tests within the same task)
- complete each task fully before moving to the next.
- make small, focused changes.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task.
  - tests are not optional — they are a required part of the checklist.
  - write unit tests for new functions/methods.
  - write unit tests for modified functions/methods.
  - add new test cases for new code paths.
  - update existing test cases if behavior changes.
  - tests cover both success and error scenarios.
- **CRITICAL: all tests must pass before starting next task** — no exceptions.
- **CRITICAL: update this plan file when scope changes during implementation**.
- run tests after each change.
- maintain backward compatibility — search behavior without using Up/Down is byte-identical to current.

## Testing Strategy
- **unit tests**: required for every task (see Development Approach).
- **e2e tests**: project has no UI-based e2e suite; not applicable.
- run via `go test ./app/ui/...` (or the full `go test ./...` at task gates).
- race detector run at the final acceptance task: `go test -race ./...`.

## Progress Tracking
- mark completed items with `[x]` immediately when done.
- add newly discovered tasks with ➕ prefix.
- document issues/blockers with ⚠️ prefix.
- update plan if implementation deviates from original scope.
- keep plan in sync with actual work done.

## Solution Overview

**Approach: extend `searchState` with two fields and intercept history-recall keys in `handleSearchKey` before delegating to the textinput.**

This is the smallest design that keeps the existing concern-grouping intact:
- search lifecycle stays in one struct (`searchState`) and one source file (`search.go`).
- no new package, no new file, no new exported surface.
- the textinput is still the source of truth for the typed buffer; history navigation just calls `SetValue` on it.

Rejected alternatives (from brainstorm):
- nested `searchHistoryState` substruct — over-structured for two fields and three trivial operations.
- separate `app/ui/searchhistory/` package — pure in-memory state with no OS boundary, so the `app/editor/` precedent does not apply; would be overengineering per CLAUDE.md "Minimize exported surface" and "prefer simple and focused solutions".

## Technical Details

**State changes** (`app/ui/model.go`, on `searchState`):
- `history []string` — submitted queries, oldest-first.
- `historyIdx int` — current position; `len(history)` represents the "no recall active" state (input shows whatever the user typed or empty).

**Constants** (`app/ui/search.go`):
- `searchHistoryMax = 50` — bound on retained queries; oldest entries are dropped when the cap is exceeded.

**Behavior contract:**

1. **`startSearch`** — after creating the textinput, set `m.search.historyIdx = len(m.search.history)`. This makes each new `/` invocation start at the "draft" position regardless of how the previous search ended (submit, Esc, focus loss).

2. **`submitSearch`** — append happens **immediately after the empty-input clear-and-return guard, before the match scan** (`m.search.term = strings.ToLower(query)`). This placement matters: zero-match queries must still be recallable so the user can edit and retry. The three return paths that follow (no matches, no visible match, match found) all preserve the appended history. Append logic:
   - skip the append if `history[len-1] == query` (consecutive-duplicate dedup, less-style).
   - if the cap is exceeded, slice off the oldest entry: `history = history[len(history)-searchHistoryMax:]`.
   - reset `historyIdx = len(history)` so the next `/` starts fresh at the draft slot.
   - re-submitting an older entry (recalled, then Enter) appends it again, moving it to "most recent".

3. **`handleSearchKey`** — add cases before the `default:` forward:
   - `tea.KeyUp`, `tea.KeyCtrlP` → `m.recallHistory(-1)`, return without forwarding.
   - `tea.KeyDown`, `tea.KeyCtrlN` → `m.recallHistory(+1)`, return without forwarding.

4. **`recallHistory(direction int)`** — new helper, executed in this order:
   - **first**: short-circuit if `len(m.search.history) == 0` (no-op).
   - clamp `m.search.historyIdx + direction` to `[0, len(history)]`.
   - if the clamped index equals `len(history)`: `input.SetValue("")`.
   - else: `input.SetValue(history[clampedIdx])`.
   - call `input.CursorEnd()` to place the cursor at end of recalled text.
   - update `m.search.historyIdx`.

   `recallHistory` is a method on `*Model`. It is invoked from value-receiver `handleSearchKey(m Model, ...)` via standard Go auto-addressing of the local copy (`m.recallHistory(...)` becomes `(&m).recallHistory(...)`); mutations land on that copy and are returned through `return m, nil`. This is the same pattern already used by `submitSearch`/`startSearch`/`cancelSearch`. **Do not "fix" this receiver inconsistency by changing `handleSearchKey` to a pointer receiver — bubbletea's update flow expects value semantics.**

5. **`clearSearch` is intentionally not modified.** It resets `term`, `matches`, `cursor`, `matchSet` after Esc-without-submit or empty-input submit. History must survive those resets — it spans the full session, not a single search. Do not add `history = nil` or `historyIdx = 0` to `clearSearch`.

**Edge cases handled:**
- empty history + Up/Down → no-op (clamp + len-zero guard).
- Up at oldest entry → stays at index 0.
- Down at "draft" position → stays there.
- recalled-then-cancelled (Esc) → next `/` starts fresh; the recalled value is **not** appended (only `submitSearch` appends).
- recalled-then-submitted → appended via the dedup-append path; if it was already most-recent, dedup skips the append.

**Compatibility:** users who never press Up/Down see byte-identical behavior to the current implementation — `textinput.Update` was a no-op for those keys anyway, so reclaiming them does not regress anything.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): all code, tests, and acceptance verification.
- **Post-Completion** (no checkboxes): manual smoke (open revdiff, run a few searches, exercise Up/Down/Ctrl+P/Ctrl+N).

## Implementation Steps

### Task 1: Add history state and recall behavior to search

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/search.go`
- Modify: `app/ui/search_test.go`

- [x] add `history []string` and `historyIdx int` fields to `searchState` in `app/ui/model.go`, with comments documenting the "draft = `len(history)`" convention.
- [x] add `searchHistoryMax = 50` const at the top of `app/ui/search.go`.
- [x] update `startSearch` in `app/ui/search.go` to reset `m.search.historyIdx = len(m.search.history)` after creating the textinput.
- [x] update `submitSearch` in `app/ui/search.go` to dedup-append the trimmed query (skip if equal to last entry), enforce the cap by re-slicing from the tail, and reset `historyIdx` to `len(history)` on the success path.
- [x] add `recallHistory(direction int)` method on `*Model` in `app/ui/search.go` that clamps `historyIdx`, calls `input.SetValue` (empty at draft slot, else `history[idx]`), and `input.CursorEnd`.
- [x] update `handleSearchKey` in `app/ui/search.go` to intercept `tea.KeyUp`/`tea.KeyCtrlP` and `tea.KeyDown`/`tea.KeyCtrlN` before the `default:` forward, calling `recallHistory` with `-1` / `+1`.
- [x] write tests in `app/ui/search_test.go`:
  - submitting unique queries appends to history, idx resets to `len(history)`.
  - submitting a query with **zero matches** still appends to history (verifies append placement is before the match scan).
  - consecutive duplicate submit does not grow history.
  - resubmitting an older entry appends it (moves to most recent).
  - cap enforced: pushing 51 entries drops the oldest.
  - Up recalls most recent, then older with subsequent presses.
  - Up at oldest is a no-op (idx stays at 0, input keeps the oldest entry).
  - Down past newest clears the input and idx == len(history).
  - Ctrl+P / Ctrl+N parity with Up / Down (single shared assertion path is fine).
  - `startSearch` resets `historyIdx` to `len(history)` even when called after a partial recall (simulate by setting historyIdx to 0 then re-entering).
  - **recall-then-Esc transition**: enter search, press Up to recall, press Esc, press `/` again — input must be empty (draft slot) and `historyIdx == len(history)`.
  - empty-input submit (whitespace only) does not append to history.
  - `clearSearch` does not touch history fields (history slice and idx survive a `clearSearch` call).
- [x] run `go test ./app/ui/...` — must pass before next task.

### Task 2: Verify acceptance criteria
- [x] verify each Overview bullet is implemented:
  - Up / Ctrl+P walks backward
  - Down / Ctrl+N walks forward
  - Down past newest clears input
  - in-session only (no persistence file created, no config key added)
- [x] verify edge cases from Technical Details (empty history, oldest, cap, dedup, cancel-doesn't-append).
- [x] run full test suite: `go test ./...`.
- [x] run race detector: `go test -race ./...`.
- [x] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` from repo root.
- [x] run formatters: `~/.claude/format.sh` (or gofmt/goimports if not available).
- [x] verify no new files were created (per design — additive change only).

### Task 3: [Final] Update documentation and finalize

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff/references/usage.md`
- Move: this plan to `docs/plans/completed/`

Concrete docs-sync edits — none of these are conditional. The new behavior must appear in all four files in lockstep, and the two `usage.md` copies must remain byte-identical (md5-verified).

- [x] **README.md** — locate the search keybindings table (around lines 628-631 — `/`, `n`, `N`, `Esc` rows) and add two new rows describing the in-prompt recall keys. Suggested wording (table cells stay short):
  - `↑ / Ctrl+P` — Recall previous search query (in search prompt)
  - `↓ / Ctrl+N` — Recall next search query / clear (in search prompt)
- [x] **site/docs.html** — locate the matching keybindings rows (around lines 480-482) and add the same two rows in the existing `<tr><td><code>...</code></td><td>...</td></tr>` shape, with prose matching the README.
- [x] **`.claude-plugin/skills/revdiff/references/usage.md`** — locate the search keybindings table (around lines 115-117) and add the same two rows.
- [x] **`plugins/codex/skills/revdiff/references/usage.md`** — apply the identical edit (the simplest path is to copy the modified `.claude-plugin/...` file over this one). Then verify with `diff` and `md5`:
   ```
   diff .claude-plugin/skills/revdiff/references/usage.md plugins/codex/skills/revdiff/references/usage.md && echo identical
   ```
   The check must print `identical` before this step is marked done.
- [x] no `site/index.html` version-badge change in this PR — that bump belongs to the release that ships this feature, not to the feature PR.
- [x] mark all checkboxes `[x]` and move this plan: `mkdir -p docs/plans/completed && mv docs/plans/20260507-search-history.md docs/plans/completed/`.

## Post-Completion
*Items requiring manual intervention or external systems — informational only*

**Manual verification** (recommended before merge):
- run `./.bin/revdiff` against any repo with a few changes; press `/`, type a query, Enter, then `/` again and press Up — previous query should appear with cursor at end.
- press Up several more times to confirm older queries surface and oldest is sticky.
- press Down to walk forward; one Down past the newest must clear the input.
- press Ctrl+P / Ctrl+N to confirm parity with Up / Down.
- press Esc mid-recall, then `/` again — the input must be empty, not the value that was being recalled when Esc fired.

**No external system updates** — feature is local to revdiff binary; no consuming projects, no deployment changes.
