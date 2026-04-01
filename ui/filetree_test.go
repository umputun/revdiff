package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileTree_BuildEntries(t *testing.T) {
	files := []string{"internal/handler.go", "internal/store.go", "main.go"}
	ft := newFileTree(files)

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
	ft := newFileTree(nil)
	assert.Empty(t, ft.entries)
}

func TestFileTree_SelectedFile(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go"})

	// cursor starts on first file
	assert.Equal(t, "a.go", ft.selectedFile())

	ft.moveDown()
	assert.Equal(t, "b.go", ft.selectedFile())
}

func TestFileTree_MoveDownUp(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go", "c.go"})

	// cursor starts on first file (a.go)
	assert.Equal(t, "a.go", ft.selectedFile())

	ft.moveDown()
	assert.Equal(t, "b.go", ft.selectedFile())

	ft.moveDown()
	assert.Equal(t, "c.go", ft.selectedFile())

	// at the end, moveDown does nothing
	ft.moveDown()
	assert.Equal(t, "c.go", ft.selectedFile())

	ft.moveUp()
	assert.Equal(t, "b.go", ft.selectedFile())

	ft.moveUp()
	assert.Equal(t, "a.go", ft.selectedFile())

	// at the top, moveUp does nothing
	ft.moveUp()
	assert.Equal(t, "a.go", ft.selectedFile())
}

func TestFileTree_NextPrevFile(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go", "c.go"})
	// cursor starts on first file (a.go)

	ft.nextFile()
	assert.Equal(t, "b.go", ft.selectedFile())

	ft.nextFile()
	assert.Equal(t, "c.go", ft.selectedFile())

	// wraps around
	ft.nextFile()
	assert.Equal(t, "a.go", ft.selectedFile())

	ft.prevFile()
	assert.Equal(t, "c.go", ft.selectedFile())

	ft.prevFile()
	assert.Equal(t, "b.go", ft.selectedFile())

	ft.prevFile()
	assert.Equal(t, "a.go", ft.selectedFile())
}

func TestFileTree_ToggleFilter(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go", "c.go"})
	annotated := map[string]bool{"a.go": true, "c.go": true}

	ft.toggleFilter(annotated)
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
	ft.toggleFilter(annotated)
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
	ft := newFileTree([]string{"a.go", "b.go"})
	annotated := map[string]bool{}

	ft.toggleFilter(annotated)
	// should stay on all files since no annotated files
	assert.False(t, ft.filter)
}

func TestFileTree_Render(t *testing.T) {
	ft := newFileTree([]string{"internal/handler.go", "main.go"})
	ft.cursor = 1 // select handler.go
	s := defaultStyles()

	result := ft.render(30, 100, map[string]bool{"internal/handler.go": true}, s)
	assert.Contains(t, result, "internal/")
	assert.Contains(t, result, "handler.go")
	assert.Contains(t, result, "main.go")
}

func TestFileTree_RenderEmpty(t *testing.T) {
	ft := newFileTree(nil)
	s := defaultStyles()
	result := ft.render(30, 100, nil, s)
	assert.Contains(t, result, "no changed files")
}

func TestFileTree_SetFiles(t *testing.T) {
	ft := newFileTree([]string{"b.go", "c.go", "d.go"})
	// first file should be selected by default
	assert.Equal(t, "b.go", ft.selectedFile())
}

func TestFileTree_SetFilesNewList(t *testing.T) {
	ft := newFileTree([]string{"x.go", "y.go"})
	// should position on first file
	assert.Equal(t, "x.go", ft.selectedFile())
}

func TestFileTree_DirectoryGrouping(t *testing.T) {
	files := []string{"cmd/main.go", "internal/handler.go", "internal/store.go"}
	ft := newFileTree(files)

	// should have: cmd/ dir, main.go, internal/ dir, handler.go, store.go
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
	ft := newFileTree([]string{"a.go", "b.go"})
	indices := ft.fileIndices()
	// dir at 0, files at 1 and 2
	assert.Equal(t, []int{1, 2}, indices)
}

func TestFileTree_RefreshFilter(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go", "c.go"})

	// enable filter with a.go annotated
	ft.toggleFilter(map[string]bool{"a.go": true})
	assert.True(t, ft.filter)

	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 1, fileCount)

	// refresh with a.go and c.go annotated - c.go should now appear
	ft.refreshFilter(map[string]bool{"a.go": true, "c.go": true})
	fileCount = 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount)
}

func TestFileTree_RefreshFilterNoAnnotations(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go"})
	ft.toggleFilter(map[string]bool{"a.go": true})
	assert.True(t, ft.filter)

	// refresh with no annotations - should disable filter
	ft.refreshFilter(map[string]bool{})
	assert.False(t, ft.filter)

	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount)
}

