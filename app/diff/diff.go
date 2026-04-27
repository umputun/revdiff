package diff

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ChangeType represents the type of change for a diff line.
type ChangeType string

const (
	ChangeAdd     ChangeType = "+"
	ChangeRemove  ChangeType = "-"
	ChangeContext ChangeType = " "
	ChangeDivider ChangeType = "~" // marks a skipped unchanged region (leading, between-hunk, or trailing)

	// fullContextSentinel is the numeric threshold that callers use to request
	// full-file diff context. contextLines <= 0 or >= fullContextSentinel causes
	// the per-VCS *ContextArg helpers to return the full-file argument form.
	fullContextSentinel = 1000000

	// fullFileContext is the -U value treated as "give me the full file"; use
	// gitContextArg / hgContextArg helpers at call sites to choose between full-file
	// and small-context based on the caller's contextLines value.
	fullFileContext = "-U1000000"

	// MaxLineLength is the maximum line length (in bytes) that scanners will accept.
	// used by parseUnifiedDiff, readReaderAsContext, and parseBlame.
	MaxLineLength = 1024 * 1024

	// BinaryPlaceholder is the content used for binary file placeholders.
	// parseUnifiedDiff returns this when git reports "Binary files ... differ".
	BinaryPlaceholder = "(binary file)"
)

// DiffLine holds parsed line info from a diff.
type DiffLine struct {
	OldNum        int        // line number in old version (0 for additions)
	NewNum        int        // line number in new version (0 for removals)
	Content       string     // line content without the +/- prefix; for ChangeDivider rows it is a human-readable "⋯ N line[s] ⋯" label — never pattern-match it, dispatch on ChangeType
	ChangeType    ChangeType // changeAdd, ChangeRemove, ChangeContext, or ChangeDivider
	IsBinary      bool       // true when this line is a binary file placeholder
	IsPlaceholder bool       // true for non-content placeholders (broken symlink, non-regular file, too-long lines)
}

// FileStatus represents the change type of a file in a VCS diff.
type FileStatus string

const (
	FileAdded     FileStatus = "A"
	FileModified  FileStatus = "M"
	FileDeleted   FileStatus = "D"
	FileRenamed   FileStatus = "R"
	FileUntracked FileStatus = "?"
)

// FileEntry represents a file with its change status from a VCS diff.
type FileEntry struct {
	Path   string     // file path relative to repo root
	Status FileStatus // file change status, empty for non-git renderers
}

// FileEntryPaths extracts just the paths from a slice of FileEntry.
func FileEntryPaths(entries []FileEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths
}

// MaxCommits is the hard cap on the number of commits returned by any CommitLogger
// implementation. Callers should treat a result of exactly MaxCommits entries as
// potentially truncated and surface that to the user (e.g. via CommitInfoSpec.Truncated).
const MaxCommits = 500

// CommitInfo holds metadata and message fields for a single commit in a ref range.
// Author, Subject, and Body are pre-sanitized by sanitizeCommitText so overlay
// renderers can treat them as literal text without a second sanitization pass.
type CommitInfo struct {
	Hash    string    // full hash or VCS change id
	Author  string    // "Name <email>" or VCS equivalent
	Date    time.Time // committer date (RFC3339-parsed)
	Subject string    // first line of the commit message
	Body    string    // remainder of the message with trailing blank lines trimmed (may be empty)
}

// CommitLogger is an optional capability interface implemented by VCS renderers
// (Git, Hg, Jj) that can enumerate commits in a ref range. It is deliberately
// separate from Renderer so consumers type-assert for the capability and gracefully
// fall back when unavailable (e.g. FileReader, DirectoryReader).
//
// The ref argument follows revdiff's combined-ref convention produced by
// options.ref(): "" means no range is selected, "X" is the single ref form,
// and "X..Y" is the explicit range form. Each implementation translates the
// string to its native log syntax.
type CommitLogger interface {
	CommitLog(ref string) ([]CommitInfo, error)
}

// commitLogFormat is the git-log --format template used by (*Git).CommitLog.
// Fields inside a record are separated by ASCII US (\x1f); records are
// NUL-separated via -z. Subject and body are joined by a newline inside the
// final field so the parser's SplitN naturally absorbs any control bytes
// (including \x1f) that a crafted commit message might embed — splitCommitDesc
// then separates subject and body on the first newline, matching hg/jj.
const commitLogFormat = "%H%x1f%an <%ae>%x1f%cI%x1f%s%n%b"

