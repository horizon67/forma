package agentrequest

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

// ErrNoChanges preserves the request command's contract: it emits a Generation
// Request or fails, never mixes a no-op message into its JSON output.
var ErrNoChanges = errors.New("build incremental Generation Request: baseline and current intent and policy are identical")

type PolicyChange struct {
	Kind     string `json:"kind"`
	PolicyID string `json:"policyId"`
}

// ConventionChange records advisory text, not an instruction to reverse the
// implementation when advice is removed. Text edits are removal plus addition.
type ConventionChange struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// GenerationPlan is an orchestration result, not a new wire request kind.
// On NoOp, Request is the current full snapshot for inspection only and must
// not be sent to an agent. Baseline identifies the explicitly supplied input;
// it does not prove that this input was applied to a repository.
type GenerationPlan struct {
	Request  Request
	Baseline *RequestBaseline
	NoOp     bool
}

// PlanGeneration shares full/incremental/no-op selection across the CLI and
// request builder. Neither prompt bytes nor source locations select an update.
func PlanGeneration(previous *Request, result compiler.Result, manifest *implementationpolicy.Manifest) (GenerationPlan, error) {
	if previous != nil {
		if err := ValidateRequest(*previous); err != nil {
			return GenerationPlan{}, fmt.Errorf("build incremental Generation Request: invalid baseline: %w", err)
		}
		if manifest == nil {
			manifest = previous.ImplementationPolicy
		}
	}
	current, err := BuildFullWithPolicy(result, manifest)
	if err != nil {
		return GenerationPlan{}, err
	}
	plan := GenerationPlan{Request: current}
	if previous == nil {
		return plan, nil
	}
	before, err := compilerOutputsForDiff(*previous)
	if err != nil {
		return GenerationPlan{}, fmt.Errorf("build incremental Generation Request: upgrade baseline: %w", err)
	}
	change := RequestedChange{Kind: "incremental"}
	change.IntentChanges, change.UnchangedIntentNodes, err = diffIntent(before.intent, current.ResolvedIntent)
	if err != nil {
		return GenerationPlan{}, err
	}
	change.FactChanges, change.UnchangedFacts, err = diffFacts(before.facts, current.AcceptanceFacts)
	if err != nil {
		return GenerationPlan{}, err
	}
	change.ReviewRequirementChanges, change.UnchangedReviewRequirements, err = diffReviewRequirements(before.reviews, current.ReviewRequirements)
	if err != nil {
		return GenerationPlan{}, err
	}
	for _, changes := range [][]SemanticChange{change.IntentChanges, change.FactChanges, change.ReviewRequirementChanges} {
		for _, item := range changes {
			if item.Kind == "removed" {
				return GenerationPlan{}, fmt.Errorf("build incremental Generation Request: removed nodes are not supported by the first incremental slice")
			}
		}
	}
	change.PolicyChanges, change.UnchangedPolicies, change.ConventionChanges, err = diffPolicies(previous.ImplementationPolicy, current.ImplementationPolicy)
	if err != nil {
		return GenerationPlan{}, err
	}
	content, err := Marshal(*previous)
	if err != nil {
		return GenerationPlan{}, err
	}
	plan.Baseline = &RequestBaseline{
		RequestSHA256: fmt.Sprintf("%x", sha256.Sum256(content)), RequestSchema: previous.Schema,
		ResolvedIntentVersion: previous.ResolvedIntent.Version, AcceptanceFactsVersion: previous.AcceptanceFacts.Version,
		SourceMapVersion: previous.SourceMap.Version, ReviewRequirementsVersion: reviewRequirementsVersion(*previous),
	}
	if !hasChanges(change) {
		plan.NoOp = true
		return plan, nil
	}
	baselineCopy := *plan.Baseline
	change.Baseline = &baselineCopy
	// A nil slice is canonical for policy-only updates, including after JSON
	// round-tripping the omitempty field.
	for _, item := range change.IntentChanges {
		change.IntentNodes = append(change.IntentNodes, item.NodeID)
	}
	plan.Request.RequestedChange = change
	if err := ValidateIncrementalBaseline(plan.Request, *previous); err != nil {
		return GenerationPlan{}, err
	}
	return plan, nil
}

func hasCurrentCompilerOutputs(schema string) bool {
	return schema == RequestSchema || schema == PreviousRequestSchema
}

