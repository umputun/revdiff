# In-Session Annotation Flush to Output File

## Overview
- Add an `O` key that writes the current review annotations to the configured `--output` file **without exiting** revdiff (issue #248), enabling a permanently-open review tab: annotate → `O` flush → hand file to an AI agent → `R` reload → continue in the same session.
- Mode-gated by the presence of `--output`: flush is only meaningful with a file destination (the TUI owns the terminal in alt-screen, so a mid-session write to stdout is impossible). With no `-o`, pressing `O` shows a hint and does nothing.
- Persistence lives in the annotation domain: `annotation.Store` already owns serialization (`FormatOutput`/`Load`), so it also owns writing itself to a file — `(s *Store) WriteFile(path string) error` does an atomic temp-file+rename (replacing the exit path's bare `os.WriteFile`, a truncate-then-write partial-read hazard against a concurrent AI reader). No new package, no interface, no injected dependency — the Model calls the store directly, and `main` reuses the same method at exit.

## Context (from discovery)
- files/components involved: `app/annotation/store.go` (+ `store_test.go`); `app/keymap/keymap.go` (+ test); `app/ui/model.go`, new `app/ui/output.go` (+ test), `app/ui/view.go`, `app/ui/mouse.go`; `app/main.go` (+ test); docs (`README.md`, `site/docs.html`, `.claude-plugin/skills/revdiff/references/config.md` + `usage.md` + `SKILL.md`, `plugins/codex/skills/revdiff/SKILL.md`, `plugins/pi/skills/revdiff/SKILL.md`); `CLAUDE.md` gotcha.
- related patterns found:
  - Domain package owns its own file write: `app/history` does `os.WriteFile` inside `history.Save` (`history/history.go:79`); `main` calls it at exit, the Model is not involved. Persistence lives in the domain, not behind a UI-injected interface.
  - `annotation.Store` is the serialization domain: `FormatOutput() string` (`store.go:142`), `Load(io.Reader) error` (`store.go:121`), `Count() int` (`store.go:81`), `Clear()` (`store.go:90`). `WriteFile` is the natural persist-to-path counterpart.
  - Exit-time output: `app/main.go:258` `m.Store().FormatOutput()` → `writeAnnotationOutput` (`main.go:274-286`) does `os.WriteFile(opts.Output, …, 0o600)` or `fmt.Fprint(stdout, …)`; empty output skips the write entirely (`main.go:259-261`).
  - Plain config values reach a live Model through `modelConfigState` (`model.go:289`, holds `ref`, `staged`, `only`, `workDir`, `noStatusBar`, …) — set once in `NewModel`, never mutated. A new `outputPath string` there is a plain value like the rest, NOT an injected dependency.
  - Transient status feedback: `reloadState.hint` set in `handleReload` (`model.go:1122-1138`); rendered by the priority switch in `transientHint()` (`view.go:206-220`); cleared alongside other hints in `handleKey` (`model.go:912-916`) and `mouse.go:156-158`.
  - Keybinding surface: single action-descriptions table (`keymap.go:~180-245`, `{Action, description, section}`) feeds BOTH the help overlay (`KeysFor`, `keymap.go:419`) and `Dump` (`keymap.go:456`); default bindings map (`keymap.go:249-302`). `O` is currently unbound (`o` is also free; `e`/`ctrl+e` are the editor keys).
  - `R` reload deliberately clears the store on reload (`applyReloadCleanup` → `store.Clear()`, `model.go:1080`) with a "Annotations will be dropped" confirm — so an in-session flush is the only way to persist annotations without quitting.
- dependencies identified: `annotation.Store.FormatOutput()` is pure and repeat-safe; `Store.Count()` gives the annotation count for the hint.
- design decision (supersedes the earlier interface/injection sketch): NO consumer-side `AnnotationWriter` interface, NO moq mock, NO `app/output` package, NO `NewModel` dependency injection. The store persists itself; the Model holds the destination path as a plain config value.

## Development Approach
- **testing approach**: Regular (code first, then tests, before moving to the next task).
- complete each task fully before moving to the next
- make small, focused changes
- every code-changing task includes new/updated tests as separate checklist items
- all tests must pass before starting the next task
- update this plan when scope changes
- maintain backward compatibility: default behavior (no `-o`, exit-time output) is unchanged except that the exit-time file write becomes atomic

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
   - Every new exported symbol has a godoc comment starting with its name; unexported items get a lowercase non-godoc comment or none.
5. Only after 1–4 pass: mark the task complete.

If a previous task shipped a violation (spotted later by user, reviewer, or yourself): fix it in the next commit BEFORE starting the next task. Do not let violations accumulate.

**Project-specific stricter addendum:** In this repository, count `ctx context.Context` toward the 4-parameter limit from CLAUDE.md. Do not use the `ctx`-exempt exception in the canonical block above when implementing this plan.

## Testing Strategy
- **unit tests**: required for every task. `Store.WriteFile` (success overwrite of existing file, empty content, target-dir-missing → error, resulting mode 0o600, no leftover temp file on success, temp removed on error, byte-identical to `FormatOutput()`); keymap default binding + valid action + Dump/help entry; `handleFlushOutput` (empty path → hint + no write, empty store → "no annotations" hint + no write, success → file matches `FormatOutput()` + hint with count, write error → "flush failed" hint); dispatch routing of `ActionFlushOutput`; hint cleared on next key; `transientHint()` includes the new hint at the right priority; `writeAnnotationOutput` file branch now routes through the atomic store write.
- **e2e tests**: none — revdiff has no browser/UI e2e harness; the TUI is exercised via `Model` unit tests.
- No mock needed: tests write to a real `t.TempDir()` path and read the file back (the error path is exercised with a non-existent directory), so `Store.WriteFile` and `handleFlushOutput` are covered without introducing an interface.

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with `➕` prefix
- document issues/blockers with `⚠️` prefix
- update plan if implementation deviates from original scope

## Solution Overview
- `annotation.Store` gains `WriteFile(path string) error`: it formats itself via `FormatOutput()` and writes atomically — a temp file in the target's directory, then `os.Rename` over the target (mode 0o600). Atomic on a single filesystem, so a concurrent reader sees either the old or the new complete file, never a truncated one. The temp+rename logic is inline in the method (called only there); no standalone helper.
- The Model holds the destination as a plain value: `modelConfigState.outputPath`, set from `ModelConfig.OutputPath` in `NewModel` (a plain string copy, exactly like `ref`/`only`/`workDir`). `outputPath == ""` IS the mode gate — no interface, no injected writer, no applicability bool.
- A new `ActionFlushOutput` keymap action (default `O`) dispatches to `Model.handleFlushOutput`, which flushes via `m.store.WriteFile(m.cfg.outputPath)` and reports through a new `outputState.hint` shown in the status bar.
- `app/main.go` sets `ModelConfig.OutputPath: opts.Output`; the exit-time `writeAnnotationOutput` file branch calls `m.Store().WriteFile(opts.Output)` instead of `os.WriteFile`, unifying on the atomic write. The stdout branch, empty-output skip, and exit-code logic stay at the composition root.
- Flush is a pure export: `handleFlushOutput` never mutates the store, so annotations persist in-session after `O` and can be re-flushed (each flush overwrites the file with the full current set — a snapshot, not an append log). Clearing annotations remains exclusively `R` reload's job (`store.Clear()`, `model.go:1080`, behind its `y` confirm).
- **Accepted behavior (by design, not a follow-up):** because the store carries no "flushed" state, `R` reload still shows its "Annotations will be dropped — press y to confirm" prompt even immediately after an `O` flush. This is intentional: the warning is harmless (the flushed file already holds the annotations) and a "dirty-since-flush" flag to suppress it is deliberately out of scope.

## Technical Details

`app/annotation/store.go` (method on the existing `*Store`, so no Design Contract required):
```go
func (s *Store) WriteFile(path string) error   // FormatOutput() + atomic temp-file + rename over path, 0o600
```
`WriteFile`: `content := s.FormatOutput()`; `os.CreateTemp(filepath.Dir(path), ".revdiff-output-*")` (mode 0600 by default), write content, close, `os.Rename(tmp, path)`; remove the temp file and wrap with context on any error. Adds `os`/`path/filepath` imports to the annotation package (already the pattern in `app/history`).

`app/ui` (new `app/ui/output.go` for the handler + `outputState`; plain fields in `model.go`):
```go
type outputState struct { hint string }              // unexported, mirrors reloadState

func (m Model) handleFlushOutput() (tea.Model, tea.Cmd)
```
`handleFlushOutput` (value receiver, returns modified `m` — matches `handleReload`):
- `m.cfg.outputPath == ""` → `m.output.hint = "Output flush requires -o/--output"`; return.
- `m.store.FormatOutput() == ""` → `m.output.hint = "No annotations to flush"`; return (matches exit-time empty-skip).
- `m.store.WriteFile(m.cfg.outputPath)` error → `log.Printf("[WARN] flush annotations to output: %v", err)`; `m.output.hint = "Flush failed"`; return.
- success → `m.output.hint = fmt.Sprintf("Wrote %d %s to output file", n, noun)` where `n := m.store.Count()` and `noun` is `"annotation"`/`"annotations"` (inline `if n == 1`; no shared helper).

`model.go`: add `OutputPath string` to `ModelConfig`; add `outputPath string` to `modelConfigState` and `output outputState` to `Model`; set `outputPath: cfg.OutputPath` in the `modelConfigState` literal in `NewModel`.

`app/main.go`:
```go
// ModelConfig{ …, OutputPath: opts.Output }
```
`writeAnnotationOutput` file branch: replace `os.WriteFile(r.opts.Output, []byte(r.output), 0o600)` with `r.store.WriteFile(r.opts.Output)` (thread the store into `annotationOutputReq`, or call `m.Store().WriteFile(opts.Output)` at the callsite and keep `writeAnnotationOutput` for stdout+exit-code only). Keep the stdout branch, empty-output skip, and exit-code logic.

## What Goes Where
- **Implementation Steps** (`[ ]`): all code, tests, and docs in this repo.
- **Post-Completion** (no checkboxes): manual permanent-tab workflow verification; plugin/package version bumps deferred to the binary release (see note).

## Implementation Steps

### Task 1: Store-owned atomic persistence

**Files:**
- Modify: `app/annotation/store.go`
- Modify: `app/annotation/store_test.go`

- [x] add `(s *Store) WriteFile(path string) error` — `FormatOutput()` + atomic temp-file+rename over `path` (0o600, temp cleaned up on error), temp+rename inline (no standalone helper)
- [x] add godoc starting with `WriteFile`
- [x] write tests: success overwrite of existing file; empty store writes an empty file; target directory missing → error; resulting mode is 0o600; no leftover temp file after success; error path removes the temp file; written bytes equal `FormatOutput()`
- [x] run `go test ./app/annotation/... -race` — must pass before next task

### Task 2: Keymap action `flush_output` + default `O` binding

**Files:**
- Modify: `app/keymap/keymap.go`
- Modify: `app/keymap/keymap_test.go`

- [x] add `ActionFlushOutput Action = "flush_output"` constant
- [x] add `ActionFlushOutput: true` to the `validActions` map
- [x] add `{ActionFlushOutput, "flush annotations to output file", "Annotations"}` to the action-descriptions table (feeds both help overlay and `Dump`)
- [x] add `"O": ActionFlushOutput` to `defaultBindings()`
- [x] add `{"O", ActionFlushOutput}` to the `TestDefault_allExpectedBindings` table (`keymap_test.go`) — it ends with `assert.Len(t, km.bindings, len(tests))`, so a new binding without a table entry breaks the count assertion
- [x] write/extend tests: `O` resolves to `ActionFlushOutput`; action is valid; description/Dump line present
- [x] run `go test ./app/keymap/... -race` — must pass before next task

### Task 3: Model output-path config value + outputState

**Files:**
- Modify: `app/ui/model.go`
- Create: `app/ui/output.go`
- Create: `app/ui/output_test.go`

- [ ] add `OutputPath string` to `ModelConfig` with a doc comment consistent with surrounding `ModelConfig` fields
- [ ] add `outputPath string` to `modelConfigState` and set `outputPath: cfg.OutputPath` in the `NewModel` `modelConfigState` literal
- [ ] add `output outputState` field to `Model`; add `type outputState struct { hint string }` to `app/ui/output.go`
- [ ] write tests: `NewModel` copies `OutputPath` into `m.cfg.outputPath` (present and empty cases)
- [ ] run `go test ./app/ui/... -race` — must pass before next task

### Task 4: handleFlushOutput handler, dispatch, hint render + reset

**Files:**
- Modify: `app/ui/output.go`
- Modify: `app/ui/output_test.go`
- Modify: `app/ui/model.go` (dispatch case + hint reset)
- Modify: `app/ui/view.go` (`transientHint` case + comment)
- Modify: `app/ui/mouse.go` (hint reset)

- [ ] implement `handleFlushOutput` (empty-path hint; empty-store hint + no write; success hint with count; write error → `[WARN]` log + hint) calling `m.store.WriteFile(m.cfg.outputPath)`
- [ ] add `case keymap.ActionFlushOutput: return m.handleFlushOutput()` to the action switch (`model.go:~1008`)
- [ ] add `m.output.hint` case to `transientHint()` (after `reload`) and update its priority comment (keep it lowercase, non-godoc — `transientHint` is unexported)
- [ ] add `m.output.hint = ""` to the hint-reset blocks in `handleKey` (`model.go:~912-916`) and `mouse.go` (`~156-158`)
- [ ] write tests (real store + `t.TempDir()`): empty path → hint only, no file; empty store → "No annotations to flush", no file; success → file content equals `store.FormatOutput()`, hint has correct count/pluralization; write error (bad dir) → "Flush failed" hint; dispatch of `ActionFlushOutput` reaches the handler; hint appears in `transientHint()` and clears on next key
- [ ] run `go test ./app/ui/... -race` — must pass before next task

### Task 5: Unify exit-time write on the atomic store write + main wiring

**Files:**
- Modify: `app/main.go`
- Modify: `app/main_test.go`

- [ ] set `ModelConfig.OutputPath: opts.Output` in the `NewModel` call
- [ ] replace `os.WriteFile(...)` in the `writeAnnotationOutput` file branch with the store's atomic write (`m.Store().WriteFile(opts.Output)` at the callsite, or thread the store into `annotationOutputReq`); keep stdout branch, empty-output skip, and exit-code logic
- [ ] write/adjust tests: exit-time file write still produces identical content and still returns the right exit code; the write is atomic (no partial file); `OutputPath` empty when `-o` absent, set when present
- [ ] run `go test ./app/... -race` — must pass before next task

### Task 6: Documentation and plugin surfaces

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/references/config.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `.claude-plugin/skills/revdiff/SKILL.md`
- Modify: `plugins/codex/skills/revdiff/SKILL.md`
- Modify: `plugins/pi/skills/revdiff/SKILL.md`
- Modify: `CLAUDE.md`

- [ ] document the `O` key (flush annotations to `--output` file without exiting; requires `-o`) in README keybindings table and `site/docs.html` (keep the two in sync)
- [ ] add the key to `references/usage.md` key-bindings list and note the flush/`-o` relationship in `references/config.md`
- [ ] update the three `SKILL.md` files: teach agents that a human reviewer may keep revdiff open and flush with `O`; do NOT add new launcher flags (launchers are block-until-exit and unaffected)
- [ ] add a `CLAUDE.md` gotcha entry: `O` flush is gated on `m.cfg.outputPath` (from `-o`), persists via `annotation.Store.WriteFile` (atomic temp+rename, shared with the exit-time write), reports via `outputState.hint`; empty path = disabled
- [ ] verify no stray "at startup"/toggle-cross-reference phrasing per the CLI-description style rule (this is a runtime key, not a flag — keep it in the keybindings table only)
- [ ] **DO NOT** bump `plugin.json` / `marketplace.json` / `package.json` — this depends on a new binary feature not yet released (plugin-version-bump-deferral rule)

### Task 7: Verify acceptance criteria and finalize
- [ ] verify Overview requirements: `O` flushes to `-o` file without exiting; hint feedback; no-op-with-hint when `-o` absent; exit-time write is atomic and byte-identical to before
- [ ] manual smoke: `revdiff -o /tmp/a.md`, add an annotation, press `O`, confirm `/tmp/a.md` contents match a normal quit's output while revdiff stays open; press `O` with no annotations (hint, no file clobber); run without `-o` and press `O` (hint only)
- [ ] run full suite: `make test` and `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [ ] run `~/.claude/format.sh`
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification:**
- End-to-end permanent-tab loop with a real AI agent: keep revdiff open, `O` to export, have the agent read the file and edit code, `R` to reload, confirm the loop is smoother than quit+relaunch.

**External / release coordination:**
- Plugin (`.claude-plugin`) and Pi (`package.json`) version bumps happen at the binary release, AFTER `revdiff` is tagged with this feature — never on this feature branch. Shipping an updated launcher/skill ahead of the binary would advertise `O` to users on a binary that lacks it.

---
Smells pre-check: passed (carried forward — signatures are a strict simplification of the prior smells-passed revision: `Store.WriteFile(path string) error` and `handleFlushOutput() (tea.Model, tea.Cmd)` are within the 4-param/4-return limits, temp+rename stays inline as a method-local block (no standalone helper), `Store.WriteFile`/`ModelConfig.OutputPath` exports have cross-package callers, `outputState` is unexported; no interface or moq mock in this design).
