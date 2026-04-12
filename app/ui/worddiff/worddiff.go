// Package worddiff provides intra-line word-diff algorithms and a shared text-range
// highlight insertion engine. It owns tokenization, LCS computation, byte-offset range
// building, similarity gating, and line pairing for add/remove diff blocks.
//
// The public API is exposed as methods on the stateless Differ type, enabling consumer-side
// interface wrapping in the ui package (same pattern as style.SGR).
package worddiff

import "regexp"

// Differ provides intra-line word-diff algorithms and highlight marker insertion.
// stateless — the receiver carries no mutable state. exists to group related methods
// under a type for consumer-side interface wrapping (same pattern as style.SGR).
type Differ struct{}

// New returns a Differ. the constructor exists for consistency with the DI pattern
// (main.go calls worddiff.New() and injects into ModelConfig).
func New() *Differ { return &Differ{} }

// Range represents a byte-offset range in a text line.
// used for both intra-line word-diff highlighting and search match highlighting.
type Range struct {
	Start int // byte offset of range start
	End   int // byte offset past last byte
}

// LinePair represents a line of content with its change direction for pairing.
type LinePair struct {
	Content  string
	IsRemove bool
}

// Pair represents a matched pair of remove/add line indices for intra-line diffing.
type Pair struct {
	RemoveIdx int
	AddIdx    int
}

// maxLineLenForDiff caps intra-line diff to lines up to this many bytes.
// longer lines skip word-level highlighting to avoid O(m*n) LCS memory blowup
// on pathological input (minified files, very long configs).
const maxLineLenForDiff = 500

// similarityThreshold is the minimum percentage of common tokens for highlighting.
// pairs with less than this percentage of common content get no intra-line overlay.
const similarityThreshold = 30

// intralineToken represents a single token from the regex tokenizer with its byte offset in the source line.
type intralineToken struct {
	text  string // token text
	start int    // byte offset in the original line
	end   int    // byte offset past the last byte
}

// tokenPattern splits a line into word tokens (letters/digits/underscore), whitespace runs, and punctuation runs.
var tokenPattern = regexp.MustCompile(`[\pL\pN_]+|\s+|[^\pL\pN_\s]+`)

// ComputeIntraRanges computes changed byte-offset ranges for a pair of minus/plus lines.
// returns ranges for the minus line and plus line respectively.
// returns nil ranges if either line is empty, exceeds maxLineLenForDiff,
// or fails the similarity gate (< 30% common non-whitespace tokens).
func (d *Differ) ComputeIntraRanges(minusLine, plusLine string) ([]Range, []Range) {
	if minusLine == "" || plusLine == "" {
		return nil, nil
	}
	if len(minusLine) > maxLineLenForDiff || len(plusLine) > maxLineLenForDiff {
		return nil, nil
	}

	minusToks := d.tokenizeLineWithOffsets(minusLine)
	plusToks := d.tokenizeLineWithOffsets(plusLine)
	if len(minusToks) == 0 || len(plusToks) == 0 {
		return nil, nil
	}

	keepMinus, keepPlus := d.lcsKeptTokens(minusToks, plusToks)
	minusRanges := d.buildChangedRanges(minusToks, keepMinus)
	plusRanges := d.buildChangedRanges(plusToks, keepPlus)
	if len(minusRanges) == 0 && len(plusRanges) == 0 {
		return nil, nil // identical lines after tokenization
	}

	if !d.passesSimilarityGateFromKeep(minusToks, plusToks, keepMinus) {
		return nil, nil
	}

	return minusRanges, plusRanges
}

// PairLines pairs remove and add lines within a contiguous change block.
// equal-length runs pair 1:1 in order. unequal runs use greedy best-match scoring.
// the indices in the returned Pair values are indices into the input lines slice.
func (d *Differ) PairLines(lines []LinePair) []Pair {
	var removes, adds []int
	for i, lp := range lines {
		if lp.IsRemove {
			removes = append(removes, i)
		} else {
			adds = append(adds, i)
		}
	}

	if len(removes) == 0 || len(adds) == 0 {
		return nil
	}

	// equal-length: pair 1:1 in order
	if len(removes) == len(adds) {
		pairs := make([]Pair, len(removes))
		for i := range removes {
			pairs[i] = Pair{RemoveIdx: removes[i], AddIdx: adds[i]}
		}
		return pairs
	}

	// unequal: greedy best-match
	return d.greedyPair(lines, removes, adds)
}

