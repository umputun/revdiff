# shellcheck shell=bash
# revdiff — tmux window backend. SOURCED by launch-revdiff.sh (never run standalone).
#
# This is the general tmux window backend: it runs revdiff in a server-owned tmux *window*
# instead of a `display-popup`. It has two triggers:
#   - agent-deck auto-detection — a background window that never steals focus, and
#   - explicit REVDIFF_TMUX_WINDOW=1 — first-class disconnect-resilient interactive review:
#     the window opens focused and the prior active window is restored on exit.
# A server-owned tmux window survives a client disconnect (SSH drop / VPN expiry) with zero loss —
# reattaching brings the still-running review back, which a `display-popup` cannot do.
#
# Why the agent-deck trigger exists: agent-deck (https://github.com/asheshgoplani/agent-deck)
# renders each of its sessions through a tmux *control-mode* client, which does NOT render
# `tmux display-popup` at all — the popup is invisible to the user and `display-popup -E` blocks
# forever (the agent waits on a review the human can never see). agent-deck DOES surface real tmux
# windows, so under agent-deck revdiff runs in a background tmux window named after the content
# under review. It appears in the agent-deck session tree for the user to switch to when ready and
# never pops over whatever other session they are working in.
#
# Activation:
#   REVDIFF_TMUX_WINDOW=1      force window mode, focused + restore prior window on exit (any tmux)
#   REVDIFF_TMUX_WINDOW=0      force the popup path (skip this backend)
#   REVDIFF_TMUX_TARGET=NAME   force window mode + focus at session NAME, addressing it with an
#                              explicit `-t` so nothing depends on tmux's client-less "current
#                              session" resolution. This is how a headless caller hands the review
#                              to a live client's session; also usable from inside tmux to target
#                              another session.
#   unset                      auto: background window mode only when agent-deck is detected
#
# Reuses from the caller (launch-revdiff.sh): TMPBASE, CWD, DIR_NAME, TITLE_REF, an optional
# _rd_target, and the helpers sq() / write_rc_cmd() / read_rc() / print_output_and_exit(). The caller
# guarantees tmux is on PATH and that bare `tmux` reaches the intended server (via $TMUX when inside
# tmux, or the default socket when a headless caller probed it). This file is SOURCED, so it must not
# install an EXIT trap (that would clobber the caller's cleanup trap); it cleans up explicitly instead
# and either returns (window mode off → caller falls through to the popup) or exits the process.

# explicit target session for the review window (and its prior-window restore). Comes from
# REVDIFF_TMUX_TARGET or from an _rd_target the caller already resolved (headless server probe).
# A non-empty target implies window mode + focus: the caller has decided which session hosts the
# review, so nothing falls back to tmux's client-less "current session" resolution.
_rd_target="${REVDIFF_TMUX_TARGET:-${_rd_target:-}}"

# _rd_focus marks the first-class interactive opt-in: set when the user forces window mode with
# REVDIFF_TMUX_WINDOW=1 or when an explicit target session is given. Capture it BEFORE the
# auto-detection below folds user-opt and agent-deck detection into the same _rd_winmode=1 — the
# focus + restore behavior is opt-in only (agent-deck auto-detection stays background).
_rd_focus=0
[ "${REVDIFF_TMUX_WINDOW:-}" = 1 ] && _rd_focus=1
[ -n "$_rd_target" ] && _rd_focus=1

# an explicit target forces window mode; otherwise honor REVDIFF_TMUX_WINDOW, else auto-detect.
_rd_winmode="${REVDIFF_TMUX_WINDOW:-}"
[ -n "$_rd_target" ] && _rd_winmode=1
if [ -z "$_rd_winmode" ]; then
    # agent-deck markers: its env var (also mirrored into the tmux session env), with the
    # agentdeck_* session-name prefix as a fallback signal.
    if [ -n "${AGENTDECK_INSTANCE_ID:-}" ] \
        || tmux show-environment AGENTDECK_INSTANCE_ID >/dev/null 2>&1 \
        || tmux display-message -p '#{session_name}' 2>/dev/null | grep -q '^agentdeck_'; then
        _rd_winmode=1
    else
        _rd_winmode=0
    fi
