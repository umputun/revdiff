package ui

import (
	"fmt"
	"log"
	"maps"
	"path/filepath"
	"sort"
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
	reviewed := m.tree.ReviewedFingerprints()
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
				renames, rWarn := m.detectUntrackedRenames(ut)
				if rWarn != "" {
					warnings = append(warnings, rWarn)
				}
				entries = m.mergeUntrackedEntries(entries, ut, renames)
			}
		}
		fingerprints, fingerprintWarnings := m.loadReviewedFingerprints(entries, reviewed)
		warnings = append(warnings, fingerprintWarnings...)
		return filesLoadedMsg{
			seq:                  seq,
			entries:              entries,
			reviewedBefore:       reviewed,
			reviewedFingerprints: fingerprints,
			warnings:             warnings,
		}
	}
}

const maxReviewFingerprintWorkers = 4

// loadReviewedFingerprints refreshes identities only for paths that were
// reviewed before this file-list load. A small worker pool prevents a large
// review from serializing one VCS process per file without creating unbounded
// subprocess concurrency.
func (m Model) loadReviewedFingerprints(entries []diff.FileEntry, reviewed map[string]string) (map[string]string, []string) {
	current := make(map[string]string, len(reviewed))
	if len(reviewed) == 0 {
		return current, nil
	}

	jobs := make([]diff.FileEntry, 0, len(reviewed))
	for _, entry := range entries {
		if _, ok := reviewed[entry.Path]; ok {
			jobs = append(jobs, entry)
		}
	}
	if len(jobs) == 0 {
		return current, nil
	}

	type fingerprintResult struct {
		path        string
		fingerprint string
		stable      bool
		err         error
	}
	jobCh := make(chan diff.FileEntry, len(jobs))
	resultCh := make(chan fingerprintResult, len(jobs))
	for _, entry := range jobs {
		jobCh <- entry
	}
	close(jobCh)

	workers := min(maxReviewFingerprintWorkers, len(jobs))
	for range workers {
		go func() {
			for entry := range jobCh {
				lines, err := m.fetchEffectiveFileDiff(entry, 0, true)
				result := fingerprintResult{path: entry.Path, err: err}
				if err == nil {
					result.fingerprint = diff.FileFingerprint(entry, lines)
					result.stable = diff.ReviewFingerprintStable(lines)
				}
				resultCh <- result
			}
		}()
	}

	warnings := make([]string, 0)
	for range jobs {
		result := <-resultCh
		if result.err != nil {
			warnings = append(warnings, fmt.Sprintf("reviewed fingerprint %s: %v", result.path, result.err))
			continue
		}
		if !result.stable {
			continue
		}
		current[result.path] = result.fingerprint
	}
	sort.Strings(warnings)
	return current, warnings
}

// detectUntrackedRenames pairs untracked renames with their deleted origin. It is a
// no-op (returns nil) unless a git rename detector is wired and the review is in
// unstaged working-tree mode — the only mode where a `mv old new` leaves new untracked
// and old deleted. The returned warning string is non-empty only when detection failed.
func (m Model) detectUntrackedRenames(untracked []string) ([]diff.FileEntry, string) {
	if m.loadUntrackedRenames == nil || m.cfg.ref != "" || m.cfg.staged {
		return nil, ""
	}
	renames, err := m.loadUntrackedRenames(untracked)
	if err != nil {
		return nil, fmt.Sprintf("untracked renames: %v", err)
	}
	return renames, ""
}

