package sidepane

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// fileEntries is a test helper converting a slice of file paths to diff.FileEntry.
func fileEntries(paths ...string) []diff.FileEntry {
	entries := make([]diff.FileEntry, len(paths))
	for i, p := range paths {
		entries[i] = diff.FileEntry{Path: p}
	}
	return entries
}

func TestFileTree_BuildEntries(t *testing.T) {
	ft := NewFileTree(fileEntries("internal/handler.go", "internal/store.go", "main.go"))

	assert.Len(t, ft.entries, 5) // 2 dirs (./, internal/) + 3 files
	assert.True(t, ft.entries[0].isDir)
	assert.Equal(t, "./", ft.entries[0].name)
	assert.Equal(t, "main.go", ft.entries[1].name)
	assert.Equal(t, "main.go", ft.entries[1].path)
	assert.True(t, ft.entries[2].isDir)
	assert.Equal(t, "internal/", ft.entries[2].name)
	assert.Equal(t, "handler.go", ft.entries[3].name)
	assert.Equal(t, "internal/handler.go", ft.entries[3].path)
	assert.Equal(t, "store.go", ft.entries[4].name)
}

func TestFileTree_BuildEntriesEmpty(t *testing.T) {
	ft := NewFileTree(nil)
	assert.Empty(t, ft.entries)
}

func TestFileTree_SelectedFile(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go"))

	assert.Equal(t, "a.go", ft.SelectedFile())

	ft.Move(MotionDown)
	assert.Equal(t, "b.go", ft.SelectedFile())
}

func TestFileTree_MoveDownUp(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))

	assert.Equal(t, "a.go", ft.SelectedFile())

	ft.Move(MotionDown)
	assert.Equal(t, "b.go", ft.SelectedFile())

	ft.Move(MotionDown)
	assert.Equal(t, "c.go", ft.SelectedFile())

	// at the end, move down does nothing
	ft.Move(MotionDown)
	assert.Equal(t, "c.go", ft.SelectedFile())

	ft.Move(MotionUp)
	assert.Equal(t, "b.go", ft.SelectedFile())

	ft.Move(MotionUp)
	assert.Equal(t, "a.go", ft.SelectedFile())

	// at the top, move up does nothing
	ft.Move(MotionUp)
	assert.Equal(t, "a.go", ft.SelectedFile())
}

func TestFileTree_NextPrevFile(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))

	ft.StepFile(DirectionNext)
	assert.Equal(t, "b.go", ft.SelectedFile())

	ft.StepFile(DirectionNext)
	assert.Equal(t, "c.go", ft.SelectedFile())

	// wraps around
	ft.StepFile(DirectionNext)
	assert.Equal(t, "a.go", ft.SelectedFile())

	ft.StepFile(DirectionPrev)
	assert.Equal(t, "c.go", ft.SelectedFile())

	ft.StepFile(DirectionPrev)
	assert.Equal(t, "b.go", ft.SelectedFile())

	ft.StepFile(DirectionPrev)
	assert.Equal(t, "a.go", ft.SelectedFile())
}

func TestFileTree_HasNextPrevFile(t *testing.T) {
	t.Run("single file", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go"))
		assert.False(t, ft.HasFile(DirectionNext))
		assert.False(t, ft.HasFile(DirectionPrev))
	})

	t.Run("first file", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		assert.Equal(t, "a.go", ft.SelectedFile())
		assert.True(t, ft.HasFile(DirectionNext))
		assert.False(t, ft.HasFile(DirectionPrev))
	})

	t.Run("middle file", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		ft.Move(MotionDown)
		assert.Equal(t, "b.go", ft.SelectedFile())
		assert.True(t, ft.HasFile(DirectionNext))
		assert.True(t, ft.HasFile(DirectionPrev))
	})

	t.Run("last file", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		ft.Move(MotionLast)
		assert.Equal(t, "c.go", ft.SelectedFile())
		assert.False(t, ft.HasFile(DirectionNext))
		assert.True(t, ft.HasFile(DirectionPrev))
	})
}

