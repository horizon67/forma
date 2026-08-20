package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/horizon67/forma/internal/agentrequest"
)

func TestRunRetriesRepairsAFailedMeasurementAndVerifies(t *testing.T) {
	measurements := []agentrequest.Feedback{
		{Stage: "test", Status: "failed"},
		{Stage: "test", Status: "succeeded"},
	}
	var attempts []int
	verified := 0
	var output bytes.Buffer
	err := runRetries(2, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurement := measurements[0]
			measurements = measurements[1:]
			return measurement, nil
		},
		repair: func(attempt int, feedback agentrequest.Feedback) (repairResult, error) {
			attempts = append(attempts, attempt)
			if feedback.Stage != "test" || feedback.Status != "failed" {
				t.Fatalf("repair received %s/%s", feedback.Stage, feedback.Status)
			}
			return repairResult{}, nil
		},
		verify: func() error {
			verified++
			return nil
		},
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attempts, []int{1}) || verified != 1 {
		t.Fatalf("attempts=%v verified=%d", attempts, verified)
	}
	if !strings.Contains(output.String(), "repair loop succeeded after 1 attempt") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunRetriesStartsEachAttemptFromTheLatestFeedback(t *testing.T) {
	measurements := []agentrequest.Feedback{
		{Stage: "test", Status: "failed"},
		{Stage: "build", Status: "failed"},
		{Stage: "test", Status: "succeeded"},
	}
	var stages []string
	err := runRetries(2, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurement := measurements[0]
			measurements = measurements[1:]
			return measurement, nil
		},
		repair: func(_ int, feedback agentrequest.Feedback) (repairResult, error) {
			stages = append(stages, feedback.Stage)
			return repairResult{}, nil
		},
		verify: func() error { return nil },
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stages, []string{"test", "build"}) {
		t.Fatalf("repair stages = %v", stages)
	}
}

func TestRunRetriesStopsAtTheGuard(t *testing.T) {
	measurements := []agentrequest.Feedback{
		{Stage: "test", Status: "failed"},
		{Stage: "inspect", Status: "blocked", Summary: "protected test changed"},
	}
	repairs := 0
	verified := false
	err := runRetries(3, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurement := measurements[0]
			measurements = measurements[1:]
			return measurement, nil
		},
		repair: func(_ int, _ agentrequest.Feedback) (repairResult, error) {
			repairs++
			return repairResult{}, nil
		},
		verify: func() error {
			verified = true
			return nil
		},
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "changed the retry baseline") {
		t.Fatalf("error = %v", err)
	}
	if repairs != 1 || verified {
		t.Fatalf("repairs=%d verified=%v", repairs, verified)
	}
}

func TestRunRetriesIsBoundedAndLeavesTheLastFailure(t *testing.T) {
	measurements := []agentrequest.Feedback{
		{Stage: "test", Status: "failed"},
		{Stage: "test", Status: "failed"},
		{Stage: "build", Status: "failed"},
	}
	repairs := 0
	err := runRetries(2, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurement := measurements[0]
			measurements = measurements[1:]
			return measurement, nil
		},
		repair: func(_ int, _ agentrequest.Feedback) (repairResult, error) {
			repairs++
			return repairResult{}, nil
		},
		verify: func() error { t.Fatal("verify called on failure"); return nil },
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "remained failed after 2 attempt") {
		t.Fatalf("error = %v", err)
	}
	if repairs != 2 {
		t.Fatalf("repairs = %d", repairs)
	}
}

func TestRunRetriesDoesNotRepairAnAlreadySuccessfulTree(t *testing.T) {
	repaired := false
	verified := 0
	err := runRetries(2, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			return agentrequest.Feedback{Stage: "test", Status: "succeeded"}, nil
		},
		repair: func(_ int, _ agentrequest.Feedback) (repairResult, error) {
			repaired = true
			return repairResult{}, nil
		},
		verify: func() error { verified++; return nil },
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if repaired || verified != 1 {
		t.Fatalf("repaired=%v verified=%d", repaired, verified)
	}
}

func TestRunRetriesStopsWhenTheRepairProcessCannotProceed(t *testing.T) {
	measurements := 0
	want := errors.New("agent needs human input")
	err := runRetries(2, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurements++
			return agentrequest.Feedback{Stage: "test", Status: "failed"}, nil
		},
		repair: func(_ int, _ agentrequest.Feedback) (repairResult, error) { return repairResult{}, want },
		verify: func() error { t.Fatal("verify called after repair failure"); return nil },
	}, io.Discard)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	if measurements != 1 {
		t.Fatalf("measurements = %d", measurements)
	}
}