// mergeUntrackedEntries folds untracked files and detected untracked renames into the
// tracked-change entries. Each rename replaces the standalone deletion of its origin
// with a single rename entry; the remaining untracked paths are appended as additions,
// skipping any already present (including rename new-sides) to avoid duplicates.
func (m Model) mergeUntrackedEntries(entries []diff.FileEntry, untracked []string, renames []diff.FileEntry) []diff.FileEntry {
	renameOld := make(map[string]bool, len(renames))
	for _, r := range renames {
		renameOld[r.OldPath] = true
	}

	// drop standalone deletions that are actually rename origins, then add the renames
	if len(renameOld) > 0 {
		kept := make([]diff.FileEntry, 0, len(entries))
		for _, e := range entries {
			if e.Status == diff.FileDeleted && renameOld[e.Path] {
				continue
			}
			kept = append(kept, e)
		}
		entries = kept
	}
	entries = append(entries, renames...)

	// rename new-sides are already in entries (appended above), so entrySet covers
	// them — any untracked path equal to a rename new-side is skipped here too.
	entrySet := make(map[string]bool, len(entries))
	for _, e := range entries {
		entrySet[e.Path] = true
	}
	for _, f := range untracked {
		if !entrySet[f] {
			entries = append(entries, diff.FileEntry{Path: f, Status: diff.FileUntracked})
		}
	}
	return entries
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
	contextLines := m.currentContextLines()
	entry := diff.FileEntry{Path: file, OldPath: m.tree.OldPath(file), Status: m.tree.FileStatus(file)}
	return func() tea.Msg {
		lines, err := m.fetchEffectiveFileDiff(entry, contextLines, false)
		return fileLoadedMsg{file: file, oldName: entry.OldPath, seq: seq, lines: lines, err: err}
	}
}

// fetchEffectiveFileDiff applies the same staged-only and untracked fallbacks
// used by the visible diff. Fingerprints call this helper too, so review state
// is always based on the content revdiff would actually display.
func (m Model) fetchEffectiveFileDiff(entry diff.FileEntry, contextLines int, strict bool) ([]diff.DiffLine, error) {
	req := diff.FileDiffRequest{
		Ref:          m.cfg.ref,
		Path:         entry.Path,
		OldPath:      entry.OldPath,
		Staged:       m.cfg.staged,
		ContextLines: contextLines,
	}
	lines, err := m.diffRenderer.FileDiff(req)
	if err != nil {
		return lines, fmt.Errorf("load effective diff %s: %w", entry.Path, err)
	}
	if len(lines) > 0 {
		return lines, nil
	}

	if !m.cfg.staged && entry.Status == diff.FileAdded {
		req.Staged = true
		cachedLines, cachedErr := m.diffRenderer.FileDiff(req)
		if cachedErr != nil {
			return cachedLines, fmt.Errorf("load staged effective diff %s: %w", entry.Path, cachedErr)
		}
		if len(cachedLines) > 0 {
			return cachedLines, nil
		}
	}
	if m.cfg.workDir != "" && entry.Status == diff.FileUntracked {
		added, readErr := diff.ReadFileAsAdded(filepath.Join(m.cfg.workDir, entry.Path))
		if readErr != nil && !strict {
			log.Printf("[WARN] read untracked file %s: %v", entry.Path, readErr)
			return nil, nil
		}
		if readErr != nil {
			return added, fmt.Errorf("read untracked file %s: %w", entry.Path, readErr)
		}
		return added, nil
	}
	return lines, nil
}

// loadReviewFingerprint fetches a semantic identity for a tree selection that
// was marked before its normal visible diff load completed.
func (m Model) loadReviewFingerprint(entry diff.FileEntry, seq uint64) tea.Cmd {
	filesSeq := m.filesLoadSeq
	return func() tea.Msg {
		lines, err := m.fetchEffectiveFileDiff(entry, 0, true)
		msg := reviewFingerprintLoadedMsg{path: entry.Path, seq: seq, filesSeq: filesSeq, err: err}
		if err == nil {
			msg.fingerprint = diff.FileFingerprint(entry, lines)
		}
		return msg
	}
}

// currentContextLines returns the context-lines count to pass to FileDiff based
// on the current compact mode state. returns compactContext when compact mode
// is enabled and the feature is applicable to this source; otherwise 0, which
// the renderers treat as "full-file context" (the revdiff default).
func (m Model) currentContextLines() int {
	if m.modes.compact && m.compact.applicable {
		return m.modes.compactContext
	}
	return 0
}

