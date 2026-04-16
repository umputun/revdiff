# External Editor Annotations

## Overview
Add `$EDITOR` integration to annotation input so users can write multi-line annotations (numbered lists, pasted DB slices, longer explanations) without swapping the single-line `textinput` widget for a full textarea.

**Trigger**: discussion [#111](https://github.com/umputun/revdiff/discussions/111). The 500→8000 CharLimit bump (commit 6395e6d) addressed length. This plan addresses structure: the single-line widget flattens pasted newlines and has no way to produce multi-line content. Crazyproger proposed `Ctrl+E` opening `$EDITOR`; umputun accepted the approach.

**How it integrates**:
- Annotation mode (`a` / `A` / `Enter`) still starts the same single-line textinput for quick notes
- While annotating, `Ctrl+E` pauses bubbletea via `tea.ExecProcess`, opens `$EDITOR` on a temp file containing the current input text, and — on editor save — commits the annotation directly to the store (bypasses the single-line widget)
- Multi-line comments flow through `renderWrappedAnnotation` (new `\n`-aware path), `FormatOutput` (already handles `\n` via its `## header\n<body>\n` framing, just needs test coverage), and `annotlist.formatItem` (flatten for the single-row overlay display)

## Context (from discovery)

- **Core input path**: `app/ui/annotate.go` — `newAnnotationInput` (factory), `startAnnotation` / `startFileAnnotation` (entry points), `handleAnnotateKey` (Enter=save, Esc=cancel, default=forward to `textinput.Update`), `saveAnnotation` (reads `m.annot.input.Value()` → `store.Add`)
- **Render path**: `app/ui/diffview.go:598` `renderWrappedAnnotation` wraps the comment as one blob; `app/ui/annotate.go:299` `wrappedAnnotationLineCount` computes visual rows via `wrapContent`. Neither splits on `\n`
- **Store**: `app/annotation/store.go` — `Annotation.Comment` is a plain `string`; `FormatOutput` uses `fmt.Fprintf("## %s:%d (%s)\n%s\n", ...)` — embedded `\n` in `Comment` flows through naturally, `##` acts as next-entry delimiter
- **Overlay list**: `app/ui/overlay/annotlist.go:82` `formatItem` single-row display, truncates via `ansi.Truncate` — must flatten `\n` before measuring/truncating
- **Keymap**: `app/keymap/keymap.go` — Action constants + `defaultBindings`, user overrides via `~/.config/revdiff/keybindings`. `ctrl+`-prefixed keys already supported (`ctrl+d`, `ctrl+u` exist). `ctrl+e` currently unbound
- **Bubbletea**: `vendor/.../bubbletea/exec.go:50` `tea.ExecProcess` — handles `ReleaseTerminal`/`RestoreTerminal` around the spawned command, accepts an `*exec.Cmd` and a completion callback returning `tea.Msg`. Exactly the primitive this needs

Related patterns observed:
- `fileLoadedMsg` / `blameLoadedMsg` are precedent for async-result messages routed through the top `Update` switch (`app/ui/model.go:525-529`)
- Moq mocks in `app/ui/mocks/` for UI-side interfaces

Dependencies identified:
- Standard library `os`, `os/exec`, `os/user` (none new)
- `github.com/charmbracelet/bubbletea.ExecProcess` (already vendored)

## Development Approach
- **testing approach**: Regular (code first, then tests per task)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**:
  - tests are a required deliverable, not optional
  - unit tests for new functions/methods
  - new test cases for new code paths (e.g., multi-line render, editor callback)
  - both success and error scenarios (e.g., editor exit with non-zero status, empty file, temp-file write failure)
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run `make test` and `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` after each task
- maintain backward compatibility — existing single-line typing flow, existing saved annotations, existing output format all unchanged for callers that don't embed `\n`

## Testing Strategy
- **unit tests**: required every task
  - render and wrap logic tested by constructing `Model` directly (matches existing `annotate_test.go`, `diffview_test.go` patterns)
  - editor integration: extract the command-construction and temp-file handling into testable helpers; the `tea.ExecProcess` wrapping itself is thin and exercised via integration behavior (msg dispatch, store update on success)
  - `editorFinishedMsg` handler tested by constructing the msg directly and feeding into `Update` — avoids spawning a real editor
  - use `t.Setenv` to control `EDITOR` / `VISUAL` in tests
- **e2e tests**: revdiff has no Playwright-style e2e harness; manual validation is in Post-Completion

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

**Key mental model**: `$EDITOR` replaces the input widget for multi-line work. The single-line textinput remains the fast path for one-liners.

**Flow** (line-level annotation example):
1. User presses `a` → `startAnnotation` focuses textinput, pre-fills with existing comment if any
2. User presses `Ctrl+E` → `handleAnnotateKey` resolves to `ActionOpenEditor` → `openEditor` helper writes current input value to a `.md` temp file, returns `tea.ExecProcess` wrapping `$EDITOR <tempfile>` with a callback that reads the file back and emits `editorFinishedMsg`
3. Bubbletea pauses, editor takes over the tty, user writes multi-line content, saves, quits
4. Bubbletea resumes, `editorFinishedMsg` lands in `Update`, handler reads the content, calls `saveAnnotation` with the content directly (bypassing the textinput), cleans up the temp file, exits annotation mode
5. Subsequent renders show the multi-line annotation below the diff line, each line on its own visual row

**Design decisions**:
- **Save directly on editor exit** (user-approved): the editor is now the input — no round-trip back to the textinput (which would flatten newlines anyway). Matches crazyproger's request and umputun's direction.
- **Temp file suffix `.md`**: most editors syntax-highlight markdown, benefiting users who write list-style comments.
- **Editor resolution**: `$EDITOR` → `$VISUAL` → `vi`. Standard Unix convention; no config needed.
- **Key binding**: hard-coded `tea.KeyCtrlE` inside `handleAnnotateKey` — only active during annotation input, no conflict with the general keymap. Kept out of the `Action`/`defaultBindings` system for now to minimize surface area; remapping can be added later if there is demand.
- **Empty result**: if editor exits cleanly with an empty file, route through `cancelAnnotation()` — do NOT touch the store. This preserves any pre-existing annotation on that line (user re-annotated, accidentally quit editor without saving, should not silently lose the original). Matches `saveAnnotation`'s "empty value = cancel" behavior.
- **Non-zero editor exit**: log and return to annotation-input state with existing content preserved; do not discard user work.
- **Multi-line rendering**: first visual row gets the `💬 ` (or `💬 file: `) prefix, continuation lines get a 2-space (or 8-space for file-level) alignment gutter so the comment body lines up. Each logical line is then word-wrapped individually to `diffContentWidth()-1`.

## Technical Details

### New message type
```go
// in app/ui/annotate.go (or a new editor.go in the same package)
type editorFinishedMsg struct {
    content   string
    err       error
    tempPath  string // for cleanup regardless of err
    fileLevel bool   // captured at open time
    line      int    // captured at open time (line-level only)
    changeType string
}
```
Captured fields freeze the annotation target at open time so the user scrolling during editor use doesn't misroute the save.

### Editor resolution
```go
func resolveEditor() []string {
    for _, env := range []string{"EDITOR", "VISUAL"} {
        if v := strings.TrimSpace(os.Getenv(env)); v != "" {
            return strings.Fields(v)
        }
    }
    return []string{"vi"}
}
```
Returning `[]string` handles `EDITOR="code --wait"` — the caller uses `exec.Command(parts[0], append(parts[1:], tempPath)...)`.

### Temp file
- Path: `os.CreateTemp("", "revdiff-annot-*.md")`
- Written with current `m.annot.input.Value()` (preserves any text user typed before pressing Ctrl+E)
- Cleaned up in the callback regardless of error path

### Multi-line render (in `renderWrappedAnnotation`)
- `strings.Split(text, "\n")` into logical lines
- First logical line carries the existing `"💬 "` prefix (already baked into `text`)
- For continuation logical lines, drop the emoji, pad with 2 spaces (line-level) or 8 (file-level, to align past `💬 file: `)
- Each logical line independently wrapped via `wrapContent`
- Cursor column: `cursor` on very first visual row, space on all subsequent visual rows

### Multi-line row count (in `wrappedAnnotationLineCount`)
- Sum of `wrappedRowsFor(logicalLine)` across all `\n`-split segments
- Minimum 1 to preserve existing behavior for empty annotations

### Annotation list display (`annotlist.formatItem`)
- Replace `\n` with `⏎ ` (or literal space) in the displayed comment before `ansi.Truncate` — single-row constraint remains
- Truncation already caps width; multi-line comments display as "first line ⏎ second line…" until truncation kicks in

### Keybinding
- `tea.KeyCtrlE` handled directly inside `handleAnnotateKey`, before the default branch that forwards to `textinput.Update`
- Only active while `m.annot.annotating == true`, so it does not shadow any global binding
- Not registered as a `keymap.Action` — single hard-coded binding keeps the change focused
- Placeholder text for the annotation input is extended to `"annotation... (Ctrl+E for editor)"` so the feature is discoverable without a help overlay entry

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): all code, tests, and documentation within this repo
- **Post-Completion** (no checkboxes): manual terminal testing (Ctrl+E in an actual revdiff session), verification that `$EDITOR` handoff preserves scrollback/altscreen across common terminals (kitty, iTerm2, tmux)

