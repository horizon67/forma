package compiler

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMembershipIdentityAcceptanceFactsGolden(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	identityFacts := membershipIdentityFacts(facts.Facts)
	if len(identityFacts) != 29 {
		t.Fatalf("Identity fact count = %d, want 29", len(identityFacts))
	}
	wantIDs := []SemanticID{
		"fact/identity/UserAccount/credential/password/non-projectable",
		"fact/identity/UserAccount/operation/register/at-most-once",
		"fact/identity/UserAccount/operation/register/credential/bound",
		"fact/identity/UserAccount/operation/register/identifier/duplicate",
		"fact/identity/UserAccount/operation/register/notice/emitted",
		"fact/identity/UserAccount/operation/register/subject/created",
		"fact/identity/UserAccount/operation/register/validation/rejected",
		"fact/identity/UserAccount/operation/register/verification/issued",
		"fact/identity/UserAccount/operation/resend/accepted",
		"fact/identity/UserAccount/operation/resend/at-most-once",
		"fact/identity/UserAccount/operation/resend/evidence/rotated",
		"fact/identity/UserAccount/operation/signin/accepted",
		"fact/identity/UserAccount/operation/signin/rejected/generic",
		"fact/identity/UserAccount/operation/signin/state/ineligible",
		"fact/identity/UserAccount/operation/signout/session/terminated",
		"fact/identity/UserAccount/operation/verify/accepted",
		"fact/identity/UserAccount/operation/verify/evidence/consumed",
		"fact/identity/UserAccount/operation/verify/evidence/rejected",
		"fact/identity/UserAccount/operation/verify/expiry/boundary",
		"fact/identity/UserAccount/ownership/self/access/allowed/self",
		"fact/identity/UserAccount/ownership/self/access/denied/anonymous",
		"fact/identity/UserAccount/ownership/self/access/denied/other-subject",
		"fact/identity/UserAccount/verification/email/notice/delivery/failure",
		"fact/page/CheckEmail/identity/resend/UserAccount/disclosure/uniform",
		"fact/page/SignUp/identity/register/UserAccount/access/allowed/anonymous",
		"fact/page/SignUp/identity/register/UserAccount/inputs",
		"fact/page/SignUp/identity/register/UserAccount/navigation",
		"fact/page/SignUp/identity/register/UserAccount/validation/preserve-input",
		"fact/page/VerifyEmail/identity/verify/UserAccount/navigation",
	}
	// The canonical list above intentionally catches an extra or missing fact;
	// sort order comes from BuildAcceptanceFacts.
	var gotIDs []SemanticID
	for _, fact := range identityFacts {
		gotIDs = append(gotIDs, fact.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("Identity fact IDs =\n%v\nwant\n%v", gotIDs, wantIDs)
	}
	content, err := MarshalAcceptanceFacts(facts)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, '\n')
	path := filepath.Join("testdata", "membership.facts.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(content, expected) {
		t.Fatalf("Acceptance Facts differ from %s", path)
	}
	second, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	secondContent, err := MarshalAcceptanceFacts(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(content), secondContent) {
		t.Fatal("same Resolved Intent did not produce byte-identical Acceptance Facts")
	}
	for _, forbidden := range []string{
		`"dependsOn"`, `"passwordValue"`, `"tokenValue"`, `"rawValue"`, `"hash"`,
		`"cookie"`, `"header"`, `"httpMethod"`, `"route"`, `"sql"`, `"smtp"`,
	} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Errorf("Acceptance Facts contain forbidden target or secret vocabulary %s", forbidden)
		}
	}
}

