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
const (
	ChangeAdd     = "+"
	ChangeRemove  = "-"
	ChangeContext = " "
	ChangeDivider = "~" // separates non-adjacent hunks
)

// DiffLine holds parsed line info from a diff.
type DiffLine struct {
	OldNum     int    // line number in old version (0 for additions)
	NewNum     int    // line number in new version (0 for removals)
	Content    string // line content without the +/- prefix
	ChangeType string // changeAdd, ChangeRemove, ChangeContext, or ChangeDivider
}

// Git provides methods to extract changed files and build full-file diff views.
type Git struct {
	workDir string // working directory for git commands
}

// NewGit creates a new Git diff renderer rooted at the given working directory.
func NewGit(workDir string) *Git {
	return &Git{workDir: workDir}
}

// ChangedFiles returns a list of files changed relative to the given ref.
// If ref is empty, it shows uncommitted changes. If staged is true, shows only staged changes.
func (g *Git) ChangedFiles(ref string, staged bool) ([]string, error) {
	args := g.diffArgs(ref, staged)
	args = append(args, "--name-only")

	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// FileDiff returns the full-file diff view for a single file.
// The result is a sequence of DiffLine entries representing unchanged, added, and removed lines
// interleaved at their correct positions.
func (g *Git) FileDiff(ref, file string, staged bool) ([]DiffLine, error) {
	args := g.diffArgs(ref, staged)
	args = append(args, "-U1000000", "--", file) // large context to get full file

	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("get file diff for %s: %w", file, err)
	}

	return ParseUnifiedDiff(out)
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
	cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // git args are constructed internally
	cmd.Dir = g.workDir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// hunkHeaderRe matches unified diff hunk headers like @@ -1,5 +1,7 @@
var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// ParseUnifiedDiff parses unified diff output into a slice of DiffLine entries.
// It handles the diff header, hunk headers, and content lines.
func ParseUnifiedDiff(raw string) ([]DiffLine, error) {
	var lines []DiffLine
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024) // 1MB max line

	// skip diff header lines (---, +++, diff --git, index, etc.)
	inHeader := true
	var oldNum, newNum int
	firstHunk := true

	for scanner.Scan() {
		line := scanner.Text()

		if inHeader {
			if !hunkHeaderRe.MatchString(line) {
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
