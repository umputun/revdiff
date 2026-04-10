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
)

// ChangeType represents the type of change for a diff line.
type ChangeType string

const (
	ChangeAdd     ChangeType = "+"
	ChangeRemove  ChangeType = "-"
	ChangeContext ChangeType = " "
	ChangeDivider ChangeType = "~" // separates non-adjacent hunks

	fullFileContext = "-U1000000" // request full file as diff context

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
	Content       string     // line content without the +/- prefix
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

// Git provides methods to extract changed files and build full-file diff views.
type Git struct {
	workDir string // working directory for git commands
}

// NewGit creates a new Git diff renderer rooted at the given working directory.
func NewGit(workDir string) *Git {
	return &Git{workDir: workDir}
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

// FileDiff returns the full-file diff view for a single file.
// The result is a sequence of DiffLine entries representing unchanged, added, and removed lines
// interleaved at their correct positions.
// For binary files, it returns a single placeholder line with size delta information.
func (g *Git) FileDiff(ref, file string, staged bool) ([]DiffLine, error) {
	args := g.diffArgs(ref, staged)
	args = append(args, fullFileContext, "--", file) // large context to get full file

	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	lines, err := parseUnifiedDiff(out)
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

// hunkHeaderRe matches unified diff hunk headers like @@ -1,5 +1,7 @@
var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// binaryFilesRe matches git's "Binary files ... differ" line for binary diffs.
// Assumes English locale; non-English git may localize this message.
var binaryFilesRe = regexp.MustCompile(`^Binary files .+ and .+ differ$`)

// parseUnifiedDiff parses unified diff output into a slice of DiffLine entries.
// it handles the diff header, hunk headers, and content lines.
// for binary diffs ("Binary files ... differ"), it returns a single placeholder DiffLine.
// intended for single-file diffs; multi-file diffs are not fully supported.
func parseUnifiedDiff(raw string) ([]DiffLine, error) {
	var lines []DiffLine
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), MaxLineLength)

	// skip diff header lines (---, +++, diff --git, index, etc.)
	inHeader := true
	var oldNum, newNum int
	firstHunk := true
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
			newStart, errNew := strconv.Atoi(m[2])
			if errOld != nil || errNew != nil {
				return nil, fmt.Errorf("parse hunk header %q: old=%w new=%w", line, errOld, errNew)
			}

			// add divider between non-adjacent hunks (when using normal context, not -U1000000)
			if !firstHunk {
				lines = append(lines, DiffLine{ChangeType: ChangeDivider, Content: "..."})
			}
			firstHunk = false

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

	return lines, nil
}
