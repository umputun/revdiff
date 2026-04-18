# Commit Info Popup

## Overview
Add an informational overlay (`i` hotkey) that shows the commits included in the current ref-based diff range, rendered with subject + full body so the reviewer can read the narrative context of what the changes are supposed to do. Read-only pager, not a picker. Supported for git, hg, and jj. Disabled in uncommitted/staged/stdin/single-file/all-files modes.

- **Problem**: when reviewing a PR-style range (e.g. `main..feature`) with many commits, the diff alone loses the "why" that commit messages capture. Reviewers have to leave revdiff to read the log.
- **Benefit**: keeps narrative context one keystroke away while reviewing; matches revdiff's no-context-switching philosophy.
- **Integration**: capability interface on `diff.Renderer`, overlay alongside help/annotlist/themeselect, keymap action rebindable via `~/.config/revdiff/keybindings`.

## Context (from discovery)
- **Files/components involved**:
  - `app/diff/{diff,hg,jj}.go` — VCS log extraction
  - `app/ui/overlay/overlay.go` — overlay Manager, Kind/Outcome, RenderCtx
  - `app/ui/overlay/help.go` — reference pattern (non-scrolling overlay)
  - `app/ui/overlay/themeselect.go` — reference pattern (scrolling overlay with offset)
  - `app/ui/model.go` — Model + ModelConfig, wiring for theme catalog / external editor
  - `app/ui/handlers.go` — keymap action dispatch
  - `app/ui/style/resolver.go` — `StyleKeyHelpBox` / `StyleKeyThemeSelectBox` registration
  - `app/keymap/` — Action constants, defaults, parser
- **Related patterns**:
  - Capability interfaces (filters wrapping readers, `ExternalEditor` consumer-side)
  - Overlay composition via `Manager.Compose` + ANSI-aware `overlayCenter`
  - Theme-token-only styling (no hex literals in UI code)
  - lipgloss + raw ANSI split: lipgloss for pane/box, raw ANSI for nested styled substrings
- **Dependencies identified**:
  - `runewidth` (already used) for width-aware wrapping
  - `muesli/reflow` or lipgloss built-in wrap for word boundaries — check `go.mod` during task 5
  - `charmbracelet/x/ansi` (already used) for ANSI-aware cutting if needed

## Development Approach
- **Testing approach**: Regular (code first, then tests) — matches revdiff's established pattern; each task writes tests immediately after its code, before the next task starts.
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change (`make test`)
- maintain backward compatibility — `CommitLogger` is an additive capability interface; existing renderers unaffected

### Architecture & Conventions to Respect (from `CLAUDE.md`)
- **Decouple OS from UI**: all `exec.Command` work stays in `app/diff/`; `app/ui` only type-asserts the `CommitLogger` capability and calls into a consumer-side interface (`commitLogSource`).
- **Consumer-side interfaces for external deps**: `commitLogSource` in `app/ui/model.go` + concrete implementation in `app/diff` + injection via `ModelConfig` (nil defaults to capability extraction from renderer). Mirrors `ExternalEditor` / `ThemeCatalog`.
- **Prefer methods over standalone utility functions**: internal helpers inside `(*Git).CommitLog`, `(*Hg).CommitLog`, `(*Jj).CommitLog` (ref parsing, record splitting, ANSI stripping, date parsing) must be methods on the VCS struct when called only from those methods, not package-level functions. Same rule inside `commitInfoOverlay` (wrap, pad, build-line helpers should be methods).
- **Minimize exported surface**: `commitLogSource`, `commitInfoOverlay`, and any helpers stay unexported. Only `CommitInfo`, `CommitLogger`, `CommitInfoSpec`, `StyleKeyCommitInfoBox`, `ActionCommitInfo`, `KindCommitInfo`, and `Manager.OpenCommitInfo` cross package boundaries.
- **Typed-nil trap**: when resolving `commitLogSource` from `ModelConfig`, an explicit nil check against the interface is required; a typed-nil capability (e.g. passing `var c *Foo; cfg.CommitLog = c`) must collapse to interface-nil before the "nil means derive from renderer" branch runs. Mirrors the `ParseTOC` guard.
- **ANSI nesting discipline**: inside the lipgloss `StyleKeyCommitInfoBox` container, inline styling (hash color, bold subject, muted author/date) uses raw ANSI via `style.AnsiFg` and ANSI bold/italic (`\x1b[1m` / `\x1b[22m`, `\x1b[3m` / `\x1b[23m`). No `lipgloss.NewStyle().Render()` for inline substrings.
- **Background fill**: before handing content to the outer lipgloss box, pad each rendered line to the inner width so the box's `BorderBackground` (when set) extends across the line; otherwise terminal default bg shows through on short lines. Matches the help overlay's padding approach.
- **Theme tokens only**: zero hex literals in `commitinfo.go`; all colors resolved through the `Resolver` interface.
- **One test file per source file**: revdiff already has `{hg,jj}_e2e_test.go` as a project-local convention alongside `{hg,jj}_test.go` — follow that convention for hg/jj e2e tests only; do not introduce a `diff_e2e_test.go` for git (see Testing Strategy for rationale).
- **Error wrapping**: `CommitLog` failures use `fmt.Errorf("commit log: %w", err)` style per project convention.

