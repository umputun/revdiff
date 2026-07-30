package main

import (
	"errors"
	"log"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // intentional: --pprof opts in to this debug endpoint, and it stays off unless the flag is set
	"time"
)

// startPprof launches a debug-only pprof http server in the background when
// addr is non-empty. It never blocks or fails run() — a bind error (e.g. the
// port is already taken) is logged and profiling is simply unavailable for
// that session, which isn't worth aborting a review over.
func startPprof(addr string) {
	if addr == "" {
		return
	}
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[WARN] pprof server on %s: %v", addr, err)
		}
	}()
}
