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

`/revdiff` routes requests through `/skill:revdiff`. The skill resolves refs, files, and natural-language targets before calling the `revdiff_review` tool. The tool launches the external `revdiff` binary through direct terminal handoff. pi temporarily suspends, revdiff takes over the terminal, and pi resumes when revdiff exits. If no annotations were captured, the review is clean.

The agent uses the `revdiff_review` tool for the review loop after handling annotations. The loop is: classify annotations, answer explanation requests first in normal chat, ask whether to continue or finish after explanation-only annotations, stop after any clean/no-annotation result, list planned code changes for code-change directives, edit files, and rerun `revdiff_review` with the same args only after repository files changed or when the user asks to continue reviewing.

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

Natural-language targets are supported because `/revdiff` routes through the skill:

```text
/revdiff prev commit
/revdiff last tag
/revdiff 2 weeks ago
```

You can also call the skill explicitly with `/skill:revdiff <request>`.

## Notes

- Requires the `revdiff` binary on `PATH`
- Set `REVDIFF_BIN=/absolute/path/to/revdiff` if pi cannot find the binary
- Direct terminal handoff is the only Pi launch mode
- Exit code `10` means annotations were captured, not failure
- Use `--untracked` when agent-created files should be reviewed before they are staged
- Use `--description` or `--description-file` after analysis/refactor work so the info popup carries review context
- Use `--annotations=<tempfile>` to preload in-session review notes
- Successful `revdiff_review` results include captured annotation text; history is only for explicit latest-history requests or missing-output fallback
- In the repo, the pi-specific resources live under `plugins/pi/` to keep harness integrations clearly separated

This integration is intentionally kept separate from other harnesses:

- Claude Code integration lives in `.claude-plugin/`
- OpenCode integration lives in `plugins/opencode/`
- pi integration lives here in `plugins/pi/`
