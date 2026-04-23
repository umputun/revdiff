# Leader-Based Chord Keybindings (kitty-style `ctrl+w>x`)

## Overview

Add multi-key chord support to revdiff's keybindings system, restricted to control/alt-prefixed leader keys followed by a single second-stage key. Format mirrors kitty terminal: `map ctrl+w>x close_file`. Ships zero default chord bindings — purely a user-customization feature exposed through `~/.config/revdiff/keybindings`.

**Problem solved.** Single-key namespace is finite. Power users with custom workflows want richer keybindings (e.g., `ctrl+t>n` next theme, `ctrl+t>s` theme picker). Today the only way to add such bindings is to consume another single key. Leader chords give an unbounded namespace under prefixed leaders without invading the existing flat key space.

**Scope-limited by design.** The leader is restricted to `ctrl+*` / `alt+*` combos, which sidesteps every open question on issue #138 (vim-style `gg`/`yy`/count-prefix support):
- Layout-resolve broken on Cyrillic for letter-leaders → not applicable to control combos (layout-independent in bubbletea)
- Single-key vs prefix conflict (`g` action vs `gg` chord) → not applicable (no app uses ctrl combos as standalone actions today)
- Stale-prefix timeout → not needed (esc-only cancel, hint visible in status bar)

This plan ships the engine + user-facing config support. Vim-style chords remain out of scope (issue #138 stays open).

## Context (from discovery)

**Files involved:**
- `app/keymap/keymap.go` — Keymap struct, `parse()`, `Load()` (line 425), `Resolve()`, `Bind`/`Unbind`, `KeysFor`, `HelpSections`, `Dump`
- `app/keymap/keymap_test.go` — existing parser/resolve coverage to mirror for chord cases
- `app/ui/model.go` — `Model` struct (lines ~270-410), `navigationState` (lines 285-310), `handleKey` (line 711), `handleModalKey` (line 862), `handleOverlayOpen` (line 775)
- `app/ui/search.go` — `startSearch()` (line 11), `handleSearchKey()`
- `app/ui/annotate.go` — `startAnnotation()` (line 53)
- `app/ui/themeselect.go` — `openThemeSelector()` (line 20)
- `app/ui/handlers.go` — pane-specific handlers for context on dispatch ordering
- `README.md` + `site/docs.html` — keybindings docs (must stay in sync per CLAUDE.md)
- `.claude-plugin/skills/revdiff/references/usage.md` + `plugins/codex/skills/revdiff/references/usage.md` — must stay byte-identical (per `feedback_revdiff-docs-sync-plugins.md`)

**Patterns observed:**
- Bindings are flat `map[string]Action`. Keys are bubbletea's `KeyMsg.String()` output. Chord keys naturally fit as flat strings with `>` separator (`"ctrl+w>x"`).
- `parse()` is permissive: warns + skips invalid lines, does not abort. Chord parsing follows the same convention.
- `Resolve()` has a layout-resolve fallback: try direct key first, then translate non-Latin runes to Latin equivalent and retry. The chord second-stage key needs the same fallback.
- `handleKey` already has multiple guards (annotation mode, search mode, filter mode, pending-reload, overlays) before single-key dispatch. Chord guards slot into this chain.
- `KeysFor()` returns sorted slice; `HelpSections()` joins with `" / "`. Chord keys round-trip verbatim — no formatting transformation.
- `feedback_use-moq-not-manual-mocks.md` + `feedback_no-test-only-code.md` — apply when adding test infrastructure.

**Architectural validation (go-architect feedback applied):**
- `chordPending` lives on a new `keyState` sub-struct on `Model`, NOT inside `navigationState` (which is for cursor/hunk-jump state — adding chord state would make it a grab-bag).
- `handleKey` chord-second guard runs BEFORE modal/textinput guards (otherwise active search would eat the second key into its input). Plus modal-entering paths clear chord state explicitly so the two never coexist.
- Chord-prefix index is **lazy**: nil by default, built on first `IsChordLeader()` call, nil-reset on `Bind`/`Unbind`. Avoids rebuilding N times during `Load()` which calls `Bind` in a loop.

## Development Approach

- **Testing approach**: Regular (code first, then tests). Each task implements + tests in the same task. No task closes with failing tests.
- Make small, focused changes — each task is a single concern.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task. Tests are not optional.
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** — no exceptions.
- Run `make test` after each task before moving on.
- Run `make lint` before final commit.
- Maintain backward compatibility — all existing single-key bindings continue to work unchanged.

## Testing Strategy

- **Unit tests** (`app/keymap/keymap_test.go`): parser cases, validation, conflict resolution, IsChordLeader, ResolveChord with layout-resolve, Dump round-trip.
- **Integration tests** (`app/ui/model_test.go` or new `app/ui/chord_test.go`): full handleKey precedence matrix — chord + overlay open, chord + search active, chord + annotate active, chord + pending-reload, esc cancel, unknown second key, resolved second key.
- **No e2e tests** in this project (revdiff is a TUI; bubbletea integration tested via Model unit tests).

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview

**Engine in `app/keymap`.** The Keymap struct holds bindings as `map[string]Action` with chord keys stored as flat strings (`"ctrl+w>x"`). Parser splits on `>`, validates leader is a ctrl/alt combo, second is a single key, depth is exactly 2. At the end of `Load()`, when both `ctrl+w` standalone and `ctrl+w>x` chord exist, the standalone is dropped with a warn (chord wins). A lazy chord-prefix index (`chordPrefixCache map[string]struct{}`) supports O(1) `IsChordLeader()` lookups.

**Dispatch in `app/ui`.** Model gains a new `keyState struct { chordPending string }` field. `handleKey` gets a chord-second guard inserted **before** the `handleModalKey` call (architect's feedback — second key must not leak into a textinput). When chord-second resolves, dispatch normally; when it doesn't, set "Unknown chord" hint and consume the key (strict). Esc cancels silently. After `handleModalKey` returns unhandled, a chord-first guard checks `IsChordLeader` and enters pending state with the status hint. All modal-entering code paths (`startSearch`, `startAnnotation`, `handleOverlayOpen`) explicitly clear chord state — chord and modal modes never coexist in normal flow; the early chord-second guard is defense-in-depth.

**Help & Dump.** Help overlay renders chord keys verbatim alongside single-key bindings under the same action. Sort order is plain ASCII ascending on the raw key string (whatever `sort.Strings` produces). No special "chord first" or "chord last" rule — chords land where their raw string lands. Example: `ctrl+w>x` and `x` sort to `["ctrl+w>x", "x"]` because `'c' (0x63) < 'x' (0x78)`; but `ctrl+w>a` and `a` sort to `["a", "ctrl+w>a"]` because `'a' (0x61) < 'c' (0x63)`. `HelpSections()` joins the slice with `" / "` directly. No case transformation, no display formatter — matches existing convention. Dump round-trips chord bindings as `map ctrl+w>x close_file` because the storage key is already a flat string.

**Hint field.** Status-bar hints follow the existing per-feature pattern: each sub-struct that wants to surface a transient hint has its own `hint string` field, and `transientHint()` (view.go:103-113) checks them in priority order. Chord adds `m.keys.hint string` to the new `keyState` struct; `transientHint()` gets a new case for it; `handleKey`'s hint-clear block (model.go:714-716) gets a new line clearing it.

**Dispatch helper.** `handleChordSecond` needs to dispatch a resolved action through the same machinery as a single-key press. The existing flow lives inline in `handleKey` (resolve → handleOverlayOpen → switch → pane-nav fallback), and the pane-nav handlers re-resolve from `msg.String()`. To make chord-second dispatch correctly without synthesizing fake keys, we extract a `dispatchAction(action keymap.Action) (tea.Model, tea.Cmd)` helper from `handleKey`'s post-resolve flow, plus split `handleDiffNav`/`handleTreeNav` into thin msg-receivers + new `handleDiffAction(action)`/`handleTreeAction(action)` cores. `handleKey` and `handleChordSecond` both call `dispatchAction`. This is a real refactor (Task 5 below) — necessary because chord-resolved actions must reach pane-nav handlers without going through `Resolve(msg.String())` again (the second-stage key may not resolve to anything in the standalone bindings, only via the chord).

**Docs.** README + site docs gain a brief paragraph on chord syntax. The two skill reference copies stay byte-identical.

## Technical Details

**Storage** — `Keymap.bindings map[string]Action`:
- Single-key entry: `"j" → Action("down")`
- Chord entry: `"ctrl+w>x" → Action("close_file")`
- Sort order in `KeysFor()`: ASCII ascending (`sort.Strings`). Chord position relative to single-key counterparts depends on the second-stage character — e.g., `ctrl+w>x` sorts before `x` (`'c' < 'x'`) but `ctrl+w>a` sorts after `a` (`'c' > 'a'`). No special-casing.

**Parser pseudocode** (inside `parse()`):
```go
// detect chord BEFORE normalization to avoid double-work (split first, then normalize each half)
rawKey := fields[1]
if strings.Contains(rawKey, ">") {
    parts := strings.SplitN(rawKey, ">", 2)
    leader, second := parts[0], parts[1]
    if leader == "" || second == "" {
        log.Printf("[WARN] keybindings:%d: chord halves cannot be empty, skipping", lineNum)
        continue
    }
    if strings.Contains(second, ">") {
        log.Printf("[WARN] keybindings:%d: only 2-stage chords supported, skipping", lineNum)
        continue
    }
    leaderNorm := normalizeKey(leader)
    if !strings.HasPrefix(leaderNorm, "ctrl+") && !strings.HasPrefix(leaderNorm, "alt+") {
        log.Printf("[WARN] keybindings:%d: chord leader must be ctrl+ or alt+ combo, skipping", lineNum)
        continue
    }
    // second-stage case is preserved (consistent with existing single-key behavior;
    // ctrl+w>x and ctrl+w>X are distinct bindings, same as standalone x and X today)
    key := leaderNorm + ">" + normalizeKey(second)
    maps = append(maps, mapEntry{key: key, action: action})
    continue
}
// non-chord path unchanged
key := normalizeKey(rawKey)
maps = append(maps, mapEntry{key: key, action: action})
```

**Conflict resolution at end of `Load()`:**
After all map/unmap entries are applied via `Bind`/`Unbind`, scan for chord bindings:
```go
for chordKey := range km.bindings {
    if !strings.Contains(chordKey, ">") { continue }
    leader := strings.SplitN(chordKey, ">", 2)[0]
    if _, exists := km.bindings[leader]; exists {
        log.Printf("[WARN] keybindings: %s bound as both standalone and chord prefix; standalone dropped", leader)
        delete(km.bindings, leader)
    }
}
km.chordPrefixCache = nil // invalidate index after structural change
```

**New methods on Keymap:**
```go
// IsChordLeader returns true if the given key is the leader of any chord binding.
// Lookup is O(1) via cached prefix index, built on first call.
func (km *Keymap) IsChordLeader(key string) bool

// ResolveChord returns the action bound to the chord (prefix, second), or empty
// Action if unbound. Applies layout-resolve fallback to second key (translates
// non-Latin runes to Latin equivalent and retries lookup).
func (km *Keymap) ResolveChord(prefix, second string) Action

// chordPrefixes returns the set of leader keys having at least one chord binding.
// Built lazily and cached; invalidated by Bind/Unbind and at end of Load.
func (km *Keymap) chordPrefixes() map[string]struct{}
```

`Bind` and `Unbind` set `chordPrefixCache = nil` so the next `IsChordLeader()` rebuilds.

**Model state** — new sub-struct on Model:
```go
// keyState holds transient key-dispatch state (chord pending, hint).
// Distinct from navigationState which holds cursor/scroll state.
type keyState struct {
    chordPending string // leader key when waiting for second-stage; "" otherwise
    hint         string // transient status-bar hint set by chord dispatcher
}

type Model struct {
    // ...existing fields...
    keys keyState
}
```

`transientHint()` (view.go:103-113) gets a new case for `m.keys.hint != ""`. Priority order: existing `commits` → `reload` → `compact` → NEW `keys` (chord hints are lowest priority — if a reload is happening or a compact-mode toggle just fired, those hints win, since chord hints are user-driven and recoverable).

**Dispatcher ordering in `handleKey` (actual structure, insertion points marked NEW):**

The current `handleKey` body (model.go:711-773) is:
```
1. hint-clear: m.commits.hint = "" / m.reload.hint = "" / m.compact.hint = ""
2. if m.reload.pending { return m.handlePendingReload(msg) }
3. if handled, model, cmd := m.handleModalKey(msg); handled { return ... }
   // handleModalKey bundles annotate-mode + search-mode + active-overlay dispatch
4. action := m.keymap.Resolve(msg.String())
5. if model, ok := m.handleOverlayOpen(action); ok { return model, nil }
   // handleOverlayOpen handles ActionHelp / ActionAnnotList / ActionThemeSelect / ActionCommitInfo
6. switch action { ... }  // ActionDismiss, ActionQuit, ActionFilter, etc.
7. pane-specific: m.handleTreeNav(msg) or m.handleDiffNav(msg)
```

The two new guards slot in like this:

```
1. hint-clear (existing) — add NEW line: m.keys.hint = ""
2. pending-reload (existing)
3. NEW: chord-second guard — if m.keys.chordPending != "", return m.handleChordSecond(msg.String())
4. handleModalKey (existing) — startSearch/startAnnotation/handleOverlayOpen all clear chord state on entry,
   so chord and modal never coexist in normal flow; this guard is defense-in-depth
5. action := m.keymap.Resolve(msg.String()) (existing)
6. NEW: chord-first guard — if action == "" && m.keymap.IsChordLeader(msg.String()):
       m.keys.chordPending = msg.String()
       m.keys.hint = "Pending: " + msg.String() + ", esc to cancel"
       return m, nil
   // Note: action is guaranteed empty for a chord leader because Load() conflict resolution
   // dropped any standalone binding for a key that's also a chord prefix.
7. return m.dispatchAction(action, msg) — REFACTORED helper that wraps existing handleOverlayOpen + switch + pane-fallback
```

After the refactor (Task 5 below), steps 7-9 collapse into a single `dispatchAction` call that both `handleKey` and `handleChordSecond` invoke.

Why chord-second goes BEFORE `handleModalKey`: if a second key arrives while chord is pending, it must be consumed as chord-second regardless of whether a modal would otherwise eat it. The modal-entry paths (`startSearch`, `startAnnotation`, `handleOverlayOpen`) explicitly clear chord state, so chord-pending + modal-active should never coexist in practice — but the early chord-second guard ensures correct behavior even if a future code path accidentally enters a modal without clearing chord state.

Why chord-first goes AFTER `Resolve()`: by Load-time conflict resolution, no key is bound both standalone and as a chord prefix. So `action == ""` is guaranteed when `IsChordLeader(keyStr) == true`. Putting chord-first after `Resolve()` keeps the guard purely additive — never overrides an action.

**`handleChordSecond` (new method, ~30 lines):**
```go
func (m Model) handleChordSecond(keyStr string) (tea.Model, tea.Cmd) {
    prefix := m.keys.chordPending
    m.keys.chordPending = ""
    m.keys.hint = ""
    if keyStr == "esc" {
        return m, nil  // silent cancel
    }
    action := m.keymap.ResolveChord(prefix, keyStr)
    if action == "" {
        m.keys.hint = "Unknown chord: " + prefix + ">" + keyStr
        return m, nil
    }
    // dispatch through the unified helper extracted in Task 5;
    // synthesize a placeholder KeyMsg from the second-stage key for any
    // pane-nav handler that consults msg context
    msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(keyStr)}
    return m.dispatchAction(action, msg)
}
```

**Status hint string when entering pending state:**
```
"Pending: " + leader + ", esc to cancel"
```
Example: `Pending: ctrl+w, esc to cancel`. Verbatim — no case transformation.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): all code changes, tests, and doc updates achievable in this codebase.
- **Post-Completion** (no checkboxes): manual verification scenarios for chord behavior under unusual modal interactions.

## Implementation Steps

### Task 1: Keymap chord parser + validation

**Files:**
- Modify: `app/keymap/keymap.go`
- Modify: `app/keymap/keymap_test.go`

- [x] in `parse()`, when a `map` line's key contains `>`, validate: leader must start with `ctrl+` or `alt+`, second must not contain `>` (2-stage only); warn + skip on violation
- [x] normalize each half through `normalizeKey()` then rejoin with `>` so user can write `Ctrl+W>X` and have it stored as `ctrl+w>X`
- [x] write `TestParse_ChordBinding` — `map ctrl+w>x close_file` parses correctly, stored as `ctrl+w>x`
- [x] write `TestParse_ChordBinding_NormalizesCase` — `map Ctrl+W>X close_file` stored as `ctrl+w>X` (ctrl lowercased, second-stage case preserved)
- [x] write `TestParse_ChordBinding_RejectsNonModifierLeader` — `map g>g home` warns and is skipped (not added to bindings)
- [x] write `TestParse_ChordBinding_RejectsThreeStage` — `map ctrl+w>x>y foo` warns and is skipped
- [x] write `TestParse_ChordBinding_RejectsEmptyHalves` — `map ctrl+w> foo` and `map >x foo` warn and are skipped
- [x] run `make test` — must pass before task 2

### Task 2: Conflict resolution + lazy chord-prefix index

**Files:**
- Modify: `app/keymap/keymap.go`
- Modify: `app/keymap/keymap_test.go`

- [x] add `chordPrefixCache map[string]struct{}` field to Keymap (nil = not yet built)
- [x] add unexported `chordPrefixes()` method that lazily builds the cache from `bindings` on first call
- [x] add exported `IsChordLeader(key string) bool` method that wraps the cache lookup
- [x] update `Bind` and `Unbind` to set `chordPrefixCache = nil` on every call (lazy invalidation; rebuild deferred to next `IsChordLeader`)
- [x] add a `resolveConflicts()` step that runs at the end of `Load()`: for every chord key, if the leader exists as a standalone binding, log warn + delete the standalone; then set `chordPrefixCache = nil` once at the end
- [x] write `TestIsChordLeader` — true for `ctrl+w` when `ctrl+w>x` is bound; false for `ctrl+w` when only standalone `ctrl+w` is bound; false for non-prefix keys
- [x] write `TestIsChordLeader_LazyAndInvalidated` — first call builds cache, subsequent calls reuse; Bind/Unbind invalidates so the next call rebuilds (assert via observable behavior: bind a new chord, expect IsChordLeader to return true on the next call)
- [x] write `TestLoad_ConflictDropsStandalone` — `~/.config/revdiff/keybindings` with both `map ctrl+w toggle_pane` and `map ctrl+w>x close_file` results in only the chord remaining; warn logged
- [x] write `TestLoad_NoConflictKeepsBoth` — bindings without conflict are unaffected
- [x] run `make test` — must pass before task 3

### Task 3: ResolveChord with layout-resolve fallback

**Files:**
- Modify: `app/keymap/keymap.go`
- Modify: `app/keymap/keymap_test.go`

- [x] add `ResolveChord(prefix, second string) Action` method
- [x] direct lookup first: `bindings[prefix + ">" + second]`
- [x] fallback: if direct miss AND `second` is a single rune, decode the rune and call `layoutResolve(r rune) (rune, bool)` (layout.go:68); if it returns a translated rune (`ok == true`), retry lookup with `prefix + ">" + string(translatedRune)`
- [x] write `TestResolveChord_Direct` — `ResolveChord("ctrl+w", "x")` returns the bound action when `ctrl+w>x` exists
- [x] write `TestResolveChord_LayoutFallback` — `ResolveChord("ctrl+w", "ч")` (Cyrillic ch) returns the action bound to `ctrl+w>x` (because `ч` translates to `x` on Cyrillic)
- [x] write `TestResolveChord_Unbound` — returns empty Action when no binding matches
- [x] write `TestResolveChord_PrefixOnly` — returns empty Action when only the leader is bound (i.e., no chord exists)
- [x] run `make test` — must pass before task 4

### Task 4: Dump round-trip + KeysFor includes chord keys

**Files:**
- Modify: `app/keymap/keymap_test.go`

- [x] verify (no code change expected) `Dump()` writes chord bindings as `map ctrl+w>x close_file` — they're flat strings already
- [x] verify `KeysFor(action)` returns chord keys sorted alphabetically alongside single-key bindings
- [x] write `TestDump_RoundTripsChords` — define a Keymap with mixed single + chord bindings, Dump to a buffer, Parse the buffer back, assert resulting Keymap has identical bindings (use `reflect.DeepEqual` on `bindings` map)
- [x] write `TestKeysFor_IncludesChordKeys` — action with both `x` and `ctrl+w>x` bindings returns `["ctrl+w>x", "x"]` (ASCII alphabetic: `'c' (0x63)` < `'x' (0x78)`); HelpSections joins as `ctrl+w>x / x`
- [x] run `make test` — must pass before task 5

### Task 5: Extract dispatchAction helper + split pane-nav handlers

**Files:**
- Modify: `app/ui/model.go` (`handleKey`, new `dispatchAction`)
- Modify: `app/ui/diffnav.go` (split `handleDiffNav` + new `handleDiffAction`, split `handleTreeNav` + new `handleTreeAction`)
- Modify: `app/ui/diffnav_test.go` (rename/adjust existing nav tests; new tests for action variants)
- Modify: `app/ui/model_test.go` (test for dispatchAction)

This is a pure refactor — no behavior change. Required because chord-resolved actions (Task 6) need to flow through the same dispatch machinery as keymap-resolved actions, but the existing code re-resolves from `msg.String()` inside pane handlers.

- [x] add `dispatchAction(action keymap.Action, msg tea.KeyMsg) (tea.Model, tea.Cmd)` method on Model containing the post-Resolve flow currently inline in `handleKey` (model.go:729-771): `handleOverlayOpen` check, then the `switch action` block, then the pane-nav fallback. The `msg` parameter is needed because `handleDiffAction`/`handleTreeAction` may need it for context (e.g., raw key for some pane operations); pass it through.
- [x] split `handleDiffNav(msg tea.KeyMsg)` into a thin wrapper `func (m Model) handleDiffNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) { return m.handleDiffAction(m.keymap.Resolve(msg.String()), msg) }` plus a new `handleDiffAction(action keymap.Action, msg tea.KeyMsg) (tea.Model, tea.Cmd)` containing the existing `switch action` body
- [x] split `handleTreeNav` identically: thin wrapper + new `handleTreeAction(action, msg)` core
- [x] update `handleKey` to replace the current post-Resolve inline code with `return m.dispatchAction(action, msg)`
- [x] verify all existing tests in `diffnav_test.go`, `model_test.go`, `handlers_test.go` still pass — this is a refactor, no behavior change expected
- [x] write `TestDispatchAction_Resolves` — table-driven test exercising the dispatch matrix: pass each action constant, assert the correct handler is invoked (use a moq for any external dep that needs verification)
- [x] write `TestDispatchAction_PaneNavFallback` — when action is a navigation action (Down, Up, etc.) and focus is paneDiff, assert `handleDiffAction` invoked; same for paneTree
- [x] write `TestDispatchAction_OverlayOpen` — when action is ActionHelp, assert overlay opens
- [x] write `TestHandleDiffNav_StillWorks` — sanity check that the thin wrapper produces identical results to pre-refactor (use existing nav tests as reference)
- [x] run `make test` — must pass before task 6

### Task 6: Model state (keyState) + handleChordSecond

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/view.go` (add `m.keys.hint` to `transientHint()` priority chain)
- Modify: `app/ui/model_test.go` (or create `app/ui/chord_test.go` if growing)

- [x] add `keyState` struct in `app/ui/model.go` with two fields: `chordPending string` and `hint string`; godoc comment notes it's separate from `navigationState` because chord state is key-dispatch concern, not cursor/scroll concern (per architect feedback — avoid `navigationState` becoming a grab-bag)
- [x] add `keys keyState` field to Model struct (plain struct, not pointer)
- [x] in `view.go` `transientHint()`, add a new case `case m.keys.hint != "": return m.keys.hint` AFTER the existing commits/reload/compact cases (lowest priority — chord hints lose to in-flight reload/compact toggles)
- [x] in `handleKey` (model.go:714-716), add `m.keys.hint = ""` to the existing hint-clear block
- [x] add `handleChordSecond(keyStr string) (tea.Model, tea.Cmd)` method with VALUE receiver (matches `handlePendingReload` pattern at model.go:767): clears `m.keys.chordPending` and `m.keys.hint` on the local copy; returns `m, nil` on esc; calls `m.keymap.ResolveChord(prefix, keyStr)` and dispatches via `m.dispatchAction(action, /* synthesize msg */)` if non-empty; sets `m.keys.hint = "Unknown chord: " + prefix + ">" + keyStr` otherwise; returns updated `m` so bubbletea picks up state changes
- [x] for the `dispatchAction` call inside `handleChordSecond`, synthesize a placeholder `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(keyStr)}` (the second-stage key as msg context); pane-nav handlers that consult `msg.String()` will see the literal second key, which is correct context for any chord-bound nav action
- [x] write `TestHandleChordSecond_ResolvedDispatches` — set chordPending, call handleChordSecond with bound key, assert action invoked (via dispatchAction) + chordPending cleared + hint cleared
- [x] write `TestHandleChordSecond_UnboundShowsHint` — set chordPending, call with unbound key, assert `m.keys.hint == "Unknown chord: ctrl+w>q"` + chordPending cleared + no action invoked
- [x] write `TestHandleChordSecond_EscCancels` — set chordPending, call with "esc", assert chordPending cleared + hint cleared + no action invoked
- [x] write `TestHandleChordSecond_LayoutFallback` — chord `ctrl+w>x` bound, set chordPending to `ctrl+w`, call with `ч`, assert action invoked (verifies ResolveChord's layout-fallback path)
- [x] write `TestTransientHint_ChordHintLowestPriority` — set `m.commits.hint = "x"` AND `m.keys.hint = "y"`, assert `transientHint()` returns "x" (commits wins)
- [x] run `make test` — must pass before task 7

### Task 7: handleKey integration — chord-first + chord-second guards

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/ui/model_test.go`

