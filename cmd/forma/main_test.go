package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

func TestGenerateHelpDescribesAutomaticPlanSelection(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exit %d: %s", code, &stderr)
	}
	if !strings.Contains(stdout.String(), "[--previous <request.json>]") || !strings.Contains(stdout.String(), "select full, incremental, or no-op") {
		t.Fatalf("help omits generation plan selection: %s", &stdout)
	}
}

func TestPathOptionsRejectMissingValuesConsistently(t *testing.T) {
	for _, tc := range []struct {
		command string
		option  string
		parse   func([]string) (string, error)
	}{
		{"generate", "--repository", func(args []string) (string, error) {
			options, _, err := parseGenerateOptions(args)
			return options.repository, err
		}},
		{"generate", "--previous", func(args []string) (string, error) {
			options, _, err := parseGenerateOptions(append([]string{"--repository", "/target"}, args...))
			return options.previousPath, err
		}},
		{"generate", "--manifest", func(args []string) (string, error) {
			options, _, err := parseGenerateOptions(append([]string{"--repository", "/target"}, args...))
			return options.manifestPath, err
		}},
		{"request", "--previous", func(args []string) (string, error) {
			options, _, err := parseGenerationRequestOptions(args)
			return options.previousPath, err
		}},
		{"request", "--manifest", func(args []string) (string, error) {
			options, _, err := parseGenerationRequestOptions(args)
			return options.manifestPath, err
		}},
		{"verify", "--repository", func(args []string) (string, error) {
			options, _, err := parseVerifyOptions(args)
			return options.repositoryRoot, err
		}},
		{"verify", "--baseline", func(args []string) (string, error) {
			options, _, err := parseVerifyOptions(args)
			return options.baselinePath, err
		}},
	} {
		t.Run(tc.command+"/"+tc.option, func(t *testing.T) {
			for _, suffix := range [][]string{nil, {""}, {"--manifest", "app.forma"}, {"--"}} {
				args := append([]string{tc.option}, suffix...)
				_, err := tc.parse(args)
				want := tc.command + " option " + tc.option + " requires "
				if err == nil || !strings.HasPrefix(err.Error(), want) {
					t.Fatalf("%v: error %v, want %q", args, err, want)
				}
				var stdout, stderr bytes.Buffer
				if code := run(append([]string{tc.command}, args...), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), want) {
					t.Fatalf("%v: exit %d, stderr %s", args, code, stderr.String())
				}
			}
			for _, value := range []string{"relative/path", "./--literal-path", "/absolute/--literal-path", "path with spaces"} {
				got, err := tc.parse([]string{tc.option, value})
				if err != nil || got != value {
					t.Fatalf("path %q: got %q, error %v", value, got, err)
				}
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	previous := versionOverride
	versionOverride = "v0.1.0-alpha.2"
	t.Cleanup(func() { versionOverride = previous })

	for _, command := range []string{"version", "--version"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run([]string{command}, &stdout, &stderr); exitCode != 0 {
				t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
			}
			if got, want := stdout.String(), "forma v0.1.0-alpha.2\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestVersionCommandRejectsArguments(t *testing.T) {
	for _, command := range []string{"version", "--version"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run([]string{command, "unexpected"}, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
			}
			if got, want := stderr.String(), "forma: "+command+" does not accept arguments\n"; got != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q", stdout.String())
			}
		})
	}
}

func TestAuthoringContextCommandUsesTheInstalledBinaryVersion(t *testing.T) {
	previous := versionOverride
	versionOverride = "v0.1.0-alpha.2"
	t.Cleanup(func() { versionOverride = previous })

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"authoring-context"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	for _, want := range []string{
		"Context schema: `forma/authoring-context/v0alpha1`",
		"Forma binary: `v0.1.0-alpha.2`",
		"Language profile: `v0.1.0-alpha.2`",
		"## Authoring protocol",
		"## Bundled complete example",
		"entry Welcome",
		"forma check app.forma",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout does not contain %q", want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAuthoringContextCommandRejectsArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"authoring-context", "app.forma"}, &stdout, &stderr); exitCode != 2 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got, want := stderr.String(), "forma: authoring-context does not accept arguments\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestVersionFromBuildInfoDistinguishesReleasesAndSourceBuilds(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		settings []debug.BuildSetting
		want     string
	}{
		{name: "tagged module", version: "v0.1.0-alpha.1", want: "v0.1.0-alpha.1"},
		{name: "second alpha", version: "v0.1.0-alpha.2", want: "v0.1.0-alpha.2"},
		{name: "development", version: "(devel)", want: "devel"},
		{
			name:    "proxy-installed pseudo version",
			version: "v0.0.0-20260821225903-7c507abd0003",
			want:    "devel 7c507ab",
		},
		{
			name:    "clean pseudo version",
			version: "v0.0.0-20260821225903-7c507abd0003",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "7c507abd0003deadbeef"},
			},
			want: "devel 7c507ab",
		},
		{
			name:    "pseudo version after a tag",
			version: "v0.1.1-0.20260821225903-7c507abd0003",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "7c507abd0003deadbeef"},
			},
			want: "devel 7c507ab",
		},
		{
			name:    "dirty tagged checkout",
			version: "v0.1.0-alpha.1",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "7c507abd0003deadbeef"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: "devel 7c507ab dirty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := versionFromBuildInfo(test.version, test.settings); got != test.want {
				t.Fatalf("versionFromBuildInfo() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestVersionCommandWithoutOverrideDoesNotReportAPseudoVersion(t *testing.T) {
	previous := versionOverride
	versionOverride = ""
	t.Cleanup(func() { versionOverride = previous })

	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"version"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if output := stdout.String(); !strings.HasPrefix(output, "forma devel") || strings.Contains(output, "v0.0.0-") {
		t.Fatalf("source build reported an ambiguous version: %q", output)
	}
}

func TestAlphaLanguageProfileArtifactVersionsMatchCode(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "alpha-language-profile.md"))
	if err != nil {
		t.Fatal(err)
	}
	const heading = "## Artifact baseline"
	start := strings.Index(string(content), heading)
	if start < 0 {
		t.Fatal("alpha profile has no Artifact baseline section")
	}
	section := string(content[start+len(heading):])
	var found bool
	section, _, found = strings.Cut(section, "## Supported core declarations")
	if !found {
		t.Fatal("alpha profile Artifact baseline section has no closing heading")
	}
	currentSection, historicalSection, found := strings.Cut(section, "### Historical input compatibility")
	if !found {
		t.Fatal("alpha profile has no Historical input compatibility table")
	}
	got := map[string]string{}
	for _, line := range strings.Split(currentSection, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Artifact ") || strings.HasPrefix(line, "| --- ") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) != 4 {
			t.Fatalf("invalid artifact table row %q", line)
		}
		got[strings.TrimSpace(cells[1])] = strings.Trim(strings.TrimSpace(cells[2]), "`")
	}
	want := map[string]string{
		"Resolved Intent":                compiler.ResolvedIntentVersion,
		"Source Map":                     compiler.SourceMapVersion,
		"Acceptance Facts":               compiler.AcceptanceFactsVersion,
		"Navigation Projection":          compiler.NavigationProjectionVersion,
		"Outcome Projection":             compiler.OutcomeProjectionVersion,
		"Domain State Projection":        compiler.DomainStateProjectionVersion,
		"Flow Projection":                compiler.FlowProjectionVersion,
		"Review Requirements":            compiler.ReviewRequirementsVersion,
		"Implementation Policy Manifest": implementationpolicy.Schema,
		"Generation Request":             agentrequest.RequestSchema,
		"Generation Feedback":            agentrequest.FeedbackSchema,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("alpha artifact table = %#v, want %#v", got, want)
	}

	historical := map[string]string{}
	for _, line := range strings.Split(historicalSection, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Input position ") || strings.HasPrefix(line, "| --- ") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) != 5 {
			t.Fatalf("invalid historical compatibility row %q", line)
		}
		historical[strings.TrimSpace(cells[1])] = strings.Trim(strings.TrimSpace(cells[2]), "`")
	}
	wantHistorical := map[string]string{
		"Generation Request (historical full)":        agentrequest.LegacyRequestSchema,
		"Generation Request (historical incremental)": agentrequest.HistoricalIncrementalRequestSchema,
		"Generation Request (previous alpha)":         agentrequest.PreviousRequestSchema,
		"Generation Feedback (legacy pair)":           agentrequest.LegacyFeedbackSchema,
		"Resolved Intent (historical request)":        agentrequest.HistoricalResolvedIntentVersion,
		"Acceptance Facts (historical request)":       agentrequest.HistoricalAcceptanceFactsVersion,
		"Source Map (historical request)":             agentrequest.HistoricalSourceMapVersion,
	}
	if !reflect.DeepEqual(historical, wantHistorical) {
		t.Fatalf("alpha historical artifact table = %#v, want %#v", historical, wantHistorical)
	}
}

func TestResolveCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"resolve", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var intent struct {
		Version string `json:"version"`
		Pages   []any  `json:"pages"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &intent); err != nil {
		t.Fatalf("resolve output is not JSON: %v\n%s", err, stdout.String())
	}
	if intent.Version != "forma/resolved-intent/v0.12" || len(intent.Pages) == 0 {
		t.Fatalf("resolved intent = %#v", intent)
	}
}

func TestProjectNavigationCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.navigation.txt")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "navigation", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected navigation projection:\n%s", stdout.String())
	}
}

func TestProjectOutcomesCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.outcomes.txt")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "outcomes", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected outcome projection:\n%s", stdout.String())
	}
}

func TestProjectStatesCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.states.txt")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "states", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected domain state projection:\n%s", stdout.String())
	}
}

func TestProjectFlowCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	wantPath := filepath.Join("..", "..", "internal", "compiler", "testdata", "users.flow.md")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"project", "flow", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.String() != string(want) {
		t.Fatalf("unexpected flow projection:\n%s", stdout.String())
	}
}

func TestProjectCommandRequiresAKnownProjection(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", args: []string{"project"}, want: "project requires a projection name"},
		{name: "unknown", args: []string{"project", "sequence"}, want: "unknown projection \"sequence\""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(test.args, &stdout, &stderr); exitCode != 2 {
				t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q", stdout.String())
			}
		})
	}
}

func TestRequestCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"request", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var request struct {
		Schema          string `json:"schema"`
		ResolvedIntent  any    `json:"resolvedIntent"`
		AcceptanceFacts struct {
			Facts []any `json:"facts"`
		} `json:"acceptanceFacts"`
		Verification struct {
			RequiredFactIDs []string `json:"requiredFactIds"`
		} `json:"verification"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &request); err != nil {
		t.Fatalf("request output is not JSON: %v\n%s", err, stdout.String())
	}
	if request.Schema != agentrequest.RequestSchema || request.ResolvedIntent == nil {
		t.Fatalf("generation request = %#v", request)
	}
	if len(request.AcceptanceFacts.Facts) == 0 || len(request.AcceptanceFacts.Facts) != len(request.Verification.RequiredFactIDs) {
		t.Fatalf("fact coverage policy does not cover request: %#v", request)
	}
}

