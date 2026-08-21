package compiler

import (
	"fmt"
	"reflect"
	"sort"
)

const ReviewRequirementsVersion = "forma/review-requirements/v0alpha5"

// ReviewRequirements contains implementation properties that Forma cannot
// mechanically prove from agent feedback. They remain separate from
// Acceptance Facts and never contribute to passed coverage counts.
type ReviewRequirements struct {
	Version       string              `json:"version"`
	IntentVersion string              `json:"intentVersion"`
	Requirements  []ReviewRequirement `json:"requirements"`
}

type ReviewRequirement struct {
	ID          SemanticID   `json:"id"`
	Kind        string       `json:"kind"`
	Subject     SemanticID   `json:"subject"`
	SourceNodes []SemanticID `json:"sourceNodes"`
	Instruction string       `json:"instruction"`
}

var reviewInstructions = map[string]string{
	"secret-redaction":                      "Review agent feedback, user-visible diagnostics, and repository logs; confirm that no runtime credential or verification-evidence value is exposed.",
	"secret-storage":                        "Review repository storage paths; confirm that credentials and verification evidence are not stored as plaintext domain data and use the repository's established secure mechanism.",
	"fixture-fidelity":                      "Review generated tests; confirm that semantic setup does not stub or directly inject the operation, authorization decision, or observation whose behavior the Acceptance Fact tests.",
	"concurrent-invariant-enforcement":      "Review every authoritative mutation boundary that can change a field referenced by this invariant; confirm that concurrent operations cannot commit a post-state that violates it and that enforcement is not limited to a user interface or single-threaded test.",
	"atomic-changes-enforcement":            "Review the action implementation; confirm that target identity resolution, pre-state reads, invariant checks, and commit share one transaction, lock, or conflict-retry boundary, and that concurrent invocation or process failure cannot commit only part of the transition and explicit Changes.",
	"cross-entity-write-authorization":      "Review the action access, every presenting surface's effective access, the changed entity and field, and existing surfaces that can change that field; confirm that the cross-entity write path is intentional without inferring the target surfaces' roles as additional action authorization.",
	"cross-entity-value-read-authorization": "Review the action access, every presenting surface's effective access, each relation value source entity and field and existing surfaces that present it, and each stored target field and existing surfaces that present it; confirm that roles allowed to invoke the action may use the source value and that downstream disclosure is intentional without inferring the source entity's surface roles as additional action authorization.",
	"exact-numeric-expression-enforcement":  "Review the action's Int or Decimal representation, addition operation, storage boundary, and failure path; confirm that wrap, binary floating-point rounding, or silent saturation cannot commit as success and that an unrepresentable exact result leaves state and every changed subject unmodified.",
}

