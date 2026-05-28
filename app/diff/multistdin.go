package diff

import (
	"errors"
	"fmt"
)

// ErrNotUnifiedDiff is returned by NewMultiFileStdinReader when the input does
// not look like a git unified diff. Callers use errors.Is to silently fall
// back to the raw-text StdinReader without logging — the sniff failing is the
// expected path for plain text input, not a parse error worth surfacing.
var ErrNotUnifiedDiff = errors.New("input is not a unified diff")

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

// NewMultiFileStdinReader parses multi-file unified diff content. The sniff
// is internal: when content does not look like a unified diff the call
// returns ErrNotUnifiedDiff so the caller can silently route to the raw-text
// StdinReader. Any per-section parse failure fails the whole call — partial
// success would leak a tree where one file's hunks are silently dropped,
// which is a review-integrity hazard.
func NewMultiFileStdinReader(content string) (*MultiFileStdinReader, error) {
	if !isUnifiedDiff(content) {
		return nil, ErrNotUnifiedDiff
	}

	sections, err := splitMultiFileDiff(content)
	if err != nil {
		return nil, fmt.Errorf("split multi-file diff: %w", err)
	}

	r := &MultiFileStdinReader{
		sections: make(map[string]parsedSection, len(sections)),
		order:    make([]string, 0, len(sections)),
	}

	for _, sec := range sections {
		lines, parseErr := parseUnifiedDiff(sec.diffText, 0)
		if parseErr != nil {
			// fail the whole reader so the caller falls back to raw-text mode
			// for the entire input rather than silently dropping one file's hunks.
			return nil, fmt.Errorf("parse section %q: %w", sec.path, parseErr)
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
