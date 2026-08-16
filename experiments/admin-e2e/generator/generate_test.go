package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/experiments/conformance"
	"github.com/horizon67/forma/internal/compiler"
)

func TestBuildSpecLowersAdminPresentations(t *testing.T) {
	source := readExperimentSource(t)
	spec := buildUsersSpec(t, source)

	if spec.Entity != "User" || spec.BasePath != "/users" {
		t.Fatalf("entity route = %s %s", spec.Entity, spec.BasePath)
	}
	if !reflect.DeepEqual(spec.Roles, []string{"admin"}) {
		t.Fatalf("roles = %#v", spec.Roles)
	}
	if !reflect.DeepEqual(spec.List.Fields, []string{"name", "email", "team", "plan", "status"}) {
		t.Fatalf("list fields = %#v", spec.List.Fields)
	}
	if !reflect.DeepEqual(spec.Detail.Fields, []string{"name", "email", "team", "plan", "status"}) {
		t.Fatalf("detail fields = %#v", spec.Detail.Fields)
	}
	if !reflect.DeepEqual(spec.Edit.Fields, []string{"name", "email", "team", "plan"}) {
		t.Fatalf("edit fields = %#v", spec.Edit.Fields)
	}
	if spec.EntityLabel != "name" {
		t.Fatalf("entity label = %q", spec.EntityLabel)
	}
	if !reflect.DeepEqual(actionNames(spec.List.Actions), []string{"view", "edit"}) || !reflect.DeepEqual(actionNames(spec.Detail.Actions), []string{"edit"}) {
		t.Fatalf("actions were not preserved: list=%v detail=%v", spec.List.Actions, spec.Detail.Actions)
	}
	for _, action := range append(append([]ActionSpec(nil), spec.List.Actions...), spec.Detail.Actions...) {
		if action.Kind != "standard" || !action.PreventDuplicateDispatch || !action.FailureFeedback {
			t.Fatalf("action contract was not preserved: %#v", action)
		}
		assertAdminAccess(t, action.Access)
	}
	if spec.List.Actions[0].TargetPage != spec.Detail.PageName || spec.List.Actions[0].SuccessPage != "" ||
		spec.List.Actions[1].TargetPage != spec.Edit.PageName || spec.List.Actions[1].SuccessPage != spec.Detail.PageName ||
		spec.Detail.Actions[0].TargetPage != spec.Edit.PageName || spec.Detail.Actions[0].SuccessPage != spec.Detail.PageName {
		t.Fatalf("action navigation was not preserved: list=%#v detail=%#v", spec.List.Actions, spec.Detail.Actions)
	}
	if spec.Edit.Submit == nil || spec.Edit.Submit.Action != "edit" ||
		spec.Edit.Submit.Success.Kind != "page" || spec.Edit.Submit.Success.Page != spec.Detail.PageName ||
		!spec.Edit.Submit.Success.RecheckAccess || !spec.Edit.Submit.PreventDuplicateDispatch || !spec.Edit.Submit.FailureFeedback {
		t.Fatalf("submit contract was not preserved: %#v", spec.Edit.Submit)
	}
	assertAdminAccess(t, spec.Edit.Submit.Access)
	if !reflect.DeepEqual(spec.List.InteractionStates, []string{"loading", "ready", "empty", "failure"}) {
		t.Fatalf("list interaction states = %#v", spec.List.InteractionStates)
	}
	if field := spec.Fields["email"]; field.InputType != "text" || field.Label != "Email" {
		t.Fatalf("email presentation = %#v", field)
	}
	if !reflect.DeepEqual(spec.List.Allows, []string{"admin"}) ||
		!reflect.DeepEqual(spec.Detail.Allows, []string{"admin"}) ||
		!reflect.DeepEqual(spec.Edit.Allows, []string{"admin"}) {
		t.Fatalf("page access was not preserved: list=%v detail=%v edit=%v", spec.List.Allows, spec.Detail.Allows, spec.Edit.Allows)
	}
	if field := spec.Fields["team"]; field.RelationEntity != "Team" || spec.RelatedLabels["Team"] != "name" {
		t.Fatalf("team relation = %#v, labels = %#v", field, spec.RelatedLabels)
	}
	if field := spec.Fields["plan"]; !field.Required || !reflect.DeepEqual(field.Variants, []string{"Free", "Pro", "Enterprise"}) {
		t.Fatalf("plan field = %#v", field)
	}
	if field := spec.Fields["status"]; !field.State || !field.Required ||
		!reflect.DeepEqual(field.Variants, []string{"Pending", "Confirmed", "Active", "Suspended"}) {
		t.Fatalf("state field = %#v", field)
	}
}

