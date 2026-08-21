package agentrequest

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
	assertGitBlobID(t, content, "154ae2e37b6823864db46ce2cc86d09f6c1576e0")
	if err := ValidateRequest(legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Schema != LegacyRequestSchema || legacy.Verification.FeedbackSchema != LegacyFeedbackSchema {
		t.Fatalf("legacy boundary = %#v", legacy)
	}
	canonical, err := Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, bytes.TrimSuffix(content, []byte("\n"))) {
		t.Fatalf("historical codec changed immutable v0alpha1 bytes in %s", goldenPath)
	}
	upgraded, err := compilerOutputsForDiff(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.ResolvedIntent, upgraded.intent) ||
		!reflect.DeepEqual(current.AcceptanceFacts, upgraded.facts) ||
		!reflect.DeepEqual(current.SourceMap, upgraded.sources) ||
		!reflect.DeepEqual(current.ReviewRequirements, upgraded.reviews) {
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
	assertGitBlobID(t, historicalContent, "5751ecf85e9b7be2665aa91854ee5b69798e81a3")
	if historical.Schema != HistoricalIncrementalRequestSchema {
		t.Fatalf("historical incremental schema = %s, want %s", historical.Schema, HistoricalIncrementalRequestSchema)
	}
	if err := ValidateRequest(historical); err != nil {
		t.Fatal(err)
	}
	canonicalHistorical, err := Marshal(historical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalHistorical, bytes.TrimSuffix(historicalContent, []byte("\n"))) {
		t.Fatal("historical codec changed immutable v0alpha2 bytes")
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
	upgradedHistorical, err := compilerOutputsForDiff(historical)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.ResolvedIntent, upgradedHistorical.intent) ||
		!reflect.DeepEqual(request.AcceptanceFacts, upgradedHistorical.facts) ||
		!reflect.DeepEqual(request.SourceMap, upgradedHistorical.sources) ||
		!reflect.DeepEqual(request.ReviewRequirements, upgradedHistorical.reviews) ||
		!reflect.DeepEqual(request.ImplementationPolicy, historical.ImplementationPolicy) ||
		!reflect.DeepEqual(request.RequestedChange.IntentChanges, historical.RequestedChange.IntentChanges) ||
		!reflect.DeepEqual(request.RequestedChange.FactChanges, historical.RequestedChange.FactChanges) ||
		request.RequestedChange.UnchangedIntentNodes != historical.RequestedChange.UnchangedIntentNodes ||
		request.RequestedChange.UnchangedFacts != historical.RequestedChange.UnchangedFacts {
		t.Fatal("current request changed historical admin semantics")
	}
	if request.RequestedChange.Baseline.SourceMapVersion != historicalSourceMapVersion ||
		request.RequestedChange.Baseline.ReviewRequirementsVersion != noReviewRequirementsVersion {
		t.Fatalf("B4 baseline versions = %#v", request.RequestedChange.Baseline)
	}
	if err := ValidateIncrementalBaseline(request, previous); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIncrementalBaseline(historical, previous); err != nil {
		t.Fatalf("historical pairwise lineage is invalid: %v", err)
	}
	if len(request.AcceptanceFacts.Facts) != 43 || len(request.RequestedChange.IntentChanges) != 8 ||
		len(request.RequestedChange.FactChanges) != 13 || request.RequestedChange.UnchangedFacts != 30 {
		t.Fatalf("incremental change summary = %#v", request.RequestedChange)
	}
}

