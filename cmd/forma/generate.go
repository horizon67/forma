package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/horizon67/forma/internal/agentbackend/codex"
	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/generationprogress"
)

const alphaGenerationTimeout = 30 * time.Minute

var (
	errCodexUnavailable = agentrunner.ErrBackendUnavailable
	errGitUnavailable   = errors.New("Git is unavailable")
	invokeGeneration    = invokeCodexGeneration
	findExecutable      = exec.LookPath
)

type generateInvocation struct {
	Repository         string
	AllowDirty         bool
	Request            []byte
	ReviewRequirements *compiler.ReviewRequirements
	State              *agentrunner.RepositoryState
	GitExecutable      string
	BeforeExecute      func() error
	Progress           *generationprogress.Reporter
}

func invokeCodexGeneration(ctx context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
	runner := agentrunner.Generator{
		Repository: agentrunner.RepositoryPreflight{Commands: agentrunner.OSCommandRunner{}, Locks: agentrunner.WorktreeLocker{}},
		Backend:    codex.Backend{LookPath: findExecutable, Environment: os.Environ()},
	}
	options := agentrunner.GenerateOptions{
		Repository: invocation.Repository, GitExecutable: invocation.GitExecutable,
		AllowDirty: invocation.AllowDirty, Request: append([]byte(nil), invocation.Request...),
		BeforeExecute: invocation.BeforeExecute, Progress: invocation.Progress,
	}
	return runner.RunPrepared(ctx, options, invocation.State)
}

func absoluteExecutable(name string) (string, error) {
	path, err := findExecutable(name)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func runGenerationAttempt(ctx context.Context, invocation generateInvocation, stdout io.Writer) (agentrunner.GenerateResult, error) {
	fmt.Fprintf(stdout, "preparing agent generation in %s (timeout %s)\n", invocation.Repository, alphaGenerationTimeout)
	fmt.Fprintln(stdout, "Press Ctrl+C to cancel safely; cancellation is not rollback, so review the diff before retrying.")
	printReviewRequirements(stdout, invocation.ReviewRequirements)
	return invokeGeneration(ctx, invocation)
}

func printNoOpGeneration(state *agentrunner.RepositoryState, baseline *agentrequest.RequestBaseline, stdout, stderr io.Writer) int {
	if baseline == nil {
		fmt.Fprintln(stderr, "forma: no-op plan has no baseline identity")
		return 1
	}
	if state == nil {
		fmt.Fprintln(stderr, "forma: no-op plan has no repository preflight")
		return 1
	}
	fmt.Fprintln(stdout, "no application or policy changes; agent was not started")
	fmt.Fprintf(stdout, "baseline request SHA-256: %s\n", baseline.RequestSHA256)
	fmt.Fprintf(stdout, "repository preflight passed: %s\n", state.Target)
	if state.Dirty {
		fmt.Fprintln(stdout, "existing uncommitted changes were left untouched (--allow-dirty)")
	}
	fmt.Fprintln(stdout, "No update was applied. Repository behavior and tests were not verified; no repair or audit was performed.")
	return 0
}

func printGenerationResult(writer io.Writer, result agentrunner.GenerateResult) {
	if result.Target == "" {
		return
	}
	if len(bytes.TrimSpace(result.Summary)) > 0 {
		fmt.Fprintln(writer, "AI summary (unverified text; may contain sensitive information):")
		fmt.Fprintln(writer, safeSummary(string(result.Summary)))
	}
	fmt.Fprintf(writer, "repository: %s\n", result.Target)
	if result.ImplementationPromptSHA256 != "" {
		fmt.Fprintf(writer, "implementation prompt SHA-256: %s\n", result.ImplementationPromptSHA256)
	}
	if result.InitialDirty {
		fmt.Fprintln(writer, "warning: generation started from a dirty worktree; current status includes pre-existing changes")
	}
	if !result.FinalStatusKnown {
		fmt.Fprintln(writer, "current Git status: not inspected")
	} else if strings.TrimSpace(result.FinalStatus) == "" {
		fmt.Fprintln(writer, "current Git status: clean")
	} else {
		fmt.Fprintln(writer, "current Git status:")
		lines := strings.Split(strings.TrimSuffix(result.FinalStatus, "\n"), "\n")
		sort.Strings(lines)
		for _, line := range lines {
			fmt.Fprintf(writer, "  %s\n", line)
		}
	}
	fmt.Fprintln(writer, "Forma did not run generated application code or repository tests.")
	fmt.Fprintln(writer, "Agent completion is not verification success; unrun or skipped checks remain unverified.")
	fmt.Fprintln(writer, "Next: review the Git diff, then explicitly run the repository's build and test commands.")
	fmt.Fprintln(writer, "Confirm that boundary tests actually ran and did not skip assertions because the sandbox lacked a runtime capability.")
}

func generationSetupError(err error) bool {
	return errors.Is(err, errCodexUnavailable) ||
		errors.Is(err, errGitUnavailable) ||
		errors.Is(err, agentrunner.ErrAuthentication) ||
		errors.Is(err, agentrunner.ErrInvalidRepository) ||
		errors.Is(err, agentrunner.ErrNotGitWorktree) ||
		errors.Is(err, agentrunner.ErrRepositoryHasNoCommit) ||
		errors.Is(err, agentrunner.ErrWorktreeLocked) ||
		errors.Is(err, agentrunner.ErrWorktreeLockUnavailable) ||
		errors.Is(err, agentrunner.ErrDirtyWorktree) ||
		errors.Is(err, agentrunner.ErrHiddenIndexState) ||
		errors.Is(err, agentrunner.ErrUnsafeRepositoryOwnership)
}

// Preserve readable text/newlines/tabs, but remove terminal controls, Unicode
// format controls (including bidi overrides), and implicit line separators.
func safeSummary(summary string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, summary))
}
