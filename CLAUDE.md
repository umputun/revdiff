# revdiff

Terminal UI diff viewer with inline annotations, built with bubbletea.

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
- `annotation/` - in-memory annotation store, structured output formatting
- `ui/mocks/` - moq-generated mocks (never edit manually)

## Key Interfaces (consumer-side, in `ui/`)
- `Renderer` - `ChangedFiles()`, `FileDiff()` - implemented by `diff.Git`, `diff.FallbackRenderer`, `diff.FileReader`
- `SyntaxHighlighter` - `HighlightLines()` - implemented by `highlight.Highlighter`

## Data Flow
```
git diff → diff.ParseUnifiedDiff() → []DiffLine
  (or: disk file → diff.readFileAsContext() → []DiffLine, all ChangeContext)
  → highlight.HighlightLines() → []string (ANSI foreground-only)
  → ui.renderDiff() dispatches:
    expanded (default): renderDiffLine() for each line
    collapsed (`v` toggle): renderCollapsedDiff() → skips removed lines,
      uses buildModifiedSet() to style adds as modify (amber ~) or pure add (green +)
      expanded hunks (`.` toggle) show all lines inline
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
- `no-ini:"true"` tag excludes fields from config file (used for --config, --dump-config, --version)
- `ini-name` tags ensure config keys match CLI long flag names

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
- `run()` resolves git repo root via `git rev-parse --show-toplevel`; if git is unavailable and `--only` is set, uses `FileReader` for standalone file review. Renderer selection is in `makeRenderer()`
- Help overlay uses `overlayCenter()` (ANSI-aware compositing via `charmbracelet/x/ansi.Cut`) to render on top of existing content; background (tree pane) remains visible at the edges
- **ANSI nesting with lipgloss**: `lipgloss.Render()` emits `\033[0m` (full reset) which breaks outer style backgrounds. For styled substrings inside a lipgloss container (status bar separators, search highlights), use raw ANSI sequences via `ansiColor(hex, code)` — code 38 for fg, 48 for bg. Never use `lipgloss.NewStyle().Render()` for inline elements within a lipgloss-rendered parent.
- Status bar mode icons (`▼ ◉ ↩ ≋ ⊟`) are always rendered on the right side via `statusModeIcons()`. `⊟` indicates tree/TOC pane hidden via `t` toggle. Active modes use `StatusFg`, inactive use `Muted` — both via raw ANSI fg sequences. Graceful degradation on narrow terminals drops left segments: search position first (`statusSegmentsNoSearch`), then hunk info (`statusSegmentsMinimal`), then truncates filename.
- Search and hunk navigation both use `centerViewportOnCursor()` to center the target in the middle of the viewport. Use `syncViewportToCursor()` only for cursor movements that should keep the cursor barely visible (j/k scrolling).
- Single-file mode (`m.singleFile`): when diff has exactly one file, tree pane is hidden, `treeWidth = 0`, diff gets full width (`m.width - 2` for borders, content width `m.width - 3`). Pane-switching keys (tab, h, l) and file navigation (n/p, f) become no-ops. Search nav (n/N) still works. Detection happens in `handleFilesLoaded`. Exception: when the file is markdown and full-context (all `ChangeContext` lines), an `mdTOC` pane replaces the tree pane with header navigation — see `ui/mdtoc.go`.
- Tree pane toggle (`t` key): `m.treeHidden` hides the tree/TOC pane and gives diff full width. Orthogonal to `singleFile` — sets `treeWidth = 0`, forces `focus = paneDiff`, blocks `togglePane()`/`handleSwitchToTree()`. `handleViewToggle()` dispatches `v`, `w`, and `t` keys. `handleFileLoaded` respects `treeHidden` when setting up mdTOC layout.
- Markdown TOC (`ui/mdtoc.go`): `mdTOC` component mirrors `fileTree` pattern (entries/cursor/offset/render). Activated in `handleFileLoaded` when `singleFile && isMarkdownFile && isFullContext`. Uses `paneTree` slot so `togglePane()` and key dispatch work unchanged. `handleTOCNav` routes j/k/pgdn/pgup/home/end to TOC cursor; Enter jumps to header line via `centerViewportOnCursor()`. `n/p` keys in diff pane jump to next/prev TOC entry via `jumpTOCEntry()`. `syncTOCActiveSection()` called on diff cursor movement to track current section. `syncTOCCursorToActive()` syncs cursor when switching back to TOC pane. `syncDiffToTOCCursor()` jumps diff viewport to current TOC cursor.
- Annotation list popup (`@` key): `ui/annotlist.go` — overlay listing all annotations across files. Cross-file jumps use `pendingAnnotJump` field: stores target annotation, triggers file load via `selectByPath`, then `handleFileLoaded` checks and positions cursor. Guard: `pendingAnnotJump.File == msg.file` prevents stale jumps.
