package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestUsersExampleGoldenIR(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := Compile([]SourceFile{NewSourceFile("examples/users.forma", string(content))})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	actual, err := MarshalIR(result.IR)
	if err != nil {
		t.Fatal(err)
	}
	actual = append(actual, '\n')
	goldenPath := filepath.Join("testdata", "users.ir.json")
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
		t.Fatalf("semantic IR differs from %s\nactual:\n%s", goldenPath, actual)
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
	if result.IR == nil || len(result.IR.Entities) != 1 || result.IR.Entities[0].State == nil {
		t.Fatal("expected one entity with state in Semantic IR")
	}
	if got := result.IR.Entities[0].State.Initial; got != "Active" {
		t.Fatalf("initial state = %q, want Active", got)
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
	forwardJSON, _ := MarshalIR(forward.IR)
	reverseJSON, _ := MarshalIR(reverse.IR)
	if !bytes.Equal(forwardJSON, reverseJSON) {
		t.Fatalf("source argument order changed IR\nforward:\n%s\nreverse:\n%s", forwardJSON, reverseJSON)
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
