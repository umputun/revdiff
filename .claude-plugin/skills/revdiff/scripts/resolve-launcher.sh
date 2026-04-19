#!/usr/bin/env bash
# resolve launcher script through two-layer override chain (user → bundled)
# usage: resolve-launcher.sh <launcher-name> [data-dir]
# outputs absolute path of the first-found executable launcher
#
# No project (.claude/...) layer by design: this script can be invoked
# automatically by hooks (e.g. revdiff-planning's ExitPlanMode), and a
# repo-controlled executable layer would let an untrusted repo run
# arbitrary code on routine Claude actions. User layer + bundled only.
set -euo pipefail

name="${1:-}"
if [ -z "$name" ]; then
    echo "error: usage: resolve-launcher.sh <launcher-name> [data-dir]" >&2
    exit 1
fi
data_dir="${2:-${CLAUDE_PLUGIN_DATA:-}}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

abspath() { (cd "$(dirname "$1")" && printf '%s/%s\n' "$(pwd)" "$(basename "$1")"); }

# user layer
if [ -n "$data_dir" ] && [ -x "$data_dir/scripts/$name" ]; then
    abspath "$data_dir/scripts/$name"
    exit 0
fi
# bundled default
if [ -x "$SCRIPT_DIR/$name" ]; then
    abspath "$SCRIPT_DIR/$name"
    exit 0
fi
echo "error: launcher not found in override chain: $name" >&2
exit 1
