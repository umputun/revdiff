// Package keymap provides user-configurable key bindings for revdiff.
// It maps key names (as returned by bubbletea's KeyMsg.String()) to action names.
package keymap

import (
	"sort"
	"strings"
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
	ActionToggleHunk       Action = "toggle_hunk"
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
	ActionToggleLineNums: true, ActionToggleHunk: true, ActionFilter: true,
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
		{ActionToggleWrap, "toggle line wrap", "View"},
		{ActionToggleTree, "toggle tree pane", "View"},
		{ActionToggleLineNums, "toggle line numbers", "View"},
		{ActionToggleHunk, "toggle hunk in collapsed", "View"},
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
		".":      ActionToggleHunk,
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

// Resolve returns the action bound to the given key, or empty string if unbound.
func (km *Keymap) Resolve(key string) Action {
	return km.bindings[key]
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