func TestFileTree_ToggleFilter(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
	annotated := map[string]bool{"a.go": true, "c.go": true}

	ft.ToggleFilter(annotated)
	assert.True(t, ft.filter)

	// should only show annotated files
	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
			assert.True(t, annotated[e.path], "file %s should be annotated", e.path)
		}
	}
	assert.Equal(t, 2, fileCount)

	// toggle back
	ft.ToggleFilter(annotated)
	assert.False(t, ft.filter)

	fileCount = 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 3, fileCount)
}

func TestFileTree_ToggleFilterNoAnnotations(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go"))
	annotated := map[string]bool{}

	ft.ToggleFilter(annotated)
	// should stay on all files since no annotated files
	assert.False(t, ft.FilterActive())
}

func TestFileTree_Render(t *testing.T) {
	ft := NewFileTree(fileEntries("internal/handler.go", "main.go"))
	ft.cursor = 1 // select handler.go
	res := style.NewResolver(style.Colors{Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700", CursorBg: "#3a3a3a", AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100"})
	rnd := style.NewRenderer(res)

	result := ft.Render(FileTreeRender{Width: 30, Height: 100, Annotated: map[string]bool{"internal/handler.go": true}, Resolver: res, Renderer: rnd})
	assert.Contains(t, result, "internal/")
	assert.Contains(t, result, "handler.go")
	assert.Contains(t, result, "main.go")
}

func TestFileTree_RenderEmpty(t *testing.T) {
	ft := NewFileTree(nil)
	res := style.NewResolver(style.Colors{Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700", CursorBg: "#3a3a3a", AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100"})
	rnd := style.NewRenderer(res)
	result := ft.Render(FileTreeRender{Width: 30, Height: 100, Resolver: res, Renderer: rnd})
	assert.Contains(t, result, "no changed files")
}

func TestFileTree_SetFiles(t *testing.T) {
	ft := NewFileTree(fileEntries("b.go", "c.go", "d.go"))
	assert.Equal(t, "b.go", ft.SelectedFile())
}

func TestFileTree_SetFilesNewList(t *testing.T) {
	ft := NewFileTree(fileEntries("x.go", "y.go"))
	assert.Equal(t, "x.go", ft.SelectedFile())
}

func TestFileTree_DirectoryGrouping(t *testing.T) {
	ft := NewFileTree(fileEntries("cmd/main.go", "internal/handler.go", "internal/store.go"))

	assert.Len(t, ft.entries, 5)
	assert.Equal(t, "cmd/", ft.entries[0].name)
	assert.True(t, ft.entries[0].isDir)
	assert.Equal(t, "main.go", ft.entries[1].name)
	assert.Equal(t, "internal/", ft.entries[2].name)
	assert.True(t, ft.entries[2].isDir)
	assert.Equal(t, "handler.go", ft.entries[3].name)
	assert.Equal(t, "store.go", ft.entries[4].name)
}

func TestFileTree_FileIndices(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go"))
	indices := ft.fileIndices()
	// dir at 0, files at 1 and 2
	assert.Equal(t, []int{1, 2}, indices)
}

func TestFileTree_RefreshFilter(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))

	ft.ToggleFilter(map[string]bool{"a.go": true})
	assert.True(t, ft.FilterActive())

	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 1, fileCount)

	// refresh with a.go and c.go annotated - c.go should now appear
	ft.RefreshFilter(map[string]bool{"a.go": true, "c.go": true})
	fileCount = 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount)
}

func TestFileTree_RefreshFilterNoAnnotations(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go"))
	ft.ToggleFilter(map[string]bool{"a.go": true})
	assert.True(t, ft.FilterActive())

	// refresh with no annotations - should disable filter
	ft.RefreshFilter(map[string]bool{})
	assert.False(t, ft.FilterActive())

	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount)
}

