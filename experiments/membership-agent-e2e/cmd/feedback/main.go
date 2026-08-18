package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "membership-agent-e2e feedback:", err)
		os.Exit(1)
	}
}

func run() error {
	root := "."
	if _, err := os.Stat("go.mod"); err != nil {
		root = filepath.Join("..", "..")
	}
	out := filepath.Join(root, "experiments", "membership-agent-e2e", "target", "generation-feedback.json")
	// The feedback is a measurement, not a document. Retract the previous one
	// before this run starts so a crash cannot leave a succeeded record that
	// `forma verify` would still accept. This run then publishes succeeded or
	// failed coverage with a single rename.
	if err := os.Remove(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("retract the previous feedback: %w", err)
	}
	requestPath := filepath.Join(root, "experiments", "membership-agent-e2e", "generation-request.json")
	content, err := os.ReadFile(requestPath)
	if err != nil {
		return fmt.Errorf("read Generation Request: %w", err)
	}
	request, err := agentrequest.UnmarshalRequest(content)
	if err != nil {
		return fmt.Errorf("decode Generation Request: %w", err)
	}

	targetDir := filepath.Join(root, "experiments", "membership-agent-e2e", "target")
	module, err := modulePath(targetDir)
	if err != nil {
		return err
	}
	if err := validateCoverageReferences(targetDir, module); err != nil {
		return err
	}

	testRun := runTargetTests(targetDir)

	var missing, extra []string
	seen := map[string]bool{}
	failedFacts := []compiler.SemanticID{}
	feedback := agentrequest.Feedback{
		Schema: agentrequest.FeedbackSchema, Stage: testRun.stage, Status: "succeeded",
		Command: feedbackCommand,
		Summary: "Email-verified membership added to the admin application; every Acceptance Fact is observed by a repository test.",
	}
	for _, fact := range request.AcceptanceFacts.Facts {
		key := strings.TrimPrefix(string(fact.ID), "fact/")
		references, ok := coverage[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		seen[key] = true
		result, err := testRun.factResult(module, references)
		if err != nil {
			return err
		}
		if result == "failed" {
			failedFacts = append(failedFacts, fact.ID)
		}
		feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
			FactID: fact.ID, TestReferences: references, Result: result,
		})
	}
	for key := range coverage {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		return fmt.Errorf("fact coverage mismatch: unmapped=%v unreachable=%v", missing, extra)
	}
	sort.Slice(feedback.FactCoverage, func(i, j int) bool {
		return feedback.FactCoverage[i].FactID < feedback.FactCoverage[j].FactID
	})
	sort.Slice(failedFacts, func(i, j int) bool { return failedFacts[i] < failedFacts[j] })

	if request.ImplementationPolicy != nil {
		for _, policy := range request.ImplementationPolicy.Policies {
			entry, ok := policyEvidence[policy.ID]
			if !ok {
				return fmt.Errorf("no evidence recorded for implementation policy %s", policy.ID)
			}
			feedback.PolicyCoverage = append(feedback.PolicyCoverage, entry)
		}
		sort.Slice(feedback.PolicyCoverage, func(i, j int) bool {
			return feedback.PolicyCoverage[i].PolicyID < feedback.PolicyCoverage[j].PolicyID
		})
	}

	if testRun.failed {
		nodes, err := agentrequest.IntentNodesForFacts(request, failedFacts)
		if err != nil {
			return err
		}
		feedback.Status = "failed"
		feedback.RelatedIntentNodes = nodes
		feedback.Diagnostics = testRun.diagnostics
		feedback.Summary = failedSummary(failedFacts)
	} else {
		for _, coverage := range feedback.FactCoverage {
			if coverage.Result != "passed" {
				return fmt.Errorf("test command succeeded but fact %s is %s", coverage.FactID, coverage.Result)
			}
		}
		for _, node := range changedIntentNodes(request) {
			feedback.RelatedIntentNodes = append(feedback.RelatedIntentNodes, node)
		}
	}

	encoded, err := json.MarshalIndent(feedback, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(out, append(encoded, '\n')); err != nil {
		return err
	}
	fmt.Printf("mapped %d facts and %d policies\n", len(feedback.FactCoverage), len(feedback.PolicyCoverage))
	fmt.Printf("coverage fingerprint %s\n", coverageFingerprint())
	fmt.Print(testRun.summary)
	if testRun.failed {
		return fmt.Errorf("target tests failed; published failed Generation Feedback")
	}
	return nil
}

func writeAtomic(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".generation-feedback-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func failedSummary(failedFacts []compiler.SemanticID) string {
	if len(failedFacts) == 0 {
		return "Target tests failed; no mapped Acceptance Fact observed the failure."
	}
	ids := make([]string, len(failedFacts))
	for index, id := range failedFacts {
		ids[index] = string(id)
	}
	return fmt.Sprintf("Target tests failed; %d mapped Acceptance Fact(s) did not pass: %s.", len(failedFacts), strings.Join(ids, ", "))
}

func changedIntentNodes(request agentrequest.Request) []compiler.SemanticID {
	nodes := append([]compiler.SemanticID(nil), request.RequestedChange.IntentNodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	return nodes
}
