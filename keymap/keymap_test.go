package keymap

import (
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
