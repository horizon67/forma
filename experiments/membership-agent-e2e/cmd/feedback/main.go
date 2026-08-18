package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/horizon67/forma/experiments/membership-agent-e2e/internal/retryintegrity"
	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

func main() {
	takeSnapshot := flag.String("snapshot-out", "", "write the retry baseline of the current tree to this path and exit")
	snapshot := flag.String("snapshot", "", "check the tree against this retry baseline before measuring anything")
	flag.Parse()
	if err := run(*takeSnapshot, *snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "membership-agent-e2e feedback:", err)
		os.Exit(1)
	}
}

func run(takeSnapshot, snapshotPath string) error {
	root := "."
	if _, err := os.Stat("go.mod"); err != nil {
		root = filepath.Join("..", "..")
	}
	if takeSnapshot != "" {
		// Taking the baseline is its own mode: it must run before the retry
		// starts, against a tree nobody has edited yet, and it measures nothing.
		return writeSnapshot(root, takeSnapshot)
	}
	out := filepath.Join(root, "experiments", "membership-agent-e2e", "target", "generation-feedback.json")
	blocked, err := retractAndGate(root, snapshotPath, out)
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("retry baseline violated; published blocked Generation Feedback")
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
		feedback.Summary = failedSummary(testRun.stage, failedFacts, tallyResults(feedback.FactCoverage))
		// `forma verify` only validates policy coverage on a succeeded feedback,
		// so anything published here is a claim nothing checks. This run
		// verified no policy, and the coverage vocabulary has no "not-run" for
		// policies the way factCoverage does, so the feedback asserts nothing
		// rather than asserting "satisfied".
		feedback.PolicyCoverage = nil
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
		if testRun.stage == "build" {
			return fmt.Errorf("target build failed; published failed Generation Feedback")
		}
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

// resultTally counts what the run actually observed. Every failure summary is
// built from it so the text cannot drift from the published factCoverage.
type resultTally struct {
	passed int
	failed int
	notRun int
}

func tallyResults(coverage []agentrequest.FactCoverage) resultTally {
	tally := resultTally{}
	for _, entry := range coverage {
		switch entry.Result {
		case "passed":
			tally.passed++
		case "failed":
			tally.failed++
		default:
			tally.notRun++
		}
	}
	return tally
}

func (tally resultTally) String() string {
	return fmt.Sprintf("%d passed, %d failed, %d not-run", tally.passed, tally.failed, tally.notRun)
}

// noPolicyClaim is shared by every failed stage. A failed feedback drops policy
// coverage whatever the stage, so every failure summary has to say so.
const noPolicyClaim = " No implementation policy was verified in this run, so this feedback reports no policy coverage."

func failedSummary(stage string, failedFacts []compiler.SemanticID, tally resultTally) string {
	return stageSummary(stage, failedFacts, tally) + noPolicyClaim
}

func stageSummary(stage string, failedFacts []compiler.SemanticID, tally resultTally) string {
	if stage == "build" {
		// A build failure is not a rejected assertion. Packages that still
		// compiled can observe facts, and a package that compiled can even
		// reject one, so report the tally instead of claiming nothing ran.
		// factResult needs every reference of a fact to complete, so a fact
		// split across a package that built and one that did not is also
		// not-run — being observed only through the failed package is
		// sufficient, not necessary.
		summary := fmt.Sprintf(
			"The target did not compile, so any fact whose required test references did not all complete is not-run: %s. The compiler diagnostic is in diagnostics.",
			tally,
		)
		if len(failedFacts) != 0 {
			summary += fmt.Sprintf(
				" Packages that did compile also rejected %d fact(s): %s.",
				len(failedFacts), joinFactIDs(failedFacts),
			)
		}
		return summary
	}
	if len(failedFacts) == 0 {
		return fmt.Sprintf("Target tests failed; no mapped Acceptance Fact observed the failure: %s.", tally)
	}
	return fmt.Sprintf(
		"Target tests failed; %s. %d mapped Acceptance Fact(s) did not pass: %s.",
		tally, len(failedFacts), joinFactIDs(failedFacts),
	)
}

func joinFactIDs(facts []compiler.SemanticID) string {
	ids := make([]string, len(facts))
	for index, id := range facts {
		ids[index] = string(id)
	}
	return strings.Join(ids, ", ")
}

func changedIntentNodes(request agentrequest.Request) []compiler.SemanticID {
	nodes := append([]compiler.SemanticID(nil), request.RequestedChange.IntentNodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	return nodes
}

// retractAndGate withdraws the previous feedback and then runs the retry gate,
// in that order. The order is the point: a rejected retry must not leave the
// succeeded feedback of an earlier run behind, because `forma verify` would
// still accept that file and the retry would look like it had passed.
func retractAndGate(root, snapshotPath, out string) (bool, error) {
	if err := retryintegrity.Retract(out); err != nil {
		return false, err
	}
	if snapshotPath == "" {
		return false, nil
	}
	// The gate runs before anything is measured. A retry that changed what the
	// measurement means never reaches the test command, so a green `go test`
	// cannot be turned into a succeeded feedback by editing the tests it runs.
	return checkRetryBaseline(root, snapshotPath, out)
}

// retryBaselineCommand records how the gate was invoked, so a blocked feedback
// says what produced it.
const retryBaselineCommand = "go run ./experiments/membership-agent-e2e/cmd/feedback -snapshot <retry-baseline.json>"

// retryBaselineConfig fixes what a repair retry may not change. The Forma
// source, the request, the historical baseline and the manifest are what the
// facts mean; the coverage map and the tests it points at are how they are
// observed; the generator's own package is what decides whether an observation
// counts. The target implementation is deliberately absent: it is what a repair
// is supposed to change.
func retryBaselineConfig(root string) retryintegrity.Config {
	experiment := "experiments/membership-agent-e2e"
	references := make([]string, 0, len(coverage))
	for _, entries := range coverage {
		references = append(references, entries...)
	}
	// Derive sorts, so the map iteration above cannot reach the result.
	return retryintegrity.Config{
		Root: root,
		Fixed: map[string]string{
			experiment + "/app.forma":                                       retryintegrity.ReasonFormaSource,
			experiment + "/generation-request.json":                         retryintegrity.ReasonRequest,
			experiment + "/target/forma.implementation.yaml":                retryintegrity.ReasonManifest,
			experiment + "/cmd/feedback/coverage.go":                        retryintegrity.ReasonCoverageMap,
			"internal/agentrequest/testdata/admin.incremental.request.json": retryintegrity.ReasonBaseline,
		},
		TestRoot:       experiment + "/target",
		TestReferences: references,
		RuleDirs: []string{
			experiment + "/cmd/feedback",
			experiment + "/cmd/retryguard",
			experiment + "/internal/retryintegrity",
		},
	}
}

func writeSnapshot(root, out string) error {
	snapshot, err := retryintegrity.Take(retryBaselineConfig(root))
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("retry baseline pinned %d paths\n", len(snapshot.Entries))
	return nil
}

// checkRetryBaseline reports whether the retry may proceed. It runs the same
// comparison as cmd/retryguard, for convenience while developing. It cannot
// make the guarantee that command makes: this binary compiles whatever the
// retry added to its own package, and that code runs before this check does.
// The authoritative gate is the prebuilt guard.
func checkRetryBaseline(root, snapshotPath, out string) (bool, error) {
	content, err := os.ReadFile(snapshotPath)
	if err != nil {
		return false, fmt.Errorf("read the retry baseline: %w", err)
	}
	var snapshot retryintegrity.Snapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return false, fmt.Errorf("decode the retry baseline: %w", err)
	}
	violations, err := retryintegrity.Check(root, snapshot)
	if err != nil {
		return false, err
	}
	if len(violations) == 0 {
		fmt.Printf("retry baseline intact across %d paths\n", len(snapshot.Entries))
		return false, nil
	}
	if err := retryintegrity.PublishBlocked(out, retryBaselineCommand, violations); err != nil {
		return false, err
	}
	for _, line := range retryintegrity.Diagnostics(violations) {
		fmt.Fprintln(os.Stderr, line)
	}
	return true, nil
}
