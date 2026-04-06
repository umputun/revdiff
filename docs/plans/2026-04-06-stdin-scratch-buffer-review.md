# Stdin / Scratch-Buffer Review

## Overview
- Add a scratch-buffer review mode for arbitrary piped content: `echo "..." | revdiff`
- Add explicit `--stdin` flag for redirected input and scripting: `revdiff --stdin < /tmp/output.txt`
- Preserve the normal revdiff UX: single-file view, inline annotations, file-level notes, search, wrap, collapsed mode, and structured annotation output
- Keep git/file review modes unchanged

## Context
- `cmd/revdiff/main.go` currently picks a `ui.Renderer` via `makeRenderer()` using git refs, `--only`, and `--all-files`
- `diff/fallback.go` already has file-backed context readers (`FileReader`, `DirectoryReader`, `readFileAsContext`) that feed the same `[]diff.DiffLine` model used by the UI
- `ui.Model` already auto-switches to single-file mode when `ChangedFiles()` returns exactly one file
- Syntax highlighting and markdown TOC activation are filename-based:
  - highlighting uses `highlight.HighlightLines(filename, lines)`
  - markdown TOC only activates for a single full-context file with `.md` / `.markdown`
- Bubble Tea currently uses `os.Stdin` for interactive input; that conflicts with piped content because stdin is consumed by the payload rather than keypresses
- Releases only target linux/darwin (`.goreleaser.yml`), so reopening `/dev/tty` for interactive input is a valid implementation strategy

## Design Decisions

### 1. Treat stdin review as a separate source mode
- Add a new source mode for scratch content, distinct from git diff, `--only`, and `--all-files`
- In stdin mode, revdiff presents exactly one synthetic file and all lines are `ChangeContext`
- This keeps the UI and annotation model unchanged

### 2. Explicit activation only
- `--stdin` flag is required to enter stdin mode
- No implicit detection of piped stdin — without `--stdin`, revdiff behaves normally regardless of stdin state
- This keeps the resolver simple: `--stdin` present → stdin mode, otherwise → normal mode
- Implicit detection could be added later if users request it (additive change)

### 3. Separate payload input from interactive terminal input
- When stdin mode is active:
  - read payload from `os.Stdin` before starting the TUI
  - reopen `/dev/tty` and pass it to Bubble Tea with `tea.WithInput(...)`
- Without this split, `echo "..." | revdiff` would start a non-interactive TUI because Bubble Tea would be reading from an exhausted pipe
- Non-stdin modes continue to use the default Bubble Tea input path

### 4. Add a synthetic filename, with optional override
- Add `--stdin-name` as an optional CLI-only flag
- Default synthetic name: `scratch-buffer`
- Examples:
  - `echo "# plan" | revdiff --stdin-name plan.md`
  - `git show HEAD~1:README.md | revdiff --stdin-name README.md`
- Rationale:
  - gives annotation output a stable file key
  - enables better syntax highlighting when the user provides an extension
  - enables markdown TOC for piped plans/documents when the name ends with `.md`

### 5. Keep annotation output format unchanged
- Stdin mode uses the existing output format:
  - `## scratch-buffer:12 ( )`
  - `## plan.md (file-level)`
- No new serialization format is needed
- Downstream tooling can treat the synthetic filename exactly like a normal file key

### 6. Reuse the context-line pipeline instead of inventing a new UI path
- Add a new in-memory renderer in `diff/`, e.g. `StdinReader`
- It returns one file from `ChangedFiles()` and the pre-read `[]DiffLine` from `FileDiff()`
- This is the smallest change because:
  - single-file mode already works
  - annotations already work on context-only lines
  - search, wrap, collapsed mode, and status bar already work on context-only files

### 7. Share stream parsing with file parsing
- Refactor the current file reader path so stdin and file review use the same line-construction rules
- Add a reader-based helper, e.g. `readReaderAsContext(r io.Reader) ([]DiffLine, error)`
- Keep `readFileAsContext(path)` as the file-specific wrapper responsible for:
  - regular-file checks
  - broken symlink placeholder
  - non-regular-file placeholder
- The shared reader helper should preserve current behavior for:
  - binary detection via NUL bytes in the first 8KB
  - 1MB max line length placeholder
  - all-context line numbering

## CLI Semantics

### New flags
- `--stdin`
  - force scratch-buffer review from stdin
  - CLI-only, not config-saveable
- `--stdin-name`
  - synthetic file name for stdin content
  - CLI-only, not config-saveable
  - only meaningful when stdin mode is active

### Validation rules
- `--stdin` conflicts with:
  - positional refs
  - `--staged`
  - `--only`
  - `--all-files`
  - `--exclude`
