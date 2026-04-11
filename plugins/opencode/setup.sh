#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

OPENCODE_CONFIG="$HOME/.config/opencode"

copy_file() {
  local src="$1"
  local dst="$2"
  local make_exec="${3:-}"

  if [[ ! -f "$src" ]]; then
    echo "ERROR: source file not found: $src" >&2
    return 1
  fi

  cp "$src" "$dst"
  [[ "$make_exec" == "exec" ]] && chmod +x "$dst"
  echo "Copied $(realpath --relative-to="$REPO_ROOT" "$src" 2>/dev/null || echo "$src") -> $dst"
}

mkdir -p "$OPENCODE_CONFIG/commands"
mkdir -p "$OPENCODE_CONFIG/tools"
mkdir -p "$OPENCODE_CONFIG/plugins"

errors=0

copy_file "$REPO_ROOT/.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh"  "$OPENCODE_CONFIG/tools/launch-revdiff.sh" exec || ((errors++))
copy_file "$REPO_ROOT/plugins/revdiff-planning/scripts/launch-plan-review.sh"   "$OPENCODE_CONFIG/plugins/launch-plan-review.sh" exec || ((errors++))
copy_file "$SCRIPT_DIR/commands/revdiff.md"  "$OPENCODE_CONFIG/commands/revdiff.md" || ((errors++))
copy_file "$SCRIPT_DIR/tools/revdiff.ts"     "$OPENCODE_CONFIG/tools/revdiff.ts" || ((errors++))
copy_file "$SCRIPT_DIR/plugins/revdiff-plan-review.ts"   "$OPENCODE_CONFIG/plugins/revdiff-plan-review.ts" || ((errors++))


OPENCODE_JSON="$HOME/.config/opencode/opencode.json"
PLUGIN_ENTRY="./plugins/revdiff-plan-review.ts"

if [[ -f "$OPENCODE_JSON" ]]; then
  if ! jq -e --arg e "$PLUGIN_ENTRY" '.plugin // [] | index($e) != null' "$OPENCODE_JSON" >/dev/null 2>&1; then
    tmp=$(mktemp)
    jq --arg e "$PLUGIN_ENTRY" '.plugin = ((.plugin // []) + [$e])' "$OPENCODE_JSON" > "$tmp" && mv "$tmp" "$OPENCODE_JSON"
    echo "Appended \"$PLUGIN_ENTRY\" to plugin array in $OPENCODE_JSON"
  else
    echo "\"$PLUGIN_ENTRY\" already present in $OPENCODE_JSON, skipping."
  fi
else
  echo "WARNING: $OPENCODE_JSON not found, skipping plugin registration." >&2
fi

if ((errors > 0)); then
  echo "Done with $errors error(s)." >&2
  exit 1
fi

echo "Done."
