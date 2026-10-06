//go:build darwin || linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	bridge := terminalBridge{input: os.Stdin, output: os.Stdout}
	code, err := bridge.run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

type terminalBridge struct {
	input  io.Reader
	output io.Writer
}

func (b terminalBridge) run(ctx context.Context, args []string) (int, error) {
	if len(args) == 0 {
		return 1, errors.New("usage: runtime-pty <command> [args...]")
	}
	master, slave, err := b.openTerminal()
	if err != nil {
		return 1, err
	}
	defer master.Close()
	defer slave.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // argv comes from the integration fixture, not a shell expansion
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	command.WaitDelay = time.Second
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		if err != nil {
			return fmt.Errorf("stop terminal process group: %w", err)
		}
		return nil
	}
	if err := command.Start(); err != nil {
		return 1, fmt.Errorf("start terminal command: %w", err)
	}
	_ = slave.Close()
	go func() {
		_, _ = io.Copy(master, b.input)
		cancel()
	}()
	drained := make(chan error, 1)
	go func() {
		_, err := io.Copy(b.output, master)
		drained <- err
	}()
	waitErr := command.Wait()
	// The fixture owns the entire session, including a private OpenCode server.
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err := <-drained; err != nil && !errors.Is(err, syscall.EIO) {
		return 1, fmt.Errorf("read terminal output: %w", err)
	}
	if exited, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		status := exited.Sys().(syscall.WaitStatus)
		if status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return status.ExitStatus(), nil
	}
	if waitErr != nil {
		return 1, fmt.Errorf("wait for terminal command: %w", waitErr)
	}
	return 0, nil
}

func (b terminalBridge) openTerminal() (*os.File, *os.File, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open PTY: %w", err)
	}
	if fd < 0 {
		return nil, nil, fmt.Errorf("invalid PTY descriptor: %d", fd)
	}
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	name, err := b.slaveName(fd)
	if err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("resolve PTY: %w", err)
	}
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0) //nolint:gosec // path is returned by the kernel for our PTY
	if err == nil {
		err = unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: 35, Col: 120})
	}
	if err != nil {
		_ = master.Close()
		if slave != nil {
			_ = slave.Close()
		}
		return nil, nil, fmt.Errorf("configure PTY: %w", err)
	}
	return master, slave, nil
}
