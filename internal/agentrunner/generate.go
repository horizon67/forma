package agentrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrCodexAuthentication identifies a Codex installation that has no
	// usable saved login. The caller can direct the user to `codex login`
	// without conflating setup with an implementation failure.
	ErrCodexAuthentication = errors.New("Codex CLI is not authenticated")
	// ErrCodexFailed identifies a completed Codex process that rejected or
	// failed the implementation task.
	ErrCodexFailed = errors.New("Codex generation failed")
)

// GenerateOptions are the complete inputs to one thin alpha generation run.
// Request contains canonical Generation Request JSON and is passed through
// stdin; it is never materialized in the target repository.
type GenerateOptions struct {
	Repository      string
	GitExecutable   string
	CodexExecutable string
	AllowDirty      bool
	Environment     []string
	Request         []byte
}

// GenerateResult reports what Codex said and the Git-visible state left for a
// person to review. FinalStatus is the repository's complete current status;
// when InitialDirty is true it cannot be attributed solely to Codex.
type GenerateResult struct {
	Target           string
	Worktree         string
	InitialHead      string
	InitialDirty     bool
	FinalStatusKnown bool
	FinalStatus      string
	CodexMessage     []byte
	CodexDiagnostics []byte
}

// Generator connects the existing repository preflight to one Codex process.
// It never runs target application code, tests, or a feedback adapter.
type Generator struct {
	Repository RepositoryPreflight
	Codex      CommandRunner
}

// Run performs one full-request alpha generation and keeps the containing
// worktree lock for authentication, Codex execution, and final status capture.
func (generator Generator) Run(ctx context.Context, options GenerateOptions) (result GenerateResult, returnErr error) {
	if generator.Codex == nil {
		return result, errors.New("generation requires a Codex command runner")
	}
	if len(bytes.TrimSpace(options.Request)) == 0 {
		return result, errors.New("generation requires canonical Generation Request JSON")
	}

	state, err := generator.Repository.Prepare(ctx, RepositoryPreflightOptions{
		Repository:    options.Repository,
		GitExecutable: options.GitExecutable,
		AllowDirty:    options.AllowDirty,
	})
	if err != nil {
		return result, err
	}
	defer func() {
		if closeErr := state.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("release Git worktree lock: %w", closeErr))
		}
	}()
	result.Target = state.Target
	result.Worktree = state.Worktree
	result.InitialHead = state.Head
	result.InitialDirty = state.Dirty

	auth, err := generator.Codex.Run(ctx, Command{
		Executable:  options.CodexExecutable,
		Arguments:   []string{"login", "status"},
		Directory:   state.Target,
		Environment: append([]string(nil), options.Environment...),
	})
	if err != nil {
		return result, fmt.Errorf("check Codex authentication: %w", err)
	}
	if auth.StdoutTruncated || auth.StderrTruncated || auth.WaitDelayed {
		return result, fmt.Errorf("check Codex authentication: output was truncated before status could be established")
	}
	if auth.ExitCode != 0 {
		return result, fmt.Errorf("%w; run `codex login` and retry: %s", ErrCodexAuthentication, commandFailure(auth))
	}

	prompt := implementationPrompt(options.Request)
	codex, codexErr := generator.Codex.Run(ctx, Command{
		Executable: options.CodexExecutable,
		Arguments: []string{
			"exec",
			"--ephemeral",
			"--ignore-user-config",
			"--ignore-rules",
			"--sandbox", "workspace-write",
			"--color", "never",
			"-C", state.Target,
			"-",
		},
		Directory:   state.Target,
		Environment: append([]string(nil), options.Environment...),
		Stdin:       prompt,
	})
	result.CodexMessage = append([]byte(nil), codex.Stdout...)
	result.CodexDiagnostics = append([]byte(nil), codex.Stderr...)

	statusContext := ctx
	cancelStatus := func() {}
	if ctx.Err() != nil {
		statusContext, cancelStatus = context.WithTimeout(context.Background(), 5*time.Second)
	}
	defer cancelStatus()
	status, statusErr := generator.Repository.runGit(statusContext, RepositoryPreflightOptions{
		GitExecutable: options.GitExecutable,
	}, state.Worktree, "status", "--porcelain=v1", "--untracked-files=normal")
	if statusErr == nil {
		statusErr = rejectTruncatedGitOutput("read final Git status", status)
	}
	if statusErr == nil && status.ExitCode != 0 {
		statusErr = fmt.Errorf("read final Git status: %s", commandFailure(status))
	}
	if statusErr == nil {
		result.FinalStatusKnown = true
		result.FinalStatus = string(status.Stdout)
	}

	if codexErr != nil {
		return result, fmt.Errorf("run Codex generation: %w", codexErr)
	}
	if codex.StdoutTruncated || codex.StderrTruncated || codex.WaitDelayed {
		return result, fmt.Errorf("run Codex generation: output was truncated; inspect the repository before retrying")
	}
	if codex.ExitCode != 0 {
		return result, fmt.Errorf("%w with exit %d: %s", ErrCodexFailed, codex.ExitCode, commandFailure(codex))
	}
	if statusErr != nil {
		return result, statusErr
	}
	return result, nil
}

func implementationPrompt(request []byte) []byte {
	var prompt strings.Builder
	prompt.WriteString(`Implement the application described by the authoritative Forma Generation Request below in the current repository.

Rules:
- Treat the request as structured application intent, not as repository-specific implementation instructions.
- Implement every requested intent node and Acceptance Fact in ordinary application code appropriate for this repository.
- Test each Acceptance Fact at its named subject boundary. A page, view, or action surface Fact cannot be satisfied only by testing a lower-layer helper.
- Preserve existing repository conventions and do not weaken or delete existing tests to make the task appear complete.
- Do not modify .forma source files or invent requirements that are absent from the request.
- Human Review Requirements are not machine-verified; make the relevant implementation visible for later human review.
- You may inspect files and run relevant non-destructive build or test commands inside your workspace sandbox.
- Do not create Generation Feedback. Forma stops after your repository edits so a person can review the diff and explicitly run commands.
- Finish with a concise summary of changed files, validation you ran, and remaining human review items.

BEGIN FORMA GENERATION REQUEST JSON
`)
	prompt.Write(bytes.TrimSpace(request))
	prompt.WriteString("\nEND FORMA GENERATION REQUEST JSON\n")
	return []byte(prompt.String())
}
