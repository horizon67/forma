package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMembershipOutcomeProjectionMatchesGoldenAndKeepsSecurityBoundaries(t *testing.T) {
	path := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "app.forma")
	projection, formatted, sourceMap := outcomeProjectionFromFile(t, path)
	assertOutcomeGolden(t, formatted, filepath.Join("testdata", "membership.outcomes.txt"))

	if count := outcomeRowCount(projection); count != 87 {
		t.Fatalf("outcome rows = %d, want 87", count)
	}
	entries := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		entries[entry.NodeID] = true
	}
	for _, group := range projection.Groups {
		for _, row := range group.Rows {
			if len(row.SourceNodes) == 0 {
				t.Fatalf("row %s has no provenance", row.ID)
			}
			for _, node := range row.SourceNodes {
				if !entries[node] {
					t.Fatalf("row %s source node %s is not traceable", row.ID, node)
				}
			}
		}
	}

	registrationRejected := outcomeRowByFactAndCase(t, projection,
		semanticID("fact", "identity", "UserAccount", "operation", "register", "validation", "rejected"), "invalid-credential")
	if registrationRejected.CaseInput == nil || registrationRejected.CaseInput.Credential == nil || registrationRejected.CaseInput.Credential.Relation != "violates-policy" {
		t.Fatalf("registration case input = %#v", registrationRejected.CaseInput)
	}
	if registrationRejected.Input == nil || registrationRejected.Input.Identity == nil || len(registrationRejected.Input.Identity.Cases) != 0 {
		t.Fatalf("common registration input still embeds cases: %#v", registrationRejected.Input)
	}
	expected, prohibited := summarizeOutcomeRow(registrationRejected)
	assertOutcomeContains(t, expected, "outcome=rejected")
	for _, value := range []string{"subject present", "credential present=UserAccount.password", "evidence present", "notice present"} {
		assertOutcomeContains(t, prohibited, value)
	}
	preserveInput := outcomeRowByFactAndCase(t, projection,
		semanticID("fact", "page", "SignUp", "identity", "register", "UserAccount", "validation", "preserve-input"), "invalid-credential")
	expected, prohibited = summarizeOutcomeRow(preserveInput)
	assertOutcomeContains(t, expected, "outcome=invalid-input-redisplayed")
	assertOutcomeContains(t, expected, "case-outcome=rejected")
	assertOutcomeContains(t, prohibited, "credential included=UserAccount.password")

	duplicate := outcomeRowByFactAndCase(t, projection,
		semanticID("fact", "identity", "UserAccount", "operation", "register", "identifier", "duplicate"), "canonical-equivalent")
	expected, prohibited = summarizeOutcomeRow(duplicate)
	assertOutcomeContains(t, expected, "subject unchanged")
	assertOutcomeContains(t, expected, "credential=UserAccount.password:unchanged")
	assertOutcomeContains(t, prohibited, "evidence added")
	assertOutcomeContains(t, prohibited, "notice added")

	delivery := outcomeRowByFactAndCase(t, projection,
		semanticID("fact", "identity", "UserAccount", "verification", "email", "notice", "delivery", "failure"), "delivery-failure-separated")
	expected, _ = summarizeOutcomeRow(delivery)
	for _, value := range []string{"outcome=retryable", "subject.state=User.status:Pending", "notice.emission=durable-record-required", "notice.delivery=failed"} {
		assertOutcomeContains(t, expected, value)
	}

	signInRejected := outcomeRowByFactAndCase(t, projection,
		semanticID("fact", "identity", "UserAccount", "operation", "signin", "rejected", "generic"), "unknown-identifier")
	expected, prohibited = summarizeOutcomeRow(signInRejected)
	assertOutcomeContains(t, expected, "disclosure=generic")
	assertOutcomeContains(t, prohibited, "session present")

	expiry := outcomeRowByFactAndCase(t, projection,
		semanticID("fact", "identity", "UserAccount", "operation", "verify", "expiry", "boundary"), "at-expiry")
	expected, _ = summarizeOutcomeRow(expiry)
	for _, value := range []string{"outcome=rejected", "subject.state=User.status:Pending", "evidence.condition=issued"} {
		assertOutcomeContains(t, expected, value)
	}
}

func TestAdminOutcomeProjectionMatchesGolden(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	_, formatted, _ := outcomeProjectionFromFile(t, path)
	assertOutcomeGolden(t, formatted, filepath.Join("testdata", "users.outcomes.txt"))
}

