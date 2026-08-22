//go:build !darwin && !linux

package agentrunner

import (
	"fmt"
	"io"
)

// WorktreeLocker fails closed on platforms where the alpha advisory-lock
// contract has not been implemented and tested.
type WorktreeLocker struct{}

func (WorktreeLocker) Lock(string) (io.Closer, error) {
	return nil, fmt.Errorf("%w on this platform", ErrWorktreeLockUnavailable)
}
