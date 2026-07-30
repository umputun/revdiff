package main

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartPprof_EmptyAddrIsNoop(t *testing.T) {
	t.Log("startPprof with an empty addr must not panic or bind a listener")
	startPprof("")
}

func TestStartPprof_ServesDebugEndpoint(t *testing.T) {
	// port 0 lets the OS pick a free port, but startPprof only accepts a fixed
	// addr string, so bind to loopback on an OS-assigned port ourselves first
	// to get a free one, then hand that exact addr to startPprof.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	startPprof(addr)

	var status int
	require.Eventually(t, func() bool {
		resp, getErr := http.Get("http://" + addr + "/debug/pprof/")
		if getErr != nil {
			return false
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		status = resp.StatusCode
		return true
	}, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, http.StatusOK, status)
}
