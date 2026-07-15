package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsideTmux(t *testing.T) {
	tests := []struct {
		name        string
		tmuxSocket  string
		term        string
		termProgram string
		want        bool
	}{
		{name: "TMUX socket set", tmuxSocket: "/tmp/tmux-501/default,123,0", term: "xterm-256color", want: true},
		{name: "tmux TERM", term: "tmux-256color", want: true},
		{name: "plain tmux TERM", term: "tmux", want: true},
		{name: "screen TERM under tmux", term: "screen-256color", termProgram: "tmux", want: true},
		{name: "screen TERM without tmux", term: "screen-256color", want: false},
		{name: "plain GNU screen", term: "screen", termProgram: "", want: false},
		{name: "ghostty", term: "xterm-ghostty", want: false},
		{name: "xterm", term: "xterm-256color", want: false},
		{name: "empty TERM", term: "", want: false},
		{name: "empty TERM with tmux program", term: "", termProgram: "tmux", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, insideTmux(tt.tmuxSocket, tt.term, tt.termProgram))
		})
	}
}

func TestParseTmuxClientTheme(t *testing.T) {
	tests := []struct {
		name     string
		theme    string
		wantDark bool
		wantOK   bool
	}{
		{name: "dark", theme: "dark", wantDark: true, wantOK: true},
		{name: "light", theme: "light", wantDark: false, wantOK: true},
		{name: "trailing newline from display-message", theme: "light\n", wantDark: false, wantOK: true},
		{name: "empty means unreported", theme: "", wantOK: false},
		{name: "whitespace only", theme: "\n", wantOK: false},
		{name: "unknown value", theme: "auto", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dark, ok := parseTmuxClientTheme(tt.theme)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantDark, dark)
			}
		})
	}
}

func TestTmuxEnviron_Getenv(t *testing.T) {
	t.Setenv("TERM", "tmux-256color")
	t.Setenv("REVDIFF_TEST_PASSTHROUGH", "value")

	env := tmuxEnviron{}
	assert.Equal(t, maskedTerm, env.Getenv("TERM"), "TERM should be masked")
	assert.Equal(t, "value", env.Getenv("REVDIFF_TEST_PASSTHROUGH"), "other keys should pass through")
}

func TestTmuxEnviron_Environ(t *testing.T) {
	t.Setenv("TERM", "tmux-256color")

	env := tmuxEnviron{}.Environ()
	assert.Contains(t, env, "TERM="+maskedTerm, "TERM entry should be masked")
	assert.NotContains(t, env, "TERM=tmux-256color", "real TERM should not leak through")
}