func TestBuildSpecRejectsUnrealizedNavigationIntent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compiler.ResolvedIntent)
		want   string
	}{
		{
			name: "action target",
			mutate: func(ir *compiler.ResolvedIntent) {
				findViewAction(t, ir, "list", "view").TargetPage = "Missing"
			},
			want: "does not realize target page Missing",
		},
		{
			name: "action success",
			mutate: func(ir *compiler.ResolvedIntent) {
				findViewAction(t, ir, "list", "edit").SuccessPage = "Missing"
			},
			want: "does not realize success page Missing",
		},
		{
			name: "submit navigation",
			mutate: func(ir *compiler.ResolvedIntent) {
				findEditSubmit(t, ir).Success.Page = "Missing"
			},
			want: "does not realize submit navigation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := compiler.Compile([]compiler.SourceFile{
				compiler.NewSourceFile("action-contract.forma", readExperimentSource(t)),
			})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("compile diagnostics: %v", result.Diagnostics)
			}
			test.mutate(result.Intent)
			_, err := BuildSpec(result.Intent, testProfile())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("action contract error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildSpecRejectsPublicEditWithBoundedSubmissionTokens(t *testing.T) {
	source := strings.Replace(readExperimentSource(t), `page UserEdit(user User) {
    allow admin

    form user {`, `page UserEdit(user User) {
    form user {`, 1)
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("public-edit.forma", source),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	_, err := BuildSpec(result.Intent, testProfile())
	if err == nil || !strings.Contains(err.Error(), "declare allow roles for a bounded submission principal scope") {
		t.Fatalf("public edit error = %v", err)
	}
}

func TestBuildSpecReflectsFormaEditFieldChange(t *testing.T) {
	source := readExperimentSource(t)
	editPage := strings.Index(source, "page UserEdit")
	if editPage < 0 {
		t.Fatal("UserEdit page not found")
	}
	changedTail := strings.Replace(source[editPage:], "fields name, email, team, plan", "fields name, email", 1)
	if changedTail == source[editPage:] {
		t.Fatal("UserEdit fields were not changed")
	}
	spec := buildUsersSpec(t, source[:editPage]+changedTail)
	if !reflect.DeepEqual(spec.Edit.Fields, []string{"name", "email"}) {
		t.Fatalf("changed edit fields = %#v", spec.Edit.Fields)
	}
}

func TestGenerateWritesReplaceableStandaloneArtifact(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	output := filepath.Join(t.TempDir(), "artifact")
	options := Options{
		SourcePath:   filepath.Join(root, "experiments", "admin-e2e", "app.forma"),
		ProfilePath:  filepath.Join(root, "experiments", "admin-e2e", "profile.json"),
		FixturesPath: filepath.Join(root, "experiments", "admin-e2e", "fixtures.json"),
		OutputPath:   output,
	}
	if err := Generate(options); err != nil {
		t.Fatal(err)
	}
	for _, name := range generatedArtifactFiles() {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Errorf("generated %s: %v", name, err)
		}
	}
	contractContent, err := os.ReadFile(filepath.Join(output, "conformance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract conformance.Contract
	if err := json.Unmarshal(contractContent, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Schema != conformance.Schema || len(contract.Cases) != 7 {
		t.Fatalf("generated conformance contract = %#v", contract)
	}
	before := readArtifact(t, output)
	if err := Generate(options); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second generation without force = %v", err)
	}
	options.Force = true
	if err := Generate(options); err != nil {
		t.Fatalf("force regeneration: %v", err)
	}
	after := readArtifact(t, output)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("force regeneration was not byte-identical")
	}
}

