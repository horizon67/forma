package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMembershipNavigationProjectionMatchesGoldenAndKeepsBoundaries(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	projection, formatted, sourceMap := navigationProjectionFromFile(t, sourcePath)
	assertNavigationGolden(t, formatted, filepath.Join("testdata", "membership.navigation.txt"))

	if projection.DefaultEntry.Kind != navigationEndpointUnspecified {
		t.Fatalf("default entry = %#v", projection.DefaultEntry)
	}
	sourceEntries := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		sourceEntries[entry.NodeID] = true
	}
	for _, edge := range projection.Edges {
		if len(edge.SourceNodes) == 0 {
			t.Fatalf("edge %s has no provenance", edge.ID)
		}
		for _, node := range edge.SourceNodes {
			if !sourceEntries[node] {
				t.Fatalf("edge %s source node %s is not traceable", edge.ID, node)
			}
		}
	}

	external := navigationEdgesOfKind(projection, "external-entry")
	if len(external) != 1 || external[0].SourceKind != navigationEndpointExternalBoundary || external[0].Destination != pageID("VerifyEmail") {
		t.Fatalf("external entry edges = %#v", external)
	}
	resend := navigationEdgeByID(t, projection, semanticID("page", "CheckEmail", "identity", "resend", "UserAccount", "success"))
	if resend.DestinationKind != navigationEndpointSameContext || resend.Destination != pageID("CheckEmail") {
		t.Fatalf("resend destination = %#v", resend)
	}
	verify := navigationEdgeByID(t, projection, semanticID("page", "VerifyEmail", "identity", "verify", "UserAccount", "success"))
	if len(verify.Effects) != 1 || verify.Effects[0].Node != actionID("User", "activate") || verify.Effects[0].Label != "User.activate" {
		t.Fatalf("verify effects = %#v", verify.Effects)
	}
}

func TestAdminNavigationProjectionMatchesGolden(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "examples", "users.forma")
	_, formatted, _ := navigationProjectionFromFile(t, sourcePath)
	assertNavigationGolden(t, formatted, filepath.Join("testdata", "users.navigation.txt"))
}

func TestNavigationProjectionIsIndependentOfDeclarationOrder(t *testing.T) {
	first := `entry Users
entity User {
    name String required
}
page Users {
    list User {
        columns name
        actions view
    }
}
page UserDetail(user User) {
    detail user {
        fields name
    }
}
`
	second := `page UserDetail(user User) {
    detail user {
        fields name
    }
}
page Users {
    list User {
        columns name
        actions view
    }
}
entry Users
entity User {
    name String required
}
`

	_, firstText, _ := navigationProjectionFromSource(t, "first.forma", first)
	_, secondText, _ := navigationProjectionFromSource(t, "moved.forma", second)
	if firstText != secondText {
		t.Fatalf("declaration order or source path changed navigation projection\nfirst:\n%s\nsecond:\n%s", firstText, secondText)
	}
}

func TestNavigationDestinationMutationChangesExactlyOneEdge(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := string(content)
	afterSource := strings.Replace(
		beforeSource,
		"actions view goto UserDetail, edit goto UserEdit",
		"actions view goto Profile, edit goto UserEdit",
		1,
	)
	if afterSource == beforeSource {
		t.Fatal("destination mutation did not change fixture")
	}

	before, _, _ := navigationProjectionFromSource(t, "membership.forma", beforeSource)
	after, _, _ := navigationProjectionFromSource(t, "membership.forma", afterSource)
	beforeEdges := navigationEdgeMap(before)
	afterEdges := navigationEdgeMap(after)
	if len(beforeEdges) != len(afterEdges) {
		t.Fatalf("edge count changed from %d to %d", len(beforeEdges), len(afterEdges))
	}
	changed := []SemanticID{}
	for id, beforeEdge := range beforeEdges {
		afterEdge, ok := afterEdges[id]
		if !ok {
			t.Fatalf("edge %s disappeared", id)
		}
		if !reflect.DeepEqual(beforeEdge, afterEdge) {
			changed = append(changed, id)
		}
	}
	if len(changed) != 1 {
		t.Fatalf("changed edges = %v, want one", changed)
	}
	wantID := projectionNavigationID(semanticID("page", "Users", "view", "list", "User", "action", "view"), "target")
	if changed[0] != wantID {
		t.Fatalf("changed edge = %s, want %s", changed[0], wantID)
	}
	if beforeEdges[wantID].Destination != pageID("UserDetail") || afterEdges[wantID].Destination != pageID("Profile") {
		t.Fatalf("destination mutation = %s -> %s", beforeEdges[wantID].Destination, afterEdges[wantID].Destination)
	}
}

