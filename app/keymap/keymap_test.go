package keymap

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	km := Default()
	require.NotNil(t, km)
	assert.NotEmpty(t, km.bindings)
	assert.NotEmpty(t, km.descriptions)
}

func TestDefault_allExpectedBindings(t *testing.T) {
	km := Default()
	tests := []struct {
		key    string
		action Action
	}{
		{"j", ActionDown}, {"k", ActionUp}, {"down", ActionDown}, {"up", ActionUp},
		{"pgdown", ActionPageDown}, {"pgup", ActionPageUp},
		{"ctrl+d", ActionHalfPageDown}, {"ctrl+u", ActionHalfPageUp},
		{"home", ActionHome}, {"end", ActionEnd},
		{"left", ActionScrollLeft}, {"right", ActionScrollRight},
		{"n", ActionNextItem}, {"N", ActionPrevItem}, {"p", ActionPrevItem},
		{"]", ActionNextHunk}, {"[", ActionPrevHunk},
		{"tab", ActionTogglePane}, {"h", ActionFocusTree}, {"l", ActionFocusDiff},
		{"/", ActionSearch},
		{"a", ActionConfirm}, {"enter", ActionConfirm},
		{"A", ActionAnnotateFile}, {"d", ActionDeleteAnnotation}, {"@", ActionAnnotList},
		{"}", ActionNextAnnotation}, {"{", ActionPrevAnnotation},
		{"v", ActionToggleCollapsed}, {"C", ActionToggleCompact}, {"w", ActionToggleWrap}, {"t", ActionToggleTree},
		{"L", ActionToggleLineNums}, {"B", ActionToggleBlame}, {"W", ActionToggleWordDiff},
		{".", ActionToggleHunk}, {" ", ActionMarkReviewed}, {"f", ActionFilter},
		{"u", ActionToggleUntracked},
		{"q", ActionQuit}, {"Q", ActionDiscardQuit}, {"?", ActionHelp}, {"T", ActionThemeSelect}, {"esc", ActionDismiss},
		{"i", ActionInfo},
		{"R", ActionReload},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.action, km.Resolve(tt.key), "key %q should map to %q", tt.key, tt.action)
	}
	// verify binding count matches expected
	assert.Len(t, km.bindings, len(tests), "default keymap should have exactly %d bindings", len(tests))
}

func TestDefault_specialKeysMatchBubbletea(t *testing.T) {
	// verify that our key names match what bubbletea's KeyMsg.String() actually returns
	tests := []struct {
		keyType tea.KeyType
		want    string
	}{
		{tea.KeyPgDown, "pgdown"},
		{tea.KeyPgUp, "pgup"},
		{tea.KeyHome, "home"},
		{tea.KeyEnd, "end"},
		{tea.KeyUp, "up"},
		{tea.KeyDown, "down"},
		{tea.KeyLeft, "left"},
		{tea.KeyRight, "right"},
		{tea.KeyEnter, "enter"},
		{tea.KeyEsc, "esc"},
		{tea.KeyTab, "tab"},
	}

	km := Default()
	for _, tt := range tests {
		msg := tea.KeyMsg{Type: tt.keyType}
		actual := msg.String()
		assert.Equal(t, tt.want, actual, "bubbletea KeyType %d String() should be %q", tt.keyType, tt.want)
		// and verify the default keymap has a binding for it
		action := km.Resolve(actual)
		assert.NotEmpty(t, action, "default keymap should have binding for %q", actual)
	}
}

func TestDefault_ctrlKeysMatchBubbletea(t *testing.T) {
	// ctrl+d and ctrl+u: bubbletea represents these as KeyMsg with specific types
	ctrlD := tea.KeyMsg{Type: tea.KeyCtrlD}
	ctrlU := tea.KeyMsg{Type: tea.KeyCtrlU}

	km := Default()
	assert.Equal(t, ActionHalfPageDown, km.Resolve(ctrlD.String()))
	assert.Equal(t, ActionHalfPageUp, km.Resolve(ctrlU.String()))
}

func TestResolve(t *testing.T) {
	km := Default()

	t.Run("existing key", func(t *testing.T) {
		assert.Equal(t, ActionDown, km.Resolve("j"))
	})

	t.Run("missing key", func(t *testing.T) {
		assert.Equal(t, Action(""), km.Resolve("z"))
	})

	t.Run("after override", func(t *testing.T) {
		km.Bind("z", ActionQuit)
		assert.Equal(t, ActionQuit, km.Resolve("z"))
	})
}

func TestKeysFor(t *testing.T) {
	km := Default()

	t.Run("single key action", func(t *testing.T) {
		keys := km.KeysFor(ActionSearch)
		assert.Equal(t, []string{"/"}, keys)
	})

	t.Run("multiple keys action", func(t *testing.T) {
		keys := km.KeysFor(ActionDown)
		assert.Contains(t, keys, "j")
		assert.Contains(t, keys, "down")
		assert.Len(t, keys, 2)
	})

	t.Run("two keys action", func(t *testing.T) {
		keys := km.KeysFor(ActionPrevItem)
		assert.Contains(t, keys, "N")
		assert.Contains(t, keys, "p")
		assert.Len(t, keys, 2)
	})

	t.Run("unknown action", func(t *testing.T) {
		keys := km.KeysFor(Action("nonexistent"))
		assert.Empty(t, keys)
	})
}

