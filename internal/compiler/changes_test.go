package compiler

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestChangesBuildsResolvedIntentAndSourceMap(t *testing.T) {
	result := compileChangesFixture(t)
	action := actionByID(t, result.Intent, "action/StockReservation/commit")
	if action.Atomicity != "all-or-nothing" || len(action.Changes) != 1 {
		t.Fatalf("resolved Changes action = %#v", action)
	}
	change := action.Changes[0]
	wantChange := SemanticID("action/StockReservation/commit/change/stock/reserved")
	if change.ID != wantChange || change.Evaluation != "pre-state" || change.Target.Binding != "self" ||
		!reflect.DeepEqual(change.Target.RelationPath, []SemanticID{"entity/StockReservation/field/stock"}) ||
		change.Target.Field != "entity/StockItem/field/reserved" {
		t.Fatalf("resolved change = %#v", change)
	}
	if change.Value.ID != SemanticID(string(wantChange)+"/value") || change.Value.Kind != "field-reference" ||
		change.Value.Binding != "self" || change.Value.Field != "entity/StockReservation/field/reservedAfter" ||
		change.Value.ResultType != "Quantity" {
		t.Fatalf("resolved pre-state value = %#v", change.Value)
	}

	entries := map[SemanticID]string{}
	for _, entry := range result.SourceMap.Entries {
		entries[entry.NodeID] = entry.Kind
	}
	wantEntries := map[SemanticID]string{
		wantChange: "change",
		SemanticID(string(wantChange) + "/target"): "change-target",
		SemanticID(string(wantChange) + "/value"):  "field-reference",
	}
	for id, kind := range wantEntries {
		if entries[id] != kind {
			t.Errorf("Source Map %s = %q, want %q", id, entries[id], kind)
		}
	}
	if err := ValidateSourceMapCoverage(result.Intent, result.SourceMap); err != nil {
		t.Fatal(err)
	}

	ref := actionRefByID(t, result.Intent, "page/Reservations/view/list/StockReservation/action/commit")
	if ref.Action != action.ID || !reflect.DeepEqual(ref.InteractionStates, []string{"invalid", "failure"}) {
		t.Fatalf("resolved action reference = %#v", ref)
	}
}

func TestChangesFactsDescribeDomainAndSurfaceAtomicOutcomes(t *testing.T) {
	result := compileChangesFixture(t)
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	domain := SemanticID("action/StockReservation/commit")
	surface := SemanticID("page/Reservations/view/list/StockReservation/action/commit")
	invariant := SemanticID("entity/StockItem/invariant/stockAvailable")

	accepted := acceptanceFactByID(t, facts, factID(domain, "changes", "accepted", "from", "Pending"))
	if accepted.Kind != "changes-accepted" || accepted.Expected.Outcome != "accepted" ||
		accepted.Expected.Atomicity != "all-changes-committed" || accepted.Expected.AppliedMutations != 2 ||
		len(accepted.Expected.Subjects) != 2 || accepted.Input == nil || len(accepted.Input.Invariants) != 1 ||
		!accepted.Input.Invariants[0].Result {
		t.Fatalf("accepted Changes fact = %#v", accepted)
	}
	if got := accepted.Expected.Subjects[0]; got.Handle != "subject/action" || got.State == nil || got.State.Value != "Committed" {
		t.Fatalf("accepted action subject = %#v", got)
	}
	if got := accepted.Expected.Subjects[1]; got.Handle != "subject/target" || len(got.Fields) != 1 ||
		got.Fields[0].Field != "entity/StockItem/field/reserved" || got.Fields[0].ValueField != "entity/StockReservation/field/reservedAfter" {
		t.Fatalf("accepted target subject = %#v", got)
	}

	rejected := acceptanceFactByID(t, facts, factID(surface, "changes", "invariant", string(invariant), "rejected", "from", "Pending"))
	if rejected.Kind != "action-changes-invariant-rejected" || rejected.Expected.Reason != "invariant-violated" ||
		rejected.Expected.Violated != invariant || !reflect.DeepEqual(rejected.Expected.Feedback, []string{"invalid"}) ||
		rejected.Expected.Atomicity != "no-changes-committed" || len(rejected.Expected.Subjects) != 2 {
		t.Fatalf("surface invariant rejection = %#v", rejected)
	}

	unavailable := acceptanceFactByID(t, facts, factID(surface, "changes", "target-unavailable", "from", "Pending"))
	if unavailable.Expected.Reason != "target-unavailable" || !reflect.DeepEqual(unavailable.Expected.Feedback, []string{"failure"}) ||
		unavailable.Setup == nil || len(unavailable.Setup.Relations) != 1 || unavailable.Setup.Relations[0].Condition != "target-unavailable" {
		t.Fatalf("surface target-unavailable = %#v", unavailable)
	}

	transition := acceptanceFactByID(t, facts, factID(domain, "transition", "accepted", "from", "Pending"))
	if transition.Kind != "transition-accepted" || transition.Expected.Subjects[0].State.Value != "Committed" {
		t.Fatalf("domain transition = %#v", transition)
	}
	sourceRejected := acceptanceFactByID(t, facts, factID(surface, "transition", "rejected", "from", "Committed"))
	if sourceRejected.Kind != "action-transition-source-rejected" || !reflect.DeepEqual(sourceRejected.Expected.Feedback, []string{"invalid"}) ||
		!sourceRejected.Expected.Subjects[0].Unchanged {
		t.Fatalf("surface source rejection = %#v", sourceRejected)
	}
	declined := acceptanceFactByID(t, facts, factID(surface, "confirmation", "declined"))
	if declined.Expected.Dispatch != "none" || declined.Input.Action.Dispatches != 0 || !declined.Expected.Subjects[0].Unchanged {
		t.Fatalf("declined confirmation = %#v", declined)
	}
	acceptedConfirmation := acceptanceFactByID(t, facts, factID(surface, "confirmation", "accepted"))
	if acceptedConfirmation.Expected.Dispatch != "once" {
		t.Fatalf("accepted confirmation = %#v", acceptedConfirmation)
	}
}

