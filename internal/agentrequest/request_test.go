package agentrequest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

func TestAdminAgentExperimentGoldenRequest(t *testing.T) {
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
	request, err := BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	assertTargetNeutralRequest(t, request)
	actual, err := Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	actual = append(actual, '\n')
	goldenPath := filepath.Join("testdata", "admin.request.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("Generation Request differs from %s\nactual:\n%s", goldenPath, actual)
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

func assertTargetNeutralRequest(t *testing.T, request Request) {
	t.Helper()
	encoded, err := Marshal(request)
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
	feedbackContent, err := os.ReadFile(filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target", "generation-feedback.json"))
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
