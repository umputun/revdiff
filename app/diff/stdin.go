package diff

import "io"

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

// ChangedFiles returns the single synthetic filename.
func (r *StdinReader) ChangedFiles(_ string, _ bool) ([]string, error) {
	return []string{r.name}, nil
}

// FileDiff returns the stored context lines for the synthetic file.
func (r *StdinReader) FileDiff(_, file string, _ bool) ([]DiffLine, error) {
	if file != r.name {
		return nil, nil
	}
	return r.lines, nil
}
