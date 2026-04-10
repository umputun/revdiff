package ui

import (
	"regexp"
	"strings"

	"github.com/umputun/revdiff/app/diff"
)

// intralineToken represents a single token from the regex tokenizer with its byte offset in the source line.
type intralineToken struct {
	text  string // token text
	start int    // byte offset in the original line
	end   int    // byte offset past the last byte
}

// tokenPattern splits a line into word tokens (letters/digits/underscore), whitespace runs, and punctuation runs.
var tokenPattern = regexp.MustCompile(`[\pL\pN_]+|\s+|[^\pL\pN_\s]+`)

// tokenizeLineWithOffsets splits a line into tokens with byte offsets.
// each token is a word (letters/digits/underscore), whitespace run, or punctuation run.
func tokenizeLineWithOffsets(line string) []intralineToken {
	locs := tokenPattern.FindAllStringIndex(line, -1)
	tokens := make([]intralineToken, len(locs))
	for i, loc := range locs {
		tokens[i] = intralineToken{text: line[loc[0]:loc[1]], start: loc[0], end: loc[1]}
	}
	return tokens
}

// lcsKeptTokens computes which tokens from minus and plus lines are kept (unchanged) via LCS.
// returns two boolean slices parallel to the input token slices: true = kept, false = changed.
func lcsKeptTokens(minusToks, plusToks []intralineToken) ([]bool, []bool) {
	m, n := len(minusToks), len(plusToks)
	if m == 0 || n == 0 {
		return make([]bool, m), make([]bool, n)
	}

	// build LCS DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			switch {
			case minusToks[i-1].text == plusToks[j-1].text:
				dp[i][j] = dp[i-1][j-1] + 1
			case dp[i-1][j] >= dp[i][j-1]:
				dp[i][j] = dp[i-1][j]
			default:
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// backtrace to mark kept tokens
	keepMinus := make([]bool, m)
	keepPlus := make([]bool, n)
	i, j := m, n
	for i > 0 && j > 0 {
		switch {
		case minusToks[i-1].text == plusToks[j-1].text:
			keepMinus[i-1] = true
			keepPlus[j-1] = true
			i--
			j--
		case dp[i-1][j] >= dp[i][j-1]:
			i--
		default:
			j--
		}
	}
	return keepMinus, keepPlus
}

