// Package review owns review-info computation that does not belong inside
// app/ui: aggregating per-file diff stats, falling back to the index for
// added-but-still-tracked files, and reading untracked content off disk
// under a path-safety guard. The UI consumes Stats via a tea.Cmd and only
// renders the result; the diff/filesystem calls live here.
package review

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/umputun/revdiff/app/diff"
)

// maxUntrackedBytes bounds the per-file size for untracked-content reads
// during stats computation. The popup only needs +/- counts, so loading a
// multi-GB log/dump that happens to be untracked would balloon memory and
// block the goroutine for no user-visible payoff. Files exceeding the cap
// are skipped and the stats result is marked partial.
const maxUntrackedBytes = 16 * 1024 * 1024

// FileDiffer is the minimal subset of a diff renderer that stats computation
// needs. Defined consumer-side so tests can supply mocks without importing
// the full ui.Renderer surface.
type FileDiffer interface {
	FileDiff(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error)
}

// Stats holds the aggregate add/remove counts for a review and whether one
// or more per-file fallbacks failed. Err is set when a primary FileDiff call
// returned an error; the caller stops aggregating in that case so the UI can
// surface the message instead of reporting silently-zero totals.
type Stats struct {
	Adds    int
	Removes int
	Partial bool
	Err     error
}

// StatsRequest carries the inputs ComputeStats needs to walk a review's file
// list and aggregate +/- counts. Bundled into a struct because the prior
// 5-positional shape (differ, ref, staged, workDir, entries) hit the
// project's "4+ params → option struct" rule and made the call site harder
// to skim.
type StatsRequest struct {
	Differ  FileDiffer
	Ref     string
	Staged  bool
	WorkDir string
	Entries []diff.FileEntry
}

// workDirRoots holds the original workDir alongside its symlink-resolved
// twin so callers in hot loops resolve EvalSymlinks(workDir) once and pass
// the pair around. Bundled as a struct because the "two same-typed paths"
// shape was the silent-swap risk flagged in review.
type workDirRoots struct {
	workDir     string // original (un-resolved) workDir
	realWorkDir string // filepath.EvalSymlinks(workDir); "" when workDir is empty or unresolvable
}

// resolveWorkDir returns filepath.EvalSymlinks(workDir) or "" when workDir
// is empty or cannot be resolved. Called once per ComputeStats invocation
// (hot loops never call this — see workDirRoots) and from tests that exercise
// safeWorkDirPath through the same call shape ComputeStats uses.
func resolveWorkDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	r, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return ""
	}
	return r
}

// ComputeStats walks req.Entries and aggregates +/- lines. It mirrors what
// the VCS produces for the popup's footer summary, including two fallbacks
// the renderers cannot do internally:
//   - added files whose primary diff is empty (the change is staged-only):
//     re-fetch with staged=true so the index content is counted.
//   - untracked files: read from disk via diff.ReadFileAsAdded, gated by
//     safeWorkDirPath so symlinks inside workDir cannot redirect the read
//     outside the tree.
//
// On the first FileDiff error, computation stops and Err is returned so the
// UI can render "stats unavailable" rather than reporting partial totals as
// if they were complete.
func ComputeStats(req StatsRequest) Stats {
	var stats Stats
	// Resolve workDir symlinks once instead of per untracked entry; each call
	// to EvalSymlinks walks every path component, so hoisting this out of the
	// loop is the difference between O(N) and O(1) FS work for the workDir
	// side of the path-safety check.
	roots := workDirRoots{workDir: req.WorkDir, realWorkDir: resolveWorkDir(req.WorkDir)}
	// contextLines=0 requests full-file context, which skips the per-file
	// totalOldLines probe inside the VCS renderers — that probe fires a
	// separate `git show <ref>:<file>` (or hg/jj equivalent) per file solely to
	// emit the trailing divider, and stats only consumes diff.CountChanges so
	// the divider would be discarded anyway.
	for _, e := range req.Entries {
		if e.Status == diff.FileUntracked {
			lines, partial := readUntracked(roots, e.Path, stats.Partial)
			stats.Partial = partial
			adds, removes := diff.CountChanges(lines)
			stats.Adds += adds
			stats.Removes += removes
			continue
		}
		lines, err := req.Differ.FileDiff(req.Ref, e.Path, req.Staged, 0)
		if err != nil {
			stats.Err = err
			return stats
		}
		if len(lines) == 0 && !req.Staged && e.Status == diff.FileAdded {
			cached, cachedErr := req.Differ.FileDiff(req.Ref, e.Path, true, 0)
			switch {
			case cachedErr != nil:
				stats.Partial = true
			case len(cached) > 0:
				lines = cached
			}
		}
		adds, removes := diff.CountChanges(lines)
		stats.Adds += adds
		stats.Removes += removes
	}
	return stats
}

