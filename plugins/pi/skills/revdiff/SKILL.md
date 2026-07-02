---
name: revdiff
description: Pi-only interactive diff and file review with revdiff. Use when the user explicitly asks for revdiff, interactive annotations, or captured revdiff comments inside pi.
---

# revdiff for pi

This skill is specific to the **pi** harness.
Use the revdiff pi extension for interactive review sessions.

## Agent usage

Call the `revdiff_review` tool only when the user explicitly asks for revdiff, an interactive annotation pass, or captured revdiff annotations. Do **not** call it for ordinary autonomous requests like "review the code", "review my changes", or "review the diff"; handle those by inspecting the code directly. Do **not** tell the user to run `/revdiff`; slash commands are user-invoked only.

When the user invokes `/skill:revdiff <input>`, treat `<input>` as a request to launch revdiff unless it is clearly a usage/configuration question or an existing-history request. First figure out the concrete reference point(s) or file target the user asked for, then call `revdiff_review`. Do not stop after printing the resolved ref.

Reference resolution rules:

- Accept natural language. Resolve the user's requested target to concrete revdiff args before launching.
- If the request identifies another working directory (for example `in ~/source/repo`), use that directory for any git/ref/path resolution, omit that directory phrase from the revdiff args, and pass the directory as the `revdiff_review` tool's `cwd` parameter.
- For commit-count requests, use the matching git rev: `prev commit`, `previous commit`, `last commit` → `HEAD~1`; `head-3`, `head 3`, `HEAD~3`, `previous 3 commits`, `last 3 commits` → `HEAD~3`.
- For tag requests, resolve the actual tag first. `last tag` or `latest tag` → run `git describe --tags --abbrev=0`, then pass that tag as `args`.
- For date requests, resolve the commit first. Examples: `2 weeks ago`, `yesterday`, `last Friday` → run `git rev-list -1 --before=<phrase> HEAD`, then pass the resulting commit hash as `args`.
- For file targets, use `args: "--only <path>"`.
- For all-files requests, map excludes explicitly. Example: `all files exclude vendor and dist` → `args: "--all-files --exclude=vendor --exclude=dist"`.
- For explicit refs, ranges, flags, or two-ref requests, pass them through as revdiff args.
- If the requested natural language target cannot be resolved, say what failed and ask for a concrete ref/path. Do not guess silently.

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
- `args: "main", cwd: "~/source/repo"`: run revdiff from a specific working directory when the session was started elsewhere.
- `args: "--stdin"` with a unified diff piped to stdin (e.g. `gh pr diff 123`, `git format-patch -1 --stdout`, a `.patch` file): review a multi-file diff that lives outside the working tree — revdiff parses it as a real diff (one tree entry per file, hunk navigation, per-file annotations) instead of a context-only buffer

After `revdiff_review` returns annotations, address them directly from the tool result content. Do not read revdiff history after a successful captured-annotation result. Exit code `10` is success-with-annotations and is handled by the extension; do not report it as a failure. If it returns no annotations, report that no annotations were captured and stop. Do not relaunch revdiff after any no-annotation result unless the user explicitly asks for another review.

## Annotation handling loop

When annotations arrive from `/revdiff` or `revdiff_review`:

1. If any `revdiff_review` call returns no annotations, stop. Do not relaunch revdiff after a no-annotation result unless the user explicitly asks for another review.
2. Classify each annotation into:
   - **explanation requests**: questions or requests to explain/clarify behavior
   - **code-change directives**: requested repository changes
3. Answer explanation requests first in normal chat. Do not open another revdiff session just to show the explanation.
4. If all annotations are explanation requests and no repository files change, ask the user to choose between:
   - `Continue review` — rerun the original `revdiff_review` target
   - `Done with review` — stop
5. Before editing repository files, list the planned file/code changes.
6. Apply code-change directives.
7. Rerun the original `revdiff_review` target only after repository files changed or when the user chooses to continue reviewing; preserve the original `cwd` parameter when one was used.
8. Add `--untracked` on reruns when agent-created files should be included.

## User commands

- `/revdiff [args]` — alias for `/skill:revdiff [args]`; the skill resolves the request and calls `revdiff_review`.
- `/skill:revdiff <request>` — explicit skill command; same behavior as `/revdiff <request>`.

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
gh pr diff 123 | revdiff --stdin
git format-patch -1 --stdout | revdiff --stdin
```

## Recommended natural-language examples

```text
/revdiff prev commit
/revdiff last tag
/revdiff 2 weeks ago
/revdiff all files exclude vendor and dist
/revdiff README.md
```

Behavior:

- With no arguments, the extension uses smart detection:
  - on main/master with staged-only changes → review staged changes with `--staged`
  - on main/master with uncommitted changes → review uncommitted changes
  - on main/master with a clean tree → review `HEAD~1`
  - on a clean feature branch → review against the detected main branch
  - on a dirty feature branch → asks whether to review uncommitted changes or the branch diff; staged-only uncommitted review uses `--staged`
- After revdiff exits with annotations, `revdiff_review` returns them in the tool result; the agent processes that result directly.
- If revdiff exits without captured annotations, report that no annotations were captured and stop.
- When recent agent work created new untracked files, include `--untracked` so those files appear in the review tree.
- When launching after analysis or refactor work, include `--description` or `--description-file` so the info popup explains the review context.

## Existing review history

If the user says "use my latest revdiff annotations", "pull up my last revdiff review", or similar, do not launch revdiff again. Read the newest markdown file from the revdiff history directory and process its annotations through the same annotation handling loop. Also use history as a fallback only when a revdiff launch reports annotations but the tool did not return annotation text, or when the review did not complete and recent history may contain the saved annotations.

- Use `$REVDIFF_HISTORY_DIR` when set; otherwise use `~/.config/revdiff/history/`.
- Prefer the history subdirectory matching the current repository root name when present.
- History files contain annotation blocks in `## file:line (type)` format, usually followed by captured diff context.

## In-session review preload

When the user wants to review comments already present in the current conversation, write those comments to a temporary markdown file using revdiff's annotation output format, then run `revdiff_review` with `--annotations=<tempfile>` plus the normal review target args. This preloads the notes into the review session so the user can accept, edit, or add annotations in context.

## Notes

- The extension launches the external `revdiff` binary in the current terminal session, temporarily suspending pi while revdiff is running.
- If `revdiff` is not on `PATH`, set `REVDIFF_BIN` to its absolute path.
- The extension sets `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`; `10` means annotations were captured, not failure.
- Inside a review the user can press `O` to flush annotations to the output file mid-session, but the pi flow does not need it: pi is suspended until revdiff exits and returns the captured annotations on quit. The keep-open flush loop matters only for standalone use outside pi.
- You can still use revdiff standalone outside pi; the extension is only a convenience layer around the existing binary.
