package compiler

import (
	"bytes"
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
