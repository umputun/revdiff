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
// A line beginning with "## " that does not match the header grammar is a hard
// error rather than a silent skip — the format is bidirectional and a stray
// header indicates a malformed input.
func Parse(r io.Reader) ([]Annotation, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var (
		out          []Annotation
		current      *Annotation
		body         []string
		seenHeader   bool
		nonBlankSeen bool
	)

	flush := func() {
		if current == nil {
			return
		}
		// The format always emits a trailing newline after the body. Strip
		// exactly one trailing empty line that came from the format separator.
		if n := len(body); n > 0 && body[n-1] == "" {
			body = body[:n-1]
		}
		current.Comment = strings.Join(body, "\n")
		out = append(out, *current)
		current = nil
		body = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			ann, err := parseHeader(line)
			if err != nil {
				return nil, err
			}
			flush()
			seenHeader = true
			current = &ann
			continue
		}

		if !seenHeader {
			if strings.TrimSpace(line) == "" {
				continue
			}
			nonBlankSeen = true
			break
		}

		// Inverse of escapeHeaderLines: strip exactly one leading space from
		// body lines whose first non-space content begins with "## ".
		if strings.HasPrefix(line, " ") && strings.HasPrefix(strings.TrimLeft(line, " "), "## ") {
			line = line[1:]
		}
		body = append(body, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan annotations: %w", err)
	}

	if !seenHeader && nonBlankSeen {
		return nil, errors.New("annotation input has content before any header")
	}

	flush()
	return out, nil
}

// parseHeader parses a single "## ..." header line into an Annotation.
// returns an error if the line does not match the expected grammar.
func parseHeader(line string) (Annotation, error) {
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
