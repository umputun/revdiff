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
- `SyntaxHighlighter` - `HighlightLines()`, `SetEnabled()`, `Enabled()` - implemented by `highlight.Highlighter`

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

## Gotchas
- Project uses vendoring - run `go mod vendor` after adding/updating dependencies
- Chroma API uses British spelling (`Colour`), suppress with `//nolint:misspell`
- Syntax highlighting uses specific ANSI resets (`\033[39m`, `\033[22m`, `\033[23m`) instead of full reset (`\033[0m`) to preserve lipgloss backgrounds
- Highlighted lines are pre-computed once per file load, stored parallel to `diffLines`
- `DiffLine.Content` has no `+`/`-` prefix - prefix is re-added at render time
- Tab replacement happens at render time in `renderDiffLine`, not in diff parsing
