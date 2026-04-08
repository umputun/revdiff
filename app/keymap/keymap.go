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
	ActionToggleCollapsed  Action = "toggle_collapsed"
	ActionToggleWrap       Action = "toggle_wrap"
	ActionToggleTree       Action = "toggle_tree"
	ActionToggleLineNums   Action = "toggle_line_numbers"
	ActionToggleBlame      Action = "toggle_blame"
	ActionToggleHunk       Action = "toggle_hunk"
	ActionToggleUntracked Action = "toggle_untracked"
	ActionMarkReviewed     Action = "mark_reviewed"
	ActionFilter           Action = "filter"
	ActionQuit             Action = "quit"
	ActionDiscardQuit      Action = "discard_quit"
	ActionHelp             Action = "help"
	ActionDismiss          Action = "dismiss"
)

// validActions contains all known action names for validation.
var validActions = map[Action]bool{
	ActionDown: true, ActionUp: true, ActionPageDown: true, ActionPageUp: true,
	ActionHalfPageDown: true, ActionHalfPageUp: true, ActionHome: true, ActionEnd: true,
	ActionScrollLeft: true, ActionScrollRight: true,
	ActionNextItem: true, ActionPrevItem: true, ActionNextHunk: true, ActionPrevHunk: true,
	ActionTogglePane: true, ActionFocusTree: true, ActionFocusDiff: true,
	ActionSearch:  true,
	ActionConfirm: true, ActionAnnotateFile: true, ActionDeleteAnnotation: true, ActionAnnotList: true,
	ActionToggleCollapsed: true, ActionToggleWrap: true, ActionToggleTree: true,
	ActionToggleLineNums: true, ActionToggleBlame: true, ActionToggleHunk: true, ActionMarkReviewed: true, ActionFilter: true, ActionToggleUntracked: true,
	ActionQuit: true, ActionDiscardQuit: true, ActionHelp: true, ActionDismiss: true,
}

// IsValidAction returns true if the action name is recognized.
func IsValidAction(a Action) bool {
	return validActions[a]
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
	bindings     map[string]Action
	descriptions []HelpEntry // ordered list of action descriptions
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

		// file/hunk
		{ActionNextItem, "next file / search match", "File/Hunk"},
		{ActionPrevItem, "prev file / search match", "File/Hunk"},
		{ActionNextHunk, "next hunk", "File/Hunk"},
		{ActionPrevHunk, "prev hunk", "File/Hunk"},

		// pane
		{ActionTogglePane, "toggle pane focus", "Pane"},
		{ActionFocusTree, "focus tree pane", "Pane"},
		{ActionFocusDiff, "focus diff pane", "Pane"},

		// search
		{ActionSearch, "search in diff", "Search"},

		// annotations
		{ActionConfirm, "annotate line / select file", "Annotations"},
		{ActionAnnotateFile, "annotate file", "Annotations"},
		{ActionDeleteAnnotation, "delete annotation", "Annotations"},
		{ActionAnnotList, "annotation list", "Annotations"},

		// view toggles
		{ActionToggleCollapsed, "toggle collapsed view", "View"},
		{ActionToggleWrap, "toggle word wrap", "View"},
		{ActionToggleTree, "toggle tree pane", "View"},
		{ActionToggleLineNums, "toggle line numbers", "View"},
		{ActionToggleBlame, "toggle blame gutter", "View"},
		{ActionToggleHunk, "toggle hunk in collapsed", "View"},
		{ActionToggleUntracked, "show/hide untracked files", "View"},
		{ActionMarkReviewed, "mark file as reviewed", "View"},
		{ActionFilter, "filter files", "View"},

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
		"v":      ActionToggleCollapsed,
		"w":      ActionToggleWrap,
		"t":      ActionToggleTree,
		"L":      ActionToggleLineNums,
		"B":      ActionToggleBlame,
		".":      ActionToggleHunk,
		" ":      ActionMarkReviewed,
		"u":      ActionToggleUntracked,
		"f":      ActionFilter,
		"q":      ActionQuit,
		"Q":      ActionDiscardQuit,
		"?":      ActionHelp,
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
}

// Unbind removes the binding for the given key. No-op if key is not bound.
func (km *Keymap) Unbind(key string) {
	delete(km.bindings, key)
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
				if _, err := fmt.Fprintf(w, "map %s %s\n", dumpKeyName(k), entry.Action); err != nil {
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
func dumpKeyName(key string) string {
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
	// preserve original case for single chars (j vs J matters)
	return key
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
			key := normalizeKey(fields[1])
			action := Action(fields[2])
			if !IsValidAction(action) {
				log.Printf("[WARN] keybindings:%d: unknown action %q, skipping", lineNum, action)
				continue
			}
			maps = append(maps, mapEntry{key: key, action: action})
		case "unmap":
			key := normalizeKey(fields[1])
			unmaps = append(unmaps, key)
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

	return km, nil
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
