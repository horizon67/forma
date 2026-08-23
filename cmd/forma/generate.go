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

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/compiler"
)

const alphaGenerationTimeout = 30 * time.Minute

var (
	errCodexUnavailable = errors.New("Codex CLI is unavailable")
	errGitUnavailable   = errors.New("Git is unavailable")
	invokeGeneration    = invokeCodexGeneration
	findExecutable      = exec.LookPath
)

type generateInvocation struct {
	Repository         string
	AllowDirty         bool
	Request            []byte
	ReviewRequirements *compiler.ReviewRequirements
}

func invokeCodexGeneration(ctx context.Context, invocation generateInvocation) (agentrunner.GenerateResult, error) {
	gitExecutable, err := absoluteExecutable("git")
	if err != nil {
		return agentrunner.GenerateResult{}, fmt.Errorf("%w: %v", errGitUnavailable, err)
	}
	codexExecutable, err := absoluteExecutable("codex")
	if err != nil {
		return agentrunner.GenerateResult{}, fmt.Errorf("%w: %v", errCodexUnavailable, err)
	}

	runner := agentrunner.Generator{
		Repository: agentrunner.RepositoryPreflight{
			Commands: agentrunner.OSCommandRunner{},
			Locks:    agentrunner.WorktreeLocker{},
		},
		Codex: agentrunner.OSCommandRunner{ProcessGroup: true},
	}
	return runner.Run(ctx, agentrunner.GenerateOptions{
		Repository:      invocation.Repository,
		GitExecutable:   gitExecutable,
		CodexExecutable: codexExecutable,
		AllowDirty:      invocation.AllowDirty,
		Environment:     codexEnvironment(os.Environ()),
		Request:         append([]byte(nil), invocation.Request...),
	})
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

// codexEnvironment is deliberately smaller than the caller environment. The
// thin runner reuses Codex's saved login and basic host/network settings, but
// never forwards arbitrary application secrets or API-key values.
func codexEnvironment(environ []string) []string {
	values := map[string]string{}
	for _, entry := range environ {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	allowed := []string{
		"PATH", "HOME", "CODEX_HOME",
		"TMPDIR", "TMP", "TEMP",
		"USER", "LOGNAME", "SHELL", "TERM", "COLORTERM",
		"LANG", "LC_ALL", "LC_CTYPE", "NO_COLOR",
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
		"http_proxy", "https_proxy", "all_proxy", "no_proxy",
		"CODEX_CA_CERTIFICATE", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
	}
	result := make([]string, 0, len(allowed))
	for _, name := range allowed {
		if value, ok := values[name]; ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func runGeneration(invocation generateInvocation, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), alphaGenerationTimeout)
	defer cancel()

	fmt.Fprintf(stdout, "starting Codex generation in %s (timeout %s)\n", invocation.Repository, alphaGenerationTimeout)
	printReviewRequirements(stdout, invocation.ReviewRequirements)
	result, err := invokeGeneration(ctx, invocation)
	printGenerationResult(stdout, result)
	if err != nil {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		if len(result.CodexDiagnostics) > 0 {
			fmt.Fprintln(stderr, "Codex diagnostics:")
			fmt.Fprint(stderr, strings.TrimSpace(string(result.CodexDiagnostics)))
			fmt.Fprintln(stderr)
		}
		if generationSetupError(err) {
			return 2
		}
		return 1
	}
	return 0
}

func printGenerationResult(writer io.Writer, result agentrunner.GenerateResult) {
	if result.Target == "" {
		return
	}
	if len(bytes.TrimSpace(result.CodexMessage)) > 0 {
		fmt.Fprintln(writer, "Codex summary:")
		fmt.Fprintln(writer, strings.TrimSpace(string(result.CodexMessage)))
	}
	fmt.Fprintf(writer, "repository: %s\n", result.Target)
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
	fmt.Fprintln(writer, "Next: review the Git diff, then explicitly run the repository's build and test commands.")
}

func generationSetupError(err error) bool {
	return errors.Is(err, errCodexUnavailable) ||
		errors.Is(err, errGitUnavailable) ||
		errors.Is(err, agentrunner.ErrCodexAuthentication) ||
		errors.Is(err, agentrunner.ErrInvalidRepository) ||
		errors.Is(err, agentrunner.ErrNotGitWorktree) ||
		errors.Is(err, agentrunner.ErrRepositoryHasNoCommit) ||
		errors.Is(err, agentrunner.ErrWorktreeLocked) ||
		errors.Is(err, agentrunner.ErrWorktreeLockUnavailable) ||
		errors.Is(err, agentrunner.ErrDirtyWorktree) ||
		errors.Is(err, agentrunner.ErrHiddenIndexState) ||
		errors.Is(err, agentrunner.ErrUnsafeRepositoryOwnership)
}
