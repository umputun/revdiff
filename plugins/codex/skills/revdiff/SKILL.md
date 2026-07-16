---
name: revdiff
description: Review diffs, files, and documents with inline annotations in a TUI overlay, or answer questions about revdiff usage, configuration, themes, and keybindings. Opens revdiff in agterm/tmux/zellij/herdr/kitty/wezterm/cmux/ghostty/iterm2/emacs-vterm, captures annotations, and addresses them. Works in git, hg, and jj repos (auto-detected). Activates on "revdiff", "review diff", "review changes", "annotate diff", "git review with revdiff", "hg review with revdiff", "review jj change", "interactive diff review", "revdiff all files", "review all files", "browse all files", "revdiff <file>", "revdiff README.md", "revdiff /tmp/notes.txt", "review this file", "annotate this file", "review file with revdiff", "open this review in revdiff", "show review in revdiff", "review in revdiff", "revdiff config", "revdiff themes", "revdiff keybindings", "how to configure revdiff", "what themes does revdiff have".
argument-hint: 'optional: ref(s), "all files", or file path'
allowed-tools: [Bash, Read, Edit, Write, Grep, Glob]
---

# revdiff - TUI Diff Review

Review diffs with inline annotations using revdiff TUI in a terminal overlay. Works in git, hg, and jj repos (auto-detected).

## Script Path Resolution

Resolve the script directory using repo root first, then fall back to Codex home:

```bash
SCRIPT_DIR="$(git rev-parse --show-toplevel 2>/dev/null)/plugins/codex/skills/revdiff/scripts"
if [ ! -d "$SCRIPT_DIR" ]; then
    SCRIPT_DIR="${CODEX_HOME:-$HOME/.codex}/skills/revdiff/scripts"
fi
```

Use `$SCRIPT_DIR` in place of script paths throughout this skill.

**Note**: the launcher override chain (user via `${CLAUDE_PLUGIN_DATA}` → bundled) is Claude-only — codex users customize the launcher by editing `~/.codex/skills/revdiff/scripts/launch-revdiff.sh` directly.

## Activation Triggers

- "revdiff", "review diff", "review changes", "annotate diff"
- "revdiff HEAD~1", "revdiff main"
- "hg review with revdiff", "review jj change"
- "revdiff all files", "review all files", "browse all files"
- "revdiff all files exclude vendor"
- "revdiff README.md", "revdiff docs/plan.md", "revdiff /tmp/notes.txt" — single-file review (`--only` mode)
- "review this file", "annotate this file", "review file with revdiff"
- "open this review in revdiff", "show review in revdiff", "review in revdiff" — open an in-session review (preload mode)

## Answering Questions

If the user asks a question about revdiff (configuration, themes, keybindings, installation, usage) rather than requesting a review session, consult the reference files in `references/` and answer directly. Do NOT launch the TUI for informational questions.

- `references/install.md` — installation methods and plugin setup
- `references/config.md` — config file, options, colors, chroma themes
- `references/usage.md` — examples, key bindings, output format

## Using Existing Review History

If the user says things like "locate my review", "use my latest revdiff annotations", "pull up the review I just did in another terminal", or "what did I annotate earlier" — the user ran revdiff outside this plugin flow and wants Codex to process the stored annotations. Read the most recent file from the persistent history directory via the helper script, then process the annotations through Step 3.5 classification as if they had come from a fresh launcher call:

```bash
$SCRIPT_DIR/read-latest-history.sh
```

The script resolves the history dir from `$REVDIFF_HISTORY_DIR` (default `~/.config/revdiff/history`), finds the repo subdir via VCS root basename (jj/git/hg), and prints the newest `.md` file found. Each history file contains a header (path, refs, and — when available — a git commit hash), the annotations in `## file:line (type)` format, and the raw git diff for annotated files. The `commit:` line and diff block are captured from git only; in hg/jj repos the diff block will be empty and no commit hash is recorded. See `references/usage.md` "Review History" section for directory layout, stdin/only handling, and override options.