// reloadCurrentFile re-fetches the diff for the currently displayed file and
// invalidates any in-flight load for it via a loadSeq bump. Used by the compact
// toggle to pick up the new context size without re-fetching the full file list
// or commit log (unlike triggerReload). Returns nil when no file is loaded.
func (m *Model) reloadCurrentFile() tea.Cmd {
	if m.file.name == "" {
		return nil
	}
	m.file.loadSeq++
	return m.loadFileDiff(m.file.name)
}

// captureCompactAnchor records the cursor's semantic position before a compact
// toggle so applyCompactAnchor can restore the view after the re-fetch at the
// new context size. Returns nil when no file is loaded or the cursor is unset,
// in which case the toggle falls back to the default top positioning.
func (m Model) captureCompactAnchor() *compactAnchor {
	if m.file.name == "" || m.nav.diffCursor < 0 || m.nav.diffCursor >= len(m.file.lines) {
		return nil
	}
	dl := m.file.lines[m.nav.diffCursor]
	return &compactAnchor{
		srcLine:    m.diffLineNum(dl),
		changeType: dl.ChangeType,
		hunkIdx:    m.nearestHunkIndex(m.nav.diffCursor),
	}
}

// applyCompactAnchor restores the cursor to the position captured before a
// compact toggle and centers the viewport on it. It resolves the anchor by
// source line number; when that line is absent from the re-fetched diff (a
// context line dropped in the full→compact direction), it falls back to the
// nearest hunk captured at toggle time, then to the first visible line. In
// collapsed mode adjustCursorIfHidden nudges the cursor off a hidden removed
// line (e.g. a modify hunk's start) so it never lands out of sight.
func (m *Model) applyCompactAnchor(a *compactAnchor) {
	m.annot.cursorOnAnnotation = false
	if a.srcLine > 0 && a.changeType != diff.ChangeDivider {
		if idx := m.findDiffLineIndex(a.srcLine, string(a.changeType)); idx >= 0 {
			m.nav.diffCursor = idx
			m.adjustCursorIfHidden()
			m.centerViewportOnCursor()
			return
		}
	}
	if hunks := m.findHunks(); a.hunkIdx >= 0 && a.hunkIdx < len(hunks) {
		m.nav.diffCursor = hunks[a.hunkIdx]
		m.adjustCursorIfHidden()
		m.centerViewportOnCursor()
		return
	}
	m.skipInitialDividers()
	m.centerViewportOnCursor()
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
// in-flight loads, clears the commit cache and review-stats cache so the
// overlay shows loading state (not stale data) while the re-fetch is in
// flight, then re-runs the same parallel pipeline as startup via tea.Batch.
// The selected file in the tree is restored by SelectByPath in
// handleFilesLoaded; the diff cursor resets to the top of the file. Named
// triggerReload (not reload) to avoid shadowing the Model.reload field.
func (m *Model) triggerReload() tea.Cmd {
	m.filesLoadSeq++
	m.file.loadSeq++ // invalidate in-flight fileLoadedMsg from pre-reload selection
	m.commits.loadSeq++
	m.commits.loaded = false
	m.commits.list = nil
	m.filesLoaded = false
	m.review.adds = 0
	m.review.removes = 0
	m.review.partial = false
	m.review.statsLoaded = false
	m.review.statsRequested = false
	m.review.statsLoadSeq++ // invalidate any in-flight stats fetch
	m.reviewed.loadSeq++
	m.reviewed.cache = make(map[string]string)
	m.reviewed.pending = make(map[string]uint64)
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
	statsPending := m.review.statsRequested && !m.review.statsLoaded
	m.setReviewEntries(entries)
	var statsCmd tea.Cmd
	if statsPending {
		statsCmd = m.triggerReviewStats()
	}
	m.refreshInfoOverlay()
	if len(entries) == 0 && len(m.cfg.only) > 0 {
		m.layout.viewport.SetContent("no files match --only filter")
		return m, statsCmd
	}
	// Pending marks were started against the previous tree. Keep requests for
	// surviving paths, but cancel removed ones so a late fingerprint cannot
	// reintroduce an invisible reviewed entry.
	present := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		present[entry.Path] = struct{}{}
	}
	for path := range m.reviewed.pending {
		if _, ok := present[path]; !ok {
			delete(m.reviewed.pending, path)
		}
	}
	m.tree.Rebuild(entries)
	m.tree.ReconcileReviewed(msg.reviewedBefore, msg.reviewedFingerprints)
	m.reviewed.cache = make(map[string]string, len(msg.reviewedFingerprints))
	maps.Copy(m.reviewed.cache, msg.reviewedFingerprints)
	if m.tree.FilterActive() {
		m.tree.RefreshFilter(m.annotatedFiles())
	}
	if m.tree.UnreviewedFilterActive() {
		m.tree.RefreshUnreviewedFilter()
	}
	if m.file.name != "" {
		m.tree.SelectByPath(m.file.name)
	}
	m.file.singleFile = m.tree.TotalFiles() == 1
	if len(entries) == 0 {
		m.file.name = ""
		m.file.oldName = ""
		m.file.lines = nil
		m.file.highlighted = nil
		m.layout.viewport.SetContent("")
		return m, statsCmd
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
		return m, tea.Batch(m.loadFileDiff(f), statsCmd)
	}
	return m, statsCmd
}

