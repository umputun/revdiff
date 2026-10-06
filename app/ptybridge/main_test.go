//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTerminalIOAndExitCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	go func() { _, _ = io.WriteString(writer, "hello\n") }()
	var output bytes.Buffer
	bridge := terminalBridge{input: input, output: &output}
	code, err := bridge.run(ctx, []string{"/bin/sh", "-c", `
test -t 0 && test -t 1 && test -t 2 || exit 90
stty size < /dev/tty
IFS= read -r value
printf 'received:%s\nargument:%s\n' "$value" "$1"
printf 'stderr-line\n' >&2
exit 7
`, "fixture", "a b"})
	require.NoError(t, err)
	require.Equal(t, 7, code, output.String())
	require.Contains(t, output.String(), "35 120")
	require.Contains(t, output.String(), "received:hello")
	require.Contains(t, output.String(), "argument:a b")
	require.Contains(t, output.String(), "stderr-line")
}

func TestCancellationReapsTerminalChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	input, inputWriter := io.Pipe()
	defer input.Close()
	defer inputWriter.Close()
	output, outputWriter := io.Pipe()
	defer output.Close()
	defer outputWriter.Close()
	bridge := terminalBridge{input: input, output: outputWriter}
	type result struct {
		code int
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		code, err := bridge.run(ctx, []string{"/bin/sh", "-c", `trap '' TERM HUP; printf '%s\n' "$$"; while :; do sleep 1; done`})
		_ = outputWriter.Close()
		finished <- result{code: code, err: err}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
	cancel()
	_, _ = io.Copy(io.Discard, output)
	outcome := <-finished
	require.NoError(t, outcome.err)
	require.Equal(t, 137, outcome.code)
	require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
	reaped = true
}
