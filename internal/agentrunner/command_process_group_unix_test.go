//go:build darwin || linux

package agentrunner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOSCommandRunnerQuiescesItsDedicatedProcessGroup(t *testing.T) {
	started := time.Now()
	result, err := runPipeHoldingCommand(t, OSCommandRunner{
		WaitDelay:    time.Second,
		ProcessGroup: true,
	}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.WaitDelayed {
		t.Fatalf("process group left an inherited pipe open: %#v", result)
	}
	if !strings.Contains(string(result.Stderr), "grandchild pipe open") {
		t.Fatalf("captured stderr = %q", result.Stderr)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("process group teardown took %s", elapsed)
	}
}

func TestOSCommandRunnerCancelsItsDedicatedProcessGroup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	markerPath := filepath.Join(directory, "group-ready")
	childReadyPath := filepath.Join(directory, "grandchild-ready")
	ctx := newManualDeadlineContext()
	defer ctx.expire()
	type outcome struct {
		result CommandResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := (OSCommandRunner{
			WaitDelay:    100 * time.Millisecond,
			ProcessGroup: true,
		}).Run(ctx, Command{
			Executable: executable,
			Arguments:  []string{"-test.run=^TestCommandRunnerHelperProcess$"},
			Directory:  directory,
			Environment: []string{
				"FORMA_COMMAND_HELPER=spawn-grandchild-and-block",
				"FORMA_COMMAND_MARKER=" + markerPath,
				"FORMA_COMMAND_CHILD_READY=" + childReadyPath,
			},
		})
		finished <- outcome{result: result, err: err}
	}()
	waitForMarker(t, markerPath)
	ctx.expire()
	observed := <-finished
	if !errors.Is(observed.err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", observed.err)
	}
	if !observed.result.TimedOut || observed.result.WaitDelayed {
		t.Fatalf("result = %#v", observed.result)
	}
	if !strings.Contains(string(observed.result.Stderr), "process group ready") {
		t.Fatalf("captured stderr = %q", observed.result.Stderr)
	}
}
