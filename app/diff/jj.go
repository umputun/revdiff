package diff

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// jjFullContext is jj's equivalent of git's -U1000000 — request full-file diff context.
// jj only accepts --context (not -U).
const jjFullContext = "--context=1000000"

// Jj provides methods to extract changed files and build full-file diff views for Jujutsu repos.
// ref semantics are translated from the git-flavored syntax revdiff accepts:
//   - empty ref          → working-copy vs parent (jj default)
//   - single ref X       → changes from X to the working copy (@)
//   - range A..B / A...B → expanded to --from/--to for jj diff
//
// HEAD and friends are translated to jj revsets (@-, @--, …). Jujutsu auto-tracks
// every file in the working copy, so UntrackedFiles always returns an empty list.
type Jj struct {
	workDir   string   // working directory for jj commands
	extraArgs []string // leading args prepended to every jj invocation (e.g. --config overrides for tests)
}

// NewJj creates a new Jj diff renderer rooted at the given working directory.
func NewJj(workDir string) *Jj {
	return &Jj{workDir: workDir}
}

// UntrackedFiles returns untracked files. Jujutsu auto-snapshots every file in the
// working copy (respecting .gitignore), so there is no "untracked" state — this
// always returns nil.
func (j *Jj) UntrackedFiles() ([]string, error) {
	return nil, nil
}

// jjSummaryRe matches lines from `jj diff --summary`, e.g. "M path/to/file".
// Rename summary lines ("R {old => new}") are handled separately in parseSummary.
var jjSummaryRe = regexp.MustCompile(`^([MAD]) (.+)$`)

// jjRenameRe matches a jj rename summary line: "R {old => new}" or "R prefix/{old => new}/suffix".
var jjRenameRe = regexp.MustCompile(`^R (.+)$`)

// jjRenameBraceRe extracts the "old => new" pair from a rename target, optionally
// surrounded by a static path prefix and suffix.
var jjRenameBraceRe = regexp.MustCompile(`^(.*)\{(.+) => (.+)\}(.*)$`)

// jjStatusToFileStatus maps a jj summary status letter to a FileStatus.
// Renames ("R") are caller-handled — they expand to two entries.
func (j *Jj) jjStatusToFileStatus(status string) FileStatus {
	switch FileStatus(status) {
	case FileModified:
		return FileModified
	case FileAdded:
		return FileAdded
	case FileDeleted:
		return FileDeleted
	default:
		return ""
	}
}

// ChangedFiles lists files changed for the given ref.
// If ref is empty, reports uncommitted working-copy changes. The staged flag is
// ignored — Jujutsu has no staging area.
func (j *Jj) ChangedFiles(ref string, _ bool) ([]FileEntry, error) {
	rangeArgs := j.diffRangeFlags(ref)
	args := make([]string, 0, 2+len(rangeArgs))
	args = append(args, "diff", "--summary")
	args = append(args, rangeArgs...)

	out, err := j.runJj(args...)
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	return j.parseSummary(out), nil
}

// parseSummary parses `jj diff --summary` output into file entries.
// Rename lines expand to a delete + add pair so the rest of the UI can treat
// each side independently (matching hg/git behavior in this codebase).
func (j *Jj) parseSummary(out string) []FileEntry {
	var entries []FileEntry
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		// rename?
		if m := jjRenameRe.FindStringSubmatch(line); m != nil && strings.HasPrefix(line, "R ") {
			if oldPath, newPath, ok := expandJjRename(m[1]); ok {
				entries = append(entries,
					FileEntry{Path: oldPath, Status: FileDeleted},
					FileEntry{Path: newPath, Status: FileAdded},
				)
				continue
			}
		}
		m := jjSummaryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		status, path := m[1], m[2]
		fs := j.jjStatusToFileStatus(status)
		if fs == "" {
			continue
		}
		entries = append(entries, FileEntry{Path: path, Status: fs})
	}
	return entries
}

// expandJjRename decodes a jj rename summary target like "prefix/{old => new}/suffix"
// into the full old and new paths. Returns false when the target is not a brace form.
func expandJjRename(target string) (oldPath, newPath string, ok bool) {
	m := jjRenameBraceRe.FindStringSubmatch(target)
	if m == nil {
		// no braces — jj printed "R old new" or similar; fall back to splitting on " => "
		if left, right, cut := strings.Cut(target, " => "); cut {
			return left, right, true
		}
		return "", "", false
	}
	prefix, oldMid, newMid, suffix := m[1], m[2], m[3], m[4]
	return prefix + oldMid + suffix, prefix + newMid + suffix, true
}

