package sidepane

import (
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// FileTree manages the list of changed files grouped by directory.
type FileTree struct {
	entries      []treeEntry                // flat list of directories and files for display
	cursor       int                        // currently highlighted entry index
	offset       int                        // first visible entry index for viewport scrolling
	allFiles     []string                   // original full file paths
	filter       bool                       // when true, show only annotated files
	unreviewed   bool                       // when true, show only files not marked reviewed
	reviewed     map[string]string          // semantic diff fingerprint for files marked reviewed
	fileStatuses map[string]diff.FileStatus // file change status from git, empty for non-git
	oldPaths     map[string]string          // rename origin keyed by new path, empty for non-renames
}

// treeEntry represents a single line in the file tree display.
type treeEntry struct {
	name  string // display name (directory name or file basename)
	path  string // full file path (empty for directory entries)
	isDir bool
	depth int // indentation level
}

// renderCtx holds rendering context for a file tree entry,
// reducing the parameter count of renderFileEntry.
type renderCtx struct {
	annotatedFiles map[string]bool
	res            Resolver
	rnd            Renderer
}

// NewFileTree builds a FileTree from a list of changed file entries.
// handles entries == nil gracefully, returning a valid empty *FileTree.
func NewFileTree(entries []diff.FileEntry) *FileTree {
	paths := diff.FileEntryPaths(entries)
	ft := &FileTree{
		allFiles:     paths,
		reviewed:     make(map[string]string),
		fileStatuses: make(map[string]diff.FileStatus),
		oldPaths:     make(map[string]string),
	}
	ft.entries = ft.buildEntries(paths)

	// store file statuses and rename origins from entries
	for _, e := range entries {
		if e.Status != "" {
			ft.fileStatuses[e.Path] = e.Status
		}
		if e.OldPath != "" {
			ft.oldPaths[e.Path] = e.OldPath
		}
	}

	// position cursor on first file entry
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			break
		}
	}
	return ft
}

// SelectedFile returns the full path of the currently selected file,
// or empty string if a directory is selected or entries are empty.
func (ft *FileTree) SelectedFile() string {
	if ft.cursor < 0 || ft.cursor >= len(ft.entries) {
		return ""
	}
	return ft.entries[ft.cursor].path
}

// TotalFiles returns the count of original file paths (before filtering).
func (ft *FileTree) TotalFiles() int {
	return len(ft.allFiles)
}

// FileStatus returns the git change status for the given file path.
func (ft *FileTree) FileStatus(path string) diff.FileStatus {
	return ft.fileStatuses[path]
}

// OldPath returns the rename origin for the given file path, or empty when the
// file is not a rename or the path is unknown.
func (ft *FileTree) OldPath(path string) string {
	return ft.oldPaths[path]
}

// FilterActive returns true when the file tree is showing only annotated files.
func (ft *FileTree) FilterActive() bool {
	return ft.filter
}

// UnreviewedFilterActive returns true when the file tree is showing only
// files that have not been marked reviewed.
func (ft *FileTree) UnreviewedFilterActive() bool {
	return ft.unreviewed
}

// ReviewedCount returns the number of files marked as reviewed.
func (ft *FileTree) ReviewedCount() int {
	return len(ft.reviewed)
}

// ReviewedFingerprints returns a copy of the reviewed path-to-fingerprint map.
func (ft *FileTree) ReviewedFingerprints() map[string]string {
	return maps.Clone(ft.reviewed)
}

// IsReviewed reports whether path is currently marked reviewed.
func (ft *FileTree) IsReviewed(path string) bool {
	_, ok := ft.reviewed[path]
	return ok
}

// HasFile returns true if there is a file entry in the given direction
// from the current cursor position (no wrap-around).
func (ft *FileTree) HasFile(dir Direction) bool {
	switch dir {
	case DirectionNext:
		for i := ft.cursor + 1; i < len(ft.entries); i++ {
			if !ft.entries[i].isDir {
				return true
			}
		}
	case DirectionPrev:
		for i := ft.cursor - 1; i >= 0; i-- {
			if !ft.entries[i].isDir {
				return true
			}
		}
	}
	return false
}

// Move navigates the cursor according to the given motion.
// count is variadic: page motions use count[0] for the page size,
// non-page motions ignore count entirely. Missing count for page motions
// defaults to 1 (single step), which is harmless.
func (ft *FileTree) Move(m Motion, count ...int) {
	switch m {
	case MotionUp:
		ft.moveUp()
	case MotionDown:
		ft.moveDown()
	case MotionPageUp:
		n := 1
		if len(count) > 0 {
			n = count[0]
		}
		ft.pageUp(n)
	case MotionPageDown:
		n := 1
		if len(count) > 0 {
			n = count[0]
		}
		ft.pageDown(n)
	case MotionFirst:
		ft.moveToFirst()
	case MotionLast:
		ft.moveToLast()
	}
}