func TestIncrementalRequestCommand(t *testing.T) {
	previousPath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.request.json")
	manifestPath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target", "forma.implementation.yaml")
	sourcePath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "app.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"request", "--previous", previousPath, "--manifest", manifestPath, sourcePath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var request struct {
		ImplementationPolicy struct {
			Policies []any `json:"policies"`
		} `json:"implementationPolicy"`
		RequestedChange struct {
			Kind          string `json:"kind"`
			IntentChanges []any  `json:"intentChanges"`
			FactChanges   []any  `json:"factChanges"`
		} `json:"requestedChange"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &request); err != nil {
		t.Fatalf("request output is not JSON: %v\n%s", err, stdout.String())
	}
	if request.RequestedChange.Kind != "incremental" || len(request.RequestedChange.IntentChanges) != 8 || len(request.RequestedChange.FactChanges) != 13 {
		t.Fatalf("requested change = %#v", request.RequestedChange)
	}
	if len(request.ImplementationPolicy.Policies) != 3 {
		t.Fatalf("implementation policy = %#v", request.ImplementationPolicy)
	}
}

func TestVerifyCommand(t *testing.T) {
	requestPath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.request.json")
	feedbackPath := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "baseline", "generation-feedback.json")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "verified 43 acceptance facts: all passed\n  12 distinct tests, max 8 facts per test\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestVerifyIncrementalCommandChecksRepositoryPolicies(t *testing.T) {
	requestPath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.incremental.request.json")
	baselinePath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.request.json")
	targetRoot := filepath.Join("..", "..", "experiments", "admin-agent-e2e", "target")
	feedbackPath := filepath.Join(targetRoot, "generation-feedback.json")
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"verify", "--repository", targetRoot, requestPath, feedbackPath}, &stdout, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "requires its baseline request") {
		t.Fatalf("missing baseline exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exitCode := run([]string{"verify", "--repository", targetRoot, "--baseline", baselinePath, requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	want := "verified 43 acceptance facts: all passed\n" +
		"  12 distinct tests, max 8 facts per test\n" +
		"verified 3 implementation policies\n" +
		"  2 satisfied, 1 deviated, 0 flagged\n" +
		"  deviated implementation/persistence: This controlled experiment retains the existing in-memory store.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestVerifyIdentityRequestAlwaysDisplaysHumanReview(t *testing.T) {
	read := func(name string, target any) {
		t.Helper()
		content, err := os.ReadFile(filepath.Join("..", "..", "internal", "compiler", "testdata", name))
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
	request, err := agentrequest.BuildFull(compiler.Result{Intent: &intent, SourceMap: &sourceMap})
	if err != nil {
		t.Fatal(err)
	}
	feedback := agentrequest.Feedback{Schema: agentrequest.FeedbackSchema, Stage: "test", Status: "succeeded"}
	for _, factID := range request.Verification.RequiredFactIDs {
		feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
			FactID: factID, TestReferences: []string{"tests/membership_test.go#" + strings.ReplaceAll(string(factID), "/", "_")}, Result: "passed",
		})
	}
	directory := t.TempDir()
	requestPath := filepath.Join(directory, "request.json")
	requestContent, err := agentrequest.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requestPath, requestContent, 0o644); err != nil {
		t.Fatal(err)
	}
	feedbackPath := filepath.Join(directory, "feedback.json")
	feedbackContent, err := json.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(feedbackPath, feedbackContent, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "verified 41 acceptance facts: all passed") ||
		!strings.Contains(output, "human review required: 3 requirements are not machine-verified") {
		t.Fatalf("review output = %q", output)
	}
	for _, requirement := range request.ReviewRequirements.Requirements {
		if !strings.Contains(output, string(requirement.ID)) || !strings.Contains(output, requirement.Instruction) {
			t.Fatalf("review output omits %s: %q", requirement.ID, output)
		}
	}
}

func TestVerifyCommandRejectsFailedFeedback(t *testing.T) {
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
	request, err := agentrequest.BuildFull(result)
	if err != nil {
		t.Fatal(err)
	}
	feedback := agentrequest.Feedback{
		Schema: agentrequest.FeedbackSchema, Stage: "test", Status: "failed",
		RelatedIntentNodes: []compiler.SemanticID{request.AcceptanceFacts.Facts[0].SourceNodes[0]},
		Command:            "go test ./...",
		Diagnostics: []string{
			"--- FAIL: TestAdminFlow",
			"tests/admin_test.go:10: the duplicate attempt's secret signed in: 303",
		},
		Summary: "Target tests failed; 1 mapped Acceptance Fact(s) did not pass.",
	}
	feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
		FactID:         request.Verification.RequiredFactIDs[0],
		TestReferences: []string{"tests/admin_test.go#TestAdminFlow"},
		Result:         "failed",
	})
	requestContent, err := agentrequest.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	requestPath := filepath.Join(directory, "request.json")
	if err := os.WriteFile(requestPath, requestContent, 0o644); err != nil {
		t.Fatal(err)
	}
	feedbackPath := filepath.Join(directory, "feedback.json")
	feedbackContent, err := json.Marshal(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(feedbackPath, feedbackContent, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code %d\nstdout:\n%s\nstderr:\n%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Generation Feedback status is failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed verify must not report a passing coverage summary: %q", stdout.String())
	}
}

func TestVerifyCommandRejectsMeasuredMembershipRepairFailure(t *testing.T) {
	requestPath := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "generation-request.json")
	baselinePath := filepath.Join("..", "..", "internal", "agentrequest", "testdata", "admin.incremental.request.json")
	targetRoot := filepath.Join("..", "..", "experiments", "membership-agent-e2e", "target")
	feedbackPath := filepath.Join("..", "..", "experiments", "membership-repair-loop", "generation-feedback.failed.json")
	feedbackContent, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := agentrequest.UnmarshalFeedback(feedbackContent)
	if err != nil {
		t.Fatal(err)
	}
	if feedback.Status != "failed" || feedback.Stage != "test" {
		t.Fatalf("measured failed feedback = %#v", feedback)
	}
	if !strings.Contains(feedback.Command, "go test -count=1 -json ./...") {
		t.Fatalf("measured command = %q", feedback.Command)
	}
	failed := 0
	for _, coverage := range feedback.FactCoverage {
		if coverage.Result == "failed" {
			failed++
		}
		if coverage.Result != "passed" && coverage.Result != "failed" && coverage.Result != "not-run" {
			t.Fatalf("fact %s has result %q", coverage.FactID, coverage.Result)
		}
	}
	if failed != 1 {
		t.Fatalf("measured failed facts = %d, want 1", failed)
	}
	for _, line := range feedback.Diagnostics {
		if strings.Contains(line, "(") && strings.Contains(line, "s)") {
			t.Fatalf("measured diagnostics contain a duration: %q", line)
		}
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"verify", "--repository", targetRoot, "--baseline", baselinePath, requestPath, feedbackPath}, &stdout, &stderr)
	if exitCode != 1 || !strings.Contains(stderr.String(), "Generation Feedback status is failed") {
		t.Fatalf("measured failed feedback exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed verify must not report a passing coverage summary: %q", stdout.String())
	}
}

func TestCheckCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "checked 1 file: no errors\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestCheckCommandRendersDiagnostic(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "invalid.forma")
	source := "entity User {\n    name String\n}\npage Users {\n    list User {\n        columns missing\n    }\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	for _, expected := range []string{"error[F2402]", "columns missing", "help:", "forma check failed with 1 error"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("stderr does not contain %q:\n%s", expected, stderr.String())
		}
	}
}

func TestCheckCommandAcceptsSelfOnlyInvariant(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "stock.forma")
	source := `type Quantity = Int min 0
entity StockItem {
    onHand Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "checked 1 file: no errors\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestCheckRequiresAnExplicitCompilationUnit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no source files or directories specified") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestDirectoryArgumentIsOneCompilationUnit(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.forma")
	nested := filepath.Join(directory, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(nested, "second.forma")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("role admin\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{first, second} {
		var stdout, stderr bytes.Buffer
		if exitCode := run([]string{"check", path}, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("independent unit %s failed with %d:\n%s", path, exitCode, stderr.String())
		}
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", directory}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("combined directory exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[F2001]: duplicate role `admin`") {
		t.Fatalf("directory was not compiled as one unit:\n%s", stderr.String())
	}
}
