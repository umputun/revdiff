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

	result := ft.render(30, map[string]bool{"internal/handler.go": true}, s)
	assert.Contains(t, result, "internal/")
	assert.Contains(t, result, "handler.go")
	assert.Contains(t, result, "main.go")
}

func TestFileTree_RenderEmpty(t *testing.T) {
	ft := newFileTree(nil)
	s := defaultStyles()
	result := ft.render(30, nil, s)
	assert.Contains(t, result, "no changed files")
}

func TestFileTree_SetFiles(t *testing.T) {
	ft := newFileTree([]string{"a.go", "b.go"})
	ft.moveDown() // move to b.go

	ft.setFiles([]string{"b.go", "c.go", "d.go"})
	// should restore cursor to b.go
	assert.Equal(t, "b.go", ft.selectedFile())
}

func TestFileTree_SetFilesNewList(t *testing.T) {
	ft := newFileTree([]string{"a.go"})

	ft.setFiles([]string{"x.go", "y.go"})
	// previous file gone, should position on first file
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

	result := ft.render(40, nil, s)
	lines := strings.Split(result, "\n")
	assert.GreaterOrEqual(t, len(lines), 5, "expected at least 5 lines (2 dirs + 3 files)")

	// directory lines should not have leading spaces, file lines should be indented
	for _, e := range ft.entries {
		if e.isDir {
			// directory entry starts with the dir icon, no leading spaces
			assert.Contains(t, result, "▾ "+e.name, "directory %q should appear without indent", e.name)
		} else {
			// file entries should have leading spaces (indentation from depth=1)
			assert.Contains(t, result, "  "+e.name, "file %q should be indented under its directory", e.name)
		}
	}
}
