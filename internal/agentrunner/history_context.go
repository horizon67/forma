package agentrunner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// HistoryContext describes local, per-worktree Git metadata. It is deliberately
// not obtained from ambient GIT_DIR / GIT_WORK_TREE environment variables.
type HistoryContext struct {
	GitDirectory string
	Branch       string
	Head         string
}

func (preflight RepositoryPreflight) HistoryContext(ctx context.Context, state *RepositoryState, git string) (HistoryContext, error) {
	if state == nil || state.lock == nil {
		return HistoryContext{}, errors.New("history requires the worktree lock")
	}
	options := RepositoryPreflightOptions{GitExecutable: git}
	read := func(args ...string) (string, int, error) {
		result, err := preflight.runGit(ctx, options, state.Worktree, args...)
		if err == nil {
			err = rejectTruncatedGitOutput("read history context", result)
		}
		if err != nil {
			return "", 0, err
		}
		if result.ExitCode != 0 {
			return "", result.ExitCode, nil
		}
		value, err := gitSingleLine(result.Stdout)
		return value, 0, err
	}
	directory, code, err := read("rev-parse", "--absolute-git-dir")
	if err != nil || code != 0 || !filepath.IsAbs(directory) {
		return HistoryContext{}, fmt.Errorf("resolve per-worktree Git directory: exit %d: %v", code, err)
	}
	directory, err = canonicalDirectory(directory)
	if err != nil {
		return HistoryContext{}, err
	}
	head, code, err := read("rev-parse", "--verify", "HEAD")
	if err != nil || code != 0 {
		return HistoryContext{}, fmt.Errorf("read history HEAD: exit %d: %v", code, err)
	}
	branch, code, err := read("symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return HistoryContext{}, err
	}
	if code == 1 {
		branch = "detached:" + head
	} else if code != 0 {
		return HistoryContext{}, fmt.Errorf("read history branch: exit %d", code)
	}
	return HistoryContext{GitDirectory: directory, Branch: branch, Head: head}, nil
}

func (preflight RepositoryPreflight) HistoryAncestor(ctx context.Context, state *RepositoryState, git, before string) (bool, error) {
	if state == nil || state.lock == nil {
		return false, errors.New("history requires the worktree lock")
	}
	result, err := preflight.runGit(ctx, RepositoryPreflightOptions{GitExecutable: git}, state.Worktree,
		"merge-base", "--is-ancestor", before, state.Head)
	if err != nil {
		return false, err
	}
	if err := rejectTruncatedGitOutput("check history ancestry", result); err != nil {
		return false, err
	}
	switch result.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("check history ancestry: %s", commandFailure(result))
	}
}
