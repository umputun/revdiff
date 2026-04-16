# Changelog

## v0.18.1 - 2026-04-16

### Improvements

- structural refactor: split main.go, theme boundary cleanup, UI state consolidation #107 @umputun
- bump github.com/mattn/go-runewidth from 0.0.22 to 0.0.23 #106 @app/dependabot
- bump github.com/charmbracelet/x/ansi from 0.11.6 to 0.11.7 #105 @app/dependabot
- bump plugin versions (claude 0.7.4, planning 0.2.3, pi 0.1.1) 5555d85
- clean up dependencies and update Go version to 1.26 a98c168
- add ARCHITECTURE.md and slim CLAUDE.md 2efe306

### Bug Fixes

- fix mktemp file/path.XXXX.suffix bug #108 @tsimoshka
- fix exhaustive linter errors in action switch statements f714138

## v0.18.0 - 2026-04-13

### New Features

- add `--include` prefix filter flag #103 @rashpile
- opencode config docs #41 @hackmajoris

### Improvements

- add AUR install instructions #101 @kovstas
- extract overlay sub-package from model #99 @umputun
- add sandbox workaround for Ghostty and iTerm2 launchers ea38082
- add deb, rpm, and AUR install instructions to readme and site 6663e80
- improve opencode integration and add docs 4e8cda7

### Bug Fixes

- show single-column line numbers for full-context files #98 @rashpile

## v0.17.0 - 2026-04-12

### New Features

- add Mercurial VCS support #90 @paskal
- add history fallback to revdiff skill #95 @umputun

### Improvements

- extract style sub-package with Resolver/Renderer/SGR #92 @umputun
- extract sidepane sub-package with FileTree and TOC #93 @umputun
- extract worddiff sub-package with Differ type #96 @umputun
- add lint-scripts CI job, fix SC2181 shellcheck warnings #91 @paskal
- harden ci.yml caching and pin golangci-lint version 3b6559b
- pin hunk-centering test expectations to literals e50fade
- add status bar icons reference table f4421f3

### Bug Fixes

- center entire hunk in viewport during hunk navigation #83 @p4elkin

## v0.16.1 - 2026-04-10

### Improvements

- add revdiff review tasks to local zed config 215df34

### Bug Fixes

- soften auto-derived word-diff bg shift for better contrast 8ec62e5
- draw right scroll overflow glyph on DiffBg instead of line bg 282f174

## v0.16.0 - 2026-04-10

### New Features

- add horizontal scroll overflow indicators #89 @umputun
- make intra-line word-diff highlighting opt-in #88 @umputun
- add intra-line word-diff highlighting #87 @umputun
- add Codex CLI plugin for revdiff #86 @umputun
- add Zed integration task for running revdiff #77 @rashpile
- show explanations in revdiff TUI via --only e70c9c4

### Improvements

- wrap Zed tasks in JSON array and quote shell vars d84767a
- add clone step and symlink alternative to codex plugin install 7e1ea68
- add contribution guidelines for issues, PRs, and scope evaluation 7f45727

### Bug Fixes

- preserve syntax highlight foreground on wrapped lines #85 @umputun
- correct codex skill install path and remove dead plugin manifest f16be04
- use window_id instead of id in kitty overlay matcher aa83b55
- auto-detect staged-only changes and pass --staged to revdiff 249abf9

## v0.15.3 - 2026-04-09

### Bug Fixes

- suppress cmux send stdout to prevent output leak #81 @jimmyn
- load staged diff content for staged-only files in default mode #80 @sanchesfree

## v0.15.2 - 2026-04-09

### New Features

- add screenshot gallery with auto-rotating crossfade to site

### Improvements

- comprehensive code smells cleanup #76 @umputun

### Bug Fixes

- use raw ANSI for cursor and annotation to preserve DiffBg theme background
- change untracked files status icon from ? to ∅ to avoid conflict with help key
- set cursor-bg to match diff-bg in all bundled themes

## v0.15.1 - 2026-04-08

### Bug Fixes

- exclude file with no changes from file list #75 @daulet

## v0.15.0 - 2026-04-08

### New Features

- review history auto-save on quit #72 @umputun
- community themes, gallery, CLI install, and interactive selector #69 @melonamin
- pause review loop on explanation annotations

### Improvements

- layout-agnostic key bindings for non-Latin keyboards #71 @sanchesfree
- use CLAUDE_SKILL_DIR instead of CLAUDE_PLUGIN_ROOT #70 @rashpile
- handle long-running launcher on harnesses with short bash timeouts #51 @rashpile

### Bug Fixes

- shell-quote arguments in overlay launcher scripts #58 @melonamin
- add fsutil tests and handle SetStyle error in theme cancel

## v0.14.1 - 2026-04-08

### Bug Fixes

- truncate long diff lines to prevent overflow past right padding
- extend line bg after scroll so colored backgrounds fill at any offset

## v0.14.0 - 2026-04-08

### New Features

