#!/usr/bin/env bash
# launch revdiff in a terminal overlay (agterm/tmux/zellij/herdr/kitty/wezterm/cmux/ghostty/iterm2) and capture annotations.
# source: .claude-plugin/skills/revdiff/scripts/launch-revdiff.sh (keep in sync)
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
ERR_FILE=$(mktemp "$TMPBASE/revdiff-err-XXXXXX")
trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE"' EXIT

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
# the overlay closes the moment a fast-failing revdiff exits, taking the error
# text with it. every backend runs this command string, so one redirect here
# captures stderr for all of them; print_output_and_exit replays it on failure
REVDIFF_CMD="$REVDIFF_CMD 2>$(sq "$ERR_FILE")"

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
    # 0 is a clean quit and 10 means annotations were captured; both are
    # successes, and revdiff writes ordinary warnings to stderr, so relaying
    # them would put noise on every successful review
    if [ "$rc" -ne 0 ] && [ "$rc" -ne 10 ] && [ -s "$ERR_FILE" ]; then
        cat "$ERR_FILE" >&2
    fi
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

# whether this agtermctl accepts `--pane` on overlay open. It reached the CLI after agterm v0.9.0, and
# on an older one it is a usage error that --block would surface as a nonzero exit inside revdiff's own
# 0/10 vocabulary, so ask the CLI rather than assume. The answer is the PATH agtermctl's, which is not
# always the CLI belonging to the running app; the refusal fallback below is what covers that skew.
agterm_supports_pane_overlay() {
    agtermctl session overlay open --help 2>/dev/null | grep -q -- '--pane'
}

# 1 when the calling session carries a split, 0 otherwise. Window-scoped because `tree` defaults to the
# FRONTMOST window, which is not the agent's whenever the user is looking elsewhere — unscoped it would
# find no session and report every split as absent. jq is what parses it; without jq there is no honest
# read, so it reports "not split" and the session-wide overlay stands.
agterm_session_split() {
    local tree args=(tree --json)
    command -v jq >/dev/null 2>&1 || { printf '0'; return 0; }
    [ -n "${AGTERM_WINDOW_ID:-}" ] && args+=(--window "$AGTERM_WINDOW_ID")
    [ -n "${AGTERM_SOCKET:-}" ] && args+=(--socket "$AGTERM_SOCKET")
    tree=$(agtermctl "${args[@]}" 2>/dev/null) || tree=""
    [ -n "$tree" ] || { printf '0'; return 0; }
    printf '%s' "$tree" | jq -r --arg s "$AGTERM_SESSION_ID" '
        [.result.tree.workspaces[].sessions[] | select(.id == $s)][0].split // false
        | if . then 1 else 0 end
    ' 2>/dev/null || printf '0'
}

