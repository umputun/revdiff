# revdiff Architecture

TUI for reviewing diffs, files, and documents with inline annotations, built with bubbletea.

## System Overview

```
┌─────────────────────────────────────────────────────┐
│  app/main.go — CLI parsing, config, wiring          │
├─────────────────────────────────────────────────────┤
│  app/ui/ — bubbletea TUI (single Model struct)      │
│    ├── overlay/   — popup layers (help, annots,     │
│    │                theme selector)                 │
│    ├── sidepane/  — file tree + markdown TOC        │
│    ├── style/     — color/ANSI resolution           │
│    └── worddiff/  — intra-line diff engine          │
├─────────────────────────────────────────────────────┤
│  app/diff/        — VCS detection + diff parsing    │
│  app/highlight/   — chroma syntax coloring          │
│  app/annotation/  — in-memory annotation store      │
│  app/keymap/      — configurable keybindings        │
│  app/theme/       — theme parse/load/dump           │
│  app/history/     — review session auto-save        │
│  app/fsutil/      — filesystem utilities            │
└─────────────────────────────────────────────────────┘
```

## Package Responsibilities

### app/ (entry point)

`main.go` is the composition root. Responsibilities:
- CLI flag parsing via `go-flags` with INI config file support
- VCS detection (`diff.DetectVCS()`) and renderer selection
- Theme resolution and application (`applyTheme()`)
- Keybinding loading (`keymap.LoadOrDefault()`)
- Constructing all dependencies and wiring them into `ui.ModelConfig`
- Starting bubbletea program

Key wiring pattern — all concrete types constructed here, injected into `ui.Model` through interfaces and factory closures:

```go
ModelConfig{
    Renderer:      diffRenderer,           // diff.Git, diff.Hg, etc.
    Highlighter:   highlighter,            // highlight.Highlighter
    StyleResolver: styleResolver,          // style.Resolver
    StyleRenderer: styleRenderer,          // style.Renderer
    SGR:           sgr,                    // style.SGR
    WordDiffer:    &worddiff.Differ{},
    Overlay:       overlay.NewManager(...),
    NewFileTree:   func(...) FileTreeComponent { ... },
    ParseTOC:      func(...) TOCComponent { ... },
    // ...flags, store, keymap
}
```

### app/diff/ — VCS and diff parsing

Handles all interaction with version control systems and diff parsing.

**VCS detection** (`vcs.go`): `DetectVCS()` walks up directory tree looking for `.git`/`.hg` markers, returns `VCSGit`, `VCSHg`, or `VCSNone`.

**Renderer implementations** — all implement the `ui.Renderer` interface (`ChangedFiles()` + `FileDiff()`):
- `Git` — runs `git diff`, parses unified diff output
- `Hg` — runs `hg diff --git`, parses unified diff output
- `FileReader` — reads standalone files as full-context (no VCS needed)
- `DirectoryReader` — lists all tracked files via `git ls-files` (for `--all-files` mode)
- `StdinReader` — reads from stdin as scratch buffer
- `FallbackRenderer` — wraps a primary renderer with fallback for files not in diff
- `ExcludeFilter` / `IncludeFilter` — decorators for prefix-based file filtering

**Diff parsing** (`parseUnifiedDiff`): converts unified diff output into `[]DiffLine`. Each `DiffLine` carries:
- `Content` — line text without `+`/`-` prefix (prefix re-added at render time)
- `Change` — `ChangeAdd`, `ChangeRemove`, `ChangeContext`, or `ChangeDivider`
- `OldNum` / `NewNum` — original and new line numbers (0 for non-applicable)

**Blame** (`blame.go`, `hgblame.go`): `Blamer` interface provides `FileBlame()` returning `map[int]BlameLine` keyed by new line number.

### app/ui/ — TUI package

Central package. Single `Model` struct implements bubbletea's `Model` interface. Methods split across files by concern to keep files under ~500 lines:

