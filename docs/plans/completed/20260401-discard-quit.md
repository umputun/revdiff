# Discard and Quit (Q hotkey)

## Overview
- Add a "discard and quit" hotkey (`Q`) that exits revdiff without outputting any annotations
- When annotations exist, show an inline confirmation prompt in the status bar before discarding
- When no annotations exist, `Q` behaves identically to `q` (just exits, nothing to discard)
- The confirmation prompt is suppressible via `--no-confirm-discard` CLI flag / env / config
- Solves the problem of accidentally sending annotations back to the calling process when the user just wants to exit

## Context (from discovery)
- Key files: `ui/model.go` (Model struct, handleKey, statusBarText), `cmd/revdiff/main.go` (options, run)
- `Store.Count()` already exists for checking annotation count
- Existing accessor pattern: `Store()` returns private field, same pattern for `Discarded()`
- Status bar already has context-sensitive text via `statusBarText()`
- Existing `--no-*` flag pattern: `--no-colors`, `--no-status-bar`

## Solution Overview
- `Q` (shift+q) quits without annotations; `q` continues to quit with annotations
- If annotations exist and `--no-confirm-discard` is not set, status bar shows `"discard N annotations? [y/n]"`
- `y` or second `Q` confirms discard, `n`/`Esc` cancels back to normal mode
- If no annotations or `--no-confirm-discard` is set, `Q` quits immediately
- `main.go` checks `m.Discarded()` before calling `FormatOutput()`

## Technical Details
- New Model fields: `confirmingDiscard bool`, `discarded bool`, `noConfirmDiscard bool`
- New ModelConfig field: `NoConfirmDiscard bool`
- New CLI option: `--no-confirm-discard` (env: `REVDIFF_NO_CONFIRM_DISCARD`, ini: `no-confirm-discard`)
- New accessor: `func (m Model) Discarded() bool`
- Status bar in confirming state: `"discard N annotations? [y/n]"` (replaces normal hints)
- `Q` during annotation input mode is ignored (same as other navigation keys)

## Development Approach
- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change
- maintain backward compatibility

## Testing Strategy
- **unit tests**: required for every task
- Test Q with no annotations (immediate quit, discarded=true)
- Test Q with annotations and confirmation (y confirms, n/Esc cancels)
- Test Q with annotations and noConfirmDiscard (immediate quit)
- Test Q during annotation input (ignored)
- Test Discarded() accessor
- Test main.go skips output when discarded
- Test status bar text during confirmation

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Add discard state to Model and CLI option

**Files:**
- Modify: `ui/model.go`
- Modify: `cmd/revdiff/main.go`

- [x] add `discarded bool`, `noConfirmDiscard bool` fields to Model struct (`confirmingDiscard` deferred to Task 2 to avoid unused-field lint error)
- [x] add `NoConfirmDiscard bool` to ModelConfig
- [x] add `Discarded() bool` accessor method on Model
- [x] wire `noConfirmDiscard` in NewModel from ModelConfig
- [x] add `NoConfirmDiscard` option to `options` struct in main.go (`--no-confirm-discard`, env `REVDIFF_NO_CONFIRM_DISCARD`, ini `no-confirm-discard`)
- [x] pass `NoConfirmDiscard` from options to ModelConfig in `run()`
- [x] write tests in `ui/model_test.go` for Discarded() accessor (default false, set true)
- [x] write test in `cmd/revdiff/main_test.go` for `--no-confirm-discard` flag parsing
- [x] run `go test ./...` - must pass before task 2

### Task 2: Handle Q keypress and confirmation flow

**Files:**
- Modify: `ui/model.go`

- [x] add `Q` case in handleKey: if no annotations or noConfirmDiscard, set `discarded=true` and return `tea.Quit`
- [x] if annotations exist and confirm required, set `confirmingDiscard=true` and return
- [x] ignore `Q` when `m.annotating` is true (annotation input mode)
- [x] add confirmation key handling: when `confirmingDiscard` is true, `y` or `Q` sets `discarded=true` and returns `tea.Quit`, `n`/`Esc` sets `confirmingDiscard=false`
- [x] block other keys while `confirmingDiscard` is true (only y/Q/n/Esc accepted); confirmation blocking applies only in `handleKey`, non-key messages (WindowSizeMsg etc.) are handled normally
- [x] write tests for Q with no annotations (immediate quit, discarded=true)
- [x] write tests for Q with annotations (enters confirming state)
- [x] write tests for y during confirmation (quits with discarded=true)
- [x] write tests for n and Esc during confirmation (cancels back to normal)
- [x] write test for second Q during confirmation (confirms discard)
- [x] write test for Q during annotation input (ignored)
- [x] write test for Q with noConfirmDiscard and annotations (immediate quit)
- [x] run `go test ./...` - must pass before task 3

### Task 3: Update status bar and output handling

**Files:**
- Modify: `ui/model.go`
- Modify: `cmd/revdiff/main.go`

- [x] update `statusBarText()` to show `"discard N annotations? [y/n]"` when `confirmingDiscard` is true
- [x] add `[Q] discard` hint to normal status bar text (both tree and diff pane hints)
- [x] in main.go `run()`, check `m.Discarded()` before `FormatOutput()` — if true, return nil (skip output)
- [x] write test for status bar text during confirmation state
- [x] write test for status bar showing Q hint in normal mode
- [x] write test verifying main.go skips output when discarded (if feasible with existing test patterns)
- [x] run `go test ./...` - must pass before task 4

### Task 4: Verify acceptance criteria
- [x] verify Q with no annotations exits silently (no output)
- [x] verify Q with annotations shows confirmation, y discards, n cancels
- [x] verify --no-confirm-discard skips prompt
- [x] verify q still works as before (outputs annotations)
- [x] verify Q is ignored during annotation text input
- [x] run full test suite: `go test ./...`
- [x] run linter: `golangci-lint run`
- [x] run formatters: `~/.claude/format.sh`

### Task 5: [Final] Update documentation
- [x] update README.md key bindings table (add Q)
- [x] update README.md options table (add --no-confirm-discard)
- [x] update CLAUDE.md if needed
- [x] update plugin reference docs (`.claude-plugin/skills/revdiff/references/usage.md`, `config.md`)
- [x] move this plan to `docs/plans/completed/`
