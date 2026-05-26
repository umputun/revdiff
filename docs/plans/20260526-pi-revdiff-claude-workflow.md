# Align Pi revdiff Integration with Claude Review Workflow

## Overview
- Implement issue #207 by replacing the Pi-specific pending annotation workflow with the Claude-style revdiff review loop.
- The current Pi integration stores annotations in side-panel/widget state and requires `/revdiff-apply` to send them back to the agent. The new flow sends captured annotations to the agent immediately.
- The public Pi command surface becomes one command: `/revdiff [args]`. The agent rerun loop remains available through the `revdiff_review` tool.
- The Pi integration uses direct terminal handoff only. Overlay mode and the Claude `launch-revdiff.sh` dependency are removed from the Pi review path.

## Context (from discovery)
- Files/components involved: `plugins/pi/extensions/revdiff.ts`, `plugins/pi/extensions/revdiff-post-edit.ts`, `plugins/pi/skills/revdiff/SKILL.md`, `plugins/pi/README.md`, README Pi section, `site/docs.html`, `package.json`, `.claude-plugin/skills/revdiff/scripts/detect-ref.sh`.
- Current `plugins/pi/extensions/revdiff.ts` registers direct and overlay launch modes, pending review state, a message renderer, side-panel UI, status/widget UI, `/revdiff-rerun`, `/revdiff-results`, `/revdiff-apply`, `/revdiff-clear`, and `revdiff_review` with `mode`/`openPanel` parameters.
- `/revdiff` currently captures annotations as custom message data and opens a panel; it does not trigger the agent until `/revdiff-apply` builds a hardcoded apply prompt.
- `revdiff_review` already returns annotations to the agent as a tool result and is the right path for agent-driven reruns.
- `.claude-plugin/skills/revdiff/scripts/detect-ref.sh` already emits `use_staged`, but the Pi extension does not parse it yet.
- There is no existing TypeScript test harness or `package.json` test script for the Pi extension. Manual validation is the selected testing approach for this ticket.

## Development Approach
- **testing approach**: Manual validation for the Pi TypeScript extension, per user decision during brainstorming. Do not add a TypeScript test harness in this ticket.
- Complete each task fully before moving to the next.
- Make small, focused changes.
- Keep the implementation direct; do not add compatibility shims for removed Pi commands.
- Run repository format/lint/test commands before completion where practical.
- Update this plan when scope changes.
- Maintain compatibility for normal `/revdiff [args]` and `revdiff_review` usage; reject compatibility for removed pending-UI commands by design.

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

**TypeScript extension checklist for this plan:**
- Keep `plugins/pi/extensions/revdiff.ts` functions small and direct; avoid compatibility hooks for removed commands.
- No new function or constructor should take 4+ parameters unless grouped behind an option object.
- Remove stale public command/tool parameters instead of keeping deprecated aliases.
- Remove dead events, restore hooks, persisted state, and helper functions when their only consumer is removed.
- Add comments only for non-obvious Pi API behavior or shell-quoting invariants.

## Testing Strategy
- Manual validation is required for the Pi TUI flow:
  - `/revdiff` with no annotations reports clean and does not trigger an agent turn.
  - `/revdiff` with annotations sends a real user message to the agent immediately.
  - `revdiff_review` returns annotations as a tool result and remains direct-only.
  - staged-only no-arg detection opens staged changes with `--staged`.
  - a dirty feature branch with staged-only changes still asks branch-vs-uncommitted; `--staged` is applied only when the uncommitted path is selected.
- No TypeScript test harness is added in this ticket.
- Run `npm pack --dry-run` and verify the tarball includes executable `detect-ref.sh` but not `launch-revdiff.sh` or `plugins/pi/extensions/revdiff-post-edit.ts`.
- Run `~/.claude/format.sh`, `go test ./...`, and `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` before completion if the local environment supports them.
- Run focused package/runtime checks for the Pi extension when available, such as loading the package in Pi or invoking the changed command manually.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with `➕`.
- Document blockers with `⚠️`.
- Keep plan in sync with actual work.

## Solution Overview
- Use a hard cut rather than a compatibility transition.
- `/revdiff [args]` remains the only user command and uses the existing direct terminal handoff mechanism.
- Captured annotations are sent to the main agent with `pi.sendUserMessage()` using a prompt that preserves the Claude-style workflow: classify annotations, answer explanation requests first, review explanation drafts when needed, list planned changes before editing, apply code-change directives, and rerun `revdiff_review` with the same args until clean.
- `revdiff_review` stays as the agent-only interactive review tool for rerun loops, but becomes direct-only and loses Pi panel options.
- Removed workflows are deleted rather than kept as deprecated aliases: no pending inbox, no results panel, no apply/clear/rerun commands, no overlay path, and no post-edit reminder command in the default package surface.
- `use_staged` from the existing detect-ref script is honored so staged-only no-arg reviews open staged changes correctly without bypassing the dirty-feature-branch choice.

