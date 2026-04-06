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

# ghostty: split pane via AppleScript (macOS only, requires Ghostty 1.3.0+)
if [ "${TERM_PROGRAM:-}" = "ghostty" ] && command -v osascript >/dev/null 2>&1; then

    SENTINEL=$(mktemp /tmp/plan-review-done-XXXXXX)
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp /tmp/plan-review-launch-XXXXXX.sh)
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; touch '$SENTINEL'
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    GHOSTTY_TERM_ID=$(osascript - "$LAUNCH_SCRIPT" <<'APPLESCRIPT'
on run argv
    set launchScript to item 1 of argv
    tell application "Ghostty"
        set cfg to new surface configuration
        set command of cfg to launchScript
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
    osascript - "$GHOSTTY_TERM_ID" <<'APPLESCRIPT' 2>/dev/null
on run argv
    tell application "Ghostty" to close terminal id (item 1 of argv)
end run
APPLESCRIPT
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    cat "$OUTPUT_FILE"
    exit 0
fi

# iterm2: split pane via AppleScript (macOS only)
if [ -n "${ITERM_SESSION_ID:-}" ] && command -v osascript >/dev/null 2>&1; then
    SENTINEL=$(mktemp /tmp/plan-review-done-XXXXXX)
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp /tmp/plan-review-launch-XXXXXX.sh)
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; touch "\$1"
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    ITERM_UUID="${ITERM_SESSION_ID##*:}"

    ITERM_NEW_SESSION=$(osascript - "$ITERM_UUID" "$LAUNCH_SCRIPT" "$SENTINEL" <<'APPLESCRIPT' 2>&1
on run argv
    set targetId to item 1 of argv
    set launchScript to item 2 of argv
    set sentinel to item 3 of argv
    set cmd to launchScript & " " & quoted form of sentinel
    tell application id "com.googlecode.iterm2"
        repeat with w in windows
            repeat with t in tabs of w
                repeat with s in sessions of t
                    if id of s is targetId then
                        set colCount to columns of s
                        set rowCount to rows of s
                        tell s
                            if colCount ≥ 160 and colCount > (rowCount * 2) then
                                set newSession to split vertically with same profile command cmd
                            else
                                set newSession to split horizontally with same profile command cmd
                            end if
                        end tell
                        return id of newSession
                    end if
                end repeat
            end repeat
        end repeat
    end tell
    error "session not found: " & targetId
end run
APPLESCRIPT
    ) || {
        echo "error: failed to open iTerm2 split via osascript: $ITERM_NEW_SESSION" >&2
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        exit 1
    }

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    osascript - "$ITERM_NEW_SESSION" <<'APPLESCRIPT' 2>/dev/null
on run argv
    set sid to item 1 of argv
    tell application id "com.googlecode.iterm2"
        repeat with w in windows
            repeat with t in tabs of w
                repeat with s in sessions of t
                    if id of s is sid then
                        tell s to close
                        return
                    end if
                end repeat
            end repeat
        end repeat
    end tell
end run
APPLESCRIPT
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    cat "$OUTPUT_FILE"
    exit 0
fi

echo "error: no overlay terminal available (requires tmux, kitty, wezterm, ghostty, or iTerm2)" >&2
exit 1
