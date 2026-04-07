# Hunk Keyword Expansion in Annotation Output

## Overview
When annotation text contains keywords like "hunk" or "block", automatically expand the output header to include the full hunk line range. For example, if a user annotates line 43 with "refactor this hunk", the output becomes `## file.go:43-67 (+)` instead of `## file.go:43 (+)`. This gives AI consumers the range context without any new UI modes.

## Context
- `annotation/store.go` - Annotation struct, FormatOutput() builds `## file:line (type)` headers
- `ui/diffview.go` - findHunks() returns indices of hunk starts in diffLines
- `ui/collapsed.go` - hunkStartFor() finds which hunk contains a given line index
- `ui/annotate.go` - diffLineNum() maps DiffLine to display line number
- Keywords to detect: "hunk", "block" (case-insensitive, whole word)

## Development Approach
- **testing approach**: regular (code first, then tests)
- small change, ~30-50 lines of new code
- no UI changes, no new modes, no rendering changes
- **CRITICAL: every task MUST include new/updated tests**

## Solution Overview
Add an `EndLine` field to `Annotation`. When creating an annotation, if the comment text contains hunk keywords and the annotated line is inside a change hunk, compute the hunk's end line and set `EndLine`. `FormatOutput` renders `file:start-end (type)` when `EndLine > 0`.

## Implementation Steps

### Task 1: Add EndLine to Annotation and update FormatOutput

**Files:**
- Modify: `annotation/store.go`
- Modify: `annotation/store_test.go`

- [ ] add `EndLine int` field to `Annotation` struct (0 means no range)
- [ ] update `FormatOutput()` to render `file:line-endline (type)` when `EndLine > 0`
- [ ] write test: annotation with EndLine=0 produces `## file:43 (+)` (unchanged)
- [ ] write test: annotation with EndLine=67 produces `## file:43-67 (+)`
- [ ] write test: file-level annotations (Line=0) ignore EndLine
- [ ] run `make test && make lint`

### Task 2: Detect hunk keywords and populate EndLine

**Files:**
- Modify: `ui/annotate.go`
- Modify: `ui/model_test.go`

- [ ] add `hunkEndLine(idx int) int` method that finds the last line of the hunk containing diffLines[idx]
- [ ] in the annotation creation path, after building the Annotation, check if comment contains hunk keywords (case-insensitive "hunk" or "block" as whole words)
- [ ] if keyword found and line is in a change hunk, set `EndLine` to hunk end line number via `hunkEndLine`
- [ ] write test: annotation with "refactor this hunk" gets EndLine populated
- [ ] write test: annotation with "this is fine" does NOT get EndLine
- [ ] write test: annotation on context line (not in hunk) does NOT get EndLine even with keyword
- [ ] run `make test && make lint`

### Task 3: Verify and document

- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] manual test: annotate a line with "fix this hunk", verify output shows range
- [ ] update README.md output format section to mention range expansion
- [ ] update site/docs.html output format section
- [ ] update CLAUDE.md if needed
- [ ] move this plan to `docs/plans/completed/`
