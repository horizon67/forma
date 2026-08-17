package compiler

import (
	"strings"
	"testing"
)

// twoSurfaceSource builds a program where an admin surface and a member-owned
// surface both declare a detail and an edit form for User. The `actions` and
// `submit` bodies are supplied so each test can vary only the destinations.
func twoSurfaceSource(listActions, detailActions, adminSubmit, memberSubmit string) string {
	return `role admin

entity User {
    name String required label
}

action User.archive: Active -> Archived

page Users {
    allow admin

    list User {
        columns name
        actions ` + listActions + `
    }
}

page UserDetail(user User) {
    allow admin

    detail user {
        fields name
        actions ` + detailActions + `
    }
}

page UserEdit(user User) {
    allow admin

    form user {
        fields name
        submit ` + adminSubmit + `
    }
}

page Profile(user User) {
    allow member

    detail user {
        fields name
    }
}

page ProfileEdit(user User) {
    allow member

    form user {
        fields name
        submit ` + memberSubmit + `
    }
}
`
}

func twoSurfaceProgram(listActions, detailActions, adminSubmit, memberSubmit string) string {
	source := twoSurfaceSource(listActions, detailActions, adminSubmit, memberSubmit)
	source = strings.Replace(source, "role admin\n", "role admin\nrole member\n", 1)
	source = strings.Replace(source, "action User.archive: Active -> Archived\n", "", 1)
	return source
}

func TestAmbiguousDestinationRequiresExplicitGoto(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("ambiguous.forma",
		twoSurfaceProgram("view, edit", "edit", "edit", "edit"))})
	messages := diagnosticMessages(result.Diagnostics)
	for _, want := range []string{
		"resolves to 2 detail destinations",
		"resolves to 2 edit form destinations",
	} {
		if !strings.Contains(messages, want) {
			t.Errorf("diagnostics missing %q:\n%s", want, messages)
		}
	}
}

func TestExplicitGotoResolvesBothSurfaces(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("explicit.forma", twoSurfaceProgram(
		"view goto UserDetail, edit goto UserEdit",
		"edit goto UserEdit",
		"edit goto UserDetail",
		"edit goto Profile"))})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("explicit destinations should resolve:\n%s", diagnosticMessages(result.Diagnostics))
	}

	targets := map[string]string{}
	successes := map[string]string{}
	for _, page := range result.Intent.Pages {
		for _, view := range page.Views {
			for _, action := range view.Actions {
				targets[string(page.ID)+"/"+action.Name] = action.TargetPage
				if action.SuccessPage != "" {
					t.Errorf("standard action %s must not carry a success page", action.ID)
				}
			}
			if view.Submit != nil {
				successes[string(page.ID)] = view.Submit.Success.Page
			}
		}
	}
	for id, want := range map[string]string{
		"page/Users/view":      "UserDetail",
		"page/Users/edit":      "UserEdit",
		"page/UserDetail/edit": "UserEdit",
	} {
		if targets[id] != want {
			t.Errorf("target %s = %q, want %q", id, targets[id], want)
		}
	}
	for id, want := range map[string]string{
		"page/UserEdit":    "UserDetail",
		"page/ProfileEdit": "Profile",
	} {
		if successes[id] != want {
			t.Errorf("submit success %s = %q, want %q", id, successes[id], want)
		}
	}
}

func TestGotoMustNameACandidate(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("wrong.forma", twoSurfaceProgram(
		"view goto UserEdit, edit goto UserEdit",
		"edit goto UserEdit",
		"edit goto UserDetail",
		"edit goto Profile"))})
	messages := diagnosticMessages(result.Diagnostics)
	if !strings.Contains(messages, "`goto UserEdit` is not a detail for `User`") {
		t.Fatalf("naming a non-candidate must fail:\n%s", messages)
	}
}