## Technical Details
- `LaunchSpec` should only carry `args` and `label`; remove launch mode from the Pi extension's data model.
- Replace persisted `ReviewState`/`LaunchMemory` with a transient run result that carries `args`, a shell-quoted rerun args string, `label`, `rawOutput`, and `annotations` only while handling one launch. Do not keep `createdAt` unless a concrete remaining consumer needs it.
- Remove `STATE_TYPE`, `LAST_LAUNCH_TYPE`, `LaunchMemory`, `setLastLaunch`, `restoreState`, `session_start`/`session_tree` restore handlers, and `pi.events.emit("revdiff:launch")` after their consumers are deleted.
- `startReview()` handles user-launched `/revdiff`: run direct review, notify clean result, or send annotations to the agent.
- `revdiff_review` handles agent-launched reviews: resolve args, run direct review, and return tool text/details. It must not persist pending state or open panels.
- Preserve round-trippable arguments. Add or keep a shell-quoting helper for prompt/tool details so rerun guidance uses exactly the original args, including paths or descriptions with spaces.
- Preserve file review detection for existing files and path-like args while simplifying `resolveLaunchSpec`. Path-like args include existing paths, `./...`, `/...`, or file-looking args that should map to `--only` consistently with current skill guidance.
- `sendAnnotationsToAgent()` should include:
  - review target
  - original command as a shell-quoted `revdiff ...`
  - rerun args string suitable for `revdiff_review args: "..."`
  - raw annotation output
  - instructions to classify explanation requests vs code-change directives
  - instruction to answer questions before editing
  - instruction to write explanation answers to a temp markdown file and review that file with `revdiff_review --only <tempfile>` when explanation notes require user review/refinement
  - instruction to list planned file/code changes before editing
  - instruction to rerun `revdiff_review` with the same args until clean
  - guidance to include `--untracked` on reruns when agent-created files should be reviewed
- `detectSmartLaunch()` should apply `--staged` only to the uncommitted-review path when `useStaged=true`. On dirty feature branches, keep the existing branch-diff vs uncommitted prompt and use `--staged` only if the user chooses uncommitted changes.
- `runDetectRefScript()` should parse `use_staged`. `detectSmartRefFallback()` should compute staged-only for git and set the same field.
- `package.json` should include `.claude-plugin/skills/revdiff/scripts/detect-ref.sh` rather than the whole scripts directory.
- Deleting `plugins/pi/extensions/revdiff-post-edit.ts` removes `/revdiff-reminders` from extension auto-discovery.
- After any Pi plugin file change, ask whether to bump `package.json` version. Do not bump it automatically.

## What Goes Where
- **Implementation Steps**: Pi extension simplification, staged detection, docs, manual validation, plan lifecycle.
- **Post-Completion**: Package version bump decision and any external release/publish action. No checkboxes.

## Implementation Steps

### Task 1: Simplify Pi revdiff command and tool runtime

**Files:**
- Modify: `plugins/pi/extensions/revdiff.ts`
- Modify: `app/plugin_exit_code_test.go`

- [x] remove pending review state persistence, status/widget updates, message renderer, results panel UI, and apply prompt code from `plugins/pi/extensions/revdiff.ts`
- [x] remove `/revdiff-rerun`, `/revdiff-results`, `/revdiff-apply`, and `/revdiff-clear` command registrations
- [x] remove `STATE_TYPE`, `LAST_LAUNCH_TYPE`, `LaunchMemory`, `lastLaunch`, `setLastLaunch`, `restoreState`, and session restore hooks
- [x] remove `pi.events.emit("revdiff:launch")` and drop the now-unneeded `pi` parameter from `runReview`
- [x] remove Pi overlay mode handling, `--pi-overlay`, `--pi-direct`, `REVDIFF_PI_MODE`, and `launch-revdiff.sh` resolution from the Pi review path
- [x] keep `/revdiff [args]` as the only user command and make it direct-terminal only
- [x] add immediate annotation delivery from `/revdiff` through `pi.sendUserMessage()` using the Claude-style loop instructions
- [x] simplify `revdiff_review` to direct-only parameters and remove `mode`/`openPanel`
- [x] preserve round-trippable shell-quoted rerun args in the generated agent prompt and tool details
- [x] confirm no automated TypeScript tests are added for this task per manual-validation decision
- [x] manually validate clean and annotation-captured `/revdiff` behavior in Pi if the local interactive environment allows it (skipped - not automatable in this subagent)
- [x] run tests: `go test ./...`

