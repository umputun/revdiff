package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func (terminalBridge) slaveName(fd int) (string, error) {
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		return "", fmt.Errorf("grant PTY: %w", err)
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		return "", fmt.Errorf("unlock PTY: %w", err)
	}
	var name [128]byte                                                                                                   // Darwin's TIOCPTYGNAME takes a 128-byte pathname buffer.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))) //nolint:gosec // fixed ioctl writes exactly the 128-byte buffer above
	if errno != 0 {
		return "", errno
	}
	return unix.ByteSliceToString(name[:]), nil
}