func TestFileTree_RefreshFilterPreservesCursor(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go", "c.go"})
	ft.toggleFilter(map[string]bool{"a.go": true, "b.go": true})
	assert.True(t, ft.filter)

	// move to b.go
	ft.moveDown()
	assert.Equal(t, "b.go", ft.selectedFile())

	// refresh filter - cursor should remain on b.go
	ft.refreshFilter(map[string]bool{"a.go": true, "b.go": true})
	assert.Equal(t, "b.go", ft.selectedFile())
}

func TestFileTree_RefreshFilterNotActive(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go"})
	assert.False(t, ft.filter)

	// refresh when filter is not active should be a no-op
	ft.refreshFilter(map[string]bool{"a.go": true})
	assert.False(t, ft.filter)

	fileCount := 0
	for _, e := range ft.entries {
		if !e.isDir {
			fileCount++
		}
	}
	assert.Equal(t, 2, fileCount, "all files should remain visible")
}

func TestFileTree_PageDown(t *testing.T) {
	// create tree with many files across directories (sorted: flags.go, main.go, a-h.go)
	files := []string{
		"cmd/main.go", "cmd/flags.go",
		"internal/a.go", "internal/b.go", "internal/c.go", "internal/d.go",
		"internal/e.go", "internal/f.go", "internal/g.go", "internal/h.go",
	}
	ft := newFileTree(files)
	assert.Equal(t, "cmd/flags.go", ft.selectedFile(), "cursor should start on first file (flags.go)")

	// page down by 3 should advance ~3 visual rows: flags->main->(dir header)->a
	ft.pageDown(3)
	assert.Equal(t, "internal/a.go", ft.selectedFile(), "pageDown(3) should advance ~3 visual rows")

	// page down by large number should clamp at last file
	ft.pageDown(100)
	assert.Equal(t, "internal/h.go", ft.selectedFile(), "pageDown past end should clamp at last file")

	// page down at end should stay at last file
	ft.pageDown(1)
	assert.Equal(t, "internal/h.go", ft.selectedFile(), "pageDown at end should stay at last file")
}

func TestFileTree_PageUp(t *testing.T) {
	files := []string{
		"cmd/main.go", "cmd/flags.go",
		"internal/a.go", "internal/b.go", "internal/c.go", "internal/d.go",
		"internal/e.go", "internal/f.go", "internal/g.go", "internal/h.go",
	}
	ft := newFileTree(files)

	// move to last file first
	ft.moveToLast()
	assert.Equal(t, "internal/h.go", ft.selectedFile())

	// page up by 3 should move back 3 file entries
	ft.pageUp(3)
	assert.Equal(t, "internal/e.go", ft.selectedFile(), "pageUp(3) should move back 3 files")

	// page up by large number should clamp at first file
	ft.pageUp(100)
	assert.Equal(t, "cmd/flags.go", ft.selectedFile(), "pageUp past start should clamp at first file")

	// page up at start should stay at first file
	ft.pageUp(1)
	assert.Equal(t, "cmd/flags.go", ft.selectedFile(), "pageUp at start should stay at first file")
}

func TestFileTree_PageDownAccountsForDirHeaders(t *testing.T) {
	// 3 directories with 2 files each: entries include 3 dir headers
	files := []string{"a/x.go", "a/y.go", "b/x.go", "b/y.go", "c/x.go", "c/y.go"}
	ft := newFileTree(files)
	// entries: a/ (dir,0), x.go(1), y.go(2), b/ (dir,3), x.go(4), y.go(5), c/ (dir,6), x.go(7), y.go(8)
	assert.Equal(t, "a/x.go", ft.selectedFile())

	// pageDown(3) counts visual rows: x->y (1 row), y->(b/ dir)->x (2 rows) = 3 total
	ft.pageDown(3)
	assert.Equal(t, "b/x.go", ft.selectedFile(), "pageDown should account for directory header rows")

	// pageDown(3) from b/x.go: x->y (1 row), y->(c/ dir)->x (2 rows) = 3 total
	ft.pageDown(3)
	assert.Equal(t, "c/x.go", ft.selectedFile(), "pageDown across directory should account for dir header")
}

func TestFileTree_MoveToFirstLast(t *testing.T) {
	files := []string{"cmd/main.go", "internal/a.go", "internal/b.go", "internal/c.go"}
	ft := newFileTree(files)

	// starts on first file
	assert.Equal(t, "cmd/main.go", ft.selectedFile())

	// moveToLast should go to last file
	ft.moveToLast()
	assert.Equal(t, "internal/c.go", ft.selectedFile(), "moveToLast should select last file")

	// moveToFirst should go back to first file
	ft.moveToFirst()
	assert.Equal(t, "cmd/main.go", ft.selectedFile(), "moveToFirst should select first file")

	// moveToFirst when already at first should be idempotent
	ft.moveToFirst()
	assert.Equal(t, "cmd/main.go", ft.selectedFile())

	// moveToLast when already at last should be idempotent
	ft.moveToLast()
	ft.moveToLast()
	assert.Equal(t, "internal/c.go", ft.selectedFile())
}

