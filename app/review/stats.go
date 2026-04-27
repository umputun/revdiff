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

// ComputeStats walks entries and aggregates +/- lines. It mirrors what the
// VCS produces for the popup's footer summary, including two fallbacks the
// renderers cannot do internally:
//   - added files whose primary diff is empty (the change is staged-only):
//     re-fetch with staged=true so the index content is counted.
//   - untracked files: read from disk via diff.ReadFileAsAdded, gated by
//     safeWorkDirPath so symlinks inside workDir cannot redirect the read
//     outside the tree.
//
// On the first FileDiff error, computation stops and Err is returned so the
// UI can render "stats unavailable" rather than reporting partial totals as
// if they were complete.
func ComputeStats(differ FileDiffer, ref string, staged bool, workDir string, entries []diff.FileEntry) Stats {
	var stats Stats
	// Resolve workDir symlinks once instead of per untracked entry; each call
	// to EvalSymlinks walks every path component, so hoisting this out of the
	// loop is the difference between O(N) and O(1) FS work for the workDir
	// side of the path-safety check.
	realWorkDir := ""
	if workDir != "" {
		if rwd, err := filepath.EvalSymlinks(workDir); err == nil {
			realWorkDir = rwd
		}
	}
	// contextLines=0 requests full-file context, which skips the per-file
	// totalOldLines probe inside the VCS renderers — that probe fires a
	// separate `git show <ref>:<file>` (or hg/jj equivalent) per file solely to
	// emit the trailing divider, and stats only consumes diff.CountChanges so
	// the divider would be discarded anyway.
	for _, e := range entries {
		lines, err := differ.FileDiff(ref, e.Path, staged, 0)
		if err != nil {
			stats.Err = err
			return stats
		}
		if len(lines) == 0 && !staged && e.Status == diff.FileAdded {
			cached, cachedErr := differ.FileDiff(ref, e.Path, true, 0)
			switch {
			case cachedErr != nil:
				stats.Partial = true
			case len(cached) > 0:
				lines = cached
			}
		}
		if len(lines) == 0 && e.Status == diff.FileUntracked {
			lines, stats.Partial = readUntracked(workDir, realWorkDir, e.Path, stats.Partial)
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
// itself. Wrapper around safeWorkDirPathResolved + diff.ReadFileAsAdded;
// isolates the fallback-failure bookkeeping to keep ComputeStats's outer
// loop flat.
func readUntracked(workDir, realWorkDir, relPath string, prevPartial bool) ([]diff.DiffLine, bool) {
	path, ok := safeWorkDirPath(workDir, realWorkDir, relPath)
	if !ok {
		return nil, true
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxUntrackedBytes {
		return nil, true
	}
	lines, err := diff.ReadFileAsAdded(path)
	if err != nil {
		return nil, true
	}
	return lines, prevPartial
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

