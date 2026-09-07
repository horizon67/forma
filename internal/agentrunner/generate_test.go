package agentrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/generationprogress"
)

// This fake imports no provider and uses no process. It models an API-backed
// implementation with immutable request bytes and optional notifications.
type fakeBackend struct {
	prepare               func(context.Context, Input) error
	run                   func(context.Context, Notify) (ExecutionResult, error)
	input                 []byte
	prepared, ran, closed int
	closeErr              error
	digest                string
}

func (b *fakeBackend) Prepare(ctx context.Context, in Input) (Prepared, error) {
	b.prepared++
	if b.prepare != nil {
		if err := b.prepare(ctx, in); err != nil {
			return nil, err
		}
	}
	b.input = append([]byte(nil), in.Request...)
	return b, nil
}
func (b *fakeBackend) InputSHA256() string {
	if b.digest != "" {
		return b.digest
	}
	return fmt.Sprintf("%x", sha256.Sum256(b.input))
}
func (b *fakeBackend) Run(ctx context.Context, n Notify) (ExecutionResult, error) {
	b.ran++
	if b.run != nil {
		return b.run(ctx, n)
	}
	return ExecutionResult{Summary: []byte("API result"), Status: generationprogress.Completed, CleanupComplete: true}, nil
}
func (b *fakeBackend) Close() error { b.closed++; return b.closeErr }

func cleanRepositorySteps(target string, locker *recordingLocker, includeFinal bool) []commandStep {
	canonicalTarget, err := canonicalDirectory(target)
	if err != nil {
		panic(err)
	}
	steps := []commandStep{{stdout: canonicalTarget + "\n"}, {requireLocked: locker, stdout: "head\n"}, {requireLocked: locker, stdout: "H README.md\x00"}, {requireLocked: locker}}
	if includeFinal {
		steps = append(steps, commandStep{requireLocked: locker})
	}
	return steps
}

func TestProviderIndependentPreparationCandidateExecutionAndCleanup(t *testing.T) {
	for _, failure := range []string{"none", "preparation", "candidate", "execution", "cancel", "timeout", "cleanup", "unknown cleanup"} {
		t.Run(failure, func(t *testing.T) {
			target := t.TempDir()
			locker := &recordingLocker{}
			git := &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, true)}
			backend := &fakeBackend{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backend.prepare = func(context.Context, Input) error {
				if !locker.locked {
					t.Fatal("prepare without lock")
				}
				if failure == "preparation" {
					return ErrAuthentication
				}
				return nil
			}
			candidate := false
			backend.run = func(_ context.Context, n Notify) (ExecutionResult, error) {
				if !candidate || !locker.locked {
					t.Fatal("run before candidate/without lock")
				}
				result := ExecutionResult{Status: generationprogress.Completed, CleanupComplete: true, Summary: []byte("API result")}
				switch failure {
				case "execution":
					return result, ErrAgentFailed
				case "cancel":
					cancel()
					return result, ctx.Err()
				case "timeout":
					return result, context.DeadlineExceeded
				case "unknown cleanup":
					result.CleanupComplete = false
				}
				// No notifications is a supported backend, not a hung execution.
				return result, nil
			}
			if failure == "cleanup" {
				backend.closeErr = errors.New("private cleanup")
			}
			request := []byte(`{"requestedChange":{"kind":"full"}}`)
			result, err := (Generator{Repository: RepositoryPreflight{Commands: git, Locks: locker}, Backend: backend}).Run(ctx, GenerateOptions{Repository: target, GitExecutable: "/absolute/git", Request: request, BeforeExecute: func() error {
				if backend.prepared != 1 || backend.ran != 0 {
					t.Fatal("candidate order")
				}
				if failure == "candidate" {
					return errors.New("disk full")
				}
				candidate = true
				return nil
			}})
			if (err != nil) != (failure != "none") {
				t.Fatalf("error: %v", err)
			}
			if locker.locked {
				t.Fatal("lock retained")
			}
			if failure == "preparation" {
				if candidate || backend.ran != 0 || backend.closed != 0 {
					t.Fatal("preparation failure dispatched")
				}
				return
			}
			if backend.closed != 1 {
				t.Fatal("prepared input not closed exactly once")
			}
			if failure == "candidate" {
				if backend.ran != 0 {
					t.Fatal("candidate failure dispatched")
				}
				return
			}
			if !result.FinalStatusKnown || len(git.calls) != 5 || result.ImplementationPromptSHA256 != backend.InputSHA256() {
				t.Fatalf("missing final evidence: %#v", result)
			}
			if result.Status != generationprogress.Outcome(err) {
				t.Fatalf("outcome %s for %v", result.Status, err)
			}
		})
	}
}

