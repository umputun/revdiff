package ui

import (
	"path/filepath"
	"sort"
	"strings"
)

// fileTree manages the list of changed files grouped by directory.
type fileTree struct {
	entries  []treeEntry // flat list of directories and files for display
	cursor   int         // currently highlighted entry index
	offset   int         // first visible entry index for viewport scrolling
	allFiles []string    // original full file paths
	filter   bool        // when true, show only annotated files
}

// treeEntry represents a single line in the file tree display.
type treeEntry struct {
	name  string // display name (directory name or file basename)
	path  string // full file path (empty for directory entries)
	isDir bool
	depth int // indentation level
}

// newFileTree builds a file tree from a list of changed file paths.
func newFileTree(files []string) fileTree {
	ft := fileTree{allFiles: files}
	ft.entries = ft.buildEntries(files)
	// position cursor on first file entry
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			break
		}
	}
	return ft
}

// buildEntries groups files by directory and creates a flat entry list.
func (ft *fileTree) buildEntries(files []string) []treeEntry {
	if len(files) == 0 {
		return nil
	}

	// group files by directory
	dirFiles := make(map[string][]string)
	var dirs []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if _, ok := dirFiles[dir]; !ok {
			dirs = append(dirs, dir)
		}
		dirFiles[dir] = append(dirFiles[dir], f)
	}
	sort.Strings(dirs)

	var entries []treeEntry
	for _, dir := range dirs {
		// add directory entry
		dirName := dir
		if dirName == "." {
			dirName = "./"
		} else {
			dirName = dir + "/"
		}
		entries = append(entries, treeEntry{name: dirName, isDir: true, depth: 0})

		// add file entries under this directory, sorted
		dirFileList := dirFiles[dir]
		sort.Strings(dirFileList)
		for _, f := range dirFileList {
			entries = append(entries, treeEntry{
				name:  filepath.Base(f),
				path:  f,
				isDir: false,
				depth: 1,
			})
		}
	}
	return entries
}

// selectedFile returns the full path of the currently selected file, or empty string if a directory is selected.
func (ft *fileTree) selectedFile() string {
	if ft.cursor < 0 || ft.cursor >= len(ft.entries) {
		return ""
	}
	return ft.entries[ft.cursor].path
}

// ensureVisible adjusts offset so the cursor is within the visible range of given height.
func (ft *fileTree) ensureVisible(height int) {
	if height <= 0 {
		return
	}
	if ft.cursor < ft.offset {
		ft.offset = ft.cursor
	} else if ft.cursor >= ft.offset+height {
		ft.offset = ft.cursor - height + 1
	}
	if ft.offset < 0 {
		ft.offset = 0
	}
	if maxOff := max(len(ft.entries)-height, 0); ft.offset > maxOff {
		ft.offset = maxOff
	}
}

// moveDown moves cursor to the next file entry (skips directories).
func (ft *fileTree) moveDown() {
	for i := ft.cursor + 1; i < len(ft.entries); i++ {
		if !ft.entries[i].isDir {
			ft.cursor = i
			return
		}
	}
}

// moveUp moves cursor to the previous file entry (skips directories).
func (ft *fileTree) moveUp() {
	for i := ft.cursor - 1; i >= 0; i-- {
		if !ft.entries[i].isDir {
			ft.cursor = i
			return
		}
	}
}

// pageDown moves cursor down by approximately n visual rows,
// accounting for directory header rows that occupy rendered space.
func (ft *fileTree) pageDown(n int) {
	rowsMoved := 0
	for rowsMoved < n {
		prev := ft.cursor
		ft.moveDown()
		if ft.cursor == prev {
			break
		}
		rowsMoved += ft.cursor - prev // counts skipped directory entries too
	}
}

// pageUp moves cursor up by approximately n visual rows,
// accounting for directory header rows that occupy rendered space.
func (ft *fileTree) pageUp(n int) {
	rowsMoved := 0
	for rowsMoved < n {
		prev := ft.cursor
		ft.moveUp()
		if ft.cursor == prev {
			break
		}
		rowsMoved += prev - ft.cursor // counts skipped directory entries too
	}
}

