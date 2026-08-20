package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

func TestResolveCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"resolve", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var intent struct {
		Version string `json:"version"`
		Pages   []any  `json:"pages"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &intent); err != nil {
		t.Fatalf("resolve output is not JSON: %v\n%s", err, stdout.String())
	}
	if intent.Version != "forma/resolved-intent/v0.8" || len(intent.Pages) == 0 {
		t.Fatalf("resolved intent = %#v", intent)
	}
}

func TestProjectNavigationCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.navigation.txt")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "navigation", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected navigation projection:\n%s", stdout.String())
	}
}

func TestProjectOutcomesCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.outcomes.txt")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "outcomes", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected outcome projection:\n%s", stdout.String())
	}
}

func TestProjectStatesCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.states.txt")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "states", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected domain state projection:\n%s", stdout.String())
	}
}

func TestProjectFlowCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.flow.md")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "flow", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected flow projection:\n%s", stdout.String())
	}
}

func TestProjectCommandRequiresAKnownProjection(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", args: []string{"project"}, want: "project requires a projection name"},
		{name: "unknown", args: []string{"project", "sequence"}, want: "unknown projection \"sequence\""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q", stdout.String())
			}
		})
	}
}

func TestRequestCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"request", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var request struct {
		Schema          string `json:"schema"`
		ResolvedIntent  any    `json:"resolvedIntent"`
		AcceptanceFacts struct {
			Facts []any `json:"facts"`
		} `json:"acceptanceFacts"`
		Verification struct {
			RequiredFactIDs []string `json:"requiredFactIds"`
		} `json:"verification"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &request); err != nil {
		t.Fatalf("request output is not JSON: %v\n%s", err, stdout.String())
	}
	if request.Schema != "forma/generation-request/v0alpha4" || request.ResolvedIntent == nil {
		t.Fatalf("generation request = %#v", request)
	}
	if len(request.AcceptanceFacts.Facts) == 0 || len(request.AcceptanceFacts.Facts) != len(request.Verification.RequiredFactIDs) {
		t.Fatalf("fact coverage policy does not cover request: %#v", request)
	}
}