- [x] in `handleKey` (model.go:711), insert chord-second guard immediately after the pending-reload check (model.go:719-721) and BEFORE the `handleModalKey` call (model.go:723): `if m.keys.chordPending != "" { return m.handleChordSecond(msg.String()) }`
- [x] in `handleKey`, insert chord-first guard immediately after `action := m.keymap.Resolve(msg.String())` (model.go:727) and BEFORE the `dispatchAction` call (replacing the old `handleOverlayOpen` line at model.go:729 — see Task 5 refactor): `if action == "" && m.keymap.IsChordLeader(msg.String()) { m.keys.chordPending = msg.String(); m.keys.hint = "Pending: " + msg.String() + ", esc to cancel"; return m, nil }`
- [x] write `TestHandleKey_EntersChordPending` — bind `ctrl+w>x`, send `tea.KeyMsg{Type: tea.KeyCtrlW}`, assert `m.keys.chordPending == "ctrl+w"` + `m.keys.hint == "Pending: ctrl+w, esc to cancel"`
- [x] write `TestHandleKey_ChordSecondCoexistenceGuard` — defense-in-depth test: set `m.keys.chordPending = "ctrl+w"` AND `m.search.active = true` simultaneously (simulating buggy coexistence), send a printable key like `x`, assert chord-second resolves (action dispatched if `ctrl+w>x` bound; otherwise hint set) and search input is NOT mutated
- [x] write `TestHandleKey_ChordIgnoredWhenPendingReload` — set `m.reload.pending = true`, send a chord leader key, assert reload-confirmation handler runs (chord not entered, chordPending stays empty)
- [x] write `TestHandleKey_LeaderWithStandaloneActionDoesNotEnterChord` — sanity check: bind `ctrl+w` standalone (not a chord), send `ctrl+w`, assert standalone action fires + chordPending stays empty (verifies `action == ""` guard prevents accidental chord-entry on non-chord-leader keys)
- [x] run `make test` — must pass before task 8

