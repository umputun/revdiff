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
- `Renderer` - `ChangedFiles()`, `FileDiff()` - implemented by `diff.Git`
- `SyntaxHighlighter` - `HighlightLines()` - implemented by `highlight.Highlighter`

## Data Flow
```
git diff → diff.ParseUnifiedDiff() → []DiffLine
  → highlight.HighlightLines() → []string (ANSI foreground-only)
  → ui.renderDiffLine() → lipgloss styles (background) + chroma (foreground)
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
