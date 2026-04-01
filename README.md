# revdiff [![Build Status](https://github.com/umputun/revdiff/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/revdiff/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/umputun/revdiff/badge.svg?branch=master)](https://coveralls.io/github/umputun/revdiff?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/umputun/revdiff)](https://goreportcard.com/report/github.com/umputun/revdiff)

Lightweight TUI for reviewing git diffs with inline annotations. Outputs structured annotations to stdout on quit, making it easy to pipe results into AI agents, scripts, or other tools.

Built for a specific use case: reviewing code changes without leaving a terminal-based AI coding session (e.g., Claude Code). Just enough UI to navigate a full-file diff, annotate specific lines, and return the results to the calling process - no more, no less.

## Features

- Structured annotation output to stdout - pipe into AI agents, scripts, or other tools
- Full-file diff view with syntax highlighting (powered by [Chroma](https://github.com/alecthomas/chroma))
- Annotate any line in the diff (added, removed, or context) plus file-level notes
- Two-pane TUI: file tree (left) + colorized diff viewport (right) with cursor bar indicator
- Chunk navigation to jump between change groups
- Filter file tree to show only annotated files

## Installation

```bash
go install github.com/umputun/revdiff/cmd/revdiff@latest
```

## Usage

```
revdiff [OPTIONS] [ref]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `ref` | Git ref to diff against | uncommitted changes |
| `--staged` | Show staged changes, env: `REVDIFF_STAGED` | `false` |
| `--tree-width` | File tree panel width in units (1-10), env: `REVDIFF_TREE_WIDTH` | `3` |
| `--tab-width` | Number of spaces per tab character, env: `REVDIFF_TAB_WIDTH` | `4` |
| `--no-colors` | Disable all colors including syntax highlighting, env: `REVDIFF_NO_COLORS` | `false` |
| `--chroma-style` | Chroma color theme for syntax highlighting, env: `REVDIFF_CHROMA_STYLE` | `monokai` |
| `--config` | Path to config file, env: `REVDIFF_CONFIG` | `~/.config/revdiff/config` |
| `--dump-config` | Print default config to stdout and exit | |
| `-V`, `--version` | Show version info | |

### Config File

All options can be set in a config file at `~/.config/revdiff/config` (INI format). CLI flags and environment variables override config file values.

Generate a default config file:

```bash
mkdir -p ~/.config/revdiff
revdiff --dump-config > ~/.config/revdiff/config
```

Then uncomment and edit the values you want to change.

<details>
<summary>Color customization flags (click to expand)</summary>

All color options accept hex values (`#rrggbb`) and have corresponding `REVDIFF_COLOR_*` env vars.

| Option | Description | Default |
|--------|-------------|---------|
| `--color-accent` | Active pane borders and directory names | `#5f87ff` |
| `--color-border` | Inactive pane borders | `#585858` |
| `--color-normal` | File entries and context lines | `#d0d0d0` |
| `--color-muted` | Line numbers and status bar | `#6c6c6c` |
| `--color-selected-fg` | Selected file text | `#ffffaf` |
| `--color-selected-bg` | Selected file background | `#303030` |
| `--color-annotation` | Annotation text and markers | `#ffd700` |
| `--color-cursor-bg` | Cursor line background | `#3a3a3a` |
| `--color-cursor-bar` | Cursor line vertical bar | `#d7af00` |
| `--color-add-fg` | Added line text | `#87d787` |
| `--color-add-bg` | Added line background | `#022800` |
| `--color-remove-fg` | Removed line text | `#ff8787` |
| `--color-remove-bg` | Removed line background | `#3D0100` |

</details>

<details>
<summary>Available chroma styles (click to expand)</summary>

**Dark themes:** `aura-theme-dark`, `aura-theme-dark-soft`, `base16-snazzy`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `doom-one`, `doom-one2`, `dracula`, `evergarden`, `fruity`, `github-dark`, `gruvbox`, `hrdark`, `monokai` (default), `modus-vivendi`, `native`, `nord`, `nordic`, `onedark`, `paraiso-dark`, `rose-pine`, `rose-pine-moon`, `rrt`, `solarized-dark`, `solarized-dark256`, `tokyonight-moon`, `tokyonight-night`, `tokyonight-storm`, `vim`, `vulcan`, `witchhazel`, `xcode-dark`

**Light themes:** `autumn`, `borland`, `catppuccin-latte`, `colorful`, `emacs`, `friendly`, `github`, `gruvbox-light`, `igor`, `lovelace`, `manni`, `modus-operandi`, `monokailight`, `murphy`, `paraiso-light`, `pastie`, `perldoc`, `pygments`, `rainbow_dash`, `rose-pine-dawn`, `solarized-light`, `tango`, `tokyonight-day`, `trac`, `vs`, `xcode`

**Other:** `RPGLE`, `abap`, `algol`, `algol_nu`, `arduino`, `ashen`, `average`, `bw`, `hr_high_contrast`, `onesenterprise`, `swapoff`

</details>

### Examples

```bash
# review uncommitted changes
revdiff

# review changes against a branch
revdiff main

# review staged changes
revdiff --staged

# review last commit
revdiff HEAD~1
```

### Key Bindings

**Navigation:**

| Key | Action |
|-----|--------|
| `j/k` or up/down | Navigate files (tree) / scroll diff (diff pane) |
| `h/l` | Switch between file tree and diff pane |
| left/right | Horizontal scroll in diff pane |
| `Tab` | Switch between file tree and diff pane |
| `PgDown/PgUp` | Page scroll in file tree and diff pane |
| `Ctrl+d/Ctrl+u` | Page scroll in file tree and diff pane |
| `Home/End` | Jump to first/last item |
| `Enter` | Switch to diff pane (tree) / start annotation (diff pane) |
| `n/p` | Next/previous changed file |
| `[` / `]` | Jump to previous/next change hunk in diff |

**Annotations:**

| Key | Action |
|-----|--------|
| `a` or `Enter` (diff pane) | Annotate current diff line |
| `A` | Add file-level annotation (stored at top of diff) |
| `d` | Delete annotation under cursor |
| `Esc` | Cancel annotation input |

**View:**

| Key | Action |
|-----|--------|
| `f` | Toggle filter: all files / annotated only (shown when annotations exist) |
| `q` | Quit, output annotations to stdout |

### Output Format

```
## handler.go (file-level)
consider splitting this file into smaller modules

## handler.go:43 (+)
use errors.Is() instead of direct comparison

## store.go:18 (-)
don't remove this validation
```

### Integration with AI Agents

The structured stdout output makes revdiff a natural fit for AI-assisted code review workflows. Launch revdiff as a terminal overlay (tmux popup, kitty overlay, wezterm split-pane), annotate the diff, quit, and feed the annotations back to the calling process.

```bash
# review changes and feed annotations to an AI agent
annotations=$(revdiff main)
if [ -n "$annotations" ]; then
  echo "$annotations" | your-ai-agent fix
fi
```

See [cc-thingz](https://github.com/umputun/cc-thingz) for Claude Code plugins with terminal overlay support (tmux, kitty, wezterm).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT License - see [LICENSE](LICENSE) file for details.
