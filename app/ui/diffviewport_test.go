package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestDiffViewport_SetContent(t *testing.T) {
	t.Run("one row per line, CRLF normalized", func(t *testing.T) {
		v := newDiffViewport(10, 2)
		v.SetContent("a\r\nb\nc")
		assert.Equal(t, 3, v.TotalLineCount())
		assert.Equal(t, []string{"a", "b"}, v.visibleLines())
	})

	t.Run("offset inside the new content is kept", func(t *testing.T) {
		v := newDiffViewport(10, 2)
		v.SetContent("1\n2\n3\n4\n5")
		v.SetYOffset(2)
		v.SetContent("a\nb\nc\nd\ne\nf")
		assert.Equal(t, 2, v.YOffset)
	})

	t.Run("offset past the new last row moves to the bottom", func(t *testing.T) {
		v := newDiffViewport(10, 2)
		v.SetContent("1\n2\n3\n4\n5\n6")
		v.SetYOffset(4)
		v.SetContent("a\nb\nc")
		assert.Equal(t, 1, v.YOffset)
		assert.Equal(t, []string{"b", "c"}, v.visibleLines())
	})
}

func TestDiffViewport_SetYOffset(t *testing.T) {
	v := newDiffViewport(10, 3)
	v.SetContent("1\n2\n3\n4\n5")

	v.SetYOffset(-4)
	assert.Equal(t, 0, v.YOffset)
	v.SetYOffset(1)
	assert.Equal(t, 1, v.YOffset)
	v.SetYOffset(100)
	assert.Equal(t, 2, v.YOffset, "clamped so the last row stays at the bottom")

	v.GotoTop()
	assert.Equal(t, 0, v.YOffset)

	short := newDiffViewport(10, 5)
	short.SetContent("1\n2")
	short.SetYOffset(1)
	assert.Equal(t, 0, short.YOffset, "content shorter than the window cannot scroll")
}

func TestDiffViewport_View(t *testing.T) {
	t.Run("pads to width and height", func(t *testing.T) {
		v := newDiffViewport(6, 4)
		v.SetContent("ab\ncd")
		out := v.View()
		assert.Equal(t, 4, lipgloss.Height(out))
		for row := range strings.SplitSeq(out, "\n") {
			assert.Equal(t, 6, ansi.StringWidth(row))
		}
		assert.True(t, strings.HasPrefix(out, "ab    \ncd    \n"))
	})

	t.Run("shows only the rows at the offset", func(t *testing.T) {
		v := newDiffViewport(3, 2)
		v.SetContent("r0\nr1\nr2\nr3")
		v.SetYOffset(2)
		assert.Equal(t, "r2 \nr3 ", v.View())
	})

	t.Run("a row wider than the window is cut, not wrapped", func(t *testing.T) {
		v := newDiffViewport(5, 2)
		v.SetContent("\x1b[31mabcdefghij\x1b[39m\nok")
		out := v.View()
		rows := strings.Split(out, "\n")
		assert.Len(t, rows, 2, "a wrapped row would push the second row out of the window")
		assert.Equal(t, "abcde", ansi.Strip(rows[0]))
		assert.Contains(t, rows[0], "\x1b[31m", "styling survives the cut")
		assert.Equal(t, "ok   ", rows[1])
	})

	t.Run("a wide row outside the window costs the visible rows nothing", func(t *testing.T) {
		v := newDiffViewport(5, 2)
		v.SetContent("ab\ncd\n" + strings.Repeat("x", 50))
		lines := v.visibleLines()
		assert.Equal(t, []string{"ab", "cd"}, lines)
		assert.Same(t, &v.lines[0], &lines[0], "no copy when no visible row needs cutting")
	})

	t.Run("empty", func(t *testing.T) {
		v := newDiffViewport(3, 2)
		assert.Equal(t, 0, v.TotalLineCount())
		assert.Equal(t, "   \n   ", v.View())
	})
}
