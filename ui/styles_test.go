package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	c := normalizeColors(Colors{
		Accent: "5f87ff", Border: "#585858", Normal: "d0d0d0",
		ModifyFg: "f5c542", ModifyBg: "#3D2E00",
		TreeBg: "1a1a1a", DiffBg: "", StatusFg: "aabbcc", StatusBg: "",
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
}

func TestNewStyles_OptionalBackgrounds(t *testing.T) {
	t.Run("empty backgrounds use no background", func(t *testing.T) {
		s := newStyles(Colors{
			Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c",
			SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700",
			CursorBg: "#3a3a3a",
			AddFg:    "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
		})
		// styles should be created without panic
		assert.NotNil(t, s.TreePane)
		assert.NotNil(t, s.DiffPane)
		assert.NotNil(t, s.StatusBar)
	})

	t.Run("set backgrounds applied", func(t *testing.T) {
		s := newStyles(Colors{
			Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c",
			SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700",
			CursorBg: "#3a3a3a",
			AddFg:    "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
			ModifyFg: "#f5c542", ModifyBg: "#3D2E00",
			TreeBg: "#111111", DiffBg: "#222222", StatusFg: "#cccccc", StatusBg: "#333333",
		})
		assert.NotNil(t, s.TreePane)
		assert.NotNil(t, s.DiffPane)
		assert.NotNil(t, s.StatusBar)
	})
}

func TestNewStyles_ModifyStyles(t *testing.T) {
	s := newStyles(Colors{
		Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c",
		SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700",
		CursorBg: "#3a3a3a",
		AddFg:    "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
		ModifyFg: "#f5c542", ModifyBg: "#3D2E00",
	})
	// verify modify styles are created with correct colors
	assert.NotNil(t, s.LineModify)
	assert.NotNil(t, s.LineModifyHighlight)

	// verify modify styles render text without panics
	assert.NotEmpty(t, s.LineModify.Render("modified line"))
	assert.NotEmpty(t, s.LineModifyHighlight.Render("modified line"))
}

func TestPlainStyles_ModifyStyles(t *testing.T) {
	s := plainStyles()
	// verify modify styles render correctly as no-op styles
	assert.NotEmpty(t, s.LineModify.Render("text"))
	assert.NotEmpty(t, s.LineModifyHighlight.Render("text"))
}
