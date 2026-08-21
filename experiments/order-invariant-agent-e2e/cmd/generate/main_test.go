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

func TestRecordedArtifactsMatchSourceCompilerAndCoverageRules(t *testing.T) {
	root := formaRoot(t)
	wantRequest, wantCoverage, err := buildArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	experiment := filepath.Join(root, "experiments", "order-invariant-agent-e2e")
	assertFileEquals(t, filepath.Join(experiment, "generation-request.json"), wantRequest)
	assertFileEquals(t, filepath.Join(experiment, "coverage.json"), wantCoverage)
}

func TestEveryPageFactIsMeasuredThroughAnHTTPHandler(t *testing.T) {
	root := formaRoot(t)
	experiment := filepath.Join(root, "experiments", "order-invariant-agent-e2e")
	requestContent, err := os.ReadFile(filepath.Join(experiment, "generation-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := agentrequest.UnmarshalRequest(requestContent)
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

	pageFacts := 0
	accessFacts := 0
	accessFactsPerTest := map[string]int{}
	for _, fact := range request.AcceptanceFacts.Facts {
		if !strings.HasPrefix(string(fact.Subject), "page/") {
			continue
		}
		pageFacts++
		isAccess := fact.Kind == "access-allowed" || fact.Kind == "access-denied"
		if isAccess {
			accessFacts++
		}
		references := coverage[fact.ID]
		hasHTTPBoundary := false
		for _, reference := range references {
			if strings.HasPrefix(reference, "internal/web/server_test.go#Test") {
				hasHTTPBoundary = true
				if isAccess {
					accessFactsPerTest[reference]++
				}
			}
		}
		if !hasHTTPBoundary {
			t.Errorf("page Fact %s (%s) is not measured through an HTTP handler: %v", fact.ID, fact.Kind, references)
		}
	}
	if pageFacts != 242 {
		t.Fatalf("page Facts = %d, want 242", pageFacts)
	}
	if accessFacts != 104 {
		t.Fatalf("access Facts = %d, want 104", accessFacts)
	}
	for reference, count := range accessFactsPerTest {
		if count > 8 {
			t.Errorf("HTTP access test %s carries %d Facts, want at most 8", reference, count)
		}
	}
}

func assertFileEquals(t *testing.T, filename string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; run go run ./experiments/order-invariant-agent-e2e/cmd/generate", filename)
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
