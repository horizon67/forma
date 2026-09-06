package agentrunner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratorPassesCanonicalRequestOnlyThroughCodexStdinAndStopsForReview(t *testing.T) {
	target := t.TempDir()
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	locker := &recordingLocker{}
	git := &scriptedCommandRunner{t: t, steps: []commandStep{
		{stdout: canonicalTarget + "\n"},
		{requireLocked: locker, stdout: "0123456789abcdef\n"},
		{requireLocked: locker, stdout: "H README.md\x00"},
		{requireLocked: locker},
		{requireLocked: locker, stdout: " M README.md\n?? internal/app.go\n"},
	}}
	codex := &scriptedCommandRunner{t: t, steps: []commandStep{
		{requireLocked: locker, stdout: "Logged in using ChatGPT\n"},
		{requireLocked: locker, stdout: "Implemented the application.\n"},
	}}
	request := []byte(`{"schema":"forma/generation-request/v0alpha5","resolvedIntent":{"version":"test"},"requestedChange":{"kind":"full"}}`)
	environment := []string{"HOME=/trusted/home", "PATH=/trusted/bin"}

	result, err := (Generator{
		Repository: RepositoryPreflight{Commands: git, Locks: locker},
		Codex:      codex,
	}).Run(context.Background(), GenerateOptions{
		Repository:      target,
		GitExecutable:   "/absolute/git",
		CodexExecutable: "/absolute/codex",
		Environment:     environment,
		Request:         request,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != canonicalTarget || result.Worktree != canonicalTarget || result.InitialHead != "0123456789abcdef" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.InitialDirty || !result.FinalStatusKnown || result.FinalStatus != " M README.md\n?? internal/app.go\n" {
		t.Fatalf("result status = %#v", result)
	}
	if string(result.CodexMessage) != "Implemented the application.\n" || len(result.CodexDiagnostics) != 0 {
		t.Fatalf("Codex output = %q / %q", result.CodexMessage, result.CodexDiagnostics)
	}
	if locker.locked {
		t.Fatal("generation retained the worktree lock")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runner materialized files in target: %#v", entries)
	}

	if len(codex.calls) != 2 {
		t.Fatalf("Codex calls = %d", len(codex.calls))
	}
	auth := codex.calls[0]
	if auth.Executable != "/absolute/codex" || !reflect.DeepEqual(auth.Arguments, []string{"login", "status"}) || len(auth.Stdin) != 0 {
		t.Fatalf("auth call = %#v", auth)
	}
	wantArguments := []string{
		"exec", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--sandbox", "workspace-write", "--color", "never", "-C", canonicalTarget, "-",
	}
	implementation := codex.calls[1]
	if implementation.Executable != "/absolute/codex" || !reflect.DeepEqual(implementation.Arguments, wantArguments) {
		t.Fatalf("implementation call = %#v", implementation)
	}
	if implementation.Directory != canonicalTarget || !reflect.DeepEqual(implementation.Environment, environment) {
		t.Fatalf("implementation boundary = %#v", implementation)
	}
	prompt := string(implementation.Stdin)
	wantPromptDigest := fmt.Sprintf("%x", sha256.Sum256(implementation.Stdin))
	if result.ImplementationPromptSHA256 != wantPromptDigest {
		t.Fatalf("prompt SHA-256 = %q, want %q", result.ImplementationPromptSHA256, wantPromptDigest)
	}
	if !strings.Contains(prompt, "BEGIN FORMA GENERATION REQUEST JSON\n"+string(request)+"\nEND FORMA GENERATION REQUEST JSON") {
		t.Fatalf("implementation prompt does not preserve request bytes:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not create Generation Feedback") || !strings.Contains(prompt, "Do not modify .forma source files") {
		t.Fatalf("implementation prompt omits the thin-runner boundary:\n%s", prompt)
	}
	for _, required := range []string{
		"Implementation scope:",
		"Implement and test each Acceptance Fact at its named subject boundary.",
		"expected.enforcement=authoritative",
		"the application's public boundary that presents or invokes the subject must enforce it",
		"An anonymous principal means no authenticated identity and no roles",
		"Never turn missing identity, session, or role state into an allowed default role",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("implementation prompt omits dogfood-derived boundary %q:\n%s", required, prompt)
		}
	}
	if len(git.calls) != 5 || !reflect.DeepEqual(git.calls[4].Arguments, gitArguments(canonicalTarget, "status", "--porcelain=v1", "--untracked-files=normal")) {
		t.Fatalf("final Git status call = %#v", git.calls)
	}
}

func TestGeneratorDistinguishesMissingAuthenticationBeforeCodexExec(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	git := &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, false)}
	codex := &scriptedCommandRunner{t: t, steps: []commandStep{{
		requireLocked: locker,
		exitCode:      1,
		stderr:        "Not logged in\n",
	}}}

	_, err := (Generator{
		Repository: RepositoryPreflight{Commands: git, Locks: locker},
		Codex:      codex,
	}).Run(context.Background(), GenerateOptions{
		Repository: target, GitExecutable: "/absolute/git", CodexExecutable: "/absolute/codex", Request: []byte(`{"requestedChange":{"kind":"full"}}`),
	})
	if !errors.Is(err, ErrCodexAuthentication) || !strings.Contains(err.Error(), "codex login") {
		t.Fatalf("error = %v", err)
	}
	if len(codex.calls) != 1 || locker.locked {
		t.Fatalf("Codex calls / lock = %d / %v", len(codex.calls), locker.locked)
	}
}

