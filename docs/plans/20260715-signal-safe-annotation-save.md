# Signal-safe annotation save + first-class tmux window mode (#262)

## Overview

revdiff loses all in-memory annotations when its process dies by an OS signal instead of quitting gracefully. Reported case (#262): a review runs in a `tmux display-popup`, the SSH connection drops (VPN expiry), the tmux server tears the popup down, and the revdiff process is killed by SIGHUP before `p.Run()` returns, so neither the history auto-save nor the output write ever runs.

Two independent fixes, both scoped to #262:

- **Part A (binary safety net):** on signal-delivered termination (SIGHUP/SIGTERM/SIGINT) save the current annotations to the history file only, never the `-o` output. This preserves the two existing invariants (the `-o` output is a deliberate "ready" handoff written only on `O`/graceful-quit; `Q` discard saves nothing) while making sure an abrupt disconnect no longer drops work. Recovery stays the existing "load newest from `~/.config/revdiff/history/`" flow.
- **Part C (launcher, promote existing):** window mode already exists (`REVDIFF_TMUX_WINDOW=1`, in `agentdeck-window.sh`) and already survives a client disconnect because a `tmux new-window` is server-owned. Promote it to a first-class interactive mode: when the user opts in (not agent-deck auto-detection), open the window focused and restore the prior window on exit, and document it as the disconnect-resilient mode. A live review in a server-owned window survives an SSH drop with zero loss — reattach brings it back running.

Scope notes:
- Skip continuous auto-save (option B): it only adds SIGKILL/power-loss coverage at real complexity and would blur the `O`/`Q` semantics.
- Interactive typed **Ctrl-C is out of scope**: bubbletea runs the TUI in raw mode (ISIG off), so a typed `^C` arrives as a keystroke, not SIGINT; revdiff binds no `ctrl+c` action, so it is currently a no-op (no quit, no loss). The guard's SIGINT catch only affects a *signal-delivered* SIGINT (`kill -INT`, non-TTY, or a cooked-mode window during an external-editor `tea.ExecProcess`), not typed `^C`.

## Context (from discovery)

- Composition root: `app/main.go` `run()` (lines 96-268). `p.Run()` at 248 blocks until graceful quit; `saveHistory` (266) and `writeAnnotationOutput` (268) run only after it returns. `m.Discarded()` at 258 already short-circuits both (that is `Q`).
- Vendored bubbletea installs its own SIGINT/SIGTERM handler (`vendor/.../bubbletea/tea.go:284`); a *signal-delivered* SIGTERM becomes a normal QuitMsg (the output-writing path), a *signal-delivered* SIGINT becomes `ErrInterrupted`. SIGHUP is unhandled. `tea.WithoutSignalHandler()` (`vendor/.../bubbletea/options.go:69`) disables this so the app can own all three.
- Persistence primitives: `saveHistory` (`app/history_save.go`) -> `history.New(...).Save(...)` (`app/history/history.go`); `writeAnnotationOutput` (`app/main.go:277`) -> `fsutil.AtomicWriteFile`. `app/history/history.go:79` uses plain `os.WriteFile`, not the atomic helper (latent robustness gap, in scope to fix).
- `annotation.Store` is an unsynchronized map (`app/annotation/store.go:22`) — must NOT be read from a signal goroutine.
- History dir is configurable via `--history-dir` / `REVDIFF_HISTORY_DIR` (`app/config.go:53`), so the finalize decision is testable against a temp dir; passing `GitRoot:""` skips the `git diff`/`git rev-parse` exec paths in `history.go`.
- Window backend already exists: `.claude-plugin/skills/revdiff/scripts/agentdeck-window.sh`, sourced by `launch-revdiff.sh:152`, gated on `REVDIFF_TMUX_WINDOW` (`=1` force window, `=0` force popup, unset=auto for agent-deck). It uses `tmux new-window -d` + a sentinel wait. Copied to `plugins/codex/skills/revdiff/scripts/agentdeck-window.sh` (keep both in sync, same as `detect-ref.sh`). Pi is direct-terminal, no tmux window backend.
- The single `_rd_winmode` var in `agentdeck-window.sh` collapses "user forced (`REVDIFF_TMUX_WINDOW=1`)" and "auto agent-deck detection" both to `1`; first-class focus/restore must gate on a *separate* flag captured only for the explicit opt-in.

## Development Approach
- **testing approach**: Regular (code first, then tests) — per planning decision
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task with Go code MUST include new/updated tests** for that task
  - unit tests for new/modified functions, both success and error scenarios
  - launcher (shell) tasks have no Go test harness — their verification is manual tmux runs, listed in Post-Completion
- **CRITICAL: all tests must pass before starting next task**
- run tests after each change
- maintain backward compatibility (default behavior unchanged when no signal fires and window mode not opted into)

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
- **unit tests**: required for every Go task (see Development Approach). Table-driven with testify, both success and error cases.
- `shutdownGuard.trigger` is testable with a fake quit func; `handle` is testable by feeding OS-signal values on a plain channel (no real process signal); `wasSignaled` from those. This avoids raising a real signal at the whole test process.
- `finalize` is testable against a temp `REVDIFF_HISTORY_DIR` + a `bytes.Buffer` stdout: assert which files/writes happen per branch (signal -> history only, no output; normal+`-o` -> history + output file; normal+no `-o` -> history + stdout buffer; discarded -> neither; empty -> neither).
- **e2e tests**: none (no UI e2e harness). Launcher window-mode and live SIGHUP behavior are verified manually in a real tmux (Post-Completion).

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

Part A adds a tiny `shutdownGuard` (`sync.Once` + `atomic.Bool`) at the composition root. `run()` appends `tea.WithoutSignalHandler()`, starts `guard.watch(program)` (installs the SIGHUP/SIGTERM/SIGINT handler that calls `Quit()` exactly once and flips the flag), and after `p.Run()` returns routes the finalize decision through `guard.wasSignaled()`. The store is never touched off the main goroutine; the flag is read only after the `p.Run()` join, so there is no race. `watch` takes a small consumer-side `quitter` interface (satisfied by `*tea.Program`) so the signal->quit wiring is unit-testable with a mock.

**Deliberate semantic change to call out (finding from review):** today a *signal-delivered* SIGTERM reaches bubbletea's handler -> QuitMsg -> the normal tail writes BOTH history and the `-o` output. After this change SIGTERM (like SIGHUP) is a safety-net save: history only, never `-o`. That is the intended invariant (a signal is not a deliberate handoff), but it IS a behavior change and must be documented.

The finalize decision is extracted from run()'s tail into `finalize(finalizeReq)` so the "signal -> history only, never output" invariant is unit-tested rather than buried in `run()`.

Part C is launcher-only: the window backend already survives disconnect; the change is to open the window focused (and restore the prior window on exit) when window mode is user-opted and not agent-deck, plus documentation. Both script copies (.claude-plugin, codex) change together.

**Rejected during review — explicit `exec` of revdiff in the launcher.** The idea was to make revdiff the deterministic signal target. It is unnecessary (on PTY hangup the kernel delivers SIGHUP to the whole foreground process group, and the popup's single-command `sh -c` already exec-chains to revdiff, so revdiff receives the signal today) and actively harmful: `exec $REVDIFF_CMD` would try to exec a program literally named `REVDIFF_EXIT_CODE_ON_ANNOTATIONS=true` (the leading inline env assignment at `launch-revdiff.sh:31`, since the `/usr/bin/env` prefix is only conditional), and `exec` on the window backend's `write_rc_cmd` compound (`agentdeck-window.sh:57`) would skip the `rc=$?; ... > sentinel` tail, breaking the wait loop. Dropped.

## Technical Details

`app/signal.go`:
```
type quitter interface{ Quit() }

type shutdownGuard struct {
    once     sync.Once
    signaled atomic.Bool
}
func (g *shutdownGuard) trigger(quit func())                     // once: signaled.Store(true); quit()
func (g *shutdownGuard) handle(ch <-chan os.Signal, quit func()) // range ch { trigger(quit) } — testable
func (g *shutdownGuard) watch(q quitter) (stop func())           // Notify HUP/TERM/INT; go handle(ch, q.Quit); stop = Stop+close
func (g *shutdownGuard) wasSignaled() bool                       // signaled.Load()
```

`app/main.go` finalize (data struct, no methods, sibling of existing `histReq`/`annotationOutputReq`):
```
type finalizeReq struct {
    opts      options
    output    string
    files     []string
    discarded bool
    gitRoot   string
    workDir   string
    signaled  bool
    stdout    io.Writer
}
func finalize(r finalizeReq) (int, error)
// if r.discarded || r.output == "" -> return 0, nil
// saveHistory(...)                          // always, safety net
// if r.signaled -> return 0, nil            // history only, no handoff
// return writeAnnotationOutput(...) using r.stdout
```

`run()` change: append `tea.WithoutSignalHandler()`; `guard := &shutdownGuard{}`; `stop := guard.watch(p); defer stop()`; replace the 254-268 tail with a single `finalize(finalizeReq{... signaled: guard.wasSignaled(), stdout: os.Stdout})` call.

Signal-originated exit returns code 0 (no annotations delivered over stdout — the consumer pipe is gone on a disconnect anyway; history holds the recovery copy).

## What Goes Where
- **Implementation Steps** (`[ ]`): Go code + Go tests + doc file edits in this repo.
- **Post-Completion** (no checkboxes): manual tmux verification of window mode and live SIGHUP behavior; binary release + plugin-version decisions (release-time only, per CLAUDE.md).

## Implementation Steps

### Task 1: Add shutdownGuard signal type

**Files:**
- Create: `app/signal.go`
- Create: `app/signal_test.go`

**Design Contract:**

Type:
- `shutdownGuard` (unexported — only referenced within package main; no out-of-package caller)
- `quitter` interface (unexported — consumer-side interface for `*tea.Program.Quit`, per CLAUDE.md "consumer-side interfaces for external deps")

Methods (full signatures):
- `(g *shutdownGuard) trigger(quit func())`
- `(g *shutdownGuard) handle(ch <-chan os.Signal, quit func())`
- `(g *shutdownGuard) watch(q quitter) (stop func())`
- `(g *shutdownGuard) wasSignaled() bool`

Fields:
- `once sync.Once`, `signaled atomic.Bool`

Standalone helpers planned (justification why NOT a method): none — `trigger`/`handle`/`watch` are all methods on `shutdownGuard`; `handle` is called only from `watch` (same struct) so it is correctly a method.

Exports (who outside the package calls this?): none

- [x] create `app/signal.go` with `quitter`, `shutdownGuard`, and its four methods
- [x] `trigger` uses `once.Do` to `signaled.Store(true)` then call `quit` (dedupes the HUP+TERM burst)
- [x] `handle` ranges the signal channel and calls `trigger(quit)` per signal
- [x] `watch` installs `signal.Notify` for SIGHUP/SIGTERM/SIGINT, spawns `handle(ch, q.Quit)`, returns a stop func (`signal.Stop` + close)
- [x] write tests: `trigger` called twice -> quit invoked once + `wasSignaled()` true; `handle` fed two fake signals on a channel -> quit invoked once; default state -> `wasSignaled()` false
- [x] run tests - must pass before next task

### Task 2: Extract finalize decision from run()

**Files:**
- Modify: `app/main.go`
- Modify: `app/main_test.go`

- [x] add `finalizeReq` struct (with `stdout io.Writer`) and `finalize(finalizeReq) (int, error)` in `app/main.go` next to `writeAnnotationOutput`
- [x] move the existing tail logic (discarded / empty / saveHistory / writeAnnotationOutput) into `finalize`, adding the `signaled` branch that returns after `saveHistory` without writing output; forward `r.stdout` into `annotationOutputReq`
- [x] rewrite run()'s 254-268 tail to compute `output`/`files`/`discarded` from the final model and call `finalize` (signaled hard-wired `false`, `stdout: os.Stdout` until Task 3)
- [x] write tests for `finalize` using temp `REVDIFF_HISTORY_DIR` + `bytes.Buffer` stdout: signaled=true -> history file exists, buffer empty; signaled=false & `-o` set -> history + output file; signaled=false & no `-o` -> history + buffer non-empty; discarded=true -> neither; empty output -> neither
- [x] run tests - must pass before next task

### Task 3: Wire signal handling into run()

**Files:**
- Modify: `app/main.go`
- Modify: `app/main_test.go`

- [x] append `tea.WithoutSignalHandler()` to `programOptions`
- [x] construct `guard := &shutdownGuard{}`, call `stop := guard.watch(p); defer stop()` after `tea.NewProgram`
- [x] pass `guard.wasSignaled()` into the `finalize` call
- [x] add a focused test using a mock `quitter` (records Quit calls): drive `guard.handle` with a fake signal channel and assert Quit fires once and `wasSignaled()` flips (no real process signal)
- [x] run tests - must pass before next task

### Task 4: History write uses the atomic helper

**Files:**
- Modify: `app/history/history.go`
- Modify: `app/history/history_test.go`

- [x] replace the plain `os.WriteFile` at `history.go:79` with `fsutil.AtomicWriteFile` (temp-file + rename, 0o600); add the `fsutil` import (no cycle risk — `fsutil` is stdlib-only)
- [x] write/extend tests asserting the history file is written completely and with mode 0o600
- [x] run tests - must pass before next task

### Task 5: First-class interactive window mode

**Files:**
- Modify: `.claude-plugin/skills/revdiff/scripts/agentdeck-window.sh`
- Modify: `plugins/codex/skills/revdiff/scripts/agentdeck-window.sh`

- [x] capture a separate opt-in flag (e.g. `_rd_focus=1`) ONLY when `REVDIFF_TMUX_WINDOW=1` is explicitly set, BEFORE `_rd_winmode` collapses user-opt and agent-deck auto-detection together
- [x] when `_rd_focus=1`: open the window focused (drop `-d`) and capture the previously-active window id, restoring it with `tmux select-window` after the review exits
- [x] keep agent-deck behavior unchanged (background `-d`, no focus steal) when window mode came from auto-detection
- [x] generalize the file header comment: it is the general tmux window backend; agent-deck auto-detection and explicit `REVDIFF_TMUX_WINDOW=1` are its two triggers
- [x] apply the identical change to both copies (keep in sync)
- [x] shellcheck both scripts clean
- [x] manual test (real tmux) — see Post-Completion

### Task 6: Documentation

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/references/config.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `.claude-plugin/skills/revdiff/SKILL.md`, `plugins/codex/skills/revdiff/SKILL.md`, `plugins/pi/skills/revdiff/SKILL.md`
- Modify: `CLAUDE.md`, `docs/ARCHITECTURE.md`

- [x] document `REVDIFF_TMUX_WINDOW=1` as the disconnect-resilient window mode (minimal description style, no runtime-toggle cross-refs) in README, docs.html, config.md, usage.md, and the two tmux-backend SKILL.md copies (.claude-plugin, codex)
- [x] document the safety-net behavior: annotations are saved to history (not `-o` output) on signal-delivered termination (SIGHUP disconnect, SIGTERM); recovery is "load newest from history". Explicitly note the SIGTERM semantic change (no longer writes `-o`)
- [x] pi `SKILL.md` gets ONLY the Part A safety-net/recovery note (pi is direct-terminal, no tmux window backend) — not window mode
- [x] add a CLAUDE.md gotcha entry for the signal-save path (`shutdownGuard`, `WithoutSignalHandler`, history-only-on-signal, SIGTERM change) and the window-mode promotion; add an ARCHITECTURE.md note
- [x] keep README and docs.html in sync; verify config.md/usage.md and SKILL.md copies match
- [x] run tests - must pass before next task

### Task 7: Verify acceptance criteria
- [x] verify all Overview requirements: signal -> history-only; `O`/`Q` unchanged; SIGTERM no longer writes `-o`; window mode focused + restores prior window
- [x] verify edge cases: empty store (no files written), rapid HUP+TERM (single save), `Q` then signal (still discarded)
- [x] run full suite: `make test`
- [x] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [x] verify coverage did not regress

### Task 8: Finalize documentation and plan
- [x] final pass on README.md / docs.html / CLAUDE.md / ARCHITECTURE.md for accuracy
- [x] move to completed/ (deferred to end-of-run finalize — orchestrator moves it after review phases)

## Post-Completion
*Items requiring manual intervention or external systems - no checkboxes, informational only*

**Manual verification (real tmux, cannot be unit-tested):**
- window mode: `REVDIFF_TMUX_WINDOW=1`, start a review, detach the tmux client (or drop SSH), reattach — the review window is still there, live, annotations intact; on graceful exit the prior active window is restored; agent-deck auto-mode still opens in the background.
- signal save: start a review with annotations in a popup, detach/`tmux kill-window` to tear the popup down (SIGHUP), confirm a fresh file appears under `~/.config/revdiff/history/<repo>/` and the `-o` output (if set) was NOT written. Repeat with `kill -TERM <pid>` and `kill -INT <pid>`.
- confirm typed interactive `Ctrl-C` remains a no-op (still ignored — it is NOT a signal in raw mode; this is intended, out of scope).

**Release / versioning (release-time only, per CLAUDE.md — not part of implementation):**
- Part A is a binary change and needs a tagged binary release before its behavior reaches brew / `go install` users.
- Part C is launcher/plugin only and works with any binary; do NOT bump plugin/marketplace/package versions on the feature branch. Bump at release time.

---
Smells pre-check: passed (initial). Revised per plan-review: `watch(q quitter)` 1 param, `handle(ch, quit)` 2 params, `finalizeReq` +`stdout io.Writer` field — all shapes still within hard limits, unexported, methods correctly scoped; no new violations.
