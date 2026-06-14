#!/usr/bin/env bash
# launch revdiff in a terminal overlay (tmux/zellij/herdr/kitty/wezterm/cmux/ghostty/iterm2) and capture annotations.
# usage: launch-revdiff.sh [ref] [--staged] [--untracked] [--only=file1 ...]
# output: annotation text from revdiff stdout (empty if no annotations)
# exit: 0 clean, 10 annotations captured, other nonzero failure

set -euo pipefail

# resolve revdiff to absolute path so overlay shells (sh -c) can find it
# even when /opt/homebrew/bin or similar dirs are not in sh's default PATH
REVDIFF_BIN=$(command -v revdiff 2>/dev/null || true)
if [ -z "$REVDIFF_BIN" ]; then
    echo "error: revdiff not found in PATH" >&2
    echo "install: brew install umputun/apps/revdiff (or download from https://github.com/umputun/revdiff/releases)" >&2
    exit 1
fi

TMPBASE="${TMPDIR:-/tmp}"
OUTPUT_FILE=$(mktemp "$TMPBASE/revdiff-output-XXXXXX")
trap 'rm -f "$OUTPUT_FILE"' EXIT

# shell-quote a single argument for safe embedding in sh -c strings.
sq() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

REVDIFF_CMD="$(sq "$REVDIFF_BIN")"
if [ -n "${REVDIFF_CONFIG:-}" ] && [ -f "$REVDIFF_CONFIG" ]; then
    REVDIFF_CMD="$REVDIFF_CMD $(sq "--config=$REVDIFF_CONFIG")"
fi
# pass exit-code-on-annotations via env, not a CLI flag: an old revdiff binary
# silently ignores an unknown env var but hard-fails on an unknown flag
REVDIFF_CMD="REVDIFF_EXIT_CODE_ON_ANNOTATIONS=true $REVDIFF_CMD $(sq "--output=$OUTPUT_FILE")"
for arg in "$@"; do
    REVDIFF_CMD="$REVDIFF_CMD $(sq "$arg")"
done

write_rc_cmd() {
    local sentinel="$1"
    # single-quoted format keeps $?/$rc literal for the generated inner script
    # shellcheck disable=SC2016
    printf '%s; rc=$?; printf "%%s" "$rc" > %s.tmp && mv -f %s.tmp %s' \
        "$REVDIFF_CMD" "$(sq "$sentinel")" "$(sq "$sentinel")" "$(sq "$sentinel")"
}

write_fifo_rc_cmd() {
    local sentinel="$1"
    # single-quoted format keeps $?/$rc literal for the generated inner script
    # shellcheck disable=SC2016
    printf '%s; rc=$?; echo "$rc" > %s; exit' "$REVDIFF_CMD" "$(sq "$sentinel")"
}

read_rc() {
    cat "$1" 2>/dev/null || echo 1
}

print_output_and_exit() {
    local rc="${1:-0}"
    cat "$OUTPUT_FILE"
    exit "$rc"
}

is_cmux_session() {
    if [ -n "${CMUX_SURFACE_ID:-}" ]; then
        return 0
    fi
    if [ "${__CFBundleIdentifier:-}" = "com.cmuxterm.app" ]; then
        return 0
    fi
    case "${GHOSTTY_RESOURCES_DIR:-}:${GHOSTTY_BIN_DIR:-}" in
        *cmux.app*) return 0 ;;
    esac
    return 1
}

# overlay backends (kitty @ launch, tmux display-popup, zellij run, etc.) spawn
# children from a server/app process whose env predates user shell rc files,
# so EDITOR/VISUAL exports from .zshrc/.bashrc are otherwise lost. prepend
# `env KEY=VAL` so revdiff itself starts with the caller's editor env, which
# its multi-line annotation flow passes to the spawned editor child.
ENV_PREFIX=""
for _name in EDITOR VISUAL; do
    if [ "${!_name+x}" = x ]; then
        ENV_PREFIX="$ENV_PREFIX $(sq "${_name}=${!_name}")"
    fi
done
unset _name
if [ -n "$ENV_PREFIX" ]; then
    REVDIFF_CMD="/usr/bin/env$ENV_PREFIX $REVDIFF_CMD"
fi

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

# popup size: override via REVDIFF_POPUP_WIDTH / REVDIFF_POPUP_HEIGHT env vars (tmux, zellij, and wezterm)
POPUP_W="${REVDIFF_POPUP_WIDTH:-90%}"
POPUP_H="${REVDIFF_POPUP_HEIGHT:-90%}"

