package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestTokenizeLineWithOffsets(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		expect []intralineToken
	}{
		{name: "simple words", line: "foo bar", expect: []intralineToken{
			{text: "foo", start: 0, end: 3}, {text: " ", start: 3, end: 4}, {text: "bar", start: 4, end: 7},
		}},
		{name: "punctuation", line: "a+b", expect: []intralineToken{
			{text: "a", start: 0, end: 1}, {text: "+", start: 1, end: 2}, {text: "b", start: 2, end: 3},
		}},
		{name: "mixed content", line: "fmt.Println(x)", expect: []intralineToken{
			{text: "fmt", start: 0, end: 3}, {text: ".", start: 3, end: 4},
			{text: "Println", start: 4, end: 11}, {text: "(", start: 11, end: 12},
			{text: "x", start: 12, end: 13}, {text: ")", start: 13, end: 14},
		}},
		{name: "whitespace runs", line: "a  b\tc", expect: []intralineToken{
			{text: "a", start: 0, end: 1}, {text: "  ", start: 1, end: 3},
			{text: "b", start: 3, end: 4}, {text: "\t", start: 4, end: 5},
			{text: "c", start: 5, end: 6},
		}},
		{name: "unicode word", line: "héllo wörld", expect: []intralineToken{
			{text: "héllo", start: 0, end: 6}, {text: " ", start: 6, end: 7},
			{text: "wörld", start: 7, end: 13},
		}},
		{name: "multibyte CJK", line: "日本語 test", expect: []intralineToken{
			{text: "日本語", start: 0, end: 9}, {text: " ", start: 9, end: 10},
			{text: "test", start: 10, end: 14},
		}},
		{name: "empty string", line: "", expect: nil},
		{name: "only whitespace", line: "   ", expect: []intralineToken{
			{text: "   ", start: 0, end: 3},
		}},
		{name: "only punctuation", line: "++--", expect: []intralineToken{
			{text: "++--", start: 0, end: 4},
		}},
		{name: "underscore in word", line: "my_var", expect: []intralineToken{
			{text: "my_var", start: 0, end: 6},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenizeLineWithOffsets(tc.line)
			if tc.expect == nil {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, len(tc.expect))
			for i, exp := range tc.expect {
				assert.Equal(t, exp.text, got[i].text, "token %d text", i)
				assert.Equal(t, exp.start, got[i].start, "token %d start", i)
				assert.Equal(t, exp.end, got[i].end, "token %d end", i)
			}
		})
	}
}

func TestLCSKeptTokens(t *testing.T) {
	tok := func(texts ...string) []intralineToken {
		result := make([]intralineToken, 0, len(texts))
		off := 0
		for _, txt := range texts {
			result = append(result, intralineToken{text: txt, start: off, end: off + len(txt)})
			off += len(txt)
		}
		return result
	}

	tests := []struct {
		name      string
		minus     []intralineToken
		plus      []intralineToken
		keepMinus []bool
		keepPlus  []bool
	}{
		{
			name: "identical lines", minus: tok("foo", " ", "bar"), plus: tok("foo", " ", "bar"),
			keepMinus: []bool{true, true, true}, keepPlus: []bool{true, true, true},
		},
		{
			name: "single word rename", minus: tok("foo", " ", "bar"), plus: tok("foo", " ", "baz"),
			keepMinus: []bool{true, true, false}, keepPlus: []bool{true, true, false},
		},
		{
			name: "fully different", minus: tok("aaa"), plus: tok("bbb"),
			keepMinus: []bool{false}, keepPlus: []bool{false},
		},
		{
			name: "empty minus", minus: nil, plus: tok("foo"),
			keepMinus: []bool{}, keepPlus: []bool{false},
		},
		{
			name: "empty plus", minus: tok("foo"), plus: nil,
			keepMinus: []bool{false}, keepPlus: []bool{},
		},
		{
			name:      "multiple changes",
			minus:     tok("return", " ", "fmt", ".", "Sprintf", "(", "hello", ")"),
			plus:      tok("return", " ", "fmt", ".", "Fprintf", "(", "world", ")"),
			keepMinus: []bool{true, true, true, true, false, true, false, true},
			keepPlus:  []bool{true, true, true, true, false, true, false, true},
		},
		{
			name: "addition at end", minus: tok("a", " ", "b"), plus: tok("a", " ", "b", " ", "c"),
			keepMinus: []bool{true, true, true}, keepPlus: []bool{true, true, true, false, false},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMinus, gotPlus := lcsKeptTokens(tc.minus, tc.plus)
			assert.Equal(t, tc.keepMinus, gotMinus, "keepMinus")
			assert.Equal(t, tc.keepPlus, gotPlus, "keepPlus")
		})
	}
}

