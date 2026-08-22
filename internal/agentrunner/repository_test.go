package agentrunner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryPreflightLocksBeforeReadingHeadOrStatus(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "service")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	locker := &recordingLocker{}
	runner := &scriptedCommandRunner{
		t: t,
		steps: []commandStep{
			{stdout: canonicalRoot + "\n"},
			{requireLocked: locker, stdout: "0123456789abcdef\n"},
			{requireLocked: locker, stdout: "H tracked.txt\x00"},
			{requireLocked: locker, stdout: ""},
		},
	}
	preflight := RepositoryPreflight{Commands: runner, Locks: locker}
	state, err := preflight.Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    target,
		GitExecutable: "/absolute/git",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	if state.Target != canonicalTarget || state.Worktree != canonicalRoot || state.Head != "0123456789abcdef" || state.Dirty {
		t.Fatalf("state = %#v", state)
	}
	if locker.path != canonicalRoot || !locker.locked {
		t.Fatalf("locker = %#v", locker)
	}
	wantArguments := [][]string{
		gitArguments(canonicalTarget, "rev-parse", "--show-toplevel"),
		gitArguments(canonicalRoot, "rev-parse", "--verify", "HEAD"),
		gitArguments(canonicalRoot, "ls-files", "-v", "-z"),
		gitArguments(canonicalRoot, "status", "--porcelain=v1", "--untracked-files=normal"),
	}
	if len(runner.calls) != len(wantArguments) {
		t.Fatalf("calls = %d, want %d", len(runner.calls), len(wantArguments))
	}
	for index, call := range runner.calls {
		if call.Executable != "/absolute/git" || !reflect.DeepEqual(call.Arguments, wantArguments[index]) {
			t.Fatalf("call %d = %#v", index, call)
		}
		if !reflect.DeepEqual(call.Environment, gitEnvironment()) || len(call.Stdin) != 0 {
			t.Fatalf("call %d environment/stdin = %#v / %q", index, call.Environment, call.Stdin)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if locker.locked {
		t.Fatal("Close did not release the worktree lock")
	}
}

func TestRepositoryPreflightRejectsDirtyWorktreeAndReleasesItsLock(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	preflight := RepositoryPreflight{
		Commands: &scriptedCommandRunner{t: t, steps: []commandStep{
			{stdout: root + "\n"},
			{requireLocked: locker, stdout: "head\n"},
			{requireLocked: locker, stdout: "H tracked.txt\x00"},
			{requireLocked: locker, stdout: "?? untracked.txt\n"},
		}},
		Locks: locker,
	}
	_, err := preflight.Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
	})
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("error = %v", err)
	}
	if locker.locked {
		t.Fatal("dirty-tree rejection retained the worktree lock")
	}
}

func TestRepositoryPreflightAllowDirtyDoesNotChangeTheLockLifetime(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	preflight := RepositoryPreflight{
		Commands: &scriptedCommandRunner{t: t, steps: []commandStep{
			{stdout: root + "\n"},
			{requireLocked: locker, stdout: "head\n"},
			{requireLocked: locker, stdout: "H tracked.txt\x00"},
			{requireLocked: locker, stdout: " M tracked.txt\n"},
		}},
		Locks: locker,
	}
	state, err := preflight.Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
		AllowDirty:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty || state.Status != " M tracked.txt\n" || !locker.locked {
		t.Fatalf("state/lock = %#v / %#v", state, locker)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryPreflightFailsBeforeLockingANonRepository(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{{exitCode: 128, stderr: "not a git repository\n"}}}
	_, err := (RepositoryPreflight{Commands: runner, Locks: locker}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
	})
	if !errors.Is(err, ErrNotGitWorktree) {
		t.Fatalf("error = %v", err)
	}
	if locker.calls != 0 || len(runner.calls) != 1 {
		t.Fatalf("locker calls / command calls = %d / %d", locker.calls, len(runner.calls))
	}
}

func TestRepositoryPreflightRejectsUnsafeRepositoryOwnershipWithDedicatedDiagnostic(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{{
		exitCode: 128,
		stderr:   "fatal: detected dubious ownership in repository; add safe.directory\n",
	}}}
	_, err := (RepositoryPreflight{Commands: runner, Locks: locker}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
	})
	if !errors.Is(err, ErrUnsafeRepositoryOwnership) || !strings.Contains(err.Error(), "repository owner") {
		t.Fatalf("error = %v", err)
	}
	if locker.calls != 0 {
		t.Fatalf("unsafe ownership reached locker: %d calls", locker.calls)
	}
}