// isWhitespaceToken returns true if the token text is all whitespace.
func isWhitespaceToken(t intralineToken) bool {
	for _, b := range []byte(t.text) {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

// buildChangedRanges converts token keep flags into byte-offset matchRange values.
// adjacent changed non-whitespace tokens are merged into single ranges.
// whitespace-only tokens are excluded from ranges (not highlighted).
func buildChangedRanges(tokens []intralineToken, keep []bool) []matchRange {
	var ranges []matchRange
	var cur *matchRange

	for i, tok := range tokens {
		if keep[i] || isWhitespaceToken(tok) {
			// flush any open range
			if cur != nil {
				ranges = append(ranges, *cur)
				cur = nil
			}
			continue
		}
		// changed non-whitespace token
		if cur != nil {
			cur.end = tok.end // extend
		} else {
			cur = &matchRange{start: tok.start, end: tok.end}
		}
	}
	if cur != nil {
		ranges = append(ranges, *cur)
	}
	return ranges
}

// changedTokenRanges computes the changed byte-offset ranges for a pair of minus/plus lines.
// returns ranges for the minus line and plus line respectively.
// if either line is empty, returns nil ranges.
// maxLineLenForDiff caps intra-line diff to lines up to this many bytes.
// longer lines skip word-level highlighting to avoid O(m*n) LCS memory blowup
// on pathological input (minified files, very long configs).
const maxLineLenForDiff = 500

func changedTokenRanges(minusLine, plusLine string) ([]matchRange, []matchRange) {
	if minusLine == "" || plusLine == "" {
		return nil, nil
	}
	if len(minusLine) > maxLineLenForDiff || len(plusLine) > maxLineLenForDiff {
		return nil, nil
	}
	minusToks := tokenizeLineWithOffsets(minusLine)
	plusToks := tokenizeLineWithOffsets(plusLine)
	if len(minusToks) == 0 || len(plusToks) == 0 {
		return nil, nil
	}

	keepMinus, keepPlus := lcsKeptTokens(minusToks, plusToks)
	return buildChangedRanges(minusToks, keepMinus), buildChangedRanges(plusToks, keepPlus)
}

// intralinePair represents a paired remove/add line for intra-line diffing.
type intralinePair struct {
	removeIdx int // index into m.diffLines for the remove line
	addIdx    int // index into m.diffLines for the add line
}

// pairHunkLines pairs remove and add lines within a contiguous change block [start, end).
// equal-length runs pair 1:1 in order. unequal runs use greedy best-match scoring.
func (m Model) pairHunkLines(start, end int) []intralinePair {
	var removes, adds []int
	for i := start; i < end; i++ {
		switch m.diffLines[i].ChangeType { //nolint:exhaustive // only add/remove relevant for pairing
		case diff.ChangeRemove:
			removes = append(removes, i)
		case diff.ChangeAdd:
			adds = append(adds, i)
		}
	}

	if len(removes) == 0 || len(adds) == 0 {
		return nil
	}

	// equal-length: pair 1:1 in order
	if len(removes) == len(adds) {
		pairs := make([]intralinePair, len(removes))
		for i := range removes {
			pairs[i] = intralinePair{removeIdx: removes[i], addIdx: adds[i]}
		}
		return pairs
	}

	// unequal: greedy best-match — iterate the shorter side, find best match in longer side
	return m.greedyPairLines(removes, adds)
}

// greedyPairLines pairs lines greedily using prefix+suffix scoring.
// iterates the shorter side and picks the best unused match from the longer side.
func (m Model) greedyPairLines(removes, adds []int) []intralinePair {
	shorter, longer := removes, adds
	shorterIsRemove := true
	if len(adds) < len(removes) {
		shorter, longer = adds, removes
		shorterIsRemove = false
	}

	used := make([]bool, len(longer))
	pairs := make([]intralinePair, 0, len(shorter))

	for _, si := range shorter {
		bestScore := -1
		bestIdx := -1
		sContent := m.diffLines[si].Content

		for li, li2 := range longer {
			if used[li] {
				continue
			}
			lContent := m.diffLines[li2].Content
			score := 2*commonPrefixLen(sContent, lContent) + 2*commonSuffixLen(sContent, lContent)
			if score > bestScore {
				bestScore = score
				bestIdx = li
			}
		}

		if bestIdx >= 0 {
			used[bestIdx] = true
			if shorterIsRemove {
				pairs = append(pairs, intralinePair{removeIdx: si, addIdx: longer[bestIdx]})
			} else {
				pairs = append(pairs, intralinePair{removeIdx: longer[bestIdx], addIdx: si})
			}
		}
	}
	return pairs
}

// commonPrefixLen returns the number of common prefix bytes between two strings.
func commonPrefixLen(a, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// commonSuffixLen returns the number of common suffix bytes between two strings.
func commonSuffixLen(a, b string) int {
	la, lb := len(a), len(b)
	n := min(la, lb)
	for i := range n {
		if a[la-1-i] != b[lb-1-i] {
			return i
		}
	}
	return n
}

// similarityThreshold is the minimum percentage of common tokens for intra-line highlighting.
// pairs with less than this percentage of common content get no intra-line overlay.
const similarityThreshold = 30

// recomputeIntraRanges walks m.diffLines, finds contiguous change blocks,
// pairs remove/add lines, runs word-diff, and stores results in m.intraRanges.
// applies a 30% similarity gate: pairs with <30% common tokens get no ranges.
func (m *Model) recomputeIntraRanges() {
	n := len(m.diffLines)
	m.intraRanges = make([][]matchRange, n)

	i := 0
	for i < n {
		// find start of a contiguous change block
		if m.diffLines[i].ChangeType != diff.ChangeAdd && m.diffLines[i].ChangeType != diff.ChangeRemove {
			i++
			continue
		}

		// find end of the contiguous change block
		blockStart := i
		for i < n && (m.diffLines[i].ChangeType == diff.ChangeAdd || m.diffLines[i].ChangeType == diff.ChangeRemove) {
			i++
		}
		blockEnd := i

		pairs := m.pairHunkLines(blockStart, blockEnd)
		for _, p := range pairs {
			minusContent := strings.ReplaceAll(m.diffLines[p.removeIdx].Content, "\t", m.tabSpaces)
			plusContent := strings.ReplaceAll(m.diffLines[p.addIdx].Content, "\t", m.tabSpaces)

			// tokenize and run LCS once, then use results for both range building and similarity gate
			minusToks := tokenizeLineWithOffsets(minusContent)
			plusToks := tokenizeLineWithOffsets(plusContent)
			if len(minusToks) == 0 || len(plusToks) == 0 {
				continue
			}

			keepMinus, keepPlus := lcsKeptTokens(minusToks, plusToks)
			minusRanges := buildChangedRanges(minusToks, keepMinus)
			plusRanges := buildChangedRanges(plusToks, keepPlus)
			if len(minusRanges) == 0 && len(plusRanges) == 0 {
				continue // identical lines after tokenization
			}

			// similarity gate: check kept vs total non-whitespace tokens
			if !passesSimilarityGateFromKeep(minusToks, plusToks, keepMinus) {
				continue
			}

			m.intraRanges[p.removeIdx] = minusRanges
			m.intraRanges[p.addIdx] = plusRanges
		}
	}
}

// passesSimilarityGateFromKeep returns true if the pair has at least 30% common non-whitespace tokens.
// uses pre-computed tokens and keep flags from lcsKeptTokens to avoid redundant tokenization/LCS.
// whitespace tokens are excluded from the calculation to avoid inflating similarity.
func passesSimilarityGateFromKeep(minusToks, plusToks []intralineToken, keepMinus []bool) bool {
	equalNonWS := 0
	for i, k := range keepMinus {
		if k && !isWhitespaceToken(minusToks[i]) {
			equalNonWS++
		}
	}

	minusNonWS := countNonWhitespace(minusToks)
	plusNonWS := countNonWhitespace(plusToks)
	shorter := min(minusNonWS, plusNonWS)
	if shorter == 0 {
		return false
	}

	return equalNonWS*100 >= shorter*similarityThreshold
}

// countNonWhitespace returns the number of non-whitespace tokens.
func countNonWhitespace(tokens []intralineToken) int {
	n := 0
	for _, t := range tokens {
		if !isWhitespaceToken(t) {
			n++
		}
	}
	return n
}
