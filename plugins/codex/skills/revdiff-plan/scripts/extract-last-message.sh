#!/usr/bin/env bash
# extract the last assistant message from a Codex rollout file.
# source: codex-specific, no Claude Code equivalent.
#
# usage: extract-last-message.sh [--skip-current] [rollout-file]
#   --skip-current  best-effort heuristic: use the second most recent rollout file
#                   by mtime instead of the newest. assumes the newest file belongs
#                   to the active session. may select the wrong file if concurrent
#                   codex sessions exist. use an explicit path argument for precision.
#   rollout-file    explicit path to a rollout JSONL file (overrides auto-detection)
# output: raw markdown text of the last assistant response (exits 1 with error to stderr if none found)

set -euo pipefail

# check jq is available
if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq is required but not installed" >&2
    echo "install: brew install jq (or see https://jqlang.github.io/jq/download/)" >&2
    exit 1
fi

SKIP_CURRENT=false
if [ "${1:-}" = "--skip-current" ]; then
    SKIP_CURRENT=true
    shift
fi

# explicit file path takes precedence over auto-detection
if [ -n "${1:-}" ]; then
    if [ ! -f "$1" ]; then
        echo "error: rollout file not found: $1" >&2
        exit 1
    fi
    ROLLOUT="$1"
else
    SESSIONS_DIR="${HOME}/.codex/sessions"
    if [ ! -d "$SESSIONS_DIR" ]; then
        echo "error: codex sessions directory not found: $SESSIONS_DIR" >&2
        echo "hint: run a codex session first to generate rollout files" >&2
        exit 1
    fi

    # find rollout files by modification time.
    # track both newest and second-newest for --skip-current support.
    # paths mix flat and hierarchical layouts, so lexicographic sort is unreliable.
    # use -nt (newer than) comparison to avoid macOS xargs running ls with no args when find is empty.
    NEWEST=""
    SECOND=""
    while IFS= read -r -d '' f; do
        if [ -z "$NEWEST" ] || [ "$f" -nt "$NEWEST" ]; then
            SECOND="$NEWEST"
            NEWEST="$f"
        elif [ -z "$SECOND" ] || [ "$f" -nt "$SECOND" ]; then
            SECOND="$f"
        fi
    done < <(find "$SESSIONS_DIR" -name '*rollout*.jsonl' -type f -print0 2>/dev/null)

    if [ -z "$NEWEST" ]; then
        echo "error: no rollout files found in $SESSIONS_DIR" >&2
        exit 1
    fi

    # when --skip-current is set and a second file exists, use it to avoid
    # extracting the active session's own assistant output.
    if $SKIP_CURRENT && [ -n "$SECOND" ]; then
        ROLLOUT="$SECOND"
    else
        ROLLOUT="$NEWEST"
    fi
fi

# extract last assistant message text from the rollout JSONL.
# each line is a JSON object; assistant messages have:
#   type: "response_item", payload.type: "message", payload.role: "assistant"
#   payload.content[].type: "output_text", payload.content[].text: "<message>"
# slurp all output_text entries and take the last one to preserve multi-line content.
MSG=$(jq -s -r '
    [.[]
     | select(.type == "response_item"
         and .payload.type == "message"
         and .payload.role == "assistant")
     | .payload.content[]
     | select(.type == "output_text")
     | .text
    ] | last // empty
' "$ROLLOUT" 2>/dev/null)

if [ -z "$MSG" ]; then
    echo "error: no assistant messages found in $ROLLOUT" >&2
    exit 1
fi

printf '%s\n' "$MSG"