func TestFileTree_RefreshFilterPreservesCursor(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
	ft.ToggleFilter(map[string]bool{"a.go": true, "b.go": true})
	assert.True(t, ft.FilterActive())

	// move to b.go
	ft.Move(MotionDown)
	assert.Equal(t, "b.go", ft.SelectedFile())

	// refresh filter - cursor should remain on b.go
	ft.RefreshFilter(map[string]bool{"a.go": true, "b.go": true})
	assert.Equal(t, "b.go", ft.SelectedFile())
}

func TestFileTree_RefreshFilterNotActive(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go"))
	assert.False(t, ft.FilterActive())

	// refresh when filter is not active should be a no-op
	ft.RefreshFilter(map[string]bool{"a.go": true})
	assert.False(t, ft.FilterActive())

	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount, "all files should remain visible")
}

func TestFileTree_PageDown(t *testing.T) {
	files := fileEntries(
		"cmd/main.go", "cmd/flags.go",
		"internal/a.go", "internal/b.go", "internal/c.go", "internal/d.go",
		"internal/e.go", "internal/f.go", "internal/g.go", "internal/h.go",
	)
	ft := NewFileTree(files)
	assert.Equal(t, "cmd/flags.go", ft.SelectedFile(), "cursor should start on first file (flags.go)")

	ft.Move(MotionPageDown, 3)
	assert.Equal(t, "internal/a.go", ft.SelectedFile(), "pageDown(3) should advance ~3 visual rows")

	ft.Move(MotionPageDown, 100)
	assert.Equal(t, "internal/h.go", ft.SelectedFile(), "pageDown past end should clamp at last file")

	ft.Move(MotionPageDown, 1)
	assert.Equal(t, "internal/h.go", ft.SelectedFile(), "pageDown at end should stay at last file")
}

func TestFileTree_PageUp(t *testing.T) {
	files := fileEntries(
		"cmd/main.go", "cmd/flags.go",
		"internal/a.go", "internal/b.go", "internal/c.go", "internal/d.go",
		"internal/e.go", "internal/f.go", "internal/g.go", "internal/h.go",
	)
	ft := NewFileTree(files)

	ft.Move(MotionLast)
	assert.Equal(t, "internal/h.go", ft.SelectedFile())

	ft.Move(MotionPageUp, 3)
	assert.Equal(t, "internal/e.go", ft.SelectedFile(), "pageUp(3) should move back 3 files")

	ft.Move(MotionPageUp, 100)
	assert.Equal(t, "cmd/flags.go", ft.SelectedFile(), "pageUp past start should clamp at first file")

	ft.Move(MotionPageUp, 1)
	assert.Equal(t, "cmd/flags.go", ft.SelectedFile(), "pageUp at start should stay at first file")
}

func TestFileTree_PageDownAccountsForDirHeaders(t *testing.T) {
	ft := NewFileTree(fileEntries("a/x.go", "a/y.go", "b/x.go", "b/y.go", "c/x.go", "c/y.go"))
	assert.Equal(t, "a/x.go", ft.SelectedFile())

	ft.Move(MotionPageDown, 3)
	assert.Equal(t, "b/x.go", ft.SelectedFile(), "pageDown should account for directory header rows")

	ft.Move(MotionPageDown, 3)
	assert.Equal(t, "c/x.go", ft.SelectedFile(), "pageDown across directory should account for dir header")
}

func TestFileTree_MoveToFirstLast(t *testing.T) {
	ft := NewFileTree(fileEntries("cmd/main.go", "internal/a.go", "internal/b.go", "internal/c.go"))

	assert.Equal(t, "cmd/main.go", ft.SelectedFile())

	ft.Move(MotionLast)
	assert.Equal(t, "internal/c.go", ft.SelectedFile(), "MotionLast should select last file")

	ft.Move(MotionFirst)
	assert.Equal(t, "cmd/main.go", ft.SelectedFile(), "MotionFirst should select first file")

	// idempotent
	ft.Move(MotionFirst)
	assert.Equal(t, "cmd/main.go", ft.SelectedFile())

	ft.Move(MotionLast)
	ft.Move(MotionLast)
	assert.Equal(t, "internal/c.go", ft.SelectedFile())
}

