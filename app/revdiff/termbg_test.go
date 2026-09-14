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

func TestPickTmuxClientTheme(t *testing.T) {
	tests := []struct {
		name         string
		clientThemes string
		wantDark     bool
		wantOK       bool
	}{
		{name: "single light client", clientThemes: "1784717957 light\n", wantDark: false, wantOK: true},
		{name: "single dark client", clientThemes: "1784717957 dark\n", wantDark: true, wantOK: true},
		{
			name:         "themeless popup client alongside real client",
			clientThemes: "1784717957 light\n1784718000 \n",
			wantDark:     false,
			wantOK:       true,
		},
		{
			name:         "most recently active themed client wins",
			clientThemes: "1784717000 dark\n1784718000 light\n",
			wantDark:     false,
			wantOK:       true,
		},
		{
			name:         "order independent",
			clientThemes: "1784718000 light\n1784717000 dark\n",
			wantDark:     false,
			wantOK:       true,
		},
		{
			name:         "equal activity keeps first listed",
			clientThemes: "1784718000 dark\n1784718000 light\n",
			wantDark:     true,
			wantOK:       true,
		},
		{
			name:         "line with extra fields skipped",
			clientThemes: "1784718000 light extra\n1784717000 dark\n",
			wantDark:     true,
			wantOK:       true,
		},
		{name: "no clients", clientThemes: "", wantOK: false},
		{name: "only themeless clients", clientThemes: "1784717957 \n1784718000 \n", wantOK: false},
		{name: "unknown theme value skipped", clientThemes: "1784717957 auto\n", wantOK: false},
		{name: "malformed activity skipped", clientThemes: "notanumber light\n", wantOK: false},
		{name: "tmux pre-3.5 empty theme format", clientThemes: "1784717957 \n", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dark, ok := pickTmuxClientTheme(tt.clientThemes)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantDark, dark)
			}
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
		{name: "trailing newline", theme: "light\n", wantDark: false, wantOK: true},
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
