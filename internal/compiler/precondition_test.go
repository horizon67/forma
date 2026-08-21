package compiler

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestActionPreconditionBuildsResolvedIntentAndSourceMap(t *testing.T) {
	result := compilePreconditionFixture(t)
	action := actionByID(t, result.Intent, "action/StockReservation/commit")
	if len(action.Preconditions) != 1 || len(action.Changes) != 1 || action.Atomicity != "all-or-nothing" {
		t.Fatalf("resolved Precondition action = %#v", action)
	}
	precondition := action.Preconditions[0]
	if precondition.ID != "action/StockReservation/commit/precondition/withinRequestLimit" ||
		precondition.Name != "withinRequestLimit" || precondition.Evaluation != "pre-state" {
		t.Fatalf("resolved Precondition = %#v", precondition)
	}
	predicate := precondition.Predicate
	if predicate.ID != SemanticID(string(precondition.ID)+"/expression") || predicate.Kind != "binary-expression" ||
		predicate.Operator != "less-than-or-equal" || predicate.ResultType != "Bool" || predicate.Left == nil || predicate.Right == nil {
		t.Fatalf("resolved predicate = %#v", predicate)
	}
	if predicate.Left.Operator != "add" || predicate.Left.ResultType != "Quantity" || predicate.Left.Left == nil || predicate.Left.Right == nil ||
		!reflect.DeepEqual(predicate.Left.Left.RelationPath, []SemanticID{"entity/StockReservation/field/stock"}) ||
		predicate.Left.Left.Field != "entity/StockItem/field/reserved" || predicate.Left.Right.Field != "entity/StockReservation/field/requestedReserved" ||
		!reflect.DeepEqual(predicate.Right.RelationPath, []SemanticID{"entity/StockReservation/field/plan"}) ||
		predicate.Right.Field != "entity/ReservationPlan/field/requestCeiling" {
		t.Fatalf("resolved predicate tree = %#v", predicate)
	}
	entries := map[SemanticID]string{}
	for _, entry := range result.SourceMap.Entries {
		entries[entry.NodeID] = entry.Kind
	}
	for id, kind := range map[SemanticID]string{
		precondition.ID:         "precondition",
		predicate.ID:            "binary-expression",
		predicate.Left.ID:       "binary-expression",
		predicate.Left.Left.ID:  "field-reference",
		predicate.Left.Right.ID: "field-reference",
		predicate.Right.ID:      "field-reference",
	} {
		if entries[id] != kind {
			t.Errorf("Source Map %s = %q, want %q", id, entries[id], kind)
		}
	}
	if err := ValidateSourceMapCoverage(result.Intent, result.SourceMap); err != nil {
		t.Fatal(err)
	}
}

func TestActionPreconditionAndChangesDeclarationOrderIsSemanticFree(t *testing.T) {
	first := compilePreconditionFixture(t)
	secondSource := strings.Replace(preconditionAcceptanceSource,
		"    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling\n\n    changes {\n        stock.reserved = stock.reserved + plan.approvedReserved\n    }",
		"    changes {\n        stock.reserved = stock.reserved + plan.approvedReserved\n    }\n\n    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling",
		1,
	)
	second := Compile([]SourceFile{NewSourceFile("precondition.forma", secondSource)})
	if len(second.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics after member reorder:\n%s", diagnosticMessages(second.Diagnostics))
	}
	if !reflect.DeepEqual(first.Intent.Actions, second.Intent.Actions) {
		t.Fatalf("action member order changed Resolved Intent:\nfirst=%#v\nsecond=%#v", first.Intent.Actions, second.Intent.Actions)
	}
	firstFacts, err := BuildAcceptanceFacts(first.Intent)
	if err != nil {
		t.Fatal(err)
	}
	secondFacts, err := BuildAcceptanceFacts(second.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstFacts, secondFacts) {
		t.Fatal("action member order changed Acceptance Facts")
	}
}

