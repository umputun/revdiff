# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Claude Code Plugin

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff@revdiff
```

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, Zellij, herdr, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm).

cmux is detected before ghostty when `$CMUX_SURFACE_ID` is set, `__CFBundleIdentifier=com.cmuxterm.app`, or `GHOSTTY_RESOURCES_DIR` / `GHOSTTY_BIN_DIR` contains `cmux.app`, so cmux uses the cmux CLI instead of Ghostty AppleScript.

**Sandbox note:** Ghostty and iTerm2 launchers use `osascript` (Apple Events), which is blocked by Claude Code's sandbox. If you use these terminals with sandbox enabled, add to your Claude Code `settings.json`:

```json
{
  "permissions": {
    "excludedCommands": ["*/launch-revdiff.sh*"]
  }
}
```

Terminals using CLI tools (tmux, Zellij, herdr, kitty, wezterm, cmux) are not affected.

### Plan Review Plugin

Automatically opens revdiff when Claude exits plan mode for interactive annotation:

```bash
/plugin install revdiff-planning@revdiff
```

### Overrides

The diff-review skill resolves `launch-revdiff.sh` through a two-layer chain (first-found wins). Drop your own launcher into the user layer to customize how revdiff opens (separate window, alternate split layout, custom terminal multiplexer) without forking the plugin.

| Layer | Path | Scope |
|---|---|---|
| User | `${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh` | every project (per-user, lives under `~/.claude/plugins/data/<plugin-id>/`) |
| Bundled | `${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh` | default — ships with the plugin, used when no override is present |

There is no project-level (`.claude/...`) override layer by design: the resolver is shared with the `revdiff-planning` hook, which fires automatically on every `ExitPlanMode`, and a repo-controlled executable layer would let an untrusted repo run arbitrary code on routine Claude actions. The diff-review skill keeps the same two-layer shape for symmetry.

The override file must be **executable** (`chmod +x`). A non-executable file in the user layer is treated as absent — the resolver falls through to the bundled default rather than erroring. Using `chmod -x` is a quick way to disable an override without deleting the file.

To start from the bundled launcher as a template:

```bash
mkdir -p "${CLAUDE_PLUGIN_DATA}/scripts"
cp "${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh" "${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh"
chmod +x "${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh"
# edit "${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh" to taste
```

**Customizing the launcher**: production overrides should start from a copy of the bundled `launch-revdiff.sh` and modify only the terminal branch you care about. The bundled launcher already handles the tricky parts — shell-quoting positional args via the `sq()` helper, output-file lifecycle, sentinel polling, env-var propagation. Writing a thin wrapper from scratch is fragile: naive use of `$*` or `$@` inside `sh -c "..."` does not preserve arguments containing spaces, quotes, or globs.

For example, to open revdiff in a fresh kitty window instead of an overlay, copy the bundled launcher and replace the existing `kitty @ launch` overlay block with a `kitty --detach --title "revdiff"` invocation, reusing the existing `$REVDIFF_CMD` variable (which is already correctly quoted) — do **not** rebuild the command line from `$*`.

The override receives the same positional arguments the bundled launcher does (`[base] [against] [--staged] [--only=file1] [--all-files] [--exclude=prefix] [--description=text|--description-file=path]`). The bundled launcher sets `REVDIFF_EXIT_CODE_ON_ANNOTATIONS` internally; custom launchers should do the same, print captured annotations to stdout, and preserve exit `10` as success-with-annotations. Other nonzero statuses remain launcher failures.

**Tagging the review session (tmux)**: the tmux backend runs each review in a transient detached session. If other tooling reacts to tmux sessions (status bars, session managers, dashboards), set `REVDIFF_TMUX_SESSION_OPTIONS` to whitespace-separated `key=value` tokens; the launcher applies each as a tmux session option before any client attaches (e.g. `REVDIFF_TMUX_SESSION_OPTIONS="@my-manager-ignore=1"` lets a session manager recognize and skip the transient session). Values cannot contain spaces. This variable configures the launcher itself — set it in the shell environment where the plugin runs; it is unrelated to revdiff's own env vars, which don't survive into the overlay. The launcher always sets one option of its own: `@revdiff_title` holds the popup title, so custom tmux bindings that reattach a backgrounded review can restore it.

**Failure mode**: if the resolver finds no launcher in any layer (user / bundled), the skill's command substitution produces an empty string and bash reports `: command not found` with exit 127. The resolver's stderr (`error: launcher not found in override chain: launch-revdiff.sh`) is preserved — check it to confirm the override file is present and executable in one of the two layers above.