func TestBind(t *testing.T) {
	km := Default()
	km.Bind("x", ActionQuit)
	assert.Equal(t, ActionQuit, km.Resolve("x"))
	// original binding still works
	assert.Equal(t, ActionQuit, km.Resolve("q"))
}

func TestUnbind(t *testing.T) {
	km := Default()
	km.Unbind("q")
	assert.Equal(t, Action(""), km.Resolve("q"))
	// other bindings unaffected
	assert.Equal(t, ActionDiscardQuit, km.Resolve("Q"), "Q should still map to discard_quit")
}

func TestUnbind_noop(t *testing.T) {
	km := Default()
	km.Unbind("nonexistent") // should not panic
	assert.Equal(t, ActionDown, km.Resolve("j"))
}

func TestHelpSections(t *testing.T) {
	km := Default()
	sections := km.HelpSections()

	require.NotEmpty(t, sections)

	// verify section names
	names := make([]string, 0, len(sections))
	for _, s := range sections {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "Navigation")
	assert.Contains(t, names, "File/Hunk")
	assert.Contains(t, names, "Pane")
	assert.Contains(t, names, "Search")
	assert.Contains(t, names, "Annotations")
	assert.Contains(t, names, "View")
	assert.Contains(t, names, "Quit")

	// verify entries have keys
	for _, s := range sections {
		for _, e := range s.Entries {
			assert.NotEmpty(t, e.Keys, "action %q in section %q should have keys", e.Action, s.Name)
			assert.NotEmpty(t, e.Description, "action %q should have description", e.Action)
		}
	}
}

func TestHelpSections_unmappedActionOmitted(t *testing.T) {
	km := Default()
	// unbind all keys for quit
	km.Unbind("q")
	sections := km.HelpSections()

	for _, s := range sections {
		for _, e := range s.Entries {
			if e.Action == ActionQuit {
				t.Error("unmapped action 'quit' should not appear in help sections")
			}
		}
	}
}

func TestHelpSections_customBindingReflected(t *testing.T) {
	km := Default()
	km.Bind("x", ActionQuit)
	sections := km.HelpSections()

	for _, s := range sections {
		for _, e := range s.Entries {
			if e.Action == ActionQuit {
				assert.Contains(t, e.Keys, "x")
				assert.Contains(t, e.Keys, "q")
				return
			}
		}
	}
	t.Error("quit action not found in help sections")
}

func TestActionReload_IsValid(t *testing.T) {
	assert.True(t, IsValidAction(ActionReload))
}

func TestActionToggleCompact_IsValid(t *testing.T) {
	assert.True(t, IsValidAction(ActionToggleCompact))
}

func TestActionToggleCompact_DefaultBinding(t *testing.T) {
	km := Default()
	assert.Equal(t, ActionToggleCompact, km.Resolve("C"))
}

func TestActionToggleCompact_HelpEntry(t *testing.T) {
	entries := defaultDescriptions()
	var found bool
	for _, e := range entries {
		if e.Action == ActionToggleCompact {
			assert.Equal(t, "toggle compact diff view", e.Description)
			assert.Equal(t, "View", e.Section)
			found = true
			break
		}
	}
	assert.True(t, found, "ActionToggleCompact should have a help entry")
}

func TestActionScrollConstants_InNavigationActions(t *testing.T) {
	// the three scroll-align actions must be recognized as valid so keybindings
	// files can map custom keys to them (e.g. "map z scroll_center").
	assert.True(t, IsValidAction(ActionScrollCenter))
	assert.True(t, IsValidAction(ActionScrollTop))
	assert.True(t, IsValidAction(ActionScrollBottom))
}

func TestActionScrollConstants_InHelpEntries(t *testing.T) {
	entries := defaultDescriptions()
	wants := map[Action]string{
		ActionScrollCenter: "center viewport on cursor",
		ActionScrollTop:    "align viewport top",
		ActionScrollBottom: "align viewport bottom",
	}
	found := make(map[Action]bool, len(wants))
	for _, e := range entries {
		if desc, ok := wants[e.Action]; ok {
			assert.Equal(t, desc, e.Description, "description mismatch for %q", e.Action)
			assert.Equal(t, "Navigation", e.Section, "section mismatch for %q", e.Action)
			found[e.Action] = true
		}
	}
	for a := range wants {
		assert.True(t, found[a], "action %q should have a help entry", a)
	}
}

