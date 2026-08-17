package compiler

import (
	"fmt"
)

func (c *checker) checkIdentities() {
	if len(c.program.Identities) > 1 {
		for _, identity := range c.program.Identities[1:] {
			c.error(identity.Name.Span, "F2701", "the first Identity slice supports exactly one identity", "keep one identity declaration")
		}
	}
	for _, identity := range c.program.Identities {
		if c.identities[identity.Name.Text] != identity {
			continue
		}
		subject := c.entities[identity.Subject.Text]
		if subject == nil {
			c.error(identity.Subject.Span, "F2702", fmt.Sprintf("unknown identity subject `%s`", identity.Subject.Text), "bind the identity to a declared entity")
			continue
		}
		if len(identity.Identifiers) != 1 || len(identity.Proofs) != 1 || len(identity.Verifications) != 1 || len(identity.Ownerships) != 1 || identity.Registration == nil || identity.Authentication == nil {
			c.error(identity.Span, "F2701", "the first Identity slice requires one identifier, proof, registration, verification, authentication, and ownership", "declare each required Identity member exactly once")
			continue
		}

		identifier := identity.Identifiers[0]
		field := c.identitySubjectField(subject, identifier.Field)
		if field != nil && (!hasFieldModifier(field, "required") || !hasFieldModifier(field, "unique") || field.Type.Collection) {
			c.error(identifier.Field.Span, "F2703", fmt.Sprintf("identifier field `%s.%s` must be required and unique", subject.Name.Text, field.Name.Text), "make the current identifier field required, unique, and non-collection")
		}
		if !nameTextsEqual(identifier.Canonicalization, []string{"trimUnicodeWhitespace", "asciiCaseFold"}) {
			c.error(identifier.Span, "F2703", "email identifier requires canonicalize trimUnicodeWhitespace, asciiCaseFold", "use the closed first-slice canonicalization order")
		}

		proof := identity.Proofs[0]
		if proof.Kind.Text != "localPassword" {
			c.error(proof.Kind.Span, "F2704", fmt.Sprintf("unsupported authentication proof `%s`", proof.Kind.Text), "the first slice supports `localPassword`; passwordless, verificationEvidence, and externalAssertion remain explicit future proof kinds")
		}
		if proof.MinLength != 12 || proof.MaxLength != 128 || proof.LengthUnit.Text != "unicodeScalarValue" || !proof.PreserveWhitespace {
			c.error(proof.Span, "F2704", "localPassword proof has an unsupported input policy", "use minLength 12, maxLength 128, lengthUnit unicodeScalarValue, and preserveWhitespace")
		}

		registration := identity.Registration
		if registration.Name.Text != "register" || registration.Identifier.Text != identifier.Name.Text || registration.Proof.Text != proof.Name.Text ||
			registration.Verification.Text != identity.Verifications[0].Name.Text || registration.InitialState.Text != "status" || registration.InitialValue.Text != "Pending" ||
			registration.ExistingIdentifierOutcome.Text != "rejectAndGuideResend" || !nameTextsEqual(registration.Attributes, []string{"name"}) {
			c.error(registration.Span, "F2705", "registration does not match the supported email-verified membership contract", "declare register with email, localPassword, name, Pending, email verification, and rejectAndGuideResend")
		}
		for _, attribute := range registration.Attributes {
			c.identitySubjectField(subject, attribute)
		}
		c.identityStateValue(subject, registration.InitialState, registration.InitialValue, "registration")

		verification := identity.Verifications[0]
		if verification.Name.Text != "email" || verification.Kind.Text != "emailLink" || verification.VerifyOperation.Text != "verify" || verification.ResendOperation.Text != "resend" ||
			verification.EligibleState.Text != "status" || verification.EligibleValue.Text != "Pending" || verification.LifetimeAmount != 30 || verification.LifetimeUnit.Text != "minute" ||
			verification.MaxUses != 1 || verification.Rotation.Text != "invalidatePriorUnconsumed" || verification.NoticeChannel.Text != "email" ||
			verification.NoticeEmission.Text != "durable" || verification.DeliveryFailure.Text != "pendingAndRetryable" || verification.ResendDisclosure.Text != "uniform" {
			c.error(verification.Span, "F2706", "verification does not match the supported opaque email-link contract", "use the closed first-slice verification values")
		}
		c.identityStateValue(subject, verification.EligibleState, verification.EligibleValue, "verification")
		action := c.actions[actionKey(verification.SuccessEntity.Text, verification.SuccessAction.Text)]
		if action == nil {
			c.error(verification.SuccessAction.Span, "F2706", fmt.Sprintf("unknown verification success action `%s.%s`", verification.SuccessEntity.Text, verification.SuccessAction.Text), "declare the Pending to Active domain action")
		} else if action.Entity.Text != subject.Name.Text || !nameTextsEqual(action.Sources, []string{"Pending"}) || action.Destination.Text != "Active" {
			c.error(verification.SuccessAction.Span, "F2706", "verification success action must transition the identity subject from Pending to Active", "reference a Pending -> Active action on the subject entity")
		}

		authentication := identity.Authentication
		if authentication.Identifier.Text != identifier.Name.Text || authentication.Proof.Text != proof.Name.Text || authentication.SignInOperation.Text != "signin" || authentication.SignOutOperation.Text != "signout" ||
			authentication.EligibleState.Text != "status" || authentication.EligibleValue.Text != "Active" || authentication.FailureDisclosure.Text != "generic" {
			c.error(authentication.Span, "F2707", "authentication does not match the supported local-password contract", "declare email/password signin, signout, Active eligibility, and generic failure")
		}
		c.identityStateValue(subject, authentication.EligibleState, authentication.EligibleValue, "authentication")
		if identity.Ownerships[0].Name.Text != "self" {
			c.error(identity.Ownerships[0].Name.Span, "F2708", "the first Identity slice supports only `ownership self`", "use `ownership self`")
		}
	}
}