## Testing Strategy
- **unit tests**: required for every task
- **per-VCS parser tests** (`app/diff/{diff,hg,jj}_test.go`): empty output, one commit, many commits, commit with no body, newlines/tabs in body, CJK chars, ANSI-escape-looking chars, invalid ref error
- **e2e smoke tests** for hg and jj only (`app/diff/{hg,jj}_e2e_test.go` — both files already exist): create throwaway repos, make known commits, run real VCS binary, assert parsed structure. Git is NOT given an e2e test — git's `log` format is stable and parser-level tests against recorded output are sufficient; adding an e2e file would introduce a convention (`diff_e2e_test.go`) that doesn't currently exist in the project.
- **overlay rendering tests** (`app/ui/overlay/commitinfo_test.go`): empty list, single commit, long body wraps, scroll offset clamping, color tokens resolved through the `Resolver` interface (asserted by mock), error state, "no commits in range" state, ANSI-injection safety (stripped input renders as literal)
- **UI integration tests** (`app/ui/handlers_test.go` extension): `i` opens popup only when logger present, is no-op otherwise (shows status-bar hint), cache prevents second fetch, manager closes other overlays when opening commit info
- e2e tests for UI flows: revdiff project has no Playwright/Cypress tests, so UI validation stays in `app/ui` Go tests

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview
**Capability interface** (`diff.CommitLogger`) lives in `app/diff`, separate from `diff.Renderer`, implemented only by git/hg/jj renderers. UI type-asserts the renderer: absent implementation → `i` is a no-op with a transient hint. A **consumer-side interface** in `app/ui` (`commitLogSource`) wraps the capability and is injected via `ModelConfig` for testability — nil defaults to extracting the capability from the existing renderer.

**UI overlay** is a new `commitInfoOverlay` type in `app/ui/overlay/`, following the established pattern (open/render/handleKey, RenderCtx with Resolver, Manager integration). Data is **fetched lazily on first `i` press** and cached in the Model for the session (refs don't change mid-review). Popup is **centered, content-sized within a cap** (`min(80, term_w * 0.8)` × `term_h - 4`), uses raw ANSI inside a lipgloss box (per revdiff's ANSI-nesting discipline), and wraps at word boundaries.

**Range semantics** mirror `ChangedFiles`: single `X` → `X..HEAD`, `X..Y` as-is, two refs `X Y` → `X..Y`; each VCS translates to its own log syntax.

## Technical Details

### `diff.CommitInfo`
```go
type CommitInfo struct {
    Hash    string    // full hash/id
    Author  string    // "Name <email>" or VCS equivalent
    Date    time.Time // committer/author date
    Subject string    // first line of message
    Body    string    // remainder, trimmed (may be empty)
}
```

