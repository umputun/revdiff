# Codex CLI integration

This directory contains the **Codex CLI** plugin for revdiff.

## Contents

- `.codex-plugin/plugin.json` — plugin manifest
- `skills/revdiff/SKILL.md` — diff review skill (same workflow as Claude Code plugin)
- `skills/revdiff/scripts/` — detect-ref.sh, launch-revdiff.sh
- `skills/revdiff/references/` — config.md, install.md, usage.md
- `skills/revdiff-plan/SKILL.md` — plan/response review skill (extracts last Codex assistant message)
- `skills/revdiff-plan/scripts/` — extract-last-message.sh

## Requirements

- `revdiff` binary — `brew install umputun/apps/revdiff` or download from [releases](https://github.com/umputun/revdiff/releases)
- `jq` — required by the `revdiff-plan` skill for extracting messages from Codex rollout files
- A supported terminal: tmux, Zellij, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm

## Install

Copy the plugin directory to your Codex plugins location:

```bash
cp -r plugins/codex ~/.codex/plugins/revdiff
```

Or, if working from a cloned repo, Codex discovers the plugin via `.agents/plugins/marketplace.json` at the repo root.

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

- No hook-based automatic plan review — Codex lacks hook support, so plan review is manual via `/revdiff-plan`
- Uses Codex rollout files (`~/.codex/sessions/`) instead of Claude session logs for message extraction
- Script path resolution falls back to `$CODEX_HOME` (or `~/.codex`) instead of `$CLAUDE_SKILL_DIR`
- `AskUserQuestion` tool replaced with numbered-list prompts (Codex convention)
- `EnterPlanMode` replaced with inline markdown plan + confirmation prompt

## Notes

This integration is intentionally kept separate from other harnesses:

- Claude Code integration lives in `.claude-plugin/`
- Pi integration lives in `plugins/pi/`
- Codex integration lives here in `plugins/codex/`
