package agentrequest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

const (
	historicalResolvedIntentVersion  = "forma/resolved-intent/v0.4"
	historicalAcceptanceFactsVersion = "forma/acceptance-facts/v0alpha1"
	historicalSourceMapVersion       = "forma/source-map/v0.2"
	noReviewRequirementsVersion      = "none"
)

type compilerOutputSet struct {
	intent  *compiler.ResolvedIntent
	facts   *compiler.AcceptanceFacts
	reviews *compiler.ReviewRequirements
	sources *compiler.SourceMap
}

// historicalRequestCodec is the exact v0alpha1/v0alpha2 top-level shape. It
// deliberately excludes fields introduced by current Request structs.
type historicalRequestCodec struct {
	Schema               string                         `json:"schema"`
	ResolvedIntent       *compiler.ResolvedIntent       `json:"resolvedIntent"`
	AcceptanceFacts      *compiler.AcceptanceFacts      `json:"acceptanceFacts"`
	SourceMap            *compiler.SourceMap            `json:"sourceMap"`
	ImplementationPolicy *implementationpolicy.Manifest `json:"implementationPolicy,omitempty"`
	RequestedChange      historicalRequestedChange      `json:"requestedChange"`
	Verification         historicalVerificationPolicy   `json:"verification"`
}

type historicalRequestedChange struct {
	Kind                 string                `json:"kind"`
	Baseline             *historicalBaseline   `json:"baseline,omitempty"`
	IntentNodes          []compiler.SemanticID `json:"intentNodes,omitempty"`
	IntentChanges        []SemanticChange      `json:"intentChanges,omitempty"`
	FactChanges          []SemanticChange      `json:"factChanges,omitempty"`
	UnchangedIntentNodes int                   `json:"unchangedIntentNodes,omitempty"`
	UnchangedFacts       int                   `json:"unchangedFacts,omitempty"`
}

type historicalBaseline struct {
	RequestSHA256          string `json:"requestSha256"`
	RequestSchema          string `json:"requestSchema"`
	ResolvedIntentVersion  string `json:"resolvedIntentVersion"`
	AcceptanceFactsVersion string `json:"acceptanceFactsVersion"`
}

type historicalVerificationPolicy struct {
	FeedbackSchema       string                `json:"feedbackSchema"`
	RequiredFactIDs      []compiler.SemanticID `json:"requiredFactIds"`
	RequireTestReference bool                  `json:"requireTestReference"`
	RejectUnknownFacts   bool                  `json:"rejectUnknownFacts"`
}

func marshalRequestForSchema(request Request) ([]byte, error) {
	switch request.Schema {
	case LegacyRequestSchema, HistoricalIncrementalRequestSchema:
		var baseline *historicalBaseline
		if request.RequestedChange.Baseline != nil {
			baseline = &historicalBaseline{
				RequestSHA256: request.RequestedChange.Baseline.RequestSHA256, RequestSchema: request.RequestedChange.Baseline.RequestSchema,
				ResolvedIntentVersion:  request.RequestedChange.Baseline.ResolvedIntentVersion,
				AcceptanceFactsVersion: request.RequestedChange.Baseline.AcceptanceFactsVersion,
			}
		}
		encoded := historicalRequestCodec{
			Schema: request.Schema, ResolvedIntent: request.ResolvedIntent, AcceptanceFacts: request.AcceptanceFacts,
			SourceMap: request.SourceMap, ImplementationPolicy: request.ImplementationPolicy,
			RequestedChange: historicalRequestedChange{
				Kind: request.RequestedChange.Kind, Baseline: baseline, IntentNodes: request.RequestedChange.IntentNodes,
				IntentChanges: request.RequestedChange.IntentChanges, FactChanges: request.RequestedChange.FactChanges,
				UnchangedIntentNodes: request.RequestedChange.UnchangedIntentNodes, UnchangedFacts: request.RequestedChange.UnchangedFacts,
			},
			Verification: historicalVerificationPolicy{
				FeedbackSchema: request.Verification.FeedbackSchema, RequiredFactIDs: request.Verification.RequiredFactIDs,
				RequireTestReference: request.Verification.RequireTestReference, RejectUnknownFacts: request.Verification.RejectUnknownFacts,
			},
		}
		content, err := json.MarshalIndent(encoded, "", "  ")
		if err != nil {
			return nil, err
		}
		return removeHistoricalEmptyAccessKinds(content), nil
	case RequestSchema:
		return json.MarshalIndent(request, "", "  ")
	default:
		return nil, fmt.Errorf("marshal Generation Request: unsupported schema %q", request.Schema)
	}
}

