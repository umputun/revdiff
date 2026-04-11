package diff

import (
	"fmt"
	"regexp"
	"strings"
)

// Hg provides methods to extract changed files and build full-file diff views for Mercurial repos.
type Hg struct {
	workDir string // working directory for hg commands
}

// NewHg creates a new Hg diff renderer rooted at the given working directory.
func NewHg(workDir string) *Hg {
	return &Hg{workDir: workDir}
}

// UntrackedFiles returns untracked files using hg status.
func (h *Hg) UntrackedFiles() ([]string, error) {
	out, err := h.runHg("status", "--no-status", "--unknown")
	if err != nil {
		return nil, err
	}
	var files []string
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// hgStatusRe matches hg status output lines: "M path/to/file" or "? path/to/file".
var hgStatusRe = regexp.MustCompile(`^([MAR?!]) (.+)$`)

// hgStatusToFileStatus converts an hg status letter to a FileStatus.
// hg uses "R" for removed (not renamed), mapping to FileDeleted.
// returns empty string for unknown or skipped statuses.
func (h *Hg) hgStatusToFileStatus(status string) FileStatus {
	switch FileStatus(status) {
	case FileModified:
		return FileModified
	case FileAdded:
		return FileAdded
	case "R": // hg "R" = removed, not renamed
		return FileDeleted
	default:
		return ""
	}
}

// ChangedFiles returns a list of files changed relative to the given ref with their change status.
// if ref is empty, shows uncommitted changes. staged flag is ignored (hg has no staging area).
func (h *Hg) ChangedFiles(ref string, _ bool) ([]FileEntry, error) {
	if ref != "" {
		return h.changedFilesFromDiff(ref)
	}

	// uncommitted changes: use hg status
	out, err := h.runHg("status", "--color=never")
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	return h.parseStatus(out), nil
}

// changedFilesFromDiff lists changed files between revisions using hg status --rev.
func (h *Hg) changedFilesFromDiff(ref string) ([]FileEntry, error) {
	revArgs := h.revFlag("--rev", ref)
	args := make([]string, 0, 2+len(revArgs))
	args = append(args, "status", "--color=never")
	args = append(args, revArgs...)

	out, err := h.runHg(args...)
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	return h.parseStatus(out), nil
}

// parseStatus parses hg status output into file entries.
func (h *Hg) parseStatus(out string) []FileEntry {
	var entries []FileEntry
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		m := hgStatusRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		status, path := m[1], m[2]
		fs := h.hgStatusToFileStatus(status)
		if fs == "" {
			continue
		}
		entries = append(entries, FileEntry{Path: path, Status: fs})
	}
	return entries
}

// revFlag builds revision arguments from a ref string using the given flag name.
// handles triple-dot (A...B) and double-dot (A..B) range refs, producing two separate flags.
func (h *Hg) revFlag(flag, ref string) []string {
	if ref == "" {
		return nil
	}

	// check triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		l := translateRef(left)
		r := translateRef(right)
		if l == "" {
			l = "0"
		}
		if r == "" {
			r = "."
		}
		return []string{flag, fmt.Sprintf("ancestor(%s,%s)", l, r), flag, r}
	}

	if left, right, ok := strings.Cut(ref, ".."); ok {
		l := translateRef(left)
		r := translateRef(right)
		if l == "" {
			l = "0"
		}
		if r == "" {
			r = "."
		}
		return []string{flag, l, flag, r}
	}

	return []string{flag, translateRef(ref)}
}

// FileDiff returns the full-file diff view for a single file.
// staged flag is ignored (hg has no staging area).
func (h *Hg) FileDiff(ref, file string, _ bool) ([]DiffLine, error) {
	rArgs := h.revFlag("-r", ref)
	args := make([]string, 0, 5+len(rArgs))
	args = append(args, "diff", "--git", "--color=never")
	args = append(args, rArgs...)
	args = append(args, fullFileContext, "--", file)

	out, err := h.runHg(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	return parseUnifiedDiff(out)
}

// translateRef converts git-style refs to mercurial revset syntax.
// HEAD -> ".", HEAD~N -> ".~N", HEAD^ -> ".^", HEAD^N (N>1) -> "pN(.)".
func translateRef(ref string) string {
	switch {
	case ref == "HEAD":
		return "."
	case strings.HasPrefix(ref, "HEAD~"):
		return ".~" + ref[5:]
	case ref == "HEAD^" || ref == "HEAD^1":
		return ".^"
	case strings.HasPrefix(ref, "HEAD^"):
		// HEAD^N where N>1 means "Nth parent" — use mercurial pN() function
		return "p" + ref[5:] + "(.)"
	default:
		return ref
	}
}

// runHg executes a mercurial command in the working directory and returns its output.
func (h *Hg) runHg(args ...string) (string, error) {
	return runVCS(h.workDir, "hg", args...)
}
