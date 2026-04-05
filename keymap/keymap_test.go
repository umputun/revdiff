package keymap

import (
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
		{"v", ActionToggleCollapsed}, {"w", ActionToggleWrap}, {"t", ActionToggleTree},
		{"L", ActionToggleLineNums}, {".", ActionToggleHunk}, {"f", ActionFilter},
		{"q", ActionQuit}, {"Q", ActionDiscardQuit}, {"?", ActionHelp}, {"esc", ActionDismiss},
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

	t.Run("three keys action", func(t *testing.T) {
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

func TestIsValidAction(t *testing.T) {
	assert.True(t, IsValidAction(ActionQuit))
	assert.True(t, IsValidAction(ActionDown))
	assert.False(t, IsValidAction(Action("nonexistent")))
	assert.False(t, IsValidAction(Action("")))
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
	maps, unmaps, err := Parse(input)
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
	maps, unmaps, err := Parse(input)
	require.NoError(t, err)
	assert.Empty(t, maps)
	assert.Equal(t, []string{"q", "j"}, unmaps)
}

func TestParse_commentsAndBlanks(t *testing.T) {
	input := strings.NewReader("# comment\n\n  # indented comment\n  \nmap x quit\n")
	maps, unmaps, err := Parse(input)
	require.NoError(t, err)
	assert.Empty(t, unmaps)
	require.Len(t, maps, 1)
	assert.Equal(t, "x", maps[0].key)
}

func TestParse_unknownAction(t *testing.T) {
	input := strings.NewReader("map x fly_away\nmap y quit\n")
	maps, unmaps, err := Parse(input)
	require.NoError(t, err)
	assert.Empty(t, unmaps)
	// unknown action skipped, valid one kept
	require.Len(t, maps, 1)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_invalidLines(t *testing.T) {
	input := strings.NewReader("map\nfoo bar baz\nmap x quit\n")
	maps, _, err := Parse(input)
	require.NoError(t, err)
	// only valid line parsed
	require.Len(t, maps, 1)
	assert.Equal(t, ActionQuit, maps[0].action)
}

func TestParse_duplicateMapLastWins(t *testing.T) {
	input := strings.NewReader("map x quit\nmap x help\n")
	maps, _, err := Parse(input)
	require.NoError(t, err)
	// both entries returned; Load applies them in order (last wins)
	require.Len(t, maps, 2)
	assert.Equal(t, ActionQuit, maps[0].action)
	assert.Equal(t, ActionHelp, maps[1].action)
}

func TestParse_keyNormalization(t *testing.T) {
	input := strings.NewReader("map page_down page_down\nmap Ctrl+D half_page_down\n")
	maps, _, err := Parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 2)
	assert.Equal(t, "pgdown", maps[0].key) // normalized from page_down
	assert.Equal(t, "ctrl+d", maps[1].key) // normalized ctrl case
}

func TestParse_unmapNormalization(t *testing.T) {
	input := strings.NewReader("unmap page_down\n")
	_, unmaps, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"pgdown"}, unmaps)
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
	km.Dump(&buf)
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
	km.Dump(&buf)

	// parse the dumped output
	maps, unmaps, err := Parse(strings.NewReader(buf.String()))
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
	km.Dump(&buf)
	output := buf.String()

	// x should appear as quit binding, q should not
	assert.Contains(t, output, "map x quit")
	assert.NotContains(t, output, "map q quit")
}

func TestDump_unmappedActionOmitted(t *testing.T) {
	km := Default()
	km.Unbind("/") // search only has one key

	var buf strings.Builder
	km.Dump(&buf)
	output := buf.String()

	// search action should not appear at all
	assert.NotContains(t, output, "search")
}

func TestParse_casePreservedForSingleChars(t *testing.T) {
	input := strings.NewReader("map N prev_item\nmap n next_item\n")
	maps, _, err := Parse(input)
	require.NoError(t, err)
	require.Len(t, maps, 2)
	assert.Equal(t, "N", maps[0].key)
	assert.Equal(t, "n", maps[1].key)
}
