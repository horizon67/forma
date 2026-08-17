package compiler

import (
	"fmt"
)

func (c *checker) checkIdentityPage(page *PageDecl) {
	seenRequirements := map[string]Span{}
	for _, requirement := range page.Requirements {
		c.checkAccessRequirement(page, requirement, seenRequirements)
	}
	seenInteractions := map[string]Span{}
	for _, interaction := range page.IdentityInteractions {
		key := interaction.Identity.Text + "." + interaction.Operation.Text
		if previous, exists := seenInteractions[key]; exists {
			c.error(interaction.Span, "F2710", fmt.Sprintf("duplicate identity interaction `%s`", key), fmt.Sprintf("the first interaction is at line %d", previous.Start.Line))
			continue
		}
		seenInteractions[key] = interaction.Span
		identity := c.identities[interaction.Identity.Text]
		if identity == nil {
			c.error(interaction.Identity.Span, "F2711", fmt.Sprintf("unknown identity `%s`", interaction.Identity.Text), "declare the identity before referencing an operation")
			continue
		}
		subject := c.entities[identity.Subject.Text]
		if subject == nil {
			continue
		}
		operation := c.identityOperationKind(identity, interaction.Operation.Text)
		if operation == "" {
			c.error(interaction.Operation.Span, "F2711", fmt.Sprintf("unknown identity operation `%s.%s`", identity.Name.Text, interaction.Operation.Text), "use register, verify, resend, signin, or signout; identifier-change operations are not implemented")
			continue
		}
		for _, field := range interaction.Fields {
			c.identitySubjectField(subject, field)
		}
		interactionRequirements := map[string]Span{}
		for _, requirement := range interaction.Requirements {
			c.checkAccessRequirement(page, requirement, interactionRequirements)
		}
		if interaction.SuccessPage == nil && !interaction.Stay {
			c.error(interaction.Span, "F2712", "identity interaction requires `success Page` or `stay`", "declare the observable success navigation")
		}
		if interaction.SuccessPage != nil && interaction.Stay {
			c.error(interaction.Span, "F2712", "identity interaction cannot combine `success` and `stay`", "choose one success navigation")
		}
		for _, target := range []*Name{interaction.SuccessPage, interaction.Continuation} {
			if target != nil && c.pages[target.Text] == nil {
				c.error(target.Span, "F2712", fmt.Sprintf("unknown navigation page `%s`", target.Text), "declare the page before using it as interaction navigation")
			}
		}
		c.checkInteractionContract(identity, operation, interaction)
	}
}

// checkIdentityInteractionCoverage keeps the closed first slice aligned with
// its Fact builder. Each declared operation must have exactly one observable
// page interaction; otherwise an accepted node could reach an agent without a
// corresponding verification obligation.
func (c *checker) checkIdentityInteractionCoverage() {
	for _, identity := range c.program.Identities {
		if c.identities[identity.Name.Text] != identity || identity.Registration == nil || identity.Authentication == nil || len(identity.Verifications) != 1 {
			continue
		}
		expected := []struct {
			name string
			span Span
		}{
			{name: identity.Registration.Name.Text, span: identity.Registration.Name.Span},
			{name: identity.Verifications[0].VerifyOperation.Text, span: identity.Verifications[0].VerifyOperation.Span},
			{name: identity.Verifications[0].ResendOperation.Text, span: identity.Verifications[0].ResendOperation.Span},
			{name: identity.Authentication.SignInOperation.Text, span: identity.Authentication.SignInOperation.Span},
			{name: identity.Authentication.SignOutOperation.Text, span: identity.Authentication.SignOutOperation.Span},
		}
		observed := map[string][]*IdentityInteractionDecl{}
		for _, page := range c.program.Pages {
			for _, interaction := range page.IdentityInteractions {
				if interaction.Identity.Text == identity.Name.Text {
					observed[interaction.Operation.Text] = append(observed[interaction.Operation.Text], interaction)
				}
			}
		}
		for _, operation := range expected {
			interactions := observed[operation.name]
			switch len(interactions) {
			case 0:
				c.error(operation.span, "F2715", fmt.Sprintf("identity operation `%s.%s` has no page interaction", identity.Name.Text, operation.name), "declare exactly one `interact Identity.operation` so Acceptance Facts cover the operation")
			case 1:
			default:
				for _, interaction := range interactions[1:] {
					c.error(interaction.Operation.Span, "F2715", fmt.Sprintf("identity operation `%s.%s` has more than one page interaction", identity.Name.Text, operation.name), "the first Identity slice requires exactly one interaction per operation; a compositional Fact builder is not implemented")
				}
			}
		}
	}
}