func TestActionScrollConstants_NoDefaultBindings(t *testing.T) {
	// vim-motion interceptor is the only way to reach these actions by default;
	// there must be NO single-key bindings in defaultBindings.
	km := Default()
	for _, a := range []Action{ActionScrollCenter, ActionScrollTop, ActionScrollBottom} {
		assert.Empty(t, km.KeysFor(a), "action %q must have no default bindings", a)
	}
}

func TestIsValidAction(t *testing.T) {
	assert.True(t, IsValidAction(ActionQuit))
	assert.True(t, IsValidAction(ActionDown))
	assert.True(t, IsValidAction(ActionInfo))
	assert.True(t, IsValidAction(Action("commit_info")), "deprecated alias must validate")
	assert.False(t, IsValidAction(Action("nonexistent")))
	assert.False(t, IsValidAction(Action("")))
}

func TestResolveAction_deprecatedCommitInfoAlias(t *testing.T) {
	// pre-v0.27 keybinding files use "commit_info"; the action was renamed
	// to "info" when the popup expanded. Existing user configs must keep
	// working — the parser rewrites the alias to the canonical name.
	canonical, deprecated, ok := resolveAction(Action("commit_info"))
	require.True(t, ok)
	assert.True(t, deprecated)
	assert.Equal(t, ActionInfo, canonical)

	// canonical name resolves to itself with no deprecation flag
	canonical, deprecated, ok = resolveAction(ActionInfo)
	require.True(t, ok)
	assert.False(t, deprecated)
	assert.Equal(t, ActionInfo, canonical)

	// unknown action stays unknown
	_, _, ok = resolveAction(Action("totally_made_up"))
	assert.False(t, ok)
}

func TestParse_acceptsDeprecatedCommitInfoAlias(t *testing.T) {
	maps, _, err := parse(strings.NewReader("map i commit_info\n"))
	require.NoError(t, err)
	require.Len(t, maps, 1)
	assert.Equal(t, "i", maps[0].key)
	assert.Equal(t, ActionInfo, maps[0].action, "alias must rewrite to canonical action")
}

func TestInfo_roundTrip(t *testing.T) {
	// default binding resolves correctly
	km := Default()
	assert.Equal(t, ActionInfo, km.Resolve("i"))

	// action appears in help sections
	sections := km.HelpSections()
	found := false
	for _, s := range sections {
		for _, e := range s.Entries {
			if e.Action == ActionInfo {
				assert.NotEmpty(t, e.Description, "info action should have description")
				assert.Contains(t, e.Keys, "i")
				found = true
			}
		}
	}
	assert.True(t, found, "info action should appear in help sections")

	// dump → parse round-trips the binding
	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	assert.Contains(t, buf.String(), "map i info")

	maps, _, err := parse(strings.NewReader(buf.String()))
	require.NoError(t, err)
	var matched bool
	for _, m := range maps {
		if m.key == "i" && m.action == ActionInfo {
			matched = true
		}
	}
	assert.True(t, matched, "parsed dump should contain i → info")
}

func TestKeysFor_sorted(t *testing.T) {
	km := Default()
	keys := km.KeysFor(ActionDown)
	// should be sorted: "down" before "j"
	assert.Equal(t, []string{"down", "j"}, keys)
}

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"j", "j"}, {"J", "J"}, // preserve case for single chars
		{"pgdown", "pgdown"}, {"pgup", "pgup"}, // already canonical
		{"page_down", "pgdown"}, {"page_up", "pgup"}, // alias
		{"pagedown", "pgdown"}, {"pageup", "pgup"}, // alias
		{"escape", "esc"}, {"return", "enter"}, // alias
		{"space", " "},                             // alias
		{"ctrl+d", "ctrl+d"}, {"Ctrl+D", "ctrl+d"}, // ctrl always lowercase
		{"alt+t", "alt+t"}, {"Alt+T", "alt+T"}, {"ALT+t", "alt+t"}, // alt+: prefix lowered, suffix case preserved
		{"esc", "esc"}, {"enter", "enter"}, {"tab", "tab"}, // pass-through
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeKey(tt.input))
		})
	}
}

func TestParse_validMapLines(t *testing.T) {
	input := strings.NewReader("map x quit\nmap ctrl+d half_page_down\n")
	maps, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Empty(t, unmaps)
	require.Len(t, maps, 2)
	assert.Equal(t, "x", maps[0].key)
	assert.Equal(t, ActionQuit, maps[0].action)
	assert.Equal(t, "ctrl+d", maps[1].key)
	assert.Equal(t, ActionHalfPageDown, maps[1].action)
}

func TestParse_unmapLines(t *testing.T) {
	input := strings.NewReader("unmap q\nunmap j\n")
	maps, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Empty(t, maps)
	assert.Equal(t, []string{"q", "j"}, unmaps)
}

func TestParse_commentsAndBlanks(t *testing.T) {
	input := strings.NewReader("# comment\n\n  # indented comment\n  \nmap x quit\n")
	maps, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Empty(t, unmaps)
	require.Len(t, maps, 1)
	assert.Equal(t, "x", maps[0].key)
}

