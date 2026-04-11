package style

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestNewRenderer(t *testing.T) {
	res := NewResolver(fullColorsForTesting)
	rnd := NewRenderer(res)
	assert.NotEmpty(t, rnd.res.colors.Accent, "renderer should hold wired resolver")
}

func TestRenderer_AnnotationInline(t *testing.T) {
	tests := []struct {
		name     string
		resolver Resolver
		text     string
		checks   []string
	}{
		{"colored", NewResolver(fullColorsForTesting), "hello", []string{"\033[3m", "hello", "\033[23m", "\033[39m", "\033[49m"}},
		{"plain", PlainResolver(), "hello", []string{"\033[3m", "hello", "\033[23m"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rnd := NewRenderer(tt.resolver)
			got := rnd.AnnotationInline(tt.text)
			for _, check := range tt.checks {
				assert.Contains(t, got, check, "expected %q in output", check)
			}
		})
	}
}

func TestRenderer_AnnotationInline_emptyText(t *testing.T) {
	rnd := NewRenderer(NewResolver(fullColorsForTesting))
	got := rnd.AnnotationInline("")
	assert.Contains(t, got, "\033[3m\033[23m", "empty text should still have italic on/off")
}

func TestRenderer_DiffCursor(t *testing.T) {
	t.Run("no colors", func(t *testing.T) {
		rnd := NewRenderer(PlainResolver())
		got := rnd.DiffCursor(true)
		assert.Equal(t, "\033[7m▶\033[27m", got, "no-colors should use reverse video")
	})

	t.Run("colored", func(t *testing.T) {
		rnd := NewRenderer(NewResolver(fullColorsForTesting))
		got := rnd.DiffCursor(false)
		assert.Contains(t, got, "▶", "should contain cursor glyph")
		assert.Contains(t, got, "\033[38;2;", "should have fg ANSI sequence")
		assert.Contains(t, got, "\033[48;2;", "should have bg ANSI sequence")
		assert.Contains(t, got, string(ResetFg), "should reset fg")
		assert.Contains(t, got, string(ResetBg), "should reset bg")
	})

	t.Run("colored with cursor bg fallback", func(t *testing.T) {
		// sparse colors have no CursorBg, should fall back to DiffBg (also empty in sparse)
		c := fullColorsForTesting
		c.CursorBg = ""
		c.DiffBg = "#aabbcc"
		rnd := NewRenderer(NewResolver(c))
		got := rnd.DiffCursor(false)
		assert.Contains(t, got, "▶")
		// the bg should use DiffBg since CursorBg is empty
		assert.Contains(t, got, "\033[48;2;170;187;204m", "should use DiffBg as fallback")
	})
}

func TestRenderer_StatusBarSeparator(t *testing.T) {
	t.Run("colored", func(t *testing.T) {
		rnd := NewRenderer(NewResolver(fullColorsForTesting))
		got := rnd.StatusBarSeparator()
		assert.Contains(t, got, "|", "should contain pipe separator")
		assert.True(t, strings.HasPrefix(got, " "), "should start with space")
		assert.True(t, strings.HasSuffix(got, " "), "should end with space")
	})

	t.Run("plain", func(t *testing.T) {
		rnd := NewRenderer(PlainResolver())
		got := rnd.StatusBarSeparator()
		assert.Equal(t, " | ", got, "plain separator has no ANSI codes")
	})
}

func TestRenderer_FileStatusMark(t *testing.T) {
	rnd := NewRenderer(NewResolver(fullColorsForTesting))

	tests := []struct {
		name   string
		status diff.FileStatus
	}{
		{"added", diff.FileAdded},
		{"modified", diff.FileModified},
		{"deleted", diff.FileDeleted},
		{"renamed", diff.FileRenamed},
		{"untracked", diff.FileUntracked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rnd.FileStatusMark(tt.status)
			require.Contains(t, got, string(tt.status), "should contain the status char")
			assert.True(t, strings.HasSuffix(got, " "), "should end with space")
			assert.Contains(t, got, "\033[38;2;", "should have ANSI fg sequence")
		})
	}
}

func TestRenderer_FileStatusMark_plain(t *testing.T) {
	rnd := NewRenderer(PlainResolver())
	got := rnd.FileStatusMark(diff.FileAdded)
	assert.Equal(t, "A ", got, "plain mode has no ANSI codes, just status + space")
}

func TestRenderer_FileReviewedMark(t *testing.T) {
	t.Run("colored", func(t *testing.T) {
		rnd := NewRenderer(NewResolver(fullColorsForTesting))
		got := rnd.FileReviewedMark()
		assert.Contains(t, got, "✓", "should contain checkmark")
		assert.Contains(t, got, "\033[38;2;", "should have ANSI fg")
		assert.True(t, strings.HasSuffix(got, " "), "should end with space")
	})

	t.Run("plain", func(t *testing.T) {
		rnd := NewRenderer(PlainResolver())
		got := rnd.FileReviewedMark()
		assert.Equal(t, "✓ ", got, "plain mode has no color")
	})
}

func TestRenderer_FileAnnotationMark(t *testing.T) {
	t.Run("colored", func(t *testing.T) {
		rnd := NewRenderer(NewResolver(fullColorsForTesting))
		got := rnd.FileAnnotationMark()
		assert.Contains(t, got, " *", "should contain annotation marker")
		assert.Contains(t, got, "\033[38;2;", "should have ANSI fg sequence")
		assert.Contains(t, got, string(ResetFg), "should reset fg")
	})

	t.Run("plain", func(t *testing.T) {
		rnd := NewRenderer(PlainResolver())
		got := rnd.FileAnnotationMark()
		assert.Equal(t, " *", got, "plain mode has no color")
	})
}

func TestRenderer_fileStatusFg(t *testing.T) {
	rnd := NewRenderer(NewResolver(fullColorsForTesting))

	t.Run("added uses AddFg", func(t *testing.T) {
		got := rnd.fileStatusFg(diff.FileAdded)
		assert.NotEmpty(t, string(got), "should have color for added files")
	})

	t.Run("untracked uses AddFg", func(t *testing.T) {
		added := rnd.fileStatusFg(diff.FileAdded)
		untracked := rnd.fileStatusFg(diff.FileUntracked)
		assert.Equal(t, added, untracked, "added and untracked should use same color")
	})

	t.Run("deleted uses RemoveFg", func(t *testing.T) {
		got := rnd.fileStatusFg(diff.FileDeleted)
		assert.NotEmpty(t, string(got))
		assert.NotEqual(t, rnd.fileStatusFg(diff.FileAdded), got, "deleted should differ from added")
	})

	t.Run("default uses Muted", func(t *testing.T) {
		got := rnd.fileStatusFg(diff.FileModified)
		assert.NotEmpty(t, string(got))
	})

	t.Run("plain resolver returns empty", func(t *testing.T) {
		plain := NewRenderer(PlainResolver())
		got := plain.fileStatusFg(diff.FileAdded)
		assert.Empty(t, string(got), "plain resolver has no colors")
	})
}
