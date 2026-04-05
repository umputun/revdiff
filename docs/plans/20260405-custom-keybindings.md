# Custom Keybindings Support

## Overview
- Add user-configurable keybindings via a separate file (`~/.config/revdiff/keybindings`)
- Kitty-style format: `map <key> <action>` / `unmap <key>` with `#` comments
- All main navigation/action keys remappable (~30 unified action names)
- Defaults preserved unless explicitly overridden; modal keys (annotation input, search input, help overlay, confirm discard) stay hardcoded
- Dynamic help overlay reflects actual effective bindings
- New CLI flags: `--keys` (path override) and `--dump-keys` (print effective bindings)
- Deferred: chord/sequence bindings (ctrl+a>b), per-pane overrides

## Context (from discovery)
- **Key handlers**: `handleKey` (ui/model.go:234), `handleDiffNav` (:521), `handleTreeNav` (:436), `handleTOCNav` (:470) — ~40 hardcoded key→action mappings across switch/case statements
- **Help overlay**: `helpOverlay()` (ui/model.go:1124) — one concatenated string literal, ~45 lines of hardcoded text
- **Config system**: `cmd/revdiff/main.go` — go-flags `IniParser`, `--config` flag, `--dump-config`, `resolveConfigPath()` pattern
- **Model constructor**: `NewModel(renderer, store, hl, ModelConfig)` at ui/model.go:140 — `ModelConfig` struct passes options in
- **Test patterns**: `testModel()` helper, key injection via `tea.KeyMsg{Type: ..., Runes: ...}`, ~180 test functions in model_test.go

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility — zero behavior change with no keybindings file

