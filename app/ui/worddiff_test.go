package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeIntraRanges_Identical(t *testing.T) {
	rm, add := computeIntraRanges("hello world", "hello world")
	assert.Nil(t, rm)
	assert.Nil(t, add)
}

func TestComputeIntraRanges_SingleWordChange(t *testing.T) {
	rm, add := computeIntraRanges("hello world", "hello globe")
	// "world" -> "globe" ; shared prefix "hello "
	assert.NotEmpty(t, rm)
	assert.NotEmpty(t, add)
	for _, r := range rm {
		assert.GreaterOrEqual(t, r.start, 6)
		assert.LessOrEqual(t, r.end, len("hello world"))
	}
	for _, r := range add {
		assert.GreaterOrEqual(t, r.start, 6)
		assert.LessOrEqual(t, r.end, len("hello globe"))
	}
}

func TestComputeIntraRanges_PrefixChange(t *testing.T) {
	rm, add := computeIntraRanges("foo bar baz", "qux bar baz")
	assert.NotEmpty(t, rm)
	assert.NotEmpty(t, add)
	assert.Equal(t, 0, rm[0].start)
	assert.Equal(t, 0, add[0].start)
}

func TestComputeIntraRanges_SuffixChange(t *testing.T) {
	old := "foo bar baz"
	nw := "foo bar qux"
	rm, add := computeIntraRanges(old, nw)
	assert.NotEmpty(t, rm)
	assert.NotEmpty(t, add)
	last := rm[len(rm)-1]
	assert.Equal(t, len(old), last.end)
	last = add[len(add)-1]
	assert.Equal(t, len(nw), last.end)
}

func TestComputeIntraRanges_EntirelyDifferent(t *testing.T) {
	// Below the similarity threshold — no intra-line highlight produced.
	rm, add := computeIntraRanges("abc", "xyz")
	assert.Nil(t, rm)
	assert.Nil(t, add)
}

func TestComputeIntraRanges_LowSimilaritySkipped(t *testing.T) {
	// Two mostly-unrelated lines that share only a few incidental characters
	// should produce no intra-line highlight (spurious token matches suppressed).
	rm, add := computeIntraRanges(
		"// buildModifiedSet returns a set of diffLines indices",
		"// linePair pairs a remove line index with a matching add",
	)
	assert.Nil(t, rm)
	assert.Nil(t, add)
}

func TestComputeIntraRanges_EmptyOld(t *testing.T) {
	rm, add := computeIntraRanges("", "hello")
	assert.Nil(t, rm)
	assert.Equal(t, []byteRange{{0, 5}}, add)
}

func TestComputeIntraRanges_EmptyNew(t *testing.T) {
	rm, add := computeIntraRanges("hello", "")
	assert.Equal(t, []byteRange{{0, 5}}, rm)
	assert.Nil(t, add)
}

func TestComputeIntraRanges_BothEmpty(t *testing.T) {
	rm, add := computeIntraRanges("", "")
	assert.Nil(t, rm)
	assert.Nil(t, add)
}

func TestComputeIntraRanges_MultiByteUTF8(t *testing.T) {
	// "héllo" -> "héllo wörld" — addition only
	rm, add := computeIntraRanges("héllo", "héllo wörld")
	assert.Empty(t, rm)
	assert.NotEmpty(t, add)
	// added span ends at byte length of new string
	last := add[len(add)-1]
	assert.Equal(t, len("héllo wörld"), last.end)
}

func TestComputeIntraRanges_MultiWordChangeSameLine(t *testing.T) {
	rm, add := computeIntraRanges("the quick brown fox", "the slow red fox")
	assert.NotEmpty(t, rm)
	assert.NotEmpty(t, add)
	// all ranges within bounds
	for _, r := range rm {
		assert.GreaterOrEqual(t, r.start, 0)
		assert.LessOrEqual(t, r.end, len("the quick brown fox"))
		assert.Less(t, r.start, r.end)
	}
	for _, r := range add {
		assert.GreaterOrEqual(t, r.start, 0)
		assert.LessOrEqual(t, r.end, len("the slow red fox"))
		assert.Less(t, r.start, r.end)
	}
}

func TestAppendRange_MergesAdjacent(t *testing.T) {
	r := appendRange(nil, 0, 3)
	r = appendRange(r, 3, 5)
	assert.Equal(t, []byteRange{{0, 5}}, r)
}

