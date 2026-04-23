# Vim-Motion Preset (--vim-motion, off by default)

## Overview

Add an opt-in vim-style motion preset to revdiff, activated via `--vim-motion` CLI flag (and matching config file / env var entries). Addresses issue #138 (vim line movements) and retroactively covers the scope of closed PR #63 (vim viewport scroll + quit aliases).

**Bindings shipped behind the flag:**
- `<N>j` / `<N>k` — move cursor N lines down/up (diff pane, 1-9999)
- `gg` — jump to first line (diff pane)
- `G` / `<N>G` — jump to last line / goto line N (diff pane)
- `zz` — center viewport on cursor (diff pane)
- `zt` — top-align viewport on cursor (diff pane)
- `zb` — bottom-align viewport on cursor (diff pane)
- `ZZ` — quit (alias for `ActionQuit`, pane-agnostic)
- `ZQ` — discard & quit (alias for `ActionDiscardQuit`, pane-agnostic)

**Out of scope** (deliberately):
- `yy` yank — separate concern (clipboard lib + ssh/OSC-52 fallback + scope decisions). Issue #138 may stay open for this, or get a follow-up plan.
- Tree-pane viewport scroll — would require adding `CenterOnCursor`/`TopAlignOnCursor`/`BottomAlignOnCursor` methods to `app/ui/sidepane/Tree`; saved for a later plan if users ask.

**Design constraint**: the feature is self-contained in a new `app/ui/vimmotion.go` file plus minimal additions elsewhere. It does NOT extend the ctrl/alt chord engine from PR #143 — vim-motion is a parallel, orthogonal dispatch layer.

## Context (from discovery)

