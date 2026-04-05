# Fix File-Level Annotation Bugs

## Overview
Fix two bugs where file-level annotations (`A` key) behave differently from line-level annotations (`a` key).
Related to #26.

**Bug 1**: file-level annotation input overflows terminal width because `newAnnotationInput`
subtracts 6 chars (for `"💬 "` prefix) but `renderFileAnnotationHeader` renders the wider
`"💬 file: "` prefix (6 extra visible chars).

**Bug 2**: pressing Enter on a file annotation line does nothing. `handleEnterKey` always
calls `startAnnotation()` which returns nil when `diffCursor == -1`. It never dispatches
to `startFileAnnotation()`.

## Context (from discovery)
- `ui/annotate.go:15-22`: `newAnnotationInput` — width calc subtracts 6 for prefix
- `ui/annotate.go:57-62`: `startFileAnnotation` — calls `newAnnotationInput` with same width
- `ui/diffview.go:55-70`: `renderFileAnnotationHeader` — renders `"💬 file: "` (wider prefix)
- `ui/model.go:1138-1162`: `handleEnterKey` — always calls `startAnnotation()` in diff pane
- `ui/annotate.go:212-213`: `cursorOnFileAnnotationLine()` — already exists, checks `diffCursor == -1`

## Development Approach
- **Testing approach**: regular (code first, then tests)
- Both fixes are small and isolated
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**

## Solution Overview
- **Bug 1**: pass a width offset parameter to `newAnnotationInput` so file annotations use a larger
  subtraction. The file prefix `"💬 file: "` is ~12 visible chars vs line prefix `"💬 "` ~6 chars.
- **Bug 2**: in `handleEnterKey`, check `cursorOnFileAnnotationLine()` before falling through
  to `startAnnotation()`. If true, call `startFileAnnotation()` instead.

## Implementation Steps

### Task 1: Fix file-level annotation input width overflow

**Files:**
- Modify: `ui/annotate.go`
- Modify: `ui/diffview.go`
- Modify: `ui/model_test.go`

- [x] change `newAnnotationInput` to accept a `prefixWidth int` parameter instead of hardcoded 6
- [x] update `startAnnotation` call to pass 6 (line-level prefix width)
- [x] update `startFileAnnotation` call to pass 12 (file-level prefix width: cursor col + `"💬 file: "` + margin)
- [x] verify the rendered prefix in `renderFileAnnotationHeader` (`"💬 file: "`) matches the offset
- [x] write test: file-level annotation input width is narrower than line-level to account for wider prefix
- [x] run tests — must pass before task 2

### Task 2: Fix Enter key to edit existing file annotation

**Files:**
- Modify: `ui/model.go`
- Modify: `ui/model_test.go`

- [ ] in `handleEnterKey`, add branch before `startAnnotation()`: when `m.cursorOnFileAnnotationLine()`, call `startFileAnnotation()` instead
- [ ] write test: Enter on file annotation line (diffCursor == -1) triggers file annotation edit
- [ ] write test: Enter on file annotation line pre-fills existing annotation text
- [ ] write test: Enter on regular diff line still triggers line annotation (regression check)
- [ ] run tests — must pass before task 3

### Task 3: Verify acceptance criteria

- [ ] verify: file-level annotation input stays within terminal width
- [ ] verify: Enter on existing file annotation opens edit with pre-filled text
- [ ] verify: line-level annotations still work identically
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] run formatters: `make fmt`

### Task 4: [Final] Update documentation

- [ ] update CLAUDE.md if new patterns discovered
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- create a file-level annotation with a long text, verify it doesn't overflow
- navigate to file annotation line, press Enter, verify edit mode opens with pre-filled text
- verify line-level annotations still work as before