### Task 8: Clear chord state when entering modal/overlay modes

**Files:**
- Modify: `app/ui/search.go` (`startSearch`)
- Modify: `app/ui/annotate.go` (`startAnnotation`)
- Modify: `app/ui/model.go` (`handleOverlayOpen`)
- Modify: `app/ui/model_test.go`

The actual modal-entry sites are exactly three: `startSearch` (search.go:11), `startAnnotation` (annotate.go:53), and `handleOverlayOpen` (model.go:775 — covers all four overlay kinds: help, annot-list, theme-select, commit-info at one site). Filter (`ActionFilter`) is NOT a modal — it's a regular tree-state toggle, no textinput, no chord-clear needed.

Three identical two-statement clears (`m.keys.chordPending = ""; m.keys.hint = ""`) are spread across three sites with a shared invariant (both fields must be cleared together). Add a private helper to keep them consistent.

- [x] add private method `func (m *Model) clearChordState() { m.keys.chordPending = ""; m.keys.hint = "" }` in `app/ui/model.go` (or `chord.go` if extracted)
- [x] in `startSearch` (search.go:11), call `m.clearChordState()` at the top
- [x] in `startAnnotation` (annotate.go:53), call `m.clearChordState()` at the top
- [x] in `handleOverlayOpen` (model.go:775), call `m.clearChordState()` at the top (before the switch — covers all 4 overlay kinds at one site)
- [x] write `TestClearChordState` — set both fields, call `clearChordState()`, assert both cleared
- [x] write `TestHandleOverlayOpen_HelpClearsChord` — set chordPending+hint, call `handleOverlayOpen(ActionHelp)`, assert both cleared + overlay opened
- [x] write `TestHandleOverlayOpen_AnnotListClearsChord` — same with `ActionAnnotList`
- [x] write `TestHandleOverlayOpen_ThemeSelectClearsChord` — same with `ActionThemeSelect`
- [x] write `TestHandleOverlayOpen_CommitInfoClearsChord` — same with `ActionCommitInfo`
- [x] write `TestStartSearch_ClearsChord` — set chordPending+hint, call `startSearch`, assert both cleared
- [x] write `TestStartAnnotation_ClearsChord` — set chordPending+hint, call `startAnnotation`, assert both cleared
- [x] run `make test` — must pass before task 9

