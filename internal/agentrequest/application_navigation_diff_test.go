package agentrequest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

func TestSurfaceTransitionDestinationProducesFocusedSemanticDiff(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "examples", "email-verified-membership.forma"))
	if err != nil {
		t.Fatal(err)
	}
	before := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("membership.forma", string(content))})
	baseline, err := BuildFull(before)
	if err != nil {
		t.Fatal(err)
	}
	afterSource := strings.Replace(string(content), "continue OnboardingGuide", "continue SignIn", 1)
	after := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("moved.forma", afterSource)})
	if len(after.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %#v", after.Diagnostics)
	}
	request, err := BuildIncremental(baseline, after, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantIntent := []SemanticChange{
		{Kind: "changed", NodeID: "page/RegistrationComplete"},
		{Kind: "changed", NodeID: "page/RegistrationComplete/transition/continue"},
	}
	wantFacts := []SemanticChange{{Kind: "changed", NodeID: "fact/page/RegistrationComplete/transition/continue/navigation"}}
	if !reflect.DeepEqual(request.RequestedChange.IntentChanges, wantIntent) {
		t.Fatalf("intent changes = %#v, want %#v", request.RequestedChange.IntentChanges, wantIntent)
	}
	if !reflect.DeepEqual(request.RequestedChange.FactChanges, wantFacts) {
		t.Fatalf("fact changes = %#v, want %#v", request.RequestedChange.FactChanges, wantFacts)
	}
	if len(request.RequestedChange.ReviewRequirementChanges) != 0 {
		t.Fatalf("review changes = %#v", request.RequestedChange.ReviewRequirementChanges)
	}
}
