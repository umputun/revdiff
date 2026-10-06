package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func (terminalBridge) slaveName(fd int) (string, error) {
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return "", fmt.Errorf("unlock PTY: %w", err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return "", fmt.Errorf("get PTY number: %w", err)
	}
	return fmt.Sprintf("/dev/pts/%d", number), nil
}
