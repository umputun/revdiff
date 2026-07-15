package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/muesli/termenv"
)

// maskedTerm is the TERM value reported to termenv when the real TERM belongs
// to tmux. any plain xterm value works — it only needs to not match termenv's
// multiplexer prefixes so the OSC query path stays enabled.
const maskedTerm = "xterm-256color"

// tmuxQueryTimeout bounds the tmux subprocess so a wedged tmux server (e.g.
// blocked on a run-shell hook) degrades to the next detection method instead
// of stalling startup indefinitely. display-message answers over the local
// socket in single-digit milliseconds when healthy.
const tmuxQueryTimeout = 500 * time.Millisecond

// detectDarkBackground reports whether the terminal background is dark-ish.
// termenv refuses to send OSC status-report queries when TERM starts with
// "tmux" or "screen" (multiplexers historically swallowed them), so inside
// tmux its detection never queries the terminal and always falls back to
// "dark". two tmux-specific paths run first:
//
//  1. `tmux display-message -p '#{client_theme}'` (tmux >= 3.5) — fed by the
//     outer terminal's native light/dark reporting, needs no tty, and works
//     in display-popup panes where tmux does not answer OSC 11 at all.
//  2. termenv's own OSC 11 query with TERM masked — tmux answers it in
//     regular panes with the outer terminal's current background.
//
// if both fail (old tmux inside a popup), termenv falls back to COLORFGBG
// and then its dark default — same as before.
func detectDarkBackground() bool {
	if !insideTmux(os.Getenv("TMUX"), os.Getenv("TERM"), os.Getenv("TERM_PROGRAM")) {
		return termenv.HasDarkBackground()
	}
	if dark, ok := parseTmuxClientTheme(tmuxClientTheme()); ok {
		return dark
	}
	return termenv.NewOutput(os.Stdout, termenv.WithEnvironment(tmuxEnviron{})).HasDarkBackground()
}

// insideTmux reports whether the process runs inside tmux: the $TMUX socket
// variable is the canonical signal; TERM=tmux-* and TERM=screen-* with
// TERM_PROGRAM=tmux cover environments that scrub $TMUX. plain GNU screen is
// excluded — it does not answer OSC 11, so masking TERM would only add a
// query round-trip.
func insideTmux(tmuxSocket, term, termProgram string) bool {
	if tmuxSocket != "" {
		return true
	}
	if strings.HasPrefix(term, "tmux") {
		return true
	}
	return strings.HasPrefix(term, "screen") && termProgram == "tmux"
}

// tmuxClientTheme asks tmux for the attached client's reported theme.
// returns the raw output; empty on error, timeout, or when the format is
// unknown (tmux < 3.5 expands unknown formats to an empty string).
// best-effort: with several clients attached to the session, tmux resolves
// the format against its notion of the current client — the OSC 11 fallback
// is relayed through the same client choice, so this path is no worse.
func tmuxClientTheme() string {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxQueryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{client_theme}").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// parseTmuxClientTheme maps tmux's client_theme output to a dark-background
// bool. ok is false when the theme is unknown or unreported, signaling the
// caller to fall through to the next detection method.
func parseTmuxClientTheme(theme string) (dark, ok bool) {
	switch strings.TrimSpace(theme) {
	case "dark":
		return true, true
	case "light":
		return false, true
	default:
		return false, false
	}
}

// tmuxEnviron exposes the process environment to termenv with TERM masked to
// a plain xterm value, re-enabling termenv's OSC 11 background query inside
// tmux. all other variables pass through unchanged.
type tmuxEnviron struct{}

// Environ returns the process environment with any TERM entry masked.
func (tmuxEnviron) Environ() []string {
	env := os.Environ()
	for i, kv := range env {
		if strings.HasPrefix(kv, "TERM=") {
			env[i] = "TERM=" + maskedTerm
		}
	}
	return env
}

// Getenv returns the masked TERM value for "TERM" and the real environment
// value for every other key.
func (tmuxEnviron) Getenv(key string) string {
	if key == "TERM" {
		return maskedTerm
	}
	return os.Getenv(key)
}
