---
name: revdiff-plan
description: Review the last Codex assistant message (plan, analysis, or proposal) with inline annotations in a TUI overlay. Extracts the most recent response from Codex rollout files and opens it in revdiff for review and annotation. Activates on "revdiff-plan", "review plan with revdiff", "annotate plan", "review last response", "annotate codex output".
argument-hint: 'none'
allowed-tools: [Bash, Read, Edit, Write, Grep, Glob]
---

# revdiff-plan - Review Codex Output

Review the last Codex assistant message with inline annotations using revdiff TUI in a terminal overlay.

## Script Path Resolution

Resolve the script directory using repo root first, then fall back to Codex home:

```bash
SCRIPT_DIR="$(git rev-parse --show-toplevel 2>/dev/null)/plugins/codex/skills/revdiff-plan/scripts"
if [ ! -d "$SCRIPT_DIR" ]; then
    SCRIPT_DIR="${CODEX_HOME:-$HOME/.codex}/plugins/revdiff/skills/revdiff-plan/scripts"
fi
```

Also resolve the launcher script from the revdiff skill:

```bash
LAUNCHER_DIR="$(git rev-parse --show-toplevel 2>/dev/null)/plugins/codex/skills/revdiff/scripts"
if [ ! -d "$LAUNCHER_DIR" ]; then
    LAUNCHER_DIR="${CODEX_HOME:-$HOME/.codex}/plugins/revdiff/skills/revdiff/scripts"
fi
```

Use `$SCRIPT_DIR` and `$LAUNCHER_DIR` in place of script paths throughout this skill.

## Activation Triggers

- "revdiff-plan", "review plan with revdiff", "annotate plan"
- "review last response", "annotate codex output"

## How It Works

1. Extract the last Codex assistant message from rollout files
2. Write it to a temp markdown file
3. Launch revdiff with `--only=<tempfile>` in a terminal overlay
4. User reads the plan, adds annotations on specific lines
5. On quit, annotations are captured from stdout
6. Codex reads annotations and addresses each one (refine plan, answer questions, fix issues)
7. Loop: re-launch revdiff to verify changes, user can add more annotations
8. Done when user quits without annotations; clean up temp file

## Workflow

### Step 0: Verify Installation

```bash
which revdiff
```

If not found, guide installation:
- `brew install umputun/apps/revdiff`
- Binary releases: https://github.com/umputun/revdiff/releases

Also verify jq is installed (required for rollout extraction):
```bash
which jq
```

If not found: `brew install jq`

### Step 1: Extract Last Assistant Message

Run the extraction script with `--skip-current` to avoid picking up this session's own output:

```bash
$SCRIPT_DIR/extract-last-message.sh --skip-current
```

The script:
- Uses a best-effort heuristic: picks the second most recent rollout JSONL file by modification time from `~/.codex/sessions/`
- This assumes the newest file belongs to the active session; if concurrent Codex sessions exist, the wrong file may be selected
- Falls back to the newest file if only one session exists
- Extracts the last assistant message text using jq
- Outputs raw markdown to stdout
- Accepts an explicit rollout file path as an argument to bypass auto-detection when precision matters

If the script fails (no sessions, no messages), inform the user and stop.
If the user reports that the wrong content was extracted, ask them to provide the rollout file path
explicitly: `$SCRIPT_DIR/extract-last-message.sh /path/to/rollout.jsonl`

Capture the output and write it to a temp file:

```bash
TMPBASE="${TMPDIR:-/tmp}"
PLAN_FILE=$(mktemp "$TMPBASE/revdiff-plan-XXXXXX.md")
$SCRIPT_DIR/extract-last-message.sh --skip-current > "$PLAN_FILE"
```

### Step 2: Launch Review

Run the launcher script with `--only=<tempfile>`:

```bash
$LAUNCHER_DIR/launch-revdiff.sh --only="$PLAN_FILE"
```

**IMPORTANT -- long-running command**: The launcher blocks until the user finishes reviewing in the TUI overlay. Set the bash timeout parameter to the **maximum your harness allows** (e.g. 1800000 or higher). Do NOT use `run_in_background`.

If the bash tool reports a timeout, use the same fallback as the revdiff skill:

1. Tell the user: "The bash tool timed out, but revdiff may still be open. Let me know when you're done reviewing."
2. Wait for the user to reply.
3. Read the most recent output file:
   ```bash
   output_file="$(ls -t "${TMPDIR:-/tmp}"/revdiff-output-* 2>/dev/null | head -1)"
   if [ -n "$output_file" ] && [ -f "$output_file" ]; then
     cat "$output_file"
   fi
   ```

### Step 3: Process Annotations

If the launcher produces output, the user made annotations. The output format is:

```
## plan-XXXXXX.md:12 ( )
this section needs more detail about error handling

## plan-XXXXXX.md:25 ( )
explain why we chose this approach over alternatives
```

Each annotation block has:
- `## filename:line (type)` -- which file and line
- Comment text below -- what the user wants changed or clarified

### Step 3.5: Classify Annotations

Split annotations into two categories:

**Explanation requests** -- annotation text starts with (case-insensitive): `explain`, `remind`, `describe`, `what is`, `what are`, `how does`, `how do`, `clarify`. These are questions the user wants answered, not plan changes.

**Plan-change directives** -- everything else. These are instructions to modify the plan content.

**If explanation requests are found:**

1. Answer each explanation request directly
2. If there are also plan-change directives, note them as pending
3. Enter the **explanation loop**:

   a. Write the explanation to a temp markdown file
   b. Launch revdiff with `--only=<explanation-file>` via the launcher script
   c. If user quits without annotations -- explanation accepted, proceed to pending directives or Step 5
   d. If user annotates -- refine explanation, loop back to (b)

**If no explanation requests** -- proceed directly to Step 4.

### Step 4: Address Annotations

For plan-change directives:
- Update the temp plan file with the requested changes
- Rewrite `$PLAN_FILE` with the updated content

### Step 5: Loop

After addressing annotations, re-launch revdiff with the updated plan:

```bash
$LAUNCHER_DIR/launch-revdiff.sh --only="$PLAN_FILE"
```

The user can:
- Add more annotations -- go back to Step 3
- Quit without annotations -- review complete

### Step 6: Done

When the launcher produces no output, the review is complete.

Clean up the temp file:

```bash
rm -f "$PLAN_FILE"
```

Inform the user that the plan review is complete. If the plan was modified during the review, present the final version.
