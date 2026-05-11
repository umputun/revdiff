---
name: revdiff
description: Pi-only interactive diff and file review with revdiff. Use when the user wants to review a diff, browse files for annotation, or revisit the last captured revdiff comments inside pi.
---

# revdiff for pi

This skill is specific to the **pi** harness.
Use the revdiff pi extension for interactive review sessions.

## Commands

- `/revdiff [args]` — launch revdiff, capture annotations, and open the results side panel
- `/revdiff-rerun [--pi-overlay|--pi-direct]` — rerun the last review with remembered args
- `/revdiff-results` — reopen the last captured results panel
- `/revdiff-apply` — send the last captured annotations to the agent as a user request
- `/revdiff-clear` — clear the stored review state widget/panel
- `/revdiff-reminders on|off` — enable or disable post-edit review reminders

## Recommended usage

Examples:

```text
/revdiff
/revdiff HEAD~1
/revdiff main
/revdiff --staged
/revdiff --untracked
/revdiff --all-files --include src
/revdiff --all-files --exclude vendor
/revdiff --only README.md
/revdiff HEAD~3 --description="why this refactor matters"
```

Behavior:

- With no arguments, the extension uses smart detection:
  - on main/master with uncommitted changes → review uncommitted changes
  - on main/master with a clean tree → review `HEAD~1`
  - on a clean feature branch → review against the detected main branch
  - on a dirty feature branch → asks whether to review uncommitted changes or the branch diff
- After revdiff exits, annotations are parsed and shown in a grouped right-side overlay panel
- A persistent widget is shown below the editor until cleared or until a clean re-review produces no annotations
- `/revdiff-rerun` remembers the last args, so the review loop stays tight
- Optional post-edit reminders can suggest `/revdiff` or `/revdiff-rerun` after the agent uses `edit`/`write`
- `/revdiff-apply` packages the structured annotations and sends them back to the agent for implementation

## Notes

- The default mode launches the external `revdiff` binary in the current terminal session, temporarily suspending pi while revdiff is running
- Optional overlay mode (`--pi-overlay` or `REVDIFF_PI_MODE=overlay`) reuses the existing `launch-revdiff.sh` script from the Claude plugin integration
- If `revdiff` is not on `PATH`, set `REVDIFF_BIN` to its absolute path
- You can still use revdiff standalone outside pi; the extension is only a convenience layer around the existing binary