### Task 9: Integration test matrix — handleKey precedence

**Files:**
- Modify: `app/ui/model_test.go` (or create `app/ui/chord_test.go` if it grows past ~150 lines and the existing model_test.go is large)

- [x] add table-driven test `TestHandleKey_ChordPrecedence` covering the dispatch matrix:
  | initial state | key sent | expected outcome |
  |---|---|---|
  | clean | leader | chord pending set, hint shown, no action |
  | chord pending | bound second | action dispatched, pending cleared, hint cleared |
  | chord pending | unbound second | "Unknown chord" hint, pending cleared, no action |
  | chord pending | esc | pending cleared, hint cleared, no action (silent cancel) |
  | chord pending | leader again | chord-second consumes leader as second-stage (Unknown chord since leader>leader is unbound), pending cleared |
  | annotate active | leader | `handleModalKey` consumes key (textinput owns it), chord NOT entered |
  | search active | leader | `handleModalKey` consumes key (textinput owns it), chord NOT entered |
  | overlay active | leader | `handleModalKey` routes to overlay's HandleKey, chord NOT entered |
  | pending reload | leader | `handlePendingReload` intercepts, chord NOT entered |
- [x] add `TestHandleKey_NonKeyMessagesPreserveChordState` — set chordPending, send a non-key message (e.g., `tea.WindowSizeMsg`, `filesLoadedMsg`, `blameLoadedMsg`), assert chordPending stays set (non-key messages route through `Update`, not `handleKey`, so they cannot affect chord state — this test locks in that invariant via direct `Update` invocation)
- [x] run `make test` — must pass before task 10