// ansiCSIRe matches complete ANSI CSI escape sequences (ESC [ ... final-byte).
// Used by sanitizeCommitText to neutralize ANSI injection via crafted commit messages.
//
//nolint:gocritic // explicit ASCII 0x20..0x2F range for CSI intermediate bytes
var ansiCSIRe = regexp.MustCompile("\x1b\\[[0-9;?]*[\x20-\x2f]*[a-zA-Z~]")

// SanitizeCommitText is the exported alias for sanitizeCommitText. Used by
// callers outside the diff package (e.g. preloaded annotation comments) that
// need the same control-byte / ANSI / C1 stripping applied before content
// reaches a terminal renderer.
func SanitizeCommitText(s string) string { return sanitizeCommitText(s) }

// sanitizeCommitText neutralizes bytes that could trigger terminal side effects
// when a crafted commit Author/Subject/Body is rendered verbatim inside the
// overlay. Used by VCS CommitLog parsers so the overlay renderer can treat the
// fields as literal text without a second pass.
//
// Strips:
//   - ANSI CSI sequences (ESC [ ... final-byte) and stray ESC bytes
//   - C0 control bytes (except TAB and LF) — BEL, BS, VT, FF, CR, SO/SI, DEL.
//     CR is stripped because a raw \r moves the terminal cursor to column 0 and
//     lets a crafted Author/Subject overwrite earlier text on the same rendered
//     line (hash, meta, leading subject chars).
//   - VCS framing delimiters (US 0x1f, RS 0x1e, NUL) that slipped into a field
//     through delimiter injection in upstream data
//   - C1 control code points (U+0080–U+009F) which some terminals interpret as
//     8-bit equivalents of ESC sequences (notably 0x9b = single-byte CSI,
//     0x9d = single-byte OSC)
//   - invalid UTF-8 bytes. Raw bytes such as 0x9b/0x9d are not valid starts of
//     a UTF-8 sequence on their own, so a bare `for _, r := range s` would
//     decode them as utf8.RuneError (U+FFFD) and let them pass; byte-level
//     scanning closes that hole so 8-bit CSI/OSC injection cannot survive when
//     a commit message carries arbitrary non-UTF-8 bytes.
//
// Preserves printable runes, TAB, and LF — scanning is done via
// utf8.DecodeRuneInString so valid UTF-8 multi-byte sequences (CJK, emoji)
// pass through unchanged even when their continuation bytes fall in the C1
// byte range.
func sanitizeCommitText(s string) string {
	if !hasUnsafeContent(s) {
		return s
	}
	if strings.ContainsRune(s, 0x1b) {
		s = ansiCSIRe.ReplaceAllString(s, "")
	}
	if !hasUnsafeContent(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// invalid UTF-8 byte — drop it so raw 0x9b/0x9d (and other
			// stray bytes) cannot reach the terminal.
			i++
			continue
		}
		if isUnsafeRune(r) {
			i += size
			continue
		}
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// hasUnsafeContent is a fast-path check used by sanitizeCommitText to skip the
// full rune scan when s contains no ESC byte, no unsafe rune, and no invalid
// UTF-8 byte.
func hasUnsafeContent(s string) bool {
	if strings.ContainsRune(s, 0x1b) {
		return true
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return true
		}
		if isUnsafeRune(r) {
			return true
		}
		i += size
	}
	return false
}

// isUnsafeRune reports whether r should be dropped from commit metadata before
// rendering. See sanitizeCommitText for the full rationale; briefly: TAB, LF,
// and printable runes are kept, everything else in the C0/DEL/C1 ranges
// (including CR) is dropped.
func isUnsafeRune(r rune) bool {
	switch {
	case r == 0x09, r == 0x0a:
		return false
	case r < 0x20:
		return true
	case r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		return true
	default:
		return false
	}
}

// splitCommitDesc splits a VCS commit description value into subject and body
// on the first newline. A single leading blank line (the conventional
// subject/body separator) is stripped, and trailing blank lines in the body
// are trimmed — matching git's %s/%b split so all VCS backends expose a
// uniform subject/body pair to the overlay renderer.
func splitCommitDesc(desc string) (subject, body string) {
	subject, rest, found := strings.Cut(desc, "\n")
	if !found {
		return desc, ""
	}
	rest = strings.TrimPrefix(rest, "\n")
	return subject, strings.TrimRight(rest, "\n")
}

