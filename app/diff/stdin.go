package diff

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// StdinReader is an in-memory renderer for scratch-buffer review mode.
type StdinReader struct {
	name  string
	lines []DiffLine
}

// NewStdinReader creates a renderer that exposes a single synthetic file backed by in-memory lines.
func NewStdinReader(name string, lines []DiffLine) *StdinReader {
	return &StdinReader{name: name, lines: lines}
}

// NewStdinReaderFromReader reads arbitrary content into context lines and exposes it as one synthetic file.
func NewStdinReaderFromReader(name string, r io.Reader) (*StdinReader, error) {
	lines, err := readReaderAsContext(r)
	if err != nil {
		return nil, err
	}
	return NewStdinReader(name, lines), nil
}

// NewStdinReaderFromString creates a StdinReader from string content.
// Used when content has already been read for multi-file detection.
func NewStdinReaderFromString(name, content string) (*StdinReader, error) {
	lines, err := readReaderAsContext(strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	return NewStdinReader(name, lines), nil
}

// ChangedFiles returns the single synthetic filename.
func (r *StdinReader) ChangedFiles(_ string, _ bool) ([]FileEntry, error) {
	return []FileEntry{{Path: r.name}}, nil
}

// FileDiff returns the stored context lines for the synthetic file.
// contextLines is ignored — StdinReader is a context-only source with no hunks.
func (r *StdinReader) FileDiff(_, file string, _ bool, _ int) ([]DiffLine, error) {
	if file != r.name {
		return nil, nil
	}
	return r.lines, nil
}

// MultiFileStdinReader implements Renderer for multi-file unified diffs from stdin.
type MultiFileStdinReader struct {
	sections map[string]parsedSection // path -> parsed diff lines
	order    []string                 // preserve file order from diff
}

// parsedSection holds parsed diff lines and status for one file.
type parsedSection struct {
	lines  []DiffLine
	status FileStatus
}

// NewMultiFileStdinReader parses multi-file unified diff content.
// Returns (*MultiFileStdinReader, nil) on success.
// Falls back to StdinReader via the caller if detection fails.
func NewMultiFileStdinReader(content string) (*MultiFileStdinReader, error) {
	sections, err := splitMultiFileDiff(content)
	if err != nil {
		return nil, fmt.Errorf("split multi-file diff: %w", err)
	}

	r := &MultiFileStdinReader{
		sections: make(map[string]parsedSection, len(sections)),
		order:    make([]string, 0, len(sections)),
	}

	for _, sec := range sections {
		// reuse existing parseUnifiedDiff for each file section
		lines, parseErr := parseUnifiedDiff(sec.diffText, 0)
		if parseErr != nil {
			// warn, skip this section, continue with the others
			fmt.Fprintf(os.Stderr, "warning: failed to parse section %s: %v\n", sec.path, parseErr)
			continue
		}
		r.sections[sec.path] = parsedSection{
			lines:  lines,
			status: sec.status,
		}
		r.order = append(r.order, sec.path)
	}

	if len(r.sections) == 0 {
		return nil, errors.New("no valid file sections parsed")
	}

	return r, nil
}

// ChangedFiles returns file entries in original diff order.
func (r *MultiFileStdinReader) ChangedFiles(_ string, _ bool) ([]FileEntry, error) {
	entries := make([]FileEntry, 0, len(r.order))
	for _, path := range r.order {
		sec := r.sections[path]
		entries = append(entries, FileEntry{
			Path:   path,
			Status: sec.status,
		})
	}
	return entries, nil
}

// FileDiff returns pre-parsed diff lines for the requested file.
// contextLines is ignored — sections are pre-parsed from the original diff.
func (r *MultiFileStdinReader) FileDiff(_, file string, _ bool, _ int) ([]DiffLine, error) {
	sec, ok := r.sections[file]
	if !ok {
		return nil, nil
	}
	return sec.lines, nil
}