### Task 10: Documentation updates (README, docs.html, plugin reference docs)

**Files:**
- Modify: `README.md`
- Modify: `site/docs.html`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `plugins/codex/skills/revdiff/references/usage.md`

- [x] in `README.md` keybindings section, add a paragraph explaining chord syntax: leader must be `ctrl+*` or `alt+*`; second stage is any single key; format `map ctrl+w>x close_file`; while pending, status bar shows `Pending: ctrl+w, esc to cancel`; press `esc` to cancel; only 2-stage chords supported
- [x] mirror the README change in `site/docs.html` (same wording, HTML-formatted)
- [x] add identical paragraph to `.claude-plugin/skills/revdiff/references/usage.md`
- [x] copy the same paragraph BYTE-IDENTICAL to `plugins/codex/skills/revdiff/references/usage.md` (per `feedback_revdiff-docs-sync-plugins.md`)
- [x] verify byte-identity: `diff .claude-plugin/skills/revdiff/references/usage.md plugins/codex/skills/revdiff/references/usage.md` shows no differences (or only differences that pre-existed before this PR)
- [x] no test changes; run `make test` to confirm nothing broke

### Task 11: Verify acceptance criteria

- [x] verify chord parser accepts `ctrl+w>x` and `alt+t>n`
- [x] verify parser rejects `g>g`, `ctrl+w>x>y`, `ctrl+w>` with appropriate warnings
- [x] verify standalone `ctrl+w` is dropped when chord `ctrl+w>x` exists
- [x] verify pressing leader key enters pending state with status hint
- [x] verify pressing bound second key dispatches the action
- [x] verify pressing unbound second key shows "Unknown chord" hint
- [x] verify pressing esc cancels chord silently
- [x] verify entering search/annotate/filter/overlay clears chord state
- [x] verify chord works under non-Latin keyboard layout (second key translates via layoutResolve)
- [x] verify Dump round-trips chord bindings without loss
- [x] verify help overlay (`?`) shows chord keys verbatim alongside single-key bindings
- [x] run full test suite: `make test`
- [x] run linter: `make lint`
- [x] verify test coverage for new code is >= existing project coverage for affected packages (`make test` reports per-package coverage)