## Implementation Steps

### Task 1: Multi-line aware annotation rendering

**Files:**
- Modify: `app/ui/diffview.go` (`renderWrappedAnnotation`)
- Modify: `app/ui/annotate.go` (`wrappedAnnotationLineCount`)
- Modify: `app/ui/diffview_test.go`
- Modify: `app/ui/annotate_test.go`

- [x] refactor `renderWrappedAnnotation` to split `text` on `\n` into logical lines. For each logical line: wrap to `diffContentWidth() - 1 - indentWidth` (where `indentWidth` is 0 for the very first logical line because it already carries the emoji prefix, `emojiPrefixWidth` for subsequent logical lines) then render each resulting visual row with the correct leading glyph (cursor on the very first visual row of the whole annotation, space elsewhere) and the correct prefix (emoji on first visual row of first logical line, indent on first visual row of continuation logical lines, plain space for wrapped sub-rows within a logical line)
- [x] refactor `wrappedAnnotationLineCount` to sum `ceil(len(logicalLine) / wrapWidth_for_that_line)` across `\n`-split segments, using the same reduced wrap width for continuation logical lines; keep minimum 1 for empty input
- [x] add test case to `diffview_test.go` covering a 3-logical-line annotation: verify output contains each logical line, correct cursor on first row only, correct indent on continuation lines
- [x] add test case to `diffview_test.go` covering a 2-logical-line annotation where each logical line further word-wraps
- [x] add test cases to `annotate_test.go` for `wrappedAnnotationLineCount` with: single-line input, multi-line with no wrap, multi-line with wrap on inner lines, file-level annotation with newlines
- [x] run `go test ./app/ui/...` — must pass before Task 2

