package diff

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// hgAnnotateTemplate is the template for hg annotate that produces tab-separated output:
// rev\tnode\tauthor\tepoch offset\tline_content
const hgAnnotateTemplate = `{lines % "{rev}\t{node|short}\t{author|person}\t{date|hgdate}\t{line}"}`

// FileBlame returns blame information for each line of a file.
// for mercurial, staged flag is ignored (no staging area).
func (h *Hg) FileBlame(ref, file string, _ bool) (map[int]BlameLine, error) {
	args := []string{"annotate", "-T", hgAnnotateTemplate}
	if targetRef := h.blameTargetRef(ref); targetRef != "" {
		args = append(args, "-r", targetRef)
	}
	args = append(args, file)

	out, err := h.runHg(args...)
	if err != nil {
		return nil, fmt.Errorf("annotate %s: %w", file, err)
	}
	return h.parseAnnotate(out)
}

// blameTargetRef extracts the target revision for hg annotate from a ref.
// unlike Git.blameTargetRef (which returns "" for single refs to blame the worktree),
// hg needs the revision passed explicitly since it has no staging area distinction.
func (h *Hg) blameTargetRef(ref string) string {
	// check triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		if left == "" || right == "" {
			return ""
		}
		return translateRef(right)
	}
	if left, right, ok := strings.Cut(ref, ".."); ok {
		if left == "" || right == "" {
			return ""
		}
		return translateRef(right)
	}
	if ref != "" {
		return translateRef(ref)
	}
	return ""
}

// parseAnnotate parses hg annotate template output into a map of line number to BlameLine.
// each line is: rev\tnode\tauthor\tepoch offset\tline_content
func (h *Hg) parseAnnotate(raw string) (map[int]BlameLine, error) {
	result := make(map[int]BlameLine)
	lineNum := 0

	for line := range strings.SplitSeq(raw, "\n") {
		// skip trailing empty element from final newline, but count all other lines
		if line == "" {
			continue
		}
		lineNum++

		// split into at most 5 fields: rev, node, author, hgdate, content
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 4 { // malformed line — still counted, just not parsed
			continue
		}

		author := parts[2]

		// parse hgdate format: "epoch offset" (e.g. "1775860468 -7200")
		var authorTime time.Time
		dateParts := strings.Fields(parts[3])
		if len(dateParts) >= 1 {
			if epoch, err := strconv.ParseInt(dateParts[0], 10, 64); err == nil {
				authorTime = time.Unix(epoch, 0)
			}
		}

		result[lineNum] = BlameLine{Author: author, Time: authorTime}
	}

	return result, nil
}
