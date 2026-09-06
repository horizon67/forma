package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationhistory"
)

func beginTestGeneration(t *testing.T, invocation generateInvocation) agentrunner.GenerateResult {
	t.Helper()
	if invocation.BeforeExecute == nil || invocation.State == nil {
		t.Fatal("missing locked history integration")
	}
	if err := invocation.BeforeExecute(); err != nil {
		t.Fatal(err)
	}
	return agentrunner.GenerateResult{Target: invocation.State.Target, Worktree: invocation.State.Worktree,
		InitialHead: invocation.State.Head, InitialDirty: invocation.State.Dirty, FinalStatusKnown: true,
		ImplementationPromptSHA256: strings.Repeat("0123456789abcdef", 4)}
}

func runHistoryCommand(t *testing.T, args []string, exit int, messages ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exit {
		t.Fatalf("exit %d want %d: %s\n%s", code, exit, &stdout, &stderr)
	}
	output := stdout.String() + stderr.String()
	for _, message := range messages {
		if !strings.Contains(output, message) {
			t.Fatalf("missing %q:\n%s", message, output)
		}
	}
	return output
}

func historyFixture(t *testing.T) (string, string, string, []string) {
	t.Helper()
	repo, inputs := newNoOpRepository(t), t.TempDir()
	source, manifest := filepath.Join(inputs, "app.forma"), filepath.Join(inputs, "policy.yaml")
	writeTestFile(t, source, updateSource)
	writeTestFile(t, manifest, updateManifest)
	return repo, source, manifest, []string{"generate", "--repository", repo, "--manifest", manifest, source}
}

func readHistoryRecord(t *testing.T, repo, target string, selectors ...string) (*generationhistory.Store, generationhistory.Identity, generationhistory.Record) {
	t.Helper()
	gitDir := strings.TrimSpace(testGitCommand(t, repo, "rev-parse", "--absolute-git-dir"))
	branch := strings.TrimSpace(testGitCommand(t, repo, "symbolic-ref", "HEAD"))
	identity, err := generationhistory.NewIdentity(repo, target, branch, selectors)
	if err != nil {
		t.Fatal(err)
	}
	store, err := generationhistory.Open(gitDir, repo)
	if err != nil {
		t.Fatal(err)
	}
	return store, identity, store.Record(identity)
}

func TestAutomaticHistoryAllowsInitialGenerationWithRepositoryScaffolding(t *testing.T) {
	repo, _, _, args := historyFixture(t)
	for _, name := range []string{".github/workflows/ci.yml", ".github/workflows/release.yaml", ".github/CODEOWNERS", "CODEOWNERS", "LICENSE.txt", ".editorconfig", "Makefile"} {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, path, "# existing scaffolding\n")
	}
	testGitCommand(t, repo, "add", ".")
	testGitCommand(t, repo, "-c", "user.name=Forma Test", "-c", "user.email=forma@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "Repository scaffolding")
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	calls := 0
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		calls++
		return beginTestGeneration(t, in), nil
	}
	runHistoryCommand(t, args, 0, "generation request: full")
	if calls != 1 || testGitCommand(t, repo, "status", "--porcelain") != "" {
		t.Fatal("initial generation skipped the runner or changed scaffolding")
	}
}

func TestMissingHistoryDistinguishesInitialGenerationFromExplicitAdoption(t *testing.T) {
	for _, name := range []string{"app.go", ".github/scripts/deploy.sh", ".github/workflows/implementation.go", "nested/Makefile", "unknown.config"} {
		t.Run(name, func(t *testing.T) {
			repo, source, manifest, args := historyFixture(t)
			path := filepath.Join(repo, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, "existing content\n")
			args = append(args, "--allow-dirty")
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
				t.Fatal("refusal or explicit no-op dispatched the runner")
				return agentrunner.GenerateResult{}, nil
			}
			runHistoryCommand(t, args, 1, "for initial generation, use an empty target directory", "for adoption only", "do not manufacture a baseline")
			store, _, record := readHistoryRecord(t, repo, repo, source)
			if record.Baseline != nil || store.PendingKey != "" {
				t.Fatal("refusal created a baseline")
			}
			baseline := saveBaseline(t, t.TempDir(), source, manifest)
			runHistoryCommand(t, append(args, "--previous", baseline), 0,
				"asserting that the target already implements the supplied Request", "no application code was generated", "explicit baseline imported")
		})
	}
}