// StepFile moves to the next or previous file entry, wrapping around at ends.
func (ft *FileTree) StepFile(dir Direction) {
	switch dir {
	case DirectionNext:
		ft.nextFile()
	case DirectionPrev:
		ft.prevFile()
	}
}

// SelectByPath sets the cursor to the file entry matching the given path.
// returns true if the file was found and cursor moved, false otherwise.
func (ft *FileTree) SelectByPath(path string) bool {
	for i, e := range ft.entries {
		if !e.isDir && e.path == path {
			ft.cursor = i
			return true
		}
	}
	return false
}

// SelectByVisibleRow sets the cursor to the entry at the given visible row.
// row is 0-based relative to the first visible tree line (ft.offset).
// returns true if the row maps to a valid entry, false otherwise.
// does not modify the cursor when returning false.
func (ft *FileTree) SelectByVisibleRow(row int) bool {
	if row < 0 {
		return false
	}
	idx := ft.offset + row
	if idx >= len(ft.entries) {
		return false
	}
	ft.cursor = idx
	return true
}

// EnsureVisible adjusts offset so the cursor is within the visible range of given height.
func (ft *FileTree) EnsureVisible(height int) {
	ensureVisible(&ft.cursor, &ft.offset, len(ft.entries), height)
}

// SelectedVisibleRow returns the selected entry's row relative to the viewport.
func (ft *FileTree) SelectedVisibleRow() int {
	return ft.cursor - ft.offset
}

// Rebuild rebuilds the file tree from new entries in-place.
// preserves reviewed map (pruned to files still present), filter state, and the
// selected file's visible row for the unreviewed filter. Other rebuilds reset
// the cursor and offset to the first file entry.
// entries are rebuilt from all files regardless of filter flags; callers
// refresh whichever filter is active after reviewed state is reconciled.
func (ft *FileTree) Rebuild(entries []diff.FileEntry) {
	selected, visibleRow := "", 0
	if ft.unreviewed {
		selected = ft.SelectedFile()
		visibleRow = ft.SelectedVisibleRow()
	}

	paths := diff.FileEntryPaths(entries)
	ft.allFiles = paths

	// build new file status and rename-origin maps from entries
	newStatuses := make(map[string]diff.FileStatus, len(entries))
	newOldPaths := make(map[string]string)
	for _, e := range entries {
		if e.Status != "" {
			newStatuses[e.Path] = e.Status
		}
		if e.OldPath != "" {
			newOldPaths[e.Path] = e.OldPath
		}
	}
	ft.fileStatuses = newStatuses
	ft.oldPaths = newOldPaths

	// prune reviewed map: drop keys no longer in entries
	fileSet := make(map[string]struct{}, len(paths))
	for _, f := range paths {
		fileSet[f] = struct{}{}
	}
	for path := range ft.reviewed {
		if _, ok := fileSet[path]; !ok {
			delete(ft.reviewed, path)
		}
	}

	// rebuild entries list with all files; filter state is preserved but can't be applied
	// without annotated map here — refreshFilter will be called separately if needed
	ft.entries = ft.buildEntries(paths)

	// reset cursor and offset, then restore the unreviewed viewport anchor when
	// its selected file survived the rebuild.
	ft.cursor = 0
	ft.offset = 0
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			break
		}
	}
	if selected != "" && ft.SelectByPath(selected) {
		ft.offset = max(ft.cursor-max(visibleRow, 0), 0)
	}
}

