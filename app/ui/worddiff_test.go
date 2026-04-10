package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
