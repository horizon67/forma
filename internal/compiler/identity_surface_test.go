package compiler

import (
	"os"
	"path/filepath"
	"strings"
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
	if len(program.Entries) != 1 || program.Entries[0].Page.Text != "SignUp" {
		t.Fatalf("parsed entry = %#v", program.Entries)
	}
	result := Compile([]SourceFile{source})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	if result.Intent.Entry == nil || result.Intent.Entry.ID != applicationEntryID() || result.Intent.Entry.Page != "SignUp" {
		t.Fatalf("resolved entry = %#v", result.Intent.Entry)
	}
	registrationComplete := resolvedPageByName(t, result.Intent, "RegistrationComplete")
	if len(registrationComplete.SurfaceTransitions) != 1 || registrationComplete.SurfaceTransitions[0] != (IRSurfaceTransition{
		ID: surfaceTransitionID("RegistrationComplete", "continue"), Kind: "continue", TargetPage: "OnboardingGuide",
	}) {
		t.Fatalf("RegistrationComplete transitions = %#v", registrationComplete.SurfaceTransitions)
	}
	onboarding := resolvedPageByName(t, result.Intent, "OnboardingGuide")
	if len(onboarding.SurfaceTransitions) != 1 || onboarding.SurfaceTransitions[0].TargetPage != "SignIn" {
		t.Fatalf("OnboardingGuide transitions = %#v", onboarding.SurfaceTransitions)
	}
	verify := resolvedPageByName(t, result.Intent, "VerifyEmail").IdentityInteractions[0]
	if verify.Success.Page != "RegistrationComplete" || verify.Continuation != nil {
		t.Fatalf("verify navigation = success %#v, continuation %#v", verify.Success, verify.Continuation)
	}
	if err := ValidateSourceMapCoverage(result.Intent, result.SourceMap); err != nil {
		t.Fatalf("surface Source Map coverage: %v", err)
	}
	if len(result.SourceMap.Entries) != 57 {
		t.Fatalf("surface Source Map entries = %d, want 57", len(result.SourceMap.Entries))
	}
	facts, err := BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Facts) != 41 {
		t.Fatalf("surface facts = %d, want 41", len(facts.Facts))
	}
	for id, target := range map[SemanticID]SemanticID{
		"fact/application/entry/navigation":                             pageID("SignUp"),
		"fact/page/RegistrationComplete/transition/continue/navigation": pageID("OnboardingGuide"),
		"fact/page/OnboardingGuide/transition/continue/navigation":      pageID("SignIn"),
	} {
		fact := acceptanceFactByID(t, facts, id)
		if fact.Expected.Navigation == nil || fact.Expected.Navigation.TargetPage != target {
			t.Fatalf("fact %s navigation = %#v", id, fact.Expected.Navigation)
		}
	}
	navigation, err := BuildNavigationProjection(result.Intent, result.SourceMap)
	if err != nil {
		t.Fatal(err)
	}
	if navigation.DefaultEntry.Kind != navigationEndpointPage || navigation.DefaultEntry.Page != pageID("SignUp") {
		t.Fatalf("navigation entry = %#v", navigation.DefaultEntry)
	}
	if edge := navigationEdgeByID(t, navigation, surfaceTransitionID("RegistrationComplete", "continue")); edge.Destination != pageID("OnboardingGuide") || edge.Kind != "surface-transition" {
		t.Fatalf("RegistrationComplete edge = %#v", edge)
	}
	navigationText, err := FormatNavigationProjection(navigation)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"default entry: page SignUp", "RegistrationComplete", "-- continue --> OnboardingGuide", "-- continue --> SignIn"} {
		if !strings.Contains(navigationText, want) {
			t.Fatalf("navigation projection omits %q:\n%s", want, navigationText)
		}
	}
	flow, err := BuildFlowProjection(result.Intent, result.SourceMap)
	if err != nil {
		t.Fatalf("flow projection: %v", err)
	}
	flowText, err := FormatFlowProjection(flow)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"default_entry([\"default entry\"])", "RegistrationComplete -> OnboardingGuide", "OnboardingGuide -> SignIn"} {
		if !strings.Contains(flowText, want) {
			t.Fatalf("flow projection omits %q:\n%s", want, flowText)
		}
	}
	reviews, err := BuildReviewRequirements(result.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews.Requirements) != 3 {
		t.Fatalf("surface review requirements = %d, want 3", len(reviews.Requirements))
	}
}

func resolvedPageByName(t *testing.T, intent *ResolvedIntent, name string) IRPage {
	t.Helper()
	for _, page := range intent.Pages {
		if page.Name == name {
			return page
		}
	}
	t.Fatalf("page %s is missing", name)
	return IRPage{}
}

func acceptanceFactByID(t *testing.T, facts *AcceptanceFacts, id SemanticID) AcceptanceFact {
	t.Helper()
	for _, fact := range facts.Facts {
		if fact.ID == id {
			return fact
		}
	}
	t.Fatalf("fact %s is missing", id)
	return AcceptanceFact{}
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
