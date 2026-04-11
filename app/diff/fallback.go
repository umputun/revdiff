package diff

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// FallbackRenderer wraps a VCS renderer and knows about --only file paths.
// it delegates to the inner renderer, falling back to disk read for --only files
// that are not present in the diff.
type FallbackRenderer struct {
	inner   renderer
	only    []string
	workDir string
}

// NewFallbackRenderer creates a FallbackRenderer that delegates to inner and falls back
// to reading files from disk for --only patterns not found in the diff.
func NewFallbackRenderer(inner renderer, only []string, workDir string) *FallbackRenderer {
	return &FallbackRenderer{inner: inner, only: only, workDir: workDir}
}

// ChangedFiles returns changed files from the inner renderer, then appends any --only files
// not already present in the result if they exist on disk.
func (fr *FallbackRenderer) ChangedFiles(ref string, staged bool) ([]FileEntry, error) {
	entries, err := fr.inner.ChangedFiles(ref, staged)
	if err != nil {
		return nil, fmt.Errorf("fallback changed files: %w", err)
	}

	for _, pattern := range fr.only {
		if fr.matchesAnyEntry(entries, pattern) {
			continue // already in diff result, skip
		}
		resolved := resolvePath(fr.workDir, pattern)
		if _, statErr := os.Stat(resolved); statErr != nil {
			continue // file doesn't exist on disk
		}
		// use the original pattern so that filterOnly (which matches against
		// the original --only values) can find the file in the result list.
		entries = append(entries, FileEntry{Path: pattern})
	}
	return entries, nil
}

// FileDiff returns the diff for a file. for files outside the repo (absolute paths
// that escape workDir), it skips the inner renderer entirely and reads from disk.
// for in-repo files, it calls the inner renderer first; if the result is empty
// (no error, no lines) and the file matches an --only pattern, it falls back to
// reading the file from disk as all-context lines.
func (fr *FallbackRenderer) FileDiff(ref, file string, staged bool) ([]DiffLine, error) {
	resolved := resolvePath(fr.workDir, file)

	// skip inner renderer for files outside the repo — VCS would reject them
	// with "is outside repository" error
	if !fr.isInsideWorkDir(resolved) {
		if _, statErr := os.Stat(resolved); statErr == nil {
			return readFileAsContext(resolved)
		}
		return nil, fmt.Errorf("file not found: %s", file)
	}

	lines, err := fr.inner.FileDiff(ref, file, staged)
	if err != nil {
		return lines, fmt.Errorf("fallback file diff %s: %w", file, err)
	}
	if len(lines) > 0 {
		return lines, nil
	}

	// empty result (no error) — check if this file matches any --only pattern
	if !fr.isOnlyFile(file) {
		return lines, nil
	}

	if _, statErr := os.Stat(resolved); statErr == nil {
		return readFileAsContext(resolved)
	}
	return lines, nil // file doesn't exist on disk, return empty result
}

// matchesAnyEntry returns true if any entry in the list matches the given pattern.
// checks exact match, suffix match (e.g. "plan.md" matches "docs/plans/plan.md"),
// and resolved-relative match (e.g. absolute "/repo/README.md" matches relative "README.md").
func (fr *FallbackRenderer) matchesAnyEntry(entries []FileEntry, pattern string) bool {
	for _, e := range entries {
		if fr.pathMatches(e.Path, pattern) {
			return true
		}
	}
	return false
}

// isOnlyFile returns true if the given file path matches any --only pattern.
func (fr *FallbackRenderer) isOnlyFile(file string) bool {
	for _, pattern := range fr.only {
		if fr.pathMatches(file, pattern) {
			return true
		}
	}
	return false
}

// pathMatches returns true if a file path matches a pattern by exact match,
// suffix match (e.g. "plan.md" matches "docs/plans/plan.md"), or resolved-relative
// match (e.g. absolute "/repo/README.md" matches relative "README.md").
func (fr *FallbackRenderer) pathMatches(file, pattern string) bool {
	if file == pattern || strings.HasSuffix(file, "/"+pattern) {
		return true
	}
	relPattern := fr.relativePath(resolvePath(fr.workDir, pattern))
	return file == relPattern
}

// isInsideWorkDir returns true if the resolved absolute path is within workDir.
func (fr *FallbackRenderer) isInsideWorkDir(absPath string) bool {
	rel, err := filepath.Rel(fr.workDir, absPath)
	return err == nil && !strings.HasPrefix(rel, "..")
}

// resolvePath resolves a path against a base directory. absolute paths are returned as-is.
func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// relativePath returns a workDir-relative path when possible. if the relative path
// escapes workDir (starts with ".."), the original absolute path is returned instead.
func (fr *FallbackRenderer) relativePath(absPath string) string {
	rel, err := filepath.Rel(fr.workDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return absPath
	}
	return rel
}