# agterm: `agtermctl session overlay open <cmd> --block` opens revdiff in a FULL-pane overlay (no
# --size-percent) over the agent's own session and blocks until it exits, returning revdiff's exit
# code directly — so, unlike the sentinel-polling backends below, no sentinel is needed. Checked
# first so an agterm session always uses its native overlay even when a multiplexer is also present.
# Needs $AGTERM_SESSION_ID (set in every agterm session) and agtermctl on PATH; passes the bound
# $AGTERM_SOCKET so it reaches the agterm instance hosting this session. Passes --cwd "$CWD" so the
# overlay runs in the launcher's working directory (e.g. a PR worktree) instead of agtermctl's
# default of the agent session's current directory. Sets the session's agent-status indicator to
# blocked (blinking) while the overlay is up and back to active after, since claude code does not
# flag the session blocked while revdiff owns the overlay.
if [ -n "${AGTERM_SESSION_ID:-}" ] && command -v agtermctl >/dev/null 2>&1; then
    # shared target (+ socket) for every agtermctl call in this branch
    AGTERM_TARGET=(--target "$AGTERM_SESSION_ID")
    [ -n "${AGTERM_SOCKET:-}" ] && AGTERM_TARGET+=(--socket "$AGTERM_SOCKET")
    # record which pane owns the block so agterm nav lands on the reviewing pane; agterm defaults to
    # the left pane otherwise, which misroutes navigation from a split or scratch session. only a
    # recognized value is passed, so anything else falls back to agterm's own default.
    AGTERM_STATUS=(session status blocked --blink)
    case "${AGTERM_PANE:-}" in
        left|right|scratch) AGTERM_STATUS+=(--pane "$AGTERM_PANE") ;;
    esac
    # claude code does not flag the session blocked while revdiff owns the overlay, so set it here
    # (blocked + blink draws attention from other windows). the EXIT trap restores active AND removes
    # the temp output file on every exit path, and INT/TERM exit through it, so an interrupt never
    # leaves the indicator stuck or the file behind (this trap supersedes the earlier output-file one).
    agtermctl "${AGTERM_STATUS[@]}" "${AGTERM_TARGET[@]}" >/dev/null 2>&1 || true
    trap 'agtermctl session status active "${AGTERM_TARGET[@]}" >/dev/null 2>&1 || true; rm -f "$OUTPUT_FILE" "$ERR_FILE"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    # optional pane-scoped overlay (REVDIFF_AGTERM_PANE=1). The overlay above covers the WHOLE session,
    # so in a visible split the review hides the sibling pane's work; `--pane` scopes it to the agent's
    # own pane and leaves the sibling live and visible. Opt-in because that also gives the review half
    # the width, which is the wrong trade for a wide diff — unset leaves the call as it was. Only the
    # split panes qualify: `scratch` is a full-coverage surface with no sibling to spare.
    # Known limitation: $AGTERM_PANE is baked into the shell's environ at spawn, so a pane promoted into
    # the main slot keeps `right`. Promote-then-re-split therefore scopes the overlay to the new sibling
    # rather than this pane, and agterm reports no error because that pane does exist — the fallback
    # below cannot see it. `session status` takes a stable `--pane-id` token for exactly this; `overlay
    # open` does not yet, and the control tree exposes nothing to resolve one against, so there is no
    # launcher-side fix. Failing closed to the session-wide overlay is deliberately NOT the answer: that
    # drops the feature for everyone to avoid a rare misplaced surface the user can see and close.
    AGTERM_OPEN=(session overlay open "$REVDIFF_CMD" "${AGTERM_TARGET[@]}" --cwd "$CWD")
    AGTERM_PANE_SCOPED=0
    if [ "${REVDIFF_AGTERM_PANE:-}" = 1 ]; then
        case "${AGTERM_PANE:-}" in
            left|right)
                if agterm_supports_pane_overlay && [ "$(agterm_session_split)" = 1 ]; then
                    AGTERM_OPEN+=(--pane "$AGTERM_PANE")
                    AGTERM_PANE_SCOPED=1
                fi
                ;;
        esac
    fi

    rc=0
    # agtermctl's own stderr, captured apart from revdiff's (which the command string sends to
    # $ERR_FILE) so the pane refusal below is decidable; replayed either way so nothing is swallowed.
    # Its stdout is dropped because print_output_and_exit owns this launcher's stdout, which carries
    # the annotations alone.
    AGTERM_ERR=$(agtermctl "${AGTERM_OPEN[@]}" --block 2>&1 >/dev/null) || rc=$?
    [ -n "$AGTERM_ERR" ] && printf '%s\n' "$AGTERM_ERR" >&2
    # A pane open agterm refused means revdiff never ran, so the session-wide overlay every version
    # supports is still worth trying rather than failing the review over geometry. Both refusals are
    # post-checks and neither is decidable from the split read above: the split can go away between that
    # read and this call, and a pane's overlay slot is separate from the session-wide one, so a stale
    # pane overlay is invisible to it. Gated on agterm's own message so a revdiff failure is never
    # retried — that would run the whole review a second time.
    if [ "$rc" -ne 0 ] && [ "$AGTERM_PANE_SCOPED" -eq 1 ] &&
        printf '%s' "$AGTERM_ERR" | grep -qE 'pane overlay already open|pane not visible'; then
        rc=0
        agtermctl session overlay open "$REVDIFF_CMD" "${AGTERM_TARGET[@]}" --cwd "$CWD" --block \
            >/dev/null || rc=$?
    fi
    print_output_and_exit "$rc"
fi

