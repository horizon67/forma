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
			want: "not a canonical self field reference",
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
		{name: "relation value", source: base("stock.reserved", "stock.reserved"), code: "F2805"},
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
