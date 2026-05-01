#!/usr/bin/env bash
# launch revdiff for plan file review via terminal overlay.
# usage:
#   launch-plan-review.sh <plan-file-path>           # --only mode
#   launch-plan-review.sh <new-path> <old-path>      # --compare-old/--compare-new mode
#
# arg order in compare mode is (new, old), NOT (old, new): a stale 1-arg
# launcher (pre-compare-mode user override copied from master before this
# feature shipped) silently picks $1 as PLAN_FILE. Putting <new> first means
# the stale launcher degrades to --only of the NEW revision (legacy UX, no
# regression) instead of opening the OLD revision the user already reviewed.
# output: annotations from revdiff stdout (empty if no annotations)

set -euo pipefail

# Keep sq() defined here so REVDIFF_ARGS can pre-quote arguments before the
# REVDIFF_CMD template assembles the full shell command line.
sq() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

if [ $# -eq 1 ]; then
    PLAN_FILE="$1"
    if [ ! -f "$PLAN_FILE" ]; then
        echo "error: file not found: $PLAN_FILE" >&2
        exit 1
    fi
    PLAN_ABS=$(cd "$(dirname "$PLAN_FILE")" && echo "$(pwd)/$(basename "$PLAN_FILE")")
    REVDIFF_ARGS="$(sq "--only=$PLAN_ABS")"
elif [ $# -eq 2 ]; then
    NEW_FILE="$1"
    OLD_FILE="$2"
    if [ ! -f "$NEW_FILE" ]; then
        echo "error: file not found: $NEW_FILE" >&2
        exit 1
    fi
    if [ ! -f "$OLD_FILE" ]; then
        echo "error: file not found: $OLD_FILE" >&2
        exit 1
    fi
    NEW_ABS=$(cd "$(dirname "$NEW_FILE")" && echo "$(pwd)/$(basename "$NEW_FILE")")
    OLD_ABS=$(cd "$(dirname "$OLD_FILE")" && echo "$(pwd)/$(basename "$OLD_FILE")")
    REVDIFF_ARGS="$(sq "--compare-old=$OLD_ABS") $(sq "--compare-new=$NEW_ABS")"
    PLAN_FILE="$NEW_FILE"
    COMPARE_MODE=1
else
    echo "usage: launch-plan-review.sh <plan-file-path> | <new-path> <old-path>" >&2
    exit 1
fi
COMPARE_MODE="${COMPARE_MODE:-0}"

# resolve revdiff to absolute path so overlay shells can find it
REVDIFF_BIN=$(command -v revdiff 2>/dev/null || true)
if [ -z "$REVDIFF_BIN" ]; then
    echo "error: revdiff not found in PATH" >&2
    exit 1
fi

TMPBASE="${TMPDIR:-/tmp}"
CWD="$(pwd)"

OUTPUT_FILE=$(mktemp "$TMPBASE/plan-review-output-XXXXXX")
trap 'rm -f "$OUTPUT_FILE"' EXIT

REVDIFF_CMD="$(sq "$REVDIFF_BIN") $REVDIFF_ARGS $(sq "--output=$OUTPUT_FILE") $(sq --wrap)"
# in compare mode, default to --collapsed so the user reads the new state with
# new-line highlights instead of full +/- diff visual clutter — better UX for
# rolling plan-revision review where each round is a focused list of edits
if [ "$COMPARE_MODE" = "1" ]; then
    REVDIFF_CMD="$REVDIFF_CMD $(sq --collapsed)"
fi
OVERLAY_TITLE="plan: $(basename "$PLAN_FILE")"

# tmux: display-popup -E blocks until command exits
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    # -T (title) requires tmux 3.3+; skip on older versions
    TMUX_ARGS=(tmux display-popup -E -w 90% -h 90%)
    if [[ "$(tmux -V 2>/dev/null)" =~ ([0-9]+)\.([0-9]+) ]]; then
        if [ "${BASH_REMATCH[1]}" -gt 3 ] || { [ "${BASH_REMATCH[1]}" -eq 3 ] && [ "${BASH_REMATCH[2]}" -ge 3 ]; }; then
            TMUX_ARGS+=(-T " $OVERLAY_TITLE ")
        fi
    fi
    TMUX_ARGS+=(-- sh -c "$REVDIFF_CMD")
    "${TMUX_ARGS[@]}"
    cat "$OUTPUT_FILE"
    exit 0
fi

# zellij: floating pane with sentinel file carrying revdiff's exit code
if [ -n "${ZELLIJ:-}" ] && command -v zellij >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/plan-review-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; rc=\$?; printf "%s" "\$rc" > $(sq "$SENTINEL").tmp && mv -f $(sq "$SENTINEL").tmp $(sq "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    zellij run --floating --close-on-exit \
        --width 90 --height 90 \
        --name "$OVERLAY_TITLE" \
        -- "$LAUNCH_SCRIPT" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(cat "$SENTINEL" 2>/dev/null || echo 1)
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    cat "$OUTPUT_FILE"
    exit "${rc:-1}"
fi

# kitty: overlay with sentinel file carrying revdiff's exit code
KITTY_SOCK="${KITTY_LISTEN_ON:-}"
if [ -n "$KITTY_SOCK" ] && command -v kitty >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
    rm -f "$SENTINEL"
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT

    KITTY_ARGS=(kitty @ --to "$KITTY_SOCK" launch --type=overlay --title="$OVERLAY_TITLE" --cwd=current)
    if [ -n "${KITTY_WINDOW_ID:-}" ]; then
        KITTY_ARGS+=(--match "window_id:${KITTY_WINDOW_ID}")
    fi
    KITTY_ARGS+=(sh -c "cd $(sq "$CWD") && $REVDIFF_CMD; rc=\$?; printf %s \"\$rc\" > $(sq "$SENTINEL").tmp && mv -f $(sq "$SENTINEL").tmp $(sq "$SENTINEL")")

    "${KITTY_ARGS[@]}" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(cat "$SENTINEL" 2>/dev/null || echo 1)
    rm -f "$SENTINEL"
    cat "$OUTPUT_FILE"
    exit "${rc:-1}"
fi

# wezterm/kaku: split-pane with sentinel file for blocking
if [ -n "${WEZTERM_PANE:-}" ]; then
    WEZTERM_CLI=()
    if command -v wezterm >/dev/null 2>&1; then
        WEZTERM_CLI=(wezterm cli)
    elif command -v kaku >/dev/null 2>&1; then
        WEZTERM_CLI=(kaku cli)
    fi

    if [ ${#WEZTERM_CLI[@]} -gt 0 ]; then
        SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
        rm -f "$SENTINEL"
        trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT

        "${WEZTERM_CLI[@]}" split-pane --bottom --percent 90 \
            --pane-id "$WEZTERM_PANE" -- sh -c "$REVDIFF_CMD; rc=\$?; printf %s \"\$rc\" > $(sq "$SENTINEL").tmp && mv -f $(sq "$SENTINEL").tmp $(sq "$SENTINEL")" >/dev/null 2>&1

        while [ ! -f "$SENTINEL" ]; do
            sleep 0.3
        done
        rc=$(cat "$SENTINEL" 2>/dev/null || echo 1)
        rm -f "$SENTINEL"
        cat "$OUTPUT_FILE"
        exit "${rc:-1}"
    fi
fi

# cmux: split pane via cmux CLI (must precede ghostty — cmux also sets TERM_PROGRAM=ghostty)
if [ -n "${CMUX_SURFACE_ID:-}" ] && command -v cmux >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/plan-review-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; rc=\$?; printf "%s" "\$rc" > $(sq "$SENTINEL").tmp && mv -f $(sq "$SENTINEL").tmp $(sq "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    CMUX_NEW=$(cmux new-split down 2>&1) || true
    CMUX_SURF=$(echo "$CMUX_NEW" | grep -o 'surface:[0-9]*' | head -1 || true)

    # bail explicitly when we can't identify the new surface — otherwise
    # `cmux send` without --surface would target the caller's pane and
    # replace the user's interactive shell via `exec ...`
    if [ -z "$CMUX_SURF" ]; then
        echo "error: cmux new-split did not return a surface id: $CMUX_NEW" >&2
        exit 1
    fi

    # send exec command immediately — the pty input buffer holds the text
    # until the new pane's shell finishes initializing and reads it
    cmux send --surface "$CMUX_SURF" "exec $(sq "$LAUNCH_SCRIPT")\n" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(cat "$SENTINEL" 2>/dev/null || echo 1)
    cmux close-surface --surface "$CMUX_SURF" 2>/dev/null || true
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    cat "$OUTPUT_FILE"
    exit "${rc:-1}"
fi

# ghostty: split pane via AppleScript (macOS only, requires Ghostty 1.3.0+)
if [ "${TERM_PROGRAM:-}" = "ghostty" ] && command -v osascript >/dev/null 2>&1; then

    SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/plan-review-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; rc=\$?; printf "%s" "\$rc" > $(sq "$SENTINEL").tmp && mv -f $(sq "$SENTINEL").tmp $(sq "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    if ! GHOSTTY_TERM_ID=$(osascript - "$LAUNCH_SCRIPT" <<'APPLESCRIPT'
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
    ); then
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        exit 1
    fi

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(cat "$SENTINEL" 2>/dev/null || echo 1)
    osascript - "$GHOSTTY_TERM_ID" <<'APPLESCRIPT' 2>/dev/null
on run argv
    tell application "Ghostty" to close terminal id (item 1 of argv)
end run
APPLESCRIPT
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    cat "$OUTPUT_FILE"
    exit "${rc:-1}"
fi

# iterm2: split pane via AppleScript (macOS only)
if [ -n "${ITERM_SESSION_ID:-}" ] && command -v osascript >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/plan-review-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; rc=\$?; printf "%s" "\$rc" > "\$1.tmp" && mv -f "\$1.tmp" "\$1"
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    ITERM_UUID="${ITERM_SESSION_ID##*:}"

    ITERM_NEW_SESSION=$(osascript - "$ITERM_UUID" "$LAUNCH_SCRIPT" "$SENTINEL" <<'APPLESCRIPT' 2>&1
on run argv
    set targetId to item 1 of argv
    set launchScript to item 2 of argv
    set sentinel to item 3 of argv
    set cmd to quoted form of launchScript & " " & quoted form of sentinel
    tell application id "com.googlecode.iterm2"
        repeat with w in windows
            repeat with t in tabs of w
                repeat with s in sessions of t
                    if id of s is targetId then
                        set colCount to columns of s
                        set rowCount to rows of s
                        tell s
                            if colCount >= 160 and colCount > (rowCount * 2) then
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
    rc=$(cat "$SENTINEL" 2>/dev/null || echo 1)
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
    exit "${rc:-1}"
fi

# emacs vterm: open revdiff in a new vterm buffer via emacsclient
if [ "${INSIDE_EMACS:-}" = "vterm" ] && command -v emacsclient >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/plan-review-done-XXXXXX")
    rm -f "$SENTINEL" && mkfifo "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/plan-review-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    # FIFO carries revdiff's exit code as its first line so the outer wait
    # can propagate the actual status instead of swallowing it as 0.
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$REVDIFF_CMD; rc=\$?; echo "\$rc" > $(sq "$SENTINEL"); exit
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    EMACS_PID=$(emacsclient --eval '(emacs-pid)' 2>/dev/null | tr -d '"')
    VTERM_PID=$$
    if [ -z "$EMACS_PID" ] || ! [ "$EMACS_PID" -gt 0 ] 2>/dev/null; then
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        echo "error: emacs server not reachable" >&2
        exit 1
    fi
    while P=$(ps -o ppid= -p "$VTERM_PID" 2>/dev/null | tr -d ' '); [ "$P" != "$EMACS_PID" ] && [ "$P" != "1" ] && [ -n "$P" ]; do VTERM_PID=$P; done

    # escape backslashes then double quotes for elisp string embedding
    elisp_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
    ESCAPED_TITLE=$(elisp_escape "$OVERLAY_TITLE")
    ESCAPED_SCRIPT=$(elisp_escape "$LAUNCH_SCRIPT")

    emacsclient --eval "(progn (require 'cl-lib)
      (when-let* ((b (cl-find-if (lambda (b) (let ((p (get-buffer-process b))) (and p (= (process-id p) $VTERM_PID)))) (buffer-list)))
                  (w (get-buffer-window b t)))
        (set-frame-parameter (window-frame w) 'revdiff-caller t))
      (let* ((buf (generate-new-buffer \"*revdiff*\"))
             (win (display-buffer buf '((display-buffer-pop-up-frame)
                     (pop-up-frame-parameters . ((name . \"$ESCAPED_TITLE\")))))))
        (set-frame-parameter (window-frame win) 'revdiff-buf (buffer-name buf))))" >/dev/null 2>&1
    emacsclient --no-wait --eval "(progn (require 'cl-lib)
      (when-let* ((f (cl-find-if (lambda (f) (string= (frame-parameter f 'name) \"$ESCAPED_TITLE\")) (frame-list)))
                  (bn (frame-parameter f 'revdiff-buf))
                  (buf (get-buffer bn)))
        (with-current-buffer buf
          (let ((vterm-shell \"$ESCAPED_SCRIPT\"))
            (vterm-mode)))))" >/dev/null 2>&1

    read -r rc < "$SENTINEL"
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    emacsclient --no-wait --eval "(progn (require 'cl-lib)
      (when-let ((f (cl-find-if (lambda (f) (string= (frame-parameter f 'name) \"$ESCAPED_TITLE\")) (frame-list))))
        (let ((bn (frame-parameter f 'revdiff-buf)))
          (delete-frame f)
          (when-let ((b (and bn (get-buffer bn)))) (kill-buffer b))))
      (when-let ((f (cl-find-if (lambda (f) (frame-parameter f 'revdiff-caller)) (frame-list))))
        (set-frame-parameter f 'revdiff-caller nil)
        (select-frame-set-input-focus f)))" >/dev/null 2>&1
    cat "$OUTPUT_FILE"
    exit "${rc:-1}"
fi

echo "error: no overlay terminal available (requires tmux, zellij, kitty, wezterm, cmux, ghostty, iTerm2, or emacs vterm)" >&2
exit 1
