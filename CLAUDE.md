# revdiff

TUI for reviewing diffs, files, and documents with inline annotations, built with bubbletea.

**Architecture**: see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for system design, data flows, interfaces, and design decisions.

## Commands
- Build: `make build` (output: `.bin/revdiff`)
- Test: `make test` (race detector + coverage, excludes mocks)
- Lint: `make lint` or `golangci-lint run`
- Format: `make fmt` or `~/.claude/format.sh`
- Generate mocks: `go generate ./...`
- Vendor after adding deps: `go mod vendor`

## Project Structure
- `app/` - entry point (`main.go`), CLI flags, wiring
- `app/diff/` - VCS interaction (git + hg), unified diff parsing, VCS detection, Mercurial support
- `app/ui/` - bubbletea TUI package. Single `Model` struct, methods split across files by concern (~500 lines each). Each source file has a matching `_test.go`. See `app/ui/doc.go` for package docs, `docs/ARCHITECTURE.md` for file-by-file breakdown
- `app/ui/style/` - color/style resolution: hex-to-ANSI, lipgloss styles, SGR tracking, HSL math. Types: `Resolver`, `Renderer`, `SGR`
- `app/ui/sidepane/` - file tree + markdown TOC components with cursor/offset management
- `app/ui/worddiff/` - intra-line word-diff: tokenizer, LCS, line pairing, highlight insertion
- `app/ui/overlay/` - layered popups: help, annotation list, theme selector. Manager enforces one-at-a-time
- `app/highlight/` - chroma syntax highlighting, foreground-only ANSI
- `app/keymap/` - configurable keybindings (`Action` constants, parser, defaults, dump)
- `app/theme/` - theme parse/load/dump/list (7 bundled + community gallery)
- `app/annotation/` - in-memory annotation store and structured output
- `app/history/` - review session auto-save to `~/.config/revdiff/history/`
- `app/fsutil/` - filesystem utilities
- `app/ui/mocks/` - moq-generated mocks (never edit manually)

## Config
- Config file: `~/.config/revdiff/config` (INI format via go-flags IniParser)
- Precedence: CLI flags > env vars > config file > built-in defaults
- `--dump-config` outputs current defaults, `--config` overrides path
- `no-ini:"true"` tag excludes fields from config file (used for --config, --dump-config, --dump-theme, --list-themes, --init-themes, --version)
- Themes dir: `~/.config/revdiff/themes/` with 7 bundled themes, auto-created on first run
- `--theme NAME` loads theme; `--dump-theme` exports resolved colors; `--list-themes` lists available; `--init-themes` re-creates bundled
- Theme precedence: `--theme` overwrites all 23 color fields + chroma-style, ignoring `--color-*` flags or env vars
- Theme values applied via `applyTheme()` in `main.go` which overwrites `opts.Colors.*` after `parseArgs()`. `colorFieldPtrs(opts)` is the single source of truth for color key → struct field mapping — adding a new color requires changes in `theme.go` colorKeys + options struct + `colorFieldPtrs()`
- `ini-name` tags ensure config keys match CLI long flag names
- Keybindings file: `~/.config/revdiff/keybindings` (`map <key> <action>` / `unmap <key>` format)
- `--keys` overrides keybindings path, `--dump-keys` prints effective bindings

## Website
- Static site in `site/` (index.html, docs.html, style.css), deployed to revdiff.com via Cloudflare Pages
- `site/docs.html` must stay in sync with README.md - when adding features, flags, keybindings, or modes, update both
- `site/index.html` landing page should reflect major new features in the features grid and plugin sections
- **CRITICAL: After each release, update the version badge in `site/index.html`** (search for `hero-badge` div) and `softwareVersion` in JSON-LD

## Claude Code Plugin
- Plugin lives at `.claude-plugin/` with `plugin.json`, `marketplace.json`, and `skills/`
- Skills path in `plugin.json` is relative to repo root, not to `.claude-plugin/`
- **CRITICAL: After any plugin file change, ask user if they want to bump the plugin version**
- When bumping, update version in both `plugin.json` and `marketplace.json`
- Reference docs at `.claude-plugin/skills/revdiff/references/` — keep in sync with README.md:
  - `install.md` — installation methods and plugin setup
  - `config.md` — options, colors, chroma styles
  - `usage.md` — examples, key bindings, output format

## Codex Skills
- Codex skills live at `plugins/codex/skills/` — two skills: `revdiff` (diff review) and `revdiff-plan` (plan review via last Codex assistant message)
- Install is a skills-only copy to `~/.codex/skills/<name>/` — codex does NOT scan `~/.codex/plugins/`, that path is reserved for plugins synced from `github.com/openai/plugins`
- No plugin manifest or marketplace envelope — the `.codex-plugin/` / `.agents/` layout was non-conformant with codex's actual format and removed
- Script path resolution in SKILL.md falls back to `${CODEX_HOME:-$HOME/.codex}/skills/<skill>/scripts` when not running inside the revdiff repo
- Scripts are copies from `.claude-plugin/skills/revdiff/scripts/`, not symlinks — each has a source comment at top
- Codex has no hook system — plan review is manual via `/revdiff-plan`

