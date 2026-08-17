package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestUsersExampleGoldenIntent(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := Compile([]SourceFile{NewSourceFile("examples/users.forma", string(content))})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	actual, err := MarshalIntent(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	actual = append(actual, '\n')
	goldenPath := filepath.Join("testdata", "users.intent.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("resolved intent differs from %s\nactual:\n%s", goldenPath, actual)
	}

	actualSourceMap, err := MarshalSourceMap(result.SourceMap)
	if err != nil {
		t.Fatal(err)
	}
	actualSourceMap = append(actualSourceMap, '\n')
	sourceMapGoldenPath := filepath.Join("testdata", "users.sourcemap.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(sourceMapGoldenPath, actualSourceMap, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expectedSourceMap, err := os.ReadFile(sourceMapGoldenPath)
	if err != nil {
		t.Fatalf("read source map golden file: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(actualSourceMap, expectedSourceMap) {
		t.Fatalf("source map differs from %s\nactual:\n%s", sourceMapGoldenPath, actualSourceMap)
	}
	if err := ValidateSourceMapCoverage(result.Intent, result.SourceMap); err != nil {
		t.Fatalf("Source Map does not cover Resolved Intent one-to-one: %v", err)
	}
}

func TestResolvedIntentExcludesProfileMechanisms(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := Compile([]SourceFile{NewSourceFile("examples/users.forma", string(content))})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	encoded, err := MarshalIntent(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"relationChoices"`, `"tieBreak"`, `"preventDuplicateDispatch"`,
		`"failureFeedback"`, `"recheckAccess"`, `"loading"`, `"ready"`, `"pending"`,
	} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Errorf("Resolved Intent contains profile mechanism %s", forbidden)
		}
	}
}

func TestSemanticDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		codes  []string
	}{
		{
			name: "relationship requires label",
			source: `entity Team {
    name String required
}
entity User {
    team Team
}
page Users {
    list User {
        columns team
    }
}
`,
			codes: []string{"F2203"},
		},
		{
			name: "invalid transition state",
			source: `entity User {
    state status Pending | Active initial Pending
}
action User.activate: Pending -> Missing
`,
			codes: []string{"F2301"},
		},
		{
			name: "standard action collision",
			source: `entity User {
    state status Pending | Active initial Pending
}
action User.delete: Pending -> Active
`,
			codes: []string{"F2302"},
		},
		{
			name: "unknown list field",
			source: `entity User {
    name String
}
page Users {
    list User {
        columns missing
    }
}
`,
			codes: []string{"F2402"},
		},
		{
			name: "state sort is forbidden",
			source: `entity User {
    state status Pending | Active initial Pending
}
page Users {
    list User {
        sort status asc
    }
}
`,
			codes: []string{"F2404"},
		},
		{
			name: "initial state must be declared",
			source: `entity User {
    state status Pending | Active initial Missing
}
`,
			codes: []string{"F2201"},
		},
		{
			name: "Date default is unavailable",
			source: `entity Event {
    startsOn Date default "2026-08-14"
}
`,
			codes: []string{"F2205"},
		},
		{
			name: "standard action destination is unresolved",
			source: `entity User {
    name String
}
page Users {
    list User {
        actions view
    }
}
`,
			codes: []string{"F2501"},
		},
		{
			name: "create form includes required fields",
			source: `entity User {
    name String required
    email String required
}
page UserCreate {
    form User {
        fields name
        submit create
    }
}
`,
			codes: []string{"F2405"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Compile([]SourceFile{NewSourceFile("test.forma", test.source)})
			actual := diagnosticCodes(result.Diagnostics)
			for _, code := range test.codes {
				if !slices.Contains(actual, code) {
					t.Fatalf("missing diagnostic %s; got %v\n%s", code, actual, diagnosticMessages(result.Diagnostics))
				}
			}
		})
	}
}

func TestContextualKeywordActionName(t *testing.T) {
	source := `entity User {
    state status Pending | Confirmed initial Pending
}
action User.confirm: Pending -> Confirmed confirm
page Users {
    list User {
        actions confirm
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("contextual.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("contextual keyword should be valid:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestStateRequiresExplicitInitialValue(t *testing.T) {
	source := `entity User {
    state status Pending | Active
}
`
	result := Compile([]SourceFile{NewSourceFile("missing-initial.forma", source)})
	if !slices.Contains(diagnosticCodes(result.Diagnostics), "F1002") {
		t.Fatalf("missing explicit initial state should be rejected:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestStateInitialValueIsIndependentOfPresentationOrder(t *testing.T) {
	source := `entity User {
    state status Pending | Active initial Active
}
`
	result := Compile([]SourceFile{NewSourceFile("explicit-initial.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	if result.Intent == nil || len(result.Intent.Entities) != 1 || result.Intent.Entities[0].State == nil {
		t.Fatal("expected one entity with state in Resolved Intent")
	}
	if got := result.Intent.Entities[0].State.Initial; got != "Active" {
		t.Fatalf("initial state = %q, want Active", got)
	}
}

func TestInvariantBuildsResolvedIRAndSourceMap(t *testing.T) {
	source := `type Quantity = Int min 0
entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}
`
	result := Compile([]SourceFile{NewSourceFile("stock.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	if len(result.Intent.Entities) != 1 || len(result.Intent.Entities[0].Invariants) != 1 {
		t.Fatalf("expected one resolved invariant: %#v", result.Intent.Entities)
	}

	invariant := result.Intent.Entities[0].Invariants[0]
	wantInvariantID := invariantID("StockItem", "stockAvailable")
	if invariant.ID != wantInvariantID || invariant.Name != "stockAvailable" {
		t.Fatalf("unexpected invariant identity: %#v", invariant)
	}
	predicate := invariant.Predicate
	if predicate.ID != semanticID(string(wantInvariantID), "expression") || predicate.Kind != "binary-expression" || predicate.Operator != "less-than-or-equal" || predicate.ResultType != "Bool" {
		t.Fatalf("unexpected invariant predicate: %#v", predicate)
	}
	if predicate.Left == nil || predicate.Right == nil {
		t.Fatalf("invariant predicate operands are missing: %#v", predicate)
	}
	if predicate.Left.Kind != "field-reference" || predicate.Left.Binding != "self" || predicate.Left.ResultType != "Quantity" || predicate.Left.Field != semanticID("entity", "StockItem", "field", "reserved") {
		t.Fatalf("unexpected left field reference: %#v", predicate.Left)
	}
	if predicate.Right.Kind != "field-reference" || predicate.Right.Binding != "self" || predicate.Right.ResultType != "Quantity" || predicate.Right.Field != semanticID("entity", "StockItem", "field", "onHand") {
		t.Fatalf("unexpected right field reference: %#v", predicate.Right)
	}

	entries := map[SemanticID]SourceMapEntry{}
	for _, entry := range result.SourceMap.Entries {
		if _, exists := entries[entry.NodeID]; exists {
			t.Fatalf("duplicate Source Map identity %q", entry.NodeID)
		}
		entries[entry.NodeID] = entry
	}
	wantKinds := map[SemanticID]string{
		wantInvariantID:                           "invariant",
		predicate.ID:                              "binary-expression",
		semanticID(string(predicate.ID), "left"):  "field-reference",
		semanticID(string(predicate.ID), "right"): "field-reference",
	}
	for id, kind := range wantKinds {
		entry, exists := entries[id]
		if !exists {
			t.Errorf("Source Map is missing %q", id)
			continue
		}
		if entry.Kind != kind || entry.Span.Start.Line != 6 {
			t.Errorf("Source Map entry %q = %#v, want kind %q on line 6", id, entry, kind)
		}
	}

	ids := semanticIDs(result.Intent)
	if len(ids) != len(result.SourceMap.Entries) {
		t.Fatalf("Resolved Intent has %d semantic nodes but Source Map has %d entries", len(ids), len(result.SourceMap.Entries))
	}
	for _, id := range ids {
		if _, exists := entries[id]; !exists {
			t.Errorf("Resolved Intent identity %q is missing from Source Map", id)
		}
	}
}

func TestInvariantDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		code    string
		message string
	}{
		{
			name: "duplicate name",
			source: `type Quantity = Int
entity StockItem {
    onHand Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
    invariant stockAvailable: onHand <= reserved
}
`,
			code: "F2601", message: "duplicate invariant",
		},
		{
			name: "unknown field",
			source: `type Quantity = Int
entity StockItem {
    onHand Quantity required
    invariant stockAvailable: missing <= onHand
}
`,
			code: "F2602", message: "unknown field or state",
		},
		{
			name: "relation traversal",
			source: `type Quantity = Int
entity Product {
    onHand Quantity required
}
entity OrderLine {
    product Product required
    quantity Quantity required
    invariant withinStock: quantity <= product.onHand
}
`,
			code: "F2603", message: "relation traversal is not allowed",
		},
		{
			name: "optional field",
			source: `type Quantity = Int
entity StockItem {
    onHand Quantity required
    reserved Quantity
    invariant stockAvailable: reserved <= onHand
}
`,
			code: "F2604", message: "optional field",
		},
		{
			name: "different nominal types",
			source: `type Quantity = Int
type Capacity = Int
entity StockItem {
    onHand Capacity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
}
`,
			code: "F2605", message: "cannot compare `Quantity` and `Capacity`",
		},
		{
			name: "unordered fields",
			source: `entity User {
    firstName String required
    lastName String required
    invariant nameOrder: firstName <= lastName
}
`,
			code: "F2605", message: "requires ordered scalar fields",
		},
		{
			name: "state is not ordered",
			source: `entity StockItem {
    onHand Int required
    state status Low | Ready initial Low
    invariant stateOrder: status <= onHand
}
`,
			code: "F2605", message: "cannot compare `status` and `Int`",
		},
		{
			name: "to-one relation is not scalar",
			source: `entity Product {
    onHand Int required
}
entity OrderLine {
    product Product required
    onHand Int required
    invariant relationOrder: product <= onHand
}
`,
			code: "F2605", message: "has non-scalar type `Product`",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Compile([]SourceFile{NewSourceFile("invariant.forma", test.source)})
			if !slices.Contains(diagnosticCodes(result.Diagnostics), test.code) {
				t.Fatalf("missing diagnostic %s:\n%s", test.code, diagnosticMessages(result.Diagnostics))
			}
			if !strings.Contains(diagnosticMessages(result.Diagnostics), test.message) {
				t.Fatalf("diagnostics do not contain %q:\n%s", test.message, diagnosticMessages(result.Diagnostics))
			}
		})
	}
}

func TestChainedInvariantComparisonHasDedicatedDiagnostic(t *testing.T) {
	source := `entity Range {
    minimum Int required
    value Int required
    maximum Int required
    invariant bounded: minimum <= value <= maximum
}
`
	result := Compile([]SourceFile{NewSourceFile("chained-comparison.forma", source)})
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code != "F1003" {
			continue
		}
		if diagnostic.Message != "comparison operators cannot be chained" {
			t.Fatalf("unexpected chained-comparison message: %#v", diagnostic)
		}
		if !strings.Contains(diagnostic.Hint, "two named invariants") || !strings.Contains(diagnostic.Hint, "`and` is not implemented") {
			t.Fatalf("unexpected chained-comparison hint: %#v", diagnostic)
		}
		return
	}
	t.Fatalf("missing dedicated chained-comparison diagnostic:\n%s", diagnosticMessages(result.Diagnostics))
}

func TestInvariantCollectionIsNotReportedAsOptional(t *testing.T) {
	source := `entity OrderLine {
    quantity Int required
}
entity Order {
    lines [OrderLine]
    total Int required
    invariant nonEmpty: lines <= total
}
`
	result := Compile([]SourceFile{NewSourceFile("collection-invariant.forma", source)})
	if !slices.Contains(diagnosticCodes(result.Diagnostics), "F2605") {
		t.Fatalf("collection operand should be rejected as non-scalar:\n%s", diagnosticMessages(result.Diagnostics))
	}
	if slices.Contains(diagnosticCodes(result.Diagnostics), "F2604") {
		t.Fatalf("collection operand must not be reported as optional:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestInvariantSupportsBuiltInOrderedScalarTypes(t *testing.T) {
	source := `entity Measurement {
    intLeft Int required
    intRight Int required
    decimalLeft Decimal required
    decimalRight Decimal required
    dateLeft Date required
    dateRight Date required
    dateTimeLeft DateTime required
    dateTimeRight DateTime required
    invariant intOrder: intLeft <= intRight
    invariant decimalOrder: decimalLeft <= decimalRight
    invariant dateOrder: dateLeft <= dateRight
    invariant dateTimeOrder: dateTimeLeft <= dateTimeRight
}
`
	result := Compile([]SourceFile{NewSourceFile("ordered-scalars.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("ordered built-in scalar types should be valid:\n%s", diagnosticMessages(result.Diagnostics))
	}
	if len(result.Intent.Entities) != 1 || len(result.Intent.Entities[0].Invariants) != 4 {
		t.Fatalf("expected four resolved invariants: %#v", result.Intent.Entities)
	}
}

func TestInvariantKeywordRemainsContextualForScalarAndCollectionFields(t *testing.T) {
	source := `entity Rule {
    invariant Bool required
}
entity RuleSet {
    invariant [Rule]
}
`
	result := Compile([]SourceFile{NewSourceFile("contextual-invariant.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("contextual field names should remain valid:\n%s", diagnosticMessages(result.Diagnostics))
	}
	if len(result.Intent.Entities) != 2 {
		t.Fatalf("expected two entities: %#v", result.Intent.Entities)
	}
	for _, entity := range result.Intent.Entities {
		if len(entity.Fields) != 1 || entity.Fields[0].Name != "invariant" {
			t.Fatalf("field named invariant was not preserved: %#v", entity)
		}
		if entity.Name == "RuleSet" && !entity.Fields[0].Collection {
			t.Fatalf("collection field named invariant was not preserved: %#v", entity.Fields[0])
		}
	}
}

func TestInvariantDeclarationOrderDoesNotChangeIR(t *testing.T) {
	firstSource := `type Quantity = Int
entity StockItem {
    onHand Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
    invariant reservationBounded: reserved <= onHand
}
`
	secondSource := `type Quantity = Int
entity StockItem {
    onHand Quantity required
    reserved Quantity required
    invariant reservationBounded: reserved <= onHand
    invariant stockAvailable: reserved <= onHand
}
`
	first := Compile([]SourceFile{NewSourceFile("first.forma", firstSource)})
	second := Compile([]SourceFile{NewSourceFile("second.forma", secondSource)})
	if len(first.Diagnostics) != 0 || len(second.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %s %s", diagnosticMessages(first.Diagnostics), diagnosticMessages(second.Diagnostics))
	}
	firstJSON, _ := MarshalIntent(first.Intent)
	secondJSON, _ := MarshalIntent(second.Intent)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("invariant declaration order changed Resolved Intent\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
}

func TestCrossFileResolution(t *testing.T) {
	sources := []SourceFile{
		NewSourceFile("domain.forma", `type Email = String matches /.+@.+/
entity User {
    email Email required
}
`),
		NewSourceFile("pages.forma", `page Users {
    list User {
        search email
    }
}
`),
	}
	result := Compile(sources)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("cross-file program should resolve:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestSourceOrderDoesNotChangeIR(t *testing.T) {
	sources := []SourceFile{
		NewSourceFile("b.forma", "page Users {\n    list User\n}\n"),
		NewSourceFile("a.forma", "entity User {\n    name String\n}\n"),
	}
	forward := Compile(sources)
	reverse := Compile([]SourceFile{sources[1], sources[0]})
	if len(forward.Diagnostics) != 0 || len(reverse.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %s %s", diagnosticMessages(forward.Diagnostics), diagnosticMessages(reverse.Diagnostics))
	}
	forwardJSON, _ := MarshalIntent(forward.Intent)
	reverseJSON, _ := MarshalIntent(reverse.Intent)
	if !bytes.Equal(forwardJSON, reverseJSON) {
		t.Fatalf("source argument order changed Resolved Intent\nforward:\n%s\nreverse:\n%s", forwardJSON, reverseJSON)
	}
}

func TestSourcePathsAndBlankLinesDoNotChangeSemanticIdentity(t *testing.T) {
	compact := []SourceFile{
		NewSourceFile("a.forma", "entity Alpha {\n    name String\n}\npage Alphas {\n    list Alpha\n}\n"),
		NewSourceFile("z.forma", "entity Zeta {\n    name String\n}\npage Zetas {\n    list Zeta\n}\n"),
	}
	moved := []SourceFile{
		NewSourceFile("a.forma", "// moved between files\n\nentity Zeta {\n\n    name String\n}\n\npage Zetas {\n    list Zeta\n}\n"),
		NewSourceFile("z.forma", "entity Alpha {\n    name String\n}\n\npage Alphas {\n    list Alpha\n}\n"),
	}
	first := Compile(compact)
	second := Compile(moved)
	if len(first.Diagnostics) != 0 || len(second.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %s %s", diagnosticMessages(first.Diagnostics), diagnosticMessages(second.Diagnostics))
	}
	firstJSON, _ := MarshalIntent(first.Intent)
	secondJSON, _ := MarshalIntent(second.Intent)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("file moves or blank lines changed Resolved Intent\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	if first.SourceMap.Entries[0].Span == second.SourceMap.Entries[0].Span {
		t.Fatal("source positions should remain separate from stable semantic identity")
	}
}

func TestFormSubmitIntentResolvesNavigationPolicies(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		wantKind     string
		wantPage     string
		wantFallback string
	}{
		{
			name: "fixed detail",
			source: `entity User {
    name String
}
page UserCreate {
    form User
}
page UserDetail(user User) {
    detail user
}
`,
			wantKind: "page",
			wantPage: "UserDetail",
		},
		{
			name: "caller list with direct navigation fallback",
			source: `entity User {
    name String
}
page Users {
    list User {
        actions create
    }
}
page UserCreate {
    form User
}
`,
			wantKind:     "caller-list",
			wantFallback: "UserCreate",
		},
		{
			name: "same context",
			source: `entity User {
    name String
}
page UserCreate {
    form User
}
`,
			wantKind: "same-context",
			wantPage: "UserCreate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Compile([]SourceFile{NewSourceFile("test.forma", test.source)})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
			}
			var submit *IRSubmitIntent
			for _, page := range result.Intent.Pages {
				for _, view := range page.Views {
					if view.Kind == "form" {
						submit = view.Submit
					}
				}
			}
			if submit == nil {
				t.Fatal("form has no SubmitIntent")
			}
			if submit.Action != "create" || submit.Success.Kind != test.wantKind || submit.Success.Page != test.wantPage || submit.Success.FallbackPage != test.wantFallback {
				t.Fatalf("unexpected SubmitIntent: %#v", submit)
			}
		})
	}
}

func TestSubmitIntentComposesSourceAndDestinationAccess(t *testing.T) {
	source := `role editor
role member
entity User {
    name String
}
page UserCreate {
    allow editor
    form User
}
page UserDetail(user User) {
    allow member
    detail user
}
`
	result := Compile([]SourceFile{NewSourceFile("access.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	var access IRAccess
	for _, page := range result.Intent.Pages {
		for _, view := range page.Views {
			if view.Submit != nil {
				access = view.Submit.Access
			}
		}
	}
	if len(access.AllOf) != 2 {
		t.Fatalf("submit access has %d clauses, want 2: %#v", len(access.AllOf), access)
	}
	if access.AllOf[0].Source != pageID("UserCreate") || !slices.Equal(access.AllOf[0].AnyOf, []string{"editor"}) {
		t.Fatalf("unexpected source access clause: %#v", access.AllOf[0])
	}
	if access.AllOf[1].Source != pageID("UserDetail") || !slices.Equal(access.AllOf[1].AnyOf, []string{"member"}) {
		t.Fatalf("unexpected destination access clause: %#v", access.AllOf[1])
	}
}

func TestSourceMapTracksStableActionAndSubmitIDs(t *testing.T) {
	source := `entity User {
    state status Pending | Active initial Pending
}
action User.activate: Pending -> Active
page UserCreate {
    form User {
        submit create
    }
}
`
	result := Compile([]SourceFile{NewSourceFile("flow.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	entries := map[SemanticID]SourceMapEntry{}
	for _, entry := range result.SourceMap.Entries {
		if _, exists := entries[entry.NodeID]; exists {
			t.Fatalf("duplicate Source Map identity %q", entry.NodeID)
		}
		entries[entry.NodeID] = entry
	}
	actionEntry, ok := entries[actionID("User", "activate")]
	if !ok || actionEntry.Span.Start.Line != 4 {
		t.Fatalf("action Source Map entry = %#v, present %v", actionEntry, ok)
	}
	submitID := semanticID("page", "UserCreate", "view", "form", "create", "User", "submit")
	submitEntry, ok := entries[submitID]
	if !ok || submitEntry.Span.Start.Line != 7 {
		t.Fatalf("submit Source Map entry = %#v, present %v", submitEntry, ok)
	}
}

func TestSourceMapCoversEveryIRNode(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := Compile([]SourceFile{NewSourceFile("examples/users.forma", string(content))})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	mapped := map[SemanticID]bool{}
	for _, entry := range result.SourceMap.Entries {
		mapped[entry.NodeID] = true
	}
	ids := semanticIDs(result.Intent)
	if len(ids) != len(mapped) {
		t.Fatalf("Resolved Intent has %d semantic nodes but Source Map has %d entries", len(ids), len(mapped))
	}
	seen := map[SemanticID]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate Resolved Intent identity %q", id)
		}
		seen[id] = true
		if !mapped[id] {
			t.Errorf("Resolved Intent identity %q is missing from Source Map", id)
		}
	}
}

func TestDuplicateSemanticViewIdentityIsRejected(t *testing.T) {
	source := `entity User {
    name String
}
page Users {
    list User
    list User
}
`
	result := Compile([]SourceFile{NewSourceFile("duplicate.forma", source)})
	if !slices.Contains(diagnosticCodes(result.Diagnostics), "F2001") {
		t.Fatalf("duplicate semantic views should be rejected:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestSyntaxDiagnosticHasStableLocation(t *testing.T) {
	source := "entity User {\n    name String;\n}\n"
	result := Compile([]SourceFile{NewSourceFile("broken.forma", source)})
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected a syntax diagnostic")
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Code != "F0001" || diagnostic.Span.Start.Line != 2 || diagnostic.Span.Start.Column != 16 {
		t.Fatalf("unexpected diagnostic: %#v", diagnostic)
	}
}

func diagnosticCodes(diagnostics []Diagnostic) []string {
	codes := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	return codes
}

func diagnosticMessages(diagnostics []Diagnostic) string {
	var messages []string
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Error())
	}
	return strings.Join(messages, "\n")
}

func semanticIDs(ir *ResolvedIntent) []SemanticID {
	var ids []SemanticID
	for _, role := range ir.Roles {
		ids = append(ids, role.ID)
	}
	for _, item := range ir.Types {
		ids = append(ids, item.ID)
		for _, constraint := range item.Constraints {
			ids = append(ids, constraint.ID)
		}
	}
	for _, entity := range ir.Entities {
		ids = append(ids, entity.ID)
		for _, field := range entity.Fields {
			ids = append(ids, field.ID)
			if field.Relation != nil {
				ids = append(ids, field.Relation.ID)
			}
		}
		if entity.State != nil {
			ids = append(ids, entity.State.ID)
		}
		for _, invariant := range entity.Invariants {
			ids = append(ids, invariant.ID)
			ids = appendExpressionSemanticIDs(ids, invariant.Predicate)
		}
	}
	for _, action := range ir.Actions {
		ids = append(ids, action.ID)
	}
	for _, page := range ir.Pages {
		ids = append(ids, page.ID)
		if page.Param != nil {
			ids = append(ids, page.Param.ID)
		}
		for _, view := range page.Views {
			ids = append(ids, view.ID)
			if view.Sort != nil {
				ids = append(ids, view.Sort.ID)
			}
			for _, action := range view.Actions {
				ids = append(ids, action.ID, action.Access.ID)
			}
			if view.Submit != nil {
				ids = append(ids, view.Submit.ID, view.Submit.Success.ID, view.Submit.Access.ID)
			}
		}
	}
	return ids
}

func appendExpressionSemanticIDs(ids []SemanticID, expression IRExpression) []SemanticID {
	ids = append(ids, expression.ID)
	if expression.Left != nil {
		ids = appendExpressionSemanticIDs(ids, *expression.Left)
	}
	if expression.Right != nil {
		ids = appendExpressionSemanticIDs(ids, *expression.Right)
	}
	return ids
}
