package keymap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLayoutResolve(t *testing.T) {
	t.Run("russian lowercase", func(t *testing.T) {
		tests := []struct{ from, to rune }{
			{'й', 'q'}, {'ц', 'w'}, {'у', 'e'}, {'к', 'r'}, {'е', 't'},
			{'н', 'y'}, {'г', 'u'}, {'ш', 'i'}, {'щ', 'o'}, {'з', 'p'},
			{'х', '['}, {'ъ', ']'},
			{'ф', 'a'}, {'ы', 's'}, {'в', 'd'}, {'а', 'f'}, {'п', 'g'},
			{'р', 'h'}, {'о', 'j'}, {'л', 'k'}, {'д', 'l'}, {'ж', ';'},
			{'э', '\''},
			{'я', 'z'}, {'ч', 'x'}, {'с', 'c'}, {'м', 'v'}, {'и', 'b'},
			{'т', 'n'}, {'ь', 'm'}, {'б', ','}, {'ю', '.'},
		}
		for _, tc := range tests {
			alias, ok := layoutResolve(tc.from)
			assert.True(t, ok, "should resolve %q", tc.from)
			assert.Equal(t, tc.to, alias, "%q should map to %q", tc.from, tc.to)
		}
	})

	t.Run("russian uppercase", func(t *testing.T) {
		tests := []struct{ from, to rune }{
			{'Й', 'Q'}, {'Ц', 'W'}, {'У', 'E'}, {'Н', 'Y'}, {'Г', 'U'},
			{'Ш', 'I'}, {'Щ', 'O'}, {'Ф', 'A'}, {'Р', 'H'}, {'О', 'J'},
			{'Л', 'K'}, {'Д', 'L'}, {'Я', 'Z'}, {'Ч', 'X'}, {'С', 'C'},
			{'М', 'V'}, {'И', 'B'}, {'Т', 'N'}, {'Ь', 'M'},
		}
		for _, tc := range tests {
			alias, ok := layoutResolve(tc.from)
			assert.True(t, ok, "should resolve %q", tc.from)
			assert.Equal(t, tc.to, alias, "%q should map to %q", tc.from, tc.to)
		}
	})

	t.Run("ukrainian extras", func(t *testing.T) {
		alias, ok := layoutResolve('і')
		assert.True(t, ok)
		assert.Equal(t, 's', alias)

		alias, ok = layoutResolve('ї')
		assert.True(t, ok)
		assert.Equal(t, ']', alias)

		alias, ok = layoutResolve('є')
		assert.True(t, ok)
		assert.Equal(t, '\'', alias)
	})

	t.Run("greek lowercase", func(t *testing.T) {
		tests := []struct{ from, to rune }{
			{'ς', 'w'}, {'ε', 'e'}, {'ρ', 'r'}, {'τ', 't'}, {'υ', 'y'},
			{'θ', 'u'}, {'ι', 'i'}, {'ο', 'o'}, {'π', 'p'},
			{'α', 'a'}, {'σ', 's'}, {'δ', 'd'}, {'φ', 'f'}, {'γ', 'g'},
			{'η', 'h'}, {'ξ', 'j'}, {'κ', 'k'}, {'λ', 'l'},
			{'ή', ';'}, {'ί', '\''},
			{'ζ', 'z'}, {'χ', 'x'}, {'ψ', 'c'}, {'ω', 'v'}, {'β', 'b'},
			{'ν', 'n'}, {'μ', 'm'},
		}
		for _, tc := range tests {
			alias, ok := layoutResolve(tc.from)
			assert.True(t, ok, "should resolve %q", tc.from)
			assert.Equal(t, tc.to, alias, "%q should map to %q", tc.from, tc.to)
		}
	})

	t.Run("greek uppercase", func(t *testing.T) {
		tests := []struct{ from, to rune }{
			{'Ε', 'E'}, {'Ρ', 'R'}, {'Τ', 'T'}, {'Υ', 'Y'},
			{'Θ', 'U'}, {'Ι', 'I'}, {'Ο', 'O'}, {'Π', 'P'},
			{'Α', 'A'}, {'Σ', 'S'}, {'Δ', 'D'}, {'Φ', 'F'},
			{'Γ', 'G'}, {'Η', 'H'}, {'Ξ', 'J'}, {'Κ', 'K'}, {'Λ', 'L'},
			{'Ζ', 'Z'}, {'Χ', 'X'}, {'Ψ', 'C'}, {'Ω', 'V'},
			{'Β', 'B'}, {'Ν', 'N'}, {'Μ', 'M'},
		}
		for _, tc := range tests {
			alias, ok := layoutResolve(tc.from)
			assert.True(t, ok, "should resolve %q", tc.from)
			assert.Equal(t, tc.to, alias, "%q should map to %q", tc.from, tc.to)
		}
	})

	t.Run("hebrew", func(t *testing.T) {
		tests := []struct{ from, to rune }{
			{'ק', 'e'}, {'ר', 'r'}, {'א', 't'}, {'ט', 'y'}, {'ו', 'u'},
			{'ן', 'i'}, {'ם', 'o'}, {'פ', 'p'},
			{'ש', 'a'}, {'ד', 's'}, {'ג', 'd'}, {'כ', 'f'}, {'ע', 'g'},
			{'י', 'h'}, {'ח', 'j'}, {'ל', 'k'}, {'ך', 'l'}, {'ף', ';'},
			{'ז', 'z'}, {'ס', 'x'}, {'ב', 'c'}, {'ה', 'v'}, {'נ', 'b'},
			{'מ', 'n'}, {'צ', 'm'}, {'ת', ','}, {'ץ', '.'},
		}
		for _, tc := range tests {
			alias, ok := layoutResolve(tc.from)
			assert.True(t, ok, "should resolve %q", tc.from)
			assert.Equal(t, tc.to, alias, "%q should map to %q", tc.from, tc.to)
		}
	})

	t.Run("ascii characters not mapped", func(t *testing.T) {
		// ASCII chars are ambiguous (same codepoint across layouts)
		for r := rune(0x20); r <= 0x7E; r++ {
			_, ok := layoutResolve(r)
			assert.False(t, ok, "ASCII %q should not be aliased", r)
		}
	})

	t.Run("unknown non-Latin not mapped", func(t *testing.T) {
		_, ok := layoutResolve('日') // Japanese kanji
		assert.False(t, ok)
		_, ok = layoutResolve('中')
		assert.False(t, ok)
		_, ok = layoutResolve(0x1F600) // emoji
		assert.False(t, ok)
	})
}

