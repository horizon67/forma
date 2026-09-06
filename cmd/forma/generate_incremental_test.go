package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/agentrunner"
)

const updateSource = `entity User {
    name String required label
}
page Users {
    list User {
        columns name
    }
}
`

const updateManifest = `schema: forma/implementation-policy/v0alpha1
policies:
  - id: implementation/package-manager
    policy: required
    value: bun
    instruction: Use Bun for frontend builds
`

func TestNoOpRejectsMissingBaselineBeforePreflight(t *testing.T) {
	original := findExecutable
	t.Cleanup(func() { findExecutable = original })
	findExecutable = func(name string) (string, error) {
		t.Fatalf("invalid no-op looked up %s", name)
		return "", nil
	}
	var stdout, stderr bytes.Buffer
	if code := printNoOpGeneration(nil, nil, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "no-op plan has no baseline identity") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func saveBaseline(t *testing.T, directory, source, manifest string) string {
	t.Helper()
	args := []string{"request"}
	if manifest != "" {
		args = append(args, "--manifest", manifest)
	}
	args = append(args, source)
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("baseline exit %d: %s", code, &stderr)
	}
	path := filepath.Join(directory, "previous-request.json")
	writeTestFile(t, path, stdout.String())
	return path
}

func testGitCommand(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func newNoOpRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	testGitCommand(t, repo, "init", "-q")
	writeTestFile(t, filepath.Join(repo, "README.md"), "hand-written implementation\n")
	testGitCommand(t, repo, "add", "README.md")
	testGitCommand(t, repo, "-c", "user.name=Forma Test", "-c", "user.email=forma@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "Initial target")
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func TestGenerateNoOpSkipsCodexAndLeavesRepositoryUntouched(t *testing.T) {
	inputs, repo := t.TempDir(), newNoOpRepository(t)
	source := filepath.Join(inputs, "app.forma")
	manifest := filepath.Join(inputs, "policy.yaml")
	writeTestFile(t, source, updateSource)
	writeTestFile(t, manifest, updateManifest)
	baseline := saveBaseline(t, inputs, source, manifest)
	beforeHead := testGitCommand(t, repo, "rev-parse", "HEAD")
	oldInvoke, oldFind := invokeGeneration, findExecutable
	t.Cleanup(func() { invokeGeneration, findExecutable = oldInvoke, oldFind })
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		t.Fatal("no-op invoked Codex runner")
		return agentrunner.GenerateResult{}, nil
	}
	findExecutable = func(name string) (string, error) {
		if name != "git" {
			t.Fatalf("no-op looked up %s", name)
			return "", errors.New("Codex unavailable")
		}
		return oldFind(name)
	}
	for _, tc := range []struct {
		name, text   string
		withManifest bool
	}{
		{"identical", updateSource, true},
		{"comment and source map only", "// comment\n\n" + updateSource, true},
		{"inherited Manifest", updateSource, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeTestFile(t, source, tc.text)
			args := []string{"generate", "--repository", repo, "--previous", baseline}
			if tc.withManifest {
				args = append(args, "--manifest", manifest)
			}
			args = append(args, source)
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			for _, message := range []string{"no application or policy changes", "Codex was not started", "baseline request SHA-256:", "tests were not verified"} {
				if !strings.Contains(stdout.String(), message) {
					t.Fatalf("missing %q: %s", message, &stdout)
				}
			}
			if strings.Contains(stdout.String(), "starting Codex") || stderr.Len() != 0 {
				t.Fatalf("output: %s / %s", &stdout, &stderr)
			}
			if testGitCommand(t, repo, "status", "--porcelain=v1", "--untracked-files=all") != "" || testGitCommand(t, repo, "rev-parse", "HEAD") != beforeHead {
				t.Fatal("no-op changed repository")
			}
			content, err := os.ReadFile(filepath.Join(repo, "README.md"))
			if err != nil || string(content) != "hand-written implementation\n" {
				t.Fatalf("content changed: %q, %v", content, err)
			}
		})
	}
	// Existing dirty-worktree protection also applies to no-op.
	writeTestFile(t, filepath.Join(repo, "README.md"), "manual edit\n")
	var stdout, stderr bytes.Buffer
	args := []string{"generate", "--repository", repo, "--previous", baseline, source}
	if code := run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("dirty no-op exit %d: %s", code, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--allow-dirty"), &stdout, &stderr); code != 0 {
		t.Fatalf("allowed dirty no-op exit %d: %s", code, &stderr)
	}
	content, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil || string(content) != "manual edit\n" {
		t.Fatal("dirty no-op changed user work")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"generate", "--repository", inputs, "--previous", baseline, source}, &stdout, &stderr); code != 2 {
		t.Fatalf("non-Git no-op exit %d: %s", code, &stderr)
	}
}

