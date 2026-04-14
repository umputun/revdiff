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
| `--line-numbers` | `REVDIFF_LINE_NUMBERS` | Show line numbers in diff gutter | `false` |
| `--blame` | `REVDIFF_BLAME` | Show blame gutter on startup | `false` |
| `--word-diff` | `REVDIFF_WORD_DIFF` | Highlight intra-line word-level changes in paired add/remove lines | `false` |
| `--no-confirm-discard` | `REVDIFF_NO_CONFIRM_DISCARD` | Skip confirmation when discarding annotations with Q | `false` |
| `--chroma-style` | `REVDIFF_CHROMA_STYLE` | Chroma color theme for syntax highlighting | `catppuccin-macchiato` |
| `--theme` | `REVDIFF_THEME` | Load color theme from `~/.config/revdiff/themes/` | |
| `--dump-theme` | | Print currently resolved colors as theme file and exit | |
| `--list-themes` | | Print available theme names and exit | |
| `--init-themes` | | Write bundled theme files to themes dir and exit | |
| `--init-all-themes` | | Write all gallery themes (bundled + community) to themes dir and exit | |
| `--install-theme` | | Install theme(s) from gallery or local file (repeatable) | |
| `-A`, `--all-files` | | Browse all git-tracked files, not just diffs (CLI-only, not saved in config) | `false` |
| `-I`, `--include` | `REVDIFF_INCLUDE` | Include only files matching prefix (may be repeated; comma-separated in env) | |
| `-X`, `--exclude` | `REVDIFF_EXCLUDE` | Exclude files matching prefix (may be repeated; comma-separated in env) | |
| `-F`, `--only` | | Show only matching files (may be repeated, matches by path or suffix) | |
| `-o`, `--output` | `REVDIFF_OUTPUT` | Write annotations to file instead of stdout | |
| `--history-dir` | `REVDIFF_HISTORY_DIR` | Directory for review history auto-saves | `~/.config/revdiff/history/` |
| `--keys` | `REVDIFF_KEYS` | Path to keybindings file | `~/.config/revdiff/keybindings` |
| `--dump-keys` | | Print effective keybindings to stdout and exit | |
| `--config` | `REVDIFF_CONFIG` | Path to config file | `~/.config/revdiff/config` |
| `--dump-config` | | Print default config to stdout and exit | |

## Popup Size (Claude Code plugin)

When launched via the Claude Code plugin skill, revdiff opens in a terminal overlay. The popup size is configurable via env vars:

| Env var | Description | Default |
|---------|-------------|---------|
| `REVDIFF_POPUP_WIDTH` | Tmux popup width (e.g., `100%`, `80%`) | `90%` |
| `REVDIFF_POPUP_HEIGHT` | Tmux popup height / wezterm split percent | `90%` |

## Themes

Seven bundled themes: **catppuccin-latte**, **catppuccin-mocha**, **dracula**, **gruvbox**, **nord**, **revdiff**, **solarized-dark**. Stored in `~/.config/revdiff/themes/`, auto-created on first run.

Press `T` inside revdiff to open the interactive theme selector with live preview — browse themes, see colors applied instantly, and persist your choice on confirm.

```bash
revdiff --theme dracula          # apply a theme
revdiff --list-themes            # list available themes
revdiff --init-themes            # re-create bundled themes
revdiff --install-theme nord     # install a specific gallery theme
revdiff --init-all-themes        # install all gallery themes
revdiff --dump-theme > ~/.config/revdiff/themes/my-custom  # export current colors
```

Set default theme in config: `theme = dracula`. Or env: `REVDIFF_THEME=dracula`.

**Custom themes:** customize colors in config or via `--color-*` flags, then `revdiff --dump-theme > ~/.config/revdiff/themes/my-custom`. Or copy a bundled theme file and edit directly — each has all 23 color keys + `chroma-style`.

Precedence: `--theme` takes over completely — overwrites all color fields, ignoring `--color-*` flags and env vars. Without `--theme`: built-in defaults → config file → env vars → CLI flags. `--theme` + `--no-colors` prints warning and applies theme.

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
| `--color-word-add-bg` | Intra-line word-diff add background | auto-derived from add-bg |
| `--color-word-remove-bg` | Intra-line word-diff remove background | auto-derived from remove-bg |
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

## Custom Keybindings

Location: `~/.config/revdiff/keybindings`. Override with `--keys` flag or `REVDIFF_KEYS` env var.

Format: `map <key> <action>` to bind, `unmap <key>` to remove a default binding. `#` comments and blank lines are ignored. Defaults are preserved unless explicitly unmapped.

Generate a template: `revdiff --dump-keys > ~/.config/revdiff/keybindings`

Example:
```
map x quit
unmap q
map ctrl+d half_page_down
```

Available actions: `down`, `up`, `page_down`, `page_up`, `half_page_down`, `half_page_up`, `home`, `end`, `scroll_left`, `scroll_right`, `next_item`, `prev_item`, `next_hunk`, `prev_hunk`, `toggle_pane`, `focus_tree`, `focus_diff`, `search`, `confirm`, `annotate_file`, `delete_annotation`, `annot_list`, `toggle_collapsed`, `toggle_wrap`, `toggle_tree`, `toggle_line_numbers`, `toggle_blame`, `toggle_hunk`, `toggle_untracked`, `mark_reviewed`, `theme_select`, `filter`, `quit`, `discard_quit`, `help`, `dismiss`

Modal keys (annotation input, search input, confirm discard) are not remappable.