# agent-deck: its control-mode tmux UI cannot render display-popup, so when detected this sourced
# backend runs revdiff in a tmux window instead and exits. It returns here (no-op) for every
# non-agent-deck tmux, leaving the popup path below unchanged.
if [ -n "${TMUX:-}" ] && command -v tmux >/dev/null 2>&1; then
    _RD_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
    # shellcheck source=/dev/null  # sibling backend resolved at runtime; not followed at lint time
    # shellcheck disable=SC1091
    [ -f "$_RD_SCRIPT_DIR/agentdeck-window.sh" ] && . "$_RD_SCRIPT_DIR/agentdeck-window.sh"
fi

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
    trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
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
# otherwise win and open an overlay window herdr cannot composite into its panes).
# REVDIFF_HERDR_PANE=1 instead runs the review in a zoomed split of the agent's
# own pane, which keeps that pane one keypress away; it is self-contained and
# never falls through to the tab path below except when the split is declined.
if [ "${HERDR_ENV:-}" = "1" ] && command -v herdr >/dev/null 2>&1; then
    # $HERDR_PANE_ID is injected by herdr into every managed pane, and the tab path
    # below reuses that name for the pane it creates — copy the caller's id out first
    HERDR_CALLER_PANE="${HERDR_PANE_ID:-}"
    # non-empty once we own a review pane that has to be closed on every exit path
    HERDR_TARGET=""
    # the pane's own shell touches this the instant it starts the launch script, so
    # ownership is decided by evidence FROM the pane rather than by `pane run` returning:
    # herdr may have started the review before the CLI call returns, and may not have
    # started it after. Assigned once $SENTINEL exists, below. `touch ... || true` and not
    # `: >`, because a redirection failure on a special builtin kills a POSIX shell outright,
    # which would stop revdiff running at all; a $TMPBASE the pane cannot write already
    # breaks the sentinel the same way, for the tab path too.
    HERDR_STARTED=""
    # set BEFORE `pane run` and never cleared again, so the in-flight window is owned
    # rather than ambiguous: a pending signal is serviced the instant the call returns,
    # before any later assignment could run. The marker is still needed for the failure
    # path, where it is the only evidence that a refused-looking dispatch actually started.
    HERDR_DISPATCHED=0

    # all three are used at runtime; a CLI lacking `get` would read every liveness poll
    # as a failure and abandon a live review. zoom stays unprobed: it is purely cosmetic.
    herdr_supports_pane_mode() {
        herdr pane split --help 2>/dev/null | grep -q -- '--direction' \
            && herdr pane get   --help >/dev/null 2>&1 \
            && herdr pane close --help >/dev/null 2>&1
    }

    # idempotent, and clears HERDR_TARGET before shelling out so a signal during the
    # close cannot re-enter through the EXIT trap and close the same id twice
    herdr_close_pane() {
        local target="$HERDR_TARGET"
        HERDR_TARGET=""
        [ -n "$target" ] || return 0
        if ! herdr pane close "$target" >/dev/null 2>&1; then
            herdr pane zoom "$target" --off >/dev/null 2>&1 || true
            printf 'revdiff: could not close herdr review pane %s\n' "$target" >&2
        fi
        return 0
    }

    # EXIT-trap cleanup. A review that has STARTED but not FINISHED belongs to the user:
    # SKILL.md promises the driving agent that a launcher killed on timeout leaves revdiff
    # open with nothing lost, so we must not close it and must not delete the script it is
    # running. Anything else is ours -- a pane that never started one (close it, it holds
    # only a shell) or one that already finished (close it, it is done).
    # shellcheck disable=SC2317,SC2329  # invoked from the EXIT trap below, which shellcheck cannot
    # see. both codes: shellcheck <0.11 reports this as SC2317, 0.11+ as SC2329
    herdr_cleanup_unlaunched() {
        # pane mode only: the tab path never sets HERDR_TARGET, so its launch-script
        # lifecycle stays exactly what master does here and what every other
        # script-using backend does -- the trap removes it, unconditionally
        if [ -n "$HERDR_TARGET" ] && [ ! -f "$SENTINEL" ] &&
            { [ -f "$HERDR_STARTED" ] || [ "$HERDR_DISPATCHED" = 1 ]; }; then
            return 0
        fi
        herdr_close_pane
        rm -f "$LAUNCH_SCRIPT" "$HERDR_STARTED"
        return 0
    }

    SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
    rm -f "$SENTINEL"
    HERDR_STARTED="$SENTINEL.started"

    LAUNCH_SCRIPT=$(mktemp "$TMPBASE/revdiff-launch-XXXXXX")
    trap 'herdr_cleanup_unlaunched || true; rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp" "$HERDR_STARTED"' EXIT
    # same shape as the agterm branch above: INT/TERM exit through the EXIT trap so a
    # signal never skips cleanup or leaves an unnormalised status
    trap 'exit 130' INT
    trap 'exit 143' TERM
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
touch $(sq "$HERDR_STARTED") 2>/dev/null || true
$(write_rc_cmd "$SENTINEL")
rm -f "\$0"
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    if [ "${REVDIFF_HERDR_PANE:-}" = 1 ] && [ -n "$HERDR_CALLER_PANE" ] && herdr_supports_pane_mode; then
        # A trapped signal is deferred until the in-flight foreground command returns, then
        # runs BEFORE the next statement -- so `exit 143` during the split would fire after
        # the pane exists but before HERDR_TARGET names it, and the EXIT trap would have
        # nothing to close. Record the signal instead of acting on it, and honor it once
        # ownership is held. This is a trap with a COMMAND, which children reset to default;
        # `trap '' INT TERM` would be SIG_IGN, inherited across exec, and would make a hung
        # herdr unkillable (see CLAUDE.md). It also adds no deferral that `exit 143` did not
        # already have: both wait out the same wedged split.
        HERDR_SIGNALLED=0
        trap 'HERDR_SIGNALLED=130' INT
        trap 'HERDR_SIGNALLED=143' TERM
        if HERDR_NEW=$(herdr pane split --pane "$HERDR_CALLER_PANE" --direction right \
                --cwd "$CWD" --focus 2>&1); then
            if command -v jq >/dev/null 2>&1; then
                HERDR_TARGET=$(printf '%s' "$HERDR_NEW" | jq -r '.result.pane.pane_id // empty' 2>/dev/null || true)
            fi
            if [ -z "$HERDR_TARGET" ]; then
                HERDR_TARGET=$(printf '%s' "$HERDR_NEW" | grep -o '"pane_id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
            fi
            # the grep fallback is positional, so a response listing the source pane first
            # would hand us the caller's own id — closing that would kill the agent's pane
            if [ "$HERDR_TARGET" = "$HERDR_CALLER_PANE" ]; then
                HERDR_TARGET=""
            fi
            if [ -z "$HERDR_TARGET" ]; then
                # never guess: a pane found by diffing `pane list` may belong to another
                # herdr client, and closing that would destroy the user's own work
                printf 'revdiff: herdr pane split returned no pane id; a stray review pane may remain: %s\n' \
                    "$HERDR_NEW" >&2
                exit 1
            fi
            # ownership is held: back to exiting traps, and pay any signal deferred above
            # through the EXIT trap, which can now close the pane it created
            trap 'exit 130' INT
            trap 'exit 143' TERM
            [ "$HERDR_SIGNALLED" != 0 ] && exit "$HERDR_SIGNALLED"
            herdr pane zoom "$HERDR_TARGET" --on >/dev/null 2>&1 || true
            # claim ownership BEFORE dispatching. A pending signal is serviced the moment
            # this call returns and before any assignment after it, so ownership taken
            # afterwards would miss a dispatch herdr had already accepted. Never cleared
            # again -- the refusal path below says why restoring a reset here is unsafe.
            HERDR_DISPATCHED=1
            if ! herdr pane run "$HERDR_TARGET" "sh $(sq "$LAUNCH_SCRIPT")" >/dev/null 2>&1; then
                # this branch does trust the return code, unlike the trap above: a lost
                # response after a successful dispatch would close a just-started review,
                # but stranding a pane on every genuine dispatch failure is the worse default
                echo "error: herdr pane run failed for pane $HERDR_TARGET" >&2
                # herdr refused the command, so nothing was dispatched -- unless the pane
                # announced itself anyway, which would mean the response was lost after
                # delivery. Give that a moment to show up, then close only a pane that
                # never announced. Paid on the failure path only.
                # HERDR_DISPATCHED deliberately stays 1 across the grace: the trap reads it,
                # so clearing it here would let a signal landing inside the sleep close the
                # pane before the marker had its chance to appear -- the exact unknown-state
                # destruction claim-before-dispatch exists to prevent. Clearing it after the
                # sleep would be dead anyway, since herdr_close_pane discharges ownership by
                # emptying HERDR_TARGET, which is what the trap gates on.
                #
                # The grace must therefore RUN TO COMPLETION even while being signalled: any
                # exit taken inside it hands the trap a state it must read as "may be live"
                # and preserve, stranding a pane the finished evidence check would close.
                # Two things are needed for that, and neither alone is enough:
                #   - record the signal instead of exiting on it, as the split window does;
                #   - survive the sleep itself being killed. A process-group signal
                #     (interactive Ctrl-C, `kill -- -pgid`) hits the foreground `sleep` too,
                #     which then returns nonzero and lets `set -e` abort the script before
                #     the evidence check -- resurrecting the strand despite the trap.
                # The grace is therefore measured in ELAPSED TIME, not in completed sleeps:
                # $SECONDS is set by the shell from the wall clock, so a killed `sleep` costs
                # an early wakeup and nothing else, and the interval is served no matter how
                # many signals arrive. Counting sleeps instead can end with zero time
                # elapsed, leaving an absent marker that proves nothing. `|| true` keeps an
                # interrupted sleep from tripping errexit; the loop body cannot fail
                # otherwise, so it cannot re-arm it either. The marker check is in the
                # condition, so the common path leaves as soon as the pane announces itself
                # rather than always paying the full interval.
                #
                # `-lt 2`, not `-lt 1`: $SECONDS counts whole-second boundaries crossed since
                # the assignment, so it can reach 1 a millisecond later if the assignment
                # lands just before a boundary. Waiting for the second boundary guarantees a
                # full second was served; the cost is up to about two seconds, on a failure path only.
                HERDR_SIGNALLED=0
                trap 'HERDR_SIGNALLED=130' INT
                trap 'HERDR_SIGNALLED=143' TERM
                SECONDS=0
                while [ ! -f "$HERDR_STARTED" ] && [ "$SECONDS" -lt 2 ]; do
                    sleep 0.3 || true
                done
                # the interval was served, so an absent marker is real evidence
                [ -f "$HERDR_STARTED" ] || herdr_close_pane
                trap 'exit 130' INT
                trap 'exit 143' TERM
                # the refusal is why we are exiting, so it owns the status; a signal that
                # arrived during the grace has already been honored by the close above
                exit 1
            fi
            HERDR_MISSES=0
            HERDR_POLL=0.3
            while [ ! -f "$SENTINEL" ]; do
                if HERDR_GET=$(herdr pane get "$HERDR_TARGET" 2>&1); then
                    HERDR_MISSES=0
                    HERDR_POLL=0.3
                elif printf '%s' "$HERDR_GET" | grep -q '"code":"pane_not_found"'; then
                    # authoritative: the pane is gone, so ownership is already discharged --
                    # nothing to close, and nothing to complain about
                    HERDR_TARGET=""
                    break
                else
                    # a generic error is NOT proof of death (socket hiccup, server restart),
                    # and there is no safe deadline when the API cannot report liveness --
                    # any bound turns unknown liveness into a closed live review. Warn once
                    # per outage and back off, then keep waiting, exactly as the tab path
                    # below does for a human review. Both reset on a good poll.
                    HERDR_MISSES=$((HERDR_MISSES + 1))
                    if [ "$HERDR_MISSES" = 10 ]; then
                        printf 'revdiff: herdr pane get keeps failing, still waiting for the review: %s\n' \
                            "$HERDR_GET" >&2
                        # stop hammering a control plane that is down; the sentinel is still
                        # checked every iteration, so finishing costs at most one interval
                        HERDR_POLL=2
                    fi
                fi
                sleep "$HERDR_POLL"
            done
            rc=$(read_rc "$SENTINEL")
            # the review is over either way: the script ran (and removed itself), or the
            # pane died before it could and nothing will ever open it now. Only a launcher
            # killed mid-review leaves it behind, which is deliberate -- the pane may still
            # be about to open it.
            rm -f "$LAUNCH_SCRIPT" "$HERDR_STARTED"
            herdr_close_pane
            print_output_and_exit "${rc:-1}"
        fi
        # nothing was created — fall through to the tab path. Restore the exiting traps
        # first: the recording trap must not survive into a path with no pane to protect,
        # or a signal there would be swallowed instead of ending the launcher.
        trap 'exit 130' INT
        trap 'exit 143' TERM
        [ "$HERDR_SIGNALLED" != 0 ] && exit "$HERDR_SIGNALLED"
        printf 'revdiff: herdr pane split declined, using tab overlay: %s\n' "$HERDR_NEW" >&2
    fi

    # pin the tab to the caller's workspace: without --workspace, herdr tab create
    # targets the server's focused workspace (what the user is currently viewing),
    # not the caller's workspace
    HERDR_TAB_ARGS=(tab create --cwd "$CWD" --label "$OVERLAY_TITLE")
    [ -n "${HERDR_WORKSPACE_ID:-}" ] && HERDR_TAB_ARGS+=(--workspace "$HERDR_WORKSPACE_ID")
    HERDR_TAB_ARGS+=(--focus)
    HERDR_NEW=$(herdr "${HERDR_TAB_ARGS[@]}" 2>&1) || {
        echo "error: herdr tab create failed: $HERDR_NEW" >&2
        exit 1
    }
    # parse the ids: jq when available, falling back to grep when jq is absent OR
    # yields empty (e.g. herdr mixed a stderr line into the JSON via 2>&1). || true
    # keeps a parse miss from tripping set -e so the explicit id check below stays
    # reachable to emit a real error and close any created tab
    HERDR_TAB_ID=""
    HERDR_PANE_ID=""
    if command -v jq >/dev/null 2>&1; then
        HERDR_TAB_ID=$(printf '%s' "$HERDR_NEW" | jq -r '.result.tab.tab_id // empty' 2>/dev/null || true)
        HERDR_PANE_ID=$(printf '%s' "$HERDR_NEW" | jq -r '.result.root_pane.pane_id // empty' 2>/dev/null || true)
    fi
    if [ -z "$HERDR_TAB_ID" ]; then
        HERDR_TAB_ID=$(printf '%s' "$HERDR_NEW" | grep -o '"tab_id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
    fi
    if [ -z "$HERDR_PANE_ID" ]; then
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
    trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT

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
        trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT
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
    trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
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
    trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
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
    trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$SENTINEL.tmp" "$LAUNCH_SCRIPT"' EXIT
    cat > "$LAUNCH_SCRIPT" <<LAUNCHER
#!/bin/sh
cd "\$1" && $REVDIFF_CMD; rc=\$?; printf "%s" "\$rc" > "\$2.tmp" && mv -f "\$2.tmp" "\$2"
LAUNCHER
    chmod +x "$LAUNCH_SCRIPT"

    # ITERM_SESSION_ID format is "w0t0p0:UUID"; AppleScript session id is the UUID part
    ITERM_UUID="${ITERM_SESSION_ID##*:}"

    # find target session by UUID, auto-detect split direction, capture new session id
    ITERM_NEW_SESSION=$(osascript - "$ITERM_UUID" "$LAUNCH_SCRIPT" "$CWD" "$SENTINEL" "$OVERLAY_TITLE" <<'APPLESCRIPT' 2>&1
on run argv
    set targetId to item 1 of argv
    set launchScript to item 2 of argv
    set cwd to item 3 of argv
    set sentinel to item 4 of argv
    set overlayTitle to item 5 of argv
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
                        -- the tab label comes from the name of its active
                        -- session, and the split gets none of its own: it
                        -- copies the parent profile but not the session
                        -- variables that the profile name may interpolate.
                        -- keep this comment free of apostrophes: bash 3.2
                        -- scans the enclosing command substitution for quotes
                        -- before it processes the heredoc, so an odd count
                        -- here opens a quote that never closes and the whole
                        -- script fails to parse
                        set name of newSession to overlayTitle
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
    trap 'rm -f "$OUTPUT_FILE" "$ERR_FILE" "$SENTINEL" "$LAUNCH_SCRIPT"' EXIT
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

echo "error: no overlay terminal available (requires agterm, tmux, zellij, herdr, kitty, wezterm, cmux, ghostty, iTerm2, or emacs vterm)" >&2
exit 1
