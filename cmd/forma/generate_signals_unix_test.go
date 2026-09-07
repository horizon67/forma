//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationprogress"
)

type signalReady struct {
	Agent, Child int
	Summary      string
}

func TestGenerateJSONOnRealTTYAndRedirectedOutput(t *testing.T) {
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("system PTY utility is unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	repo, _, _, _ := historyFixture(t)
	// Use a pre-candidate usage error to exercise the real CLI's JSON routing
	// without requiring AI authentication, then compare pipe and PTY modes.
	commandArgs := []string{executable, "-test.run=^TestGenerationSignalHelper$", "--", "parent", "unused", "--repository", repo, "--progress=json", "--verbose", "--heartbeat-interval=0s"}
	for _, terminal := range []bool{false, true} {
		var command *exec.Cmd
		if !terminal {
			command = exec.Command(commandArgs[0], commandArgs[1:]...)
		} else if runtime.GOOS == "darwin" {
			command = exec.Command(script, append([]string{"-q", "/dev/null"}, commandArgs...)...)
		} else {
			quoted := make([]string, len(commandArgs))
			for i, arg := range commandArgs {
				quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
			}
			command = exec.Command(script, "-q", "-e", "-c", strings.Join(quoted, " "), "/dev/null")
		}
		// PTYs combine the human stdout diagnostic with stderr; in pipe mode
		// inspect stderr directly. Only progress-prefixed JSON belongs to stderr.
		var out, errout bytes.Buffer
		command.Stdout = &out
		command.Stderr = &errout
		stdin, input, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		command.Stdin = stdin
		defer stdin.Close()
		defer input.Close() // Avoid script injecting an echoed EOF into the first line.
		_ = command.Run()
		content := errout.String()
		if terminal {
			content = strings.ReplaceAll(out.String(), "\r\n", "\n")
		}
		var jsonLines []string
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, `{"schema":`) {
				jsonLines = append(jsonLines, line)
			}
		}
		if !terminal && strings.TrimSpace(content) != strings.Join(jsonLines, "\n") {
			t.Fatal("non-JSON stderr", content)
		}
		events := decodeProgress(t, strings.Join(jsonLines, "\n"))
		if events[len(events)-1].Status != generationprogress.Failed || strings.Contains(content, "\x1b") {
			t.Fatalf("TTY=%v: %s", terminal, content)
		}
	}
}

// The fake agent completes only after the real parent CLI has rendered its
// running heartbeat. This tests text/PTY output without a timing-based sleep.
type heartbeatAckWriter struct {
	output io.Writer
	marker string
	once   sync.Once
}

func (w *heartbeatAckWriter) Write(b []byte) (int, error) {
	n, err := w.output.Write(b)
	if err == nil && bytes.Contains(b, []byte("agent running (waiting; heartbeat")) {
		w.once.Do(func() { _ = os.WriteFile(w.marker, []byte("observed"), 0600) })
	}
	return n, err
}