- If `--stdin-name` is set without `--stdin`, return an error
- If `--stdin` is set but stdin is a TTY, return an error like `--stdin requires piped or redirected input`

### Non-conflicts
- `--output`, `--wrap`, `--collapsed`, `--line-numbers`, themes, colors, and keybindings should work unchanged
- `--blame` is effectively disabled because stdin mode has no blamer; no extra special-case UI work is needed since `ShowBlame` already depends on `cfg.Blamer != nil`

## Implementation Shape

### `diff/`
- Add `StdinReader`:
  - fields: `name string`, `lines []DiffLine`
  - `ChangedFiles()` returns `[]string{name}`
  - `FileDiff()` returns the stored lines for the synthetic file
- Refactor shared parsing:
  - add `readReaderAsContext(r io.Reader) ([]DiffLine, error)`
  - keep `readFileAsContext(path)` as the file-path wrapper

### `cmd/revdiff/main.go`
- Extend `options` with:
  - `Stdin bool`
  - `StdinName string`
- Add stdin-mode validation:
  - validate `--stdin` conflicts with source flags
  - validate `--stdin` requires non-TTY stdin
  - validate `--stdin-name` requires `--stdin`
  - if active, read stdin into `[]DiffLine`
- Update renderer selection:
  - if stdin mode is active, return `diff.NewStdinReader(name, lines)`
  - skip git root lookup entirely in this branch
- Update Bubble Tea program creation:
  - stdin mode: `tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(tty))`
  - normal mode: existing behavior

### `ui/`
- No intended behavioral changes
- Existing single-file logic should automatically hide the tree
- Markdown TOC should work automatically when `--stdin-name` is markdown-like and all lines are context

## Testing Strategy

### `diff/`
- add tests for `readReaderAsContext()`:
  - normal text
  - empty input
  - binary placeholder
  - oversized line placeholder
- add tests for `StdinReader`:
  - one synthetic file returned
  - `FileDiff()` returns stored lines

### `cmd/revdiff/main_test.go`
- parseArgs:
  - `--stdin`
  - `--stdin-name`
  - conflict cases with refs / `--staged` / `--only` / `--all-files` / `--exclude`
  - `--stdin-name` without `--stdin`
- stdin validation:
  - `--stdin` with non-TTY stdin succeeds
  - `--stdin` with TTY stdin errors
- makeRenderer / source selection:
  - stdin mode returns `*diff.StdinReader`
  - normal git and file modes remain unchanged

### Manual verification
- `echo "hello" | revdiff --stdin`
- `printf '# title\n\nbody\n' | revdiff --stdin --stdin-name plan.md`
- `some-command | revdiff --stdin --output /tmp/annotations.txt`
- `echo "x" | revdiff --stdin HEAD~1` should fail with a clear source-conflict error

## Implementation Steps

### Task 1: Add CLI flags and stdin-mode validation
- [ ] add `--stdin` and `--stdin-name` to `options`
- [ ] add parse-time validation for `--stdin` conflicts with source flags
- [ ] add validation that `--stdin-name` requires `--stdin`
- [ ] add tests in `cmd/revdiff/main_test.go`
- [ ] run `go test ./...`

### Task 2: Add shared reader-based context parsing
- [ ] refactor `diff/fallback.go` to add `readReaderAsContext(io.Reader)`
- [ ] keep `readFileAsContext(path)` behavior identical by wrapping the new helper
- [ ] add tests for reader-based parsing
- [ ] run `go test ./...`

### Task 3: Add `diff.StdinReader`
- [ ] create in-memory renderer for one synthetic file
- [ ] add focused tests for `ChangedFiles()` and `FileDiff()`
- [ ] run `go test ./...`

### Task 4: Wire stdin mode into `run()`
- [ ] resolve stdin payload before renderer selection
- [ ] skip git lookup when stdin mode is active
- [ ] create Bubble Tea program with `/dev/tty` input in stdin mode
- [ ] keep non-stdin program setup unchanged
- [ ] add tests around source selection helpers
- [ ] run `go test ./...`

### Task 5: Verify UI behavior and docs
- [ ] verify single-file mode engages automatically for stdin content
- [ ] verify annotations, file-level notes, search, wrap, and collapsed mode all work
- [ ] update `README.md` usage and flags tables
- [ ] update `CLAUDE.md` with the new source mode and `/dev/tty` gotcha
- [ ] update `.claude-plugin/skills/revdiff/references/usage.md` if kept in sync with README
- [ ] run `go test ./...` and `golangci-lint run`

## Notes / Non-Goals
- No attempt to infer syntax from stdin content itself; filename-based hinting via `--stdin-name` is sufficient for v1
- No support for mixing stdin payload review with git/file sources in one session
- No new annotation output schema
