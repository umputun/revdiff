# Exit Code on Annotations

## Overview
- Add an opt-in revdiff exit status for automation: `0` means no annotations, `10` means annotations were produced, and `1` remains for real errors.
- Preserve default standalone behavior: revdiff still exits `0` unless the new flag/config/env setting is enabled.
- Update bundled caller integrations so agent-driven review flows pass the new flag by default and treat exit `10` as success-with-annotations.

## Context (from discovery)
- files/components involved: `app/config.go`, `app/main.go`, launcher scripts in `.claude-plugin/skills/revdiff/scripts/`, `plugins/codex/skills/revdiff/scripts/`, `plugins/revdiff-planning/scripts/`, Pi extension files in `plugins/pi/extensions/`, OpenCode files in `plugins/opencode/`, and docs/skill references.
- related patterns found: CLI/config/env flags live in `app/config.go`; `run(opts)` owns post-TUI annotation output and currently returns `nil` after writing/printing annotations; launchers use `--output=<tmpfile>` and print that file after revdiff exits.
- dependencies identified: shell launchers use `set -e` and several terminal branches use sentinel files; nonzero `10` must not prevent output capture or sentinel cleanup.
- go-architect finding: add a config-backed bool flag, keep code `10` as success status, update launchers/callers to normalize `0`/`10`, and watch for copied launcher drift.

## Development Approach
- **testing approach**: Regular. Add focused tests for Go config/exit-code decision logic, plus smoke validation for shell/TS/Python caller handling.
- complete each task fully before moving to the next
- make small, focused changes
- every code-changing task includes new/updated tests
- all tests must pass before starting next task
- update this plan when scope changes
- maintain backward compatibility unless explicitly rejected

## Code-Quality Rules (HARD — verify against every task before marking complete)

These rules supplement project AGENTS.md/CLAUDE.md and are NOT optional. They are the gate for marking any task complete. If a rule is violated, the task is not done — refactor, re-test, then mark complete.

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
- Exception (per AGENTS.md/CLAUDE.md): methods called by other structs in the same package CAN be exported for inter-component API clarity. This is the only exception. It does not extend to types, functions, constants, or variables.
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

**Project-specific stricter addendum:** In this repository, count `ctx context.Context` toward the 4-parameter limit from AGENTS.md/CLAUDE.md. Do not use the exception in the canonical block above when implementing this plan.

## Testing Strategy
- Unit tests required for Go flag parsing and the annotation exit-code decision.
- Shell launcher validation must cover no annotations (`0`), annotations (`10`), and real launcher/revdiff failure (`1`).
- Caller validation must verify Pi/OpenCode/planning integrations treat `10` as success and still process stdout/output files.
- Documentation checks must verify every documented option name, env var, config key, and exit code matches implementation.

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with `➕`
- document blockers with `⚠️`
- keep plan in sync with actual work

## Solution Overview
- Add `--exit-code-on-annotations` with env/config support and default `false`.
- Use exit code `10` only after a normal quit that produces non-empty annotation output and after the annotation output write succeeds; history auto-save remains best-effort and logs warnings.
- Treat `Q` discard as no annotations: no output and exit `0`.
- Bundled agent/plugin launchers pass `--exit-code-on-annotations` by default, but standalone revdiff remains backward-compatible.
- Caller integrations accept `0` and `10` as successful launches; `10` means process annotations, `0` with empty stdout means clean review, and other nonzero codes remain real failures.

## Technical Details
- Flag name: `--exit-code-on-annotations`.
- Config/env names: `exit-code-on-annotations`, `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`.
- Exit status contract:
  - `0`: no annotations, discarded annotations, or default mode where the flag is off.
  - `10`: annotations were produced and `ExitCodeOnAnnotations` is enabled.
  - `1`: revdiff binary parse errors, TUI errors, write failures, and other real binary errors. Launchers/callers must treat any nonzero except `10` as failure unless they deliberately normalize it to `1`.
- Semantics are based on non-empty final annotation output, not dirty-tracking changes to preloaded annotations. This keeps the automation signal simple and avoids tracking annotation mutations inside the TUI.
- Keep `exitCodeAnnotations = 10` as an unexported constant in `app/main.go` unless an implementation need proves otherwise.
- If a helper is needed for tests, keep it small and unexported, for example `annotationExitCode(enabled bool, output string) int`.

