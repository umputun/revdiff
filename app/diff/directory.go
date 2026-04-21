package diff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// DirectoryReader is a Renderer that lists all tracked files and reads them as context lines.
// used for --all-files mode where every tracked file is browsable, not just changed files.
// the file-listing strategy is pluggable so git, jj, or any future VCS can plug in.
type DirectoryReader struct {
	workDir    string
	listSource string // human label used in error messages ("git ls-files", "jj file list")
	listFiles  func() ([]byte, error)
	splitSep   byte // separator between entries in the listFiles output (NUL or newline)
}

// NewDirectoryReader creates a DirectoryReader backed by `git ls-files`.
// The directory must be inside a git repository. Use NewJjDirectoryReader for jj repos.
func NewDirectoryReader(workDir string) *DirectoryReader {
	dr := &DirectoryReader{
		workDir:    workDir,
		listSource: "git ls-files",
		splitSep:   '\x00',
	}
	dr.listFiles = func() ([]byte, error) {
		// use -z for NUL-separated output to avoid C-quoting of paths with non-ASCII characters
		cmd := exec.CommandContext(context.Background(), "git", "ls-files", "-z")
		cmd.Dir = workDir
		return cmd.Output()
	}
	return dr
}

// NewJjDirectoryReader creates a DirectoryReader backed by `jj file list`.
// jj file list emits one path per line (no -z / NUL mode), and jj auto-tracks
// every file in the working copy so its output is equivalent to git ls-files.
func NewJjDirectoryReader(workDir string) *DirectoryReader {
	dr := &DirectoryReader{
		workDir:    workDir,
		listSource: "jj file list",
		splitSep:   '\n',
	}
	dr.listFiles = func() ([]byte, error) {
		cmd := exec.CommandContext(context.Background(), "jj", "file", "list")
		cmd.Dir = workDir
		return cmd.Output()
	}
	return dr
}

// ChangedFiles returns all tracked files as sorted relative paths.
// ref and staged parameters are ignored since all tracked files are returned.
func (dr *DirectoryReader) ChangedFiles(_ string, _ bool) ([]FileEntry, error) {
	out, err := dr.listFiles()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("%s: %s", dr.listSource, stderr)
			}
		}
		return nil, fmt.Errorf("%s: %w", dr.listSource, err)
	}

	entries := make([]FileEntry, 0, bytes.Count(out, []byte{dr.splitSep}))
	for entry := range strings.SplitSeq(string(out), string(dr.splitSep)) {
		if entry == "" {
			continue
		}
		// skip files that are tracked in the index but deleted locally (unstaged deletion).
		// use Lstat to avoid following symlinks — broken symlinks are still valid tracked entries.
		resolved := resolvePath(dr.workDir, entry)
		if _, statErr := os.Lstat(resolved); statErr != nil && os.IsNotExist(statErr) {
			continue
		}
		entries = append(entries, FileEntry{Path: entry})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// FileDiff reads the file from disk and returns all lines as context DiffLines.
// ref, staged, and contextLines parameters are ignored — DirectoryReader is a
// context-only source with no hunks.
func (dr *DirectoryReader) FileDiff(_, file string, _ bool, _ int) ([]DiffLine, error) {
	resolved := resolvePath(dr.workDir, file)
	return readFileAsContext(resolved)
}