func TestFreshTargetRejectsScaffoldingSymlinks(t *testing.T) {
	for _, name := range []string{"LICENSE.txt", ".github/workflows/ci.yml"} {
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			readme := filepath.Join(target, "README.md")
			writeTestFile(t, readme, "scaffolding\n")
			path := filepath.Join(target, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(readme, path); err != nil {
				t.Fatal(err)
			}
			if err := freshGenerationTarget(target, nil, ""); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("symlink was treated as ordinary scaffolding: %v", err)
			}
		})
	}
}

func TestMovedHistoryRecoveryRequiresMovingAsideInvalidCatalogBeforeImport(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), nil
	}
	runHistoryCommand(t, args, 0)
	_, _, record := readHistoryRecord(t, repo, repo, source)
	baseline := filepath.Join(t.TempDir(), "reviewed.json")
	writeTestFile(t, baseline, record.Baseline.Request)
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}
	moved, err := filepath.EvalSymlinks(moved)
	if err != nil {
		t.Fatal(err)
	}
	args[2] = moved
	gitDir := strings.TrimSpace(testGitCommand(t, moved, "rev-parse", "--absolute-git-dir"))
	historyPath := filepath.Join(gitDir, "forma", "generation-history.json")
	markerPath := filepath.Join(gitDir, "forma", "generation-pending")
	identity, err := generationhistory.NewIdentity(moved, moved, record.Identity.Branch, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		t.Fatal("invalid history or no-op import dispatched the runner")
		return agentrunner.GenerateResult{}, nil
	}
	explicit := append(append([]string(nil), args...), "--previous", baseline)
	for _, command := range [][]string{args, explicit} {
		runHistoryCommand(t, command, 1, historyPath, markerPath, identity.Key(), "back up and move aside", "--previous alone cannot", "all applications in this worktree")
	}
	// Follow the printed recovery procedure on this disposable fixture. No
	// pending marker exists after its successful first generation.
	if err := os.Rename(historyPath, historyPath+".backup"); err != nil {
		t.Fatal(err)
	}
	runHistoryCommand(t, explicit, 0, "explicit baseline imported", "no application or policy changes")
	runHistoryCommand(t, args, 0, "no application or policy changes")
}

