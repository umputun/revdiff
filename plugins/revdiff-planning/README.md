# revdiff-planning

Claude Code plugin that intercepts `ExitPlanMode` and opens the proposed plan in the [revdiff](https://github.com/umputun/revdiff) TUI for interactive annotation. Annotations captured in the overlay are returned as the deny reason, prompting Claude to adjust the plan and call `ExitPlanMode` again.

## Install

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff-planning@umputun-revdiff
```

Requires the `revdiff` binary in `PATH` and one of: tmux, Zellij, kitty, wezterm, cmux, ghostty (macOS), iTerm2 (macOS), or Emacs vterm.

## How It Works

The plugin registers a `PreToolUse` hook on `ExitPlanMode` that:

1. Reads the plan content from the hook event JSON.
2. Writes the plan to a temp file.
3. Resolves `launch-plan-review.sh` through the override chain (see below).
4. Launches revdiff in a terminal overlay against the temp plan file.
5. Returns the annotations (if any) as the deny reason; otherwise allows the original `ExitPlanMode` to proceed.

## Overrides

The hook resolves `launch-plan-review.sh` through a three-layer chain (first-found wins). Drop your own launcher into either layer to customize how revdiff opens (separate window, alternate split layout, custom terminal multiplexer) without forking the plugin.

| Layer | Path | Scope |
|---|---|---|
| Project | `.claude/revdiff-planning/scripts/launch-plan-review.sh` | this repo only (commit alongside the project) |
| User | `${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh` | every project (per-user, lives under `~/.claude/plugins/data/<plugin-id>/`) |
| Bundled | `${CLAUDE_PLUGIN_ROOT}/scripts/launch-plan-review.sh` | default — ships with the plugin, used when no override is present |

The override file must be **executable** (`chmod +x`). A non-executable file in an override layer is treated as absent — the resolver falls through to the next layer rather than erroring. Using `chmod -x` is a quick way to disable an override without deleting the file.

To start from the bundled launcher as a template:

```bash
mkdir -p .claude/revdiff-planning/scripts
cp "${CLAUDE_PLUGIN_ROOT}/scripts/launch-plan-review.sh" .claude/revdiff-planning/scripts/launch-plan-review.sh
chmod +x .claude/revdiff-planning/scripts/launch-plan-review.sh
# edit .claude/revdiff-planning/scripts/launch-plan-review.sh to taste
```

The override receives the same single positional argument the bundled launcher does (the absolute path to the plan file). Print captured annotations to stdout on exit so the hook can include them in the deny reason; print nothing to allow the plan as-is.