func TestIdentityFactKindRegistryExactlyCoversCanonicalKinds(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, fact := range membershipIdentityFacts(facts.Facts) {
		kinds[fact.Kind] = true
	}
	if len(kinds) != 27 || len(identityFactKindContracts) != 27 {
		t.Fatalf("fact kinds = %d, contracts = %d, want 27 each", len(kinds), len(identityFactKindContracts))
	}
	if err := validateFactKindContractCoverage(kinds); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAcceptanceFactsAllowsSupportedIdentityKindSubset(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	facts.Facts = []AcceptanceFact{identityFactByKind(t, facts.Facts, "identity-inputs")}
	if err := ValidateAcceptanceFacts(intent, facts); err != nil {
		t.Fatalf("supported Identity Fact subset was rejected: %v", err)
	}
}

func TestIdentityFactsNeverPreserveOrStoreCredentialInput(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	credential := intent.Identities[0].Credentials[0].ID
	for _, fact := range facts.Facts {
		if containsSemanticID(fact.Expected.PreserveInput, credential) {
			t.Errorf("fact %s preserves Credential %s", fact.ID, credential)
		}
		if fact.Expected.Stored == "input" && fact.Input != nil && containsSemanticID(fact.Input.Fields, credential) {
			t.Errorf("fact %s stores Credential input", fact.ID)
		}
		if fact.Expected.Identity != nil {
			if containsSemanticID(fact.Expected.Identity.PreserveFields, credential) {
				t.Errorf("fact %s preserves Credential as a field", fact.ID)
			}
		}
	}
	preserve := identityFactByKind(t, facts.Facts, "secret-input-not-preserved")
	if !containsSemanticID(preserve.Expected.Identity.ExcludedCredentials, credential) {
		t.Fatalf("secret input fact does not exclude %s", credential)
	}
}

func TestIdentityFactSchemaHasNoCredentialOrEvidenceRawValueSlot(t *testing.T) {
	for _, value := range []any{
		FactCredentialBindingSetup{}, FactEvidenceSetup{}, FactSessionSetup{},
		FactIdentifierInput{}, FactCredentialInput{}, IdentityFactCase{}, IdentityFactInput{},
		FactCredentialExpectation{}, FactEvidenceExpectation{}, FactSessionExpectation{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			name := strings.ToLower(typeOf.Field(index).Name)
			for _, forbidden := range []string{"raw", "password", "token", "secret", "value", "hash", "salt"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("%s exposes forbidden value-bearing field %s", typeOf.Name(), typeOf.Field(index).Name)
				}
			}
		}
	}
}

func TestIdentityFactSetupRejectsSelfFulfillment(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	identity := intent.Identities[0]
	state := *intent.Entities[0].State
	active := IRStateValueRef{State: state.ID, Value: "Active"}

	registration := identityFactByKind(t, facts.Facts, "registration-created")
	registration.Setup = identitySetup(subjectSetup("subject/created", identity.ID, &active))
	if err := ValidateFactSetup(registration); err == nil || !strings.Contains(err.Error(), "pre-installs a fresh registration result") {
		t.Fatalf("registration setup error = %v", err)
	}

	atMostOnce := identityFactByKind(t, facts.Facts, "operation-at-most-once")
	atMostOnce.Input.Identity.Dispatches = 1
	if err := ValidateFactSetup(atMostOnce); err == nil || !strings.Contains(err.Error(), "at least 2 dispatches") {
		t.Fatalf("at-most-once setup error = %v", err)
	}

	authenticated := identityFactByKind(t, facts.Facts, "authentication-accepted")
	authenticated.Setup.Sessions = []FactSessionSetup{{
		Handle: "subject/alice/session/current", Session: identity.Authentication.Session.ID,
		Subject: "subject/alice", Condition: "active",
	}}
	if err := ValidateFactSetup(authenticated); err == nil || !strings.Contains(err.Error(), "session is pre-installed") {
		t.Fatalf("authentication setup error = %v", err)
	}

	expiry := identityFactByKind(t, facts.Facts, "verification-expiry-boundary")
	expiry.Input.Identity.Cases[0].Setup.Evidence[0].Condition = "consumed"
	if err := ValidateFactSetup(expiry); err == nil || !strings.Contains(err.Error(), "self-fulfilled or incomplete") {
		t.Fatalf("expiry setup error = %v", err)
	}

	resent := identityFactByKind(t, facts.Facts, "verification-resent")
	if len(resent.Setup.Evidence) != 1 || resent.Setup.Evidence[0].Condition != "issued" {
		t.Fatalf("resend setup evidence = %#v, want one prior issued evidence", resent.Setup.Evidence)
	}
	if !countEquals(resent.Expected.Identity.Evidence.Count, 2) || !countEquals(resent.Expected.Identity.Evidence.Added, 1) || !countEquals(resent.Expected.Identity.Notice.Added, 1) {
		t.Fatalf("resend deltas = evidence %#v, notice %#v", resent.Expected.Identity.Evidence, resent.Expected.Identity.Notice)
	}
	resent.Setup.Evidence[0].Condition = "superseded"
	if err := ValidateFactSetup(resent); err == nil || !strings.Contains(err.Error(), "exactly one prior issued evidence") {
		t.Fatalf("resend setup error = %v", err)
	}
}

