package compiler

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestIdentityVariantProposalGate(t *testing.T) {
	tests := []struct {
		name        string
		fixture     func(*testing.T) *ResolvedIntent
		want        string
		missingAxis string
	}{
		{
			name:        "passwordless",
			fixture:     passwordlessIdentityFixture,
			want:        "first Identity slice requires one identifier, proof, credential, verification, and ownership",
			missingAxis: "authentication proof independent of a local credential",
		},
		{
			name:        "external provider",
			fixture:     externalProviderIdentityFixture,
			want:        "is not the supported local-password proof",
			missingAxis: "external authority and subject mapping",
		},
		{
			name:        "email change",
			fixture:     emailChangeIdentityFixture,
			want:        "references missing operation identity/UserAccount/operation/change-email",
			missingAxis: "candidate identifier lifecycle and commit operation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := test.fixture(t)
			if err := ValidateResolvedIntent(intent); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s fixture validation = %v, want diagnostic containing %q", test.missingAxis, err, test.want)
			}
			if _, err := BuildAcceptanceFacts(intent); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s fact derivation = %v, want diagnostic containing %q", test.missingAxis, err, test.want)
			}
			if _, err := BuildReviewRequirements(intent); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s review derivation = %v, want diagnostic containing %q", test.missingAxis, err, test.want)
			}
		})
	}
}

func TestPasswordlessFactSubsetUsesGenericValidator(t *testing.T) {
	canonical, _ := membershipIntentFixture(t)
	canonicalFacts, err := BuildAcceptanceFacts(canonical)
	if err != nil {
		t.Fatal(err)
	}
	passwordless := passwordlessIdentityFixture(t)
	credentialID := credentialID("UserAccount", "password")
	filtered := &AcceptanceFacts{Version: AcceptanceFactsVersion, IntentVersion: passwordless.Version}
	for _, fact := range canonicalFacts.Facts {
		content, err := json.Marshal(fact)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(content, []byte(credentialID)) {
			filtered.Facts = append(filtered.Facts, fact)
		}
	}
	if len(filtered.Facts) != 29 || len(canonicalFacts.Facts) != 41 {
		t.Fatalf("passwordless subset = %d of %d facts, want measured B5 boundary 29 of 41", len(filtered.Facts), len(canonicalFacts.Facts))
	}
	t.Logf("passwordless-compatible subset = %d of %d canonical membership facts", len(filtered.Facts), len(canonicalFacts.Facts))
	for _, fact := range filtered.Facts {
		if fact.Kind == "credential-bound" || fact.Kind == "credential-non-projectable" || fact.Kind == "secret-input-not-preserved" {
			t.Fatalf("credential-specific fact survived passwordless subset: %s", fact.Kind)
		}
	}
	// Call the compositional fact validator directly: the end-to-end builder is
	// expected to reject this fixture until authentication proof is generalized.
	if err := ValidateAcceptanceFacts(passwordless, filtered); err != nil {
		t.Fatalf("generic Acceptance Fact validator rejected a supported passwordless subset: %v", err)
	}
}

func TestNewVariantFactKindsRequireExplicitContracts(t *testing.T) {
	base := AcceptanceFact{
		ID:      "fact/identity/variant/probe",
		Subject: "identity/UserAccount/operation/signin",
		Input: &FactInput{Identity: &IdentityFactInput{
			Operation: "identity/UserAccount/operation/signin", Dispatches: 1, Observe: []string{"user-visible-outcome"},
		}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "accepted"}},
		SourceNodes: []SemanticID{"identity/UserAccount/operation/signin"},
	}
	for _, kind := range []string{"external-authentication-accepted", "identifier-change-committed"} {
		t.Run(kind, func(t *testing.T) {
			fact := base
			fact.Kind = kind
			if err := ValidateFactSetup(fact); err == nil || !strings.Contains(err.Error(), "has no contract") {
				t.Fatalf("variant fact contract error = %v", err)
			}
		})
	}
}