func TestGenerateIncrementalSourceAndPolicyChangesPermitZeroDiff(t *testing.T) {
	oldInvoke := invokeGeneration
	t.Cleanup(func() { invokeGeneration = oldInvoke })
	for _, tc := range []struct {
		name             string
		source, manifest string
		policyOnly       bool
	}{
		{"source", strings.Replace(updateSource, "columns name", "columns name\n        paginate 10", 1), updateManifest, false},
		{"policy only", updateSource, strings.Replace(updateManifest, "value: bun", "value: pnpm", 1), true},
		{"instruction only", updateSource, strings.Replace(updateManifest, "frontend builds", "frontend builds with --frozen-lockfile", 1), true},
		{"conventions only", updateSource, updateManifest + "conventions:\n  - Preserve package boundaries\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputs := t.TempDir()
			repo := newNoOpRepository(t)
			source, manifest := filepath.Join(inputs, "app.forma"), filepath.Join(inputs, "policy.yaml")
			writeTestFile(t, source, updateSource)
			writeTestFile(t, manifest, updateManifest)
			baselinePath := saveBaseline(t, inputs, source, manifest)
			baselineBytes, err := os.ReadFile(baselinePath)
			if err != nil {
				t.Fatal(err)
			}
			baseline, err := agentrequest.UnmarshalRequest(baselineBytes)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, source, tc.source)
			writeTestFile(t, manifest, tc.manifest)
			calls := 0
			invokeGeneration = func(_ context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
				calls++
				request, err := agentrequest.UnmarshalRequest(invocation.Request)
				if err != nil {
					t.Fatal(err)
				}
				if err := agentrequest.ValidateIncrementalBaseline(request, baseline); err != nil {
					t.Fatal(err)
				}
				if tc.policyOnly && (len(request.RequestedChange.IntentChanges) != 0 || len(request.RequestedChange.FactChanges) != 0) {
					t.Fatal("policy-only request changed application scope")
				}
				if !tc.policyOnly && len(request.RequestedChange.IntentChanges) == 0 {
					t.Fatal("source delta missing")
				}
				generated := beginTestGeneration(t, invocation)
				generated.CodexMessage = []byte("Requirements already satisfied. Tests not run.")
				return generated, nil
			}
			var stdout, stderr bytes.Buffer
			args := []string{"generate", "--repository", repo, "--previous", baselinePath, "--manifest", manifest, source}
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			if calls != 1 || !strings.Contains(stdout.String(), "current Git status: clean") || !strings.Contains(stdout.String(), "completion is not verification success") {
				t.Fatalf("calls %d, output: %s", calls, &stdout)
			}
		})
	}
}

func TestGenerateInvalidBaselineAndUnsupportedRemovalDoNotInvokeRunner(t *testing.T) {
	inputs := t.TempDir()
	source, baseline := filepath.Join(inputs, "app.forma"), filepath.Join(inputs, "previous-request.json")
	writeTestFile(t, source, updateSource)
	oldInvoke, oldFind := invokeGeneration, findExecutable
	t.Cleanup(func() { invokeGeneration, findExecutable = oldInvoke, oldFind })
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		t.Fatal("invalid input invoked runner")
		return agentrunner.GenerateResult{}, nil
	}
	findExecutable = func(string) (string, error) {
		t.Fatal("invalid baseline reached executable lookup")
		return "", errors.New("unexpected")
	}
	var missingOut, missingErr bytes.Buffer
	if code := run([]string{"generate", "--repository", inputs, "--previous", baseline, source}, &missingOut, &missingErr); code != 2 {
		t.Fatalf("missing baseline exit %d: %s", code, &missingErr)
	}
	for _, content := range []string{"{", `{"schema":"unknown"}`, `{"schema":"forma/generation-request/v0alpha5"}`} {
		writeTestFile(t, baseline, content)
		var stdout, stderr bytes.Buffer
		if code := run([]string{"generate", "--repository", inputs, "--previous", baseline, source}, &stdout, &stderr); code != 1 {
			t.Fatalf("exit %d: %s", code, &stderr)
		}
	}
	for _, args := range [][]string{
		{"generate", "--repository", inputs, "--previous"},
		{"generate", "--repository", inputs, "--previous", "--manifest", "policy.yaml", source},
		{"generate", "--repository", inputs, "--previous", baseline, "--previous", baseline, source},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("parsing exit %d: %s", code, &stderr)
		}
	}
	baseline = saveBaseline(t, inputs, source, "")
	writeTestFile(t, source, "entity User {\n    name String required label\n}\n")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"generate", "--repository", inputs, "--previous", baseline, source}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "removed nodes") {
		t.Fatalf("removal exit %d: %s", code, &stderr)
	}
}
