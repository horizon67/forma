//go:build darwin || linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func generationSignalContext() (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// A disconnected progress pipe is an observation failure, not a reason to
	// abandon child processes, pending history, or the worktree lock.
	pipe := make(chan os.Signal, 1)
	signal.Notify(pipe, syscall.SIGPIPE)
	return ctx, func() { stop(); signal.Stop(pipe) }
}