func TestRunRetriesPublishesAnIntentGapAfterTheTrustedFailureRepeats(t *testing.T) {
	measurements := []agentrequest.Feedback{
		{Stage: "test", Status: "failed"},
		{Stage: "test", Status: "failed"},
	}
	decision := repairDecision{Status: "intent-gap", Summary: "the request lacks a required choice"}
	published := 0
	err := runRetries(2, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurement := measurements[0]
			measurements = measurements[1:]
			return measurement, nil
		},
		repair: func(_ int, _ agentrequest.Feedback) (repairResult, error) {
			return repairResult{intentGap: &decision}, nil
		},
		verify: func() error { t.Fatal("verify called for an intent gap"); return nil },
		publishIntent: func(feedback agentrequest.Feedback, got repairDecision) error {
			published++
			if feedback.Status != "failed" || got.Summary != decision.Summary {
				t.Fatalf("published %#v from %#v", got, feedback)
			}
			return nil
		},
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires human review") {
		t.Fatalf("error = %v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d", published)
	}
}

func TestRunRetriesRejectsAnIntentGapContradictedBySuccess(t *testing.T) {
	measurements := []agentrequest.Feedback{
		{Stage: "test", Status: "failed"},
		{Stage: "test", Status: "succeeded"},
	}
	decision := repairDecision{Status: "intent-gap"}
	err := runRetries(1, loopActions{
		measure: func() (agentrequest.Feedback, error) {
			measurement := measurements[0]
			measurements = measurements[1:]
			return measurement, nil
		},
		repair: func(_ int, _ agentrequest.Feedback) (repairResult, error) {
			return repairResult{intentGap: &decision}, nil
		},
		verify: func() error { return nil },
		publishIntent: func(agentrequest.Feedback, repairDecision) error {
			t.Fatal("contradicted intent gap was published")
			return nil
		},
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "trusted measurement is test/succeeded") {
		t.Fatalf("error = %v", err)
	}
}

type scriptedRunner struct {
	t       *testing.T
	calls   []invocation
	actions []func(invocation) error
}

func (runner *scriptedRunner) run(call invocation, _, _ io.Writer) error {
	runner.t.Helper()
	runner.calls = append(runner.calls, call)
	if len(runner.actions) == 0 {
		return nil
	}
	action := runner.actions[0]
	runner.actions = runner.actions[1:]
	return action(call)
}

func writeFeedback(t *testing.T, root string, feedback agentrequest.Feedback) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(feedbackPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("project root not found")
		}
		directory = parent
	}
}

func readFixtureFeedback(t *testing.T) agentrequest.Feedback {
	t.Helper()
	return readFeedbackFixture(t, "membership-repair-loop")
}

func readFeedbackFixture(t *testing.T, experiment string) agentrequest.Feedback {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectRoot(t), "experiments", experiment, "generation-feedback.failed.json"))
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := agentrequest.UnmarshalFeedback(content)
	if err != nil {
		t.Fatal(err)
	}
	return feedback
}

func validIntentGapDecision() repairDecision {
	return repairDecision{
		Schema:  repairDecisionSchema,
		Status:  "intent-gap",
		Summary: "The protected test requires Unicode case folding, but the request declares ASCII case folding only.",
		FactIDs: []string{"fact/identity/UserAccount/operation/register/identifier/duplicate"},
		IntentNodes: []string{
			"identity/UserAccount/identifier/email",
		},
		Diagnostics: []string{"Unicode case folding is not present in the identifier canonicalization operations."},
	}
}

