#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

OPENCODE_BIN="opencode"
CONFIG_DIR=""

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

usage() {
    echo "usage: bash setup.sh [--opencode PATH] [opencode-config-directory]"
}

install_v1() {
    local config_file="$CONFIG_DIR/opencode.json"
    local plugin_entry="./plugins/revdiff-plan-review.ts"
    if [[ -f "$config_file" ]]; then
        command -v jq >/dev/null 2>&1 || fail "jq is required to update opencode.json."
        jq -e 'type == "object" and (.plugin == null or (.plugin | type == "array"))' "$config_file" >/dev/null || fail "Invalid opencode.json or plugin array."
    fi

    mkdir -p "$CONFIG_DIR/commands" "$CONFIG_DIR/tools" "$CONFIG_DIR/plugins"
    cp "$SCRIPT_DIR/commands/revdiff.md" "$CONFIG_DIR/commands/"
    cp "$SCRIPT_DIR/tools/revdiff.ts" "$CONFIG_DIR/tools/"
    cp "$SCRIPT_DIR/plugins/revdiff-plan-review.ts" "$CONFIG_DIR/plugins/"
    cp "$REPO_ROOT/.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh" "$CONFIG_DIR/tools/"
    cp "$REPO_ROOT/plugins/revdiff-planning/scripts/launch-plan-review.sh" "$CONFIG_DIR/plugins/"
    chmod +x "$CONFIG_DIR/tools/launch-revdiff.sh" "$CONFIG_DIR/plugins/launch-plan-review.sh"

    if [[ ! -f "$config_file" ]]; then
        printf '{"plugin": ["%s"]}\n' "$plugin_entry" > "$config_file"
    elif ! jq -e --arg entry "$plugin_entry" '.plugin // [] | index($entry) != null' "$config_file" >/dev/null; then
        local temporary
        temporary=$(mktemp "$CONFIG_DIR/.revdiff-config-XXXXXX")
        if jq --arg entry "$plugin_entry" '.plugin = ((.plugin // []) + [$entry])' "$config_file" > "$temporary"; then
            mv "$temporary" "$config_file"
        else
            rm -f "$temporary"
            fail "Could not register the v1 plugin."
        fi
    fi
}

install_v2() {
    local target="$CONFIG_DIR/plugins/revdiff"
    local file
    for file in "$target"/index.* "$target"/server.*; do
        if [[ -e "$file" ]]; then
            fail "$target contains index.* or server.*, which OpenCode would load as a server plugin; move that directory aside before installing the CLI-only plugin."
        fi
    done
    local config_file="$CONFIG_DIR/opencode.json"
    local entry="./plugins/revdiff-plan-review.ts"
    if [[ -f "$config_file" ]]; then
        command -v jq >/dev/null 2>&1 || fail "jq is required to clean up opencode.json."
        if ! jq empty "$config_file" >/dev/null 2>&1; then
            echo "Notice: opencode.json is unchanged. Remove the exact $entry registration manually if present."
        else
            jq -e 'type == "object"' "$config_file" >/dev/null || fail "Invalid opencode.json."
            if jq -e --arg entry "$entry" 'any((.plugin[]?, .plugins[]?); . == $entry)' "$config_file" >/dev/null; then
                local temporary
                temporary=$(mktemp "$CONFIG_DIR/.revdiff-config-XXXXXX")
                if jq --arg entry "$entry" 'reduce ["plugin", "plugins"][] as $key (.; if (.[$key] | type) == "array" then .[$key] |= map(select(. != $entry)) else . end)' "$config_file" > "$temporary"; then
                    mv "$temporary" "$config_file"
                else
                    rm -f "$temporary"
                    fail "Could not clean up the v1 plugin registration."
                fi
            fi
        fi
    fi
    if [[ -f "$CONFIG_DIR/opencode.jsonc" ]]; then
        echo "Notice: opencode.jsonc is unchanged. Remove the exact $entry registration manually if present."
    fi

    mkdir -p "$target/scripts"
    for file in tui.ts claims.ts launcher.ts package.json; do
        cp "$SCRIPT_DIR/v2/$file" "$target/$file"
    done
    cp "$REPO_ROOT/.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh" "$target/scripts/"
    cp "$REPO_ROOT/.claude-plugin/skills/revdiff/scripts/agentdeck-window.sh" "$target/scripts/"
    chmod +x "$target/scripts/launch-revdiff.sh"
    rm -f "$CONFIG_DIR/plugins/revdiff-plan-review.ts" "$CONFIG_DIR/commands/revdiff.md"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --opencode)
            [[ $# -ge 2 && -n "$2" && "$2" != -* ]] || fail "--opencode requires a binary path."
            OPENCODE_BIN="$2"
            shift 2
            ;;
        -h|--help) usage; exit 0 ;;
        -*) usage >&2; fail "Unknown installer option." ;;
        *)
            [[ -z "$CONFIG_DIR" ]] || fail "Only one config directory may be specified."
            CONFIG_DIR="$1"
            shift
            ;;
    esac
done

command -v "$OPENCODE_BIN" >/dev/null 2>&1 || fail "OpenCode binary not found; use --opencode PATH."
VERSION_OUTPUT=$("$OPENCODE_BIN" --version) || fail "Could not determine the OpenCode version."
VERSION_PATTERN='^(opencode[[:space:]]+)?v?([0-9]+)\.([0-9]+)\.([0-9]+)([-+][[:alnum:].-]+)?$'
[[ "$VERSION_OUTPUT" =~ $VERSION_PATTERN ]] || fail "Unrecognized OpenCode version output."
MAJOR=$((10#${BASH_REMATCH[2]}))
MINOR=$((10#${BASH_REMATCH[3]}))
PATCH=$((10#${BASH_REMATCH[4]}))

case "$MAJOR" in
    1)
        [[ -z "$CONFIG_DIR" || "${CONFIG_DIR%/}" == "$HOME/.config/opencode" ]] || fail "Custom config directories are supported only for OpenCode v2; v1 plan review requires $HOME/.config/opencode."
        CONFIG_DIR="$HOME/.config/opencode"
        install_v1
        ;;
    2)
        CONFIG_DIR="${CONFIG_DIR:-${OPENCODE_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/opencode}}"
        install_v2
        ;;
    *) fail "Unsupported OpenCode major version; expected v1 or v2." ;;
esac
echo "Installed revdiff v$MAJOR integration for OpenCode $MAJOR.$MINOR.$PATCH in $CONFIG_DIR"
echo "Restart OpenCode to load the installed integration."
