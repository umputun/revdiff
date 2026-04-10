package ui

import "regexp"

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
func changedTokenRanges(minusLine, plusLine string) ([]matchRange, []matchRange) {
	if minusLine == "" || plusLine == "" {
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
