package agentrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Command describes one process invocation without a shell. Executable and
// Directory must be absolute, and Arguments are passed as distinct argv
// elements. An unset Environment means an empty child environment.
type Command struct {
	Executable  string
	Arguments   []string
	Directory   string
	Environment []string
	Stdin       []byte
	// Internal process adapters may stream stdout to a bounded, nonblocking
	// parser instead of retaining it. The parser must never perform terminal I/O.
	StdoutSink    io.Writer
	DiscardStderr bool
	OnStarted     func()
}

// CommandResult contains the captured result of a process that was started.
// A non-zero process exit is represented by ExitCode rather than Run returning
// an error. Run errors mean the process could not be started or observed.
type CommandResult struct {
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	TimedOut        bool
	WaitDelayed     bool
	ExitCode        int
	CleanupFailed   bool
}

// CommandRunner is the process boundary used by the reference agent runner.
// Tests inject this interface so every executable, argv element, working
// directory, environment entry, and stdin byte can be asserted before host
// execution is introduced.
type CommandRunner interface {
	Run(context.Context, Command) (CommandResult, error)
}

// OSCommandRunner executes commands directly without invoking a shell. It
// keeps reading after a capture limit is reached so a child cannot block on a
// full pipe, while making capture and inherited-pipe truncation explicit to
// the caller. ProcessGroup opts a command into dedicated group cancellation
// and post-exit teardown; Git and other short preflight commands leave it off.
type OSCommandRunner struct {
	CaptureLimit int
	WaitDelay    time.Duration
	ProcessGroup bool
}

const defaultCaptureLimit = 1 << 20
const defaultWaitDelay = time.Second

func (runner OSCommandRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	if !filepath.IsAbs(command.Executable) {
		return CommandResult{}, fmt.Errorf("executable must be absolute: %q", command.Executable)
	}
	if !filepath.IsAbs(command.Directory) {
		return CommandResult{}, fmt.Errorf("directory must be absolute: %q", command.Directory)
	}

	process := exec.CommandContext(ctx, command.Executable, command.Arguments...)
	process.Dir = command.Directory
	// os/exec inherits os.Environ only when Env is nil. Always assign a
	// non-nil slice so an unset Environment cannot expose parent credentials.
	process.Env = append([]string{}, command.Environment...)
	process.Stdin = bytes.NewReader(command.Stdin)
	waitDelay := runner.WaitDelay
	if waitDelay <= 0 {
		waitDelay = defaultWaitDelay
	}
	process.WaitDelay = waitDelay
	limit := runner.CaptureLimit
	if limit <= 0 {
		limit = defaultCaptureLimit
	}
	stdout := newBoundedBuffer(limit)
	stderr := newBoundedBuffer(limit)
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return CommandResult{}, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return CommandResult{}, fmt.Errorf("create stderr pipe: %w", err)
	}
	closePipes := func() {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
	}
	process.Stdout = stdoutWriter
	process.Stderr = stderrWriter
	if runner.ProcessGroup {
		if err := configureProcessGroup(process); err != nil {
			closePipes()
			return CommandResult{}, err
		}
	}
	if err := process.Start(); err != nil {
		closePipes()
		return CommandResult{}, fmt.Errorf("start %s: %w", command.Executable, err)
	}
	if command.OnStarted != nil {
		command.OnStarted()
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	captureDone := make(chan struct{}, 2)
	var stdoutSink io.Writer = &stdout
	if command.StdoutSink != nil {
		stdoutSink = command.StdoutSink
	}
	var stderrSink io.Writer = &stderr
	if command.DiscardStderr {
		stderrSink = io.Discard
	}
	go captureOutput(stdoutReader, stdoutSink, captureDone)
	go captureOutput(stderrReader, stderrSink, captureDone)

	err = process.Wait()
	var groupErr error
	if runner.ProcessGroup {
		groupErr = stopProcessGroup(process)
	}
	waitDelayed := finishOutputCapture(captureDone, stdoutReader, stderrReader, waitDelay)
	result := CommandResult{
		Stdout:          stdout.Bytes(),
		Stderr:          stderr.Bytes(),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
		WaitDelayed:     waitDelayed,
		ExitCode:        0,
		CleanupFailed:   groupErr != nil || waitDelayed,
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}
	if terminalErr := commandContextOrGroupError(ctx, command.Executable, groupErr, &result); terminalErr != nil {
		return result, terminalErr
	}
	if err == nil {
		if waitDelayed {
			return result, fmt.Errorf("wait for %s output pipes: %w", command.Executable, exec.ErrWaitDelay)
		}
		return result, nil
	}
	if exitError == nil {
		return result, fmt.Errorf("wait for %s: %w", command.Executable, err)
	}
	return result, nil
}

func commandContextOrGroupError(ctx context.Context, executable string, groupErr error, result *CommandResult) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.TimedOut = errors.Is(ctxErr, context.DeadlineExceeded)
		if groupErr != nil {
			// A context-triggered group kill can race with process exit and PID/group
			// reuse. Preserve the cancellation as the primary outcome while keeping
			// a real teardown error available to errors.Is and diagnostics.
			return errors.Join(ctxErr, fmt.Errorf("terminate process group for %s: %w", executable, groupErr))
		}
		return ctxErr
	}
	if groupErr != nil {
		return fmt.Errorf("terminate process group for %s: %w", executable, groupErr)
	}
	return nil
}

func captureOutput(reader *os.File, buffer io.Writer, done chan<- struct{}) {
	_, _ = io.Copy(buffer, reader)
	_ = reader.Close()
	done <- struct{}{}
}

func finishOutputCapture(done <-chan struct{}, stdout, stderr *os.File, waitDelay time.Duration) bool {
	timer := time.NewTimer(waitDelay)
	defer timer.Stop()
	completed := 0
	for completed < 2 {
		select {
		case <-done:
			completed++
		case <-timer.C:
			_ = stdout.Close()
			_ = stderr.Close()
			for completed < 2 {
				<-done
				completed++
			}
			return true
		}
	}
	return false
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	remaining int
	truncated bool
}

func newBoundedBuffer(limit int) boundedBuffer {
	return boundedBuffer{remaining: limit}
}

func (buffer *boundedBuffer) Write(content []byte) (int, error) {
	consumed := len(content)
	if len(content) > buffer.remaining {
		content = content[:buffer.remaining]
		buffer.truncated = true
	}
	if len(content) > 0 {
		_, _ = buffer.buffer.Write(content)
		buffer.remaining -= len(content)
	}
	return consumed, nil
}

func (buffer *boundedBuffer) Bytes() []byte {
	return append([]byte(nil), buffer.buffer.Bytes()...)
}

func (buffer *boundedBuffer) Truncated() bool { return buffer.truncated }
