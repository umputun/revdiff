package ui

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

// loadFiles returns a command that fetches the list of changed files from the renderer.
// it also appends staged-only and untracked files when applicable.
func (m Model) loadFiles() tea.Cmd {
	return func() tea.Msg {
		var warnings []string
		entries, err := m.diffRenderer.ChangedFiles(m.ref, m.staged)
		if err != nil {
			return filesLoadedMsg{entries: entries, err: err}
		}
		// include staged-only files (new files added to index but not yet committed)
		// only when there are no unstaged entries; otherwise unstaged review should stay focused
		// on actual unstaged changes.
		if m.ref == "" && !m.staged && len(entries) == 0 {
			stagedEntries, stagedErr := m.diffRenderer.ChangedFiles("", true)
			if stagedErr != nil {
				warnings = append(warnings, fmt.Sprintf("staged files: %v", stagedErr))
			} else {
				stagedSet := make(map[string]bool)
				for _, e := range entries {
					stagedSet[e.Path] = true
				}
				for _, se := range stagedEntries {
					if !stagedSet[se.Path] && se.Status == diff.FileAdded {
						entries = append(entries, se)
					}
				}
			}
		}
		// append untracked files when toggle is on (skip files already in entries to avoid dupes)
		if m.showUntracked && m.loadUntracked != nil {
			ut, utErr := m.loadUntracked()
			if utErr != nil {
				warnings = append(warnings, fmt.Sprintf("untracked files: %v", utErr))
			} else {
				entrySet := make(map[string]bool, len(entries))
				for _, e := range entries {
					entrySet[e.Path] = true
				}
				for _, f := range ut {
					if !entrySet[f] {
						entries = append(entries, diff.FileEntry{Path: f, Status: diff.FileUntracked})
					}
				}
			}
		}
		return filesLoadedMsg{entries: entries, warnings: warnings}
	}
}

// loadFileDiff returns a command that fetches the diff lines for the given file.
func (m Model) loadFileDiff(file string) tea.Cmd {
	seq := m.loadSeq
	return func() tea.Msg {
		lines, err := m.diffRenderer.FileDiff(m.ref, file, m.staged)
		return fileLoadedMsg{file: file, seq: seq, lines: lines, err: err}
	}
}

// loadBlame returns a command that fetches blame data for the given file.
// returns nil if no blamer is configured.
func (m Model) loadBlame(file string) tea.Cmd {
	if m.blamer == nil {
		return nil
	}
	seq := m.loadSeq
	ref := m.ref
	staged := m.staged
	return func() tea.Msg {
		data, err := m.blamer.FileBlame(ref, file, staged)
		return blameLoadedMsg{file: file, seq: seq, data: data, err: err}
	}
}

// loadSelectedIfChanged ensures the tree is visible and loads the selected file if it changed.
func (m Model) loadSelectedIfChanged() (tea.Model, tea.Cmd) {
	m.tree.EnsureVisible(m.treePageSize())
	if f := m.tree.SelectedFile(); f != "" && f != m.currFile {
		m.loadSeq++
		return m, m.loadFileDiff(f)
	}
	return m, nil
}

// handleFilesLoaded processes the result of loadFiles, populating the file tree
// and triggering the initial file diff load.
func (m Model) handleFilesLoaded(msg filesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.viewport.SetContent(fmt.Sprintf("error loading files: %v", msg.err))
		return m, nil
	}
	for _, w := range msg.warnings {
		log.Printf("[WARN] %s", w)
	}
	entries := m.filterOnly(msg.entries)
	if len(entries) == 0 && len(m.only) > 0 {
		m.viewport.SetContent("no files match --only filter")
		return m, nil
	}
	m.tree.Rebuild(entries)
	if m.tree.FilterActive() {
		m.tree.RefreshFilter(m.annotatedFiles())
	}
	if m.currFile != "" {
		m.tree.SelectByPath(m.currFile)
	}
	m.singleFile = m.tree.TotalFiles() == 1
	if len(entries) == 0 {
		m.currFile = ""
		m.diffLines = nil
		m.highlightedLines = nil
		m.viewport.SetContent("")
		return m, nil
	}
	if m.singleFile {
		m.focus = paneDiff
		m.treeWidth = 0
		if m.ready {
			m.viewport.Width = m.width - 2
		}
	}

	// auto-select first file
	if f := m.tree.SelectedFile(); f != "" {
		m.loadSeq++
		return m, m.loadFileDiff(f)
	}
	return m, nil
}

