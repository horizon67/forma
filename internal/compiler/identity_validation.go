package compiler

import (
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
)

// CanonicalizeResolvedIntent normalizes set-like Identity data without
// changing ordered semantics such as identifier canonicalization or page input
// order. It is exported for compiler-produced, syntax-independent fixtures.
func CanonicalizeResolvedIntent(intent *ResolvedIntent) {
	if intent == nil {
		return
	}
	sort.Slice(intent.Roles, func(i, j int) bool { return intent.Roles[i].ID < intent.Roles[j].ID })
	sort.Slice(intent.Types, func(i, j int) bool { return intent.Types[i].ID < intent.Types[j].ID })
	for typeIndex := range intent.Types {
		sort.Slice(intent.Types[typeIndex].Constraints, func(i, j int) bool {
			return intent.Types[typeIndex].Constraints[i].ID < intent.Types[typeIndex].Constraints[j].ID
		})
	}
	sort.Slice(intent.Entities, func(i, j int) bool { return intent.Entities[i].ID < intent.Entities[j].ID })
	for entityIndex := range intent.Entities {
		entity := &intent.Entities[entityIndex]
		sort.Slice(entity.Invariants, func(i, j int) bool { return entity.Invariants[i].ID < entity.Invariants[j].ID })
	}
	sort.Slice(intent.Actions, func(i, j int) bool { return intent.Actions[i].ID < intent.Actions[j].ID })
	sort.Slice(intent.Pages, func(i, j int) bool { return intent.Pages[i].ID < intent.Pages[j].ID })
	sort.Slice(intent.Identities, func(i, j int) bool { return intent.Identities[i].ID < intent.Identities[j].ID })
	for identityIndex := range intent.Identities {
		identity := &intent.Identities[identityIndex]
		sort.Slice(identity.Identifiers, func(i, j int) bool { return identity.Identifiers[i].ID < identity.Identifiers[j].ID })
		sort.Slice(identity.Proofs, func(i, j int) bool { return identity.Proofs[i].ID < identity.Proofs[j].ID })
		sort.Slice(identity.Credentials, func(i, j int) bool { return identity.Credentials[i].ID < identity.Credentials[j].ID })
		sort.Slice(identity.Verifications, func(i, j int) bool { return identity.Verifications[i].ID < identity.Verifications[j].ID })
		sort.Slice(identity.Ownership, func(i, j int) bool { return identity.Ownership[i].ID < identity.Ownership[j].ID })
		identity.Registration.Attributes = canonicalSemanticIDs(identity.Registration.Attributes)
		identity.Registration.AtomicOutcome = canonicalStrings(identity.Registration.AtomicOutcome)
	}
	for pageIndex := range intent.Pages {
		page := &intent.Pages[pageIndex]
		canonicalizeAccess(page.Access)
		sort.Slice(page.SurfaceTransitions, func(i, j int) bool {
			return page.SurfaceTransitions[i].ID < page.SurfaceTransitions[j].ID
		})
		sort.Slice(page.IdentityInteractions, func(i, j int) bool {
			return page.IdentityInteractions[i].ID < page.IdentityInteractions[j].ID
		})
		for interactionIndex := range page.IdentityInteractions {
			canonicalizeAccess(&page.IdentityInteractions[interactionIndex].Access)
		}
		for viewIndex := range page.Views {
			view := &page.Views[viewIndex]
			for actionIndex := range view.Actions {
				canonicalizeAccess(&view.Actions[actionIndex].Access)
			}
			if view.Submit != nil {
				canonicalizeAccess(&view.Submit.Access)
			}
		}
	}
}

