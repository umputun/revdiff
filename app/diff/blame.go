package diff

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// BlameLine holds blame information for a single line of a file.
type BlameLine struct {
	Author string
	Time   time.Time
}

// FileBlame returns blame information for each line of the rendered side of a file diff.
// For unstaged single-ref diffs this is the worktree; for two-ref diffs this is the target ref.
// For staged diffs this is the index snapshot. The returned map is keyed by 1-based line
// number (matching DiffLine.NewNum).
func (g *Git) FileBlame(ref, file string, staged bool) (map[int]BlameLine, error) {
	args := []string{"blame", "--line-porcelain"}
	if staged {
		tmpName, err := g.writeStagedBlameFile(file)
		if err != nil {
			return nil, err
		}
		defer func() { _ = os.Remove(tmpName) }()
		args = append(args, "--contents", tmpName)
	} else if targetRef := blameTargetRef(ref); targetRef != "" {
		args = append(args, targetRef)
	}
	args = append(args, "--", file)
	out, err := g.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("blame %s: %w", file, err)
	}
	return parseBlame(out)
}

// writeStagedBlameFile writes the staged (index) contents of file to a temp file
// and returns its path. The caller is responsible for removing the temp file.
func (g *Git) writeStagedBlameFile(file string) (string, error) {
	indexContent, err := g.runGit("show", ":"+file)
	if err != nil {
		return "", fmt.Errorf("read index contents for %s: %w", file, err)
	}
	tmp, err := os.CreateTemp("", "revdiff-blame-*")
	if err != nil {
		return "", fmt.Errorf("create temp blame file for %s: %w", file, err)
	}
	if _, err := tmp.WriteString(indexContent); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temp blame file for %s: %w", file, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp blame file for %s: %w", file, err)
	}
	return tmp.Name(), nil
}

func blameTargetRef(ref string) string {
	// check triple-dot first so "A...B" isn't mis-split on ".."
	if left, right, ok := strings.Cut(ref, "..."); ok {
		if left == "" || right == "" {
			return ""
		}
		return right
	}
	if left, right, ok := strings.Cut(ref, ".."); ok {
		if left == "" || right == "" {
			return ""
		}
		return right
	}
	return ""
}

// parseBlame parses git blame --line-porcelain output into a map of line number to BlameLine.
func parseBlame(raw string) (map[int]BlameLine, error) {
	result := make(map[int]BlameLine)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), MaxLineLength)

	var lineNum int
	var author string
	var authorTime time.Time

	for scanner.Scan() {
		line := scanner.Text()

		// header line: <40-hex-hash> <orig_line> <final_line> [<group_lines>]
		if len(line) >= 40 && isHexString(line[:40]) {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil {
					lineNum = n
				}
			}
			author = ""
			authorTime = time.Time{}
			continue
		}

		if v, ok := strings.CutPrefix(line, "author "); ok {
			author = v
			continue
		}

		if v, ok := strings.CutPrefix(line, "author-time "); ok {
			if epoch, err := strconv.ParseInt(v, 10, 64); err == nil {
				authorTime = time.Unix(epoch, 0)
			}
			continue
		}

		// content line (starts with tab) marks end of entry
		if strings.HasPrefix(line, "\t") && lineNum > 0 {
			result[lineNum] = BlameLine{Author: author, Time: authorTime}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan blame output: %w", err)
	}
	return result, nil
}

// isHexString returns true if all characters in s are hexadecimal digits.
func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// RelativeAge formats a timestamp as a compact relative age string (3 chars wide).
// Examples: " 5m" (minutes), " 3h" (hours), " 2d" (days), " 1w" (weeks), " 4M" (months), " 2y" (years).
func RelativeAge(t, now time.Time) string {
	if t.IsZero() {
		return "  ?"
	}
	d := max(0, now.Sub(t))
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%2dm", max(1, int(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%2dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%2dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%2dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%2dM", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%2dy", int(d.Hours()/(24*365)))
	}
}
