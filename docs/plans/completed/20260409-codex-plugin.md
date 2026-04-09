# Codex CLI Plugin for revdiff

## Overview
- Create a Codex CLI plugin that provides revdiff integration for diff review and plan annotation
- Addresses Issue #84 — user request for Codex plugin/skill for easy install
- Two skills: `revdiff` (diff review) and `revdiff-plan` (manual plan review after Codex generates a plan)
- Targets Claude-plugin parity (not Pi-extension parity) since Codex's plugin model is skill-based

## Context (from brainstorm + external review)
- Codex plugin format: `.codex-plugin/plugin.json` manifest + `skills/` directory with `SKILL.md` files
- Codex marketplace: `.agents/plugins/marketplace.json` (separate from `.claude-plugin/marketplace.json`)
- Codex has no hook system (no `ExitPlanMode` interception) — plan review is manual via `/revdiff-plan`
- Existing scripts (`detect-ref.sh`, `launch-revdiff.sh`) are pure shell and portable to Codex
- Plugin lives at `plugins/codex/` — coexists with `plugins/pi/` and `plugins/revdiff-planning/`
- SKILL.md format is identical between Claude Code and Codex (YAML frontmatter + markdown body)
- Codex rollout files at `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` contain assistant messages
- **Script path resolution**: Codex has no `CODEX_SKILL_DIR` env var (unlike Claude Code's `CLAUDE_SKILL_DIR`).
  Three documented patterns exist (from Codex system skills):
  1. `$CODEX_HOME/skills/<name>/scripts/` — for home-installed skills (used by imagegen)
  2. Repo-relative path with CWD=repo root — for repo-local plugins (used by plugin-creator)
  3. "Resolve relative to this skill directory" — agent infers path (used by plannotator, least reliable)
  **Our approach**: SKILL.md instructs the agent to resolve via repo root first, then fall back to `$CODEX_HOME`:
  `SCRIPT_DIR="$(git rev-parse --show-toplevel 2>/dev/null)/plugins/codex/skills/revdiff/scripts"` with
  fallback `SCRIPT_DIR="${CODEX_HOME:-$HOME/.codex}/plugins/revdiff/skills/revdiff/scripts"`.
  This covers both repo-local usage and external plugin installation.
- **Codex rollout JSONL format**: each line is a JSON object. Assistant messages have
  `type: "response_item"`, `payload.type: "message"`, `payload.role: "assistant"`,
  `payload.content[].type: "output_text"`, `payload.content[].text: "<message text>"`
- **Codex tools**: Codex supports `Bash`, `Read`, `Edit`, `Write`, `Grep`, `Glob` (same as Claude Code)

## Development Approach
- **testing approach**: manual verification after implementation. Test by either:
  (a) pointing Codex plugin path to our repo directory (if Codex supports custom plugin paths), or
  (b) copying `plugins/codex/` to `~/.codex/plugins/revdiff/` (or wherever Codex installs plugins) manually.
  Verify both skills trigger and scripts execute correctly.
- complete each task fully before moving to the next
- scripts are **copies** from `.claude-plugin/skills/revdiff/scripts/`, not symlinks
- add a comment at the top of each copied script noting source file for future sync
- SKILL.md files are adapted for Codex (no `AskUserQuestion`, no `EnterPlanMode`)
- no hooks — Codex doesn't support them reliably yet

## Solution Overview
- Port the Claude Code `revdiff` skill to Codex with minimal adaptations
- Add a new `revdiff-plan` skill for manual plan review (extracts last Codex assistant message)
- Package as a Codex plugin with marketplace entry
- Codex marketplace is separate from Claude marketplace — no cross-listing needed

## Acceptance Criteria
- Codex CLI can discover the plugin via `.agents/plugins/marketplace.json`
- `/revdiff` triggers a diff review session in terminal overlay
- `/revdiff-plan` extracts the last Codex assistant message and opens it in revdiff for annotation
- annotation feedback loop works (re-launch until user quits without annotations)
- all scripts are executable and macOS/Linux compatible

## Implementation Steps

### Task 1: Create plugin scaffold and manifest

**Files:**
- Create: `plugins/codex/.codex-plugin/plugin.json`
- Create: `.agents/plugins/marketplace.json`

- [x] create `plugins/codex/` directory structure
- [x] create `plugins/codex/.codex-plugin/plugin.json` with name, version, description, author, repository, license, keywords, skills path
- [x] create `.agents/plugins/marketplace.json` with Codex marketplace entry
- [x] verify manifest structure matches Codex plugin-creator spec

### Task 2: Create `revdiff` skill (main diff review)

**Files:**
- Create: `plugins/codex/skills/revdiff/SKILL.md`
- Create: `plugins/codex/skills/revdiff/scripts/detect-ref.sh`
- Create: `plugins/codex/skills/revdiff/scripts/launch-revdiff.sh`
- Create: `plugins/codex/skills/revdiff/references/config.md`
- Create: `plugins/codex/skills/revdiff/references/install.md`
- Create: `plugins/codex/skills/revdiff/references/usage.md`

- [x] copy `detect-ref.sh` from `.claude-plugin/skills/revdiff/scripts/`
- [x] copy `launch-revdiff.sh` from `.claude-plugin/skills/revdiff/scripts/`
- [x] copy reference docs from `.claude-plugin/skills/revdiff/references/`
- [x] create `SKILL.md` adapted for Codex:
  - [x] replace `${CLAUDE_SKILL_DIR}` with `$(git rev-parse --show-toplevel)/plugins/codex/skills/revdiff`
  - [x] replace `AskUserQuestion` with "present options as a numbered list and wait for user response"
  - [x] replace `EnterPlanMode` with "write the plan as a markdown list and ask 'proceed with these changes?'"
  - [x] set `allowed-tools: [Bash, Read, Edit, Write, Grep, Glob]` (Codex supports all six)
  - [x] keep workflow steps 0-7 (install check, ref detection, launch, capture, classify, plan, address, loop)
  - [x] keep timeout handling as-is (Codex has similar bash timeout behavior)
- [x] verify scripts have execute permissions

### Task 3: Create `revdiff-plan` skill (manual plan review)

**Files:**
- Create: `plugins/codex/skills/revdiff-plan/SKILL.md`
- Create: `plugins/codex/skills/revdiff-plan/scripts/extract-last-message.sh`

- [x] create `extract-last-message.sh` script:
  - [x] check `~/.codex/sessions/` exists, exit with error message if not
  - [x] find most recent rollout file via `ls -t ~/.codex/sessions/*/*/*/*rollout*.jsonl 2>/dev/null | head -1`
  - [x] extract last assistant message using jq forward scan + tail:
    `jq -r 'select(.type=="response_item" and .payload.type=="message" and .payload.role=="assistant") | .payload.content[] | select(.type=="output_text") | .text' "$rollout" | tail -1`
  - [x] output raw markdown text to stdout
  - [x] handle edge cases: no rollout files found, empty/no assistant messages, jq not installed (check with `command -v jq`)
- [x] create `SKILL.md` with:
  - [x] trigger on "revdiff-plan", "review plan with revdiff", "annotate plan"
  - [x] workflow: extract last message → write temp file → launch revdiff `--only=` → capture annotations → feed back
  - [x] annotation loop (re-launch until no annotations)
  - [x] cleanup temp file on completion
- [x] verify script has execute permission
- [x] test extraction against a real Codex rollout file

### Task 4: Create plugin README

**Files:**
- Create: `plugins/codex/README.md`

- [x] document installation steps (Codex plugin install command)
- [x] document available skills and usage examples
- [x] document requirements (revdiff binary, jq for plan review)
- [x] note differences from Claude Code plugin

### Task 5: Update project documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [x] add Codex plugin section to CLAUDE.md project structure
- [x] add Codex plugin mention to README.md installation/integration section
- [x] verify no stale references

### Task 6: Verify and finalize

- [x] verify plugin directory structure matches Codex conventions
- [x] verify all scripts are executable (`chmod +x`)
- [x] verify marketplace.json is valid JSON
- [x] verify SKILL.md frontmatter is valid YAML
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- install plugin in Codex CLI and test `/revdiff` skill triggers
- test `/revdiff-plan` extraction against active Codex session
- verify overlay launch in tmux/kitty/wezterm environments
- test annotation capture and feedback loop

**Issue follow-up:**
- respond to Issue #84 with installation instructions
