package compiler

import "fmt"

type identityAcceptanceContext struct {
	identity       IRIdentity
	identifier     IRIdentifier
	credential     IRCredential
	verification   IRVerification
	authentication IRAuthentication
	ownership      IROwnership
	subject        IREntity
	state          IRState
	interactions   map[SemanticID]IRIdentityInteraction
	pages          map[string]IRPage
}

func (b *acceptanceBuilder) addIdentityFacts(identity IRIdentity) error {
	context, err := b.identityAcceptanceContext(identity)
	if err != nil {
		return err
	}
	c := context
	register := c.identity.Registration.ID
	verify := c.verification.VerifyOperation
	resend := c.verification.ResendOperation
	signin := c.authentication.SignInOperation
	signout := c.authentication.SignOutOperation
	registerInteraction := c.interactions[register]
	verifyInteraction := c.interactions[verify]
	resendInteraction := c.interactions[resend]
	signoutInteraction := c.interactions[signout]
	nameField := c.subjectField("name")
	emailField := c.identifier.Field
	pending := IRStateValueRef{State: c.state.ID, Value: "Pending"}
	active := IRStateValueRef{State: c.state.ID, Value: "Active"}
	created := "subject/created"
	alice := "subject/alice"
	bob := "subject/bob"
	existing := "subject/existing"
	aliceCredential := alice + "/credential/primary"
	bobCredential := bob + "/credential/primary"
	existingCredential := existing + "/credential/primary"
	aliceEvidence := alice + "/evidence/email-verification"
	existingEvidence := existing + "/evidence/email-verification"
	aliceSession := alice + "/session/current"

	// 1. Anonymous registration access.
	b.add(AcceptanceFact{
		ID: factID(registerInteraction.ID, "access", "allowed", "anonymous"), Kind: "access-allowed", Subject: registerInteraction.ID,
		Principal:   &FactPrincipal{Kind: "anonymous"},
		Input:       &FactInput{Identity: &IdentityFactInput{Interaction: registerInteraction.ID, Observe: []string{"access"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "allowed", Surfaces: []SemanticID{pageID("SignUp"), registerInteraction.ID}}},
		SourceNodes: []SemanticID{registerInteraction.ID, pageID("SignUp")},
	})

	// 2. Registration input projection.
	b.add(AcceptanceFact{
		ID: factID(registerInteraction.ID, "inputs"), Kind: "identity-inputs", Subject: registerInteraction.ID,
		Input:       &FactInput{Identity: &IdentityFactInput{Interaction: registerInteraction.ID, Observe: []string{"resolved-inputs"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Inputs: append([]IRIdentityInputRef(nil), registerInteraction.Inputs...)}},
		SourceNodes: append([]SemanticID{registerInteraction.ID}, identityInputNodes(registerInteraction.Inputs)...),
	})

	// 3. Credential is never a domain projection.
	b.add(AcceptanceFact{
		ID: factID(c.credential.ID, "non-projectable"), Kind: "credential-non-projectable", Subject: c.credential.ID,
		Input:       &FactInput{Identity: &IdentityFactInput{Observe: []string{"resolved-intent-projections", "artifact-projections"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "absent-from-domain-projections", ExcludedCredentials: []SemanticID{c.credential.ID}}},
		SourceNodes: []SemanticID{c.identity.ID, c.credential.ID},
	})

	// 4. Invalid name, identifier, and credential are isolated cases.
	validationCases := []IdentityFactCase{
		{Kind: "invalid-name", Dispatches: 1},
		{Kind: "invalid-identifier", Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: "input/identifier/invalid", Relation: "invalid"}, Dispatches: 1},
		{Kind: "invalid-credential", Credential: &FactCredentialInput{Credential: c.credential.ID, Binding: "input/credential/invalid", Relation: "violates-policy"}, Dispatches: 1},
	}
	validationExpected := []IdentityFactCaseExpectation{
		{Kind: "invalid-name", Outcome: "rejected"},
		{Kind: "invalid-identifier", Outcome: "rejected"},
		{Kind: "invalid-credential", Outcome: "rejected"},
	}
	b.add(AcceptanceFact{
		ID: factID(register, "validation", "rejected"), Kind: "registration-validation-rejected", Subject: register,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: register, Cases: validationCases, Observe: []string{"subject-count", "credential-binding-count", "evidence-count", "notice-emission-count"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "rejected", Subject: &FactSubjectExpectation{Count: exactCount(0)},
			Credential: &FactCredentialExpectation{Credential: c.credential.ID, Subject: created, Condition: "absent"},
			Evidence:   &FactEvidenceExpectation{Verification: c.verification.ID, Count: exactCount(0)},
			Notice:     &FactNoticeExpectation{Notice: c.verification.Notice.ID, Count: exactCount(0)}, Cases: validationExpected,
		}},
		SourceNodes: []SemanticID{register, nameField, c.identifier.ID, c.credential.ID, c.verification.ID, c.verification.Notice.ID},
	})

	// 5. Domain inputs may be preserved after rejection; Credential may not.
	b.add(AcceptanceFact{
		ID: factID(registerInteraction.ID, "validation", "preserve-input"), Kind: "secret-input-not-preserved", Subject: registerInteraction.ID,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: register, Interaction: registerInteraction.ID, Cases: validationCases, Observe: []string{"redisplayed-inputs"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "invalid-input-redisplayed", PreserveFields: []SemanticID{nameField, emailField}, ExcludedCredentials: []SemanticID{c.credential.ID}, Cases: validationExpected,
		}},
		SourceNodes: []SemanticID{registerInteraction.ID, register, nameField, emailField, c.credential.ID},
	})

	// 6. Exact and canonical-equivalent duplicate identifiers share one result.
	//
	// A registered subject only exists because registration committed the
	// credential, the verification evidence, and the notice together, so the
	// setup carries all of them. Expecting no evidence at all would describe a
	// state the application cannot reach; the duplicate attempt must instead add
	// nothing to what is already there.
	existingRegistration := func() *FactSetup {
		setup := identitySetup(subjectSetup(existing, c.identity.ID, &pending, credentialSetup(existingCredential, c.credential.ID)))
		setup.Evidence = []FactEvidenceSetup{{
			Handle: existingEvidence, Verification: c.verification.ID, Subject: existing, Condition: "issued",
		}}
		return setup
	}
	duplicateCases := []IdentityFactCase{
		{
			Kind: "exact", Setup: existingRegistration(),
			Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: existing + "/identifier/primary", Relation: "exact"}, Dispatches: 1,
		},
		{
			Kind: "canonical-equivalent", Setup: existingRegistration(),
			Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: "input/identifier/canonical-equivalent", Relation: "canonical-equivalent"}, Dispatches: 1,
		},
	}
	b.add(AcceptanceFact{
		ID: factID(register, "identifier", "duplicate"), Kind: "duplicate-identifier-rejected", Subject: register,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: register, Cases: duplicateCases, Observe: []string{"subject-count", "credential-binding-count", "evidence-count", "notice-emission-count"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "rejected", Subject: &FactSubjectExpectation{Count: exactCount(1), Unchanged: true}, Disclosure: "guide-resend",
			Credential: &FactCredentialExpectation{Credential: c.credential.ID, Subject: existing, Condition: "unchanged"},
			Evidence:   &FactEvidenceExpectation{Verification: c.verification.ID, Count: exactCount(1), Added: exactCount(0), Condition: "issued"},
			Notice:     &FactNoticeExpectation{Notice: c.verification.Notice.ID, Count: exactCount(1), Added: exactCount(0)},
			Cases:      []IdentityFactCaseExpectation{{Kind: "exact", Outcome: "rejected", Disclosure: "guide-resend"}, {Kind: "canonical-equivalent", Outcome: "rejected", Disclosure: "guide-resend"}},
		}},
		SourceNodes: []SemanticID{register, c.identifier.ID, c.credential.ID, c.verification.ID, c.verification.Notice.ID},
	})

	// 7-12. Successful registration and at-most-once semantics.
	b.add(AcceptanceFact{
		ID: factID(register, "subject", "created"), Kind: "registration-created", Subject: register,
		Input:       identityOperationInput(register, 1, "subject-count"),
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "created", Subject: &FactSubjectExpectation{Count: exactCount(1), State: &pending}}},
		SourceNodes: []SemanticID{register, c.subject.ID, c.state.ID},
	})
	b.add(AcceptanceFact{
		ID: factID(register, "credential", "bound"), Kind: "credential-bound", Subject: register,
		Input: identityOperationInput(register, 1, "authentication-result"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{Credential: &FactCredentialExpectation{
			Credential: c.credential.ID, Subject: created, Condition: "satisfies-policy", ObservableVia: signin,
		}}},
		SourceNodes: []SemanticID{register, c.credential.ID, signin},
	})
	b.add(AcceptanceFact{
		ID: factID(register, "verification", "issued"), Kind: "verification-issued", Subject: register,
		Input: identityOperationInput(register, 1, "evidence-count"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{Evidence: &FactEvidenceExpectation{
			Verification: c.verification.ID, Count: exactCount(1), Condition: "issued", MaxUses: c.verification.Evidence.MaxUses,
			Lifetime: &c.verification.Evidence.Lifetime,
		}}},
		SourceNodes: []SemanticID{register, c.verification.ID},
	})
	b.add(AcceptanceFact{
		ID: factID(register, "notice", "emitted"), Kind: "notice-emitted", Subject: register,
		Input: identityOperationInput(register, 1, "notice-emission-count"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{Notice: &FactNoticeExpectation{
			Notice: c.verification.Notice.ID, Count: exactCount(1), Emission: c.verification.Notice.Emission,
		}}},
		SourceNodes: []SemanticID{register, c.verification.Notice.ID},
	})
	b.add(AcceptanceFact{
		ID: factID(registerInteraction.ID, "navigation"), Kind: "navigation", Subject: registerInteraction.ID,
		Input:       &FactInput{Identity: &IdentityFactInput{Operation: register, Interaction: registerInteraction.ID, Dispatches: 1, Observe: []string{"navigation"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Navigation: identityFactNavigation(registerInteraction.Success)}},
		SourceNodes: []SemanticID{registerInteraction.ID, registerInteraction.Success.ID, pageID("CheckEmail")},
	})
	b.add(AcceptanceFact{
		ID: factID(register, "at-most-once"), Kind: "operation-at-most-once", Subject: register,
		Input: identityOperationInput(register, 2, "subject-count", "notice-emission-count"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "applied-once", AppliedOperations: 1, Subject: &FactSubjectExpectation{Count: exactCount(1)}, Notice: &FactNoticeExpectation{Notice: c.verification.Notice.ID, Count: exactCount(1)},
		}},
		SourceNodes: []SemanticID{register, c.subject.ID, c.verification.Notice.ID},
	})

	// 13-16. Verification success, consumption, rejection, and navigation.
	verificationSetup := setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "before-expiry")
	b.add(AcceptanceFact{
		ID: factID(verify, "accepted"), Kind: "verification-accepted", Subject: verify, Setup: verificationSetup,
		Input:       &FactInput{Identity: &IdentityFactInput{Operation: verify, Evidence: aliceEvidence, Dispatches: 1, Subject: alice, Observe: []string{"subject-state"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "accepted", Subject: &FactSubjectExpectation{Count: exactCount(1), State: &active}}},
		SourceNodes: []SemanticID{c.verification.ID, verify, c.verification.SuccessAction, c.state.ID},
	})
	b.add(AcceptanceFact{
		ID: factID(verify, "evidence", "consumed"), Kind: "verification-consumed", Subject: verify, Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "before-expiry"),
		Input:       &FactInput{Identity: &IdentityFactInput{Operation: verify, Evidence: aliceEvidence, Dispatches: 1, Subject: alice, Observe: []string{"evidence-condition"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Evidence: &FactEvidenceExpectation{Verification: c.verification.ID, Count: exactCount(1), Condition: "consumed"}}},
		SourceNodes: []SemanticID{c.verification.ID, verify},
	})
	rejectedCases := []IdentityFactCase{
		{Kind: "invalid", Setup: identitySetup(subjectSetup(alice, c.identity.ID, &pending)), Evidence: "input/evidence/invalid", Dispatches: 1},
		{Kind: "expired", Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "after-expiry"), Evidence: aliceEvidence, Clock: "after-expiry", Dispatches: 1},
		// Consumed evidence can only exist because the atomic successful
		// verification already moved the subject out of Pending, so the setup
		// must start from that reachable state.
		{Kind: "consumed", Setup: setupWithEvidence(c, alice, aliceEvidence, active, "consumed", "before-expiry"), Evidence: aliceEvidence, Dispatches: 1},
	}
	b.add(AcceptanceFact{
		ID: factID(verify, "evidence", "rejected"), Kind: "verification-rejected", Subject: verify,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: verify, Cases: rejectedCases, Observe: []string{"subject-state", "evidence-condition"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "rejected", Cases: []IdentityFactCaseExpectation{
				{Kind: "invalid", Outcome: "rejected", SubjectState: &pending},
				{Kind: "expired", Outcome: "rejected", SubjectState: &pending},
				{Kind: "consumed", Outcome: "rejected", SubjectState: &active},
			},
		}},
		SourceNodes: []SemanticID{c.verification.ID, verify, c.state.ID},
	})
	verifyNavigation := identityFactNavigation(verifyInteraction.Success)
	verifySurfaces := []SemanticID{pageID(verifyInteraction.Success.Page)}
	verifySources := []SemanticID{verifyInteraction.ID, verifyInteraction.Success.ID, pageID(verifyInteraction.Success.Page)}
	verifyObservations := []string{"navigation"}
	if verifyInteraction.Continuation != nil {
		verifyNavigation.ContinuationPage = pageID(verifyInteraction.Continuation.Page)
		verifySurfaces = append(verifySurfaces, pageID(verifyInteraction.Continuation.Page))
		verifySources = append(verifySources, verifyInteraction.Continuation.ID, pageID(verifyInteraction.Continuation.Page))
		verifyObservations = append(verifyObservations, "continuation")
	}
	b.add(AcceptanceFact{
		ID: factID(verifyInteraction.ID, "navigation"), Kind: "navigation", Subject: verifyInteraction.ID,
		Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "before-expiry"),
		Input: &FactInput{Identity: &IdentityFactInput{Operation: verify, Interaction: verifyInteraction.ID, Evidence: aliceEvidence, Dispatches: 1, Observe: verifyObservations}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Navigation: verifyNavigation, Surfaces: verifySurfaces,
		}},
		SourceNodes: verifySources,
	})

	// 17-20. Resend without state change, rotation, disclosure, and at-most-once.
	pendingSetup := setupWithIssuedEvidence(c, alice, aliceEvidence, pending)
	b.add(AcceptanceFact{
		ID: factID(resend, "accepted"), Kind: "verification-resent", Subject: resend, Setup: pendingSetup,
		Input: identityOperationSubjectInput(resend, alice, 1, "subject-state", "evidence-count", "notice-emission-count"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "accepted", Subject: &FactSubjectExpectation{Count: exactCount(1), State: &pending, Unchanged: true},
			Evidence: &FactEvidenceExpectation{Verification: c.verification.ID, Count: exactCount(2), Added: exactCount(1), Condition: "issued"},
			Notice:   &FactNoticeExpectation{Notice: c.verification.Notice.ID, Added: exactCount(1)},
		}},
		SourceNodes: []SemanticID{resend, c.verification.ID, c.verification.Notice.ID, c.state.ID},
	})
	b.add(AcceptanceFact{
		ID: factID(resend, "evidence", "rotated"), Kind: "verification-rotated", Subject: resend,
		Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "before-expiry"),
		Input: &FactInput{Identity: &IdentityFactInput{Operation: resend, Subject: alice, Evidence: aliceEvidence, Dispatches: 1, Observe: []string{"evidence-condition", "evidence-count"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{Evidence: &FactEvidenceExpectation{
			Verification: c.verification.ID, Count: exactCount(2), Condition: "superseded", Rotation: c.verification.Evidence.Rotation,
		}}},
		SourceNodes: []SemanticID{resend, c.verification.ID},
	})
	resendCases := []IdentityFactCase{
		{Kind: "unknown", Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: "input/identifier/unknown", Relation: "unknown"}, Dispatches: 1},
		{Kind: "active", Setup: identitySetup(subjectSetup(alice, c.identity.ID, &active)), Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: alice + "/identifier/primary", Relation: "matching"}, Dispatches: 1},
		{Kind: "pending", Setup: identitySetup(subjectSetup(alice, c.identity.ID, &pending)), Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: alice + "/identifier/primary", Relation: "matching"}, Dispatches: 1},
	}
	b.add(AcceptanceFact{
		ID: factID(resendInteraction.ID, "disclosure", "uniform"), Kind: "enumeration-safe-outcome", Subject: resendInteraction.ID,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: resend, Interaction: resendInteraction.ID, Cases: resendCases, Observe: []string{"user-visible-outcome"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Disclosure: c.verification.ResendDisclosure,
			Cases: []IdentityFactCaseExpectation{
				{Kind: "unknown", Outcome: "accepted", Disclosure: c.verification.ResendDisclosure},
				{Kind: "active", Outcome: "accepted", Disclosure: c.verification.ResendDisclosure},
				{Kind: "pending", Outcome: "accepted", Disclosure: c.verification.ResendDisclosure},
			},
		}},
		SourceNodes: []SemanticID{resendInteraction.ID, resend, c.identifier.ID, c.verification.ID},
	})
	b.add(AcceptanceFact{
		ID: factID(resend, "at-most-once"), Kind: "operation-at-most-once", Subject: resend,
		Setup:       setupWithIssuedEvidence(c, alice, aliceEvidence, pending),
		Input:       identityOperationSubjectInput(resend, alice, 2, "notice-emission-count"),
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "applied-once", AppliedOperations: 1, Notice: &FactNoticeExpectation{Notice: c.verification.Notice.ID, Added: exactCount(1)}}},
		SourceNodes: []SemanticID{resend, c.verification.Notice.ID},
	})

	// 21-24. Authentication eligibility, generic rejection, and current-session signout.
	pendingCredentialSetup := identitySetup(subjectSetup(alice, c.identity.ID, &pending, credentialSetup(aliceCredential, c.credential.ID)))
	b.add(AcceptanceFact{
		ID: factID(signin, "state", "ineligible"), Kind: "authentication-ineligible-state", Subject: signin, Setup: pendingCredentialSetup,
		Input: signinFactInput(signin, alice, c.identifier.ID, c.credential.ID, aliceCredential, "matching"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "rejected", Subject: &FactSubjectExpectation{State: &pending, Unchanged: true},
			Session: &FactSessionExpectation{Session: c.authentication.Session.ID, Subject: alice, Condition: "absent"},
		}},
		SourceNodes: []SemanticID{signin, c.authentication.ID, c.authentication.Session.ID, c.state.ID, c.credential.ID},
	})
	activeCredentialSetup := identitySetup(subjectSetup(alice, c.identity.ID, &active, credentialSetup(aliceCredential, c.credential.ID)))
	b.add(AcceptanceFact{
		ID: factID(signin, "accepted"), Kind: "authentication-accepted", Subject: signin, Setup: activeCredentialSetup,
		Input: signinFactInput(signin, alice, c.identifier.ID, c.credential.ID, aliceCredential, "matching"),
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "accepted", Session: &FactSessionExpectation{Session: c.authentication.Session.ID, Subject: alice, Condition: "active"},
		}},
		SourceNodes: []SemanticID{signin, c.authentication.ID, c.authentication.Session.ID},
	})
	authRejectedCases := []IdentityFactCase{
		{Kind: "unknown-identifier", Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: "input/identifier/unknown", Relation: "unknown"}, Dispatches: 1},
		{
			Kind: "non-matching-credential", Setup: activeCredentialSetup,
			Identifier: &FactIdentifierInput{Identifier: c.identifier.ID, Handle: alice + "/identifier/primary", Relation: "matching"},
			Credential: &FactCredentialInput{Credential: c.credential.ID, Binding: aliceCredential, Relation: "non-matching"}, Dispatches: 1,
		},
	}
	b.add(AcceptanceFact{
		ID: factID(signin, "rejected", "generic"), Kind: "authentication-rejected", Subject: signin,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: signin, Cases: authRejectedCases, Observe: []string{"session-count", "user-visible-outcome"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "rejected", Disclosure: c.authentication.FailureDisclosure,
			Cases: []IdentityFactCaseExpectation{
				{Kind: "unknown-identifier", Outcome: "rejected", SessionCondition: "absent", Disclosure: c.authentication.FailureDisclosure},
				{Kind: "non-matching-credential", Outcome: "rejected", SessionCondition: "absent", Disclosure: c.authentication.FailureDisclosure},
			},
		}},
		SourceNodes: []SemanticID{signin, c.authentication.ID, c.identifier.ID, c.credential.ID},
	})
	signoutSetup := identitySetup(
		subjectSetup(alice, c.identity.ID, &active, credentialSetup(aliceCredential, c.credential.ID)),
	)
	signoutSetup.Sessions = []FactSessionSetup{{Handle: aliceSession, Session: c.authentication.Session.ID, Subject: alice, Condition: "active"}}
	b.add(AcceptanceFact{
		ID: factID(signout, "session", "terminated"), Kind: "session-terminated", Subject: signout, Setup: signoutSetup,
		Principal: &FactPrincipal{Kind: "authenticated", Identity: c.identity.ID, Subject: alice, Session: aliceSession},
		Input:     &FactInput{Identity: &IdentityFactInput{Operation: signout, Session: aliceSession, Subject: alice, Dispatches: 1, Observe: []string{"session-condition", "protected-access"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "terminated-and-access-denied", Session: &FactSessionExpectation{Session: c.authentication.Session.ID, Subject: alice, Condition: "terminated"},
			Surfaces: c.protectedSurfaces(),
		}},
		SourceNodes: append([]SemanticID{signoutInteraction.ID, signout, c.authentication.Session.ID}, c.protectedSurfaces()...),
	})

	// 25-27. Anonymous, self, and other-subject ownership cases.
	protectedSurfaces := c.protectedSurfaces()
	resourceSetup := identitySetup(subjectSetup(alice, c.identity.ID, &active))
	b.add(AcceptanceFact{
		ID: factID(c.ownership.ID, "access", "denied", "anonymous"), Kind: "access-denied", Subject: c.ownership.ID,
		Setup: resourceSetup, Principal: &FactPrincipal{Kind: "anonymous"},
		Input:       &FactInput{Identity: &IdentityFactInput{Resource: alice, Observe: []string{"protected-access"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "denied", Surfaces: protectedSurfaces}},
		SourceNodes: append([]SemanticID{c.ownership.ID, c.authentication.Session.ID}, protectedSurfaces...),
	})
	selfSetup := identitySetup(subjectSetup(alice, c.identity.ID, &active, credentialSetup(aliceCredential, c.credential.ID)))
	selfSetup.Sessions = []FactSessionSetup{{Handle: aliceSession, Session: c.authentication.Session.ID, Subject: alice, Condition: "active"}}
	b.add(AcceptanceFact{
		ID: factID(c.ownership.ID, "access", "allowed", "self"), Kind: "ownership-allowed", Subject: c.ownership.ID,
		Setup: selfSetup, Principal: &FactPrincipal{Kind: "authenticated", Identity: c.identity.ID, Subject: alice, Session: aliceSession},
		Input:       &FactInput{Identity: &IdentityFactInput{Resource: alice, Observe: []string{"view-access", "edit-access"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "allowed", Surfaces: protectedSurfaces}},
		SourceNodes: append([]SemanticID{c.ownership.ID, c.authentication.Session.ID}, protectedSurfaces...),
	})
	otherSetup := identitySetup(
		subjectSetup(alice, c.identity.ID, &active, credentialSetup(aliceCredential, c.credential.ID)),
		subjectSetup(bob, c.identity.ID, &active, credentialSetup(bobCredential, c.credential.ID)),
	)
	otherSetup.Sessions = []FactSessionSetup{{Handle: aliceSession, Session: c.authentication.Session.ID, Subject: alice, Condition: "active"}}
	b.add(AcceptanceFact{
		ID: factID(c.ownership.ID, "access", "denied", "other-subject"), Kind: "ownership-denied", Subject: c.ownership.ID,
		Setup: otherSetup, Principal: &FactPrincipal{Kind: "authenticated", Identity: c.identity.ID, Subject: alice, Session: aliceSession},
		Input:       &FactInput{Identity: &IdentityFactInput{Resource: bob, Observe: []string{"view-access", "edit-access"}}},
		Expected:    FactExpectation{Identity: &IdentityFactExpectation{Outcome: "denied", Surfaces: protectedSurfaces}},
		SourceNodes: append([]SemanticID{c.ownership.ID, c.authentication.Session.ID}, protectedSurfaces...),
	})

	// 28. before is valid; at and after expiry are rejected.
	expiryCases := []IdentityFactCase{
		{Kind: "before-expiry", Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "before-expiry"), Evidence: aliceEvidence, Clock: "before-expiry", Dispatches: 1},
		{Kind: "at-expiry", Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "at-expiry"), Evidence: aliceEvidence, Clock: "at-expiry", Dispatches: 1},
		{Kind: "after-expiry", Setup: setupWithEvidence(c, alice, aliceEvidence, pending, "issued", "after-expiry"), Evidence: aliceEvidence, Clock: "after-expiry", Dispatches: 1},
	}
	b.add(AcceptanceFact{
		ID: factID(verify, "expiry", "boundary"), Kind: "verification-expiry-boundary", Subject: verify,
		Input: &FactInput{Identity: &IdentityFactInput{Operation: verify, Cases: expiryCases, Observe: []string{"subject-state", "evidence-condition"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{Cases: []IdentityFactCaseExpectation{
			{Kind: "before-expiry", Outcome: "accepted", SubjectState: &active, EvidenceCondition: "consumed"},
			{Kind: "at-expiry", Outcome: "rejected", SubjectState: &pending, EvidenceCondition: "issued"},
			{Kind: "after-expiry", Outcome: "rejected", SubjectState: &pending, EvidenceCondition: "issued"},
		}}},
		SourceNodes: []SemanticID{c.verification.ID, verify, c.state.ID},
	})

	// 29. Durable emission and delivery failure are separate observations.
	b.add(AcceptanceFact{
		ID: factID(c.verification.Notice.ID, "delivery", "failure"), Kind: "delivery-failure-separated", Subject: c.verification.Notice.ID,
		Setup: &FactSetup{Delivery: &FactDeliverySetup{Notice: c.verification.Notice.ID, Condition: "fails"}},
		Input: &FactInput{Identity: &IdentityFactInput{Operation: register, Delivery: "fails", Dispatches: 1, Observe: []string{"subject-state", "notice-emission-count", "delivery-result", "resend-availability"}}},
		Expected: FactExpectation{Identity: &IdentityFactExpectation{
			Outcome: "retryable", Subject: &FactSubjectExpectation{Count: exactCount(1), State: &pending},
			Evidence: &FactEvidenceExpectation{Verification: c.verification.ID, Count: exactCount(1), Condition: "issued"},
			Notice:   &FactNoticeExpectation{Notice: c.verification.Notice.ID, Count: exactCount(1), Emission: c.verification.Notice.Emission, Delivery: "failed"},
		}},
		SourceNodes: []SemanticID{register, resend, c.verification.ID, c.verification.Notice.ID, c.state.ID},
	})
	return nil
}

func (b *acceptanceBuilder) identityAcceptanceContext(identity IRIdentity) (identityAcceptanceContext, error) {
	context := identityAcceptanceContext{identity: identity, interactions: map[SemanticID]IRIdentityInteraction{}, pages: b.pages}
	if len(identity.Identifiers) != 1 || len(identity.Proofs) != 1 || len(identity.Credentials) != 1 || len(identity.Verifications) != 1 || len(identity.Ownership) != 1 {
		return context, fmt.Errorf("build Identity Acceptance Facts: identity %s is not the supported first slice", identity.ID)
	}
	context.identifier = identity.Identifiers[0]
	context.credential = identity.Credentials[0]
	context.verification = identity.Verifications[0]
	context.authentication = identity.Authentication
	context.ownership = identity.Ownership[0]
	for _, entity := range b.intent.Entities {
		if entity.ID == identity.Subject {
			context.subject = entity
			if entity.State != nil {
				context.state = *entity.State
			}
		}
	}
	for _, page := range b.intent.Pages {
		for _, interaction := range page.IdentityInteractions {
			context.interactions[interaction.Operation] = interaction
		}
	}
	for _, operation := range []SemanticID{
		identity.Registration.ID, context.verification.VerifyOperation, context.verification.ResendOperation,
		context.authentication.SignInOperation, context.authentication.SignOutOperation,
	} {
		if context.interactions[operation].ID == "" {
			return context, fmt.Errorf("build Identity Acceptance Facts: operation %s has no page interaction", operation)
		}
	}
	return context, nil
}

func (c identityAcceptanceContext) subjectField(name string) SemanticID {
	for _, field := range c.subject.Fields {
		if field.Name == name {
			return field.ID
		}
	}
	return ""
}

func (c identityAcceptanceContext) protectedSurfaces() []SemanticID {
	result := []SemanticID{pageID("Profile"), pageID("ProfileEdit")}
	if page, ok := c.pages["Profile"]; ok && len(page.Views) != 0 {
		result = append(result, page.Views[0].ID)
	}
	if page, ok := c.pages["ProfileEdit"]; ok && len(page.Views) != 0 {
		result = append(result, page.Views[0].ID)
	}
	return canonicalSemanticIDs(result)
}

func identityOperationInput(operation SemanticID, dispatches int, observations ...string) *FactInput {
	return &FactInput{Identity: &IdentityFactInput{Operation: operation, Dispatches: dispatches, Observe: observations}}
}

func identityOperationSubjectInput(operation SemanticID, subject string, dispatches int, observations ...string) *FactInput {
	return &FactInput{Identity: &IdentityFactInput{Operation: operation, Subject: subject, Dispatches: dispatches, Observe: observations}}
}

func signinFactInput(operation SemanticID, subject string, identifier SemanticID, credential SemanticID, binding, relation string) *FactInput {
	return &FactInput{Identity: &IdentityFactInput{
		Operation: operation, Subject: subject, Dispatches: 1,
		Identifier: &FactIdentifierInput{Identifier: identifier, Handle: subject + "/identifier/primary", Relation: "matching"},
		Credential: &FactCredentialInput{Credential: credential, Binding: binding, Relation: relation},
		Observe:    []string{"session-count", "user-visible-outcome"},
	}}
}

func identityInputNodes(inputs []IRIdentityInputRef) []SemanticID {
	result := make([]SemanticID, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, input.Node)
	}
	return result
}

func identityFactNavigation(navigation IRNavigationIntent) *FactNavigation {
	result := &FactNavigation{SuccessKind: navigation.Kind}
	if navigation.Page != "" {
		result.SuccessPage = pageID(navigation.Page)
	}
	if navigation.FallbackPage != "" {
		result.FallbackPage = pageID(navigation.FallbackPage)
	}
	return result
}

func identitySetup(subjects ...FactSubjectSetup) *FactSetup {
	return &FactSetup{Subjects: subjects}
}

func subjectSetup(handle string, identity SemanticID, state *IRStateValueRef, credentials ...FactCredentialBindingSetup) FactSubjectSetup {
	return FactSubjectSetup{Handle: handle, Identity: identity, State: state, Credentials: credentials}
}

func credentialSetup(handle string, credential SemanticID) FactCredentialBindingSetup {
	return FactCredentialBindingSetup{Handle: handle, Credential: credential, Condition: "satisfies-policy"}
}

func setupWithEvidence(c identityAcceptanceContext, subject, evidence string, state IRStateValueRef, condition, clock string) *FactSetup {
	setup := setupWithEvidenceCondition(c, subject, evidence, state, condition)
	setup.Clock = &FactClockSetup{Evidence: evidence, Relation: clock}
	return setup
}

func setupWithIssuedEvidence(c identityAcceptanceContext, subject, evidence string, state IRStateValueRef) *FactSetup {
	return setupWithEvidenceCondition(c, subject, evidence, state, "issued")
}

func setupWithEvidenceCondition(c identityAcceptanceContext, subject, evidence string, state IRStateValueRef, condition string) *FactSetup {
	setup := identitySetup(subjectSetup(subject, c.identity.ID, &state))
	setup.Evidence = []FactEvidenceSetup{{Handle: evidence, Verification: c.verification.ID, Subject: subject, Condition: condition}}
	return setup
}

func exactCount(value int) *int {
	return &value
}