func (m Model) handleFileLoaded(msg fileLoadedMsg) (tea.Model, tea.Cmd) {
	// discard stale responses; only the latest load request (by sequence) is accepted
	if msg.seq != m.loadSeq {
		return m, nil
	}
	if msg.err != nil {
		m.viewport.SetContent(fmt.Sprintf("error loading diff: %v", msg.err))
		return m, nil
	}
	m.currFile = msg.file
	m.diffLines = msg.lines
	m.resolveEmptyDiff(msg.file, m.tree.FileStatus(msg.file))
	m.clearSearch()
	m.computeFileStats()
	m.highlightedLines = m.highlighter.HighlightLines(msg.file, m.diffLines)
	m.recomputeIntraRanges()
	if m.lineNumbers {
		m.lineNumWidth = m.computeLineNumWidth()
	}
	m.cursorOnAnnotation = false
	m.scrollX = 0
	m.collapsed.expandedHunks = make(map[int]bool)

	m.singleColLineNum = m.isFullContext(msg.lines)

	// detect markdown full-context mode and build TOC
	m.mdTOC = nil
	if m.singleFile && m.isMarkdownFile(msg.file) && m.singleColLineNum {
		m.mdTOC = m.parseTOC(msg.lines, msg.file)
	}
	switch {
	case m.mdTOC != nil && !m.treeHidden:
		m.treeWidth = max(minTreeWidth, m.width*m.treeWidthRatio/10)
		m.viewport.Width = m.width - m.treeWidth - 4
	case m.singleFile || m.treeHidden:
		m.treeWidth = 0
		m.viewport.Width = m.width - 2
	}

	m.skipInitialDividers()
	m.syncTOCActiveSection()

	// clear stale blame data; reload if blame is active
	var blameCmd tea.Cmd
	if m.showBlame {
		m.blameData = nil
		m.blameAuthorLen = 0
		blameCmd = m.loadBlame(msg.file)
	}

	// handle pending annotation list jump
	if m.pendingAnnotJump != nil && m.pendingAnnotJump.File == msg.file {
		a := *m.pendingAnnotJump
		m.pendingAnnotJump = nil
		m.pendingHunkJump = nil
		m.positionOnAnnotation(a)
		return m, blameCmd
	}

	// handle pending hunk jump after cross-file hunk navigation
	if m.pendingHunkJump != nil {
		m.applyPendingHunkJump()
		m.centerViewportOnCursor()
		return m, blameCmd
	}

	m.viewport.SetContent(m.renderDiff())
	m.viewport.GotoTop()
	return m, blameCmd
}

// resolveEmptyDiff populates m.diffLines when git diff returns empty.
// For staged-only FileAdded files (new files in the index), retries with --cached.
// For untracked files, reads from disk as all-added lines.
func (m *Model) resolveEmptyDiff(file string, fileStatus diff.FileStatus) {
	if len(m.diffLines) > 0 {
		return
	}
	// staged-only files: retry with git diff --cached
	if !m.staged && fileStatus == diff.FileAdded && m.diffRenderer != nil {
		if cachedLines, err := m.diffRenderer.FileDiff(m.ref, file, true); err == nil && len(cachedLines) > 0 {
			m.diffLines = cachedLines
			return
		}
	}
	// untracked files: read from disk as all-added lines
	if m.workDir != "" && fileStatus == diff.FileUntracked {
		added, err := diff.ReadFileAsAdded(filepath.Join(m.workDir, file))
		if err != nil {
			log.Printf("[WARN] read untracked file %s: %v", file, err)
		}
		if len(added) > 0 {
			m.diffLines = added
		}
	}
}

// handleBlameLoaded processes asynchronously loaded blame data for a file.
func (m Model) handleBlameLoaded(msg blameLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.loadSeq || msg.file != m.currFile || !m.showBlame {
		return m, nil
	}
	if msg.err != nil {
		// blame unavailable for this file (e.g. new untracked file); silently ignore
		return m, nil
	}
	m.blameData = msg.data
	m.blameAuthorLen = m.computeBlameAuthorLen()
	m.syncViewportToCursor()
	return m, nil
}

// maxBlameAuthor is the maximum display width for author names in the blame gutter.
const maxBlameAuthor = 8

// computeBlameAuthorLen returns the display width of the longest author name in blame data.
// caps at maxBlameAuthor characters to keep the gutter compact.
func (m Model) computeBlameAuthorLen() int {
	maxLen := 0
	for _, bl := range m.blameData {
		if l := runewidth.StringWidth(bl.Author); l > maxLen {
			maxLen = l
		}
	}
	if maxLen > maxBlameAuthor {
		maxLen = maxBlameAuthor
	}
	if maxLen == 0 {
		maxLen = 1
	}
	return maxLen
}

