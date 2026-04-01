#!/usr/bin/env bash
# launch revdiff in a terminal overlay (tmux/kitty/wezterm) and capture annotations.
# usage: launch-revdiff.sh [ref] [--staged]
# output: annotation text from revdiff stdout (empty if no annotations)

set -euo pipefail

# build revdiff command
REVDIFF_CMD="revdiff"
REVDIFF_ARGS=("$@")

# temp file for capturing annotations
OUTPUT_FILE=$(mktemp /tmp/revdiff-output-XXXXXX)
trap 'rm -f "$OUTPUT_FILE"' EXIT

# tmux: display-popup -E blocks until command exits
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    tmux display-popup -E -w 90% -h 90% -T " revdiff " -- \
        sh -c "$REVDIFF_CMD ${REVDIFF_ARGS[*]+"${REVDIFF_ARGS[*]}"} > '$OUTPUT_FILE' 2>/dev/null"
    cat "$OUTPUT_FILE"
    exit 0
fi

# kitty: overlay with sentinel file for blocking
KITTY_SOCK="${KITTY_LISTEN_ON:-}"
if [ -n "$KITTY_SOCK" ] && command -v kitty >/dev/null 2>&1; then
    SENTINEL=$(mktemp /tmp/revdiff-done-XXXXXX)
    rm -f "$SENTINEL"

    LAUNCH_CMD="$REVDIFF_CMD ${REVDIFF_ARGS[*]+"${REVDIFF_ARGS[*]}"} > '$OUTPUT_FILE' 2>/dev/null; touch '$SENTINEL'"

    KITTY_ARGS=(kitty @ --to "$KITTY_SOCK" launch --type=overlay --title="revdiff")
    if [ -n "${KITTY_WINDOW_ID:-}" ]; then
        KITTY_ARGS+=(--match "id:${KITTY_WINDOW_ID}")
    fi
    KITTY_ARGS+=(sh -c "$LAUNCH_CMD")

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
    SENTINEL=$(mktemp /tmp/revdiff-done-XXXXXX)
    rm -f "$SENTINEL"

    LAUNCH_CMD="$REVDIFF_CMD ${REVDIFF_ARGS[*]+"${REVDIFF_ARGS[*]}"} > '$OUTPUT_FILE' 2>/dev/null; touch '$SENTINEL'"

    wezterm cli split-pane --bottom --percent 90 \
        --pane-id "$WEZTERM_PANE" -- sh -c "$LAUNCH_CMD" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rm -f "$SENTINEL"

    cat "$OUTPUT_FILE"
    exit 0
fi

echo "error: no overlay terminal available (requires tmux, kitty, or wezterm)" >&2
exit 1
