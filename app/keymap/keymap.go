// Package keymap provides user-configurable key bindings for revdiff.
// It maps key names (as returned by bubbletea's KeyMsg.String()) to action names.
package keymap

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Action represents a named action that a key can trigger.
type Action string

// action constants for all mappable actions.
const (
	ActionDown             Action = "down"
	ActionUp               Action = "up"
	ActionPageDown         Action = "page_down"
	ActionPageUp           Action = "page_up"
	ActionHalfPageDown     Action = "half_page_down"
	ActionHalfPageUp       Action = "half_page_up"
	ActionHome             Action = "home"
	ActionEnd              Action = "end"
	ActionScrollLeft       Action = "scroll_left"
	ActionScrollRight      Action = "scroll_right"
	ActionScrollCenter     Action = "scroll_center"
	ActionScrollTop        Action = "scroll_top"
	ActionScrollBottom     Action = "scroll_bottom"
	ActionNextItem         Action = "next_item"
	ActionPrevItem         Action = "prev_item"
	ActionNextHunk         Action = "next_hunk"
	ActionPrevHunk         Action = "prev_hunk"
	ActionTogglePane       Action = "toggle_pane"
	ActionFocusTree        Action = "focus_tree"
	ActionFocusDiff        Action = "focus_diff"
	ActionSearch           Action = "search"
	ActionConfirm          Action = "confirm"
	ActionAnnotateFile     Action = "annotate_file"
	ActionDeleteAnnotation Action = "delete_annotation"
	ActionAnnotList        Action = "annot_list"
	ActionNextAnnotation   Action = "next_annotation"
	ActionPrevAnnotation   Action = "prev_annotation"
	ActionToggleCollapsed  Action = "toggle_collapsed"
	ActionToggleCompact    Action = "toggle_compact"
	ActionToggleWrap       Action = "toggle_wrap"
	ActionToggleTree       Action = "toggle_tree"
	ActionToggleLineNums   Action = "toggle_line_numbers"
	ActionToggleBlame      Action = "toggle_blame"
	ActionToggleWordDiff   Action = "toggle_word_diff"
	ActionToggleHunk       Action = "toggle_hunk"
	ActionToggleUntracked  Action = "toggle_untracked"
	ActionMarkReviewed     Action = "mark_reviewed"
	ActionFilter           Action = "filter"
	ActionQuit             Action = "quit"
	ActionDiscardQuit      Action = "discard_quit"
	ActionHelp             Action = "help"
	ActionDismiss          Action = "dismiss"
	ActionThemeSelect      Action = "theme_select"
	ActionInfo             Action = "info"
	ActionReload           Action = "reload"
	ActionOpenEditor       Action = "open_editor"
)

// SectionPane is the help section name for pane-related keybindings.
const SectionPane = "Pane"

// validActions contains all known action names for validation.
var validActions = map[Action]bool{
	ActionDown: true, ActionUp: true, ActionPageDown: true, ActionPageUp: true,
	ActionHalfPageDown: true, ActionHalfPageUp: true, ActionHome: true, ActionEnd: true,
	ActionScrollLeft: true, ActionScrollRight: true,
	ActionScrollCenter: true, ActionScrollTop: true, ActionScrollBottom: true,
	ActionNextItem: true, ActionPrevItem: true, ActionNextHunk: true, ActionPrevHunk: true,
	ActionTogglePane: true, ActionFocusTree: true, ActionFocusDiff: true,
	ActionSearch:  true,
	ActionConfirm: true, ActionAnnotateFile: true, ActionDeleteAnnotation: true, ActionAnnotList: true,
	ActionNextAnnotation: true, ActionPrevAnnotation: true,
	ActionToggleCollapsed: true, ActionToggleCompact: true, ActionToggleWrap: true, ActionToggleTree: true,
	ActionToggleLineNums: true, ActionToggleBlame: true, ActionToggleWordDiff: true, ActionToggleHunk: true,
	ActionMarkReviewed: true, ActionFilter: true, ActionToggleUntracked: true,
	ActionQuit: true, ActionDiscardQuit: true, ActionHelp: true, ActionDismiss: true, ActionThemeSelect: true,
	ActionInfo:       true,
	ActionReload:     true,
	ActionOpenEditor: true,
}

