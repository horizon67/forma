package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/agentrunner"
)

func TestGenerateCommandBuildsAFullRequestAndStopsForHumanReview(t *testing.T) {
	previous := invokeGeneration
	t.Cleanup(func() { invokeGeneration = previous })

	repository := newNoOpRepository(t)
	var observed generateInvocation
	var bounded bool
	invokeGeneration = func(ctx context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
		observed = invocation
		deadline, ok := ctx.Deadline()
		bounded = ok && !deadline.IsZero()
		generated := beginTestGeneration(t, invocation)
		generated.FinalStatus = "?? internal/app.go\n M README.md\n"
		generated.Summary = []byte("Implemented the application.\n")
		return generated, nil
	}

	source := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"generate", "--repository", repository, source}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if !bounded || observed.Repository != repository || observed.AllowDirty {
		t.Fatalf("invocation = %#v, bounded = %v", observed, bounded)
	}
	request, err := agentrequest.UnmarshalRequest(observed.Request)
	if err != nil {
		t.Fatalf("generation request: %v", err)
	}
	if request.RequestedChange.Kind != "full" || request.ResolvedIntent == nil || len(request.AcceptanceFacts.Facts) == 0 {
		t.Fatalf("request = %#v", request)
	}
	// Keep the dogfood-derived expected.enforcement instruction tied to the
	// actual current Generation Request wire path instead of merely asserting
	// that the prompt repeats its own spelling.
	var wire struct {
		AcceptanceFacts struct {
			Facts []struct {
				Expected map[string]json.RawMessage `json:"expected"`
			} `json:"facts"`
		} `json:"acceptanceFacts"`
	}
	if err := json.Unmarshal(observed.Request, &wire); err != nil {
		t.Fatalf("decode generation request wire shape: %v", err)
	}
	foundAuthoritative := false
	for _, fact := range wire.AcceptanceFacts.Facts {
		if string(fact.Expected["enforcement"]) == `"authoritative"` {
			foundAuthoritative = true
			break
		}
	}
	if !foundAuthoritative {
		t.Fatal(`Generation Request has no expected.enforcement="authoritative" path named by the implementation prompt`)
	}
	output := stdout.String()
	for _, want := range []string{
		"preparing agent generation",
		"Press Ctrl+C to cancel safely; cancellation is not rollback",
		"AI summary (unverified text; may contain sensitive information):\nImplemented the application.",
		"implementation prompt SHA-256: 0123456789abcdef",
		"current Git status:\n   M README.md\n  ?? internal/app.go",
		"Forma did not run generated application code or repository tests.",
		"review the Git diff",
		"Confirm that boundary tests actually ran and did not skip assertions because the sandbox lacked a runtime capability.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout does not contain %q:\n%s", want, output)
		}
	}
	if !strings.Contains(stderr.String(), "completed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestGenerateCommandSupportsExplicitDirtyAndManifestOptions(t *testing.T) {
	previous := invokeGeneration
	t.Cleanup(func() { invokeGeneration = previous })
	manifest := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target", "forma.implementation.yaml")
	source := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "app.forma")
	repository := newNoOpRepository(t)
	writeTestFile(t, filepath.Join(repository, "README.md"), "manual edit\n")
	var observed generateInvocation
	invokeGeneration = func(_ context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
		observed = invocation
		return beginTestGeneration(t, invocation), nil
	}

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"generate", "--repository", repository, "--manifest", manifest, "--allow-dirty", source}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if !observed.AllowDirty {
		t.Fatal("--allow-dirty did not reach generation")
	}
	request, err := agentrequest.UnmarshalRequest(observed.Request)
	if err != nil {
		t.Fatal(err)
	}
	if request.RequestedChange.Kind != "full" || request.ImplementationPolicy == nil {
		t.Fatalf("request = %#v", request)
	}
	if !strings.Contains(stdout.String(), "current status includes pre-existing changes") {
		t.Fatalf("stdout = %q", stdout.String())
	}

}

