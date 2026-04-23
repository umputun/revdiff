package diff

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// jjFullContext is jj's equivalent of git's -U1000000 — request full-file diff context.
// jj only accepts --context (not -U). Use jjContextArg at call sites to choose
// between full-file and small-context based on the caller's contextLines value.
const jjFullContext = "--context=1000000"

// jjCommitLogTemplate is the jj log --template expression used by (*Jj).CommitLog.
// Fields within a record are NUL-separated; records end with "\x00\x01" so the
// parser can split on \x01 and trim the trailing \x00 (mirrors hg's convention).
// Uses commit_id (git-style hash, stable for colocated repos) rather than change_id.
// The fourth field is "1"/"0" for current_working_copy — used to filter the synthetic
// empty @ placeholder without dropping real commits that happen to have no description.
const jjCommitLogTemplate = `commit_id ++ "\x00" ++ author ++ "\x00" ++ committer.timestamp().format("%Y-%m-%dT%H:%M:%S%:z") ++ "\x00" ++ if(current_working_copy, "1", "0") ++ "\x00" ++ description ++ "\x00\x01"`

// Jj provides methods to extract changed files and build full-file diff views for Jujutsu repos.
// ref semantics are translated from the git-flavored syntax revdiff accepts:
//   - empty ref          → working-copy vs parent (jj default)
//   - single ref X       → changes from X to the working copy (@)
//   - range A..B / A...B → expanded to --from/--to for jj diff
//
// HEAD and friends are translated to jj revsets (@-, @--, …). Jujutsu auto-tracks
// every file in the working copy, so UntrackedFiles always returns an empty list.
type Jj struct {
	workDir string // working directory for jj commands
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

// statusToFileStatus maps a jj summary status letter to a FileStatus.
// Renames ("R") are caller-handled — they expand to two entries.
func (j *Jj) statusToFileStatus(status string) FileStatus {
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
			if oldPath, newPath, ok := j.expandRename(m[1]); ok {
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
		fs := j.statusToFileStatus(status)
		if fs == "" {
			continue
		}
		entries = append(entries, FileEntry{Path: path, Status: fs})
	}
	return entries
}

// expandRename decodes a jj rename summary target like "prefix/{old => new}/suffix"
// into the full old and new paths. Returns false when the target is not a brace form.
func (j *Jj) expandRename(target string) (oldPath, newPath string, ok bool) {
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

// FileDiff returns the diff view for a single file.
// The staged flag is ignored — Jujutsu has no staging area. contextLines controls
// surrounding context: 0 or >= fullContextSentinel requests full-file context;
// positive values below the sentinel request that many lines on each side of a hunk.
func (j *Jj) FileDiff(ref, file string, _ bool, contextLines int) ([]DiffLine, error) {
	rangeArgs := j.diffRangeFlags(ref)
	args := make([]string, 0, 5+len(rangeArgs))
	args = append(args, "diff", "--git", jjContextArg(contextLines))
	args = append(args, rangeArgs...)
	args = append(args, "--", file)

	out, err := j.runJj(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	// jj emits raw bytes for binary files instead of git's "Binary files … differ"
	// marker. Detect and rewrite so parseUnifiedDiff produces the binary placeholder.
	normalized := j.synthesizeBinaryDiff(out)

	// trailing divider only matters in compact mode; skip the probe in full-file mode.
	total := 0
	if contextLines > 0 && contextLines < fullContextSentinel {
		total = j.totalOldLines(ref, file)
	}
	return parseUnifiedDiff(normalized, total)
}

// totalOldLines returns the line count of the pre-change version of file, used by
// parseUnifiedDiff to emit a trailing divider. Returns 0 when the old-side file is
// unavailable — the parser treats 0 as "unknown" and skips the trailing divider.
//
// Old-side resolution (mirrors diffRangeFlags):
//   - ref empty              → "@-" (parent of the working-copy commit)
//   - ref contains ".." or "..." → left operand (triple-dot checked first so A...B
//     is not mis-split on the leading "..")
//   - single ref             → use as-is
//
// For triple-dot ranges the left operand is an approximation of the true old side
// (the jj revset ancestors(A) & ancestors(B)); accurate enough for the informational
// trailing-divider count.
func (j *Jj) totalOldLines(ref, file string) int {
	oldRef := ref
	if left, _, ok := strings.Cut(ref, "..."); ok {
		oldRef = left
	}
	if left, _, ok := strings.Cut(oldRef, ".."); ok {
		oldRef = left
	}
	oldRef = j.translateRef(oldRef)
	if oldRef == "" {
		oldRef = "@-"
	}
	out, err := j.runJj("file", "show", "-r", oldRef, "--", file)
	if err != nil {
		return 0
	}
	return countLines(out)
}

// jjContextArg returns the --context argument for jj diff given the caller's
// requested context size. A non-positive contextLines or one at or above
// fullContextSentinel returns the full-file arg; any other value returns
// --context=<contextLines>.
func jjContextArg(contextLines int) string {
	if contextLines <= 0 || contextLines >= fullContextSentinel {
		return jjFullContext
	}
	return fmt.Sprintf("--context=%d", contextLines)
}

// diffRangeFlags builds the --from/--to arguments from a git-style ref.
// See Jj docs for the mapping from HEAD-style refs to jj revsets.
func (j *Jj) diffRangeFlags(ref string) []string {
	if ref == "" {
		return nil
	}

	// triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		l := j.translateRef(left)
		r := j.translateRef(right)
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
		l := j.translateRef(left)
		r := j.translateRef(right)
		if l == "" {
			l = "root()"
		}
		if r == "" {
			r = "@"
		}
		return []string{"--from", l, "--to", r}
	}

	return []string{"--from", j.translateRef(ref), "--to", "@"}
}

// translateRef converts git-style refs to jj revset syntax.
//   - HEAD       → @-         (parent of working copy; jj's "last committed" equivalent)
//   - HEAD~N     → @ followed by (N+1) dashes
//   - HEAD^      → @-- (same as HEAD~1)
//   - HEAD^1     → @--
//   - HEAD^N>1   → parents(@-) (Nth-parent for merges; we can't target a specific parent cleanly)
//
// Anything else (bookmarks, change/commit IDs, the bare "@" / "@-" forms) passes through unchanged.
func (j *Jj) translateRef(ref string) string {
	switch {
	case ref == "":
		return ""
	case ref == "HEAD":
		return "@-"
	case strings.HasPrefix(ref, "HEAD~"):
		n := ref[5:]
		count, ok := j.parseSmallPositive(n)
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

// CommitLog returns commits reachable in the given ref range, newest first.
//
// The ref argument is interpreted as follows:
//   - ""       → returns (nil, nil); there is no range to inspect
//   - "X"      → commits in revset "X..@" (exclusive of X, up to the working copy)
//   - "X..Y"   → commits in revset "X..Y" (passed through after ref translation)
//   - "X...Y"  → symmetric difference "(X..Y) | (Y..X)"
//
// Empty sides of a range default to "root()" (left) and "@" (right). Git-style
// refs (HEAD, HEAD~N, HEAD^) are translated via translateRef.
//
// The result is capped at MaxCommits entries. Callers should treat a result of
// exactly MaxCommits length as potentially truncated and signal that to the
// user via CommitInfoSpec.Truncated.
//
// jj is queried with `-n MaxCommits+1` to absorb the synthetic working-copy @
// placeholder that ranges like X..@ include. parseCommitLog drops that
// placeholder and still caps real commits at MaxCommits, so the caller's
// "len == MaxCommits means truncated" contract stays correct even when the
// placeholder is present.
//
// Author, Subject, and Body are sanitized (ANSI escape sequences, C0/DEL/C1
// control bytes, and VCS framing delimiters stripped) to neutralize terminal
// injection attempts via crafted commit metadata.
func (j *Jj) CommitLog(ref string) ([]CommitInfo, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	revset := j.commitLogRevset(ref)
	out, err := j.runJj("log", "--no-graph", "--no-pager", "--color=never",
		"-T", jjCommitLogTemplate, "-r", revset, "-n", strconv.Itoa(MaxCommits+1))
	if err != nil {
		return nil, fmt.Errorf("commit log: %w", err)
	}
	return j.parseCommitLog(out), nil
}

// commitLogRevset translates a combined ref string to jj's revset syntax for
// commit log queries. See CommitLog for supported forms.
func (j *Jj) commitLogRevset(ref string) string {
	// triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		l, r := j.rangeEnds(left, right)
		return fmt.Sprintf("(%s..%s) | (%s..%s)", l, r, r, l)
	}
	if left, right, ok := strings.Cut(ref, ".."); ok {
		l, r := j.rangeEnds(left, right)
		return fmt.Sprintf("%s..%s", l, r)
	}
	return j.translateRef(ref) + "..@"
}

// rangeEnds translates both sides of a range expression via translateRef,
// defaulting empty left to "root()" and empty right to "@" (working copy).
func (j *Jj) rangeEnds(left, right string) (string, string) {
	l := j.translateRef(left)
	r := j.translateRef(right)
	if l == "" {
		l = "root()"
	}
	if r == "" {
		r = "@"
	}
	return l, r
}

// parseCommitLog parses the raw output of "jj log --no-graph -T <jjCommitLogTemplate>"
// into a slice of CommitInfo entries. Records end with "\x00\x01"; within a
// record fields are NUL-separated (hash, author, date, current_working_copy flag,
// description). The description field holds subject and body joined by a single
// newline and includes a trailing newline that jj appends. The slice is capped
// at MaxCommits entries.
//
// The synthetic working-copy @ placeholder is skipped when its description is
// empty. jj auto-creates a new empty commit at @ every time you run `jj new`,
// and range queries like X..@ will include that placeholder. Real commits with
// empty descriptions (jj permits them) are kept so the popup reflects the range
// accurately.
func (j *Jj) parseCommitLog(raw string) []CommitInfo {
	if raw == "" {
		return nil
	}
	records := strings.Split(raw, "\x01")
	commits := make([]CommitInfo, 0, len(records))
	for _, record := range records {
		// strip exactly one trailing "\x00" (the in-record separator that precedes
		// the record terminator "\x01"). TrimRight would also eat an empty
		// description's placeholder "\x00", collapsing 5 fields to 4.
		record = strings.TrimSuffix(record, "\x00")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 5)
		if len(fields) < 5 {
			continue
		}
		isWorkingCopy := fields[3] == "1"
		desc := strings.TrimRight(fields[4], "\n")
		subject, body := splitCommitDesc(desc)
		subject = sanitizeCommitText(subject)
		body = sanitizeCommitText(body)
		if isWorkingCopy && subject == "" && body == "" {
			// synthetic working-copy @ placeholder created by `jj new` — see godoc
			continue
		}
		ci := CommitInfo{
			Hash:    fields[0],
			Author:  sanitizeCommitText(fields[1]),
			Subject: subject,
			Body:    body,
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

// parseSmallPositive parses a non-negative decimal integer. Returns false on
// failure. Used for HEAD~N parsing where N is a small positive integer.
func (j *Jj) parseSmallPositive(s string) (int, bool) {
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

// synthesizeBinaryDiff replaces the hunk body of a jj diff with a git-style
// "Binary files … differ" marker when the hunk contains NUL bytes. parseUnifiedDiff
// already recognizes the marker and returns a binary placeholder DiffLine.
// Returns the input unchanged when no NUL bytes are present in the hunk body.
func (j *Jj) synthesizeBinaryDiff(raw string) string {
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
func (j *Jj) runJj(args ...string) (string, error) {
	return runVCS(j.workDir, "jj", args...)
}