func TestIdentityFactHandlesAreScopedPerSubject(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	fact := identityFactByKind(t, facts.Facts, "ownership-denied")
	if len(fact.Setup.Subjects) != 2 || fact.Setup.Subjects[0].Handle != "subject/alice" || fact.Setup.Subjects[1].Handle != "subject/bob" {
		t.Fatalf("ownership setup subjects = %#v", fact.Setup.Subjects)
	}
	for _, subject := range fact.Setup.Subjects {
		if len(subject.Credentials) != 1 || !strings.HasPrefix(subject.Credentials[0].Handle, subject.Handle+"/credential/") {
			t.Fatalf("credential handle is not subject-scoped: %#v", subject)
		}
	}
}

func TestIdentityAcceptanceFactsRejectBrokenSemanticReferences(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	fact := identityFactByKind(t, facts.Facts, "authentication-accepted")
	fact.Setup.Subjects[0].Credentials[0].Credential = "identity/UserAccount/credential/missing"
	for index := range facts.Facts {
		if facts.Facts[index].ID == fact.ID {
			facts.Facts[index] = fact
		}
	}
	if err := ValidateAcceptanceFacts(intent, facts); err == nil || !strings.Contains(err.Error(), "missing semantic node") {
		t.Fatalf("reference validation error = %v", err)
	}

	facts, err = BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	fact = identityFactByKind(t, facts.Facts, "verification-accepted")
	fact.Setup.Subjects[0].State.Value = "Missing"
	for index := range facts.Facts {
		if facts.Facts[index].ID == fact.ID {
			facts.Facts[index] = fact
		}
	}
	if err := ValidateAcceptanceFacts(intent, facts); err == nil || !strings.Contains(err.Error(), "invalid state value") {
		t.Fatalf("state validation error = %v", err)
	}
}

func membershipIdentityFacts(facts []AcceptanceFact) []AcceptanceFact {
	result := make([]AcceptanceFact, 0, 29)
	for _, fact := range facts {
		if isIdentityFact(fact) {
			result = append(result, fact)
		}
	}
	return result
}

func identityFactByKind(t *testing.T, facts []AcceptanceFact, kind string) AcceptanceFact {
	t.Helper()
	for _, fact := range facts {
		if fact.Kind == kind && isIdentityFact(fact) {
			return fact
		}
	}
	t.Fatalf("Identity fact kind %s is missing", kind)
	return AcceptanceFact{}
}

func TestConsumedRejectionCaseMustStartFromAReachableState(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	// The canonical facts start the consumed case after the successful
	// verification, so they validate.
	if err := ValidateAcceptanceFacts(intent, facts); err != nil {
		t.Fatalf("canonical consumed case must validate: %v", err)
	}
	rejectedIndex := -1
	for index := range facts.Facts {
		if facts.Facts[index].Kind == "verification-rejected" {
			rejectedIndex = index
		}
	}
	if rejectedIndex < 0 {
		t.Fatal("the membership fixture must produce a verification-rejected fact")
	}

	stateID := intent.Entities[0].State.ID
	tests := []struct {
		name  string
		value string
		want  string
	}{
		// Returning to the pre-verification state describes evidence that could
		// not have been consumed.
		{name: "pre-verification state", value: "Pending", want: "but a successful verification reaches"},
		// A different unreachable state must be refused too: only the state the
		// accepted fact reaches is valid.
		{name: "unrelated state", value: "Suspended", want: "but a successful verification reaches"},
	}
	for _, test := range tests {
		t.Run("setup in the "+test.name, func(t *testing.T) {
			broken := cloneFactsForTest(t, facts)
			for index := range broken.Facts[rejectedIndex].Input.Identity.Cases {
				item := &broken.Facts[rejectedIndex].Input.Identity.Cases[index]
				if item.Kind != "consumed" {
					continue
				}
				for subject := range item.Setup.Subjects {
					item.Setup.Subjects[subject].State = &IRStateValueRef{State: stateID, Value: test.value}
				}
			}
			for index := range broken.Facts[rejectedIndex].Expected.Identity.Cases {
				expectation := &broken.Facts[rejectedIndex].Expected.Identity.Cases[index]
				if expectation.Kind == "consumed" {
					expectation.SubjectState = &IRStateValueRef{State: stateID, Value: test.value}
				}
			}
			if err := ValidateAcceptanceFacts(intent, broken); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation = %v, want the unreachable state to be rejected", err)
			}
		})
	}

	t.Run("expectation disagrees with the reached state", func(t *testing.T) {
		broken := cloneFactsForTest(t, facts)
		for index := range broken.Facts[rejectedIndex].Expected.Identity.Cases {
			expectation := &broken.Facts[rejectedIndex].Expected.Identity.Cases[index]
			if expectation.Kind == "consumed" {
				expectation.SubjectState = &IRStateValueRef{State: stateID, Value: "Pending"}
			}
		}
		if err := ValidateAcceptanceFacts(intent, broken); err == nil ||
			!strings.Contains(err.Error(), "a rejection must leave the subject in") {
			t.Fatalf("validation = %v, want the disagreeing expectation to be rejected", err)
		}
	})
}

