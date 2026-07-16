# Codex CLI integration

This directory contains the **Codex CLI** skills for revdiff.

## Contents

- `skills/revdiff/SKILL.md` — diff review skill (same workflow as Claude Code plugin)
- `skills/revdiff/scripts/` — detect-ref.sh, launch-revdiff.sh
- `skills/revdiff/references/` — config.md, install.md, usage.md
- `skills/revdiff-plan/SKILL.md` — plan/response review skill (extracts last Codex assistant message)
- `skills/revdiff-plan/scripts/` — extract-last-message.sh

## Requirements

- `revdiff` binary — `brew install umputun/apps/revdiff` or download from [releases](https://github.com/umputun/revdiff/releases)
- `jq` — required by the `revdiff-plan` skill for extracting messages from Codex rollout files
- A supported terminal: agterm, tmux, Zellij, herdr, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm

## Install

Install automatic Plan-mode review as an opt-in plugin:

```bash
codex plugin marketplace add umputun/revdiff
codex plugin add revdiff-planning@revdiff
```

Start a new session and trust the hook through `/hooks`. It runs only in Plan mode and first checks the current Stop payload's `last_assistant_message`. If that field has no complete `<proposed_plan>`, the hook reads the exact event transcript and selects the last assistant message for the matching session and turn, regardless of provider-specific phase fields. Annotated revisions use rolling snapshot comparisons; `/revdiff-plan` below remains the manual fallback.

Install the manual skills by cloning the repository first:

```bash
git clone https://github.com/umputun/revdiff.git
cd revdiff
```

Then copy each skill to your Codex skills directory:

```bash
cp -r plugins/codex/skills/revdiff ~/.codex/skills/revdiff
cp -r plugins/codex/skills/revdiff-plan ~/.codex/skills/revdiff-plan
```

Alternatively, symlink them so `git pull` updates propagate without re-copying:

```bash
ln -s "$PWD/plugins/codex/skills/revdiff" ~/.codex/skills/revdiff
ln -s "$PWD/plugins/codex/skills/revdiff-plan" ~/.codex/skills/revdiff-plan
```

## Skills

### `/revdiff`

Interactive diff review with inline annotations.

```text
/revdiff              — auto-detect ref (uncommitted, staged, branch vs master, or last commit)
/revdiff HEAD~3       — review last 3 commits
/revdiff main feature — two-ref diff
/revdiff all files    — browse all tracked files
/revdiff path/to/file — review a single file
```

Annotations are captured on exit. Codex classifies them as code-change directives or explanation requests, addresses each, and re-launches revdiff for verification.

### `/revdiff-plan`

Review the last Codex assistant message (plan, analysis, or proposal) with annotations.

```text
/revdiff-plan         — extract last response from Codex rollout files, open in revdiff
```

The skill reads `~/.codex/sessions/` rollout JSONL files, extracts the most recent assistant message, writes it to a temp file, and opens revdiff with `--only=<tempfile>`. Annotations feed back into a refinement loop.

## Differences from Claude Code plugin

- Automatic review uses a Codex `Stop` hook; Claude Code uses `PreToolUse/ExitPlanMode`
- The automatic hook falls back whenever `last_assistant_message` lacks a complete plan, then uses the last assistant message for the exact transcript/session/turn; manual `/revdiff-plan` uses best-effort rollout discovery
- Script path resolution falls back to `$CODEX_HOME` (or `~/.codex`) instead of `$CLAUDE_SKILL_DIR`
- `AskUserQuestion` tool replaced with numbered-list prompts (Codex convention)
- `EnterPlanMode` replaced with inline markdown plan + confirmation prompt

## Notes

This integration is intentionally kept separate from other harnesses:

- Claude Code integration lives in `.claude-plugin/`
- Pi integration lives in `plugins/pi/`
- Codex integration lives here in `plugins/codex/`
