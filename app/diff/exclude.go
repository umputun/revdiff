package diff

import "fmt"

// renderer is a local interface matching ui.Renderer to avoid import cycle.
type renderer interface {
	ChangedFiles(ref string, staged bool) ([]FileEntry, error)
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
	return &ExcludeFilter{inner: inner, prefixes: normalizePrefixes(prefixes)}
}

// ChangedFiles returns files from the inner renderer, excluding any that match a prefix.
func (ef *ExcludeFilter) ChangedFiles(ref string, staged bool) ([]FileEntry, error) {
	entries, err := ef.inner.ChangedFiles(ref, staged)
	if err != nil {
		return nil, fmt.Errorf("exclude filter, changed files: %w", err)
	}

	filtered := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		if !matchesPrefix(e.Path, ef.prefixes) {
			filtered = append(filtered, e)
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