// ToggleFilter switches between showing all files and only annotated files.
func (ft *FileTree) ToggleFilter(annotatedFiles map[string]bool) {
	ft.filter = !ft.filter
	if ft.filter {
		filtered := ft.filterFiles(annotatedFiles)
		if len(filtered) == 0 {
			ft.filter = false // nothing to filter, stay on all
			return
		}
		ft.unreviewed = false
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

// ToggleUnreviewedFilter switches between showing all files and only files
// that have not been marked reviewed. It is mutually exclusive with the
// annotated-only filter.
func (ft *FileTree) ToggleUnreviewedFilter() {
	ft.unreviewed = !ft.unreviewed
	if ft.unreviewed {
		ft.filter = false
		ft.entries = ft.buildEntries(ft.unreviewedFiles())
	} else {
		ft.entries = ft.buildEntries(ft.allFiles)
	}
	ft.selectAfterRebuild("", "")
}

// RefreshUnreviewedFilter rebuilds the unreviewed-only view after reviewed
// state changes, keeping the current file when possible and otherwise moving
// forward to the next unfinished file.
func (ft *FileTree) RefreshUnreviewedFilter() {
	if !ft.unreviewed {
		return
	}
	previous := ft.SelectedFile()
	next := ft.nextUnreviewedAfterCursor()
	ft.entries = ft.buildEntries(ft.unreviewedFiles())
	ft.selectAfterRebuild(previous, next)
}

// RefreshFilter rebuilds the filtered tree if the filter is active, preserving cursor position.
func (ft *FileTree) RefreshFilter(annotatedFiles map[string]bool) {
	if !ft.filter {
		return
	}

	// capture selected file before rebuilding entries
	prevFile := ft.SelectedFile()

	filtered := ft.filterFiles(annotatedFiles)
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

// SetReviewed marks path reviewed at the supplied semantic diff fingerprint.
func (ft *FileTree) SetReviewed(path, fingerprint string) {
	if path == "" || fingerprint == "" {
		return
	}
	ft.reviewed[path] = fingerprint
	ft.RefreshUnreviewedFilter()
}

// Unreview removes the reviewed mark for path.
func (ft *FileTree) Unreview(path string) {
	delete(ft.reviewed, path)
	ft.RefreshUnreviewedFilter()
}

// ReconcileReviewed validates the marks captured when a file-list load began.
// Marks added or changed after that snapshot are left alone for their subsequent
// file load to validate.
func (ft *FileTree) ReconcileReviewed(before, current map[string]string) {
	for path, reviewedFingerprint := range before {
		storedFingerprint, stillReviewed := ft.reviewed[path]
		if !stillReviewed || storedFingerprint != reviewedFingerprint {
			continue
		}
		if currentFingerprint, ok := current[path]; !ok || currentFingerprint != reviewedFingerprint {
			delete(ft.reviewed, path)
		}
	}
	ft.RefreshUnreviewedFilter()
}

// ReconcileReviewedPath validates a reviewed mark when that file's refreshed
// effective diff is loaded.
func (ft *FileTree) ReconcileReviewedPath(path, currentFingerprint string) {
	if reviewedFingerprint, ok := ft.reviewed[path]; ok && reviewedFingerprint != currentFingerprint {
		delete(ft.reviewed, path)
		ft.RefreshUnreviewedFilter()
	}
}

// ScrollState returns the file tree's current visible window state.
// call after Render or EnsureVisible so Offset reflects the latest cursor position.
func (ft *FileTree) ScrollState() ScrollState {
	return ScrollState{Total: len(ft.entries), Offset: ft.offset}
}

// Render produces the file tree display string, showing only entries visible within the given height.
// it adjusts the internal offset so the cursor stays within the visible window.
func (ft *FileTree) Render(r FileTreeRender) string {
	if len(ft.entries) == 0 {
		if ft.unreviewed && len(ft.allFiles) > 0 {
			return "  all files reviewed"
		}
		return "  no changed files"
	}

	ft.EnsureVisible(r.Height)
	end := min(ft.offset+r.Height, len(ft.entries))

	rc := renderCtx{annotatedFiles: r.Annotated, res: r.Resolver, rnd: r.Renderer}
	var b strings.Builder
	for idx := ft.offset; idx < end; idx++ {
		e := ft.entries[idx]
		var line string

		if e.isDir {
			line = r.Resolver.Style(style.StyleKeyDirEntry).Render(" " + ft.truncateDirName(e.name, r.Width-3))
		} else {
			line = ft.renderFileEntry(e, idx, r.Width, rc)
		}

		b.WriteString(line)
		if idx < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// selectAfterRebuild restores previous when it is still visible, then tries
// preferred, and finally resets to the first visible file. Anchored selections
// preserve the offset so EnsureVisible only scrolls when they are hidden.
func (ft *FileTree) selectAfterRebuild(previous, preferred string) {
	ft.cursor = 0
	if len(ft.entries) == 0 {
		ft.offset = 0
		return
	}
	if previous != "" && ft.SelectByPath(previous) {
		return
	}
	if preferred != "" && ft.SelectByPath(preferred) {
		return
	}
	ft.offset = 0
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			return
		}
	}
}

// nextUnreviewedAfterCursor follows the rendered tree order, which can differ
// from the renderer-provided allFiles order after directory grouping.
func (ft *FileTree) nextUnreviewedAfterCursor() string {
	start := min(max(ft.cursor+1, 0), len(ft.entries))
	for _, entry := range ft.entries[start:] {
		if !entry.isDir && !ft.IsReviewed(entry.path) {
			return entry.path
		}
	}
	return ""
}

// buildEntries groups files by directory and creates a flat entry list.
func (ft *FileTree) buildEntries(files []string) []treeEntry {
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

	entries := make([]treeEntry, 0, len(dirs)+len(files))
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

// renderFileEntry renders a single file entry in the tree, truncating long names to prevent wrapping.
func (ft *FileTree) renderFileEntry(e treeEntry, idx, width int, rc renderCtx) string {
	isSelected := idx == ft.cursor
	hasStatuses := len(ft.fileStatuses) > 0

	// use raw ANSI fg-only sequences for inline colored elements to avoid
	// lipgloss \033[0m full reset that breaks outer TreeBg backgrounds.
	reviewMark := "  "
	if ft.IsReviewed(e.path) {
		if isSelected {
			reviewMark = "✓ "
		} else {
			reviewMark = rc.rnd.FileReviewedMark()
		}
	}

	statusMark := ""
	if hasStatuses {
		status := ft.fileStatuses[e.path]
		switch {
		case status == "":
			statusMark = "  "
		case isSelected:
			statusMark = string(status) + " "
		default:
			statusMark = rc.rnd.FileStatusMark(status)
		}
	}

	marker := "  "
	if rc.annotatedFiles[e.path] {
		marker = rc.rnd.FileAnnotationMark()
	}

	prefix := reviewMark + statusMark
	name := prefix + e.name + marker
	maxWidth := width - 2

	// truncate from the left of the filename when it exceeds pane width
	if lipgloss.Width(name) > maxWidth && maxWidth > 4 {
		budget := maxWidth - lipgloss.Width(prefix) - lipgloss.Width(marker) - 1 // 1 for "…"
		if budget > 0 && runewidth.StringWidth(e.name) > budget {
			runes := []rune(e.name)
			w := 0
			start := len(runes)
			for i, r := range slices.Backward(runes) {
				rw := runewidth.RuneWidth(r)
				if w+rw > budget {
					break
				}
				w += rw
				start = i
			}
			name = prefix + "…" + string(runes[start:]) + marker
		}
	}

	if isSelected {
		return rc.res.Style(style.StyleKeyFileSelected).Width(maxWidth).Render(name)
	}
	return rc.res.Style(style.StyleKeyFileEntry).Render(name)
}

// filterFiles returns the subset of allFiles that have annotations.
func (ft *FileTree) filterFiles(annotatedFiles map[string]bool) []string {
	var filtered []string
	for _, f := range ft.allFiles {
		if annotatedFiles[f] {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

func (ft *FileTree) unreviewedFiles() []string {
	filtered := make([]string, 0, len(ft.allFiles))
	for _, path := range ft.allFiles {
		if !ft.IsReviewed(path) {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

// fileIndices returns indices of all file (non-directory) entries.
func (ft *FileTree) fileIndices() []int {
	var indices []int
	for i, e := range ft.entries {
		if !e.isDir {
			indices = append(indices, i)
		}
	}
	return indices
}

// truncateDirName trims a directory name from the left to fit maxWidth display cells,
// prepending an ellipsis when truncated.
func (ft *FileTree) truncateDirName(name string, maxWidth int) string {
	if maxWidth <= 0 || runewidth.StringWidth(name) <= maxWidth {
		return name
	}
	runes := []rune(name)
	w := 0
	start := len(runes)
	for i, r := range slices.Backward(runes) {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth-1 { // reserve 1 cell for "…"
			break
		}
		w += rw
		start = i
	}
	return "…" + string(runes[start:])
}

// moveDown moves cursor to the next file entry (skips directories).
func (ft *FileTree) moveDown() {
	for i := ft.cursor + 1; i < len(ft.entries); i++ {
		if !ft.entries[i].isDir {
			ft.cursor = i
			return
		}
	}
}

// moveUp moves cursor to the previous file entry (skips directories).
func (ft *FileTree) moveUp() {
	for i := ft.cursor - 1; i >= 0; i-- {
		if !ft.entries[i].isDir {
			ft.cursor = i
			return
		}
	}
}

// pageDown moves cursor down by approximately n visual rows,
// accounting for directory header rows that occupy rendered space.
func (ft *FileTree) pageDown(n int) {
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
func (ft *FileTree) pageUp(n int) {
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
func (ft *FileTree) moveToFirst() {
	for i, e := range ft.entries {
		if !e.isDir {
			ft.cursor = i
			return
		}
	}
}

// moveToLast moves cursor to the last file entry.
func (ft *FileTree) moveToLast() {
	for i, e := range slices.Backward(ft.entries) {
		if !e.isDir {
			ft.cursor = i
			return
		}
	}
}

// nextFile moves to the next file entry, wrapping around.
func (ft *FileTree) nextFile() {
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
func (ft *FileTree) prevFile() {
	files := ft.fileIndices()
	if len(files) == 0 {
		return
	}
	for _, f := range slices.Backward(files) {
		if f < ft.cursor {
			ft.cursor = f
			return
		}
	}
	ft.cursor = files[len(files)-1] // wrap around
}
