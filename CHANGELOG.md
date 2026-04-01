# Changelog

## v0.2.1 - 2026-04-01

### Improvements

- replace diff cursor bar (▎) with solid triangle (▶)
- add `--color-cursor-fg` flag to customize cursor indicator color

## v0.2.0 - 2026-04-01

### New Features

- add reference docs for revdiff plugin skill (install, config, usage)

### Fixed

- remove spurious `colors=` line from `--dump-config` output
- add trigger words to plugin skill description
- isolate tests from user's real config file

## v0.1.1 - 2026-04-01

### New Features

- add `--output` flag to write annotations to file instead of stdout
- Claude Code plugin with terminal overlay launcher (tmux, kitty, wezterm)
- goreleaser config and GitHub Actions release workflow
- homebrew tap via `umputun/homebrew-apps`

### Fixed

- fix plugin launcher: resolve binary to absolute path, set cwd for overlay
- fix shell quoting in `--output` path argument
- add trigger words to plugin skill description

## v0.1.0 - 2026-04-01

Initial release.

### New Features

- two-pane TUI with file tree and colorized diff viewport
- syntax highlighting via Chroma with configurable themes (`--chroma-style`)
- inline annotations on any diff line (added, removed, or context)
- file-level annotations
- hunk navigation with `[` / `]` keys
- horizontal scrolling for long lines with left/right arrows
- filter file tree to show only annotated files
- structured annotation output to stdout
- config file support (`~/.config/revdiff/config`, INI format)
- fully customizable colors via CLI flags, env vars, or config file
- configurable pane backgrounds (tree, diff, status bar)
- `--no-colors` flag to disable all colors
- `--no-status-bar` flag to hide status bar
- `--tab-width` flag for tab-to-spaces conversion
- `--dump-config` to generate default config
