//go:build !darwin && !linux

package main

import (
	"context"
	"os"
	"os/signal"
)

func generationSignalContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
