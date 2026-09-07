//go:build !darwin && !linux

package codex

import (
	"errors"
	"os"
)

func openSummary(string) (*os.File, error) {
	return nil, errors.New("safe summary file reading is unsupported on this platform")
}