func TestGenerateDefaultTextPhasesAndHeartbeatOnRealTTYAndPipe(t *testing.T) {
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("system PTY utility is unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	var reference []string
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprintf("tty=%v", terminal), func(t *testing.T) {
			_, _, _, args := historyFixture(t)
			control := t.TempDir()
			shim := filepath.Join(control, "codex")
			body := "#!/bin/sh\nexec " + quote(executable) + " -test.run=^TestGenerationSignalHelper$ -- finite-agent " + quote(control) + " \"$@\"\n"
			if err := os.WriteFile(shim, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			commandArgs := append([]string{executable, "-test.run=^TestGenerationSignalHelper$", "--", "text-parent", shim}, args[1:]...)
			commandArgs = append(commandArgs, "--heartbeat-interval=1s") // default text, not JSON
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var command *exec.Cmd
			if !terminal {
				command = exec.CommandContext(ctx, commandArgs[0], commandArgs[1:]...)
			} else if runtime.GOOS == "darwin" {
				command = exec.CommandContext(ctx, script, append([]string{"-q", "/dev/null"}, commandArgs...)...)
			} else {
				quoted := make([]string, len(commandArgs))
				for i, arg := range commandArgs {
					quoted[i] = quote(arg)
				}
				command = exec.CommandContext(ctx, script, "-q", "-e", "-c", strings.Join(quoted, " "), "/dev/null")
			}
			var out, errout bytes.Buffer
			command.Stdout = &out
			command.Stderr = &errout
			stdin, input, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			command.Stdin = stdin
			defer stdin.Close()
			defer input.Close() // Keep PTY input open until the child finishes.
			if err := command.Run(); err != nil {
				t.Fatalf("CLI: %v\n%s\n%s", err, &out, &errout)
			}
			if !strings.Contains(out.String(), "Press Ctrl+C to cancel safely") || !strings.Contains(out.String(), "comparison baseline saved") {
				t.Fatal("missing cancellation hint or completion", out.String())
			}
			content := errout.String()
			if terminal {
				content = strings.ReplaceAll(out.String(), "\r\n", "\n")
			}
			if strings.ContainsAny(content, "\r\x1b\b") {
				t.Fatal("display contains redraw controls")
			}
			labels := map[string]bool{}
			for _, line := range strings.Split(content, "\n") {
				if !strings.HasPrefix(line, "forma: [") {
					continue
				}
				_, label, ok := strings.Cut(line, "] ")
				if !ok {
					t.Fatal("invalid text phase", line)
				}
				label, _, _ = strings.Cut(label, "; remaining")
				label, _, _ = strings.Cut(label, "; last agent activity")
				// Slow CI may emit extra waiting heartbeats in preflight/cleanup;
				// compare lifecycle and the synchronized running heartbeat only.
				if strings.Contains(label, "(waiting;") && !strings.HasPrefix(label, "agent running") {
					continue
				}
				labels[label] = true
			}
			for _, want := range []string{"compiling Forma source", "validating repository", "selecting generation baseline", "preparing agent (authentication and immutable input)", "starting agent", "agent running", "agent running (waiting; heartbeat is not evidence of agent activity)", "cleaning up agent resources", "capturing final repository status", "saving generation history", "completed"} {
				if !labels[want] {
					t.Fatalf("missing %q:\n%s", want, content)
				}
			}
			var actual []string
			for label := range labels {
				actual = append(actual, label)
			}
			sort.Strings(actual)
			if !terminal {
				reference = actual
			} else if strings.Join(actual, "\n") != strings.Join(reference, "\n") {
				t.Fatalf("PTY and pipe labels differ: %v / %v", reference, actual)
			}
		})
	}
}

func TestGenerationSignalsStopDescendantsReleaseLockAndProtectBaseline(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			repo, source, manifest, args := historyFixture(t)
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				return beginTestGeneration(t, in), nil
			}
			runHistoryCommand(t, args, 0)
			_, _, before := readHistoryRecord(t, repo, repo, source)
			writeTestFile(t, manifest, strings.Replace(updateManifest, "value: bun", "value: pnpm", 1))
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			control := t.TempDir()
			shim := filepath.Join(control, "codex")
			readyPath := filepath.Join(control, "ready.json")
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
			script := "#!/bin/sh\nexec " + quote(executable) + " -test.run=^TestGenerationSignalHelper$ -- agent " + quote(control) + " \"$@\"\n"
			if err := os.WriteFile(shim, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			childArgs := append([]string{"-test.run=^TestGenerationSignalHelper$", "--", "parent", shim}, args[1:]...)
			childArgs = append(childArgs, "--progress=json", "--verbose", "--heartbeat-interval=1s")
			command := exec.CommandContext(ctx, executable, childArgs...)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() { finished <- command.Wait() }()
			var ready signalReady
			t.Cleanup(func() {
				if command.Process != nil {
					_ = command.Process.Kill()
				}
				if ready.Agent > 0 {
					_ = syscall.Kill(-ready.Agent, syscall.SIGKILL)
				}
			})
			waitSignalFile(t, readyPath)
			data, err := os.ReadFile(readyPath)
			if err != nil || json.Unmarshal(data, &ready) != nil || ready.Agent <= 0 || ready.Child <= 0 {
				t.Fatalf("bad ready record: %s %v", data, err)
			}
			if lock, err := (agentrunner.WorktreeLocker{}).Lock(repo); !errors.Is(err, agentrunner.ErrWorktreeLocked) {
				if lock != nil {
					_ = lock.Close()
				}
				t.Fatal("executing CLI did not hold lock", err)
			}
			if err := command.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-finished:
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 {
					t.Fatalf("exit: %v\n%s\n%s", err, &stdout, &stderr)
				}
			case <-ctx.Done():
				t.Fatal("interrupted CLI did not finish")
			}
			events := decodeProgress(t, stderr.String())
			if events[len(events)-1].Status != generationprogress.Cancelled {
				t.Fatal(events)
			}
			partial, running := false, false
			for _, e := range events {
				partial = partial || e.Code == generationprogress.PartialMutation
				running = running || e.Phase == generationprogress.AgentRunning
			}
			if !partial || !running {
				t.Fatal("missing start or partial mutation notice")
			}
			if strings.Contains(stderr.String(), "SECRET") {
				t.Fatal("raw agent stream leaked")
			}
			if _, err := os.Stat(filepath.Join(repo, "partial.txt")); err != nil {
				t.Fatal("interruption unexpectedly rolled back mutation")
			}
			store, _, after := readHistoryRecord(t, repo, repo, source)
			if store.PendingKey == "" || !after.Pending || after.Baseline.Request != before.Baseline.Request || after.LastAttempt.Status != "failed" {
				t.Fatal("interruption promoted baseline")
			}
			lock, err := (agentrunner.WorktreeLocker{}).Lock(repo)
			if err != nil {
				t.Fatal("lock was not released", err)
			}
			if err := lock.Close(); err != nil {
				t.Fatal(err)
			}
			for _, pid := range []int{ready.Agent, ready.Child} {
				assertSignalProcessStopped(t, pid)
			}
			if _, err := os.Stat(filepath.Dir(ready.Summary)); !os.IsNotExist(err) {
				t.Fatal("private summary directory retained", err)
			}
		})
	}
}

func waitSignalFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("process did not become ready:", path)
		case <-ticker.C:
		}
	}
}
func assertSignalProcessStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		// Linux containers may briefly leave reparented zombies; they cannot
		// execute or retain descriptors and are not live descendants.
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant %d survived cancellation", pid)
}

func TestGenerationSignalHelper(t *testing.T) {
	index := -1
	for i, arg := range os.Args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	if len(args) < 2 {
		t.Fatal("helper arguments")
	}
	switch args[0] {
	case "parent", "text-parent":
		shim := args[1]
		old := findExecutable
		findExecutable = func(name string) (string, error) {
			if name == "codex" {
				return shim, nil
			}
			return old(name)
		}
		ctx, stop := generationSignalContext()
		var progress io.Writer = os.Stderr
		if args[0] == "text-parent" {
			progress = &heartbeatAckWriter{output: os.Stderr, marker: filepath.Join(filepath.Dir(shim), "heartbeat-observed")}
		}
		code := runGenerateCommand(ctx, args[2:], os.Stdout, progress)
		stop()
		os.Exit(code)
	case "leaf":
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "agent", "finite-agent":
		control := args[1]
		if len(args) < 3 {
			t.Fatal("missing invocation")
		}
		if args[2] == "login" {
			os.Exit(0)
		}
		if args[2] != "exec" {
			t.Fatal("unexpected invocation")
		}
		_, _ = io.Copy(io.Discard, os.Stdin)
		repo, summary := "", ""
		for i, arg := range args {
			if arg == "-C" {
				repo = args[i+1]
			}
			if arg == "--output-last-message" {
				summary = args[i+1]
			}
		}
		if repo == "" || summary == "" {
			t.Fatal("missing repository or summary")
		}
		if err := os.WriteFile(filepath.Join(repo, "partial.txt"), []byte("partial mutation\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if args[0] == "finite-agent" {
			fmt.Fprintln(os.Stdout, `{"type":"turn.started"}`)
			waitSignalFile(t, filepath.Join(control, "heartbeat-observed"))
			if err := os.WriteFile(summary, []byte("Completed stand-in; verification not run."), 0600); err != nil {
				t.Fatal(err)
			}
			os.Exit(0)
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		leafPath := filepath.Join(control, "leaf-ready")
		leaf := exec.Command(executable, "-test.run=^TestGenerationSignalHelper$", "--", "leaf", leafPath)
		leaf.Stdout = os.Stdout
		leaf.Stderr = os.Stderr
		if err := leaf.Start(); err != nil {
			t.Fatal(err)
		}
		waitSignalFile(t, leafPath)
		fmt.Fprintln(os.Stdout, `{"type":"turn.started"}`)
		fmt.Fprintln(os.Stdout, `{"type":"item.started","item":{"type":"command_execution","command":"SECRET"}}`)
		fmt.Fprintln(os.Stderr, "SECRET raw stderr")
		data, err := json.Marshal(signalReady{Agent: os.Getpid(), Child: leaf.Process.Pid, Summary: summary})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(control, "ready.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	default:
		t.Fatal("unknown helper")
	}
}