func TestB4LineageFromAppliedHistoricalAdminToIdentity(t *testing.T) {
	historicalContent, err := os.ReadFile(filepath.Join("testdata", "admin.incremental.request.json"))
	if err != nil {
		t.Fatal(err)
	}
	assertGitBlobID(t, historicalContent, "5751ecf85e9b7be2665aa91854ee5b69798e81a3")
	historical, err := UnmarshalRequest(historicalContent)
	if err != nil {
		t.Fatal(err)
	}
	currentResult := historicalAdminWithIdentityResult(t, historical)
	request, err := BuildIncremental(historical, currentResult, nil)
	if err != nil {
		t.Fatal(err)
	}
	if request.Schema != RequestSchema || request.RequestedChange.Baseline.RequestSchema != HistoricalIncrementalRequestSchema {
		t.Fatalf("B4 lineage boundary = %#v", request.RequestedChange.Baseline)
	}
	if request.RequestedChange.Baseline.SourceMapVersion != historicalSourceMapVersion ||
		request.RequestedChange.Baseline.ReviewRequirementsVersion != noReviewRequirementsVersion {
		t.Fatalf("B4 historical compiler versions = %#v", request.RequestedChange.Baseline)
	}
	if len(request.RequestedChange.ReviewRequirementChanges) != 3 || request.RequestedChange.UnchangedReviewRequirements != 0 {
		t.Fatalf("B4 review requirement diff = %#v", request.RequestedChange)
	}
	if len(request.AcceptanceFacts.Facts) != 85 || len(request.RequestedChange.FactChanges) != 42 ||
		request.RequestedChange.UnchangedFacts != 43 {
		t.Fatalf("B4 fact lineage = %d total, %#v", len(request.AcceptanceFacts.Facts), request.RequestedChange)
	}
	if err := ValidateIncrementalBaseline(request, historical); err != nil {
		t.Fatal(err)
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
	if len(request.AcceptanceFacts.Facts) != 41 || len(request.Verification.RequiredFactIDs) != 41 {
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
			name: "unsupported intermediate schema",
			mutate: func(request *Request) {
				// v0alpha3 carried Review Requirements but its Acceptance Facts
				// can no longer be reproduced, so it is refused outright rather
				// than silently read with current-version expectations.
				request.Schema = "forma/generation-request/v0alpha3"
			},
			want: "unsupported schema",
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

func TestB4RecordsReviewRequirementDiff(t *testing.T) {
	baseline := membershipPreIdentityBaseline(t)
	request, err := BuildIncremental(baseline, membershipRequestResult(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.RequestedChange.ReviewRequirementChanges) != 3 || request.RequestedChange.UnchangedReviewRequirements != 0 {
		t.Fatalf("review requirement diff = %#v", request.RequestedChange)
	}
	for _, change := range request.RequestedChange.ReviewRequirementChanges {
		if change.Kind != "added" || !strings.HasPrefix(string(change.NodeID), "review/identity/") {
			t.Fatalf("review requirement change = %#v", change)
		}
	}
	if err := ValidateIncrementalBaseline(request, baseline); err != nil {
		t.Fatal(err)
	}
}

func TestValidateIncrementalBaselineIndependentlyChecksReviewDiff(t *testing.T) {
	baseline := membershipPreIdentityBaseline(t)
	request, err := BuildIncremental(baseline, membershipRequestResult(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.RequestedChange.ReviewRequirementChanges = request.RequestedChange.ReviewRequirementChanges[1:]
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("locally valid incomplete review diff should require its baseline to reject: %v", err)
	}
	if err := ValidateIncrementalBaseline(request, baseline); err == nil || !strings.Contains(err.Error(), "differ from baseline-derived") {
		t.Fatalf("incremental baseline review diff error = %v", err)
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

func TestValidateCoverageAcceptsFailedStatusWithoutClaimingSuccess(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	feedback := passingFeedback(request)
	feedback.Status = "failed"
	feedback.FactCoverage[0].Result = "failed"
	feedback.Command = "go test ./..."
	feedback.Diagnostics = []string{"--- FAIL: TestAdminFlow", "tests/admin_test.go:10: duplicate identifier overwrote the credential"}
	if err := ValidateCoverage(request, feedback); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCompletion(request, nil, feedback, ""); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCoverageAcceptsBuildFailureAsNotRun(t *testing.T) {
	request, err := BuildFull(compileRequestSource(t))
	if err != nil {
		t.Fatal(err)
	}
	feedback := passingFeedback(request)
	feedback.Status = "failed"
	feedback.Stage = "build"
	feedback.Diagnostics = []string{"# example.com/app", "app [build failed]"}
	for index := range feedback.FactCoverage {
		feedback.FactCoverage[index].Result = "not-run"
	}
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
			name: "unknown stage",
			mutate: func(feedback *Feedback) {
				feedback.Stage = "unknown"
			},
			want: "unknown feedback stage",
		},
		{
			name: "unknown result",
			mutate: func(feedback *Feedback) {
				feedback.Status = "failed"
				feedback.FactCoverage[0].Result = "banana"
				feedback.Diagnostics = []string{"go test failed"}
			},
			want: "unknown result",
		},
		{
			name: "failed without diagnostics",
			mutate: func(feedback *Feedback) {
				feedback.Status = "failed"
				feedback.FactCoverage[0].Result = "failed"
				feedback.Diagnostics = nil
			},
			want: "no diagnostics",
		},
		{
			name: "failed with every fact passed",
			mutate: func(feedback *Feedback) {
				feedback.Status = "failed"
				feedback.Diagnostics = []string{"exit status 1"}
			},
			want: "every fact as passed",
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

func membershipPreIdentityBaseline(t *testing.T) Request {
	t.Helper()
	current := membershipRequestResult(t)
	content, err := json.Marshal(current.Intent)
	if err != nil {
		t.Fatal(err)
	}
	var intent compiler.ResolvedIntent
	if err := json.Unmarshal(content, &intent); err != nil {
		t.Fatal(err)
	}
	intent.Identities = nil
	intent.Pages = nil
	sourceMap := &compiler.SourceMap{Version: current.SourceMap.Version, IntentVersion: current.SourceMap.IntentVersion}
	for _, entry := range current.SourceMap.Entries {
		if !strings.HasPrefix(string(entry.NodeID), "identity/") && !strings.HasPrefix(string(entry.NodeID), "page/") {
			sourceMap.Entries = append(sourceMap.Entries, entry)
		}
	}
	baseline, err := BuildFull(compiler.Result{Intent: &intent, SourceMap: sourceMap})
	if err != nil {
		t.Fatal(err)
	}
	return baseline
}

func historicalAdminWithIdentityResult(t *testing.T, historical Request) compiler.Result {
	t.Helper()
	upgraded, err := compilerOutputsForDiff(historical)
	if err != nil {
		t.Fatal(err)
	}
	clone := func(source, target any) {
		t.Helper()
		content, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(content, target); err != nil {
			t.Fatal(err)
		}
	}
	var intent compiler.ResolvedIntent
	var sourceMap compiler.SourceMap
	clone(upgraded.intent, &intent)
	clone(upgraded.sources, &sourceMap)
	membership := membershipRequestResult(t)
	intent.Identities = append(intent.Identities, membership.Intent.Identities...)
	intent.Pages = append(intent.Pages, membership.Intent.Pages...)
	foundActivate := false
	for index := range intent.Actions {
		if intent.Actions[index].ID == "action/User/activate" {
			intent.Actions[index].Sources = []string{"Pending"}
			foundActivate = true
		}
	}
	if !foundActivate {
		intent.Actions = append(intent.Actions, membership.Intent.Actions...)
	}
	sort.Slice(intent.Pages, func(i, j int) bool { return intent.Pages[i].ID < intent.Pages[j].ID })

	existing := make(map[compiler.SemanticID]bool, len(sourceMap.Entries))
	for _, entry := range sourceMap.Entries {
		existing[entry.NodeID] = true
	}
	for _, entry := range membership.SourceMap.Entries {
		if !existing[entry.NodeID] {
			sourceMap.Entries = append(sourceMap.Entries, entry)
			existing[entry.NodeID] = true
		}
	}
	sort.Slice(sourceMap.Entries, func(i, j int) bool { return sourceMap.Entries[i].NodeID < sourceMap.Entries[j].NodeID })
	result := compiler.Result{Intent: &intent, SourceMap: &sourceMap}
	if err := compiler.ValidateSourceMapCoverage(result.Intent, result.SourceMap); err != nil {
		t.Fatalf("combined historical admin and Identity fixture is invalid: %v", err)
	}
	return result
}

func assertGitBlobID(t *testing.T, content []byte, want string) {
	t.Helper()
	hash := sha1.New()
	_, _ = fmt.Fprintf(hash, "blob %d%c", len(content), 0)
	_, _ = hash.Write(content)
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != want {
		t.Fatalf("git blob ID = %s, want immutable historical blob %s", got, want)
	}
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
