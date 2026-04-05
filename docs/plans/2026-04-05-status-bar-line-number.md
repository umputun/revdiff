# Status Bar Line Number Indicator

## Overview
- Add a `L:42/380` indicator to the status bar showing the current cursor's line number and total lines in the file
- Uses new line number for context/add lines, old line number for removed lines (via existing `diffLineNum()`)
- Total is the max line number across old/new in the diff
- Positioned after hunk info: `filename | +X/-Y | hunk 2/5 | L:42/380 | search matches`
- Degrades gracefully on narrow terminals following existing patterns

## Context (from discovery)
- Files/components involved:
  - `ui/model.go` — `statusBarText()` (line 799), `statusSegmentsNoSearch()` (line 1002), `statusSegmentsMinimal()` (line 1014), `hunkSegment()` (line 892)
  - `ui/annotate.go` — `diffLineNum()` (line 216) — already returns the correct line number per change type
  - `ui/model_test.go` — existing status bar tests at lines 831, 1087, 1119, 1145, 2816
- Related patterns: `hunkSegment()` and `searchSegment()` return formatted strings or empty; caller appends if non-empty
- Dependencies: none new — uses existing `diffLineNum()`, `diffLines`, `diffCursor`

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task (see Development Approach above)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Add lineNumberSegment helper
- [x] add `lineNumberSegment()` method to Model in `ui/model.go` (after `hunkSegment`, ~line 909)
  - return `""` when `m.focus != paneDiff` or cursor out of range or on a divider line
  - compute current line via `m.diffLineNum(m.diffLines[m.diffCursor])`
  - compute total as max of OldNum/NewNum across all `m.diffLines`
  - format as `"L:%d/%d"` (e.g. `L:42/380`)
- [x] write tests for `lineNumberSegment` — context line, add, remove, divider, empty diffLines, tree focus
- [x] run tests — must pass before next task

### Task 2: Integrate into statusBarText and degradation
- [x] in `statusBarText()` (line 824), append `lineNumberSegment()` after hunk segment and before search segment
- [x] in `statusSegmentsNoSearch()` (line 1002), append `lineNumberSegment()` after hunk segment
- [x] line number drops at `statusSegmentsMinimal()` level (no change needed — it's already excluded)
- [x] write tests: status bar contains line number, degradation drops it at minimal level
- [x] run tests — must pass before next task

### Task 3: Verify acceptance criteria
- [ ] verify line number shows for context, add, and remove lines
- [ ] verify divider lines show no line number
- [ ] verify line number not shown when focus is on tree pane
- [ ] run full test suite (unit tests)
- [ ] run linter — all issues must be fixed

### Task 4: [Final] Update documentation
- [ ] update README.md if needed (add L:N/M to status bar description if documented)
- [ ] update CLAUDE.md if new patterns discovered

## Technical Details

**Segment format**: `L:42/380` — compact, consistent with `hunk X/Y` style

**Line number source**: `diffLineNum(dl)` in `ui/annotate.go:216-222`:
- `ChangeRemove` → `dl.OldNum`
- everything else → `dl.NewNum`

**Total computation**: scan `m.diffLines` for max of `OldNum` and `NewNum`. This gives the highest line number visible in the diff, which serves as the denominator.

**Degradation order** (left segments, widest to narrowest):
1. Full: filename | stats | hunk | **line number** | search
2. No search: filename | stats | hunk | **line number**
3. Minimal: filename | stats (line number dropped)
4. Truncated filename: ...name | stats

## Post-Completion
**Manual verification**:
- Open revdiff on a multi-file diff, navigate with j/k, verify L:N/M updates in status bar
- Test on narrow terminal to confirm graceful degradation
- Verify line number disappears when switching to tree pane
