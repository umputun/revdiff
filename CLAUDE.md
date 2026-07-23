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
- `app/diff/` - VCS interaction (git + hg + jj), unified diff parsing, VCS detection, Mercurial + Jujutsu support. `compare.go` — `CompareReader` renderer for `--compare-old/--compare-new` (runs `git diff --no-index`, no repo required)
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
- `app/handoff/` - prepares user-configured post-flush shell commands. Annotation snapshots are provided on stdin; stdout is suppressed so helpers cannot overwrite the TUI. Consumed by `app/ui` via the `PostFlushHook` interface and `tea.ExecProcess`
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
- **CLI flag description style is minimal and atomic** — match `--staged` ("show staged changes") / `--blame` ("show blame gutter"). Never include "at startup", "on startup", "(mirrors X toggle)", "(same state as X)", or cross-references to runtime toggle keys in the struct tag description, README/docs.html/plugin config.md table rows, godoc, or usage example comments. The flag description states what the flag does; users discover runtime toggles via the keybindings table or status-bar legend. This rule applies to every surface that describes a flag.
- **Mode-gating pattern for CLI flags with mode-dependent applicability**: when a flag is meaningful in some modes (working-tree, single-ref, `--staged`) but not others (two-ref, `--stdin`, `--compare-old/--compare-new`), gate it at the composition root via a method on `options` that returns the resolved bool, parallel to `options.ref()`. Example: `options.startupUntracked()` returns `false` in two-ref mode (`a b` or `a..b`) so working-tree state doesn't leak into historical diffs. `main.go` then wires `ShowUntracked: opts.startupUntracked()` into `ModelConfig`. The Model takes the resolved bool — it does NOT re-derive from CLI options or refs. Composition-root gating composes cleanly with the Model's own capability gate (e.g. `cfg.ShowUntracked && cfg.LoadUntracked != nil`) which handles the stdin/compare modes where the loader function is nil.

## Website
- Static site in `site/` (index.html, docs.html, style.css), deployed to revdiff.com via Cloudflare Pages
- Cloudflare Pages strips `.html` and 308-redirects `/docs.html` → `/docs`. Canonical tags, `og:url`, and `sitemap.xml` entries for documentation pages must use the extension-less URL (`/docs`), not the source filename (`/docs.html`), or Google indexes a redirect and tanks CTR
- `site/docs.html` must stay in sync with README.md - when adding features, flags, keybindings, or modes, update both
- `site/index.html` landing page should reflect major new features in the features grid and plugin sections
- **CRITICAL: After each release, update the version badge in `site/index.html`** (search for `hero-badge` div) and `softwareVersion` in JSON-LD

## Claude Code Plugin
- Plugin lives at `.claude-plugin/` with `plugin.json`, `marketplace.json`, and `skills/`
- Skills path in `plugin.json` is relative to repo root, not to `.claude-plugin/`
- **CRITICAL: Version bumps happen at release only — never per-PR or per-change.** Do NOT prompt to bump `plugin.json` / `marketplace.json` after a plugin file change; the bump is done as part of the release process.
- When bumping at release, keep each marketplace entry synchronized with its plugin manifest. For `revdiff-planning`, update both `.claude-plugin/plugin.json` and `.codex-plugin/plugin.json` plus its version in `.claude-plugin/marketplace.json`.
- **CRITICAL: Defer plugin version bumps when the change depends on a new binary feature.** If a plugin/launcher change relies on a `revdiff` binary feature, flag, env var, or exit code that is not yet in a tagged release, do NOT bump `plugin.json` / `marketplace.json` / `package.json` on the feature branch. The plugin (marketplace) and the binary (brew / `go install`) version independently — bumping the plugin early ships an updated launcher to users still running an old binary, causing a hard mismatch (e.g. the launcher passes an unknown flag, the old binary exits 1, every plugin-triggered review fails). Bump plugin/package versions as part of the binary version release, after the binary is tagged.
- Reference docs at `.claude-plugin/skills/revdiff/references/` — keep in sync with README.md:
  - `install.md` — installation methods and plugin setup
  - `config.md` — options, colors, chroma styles
  - `usage.md` — examples, key bindings, output format
