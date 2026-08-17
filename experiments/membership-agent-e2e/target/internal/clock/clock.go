// Package clock supplies the current time through an interface so tests can
// place an operation before, at, or after a verification expiry boundary
// without pre-computing an "expired" flag.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// System returns the wall clock used by the running server.
func System() Clock { return systemClock{} }

// Fixed is a controllable clock. Tests advance it to cross an expiry boundary
// instead of writing an expired record directly, so the expiry rule itself is
// exercised.
type Fixed struct {
	mu  sync.Mutex
	now time.Time
}

func NewFixed(start time.Time) *Fixed { return &Fixed{now: start.UTC()} }

func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