func (c *checker) checkAccessRequirement(page *PageDecl, requirement *AccessRequirementDecl, seen map[string]Span) {
	key := requirement.Kind + "/" + requirement.Identity.Text + "/" + requirement.Ownership.Text + "/" + requirement.Binding.Text
	if previous, exists := seen[key]; exists {
		c.error(requirement.Span, "F2005", "duplicate Identity access requirement", fmt.Sprintf("the first requirement is at line %d", previous.Start.Line))
	}
	seen[key] = requirement.Span
	identity := c.identities[requirement.Identity.Text]
	if identity == nil {
		c.error(requirement.Identity.Span, "F2713", fmt.Sprintf("unknown access identity `%s`", requirement.Identity.Text), "declare the identity before requiring it")
		return
	}
	if requirement.Kind == "owner" {
		if requirement.Ownership.Text != "self" || len(identity.Ownerships) != 1 || identity.Ownerships[0].Name.Text != "self" {
			c.error(requirement.Ownership.Span, "F2713", "the first Identity slice supports only `owner Identity.self`", "reference the declared self ownership")
		}
		if page.Param == nil || page.Param.Name.Text != requirement.Binding.Text || page.Param.Type.Text != identity.Subject.Text {
			c.error(requirement.Binding.Span, "F2713", "owner requirement must bind the identity subject page parameter", "declare `page Name(binding Subject)` and use that binding after `for`")
		}
	}
}

func (c *checker) identityOperationKind(identity *IdentityDecl, name string) string {
	switch {
	case identity.Registration != nil && identity.Registration.Name.Text == name:
		return "register"
	case len(identity.Verifications) == 1 && identity.Verifications[0].VerifyOperation.Text == name:
		return "verify"
	case len(identity.Verifications) == 1 && identity.Verifications[0].ResendOperation.Text == name:
		return "resend"
	case identity.Authentication != nil && identity.Authentication.SignInOperation.Text == name:
		return "signin"
	case identity.Authentication != nil && identity.Authentication.SignOutOperation.Text == name:
		return "signout"
	default:
		return ""
	}
}

func (c *checker) checkInteractionContract(identity *IdentityDecl, operation string, interaction *IdentityInteractionDecl) {
	wantFields := []string(nil)
	wantIdentifier, wantProof, wantEvidence := "", "", ""
	wantFeedback := []string(nil)
	wantContinuation := false
	wantRequirements := 0
	switch operation {
	case "register":
		wantFields = []string{"name"}
		wantIdentifier, wantProof = "email", "password"
		wantFeedback = []string{"invalid", "failure"}
	case "verify":
		wantEvidence = "email"
		wantFeedback = []string{"invalid", "expired", "failure"}
		wantContinuation = true
	case "resend":
		wantIdentifier = "email"
		wantFeedback = []string{"uniform", "failure"}
	case "signin":
		wantIdentifier, wantProof = "email", "password"
		wantFeedback = []string{"generic", "failure"}
	case "signout":
		wantFeedback = []string{"failure"}
		wantRequirements = 1
	}
	gotIdentifier, gotProof, gotEvidence := "", "", ""
	if interaction.Identifier != nil {
		gotIdentifier = interaction.Identifier.Text
	}
	if interaction.Proof != nil {
		gotProof = interaction.Proof.Text
	}
	if interaction.Evidence != nil {
		gotEvidence = interaction.Evidence.Text
	}
	if !nameTextsEqual(interaction.Fields, wantFields) || gotIdentifier != wantIdentifier || gotProof != wantProof || gotEvidence != wantEvidence ||
		!nameTextsEqual(interaction.Feedback, wantFeedback) || (interaction.Continuation != nil) != wantContinuation || len(interaction.Requirements) != wantRequirements {
		c.error(interaction.Span, "F2714", fmt.Sprintf("interaction `%s.%s` does not match the supported %s input and feedback contract", identity.Name.Text, interaction.Operation.Text, operation), "use the operation-specific fields, identifier, proof/evidence, feedback, continuation, and access from the Identity syntax proposal")
	}
	if operation == "resend" && !interaction.Stay {
		c.error(interaction.Span, "F2714", "resend interaction must use `stay`", "keep the user in the same CheckEmail context")
	}
	if operation == "signout" && (len(interaction.Requirements) != 1 || interaction.Requirements[0].Kind != "authenticated" || interaction.Requirements[0].Identity.Text != identity.Name.Text) {
		c.error(interaction.Span, "F2714", "signout interaction requires authenticated access", "add `require authenticated Identity` inside the interaction")
	}
}

