//go:build !darwin && !linux

package agentrunner

import (
	"errors"
	"os/exec"
)

var errProcessGroupUnavailable = errors.New("dedicated process groups are unavailable on this platform")

func configureProcessGroup(*exec.Cmd) error { return errProcessGroupUnavailable }

func stopProcessGroup(*exec.Cmd) error { return errProcessGroupUnavailable }
