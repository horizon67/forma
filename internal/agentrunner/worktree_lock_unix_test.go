//go:build darwin || linux

package agentrunner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWorktreeLockDescriptorIsCloseOnExec(t *testing.T) {
	lockValue, err := (WorktreeLocker{}).Lock(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lockValue.Close()
	lock, ok := lockValue.(*unixWorktreeLock)
	if !ok {
		t.Fatalf("lock type = %T", lockValue)
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, lock.file.Fd(), uintptr(syscall.F_GETFD), 0)
	if errno != 0 {
		t.Fatalf("fcntl(F_GETFD): %v", errno)
	}
	if flags&syscall.FD_CLOEXEC == 0 {
		t.Fatal("worktree lock descriptor is inherited across exec")
	}
}

func TestWorktreeLockerClassifiesContention(t *testing.T) {
	root := t.TempDir()
	first, err := (WorktreeLocker{}).Lock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	_, err = (WorktreeLocker{}).Lock(root)
	if !errors.Is(err, ErrWorktreeLocked) {
		t.Fatalf("error = %v", err)
	}
}

func TestWorktreeLockerRejectsASymlinkInsteadOfFollowingIt(t *testing.T) {
	root := t.TempDir()
	alias := root + "-alias"
	if err := syscall.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Unlink(alias) })
	_, err := (WorktreeLocker{}).Lock(alias)
	if !errors.Is(err, ErrWorktreeLockUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestWorktreeLockDescriptorIsNotInheritedAcrossExec(t *testing.T) {
	root := t.TempDir()
	parentLock, err := (WorktreeLocker{}).Lock(root)
	if err != nil {
		t.Fatal(err)
	}
	process, input := startLockHelper(t, "idle", "")
	defer stopLockHelper(process, input)

	if err := parentLock.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := (WorktreeLocker{}).Lock(root)
	if err != nil {
		t.Fatalf("exec child inherited the lifetime lock: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOperatingSystemReleasesWorktreeLockAfterAbruptProcessExit(t *testing.T) {
	root := t.TempDir()
	process, input := startLockHelper(t, "lock", root)
	defer input.Close()

	contender, err := (WorktreeLocker{}).Lock(root)
	if !errors.Is(err, ErrWorktreeLocked) {
		if contender != nil {
			_ = contender.Close()
		}
		_ = process.Process.Kill()
		_ = process.Wait()
		t.Fatalf("helper did not hold the lock: %v", err)
	}
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err == nil {
		t.Fatal("abruptly killed helper exited successfully")
	}
	reopened, err := (WorktreeLocker{}).Lock(root)
	if err != nil {
		t.Fatalf("OS retained lock after process death: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeLockHelperProcess(t *testing.T) {
	action := os.Getenv("FORMA_LOCK_HELPER")
	if action == "" {
		return
	}
	if action == "lock" {
		lock, err := (WorktreeLocker{}).Lock(os.Getenv("FORMA_LOCK_ROOT"))
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "lock: %v\n", err)
			os.Exit(81)
		}
		defer lock.Close()
	} else if action != "idle" {
		os.Exit(82)
	}
	_, _ = io.WriteString(os.Stdout, "ready\n")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func startLockHelper(t *testing.T, action, root string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorktreeLockHelperProcess$")
	command.Env = append(os.Environ(), "FORMA_LOCK_HELPER="+action, "FORMA_LOCK_ROOT="+filepath.Clean(root))
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("wait for lock helper: %v", err)
	}
	if line != "ready\n" {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("lock helper said %q", line)
	}
	return command, input
}

func stopLockHelper(process *exec.Cmd, input io.WriteCloser) {
	_ = input.Close()
	if err := process.Wait(); err != nil && process.ProcessState == nil {
		_ = process.Process.Kill()
		_ = process.Wait()
	}
}
