//go:build darwin || linux

package agentrunner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
)

// WorktreeLocker establishes the fail-closed lifetime advisory lock required
// by forma generate on supported Unix platforms.
type WorktreeLocker struct{}

func (WorktreeLocker) Lock(path string) (io.Closer, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: open canonical worktree directory: %v", ErrWorktreeLockUnavailable, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrWorktreeLocked
		}
		return nil, fmt.Errorf("%w: %v", ErrWorktreeLockUnavailable, err)
	}
	return &unixWorktreeLock{file: file}, nil
}

type unixWorktreeLock struct {
	mu   sync.Mutex
	file *os.File
}

func (lock *unixWorktreeLock) Close() error {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.file == nil {
		return nil
	}
	file := lock.file
	lock.file = nil
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
