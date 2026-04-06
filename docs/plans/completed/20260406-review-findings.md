# Address External Review Findings

## Overview
Fix 5 validated issues from external code review: theme color validation, annotation list keymap bypass, Dump error handling, handleThemes cleanup, and color schema deduplication.

## Context
- `theme/theme.go` - Parse() accepts any string for color values without hex format validation
- `ui/annotlist.go` - handleAnnotListKey() hardcodes j/k/up/down/enter/esc instead of using keymap
- `keymap/keymap.go` - Dump() discards write errors with `_, _ =`
- `cmd/revdiff/main.go` - handleThemes() calls os.Exit directly, mixes concerns
- `cmd/revdiff/main.go` - applyTheme() and collectColors() duplicate the 21-color key list from theme.colorKeys
- All 3 test files exist and have good coverage

## Development Approach
- **testing approach**: regular (code first, then tests)
- each task is independent and can be done in any order
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- run `make test` and `make lint` after each task

## Implementation Steps

### Task 1: Validate hex color format in theme.Parse()

**Files:**
- Modify: `theme/theme.go`
- Modify: `theme/theme_test.go`

- [x] add validateHexColor() package-level function that checks `#` prefix and 6 hex digits (strict, no 3-digit shorthand)
- [x] call validateHexColor() in Parse() when storing color-* values, return error on invalid
- [x] write tests for valid hex colors (#aabbcc, #AABBCC, #123456)
- [x] write tests for invalid hex colors (no #, wrong length, non-hex chars, empty)
- [x] verify existing theme tests still pass
- [x] run `make test && make lint`

### Task 2: Route annotation list keys through keymap

**Files:**
- Modify: `ui/annotlist.go`
- Modify: `ui/annotlist_test.go`

- [x] replace hardcoded `"j"`, `"down"` with `m.keymap.Resolve(msg.String()) == keymap.ActionDown`
- [x] replace hardcoded `"k"`, `"up"` with `m.keymap.Resolve(msg.String()) == keymap.ActionUp`
- [x] keep `"enter"` hardcoded (modal overlay convention, same as confirm discard)
- [x] for `"esc"`, use hybrid pattern: `action == keymap.ActionDismiss || msg.Type == tea.KeyEsc` (matches handleHelpKey)
- [x] add test: remap j to x, verify x navigates down in annotation list
- [x] add test: verify enter and esc still work regardless of keymap remapping
- [x] run `make test && make lint`

### Task 3: Return error from Keymap.Dump()

**Files:**
- Modify: `keymap/keymap.go`
- Modify: `cmd/revdiff/main.go` (caller)
- Modify: `keymap/keymap_test.go`

- [x] change Dump() signature from `Dump(w io.Writer)` to `Dump(w io.Writer) error`
- [x] propagate fmt.Fprintln/Fprintf errors instead of discarding
- [x] update caller in main.go to handle returned error
- [x] update existing Dump tests to check returned error
- [x] add test with failing writer to verify error propagation
- [x] run `make test && make lint`

### Task 4: Refactor handleThemes to return (bool, error) instead of os.Exit

**Files:**
- Modify: `cmd/revdiff/main.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] change handleThemes signature to accept `io.Writer` for stdout/stderr (matches dumpConfig pattern)
- [x] change return type to `(bool, error)` where bool=true means "should exit"
- [x] remove os.Exit calls, return appropriate values instead
- [x] write output to the io.Writer params instead of os.Stdout/os.Stderr directly
- [x] update caller in run() to handle (bool, error) and call os.Exit there
- [x] write tests calling handleThemes directly with bytes.Buffer writers
- [x] test init-themes, list-themes, load-theme, error cases via return values
- [x] run `make test && make lint`

### Task 5: Unify color schema to single source of truth

**Files:**
- Modify: `cmd/revdiff/main.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] add unexported `colorFieldPtrs(opts *options) map[string]*string` in main.go that maps color key names to opts.Colors field pointers
- [x] refactor applyTheme() to use colorFieldPtrs() instead of its hardcoded map
- [x] refactor collectColors() to use colorFieldPtrs() instead of its hardcoded map
- [x] ensure adding a new color only requires changes in theme.go colorKeys + options struct + colorFieldPtrs
- [x] verify applyTheme and collectColors produce identical results to before (existing tests)
- [x] run `make test && make lint`

### Task 6: Verify acceptance criteria

- [x] run full test suite: `make test`
- [x] run linter: `make lint`
- [x] verify all 5 bundled themes parse correctly (existing test covers this)
- [x] verify `--dump-keys` and `--dump-theme` still work

### Task 7: [Final] Update documentation

- [x] update CLAUDE.md if new patterns discovered
- [x] move this plan to `docs/plans/completed/`