// BuildReviewRequirements deterministically derives the human-review boundary
// from Resolved Intent. Empty applications receive an explicit empty artifact.
func BuildReviewRequirements(intent *ResolvedIntent) (*ReviewRequirements, error) {
	if err := ValidateResolvedIntent(intent); err != nil {
		return nil, err
	}
	requirements := make([]ReviewRequirement, 0, len(intent.Identities)*3)
	for _, identity := range intent.Identities {
		secretSources := []SemanticID{identity.ID}
		for _, proof := range identity.Proofs {
			secretSources = append(secretSources, proof.ID)
		}
		for _, credential := range identity.Credentials {
			secretSources = append(secretSources, credential.ID)
		}
		for _, verification := range identity.Verifications {
			secretSources = append(secretSources, verification.ID)
		}
		secretSources = canonicalSemanticIDs(secretSources)

		fixtureSources := []SemanticID{identity.ID, identity.Registration.ID}
		for _, verification := range identity.Verifications {
			fixtureSources = append(fixtureSources, verification.VerifyOperation, verification.ResendOperation)
		}
		fixtureSources = append(fixtureSources, identity.Authentication.SignInOperation, identity.Authentication.SignOutOperation)
		for _, ownership := range identity.Ownership {
			fixtureSources = append(fixtureSources, ownership.ID)
		}
		fixtureSources = canonicalSemanticIDs(fixtureSources)

		for _, item := range []struct {
			kind        string
			sourceNodes []SemanticID
		}{
			{kind: "secret-redaction", sourceNodes: secretSources},
			{kind: "secret-storage", sourceNodes: secretSources},
			{kind: "fixture-fidelity", sourceNodes: fixtureSources},
		} {
			requirements = append(requirements, ReviewRequirement{
				ID:          SemanticID("review/" + string(identity.ID) + "/" + item.kind),
				Kind:        item.kind,
				Subject:     identity.ID,
				SourceNodes: append([]SemanticID(nil), item.sourceNodes...),
				Instruction: reviewInstructions[item.kind],
			})
		}
	}
	for _, entity := range intent.Entities {
		for _, invariant := range entity.Invariants {
			kind := "concurrent-invariant-enforcement"
			requirements = append(requirements, ReviewRequirement{
				ID:          SemanticID("review/" + string(invariant.ID) + "/" + kind),
				Kind:        kind,
				Subject:     invariant.ID,
				SourceNodes: invariantFactSourceNodes(invariant),
				Instruction: reviewInstructions[kind],
			})
		}
	}
	for _, action := range intent.Actions {
		if len(action.Changes) == 0 {
			continue
		}
		atomicSources := actionReviewSources(intent, action, false)
		kind := "atomic-changes-enforcement"
		requirements = append(requirements, ReviewRequirement{
			ID: SemanticID("review/" + string(action.ID) + "/" + kind), Kind: kind, Subject: action.ID,
			SourceNodes: atomicSources, Instruction: reviewInstructions[kind],
		})
		crossEntity := false
		for _, change := range action.Changes {
			crossEntity = crossEntity || len(change.Target.RelationPath) != 0
		}
		if crossEntity {
			kind = "cross-entity-write-authorization"
			requirements = append(requirements, ReviewRequirement{
				ID: SemanticID("review/" + string(action.ID) + "/" + kind), Kind: kind, Subject: action.ID,
				SourceNodes: actionReviewSources(intent, action, true), Instruction: reviewInstructions[kind],
			})
		}
		crossEntityValue := false
		numericExpression := false
		for _, change := range action.Changes {
			for _, leaf := range expressionFieldReferenceNodes(change.Value) {
				crossEntityValue = crossEntityValue || len(leaf.RelationPath) != 0
			}
			numericExpression = numericExpression || expressionHasOperator(change.Value, "add")
		}
		if crossEntityValue {
			kind = "cross-entity-value-read-authorization"
			requirements = append(requirements, ReviewRequirement{
				ID: SemanticID("review/" + string(action.ID) + "/" + kind), Kind: kind, Subject: action.ID,
				SourceNodes: actionValueReadReviewSources(intent, action), Instruction: reviewInstructions[kind],
			})
		}
		if numericExpression {
			kind = "exact-numeric-expression-enforcement"
			requirements = append(requirements, ReviewRequirement{
				ID: SemanticID("review/" + string(action.ID) + "/" + kind), Kind: kind, Subject: action.ID,
				SourceNodes: actionReviewSources(intent, action, false), Instruction: reviewInstructions[kind],
			})
		}
	}
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].ID < requirements[j].ID })
	result := &ReviewRequirements{
		Version: ReviewRequirementsVersion, IntentVersion: intent.Version, Requirements: requirements,
	}
	if err := validateReviewRequirementsShape(intent, result); err != nil {
		return nil, err
	}
	return result, nil
}

func actionReviewSources(intent *ResolvedIntent, action IRAction, includeAuthorizationSurfaces bool) []SemanticID {
	sources := []SemanticID{action.ID}
	for _, entity := range intent.Entities {
		if entity.Name == action.Entity {
			sources = append(sources, entity.ID)
			if entity.State != nil {
				sources = append(sources, entity.State.ID)
			}
		}
	}
	targetFields := map[SemanticID]bool{}
	for _, change := range action.Changes {
		sources = append(sources, change.ID, change.Target.ID, change.Target.Field)
		sources = append(sources, change.Target.RelationPath...)
		sources = appendExpressionReviewSources(intent, sources, change.Value)
		targetFields[change.Target.Field] = true
		for _, entity := range intent.Entities {
			for _, field := range entity.Fields {
				if field.ID == change.Target.Field {
					sources = append(sources, entity.ID)
				}
			}
			for _, invariant := range entity.Invariants {
				for _, field := range invariantFieldReferences(invariant) {
					if targetFields[field] {
						sources = append(sources, invariantFactSourceNodes(invariant)...)
					}
				}
			}
		}
	}
	if !includeAuthorizationSurfaces {
		return canonicalSemanticIDs(sources)
	}
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			for _, ref := range view.Actions {
				if ref.Action != action.ID {
					continue
				}
				sources = append(sources, page.ID, view.ID, ref.ID, ref.Access.ID)
				for _, access := range ref.Access.AllOf {
					sources = append(sources, access.Source)
				}
			}
			if view.Submit == nil {
				continue
			}
			for _, fieldName := range view.Fields {
				field, ok := fieldByEntityAndName(intent, view.Entity, fieldName)
				if !ok || !targetFields[field.ID] {
					continue
				}
				sources = append(sources, page.ID, view.ID, view.Submit.ID, view.Submit.Access.ID, field.ID)
				for _, access := range view.Submit.Access.AllOf {
					sources = append(sources, access.Source)
				}
			}
		}
	}
	return canonicalSemanticIDs(sources)
}