func TestBuildSpecRejectsDroppedIntent(t *testing.T) {
	base := readExperimentSource(t)
	tests := []struct {
		name        string
		old         string
		replacement string
		want        string
	}{
		{name: "search", old: "columns name, email, team, plan, status", replacement: "columns name, email, team, plan, status\n        search name", want: "search intent"},
		{name: "filter", old: "columns name, email, team, plan, status", replacement: "columns name, email, team, plan, status\n        filter status", want: "filter intent"},
		{name: "sort", old: "columns name, email, team, plan, status", replacement: "columns name, email, team, plan, status\n        sort name asc", want: "sort intent"},
		{name: "paginate", old: "columns name, email, team, plan, status", replacement: "columns name, email, team, plan, status\n        paginate 20", want: "paginate intent"},
		{name: "action", old: "actions view, edit", replacement: "actions view, edit, delete", want: "does not realize action delete"},
		{name: "unique", old: "email Email required", replacement: "email Email required unique", want: "does not realize unique constraint on User.email"},
		{name: "related entity unique", old: "name String required label", replacement: "name String required unique label", want: "does not realize unique constraint on Team.name"},
		{name: "matches", old: "type Email = String", replacement: "type Email = String matches /.+@.+/", want: "does not realize matches constraint on User.email"},
		{name: "collection", old: "team  Team", replacement: "team  Team\n    teams [Team]", want: "does not realize to-many relation User.teams"},
		{name: "default", old: "team  Team", replacement: "team  Team\n    role String default \"member\"", want: "does not realize default value on User.role"},
		{name: "create view", old: "page UserDetail", replacement: `page UserCreate {
    allow admin
    form User {
        fields name, email, team, plan
        submit create
    }
}

page UserDetail`, want: "does not realize form view"},
		{name: "other entity view", old: "page Users", replacement: `page Teams {
    allow admin
    list Team {
        columns name
        search name
    }
}

page Users`, want: "does not realize list view"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := strings.Replace(base, test.old, test.replacement, 1)
			result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("unsupported.forma", source)})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("compile diagnostics: %v", result.Diagnostics)
			}
			_, err := BuildSpec(result.Intent, testProfile())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unsupported intent error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildSpecLowersUnionVariants(t *testing.T) {
	spec := buildUsersSpec(t, readExperimentSource(t))
	if !reflect.DeepEqual(spec.Fields["plan"].Variants, []string{"Free", "Pro", "Enterprise"}) {
		t.Fatalf("plan variants = %#v", spec.Fields["plan"].Variants)
	}
	if !reflect.DeepEqual(spec.Edit.Fields, []string{"name", "email", "team", "plan"}) {
		t.Fatalf("union edit fields = %#v", spec.Edit.Fields)
	}
}

func TestBuildSpecRejectsUnknownInteractionState(t *testing.T) {
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("interaction-state.forma", readExperimentSource(t)),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	for pageIndex := range result.Intent.Pages {
		for viewIndex := range result.Intent.Pages[pageIndex].Views {
			view := &result.Intent.Pages[pageIndex].Views[viewIndex]
			if view.Kind == "list" {
				view.InteractionStates = append(view.InteractionStates, "stale")
			}
		}
	}
	_, err := BuildSpec(result.Intent, testProfile())
	if err == nil || !strings.Contains(err.Error(), "does not realize interaction state stale") {
		t.Fatalf("unknown interaction state error = %v", err)
	}
}

func TestBuildSpecRequiresEmptyListInteractionState(t *testing.T) {
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("missing-empty.forma", readExperimentSource(t)),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	for pageIndex := range result.Intent.Pages {
		for viewIndex := range result.Intent.Pages[pageIndex].Views {
			view := &result.Intent.Pages[pageIndex].Views[viewIndex]
			if view.Kind == "list" {
				view.InteractionStates = []string{"failure"}
			}
		}
	}
	_, err := BuildSpec(result.Intent, testProfile())
	if err == nil || !strings.Contains(err.Error(), "requires empty interaction state") {
		t.Fatalf("missing empty state error = %v", err)
	}
}