func TestFileTree_RenderIndentation(t *testing.T) {
	ft := NewFileTree(fileEntries("cmd/main.go", "internal/handler.go", "internal/store.go"))
	res := style.NewResolver(style.Colors{Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700", CursorBg: "#3a3a3a", AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100"})
	rnd := style.NewRenderer(res)

	for _, e := range ft.entries {
		if e.isDir {
			assert.Equal(t, 0, e.depth, "directory %q should have depth 0", e.name)
		} else {
			assert.Equal(t, 1, e.depth, "file %q should have depth 1", e.name)
		}
	}

	result := ft.Render(FileTreeRender{Width: 40, Height: 100, Resolver: res, Renderer: rnd})
	lines := strings.Split(result, "\n")
	assert.GreaterOrEqual(t, len(lines), 5, "expected at least 5 lines (2 dirs + 3 files)")

	for _, e := range ft.entries {
		if e.isDir {
			assert.Contains(t, result, " "+e.name, "directory %q should appear with single leading space", e.name)
		} else {
			assert.Contains(t, result, "  "+e.name, "file %q should be indented under its directory", e.name)
		}
	}
}

func TestFileTree_EnsureVisible(t *testing.T) {
	ft := NewFileTree(fileEntries(
		"a/1.go", "a/2.go", "a/3.go", "a/4.go", "a/5.go",
		"b/1.go", "b/2.go", "b/3.go", "b/4.go", "b/5.go",
	))

	ft.offset = 0
	ft.cursor = 1
	ft.EnsureVisible(4)
	assert.Equal(t, 0, ft.offset, "cursor within view, offset should stay 0")

	ft.cursor = 7
	ft.EnsureVisible(4)
	assert.Equal(t, 4, ft.offset, "offset should scroll so cursor is visible")

	ft.cursor = 2
	ft.EnsureVisible(4)
	assert.Equal(t, 2, ft.offset, "offset should scroll back for cursor")
}

func TestFileTree_RenderViewport(t *testing.T) {
	ft := NewFileTree(fileEntries(
		"a/1.go", "a/2.go", "a/3.go", "a/4.go", "a/5.go",
		"b/1.go", "b/2.go", "b/3.go",
	))
	res := style.NewResolver(style.Colors{Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700", CursorBg: "#3a3a3a", AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100"})
	rnd := style.NewRenderer(res)

	result := ft.Render(FileTreeRender{Width: 40, Height: 3, Resolver: res, Renderer: rnd})
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 3, "should render exactly 3 lines")

	ft.Move(MotionLast)
	result = ft.Render(FileTreeRender{Width: 40, Height: 3, Resolver: res, Renderer: rnd})
	lines = strings.Split(result, "\n")
	assert.Len(t, lines, 3, "should still render exactly 3 lines")
	assert.Contains(t, result, "3.go", "last file should be visible after scrolling")
}

func TestFileTree_EnsureVisibleResetsOffsetWhenTreeFitsViewport(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))

	ft.offset = 3
	ft.cursor = 3 // cursor at c.go

	ft.EnsureVisible(20)
	assert.Equal(t, 0, ft.offset, "offset should reset to 0 when all entries fit in viewport")
}

func TestFileTree_SelectByPath(t *testing.T) {
	t.Run("selects existing file", func(t *testing.T) {
		ft := NewFileTree(fileEntries("internal/handler.go", "internal/store.go", "main.go"))
		ok := ft.SelectByPath("internal/store.go")
		assert.True(t, ok)
		assert.Equal(t, "internal/store.go", ft.SelectedFile())
	})

	t.Run("returns false for non-existent file", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go"))
		ok := ft.SelectByPath("c.go")
		assert.False(t, ok)
	})

	t.Run("does not select directory entries", func(t *testing.T) {
		ft := NewFileTree(fileEntries("internal/a.go"))
		ok := ft.SelectByPath("internal/")
		assert.False(t, ok)
	})
}