func TestPreparedRunRetainsCallerLockAndRejectsClosedState(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	preflight := RepositoryPreflight{Commands: &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, true)}, Locks: locker}
	state, err := preflight.Prepare(context.Background(), RepositoryPreflightOptions{Repository: target, GitExecutable: "/absolute/git"})
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	backend := &fakeBackend{}
	g := Generator{Repository: preflight, Backend: backend}
	options := GenerateOptions{Repository: target, GitExecutable: "/absolute/git", Request: []byte(`{"requestedChange":{"kind":"incremental"}}`)}
	if _, err := g.RunPrepared(context.Background(), options, state); err != nil {
		t.Fatal(err)
	}
	if !locker.locked || locker.calls != 1 {
		t.Fatal("caller lock lost or reacquired")
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.RunPrepared(context.Background(), options, state); err == nil {
		t.Fatal("closed state accepted")
	}
}

func TestInvalidGenerationInputDoesNotReachRepositoryOrBackend(t *testing.T) {
	for _, input := range []string{"", "{", "{}", `{"requestedChange":{"kind":"repair"}}`, `{"requestedChange":{"kind":"no-op"}}`} {
		git := &scriptedCommandRunner{t: t}
		backend := &fakeBackend{}
		_, err := (Generator{Repository: RepositoryPreflight{Commands: git, Locks: &recordingLocker{}}, Backend: backend}).Run(context.Background(), GenerateOptions{Request: []byte(input)})
		if err == nil || len(git.calls) != 0 || backend.prepared != 0 {
			t.Fatalf("invalid input reached execution: %q", input)
		}
		if input == "" && !strings.Contains(err.Error(), "requires canonical Generation Request") {
			t.Fatal(err)
		}
	}
}

func TestNotifyingAPIBackendAndTerminalResultAfterLockRelease(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	var progress bytes.Buffer
	p := generationprogress.New(&progress, generationprogress.Options{JSON: true, Verbose: true})
	backend := &fakeBackend{run: func(_ context.Context, n Notify) (ExecutionResult, error) {
		n(Notification{Started: true})
		n(Notification{Activity: generationprogress.Tool})
		return ExecutionResult{Status: generationprogress.Completed, CleanupComplete: true}, nil
	}}
	result, err := (Generator{Repository: RepositoryPreflight{Commands: &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, true)}, Locks: locker}, Backend: backend}).Run(context.Background(), GenerateOptions{Repository: target, GitExecutable: "/absolute/git", Request: []byte(`{"requestedChange":{"kind":"full"}}`), Progress: p})
	if err != nil || locker.locked || backend.closed != 1 {
		t.Fatalf("execution not finalized: %#v %v", result, err)
	}
	if !p.Finish(result.Status) {
		t.Fatal("in-memory progress writer did not drain")
	}
	var phases []generationprogress.Phase
	for _, line := range strings.Split(strings.TrimSpace(progress.String()), "\n") {
		var e generationprogress.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		if e.Type == "phase" && e.DurationMS == nil {
			phases = append(phases, e.Phase)
		}
	}
	want := []generationprogress.Phase{generationprogress.AgentPreparation, generationprogress.AgentStarting, generationprogress.AgentRunning, generationprogress.Cleanup, generationprogress.FinalStatus}
	if fmt.Sprint(phases) != fmt.Sprint(want) {
		t.Fatalf("API lifecycle %v", phases)
	}
}

func TestInvalidPreparedDigestNeverSavesCandidate(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	backend := &fakeBackend{digest: "invalid"}
	_, err := (Generator{Repository: RepositoryPreflight{Commands: &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, false)}, Locks: locker}, Backend: backend}).Run(context.Background(), GenerateOptions{Repository: target, GitExecutable: "/absolute/git", Request: []byte(`{"requestedChange":{"kind":"full"}}`), BeforeExecute: func() error { t.Fatal("invalid digest reached candidate"); return nil }})
	if err == nil || backend.ran != 0 || backend.closed != 1 || locker.locked {
		t.Fatalf("invalid digest cleanup %v %#v", err, backend)
	}
}

func TestCancellationBeforeCandidateDoesNotDispatch(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &fakeBackend{prepare: func(context.Context, Input) error { cancel(); return nil }}
	_, err := (Generator{Repository: RepositoryPreflight{Commands: &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, false)}, Locks: locker}, Backend: backend}).Run(ctx, GenerateOptions{Repository: target, GitExecutable: "/absolute/git", Request: []byte(`{"requestedChange":{"kind":"full"}}`), BeforeExecute: func() error { t.Fatal("candidate saved after cancellation"); return nil }})
	if !errors.Is(err, context.Canceled) || backend.ran != 0 || backend.closed != 1 || locker.locked {
		t.Fatalf("cancel cleanup: %v %#v", err, backend)
	}
}