func TestAutomaticHistoryFullIncrementalNoOpAndExactInput(t *testing.T) {
	repo, source, manifest, args := historyFixture(t)
	oldInvoke, oldFind := invokeGeneration, findExecutable
	t.Cleanup(func() { invokeGeneration, findExecutable = oldInvoke, oldFind })
	var requests [][]byte
	invokeGeneration = func(_ context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
		generated := beginTestGeneration(t, invocation)
		store, _, record := readHistoryRecord(t, repo, repo, source)
		if store.PendingKey == "" || !record.Pending || record.LastAttempt.Status != "running" || record.LastAttempt.Input.Request != string(invocation.Request) {
			t.Fatal("agent did not receive the exact persisted pending input")
		}
		requests = append(requests, append([]byte(nil), invocation.Request...))
		return generated, nil
	}
	runHistoryCommand(t, args, 0, "generation request: full", "comparison baseline saved", "human review: pending")
	store, _, record := readHistoryRecord(t, repo, repo, source)
	if store.PendingKey != "" || record.Pending || record.Baseline.Request != string(requests[0]) || record.Baseline.Verification != "unverified" {
		t.Fatal("invalid completed history")
	}
	originalHistory, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	findExecutable = func(name string) (string, error) {
		if name != "git" {
			t.Fatalf("no-op looked up %s", name)
		}
		return oldFind(name)
	}
	writeTestFile(t, source, "// presentation only\n"+updateSource)
	runHistoryCommand(t, args, 0, "no application or policy changes", "tests were not verified")
	after, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || !bytes.Equal(after, originalHistory) || testGitCommand(t, repo, "status", "--porcelain") != "" {
		t.Fatal("automatic no-op changed history or application")
	}
	findExecutable = oldFind
	// Baseline selection after a normal descendant commit must retain the
	// original execution HEAD, not re-label it as an assertion at the new HEAD.
	testGitCommand(t, repo, "-c", "user.name=Forma Test", "-c", "user.email=forma@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "Descendant commit")
	writeTestFile(t, manifest, strings.Replace(updateManifest, "value: bun", "value: pnpm", 1))
	runHistoryCommand(t, args, 0, "generation request: incremental")
	_, _, updated := readHistoryRecord(t, repo, repo, source)
	if updated.LastAttempt.Baseline == nil || *updated.LastAttempt.Baseline != *record.Baseline || updated.LastAttempt.Input.Head == record.Baseline.Head {
		t.Fatal("automatic selection changed the baseline's provenance")
	}
	policy, err := agentrequest.UnmarshalRequest(requests[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.RequestedChange.PolicyChanges) != 1 || len(policy.RequestedChange.IntentChanges) != 0 {
		t.Fatal("policy-only scope was not retained")
	}
	writeTestFile(t, source, strings.Replace(updateSource, "columns name", "columns name\n        paginate 10", 1))
	runHistoryCommand(t, args, 0, "generation request: incremental")
	changed, err := agentrequest.UnmarshalRequest(requests[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := agentrequest.ValidateIncrementalBaseline(changed, policy); err != nil {
		t.Fatal(err)
	}
	// Omitting Manifest inherits the last actual input, not an empty policy.
	runHistoryCommand(t, []string{"generate", "--repository", repo, source}, 0, "no application or policy changes")
	if len(requests) != 3 {
		t.Fatal("unexpected repeated dispatch")
	}
}

func TestAutomaticHistoryFailureBlocksNoOpAndExplicitRecoveryPreservesFailure(t *testing.T) {
	repo, source, manifest, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), nil
	}
	runHistoryCommand(t, args, 0)
	_, _, before := readHistoryRecord(t, repo, repo, source)
	baselineFile := filepath.Join(t.TempDir(), "reviewed.json")
	writeTestFile(t, baselineFile, before.Baseline.Request)
	writeTestFile(t, manifest, strings.Replace(updateManifest, "value: bun", "value: pnpm", 1))
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), agentrunner.ErrCodexFailed
	}
	runHistoryCommand(t, args, 1, "Codex generation failed")
	store, identity, failed := readHistoryRecord(t, repo, repo, source)
	if store.PendingKey == "" || !failed.Pending || failed.LastAttempt.Status != "failed" || failed.Baseline.Request != before.Baseline.Request {
		t.Fatal("failure replaced the valid comparison baseline")
	}
	writeTestFile(t, manifest, updateManifest)
	invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
		t.Fatal("failed history dispatched an agent")
		return agentrunner.GenerateResult{}, nil
	}
	runHistoryCommand(t, args, 1, "previous generation failed", store.Path, identity.Key())
	runHistoryCommand(t, append(append([]string(nil), args...), "--previous", baselineFile), 0, "explicit recovery", "no application or policy changes")
	store, _, recovered := readHistoryRecord(t, repo, repo, source)
	if store.PendingKey != "" || recovered.Pending || recovered.LastAttempt.Status != "failed" || recovered.Baseline.Origin != "explicit" {
		t.Fatal("explicit recovery hid the previous failure")
	}
	runHistoryCommand(t, args, 0, "no application or policy changes")
}