func TestKeymap_ResolveLayoutFallback(t *testing.T) {
	km := Default()

	t.Run("russian: г triggers toggle_untracked (u)", func(t *testing.T) {
		assert.Equal(t, ActionToggleUntracked, km.Resolve("г"))
	})

	t.Run("russian: о triggers down (j)", func(t *testing.T) {
		// о→j and 'j' is bound to ActionDown
		assert.Equal(t, ActionDown, km.Resolve("о"))
	})

	t.Run("russian: Т triggers prev_item (N)", func(t *testing.T) {
		assert.Equal(t, ActionPrevItem, km.Resolve("Т"))
	})

	t.Run("greek: ξ triggers down (j)", func(t *testing.T) {
		assert.Equal(t, ActionDown, km.Resolve("ξ"))
	})

	t.Run("greek: π triggers prev_item (p)", func(t *testing.T) {
		// π→p and 'p' is bound to ActionPrevItem
		assert.Equal(t, ActionPrevItem, km.Resolve("π"))
	})

	t.Run("hebrew: ח triggers down (j)", func(t *testing.T) {
		assert.Equal(t, ActionDown, km.Resolve("ח"))
	})

	t.Run("hebrew: ש triggers confirm (a)", func(t *testing.T) {
		// ש→a and 'a' is bound to ActionConfirm
		assert.Equal(t, ActionConfirm, km.Resolve("ש"))
	})

	t.Run("direct binding takes precedence over alias", func(t *testing.T) {
		customKm := Default()
		// bind a Cyrillic character directly to a different action
		customKm.Bind("г", ActionQuit)
		// should use the direct binding, not the layout alias to ActionToggleUntracked
		assert.Equal(t, ActionQuit, customKm.Resolve("г"))
	})

	t.Run("multi-char keys not aliased", func(t *testing.T) {
		assert.Equal(t, ActionHalfPageDown, km.Resolve("ctrl+d"))
		assert.Equal(t, ActionHalfPageUp, km.Resolve("ctrl+u"))
		assert.Equal(t, ActionPageDown, km.Resolve("pgdown"))
		assert.Equal(t, ActionTogglePane, km.Resolve("tab"))
		assert.Equal(t, ActionDismiss, km.Resolve("esc"))
	})

	t.Run("hebrew: א triggers toggle_tree (t)", func(t *testing.T) {
		// א→t which is bound to ActionToggleTree
		assert.Equal(t, ActionToggleTree, km.Resolve("א"))
	})

	t.Run("layout alias does not pollute KeysFor", func(t *testing.T) {
		km := Default()
		keys := km.KeysFor(ActionDown)
		assert.NotContains(t, keys, "ш")
		assert.NotContains(t, keys, "ξ")
		assert.NotContains(t, keys, "ח")
		assert.Contains(t, keys, "j")
		assert.Contains(t, keys, "down")
	})

	t.Run("layout alias does not pollute HelpSections", func(t *testing.T) {
		km := Default()
		sections := km.HelpSections()
		for _, sec := range sections {
			for _, entry := range sec.Entries {
				for _, k := range []string{"ш", "г", "ξ", "ח", "ש"} {
					assert.NotContains(t, entry.Keys, k,
						"help should not show layout alias %q for %s", k, entry.Action)
				}
			}
		}
	})
}
