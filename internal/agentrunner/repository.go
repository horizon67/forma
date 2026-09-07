package agentrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrNotGitWorktree identifies a target that Git cannot resolve to a
	// containing worktree.
	ErrNotGitWorktree = errors.New("target is not a Git worktree")
	// ErrInvalidRepository identifies an absent target or a target that is not
	// a directory before Git inspection begins.
	ErrInvalidRepository = errors.New("target repository is invalid")
	// ErrRepositoryHasNoCommit identifies an initialized worktree without the
	// committed HEAD required for a reviewable before/after diff.
	ErrRepositoryHasNoCommit = errors.New("Git worktree has no committed HEAD")
	// ErrWorktreeLocked identifies another Forma generation run holding the
	// containing worktree lock.
	ErrWorktreeLocked = errors.New("Git worktree is already used by another Forma generation run")
	// ErrWorktreeLockUnavailable identifies a platform or filesystem on which
	// the required lifetime advisory lock could not be established.
	ErrWorktreeLockUnavailable = errors.New("Git worktree advisory lock is unavailable")
	// ErrDirtyWorktree identifies Git-visible uncommitted user work in the
	// containing worktree. It is not a tamper signal for .git contents.
	ErrDirtyWorktree = errors.New("Git worktree has uncommitted changes")
	// ErrHiddenIndexState identifies index flags that make Git status omit
	// working-tree changes.
	ErrHiddenIndexState = errors.New("Git index contains flags that hide working tree changes")
	// ErrUnsafeRepositoryOwnership identifies Git's safe.directory ownership
	// refusal while ambient user and system configuration is disabled.
	ErrUnsafeRepositoryOwnership = errors.New("Git refused repository ownership")
)

// DirectoryLocker takes a lifetime lock on a canonical directory. The lock is
// released by closing the returned value.
type DirectoryLocker interface {
	Lock(string) (io.Closer, error)
}

// RepositoryPreflightOptions are the inputs needed for the repository-only
// portion of generate preflight.
type RepositoryPreflightOptions struct {
	Repository    string
	GitExecutable string
	AllowDirty    bool
}

// RepositoryState is the immutable Git evidence captured while Lock remains
// held. Call Close on every handled return from the enclosing generate run.
type RepositoryState struct {
	Target   string
	Worktree string
	Head     string
	Status   string
	Dirty    bool

	lock io.Closer
}

// Close releases the lifetime worktree lock.
func (state *RepositoryState) Close() error {
	if state == nil || state.lock == nil {
		return nil
	}
	err := state.lock.Close()
	state.lock = nil
	return err
}

// RepositoryPreflight resolves a target through Git, locks the canonical
// containing worktree before inspecting its status, and enforces the clean
// tree policy. It intentionally performs no agent or target executable work.
type RepositoryPreflight struct {
	Commands CommandRunner
	Locks    DirectoryLocker
}