// FileDiff returns the full-file diff view for a single file.
// The staged flag is ignored — Jujutsu has no staging area.
func (j *Jj) FileDiff(ref, file string, _ bool) ([]DiffLine, error) {
	rangeArgs := j.diffRangeFlags(ref)
	args := make([]string, 0, 5+len(rangeArgs))
	args = append(args, "diff", "--git", jjFullContext)
	args = append(args, rangeArgs...)
	args = append(args, "--", file)

	out, err := j.runJj(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	// jj emits raw bytes for binary files instead of git's "Binary files … differ"
	// marker. Detect and rewrite so parseUnifiedDiff produces the binary placeholder.
	normalized := jjSynthesizeBinaryDiff(out)
	return parseUnifiedDiff(normalized)
}

// diffRangeFlags builds the --from/--to arguments from a git-style ref.
// See Jj docs for the mapping from HEAD-style refs to jj revsets.
func (j *Jj) diffRangeFlags(ref string) []string {
	if ref == "" {
		return nil
	}

	// triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		l := translateJjRef(left)
		r := translateJjRef(right)
		if r == "" {
			r = "@"
		}
		if l == "" {
			return []string{"--from", "root()", "--to", r}
		}
		// common ancestor revset: jj's equivalent of git's merge-base
		merge := fmt.Sprintf("ancestors(%s) & ancestors(%s) & ~root()", l, r)
		return []string{"--from", merge, "--to", r}
	}

	if left, right, ok := strings.Cut(ref, ".."); ok {
		l := translateJjRef(left)
		r := translateJjRef(right)
		if l == "" {
			l = "root()"
		}
		if r == "" {
			r = "@"
		}
		return []string{"--from", l, "--to", r}
	}

	return []string{"--from", translateJjRef(ref), "--to", "@"}
}

// translateJjRef converts git-style refs to jj revset syntax.
//   - HEAD       → @-         (parent of working copy; jj's "last committed" equivalent)
//   - HEAD~N     → @ followed by (N+1) dashes
//   - HEAD^      → @-- (same as HEAD~1)
//   - HEAD^1     → @--
//   - HEAD^N>1   → parents(@-) (Nth-parent for merges; we can't target a specific parent cleanly)
//
// Anything else (bookmarks, change/commit IDs, the bare "@" / "@-" forms) passes through unchanged.
func translateJjRef(ref string) string {
	switch {
	case ref == "":
		return ""
	case ref == "HEAD":
		return "@-"
	case strings.HasPrefix(ref, "HEAD~"):
		n := ref[5:]
		count, ok := parseSmallPositive(n)
		if !ok {
			return ref
		}
		return "@" + strings.Repeat("-", count+1)
	case ref == "HEAD^" || ref == "HEAD^1":
		return "@--"
	case strings.HasPrefix(ref, "HEAD^"):
		// HEAD^N for N>1 means "Nth parent" — jj can reach parents via parents(@-),
		// but can't single out an individual parent in one revset step; this is a
		// best-effort approximation.
		return "parents(@-)"
	default:
		return ref
	}
}

// parseSmallPositive parses a non-negative decimal integer. Returns false on
// failure. Used for HEAD~N parsing where N is a small positive integer.
func parseSmallPositive(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n > 1_000_000 {
			return 0, false
		}
	}
	return n, true
}

// jjSynthesizeBinaryDiff replaces the hunk body of a jj diff with a git-style
// "Binary files … differ" marker when the hunk contains NUL bytes. parseUnifiedDiff
// already recognizes the marker and returns a binary placeholder DiffLine.
// Returns the input unchanged when no NUL bytes are present in the hunk body.
func jjSynthesizeBinaryDiff(raw string) string {
	if !bytes.ContainsRune([]byte(raw), 0) {
		return raw
	}

	lines := strings.Split(raw, "\n")
	var oldPath, newPath string
	var headerEnd int
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "--- "):
			oldPath = strings.TrimPrefix(line, "--- ")
		case strings.HasPrefix(line, "+++ "):
			newPath = strings.TrimPrefix(line, "+++ ")
			headerEnd = i + 1 // inclusive of the "+++" line
		}
	}
	if oldPath == "" || newPath == "" || headerEnd == 0 {
		return raw
	}

	marker := fmt.Sprintf("Binary files %s and %s differ", oldPath, newPath)
	return strings.Join(append(lines[:headerEnd], marker), "\n") + "\n"
}

// runJj executes a jj command in the working directory and returns its output.
// Prepends extraArgs (e.g. --no-pager) then delegates to the shared runVCS helper.
func (j *Jj) runJj(args ...string) (string, error) {
	full := make([]string, 0, len(j.extraArgs)+len(args))
	full = append(full, j.extraArgs...)
	full = append(full, args...)
	return runVCS(j.workDir, "jj", full...)
}
