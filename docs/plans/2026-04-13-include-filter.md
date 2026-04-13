# Add --include filter flag

## Overview
- Add `--include` flag (`-I`) for prefix-based file inclusion filtering
- Mirrors existing `--exclude` (`-X`) pattern: same prefix matching, same decorator approach
- Composable with `--exclude`: include narrows first, then exclude removes from the included set
- Example: `revdiff --include src --exclude src/vendor` shows `src/` minus `src/vendor/`

## Context (from discovery)
- Files/components involved:
  - `app/diff/exclude.go` — existing ExcludeFilter (pattern to mirror)
  - `app/diff/exclude_test.go` — existing tests (pattern to mirror)
  - `app/main.go` — options struct (line ~49), renderer factories (`makeGitRenderer`, `makeHgRenderer`, `makeNoVCSRenderer`), `validateStdinFlags`
  - `app/main_test.go` — CLI parsing tests, validation tests, renderer wrapping tests
- Related patterns: ExcludeFilter decorator wraps any renderer, filters at `ChangedFiles()` level only, `FileDiff()` passes through
- Dependencies: `jessevdk/go-flags` for CLI parsing with struct tags

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- Mirror existing `exclude_test.go` patterns for include filter tests
- Mirror existing `main_test.go` patterns for CLI parsing and renderer wrapping tests

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Create IncludeFilter in app/diff/
<!-- [architect,simplifier,go_idioms] Extract shared matchesPrefix(file, prefixes) and normalizePrefixes([]string) helpers instead of duplicating matchesExclude logic. Both filters call the shared helpers. Keep IncludeFilter and ExcludeFilter as separate types (matches project convention of one-type-per-file). -->
<!-- [go_idioms] Reuse the existing `renderer` interface from exclude.go — do not redefine it in include.go. -->
<!-- [conventions] The mockRenderer in exclude_test.go is visible to include_test.go (same package) — reuse it, do not redefine. -->
<!-- [conventions] Error messages should match exclude format: "include filter, changed files: %w" and "include filter, file diff %s: %w" -->
- [x] extract shared `matchesPrefix(file string, prefixes []string) bool` and `normalizePrefixes([]string) []string` helpers (can live in `exclude.go` or new `prefix.go`)
- [x] update `ExcludeFilter` to use the shared helpers
- [x] create `app/diff/include.go` with `IncludeFilter` struct, `NewIncludeFilter` constructor (uses shared `normalizePrefixes`)
- [x] implement `ChangedFiles()` — keep only files matching any include prefix (uses shared `matchesPrefix`)
- [x] implement `FileDiff()` — passthrough to inner renderer (same as ExcludeFilter)
- [x] write tests in `app/diff/include_test.go` (reuse `mockRenderer` from exclude_test.go): matchesPrefix cases, ChangedFiles filtering, prefix normalization, FileDiff passthrough and error propagation
- [x] run `make test` — must pass before next task

### Task 2: Add --include CLI flag and validation
<!-- [architect,completionist,go_idioms] Add --include + --only mutual exclusivity check in parseArgs(), next to the existing --all-files + --only check. --only specifies exact files; --include is a prefix filter — combining them is contradictory and produces silent empty results. -->
<!-- [completionist] Verify --dump-config output includes the new field (should be automatic from go-flags). -->
<!-- [architect] Verify --help output shows --include and --exclude with parallel description style. -->
- [x] add `Include` field to `options` struct: `[]string`, `long:"include"`, `short:"I"`, `ini-name:"include"`, `env:"REVDIFF_INCLUDE"`, `env-delim:","`
- [x] add `--stdin` + `--include` mutual exclusivity check in `validateStdinFlags()` (after existing --exclude check)
- [x] add `--include` + `--only` mutual exclusivity check in `parseArgs()`
- [x] verify `--help` output and `--dump-config` output look correct
- [x] write tests: `TestParseArgs_IncludeFlag`, `TestParseArgs_IncludeShortFlag`, `TestParseArgs_IncludeEnvVar`, `TestParseArgs_IncludeConfigFile` (mirror exclude patterns)
- [x] write test: stdin + include validation error, only + include validation error
- [x] run `make test` — must pass before next task

