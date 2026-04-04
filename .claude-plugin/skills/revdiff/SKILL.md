---
name: revdiff
description: Review git diffs with inline annotations in a TUI overlay, or answer questions about revdiff usage, configuration, themes, and keybindings. Opens revdiff in tmux/kitty/wezterm, captures annotations, and addresses them. Activates on "revdiff", "review diff", "annotate diff", "git review with revdiff", "interactive diff review", "revdiff config", "revdiff themes", "revdiff keybindings", "how to configure revdiff", "what themes does revdiff have".
argument-hint: 'optional git ref (e.g., HEAD~1, main)'
allowed-tools: [Bash, Read, Edit, Write, Grep, Glob]
---

# revdiff - TUI Diff Review

Review git diffs with inline annotations using revdiff TUI in a terminal overlay.

## Activation Triggers

- "revdiff", "review diff", "annotate diff"
- "revdiff HEAD~1", "revdiff main"

## Answering Questions

If the user asks a question about revdiff (configuration, themes, keybindings, installation, usage) rather than requesting a review session, consult the reference files in `references/` and answer directly. Do NOT launch the TUI for informational questions.

- `references/install.md` — installation methods and plugin setup
- `references/config.md` — config file, options, colors, chroma themes
- `references/usage.md` — examples, key bindings, output format

## How It Works

1. Launch revdiff in a terminal overlay (tmux popup, kitty overlay, or wezterm split-pane)
2. User navigates the diff, adds annotations on specific lines
3. On quit, annotations are captured from stdout
4. Claude reads annotations and addresses each one
5. Loop: re-launch revdiff to verify fixes, user can add more annotations
6. Done when user quits without annotations

## Workflow

### Step 0: Verify Installation

```bash
which revdiff
```

If not found, guide installation:
- `go install github.com/umputun/revdiff/cmd/revdiff@latest`

### Step 1: Determine Ref or File Review Mode

If `$ARGUMENTS` is a file path (e.g., `docs/plans/feature.md`, `/tmp/notes.txt`), use **file review mode**:
- Skip ref detection entirely
- Go directly to Step 2 with `--only=<filepath>` (no ref argument)
- Works both inside and outside a git repo — revdiff reads the file from disk as context-only

If `$ARGUMENTS` contains an explicit ref (e.g., `HEAD~1`, `main`), use it as-is.

If no ref provided, run the smart detection script:

```bash
${CLAUDE_PLUGIN_ROOT}/.claude-plugin/skills/revdiff/scripts/detect-ref.sh
```

The script outputs structured fields:
- `branch`, `main_branch`, `is_main`, `has_uncommitted`
- `suggested_ref` — the ref to pass to revdiff (empty = uncommitted changes)
- `needs_ask` — if `true`, ask the user before proceeding

**When `needs_ask: true`** (on a feature branch with uncommitted changes), use AskUserQuestion:
- **"Uncommitted only"** — pass no ref (review just working changes)
- **"Branch vs {main_branch}"** — pass main_branch as ref (full branch diff including uncommitted)

**When `needs_ask: false`**, use `suggested_ref` directly:
- On main + uncommitted → no ref (uncommitted changes)
- On main + clean → `HEAD~1` (last commit)
- On feature branch + clean → main branch name (full branch diff)

### Step 2: Launch Review

Run the launcher script:

```bash
${CLAUDE_PLUGIN_ROOT}/.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh [ref] [--staged] [--only=file1 --only=file2]
```

The script:
- Detects available terminal (tmux → kitty → wezterm)
- Launches revdiff in an overlay
- Captures annotation output to a temp file
- Prints captured annotations to stdout

### Step 3: Process Annotations

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

### Step 4: Plan Changes

Enter plan mode (EnterPlanMode) to analyze annotations:
- List each annotation with file and line reference
- Describe the planned change for each
- Get user approval before modifying code

### Step 5: Address Annotations

After plan approval, fix the actual source code. Each annotation is a directive.

### Step 6: Loop

After fixing, run the launcher script again with the same ref. The user can:
- Add more annotations → go back to Step 3
- Quit without annotations → review complete (no output)

### Step 7: Done

When the script produces no output, the review is complete. Inform the user.

## Example Session

```
User: "revdiff HEAD~1"
→ launch revdiff in tmux popup with HEAD~1 diff
→ user annotates: "handler.go:43 - use errors.Is()"
→ user quits
→ annotations captured
→ enter plan mode: "add errors.Is() check at handler.go:43"
→ user approves
→ fix applied
→ re-launch revdiff HEAD~1
→ user sees fix, quits without annotations
→ "review complete"
```