func TestIncrementalRequestCommand(t *testing.T) {
	previousPath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.request.json")
	manifestPath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target", "forma.implementation.yaml")
	sourcePath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "app.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"request", "--previous", previousPath, "--manifest", manifestPath, sourcePath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var request struct {
		ImplementationPolicy struct {
			Policies []any `json:"policies"`
		} `json:"implementationPolicy"`
		RequestedChange struct {
			Kind          string `json:"kind"`
			IntentChanges []any  `json:"intentChanges"`
			FactChanges   []any  `json:"factChanges"`
		} `json:"requestedChange"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &request); err != nil {
		t.Fatalf("request output is not JSON: %v\n%s", err, stdout.String())
	}
	if request.RequestedChange.Kind != "incremental" || len(request.RequestedChange.IntentChanges) != 8 || len(request.RequestedChange.FactChanges) != 13 {
		t.Fatalf("requested change = %#v", request.RequestedChange)
	}
	if len(request.ImplementationPolicy.Policies) != 3 {
		t.Fatalf("implementation policy = %#v", request.ImplementationPolicy)
	}
}

func TestVerifyCommand(t *testing.T) {
	requestPath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.request.json")
	feedbackPath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "baseline", "generation-feedback.json")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "verified 43 acceptance facts: all passed\n  12 distinct tests, max 8 facts per test\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestVerifyIncrementalCommandChecksRepositoryPolicies(t *testing.T) {
	requestPath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.incremental.request.json")
	baselinePath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.request.json")
	targetRoot := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target")
	feedbackPath := filepath.Join(targetRoot, "generation-feedback.json")
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"verify", "--repository", targetRoot, requestPath, feedbackPath}, &stdout, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "requires its baseline request") {
		t.Fatalf("missing baseline exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exitCode := run([]string{"verify", "--repository", targetRoot, "--baseline", baselinePath, requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	want := "verified 43 acceptance facts: all passed\n" +
		"  12 distinct tests, max 8 facts per test\n" +
		"verified 3 implementation policies\n" +
		"  2 satisfied, 1 deviated, 0 flagged\n" +
		"  deviated implementation/persistence: This controlled experiment retains the existing in-memory store.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestVerifyIdentityRequestAlwaysDisplaysHumanReview(t *testing.T) {
	read := func(name string, target any) {
		t.Helper()
		content, err := os.ReadFile(filepath.Join("..", "..", "internal", "compiler", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(content, target); err != nil {
			t.Fatal(err)
		}
	}
	var intent compiler.ResolvedIntent
	var sourceMap compiler.SourceMap
	read("membership.intent.json", &intent)
	read("membership.sourcemap.json", &sourceMap)
	request, err := agentrequest.BuildFull(compiler.Result{Intent: &intent, SourceMap: &sourceMap})
	if err != nil {
		t.Fatal(err)
	}
	feedback := agentrequest.Feedback{Schema: agentrequest.FeedbackSchema, Stage: "test", Status: "succeeded"}
	for _, factID := range request.Verification.RequiredFactIDs {
		feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
			FactID: factID, TestReferences: []string{"tests/membership_test.go#" + strings.ReplaceAll(string(factID), "/", "_")}, Result: "passed",
		})
	}
	directory := t.TempDir()
	requestPath := filepath.Join(directory, "request.json")
	requestContent, err := agentrequest.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requestPath, requestContent, 0o644); err != nil {
		t.Fatal(err)
	}
	feedbackPath := filepath.Join(directory, "feedback.json")
	feedbackContent, err := json.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(feedbackPath, feedbackContent, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "verified 38 acceptance facts: all passed") ||
		!strings.Contains(output, "human review required: 3 requirements are not machine-verified") {
		t.Fatalf("review output = %q", output)
	}
	for _, requirement := range request.ReviewRequirements.Requirements {
		if !strings.Contains(output, string(requirement.ID)) || !strings.Contains(output, requirement.Instruction) {
			t.Fatalf("review output omits %s: %q", requirement.ID, output)
		}
	}
}

func TestVerifyCommandRejectsFailedFeedback(t *testing.T) {
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("request.forma", `role admin
entity User {
    name String required label
}
page Users {
    allow admin
    list User {
        columns name
    }
}
`)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	request, err := agentrequest.BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	feedback := agentrequest.Feedback{
		Schema: agentrequest.FeedbackSchema, Stage: "test", Status: "failed",
		RelatedIntentNodes: []compiler.SemanticID{request.AcceptanceFacts.Facts[0].SourceNodes[0]},
		Command:            "go test ./...",
		Diagnostics: []string{
			"--- FAIL: TestAdminFlow",
			"tests/admin_test.go:10: the duplicate attempt's secret signed in: 303",
		},
		Summary: "Target tests failed; 1 mapped Acceptance Fact(s) did not pass.",
	}
	feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
		FactID:         request.Verification.RequiredFactIDs[0],
		TestReferences: []string{"tests/admin_test.go#TestAdminFlow"},
		Result:         "failed",
	})
	requestContent, err := agentrequest.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	requestPath := filepath.Join(directory, "request.json")
	if err := os.WriteFile(requestPath, requestContent, 0o644); err != nil {
		t.Fatal(err)
	}
	feedbackPath := filepath.Join(directory, "feedback.json")
	feedbackContent, err := json.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(feedbackPath, feedbackContent, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code %d\nstdout:\n%s\nstderr:\n%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Generation Feedback status is failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed verify must not report a passing coverage summary: %q", stdout.String())
	}
}

func TestVerifyCommandRejectsMeasuredMembershipRepairFailure(t *testing.T) {
	requestPath := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "generation-request.json")
	baselinePath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.incremental.request.json")
	targetRoot := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "target")
	feedbackPath := filepath.Join("..", "..", "experiments", "membership-repair-loop", "generation-feedback.failed.json")
	feedbackContent, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := agentrequest.UnmarshalFeedback(feedbackContent)
	if err != nil {
		t.Fatal(err)
	}
	if feedback.Status != "failed" || feedback.Stage != "test" {
		t.Fatalf("measured failed feedback = %#v", feedback)
	}
	if !strings.Contains(feedback.Command, "go test -count=1 -json ./...") {
		t.Fatalf("measured command = %q", feedback.Command)
	}
	failed := 0
	for _, coverage := range feedback.FactCoverage {
		if coverage.Result == "failed" {
			failed++
		}
		if coverage.Result != "passed" && coverage.Result != "failed" && coverage.Result != "not-run" {
			t.Fatalf("fact %s has result %q", coverage.FactID, coverage.Result)
		}
	}
	if failed != 1 {
		t.Fatalf("measured failed facts = %d, want 1", failed)
	}
	for _, line := range feedback.Diagnostics {
		if strings.Contains(line, "(") && strings.Contains(line, "s)") {
			t.Fatalf("measured diagnostics contain a duration: %q", line)
		}
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", "--repository", targetRoot, "--baseline", baselinePath, requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 1 || !strings.Contains(stderr.String(), "Generation Feedback status is failed") {
		t.Fatalf("measured failed feedback exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed verify must not report a passing coverage summary: %q", stdout.String())
	}
}

func TestCheckCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "checked 1 file: no errors\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestCheckCommandRendersDiagnostic(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "invalid.forma")
	source := "entity User {\n    name String\n}\npage Users {\n    list User {\n        columns missing\n    }\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	for _, expected := range []string{"error[F2402]", "columns missing", "help:", "forma check failed with 1 error"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("stderr does not contain %q:\n%s", expected, stderr.String())
		}
	}
}

func TestCheckCommandAcceptsSelfOnlyInvariant(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "stock.forma")
	source := `type Quantity = Int min 0
entity StockItem {
    onHand Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "checked 1 file: no errors\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestCheckRequiresAnExplicitCompilationUnit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no source files or directories specified") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestDirectoryArgumentIsOneCompilationUnit(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.forma")
	nested := filepath.Join(directory, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(nested, "second.forma")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("role admin\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{first, second} {
		var stdout, stderr bytes.Buffer
		if exitCode := run([]string{"check", path}, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("independent unit %s failed with %d:\n%s", path, exitCode, stderr.String())
		}
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", directory}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("combined directory exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[F2001]: duplicate role `admin`") {
		t.Fatalf("directory was not compiled as one unit:\n%s", stderr.String())
	}
}
