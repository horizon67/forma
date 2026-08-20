package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMembershipDomainStateProjectionMatchesGoldenAndSeparatesSessionState(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	projection, formatted, sourceMap := domainStateProjectionFromFile(t, path)
	assertDomainStateGolden(t, formatted, filepath.Join("testdata", "membership.states.txt"))

	if len(projection.Machines) != 1 {
		t.Fatalf("state machines = %d, want 1", len(projection.Machines))
	}
	machine := projection.Machines[0]
	if machine.ID != semanticID("entity", "User", "state", "status") || machine.Initial != "Pending" {
		t.Fatalf("state machine = %#v", machine)
	}
	if len(machine.Initializers) != 1 || machine.Initializers[0].Trigger != identityOperationID("UserAccount", "register") || machine.Initializers[0].Value != "Pending" {
		t.Fatalf("initializers = %#v", machine.Initializers)
	}
	if len(machine.Transitions) != 1 {
		t.Fatalf("transitions = %#v", machine.Transitions)
	}
	transition := machine.Transitions[0]
	if transition.Action != actionID("User", "activate") || transition.From != "Pending" || transition.To != "Active" {
		t.Fatalf("activation transition = %#v", transition)
	}
	if len(transition.InvokedBy) != 1 || transition.InvokedBy[0].Node != identityOperationID("UserAccount", "verify") || transition.InvokedBy[0].Kind != "identity-success" {
		t.Fatalf("activation invocations = %#v", transition.InvokedBy)
	}
	for _, operation := range []SemanticID{identityOperationID("UserAccount", "signin"), identityOperationID("UserAccount", "signout")} {
		for _, item := range machine.Transitions {
			if item.Action == operation {
				t.Fatalf("session operation %s was projected as domain transition", operation)
			}
		}
	}
	eligibility := map[SemanticID]string{}
	for _, item := range machine.Eligibility {
		eligibility[item.Operation] = item.State
	}
	wantEligibility := map[SemanticID]string{
		identityOperationID("UserAccount", "verify"): "Pending",
		identityOperationID("UserAccount", "resend"): "Pending",
		identityOperationID("UserAccount", "signin"): "Active",
	}
	if !reflect.DeepEqual(eligibility, wantEligibility) {
		t.Fatalf("eligibility = %#v, want %#v", eligibility, wantEligibility)
	}
	assertDomainStateProvenance(t, projection, sourceMap)
}

func TestAdminDomainStateProjectionMatchesGolden(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	projection, formatted, sourceMap := domainStateProjectionFromFile(t, path)
	assertDomainStateGolden(t, formatted, filepath.Join("testdata", "users.states.txt"))
	machine := projection.Machines[0]
	if len(machine.Initializers) != 1 || machine.Initializers[0].Trigger != semanticID("page", "UserCreate", "view", "form", "create", "User", "submit") {
		t.Fatalf("admin initializers = %#v", machine.Initializers)
	}
	suspend := domainStateTransitionByActionAndSource(t, machine, actionID("User", "suspend"), "Active")
	if !suspend.Confirm || !reflect.DeepEqual(suspend.Roles, []string{"admin"}) || len(suspend.InvokedBy) != 2 {
		t.Fatalf("suspend transition = %#v", suspend)
	}
	assertDomainStateProvenance(t, projection, sourceMap)
}

func TestDomainStateProjectionIsIndependentOfDeclarationAndValueOrder(t *testing.T) {
	first := `entity User {
    name String
    state status Pending | Active initial Pending
}
action User.activate: Pending -> Active
page UserDetail(user User) {
    detail user {
        fields name, status
        actions activate
    }
}
`
	second := `page UserDetail(user User) {
    detail user {
        fields name, status
        actions activate
    }
}
action User.activate: Pending -> Active
entity User {
    name String
    state status Active | Pending initial Pending
}
`
	_, firstText, _ := domainStateProjectionFromSource(t, "first.forma", first)
	_, secondText, _ := domainStateProjectionFromSource(t, "moved.forma", second)
	if firstText != secondText {
		t.Fatalf("declaration, value order, or source path changed state projection\nfirst:\n%s\nsecond:\n%s", firstText, secondText)
	}
}