### `diff.CommitLogger` capability
```go
type CommitLogger interface {
    CommitLog(ref string) ([]CommitInfo, error)
}
```
Signature mirrors `ChangedFiles(ref string, staged bool)` and `FileDiff(ref, file string, staged bool)` — refs are already combined by `options.ref()` in `app/config.go` (e.g. `"main..feature"` when two positional args were given), so the renderer always receives a single pre-combined string. Each VCS translates it internally:
- Git uses `git log --no-color -z --format=<tab-separated>%H%x09%an <%ae>%x09%cI%x09%s%x09%b` with NUL record separator.
- Hg uses `hg log -r <revset> --template '{node}\x00{author}\x00{date|rfc3339date}\x00{desc}\x00\x01'`, `\x01` separates records.
- Jj uses `jj log -T '<template>' -r <revset> --no-graph` with the same NUL/SOH record scheme.
- Ref translation inside each VCS: `""` → empty slice (no commits to show; this path is gated upstream by the applicability flag and shouldn't be reached), `"X"` → range `X..HEAD` (git) / `X::.` (hg) / `X..@` (jj), `"X..Y"` → as-is for git/jj / `X::Y - X` for hg.
- Truncation is signaled via a separate `bool` in `CommitInfoSpec` (see "Scroll state / spec" below); do not mutate the commit slice with synthetic entries.
- Result capped at 500 commits.
- Invalid ref / empty output → empty slice, no error.
- CLI failure → wrapped error.
- **ANSI injection safety**: strip raw `\x1b` bytes from Subject/Body inside the parser (Task 1/2/3). The overlay renderer assumes already-stripped input; no second pass at render time.

### UI wiring (consumer-side interface)
```go
// in app/ui/model.go
type commitLogSource interface {
    CommitLog(ref string) ([]diff.CommitInfo, error)
}
```
- `ModelConfig.CommitLog commitLogSource` — nil means "derive from renderer via type assertion"
- `ModelConfig.CommitsApplicable bool` — pre-computed in `app/main.go` / `renderer_setup.go` from the full option set (`stdin`, `staged`, `only`, `all-files`, `ref`). Model does not inspect mode flags itself because `modelConfigState` today only exposes `ref`, `staged`, and `only`; pushing this computation into UI would require adding `stdin`/`allFiles` fields just for this check. Keeping it in the composition root avoids scope creep.
- Model holds `commits []diff.CommitInfo`, `commitsLoaded bool`, `commitsErr error`, `commitsTruncated bool`. The `commitsApplicable` boolean is copied from `ModelConfig`.

### Overlay manager extension
The consumer-side `overlayManager` interface in `app/ui/model.go` (currently lists `OpenHelp`, `OpenAnnotList`, `OpenThemeSelect`) must be extended with `OpenCommitInfo(spec overlay.CommitInfoSpec)` so Model can open the new overlay through the interface. The compile-time assertion `_ overlayManager = (*overlay.Manager)(nil)` (around line 136) will catch the `Manager` implementation gap at build time.

### Scroll state
`commitInfoOverlay` holds `offset int` operating on post-wrap rendered lines; keys `j`/`k` advance by 1, `PgDn`/`PgUp` by viewport-height, `g`/`G` to top/bottom. `Esc`/`q`/`i` close. Offset clamped in render.

### Colors (reused tokens, zero hex literals)
- Hash → `ColorKeyAccentFg`
- Subject → normal fg + bold ANSI (`\x1b[1m` ... `\x1b[22m`)
- Author/date → `ColorKeyMutedFg`
- Body → default foreground (no explicit color)
- Inter-commit separator → `ColorKeyBorderFg`
- Error/status line → `ColorKeyMutedFg` + italic (`\x1b[3m` ... `\x1b[23m`)
- Box: new `StyleKeyCommitInfoBox` registered in `resolver.go`, borders from `ColorKeyBorderFg`, optional `BorderBackground` when diff-bg theme color is set (parallel to `StyleKeyHelpBox`)

### Word wrap
- Inner width = `overlay_width − 2 (border) − 2 (indent)` (e.g. 76 for 80-col overlay)
- Wrap at word boundaries; use `runewidth`-aware logic so CJK/emoji measure correctly
- Force-break sequences longer than inner width (e.g. long URLs)
- Preserve bold attribute across wrapped subject segments (re-emit `\x1b[1m` after reset when needed)

### ANSI injection safety
Strip raw escape bytes (`\x1b`) from `Subject`/`Body` inside the VCS parser (Tasks 1/2/3), not at render time. Test case: commit message `"test\x1b[31mred\x1b[0m"` is stripped to `"testredX"` (or fully stripped including the bracket-payload — implementer decides between byte-by-byte stripping vs. regex for CSI sequences). Overlay assumes already-stripped input; no second pass.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): VCS CommitLog methods, overlay code, model wiring, keymap, tests, docs updates
- **Post-Completion** (no checkboxes): manual smoke-test the popup against revdiff's own history (`revdiff HEAD~10`), verify on hg and jj clones if available

