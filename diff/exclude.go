package diff

import (
	"fmt"
	"strings"
)

// renderer is a local interface matching ui.Renderer to avoid import cycle.
type renderer interface {
	ChangedFiles(ref string, staged bool) ([]string, error)
	FileDiff(ref, file string, staged bool) ([]DiffLine, error)
}

// ExcludeFilter wraps a renderer and filters out files matching any of the given prefixes.
// filtering is applied only at the file list level (ChangedFiles); FileDiff delegates directly.
type ExcludeFilter struct {
	inner    renderer
	prefixes []string
}

// NewExcludeFilter creates an ExcludeFilter that removes files matching any prefix from results.
func NewExcludeFilter(inner renderer, prefixes []string) *ExcludeFilter {
	// normalize prefixes: trim whitespace, strip trailing slashes so "vendor/" and "vendor" both work,
	// and skip empty values from env/config inputs like trailing commas.
	normalized := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		p = strings.TrimRight(p, "/")
		if p == "" {
			continue
		}
		normalized = append(normalized, p)
	}
	return &ExcludeFilter{inner: inner, prefixes: normalized}
}

// ChangedFiles returns files from the inner renderer, excluding any that match a prefix.
func (ef *ExcludeFilter) ChangedFiles(ref string, staged bool) ([]string, error) {
	files, err := ef.inner.ChangedFiles(ref, staged)
	if err != nil {
		return nil, fmt.Errorf("exclude filter, changed files: %w", err)
	}

	filtered := make([]string, 0, len(files))
	for _, f := range files {
		if !ef.matchesExclude(f) {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

// FileDiff delegates directly to the inner renderer without filtering.
func (ef *ExcludeFilter) FileDiff(ref, file string, staged bool) ([]DiffLine, error) {
	lines, err := ef.inner.FileDiff(ref, file, staged)
	if err != nil {
		return nil, fmt.Errorf("exclude filter, file diff %s: %w", file, err)
	}
	return lines, nil
}

// matchesExclude returns true if the file path matches any exclude prefix.
// a prefix matches if the file equals the prefix exactly, or starts with prefix + "/".
func (ef *ExcludeFilter) matchesExclude(file string) bool {
	for _, prefix := range ef.prefixes {
		if file == prefix || strings.HasPrefix(file, prefix+"/") {
			return true
		}
	}
	return false
}