// Git provides methods to extract changed files and build full-file diff views.
type Git struct {
	workDir string // working directory for git commands
}

// NewGit creates a new Git diff renderer rooted at the given working directory.
func NewGit(workDir string) *Git {
	return &Git{workDir: workDir}
}

// CommitLog returns commits reachable in the given ref range, newest first.
//
// The ref argument is interpreted as follows:
//   - ""      → returns (nil, nil); there is no range to inspect
//   - "X"     → commits in "X..HEAD"
//   - "X..Y"  → passed through unchanged
//
// The result is capped at MaxCommits entries. Callers should treat a result
// of exactly MaxCommits length as potentially truncated and signal that to
// the user via CommitInfoSpec.Truncated.
//
// Author, Subject, and Body are sanitized (ANSI escape sequences, C0/DEL/C1
// control bytes, and VCS framing delimiters stripped) to neutralize terminal
// injection attempts via crafted commit metadata.
func (g *Git) CommitLog(ref string) ([]CommitInfo, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	args := []string{
		"log", "--no-color", "-z",
		"--format=" + commitLogFormat,
		"-n", strconv.Itoa(MaxCommits),
		g.commitLogRange(ref),
	}
	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("commit log: %w", err)
	}
	return g.parseCommitLog(out), nil
}

// commitLogRange translates a combined ref string to git's log range syntax.
// Single ref "X" becomes "X..HEAD"; "X..Y" passes through.
func (g *Git) commitLogRange(ref string) string {
	if strings.Contains(ref, "..") {
		return ref
	}
	return ref + "..HEAD"
}