func TestBuildSpecRejectsDuplicateViewInsteadOfLastWins(t *testing.T) {
	source := strings.Replace(readExperimentSource(t), "page UserDetail", `page UsersAlternate {
    allow admin
    list User {
        columns name
        actions view
    }
}

page UserDetail`, 1)
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("duplicate.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	_, err := BuildSpec(result.Intent, testProfile())
	if err == nil || !strings.Contains(err.Error(), "duplicate list views") {
		t.Fatalf("duplicate view error = %v", err)
	}
}

func TestBuildSpecRequiresExplicitEntityLabel(t *testing.T) {
	source := strings.Replace(readExperimentSource(t), "name  String required label", "name  String required", 1)
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("no-label.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	_, err := BuildSpec(result.Intent, testProfile())
	if err == nil || !strings.Contains(err.Error(), "declare a label field") {
		t.Fatalf("missing label error = %v", err)
	}
}

func TestBuildSpecRequiresLabelFieldToBeRequired(t *testing.T) {
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("optional-label.forma", readExperimentSource(t)),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	for entityIndex := range result.Intent.Entities {
		if result.Intent.Entities[entityIndex].Name != "User" {
			continue
		}
		for fieldIndex := range result.Intent.Entities[entityIndex].Fields {
			if result.Intent.Entities[entityIndex].Fields[fieldIndex].Name == "name" {
				result.Intent.Entities[entityIndex].Fields[fieldIndex].Required = false
			}
		}
	}
	_, err := BuildSpec(result.Intent, testProfile())
	if err == nil || !strings.Contains(err.Error(), "label field name to be required") {
		t.Fatalf("optional label error = %v", err)
	}
}

func readExperimentSource(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "experiments", "admin-e2e", "app.forma"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func buildUsersSpec(t *testing.T, source string) AdminSpec {
	t.Helper()
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("experiments/admin-e2e/app.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %v", result.Diagnostics)
	}
	spec, err := BuildSpec(result.Intent, testProfile())
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func actionNames(actions []ActionSpec) []string {
	names := make([]string, len(actions))
	for index, action := range actions {
		names[index] = action.Name
	}
	return names
}

func assertAdminAccess(t *testing.T, access compiler.IRAccess) {
	t.Helper()
	if access.ID == "" || len(access.AllOf) == 0 {
		t.Fatalf("access contract is incomplete: %#v", access)
	}
	for _, requirement := range access.AllOf {
		if requirement.Source == "" || !reflect.DeepEqual(requirement.AnyOf, []string{"admin"}) {
			t.Fatalf("access requirement = %#v", requirement)
		}
	}
}

func findEditSubmit(t *testing.T, ir *compiler.ResolvedIntent) *compiler.IRSubmitIntent {
	t.Helper()
	for pageIndex := range ir.Pages {
		for viewIndex := range ir.Pages[pageIndex].Views {
			view := &ir.Pages[pageIndex].Views[viewIndex]
			if view.Kind == "form" && view.Mode == "edit" && view.Submit != nil {
				return view.Submit
			}
		}
	}
	t.Fatal("edit submit intent not found")
	return nil
}

func findViewAction(t *testing.T, ir *compiler.ResolvedIntent, viewKind, actionName string) *compiler.IRActionRef {
	t.Helper()
	for pageIndex := range ir.Pages {
		for viewIndex := range ir.Pages[pageIndex].Views {
			view := &ir.Pages[pageIndex].Views[viewIndex]
			if view.Kind != viewKind {
				continue
			}
			for actionIndex := range view.Actions {
				if view.Actions[actionIndex].Name == actionName {
					return &view.Actions[actionIndex]
				}
			}
		}
	}
	t.Fatalf("%s action %s not found", viewKind, actionName)
	return nil
}

func testProfile() Profile {
	return Profile{
		Schema: "forma/experiment-profile/v0", ID: "go-stdlib-admin/v0", Runtime: "go-stdlib",
		Persistence: "memory", PrincipalAdapter: "test-cookie", DefaultAddress: "127.0.0.1:4317",
	}
}

func readArtifact(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	for _, name := range generatedArtifactFiles() {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		result[name] = content
	}
	return result
}

func generatedArtifactFiles() []string {
	return []string{
		"go.mod", "main.go", "main_test.go", "conformance.json",
		"conformance_runner_test.go", "conformance_adapter_test.go", markerName,
	}
}
