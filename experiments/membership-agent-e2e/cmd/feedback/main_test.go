package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
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