// parseCommitLog parses the raw output of "git log -z --format=<commitLogFormat>"
// into a slice of CommitInfo entries. Records are NUL-separated; within a record
// fields are ASCII-US-separated (hash, author, date, desc) and desc holds subject
// and body joined by a newline. The slice is capped at MaxCommits entries.
func (g *Git) parseCommitLog(raw string) []CommitInfo {
	raw = strings.TrimRight(raw, "\x00")
	if raw == "" {
		return nil
	}
	records := strings.Split(raw, "\x00")
	commits := make([]CommitInfo, 0, len(records))
	for _, record := range records {
		// leading newline can appear between NUL-terminated records after a body
		record = strings.TrimLeft(record, "\n")
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

// UntrackedFiles returns untracked files (not in .gitignore) using git ls-files --others --exclude-standard.
func (g *Git) UntrackedFiles() ([]string, error) {
	out, err := g.runGit("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var files []string
	for entry := range strings.SplitSeq(out, "\x00") {
		if entry != "" {
			files = append(files, entry)
		}
	}
	return files, nil
}

// ChangedFiles returns a list of files changed relative to the given ref with their change status.
// If ref is empty, it shows uncommitted changes. If staged is true, shows only staged changes.
// Uses -z for NUL-terminated output to handle filenames with special characters.
func (g *Git) ChangedFiles(ref string, staged bool) ([]FileEntry, error) {
	args := g.diffArgs(ref, staged)
	args = append(args, "--name-status", "-z")

	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	var entries []FileEntry
	fields := strings.Split(strings.TrimRight(out, "\x00"), "\x00")
	for i := 0; i < len(fields); {
		rawStatus := fields[i]
		if rawStatus == "" {
			i++
			continue
		}
		i++
		if i >= len(fields) {
			break
		}
		path := fields[i]
		i++
		// for renames/copies (R100, C100), consume two paths, use the new name
		if rawStatus[0] == 'R' || rawStatus[0] == 'C' {
			if i < len(fields) {
				path = fields[i]
				i++
			}
		}
		// normalize status to single letter (R100 -> R)
		if len(rawStatus) > 1 {
			rawStatus = rawStatus[:1]
		}
		entries = append(entries, FileEntry{Path: path, Status: FileStatus(rawStatus)})
	}
	return entries, nil
}

// FileDiff returns the diff view for a single file.
// The result is a sequence of DiffLine entries representing unchanged, added, and removed lines
// interleaved at their correct positions.
// For binary files, it returns a single placeholder line with size delta information.
// contextLines controls surrounding context: 0 or >= fullContextSentinel requests full-file
// context; positive values below the sentinel request that many lines on each side of a hunk.
func (g *Git) FileDiff(ref, file string, staged bool, contextLines int) ([]DiffLine, error) {
	args := g.diffArgs(ref, staged)
	args = append(args, gitContextArg(contextLines), "--", file)

	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	// trailing divider is only meaningful in compact mode — full-file mode always
	// reaches EOF, so probing the old-file size would be a wasted subprocess.
	total := 0
	if contextLines > 0 && contextLines < fullContextSentinel {
		total = g.totalOldLines(ref, file, staged)
	}
	lines, err := parseUnifiedDiff(out, total)
	if err != nil {
		return nil, err
	}

	// enrich binary placeholder with size delta from git diff --stat
	if len(lines) == 1 && lines[0].IsBinary {
		if desc := g.binarySizeDesc(ref, file, staged); desc != "" {
			lines[0].Content = desc
		}
	}

	return lines, nil
}

// totalOldLines returns the line count of the pre-change version of file, used by
// parseUnifiedDiff to emit a trailing divider. Returns 0 when the old-side file is
// unavailable (new files, bad refs, etc.) — the parser treats 0 as "unknown" and
// skips the trailing divider.
//
// Old-side resolution:
//   - ref empty + staged      → HEAD (git diff --cached compares HEAD against index)
//   - ref empty + not staged  → index via `git show :path`
//   - ref contains ".." or "..." → left operand (triple-dot checked first so A...B
//     is not mis-split on the leading "..")
//   - single ref              → use as-is
//
// For triple-dot ranges the left operand is an approximation of the true old side
// (merge-base(A,B)); accurate enough for the informational trailing-divider count.
func (g *Git) totalOldLines(ref, file string, staged bool) int {
	oldRef := ref
	if left, _, ok := strings.Cut(ref, "..."); ok {
		oldRef = left
	}
	if left, _, ok := strings.Cut(oldRef, ".."); ok {
		oldRef = left
	}
	if oldRef == "" && staged {
		oldRef = "HEAD"
	}
	// `git show :path` (empty oldRef) shows the index version of the file
	out, err := g.runGit("show", oldRef+":"+file)
	if err != nil {
		return 0
	}
	return countLines(out)
}

// gitContextArg returns the -U argument for git diff given the caller's requested
// context size. A non-positive contextLines or one at or above fullContextSentinel
// returns the full-file arg; any other value returns -U<contextLines>.
func gitContextArg(contextLines int) string {
	if contextLines <= 0 || contextLines >= fullContextSentinel {
		return fullFileContext
	}
	return fmt.Sprintf("-U%d", contextLines)
}

// diffArgs builds the base git diff arguments for the given ref and staged flag.
func (g *Git) diffArgs(ref string, staged bool) []string {
	args := []string{"diff", "--no-color", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	if ref != "" {
		args = append(args, ref)
	}
	return args
}

// runGit executes a git command in the working directory and returns its output.
func (g *Git) runGit(args ...string) (string, error) {
	return runVCS(g.workDir, "git", args...)
}

// runVCS executes a VCS command in the given directory and returns its output.
func runVCS(workDir, binary string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), binary, args...) //nolint:gosec // args constructed internally, not user input
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%s %s: %s", binary, strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("%s %s: %w", binary, strings.Join(args, " "), err)
	}
	return string(out), nil
}

// binarySizeDesc runs git diff --stat for a binary file and returns a human-readable
// description like "(new binary file, 2.0 KB)" or "(binary file: 1.0 KB → 2.0 KB)".
// Returns empty string if stat info is unavailable.
func (g *Git) binarySizeDesc(ref, file string, staged bool) string {
	args := g.diffArgs(ref, staged)
	args = append(args, "--stat", "--summary", "--", file)

	out, err := g.runGit(args...)
	if err != nil {
		return ""
	}

	oldSize, newSize, ok := g.parseBinaryStat(out)
	if !ok {
		return ""
	}

	return g.formatBinaryDesc(g.parseBinaryChangeKind(out), oldSize, newSize)
}

type binaryChangeKind int

const (
	binaryChangeModified binaryChangeKind = iota
	binaryChangeAdded
	binaryChangeDeleted
)

// binaryStatRe matches a git diff --stat line ending with "Bin 1234 -> 5678 bytes".
// The entire pattern ("Bin", "->", "bytes") assumes English locale; non-English git
// may localize any of these tokens, causing a graceful fallback to the header-based
// placeholder from parseUnifiedDiff (e.g. "(new binary file)" without size info).
var binaryStatRe = regexp.MustCompile(`^\s*.*\|\s+Bin (\d+) -> (\d+) bytes$`)

var (
	binaryCreateSummaryRe = regexp.MustCompile(`^\s*create mode \d+\s+`)
	binaryDeleteSummaryRe = regexp.MustCompile(`^\s*delete mode \d+\s+`)
)

// parseBinaryStat extracts old and new sizes from git diff --stat output.
// Returns (oldBytes, newBytes, ok).
func (g *Git) parseBinaryStat(statOutput string) (int64, int64, bool) {
	scanner := bufio.NewScanner(strings.NewReader(statOutput))
	for scanner.Scan() {
		m := binaryStatRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}

		oldSize, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		newSize, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		return oldSize, newSize, true
	}

	return 0, 0, false
}