func TestOutcomeProjectionIsIndependentOfDeclarationOrderAndSourcePath(t *testing.T) {
	first := `entity User {
    name String required unique
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
	second := `page UserCreate {
    form User {
        fields name
        submit create
    }
}
page Users {
    list User {
        columns name
        actions create
    }
}
entity User {
    name String required unique
}
`
	_, firstText, _ := outcomeProjectionFromSource(t, "first.forma", first)
	_, secondText, _ := outcomeProjectionFromSource(t, "moved.forma", second)
	if firstText != secondText {
		t.Fatalf("declaration order or source path changed outcome projection\nfirst:\n%s\nsecond:\n%s", firstText, secondText)
	}
}

func TestConstraintMutationRemovesExactlyItsOutcomeRow(t *testing.T) {
	beforeSource := `entity User {
    name String required unique
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
	afterSource := strings.Replace(beforeSource, "name String required unique", "name String required", 1)
	before, _, _ := outcomeProjectionFromSource(t, "users.forma", beforeSource)
	after, _, _ := outcomeProjectionFromSource(t, "users.forma", afterSource)
	beforeRows := outcomeRowMap(before)
	afterRows := outcomeRowMap(after)
	uniqueFact := semanticID("fact", "page", "UserCreate", "view", "form", "create", "User", "submit", "validation", "unique", "name")
	uniqueID := semanticID("projection", "outcomes", "row", string(uniqueFact))
	if _, ok := beforeRows[uniqueID]; !ok {
		t.Fatalf("unique outcome row %s is missing before mutation", uniqueID)
	}
	delete(beforeRows, uniqueID)
	if !reflect.DeepEqual(beforeRows, afterRows) {
		t.Fatal("constraint mutation changed unrelated outcome rows")
	}
}

func TestInvariantOutcomeProjectionShowsBothAtomicBoundaries(t *testing.T) {
	projection, formatted, _ := outcomeProjectionFromSource(t, "stock.forma", invariantAcceptanceSource)
	if outcomeRowCount(projection) != 2 {
		t.Fatalf("invariant outcome rows = %d, want 2", outcomeRowCount(projection))
	}
	invariant := invariantID("StockItem", "stockAvailable")
	satisfied := outcomeRowByFactAndCase(t, projection,
		factID(invariant, "evaluation", "satisfied"), "predicate true")
	expected, prohibited := summarizeOutcomeRow(satisfied)
	for _, value := range []string{"outcome=accepted", "enforcement=authoritative", "atomicity=all-changes-committed"} {
		assertOutcomeContains(t, expected, value)
	}
	if len(prohibited) != 0 {
		t.Fatalf("satisfied invariant prohibits %v", prohibited)
	}
	violated := outcomeRowByFactAndCase(t, projection,
		factID(invariant, "evaluation", "violated"), "predicate false")
	expected, prohibited = summarizeOutcomeRow(violated)
	for _, value := range []string{"outcome=rejected", "enforcement=authoritative"} {
		assertOutcomeContains(t, expected, value)
	}
	assertOutcomeContains(t, prohibited, "changes committed")
	for _, want := range []string{
		"case predicate true / invariant-satisfied",
		"case predicate false / invariant-violated",
		"must not: changes committed",
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted invariant outcomes omit %q:\n%s", want, formatted)
		}
	}
}

func outcomeProjectionFromFile(t *testing.T, path string) (*OutcomeProjection, string, *SourceMap) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return outcomeProjectionFromSource(t, filepath.ToSlash(path), string(content))
}

func outcomeProjectionFromSource(t *testing.T, path, source string) (*OutcomeProjection, string, *SourceMap) {
	t.Helper()
	result := Compile([]SourceFile{NewSourceFile(path, source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %#v", result.Diagnostics)
	}
	projection, err := BuildOutcomeProjection(result.Intent, result.SourceMap)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := FormatOutcomeProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	return projection, formatted, result.SourceMap
}

func assertOutcomeGolden(t *testing.T, actual, path string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual != string(expected) {
		t.Fatalf("outcome projection differs from %s\nactual:\n%s", path, actual)
	}
}

func outcomeRowCount(projection *OutcomeProjection) int {
	count := 0
	for _, group := range projection.Groups {
		count += len(group.Rows)
	}
	return count
}

func outcomeRowByFactAndCase(t *testing.T, projection *OutcomeProjection, factID SemanticID, caseName string) OutcomeRow {
	t.Helper()
	for _, group := range projection.Groups {
		for _, row := range group.Rows {
			if row.FactID == factID && row.Case == caseName {
				return row
			}
		}
	}
	t.Fatalf("outcome row %s case %s not found", factID, caseName)
	return OutcomeRow{}
}

func assertOutcomeContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("outcome values %v do not contain %q", values, want)
}

func outcomeRowMap(projection *OutcomeProjection) map[SemanticID]OutcomeRow {
	rows := map[SemanticID]OutcomeRow{}
	for _, group := range projection.Groups {
		for _, row := range group.Rows {
			rows[row.ID] = row
		}
	}
	return rows
}