func TestGeneratorCapturesPartialChangesWhenCodexFails(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	steps := cleanRepositorySteps(target, locker, true)
	steps[4].stdout = "?? partial.txt\n"
	git := &scriptedCommandRunner{t: t, steps: steps}
	codex := &scriptedCommandRunner{t: t, steps: []commandStep{
		{requireLocked: locker, stdout: "Logged in\n"},
		{requireLocked: locker, exitCode: 7, stderr: "implementation failed\n"},
	}}

	result, err := (Generator{
		Repository: RepositoryPreflight{Commands: git, Locks: locker},
		Codex:      codex,
	}).Run(context.Background(), GenerateOptions{
		Repository: target, GitExecutable: "/absolute/git", CodexExecutable: "/absolute/codex", Request: []byte(`{"requestedChange":{"kind":"full"}}`),
	})
	if !errors.Is(err, ErrCodexFailed) || !strings.Contains(err.Error(), "exit 7") {
		t.Fatalf("error = %v", err)
	}
	if !result.FinalStatusKnown || result.FinalStatus != "?? partial.txt\n" || string(result.CodexDiagnostics) != "implementation failed\n" {
		t.Fatalf("result = %#v", result)
	}
	if locker.locked {
		t.Fatal("failed Codex run retained the worktree lock")
	}
}

func TestGeneratorRequiresARequestBeforeRepositoryOrCodexWork(t *testing.T) {
	git := &scriptedCommandRunner{t: t}
	codex := &scriptedCommandRunner{t: t}
	_, err := (Generator{
		Repository: RepositoryPreflight{Commands: git, Locks: &recordingLocker{}},
		Codex:      codex,
	}).Run(context.Background(), GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires canonical Generation Request") {
		t.Fatalf("error = %v", err)
	}
	if len(git.calls) != 0 || len(codex.calls) != 0 {
		t.Fatalf("empty request reached commands: Git %d, Codex %d", len(git.calls), len(codex.calls))
	}
}

func TestIncrementalRunnerUsesBoundedPromptAndAllowsZeroDiff(t *testing.T) {
	target := t.TempDir()
	locker := &recordingLocker{}
	git := &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, true)}
	codex := &scriptedCommandRunner{t: t, steps: []commandStep{
		{requireLocked: locker, stdout: "Logged in\n"},
		{requireLocked: locker, stdout: "Already satisfied. Tests not run.\n"},
	}}
	request := []byte(`{"requestedChange":{"kind":"incremental","policyChanges":[{"kind":"added","policyId":"implementation/package-manager"}]}}`)
	result, err := (Generator{Repository: RepositoryPreflight{Commands: git, Locks: locker}, Codex: codex}).Run(context.Background(), GenerateOptions{
		Repository: target, GitExecutable: "/absolute/git", CodexExecutable: "/absolute/codex", Request: request,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FinalStatusKnown || result.FinalStatus != "" || locker.locked {
		t.Fatalf("zero-diff result: %#v", result)
	}
	prompt := string(codex.calls[1].Stdin)
	for _, required := range []string{"Apply only the incremental update", "policyChanges", "conventionChanges", "explicitly lists added/removed advisory text", "A removed convention only withdraws that advice", "does not require the opposite behavior or authorize code deletion", "including unchanged Facts", "zero-diff completion is valid", "Do not perform unrelated", "without fixing them", "Never describe unrun or skipped checks as verification success", string(request)} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("incremental prompt missing %q", required)
		}
	}
	if strings.Contains(prompt, "Implement every requested intent node") {
		t.Fatal("full-generation instruction leaked into update")
	}
}

