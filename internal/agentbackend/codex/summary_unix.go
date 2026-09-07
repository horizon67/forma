//go:build darwin || linux

package codex

import (
	"os"
	"syscall"
)

// Nonblocking/no-follow also protects against a replacement FIFO or symlink
// between lstat and open; fstat must still confirm the original regular file.
func openSummary(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