func TestFileTree_RenderIndentation(t *testing.T) {
	ft := newFileTree([]string{"cmd/main.go", "internal/handler.go", "internal/store.go"})
	s := defaultStyles()

	// verify entry depth: directories at depth 0, files at depth 1
	for _, e := range ft.entries {
		if e.isDir {
			assert.Equal(t, 0, e.depth, "directory %q should have depth 0", e.name)
		} else {
			assert.Equal(t, 1, e.depth, "file %q should have depth 1", e.name)
		}
	}

	result := ft.render(40, 100, nil, s)
	lines := strings.Split(result, "\n")
	assert.GreaterOrEqual(t, len(lines), 5, "expected at least 5 lines (2 dirs + 3 files)")

	// directory lines should not have leading spaces, file lines should be indented
	for _, e := range ft.entries {
		if e.isDir {
			// directory entry starts with the dir icon, no leading spaces
			assert.Contains(t, result, "· "+e.name, "directory %q should appear without indent", e.name)
		} else {
			// file entries should have leading spaces (indentation from depth=1)
			assert.Contains(t, result, "  "+e.name, "file %q should be indented under its directory", e.name)
		}
	}
}

func TestFileTree_EnsureVisible(t *testing.T) {
	ft := newFileTree([]string{
		"a/1.go", "a/2.go", "a/3.go", "a/4.go", "a/5.go",
		"b/1.go", "b/2.go", "b/3.go", "b/4.go", "b/5.go",
	})
	// entries: a/(0), 1.go(1), 2.go(2), 3.go(3), 4.go(4), 5.go(5), b/(6), 1.go(7)...

	// visible height of 4, cursor at start
	ft.offset = 0
	ft.cursor = 1
	ft.ensureVisible(4)
	assert.Equal(t, 0, ft.offset, "cursor within view, offset should stay 0")

	// move cursor past visible range
	ft.cursor = 7
	ft.ensureVisible(4)
	assert.Equal(t, 4, ft.offset, "offset should scroll so cursor is visible")

	// scroll back up
	ft.cursor = 2
	ft.ensureVisible(4)
	assert.Equal(t, 2, ft.offset, "offset should scroll back for cursor")
}

func TestFileTree_RenderViewport(t *testing.T) {
	ft := newFileTree([]string{
		"a/1.go", "a/2.go", "a/3.go", "a/4.go", "a/5.go",
		"b/1.go", "b/2.go", "b/3.go",
	})
	s := defaultStyles()

	// render with height=3, cursor on first file
	result := ft.render(40, 3, nil, s)
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 3, "should render exactly 3 lines")

	// move cursor to last file and render again
	ft.moveToLast()
	result = ft.render(40, 3, nil, s)
	lines = strings.Split(result, "\n")
	assert.Len(t, lines, 3, "should still render exactly 3 lines")
	assert.Contains(t, result, "3.go", "last file should be visible after scrolling")
}

func TestFileTree_EnsureVisibleResetsOffsetWhenTreeFitsViewport(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go", "c.go"})
	// entries: ./(dir,0), a.go(1), b.go(2), c.go(3) — 4 entries total

	// simulate a stale offset from when the tree had more entries or a smaller viewport
	ft.offset = 3
	ft.cursor = 3 // cursor at c.go, matching offset so cursor-based clamp won't trigger

	// viewport height is larger than entry count — everything fits
	ft.ensureVisible(20)
	assert.Equal(t, 0, ft.offset, "offset should reset to 0 when all entries fit in viewport")
}

func TestFileTree_RenderViewportCursorAlwaysVisible(t *testing.T) {
	files := []string{
		"cmd/main.go", "cmd/flags.go",
		"internal/a.go", "internal/b.go", "internal/c.go", "internal/d.go",
		"internal/e.go", "internal/f.go", "internal/g.go", "internal/h.go",
	}
	ft := newFileTree(files)
	s := defaultStyles()

	// page down past visible area with small height
	ft.pageDown(100)
	assert.Equal(t, "internal/h.go", ft.selectedFile())

	result := ft.render(40, 5, nil, s)
	assert.Contains(t, result, "h.go", "cursor file must be visible after page down")

	// page back up
	ft.pageUp(100)
	result = ft.render(40, 5, nil, s)
	// selectedFile() returns full path ("cmd/flags.go"), but render shows basename ("flags.go")
	selected := ft.selectedFile()
	assert.Contains(t, result, selected[strings.LastIndex(selected, "/")+1:],
		"cursor file must be visible after page up")
}
