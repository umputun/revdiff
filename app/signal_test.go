package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShutdownGuard_Trigger(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{name: "single trigger", count: 1},
		{name: "double trigger dedupes", count: 2},
		{name: "burst trigger dedupes", count: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var g shutdownGuard
			quitCalls := 0
			quit := func() { quitCalls++ }
			for range tt.count {
				g.trigger(quit)
			}
			assert.Equal(t, 1, quitCalls)
			assert.True(t, g.wasSignaled())
		})
	}
}

func TestShutdownGuard_Handle(t *testing.T) {
	tests := []struct {
		name         string
		signals      []os.Signal
		wantQuit     int
		wantSignaled bool
	}{
		{name: "single signal", signals: []os.Signal{syscall.SIGHUP}, wantQuit: 1, wantSignaled: true},
		{name: "hup then term burst", signals: []os.Signal{syscall.SIGHUP, syscall.SIGTERM}, wantQuit: 1, wantSignaled: true},
		{name: "term signal", signals: []os.Signal{syscall.SIGTERM}, wantQuit: 1, wantSignaled: true},
		{name: "sigint drains without quit", signals: []os.Signal{syscall.SIGINT}, wantQuit: 0, wantSignaled: false},
		{name: "sigint then hup still quits", signals: []os.Signal{syscall.SIGINT, syscall.SIGHUP}, wantQuit: 1, wantSignaled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var g shutdownGuard
			quitCalls := 0
			quit := func() { quitCalls++ }
			ch := make(chan os.Signal, len(tt.signals))
			for _, s := range tt.signals {
				ch <- s
			}
			close(ch)
			g.handle(ch, quit)
			assert.Equal(t, tt.wantQuit, quitCalls)
			assert.Equal(t, tt.wantSignaled, g.wasSignaled())
		})
	}
}

func TestShutdownGuard_WasSignaledDefault(t *testing.T) {
	var g shutdownGuard
	assert.False(t, g.wasSignaled())
}

// handle drives the quitter's Quit exactly once across a signal burst — the
// quitter interface method value is the seam watch passes to handle in run().
func TestShutdownGuard_HandleWithQuitter(t *testing.T) {
	var g shutdownGuard
	q := &quitterMock{QuitFunc: func() {}}

	ch := make(chan os.Signal, 2)
	ch <- syscall.SIGHUP
	ch <- syscall.SIGTERM
	close(ch)
	g.handle(ch, q.Quit)

	assert.Len(t, q.QuitCalls(), 1)
	assert.True(t, g.wasSignaled())
}

// watch is a lifecycle smoke test: it installs signal.Notify for SIGHUP/SIGTERM/
// SIGINT (SIGINT registered so a Ctrl-C meant for an external $EDITOR does not
// default-terminate revdiff; handle drains it without quitting) and returns an
// idempotent stop func whose signal.Stop restores default disposition. no signal
// is delivered (raising a real OS signal at the test process is avoided), so the
// delivery -> quit path is covered by the handle tests instead, not here.
func TestShutdownGuard_Watch(t *testing.T) {
	var g shutdownGuard
	q := &quitterMock{QuitFunc: func() {}}

	stop := g.watch(q)
	require.NotNil(t, stop)
	stop()
	stop() // idempotent: safe to call more than once (double close would panic without the guard)

	assert.Empty(t, q.QuitCalls())
	assert.False(t, g.wasSignaled())
}
