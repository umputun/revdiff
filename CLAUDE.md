# revdiff

TUI for reviewing diffs, files, and documents with inline annotations, built with bubbletea.

## Commands
- Build: `make build` (output: `.bin/revdiff`)
- Test: `make test` (race detector + coverage, excludes mocks)
- Lint: `make lint` or `golangci-lint run`
- Format: `make fmt` or `~/.claude/format.sh`
- Generate mocks: `go generate ./...`
- Vendor after adding deps: `go mod vendor`

## Project Structure
- `cmd/revdiff/` - entry point, CLI flags, wiring
- `diff/` - git interaction, unified diff parsing (`ParseUnifiedDiff`, `DiffLine`)
- `ui/` - bubbletea TUI model, views, styles, file tree, annotations
- `highlight/` - chroma-based syntax highlighting, foreground-only ANSI output
- `keymap/` - user-configurable keybindings (`Action` constants, `Keymap` type, parser, defaults, dump)
- `theme/` - color theme system: Parse (with hex validation), Load, List, Dump, InitBundled, BundledNames, ColorKeys (bundled: dracula, nord, solarized-dark)
- `annotation/` - in-memory annotation store, structured output formatting; `Annotation.EndLine` enables hunk range headers when comment contains "hunk" keyword
- `ui/mocks/` - moq-generated mocks (never edit manually)

## Key Interfaces (consumer-side, in `ui/`)
- `Renderer` - `ChangedFiles()`, `FileDiff()` - implemented by `diff.Git`, `diff.FallbackRenderer`, `diff.FileReader`, `diff.DirectoryReader`, `diff.StdinReader`, `diff.ExcludeFilter`
- `SyntaxHighlighter` - `HighlightLines()` - implemented by `highlight.Highlighter`

## Data Flow
```
git diff → diff.ParseUnifiedDiff() → []DiffLine
  (or: disk file → diff.readFileAsContext() → []DiffLine, all ChangeContext)
  (or: stdin / arbitrary reader → diff.readReaderAsContext() → []DiffLine, all ChangeContext)
  → highlight.HighlightLines() → []string (ANSI foreground-only)
  → ui.renderDiff() dispatches:
    expanded (default): renderDiffLine() for each line
    collapsed (`v` toggle): renderCollapsedDiff() → skips removed lines,
      uses buildModifiedSet() to style adds as modify (amber ~) or pure add (green +)
      expanded hunks (`.` toggle) show all lines inline
  when line numbers are on (`L` toggle, orthogonal to above):
    lineNumGutter(dl) formats " OOO NNN" gutter via m.styles.LineNumber,
    prepended in renderDiffLine, renderWrappedDiffLine, renderCollapsedAddLine, renderDeletePlaceholder
    lineNumWidth recomputed per file in handleFileLoaded; lineNumGutterWidth() = 2*W+2
  when blame gutter is on (`B` toggle, orthogonal to above):
    blameGutter(dl) formats " author age" gutter via m.styles.LineNumber,
    prepended after lineNumGutter in renderDiffLine, renderWrappedDiffLine, renderCollapsedAddLine, renderDeletePlaceholder
    blame data loaded async via loadBlame() → blameLoadedMsg; keyed by NewNum (blank for removed lines/dividers)
    blameAuthorLen capped at 8; blameGutterWidth() = W+5; Blamer interface (optional, nil when git unavailable)
  when wrap mode is on (`w` toggle, orthogonal to above):
    wrapContent() splits long lines via ansi.Wrap,
    continuation lines get `↪` gutter marker, cursorViewportY() sums wrapped line counts
  when search is active (`/` to search, `n`/`N` to navigate, `esc` to clear):
    buildSearchMatchSet() converts match indices to O(1) map per render,
    highlightSearchMatches() inserts ANSI bg-only sequence around matched substrings
    (preserves syntax foreground; falls back to reverse video in --no-colors mode)
  → viewport.SetContent() → terminal
```

