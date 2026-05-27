package diff

import (
	"errors"
	"regexp"
	"strings"
)

// rawFileSection holds a raw diff section before parsing.
type rawFileSection struct {
	path     string     // new-side path, extracted by parseFileHeader
	status   FileStatus // derived from mode/rename headers
	diffText string     // full section text to pass to parseUnifiedDiff
}

// unifiedDiffSniffLimit caps how many leading bytes IsUnifiedDiff inspects.
const unifiedDiffSniffLimit = 4096

// IsUnifiedDiff reports whether the content looks like a git unified diff.
// Detection criteria:
//   - starts with "diff --git a/"
//   - has "@@ -" hunk header pattern within the sniff limit
func IsUnifiedDiff(content string) bool {
	if content == "" {
		return false
	}

	// inspect only the leading bytes for efficiency
	sample := content[:min(unifiedDiffSniffLimit, len(content))]

	// primary: git diff header
	if strings.Contains(sample, "diff --git a/") {
		return true
	}

	// secondary: hunk header pattern
	return strings.Contains(sample, "@@ -")
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
func splitMultiFileDiff(raw string) ([]rawFileSection, error) {
	if raw == "" {
		return nil, errors.New("empty input")
	}

	var sections []rawFileSection
	var current strings.Builder
	inSection := false

	// flush appends the accumulated section text, resolving path/status from its header.
	// sections without a parseable new-side path are skipped.
	flush := func() {
		if !inSection {
			return
		}
		text := current.String()
		path, status := parseFileHeader(text)
		if path == "" {
			return
		}
		sections = append(sections, rawFileSection{path: path, status: status, diffText: text})
	}

	for line := range strings.SplitSeq(raw, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			// new file boundary: flush the previous section and start a fresh one
			flush()
			current.Reset()
			inSection = true
		}
		if inSection {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}
	flush()

	if len(sections) == 0 {
		return nil, errors.New("no file sections found")
	}

	return sections, nil
}

// cleanPath strips a leading "a/" or "b/" prefix and surrounding quotes from a path.
// Handles both "b/path with spaces" (quotes outside prefix) and b/"path with spaces"
// (quotes inside prefix) by stripping the prefix, then the quotes, then the prefix again.
func cleanPath(path string) string {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "b/"), "a/")
	path = strings.Trim(path, `"`)
	return strings.TrimPrefix(strings.TrimPrefix(path, "b/"), "a/")
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
			// rename target overrides the diff --git path
			if parts := strings.Fields(line); len(parts) >= 3 {
				path = strings.Join(parts[2:], " ")
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