func hasPolicyMetadata(change RequestedChange) bool {
	return len(change.PolicyChanges) != 0 || change.UnchangedPolicies != 0 || len(change.ConventionChanges) != 0
}

func hasChanges(change RequestedChange) bool {
	return len(change.IntentChanges) != 0 || len(change.FactChanges) != 0 || len(change.ReviewRequirementChanges) != 0 ||
		len(change.PolicyChanges) != 0 || len(change.ConventionChanges) != 0
}

func policiesByID(manifest *implementationpolicy.Manifest) map[string]implementationpolicy.Policy {
	result := map[string]implementationpolicy.Policy{}
	if manifest != nil {
		for _, policy := range manifest.Policies {
			result[policy.ID] = policy
		}
	}
	return result
}

func conventions(manifest *implementationpolicy.Manifest) []string {
	if manifest == nil || len(manifest.Conventions) == 0 {
		return nil
	}
	return manifest.Conventions
}

func diffPolicies(previous, current *implementationpolicy.Manifest) ([]PolicyChange, int, []ConventionChange, error) {
	before, after := policiesByID(previous), policiesByID(current)
	var ids, removed []string
	for id := range before {
		if _, exists := after[id]; !exists {
			removed = append(removed, id)
		}
	}
	if len(removed) != 0 {
		sort.Strings(removed)
		return nil, 0, nil, fmt.Errorf("build incremental Generation Request: removed policies are not supported: %s", removed[0])
	}
	for id := range after {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var changes []PolicyChange
	unchanged := 0
	for _, id := range ids {
		old, exists := before[id]
		switch {
		case !exists:
			changes = append(changes, PolicyChange{Kind: "added", PolicyID: id})
		case old != after[id]:
			changes = append(changes, PolicyChange{Kind: "changed", PolicyID: id})
		default:
			unchanged++
		}
	}
	return changes, unchanged, diffConventions(previous, current), nil
}

func diffConventions(previous, current *implementationpolicy.Manifest) []ConventionChange {
	before, after := map[string]bool{}, map[string]bool{}
	for _, value := range conventions(previous) {
		before[value] = true
	}
	for _, value := range conventions(current) {
		after[value] = true
	}
	var changes []ConventionChange
	for value := range before {
		if !after[value] {
			changes = append(changes, ConventionChange{Kind: "removed", Value: value})
		}
	}
	for value := range after {
		if !before[value] {
			changes = append(changes, ConventionChange{Kind: "added", Value: value})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Value < changes[j].Value })
	return changes
}

func validatePolicyChanges(request Request) error {
	change := request.RequestedChange
	policies := policiesByID(request.ImplementationPolicy)
	if change.UnchangedPolicies < 0 || change.UnchangedPolicies+len(change.PolicyChanges) != len(policies) {
		return fmt.Errorf("validate Generation Request: incremental policy counts differ from current Manifest")
	}
	var last string
	for _, item := range change.PolicyChanges {
		if item.PolicyID == "" || item.PolicyID <= last || (item.Kind != "added" && item.Kind != "changed") {
			return fmt.Errorf("validate Generation Request: invalid or noncanonical policy change %q", item.PolicyID)
		}
		if _, exists := policies[item.PolicyID]; !exists {
			return fmt.Errorf("validate Generation Request: changed policy %s is not in current Manifest", item.PolicyID)
		}
		last = item.PolicyID
	}
	if len(change.ConventionChanges) != 0 && request.ImplementationPolicy == nil {
		return fmt.Errorf("validate Generation Request: conventions change requires a Manifest")
	}
	current := map[string]bool{}
	for _, value := range conventions(request.ImplementationPolicy) {
		current[value] = true
	}
	last = ""
	for _, item := range change.ConventionChanges {
		if item.Value == "" || strings.TrimSpace(item.Value) != item.Value || strings.ContainsAny(item.Value, "\r\n") || item.Value <= last ||
			(item.Kind != "added" && item.Kind != "removed") {
			return fmt.Errorf("validate Generation Request: invalid or noncanonical convention change %q", item.Value)
		}
		if current[item.Value] != (item.Kind == "added") {
			return fmt.Errorf("validate Generation Request: convention change %q conflicts with current Manifest", item.Value)
		}
		last = item.Value
	}
	return nil
}
