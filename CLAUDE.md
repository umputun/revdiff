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
- `app/` - composition root (`package main`), split by concern: `main.go` (entrypoint + `run()`), `config.go` (options/parsing), `stdin.go` (stdin mode), `renderer_setup.go` (VCS wiring), `themes.go` (theme CLI + adapter), `history_save.go` (session save)
- `app/diff/` - VCS interaction (git + hg + jj), unified diff parsing, VCS detection, Mercurial + Jujutsu support
- `app/ui/` - bubbletea TUI package. Single `Model` struct with state grouped into sub-structs (`cfg`, `layout`, `file`, `modes`, `nav`, `search`, `annot`), methods split across files by concern (~500 lines each). Each source file has a matching `_test.go`. See `app/ui/doc.go` for package docs, `docs/ARCHITECTURE.md` for file-by-file breakdown. Does not import `app/theme` or `app/fsutil` — theme operations go through the `ThemeCatalog` interface
- `app/ui/style/` - color/style resolution: hex-to-ANSI, lipgloss styles, SGR tracking, HSL math. Types: `Resolver`, `Renderer`, `SGR`
- `app/ui/sidepane/` - file tree + markdown TOC components with cursor/offset management
- `app/ui/worddiff/` - intra-line word-diff: tokenizer, LCS, line pairing, highlight insertion
- `app/ui/overlay/` - layered popups: help, annotation list, theme selector. Manager enforces one-at-a-time
- `app/highlight/` - chroma syntax highlighting, foreground-only ANSI
- `app/keymap/` - configurable keybindings (`Action` constants, parser, defaults, dump)
- `app/theme/` - Catalog-centric theme system: `Theme` (data + serialization) and `Catalog` (discovery, loading, installation, gallery). Zero standalone functions — all logic as methods. Files: `theme.go` (Theme struct), `catalog.go` (Catalog struct + all operations). 7 bundled + community gallery
- `app/annotation/` - in-memory annotation store and structured output
- `app/editor/` - external `$EDITOR` invocation for multi-line annotations: temp-file lifecycle, editor resolution ($EDITOR → $VISUAL → vi), and `Command()` API returning `*exec.Cmd` + completion func for `tea.ExecProcess`. Consumed by `app/ui` via the `ExternalEditor` interface
- `app/history/` - review session auto-save to `~/.config/revdiff/history/`
- `app/fsutil/` - filesystem utilities
- `app/ui/mocks/` - moq-generated mocks (never edit manually)

## Architecture Principles
- **Decouple OS/external concerns from UI**: OS-level work (exec.Command, os.CreateTemp, env lookup, network calls) does not belong in `app/ui/`, even for small helpers. Extract to a dedicated package (e.g. `app/editor/`) even when there's only one production caller. The cost of a new package is lower than keeping `app/ui/` entangled with OS boundaries.
- **Minimize exported surface**: first pass of a new package almost always over-exports. Before finalizing, ask "which of these does the caller actually need?" and unexport the rest. For stateless helpers, prefer unexported methods on the grouping struct over top-level unexported functions — matches the global "prefer methods over standalone utilities" rule.
- **Consumer-side interfaces for external deps**: when UI uses an external subsystem, the default shape is: interface in `app/ui/` (consumer side), concrete type in the subsystem package, injection via `ModelConfig` (nil defaults to concrete). Direct import of a concrete type from an external package into `app/ui` is a smell — the interface documents the dependency direction and leaves room for alternate implementations, not just test mocks.

## Config
- Config file: `~/.config/revdiff/config` (INI format via go-flags IniParser)
- Precedence: CLI flags > env vars > config file > built-in defaults
- `--dump-config` outputs current defaults, `--config` overrides path
- `no-ini:"true"` tag excludes fields from config file (used for --config, --dump-config, --dump-theme, --list-themes, --init-themes, --version)
- Themes dir: `~/.config/revdiff/themes/` with 7 bundled themes, auto-created on first run
- `--theme NAME` loads theme; `--dump-theme` exports resolved colors; `--list-themes` lists available; `--init-themes` re-creates bundled
- Theme precedence: `--theme` overwrites all 23 color fields + chroma-style, ignoring `--color-*` flags or env vars
- Theme values applied via `applyTheme()` in `themes.go` which overwrites `opts.Colors.*` after `parseArgs()`. `colorFieldPtrs(opts)` is the single source of truth for color key → struct field mapping — adding a new color requires changes in `theme.go` colorKeys + options struct + `colorFieldPtrs()` in `themes.go`
- Theme ownership split: `app/theme.Catalog` owns discovery/loading, `app/ui.ThemeCatalog` interface consumed by UI, `app/themes.go` wires adapter composing catalog + config persistence
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
- **Launcher override chain**: both Claude plugins resolve their launcher script via `resolve-launcher.sh` through `user → bundled` layers (first executable wins). User layer is `${CLAUDE_PLUGIN_DATA}/scripts/<launcher>`. There is **no project-level (`.claude/...`) layer by design** — the planning hook fires automatically on `ExitPlanMode` in any repo, and a repo-controlled executable layer would let an untrusted repo run arbitrary code on routine Claude actions. The diff-review resolver keeps the same two-layer shape for symmetry (single mental model, shared resolver). The override chain is **Claude-only** — pi (no `CLAUDE_PLUGIN_DATA` in runtime) and codex (no plugin-data path) ignore it; codex users edit `~/.codex/skills/revdiff/scripts/launch-revdiff.sh` directly to customize.
- **Testing locally**: `claude --plugin-dir .claude-plugin` loads the diff-review skill from this checkout without going through the marketplace; `claude --plugin-dir plugins/revdiff-planning` does the same for the planning hook. Use `/reload-plugins` to pick up file edits mid-session.

