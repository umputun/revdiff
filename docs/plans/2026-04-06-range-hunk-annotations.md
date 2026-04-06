# Plan: Range & Hunk Annotations

Add visual-mode range selection (`V`) and hunk annotation shortcut (`H`) to revdiff. Currently annotations are point-based (one comment per line). This feature lets users select a line range vim-style or target an entire hunk, then attach a single annotation to the whole region. Range annotations use an `EndLine` field on the existing `Annotation` struct, render with `┌│└` gutter indicators, and output as `file:10-25` in structured format.

## Validation Commands

- `make test`
- `make lint`
- `make build`

### Task 1: Data Model — Extend Annotation Store for Ranges

Add `EndLine int` field to `Annotation` struct and update all store operations to handle range identity `(Line, EndLine, Type)`. Range annotations use `Type == ""` to avoid key collisions with point annotations on the same start line. `Delete()` gains an `endLine` parameter. New methods `HasRangeCovering()` and `GetRangeCovering()` enable interval lookups. `FormatOutput()` renders ranges as `## file:10-25`. Overlap prevention in `Add()` rejects ranges that intersect existing ranges in the same file.

- [x] Add `EndLine int` field to `Annotation` struct in `annotation/store.go`
- [x] Add `IsRange() bool` method on `Annotation`
- [x] Update `find()` to match on `(Line, EndLine, Type)` instead of `(Line, Type)`
- [x] Update `Delete()` signature to `Delete(file, line, endLine, changeType)` and fix all call sites in `ui/annotate.go`
- [x] Add `HasRangeCovering(file, lineNum) bool` — true if any range annotation covers the line
- [x] Add `GetRangeCovering(file, lineNum) (Annotation, bool)` — returns covering range annotation
- [x] Add overlap rejection in `Add()` for range annotations
- [x] Update `FormatOutput()` to render range annotations as `## file:10-25`
- [x] Add tests: range `Add`/`Delete`/`find`, `IsRange()`, `HasRangeCovering` boundaries, overlap rejection, `FormatOutput` with mixed point+range+file-level, single-line selection collapses to point

### Task 2: Keymap & Visual Selection Mode

Add `ActionSelectRange` (`V`) and `ActionAnnotateHunk` (`H`) to the keymap system. Add `selecting bool` and `selectAnchor int` fields to `Model`. Add `SelectionBg` color key to the theme system with values for all 5 bundled themes. Implement selection entry (`V` in diff pane), exit (`esc`), and highlight rendering — during selection, lines between anchor and cursor get `SelectionBg` background. `cursorOnAnnotation` stops are disabled while selecting.

- [x] Add `ActionSelectRange Action = "select_range"` and `ActionAnnotateHunk Action = "annotate_hunk"` to `keymap/keymap.go`
- [x] Add default bindings `V → select_range`, `H → annotate_hunk` in keymap defaults
- [x] Add `selecting bool` and `selectAnchor int` fields to `Model` in `ui/model.go`
- [x] Handle `ActionSelectRange` in `handleKey()` — set `selecting=true`, `selectAnchor=diffCursor`; guard against overlays, annotation input, search, dividers, `diffCursor==-1`
- [x] Handle `esc` during selection — clear `selecting` and `selectAnchor`
- [x] Disable `cursorOnAnnotation` stops in `moveDiffCursorDown/Up` when `m.selecting`
- [x] Add `SelectionBg` color key to `theme.go` colorKeys, options struct, `colorFieldPtrs()`, and all 5 bundled theme files
- [x] Add `SelectionHighlight` style in `newStyles()` built from `SelectionBg`
- [x] Render selection highlight in `renderDiffLine()` — when `m.selecting && selStart <= idx <= selEnd`, override line background with `SelectionBg`
- [x] Apply selection highlight in `renderCollapsedDiff()` for collapsed mode
- [x] Add tests: selection mode entry/exit, selection bounds calculation, selection highlight in render output

### Task 3: Range Annotation Creation

Wire `a`/`enter` during visual selection to create range annotations. Add `startRangeAnnotation(startLine, endLine, startIdx, endIdx)` which computes line numbers from the selection bounds, creates a text input (reusing `newAnnotationInput`), and pre-fills if an existing range annotation matches. `saveRangeAnnotation()` stores the annotation and clears selection state. Single-line selections (anchor == cursor) collapse to a regular point annotation via the existing `startAnnotation()` path.

- [x] Add temporary fields `rangeStartLine int`, `rangeEndLine int` to `Model` for in-progress range annotation input
- [x] Add `startRangeAnnotation(startLine, endLine, startIdx, endIdx) tea.Cmd` in `ui/annotate.go`
- [x] Add `saveRangeAnnotation()` in `ui/annotate.go` — stores `Annotation{File, Line, EndLine, Type:"", Comment}`, clears selection + annotating state, refreshes tree filter
- [x] Wire `ActionConfirm` / `a` during `m.selecting` to: collapse single-line to `startAnnotation()`, else call `startRangeAnnotation()`
- [x] Update `cancelAnnotation()` to also clear `selecting`/`selectAnchor`/`rangeStartLine`/`rangeEndLine`
- [x] Pre-fill text input when editing existing range annotation (match on `Line, EndLine`)
- [x] Add tests: `startRangeAnnotation` → `saveRangeAnnotation` lifecycle, single-line collapse to point, pre-fill existing range