func TestStateDestinationMutationChangesExactlyOneTransition(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := string(content)
	afterSource := strings.Replace(beforeSource,
		"action User.confirm:   Pending   -> Confirmed",
		"action User.confirm:   Pending   -> Active",
		1,
	)
	if afterSource == beforeSource {
		t.Fatal("state destination mutation did not change fixture")
	}
	before, _, _ := domainStateProjectionFromSource(t, "users.forma", beforeSource)
	after, _, _ := domainStateProjectionFromSource(t, "users.forma", afterSource)
	beforeTransitions := domainStateTransitionMap(before)
	afterTransitions := domainStateTransitionMap(after)
	changed := []SemanticID{}
	for id, beforeTransition := range beforeTransitions {
		afterTransition, ok := afterTransitions[id]
		if !ok {
			t.Fatalf("transition %s disappeared", id)
		}
		if !reflect.DeepEqual(beforeTransition, afterTransition) {
			changed = append(changed, id)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("changed transitions = %v, want one", changed)
	}
	transitionID := semanticID("projection", "states", "transition", string(actionID("User", "confirm")), "from", "Pending")
	if changed[0] != transitionID || beforeTransitions[transitionID].To != "Confirmed" || afterTransitions[transitionID].To != "Active" {
		t.Fatalf("state mutation = %#v -> %#v", beforeTransitions[transitionID], afterTransitions[transitionID])
	}
}

func TestRemovingASurfaceKeepsTheDomainTransitionAndChangesOnlyItsInvocation(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := string(content)
	afterSource := strings.Replace(beforeSource,
		"actions create, view, edit, delete, suspend",
		"actions create, view, edit, delete",
		1,
	)
	if afterSource == beforeSource {
		t.Fatal("surface mutation did not change fixture")
	}
	before, _, _ := domainStateProjectionFromSource(t, "users.forma", beforeSource)
	after, _, _ := domainStateProjectionFromSource(t, "users.forma", afterSource)
	beforeTransitions := domainStateTransitionMap(before)
	afterTransitions := domainStateTransitionMap(after)
	changed := []SemanticID{}
	for id, beforeTransition := range beforeTransitions {
		afterTransition, ok := afterTransitions[id]
		if !ok {
			t.Fatalf("transition %s disappeared", id)
		}
		if !reflect.DeepEqual(beforeTransition, afterTransition) {
			changed = append(changed, id)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("changed transitions = %v, want one", changed)
	}
	transition := afterTransitions[changed[0]]
	if transition.Action != actionID("User", "suspend") || len(transition.InvokedBy) != 1 || transition.InvokedBy[0].Node != semanticID("page", "UserDetail", "view", "detail", "User", "action", "suspend") {
		t.Fatalf("remaining suspend transition = %#v", transition)
	}
}

func TestMultiSourceActionProducesOneTransitionPerSource(t *testing.T) {
	source := `entity User {
    name String
    state status Pending | Active | Suspended | Archived initial Pending
}
action User.archive: Active | Suspended -> Archived
page UserDetail(user User) {
    detail user {
        fields name, status
        actions archive
    }
}
`
	projection, _, _ := domainStateProjectionFromSource(t, "multi-source.forma", source)
	machine := projection.Machines[0]
	if len(machine.Transitions) != 2 {
		t.Fatalf("transitions = %#v", machine.Transitions)
	}
	for _, sourceValue := range []string{"Active", "Suspended"} {
		transition := domainStateTransitionByActionAndSource(t, machine, actionID("User", "archive"), sourceValue)
		if transition.To != "Archived" || len(transition.InvokedBy) != 1 {
			t.Fatalf("%s transition = %#v", sourceValue, transition)
		}
	}
}

func TestDomainStateProjectionRejectsIncompleteProvenance(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	projection, _, sourceMap := domainStateProjectionFromFile(t, path)
	transition := &projection.Machines[0].Transitions[0]
	filtered := transition.SourceNodes[:0]
	for _, node := range transition.SourceNodes {
		if node != transition.Action {
			filtered = append(filtered, node)
		}
	}
	transition.SourceNodes = filtered
	if err := validateDomainStateProjection(projection, sourceMap); err == nil || !strings.Contains(err.Error(), "incomplete provenance") {
		t.Fatalf("incomplete provenance error = %v", err)
	}
}

func domainStateProjectionFromFile(t *testing.T, path string) (*DomainStateProjection, string, *SourceMap) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return domainStateProjectionFromSource(t, filepath.ToSlash(path), string(content))
}

func domainStateProjectionFromSource(t *testing.T, path, source string) (*DomainStateProjection, string, *SourceMap) {
	t.Helper()
	result := Compile([]SourceFile{NewSourceFile(path, source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %#v", result.Diagnostics)
	}
	projection, err := BuildDomainStateProjection(result.Intent, result.SourceMap)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := FormatDomainStateProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	return projection, formatted, result.SourceMap
}

func assertDomainStateGolden(t *testing.T, actual, path string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual != string(expected) {
		t.Fatalf("domain state projection differs from %s\nactual:\n%s", path, actual)
	}
}

func assertDomainStateProvenance(t *testing.T, projection *DomainStateProjection, sourceMap *SourceMap) {
	t.Helper()
	entries := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		entries[entry.NodeID] = true
	}
	check := func(id SemanticID, nodes []SemanticID) {
		if len(nodes) == 0 {
			t.Fatalf("state element %s has no provenance", id)
		}
		for _, node := range nodes {
			if !entries[node] {
				t.Fatalf("state element %s source node %s is not traceable", id, node)
			}
		}
	}
	for _, machine := range projection.Machines {
		for _, initializer := range machine.Initializers {
			check(initializer.ID, initializer.SourceNodes)
		}
		for _, transition := range machine.Transitions {
			check(transition.ID, transition.SourceNodes)
			for _, invocation := range transition.InvokedBy {
				check(transition.ID, invocation.SourceNodes)
			}
		}
		for _, eligibility := range machine.Eligibility {
			check(eligibility.ID, eligibility.SourceNodes)
		}
	}
}

func domainStateTransitionByActionAndSource(t *testing.T, machine DomainStateMachine, action SemanticID, source string) DomainStateTransition {
	t.Helper()
	for _, transition := range machine.Transitions {
		if transition.Action == action && transition.From == source {
			return transition
		}
	}
	t.Fatalf("transition %s from %s not found", action, source)
	return DomainStateTransition{}
}

func domainStateTransitionMap(projection *DomainStateProjection) map[SemanticID]DomainStateTransition {
	result := map[SemanticID]DomainStateTransition{}
	for _, machine := range projection.Machines {
		for _, transition := range machine.Transitions {
			result[transition.ID] = transition
		}
	}
	return result
}
