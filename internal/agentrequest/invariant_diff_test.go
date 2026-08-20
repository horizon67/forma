package agentrequest

import (
	"reflect"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

func TestAddingInvariantProducesResolvedExpressionAndFactDiff(t *testing.T) {
	baselineResult := compileInvariantRequestSource(t, false)
	baseline, err := BuildFull(baselineResult)
	if err != nil {
		t.Fatal(err)
	}
	currentResult := compileInvariantRequestSource(t, true)
	request, err := BuildIncremental(baseline, currentResult, nil)
	if err != nil {
		t.Fatal(err)
	}

	wantIntent := []SemanticChange{
		{Kind: "changed", NodeID: "entity/StockItem"},
		{Kind: "added", NodeID: "entity/StockItem/invariant/stockAvailable"},
		{Kind: "added", NodeID: "entity/StockItem/invariant/stockAvailable/expression"},
		{Kind: "added", NodeID: "entity/StockItem/invariant/stockAvailable/expression/left"},
		{Kind: "added", NodeID: "entity/StockItem/invariant/stockAvailable/expression/right"},
	}
	wantFacts := []SemanticChange{
		{Kind: "added", NodeID: "fact/entity/StockItem/invariant/stockAvailable/evaluation/satisfied"},
		{Kind: "added", NodeID: "fact/entity/StockItem/invariant/stockAvailable/evaluation/violated"},
		{Kind: "added", NodeID: "fact/page/StockItemEdit/view/form/edit/StockItem/submit/validation/invariant/entity/StockItem/invariant/stockAvailable"},
	}
	wantReviews := []SemanticChange{
		{Kind: "added", NodeID: "review/entity/StockItem/invariant/stockAvailable/concurrent-invariant-enforcement"},
	}
	if !reflect.DeepEqual(request.RequestedChange.IntentChanges, wantIntent) {
		t.Fatalf("intent changes = %#v, want %#v", request.RequestedChange.IntentChanges, wantIntent)
	}
	if !reflect.DeepEqual(request.RequestedChange.FactChanges, wantFacts) {
		t.Fatalf("fact changes = %#v, want %#v", request.RequestedChange.FactChanges, wantFacts)
	}
	if !reflect.DeepEqual(request.RequestedChange.ReviewRequirementChanges, wantReviews) {
		t.Fatalf("review requirement changes = %#v, want %#v", request.RequestedChange.ReviewRequirementChanges, wantReviews)
	}
	if request.RequestedChange.UnchangedIntentNodes != 19 || request.RequestedChange.UnchangedFacts != 25 || request.RequestedChange.UnchangedReviewRequirements != 0 {
		t.Fatalf("unchanged counts = %#v", request.RequestedChange)
	}
	requiredFacts := make(map[compiler.SemanticID]bool, len(request.Verification.RequiredFactIDs))
	for _, id := range request.Verification.RequiredFactIDs {
		requiredFacts[id] = true
	}
	for _, change := range wantFacts {
		if !requiredFacts[change.NodeID] {
			t.Fatalf("required facts omit added invariant fact %s: %v", change.NodeID, request.Verification.RequiredFactIDs)
		}
	}
	if !reflect.DeepEqual(request.Verification.DisplayReviewRequirementIDs, []compiler.SemanticID{wantReviews[0].NodeID}) {
		t.Fatalf("display invariant review requirements = %v", request.Verification.DisplayReviewRequirementIDs)
	}
	if err := ValidateIncrementalBaseline(request, baseline); err != nil {
		t.Fatal(err)
	}
}

func compileInvariantRequestSource(t *testing.T, withInvariant bool) compiler.Result {
	t.Helper()
	invariant := ""
	if withInvariant {
		invariant = "    invariant stockAvailable: reserved <= onHand\n"
	}
	source := `type Quantity = Int min 0
entity StockItem {
    onHand   Quantity required
    reserved Quantity required
` + invariant + `}

page StockItems {
    list StockItem {
        columns onHand, reserved
        actions edit
    }
}

page StockItemDetail(item StockItem) {
    detail item {
        fields onHand, reserved
        actions edit
    }
}

page StockItemEdit(item StockItem) {
    form item {
        fields onHand, reserved
        submit edit
    }
}
`
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("stock.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %#v", result.Diagnostics)
	}
	return result
}
