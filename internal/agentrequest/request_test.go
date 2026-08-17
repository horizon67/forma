package agentrequest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

func TestAdminAgentExperimentGoldenRequest(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "baseline", "app.forma")
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("experiments/admin-agent-e2e/app.forma", string(content)),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	current, err := BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	assertTargetNeutralRequest(t, current)
	goldenPath := filepath.Join("testdata", "admin.request.json")
	content, err = os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := UnmarshalRequest(content)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		legacy.ResolvedIntent = current.ResolvedIntent
		legacy.AcceptanceFacts = current.AcceptanceFacts
		legacy.SourceMap = current.SourceMap
		updated, err := Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, append(updated, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateRequest(legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Schema != LegacyRequestSchema || legacy.Verification.FeedbackSchema != LegacyFeedbackSchema {
		t.Fatalf("legacy boundary = %#v", legacy)
	}
	if !reflect.DeepEqual(current.ResolvedIntent, legacy.ResolvedIntent) ||
		!reflect.DeepEqual(current.AcceptanceFacts, legacy.AcceptanceFacts) ||
		!reflect.DeepEqual(current.SourceMap, legacy.SourceMap) {
		t.Fatalf("current compiler output differs from immutable v0alpha1 baseline %s", goldenPath)
	}
}

func TestAdminAgentIncrementalGoldenRequest(t *testing.T) {
	historicalContent, err := os.ReadFile(filepath.Join("testdata", "admin.incremental.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	historical, err := UnmarshalRequest(historicalContent)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Schema != PreviousRequestSchema {
		t.Fatalf("historical incremental schema = %s, want %s", historical.Schema, PreviousRequestSchema)
	}
	if err := ValidateRequest(historical); err != nil {
		t.Fatal(err)
	}

	previousContent, err := os.ReadFile(filepath.Join("testdata", "admin.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	previous, err := UnmarshalRequest(previousContent)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "app.forma")
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("experiments/admin-agent-e2e/app.forma", string(content)),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	manifestContent, err := os.ReadFile(filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target", "forma.implementation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := implementationpolicy.ParseYAML(manifestContent)
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildIncremental(previous, result, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	assertTargetNeutralRequest(t, request)
	if request.Schema != RequestSchema || request.ReviewRequirements == nil || len(request.ReviewRequirements.Requirements) != 0 {
		t.Fatalf("current admin review boundary = %#v", request.ReviewRequirements)
	}
	if !reflect.DeepEqual(request.ResolvedIntent, historical.ResolvedIntent) ||
		!reflect.DeepEqual(request.AcceptanceFacts, historical.AcceptanceFacts) ||
		!reflect.DeepEqual(request.SourceMap, historical.SourceMap) ||
		!reflect.DeepEqual(request.ImplementationPolicy, historical.ImplementationPolicy) ||
		!reflect.DeepEqual(request.RequestedChange, historical.RequestedChange) {
		t.Fatal("current request changed historical admin semantics")
	}
	if len(request.AcceptanceFacts.Facts) != 43 || len(request.RequestedChange.IntentChanges) != 8 ||
		len(request.RequestedChange.FactChanges) != 13 || request.RequestedChange.UnchangedFacts != 30 {
		t.Fatalf("incremental change summary = %#v", request.RequestedChange)
	}
}

func TestBuildFullRequestHasExactFactCoveragePolicy(t *testing.T) {
	result := compileRequestSource(t)
	request, err := BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	if request.Schema != RequestSchema || request.RequestedChange.Kind != "full" {
		t.Fatalf("request boundary = %#v", request)
	}
	if !request.Verification.RequireTestReference || !request.Verification.RejectUnknownFacts {
		t.Fatalf("verification policy = %#v", request.Verification)
	}
	if request.Verification.FeedbackSchema != FeedbackSchema {
		t.Fatalf("feedback schema = %q", request.Verification.FeedbackSchema)
	}
	if len(request.AcceptanceFacts.Facts) == 0 || len(request.AcceptanceFacts.Facts) != len(request.Verification.RequiredFactIDs) {
		t.Fatalf("fact count = %d, required IDs = %d", len(request.AcceptanceFacts.Facts), len(request.Verification.RequiredFactIDs))
	}
	for index, fact := range request.AcceptanceFacts.Facts {
		if request.Verification.RequiredFactIDs[index] != fact.ID {
			t.Fatalf("required fact %d = %s, want %s", index, request.Verification.RequiredFactIDs[index], fact.ID)
		}
	}
	assertTargetNeutralRequest(t, request)
}

func TestIdentityRequestKeepsHumanReviewOutsideFactCoverage(t *testing.T) {
	request, err := BuildFull(membershipRequestResult(t))
	if err != nil {
		t.Fatal(err)
	}
	if request.ReviewRequirements == nil || len(request.ReviewRequirements.Requirements) != 3 {
		t.Fatalf("review requirements = %#v", request.ReviewRequirements)
	}
	wantReviewIDs := compiler.ReviewRequirementIDs(request.ReviewRequirements)
	if !reflect.DeepEqual(request.Verification.DisplayReviewRequirementIDs, wantReviewIDs) {
		t.Fatalf("display review IDs = %v, want %v", request.Verification.DisplayReviewRequirementIDs, wantReviewIDs)
	}
	if len(request.AcceptanceFacts.Facts) != 38 || len(request.Verification.RequiredFactIDs) != 38 {
		t.Fatalf("fact boundary = %d facts, %d required", len(request.AcceptanceFacts.Facts), len(request.Verification.RequiredFactIDs))
	}
	for _, id := range request.Verification.RequiredFactIDs {
		if strings.HasPrefix(string(id), "review/") {
			t.Fatalf("review requirement leaked into fact coverage: %s", id)
		}
	}
	if err := ValidateCompletion(request, nil, passingFeedback(request), ""); err != nil {
		t.Fatalf("machine coverage without review self-report was rejected: %v", err)
	}
}

func TestValidateRequestRederivesReviewRequirements(t *testing.T) {
	base, err := BuildFull(membershipRequestResult(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{
			name: "missing artifact",
			mutate: func(request *Request) {
				request.ReviewRequirements = nil
			},
			want: "artifact is required",
		},
		{
			name: "rewritten instruction",
			mutate: func(request *Request) {
				request.ReviewRequirements.Requirements[0].Instruction = "agent marked this passed"
			},
			want: "differs from canonical",
		},
		{
			name: "hidden display requirement",
			mutate: func(request *Request) {
				request.Verification.DisplayReviewRequirementIDs = request.Verification.DisplayReviewRequirementIDs[1:]
			},
			want: "display review requirement IDs differ",
		},
		{
			name: "historical schema cannot hide review",
			mutate: func(request *Request) {
				request.Schema = PreviousRequestSchema
				request.ReviewRequirements = nil
				request.Verification.DisplayReviewRequirementIDs = nil
			},
			want: "historical schema cannot represent required human review",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			request, err := UnmarshalRequest(content)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&request)
			if err := ValidateRequest(request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFeedbackHasNoReviewCompletionChannel(t *testing.T) {
	content := []byte(`{"schema":"forma/generation-feedback/v0alpha2","stage":"test","status":"succeeded","reviewCoverage":[]}`)
	if _, err := UnmarshalFeedback(content); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("feedback review coverage error = %v", err)
	}
}

func TestB3RejectsIncrementalReviewChangesUntilDiffIsAvailable(t *testing.T) {
	previous, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildIncremental(previous, membershipRequestResult(t), nil); err == nil || !strings.Contains(err.Error(), "require the B4 request diff slice") {
		t.Fatalf("incremental review change error = %v", err)
	}
}

func TestValidateIncrementalBaselineIndependentlyRejectsReviewChanges(t *testing.T) {
	currentResult := membershipRequestResult(t)
	current, err := BuildFull(currentResult)
	if err != nil {
		t.Fatal(err)
	}

	intentContent, err := json.Marshal(currentResult.Intent)
	if err != nil {
		t.Fatal(err)
	}
	var baselineIntent compiler.ResolvedIntent
	if err := json.Unmarshal(intentContent, &baselineIntent); err != nil {
		t.Fatal(err)
	}
	baselineIntent.Identities = nil
	baselineIntent.Pages = nil
	baselineSourceMap := &compiler.SourceMap{Version: currentResult.SourceMap.Version, IntentVersion: currentResult.SourceMap.IntentVersion}
	for _, entry := range currentResult.SourceMap.Entries {
		if !strings.HasPrefix(string(entry.NodeID), "identity/") && !strings.HasPrefix(string(entry.NodeID), "page/") {
			baselineSourceMap.Entries = append(baselineSourceMap.Entries, entry)
		}
	}
	baseline, err := BuildFull(compiler.Result{Intent: &baselineIntent, SourceMap: baselineSourceMap})
	if err != nil {
		t.Fatal(err)
	}

	intentChanges, unchangedIntent, err := diffIntent(baseline.ResolvedIntent, current.ResolvedIntent)
	if err != nil {
		t.Fatal(err)
	}
	factChanges, unchangedFacts, err := diffFacts(baseline.AcceptanceFacts, current.AcceptanceFacts)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range append(append([]SemanticChange(nil), intentChanges...), factChanges...) {
		if change.Kind == "removed" {
			t.Fatalf("test baseline is not a semantic subset: %#v", change)
		}
	}
	baselineContent, err := Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	intentNodes := make([]compiler.SemanticID, len(intentChanges))
	for index, change := range intentChanges {
		intentNodes[index] = change.NodeID
	}
	current.RequestedChange = RequestedChange{
		Kind: "incremental",
		Baseline: &RequestBaseline{
			RequestSHA256: fmt.Sprintf("%x", sha256.Sum256(baselineContent)), RequestSchema: baseline.Schema,
			ResolvedIntentVersion: baseline.ResolvedIntent.Version, AcceptanceFactsVersion: baseline.AcceptanceFacts.Version,
		},
		IntentNodes: intentNodes, IntentChanges: intentChanges, FactChanges: factChanges,
		UnchangedIntentNodes: unchangedIntent, UnchangedFacts: unchangedFacts,
	}
	if err := ValidateRequest(current); err != nil {
		t.Fatalf("forged request-local metadata is invalid: %v", err)
	}
	if err := ValidateIncrementalBaseline(current, baseline); err == nil || !strings.Contains(err.Error(), "Review Requirement changes require the B4 request diff slice") {
		t.Fatalf("incremental baseline review error = %v", err)
	}
}

func TestValidateRequestRejectsMissingSourceMapEntry(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	missing := request.SourceMap.Entries[0].NodeID
	request.SourceMap.Entries = append([]compiler.SourceMapEntry(nil), request.SourceMap.Entries[1:]...)
	err = ValidateRequest(request)
	if err == nil || !strings.Contains(err.Error(), "node "+string(missing)+" has no source entry") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestBuildIncrementalRequestDerivesStableSemanticDiff(t *testing.T) {
	previous, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := implementationpolicy.ParseYAML([]byte(testImplementationManifest))
	if err != nil {
		t.Fatal(err)
	}
	current := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("request.forma", `role admin
entity User {
    name String required label
    nickname String
}
page Users {
    allow admin
    list User {
        columns name, nickname
        search name, nickname
        paginate 10
    }
}
`)})
	if len(current.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", current.Diagnostics)
	}
	request, err := BuildIncremental(previous, current, &manifest)
	if err != nil {
		t.Fatal(err)
	}
	if request.RequestedChange.Kind != "incremental" || request.RequestedChange.Baseline == nil {
		t.Fatalf("requested change = %#v", request.RequestedChange)
	}
	if len(request.RequestedChange.Baseline.RequestSHA256) != 64 {
		t.Fatalf("baseline = %#v", request.RequestedChange.Baseline)
	}
	wantIntent := map[compiler.SemanticID]string{
		"entity/User/field/nickname": "added",
		"entity/User":                "changed",
		"page/Users/view/list/User":  "changed",
	}
	for _, change := range request.RequestedChange.IntentChanges {
		if want, ok := wantIntent[change.NodeID]; ok {
			if want != "" && change.Kind != want {
				t.Errorf("intent change %s = %s, want %s", change.NodeID, change.Kind, want)
			}
			delete(wantIntent, change.NodeID)
		}
	}
	for id, kind := range wantIntent {
		t.Errorf("missing intent change %s (%s)", id, kind)
	}
	wantFacts := map[compiler.SemanticID]string{
		"fact/page/Users/view/list/User/fields":        "changed",
		"fact/page/Users/view/list/User/search":        "added",
		"fact/page/Users/view/list/User/page-boundary": "added",
	}
	for _, change := range request.RequestedChange.FactChanges {
		if want, ok := wantFacts[change.NodeID]; ok {
			if change.Kind != want {
				t.Errorf("fact change %s = %s, want %s", change.NodeID, change.Kind, want)
			}
			delete(wantFacts, change.NodeID)
		}
	}
	for id, kind := range wantFacts {
		t.Errorf("missing fact change %s (%s)", id, kind)
	}
	if request.ImplementationPolicy == nil || len(request.ImplementationPolicy.Policies) != 3 {
		t.Fatalf("implementation policy = %#v", request.ImplementationPolicy)
	}
	if err := ValidateRequest(request); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIncrementalRejectsNoChangeAndRemoval(t *testing.T) {
	previous, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildIncremental(previous, compileRequestSource(t), nil); err == nil || !strings.Contains(err.Error(), "identical") {
		t.Fatalf("no-change error = %v", err)
	}
	removed := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("request.forma", `role admin
entity User {
    name String required label
}
`)})
	if len(removed.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", removed.Diagnostics)
	}
	if _, err := BuildIncremental(previous, removed, nil); err == nil || !strings.Contains(err.Error(), "removed nodes are not supported") {
		t.Fatalf("removal error = %v", err)
	}
}

func TestBuildIncrementalPreservesExistingImplementationPolicy(t *testing.T) {
	manifest, err := implementationpolicy.ParseYAML([]byte(testImplementationManifest))
	if err != nil {
		t.Fatal(err)
	}
	previous, err := BuildFullWithPolicy(compileRequestSource(t), &manifest)
	if err != nil {
		t.Fatal(err)
	}
	current := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("request.forma", `role admin
entity User {
    name String required label
    nickname String
}
page Users {
    allow admin
    list User {
        columns name, nickname
    }
}
`)})
	if len(current.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", current.Diagnostics)
	}
	request, err := BuildIncremental(previous, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.ImplementationPolicy, previous.ImplementationPolicy) {
		t.Fatalf("implementation policy was not preserved: %#v", request.ImplementationPolicy)
	}
}

func TestValidateCompletionChecksImplementationPolicyAgainstRepository(t *testing.T) {
	manifest, err := implementationpolicy.ParseYAML([]byte(testImplementationManifest))
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildFullWithPolicy(compileRequestSource(t), &manifest)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	serverPath := filepath.Join(root, "internal", "web", "server.go")
	if err := os.MkdirAll(filepath.Dir(serverPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverPath, []byte("import \"html/template\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	feedback := passingFeedback(request)
	feedback.PolicyCoverage = []implementationpolicy.Coverage{
		{PolicyID: "implementation/server-rendering", Status: "satisfied", Evidence: []string{"internal/web/server.go"}},
		{PolicyID: "implementation/persistence", Status: "deviated", Reason: "the controlled target remains in memory"},
		{PolicyID: "implementation/router", Status: "satisfied"},
	}
	if err := ValidateCompletion(request, nil, feedback, root); err != nil {
		t.Fatal(err)
	}
	feedback.PolicyCoverage = feedback.PolicyCoverage[1:]
	if err := ValidateCompletion(request, nil, feedback, root); err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Fatalf("coverage error = %v", err)
	}
}

func assertTargetNeutralRequest(t *testing.T, request Request) {
	t.Helper()
	semanticPayload := struct {
		ResolvedIntent     *compiler.ResolvedIntent     `json:"resolvedIntent"`
		AcceptanceFacts    *compiler.AcceptanceFacts    `json:"acceptanceFacts"`
		ReviewRequirements *compiler.ReviewRequirements `json:"reviewRequirements"`
		SourceMap          *compiler.SourceMap          `json:"sourceMap"`
		RequestedChange    RequestedChange              `json:"requestedChange"`
		Verification       VerificationPolicy           `json:"verification"`
	}{
		ResolvedIntent: request.ResolvedIntent, AcceptanceFacts: request.AcceptanceFacts,
		ReviewRequirements: request.ReviewRequirements, SourceMap: request.SourceMap,
		RequestedChange: request.RequestedChange, Verification: request.Verification,
	}
	encoded, err := json.Marshal(semanticPayload)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"http"`, `"route"`, `"html"`, `"selector"`, `"framework"`, `"component"`,
		`"relationChoices"`, `"tieBreak"`, `"preventDuplicateDispatch"`,
		`"failureFeedback"`, `"recheckAccess"`, `"submissionToken"`,
	} {
		if bytes.Contains(bytes.ToLower(encoded), bytes.ToLower([]byte(forbidden))) {
			t.Errorf("Generation Request contains target vocabulary %s", forbidden)
		}
	}
	allowedStates := map[string]bool{"empty": true, "invalid": true, "failure": true}
	for _, page := range request.ResolvedIntent.Pages {
		for _, view := range page.Views {
			for _, state := range view.InteractionStates {
				if !allowedStates[state] {
					t.Errorf("view %s contains implementation-shaped interaction state %q", view.ID, state)
				}
			}
		}
	}
}

func TestValidateCoverageAcceptsExactPassingSet(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	feedback := passingFeedback(request)
	if err := ValidateCoverage(request, feedback); err != nil {
		t.Fatal(err)
	}
}

func TestRequestAndFeedbackJSONRoundTrip(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	requestJSON, err := Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decodedRequest, err := UnmarshalRequest(requestJSON)
	if err != nil {
		t.Fatal(err)
	}
	feedbackJSON, err := json.Marshal(passingFeedback(request))
	if err != nil {
		t.Fatal(err)
	}
	decodedFeedback, err := UnmarshalFeedback(feedbackJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCoverage(decodedRequest, decodedFeedback); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolJSONRejectsUnknownFields(t *testing.T) {
	if _, err := UnmarshalRequest([]byte(`{"schema":"forma/generation-request/v0alpha1","unknown":true}`)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("request error = %v", err)
	}
	if _, err := UnmarshalFeedback([]byte(`{"schema":"forma/generation-feedback/v0alpha1","unknown":true}`)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("feedback error = %v", err)
	}
}

func TestAdminAgentExperimentFeedbackCoversCanonicalRequest(t *testing.T) {
	requestContent, err := os.ReadFile(filepath.Join("testdata", "admin.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := UnmarshalRequest(requestContent)
	if err != nil {
		t.Fatal(err)
	}
	feedbackContent, err := os.ReadFile(filepath.Join("..", "..", "experiments", "admin-agent-e2e", "baseline", "generation-feedback.json"))
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := UnmarshalFeedback(feedbackContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCoverage(request, feedback); err != nil {
		t.Fatal(err)
	}
	summary := SummarizeCoverage(feedback)
	if summary.FactCount != 43 || summary.DistinctTests != 12 || summary.MaxFactsPerTest != 8 {
		t.Fatalf("coverage summary = %#v", summary)
	}
}

func TestAdminAgentIncrementalFeedbackCoversCurrentRequestAndPolicies(t *testing.T) {
	requestContent, err := os.ReadFile(filepath.Join("testdata", "admin.incremental.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := UnmarshalRequest(requestContent)
	if err != nil {
		t.Fatal(err)
	}
	baselineContent, err := os.ReadFile(filepath.Join("testdata", "admin.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := UnmarshalRequest(baselineContent)
	if err != nil {
		t.Fatal(err)
	}
	feedbackContent, err := os.ReadFile(filepath.Join("..", "..", "experiments", "admin-agent-e2e", "baseline", "generation-feedback.json"))
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := UnmarshalFeedback(feedbackContent)
	if err != nil {
		t.Fatal(err)
	}
	feedback.Schema = FeedbackSchema
	feedback.PolicyCoverage = []implementationpolicy.Coverage{
		{PolicyID: "implementation/server-rendering", Status: "satisfied", Evidence: []string{"internal/web/server.go"}},
		{PolicyID: "implementation/persistence", Status: "deviated", Reason: "This controlled experiment retains the existing in-memory store."},
		{PolicyID: "implementation/router", Status: "satisfied"},
	}
	feedback.Summary = "The existing target was updated incrementally for nickname and page-size intent while preserving all acceptance facts and implementation policies."
	actual, err := json.MarshalIndent(feedback, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	actual = append(actual, '\n')
	targetRoot := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target")
	goldenPath := filepath.Join(targetRoot, "generation-feedback.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read incremental feedback: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("incremental Generation Feedback differs from %s", goldenPath)
	}
	if err := ValidateCompletion(request, &baseline, feedback, targetRoot); err != nil {
		t.Fatal(err)
	}
	summary := SummarizeCoverage(feedback)
	if summary.FactCount != 43 || summary.DistinctTests != 12 || summary.MaxFactsPerTest != 8 {
		t.Fatalf("coverage summary = %#v", summary)
	}
}

func TestValidateIncrementalBaselineChecksDigestAndSemanticDiff(t *testing.T) {
	baselineContent, err := os.ReadFile(filepath.Join("testdata", "admin.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := UnmarshalRequest(baselineContent)
	if err != nil {
		t.Fatal(err)
	}
	requestContent, err := os.ReadFile(filepath.Join("testdata", "admin.incremental.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	request, err := UnmarshalRequest(requestContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateIncrementalBaseline(request, baseline); err != nil {
		t.Fatal(err)
	}

	request.RequestedChange.Baseline.RequestSHA256 = strings.Repeat("0", 64)
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("well-formed but incorrect digest should pass request-local validation: %v", err)
	}
	if err := ValidateIncrementalBaseline(request, baseline); err == nil || !strings.Contains(err.Error(), "does not match recorded") {
		t.Fatalf("digest error = %v", err)
	}

	request, err = UnmarshalRequest(requestContent)
	if err != nil {
		t.Fatal(err)
	}
	request.RequestedChange.IntentChanges = request.RequestedChange.IntentChanges[1:]
	request.RequestedChange.IntentNodes = request.RequestedChange.IntentNodes[1:]
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("locally valid incomplete diff should require its baseline to reject: %v", err)
	}
	if err := ValidateIncrementalBaseline(request, baseline); err == nil || !strings.Contains(err.Error(), "differ from baseline-derived") {
		t.Fatalf("semantic diff error = %v", err)
	}
}

func TestValidateCoverageAllowsOneTestToCoverMultipleFacts(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	feedback := passingFeedback(request)
	if len(feedback.FactCoverage) < 2 {
		t.Fatal("test requires multiple facts")
	}
	if feedback.FactCoverage[0].TestReferences[0] != feedback.FactCoverage[1].TestReferences[0] {
		t.Fatal("passingFeedback should exercise a shared integration test reference")
	}
	if err := ValidateCoverage(request, feedback); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCoverageRejectsSilentOmissionsAndUnknownFacts(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Feedback)
		want   string
	}{
		{
			name: "unknown status",
			mutate: func(feedback *Feedback) {
				feedback.Status = "complete"
			},
			want: "unknown feedback status",
		},
		{
			name: "missing",
			mutate: func(feedback *Feedback) {
				feedback.FactCoverage = feedback.FactCoverage[1:]
			},
			want: "required fact",
		},
		{
			name: "unknown",
			mutate: func(feedback *Feedback) {
				feedback.FactCoverage[0].FactID = "fact/unknown"
			},
			want: "unknown fact",
		},
		{
			name: "duplicate",
			mutate: func(feedback *Feedback) {
				feedback.FactCoverage[1].FactID = feedback.FactCoverage[0].FactID
			},
			want: "duplicate fact",
		},
		{
			name: "no test reference",
			mutate: func(feedback *Feedback) {
				feedback.FactCoverage[0].TestReferences = nil
			},
			want: "no test reference",
		},
		{
			name: "not passed",
			mutate: func(feedback *Feedback) {
				feedback.FactCoverage[0].Result = "failed"
			},
			want: "want passed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			feedback := passingFeedback(request)
			test.mutate(&feedback)
			if err := ValidateCoverage(request, feedback); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("coverage error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateCoverageRejectsTamperedRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{
			name: "required facts shortened",
			mutate: func(request *Request) {
				request.Verification.RequiredFactIDs = request.Verification.RequiredFactIDs[:1]
			},
			want: "required fact IDs differ",
		},
		{
			name: "unknown facts allowed",
			mutate: func(request *Request) {
				request.Verification.RejectUnknownFacts = false
			},
			want: "verification policy was weakened",
		},
		{
			name: "intent version changed",
			mutate: func(request *Request) {
				request.ResolvedIntent.Version = "forma/resolved-intent/v0.3"
			},
			want: "verify with the matching Forma version",
		},
		{
			name: "facts version changed",
			mutate: func(request *Request) {
				request.AcceptanceFacts.Version = "forma/acceptance-facts/v0alpha0"
			},
			want: "verify with the matching Forma version",
		},
		{
			name: "source map version changed",
			mutate: func(request *Request) {
				request.SourceMap.Version = "forma/source-map/v0.1"
			},
			want: "verify with the matching Forma version",
		},
		{
			name: "canonical fact removed",
			mutate: func(request *Request) {
				request.AcceptanceFacts.Facts = request.AcceptanceFacts.Facts[1:]
				request.Verification.RequiredFactIDs = request.Verification.RequiredFactIDs[1:]
			},
			want: "differ from canonical facts",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := BuildFull(compileRequestSource(t))
			if err != nil {
				t.Fatal(err)
			}
			feedback := passingFeedback(request)
			test.mutate(&request)
			if err := ValidateCoverage(request, feedback); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("coverage error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateCoverageRejectsInvalidTestReferences(t *testing.T) {
	tests := []struct {
		name       string
		references []string
		want       string
	}{
		{name: "empty", references: []string{""}, want: "invalid test reference"},
		{name: "missing identifier", references: []string{"tests/admin_test.go"}, want: "repository/path#test-identifier"},
		{name: "absolute path", references: []string{"/tmp/admin_test.go#TestAdmin"}, want: "not repository-relative"},
		{name: "non canonical path", references: []string{"tests/../tests/admin_test.go#TestAdmin"}, want: "not repository-relative"},
		{name: "duplicate", references: []string{"tests/admin_test.go#TestAdmin", "tests/admin_test.go#TestAdmin"}, want: "repeats test reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := BuildFull(compileRequestSource(t))
			if err != nil {
				t.Fatal(err)
			}
			feedback := passingFeedback(request)
			feedback.FactCoverage[0].TestReferences = test.references
			if err := ValidateCoverage(request, feedback); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("coverage error = %v, want %q", err, test.want)
			}
		})
	}
}

func compileRequestSource(t *testing.T) compiler.Result {
	t.Helper()
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("request.forma", `role admin
entity User {
    name String required label
}
page Users {
    allow admin
    list User {
        columns name
    }
}
`)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	return result
}

func passingFeedback(request Request) Feedback {
	feedback := Feedback{Schema: FeedbackSchema, Stage: "test", Status: "succeeded"}
	for _, id := range request.Verification.RequiredFactIDs {
		feedback.FactCoverage = append(feedback.FactCoverage, FactCoverage{
			FactID: id, TestReferences: []string{"tests/admin_test.go#TestAdminFlow"}, Result: "passed",
		})
	}
	return feedback
}

func membershipRequestResult(t *testing.T) compiler.Result {
	t.Helper()
	read := func(name string, target any) {
		t.Helper()
		content, err := os.ReadFile(filepath.Join("..", "compiler", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(content, target); err != nil {
			t.Fatal(err)
		}
	}
	var intent compiler.ResolvedIntent
	var sourceMap compiler.SourceMap
	read("membership.intent.json", &intent)
	read("membership.sourcemap.json", &sourceMap)
	return compiler.Result{Intent: &intent, SourceMap: &sourceMap}
}

const testImplementationManifest = `schema: forma/implementation-policy/v0alpha1
policies:
  - id: implementation/server-rendering
    policy: required
    value: html/template
  - id: implementation/persistence
    policy: preferred
    value: database/sql
  - id: implementation/router
    policy: forbidden
    value: github.com/gorilla/mux
`
