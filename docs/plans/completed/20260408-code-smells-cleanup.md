# Code Smells Cleanup

## Overview
Fix code smells identified across all revdiff packages. 8 smell analysis agents found 53 findings total. This plan addresses all categories except function length (category 5) and context propagation in `runGit` (AO — substantial scope, separate future task).

Categories addressed:
1. Repeated expressions → helper methods
2. Standalone functions → methods (per project convention)
3. Code duplication → extracted helpers
4. Inconsistent truncation (potential Unicode bug)
6. Excessive parameters → option structs
7. Confusing value receiver mutation
8. Over-exported symbols → unexport
9. Stale comment
10. Minor fixes (slices.Insert, sort→slices, constants, missing comments)

## Context
- Files involved: ~25 files across `app/ui/`, `app/diff/`, `app/highlight/`, `app/theme/`, `app/keymap/`, `app/main.go`
- All findings verified with grep — exact line numbers and occurrence counts confirmed
- Existing test coverage is good; most changes are mechanical renames/extractions
- Dependencies: task ordering matters for files touched by multiple tasks

## Development Approach
- **Testing approach**: Regular — code changes first, verify existing tests pass after each task
- Complete each task fully before moving to the next
- Most items are mechanical extractions/renames where existing tests provide coverage
- **CRITICAL: when extracting shared helpers, keep parameter count under 4**. If a helper needs more, use an options struct. Do not solve one smell (duplication) by creating another (excessive params)
- **CRITICAL: every task MUST include new/updated tests** when behavior changes or new functions are added
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run `go test ./...` after each task, formatters + linter after final task

## Implementation Steps