func TestEmailChangeCannotMasqueradeAsSecondIdentifier(t *testing.T) {
	intent := clonedMembershipIntent(t)
	identity := &intent.Identities[0]
	candidate := identity.Identifiers[0]
	candidate.Name = "candidate-email"
	candidate.ID = identifierID(identity.Name, candidate.Name)
	identity.Identifiers = append(identity.Identifiers, candidate)
	CanonicalizeResolvedIntent(intent)
	if err := ValidateResolvedIntent(intent); err == nil || !strings.Contains(err.Error(), "requires one identifier") {
		t.Fatalf("candidate identifier fallback error = %v", err)
	}
}

func passwordlessIdentityFixture(t *testing.T) *ResolvedIntent {
	t.Helper()
	intent := clonedMembershipIntent(t)
	identity := &intent.Identities[0]
	identity.Proofs = nil
	identity.Credentials = nil
	identity.Registration.Proof = ""
	identity.Registration.Credential = ""
	identity.Authentication.Proof = ""
	identity.Authentication.Credential = ""
	for pageIndex := range intent.Pages {
		for interactionIndex := range intent.Pages[pageIndex].IdentityInteractions {
			interaction := &intent.Pages[pageIndex].IdentityInteractions[interactionIndex]
			inputs := interaction.Inputs[:0]
			for _, input := range interaction.Inputs {
				if input.Kind != "credential" {
					inputs = append(inputs, input)
				}
			}
			interaction.Inputs = inputs
		}
	}
	CanonicalizeResolvedIntent(intent)
	return intent
}

func externalProviderIdentityFixture(t *testing.T) *ResolvedIntent {
	t.Helper()
	intent := clonedMembershipIntent(t)
	identity := &intent.Identities[0]
	provider := credentialID(identity.Name, "provider")
	providerProof := authenticationProofID(identity.Name, "provider")
	identity.Proofs[0] = IRAuthenticationProof{ID: providerProof, Name: "provider", Kind: "external-assertion"}
	identity.Credentials[0] = IRCredential{ID: provider, Name: "provider", Kind: "external-provider"}
	identity.Registration.Proof = providerProof
	identity.Registration.Credential = provider
	identity.Authentication.Proof = providerProof
	identity.Authentication.Credential = provider
	for pageIndex := range intent.Pages {
		for interactionIndex := range intent.Pages[pageIndex].IdentityInteractions {
			for inputIndex := range intent.Pages[pageIndex].IdentityInteractions[interactionIndex].Inputs {
				input := &intent.Pages[pageIndex].IdentityInteractions[interactionIndex].Inputs[inputIndex]
				if input.Kind == "credential" {
					input.Node = provider
				}
			}
		}
	}
	CanonicalizeResolvedIntent(intent)
	return intent
}

func emailChangeIdentityFixture(t *testing.T) *ResolvedIntent {
	t.Helper()
	intent := clonedMembershipIntent(t)
	identity := intent.Identities[0]
	operation := identityOperationID(identity.Name, "change-email")
	interactionID := identityInteractionID("ChangeEmail", "change-email", identity.Name)
	intent.Pages = append(intent.Pages, IRPage{
		ID: pageID("ChangeEmail"), Name: "ChangeEmail",
		IdentityInteractions: []IRIdentityInteraction{{
			ID: interactionID, Operation: operation,
			Inputs: []IRIdentityInputRef{{Kind: "identifier", Node: identity.Identifiers[0].ID}},
			Access: IRAccess{ID: semanticID(string(interactionID), "access"), AllOf: []IRAccessRequirement{{
				Source: pageID("ChangeEmail"), Kind: "authenticated", Identity: identity.ID,
			}}},
			Success:  IRNavigationIntent{ID: semanticID(string(interactionID), "success"), Kind: "page", Page: "Profile"},
			Feedback: []string{"invalid", "failure"},
		}},
	})
	CanonicalizeResolvedIntent(intent)
	return intent
}

func clonedMembershipIntent(t *testing.T) *ResolvedIntent {
	t.Helper()
	intent, _ := membershipIntentFixture(t)
	content, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	var result ResolvedIntent
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	return &result
}
