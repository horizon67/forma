package conformance

import (
	"bytes"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

const conformanceSource = `role admin

type Plan = Free | Pro | Enterprise

entity User {
    name  String required label
    email String required
    plan  Plan required
}

page Users {
    allow admin
    list User {
        columns name, email, plan
        actions view, edit
    }
}

page UserDetail(user User) {
    allow admin
    detail user {
        fields name, email, plan
        actions edit
    }
}

page UserEdit(user User) {
    allow admin
    form user {
        fields name, email, plan
        submit edit
    }
}
`

func TestBuildProducesTargetNeutralCases(t *testing.T) {
	contract := buildContract(t, "first.forma", conformanceSource)
	if contract.Schema != Schema || contract.IntentVersion != compiler.ResolvedIntentVersion {
		t.Fatalf("contract header = %#v", contract)
	}
	if len(contract.Cases) != 7 {
		t.Fatalf("cases = %d, want 7: %#v", len(contract.Cases), contract.Cases)
	}
	wantCases := map[string]bool{
		"page/Users/view/list/User/case/access-allowed/satisfying-principal": false,
		"page/Users/view/list/User/case/access-denied/no-roles":              false,
		"page/UserEdit/view/form/edit/User/case/accepted/valid-input":        false,
		"page/UserEdit/view/form/edit/User/case/required/name":               false,
		"page/UserEdit/view/form/edit/User/case/required/email":              false,
		"page/UserEdit/view/form/edit/User/case/required/plan":               false,
		"page/UserEdit/view/form/edit/User/case/closed-set/plan":             false,
	}
	for _, testCase := range contract.Cases {
		if _, ok := wantCases[testCase.ID]; !ok {
			t.Errorf("unexpected case %q", testCase.ID)
			continue
		}
		wantCases[testCase.ID] = true
		if testCase.Operation.Principal.Kind != "anonymous" && testCase.Operation.Principal.Kind != "roles" {
			t.Errorf("principal = %#v", testCase.Operation.Principal)
		}
		switch testCase.Expect.Outcome {
		case "access-allowed":
			if testCase.Operation.Principal.Kind != "roles" || len(testCase.Expect.Subjects) != 2 {
				t.Errorf("allowed query = %#v", testCase)
			}
		case "access-denied":
			if testCase.Operation.Principal.Kind != "anonymous" || len(testCase.Operation.Principal.Roles) != 0 {
				t.Errorf("denied query principal = %#v", testCase.Operation.Principal)
			}
		case "mutation-accepted":
			if testCase.Expect.Stored != "input" || len(testCase.Operation.Input) != 3 {
				t.Errorf("accepted mutation = %#v", testCase)
			}
		case "validation-rejected":
			if testCase.Expect.Outcome != "validation-rejected" || testCase.Expect.Stored != "unchanged" {
				t.Errorf("mutation expectation = %#v", testCase.Expect)
			}
			wantPreserved := 3
			if testCase.Expect.Violation != nil && testCase.Expect.Violation.Kind == "closed-set" {
				wantPreserved = 2
			}
			if len(testCase.Expect.PreserveInput) != wantPreserved {
				t.Errorf("preserved inputs = %#v, want %d", testCase.Expect.PreserveInput, wantPreserved)
			}
		default:
			t.Errorf("unknown expectation = %#v", testCase.Expect)
		}
	}
	for id, seen := range wantCases {
		if !seen {
			t.Errorf("missing case %q", id)
		}
	}

	if len(contract.Fixture.Entities) != 1 || len(contract.Fixture.Entities[0].Records) != 2 {
		t.Fatalf("fixture = %#v", contract.Fixture)
	}
	values := valueIndex(contract.Fixture.Entities[0].Records[0].Values)
	if values["entity/User/field/plan"] != "Free" {
		t.Fatalf("fixture plan = %q", values["entity/User/field/plan"])
	}
	secondValues := valueIndex(contract.Fixture.Entities[0].Records[1].Values)
	if secondValues["entity/User/field/plan"] != "Pro" {
		t.Fatalf("second fixture plan = %q", secondValues["entity/User/field/plan"])
	}
}

func TestContractIsByteIdenticalAcrossPathAndLayoutChanges(t *testing.T) {
	first := buildContract(t, "one/first.forma", conformanceSource)
	second := buildContract(t, "moved/second.forma", "// layout only\n\n"+conformanceSource+"\n")
	firstJSON, err := Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("contracts differ:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestContractSerializationHasNoTargetVocabulary(t *testing.T) {
	content, err := Marshal(buildContract(t, "target-neutral.forma", conformanceSource))
	if err != nil {
		t.Fatal(err)
	}
	lowered := strings.ToLower(string(content))
	for _, forbidden := range []string{
		"http", "https", "url", "route", "path", "cookie", "header", "status", "html",
		"<input", "<select", "get ", "post ", "403", "422", "303", "_forma_submission",
	} {
		if strings.Contains(lowered, forbidden) {
			t.Errorf("contract leaks target vocabulary %q", forbidden)
		}
	}
}

func buildContract(t *testing.T, path, source string) Contract {
	t.Helper()
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile(path, source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	contract, err := Build(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func valueIndex(values []Value) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[string(value.Field)] = value.Value
	}
	return result
}

func TestNormalizeProducesStableFixtureIdentity(t *testing.T) {
	if got := fixtureIdentity("Order_Line"); got != "conformance-order-line-1" {
		t.Fatalf("fixture identity = %q", got)
	}
	if strings.Contains(fixtureIdentity("User"), "/") {
		t.Fatal("fixture identity contains a path separator")
	}
}

func TestUnrestrictedAccessChoosesAnExplicitAnonymousPrincipal(t *testing.T) {
	principal, err := satisfyingPrincipal(nil, compiler.IRAccess{})
	if err != nil {
		t.Fatal(err)
	}
	if principal.Kind != "anonymous" || len(principal.Roles) != 0 {
		t.Fatalf("principal = %#v", principal)
	}
}
