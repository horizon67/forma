package agentrequest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

const (
	RequestSchema        = "forma/generation-request/v0alpha2"
	FeedbackSchema       = "forma/generation-feedback/v0alpha2"
	LegacyRequestSchema  = "forma/generation-request/v0alpha1"
	LegacyFeedbackSchema = "forma/generation-feedback/v0alpha1"
)

type Request struct {
	Schema               string                         `json:"schema"`
	ResolvedIntent       *compiler.ResolvedIntent       `json:"resolvedIntent"`
	AcceptanceFacts      *compiler.AcceptanceFacts      `json:"acceptanceFacts"`
	SourceMap            *compiler.SourceMap            `json:"sourceMap"`
	ImplementationPolicy *implementationpolicy.Manifest `json:"implementationPolicy,omitempty"`
	RequestedChange      RequestedChange                `json:"requestedChange"`
	Verification         VerificationPolicy             `json:"verification"`
}

type RequestedChange struct {
	Kind                 string                `json:"kind"`
	Baseline             *RequestBaseline      `json:"baseline,omitempty"`
	IntentNodes          []compiler.SemanticID `json:"intentNodes,omitempty"`
	IntentChanges        []SemanticChange      `json:"intentChanges,omitempty"`
	FactChanges          []SemanticChange      `json:"factChanges,omitempty"`
	UnchangedIntentNodes int                   `json:"unchangedIntentNodes,omitempty"`
	UnchangedFacts       int                   `json:"unchangedFacts,omitempty"`
}

type RequestBaseline struct {
	RequestSHA256          string `json:"requestSha256"`
	RequestSchema          string `json:"requestSchema"`
	ResolvedIntentVersion  string `json:"resolvedIntentVersion"`
	AcceptanceFactsVersion string `json:"acceptanceFactsVersion"`
}

type SemanticChange struct {
	Kind   string              `json:"kind"`
	NodeID compiler.SemanticID `json:"nodeId"`
}

type VerificationPolicy struct {
	FeedbackSchema       string                `json:"feedbackSchema"`
	RequiredFactIDs      []compiler.SemanticID `json:"requiredFactIds"`
	RequireTestReference bool                  `json:"requireTestReference"`
	RejectUnknownFacts   bool                  `json:"rejectUnknownFacts"`
}

type Feedback struct {
	Schema             string                          `json:"schema"`
	Stage              string                          `json:"stage"`
	Status             string                          `json:"status"`
	RelatedIntentNodes []compiler.SemanticID           `json:"relatedIntentNodes,omitempty"`
	FactCoverage       []FactCoverage                  `json:"factCoverage,omitempty"`
	Command            string                          `json:"command,omitempty"`
	Diagnostics        []string                        `json:"diagnostics,omitempty"`
	PolicyCoverage     []implementationpolicy.Coverage `json:"policyCoverage,omitempty"`
	Summary            string                          `json:"summary,omitempty"`
}

type FactCoverage struct {
	FactID         compiler.SemanticID `json:"factId"`
	TestReferences []string            `json:"testReferences"`
	Result         string              `json:"result"`
}

type CoverageSummary struct {
	FactCount       int
	DistinctTests   int
	MaxFactsPerTest int
}

func BuildFull(result compiler.Result) (Request, error) {
	return BuildFullWithPolicy(result, nil)
}

