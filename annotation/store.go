package annotation

import (
	"fmt"
	"sort"
	"strings"
)

// Annotation represents a user comment on a specific diff line or range.
type Annotation struct {
	File    string // file path relative to repo root
	Line    int    // line number in the diff (start line for ranges)
	EndLine int    // end line for range annotations (0 = point annotation)
	Type    string // change type: "+", "-", or " " (always "" for ranges)
	Comment string // user comment text
}

// IsRange returns true if this annotation represents a range annotation.
// Range annotations use Type == "" and may collapse to the same numeric line
// for replacement hunks where old/new line numbers match.
func (a Annotation) IsRange() bool {
	return a.Type == "" && a.EndLine > 0 && a.EndLine >= a.Line
}

// Store holds annotations in memory, keyed by filename.
type Store struct {
	annotations map[string][]Annotation
}

// NewStore creates a new empty annotation store.
func NewStore() *Store {
	return &Store{annotations: make(map[string][]Annotation)}
}

// Add adds an annotation for the given file and line.
// If an annotation already exists at the same (file, line, endLine, type), it is replaced.
// Range annotations that overlap existing ranges in the same file are rejected.
func (s *Store) Add(a Annotation) bool {
	existing := s.annotations[a.File]
	if i, ok := s.find(a.File, a.Line, a.EndLine, a.Type); ok {
		existing[i].Comment = a.Comment
		return true
	}
	if a.IsRange() {
		for _, other := range existing {
			if !other.IsRange() {
				continue
			}
			if a.Line <= other.EndLine && a.EndLine >= other.Line {
				return false
			}
		}
	}
	s.annotations[a.File] = append(existing, a)
	return true
}

// Delete removes the annotation at the given file, line, endLine and change type.
// Returns true if an annotation was found and removed.
func (s *Store) Delete(file string, line, endLine int, changeType string) bool {
	i, ok := s.find(file, line, endLine, changeType)
	if !ok {
		return false
	}
	existing := s.annotations[file]
	s.annotations[file] = append(existing[:i], existing[i+1:]...)
	if len(s.annotations[file]) == 0 {
		delete(s.annotations, file)
	}
	return true
}

// Has checks if an annotation exists at the given file, line and change type.
func (s *Store) Has(file string, line int, changeType string) bool {
	_, ok := s.find(file, line, 0, changeType)
	return ok
}

// HasRangeCovering returns true if any range annotation in the file covers lineNum.
func (s *Store) HasRangeCovering(file string, lineNum int) bool {
	for _, a := range s.annotations[file] {
		if a.IsRange() && lineNum >= a.Line && lineNum <= a.EndLine {
			return true
		}
	}
	return false
}

// GetRangeCovering returns the range annotation covering lineNum, if any.
func (s *Store) GetRangeCovering(file string, lineNum int) (Annotation, bool) {
	for _, a := range s.annotations[file] {
		if a.IsRange() && lineNum >= a.Line && lineNum <= a.EndLine {
			return a, true
		}
	}
	return Annotation{}, false
}

// find returns the index of an annotation matching file, line, endLine, and changeType.
func (s *Store) find(file string, line, endLine int, changeType string) (int, bool) {
	for i, a := range s.annotations[file] {
		if a.Line == line && a.EndLine == endLine && a.Type == changeType {
			return i, true
		}
	}
	return 0, false
}

// Get returns all annotations for the given file, sorted by line number.
func (s *Store) Get(file string) []Annotation {
	result := make([]Annotation, len(s.annotations[file]))
	copy(result, s.annotations[file])
	sort.Slice(result, func(i, j int) bool { return result[i].Line < result[j].Line })
	return result
}

// Count returns the total number of annotations across all files.
func (s *Store) Count() int {
	count := 0
	for _, anns := range s.annotations {
		count += len(anns)
	}
	return count
}

// All returns all annotations grouped by file. The returned map is a copy.
func (s *Store) All() map[string][]Annotation {
	result := make(map[string][]Annotation, len(s.annotations))
	for file, anns := range s.annotations {
		copied := make([]Annotation, len(anns))
		copy(copied, anns)
		sort.Slice(copied, func(i, j int) bool { return copied[i].Line < copied[j].Line })
		result[file] = copied
	}
	return result
}

// Files returns the list of files that have annotations, sorted alphabetically.
func (s *Store) Files() []string {
	files := make([]string, 0, len(s.annotations))
	for file := range s.annotations {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

// FormatOutput produces the structured output format for stdout.
// Files are sorted alphabetically, annotations within each file by line number.
// Returns empty string if no annotations exist.
func (s *Store) FormatOutput() string {
	if len(s.annotations) == 0 {
		return ""
	}

	files := s.Files()

	var buf strings.Builder
	first := true
	for _, file := range files {
		anns := s.Get(file) // sorted by line: file-level (0) first, then ascending
		for _, a := range anns {
			if !first {
				buf.WriteString("\n")
			}
			first = false
			switch {
			case a.Line == 0:
				fmt.Fprintf(&buf, "## %s (file-level)\n%s\n", a.File, a.Comment)
			case a.IsRange():
				fmt.Fprintf(&buf, "## %s:%d-%d\n%s\n", a.File, a.Line, a.EndLine, a.Comment)
			default:
				fmt.Fprintf(&buf, "## %s:%d (%s)\n%s\n", a.File, a.Line, a.Type, a.Comment)
			}
		}
	}
	return buf.String()
}
