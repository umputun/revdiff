# Ghostty Terminal Support

## Overview
- Add Ghostty as a terminal backend for revdiff's overlay launch scripts
- Uses Ghostty's AppleScript API (1.3.0+, macOS only) to create split panes with zoom
- Detection via `TERM_PROGRAM=ghostty` + AppleScript probe (`get version`)
- Positioned last in detection chain: tmux → kitty → wezterm → ghostty → error

## Context
- `launch-revdiff.sh` and `launch-plan-review.sh` detect terminal via env vars and use terminal-specific APIs
- Kitty and wezterm use sentinel file polling (sleep 0.3) for blocking — ghostty follows same pattern
- Ghostty AppleScript: `new surface configuration` with `command` field, `split` with `direction down`
- AppleScript quoting is avoided by writing the shell command to a temp script file

## Design Decisions
- Split pane with `toggle_split_zoom` — zooms revdiff pane to fill the tab (closest to kitty overlay / tmux popup)
- Temp launch script avoids AppleScript nested quoting issues
- Detection guard checks `TERM_PROGRAM=ghostty` + `command -v osascript` (skips on Linux); if AppleScript is disabled, the split osascript fails with a descriptive error on stderr
- `wait after command` set to `false` so pane auto-closes after revdiff exits
- Terminal closed via `close terminal id` after sentinel fires to dismiss "press any key" prompt
- `launch-revdiff.sh` sets `initial working directory` for CWD; `launch-plan-review.sh` omits it since `--only` uses absolute paths
- No Go code changes — purely shell script

## Implementation Steps

### Task 1: Add Ghostty support to launch-revdiff.sh
- [x] update header comment to mention ghostty: `(tmux/kitty/wezterm/ghostty)`
- [x] add ghostty block after wezterm `fi`, before the error `echo`
- [x] update final error message to: `"requires tmux, kitty, wezterm, or ghostty on macOS"`
- [x] validate: `bash -n .claude-plugin/skills/revdiff/scripts/launch-revdiff.sh`

### Task 2: Add Ghostty support to launch-plan-review.sh
- [x] add ghostty block after wezterm `fi` — same pattern but without `initial working directory` in surface config
- [x] update final error message to: `"requires tmux, kitty, wezterm, or ghostty on macOS"`
- [x] update `plan-review-hook.py` line 14: `"tmux, kitty, or wezterm terminal"` → `"tmux, kitty, wezterm, or ghostty (macOS) terminal"`
- [x] validate: `bash -n plugins/revdiff-planning/scripts/launch-plan-review.sh`