func writeDecision(t *testing.T, path string, decision repairDecision) {
	t.Helper()
	content, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFixture(t *testing.T, root, relative string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func testOrchestrator(root string, runner commandRunner) orchestrator {
	return orchestrator{
		root: root, runner: runner,
		tools: trustedTools{
			guard: "/trusted/retryguard", feedback: "/trusted/feedback",
			forma: "/trusted/forma", snapshot: "/trusted/retry-baseline.json",
			goPath: "/trusted/go/bin/go",
		},
		repair:       []string{"/trusted/repair-agent"},
		decisionPath: root + "-repair-decision.json",
		stdout:       io.Discard, stderr: io.Discard,
		baseEnv: []string{"PATH=.", "LANG=C"},
	}
}

func TestMeasureDoesNotRunTheGeneratorAfterTheGuardBlocks(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedRunner{t: t, actions: []func(invocation) error{
		func(call invocation) error {
			if call.name != "/trusted/retryguard" {
				t.Fatalf("first command = %s", call.name)
			}
			writeFeedback(t, root, agentrequest.Feedback{Schema: agentrequest.FeedbackSchema, Stage: "inspect", Status: "blocked"})
			return errors.New("guard rejected retry")
		},
	}}
	orchestrator := testOrchestrator(root, runner)
	feedback, err := orchestrator.measure()
	if err != nil {
		t.Fatal(err)
	}
	if feedback.Status != "blocked" || len(runner.calls) != 1 {
		t.Fatalf("feedback=%s/%s calls=%d", feedback.Stage, feedback.Status, len(runner.calls))
	}
}

func TestMeasureUsesThePrebuiltGeneratorAfterAnIntactGuard(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedRunner{t: t, actions: []func(invocation) error{
		func(call invocation) error {
			if call.name != "/trusted/retryguard" {
				t.Fatalf("first command = %s", call.name)
			}
			return nil
		},
		func(call invocation) error {
			if call.name != "/trusted/feedback" {
				t.Fatalf("second command = %s", call.name)
			}
			writeFeedback(t, root, agentrequest.Feedback{Schema: agentrequest.FeedbackSchema, Stage: "build", Status: "failed"})
			return errors.New("build failed")
		},
	}}
	orchestrator := testOrchestrator(root, runner)
	feedback, err := orchestrator.measure()
	if err != nil {
		t.Fatal(err)
	}
	if feedback.Stage != "build" || feedback.Status != "failed" || len(runner.calls) != 2 {
		t.Fatalf("feedback=%s/%s calls=%d", feedback.Stage, feedback.Status, len(runner.calls))
	}
}

func TestPrepareBuildsEveryTrustedToolBeforeTakingTheSnapshot(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedRunner{t: t}
	orchestrator := testOrchestrator(root, runner)
	if err := orchestrator.prepare(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
	wantOutputs := []string{"/trusted/retryguard", "/trusted/feedback", "/trusted/forma"}
	for index, output := range wantOutputs {
		call := runner.calls[index]
		if call.name != "/trusted/go/bin/go" || !reflect.DeepEqual(call.args[:2], []string{"build", "-o"}) || call.args[2] != output {
			t.Fatalf("build %d = %#v", index, call)
		}
	}
	last := runner.calls[3]
	if last.name != "/trusted/feedback" || !reflect.DeepEqual(last.args, []string{"-snapshot-out", "/trusted/retry-baseline.json"}) {
		t.Fatalf("snapshot call = %#v", last)
	}
}

func TestRepairProcessReceivesOnlyPublicRetryContext(t *testing.T) {
	root := t.TempDir()
	writeFeedback(t, root, agentrequest.Feedback{Schema: agentrequest.FeedbackSchema, Stage: "build", Status: "failed"})
	runner := &scriptedRunner{t: t}
	orchestrator := testOrchestrator(root, runner)
	if _, err := orchestrator.runRepair(2, agentrequest.Feedback{Stage: "build", Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != "/trusted/repair-agent" {
		t.Fatalf("calls = %#v", runner.calls)
	}
	environment := map[string]string{}
	for _, entry := range runner.calls[0].env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[key] = value
		}
	}
	for key, want := range map[string]string{
		"FORMA_RETRY_ATTEMPT":  "2",
		"FORMA_RETRY_STAGE":    "build",
		"FORMA_RETRY_FEEDBACK": filepath.Join(root, filepath.FromSlash(feedbackPath)),
		"FORMA_RETRY_REQUEST":  filepath.Join(root, filepath.FromSlash(requestPath)),
		"FORMA_RETRY_TARGET":   filepath.Join(root, filepath.FromSlash(targetPath)),
		"FORMA_RETRY_DECISION": root + "-repair-decision.json",
	} {
		if environment[key] != want {
			t.Errorf("%s = %q, want %q", key, environment[key], want)
		}
	}
	for _, secret := range []string{"/trusted/retry-baseline.json", "/trusted/retryguard", "/trusted/feedback", "/trusted/forma"} {
		for _, entry := range runner.calls[0].env {
			if strings.Contains(entry, secret) {
				t.Errorf("repair environment exposed %s in %q", secret, entry)
			}
		}
	}
}

func TestFailedRepairRestoresTheTrustedFailedFeedback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(feedbackPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("{\n  \"schema\": \"forma/generation-feedback/v0alpha2\",\n  \"stage\": \"test\",\n  \"status\": \"failed\"\n}\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{t: t, actions: []func(invocation) error{
		func(invocation) error {
			if err := os.WriteFile(path, []byte(`{"status":"succeeded"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			return errors.New("repair process crashed")
		},
	}}
	orchestrator := testOrchestrator(root, runner)
	_, err := orchestrator.runRepair(1, agentrequest.Feedback{Stage: "test", Status: "failed"})
	if err == nil || !strings.Contains(err.Error(), "remains for human inspection") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("feedback after failed repair = %q, want %q", got, want)
	}
}

func TestRepairDecisionMustNameObservedFactsAndSourceMappedNodes(t *testing.T) {
	feedback := readFixtureFeedback(t)
	request := filepath.Join(projectRoot(t), filepath.FromSlash(requestPath))
	for _, testCase := range []struct {
		name   string
		mutate func(*repairDecision)
		want   string
	}{
		{
			name: "unknown fact",
			mutate: func(decision *repairDecision) {
				decision.FactIDs = []string{"fact/unknown"}
			},
			want: "not in the Generation Request",
		},
		{
			name: "passing fact",
			mutate: func(decision *repairDecision) {
				decision.FactIDs = []string{"fact/entity/User/field/team/relation/resolved"}
			},
			want: "was not failed",
		},
		{
			name: "unknown node",
			mutate: func(decision *repairDecision) {
				decision.IntentNodes = []string{"identity/unknown"}
			},
			want: "is not in the Source Map",
		},
		{
			name: "unrelated node",
			mutate: func(decision *repairDecision) {
				decision.IntentNodes = []string{"action/User/activate"}
			},
			want: "do not overlap",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			decision := validIntentGapDecision()
			testCase.mutate(&decision)
			path := filepath.Join(t.TempDir(), "decision.json")
			writeDecision(t, path, decision)
			if _, _, err := readRepairDecision(path, feedback, request); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestBuildFailureCannotSupportAnIntentGapDecision(t *testing.T) {
	feedback := readFeedbackFixture(t, "membership-build-repair-loop")
	decision := validIntentGapDecision()
	path := filepath.Join(t.TempDir(), "decision.json")
	writeDecision(t, path, decision)
	request := filepath.Join(projectRoot(t), filepath.FromSlash(requestPath))
	if _, _, err := readRepairDecision(path, feedback, request); err == nil || !strings.Contains(err.Error(), "needs a rejected Acceptance Fact") {
		t.Fatalf("error = %v", err)
	}
}

func TestIntentGapNeedsRelatedIntentNodesEvenWhenAFactFailed(t *testing.T) {
	feedback := readFixtureFeedback(t)
	feedback.RelatedIntentNodes = nil
	path := filepath.Join(t.TempDir(), "decision.json")
	writeDecision(t, path, validIntentGapDecision())
	request := filepath.Join(projectRoot(t), filepath.FromSlash(requestPath))
	if _, _, err := readRepairDecision(path, feedback, request); err == nil || !strings.Contains(err.Error(), "do not overlap") {
		t.Fatalf("error = %v, want the decision rejected", err)
	}
}

func TestRepairDecisionFileLimitAllowsEveryFieldLimit(t *testing.T) {
	values := func(count, size int) []string {
		result := make([]string, count)
		for index := range result {
			prefix := fmt.Sprintf("%03d", index)
			result[index] = prefix + strings.Repeat("\x00", size-len(prefix))
		}
		return result
	}
	decision := repairDecision{
		Schema: repairDecisionSchema, Status: "intent-gap",
		Summary:     strings.Repeat("\x00", maxDecisionSummaryBytes),
		Diagnostics: values(maxDecisionDiagnostics, maxDecisionDiagnosticBytes),
		FactIDs:     values(maxDecisionEntries, maxDecisionIDBytes),
		IntentNodes: values(maxDecisionEntries, maxDecisionIDBytes),
	}
	if strings.TrimSpace(decision.Summary) == "" || len(decision.Summary) > maxDecisionSummaryBytes {
		t.Fatal("maximum summary does not satisfy its field limit")
	}
	for _, check := range []struct {
		name      string
		values    []string
		maximum   int
		maxLength int
	}{
		{name: "diagnostics", values: decision.Diagnostics, maximum: maxDecisionDiagnostics, maxLength: maxDecisionDiagnosticBytes},
		{name: "factIds", values: decision.FactIDs, maximum: maxDecisionEntries, maxLength: maxDecisionIDBytes},
		{name: "intentNodes", values: decision.IntentNodes, maximum: maxDecisionEntries, maxLength: maxDecisionIDBytes},
	} {
		if err := validateDecisionStrings(check.name, check.values, 1, check.maximum, check.maxLength); err != nil {
			t.Fatalf("maximum %s does not satisfy its field limit: %v", check.name, err)
		}
	}
	content, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) > maxDecisionBytes {
		t.Fatalf("maximum field limits encode to %d bytes, exceeding file limit %d", len(content), maxDecisionBytes)
	}
}

func TestRepairDecisionHasABoundedSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decision.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxDecisionBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readRepairDecision(path, agentrequest.Feedback{}, "unused"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectedRepairDecisionRestoresTheTrustedFailedFeedback(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, requestPath)
	feedback := readFixtureFeedback(t)
	writeFeedback(t, root, feedback)
	feedbackFile := filepath.Join(root, filepath.FromSlash(feedbackPath))
	want, err := os.ReadFile(feedbackFile)
	if err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{t: t, actions: []func(invocation) error{
		func(invocation) error {
			if err := os.WriteFile(feedbackFile, []byte(`{"status":"succeeded"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(root+"-repair-decision.json", []byte(`{"schema":"invalid"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			return nil
		},
	}}
	orchestrator := testOrchestrator(root, runner)
	if _, err := orchestrator.runRepair(1, feedback); err == nil || !strings.Contains(err.Error(), "validate repair decision") {
		t.Fatalf("error = %v", err)
	}
	got, err := os.ReadFile(feedbackFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("feedback after rejected decision = %q, want %q", got, want)
	}
}

func TestIntentGapDecisionRequiresAnUnchangedRepository(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, requestPath)
	feedback := readFixtureFeedback(t)
	writeFeedback(t, root, feedback)
	implementation := filepath.Join(root, "implementation.go")
	if err := os.WriteFile(implementation, []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &scriptedRunner{t: t, actions: []func(invocation) error{
		func(invocation) error {
			writeDecision(t, root+"-repair-decision.json", validIntentGapDecision())
			if err := os.WriteFile(implementation, []byte("package fixture // workaround\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return nil
		},
	}}
	orchestrator := testOrchestrator(root, runner)
	if _, err := orchestrator.runRepair(1, feedback); err == nil || !strings.Contains(err.Error(), "intent-gap decision left repository changes") {
		t.Fatalf("error = %v", err)
	}
}

func TestIntentGapSnapshotIgnoresDeclaredBuildOutputs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "implementation.go")
	if err := os.WriteFile(source, []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{".git", ".forma-build", ".claude/skills", filepath.Dir(feedbackPath), "nested"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for relative, content := range map[string]string{
		".git/index.lock":         "scratch",
		".forma-build/repair.log": "build output",
		".claude/skills/cache":    "tool cache",
		"forma":                   "binary",
		"coverage.out":            "coverage",
		"nested/.DS_Store":        "finder",
		feedbackPath:              `{"status":"blocked"}`,
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	after, err := snapshotRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if changes := compareSnapshots(before, after); len(changes) != 0 {
		t.Fatalf("ignored outputs changed the repository snapshot: %v", changes)
	}
	if err := os.WriteFile(source, []byte("package fixture // changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err = snapshotRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if changes := compareSnapshots(before, after); !reflect.DeepEqual(changes, []string{"modified implementation.go"}) {
		t.Fatalf("source changes = %v", changes)
	}
}

func TestSnapshotIgnoreListMatchesRepositoryIgnoreRules(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(projectRoot(t), ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	var patterns []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	want := []string{"/forma", "/coverage.out", "/.forma-build/", "/.claude/skills/", ".DS_Store"}
	if !reflect.DeepEqual(patterns, want) {
		t.Fatalf(".gitignore patterns = %v; update the intent-gap snapshot rules for %v", patterns, want)
	}
}

func TestPublishIntentGapProducesValidatedBlockedFeedback(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, requestPath)
	copyFixture(t, root, baselinePath)
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(targetPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	feedback := readFixtureFeedback(t)
	runner := &scriptedRunner{t: t}
	orchestrator := testOrchestrator(root, runner)
	if err := orchestrator.publishIntentGap(feedback, validIntentGapDecision()); err != nil {
		t.Fatal(err)
	}
	published, err := readFeedback(filepath.Join(root, filepath.FromSlash(feedbackPath)))
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "blocked" || published.Stage != feedback.Stage {
		t.Fatalf("published %s/%s", published.Stage, published.Status)
	}
	if published.Command != feedback.Command {
		t.Fatalf("command = %q, want measured command %q", published.Command, feedback.Command)
	}
	if len(published.FactCoverage) != len(feedback.FactCoverage) || len(published.PolicyCoverage) != 0 {
		t.Fatalf("facts=%d policies=%d", len(published.FactCoverage), len(published.PolicyCoverage))
	}
	if !strings.Contains(published.Summary, "Human decision required") || len(published.RelatedIntentNodes) != 1 {
		t.Fatalf("summary=%q nodes=%v", published.Summary, published.RelatedIntentNodes)
	}
}

func TestRecordedIntentGapHandoffIsValidatedAndKeepsTheMeasurement(t *testing.T) {
	root := projectRoot(t)
	artifact := filepath.Join(root, "experiments", "membership-automated-repair-loop", "generation-feedback.blocked-intent-gap.json")
	feedback, err := readFeedback(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if feedback.Stage != "test" || feedback.Status != "blocked" {
		t.Fatalf("recorded handoff = %s/%s", feedback.Stage, feedback.Status)
	}
	const measuredCommand = "cd experiments/membership-agent-e2e/target && go test -count=1 -json ./..."
	if feedback.Command != measuredCommand {
		t.Fatalf("command = %q, want %q", feedback.Command, measuredCommand)
	}
	if len(feedback.PolicyCoverage) != 0 || len(feedback.RelatedIntentNodes) != 1 {
		t.Fatalf("policies=%d relatedNodes=%v", len(feedback.PolicyCoverage), feedback.RelatedIntentNodes)
	}
	failed := 0
	for _, coverage := range feedback.FactCoverage {
		if coverage.Result == "failed" {
			failed++
			if string(coverage.FactID) != "fact/identity/UserAccount/operation/register/identifier/duplicate" {
				t.Fatalf("unexpected failed Fact %s", coverage.FactID)
			}
		}
	}
	if failed != 1 {
		t.Fatalf("failed Facts = %d", failed)
	}
	requestContent, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(requestPath)))
	if err != nil {
		t.Fatal(err)
	}
	request, err := agentrequest.UnmarshalRequest(requestContent)
	if err != nil {
		t.Fatal(err)
	}
	baselineContent, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(baselinePath)))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := agentrequest.UnmarshalRequest(baselineContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := agentrequest.ValidateCompletion(request, &baseline, feedback, filepath.Join(root, filepath.FromSlash(targetPath))); err != nil {
		t.Fatalf("recorded handoff is invalid: %v", err)
	}
}

func TestBoundedCommandOutputRemainsValidUTF8(t *testing.T) {
	output := boundedCommandOutput("界"+strings.Repeat("x", 3999), "")
	if !utf8.ValidString(output) || strings.ContainsRune(output, '\uFFFD') {
		t.Fatalf("bounded output split UTF-8: %q", output[len(output)-20:])
	}
}

func TestVerifyUsesThePrebuiltVerifier(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedRunner{t: t}
	orchestrator := testOrchestrator(root, runner)
	if err := orchestrator.verify(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "/trusted/forma" || len(call.args) == 0 || call.args[0] != "verify" {
		t.Fatalf("verify call = %#v", call)
	}
}

func TestTrustedToolEnvironmentCannotPreferARepositoryExecutable(t *testing.T) {
	orchestrator := testOrchestrator(t.TempDir(), &scriptedRunner{t: t})
	environment := orchestrator.trustedToolEnv()
	var path string
	for _, entry := range environment {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			path = value
		}
	}
	if path != "/trusted/go/bin:/usr/bin:/bin" {
		t.Fatalf("PATH = %q", path)
	}
}
