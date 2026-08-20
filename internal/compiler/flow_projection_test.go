package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMembershipFlowProjectionMatchesGoldenAndLinksThreeViews(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	projection, formatted, sourceMap := flowProjectionFromFile(t, path)
	assertFlowGolden(t, formatted, filepath.Join("testdata", "membership.flow.md"))

	if projection.DefaultEntry.Kind != navigationEndpointUnspecified || projection.DefaultEntry.Page != "" {
		t.Fatalf("default entry = %#v", projection.DefaultEntry)
	}
	if projection.Outcomes.TotalGroups != 17 || projection.Outcomes.TotalCases != 83 || projection.Outcomes.LinkedGroups != 10 || projection.Outcomes.LinkedCases != 60 {
		t.Fatalf("outcome coverage = %#v", projection.Outcomes)
	}
	if projection.States.TotalElements != 5 || projection.States.LinkedElements != 5 || projection.States.EdgeAnnotations != 5 {
		t.Fatalf("state coverage = %#v", projection.States)
	}

	register := flowEdgeByTriggerAndKind(t, projection, semanticID("identity", "UserAccount", "operation", "register"), "identity-success")
	assertFlowOutcomeGroups(t, register, semanticID("identity", "UserAccount", "operation", "register"))
	assertFlowStateKinds(t, register, "initializer")
	if register.StateReferences[0].Label != "initialize User.status=Pending" {
		t.Fatalf("registration state = %#v", register.StateReferences)
	}

	verify := flowEdgeByTriggerAndKind(t, projection, semanticID("identity", "UserAccount", "operation", "verify"), "identity-success")
	assertFlowOutcomeGroups(t, verify, semanticID("identity", "UserAccount", "operation", "verify"))
	assertFlowStateKinds(t, verify, "eligibility", "transition")
	if !flowStateLabels(verify)["User.status: Pending -> Active"] {
		t.Fatalf("verification states = %#v", verify.StateReferences)
	}

	external := flowEdgeByKind(t, projection, "external-entry")
	if len(external.OutcomeReferences) != 0 || len(external.StateReferences) != 0 {
		t.Fatalf("external open boundary acquired operation semantics = %#v", external)
	}
	if !flowIDSet(external.Navigation.SourceNodes)[semanticID("identity", "UserAccount", "operation", "verify")] {
		t.Fatal("fixture no longer proves that mere source-node overlap is insufficient")
	}
	assertFlowProvenance(t, projection, sourceMap)
}

func TestAdminFlowProjectionMatchesGoldenAndKeepsSurfaceSpecificOutcomes(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	projection, formatted, sourceMap := flowProjectionFromFile(t, path)
	assertFlowGolden(t, formatted, filepath.Join("testdata", "users.flow.md"))

	suspendEdges := []FlowEdge{}
	for _, edge := range projection.Edges {
		if edge.Navigation.Trigger == actionID("User", "suspend") {
			suspendEdges = append(suspendEdges, edge)
		}
	}
	if len(suspendEdges) != 2 {
		t.Fatalf("suspend edges = %#v", suspendEdges)
	}
	wantGroups := map[SemanticID]bool{
		semanticID("page", "UserDetail", "view", "detail", "User", "action", "suspend"): true,
		semanticID("page", "Users", "view", "list", "User", "action", "suspend"):        true,
	}
	for _, edge := range suspendEdges {
		if len(edge.OutcomeReferences) != 1 || !wantGroups[edge.OutcomeReferences[0].GroupID] {
			t.Fatalf("surface-specific suspend outcome = %#v", edge.OutcomeReferences)
		}
		delete(wantGroups, edge.OutcomeReferences[0].GroupID)
		assertFlowStateKinds(t, edge, "transition")
		if !reflect.DeepEqual(edge.StateReferences[0].Requirements, []string{"confirmation", "roles=admin"}) {
			t.Fatalf("suspend requirements = %#v", edge.StateReferences[0])
		}
	}
	if len(wantGroups) != 0 {
		t.Fatalf("missing suspend groups = %#v", wantGroups)
	}
	assertFlowProvenance(t, projection, sourceMap)
}

