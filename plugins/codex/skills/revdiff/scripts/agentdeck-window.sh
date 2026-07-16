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
#   REVDIFF_TMUX_WINDOW=1   force window mode, focused + restore prior window on exit (any tmux)
#   REVDIFF_TMUX_WINDOW=0   force the popup path (skip this backend)
#   unset                   auto: background window mode only when agent-deck is detected
#
# Reuses from the caller (launch-revdiff.sh): TMPBASE, CWD, DIR_NAME, TITLE_REF and the helpers
# sq() / write_rc_cmd() / read_rc() / print_output_and_exit(). The caller guarantees $TMUX is
# set and tmux is on PATH before sourcing this. This file is SOURCED, so it must not install an
# EXIT trap (that would clobber the caller's cleanup trap); it cleans up explicitly instead and
# either returns (window mode off → caller falls through to the popup) or exits the process.

# _rd_focus marks the first-class interactive opt-in: set only when the user explicitly forces
# window mode with REVDIFF_TMUX_WINDOW=1. Capture it BEFORE the auto-detection below folds user-opt
# and agent-deck detection into the same _rd_winmode=1 — the focus + restore behavior is opt-in only.
_rd_focus=0
[ "${REVDIFF_TMUX_WINDOW:-}" = 1 ] && _rd_focus=1

_rd_winmode="${REVDIFF_TMUX_WINDOW:-}"
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
    _rd_prevwin=$(tmux display-message -p '#{window_id}' 2>/dev/null || true)
fi

# Open the review in a background window (-d: don't steal the active window; -c: start dir).
# -P -F prints the new window id so we can watch it; mirror the popup path's `sh -c "$REVDIFF_CMD"`
# invocation (every backend runs the command through sh, and REVDIFF_CMD is built sh-compatible).
# If tmux can't create the window, fail loudly instead of busy-waiting on a sentinel that will
# never appear.
if ! _rd_winid=$(tmux new-window -d -P -F '#{window_id}' -c "$CWD" -n "$_rd_winname" \
        -- sh -c "$(write_rc_cmd "$_rd_sentinel")"); then
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
    tmux list-windows -F '#{window_id}' 2>/dev/null | grep -qxF "$_rd_winid" || break
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
