package diff

import "fmt"

// IncludeFilter wraps a renderer and keeps only files matching any of the given prefixes.
// Filtering is applied only at the file list level (ChangedFiles); FileDiff delegates directly.
type IncludeFilter struct {
	inner    Renderer
	prefixes []string
}

// NewIncludeFilter creates an IncludeFilter that keeps only files matching any prefix.
func NewIncludeFilter(inner Renderer, prefixes []string) *IncludeFilter {
	return &IncludeFilter{inner: inner, prefixes: normalizePrefixes(prefixes)}
}

// ChangedFiles returns files from the inner renderer, keeping only those matching a prefix.
// If all prefixes normalized to empty, acts as a no-op and returns all files.
func (f *IncludeFilter) ChangedFiles(ref string, staged bool) ([]FileEntry, error) {
	entries, err := f.inner.ChangedFiles(ref, staged)
	if err != nil {
		return nil, fmt.Errorf("include filter, changed files: %w", err)
	}

	if len(f.prefixes) == 0 {
		return entries, nil
	}

	filtered := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		if matchesPrefix(e.Path, f.prefixes) {
			filtered = append(filtered, e)
		}
	}
	return filtered, nil
}

// FileDiff delegates directly to the inner renderer without filtering.
// contextLines is passed through unchanged.
func (f *IncludeFilter) FileDiff(ref, file string, staged bool, contextLines int) ([]DiffLine, error) {
	lines, err := f.inner.FileDiff(ref, file, staged, contextLines)
	if err != nil {
		return nil, fmt.Errorf("include filter, file diff %s: %w", file, err)
	}
	return lines, nil
}
