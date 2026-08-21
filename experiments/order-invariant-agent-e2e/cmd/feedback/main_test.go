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

func TestRecordedCoverageMatchesGenerationRequestAndTargetTests(t *testing.T) {
	root := formaRoot(t)
	experiment := filepath.Join(root, "experiments", "order-invariant-agent-e2e")
	content, err := os.ReadFile(filepath.Join(experiment, "generation-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := agentrequest.UnmarshalRequest(content)
	if err != nil {
		t.Fatal(err)
	}
	coverageContent, err := os.ReadFile(filepath.Join(experiment, "coverage.json"))
	if err != nil {
		t.Fatal(err)
	}
	coverage := map[compiler.SemanticID][]string{}
	if err := json.Unmarshal(coverageContent, &coverage); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(experiment, "target")
	module, err := modulePath(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCoverage(request, coverage, target, module); err != nil {
		t.Fatal(err)
	}
	if got, want := len(coverage), 278; got != want {
		t.Fatalf("coverage entries = %d, want %d", got, want)
	}
}

func TestValidateCoverageRejectsOmittedInventedAndMissingReferences(t *testing.T) {
	root := formaRoot(t)
	experiment := filepath.Join(root, "experiments", "order-invariant-agent-e2e")
	content, err := os.ReadFile(filepath.Join(experiment, "generation-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := agentrequest.UnmarshalRequest(content)
	if err != nil {
		t.Fatal(err)
	}
	coverageContent, err := os.ReadFile(filepath.Join(experiment, "coverage.json"))
	if err != nil {
		t.Fatal(err)
	}
	base := map[compiler.SemanticID][]string{}
	if err := json.Unmarshal(coverageContent, &base); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(experiment, "target")
	module, err := modulePath(target)
	if err != nil {
		t.Fatal(err)
	}

	copyCoverage := func() map[compiler.SemanticID][]string {
		result := make(map[compiler.SemanticID][]string, len(base))
		for id, references := range base {
			result[id] = append([]string(nil), references...)
		}
		return result
	}
	first := request.AcceptanceFacts.Facts[0].ID

	omitted := copyCoverage()
	delete(omitted, first)
	if err := validateCoverage(request, omitted, target, module); err == nil || !strings.Contains(err.Error(), "coverage omits") {
		t.Fatalf("omitted Fact error = %v", err)
	}

	invented := copyCoverage()
	invented[compiler.SemanticID("fact/invented")] = append([]string(nil), invented[first]...)
	if err := validateCoverage(request, invented, target, module); err == nil || !strings.Contains(err.Error(), "coverage invents") {
		t.Fatalf("invented Fact error = %v", err)
	}

	missing := copyCoverage()
	missing[first] = []string{"internal/web/server_test.go#TestDoesNotExist"}
	if err := validateCoverage(request, missing, target, module); err == nil || !strings.Contains(err.Error(), "missing test") {
		t.Fatalf("missing test error = %v", err)
	}
}

func TestReferenceIdentityRejectsEscapesAndMalformedReferences(t *testing.T) {
	for _, reference := range []string{
		"../outside_test.go#TestOutside",
		"internal/web/../store/store_test.go#TestStore",
		"/absolute/server_test.go#TestServer",
		"internal/web/server_test.go",
		"#TestServer",
		"internal/web/server_test.go#",
	} {
		t.Run(reference, func(t *testing.T) {
			if _, _, err := referenceIdentity("example.com/target", reference); err == nil {
				t.Fatalf("referenceIdentity accepted %q", reference)
			}
		})
	}

	pkg, name, err := referenceIdentity("example.com/target", "internal/web/server_test.go#TestServer")
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "example.com/target/internal/web" || name != "TestServer" {
		t.Fatalf("identity = %s#%s", pkg, name)
	}
}

func formaRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.HasPrefix(string(content), "module github.com/horizon67/forma\n") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Forma module root not found")
		}
		dir = parent
	}
}