func actionValueReadReviewSources(intent *ResolvedIntent, action IRAction) []SemanticID {
	sources := actionReviewSources(intent, action, false)
	fields := map[SemanticID]bool{}
	for _, change := range action.Changes {
		fields[change.Target.Field] = true
		for _, leaf := range expressionFieldReferenceNodes(change.Value) {
			fields[leaf.Field] = true
		}
	}
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			for _, ref := range view.Actions {
				if ref.Action != action.ID {
					continue
				}
				sources = append(sources, page.ID, view.ID, ref.ID, ref.Access.ID)
				for _, access := range ref.Access.AllOf {
					sources = append(sources, access.Source)
				}
			}
			presentsField := false
			for _, fieldName := range view.Fields {
				field, ok := fieldByEntityAndName(intent, view.Entity, fieldName)
				if ok && fields[field.ID] {
					presentsField = true
					sources = append(sources, field.ID)
				}
			}
			if !presentsField {
				continue
			}
			sources = append(sources, page.ID, view.ID)
			if page.Access != nil {
				sources = append(sources, page.Access.ID)
				for _, access := range page.Access.AllOf {
					sources = append(sources, access.Source)
				}
			}
		}
	}
	return canonicalSemanticIDs(sources)
}

func appendExpressionReviewSources(intent *ResolvedIntent, sources []SemanticID, expression IRExpression) []SemanticID {
	sources = append(sources, expressionSemanticIDs(expression)...)
	for _, leaf := range expressionFieldReferenceNodes(expression) {
		for _, entity := range intent.Entities {
			for _, field := range entity.Fields {
				if field.ID != leaf.Field {
					continue
				}
				sources = append(sources, entity.ID)
				for _, item := range intent.Types {
					if item.Name != field.Type {
						continue
					}
					sources = append(sources, item.ID)
					for _, constraint := range item.Constraints {
						sources = append(sources, constraint.ID)
					}
				}
			}
		}
	}
	return sources
}

func expressionHasOperator(expression IRExpression, operator string) bool {
	if expression.Operator == operator {
		return true
	}
	return expression.Left != nil && expressionHasOperator(*expression.Left, operator) ||
		expression.Right != nil && expressionHasOperator(*expression.Right, operator)
}

func fieldByEntityAndName(intent *ResolvedIntent, entityName, fieldName string) (IRField, bool) {
	for _, entity := range intent.Entities {
		if entity.Name != entityName {
			continue
		}
		for _, field := range entity.Fields {
			if field.Name == fieldName {
				return field, true
			}
		}
	}
	return IRField{}, false
}

// ValidateReviewRequirements rejects requests that omit, invent, or rewrite a
// compiler-owned review obligation.
func ValidateReviewRequirements(intent *ResolvedIntent, requirements *ReviewRequirements) error {
	if requirements == nil {
		return fmt.Errorf("validate Review Requirements: artifact is required")
	}
	canonical, err := BuildReviewRequirements(intent)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(canonical, requirements) {
		return fmt.Errorf("validate Review Requirements: artifact differs from canonical requirements derived from Resolved Intent")
	}
	return nil
}

func validateReviewRequirementsShape(intent *ResolvedIntent, requirements *ReviewRequirements) error {
	if requirements.Version != ReviewRequirementsVersion || requirements.IntentVersion != intent.Version {
		return fmt.Errorf("validate Review Requirements: schema versions do not match Resolved Intent")
	}
	semanticIDs, err := resolvedIntentSemanticIDs(intent)
	if err != nil {
		return fmt.Errorf("validate Review Requirements: %w", err)
	}
	seen := map[SemanticID]bool{}
	for index, requirement := range requirements.Requirements {
		if requirement.ID == "" || seen[requirement.ID] {
			return fmt.Errorf("validate Review Requirements: empty or duplicate requirement ID %s", requirement.ID)
		}
		seen[requirement.ID] = true
		if index > 0 && requirements.Requirements[index-1].ID >= requirement.ID {
			return fmt.Errorf("validate Review Requirements: requirements are not in canonical order")
		}
		if requirement.Instruction != reviewInstructions[requirement.Kind] || requirement.Instruction == "" {
			return fmt.Errorf("validate Review Requirements: requirement %s has a non-canonical instruction", requirement.ID)
		}
		if !semanticIDs[requirement.Subject] {
			return fmt.Errorf("validate Review Requirements: requirement %s has missing subject %s", requirement.ID, requirement.Subject)
		}
		if len(requirement.SourceNodes) == 0 || !containsSemanticID(requirement.SourceNodes, requirement.Subject) ||
			!reflect.DeepEqual(requirement.SourceNodes, canonicalSemanticIDs(requirement.SourceNodes)) {
			return fmt.Errorf("validate Review Requirements: requirement %s has non-canonical source nodes", requirement.ID)
		}
		for _, source := range requirement.SourceNodes {
			if !semanticIDs[source] {
				return fmt.Errorf("validate Review Requirements: requirement %s references missing source node %s", requirement.ID, source)
			}
		}
	}
	return nil
}

func ReviewRequirementIDs(requirements *ReviewRequirements) []SemanticID {
	if requirements == nil || len(requirements.Requirements) == 0 {
		return nil
	}
	result := make([]SemanticID, len(requirements.Requirements))
	for index, requirement := range requirements.Requirements {
		result[index] = requirement.ID
	}
	return result
}