func TestChangesAcceptanceValidationRejectsOmissionAndSemanticDrift(t *testing.T) {
	build := func(t *testing.T) (*ResolvedIntent, *AcceptanceFacts) {
		t.Helper()
		result := compileChangesFixture(t)
		facts, err := BuildAcceptanceFacts(result.Intent)
		if err != nil {
			t.Fatal(err)
		}
		return result.Intent, facts
	}

	t.Run("missing action fact", func(t *testing.T) {
		intent, facts := build(t)
		id := factID(SemanticID("action/StockReservation/commit"), "changes", "accepted", "from", "Pending")
		filtered := make([]AcceptanceFact, 0, len(facts.Facts)-1)
		for _, fact := range facts.Facts {
			if fact.ID != id {
				filtered = append(filtered, fact)
			}
		}
		facts.Facts = filtered
		if err := ValidateAcceptanceFacts(intent, facts); err == nil || !strings.Contains(err.Error(), "missing compiler-derived fact") {
			t.Fatalf("missing Changes fact error = %v", err)
		}
	})

	t.Run("invariant result", func(t *testing.T) {
		intent, facts := build(t)
		id := factID(SemanticID("action/StockReservation/commit"), "changes", "accepted", "from", "Pending")
		fact, ok := acceptanceFactPointerByID(facts, id)
		if !ok {
			t.Fatal("missing accepted Changes fact")
		}
		fact.Input.Invariants[0].Result = false
		if err := ValidateAcceptanceFacts(intent, facts); err == nil || !strings.Contains(err.Error(), "differs from its canonical derivation") {
			t.Fatalf("tampered Changes fact error = %v", err)
		}
	})
}

