# revdiff Architecture

TUI for reviewing diffs, files, and documents with inline annotations, built with bubbletea.

## System Overview

```
┌─────────────────────────────────────────────────────┐
│  app/ — composition root (package main)             │
│    main.go          — main(), early-exit flow       │
│    config.go        — options, parseArgs, config IO │
│    stdin.go         — stdin validation, /dev/tty    │
│    renderer_setup.go — VCS detection, renderer pick │
│    themes.go        — theme CLI commands, wiring    │
│    history_save.go  — history-save policy           │
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
│  app/editor/      — external $EDITOR invocation     │
│  app/keymap/      — configurable keybindings        │
│  app/theme/       — Catalog-centric theme system    │
│  app/history/     — review session auto-save        │
│  app/fsutil/      — filesystem utilities            │
└─────────────────────────────────────────────────────┘
```

## Package Responsibilities

### app/ (composition root)

`package main` is the composition root, split across files by concern:

| File | Responsibility |
|------|---------------|
| `main.go` | `main()`, early-exit commands (version, dump-config, dump-keys), `run()` orchestration |
| `config.go` | `options` struct, `parseArgs`, `dumpConfig`, `loadConfigFile`, config-path helpers |
| `stdin.go` | stdin validation, `/dev/tty` reopen, stdin renderer prep |
| `renderer_setup.go` | `DetectVCS` wiring, `setupVCSRenderer` (git/hg/jj/no-VCS/all-files) |
| `themes.go` | theme CLI commands (`--init-themes`, `--install-theme`, `--list-themes`, `--theme`), `applyTheme()`, `themeCatalog` adapter (composes `theme.Catalog` + config persistence for `ui.ThemeCatalog` interface) |
| `history_save.go` | `histReq` struct and `saveHistory` |

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
    Themes:        themes,                 // themeCatalog adapter
    NewFileTree:   func(...) FileTreeComponent { ... },
    ParseTOC:      func(...) TOCComponent { ... },
    // ...flags, store, keymap
}
```

### app/diff/ — VCS and diff parsing

Handles all interaction with version control systems and diff parsing.

**VCS detection** (`vcs.go`): `DetectVCS()` walks up directory tree looking for `.jj`/`.git`/`.hg` markers, returns `VCSJJ`, `VCSGit`, `VCSHg`, or `VCSNone`. `.jj` is checked before `.git` so colocated jj+git repositories resolve as jj (reads go through the jj working-copy model instead of bypassing it via git).

**Renderer implementations** — all implement the `ui.Renderer` interface (`ChangedFiles()` + `FileDiff()`):
- `Git` — runs `git diff`, parses unified diff output
- `Hg` — runs `hg diff --git`, parses unified diff output
- `Jj` — runs `jj diff --git`, parses unified diff output; git-style refs (HEAD, HEAD~N, A..B) translate to jj revsets via `--from`/`--to`. jj emits raw bytes for binary files, so `(*Jj).synthesizeBinaryDiff` rewrites such diffs with the git-style "Binary files … differ" marker so `parseUnifiedDiff` produces a binary placeholder.

**CommitLogger capability** (`CommitLog(ref string) ([]CommitInfo, error)`) — an additive capability interface implemented by `Git`/`Hg`/`Jj` and consumed by the `i` info overlay. Separate from the base `Renderer` so non-VCS renderers (`FileReader`, `DirectoryReader`, `StdinReader`) stay unaffected. Each VCS translates the pre-combined ref string to its own log syntax (`X..HEAD` for git, `X::.` for hg, `X..@` for jj), caps results at 500 commits, and strips raw `\x1b` bytes from subject/body at parse time so the overlay can render without re-scanning for ANSI injection. Hg uses ASCII US/RS separators (`\x1f`/`\x1e`) because literal NUL is invalid in argv; git and jj use NUL/SOH via stdout.
- `FileReader` — reads standalone files as full-context (no VCS needed)
- `DirectoryReader` — lists all tracked files via a pluggable lister (`git ls-files` by default; `NewJjDirectoryReader` uses `jj file list`) for `--all-files` mode
- `StdinReader` — reads from stdin as scratch buffer
- `FallbackRenderer` — wraps a primary renderer with fallback for files not in diff
- `ExcludeFilter` / `IncludeFilter` — decorators for prefix-based file filtering

**Diff parsing** (`parseUnifiedDiff`): converts unified diff output into `[]DiffLine`. Each `DiffLine` carries:
- `Content` — line text without `+`/`-` prefix (prefix re-added at render time)
- `Change` — `ChangeAdd`, `ChangeRemove`, `ChangeContext`, or `ChangeDivider`
- `OldNum` / `NewNum` — original and new line numbers (0 for non-applicable)

**Blame** (`blame.go`, `hgblame.go`, `jjblame.go`): `Blamer` interface provides `FileBlame()` returning `map[int]BlameLine` keyed by new line number. jj blame uses `jj file annotate -T <template>` with a tab-separated template.

### app/ui/ — TUI package

Central package. Single `Model` struct implements bubbletea's `Model` interface. Methods split across files by concern to keep files under ~500 lines:

| File | Responsibility |
|------|---------------|
| `model.go` | Model struct, sub-state structs, `NewModel`, `Init`, `Update`, `handleKey`, interfaces |
| `view.go` | `View()`, status bar rendering, ANSI helpers |
| `handlers.go` | Modal handlers (enter/esc, discard, filter, reviewed), help spec |
| `loaders.go` | Async file/blame loading, loaded-message handlers, data helpers |
| `diffview.go` | Diff line rendering, gutters, line styling, search highlights |
| `diffnav.go` | Cursor movement, hunk navigation, viewport sync, horizontal scroll |
| `scrollbar.go` | Vertical scrollbar thumb post-processing on rendered diff/tree/TOC panes (replaces right-border `│` with `┃` on rows mapped to the visible viewport portion) |
| `collapsed.go` | Collapsed diff mode: hide removes, show modified markers |
| `annotate.go` | Annotation input lifecycle (start, save, cancel, delete) and the visual-row chokepoint: `annotationVisualRows` is the single source of truth for "how many rows + what content does this annotation paint as." Memoized on `annot.rowCache`, invalidated by `handleFileLoaded`, `applyTheme`, and `cancelThemeSelect` |
| `annotlist.go` | Annotation list spec building, cross-file jump logic (`jumpToAnnotationTarget` for the `@` popup, `tryJumpToAnnotationTarget` returning a jumped-bool for the `}`/`{` walker) |
| `annotnav.go` | Cross-file annotation navigation (`}` / `{`): builds the flat annotation list, computes adjacent target via exact-match or insertion-point fallback, retries through non-jumpable targets so a hidden annotation cannot trap the walker |
| `editor.go` | `$EDITOR` handoff for multi-line annotations: `openEditor()` wraps `app/editor.Editor` in `tea.ExecProcess`, `editorFinishedMsg` dispatch, `handleEditorFinished` routing (save / cancel / error-preserve) |
| `themeselect.go` | Theme selector operations: open, preview, confirm, apply (via injected `ThemeCatalog`) |
| `search.go` | Search input handling, match computation, navigation |
| `mouse.go` | Mouse event routing: `handleMouse` dispatch, `hitTest` pane classification (`hitZone`), wheel/left-click helpers (`clickTree`, `clickDiff`), layout helpers (`statusBarHeight`, `diffTopRow`, `treeTopRow`). Diff-pane wheel events defer both the cursor pin and the `SetContent(renderDiff())` call via a single in-flight `tea.Tick(wheelRenderDelay)` debounce (issue #179) — `wheelState.tickInFlight` ensures one tick at a time across an entire burst (subsequent wheels just bump `gen`); stale ticks reschedule, matching ticks flush. `flushWheelPending()` is called from `handleWheelDebounce`, `handleKey`, `handleResize`, and `handleBlameLoaded` (any path that runs `syncViewportToCursor` or reads `m.nav.diffCursor` must flush first). Mouse tracking is enabled program-wide via `tea.WithMouseCellMotion()` in `app/main.go` unless `--no-mouse` / `REVDIFF_NO_MOUSE` is set |

Each source file has a matching `_test.go`.

**Model state grouping** — `Model` fields are organized into explicit sub-structs by concern:

| Sub-struct | Purpose | Key fields |
|------------|---------|------------|
| `modelConfigState` (`m.cfg`) | immutable session config | `ref`, `staged`, `only`, `noColors`, `tabSpaces`, etc. |
| `layoutState` (`m.layout`) | viewport and pane geometry | `viewport`, `focus`, `treeHidden`, `width`, `height`, `scrollX` |
| `loadedFileState` (`m.file`) | current file's loaded state | `lines`, `highlighted`, `intraRanges`, `blameData`, `mdTOC`, `singleFile` |
| `modeState` (`m.modes`) | user-togglable view modes | `wrap`, `collapsed`, `compact`, `compactContext`, `lineNumbers`, `wordDiff`, `showBlame` |
| `navigationState` (`m.nav`) | cursor position | `diffCursor`, `pendingHunkJump` |
| `searchState` (`m.search`) | search lifecycle | `active`, `term`, `matches`, `cursor`, `input`, `matchSet`, `history`, `historyIdx` |
| `annotationState` (`m.annot`) | annotation input lifecycle and visual-row cache | `annotating`, `fileAnnotating`, `cursorOnAnnotation`, `input`, `rowCache` |
| `wheelState` (`m.wheel`) | diff-pane wheel coalescing (issue #179) | `gen`, `renderPending`, `tickInFlight` |

Methods remain on `Model` — the sub-structs group mutable state for clarity, not to create mini-models.

**Theme boundary** — `app/ui` does not import `app/theme` or `app/fsutil`. Theme discovery and persistence are accessed through the `ThemeCatalog` interface (defined in `model.go`), with a concrete adapter wired in `app/themes.go`.

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
- **`infoOverlay`** (`info.go`) — unified info popup (description + session details + commit log). Description prose comes from `--description` / `--description-file`, sanitized to strip ANSI/control bytes, then highlighted via the markdown chroma path once at `NewModel` time and cached on `reviewInfoState.descriptionHighlighted` (the description is static, so re-highlighting on every overlay refresh would just produce identical bytes). Session metadata (mode, scope, filters, file/status counts, aggregate `+/-`) lives in the popup's top/bottom borders; commit log shows subject + body of every commit in the current ref range. Commits are populated eagerly at startup via `loadCommits()` in parallel with `loadFiles()` under `tea.Batch`; re-fetched on `R` reload. `handleInfo` always opens the popup; if the user presses `i` before the fetch lands, the commits section renders an inline "loading commits…" placeholder which flips to the rendered list when `commitsLoadedMsg` arrives (`refreshInfoOverlay` pushes a fresh spec into the open overlay). Sized via `clamp(term_w * 0.9, 30, 90)` × `term_h - 4`, wraps body text at word boundaries using `ansi.Wrap` from `charmbracelet/x/ansi` (ANSI-aware, preserves inline escapes). Renders "no commits in range" centered for the empty-list case and a truncated, italicized one-liner for fetch errors (`infoErrMaxLen` caps total length to keep the popup bounded against megabyte stderr).

`Manager.HandleKey()` returns an `Outcome` — Model switches on `OutcomeKind` to perform side effects (file jumps, theme apply/persist). This keeps overlay package free of Model dependencies. `Manager.HandleMouse()` mirrors the same shape for wheel and click events: `app/ui/mouse.go::handleOverlayMouse` delegates when an overlay is active so info scrolls, annotlist/themeselect move their cursors (themeselect emits `OutcomeThemePreview` to restyle the background live), and help treats wheel as a no-op. Left-click in annotlist selects the clicked row and emits `OutcomeAnnotationChosen`; left-click in themeselect emits `OutcomeThemeConfirmed` on entry rows (filter and blank separator are no-ops). Click hit-testing uses the last-composed popup bounds recorded in `Manager.bounds` during `Compose()`; clicks outside the popup rectangle are swallowed so accidental clicks don't dismiss the overlay.

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

Two-stage chord bindings (kitty-style, e.g. `map ctrl+w>x mark_reviewed`) are supported with a ctrl+/alt+ leader restriction. Storage is flat strings in `bindings` (`"ctrl+w>x" → Action`); a lazy `chordPrefixCache` provides O(1) `IsChordLeader` lookups. `Load` resolves conflicts by dropping a standalone whose key is also a chord leader. `ResolveChord` applies the same Latin layout-resolve fallback as `Resolve` for the second-stage key. Chord dispatch lives in `app/ui` on `keyState` (`handleChordSecond`, `clearPendingInputState`) and flows through the shared `dispatchAction` path so chord-resolved actions share handlers with single-key actions.

Help overlay dynamically rendered from `m.keymap.HelpSections()`.

### app/theme/ — theme system

Catalog-centric API with two types: `Theme` (data + serialization) and `Catalog` (directory-aware operations). Zero standalone functions — all logic lives as methods on `Theme` or `Catalog`.

**`Theme`** — color palette data with metadata. Methods: `Dump(io.Writer)` for serialization.

**`Catalog`** — theme discovery, loading, installation, and gallery access. Created via `NewCatalog(themesDir)`. Public methods: `Entries`, `Load`, `Resolve`, `InitBundled`, `InitAll`, `Install`, `PrintList`, `ActiveName`, `OptionalColorKeys`.

File layout:
- `theme.go` — `Theme` struct, `Dump`, package-level vars (`colorKeys`, `optionalColorKeys`)
- `catalog.go` — `Catalog` struct, `NewCatalog`, all catalog methods (discovery, loading, installation, gallery)

Bundled themes: revdiff, catppuccin-mocha, catppuccin-latte, dracula, gruvbox, nord, solarized-dark. Community themes live in `themes/gallery/`.

23 color keys mapped via `colorFieldPtrs()` in `app/themes.go` — single source of truth for color key to struct field mapping.

### app/annotation/ — annotation store

In-memory store for annotations. Each `Annotation` has file, line, text, and optional `EndLine` for hunk range headers (triggered when comment contains "hunk" keyword). Structured output formatting for export. `FormatOutput` escapes body lines that start with `## ` (with trailing space, matching the record-header form) by prefixing a single space so downstream parsers cannot confuse a comment line for a new record header. Lines starting with `###` or `##` without a space are left unchanged.

### app/editor/ — external editor invocation

Spawns the user's `$EDITOR` on a seeded temp file and reads the result back. TUI-agnostic — the caller wraps the returned `*exec.Cmd` with bubbletea's `tea.ExecProcess` (or runs it directly).

Single stateless type `Editor` bundling all behavior as methods (no standalone functions):
- `Command(content)` — writes content to a `revdiff-annot-*.md` temp file, resolves the editor (`$EDITOR` → `$VISUAL` → `vi`, whitespace-split so `code --wait` works), returns `*exec.Cmd` + a `complete(runErr) (string, error)` function. `complete` reads the file, removes it regardless of outcome, and preserves `runErr` — content is still returned alongside a non-nil `runErr` so callers can keep user work on soft editor failures.

Consumed by `app/ui` via the `ExternalEditor` interface (defined in `app/ui/editor.go`, consumer side). The default wiring is `editor.Editor{}` injected through `ModelConfig.Editor`.

### app/history/ — session auto-save

`Save(Params)` writes review session as markdown to `~/.config/revdiff/history/`. Includes header, annotations, and git diff for annotated files.

## Key Interfaces

All consumer-side — defined in `app/ui/model.go`, not in implementor packages (exception: `diff.Renderer` is a local mirror exported for moq generation). This is idiomatic Go: interfaces belong to the consumer.

| Interface | Methods | Implementors |
|-----------|---------|-------------|
| `Renderer` | `ChangedFiles()`, `FileDiff()` | `diff.Git`, `diff.Hg`, `diff.FileReader`, `diff.DirectoryReader`, `diff.StdinReader`, `diff.FallbackRenderer`, `diff.ExcludeFilter`, `diff.IncludeFilter` |
| `commitLogSource` | `CommitLog(ref)` | `diff.Git`, `diff.Hg`, `diff.Jj` (via `diff.CommitLogger` capability; resolved at Model construction by type-assertion on the Renderer when `ModelConfig.CommitLog` is nil) |
| `SyntaxHighlighter` | `HighlightLines()`, `SetStyle()`, `StyleName()` | `highlight.Highlighter` |
| `Blamer` | `FileBlame()` | `diff.Git`, `diff.Hg` |
| `styleResolver` | `Color()`, `Style()`, `LineBg()`, `LineStyle()`, `WordDiffBg()`, `IndicatorBg()` | `style.Resolver` |
| `styleRenderer` | `AnnotationInline()`, `DiffCursor()`, `StatusBarSeparator()`, `FileStatusMark()`, `FileReviewedMark()`, `FileAnnotationMark()` | `style.Renderer` |
| `sgrProcessor` | `Reemit()` | `style.SGR` |
| `wordDiffer` | `ComputeIntraRanges()`, `PairLines()`, `InsertHighlightMarkers()` | `worddiff.Differ` |
| `FileTreeComponent` | 17 methods (navigation, query, mutation, scroll-state, render) | `sidepane.FileTree` |
| `TOCComponent` | 9 methods (navigation, cursor/section query+set, scroll-state, render) | `sidepane.TOC` |
| `overlayManager` | `Active()`, `Kind()`, `OpenHelp()`, `OpenAnnotList()`, `OpenThemeSelect()`, `OpenInfo()`, `UpdateInfo()`, `Close()`, `HandleKey()`, `HandleMouse()`, `Compose()` | `overlay.Manager` |
| `ThemeCatalog` | `Entries()`, `Resolve()`, `Persist()` | `themeCatalog` adapter in `app/themes.go` (composes `theme.Catalog` + config persistence) |
| `ExternalEditor` | `Command(content)` returning `*exec.Cmd`, `complete(error) (string, error)`, `error` | `editor.Editor` (default wiring via `ModelConfig.Editor`; stubbed in tests) |

## Data Flow

### Startup

```
main()  [main.go]
  → parseArgs()          [config.go] → config file + CLI flags + env vars
  → handleThemes()       [themes.go] → theme resolution, apply
  → run()                [main.go]
      → prepareStdinMode [stdin.go]  (if --stdin)
      → setupVCSRenderer [renderer_setup.go] (otherwise)
      → construct style, theme catalog adapter, all dependencies
      → ui.NewModel(ModelConfig{...})
      → tea.NewProgram(model).Run()
      → saveHistory()   [history_save.go]
```

### File Loading (async)

```
Model.Init() → loadFiles cmd
            → Renderer.ChangedFiles() → filesLoadedMsg
            → handleFilesLoaded drops stale msg (seq mismatch), else m.filesLoaded = true
            → (on success) tree.Rebuild(entries)
            → auto-select first file → loadFileDiff cmd
            → Renderer.FileDiff() → fileLoadedMsg
            → highlight.HighlightLines() → highlightedLines
            → (optional) loadBlame cmd → blameLoadedMsg
```

`View()` is gated on two flags so the user never sees an empty two-pane layout
during async initialisation: it returns the literal `"loading..."` while
`!m.ready` (before the first `WindowSizeMsg`), then `"loading files..."` while
`m.ready && !m.filesLoaded` (after resize, before an accepted `filesLoadedMsg`).
`filesLoaded` flips to true on every accepted `filesLoadedMsg` (success or
error), so the loading screen always exits once the current in-flight load
returns. Stale responses (`msg.seq != m.filesLoadSeq`, e.g. an older load still
in flight after `toggleUntracked` bumped the sequence) are dropped and do not
flip the flag or rebuild the tree.

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

  → viewport.SetContent()
  → View():
    ├── truncateHeaderTitle() (sanitize + left-truncate filename to 1 row)
    ├── lipgloss.JoinVertical(header, viewport.View())
    ├── padContentBg() (pre-render: pane bg fill on assembled content)
    ├── lipgloss.Render() with Border() + Width()/Height()
    └── applyScrollbar() (post-render: thumb glyph on diff right-border rows)

sidepane.Render() (file tree or markdown TOC)
  → padContentBg()
  → lipgloss.Render() with Border() + Width()/Height()
  → applyNavigationScrollbar() (post-render: thumb glyph on navigation right-border rows)
  → terminal
```

Each rendering feature (line numbers, blame, word-diff, search, wrap, collapsed) is orthogonal — can be independently toggled.

### Annotation Flow

```
User presses 'a' on diff line
  → annotating = true, annotateInput focused
  → Enter → store.Add(file, line, text)  (single-line fast path)
  → Ctrl+E → openEditor()
      → editor.Editor.Command(seed)     (app/editor)
      → tea.ExecProcess(cmd, complete)  (suspends bubbletea, hands over tty)
      → editorFinishedMsg{content, err, target...}
      → handleEditorFinished:
          err != nil     → log, keep annotation mode open, preserve input
          content == ""  → cancelAnnotation (preserve existing annotation)
          otherwise      → saveComment(content, fileLevel, line, type)
  → re-render shows annotation (multi-line aware) below diff line
  → on quit: store.Format() → structured output to stdout/file
  → (optional) history.Save() → markdown to ~/.config/revdiff/history/
```

### Overlay Flow

```
User presses '?' / '@' / 'T' / 'i'
  → Model calls overlay.OpenHelp/OpenAnnotList/OpenThemeSelect/OpenInfo
      (for 'i': review scope is assembled from ReviewInfoConfig and current
       file-load state; aggregate +/- stats are fetched lazily on first open
       via loadReviewStats() and pushed into the open popup with UpdateInfo().
       Commits are fetched eagerly at startup via loadCommits(), running in
       parallel with loadFiles() under tea.Batch from Init(); triggerReload()
       re-fires both together. handleCommitsLoaded caches the result under a
       seq-guard (m.commits.loadSeq) and refreshes the open popup. The info
       popup always opens; modes without a meaningful commit range hide the
       commits section instead of treating `i` as a no-op.)
  → overlay.Manager activates popup, blocks other overlays
  → key events route through Manager.HandleKey() → Outcome
  → Model switches on OutcomeKind:
      OutcomeAnnotationChosen → load target file, position cursor
      OutcomeThemePreview → preview theme colors, update resolver
      OutcomeThemeConfirmed → apply theme, persist to config file
      OutcomeThemeCanceled → restore original theme
      OutcomeClosed → close overlay, resume normal mode
  → Manager.Compose() renders popup over background content
```

## Configuration System

**Precedence**: CLI flags > env vars > config file > built-in defaults

- **Config file**: `~/.config/revdiff/config` (INI format via go-flags IniParser)
- **Theme files**: `~/.config/revdiff/themes/` (auto-created on first run)
- **Keybindings**: `~/.config/revdiff/keybindings` (`map`/`unmap` format)
- **History**: `~/.config/revdiff/history/` (auto-save dir)

Theme precedence: `--theme` overwrites all 23 color fields + chroma-style, ignoring `--color-*` flags or env vars. Applied via `applyTheme()` in `app/themes.go` which directly overwrites `opts.Colors.*` fields after `parseArgs()`.

Adding a new color requires changes in three places: `theme.go` colorKeys + options struct + `colorFieldPtrs()` in `app/themes.go`.

Theme ownership is split by concern: `app/theme` owns discovery/loading/installation via `Catalog`, `app/ui` consumes a `ThemeCatalog` interface for selector/preview/apply, and `app/themes.go` wires a thin adapter composing `theme.Catalog` + config file persistence.

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

**Loaded-file state object** — `diffLines`, `highlightedLines`, `intraRanges`, and related per-file metadata are grouped into a single `loadedFileState` struct (`m.file`). This makes the synchronization invariant explicit — all parallel arrays and derived data for the current file are co-located rather than scattered across top-level Model fields.

**Model state sub-structs** — `Model` fields are grouped into named sub-structs (`cfg`, `layout`, `file`, `modes`, `nav`, `search`, `annot`) by concern. Methods remain on `Model` — the sub-structs make state ownership explicit without splitting into mini-models.

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
