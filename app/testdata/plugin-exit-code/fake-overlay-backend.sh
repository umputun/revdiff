#!/usr/bin/env bash
set -euo pipefail
cmd_name=$(basename "$0")

run_after_double_dash() {
    while [ "$#" -gt 0 ]; do
        if [ "$1" = "--" ]; then
            shift
            "$@"
            exit $?
        fi
        shift
    done
    echo "missing command after --" >&2
    exit 1
}

run_sh_c() {
    while [ "$#" -gt 0 ]; do
        if [ "$1" = "sh" ] && [ "${2:-}" = "-c" ]; then
            shift 2
            sh -c "$1"
            exit $?
        fi
        shift
    done
    echo "missing sh -c command" >&2
    exit 1
}

run_cmux_send() {
    local cmd="${*: -1}"
    cmd="${cmd%\\n}"
    cmd="${cmd#exec }"
    eval "$cmd"
}

case "$cmd_name" in
    tmux)
        if [ "${1:-}" = "-V" ]; then
            echo "tmux 3.4"
            exit 0
        fi
        run_after_double_dash "$@"
        ;;
    zellij)
        run_after_double_dash "$@"
        ;;
    kitty|wezterm)
        run_sh_c "$@"
        ;;
    cmux)
        case "${1:-}" in
            new-split)
                echo "OK surface:1"
                ;;
            send)
                run_cmux_send "$@"
                ;;
            close-surface)
                exit 0
                ;;
            *)
                echo "unexpected cmux command: ${1:-}" >&2
                exit 1
                ;;
        esac
        ;;
    osascript)
        if [ "$#" -ge 5 ] && [ -x "${3:-}" ]; then
            "$3" "$4" "$5"
            echo "session:1"
            exit 0
        fi
        if [ "$#" -ge 4 ] && [ -x "${3:-}" ]; then
            "$3" "$4"
            echo "session:1"
            exit 0
        fi
        if [ "$#" -ge 2 ] && [ -x "${2:-}" ]; then
            "$2"
            echo "session:1"
            exit 0
        fi
        exit 0
        ;;
    emacsclient)
        if [ "${1:-}" = "--eval" ] && [ "${2:-}" = "(emacs-pid)" ]; then
            echo '"1"'
            exit 0
        fi
        for arg in "$@"; do
            script=$(printf '%s' "$arg" | sed -n 's/.*vterm-shell "\([^"]*\)".*/\1/p')
            if [ -n "$script" ]; then
                "$script" &
            fi
        done
        exit 0
        ;;
    herdr)
        case "${1:-} ${2:-}" in
            "tab create")
                # ids satisfy both the jq path (.result.tab.tab_id /
                # .result.root_pane.pane_id) and the grep fallback ("tab_id":"..." /
                # "pane_id":"...")
                echo '{"result":{"tab":{"tab_id":"w1:1"},"root_pane":{"pane_id":"w1-1"}}}'
                ;;
            "tab close")
                exit 0
                ;;
            "pane run")
                # herdr pane run <pane_id> <command>; run the launch command and
                # report success, mimicking herdr's fire-and-forget (the real rc
                # still arrives via the sentinel the launch script writes)
                eval "${4:-}" || true
                exit 0
                ;;
            *)
                echo "unexpected herdr command: ${1:-} ${2:-}" >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "unexpected fake backend: $cmd_name" >&2
        exit 1
        ;;
esac