func BuildFullWithPolicy(result compiler.Result, manifest *implementationpolicy.Manifest) (Request, error) {
	if len(result.Diagnostics) != 0 {
		return Request{}, fmt.Errorf("build Generation Request: compilation has diagnostics")
	}
	if result.Intent == nil || result.SourceMap == nil {
		return Request{}, fmt.Errorf("build Generation Request: compiler output is incomplete")
	}
	facts, err := compiler.BuildAcceptanceFacts(result.Intent)
	if err != nil {
		return Request{}, err
	}
	required := make([]compiler.SemanticID, 0, len(facts.Facts))
	for _, fact := range facts.Facts {
		required = append(required, fact.ID)
	}
	sort.Slice(required, func(i, j int) bool { return required[i] < required[j] })
	request := Request{
		Schema: RequestSchema, ResolvedIntent: result.Intent, AcceptanceFacts: facts, SourceMap: result.SourceMap,
		RequestedChange: RequestedChange{Kind: "full"},
		Verification: VerificationPolicy{
			FeedbackSchema: FeedbackSchema, RequiredFactIDs: required,
			RequireTestReference: true, RejectUnknownFacts: true,
		},
	}
	if manifest != nil {
		normalized, err := implementationpolicy.Normalize(*manifest)
		if err != nil {
			return Request{}, err
		}
		request.ImplementationPolicy = &normalized
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func BuildIncremental(previous Request, result compiler.Result, manifest *implementationpolicy.Manifest) (Request, error) {
	if err := ValidateRequest(previous); err != nil {
		return Request{}, fmt.Errorf("build incremental Generation Request: invalid baseline: %w", err)
	}
	if manifest == nil && previous.ImplementationPolicy != nil {
		preserved := *previous.ImplementationPolicy
		manifest = &preserved
	}
	current, err := BuildFullWithPolicy(result, manifest)
	if err != nil {
		return Request{}, err
	}
	intentChanges, unchangedIntent, err := diffIntent(previous.ResolvedIntent, current.ResolvedIntent)
	if err != nil {
		return Request{}, fmt.Errorf("build incremental Generation Request: diff Resolved Intent: %w", err)
	}
	factChanges, unchangedFacts, err := diffFacts(previous.AcceptanceFacts, current.AcceptanceFacts)
	if err != nil {
		return Request{}, fmt.Errorf("build incremental Generation Request: diff Acceptance Facts: %w", err)
	}
	for _, change := range append(append([]SemanticChange(nil), intentChanges...), factChanges...) {
		if change.Kind == "removed" {
			return Request{}, fmt.Errorf("build incremental Generation Request: removed nodes are not supported by the first incremental slice")
		}
	}
	if len(intentChanges) == 0 && len(factChanges) == 0 {
		return Request{}, fmt.Errorf("build incremental Generation Request: baseline and current intent are identical")
	}
	baselineContent, err := Marshal(previous)
	if err != nil {
		return Request{}, fmt.Errorf("build incremental Generation Request: marshal baseline: %w", err)
	}
	digest := sha256.Sum256(baselineContent)
	intentNodes := make([]compiler.SemanticID, 0, len(intentChanges))
	for _, change := range intentChanges {
		intentNodes = append(intentNodes, change.NodeID)
	}
	current.RequestedChange = RequestedChange{
		Kind: "incremental",
		Baseline: &RequestBaseline{
			RequestSHA256: fmt.Sprintf("%x", digest), RequestSchema: previous.Schema,
			ResolvedIntentVersion:  previous.ResolvedIntent.Version,
			AcceptanceFactsVersion: previous.AcceptanceFacts.Version,
		},
		IntentNodes: intentNodes, IntentChanges: intentChanges, FactChanges: factChanges,
		UnchangedIntentNodes: unchangedIntent, UnchangedFacts: unchangedFacts,
	}
	if err := ValidateRequest(current); err != nil {
		return Request{}, err
	}
	return current, nil
}

func Marshal(request Request) ([]byte, error) {
	return json.MarshalIndent(request, "", "  ")
}

func UnmarshalRequest(content []byte) (Request, error) {
	var request Request
	if err := unmarshalExact(content, &request); err != nil {
		return Request{}, fmt.Errorf("decode Generation Request: %w", err)
	}
	return request, nil
}

func UnmarshalFeedback(content []byte) (Feedback, error) {
	var feedback Feedback
	if err := unmarshalExact(content, &feedback); err != nil {
		return Feedback{}, fmt.Errorf("decode Generation Feedback: %w", err)
	}
	return feedback, nil
}

// ValidateRequest verifies duplicated request metadata against the canonical
// compiler outputs. The orchestration layer must retain this compiler-produced
// request as immutable input and must not validate against a copy returned by
// the coding agent.
func ValidateRequest(request Request) error {
	if request.Schema != RequestSchema && request.Schema != LegacyRequestSchema {
		return fmt.Errorf("validate Generation Request: unsupported schema %q", request.Schema)
	}
	if request.ResolvedIntent == nil || request.AcceptanceFacts == nil || request.SourceMap == nil {
		return fmt.Errorf("validate Generation Request: compiler output is incomplete")
	}
	if request.ResolvedIntent.Version != compiler.ResolvedIntentVersion {
		return fmt.Errorf(
			"validate Generation Request: request uses Resolved Intent version %q, this binary supports %q; verify with the matching Forma version",
			request.ResolvedIntent.Version, compiler.ResolvedIntentVersion,
		)
	}
	if request.AcceptanceFacts.Version != compiler.AcceptanceFactsVersion {
		return fmt.Errorf(
			"validate Generation Request: request uses Acceptance Facts version %q, this binary supports %q; verify with the matching Forma version",
			request.AcceptanceFacts.Version, compiler.AcceptanceFactsVersion,
		)
	}
	if request.SourceMap.Version != compiler.SourceMapVersion {
		return fmt.Errorf(
			"validate Generation Request: request uses Source Map version %q, this binary supports %q; verify with the matching Forma version",
			request.SourceMap.Version, compiler.SourceMapVersion,
		)
	}
	if request.AcceptanceFacts.IntentVersion != request.ResolvedIntent.Version || request.SourceMap.IntentVersion != request.ResolvedIntent.Version {
		return fmt.Errorf("validate Generation Request: compiler output versions do not match")
	}
	if err := compiler.ValidateSourceMapCoverage(request.ResolvedIntent, request.SourceMap); err != nil {
		return err
	}
	if request.ImplementationPolicy != nil {
		if err := implementationpolicy.ValidateCanonical(*request.ImplementationPolicy); err != nil {
			return err
		}
	}
	canonicalFacts, err := compiler.BuildAcceptanceFacts(request.ResolvedIntent)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(canonicalFacts, request.AcceptanceFacts) {
		return fmt.Errorf("validate Generation Request: Acceptance Facts differ from canonical facts derived from Resolved Intent")
	}
	expectedFeedbackSchema := FeedbackSchema
	if request.Schema == LegacyRequestSchema {
		expectedFeedbackSchema = LegacyFeedbackSchema
		if request.ImplementationPolicy != nil || request.RequestedChange.Kind != "full" {
			return fmt.Errorf("validate Generation Request: legacy schema does not support implementation policy or incremental changes")
		}
	}
	if request.Verification.FeedbackSchema != expectedFeedbackSchema || !request.Verification.RequireTestReference || !request.Verification.RejectUnknownFacts {
		return fmt.Errorf("validate Generation Request: verification policy was weakened")
	}
	canonicalIDs := factIDs(canonicalFacts)
	if !reflect.DeepEqual(canonicalIDs, request.Verification.RequiredFactIDs) {
		return fmt.Errorf("validate Generation Request: required fact IDs differ from Acceptance Facts")
	}
	switch request.RequestedChange.Kind {
	case "full":
		if request.RequestedChange.Baseline != nil || len(request.RequestedChange.IntentNodes) != 0 ||
			len(request.RequestedChange.IntentChanges) != 0 || len(request.RequestedChange.FactChanges) != 0 ||
			request.RequestedChange.UnchangedIntentNodes != 0 || request.RequestedChange.UnchangedFacts != 0 {
			return fmt.Errorf("validate Generation Request: full request contains incremental change metadata")
		}
	case "incremental":
		if err := validateIncrementalChange(request); err != nil {
			return err
		}
	default:
		return fmt.Errorf("validate Generation Request: unsupported requested change kind %q", request.RequestedChange.Kind)
	}
	allowedStates := map[string]bool{"empty": true, "invalid": true, "failure": true}
	for _, page := range request.ResolvedIntent.Pages {
		for _, view := range page.Views {
			for _, state := range view.InteractionStates {
				if !allowedStates[state] {
					return fmt.Errorf("validate Generation Request: view %s contains implementation-shaped interaction state %q", view.ID, state)
				}
			}
		}
	}
	return nil
}

func SummarizeCoverage(feedback Feedback) CoverageSummary {
	factsByTest := map[string]int{}
	for _, coverage := range feedback.FactCoverage {
		for _, reference := range coverage.TestReferences {
			factsByTest[reference]++
		}
	}
	summary := CoverageSummary{FactCount: len(feedback.FactCoverage), DistinctTests: len(factsByTest)}
	for _, count := range factsByTest {
		if count > summary.MaxFactsPerTest {
			summary.MaxFactsPerTest = count
		}
	}
	return summary
}

// ValidateCoverage checks the target-neutral completion condition. It does not
// judge whether target-specific tests faithfully implement a fact; it prevents
// facts from being silently omitted or invented.
func ValidateCoverage(request Request, feedback Feedback) error {
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if feedback.Schema != request.Verification.FeedbackSchema {
		return fmt.Errorf("validate fact coverage: feedback schema %q is not %q", feedback.Schema, request.Verification.FeedbackSchema)
	}
	if feedback.Status != "succeeded" && feedback.Status != "failed" && feedback.Status != "blocked" {
		return fmt.Errorf("validate fact coverage: unknown feedback status %q", feedback.Status)
	}
	requiredIDs := factIDs(request.AcceptanceFacts)
	required := make(map[compiler.SemanticID]bool, len(requiredIDs))
	for _, id := range requiredIDs {
		required[id] = true
	}
	seen := map[compiler.SemanticID]bool{}
	for _, coverage := range feedback.FactCoverage {
		if seen[coverage.FactID] {
			return fmt.Errorf("validate fact coverage: duplicate fact %s", coverage.FactID)
		}
		seen[coverage.FactID] = true
		if !required[coverage.FactID] {
			return fmt.Errorf("validate fact coverage: unknown fact %s", coverage.FactID)
		}
		if feedback.Status == "succeeded" {
			if len(coverage.TestReferences) == 0 {
				return fmt.Errorf("validate fact coverage: fact %s has no test reference", coverage.FactID)
			}
			if err := validateTestReferences(coverage.FactID, coverage.TestReferences); err != nil {
				return err
			}
			if coverage.Result != "passed" {
				return fmt.Errorf("validate fact coverage: fact %s is %s, want passed", coverage.FactID, coverage.Result)
			}
		}
	}
	if feedback.Status == "succeeded" {
		for _, id := range requiredIDs {
			if !seen[id] {
				return fmt.Errorf("validate fact coverage: required fact %s is missing", id)
			}
		}
	}
	return nil
}

func ValidateIncrementalBaseline(request, baseline Request) error {
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if request.RequestedChange.Kind != "incremental" || request.RequestedChange.Baseline == nil {
		return fmt.Errorf("validate incremental baseline: request is not incremental")
	}
	if err := ValidateRequest(baseline); err != nil {
		return fmt.Errorf("validate incremental baseline: invalid baseline request: %w", err)
	}
	recorded := request.RequestedChange.Baseline
	if recorded.RequestSchema != baseline.Schema || recorded.ResolvedIntentVersion != baseline.ResolvedIntent.Version ||
		recorded.AcceptanceFactsVersion != baseline.AcceptanceFacts.Version {
		return fmt.Errorf("validate incremental baseline: baseline versions do not match recorded versions")
	}
	content, err := Marshal(baseline)
	if err != nil {
		return fmt.Errorf("validate incremental baseline: marshal baseline: %w", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	if digest != recorded.RequestSHA256 {
		return fmt.Errorf("validate incremental baseline: canonical baseline digest %s does not match recorded %s", digest, recorded.RequestSHA256)
	}
	intentChanges, unchangedIntent, err := diffIntent(baseline.ResolvedIntent, request.ResolvedIntent)
	if err != nil {
		return fmt.Errorf("validate incremental baseline: diff Resolved Intent: %w", err)
	}
	factChanges, unchangedFacts, err := diffFacts(baseline.AcceptanceFacts, request.AcceptanceFacts)
	if err != nil {
		return fmt.Errorf("validate incremental baseline: diff Acceptance Facts: %w", err)
	}
	intentNodes := make([]compiler.SemanticID, 0, len(intentChanges))
	for _, change := range intentChanges {
		intentNodes = append(intentNodes, change.NodeID)
	}
	requested := request.RequestedChange
	if !reflect.DeepEqual(intentChanges, requested.IntentChanges) ||
		!reflect.DeepEqual(factChanges, requested.FactChanges) ||
		!reflect.DeepEqual(intentNodes, requested.IntentNodes) ||
		unchangedIntent != requested.UnchangedIntentNodes || unchangedFacts != requested.UnchangedFacts {
		return fmt.Errorf("validate incremental baseline: recorded semantic changes differ from baseline-derived changes")
	}
	return nil
}

func ValidateCompletion(request Request, baseline *Request, feedback Feedback, repositoryRoot string) error {
	if err := ValidateCoverage(request, feedback); err != nil {
		return err
	}
	switch request.RequestedChange.Kind {
	case "full":
		if baseline != nil {
			return fmt.Errorf("validate completion: full request must not have an incremental baseline")
		}
	case "incremental":
		if baseline == nil {
			return fmt.Errorf("validate completion: incremental request requires its baseline request")
		}
		if err := ValidateIncrementalBaseline(request, *baseline); err != nil {
			return err
		}
	}
	if request.ImplementationPolicy == nil {
		if len(feedback.PolicyCoverage) != 0 {
			return fmt.Errorf("validate implementation policy coverage: feedback contains policies but request has no Implementation Policy Manifest")
		}
		return nil
	}
	if feedback.Status != "succeeded" {
		return nil
	}
	return implementationpolicy.ValidateCoverage(*request.ImplementationPolicy, feedback.PolicyCoverage, repositoryRoot)
}

func validateIncrementalChange(request Request) error {
	change := request.RequestedChange
	if change.Baseline == nil {
		return fmt.Errorf("validate Generation Request: incremental request has no baseline")
	}
	if len(change.Baseline.RequestSHA256) != sha256.Size*2 {
		return fmt.Errorf("validate Generation Request: incremental baseline has invalid SHA-256 digest")
	}
	for _, character := range change.Baseline.RequestSHA256 {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return fmt.Errorf("validate Generation Request: incremental baseline has invalid SHA-256 digest")
		}
	}
	if (change.Baseline.RequestSchema != RequestSchema && change.Baseline.RequestSchema != LegacyRequestSchema) ||
		change.Baseline.ResolvedIntentVersion != compiler.ResolvedIntentVersion ||
		change.Baseline.AcceptanceFactsVersion != compiler.AcceptanceFactsVersion {
		return fmt.Errorf("validate Generation Request: incremental baseline versions are unsupported")
	}
	if len(change.IntentChanges) == 0 && len(change.FactChanges) == 0 {
		return fmt.Errorf("validate Generation Request: incremental request has no changes")
	}
	if change.UnchangedIntentNodes < 0 || change.UnchangedFacts < 0 {
		return fmt.Errorf("validate Generation Request: incremental unchanged counts must not be negative")
	}
	intentNodes, err := resolvedIntentNodes(request.ResolvedIntent)
	if err != nil {
		return fmt.Errorf("validate Generation Request: index Resolved Intent: %w", err)
	}
	canonicalIntentNodes := make([]compiler.SemanticID, 0, len(change.IntentChanges))
	if err := validateSemanticChanges("intent", change.IntentChanges, intentNodes); err != nil {
		return err
	}
	for _, item := range change.IntentChanges {
		canonicalIntentNodes = append(canonicalIntentNodes, item.NodeID)
	}
	if !reflect.DeepEqual(canonicalIntentNodes, change.IntentNodes) {
		return fmt.Errorf("validate Generation Request: incremental intentNodes differ from intentChanges")
	}
	factNodes, err := acceptanceFactNodes(request.AcceptanceFacts)
	if err != nil {
		return fmt.Errorf("validate Generation Request: index Acceptance Facts: %w", err)
	}
	if err := validateSemanticChanges("fact", change.FactChanges, factNodes); err != nil {
		return err
	}
	return nil
}

func validateSemanticChanges(label string, changes []SemanticChange, current map[compiler.SemanticID]json.RawMessage) error {
	var previous compiler.SemanticID
	for index, change := range changes {
		if change.Kind != "added" && change.Kind != "changed" {
			return fmt.Errorf("validate Generation Request: incremental %s %s has unsupported kind %q", label, change.NodeID, change.Kind)
		}
		if change.NodeID == "" {
			return fmt.Errorf("validate Generation Request: incremental %s change has empty node ID", label)
		}
		if index > 0 && previous >= change.NodeID {
			return fmt.Errorf("validate Generation Request: incremental %s changes are not in canonical order", label)
		}
		if _, ok := current[change.NodeID]; !ok {
			return fmt.Errorf("validate Generation Request: incremental %s %s is not in current compiler output", label, change.NodeID)
		}
		previous = change.NodeID
	}
	return nil
}

func factIDs(facts *compiler.AcceptanceFacts) []compiler.SemanticID {
	result := make([]compiler.SemanticID, 0, len(facts.Facts))
	for _, fact := range facts.Facts {
		result = append(result, fact.ID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func diffIntent(previous, current *compiler.ResolvedIntent) ([]SemanticChange, int, error) {
	previousNodes, err := resolvedIntentNodes(previous)
	if err != nil {
		return nil, 0, err
	}
	currentNodes, err := resolvedIntentNodes(current)
	if err != nil {
		return nil, 0, err
	}
	return diffSemanticNodes(previousNodes, currentNodes)
}

func diffFacts(previous, current *compiler.AcceptanceFacts) ([]SemanticChange, int, error) {
	previousNodes, err := acceptanceFactNodes(previous)
	if err != nil {
		return nil, 0, err
	}
	currentNodes, err := acceptanceFactNodes(current)
	if err != nil {
		return nil, 0, err
	}
	return diffSemanticNodes(previousNodes, currentNodes)
}

func diffSemanticNodes(previous, current map[compiler.SemanticID]json.RawMessage) ([]SemanticChange, int, error) {
	ids := make([]compiler.SemanticID, 0, len(previous)+len(current))
	seen := map[compiler.SemanticID]bool{}
	for id := range previous {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range current {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var changes []SemanticChange
	unchanged := 0
	for _, id := range ids {
		before, existed := previous[id]
		after, exists := current[id]
		switch {
		case !existed && exists:
			changes = append(changes, SemanticChange{Kind: "added", NodeID: id})
		case existed && !exists:
			changes = append(changes, SemanticChange{Kind: "removed", NodeID: id})
		case bytes.Equal(before, after):
			unchanged++
		default:
			changes = append(changes, SemanticChange{Kind: "changed", NodeID: id})
		}
	}
	return changes, unchanged, nil
}

func resolvedIntentNodes(intent *compiler.ResolvedIntent) (map[compiler.SemanticID]json.RawMessage, error) {
	if intent == nil {
		return nil, fmt.Errorf("nil Resolved Intent")
	}
	content, err := json.Marshal(intent)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, err
	}
	result := map[compiler.SemanticID]json.RawMessage{}
	if err := collectResolvedNodes(root, result); err != nil {
		return nil, err
	}
	return result, nil
}

func collectResolvedNodes(value any, result map[compiler.SemanticID]json.RawMessage) error {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if err := collectResolvedNodes(child, result); err != nil {
				return err
			}
		}
	case map[string]any:
		if rawID, ok := item["id"]; ok {
			id, ok := rawID.(string)
			if !ok || id == "" {
				return fmt.Errorf("resolved node has invalid id")
			}
			content, err := json.Marshal(item)
			if err != nil {
				return err
			}
			semanticID := compiler.SemanticID(id)
			if _, exists := result[semanticID]; exists {
				return fmt.Errorf("duplicate resolved node %s", semanticID)
			}
			result[semanticID] = content
		}
		for _, child := range item {
			if err := collectResolvedNodes(child, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func acceptanceFactNodes(facts *compiler.AcceptanceFacts) (map[compiler.SemanticID]json.RawMessage, error) {
	if facts == nil {
		return nil, fmt.Errorf("nil Acceptance Facts")
	}
	result := make(map[compiler.SemanticID]json.RawMessage, len(facts.Facts))
	for _, fact := range facts.Facts {
		if fact.ID == "" {
			return nil, fmt.Errorf("Acceptance Fact has empty ID")
		}
		if _, exists := result[fact.ID]; exists {
			return nil, fmt.Errorf("duplicate Acceptance Fact %s", fact.ID)
		}
		content, err := json.Marshal(fact)
		if err != nil {
			return nil, err
		}
		result[fact.ID] = content
	}
	return result, nil
}

func validateTestReferences(factID compiler.SemanticID, references []string) error {
	seen := map[string]bool{}
	for _, reference := range references {
		if reference == "" || strings.TrimSpace(reference) != reference || strings.ContainsAny(reference, "\r\n") {
			return fmt.Errorf("validate fact coverage: fact %s has invalid test reference %q", factID, reference)
		}
		parts := strings.SplitN(reference, "#", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("validate fact coverage: fact %s test reference %q must be repository/path#test-identifier", factID, reference)
		}
		clean := path.Clean(parts[0])
		if path.IsAbs(parts[0]) || clean != parts[0] || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("validate fact coverage: fact %s test reference %q is not repository-relative", factID, reference)
		}
		if seen[reference] {
			return fmt.Errorf("validate fact coverage: fact %s repeats test reference %q", factID, reference)
		}
		seen[reference] = true
	}
	return nil
}

func unmarshalExact(content []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
