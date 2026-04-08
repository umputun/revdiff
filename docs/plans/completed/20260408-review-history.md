# Review History Auto-Save

## Overview
- Auto-save review sessions (annotations + diff) to `~/.config/revdiff/history/<repo-name>/<timestamp>.md` on normal quit
- Default location: `~/.config/revdiff/history/`, configurable via `--history-dir` flag / `REVDIFF_HISTORY_DIR` env / config file
- Safety net for lost annotations (process crash after quit, agent fails to capture stdout) and future reference
- Always-on, silent operation — errors logged to stderr, never fail the process

## Context (from discovery)
- Files/components involved: `app/main.go` (run function, quit path), `app/annotation/store.go` (FormatOutput, Files)
- Related patterns: `defaultConfigPath()`, `defaultThemesDir()` for `~/.config/revdiff/` path resolution
- Dependencies: `annotation.Store` for formatted output and file list, `os/exec` for git diff call
- Integration point: `run()` in `app/main.go` lines 445-464, after FormatOutput succeeds

## Development Approach
- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change

## Testing Strategy
- **unit tests**: required for every task
- test Save with annotations + git diff (mock git execution)
- test Save with annotations only (non-git mode)
- test directory creation
- test error handling (write failures logged, not fatal)
- test filename/path generation

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with + prefix
- document issues/blockers with warning prefix

## Solution Overview
New `app/history/` package with a single `Save()` function. Called from `run()` in `app/main.go` on normal quit when annotations exist. Assembles a markdown file with header metadata, annotations (reusing `Store.FormatOutput()`), and raw git diff for annotated files only.

**File format:**
```markdown
# Review: 2026-04-08 14:30:05
path: /Users/umputun/dev.umputun/revdiff
refs: master..HEAD
commit: abc1234

## Annotations

## app/ui/model.go:42 (+)
this needs refactoring

---

## Diff

diff --git a/app/ui/model.go b/app/ui/model.go
...raw unified diff...
```

**File organization:** `~/.config/revdiff/history/<repo-basename>/<timestamp>.md`

## Technical Details
- **History directory**: default `~/.config/revdiff/history/`, overridable via `--history-dir` CLI flag, `REVDIFF_HISTORY_DIR` env var, or `history-dir` config key. Same precedence as other options: CLI > env > config > default
- Timestamp format for filenames: `2006-01-02T15-04-05` (filesystem-safe, no colons)
- Timestamp format in header: `2006-01-02 15:04:05` (human-readable)
- Repo name: `filepath.Base(path)` — base name of git root or file directory
- For `--stdin` mode: subdir = `stdin`, path header = `stdin`
- For `--only` without git: subdir = base name of file's parent dir, path = full file path
- For `--all-files` mode: annotations-only history (no diff section) — git diff returns empty for unchanged files
- Git diff command: `git diff --no-color --no-ext-diff [--cached] [ref] -- file1 file2 ...`
- Git commit hash: `git rev-parse --short HEAD`
- Directory created with `os.MkdirAll` on each save (idempotent)
- All errors logged to stderr via `log.Printf`, never returned

## Implementation Steps

### Task 1: Create history package with Save function

**Files:**
- Create: `app/history/history.go`
- Create: `app/history/history_test.go`

- [x] create `app/history/history.go` with package declaration and imports
- [x] define `Params` struct with fields: `Annotations` (string, from FormatOutput), `Path` (string, full repo/file path), `Ref` (string, git ref or empty), `Staged` (bool), `GitRoot` (string, empty when git unavailable), `AnnotatedFiles` ([]string, from Store.Files()), `HistoryDir` (string, base history directory override, empty = default)
- [x] implement `Save(p Params)` function: create directory with `os.MkdirAll`, assemble header, annotations section, diff section (if git available), write to history file
- [x] implement `historyDir()` helper — uses `p.HistoryDir` if set, otherwise `~/.config/revdiff/history/`; appends `<repo-basename>/` subdir using `filepath.Base(p.Path)`
- [x] implement `timestamp()` helper for filename and header formatting
- [x] implement `gitDiff()` helper — runs `git diff --no-color --no-ext-diff [--cached] [ref] -- files...` via `exec.Command`, returns raw output string; returns empty string on error
- [x] implement `gitCommitHash()` helper — runs `git rev-parse --short HEAD` in git root, returns hash or empty string on error
- [x] write tests for Save with annotations and simulated git repo (verify file created, format correct)
- [x] write tests for Save without git (non-git mode, no diff section)
- [x] write tests for historyDir path generation (various path inputs)
- [x] write tests for error handling (unwritable directory — verify no panic, logged error)
- [x] run tests — must pass before next task

### Task 2: Add --history-dir flag and wire history.Save into app/main.go

**Files:**
- Modify: `app/main.go`

- [x] add `HistoryDir` field to `options` struct: `long:"history-dir" ini-name:"history-dir" env:"REVDIFF_HISTORY_DIR" description:"directory for review history auto-saves"`
- [x] import `app/history` package
- [x] hoist `gitRoot` variable from inner `else` scope (line 381) to the outer `var` block (lines 363-368) so it is accessible at the integration point after TUI quit
- [x] after `output := m.Store().FormatOutput()` succeeds (line 453), call `history.Save()` with appropriate params assembled from `opts`, `gitRoot`, `workDir`, and `store`
- [x] pass `store.FormatOutput()` as Annotations, `workDir` or gitRoot as Path, `opts.ref()` as Ref, `opts.Staged` as Staged, gitRoot as GitRoot (empty string when git unavailable), `store.Files()` as AnnotatedFiles, `opts.HistoryDir` as HistoryDir
- [x] for `--stdin` mode: set Path to `stdin`, GitRoot to empty
- [x] verify history save is skipped on discard (already guarded by `m.Discarded()` check above)
- [x] run tests — must pass before next task

### Task 3: Verify acceptance criteria

- [x] verify history file created in correct location on normal quit with annotations
- [x] verify no history file on discard quit or empty annotations
- [x] verify header contains correct metadata (path, refs, commit)
- [x] verify annotations section matches FormatOutput format
- [x] verify diff section contains only annotated files' diffs
- [x] verify non-git mode produces annotations-only file
- [x] run full test suite: `make test`
- [x] run linter: `make lint`

### Task 4: [Final] Update documentation

- [x] update README.md with history auto-save feature description
- [x] update CLAUDE.md project structure section with `app/history/` package
- [x] update `site/docs.html` if applicable
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- run `revdiff` on a real diff, add annotations, quit — verify `~/.config/revdiff/history/` populated
- check file content format is correct and readable
- verify stdin mode saves annotations-only history
- verify discard quit (Q) does not save history