The history file is also the recovery path when a review is cut short by a lost connection: on signal termination (a SIGHUP from a dropped SSH/tmux client, or a SIGTERM) revdiff saves the current annotations to history — never the `-o` output — so `read-latest-history.sh` still recovers them.

## Opening an In-Session Review

When the user asks to open an in-session review in revdiff (the conversation already contains review comments produced earlier in the session), write those comments to a temp file (e.g. `/tmp/revdiff-review-XXXXXX.md`) using the format documented in `references/usage.md` ("Output Format" section), then run the normal launcher flow (Step 1 ref detection, Step 2 invocation) with `--annotations=<temp-path>` appended. Step 3 onward handles the curated annotations as usual.

## Reviewing a Diff That Lives Outside the Working Tree

Some review targets are not the current repo state: a GitHub PR diff, a patch file on disk, or `git format-patch -1 --stdout` output. Pipe the unified diff into `revdiff --stdin` and the input is parsed as a real multi-file diff (one tree entry per file, hunk navigation, per-file annotations) instead of a context-only buffer. revdiff auto-detects the unified-diff signature; on a malformed patch the input falls back silently to raw-text mode.

Use this instead of the normal launcher flow when:
- the user asks to "review PR #N", "review this patch", "review `gh pr diff` output", or supplies a patch URL/path
- the diff describes commits that are not checked out locally (e.g. someone else's branch on a remote-only PR)
- the user pastes a unified diff and asks for a review of *that diff*, not the working tree

Example invocations (route through the same launcher as the normal flow):

```bash
gh pr diff 123 | $SCRIPT_DIR/launch-revdiff.sh --stdin
git format-patch -1 --stdout | $SCRIPT_DIR/launch-revdiff.sh --stdin
cat /tmp/feature.patch | $SCRIPT_DIR/launch-revdiff.sh --stdin
```

`--stdin` is mutually exclusive with refs, `--staged`, `--only`, `--all-files`, `--include`, `--exclude`, and `--annotations`, so do not combine with the Step 1 ref detection — go directly to Step 3 once the launcher returns. Annotations come back keyed by the real file paths from the diff (not by `--stdin-name`).

## How It Works

1. Launch revdiff in a terminal overlay (agterm full-pane overlay, tmux popup, Zellij floating pane, herdr tab, kitty overlay, wezterm/Kaku split-pane, cmux split, ghostty split+zoom, iTerm2 split pane, or Emacs vterm frame)
2. User navigates the diff, adds annotations on specific lines
3. On quit, annotations are captured from stdout
4. Codex reads annotations and addresses each one
5. Loop: re-launch revdiff to verify fixes, user can add more annotations
6. Done when user quits without annotations

## Workflow

### Step 0: Verify Installation

```bash
which revdiff
```

If not found, guide installation:
- `brew install umputun/apps/revdiff`
- Binary releases: https://github.com/umputun/revdiff/releases

### Step 1: Determine Review Mode

**All-files mode**: If `$ARGUMENTS` matches "all files", "all-files", or "browse all files" (with optional "exclude <prefix>" parts), use **all-files mode**:
- Pass `--all-files` to the launcher
- If user mentions exclude patterns (e.g., "exclude vendor", "exclude vendor and mocks"), pass each as `--exclude=<prefix>`
- Skip ref detection entirely, go directly to Step 2
- Example: "all files exclude vendor" → `--all-files --exclude=vendor`

**File review mode**: If `$ARGUMENTS` is a single token that points at a file on disk (e.g., `docs/plans/feature.md`, `/tmp/notes.txt`, `README.md`, `main.go`, `file.blah`), treat it as file review:
- Decide with `test -f "$ARGUMENTS"` — if the file exists, it's file review mode
- Also treat as file review if the token starts with `/` or `./`, or contains `/` and has a file extension (e.g., `src/app.go`), even when the file is not yet reachable from the current directory
- Skip ref detection entirely
- Go directly to Step 2 with `--only=<filepath>` (no ref argument)
- Works both inside and outside a VCS repo — revdiff reads the file from disk as context-only
- Ambiguous token (e.g., `main` — both a branch name and a potential filename without extension) → prefer ref mode; ask the user only if neither `test -f` nor `git rev-parse --verify` resolves

**Ref mode**: If `$ARGUMENTS` contains explicit ref(s) (e.g., `HEAD~1`, `main`, or `main feature` for two-ref diff), use as-is.

**Auto-detect**: If no ref provided, run the smart detection script:

```bash
$SCRIPT_DIR/detect-ref.sh
```

The script outputs structured fields:
- `branch`, `main_branch`, `is_main`, `has_uncommitted`, `has_staged_only`
- `suggested_ref` — the ref to pass to revdiff (empty = uncommitted changes)
- `use_staged` — if `true`, pass `--staged` to the launcher (staged-only changes detected)
- `needs_ask` — if `true`, ask the user before proceeding

**When `use_staged: true`**, pass `--staged` to the launcher. This means all changes are in the index (staged) with nothing unstaged — without `--staged`, revdiff would show an empty diff.

**When `needs_ask: true`** (on a feature branch with uncommitted changes), present the user with options as a numbered list and wait for their response:

1. **Uncommitted only** — pass no ref (review just working changes)
2. **Branch vs {main_branch}** — pass main_branch as ref (full branch diff including uncommitted)

**When `needs_ask: false`**, use `suggested_ref` directly:
- On main + uncommitted → no ref (uncommitted changes)
- On main + staged only → no ref + `--staged` (staged changes)
- On main + clean → `HEAD~1` (last commit)
- On feature branch + clean → main branch name (full branch diff)

### Step 2: Launch Review

When you are launching revdiff for the user (e.g., right after a refactor or analysis), pass `--description="..."` so the info popup (`i` key) explains what the change is and what to look at — markdown is supported. For longer prose, write the markdown to a temp file and pass `--description-file=/tmp/revdiff-desc-XXXXXX.md`. The two flags are mutually exclusive; both are optional. Skip when there's no useful context to add.

**When the recent change likely created new untracked files** (new packages, new test files, new docs, new scripts that haven't been `git add`-ed yet), pass `--untracked` so those files appear in the tree. Use this in working-tree mode (no ref, no `--staged`); skip it for ref-to-ref reviews where untracked files are not part of the historical diff.

Run the launcher script:

```bash
$SCRIPT_DIR/launch-revdiff.sh [base] [against] [--staged] [--untracked] [--only=file1] [--all-files] [--exclude=prefix] [--description=text|--description-file=path]
```

**IMPORTANT — long-running command**: The launcher blocks until the user finishes reviewing in the TUI overlay, which can exceed the default bash tool timeout. Set the bash timeout parameter to the **maximum your harness allows** (e.g. 1800000 or higher). Do NOT use `run_in_background` for this — background-task handling is unreliable for interactive TUI launchers. If the review outlasts the timeout cap, the fallback in Step 3 handles it.

**Disconnect-resilient tmux window mode**: when running under tmux, prefix the launcher with `REVDIFF_TMUX_WINDOW=1` to open revdiff in a persistent, server-owned tmux window instead of a client-owned `display-popup`. The review then survives a dropped SSH or tmux client — reattach and it is still there. This is a launcher environment variable, not a revdiff flag.

The script:
- Detects available terminal (agterm → tmux → Zellij → herdr → kitty → wezterm/Kaku → cmux → ghostty → iTerm2 → Emacs vterm)
- Launches revdiff in an overlay
- Captures annotation output to a temp file
- Prints captured annotations to stdout

The bundled launcher sets `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`; exit `10` means annotations were captured and is not a launcher failure. Treat other nonzero statuses as failures.

#### Agterm sessions and approval escalation

When Codex runs inside an agterm session, the launcher opens revdiff in agterm's native full-pane overlay — its first-choice backend, checked ahead of tmux. That branch needs `AGTERM_SESSION_ID` and `AGTERM_SOCKET` in the launcher's environment.

Codex commands that require approval or escalation run in a fresh environment that **strips every `AGTERM_*` variable** (along with `TERM_PROGRAM` and `GHOSTTY_*`), keeping only `PATH`. Launched from there without those variables, the launcher skips the agterm branch and fails with `no overlay terminal available`.

Before escalating, read the values in the normal (unescalated) environment and inline them as **literal** values in the approved command:

```bash
AGTERM_SESSION_ID='<captured-id>' \
AGTERM_SOCKET='<captured-socket>' \
$SCRIPT_DIR/launch-revdiff.sh --only=<file>
```

- Do NOT reference `$AGTERM_SESSION_ID` / `$AGTERM_SOCKET` inside the approved command — they are already unset there, so the expansion is empty. Read them first in the normal environment, then paste the literal strings.
- Do NOT work around the missing session id by targeting the `active` session (agterm's default): that can open revdiff in an unrelated agterm window or session. The captured session id keeps the overlay on the reviewing session.

### Step 3: Process Annotations

**Collecting launcher output**: In the normal case the launcher returns synchronously with annotations on stdout — process them as described below. If the bash tool reports exit `10`, read stdout and process it as annotations; do not call it a failure. If the bash tool instead reports a timeout, only the launcher process died, but revdiff itself is still open in the overlay and no annotations are lost: revdiff writes them to disk the moment the user quits, and `O` flushes them any time. Do NOT retry the launcher. Use the fallback:

1. Reassure the user and offer both paths, making clear nothing is lost — keep it short and do NOT explain the save mechanics (disk writes, `O` flush, quit-to-save); the user does not need them. Say something like: "The process waiting on your revdiff review timed out and exited — that's harmless, and any annotations you made are safe. Whenever you're done, message me and tell me to either load your annotations and continue, or that you're done and want to stop." Do NOT assume they want to load; quitting with no annotations, or choosing to stop, is a valid outcome.
2. Wait for the user to reply. They cannot respond while the overlay has focus, so their reply means they are back at the session (they quit, or flushed with `O` and switched back).
3. On their reply you MUST act; do not stop at step 1. If they chose to stop, acknowledge and end. Otherwise read the persisted annotations, most recent output file first (the launcher writes to `$TMPDIR` when set, falling back to `/tmp`):
   ```bash
   output_file="$(ls -t "${TMPDIR:-/tmp}"/revdiff-output-* 2>/dev/null | head -1)"
   if [ -n "$output_file" ] && [ -f "$output_file" ]; then
     cat "$output_file"
   fi
   ```
4. If the output file has content, process it as annotations below. If it is empty or missing, fall back to the durable review history, which survives even when the launcher's cleanup removed the temp file: run `$SCRIPT_DIR/read-latest-history.sh` and process the annotations from its `## Annotations` section (see "Using Existing Review History"). Only if both are empty did the user quit without annotating.

This fallback is safe because revdiff writes the output file atomically on exit, and the history entry is complete before the process exits — there is never a partial read.

A reviewer may also keep revdiff open on purpose and press `O` to flush the current annotations to the same output file mid-session, without quitting. The flush uses the same atomic write, so the fallback read above still returns a complete file. When the user says something like "I flushed my notes, go ahead" while the overlay is still open, read the most recent output file exactly as in the timeout fallback and process the annotations; do NOT relaunch revdiff. After you finish the code changes, the reviewer reloads with `R` and continues in the same session. No launcher flags change for this — the launcher already passes an output file, and `O` reuses it.

If the script produces output, the user made annotations. The output format is:

```
## file.go:43 (+)
use errors.Is() instead of direct comparison

## store.go:18 (-)
don't remove this validation
```

Each annotation block has:
- `## filename:line (type)` — which file and line, `(+)` = added, `(-)` = removed, `(file-level)` = file note
- Comment text below — what the user wants changed

### Step 3.5: Classify Annotations

Split annotations into two categories:

**Explanation requests** — annotation matches either rule (case-insensitive):
- contains two or more consecutive question marks anywhere in the text (`??`, `???`, etc.) — a language-neutral shortcut for "please explain"
- OR starts with one of: `explain`, `remind`, `describe`, `what is`, `what are`, `how does`, `how do`, `clarify`

These are questions the user wants answered, not code changes.

**Code-change directives** — everything else. These are instructions to modify code.

**If explanation requests are found:**

1. Answer each explanation request — read the referenced code, generate a clear markdown explanation
2. If there are also code-change directives in the same batch, note them as pending (they carry over to Step 4 after the explanation loop)
3. Enter the **explanation loop**:

   a. Write the explanation to a temp markdown file (e.g., `/tmp/revdiff-explain-XXXXXX.md`)
   b. Launch revdiff with `--only=/tmp/revdiff-explain-XXXXXX.md` via the launcher script — this opens the explanation as a scrollable markdown view with TOC sidebar
   c. **If user quits without annotations** → explanation accepted, clean up temp file, proceed:
      - If pending code-change directives exist → go to Step 4
      - Otherwise → go to Step 6 (re-launch revdiff with the original diff ref)
   d. **If user annotates the explanation** → these are follow-up questions or clarification requests. Read the annotations, refine/extend the explanation markdown, write updated temp file, go back to step (b)

The explanation loop continues until the user quits without annotating. This allows a natural back-and-forth dialogue where the user can ask for more detail or corrections on specific parts of the explanation.

**If no explanation requests** — all annotations are code-change directives, proceed directly to Step 4.

### Step 4: Plan Changes

Write the plan as a markdown list analyzing code-change annotations:
- List each annotation with file and line reference
- Describe the planned change for each
- Ask the user: "Proceed with these changes?"

Wait for user confirmation before modifying code.

### Step 5: Address Annotations

After user approves the plan, fix the actual source code. Each annotation is a directive.

### Step 6: Loop

After fixing (or after "Continue review" from Step 3.5), run the launcher script again with the same ref. The user can:
- Add more annotations → go back to Step 3
- Quit without annotations → review complete (no output)

### Step 7: Done

When the script produces no output, the review is complete. Inform the user.

## Example Sessions

```
User: "revdiff HEAD~1"
→ launch revdiff in tmux popup with HEAD~1 diff
→ user annotates: "handler.go:43 - use errors.Is()"
→ user quits
→ annotations captured
→ plan changes: "add errors.Is() check at handler.go:43"
→ user approves
→ fix applied
→ re-launch revdiff HEAD~1
→ user sees fix, quits without annotations
→ "review complete"
```

```
User: "revdiff HEAD~3"
→ launch revdiff in tmux popup with HEAD~3 diff
→ user annotates: "server.go:72 - explain what this mutex protects"
→ user quits
→ annotation classified as explanation request (starts with "explain")
→ Codex reads server.go:72, generates markdown explanation
→ writes to /tmp/revdiff-explain-XXXXXX.md
→ launch revdiff --only=/tmp/revdiff-explain-XXXXXX.md (explanation view with TOC)
→ user reads explanation, annotates: "what about the race condition on line 80?"
→ Codex refines explanation, rewrites temp file
→ re-launch revdiff --only=/tmp/revdiff-explain-XXXXXX.md
→ user reads updated explanation, quits without annotations
→ explanation accepted, clean up temp file
→ re-launch revdiff HEAD~3 (back to diff review)
→ user quits without annotations
→ "review complete"
```

```
User: "revdiff all files exclude vendor"
→ launch revdiff with --all-files --exclude=vendor
→ user browses all tracked files, annotates as needed
→ same annotation loop as above
```

```
User: "revdiff docs/plans/feature.md"
→ test -f docs/plans/feature.md succeeds → file review mode
→ launch revdiff with --only=docs/plans/feature.md (context-only view, no ref)
→ user annotates prose: "section 'Open questions':3 - drop this, resolved"
→ user quits
→ same annotation loop as above (applies to the file content)
```
