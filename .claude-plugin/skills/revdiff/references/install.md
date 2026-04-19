# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Claude Code Plugin

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff@umputun-revdiff
```

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, Zellij, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm).

**Sandbox note:** Ghostty and iTerm2 launchers use `osascript` (Apple Events), which is blocked by Claude Code's sandbox. If you use these terminals with sandbox enabled, add to your Claude Code `settings.json`:

```json
{
  "permissions": {
    "excludedCommands": ["*/launch-revdiff.sh*"]
  }
}
```

Terminals using CLI tools (tmux, Zellij, kitty, wezterm, cmux) are not affected.

### Plan Review Plugin

Automatically opens revdiff when Claude exits plan mode for interactive annotation:

```bash
/plugin install revdiff-planning@umputun-revdiff
```

### Overrides

The diff-review skill resolves `launch-revdiff.sh` through a three-layer chain (first-found wins). Drop your own launcher into either layer to customize how revdiff opens (separate window, alternate split layout, custom terminal multiplexer) without forking the plugin.

| Layer | Path | Scope |
|---|---|---|
| Project | `.claude/revdiff/scripts/launch-revdiff.sh` | this repo only (commit alongside the project) |
| User | `${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh` | every project (per-user, lives under `~/.claude/plugins/data/<plugin-id>/`) |
| Bundled | `${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh` | default — ships with the plugin, used when no override is present |

The override file must be **executable** (`chmod +x`). A non-executable file in an override layer is treated as absent — the resolver falls through to the next layer rather than erroring. Using `chmod -x` is a quick way to disable an override without deleting the file.

To start from the bundled launcher as a template:

```bash
mkdir -p .claude/revdiff/scripts
cp "${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh" .claude/revdiff/scripts/launch-revdiff.sh
chmod +x .claude/revdiff/scripts/launch-revdiff.sh
# edit .claude/revdiff/scripts/launch-revdiff.sh to taste
```

Example — open revdiff in a fresh kitty window instead of an overlay (place at `${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh` for user-level scope):

```bash
#!/usr/bin/env bash
set -euo pipefail
TMPBASE="${TMPDIR:-/tmp}"
OUTPUT_FILE=$(mktemp "$TMPBASE/revdiff-output-XXXXXX")
trap 'rm -f "$OUTPUT_FILE"' EXIT
SENTINEL=$(mktemp "$TMPBASE/revdiff-done-XXXXXX")
rm -f "$SENTINEL"
kitty --detach --title "revdiff" sh -c "revdiff --output=$OUTPUT_FILE $*; touch $SENTINEL"
while [ ! -f "$SENTINEL" ]; do sleep 0.3; done
rm -f "$SENTINEL"
cat "$OUTPUT_FILE"
```

The override receives the same positional arguments the bundled launcher does (`[base] [against] [--staged] [--only=file1] [--all-files] [--exclude=prefix]`). Read stdout into the calling skill the same way the bundled launcher does — print captured annotations to stdout on exit.
