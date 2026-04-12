package worddiff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsertHighlightMarkers(t *testing.T) {
	d := New()
	hlOn := "\033[48;2;215;215;0m" // search bg on
	hlOff := "\033[49m"            // bg reset

	tests := []struct {
		name    string
		input   string
		matches []Range
		want    string
	}{
		{
			name:    "no matches returns unchanged",
			input:   "hello world",
			matches: nil,
			want:    "hello world",
		},
		{
			name:    "empty input returns empty",
			input:   "",
			matches: nil,
			want:    "",
		},
		{
			name:    "match at start",
			input:   "hello world",
			matches: []Range{{Start: 0, End: 5}},
			want:    hlOn + "hello" + hlOff + " world",
		},
		{
			name:    "match at end",
			input:   "hello world",
			matches: []Range{{Start: 6, End: 11}},
			want:    "hello " + hlOn + "world" + hlOff,
		},
		{
			name:    "match in middle",
			input:   "say hello world",
			matches: []Range{{Start: 4, End: 9}},
			want:    "say " + hlOn + "hello" + hlOff + " world",
		},
		{
			name:    "two separate matches",
			input:   "ab cd ab",
			matches: []Range{{Start: 0, End: 2}, {Start: 6, End: 8}},
			want:    hlOn + "ab" + hlOff + " cd " + hlOn + "ab" + hlOff,
		},
		{
			name:    "adjacent matches",
			input:   "abcd",
			matches: []Range{{Start: 0, End: 2}, {Start: 2, End: 4}},
			want:    hlOn + "ab" + hlOff + hlOn + "cd" + hlOff,
		},
		{
			name:    "match covers entire string",
			input:   "hello",
			matches: []Range{{Start: 0, End: 5}},
			want:    hlOn + "hello" + hlOff,
		},
		{
			name:    "preserves existing ANSI in non-matched region",
			input:   "\033[32mhello world\033[0m",
			matches: []Range{{Start: 6, End: 11}},
			// match ends at visPos=11, but \033[0m is after 'world' in bytes
			// so the ANSI is copied first (inside match), then hlOff at match boundary
			want: "\033[32mhello " + hlOn + "world\033[0m" + hlOff,
		},
		{
			name:    "preserves ANSI inside matched region",
			input:   "he\033[31mllo wo\033[0mrld",
			matches: []Range{{Start: 0, End: 11}},
			// fg-only \033[31m and full reset \033[0m don't change restoreBg (stays at hlOff),
			// so hlOn is NOT re-emitted after them
			want: hlOn + "he\033[31mllo wo\033[0mrld" + hlOff,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := d.InsertHighlightMarkers(tc.input, tc.matches, hlOn, hlOff)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestInsertHighlightMarkers_BgStateTracking(t *testing.T) {
	d := New()
	hlOn := "\033[48;2;215;215;0m"  // search highlight bg
	lineBg := "\033[48;2;0;34;0m"   // line bg (used as hlOff)
	wordBg := "\033[48;2;45;90;58m" // word-diff bg

	t.Run("bg change inside match re-emits hlOn", func(t *testing.T) {
		// input has a word-diff bg sequence inside a search match
		input := "foo" + wordBg + "bar"
		matches := []Range{{Start: 0, End: 6}}
		got := d.InsertHighlightMarkers(input, matches, hlOn, lineBg)
		// hlOn should be emitted, then word-diff bg changes restoreBg,
		// hlOn re-emitted to maintain search highlight, then restoreBg (wordBg) at end
		assert.Contains(t, got, hlOn+"foo"+wordBg+hlOn+"bar"+wordBg)
	})

	t.Run("bg reset inside match after bg change re-emits hlOn", func(t *testing.T) {
		// wordBg before match changes restoreBg, then \033[0m inside match resets it,
		// which is a different value -> hlOn re-emitted
		input := wordBg + "foo\033[0mbar"
		matches := []Range{{Start: 0, End: 6}}
		got := d.InsertHighlightMarkers(input, matches, hlOn, lineBg)
		// wordBg sets restoreBg=wordBg, hlOn emitted at match start,
		// \033[0m resets restoreBg to lineBg (change!), hlOn re-emitted
		assert.Equal(t, wordBg+hlOn+"foo\033[0m"+hlOn+"bar"+lineBg, got)
	})

	t.Run("fg only change inside match does not re-emit hlOn", func(t *testing.T) {
		// fg-only sequences inside a match should not change restoreBg
		input := "foo\033[31mbar"
		matches := []Range{{Start: 0, End: 6}}
		got := d.InsertHighlightMarkers(input, matches, hlOn, lineBg)
		assert.Equal(t, hlOn+"foo\033[31mbar"+lineBg, got)
	})

	t.Run("bg change before match updates restoreBg for post-match", func(t *testing.T) {
		// word-diff bg set before a search match, match end should restore to wordBg
		input := wordBg + "foobar"
		matches := []Range{{Start: 0, End: 3}}
		got := d.InsertHighlightMarkers(input, matches, hlOn, lineBg)
		// wordBg sets restoreBg=wordBg before match, at match end restore is wordBg
		assert.Equal(t, wordBg+hlOn+"foo"+wordBg+"bar", got)
	})
}

func TestUpdateRestoreBg(t *testing.T) {
	d := New()
	hlOff := "\033[48;2;0;34;0m" // line bg (AddBg)

	tests := []struct {
		name, seq, current, want string
	}{
		{name: "24-bit bg sets restore", seq: "\033[48;2;45;90;58m", current: hlOff, want: "\033[48;2;45;90;58m"},
		{name: "basic bg sets restore", seq: "\033[42m", current: hlOff, want: "\033[42m"},
		{name: "reverse on sets restore", seq: "\033[7m", current: hlOff, want: "\033[7m"},
		{name: "bg reset returns hlOff", seq: "\033[49m", current: "\033[48;2;45;90;58m", want: hlOff},
		{name: "reverse off returns hlOff", seq: "\033[27m", current: "\033[7m", want: hlOff},
		{name: "full reset returns hlOff", seq: "\033[0m", current: "\033[48;2;45;90;58m", want: hlOff},
		{name: "bare reset returns hlOff", seq: "\033[m", current: "\033[7m", want: hlOff},
		{name: "fg color unchanged", seq: "\033[38;2;255;0;0m", current: "\033[48;2;45;90;58m", want: "\033[48;2;45;90;58m"},
		{name: "bold unchanged", seq: "\033[1m", current: hlOff, want: hlOff},
		{name: "too short unchanged", seq: "\033[", current: hlOff, want: hlOff},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := d.updateRestoreBg(tc.seq, tc.current, hlOff)
			assert.Equal(t, tc.want, got)
		})
	}
}
