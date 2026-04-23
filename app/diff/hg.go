package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// hgCommitLogTemplate is the hg log --template used by (*Hg).CommitLog.
// Fields within a record are separated by ASCII US (\x1f); records end with
// ASCII RS (\x1e). These control characters are valid in argv (unlike NUL,
// which exec() rejects) and essentially never appear in commit messages,
// so they work as reliable delimiters without escaping.
const hgCommitLogTemplate = "{node}\x1f{author}\x1f{date|rfc3339date}\x1f{desc}\x1e"

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

// FileDiff returns the diff view for a single file.
// staged flag is ignored (hg has no staging area). contextLines controls surrounding
// context: 0 or >= fullContextSentinel requests full-file context; positive values
// below the sentinel request that many lines on each side of a hunk.
func (h *Hg) FileDiff(ref, file string, _ bool, contextLines int) ([]DiffLine, error) {
	rArgs := h.revFlag("-r", ref)
	args := make([]string, 0, 5+len(rArgs))
	args = append(args, "diff", "--git", "--color=never")
	args = append(args, rArgs...)
	args = append(args, hgContextArg(contextLines), "--", file)

	out, err := h.runHg(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	// trailing divider only matters in compact mode; skip the probe in full-file mode.
	total := 0
	if contextLines > 0 && contextLines < fullContextSentinel {
		total = h.totalOldLines(ref, file)
	}
	return parseUnifiedDiff(out, total)
}

// totalOldLines returns the line count of the pre-change version of file, used by
// parseUnifiedDiff to emit a trailing divider. Returns 0 when the old-side file is
// unavailable — the parser treats 0 as "unknown" and skips the trailing divider.
//
// Old-side resolution (mirrors revFlag):
//   - ref empty              → "." (working-copy parent)
//   - ref contains ".." or "..." → left operand (triple-dot checked first so A...B
//     is not mis-split on the leading "..")
//   - single ref             → use as-is
//
// For triple-dot ranges the left operand is an approximation of the true old side
// (ancestor(A,B)); accurate enough for the informational trailing-divider count.
func (h *Hg) totalOldLines(ref, file string) int {
	oldRef := ref
	if left, _, ok := strings.Cut(ref, "..."); ok {
		oldRef = left
	}
	if left, _, ok := strings.Cut(oldRef, ".."); ok {
		oldRef = left
	}
	oldRef = translateRef(oldRef)
	if oldRef == "" {
		oldRef = "."
	}
	out, err := h.runHg("cat", "-r", oldRef, "--", file)
	if err != nil {
		return 0
	}
	return countLines(out)
}

// hgContextArg returns the -U argument for hg diff given the caller's requested
// context size. A non-positive contextLines or one at or above fullContextSentinel
// returns the full-file arg; any other value returns -U<contextLines>.
func hgContextArg(contextLines int) string {
	if contextLines <= 0 || contextLines >= fullContextSentinel {
		return fullFileContext
	}
	return fmt.Sprintf("-U%d", contextLines)
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

// CommitLog returns commits reachable in the given ref range, newest first.
//
// The ref argument is interpreted as follows:
//   - ""      → returns (nil, nil); there is no range to inspect
//   - "X"     → commits in revset "X::." (X and descendants up to working copy parent)
//   - "X..Y"  → commits in revset "X::Y - X" (reachable from Y but not X)
//   - "X...Y" → symmetric difference "only(X,Y) + only(Y,X)"
//
// Empty sides of a range default to "0" (left) and "." (right) to mirror
// revFlag. Git-style refs (HEAD, HEAD~N, HEAD^) are translated via translateRef.
//
// The result is capped at MaxCommits entries. Callers should treat a result of
// exactly MaxCommits length as potentially truncated and signal that to the
// user via CommitInfoSpec.Truncated.
//
// Author, Subject, and Body are sanitized (ANSI escape sequences, C0/DEL/C1
// control bytes, and VCS framing delimiters stripped) to neutralize terminal
// injection attempts via crafted commit metadata.
func (h *Hg) CommitLog(ref string) ([]CommitInfo, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	revset := h.commitLogRevset(ref)
	out, err := h.runHg("log", "--color=never", "-r", revset, "--template", hgCommitLogTemplate, "-l", strconv.Itoa(MaxCommits))
	if err != nil {
		return nil, fmt.Errorf("commit log: %w", err)
	}
	return h.parseCommitLog(out), nil
}

// commitLogRevset translates a combined ref string to hg's revset syntax for
// commit log queries. See CommitLog for supported forms.
func (h *Hg) commitLogRevset(ref string) string {
	// triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		l, r := h.rangeEnds(left, right)
		return fmt.Sprintf("only(%s,%s) + only(%s,%s)", l, r, r, l)
	}
	if left, right, ok := strings.Cut(ref, ".."); ok {
		l, r := h.rangeEnds(left, right)
		return fmt.Sprintf("%s::%s - %s", l, r, l)
	}
	r := translateRef(ref)
	return r + "::. - " + r
}

// rangeEnds translates both sides of a range expression via translateRef,
// defaulting empty left to "0" (repo root) and empty right to "." (working copy parent).
func (h *Hg) rangeEnds(left, right string) (string, string) {
	l := translateRef(left)
	r := translateRef(right)
	if l == "" {
		l = "0"
	}
	if r == "" {
		r = "."
	}
	return l, r
}

// parseCommitLog parses the raw output of "hg log --template <hgCommitLogTemplate>"
// into a slice of CommitInfo entries. Records end with RS (\x1e); within a
// record fields are US-separated (\x1f) — hash, author, date, desc. The desc
// field holds subject and body joined by a single newline. The slice is capped
// at MaxCommits entries.
func (h *Hg) parseCommitLog(raw string) []CommitInfo {
	if raw == "" {
		return nil
	}
	records := strings.Split(raw, "\x1e")
	commits := make([]CommitInfo, 0, len(records))
	for _, record := range records {
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x1f", 4)
		if len(fields) < 4 {
			continue
		}
		subject, body := splitCommitDesc(fields[3])
		ci := CommitInfo{
			Hash:    fields[0],
			Author:  sanitizeCommitText(fields[1]),
			Subject: sanitizeCommitText(subject),
			Body:    sanitizeCommitText(body),
		}
		if t, err := time.Parse(time.RFC3339, fields[2]); err == nil {
			ci.Date = t
		}
		commits = append(commits, ci)
		if len(commits) >= MaxCommits {
			break
		}
	}
	return commits
}

// runHg executes a mercurial command in the working directory and returns its output.
func (h *Hg) runHg(args ...string) (string, error) {
	return runVCS(h.workDir, "hg", args...)
}