func TestRepositoryPreflightRejectsAWorktreeThatDoesNotContainTheTarget(t *testing.T) {
	target := t.TempDir()
	unrelated := t.TempDir()
	locker := &recordingLocker{}
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{{stdout: unrelated + "\n"}}}
	_, err := (RepositoryPreflight{Commands: runner, Locks: locker}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    target,
		GitExecutable: "/absolute/git",
	})
	if !errors.Is(err, ErrNotGitWorktree) {
		t.Fatalf("error = %v", err)
	}
	if locker.calls != 0 {
		t.Fatalf("unrelated worktree reached locker: %d calls", locker.calls)
	}
}

func TestRepositoryPreflightFailsClosedOnTruncatedGitEvidence(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{{stdout: root, stdoutTruncated: true}}}
	_, err := (RepositoryPreflight{Commands: runner, Locks: locker}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
	})
	if err == nil || !strings.Contains(err.Error(), "output exceeds") {
		t.Fatalf("error = %v", err)
	}
	if locker.calls != 0 {
		t.Fatalf("truncated root evidence reached the locker: %d calls", locker.calls)
	}
}

func TestRepositoryPreflightReleasesTheLockOnTruncatedStatusEvidence(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{
		{stdout: root + "\n"},
		{requireLocked: locker, stdout: "head\n"},
		{requireLocked: locker, stdout: "H tracked.txt\x00"},
		{requireLocked: locker, stdout: "?? partial", stdoutTruncated: true},
	}}
	_, err := (RepositoryPreflight{Commands: runner, Locks: locker}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
		AllowDirty:    true,
	})
	if err == nil || !strings.Contains(err.Error(), "output exceeds") {
		t.Fatalf("error = %v", err)
	}
	if locker.locked {
		t.Fatal("truncated status evidence retained the worktree lock")
	}
}

func TestRepositoryPreflightLockFailureStopsBeforeTargetInspection(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{{stdout: root + "\n"}}}
	locker := &recordingLocker{err: ErrWorktreeLocked}
	_, err := (RepositoryPreflight{Commands: runner, Locks: locker}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
		AllowDirty:    true,
	})
	if !errors.Is(err, ErrWorktreeLocked) {
		t.Fatalf("error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("commands after lock contention = %d", len(runner.calls)-1)
	}
}

func TestRepositoryPreflightFailsClosedWhenLockerReturnsNoHandle(t *testing.T) {
	root := t.TempDir()
	runner := &scriptedCommandRunner{t: t, steps: []commandStep{{stdout: root + "\n"}}}
	_, err := (RepositoryPreflight{Commands: runner, Locks: nilHandleLocker{}}).Prepare(context.Background(), RepositoryPreflightOptions{
		Repository:    root,
		GitExecutable: "/absolute/git",
	})
	if !errors.Is(err, ErrWorktreeLockUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("commands after missing lock handle = %d", len(runner.calls)-1)
	}
}

func TestWorktreeLockSerializesCanonicalAliasesAndNestedTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("alpha worktree lock is fail-closed on Windows")
	}
	gitExecutable, root := newCommittedRepository(t)
	nested := filepath.Join(root, "services", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "repository-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	preflight := RepositoryPreflight{Commands: OSCommandRunner{}, Locks: WorktreeLocker{}}
	state, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, root, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })

	for _, test := range []struct {
		name       string
		target     string
		allowDirty bool
	}{
		{name: "same target", target: root},
		{name: "nested target", target: nested},
		{name: "symlink alias", target: alias},
		{name: "allow dirty cannot bypass", target: nested, allowDirty: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, test.target, test.allowDirty))
			if !errors.Is(err, ErrWorktreeLocked) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestWorktreeLockDoesNotContendAcrossRepositoriesAndReleasesOnClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("alpha worktree lock is fail-closed on Windows")
	}
	gitExecutable, firstRoot := newCommittedRepository(t)
	_, secondRoot := newCommittedRepository(t)
	preflight := RepositoryPreflight{Commands: OSCommandRunner{}, Locks: WorktreeLocker{}}
	first, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, firstRoot, false))
	if err != nil {
		t.Fatal(err)
	}
	second, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, secondRoot, false))
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, firstRoot, false))
	if err != nil {
		t.Fatalf("lock remained after Close: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRealRepositoryPreflightEnforcesDirtyPolicyAndCapturesEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("alpha worktree lock is fail-closed on Windows")
	}
	gitExecutable, root := newCommittedRepository(t)
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	preflight := RepositoryPreflight{Commands: OSCommandRunner{}, Locks: WorktreeLocker{}}
	_, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, root, false))
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("error = %v", err)
	}
	state, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, root, true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	if !state.Dirty || !strings.Contains(state.Status, "untracked.txt") || len(state.Head) != 40 {
		t.Fatalf("state = %#v", state)
	}
}

func TestRepositoryPreflightIgnoresInheritedGitConfigInjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("alpha worktree lock is fail-closed on Windows")
	}
	gitExecutable, root := newCommittedRepository(t)
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	excludes := filepath.Join(t.TempDir(), "global-excludes")
	if err := os.WriteFile(excludes, []byte("untracked.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.excludesFile")
	t.Setenv("GIT_CONFIG_VALUE_0", excludes)

	preflight := RepositoryPreflight{Commands: OSCommandRunner{}, Locks: WorktreeLocker{}}
	_, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, root, false))
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("inherited Git environment bypassed dirty policy: %v", err)
	}
}