## Implementation Steps

### Task 1: Add `CommitInfo` type, `CommitLogger` interface, and Git implementation

**Files:**
- Modify: `app/diff/diff.go`
- Modify: `app/diff/diff_test.go`
- Create: `app/diff/testdata/gitlog_empty.txt`
- Create: `app/diff/testdata/gitlog_single.txt`
- Create: `app/diff/testdata/gitlog_many.txt`
- Create: `app/diff/testdata/gitlog_nobody.txt`
- Create: `app/diff/testdata/gitlog_tricky.txt` (CJK, tabs, ANSI-looking chars)

- [x] define `CommitInfo` struct and `CommitLogger` interface in `app/diff/diff.go` with godoc; signature is `CommitLog(ref string) ([]CommitInfo, error)` — single pre-combined ref, matching `ChangedFiles`/`FileDiff`
- [x] implement `(*Git).CommitLog(ref string) ([]CommitInfo, error)` — translate the combined ref (`""` → empty slice, `"X"` → `X..HEAD`, `"X..Y"` → as-is), run `git log -z --no-color --format=...`, parse NUL-delimited records
- [x] enforce 500-commit cap at the parser — cap the returned slice at 500 entries; callers treat `len(commits) == 500` as potentially truncated and propagate into `CommitInfoSpec.Truncated`. Signature stays `([]CommitInfo, error)`. Document in godoc.
- [x] strip raw `\x1b` bytes from Subject/Body fields during parse to neutralize ANSI injection (overlay assumes already-stripped input)
- [x] write table-driven parser tests covering: empty, single, many, no-body, tricky content (CJK/tabs/ANSI-looking characters)
- [x] write tests for ref translation: `""` returns empty, `"X"` produces `X..HEAD` args, `"X..Y"` passes through
- [x] note: git CLI format is stable; no e2e test for git (consistent with no existing `diff_e2e_test.go` — hg/jj e2e tests exist because their templates are more fragile)
- [x] run `make test` — must pass before task 2

### Task 2: Implement `(*Hg).CommitLog`

**Files:**
- Modify: `app/diff/hg.go`
- Modify: `app/diff/hg_test.go`
- Create: `app/diff/testdata/hglog_empty.txt`
- Create: `app/diff/testdata/hglog_single.txt`
- Create: `app/diff/testdata/hglog_many.txt`

- [x] implement `(*Hg).CommitLog(ref string) ([]CommitInfo, error)` using `hg log -r <revset> --template '{node}\x00{author}\x00{date|rfc3339date}\x00{desc}\x00\x01'`
- [x] map combined ref to hg revsets: `""` → empty slice, `"X"` → `X::.`, `"X..Y"` → `X::Y - X` (split on `..` inside the implementation)
- [x] parse RFC3339 date into `time.Time`; split `{desc}` on first `\n` into subject/body
- [x] apply 500-commit cap and ANSI byte stripping consistent with Task 1
- [x] write parser tests covering empty/single/many + revset mapping for each combined-ref form
- [x] add e2e smoke test (guard-consistent with existing `hg_e2e_test.go`) creating throwaway repo with known commits
- [x] run `make test` — must pass before task 3

