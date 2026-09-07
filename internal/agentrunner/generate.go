package agentrunner

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/horizon67/forma/internal/generationprogress"
)

var (
	ErrAuthentication     = errors.New("agent authentication is unavailable")
	ErrBackendUnavailable = errors.New("agent backend is unavailable")
	ErrAgentFailed        = errors.New("agent generation failed")
)

// Backend prepares immutable input without editing the target. Neither this
// boundary nor Prepared requires a process, command line, or output stream.
type Backend interface {
	Prepare(context.Context, Input) (Prepared, error)
}
type Input struct {
	Repository string
	Request    []byte
}

// Prepared owns private resources until Close. Run must use exactly the input
// identified by InputSHA256, without rebuilding it from mutable files.
type Prepared interface {
	InputSHA256() string
	Run(context.Context, Notify) (ExecutionResult, error)
	Close() error
}
type Notification struct {
	Started    bool
	Activity   generationprogress.Activity
	Diagnostic generationprogress.Code
}

// Notify returns promptly. Notifications are optional, not completion evidence.
type Notify func(Notification)
type ExecutionResult struct {
	Summary         []byte // Untrusted AI text, never a progress field.
	Status          generationprogress.Status
	CleanupComplete bool
}
type GenerateOptions struct {
	Repository    string
	GitExecutable string
	AllowDirty    bool
	Request       []byte
	BeforeExecute func() error
	Progress      *generationprogress.Reporter
}
type GenerateResult struct {
	Target                     string
	Worktree                   string
	InitialHead                string
	InitialDirty               bool
	FinalStatusKnown           bool
	FinalStatus                string
	ImplementationPromptSHA256 string
	Summary                    []byte
	Status                     generationprogress.Status
	CleanupComplete            bool
}
type Generator struct {
	Repository RepositoryPreflight
	Backend    Backend
}

func validateGenerationInput(request []byte) error {
	if len(bytes.TrimSpace(request)) == 0 {
		return errors.New("generation requires canonical Generation Request JSON")
	}
	var envelope struct {
		RequestedChange struct {
			Kind string `json:"kind"`
		} `json:"requestedChange"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		return errors.New("invalid Generation Request JSON")
	}
	if envelope.RequestedChange.Kind != "full" && envelope.RequestedChange.Kind != "incremental" {
		return errors.New("unsupported generation mode")
	}
	return nil
}

func (g Generator) Run(ctx context.Context, options GenerateOptions) (result GenerateResult, returnErr error) {
	defer func() {
		if returnErr != nil {
			result.Status = generationprogress.Outcome(returnErr)
		}
	}()
	if err := validateGenerationInput(options.Request); err != nil {
		return result, err
	}
	state, err := g.Repository.Prepare(ctx, RepositoryPreflightOptions{Repository: options.Repository, GitExecutable: options.GitExecutable, AllowDirty: options.AllowDirty})
	if err != nil {
		return result, err
	}
	defer func() {
		if err := state.Close(); err != nil {
			returnErr = errors.Join(returnErr, err)
			result.Status = generationprogress.Failed
			result.CleanupComplete = false
		}
	}()
	return g.RunPrepared(ctx, options, state)
}

// RunPrepared leaves the lock with its caller for durable history finalization.
func (g Generator) RunPrepared(ctx context.Context, options GenerateOptions, state *RepositoryState) (result GenerateResult, returnErr error) {
	defer func() {
		if returnErr != nil {
			result.Status = generationprogress.Outcome(returnErr)
		}
	}()
	if state == nil || state.lock == nil || g.Backend == nil || g.Repository.Commands == nil {
		return result, errors.New("prepared generation requires a locked repository and backend")
	}
	if err := validateGenerationInput(options.Request); err != nil {
		return result, err
	}
	target, err := canonicalDirectory(options.Repository)
	if err != nil || target != state.Target {
		return result, errors.New("prepared generation target differs from locked repository")
	}
	result = GenerateResult{Target: state.Target, Worktree: state.Worktree, InitialHead: state.Head, InitialDirty: state.Dirty, Status: generationprogress.Failed}
	p := options.Progress
	p.Phase(generationprogress.AgentPreparation)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	prepared, err := g.Backend.Prepare(ctx, Input{Repository: target, Request: append([]byte(nil), options.Request...)})
	if err != nil {
		return result, err
	}
	if prepared == nil {
		return result, errors.New("backend returned no prepared input")
	}
	closed := false
	closePrepared := func() error {
		if closed {
			return nil
		}
		closed = true
		return prepared.Close()
	}
	defer func() {
		if err := closePrepared(); err != nil {
			returnErr = errors.Join(returnErr, errors.New("agent private resource cleanup failed"))
			result.CleanupComplete = false
		}
		result.Status = generationprogress.Outcome(returnErr)
	}()
	result.ImplementationPromptSHA256 = prepared.InputSHA256()
	digest, err := hex.DecodeString(result.ImplementationPromptSHA256)
	if err != nil || len(digest) != 32 || fmt.Sprintf("%x", digest) != result.ImplementationPromptSHA256 {
		return result, errors.New("backend returned an invalid input SHA-256")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if options.BeforeExecute != nil {
		if err := options.BeforeExecute(); err != nil {
			return result, fmt.Errorf("prepare generation history: %w", err)
		}
	}
	p.Phase(generationprogress.AgentStarting)
	// Without a Started notification we do not invent evidence of liveness.
	execution, runErr := prepared.Run(ctx, func(n Notification) {
		if n.Started {
			p.AgentStarted()
		}
		if n.Activity != "" {
			p.Activity(n.Activity)
		}
		if n.Diagnostic != "" {
			p.Diagnostic(n.Diagnostic)
		}
	})
	p.Phase(generationprogress.Cleanup)
	result.Summary = append([]byte(nil), execution.Summary...)
	result.CleanupComplete = execution.CleanupComplete
	if err := closePrepared(); err != nil {
		runErr = errors.Join(runErr, errors.New("agent private resource cleanup failed"))
		result.CleanupComplete = false
	}
	if !result.CleanupComplete {
		runErr = errors.Join(runErr, errors.New("agent cleanup did not complete"))
	}
	if execution.Status != generationprogress.Completed && runErr == nil {
		switch execution.Status {
		case generationprogress.Cancelled:
			runErr = context.Canceled
		case generationprogress.TimedOut:
			runErr = context.DeadlineExceeded
		default:
			runErr = ErrAgentFailed
		}
	}
	if ctx.Err() != nil {
		runErr = errors.Join(runErr, ctx.Err())
	}
	p.Phase(generationprogress.FinalStatus)
	statusCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, statusErr := g.Repository.runGit(statusCtx, RepositoryPreflightOptions{GitExecutable: options.GitExecutable}, state.Worktree, "status", "--porcelain=v1", "--untracked-files=normal")
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
	returnErr = errors.Join(runErr, statusErr)
	result.Status = generationprogress.Outcome(returnErr)
	return result, returnErr
}
