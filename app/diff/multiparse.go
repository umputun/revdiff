package diff

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// rawFileSection holds a raw diff section before parsing.
type rawFileSection struct {
	path     string     // new-side path, extracted by parseFileHeader
	status   FileStatus // derived from mode/rename headers
	diffText string     // full section text to pass to parseUnifiedDiff
}

// unifiedDiffSniffLimit caps how many leading bytes isUnifiedDiff inspects.
// 4 KiB comfortably covers `git diff` output (the boundary marker is the very
// first line) but can miss `git format-patch` output when a long commit body
// or large mail header pushes the first `diff --git` past this window — those
// inputs fall back to raw-text rendering. Raising the cap trades worst-case
// sniff cost for broader format-patch coverage; pick a larger value if that
// tradeoff changes.
const unifiedDiffSniffLimit = 4096

// isUnifiedDiff reports whether the content looks like a git unified diff.
// A line in the leading sniff window must START with "diff --git a/" (or the
// quoted form "diff --git \"a/" used by git for paths containing spaces). The
// previous substring check falsely classified prose that merely mentioned the
// marker (e.g. a markdown file documenting diff output). No "@@ -" fallback:
// revdiff only knows how to split sections by "diff --git" boundaries, so
// hunk-only input would mis-render anyway.
//
// Only the first unifiedDiffSniffLimit bytes are inspected — see the const
// comment for the format-patch caveat.
func isUnifiedDiff(content string) bool {
	if content == "" {
		return false
	}

	// inspect only the leading bytes for efficiency
	sample := content[:min(unifiedDiffSniffLimit, len(content))]

	for line := range strings.SplitSeq(sample, "\n") {
		if strings.HasPrefix(line, "diff --git a/") || strings.HasPrefix(line, `diff --git "a/`) {
			return true
		}
	}
	return false
}

// diffGitHeaderRe matches "diff --git a/path b/path", including quoted paths with spaces.
// Handles git's path forms:
//   - diff --git a/path b/path (simple)
//   - diff --git "a/path with spaces" "b/path with spaces" (quoted whole path)
//   - diff --git a/"path with spaces" b/"path with spaces" (quoted path after prefix)
var diffGitHeaderRe = regexp.MustCompile(`^diff --git ("?a/[^"]*"?|"?a/.*?") ("?b/[^"]*"?|"?b/.*?")`)

// splitMultiFileDiff splits a multi-file unified diff into per-file sections.
// it handles git format: "diff --git a/path b/path".
// each section includes the full diff header through to the next file boundary
// (next "diff --git" or end of input). path and status are resolved once per
// section by parseFileHeader — there is no separate inline header parse.
//
// A section whose header yields no parseable new-side path fails the whole
// call so the caller can fall back to raw-text mode. Silently skipping the
// section would let a single crafted "diff --git" line followed by prose
// drop real content from the rendering with no in-TUI signal.
func splitMultiFileDiff(raw string) ([]rawFileSection, error) {
	if raw == "" {
		return nil, errors.New("empty input")
	}

	var sections []rawFileSection
	var current strings.Builder
	inSection := false

	// flush resolves path/status from the accumulated section text and appends
	// it. An empty path is a hard failure: the caller falls back to raw-text mode.
	flush := func() error {
		if !inSection {
			return nil
		}
		text := current.String()
		path, status := parseFileHeader(text)
		if path == "" {
			return fmt.Errorf("section %q has no parseable new-side path", firstLine(text))
		}
		sections = append(sections, rawFileSection{path: path, status: status, diffText: text})
		return nil
	}

	for line := range strings.SplitSeq(raw, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			// new file boundary: flush the previous section and start a fresh one
			if err := flush(); err != nil {
				return nil, err
			}
			current.Reset()
			inSection = true
		}
		if inSection {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}

	if len(sections) == 0 {
		return nil, errors.New("no file sections found")
	}

	return sections, nil
}

// firstLine returns the first line of s, used to identify a malformed section
// in error messages without dumping the whole diff text into the log.
func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}

// cleanPath strips a leading "a/" or "b/" prefix and surrounding quotes from a path.
// Handles both git path forms:
//   - "b/path with spaces" — quotes wrap the prefix; strip quotes first, then prefix
//   - b/"path with spaces" — prefix wraps the quotes; strip prefix first, then quotes
//
// Strips the prefix exactly once so a legitimate top-level directory literally
// named "a" or "b" (e.g. "b/b/weird.go" representing repo path b/weird.go)
// resolves to "b/weird.go", not "weird.go".
func cleanPath(path string) string {
	// outer-quotes form: "a/foo" or "b/foo"
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	switch {
	case strings.HasPrefix(path, "a/"):
		path = path[2:]
	case strings.HasPrefix(path, "b/"):
		path = path[2:]
	}
	// inner-quotes form: prefix already stripped, surviving quotes wrap the body
	return strings.Trim(path, `"`)
}

// parseFileHeader extracts the new-side path and change status from a diff
// section's header lines. it parses:
//   - "diff --git a/old b/new" → path
//   - "new file mode" → FileAdded
//   - "deleted file mode" → FileDeleted
//   - "rename from/to" → FileRenamed (uses the new path)
//
// status defaults to FileModified.
func parseFileHeader(section string) (path string, status FileStatus) {
	status = FileModified

	for line := range strings.SplitSeq(section, "\n") {
		// header ends at the first hunk
		if strings.HasPrefix(line, "@@") {
			break
		}

		switch {
		case strings.HasPrefix(line, "new file mode"):
			status = FileAdded
		case strings.HasPrefix(line, "deleted file mode"):
			status = FileDeleted
		case strings.HasPrefix(line, "rename from"):
			status = FileRenamed
		case strings.HasPrefix(line, "diff --git "):
			if m := diffGitHeaderRe.FindStringSubmatch(line); len(m) >= 3 {
				// use the b-side (new) path
				path = cleanPath(m[2])
			} else if rest := strings.TrimPrefix(line, "diff --git "); rest != "" {
				// fallback for paths the regex cannot split: take the last field as b-path
				if parts := strings.Fields(rest); len(parts) >= 2 {
					path = cleanPath(parts[len(parts)-1])
				}
			}
		case strings.HasPrefix(line, "rename to "):
			// rename target overrides the diff --git path; route through cleanPath
			// so quoted paths shed their quotes like every other branch does.
			if rest := strings.TrimSpace(strings.TrimPrefix(line, "rename to ")); rest != "" {
				path = cleanPath(rest)
			}
		case strings.HasPrefix(line, "+++ "):
			// fallback: derive the path from the +++ line when the header lacked one
			if path == "" {
				path = cleanPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			}
		}
	}

	return path, status
}
