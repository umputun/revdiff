package style

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSGRState_scan(t *testing.T) {
	tests := []struct {
		name   string
		init   sgrState
		line   string
		expect sgrState
	}{
		{name: "no sgr sequences", init: sgrState{}, line: "hello world", expect: sgrState{}},
		{name: "single fg 24-bit", init: sgrState{}, line: "\033[38;2;100;200;50m", expect: sgrState{fg: "\033[38;2;100;200;50m"}},
		{name: "single bg 24-bit", init: sgrState{}, line: "\033[48;2;10;20;30m", expect: sgrState{bg: "\033[48;2;10;20;30m"}},
		{name: "bold on", init: sgrState{}, line: "\033[1m", expect: sgrState{bold: true}},
		{name: "italic on", init: sgrState{}, line: "\033[3m", expect: sgrState{italic: true}},
		{name: "reverse on", init: sgrState{}, line: "\033[7m", expect: sgrState{reverse: true}},
		{name: "bold off", init: sgrState{bold: true}, line: "\033[22m", expect: sgrState{}},
		{name: "italic off", init: sgrState{italic: true}, line: "\033[23m", expect: sgrState{}},
		{name: "reverse off", init: sgrState{reverse: true}, line: "\033[27m", expect: sgrState{}},
		{name: "full reset bare", init: sgrState{fg: "\033[31m", bold: true}, line: "\033[m", expect: sgrState{}},
		{name: "full reset 0", init: sgrState{fg: "\033[31m", bg: "\033[42m", italic: true}, line: "\033[0m", expect: sgrState{}},
		{name: "fg reset", init: sgrState{fg: "\033[31m", bg: "\033[42m"}, line: "\033[39m", expect: sgrState{bg: "\033[42m"}},
		{name: "bg reset", init: sgrState{fg: "\033[31m", bg: "\033[42m"}, line: "\033[49m", expect: sgrState{fg: "\033[31m"}},
		{name: "multiple sgrs", init: sgrState{}, line: "\033[38;2;1;2;3mhello\033[1m", expect: sgrState{fg: "\033[38;2;1;2;3m", bold: true}},
		{name: "basic fg", init: sgrState{}, line: "\033[34m", expect: sgrState{fg: "\033[34m"}},
		{name: "basic bg", init: sgrState{}, line: "\033[42m", expect: sgrState{bg: "\033[42m"}},
		{name: "overwrite fg", init: sgrState{fg: "\033[31m"}, line: "\033[32m", expect: sgrState{fg: "\033[32m"}},
		{name: "preserves existing on plain text", init: sgrState{fg: "\033[31m", bold: true}, line: "no ansi", expect: sgrState{fg: "\033[31m", bold: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.init.scan(tt.line)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestSGRState_prefix(t *testing.T) {
	tests := []struct {
		name   string
		state  sgrState
		expect string
	}{
		{name: "empty state", state: sgrState{}, expect: ""},
		{name: "fg only", state: sgrState{fg: "\033[31m"}, expect: "\033[31m"},
		{name: "bg only", state: sgrState{bg: "\033[42m"}, expect: "\033[42m"},
		{name: "fg and bg", state: sgrState{fg: "\033[31m", bg: "\033[42m"}, expect: "\033[31m\033[42m"},
		{name: "bold only", state: sgrState{bold: true}, expect: "\033[1m"},
		{name: "italic only", state: sgrState{italic: true}, expect: "\033[3m"},
		{name: "reverse only", state: sgrState{reverse: true}, expect: "\033[7m"},
		{name: "all attributes", state: sgrState{fg: "\033[31m", bg: "\033[42m", bold: true, italic: true, reverse: true},
			expect: "\033[31m\033[42m\033[1m\033[3m\033[7m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.state.prefix())
		})
	}
}

func TestParseSGR(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		pos       int
		expectSeq string
		expectPar string
		expectEnd int
	}{
		{name: "valid sgr", input: "\033[1m", pos: 0, expectSeq: "\033[1m", expectPar: "1", expectEnd: 3},
		{name: "24-bit color", input: "\033[38;2;100;200;50m", pos: 0, expectSeq: "\033[38;2;100;200;50m", expectPar: "38;2;100;200;50", expectEnd: 17},
		{name: "bare reset", input: "\033[m", pos: 0, expectSeq: "\033[m", expectPar: "", expectEnd: 2},
		{name: "unterminated csi", input: "\033[123", pos: 0, expectSeq: "", expectPar: "", expectEnd: -1},
		{name: "non-sgr csi (H)", input: "\033[2J", pos: 0, expectSeq: "", expectPar: "", expectEnd: 3},
		{name: "mid-string", input: "abc\033[3mdef", pos: 3, expectSeq: "\033[3m", expectPar: "3", expectEnd: 6},
		{name: "string boundary", input: "\033[", pos: 0, expectSeq: "", expectPar: "", expectEnd: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq, params, end := parseSGR(tt.input, tt.pos)
			assert.Equal(t, tt.expectSeq, seq)
			assert.Equal(t, tt.expectPar, params)
			assert.Equal(t, tt.expectEnd, end)
		})
	}
}

func TestIsFgColor(t *testing.T) {
	tests := []struct {
		name   string
		params string
		expect bool
	}{
		{name: "24-bit fg", params: "38;2;100;200;50", expect: true},
		{name: "basic fg 30", params: "30", expect: true},
		{name: "basic fg 37", params: "37", expect: true},
		{name: "basic fg 34", params: "34", expect: true},
		{name: "bg 24-bit rejected", params: "48;2;10;20;30", expect: false},
		{name: "basic bg rejected", params: "42", expect: false},
		{name: "bold rejected", params: "1", expect: false},
		{name: "reset rejected", params: "0", expect: false},
		{name: "fg out of range 38", params: "38", expect: false},
		{name: "fg out of range 39", params: "39", expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, isFgColor(tt.params))
		})
	}
}

