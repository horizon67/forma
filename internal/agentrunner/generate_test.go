package agentrunner

import (
	"context"
	"errors"
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
	request := []byte(`{"schema":"forma/generation-request/v0alpha4","resolvedIntent":{"version":"test"}}`)
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
	if !strings.Contains(prompt, "BEGIN FORMA GENERATION REQUEST JSON\n"+string(request)+"\nEND FORMA GENERATION REQUEST JSON") {
		t.Fatalf("implementation prompt does not preserve request bytes:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not create Generation Feedback") || !strings.Contains(prompt, "Do not modify .forma source files") {
		t.Fatalf("implementation prompt omits the thin-runner boundary:\n%s", prompt)
	}
	if !strings.Contains(prompt, "A page, view, or action surface Fact cannot be satisfied only by testing a lower-layer helper") {
		t.Fatalf("implementation prompt omits dogfood-derived review boundaries:\n%s", prompt)
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
		Repository: target, GitExecutable: "/absolute/git", CodexExecutable: "/absolute/codex", Request: []byte(`{"schema":"request"}`),
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
		Repository: target, GitExecutable: "/absolute/git", CodexExecutable: "/absolute/codex", Request: []byte(`{"schema":"request"}`),
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