// filterOnly returns only files matching the --only patterns, or all files if no filter is set.
// matches by exact path or path suffix (e.g. "model.go" matches "ui/model.go").
// when a pattern is an absolute path, it is also resolved relative to workDir for matching
// (e.g. "/repo/README.md" with workDir="/repo" matches "README.md").
func (m Model) filterOnly(entries []diff.FileEntry) []diff.FileEntry {
	if len(m.only) == 0 {
		return entries
	}
	var filtered []diff.FileEntry
	for _, e := range entries {
		for _, pattern := range m.only {
			if e.Path == pattern || strings.HasSuffix(e.Path, "/"+pattern) {
				filtered = append(filtered, e)
				break
			}
			// resolve absolute pattern relative to workDir for matching against repo-relative files
			if m.workDir != "" && filepath.IsAbs(pattern) {
				rel, err := filepath.Rel(m.workDir, pattern)
				if err == nil && !strings.HasPrefix(rel, "..") && (e.Path == rel || strings.HasSuffix(e.Path, "/"+rel)) {
					filtered = append(filtered, e)
					break
				}
			}
		}
	}
	return filtered
}

// computeFileStats counts added and removed lines in the current diffLines.
func (m *Model) computeFileStats() {
	m.fileAdds, m.fileRemoves = 0, 0
	for _, dl := range m.diffLines {
		switch dl.ChangeType {
		case diff.ChangeAdd:
			m.fileAdds++
		case diff.ChangeRemove:
			m.fileRemoves++
		case diff.ChangeContext, diff.ChangeDivider:
			// not counted in stats
		}
	}
}

// fileStatsText returns the stats segment for the status bar.
// shows total line count for context-only files, or +adds/-removes for diffs.
func (m Model) fileStatsText() string {
	if m.fileAdds == 0 && m.fileRemoves == 0 && len(m.diffLines) > 0 {
		return fmt.Sprintf("%d lines", len(m.diffLines))
	}
	return fmt.Sprintf("+%d/-%d", m.fileAdds, m.fileRemoves)
}

// skipInitialDividers positions diffCursor on the first visible line.
// skips divider lines, and in collapsed mode also skips removed lines
// unless their hunk is expanded.
func (m *Model) skipInitialDividers() {
	m.diffCursor = 0
	hunks := m.findHunks()
	for i, dl := range m.diffLines {
		if dl.ChangeType == diff.ChangeDivider || m.isCollapsedHidden(i, hunks) {
			continue
		}
		m.diffCursor = i
		return
	}
}

// isFullContext returns true when every non-divider line is ChangeContext.
func (m Model) isFullContext(lines []diff.DiffLine) bool {
	hasContext := false
	for _, line := range lines {
		if line.ChangeType == diff.ChangeDivider {
			continue
		}
		if line.ChangeType != diff.ChangeContext {
			return false
		}
		hasContext = true
	}
	return hasContext
}

// isMarkdownFile returns true when the filename has a markdown extension.
func (m Model) isMarkdownFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown"
}

// recomputeIntraRanges walks m.diffLines, finds contiguous change blocks,
// pairs remove/add lines, runs word-diff, and stores results in m.intraRanges.
// no-op when m.wordDiff is off: clears m.intraRanges to nil so callers don't
// need to duplicate the guard at each call site.
func (m *Model) recomputeIntraRanges() {
	if !m.wordDiff {
		m.intraRanges = nil
		return
	}
	n := len(m.diffLines)
	m.intraRanges = make([][]worddiff.Range, n)

	i := 0
	for i < n {
		if m.diffLines[i].ChangeType != diff.ChangeAdd && m.diffLines[i].ChangeType != diff.ChangeRemove {
			i++
			continue
		}
		blockStart := i
		for i < n && (m.diffLines[i].ChangeType == diff.ChangeAdd || m.diffLines[i].ChangeType == diff.ChangeRemove) {
			i++
		}

		// build LinePair slice for the block
		block := make([]worddiff.LinePair, i-blockStart)
		for j := blockStart; j < i; j++ {
			block[j-blockStart] = worddiff.LinePair{
				Content:  strings.ReplaceAll(m.diffLines[j].Content, "\t", m.tabSpaces),
				IsRemove: m.diffLines[j].ChangeType == diff.ChangeRemove,
			}
		}

		pairs := m.differ.PairLines(block)
		for _, p := range pairs {
			minusRanges, plusRanges := m.differ.ComputeIntraRanges(block[p.RemoveIdx].Content, block[p.AddIdx].Content)
			m.intraRanges[blockStart+p.RemoveIdx] = minusRanges
			m.intraRanges[blockStart+p.AddIdx] = plusRanges
		}
	}
}