func TestUnknownGenerationModeFailsBeforeCommands(t *testing.T) {
	for _, input := range []string{`{`, `{}`, `{"requestedChange":{"kind":"repair"}}`, `{"requestedChange":{"kind":"no-op"}}`} {
		git, codex := &scriptedCommandRunner{t: t}, &scriptedCommandRunner{t: t}
		_, err := (Generator{Repository: RepositoryPreflight{Commands: git, Locks: &recordingLocker{}}, Codex: codex}).Run(context.Background(), GenerateOptions{Request: []byte(input)})
		if err == nil || len(git.calls) != 0 || len(codex.calls) != 0 {
			t.Fatalf("invalid mode reached execution: %q, %v", input, err)
		}
	}
}

func TestPreparedGenerationAuthenticatesBeforeCandidateAndRetainsLock(t *testing.T) {
	for _, failure := range []string{"none", "authentication", "candidate save"} {
		t.Run(failure, func(t *testing.T) {
			target := t.TempDir()
			locker := &recordingLocker{}
			git := &scriptedCommandRunner{t: t, steps: cleanRepositorySteps(target, locker, true)}
			codex := &scriptedCommandRunner{t: t, steps: []commandStep{{requireLocked: locker, stdout: "Logged in"}, {requireLocked: locker}}}
			if failure == "authentication" {
				codex.steps[0].exitCode = 1
			}
			preflight := RepositoryPreflight{Commands: git, Locks: locker}
			state, err := preflight.Prepare(context.Background(), RepositoryPreflightOptions{Repository: target, GitExecutable: "/absolute/git"})
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			calls := 0
			options := GenerateOptions{Repository: target, GitExecutable: "/absolute/git", CodexExecutable: "/absolute/codex", Request: []byte(`{"requestedChange":{"kind":"full"}}`), BeforeExecute: func() error {
				calls++
				if !locker.locked || len(codex.calls) != 1 {
					t.Fatal("candidate not saved between authentication and exec under lock")
				}
				if failure == "candidate save" {
					return errors.New("disk full")
				}
				return nil
			}}
			runner := Generator{Repository: preflight, Codex: codex}
			_, err = runner.RunPrepared(context.Background(), options, state)
			if (err != nil) != (failure != "none") {
				t.Fatalf("result: %v", err)
			}
			want := 1
			if failure == "authentication" {
				want = 0
			}
			if calls != want {
				t.Fatalf("candidate calls %d", calls)
			}
			want = 2
			if failure != "none" {
				want = 1
			}
			if len(codex.calls) != want {
				t.Fatal("Codex exec ran after pre-dispatch failure")
			}
			if !locker.locked || locker.calls != 1 {
				t.Fatal("prepared runner released or reacquired worktree lock")
			}
			if err := state.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := runner.RunPrepared(context.Background(), options, state); err == nil {
				t.Fatal("closed preflight accepted")
			}
		})
	}
}

func cleanRepositorySteps(target string, locker *recordingLocker, includeFinal bool) []commandStep {
	canonicalTarget, err := canonicalDirectory(target)
	if err != nil {
		panic(err)
	}
	steps := []commandStep{
		{stdout: canonicalTarget + "\n"},
		{requireLocked: locker, stdout: "head\n"},
		{requireLocked: locker, stdout: "H README.md\x00"},
		{requireLocked: locker},
	}
	if includeFinal {
		steps = append(steps, commandStep{requireLocked: locker})
	}
	return steps
}