## Libraries
- TUI: `bubbletea` + `lipgloss` + `bubbles`
- CLI flags: `jessevdk/go-flags`
- Syntax highlighting: `alecthomas/chroma/v2`
- Testing: `stretchr/testify`, mocks via `matryer/moq`

## Config
- Config file: `~/.config/revdiff/config` (INI format via go-flags built-in IniParser)
- Precedence: CLI flags > env vars > config file > built-in defaults
- `--dump-config` outputs current defaults, `--config` overrides path
- `no-ini:"true"` tag excludes fields from config file (used for --config, --dump-config, --dump-theme, --list-themes, --init-themes, --version)
- Themes dir: `~/.config/revdiff/themes/` with 5 bundled themes (catppuccin-mocha, dracula, gruvbox, nord, solarized-dark), auto-created on first run
- `--theme NAME` loads theme; `--dump-theme` exports resolved colors; `--list-themes` lists available; `--init-themes` re-creates bundled
- Theme precedence: `--theme` takes over completely — overwrites all 21 color fields + chroma-style, ignoring any `--color-*` flags or env vars. `--theme` + `--no-colors` prints warning and applies theme.
- Theme values applied via `applyTheme()` in `main.go` which directly overwrites `opts.Colors.*` fields after `parseArgs()`. `colorFieldPtrs(opts)` is the single source of truth for the color key → struct field mapping, used by both `applyTheme()` and `collectColors()` — adding a new color requires changes in `theme.go` colorKeys + options struct + `colorFieldPtrs()`
- `ini-name` tags ensure config keys match CLI long flag names
- Keybindings file: `~/.config/revdiff/keybindings` (`map <key> <action>` / `unmap <key>` format)
- `--keys` overrides keybindings path, `--dump-keys` prints effective bindings
- `keymap.Keymap` passed to `Model` via `ModelConfig.Keymap`; handlers switch on `m.keymap.Resolve(msg.String())` instead of raw key strings
- ~30 `Action` constants in `keymap/keymap.go` (e.g., `ActionDown`, `ActionQuit`); modal text-entry keys (annotation input, search input, confirm discard) stay hardcoded; modal overlay navigation (annotation list, help) uses keymap for j/k/up/down but keeps `enter` and `esc` hardcoded
- Help overlay is dynamically rendered from `m.keymap.HelpSections()`

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