func TestResolvedChangesValidationRejectsUnsupportedOrTamperedIR(t *testing.T) {
	build := func(t *testing.T) *ResolvedIntent {
		t.Helper()
		return compileChangesFixture(t).Intent
	}
	transitionRef := func(t *testing.T, intent *ResolvedIntent) *IRActionRef {
		t.Helper()
		for pageIndex := range intent.Pages {
			for viewIndex := range intent.Pages[pageIndex].Views {
				for refIndex := range intent.Pages[pageIndex].Views[viewIndex].Actions {
					ref := &intent.Pages[pageIndex].Views[viewIndex].Actions[refIndex]
					if ref.Kind == "transition" {
						return ref
					}
				}
			}
		}
		t.Fatal("missing transition action reference")
		return nil
	}
	standardRef := func(t *testing.T, intent *ResolvedIntent) *IRActionRef {
		t.Helper()
		for pageIndex := range intent.Pages {
			for viewIndex := range intent.Pages[pageIndex].Views {
				for refIndex := range intent.Pages[pageIndex].Views[viewIndex].Actions {
					ref := &intent.Pages[pageIndex].Views[viewIndex].Actions[refIndex]
					if ref.Kind == "standard" && ref.Name == "edit" {
						return ref
					}
				}
			}
		}
		t.Fatal("missing standard edit action reference")
		return nil
	}
	tests := []struct {
		name   string
		mutate func(*testing.T, *ResolvedIntent)
		want   string
	}{
		{
			name: "non-atomic Changes",
			mutate: func(_ *testing.T, intent *ResolvedIntent) {
				intent.Actions[0].Atomicity = ""
			},
			want: "outside the first Changes slice",
		},
		{
			name: "post-state evaluation",
			mutate: func(_ *testing.T, intent *ResolvedIntent) {
				intent.Actions[0].Changes[0].Evaluation = "post-state"
			},
			want: "non-canonical evaluation or target binding",
		},
		{
			name: "non-self value binding",
			mutate: func(_ *testing.T, intent *ResolvedIntent) {
				intent.Actions[0].Changes[0].Value.Binding = "target"
			},
			want: "not a canonical field reference",
		},
		{
			name: "transition feedback omitted",
			mutate: func(t *testing.T, intent *ResolvedIntent) {
				transitionRef(t, intent).InteractionStates = nil
			},
			want: "non-canonical transition binding or feedback",
		},
		{
			name: "standard navigation invents invalid feedback",
			mutate: func(t *testing.T, intent *ResolvedIntent) {
				standardRef(t, intent).InteractionStates = []string{"invalid"}
			},
			want: "non-canonical feedback",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := build(t)
			test.mutate(t, intent)
			err := ValidateResolvedIntent(intent)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolvedRelationValueValidationRejectsUnsupportedOrTamperedIR(t *testing.T) {
	build := func(t *testing.T) *ResolvedIntent {
		t.Helper()
		result := Compile([]SourceFile{NewSourceFile("relation-value-validation.forma", relationValueAcceptanceSource)})
		if len(result.Diagnostics) != 0 {
			t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
		}
		return result.Intent
	}
	tests := []struct {
		name   string
		mutate func(*IRExpression, *ResolvedIntent)
		want   string
	}{
		{
			name: "two relation segments",
			mutate: func(value *IRExpression, _ *ResolvedIntent) {
				value.RelationPath = append(value.RelationPath, value.RelationPath[0])
			},
			want: "not a canonical field reference",
		},
		{
			name: "optional relation",
			mutate: func(_ *IRExpression, intent *ResolvedIntent) {
				irFieldPointerByID(t, intent, "entity/InventorySnapshot/field/stock").Required = false
			},
			want: "does not traverse one required to-one relation",
		},
		{
			name: "to-many relation",
			mutate: func(_ *IRExpression, intent *ResolvedIntent) {
				irFieldPointerByID(t, intent, "entity/InventorySnapshot/field/stock").Collection = true
			},
			want: "does not traverse one required to-one relation",
		},
		{
			name: "relation terminal",
			mutate: func(value *IRExpression, _ *ResolvedIntent) {
				value.Field = "entity/InventorySnapshot/field/stock"
			},
			want: "does not reference a required scalar field on its resolved owner",
		},
		{
			name: "foreign terminal owner",
			mutate: func(value *IRExpression, _ *ResolvedIntent) {
				value.Field = "entity/InventorySnapshot/field/observedOnHand"
			},
			want: "does not reference a required scalar field on its resolved owner",
		},
		{
			name: "optional terminal",
			mutate: func(_ *IRExpression, intent *ResolvedIntent) {
				irFieldPointerByID(t, intent, "entity/StockItem/field/onHand").Required = false
			},
			want: "does not reference a required scalar field on its resolved owner",
		},
		{
			name: "result type drift",
			mutate: func(value *IRExpression, _ *ResolvedIntent) {
				value.ResultType = "Int"
			},
			want: "does not reference a required scalar field on its resolved owner",
		},
		{
			name: "binding drift",
			mutate: func(value *IRExpression, _ *ResolvedIntent) {
				value.Binding = "target"
			},
			want: "not a canonical field reference",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := build(t)
			value := &intent.Actions[0].Changes[0].Value
			test.mutate(value, intent)
			err := ValidateResolvedIntent(intent)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestChangesDoesNotInventARejectionForAnUnaffectedInvariant(t *testing.T) {
	source := strings.Replace(
		changesAcceptanceSource,
		"    invariant stockAvailable: reserved <= onHand\n",
		"    invariant stockAvailable: reserved <= onHand\n    invariant stableOnHand: onHand <= onHand\n",
		1,
	)
	result := Compile([]SourceFile{NewSourceFile("changes-unrelated-invariant.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	accepted := acceptanceFactByID(t, facts, factID(SemanticID("action/StockReservation/commit"), "changes", "accepted", "from", "Pending"))
	if accepted.Input == nil || len(accepted.Input.Invariants) != 2 {
		t.Fatalf("accepted Changes fact invariants = %#v", accepted.Input)
	}
	for _, fact := range facts.Facts {
		if fact.Expected.Violated == "entity/StockItem/invariant/stableOnHand" {
			t.Fatalf("unaffected invariant received impossible rejection Fact %s", fact.ID)
		}
	}
}

func TestChangesRelationValueBuildsResolvedIRFactsAndUnavailableOutcome(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("relation-value.forma", relationValueAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	action := actionByID(t, result.Intent, "action/InventorySnapshot/capture")
	change := action.Changes[0]
	if !reflect.DeepEqual(change.Value.RelationPath, []SemanticID{"entity/InventorySnapshot/field/stock"}) ||
		change.Value.Field != "entity/StockItem/field/onHand" || change.Value.ResultType != "Quantity" {
		t.Fatalf("resolved relation value = %#v", change.Value)
	}
	var valueEntry *SourceMapEntry
	for index := range result.SourceMap.Entries {
		if result.SourceMap.Entries[index].NodeID == change.Value.ID {
			valueEntry = &result.SourceMap.Entries[index]
		}
	}
	if valueEntry == nil || valueEntry.Kind != "field-reference" ||
		valueEntry.Span.Start.Offset < 0 || valueEntry.Span.End.Offset > len(relationValueAcceptanceSource) ||
		relationValueAcceptanceSource[valueEntry.Span.Start.Offset:valueEntry.Span.End.Offset] != "stock.onHand" {
		t.Fatalf("relation value Source Map entry = %#v", valueEntry)
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	domain := SemanticID("action/InventorySnapshot/capture")
	surface := SemanticID("page/Snapshots/view/list/InventorySnapshot/action/capture")
	accepted := acceptanceFactByID(t, facts, factID(domain, "changes", "accepted", "from", "Pending"))
	if accepted.Setup == nil || len(accepted.Setup.Relations) != 1 || accepted.Setup.Relations[0].Target != "subject/value" {
		t.Fatalf("accepted relation value setup = %#v", accepted.Setup)
	}
	if got := accepted.Expected.Subjects[0].Fields[0]; got.ValueSubject != "subject/value" || got.ValueField != "entity/StockItem/field/onHand" {
		t.Fatalf("accepted relation value expectation = %#v", got)
	}
	unavailable := acceptanceFactByID(t, facts, factID(surface, "changes", "value-unavailable", "from", "Pending"))
	if unavailable.Kind != "action-changes-value-unavailable" || unavailable.Expected.Reason != "value-unavailable" ||
		!reflect.DeepEqual(unavailable.Expected.Feedback, []string{"failure"}) || unavailable.Setup == nil ||
		len(unavailable.Setup.Relations) != 1 || unavailable.Setup.Relations[0].Condition != "value-unavailable" ||
		unavailable.Expected.Atomicity != "no-changes-committed" || unavailable.Expected.AppliedMutations != 0 {
		t.Fatalf("value unavailable Fact = %#v", unavailable)
	}
	facts.Facts = slices.DeleteFunc(facts.Facts, func(fact AcceptanceFact) bool {
		return fact.ID == factID(domain, "changes", "value-unavailable", "from", "Pending")
	})
	if err := ValidateAcceptanceFacts(result.Intent, facts); err == nil || !strings.Contains(err.Error(), "missing compiler-derived fact") {
		t.Fatalf("missing value-unavailable Fact error = %v", err)
	}
}

func TestChangesRelationValueSharesTheTargetSubject(t *testing.T) {
	source := strings.Replace(relationValueAcceptanceSource,
		"observedOnHand = stock.onHand", "stock.onHand = stock.onHand", 1)
	result := Compile([]SourceFile{NewSourceFile("shared-relation-value.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	domain := SemanticID("action/InventorySnapshot/capture")
	accepted := acceptanceFactByID(t, facts, factID(domain, "changes", "accepted", "from", "Pending"))
	if len(accepted.Setup.Subjects) != 2 || len(accepted.Setup.Relations) != 1 ||
		accepted.Expected.Subjects[1].Fields[0].ValueSubject != "subject/target" {
		t.Fatalf("shared target/value Fact = %#v", accepted)
	}
	targetUnavailable := acceptanceFactByID(t, facts, factID(domain, "changes", "target-unavailable", "from", "Pending"))
	for _, source := range []SemanticID{
		"entity/InventorySnapshot/field/stock",
		"entity/StockItem",
		"entity/StockItem/field/onHand",
	} {
		if !containsSemanticID(targetUnavailable.SourceNodes, source) {
			t.Errorf("target-unavailable Fact omits value provenance %s: %v", source, targetUnavailable.SourceNodes)
		}
	}
	for _, fact := range facts.Facts {
		if strings.Contains(string(fact.ID), "/changes/value-unavailable/") {
			t.Fatalf("shared relation received unreachable Fact %s", fact.ID)
		}
	}
}

func TestChangesRelationValueKeepsDistinctTargetAndValueSubjects(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("distinct-relation-value.forma", distinctRelationValueSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	domain := SemanticID("action/Transfer/apply")
	accepted := acceptanceFactByID(t, facts, factID(domain, "changes", "accepted", "from", "Pending"))
	if accepted.Setup == nil || len(accepted.Setup.Subjects) != 3 || len(accepted.Setup.Relations) != 2 ||
		accepted.Expected.Subjects[1].Handle != "subject/target" ||
		accepted.Expected.Subjects[1].Fields[0].ValueSubject != "subject/value" {
		t.Fatalf("distinct target/value Fact = %#v", accepted)
	}
	valueUnavailable := acceptanceFactByID(t, facts, factID(domain, "changes", "value-unavailable", "from", "Pending"))
	relationConditions := map[SemanticID]string{}
	if valueUnavailable.Setup != nil {
		for _, relation := range valueUnavailable.Setup.Relations {
			relationConditions[relation.Field] = relation.Condition
		}
	}
	if valueUnavailable.Setup == nil || len(valueUnavailable.Setup.Relations) != 2 ||
		relationConditions["entity/Transfer/field/target"] != "resolved" ||
		relationConditions["entity/Transfer/field/source"] != "value-unavailable" ||
		len(valueUnavailable.Expected.Subjects) != 2 || valueUnavailable.Expected.Subjects[1].Handle != "subject/target" ||
		!valueUnavailable.Expected.Subjects[0].Unchanged || !valueUnavailable.Expected.Subjects[1].Unchanged {
		t.Fatalf("distinct value-unavailable Fact = %#v", valueUnavailable)
	}
	targetUnavailable := acceptanceFactByID(t, facts, factID(domain, "changes", "target-unavailable", "from", "Pending"))
	if targetUnavailable.Setup == nil || len(targetUnavailable.Setup.Relations) != 1 ||
		targetUnavailable.Setup.Relations[0].Condition != "target-unavailable" {
		t.Fatalf("target-unavailable precedence Fact = %#v", targetUnavailable)
	}
}

func TestCloneIRExpressionDoesNotAliasRelationPath(t *testing.T) {
	original := IRExpression{RelationPath: []SemanticID{"entity/A/field/relation"}}
	cloned := cloneIRExpression(original)
	cloned.RelationPath[0] = "entity/B/field/relation"
	if original.RelationPath[0] != "entity/A/field/relation" {
		t.Fatalf("clone shares relationPath backing storage: %v", original.RelationPath)
	}
}

func TestChangesRelationValueAddsDisclosureReviewRequirement(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("relation-value-review.forma", relationValueAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	requirements, err := BuildReviewRequirements(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	id := SemanticID("review/action/InventorySnapshot/capture/cross-entity-value-read-authorization")
	var got *ReviewRequirement
	for index := range requirements.Requirements {
		if requirements.Requirements[index].ID == id {
			got = &requirements.Requirements[index]
		}
	}
	if got == nil || got.Kind != "cross-entity-value-read-authorization" {
		t.Fatalf("missing relation value disclosure requirement: %#v", requirements.Requirements)
	}
	for _, source := range []SemanticID{
		"entity/InventorySnapshot/field/observedOnHand",
		"entity/InventorySnapshot/field/stock",
		"entity/StockItem/field/onHand",
		"page/StockItems/view/list/StockItem",
		"page/Snapshots/view/list/InventorySnapshot",
		"page/Snapshots/view/list/InventorySnapshot/action/capture",
	} {
		if !containsSemanticID(got.SourceNodes, source) {
			t.Errorf("disclosure requirement omits %s: %v", source, got.SourceNodes)
		}
	}
	requirements.Requirements = slices.DeleteFunc(requirements.Requirements, func(requirement ReviewRequirement) bool {
		return requirement.ID == id
	})
	if err := ValidateReviewRequirements(result.Intent, requirements); err == nil || !strings.Contains(err.Error(), "differs from canonical") {
		t.Fatalf("missing disclosure review requirement error = %v", err)
	}
}

func TestChangesReviewRequirementsKeepAtomicityAndAuthorizationSeparate(t *testing.T) {
	result := compileChangesFixture(t)
	requirements, err := BuildReviewRequirements(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	want := []SemanticID{
		"review/action/StockReservation/commit/atomic-changes-enforcement",
		"review/action/StockReservation/commit/cross-entity-write-authorization",
		"review/entity/StockItem/invariant/stockAvailable/concurrent-invariant-enforcement",
	}
	if got := ReviewRequirementIDs(requirements); !reflect.DeepEqual(got, want) {
		t.Fatalf("Changes review requirements = %v, want %v", got, want)
	}
	for _, requirement := range requirements.Requirements {
		if !containsSemanticID(requirement.SourceNodes, requirement.Subject) {
			t.Errorf("requirement %s omits subject provenance", requirement.ID)
		}
	}
	cross := requirements.Requirements[1]
	for _, id := range []SemanticID{
		"entity/StockItem/field/reserved",
		"page/Reservations/view/list/StockReservation/action/commit",
		"page/StockItemEdit/view/form/edit/StockItem/submit",
	} {
		if !containsSemanticID(cross.SourceNodes, id) {
			t.Errorf("cross-entity authorization review omits %s: %v", id, cross.SourceNodes)
		}
	}
}

func TestChangesDiagnosticsCloseTheFirstSlice(t *testing.T) {
	base := func(target, value string) string {
		return `type Quantity = Int min 0
entity StockItem {
    reserved Quantity required
}
entity StockReservation {
    stock StockItem required
    reservedAfter Quantity required
    optionalAfter Quantity
    state status Pending | Committed initial Pending
}
action StockReservation.commit: Pending -> Committed {
    changes {
        ` + target + ` = ` + value + `
    }
}
`
	}
	tests := []struct {
		name, source, code string
	}{
		{name: "optional relation", source: strings.Replace(base("stock.reserved", "reservedAfter"), "stock StockItem required", "stock StockItem", 1), code: "F2803"},
		{name: "optional value relation", source: strings.Replace(base("reservedAfter", "stock.reserved"), "stock StockItem required", "stock StockItem", 1), code: "F2805"},
		{name: "to-many value relation", source: strings.Replace(base("reservedAfter", "stock.reserved"), "stock StockItem required", "stock [StockItem]", 1), code: "F2805"},
		{name: "multi-hop value", source: base("reservedAfter", "stock.owner.limit"), code: "F2805"},
		{name: "state value", source: base("stock.reserved", "status"), code: "F2805"},
		{name: "relation terminal value", source: base("reservedAfter", "stock"), code: "F2805"},
		{name: "optional value", source: base("stock.reserved", "optionalAfter"), code: "F2805"},
		{name: "state target", source: base("status", "reservedAfter"), code: "F2804"},
		{name: "empty changes", source: strings.Replace(base("stock.reserved", "reservedAfter"), "        stock.reserved = reservedAfter\n", "", 1), code: "F2802"},
		{name: "two assignments", source: strings.Replace(base("stock.reserved", "reservedAfter"), "        stock.reserved = reservedAfter\n", "        stock.reserved = reservedAfter\n        stock.reserved = reservedAfter\n", 1), code: "F2802"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Compile([]SourceFile{NewSourceFile("changes-invalid.forma", test.source)})
			if !slices.Contains(diagnosticCodes(result.Diagnostics), test.code) {
				t.Fatalf("missing diagnostic %s:\n%s", test.code, diagnosticMessages(result.Diagnostics))
			}
		})
	}
}

func compileChangesFixture(t *testing.T) Result {
	t.Helper()
	result := Compile([]SourceFile{NewSourceFile("changes.forma", changesAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	return result
}

func actionByID(t *testing.T, intent *ResolvedIntent, id SemanticID) IRAction {
	t.Helper()
	for _, action := range intent.Actions {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("missing action %s", id)
	return IRAction{}
}

func actionRefByID(t *testing.T, intent *ResolvedIntent, id SemanticID) IRActionRef {
	t.Helper()
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			for _, ref := range view.Actions {
				if ref.ID == id {
					return ref
				}
			}
		}
	}
	t.Fatalf("missing action reference %s", id)
	return IRActionRef{}
}

func irFieldPointerByID(t *testing.T, intent *ResolvedIntent, id SemanticID) *IRField {
	t.Helper()
	for entityIndex := range intent.Entities {
		for fieldIndex := range intent.Entities[entityIndex].Fields {
			field := &intent.Entities[entityIndex].Fields[fieldIndex]
			if field.ID == id {
				return field
			}
		}
	}
	t.Fatalf("missing field %s", id)
	return nil
}

const changesAcceptanceSource = `role admin
role staff

type Quantity = Int min 0

entity StockItem {
    sku      String required label
    onHand   Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
}

entity StockReservation {
    stock         StockItem required
    reservedAfter Quantity required
    state status Pending | Committed initial Pending
}

action StockReservation.commit: Pending -> Committed confirm allow staff {
    changes {
        stock.reserved = reservedAfter
    }
}

page Reservations {
    allow staff
    list StockReservation {
        columns stock, reservedAfter, status
        actions commit
    }
}

page StockItems {
    allow admin, staff
    list StockItem {
        columns sku, onHand, reserved
        actions edit
    }
}

page StockItemEdit(item StockItem) {
    allow admin
    form item {
        fields sku, onHand, reserved
        submit edit
    }
}
`

const relationValueAcceptanceSource = `role admin
role staff

type Quantity = Int min 0

entity StockItem {
    code   String required label
    onHand Quantity required
}

entity InventorySnapshot {
    stock          StockItem required
    observedOnHand Quantity required default 0
    state status Pending | Captured initial Pending
}

action InventorySnapshot.capture: Pending -> Captured confirm allow staff {
    changes {
        observedOnHand = stock.onHand
    }
}

page StockItems {
    allow admin
    list StockItem {
        columns code, onHand
    }
}

page Snapshots {
    allow staff
    list InventorySnapshot {
        columns stock, observedOnHand, status
        actions capture
    }
}
`

const distinctRelationValueSource = `type Quantity = Int min 0

entity TargetValue {
    amount Quantity required
}

entity SourceValue {
    amount Quantity required
}

entity Transfer {
    target TargetValue required
    source SourceValue required
    state status Pending | Applied initial Pending
}

action Transfer.apply: Pending -> Applied {
    changes {
        target.amount = source.amount
    }
}
`
