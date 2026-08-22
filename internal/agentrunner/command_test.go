package agentrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOSCommandRunnerPreservesTheExplicitProcessBoundary(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	stdin := []byte("canonical request bytes\n")
	arguments := []string{
		"-test.run=^TestCommandRunnerHelperProcess$",
		"--",
		"$(touch should-not-exist)",
		"value with spaces",
	}
	environment := []string{"FORMA_COMMAND_HELPER=inspect", "LANG=C"}

	result, err := (OSCommandRunner{}).Run(context.Background(), Command{
		Executable:  executable,
		Arguments:   arguments,
		Directory:   directory,
		Environment: environment,
		Stdin:       stdin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", result.ExitCode, result.Stderr)
	}
	var observed struct {
		Arguments   []string `json:"arguments"`
		Directory   string   `json:"directory"`
		Environment []string `json:"environment"`
		Stdin       string   `json:"stdin"`
	}
	if err := json.Unmarshal(result.Stdout, &observed); err != nil {
		t.Fatalf("decode helper output: %v\n%s", err, result.Stdout)
	}
	if want := arguments[2:]; !reflect.DeepEqual(observed.Arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", observed.Arguments, want)
	}
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Directory != canonicalDirectory {
		t.Fatalf("directory = %q, want %q", observed.Directory, canonicalDirectory)
	}
	if !reflect.DeepEqual(observed.Environment, environment) {
		t.Fatalf("environment = %#v, want %#v", observed.Environment, environment)
	}
	if observed.Stdin != string(stdin) {
		t.Fatalf("stdin = %q, want %q", observed.Stdin, stdin)
	}
	if _, err := os.Stat(filepath.Join(directory, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("argument was interpreted by a shell: %v", err)
	}
}

func TestOSCommandRunnerReturnsANonZeroExitAsAResult(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := (OSCommandRunner{}).Run(context.Background(), Command{
		Executable:  executable,
		Arguments:   []string{"-test.run=^TestCommandRunnerHelperProcess$"},
		Directory:   t.TempDir(),
		Environment: []string{"FORMA_COMMAND_HELPER=exit-17"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 17 || string(result.Stderr) != "intentional failure\n" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOSCommandRunnerRejectsAPathSearch(t *testing.T) {
	_, err := (OSCommandRunner{}).Run(context.Background(), Command{Executable: "git"})
	if err == nil || !strings.Contains(err.Error(), "executable must be absolute") {
		t.Fatalf("error = %v", err)
	}
}

func TestOSCommandRunnerRejectsAnImplicitWorkingDirectory(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	_, err = (OSCommandRunner{}).Run(context.Background(), Command{Executable: executable})
	if err == nil || !strings.Contains(err.Error(), "directory must be absolute") {
		t.Fatalf("error = %v", err)
	}
}

func TestOSCommandRunnerDoesNotImplicitlyInheritTheParentEnvironment(t *testing.T) {
	envExecutable, err := exec.LookPath("env")
	if err != nil {
		t.Skip("env executable is unavailable")
	}
	envExecutable, err = filepath.Abs(envExecutable)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORMA_PARENT_SECRET", "must-not-be-inherited")
	result, err := (OSCommandRunner{}).Run(context.Background(), Command{
		Executable:  envExecutable,
		Directory:   t.TempDir(),
		Environment: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit %d: %s", result.ExitCode, result.Stderr)
	}
	if strings.Contains(string(result.Stdout), "FORMA_PARENT_SECRET") {
		t.Fatalf("parent environment leaked:\n%s", result.Stdout)
	}
}

func TestOSCommandRunnerTreatsAnUnsetEnvironmentAsEmpty(t *testing.T) {
	envExecutable, err := exec.LookPath("env")
	if err != nil {
		t.Skip("env executable is unavailable")
	}
	envExecutable, err = filepath.Abs(envExecutable)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORMA_PARENT_SECRET", "must-not-be-inherited")
	result, err := (OSCommandRunner{}).Run(context.Background(), Command{
		Executable: envExecutable,
		Directory:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result.Stdout), "FORMA_PARENT_SECRET") {
		t.Fatalf("unset environment inherited parent values:\n%s", result.Stdout)
	}
}

func TestOSCommandRunnerBoundsCapturedOutputWithoutBlockingTheChild(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := (OSCommandRunner{CaptureLimit: 8}).Run(context.Background(), Command{
		Executable: executable,
		Arguments:  []string{"-test.run=^TestCommandRunnerHelperProcess$"},
		Directory:  t.TempDir(),
		Environment: []string{
			"FORMA_COMMAND_HELPER=large-output",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(result.Stdout), "01234567"; got != want || !result.StdoutTruncated {
		t.Fatalf("stdout/truncated = %q / %t, want %q / true", got, result.StdoutTruncated, want)
	}
	if got, want := string(result.Stderr), "abcdefgh"; got != want || !result.StderrTruncated {
		t.Fatalf("stderr/truncated = %q / %t, want %q / true", got, result.StderrTruncated, want)
	}
}

func TestOSCommandRunnerBoundsInheritedPipeWait(t *testing.T) {
	started := time.Now()
	result, err := runPipeHoldingCommand(t, OSCommandRunner{WaitDelay: 30 * time.Millisecond}, 0, true)
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("error = %v", err)
	}
	if !result.WaitDelayed {
		t.Fatalf("result = %#v", result)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("pipe wait took %s", elapsed)
	}
	if !strings.Contains(string(result.Stderr), "grandchild pipe open") {
		t.Fatalf("captured stderr = %q", result.Stderr)
	}
}

func TestOSCommandRunnerMarksInheritedPipeCutoffAfterNonZeroExit(t *testing.T) {
	result, err := runPipeHoldingCommand(t, OSCommandRunner{WaitDelay: 30 * time.Millisecond}, 3, true)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result.ExitCode != 3 || !result.WaitDelayed {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(string(result.Stderr), "grandchild pipe open") {
		t.Fatalf("captured stderr = %q", result.Stderr)
	}
}

func TestOSCommandRunnerPreservesDiagnosticsOnContextDeadline(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	markerPath := filepath.Join(directory, "diagnostics-written")
	ctx := newManualDeadlineContext()
	defer ctx.expire()
	type outcome struct {
		result CommandResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := (OSCommandRunner{WaitDelay: 30 * time.Millisecond}).Run(ctx, Command{
			Executable: executable,
			Arguments:  []string{"-test.run=^TestCommandRunnerHelperProcess$"},
			Directory:  directory,
			Environment: []string{
				"FORMA_COMMAND_HELPER=block-until-timeout",
				"FORMA_COMMAND_MARKER=" + markerPath,
			},
		})
		finished <- outcome{result: result, err: err}
	}()
	waitForMarker(t, markerPath)
	ctx.expire()
	observed := <-finished
	result, err := observed.result, observed.err
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if !result.TimedOut || !strings.Contains(string(result.Stderr), "before timeout") {
		t.Fatalf("result = %#v", result)
	}
}

func TestCommandRunnerHelperProcess(t *testing.T) {
	switch os.Getenv("FORMA_COMMAND_HELPER") {
	case "":
		return
	case "exit-17":
		_, _ = io.WriteString(os.Stderr, "intentional failure\n")
		os.Exit(17)
	case "inspect":
		directory, err := os.Getwd()
		if err != nil {
			os.Exit(90)
		}
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(91)
		}
		separator := -1
		for index, argument := range os.Args {
			if argument == "--" {
				separator = index
				break
			}
		}
		arguments := []string{}
		if separator >= 0 {
			arguments = append(arguments, os.Args[separator+1:]...)
		}
		environment := []string{}
		for _, item := range os.Environ() {
			if strings.HasPrefix(item, "FORMA_COMMAND_HELPER=") || strings.HasPrefix(item, "LANG=") {
				environment = append(environment, item)
			}
		}
		payload := struct {
			Arguments   []string `json:"arguments"`
			Directory   string   `json:"directory"`
			Environment []string `json:"environment"`
			Stdin       string   `json:"stdin"`
		}{arguments, directory, environment, string(stdin)}
		var output bytes.Buffer
		if err := json.NewEncoder(&output).Encode(payload); err != nil {
			os.Exit(92)
		}
		_, _ = os.Stdout.Write(output.Bytes())
		os.Exit(0)
	case "large-output":
		_, _ = io.WriteString(os.Stdout, "0123456789abcdef")
		_, _ = io.WriteString(os.Stderr, "abcdefghijklmnop")
		os.Exit(0)
	case "leave-pipe-open":
		if err := startPipeHoldingGrandchild(); err != nil {
			os.Exit(95)
		}
		exitCode, _ := strconv.Atoi(os.Getenv("FORMA_COMMAND_EXIT"))
		os.Exit(exitCode)
	case "spawn-grandchild-and-block":
		if err := startPipeHoldingGrandchild(); err != nil {
			os.Exit(96)
		}
		_, _ = io.WriteString(os.Stderr, "process group ready\n")
		_ = os.WriteFile(os.Getenv("FORMA_COMMAND_MARKER"), []byte("ready\n"), 0o600)
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "hold-pipes":
		_, _ = io.WriteString(os.Stderr, "grandchild pipe open\n")
		_ = os.WriteFile(os.Getenv("FORMA_COMMAND_CHILD_READY"), []byte("ready\n"), 0o600)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("FORMA_COMMAND_STOP")); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = os.WriteFile(os.Getenv("FORMA_COMMAND_DONE"), []byte("done\n"), 0o600)
		os.Exit(0)
	case "block-until-timeout":
		_, _ = io.WriteString(os.Stderr, "before timeout\n")
		_ = os.WriteFile(os.Getenv("FORMA_COMMAND_MARKER"), []byte("ready\n"), 0o600)
		time.Sleep(5 * time.Second)
		os.Exit(0)
	default:
		os.Exit(93)
	}
}

func runPipeHoldingCommand(t *testing.T, runner OSCommandRunner, exitCode int, waitForGrandchild bool) (CommandResult, error) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	stopPath := filepath.Join(directory, "stop-grandchild")
	donePath := filepath.Join(directory, "grandchild-done")
	readyPath := filepath.Join(directory, "grandchild-ready")
	defer func() { _ = os.WriteFile(stopPath, []byte("stop\n"), 0o600) }()
	result, runErr := runner.Run(context.Background(), Command{
		Executable: executable,
		Arguments:  []string{"-test.run=^TestCommandRunnerHelperProcess$"},
		Directory:  directory,
		Environment: []string{
			"FORMA_COMMAND_HELPER=leave-pipe-open",
			"FORMA_COMMAND_STOP=" + stopPath,
			"FORMA_COMMAND_DONE=" + donePath,
			"FORMA_COMMAND_CHILD_READY=" + readyPath,
			"FORMA_COMMAND_EXIT=" + strconv.Itoa(exitCode),
		},
	})
	_ = os.WriteFile(stopPath, []byte("stop\n"), 0o600)
	if waitForGrandchild {
		waitForMarker(t, donePath)
	}
	return result, runErr
}

func waitForMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("marker was not created: %s", path)
}

func startPipeHoldingGrandchild() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	grandchild := exec.Command(executable, "-test.run=^TestCommandRunnerHelperProcess$")
	grandchild.Env = []string{
		"FORMA_COMMAND_HELPER=hold-pipes",
		"FORMA_COMMAND_STOP=" + os.Getenv("FORMA_COMMAND_STOP"),
		"FORMA_COMMAND_DONE=" + os.Getenv("FORMA_COMMAND_DONE"),
		"FORMA_COMMAND_CHILD_READY=" + os.Getenv("FORMA_COMMAND_CHILD_READY"),
	}
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	if err := grandchild.Start(); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(os.Getenv("FORMA_COMMAND_CHILD_READY")); err == nil {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("grandchild did not signal readiness")
}

type manualDeadlineContext struct {
	done chan struct{}
	once sync.Once
}

func newManualDeadlineContext() *manualDeadlineContext {
	return &manualDeadlineContext{done: make(chan struct{})}
}

func (ctx *manualDeadlineContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *manualDeadlineContext) Done() <-chan struct{}       { return ctx.done }
func (ctx *manualDeadlineContext) Value(any) any               { return nil }
func (ctx *manualDeadlineContext) Err() error {
	select {
	case <-ctx.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}
func (ctx *manualDeadlineContext) expire() { ctx.once.Do(func() { close(ctx.done) }) }
