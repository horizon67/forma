package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationprogress"
)

func decodeProgress(t *testing.T, content string) []generationprogress.Event {
	t.Helper()
	var events []generationprogress.Event
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		var e generationprogress.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil || e.Schema != generationprogress.Schema {
			t.Fatalf("non-progress stderr line: %q (%v)", line, err)
		}
		switch e.Type {
		case "phase", "heartbeat", "activity", "diagnostic", "result":
		default:
			t.Fatalf("unknown event: %#v", e)
		}
		events = append(events, e)
	}
	if len(events) == 0 || events[len(events)-1].Type != "result" {
		t.Fatal("no terminal result")
	}
	return events
}

func TestGenerateProgressJSONEveryErrorPath(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "invalid.forma")
	writeTestFile(t, invalid, "entity Broken { field String }\n")
	valid := filepath.Join("..", "..", "examples", "users.forma")
	for _, tc := range []struct {
		name string
		args []string
		exit int
	}{
		{"options", []string{"--progress=json"}, 2},
		{"bad interval", []string{"--progress=json", "--heartbeat-interval=0s"}, 2},
		{"compiler", []string{"--progress=json", "--repository", t.TempDir(), invalid}, 1},
		{"source", []string{"--progress=json", "--repository", t.TempDir(), "missing.forma"}, 2},
		{"manifest", []string{"--progress=json", "--repository", t.TempDir(), "--manifest", "missing.yaml", valid}, 2},
		{"repository", []string{"--progress=json", "--repository", t.TempDir(), valid}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, progress bytes.Buffer
			code := runGenerateCommand(context.Background(), tc.args, &out, &progress)
			if code != tc.exit {
				t.Fatalf("exit %d: %s %s", code, &out, &progress)
			}
			events := decodeProgress(t, progress.String())
			for _, event := range events {
				if event.Code == generationprogress.GenerationError {
					t.Fatal("pre-agent error reported generation failure", events)
				}
			}
			if events[len(events)-1].Status != generationprogress.Failed {
				t.Fatal(events)
			}
			if tc.name == "compiler" && !strings.Contains(out.String(), "forma generate failed") {
				t.Fatal("compiler detail missing from stdout")
			}
		})
	}
}

func TestSlowOrBrokenProgressPreservesHumanDiagnostics(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "invalid.forma")
	writeTestFile(t, invalid, "entity Broken { field String }\n")
	for _, tc := range []struct {
		name string
		args []string
		want string
		exit int
	}{
		{"usage", []string{"--progress=yaml"}, "--progress requires text or json", 2},
		{"compiler", []string{"--repository", t.TempDir(), invalid}, "forma generate failed", 1},
	} {
		for _, broken := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/broken=%v", tc.name, broken), func(t *testing.T) {
				release := make(chan struct{})
				defer close(release)
				var sink io.Writer = blockedProgressWriter{release}
				if broken {
					sink = errorWriter{}
				}
				var out bytes.Buffer
				finished := make(chan int, 1)
				go func() { finished <- runGenerateCommand(context.Background(), tc.args, &out, sink) }()
				select {
				case code := <-finished:
					if code != tc.exit {
						t.Fatal(code)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("diagnostic fallback blocked on progress output")
				}
				if !strings.Contains(out.String(), tc.want) {
					t.Fatalf("diagnostic lost: %q", out.String())
				}
			})
		}
	}
}

type cancellingResultWriter struct {
	bytes.Buffer
	cancel  context.CancelFunc
	trigger string
}

func (w *cancellingResultWriter) Write(b []byte) (int, error) {
	if bytes.Contains(b, []byte(w.trigger)) {
		w.cancel()
	}
	return w.Buffer.Write(b)
}
func TestLateCancellationDoesNotDowngradeDurableCompletionOrNoOp(t *testing.T) {
	for _, noop := range []bool{false, true} {
		t.Run(fmt.Sprint(noop), func(t *testing.T) {
			repo, source, _, args := historyFixture(t)
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				return beginTestGeneration(t, in), nil
			}
			trigger := "repository: "
			want := generationprogress.Completed
			if noop {
				runHistoryCommand(t, args, 0)
				trigger = "No update was applied."
				want = generationprogress.NoOp
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := &cancellingResultWriter{cancel: cancel, trigger: trigger}
			var progress bytes.Buffer
			if code := runGenerateCommand(ctx, append(args[1:], "--progress=json"), out, &progress); code != 0 {
				t.Fatalf("durable completion downgraded: %d\n%s\n%s", code, out.String(), progress.String())
			}
			if ctx.Err() == nil {
				t.Fatal("test did not cancel at the output boundary")
			}
			events := decodeProgress(t, progress.String())
			if events[len(events)-1].Status != want {
				t.Fatal(events)
			}
			store, _, record := readHistoryRecord(t, repo, repo, source)
			if store.PendingKey != "" || record.Pending || record.LastAttempt.Status != "completed" {
				t.Fatal("late cancellation changed history")
			}
		})
	}
}