### Task 2: Editor invocation helper and temp-file lifecycle

**Files:**
- Create: `app/ui/editor.go` (new — keeps `annotate.go` focused on input lifecycle)
- Create: `app/ui/editor_test.go`

- [x] define `editorFinishedMsg` struct (fields: `content string`, `err error`, `tempPath string`, `fileLevel bool`, `line int`, `changeType string`)
- [x] add unexported `resolveEditor() []string` helper: reads `EDITOR`, falls back to `VISUAL`, falls back to `[]string{"vi"}`; splits on `strings.Fields` so `"code --wait"` works
- [x] add unexported `writeAnnotTempFile(content string) (string, error)` helper: `os.CreateTemp("", "revdiff-annot-*.md")`, writes `content`, returns path
- [x] extract the callback body as an unexported standalone function: `readEditorResult(tempPath string, fileLevel bool, line int, changeType string, runErr error) editorFinishedMsg` — reads the file, deletes it regardless of errors, returns a fully-populated `editorFinishedMsg`. This is directly testable without `tea.ExecProcess`.
- [x] add `(m *Model) openEditor() tea.Cmd` method: captures current `m.annot.input.Value()`, `m.annot.fileAnnotating`, target line/type; writes temp file; returns `tea.ExecProcess(exec.Command(editor[0], append(editor[1:], tempPath)...), adapter)` where `adapter` is a one-line closure over the captured context that calls `readEditorResult(tempPath, fileLevel, line, changeType, err)`
- [x] write tests for `resolveEditor` using `t.Setenv` covering all three paths (EDITOR set, VISUAL fallback, vi default); also covers whitespace splitting
- [x] write tests for `writeAnnotTempFile`: returns readable file with exact content; cleanup via `os.Remove` works; empty content is still written
- [x] write tests for `readEditorResult` directly (no `tea.ExecProcess`): pre-existing temp file with content → msg has expected content + no err, file is removed; `runErr` non-nil → msg has err, file is still removed; missing temp file → msg carries read error but does not panic
- [x] run `go test ./app/ui/...` — must pass before Task 3