- show untracked and staged-only files in file tree (u toggle) (#62)
- reviewed file marks and A/M/D status indicators in file tree (#54)
- global hunk navigation (#59)
- zellij.dev support (#53)
- pi package integration (#52)

### Improvements

- split large files by concern (#65)
- move source packages into app/ directory (#49)
- consolidate assets into site/assets, remove root assets/
- bump revdiff plugin version to 0.5.0

### Bug Fixes

- annotation on last row of the view (#60)
- lowercase zellij name to match other terminal entries
- add missing feature cards and center install grid
- restore correct two-color logo in site/assets
- handle no-commits repo in detect-ref script
- use TMPDIR for temp files in launch script to avoid macOS sandbox restriction

## v0.13.0 - 2026-04-06

### New Features

- --stdin scratch-buffer review mode for piped content (#46)
- hunk keyword expansion in annotation output (#47)
- binary file detection with size delta and IsBinary flag (#44)
- two-column help overlay with colored section headers and keys
- zellij terminal support for floating pane overlay launcher

### Bug Fixes

- add right padding to prevent wrapped text from touching pane border (#45)
- enable word wrap by default for plan review

## v0.12.0 - 2026-04-06

### New Features

- git blame gutter toggle (B key) (#38)
- --line-numbers config option (#37)
- kaku terminal support for wezterm-based terminals (#42)
- cmux terminal support for overlay launcher (#35)
- Emacs vterm support (#33)
- project website and branding

### Improvements

- "beyond code review" section with use cases for --only flag

### Bug Fixes

- skip tmux -T title flag on versions older than 3.3 (#40)
- Safari iOS mobile layout issues (#39)
- improve site readability by lifting dark theme palette
- increase docs page font sizes

## v0.11.0 - 2026-04-05

### New Features

- custom keybindings via ~/.config/revdiff/keymap
- color theme system with 5 bundled themes (dracula, nord, gruvbox, solarized-dark, catppuccin-mocha)
- Ghostty terminal support
- line numbers gutter toggle (L key)

## v0.10.0 - 2026-04-05

### New Features

- add --all-files and --exclude modes (#19)
- add t hotkey to toggle tree/TOC pane visibility (#18)
- pass REVDIFF_CONFIG to overlay and add configurable popup size (#17)
- add revdiff-planning plugin for automatic plan review

### Bug Fixes

- fix TOC highlighted entry wrapping to two lines

## v0.9.0 - 2026-04-04

### New Features

- markdown TOC navigation pane for single-file full-context mode (#16)

### Improvements

- track vendor directory and update dependencies

## v0.8.0 - 2026-04-04

### New Features

- annotation list popup (`@` key) to view, navigate, and jump to any annotation across all files (#15)

## v0.7.2 - 2026-04-04

### Bug Fixes

- add `--collapsed` flag to start in collapsed diff mode, allowing users to persist preference via CLI, config file, or `REVDIFF_COLLAPSED` env var (#14)

## v0.7.1 - 2026-04-04

### New Features

- two-ref positional args: `revdiff base against` (e.g. `revdiff main feature`) for diffing between arbitrary refs (#13)
- `..` and `...` syntax supported in single arg (e.g. `revdiff main..feature`)
- validation: `--staged` rejected with two-ref or range diffs

## v0.7.0 - 2026-04-04

### New Features

- no-git file review mode: `--only` files without git changes shown as context-only with full annotation and syntax highlighting support (#12)
- standalone file review outside a git repo via `--only` (reads files directly from disk)

### Improvements

- plugin skill updated with file review mode guidance (v0.2.3)

## v0.6.0 - 2026-04-03

### New Features

- `--only`/`-F` flag to filter files by exact path or suffix, may be repeated (#11)
- shows "no files match --only filter" message when filter has no matches

## v0.5.0 - 2026-04-03

### New Features

- single-file auto-detection: when diff has exactly one file, hides the tree pane and gives full terminal width to the diff view (#10)

### Bug Fixes

- correct annotation input width to fit within diff pane, preventing cursor overflow

## v0.4.2 - 2026-04-03

### Bug Fixes

- wrap long annotations at pane width regardless of wrap mode

## v0.4.1 - 2026-04-03

### Bug Fixes

- center viewport on search match navigation (matches hunk navigation centering behavior)

### Improvements

- add project logo and move assets to `assets/` directory

## v0.4.0 - 2026-04-02

### New Features

- collapsed diff mode — toggle with `v`, shows final text with change markers, expand individual hunks with `.`
- status line with filename, diff stats, hunk position, and always-visible mode indicators (▼ ◉ ↩ ≋)
- help overlay — press `?` for organized keybinding reference, composited on top of content
- word wrap mode — toggle with `w`, wraps long lines with `↪` continuation markers, `--wrap` CLI flag
- vim-style `/` search in diff pane with `n`/`N` match navigation, `esc` to clear
- configurable search highlight colors (`--color-search-fg`, `--color-search-bg`)

### Improvements

- search highlighting uses background-only ANSI to preserve syntax colors within matches
- reverse video fallback for search highlights in `--no-colors` mode
- mode indicators always visible (muted when inactive, active foreground when on)
- muted pipe separators in status line using raw ANSI to preserve background
- truncate long filenames in tree pane to prevent selection highlight wrapping
- extract collapsed diff mode into separate file for maintainability

### Fixed

- help overlay renders on top of content instead of replacing it
- hunk count always shown in status line (not just when cursor is on changed line)
- singular/plural handling for "1 hunk" vs "N hunks"
- launch script flag parsing hardened for short flags and `-o`/`--output`

## v0.3.0 - 2026-04-02

### New Features

- add Q hotkey to discard annotations and quit without output (#4)
- update default color scheme to catppuccin-macchiato with warm accent colors

### Improvements

- expand Claude Code plugin section with usage examples and smart detection

## v0.2.4 - 2026-04-01

### New Features

- smart ref detection in Claude Code plugin — auto-detects branch and uncommitted state

### Fixed

- change hunk navigation hint from `[/]` to `[ ]` in status bar to avoid confusion with key grouping

## v0.2.3 - 2026-04-01

### Fixed

- resolve git repo root so revdiff works from subdirectories (#2)

### Improvements

- document supported terminal overlays for Claude Code plugin

## v0.2.2 - 2026-04-01

### Fixed

- remove default cursor background so triangle uses terminal default
- truncate long directory names from the left with ellipsis in file tree

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