func TestSafeSummaryRemovesDisplayControlsButPreservesText(t *testing.T) {
	input := "\x1b\x00\x7f\u0085\u202e日本語\u202c\u2066 العربية\u2069\u200e\u200f\u061c\u2028\u2029\n\ttext"
	if got := safeSummary(input); got != "日本語 العربية\n\ttext" {
		t.Fatalf("unsafe summary: %q", got)
	}
}

func TestGenerateProgressOptionsDoNotChangeInputOrHistoryAndNoOpNeverCallsBackend(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	old := invokeGeneration
	oldFind := findExecutable
	t.Cleanup(func() { invokeGeneration = old; findExecutable = oldFind })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), nil
	}
	runHistoryCommand(t, args, 0)
	store, _, record := readHistoryRecord(t, repo, repo, source)
	before, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		t.Fatal("no-op called backend preparation/execution")
		return agentrunner.GenerateResult{}, nil
	}
	findExecutable = func(name string) (string, error) {
		if name != "git" {
			t.Fatal("no-op looked up backend", name)
		}
		return oldFind(name)
	}
	for _, flags := range [][]string{{"--progress=json"}, {"--progress", "text", "--verbose", "--heartbeat-interval", "1s"}, {"--progress=json", "--verbose", "--heartbeat-interval=5m"}} {
		var out, progress bytes.Buffer
		current := append(append([]string(nil), args...), flags...)
		if code := run(current, &out, &progress); code != 0 {
			t.Fatalf("no-op: %s %s", &out, &progress)
		}
		if strings.Contains(strings.Join(flags, " "), "json") {
			events := decodeProgress(t, progress.String())
			if events[len(events)-1].Status != generationprogress.NoOp {
				t.Fatal(events)
			}
			for _, e := range events {
				if e.Phase == generationprogress.AgentPreparation || e.Phase == generationprogress.AgentRunning {
					t.Fatal("no-op reported agent work")
				}
			}
		}
		after, err := os.ReadFile(store.Path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("display options changed history")
		}
		_, _, afterRecord := readHistoryRecord(t, repo, repo, source)
		if afterRecord.Baseline.Request != record.Baseline.Request {
			t.Fatal("request changed")
		}
	}
}

func TestGenerateCancelledAndTimedOutKeepBaselineAndPending(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			repo, source, manifest, args := historyFixture(t)
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				return beginTestGeneration(t, in), nil
			}
			runHistoryCommand(t, args, 0)
			_, _, before := readHistoryRecord(t, repo, repo, source)
			writeTestFile(t, manifest, strings.Replace(updateManifest, "value: bun", "value: pnpm", 1))
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				r := beginTestGeneration(t, in)
				writeTestFile(t, filepath.Join(repo, "partial.txt"), "partial")
				return r, failure
			}
			var out, progress bytes.Buffer
			args = append(args, "--progress=json")
			if code := run(args, &out, &progress); code != 1 {
				t.Fatalf("exit %d", code)
			}
			events := decodeProgress(t, progress.String())
			if events[len(events)-1].Status != generationprogress.Outcome(failure) {
				t.Fatal(events)
			}
			warning := false
			for _, e := range events {
				warning = warning || e.Code == generationprogress.PartialMutation
			}
			if !warning {
				t.Fatal("partial mutation warning missing")
			}
			store, _, after := readHistoryRecord(t, repo, repo, source)
			if !after.Pending || store.PendingKey == "" || after.Baseline.Request != before.Baseline.Request || after.LastAttempt.Status != "failed" {
				t.Fatal("interruption promoted baseline")
			}
		})
	}
}

func TestGeneratePreCandidateCancellationCreatesNoHistory(t *testing.T) {
	repo, _, _, args := historyFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, progress bytes.Buffer
	if code := runGenerateCommand(ctx, append(args[1:], "--progress=json"), &out, &progress); code != 1 {
		t.Fatal(code)
	}
	events := decodeProgress(t, progress.String())
	if events[len(events)-1].Status != generationprogress.Cancelled {
		t.Fatal(events)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "forma", "generation-history.json")); !os.IsNotExist(err) {
		t.Fatal("pre-candidate interruption created history")
	}
}

