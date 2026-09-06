package agentrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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
	// BeforeExecute persists the candidate after authentication but before any
	// editing process starts. An error prevents Codex exec entirely.
	BeforeExecute func() error
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
	// ImplementationPromptSHA256 identifies the exact instructions and
	// canonical request bytes passed to Codex without retaining either in the
	// target repository.
	ImplementationPromptSHA256 string
	CodexMessage               []byte
	CodexDiagnostics           []byte
}

// Generator connects the existing repository preflight to one Codex process.
// It never runs target application code, tests, or a feedback adapter.
type Generator struct {
	Repository RepositoryPreflight
	Codex      CommandRunner
}

// Run performs one full or incremental alpha generation and keeps the containing
// worktree lock for authentication, Codex execution, and final status capture.
func (generator Generator) Run(ctx context.Context, options GenerateOptions) (result GenerateResult, returnErr error) {
	if generator.Codex == nil {
		return result, errors.New("generation requires a Codex command runner")
	}
	if len(bytes.TrimSpace(options.Request)) == 0 {
		return result, errors.New("generation requires canonical Generation Request JSON")
	}
	prompt, err := implementationPrompt(options.Request)
	if err != nil {
		return result, err
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
	return generator.runPrepared(ctx, options, state, prompt)
}

// RunPrepared leaves ownership of the worktree lock with the caller, allowing
// history selection, execution and durable completion to share one lock.
func (generator Generator) RunPrepared(ctx context.Context, options GenerateOptions, state *RepositoryState) (GenerateResult, error) {
	if state == nil || state.lock == nil || generator.Codex == nil || generator.Repository.Commands == nil {
		return GenerateResult{}, errors.New("prepared generation requires a locked repository and command runners")
	}
	target, err := canonicalDirectory(options.Repository)
	if err != nil || target != state.Target {
		return GenerateResult{}, errors.New("prepared generation target differs from locked repository")
	}
	prompt, err := implementationPrompt(options.Request)
	if err != nil {
		return GenerateResult{}, err
	}
	return generator.runPrepared(ctx, options, state, prompt)
}

func (generator Generator) runPrepared(ctx context.Context, options GenerateOptions, state *RepositoryState, prompt []byte) (result GenerateResult, returnErr error) {
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

	promptDigest := sha256.Sum256(prompt)
	result.ImplementationPromptSHA256 = fmt.Sprintf("%x", promptDigest)
	if options.BeforeExecute != nil {
		if err := options.BeforeExecute(); err != nil {
			return result, fmt.Errorf("prepare generation history: %w", err)
		}
	}
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

func implementationPrompt(request []byte) ([]byte, error) {
	// The CLI validates the canonical request. Read the mode from those exact
	// bytes so an independently supplied flag cannot select a different scope.
	var envelope struct {
		RequestedChange struct {
			Kind string `json:"kind"`
		} `json:"requestedChange"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		return nil, fmt.Errorf("read generation mode: %w", err)
	}
	kind := envelope.RequestedChange.Kind
	if kind != "full" && kind != "incremental" {
		return nil, fmt.Errorf("unsupported generation mode %q", kind)
	}
	var prompt strings.Builder
	if kind == "incremental" {
		prompt.WriteString(`Apply only the incremental update described by requestedChange in the authoritative Forma Generation Request below.

Incremental scope:
- The complete current Intent, Acceptance Facts, Review Requirements, and Implementation Policy remain constraints. Limit edits to the intentChanges, factChanges, reviewRequirementChanges, policyChanges, and conventionChanges indicated by requestedChange.
- conventionChanges explicitly lists added/removed advisory text; an edit is a removal plus an addition. A removed convention only withdraws that advice. It does not require the opposite behavior or authorize code deletion, refactoring, or migration. Apply added advice only within the requested delta; a convention-only update may finish with zero diff.
- Preserve unchanged Intent, existing implementation, and hand-written code. Retain all Acceptance Facts, including unchanged Facts, for regression verification; do not weaken expectations or remove coverage. Use existing tests for unchanged behavior; unrelated missing coverage is a separate finding, not permission to broaden the update.
- Inspect whether the repository already satisfies the changed requirements. If it does, make no edits; a zero-diff completion is valid. Explain the evidence and any unverified checks without manufacturing a change.
- Make only changes necessary to implement the requested delta, including directly affected dependencies, build/CI configuration, test commands, and documentation. Do not perform unrelated refactoring, documentation updates, or test rewrites.
- This is an update, not a repair or audit. Report unrelated pre-existing failures or review findings separately without fixing them. If they block the update, report the blocker instead of broadening scope.
- Do not infer unsupported removals, renames, deletions, or migrations. Do not edit the baseline request, Implementation Manifest, or Forma source to make the implementation fit.

`)
	} else {
		prompt.WriteString(`Implement the application described by the authoritative Forma Generation Request below in the current repository.

Implementation scope:
- Implement every requested intent node and Acceptance Fact in ordinary application code appropriate for this repository.
- Implement and test each Acceptance Fact at its named subject boundary.

`)
	}
	prompt.WriteString(`Rules:
- Treat the request as structured application intent, not as repository-specific implementation instructions.
- Verify each Acceptance Fact at its named subject boundary. For an access Fact with expected.enforcement=authoritative, the application's public boundary that presents or invokes the subject must enforce it; a UI visibility check or direct call to a pure role helper is insufficient.
- An anonymous principal means no authenticated identity and no roles. Never turn missing identity, session, or role state into an allowed default role.
- Preserve existing repository conventions and do not weaken or delete existing tests to make the task appear complete.
- Do not modify .forma source files or invent requirements that are absent from the request.
- Do not modify the Implementation Manifest or any baseline request.
- Do not commit, switch branches, reset Git state, or modify Git metadata or Forma generation history.
- Human Review Requirements are not machine-verified; make the relevant implementation visible for later human review.
- You may inspect files and run relevant non-destructive build or test commands inside your workspace sandbox.
- Do not create Generation Feedback. Forma stops after your repository edits so a person can review the diff and explicitly run commands.
- Distinguish implementation completion from verification: report each build/test check as passed, failed, or not run (including skipped assertions and the reason). Never describe unrun or skipped checks as verification success.
- Finish with a concise summary of changed files, validation you ran, and remaining human review items.

BEGIN FORMA GENERATION REQUEST JSON
`)
	prompt.Write(bytes.TrimSpace(request))
	prompt.WriteString("\nEND FORMA GENERATION REQUEST JSON\n")
	return []byte(prompt.String()), nil
}
