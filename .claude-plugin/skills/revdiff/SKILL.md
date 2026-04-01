---
name: revdiff
description: Review git diffs with inline annotations in a TUI overlay. Opens revdiff in tmux/kitty/wezterm, captures annotations, and addresses them. Activates on "revdiff", "review diff", "annotate diff", "git review with revdiff", "interactive diff review".
argument-hint: 'optional git ref (e.g., HEAD~1, main)'
allowed-tools: [Bash, Read, Edit, Write, Grep, Glob, EnterPlanMode]
---

# revdiff - TUI Diff Review

Review git diffs with inline annotations using revdiff TUI in a terminal overlay.

## Activation Triggers

- "revdiff", "review diff", "annotate diff"
- "revdiff HEAD~1", "revdiff main"

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

### Step 1: Determine Ref

Check `$ARGUMENTS` for optional git ref:
- If provided: use as the ref argument (e.g., `HEAD~1`, `main`)
- If empty: revdiff will diff uncommitted changes by default

### Step 2: Launch Review

Run the launcher script:

```bash
${CLAUDE_PLUGIN_ROOT}/.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh [ref] [--staged]
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