// handleCommitsLoaded processes the result of loadCommits, populating the
// cached commit log on the model. Drops stale results from earlier in-flight
// loads (triggered by R reload) via the seq tag, mirroring handleFilesLoaded.
// An error is cached the same way as success: loaded is set to true so the
// overlay shows the error once instead of re-triggering the fetch.
//
// When the info popup is already open, refreshes its spec so the
// "loading commits…" placeholder flips to the rendered list inline; the
// popup's scroll offset is preserved across the refresh.
func (m Model) handleCommitsLoaded(msg commitsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.commits.loadSeq {
		return m, nil
	}
	m.commits.list = msg.list
	m.commits.err = msg.err
	m.commits.truncated = msg.truncated
	m.commits.loaded = true
	m.refreshInfoOverlay()
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
	m.file.oldName = msg.oldName
	m.file.lines = msg.lines
	entry := diff.FileEntry{Path: msg.file, OldPath: m.tree.OldPath(msg.file), Status: m.tree.FileStatus(msg.file)}
	fingerprint := diff.FileFingerprint(entry, m.file.lines)
	m.reviewed.cache[msg.file] = fingerprint
	m.tree.ReconcileReviewedPath(msg.file, fingerprint)
	m.invalidateAnnotationRows()
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

	// handle pending compact-toggle anchor: restore the cursor to where it was
	// before the toggle instead of resetting to the top. seq ties the anchor to
	// this exact reload, so a stale anchor from a superseded load is dropped.
	if m.compact.pendingAnchor != nil {
		a := m.compact.pendingAnchor
		m.compact.pendingAnchor = nil
		if a.seq == msg.seq {
			m.applyCompactAnchor(a)
			return m, blameCmd
		}
	}

	m.layout.viewport.SetContent(m.renderDiff())
	m.layout.viewport.GotoTop()
	return m, blameCmd
}

func (m Model) handleReviewFingerprintLoaded(msg reviewFingerprintLoadedMsg) (tea.Model, tea.Cmd) {
	seq, pending := m.reviewed.pending[msg.path]
	if !pending || seq != msg.seq {
		return m, nil
	}
	delete(m.reviewed.pending, msg.path)
	if msg.filesSeq != m.filesLoadSeq {
		return m, nil
	}
	if msg.err != nil {
		log.Printf("[WARN] fingerprint reviewed file %s: %v", msg.path, msg.err)
		return m, nil
	}
	m.reviewed.cache[msg.path] = msg.fingerprint
	m.tree.SetReviewed(msg.path, msg.fingerprint)
	return m.loadSelectedIfChanged()
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
	// flush deferred wheel pin before syncViewportToCursor so the post-blame
	// scroll is anchored to the user's wheeled-to position rather than the
	// pre-burst cursor. mirrors the handleResize flush rationale.
	m.flushWheelPending()
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
	m.file.adds, m.file.removes = diff.CountChanges(m.file.lines)
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
