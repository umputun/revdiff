package annotation

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Annotation represents a user comment on a specific diff line.
type Annotation struct {
	File    string // file path relative to repo root
	Line    int    // line number in the diff
	EndLine int    // end line of hunk range, 0 means no range
	Type    string // change type: "+", "-", or " "
	Comment string // user comment text
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
// If an annotation already exists at the same file:line, it is replaced.
func (s *Store) Add(a Annotation) {
	existing := s.annotations[a.File]
	if i, ok := s.find(a.File, a.Line, a.Type); ok {
		existing[i].Comment = a.Comment
		existing[i].EndLine = a.EndLine
		return
	}
	s.annotations[a.File] = append(existing, a)
}

// Delete removes the annotation at the given file, line and change type.
// Returns true if an annotation was found and removed.
func (s *Store) Delete(file string, line int, changeType string) bool {
	i, ok := s.find(file, line, changeType)
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
	_, ok := s.find(file, line, changeType)
	return ok
}

// find returns the index of an annotation matching file, line, and changeType.
func (s *Store) find(file string, line int, changeType string) (int, bool) {
	for i, a := range s.annotations[file] {
		if a.Line == line && a.Type == changeType {
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

// Clear removes all annotations from the store.
func (s *Store) Clear() {
	s.annotations = make(map[string][]Annotation)
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

// Load parses markdown produced by FormatOutput from r and adds each recovered
// annotation via Add (so duplicate file/line/type pairs apply last-write-wins).
// It is the symmetric inverse of FormatOutput on the API surface; callers that
// need to filter records (e.g. drop orphans against a diff) should use Parse
// directly and Add the survivors themselves.
func (s *Store) Load(r io.Reader) error {
	records, err := Parse(r)
	if err != nil {
		return err
	}
	for _, a := range records {
		s.Add(a)
	}
	return nil
}

// FormatOutput produces the structured output format for stdout.
// Files are sorted alphabetically, annotations within each file by line number.
// Returns empty string if no annotations exist.
//
// Body lines that start with "## " (the record-header form) are prefixed with a
// single space on output so parsers that split on "## " record headers cannot
// mistake a comment line for a new record. The added space is cosmetic
// (markdown renderers treat leading whitespace before a heading marker as
// paragraph text) and preserves the original text when whitespace is trimmed
// per line. Other markdown heading forms like "### subheader" are not escaped.
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
			body := s.escapeHeaderLines(a.Comment)
			switch {
			case a.Line == 0:
				fmt.Fprintf(&buf, "## %s (file-level)\n%s\n", a.File, body)
			case a.EndLine > 0:
				fmt.Fprintf(&buf, "## %s:%d-%d (%s)\n%s\n", a.File, a.Line, a.EndLine, a.Type, body)
			default:
				fmt.Fprintf(&buf, "## %s:%d (%s)\n%s\n", a.File, a.Line, a.Type, body)
			}
		}
	}
	return buf.String()
}

// escapeHeaderLines prefixes any body line whose first non-space content is
// "## " with a single extra space. The parser inverts this by stripping one
// leading space from any body line that, after left-trimming, begins with
// "## ". Escaping pre-indented variants (e.g. " ## ") keeps the round-trip
// symmetric for arbitrary user content. Other heading forms like "### " are
// not escaped since they cannot collide with the record-header split marker.
func (s *Store) escapeHeaderLines(body string) string {
	if !strings.Contains(body, "## ") {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "## ") {
			lines[i] = " " + line
		}
	}
	return strings.Join(lines, "\n")
}
