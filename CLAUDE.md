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
- `app/` - entry point (`main.go`), CLI flags, wiring
- `app/diff/` - VCS interaction (git + hg), unified diff parsing (`parseUnifiedDiff`, `DiffLine`), VCS detection (`vcs.go`), Mercurial support (`hg.go`, `hgblame.go`)
- `app/ui/` - bubbletea TUI package. All files share one `Model` struct — methods are split across files by concern to keep code files under ~500 lines and test files around ~1000 lines (soft target). Each source file has a matching `_test.go` file. See `app/ui/doc.go` for package-level documentation.
  - `model.go` - Model struct, NewModel, Init, Update, handleKey, view toggles
  - `view.go` - View(), status bar, ANSI helpers
  - `handlers.go` - modal handlers (help overlay, enter/esc, discard, filter, mark reviewed)
  - `loaders.go` - async file/blame loading, loaded-message handlers, data helpers
  - `diffview.go` - diff rendering, gutters, line styling, search highlights
  - `diffnav.go` - nav dispatchers, cursor movement, hunk nav, viewport sync
  - `collapsed.go` - collapsed diff mode logic and rendering
  - `annotate.go` - annotation input/CRUD
  - `annotlist.go` - annotation list overlay
  - `search.go` - search input and navigation
  - `model.go` also holds consumer-side interfaces (styleResolver, styleRenderer, sgrProcessor, wordDiffer) with compile-time assertions, and exported `FileTreeComponent`/`TOCComponent` interfaces for sidepane components
- `app/ui/style/` - color and style resolution sub-package. Owns all hex-to-ANSI conversion, lipgloss style construction, SGR state tracking, HSL color math, and semantic color accessors. Three main types:
  - `style.Resolver` - static and runtime style/color lookups (Color, Style, LineBg, LineStyle, WordDiffBg, IndicatorBg)
  - `style.Renderer` - compound ANSI rendering (AnnotationInline, DiffCursor, StatusBarSeparator, FileStatusMark, FileReviewedMark, FileAnnotationMark)
  - `style.SGR` - ANSI SGR stream processing (Reemit for wrap-mode continuation lines)
- `app/ui/sidepane/` - left-pane navigation sub-package. Owns file tree (`FileTree`) and markdown table-of-contents (`TOC`) types with cursor/offset management, entry parsing, and rendering. Concrete construction lives in `app/main.go` via factory closures injected into `ModelConfig.NewFileTree` and `ModelConfig.ParseTOC`. Two types:
  - `sidepane.FileTree` - file tree sidebar with navigation (`Move`/`StepFile`), filtering, reviewed tracking, and rendering
  - `sidepane.TOC` - markdown TOC with navigation, active section tracking, and rendering
- `app/ui/worddiff/` - intra-line word-diff algorithms and shared highlight marker insertion engine. Owns tokenizer, LCS algorithm, line pairing, similarity gate, and ANSI-aware highlight insertion used by both word-diff and search highlighting. Single type:
  - `worddiff.Differ` - stateless type grouping all word-diff methods (`ComputeIntraRanges`, `PairLines`, `InsertHighlightMarkers`). Injected into Model via `ModelConfig.WordDiffer` wired in `app/main.go`
- `app/highlight/` - chroma-based syntax highlighting, foreground-only ANSI output
- `app/keymap/` - user-configurable keybindings (`Action` constants, `Keymap` type, parser, defaults, dump)
- `app/theme/` - color theme system: Parse (with hex validation), Load, List, Dump, InitBundled, ColorKeys (bundled: revdiff, catppuccin-mocha, catppuccin-latte, dracula, gruvbox, nord, solarized-dark)
- `app/annotation/` - in-memory annotation store, structured output formatting; `Annotation.EndLine` enables hunk range headers when comment contains "hunk" keyword
- `app/history/` - review session auto-save to `~/.config/revdiff/history/`; `Save(Params)` writes markdown with header, annotations, and git diff for annotated files
- `app/ui/mocks/` - moq-generated mocks (never edit manually)