func TestFileTree_SelectByVisibleRow(t *testing.T) {
	t.Run("first row at offset zero selects first entry", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go"))
		// entries: ["./", "a.go", "b.go"]
		ok := ft.SelectByVisibleRow(0)
		assert.True(t, ok)
		assert.Equal(t, 0, ft.cursor)
	})

	t.Run("row within visible range selects matching entry", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		// entries: ["./", "a.go", "b.go", "c.go"], offset=0
		ok := ft.SelectByVisibleRow(2)
		assert.True(t, ok)
		assert.Equal(t, 2, ft.cursor) // "b.go"
		assert.Equal(t, "b.go", ft.SelectedFile())
	})

	t.Run("row with non-zero offset adds offset", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go", "d.go", "e.go"))
		// entries: ["./", "a.go", "b.go", "c.go", "d.go", "e.go"]
		ft.offset = 3
		ok := ft.SelectByVisibleRow(1)
		assert.True(t, ok)
		assert.Equal(t, 4, ft.cursor) // offset(3) + row(1) = "d.go"
		assert.Equal(t, "d.go", ft.SelectedFile())
	})

	t.Run("click past end returns false and does not modify cursor", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go"))
		// entries: ["./", "a.go", "b.go"] — only 3 entries
		prev := ft.cursor
		ok := ft.SelectByVisibleRow(10)
		assert.False(t, ok)
		assert.Equal(t, prev, ft.cursor)
	})

	t.Run("click on directory row succeeds", func(t *testing.T) {
		ft := NewFileTree(fileEntries("internal/a.go"))
		// entries: ["internal/", "a.go"] — first is directory
		ok := ft.SelectByVisibleRow(0)
		assert.True(t, ok)
		assert.Equal(t, 0, ft.cursor)
		assert.True(t, ft.entries[ft.cursor].isDir)
		// SelectedFile returns empty for dir entries, mirroring j-landing behavior
		assert.Empty(t, ft.SelectedFile())
	})

	t.Run("negative row returns false and does not modify cursor", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go"))
		prev := ft.cursor
		ok := ft.SelectByVisibleRow(-1)
		assert.False(t, ok)
		assert.Equal(t, prev, ft.cursor)
	})

	t.Run("empty tree returns false for any row", func(t *testing.T) {
		ft := NewFileTree(nil)
		ok := ft.SelectByVisibleRow(0)
		assert.False(t, ok)
	})

	t.Run("row past end with offset returns false", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		ft.offset = 2
		prev := ft.cursor
		// entries has 4 items (./, a.go, b.go, c.go); offset=2, row=5 -> idx=7, out of range
		ok := ft.SelectByVisibleRow(5)
		assert.False(t, ok)
		assert.Equal(t, prev, ft.cursor)
	})
}

func TestFileTree_RenderTruncatesLongDirNames(t *testing.T) {
	ft := NewFileTree(fileEntries(".claude-plugin/skills/revdiff/references/config.md"))
	res := style.NewResolver(style.Colors{Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700", AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100"})
	rnd := style.NewRenderer(res)

	result := ft.Render(FileTreeRender{Width: 30, Height: 10, Resolver: res, Renderer: rnd})
	assert.Contains(t, result, "…", "long dir name should be truncated with ellipsis")
	assert.Contains(t, result, "references/", "truncated dir should show trailing path")
	assert.NotContains(t, result, ".claude-plugin/skills/revdiff/references/", "full dir name should not appear")
}

func TestFileTree_RenderTruncatesLongFileNames(t *testing.T) {
	ft := NewFileTree(fileEntries("docs/plans/completed/20260402-status-line-help-overlay.md"))
	res := style.PlainResolver()
	rnd := style.NewRenderer(res)

	result := ft.Render(FileTreeRender{Width: 30, Height: 10, Resolver: res, Renderer: rnd})
	lines := strings.Split(result, "\n")

	var fileLine string
	for _, l := range lines {
		if strings.Contains(l, ".md") {
			fileLine = l
			break
		}
	}
	require.NotEmpty(t, fileLine, "should find the .md file entry")
	assert.Contains(t, fileLine, "…", "long filename should be truncated with ellipsis")
	assert.LessOrEqual(t, lipgloss.Width(fileLine), 30, "file entry should not exceed pane width")
}

