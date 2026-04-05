# All-Files Mode: Browse and Annotate Entire Projects

## Overview
- Add `--all-files` / `-A` flag to browse all git-tracked files in a project, not just diffs
- Add `--exclude` / `-X` flag with prefix matching to filter out unwanted paths (vendor, mocks, etc.)
- Enables using revdiff as a general-purpose code annotation tool, not just a diff viewer
- `--exclude` works in both `--all-files` and normal diff modes

## Context
- `diff/fallback.go` has `FileReader` and `readFileAsContext()` — reusable for reading files as all-context lines
- `cmd/revdiff/main.go` has `makeRenderer()` for renderer selection and `options` struct for CLI flags
- `ui.Renderer` interface (`ChangedFiles`, `FileDiff`) is the contract — new readers just need to satisfy it
- File tree UI already handles flat file lists, no UI changes needed

## Design Decisions
- `--all-files` requires git repo, uses `git ls-files` for file discovery (no directory walking)
- `--all-files` mutually exclusive with refs, `--staged`, and `--only`
- `--exclude` uses prefix matching (`--exclude vendor` skips `vendor/`, `vendor/foo.go`, etc.)
- `--exclude` is ini-saveable (persistent in config file), `--all-files` is CLI-only (no-ini)
- New `DirectoryReader` in `diff/` for git ls-files based file discovery
- New `ExcludeFilter` wrapper in `diff/` to filter any renderer's file list
- Flat file list in tree pane (no collapsible directory tree)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Implementation Steps

### Task 1: Add ExcludeFilter wrapper
- [x] create `diff/exclude.go` with local `renderer` interface (same signatures as `ui.Renderer`: `ChangedFiles`, `FileDiff`) to avoid import cycle (diff cannot import ui)
- [x] create `ExcludeFilter` struct wrapping the local `renderer` interface
- [x] implement `ChangedFiles()` that delegates to inner renderer then filters out files matching any exclude prefix
- [x] implement `FileDiff()` that delegates directly to inner renderer (filtering happens at file list level only)
- [x] implement `matchesExclude()` helper method for prefix matching logic
- [x] write tests in `diff/exclude_test.go` for prefix matching (exact prefix, nested paths, no false matches)
- [x] write tests for `ChangedFiles()` filtering with mock renderer
- [x] write tests for `FileDiff()` passthrough behavior
- [x] run tests — must pass before next task

### Task 2: Add DirectoryReader
- [x] create `diff/directory.go` with `DirectoryReader` struct (workDir string)
- [x] implement `ChangedFiles()` using `git ls-files` to list tracked files, return sorted relative paths
- [x] implement `FileDiff()` delegating to existing `readFileAsContext()`
- [x] write tests in `diff/directory_test.go` for `ChangedFiles()` with a temp git repo
- [x] write tests for `FileDiff()` reading files as all-context lines
- [x] write tests for edge cases (empty repo, binary files in list)
- [x] run tests — must pass before next task

### Task 3: Add CLI flags and wiring
- [x] add `AllFiles` flag to `options` struct: `long:"all-files" short:"A" no-ini:"true"`
- [x] add `Exclude` flag to `options` struct: `long:"exclude" short:"X" ini-name:"exclude"` (repeatable `[]string`)
- [x] add `env:"REVDIFF_EXCLUDE" env-delim:","` to Exclude field (comma-separated in env, repeated in CLI/config)
- [x] add validation in `parseArgs()`: `--all-files` conflicts with refs, `--staged`, and `--only`
- [x] update `makeRenderer()` signature to accept allFiles/excludes params (or pass via opts struct)
- [x] create `DirectoryReader` when `--all-files` is set (error if no git repo)
- [x] wrap any renderer with `ExcludeFilter` when `--exclude` patterns are present
- [x] update `run()` to pass new opts to `makeRenderer()`
- [x] write tests in `cmd/revdiff/main_test.go` for parseArgs validation (conflict detection)
- [x] write tests for makeRenderer with `--all-files` returning DirectoryReader
- [x] write tests for makeRenderer with `--exclude` wrapping with ExcludeFilter
- [x] write tests for `--exclude` combined with normal diff mode (ExcludeFilter wrapping Git renderer)
- [x] write tests for `--all-files` conflicting with `--only`
- [x] run tests — must pass before next task

### Task 4: Verify acceptance criteria
- [ ] verify `revdiff --all-files` shows all git-tracked files in file tree
- [ ] verify `revdiff --all-files --exclude vendor` filters correctly
- [ ] verify `--exclude` in config file works persistently
- [ ] verify `--exclude` works in normal diff mode (e.g., `revdiff --exclude vendor`)
- [ ] verify `--all-files` with refs or `--staged` produces clear error
- [ ] run full test suite
- [ ] run linter — all issues must be fixed
- [ ] verify test coverage meets 80%+

### Task 5: [Final] Update documentation
- [ ] update README.md with `--all-files` and `--exclude` flag documentation
- [ ] update `.claude-plugin/skills/revdiff/references/usage.md` with `--all-files` examples
- [ ] update `.claude-plugin/skills/revdiff/references/config.md` with `--exclude` config option
- [ ] update CLAUDE.md if new patterns or gotchas discovered
- [ ] verify `--dump-config` output includes `exclude` field
- [ ] ask about plugin version bump (plugin reference docs changed)

## Technical Details

**DirectoryReader.ChangedFiles():**
```
git ls-files
```
Lists tracked files only (defaults to `--cached`). Returns sorted list of relative paths.

**ExcludeFilter.matchesExclude(file, prefix):**
- `strings.HasPrefix(file, prefix)` or `strings.HasPrefix(file, prefix+"/")`
- Handles both `vendor` and `vendor/` as prefix input

**Renderer selection in makeRenderer():**
```
if allFiles && gitErr == nil → DirectoryReader
if allFiles && gitErr != nil → error ("--all-files requires git repo")
existing logic for Git/FallbackRenderer/FileReader unchanged
then: wrap with ExcludeFilter if excludes non-empty
```

**Config file example:**
```ini
exclude = vendor
exclude = mocks
```

## Post-Completion

**Manual verification:**
- test with a real large repo to verify performance of git ls-files
- test annotation workflow across many files
- verify config file exclude persistence works end-to-end