// FileReader is a standalone Renderer for use when no git repo is available.
// it reads --only files directly from disk and presents them as all-context lines.
type FileReader struct {
	files   []string
	workDir string
}

// NewFileReader creates a FileReader that reads the given files from disk.
// relative paths are resolved against workDir.
func NewFileReader(files []string, workDir string) *FileReader {
	return &FileReader{files: files, workDir: workDir}
}

// ChangedFiles returns the file list, resolved against workDir, filtered to only those that exist on disk.
func (r *FileReader) ChangedFiles(_ string, _ bool) ([]FileEntry, error) {
	if len(r.files) == 0 {
		return nil, nil
	}
	result := make([]FileEntry, 0, len(r.files))
	for _, f := range r.files {
		resolved := resolvePath(r.workDir, f)
		if _, err := os.Stat(resolved); err != nil {
			continue // skip files that don't exist
		}
		result = append(result, FileEntry{Path: resolved})
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// FileDiff reads the file from disk and returns all lines as context DiffLines.
func (r *FileReader) FileDiff(_, file string, _ bool) ([]DiffLine, error) {
	resolved := resolvePath(r.workDir, file)
	return readFileAsContext(resolved)
}

type readerContextError struct {
	op  string
	err error
}

func (e readerContextError) Error() string { return e.op + ": " + e.err.Error() }
func (e readerContextError) Unwrap() error { return e.err }

// readReaderAsContext reads arbitrary text content and returns all lines as context DiffLines.
// binary content (detected by null bytes in the first 8KB) returns a single placeholder line.
func readReaderAsContext(r io.Reader) ([]DiffLine, error) {
	reader := bufio.NewReaderSize(r, 8192)

	probe, err := reader.Peek(8192)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, readerContextError{op: "read", err: err}
	}
	if slices.Contains(probe, byte(0)) {
		return []DiffLine{{OldNum: 1, NewNum: 1, Content: BinaryPlaceholder, ChangeType: ChangeContext, IsBinary: true}}, nil
	}

	var lines []DiffLine
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), MaxLineLength)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		lines = append(lines, DiffLine{
			OldNum:     lineNum,
			NewNum:     lineNum,
			Content:    scanner.Text(),
			ChangeType: ChangeContext,
		})
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return []DiffLine{{OldNum: 1, NewNum: 1, Content: "(file has lines too long to display)", ChangeType: ChangeContext, IsPlaceholder: true}}, nil
		}
		return nil, readerContextError{op: "scan", err: err}
	}
	return lines, nil
}

// readFileAsContext reads a file from disk and returns all lines as context DiffLines.
// each line gets ChangeContext type with both OldNum and NewNum set to the 1-based line number.
// binary files (detected by null bytes in the first 8KB) return a single placeholder line.
// handles broken symlinks, non-regular files, binary detection, and error unwrapping.
func readFileAsContext(path string) ([]DiffLine, error) {
	info, err := os.Stat(path)
	if err != nil {
		// broken symlink: lstat succeeds (symlink entry exists) but stat fails because target is gone.
		// only treat as broken symlink when target is actually missing (ENOENT), not for other errors
		// like permission denied or I/O errors — those should propagate as real errors.
		if os.IsNotExist(err) {
			if linfo, lErr := os.Lstat(path); lErr == nil && linfo.Mode()&os.ModeSymlink != 0 {
				return []DiffLine{{OldNum: 1, NewNum: 1, Content: "(broken symlink)", ChangeType: ChangeContext, IsPlaceholder: true}}, nil
			}
		}
		return nil, fmt.Errorf("stat file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return []DiffLine{{OldNum: 1, NewNum: 1, Content: "(not a regular file)", ChangeType: ChangeContext, IsPlaceholder: true}}, nil
	}

	f, err := os.Open(path) //nolint:gosec // path comes from user-provided --only flag or git ls-files
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	defer f.Close()

	lines, err := readReaderAsContext(f)
	if err != nil {
		var ctxErr readerContextError
		if errors.As(err, &ctxErr) {
			return nil, fmt.Errorf("%s file %s: %w", ctxErr.op, path, ctxErr.err)
		}
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return lines, nil
}

// ReadFileAsAdded reads a file from disk and returns all lines as ChangeAdd type.
// single-line placeholder results (broken symlinks, non-regular files, binary, too-long lines) keep ChangeContext.
func ReadFileAsAdded(path string) ([]DiffLine, error) {
	lines, err := readFileAsContext(path)
	if err != nil {
		return nil, err
	}
	// single-line placeholders (broken symlink, non-regular, binary, too-long lines) are returned as-is
	// but with OldNum zeroed because "added" file placeholders should not show old line numbers
	if len(lines) == 1 && (lines[0].IsBinary || lines[0].IsPlaceholder) {
		lines[0].OldNum = 0
		return lines, nil
	}
	for i := range lines {
		lines[i].ChangeType = ChangeAdd
		lines[i].OldNum = 0
	}
	return lines, nil
}