### Task 3: Wire `Ctrl+E` into annotation input flow

**Files:**
- Modify: `app/ui/annotate.go` (`newAnnotationInput` placeholder text, `handleAnnotateKey`, `saveAnnotation`)
- Modify: `app/ui/model.go` (`Update` msg switch)
- Modify: `app/ui/annotate_test.go`
- Modify: `app/ui/model_test.go`

- [x] extend the input placeholder in `newAnnotationInput` (or at the two call sites) to include "(Ctrl+E for editor)" so the binding is discoverable; adjust the `prefixWidth` / width math if the placeholder length affects layout (likely not — placeholder is only visible when empty)
- [x] in `handleAnnotateKey`, add `case tea.KeyCtrlE:` before the default branch; call `m.openEditor()` and return its `tea.Cmd`; leave `m.annot.annotating = true` so the mode survives the exec suspend/resume and `editorFinishedMsg` routes correctly
- [x] extract the common save-from-text logic out of `saveAnnotation` into `saveComment(text string, fileLevel bool, line int, changeType string)` with **explicit arguments** (do NOT read model state inside `saveComment` — that lets both the Enter-key path and the editor-finished path reuse it without temporal coupling)
- [x] keep existing `saveAnnotation()` as a thin wrapper that reads model state and calls `saveComment` with the current input value plus captured-at-call-time target fields
- [x] add `case editorFinishedMsg:` to `Update` (near `fileLoadedMsg` at `model.go:525`): on `msg.err == nil && msg.content != ""`, call `saveComment(msg.content, msg.fileLevel, msg.line, msg.changeType)` and set `annotating = false`; on `msg.err == nil && msg.content == ""` (editor quit without saving or saved empty file), route through `cancelAnnotation()` — do NOT touch the store, preserving any existing annotation at that line; on `msg.err != nil`, `log.Printf` and leave annotation mode open with the existing input value untouched so the user can retry or press Esc
- [x] write `annotate_test.go` case: set `annotating=true`, send `tea.KeyMsg{Type: tea.KeyCtrlE}`, assert returned cmd is non-nil and a temp file was created with current input content
- [x] write `model_test.go` (or `annotate_test.go`) case feeding `editorFinishedMsg{content: "line1\nline2", line: 3, changeType: "+"}` directly into `Update`; assert `store` contains an annotation with `Comment == "line1\nline2"` and `annotating` is false
- [x] write test for error path: `editorFinishedMsg{err: errors.New(...)}`; assert `store` is unchanged, `annotating` remains true, input value preserved
- [x] write test for empty-content path with **pre-existing annotation on target line**: store has "old comment" on line 3; feed `editorFinishedMsg{content: "", fileLevel: false, line: 3, changeType: "+"}`; assert the existing "old comment" is still in the store (empty editor result must not silently delete a pre-existing annotation)
- [x] run `go test ./app/ui/...` — must pass before Task 4

