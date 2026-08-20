package compiler

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestBuildAcceptanceFactsForAdminFlow(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("admin.forma", adminAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Version != AcceptanceFactsVersion || facts.IntentVersion != ResolvedIntentVersion {
		t.Fatalf("fact versions = %#v", facts)
	}
	wanted := map[SemanticID]string{
		"fact/page/Users/view/list/User/access/allowed/admin":                                                             "access-allowed",
		"fact/page/Users/view/list/User/access/denied/anonymous":                                                          "access-denied",
		"fact/page/Users/view/list/User/records/visible":                                                                  "records-visible",
		"fact/page/Users/view/list/User/search":                                                                           "list-search",
		"fact/page/Users/view/list/User/filter/team":                                                                      "list-filter",
		"fact/page/Users/view/list/User/filter/plan":                                                                      "list-filter",
		"fact/page/Users/view/list/User/sort":                                                                             "list-sort",
		"fact/page/Users/view/list/User/page-boundary":                                                                    "list-page-boundary",
		"fact/page/UserEdit/view/form/edit/User/submit/mutation/accepted":                                                 "mutation-accepted",
		"fact/page/UserEdit/view/form/edit/User/submit/mutation/at-most-once":                                             "mutation-at-most-once",
		"fact/page/UserEdit/view/form/edit/User/submit/validation/required/email":                                         "validation-rejected",
		"fact/page/UserEdit/view/form/edit/User/submit/validation/unique/email":                                           "validation-rejected",
		"fact/page/UserEdit/view/form/edit/User/submit/validation/matches/email/constraint/type/Email/constraint/matches": "validation-rejected",
		"fact/page/UserEdit/view/form/edit/User/submit/validation/closed-set/plan":                                        "validation-rejected",
		"fact/page/UserEdit/view/form/edit/User/submit/navigation":                                                        "navigation",
		"fact/entity/User/field/team/relation/resolved":                                                                   "relation-resolved",
	}
	seen := map[SemanticID]bool{}
	for index, fact := range facts.Facts {
		if index > 0 && facts.Facts[index-1].ID >= fact.ID {
			t.Fatalf("facts are not in stable ID order: %s then %s", facts.Facts[index-1].ID, fact.ID)
		}
		if seen[fact.ID] {
			t.Fatalf("duplicate fact ID %s", fact.ID)
		}
		seen[fact.ID] = true
		if kind, ok := wanted[fact.ID]; ok {
			if fact.Kind != kind {
				t.Errorf("fact %s kind = %s, want %s", fact.ID, fact.Kind, kind)
			}
			delete(wanted, fact.ID)
		}
		if len(fact.SourceNodes) == 0 {
			t.Errorf("fact %s has no source nodes", fact.ID)
		}
	}
	for id, kind := range wanted {
		t.Errorf("missing %s fact %s", kind, id)
	}
}

func TestAcceptanceFactsExcludeTargetVocabulary(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("admin.forma", adminAcceptanceSource)})
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := MarshalAcceptanceFacts(facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"route"`, `"http"`, `"html"`, `"dom"`, `"selector"`, `"statusCode"`,
		`"component"`, `"submissionToken"`, `"framework"`,
	} {
		if bytes.Contains(bytes.ToLower(encoded), bytes.ToLower([]byte(forbidden))) {
			t.Errorf("Acceptance Facts contain target vocabulary %s", forbidden)
		}
	}
}

func TestAcceptanceFactsPreserveAllOfAnyOfAccess(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("access.forma", `role admin
role editor
role member
entity User {
    name String required label
}
page Users {
    allow admin, editor
    list User {
        columns name
        actions edit
    }
}
page UserEdit(user User) {
    allow member
    form user {
        fields name
        submit edit
    }
}
`)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	want := map[SemanticID]bool{
		"fact/page/Users/view/list/User/action/edit/access/allowed/admin+member":  false,
		"fact/page/Users/view/list/User/action/edit/access/allowed/editor+member": false,
		"fact/page/Users/view/list/User/action/edit/access/denied/anonymous":      false,
		"fact/page/Users/view/list/User/action/edit/access/denied/admin":          false,
		"fact/page/Users/view/list/User/action/edit/access/denied/editor":         false,
		"fact/page/Users/view/list/User/action/edit/access/denied/member":         false,
	}
	for _, fact := range facts.Facts {
		if _, ok := want[fact.ID]; ok {
			want[fact.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("missing access fact %s", id)
		}
	}
}

func TestInvariantAcceptanceFactsPinPostStateAndAtomicRejection(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Facts) != 2 {
		t.Fatalf("invariant facts = %d, want 2: %#v", len(facts.Facts), facts.Facts)
	}
	invariant := result.Intent.Entities[0].Invariants[0]
	want := map[SemanticID]struct {
		kind      string
		result    bool
		outcome   string
		atomicity string
	}{
		factID(invariant.ID, "evaluation", "satisfied"): {
			kind: "invariant-satisfied", result: true, outcome: "accepted", atomicity: "all-changes-committed",
		},
		factID(invariant.ID, "evaluation", "violated"): {
			kind: "invariant-violated", result: false, outcome: "rejected", atomicity: "no-changes-committed",
		},
	}
	for _, fact := range facts.Facts {
		expected, ok := want[fact.ID]
		if !ok {
			t.Fatalf("unexpected invariant fact %s", fact.ID)
		}
		if fact.Kind != expected.kind || fact.Subject != invariant.ID || fact.Input == nil || fact.Input.Predicate == nil {
			t.Fatalf("invariant fact shape = %#v", fact)
		}
		predicate := fact.Input.Predicate
		if predicate.Evaluation != "post-state" || predicate.OtherRequirements != "satisfied" || predicate.Result != expected.result || !reflect.DeepEqual(predicate.Expression, invariant.Predicate) {
			t.Fatalf("predicate input = %#v, want resolved post-state result %t", predicate, expected.result)
		}
		if fact.Expected.Outcome != expected.outcome || fact.Expected.Enforcement != "authoritative" || fact.Expected.Atomicity != expected.atomicity {
			t.Fatalf("invariant expectation = %#v", fact.Expected)
		}
		if !reflect.DeepEqual(fact.SourceNodes, invariantFactSourceNodes(invariant)) {
			t.Fatalf("invariant provenance = %v, want %v", fact.SourceNodes, invariantFactSourceNodes(invariant))
		}
	}
}

func TestInvariantAcceptanceFactValidationRejectsSemanticDrift(t *testing.T) {
	build := func(t *testing.T) (*ResolvedIntent, *AcceptanceFacts) {
		t.Helper()
		result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantAcceptanceSource)})
		if len(result.Diagnostics) != 0 {
			t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
		}
		facts, err := BuildAcceptanceFacts(result.Intent)
		if err != nil {
			t.Fatal(err)
		}
		return result.Intent, facts
	}
	tests := []struct {
		name   string
		mutate func(*AcceptanceFacts)
		want   string
	}{
		{
			name: "result without matching identity",
			mutate: func(facts *AcceptanceFacts) {
				facts.Facts[0].Input.Predicate.Result = !facts.Facts[0].Input.Predicate.Result
			},
			want: "non-canonical identity or kind",
		},
		{
			name: "predicate tree",
			mutate: func(facts *AcceptanceFacts) {
				facts.Facts[0].Input.Predicate.Expression.Operator = "greater-than"
			},
			want: "differs from its resolved post-state predicate",
		},
		{
			name: "atomicity",
			mutate: func(facts *AcceptanceFacts) {
				facts.Facts[0].Expected.Atomicity = "best-effort"
			},
			want: "non-canonical expectation",
		},
		{
			name: "extra input",
			mutate: func(facts *AcceptanceFacts) {
				facts.Facts[0].Input.Dispatches = 1
			},
			want: "non-canonical predicate input",
		},
		{
			name: "provenance",
			mutate: func(facts *AcceptanceFacts) {
				facts.Facts[0].SourceNodes = []SemanticID{facts.Facts[0].Subject}
			},
			want: "incomplete predicate provenance",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent, facts := build(t)
			test.mutate(facts)
			err := ValidateAcceptanceFacts(intent, facts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInvariantAcceptanceFactValidationRequiresBothEvaluationCases(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	facts.Facts = facts.Facts[:1]
	if err := ValidateAcceptanceFacts(result.Intent, facts); err == nil || !strings.Contains(err.Error(), "missing invariant evaluation fact") {
		t.Fatalf("missing invariant evaluation error = %v", err)
	}
}

func TestInvariantAcceptanceFactsConnectAuthoritativeRejectionToMutation(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantMutationAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	entity := result.Intent.Entities[0]
	invariant := entity.Invariants[0]
	var view IRView
	for _, page := range result.Intent.Pages {
		for _, candidate := range page.Views {
			if candidate.Submit != nil {
				view = candidate
			}
		}
	}
	if view.Submit == nil {
		t.Fatal("compiled fixture has no form submit")
	}
	id := invariantValidationFactID(view.Submit.ID, invariant.ID)
	fact := acceptanceFactByID(t, facts, id)
	wantInputFields := []SemanticID{
		"entity/StockItem/field/onHand",
		"entity/StockItem/field/reserved",
	}
	if fact.Kind != "invariant-validation-rejected" || fact.Subject != view.Submit.ID || fact.Input == nil ||
		fact.Input.Predicate == nil || fact.Input.Predicate.Result ||
		!reflect.DeepEqual(fact.Input.Fields, wantInputFields) ||
		!reflect.DeepEqual(fact.Input.Predicate.Expression, invariant.Predicate) {
		t.Fatalf("mutation invariant input = %#v", fact)
	}
	wantExpected := FactExpectation{
		Outcome: "rejected", Feedback: []string{"invalid"}, Enforcement: "authoritative",
		Atomicity: "no-changes-committed", Stored: "unchanged", PreserveInput: wantInputFields,
	}
	if !reflect.DeepEqual(fact.Expected, wantExpected) {
		t.Fatalf("mutation invariant expectation = %#v, want %#v", fact.Expected, wantExpected)
	}
	for _, required := range []SemanticID{view.ID, view.Submit.ID, invariant.ID,
		"entity/StockItem/field/onHand", "entity/StockItem/field/reserved"} {
		if !containsSemanticID(fact.SourceNodes, required) {
			t.Errorf("mutation invariant provenance omits %s: %v", required, fact.SourceNodes)
		}
	}
}

func TestInvariantMutationFactsCoverSupportedFormShapes(t *testing.T) {
	onHand := SemanticID("entity/StockItem/field/onHand")
	reserved := SemanticID("entity/StockItem/field/reserved")
	note := SemanticID("entity/StockItem/field/note")
	tests := []struct {
		name             string
		pages            string
		wantSubject      SemanticID
		wantInputFields  []SemanticID
		wantPreserve     []SemanticID
		wantMutationFact bool
	}{
		{
			name: "create form",
			pages: `page StockItems {
    list StockItem {
        columns note, onHand, reserved
        actions create
    }
}
page StockItemCreate {
    form StockItem {
        fields note, onHand, reserved
        submit create
    }
}
`,
			wantSubject:      "page/StockItemCreate/view/form/create/StockItem/submit",
			wantInputFields:  []SemanticID{onHand, reserved},
			wantPreserve:     []SemanticID{note, onHand, reserved},
			wantMutationFact: true,
		},
		{
			name: "partial edit reads the other predicate field from post-state",
			pages: `page StockItems {
    list StockItem {
        columns note, onHand, reserved
        actions edit
    }
}
page StockItemEdit(item StockItem) {
    form item {
        fields reserved
        submit edit
    }
}
`,
			wantSubject:      "page/StockItemEdit/view/form/edit/StockItem/submit",
			wantInputFields:  []SemanticID{reserved},
			wantPreserve:     []SemanticID{reserved},
			wantMutationFact: true,
		},
		{
			name: "unrelated edit",
			pages: `page StockItems {
    list StockItem {
        columns note, onHand, reserved
        actions edit
    }
}
page StockItemEdit(item StockItem) {
    form item {
        fields note
        submit edit
    }
}
`,
			wantMutationFact: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := `type Quantity = Int min 0
entity StockItem {
    onHand   Quantity required
    reserved Quantity required
    note     String required label
    invariant stockAvailable: reserved <= onHand
}
` + test.pages
			result := Compile([]SourceFile{NewSourceFile("stock.forma", source)})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
			}
			facts, err := BuildAcceptanceFacts(result.Intent)
			if err != nil {
				t.Fatal(err)
			}
			var mutationFacts []AcceptanceFact
			for _, fact := range facts.Facts {
				if fact.Kind == "invariant-validation-rejected" {
					mutationFacts = append(mutationFacts, fact)
				}
			}
			wantCount := 0
			if test.wantMutationFact {
				wantCount = 1
			}
			if len(mutationFacts) != wantCount {
				t.Fatalf("invariant mutation facts = %#v, want %d", mutationFacts, wantCount)
			}
			if !test.wantMutationFact {
				return
			}

			fact := mutationFacts[0]
			invariant := result.Intent.Entities[0].Invariants[0]
			if fact.Subject != test.wantSubject || fact.Input == nil || fact.Input.Predicate == nil ||
				!reflect.DeepEqual(fact.Input.Fields, test.wantInputFields) ||
				!reflect.DeepEqual(fact.Expected.PreserveInput, test.wantPreserve) {
				t.Fatalf("invariant mutation form shape = %#v", fact)
			}
			if fact.Input.Predicate.Evaluation != "post-state" || fact.Input.Predicate.Result ||
				!reflect.DeepEqual(fact.Input.Predicate.Expression, invariant.Predicate) {
				t.Fatalf("invariant mutation predicate = %#v, want full post-state predicate %#v", fact.Input.Predicate, invariant.Predicate)
			}
			for _, predicateField := range []SemanticID{onHand, reserved} {
				if !containsSemanticID(fact.SourceNodes, predicateField) {
					t.Errorf("mutation fact provenance omits predicate field %s: %v", predicateField, fact.SourceNodes)
				}
			}
		})
	}
}

func TestInvariantAcceptanceFactValidationRequiresAndPinsMutationEnforcement(t *testing.T) {
	build := func(t *testing.T) (*ResolvedIntent, *AcceptanceFacts, SemanticID) {
		t.Helper()
		result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantMutationAcceptanceSource)})
		if len(result.Diagnostics) != 0 {
			t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
		}
		facts, err := BuildAcceptanceFacts(result.Intent)
		if err != nil {
			t.Fatal(err)
		}
		invariant := result.Intent.Entities[0].Invariants[0]
		for _, page := range result.Intent.Pages {
			for _, view := range page.Views {
				if view.Submit != nil {
					return result.Intent, facts, invariantValidationFactID(view.Submit.ID, invariant.ID)
				}
			}
		}
		t.Fatal("compiled fixture has no form submit")
		return nil, nil, ""
	}

	t.Run("missing", func(t *testing.T) {
		intent, facts, id := build(t)
		filtered := facts.Facts[:0]
		for _, fact := range facts.Facts {
			if fact.ID != id {
				filtered = append(filtered, fact)
			}
		}
		facts.Facts = filtered
		if err := ValidateAcceptanceFacts(intent, facts); err == nil || !strings.Contains(err.Error(), "missing invariant mutation fact") {
			t.Fatalf("missing invariant mutation error = %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*AcceptanceFact)
		want   string
	}{
		{name: "input fields", mutate: func(fact *AcceptanceFact) { fact.Input.Fields = fact.Input.Fields[:1] }, want: "non-canonical input or subject"},
		{name: "enforcement", mutate: func(fact *AcceptanceFact) { fact.Expected.Enforcement = "user-interface" }, want: "non-canonical expectation"},
		{name: "atomicity", mutate: func(fact *AcceptanceFact) { fact.Expected.Atomicity = "best-effort" }, want: "non-canonical expectation"},
		{name: "feedback", mutate: func(fact *AcceptanceFact) { fact.Expected.Feedback = nil }, want: "non-canonical expectation"},
		{name: "provenance", mutate: func(fact *AcceptanceFact) { fact.SourceNodes = canonicalSemanticIDs([]SemanticID{fact.Subject}) }, want: "incomplete mutation provenance"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent, facts, id := build(t)
			fact, ok := acceptanceFactPointerByID(facts, id)
			if !ok {
				t.Fatalf("missing mutation invariant fact %s", id)
			}
			test.mutate(fact)
			if err := ValidateAcceptanceFacts(intent, facts); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func acceptanceFactPointerByID(facts *AcceptanceFacts, id SemanticID) (*AcceptanceFact, bool) {
	for index := range facts.Facts {
		if facts.Facts[index].ID == id {
			return &facts.Facts[index], true
		}
	}
	return nil, false
}

func TestInvariantFactExpressionDoesNotAliasResolvedIntent(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("stock.forma", invariantAcceptanceSource)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	wantField := result.Intent.Entities[0].Invariants[0].Predicate.Left.Field
	facts.Facts[0].Input.Predicate.Expression.Left.Field = "entity/Other/field/reserved"
	if got := result.Intent.Entities[0].Invariants[0].Predicate.Left.Field; got != wantField {
		t.Fatalf("mutating a Fact changed Resolved Intent field %s to %s", wantField, got)
	}
	if err := ValidateAcceptanceFacts(result.Intent, facts); err == nil {
		t.Fatal("tampered Fact expression passed validation")
	}
}

const invariantAcceptanceSource = `type Quantity = Int min 0
entity StockItem {
    onHand   Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
}
`

const invariantMutationAcceptanceSource = invariantAcceptanceSource + `
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

const adminAcceptanceSource = `role admin

type Email = String matches /.+@.+/
type Plan = Free | Pro | Enterprise

entity Team {
    name String required label
}

entity User {
    name  String required label
    email Email required unique
    team  Team
    plan  Plan required
}

page Users {
    allow admin
    list User {
        columns name, email, team, plan
        search name, email
        filter team, plan
        sort email asc
        paginate 2
        actions view, edit
    }
}

page UserDetail(user User) {
    allow admin
    detail user {
        fields name, email, team, plan
        actions edit
    }
}

page UserEdit(user User) {
    allow admin
    form user {
        fields name, email, team, plan
        submit edit
    }
}
`
