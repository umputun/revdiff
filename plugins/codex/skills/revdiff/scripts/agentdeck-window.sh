# shellcheck shell=bash
# revdiff — agent-deck backend. SOURCED by launch-revdiff.sh (never run standalone).
#
# Why this exists: agent-deck (https://github.com/asheshgoplani/agent-deck) renders each of its
# sessions through a tmux *control-mode* client, which does NOT render `tmux display-popup` at
# all — the popup is invisible to the user and `display-popup -E` blocks forever (the agent
# waits on a review the human can never see). agent-deck DOES surface real tmux windows, so
# under agent-deck revdiff runs in a background tmux window named after the content under
# review. It appears in the agent-deck session tree for the user to switch to when ready and
# never pops over whatever other session they are working in.
#
# Activation:
#   REVDIFF_TMUX_WINDOW=1   force window mode (any tmux)
#   REVDIFF_TMUX_WINDOW=0   force the popup path (skip this backend)
#   unset                   auto: window mode only when agent-deck is detected
#
# Reuses from the caller (launch-revdiff.sh): TMPBASE, OUTPUT_FILE, REVDIFF_CMD, CWD, DIR_NAME,
# TITLE_REF, and the helpers sq() / write_rc_cmd() / read_rc() / print_output_and_exit().
# The caller guarantees $TMUX is set and tmux is on PATH before sourcing this.

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
# not agent-deck (and not forced) → return to the launcher, which uses the normal popup path
[ "$_rd_winmode" = 1 ] || return 0

# window name from the content under review: the --only file, else the ref, else the directory
_rd_winname=""
for _rd_arg in "$@"; do
    case "$_rd_arg" in
        --only=*) _rd_f="${_rd_arg#--only=}"; _rd_winname="review: ${_rd_f##*/}" ;;
    esac
done
[ -z "$_rd_winname" ] && _rd_winname="review: ${DIR_NAME}${TITLE_REF:+ [$TITLE_REF]}"

SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
rm -f "$SENTINEL"
trap 'rm -f "$OUTPUT_FILE" "$SENTINEL" "$SENTINEL.tmp"' EXIT

# -d: create the window in the BACKGROUND so it does not steal the active window — it waits in
# the agent-deck tree until the user switches to it. -c: start directory.
tmux new-window -d -c "$CWD" -n "$_rd_winname" -- sh -c "$(write_rc_cmd "$SENTINEL")"

while [ ! -f "$SENTINEL" ]; do
    sleep 0.3
done
rc=$(read_rc "$SENTINEL")
rm -f "$SENTINEL"
print_output_and_exit "${rc:-1}"