func TestAutomaticHistoryRejectsMissingCorruptAndMismatchedState(t *testing.T) {
	for _, scenario := range []string{"existing implementation", "ignored implementation", "broken JSON", "wrong schema", "missing catalog", "different worktree"} {
		t.Run(scenario, func(t *testing.T) {
			repo, source, _, args := historyFixture(t)
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				return beginTestGeneration(t, in), nil
			}
			if scenario == "existing implementation" || scenario == "ignored implementation" {
				writeTestFile(t, filepath.Join(repo, "app.go"), "package app\n")
				if scenario == "ignored implementation" {
					writeTestFile(t, filepath.Join(repo, ".gitignore"), "app.go\n")
				}
				args = append(args, "--allow-dirty")
			} else {
				runHistoryCommand(t, args, 0)
				store, _, _ := readHistoryRecord(t, repo, repo, source)
				b, err := os.ReadFile(store.Path)
				if err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "broken JSON":
					writeTestFile(t, store.Path, "{")
				case "wrong schema":
					writeTestFile(t, store.Path, strings.Replace(string(b), generationhistory.Schema, "future", 1))
				case "different worktree":
					writeTestFile(t, store.Path, strings.Replace(string(b), repo, "/different/checkout", 1))
				case "missing catalog":
					if err := os.Remove(store.Path); err != nil {
						t.Fatal(err)
					}
					writeTestFile(t, filepath.Join(repo, "app.go"), "package app\n")
					args = append(args, "--allow-dirty")
				}
			}
			invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
				t.Fatal("invalid history dispatched")
				return agentrunner.GenerateResult{}, errors.New("unexpected")
			}
			runHistoryCommand(t, args, 1)
		})
	}
}

func TestHistoryLockCoversSelectionExecutionAndCompletion(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		generated := beginTestGeneration(t, in)
		runHistoryCommand(t, args, 2, "already used by another Forma generation")
		_, _, record := readHistoryRecord(t, repo, repo, source)
		if !record.Pending {
			t.Fatal("concurrent command cleared pending history")
		}
		return generated, nil
	}
	runHistoryCommand(t, args, 0)
	runHistoryCommand(t, args, 0, "no application or policy changes")
}

func TestDirectorySelectorKeepsIdentityWhenSourcesAreAdded(t *testing.T) {
	_, source, _, args := historyFixture(t)
	directory := filepath.Dir(source)
	args[len(args)-1] = directory
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	var seen []agentrequest.Request
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		request, err := agentrequest.UnmarshalRequest(in.Request)
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, request)
		return beginTestGeneration(t, in), nil
	}
	runHistoryCommand(t, args, 0, "generation request: full")
	writeTestFile(t, filepath.Join(directory, "roles.forma"), "role guest\n")
	runHistoryCommand(t, args, 0, "generation request: incremental")
	if len(seen) != 2 || len(seen[1].RequestedChange.IntentChanges) == 0 {
		t.Fatal("source-set addition lost application identity")
	}
	runHistoryCommand(t, args, 0, "no application or policy changes")
	args[len(args)-1] = source
	runHistoryCommand(t, args, 1, "different source selectors or branch")
}

func TestMultipleTargetsAndApplicationsHaveSeparateHistory(t *testing.T) {
	repo, source, manifest, _ := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	calls := 0
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		calls++
		return beginTestGeneration(t, in), nil
	}
	a, b := filepath.Join(repo, "a"), filepath.Join(repo, "b")
	for _, target := range []string{a, b} {
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, target := range []string{a, b, a, b} {
		runHistoryCommand(t, []string{"generate", "--repository", target, "--manifest", manifest, source}, 0)
	}
	if calls != 2 {
		t.Fatalf("target history conflated: %d calls", calls)
	}
	other := filepath.Join(filepath.Dir(source), "other.forma")
	writeTestFile(t, other, updateSource)
	args := []string{"generate", "--repository", a, "--manifest", manifest, other}
	runHistoryCommand(t, args, 1, "different source selectors")
	baseline := saveBaseline(t, t.TempDir(), other, manifest)
	runHistoryCommand(t, append(append([]string(nil), args...), "--previous", baseline), 0, "explicit baseline imported")
	runHistoryCommand(t, args, 0, "no application or policy changes")
	_, _, first := readHistoryRecord(t, repo, a, source)
	_, _, second := readHistoryRecord(t, repo, a, other)
	if first.Baseline == nil || second.Baseline == nil || first.Identity.Key() == second.Identity.Key() {
		t.Fatal("application source selections conflated")
	}
}