func (g *Git) parseBinaryChangeKind(summaryOutput string) binaryChangeKind {
	scanner := bufio.NewScanner(strings.NewReader(summaryOutput))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case binaryCreateSummaryRe.MatchString(line):
			return binaryChangeAdded
		case binaryDeleteSummaryRe.MatchString(line):
			return binaryChangeDeleted
		}
	}

	return binaryChangeModified
}

// formatBinaryDesc builds a human-readable binary file description from old/new byte sizes.
func (g *Git) formatBinaryDesc(kind binaryChangeKind, oldSize, newSize int64) string {
	switch kind {
	case binaryChangeAdded:
		return fmt.Sprintf("(new binary file, %s)", g.formatSize(newSize))
	case binaryChangeDeleted:
		return fmt.Sprintf("(deleted binary file, %s)", g.formatSize(oldSize))
	default:
		return fmt.Sprintf("(binary file: %s → %s)", g.formatSize(oldSize), g.formatSize(newSize))
	}
}

// formatSize formats a byte count as a human-readable string.
func (g *Git) formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// hunkHeaderRe matches unified diff hunk headers like @@ -1,5 +1,7 @@.
// Lengths are optional per git's spec (omitted means length 1) and are captured
// so the parser can compute the old-side end of each hunk.
var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// countLines returns the number of lines in s, counting a final non-newline-terminated
// line as one additional line. Empty input returns 0. Used by the per-VCS totalOldLines
// methods to translate file contents into a line count for the trailing divider.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// appendGapDivider appends a "⋯ N lines ⋯" divider to lines when gap is positive.
// Used for leading, between-hunks, and trailing dividers — same format, different
// source of the gap count. Returns lines unchanged when gap <= 0 (nothing to show).
func appendGapDivider(lines []DiffLine, gap int) []DiffLine {
	switch {
	case gap == 1:
		return append(lines, DiffLine{ChangeType: ChangeDivider, Content: "⋯ 1 line ⋯"})
	case gap > 1:
		return append(lines, DiffLine{ChangeType: ChangeDivider, Content: fmt.Sprintf("⋯ %d lines ⋯", gap)})
	}
	return lines
}

// binaryFilesRe matches git's "Binary files ... differ" line for binary diffs.
// Assumes English locale; non-English git may localize this message.
var binaryFilesRe = regexp.MustCompile(`^Binary files .+ and .+ differ$`)