func TestDomainActionReferenceRejectsInlineGoto(t *testing.T) {
	source := `role admin

entity User {
    name String required label

    state status Active | Archived initial Active
}

action User.archive: Active -> Archived

page Users {
    allow admin

    list User {
        columns name
        actions archive goto UserDetail
    }
}

page UserDetail(user User) {
    allow admin

    detail user {
        fields name
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("domain.forma", source)})
	messages := diagnosticMessages(result.Diagnostics)
	if !strings.Contains(messages, "domain action `archive` cannot name a destination at the reference") {
		t.Fatalf("inline goto on a domain action must fail:\n%s", messages)
	}
	// The help text carries the remedy; assert it on the diagnostic itself.
	found := false
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Hint, "declare `goto` on `action User.archive` instead") {
			found = true
		}
	}
	if !found {
		t.Error("diagnostic help must point at the top-level declaration")
	}
}

func TestExplicitGotoAllowedWhenDestinationIsUnique(t *testing.T) {
	// Naming the only candidate stays legal so that adding or removing a second
	// surface does not force unrelated source churn.
	source := `role admin

entity User {
    name String required label
}

page Users {
    allow admin

    list User {
        columns name
        actions view goto UserDetail, edit goto UserEdit
    }
}

page UserDetail(user User) {
    allow admin

    detail user {
        fields name
    }
}

page UserEdit(user User) {
    allow admin

    form user {
        fields name
        submit edit goto UserDetail
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("unique.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("naming a unique destination must stay legal:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestEveryResolutionPointHonoursExplicitGoto(t *testing.T) {
	// create target, view target, edit target, delete success, and submit success.
	source := `role admin
role member

entity User {
    name String required label
}

page Users {
    allow admin

    list User {
        columns name
        actions create goto UserCreate, view goto UserDetail, edit goto UserEdit
    }
}

page Members {
    allow member

    list User {
        columns name
    }
}

page MemberInvite {
    allow member

    form User {
        fields name
        submit create goto Profile
    }
}

page UserCreate {
    allow admin

    form User {
        fields name
        submit create goto UserDetail
    }
}

page UserDetail(user User) {
    allow admin

    detail user {
        fields name
        actions delete goto Users
    }
}

page UserEdit(user User) {
    allow admin

    form user {
        fields name
        submit edit goto UserDetail
    }
}

page Profile(user User) {
    allow member

    detail user {
        fields name
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("points.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("all five resolution points should accept `goto`:\n%s", diagnosticMessages(result.Diagnostics))
	}

	var deleteSuccess, createTarget, submitSuccess string
	for _, page := range result.Intent.Pages {
		for _, view := range page.Views {
			for _, action := range view.Actions {
				switch action.Name {
				case "create":
					createTarget = action.TargetPage
				case "delete":
					deleteSuccess = action.SuccessPage
				}
			}
			if view.Submit != nil && view.Submit.Action == "create" {
				submitSuccess = view.Submit.Success.Page
			}
		}
	}
	if createTarget != "UserCreate" {
		t.Errorf("create target = %q, want UserCreate (two create forms exist, so `goto` must decide)", createTarget)
	}
	if deleteSuccess != "Users" {
		t.Errorf("delete success = %q, want Users", deleteSuccess)
	}
	if submitSuccess != "UserDetail" {
		t.Errorf("create submit success = %q, want UserDetail", submitSuccess)
	}
}

func TestNarrowerDestinationStillResolvesWithoutGoto(t *testing.T) {
	// The destination demands a role the source does not. Access must not be used
	// to drop it, because the action's own access is the conjunction of both.
	source := `role admin
role auditor

entity User {
    name String required label
}

page Users {
    allow admin

    list User {
        columns name
        actions view
    }
}

page UserDetail(user User) {
    allow auditor

    detail user {
        fields name
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("narrow.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("a stricter single destination must still resolve:\n%s", diagnosticMessages(result.Diagnostics))
	}
	found := false
	for _, page := range result.Intent.Pages {
		for _, view := range page.Views {
			for _, action := range view.Actions {
				if action.Name == "view" {
					found = true
					if action.TargetPage != "UserDetail" {
						t.Errorf("view target = %q, want UserDetail", action.TargetPage)
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("view action was not resolved")
	}
}

func TestRoleCombinationDestinationsStillNeedGoto(t *testing.T) {
	// Both destinations remain valid under AND composition, so the ambiguity is
	// real and only an explicit destination can resolve it.
	source := `role admin
role auditor

entity User {
    name String required label
}

page Users {
    allow admin

    list User {
        columns name
        actions view
    }
}

page UserDetail(user User) {
    allow auditor

    detail user {
        fields name
    }
}

page UserAudit(user User) {
    allow admin

    detail user {
        fields name
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("roles.forma", source)})
	if !strings.Contains(diagnosticMessages(result.Diagnostics), "resolves to 2 detail destinations") {
		t.Fatalf("role-composed destinations must stay ambiguous:\n%s", diagnosticMessages(result.Diagnostics))
	}
}