func TestFileTree_RenderViewportCursorAlwaysVisible(t *testing.T) {
	files := fileEntries(
		"cmd/main.go", "cmd/flags.go",
		"internal/a.go", "internal/b.go", "internal/c.go", "internal/d.go",
		"internal/e.go", "internal/f.go", "internal/g.go", "internal/h.go",
	)
	ft := NewFileTree(files)
	res := style.NewResolver(style.Colors{Accent: "#5f87ff", Border: "#585858", Normal: "#d0d0d0", Muted: "#6c6c6c", SelectedFg: "#ffffaf", SelectedBg: "#303030", Annotation: "#ffd700", CursorBg: "#3a3a3a", AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100"})
	rnd := style.NewRenderer(res)

	ft.Move(MotionPageDown, 100)
	assert.Equal(t, "internal/h.go", ft.SelectedFile())

	result := ft.Render(FileTreeRender{Width: 40, Height: 5, Resolver: res, Renderer: rnd})
	assert.Contains(t, result, "h.go", "cursor file must be visible after page down")

	ft.Move(MotionPageUp, 100)
	result = ft.Render(FileTreeRender{Width: 40, Height: 5, Resolver: res, Renderer: rnd})
	selected := ft.SelectedFile()
	assert.Contains(t, result, selected[strings.LastIndex(selected, "/")+1:],
		"cursor file must be visible after page up")
}

func TestFileTree_ToggleReviewed(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))

	assert.Equal(t, 0, ft.ReviewedCount())

	ft.ToggleReviewed("a.go")
	assert.True(t, ft.reviewed["a.go"])
	assert.Equal(t, 1, ft.ReviewedCount())

	ft.ToggleReviewed("b.go")
	assert.Equal(t, 2, ft.ReviewedCount())

	// toggle off
	ft.ToggleReviewed("a.go")
	assert.False(t, ft.reviewed["a.go"])
	assert.Equal(t, 1, ft.ReviewedCount())

	// no-op for empty path
	ft.ToggleReviewed("")
	assert.Equal(t, 1, ft.ReviewedCount())
}

func TestFileTree_RenderReviewedCheckmark(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go"))
	res := style.PlainResolver()
	rnd := style.NewRenderer(res)

	result := ft.Render(FileTreeRender{Width: 40, Height: 10, Resolver: res, Renderer: rnd})
	assert.NotContains(t, result, "✓")

	ft.ToggleReviewed("a.go")
	result = ft.Render(FileTreeRender{Width: 40, Height: 10, Resolver: res, Renderer: rnd})
	assert.Contains(t, result, "✓")

	ft.ToggleReviewed("b.go")
	annotated := map[string]bool{"b.go": true}
	result = ft.Render(FileTreeRender{Width: 40, Height: 10, Annotated: annotated, Resolver: res, Renderer: rnd})
	assert.Contains(t, result, "✓")
	assert.Contains(t, result, "*")
}

func TestFileTree_RenderFileEntryRestoresNormalForegroundAfterColoredPrefix(t *testing.T) {
	ft := NewFileTree([]diff.FileEntry{{Path: "a.go", Status: diff.FileAdded}})
	ft.cursor = 0 // keep the file unselected so the inline ANSI path is used
	ft.ToggleReviewed("a.go")

	res := style.NewResolver(style.Colors{Normal: "#d0d0d0", AddFg: "#87d787", Muted: "#6c6c6c"})
	rnd := style.NewRenderer(res)

	line := ft.renderFileEntry(ft.entries[1], 1, 40, renderCtx{annotatedFiles: nil, res: res, rnd: rnd})

	assert.Contains(t, line, "\033[38;2;135;215;135m✓\033[38;2;208;208;208m ")
	assert.Contains(t, line, "\033[38;2;135;215;135mA\033[38;2;208;208;208m a.go")
}