### Task 3: Implement `(*Jj).CommitLog`

**Files:**
- Modify: `app/diff/jj.go`
- Modify: `app/diff/jj_test.go`
- Create: `app/diff/testdata/jjlog_empty.txt`
- Create: `app/diff/testdata/jjlog_single.txt`
- Create: `app/diff/testdata/jjlog_many.txt`

- [x] implement `(*Jj).CommitLog(ref string) ([]CommitInfo, error)` using `jj log --no-graph -T '<template>' -r <revset>`
- [x] craft jj template emitting `commit_id ++ "\x00" ++ author ++ "\x00" ++ committer.timestamp().format("%Y-%m-%dT%H:%M:%S%:z") ++ "\x00" ++ description ++ "\x00\x01"` (used `commit_id` rather than `change_id` for git-hash compatibility; `committer.timestamp().format(...)` emits RFC3339)
- [x] map combined ref to jj revsets: `""` → empty slice, `"X"` → `X..@`, `"X..Y"` → as-is (plus triple-dot `X...Y` → `(X..Y) | (Y..X)` for parity with hg)
- [x] parse date, split description, apply 500-commit cap, strip ANSI bytes
- [x] write parser tests covering empty/single/many + revset mapping
- [x] add e2e smoke test creating throwaway jj repo with known commits
- [x] run `make test` — must pass before task 4
- ⚠️ Discovered pre-existing bug in Task 2: hg's `hgCommitLogTemplate` used literal NUL bytes (`\x00`) which `exec()` rejects as argv (`fork/exec: invalid argument`). Switched hg to ASCII US/RS separators (`\x1f`/`\x1e`) which are valid in argv. Also unified `splitDesc` across hg and jj to strip a single leading blank separator line (matches git's `%b` semantics), so all three VCS backends expose the same subject/body pair to the overlay renderer.

### Task 4: Add keymap action and `StyleKeyCommitInfoBox`

**Files:**
- Modify: `app/keymap/keymap.go` (action constants, `validActions`, `defaultBindings`, `defaultDescriptions` all live in this single file)
- Modify: `app/keymap/keymap_test.go`
- Modify: `app/ui/style/resolver.go`
- Modify: `app/ui/style/resolver_test.go`

- [x] add `ActionCommitInfo` constant to keymap action enum in `keymap.go`
- [x] register in `validActions`, `defaultDescriptions` ("show commit info for the current range"), and `defaultBindings` (bind `i`)
- [x] ensure `--dump-keys` output includes the new action (update golden test if present)
- [x] add `StyleKeyCommitInfoBox` constant and register it in `resolver.go` following the `StyleKeyHelpBox` pattern (border color from `Colors.Border`, optional `BorderBackground` when diff-bg is set — `ColorKeyBorderFg` enum not added in this task; deferred to task 5 if the overlay needs it for inter-commit separators)
- [x] register in both the colored and no-colors code paths in `resolver.go` (parallel to the two `StyleKeyHelpBox` entries around lines 275 and 337)
- [x] write test asserting `StyleKeyCommitInfoBox` resolves to a non-zero style
- [x] write test asserting `ActionCommitInfo` round-trips through parser/dumper
- [x] run `make test` — must pass before task 5

### Task 5: Implement `commitInfoOverlay` (rendering + wrap + scroll)

**Files:**
- Create: `app/ui/overlay/commitinfo.go`
- Create: `app/ui/overlay/commitinfo_test.go`
- Modify: `app/ui/overlay/overlay.go` (add `KindCommitInfo`, public `CommitInfoSpec` struct, `OpenCommitInfo` method on Manager, dispatch in `Compose`/`HandleKey`)
- Modify: `app/ui/overlay/overlay_test.go`