func removeHistoricalEmptyAccessKinds(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	result := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if bytes.Equal(bytes.TrimSpace(line), []byte(`"kind": "",`)) {
			continue
		}
		result = append(result, line)
	}
	return bytes.Join(result, []byte("\n"))
}

func validateHistoricalRequest(request Request) error {
	if request.Schema != LegacyRequestSchema && request.Schema != HistoricalIncrementalRequestSchema {
		return fmt.Errorf("validate historical Generation Request: unsupported schema %q", request.Schema)
	}
	if request.ReviewRequirements != nil || len(request.Verification.DisplayReviewRequirementIDs) != 0 ||
		len(request.RequestedChange.ReviewRequirementChanges) != 0 || request.RequestedChange.UnchangedReviewRequirements != 0 {
		return fmt.Errorf("validate historical Generation Request: current review fields are not allowed")
	}
	outputs, err := upgradeHistoricalCompilerOutputs(request)
	if err != nil {
		return err
	}
	if err := compiler.ValidateSourceMapCoverage(outputs.intent, outputs.sources); err != nil {
		return err
	}
	canonicalFacts, err := compiler.BuildAcceptanceFacts(outputs.intent)
	if err != nil {
		return err
	}
	if !canonicalJSONEqual(canonicalFacts, outputs.facts) {
		return fmt.Errorf("validate historical Generation Request: Acceptance Facts differ from the version-dispatched canonical facts")
	}
	if len(outputs.reviews.Requirements) != 0 {
		return fmt.Errorf("validate historical Generation Request: historical Identity semantics are unsupported")
	}
	if request.ImplementationPolicy != nil {
		if request.Schema == LegacyRequestSchema {
			return fmt.Errorf("validate historical Generation Request: v0alpha1 does not support Implementation Policy")
		}
		if err := implementationpolicy.ValidateCanonical(*request.ImplementationPolicy); err != nil {
			return err
		}
	}
	wantFeedback := FeedbackSchema
	if request.Schema == LegacyRequestSchema {
		wantFeedback = LegacyFeedbackSchema
	}
	if request.Verification.FeedbackSchema != wantFeedback || !request.Verification.RequireTestReference || !request.Verification.RejectUnknownFacts {
		return fmt.Errorf("validate historical Generation Request: verification policy was weakened")
	}
	if !reflect.DeepEqual(factIDs(outputs.facts), request.Verification.RequiredFactIDs) {
		return fmt.Errorf("validate historical Generation Request: required fact IDs differ from canonical facts")
	}
	switch request.RequestedChange.Kind {
	case "full":
		if request.Schema != LegacyRequestSchema || !emptyHistoricalChange(request.RequestedChange) {
			return fmt.Errorf("validate historical Generation Request: invalid full request metadata")
		}
	case "incremental":
		if request.Schema != HistoricalIncrementalRequestSchema {
			return fmt.Errorf("validate historical Generation Request: v0alpha1 does not support incremental changes")
		}
		if err := validateHistoricalIncrementalChange(request, outputs); err != nil {
			return err
		}
	default:
		return fmt.Errorf("validate historical Generation Request: unsupported requested change kind %q", request.RequestedChange.Kind)
	}
	return nil
}

func emptyHistoricalChange(change RequestedChange) bool {
	return change.Baseline == nil && len(change.IntentNodes) == 0 && len(change.IntentChanges) == 0 && len(change.FactChanges) == 0 &&
		change.UnchangedIntentNodes == 0 && change.UnchangedFacts == 0
}