// parseUnifiedDiff parses unified diff output into a slice of DiffLine entries.
// it handles the diff header, hunk headers, and content lines.
// for binary diffs ("Binary files ... differ"), it returns a single placeholder DiffLine.
// intended for single-file diffs; multi-file diffs are not fully supported.
//
// totalOldLines is the total line count of the pre-change file, used to emit a
// trailing "⋯ N lines ⋯" divider after the last hunk when it does not reach EOF.
// Pass 0 when unknown (context-only sources, tests, or any case where the caller
// cannot cheaply determine the old file's size) — trailing divider is then skipped.
func parseUnifiedDiff(raw string, totalOldLines int) ([]DiffLine, error) {
	var lines []DiffLine
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), MaxLineLength)

	// skip diff header lines (---, +++, diff --git, index, etc.)
	inHeader := true
	var oldNum, newNum int
	// prevOldEnd = next untouched old-side line. Initialized to 1 so the first hunk's
	// leading divider uses the same `oldStart - prevOldEnd` formula as between-hunks gaps.
	prevOldEnd := 1
	// sawHunk tracks whether any hunk header was parsed — used as the guard for the
	// trailing divider so that insertion-at-start hunks (@@ -0,0 ...) don't collide
	// with the prevOldEnd==1 initialization sentinel.
	var sawHunk bool
	var isNewFile, isDeletedFile bool

	for scanner.Scan() {
		line := scanner.Text()

		if inHeader {
			switch {
			case strings.HasPrefix(line, "new file mode"):
				isNewFile = true
				continue
			case strings.HasPrefix(line, "deleted file mode"):
				isDeletedFile = true
				continue
			case binaryFilesRe.MatchString(line):
				content := BinaryPlaceholder
				switch {
				case isNewFile:
					content = "(new binary file)"
				case isDeletedFile:
					content = "(deleted binary file)"
				}
				return []DiffLine{{OldNum: 1, NewNum: 1, Content: content, ChangeType: ChangeContext, IsBinary: true}}, nil
			case !hunkHeaderRe.MatchString(line):
				continue
			}
			inHeader = false
		}

		// parse hunk header
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			oldStart, errOld := strconv.Atoi(m[1])
			newStart, errNew := strconv.Atoi(m[3])
			if errOld != nil || errNew != nil {
				return nil, fmt.Errorf("parse hunk header %q: old=%w new=%w", line, errOld, errNew)
			}
			// Atoi("") returns 0 with error; regex guarantees m[2] is digits when non-empty.
			// Both the omitted-length case (git spec: implicit 1) and the literal `,0` (insertion-only)
			// end up at oldLen=0 here, and max(oldLen,1) below resolves both to the same advance.
			oldLen, _ := strconv.Atoi(m[2])

			// emit divider representing unchanged lines BEFORE this hunk.
			// Leading divider (first hunk) uses prevOldEnd=1 initialization; between-hunks use
			// prevOldEnd from prior iteration. Gap uses hunk-header metadata not oldNum, so
			// insertion-only hunks (@@ -K,0 ...) compute correctly; oldNum stays put on `+` lines.
			lines = appendGapDivider(lines, oldStart-prevOldEnd)
			sawHunk = true
			// prevOldEnd = line number AFTER the current hunk on the old side. Normal hunks
			// (oldLen>0) cover [oldStart, oldStart+oldLen). Insertion-only hunks (oldLen==0,
			// e.g. @@ -K,0 ...) insert between old lines K and K+1 — handled by max(oldLen,1).
			prevOldEnd = oldStart + max(oldLen, 1)

			oldNum = oldStart
			newNum = newStart
			continue
		}

		// no-newline marker
		if strings.HasPrefix(line, `\ No newline at end of file`) {
			continue
		}

		if line == "" {
			// empty context line (happens for blank lines in source)
			lines = append(lines, DiffLine{OldNum: oldNum, NewNum: newNum, Content: "", ChangeType: ChangeContext})
			oldNum++
			newNum++
			continue
		}

		prefix := line[0]
		content := line[1:]

		switch prefix {
		case '+':
			lines = append(lines, DiffLine{OldNum: 0, NewNum: newNum, Content: content, ChangeType: ChangeAdd})
			newNum++
		case '-':
			lines = append(lines, DiffLine{OldNum: oldNum, NewNum: 0, Content: content, ChangeType: ChangeRemove})
			oldNum++
		case ' ':
			lines = append(lines, DiffLine{OldNum: oldNum, NewNum: newNum, Content: content, ChangeType: ChangeContext})
			oldNum++
			newNum++
		default:
			// unknown prefix, treat as context
			lines = append(lines, DiffLine{OldNum: oldNum, NewNum: newNum, Content: line, ChangeType: ChangeContext})
			oldNum++
			newNum++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan diff: %w", err)
	}

	// trailing divider: unchanged lines after the last hunk on the old side.
	// Emitted only when the caller supplied totalOldLines AND at least one hunk
	// was processed. sawHunk (not prevOldEnd > 1) is the correct "processed" flag
	// — insertion-at-start hunks (@@ -0,0 ...) leave prevOldEnd at 1 but still count.
	if totalOldLines > 0 && sawHunk {
		lines = appendGapDivider(lines, totalOldLines-prevOldEnd+1)
	}

	return lines, nil
}

// normalizePrefixes trims whitespace and trailing slashes from each prefix,
// skipping empty values (e.g., from env var trailing commas).
func normalizePrefixes(prefixes []string) []string {
	normalized := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		p = strings.TrimRight(p, "/")
		if p == "" {
			continue
		}
		normalized = append(normalized, p)
	}
	return normalized
}

// matchesPrefix returns true if the file path matches any prefix.
// A prefix matches if the file equals the prefix exactly, or starts with prefix + "/".
func matchesPrefix(file string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if file == prefix || strings.HasPrefix(file, prefix+"/") {
			return true
		}
	}
	return false
}