func canonicalizeAccess(access *IRAccess) {
	if access == nil {
		return
	}
	for index := range access.AllOf {
		if access.AllOf[index].Kind == "roles" {
			access.AllOf[index].AnyOf = canonicalStrings(access.AllOf[index].AnyOf)
		}
	}
	sort.Slice(access.AllOf, func(i, j int) bool {
		left, right := access.AllOf[i], access.AllOf[j]
		leftKey := strings.Join([]string{string(left.Source), left.Kind, strings.Join(left.AnyOf, "\x00"), string(left.Identity), string(left.Ownership), string(left.ResourceBinding)}, "\x01")
		rightKey := strings.Join([]string{string(right.Source), right.Kind, strings.Join(right.AnyOf, "\x00"), string(right.Identity), string(right.Ownership), string(right.ResourceBinding)}, "\x01")
		return leftKey < rightKey
	})
}

// ValidateResolvedIntent checks stable identity and the closed Identity slice
// before it can be serialized or handed to a coding agent.
func ValidateResolvedIntent(intent *ResolvedIntent) error {
	if intent == nil {
		return fmt.Errorf("validate Resolved Intent: nil intent")
	}
	if intent.Version != ResolvedIntentVersion {
		return fmt.Errorf("validate Resolved Intent: version %q is not %q", intent.Version, ResolvedIntentVersion)
	}
	if _, err := resolvedIntentSemanticIDs(intent); err != nil {
		return fmt.Errorf("validate Resolved Intent: %w", err)
	}
	if err := validateInvariantSemantics(intent); err != nil {
		return err
	}
	if err := validateApplicationNavigation(intent); err != nil {
		return err
	}
	if err := validateActionRefNavigation(intent); err != nil {
		return err
	}
	return validateIdentitySemantics(intent)
}

func validateApplicationNavigation(intent *ResolvedIntent) error {
	pages := make(map[string]IRPage, len(intent.Pages))
	for _, page := range intent.Pages {
		if page.ID != pageID(page.Name) {
			return fmt.Errorf("validate Resolved Intent: page %s has non-canonical ID", page.ID)
		}
		pages[page.Name] = page
	}
	validateTarget := func(owner SemanticID, target string) error {
		page, ok := pages[target]
		if !ok {
			return fmt.Errorf("validate Resolved Intent: navigation %s references missing page %q", owner, target)
		}
		if page.Param != nil {
			return fmt.Errorf("validate Resolved Intent: navigation %s targets parameterized page %q without a binding", owner, target)
		}
		return nil
	}
	if intent.Entry != nil {
		if intent.Entry.ID != applicationEntryID() {
			return fmt.Errorf("validate Resolved Intent: application entry %s has non-canonical ID", intent.Entry.ID)
		}
		if err := validateTarget(intent.Entry.ID, intent.Entry.Page); err != nil {
			return err
		}
	}
	for _, page := range intent.Pages {
		seen := map[string]bool{}
		for _, transition := range page.SurfaceTransitions {
			if transition.Kind != "continue" {
				return fmt.Errorf("validate Resolved Intent: surface transition %s has unsupported kind %q", transition.ID, transition.Kind)
			}
			if transition.ID != surfaceTransitionID(page.Name, transition.Kind) {
				return fmt.Errorf("validate Resolved Intent: surface transition %s has non-canonical ID", transition.ID)
			}
			if seen[transition.Kind] {
				return fmt.Errorf("validate Resolved Intent: page %s has duplicate %s transition", page.ID, transition.Kind)
			}
			seen[transition.Kind] = true
			if err := validateTarget(transition.ID, transition.TargetPage); err != nil {
				return err
			}
		}
	}
	for _, owner := range intent.Pages {
		for _, interaction := range owner.IdentityInteractions {
			if interaction.Continuation == nil || interaction.Success.Kind != "page" {
				continue
			}
			successPage, ok := pages[interaction.Success.Page]
			if !ok {
				continue
			}
			for _, transition := range successPage.SurfaceTransitions {
				if transition.Kind == "continue" {
					return fmt.Errorf("validate Resolved Intent: continuation from page %s is declared by both %s and %s", successPage.ID, interaction.Continuation.ID, transition.ID)
				}
			}
		}
	}
	return nil
}