// deprecatedActionAliases maps obsolete action names parsed from user
// keybinding files onto their canonical replacement. The action was renamed
// from "commit_info" to "info" when the popup expanded to cover description
// and aggregate stats; honoring the old name lets pre-existing
// ~/.config/revdiff/keybindings files keep working without manual edits.
// The parser surfaces a single [WARN] per deprecated alias for the lifetime
// of the process (see warnOnceDeprecatedAlias) so that a file with several
// "map ... commit_info" lines does not spam the log.
var deprecatedActionAliases = map[Action]Action{
	"commit_info": ActionInfo,
}

// IsValidAction returns true if the action name is recognized. Deprecated
// aliases also report true so the parser accepts them; resolveAction performs
// the rewrite to the canonical name before storage.
func IsValidAction(a Action) bool {
	if validActions[a] {
		return true
	}
	_, ok := deprecatedActionAliases[a]
	return ok
}

// resolveAction returns the canonical Action for a, rewriting any deprecated
// alias to its replacement. ok is false when a is neither a valid action nor a
// known alias. Returns the canonical action plus a deprecated flag so callers
// can surface a one-time warning to the user.
func resolveAction(a Action) (canonical Action, deprecated, ok bool) {
	if validActions[a] {
		return a, false, true
	}
	if alias, found := deprecatedActionAliases[a]; found {
		return alias, true, true
	}
	return "", false, false
}

// loggedDeprecatedAliases tracks which deprecated aliases have already
// surfaced a [WARN] line during the program's lifetime. Process-wide so a
// keybindings file with several occurrences of "map i commit_info" produces
// exactly one warning instead of one per line; mirrors the behavior promised
// by the PR introducing the alias.
var loggedDeprecatedAliases sync.Map

// warnOnceDeprecatedAlias logs a deprecation warning the first time alias is
// observed in the running process. Subsequent calls with the same alias are
// no-ops. Called from parse() when resolveAction reports deprecated=true.
func warnOnceDeprecatedAlias(alias, canonical Action) {
	if _, loaded := loggedDeprecatedAliases.LoadOrStore(string(alias), struct{}{}); loaded {
		return
	}
	log.Printf("[WARN] keybindings: action %q is deprecated, use %q", alias, canonical)
}

// HelpEntry describes a single action for the help overlay.
type HelpEntry struct {
	Action      Action
	Description string
	Section     string
}

// HelpSection groups help entries under a section heading.
type HelpSection struct {
	Name    string
	Entries []HelpEntryWithKeys
}

// HelpEntryWithKeys is a help entry with the effective key bindings attached.
type HelpEntryWithKeys struct {
	Action      Action
	Description string
	Keys        string // formatted as "key1 / key2"
}

// Keymap maps key names to actions. Keys are stored as the string returned
// by bubbletea's tea.KeyMsg.String().
type Keymap struct {
	bindings         map[string]Action
	descriptions     []HelpEntry         // ordered list of action descriptions
	chordPrefixCache map[string]struct{} // lazy cache of chord leader keys; nil = not yet built
}