func TestFlowProjectionIsIndependentOfDeclarationOrderAndSourcePath(t *testing.T) {
	first := `entity User {
    name String required
    state status Pending | Active initial Pending
}
action User.activate: Pending -> Active
page Users {
    list User {
        columns name, status
        actions view goto UserDetail
    }
}
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
page Users {
    list User {
        columns name, status
        actions view goto UserDetail
    }
}
action User.activate: Pending -> Active
entity User {
    state status Active | Pending initial Pending
    name String required
}
`
	_, firstText, _ := flowProjectionFromSource(t, "first.forma", first)
	_, secondText, _ := flowProjectionFromSource(t, "moved.forma", second)
	if firstText != secondText {
		t.Fatalf("declaration order or source path changed flow projection\nfirst:\n%s\nsecond:\n%s", firstText, secondText)
	}
}

func TestFlowProjectionVisualizesCallerListAsAPolicyNotAFixedDestination(t *testing.T) {
	source := `entity User {
    name String required
}
page Users {
    list User {
        columns name
        actions create
    }
}
page UserCreate {
    form User {
        fields name
        submit create
    }
}
`
	projection, formatted, _ := flowProjectionFromSource(t, "caller-list.forma", source)
	edge := flowEdgeByKind(t, projection, "submit-success")
	if edge.Navigation.DestinationKind != navigationEndpointCallerList || edge.Navigation.Destination != "" || edge.Navigation.Fallback != pageID("UserCreate") {
		t.Fatalf("caller-list edge = %#v", edge.Navigation)
	}
	if !strings.Contains(formatted, `{"caller list"}:::policy`) || !strings.Contains(formatted, `-. "fallback only" .->`) {
		t.Fatalf("visual flow collapsed caller-list into its fallback:\n%s", formatted)
	}
}

func TestStateMutationChangesOnlyTheInvokingFlowEdgeAnnotation(t *testing.T) {
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
		t.Fatal("state mutation did not change fixture")
	}
	before, _, _ := flowProjectionFromSource(t, "users.forma", beforeSource)
	after, _, _ := flowProjectionFromSource(t, "users.forma", afterSource)
	beforeEdges := flowEdgeMap(before)
	afterEdges := flowEdgeMap(after)
	changed := []SemanticID{}
	for id, beforeEdge := range beforeEdges {
		afterEdge, ok := afterEdges[id]
		if !ok {
			t.Fatalf("flow edge %s disappeared", id)
		}
		if !reflect.DeepEqual(beforeEdge, afterEdge) {
			changed = append(changed, id)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("changed flow edges = %v, want one", changed)
	}
	edge := afterEdges[changed[0]]
	if edge.Navigation.Trigger != actionID("User", "confirm") || len(edge.StateReferences) != 1 || edge.StateReferences[0].Label != "User.status: Pending -> Active" {
		t.Fatalf("changed flow edge = %#v", edge)
	}
}

func TestVerificationDestinationMutationAlsoMovesContinuationOrigin(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := string(content)
	afterSource := strings.Replace(beforeSource,
		"        success RegistrationComplete\n        continue SignIn",
		"        success Profile\n        continue SignIn",
		1,
	)
	if afterSource == beforeSource {
		t.Fatal("verification destination mutation did not change fixture")
	}
	before, _, _ := flowProjectionFromSource(t, "membership.forma", beforeSource)
	after, _, _ := flowProjectionFromSource(t, "membership.forma", afterSource)
	beforeEdges := flowEdgeMap(before)
	afterEdges := flowEdgeMap(after)
	changed := map[SemanticID]bool{}
	for id, beforeEdge := range beforeEdges {
		if !reflect.DeepEqual(beforeEdge, afterEdges[id]) {
			changed[id] = true
		}
	}
	verifyID := semanticID("page", "VerifyEmail", "identity", "verify", "UserAccount", "success")
	continuationID := semanticID("page", "VerifyEmail", "identity", "verify", "UserAccount", "continuation")
	if !reflect.DeepEqual(changed, map[SemanticID]bool{verifyID: true, continuationID: true}) {
		t.Fatalf("changed flow edges = %#v", changed)
	}
	if afterEdges[verifyID].Navigation.Destination != pageID("Profile") || afterEdges[continuationID].Navigation.Source != pageID("Profile") {
		t.Fatalf("mutated verification route = %#v / %#v", afterEdges[verifyID].Navigation, afterEdges[continuationID].Navigation)
	}
	if !flowStateLabels(afterEdges[verifyID])["User.status: Pending -> Active"] {
		t.Fatalf("verification mutation lost state effect = %#v", afterEdges[verifyID].StateReferences)
	}
}