## What Goes Where
- **Implementation Steps**: Go CLI/config changes, shell launcher rc propagation, Pi/OpenCode/Python caller updates, docs, tests, plan lifecycle.
- **Post-Completion**: plugin/package version bump decisions and external release notes only, no checkboxes.

## Implementation Steps

### Task 1: Add core exit-code support

**Files:**
- Modify: `app/config.go`
- Modify: `app/main.go`
- Modify: `app/config_test.go`
- Create or modify: `app/main_test.go` if a helper is added for exit-code decision tests

- [x] add `ExitCodeOnAnnotations bool` to `options` with `long:"exit-code-on-annotations"`, `ini-name:"exit-code-on-annotations"`, and `env:"REVDIFF_EXIT_CODE_ON_ANNOTATIONS"`
- [x] update `main()`/`run()` so successful annotation output can return exit code `10` without printing an error
- [x] preserve default `0` exit behavior when `ExitCodeOnAnnotations` is false
- [x] preserve `0` exit for clean quit and discard quit
- [x] write parse tests for CLI, env, config file, and default false behavior
- [x] write a `dumpConfig` assertion that `exit-code-on-annotations` appears with the exact config key
- [x] write exit-code decision tests for no output, output with flag disabled, output with flag enabled, and discarded/empty-output behavior
- [x] run tests: `go test ./app -run 'TestParseArgs|Test.*ExitCode'`
- [x] run formatter: `~/.claude/format.sh`

### Task 2: Make bundled shell launchers pass and preserve annotation exit code

**Files:**
- Modify: `.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh`
- Modify: `plugins/codex/skills/revdiff/scripts/launch-revdiff.sh`
- Modify: `plugins/revdiff-planning/scripts/launch-plan-review.sh`

- [x] add `--exit-code-on-annotations` to bundled revdiff command construction in each launcher
- [x] update terminal branches so `revdiff` exit `10` does not trip `set -e` before annotation output is printed
- [x] make every branch print the output file before exiting with the captured revdiff status
- [x] preserve real launcher errors as nonzero failures distinct from annotation status `10`
- [x] keep Claude and Codex launcher copies in sync except for the Codex source comment
- [x] write/run fake-`revdiff` smoke validation for launcher `0`, `10`, and `1` paths
- [x] validate each distinct launcher branch shape handles rc/output correctly: tmux `set -e` branch, sentinel-file branches, AppleScript wrapper branches, and Emacs FIFO branch
- [x] run syntax checks: `bash -n .claude-plugin/skills/revdiff/scripts/launch-revdiff.sh plugins/codex/skills/revdiff/scripts/launch-revdiff.sh plugins/revdiff-planning/scripts/launch-plan-review.sh`

### Task 3: Update caller integrations to treat 10 as success-with-annotations

**Files:**
- Modify: `plugins/pi/extensions/revdiff.ts`
- Modify: `plugins/opencode/tools/revdiff.ts`
- Modify: `plugins/opencode/plugins/revdiff-plan-review.ts`
- Modify: `plugins/revdiff-planning/scripts/plan-review-hook.py`

- [x] update Pi direct mode to pass `--exit-code-on-annotations` and accept exit codes `0` and `10`
- [x] update Pi overlay mode to pass the new flag and accept launcher status `10` while still parsing stdout
- [x] update OpenCode tool execution so Bun does not discard stdout or throw away annotations on exit `10`
- [x] update OpenCode plan-review plugin execution to accept exit `10` and inject annotations from stdout
- [x] update Claude planning hook Python to treat `returncode in (0, 10)` as a reviewed plan; use stdout to decide whether annotations exist
- [x] write/update focused tests where a harness exists; otherwise document the missing harness in task notes and validate with controlled fake-launcher smoke runs
- [x] run controlled fake-launcher smoke validation for OpenCode tool/plugin status `0`, `10`, and failure paths
- [x] run Python syntax check: `python3 -m py_compile plugins/revdiff-planning/scripts/plan-review-hook.py`
- [x] run package/smoke validation: `npm pack --dry-run`

Task 3 notes: no in-repo Pi/OpenCode TypeScript test harness exists, so validation used controlled fake-launcher smoke runs for OpenCode tool/plugin and Pi direct/overlay `0`/`10`/failure paths.