func (c *checker) identitySubjectField(subject *EntityDecl, name Name) *FieldDecl {
	for _, field := range subject.Fields {
		if field.Name.Text == name.Text {
			return field
		}
	}
	c.error(name.Span, "F2702", fmt.Sprintf("unknown identity subject field `%s.%s`", subject.Name.Text, name.Text), "use a field declared directly by the identity subject")
	return nil
}

func (c *checker) identityStateValue(subject *EntityDecl, stateName, value Name, context string) {
	if subject.State == nil || subject.State.Name.Text != stateName.Text {
		c.error(stateName.Span, "F2702", fmt.Sprintf("unknown %s state `%s.%s`", context, subject.Name.Text, stateName.Text), "use the subject's named state")
		return
	}
	for _, candidate := range subject.State.Values {
		if candidate.Text == value.Text {
			return
		}
	}
	c.error(value.Span, "F2702", fmt.Sprintf("unknown %s state value `%s`", context, value.Text), "use a value declared by the subject state")
}

func nameTextsEqual(names []Name, values []string) bool {
	if len(names) != len(values) {
		return false
	}
	for index := range names {
		if names[index].Text != values[index] {
			return false
		}
	}
	return true
}

func (c *checker) buildIdentityIR(decl *IdentityDecl, sourceMap *sourceMapBuilder) IRIdentity {
	id := identityID(decl.Name.Text)
	identifierDecl := decl.Identifiers[0]
	proofDecl := decl.Proofs[0]
	registrationDecl := decl.Registration
	verificationDecl := decl.Verifications[0]
	authenticationDecl := decl.Authentication
	identifier := identifierID(decl.Name.Text, identifierDecl.Name.Text)
	proof := authenticationProofID(decl.Name.Text, proofDecl.Name.Text)
	credential := credentialID(decl.Name.Text, proofDecl.Name.Text)
	verification := verificationID(decl.Name.Text, verificationDecl.Name.Text)
	state := semanticID(string(entityID(decl.Subject.Text)), "state", registrationDecl.InitialState.Text)

	item := IRIdentity{
		ID: id, Name: decl.Name.Text, Subject: entityID(decl.Subject.Text),
		Identifiers: []IRIdentifier{{
			ID: identifier, Name: identifierDecl.Name.Text,
			Field:            semanticID(string(entityID(decl.Subject.Text)), "field", identifierDecl.Field.Text),
			Canonicalization: []IRCanonicalizationStep{{Kind: "trim-unicode-white-space"}, {Kind: "ascii-case-fold"}},
		}},
		Proofs: []IRAuthenticationProof{{ID: proof, Name: proofDecl.Name.Text, Kind: "local-password", Credential: credential}},
		Credentials: []IRCredential{{
			ID: credential, Name: proofDecl.Name.Text, Kind: "password",
			InputPolicy: IRCredentialInputPolicy{PreserveWhitespace: true, Length: IRLengthConstraint{Min: proofDecl.MinLength, Max: proofDecl.MaxLength, Unit: "unicode-scalar-value"}},
		}},
		Registration: IRRegistration{
			ID: identityOperationID(decl.Name.Text, registrationDecl.Name.Text), Identifier: identifier, Proof: proof, Credential: credential,
			InitialState: IRStateValueRef{State: state, Value: registrationDecl.InitialValue.Text}, Verification: verification,
			AtomicOutcome:             []string{"verification-evidence", "subject", "credential-binding", "notice-emission-record"},
			ExistingIdentifierOutcome: "reject-and-guide-resend",
		},
		Verifications: []IRVerification{{
			ID: verification, Kind: "opaque-email-link", Subject: entityID(decl.Subject.Text),
			VerifyOperation:  identityOperationID(decl.Name.Text, verificationDecl.VerifyOperation.Text),
			ResendOperation:  identityOperationID(decl.Name.Text, verificationDecl.ResendOperation.Text),
			EligibleState:    IRStateValueRef{State: state, Value: verificationDecl.EligibleValue.Text},
			SuccessAction:    actionID(verificationDecl.SuccessEntity.Text, verificationDecl.SuccessAction.Text),
			Evidence:         IRVerificationEvidence{Kind: "opaque", Lifetime: IRDuration{Amount: verificationDecl.LifetimeAmount, Unit: verificationDecl.LifetimeUnit.Text}, ValidBoundary: "now-before-issued-plus-lifetime", MaxUses: verificationDecl.MaxUses, Rotation: "invalidate-prior-unconsumed"},
			Notice:           IRVerificationNotice{ID: verificationNoticeID(decl.Name.Text, verificationDecl.Name.Text), Channel: "email", Recipient: identifier, Emission: "durable-record-required", DeliveryFailure: "subject-remains-pending-and-retryable"},
			ResendDisclosure: "uniform-for-pending-active-and-unknown",
		}},
		Authentication: IRAuthentication{
			ID: authenticationID(decl.Name.Text), SignInOperation: identityOperationID(decl.Name.Text, authenticationDecl.SignInOperation.Text),
			SignOutOperation: identityOperationID(decl.Name.Text, authenticationDecl.SignOutOperation.Text), Identifier: identifier, Proof: proof, Credential: credential,
			EligibleState: IRStateValueRef{State: state, Value: authenticationDecl.EligibleValue.Text}, FailureDisclosure: "generic",
			Session: IRSession{ID: sessionID(decl.Name.Text, "current"), PrincipalSubject: entityID(decl.Subject.Text), SignOutScope: "current-session"},
		},
		Ownership: []IROwnership{{ID: ownershipID(decl.Name.Text, "self"), Identity: id, Resource: entityID(decl.Subject.Text), Relation: "principal-subject-equals-resource-identity"}},
	}
	for _, attribute := range registrationDecl.Attributes {
		item.Registration.Attributes = append(item.Registration.Attributes, semanticID(string(entityID(decl.Subject.Text)), "field", attribute.Text))
	}

	sourceMap.add(id, "identity", decl.Span)
	sourceMap.add(identifier, "identity-identifier", identifierDecl.Span)
	sourceMap.add(proof, "identity-proof", proofDecl.Span)
	sourceMap.add(credential, "identity-credential", proofDecl.Span)
	sourceMap.add(item.Registration.ID, "identity-registration", registrationDecl.Span)
	sourceMap.add(verification, "identity-verification", verificationDecl.Span)
	sourceMap.add(item.Verifications[0].VerifyOperation, "identity-operation", verificationDecl.Span)
	sourceMap.add(item.Verifications[0].ResendOperation, "identity-operation", verificationDecl.Span)
	sourceMap.add(item.Verifications[0].Notice.ID, "identity-verification-notice", verificationDecl.Span)
	sourceMap.add(item.Authentication.ID, "identity-authentication", authenticationDecl.Span)
	sourceMap.add(item.Authentication.SignInOperation, "identity-operation", authenticationDecl.Span)
	sourceMap.add(item.Authentication.SignOutOperation, "identity-operation", authenticationDecl.Span)
	sourceMap.add(item.Authentication.Session.ID, "identity-session", authenticationDecl.Span)
	sourceMap.add(item.Ownership[0].ID, "identity-ownership", decl.Ownerships[0].Span)
	return item
}
