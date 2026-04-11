package style

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeColor(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{name: "empty", input: "", want: ""},
		{name: "with hash", input: "#ff0000", want: "#ff0000"},
		{name: "without hash", input: "ff0000", want: "#ff0000"},
		{name: "short hex", input: "abc", want: "#abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeColor(tc.input))
		})
	}
}

func TestNormalizeColors(t *testing.T) {
	t.Run("adds # prefix to bare hex values", func(t *testing.T) {
		c := normalizeColors(Colors{
			Accent: "5f87ff", Border: "#585858", Normal: "d0d0d0",
			ModifyFg: "f5c542", ModifyBg: "#3D2E00",
			TreeBg: "1a1a1a", DiffBg: "", StatusFg: "aabbcc", StatusBg: "",
			SearchFg: "1a1a1a", SearchBg: "#d7d700",
		})
		assert.Equal(t, "#5f87ff", c.Accent, "should add # prefix")
		assert.Equal(t, "#585858", c.Border, "should keep existing #")
		assert.Equal(t, "#d0d0d0", c.Normal)
		assert.Equal(t, "#f5c542", c.ModifyFg, "should add # prefix to modify fg")
		assert.Equal(t, "#3D2E00", c.ModifyBg, "should keep existing # on modify bg")
		assert.Equal(t, "#1a1a1a", c.TreeBg)
		assert.Empty(t, c.DiffBg, "empty should stay empty")
		assert.Equal(t, "#aabbcc", c.StatusFg)
		assert.Empty(t, c.StatusBg, "empty should stay empty")
		assert.Equal(t, "#1a1a1a", c.SearchFg, "should add # prefix to search fg")
		assert.Equal(t, "#d7d700", c.SearchBg, "should keep existing # on search bg")
	})

	t.Run("auto-derives WordAddBg from AddBg when empty", func(t *testing.T) {
		c := normalizeColors(Colors{AddBg: "#022800", RemoveBg: "#3D0100"})
		require.NotEmpty(t, c.WordAddBg, "WordAddBg should be auto-derived")
		assert.Equal(t, byte('#'), c.WordAddBg[0], "auto-derived WordAddBg should have # prefix")
		assert.NotEqual(t, c.AddBg, c.WordAddBg, "WordAddBg should differ from AddBg")
	})

	t.Run("auto-derives WordRemoveBg from RemoveBg when empty", func(t *testing.T) {
		c := normalizeColors(Colors{AddBg: "#022800", RemoveBg: "#3D0100"})
		require.NotEmpty(t, c.WordRemoveBg, "WordRemoveBg should be auto-derived")
		assert.Equal(t, byte('#'), c.WordRemoveBg[0], "auto-derived WordRemoveBg should have # prefix")
		assert.NotEqual(t, c.RemoveBg, c.WordRemoveBg, "WordRemoveBg should differ from RemoveBg")
	})

	t.Run("preserves explicitly set WordAddBg/WordRemoveBg", func(t *testing.T) {
		c := normalizeColors(Colors{
			AddBg: "#022800", RemoveBg: "#3D0100",
			WordAddBg: "#045e04", WordRemoveBg: "#5e0404",
		})
		assert.Equal(t, "#045e04", c.WordAddBg, "explicit WordAddBg should be preserved")
		assert.Equal(t, "#5e0404", c.WordRemoveBg, "explicit WordRemoveBg should be preserved")
	})

	t.Run("no derivation when AddBg/RemoveBg are empty", func(t *testing.T) {
		c := normalizeColors(Colors{})
		assert.Empty(t, c.WordAddBg, "WordAddBg should stay empty when AddBg is empty")
		assert.Empty(t, c.WordRemoveBg, "WordRemoveBg should stay empty when RemoveBg is empty")
	})

	t.Run("normalizes all 23 fields", func(t *testing.T) {
		c := normalizeColors(Colors{
			Accent: "aaaaaa", Border: "bbbbbb", Normal: "cccccc", Muted: "dddddd",
			SelectedFg: "111111", SelectedBg: "222222", Annotation: "333333",
			CursorFg: "444444", CursorBg: "555555",
			AddFg: "666666", AddBg: "777777", RemoveFg: "888888", RemoveBg: "999999",
			WordAddBg: "aabbcc", WordRemoveBg: "ddeeff",
			ModifyFg: "112233", ModifyBg: "445566",
			TreeBg: "778899", DiffBg: "001122",
			StatusFg: "334455", StatusBg: "667788",
			SearchFg: "990011", SearchBg: "223344",
		})
		assert.Equal(t, "#aaaaaa", c.Accent)
		assert.Equal(t, "#bbbbbb", c.Border)
		assert.Equal(t, "#cccccc", c.Normal)
		assert.Equal(t, "#dddddd", c.Muted)
		assert.Equal(t, "#111111", c.SelectedFg)
		assert.Equal(t, "#222222", c.SelectedBg)
		assert.Equal(t, "#333333", c.Annotation)
		assert.Equal(t, "#444444", c.CursorFg)
		assert.Equal(t, "#555555", c.CursorBg)
		assert.Equal(t, "#666666", c.AddFg)
		assert.Equal(t, "#777777", c.AddBg)
		assert.Equal(t, "#888888", c.RemoveFg)
		assert.Equal(t, "#999999", c.RemoveBg)
		assert.Equal(t, "#aabbcc", c.WordAddBg)
		assert.Equal(t, "#ddeeff", c.WordRemoveBg)
		assert.Equal(t, "#112233", c.ModifyFg)
		assert.Equal(t, "#445566", c.ModifyBg)
		assert.Equal(t, "#778899", c.TreeBg)
		assert.Equal(t, "#001122", c.DiffBg)
		assert.Equal(t, "#334455", c.StatusFg)
		assert.Equal(t, "#667788", c.StatusBg)
		assert.Equal(t, "#990011", c.SearchFg)
		assert.Equal(t, "#223344", c.SearchBg)
	})
}