### Task 3: Wire IncludeFilter into renderer factories
<!-- [architect,conventions,go_idioms] Clarify wrapping order: `r = NewIncludeFilter(r, include)` placed BEFORE the existing `r = NewExcludeFilter(r, exclude)` block. Include wraps inner first, exclude wraps that result. -->
<!-- [go_idioms] Guard with `if len(include) > 0` — unguarded wrap with empty prefixes would return zero files, breaking the default case. -->
<!-- [go_idioms] Factory signatures are growing (6 params with include). Consider grouping only/include/exclude/allFiles into a filterOpts struct, or accept opts directly. -->
<!-- [completionist] Add a functional composition test: mock renderer with known files → wrap with both filters → assert final ChangedFiles output, not just structural type checks. -->
- [ ] update `makeGitRenderer` — add `include []string` param, wrap with `if len(include) > 0 { r = diff.NewIncludeFilter(r, include) }` before the existing exclude wrapping
- [ ] update `makeHgRenderer` — same pattern
- [ ] update `makeNoVCSRenderer` — same pattern (--only is now mutually exclusive, so this is safe)
- [ ] update `setupVCSRenderer` to pass `opts.Include` to factory calls
- [ ] write tests: `TestMakeGitRenderer_WithInclude`, `TestMakeHgRenderer_WithInclude`
- [ ] write functional composition test: mock with known files → IncludeFilter + ExcludeFilter → assert correct filtered output
- [ ] write test: `TestMakeGitRenderer_AllFilesWithInclude`
- [ ] run `make test` — must pass before next task

### Task 4: Verify acceptance criteria
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`

### Task 5: Update documentation
<!-- [architect,conventions,completionist] Additional doc targets: site/index.html features grid (if filtering is mentioned), codex skill references at plugins/codex/skills/revdiff/, pi plugin docs if they document flags. -->
- [ ] update README.md with `--include` flag documentation
- [ ] update `site/docs.html` with `--include` flag
- [ ] update `.claude-plugin/skills/revdiff/references/config.md` if it documents flags
- [ ] update `plugins/codex/skills/revdiff/` references if they document flags
- [ ] check `site/index.html` features grid — update if filtering is mentioned

## Technical Details

### Composition order
```go
// in each factory function, after the switch block:
if len(include) > 0 {
    r = diff.NewIncludeFilter(r, include)  // narrows to matching prefixes
}
if len(exclude) > 0 {
    r = diff.NewExcludeFilter(r, exclude)  // removes excluded prefixes
}
```

IncludeFilter wraps inner first, ExcludeFilter wraps that result. Call chain: `ExcludeFilter.ChangedFiles() → IncludeFilter.ChangedFiles() → inner.ChangedFiles()`. This means:
- `--include src` → only `src/**` files
- `--include src --exclude src/test` → `src/**` minus `src/test/**`
- `--exclude vendor` (no include) → everything minus `vendor/**`

### Mutual exclusivity
- `--include` + `--stdin` → error (stdin has no filenames to filter)
- `--include` + `--only` → error (conflicting narrowing: exact files vs prefix filter)
- `--include` + `--exclude` → composable (include narrows, exclude refines)
- `--include` + `--all-files` → composable (all tracked files narrowed by prefix)

### Matching semantics
Same as exclude: `file == prefix || strings.HasPrefix(file, prefix+"/")`

### Struct tag
```go
Include []string `long:"include" short:"I" ini-name:"include" env:"REVDIFF_INCLUDE" env-delim:"," description:"include only files matching prefix (may be repeated)"`
```

## Post-Completion

**Manual verification:**
- Test with a real git repo: `revdiff --include src`
- Test composition: `revdiff --include src --exclude src/vendor`
- Test with `--all-files --include src`
- Verify config file parsing: add `include = src` to `~/.config/revdiff/config`
