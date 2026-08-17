package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEmailVerifiedMembershipSurfaceResolvesCanonicalIntent(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "email-verified-membership.forma")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := NewSourceFile("examples/email-verified-membership.forma", string(content))
	program, parseDiagnostics := parse(source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics:\n%s", diagnosticMessages(parseDiagnostics))
	}
	if len(program.Identities) != 1 || program.Identities[0].Name.Text != "UserAccount" {
		t.Fatalf("parsed identities = %#v", program.Identities)
	}
	result := Compile([]SourceFile{source})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	want, _ := membershipIntentFixture(t)
	if !reflect.DeepEqual(result.Intent, want) {
		actualJSON, _ := MarshalIntent(result.Intent)
		wantJSON, _ := MarshalIntent(want)
		t.Fatalf("surface intent differs from canonical membership fixture\nactual:\n%s\nwant:\n%s", actualJSON, wantJSON)
	}
	if err := ValidateSourceMapCoverage(result.Intent, result.SourceMap); err != nil {
		t.Fatalf("surface Source Map coverage: %v", err)
	}
	if len(result.SourceMap.Entries) != 54 {
		t.Fatalf("surface Source Map entries = %d, want 54", len(result.SourceMap.Entries))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Facts) != 38 {
		t.Fatalf("surface facts = %d, want 38", len(facts.Facts))
	}
	reviews, err := BuildReviewRequirements(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews.Requirements) != 3 {
		t.Fatalf("surface review requirements = %d, want 3", len(reviews.Requirements))
	}
}

func TestIdentitySurfaceRejectsUnsupportedProofAndLifecycle(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "examples", "email-verified-membership.forma"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		from string
		to   string
		code string
		want string
	}{
		{name: "unsupported proof", from: "proof password localPassword", to: "proof password externalAssertion", code: "F2704", want: "unsupported authentication proof"},
		{name: "unknown identifier lifecycle", from: "interact UserAccount.register", to: "interact UserAccount.changeEmail", code: "F2711", want: "unknown identity operation"},
		{name: "owner must bind page parameter", from: "require owner UserAccount.self for user\n\n    detail", to: "require owner UserAccount.self for account\n\n    detail", code: "F2713", want: "owner requirement must bind"},
		{
			name: "duplicate operation interaction",
			from: "page Profile(user User) {",
			to: `page SignInModal {
    interact UserAccount.signin {
        identifier email
        proof password
        success Profile
        feedback generic, failure
    }
}

page Profile(user User) {`,
			code: "F2715", want: "has more than one page interaction",
		},
		{
			name: "missing operation interaction",
			from: `page SignIn {
    interact UserAccount.signin {
        identifier email
        proof password
        success Profile
        feedback generic, failure
    }
}`,
			to: `page SignIn {
}`,
			code: "F2715", want: "has no page interaction",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := string(content)
			source = replaceOnce(t, source, test.from, test.to)
			result := Compile([]SourceFile{NewSourceFile("negative.forma", source)})
			if !hasDiagnostic(result.Diagnostics, test.code, test.want) {
				t.Fatalf("diagnostics:\n%s\nwant %s containing %q", diagnosticMessages(result.Diagnostics), test.code, test.want)
			}
		})
	}
}

func replaceOnce(t *testing.T, source, from, to string) string {
	t.Helper()
	index := -1
	for offset := 0; offset+len(from) <= len(source); offset++ {
		if source[offset:offset+len(from)] == from {
			if index != -1 {
				t.Fatalf("fixture contains %q more than once", from)
			}
			index = offset
		}
	}
	if index == -1 {
		t.Fatalf("fixture does not contain %q", from)
	}
	return source[:index] + to + source[index+len(from):]
}

func hasDiagnostic(diagnostics []Diagnostic, code, message string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && containsText(diagnostic.Message, message) {
			return true
		}
	}
	return false
}

func containsText(value, fragment string) bool {
	if fragment == "" {
		return true
	}
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