func TestGenerateCommandRejectsInvalidOptionsAndCompilerDiagnosticsBeforeCodex(t *testing.T) {
	previous := invokeGeneration
	t.Cleanup(func() { invokeGeneration = previous })
	calls := 0
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		calls++
		return agentrunner.GenerateResult{}, nil
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing repository", args: []string{"generate", "app.forma"}, want: "generate requires --repository"},
		{name: "missing repository value", args: []string{"generate", "--repository"}, want: "--repository requires a directory"},
		{name: "repeated dirty", args: []string{"generate", "--repository", ".", "--allow-dirty", "--allow-dirty", "app.forma"}, want: "--allow-dirty was repeated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("exit code = %d", exitCode)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
		})
	}

	invalid := filepath.Join(t.TempDir(), "invalid.forma")
	writeTestFile(t, invalid, "entity Broken { field String }\n")
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"generate", "--repository", t.TempDir(), invalid}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if calls != 0 || !strings.Contains(stderr.String(), "forma generate failed") {
		t.Fatalf("Codex calls / stderr = %d / %q", calls, stderr.String())
	}
}

func TestGenerateCommandDistinguishesSetupFromAgentFailure(t *testing.T) {
	previous := invokeGeneration
	t.Cleanup(func() { invokeGeneration = previous })
	source := filepath.Join("..", "..", "examples", "users.forma")

	for _, test := range []struct {
		name     string
		err      error
		wantExit int
	}{
		{name: "authentication", err: agentrunner.ErrAuthentication, wantExit: 2},
		{name: "missing CLI", err: errCodexUnavailable, wantExit: 2},
		{name: "agent failure", err: agentrunner.ErrAgentFailed, wantExit: 1},
		{name: "timeout", err: context.DeadlineExceeded, wantExit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
				return agentrunner.GenerateResult{}, test.err
			}
			var stdout, stderr bytes.Buffer
			if exitCode := run([]string{"generate", "--repository", newNoOpRepository(t), source}, &stdout, &stderr); exitCode != test.wantExit {
				t.Fatalf("exit code %d, want %d\nstderr:\n%s", exitCode, test.wantExit, stderr.String())
			}
		})
	}
}

func TestGenerateCommandDisplaysHumanReviewRequirementsBeforeCodexRuns(t *testing.T) {
	previous := invokeGeneration
	t.Cleanup(func() { invokeGeneration = previous })
	var stdout, stderr bytes.Buffer
	calls := 0
	invokeGeneration = func(_ context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
		calls++
		request, err := agentrequest.UnmarshalRequest(invocation.Request)
		if err != nil {
			t.Fatal(err)
		}
		if request.ReviewRequirements == nil || len(request.ReviewRequirements.Requirements) == 0 ||
			!reflect.DeepEqual(request.ReviewRequirements, invocation.ReviewRequirements) {
			t.Fatal("actual Generation Request lacks the displayed Review Requirements")
		}
		if !strings.Contains(stdout.String(), "human review required:") {
			t.Fatal("review warning was not displayed before dispatch")
		}
		for _, requirement := range request.ReviewRequirements.Requirements {
			line := fmt.Sprintf("%s [%s]: %s", requirement.ID, requirement.Kind, requirement.Instruction)
			if !strings.Contains(stdout.String(), line) {
				t.Fatalf("requirement was not displayed before dispatch: %s", line)
			}
		}
		return agentrunner.GenerateResult{}, agentrunner.ErrAgentFailed
	}
	source := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	if code := run([]string{"generate", "--repository", newNoOpRepository(t), source}, &stdout, &stderr); code != 1 || calls != 1 {
		t.Fatalf("exit code = %d; dispatches = %d; stderr: %s", code, calls, &stderr)
	}
}

func TestCompilerOnlyCommandsNeverInvokeCodexGeneration(t *testing.T) {
	previous := invokeGeneration
	t.Cleanup(func() { invokeGeneration = previous })
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		t.Fatal("compiler-only command invoked Codex generation")
		return agentrunner.GenerateResult{}, errors.New("unreachable")
	}
	source := filepath.Join("..", "..", "examples", "users.forma")
	for _, arguments := range [][]string{
		{"authoring-context"},
		{"check", source},
		{"resolve", source},
		{"project", "flow", source},
		{"request", source},
	} {
		var stdout, stderr bytes.Buffer
		if exitCode := run(arguments, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("%v exit code %d\nstderr:\n%s", arguments, exitCode, stderr.String())
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