func validateHistoricalIncrementalChange(request Request, outputs compilerOutputSet) error {
	change := request.RequestedChange
	if change.Baseline == nil || !validSHA256(change.Baseline.RequestSHA256) || change.Baseline.RequestSchema != LegacyRequestSchema ||
		change.Baseline.ResolvedIntentVersion != historicalResolvedIntentVersion ||
		change.Baseline.AcceptanceFactsVersion != historicalAcceptanceFactsVersion ||
		change.Baseline.SourceMapVersion != "" || change.Baseline.ReviewRequirementsVersion != "" {
		return fmt.Errorf("validate historical Generation Request: invalid incremental baseline metadata")
	}
	if len(change.IntentChanges) == 0 && len(change.FactChanges) == 0 {
		return fmt.Errorf("validate historical Generation Request: incremental request has no changes")
	}
	if change.UnchangedIntentNodes < 0 || change.UnchangedFacts < 0 {
		return fmt.Errorf("validate historical Generation Request: unchanged counts must not be negative")
	}
	intentNodes, err := resolvedIntentNodes(outputs.intent)
	if err != nil {
		return err
	}
	if err := validateSemanticChanges("intent", change.IntentChanges, intentNodes); err != nil {
		return err
	}
	canonicalIntentNodes := make([]compiler.SemanticID, len(change.IntentChanges))
	for index, item := range change.IntentChanges {
		canonicalIntentNodes[index] = item.NodeID
	}
	if !reflect.DeepEqual(canonicalIntentNodes, change.IntentNodes) {
		return fmt.Errorf("validate historical Generation Request: intentNodes differ from intentChanges")
	}
	factNodes, err := acceptanceFactNodes(outputs.facts)
	if err != nil {
		return err
	}
	return validateSemanticChanges("fact", change.FactChanges, factNodes)
}

func upgradeHistoricalCompilerOutputs(request Request) (compilerOutputSet, error) {
	var result compilerOutputSet
	if request.ResolvedIntent == nil || request.AcceptanceFacts == nil || request.SourceMap == nil {
		return result, fmt.Errorf("upgrade historical Generation Request: compiler output is incomplete")
	}
	if request.ResolvedIntent.Version != historicalResolvedIntentVersion ||
		request.AcceptanceFacts.Version != historicalAcceptanceFactsVersion ||
		request.AcceptanceFacts.IntentVersion != historicalResolvedIntentVersion ||
		request.SourceMap.Version != historicalSourceMapVersion || request.SourceMap.IntentVersion != historicalResolvedIntentVersion {
		return result, fmt.Errorf("upgrade historical Generation Request: unsupported compiler output versions")
	}
	if len(request.ResolvedIntent.Identities) != 0 {
		return result, fmt.Errorf("upgrade historical Generation Request: v0.4 must not contain Identity nodes")
	}
	clone := func(source, target any) error {
		content, err := json.Marshal(source)
		if err != nil {
			return err
		}
		return json.Unmarshal(content, target)
	}
	var intent compiler.ResolvedIntent
	var facts compiler.AcceptanceFacts
	var sourceMap compiler.SourceMap
	if err := clone(request.ResolvedIntent, &intent); err != nil {
		return result, err
	}
	if err := clone(request.AcceptanceFacts, &facts); err != nil {
		return result, err
	}
	if err := clone(request.SourceMap, &sourceMap); err != nil {
		return result, err
	}
	droppedSuccess := map[compiler.SemanticID]compiler.SemanticID{}
	for pageIndex := range intent.Pages {
		page := &intent.Pages[pageIndex]
		if page.Access != nil || len(page.IdentityInteractions) != 0 {
			return result, fmt.Errorf("upgrade historical Generation Request: v0.4 page %s contains Identity-era fields", page.ID)
		}
		for viewIndex := range page.Views {
			view := &page.Views[viewIndex]
			for actionIndex := range view.Actions {
				action := &view.Actions[actionIndex]
				if action.Kind == "transition" {
					action.Action = compiler.SemanticID("action/" + view.Entity + "/" + action.Name)
					action.InteractionStates = []string{"invalid", "failure"}
				} else if action.Kind == "standard" && action.Name == "delete" {
					action.InteractionStates = []string{"failure"}
				}
				if err := upgradeHistoricalAccess(&action.Access); err != nil {
					return result, fmt.Errorf("upgrade historical Generation Request: %w", err)
				}
				// v0.4 recorded post-write navigation on the create/edit
				// reference as well as on the target form's submit intent. The
				// current shape keeps only the submit intent, so drop the
				// duplicate instead of leaving a field the invariant rejects.
				if action.Kind == "standard" && (action.Name == "create" || action.Name == "edit") && action.SuccessPage != "" {
					// sourceNodes are deduplicated, so a success page that is also
					// the target page appears once and must survive. Only record a
					// removal when the success page is a distinct source.
					if action.SuccessPage != action.TargetPage {
						droppedSuccess[action.ID] = compiler.SemanticID("page/" + action.SuccessPage)
					} else {
						droppedSuccess[action.ID] = ""
					}
					action.SuccessPage = ""
				}
			}
			if view.Submit != nil {
				if err := upgradeHistoricalAccess(&view.Submit.Access); err != nil {
					return result, fmt.Errorf("upgrade historical Generation Request: %w", err)
				}
			}
		}
	}
	for factIndex := range facts.Facts {
		fact := &facts.Facts[factIndex]
		successPage, dropped := droppedSuccess[fact.Subject]
		if !dropped || fact.Kind != "navigation" || fact.Expected.Navigation == nil {
			continue
		}
		fact.Expected.Navigation.SuccessKind = ""
		fact.Expected.Navigation.SuccessPage = ""
		if successPage == "" {
			continue
		}
		sources := fact.SourceNodes[:0]
		for _, source := range fact.SourceNodes {
			if source != successPage {
				sources = append(sources, source)
			}
		}
		fact.SourceNodes = sources
	}
	intent.Version = compiler.ResolvedIntentVersion
	facts.Version = compiler.AcceptanceFactsVersion
	facts.IntentVersion = compiler.ResolvedIntentVersion
	sourceMap.Version = compiler.SourceMapVersion
	sourceMap.IntentVersion = compiler.ResolvedIntentVersion
	reviews, err := compiler.BuildReviewRequirements(&intent)
	if err != nil {
		return result, err
	}
	result = compilerOutputSet{intent: &intent, facts: &facts, reviews: reviews, sources: &sourceMap}
	return result, nil
}

