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
        if [ -n "${FAKE_OSASCRIPT_ARGS_FILE:-}" ]; then
            printf '%s\n' "$*" >> "$FAKE_OSASCRIPT_ARGS_FILE"
        fi
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
        # record every invocation so tests can assert the exact call sequence
        if [ -n "${FAKE_HERDR_ARGS_FILE:-}" ]; then
            echo "$*" >> "$FAKE_HERDR_ARGS_FILE"
        fi
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
            "pane split")
                # the --direction feature probe; FAKE_HERDR_PANE=1 fakes a CLI new
                # enough to carry pane mode
                if [ "${3:-}" = "--help" ]; then
                    if [ "${FAKE_HERDR_PANE:-}" = "1" ]; then
                        echo "      --direction <DIRECTION>  [possible values: right, down]"
                        exit 0
                    fi
                    exit 1
                fi
                [ "${FAKE_HERDR_SPLIT_FAIL:-}" = "1" ] && { echo "herdr: split refused" >&2; exit 1; }
                # malformed payload: exercises the never-guess bail-out
                if [ -n "${FAKE_HERDR_SPLIT_JSON:-}" ]; then
                    echo "$FAKE_HERDR_SPLIT_JSON"
                    exit 0
                fi
                # HANG wedges the launcher inside the split itself. The call is already
                # recorded above, so a test can signal deterministically while the pane is
                # being created and the launcher cannot yet name it.
                [ -n "${FAKE_HERDR_SPLIT_HANG:-}" ] && sleep "$FAKE_HERDR_SPLIT_HANG"
                echo '{"result":{"pane":{"pane_id":"w1-2"}}}'
                ;;
            "pane get")
                if [ "${3:-}" = "--help" ]; then
                    [ "${FAKE_HERDR_PANE:-}" = "1" ] && exit 0
                    exit 1
                fi
                # GONE is authoritative death; FAIL is a generic/transient error. The
                # launcher must treat these differently, so the fake must too.
                if [ "${FAKE_HERDR_GET_GONE:-}" = "1" ]; then
                    echo '{"error":{"code":"pane_not_found","message":"pane w1-2 not found"},"id":"cli:pane:get"}'
                    exit 1
                fi
                [ "${FAKE_HERDR_GET_FAIL:-}" = "1" ] && { echo "herdr: connection reset" >&2; exit 1; }
                echo '{"result":{"pane":{"pane_id":"w1-2"}}}'
                ;;
            "pane close")
                if [ "${3:-}" = "--help" ]; then
                    [ "${FAKE_HERDR_PANE:-}" = "1" ] && exit 0
                    exit 1
                fi
                exit 0
                ;;
            "pane zoom")
                # HANG wedges the launcher after the pane exists but before anything is
                # dispatched into it -- the window where a signal must reach the trap's
                # close. Seconds, so the SIGTERM (milliseconds away) always lands inside it.
                [ -n "${FAKE_HERDR_ZOOM_HANG:-}" ] && sleep "$FAKE_HERDR_ZOOM_HANG"
                exit 0
                ;;
            "pane run")
                # herdr pane run <pane_id> <command>; run the launch command and
                # report success, mimicking herdr's fire-and-forget (the real rc
                # still arrives via the sentinel the launch script writes)
                # herdr refuses outright: nothing was dispatched, and nothing announced
                [ "${FAKE_HERDR_RUN_FAIL:-}" = "1" ] && exit 1
                # hang and never dispatch, so a test can signal the launcher while the pane
                # exists but holds nothing but a shell. It must NOT fall through to the eval:
                # a trapped TERM is deferred until this foreground call returns, so dispatching
                # afterwards would let the marker appear and turn the case into a
                # finished-review test.
                if [ -n "${FAKE_HERDR_RUN_HANG:-}" ]; then
                    sleep "$FAKE_HERDR_RUN_HANG"
                    exit 0
                fi
                # the review really starts, but the response is lost: a nonzero return that
                # is NOT evidence the review failed to start
                if [ "${FAKE_HERDR_RUN_FAIL_AFTER_START:-}" = "1" ]; then
                    # background it so the review is still RUNNING when the response is
                    # lost: delivered, announced, response lost. The marker beats the
                    # launcher's check because that path waits 0.3s before deciding, which
                    # is far longer than an `sh` spawn -- no head start is needed here.
                    ( eval "${4:-}" || true ) >/dev/null 2>&1 &
                    exit 1
                fi
                if [ "${FAKE_HERDR_NO_SENTINEL:-}" = "1" ]; then
                    # model a pane that dies without ever writing the sentinel
                    exit 0
                fi
                if [ "${FAKE_HERDR_ASYNC:-}" = "1" ]; then
                    # dispatch immediately, like herdr does, and let the review itself take
                    # time (FAKE_REVDIFF_DELAY). That is what makes the wait loop iterate and
                    # leaves a genuine "started but not finished" window for signal tests.
                    ( eval "${4:-}" || true ) >/dev/null 2>&1 &
                    exit 0
                fi
                eval "${4:-}" || true
                exit 0
                ;;
            *)
                echo "unexpected herdr command: ${1:-} ${2:-}" >&2
                exit 1
                ;;
        esac
        ;;
    agtermctl)
        case "${1:-} ${2:-}" in
            "session overlay")
                # the --pane feature probe; FAKE_AGTERM_PANE=1 fakes a CLI new
                # enough to carry the flag
                if [ "${3:-}" = "open" ] && [ "${4:-}" = "--help" ]; then
                    echo "  --size-percent <size-percent>"
                    if [ "${FAKE_AGTERM_PANE:-}" = "1" ]; then
                        echo "  --pane <pane>  Scope the overlay to ONE split pane"
                    fi
                    exit 0
                fi
                # session overlay open <cmd> --target ... --cwd ... --block; run
                # the command string in a shell and propagate its exit code,
                # mirroring agtermctl running the overlay command and returning rc
                if [ "${3:-}" = "open" ]; then
                    if [ -n "${FAKE_AGTERM_ARGS_FILE:-}" ]; then
                        printf '%s\n' "$*" >> "$FAKE_AGTERM_ARGS_FILE"
                    fi
                    # FAKE_AGTERM_PANE_REFUSE=1 mimics agterm rejecting a pane
                    # overlay without running the command, so the launcher's
                    # session-wide retry is exercised
                    if [ "${FAKE_AGTERM_PANE_REFUSE:-}" = "1" ] && printf '%s' "$*" | grep -q -- '--pane'; then
                        echo "pane not visible" >&2
                        exit 1
                    fi
                    sh -c "${4:-}"
                    exit $?
                fi
                exit 0
                ;;
            "tree --json")
                # split state for the launcher's pane-overlay gate
                printf '{"result":{"tree":{"workspaces":[{"sessions":[{"id":"%s","split":%s}]}]}}}\n' \
                    "${AGTERM_SESSION_ID:-}" "${FAKE_AGTERM_SPLIT:-false}"
                exit 0
                ;;
            "session status")
                # blocked/active indicator toggles; no-op in the fake
                exit 0
                ;;
            *)
                echo "unexpected agtermctl command: $*" >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "unexpected fake backend: $cmd_name" >&2
        exit 1
        ;;
esac
