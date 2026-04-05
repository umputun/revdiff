package diff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// DirectoryReader is a Renderer that lists all git-tracked files and reads them as context lines.
// used for --all-files mode where every tracked file is browsable, not just changed files.
type DirectoryReader struct {
	workDir string
}

// NewDirectoryReader creates a DirectoryReader rooted at the given working directory.
// the directory must be inside a git repository.
func NewDirectoryReader(workDir string) *DirectoryReader {
	return &DirectoryReader{workDir: workDir}
}

// ChangedFiles returns all git-tracked files as sorted relative paths.
// ref and staged parameters are ignored since all tracked files are returned.
func (dr *DirectoryReader) ChangedFiles(_ string, _ bool) ([]string, error) {
	// use -z for NUL-separated output to avoid C-quoting of paths with non-ASCII characters
	cmd := exec.CommandContext(context.Background(), "git", "ls-files", "-z")
	cmd.Dir = dr.workDir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git ls-files: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var files []string
	for entry := range strings.SplitSeq(string(out), "\x00") {
		if entry == "" {
			continue
		}
		// skip files that are tracked in the index but deleted locally (unstaged deletion).
		// use Lstat to avoid following symlinks — broken symlinks are still valid tracked entries.
		resolved := resolvePath(dr.workDir, entry)
		if _, statErr := os.Lstat(resolved); statErr != nil && os.IsNotExist(statErr) {
			continue
		}
		files = append(files, entry)
	}
	sort.Strings(files)
	return files, nil
}

// FileDiff reads the file from disk and returns all lines as context DiffLines.
// ref and staged parameters are ignored since the file is read directly from disk.
func (dr *DirectoryReader) FileDiff(_, file string, _ bool) ([]DiffLine, error) {
	resolved := resolvePath(dr.workDir, file)
	return readFileAsContext(resolved)
}
