# Configuration

## Config File

Location: `~/.config/revdiff/config` (INI format). Override with `--config` flag or `REVDIFF_CONFIG` env var.

Precedence: CLI flags > env vars > config file > built-in defaults.

Generate a default config:
```bash
mkdir -p ~/.config/revdiff
revdiff --dump-config > ~/.config/revdiff/config
```

Then uncomment and edit the values you want to change.

## Options

| Option | Env var | Description | Default |
|--------|---------|-------------|---------|
| `--staged` | `REVDIFF_STAGED` | Show staged changes | `false` |
| `--tree-width` | `REVDIFF_TREE_WIDTH` | File tree panel width in units (1-10) | `2` |
| `--tab-width` | `REVDIFF_TAB_WIDTH` | Spaces per tab character | `4` |
| `--no-colors` | `REVDIFF_NO_COLORS` | Disable all colors including syntax highlighting | `false` |
| `--no-status-bar` | `REVDIFF_NO_STATUS_BAR` | Hide the status bar | `false` |
| `--wrap` | `REVDIFF_WRAP` | Enable line wrapping in diff view | `false` |
| `--collapsed` | `REVDIFF_COLLAPSED` | Start in collapsed diff mode | `false` |
| `--no-confirm-discard` | `REVDIFF_NO_CONFIRM_DISCARD` | Skip confirmation when discarding annotations with Q | `false` |
| `--chroma-style` | `REVDIFF_CHROMA_STYLE` | Chroma color theme for syntax highlighting | `catppuccin-macchiato` |
| `-F`, `--only` | | Show only matching files (may be repeated, matches by path or suffix) | |
| `-o`, `--output` | `REVDIFF_OUTPUT` | Write annotations to file instead of stdout | |
| `--config` | `REVDIFF_CONFIG` | Path to config file | `~/.config/revdiff/config` |
| `--dump-config` | | Print default config to stdout and exit | |

## Popup Size (Claude Code plugin)

When launched via the Claude Code plugin skill, revdiff opens in a terminal overlay. The popup size is configurable via env vars:

| Env var | Description | Default |
|---------|-------------|---------|
| `REVDIFF_POPUP_WIDTH` | Tmux popup width (e.g., `100%`, `80%`) | `90%` |
| `REVDIFF_POPUP_HEIGHT` | Tmux popup height / wezterm split percent | `90%` |

## Color Customization

All color options accept hex values (`#rrggbb`) and have corresponding `REVDIFF_COLOR_*` env vars.

| Option | Description | Default |
|--------|-------------|---------|
| `--color-accent` | Active pane borders and directory names | `#D5895F` |
| `--color-border` | Inactive pane borders | `#585858` |
| `--color-normal` | File entries and context lines | `#d0d0d0` |
| `--color-muted` | Divider lines and status bar | `#585858` |
| `--color-selected-fg` | Selected file text | `#ffffaf` |
| `--color-selected-bg` | Selected file background | `#D5895F` |
| `--color-annotation` | Annotation text and markers | `#ffd700` |
| `--color-cursor-fg` | Cursor indicator color | `#bbbb44` |
| `--color-cursor-bg` | Cursor indicator background | terminal default |
| `--color-add-fg` | Added line text | `#87d787` |
| `--color-add-bg` | Added line background | `#123800` |
| `--color-remove-fg` | Removed line text | `#ff8787` |
| `--color-remove-bg` | Removed line background | `#4D1100` |
| `--color-modify-fg` | Modified line text (collapsed mode) | `#f5c542` |
| `--color-modify-bg` | Modified line background (collapsed mode) | `#3D2E00` |
| `--color-tree-bg` | File tree pane background | terminal default |
| `--color-diff-bg` | Diff pane background | terminal default |
| `--color-status-fg` | Status bar foreground | `#202020` |
| `--color-status-bg` | Status bar background | `#C5794F` |
| `--color-search-fg` | Search match text | `#1a1a1a` |
| `--color-search-bg` | Search match background | `#4a4a00` |

## Chroma Syntax Highlighting Styles

Set via `--chroma-style=<name>`, env var `REVDIFF_CHROMA_STYLE`, or config file `chroma-style = <name>`.

**Dark themes:** `aura-theme-dark`, `aura-theme-dark-soft`, `base16-snazzy`, `catppuccin-frappe`, `catppuccin-macchiato` (default), `catppuccin-mocha`, `doom-one`, `doom-one2`, `dracula`, `evergarden`, `fruity`, `github-dark`, `gruvbox`, `hrdark`, `monokai`, `modus-vivendi`, `native`, `nord`, `nordic`, `onedark`, `paraiso-dark`, `rose-pine`, `rose-pine-moon`, `rrt`, `solarized-dark`, `solarized-dark256`, `tokyonight-moon`, `tokyonight-night`, `tokyonight-storm`, `vim`, `vulcan`, `witchhazel`, `xcode-dark`

**Light themes:** `autumn`, `borland`, `catppuccin-latte`, `colorful`, `emacs`, `friendly`, `github`, `gruvbox-light`, `igor`, `lovelace`, `manni`, `modus-operandi`, `monokailight`, `murphy`, `paraiso-light`, `pastie`, `perldoc`, `pygments`, `rainbow_dash`, `rose-pine-dawn`, `solarized-light`, `tango`, `tokyonight-day`, `trac`, `vs`, `xcode`

**Other:** `RPGLE`, `abap`, `algol`, `algol_nu`, `arduino`, `ashen`, `average`, `bw`, `hr_high_contrast`, `onesenterprise`, `swapoff`
