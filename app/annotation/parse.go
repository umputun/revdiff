package annotation

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// headerRe matches the four record-header shapes emitted by Store.FormatOutput:
//
//	## path (file-level)
//	## path:N (T)
//	## path:N-M (T)
//
// where T is one of "+", "-", or " " (literal space).
var headerRe = regexp.MustCompile(`^## (.+?)(?::(\d+)(?:-(\d+))?)? \((file-level|\+|-| )\)$`)

// Parse reads the markdown produced by Store.FormatOutput and returns the
// recovered annotations in source order. Duplicates (same file/line/type) are
// returned as separate records; callers feed them through Store.Add to apply
// last-write-wins semantics.
//
// A line beginning with "## " that does NOT match the header grammar is folded
// into the body of the current record so hand-authored or LLM-generated bodies
// can mention "## something" without escaping. If such a line appears before
// any record header, it is reported as an error.
func Parse(r io.Reader) ([]Annotation, error) {
	p := parser{scanner: bufio.NewScanner(r)}
	p.scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	return p.parse()
}

type parser struct {
	scanner *bufio.Scanner

	out          []Annotation
	current      *Annotation
	body         []string
	seenHeader   bool
	nonBlankSeen bool
}

func (p *parser) parse() ([]Annotation, error) {
	for p.scanner.Scan() {
		line := p.scanner.Text()
		if strings.HasPrefix(line, "## ") {
			ann, err := p.parseHeader(line)
			if err != nil {
				// non-grammar "## " line inside a record: treat as body content
				// (post-strip of any leading-space escape) so authored bodies
				// can mention "## foo" without escaping. Before the first
				// header, propagate the error.
				if !p.seenHeader {
					return nil, err
				}
				p.appendBody(line)
				continue
			}
			p.flush()
			p.seenHeader = true
			p.current = &ann
			continue
		}

		if !p.seenHeader {
			if strings.TrimSpace(line) == "" {
				continue
			}
			p.nonBlankSeen = true
			break
		}

		p.appendBody(line)
	}
	if err := p.scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan annotations: %w", err)
	}

	if !p.seenHeader && p.nonBlankSeen {
		return nil, errors.New("annotation input has content before any header")
	}

	p.flush()
	return p.out, nil
}

// appendBody adds a body line, stripping the inverse of escapeHeaderLines:
// exactly one leading space when the line's first non-space content begins
// with "## ".
func (p *parser) appendBody(line string) {
	if strings.HasPrefix(line, " ") && strings.HasPrefix(strings.TrimLeft(line, " "), "## ") {
		line = line[1:]
	}
	p.body = append(p.body, line)
}

func (p *parser) flush() {
	if p.current == nil {
		return
	}
	// FormatOutput always emits a trailing newline after the body. Strip
	// exactly one trailing empty line that came from the format separator.
	if n := len(p.body); n > 0 && p.body[n-1] == "" {
		p.body = p.body[:n-1]
	}
	p.current.Comment = strings.Join(p.body, "\n")
	p.out = append(p.out, *p.current)
	p.current = nil
	p.body = nil
}

// parseHeader parses a single "## ..." header line into an Annotation.
// returns an error if the line does not match the expected grammar.
func (p *parser) parseHeader(line string) (Annotation, error) {
	m := headerRe.FindStringSubmatch(line)
	if m == nil {
		return Annotation{}, fmt.Errorf("malformed annotation header: %q", line)
	}
	ann := Annotation{File: m[1]}
	if m[4] == "file-level" {
		// file-level headers are emitted as "## path (file-level)" with no
		// numeric suffix on the path. if the regex consumed a `:N`/`:N-M`
		// tail into the optional line group, the path itself ended in
		// `:N`/`:N-M` — restore it so paths that look numeric round-trip.
		if m[2] != "" {
			ann.File += ":" + m[2]
			if m[3] != "" {
				ann.File += "-" + m[3]
			}
		}
		return ann, nil
	}
	ann.Type = m[4]
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return Annotation{}, fmt.Errorf("malformed annotation header line number: %q", line)
	}
	ann.Line = n
	if m[3] != "" {
		end, err := strconv.Atoi(m[3])
		if err != nil {
			return Annotation{}, fmt.Errorf("malformed annotation header end line: %q", line)
		}
		ann.EndLine = end
	}
	return ann, nil
}