func TestFileTree_TruncateDirName(t *testing.T) {
	ft := &FileTree{}
	tests := []struct {
		name, input, expected string
		maxWidth              int
	}{
		{name: "no truncation needed", input: "short", maxWidth: 10, expected: "short"},
		{name: "exact fit", input: "exact", maxWidth: 5, expected: "exact"},
		{name: "zero width", input: "anything", maxWidth: 0, expected: "anything"},
		{name: "ascii truncation", input: "very/long/path/name", maxWidth: 10, expected: "…path/name"},
		{name: "cjk truncation", input: "目录/很长的路径/名称", maxWidth: 6, expected: "…/名称"},
		{name: "emoji truncation", input: "📁folder/📂sub/file", maxWidth: 10, expected: "…sub/file"},
		{name: "mixed ascii and cjk", input: "abc/目录/xyz", maxWidth: 7, expected: "…录/xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ft.truncateDirName(tt.input, tt.maxWidth))
		})
	}
}

func TestFileTreeRebuild(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
	ft.ToggleReviewed("a.go")
	ft.ToggleReviewed("b.go")
	ft.Move(MotionLast)
	assert.Equal(t, "c.go", ft.SelectedFile())
	assert.Equal(t, 2, ft.ReviewedCount())

	t.Run("reviewed map preserved for files still present", func(t *testing.T) {
		ft.Rebuild(fileEntries("a.go", "b.go", "d.go"))
		assert.True(t, ft.reviewed["a.go"], "a.go was reviewed and still present")
		assert.True(t, ft.reviewed["b.go"], "b.go was reviewed and still present")
		assert.Equal(t, 2, ft.ReviewedCount())
	})

	t.Run("reviewed map pruned for removed files", func(t *testing.T) {
		ft.ToggleReviewed("d.go")
		assert.Equal(t, 3, ft.ReviewedCount())
		ft.Rebuild(fileEntries("a.go", "e.go"))
		assert.True(t, ft.reviewed["a.go"], "a.go still present")
		assert.False(t, ft.reviewed["b.go"], "b.go was removed")
		assert.False(t, ft.reviewed["d.go"], "d.go was removed")
		assert.Equal(t, 1, ft.ReviewedCount())
	})

	t.Run("cursor resets to first file entry", func(t *testing.T) {
		ft.Rebuild(fileEntries("x.go", "y.go", "z.go"))
		assert.Equal(t, "x.go", ft.SelectedFile())
		assert.Equal(t, 0, ft.offset)
	})

	t.Run("fileStatuses refreshed from new entries", func(t *testing.T) {
		ft.Rebuild([]diff.FileEntry{{Path: "m.go", Status: diff.FileModified}, {Path: "n.go", Status: diff.FileAdded}})
		assert.Equal(t, diff.FileModified, ft.FileStatus("m.go"))
		assert.Equal(t, diff.FileAdded, ft.FileStatus("n.go"))
		assert.Equal(t, diff.FileStatus(""), ft.FileStatus("x.go"), "old statuses should be cleared")
	})

	t.Run("filter state preserved", func(t *testing.T) {
		ft2 := NewFileTree(fileEntries("a.go", "b.go"))
		ft2.ToggleFilter(map[string]bool{"a.go": true})
		assert.True(t, ft2.FilterActive())
		ft2.Rebuild(fileEntries("a.go", "c.go"))
		assert.True(t, ft2.FilterActive(), "filter state should be preserved across rebuild")
	})
}

