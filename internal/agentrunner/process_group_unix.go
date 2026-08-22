//go:build darwin || linux

package agentrunner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessGroup(process *exec.Cmd) error {
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error {
		if process.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}

func stopProcessGroup(process *exec.Cmd) error {
	if process.Process == nil {
		return nil
	}
	err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kill process group %d: %w", process.Process.Pid, err)
	}
	return nil
}
