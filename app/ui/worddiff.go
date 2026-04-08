package ui

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// byteRange identifies a [start, end) byte span within a line of text.
type byteRange struct {
	start, end int
}

// computeIntraRanges returns the byte ranges within oldText and newText that
// differ, as produced by a character-level diff with semantic cleanup.
//
// Trivial-case short-circuit: if the two strings share no common prefix or
// suffix and differ entirely, each side returns a single full-length range.
func computeIntraRanges(oldText, newText string) (removeRanges, addRanges []byteRange) {
	if oldText == newText {
		return nil, nil
	}
	if oldText == "" {
		if newText == "" {
			return nil, nil
		}
		return nil, []byteRange{{0, len(newText)}}
	}
	if newText == "" {
		return []byteRange{{0, len(oldText)}}, nil
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldText, newText, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Similarity gate: if the total length of "equal" spans is too small
	// relative to the shorter line, treat this as a whole-line replacement
	// with no meaningful intra-line structure and skip the overlay entirely.
	// Force-pairing unrelated lines otherwise produces noisy highlights on
	// spurious character matches (e.g. "ind", "diff") inside wholly different
	// content. Threshold chosen empirically: pairs below 30% shared characters
	// rarely represent a real edit.
	totalEqual := 0
	for _, d := range diffs {
		if d.Type == diffmatchpatch.DiffEqual {
			totalEqual += len(d.Text)
		}
	}
	shorter := min(len(oldText), len(newText))
	const minSimilarityPct = 30
	if shorter == 0 || totalEqual*100 < shorter*minSimilarityPct {
		return nil, nil
	}

	var oldPos, newPos int
	for _, d := range diffs {
		n := len(d.Text)
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			oldPos += n
			newPos += n
		case diffmatchpatch.DiffDelete:
			removeRanges = appendRange(removeRanges, oldPos, oldPos+n)
			oldPos += n
		case diffmatchpatch.DiffInsert:
			addRanges = appendRange(addRanges, newPos, newPos+n)
			newPos += n
		}
	}
	return removeRanges, addRanges
}

// recomputeIntraRanges rebuilds m.intraRanges from the current diffLines.
// when wordDiff is disabled, the slice is cleared. non-paired indices are left nil.
func (m *Model) recomputeIntraRanges() {
	if !m.wordDiff || len(m.diffLines) == 0 {
		m.intraRanges = nil
		return
	}
	ranges := make([][]byteRange, len(m.diffLines))
	pairs := m.pairHunkLines(m.findHunks())
	for _, p := range pairs {
		if p.removeIdx < 0 || p.removeIdx >= len(m.diffLines) ||
			p.addIdx < 0 || p.addIdx >= len(m.diffLines) {
			continue
		}
		rem, add := computeIntraRanges(m.diffLines[p.removeIdx].Content, m.diffLines[p.addIdx].Content)
		ranges[p.removeIdx] = rem
		ranges[p.addIdx] = add
	}
	m.intraRanges = ranges
}

// toggleWordDiff flips word-diff highlighting and recomputes intra-line ranges.
func (m *Model) toggleWordDiff() {
	m.wordDiff = !m.wordDiff
	m.recomputeIntraRanges()
	if m.ready && m.currFile != "" {
		m.viewport.SetContent(m.renderDiff())
	}
}

// insertBgMarkers walks s byte-by-byte, skipping ANSI escape sequences, and
// inserts hlOn at the start of each range and hlOff at the end. Range offsets
// are expressed in bytes of the non-ANSI (plain) content of s. This is a
// generalized version of insertHighlightMarkers usable for any background
// overlay driven by byte ranges.
func insertBgMarkers(s string, ranges []byteRange, hlOn, hlOff string) string {
	if len(ranges) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(ranges)*(len(hlOn)+len(hlOff)))
	pos := 0  // byte position in non-ANSI content
	rIdx := 0 // current range index
	inRange := false
	i := 0
	for i < len(s) {
		// copy ANSI escape sequences verbatim without advancing pos
		if s[i] == '\033' {
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}

		// close any range that ends at current position
		if inRange && rIdx < len(ranges) && pos == ranges[rIdx].end {
			b.WriteString(hlOff)
			inRange = false
			rIdx++
		}
		// open next range when we reach its start
		if !inRange && rIdx < len(ranges) && pos == ranges[rIdx].start {
			// skip zero-length ranges
			if ranges[rIdx].start == ranges[rIdx].end {
				rIdx++
			} else {
				b.WriteString(hlOn)
				inRange = true
			}
		}

		b.WriteByte(s[i])
		pos++
		i++
	}
	// close any open range at EOL
	if inRange {
		b.WriteString(hlOff)
	}
	return b.String()
}

// highlightIntraLineChanges overlays the intra-line word-diff background on a
// styled diff line. No-op when word-diff is disabled, the line has no paired
// ranges, or the feature is not in scope.
func (m Model) highlightIntraLineChanges(idx int, s string, isAdd bool) string {
	if !m.wordDiff || idx < 0 || idx >= len(m.intraRanges) {
		return s
	}
	ranges := m.intraRanges[idx]
	if len(ranges) == 0 {
		return s
	}
	bg := m.styles.colors.WordRemoveBg
	baseBg := m.styles.colors.RemoveBg
	if isAdd {
		bg = m.styles.colors.WordAddBg
		baseBg = m.styles.colors.AddBg
	}
	hlOn := m.ansiBg(bg)
	hlOff := m.ansiBg(baseBg)
	if hlOff == "" {
		hlOff = "\033[49m"
	}
	if hlOn == "" {
		// --no-colors mode (plainStyles leaves WordAddBg/WordRemoveBg empty) or
		// an unset theme: fall back to reverse video so changed ranges stay
		// visible, matching the search-highlight fallback.
		hlOn = "\033[7m"
		hlOff = "\033[27m"
	}
	return insertBgMarkers(s, ranges, hlOn, hlOff)
}

// appendRange appends [start, end), merging with the previous range if adjacent.
func appendRange(ranges []byteRange, start, end int) []byteRange {
	if start >= end {
		return ranges
	}
	if n := len(ranges); n > 0 && ranges[n-1].end == start {
		ranges[n-1].end = end
		return ranges
	}
	return append(ranges, byteRange{start, end})
}
