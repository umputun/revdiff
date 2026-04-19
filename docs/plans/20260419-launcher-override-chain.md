# Launcher override chain for Claude plugins

## Overview

Adopt the cc-thingz override-chain pattern for revdiff's two Claude-side launcher scripts (`launch-revdiff.sh` in `.claude-plugin/skills/revdiff` and `launch-plan-review.sh` in `plugins/revdiff-planning`) so users can drop their own launcher at `.claude/<plugin-namespace>/scripts/<launcher>.sh` (project) or `${CLAUDE_PLUGIN_DATA}/scripts/<launcher>.sh` (user) and have it picked up instead of the bundled default.

Resolves the action item from [discussion #121](https://github.com/umputun/revdiff/discussions/121): "Same mechanism would work for the `/revdiff:revdiff` launcher too." Lets users customize how revdiff is launched (separate window, alternate split layout, custom terminal multiplexer) without forking the plugin.

Bundled launcher remains the default — zero behavior change for users who don't create overrides.

## Context (from discovery)

- **Pattern source**: `cc-thingz/plugins/planning` v3.4.0 — `skills/exec/scripts/resolve-file.sh` walks `project → user → bundled`, outputs file content to stdout. SKILL.md mandates "ALWAYS use the resolver, NEVER construct paths manually." A `SessionStart` hook seeds bundled defaults to `${CLAUDE_PLUGIN_DATA}/`, only copying files that don't already exist.
- **Targets in revdiff**:
  - `.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh` — consumed by `SKILL.md:103` as `${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh`
  - `plugins/revdiff-planning/scripts/launch-plan-review.sh` — consumed by `plan-review-hook.py:71` as `Path(plugin_root) / "scripts" / "launch-plan-review.sh"`
- **Out of scope**:
  - `plugins/codex/skills/revdiff/scripts/launch-revdiff.sh` — installed via `cp` to `~/.codex/skills/revdiff/scripts/`; user owns the install path and can edit directly. Codex has no `CLAUDE_PLUGIN_DATA` equivalent. Doc note added to codex SKILL.md so users don't expect Claude-style overrides to apply there.
  - `plugins/opencode/setup.sh` — copies launchers to `~/.config/opencode/...`; user owns the destination.
  - `plugins/pi/extensions/revdiff.ts` — TypeScript launcher discovery via `resolveLauncherScript()` at `revdiff.ts:907-912`. Pi resolves to either the in-repo `LAUNCH_REVDIFF_SCRIPT` constant (bundled-only, ignores any override) or `PATH`. **Pi runs outside Claude — `CLAUDE_PLUGIN_DATA` isn't set in the Pi runtime, so the override chain physically can't apply there**. README.md:197 advertises pi as "reuses the existing `launch-revdiff.sh` script from the Claude plugin integration" — that statement remains accurate (pi reuses the bundled script file path, not the resolution mechanism). Doc updates in Task 3b add a one-sentence clarification so pi users don't expect their Claude overrides to apply.
- **Differences from cc-thingz**:
  - Targets are executable scripts, not prompt templates → resolver outputs the resolved **path** (so consumer can exec with original argv preserved), not file content.
  - No SessionStart seeding — single launcher per plugin, auto-copy machinery would just create a stale copy that drifts from bundled fixes. Document the override path; user runs `cp` if they want a starting template.
  - Per-plugin resolver duplication (~25 lines bash each) — same accepted-duplication pattern as the launchers themselves (CLAUDE.md already calls it out as expected decoupling).

## Development Approach

- **testing approach during implementation**: static checks only (shellcheck/shfmt for shell, `python3 -m py_compile` for Python). All behavior verification — override matrix, skill/hook integration, regression, marketplace install — is post-plan manual testing (see Post-Completion).
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include the static checks for the files it touches** — shellcheck/shfmt clean for any new bash, py_compile clean for any Python edit
- **CRITICAL: all static checks must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- bundled launcher path stays the default — zero behavior change without overrides
- maintain backward compatibility: existing installs with no overrides must behave identically to today

## Testing Strategy

This plan introduces bash resolver scripts and a small Python hook helper. The repo has no unit-test framework for either; behavior verification is manual and runs as post-plan testing rather than per-task. Inside tasks, static checks are the only required gate.

**Static checks (per task, automatable)**:
- `shellcheck` and `shfmt -d` on each new resolver script (matching `set -euo pipefail` convention used by the existing launchers)
- `python3 -m py_compile` on the modified hook

**Manual verification (post-plan, see Post-Completion)**:
- override matrix per resolver (6 rows)
- skill integration via `/revdiff:revdiff` with stub override
- hook integration via real `ExitPlanMode` trigger with stub override
- regression: no-override path behaves identically to today
- marketplace install smoke test

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

### Override paths

For both plugins, the chain is `project → user → bundled`, first-found-wins:

| Plugin | Project layer | User layer | Bundled |
|---|---|---|---|
| `.claude-plugin/skills/revdiff` | `.claude/revdiff/scripts/launch-revdiff.sh` | `${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh` | `${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh` |
| `plugins/revdiff-planning` | `.claude/revdiff-planning/scripts/launch-plan-review.sh` | `${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh` | `${CLAUDE_PLUGIN_ROOT}/scripts/launch-plan-review.sh` |

The diff-review skill uses `${CLAUDE_SKILL_DIR}` and revdiff-planning uses `${CLAUDE_PLUGIN_ROOT}` for the bundled layer — that's the existing env-var convention for skills vs hooks. The two resolver copies are **structurally identical** and differ ONLY in the `NAMESPACE` constant. `SCRIPT_DIR` resolves the same way in both because each script lives at `<plugin>/scripts/`. Each plugin gets its own resolver because plugins can't share files — same low-cost duplication CLAUDE.md already accepts for the launchers themselves.

### Resolver script shape

```bash
#!/usr/bin/env bash
# resolve launcher script through three-layer override chain
# usage: resolve-launcher.sh <launcher-name> [data-dir]
# outputs absolute path of the first-found executable launcher
set -euo pipefail

NAMESPACE="<plugin-namespace>"   # "revdiff" or "revdiff-planning"

name="${1:-}"
if [ -z "$name" ]; then
    echo "error: usage: resolve-launcher.sh <launcher-name> [data-dir]" >&2
    exit 1
fi
data_dir="${2:-${CLAUDE_PLUGIN_DATA:-}}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

abspath() { (cd "$(dirname "$1")" && printf '%s/%s\n' "$(pwd)" "$(basename "$1")"); }

# project layer
if [ -x ".claude/$NAMESPACE/scripts/$name" ]; then
    abspath ".claude/$NAMESPACE/scripts/$name"
    exit 0
fi
# user layer
if [ -n "$data_dir" ] && [ -x "$data_dir/scripts/$name" ]; then
    abspath "$data_dir/scripts/$name"
    exit 0
fi
# bundled default
if [ -x "$SCRIPT_DIR/$name" ]; then
    abspath "$SCRIPT_DIR/$name"
    exit 0
fi
echo "error: launcher not found in override chain: $name" >&2
exit 1
```

`set -euo pipefail` matches the existing launchers (`launch-revdiff.sh:6`, `launch-plan-review.sh:6`). Path absolutization uses `cd && pwd` (portable, BSD-compatible) — same pattern `launch-plan-review.sh:31` already uses, not `realpath` (not available on older macOS).

### Consumer changes

**Diff-review skill** (`SKILL.md:103`): the resolver call and launcher exec must run in the **same bash invocation** so the shell variable holding the resolved path survives. Use a single-line composition inside one fenced code block:

```bash
# before (single line in one fenced block)
${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh [base] [against] [--staged] [--only=file1] [--all-files] [--exclude=prefix]

# after (single line in one fenced block; resolver runs as a sub-shell)
"$(${CLAUDE_SKILL_DIR}/scripts/resolve-launcher.sh launch-revdiff.sh ${CLAUDE_PLUGIN_DATA})" [base] [against] [--staged] [--only=file1] [--all-files] [--exclude=prefix]
```

This preserves the existing "long-running command, set bash timeout to max" instruction — the timeout cap still applies to the launcher, not the resolver (resolver is sub-shell, returns in ms). Add a sentence to SKILL.md Step 2 explicitly stating "the resolver and launcher must be in the same bash invocation."

**Plan-review hook** (`plan-review-hook.py`): add helper

```python
def resolve_launcher(plugin_root: str, name: str) -> Path | None:
    """resolve launcher path through override chain.
    returns Path on resolver success (exit 0); returns None if resolver exits non-zero
    (all layers absent, or invocation error). logs resolver stderr for diagnostics."""
```

Helper shells out to `<plugin_root>/scripts/resolve-launcher.sh <name> <CLAUDE_PLUGIN_DATA>`. Replace the existing hard-coded `Path(plugin_root) / "scripts" / "launch-plan-review.sh"` with `resolve_launcher(plugin_root, "launch-plan-review.sh")`. Keep the current "launcher not found" branch as the `None`-return handler.

## Technical Details

- **Why two ways to pass `CLAUDE_PLUGIN_DATA`**: SKILL.md text-substitutes `${CLAUDE_PLUGIN_DATA}` before calling the resolver as the 2nd arg. The env-var fallback covers cases where the resolver is invoked from a sub-shell that lost the var (e.g., direct invocation from terminal).
- **Project-layer namespace** (`.claude/<ns>/scripts/`): namespacing prevents collision when both plugins are installed in the same project — without it, `.claude/scripts/launch-revdiff.sh` and `.claude/scripts/launch-plan-review.sh` would collide if a user wanted to override only one but not the other (or worse, override paths from third-party plugins).
- **Executable check (`-x`)**: matches cc-thingz semantics. A non-executable file in an override layer is treated as absent — the resolver falls through to the next layer rather than erroring. Matches user expectation that `chmod -x` disables an override without deleting the file.
- **Path absolutization**: uses `cd "$(dirname X)" && pwd`/`basename X` composition — portable across BSD and GNU coreutils, matches the existing `launch-plan-review.sh:31` pattern. `realpath` is intentionally avoided (not present on older macOS without coreutils).
- **Shell mode**: `set -euo pipefail` matches the convention used by the existing launchers in this repo.

## What Goes Where

- **Implementation Steps** (`[ ]`): code/script changes inside revdiff (resolvers, SKILL.md edit, Python hook edit, doc updates, plugin manifest version bumps)
- **Post-Completion** (no checkboxes): smoke-testing the marketplace install flow, confirming downstream consumers (cc-thingz, ralphex if any) aren't affected, communicating the new override paths in discussion #121

## Implementation Steps

### Task 1: Add resolver to revdiff diff-review skill

**Files:**
- Create: `.claude-plugin/skills/revdiff/scripts/resolve-launcher.sh`
- Modify: `.claude-plugin/skills/revdiff/SKILL.md`
- Modify: `.claude-plugin/skills/revdiff/references/install.md`

- [x] create `.claude-plugin/skills/revdiff/scripts/resolve-launcher.sh` matching the shape in Solution Overview (NAMESPACE = `revdiff`, `set -euo pipefail`, `cd && pwd` for path absolutization)
- [x] `chmod +x` the new resolver
- [x] update `SKILL.md` Step 2: replace the direct `${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh ...` invocation with the single-line composition `"$(${CLAUDE_SKILL_DIR}/scripts/resolve-launcher.sh launch-revdiff.sh ${CLAUDE_PLUGIN_DATA})" ...` inside one fenced bash block; add a sentence noting "resolver and launcher must run in the same bash invocation"
- [x] add "Overrides" subsection to `references/install.md` documenting the project / user / bundled paths and an example "open in a new kitty window instead of an overlay" stub
- [x] run `shellcheck .claude-plugin/skills/revdiff/scripts/resolve-launcher.sh` and `shfmt -d` — must be clean

### Task 2: Add resolver to revdiff-planning plugin

**Files:**
- Create: `plugins/revdiff-planning/scripts/resolve-launcher.sh`
- Modify: `plugins/revdiff-planning/scripts/plan-review-hook.py`
- Create: `plugins/revdiff-planning/README.md`

- [x] create `plugins/revdiff-planning/scripts/resolve-launcher.sh` matching the shape in Solution Overview (NAMESPACE = `revdiff-planning`, identical to Task 1's resolver except for the namespace constant)
- [x] `chmod +x` the new resolver
- [x] update `plan-review-hook.py`: add `resolve_launcher(plugin_root, name) -> Path | None` helper per the contract in Solution Overview (returns `Path` on resolver exit 0; returns `None` on non-zero; logs resolver stderr); replace the hard-coded `Path(plugin_root) / "scripts" / "launch-plan-review.sh"` with the helper; keep the existing not-found branch as the `None`-return handler
- [x] create `plugins/revdiff-planning/README.md` with plugin description and "Overrides" subsection (project / user / bundled paths for `launch-plan-review.sh`)
- [x] run `shellcheck plugins/revdiff-planning/scripts/resolve-launcher.sh` and `shfmt -d` — must be clean
- [x] run `python3 -m py_compile plugins/revdiff-planning/scripts/plan-review-hook.py` to syntax-check the hook change

### Task 3a: Bump plugin versions (ASK USER FIRST)

**Files:**
- Modify: `.claude-plugin/plugin.json` (version field, line 3)
- Modify: `.claude-plugin/marketplace.json` (revdiff-skills entry version, line 12)
- Modify: `plugins/revdiff-planning/.claude-plugin/plugin.json` (version field, line 4)
- Modify: `.claude-plugin/marketplace.json` (revdiff-planning entry version, line 21)

- [ ] **ASK USER** before bumping any plugin version (per CLAUDE.md "ALWAYS ask about plugin version bump after `.claude-plugin/` file change")
- [ ] bump version in `.claude-plugin/plugin.json` (line 3) and the corresponding entry in `.claude-plugin/marketplace.json` (line 12) — keep in lockstep
- [ ] bump version in `plugins/revdiff-planning/.claude-plugin/plugin.json` (line 4) and the corresponding entry in `.claude-plugin/marketplace.json` (line 21) — keep in lockstep
- [ ] verify all four version fields match the intended values via `grep -n version .claude-plugin/plugin.json .claude-plugin/marketplace.json plugins/revdiff-planning/.claude-plugin/plugin.json`

### Task 3b: Sync top-level documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `site/docs.html`
- Modify: `plugins/codex/skills/revdiff/SKILL.md` (note: codex skills do not honor the override chain)

- [ ] update `README.md`: add a sentence to the existing Claude plugin section pointing at the override paths in `.claude-plugin/skills/revdiff/references/install.md` and `plugins/revdiff-planning/README.md`; add a one-sentence clarification near the pi mention (line ~197) that pi uses the bundled `launch-revdiff.sh` directly and does not honor Claude-plugin overrides (CLAUDE_PLUGIN_DATA is not set in the Pi runtime)
- [ ] update `CLAUDE.md`: add a note under the "Claude Code Plugin" section that launchers go through `resolve-launcher.sh` with the three-layer chain; spell out which namespace each plugin uses (`revdiff` and `revdiff-planning`); note the pi/codex asymmetry (overrides are Claude-only); add a "Testing locally" line documenting `claude --plugin-dir .claude-plugin` and `claude --plugin-dir plugins/revdiff-planning` plus `/reload-plugins` for iteration (matches cc-thingz `CLAUDE.md:39` convention)
- [ ] update `site/docs.html`: mirror the README override-chain note (per CLAUDE.md "must stay in sync with README.md") AND mirror the pi asymmetry sentence
- [ ] update `plugins/codex/skills/revdiff/SKILL.md`: add a one-sentence note that the override chain is Claude-only (codex users edit `~/.codex/skills/revdiff/scripts/launch-revdiff.sh` directly to customize)
- [ ] verify no broken links: every doc that mentions launchers references the override path correctly
- [ ] **NOT changed**: `site/index.html` version badge — that tracks revdiff CLI releases, not plugin versions; intentionally omitted

### Task 4: [Final] Verify doc consistency and archive plan

- [ ] confirm `README.md`, `CLAUDE.md`, `site/docs.html`, `.claude-plugin/skills/revdiff/references/install.md`, `plugins/revdiff-planning/README.md`, and `plugins/codex/skills/revdiff/SKILL.md` all describe the same override paths and asymmetries (no doc drift)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual testing — resolver behavior** (run after Task 1 and Task 2 are merged):

For each resolver (`.claude-plugin/skills/revdiff/scripts/resolve-launcher.sh` and `plugins/revdiff-planning/scripts/resolve-launcher.sh`), set up throwaway stubs in each layer and verify resolver picks the right one:

| project override | user override | bundled | env / arg state | expected resolver output |
|---|---|---|---|---|
| present (executable) | present | present | both set | project |
| absent | present (executable) | present | both set | user |
| absent | absent | present | both set | bundled |
| absent | absent | absent | both set | error, exit 1 |
| present (non-executable) | absent | present | both set | bundled (skips non-exec) |
| absent | absent | present | `CLAUDE_PLUGIN_DATA` unset, no 2nd arg | bundled (clean fall-through, no error) |

**Manual testing — integration**:
- skill integration: stub `.claude/revdiff/scripts/launch-revdiff.sh` emitting a sentinel string in a test repo, invoke `/revdiff:revdiff`, verify the sentinel reaches Claude
- hook integration: stub `${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh` emitting a sentinel, trigger real `ExitPlanMode`, verify the sentinel reaches the hook deny text
- CLI smoke for the hook helper: set up override fixture, run `plan-review-hook.py` with stdin JSON, verify sentinel appears in deny output
- regression: with no overrides present, both flows behave identically to today (bundled launcher runs, no observable difference)

**Manual testing — local plugin runs (fast iteration loop)**:

Per cc-thingz `CLAUDE.md:39` convention: `claude --plugin-dir <path>` loads a local plugin directly without going through the marketplace, and `/reload-plugins` picks up file edits mid-session.

- diff-review skill: `claude --plugin-dir .claude-plugin` → run `/revdiff:revdiff` with and without overrides
- planning hook: `claude --plugin-dir plugins/revdiff-planning` → trigger `ExitPlanMode` with and without overrides
- iterate: edit resolver, run `/reload-plugins`, retest — no reinstall needed

**Manual testing — marketplace install (final smoke)**:
- `/plugin marketplace update` locally
- `/plugin reinstall` both plugins
- verify versions visible in `/plugin` UI
- re-run integration tests against the freshly installed plugins to confirm the marketplace install path resolves overrides correctly
- confirm `${CLAUDE_PLUGIN_DATA}` resolves to the expected user-data directory under `~/.claude/plugins/data/<plugin-id>/`
- have a real user with a custom-launcher need (e.g., the discussion-#121 reporter) drop their override and confirm it activates

**Communication**:
- post follow-up in discussion #121 confirming the override mechanism is shipped, with a copy-pasteable example for the user-level `launch-plan-review.sh` override
- decide separately whether to fix the related discussion-#121 gap (plan-review hook deny-text not referencing the classify/explain `??` loop) — out of scope for this plan, but log a separate task

**Downstream awareness**:
- `cc-thingz` and other downstream projects bundling their own copy of `launch-revdiff.sh` are unaffected (they continue using their own copy); no coordination needed
- pi extension: behavior unchanged (still uses the bundled script path directly); the asymmetry is now documented in README/CLAUDE.md/site