func TestBranchAndLinkedWorktreeRequireDeliberateBaselineBinding(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	old := invokeGeneration
	t.Cleanup(func() { invokeGeneration = old })
	invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
		return beginTestGeneration(t, in), nil
	}
	originalBranch := strings.TrimSpace(testGitCommand(t, repo, "symbolic-ref", "--short", "HEAD"))
	runHistoryCommand(t, args, 0)
	store, originalIdentity, record := readHistoryRecord(t, repo, repo, source)
	baseline := filepath.Join(t.TempDir(), "reviewed.json")
	writeTestFile(t, baseline, record.Baseline.Request)
	writeTestFile(t, filepath.Join(repo, "app.go"), "package app\n")
	testGitCommand(t, repo, "add", "app.go")
	testGitCommand(t, repo, "-c", "user.name=Forma Test", "-c", "user.email=forma@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "Reviewed implementation")
	runHistoryCommand(t, args, 0, "no application or policy changes")
	testGitCommand(t, repo, "switch", "-qc", "other")
	_, currentIdentity, _ := readHistoryRecord(t, repo, repo, source)
	runHistoryCommand(t, args, 1, "different source selectors or branch", store.Path,
		"generation history key: "+currentIdentity.Key(), "existing history key: "+originalIdentity.Key(), originalIdentity.Branch)
	runHistoryCommand(t, append(append([]string(nil), args...), "--previous", baseline), 0, "explicit baseline imported")
	runHistoryCommand(t, args, 0, "no application or policy changes")
	testGitCommand(t, repo, "switch", "-q", originalBranch)
	runHistoryCommand(t, args, 0, "no application or policy changes")
	linked := filepath.Join(t.TempDir(), "linked")
	testGitCommand(t, repo, "worktree", "add", "-qb", "linked", linked)
	linked, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	linkedArgs := append([]string(nil), args...)
	linkedArgs[2] = linked
	runHistoryCommand(t, linkedArgs, 1, "refusing full regeneration")
	runHistoryCommand(t, append(append([]string(nil), linkedArgs...), "--previous", baseline), 0)
	linkedStore, _, _ := readHistoryRecord(t, linked, linked, source)
	if linkedStore.Path == store.Path {
		t.Fatal("linked worktree shared generation history")
	}
	runHistoryCommand(t, linkedArgs, 0, "no application or policy changes")
	// Rewrite only this disposable fixture's ref to an unrelated commit using
	// the identical tree: it stays clean, but ancestry must fail closed.
	tree := strings.TrimSpace(testGitCommand(t, repo, "rev-parse", "HEAD^{tree}"))
	unrelated := strings.TrimSpace(testGitCommand(t, repo, "-c", "user.name=Forma Test", "-c", "user.email=forma@example.invalid", "commit-tree", tree, "-m", "Unrelated history"))
	testGitCommand(t, repo, "update-ref", "HEAD", unrelated)
	runHistoryCommand(t, args, 1, "not an ancestor", store.Path, originalIdentity.Key())
}