func (c *checker) buildAccess(source, id SemanticID, requirements []*AccessRequirementDecl, sourceMap *sourceMapBuilder) IRAccess {
	access := IRAccess{ID: id, AllOf: []IRAccessRequirement{}}
	for _, requirement := range requirements {
		item := IRAccessRequirement{Source: source, Kind: requirement.Kind}
		if requirement.Kind == "authenticated" {
			item.Identity = identityID(requirement.Identity.Text)
		} else if requirement.Kind == "owner" {
			item.Kind = "ownership"
			item.Ownership = ownershipID(requirement.Identity.Text, requirement.Ownership.Text)
			item.ResourceBinding = semanticID(string(source), "parameter")
		}
		access.AllOf = append(access.AllOf, item)
	}
	canonicalizeAccess(&access)
	if sourceMap != nil {
		span := Span{}
		if len(requirements) > 0 {
			span = requirements[0].Span
		}
		sourceMap.add(id, "access", span)
	}
	return access
}

func (c *checker) buildIdentityInteraction(page *PageDecl, decl *IdentityInteractionDecl, sourceMap *sourceMapBuilder) IRIdentityInteraction {
	id := identityInteractionID(page.Name.Text, decl.Operation.Text, decl.Identity.Text)
	item := IRIdentityInteraction{
		ID: id, Operation: identityOperationID(decl.Identity.Text, decl.Operation.Text),
		Access:   c.buildAccess(pageID(page.Name.Text), semanticID(string(id), "access"), decl.Requirements, sourceMap),
		Feedback: namesToStrings(decl.Feedback),
	}
	if len(decl.Requirements) == 0 {
		sourceMap.add(item.Access.ID, "access", decl.Span)
	}
	identity := c.identities[decl.Identity.Text]
	for _, field := range decl.Fields {
		item.Inputs = append(item.Inputs, IRIdentityInputRef{Kind: "field", Node: semanticID(string(entityID(identity.Subject.Text)), "field", field.Text)})
	}
	if decl.Identifier != nil {
		item.Inputs = append(item.Inputs, IRIdentityInputRef{Kind: "identifier", Node: identifierID(identity.Name.Text, decl.Identifier.Text)})
	}
	if decl.Proof != nil {
		item.Inputs = append(item.Inputs, IRIdentityInputRef{Kind: "credential", Node: credentialID(identity.Name.Text, decl.Proof.Text)})
	}
	if decl.Evidence != nil {
		item.Inputs = append(item.Inputs, IRIdentityInputRef{Kind: "evidence", Node: verificationID(identity.Name.Text, decl.Evidence.Text)})
	}
	item.Success = IRNavigationIntent{ID: semanticID(string(id), "success")}
	if decl.Stay {
		item.Success.Kind = "same-context"
		item.Success.Page = page.Name.Text
	} else if decl.SuccessPage != nil {
		item.Success.Kind = "page"
		item.Success.Page = decl.SuccessPage.Text
	}
	if decl.Continuation != nil {
		item.Continuation = &IRNavigationIntent{ID: semanticID(string(id), "continuation"), Kind: "page", Page: decl.Continuation.Text}
	}
	sourceMap.add(id, "identity-interaction", decl.Span)
	sourceMap.add(item.Success.ID, "navigation", decl.Span)
	if item.Continuation != nil {
		sourceMap.add(item.Continuation.ID, "navigation", decl.Continuation.Span)
	}
	return item
}

func (c *checker) pageSemanticRequirements(page *PageDecl) []IRAccessRequirement {
	if page == nil || len(page.Requirements) == 0 {
		return nil
	}
	access := c.buildAccess(pageID(page.Name.Text), "temporary/access", page.Requirements, nil)
	return access.AllOf
}