func TestFileTree_ScrollState(t *testing.T) {
	t.Run("empty tree reports zero total and zero offset", func(t *testing.T) {
		ft := NewFileTree(nil)
		s := ft.ScrollState()
		assert.Equal(t, 0, s.Total)
		assert.Equal(t, 0, s.Offset)
	})

	t.Run("fresh tree starts at offset zero", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		s := ft.ScrollState()
		assert.Equal(t, len(ft.entries), s.Total, "total counts all entries (dirs + files)")
		assert.Equal(t, 0, s.Offset)
	})

	t.Run("offset updates after EnsureVisible when cursor moves past viewport", func(t *testing.T) {
		paths := make([]string, 50)
		for i := range paths {
			paths[i] = fmt.Sprintf("pkg/file-%02d.go", i)
		}
		ft := NewFileTree(fileEntries(paths...))
		ft.Move(MotionLast)
		// offset is stale until EnsureVisible runs
		assert.Equal(t, 0, ft.ScrollState().Offset, "offset is stale before EnsureVisible")

		ft.EnsureVisible(10)
		s := ft.ScrollState()
		assert.Equal(t, len(ft.entries), s.Total)
		assert.Positive(t, s.Offset, "EnsureVisible scrolls to keep cursor visible")
		assert.LessOrEqual(t, s.Offset, len(ft.entries)-10)
	})

	t.Run("offset reflects post-Render state via the Render path", func(t *testing.T) {
		paths := make([]string, 50)
		for i := range paths {
			paths[i] = fmt.Sprintf("pkg/file-%02d.go", i)
		}
		ft := NewFileTree(fileEntries(paths...))
		ft.Move(MotionLast)

		res := style.NewResolver(style.Colors{Normal: "#d0d0d0", Muted: "#6c6c6c"})
		rnd := style.NewRenderer(res)
		_ = ft.Render(FileTreeRender{Width: 30, Height: 10, Resolver: res, Renderer: rnd})

		s := ft.ScrollState()
		assert.Positive(t, s.Offset, "Render calls EnsureVisible which moves offset")
	})
}

func TestFileTreeRebuildThenSelectByPath(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))

	t.Run("select existing file after rebuild", func(t *testing.T) {
		ft.Rebuild(fileEntries("x.go", "y.go", "z.go"))
		ok := ft.SelectByPath("y.go")
		assert.True(t, ok)
		assert.Equal(t, "y.go", ft.SelectedFile())
	})

	t.Run("select deleted file stays on first file", func(t *testing.T) {
		ft.Rebuild(fileEntries("p.go", "q.go"))
		ok := ft.SelectByPath("y.go")
		assert.False(t, ok)
		assert.Equal(t, "p.go", ft.SelectedFile())
	})
}

func TestFileTreeMove_Exhaustive(t *testing.T) {
	ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
	for _, m := range MotionValues {
		t.Run("no count/"+m.String(), func(t *testing.T) {
			assert.NotPanics(t, func() { ft.Move(m) })
		})
		t.Run("with count/"+m.String(), func(t *testing.T) {
			assert.NotPanics(t, func() { ft.Move(m, 5) })
		})
	}
}

func TestFileTreeMove_VariadicCount(t *testing.T) {
	t.Run("page down with no count defaults to 1", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		ft.Move(MotionPageDown)
		assert.Equal(t, "b.go", ft.SelectedFile())
	})

	t.Run("page down with explicit count", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		ft.Move(MotionPageDown, 5)
		assert.Equal(t, "c.go", ft.SelectedFile())
	})

	t.Run("count ignored for non-page motions", func(t *testing.T) {
		ft := NewFileTree(fileEntries("a.go", "b.go", "c.go"))
		ft.Move(MotionDown, 99)
		assert.Equal(t, "b.go", ft.SelectedFile(), "count should be ignored for MotionDown")
	})
}

func TestNewFileTree_Nil(t *testing.T) {
	ft := NewFileTree(nil)
	require.NotNil(t, ft, "NewFileTree(nil) should return a valid empty *FileTree")
	assert.Equal(t, 0, ft.TotalFiles())
	assert.Empty(t, ft.SelectedFile())
	assert.Equal(t, 0, ft.ReviewedCount())
	assert.False(t, ft.FilterActive())
	assert.False(t, ft.HasFile(DirectionNext))
	assert.False(t, ft.HasFile(DirectionPrev))

	// should not panic on operations
	assert.NotPanics(t, func() { ft.Move(MotionDown) })
	assert.NotPanics(t, func() { ft.StepFile(DirectionNext) })
	assert.NotPanics(t, func() { ft.EnsureVisible(10) })
	assert.NotPanics(t, func() { ft.ToggleReviewed("x.go") })
}