func TestActionPreconditionFactsFixPriorityBindingsAndSurfaceFeedback(t *testing.T) {
	result := compilePreconditionFixture(t)
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	domain := SemanticID("action/StockReservation/commit")
	surface := SemanticID("page/Reservations/view/list/StockReservation/action/commit")
	precondition := SemanticID("action/StockReservation/commit/precondition/withinRequestLimit")

	accepted := acceptanceFactByID(t, facts, factID(domain, "transition", "accepted", "from", "Pending"))
	assertPreconditionFactInput(t, accepted, precondition, true)
	if accepted.Setup == nil || !reflect.DeepEqual(accepted.Setup.Relations, []FactRelationSetup{
		{Source: "subject/action", Field: "entity/StockReservation/field/plan", Target: "subject/value/plan", Condition: "resolved"},
		{Source: "subject/action", Field: "entity/StockReservation/field/stock", Target: "subject/target", Condition: "resolved"},
	}) {
		t.Fatalf("accepted action binding plan = %#v", accepted.Setup)
	}
	bindings := accepted.Input.Preconditions[0].Bindings
	if !reflect.DeepEqual(bindings, []FactExpressionBinding{
		{Node: SemanticID(string(precondition) + "/expression/left/left"), Subject: "subject/target"},
		{Node: SemanticID(string(precondition) + "/expression/left/right"), Subject: "subject/action"},
		{Node: SemanticID(string(precondition) + "/expression/right"), Subject: "subject/value/plan"},
	}) {
		t.Fatalf("Precondition leaf bindings = %#v", bindings)
	}

	sourceRejected := acceptanceFactByID(t, facts, factID(domain, "transition", "rejected", "from", "Committed"))
	if sourceRejected.Expected.Reason != "source-state-mismatch" || len(sourceRejected.Input.Preconditions) != 0 || len(sourceRejected.Setup.Relations) != 0 {
		t.Fatalf("source rejection priority = %#v", sourceRejected)
	}

	unsatisfied := acceptanceFactByID(t, facts, factID(surface, "precondition", "withinRequestLimit", "unsatisfied", "from", "Pending"))
	assertPreconditionFactInput(t, unsatisfied, precondition, false)
	if unsatisfied.Kind != "action-precondition-unsatisfied" || unsatisfied.Expected.Reason != "precondition-unsatisfied" ||
		!reflect.DeepEqual(unsatisfied.Expected.Feedback, []string{"invalid"}) || unsatisfied.Expected.Atomicity != "no-changes-committed" ||
		len(unsatisfied.Expected.Subjects) != 3 {
		t.Fatalf("surface Precondition rejection = %#v", unsatisfied)
	}

	changesAccepted := acceptanceFactByID(t, facts, factID(domain, "changes", "accepted", "from", "Pending"))
	assertPreconditionFactInput(t, changesAccepted, precondition, true)
	invariantRejected := acceptanceFactByID(t, facts, factID(domain, "changes", "invariant", "entity/StockItem/invariant/stockAvailable", "rejected", "from", "Pending"))
	assertPreconditionFactInput(t, invariantRejected, precondition, true)
	for _, id := range []SemanticID{
		factID(domain, "changes", "target-unavailable", "from", "Pending"),
		factID(domain, "changes", "value-unavailable", "via", "plan", "from", "Pending"),
	} {
		fact := acceptanceFactByID(t, facts, id)
		if len(fact.Input.Preconditions) != 0 {
			t.Fatalf("unavailable Fact %s evaluated Precondition: %#v", id, fact.Input.Preconditions)
		}
	}
	for _, fact := range facts.Facts {
		if strings.Contains(string(fact.ID), "/precondition/withinRequestLimit/value-unavailable/") {
			t.Fatalf("shared target/value relation invented Precondition-only availability Fact %s", fact.ID)
		}
	}
}