// defaultDescriptions returns the ordered help entries grouped by section.
func defaultDescriptions() []HelpEntry {
	return []HelpEntry{
		// navigation
		{ActionDown, "move cursor down", "Navigation"},
		{ActionUp, "move cursor up", "Navigation"},
		{ActionPageDown, "page down", "Navigation"},
		{ActionPageUp, "page up", "Navigation"},
		{ActionHalfPageDown, "half page down", "Navigation"},
		{ActionHalfPageUp, "half page up", "Navigation"},
		{ActionHome, "go to top", "Navigation"},
		{ActionEnd, "go to bottom", "Navigation"},
		{ActionScrollLeft, "scroll left", "Navigation"},
		{ActionScrollRight, "scroll right / focus diff", "Navigation"},
		{ActionScrollCenter, "center viewport on cursor", "Navigation"},
		{ActionScrollTop, "align viewport top", "Navigation"},
		{ActionScrollBottom, "align viewport bottom", "Navigation"},

		// file/hunk
		{ActionNextItem, "next file / search match", "File/Hunk"},
		{ActionPrevItem, "prev file / search match", "File/Hunk"},
		{ActionNextHunk, "next hunk", "File/Hunk"},
		{ActionPrevHunk, "prev hunk", "File/Hunk"},

		// pane
		{ActionTogglePane, "toggle pane focus", SectionPane},
		{ActionFocusTree, "focus tree pane", SectionPane},
		{ActionFocusDiff, "focus diff pane", SectionPane},

		// search
		{ActionSearch, "search in diff", "Search"},

		// annotations
		{ActionConfirm, "annotate line / select file", "Annotations"},
		{ActionAnnotateFile, "annotate file", "Annotations"},
		{ActionDeleteAnnotation, "delete annotation", "Annotations"},
		{ActionAnnotList, "annotation list", "Annotations"},
		{ActionOpenEditor, "open annotation in $EDITOR", "Annotations"},
		{ActionNextAnnotation, "next annotation (across files)", "Annotations"},
		{ActionPrevAnnotation, "previous annotation (across files)", "Annotations"},

		// view toggles
		{ActionToggleCollapsed, "toggle collapsed view", "View"},
		{ActionToggleCompact, "toggle compact diff view", "View"},
		{ActionToggleWrap, "toggle word wrap", "View"},
		{ActionToggleTree, "toggle tree pane", "View"},
		{ActionToggleLineNums, "toggle line numbers", "View"},
		{ActionToggleBlame, "toggle blame gutter", "View"},
		{ActionToggleWordDiff, "toggle word-diff highlighting", "View"},
		{ActionToggleHunk, "toggle hunk in collapsed", "View"},
		{ActionToggleUntracked, "show/hide untracked files", "View"},
		{ActionMarkReviewed, "mark file as reviewed", "View"},
		{ActionFilter, "filter files", "View"},
		{ActionThemeSelect, "theme selector", "View"},
		{ActionInfo, "show review info popup", "View"},
		{ActionReload, "reload diff from VCS", "View"},

		// quit
		{ActionQuit, "quit", "Quit"},
		{ActionDiscardQuit, "discard and quit", "Quit"},
		{ActionHelp, "show help", "Quit"},
		{ActionDismiss, "dismiss / cancel", "Quit"},
	}
}

// defaultBindings returns the default key-to-action mapping.
func defaultBindings() map[string]Action {
	return map[string]Action{
		"j":      ActionDown,
		"k":      ActionUp,
		"down":   ActionDown,
		"up":     ActionUp,
		"pgdown": ActionPageDown,
		"pgup":   ActionPageUp,
		"ctrl+d": ActionHalfPageDown,
		"ctrl+u": ActionHalfPageUp,
		"home":   ActionHome,
		"end":    ActionEnd,
		"left":   ActionScrollLeft,
		"right":  ActionScrollRight,
		"n":      ActionNextItem,
		"N":      ActionPrevItem,
		"p":      ActionPrevItem,
		"]":      ActionNextHunk,
		"[":      ActionPrevHunk,
		"tab":    ActionTogglePane,
		"h":      ActionFocusTree,
		"l":      ActionFocusDiff,
		"/":      ActionSearch,
		"a":      ActionConfirm,
		"enter":  ActionConfirm,
		"A":      ActionAnnotateFile,
		"d":      ActionDeleteAnnotation,
		"@":      ActionAnnotList,
		"ctrl+e": ActionOpenEditor,
		"}":      ActionNextAnnotation,
		"{":      ActionPrevAnnotation,
		"v":      ActionToggleCollapsed,
		"C":      ActionToggleCompact,
		"w":      ActionToggleWrap,
		"t":      ActionToggleTree,
		"L":      ActionToggleLineNums,
		"B":      ActionToggleBlame,
		"W":      ActionToggleWordDiff,
		".":      ActionToggleHunk,
		" ":      ActionMarkReviewed,
		"u":      ActionToggleUntracked,
		"f":      ActionFilter,
		"q":      ActionQuit,
		"Q":      ActionDiscardQuit,
		"?":      ActionHelp,
		"T":      ActionThemeSelect,
		"i":      ActionInfo,
		"R":      ActionReload,
		"esc":    ActionDismiss,
	}
}