// ValidateSourceMapCoverage requires a one-to-one entry for every semantic
// node. Multiple nodes may still point at the same source span.
// validateActionRefNavigation rejects an intent that carries a post-write
// destination on a standard `create` or `edit` reference. The chosen form's
// submit intent is the only source of truth for navigation after a write, so a
// success page here would be a second, possibly disagreeing, record.
func validateActionRefNavigation(intent *ResolvedIntent) error {
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			for _, action := range view.Actions {
				if action.Kind != "standard" {
					continue
				}
				if action.Name != "create" && action.Name != "edit" {
					continue
				}
				if action.SuccessPage != "" {
					return fmt.Errorf(
						"validate Resolved Intent: standard action %s carries success page %q; the target form's submit intent owns post-write navigation",
						action.ID, action.SuccessPage)
				}
			}
		}
	}
	return nil
}

func ValidateSourceMapCoverage(intent *ResolvedIntent, sourceMap *SourceMap) error {
	if err := ValidateResolvedIntent(intent); err != nil {
		return err
	}
	if sourceMap == nil {
		return fmt.Errorf("validate Source Map: nil map")
	}
	if sourceMap.Version != SourceMapVersion || sourceMap.IntentVersion != intent.Version {
		return fmt.Errorf("validate Source Map: schema versions do not match Resolved Intent")
	}
	ids, err := resolvedIntentSemanticIDs(intent)
	if err != nil {
		return fmt.Errorf("validate Source Map: %w", err)
	}
	entries := make(map[SemanticID]bool, len(sourceMap.Entries))
	for _, entry := range sourceMap.Entries {
		if entry.NodeID == "" || entry.Kind == "" {
			return fmt.Errorf("validate Source Map: entry has empty node ID or kind")
		}
		if entries[entry.NodeID] {
			return fmt.Errorf("validate Source Map: duplicate entry %s", entry.NodeID)
		}
		entries[entry.NodeID] = true
		if !ids[entry.NodeID] {
			return fmt.Errorf("validate Source Map: entry %s has no Resolved Intent node", entry.NodeID)
		}
	}
	missing := make([]SemanticID, 0)
	for id := range ids {
		if !entries[id] {
			missing = append(missing, id)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	if len(missing) == 1 {
		return fmt.Errorf("validate Source Map: node %s has no source entry", missing[0])
	}
	if len(missing) > 1 {
		values := make([]string, len(missing))
		for index, id := range missing {
			values[index] = string(id)
		}
		return fmt.Errorf("validate Source Map: nodes %s have no source entries", strings.Join(values, ", "))
	}
	return nil
}

func resolvedIntentSemanticIDs(intent *ResolvedIntent) (map[SemanticID]bool, error) {
	ids := map[SemanticID]bool{}
	var visit func(reflect.Value) error
	visit = func(value reflect.Value) error {
		if !value.IsValid() {
			return nil
		}
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return nil
			}
			return visit(value.Elem())
		}
		switch value.Kind() {
		case reflect.Struct:
			typeOf := value.Type()
			for index := 0; index < value.NumField(); index++ {
				field := value.Field(index)
				if typeOf.Field(index).Name == "ID" && field.Type() == reflect.TypeOf(SemanticID("")) {
					id := field.Interface().(SemanticID)
					if id == "" {
						return fmt.Errorf("semantic node has empty ID")
					}
					if ids[id] {
						return fmt.Errorf("duplicate semantic node %s", id)
					}
					ids[id] = true
				}
				if err := visit(field); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for index := 0; index < value.Len(); index++ {
				if err := visit(value.Index(index)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(reflect.ValueOf(intent)); err != nil {
		return nil, err
	}
	// Verification and authentication operations are declarations even though
	// their owning semantic nodes also carry the operation-specific contract.
	for _, identity := range intent.Identities {
		for _, verification := range identity.Verifications {
			for _, id := range []SemanticID{verification.VerifyOperation, verification.ResendOperation} {
				if id == "" {
					return nil, fmt.Errorf("identity operation has empty ID")
				}
				if ids[id] {
					return nil, fmt.Errorf("duplicate semantic node %s", id)
				}
				ids[id] = true
			}
		}
		for _, id := range []SemanticID{identity.Authentication.SignInOperation, identity.Authentication.SignOutOperation} {
			if id == "" {
				return nil, fmt.Errorf("identity operation has empty ID")
			}
			if ids[id] {
				return nil, fmt.Errorf("duplicate semantic node %s", id)
			}
			ids[id] = true
		}
	}
	return ids, nil
}

func validateIdentitySemantics(intent *ResolvedIntent) error {
	if len(intent.Identities) == 0 {
		return validateAllAccess(intent, nil, nil)
	}
	if len(intent.Identities) != 1 {
		return fmt.Errorf("validate Resolved Intent: first Identity slice requires exactly one identity")
	}
	entities := map[SemanticID]IREntity{}
	fields := map[SemanticID]IRField{}
	states := map[SemanticID]IRState{}
	actions := map[SemanticID]IRAction{}
	pages := map[SemanticID]IRPage{}
	parameters := map[SemanticID]IRParameter{}
	roles := map[string]bool{}
	for _, role := range intent.Roles {
		roles[role.Name] = true
	}
	for _, entity := range intent.Entities {
		entities[entity.ID] = entity
		for _, field := range entity.Fields {
			fields[field.ID] = field
		}
		if entity.State != nil {
			states[entity.State.ID] = *entity.State
		}
	}
	for _, action := range intent.Actions {
		actions[action.ID] = action
	}
	for _, page := range intent.Pages {
		pages[page.ID] = page
		if page.Param != nil {
			parameters[page.Param.ID] = *page.Param
		}
	}

	identity := intent.Identities[0]
	if identity.ID != identityID(identity.Name) {
		return fmt.Errorf("validate Resolved Intent: identity %s has non-canonical ID", identity.ID)
	}
	subject, ok := entities[identity.Subject]
	if !ok {
		return fmt.Errorf("validate Resolved Intent: identity %s references missing subject %s", identity.ID, identity.Subject)
	}
	if len(identity.Identifiers) != 1 || len(identity.Proofs) != 1 || len(identity.Credentials) != 1 || len(identity.Verifications) != 1 || len(identity.Ownership) != 1 {
		return fmt.Errorf("validate Resolved Intent: first Identity slice requires one identifier, proof, credential, verification, and ownership")
	}

	identifier := identity.Identifiers[0]
	if identifier.ID != identifierID(identity.Name, identifier.Name) {
		return fmt.Errorf("validate Resolved Intent: identifier %s has non-canonical ID", identifier.ID)
	}
	identifierField, ok := fields[identifier.Field]
	if !ok || !fieldBelongsTo(subject, identifier.Field) {
		return fmt.Errorf("validate Resolved Intent: identifier %s must reference a subject field", identifier.ID)
	}
	if !identifierField.Required || !identifierField.Unique || identifierField.Collection {
		return fmt.Errorf("validate Resolved Intent: identifier field %s must be required, unique, and non-collection", identifier.Field)
	}
	if !reflect.DeepEqual(identifier.Canonicalization, []IRCanonicalizationStep{{Kind: "trim-unicode-white-space"}, {Kind: "ascii-case-fold"}}) {
		return fmt.Errorf("validate Resolved Intent: email identifier has unsupported canonicalization")
	}

	credential := identity.Credentials[0]
	proof := identity.Proofs[0]
	if proof.ID != authenticationProofID(identity.Name, proof.Name) || proof.Kind != "local-password" {
		return fmt.Errorf("validate Resolved Intent: proof %s is not the supported local-password proof", proof.ID)
	}
	if credential.ID != credentialID(identity.Name, credential.Name) || credential.Kind != "password" {
		return fmt.Errorf("validate Resolved Intent: credential %s is not the canonical password credential", credential.ID)
	}
	if proof.Credential != credential.ID {
		return fmt.Errorf("validate Resolved Intent: proof %s is not the supported local-password proof", proof.ID)
	}
	if !credential.InputPolicy.PreserveWhitespace || credential.InputPolicy.Length != (IRLengthConstraint{Min: 12, Max: 128, Unit: "unicode-scalar-value"}) {
		return fmt.Errorf("validate Resolved Intent: password credential has unsupported input policy")
	}

	verification := identity.Verifications[0]
	if err := validateRegistration(identity, subject, identifier, proof, credential, verification, fields, states); err != nil {
		return err
	}
	if err := validateVerification(identity, subject, identifier, verification, states, actions); err != nil {
		return err
	}
	if err := validateAuthentication(identity, subject, identifier, proof, credential, states); err != nil {
		return err
	}
	ownership := identity.Ownership[0]
	if ownership.ID != ownershipID(identity.Name, "self") || ownership.Identity != identity.ID || ownership.Resource != identity.Subject || ownership.Relation != "principal-subject-equals-resource-identity" {
		return fmt.Errorf("validate Resolved Intent: ownership %s is not the supported self predicate", ownership.ID)
	}

	identityNodes := map[SemanticID]string{
		identifier.ID: "identifier", credential.ID: "credential", verification.ID: "evidence",
	}
	operations := map[SemanticID]bool{
		identity.Registration.ID: true, verification.VerifyOperation: true, verification.ResendOperation: true,
		identity.Authentication.SignInOperation: true, identity.Authentication.SignOutOperation: true,
	}
	for _, pageItem := range intent.Pages {
		for _, interaction := range pageItem.IdentityInteractions {
			if !operations[interaction.Operation] {
				return fmt.Errorf("validate Resolved Intent: interaction %s references missing operation %s", interaction.ID, interaction.Operation)
			}
			operationName := path.Base(string(interaction.Operation))
			if interaction.ID != identityInteractionID(pageItem.Name, operationName, identity.Name) {
				return fmt.Errorf("validate Resolved Intent: interaction %s has non-canonical ID", interaction.ID)
			}
			for _, input := range interaction.Inputs {
				if input.Kind == "field" {
					if !fieldBelongsTo(subject, input.Node) {
						return fmt.Errorf("validate Resolved Intent: interaction %s references missing subject field %s", interaction.ID, input.Node)
					}
					continue
				}
				if identityNodes[input.Node] != input.Kind {
					return fmt.Errorf("validate Resolved Intent: interaction %s has invalid %s input %s", interaction.ID, input.Kind, input.Node)
				}
			}
			if err := validateNavigation(interaction.Success, pages); err != nil {
				return fmt.Errorf("validate Resolved Intent: interaction %s success: %w", interaction.ID, err)
			}
			if interaction.Continuation != nil {
				if err := validateNavigation(*interaction.Continuation, pages); err != nil {
					return fmt.Errorf("validate Resolved Intent: interaction %s continuation: %w", interaction.ID, err)
				}
			}
		}
	}
	return validateAllAccess(intent, roles, map[SemanticID]IROwnership{ownership.ID: ownership})
}

func validateRegistration(identity IRIdentity, subject IREntity, identifier IRIdentifier, proof IRAuthenticationProof, credential IRCredential, verification IRVerification, fields map[SemanticID]IRField, states map[SemanticID]IRState) error {
	registration := identity.Registration
	if registration.ID != identityOperationID(identity.Name, "register") || registration.Identifier != identifier.ID || registration.Proof != proof.ID || registration.Credential != credential.ID || registration.Verification != verification.ID {
		return fmt.Errorf("validate Resolved Intent: registration %s has invalid references", registration.ID)
	}
	for _, attribute := range registration.Attributes {
		if _, ok := fields[attribute]; !ok || !fieldBelongsTo(subject, attribute) || attribute == identifier.Field {
			return fmt.Errorf("validate Resolved Intent: registration %s has invalid attribute %s", registration.ID, attribute)
		}
	}
	if err := validateStateValue(subject, registration.InitialState, states, "Pending"); err != nil {
		return fmt.Errorf("validate Resolved Intent: registration %s: %w", registration.ID, err)
	}
	if !reflect.DeepEqual(registration.AtomicOutcome, []string{"credential-binding", "notice-emission-record", "subject", "verification-evidence"}) || registration.ExistingIdentifierOutcome != "reject-and-guide-resend" {
		return fmt.Errorf("validate Resolved Intent: registration %s has unsupported outcome contract", registration.ID)
	}
	return nil
}

func validateVerification(identity IRIdentity, subject IREntity, identifier IRIdentifier, verification IRVerification, states map[SemanticID]IRState, actions map[SemanticID]IRAction) error {
	if verification.ID != verificationID(identity.Name, "email") || verification.Kind != "opaque-email-link" || verification.Subject != identity.Subject || verification.VerifyOperation != identityOperationID(identity.Name, "verify") || verification.ResendOperation != identityOperationID(identity.Name, "resend") {
		return fmt.Errorf("validate Resolved Intent: verification %s has unsupported identity contract", verification.ID)
	}
	if err := validateStateValue(subject, verification.EligibleState, states, "Pending"); err != nil {
		return fmt.Errorf("validate Resolved Intent: verification %s: %w", verification.ID, err)
	}
	action, ok := actions[verification.SuccessAction]
	if !ok || action.Entity != subject.Name || !reflect.DeepEqual(action.Sources, []string{"Pending"}) || action.Destination != "Active" {
		return fmt.Errorf("validate Resolved Intent: verification %s success action must transition Pending to Active", verification.ID)
	}
	if verification.Evidence != (IRVerificationEvidence{Kind: "opaque", Lifetime: IRDuration{Amount: 30, Unit: "minute"}, ValidBoundary: "now-before-issued-plus-lifetime", MaxUses: 1, Rotation: "invalidate-prior-unconsumed"}) {
		return fmt.Errorf("validate Resolved Intent: verification %s has unsupported evidence contract", verification.ID)
	}
	notice := verification.Notice
	if notice.ID != verificationNoticeID(identity.Name, "email") || notice.Channel != "email" || notice.Recipient != identifier.ID || notice.Emission != "durable-record-required" || notice.DeliveryFailure != "subject-remains-pending-and-retryable" || verification.ResendDisclosure != "uniform-for-pending-active-and-unknown" {
		return fmt.Errorf("validate Resolved Intent: verification %s has unsupported notice contract", verification.ID)
	}
	return nil
}

func validateAuthentication(identity IRIdentity, subject IREntity, identifier IRIdentifier, proof IRAuthenticationProof, credential IRCredential, states map[SemanticID]IRState) error {
	authentication := identity.Authentication
	if authentication.ID != authenticationID(identity.Name) || authentication.SignInOperation != identityOperationID(identity.Name, "signin") || authentication.SignOutOperation != identityOperationID(identity.Name, "signout") || authentication.Identifier != identifier.ID || authentication.Proof != proof.ID || authentication.Credential != credential.ID || authentication.FailureDisclosure != "generic" {
		return fmt.Errorf("validate Resolved Intent: authentication %s has unsupported references", authentication.ID)
	}
	if err := validateStateValue(subject, authentication.EligibleState, states, "Active"); err != nil {
		return fmt.Errorf("validate Resolved Intent: authentication %s: %w", authentication.ID, err)
	}
	if authentication.Session.ID != sessionID(identity.Name, "current") || authentication.Session.PrincipalSubject != identity.Subject || authentication.Session.SignOutScope != "current-session" {
		return fmt.Errorf("validate Resolved Intent: authentication %s has unsupported session", authentication.ID)
	}
	return nil
}

func validateStateValue(subject IREntity, ref IRStateValueRef, states map[SemanticID]IRState, expected string) error {
	state, ok := states[ref.State]
	if !ok || subject.State == nil || subject.State.ID != ref.State {
		return fmt.Errorf("state reference %s does not belong to subject", ref.State)
	}
	found := false
	for _, value := range state.Values {
		found = found || value == ref.Value
	}
	if !found || ref.Value != expected {
		return fmt.Errorf("state value %q is not supported", ref.Value)
	}
	return nil
}

func validateNavigation(navigation IRNavigationIntent, pages map[SemanticID]IRPage) error {
	switch navigation.Kind {
	case "page":
		if _, ok := pages[pageID(navigation.Page)]; !ok || navigation.FallbackPage != "" {
			return fmt.Errorf("page navigation references missing page %q", navigation.Page)
		}
	case "same-context":
		if navigation.Page == "" || navigation.FallbackPage != "" {
			return fmt.Errorf("same-context navigation requires its page")
		}
	case "caller-list":
		if navigation.FallbackPage == "" || navigation.Page != "" {
			return fmt.Errorf("caller-list navigation requires a fallback page")
		}
	default:
		return fmt.Errorf("unsupported navigation kind %q", navigation.Kind)
	}
	return nil
}

func validateAllAccess(intent *ResolvedIntent, roles map[string]bool, ownership map[SemanticID]IROwnership) error {
	identities := map[SemanticID]bool{}
	parameters := map[SemanticID]IRParameter{}
	for _, identity := range intent.Identities {
		identities[identity.ID] = true
	}
	for _, page := range intent.Pages {
		if page.Param != nil {
			parameters[page.Param.ID] = *page.Param
		}
		if page.Access != nil {
			if err := validateAccess(*page.Access, roles, identities, ownership, parameters); err != nil {
				return err
			}
		}
		for _, interaction := range page.IdentityInteractions {
			if err := validateAccess(interaction.Access, roles, identities, ownership, parameters); err != nil {
				return err
			}
		}
		for _, view := range page.Views {
			for _, action := range view.Actions {
				if err := validateAccess(action.Access, roles, identities, ownership, parameters); err != nil {
					return err
				}
			}
			if view.Submit != nil {
				if err := validateAccess(view.Submit.Access, roles, identities, ownership, parameters); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateAccess(access IRAccess, roles map[string]bool, identities map[SemanticID]bool, ownership map[SemanticID]IROwnership, parameters map[SemanticID]IRParameter) error {
	for _, requirement := range access.AllOf {
		switch requirement.Kind {
		case "roles":
			if len(requirement.AnyOf) == 0 || requirement.Identity != "" || requirement.Ownership != "" || requirement.ResourceBinding != "" {
				return fmt.Errorf("validate Resolved Intent: access %s has invalid roles requirement", access.ID)
			}
			for _, role := range requirement.AnyOf {
				if roles != nil && !roles[role] {
					return fmt.Errorf("validate Resolved Intent: access %s references missing role %s", access.ID, role)
				}
			}
		case "authenticated":
			if len(requirement.AnyOf) != 0 || !identities[requirement.Identity] || requirement.Ownership != "" || requirement.ResourceBinding != "" {
				return fmt.Errorf("validate Resolved Intent: access %s has invalid authenticated requirement", access.ID)
			}
		case "ownership":
			predicate, ok := ownership[requirement.Ownership]
			parameter, parameterOK := parameters[requirement.ResourceBinding]
			if len(requirement.AnyOf) != 0 || requirement.Identity != "" || !ok || !parameterOK || entityID(parameter.Entity) != predicate.Resource {
				return fmt.Errorf("validate Resolved Intent: access %s has invalid ownership requirement", access.ID)
			}
		default:
			return fmt.Errorf("validate Resolved Intent: access %s has unsupported requirement kind %q", access.ID, requirement.Kind)
		}
	}
	return nil
}

func fieldBelongsTo(entity IREntity, fieldID SemanticID) bool {
	for _, field := range entity.Fields {
		if field.ID == fieldID {
			return true
		}
	}
	return false
}