func TestProgressOptionValidation(t *testing.T) {
	for _, flags := range [][]string{{"--progress=yaml"}, {"--progress=json", "--progress=text"}, {"--heartbeat-interval"}, {"--heartbeat-interval=-1s"}, {"--heartbeat-interval=999ms"}, {"--heartbeat-interval=301s"}, {"--heartbeat-interval=1s", "--heartbeat-interval=2s"}, {"--verbose", "--verbose"}} {
		if _, _, err := parseGenerateOptions(append([]string{"--repository", "."}, flags...)); err == nil {
			t.Fatal("invalid options accepted", flags)
		}
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("broken display") }
func TestBrokenProgressDisplayDoesNotRepeatOrFailExecution(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	calls := 0
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		calls++
		return beginTestGeneration(t, in), nil
	}
	if code := run(args, io.Discard, errorWriter{}); code != 0 || calls != 1 {
		t.Fatalf("display failure changed execution: %d %d", code, calls)
	}
	_, _, record := readHistoryRecord(t, repo, repo, source)
	if record.Pending || record.LastAttempt.Status != "completed" {
		t.Fatal("display blocked durable completion")
	}
}

type blockedProgressWriter struct{ release chan struct{} }

func (w blockedProgressWriter) Write(b []byte) (int, error) { <-w.release; return len(b), nil }
func TestBlockedProgressDisplayDoesNotHoldWorktreeLock(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), nil
	}
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	finished := make(chan int, 1)
	go func() { finished <- run(args, io.Discard, blockedProgressWriter{release}) }()
	select {
	case code := <-finished:
		if code != 0 {
			t.Fatal(code)
		}
	case <-time.After(5 * time.Second):
		once.Do(func() { close(release) })
		<-finished
		t.Fatal("blocked display held generation")
	}
	lock, err := (agentrunner.WorktreeLocker{}).Lock(repo)
	if err != nil {
		t.Fatal("display retained lock", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, record := readHistoryRecord(t, repo, repo, source)
	if record.Pending {
		t.Fatal("display prevented history completion")
	}
}

type resultLockWriter struct {
	repository string
	checked    bool
	err        error
	done       chan struct{}
}

func (w *resultLockWriter) Write(b []byte) (int, error) {
	var event generationprogress.Event
	if err := json.Unmarshal(b, &event); err != nil {
		return 0, err
	}
	if event.Type == "result" {
		w.checked = true
		lock, err := (agentrunner.WorktreeLocker{}).Lock(w.repository)
		w.err = err
		if lock != nil {
			w.err = lock.Close()
		}
		close(w.done)
	}
	return len(b), nil
}
func TestTerminalResultIsRenderedAfterWorktreeLockRelease(t *testing.T) {
	repo, _, _, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), nil
	}
	w := &resultLockWriter{repository: repo, done: make(chan struct{})}
	if code := run(append(args, "--progress=json"), io.Discard, w); code != 0 {
		t.Fatal(code)
	}
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
		t.Fatal("terminal result was not observed")
	}
	if !w.checked || w.err != nil {
		t.Fatal("result preceded lock release", w.err)
	}
}

func TestAgentFailureDetailSurvivesStalledTextOutputAndJSON(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(fmt.Sprint(jsonMode), func(t *testing.T) {
			repo, source, _, args := historyFixture(t)
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				return beginTestGeneration(t, in), fmt.Errorf("%w: agent CLI exited with 7", agentrunner.ErrAgentFailed)
			}
			var out, progress bytes.Buffer
			release := make(chan struct{})
			defer close(release)
			var sink io.Writer = blockedProgressWriter{release}
			if jsonMode {
				args = append(args, "--progress=json")
				sink = &progress
			}
			finished := make(chan int, 1)
			go func() { finished <- run(args, &out, sink) }()
			select {
			case code := <-finished:
				if code != 1 {
					t.Fatal(code)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stalled display blocked failure finalization")
			}
			if !strings.Contains(out.String(), "agent CLI exited with 7") {
				t.Fatal("failure detail discarded", out.String())
			}
			if jsonMode {
				events := decodeProgress(t, progress.String())
				found := false
				for _, event := range events {
					found = found || event.Code == generationprogress.GenerationError
				}
				if !found || events[len(events)-1].Status != generationprogress.Failed {
					t.Fatal("editing failure not classified", events)
				}
			}
			store, _, record := readHistoryRecord(t, repo, repo, source)
			if store.PendingKey == "" || !record.Pending {
				t.Fatal("failed attempt lost pending recovery")
			}
			lock, err := (agentrunner.WorktreeLocker{}).Lock(repo)
			if err != nil {
				t.Fatal("failed attempt retained lock", err)
			}
			if err := lock.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