## Pi Plugin
- Pi package defined in root `package.json`, extensions and skills in `plugins/pi/`
- **CRITICAL: After any pi plugin file change, ask user if they want to bump the version in `package.json`**
- Version in `package.json` is independently versioned (does not track the project's git tags)

## Gotchas
- Project uses vendoring - run `go mod vendor` after adding/updating dependencies
- Chroma API uses British spelling (`Colour`), suppress with `//nolint:misspell`
- Syntax highlighting uses specific ANSI resets (`\033[39m`, `\033[22m`, `\033[23m`) instead of full reset (`\033[0m`) to preserve lipgloss backgrounds
- Highlighted lines are pre-computed once per file load, stored parallel to `diffLines`
- `DiffLine.Content` has no `+`/`-` prefix - prefix is re-added at render time
- Tab replacement happens at render time in `renderDiffLine`, not in diff parsing
- `run()` detects VCS via `diff.DetectVCS()` (walks up looking for `.git`/`.hg`); if no VCS is found and `--only` is set, uses `FileReader` for standalone file review. `--stdin` skips VCS lookup entirely, validates non-TTY stdin, reads payload before starting Bubble Tea, and reopens `/dev/tty` for interactive key input. `--all-files` is git-only (not supported in hg repos).
- `--all-files` mode uses `DirectoryReader` (git ls-files) to list all tracked files; `--include` wraps any renderer with `IncludeFilter` for prefix-based inclusion, `--exclude` wraps with `ExcludeFilter` for prefix-based exclusion (include narrows first, then exclude removes). `--include` is mutually exclusive with `--only`. `--all-files` is mutually exclusive with refs, `--staged`, and `--only`. `--stdin` is mutually exclusive with refs, `--staged`, `--only`, `--all-files`, `--include`, and `--exclude`.
- `diff.readReaderAsContext()` is the shared parser for file-backed and stdin-backed context-only views. Preserve its behavior if you change binary detection, line-length handling, or line numbering.
- Overlay popups managed by `overlay.Manager`. `Compose()` uses ANSI-aware compositing via `charmbracelet/x/ansi.Cut`. `HandleKey()` returns `Outcome` — Model switches on `OutcomeKind` for side effects (file jumps, theme apply/persist)
- **ANSI nesting with lipgloss**: `lipgloss.Render()` emits `\033[0m` (full reset) which breaks outer style backgrounds. For styled substrings inside a lipgloss container (status bar separators, search highlights, diff cursor, annotation lines), use raw ANSI sequences via the style sub-package (`style.AnsiFg`, `resolver.Color`), or dedicated `Renderer` methods. Never use `lipgloss.NewStyle().Render()` for inline elements within a lipgloss-rendered parent.
- **Background fill for themed panes**: lipgloss pane `Render()` and viewport internal padding emit plain spaces after reset, causing terminal default bg. Workarounds: (1) `extendLineBg()` pads lines to full width, (2) `padContentBg()` re-pads pane content, (3) `BorderBackground()` on border styles. **Ordering**: `extendLineBg()` must be called AFTER `applyHorizontalScroll()`.
- Horizontal scroll indicators (`«`/`»`): see `applyHorizontalScroll()` in `diffview.go`. `«` replaces first visible column when scrolled past hidden content. `»` extends 1 col into right padding. Bg split: `«` and separator space use line bg via `indicatorBg()`, `»` glyph uses `DiffBg`. Only in unwrapped mode.
- Status bar mode icons: `▼◉↩≋⊟#b±✓∅` rendered via `statusModeIcons()`. Graceful degradation drops segments on narrow terminals.
- Search and hunk navigation use `centerViewportOnCursor()`. Use `syncViewportToCursor()` only for j/k scrolling.
- Single-file mode (`m.singleFile`): one file → tree hidden, diff full width. Exception: markdown full-context gets TOC pane.
- Tree pane toggle (`t` key): `m.treeHidden` orthogonal to `singleFile`.
- Markdown TOC: activated when `singleFile && isMarkdownFile && isFullContext`. Uses `paneTree` slot.
- Annotation list popup (`@` key): cross-file jumps via `pendingAnnotJump` field with stale-jump guard.
- **Typed-nil trap**: `ParseTOC` factory MUST guard `if toc == nil { return nil }` to collapse typed-nil `*sidepane.TOC` into interface-nil.
