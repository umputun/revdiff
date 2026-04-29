package style

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/revdiff/app/diff"
)

// fullColorsForTesting has every field populated — exercises the primary resolution path.
var fullColorsForTesting = Colors{
	Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Annotation: "#ffd700",
	SelectedFg: "#ffffaf", SelectedBg: "#303030",
	AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
	ModifyFg: "#f5c542", ModifyBg: "#3D2E00",
	CursorFg: "#000000", CursorBg: "#3a3a3a",
	WordAddBg: "#045e04", WordRemoveBg: "#5e0404",
	TreeBg: "#111111", DiffBg: "#222222",
	StatusFg: "#cccccc", StatusBg: "#333333",
	SearchFg: "#1a1a1a", SearchBg: "#d7d700",
	Border: "#585858",
}

// sparseColorsForTesting omits optional fields — exercises fallback resolution paths.
var sparseColorsForTesting = Colors{
	Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Annotation: "#ffd700",
	SelectedFg: "#ffffaf", SelectedBg: "#303030",
	AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
	Border: "#585858",
}

func TestNewResolver(t *testing.T) {
	r := NewResolver(fullColorsForTesting)
	assert.NotNil(t, r.styles, "styles map should be initialized")
	assert.NotEmpty(t, r.colors.Accent, "colors should be stored")
}

func TestPlainResolver(t *testing.T) {
	r := PlainResolver()
	assert.NotNil(t, r.styles, "styles map should be initialized")
	assert.Empty(t, r.colors.Accent, "colors should be empty for plain resolver")
}

func TestResolver_Color(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	tests := []struct {
		name string
		key  ColorKey
		want bool // true if non-empty expected
	}{
		{"accent fg", ColorKeyAccentFg, true},
		{"muted fg", ColorKeyMutedFg, true},
		{"annotation fg", ColorKeyAnnotationFg, true},
		{"status fg", ColorKeyStatusFg, true},
		{"diff pane bg", ColorKeyDiffPaneBg, true},
		{"add line bg", ColorKeyAddLineBg, true},
		{"remove line bg", ColorKeyRemoveLineBg, true},
		{"word add bg", ColorKeyWordAddBg, true},
		{"word remove bg", ColorKeyWordRemoveBg, true},
		{"search bg", ColorKeySearchBg, true},
		{"unknown", ColorKeyUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Color(tt.key)
			if tt.want {
				assert.NotEmpty(t, string(got), "expected non-empty color for %s", tt.key)
				assert.Contains(t, string(got), "\033[", "expected ANSI sequence for %s", tt.key)
			} else {
				assert.Empty(t, string(got), "expected empty color for %s", tt.key)
			}
		})
	}
}

func TestResolver_Color_coversAllKeys(t *testing.T) {
	for _, fixture := range []struct {
		name string
		c    Colors
	}{
		{"full", fullColorsForTesting},
		{"sparse", sparseColorsForTesting},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			r := NewResolver(fixture.c)
			for _, k := range ColorKeyValues {
				if k == ColorKeyUnknown {
					continue
				}
				// no panic = switch handles every key
				_ = r.Color(k)
			}
		})
	}
}

func TestResolver_Color_StatusFgFallback(t *testing.T) {
	r := NewResolver(sparseColorsForTesting)
	got := r.Color(ColorKeyStatusFg)
	assert.NotEmpty(t, string(got), "StatusFg with empty StatusFg field should fall back to Muted")
	assert.Equal(t, r.Color(ColorKeyMutedFg), got, "StatusFg fallback should equal MutedFg")
}

func TestResolver_Color_PlainResolver(t *testing.T) {
	r := PlainResolver()
	// plain resolver has no colors, all keys should return empty
	for _, k := range ColorKeyValues {
		got := r.Color(k)
		assert.Empty(t, string(got), "plain resolver should return empty for %s", k)
	}
}