func TestDuplicateIdentifierCaseMustStartFromACommittedRegistration(t *testing.T) {
	intent, _ := membershipIntentFixture(t)
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAcceptanceFacts(intent, facts); err != nil {
		t.Fatalf("canonical duplicate case must validate: %v", err)
	}
	duplicateIndex := -1
	for index := range facts.Facts {
		if facts.Facts[index].Kind == "duplicate-identifier-rejected" {
			duplicateIndex = index
		}
	}
	if duplicateIndex < 0 {
		t.Fatal("the membership fixture must produce a duplicate-identifier-rejected fact")
	}

	tests := []struct {
		name   string
		break_ func(fact *AcceptanceFact)
		want   string
	}{
		// A prior registration without its evidence is the state this fact used
		// to describe. Registration commits the evidence with the subject, so
		// no run can present the identifier that is being duplicated here.
		{
			name: "existing subject with no evidence",
			break_: func(fact *AcceptanceFact) {
				for index := range fact.Input.Identity.Cases {
					fact.Input.Identity.Cases[index].Setup.Evidence = nil
				}
			},
			want: "omits the evidence the prior registration issued",
		},
		// Evidence already consumed would mean the subject verified, which is a
		// different starting point with a different duplicate response.
		{
			name: "evidence already consumed",
			break_: func(fact *AcceptanceFact) {
				for index := range fact.Input.Identity.Cases {
					fact.Input.Identity.Cases[index].Setup.Evidence[0].Condition = "consumed"
				}
			},
			want: "omits the evidence the prior registration issued",
		},
		// Dropping the credential describes a subject that registration could
		// not have produced either.
		{
			name: "existing subject with no credential",
			break_: func(fact *AcceptanceFact) {
				for index := range fact.Input.Identity.Cases {
					fact.Input.Identity.Cases[index].Setup.Subjects[0].Credentials = nil
				}
			},
			want: "not a single credentialed subject",
		},
		// With evidence in the setup, an absolute count of one would also hold
		// if the duplicate attempt had replaced it. Only growth of zero states
		// that the attempt changed nothing.
		{
			name:   "evidence expectation without growth",
			break_: func(fact *AcceptanceFact) { fact.Expected.Identity.Evidence.Added = nil },
			want:   "no evidence added to the existing registration",
		},
		{
			name:   "notice expectation without growth",
			break_: func(fact *AcceptanceFact) { fact.Expected.Identity.Notice.Added = nil },
			want:   "no notice added to the existing registration",
		},
		{
			name:   "notice growth permitted",
			break_: func(fact *AcceptanceFact) { fact.Expected.Identity.Notice.Added = exactCount(1) },
			want:   "no notice added to the existing registration",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			broken := cloneFactsForTest(t, facts)
			test.break_(&broken.Facts[duplicateIndex])
			if err := ValidateAcceptanceFacts(intent, broken); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation = %v, want the unreachable duplicate setup to be rejected", err)
			}
		})
	}
}

func cloneFactsForTest(t *testing.T, facts *AcceptanceFacts) *AcceptanceFacts {
	t.Helper()
	content, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	var clone AcceptanceFacts
	if err := json.Unmarshal(content, &clone); err != nil {
		t.Fatal(err)
	}
	return &clone
}
