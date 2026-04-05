#!/usr/bin/env bash
# launch revdiff for plan file review via terminal overlay.
# usage: launch-plan-review.sh <plan-file-path>
# output: annotations from revdiff stdout (empty if no annotations)

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "usage: launch-plan-review.sh <plan-file-path>" >&2
    exit 1
fi

PLAN_FILE="$1"

if [ ! -f "$PLAN_FILE" ]; then
    echo "error: file not found: $PLAN_FILE" >&2
    exit 1
fi

# resolve revdiff to absolute path so overlay shells can find it
REVDIFF_BIN=$(command -v revdiff 2>/dev/null || true)
if [ -z "$REVDIFF_BIN" ]; then
    echo "error: revdiff not found in PATH" >&2
    exit 1
fi

OUTPUT_FILE=$(mktemp /tmp/plan-review-output-XXXXXX)
trap 'rm -f "$OUTPUT_FILE"' EXIT

# make plan path absolute for the overlay shell
PLAN_ABS=$(cd "$(dirname "$PLAN_FILE")" && echo "$(pwd)/$(basename "$PLAN_FILE")")

REVDIFF_CMD="$REVDIFF_BIN --only=$PLAN_ABS --output=$OUTPUT_FILE"
OVERLAY_TITLE="plan: $(basename "$PLAN_FILE")"

# tmux: display-popup -E blocks until command exits
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    tmux display-popup -E -w 90% -h 90% -T " $OVERLAY_TITLE " -- sh -c "$REVDIFF_CMD"
    cat "$OUTPUT_FILE"
    exit 0
fi

# kitty: overlay with sentinel file for blocking
KITTY_SOCK="${KITTY_LISTEN_ON:-}"
if [ -n "$KITTY_SOCK" ] && command -v kitty >/dev/null 2>&1; then
    SENTINEL=$(mktemp /tmp/plan-review-done-XXXXXX)
    rm -f "$SENTINEL"

    KITTY_ARGS=(kitty @ --to "$KITTY_SOCK" launch --type=overlay --title="$OVERLAY_TITLE")
    if [ -n "${KITTY_WINDOW_ID:-}" ]; then
        KITTY_ARGS+=(--match "id:${KITTY_WINDOW_ID}")
    fi
    KITTY_ARGS+=(sh -c "$REVDIFF_CMD; touch '$SENTINEL'")

    "${KITTY_ARGS[@]}" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rm -f "$SENTINEL"
    cat "$OUTPUT_FILE"
    exit 0
fi

# wezterm: split-pane with sentinel file for blocking
if [ -n "${WEZTERM_PANE:-}" ] && command -v wezterm >/dev/null 2>&1; then
    SENTINEL=$(mktemp /tmp/plan-review-done-XXXXXX)
    rm -f "$SENTINEL"

    wezterm cli split-pane --bottom --percent 90 \
        --pane-id "$WEZTERM_PANE" -- sh -c "$REVDIFF_CMD; touch '$SENTINEL'" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rm -f "$SENTINEL"
    cat "$OUTPUT_FILE"
    exit 0
fi

echo "error: no overlay terminal available (requires tmux, kitty, or wezterm)" >&2
exit 1
