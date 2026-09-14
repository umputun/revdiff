package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/muesli/termenv"
)

// maskedTerm is the TERM value reported to termenv when the real TERM belongs
// to tmux. any plain xterm value works — it only needs to not match termenv's
// multiplexer prefixes so the OSC query path stays enabled.
const maskedTerm = "xterm-256color"

// tmuxQueryTimeout bounds each tmux subprocess so a wedged tmux server (e.g.
// blocked on a run-shell hook) degrades to the next detection method instead
// of stalling startup indefinitely. display-message and list-clients answer
// over the local socket in single-digit milliseconds when healthy.
const tmuxQueryTimeout = 500 * time.Millisecond

// detectDarkBackground reports whether the terminal background is dark-ish.
// termenv refuses to send OSC status-report queries when TERM starts with
// "tmux" or "screen" (multiplexers historically swallowed them), so inside
// tmux its detection never queries the terminal and always falls back to
// "dark". three tmux-specific paths run in order (tmux >= 3.5 for the first
// two; client themes are fed by each outer terminal's native light/dark
// reporting and need no tty):
//
//  1. the current client's theme via `tmux display-message` — scoped to the
//     launching session's client, so with several terminals attached to one
//     server an unrelated session's client cannot win.
//  2. the server-wide `tmux list-clients` scan — rescues the detached-session
//     topology, where the current client is a nested tmux client (the
//     launcher's display-popup attach) that never learns a theme: the outer
//     terminal's client only shows up in the full list.
//  3. termenv's own OSC 11 query with TERM masked — tmux answers it in
//     regular attached panes with the outer terminal's current background.
//
// if all fail (old tmux, or a detached session where tmux answers no tty
// queries at all), termenv falls back to COLORFGBG and then its dark default.
func detectDarkBackground() bool {
	if !insideTmux(os.Getenv("TMUX"), os.Getenv("TERM"), os.Getenv("TERM_PROGRAM")) {
		return termenv.HasDarkBackground()
	}
	if dark, ok := parseTmuxClientTheme(tmuxClientTheme()); ok {
		return dark
	}
	if dark, ok := pickTmuxClientTheme(tmuxClientThemes()); ok {
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

// tmuxClientTheme asks tmux for the current client's reported theme. returns
// the raw output; empty on error, timeout, or when the format is unknown
// (tmux < 3.5 expands unknown formats to an empty string).
func tmuxClientTheme() string {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxQueryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{client_theme}").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// tmuxClientThemes asks tmux for every server client's reported theme, one
// "<activity-epoch> <theme>" line per client. returns the raw output; empty
// on error, timeout, or when the format is unknown (tmux < 3.5 expands
// unknown formats to an empty string).
func tmuxClientThemes() string {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxQueryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "list-clients", "-F", "#{client_activity} #{client_theme}").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// pickTmuxClientTheme scans tmuxClientThemes output and returns the theme of
// the most recently active client that reports one. Clients with no theme
// (nested tmux clients, terminals without light/dark reporting) are skipped.
// with several real terminals attached to the server, recency is a heuristic
// for "the terminal the user is looking at" — not a guarantee. ok is false
// when no client reports a theme, signaling the caller to fall through to
// the next detection method.
func pickTmuxClientTheme(clientThemes string) (dark, ok bool) {
	bestActivity := int64(-1)
	for line := range strings.SplitSeq(clientThemes, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		activity, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || activity <= bestActivity {
			continue
		}
		if lineDark, themeOK := parseTmuxClientTheme(fields[1]); themeOK {
			bestActivity, dark, ok = activity, lineDark, true
		}
	}
	return dark, ok
}

// parseTmuxClientTheme maps tmux's client_theme value to a dark-background
// bool. ok is false when the theme is unknown or unreported, signaling the
// caller to skip the client.
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