func TestPreconditionOnlyActionOwnsRelationAvailabilityAndReviews(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("precondition-only.forma", preconditionOnlySource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	action := actionByID(t, result.Intent, "action/Job/start")
	if action.Atomicity != "" || len(action.Changes) != 0 || len(action.Preconditions) != 1 {
		t.Fatalf("Precondition-only action = %#v", action)
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := acceptanceFactByID(t, facts, "fact/action/Job/start/precondition/withinLimit/value-unavailable/via/limit/from/Pending")
	if unavailable.Kind != "precondition-value-unavailable" || unavailable.Expected.Reason != "value-unavailable" ||
		len(unavailable.Input.Preconditions) != 0 || unavailable.Setup.Relations[0].Condition != "value-unavailable" ||
		unavailable.Setup.Relations[0].Target != "" {
		t.Fatalf("Precondition-only unavailable Fact = %#v", unavailable)
	}
	accepted := acceptanceFactByID(t, facts, "fact/action/Job/start/transition/accepted/from/Pending")
	if got := accepted.Input.Preconditions[0].Bindings; !reflect.DeepEqual(got, []FactExpressionBinding{
		{Node: "action/Job/start/precondition/withinLimit/expression/left/left", Subject: "subject/action"},
		{Node: "action/Job/start/precondition/withinLimit/expression/left/right", Subject: "subject/precondition/limit"},
		{Node: "action/Job/start/precondition/withinLimit/expression/right", Subject: "subject/precondition/limit"},
	}) {
		t.Fatalf("Precondition-only bindings = %#v", got)
	}
	reviews, err := BuildReviewRequirements(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, requirement := range reviews.Requirements {
		kinds = append(kinds, requirement.Kind)
	}
	for _, kind := range []string{"concurrent-action-precondition-enforcement", "cross-entity-value-read-authorization", "exact-numeric-expression-enforcement"} {
		if !slices.Contains(kinds, kind) {
			t.Errorf("Precondition-only reviews %v omit %s", kinds, kind)
		}
	}
	if slices.Contains(kinds, "atomic-changes-enforcement") {
		t.Errorf("Precondition-only action invented Changes review: %v", kinds)
	}
}

func TestActionPreconditionFactValidationRejectsOmissionAndPriorityDrift(t *testing.T) {
	build := func(t *testing.T) (*ResolvedIntent, *AcceptanceFacts) {
		t.Helper()
		intent := compilePreconditionFixture(t).Intent
		facts, err := BuildAcceptanceFacts(intent)
		if err != nil {
			t.Fatal(err)
		}
		return intent, facts
	}
	unsatisfiedID := SemanticID("fact/action/StockReservation/commit/precondition/withinRequestLimit/unsatisfied/from/Pending")
	acceptedID := SemanticID("fact/action/StockReservation/commit/transition/accepted/from/Pending")
	sourceRejectedID := SemanticID("fact/action/StockReservation/commit/transition/rejected/from/Committed")
	tests := []struct {
		name   string
		mutate func(*AcceptanceFacts)
	}{
		{name: "missing false Fact", mutate: func(facts *AcceptanceFacts) {
			filtered := facts.Facts[:0]
			for _, fact := range facts.Facts {
				if fact.ID != unsatisfiedID {
					filtered = append(filtered, fact)
				}
			}
			facts.Facts = filtered
		}},
		{name: "accepted omits true predicate", mutate: func(facts *AcceptanceFacts) {
			fact, _ := acceptanceFactPointerByID(facts, acceptedID)
			fact.Input.Preconditions = nil
		}},
		{name: "false result reversed", mutate: func(facts *AcceptanceFacts) {
			fact, _ := acceptanceFactPointerByID(facts, unsatisfiedID)
			fact.Input.Preconditions[0].Result = true
		}},
		{name: "source reason omitted", mutate: func(facts *AcceptanceFacts) {
			fact, _ := acceptanceFactPointerByID(facts, sourceRejectedID)
			fact.Expected.Reason = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent, facts := build(t)
			test.mutate(facts)
			if err := ValidateAcceptanceFacts(intent, facts); err == nil {
				t.Fatal("tampered Action Precondition Facts passed validation")
			}
		})
	}
}

func TestResolvedPreconditionValidationRejectsUnsupportedOrTamperedIR(t *testing.T) {
	build := func(t *testing.T) *ResolvedIntent {
		t.Helper()
		return compilePreconditionFixture(t).Intent
	}
	tests := []struct {
		name   string
		mutate func(*IRActionPrecondition)
		want   string
	}{
		{name: "identity", mutate: func(value *IRActionPrecondition) { value.ID = "action/StockReservation/commit/precondition/other" }, want: "non-canonical identity or evaluation"},
		{name: "evaluation", mutate: func(value *IRActionPrecondition) { value.Evaluation = "post-state" }, want: "non-canonical identity or evaluation"},
		{name: "root operator", mutate: func(value *IRActionPrecondition) { value.Predicate.Operator = "less-than" }, want: "first <= predicate slice"},
		{name: "root result", mutate: func(value *IRActionPrecondition) { value.Predicate.ResultType = "Quantity" }, want: "first <= predicate slice"},
		{name: "field owner", mutate: func(value *IRActionPrecondition) {
			value.Predicate.Right.Field = "entity/StockItem/field/onHand"
		}, want: "does not reference a required scalar field"},
		{name: "relation path", mutate: func(value *IRActionPrecondition) {
			value.Predicate.Right.RelationPath = append(value.Predicate.Right.RelationPath, "entity/StockReservation/field/stock")
		}, want: "not a canonical field reference"},
		{name: "duplicate expression identity", mutate: func(value *IRActionPrecondition) {
			right := *value.Predicate.Right
			value.Predicate.Right = &IRExpression{ID: right.ID, Kind: "binary-expression", ResultType: "Quantity", Operator: "add", Left: &right, Right: &right}
		}, want: "duplicate semantic node"},
		{name: "second addition with canonical identities", mutate: func(value *IRActionPrecondition) {
			right := *value.Predicate.Right
			leftLeaf := right
			leftLeaf.ID = semanticID(string(right.ID), "left")
			rightLeaf := right
			rightLeaf.ID = semanticID(string(right.ID), "right")
			rightLeaf.Field = "entity/ReservationPlan/field/approvedReserved"
			value.Predicate.Right = &IRExpression{
				ID: right.ID, Kind: "binary-expression", ResultType: "Quantity", Operator: "add",
				Left: &leftLeaf, Right: &rightLeaf,
			}
		}, want: "first <= predicate slice"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := build(t)
			test.mutate(&intent.Actions[len(intent.Actions)-1].Preconditions[0])
			if err := ValidateResolvedIntent(intent); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("tampered Precondition error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestActionPreconditionDiagnosticsCloseTheFirstSlice(t *testing.T) {
	base := preconditionAcceptanceSource
	tests := []struct {
		name, source, code string
	}{
		{name: "two preconditions", source: strings.Replace(base, "    changes {", "    precondition second: requestedReserved <= plan.requestCeiling\n\n    changes {", 1), code: "F2810"},
		{name: "duplicate precondition", source: strings.Replace(base, "    changes {", "    precondition withinRequestLimit: requestedReserved <= plan.requestCeiling\n\n    changes {", 1), code: "F2810"},
		{name: "two additions", source: strings.Replace(base, "stock.reserved + requestedReserved <= plan.requestCeiling", "stock.reserved + requestedReserved <= plan.approvedReserved + plan.requestCeiling", 1), code: "F2811"},
		{name: "unknown field", source: strings.Replace(base, "stock.reserved + requestedReserved <= plan.requestCeiling", "stock.reserved + missing <= plan.requestCeiling", 1), code: "F2812"},
		{name: "state value", source: strings.Replace(base, "stock.reserved + requestedReserved <= plan.requestCeiling", "stock.reserved + status <= plan.requestCeiling", 1), code: "F2812"},
		{name: "optional relation", source: strings.Replace(base, "plan              ReservationPlan required", "plan              ReservationPlan", 1), code: "F2812"},
		{name: "collection relation", source: strings.Replace(base, "plan              ReservationPlan required", "plan              [ReservationPlan] required", 1), code: "F2812"},
		{name: "optional terminal", source: strings.Replace(base, "requestCeiling   Quantity required", "requestCeiling   Quantity", 1), code: "F2812"},
		{name: "collection terminal", source: strings.Replace(base, "requestCeiling   Quantity required", "requestCeiling   [Quantity] required", 1), code: "F2812"},
		{name: "relation terminal", source: strings.Replace(base, "plan.requestCeiling", "plan", 1), code: "F2812"},
		{name: "two-hop relation", source: twoHopPreconditionSource(base), code: "F2812"},
		{name: "nominal mismatch", source: strings.Replace(base, "requestCeiling   Quantity required", "requestCeiling   Int required", 1), code: "F2813"},
		{name: "non-closed addition", source: strings.Replace(base, "type Quantity = Int min 0", "type Quantity = Int min 0 max 100", 1), code: "F2814"},
		{name: "empty body", source: strings.Replace(base, "    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling\n\n    changes {\n        stock.reserved = stock.reserved + plan.approvedReserved\n    }", "", 1), code: "F2810"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Compile([]SourceFile{NewSourceFile("precondition-invalid.forma", test.source)})
			if !slices.Contains(diagnosticCodes(result.Diagnostics), test.code) {
				t.Fatalf("missing diagnostic %s:\n%s", test.code, diagnosticMessages(result.Diagnostics))
			}
		})
	}
}

func twoHopPreconditionSource(source string) string {
	source = strings.Replace(source, "entity StockReservation {", "entity ReservationEnvelope {\n    plan ReservationPlan required\n}\n\nentity StockReservation {\n    envelope          ReservationEnvelope required", 1)
	return strings.Replace(source, "plan.requestCeiling", "envelope.plan.requestCeiling", 1)
}

func assertPreconditionFactInput(t *testing.T, fact AcceptanceFact, id SemanticID, result bool) {
	t.Helper()
	if fact.Input == nil || len(fact.Input.Preconditions) != 1 {
		t.Fatalf("Fact %s Precondition input = %#v", fact.ID, fact.Input)
	}
	input := fact.Input.Preconditions[0]
	if input.Precondition != id || input.Subject != "subject/action" || input.Evaluation != "pre-state" || input.Result != result ||
		input.Expression.ID != SemanticID(string(id)+"/expression") {
		t.Fatalf("Fact %s Precondition input = %#v", fact.ID, input)
	}
}

func compilePreconditionFixture(t *testing.T) Result {
	t.Helper()
	result := Compile([]SourceFile{NewSourceFile("precondition.forma", preconditionAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	return result
}

const preconditionAcceptanceSource = `role staff

type Quantity = Int min 0

entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}

entity ReservationPlan {
    approvedReserved Quantity required
    requestCeiling   Quantity required
}

entity StockReservation {
    stock             StockItem required
    plan              ReservationPlan required
    requestedReserved Quantity required
    state status Pending | Committed initial Pending
}

action StockReservation.commit: Pending -> Committed confirm allow staff {
    precondition withinRequestLimit: stock.reserved + requestedReserved <= plan.requestCeiling

    changes {
        stock.reserved = stock.reserved + plan.approvedReserved
    }
}

page Reservations {
    allow staff
    list StockReservation {
        columns requestedReserved, status
        actions commit
    }
}
`

const preconditionOnlySource = `role staff

type Quantity = Int min 0

entity Limit {
    increment Quantity required
    maximum   Quantity required
}

entity Job {
    used  Quantity required
    limit Limit required
    state status Pending | Started initial Pending
}

action Job.start: Pending -> Started allow staff {
    precondition withinLimit: used + limit.increment <= limit.maximum
}

page Jobs {
    allow staff
    list Job {
        columns used, status
        actions start
    }
}
`
