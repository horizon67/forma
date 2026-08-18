package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/experiments/membership-agent-e2e/internal/retryintegrity"
	"github.com/horizon67/forma/internal/agentrequest"
)

// staleSucceeded is what a previous retry left behind. It is the artifact
// `forma verify` would read and accept if a rejected retry stopped without
// withdrawing it.
const staleSucceeded = `{"schema":"forma/generation-feedback/v0alpha2","stage":"test","status":"succeeded","summary":"every Acceptance Fact is observed"}`

func setup(t *testing.T) (root, snapshotPath, feedbackPath string) {
	t.Helper()
	root = t.TempDir()
	protected := filepath.Join(root, "coverage.go")
	if err := os.WriteFile(protected, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := retryintegrity.Take(retryintegrity.Config{
		Root:  root,
		Fixed: map[string]string{"coverage.go": retryintegrity.ReasonCoverageMap},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath = filepath.Join(t.TempDir(), "retry-baseline.json")
	if err := os.WriteFile(snapshotPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	feedbackPath = filepath.Join(root, "generation-feedback.json")
	if err := os.WriteFile(feedbackPath, []byte(staleSucceeded), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, snapshotPath, feedbackPath
}

func readFeedback(t *testing.T, path string) agentrequest.Feedback {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no feedback at %s: %v", path, err)
	}
	var feedback agentrequest.Feedback
	if err := json.Unmarshal(content, &feedback); err != nil {
		t.Fatal(err)
	}
	return feedback
}

// TestGuardReplacesAStaleSucceededFeedbackOnRejection is the case the gate
// exists for. The guard is the authoritative stop, so stopping at the guard has
// to be safe on its own: continuing into the generator is exactly what a
// tampered retry wants.
func TestGuardReplacesAStaleSucceededFeedbackOnRejection(t *testing.T) {
	root, snapshotPath, feedbackPath := setup(t)
	if err := os.WriteFile(filepath.Join(root, "coverage.go"), []byte("package main // weakened\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(root, snapshotPath, feedbackPath, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	feedback := readFeedback(t, feedbackPath)
	if feedback.Status != "blocked" || feedback.Stage != "inspect" {
		t.Fatalf("published %s/%s, want blocked/inspect", feedback.Status, feedback.Stage)
	}
	if strings.Contains(feedback.Summary, "every Acceptance Fact is observed") {
		t.Fatal("the stale succeeded feedback survived the rejection")
	}
	if len(feedback.FactCoverage) != 0 || len(feedback.PolicyCoverage) != 0 {
		t.Fatalf("a blocked retry claimed %d facts and %d policies", len(feedback.FactCoverage), len(feedback.PolicyCoverage))
	}
	if len(feedback.Diagnostics) == 0 {
		t.Fatal("a blocked retry must say which paths moved")
	}
	if !strings.Contains(stderr.String(), "do not run the generator") {
		t.Fatalf("the guard does not tell the orchestrator to stop:\n%s", stderr.String())
	}
}

// TestGuardWithdrawsTheStaleFeedbackEvenWhenIntact keeps the withdrawal ahead of
// the check. A retry that is allowed to proceed must still not carry the
// previous run's artifact into its own measurement.
func TestGuardWithdrawsTheStaleFeedbackEvenWhenIntact(t *testing.T) {
	root, snapshotPath, feedbackPath := setup(t)

	var stdout, stderr bytes.Buffer
	if code := run(root, snapshotPath, feedbackPath, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, stderr.String())
	}
	if _, err := os.Stat(feedbackPath); err == nil {
		t.Fatal("the previous feedback survived an intact check")
	}
	if !strings.Contains(stdout.String(), "intact") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

// TestGuardRefusesToRunWithoutBothPaths keeps the gate from silently degrading
// into a check that owns no feedback.
func TestGuardRefusesToRunWithoutBothPaths(t *testing.T) {
	root, snapshotPath, feedbackPath := setup(t)
	for _, testCase := range []struct{ name, snapshot, feedback string }{
		{name: "no snapshot", feedback: feedbackPath},
		{name: "no feedback", snapshot: snapshotPath},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(root, testCase.snapshot, testCase.feedback, &stdout, &stderr); code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			// The stale artifact must not be withdrawn by a call that never ran.
			if content, err := os.ReadFile(feedbackPath); err != nil || !strings.Contains(string(content), "succeeded") {
				t.Fatalf("a refused invocation touched the feedback: %v", err)
			}
		})
	}
}

// TestGuardReportsAnUnusableSnapshotWithoutPublishing separates "the retry was
// tampered with" from "the gate could not run". The second must not be recorded
// as a blocked retry, but it must still leave no succeeded artifact.
func TestGuardReportsAnUnusableSnapshotWithoutPublishing(t *testing.T) {
	root, _, feedbackPath := setup(t)
	missing := filepath.Join(t.TempDir(), "absent.json")

	var stdout, stderr bytes.Buffer
	if code := run(root, missing, feedbackPath, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, err := os.Stat(feedbackPath); err == nil {
		t.Fatal("the stale succeeded feedback survived an unusable baseline")
	}
}