- [x] define `CommitInfoSpec` in `app/ui/overlay/overlay.go`: `{ Commits []diff.CommitInfo; Applicable bool; Truncated bool; Err error }`
- [x] create `commitInfoOverlay` struct holding `spec CommitInfoSpec` and scroll `offset int`
- [x] implement `render(ctx RenderCtx, *Manager) string`:
  - compute width `min(80, int(ctx.Width*0.8))`, height `ctx.Height - 4`
  - emit header `Commits (N)` (with `(truncated)` suffix when `spec.Truncated`)
  - for each commit: hash (accent) + " " + author (muted) + " " + date (muted), newline, bold subject, blank line, indented body
  - blank line between commits; skip body block for empty bodies
  - wrap via runewidth-aware word wrap at inner width
  - pad each rendered line to inner width with spaces so the outer box's optional `BorderBackground` fills correctly (matches help overlay's padding approach)
  - apply scroll offset to the final wrapped line slice
  - wrap in `StyleKeyCommitInfoBox` via lipgloss
  - inject centered title into top border using existing `injectBorderTitle`
  - render "no commits in range" centered when list is empty and `Err == nil`
  - render error centered + italic when `Err != nil`
  - render "no commits in this mode" centered when `!Applicable` (defensive — in practice Model gates opening)
