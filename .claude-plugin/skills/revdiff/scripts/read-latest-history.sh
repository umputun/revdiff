#!/usr/bin/env bash
# read-latest-history.sh - print the most recent revdiff review history file to stdout.
# used by the skill as a fallback when the live launcher output is unavailable
# (temp file cleaned up, or user ran revdiff outside the plugin flow).
#
# resolves the history dir from $REVDIFF_HISTORY_DIR, falling back to
# ~/.config/revdiff/history. resolves the repo subdir from `git rev-parse
# --show-toplevel` basename, falling back to the cwd basename.
#
# prints file contents if found, prints nothing if not. exits 0 in both cases.

set -uo pipefail  # not -e: final cat must not abort the fallback on permission/race errors

# resolve hist_dir defensively so unset HOME doesn't trip set -u
hist_dir="${REVDIFF_HISTORY_DIR:-}"
if [ -z "$hist_dir" ]; then
    hist_dir="${HOME:-}/.config/revdiff/history"
fi

repo="$(basename "$(git rev-parse --show-toplevel 2>/dev/null || pwd)")"
repo_dir="$hist_dir/$repo"

# find newest .md via -nt comparison instead of `ls -t` (shellcheck SC2012,
# and -nt is portable across macos/linux without find -printf tricks).
latest=""
for f in "$repo_dir"/*.md; do
    [ -e "$f" ] || continue
    if [ -z "$latest" ] || [ "$f" -nt "$latest" ]; then
        latest="$f"
    fi
done

if [ -n "$latest" ]; then
    cat "$latest"
fi
