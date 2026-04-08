package ui

import (
	"regexp"
	"strings"

	"github.com/umputun/revdiff/app/diff"
)

const (
	intralineMaxLineDistance      = 0.60
	intralineNaiveMaxLineDistance = 1.00
)

var intralineTokenRe = regexp.MustCompile(`[\pL\pN_]+|\s+|[^\pL\pN_\s]+`)

type intralineToken struct {
	text       string
	start, end int
}

type intralinePair struct {
	minus int
	plus  int
}

// computeIntralineRanges infers paired remove/add lines within each contiguous change block
// and computes token-level changed ranges for each paired line.
func computeIntralineRanges(lines []diff.DiffLine, tabSpaces string) map[int][]matchRange {
	ranges := make(map[int][]matchRange)
	if len(lines) == 0 {
		return nil
	}

	for start := 0; start < len(lines); {
		if !isIntralineChange(lines[start].ChangeType) {
			start++
			continue
		}

		end := start + 1
		for end < len(lines) && isIntralineChange(lines[end].ChangeType) {
			end++
		}

		var minusIdx, plusIdx []int
		for i := start; i < end; i++ {
			switch lines[i].ChangeType {
			case diff.ChangeRemove:
				minusIdx = append(minusIdx, i)
			case diff.ChangeAdd:
				plusIdx = append(plusIdx, i)
			case diff.ChangeContext, diff.ChangeDivider:
				// contiguous change blocks are pre-filtered, ignore non-change types defensively.
			}
		}

		for _, pair := range pairChangedLines(lines, minusIdx, plusIdx, tabSpaces) {
			minusLine := strings.ReplaceAll(lines[pair.minus].Content, "\t", tabSpaces)
			plusLine := strings.ReplaceAll(lines[pair.plus].Content, "\t", tabSpaces)
			minusRanges, plusRanges := changedTokenRanges(minusLine, plusLine)
			if len(minusRanges) > 0 {
				ranges[pair.minus] = minusRanges
			}
			if len(plusRanges) > 0 {
				ranges[pair.plus] = plusRanges
			}
		}

		start = end
	}

	if len(ranges) == 0 {
		return nil
	}
	return ranges
}

func isIntralineChange(changeType diff.ChangeType) bool {
	return changeType == diff.ChangeAdd || changeType == diff.ChangeRemove
}

// pairChangedLines greedily pairs remove and add lines while preserving order.
func pairChangedLines(lines []diff.DiffLine, minusIdx, plusIdx []int, tabSpaces string) []intralinePair {
	if len(minusIdx) == 0 || len(plusIdx) == 0 {
		return nil
	}

	naiveMode := len(minusIdx) == len(plusIdx)
	limit := intralineMaxLineDistance
	if naiveMode {
		limit = intralineNaiveMaxLineDistance
	}

	var pairs []intralinePair
	nextPlus := 0

	for _, mi := range minusIdx {
		minusLine := strings.ReplaceAll(lines[mi].Content, "\t", tabSpaces)
		bestJ := -1
		bestDist := 2.0

		for j := nextPlus; j < len(plusIdx); j++ {
			plusLine := strings.ReplaceAll(lines[plusIdx[j]].Content, "\t", tabSpaces)
			dist := normalizedRuneDistance(minusLine, plusLine)
			if dist < bestDist {
				bestDist = dist
				bestJ = j
			}
		}

		if bestJ >= 0 && bestDist <= limit {
			pairs = append(pairs, intralinePair{minus: mi, plus: plusIdx[bestJ]})
			nextPlus = bestJ + 1
		}
	}

	return pairs
}

func normalizedRuneDistance(a, b string) float64 {
	a = normalizeForDistance(a)
	b = normalizeForDistance(b)
	if a == b {
		return 0
	}

	ar := []rune(a)
	br := []rune(b)
	denom := max(len(ar), len(br))
	if denom == 0 {
		return 0
	}

	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}

	for i, ra := range ar {
		curr := make([]int, len(br)+1)
		curr[0] = i + 1
		for j, rb := range br {
			cost := 0
			if ra != rb {
				cost = 1
			}
			del := prev[j+1] + 1
			ins := curr[j] + 1
			sub := prev[j] + cost
			curr[j+1] = min(del, min(ins, sub))
		}
		prev = curr
	}

	return float64(prev[len(br)]) / float64(denom)
}

func normalizeForDistance(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func changedTokenRanges(minusLine, plusLine string) ([]matchRange, []matchRange) {
	minusTokens := tokenizeLineWithOffsets(minusLine)
	plusTokens := tokenizeLineWithOffsets(plusLine)
	if len(minusTokens) == 0 && len(plusTokens) == 0 {
		return nil, nil
	}

	keepMinus, keepPlus := lcsKeptTokens(minusTokens, plusTokens)
	return buildChangedRanges(minusTokens, keepMinus), buildChangedRanges(plusTokens, keepPlus)
}

func tokenizeLineWithOffsets(line string) []intralineToken {
	idxs := intralineTokenRe.FindAllStringIndex(line, -1)
	if len(idxs) == 0 {
		if line == "" {
			return nil
		}
		return []intralineToken{{text: line, start: 0, end: len(line)}}
	}

	tokens := make([]intralineToken, 0, len(idxs))
	next := 0
	for _, idx := range idxs {
		if idx[0] > next {
			tokens = append(tokens, intralineToken{text: line[next:idx[0]], start: next, end: idx[0]})
		}
		tokens = append(tokens, intralineToken{text: line[idx[0]:idx[1]], start: idx[0], end: idx[1]})
		next = idx[1]
	}
	if next < len(line) {
		tokens = append(tokens, intralineToken{text: line[next:], start: next, end: len(line)})
	}

	return tokens
}

func lcsKeptTokens(minusTokens, plusTokens []intralineToken) ([]bool, []bool) {
	m, n := len(minusTokens), len(plusTokens)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if minusTokens[i-1].text == plusTokens[j-1].text {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	keepMinus := make([]bool, m)
	keepPlus := make([]bool, n)
	i, j := m, n
	for i > 0 && j > 0 {
		if minusTokens[i-1].text == plusTokens[j-1].text {
			keepMinus[i-1] = true
			keepPlus[j-1] = true
			i--
			j--
			continue
		}
		if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return keepMinus, keepPlus
}

func buildChangedRanges(tokens []intralineToken, keep []bool) []matchRange {
	var (
		ranges []matchRange
		active bool
		cur    matchRange
	)

	for i, tok := range tokens {
		changed := (i >= len(keep) || !keep[i]) && strings.TrimSpace(tok.text) != ""
		if changed {
			if !active {
				cur = matchRange{start: tok.start, end: tok.end}
				active = true
				continue
			}
			if tok.start <= cur.end {
				cur.end = tok.end
				continue
			}
			ranges = append(ranges, cur)
			cur = matchRange{start: tok.start, end: tok.end}
			continue
		}
		if active {
			ranges = append(ranges, cur)
			active = false
		}
	}
	if active {
		ranges = append(ranges, cur)
	}

	return ranges
}
