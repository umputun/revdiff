#!/usr/bin/env bash
# launch revdiff in a terminal overlay (tmux/kitty/wezterm/ghostty) and capture annotations.
# usage: launch-revdiff.sh [ref] [--staged] [--only=file1 ...]
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

CONFIG_FLAG=""
if [ -n "${REVDIFF_CONFIG:-}" ] && [ -f "$REVDIFF_CONFIG" ]; then
    CONFIG_FLAG="--config=$REVDIFF_CONFIG"
fi
REVDIFF_CMD="$REVDIFF_BIN $CONFIG_FLAG --output=$OUTPUT_FILE $*"
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

# popup size: override via REVDIFF_POPUP_WIDTH / REVDIFF_POPUP_HEIGHT env vars (tmux and wezterm)
POPUP_W="${REVDIFF_POPUP_WIDTH:-90%}"
POPUP_H="${REVDIFF_POPUP_HEIGHT:-90%}"

# tmux: display-popup -E blocks until command exits
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    tmux display-popup -E -w "$POPUP_W" -h "$POPUP_H" -T " $OVERLAY_TITLE " -d "$CWD" -- sh -c "$REVDIFF_CMD"
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

    WEZTERM_PCT="${REVDIFF_POPUP_HEIGHT:-90%}"
    WEZTERM_PCT="${WEZTERM_PCT%%%}"
    wezterm cli split-pane --bottom --percent "$WEZTERM_PCT" \
        --pane-id "$WEZTERM_PANE" --cwd "$CWD" -- sh -c "$REVDIFF_CMD; touch '$SENTINEL'" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rm -f "$SENTINEL"
    cat "$OUTPUT_FILE"
    exit 0
fi

# ghostty: split pane via AppleScript (macOS only, requires Ghostty 1.3.0+)
if [ "${TERM_PROGRAM:-}" = "ghostty" ] && command -v osascript >/dev/null 2>&1; then

    SENTINEL=$(mktemp /tmp/revdiff-done-XXXXXX)
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp /tmp/revdiff-launch-XXXXXX.sh)
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; touch '$SENTINEL'
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    GHOSTTY_TERM_ID=$(osascript - "$LAUNCH_SCRIPT" "$CWD" <<'APPLESCRIPT'
on run argv
    set launchScript to item 1 of argv
    set cwd to item 2 of argv
    tell application "Ghostty"
        set cfg to new surface configuration
        set command of cfg to launchScript
        set initial working directory of cfg to cwd
        set wait after command of cfg to false
        set ft to focused terminal of selected tab of front window
        set newTerm to split ft direction down with configuration cfg
        perform action "toggle_split_zoom" on newTerm
        return id of newTerm
    end tell
end run
APPLESCRIPT
    )
    if [ $? -ne 0 ]; then
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        exit 1
    fi

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    # close the split pane (dismisses "press any key" prompt)
    osascript - "$GHOSTTY_TERM_ID" <<'APPLESCRIPT' 2>/dev/null
on run argv
    tell application "Ghostty" to close terminal id (item 1 of argv)
end run
APPLESCRIPT
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    cat "$OUTPUT_FILE"
    exit 0
fi

echo "error: no overlay terminal available (requires tmux, kitty, wezterm, or ghostty on macOS)" >&2
exit 1
