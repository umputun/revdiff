# pi integration

This directory contains the **pi-specific** integration for revdiff.

## Contents

- `extensions/revdiff.ts` — pi extension that launches revdiff with `REVDIFF_EXIT_CODE_ON_ANNOTATIONS` set, captures annotations, treats exit `10` as success-with-annotations, exposes the `revdiff_review` agent tool, and sends user-launched annotations to the agent immediately
- `skills/revdiff/SKILL.md` — pi skill describing the review workflow and command

## Install

From the repo root:

```bash
pi install https://github.com/umputun/revdiff
```

The root `package.json` exposes these resources via:

- `plugins/pi/extensions`
- `plugins/pi/skills`

## Usage

The pi package exposes one user command:

```text
/revdiff [args]
```

`/revdiff` launches the external `revdiff` binary through direct terminal handoff. pi temporarily suspends, revdiff takes over the terminal, and pi resumes when revdiff exits. If annotations were captured, the extension sends them to the agent immediately as a user message. If no annotations were captured, the review is clean.

The agent uses the `revdiff_review` tool for follow-up review loops after handling annotations. The loop is: classify annotations, answer explanation requests first, list planned code changes, edit files, rerun `revdiff_review` with the same args, and stop only when no annotations are captured.

Useful args:

```text
/revdiff                         -- detect uncommitted, staged, or branch changes, then open revdiff
/revdiff HEAD~1                  -- review last commit
/revdiff main                    -- review against main
/revdiff --staged                -- review staged changes
/revdiff --untracked             -- include untracked files in working-tree review
/revdiff --all-files             -- browse all tracked files
/revdiff --all-files --exclude vendor
/revdiff --only README.md        -- review a single file in context-only mode
/revdiff HEAD~3 --description="why this refactor matters"
/revdiff HEAD~3 --description-file=/tmp/revdiff-desc.md
/revdiff main --annotations=/tmp/revdiff-review.md
```

## Notes

- Requires the `revdiff` binary on `PATH`
- Set `REVDIFF_BIN=/absolute/path/to/revdiff` if pi cannot find the binary
- Direct terminal handoff is the only Pi launch mode
- Exit code `10` means annotations were captured, not failure
- Use `--untracked` when agent-created files should be reviewed before they are staged
- Use `--description` or `--description-file` after analysis/refactor work so the info popup carries review context
- Use `--annotations=<tempfile>` to preload in-session review notes
- In the repo, the pi-specific resources live under `plugins/pi/` to keep harness integrations clearly separated

This integration is intentionally kept separate from other harnesses:

- Claude Code integration lives in `.claude-plugin/`
- OpenCode integration lives in `plugins/opencode/`
- pi integration lives here in `plugins/pi/`