// tokenizeLineWithOffsets splits a line into tokens with byte offsets.
// each token is a word (letters/digits/underscore), whitespace run, or punctuation run.
func (d *Differ) tokenizeLineWithOffsets(line string) []intralineToken {
	locs := tokenPattern.FindAllStringIndex(line, -1)
	tokens := make([]intralineToken, len(locs))
	for i, loc := range locs {
		tokens[i] = intralineToken{text: line[loc[0]:loc[1]], start: loc[0], end: loc[1]}
	}
	return tokens
}

// lcsKeptTokens computes which tokens from minus and plus lines are kept (unchanged) via LCS.
// returns two boolean slices parallel to the input token slices: true = kept, false = changed.
func (d *Differ) lcsKeptTokens(minusToks, plusToks []intralineToken) ([]bool, []bool) {
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
func (d *Differ) isWhitespaceToken(t intralineToken) bool {
	for _, b := range []byte(t.text) {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

// buildChangedRanges converts token keep flags into byte-offset Range values.
// adjacent changed non-whitespace tokens are merged into single ranges.
// whitespace-only tokens are excluded from ranges (not highlighted).
func (d *Differ) buildChangedRanges(tokens []intralineToken, keep []bool) []Range {
	var ranges []Range
	var cur *Range

	for i, tok := range tokens {
		if keep[i] || d.isWhitespaceToken(tok) {
			// flush any open range
			if cur != nil {
				ranges = append(ranges, *cur)
				cur = nil
			}
			continue
		}
		// changed non-whitespace token
		if cur != nil {
			cur.End = tok.end // extend
		} else {
			cur = &Range{Start: tok.start, End: tok.end}
		}
	}
	if cur != nil {
		ranges = append(ranges, *cur)
	}
	return ranges
}

// passesSimilarityGateFromKeep returns true if the pair has at least 30% common non-whitespace tokens.
// uses pre-computed tokens and keep flags from lcsKeptTokens to avoid redundant tokenization/LCS.
// whitespace tokens are excluded from the calculation to avoid inflating similarity.
func (d *Differ) passesSimilarityGateFromKeep(minusToks, plusToks []intralineToken, keepMinus []bool) bool {
	equalNonWS := 0
	for i, k := range keepMinus {
		if k && !d.isWhitespaceToken(minusToks[i]) {
			equalNonWS++
		}
	}

	minusNonWS := d.countNonWhitespace(minusToks)
	plusNonWS := d.countNonWhitespace(plusToks)
	shorter := min(minusNonWS, plusNonWS)
	if shorter == 0 {
		return false
	}

	return equalNonWS*100 >= shorter*similarityThreshold
}

// countNonWhitespace returns the number of non-whitespace tokens.
func (d *Differ) countNonWhitespace(tokens []intralineToken) int {
	n := 0
	for _, t := range tokens {
		if !d.isWhitespaceToken(t) {
			n++
		}
	}
	return n
}

// greedyPair pairs lines greedily using prefix+suffix scoring.
// iterates the shorter side and picks the best unused match from the longer side.
func (d *Differ) greedyPair(lines []LinePair, removes, adds []int) []Pair {
	shorter, longer := removes, adds
	shorterIsRemove := true
	if len(adds) < len(removes) {
		shorter, longer = adds, removes
		shorterIsRemove = false
	}

	used := make([]bool, len(longer))
	pairs := make([]Pair, 0, len(shorter))

	for _, si := range shorter {
		bestScore := -1
		bestIdx := -1
		sContent := lines[si].Content

		for li, li2 := range longer {
			if used[li] {
				continue
			}
			lContent := lines[li2].Content
			score := 2*d.commonPrefixLen(sContent, lContent) + 2*d.commonSuffixLen(sContent, lContent)
			if score > bestScore {
				bestScore = score
				bestIdx = li
			}
		}

		if bestIdx >= 0 {
			used[bestIdx] = true
			if shorterIsRemove {
				pairs = append(pairs, Pair{RemoveIdx: si, AddIdx: longer[bestIdx]})
			} else {
				pairs = append(pairs, Pair{RemoveIdx: longer[bestIdx], AddIdx: si})
			}
		}
	}
	return pairs
}

// commonPrefixLen returns the number of common prefix bytes between two strings.
func (d *Differ) commonPrefixLen(a, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// commonSuffixLen returns the number of common suffix bytes between two strings.
func (d *Differ) commonSuffixLen(a, b string) int {
	la, lb := len(a), len(b)
	n := min(la, lb)
	for i := range n {
		if a[la-1-i] != b[lb-1-i] {
			return i
		}
	}
	return n
}
