package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestMembershipIntentFixtureGolden(t *testing.T) {
	intent, sourceMap := membershipIntentFixture(t)
	actual, err := MarshalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	actual = append(actual, '\n')
	assertIdentityGolden(t, filepath.Join("testdata", "membership.intent.json"), actual)

	actualSourceMap, err := MarshalSourceMap(sourceMap)
	if err != nil {
		t.Fatal(err)
	}
	actualSourceMap = append(actualSourceMap, '\n')
	assertIdentityGolden(t, filepath.Join("testdata", "membership.sourcemap.json"), actualSourceMap)

	if err := ValidateSourceMapCoverage(intent, sourceMap); err != nil {
		t.Fatal(err)
	}
}

func TestMembershipIntentCanonicalOrderAndStableIDs(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	if got := intent.Identities[0].ID; got != "identity/UserAccount" {
		t.Fatalf("identity ID = %s", got)
	}
	wantPages := []SemanticID{
		"page/CheckEmail", "page/Profile", "page/ProfileEdit", "page/RegistrationComplete", "page/SignIn", "page/SignUp", "page/VerifyEmail",
	}
	var gotPages []SemanticID
	for _, page := range intent.Pages {
		gotPages = append(gotPages, page.ID)
	}
	if !reflect.DeepEqual(gotPages, wantPages) {
		t.Fatalf("page order = %v, want %v", gotPages, wantPages)
	}
	registration := intent.Identities[0].Registration
	if want := []string{"credential-binding", "notice-emission-record", "subject", "verification-evidence"}; !reflect.DeepEqual(registration.AtomicOutcome, want) {
		t.Fatalf("atomic outcome = %v, want %v", registration.AtomicOutcome, want)
	}
	if got := identityInteractionID("SignUp", "register", "UserAccount"); got != "page/SignUp/identity/register/UserAccount" {
		t.Fatalf("interaction ID = %s", got)
	}
}

func TestMembershipIntentRejectsDuplicateSemanticID(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	intent.Identities[0].Credentials[0].ID = intent.Identities[0].Identifiers[0].ID
	if err := ValidateResolvedIntent(intent); err == nil || !strings.Contains(err.Error(), "duplicate semantic node") {
		t.Fatalf("duplicate validation error = %v", err)
	}
}

func TestSourceMapCoverageReportsAllMissingNodesInCanonicalOrder(t *testing.T) {
	intent, sourceMap := membershipIntentFixture(t)
	first := sourceMap.Entries[0].NodeID
	second := sourceMap.Entries[1].NodeID
	sourceMap.Entries = append([]SourceMapEntry(nil), sourceMap.Entries[2:]...)
	want := "validate Source Map: nodes " + string(first) + ", " + string(second) + " have no source entries"
	if err := ValidateSourceMapCoverage(intent, sourceMap); err == nil || err.Error() != want {
		t.Fatalf("coverage validation error = %v, want %q", err, want)
	}
}

