# pi integration

This directory contains the **pi-specific** integration for revdiff.

## Contents

- `extensions/revdiff.ts` — pi extension that launches revdiff with `REVDIFF_EXIT_CODE_ON_ANNOTATIONS` set, captures annotations, treats exit `10` as success-with-annotations, exposes the `revdiff_review` agent tool, and shows results in pi
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

To customize the prompt used by `/revdiff-apply`, create:

```text
~/.config/revdiff/pi-apply-prompt.md
```

Or set `REVDIFF_PI_APPLY_PROMPT_FILE` to another template path. Supported placeholders: `{{target}}`, `{{mode}}`, `{{command}}`, and `{{annotations}}`. If the template omits `{{annotations}}`, captured annotations are appended after the template.

This integration is intentionally kept separate from other harnesses:

- Claude Code integration lives in `.claude-plugin/`
- OpenCode integration lives in `plugins/opencode/`
- pi integration lives here in `plugins/pi/`