## Codex Skills
- Codex skills live at `plugins/codex/skills/` — two skills: `revdiff` (diff review) and `revdiff-plan` (plan review via last Codex assistant message)
- Install is a skills-only copy to `~/.codex/skills/<name>/` — codex does NOT scan `~/.codex/plugins/`, that path is reserved for plugins synced from `github.com/openai/plugins`
- No plugin manifest or marketplace envelope — the `.codex-plugin/` / `.agents/` layout was non-conformant with codex's actual format and removed
- Script path resolution in SKILL.md falls back to `${CODEX_HOME:-$HOME/.codex}/skills/<skill>/scripts` when not running inside the revdiff repo
- Scripts are copies from `.claude-plugin/skills/revdiff/scripts/`, not symlinks — each has a source comment at top
- `detect-ref.sh` dispatches by VCS (`detect_git` / `detect_hg` / `detect_jj`) via `command -v` probes (jj → git → hg, matching `DetectVCS` precedence); git path stays byte-identical to the pre-refactor output. `read-latest-history.sh` uses the same VCS probe order for repo-root resolution.
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
- `setupVCSRenderer()` (in `renderer_setup.go`) detects VCS via `diff.DetectVCS()` (walks up looking for `.jj`, `.git`, `.hg` — in that precedence order; `.jj` wins over `.git` in colocated repos); if no VCS is found and `--only` is set, uses `FileReader` for standalone file review. `--stdin` skips VCS lookup entirely, validates non-TTY stdin, reads payload before starting Bubble Tea, and reopens `/dev/tty` for interactive key input. `--all-files` is supported for git and jj; not supported in hg.
- `--all-files` mode uses `DirectoryReader` (git ls-files) to list all tracked files; `--include` wraps any renderer with `IncludeFilter` for prefix-based inclusion, `--exclude` wraps with `ExcludeFilter` for prefix-based exclusion (include narrows first, then exclude removes). `--include` is mutually exclusive with `--only`. `--all-files` is mutually exclusive with refs, `--staged`, and `--only`. `--stdin` is mutually exclusive with refs, `--staged`, `--only`, `--all-files`, `--include`, and `--exclude`.
- `diff.readReaderAsContext()` is the shared parser for file-backed and stdin-backed context-only views. Preserve its behavior if you change binary detection, line-length handling, or line numbering.
- Overlay popups managed by `overlay.Manager`. `Compose()` uses ANSI-aware compositing via `charmbracelet/x/ansi.Cut`. `HandleKey()` returns `Outcome` — Model switches on `OutcomeKind` for side effects (file jumps, theme apply/persist). Overlay kinds: help, annot-list, theme-select, commit-info. One overlay at a time — opening any overlay auto-closes whichever was previously open
- Reload (`R` key): `reloadState` on `Model` holds `pending bool` (waiting for y/cancel), `hint string` (transient status-bar message), and `applicable bool` (false in `--stdin` mode — stream consumed). `ReloadApplicable` is wired at the composition root in `main.go`, following the same pattern as `CommitsApplicable`. The reload method is named `triggerReload()` — not `reload()` — because Go forbids a method and a field with the same name on the same type (`Model.reload` is the state field). Reload resets the diff cursor to the top of the file; tree selection (which file) is restored by `SelectByPath` in `handleFilesLoaded`.
- Commit info overlay (`i` key) uses the `diff.CommitLogger` capability interface (additive to `diff.Renderer`). Model resolves a `commitLogSource` at construction: explicit `ModelConfig.CommitLog` wins, else type-asserts the renderer for `CommitLogger`, else the feature is unavailable and `i` is a no-op with a transient status-bar hint. `CommitsApplicable` is computed at the composition root by `commitsApplicable()` in `main.go` (using the `commitLogger` field populated by `setupVCSRenderer` in `renderer_setup.go`) — Model copies it, does not re-derive. Data is fetched eagerly at startup and on `R` reload: `Init()` and `triggerReload()` both return `tea.Batch(m.loadFiles(), m.loadCommits())`, running files and commits loads in parallel as independent goroutines. `loadCommits()` captures `m.commits.loadSeq` at invocation time and tags the resulting `commitsLoadedMsg` with it; `handleCommitsLoaded` drops any message whose seq no longer matches (stale-result guard, mirrors the files-load pattern). Eager parallel fetch shrinks the window where overlay and diff can disagree from "time until first `i` press" to "skew between two parallel goroutine starts" (milliseconds). Not a strict snapshot guarantee — the two subprocesses each resolve `HEAD` independently — but the practical race window is tens of ms instead of minutes. If the user presses `i` before `commitsLoadedMsg` arrives, `handleCommitInfo` sets a transient `loading commits…` hint instead of opening the overlay — a second press after load succeeds. Hg cannot use literal NUL in argv templates — use ASCII US/RS (`\x1f`/`\x1e`) as field/record separators for hg only
- **ANSI nesting with lipgloss**: `lipgloss.Render()` emits `\033[0m` (full reset) which breaks outer style backgrounds. For styled substrings inside a lipgloss container (status bar separators, search highlights, diff cursor, annotation lines), use raw ANSI sequences via the style sub-package (`style.AnsiFg`, `resolver.Color`), or dedicated `Renderer` methods. Never use `lipgloss.NewStyle().Render()` for inline elements within a lipgloss-rendered parent.
- **Background fill for themed panes**: lipgloss pane `Render()` and viewport internal padding emit plain spaces after reset, causing terminal default bg. Workarounds: (1) `extendLineBg()` pads lines to full width, (2) `padContentBg()` re-pads pane content, (3) `BorderBackground()` on border styles. **Ordering**: `extendLineBg()` must be called AFTER `applyHorizontalScroll()`.
- Horizontal scroll indicators (`«`/`»`): see `applyHorizontalScroll()` in `diffview.go`. `«` replaces first visible column when scrolled past hidden content. `»` extends 1 col into right padding. Bg split: `«` and separator space use line bg via `indicatorBg()`, `»` glyph uses `DiffBg`. Only in unwrapped mode.
- Status bar mode icons: `▼◉↩≋⊟#b±✓∅` rendered via `statusModeIcons()`. Graceful degradation drops segments on narrow terminals.
- Search and hunk navigation use `centerViewportOnCursor()` (cursor ends up in the middle of the page). `syncViewportToCursor()` is the general "keep cursor visible" path and is called from cursor moves (j/k, g/G, page up/down), content mutations (annotation save/delete, blame load), and layout changes (tree/wrap/blame/line-number toggles, resize, file load). It uses `cursorVisualRange()` so the cursor's full logical line — wrap-continuation rows plus any injected annotation rows — stays visible, not just the cursor's top row.
- **Viewport scroll after content mutation**: `syncViewportToCursor()` calls `SetContent(renderDiff())` itself before setting `YOffset` — callers must not pre-`SetContent` around it, and any code that injects rows below a diff line (wrap continuations, multi-line annotations, future overlays) must route scroll through this function so visual-height math stays consistent with `cursorVisualRange()` / `hunkLineHeight()`.
- Single-file mode (`m.file.singleFile`): one file → tree hidden, diff full width. Exception: markdown full-context gets TOC pane.
- Tree pane toggle (`t` key): `m.layout.treeHidden` orthogonal to `singleFile`.
- Markdown TOC: activated when `singleFile && isMarkdownFile && isFullContext`. Uses `paneTree` slot.
- Annotation list popup (`@` key): cross-file jumps via `pendingAnnotJump` field with stale-jump guard.
- **Typed-nil trap**: `ParseTOC` factory MUST guard `if toc == nil { return nil }` to collapse typed-nil `*sidepane.TOC` into interface-nil.
- **External editor exec (`tea.ExecProcess`)**: long-running external commands (e.g. `$EDITOR` for multi-line annotations in `app/ui/editor.go`) suspend bubbletea and hand over the tty. Capture target state (target line, file-level flag, change type) at spawn time and pass it through the completion message — cursor movement during the exec window otherwise misroutes the result. Temp files are always cleaned up in the completion callback regardless of error path.