fi
# not window mode (forced off or auto-off) → return to the launcher, which uses the normal popup
# path. returning here (before any trap/sentinel work) leaves the caller's environment untouched.
[ "$_rd_winmode" = 1 ] || return 0

# window name from the content under review: the --only file, else the ref, else the directory
_rd_winname=""
for _rd_arg in "$@"; do
    case "$_rd_arg" in
        --only=*) _rd_f="${_rd_arg#--only=}"; _rd_winname="review: ${_rd_f##*/}" ;;
    esac
done
[ -z "$_rd_winname" ] && _rd_winname="review: ${DIR_NAME}${TITLE_REF:+ [$TITLE_REF]}"

_rd_sentinel=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
rm -f "$_rd_sentinel"

# first-class interactive mode: remember the active window so it can be restored after the review.
_rd_prevwin=""
if [ "$_rd_focus" = 1 ]; then
    if [ -n "$_rd_target" ]; then
        _rd_prevwin=$(tmux display-message -t "$_rd_target" -p '#{window_id}' 2>/dev/null || true)
    else
        _rd_prevwin=$(tmux display-message -p '#{window_id}' 2>/dev/null || true)
    fi
fi

# Open the review in a background window (-d: don't steal the active window; -c: start dir).
# -P -F prints the new window id so we can watch it; mirror the popup path's `sh -c "$REVDIFF_CMD"`
# invocation (every backend runs the command through sh, and REVDIFF_CMD is built sh-compatible).
# If tmux can't create the window, fail loudly instead of busy-waiting on a sentinel that will
# never appear.
# build the invocation; an explicit target lands the window in the chosen session. The arg array is
# always non-empty, so "${_rd_neww[@]}" is safe to expand under set -u (bash 3.2 compatible).
_rd_neww=(new-window -d -P -F '#{window_id}')
[ -n "$_rd_target" ] && _rd_neww+=(-t "$_rd_target")
_rd_neww+=(-c "$CWD" -n "$_rd_winname" -- sh -c "$(write_rc_cmd "$_rd_sentinel")")
if ! _rd_winid=$(tmux "${_rd_neww[@]}"); then
    rm -f "$_rd_sentinel" "$_rd_sentinel".tmp
    echo "revdiff: failed to open tmux review window" >&2
    exit 1
fi

# first-class interactive mode: bring the review window to the foreground (agent-deck stays -d).
if [ "$_rd_focus" = 1 ]; then
    tmux select-window -t "$_rd_winid" 2>/dev/null || true
fi

# Wait for the review to finish. The sentinel carries revdiff's exit code (written before the
# inner shell exits, so it exists by the time the window closes on a normal finish). Bound the
# wait on the window still existing rather than a timer: a real review may take a long time, but
# if the window disappears without a sentinel (killed / tmux died) we stop instead of hanging.
while [ ! -f "$_rd_sentinel" ]; do
    # -a lists windows across every session so the watch finds the review window whichever session
    # hosts it (window ids are server-global), including an explicit -t target session.
    tmux list-windows -a -F '#{window_id}' 2>/dev/null | grep -qxF "$_rd_winid" || break
    sleep 0.3
done

# restore the window that was active before the review took focus (first-class mode only).
if [ "$_rd_focus" = 1 ] && [ -n "$_rd_prevwin" ]; then
    tmux select-window -t "$_rd_prevwin" 2>/dev/null || true
fi

_rd_rc=1
[ -f "$_rd_sentinel" ] && _rd_rc=$(read_rc "$_rd_sentinel")
rm -f "$_rd_sentinel" "$_rd_sentinel".tmp
print_output_and_exit "${_rd_rc:-1}"