func TestRepositoryPreflightOverridesRepositoryLocalExcludesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("alpha worktree lock is fail-closed on Windows")
	}
	gitExecutable, root := newCommittedRepository(t)
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	excludes := filepath.Join(t.TempDir(), "local-excludes")
	if err := os.WriteFile(excludes, []byte("untracked.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, gitExecutable, root, "config", "core.excludesFile", excludes)

	preflight := RepositoryPreflight{Commands: OSCommandRunner{}, Locks: WorktreeLocker{}}
	_, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, root, false))
	if !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("repository-local excludes file bypassed dirty policy: %v", err)
	}
}

func TestRepositoryPreflightRejectsIndexFlagsThatHideChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("alpha worktree lock is fail-closed on Windows")
	}
	for _, test := range []struct {
		name string
		flag string
	}{
		{name: "assume unchanged", flag: "--assume-unchanged"},
		{name: "skip worktree", flag: "--skip-worktree"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gitExecutable, root := newCommittedRepository(t)
			runTestGit(t, gitExecutable, root, "update-index", test.flag, "tracked.txt")
			preflight := RepositoryPreflight{Commands: OSCommandRunner{}, Locks: WorktreeLocker{}}
			_, err := preflight.Prepare(context.Background(), realGitOptions(gitExecutable, root, false))
			if !errors.Is(err, ErrHiddenIndexState) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGitSingleLineRejectsEmptyOrMultipleValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "line ending only", input: "\n"},
		{name: "two lines", input: "first\nsecond\n"},
		{name: "embedded carriage return", input: "first\rsecond\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := gitSingleLine([]byte(test.input)); err == nil {
				t.Fatalf("gitSingleLine(%q) succeeded", test.input)
			}
		})
	}
}

type commandStep struct {
	stdout          string
	stderr          string
	stdoutTruncated bool
	stderrTruncated bool
	exitCode        int
	err             error
	requireLocked   *recordingLocker
}

type scriptedCommandRunner struct {
	t     *testing.T
	steps []commandStep
	calls []Command
}

func (runner *scriptedCommandRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	runner.t.Helper()
	index := len(runner.calls)
	runner.calls = append(runner.calls, command)
	if index >= len(runner.steps) {
		runner.t.Fatalf("unexpected command %#v", command)
	}
	step := runner.steps[index]
	if step.requireLocked != nil && !step.requireLocked.locked {
		runner.t.Fatalf("command %d ran before the worktree lock was acquired", index)
	}
	return CommandResult{
		Stdout:          []byte(step.stdout),
		Stderr:          []byte(step.stderr),
		StdoutTruncated: step.stdoutTruncated,
		StderrTruncated: step.stderrTruncated,
		ExitCode:        step.exitCode,
	}, step.err
}

type recordingLocker struct {
	path   string
	calls  int
	locked bool
	err    error
}

func (locker *recordingLocker) Lock(path string) (io.Closer, error) {
	locker.calls++
	locker.path = path
	if locker.err != nil {
		return nil, locker.err
	}
	locker.locked = true
	return closerFunc(func() error {
		locker.locked = false
		return nil
	}), nil
}

type closerFunc func() error

func (closer closerFunc) Close() error { return closer() }

type nilHandleLocker struct{}

func (nilHandleLocker) Lock(string) (io.Closer, error) { return nil, nil }

func newCommittedRepository(t *testing.T) (string, string) {
	t.Helper()
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	gitExecutable, err = filepath.Abs(gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	runTestGit(t, gitExecutable, root, "init", "--quiet")
	runTestGit(t, gitExecutable, root, "config", "user.name", "Forma Test")
	runTestGit(t, gitExecutable, root, "config", "user.email", "forma@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, gitExecutable, root, "add", "tracked.txt")
	runTestGit(t, gitExecutable, root, "commit", "--quiet", "-m", "initial")
	return gitExecutable, root
}

func runTestGit(t *testing.T, executable, root string, arguments ...string) {
	t.Helper()
	command := exec.Command(executable, append([]string{"-C", root}, arguments...)...)
	command.Env = gitEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func realGitOptions(executable, repository string, allowDirty bool) RepositoryPreflightOptions {
	return RepositoryPreflightOptions{
		Repository:    repository,
		GitExecutable: executable,
		AllowDirty:    allowDirty,
	}
}

func gitArguments(directory string, arguments ...string) []string {
	return append([]string{
		"-c", "core.excludesFile=" + os.DevNull,
		"-c", "core.fsmonitor=false",
		"-C", directory,
	}, arguments...)
}
