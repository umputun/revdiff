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
// the caller must bump m.filesLoadSeq before invoking loadFiles when issuing a new
// reload (e.g. toggleUntracked); the captured seq tags every emitted filesLoadedMsg
// so handleFilesLoaded can drop stale results from earlier in-flight loads.
func (m Model) loadFiles() tea.Cmd {
	seq := m.filesLoadSeq
	return func() tea.Msg {
		var warnings []string
		entries, err := m.diffRenderer.ChangedFiles(m.cfg.ref, m.cfg.staged)
		if err != nil {
			return filesLoadedMsg{seq: seq, entries: entries, err: err}
		}
		// include staged-only files (new files added to index but not yet committed)
		// only when there are no unstaged entries; otherwise unstaged review should stay focused
		// on actual unstaged changes.
		if m.cfg.ref == "" && !m.cfg.staged && len(entries) == 0 {
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
		if m.modes.showUntracked && m.loadUntracked != nil {
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
		return filesLoadedMsg{seq: seq, entries: entries, warnings: warnings}
	}
}

// loadCommits returns a command that fetches the commit log for the current ref range.
// returns nil when the feature is not applicable or no source is configured, so callers
// can unconditionally include it in tea.Batch alongside loadFiles.
// the caller must bump m.commits.loadSeq before invoking loadCommits when issuing a new
// reload (e.g. triggerReload); the captured seq tags every emitted commitsLoadedMsg
// so handleCommitsLoaded can drop stale results from earlier in-flight loads.
func (m Model) loadCommits() tea.Cmd {
	if !m.commits.applicable || m.commits.source == nil {
		return nil
	}
	seq := m.commits.loadSeq
	ref := m.cfg.ref
	src := m.commits.source
	return func() tea.Msg {
		list, err := src.CommitLog(ref)
		return commitsLoadedMsg{
			seq:       seq,
			list:      list,
			err:       err,
			truncated: len(list) >= diff.MaxCommits,
		}
	}
}

// loadFileDiff returns a command that fetches the diff lines for the given file.
func (m Model) loadFileDiff(file string) tea.Cmd {
	seq := m.file.loadSeq
	return func() tea.Msg {
		lines, err := m.diffRenderer.FileDiff(m.cfg.ref, file, m.cfg.staged)
		return fileLoadedMsg{file: file, seq: seq, lines: lines, err: err}
	}
}

// loadBlame returns a command that fetches blame data for the given file.
// returns nil if no blamer is configured.
func (m Model) loadBlame(file string) tea.Cmd {
	if m.blamer == nil {
		return nil
	}
	seq := m.file.loadSeq
	ref := m.cfg.ref
	staged := m.cfg.staged
	return func() tea.Msg {
		data, err := m.blamer.FileBlame(ref, file, staged)
		return blameLoadedMsg{file: file, seq: seq, data: data, err: err}
	}
}

// loadSelectedIfChanged ensures the tree is visible and loads the selected file if it changed.
func (m Model) loadSelectedIfChanged() (tea.Model, tea.Cmd) {
	m.tree.EnsureVisible(m.treePageSize())
	if f := m.tree.SelectedFile(); f != "" && f != m.file.name {
		m.file.loadSeq++
		return m, m.loadFileDiff(f)
	}
	return m, nil
}

// triggerReload triggers a full reload of the file list, current file diff,
// and commit log. It bumps filesLoadSeq and commits.loadSeq to invalidate any
// in-flight loads, clears the commit cache so the overlay shows the loading
// hint (not stale data) while the re-fetch is in flight, then re-runs the same
// parallel pipeline as startup via tea.Batch. The selected file in the tree is
// restored by SelectByPath in handleFilesLoaded; the diff cursor resets to
// the top of the file. Named triggerReload (not reload) to avoid shadowing
// the Model.reload field.
func (m *Model) triggerReload() tea.Cmd {
	m.filesLoadSeq++
	m.file.loadSeq++ // invalidate in-flight fileLoadedMsg from pre-reload selection
	m.commits.loadSeq++
	m.commits.loaded = false
	m.commits.list = nil
	return tea.Batch(m.loadFiles(), m.loadCommits())
}

// handleFilesLoaded processes the result of loadFiles, populating the file tree
// and triggering the initial file diff load.
func (m Model) handleFilesLoaded(msg filesLoadedMsg) (tea.Model, tea.Cmd) {
	// drop stale responses from an earlier load; only the latest load request
	// (matching m.filesLoadSeq) is accepted. Prevents an older in-flight load from
	// overwriting the tree after toggleUntracked (or a rapid double-toggle).
	if msg.seq != m.filesLoadSeq {
		return m, nil
	}
	m.filesLoaded = true
	if msg.err != nil {
		m.layout.viewport.SetContent(fmt.Sprintf("error loading files: %v", msg.err))
		return m, nil
	}
	for _, w := range msg.warnings {
		log.Printf("[WARN] %s", w)
	}
	entries := m.filterOnly(msg.entries)
	if len(entries) == 0 && len(m.cfg.only) > 0 {
		m.layout.viewport.SetContent("no files match --only filter")
		return m, nil
	}
	m.tree.Rebuild(entries)
	if m.tree.FilterActive() {
		m.tree.RefreshFilter(m.annotatedFiles())
	}
	if m.file.name != "" {
		m.tree.SelectByPath(m.file.name)
	}
	m.file.singleFile = m.tree.TotalFiles() == 1
	if len(entries) == 0 {
		m.file.name = ""
		m.file.lines = nil
		m.file.highlighted = nil
		m.layout.viewport.SetContent("")
		return m, nil
	}
	if m.file.singleFile {
		m.layout.focus = paneDiff
		m.layout.treeWidth = 0
		if m.ready {
			m.layout.viewport.Width = m.layout.width - 2
		}
	}

	// auto-select first file
	if f := m.tree.SelectedFile(); f != "" {
		m.file.loadSeq++
		return m, m.loadFileDiff(f)
	}
	return m, nil
}

// handleCommitsLoaded processes the result of loadCommits, populating the
// cached commit log on the model. Drops stale results from earlier in-flight
// loads (triggered by R reload) via the seq tag, mirroring handleFilesLoaded.
// An error is cached the same way as success: loaded is set to true so the
// overlay shows the error once instead of re-triggering the fetch.
func (m Model) handleCommitsLoaded(msg commitsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.commits.loadSeq {
		return m, nil
	}
	m.commits.list = msg.list
	m.commits.err = msg.err
	m.commits.truncated = msg.truncated
	m.commits.loaded = true
	return m, nil
}

func (m Model) handleFileLoaded(msg fileLoadedMsg) (tea.Model, tea.Cmd) {
	// discard stale responses; only the latest load request (by sequence) is accepted
	if msg.seq != m.file.loadSeq {
		return m, nil
	}
	if msg.err != nil {
		m.layout.viewport.SetContent(fmt.Sprintf("error loading diff: %v", msg.err))
		return m, nil
	}
	m.file.name = msg.file
	m.file.lines = msg.lines
	m.resolveEmptyDiff(msg.file, m.tree.FileStatus(msg.file))
	m.clearSearch()
	m.computeFileStats()
	m.file.highlighted = m.highlighter.HighlightLines(msg.file, m.file.lines)
	m.recomputeIntraRanges()
	if m.modes.lineNumbers {
		m.file.lineNumWidth = m.computeLineNumWidth()
	}
	m.annot.cursorOnAnnotation = false
	m.layout.scrollX = 0
	m.modes.collapsed.expandedHunks = make(map[int]bool)

	m.file.singleColLineNum = m.isFullContext(msg.lines)

	// detect markdown full-context mode and build TOC
	m.file.mdTOC = nil
	if m.file.singleFile && m.isMarkdownFile(msg.file) && m.file.singleColLineNum {
		m.file.mdTOC = m.parseTOC(msg.lines, msg.file)
	}
	switch {
	case m.file.mdTOC != nil && !m.layout.treeHidden:
		m.layout.treeWidth = max(minTreeWidth, m.layout.width*m.cfg.treeWidthRatio/10)
		m.layout.viewport.Width = m.layout.width - m.layout.treeWidth - 4
	case m.file.singleFile || m.layout.treeHidden:
		m.layout.treeWidth = 0
		m.layout.viewport.Width = m.layout.width - 2
	}

	m.skipInitialDividers()
	m.syncTOCActiveSection()

	// clear stale blame data; reload if blame is active
	var blameCmd tea.Cmd
	if m.modes.showBlame {
		m.file.blameData = nil
		m.file.blameAuthorLen = 0
		blameCmd = m.loadBlame(msg.file)
	}

	// handle pending annotation list jump
	if m.pendingAnnotJump != nil && m.pendingAnnotJump.File == msg.file {
		a := *m.pendingAnnotJump
		m.pendingAnnotJump = nil
		m.nav.pendingHunkJump = nil
		m.positionOnAnnotation(a)
		return m, blameCmd
	}

	// handle pending hunk jump after cross-file hunk navigation
	if m.nav.pendingHunkJump != nil {
		m.applyPendingHunkJump()
		m.centerViewportOnCursor()
		return m, blameCmd
	}

	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.GotoTop()
	return m, blameCmd
}

// resolveEmptyDiff populates m.file.lines when git diff returns empty.
// For staged-only FileAdded files (new files in the index), retries with --cached.
// For untracked files, reads from disk as all-added lines.
func (m *Model) resolveEmptyDiff(file string, fileStatus diff.FileStatus) {
	if len(m.file.lines) > 0 {
		return
	}
	// staged-only files: retry with git diff --cached
	if !m.cfg.staged && fileStatus == diff.FileAdded && m.diffRenderer != nil {
		if cachedLines, err := m.diffRenderer.FileDiff(m.cfg.ref, file, true); err == nil && len(cachedLines) > 0 {
			m.file.lines = cachedLines
			return
		}
	}
	// untracked files: read from disk as all-added lines
	if m.cfg.workDir != "" && fileStatus == diff.FileUntracked {
		added, err := diff.ReadFileAsAdded(filepath.Join(m.cfg.workDir, file))
		if err != nil {
			log.Printf("[WARN] read untracked file %s: %v", file, err)
		}
		if len(added) > 0 {
			m.file.lines = added
		}
	}
}

// handleBlameLoaded processes asynchronously loaded blame data for a file.
func (m Model) handleBlameLoaded(msg blameLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.file.loadSeq || msg.file != m.file.name || !m.modes.showBlame {
		return m, nil
	}
	if msg.err != nil {
		// blame unavailable for this file (e.g. new untracked file); silently ignore
		return m, nil
	}
	m.file.blameData = msg.data
	m.file.blameAuthorLen = m.computeBlameAuthorLen()
	m.syncViewportToCursor()
	return m, nil
}

// maxBlameAuthor is the maximum display width for author names in the blame gutter.
const maxBlameAuthor = 8

// computeBlameAuthorLen returns the display width of the longest author name in blame data.
// caps at maxBlameAuthor characters to keep the gutter compact.
func (m Model) computeBlameAuthorLen() int {
	maxLen := 0
	for _, bl := range m.file.blameData {
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
// when workDir is set, both entries and patterns are also normalized to workDir-relative
// form for matching, so "./CLAUDE.md" matches "CLAUDE.md" and an absolute entry from
// FileReader ("/repo/CLAUDE.md") matches a relative pattern ("CLAUDE.md").
func (m Model) filterOnly(entries []diff.FileEntry) []diff.FileEntry {
	if len(m.cfg.only) == 0 {
		return entries
	}
	var filtered []diff.FileEntry
	for _, e := range entries {
		for _, pattern := range m.cfg.only {
			if m.entryMatchesOnly(e.Path, pattern) {
				filtered = append(filtered, e)
				break
			}
		}
	}
	return filtered
}

// entryMatchesOnly reports whether a file entry matches a single --only pattern.
// checks exact match, suffix match, and workDir-normalized equality/suffix.
// normalization handles "./" prefixes, absolute patterns, and absolute entries uniformly.
func (m Model) entryMatchesOnly(entry, pattern string) bool {
	if entry == pattern || strings.HasSuffix(entry, "/"+pattern) {
		return true
	}
	if m.cfg.workDir == "" {
		return false
	}
	entryRel := m.workDirRel(entry)
	patternRel := m.workDirRel(pattern)
	if entryRel == "" || patternRel == "" {
		return false
	}
	return entryRel == patternRel || strings.HasSuffix(entryRel, "/"+patternRel)
}

// workDirRel returns path as workDir-relative, or "" if path escapes workDir.
// relative input is joined with workDir first; absolute input is used as-is.
func (m Model) workDirRel(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.cfg.workDir, path)
	}
	rel, err := filepath.Rel(m.cfg.workDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}

// computeFileStats counts added and removed lines in the current file.
func (m *Model) computeFileStats() {
	m.file.adds, m.file.removes = 0, 0
	for _, dl := range m.file.lines {
		switch dl.ChangeType {
		case diff.ChangeAdd:
			m.file.adds++
		case diff.ChangeRemove:
			m.file.removes++
		case diff.ChangeContext, diff.ChangeDivider:
			// not counted in stats
		}
	}
}

// fileStatsText returns the stats segment for the status bar.
// shows total line count for context-only files, or +adds/-removes for diffs.
func (m Model) fileStatsText() string {
	if m.file.adds == 0 && m.file.removes == 0 && len(m.file.lines) > 0 {
		return fmt.Sprintf("%d lines", len(m.file.lines))
	}
	return fmt.Sprintf("+%d/-%d", m.file.adds, m.file.removes)
}

// skipInitialDividers positions diffCursor on the first visible line.
// skips divider lines, and in collapsed mode also skips removed lines
// unless their hunk is expanded.
func (m *Model) skipInitialDividers() {
	m.nav.diffCursor = 0
	hunks := m.findHunks()
	for i, dl := range m.file.lines {
		if dl.ChangeType == diff.ChangeDivider || m.isCollapsedHidden(i, hunks) {
			continue
		}
		m.nav.diffCursor = i
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

// recomputeIntraRanges walks m.file.lines, finds contiguous change blocks,
// pairs remove/add lines, runs word-diff, and stores results in m.file.intraRanges.
// no-op when m.modes.wordDiff is off: clears m.file.intraRanges to nil so callers don't
// need to duplicate the guard at each call site.
func (m *Model) recomputeIntraRanges() {
	if !m.modes.wordDiff {
		m.file.intraRanges = nil
		return
	}
	n := len(m.file.lines)
	m.file.intraRanges = make([][]worddiff.Range, n)

	i := 0
	for i < n {
		if m.file.lines[i].ChangeType != diff.ChangeAdd && m.file.lines[i].ChangeType != diff.ChangeRemove {
			i++
			continue
		}
		blockStart := i
		for i < n && (m.file.lines[i].ChangeType == diff.ChangeAdd || m.file.lines[i].ChangeType == diff.ChangeRemove) {
			i++
		}

		// build LinePair slice for the block
		block := make([]worddiff.LinePair, i-blockStart)
		for j := blockStart; j < i; j++ {
			block[j-blockStart] = worddiff.LinePair{
				Content:  strings.ReplaceAll(m.file.lines[j].Content, "\t", m.cfg.tabSpaces),
				IsRemove: m.file.lines[j].ChangeType == diff.ChangeRemove,
			}
		}

		pairs := m.differ.PairLines(block)
		for _, p := range pairs {
			minusRanges, plusRanges := m.differ.ComputeIntraRanges(block[p.RemoveIdx].Content, block[p.AddIdx].Content)
			m.file.intraRanges[blockStart+p.RemoveIdx] = minusRanges
			m.file.intraRanges[blockStart+p.AddIdx] = plusRanges
		}
	}
}
