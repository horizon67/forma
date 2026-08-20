package compiler

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMembershipReviewRequirementsGolden(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	requirements, err := BuildReviewRequirements(intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements.Requirements) != 3 {
		t.Fatalf("review requirements = %d, want 3", len(requirements.Requirements))
	}
	wantIDs := []SemanticID{
		"review/identity/UserAccount/fixture-fidelity",
		"review/identity/UserAccount/secret-redaction",
		"review/identity/UserAccount/secret-storage",
	}
	if got := ReviewRequirementIDs(requirements); !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("review requirement IDs = %v, want %v", got, wantIDs)
	}
	content, err := json.MarshalIndent(requirements, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, '\n')
	path := filepath.Join("testdata", "membership.reviews.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(content, expected) {
		t.Fatalf("Review Requirements differ from %s", path)
	}
	for _, forbidden := range []string{`"http"`, `"route"`, `"cookie"`, `"sql"`, `"bcrypt"`, `"tokenValue"`, `"passwordValue"`} {
		if bytes.Contains(bytes.ToLower(content), bytes.ToLower([]byte(forbidden))) {
			t.Errorf("Review Requirements contain target or secret vocabulary %s", forbidden)
		}
	}
}

func TestReviewRequirementsAreCompilerOwnedAndSeparateFromFacts(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	requirements, err := BuildReviewRequirements(intent)
	if err != nil {
		t.Fatal(err)
	}
	requirements.Requirements[0].Instruction = "agent says this passed"
	if err := ValidateReviewRequirements(intent, requirements); err == nil || !strings.Contains(err.Error(), "differs from canonical") {
		t.Fatalf("tampered instruction error = %v", err)
	}

	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range facts.Facts {
		if strings.HasPrefix(string(fact.ID), "review/") {
			t.Fatalf("review requirement leaked into Acceptance Facts: %s", fact.ID)
		}
	}
}

func TestReviewRequirementsAreExplicitlyEmptyWithoutIdentity(t *testing.T) {
	intent := &ResolvedIntent{Version: ResolvedIntentVersion}
	requirements, err := BuildReviewRequirements(intent)
	if err != nil {
		t.Fatal(err)
	}
	if requirements.Requirements == nil || len(requirements.Requirements) != 0 {
		t.Fatalf("empty review requirements = %#v", requirements.Requirements)
	}
}

func TestInvariantConcurrencyIsAnExplicitReviewRequirement(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	requirements, err := BuildReviewRequirements(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements.Requirements) != 1 {
		t.Fatalf("invariant review requirements = %#v, want exactly one", requirements.Requirements)
	}
	invariant := result.Intent.Entities[0].Invariants[0]
	want := ReviewRequirement{
		ID:          "review/entity/StockItem/invariant/stockAvailable/concurrent-invariant-enforcement",
		Kind:        "concurrent-invariant-enforcement",
		Subject:     invariant.ID,
		SourceNodes: invariantFactSourceNodes(invariant),
		Instruction: reviewInstructions["concurrent-invariant-enforcement"],
	}
	if !reflect.DeepEqual(requirements.Requirements[0], want) {
		t.Fatalf("invariant review requirement = %#v, want %#v", requirements.Requirements[0], want)
	}

	requirements.Requirements[0].SourceNodes = []SemanticID{invariant.ID}
	if err := ValidateReviewRequirements(result.Intent, requirements); err == nil || !strings.Contains(err.Error(), "differs from canonical") {
		t.Fatalf("tampered invariant review requirement error = %v", err)
	}
}
