package diff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CompareReader is a Renderer that diffs two arbitrary files using `git diff --no-index`.
// It does not require a git repository — git diff --no-index works without one.
type CompareReader struct {
	oldPath string
	newPath string
}

// NewCompareReader creates a CompareReader that diffs oldPath against newPath.
func NewCompareReader(oldPath, newPath string) *CompareReader {
	return &CompareReader{oldPath: oldPath, newPath: newPath}
}

// ChangedFiles returns a single synthetic file entry named after the new file's base name.
func (r *CompareReader) ChangedFiles(_ string, _ bool) ([]FileEntry, error) {
	return []FileEntry{{Path: filepath.Base(r.newPath)}}, nil
}

// FileDiff runs `git diff --no-index` between the two paths and returns parsed diff lines.
// Exit code 1 is treated as success (files differ — the normal case for git diff).
// ref, file, and staged are unused; paths come from the constructor.
func (r *CompareReader) FileDiff(_, _ string, _ bool, contextLines int) ([]DiffLine, error) {
	args := []string{"diff", "--no-index", "--no-color", "--no-ext-diff", unifiedContextArg(contextLines), "--", r.oldPath, r.newPath}

	totalOldLines := 0
	if contextLines > 0 && contextLines < fullContextSentinel {
		totalOldLines = r.countFileLines()
	}

	cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // args constructed from validated CLI paths
	out, err := cmd.Output()
	if err != nil {
		if diffErr := r.diffError(err, out); diffErr != nil {
			return nil, diffErr
		}
		// diffError returned nil: exit code 1 with diff output means files differ;
		// cmd.Output() still populated out with the diff — fall through to parse.
	}

	return parseUnifiedDiff(string(out), totalOldLines)
}

// diffError converts a git diff --no-index execution error into a user-facing error.
// Exit code 1 with diff output on stdout is not an error (files differ — normal case;
// git may emit warnings on stderr while still producing a valid diff on stdout).
// Exit code 1 with no stdout output means the failure is real (e.g. file not found).
func (r *CompareReader) diffError(err error, out []byte) error {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return fmt.Errorf("git diff --no-index: %w", err)
	}
	stderr := strings.TrimSpace(string(exitErr.Stderr))
	switch {
	case exitErr.ExitCode() == 1 && len(out) > 0:
		// files differ — not an error; stdout has the diff even if stderr has warnings
		return nil
	case stderr != "":
		return fmt.Errorf("git diff --no-index: %s", stderr)
	default:
		return fmt.Errorf("git diff --no-index: exit %d", exitErr.ExitCode())
	}
}

// countFileLines returns the number of lines in r.oldPath using streaming
// reads to avoid loading the entire file into memory.
// A non-empty file not ending with '\n' counts as one additional line.
// Returns 0 (treated as unknown by parseUnifiedDiff) when the file cannot
// be read in full — including mid-read errors that would otherwise yield a
// partial, misleading count for the trailing-divider label.
func (r *CompareReader) countFileLines() int {
	info, err := os.Stat(r.oldPath)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	f, err := os.Open(r.oldPath)
	if err != nil {
		return 0
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	count := 0
	hasContent := false
	endsWithNewline := false
	for {
		n, rerr := f.Read(buf)
		for _, b := range buf[:n] {
			hasContent = true
			if b == '\n' {
				count++
				endsWithNewline = true
			} else {
				endsWithNewline = false
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			// non-EOF read error: treat the whole file as unknown rather
			// than returning a partial count that would mislabel the
			// trailing divider.
			return 0
		}
	}
	if hasContent && !endsWithNewline {
		count++
	}
	return count
}