// Default returns a Keymap with all default bindings.
func Default() *Keymap {
	return &Keymap{
		bindings:     defaultBindings(),
		descriptions: defaultDescriptions(),
	}
}

// NormalizeKey returns the Latin QWERTY equivalent of a single non-Latin key
// character, or the input unchanged when it has no mapping. Multi-character key
// strings (e.g. "esc", "ctrl+w") pass through unchanged. Used by dispatch paths
// that compare keys against literal bindings without going through Resolve, so
// non-Latin layouts behave identically to Latin ones.
func NormalizeKey(key string) string {
	if r, size := utf8.DecodeRuneInString(key); size == len(key) {
		if alias, ok := layoutResolve(r); ok {
			return string(alias)
		}
	}
	return key
}

// Resolve returns the action bound to the given key, or empty Action if unbound.
// For non-Latin keyboard layouts, if the key has no direct binding, it is
// translated to its Latin QWERTY equivalent and looked up again.
func (km *Keymap) Resolve(key string) Action {
	if a, ok := km.bindings[key]; ok {
		return a
	}
	// fallback: translate non-Latin character to Latin equivalent
	if r, size := utf8.DecodeRuneInString(key); size == len(key) {
		if alias, ok := layoutResolve(r); ok {
			if a, ok := km.bindings[string(alias)]; ok {
				return a
			}
		}
	}
	return ""
}

// ResolveChord returns the action bound to the chord (prefix, second), or empty
// Action if unbound. Applies a layout-resolve fallback to the second key: when
// the direct lookup misses and the second key is a single rune, the rune is
// translated to its Latin QWERTY equivalent and the lookup is retried.
func (km *Keymap) ResolveChord(prefix, second string) Action {
	if a, ok := km.bindings[prefix+">"+second]; ok {
		return a
	}
	if r, size := utf8.DecodeRuneInString(second); size == len(second) {
		if alias, ok := layoutResolve(r); ok {
			if a, ok := km.bindings[prefix+">"+string(alias)]; ok {
				return a
			}
		}
	}
	return ""
}