# tmux: display-popup -E blocks until command exits
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    # -T (title) requires tmux 3.3+; skip on older versions
    TMUX_ARGS=(tmux display-popup -E -w "$POPUP_W" -h "$POPUP_H")
    if [[ "$(tmux -V 2>/dev/null)" =~ ([0-9]+)\.([0-9]+) ]]; then
        if [ "${BASH_REMATCH[1]}" -gt 3 ] || { [ "${BASH_REMATCH[1]}" -eq 3 ] && [ "${BASH_REMATCH[2]}" -ge 3 ]; }; then
            TMUX_ARGS+=(-T " $OVERLAY_TITLE ")
        fi
    fi
    TMUX_ARGS+=(-d "$CWD" -- sh -c "$REVDIFF_CMD")
    rc=0
    "${TMUX_ARGS[@]}" || rc=$?
    print_output_and_exit "$rc"
fi

# zellij: floating pane with sentinel file for blocking
if [ -n "${ZELLIJ:-}" ] && command -v zellij >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$(write_rc_cmd "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    ZELLIJ_ORIG_TAB_ID=""
    if [ -n "${ZELLIJ_PANE_ID:-}" ] && command -v jq >/dev/null 2>&1; then
        ZELLIJ_ORIG_TAB_ID=$(zellij action list-panes --json --tab 2>/dev/null \
            | jq -r --arg p "$ZELLIJ_PANE_ID" \
                '.[] | select((.is_plugin // false) == false and .tab_id != null and .id == ($p | tonumber)) | .tab_id' 2>/dev/null \
            | head -1 || true)
    fi

    if [ -n "$ZELLIJ_ORIG_TAB_ID" ] && zellij run --floating --close-on-exit --tab-id "$ZELLIJ_ORIG_TAB_ID" \
            --width "$POPUP_W" --height "$POPUP_H" \
            --name "$OVERLAY_TITLE" --cwd "$CWD" \
            -- "$LAUNCH_SCRIPT" >/dev/null 2>&1; then
        :
    else
        zellij run --floating --close-on-exit \
            --width "$POPUP_W" --height "$POPUP_H" \
            --name "$OVERLAY_TITLE" --cwd "$CWD" \
            -- "$LAUNCH_SCRIPT" >/dev/null 2>&1
    fi

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(read_rc "$SENTINEL")
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    print_output_and_exit "${rc:-1}"
fi

# herdr: open a new fullscreen tab via the herdr CLI (must precede kitty —
# inside herdr-in-kitty KITTY_LISTEN_ON is set, so the kitty branch would
# otherwise win and open an overlay window herdr cannot composite into its panes)
if [ "${HERDR_ENV:-}" = "1" ] && command -v herdr >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$(write_rc_cmd "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    # create a focused tab (cwd set so revdiff runs in the caller's directory);
    # parse the new tab + root pane ids from the JSON reply
    HERDR_NEW=$(herdr tab create --cwd "$CWD" --label "$OVERLAY_TITLE" --focus 2>&1) || {
        echo "error: herdr tab create failed: $HERDR_NEW" >&2
        exit 1
    }
    # || true on both branches keeps a parse miss (jq exits nonzero on malformed
    # JSON, grep exits 1 on no match) from tripping set -e, so the explicit id
    # check below stays reachable to emit a real error and close any created tab
    if command -v jq >/dev/null 2>&1; then
        HERDR_TAB_ID=$(printf '%s' "$HERDR_NEW" | jq -r '.result.tab.tab_id // empty' 2>/dev/null || true)
        HERDR_PANE_ID=$(printf '%s' "$HERDR_NEW" | jq -r '.result.root_pane.pane_id // empty' 2>/dev/null || true)
    else
        HERDR_TAB_ID=$(printf '%s' "$HERDR_NEW" | grep -o '"tab_id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
        HERDR_PANE_ID=$(printf '%s' "$HERDR_NEW" | grep -o '"pane_id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
    fi

    # bail explicitly when ids are missing — sending the launch command into the
    # wrong pane would type it into the caller's interactive shell
    if [ -z "$HERDR_PANE_ID" ] || [ -z "$HERDR_TAB_ID" ]; then
        echo "error: herdr tab create did not return pane/tab ids: $HERDR_NEW" >&2
        if [ -n "$HERDR_TAB_ID" ]; then
            herdr tab close "$HERDR_TAB_ID" >/dev/null 2>&1 || true
        fi
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        exit 1
    fi

    if ! herdr pane run "$HERDR_PANE_ID" "sh $(sq "$LAUNCH_SCRIPT")" >/dev/null 2>&1; then
        echo "error: herdr pane run failed for pane $HERDR_PANE_ID" >&2
        herdr tab close "$HERDR_TAB_ID" >/dev/null 2>&1 || true
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        exit 1
    fi

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(read_rc "$SENTINEL")
    herdr tab close "$HERDR_TAB_ID" >/dev/null 2>&1 || true
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    print_output_and_exit "${rc:-1}"
fi

# kitty: overlay with sentinel file for blocking
KITTY_SOCK="${KITTY_LISTEN_ON:-}"
if [ -n "$KITTY_SOCK" ] && command -v kitty >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT

    KITTY_ARGS=(kitty @ --to "$KITTY_SOCK" launch --type=overlay --title="$OVERLAY_TITLE" --cwd=current)
    if [ -n "${KITTY_WINDOW_ID:-}" ]; then
        KITTY_ARGS+=(--match "window_id:${KITTY_WINDOW_ID}")
    fi
    KITTY_ARGS+=(sh -c "cd $(sq "$CWD") && $(write_rc_cmd "$SENTINEL")")

    "${KITTY_ARGS[@]}" >/dev/null 2>&1

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(read_rc "$SENTINEL")
    rm -f "$SENTINEL"
    print_output_and_exit "${rc:-1}"
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
        SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
        rm -f "$SENTINEL"

        WEZTERM_PCT="${REVDIFF_POPUP_HEIGHT:-90%}"
        WEZTERM_PCT="${WEZTERM_PCT%%%}"
        trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT
        "${WEZTERM_CLI[@]}" split-pane --bottom --percent "$WEZTERM_PCT" \
            --pane-id "$WEZTERM_PANE" --cwd "$CWD" -- sh -c "$(write_rc_cmd "$SENTINEL")" >/dev/null 2>&1

        while [ ! -f "$SENTINEL" ]; do
            sleep 0.3
        done
        rc=$(read_rc "$SENTINEL")
        rm -f "$SENTINEL"
        print_output_and_exit "${rc:-1}"
    fi
fi

# cmux: split pane via cmux CLI (must precede ghostty because cmux may expose Ghostty env vars)
if is_cmux_session; then
    if ! command -v cmux >/dev/null 2>&1; then
        echo "error: cmux session detected but cmux CLI not found" >&2
        exit 1
    fi
    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$(write_rc_cmd "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    # capture new surface ref from "OK surface:N ..." output
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
    rc=$(read_rc "$SENTINEL")
    # no explicit close: the exec'd launch script exits when revdiff does, so
    # cmux auto-closes the surface. closing by the short ref (surface:N) here
    # would risk hitting a recycled ref — another tab or the caller (see #217).
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    print_output_and_exit "${rc:-1}"
fi

# ghostty: split pane via AppleScript (macOS only, requires Ghostty 1.3.0+)
if [ "${TERM_PROGRAM:-}" = "ghostty" ] && command -v osascript >/dev/null 2>&1; then

    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
$(write_rc_cmd "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    if ! GHOSTTY_TERM_ID=$(osascript - "$LAUNCH_SCRIPT" "$CWD" <<'APPLESCRIPT'
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
    ); then
        rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
        exit 1
    fi

    while [ ! -f "$SENTINEL" ]; do
        sleep 0.3
    done
    rc=$(read_rc "$SENTINEL")
    # close the split pane (dismisses "press any key" prompt)
    osascript - "$GHOSTTY_TERM_ID" <<'APPLESCRIPT' 2>/dev/null
on run argv
    tell application "Ghostty" to close terminal id (item 1 of argv)
end run
APPLESCRIPT
    rm -f "$SENTINEL" "$LAUNCH_SCRIPT"
    print_output_and_exit "${rc:-1}"
fi

# iterm2: split pane via AppleScript (macOS only)
if [ -n "${ITERM_SESSION_ID:-}" ] && command -v osascript >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"

    # use launcher script to avoid single-quote injection in paths
    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
cd "\$1" && $REVDIFF_CMD; rc=\$?; printf "%s" "\$rc" > "\$2.tmp" && mv -f "\$2.tmp" "\$2"
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    # ITERM_SESSION_ID format is "w0t0p0:UUID"; AppleScript session id is the UUID part
    ITERM_UUID="${ITERM_SESSION_ID##*:}"

    # find target session by UUID, auto-detect split direction, capture new session id
    ITERM_NEW_SESSION=$(osascript - "$ITERM_UUID" "$LAUNCH_SCRIPT" "$CWD" "$SENTINEL" <<'APPLESCRIPT' 2>&1
on run argv
    set targetId to item 1 of argv
    set launchScript to item 2 of argv
    set cwd to item 3 of argv
    set sentinel to item 4 of argv
    set cmd to quoted form of launchScript & " " & quoted form of cwd & " " & quoted form of sentinel
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
    rc=$(read_rc "$SENTINEL")
    # close the split pane to avoid a dead session
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
    print_output_and_exit "${rc:-1}"
fi

# emacs vterm: open revdiff in a new vterm buffer via emacsclient
if [ "${INSIDE_EMACS:-}" = "vterm" ] && command -v emacsclient >/dev/null 2>&1; then
    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL" && mkfifo "$SENTINEL"

    # use launcher script to avoid shell interpolation issues in elisp strings;
    # embed all paths directly so vterm-shell needs no arguments
    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
cd $(sq "$CWD") && $(write_fifo_rc_cmd "$SENTINEL")
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    # find calling vterm shell PID (direct child of Emacs) to tag caller frame
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
    print_output_and_exit "${rc:-1}"
fi

echo "error: no overlay terminal available (requires tmux, zellij, herdr, kitty, wezterm, cmux, ghostty, iTerm2, or emacs vterm)" >&2
exit 1