### Task 4: Multi-line output and annotation list compatibility

**Files:**
- Modify: `app/annotation/store_test.go`
- Modify: `app/ui/overlay/annotlist.go` (`formatItem`)
- Modify: `app/ui/overlay/annotlist_test.go`

- [ ] audit `FormatOutput` with multi-line `Comment`: the existing `## header\n%s\n` framing should already be correct; confirm by adding test cases in `store_test.go` for line-level multi-line, file-level multi-line, and mixed single/multi-line annotations; check `##` still delimits unambiguously
- [ ] update `formatItem` in `annotlist.go`: before width measurement / truncation, replace `\n` in the comment with `" ⏎ "` (or literal space — verify which reads better during Task 6 manual smoke)
- [ ] add `annotlist_test.go` case: comment with `\n` renders as a single row with no literal newlines in output; truncation still applies
- [ ] run `go test ./app/annotation/... ./app/ui/overlay/...` — must pass before Task 5

### Task 5: Update documentation

**Files:**
- Modify: `README.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff/references/usage.md`
- Modify: `site/docs.html`
- Review: `CLAUDE.md` (project) — add note only if a new project convention emerged

- [ ] update README.md annotation section: document `Ctrl+E` opens `$EDITOR`, `$EDITOR`→`$VISUAL`→`vi` resolution, multi-line content is preserved in output, empty editor exit cancels
- [ ] mirror the README change into `.claude-plugin/skills/revdiff/references/usage.md` (byte-identical wording per CLAUDE.md sync rule)
- [ ] mirror into `plugins/codex/skills/revdiff/references/usage.md`
- [ ] update `site/docs.html` annotation section to match README
- [ ] review `CLAUDE.md` — add a Gotcha entry only if the editor-exec pattern is reusable project knowledge (likely yes: "long-running external commands use `tea.ExecProcess`; capture target state at spawn time so cursor movement during exec doesn't misroute the result")
- [ ] no test step for this task — documentation-only

### Task 6: Acceptance verification

- [ ] verify all requirements from Overview are implemented
- [ ] verify edge cases: empty editor result = cancel; editor non-zero exit = log + keep input; pre-existing single-line-only annotations still render correctly
- [ ] run full test suite: `make test`
- [ ] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [ ] run formatters: `~/.claude/format.sh`
- [ ] manual smoke: build (`make build`), run against a real repo, annotate a line with `Ctrl+E`, save multi-line content, quit, verify output contains newlines and `##` delimiter works
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention or external systems — no checkboxes, informational only.*

**Manual verification**:
- run revdiff on a real diff, press `Ctrl+E` during annotation, verify:
  - terminal hands over cleanly to `$EDITOR` (no cursor/altscreen corruption)
  - on editor save + quit, bubbletea resumes with the annotation persisted and rendered multi-line
  - Ctrl+C inside editor (if supported by the editor) cancels cleanly
  - test with at least: `vim`, `nano`, and one GUI editor with a `--wait` flag if available (e.g., `code --wait`)
- verify in kitty (primary dev terminal), iTerm2, and tmux — `tea.ExecProcess` is expected to work in all three, but worth confirming no altscreen leaks
- verify annotation list overlay (`@`) shows multi-line comments with a compact one-row representation

**Documentation surfaces to watch**:
- if users report confusion, revisit wording in README / usage.md about "editor exit = save" vs "empty file = cancel"

**Out of scope (explicitly)**:
- textarea widget swap (superseded by this editor approach)
- pre-existing Enter-on-paste behavior in the single-line textinput (users wanting multi-line now have Ctrl+E; revisit only if complaints persist)
- editor auto-detection beyond `$EDITOR`/`$VISUAL`/`vi` (e.g., trying `nvim` or `nano` directly) — unnecessary and opinionated