**Files involved:**
- `app/ui/model.go` — `Model` struct (line ~265 `focus pane`, line 356 `keyState`), `handleKey` (line 711), `dispatchAction(action)` (line 770, takes only action — NOT action+msg as in the completed #143 plan's proposal), `clearChordState` (line 848), `ModelConfig` (line 456)
- `app/ui/view.go` — `transientHint()` (line 105) with existing priority chain: `commits → reload → compact → keys`
- `app/ui/diffnav.go` — `centerViewportOnCursor` (line 220), `topAlignViewportOnCursor` (line 281), `handleDiffAction(action)` (line 424), `handleTreeAction(action)` (line 470); **no `bottomAlignViewportOnCursor` yet**
- `app/ui/search.go` — `startSearch` (line 11)
- `app/ui/annotate.go` — `startAnnotation` (line 53)
- `app/keymap/keymap.go` — `Action` constants, `navigationActions` allowlist (line ~68), `helpEntries` (line ~120), default bindings (line ~178)
- `app/config.go` — options struct with go-flags tags; `Colors`, `VimMotion` will be added as sibling
- `app/renderer_setup.go` — VCS wiring, ModelConfig construction site
- `README.md`, `site/docs.html`, `.claude-plugin/skills/revdiff/references/usage.md`, `plugins/codex/skills/revdiff/references/usage.md` — docs sync per `feedback_revdiff-docs-sync-plugins.md` (byte-identical between the two plugin copies)

**Patterns observed:**
- `keyState` sub-struct on Model is the existing convention for key-dispatch state; `vimState` mirrors it as a sibling
- `handleDiffAction`/`handleTreeAction`/`dispatchAction` all take only `action keymap.Action` — vim-motion dispatch will match this (no `msg` parameter)
- `transientHint()` priority chain is a simple switch — new case appended at the end (lowest priority)
- Chord-second guard inside `handleKey` (added in #143) demonstrates the "intercept before modal" pattern — vim-motion follows the same shape
- `clearChordState()` is a `*Model` pointer-receiver helper; modal-entering paths call it at entry. Vim-motion extends this into `clearPendingInputState()`.

**Architectural validation (from brainstorm):**
- Option A chosen over B (split handlers) and C (extend chord engine). Keeps vim state contained in one file, doesn't pollute keymap engine with mode switches.
- Count accumulator and letter-leader are sub-states of a single interceptor, not separate handlers (brainstorm priority list shows they share clearing lifetime and dispatch target).
- Diff-pane-only for motion and viewport scroll avoids adding unrelated methods to the sidepane Tree type.

## Development Approach

- **Testing approach**: Regular (code first, then tests in same task). Matches the convention used in the #143 plan.
- Make small, focused changes — each task is a single concern.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task.
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions.
- Run `make test` after each task before moving on.
- Run `make lint` before final commit.
- Maintain backward compatibility — when `--vim-motion` is off, behavior is byte-identical to current. All existing single-key bindings continue to work unchanged.

## Testing Strategy

- **Unit tests** (`app/keymap/keymap_test.go`): new action constants appear in `navigationActions` + `helpEntries`.
- **Unit tests** (`app/ui/vimmotion_test.go`): table-driven state-machine tests covering the full priority 1–5 matrix; per-key behavior when vim-motion is on vs off; `clearPendingInputState` resets all vim state.
- **Unit tests** (`app/ui/diffnav_test.go`): `bottomAlignViewportOnCursor` symmetry with top/center; `jumpToLineN` clamps bounds.
- **Integration tests** (`app/ui/model_test.go`): `handleKey` precedence with vim-motion — interceptor position in the flow; pending-reload preempts vim; chord-second preempts vim; vim-motion off path is untouched; modal-entry paths clear vim state.
- **No e2e tests** in this project (revdiff is a TUI; bubbletea integration is tested via Model unit tests).

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

**State** — a new `vimState` sub-struct on `Model`, parallel to `keyState`:

```go
// vimState holds vim-motion preset state: count prefix accumulator and
// pending letter-leader. Distinct from keyState (ctrl/alt chord dispatch);
// the two are orthogonal and run in different guards of handleKey.
type vimState struct {
    count  int    // accumulated count prefix; 0 = none pending
    leader string // pending letter leader: "g", "z", "Z", or ""
    hint   string // transient status-bar hint
}
```

Invariant: `count > 0` and `leader != ""` never coexist. Enforced in interceptor code, not types (simpler).

**Dispatch integration** — new interceptor call slots into `handleKey` AFTER `handleModalKey` and BEFORE `keymap.Resolve`:

```
1. hint-clear block (+ NEW: m.vim.hint = "")
2. pending-reload
3. chord-second guard
4. handleModalKey (existing, unchanged)
5. NEW: if m.modes.vimMotion { handled := m.interceptVimMotion(msg); if handled { return ... } }
6. Resolve + chord-first + dispatchAction (existing, unchanged)
```

**Interceptor signature**: `func (m Model) interceptVimMotion(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool)`. Returns `handled=true` to short-circuit; `handled=false` to fall through to `Resolve`/dispatch.

**Why AFTER `handleModalKey` (revised from initial design)**: if a modal (search, annotate, overlay) is active, the modal must consume the key — the user is typing INTO the modal's textinput or navigating the overlay. Running vim interceptor first would swallow digits and letters before the modal sees them. The earlier framing ("before modal so keys don't leak into textinput") was backwards: the textinput IS where those keys should go when the modal is active. Placing vim after modal is simpler and correct — when modal is inactive, `handleModalKey` returns unhandled and control reaches the interceptor; when modal is active, the modal handles the key and vim never runs. No defensive short-circuit needed inside the interceptor.

**State-machine rules** (linear priority list):

1. **Pending letter leader wins** — if `m.vim.leader != ""`, use the second-stage key to look up in a static map:

| Leader | Second | Action |
|---|---|---|
| `g` | `g` | `ActionHome` |
| `z` | `z` | `ActionScrollCenter` |
| `z` | `t` | `ActionScrollTop` |
| `z` | `b` | `ActionScrollBottom` |
| `Z` | `Z` | `ActionQuit` |
| `Z` | `Q` | `ActionDiscardQuit` |

   On match: clear leader, dispatch via `m.dispatchAction(action)`, return handled. On `esc`: clear silently. On other key: `hint = "Unknown: <leader><key>"`, clear leader, return handled (don't fall through).

2. **Digit accumulation** — if key is `0–9` AND (`count > 0` OR key != `"0"`): accumulate (cap at 9999), update hint, return handled. `"0"` alone (count==0) falls through unhandled.

3. **`G` motion (any count, diff pane)** — if `key == "G"` and `m.layout.focus == paneDiff`:
   - if `count > 0`: `jumpToLineN(count)` (clamped to diff length)
   - if `count == 0`: `jumpToLineN(lastLine)` — bare `G` jumps to last line
   - clear count, return handled

4. **Other count consumer keys** (if `count > 0`):
   - `j`: call `handleDiffAction(ActionDown)` in loop `count` times, clear count, return handled (diff-pane-only — skip if `m.layout.focus != paneDiff`, falls through instead)
   - `k`: same with `ActionUp` (diff-pane-only)
   - any other key: clear count silently, return NOT handled (e.g., `5q` still quits)

5. **Leader entry** — if key is `g`/`z`/`Z` and `count == 0`: set `m.vim.leader = key`, set hint (`"g…"` / `"z…"` / `"Z…"`), return handled. For `g` specifically, require `m.layout.focus == paneDiff` (since `gg` is motion). For `z`, require `paneDiff` too. `Z` is pane-agnostic.

6. **Fall through** — return handled=false.

**Helpers needed:**
- `bottomAlignViewportOnCursor()` in `diffnav.go` — mirror of `topAlignViewportOnCursor`, one subtraction flip
- `jumpToLineN(n int)` in `diffnav.go` — clamps target to valid bounds, moves cursor, calls `centerViewportOnCursor`

**New action constants in `app/keymap/keymap.go`:**
- `ActionScrollCenter Action = "scroll_center"`
- `ActionScrollTop    Action = "scroll_top"`
- `ActionScrollBottom Action = "scroll_bottom"`

Added to the `navigationActions` allowlist so `handleDiffAction`/`handleTreeAction` route them, and to `helpEntries` so they appear in the help overlay. **No default single-key bindings** — the vim-motion interceptor is the only way to reach them unless the user maps them explicitly. Keeps the default keymap clean and makes `--vim-motion` the feature boundary.

**Dispatch wiring**: `handleDiffAction` gets three new `case` arms (`ActionScrollCenter → centerViewportOnCursor`, etc.). `handleTreeAction` does NOT handle the scroll cases (diff-pane-only scope), so these three cases only appear in `handleDiffAction`.

**Modal clearing** — extend existing `clearChordState` helper into `clearPendingInputState`:

```go
func (m *Model) clearPendingInputState() {
    m.keys.chordPending = ""
    m.keys.hint = ""
    m.vim = vimState{}
}
```

Rename all three `clearChordState` callsites (`startSearch`, `startAnnotation`, `handleOverlayOpen`) to `clearPendingInputState`. Delete the old `clearChordState` (no other callers).

**Config** — one new bool field on the existing options struct in `app/config.go`:

```go
VimMotion bool `long:"vim-motion" env:"VIM_MOTION" description:"enable vim-style motion preset (counts, gg, G, zz/zt/zb, ZZ/ZQ)" ini-name:"vim-motion"`
```

Default false. Wired through `ModelConfig.VimMotion` (new field on `ModelConfig`), and `NewModel` copies it into `m.modes.vimMotion` alongside `m.modes.compact` etc. — matches the existing `modeState` convention for user-togglable view flags (model.go:273–292). Guard uses `m.modes.vimMotion`.

**Status-bar hint** — append to `transientHint()` priority chain in `view.go`:

```go
case m.vim.hint != "":
    return m.vim.hint
```

Lowest priority (below commits/reload/compact/keys) — vim-motion hints are recoverable and never urgent.

**Performance**: when `--vim-motion` is off, the interceptor is short-circuited at the top of `handleKey` via `if m.modes.vimMotion`; cost is ~1 branch per keypress.

## Technical Details

**`interceptVimMotion` pseudocode:**

```go
func (m Model) interceptVimMotion(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
    keyStr := msg.String()

    // priority 1: pending letter leader
    if m.vim.leader != "" {
        return m.resolveVimLeader(keyStr)
    }

    // priority 2: digit accumulation
    if isDigit(keyStr) {
        if keyStr == "0" && m.vim.count == 0 {
            return m, nil, false // "0" alone falls through
        }
        m.vim.count = m.vim.count*10 + digitValue(keyStr)
        if m.vim.count > 9999 {
            m.vim.count = 9999
        }
        m.vim.hint = strconv.Itoa(m.vim.count)
        return m, nil, true
    }

    // priority 3: G motion (any count, diff pane) — bare G goes to last line
    if keyStr == "G" && m.layout.focus == paneDiff {
        n := m.vim.count
        m.vim.count = 0
        m.vim.hint = ""
        if n == 0 {
            n = len(m.diffLines) // jumpToLineN clamps
        }
        m.jumpToLineN(n)
        return m, nil, true
    }

    // priority 4: other count consumer keys (count > 0)
    if m.vim.count > 0 {
        switch {
        case keyStr == "j" && m.layout.focus == paneDiff:
            return m.repeatDiffAction(keymap.ActionDown, m.vim.count)
        case keyStr == "k" && m.layout.focus == paneDiff:
            return m.repeatDiffAction(keymap.ActionUp, m.vim.count)
        default:
            m.vim.count = 0
            m.vim.hint = ""
            return m, nil, false // fall through (e.g., "5q" still quits)
        }
    }

    // priority 5: leader entry
    if (keyStr == "g" || keyStr == "z") && m.layout.focus == paneDiff {
        m.vim.leader = keyStr
        m.vim.hint = keyStr + "…"
        return m, nil, true
    }
    if keyStr == "Z" {
        m.vim.leader = keyStr
        m.vim.hint = keyStr + "…"
        return m, nil, true
    }

    return m, nil, false
}

// resolveVimLeader handles the second-stage key after a leader is pending.
func (m Model) resolveVimLeader(keyStr string) (tea.Model, tea.Cmd, bool) {
    leader := m.vim.leader
    m.vim.leader = ""
    m.vim.hint = ""
    if keyStr == "esc" {
        return m, nil, true // silent cancel
    }
    action, ok := vimChordTable[leader+keyStr]
    if !ok {
        m.vim.hint = "Unknown: " + leader + keyStr
        return m, nil, true
    }
    model, cmd := m.dispatchAction(action)
    return model, cmd, true
}

var vimChordTable = map[string]keymap.Action{
    "gg": keymap.ActionHome,
    "zz": keymap.ActionScrollCenter,
    "zt": keymap.ActionScrollTop,
    "zb": keymap.ActionScrollBottom,
    "ZZ": keymap.ActionQuit,
    "ZQ": keymap.ActionDiscardQuit,
}

// repeatDiffAction invokes handleDiffAction n times. Clears count/hint on the
// returned model. For n > small (e.g., 100), the loop is still fast because
// handleDiffAction is O(1) per invocation for Down/Up (just cursor + viewport sync).
// handleDiffAction(ActionDown/ActionUp) returns nil cmd in current code; if a
// future change makes iterations produce meaningful cmds, collect them into a
// tea.Batch instead of keeping only the last.
func (m Model) repeatDiffAction(action keymap.Action, n int) (tea.Model, tea.Cmd, bool) {
    var model tea.Model = m
    var cmd tea.Cmd
    for i := 0; i < n; i++ {
        mm := model.(Model)
        mm.vim.count = 0
        mm.vim.hint = ""
        model, cmd = mm.handleDiffAction(action)
    }
    mm := model.(Model)
    mm.vim.count = 0
    mm.vim.hint = ""
    return mm, cmd, true
}
```

(Exact implementation may simplify the `model.(Model)` round-tripping; the concept is: iterate `handleDiffAction`, keep the last `cmd`, ensure final state has vim cleared.)

**`jumpToLineN` pseudocode:**

```go
// jumpToLineN moves the diff cursor to line n (1-indexed), clamped to [1, total].
// After moving, centers the viewport on the new cursor position.
func (m *Model) jumpToLineN(n int) {
    total := len(m.diffLines)
    if total == 0 {
        return
    }
    if n < 1 {
        n = 1
    }
    if n > total {
        n = total
    }
    m.nav.diffCursor = n - 1 // convert to 0-indexed
    m.centerViewportOnCursor()
}
```

**`bottomAlignViewportOnCursor` pseudocode:**

```go
// bottomAlignViewportOnCursor scrolls the viewport to place the cursor at the
// bottom of the page. Mirror of topAlignViewportOnCursor with offset flipped.
func (m *Model) bottomAlignViewportOnCursor() {
    // read viewport height, compute offset so cursor lands on the last visible row
    // (implementation mirrors topAlignViewportOnCursor line-for-line)
}
```

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): all code, tests, and docs within this repo.
- **Post-Completion** (no checkboxes): GitHub issue/PR comments after merge, manual verification scenarios.

## Implementation Steps

### Task 1: Add scroll action constants + help entries

**Files:**
- Modify: `app/keymap/keymap.go`
- Modify: `app/keymap/keymap_test.go`

- [x] add `ActionScrollCenter`, `ActionScrollTop`, `ActionScrollBottom` Action constants
- [x] add all three to `navigationActions` allowlist so `handleDiffAction` will route them
- [x] add all three to `helpEntries` with descriptions like "center viewport on cursor", "align viewport top", "align viewport bottom" under "Navigation" category
- [x] no default single-key bindings — leave out of the `defaultBindings` map
- [x] write `TestActionScrollConstants_InNavigationActions` — assert all three are true in the allowlist
- [x] write `TestActionScrollConstants_InHelpEntries` — assert all three have help entries
- [x] run `make test` — must pass before task 2

### Task 2: Add bottomAlignViewportOnCursor + jumpToLineN helpers

**Files:**
- Modify: `app/ui/diffnav.go`
- Modify: `app/ui/diffnav_test.go`

- [x] add `bottomAlignViewportOnCursor()` as a mirror of `topAlignViewportOnCursor` (diffnav.go:281) — compute offset so cursor lands on last visible row
- [x] add `jumpToLineN(n int)` that clamps `n` to `[1, len(diffLines)]`, sets `m.nav.diffCursor = n-1`, calls `centerViewportOnCursor`
- [x] godoc both new methods
- [x] write `TestBottomAlignViewportOnCursor_PlacesCursorAtBottom` — cursor mid-file, call method, assert offset puts cursor on last visible row
- [x] write `TestBottomAlignViewportOnCursor_ShortFile` — diff shorter than viewport, assert graceful behavior (no scroll needed)
- [x] write `TestJumpToLineN_WithinBounds` — diff has 100 lines, call `jumpToLineN(50)`, assert cursor at 49 (0-indexed)
- [x] write `TestJumpToLineN_ClampsLow` — call `jumpToLineN(0)`, assert cursor at 0 (clamps to first line)
- [x] write `TestJumpToLineN_ClampsHigh` — call `jumpToLineN(99999)`, assert cursor at last line
- [x] write `TestJumpToLineN_EmptyDiff` — empty diff, call `jumpToLineN(5)`, assert no panic, cursor unchanged
- [x] run `make test` — must pass before task 3

### Task 3: Wire scroll actions into handleDiffAction

**Files:**
- Modify: `app/ui/diffnav.go`
- Modify: `app/ui/diffnav_test.go`

- [x] in `handleDiffAction` (diffnav.go:424), add three `case` arms: `ActionScrollCenter → centerViewportOnCursor`, `ActionScrollTop → topAlignViewportOnCursor`, `ActionScrollBottom → bottomAlignViewportOnCursor`
- [x] do NOT add to `handleTreeAction` (diff-pane-only scope)
- [x] write `TestHandleDiffAction_ScrollCenter` — focus diff, call with `ActionScrollCenter`, assert viewport repositioned
- [x] write `TestHandleDiffAction_ScrollTop` — same for top
- [x] write `TestHandleDiffAction_ScrollBottom` — same for bottom
- [x] run `make test` — must pass before task 4

### Task 4: Add VimMotion config field

**Files:**
- Modify: `app/config.go`
- Modify: `app/config_test.go`
- Modify: `app/renderer_setup.go` (wire through to ModelConfig)

- [x] add `VimMotion bool` field to the options struct with go-flags/env/ini tags as specified in Solution Overview
- [x] thread the value through to `ModelConfig.VimMotion` in `renderer_setup.go` (or wherever ModelConfig is built — verify during implementation; may be `main.go`)
- [x] add `VimMotion bool` field to `ModelConfig` struct in `app/ui/model.go`
- [x] add `vimMotion bool` field to `modeState` struct (model.go:273–292, alongside `compact`, `wrap`, `lineNumbers`, `showBlame`, `wordDiff`, `showUntracked`); godoc comment notes it gates the vim-motion interceptor
- [x] in `NewModel`, copy `cfg.VimMotion` into `m.modes.vimMotion` alongside the existing modes copy block
- [x] write `TestConfig_VimMotionFlag` — parse args with `--vim-motion`, assert opts.VimMotion is true
- [x] write `TestConfig_VimMotionDefault` — parse args without the flag, assert opts.VimMotion is false
- [x] write `TestConfig_VimMotionEnv` — set VIM_MOTION=true, parse args, assert true
- [x] write `TestConfig_VimMotionIni` — use dump-config flow or ini parser test convention, assert round-trip
- [x] run `make test` — must pass before task 5

### Task 5: Add vimState struct + hint wiring in transientHint

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/view.go`
- Modify: `app/ui/view_test.go`

- [x] define `vimState` struct (count, leader, hint) in `app/ui/model.go` next to `keyState` (godoc both fields and struct purpose)
- [x] add `vim vimState` field to Model struct as a plain value (not pointer)
- [x] in `handleKey` (model.go:~711), add `m.vim.hint = ""` to the existing hint-clear block alongside existing `m.commits.hint = ""` etc.
- [x] in `view.go` `transientHint()` (line 105), append `case m.vim.hint != "": return m.vim.hint` as the LAST case (lowest priority)
- [x] write `TestVimState_ZeroValue` — default Model has `vim.count == 0`, `vim.leader == ""`, `vim.hint == ""`
- [x] write `TestTransientHint_VimLowestPriority` — set `m.commits.hint = "x"` AND `m.vim.hint = "y"`, assert `transientHint()` returns "x"
- [x] write `TestTransientHint_VimShownWhenOthersEmpty` — only `m.vim.hint = "5"` set, assert returns "5"
- [x] write `TestHandleKey_ClearsVimHint` — set `m.vim.hint = "x"`, send any KeyMsg (vim-motion off), assert hint cleared
- [x] run `make test` — must pass before task 6

### Task 6: Add vimmotion.go with interceptor + state-machine logic

**Files:**
- Create: `app/ui/vimmotion.go`
- Create: `app/ui/vimmotion_test.go`

- [x] create `app/ui/vimmotion.go` with package-level `vimChordTable` map (6 entries from Solution Overview)
- [x] implement `interceptVimMotion(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool)` method on Model with the priority 1–5 decision list
- [x] implement `resolveVimLeader(keyStr string) (tea.Model, tea.Cmd, bool)` helper
- [x] implement `repeatDiffAction(action keymap.Action, n int) (tea.Model, tea.Cmd, bool)` helper that loops `handleDiffAction`
- [x] helper `isDigit(keyStr string) bool` and `digitValue(keyStr string) int` (unexported, package-private)
- [x] do NOT wire into handleKey yet — that happens in Task 7
- [x] write table-driven test `TestInterceptVimMotion_Priority` covering the full matrix:
  - leader pending + bound second → action dispatched, leader cleared, handled=true
  - leader pending + unbound second → hint set, leader cleared, handled=true
  - leader pending + esc → leader cleared silently, handled=true
  - digit key, count=0, key="0" → falls through, handled=false
  - digit key, count=0, key="5" → count=5, hint="5", handled=true
  - digit key, count=5, key="3" → count=53, hint="53", handled=true
  - digit key pushes count over 9999 → clamped to 9999
  - count=5, key="j", focus=diff → ActionDown repeated 5x, count cleared, handled=true
  - count=5, key="j", focus=tree → falls through, handled=false (diff-only)
  - count=5, key="G", focus=diff → jumpToLineN(5), count cleared, handled=true
  - count=0, key="G", focus=diff → jumpToLineN(lastLine), handled=true (bare G goes to end)
  - count=0, key="G", focus=tree → falls through, handled=false (diff-only)
  - count=5, key="q" → count cleared silently, handled=false (q still quits)
  - count=0, key="g", focus=diff → leader="g", hint="g…", handled=true
  - count=0, key="g", focus=tree → falls through, handled=false
  - count=0, key="z", focus=diff → leader="z", hint="z…", handled=true
  - count=0, key="Z", any focus → leader="Z", hint="Z…", handled=true (pane-agnostic)
  - count=0, non-vim key → falls through, handled=false
- [x] write `TestResolveVimLeader_AllChordTableEntries` — for each entry in `vimChordTable`, set leader, call `resolveVimLeader(second)`, assert correct action dispatched via dispatchAction (use moq or direct Model inspection)
- [x] write `TestRepeatDiffAction_MultipleIterations` — start at cursor 0, call `repeatDiffAction(ActionDown, 5)`, assert cursor at 5
- [x] write `TestRepeatDiffAction_ClampsAtBoundary` — near end of diff, `repeatDiffAction(ActionDown, 9999)`, assert cursor at last line (relies on `handleDiffAction`'s own clamping)
- [x] run `make test` — must pass before task 7

### Task 7: Wire interceptor into handleKey

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/model_test.go`

- [x] in `handleKey`, insert the vim-motion interceptor call AFTER `handleModalKey` and BEFORE `keymap.Resolve`:
  ```go
  if m.modes.vimMotion {
      if model, cmd, handled := m.interceptVimMotion(msg); handled {
          return model, cmd
      }
  }
  ```
- [x] write `TestHandleKey_VimMotionOff_InterceptorSkipped` — `modes.vimMotion = false`, send `5`, assert no vim state set, key falls through normally (likely unhandled, no-op)
- [x] write `TestHandleKey_VimMotionOn_DigitAccumulates` — `modes.vimMotion = true`, send `5`, assert `m.vim.count == 5`, `m.vim.hint == "5"`
- [x] write `TestHandleKey_VimMotionOn_ChordSecondWins` — `m.keys.chordPending = "ctrl+w"`, `modes.vimMotion = true`, send `5`, assert chord-second handles the key (vim NOT reached); verify `m.keys.chordPending` cleared and `m.vim.count == 0`
- [x] write `TestHandleKey_VimMotionOn_PendingReloadWins` — `m.reload.pending = true`, `modes.vimMotion = true`, send `y`, assert reload confirmed, vim state untouched
- [x] write `TestHandleKey_VimMotionOn_SearchActiveModalWins` — `modes.vimMotion = true`, `m.search.active = true`, send `5`, assert search textinput receives the `5` (modal handles the key), `m.vim.count == 0` (interceptor never ran). Locks in the ordering invariant: modal preempts vim.
- [x] write `TestHandleKey_VimMotionOn_AnnotateActiveModalWins` — same assertion with annotation mode active
- [x] write `TestHandleKey_VimMotionOn_OverlayActiveModalWins` — same with overlay open (e.g., help overlay)
- [x] write `TestHandleKey_VimMotionOn_NonVimKeyFallsThrough` — `modes.vimMotion = true`, send `q`, assert quit handled normally (interceptor returns handled=false for non-vim keys with no pending state)
- [x] run `make test` — must pass before task 8

### Task 8: Extend clearChordState → clearPendingInputState

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/search.go`
- Modify: `app/ui/annotate.go`
- Modify: `app/ui/model_test.go`
- Modify: `app/ui/search_test.go`
- Modify: `app/ui/annotate_test.go`

- [x] rename `clearChordState` (model.go:848) to `clearPendingInputState`; extend body to also reset `m.vim = vimState{}`
- [x] update all three callsites: `startSearch` (search.go), `startAnnotation` (annotate.go), `handleOverlayOpen` (model.go)
- [x] update existing tests that referenced `clearChordState` (rename in test names too; search test files for "clearChordState" to find them)
- [x] write `TestClearPendingInputState_ClearsAllFields` — set chordPending, keys.hint, vim.count, vim.leader, vim.hint, call method, assert all cleared
- [x] write `TestStartSearch_ClearsVimState` — set `m.vim.count = 5`, `m.vim.hint = "5"`, call `startSearch`, assert both cleared
- [x] write `TestStartAnnotation_ClearsVimState` — same for annotation
- [x] write `TestHandleOverlayOpen_ClearsVimState_Help` — set vim state, call `handleOverlayOpen(ActionHelp)`, assert vim state cleared
- [x] write `TestHandleOverlayOpen_ClearsVimState_ThemeSelect` — same for ActionThemeSelect
- [x] run `make test` — must pass before task 9

### Task 9: Integration test matrix

**Files:**
- Modify: `app/ui/vimmotion_test.go` (or create `app/ui/vimmotion_integration_test.go` if growing past ~300 lines and vimmotion_test.go is full)

- [x] add `TestVimMotion_FullFlow_5j` — `modes.vimMotion = true`, start at cursor 0, send `5`, `j`, assert cursor at 5, count/hint cleared
- [x] add `TestVimMotion_FullFlow_gg` — send `g`, `g`, assert cursor at 0, leader/hint cleared
- [x] add `TestVimMotion_FullFlow_G_Bare` — send `G` with no count (count==0), assert cursor at last line, count/hint still zero
- [x] add `TestVimMotion_FullFlow_5G` — send `5`, `G`, assert cursor at line 5 (0-indexed: 4), count/hint cleared
- [x] add `TestVimMotion_FullFlow_zz` — cursor mid-file, send `z`, `z`, assert viewport centered, leader/hint cleared
- [x] add `TestVimMotion_FullFlow_zt` — send `z`, `t`, assert top-aligned
- [x] add `TestVimMotion_FullFlow_zb` — send `z`, `b`, assert bottom-aligned
- [x] add `TestVimMotion_FullFlow_ZZ` — send `Z`, `Z`, assert quit (return tea.Quit or equivalent)
- [x] add `TestVimMotion_FullFlow_ZQ` — send `Z`, `Q`, assert discard-quit
- [x] add `TestVimMotion_LeaderCancelled` — send `g`, `esc`, assert leader cleared, cursor unchanged
- [x] add `TestVimMotion_UnknownChord` — send `g`, `x`, assert leader cleared, hint="Unknown: gx", cursor unchanged
- [x] add `TestVimMotion_CountThenUnrelatedKey` — send `5`, `q`, assert quit fires (5 dropped silently)
- [x] add `TestVimMotion_MotionInTreePane` — `m.layout.focus = paneTree`, send `5`, `j`, assert falls through (vim motion is diff-only)
- [x] add `TestVimMotion_ZQuitFromTreePane` — `m.layout.focus = paneTree`, send `Z`, `Z`, assert quit (Z is pane-agnostic)
- [x] run `make test` — must pass before task 10

### Task 10: Help overlay + documentation updates

**Files:**
- Modify: `app/ui/handlers.go` (`buildHelpSpec`)
- Modify: `app/ui/handlers_test.go`
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff/references/usage.md`

Scroll actions (`ActionScrollCenter`/`ActionScrollTop`/`ActionScrollBottom`) have no default single-key bindings, so `keymap.KeysFor()` returns empty for them and the help overlay would render them with no keys. Fix by augmenting `buildHelpSpec` to append a synthetic "Vim motion" section when `m.modes.vimMotion` is on — listing the 8 preset bindings with hardcoded key strings.

- [x] in `buildHelpSpec` (handlers.go:52), after the existing `km.HelpSections()`-based section build, append a conditional section when `m.modes.vimMotion`:
  ```go
  if m.modes.vimMotion {
      result = append(result, overlay.HelpSection{
          Title: "Vim motion",
          Entries: []overlay.HelpEntry{
              {Keys: "N j / N k", Desc: "move cursor N lines down/up"},
              {Keys: "gg",        Desc: "jump to first line"},
              {Keys: "G / N G",   Desc: "jump to last line / goto line N"},
              {Keys: "zz",        Desc: "center viewport on cursor"},
              {Keys: "zt",        Desc: "align viewport top"},
              {Keys: "zb",        Desc: "align viewport bottom"},
              {Keys: "ZZ",        Desc: "quit"},
              {Keys: "ZQ",        Desc: "discard and quit"},
          },
      })
  }
  ```
  (Adjust struct field names to match the actual `overlay.HelpEntry` shape — verify names during implementation; likely `Keys` and `Description` or similar.)
- [x] write `TestBuildHelpSpec_VimMotionSectionOff` — `modes.vimMotion = false`, call `buildHelpSpec`, assert no "Vim motion" section
- [x] write `TestBuildHelpSpec_VimMotionSectionOn` — `modes.vimMotion = true`, call `buildHelpSpec`, assert a "Vim motion" section is present with all 8 entries
- [x] in `README.md`, add a "Vim-motion preset" subsection under the existing keybindings section. Cover: enable via `--vim-motion` CLI flag, `VIM_MOTION` env var, or `vim-motion = true` in config; list all bindings in a table (same layout as the existing keybindings table); note that vim-motion keys (`g`, `z`, `Z`, digits) override any standalone single-key bindings when the flag is on; note that `ZZ`/`ZQ` work in any pane, motion keys are diff-pane only
- [x] mirror the README addition in `site/docs.html` (same content, HTML-formatted)
- [x] manual check: `diff <(sed -n '/vim-motion/,/^## /p' README.md) <(sed -n '/vim-motion/,/<h2/p' site/docs.html)` — content should match (allow for HTML tag differences; same bindings, same flag names, same semantics)
- [x] add identical paragraph to `.claude-plugin/skills/revdiff/references/usage.md`
- [x] copy BYTE-IDENTICAL to `plugins/codex/skills/revdiff/references/usage.md` (per `feedback_revdiff-docs-sync-plugins.md`)
- [x] verify: `diff .claude-plugin/skills/revdiff/references/usage.md plugins/codex/skills/revdiff/references/usage.md` shows no differences (or only pre-existing differences)
- [x] run `make test` — must pass before task 11

### Task 11: Verify acceptance criteria

- [ ] `--vim-motion` flag parses from CLI, env (`VIM_MOTION`), config (`vim-motion = true`)
- [ ] with `--vim-motion` on: `<N>j`, `<N>k`, `<N>G`, `gg`, `G`, `zz`, `zt`, `zb`, `ZZ`, `ZQ` all work as specified
- [ ] with `--vim-motion` off: NO vim-motion behavior; existing bindings untouched
- [ ] bare `G` (no count) jumps to last line; `5G` jumps to line 5
- [ ] `esc` cancels pending leader silently
- [ ] Unknown second key shows "Unknown: <chord>" hint
- [ ] digits over 9999 are clamped
- [ ] `0` alone doesn't trigger vim accumulator
- [ ] count + unrelated key (e.g., `5q`) falls through to quit
- [ ] entering search/annotate/overlay: modal consumes keys, vim interceptor does not fire (verified by Task 7 tests)
- [ ] ctrl+w chord still works when vim-motion is on (orthogonal)
- [ ] pending-reload (`R` then `y/n`) preempts vim (not broken by vim-motion)
- [ ] motion keys in tree pane fall through (not intercepted)
- [ ] `ZZ`/`ZQ` work from any pane
- [ ] help overlay (`?`) with `--vim-motion` on shows a "Vim motion" section listing all 8 preset bindings; without the flag, no such section appears
- [ ] vim motion composes cleanly with compact mode and collapsed mode: `jumpToLineN` operates on the currently-visible `diffLines`, so line numbers are relative to the current view (compact shrinks the diff before parsing; collapsed hides rendered rows but doesn't change `diffLines` length)
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] verify `--dump-config` includes `vim-motion` key (via `make run -- --dump-config` or similar)

### Task 12: Final — update CLAUDE.md and move plan

**Files:**
- Modify: `CLAUDE.md`

- [ ] add a bullet in CLAUDE.md "Gotchas" section noting: vim-motion preset is gated by `--vim-motion`; state lives on `vimState` (separate from `keyState`); interceptor runs between chord-second and modal key; diff-pane scope for motion/viewport, pane-agnostic for quit aliases
- [ ] move this plan file to `docs/plans/completed/20260423-vim-motion-preset.md`

## Post-Completion

*Items requiring external action — informational only*

**Manual verification scenarios** (do before merge):
- launch `revdiff --vim-motion`, open a long diff, type `42j` — cursor jumps 42 lines
- type `gg` — cursor at first line
- type `G` — cursor at last line
- type `5G` — cursor at line 5
- scroll viewport with `zz`/`zt`/`zb` — cursor doesn't move, viewport repositions
- type `ZZ` — app quits cleanly
- type `ZQ` — app quits without saving (annotations auto-save so effect matches `ZZ`)
- launch without `--vim-motion`, type `5j` — `5` is unhandled, `j` moves one line (existing behavior)
- enable via config: add `vim-motion = true` to `~/.config/revdiff/config`, launch without CLI flag, verify vim motion is on
- tmux/ssh compatibility: launch over ssh, verify `5j` works (no special terminal requirements)

**GitHub issue/PR updates** (after merge):
- comment on issue #138 (open): motion preset shipped behind `--vim-motion` flag; covers `<N>j`/`<N>k`/`gg`/`<N>G`; yank (`yy`) is not shipped and remains open if there's demand — close at user's discretion
- comment on closed PR #63 (pinging `@anpryl`): thank for the original proposal, note that `zz`/`zt`/`zb`/`ZZ`/`ZQ` landed as part of vim-motion preset and are reachable via `--vim-motion`

**External system updates:**
- none — feature is self-contained in revdiff
- revdiff.com site update: `site/docs.html` is part of this plan (Task 10), no separate deploy step beyond the usual Cloudflare Pages auto-deploy on merge to master
- plugin version bump: since `.claude-plugin/skills/revdiff/references/usage.md` is modified (per `feedback_plugin-version-bump.md`), ASK at release time whether to bump the plugin version in `.claude-plugin/plugin.json` + `.claude-plugin/marketplace.json` alongside the release
