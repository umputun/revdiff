---
name: revdiff
description: Pi-only interactive diff and file review with revdiff. Use when the user explicitly asks for revdiff, interactive annotations, or captured revdiff comments inside pi.
---

# revdiff for pi

This skill is specific to the **pi** harness.
Use the revdiff pi extension for interactive review sessions.

## Agent usage

Call the `revdiff_review` tool only when the user explicitly asks for revdiff, an interactive annotation pass, or captured revdiff annotations. Do **not** call it for ordinary autonomous requests like "review the code", "review my changes", or "review the diff"; handle those by inspecting the code directly. Do **not** tell the user to run `/revdiff`; slash commands are user-invoked only.

Tool examples:

- No args: smart detection, same default target as `/revdiff`
- `args: "main"`: review the current branch against `main`
- `args: "--staged"`: review staged changes
- `args: "--untracked"`: review untracked files with working-tree changes
- `args: "--only README.md"`: review one standalone file
- `args: "--all-files --exclude vendor"`: review all tracked files except vendor
- `args: "--description='why this refactor matters' main"`: include review context in the info popup
- `args: "--description-file=/tmp/revdiff-desc.md main"`: include longer markdown review context
- `args: "--annotations=/tmp/revdiff-review.md main"`: preload in-session review notes

After `revdiff_review` returns annotations, address them directly. Exit code `10` is success-with-annotations and is handled by the extension; do not report it as a failure. If it returns no annotations, report that the review was clean.

## Annotation handling loop

When annotations arrive from `/revdiff` or `revdiff_review`:

1. Classify each annotation into:
   - **explanation requests**: questions or requests to explain/clarify behavior
   - **code-change directives**: requested repository changes
2. Answer explanation requests first.
3. If an explanation answer needs user review, write it to a temporary markdown file and run `revdiff_review` with `args: "--only <tempfile>"`. Refine the explanation and rerun until that explanation review returns clean.
4. Before editing repository files, list the planned file/code changes.
5. Apply code-change directives.
6. Rerun `revdiff_review` with the same args until no annotations are captured.
7. Add `--untracked` on reruns when agent-created files should be included.

## User commands

- `/revdiff [args]` — launch revdiff through direct terminal handoff, capture annotations, and send them to the agent immediately

## Recommended user command examples

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
/revdiff HEAD~3 --description-file=/tmp/revdiff-desc.md
/revdiff main --annotations=/tmp/revdiff-review.md
```

Behavior:

- With no arguments, the extension uses smart detection:
  - on main/master with staged-only changes → review staged changes with `--staged`
  - on main/master with uncommitted changes → review uncommitted changes
  - on main/master with a clean tree → review `HEAD~1`
  - on a clean feature branch → review against the detected main branch
  - on a dirty feature branch → asks whether to review uncommitted changes or the branch diff; staged-only uncommitted review uses `--staged`
- After revdiff exits with annotations, the extension sends a user message to the agent immediately; the agent continues the loop with `revdiff_review`.
- If revdiff exits without annotations, the review is clean.
- When recent agent work created new untracked files, include `--untracked` so those files appear in the review tree.
- When launching after analysis or refactor work, include `--description` or `--description-file` so the info popup explains the review context.

## Existing review history

If the user says "use my latest revdiff annotations", "pull up my last revdiff review", or similar, do not launch revdiff again. Read the newest markdown file from the revdiff history directory and process its annotations through the same annotation handling loop.

- Use `$REVDIFF_HISTORY_DIR` when set; otherwise use `~/.config/revdiff/history/`.
- Prefer the history subdirectory matching the current repository root name when present.
- History files contain annotation blocks in `## file:line (type)` format, usually followed by captured diff context.

## In-session review preload

When the user wants to review comments already present in the current conversation, write those comments to a temporary markdown file using revdiff's annotation output format, then run `revdiff_review` with `--annotations=<tempfile>` plus the normal review target args. This preloads the notes into the review session so the user can accept, edit, or add annotations in context.

## Notes

- The extension launches the external `revdiff` binary in the current terminal session, temporarily suspending pi while revdiff is running.
- If `revdiff` is not on `PATH`, set `REVDIFF_BIN` to its absolute path.
- The extension sets `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`; `10` means annotations were captured, not failure.
- You can still use revdiff standalone outside pi; the extension is only a convenience layer around the existing binary.