func TestAppendRange_SkipsEmpty(t *testing.T) {
	r := appendRange(nil, 5, 5)
	assert.Nil(t, r)
}

func TestInsertBgMarkers_PlainASCII(t *testing.T) {
	got := insertBgMarkers("hello world", []byteRange{{6, 11}}, "[ON]", "[OFF]")
	assert.Equal(t, "hello [ON]world[OFF]", got)
}

func TestInsertBgMarkers_EmptyRanges(t *testing.T) {
	assert.Equal(t, "hello", insertBgMarkers("hello", nil, "[ON]", "[OFF]"))
}

func TestInsertBgMarkers_RangeAtStart(t *testing.T) {
	got := insertBgMarkers("abcdef", []byteRange{{0, 3}}, "(", ")")
	assert.Equal(t, "(abc)def", got)
}

func TestInsertBgMarkers_RangeAtEnd(t *testing.T) {
	got := insertBgMarkers("abcdef", []byteRange{{3, 6}}, "(", ")")
	assert.Equal(t, "abc(def)", got)
}

func TestInsertBgMarkers_MultipleRanges(t *testing.T) {
	got := insertBgMarkers("abcdefghij", []byteRange{{1, 3}, {5, 7}}, "<", ">")
	assert.Equal(t, "a<bc>de<fg>hij", got)
}

func TestInsertBgMarkers_AdjacentRanges(t *testing.T) {
	got := insertBgMarkers("abcdef", []byteRange{{1, 3}, {3, 5}}, "<", ">")
	assert.Equal(t, "a<bc><de>f", got)
}

func TestInsertBgMarkers_SkipsANSIEscapes(t *testing.T) {
	// chroma-style fg escape around "world" — range must land on plain bytes
	in := "hello \033[38;5;12mworld\033[39m"
	got := insertBgMarkers(in, []byteRange{{6, 11}}, "[ON]", "[OFF]")
	// the [ON] should land after the fg escape, before "world"
	assert.Contains(t, got, "[ON]world")
	assert.Contains(t, got, "[OFF]")
	// the OFF marker must come after "world" in visible order
	assert.Greater(t, strings.Index(got, "[OFF]"), strings.Index(got, "world"))
}

func TestInsertBgMarkers_RangeSpanningANSI(t *testing.T) {
	// range spans across an embedded fg escape; ON opens at pos 0, OFF closes at pos 6
	in := "abc\033[38;5;1mdef"
	got := insertBgMarkers(in, []byteRange{{0, 6}}, "<", ">")
	assert.Equal(t, "<abc\033[38;5;1mdef>", got)
}

func TestHighlightIntraLineChanges_NoOpWhenDisabled(t *testing.T) {
	m := Model{}
	assert.Equal(t, "hello", m.highlightIntraLineChanges(0, "hello", true))
}

func TestHighlightIntraLineChanges_ReverseVideoFallback(t *testing.T) {
	m := Model{wordDiff: true, intraRanges: [][]byteRange{{{0, 3}}}}
	// WordAddBg empty → reverse video fallback
	got := m.highlightIntraLineChanges(0, "abcdef", true)
	assert.Equal(t, "\033[7mabc\033[27mdef", got)
}

func TestHighlightIntraLineChanges_NilEntry(t *testing.T) {
	m := Model{wordDiff: true, intraRanges: [][]byteRange{nil}}
	assert.Equal(t, "hello", m.highlightIntraLineChanges(0, "hello", true))
}

func TestHighlightIntraLineChanges_NoColorsModeFallback(t *testing.T) {
	// in --no-colors mode plainStyles leaves colors zeroed, so ansiBg returns ""
	// and the overlay must substitute reverse-video (\033[7m ... \033[27m).
	m := Model{styles: plainStyles(), wordDiff: true, intraRanges: [][]byteRange{{{1, 4}}}}
	gotAdd := m.highlightIntraLineChanges(0, "abcdef", true)
	assert.Equal(t, "a\033[7mbcd\033[27mef", gotAdd)
	gotRemove := m.highlightIntraLineChanges(0, "abcdef", false)
	assert.Equal(t, "a\033[7mbcd\033[27mef", gotRemove)
}