// moveToFirst moves cursor to the first file entry.
func (ft *fileTree) moveToFirst() {
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			return
		}
	}
}

// moveToLast moves cursor to the last file entry.
func (ft *fileTree) moveToLast() {
	for i := len(ft.entries) - 1; i >= 0; i-- {
		if !ft.entries[i].isDir {
			ft.cursor = i
			return
		}
	}
}

// nextFile moves to the next file entry, wrapping around.
func (ft *fileTree) nextFile() {
	files := ft.fileIndices()
	if len(files) == 0 {
		return
	}
	for _, idx := range files {
		if idx > ft.cursor {
			ft.cursor = idx
			return
		}
	}
	ft.cursor = files[0] // wrap around
}

// prevFile moves to the previous file entry, wrapping around.
func (ft *fileTree) prevFile() {
	files := ft.fileIndices()
	if len(files) == 0 {
		return
	}
	for i := len(files) - 1; i >= 0; i-- {
		if files[i] < ft.cursor {
			ft.cursor = files[i]
			return
		}
	}
	ft.cursor = files[len(files)-1] // wrap around
}

// fileIndices returns indices of all file (non-directory) entries.
func (ft *fileTree) fileIndices() []int {
	var indices []int
	for i, e := range ft.entries {
		if !e.isDir {
			indices = append(indices, i)
		}
	}
	return indices
}

// render produces the file tree display string, showing only entries visible within the given height.
// it adjusts the internal offset so the cursor stays within the visible window.
func (ft *fileTree) render(width, height int, annotatedFiles map[string]bool, s styles) string {
	if len(ft.entries) == 0 {
		return "  no changed files"
	}

	ft.ensureVisible(height)
	end := min(ft.offset+height, len(ft.entries))

	var b strings.Builder
	for idx := ft.offset; idx < end; idx++ {
		e := ft.entries[idx]
		indent := strings.Repeat("  ", e.depth)
		var line string

		if e.isDir {
			line = s.DirEntry.Render(indent + "· " + e.name)
		} else {
			marker := "  "
			if annotatedFiles[e.path] {
				marker = s.AnnotationMark.Render(" *")
			}
			name := indent + e.name + marker

			if idx == ft.cursor {
				line = s.FileSelected.Width(width - 2).Render(name)
			} else {
				line = s.FileEntry.Render(name)
			}
		}

		b.WriteString(line)
		if idx < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// toggleFilter switches between showing all files and only annotated files.
func (ft *fileTree) toggleFilter(annotatedFiles map[string]bool) {
	ft.filter = !ft.filter
	if ft.filter {
		// show only annotated files
		var filtered []string
		for _, f := range ft.allFiles {
			if annotatedFiles[f] {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			ft.filter = false // nothing to filter, stay on all
			return
		}
		ft.entries = ft.buildEntries(filtered)
	} else {
		ft.entries = ft.buildEntries(ft.allFiles)
	}

	// position cursor on first file
	ft.cursor = 0
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			return
		}
	}
}

// refreshFilter rebuilds the filtered tree if the filter is active, preserving cursor position.
func (ft *fileTree) refreshFilter(annotatedFiles map[string]bool) {
	if !ft.filter {
		return
	}

	// capture selected file before rebuilding entries
	prevFile := ft.selectedFile()

	var filtered []string
	for _, f := range ft.allFiles {
		if annotatedFiles[f] {
			filtered = append(filtered, f)
		}
	}
	if len(filtered) == 0 {
		// no annotated files left, switch back to all files
		ft.filter = false
		ft.entries = ft.buildEntries(ft.allFiles)
	} else {
		ft.entries = ft.buildEntries(filtered)
	}

	// try to keep cursor on same file, otherwise position on first file
	ft.cursor = 0
	if prevFile != "" {
		for i, e := range ft.entries {
			if e.path == prevFile {
				ft.cursor = i
				return
			}
		}
	}
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			return
		}
	}
}