// KeysFor returns all keys bound to the given action, sorted alphabetically.
func (km *Keymap) KeysFor(action Action) []string {
	var keys []string
	for k, a := range km.bindings {
		if a == action {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Bind maps a key to an action, overriding any previous binding for that key.
func (km *Keymap) Bind(key string, action Action) {
	km.bindings[key] = action
	km.chordPrefixCache = nil
}

// Unbind removes the binding for the given key. No-op if key is not bound.
func (km *Keymap) Unbind(key string) {
	delete(km.bindings, key)
	km.chordPrefixCache = nil
}

// chordPrefixes returns the set of leader keys that have at least one chord binding.
// The result is built lazily on first call and cached until a Bind/Unbind invalidates it.
func (km *Keymap) chordPrefixes() map[string]struct{} {
	if km.chordPrefixCache != nil {
		return km.chordPrefixCache
	}
	cache := make(map[string]struct{})
	for k := range km.bindings {
		idx := strings.Index(k, ">")
		if idx <= 0 {
			continue
		}
		cache[k[:idx]] = struct{}{}
	}
	km.chordPrefixCache = cache
	return cache
}

// IsChordLeader returns true if the given key is the leader of any chord binding.
// Lookup is O(1) via a cached prefix index, built on first call.
func (km *Keymap) IsChordLeader(key string) bool {
	_, ok := km.chordPrefixes()[key]
	return ok
}

// HelpSections returns grouped help entries with effective key bindings.
// Actions with no bound keys are omitted.
func (km *Keymap) HelpSections() []HelpSection {
	// collect unique sections in order
	var sections []HelpSection
	sectionIdx := make(map[string]int)

	for _, desc := range km.descriptions {
		keys := km.KeysFor(desc.Action)
		if len(keys) == 0 {
			continue // skip unmapped actions
		}

		entry := HelpEntryWithKeys{
			Action:      desc.Action,
			Description: desc.Description,
			Keys:        strings.Join(keys, " / "),
		}

		idx, exists := sectionIdx[desc.Section]
		if !exists {
			idx = len(sections)
			sectionIdx[desc.Section] = idx
			sections = append(sections, HelpSection{Name: desc.Section})
		}
		sections[idx].Entries = append(sections[idx].Entries, entry)
	}

	return sections
}

// Dump writes the effective bindings to w in the keybindings file format,
// grouped by section with # comments. Output can be loaded back with Parse.
func (km *Keymap) Dump(w io.Writer) error {
	sections := km.HelpSections()
	for i, sec := range sections {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("dump keybindings: %w", err)
			}
		}
		if _, err := fmt.Fprintf(w, "# %s\n", sec.Name); err != nil {
			return fmt.Errorf("dump keybindings: %w", err)
		}
		for _, entry := range sec.Entries {
			keys := km.KeysFor(entry.Action)
			for _, k := range keys {
				if _, err := fmt.Fprintf(w, "map %s %s\n", km.dumpKeyName(k), entry.Action); err != nil {
					return fmt.Errorf("dump keybindings: %w", err)
				}
			}
		}
	}
	return nil
}

// reverseAliases maps canonical bubbletea key strings back to user-friendly names
// for keys that would not survive a round-trip through strings.Fields.
var reverseAliases = map[string]string{
	" ": "space",
}

// dumpKeyName converts a canonical key string to a user-friendly name for dump output.
// keys that are whitespace-only need special handling so the output can be reloaded.
// chord keys are split on ">" and each half is dumped independently so that an embedded
// literal space in either half is rewritten to its "space" alias, preserving the round-trip.
func (km *Keymap) dumpKeyName(key string) string {
	if leader, second, ok := strings.Cut(key, ">"); ok {
		return km.dumpKeyName(leader) + ">" + km.dumpKeyName(second)
	}
	if alias, ok := reverseAliases[key]; ok {
		return alias
	}
	return key
}

// mapEntry represents a parsed "map <key> <action>" line.
type mapEntry struct {
	key    string
	action Action
}

// keyAliases maps user-friendly key names to bubbletea's KeyMsg.String() output.
// keys that already match bubbletea's output are not listed here.
var keyAliases = map[string]string{
	"page_down":  "pgdown",
	"page_up":    "pgup",
	"pagedown":   "pgdown",
	"pageup":     "pgup",
	"escape":     "esc",
	"return":     "enter",
	"space":      " ",
	"ctrl+enter": "ctrl+m", // bubbletea maps enter to ctrl+m internally
}

// normalizeKey converts a user-provided key name to the canonical form
// used by bubbletea's KeyMsg.String(). Returns the normalized key.
func normalizeKey(key string) string {
	lower := strings.ToLower(key)
	if alias, ok := keyAliases[lower]; ok {
		return alias
	}
	// ctrl+ prefixed keys are always lowercase in bubbletea
	if strings.HasPrefix(lower, "ctrl+") {
		return lower
	}
	// alt+ prefixed keys: lowercase only the "alt+" prefix; preserve post-prefix
	// case because bubbletea distinguishes alt+t from alt+T (shift-modifier matters)
	if strings.HasPrefix(lower, "alt+") {
		return "alt+" + key[4:]
	}
	// preserve original case for single chars (j vs J matters)
	return key
}

// parseChordKey validates and normalizes a chord key of the form "<leader>><second>".
// Returns the normalized chord key and true on success, or "" and false after logging
// a warning for empty halves, three-stage chords, non-ctrl/alt leaders, or esc as the
// second-stage key (reserved for cancel). Leader case is normalized via normalizeKey
// (ctrl+/alt+ lowercased); second-stage case is preserved so that ctrl+w>x and ctrl+w>X
// remain distinct.
func parseChordKey(rawKey string, lineNum int) (string, bool) {
	parts := strings.SplitN(rawKey, ">", 2)
	leader, second := parts[0], parts[1]
	if leader == "" || second == "" {
		log.Printf("[WARN] keybindings:%d: chord halves cannot be empty, skipping", lineNum)
		return "", false
	}
	if strings.Contains(second, ">") {
		log.Printf("[WARN] keybindings:%d: only 2-stage chords supported, skipping", lineNum)
		return "", false
	}
	leaderNorm := normalizeKey(leader)
	if !strings.HasPrefix(leaderNorm, "ctrl+") && !strings.HasPrefix(leaderNorm, "alt+") {
		log.Printf("[WARN] keybindings:%d: chord leader must be ctrl+ or alt+ combo, skipping", lineNum)
		return "", false
	}
	secondNorm := normalizeKey(second)
	if secondNorm == "esc" {
		log.Printf("[WARN] keybindings:%d: esc cannot be a chord second-stage key (reserved for cancel), skipping", lineNum)
		return "", false
	}
	return leaderNorm + ">" + secondNorm, true
}

// parse reads keybinding definitions from r and returns map entries and unmap keys.
// format: "map <key> <action>" or "unmap <key>", with # comments and blank lines ignored.
// unknown action names are reported via log and skipped. Duplicate mappings: last wins.
func parse(r io.Reader) (maps []mapEntry, unmaps []string, err error) {
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			log.Printf("[WARN] keybindings:%d: invalid line %q, skipping", lineNum, line)
			continue
		}

		cmd := strings.ToLower(fields[0])
		switch cmd {
		case "map":
			if len(fields) < 3 {
				log.Printf("[WARN] keybindings:%d: map requires key and action, skipping", lineNum)
				continue
			}
			rawKey := fields[1]
			rawAction := Action(fields[2])
			action, deprecated, ok := resolveAction(rawAction)
			if !ok {
				log.Printf("[WARN] keybindings:%d: unknown action %q, skipping", lineNum, rawAction)
				continue
			}
			if deprecated {
				warnOnceDeprecatedAlias(rawAction, action)
			}
			if strings.Contains(rawKey, ">") && rawKey != ">" {
				key, ok := parseChordKey(rawKey, lineNum)
				if !ok {
					continue
				}
				maps = append(maps, mapEntry{key: key, action: action})
				continue
			}
			maps = append(maps, mapEntry{key: normalizeKey(rawKey), action: action})
		case "unmap":
			rawKey := fields[1]
			if strings.Contains(rawKey, ">") && rawKey != ">" {
				key, ok := parseChordKey(rawKey, lineNum)
				if !ok {
					continue
				}
				unmaps = append(unmaps, key)
				continue
			}
			unmaps = append(unmaps, normalizeKey(rawKey))
		default:
			log.Printf("[WARN] keybindings:%d: unknown command %q in line %q, skipping", lineNum, cmd, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading keybindings: %w", err)
	}
	return maps, unmaps, nil
}

// Load reads a keybindings file from path and returns a Keymap with defaults
// overridden by the file contents. Returns error if the file cannot be opened or parsed.
func Load(path string) (*Keymap, error) {
	f, err := os.Open(path) //nolint:gosec // path is user-provided config file location
	if err != nil {
		return nil, fmt.Errorf("opening keybindings file: %w", err)
	}
	defer f.Close()

	maps, unmaps, err := parse(f)
	if err != nil {
		return nil, err
	}

	km := Default()

	// apply unmaps first, then maps (so "unmap q" + "map x quit" works)
	for _, key := range unmaps {
		km.Unbind(key)
	}
	for _, m := range maps {
		km.Bind(m.key, m.action)
	}

	km.resolveConflicts()
	return km, nil
}

// resolveConflicts drops any standalone binding whose key is also a chord leader.
// When both "ctrl+w" and "ctrl+w>x" exist, the standalone is removed with a warning
// so that pressing the leader always enters chord-pending state instead of firing
// the standalone action. Invalidates the chord-prefix cache once at the end.
func (km *Keymap) resolveConflicts() {
	for chordKey := range km.bindings {
		idx := strings.Index(chordKey, ">")
		if idx <= 0 {
			continue
		}
		leader := chordKey[:idx]
		if _, exists := km.bindings[leader]; exists {
			log.Printf("[WARN] keybindings: %s bound as both standalone and chord prefix; standalone dropped", leader)
			delete(km.bindings, leader)
		}
	}
	km.chordPrefixCache = nil
}

// LoadOrDefault loads keybindings from path if the file exists, otherwise returns
// Default(). Parse errors are logged as warnings and Default() is returned.
func LoadOrDefault(path string) *Keymap {
	if path == "" {
		return Default()
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Default()
	}
	km, err := Load(path)
	if err != nil {
		log.Printf("[WARN] failed to load keybindings from %s: %v, using defaults", path, err)
		return Default()
	}
	return km
}