func (preflight RepositoryPreflight) Prepare(ctx context.Context, options RepositoryPreflightOptions) (*RepositoryState, error) {
	if preflight.Commands == nil {
		return nil, errors.New("repository preflight requires a command runner")
	}
	if preflight.Locks == nil {
		return nil, errors.New("repository preflight requires a directory locker")
	}
	if !filepath.IsAbs(options.GitExecutable) {
		return nil, fmt.Errorf("Git executable must be absolute: %q", options.GitExecutable)
	}

	target, err := canonicalDirectory(options.Repository)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve target repository: %v", ErrInvalidRepository, err)
	}
	rootResult, err := preflight.runGit(ctx, options, target, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	if err := rejectTruncatedGitOutput("resolve Git worktree", rootResult); err != nil {
		return nil, err
	}
	if rootResult.ExitCode != 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotGitWorktree, commandFailure(rootResult))
	}
	rootText, err := gitSingleLine(rootResult.Stdout)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid git rev-parse output: %v", ErrNotGitWorktree, err)
	}
	if !filepath.IsAbs(rootText) {
		rootText = filepath.Join(target, rootText)
	}
	worktree, err := canonicalDirectory(rootText)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve Git worktree: %v", ErrNotGitWorktree, err)
	}
	if !pathContains(worktree, target) {
		return nil, fmt.Errorf("%w: Git returned %s for target %s", ErrNotGitWorktree, worktree, target)
	}

	lock, err := preflight.Locks.Lock(worktree)
	if err != nil {
		return nil, fmt.Errorf("lock Git worktree %s: %w", worktree, err)
	}
	if lock == nil {
		return nil, fmt.Errorf("lock Git worktree %s: %w: locker returned no lifetime handle", worktree, ErrWorktreeLockUnavailable)
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()

	headResult, err := preflight.runGit(ctx, options, worktree, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, err
	}
	if err := rejectTruncatedGitOutput("read Git HEAD", headResult); err != nil {
		return nil, err
	}
	if headResult.ExitCode != 0 {
		return nil, fmt.Errorf("%w; create an initial commit before generation: %s", ErrRepositoryHasNoCommit, commandFailure(headResult))
	}
	head, err := gitSingleLine(headResult.Stdout)
	if err != nil {
		return nil, fmt.Errorf("read Git HEAD: %w", err)
	}

	indexResult, err := preflight.runGit(ctx, options, worktree, "ls-files", "-v", "-z")
	if err != nil {
		return nil, err
	}
	if err := rejectTruncatedGitOutput("read Git index flags", indexResult); err != nil {
		return nil, err
	}
	if indexResult.ExitCode != 0 {
		return nil, fmt.Errorf("read Git index flags: %s", commandFailure(indexResult))
	}
	hiddenPath, hidden, err := hiddenIndexEntry(indexResult.Stdout)
	if err != nil {
		return nil, fmt.Errorf("read Git index flags: %w", err)
	}
	if hidden {
		return nil, fmt.Errorf("%w: %q uses assume-unchanged or skip-worktree", ErrHiddenIndexState, hiddenPath)
	}

	statusResult, err := preflight.runGit(ctx, options, worktree, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	if err := rejectTruncatedGitOutput("read Git status", statusResult); err != nil {
		return nil, err
	}
	if statusResult.ExitCode != 0 {
		return nil, fmt.Errorf("read Git status: %s", commandFailure(statusResult))
	}
	status := string(statusResult.Stdout)
	dirty := status != ""
	if dirty && !options.AllowDirty {
		return nil, fmt.Errorf("%w; commit or stash the changes, or pass --allow-dirty", ErrDirtyWorktree)
	}

	keepLock = true
	return &RepositoryState{
		Target:   target,
		Worktree: worktree,
		Head:     head,
		Status:   status,
		Dirty:    dirty,
		lock:     lock,
	}, nil
}

func (preflight RepositoryPreflight) runGit(ctx context.Context, options RepositoryPreflightOptions, directory string, arguments ...string) (CommandResult, error) {
	result, err := preflight.Commands.Run(ctx, Command{
		Executable:  options.GitExecutable,
		Arguments:   append([]string{"-c", "core.excludesFile=" + os.DevNull, "-c", "core.fsmonitor=false", "-C", directory}, arguments...),
		Directory:   directory,
		Environment: gitEnvironment(),
	})
	if err != nil {
		return CommandResult{}, fmt.Errorf("run Git: %w", err)
	}
	if result.ExitCode != 0 && isUnsafeRepositoryOwnership(result.Stderr) {
		return result, fmt.Errorf("%w; run Forma as the repository owner or correct the ownership (ambient safe.directory overrides are intentionally ignored)", ErrUnsafeRepositoryOwnership)
	}
	return result, nil
}

// gitEnvironment is owned by repository preflight rather than its caller.
// Git uses GIT_* variables to select configuration, worktrees, and indexes, so
// forwarding a caller environment could make Git-visible user changes appear
// clean. This is a user-work protection gate, not a tamper boundary against a
// target-controlled .git directory; later stages must use trusted snapshots
// and external evidence for that boundary. PATH and HOME stay absent because
// the executable and configuration inputs are explicit.
func gitEnvironment() []string {
	return []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"LANG=C",
		"LC_ALL=C",
	}
}

func canonicalDirectory(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return filepath.Clean(canonical), nil
}

func pathContains(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func commandFailure(result CommandResult) string {
	message := strings.TrimSpace(string(result.Stderr))
	if message == "" {
		message = strings.TrimSpace(string(result.Stdout))
	}
	if message == "" {
		message = fmt.Sprintf("exit %d", result.ExitCode)
	}
	return message
}

func rejectTruncatedGitOutput(operation string, result CommandResult) error {
	if result.StdoutTruncated || result.StderrTruncated {
		return fmt.Errorf("%s: Git output exceeds the configured capture limit", operation)
	}
	return nil
}

func gitSingleLine(output []byte) (string, error) {
	value := string(output)
	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" {
		return "", errors.New("Git returned an empty value")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("Git returned more than one line")
	}
	return value, nil
}

func hiddenIndexEntry(output []byte) (string, bool, error) {
	if len(output) == 0 {
		return "", false, nil
	}
	if output[len(output)-1] != 0 {
		return "", false, errors.New("Git returned a non-NUL-terminated index entry")
	}
	for _, record := range bytes.Split(output[:len(output)-1], []byte{0}) {
		if len(record) < 3 || record[1] != ' ' {
			return "", false, errors.New("Git returned a malformed index entry")
		}
		tag := record[0]
		if tag == 'S' || (tag >= 'a' && tag <= 'z') {
			return string(record[2:]), true, nil
		}
	}
	return "", false, nil
}

func isUnsafeRepositoryOwnership(stderr []byte) bool {
	message := strings.ToLower(string(stderr))
	return strings.Contains(message, "detected dubious ownership") || strings.Contains(message, "safe.directory")
}
