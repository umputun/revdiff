package diff

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// jjAnnotateTemplate is the template for `jj file annotate` that produces
// tab-separated output: change_id_short\tauthor_name\tepoch_seconds\tline_content.
// content already includes the source line's trailing newline (when present), so we
// don't append one here — doing so would introduce spurious blank lines between rows.
const jjAnnotateTemplate = `commit.change_id().short() ++ "\t" ++ commit.author().name() ++ "\t" ++ commit.author().timestamp().format("%s") ++ "\t" ++ content`

// FileBlame returns blame information for each line of a file.
// For Jujutsu the staged flag is ignored — there is no staging area.
func (j *Jj) FileBlame(ref, file string, _ bool) (map[int]BlameLine, error) {
	args := []string{"file", "annotate", "-T", jjAnnotateTemplate}
	if targetRef := j.blameTargetRef(ref); targetRef != "" {
		args = append(args, "-r", targetRef)
	}
	args = append(args, file)

	out, err := j.runJj(args...)
	if err != nil {
		return nil, fmt.Errorf("annotate %s: %w", file, err)
	}
	return j.parseAnnotate(out)
}

// blameTargetRef picks the revision to blame at.
// Unlike git, jj can't implicitly blame the working copy — we always need to name
// a revision. Empty and single-sided ranges fall back to "@" (working copy).
func (j *Jj) blameTargetRef(ref string) string {
	// check triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		if left == "" || right == "" {
			return ""
		}
		return translateJjRef(right)
	}
	if left, right, ok := strings.Cut(ref, ".."); ok {
		if left == "" || right == "" {
			return ""
		}
		return translateJjRef(right)
	}
	if ref != "" {
		return translateJjRef(ref)
	}
	return ""
}

// parseAnnotate turns `jj file annotate` template output into a map of 1-based
// line number to BlameLine. Each row looks like:
//
//	change_id\tauthor\tepoch\tcontent
//
// content ends with \n (except possibly the final line). We rely on a 5-way
// Split (max 4 separators) so embedded tabs in content don't confuse us.
func (j *Jj) parseAnnotate(raw string) (map[int]BlameLine, error) {
	result := make(map[int]BlameLine)
	// strip exactly one trailing newline so we don't produce a spurious final empty row
	raw = strings.TrimSuffix(raw, "\n")
	if raw == "" {
		return result, nil
	}

	lineNum := 0
	for row := range strings.SplitSeq(raw, "\n") {
		lineNum++

		parts := strings.SplitN(row, "\t", 4)
		if len(parts) < 4 { // malformed — still advance lineNum so downstream alignment holds
			continue
		}

		author := parts[1]
		var authorTime time.Time
		if epoch, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			authorTime = time.Unix(epoch, 0)
		}

		result[lineNum] = BlameLine{Author: author, Time: authorTime}
	}

	return result, nil
}
