#!/usr/bin/env bash
# launch revdiff in a terminal overlay (tmux/kitty/wezterm) and capture annotations.
# usage: launch-revdiff.sh [ref] [--staged]
# output: annotation text from revdiff stdout (empty if no annotations)

set -euo pipefail

# resolve revdiff to absolute path so overlay shells (sh -c) can find it
# even when /opt/homebrew/bin or similar dirs are not in sh's default PATH
REVDIFF_BIN=$(command -v revdiff 2>/dev/null || true)
if [ -z "$REVDIFF_BIN" ]; then
    echo "error: revdiff not found in PATH" >&2
    echo "install: go install github.com/umputun/revdiff/cmd/revdiff@latest" >&2
    exit 1
fi

OUTPUT_FILE=$(mktemp /tmp/revdiff-output-XXXXXX)
trap 'rm -f "$OUTPUT_FILE"' EXIT

REVDIFF_CMD="$REVDIFF_BIN --output=$OUTPUT_FILE $*"
CWD="$(pwd)"

# build descriptive title: "rd: dirname [ref]"
DIR_NAME=$(basename "$CWD")
TITLE_REF=""
SKIP_NEXT=0
for arg in "$@"; do
    if [ "$SKIP_NEXT" -eq 1 ]; then SKIP_NEXT=0; continue; fi
    case "$arg" in
        -o|--output) SKIP_NEXT=1 ;;
        --output=*) ;;
        -*) ;;
        *) TITLE_REF="$arg"; break ;;
    esac
done
OVERLAY_TITLE="rd: ${DIR_NAME}${TITLE_REF:+ [$TITLE_REF]}"

# tmux: display-popup -E blocks until command exits
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    tmux display-popup -E -w 90% -h 90% -T " $OVERLAY_TITLE " -d "$CWD" -- sh -c "$REVDIFF_CMD"
    cat "$OUTPUT_FILE"
    exit 0
fi

# kitty: overlay with sentinel file for blocking
KITTY_SOCK="${KITTY_LISTEN_ON:-}"
if [ -n "$KITTY_SOCK" ] && command -v kitty >/dev/null 2>&1; then
    SENTINEL=$(mktemp /tmp/revdiff-done-XXXXXX)
    rm -f "$SENTINEL"

    KITTY_ARGS=(kitty @ --to "$KITTY_SOCK" launch --type=overlay --title="$OVERLAY_TITLE" --cwd="$CWD")
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
    SENTINEL=$(mktemp /tmp/revdiff-done-XXXXXX)
    rm -f "$SENTINEL"

    wezterm cli split-pane --bottom --percent 90 \
        --pane-id "$WEZTERM_PANE" --cwd "$CWD" -- sh -c "$REVDIFF_CMD; touch '$SENTINEL'" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rm -f "$SENTINEL"
    cat "$OUTPUT_FILE"
    exit 0
fi

echo "error: no overlay terminal available (requires tmux, kitty, or wezterm)" >&2
exit 1
