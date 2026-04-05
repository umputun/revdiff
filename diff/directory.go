package diff

import (
	"bufio"
	"context"
	"errors"
	"fmt"
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
	cmd := exec.CommandContext(context.Background(), "git", "ls-files")
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
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			files = append(files, line)
		}
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
