package style

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeFilenameForDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "clean ascii", in: "main.go", want: "main.go"},
		{name: "newline stripped", in: "foo\nbar.go", want: "foobar.go"},
		{name: "carriage return stripped", in: "foo\rbar.go", want: "foobar.go"},
		{name: "tab stripped", in: "foo\tbar.go", want: "foobar.go"},
		{name: "esc stripped", in: "foo\x1b[31mbar\x1b[0m.go", want: "foo[31mbar[0m.go"},
		{name: "del stripped", in: "foo\x7fbar.go", want: "foobar.go"},
		{name: "c1 control stripped", in: "foo\x9bbar.go", want: "foobar.go"},
		{name: "rtl override stripped", in: "good\u202egp.os", want: "goodgp.os"},
		{name: "lri stripped", in: "a\u2066b.go", want: "ab.go"},
		{name: "pdi stripped", in: "a\u2069b.go", want: "ab.go"},
		{name: "zwj stripped", in: "a\u200dlogo.go", want: "alogo.go"},
		{name: "zwsp stripped", in: "a\u200bb.go", want: "ab.go"},
		{name: "bom stripped", in: "\ufefffile.go", want: "file.go"},
		{name: "cjk preserved", in: "テスト/ファイル.go", want: "テスト/ファイル.go"},
		{name: "spaces preserved", in: "my file.go", want: "my file.go"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SanitizeFilenameForDisplay(tt.in))
		})
	}
}

func TestTruncateLeftToWidth(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		budget int
		want   string
	}{
		{name: "fits exactly", s: "abcd", budget: 4, want: "abcd"},
		{name: "fits with room", s: "abcd", budget: 10, want: "abcd"},
		{name: "truncates from left", s: "very/long/path/file.go", budget: 10, want: "…h/file.go"},
		{name: "wide chars by display width", s: "テスト.go", budget: 5, want: "….go"},
		{name: "budget zero", s: "anything", budget: 0, want: ""},
		{name: "budget negative", s: "anything", budget: -3, want: ""},
		{name: "budget one", s: "anything", budget: 1, want: "…"},
		{name: "empty string", s: "", budget: 5, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateLeftToWidth(tt.s, tt.budget)
			assert.Equal(t, tt.want, got)
			if tt.budget >= 0 {
				assert.LessOrEqual(t, lipgloss.Width(got), tt.budget, "must fit budget")
			}
		})
	}
}