func TestFlowProjectionRejectsIncompleteCrossProjectionProvenance(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	projection, _, sourceMap := flowProjectionFromFile(t, path)
	edge := flowEdgeByTriggerAndKind(t, projection, semanticID("identity", "UserAccount", "operation", "register"), "identity-success")
	groupID := edge.OutcomeReferences[0].GroupID
	for edgeIndex := range projection.Edges {
		if projection.Edges[edgeIndex].Navigation.ID != edge.Navigation.ID {
			continue
		}
		reference := &projection.Edges[edgeIndex].OutcomeReferences[0]
		filtered := reference.SourceNodes[:0]
		for _, node := range reference.SourceNodes {
			if node != groupID {
				filtered = append(filtered, node)
			}
		}
		reference.SourceNodes = filtered
		break
	}
	if err := validateFlowProjection(projection, sourceMap); err == nil || !strings.Contains(err.Error(), "incomplete provenance") {
		t.Fatalf("incomplete provenance error = %v", err)
	}
}

func flowProjectionFromFile(t *testing.T, path string) (*FlowProjection, string, *SourceMap) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return flowProjectionFromSource(t, filepath.ToSlash(path), string(content))
}

func flowProjectionFromSource(t *testing.T, path, source string) (*FlowProjection, string, *SourceMap) {
	t.Helper()
	result := Compile([]SourceFile{NewSourceFile(path, source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %#v", result.Diagnostics)
	}
	projection, err := BuildFlowProjection(result.Intent, result.SourceMap)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := FormatFlowProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	return projection, formatted, result.SourceMap
}

func assertFlowGolden(t *testing.T, actual, path string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual != string(expected) {
		t.Fatalf("flow projection differs from %s\nactual:\n%s", path, actual)
	}
}

func flowEdgeByTriggerAndKind(t *testing.T, projection *FlowProjection, trigger SemanticID, kind string) FlowEdge {
	t.Helper()
	for _, edge := range projection.Edges {
		if edge.Navigation.Trigger == trigger && edge.Navigation.Kind == kind {
			return edge
		}
	}
	t.Fatalf("flow edge trigger=%s kind=%s not found", trigger, kind)
	return FlowEdge{}
}

func flowEdgeByKind(t *testing.T, projection *FlowProjection, kind string) FlowEdge {
	t.Helper()
	for _, edge := range projection.Edges {
		if edge.Navigation.Kind == kind {
			return edge
		}
	}
	t.Fatalf("flow edge kind=%s not found", kind)
	return FlowEdge{}
}

func assertFlowOutcomeGroups(t *testing.T, edge FlowEdge, want ...SemanticID) {
	t.Helper()
	got := make([]SemanticID, 0, len(edge.OutcomeReferences))
	for _, reference := range edge.OutcomeReferences {
		got = append(got, reference.GroupID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outcome groups = %v, want %v", got, want)
	}
}

func assertFlowStateKinds(t *testing.T, edge FlowEdge, want ...string) {
	t.Helper()
	got := make([]string, 0, len(edge.StateReferences))
	for _, reference := range edge.StateReferences {
		got = append(got, reference.Kind)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state kinds = %v, want %v", got, want)
	}
}

func flowStateLabels(edge FlowEdge) map[string]bool {
	result := map[string]bool{}
	for _, reference := range edge.StateReferences {
		result[reference.Label] = true
	}
	return result
}

func assertFlowProvenance(t *testing.T, projection *FlowProjection, sourceMap *SourceMap) {
	t.Helper()
	entries := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		entries[entry.NodeID] = true
	}
	for _, edge := range projection.Edges {
		if len(edge.SourceNodes) == 0 {
			t.Fatalf("flow edge %s has no provenance", edge.Navigation.ID)
		}
		for _, node := range edge.SourceNodes {
			if !entries[node] {
				t.Fatalf("flow edge %s source node %s is not traceable", edge.Navigation.ID, node)
			}
		}
	}
}

func flowEdgeMap(projection *FlowProjection) map[SemanticID]FlowEdge {
	result := map[SemanticID]FlowEdge{}
	for _, edge := range projection.Edges {
		result[edge.Navigation.ID] = edge
	}
	return result
}