func TestResolver_Style(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	tests := []struct {
		name string
		key  StyleKey
	}{
		{"line add", StyleKeyLineAdd},
		{"line remove", StyleKeyLineRemove},
		{"line context", StyleKeyLineContext},
		{"line modify", StyleKeyLineModify},
		{"line add highlight", StyleKeyLineAddHighlight},
		{"line remove highlight", StyleKeyLineRemoveHighlight},
		{"line context highlight", StyleKeyLineContextHighlight},
		{"line modify highlight", StyleKeyLineModifyHighlight},
		{"line number", StyleKeyLineNumber},
		{"tree pane", StyleKeyTreePane},
		{"tree pane active", StyleKeyTreePaneActive},
		{"diff pane", StyleKeyDiffPane},
		{"diff pane active", StyleKeyDiffPaneActive},
		{"dir entry", StyleKeyDirEntry},
		{"file entry", StyleKeyFileEntry},
		{"file selected", StyleKeyFileSelected},
		{"annotation mark", StyleKeyAnnotationMark},
		{"reviewed mark", StyleKeyReviewedMark},
		{"status added", StyleKeyStatusAdded},
		{"status deleted", StyleKeyStatusDeleted},
		{"status untracked", StyleKeyStatusUntracked},
		{"status default", StyleKeyStatusDefault},
		{"status bar", StyleKeyStatusBar},
		{"search match", StyleKeySearchMatch},
		{"annot input text", StyleKeyAnnotInputText},
		{"annot input placeholder", StyleKeyAnnotInputPlaceholder},
		{"annot input cursor", StyleKeyAnnotInputCursor},
		{"annot list border", StyleKeyAnnotListBorder},
		{"help box", StyleKeyHelpBox},
		{"theme select box", StyleKeyThemeSelectBox},
		{"theme select box focused", StyleKeyThemeSelectBoxFocused},
		{"info box", StyleKeyInfoBox},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// should not panic and should return a usable style
			got := r.Style(tt.key)
			_ = got.Render("test") // verify style is functional
		})
	}
}

func TestResolver_Style_coversAllKeys(t *testing.T) {
	for _, fixture := range []struct {
		name string
		c    Colors
	}{
		{"full", fullColorsForTesting},
		{"sparse", sparseColorsForTesting},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			r := NewResolver(fixture.c)
			for _, k := range StyleKeyValues {
				if k == StyleKeyUnknown {
					continue
				}
				// exhaustiveness check: no panic for any defined key.
				// lipgloss.Style is a struct value type so NotNil would be tautological.
				_ = r.Style(k)
			}
		})
	}
}

func TestResolver_Style_unknownReturnsEmpty(t *testing.T) {
	r := NewResolver(fullColorsForTesting)
	got := r.Style(StyleKeyUnknown)
	// should return an empty/default style, not panic
	_ = got.Render("test")
}

func TestResolver_LineBg(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	tests := []struct {
		name   string
		change diff.ChangeType
		want   bool // true if non-empty expected
	}{
		{"add", diff.ChangeAdd, true},
		{"remove", diff.ChangeRemove, true},
		{"context", diff.ChangeContext, false},
		{"divider", diff.ChangeDivider, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.LineBg(tt.change)
			if tt.want {
				assert.NotEmpty(t, string(got), "expected non-empty LineBg for %s", tt.change)
				assert.Contains(t, string(got), "\033[48;2;", "expected ANSI bg sequence")
			} else {
				assert.Empty(t, string(got), "expected empty LineBg for %s", tt.change)
			}
		})
	}
}

func TestResolver_LineFg(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	tests := []struct {
		name   string
		change diff.ChangeType
		want   bool // true if non-empty expected
	}{
		{"add", diff.ChangeAdd, true},
		{"remove", diff.ChangeRemove, true},
		{"context", diff.ChangeContext, false},
		{"divider", diff.ChangeDivider, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.LineFg(tt.change)
			if tt.want {
				assert.NotEmpty(t, string(got), "expected non-empty LineFg for %s", tt.change)
				assert.Contains(t, string(got), "\033[38;2;", "expected ANSI fg sequence")
			} else {
				assert.Empty(t, string(got), "expected empty LineFg for %s", tt.change)
			}
		})
	}
}

func TestResolver_LineStyle(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	tests := []struct {
		name        string
		change      diff.ChangeType
		highlighted bool
		wantKey     StyleKey
	}{
		{"add plain", diff.ChangeAdd, false, StyleKeyLineAdd},
		{"add highlighted", diff.ChangeAdd, true, StyleKeyLineAddHighlight},
		{"remove plain", diff.ChangeRemove, false, StyleKeyLineRemove},
		{"remove highlighted", diff.ChangeRemove, true, StyleKeyLineRemoveHighlight},
		{"context plain", diff.ChangeContext, false, StyleKeyLineContext},
		{"context highlighted", diff.ChangeContext, true, StyleKeyLineContextHighlight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.LineStyle(tt.change, tt.highlighted)
			expected := r.Style(tt.wantKey)
			// compare rendered output to verify they produce the same style
			assert.Equal(t, expected.Render("x"), got.Render("x"))
		})
	}
}

