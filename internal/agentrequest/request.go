package agentrequest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/compiler"
)

const (
	RequestSchema  = "forma/generation-request/v0alpha1"
	FeedbackSchema = "forma/generation-feedback/v0alpha1"
)

type Request struct {
	Schema          string                    `json:"schema"`
	ResolvedIntent  *compiler.ResolvedIntent  `json:"resolvedIntent"`
	AcceptanceFacts *compiler.AcceptanceFacts `json:"acceptanceFacts"`
	SourceMap       *compiler.SourceMap       `json:"sourceMap"`
	RequestedChange RequestedChange           `json:"requestedChange"`
	Verification    VerificationPolicy        `json:"verification"`
}

type RequestedChange struct {
	Kind        string                `json:"kind"`
	IntentNodes []compiler.SemanticID `json:"intentNodes,omitempty"`
}

type VerificationPolicy struct {
	FeedbackSchema       string                `json:"feedbackSchema"`
	RequiredFactIDs      []compiler.SemanticID `json:"requiredFactIds"`
	RequireTestReference bool                  `json:"requireTestReference"`
	RejectUnknownFacts   bool                  `json:"rejectUnknownFacts"`
}

type Feedback struct {
	Schema             string                `json:"schema"`
	Stage              string                `json:"stage"`
	Status             string                `json:"status"`
	RelatedIntentNodes []compiler.SemanticID `json:"relatedIntentNodes,omitempty"`
	FactCoverage       []FactCoverage        `json:"factCoverage,omitempty"`
	Command            string                `json:"command,omitempty"`
	Diagnostics        []string              `json:"diagnostics,omitempty"`
	Summary            string                `json:"summary,omitempty"`
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
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
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
	if request.Schema != RequestSchema {
		return fmt.Errorf("validate Generation Request: schema %q is not %q", request.Schema, RequestSchema)
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
	canonicalFacts, err := compiler.BuildAcceptanceFacts(request.ResolvedIntent)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(canonicalFacts, request.AcceptanceFacts) {
		return fmt.Errorf("validate Generation Request: Acceptance Facts differ from canonical facts derived from Resolved Intent")
	}
	if request.Verification.FeedbackSchema != FeedbackSchema || !request.Verification.RequireTestReference || !request.Verification.RejectUnknownFacts {
		return fmt.Errorf("validate Generation Request: verification policy was weakened")
	}
	canonicalIDs := factIDs(canonicalFacts)
	if !reflect.DeepEqual(canonicalIDs, request.Verification.RequiredFactIDs) {
		return fmt.Errorf("validate Generation Request: required fact IDs differ from Acceptance Facts")
	}
	if request.RequestedChange.Kind != "full" {
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
	if feedback.Schema != FeedbackSchema {
		return fmt.Errorf("validate fact coverage: feedback schema %q is not %q", feedback.Schema, FeedbackSchema)
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

func factIDs(facts *compiler.AcceptanceFacts) []compiler.SemanticID {
	result := make([]compiler.SemanticID, 0, len(facts.Facts))
	for _, fact := range facts.Facts {
		result = append(result, fact.ID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
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