### Task 4: Update documentation, skills, and website references

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/SKILL.md`
- Modify: `.claude-plugin/skills/revdiff/references/config.md`
- Modify: `.claude-plugin/skills/revdiff/references/install.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff/SKILL.md`
- Modify: `plugins/codex/skills/revdiff/references/config.md`
- Modify: `plugins/codex/skills/revdiff/references/install.md`
- Modify: `plugins/codex/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff-plan/SKILL.md`
- Modify: `plugins/pi/skills/revdiff/SKILL.md`
- Modify: `plugins/pi/README.md`
- Modify: `plugins/opencode/README.md`
- Modify: `plugins/opencode/commands/revdiff.md`
- Modify: `plugins/revdiff-planning/README.md`
- Modify: launcher usage-comment headers where arguments are documented

- [x] document `--exit-code-on-annotations`, `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`, and `exit-code-on-annotations` in config/usage tables using the exact implementation names
- [x] document exit status contract `0` / `10` / `1` in README and relevant skill references
- [x] update Claude, Codex, Pi, and OpenCode workflow instructions so agents treat exit `10` as success-with-annotations
- [x] update plan-review docs/skills so plan annotation loops do not call `10` a launcher failure
- [x] keep CLI flag descriptions minimal and atomic across code/docs
- [x] run documentation consistency checks with `rg 'exit-code-on-annotations|REVDIFF_EXIT_CODE_ON_ANNOTATIONS|exit 10|code 10' README.md site .claude-plugin plugins`
- [x] run tests: `go test ./...`

Task 4 notes: documentation-only; no new tests added. Validation also ran formatter, race tests, linter, shell syntax checks, and `git diff --check`.

### Task 5: Verify acceptance criteria

**Files:**
- Modify: files touched by Tasks 1-4 only if validation finds defects

- [x] verify default `revdiff` exits `0` with annotations when the new flag/config is off
- [x] verify `revdiff --exit-code-on-annotations` exits `10` with annotations and still writes/prints annotation output
- [x] verify clean quit exits `0` with the flag on
- [x] verify discard quit exits `0` with the flag on
- [x] verify Pi direct/overlay, Claude/Codex launchers, OpenCode tool/plugin, and plan-review hook tolerate `10` as success-with-annotations
- [x] run full test suite: `make test`
- [x] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [x] run formatter: `~/.claude/format.sh`
- [x] run shell syntax checks for all modified shell scripts

Task 5 notes: pty smoke used real `revdiff` for default annotations, flagged annotations, clean quit, and discard quit. Validation found and fixed one Pi direct-mode clean-review defect: exit `0` without an output file is now treated as a successful no-annotation review, while exit `10` still requires annotation output. Mocked Pi/OpenCode/Python and fake tmux/revdiff launcher smokes covered `0`, `10`, and failure paths.

### Task 6: [Final] Archive plan

**Files:**
- Modify: `docs/plans/20260521-exit-code-on-annotations.md`
- Move: `docs/plans/20260521-exit-code-on-annotations.md` to `docs/plans/completed/20260521-exit-code-on-annotations.md`

- [x] verify all requirements from Overview are implemented
- [x] verify edge cases are handled
- [x] verify README/site/skill docs match code
- [x] update project agent guidance if new patterns were discovered (no update needed - no new reusable pattern discovered)
- [x] move this plan to `docs/plans/completed/20260521-exit-code-on-annotations.md`

## Post-Completion
*Items requiring manual intervention or external systems. No checkboxes.*

- Ask Eugene whether to bump Claude plugin version after `.claude-plugin/` changes.
- Ask Eugene whether to bump revdiff-planning plugin version in `plugins/revdiff-planning/.claude-plugin/plugin.json` after planning-plugin changes.
- Ask Eugene whether to bump Pi package version in `package.json` after `plugins/pi/` changes.
- Consider release notes mentioning `--exit-code-on-annotations` and exit code `10` for automation.

Smells pre-check: 1 item fixed before save — added project-specific stricter addendum for counting `ctx context.Context` toward the 4-parameter limit
Plan auto-review: 4 important items fixed before implementation — added planning README/version prompt, strengthened launcher branch validation, added OpenCode/Python validation, clarified exit-code contract, and added dump-config coverage