| File | Responsibility |
|------|---------------|
| `model.go` | Model struct, `NewModel`, `Init`, `Update`, `handleKey`, interfaces |
| `view.go` | `View()`, status bar rendering, ANSI helpers |
| `handlers.go` | Modal handlers (enter/esc, discard, filter, reviewed), help spec |
| `loaders.go` | Async file/blame loading, loaded-message handlers, data helpers |
| `diffview.go` | Diff line rendering, gutters, line styling, search highlights |
| `diffnav.go` | Cursor movement, hunk navigation, viewport sync, horizontal scroll |
| `collapsed.go` | Collapsed diff mode: hide removes, show modified markers |
| `annotate.go` | Annotation input lifecycle: start, save, cancel, delete |
| `annotlist.go` | Annotation list spec building, cross-file jump logic |
| `themeselect.go` | Theme selector operations: open, preview, confirm, apply |
| `search.go` | Search input handling, match computation, navigation |
| `configpatch.go` | Config file patching for persisting theme choice |

Each source file has a matching `_test.go`.

### app/ui/style/ — color and style resolution

Owns all hex-to-ANSI conversion, lipgloss style construction, SGR state tracking, HSL color math, and semantic color accessors.

Three main types:
- **`Resolver`** — static and runtime style/color lookups. Methods: `Color()`, `Style()`, `LineBg()`, `LineStyle()`, `WordDiffBg()`, `IndicatorBg()`
- **`Renderer`** — compound ANSI rendering for elements that need raw ANSI (not lipgloss). Methods: `AnnotationInline()`, `DiffCursor()`, `StatusBarSeparator()`, `FileStatusMark()`, `FileReviewedMark()`, `FileAnnotationMark()`
- **`SGR`** — ANSI SGR stream processor. `Reemit()` re-prepends active fg/bg/bold/italic state at continuation line starts (needed for wrap mode because `ansi.Wrap` doesn't preserve SGR across newlines)

### app/ui/sidepane/ — left-pane navigation

Two independent component types, both with cursor/offset management, rendering, and keyboard navigation:
- **`FileTree`** — file tree sidebar. Supports navigation (`Move`/`StepFile`), filtering (annotated-only), reviewed tracking, directory grouping
- **`TOC`** — markdown table-of-contents. Activated for single-file full-context markdown. Active section tracking, header-level navigation

Both constructed via factory closures in `main.go`, consumed through `FileTreeComponent`/`TOCComponent` interfaces.

### app/ui/overlay/ — popup layers

Layered popup system with mutual exclusivity (one overlay at a time).

- **`Manager`** — coordinator. Routes key events, manages open/close lifecycle, `Compose()` renders popup on top of background using ANSI-aware compositing (`overlayCenter()`)
- **`helpOverlay`** — two-column keybinding help popup
- **`annotListOverlay`** — scrollable annotation list with cross-file jump
- **`themeSelectOverlay`** — theme picker with fzf-style filter, live swatch preview

`Manager.HandleKey()` returns an `Outcome` — Model switches on `OutcomeKind` to perform side effects (file jumps, theme apply/persist). This keeps overlay package free of Model dependencies.

### app/ui/worddiff/ — intra-line diff engine

Single stateless type `Differ` grouping all word-diff algorithms:
- `PairLines()` — matches add/remove lines within hunks for comparison
- `ComputeIntraRanges()` — token-level LCS diff producing byte-offset ranges
- `InsertHighlightMarkers()` — ANSI-aware highlight insertion, shared by both word-diff and search highlighting

30% similarity gate discards ranges for dissimilar pairs. Ranges are byte offsets on tab-replaced content, aligning with `prepareLineContent` output.

### app/highlight/ — syntax highlighting

Chroma-based syntax highlighter. Produces foreground-only ANSI output (no backgrounds) so that diff line backgrounds from the style system are preserved. Highlighted lines pre-computed once per file load, stored parallel to `diffLines`.

### app/keymap/ — keybindings

~30 `Action` constants (e.g., `ActionDown`, `ActionQuit`). `Keymap` type maps key strings to actions. Loaded from file (`map <key> <action>` / `unmap <key>` format) or defaults.

Handlers use `m.keymap.Resolve(msg.String())` instead of raw key strings. Modal text-entry keys (annotation input, search input, confirm discard) stay hardcoded. Overlay key dispatch uses keymap actions for j/k/up/down but keeps `enter` and `esc` hardcoded.

Help overlay dynamically rendered from `m.keymap.HelpSections()`.

### app/theme/ — theme system

`Parse()` reads theme files (TOML-like format with hex validation), `Load()` from disk, `List()` available themes, `Dump()` to stdout, `InitBundled()` writes bundled themes to disk.

Bundled themes: revdiff, catppuccin-mocha, catppuccin-latte, dracula, gruvbox, nord, solarized-dark. Community themes live in `themes/gallery/`.

23 color keys mapped via `colorFieldPtrs()` in `main.go` — single source of truth for color key to struct field mapping.

### app/annotation/ — annotation store

In-memory store for annotations. Each `Annotation` has file, line, text, and optional `EndLine` for hunk range headers (triggered when comment contains "hunk" keyword). Structured output formatting for export.

### app/history/ — session auto-save

`Save(Params)` writes review session as markdown to `~/.config/revdiff/history/`. Includes header, annotations, and git diff for annotated files.

## Key Interfaces

All consumer-side — defined in `app/ui/model.go`, not in implementor packages. This is idiomatic Go: interfaces belong to the consumer.

| Interface | Methods | Implementors |
|-----------|---------|-------------|
| `Renderer` | `ChangedFiles()`, `FileDiff()` | `diff.Git`, `diff.Hg`, `diff.FileReader`, `diff.DirectoryReader`, `diff.StdinReader`, `diff.FallbackRenderer`, `diff.ExcludeFilter`, `diff.IncludeFilter` |
| `SyntaxHighlighter` | `HighlightLines()`, `SetStyle()`, `StyleName()` | `highlight.Highlighter` |
| `Blamer` | `FileBlame()` | `diff.GitBlamer`, `diff.HgBlamer` |
| `styleResolver` | `Color()`, `Style()`, `LineBg()`, `LineStyle()`, `WordDiffBg()`, `IndicatorBg()` | `style.Resolver` |
| `styleRenderer` | `AnnotationInline()`, `DiffCursor()`, `StatusBarSeparator()`, `FileStatusMark()`, `FileReviewedMark()`, `FileAnnotationMark()` | `style.Renderer` |
| `sgrProcessor` | `Reemit()` | `style.SGR` |
| `wordDiffer` | `ComputeIntraRanges()`, `PairLines()`, `InsertHighlightMarkers()` | `worddiff.Differ` |
| `FileTreeComponent` | 15 methods (navigation, query, mutation, render) | `sidepane.FileTree` |
| `TOCComponent` | 11 methods (navigation, cursor/section query+set, render) | `sidepane.TOC` |
| `overlayManager` | `Active()`, `Kind()`, `OpenHelp()`, `OpenAnnotList()`, `OpenThemeSelect()`, `Close()`, `HandleKey()`, `Compose()` | `overlay.Manager` |

## Data Flow

### Startup

```
main() → parseArgs() → config file + CLI flags + env vars
       → handleThemes() → theme resolution
       → run() → DetectVCS() → select Renderer
              → construct all dependencies
              → ui.NewModel(ModelConfig{...})
              → tea.NewProgram(model).Run()
```

### File Loading (async)

```
Model.Init() → loadFiles cmd
            → Renderer.ChangedFiles() → filesLoadedMsg
            → tree.Rebuild(entries)
            → auto-select first file → loadFileDiff cmd
            → Renderer.FileDiff() → fileLoadedMsg
            → highlight.HighlightLines() → highlightedLines
            → (optional) loadBlame cmd → blameLoadedMsg
```

### Rendering Pipeline

```
diffLines + highlightedLines
  → renderDiff() dispatches by mode:
    ├── expanded: renderDiffLine() per line
    │     → prepareLineContent() (tab replacement)
    │     → styleDiffContent() (syntax-highlighted or plain, with line bg)
    │     → applyIntraLineHighlight() (word-diff ranges, if active)
    │     → highlightSearchMatches() (search bg overlay, if active)
    │     → lineNumGutter() (if line numbers on)
    │     → blameGutter() (if blame on)
    │     → applyHorizontalScroll() (if not wrapped)
    │     → extendLineBg() (pad to full width)
    │     → wrapContent() + sgr.Reemit() (if wrapped)
    │
    └── collapsed: renderCollapsedDiff()
          → skip removed lines
          → buildModifiedSet() for modify vs pure-add styling

  → viewport.SetContent() → terminal
```

Each rendering feature (line numbers, blame, word-diff, search, wrap, collapsed) is orthogonal — can be independently toggled.

### Annotation Flow

```
User presses 'a' on diff line
  → annotating = true, annotateInput focused
  → Enter → store.Add(file, line, text)
  → re-render shows annotation inline below diff line
  → on quit: store.Format() → structured output to stdout/file
  → (optional) history.Save() → markdown to ~/.config/revdiff/history/
```

### Overlay Flow

```
User presses '?' / '@' / 'T'
  → Model calls overlay.OpenHelp/OpenAnnotList/OpenThemeSelect
  → overlay.Manager activates popup, blocks other overlays
  → key events route through Manager.HandleKey() → Outcome
  → Model switches on OutcomeKind:
      OutcomeJump → load target file, position cursor
      OutcomeApply → apply theme colors, update resolver
      OutcomePersist → write theme to config file
      OutcomeClose → close overlay, resume normal mode
  → Manager.Compose() renders popup over background content
```

## Configuration System

**Precedence**: CLI flags > env vars > config file > built-in defaults

- **Config file**: `~/.config/revdiff/config` (INI format via go-flags IniParser)
- **Theme files**: `~/.config/revdiff/themes/` (auto-created on first run)
- **Keybindings**: `~/.config/revdiff/keybindings` (`map`/`unmap` format)
- **History**: `~/.config/revdiff/history/` (auto-save dir)

Theme precedence: `--theme` overwrites all 23 color fields + chroma-style, ignoring `--color-*` flags or env vars. Applied via `applyTheme()` which directly overwrites `opts.Colors.*` fields after `parseArgs()`.

Adding a new color requires changes in three places: `theme.go` colorKeys + options struct + `colorFieldPtrs()`.

## Input Modes

Several mutually exclusive input sources, validated at parse time:

| Mode | Flag | Renderer | Notes |
|------|------|----------|-------|
| VCS diff (default) | `[base] [against]` | `Git` or `Hg` | Detects VCS, runs diff |
| Staged changes | `--staged` | `Git` or `Hg` | Cannot combine with refs |
| All tracked files | `--all-files` / `-A` | `DirectoryReader` | Git only, not with refs/staged/only |
| Single file(s) | `--only` / `-F` | `FileReader` | Not with include |
| Stdin | `--stdin` | `StdinReader` | Reads before TUI, reopens `/dev/tty` |

Filters stack: `--include` narrows first, then `--exclude` removes. Both wrap any renderer as decorators.

## Design Decisions

**Single Model struct with split files** — bubbletea's architecture centers on one Model. Splitting methods across files by concern keeps each file manageable while avoiding the complexity of multiple coordinating models.

**Consumer-side interfaces** — all interfaces defined in `app/ui/model.go`, not in implementor packages. Idiomatic Go pattern that keeps packages decoupled and makes the dependency direction explicit.

**Factory closures for sidepane components** — `NewFileTree` and `ParseTOC` are factory closures, not direct constructor calls, because they need runtime parameters from `main.go` that Model shouldn't know about.

**Raw ANSI instead of lipgloss for inline elements** — lipgloss `Render()` emits `\033[0m` (full reset) which breaks outer backgrounds. Elements rendered inside a lipgloss container (status bar separators, cursor markers, annotation text) use raw ANSI sequences via the style sub-package.

**Overlay Outcome pattern** — overlays return `Outcome` values instead of directly modifying Model state. Keeps overlay package independent from Model, makes side effects explicit and testable.

**Foreground-only syntax highlighting** — chroma output limited to foreground colors so diff line backgrounds (add/remove/modify) from the style system are preserved without conflict.

**Parallel data arrays** — `diffLines`, `highlightedLines`, and `intraRanges` are parallel arrays indexed by line position. Simple, cache-friendly, avoids complex data structures.

## Libraries

| Library | Purpose |
|---------|---------|
| `charmbracelet/bubbletea` | TUI framework (Elm architecture) |
| `charmbracelet/lipgloss` | Terminal styling |
| `charmbracelet/bubbles` | TUI components (viewport, textinput) |
| `jessevdk/go-flags` | CLI flag parsing with INI config support |
| `alecthomas/chroma/v2` | Syntax highlighting |
| `stretchr/testify` | Test assertions |
| `matryer/moq` | Mock generation |