## Gotchas
- Project uses vendoring - run `go mod vendor` after adding/updating dependencies
- Chroma API uses British spelling (`Colour`), suppress with `//nolint:misspell`
- Syntax highlighting uses specific ANSI resets (`\033[39m`, `\033[22m`, `\033[23m`) instead of full reset (`\033[0m`) to preserve lipgloss backgrounds
- Highlighted lines are pre-computed once per file load, stored parallel to `diffLines`
- `DiffLine.Content` has no `+`/`-` prefix - prefix is re-added at render time
- Tab replacement happens at render time in `renderDiffLine`, not in diff parsing
- `run()` resolves git repo root via `git rev-parse --show-toplevel`; if git is unavailable and `--only` is set, uses `FileReader` for standalone file review. `--stdin` skips git lookup entirely, validates non-TTY stdin, reads payload before starting Bubble Tea, and reopens `/dev/tty` for interactive key input.
- `--all-files` mode uses `DirectoryReader` (git ls-files) to list all tracked files; `--exclude` wraps any renderer with `ExcludeFilter` for prefix-based filtering. `--all-files` is mutually exclusive with refs, `--staged`, and `--only`. `--stdin` is mutually exclusive with refs, `--staged`, `--only`, `--all-files`, and `--exclude`.
- `diff.readReaderAsContext()` is the shared parser for file-backed and stdin-backed context-only views. Preserve its behavior if you change binary detection, line-length handling, or line numbering.
- Help overlay uses `overlayCenter()` (ANSI-aware compositing via `charmbracelet/x/ansi.Cut`) to render on top of existing content; background (tree pane) remains visible at the edges
- **ANSI nesting with lipgloss**: `lipgloss.Render()` emits `\033[0m` (full reset) which breaks outer style backgrounds. For styled substrings inside a lipgloss container (status bar separators, search highlights), use raw ANSI sequences via `ansiColor(hex, code)` — code 38 for fg, 48 for bg. Never use `lipgloss.NewStyle().Render()` for inline elements within a lipgloss-rendered parent.
- **Background fill for themed panes**: lipgloss pane `Render()` and viewport internal padding emit plain spaces after `\033[0m` reset, causing pane background to show terminal default. Three workarounds: (1) `extendLineBg()` pads individual add/remove/modify lines to full content width with their specific bg color; (2) `padContentBg()` strips viewport trailing spaces and re-pads every line of pane content with DiffBg/TreeBg; (3) `BorderBackground()` is set on pane border styles to match pane bg. Context and line-number styles also set DiffBg explicitly via `contextStyle()`/`lineNumberStyle()`/`contextHighlightStyle()`.
- Status bar mode icons (`▼ ◉ ↩ ≋ ⊟ # @`) are always rendered on the right side via `statusModeIcons()`. `@` indicates blame gutter active via `B` toggle. `⊟` indicates tree/TOC pane hidden via `t` toggle. Active modes use `StatusFg`, inactive use `Muted` — both via raw ANSI fg sequences. Graceful degradation on narrow terminals drops left segments: search position first (`statusSegmentsNoSearch`), then line number and hunk info (`statusSegmentsMinimal`), then truncates filename.
- Search and hunk navigation both use `centerViewportOnCursor()` to center the target in the middle of the viewport. Use `syncViewportToCursor()` only for cursor movements that should keep the cursor barely visible (j/k scrolling).
- Single-file mode (`m.singleFile`): when diff has exactly one file, tree pane is hidden, `treeWidth = 0`, diff gets full width (`m.width - 2` for borders, content width `m.width - 4` including right padding). Pane-switching keys (tab, h, l) and file navigation (n/p, f) become no-ops. Search nav (n/N) still works. Detection happens in `handleFilesLoaded`. Exception: when the file is markdown and full-context (all `ChangeContext` lines), an `mdTOC` pane replaces the tree pane with header navigation — see `ui/mdtoc.go`.
- Tree pane toggle (`t` key): `m.treeHidden` hides the tree/TOC pane and gives diff full width. Orthogonal to `singleFile` — sets `treeWidth = 0`, forces `focus = paneDiff`, blocks `togglePane()`/`handleSwitchToTree()`. `handleViewToggle()` dispatches `v`, `w`, `t`, and `L` keys. `handleFileLoaded` respects `treeHidden` when setting up mdTOC layout.
- Markdown TOC (`ui/mdtoc.go`): `mdTOC` component mirrors `fileTree` pattern (entries/cursor/offset/render). Activated in `handleFileLoaded` when `singleFile && isMarkdownFile && isFullContext`. Uses `paneTree` slot so `togglePane()` and key dispatch work unchanged. `handleTOCNav` routes j/k/pgdn/pgup/home/end to TOC cursor; Enter jumps to header line via `centerViewportOnCursor()`. `n/p` keys in diff pane jump to next/prev TOC entry via `jumpTOCEntry()`. `syncTOCActiveSection()` called on diff cursor movement to track current section. `syncTOCCursorToActive()` syncs cursor when switching back to TOC pane. `syncDiffToTOCCursor()` jumps diff viewport to current TOC cursor.
- Annotation list popup (`@` key): `ui/annotlist.go` — overlay listing all annotations across files. Navigation keys (j/k/up/down) routed through `m.keymap.Resolve()`, `enter` and `esc` hardcoded (modal overlay convention). Cross-file jumps use `pendingAnnotJump` field: stores target annotation, triggers file load via `selectByPath`, then `handleFileLoaded` checks and positions cursor. Guard: `pendingAnnotJump.File == msg.file` prevents stale jumps.