func TestMembershipIntentReferenceIntegrity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ResolvedIntent)
		want   string
	}{
		{
			name: "missing subject",
			mutate: func(intent *ResolvedIntent) {
				intent.Identities[0].Subject = "entity/Missing"
			},
			want: "missing subject",
		},
		{
			name: "optional identifier",
			mutate: func(intent *ResolvedIntent) {
				intent.Entities[0].Fields[2].Required = false
			},
			want: "must be required, unique, and non-collection",
		},
		{
			name: "unknown registration attribute",
			mutate: func(intent *ResolvedIntent) {
				intent.Identities[0].Registration.Attributes = []SemanticID{"entity/User/field/missing"}
			},
			want: "invalid attribute",
		},
		{
			name: "invalid success transition",
			mutate: func(intent *ResolvedIntent) {
				intent.Actions[0].Destination = "Suspended"
			},
			want: "must transition Pending to Active",
		},
		{
			name: "missing interaction operation",
			mutate: func(intent *ResolvedIntent) {
				intent.Pages[5].IdentityInteractions[0].Operation = "identity/UserAccount/operation/missing"
			},
			want: "references missing operation",
		},
		{
			name: "ownership binding entity mismatch",
			mutate: func(intent *ResolvedIntent) {
				intent.Pages[1].Param.Entity = "Other"
			},
			want: "invalid ownership requirement",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent, _ := membershipIntentFixture(t)
			test.mutate(intent)
			err := ValidateResolvedIntent(intent)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestMembershipIntentContainsNoImplementationOrSecretValues(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	content, err := MarshalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"passwordValue"`, `"hash"`, `"salt"`, `"token"`, `"cookie"`, `"header"`, `"route"`, `"httpMethod"`, `"database"`,
	} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Errorf("membership intent contains forbidden implementation or secret field %s", forbidden)
		}
	}
}

func membershipIntentFixture(t *testing.T) (*ResolvedIntent, *SourceMap) {
	t.Helper()
	userID := entityID("User")
	nameField := semanticID(string(userID), "field", "name")
	nicknameField := semanticID(string(userID), "field", "nickname")
	emailField := semanticID(string(userID), "field", "email")
	stateID := semanticID(string(userID), "state", "status")
	accountID := identityID("UserAccount")
	emailIdentifier := identifierID("UserAccount", "email")
	passwordCredential := credentialID("UserAccount", "password")
	registerOperation := identityOperationID("UserAccount", "register")
	verifyOperation := identityOperationID("UserAccount", "verify")
	resendOperation := identityOperationID("UserAccount", "resend")
	signInOperation := identityOperationID("UserAccount", "signin")
	signOutOperation := identityOperationID("UserAccount", "signout")
	emailVerification := verificationID("UserAccount", "email")
	selfOwnership := ownershipID("UserAccount", "self")

	identity := IRIdentity{
		ID: accountID, Name: "UserAccount", Subject: userID,
		Identifiers: []IRIdentifier{{
			ID: emailIdentifier, Name: "email", Field: emailField,
			Canonicalization: []IRCanonicalizationStep{{Kind: "trim-unicode-white-space"}, {Kind: "ascii-case-fold"}},
		}},
		Credentials: []IRCredential{{
			ID: passwordCredential, Name: "password", Kind: "password",
			InputPolicy: IRCredentialInputPolicy{
				PreserveWhitespace: true,
				Length:             IRLengthConstraint{Min: 12, Max: 128, Unit: "unicode-scalar-value"},
			},
		}},
		Registration: IRRegistration{
			ID: registerOperation, Identifier: emailIdentifier, Credential: passwordCredential,
			Attributes: []SemanticID{nameField}, InitialState: IRStateValueRef{State: stateID, Value: "Pending"},
			Verification:              emailVerification,
			AtomicOutcome:             []string{"verification-evidence", "subject", "credential-binding", "notice-emission-record"},
			ExistingIdentifierOutcome: "reject-and-guide-resend",
		},
		Verifications: []IRVerification{{
			ID: emailVerification, Kind: "opaque-email-link", Subject: userID,
			VerifyOperation: verifyOperation, ResendOperation: resendOperation,
			EligibleState: IRStateValueRef{State: stateID, Value: "Pending"}, SuccessAction: actionID("User", "activate"),
			Evidence: IRVerificationEvidence{
				Kind: "opaque", Lifetime: IRDuration{Amount: 30, Unit: "minute"},
				ValidBoundary: "now-before-issued-plus-lifetime", MaxUses: 1, Rotation: "invalidate-prior-unconsumed",
			},
			Notice: IRVerificationNotice{
				ID: verificationNoticeID("UserAccount", "email"), Channel: "email", Recipient: emailIdentifier,
				Emission: "durable-record-required", DeliveryFailure: "subject-remains-pending-and-retryable",
			},
			ResendDisclosure: "uniform-for-pending-active-and-unknown",
		}},
		Authentication: IRAuthentication{
			ID: authenticationID("UserAccount"), SignInOperation: signInOperation, SignOutOperation: signOutOperation,
			Identifier: emailIdentifier, Credential: passwordCredential,
			EligibleState: IRStateValueRef{State: stateID, Value: "Active"}, FailureDisclosure: "generic",
			Session: IRSession{ID: sessionID("UserAccount", "current"), PrincipalSubject: userID, SignOutScope: "current-session"},
		},
		Ownership: []IROwnership{{
			ID: selfOwnership, Identity: accountID, Resource: userID,
			Relation: "principal-subject-equals-resource-identity",
		}},
	}

	pages := []IRPage{
		membershipProfileEditPage(accountID, selfOwnership, nameField, nicknameField),
		membershipInteractionPage("SignUp", "register", registerOperation,
			[]IRIdentityInputRef{{Kind: "field", Node: nameField}, {Kind: "identifier", Node: emailIdentifier}, {Kind: "credential", Node: passwordCredential}},
			"page", "CheckEmail", nil, []string{"invalid", "failure"}),
		membershipInteractionPage("VerifyEmail", "verify", verifyOperation,
			[]IRIdentityInputRef{{Kind: "evidence", Node: emailVerification}}, "page", "RegistrationComplete",
			&IRNavigationIntent{ID: semanticID(string(identityInteractionID("VerifyEmail", "verify", "UserAccount")), "continuation"), Kind: "page", Page: "SignIn"},
			[]string{"invalid", "expired", "failure"}),
		membershipProfilePage(accountID, selfOwnership, nameField, nicknameField, emailField, stateID, signOutOperation),
		membershipInteractionPage("CheckEmail", "resend", resendOperation,
			[]IRIdentityInputRef{{Kind: "identifier", Node: emailIdentifier}}, "same-context", "CheckEmail", nil, []string{"uniform", "failure"}),
		{ID: pageID("RegistrationComplete"), Name: "RegistrationComplete"},
		membershipInteractionPage("SignIn", "signin", signInOperation,
			[]IRIdentityInputRef{{Kind: "identifier", Node: emailIdentifier}, {Kind: "credential", Node: passwordCredential}},
			"page", "Profile", nil, []string{"generic", "failure"}),
	}
	intent := &ResolvedIntent{
		Version: ResolvedIntentVersion,
		Types: []IRType{{
			ID: typeID("Email"), Name: "Email", Kind: "scalar", Base: "String",
			Constraints: []IRConstraint{{ID: semanticID(string(typeID("Email")), "constraint", "matches"), Kind: "matches", Value: ".+@.+"}},
		}},
		Entities: []IREntity{{
			ID: userID, Name: "User",
			Fields: []IRField{
				{ID: nameField, Name: "name", Type: "String", Required: true},
				{ID: nicknameField, Name: "nickname", Type: "String"},
				{ID: emailField, Name: "email", Type: "Email", Required: true, Unique: true},
			},
			State: &IRState{ID: stateID, Name: "status", Initial: "Pending", Values: []string{"Pending", "Active", "Suspended"}},
		}},
		Actions: []IRAction{{
			ID: actionID("User", "activate"), Entity: "User", Name: "activate", Sources: []string{"Pending"}, Destination: "Active",
		}},
		Identities: []IRIdentity{identity},
		Pages:      pages,
	}
	CanonicalizeResolvedIntent(intent)
	if err := ValidateResolvedIntent(intent); err != nil {
		t.Fatalf("membership fixture is invalid: %v", err)
	}
	sourceMap := membershipSourceMap(t, intent)
	return intent, sourceMap
}

func membershipInteractionPage(name, operationName string, operation SemanticID, inputs []IRIdentityInputRef, successKind, successPage string, continuation *IRNavigationIntent, feedback []string) IRPage {
	interactionID := identityInteractionID(name, operationName, "UserAccount")
	return IRPage{
		ID: pageID(name), Name: name,
		IdentityInteractions: []IRIdentityInteraction{{
			ID: interactionID, Operation: operation, Inputs: inputs,
			Access:       IRAccess{ID: semanticID(string(interactionID), "access"), AllOf: []IRAccessRequirement{}},
			Success:      IRNavigationIntent{ID: semanticID(string(interactionID), "success"), Kind: successKind, Page: successPage},
			Continuation: continuation, Feedback: feedback,
		}},
	}
}

func membershipProfilePage(identity, ownership, nameField, nicknameField, emailField, stateID, signOutOperation SemanticID) IRPage {
	page := membershipProtectedPage("Profile", identity, ownership)
	page.Views = []IRView{{
		ID: semanticID("page", "Profile", "view", "detail", "User"), Kind: "detail", Entity: "User", Binding: "user", Mode: "read",
		Fields: []string{"name", "nickname", "email", "status"}, InteractionStates: []string{"empty", "failure"},
	}}
	interactionID := identityInteractionID("Profile", "signout", "UserAccount")
	page.IdentityInteractions = []IRIdentityInteraction{{
		ID: interactionID, Operation: signOutOperation,
		Access: IRAccess{ID: semanticID(string(interactionID), "access"), AllOf: []IRAccessRequirement{{
			Source: page.ID, Kind: "authenticated", Identity: identity,
		}}},
		Success:  IRNavigationIntent{ID: semanticID(string(interactionID), "success"), Kind: "page", Page: "SignIn"},
		Feedback: []string{"failure"},
	}}
	return page
}

func membershipProfileEditPage(identity, ownership, nameField, nicknameField SemanticID) IRPage {
	page := membershipProtectedPage("ProfileEdit", identity, ownership)
	viewID := semanticID("page", "ProfileEdit", "view", "form", "edit", "User")
	submitID := semanticID(string(viewID), "submit")
	page.Views = []IRView{{
		ID: viewID, Kind: "form", Entity: "User", Binding: "user", Mode: "edit",
		Fields: []string{"name", "nickname"},
		Submit: &IRSubmitIntent{
			ID: submitID, Action: "edit",
			Success: IRNavigationIntent{ID: semanticID(string(submitID), "success"), Kind: "page", Page: "Profile"},
			Access:  membershipOwnershipAccess(semanticID(string(submitID), "access"), page.ID, identity, ownership, page.Param.ID),
		},
		InteractionStates: []string{"invalid", "failure"},
	}}
	return page
}

func membershipProtectedPage(name string, identity, ownership SemanticID) IRPage {
	page := IRPage{ID: pageID(name), Name: name}
	page.Param = &IRParameter{ID: semanticID(string(page.ID), "parameter"), Name: "user", Entity: "User"}
	access := membershipOwnershipAccess(semanticID(string(page.ID), "access"), page.ID, identity, ownership, page.Param.ID)
	page.Access = &access
	return page
}

func membershipOwnershipAccess(id, source, identity, ownership, binding SemanticID) IRAccess {
	return IRAccess{ID: id, AllOf: []IRAccessRequirement{
		{Source: source, Kind: "ownership", Ownership: ownership, ResourceBinding: binding},
		{Source: source, Kind: "authenticated", Identity: identity},
	}}
}

func membershipSourceMap(t *testing.T, intent *ResolvedIntent) *SourceMap {
	t.Helper()
	ids, err := resolvedIntentSemanticIDs(intent)
	if err != nil {
		t.Fatal(err)
	}
	ordered := make([]SemanticID, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	entries := make([]SourceMapEntry, 0, len(ordered))
	for index, id := range ordered {
		line := index + 1
		entries = append(entries, SourceMapEntry{
			NodeID: id, Kind: membershipNodeKind(id),
			Span: Span{File: "testdata/membership.intent.fixture", Start: Position{Line: line, Column: 1}, End: Position{Line: line, Column: 2}},
		})
	}
	return &SourceMap{Version: SourceMapVersion, IntentVersion: intent.Version, Entries: entries}
}

func membershipNodeKind(id SemanticID) string {
	value := string(id)
	switch {
	case strings.HasPrefix(value, "identity/"):
		switch {
		case strings.Contains(value, "/identifier/"):
			return "identity-identifier"
		case strings.Contains(value, "/credential/"):
			return "identity-credential"
		case strings.Contains(value, "/operation/register"):
			return "identity-registration"
		case strings.Contains(value, "/operation/"):
			return "identity-operation"
		case strings.HasSuffix(value, "/notice"):
			return "identity-verification-notice"
		case strings.Contains(value, "/verification/"):
			return "identity-verification"
		case strings.HasSuffix(value, "/authentication"):
			return "identity-authentication"
		case strings.Contains(value, "/session/"):
			return "identity-session"
		case strings.Contains(value, "/ownership/"):
			return "identity-ownership"
		default:
			return "identity"
		}
	case strings.HasPrefix(value, "entity/"):
		if strings.Contains(value, "/field/") {
			return "field"
		}
		if strings.Contains(value, "/state/") {
			return "state"
		}
		return "entity"
	case strings.HasPrefix(value, "type/"):
		if strings.Contains(value, "/constraint/") {
			return "constraint"
		}
		return "type"
	case strings.HasPrefix(value, "action/"):
		return "action"
	case strings.HasPrefix(value, "page/"):
		switch {
		case strings.HasSuffix(value, "/parameter"):
			return "parameter"
		case strings.HasSuffix(value, "/access"):
			return "access"
		case strings.HasSuffix(value, "/success"), strings.HasSuffix(value, "/continuation"):
			return "navigation"
		case strings.Contains(value, "/identity/"):
			return "identity-interaction"
		case strings.Contains(value, "/submit"):
			return "submit"
		case strings.Contains(value, "/view/detail/"):
			return "detail"
		case strings.Contains(value, "/view/form/"):
			return "form"
		default:
			return "page"
		}
	default:
		return "semantic-node"
	}
}

func assertIdentityGolden(t *testing.T, path string, actual []byte) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("output differs from %s\nactual:\n%s", path, actual)
	}
}