func TestResolver_WordDiffBg(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	tests := []struct {
		name   string
		change diff.ChangeType
		want   bool
	}{
		{"add", diff.ChangeAdd, true},
		{"remove", diff.ChangeRemove, true},
		{"context", diff.ChangeContext, false},
		{"divider", diff.ChangeDivider, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.WordDiffBg(tt.change)
			if tt.want {
				assert.NotEmpty(t, string(got))
				assert.Contains(t, string(got), "\033[48;2;")
			} else {
				assert.Empty(t, string(got))
			}
		})
	}
}

func TestResolver_IndicatorBg(t *testing.T) {
	r := NewResolver(fullColorsForTesting)

	t.Run("add line uses add bg", func(t *testing.T) {
		got := r.IndicatorBg(diff.ChangeAdd)
		assert.Equal(t, r.LineBg(diff.ChangeAdd), got)
	})

	t.Run("remove line uses remove bg", func(t *testing.T) {
		got := r.IndicatorBg(diff.ChangeRemove)
		assert.Equal(t, r.LineBg(diff.ChangeRemove), got)
	})

	t.Run("context falls back to diff pane bg", func(t *testing.T) {
		got := r.IndicatorBg(diff.ChangeContext)
		assert.NotEmpty(t, string(got), "context indicator should fall back to DiffBg")
		assert.Equal(t, r.Color(ColorKeyDiffPaneBg), got)
	})

	t.Run("divider falls back to diff pane bg", func(t *testing.T) {
		got := r.IndicatorBg(diff.ChangeDivider)
		assert.Equal(t, r.Color(ColorKeyDiffPaneBg), got)
	})
}

func TestResolver_IndicatorBg_noDiffBg(t *testing.T) {
	// when DiffBg is empty, context indicator should return empty
	c := sparseColorsForTesting
	c.DiffBg = ""
	r := NewResolver(c)

	got := r.IndicatorBg(diff.ChangeContext)
	assert.Empty(t, string(got), "indicator bg with no DiffBg should be empty for context")
}

func TestPlainResolver_Style(t *testing.T) {
	r := PlainResolver()

	// plain resolver should have styles with borders but no colors
	t.Run("tree pane has border", func(t *testing.T) {
		s := r.Style(StyleKeyTreePane)
		rendered := s.Render("test")
		assert.NotEqual(t, "test", rendered, "tree pane should have border decoration")
	})

	t.Run("file selected configured", func(t *testing.T) {
		s := r.Style(StyleKeyFileSelected)
		assert.True(t, s.GetReverse(), "file selected should use reverse")
	})

	t.Run("search match configured", func(t *testing.T) {
		s := r.Style(StyleKeySearchMatch)
		assert.True(t, s.GetReverse(), "search match should use reverse")
	})
}

func TestPlainResolver_LineBg(t *testing.T) {
	r := PlainResolver()
	// plain resolver has no colors, LineBg should return empty
	assert.Empty(t, string(r.LineBg(diff.ChangeAdd)))
	assert.Empty(t, string(r.LineBg(diff.ChangeRemove)))
	assert.Empty(t, string(r.LineBg(diff.ChangeContext)))
}

func TestPlainResolver_WordDiffBg(t *testing.T) {
	r := PlainResolver()
	assert.Empty(t, string(r.WordDiffBg(diff.ChangeAdd)))
	assert.Empty(t, string(r.WordDiffBg(diff.ChangeRemove)))
}

func TestResolver_StyleKeyInfoBox(t *testing.T) {
	t.Run("full colors draws border and background", func(t *testing.T) {
		r := NewResolver(fullColorsForTesting)
		s := r.Style(StyleKeyInfoBox)
		rendered := s.Render("x")
		// border decoration or background should make the output differ from input
		assert.NotEqual(t, "x", rendered, "info box should render with border/background")
		// diff bg is set, so background escape should be present
		assert.Contains(t, rendered, "\x1b[", "styled output should contain ANSI escapes")
	})

	t.Run("sparse colors still render border", func(t *testing.T) {
		r := NewResolver(sparseColorsForTesting)
		s := r.Style(StyleKeyInfoBox)
		rendered := s.Render("x")
		assert.NotEqual(t, "x", rendered, "info box should render border even without DiffBg")
	})

	t.Run("plain resolver preserves border", func(t *testing.T) {
		r := PlainResolver()
		s := r.Style(StyleKeyInfoBox)
		rendered := s.Render("x")
		assert.NotEqual(t, "x", rendered, "plain info box should still have border decoration")
	})
}