### Task 2: Honor staged-only smart detection and path-like file args

**Files:**
- Modify: `plugins/pi/extensions/revdiff.ts`
- Reference: `.claude-plugin/skills/revdiff/scripts/detect-ref.sh`

- [x] add `useStaged` to the Pi smart-detection result type
- [x] parse `use_staged` from `.claude-plugin/skills/revdiff/scripts/detect-ref.sh` output
- [x] update fallback git detection to set `useStaged` for staged-only changes
- [x] make no-arg smart detection return `args: ["--staged"]` and label `staged changes` when `useStaged` is true on the uncommitted-review path
- [x] preserve the dirty-feature-branch branch-vs-uncommitted prompt and apply `--staged` only when the user chooses uncommitted changes
- [x] preserve file review detection for existing files and path-like args while simplifying argument handling
- [x] manually validate staged-only no-arg detection and dirty-feature-branch staged-only choice when practical (validated detect-ref staged-only outputs; interactive Pi choice skipped - not automatable in this subagent)
- [x] run tests: `go test ./...`

### Task 3: Remove post-edit reminder from default Pi package surface

**Files:**
- Delete: `plugins/pi/extensions/revdiff-post-edit.ts`
- Modify: `package.json`

- [x] delete `plugins/pi/extensions/revdiff-post-edit.ts`
- [x] update `package.json` `files` to include `.claude-plugin/skills/revdiff/scripts/detect-ref.sh` instead of the whole scripts directory
- [x] verify package discovery no longer exposes `/revdiff-reminders`
- [x] run package dry-run: `npm pack --dry-run`
- [x] verify dry-run output includes `detect-ref.sh`
- [x] verify `detect-ref.sh` remains executable for direct `spawnSync` use
- [x] verify dry-run output excludes `launch-revdiff.sh`
- [x] verify dry-run output excludes `plugins/pi/extensions/revdiff-post-edit.ts`
- [x] run tests: `go test ./...`

### Task 4: Update Pi workflow documentation

**Files:**
- Modify: `plugins/pi/skills/revdiff/SKILL.md`
- Modify: `plugins/pi/README.md`
- Modify: `README.md`
- Modify: `site/docs.html`

- [x] document `/revdiff [args]` as the only Pi user command
- [x] document direct terminal handoff only; remove overlay, pending UI, apply/results/rerun/clear, and reminder references
- [x] document agent handling of captured annotations: classify explanation requests and code-change directives, answer questions first, review explanation drafts through temp markdown files when needed, list planned changes before editing, rerun `revdiff_review` until clean
- [x] document `--untracked` guidance for agent-created files
- [x] document `--description` and `--description-file` guidance after analysis/refactor work
- [x] document existing-history workflow for "use my latest revdiff annotations"
- [x] document in-session review preload with `--annotations=<tempfile>`
- [x] keep `site/docs.html` in sync with the README Pi section
- [x] run tests: `go test ./...`

### Task 5: Verify acceptance criteria

**Files:**
- Inspect: `plugins/pi/extensions/revdiff.ts`
- Inspect: `plugins/pi/skills/revdiff/SKILL.md`
- Inspect: `plugins/pi/README.md`
- Inspect: `README.md`
- Inspect: `site/docs.html`
- Inspect: `package.json`

- [x] verify `/revdiff [args]` starts direct revdiff and sends captured annotations to the agent without an apply step
- [x] verify no pending annotation widget, status, results panel, or apply/clear/results/rerun command remains in the Pi workflow
- [x] verify staged-only no-arg review opens staged changes with `--staged`
- [x] verify dirty feature branch staged-only state still asks branch-vs-uncommitted
- [x] verify path-like single-file review detection still maps to `--only` as intended
- [x] verify `revdiff_review` is direct-only and suitable for rerun loops after fixes
- [x] verify docs match the new workflow and do not mention removed Pi overlay/pending commands
- [x] verify package dry-run includes only the intended Pi files and executable detect-ref script
- [x] run full test suite: `go test ./...`
- [x] run race tests: `go test -race ./...`
- [x] run formatter: `~/.claude/format.sh`
- [x] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`

### Task 6: [Final] Update plan lifecycle

**Files:**
- Move: `docs/plans/20260526-pi-revdiff-claude-workflow.md` to `docs/plans/completed/20260526-pi-revdiff-claude-workflow.md`

- [ ] update project agent guidance if implementation reveals new durable Pi integration patterns
- [ ] ask whether to bump `package.json` version because Pi plugin files changed
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems. No checkboxes.*

- Decide whether to bump and publish the Pi package version.
- If publishing or pushing is needed, request fresh explicit approval for the exact remote write.
- If a package version bump is selected, update `package.json` in a separate focused change or task before release.
