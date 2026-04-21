package diff

import "fmt"

//go:generate moq -out mocks/Renderer.go -pkg mocks -skip-ensure -fmt goimports . Renderer

// Renderer is the local interface mirroring ui.Renderer, exported for moq generation.
// contextLines controls how many lines of context surround each hunk in the diff output.
// 0 or values >= fullContextSentinel request full-file context (the revdiff default);
// positive values below the sentinel request that many lines on each side. Context-only
// sources (e.g. FileReader, DirectoryReader, StdinReader) ignore this parameter since
// they have no hunks.
type Renderer interface {
	ChangedFiles(ref string, staged bool) ([]FileEntry, error)
	FileDiff(ref, file string, staged bool, contextLines int) ([]DiffLine, error)
}

// ExcludeFilter wraps a renderer and filters out files matching any of the given prefixes.
// Filtering is applied only at the file list level (ChangedFiles); FileDiff delegates directly.
type ExcludeFilter struct {
	inner    Renderer
	prefixes []string
}

// NewExcludeFilter creates an ExcludeFilter that removes files matching any prefix from results.
func NewExcludeFilter(inner Renderer, prefixes []string) *ExcludeFilter {
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
// contextLines is passed through unchanged.
func (ef *ExcludeFilter) FileDiff(ref, file string, staged bool, contextLines int) ([]DiffLine, error) {
	lines, err := ef.inner.FileDiff(ref, file, staged, contextLines)
	if err != nil {
		return nil, fmt.Errorf("exclude filter, file diff %s: %w", file, err)
	}
	return lines, nil
}