## Key Interfaces (consumer-side, in `app/ui/`)
- `Renderer` - `ChangedFiles()`, `FileDiff()` - implemented by `diff.Git`, `diff.Hg`, `diff.FallbackRenderer`, `diff.FileReader`, `diff.DirectoryReader`, `diff.StdinReader`, `diff.ExcludeFilter`
- `SyntaxHighlighter` - `HighlightLines()` - implemented by `highlight.Highlighter`
- `styleResolver` - `Color()`, `Style()`, `LineBg()`, `LineStyle()`, `WordDiffBg()`, `IndicatorBg()` - implemented by `style.Resolver`
- `styleRenderer` - `AnnotationInline()`, `DiffCursor()`, `StatusBarSeparator()`, `FileStatusMark()`, `FileReviewedMark()`, `FileAnnotationMark()` - implemented by `style.Renderer`
- `sgrProcessor` - `Reemit()` - implemented by `style.SGR`
- `wordDiffer` - `ComputeIntraRanges()`, `PairLines()`, `InsertHighlightMarkers()` - implemented by `*worddiff.Differ`. Injected via `ModelConfig.WordDiffer` wired in `app/main.go`
- `FileTreeComponent` - exported, 15 methods (navigation, query, mutation, render) - implemented by `*sidepane.FileTree`. Concrete construction injected via `ModelConfig.NewFileTree` factory closure wired in `app/main.go`
- `TOCComponent` - exported, 11 methods (navigation, cursor/section query+set, render) - implemented by `*sidepane.TOC`. Concrete construction injected via `ModelConfig.ParseTOC` factory closure wired in `app/main.go`

