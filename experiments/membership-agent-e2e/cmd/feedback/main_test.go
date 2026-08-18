package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

const targetModule = "example.com/forma-admin-target"

func TestCoverageFingerprintIsLocked(t *testing.T) {
	if got := coverageFingerprint(); got != coverageFingerprintLocked {
		t.Fatalf("coverage fingerprint = %s, want locked %s", got, coverageFingerprintLocked)
	}
}

func TestParseTestJSONMarksUnobservedFactsNotRun(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"fail","Package":"example.com/forma-admin-target/internal/web","FailedBuild":"example.com/forma-admin-target/internal/web"}`,
		`{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Output":"# example.com/forma-admin-target/internal/web\n"}`,
		`{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Output":"internal/web/membership.go:1: expected 'package', found 'xxx'\n"}`,
	}, "\n")
	testRun := parseTestJSON([]byte(output))
	if !testRun.failed || testRun.stage != "build" {
		t.Fatalf("run = failed %v stage %q", testRun.failed, testRun.stage)
	}
	result, err := testRun.factResult(targetModule, []string{"internal/web/membership_e2e_test.go#TestDuplicateIdentifierCoversExactAndCanonicalForms"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "not-run" {
		t.Fatalf("unobserved fact result = %s, want not-run", result)
	}
	if len(testRun.diagnostics) == 0 {
		t.Fatal("build failure must produce diagnostics")
	}
	for _, line := range testRun.diagnostics {
		if strings.Contains(line, "s)") || strings.Contains(line, "\t0.") {
			t.Fatalf("diagnostics still contain a duration: %q", line)
		}
	}
}

func TestParseTestJSONDistinguishesPackagesAndStripsDurations(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"pass","Package":"example.com/forma-admin-target/internal/store","Test":"TestSave"}`,
		`{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Test":"TestSave","Output":"--- FAIL: TestSave (0.12s)\n"}`,
		`{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Test":"TestSave","Output":"    membership_e2e_test.go:739: the duplicate attempt's secret signed in: 303\n"}`,
		`{"Action":"fail","Package":"example.com/forma-admin-target/internal/web","Test":"TestSave"}`,
		`{"Action":"fail","Package":"example.com/forma-admin-target/internal/web"}`,
		`{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Output":"FAIL\texample.com/forma-admin-target/internal/web\t0.234s\n"}`,
	}, "\n")
	testRun := parseTestJSON([]byte(output))
	web, err := testRun.factResult(targetModule, []string{"internal/web/membership_e2e_test.go#TestSave"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := testRun.factResult(targetModule, []string{"internal/store/store_test.go#TestSave"})
	if err != nil {
		t.Fatal(err)
	}
	if web != "failed" {
		t.Fatalf("web TestSave = %s, want failed", web)
	}
	if store != "passed" {
		t.Fatalf("store TestSave = %s, want passed", store)
	}
	joined := strings.Join(testRun.diagnostics, "\n")
	if !strings.Contains(joined, "the duplicate attempt's secret signed in") {
		t.Fatalf("diagnostics = %#v", testRun.diagnostics)
	}
	for _, line := range testRun.diagnostics {
		if strings.Contains(line, "(0.12s)") || strings.HasSuffix(line, "0.234s") {
			t.Fatalf("diagnostics still contain a duration: %q", line)
		}
	}
}

func TestParseTestJSONTreatsSubtestFailureAsParentFailure(t *testing.T) {
	output := `{"Action":"fail","Package":"example.com/forma-admin-target/internal/web","Test":"TestDuplicateIdentifierCoversExactAndCanonicalForms/mia@example.com"}`
	testRun := parseTestJSON([]byte(output))
	result, err := testRun.factResult(targetModule, []string{"internal/web/membership_e2e_test.go#TestDuplicateIdentifierCoversExactAndCanonicalForms"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "failed" {
		t.Fatalf("parent fact result = %s, want failed", result)
	}
}

func TestParseTestJSONTreatsSkippedSubtestAsNotRun(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"pass","Package":"example.com/forma-admin-target/internal/web","Test":"TestDuplicateIdentifierCoversExactAndCanonicalForms"}`,
		`{"Action":"skip","Package":"example.com/forma-admin-target/internal/web","Test":"TestDuplicateIdentifierCoversExactAndCanonicalForms/mia@example.com"}`,
	}, "\n")
	testRun := parseTestJSON([]byte(output))
	result, err := testRun.factResult(targetModule, []string{"internal/web/membership_e2e_test.go#TestDuplicateIdentifierCoversExactAndCanonicalForms"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "not-run" {
		t.Fatalf("skip child + pass parent = %s, want not-run", result)
	}
}

func TestCoverageMapMatchesGenerationRequest(t *testing.T) {
	root := formaRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "experiments", "membership-agent-e2e", "generation-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := agentrequest.UnmarshalRequest(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.AcceptanceFacts.Facts) != len(coverage) {
		t.Fatalf("coverage map has %d entries, request has %d facts", len(coverage), len(request.AcceptanceFacts.Facts))
	}
	for _, fact := range request.AcceptanceFacts.Facts {
		key := strings.TrimPrefix(string(fact.ID), "fact/")
		if _, ok := coverage[key]; !ok {
			t.Errorf("unmapped fact %s", fact.ID)
		}
	}
	for key := range coverage {
		found := false
		for _, fact := range request.AcceptanceFacts.Facts {
			if strings.TrimPrefix(string(fact.ID), "fact/") == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("coverage map entry %s is not in the Generation Request", key)
		}
	}
	target := filepath.Join(root, "experiments", "membership-agent-e2e", "target")
	if err := validateCoverageReferences(target, targetModule); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageReferencesRequireExistingTestFunctions(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "internal", "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package web\n\nimport \"testing\"\n\nfunc TestSave(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(web, "foo_test.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	mapping := map[string][]string{"fact": {"internal/web/foo_test.go#TestSave"}}
	if err := validateReferenceFiles(dir, targetModule, mapping); err != nil {
		t.Fatal(err)
	}

	missing := map[string][]string{"fact": {"internal/web/missing_test.go#TestSave"}}
	if err := validateReferenceFiles(dir, targetModule, missing); err == nil {
		t.Fatal("missing file must be rejected")
	}

	wrong := map[string][]string{"fact": {"internal/web/foo_test.go#TestMissing"}}
	if err := validateReferenceFiles(dir, targetModule, wrong); err == nil || !strings.Contains(err.Error(), "no Test function") {
		t.Fatalf("missing Test function error = %v", err)
	}
}

func formaRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		content, err := os.ReadFile(mod)
		if err == nil && strings.HasPrefix(string(content), "module github.com/horizon67/forma\n") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("forma module root not found")
		}
		dir = parent
	}
}

// buildFailureJSON mirrors the events `go test -count=1 -json ./...` emitted for
// this experiment's build fault under go1.26.1. The compiler error arrives as
// build-output records keyed by ImportPath with no Package and no Test, while
// internal/store still builds and runs. A hand-written fixture that folds the
// compiler error into package output would hide that shape, so these lines are
// copied from a real run.
const buildFailureJSON = `{"ImportPath":"example.com/forma-admin-target/internal/web","Action":"build-output","Output":"# example.com/forma-admin-target/internal/web\n"}
{"ImportPath":"example.com/forma-admin-target/internal/web","Action":"build-output","Output":"internal/web/membership.go:309:47: too many arguments in call to stored.Matches\n"}
{"ImportPath":"example.com/forma-admin-target/internal/web","Action":"build-output","Output":"\thave (string, string)\n"}
{"ImportPath":"example.com/forma-admin-target/internal/web","Action":"build-output","Output":"\twant (string)\n"}
{"ImportPath":"example.com/forma-admin-target/internal/web","Action":"build-fail"}
{"Action":"run","Package":"example.com/forma-admin-target/internal/store","Test":"TestUpdateUserEnforcesUniqueEmail"}
{"Action":"pass","Package":"example.com/forma-admin-target/internal/store","Test":"TestUpdateUserEnforcesUniqueEmail"}
{"Action":"pass","Package":"example.com/forma-admin-target/internal/store","Elapsed":0.99}
{"Action":"fail","Package":"example.com/forma-admin-target/internal/web","FailedBuild":"example.com/forma-admin-target/internal/web"}
{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Output":"FAIL\texample.com/forma-admin-target/internal/web [build failed]\n"}
{"Action":"fail","Package":"example.com/forma-admin-target/cmd/server","FailedBuild":"example.com/forma-admin-target/internal/web"}
{"Action":"output","Package":"example.com/forma-admin-target/cmd/server","Output":"FAIL\texample.com/forma-admin-target/cmd/server [build failed]\n"}`

func TestBuildFailureIsStagedAsBuildNotTest(t *testing.T) {
	testRun := parseTestJSON([]byte(buildFailureJSON))
	if !testRun.failed {
		t.Fatal("a package that fails to build must fail the run")
	}
	if testRun.stage != "build" {
		t.Fatalf("stage = %q, want build", testRun.stage)
	}
	for key, action := range testRun.tests {
		if key.Package == "example.com/forma-admin-target/internal/web" {
			t.Fatalf("a package that never compiled reported test %s as %s", key.Name, action)
		}
	}
}

func TestBuildFailureKeepsCompilerDiagnostic(t *testing.T) {
	testRun := parseTestJSON([]byte(buildFailureJSON))
	if len(testRun.diagnostics) == 0 {
		t.Fatal("a build failure must not publish empty diagnostics")
	}
	joined := strings.Join(testRun.diagnostics, "\n")
	for _, want := range []string{
		"# example.com/forma-admin-target/internal/web",
		"internal/web/membership.go:309:47: too many arguments in call to stored.Matches",
		"want (string)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("diagnostics lost the compiler error %q:\n%s", want, joined)
		}
	}
	// "[build failed]" alone names a symptom. It must not be the whole record.
	if !strings.Contains(testRun.diagnostics[0], "membership.go") &&
		!strings.HasPrefix(testRun.diagnostics[0], "# ") {
		t.Fatalf("diagnostics lead with %q instead of the compiler error", testRun.diagnostics[0])
	}
}

func TestBuildFailureLeavesUnobservedFactsNotRun(t *testing.T) {
	testRun := parseTestJSON([]byte(buildFailureJSON))
	for _, testCase := range []struct {
		name       string
		references []string
		want       string
	}{
		{
			name:       "only the package that did not compile",
			references: []string{memberTests + "TestDuplicateIdentifierCoversExactAndCanonicalForms"},
			want:       "not-run",
		},
		{
			name:       "a package that did not compile plus one that passed",
			references: []string{adminTests + "TestEditValidationAndFailure", storeTests + "TestUpdateUserEnforcesUniqueEmail"},
			want:       "not-run",
		},
		{
			name:       "a package that compiled and ran",
			references: []string{storeTests + "TestUpdateUserEnforcesUniqueEmail"},
			want:       "passed",
		},
		{
			name:       "a package that was never reached",
			references: []string{executableTests + "TestExecutableServesBothSurfaces"},
			want:       "not-run",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := testRun.factResult(targetModule, testCase.references)
			if err != nil {
				t.Fatal(err)
			}
			if result != testCase.want {
				t.Fatalf("result = %s, want %s", result, testCase.want)
			}
			// No assertion was evaluated, so nothing may be reported as a
			// rejected fact.
			if result == "failed" {
				t.Fatal("a build failure must not mark a fact failed")
			}
		})
	}
}

func TestTestFailureIsNotStagedAsBuild(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"output","Package":"example.com/forma-admin-target/internal/web","Test":"TestDuplicateIdentifierCoversExactAndCanonicalForms","Output":"--- FAIL: TestDuplicateIdentifierCoversExactAndCanonicalForms (0.10s)\n"}`,
		`{"Action":"fail","Package":"example.com/forma-admin-target/internal/web","Test":"TestDuplicateIdentifierCoversExactAndCanonicalForms"}`,
		`{"Action":"fail","Package":"example.com/forma-admin-target/internal/web"}`,
	}, "\n")
	testRun := parseTestJSON([]byte(output))
	if testRun.stage != "test" {
		t.Fatalf("stage = %q, want test", testRun.stage)
	}
	result, err := testRun.factResult(targetModule, []string{memberTests + "TestDuplicateIdentifierCoversExactAndCanonicalForms"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "failed" {
		t.Fatalf("an executed and rejected assertion = %s, want failed", result)
	}
}

func TestFailedSummaryDoesNotCallABuildFailureATestFailure(t *testing.T) {
	summary := failedSummary("build", nil, resultTally{notRun: 81})
	if !strings.Contains(summary, "not compile") || !strings.Contains(summary, "0 passed, 0 failed, 81 not-run") {
		t.Fatalf("build summary = %q", summary)
	}
	if strings.Contains(summary, "tests failed") {
		t.Fatalf("build summary claims a test failure: %q", summary)
	}
}

// A build failure can coexist with observations: packages that still compile run
// their tests, and one of them can even reject a fact. The summary must report
// the tally rather than assert that nothing was observed.
func TestBuildSummaryReportsWhatStillRan(t *testing.T) {
	summary := failedSummary("build", nil, resultTally{passed: 4, notRun: 77})
	if !strings.Contains(summary, "4 passed, 0 failed, 77 not-run") {
		t.Fatalf("build summary hides the packages that ran: %q", summary)
	}
	if strings.Contains(summary, "no Acceptance Fact was observed") {
		t.Fatalf("build summary claims nothing was observed: %q", summary)
	}

	rejected := failedSummary("build", []compiler.SemanticID{"fact/a"}, resultTally{passed: 3, failed: 1, notRun: 77})
	if !strings.Contains(rejected, "fact/a") || !strings.Contains(rejected, "3 passed, 1 failed, 77 not-run") {
		t.Fatalf("build summary drops a fact rejected by a package that compiled: %q", rejected)
	}
}

// The policy claim is dropped on every failed stage, so every failed stage has
// to say so. The build branch alone is not enough.
func TestEveryFailedStageSummaryDeclaresNoPolicyWasVerified(t *testing.T) {
	for _, testCase := range []struct {
		stage string
		facts []compiler.SemanticID
		tally resultTally
	}{
		{stage: "build", tally: resultTally{notRun: 81}},
		{stage: "test", tally: resultTally{passed: 80, notRun: 1}},
		{stage: "test", facts: []compiler.SemanticID{"fact/a"}, tally: resultTally{passed: 80, failed: 1}},
	} {
		summary := failedSummary(testCase.stage, testCase.facts, testCase.tally)
		if !strings.Contains(summary, "no policy coverage") {
			t.Fatalf("%s summary does not say policy coverage is absent: %q", testCase.stage, summary)
		}
		if !strings.Contains(summary, testCase.tally.String()) {
			t.Fatalf("%s summary does not carry the tally %q: %q", testCase.stage, testCase.tally, summary)
		}
	}
}

func TestFallbackDiagnosticsAreNeverEmpty(t *testing.T) {
	if got := fallbackDiagnostics(nil); len(got) == 0 {
		t.Fatal("a failure with no parsable output must still carry a diagnostic")
	}
}

// TestFailedFeedbackPublishesNoPolicyClaim pins the recorded artifacts, not the
// code path: ValidateCompletion skips implementationpolicy.ValidateCoverage
// whenever the status is not succeeded, so a policy status published on a failed
// run is a claim nothing verifies. The generator is shared, so both the build
// failure and the test failure experiment have to hold.
func TestFailedFeedbackPublishesNoPolicyClaim(t *testing.T) {
	root := formaRoot(t)
	for _, experiment := range []string{"membership-build-repair-loop", "membership-repair-loop"} {
		t.Run(experiment, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, "experiments", experiment, "generation-feedback.failed.json"))
			if err != nil {
				t.Fatal(err)
			}
			var feedback agentrequest.Feedback
			if err := json.Unmarshal(content, &feedback); err != nil {
				t.Fatal(err)
			}
			if feedback.Status != "failed" {
				t.Fatalf("fixture is not a failed feedback: %s", feedback.Status)
			}
			if len(feedback.PolicyCoverage) != 0 {
				t.Fatalf("failed feedback claims %d unverified policy status(es)", len(feedback.PolicyCoverage))
			}
			if len(feedback.Diagnostics) == 0 {
				t.Fatal("failed feedback must carry diagnostics")
			}
			// Guard against a vacuous pass: if the field names ever drift, the
			// checks below would run over nothing and prove nothing.
			if len(feedback.FactCoverage) != len(coverage) {
				t.Fatalf("failed feedback decoded %d facts, coverage map has %d", len(feedback.FactCoverage), len(coverage))
			}
			if !strings.Contains(feedback.Summary, "no policy coverage") {
				t.Fatalf("summary does not say policy coverage is absent: %q", feedback.Summary)
			}
			switch feedback.Stage {
			case "build":
				// Nothing compiled that observes a fact, so no fact may be
				// reported as an evaluated assertion.
				for _, entry := range feedback.FactCoverage {
					if entry.Result == "passed" || entry.Result == "failed" {
						t.Fatalf("build failure recorded fact %s as %s", entry.FactID, entry.Result)
					}
				}
				if len(feedback.RelatedIntentNodes) != 0 {
					t.Fatalf("build failure related %d intent nodes without a failed fact", len(feedback.RelatedIntentNodes))
				}
			case "test":
				// An assertion ran and was rejected, so the artifact must still
				// carry that fact and the intent nodes it reached.
				failed := 0
				for _, entry := range feedback.FactCoverage {
					if entry.Result == "failed" {
						failed++
					}
				}
				if failed == 0 {
					t.Fatal("test failure recorded no failed fact")
				}
				if len(feedback.RelatedIntentNodes) == 0 {
					t.Fatal("test failure recorded no related intent nodes")
				}
			default:
				t.Fatalf("unexpected stage %q", feedback.Stage)
			}
		})
	}
}