func TestNavigationOperationMutationReplacesOnlyItsEdge(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := string(content)
	afterSource := strings.Replace(
		beforeSource,
		"action User.confirm:   Pending   -> Confirmed",
		"action User.approve:   Pending   -> Confirmed",
		1,
	)
	afterSource = strings.Replace(
		afterSource,
		"actions edit, delete, confirm, activate, suspend, reinstate",
		"actions edit, delete, approve, activate, suspend, reinstate",
		1,
	)
	if afterSource == beforeSource || strings.Contains(afterSource, "User.confirm") {
		t.Fatal("operation mutation did not change fixture")
	}

	before, _, _ := navigationProjectionFromSource(t, "users.forma", beforeSource)
	after, _, _ := navigationProjectionFromSource(t, "users.forma", afterSource)
	beforeEdges := navigationEdgeMap(before)
	afterEdges := navigationEdgeMap(after)
	oldID := projectionNavigationID(semanticID("page", "UserDetail", "view", "detail", "User", "action", "confirm"), "success")
	newID := projectionNavigationID(semanticID("page", "UserDetail", "view", "detail", "User", "action", "approve"), "success")
	oldEdge, oldExists := beforeEdges[oldID]
	newEdge, newExists := afterEdges[newID]
	if !oldExists || !newExists || oldEdge.Trigger != actionID("User", "confirm") || newEdge.Trigger != actionID("User", "approve") {
		t.Fatalf("operation edges = before %#v, after %#v", oldEdge, newEdge)
	}
	if oldEdge.Destination != newEdge.Destination || oldEdge.DestinationKind != newEdge.DestinationKind {
		t.Fatalf("operation mutation changed navigation policy: %#v -> %#v", oldEdge, newEdge)
	}
	delete(beforeEdges, oldID)
	delete(afterEdges, newID)
	if !reflect.DeepEqual(beforeEdges, afterEdges) {
		t.Fatal("operation mutation changed unrelated navigation edges")
	}
}

func TestExternalEntrySurfaceMutationKeepsTheBoundaryExplicit(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSource := string(content)
	oldSurface := `page VerifyEmail {
    interact UserAccount.verify {
        evidence email
        success RegistrationComplete
        continue SignIn
        feedback invalid, expired, failure
    }
}

page RegistrationComplete {
}`
	newSurface := `page VerifyEmail {
}

page RegistrationComplete {
    interact UserAccount.verify {
        evidence email
        success VerifyEmail
        continue SignIn
        feedback invalid, expired, failure
    }
}`
	afterSource := strings.Replace(beforeSource, oldSurface, newSurface, 1)
	if afterSource == beforeSource {
		t.Fatal("external entry surface mutation did not change fixture")
	}

	before, _, _ := navigationProjectionFromSource(t, "membership.forma", beforeSource)
	after, _, _ := navigationProjectionFromSource(t, "membership.forma", afterSource)
	edgeID := projectionExternalEntryID(semanticID("identity", "UserAccount", "verification", "email"))
	beforeEntry := navigationEdgeByID(t, before, edgeID)
	afterEntry := navigationEdgeByID(t, after, edgeID)
	if beforeEntry.Destination != pageID("VerifyEmail") || afterEntry.Destination != pageID("RegistrationComplete") {
		t.Fatalf("external entry destination = %s -> %s", beforeEntry.Destination, afterEntry.Destination)
	}
	if beforeEntry.SourceKind != navigationEndpointExternalBoundary || afterEntry.SourceKind != navigationEndpointExternalBoundary || beforeEntry.Kind != "external-entry" || afterEntry.Kind != "external-entry" {
		t.Fatalf("external entry boundary was collapsed: %#v -> %#v", beforeEntry, afterEntry)
	}
}

func TestNavigationProjectionPreservesCallerListPolicy(t *testing.T) {
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
	projection, formatted, _ := navigationProjectionFromSource(t, "caller-list.forma", source)
	edge := navigationEdgeByID(t, projection, semanticID("page", "UserCreate", "view", "form", "create", "User", "submit", "success"))
	if edge.DestinationKind != navigationEndpointCallerList || edge.Fallback != pageID("UserCreate") || edge.Destination != "" {
		t.Fatalf("caller-list destination = %#v", edge)
	}
	if !strings.Contains(formatted, "caller list (fallback UserCreate)") {
		t.Fatalf("formatted projection hides caller-list policy:\n%s", formatted)
	}
}

func navigationProjectionFromFile(t *testing.T, path string) (*NavigationProjection, string, *SourceMap) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return navigationProjectionFromSource(t, filepath.ToSlash(path), string(content))
}

func navigationProjectionFromSource(t *testing.T, path, source string) (*NavigationProjection, string, *SourceMap) {
	t.Helper()
	result := Compile([]SourceFile{NewSourceFile(path, source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %#v", result.Diagnostics)
	}
	projection, err := BuildNavigationProjection(result.Intent, result.SourceMap)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := FormatNavigationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	return projection, formatted, result.SourceMap
}

func assertNavigationGolden(t *testing.T, actual, path string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual != string(expected) {
		t.Fatalf("navigation projection differs from %s\nactual:\n%s", path, actual)
	}
}

func navigationEdgesOfKind(projection *NavigationProjection, kind string) []NavigationEdge {
	var edges []NavigationEdge
	for _, edge := range projection.Edges {
		if edge.Kind == kind {
			edges = append(edges, edge)
		}
	}
	return edges
}

func navigationEdgeByID(t *testing.T, projection *NavigationProjection, id SemanticID) NavigationEdge {
	t.Helper()
	for _, edge := range projection.Edges {
		if edge.ID == id {
			return edge
		}
	}
	t.Fatalf("navigation edge %s not found", id)
	return NavigationEdge{}
}

func navigationEdgeMap(projection *NavigationProjection) map[SemanticID]NavigationEdge {
	edges := make(map[SemanticID]NavigationEdge, len(projection.Edges))
	for _, edge := range projection.Edges {
		edges[edge.ID] = edge
	}
	return edges
}