## Data Flow
```
DetectVCS() → VCSGit | VCSHg | VCSNone
  git diff / hg diff --git → diff.parseUnifiedDiff() → []DiffLine
  (or: disk file → diff.readFileAsContext() → []DiffLine, all ChangeContext)
  (or: stdin / arbitrary reader → diff.readReaderAsContext() → []DiffLine, all ChangeContext)
  → highlight.HighlightLines() → []string (ANSI foreground-only)
  → when m.wordDiff is true (opt-in via --word-diff flag or `W` toggle):
    recomputeIntraRanges() pairs add/remove lines within hunks via m.differ.PairLines(),
    runs token-level LCS diff (m.differ.ComputeIntraRanges()) on each pair,
    stores [][]worddiff.Range parallel to diffLines. 30% similarity gate discards
    ranges for dissimilar pairs. All algorithms live in app/ui/worddiff/ sub-package.
    Ranges are byte offsets on tab-replaced content, aligning with prepareLineContent output.
    When wordDiff is off, m.intraRanges stays nil and applyIntraLineHighlight is a no-op.
  → ui.renderDiff() dispatches:
    expanded (default): renderDiffLine() for each line
      intra-line word-diff: when intraRanges[idx] is non-nil, applyIntraLineHighlight()
      adds WordAddBg/WordRemoveBg ANSI bg markers around changed spans (reverse-video in no-color mode)
    collapsed (`v` toggle): renderCollapsedDiff() → skips removed lines,
      uses buildModifiedSet() to style adds as modify (amber ~) or pure add (green +)
      expanded hunks (`.` toggle) show all lines inline
  when line numbers are on (`L` toggle, orthogonal to above):
    lineNumGutter(dl) formats gutter via m.resolver.Style(StyleKeyLineNumber):
      two-column (diff files): " OOO NNN", lineNumGutterWidth() = 2*W+2
      single-column (full-context files): " NNN", lineNumGutterWidth() = W+1
    singleColLineNum detected per-file in handleFileLoaded via isFullContext()
    prepended in renderDiffLine, renderWrappedDiffLine, renderCollapsedAddLine, renderDeletePlaceholder
    lineNumWidth recomputed per file in handleFileLoaded
  when blame gutter is on (`B` toggle, orthogonal to above):
    blameGutter(dl, now) formats " author age" gutter via m.resolver.Style(StyleKeyLineNumber),
    prepended after lineNumGutter in renderDiffLine, renderWrappedDiffLine, renderCollapsedAddLine, renderDeletePlaceholder
    blame data loaded async via loadBlame() → blameLoadedMsg; keyed by NewNum (blank for removed lines/dividers)
    blameAuthorLen capped at 8; blameGutterWidth() = W+5; Blamer interface (optional, nil when no VCS available)
  when wrap mode is on (`w` toggle, orthogonal to above):
    wrapContent() splits long lines via ansi.Wrap,
    continuation lines get `↪` gutter marker, cursorViewportY() sums wrapped line counts.
    ansi.Wrap does not preserve SGR state across inserted newlines, so m.sgr.Reemit()
    re-prepends active fg color, bold, italic, and bg at the start of each continuation line.
    State tracking via style.SGR internal methods; handles chroma's fg (24-bit/basic),
    bold (1/22), italic (3/23), bg (48;2;r;g;b/49), fg reset (39), and full reset (0/bare)
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
- Themes dir: `~/.config/revdiff/themes/` with 7 bundled themes (revdiff, catppuccin-mocha, catppuccin-latte, dracula, gruvbox, nord, solarized-dark), auto-created on first run
- `--theme NAME` loads theme; `--dump-theme` exports resolved colors; `--list-themes` lists available; `--init-themes` re-creates bundled
- Theme precedence: `--theme` takes over completely — overwrites all 23 color fields + chroma-style, ignoring any `--color-*` flags or env vars. `--theme` + `--no-colors` prints warning and applies theme.
- Theme values applied via `applyTheme()` in `main.go` which directly overwrites `opts.Colors.*` fields after `parseArgs()`. `colorFieldPtrs(opts)` is the single source of truth for the color key → struct field mapping, used by both `applyTheme()` and `collectColors()` — adding a new color requires changes in `theme.go` colorKeys + options struct + `colorFieldPtrs()`
- `ini-name` tags ensure config keys match CLI long flag names
- Keybindings file: `~/.config/revdiff/keybindings` (`map <key> <action>` / `unmap <key>` format)
- `--keys` overrides keybindings path, `--dump-keys` prints effective bindings
- `keymap.Keymap` passed to `Model` via `ModelConfig.Keymap`; handlers switch on `m.keymap.Resolve(msg.String())` instead of raw key strings
- ~30 `Action` constants in `app/keymap/keymap.go` (e.g., `ActionDown`, `ActionQuit`); modal text-entry keys (annotation input, search input, confirm discard) stay hardcoded; modal overlay navigation (annotation list, help) uses keymap for j/k/up/down but keeps `enter` and `esc` hardcoded
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
- `--all-files` mode uses `DirectoryReader` (git ls-files) to list all tracked files; `--exclude` wraps any renderer with `ExcludeFilter` for prefix-based filtering. `--all-files` is mutually exclusive with refs, `--staged`, and `--only`. `--stdin` is mutually exclusive with refs, `--staged`, `--only`, `--all-files`, and `--exclude`.
- `diff.readReaderAsContext()` is the shared parser for file-backed and stdin-backed context-only views. Preserve its behavior if you change binary detection, line-length handling, or line numbering.
- Help overlay uses `overlayCenter()` (ANSI-aware compositing via `charmbracelet/x/ansi.Cut`) to render on top of existing content; background (tree pane) remains visible at the edges
- **ANSI nesting with lipgloss**: `lipgloss.Render()` emits `\033[0m` (full reset) which breaks outer style backgrounds. For styled substrings inside a lipgloss container (status bar separators, search highlights, diff cursor, annotation lines), use raw ANSI sequences via the style sub-package (`style.AnsiFg`, `resolver.Color`), or dedicated `Renderer` methods (`Renderer.DiffCursor`, `Renderer.AnnotationInline`). Never use `lipgloss.NewStyle().Render()` for inline elements within a lipgloss-rendered parent.
- **Background fill for themed panes**: lipgloss pane `Render()` and viewport internal padding emit plain spaces after `\033[0m` reset, causing pane background to show terminal default. Three workarounds: (1) `extendLineBg()` pads individual add/remove/modify lines to full content width with their specific bg color; (2) `padContentBg()` strips viewport trailing spaces and re-pads every line of pane content with DiffBg/TreeBg; (3) `BorderBackground()` is set on pane border styles to match pane bg. Context and line-number styles also set DiffBg explicitly via internal `Colors` methods in the style sub-package. **Ordering constraint**: `extendLineBg()` must be called AFTER `applyHorizontalScroll()` — extending before scroll causes bg fill to be computed for the wrong visible width. `styleDiffContent()` does NOT call `extendLineBg` internally; callers handle it via `resolver.LineBg()`.
- Horizontal scroll overflow indicators (`«` / `»`): `applyHorizontalScroll()` replaces the first visible column with `«` when scrolled right past hidden content and extends 1 col beyond `cutWidth` into the pane's right padding column to place `»` flush against the pane border. The right indicator always includes a leading space separator so the glyph doesn't touch the last content character. Because the right-overflow path outputs width `cutWidth + 1`, `extendLineBg()` becomes a no-op for these lines (current > target). Non-overflow lines keep the design-intent 1-col right padding via `padContentBg()`. Background resolution is split between the two cells: the `«` glyph and the right indicator's leading separator space both use the line bg resolved by `indicatorBg(lineBg)` (line bg for add/remove/modify, `DiffBg` for context/divider) so the colored content area extends naturally; the `»` glyph itself is always drawn on `DiffBg` regardless of line bg so it reads as pane chrome, not as part of the line. This split is handled inside `rightScrollIndicator()` via `scrollIndicatorANSI(glyph, spaceBg, glyphBg, leadingSpace)`. Fg is always `Muted`; no-colors mode falls back to reverse video. Only active in unwrapped mode — `renderWrappedDiffLine()` and wrapped-collapsed paths never call `applyHorizontalScroll()`.
- Status bar mode icons (`▼ ◉ ↩ ≋ ⊟ # b ± ✓ ∅`) are always rendered on the right side via `statusModeIcons()`. `▼` collapsed mode (`v`), `◉` filter (`f`), `↩` wrap (`w`), `≋` search active, `⊟` tree/TOC pane hidden (`t`), `#` line numbers (`L`), `b` blame gutter (`B`), `±` intra-line word-diff (`W`), `✓` reviewed count, `∅` untracked visible (`u`). Active modes use `StatusFg`, inactive use `Muted` — both via raw ANSI fg sequences. Graceful degradation on narrow terminals drops left segments: search position first (`statusSegmentsNoSearch`), then line number and hunk info (`statusSegmentsMinimal`), then truncates filename.
- Search and hunk navigation both use `centerViewportOnCursor()` to center the target in the middle of the viewport. Use `syncViewportToCursor()` only for cursor movements that should keep the cursor barely visible (j/k scrolling).
- Single-file mode (`m.singleFile`): when diff has exactly one file, tree pane is hidden, `treeWidth = 0`, diff gets full width (`m.width - 2` for borders, content width `m.width - 4` including right padding). Pane-switching keys (tab, h, l) and file navigation (n/p, f) become no-ops. Search nav (n/N) still works. Detection happens in `handleFilesLoaded`. Exception: when the file is markdown and full-context (all `ChangeContext` lines), a TOC pane replaces the tree pane with header navigation — see `app/ui/sidepane/toc.go`.
- Tree pane toggle (`t` key): `m.treeHidden` hides the tree/TOC pane and gives diff full width. Orthogonal to `singleFile` — sets `treeWidth = 0`, forces `focus = paneDiff`, blocks `togglePane()`/`handleSwitchToTree()`. `handleViewToggle()` dispatches `v`, `w`, `t`, and `L` keys. `handleFileLoaded` respects `treeHidden` when setting up mdTOC layout.
- Markdown TOC (`app/ui/sidepane/toc.go`): `TOC` component lives in the sidepane sub-package. Activated in `handleFileLoaded` when `singleFile && isMarkdownFile && isFullContext`. Uses `paneTree` slot so `togglePane()` and key dispatch work unchanged. `handleTOCNav` routes j/k/pgdn/pgup/home/end to TOC cursor; Enter jumps to header line via `centerViewportOnCursor()`. `n/p` keys in diff pane jump to next/prev TOC entry via `jumpTOCEntry()`. `syncTOCActiveSection()` called on diff cursor movement to track current section. `syncTOCCursorToActive()` syncs cursor when switching back to TOC pane. `syncDiffToTOCCursor()` jumps diff viewport to current TOC cursor.
- Annotation list popup (`@` key): `app/ui/annotlist.go` — overlay listing all annotations across files. Navigation keys (j/k/up/down) routed through `m.keymap.Resolve()`, `enter` and `esc` hardcoded (modal overlay convention). Cross-file jumps use `pendingAnnotJump` field: stores target annotation, triggers file load via `selectByPath`, then `handleFileLoaded` checks and positions cursor. Guard: `pendingAnnotJump.File == msg.file` prevents stale jumps.
- Typed-nil trap for `ParseTOC` factory: the `ParseTOC` factory closure in `app/main.go` MUST guard `if toc == nil { return nil }` to collapse typed-nil `*sidepane.TOC` into a truly nil interface. Without this guard, returning a typed-nil `*TOC` from a closure with return type `ui.TOCComponent` creates a non-nil interface (carries type info + nil pointer), causing `m.mdTOC != nil` checks inside Model to misbehave.