func upgradeHistoricalAccess(access *compiler.IRAccess) error {
	for index := range access.AllOf {
		requirement := &access.AllOf[index]
		if requirement.Kind != "" || len(requirement.AnyOf) == 0 || requirement.Identity != "" || requirement.Ownership != "" || requirement.ResourceBinding != "" {
			return fmt.Errorf("v0.4 access %s is not a roles-only requirement", access.ID)
		}
		requirement.Kind = "roles"
	}
	return nil
}

func compilerOutputsForDiff(request Request) (compilerOutputSet, error) {
	switch request.Schema {
	case LegacyRequestSchema, HistoricalIncrementalRequestSchema:
		return upgradeHistoricalCompilerOutputs(request)
	case RequestSchema:
		if request.ResolvedIntent == nil || request.AcceptanceFacts == nil || request.ReviewRequirements == nil || request.SourceMap == nil {
			return compilerOutputSet{}, fmt.Errorf("index Generation Request: compiler output is incomplete")
		}
		return compilerOutputSet{
			intent: request.ResolvedIntent, facts: request.AcceptanceFacts, reviews: request.ReviewRequirements, sources: request.SourceMap,
		}, nil
	default:
		return compilerOutputSet{}, fmt.Errorf("index Generation Request: unsupported schema %q", request.Schema)
	}
}

func reviewRequirementsVersion(request Request) string {
	if request.ReviewRequirements == nil {
		return noReviewRequirementsVersion
	}
	return request.ReviewRequirements.Version
}

func compilerVersionsForRequestSchema(schema string) (intent, facts, sourceMap, reviews string, ok bool) {
	switch schema {
	case LegacyRequestSchema, HistoricalIncrementalRequestSchema:
		return historicalResolvedIntentVersion, historicalAcceptanceFactsVersion, historicalSourceMapVersion, noReviewRequirementsVersion, true
	case RequestSchema:
		return compiler.ResolvedIntentVersion, compiler.AcceptanceFactsVersion, compiler.SourceMapVersion, compiler.ReviewRequirementsVersion, true
	default:
		return "", "", "", "", false
	}
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