- **Adding a new CLI flag requires SKILL.md updates, not just reference docs.** `references/config.md` and `references/usage.md` document the flag's *existence*; `SKILL.md` teaches AI agents *when* to pass it during automatic launches (e.g. "pass `--untracked` when the recent change likely created new untracked files"). Without a SKILL.md entry, AI agents using the plugin will not know to pass the flag even though it's documented. Apply the same update to `plugins/codex/skills/revdiff/SKILL.md` (keep in sync with `.claude-plugin/skills/revdiff/SKILL.md`) and to `plugins/pi/skills/revdiff/SKILL.md` (which lists user-facing command examples). The launcher scripts (`launch-revdiff.sh`) pass `"$@"` through, so no script changes are needed beyond updating the usage-comment header for documentation parity.
- **Launcher override chain**: both Claude plugins resolve their launcher script via `resolve-launcher.sh` through `user → bundled` layers (first executable wins). The planning plugin's user layer is `${CLAUDE_PLUGIN_DATA}/scripts/<launcher>` under Claude and `${PLUGIN_DATA}/scripts/<launcher>` under Codex. There is **no project-level (`.claude/...` or `.codex/...`) executable layer by design** — the planning hook fires automatically in any repo, and a repo-controlled launcher would run on routine agent actions. The Pi extension and manual Codex diff-review skill do not use this plugin-data override.
- **tmux backend runs revdiff in a detached session, not directly in the popup**: `launch-revdiff.sh` creates a detached session named `revdiff-<pid>` running revdiff, then opens `display-popup` attached to it (`TMUX=` lifts the same-server nesting guard). Detaching (`prefix d` or a user toggle binding) backgrounds the review — the popup closes, revdiff keeps running — so **popup exit does NOT mean review finished**; completion is signalled by a sentinel file (same pattern as the zellij/kitty backends) and the launcher waits until the sentinel appears or the session is gone. Follow-up tmux calls target the session ID captured from `new-session -P -F '#{session_id}'`, never the name: name targets prefix-match, and `set-option`/`show-options` reject the `=` exact-match prefix that `has-session`/`kill-session` accept (observed on tmux 3.7b). Per-session options set at launch: `status off`, `detach-on-destroy on` (a global `off` would switch the popup to another session instead of closing it when revdiff exits), `destroy-unattached off`, and `@revdiff_title` (stashed so reattach bindings can restore the popup title). The launcher also applies user-supplied session options from `REVDIFF_TMUX_SESSION_OPTIONS` so external session-manager tooling can tag and recognize the transient session (token format documented in `references/install.md` — keep README/docs.html/install.md as the synced sources, don't restate the contract here). Two agent-relevant facts: the manager's opt-out contract lives on the manager's side, never named in revdiff; and this env var configures the launcher process itself (runs in the parent shell), so the "launcher env vars don't reach the popup" caveat below does not apply to it. Signal handling is asymmetric by design: INT kills the detached session (user cancelled), TERM does NOT — harnesses send TERM at command-timeout caps while the reviewer is mid-review, and the popup + session outlive the launcher process (verified: killing both the launcher and its `display-popup` client leaves the popup attached), so the TERM trap prints a recovery note, disarms the EXIT cleanup (`trap - EXIT`) to preserve the output file as a recovery artifact, and exits 143. All sentinel backends capture revdiff's stderr to `<sentinel>.err` (the overlay closes too fast to read a fast-fail error otherwise); `print_output_and_exit` replays it on any exit other than 0/10 — this is how unknown-flag failures from old binaries become diagnosable instead of a bare exit 1.
- **Launcher env vars don't reach the tmux/zellij popup**: `launch-revdiff.sh` spawns the revdiff process in a fresh shell inside the multiplexer popup that does NOT inherit the parent shell's environment, so env-var config set before the launch is dropped (e.g. `REVDIFF_THEME=gruvbox launch-revdiff.sh HEAD~10` does not apply the theme). Pass it as a CLI flag instead: `launch-revdiff.sh --theme gruvbox HEAD~10`. Applies to any env-var-configurable option launched through the overlay.
- **Testing locally**: `claude --plugin-dir .claude-plugin` loads the diff-review skill from this checkout without going through the marketplace; `claude --plugin-dir plugins/revdiff-planning` does the same for the planning hook. Use `/reload-plugins` to pick up file edits mid-session.

## Codex Plugin and Skills
- Codex skills live at `plugins/codex/skills/` — two skills: `revdiff` (diff review) and `revdiff-plan` (plan review via last Codex assistant message)
- Manual skill install copies to `~/.codex/skills/<name>/`; automatic plan review is distributed separately through the `revdiff-planning` Codex plugin
- Keep Claude's default-discovered `PreToolUse/ExitPlanMode` config in `hooks/hooks.json`; the Codex manifest explicitly points its opt-in `Stop` hook at `hooks/codex-hooks.json`
- Script path resolution in SKILL.md falls back to `${CODEX_HOME:-$HOME/.codex}/skills/<skill>/scripts` when not running inside the revdiff repo
- Scripts are copies from `.claude-plugin/skills/revdiff/scripts/`, not symlinks — each has a source comment at top
- `detect-ref.sh` dispatches by VCS (`detect_git` / `detect_hg` / `detect_jj`) via `command -v` probes (jj → git → hg, matching `DetectVCS` precedence); git path stays byte-identical to the pre-refactor output. `read-latest-history.sh` uses the same VCS probe order for repo-root resolution.
- Codex automatic plan review runs only for `permission_mode=plan`, prefers a complete plan in `last_assistant_message`, and falls back whenever that field has no complete block to the last assistant message for the exact transcript/session/turn; manual `/revdiff-plan` remains the best-effort rollout fallback

## Pi Plugin
- Pi package defined in root `package.json`, extensions and skills in `plugins/pi/`
- Pi review path is direct-terminal only: `/revdiff [args]` suspends pi, runs the `revdiff` binary directly, and sends captured annotations to the agent immediately. There is no Pi overlay mode, pending annotation widget/panel, `/revdiff-rerun`, `/revdiff-results`, `/revdiff-apply`, `/revdiff-clear`, or default post-edit reminder command.
- Pi ships its own copy of `detect-ref.sh` at `plugins/pi/scripts/detect-ref.sh` (source-of-truth is `.claude-plugin/skills/revdiff/scripts/detect-ref.sh` — keep in sync, same pattern as the codex copy). The pi extension resolves the script relative to its own plugin root, never via `.claude-plugin/`; the pi package must stay installable standalone with no Claude plugin files present. Do not re-add `launch-revdiff.sh` to the Pi package surface unless the workflow is explicitly changed.
- **CRITICAL: Version bumps happen at release only — never per-PR or per-change.** Do NOT prompt to bump `package.json` after a pi plugin file change; the bump is done as part of the release process.
- Version in `package.json` is independently versioned (does not track the project's git tags)