## Testing Strategy
- **Unit tests**: keymap package (parse, load, default, action lookup, dump, help entries)
- **Unit tests**: ui handler refactor (verify all existing key tests still pass with default keymap)
- **Unit tests**: custom binding tests (override a key, verify new binding works, old one doesn't)
- **Integration tests**: CLI flags (--keys, --dump-keys)

## Solution Overview

### Action Names (~30 unified actions)
Actions are pane-agnostic — `down` means "down in whatever pane has focus":

**Navigation**: `down`, `up`, `page_down`, `page_up`, `half_page_down`, `half_page_up`, `home`, `end`, `scroll_left`, `scroll_right`
**File/hunk**: `next_item`, `prev_item`, `next_hunk`, `prev_hunk`
**Pane**: `toggle_pane`, `focus_tree`, `focus_diff`
**Search**: `search`
**Annotations**: `confirm` (context-aware: annotate in diff, focus/jump in tree/TOC), `annotate_file`, `delete_annotation`, `annot_list`
**View toggles**: `toggle_collapsed`, `toggle_wrap`, `toggle_tree`, `toggle_line_numbers`, `filter`, `toggle_hunk`
**Quit**: `quit`, `discard_quit`, `help`, `dismiss` (esc)

Note on context-dependent keys:
- `next_item` / `prev_item`: context-aware — navigates files in tree, search matches when search is active, TOC headers in TOC mode. The handler checks state internally (same as current code).
- `confirm` (enter): in tree pane = focus diff, in TOC pane = jump to header, in diff pane = start annotation. Named `confirm` (not `annotate`) because it's the generic "primary action" key.
- `scroll_left` / `scroll_right`: in tree/TOC panes, `scroll_right` is treated as `focus_diff` (tree has no horizontal scroll). This is implicit fallback behavior, not a separate mapping.
- `search`: only works in diff pane (same as current behavior). No-op in tree/TOC panes.

### Default Bindings
```
map j down
map k up
map down down
map up up
map pgdown page_down
map pgup page_up
map ctrl+d half_page_down
map ctrl+u half_page_up
map home home
map end end
map left scroll_left
map right scroll_right
map n next_item
map N prev_item
map p prev_item
map ] next_hunk
map [ prev_hunk
map tab toggle_pane
map h focus_tree
map l focus_diff
map / search
map a confirm
map enter confirm
map A annotate_file
map d delete_annotation
map @ annot_list
map v toggle_collapsed
map w toggle_wrap
map t toggle_tree
map L toggle_line_numbers
map . toggle_hunk
map f filter
map q quit
map Q discard_quit
map ? help
map esc dismiss
```

Note: see "Note on context-dependent keys" in Action Names section above for `n`/`N`, `enter`, `right` behavior.

### File Format
```
# ~/.config/revdiff/keybindings
# format: map <key> <action>
# use "unmap <key>" to remove a default binding
# blank lines and # comments are ignored

map ctrl+d half_page_down
map ctrl+u half_page_up
unmap j
```

### Architecture
- **`keymap/` package**: `Keymap` type (thin wrapper around `map[string]Action`), parser, defaults, dump, help generation
- **`Action` type**: string enum with constants for all action names
- **`Keymap.Resolve(key string) Action`**: looks up key → action, returns empty string if unbound
- **Reverse lookup**: `Keymap.KeysFor(action Action) []string` — needed for help overlay (show which keys trigger an action)
- **Help entries**: `Keymap.HelpSections() []HelpSection` — returns grouped entries for the help overlay, with descriptions stored alongside defaults
- **Model integration**: `Model.keymap` field, set via `ModelConfig.Keymap`
- **Handler refactor**: switch on `m.keymap.Resolve(msg.String())` instead of `msg.String()`

### Key Representation
Keys are stored as the string that `tea.KeyMsg.String()` returns:
- Single chars: `"j"`, `"k"`, `"a"`, `"/"`, `"?"`, `"@"`, `"["`, `"]"`, `"."`
- Modifiers: `"ctrl+d"`, `"ctrl+u"`
- Special keys: `"enter"`, `"esc"`, `"tab"`, `"up"`, `"down"`, `"left"`, `"right"`, `"home"`, `"end"`, `"pgdown"`, `"pgup"`

The parser normalizes input to match bubbletea's `KeyMsg.String()` output. Special key name mapping in parser must be verified against actual bubbletea values — Task 1 tests must assert `Default()` resolves correctly for all special keys using real `tea.KeyMsg` values (not hardcoded strings).

**IMPORTANT**: current code uses `msg.Type == tea.KeyPgDown` (type check) not `msg.String()` for special keys. After refactor, `Resolve(msg.String())` must return the correct action. This requires knowing the exact `.String()` output for `KeyPgDown`, `KeyPgUp`, `KeyHome`, `KeyEnd`. If bubbletea's `.String()` doesn't match user-friendly names, the parser normalizes both directions.

## Implementation Steps

### Task 1: Create keymap package with types and defaults

**Files:**
- Create: `keymap/keymap.go`
- Create: `keymap/keymap_test.go`

- [x] define `Action` type (string) with constants for all ~30 actions
- [x] define `HelpEntry` struct: `Action`, `Description`, `Section` (navigation/search/annotations/view/quit)
- [x] define `HelpSection` struct: `Name`, `Entries []HelpEntry`
- [x] define `Keymap` struct with internal `bindings map[string]Action` and `descriptions map[Action]HelpEntry`
- [x] implement `Default() *Keymap` returning all default bindings from the table above
- [x] implement `Resolve(key string) Action` — returns action for key, empty if unbound
- [x] implement `KeysFor(action Action) []string` — reverse lookup for help overlay
- [x] implement `HelpSections() []HelpSection` — returns grouped help entries with effective keys
- [x] write tests for Default() — verify all expected bindings exist
- [x] write tests for Resolve() — lookup, missing key, overridden key
- [x] write tests for KeysFor() — single key, multiple keys (j and down both map to "down")
- [x] write tests for HelpSections() — verify sections and key display
- [x] run tests, run linter

### Task 2: Add keybindings file parser

**Files:**
- Modify: `keymap/keymap.go`
- Modify: `keymap/keymap_test.go`

- [x] implement `Parse(r io.Reader) (maps []mapEntry, unmaps []string, err error)` — parses `map`/`unmap` lines, skips comments and blanks
- [x] implement key name normalization (handle `pgdown`→bubbletea string, `ctrl+d` already matches)
- [x] implement `Load(path string) (*Keymap, error)` — reads file, calls Parse, applies overrides on Default()
- [x] implement `LoadOrDefault(path string) *Keymap` — returns Default() if file doesn't exist, warns on parse errors
- [x] handle edge cases: unknown action names (warn + skip), duplicate mappings (last wins), unmap of unbound key (no-op)
- [x] write tests for Parse() — valid map, unmap, comments, blank lines, invalid lines
- [x] write tests for Load() — file with overrides, file with unmaps, missing file, malformed lines
- [x] write tests for key normalization
- [x] run tests, run linter

### Task 3: Add Dump functionality

**Files:**
- Modify: `keymap/keymap.go`
- Modify: `keymap/keymap_test.go`

- [x] implement `Dump(w io.Writer)` — writes effective bindings in `map <key> <action>` format, grouped by section with `#` comments
- [x] write test for Dump() — verify output format, round-trip (dump then parse produces same keymap)
- [x] run tests, run linter

### Task 4: Wire keymap into Model and CLI

**Files:**
- Modify: `ui/model.go` (ModelConfig, NewModel, Model struct)
- Modify: `cmd/revdiff/main.go` (options struct, parseArgs, run)
- Modify: `cmd/revdiff/main_test.go`

- [x] add `Keymap *keymap.Keymap` field to `ModelConfig`
- [x] add `keymap *keymap.Keymap` field to `Model` struct, set in `NewModel()`. If `cfg.Keymap == nil`, default to `keymap.Default()` — this ensures all ~180 existing tests work without modification (same pattern as TreeWidthRatio defaulting)
- [x] add `Keys string` and `DumpKeys bool` fields to `options` struct with appropriate go-flags tags (`--keys`, `--dump-keys`, `no-ini:"true"`)
- [x] add `resolveKeysPath()` function (same pattern as `resolveConfigPath` — check --keys flag, env `REVDIFF_KEYS`, default `~/.config/revdiff/keybindings`)
- [x] call `keymap.LoadOrDefault(keysPath)` in `run()`, pass to `ModelConfig`
- [x] handle `--dump-keys`: call `km.Dump(os.Stdout)` and exit (same pattern as `--dump-config`)
- [x] write tests for --keys flag parsing, --dump-keys flag, resolveKeysPath()
- [x] run tests, run linter
- [x] run `go mod vendor` if keymap package needs vendoring (internal package, should not)

### Task 5: Refactor handleKey to use keymap

**Files:**
- Modify: `ui/model.go` (handleKey function)
- Modify: `ui/model_test.go`

- [x] refactor `handleKey()` (ui/model.go:234-289) to switch on `m.keymap.Resolve(msg.String())` instead of raw key strings
- [x] keep modal checks at top unchanged (annotating, searching, showAnnotList, showHelp — these stay hardcoded)
- [x] map current cases: `"@"` → `annot_list`, `"esc"` → `dismiss`, `"Q"` → `discard_quit`, `"q"` → `quit`, `"tab"` → `toggle_pane`, `"f"` → `filter`, `"n"`/`"N"` → `next_item`/`prev_item` (handler checks search state internally), `"p"` → `prev_item`, `"enter"` → `confirm`, `"A"` → `annotate_file`, view toggle actions (`toggle_collapsed`, `toggle_wrap`, `toggle_tree`, `toggle_line_numbers`)
- [x] refactor `handleViewToggle()` to accept/switch on action names (not raw key strings) — otherwise custom bindings like `map x toggle_wrap` won't work since `handleViewToggle("x")` won't match
- [x] verify all existing handleKey tests still pass with default keymap
- [x] write test: override a global key (e.g., map `x` to `quit`), verify `x` quits and `q` doesn't
- [x] run tests, run linter

### Task 6: Refactor handleDiffNav to use keymap

**Files:**
- Modify: `ui/model.go` (handleDiffNav function)
- Modify: `ui/model_test.go`

- [ ] refactor `handleDiffNav()` (ui/model.go:521-568) to switch on `m.keymap.Resolve(msg.String())`
- [ ] map: `h` → `focus_tree`, `left/right` → `scroll_left/scroll_right`, `j/down` → `down`, `k/up` → `up`, `pgdown` → `page_down`, `ctrl+d` → `half_page_down`, `pgup` → `page_up`, `ctrl+u` → `half_page_up`, `home/end` → `home/end`, `]/[` → `next_hunk/prev_hunk`, `a` → `annotate`, `d` → `delete_annotation`, `.` → `toggle_hunk`, `/` → `search`
- [ ] verify all existing diff nav tests still pass
- [ ] write test: custom diff binding (e.g., map `x` to `next_hunk`)
- [ ] run tests, run linter

### Task 7: Refactor handleTreeNav and handleTOCNav to use keymap

**Files:**
- Modify: `ui/model.go` (handleTreeNav, handleTOCNav functions)
- Modify: `ui/model_test.go`

- [ ] refactor `handleTreeNav()` (ui/model.go:436-467) to switch on `m.keymap.Resolve(msg.String())`
- [ ] refactor `handleTOCNav()` (ui/model.go:470-505) to switch on `m.keymap.Resolve(msg.String())`
- [ ] both use same action names: `down`, `up`, `page_down`, `half_page_down`, `page_up`, `half_page_up`, `home`, `end`, `focus_diff`
- [ ] tree/TOC handlers must also accept `scroll_right` as `focus_diff` (tree has no horizontal scroll; `right` key maps to `scroll_right` globally but means "focus diff" in tree — implicit fallback)
- [ ] verify all existing tree/TOC nav tests still pass
- [ ] write test: custom tree binding
- [ ] run tests, run linter

### Task 8: Dynamic help overlay

**Files:**
- Modify: `ui/model.go` (helpOverlay function)
- Modify: `ui/model_test.go`

- [ ] replace hardcoded `helpOverlay()` string with dynamic rendering from `m.keymap.HelpSections()`
- [ ] format each section: section name header, then `"  key1 / key2    description"` lines with aligned columns
- [ ] handle actions with no keys (unmapped) — skip them in help
- [ ] handle actions with multiple keys — show as `"key1 / key2"` (same as current `"j / k"` format)
- [ ] verify help overlay tests still pass (content will change if bindings are customized but structure stays same)
- [ ] write test: custom binding appears in help overlay
- [ ] write test: unmapped action doesn't appear in help
- [ ] run tests, run linter

### Task 9: Verify acceptance criteria

- [ ] verify: no keybindings file → identical behavior to current (all defaults work)
- [ ] verify: `map x quit` in keybindings file → `x` quits, `q` still quits (additive)
- [ ] verify: `unmap q` + `map x quit` → only `x` quits
- [ ] verify: `--dump-keys` prints all effective bindings
- [ ] verify: `--keys /path/to/file` loads custom file
- [ ] verify: help overlay reflects custom bindings
- [ ] verify: invalid action names in file produce warning, don't crash
- [ ] run full test suite: `go test -race ./...`
- [ ] run linter: `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`

### Task 10: [Final] Update documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `.claude-plugin/skills/revdiff/references/config.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`

- [ ] add keybindings section to README.md (file location, format, example, --keys flag, --dump-keys)
- [ ] update CLAUDE.md with keymap package info, action names, keybindings file format
- [ ] update plugin reference docs (config.md with --keys/--dump-keys, usage.md with keybinding customization)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- test with actual keybindings file on real diffs
- verify help overlay looks correct with custom bindings (column alignment)
- verify --dump-keys output can be used as a starting template (copy, modify, load)

**Future enhancements (deferred):**
- chord/sequence bindings (ctrl+a>b) — architecture supports it via state machine in Resolve
- per-pane overrides (diff.down vs tree.down) — architecture supports it via namespaced action lookup
- plugin version bump after merge (ask user)