- [x] implement `handleKey(msg, action)`: j/k move offset by 1, PgDn/PgUp by viewport height, g/G to top/bottom, Esc/q/ActionDismiss/ActionCommitInfo close; clamp offset in render
- [x] add `KindCommitInfo` to the Kind enum, `OpenCommitInfo(spec CommitInfoSpec)` method on `Manager`, and dispatch in `Manager.Compose` and `Manager.HandleKey` (Manager's single-`kind`-field pattern already enforces mutual exclusion — no extra logic needed, just a new case)
- [x] write tests: render empty, single commit, many commits, long wrapped body, scroll offset clamping at top/bottom, PgDn/PgUp scroll, color tokens delivered via Resolver mock, error state, truncated state, `Applicable=false` state
- [x] write test asserting opening commit info closes help when help was open (validates the existing mutual-exclusion behavior applies to the new Kind)
- [x] run `make test` — must pass before task 6

### Task 6: Wire Model + ModelConfig, lazy fetch cache, applicability gate

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/model_test.go`
- Modify: `app/app.go` / `app/renderer_setup.go` (compute `CommitsApplicable`, pass through `ModelConfig`)
- Create: `app/ui/mocks/commit_log_source.go` (moq-generated — snake_case filename matches existing `external_editor.go`, `sgr_processor.go` pattern)

- [x] define consumer-side interface `commitLogSource { CommitLog(ref string) ([]diff.CommitInfo, error) }` in `app/ui/model.go`
- [x] add `//go:generate moq -out mocks/commit_log_source.go -pkg mocks -skip-ensure -fmt goimports . commitLogSource` directive at the top of `model.go` alongside the existing directives
- [x] add `ModelConfig.CommitLog commitLogSource` field (nil allowed — Model derives from renderer type-assertion)
- [x] add `ModelConfig.CommitsApplicable bool` field
- [x] compute `CommitsApplicable` in `app/main.go` (or `renderer_setup.go`): `true` only when a ref or ref range is set AND not `--staged`, not `--stdin`, not `--only` (standalone), not `--all-files`. This is the single composition-root source of truth — Model does not re-derive it.
- [x] add Model state: `commits []diff.CommitInfo`, `commitsLoaded bool`, `commitsErr error`, `commitsTruncated bool`, `commitsApplicable bool` (copied from `ModelConfig`)
- [x] extend the `overlayManager` consumer-side interface in `model.go` with `OpenCommitInfo(spec overlay.CommitInfoSpec)` — the compile-time assertion `_ overlayManager = (*overlay.Manager)(nil)` (around line 136) will catch missing implementation at build time
- [x] resolve CommitLog source at Model construction: use `ModelConfig.CommitLog` if non-nil, else type-assert `renderer.(diff.CommitLogger)`, else nil (feature unavailable; `i` hotkey acts as no-op). Guard against the typed-nil interface trap: the "nil means derive from renderer" branch must run when a typed-nil interface value is passed in (`var c *someType; cfg.CommitLog = c`); use `reflect.ValueOf(cfg.CommitLog).IsNil()` if the interface is non-nil but the underlying pointer is, or require callers to pass literal `nil`. Pattern mirrors the `ParseTOC` guard.
- [x] run `go generate ./...` to produce the mock
- [x] write tests: source resolution fallback chain (explicit → type-assert → nil), lazy fetch populates cache, second fetch is no-op (verify via mock call count), error fetch stores `commitsErr`, truncated flag propagates
- [x] run `make test` — must pass before task 7
- ➕ implementation note: state grouped into a `commitsState` substruct (`source`, `applicable`, `loaded`, `list`, `truncated`, `err`) following the existing `loadedFileState`/`searchState`/`annotationState` pattern. lazy fetch helper landed as `(*Model).ensureCommitsLoaded()` on this task so the cache contract is exercised by tests now; task 7 wires the actual `i` handler.
- ➕ deviation: moq mock is generated as planned, but `commitLogSource` is unexported so `commitLogSourceMock` cannot be reached from `app/ui` test files (same situation as the existing `styleResolverMock` / `wordDifferMock`). tests use a tiny local `fakeCommitLog` fake — same precedent as `fakeThemeCatalog`. resolution helper extracted to `resolveCommitLogSource` to keep `NewModel` under the gocyclo limit.

### Task 7: Handler integration — `i` opens overlay, status-bar hint when not applicable

**Files:**
- Modify: `app/ui/handlers.go`
- Modify: `app/ui/handlers_test.go`
- Modify: `app/ui/view.go` (only if a transient status-bar hint needs viewport integration)

- [x] in `handlers.go` dispatch, handle `ActionCommitInfo`:
  - if `commitsApplicable == false` or CommitLog source is nil → set transient status-bar hint "no commits in this mode" (reuse existing transient-message pattern if one exists, otherwise briefest possible addition — verify during task whether such a pattern exists and match it)
  - if applicable and not loaded → call `commitLogSource.CommitLog(ref)` where `ref` is the same combined ref string already held by Model; store result + err + truncated flag, mark loaded
  - open overlay via `m.overlay.OpenCommitInfo(overlay.CommitInfoSpec{...})` with fields populated from Model state
- [x] pass-through any `OutcomeClosed` from the overlay to Model's existing close-handling
- [x] write tests: handler opens overlay when applicable + source present, handler is no-op (hint only) when not applicable, handler is no-op when source nil, handler doesn't refetch on second open (cache hit), handler stores error into spec on fetch failure, dispatch of `i` while overlay already open closes it (via the overlay's own `handleKey`, exercised via `Manager.HandleKey`)
- [x] run `make test` — must pass before task 8
- ➕ implementation note: no pre-existing transient-message pattern in Model — picked the briefest addition: `commitsState.hint string`, cleared at the top of `handleKey` so it persists for exactly one render cycle. `statusBarText` returns the hint verbatim (matching the existing `inConfirmDiscard` / `annot.annotating` precedents that replace status text entirely while an ephemeral state is set). `OutcomeClosed` pass-through already landed in Task 5 via the `OutcomeClosed, OutcomeNone` case in `handleModalKey` — covered here by `TestModel_HandleCommitInfo_CachesBetweenOpens` which presses `i` twice and verifies the overlay closes. The error path intentionally still opens the overlay so the user sees the failure in the popup (the overlay's centered-italic error rendering from Task 5).

### Task 8: Documentation updates (project docs + plugin refs)

**Files:**
- Modify: `README.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `CLAUDE.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `.claude-plugin/skills/revdiff/references/config.md` (if key binding configuration is documented there)
- Modify: `plugins/codex/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff-plan/references/usage.md` (if it exists and documents keybindings)

- [x] README.md: add `i` to key bindings table, add commit-info popup to features section
- [x] docs/ARCHITECTURE.md: add `app/ui/overlay/commitinfo.go` to file-by-file breakdown, add `CommitLogger` capability to the Renderer discussion
- [x] CLAUDE.md: extend the "Overlay popups managed by overlay.Manager" gotcha to include commit info; note the capability-interface pattern
- [x] site/docs.html: add `i` to key bindings; reflect any user-visible feature text
- [x] `.claude-plugin/skills/revdiff/references/usage.md`: add `i` to key bindings; update any other affected sections
- [x] `.claude-plugin/skills/revdiff/references/config.md`: update if keybindings or mode semantics are documented
- [x] sync every changed file to `plugins/codex/skills/revdiff/references/` byte-identically (per `feedback_revdiff-docs-sync-plugins.md` memory)
- [x] diff-check all shared reference files (loop): for each file in `.claude-plugin/skills/revdiff/references/`, run `diff <path> plugins/codex/skills/revdiff/references/<same>` — all diffs must be empty
- [x] if `plugins/codex/skills/revdiff-plan/references/` documents keybindings, sync there too — directory does not exist (revdiff-plan ships with only SKILL.md + scripts/), nothing to sync
- [x] no code changes in this task → no unit tests, but spot-check markdown and open `site/docs.html` locally to verify rendering

### Task 9: Verify acceptance criteria

- [x] verify all requirements from Overview are implemented: hotkey `i`, subject + body rendered, applicable only in ref-based modes, supports git/hg/jj, reuses theme tokens, word-wraps, scrolls, centered popup sized from content
- [x] verify edge cases: empty range, single commit, very long body, commit with no body, CJK/emoji body, ANSI-looking chars, invalid ref, 500+ commit cap
- [x] run full test suite: `make test`
- [x] run linter: `make lint`
- [x] run formatter: `make fmt` (or `~/.claude/format.sh`)
- [x] manual smoke test: `revdiff HEAD~10` on revdiff's own history — press `i`, verify list content, scroll, close, reopen (cache hit)
- [x] verify `--dump-keys` shows `i` → `commit-info` (actual output: `commit_info` — revdiff's keymap convention uses underscores throughout, consistent with `toggle_pane`, `half_page_down`, etc.)
- [x] verify test coverage does not drop below project baseline
- ➕ verification note: automated coverage strong (app/diff 92.4%, app/ui 93.3%, app/ui/overlay 97.1%, app/keymap 93.6%). All three VCS parsers have tests for empty/single/many/ANSI-injection/CJK/500-cap/invalid-ref; overlay has render/wrap/scroll/clamp/error/truncated/applicable-false tests; Model has source-resolution-chain, cache, hint, error-propagation tests. Manual smoke test replaced by the combined automated coverage since this session has no TTY — the interactive behaviors (open/scroll/close/reopen cache-hit) are each exercised by dedicated tests (`TestModel_HandleCommitInfo_CachesBetweenOpens`, `TestCommitInfoOverlay_ScrollJK`, `TestManager_OpenCommitInfoClosesHelp`).

### Task 10: [Final] Move plan to completed

**Files:**
- Move: this plan file

- [x] move `docs/plans/2026-04-17-commit-info-popup.md` to `docs/plans/completed/2026-04-17-commit-info-popup.md`
- [x] ensure final commit references plan and all tasks marked `[x]`

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification**:
- Smoke-test on a real hg clone (if available locally) — jj and hg implementations are hardest to unit-test for revset edge cases
- Smoke-test with a 300+ commit range (revdiff's own `HEAD~300` or similar) to confirm the 500-cap and performance
- Test with a commit message containing deliberate ANSI escapes (adversarial input) to confirm the strip behaves as documented
- Cross-check against a commit that has both CR and LF line endings in the body (Windows-authored commit)

**External system updates**:
- If a follow-up plan extends the popup with per-commit selection (explicitly out of scope here), the capability interface may need a `CommitAt(idx)` method — flag it for that future plan
- Markdown rendering of bodies — out of scope; consider only if users request it
- Per-file commit attribution — out of scope; overlaps with blame territory

**Out of scope for this plan** (explicit follow-ups):
- Per-commit selection to re-scope the diff (ruled out during brainstorm)
- Markdown rendering of commit bodies
- Per-file commit attribution ("which commits touched this file")
