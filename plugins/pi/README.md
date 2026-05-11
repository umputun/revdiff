# pi integration

This directory contains the **pi-specific** integration for revdiff.

## Contents

- `extensions/revdiff.ts` — pi extension that launches revdiff, captures annotations, exposes the `revdiff_review` agent tool, and shows results in pi
- `extensions/revdiff-post-edit.ts` — optional post-edit reminder extension (disabled by default)
- `skills/revdiff/SKILL.md` — pi skill describing the review workflow and commands

## Install

From the repo root:

```bash
pi install https://github.com/umputun/revdiff
```

The root `package.json` exposes these resources via:

- `plugins/pi/extensions`
- `plugins/pi/skills`

## Notes

To enable post-edit reminders after agent edits, run:

```text
/revdiff-reminders on
```

This integration is intentionally kept separate from other harnesses:

- Claude Code integration lives in `.claude-plugin/`
- OpenCode integration lives in `plugins/opencode/`
- pi integration lives here in `plugins/pi/`