### Task 12: Final — update CLAUDE.md and move plan to completed

**Files:**
- Modify: `CLAUDE.md`

- [ ] add a brief entry in CLAUDE.md's "Gotchas" or "Architecture" section noting that chord keybindings exist, leaders must be ctrl/alt, and chord state lives on `keyState` (not `navigationState`)
- [ ] move this plan file to `docs/plans/completed/20260423-leader-chord-keybindings.md`

## Task Order (revised)

1. Keymap chord parser + validation
2. Conflict resolution + lazy chord-prefix index
3. ResolveChord with layout-resolve fallback
4. Dump round-trip + KeysFor includes chord keys
5. **Extract dispatchAction helper + split pane-nav handlers** (NEW — refactor required for chord-second to dispatch correctly)
6. Model state (keyState) + handleChordSecond
7. handleKey integration — chord-first + chord-second guards
8. Clear chord state when entering modal/overlay modes
9. Integration test matrix — handleKey precedence
10. Documentation updates (README, docs.html, plugin reference docs)
11. Verify acceptance criteria
12. Final — update CLAUDE.md and move plan to completed

## Post-Completion

*Items requiring manual intervention or external systems — informational only*

**Manual verification scenarios:**
- launch revdiff with `~/.config/revdiff/keybindings` containing a chord (e.g., `map ctrl+w>1 next_item`); press `Ctrl+W` then `1`; verify the bound action fires and the status bar updates
- press `Ctrl+W` then `q`; verify "Unknown chord: ctrl+w>q" hint appears and `q` does NOT quit the app
- press `Ctrl+W` then `esc`; verify pending state clears silently with no hint or action
- with chord pending, press `?` to open help overlay; verify chord state is cleared (no orphan hint behind the overlay)
- with chord pending, press `/` to open search; verify chord state is cleared (search input doesn't see leftover chord state)
- under Cyrillic keyboard layout, verify `Ctrl+W` then physically pressing `ч` (which maps to `x`) resolves a `ctrl+w>x` chord binding correctly

**External system updates:**
- none — feature is self-contained in revdiff
- if downstream packagers (homebrew, AUR) build from this version, no packaging changes needed