func TestParse_unknownAction(t *testing.T) {
	input := strings.NewReader("map x fly_away\nmap y quit\n")
	maps, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Empty(t, unmaps)
	// unknown action skipped, valid one kept
	require.Len(t, maps, 1)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_invalidLines(t *testing.T) {
	input := strings.NewReader("map\nfoo bar baz\nmap x quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	// only valid line parsed
	require.Len(t, maps, 1)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_duplicateMapLastWins(t *testing.T) {
	input := strings.NewReader("map x quit\nmap x help\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	// both entries returned; Load applies them in order (last wins)
	require.Len(t, maps, 2)
	assert.Equal(t, ActionQuit, maps[0].action)
	assert.Equal(t, ActionHelp, maps[1].action)
}

func TestParse_keyNormalization(t *testing.T) {
	input := strings.NewReader("map page_down page_down\nmap Ctrl+D half_page_down\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 2)
	assert.Equal(t, "pgdown", maps[0].key) // normalized from page_down
	assert.Equal(t, "ctrl+d", maps[1].key) // normalized ctrl case
}

func TestParse_unmapNormalization(t *testing.T) {
	input := strings.NewReader("unmap page_down\n")
	_, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"pgdown"}, unmaps)
}

func TestParse_ChordBinding(t *testing.T) {
	input := strings.NewReader("map ctrl+w>x quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 1)
	assert.Equal(t, "ctrl+w>x", maps[0].key)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_ChordBinding_NormalizesCase(t *testing.T) {
	// leader is case-normalized via normalizeKey (ctrl+/alt+ lowercased)
	// second-stage case is preserved (X stays X)
	input := strings.NewReader("map Ctrl+W>X quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 1)
	assert.Equal(t, "ctrl+w>X", maps[0].key)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_ChordBinding_AltLeader(t *testing.T) {
	input := strings.NewReader("map alt+t>n quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 1)
	assert.Equal(t, "alt+t>n", maps[0].key)
}

func TestParse_ChordBinding_AltLeaderCapitalizedPrefix(t *testing.T) {
	// user-typed "Alt+T" must normalize the "Alt+" prefix to "alt+" (matches bubbletea),
	// while preserving the "T" case because alt+T and alt+t are distinct in bubbletea.
	input := strings.NewReader("map Alt+T>n theme_select\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 1)
	assert.Equal(t, "alt+T>n", maps[0].key)
	assert.Equal(t, ActionThemeSelect, maps[0].action)
}

func TestParse_ChordBinding_RejectsNonModifierLeader(t *testing.T) {
	input := strings.NewReader("map g>g home\nmap ctrl+w>x quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	// only the valid chord should remain; g>g rejected
	require.Len(t, maps, 1)
	assert.Equal(t, "ctrl+w>x", maps[0].key)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_ChordBinding_RejectsShiftLeader(t *testing.T) {
	// shift+ is neither ctrl+ nor alt+; rejected
	input := strings.NewReader("map shift+w>x quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	assert.Empty(t, maps)
}

func TestParse_ChordBinding_RejectsThreeStage(t *testing.T) {
	input := strings.NewReader("map ctrl+w>x>y quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	assert.Empty(t, maps)
}

func TestParse_ChordBinding_RejectsEmptyHalves(t *testing.T) {
	cases := []string{
		"map ctrl+w> quit\n",
		"map >x quit\n",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			maps, _, err := parse(strings.NewReader(c))
			require.NoError(t, err)
			assert.Empty(t, maps, "input %q should parse to no map entries", c)
		})
	}
}

func TestParse_GreaterThanStandalone(t *testing.T) {
	// the bare ">" key is a valid standalone binding and must not be confused
	// with chord syntax; only rawKey values containing ">" AND longer than a
	// single character trigger chord parsing
	input := strings.NewReader("map > quit\nunmap >\n")
	maps, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Equal(t, []mapEntry{{key: ">", action: ActionQuit}}, maps)
	assert.Equal(t, []string{">"}, unmaps)
}

func TestParse_ChordBinding_RejectsEscSecondStage(t *testing.T) {
	// esc is reserved for chord cancel in handleChordSecond; a chord with esc
	// as the second stage would parse but never fire, so reject at parse time
	// to make the silent failure loud. cover both the canonical "esc" and the
	// "escape" alias (normalizeKey folds both to "esc")
	cases := []string{
		"map ctrl+w>esc quit\n",
		"map ctrl+w>escape quit\n",
		"map alt+t>esc quit\n",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			maps, _, err := parse(strings.NewReader(c))
			require.NoError(t, err)
			assert.Empty(t, maps, "input %q must be rejected", c)
		})
	}
}

func TestParse_ChordBinding_UnmapChord(t *testing.T) {
	input := strings.NewReader("unmap ctrl+w>x\n")
	_, unmaps, err := parse(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"ctrl+w>x"}, unmaps)
}

func TestParse_ChordBinding_UnmapInvalidChordSkipped(t *testing.T) {
	input := strings.NewReader("unmap g>g\nunmap ctrl+w>x\n")
	_, unmaps, err := parse(input)
	require.NoError(t, err)
	// invalid chord leader rejected; only the valid chord unmap remains
	assert.Equal(t, []string{"ctrl+w>x"}, unmaps)
}

func TestLoad_withOverrides(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	err := os.WriteFile(tmpFile, []byte("map x quit\nunmap j\n"), 0o600)
	require.NoError(t, err)

	km, err := Load(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, ActionQuit, km.Resolve("x"))    // new binding
	assert.Equal(t, ActionQuit, km.Resolve("q"))    // default still works
	assert.Equal(t, Action(""), km.Resolve("j"))    // unmapped
	assert.Equal(t, ActionDown, km.Resolve("down")) // other default still works
}

func TestLoad_unmapThenRemap(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	err := os.WriteFile(tmpFile, []byte("unmap q\nmap x quit\n"), 0o600)
	require.NoError(t, err)

	km, err := Load(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, Action(""), km.Resolve("q")) // unmapped
	assert.Equal(t, ActionQuit, km.Resolve("x")) // remapped
}

func TestLoad_missingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/keybindings")
	assert.Error(t, err)
}

func TestLoad_malformedLines(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	err := os.WriteFile(tmpFile, []byte("garbage line\nmap x quit\n"), 0o600)
	require.NoError(t, err)

	km, err := Load(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, ActionQuit, km.Resolve("x")) // valid line still applied
}

func TestLoadOrDefault_noFile(t *testing.T) {
	km := LoadOrDefault("/nonexistent/path/keybindings")
	// should return defaults
	assert.Equal(t, ActionDown, km.Resolve("j"))
	assert.Equal(t, ActionQuit, km.Resolve("q"))
}

func TestLoadOrDefault_emptyPath(t *testing.T) {
	km := LoadOrDefault("")
	assert.Equal(t, ActionDown, km.Resolve("j"))
}

func TestLoadOrDefault_withFile(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	err := os.WriteFile(tmpFile, []byte("map x quit\n"), 0o600)
	require.NoError(t, err)

	km := LoadOrDefault(tmpFile)
	assert.Equal(t, ActionQuit, km.Resolve("x"))
	assert.Equal(t, ActionDown, km.Resolve("j")) // defaults still present
}

func TestLoad_unmapOfUnboundKey(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	err := os.WriteFile(tmpFile, []byte("unmap z\n"), 0o600)
	require.NoError(t, err)

	km, err := Load(tmpFile)
	require.NoError(t, err)
	// should not panic, defaults should be intact
	assert.Equal(t, ActionDown, km.Resolve("j"))
}

func TestDump_format(t *testing.T) {
	km := Default()
	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()

	// should contain section headers
	assert.Contains(t, output, "# Navigation")
	assert.Contains(t, output, "# File/Hunk")
	assert.Contains(t, output, "# Pane")
	assert.Contains(t, output, "# Search")
	assert.Contains(t, output, "# Annotations")
	assert.Contains(t, output, "# View")
	assert.Contains(t, output, "# Quit")

	// should contain map lines for known bindings
	assert.Contains(t, output, "map j down")
	assert.Contains(t, output, "map q quit")
	assert.Contains(t, output, "map / search")

	// should not contain unmap lines (dump only writes effective bindings)
	assert.NotContains(t, output, "unmap")
}

func TestDump_roundTrip(t *testing.T) {
	km := Default()
	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))

	// parse the dumped output
	maps, unmaps, err := parse(strings.NewReader(buf.String()))
	require.NoError(t, err)
	assert.Empty(t, unmaps)

	// rebuild a keymap from parsed output (start empty, apply all maps)
	rebuilt := &Keymap{
		bindings:     make(map[string]Action),
		descriptions: defaultDescriptions(),
	}
	for _, m := range maps {
		rebuilt.Bind(m.key, m.action)
	}

	// verify all original bindings are present in rebuilt
	for key, action := range km.bindings {
		assert.Equal(t, action, rebuilt.Resolve(key), "round-trip: key %q should map to %q", key, action)
	}
	// verify no extra bindings
	assert.Len(t, rebuilt.bindings, len(km.bindings))
}

func TestDump_customBindings(t *testing.T) {
	km := Default()
	km.Unbind("q")
	km.Bind("x", ActionQuit)

	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()

	// x should appear as quit binding, q should not
	assert.Contains(t, output, "map x quit")
	assert.NotContains(t, output, "map q quit")
}

func TestDump_chordWithSpaceSecondStageRoundTrip(t *testing.T) {
	// chord binding with "space" as the second stage must round-trip: the literal
	// space stored as "ctrl+w> " needs to dump as "ctrl+w>space" so that a later
	// reload re-parses into the same chord key.
	km := &Keymap{bindings: make(map[string]Action), descriptions: defaultDescriptions()}
	km.Bind("ctrl+w> ", ActionMarkReviewed)

	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()

	assert.Contains(t, output, "map ctrl+w>space mark_reviewed", "chord second-stage space must dump as 'space' alias")
	assert.NotContains(t, output, "map ctrl+w>  mark_reviewed", "no double-space output (would break reload)")

	// round-trip: parse the dump and verify the chord binding survives
	maps, _, err := parse(strings.NewReader(output))
	require.NoError(t, err)

	rebuilt := &Keymap{bindings: make(map[string]Action), descriptions: defaultDescriptions()}
	for _, m := range maps {
		rebuilt.Bind(m.key, m.action)
	}
	assert.Equal(t, ActionMarkReviewed, rebuilt.ResolveChord("ctrl+w", " "), "chord with space must survive dump -> parse round-trip")
}

func TestDump_spaceKeyRoundTrip(t *testing.T) {
	km := Default()
	km.Bind(" ", ActionPageDown) // bind space to an action

	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()

	// space key should be written as "space" alias, not literal " "
	assert.Contains(t, output, "map space page_down")
	assert.NotContains(t, output, "map  page_down") // no bare space

	// round-trip: parse the dump and verify space binding survives
	maps, _, err := parse(strings.NewReader(output))
	require.NoError(t, err)

	rebuilt := &Keymap{bindings: make(map[string]Action), descriptions: defaultDescriptions()}
	for _, m := range maps {
		rebuilt.Bind(m.key, m.action)
	}
	assert.Equal(t, ActionPageDown, rebuilt.Resolve(" "), "space binding should survive round-trip")
}

func TestDump_unmappedActionOmitted(t *testing.T) {
	km := Default()
	km.Unbind("/") // search only has one key

	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()

	// search action should not appear at all
	assert.NotContains(t, output, "search")
}

func TestDump_failingWriter(t *testing.T) {
	km := Default()
	w := &failWriter{errAfter: 0}
	err := km.Dump(w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write error")
}

func TestDump_failingWriterAfterSomeOutput(t *testing.T) {
	km := Default()
	w := &failWriter{errAfter: 5} // fail after 5 successful writes
	err := km.Dump(w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write error")
}

// failWriter is an io.Writer that returns an error after errAfter successful writes.
type failWriter struct {
	errAfter int
	count    int
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.count >= w.errAfter {
		return 0, errors.New("write error")
	}
	w.count++
	return len(p), nil
}

// acceptance tests verifying end-to-end keybinding scenarios

func TestAcceptance_defaultKeymapPreservesAllBindings(t *testing.T) {
	// no keybindings file → identical behavior to current defaults
	km := Default()
	assert.Equal(t, ActionDown, km.Resolve("j"))
	assert.Equal(t, ActionUp, km.Resolve("k"))
	assert.Equal(t, ActionQuit, km.Resolve("q"))
	assert.Equal(t, ActionHelp, km.Resolve("?"))
	assert.Equal(t, ActionSearch, km.Resolve("/"))
	assert.Equal(t, ActionToggleCollapsed, km.Resolve("v"))
	assert.Equal(t, ActionConfirm, km.Resolve("enter"))
	assert.Equal(t, ActionNextHunk, km.Resolve("]"))
	assert.Equal(t, ActionPrevHunk, km.Resolve("["))
}

func TestAcceptance_additiveBinding(t *testing.T) {
	// map x quit → x quits, q still quits (additive, not replacement)
	km := Default()
	km.Bind("x", ActionQuit)
	assert.Equal(t, ActionQuit, km.Resolve("x"), "x should quit after binding")
	assert.Equal(t, ActionQuit, km.Resolve("q"), "q should still quit (additive)")
}

func TestAcceptance_unmapThenRemap(t *testing.T) {
	// unmap q + map x quit → only x quits
	km := Default()
	km.Unbind("q")
	km.Bind("x", ActionQuit)
	assert.Equal(t, ActionQuit, km.Resolve("x"), "x should quit")
	assert.Equal(t, Action(""), km.Resolve("q"), "q should not quit after unmap")
}

func TestAcceptance_dumpKeysShowsEffective(t *testing.T) {
	// --dump-keys prints all effective bindings in parseable format
	km := Default()
	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()
	assert.Contains(t, output, "map j down")
	assert.Contains(t, output, "map q quit")
	assert.Contains(t, output, "map ? help")
}

func TestAcceptance_loadCustomFile(t *testing.T) {
	// --keys /path/to/file loads custom bindings
	tmp, err := os.CreateTemp("", "keybindings-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.WriteString("map x quit\nunmap q\n")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	km, err := Load(tmp.Name())
	require.NoError(t, err)
	assert.Equal(t, ActionQuit, km.Resolve("x"))
	assert.Equal(t, Action(""), km.Resolve("q"))
}

func TestAcceptance_helpReflectsCustomBindings(t *testing.T) {
	// help overlay reflects custom bindings
	km := Default()
	km.Bind("x", ActionQuit)
	sections := km.HelpSections()

	found := false
	for _, sec := range sections {
		for _, entry := range sec.Entries {
			if entry.Action == ActionQuit {
				keys := km.KeysFor(ActionQuit)
				assert.Contains(t, keys, "q")
				assert.Contains(t, keys, "x")
				found = true
			}
		}
	}
	assert.True(t, found, "quit action should appear in help sections")
}

func TestAcceptance_invalidActionWarnsNoCrash(t *testing.T) {
	// invalid action names produce no error, just skip
	input := strings.NewReader("map x fly_away\nmap y unknown_action\nmap z quit\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 1, "only valid action should be parsed")
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestIsChordLeader(t *testing.T) {
	km := Default()
	km.Bind("ctrl+w>x", ActionQuit)

	assert.True(t, km.IsChordLeader("ctrl+w"), "ctrl+w should be a chord leader")
	assert.False(t, km.IsChordLeader("ctrl+d"), "ctrl+d has no chord binding")
	assert.False(t, km.IsChordLeader("j"), "plain keys should never be chord leaders")
	assert.False(t, km.IsChordLeader("ctrl+w>x"), "the full chord key is not itself a leader")
}

func TestIsChordLeader_standaloneIsNotLeader(t *testing.T) {
	// standalone ctrl+w without any ctrl+w>* chord → not a leader
	km := Default()
	km.Bind("ctrl+w", ActionQuit)
	assert.False(t, km.IsChordLeader("ctrl+w"), "standalone-only binding should not be a chord leader")
}

func TestIsChordLeader_LazyAndInvalidated(t *testing.T) {
	km := Default()
	// no chord bindings yet
	assert.False(t, km.IsChordLeader("ctrl+w"))

	// binding a chord must invalidate the cache so the next call sees it
	km.Bind("ctrl+w>x", ActionQuit)
	assert.True(t, km.IsChordLeader("ctrl+w"), "Bind must invalidate chord-prefix cache")

	// unbinding the only chord under a leader must invalidate the cache
	km.Unbind("ctrl+w>x")
	assert.False(t, km.IsChordLeader("ctrl+w"), "Unbind must invalidate chord-prefix cache")

	// multiple chords under same leader: removing one keeps leader active
	km.Bind("ctrl+w>x", ActionQuit)
	km.Bind("ctrl+w>y", ActionHelp)
	km.Unbind("ctrl+w>x")
	assert.True(t, km.IsChordLeader("ctrl+w"), "leader still active while any chord under it remains")
}

func TestLoad_ConflictDropsStandalone(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	content := "map ctrl+w quit\nmap ctrl+w>x help\n"
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	km, err := Load(tmpFile)
	require.NoError(t, err)

	// the chord binding survives
	assert.Equal(t, ActionHelp, km.Resolve("ctrl+w>x"))
	// the standalone is dropped so the leader can enter chord-pending state
	assert.Equal(t, Action(""), km.Resolve("ctrl+w"))
	// the leader is recognized as a chord leader
	assert.True(t, km.IsChordLeader("ctrl+w"))
}

func TestLoad_NoConflictKeepsBoth(t *testing.T) {
	tmpFile := t.TempDir() + "/keybindings"
	content := "map ctrl+w>x help\nmap ctrl+t quit\n"
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	km, err := Load(tmpFile)
	require.NoError(t, err)

	// chord survives
	assert.Equal(t, ActionHelp, km.Resolve("ctrl+w>x"))
	// unrelated standalone binding untouched
	assert.Equal(t, ActionQuit, km.Resolve("ctrl+t"))
	// leader of the chord has no standalone action
	assert.Equal(t, Action(""), km.Resolve("ctrl+w"))
	// defaults that are neither leaders nor chord bindings remain
	assert.Equal(t, ActionDown, km.Resolve("j"))
}

func TestLoad_ConflictInvalidatesChordCache(t *testing.T) {
	// prime a state where the default ctrl+d binding would conflict with a chord,
	// then verify the conflict-resolution pass invalidates the cache correctly.
	tmpFile := t.TempDir() + "/keybindings"
	content := "map ctrl+d>x help\n"
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	km, err := Load(tmpFile)
	require.NoError(t, err)

	// the default ctrl+d standalone binding was dropped by resolveConflicts
	assert.Equal(t, Action(""), km.Resolve("ctrl+d"))
	// chord leader is recognized immediately after Load
	assert.True(t, km.IsChordLeader("ctrl+d"))
	// the chord itself resolves
	assert.Equal(t, ActionHelp, km.Resolve("ctrl+d>x"))
}

func TestNormalizeKey_LatinPassThrough(t *testing.T) {
	// ASCII keys must be returned unchanged; no spurious translation.
	assert.Equal(t, "j", NormalizeKey("j"))
	assert.Equal(t, "G", NormalizeKey("G"))
	assert.Equal(t, "5", NormalizeKey("5"))
}

func TestNormalizeKey_MultiRunePassThrough(t *testing.T) {
	// multi-character key strings (special keys, modifier combos) bypass
	// the layout alias entirely.
	assert.Equal(t, "esc", NormalizeKey("esc"))
	assert.Equal(t, "ctrl+w", NormalizeKey("ctrl+w"))
	assert.Equal(t, "tab", NormalizeKey("tab"))
}

func TestNormalizeKey_CyrillicToLatin(t *testing.T) {
	// single-rune non-Latin keys translate to their Latin QWERTY equivalent.
	tests := []struct{ in, want string }{
		{"о", "j"}, {"л", "k"}, {"п", "g"}, {"я", "z"},
		{"Я", "Z"}, {"О", "J"}, {"ь", "m"}, {"м", "v"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, NormalizeKey(tc.in), "NormalizeKey(%q)", tc.in)
	}
}

func TestNormalizeKey_UnmappedRunePassThrough(t *testing.T) {
	// a non-Latin rune with no layout mapping returns unchanged.
	assert.Equal(t, "日", NormalizeKey("日"))
}

func TestResolveChord_Direct(t *testing.T) {
	km := Default()
	km.Bind("ctrl+w>x", ActionQuit)
	assert.Equal(t, ActionQuit, km.ResolveChord("ctrl+w", "x"))
}

func TestResolveChord_LayoutFallback(t *testing.T) {
	// ч (Cyrillic che) sits on the same physical key as x on QWERTY.
	// chord bound under the latin "x" must still resolve when user presses ч.
	km := Default()
	km.Bind("ctrl+w>x", ActionHelp)
	assert.Equal(t, ActionHelp, km.ResolveChord("ctrl+w", "ч"))
}

func TestResolveChord_Unbound(t *testing.T) {
	km := Default()
	km.Bind("ctrl+w>x", ActionQuit)
	assert.Equal(t, Action(""), km.ResolveChord("ctrl+w", "q"))
	assert.Equal(t, Action(""), km.ResolveChord("ctrl+t", "x"))
}

func TestResolveChord_PrefixOnly(t *testing.T) {
	// only the leader is bound (no chord under it) → ResolveChord returns empty
	km := Default()
	km.Bind("ctrl+w", ActionQuit)
	assert.Equal(t, Action(""), km.ResolveChord("ctrl+w", "x"))
}

func TestResolveChord_LayoutFallbackMissingForMultiRuneSecond(t *testing.T) {
	// layout fallback only applies when second is a single rune; multi-rune
	// strings like "esc" should not trigger a translation attempt
	km := Default()
	km.Bind("ctrl+w>esc", ActionDismiss)
	assert.Equal(t, ActionDismiss, km.ResolveChord("ctrl+w", "esc"))
	assert.Equal(t, Action(""), km.ResolveChord("ctrl+w", "tab"))
}

func TestParse_casePreservedForSingleChars(t *testing.T) {
	input := strings.NewReader("map N prev_item\nmap n next_item\n")
	maps, _, err := parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 2)
	assert.Equal(t, "N", maps[0].key)
	assert.Equal(t, "n", maps[1].key)
}

func TestDump_RoundTripsChords(t *testing.T) {
	// build a Keymap with a mix of single-key and chord bindings, dump, parse,
	// rebuild, and assert the bindings are structurally identical.
	km := &Keymap{bindings: make(map[string]Action), descriptions: defaultDescriptions()}
	km.Bind("j", ActionDown)
	km.Bind("q", ActionQuit)
	km.Bind("ctrl+w>x", ActionQuit)
	km.Bind("alt+t>n", ActionNextItem)
	km.Bind("ctrl+w>h", ActionHelp)

	var buf strings.Builder
	require.NoError(t, km.Dump(&buf))
	output := buf.String()

	// chord keys appear verbatim in dump output
	assert.Contains(t, output, "map ctrl+w>x quit")
	assert.Contains(t, output, "map alt+t>n next_item")
	assert.Contains(t, output, "map ctrl+w>h help")

	// parse the output back
	maps, unmaps, err := parse(strings.NewReader(output))
	require.NoError(t, err)
	assert.Empty(t, unmaps)

	rebuilt := &Keymap{bindings: make(map[string]Action), descriptions: defaultDescriptions()}
	for _, m := range maps {
		rebuilt.Bind(m.key, m.action)
	}

	// full structural equality on bindings
	assert.Equal(t, km.bindings, rebuilt.bindings, "dump -> parse -> rebuild must preserve bindings exactly")
}

func TestKeysFor_IncludesChordKeys(t *testing.T) {
	km := Default()
	km.Bind("ctrl+w>x", ActionQuit)

	keys := km.KeysFor(ActionQuit)
	// ASCII sort: 'c' (0x63) < 'q' (0x71); chord sorts before "q"
	assert.Equal(t, []string{"ctrl+w>x", "q"}, keys)

	// HelpSections joins the same slice with " / "
	sections := km.HelpSections()
	var joined string
	for _, sec := range sections {
		for _, entry := range sec.Entries {
			if entry.Action == ActionQuit {
				joined = entry.Keys
			}
		}
	}
	assert.Equal(t, "ctrl+w>x / q", joined)
}