### Task 1: Extract repeated expressions into helper methods

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/view.go`
- Modify: `app/ui/diffview.go`
- Modify: `app/ui/collapsed.go`

- [x] Add `treePaneHidden() bool` method to `model.go` — returns `m.treeHidden || (m.singleFile && m.mdTOC == nil)`. Replace at `view.go:32`, `model.go:369`, `model.go:493`, `diffview.go:501`
- [x] Add `wrapWidth() int` method to `diffview.go` (near `diffContentWidth`) — returns `m.diffContentWidth() - wrapGutterWidth - m.gutterExtra()`. Replace at `diffview.go:233,270`, `collapsed.go:129,183,206`
- [x] Add `isCursorLine(idx int) bool` method to `model.go` — returns `idx == m.diffCursor && m.focus == paneDiff && !m.cursorOnAnnotation`. Replace at `diffview.go:202`, `collapsed.go:96,198`
- [x] Add `effectiveStatusFg() string` method to `view.go` — replaces the 3-line statusFg fallback pattern at `view.go:172` and `view.go:384`
- [x] Run `go test ./app/ui/...` — must pass before task 2

### Task 2: Convert standalone functions to struct methods

**Files:**
- Modify: `app/diff/blame.go`
- Modify: `app/diff/blame_test.go`
- Modify: `app/diff/diff.go`
- Modify: `app/diff/diff_test.go`
- Modify: `app/highlight/highlight.go`
- Modify: `app/highlight/highlight_test.go`
- Modify: `app/ui/handlers.go`

- [x] `app/diff/blame.go`: convert `blameTargetRef` (63), `parseBlame` (81), `isHexString` (132) to `(g *Git)` methods. Update test call sites in `blame_test.go` to call via a `Git{}` value
- [x] `app/diff/diff.go`: unexport `ParseUnifiedDiff` → standalone `parseUnifiedDiff` (NOT a method — it is a pure parser with no `Git` state). Update caller in `FileDiff` (149). Update all test call sites in `diff_test.go` (same package, unexported access works)
- [x] `app/highlight/highlight.go`: convert `reconstructFiles` (86), `highlightFile` (109), `mapHighlightedLines` (182) to `(h *Highlighter)` methods. Keep `writeTokenANSI` (149) as package-level function — it is a generic ANSI token writer with no `Highlighter` state coupling. Update call sites in `HighlightLines`. Update test call sites in `highlight_test.go`
- [x] `app/ui/handlers.go`: convert standalone `displayKeyName` (32) to `(m Model) displayKeyName` method
- [x] Run `go test ./app/diff/... ./app/highlight/... ./app/ui/...` — must pass before task 3

### Task 3: Simple deduplication

**Files:**
- Modify: `app/main.go`
- Modify: `app/main_test.go`
- Modify: `app/ui/annotlist.go`
- Modify: `app/ui/handlers.go`
- Modify: `app/ui/filetree.go`
- Modify: `app/ui/mdtoc.go`

- [x] `app/main.go`: extract `resolveFlagPath(args []string, flag, envVar string, defaultFn func() string) string` from near-identical `resolveConfigPath` (253) and `resolveKeysPath` (281). Follow-up: inlined callers to use `resolveFlagPath` directly and removed the single-line wrapper functions
- [x] `app/ui/annotlist.go`: extract `annotListBoxStyle(width int) lipgloss.Style` from identical 5-line boxStyle construction in `annotListOverlay` (51) and `annotListEmptyOverlay` (68)
- [x] `app/ui/handlers.go`: extract `helpColors() (reset, header, key string)` from duplicate ANSI color var setup in `helpOverlay` (63-65) and `writeTOCHelpSection` (210-212). Define `type helpLine struct{ keys, desc string }` at package level instead of inside both methods
- [x] `app/ui/filetree.go` + `mdtoc.go`: extract package-level `ensureVisibleInList(cursor, offset *int, count, height int)` from identical `fileTree.ensureVisible` (110) and `mdTOC.ensureVisible` (118). Both keep their methods as thin wrappers calling the shared function
- [x] Run `go test ./app/... ./app/ui/...` — must pass before task 4

### Task 4: Fix truncation inconsistency (potential Unicode bug)

**Files:**
- Modify: `app/ui/filetree.go`
- Modify: `app/ui/filetree_test.go`
- Modify: `app/ui/themeselect.go`
- Modify: `app/ui/themeselect_test.go`

- [x] `app/ui/filetree.go:403` (`truncateDirName`): replace `len(name)` with `len([]rune(name))` and byte-based slicing `name[len(name)-maxWidth+1:]` with rune-based left-truncation. Keep Unicode ellipsis `"..."`
- [x] `app/ui/themeselect.go:196` (`formatThemeEntry`): replace `len(name)` with `len([]rune(name))`, byte-based `name[:nameMaxWidth-3]` with rune-based slicing, and ASCII `"..."` with Unicode `"..."`
- [x] Add test cases for both functions with multi-byte Unicode input (e.g., CJK characters, emoji) to verify correct truncation
- [x] Run `go test ./app/ui/...` — must pass before task 5

### Task 5: Complex deduplication

**Files:**
- Modify: `app/theme/theme.go`
- Modify: `app/theme/theme_test.go`
- Modify: `app/diff/fallback.go`
- Modify: `app/diff/fallback_test.go`
- Modify: `app/ui/view.go`
- Modify: `app/ui/search.go`

- [x] `app/theme/theme.go`: extract `initThemes(themesDir string, filter func(string, Theme) bool) error` from `InitBundled` (279), `InitAll` (302), `InitNames` (322). Shared function handles gallery-load + mkdir + iterate-filter-write. `InitBundled` passes `th.Bundled` filter, `InitAll` passes nil. `InitNames` keeps its upfront name-validation loop (returns error for unknown names) and only delegates the write loop to `initThemes`
- [x] `app/diff/fallback.go`: extract shared stat+open+read+error-unwrap logic from `readFileAsContext` (229) and `ReadFileAsAdded` (266). Follow-up: renamed `openFileForReading` back to `readFileAsContext` and removed the single-line alias wrapper; `ReadFileAsAdded` calls `readFileAsContext` directly
- [x] `app/ui/view.go`: extract `renderTwoPaneLayout(leftContent string, ph int) string` from duplicate TOC/tree branches in `View()` (46-68 vs 74-97). Handles focus-based style selection, `padContentBg`, width calc, `JoinHorizontal`
- [x] `app/ui/search.go`: extract `findFirstVisibleMatch(startIdx int) int` from duplicate wrap-around scan in `submitSearch` (52-75) and `realignSearchCursor` (139-152). Returns match index or -1. Each caller applies its own side effects
- [x] Run `go test ./app/theme/... ./app/diff/... ./app/ui/...` — must pass before task 6

### Task 6: Reduce parameter counts + fix value receiver confusion

**Files:**
- Modify: `app/ui/collapsed.go`
- Modify: `app/ui/filetree.go`
- Modify: `app/ui/diffview.go`

- [x] `app/ui/collapsed.go:126`: define `type wrappedLineCtx struct` with fields `gutter, numGutter, blGutter string; isCursor, hasHighlight bool; style, hlStyle lipgloss.Style; bgColor string`. Change `renderWrappedCollapsedLine` signature to `(b *strings.Builder, textContent string, ctx wrappedLineCtx)`. Update caller in `renderCollapsedAddLine`
- [x] `app/ui/filetree.go:286`: define `type renderCtx struct { annotatedFiles map[string]bool; s styles }` and pass it as a single parameter to `renderFileEntry` instead of two separate params. New signature: `(e treeEntry, idx, width int, rc renderCtx)` — avoids temporal state coupling from storing fields on the struct
- [x] `app/ui/diffview.go:144`: replace `m.blameNow = time.Now()` with local `now := time.Now()` passed to `blameGutter`. Change `blameGutter` to accept `now time.Time` parameter instead of reading `m.blameNow`
- [x] `app/ui/diffview.go:150`: change `buildSearchMatchSet` to return `map[int]bool` instead of mutating `m.searchMatchSet`. If passing through cascades to too many methods (renderDiffLine, renderDeletePlaceholder read it), keep the field but add comment explaining the value-receiver copy semantics
- [x] Run `go test ./app/ui/...` — must pass before task 7

### Task 7: Unexport over-exported symbols

**Files:**
- Modify: `app/theme/theme.go`
- Modify: `app/theme/theme_test.go`

- [x] Unexport `InstallFile` (352) → `installFile`. Update caller in `Install` (413)
- [x] Replace `ChromaValidator` type (346) with inline `func(string) bool` in `Install` and `installFile` signatures. Remove the named type
- [x] Unexport `IsLocalPath` (436) → `isLocalPath`. Update caller in `Install`
- [x] Unexport `BundledNames` (477) → `bundledNames`. Change return to `([]string, error)` matching `GalleryNames` pattern (behavior change: callers must now handle error instead of getting silent nil). Update test callers to handle error return
- [x] Run `go test ./app/theme/...` — must pass before task 8

### Task 8: Stale comment + minor fixes

**Files:**
- Modify: `app/ui/annotate.go`
- Modify: `app/ui/configpatch.go`
- Modify: `app/ui/loaders.go`
- Modify: `app/theme/theme.go`
- Modify: `app/keymap/keymap.go`
- Modify: `app/ui/handlers.go`
- Modify: `app/ui/diffnav.go`

- [x] `annotate.go:172`: fix stale comment — remove "checks the previous line" clause, accurately describe the `cursorOnAnnotation` guard
- [x] `configpatch.go:58`: replace nested `append` with `slices.Insert(lines, insertIdx, "theme = "+themeName)`
- [x] `loaders.go:30`: remove misleading capacity hint — change `make(map[string]bool, len(entries))` to `make(map[string]bool)` (entries is always empty at that point)
- [x] `theme/theme.go`: replace `sort.Strings` → `slices.Sort`, `sort.Slice` → `slices.SortFunc` throughout. Drop `"sort"` import
- [x] `keymap/keymap.go`: add `const SectionPane = "Pane"` near section definitions. Use in `handlers.go:89` and in keymap's own section definition
- [x] `loaders.go`: add doc comments on `loadFiles`, `loadFileDiff`, `loadBlame`, `handleFilesLoaded`
- [x] `diffnav.go:514`: change `jumpTOCEntry` return to `(tea.Model, tea.Cmd)`, return `m, nil`. Update caller in `handlers.go`
- [x] `annotate.go:282`: define `const annotKeyFile = "file"`. Replace magic `"file"` string in `wrappedAnnotationLineCount` and callers
- [x] Run `go test ./...` — must pass

### Task 9: Final verification and documentation

**Files:**
- Modify: `CLAUDE.md`

- [x] Run `~/.claude/format.sh` (gofmt + goimports)
- [x] Run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [x] Run `go test -race ./...`
- [x] Update CLAUDE.md data flow section: replace `ParseUnifiedDiff` reference with `parseUnifiedDiff` (now unexported method)
- [x] Move this plan to `docs/plans/completed/`

## Task Order & Dependencies

```
Task 1 (repeated exprs) ──→ Task 6 (shares diffview.go + collapsed.go)
Task 2 (methods)         ──→ independent
Task 3 (simple dedup)    ──→ independent
Task 4 (truncation)      ──→ independent
Task 5 (complex dedup)   ──→ Task 7 (both touch theme.go)
Task 8 (minor)           ──→ after Task 7 (AN touches theme.go)
```

Recommended order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9

### Task 10: Post-review cleanup

- [x] Remove single-line wrapper functions: inlined `resolveConfigPath`/`resolveKeysPath` callers to use `resolveFlagPath` directly, renamed `openFileForReading` back to `readFileAsContext` and dropped alias wrapper
- [x] Replace all `else if` patterns (7 instances) with `switch` statements or split `if` blocks across `blame.go`, `theme.go`, `mdtoc.go`, `filetree.go`, `diffnav.go`, `annotate.go`
- [x] Fix stale `placeholderContents` comment referencing deleted `openFileForReading`

## Post-Completion

**Deferred items** (not in scope):
- AO: Add `context.Context` parameter to `runGit` for cancellation support — substantial refactor affecting all git command callers, separate future task
- Category 5 (function length): `helpOverlay` 120 lines, `handleFileLoaded` 76 lines, `View()` 100 lines, `statusBarText` 99 lines, `ParseUnifiedDiff` 96 lines — excluded per user decision
