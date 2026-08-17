package compiler

import (
	"fmt"
	"reflect"
	"sort"
)

const ReviewRequirementsVersion = "forma/review-requirements/v0alpha1"

// ReviewRequirements contains security properties that Forma cannot
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

var identityReviewInstructions = map[string]string{
	"secret-redaction": "Review agent feedback, user-visible diagnostics, and repository logs; confirm that no runtime credential or verification-evidence value is exposed.",
	"secret-storage":   "Review repository storage paths; confirm that credentials and verification evidence are not stored as plaintext domain data and use the repository's established secure mechanism.",
	"fixture-fidelity": "Review generated tests; confirm that semantic setup does not stub or directly inject the operation, authorization decision, or observation whose behavior the Acceptance Fact tests.",
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
				Instruction: identityReviewInstructions[item.kind],
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
		if requirement.Instruction != identityReviewInstructions[requirement.Kind] || requirement.Instruction == "" {
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
