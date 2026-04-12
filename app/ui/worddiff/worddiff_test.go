package worddiff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changedRangesHelper is a test-only helper that computes changed ranges without the
// similarity gate. this exercises tokenize + LCS + buildChangedRanges directly.
func changedRangesHelper(d *Differ, minusLine, plusLine string) ([]Range, []Range) {
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
	return d.buildChangedRanges(minusToks, keepMinus), d.buildChangedRanges(plusToks, keepPlus)
}

func TestTokenizeLineWithOffsets(t *testing.T) {
	d := New()
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
			got := d.tokenizeLineWithOffsets(tc.line)
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
	d := New()
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
			gotMinus, gotPlus := d.lcsKeptTokens(tc.minus, tc.plus)
			assert.Equal(t, tc.keepMinus, gotMinus, "keepMinus")
			assert.Equal(t, tc.keepPlus, gotPlus, "keepPlus")
		})
	}
}

func TestBuildChangedRanges(t *testing.T) {
	d := New()
	tests := []struct {
		name   string
		tokens []intralineToken
		keep   []bool
		expect []Range
	}{
		{
			name: "single changed word",
			tokens: []intralineToken{
				{text: "foo", start: 0, end: 3}, {text: " ", start: 3, end: 4}, {text: "bar", start: 4, end: 7},
			},
			keep:   []bool{true, true, false},
			expect: []Range{{Start: 4, End: 7}},
		},
		{
			name: "adjacent changed words merge",
			tokens: []intralineToken{
				{text: "aaa", start: 0, end: 3}, {text: "bbb", start: 3, end: 6},
				{text: " ", start: 6, end: 7}, {text: "ccc", start: 7, end: 10},
			},
			keep:   []bool{false, false, true, true},
			expect: []Range{{Start: 0, End: 6}},
		},
		{
			name: "whitespace excluded from ranges",
			tokens: []intralineToken{
				{text: "a", start: 0, end: 1}, {text: " ", start: 1, end: 2}, {text: "b", start: 2, end: 3},
			},
			keep:   []bool{false, false, false},
			expect: []Range{{Start: 0, End: 1}, {Start: 2, End: 3}},
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
			expect: []Range{{Start: 1, End: 4}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := d.buildChangedRanges(tc.tokens, tc.keep)
			if tc.expect == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestChangedRanges(t *testing.T) {
	d := New()
	tests := []struct {
		name      string
		minusLine string
		plusLine  string
		wantMinus []Range
		wantPlus  []Range
	}{
		{
			name:      "single word change",
			minusLine: "return foo(bar)", plusLine: "return foo(baz)",
			wantMinus: []Range{{Start: 11, End: 14}},
			wantPlus:  []Range{{Start: 11, End: 14}},
		},
		{
			name:      "identical lines, no ranges",
			minusLine: "hello world", plusLine: "hello world",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:      "fully different",
			minusLine: "aaa bbb", plusLine: "ccc ddd",
			wantMinus: []Range{{Start: 0, End: 3}, {Start: 4, End: 7}},
			wantPlus:  []Range{{Start: 0, End: 3}, {Start: 4, End: 7}},
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
			wantMinus: []Range{{Start: 10, End: 15}},
			wantPlus:  []Range{{Start: 10, End: 15}},
		},
		{
			name:      "function rename",
			minusLine: "func oldName() {", plusLine: "func newName() {",
			wantMinus: []Range{{Start: 5, End: 12}},
			wantPlus:  []Range{{Start: 5, End: 12}},
		},
		{
			name:      "multiple changes",
			minusLine: "x := foo + bar", plusLine: "y := foo + baz",
			wantMinus: []Range{{Start: 0, End: 1}, {Start: 11, End: 14}},
			wantPlus:  []Range{{Start: 0, End: 1}, {Start: 11, End: 14}},
		},
		{
			name:      "added word",
			minusLine: "a b", plusLine: "a b c",
			wantMinus: nil, wantPlus: []Range{{Start: 4, End: 5}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMinus, gotPlus := changedRangesHelper(d, tc.minusLine, tc.plusLine)
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

func TestChangedRanges_MultibytePrecision(t *testing.T) {
	d := New()
	// verify that byte offsets are correct for multibyte content
	minus := "héllo wörld"
	plus := "héllo earth"

	minusRanges, plusRanges := changedRangesHelper(d, minus, plus)

	// "wörld" starts at byte offset 7 (h=1, é=2, l=1, l=1, o=1, space=1 = 7)
	require.Len(t, minusRanges, 1)
	assert.Equal(t, 7, minusRanges[0].Start)
	assert.Equal(t, "wörld", minus[minusRanges[0].Start:minusRanges[0].End])

	// "earth" starts at byte 7 too
	require.Len(t, plusRanges, 1)
	assert.Equal(t, 7, plusRanges[0].Start)
	assert.Equal(t, "earth", plus[plusRanges[0].Start:plusRanges[0].End])
}

func TestChangedRanges_SkipsVeryLongLines(t *testing.T) {
	d := New()
	// lines above maxLineLenForDiff must skip intra-line diff to prevent
	// LCS memory blowup on pathological input (minified content).
	longMinus := strings.Repeat("a", maxLineLenForDiff+1)
	longPlus := strings.Repeat("b", maxLineLenForDiff+1)

	minusRanges, plusRanges := changedRangesHelper(d, longMinus, longPlus)
	assert.Nil(t, minusRanges, "very long minus line should skip diff")
	assert.Nil(t, plusRanges, "very long plus line should skip diff")

	// line at exactly the cap still gets diffed
	atCapMinus := strings.Repeat("a", maxLineLenForDiff)
	atCapPlus := strings.Repeat("b", maxLineLenForDiff)
	mr, pr := changedRangesHelper(d, atCapMinus, atCapPlus)
	assert.NotNil(t, mr, "line at cap should still be diffed")
	assert.NotNil(t, pr, "line at cap should still be diffed")

	// asymmetric: only one line above cap still skips
	shortLine := "abc"
	mr2, pr2 := changedRangesHelper(d, longMinus, shortLine)
	assert.Nil(t, mr2, "asymmetric long minus should skip")
	assert.Nil(t, pr2, "asymmetric long minus should skip")
}

func TestPassesSimilarityGateFromKeep(t *testing.T) {
	d := New()
	tests := []struct {
		name  string
		minus string
		plus  string
		want  bool
	}{
		{name: "similar lines pass", minus: "return foo(bar)", plus: "return foo(baz)", want: true},
		{name: "identical lines pass", minus: "hello world", plus: "hello world", want: true},
		{name: "dissimilar lines fail", minus: "aaa bbb ccc", plus: "xxx yyy zzz", want: false},
		{name: "one of three common tokens passes 33%", minus: "a b c", plus: "a x y", want: true},
		{name: "one of four below threshold 25%", minus: "a b c d", plus: "a x y z", want: false},
		{name: "two of three common tokens pass", minus: "a b c", plus: "a b x", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			minusToks := d.tokenizeLineWithOffsets(tc.minus)
			plusToks := d.tokenizeLineWithOffsets(tc.plus)
			keepMinus, _ := d.lcsKeptTokens(minusToks, plusToks)
			got := d.passesSimilarityGateFromKeep(minusToks, plusToks, keepMinus)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPassesSimilarityGateFromKeep_WhitespaceOnly(t *testing.T) {
	d := New()
	// whitespace-only tokens produce shorter==0, should return false
	minusToks := d.tokenizeLineWithOffsets("   ")
	plusToks := d.tokenizeLineWithOffsets("   ")
	require.NotEmpty(t, minusToks, "whitespace should tokenize")
	keepMinus, _ := d.lcsKeptTokens(minusToks, plusToks)
	assert.False(t, d.passesSimilarityGateFromKeep(minusToks, plusToks, keepMinus))
}

func TestComputeIntraRanges(t *testing.T) {
	d := New()
	tests := []struct {
		name      string
		minus     string
		plus      string
		wantMinus []Range
		wantPlus  []Range
	}{
		{
			name:  "similar pair returns ranges",
			minus: "return foo(bar)", plus: "return foo(baz)",
			wantMinus: []Range{{Start: 11, End: 14}},
			wantPlus:  []Range{{Start: 11, End: 14}},
		},
		{
			name:  "dissimilar pair returns nil",
			minus: "aaa bbb ccc ddd", plus: "xxx yyy zzz www",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:  "identical lines return nil",
			minus: "hello world", plus: "hello world",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:  "empty minus returns nil",
			minus: "", plus: "something",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:  "empty plus returns nil",
			minus: "something", plus: "",
			wantMinus: nil, wantPlus: nil,
		},
		{
			name:      "very long lines return nil",
			minus:     strings.Repeat("a", maxLineLenForDiff+1),
			plus:      strings.Repeat("b", maxLineLenForDiff+1),
			wantMinus: nil, wantPlus: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMinus, gotPlus := d.ComputeIntraRanges(tc.minus, tc.plus)
			if tc.wantMinus == nil {
				assert.Nil(t, gotMinus, "minus ranges")
			} else {
				assert.Equal(t, tc.wantMinus, gotMinus, "minus ranges")
			}
			if tc.wantPlus == nil {
				assert.Nil(t, gotPlus, "plus ranges")
			} else {
				assert.Equal(t, tc.wantPlus, gotPlus, "plus ranges")
			}
		})
	}
}

func TestCommonPrefixLen(t *testing.T) {
	d := New()
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
			assert.Equal(t, tc.want, d.commonPrefixLen(tc.a, tc.b))
		})
	}
}

func TestCommonSuffixLen(t *testing.T) {
	d := New()
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
			assert.Equal(t, tc.want, d.commonSuffixLen(tc.a, tc.b))
		})
	}
}

func TestPairLines(t *testing.T) {
	d := New()
	tests := []struct {
		name  string
		lines []LinePair
		want  []Pair
	}{
		{
			name: "equal count pairs 1:1",
			lines: []LinePair{
				{Content: "old line 1", IsRemove: true},
				{Content: "old line 2", IsRemove: true},
				{Content: "new line 1", IsRemove: false},
				{Content: "new line 2", IsRemove: false},
			},
			want: []Pair{{RemoveIdx: 0, AddIdx: 2}, {RemoveIdx: 1, AddIdx: 3}},
		},
		{
			name: "pure add, no pairs",
			lines: []LinePair{
				{Content: "added 1", IsRemove: false},
				{Content: "added 2", IsRemove: false},
			},
			want: nil,
		},
		{
			name: "pure remove, no pairs",
			lines: []LinePair{
				{Content: "removed 1", IsRemove: true},
				{Content: "removed 2", IsRemove: true},
			},
			want: nil,
		},
		{
			name: "unequal count, more adds than removes",
			lines: []LinePair{
				{Content: "return foo(bar)", IsRemove: true},
				{Content: "return foo(baz)", IsRemove: false},
				{Content: "return extra()", IsRemove: false},
			},
			want: []Pair{{RemoveIdx: 0, AddIdx: 1}},
		},
		{
			name: "unequal count, more removes than adds",
			lines: []LinePair{
				{Content: "return foo(bar)", IsRemove: true},
				{Content: "return extra()", IsRemove: true},
				{Content: "return foo(baz)", IsRemove: false},
			},
			want: []Pair{{RemoveIdx: 0, AddIdx: 2}},
		},
		{
			name: "single pair",
			lines: []LinePair{
				{Content: "old", IsRemove: true},
				{Content: "new", IsRemove: false},
			},
			want: []Pair{{RemoveIdx: 0, AddIdx: 1}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := d.PairLines(tc.lines)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPairLines_BestMatchScoring(t *testing.T) {
	d := New()
	// verify that greedy scoring picks the best match, not just first match
	lines := []LinePair{
		{Content: "func alpha() {", IsRemove: true},
		{Content: "func completely_different() {", IsRemove: false},
		{Content: "func alpha(ctx) {", IsRemove: false},
	}
	pairs := d.PairLines(lines)
	require.Len(t, pairs, 1)
	// alpha() should pair with alpha(ctx), not completely_different()
	assert.Equal(t, 0, pairs[0].RemoveIdx)
	assert.Equal(t, 2, pairs[0].AddIdx)
}