### Task 4: Range Annotation Rendering

Render saved range annotations in the diff view. Build `rangeRenderInfo` (start/end diffLines indices + comment) before the render loop. Show `┌│└` gutter indicators on lines within a range using `AnnotationLine` style. Render the annotation comment after the last line of the range with `💬 [lines N-M]` prefix. Update `cursorViewportY()` to account for range annotation rows at range ends. Update `moveDiffCursorDown/Up` to stop on range annotation sub-rows at the last line of a range.

- [x] Add `rangeRenderInfo` struct with `startIdx`, `endIdx`, `comment` fields
- [x] Add `buildRangeAnnotations() []rangeRenderInfo` — scan store + diffLines to map range annotations to diffLines indices
- [x] Add `rangeGutterFor(idx, ranges) string` — returns `┌`, `│`, `└`, or `""` for the given diffLines index
- [x] Integrate gutter indicator into `renderDiffLine()` — prepend range gutter before line number gutter
- [x] Integrate gutter indicator into `renderWrappedDiffLine()` for wrap mode
- [x] Render range annotation comment after last line in `renderAnnotationOrInput()` — check `rangeEndsAt` set, use `💬 [lines N-M]` prefix
- [x] Update `cursorViewportY()` — add range annotation rows at range-end indices (parallel to existing point annotation accounting)
- [x] Update `moveDiffCursorDown()` — stop on range annotation sub-row at range-end line
- [x] Update `moveDiffCursorUp()` — land on range annotation sub-row when moving up past range-end line
- [x] Apply range gutter in `renderCollapsedDiff()` for collapsed mode
- [x] Add tests: `buildRangeAnnotations` mapping, gutter indicator assignment (first/middle/last), `cursorViewportY` with ranges, cursor movement through range sub-rows

### Task 5: Hunk Annotation Shortcut

Wire `ActionAnnotateHunk` (`H` key) to auto-select the current hunk and open annotation input. Uses existing `findHunks()` and `hunkStartFor()` to determine hunk boundaries, scans forward to find hunk end, then calls `startRangeAnnotation()`. No-op when cursor is on a context line (not in a hunk).

- [x] Add `annotateHunk() tea.Cmd` in `ui/annotate.go` — find hunk start via `hunkStartFor()`, scan forward for hunk end, compute line numbers, call `startRangeAnnotation()`
- [x] Add `hunkEndFor(startIdx int) int` helper — returns last diffLines index of the hunk containing `startIdx`
- [x] Handle `ActionAnnotateHunk` in `handleKey()` — guard same as `ActionSelectRange` (diff pane, no overlays), call `annotateHunk()`
- [x] Add tests: hunk boundary detection, hunk annotation creates correct range, no-op on context line

### Task 6: Integration — Annotation List, Delete, Status Bar, Help

Wire range annotations through all remaining UI surfaces. Annotation list (`@`) displays ranges as `file:10-25` and jumps to range start. `deleteAnnotation()` handles range annotations when `cursorOnAnnotation` at range end. Status bar shows `▋` icon during selection and `SEL: N lines` count. Help overlay includes `V` and `H` bindings in the Annotations section.

- [x] Update `formatAnnotListItem()` in `ui/annotlist.go` — check `IsRange()`, format as `basename:N-M  comment`
- [x] Update `jumpToAnnotation()` — for ranges, position cursor at start line of range via `findDiffLineIndex(a.Line, ...)`
- [x] Update `deleteAnnotation()` — detect range annotation at cursor (via `GetRangeCovering`), call `Delete(file, line, endLine, type)`
- [x] Update `positionOnAnnotation()` to handle range annotations (find range-end line for `cursorOnAnnotation`)
- [x] Add `▋` selection mode icon in `statusModeIcons()` when `m.selecting`
- [x] Show `SEL: N lines` in status bar left side during selection (count visible lines in selection range)
- [x] Add `V select range` and `H annotate hunk` to help overlay annotations section via `HelpSections()`
- [x] Update all 5 bundled theme TOML files with `SelectionBg` value (if not done in Task 2)
- [x] Add tests: annotation list format for ranges, delete range annotation, status bar selection indicator

### Task 7: Documentation

Update all documentation surfaces to reflect range and hunk annotations: keybindings, output format, usage examples.

- [x] Update README.md — add range/hunk annotation docs to keybindings table, output format section, and usage examples
- [x] Update `site/docs.html` — mirror README changes
- [x] Update plugin reference docs in `.claude-plugin/skills/revdiff/references/usage.md` — add `V`/`H` keybindings and range output format
- [x] Update `.claude-plugin/skills/revdiff/references/config.md` — add `SelectionBg` color key