func TestBuildChangedRanges(t *testing.T) {
	tests := []struct {
		name   string
		tokens []intralineToken
		keep   []bool
		expect []matchRange
	}{
		{
			name: "single changed word",
			tokens: []intralineToken{
				{text: "foo", start: 0, end: 3}, {text: " ", start: 3, end: 4}, {text: "bar", start: 4, end: 7},
			},
			keep:   []bool{true, true, false},
			expect: []matchRange{{start: 4, end: 7}},
		},
		{
			name: "adjacent changed words merge",
			tokens: []intralineToken{
				{text: "aaa", start: 0, end: 3}, {text: "bbb", start: 3, end: 6}, {text: " ", start: 6, end: 7}, {text: "ccc", start: 7, end: 10},
			},
			keep:   []bool{false, false, true, true},
			expect: []matchRange{{start: 0, end: 6}},
		},
		{
			name: "whitespace excluded from ranges",
			tokens: []intralineToken{
				{text: "a", start: 0, end: 1}, {text: " ", start: 1, end: 2}, {text: "b", start: 2, end: 3},
			},
			keep:   []bool{false, false, false},
			expect: []matchRange{{start: 0, end: 1}, {start: 2, end: 3}},
		},
		{
			name: "all kept, no ranges",
			tokens: []intralineToken{
				{text: "foo", start: 0, end: 3}, {text: " ", start: 3, end: 4}, {text: "bar", start: 4, end: 7},
			},
			keep:   []bool{true, true, true},
			expect: nil,
		},
		{
			name:   "empty tokens",
			tokens: nil,
			keep:   nil,
			expect: nil,
		},
		{
			name: "changed word between whitespace",
			tokens: []intralineToken{
				{text: " ", start: 0, end: 1}, {text: "old", start: 1, end: 4}, {text: " ", start: 4, end: 5},
			},
			keep:   []bool{true, false, true},
			expect: []matchRange{{start: 1, end: 4}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildChangedRanges(tc.tokens, tc.keep)
			if tc.expect == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestChangedTokenRanges(t *testing.T) {
	tests := []struct {
		name      string
		minusLine string
		plusLine  string
		wantMinus []matchRange
		wantPlus  []matchRange
	}{
		{
			name:      "single word change",
			minusLine: "return foo(bar)", plusLine: "return foo(baz)",
			wantMinus: []matchRange{{start: 11, end: 14}},
			wantPlus:  []matchRange{{start: 11, end: 14}},
		},
		{
			name:      "identical lines, no ranges",
			minusLine: "hello world", plusLine: "hello world",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:      "fully different",
			minusLine: "aaa bbb", plusLine: "ccc ddd",
			wantMinus: []matchRange{{start: 0, end: 3}, {start: 4, end: 7}},
			wantPlus:  []matchRange{{start: 0, end: 3}, {start: 4, end: 7}},
		},
		{
			name: "empty minus", minusLine: "", plusLine: "something",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name: "empty plus", minusLine: "something", plusLine: "",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:      "multibyte characters",
			minusLine: "日本語 hello", plusLine: "日本語 world",
			wantMinus: []matchRange{{start: 10, end: 15}}, // "hello" starts at byte 10
			wantPlus:  []matchRange{{start: 10, end: 15}}, // "world" starts at byte 10
		},
		{
			name:      "function rename",
			minusLine: "func oldName() {", plusLine: "func newName() {",
			wantMinus: []matchRange{{start: 5, end: 12}}, // "oldName"
			wantPlus:  []matchRange{{start: 5, end: 12}}, // "newName"
		},
		{
			name:      "multiple changes",
			minusLine: "x := foo + bar", plusLine: "y := foo + baz",
			wantMinus: []matchRange{{start: 0, end: 1}, {start: 11, end: 14}},
			wantPlus:  []matchRange{{start: 0, end: 1}, {start: 11, end: 14}},
		},
		{
			name:      "added word",
			minusLine: "a b", plusLine: "a b c",
			wantMinus: nil, wantPlus: []matchRange{{start: 4, end: 5}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMinus, gotPlus := changedTokenRanges(tc.minusLine, tc.plusLine)
			if tc.wantMinus == nil {
				assert.Empty(t, gotMinus, "minus ranges")
			} else {
				assert.Equal(t, tc.wantMinus, gotMinus, "minus ranges")
			}
			if tc.wantPlus == nil {
				assert.Empty(t, gotPlus, "plus ranges")
			} else {
				assert.Equal(t, tc.wantPlus, gotPlus, "plus ranges")
			}
		})
	}
}

func TestChangedTokenRanges_MultibytePrecision(t *testing.T) {
	// verify that byte offsets are correct for multibyte content
	minus := "héllo wörld"
	plus := "héllo earth"

	minusRanges, plusRanges := changedTokenRanges(minus, plus)

	// "wörld" starts at byte offset 7 (h=1, é=2, l=1, l=1, o=1, space=1 = 7)
	require.Len(t, minusRanges, 1)
	assert.Equal(t, 7, minusRanges[0].start)
	assert.Equal(t, "wörld", minus[minusRanges[0].start:minusRanges[0].end])

	// "earth" starts at byte 7 too
	require.Len(t, plusRanges, 1)
	assert.Equal(t, 7, plusRanges[0].start)
	assert.Equal(t, "earth", plus[plusRanges[0].start:plusRanges[0].end])
}

func TestChangedTokenRanges_SkipsVeryLongLines(t *testing.T) {
	// lines above maxLineLenForDiff must skip intra-line diff to prevent
	// LCS memory blowup on pathological input (minified content).
	longMinus := strings.Repeat("a", maxLineLenForDiff+1)
	longPlus := strings.Repeat("b", maxLineLenForDiff+1)

	minusRanges, plusRanges := changedTokenRanges(longMinus, longPlus)
	assert.Nil(t, minusRanges, "very long minus line should skip diff")
	assert.Nil(t, plusRanges, "very long plus line should skip diff")

	// line at exactly the cap still gets diffed
	atCapMinus := strings.Repeat("a", maxLineLenForDiff)
	atCapPlus := strings.Repeat("b", maxLineLenForDiff)
	mr, pr := changedTokenRanges(atCapMinus, atCapPlus)
	assert.NotNil(t, mr, "line at cap should still be diffed")
	assert.NotNil(t, pr, "line at cap should still be diffed")

	// asymmetric: only one line above cap still skips
	shortLine := "abc"
	mr2, pr2 := changedTokenRanges(longMinus, shortLine)
	assert.Nil(t, mr2, "asymmetric long minus should skip")
	assert.Nil(t, pr2, "asymmetric long minus should skip")
}

func TestModel_PairHunkLines(t *testing.T) {
	tests := []struct {
		name  string
		lines []diff.DiffLine
		start int
		end   int
		want  []intralinePair
	}{
		{
			name: "equal count pairs 1:1",
			lines: []diff.DiffLine{
				{Content: "old line 1", ChangeType: diff.ChangeRemove},
				{Content: "old line 2", ChangeType: diff.ChangeRemove},
				{Content: "new line 1", ChangeType: diff.ChangeAdd},
				{Content: "new line 2", ChangeType: diff.ChangeAdd},
			},
			start: 0, end: 4,
			want: []intralinePair{{removeIdx: 0, addIdx: 2}, {removeIdx: 1, addIdx: 3}},
		},
		{
			name: "pure add, no pairs",
			lines: []diff.DiffLine{
				{Content: "added 1", ChangeType: diff.ChangeAdd},
				{Content: "added 2", ChangeType: diff.ChangeAdd},
			},
			start: 0, end: 2,
			want: nil,
		},
		{
			name: "pure remove, no pairs",
			lines: []diff.DiffLine{
				{Content: "removed 1", ChangeType: diff.ChangeRemove},
				{Content: "removed 2", ChangeType: diff.ChangeRemove},
			},
			start: 0, end: 2,
			want: nil,
		},
		{
			name: "unequal count, more adds than removes",
			lines: []diff.DiffLine{
				{Content: "return foo(bar)", ChangeType: diff.ChangeRemove},
				{Content: "return foo(baz)", ChangeType: diff.ChangeAdd},
				{Content: "return extra()", ChangeType: diff.ChangeAdd},
			},
			start: 0, end: 3,
			want: []intralinePair{{removeIdx: 0, addIdx: 1}}, // best match by prefix/suffix
		},
		{
			name: "unequal count, more removes than adds",
			lines: []diff.DiffLine{
				{Content: "return foo(bar)", ChangeType: diff.ChangeRemove},
				{Content: "return extra()", ChangeType: diff.ChangeRemove},
				{Content: "return foo(baz)", ChangeType: diff.ChangeAdd},
			},
			start: 0, end: 3,
			want: []intralinePair{{removeIdx: 0, addIdx: 2}}, // foo(bar)->foo(baz) scores higher
		},
		{
			name: "single pair",
			lines: []diff.DiffLine{
				{Content: "old", ChangeType: diff.ChangeRemove},
				{Content: "new", ChangeType: diff.ChangeAdd},
			},
			start: 0, end: 2,
			want: []intralinePair{{removeIdx: 0, addIdx: 1}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.diffLines = tc.lines
			got := m.pairHunkLines(tc.start, tc.end)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestModel_PairHunkLines_BestMatchScoring(t *testing.T) {
	// verify that greedy scoring picks the best match, not just first match
	m := testModel(nil, nil)
	m.diffLines = []diff.DiffLine{
		{Content: "func alpha() {", ChangeType: diff.ChangeRemove},
		{Content: "func completely_different() {", ChangeType: diff.ChangeAdd},
		{Content: "func alpha(ctx) {", ChangeType: diff.ChangeAdd},
	}
	pairs := m.pairHunkLines(0, 3)
	require.Len(t, pairs, 1)
	// alpha() should pair with alpha(ctx), not completely_different()
	assert.Equal(t, 0, pairs[0].removeIdx)
	assert.Equal(t, 2, pairs[0].addIdx)
}

func TestPassesSimilarityGateFromKeep(t *testing.T) {
	tests := []struct {
		name  string
		minus string
		plus  string
		want  bool
	}{
		{name: "similar lines pass", minus: "return foo(bar)", plus: "return foo(baz)", want: true},
		{name: "identical lines pass", minus: "hello world", plus: "hello world", want: true},
		{name: "dissimilar lines fail", minus: "aaa bbb ccc", plus: "xxx yyy zzz", want: false},
		{name: "empty minus fails", minus: "", plus: "something", want: false},
		{name: "empty plus fails", minus: "something", plus: "", want: false},
		{name: "one of three common tokens passes 33%", minus: "a b c", plus: "a x y", want: true},
		{name: "one of four below threshold 25%", minus: "a b c d", plus: "a x y z", want: false},
		{name: "two of three common tokens pass", minus: "a b c", plus: "a b x", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			minusToks := tokenizeLineWithOffsets(tc.minus)
			plusToks := tokenizeLineWithOffsets(tc.plus)
			if len(minusToks) == 0 || len(plusToks) == 0 {
				assert.False(t, tc.want)
				return
			}
			keepMinus, _ := lcsKeptTokens(minusToks, plusToks)
			got := passesSimilarityGateFromKeep(minusToks, plusToks, keepMinus)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPassesSimilarityGateFromKeep_WhitespaceOnly(t *testing.T) {
	// whitespace-only tokens produce shorter==0, should return false
	minusToks := tokenizeLineWithOffsets("   ")
	plusToks := tokenizeLineWithOffsets("   ")
	require.NotEmpty(t, minusToks, "whitespace should tokenize")
	keepMinus, _ := lcsKeptTokens(minusToks, plusToks)
	assert.False(t, passesSimilarityGateFromKeep(minusToks, plusToks, keepMinus))
}

func TestModel_RecomputeIntraRanges(t *testing.T) {
	m := testModel(nil, nil)
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "context before", ChangeType: diff.ChangeContext},
		{Content: "return foo(bar)", ChangeType: diff.ChangeRemove},
		{Content: "return foo(baz)", ChangeType: diff.ChangeAdd},
		{Content: "context after", ChangeType: diff.ChangeContext},
	}

	m.recomputeIntraRanges()

	require.Len(t, m.intraRanges, 4)
	assert.Nil(t, m.intraRanges[0], "context line should have no ranges")
	assert.NotNil(t, m.intraRanges[1], "remove line should have ranges")
	assert.NotNil(t, m.intraRanges[2], "add line should have ranges")
	assert.Nil(t, m.intraRanges[3], "context line should have no ranges")

	// verify the ranges point to "bar" and "baz"
	require.Len(t, m.intraRanges[1], 1)
	assert.Equal(t, matchRange{start: 11, end: 14}, m.intraRanges[1][0])
	require.Len(t, m.intraRanges[2], 1)
	assert.Equal(t, matchRange{start: 11, end: 14}, m.intraRanges[2][0])
}

func TestModel_RecomputeIntraRanges_IdenticalPair(t *testing.T) {
	m := testModel(nil, nil)
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "same line content", ChangeType: diff.ChangeRemove},
		{Content: "same line content", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// identical lines produce no changed ranges, so intra-line ranges remain nil
	assert.Nil(t, m.intraRanges[0], "identical remove should have no ranges")
	assert.Nil(t, m.intraRanges[1], "identical add should have no ranges")
}

func TestModel_RecomputeIntraRanges_PureAddBlock(t *testing.T) {
	m := testModel(nil, nil)
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "context", ChangeType: diff.ChangeContext},
		{Content: "new line 1", ChangeType: diff.ChangeAdd},
		{Content: "new line 2", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// pure add block has no pairs, so no intra-line ranges
	for i, r := range m.intraRanges {
		assert.Nil(t, r, "line %d should have no ranges", i)
	}
}

func TestModel_RecomputeIntraRanges_DissimilarPair(t *testing.T) {
	m := testModel(nil, nil)
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "alpha bravo charlie delta echo foxtrot golf hotel india juliet", ChangeType: diff.ChangeRemove},
		{Content: "xxx yyy zzz aaa bbb ccc ddd eee fff ggg", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// dissimilar pair should have no ranges due to similarity gate
	assert.Nil(t, m.intraRanges[0])
	assert.Nil(t, m.intraRanges[1])
}

func TestModel_RecomputeIntraRanges_TabContent(t *testing.T) {
	m := testModel(nil, nil)
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "\treturn foo(bar)", ChangeType: diff.ChangeRemove},
		{Content: "\treturn foo(baz)", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// ranges should be on tab-replaced content
	require.NotNil(t, m.intraRanges[0])
	require.NotNil(t, m.intraRanges[1])

	// after tab replacement, "\t" becomes "    " (4 spaces), so "bar" starts at 4+11=15
	tabReplaced := strings.ReplaceAll(m.diffLines[0].Content, "\t", m.tabSpaces)
	require.Len(t, m.intraRanges[0], 1)
	changed := tabReplaced[m.intraRanges[0][0].start:m.intraRanges[0][0].end]
	assert.Equal(t, "bar", changed)
}

func TestModel_RecomputeIntraRanges_MultipleBlocks(t *testing.T) {
	m := testModel(nil, nil)
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "old first", ChangeType: diff.ChangeRemove},
		{Content: "new first", ChangeType: diff.ChangeAdd},
		{Content: "context between", ChangeType: diff.ChangeContext},
		{Content: "old second", ChangeType: diff.ChangeRemove},
		{Content: "new second", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// both blocks should have ranges
	assert.NotNil(t, m.intraRanges[0], "first block remove")
	assert.NotNil(t, m.intraRanges[1], "first block add")
	assert.Nil(t, m.intraRanges[2], "context line")
	assert.NotNil(t, m.intraRanges[3], "second block remove")
	assert.NotNil(t, m.intraRanges[4], "second block add")
}

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{name: "identical", a: "hello", b: "hello", want: 5},
		{name: "common prefix", a: "hello world", b: "hello earth", want: 6},
		{name: "no common", a: "abc", b: "xyz", want: 0},
		{name: "empty a", a: "", b: "xyz", want: 0},
		{name: "empty b", a: "xyz", b: "", want: 0},
		{name: "both empty", a: "", b: "", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, commonPrefixLen(tc.a, tc.b))
		})
	}
}

func TestCommonSuffixLen(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{name: "identical", a: "hello", b: "hello", want: 5},
		{name: "common suffix", a: "old world", b: "new world", want: 6},
		{name: "no common", a: "abc", b: "xyz", want: 0},
		{name: "empty a", a: "", b: "xyz", want: 0},
		{name: "empty b", a: "xyz", b: "", want: 0},
		{name: "both empty", a: "", b: "", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, commonSuffixLen(tc.a, tc.b))
		})
	}
}