func TestHistoryDoesNotPromoteAfterGitOrHistoryChangesDuringAgentRun(t *testing.T) {
	for _, mutation := range []string{"branch", "catalog", "unknown final status", "timeout"} {
		t.Run(mutation, func(t *testing.T) {
			repo, source, _, args := historyFixture(t)
			old := invokeGeneration
			t.Cleanup(func() { invokeGeneration = old })
			invokeGeneration = func(_ context.Context, in generateInvocation) (agentrunner.GenerateResult, error) {
				generated := beginTestGeneration(t, in)
				switch mutation {
				case "branch":
					testGitCommand(t, repo, "switch", "-qc", "agent-changed")
				case "catalog":
					store, _, _ := readHistoryRecord(t, repo, repo, source)
					data, err := os.ReadFile(store.Path)
					if err != nil {
						t.Fatal(err)
					}
					writeTestFile(t, store.Path, string(data)+"\n")
				case "unknown final status":
					generated.FinalStatusKnown = false
				case "timeout":
					return generated, context.DeadlineExceeded
				}
				return generated, nil
			}
			runHistoryCommand(t, args, 1)
			invokeGeneration = func(context.Context, generateInvocation) (agentrunner.GenerateResult, error) {
				t.Fatal("incomplete history dispatched")
				return agentrunner.GenerateResult{}, nil
			}
			runHistoryCommand(t, args, 1)
		})
	}
}

// Exercise the actual process runner and history hook with a deterministic
// local stand-in, never a real Codex service or generated application.
func TestManagedGenerationThroughRealRunnerWithCodexStandIn(t *testing.T) {
	repo, source, _, args := historyFixture(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	standIn := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\nexec " + quote(executable) + " -test.run=^TestHistoryCodexHelperProcess$ -- " + quote(repo) + " " + quote(source) + " \"$@\"\n"
	if err := os.WriteFile(standIn, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	oldInvoke, oldFind := invokeGeneration, findExecutable
	t.Cleanup(func() { invokeGeneration, findExecutable = oldInvoke, oldFind })
	invokeGeneration = invokeCodexGeneration
	findExecutable = func(name string) (string, error) {
		if name == "codex" {
			return standIn, nil
		}
		return oldFind(name)
	}
	args = append(args, "--allow-dirty")
	runHistoryCommand(t, args, 0, "Deterministic stand-in completed", "comparison baseline saved")
	_, _, record := readHistoryRecord(t, repo, repo, source)
	if record.LastAttempt.Status != "completed" || record.LastAttempt.PromptSHA256 == "" {
		t.Fatal("real runner did not record completed execution")
	}
	findExecutable = func(name string) (string, error) {
		if name == "codex" {
			t.Fatal("automatic no-op looked up the stand-in")
		}
		return oldFind(name)
	}
	runHistoryCommand(t, args, 0, "no application or policy changes")
	content, err := os.ReadFile(filepath.Join(repo, "generated.txt"))
	if err != nil || string(content) != "stand-in output\n" {
		t.Fatal("no-op changed stand-in output")
	}
}

func TestHistoryCodexHelperProcess(t *testing.T) {
	index := -1
	for n, arg := range os.Args {
		if arg == "--" {
			index = n
			break
		}
	}
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	if len(args) < 3 {
		t.Fatal("missing helper arguments")
	}
	repo, source := args[0], args[1]
	if args[2] == "login" {
		fmt.Println("Logged in using a deterministic stand-in")
		os.Exit(0)
	}
	if args[2] != "exec" {
		t.Fatal("unexpected stand-in command")
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	_, _, record := readHistoryRecord(t, repo, repo, source)
	if !record.Pending || record.LastAttempt.Status != "running" {
		t.Fatal("editing process started before the candidate was saved")
	}
	if !bytes.Contains(input, []byte("BEGIN FORMA GENERATION REQUEST JSON\n"+record.LastAttempt.Input.Request+"\nEND FORMA GENERATION REQUEST JSON")) {
		t.Fatal("saved request differs from actual process stdin")
	}
	if err := os.WriteFile(filepath.Join(repo, "generated.txt"), []byte("stand-in output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fmt.Println("Deterministic stand-in completed; tests not run.")
	os.Exit(0)
}