// readUntracked reads an untracked file off disk for stats accounting.
// Returns the read lines (or nil) and a partial flag combining the prior
// partial state with any failure from the path-safety check or the read
// itself. Wrapper around safeWorkDirPath + diff.ReadFileAsAdded; isolates
// the fallback-failure bookkeeping to keep ComputeStats's outer loop flat.
//
// Non-regular files (FIFOs, devices, sockets, directories that survived the
// VCS listing) and oversized files are skipped and counted as partial — the
// reader cannot trust Size() on a non-regular path, and reading one would
// either block or balloon memory. Same goes for the binary/placeholder
// path inside diff.ReadFileAsAdded: a successful return with a single
// placeholder line still maps to "+0/-0", which would silently mark the
// file as fully accounted; flagging Partial here keeps the footer honest.
func readUntracked(roots workDirRoots, relPath string, prevPartial bool) ([]diff.DiffLine, bool) {
	path, ok := safeWorkDirPath(roots.workDir, roots.realWorkDir, relPath)
	if !ok {
		return nil, true
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, true
	}
	if !info.Mode().IsRegular() {
		return nil, true
	}
	if info.Size() > maxUntrackedBytes {
		return nil, true
	}
	lines, err := diff.ReadFileAsAdded(path)
	if err != nil {
		return nil, true
	}
	if isBinaryPlaceholderLines(lines) {
		// the file is binary or otherwise unreadable as text; ReadFileAsAdded
		// emits a placeholder but no add/remove signal. Counting it as zero
		// would silently drop it from the footer's total — flag partial.
		return nil, true
	}
	return lines, prevPartial
}

// isBinaryPlaceholderLines reports whether ReadFileAsAdded returned a
// single placeholder row (binary file, broken symlink, non-regular target,
// over-long line). Such rows carry IsBinary or IsPlaceholder; the row keeps
// its ChangeContext type so it contributes 0/0 to CountChanges, which would
// silently mark the file as fully accounted. Flagging Partial here keeps
// the footer honest.
func isBinaryPlaceholderLines(lines []diff.DiffLine) bool {
	if len(lines) != 1 {
		return false
	}
	return lines[0].IsBinary || lines[0].IsPlaceholder
}

// safeWorkDirPath joins workDir + relPath and confirms the result stays under
// workDir. realWorkDir must be a pre-resolved filepath.EvalSymlinks(workDir);
// callers in hot loops resolve once and pass the result here so each entry
// pays only the per-file EvalSymlinks(full) cost. relPath usually comes from
// VCS output (trusted) but defense-in-depth rejects:
//   - empty workDir or empty realWorkDir (caller should skip the read)
//   - absolute relPath (would escape workDir or, on Windows, hit a UNC path)
//   - lexical ".." escapes
//   - symlink escapes — realWorkDir + EvalSymlinks(full) feed the rel
//     comparison, so a symlink inside workDir does not redirect the read at a
//     target outside the tree, given a non-racing filesystem state
//
// This is a best-effort lexical+symlink check, not a TOCTOU-proof guard.
// safeWorkDirPath returns the pre-resolution full path; the caller later
// re-opens it in os.Stat / diff.ReadFileAsAdded, so a local attacker who can
// swap a path component (or a symlink in the chain) between this validation
// and the read can still redirect that subsequent open. The threat model
// here is "untrusted VCS listing under a trusted working tree", not "hostile
// filesystem racing the reviewer process"; the fd-handoff hardening required
// to close that race would push the implementation onto platform-specific
// openat/O_NOFOLLOW APIs and is intentionally out of scope.
//
// EvalSymlinks fails when the target does not exist; that returns ("", false)
// rather than falling back to a lexical comparison. Untracked files come from
// VCS listings, so a missing path is either a race or a hostile entry — in
// both cases skipping the read and marking stats partial is the right call.
func safeWorkDirPath(workDir, realWorkDir, relPath string) (string, bool) {
	if workDir == "" || realWorkDir == "" || filepath.IsAbs(relPath) {
		return "", false
	}
	full := filepath.Join(workDir, relPath)
	realFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(realWorkDir, realFull)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}