func TestIsBgColor(t *testing.T) {
	tests := []struct {
		name   string
		params string
		expect bool
	}{
		{name: "24-bit bg", params: "48;2;10;20;30", expect: true},
		{name: "basic bg 40", params: "40", expect: true},
		{name: "basic bg 47", params: "47", expect: true},
		{name: "basic bg 43", params: "43", expect: true},
		{name: "fg 24-bit rejected", params: "38;2;100;200;50", expect: false},
		{name: "basic fg rejected", params: "34", expect: false},
		{name: "bold rejected", params: "1", expect: false},
		{name: "reset rejected", params: "0", expect: false},
		{name: "bg out of range 48", params: "48", expect: false},
		{name: "bg out of range 49", params: "49", expect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, isBgColor(tt.params))
		})
	}
}

func TestSGR_Reemit(t *testing.T) {
	sgr := SGR{}

	t.Run("empty input", func(t *testing.T) {
		assert.Equal(t, []string{}, sgr.Reemit([]string{}))
	})

	t.Run("single line unchanged", func(t *testing.T) {
		lines := []string{"\033[31mhello"}
		got := sgr.Reemit(lines)
		assert.Equal(t, []string{"\033[31mhello"}, got)
	})

	t.Run("multi-line fg carryover", func(t *testing.T) {
		lines := []string{"\033[38;2;100;200;50mhello", "world", "end"}
		got := sgr.Reemit(lines)
		assert.Equal(t, "\033[38;2;100;200;50mhello", got[0])
		assert.Equal(t, "\033[38;2;100;200;50mworld", got[1])
		assert.Equal(t, "\033[38;2;100;200;50mend", got[2])
	})

	t.Run("reset mid-stream", func(t *testing.T) {
		lines := []string{"\033[1m\033[31mbold red", "\033[0mreset here", "no style"}
		got := sgr.Reemit(lines)
		assert.Equal(t, "\033[1m\033[31mbold red", got[0])
		assert.Equal(t, "\033[31m\033[1m\033[0mreset here", got[1]) // prefix from line 0 state
		assert.Equal(t, "no style", got[2])                         // reset cleared everything
	})

	t.Run("bold and italic carryover", func(t *testing.T) {
		lines := []string{"\033[1m\033[3mbold italic", "continuation"}
		got := sgr.Reemit(lines)
		assert.Equal(t, "\033[1m\033[3mbold italic", got[0])
		assert.Equal(t, "\033[1m\033[3mcontinuation", got[1])
	})

	t.Run("no sgr no prefix", func(t *testing.T) {
		lines := []string{"plain", "text"}
		got := sgr.Reemit(lines)
		assert.Equal(t, "plain", got[0])
		assert.Equal(t, "text", got[1])
	})

	t.Run("bg carryover", func(t *testing.T) {
		lines := []string{"\033[48;2;10;20;30mwith bg", "next"}
		got := sgr.Reemit(lines)
		assert.Equal(t, "\033[48;2;10;20;30mwith bg", got[0])
		assert.Equal(t, "\033[48;2;10;20;30mnext", got[1])
	})
}

func TestSGRState_applySGR(t *testing.T) {
	tests := []struct {
		name   string
		init   sgrState
		params string
		seq    string
		expect sgrState
	}{
		{name: "full reset bare", init: sgrState{fg: "x", bold: true}, params: "", seq: "\033[m", expect: sgrState{}},
		{name: "full reset 0", init: sgrState{bg: "y", italic: true}, params: "0", seq: "\033[0m", expect: sgrState{}},
		{name: "bold on", init: sgrState{}, params: "1", seq: "\033[1m", expect: sgrState{bold: true}},
		{name: "italic on", init: sgrState{}, params: "3", seq: "\033[3m", expect: sgrState{italic: true}},
		{name: "reverse on", init: sgrState{}, params: "7", seq: "\033[7m", expect: sgrState{reverse: true}},
		{name: "bold off", init: sgrState{bold: true}, params: "22", seq: "\033[22m", expect: sgrState{}},
		{name: "italic off", init: sgrState{italic: true}, params: "23", seq: "\033[23m", expect: sgrState{}},
		{name: "reverse off", init: sgrState{reverse: true}, params: "27", seq: "\033[27m", expect: sgrState{}},
		{name: "fg reset", init: sgrState{fg: "x"}, params: "39", seq: "\033[39m", expect: sgrState{}},
		{name: "bg reset", init: sgrState{bg: "y"}, params: "49", seq: "\033[49m", expect: sgrState{}},
		{name: "fg 24-bit", init: sgrState{}, params: "38;2;1;2;3", seq: "\033[38;2;1;2;3m", expect: sgrState{fg: "\033[38;2;1;2;3m"}},
		{name: "bg 24-bit", init: sgrState{}, params: "48;2;4;5;6", seq: "\033[48;2;4;5;6m", expect: sgrState{bg: "\033[48;2;4;5;6m"}},
		{name: "basic fg", init: sgrState{}, params: "34", seq: "\033[34m", expect: sgrState{fg: "\033[34m"}},
		{name: "basic bg", init: sgrState{}, params: "42", seq: "\033[42m", expect: sgrState{bg: "\033[42m"}},
		{name: "unknown param ignored", init: sgrState{fg: "x"}, params: "99", seq: "\033[99m", expect: sgrState{fg: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.init.applySGR(tt.params, tt.seq)
			assert.Equal(t, tt.expect, got)
		})
	}
}
