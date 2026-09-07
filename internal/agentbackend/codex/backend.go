// Package codex is the only interpreter of Codex's authentication, prompt,
// command-line, and output protocols. It never writes to a terminal.
package codex

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationprogress"
)

const summaryLimit = 1 << 20

type Backend struct {
	Commands    agentrunner.CommandRunner
	LookPath    func(string) (string, error)
	Environment []string
}

func (b Backend) Prepare(ctx context.Context, input agentrunner.Input) (agentrunner.Prepared, error) {
	prompt, err := implementationPrompt(input.Request)
	if err != nil {
		return nil, err
	}
	lookup := b.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	executable, err := lookup("codex")
	if err != nil {
		return nil, fmt.Errorf("%w: install Codex CLI and run `codex login`", agentrunner.ErrBackendUnavailable)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, agentrunner.ErrBackendUnavailable
	}
	runner := b.Commands
	if runner == nil {
		runner = agentrunner.OSCommandRunner{ProcessGroup: true}
	}
	environment := environmentAllowlist(b.Environment)
	auth, err := runner.Run(ctx, agentrunner.Command{Executable: executable, Arguments: []string{"login", "status"}, Directory: input.Repository, Environment: environment})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || auth.ExitCode != 0 || auth.StdoutTruncated || auth.StderrTruncated || auth.WaitDelayed || auth.CleanupFailed {
		// Authentication output can contain credentials. Never attach raw output
		// or a command runner's error to a public diagnostic.
		return nil, fmt.Errorf("%w: check Codex CLI and run `codex login`", agentrunner.ErrAuthentication)
	}
	// Reject a target-local TMPDIR before creating anything, not after a
	// transient write in the application tree during preparation.
	base, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return nil, errors.New("cannot locate private agent summary storage")
	}
	baseRelative, err := filepath.Rel(input.Repository, base)
	if err != nil || (baseRelative != ".." && !strings.HasPrefix(baseRelative, ".."+string(filepath.Separator))) {
		return nil, errors.New("agent summary storage must be outside the target; choose an external TMPDIR")
	}
	dir, err := os.MkdirTemp(base, "forma-agent-summary-")
	if err != nil {
		return nil, errors.New("cannot create private agent summary directory")
	}
	canonical, err := filepath.EvalSymlinks(dir)
	rel, relErr := filepath.Rel(input.Repository, canonical)
	if err != nil || relErr != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		_ = os.RemoveAll(dir)
		return nil, errors.New("agent summary directory must be outside the target")
	}
	path := filepath.Join(dir, "summary.txt")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, errors.New("cannot prepare private agent summary")
	}
	return &prepared{runner: runner, executable: executable, repository: input.Repository, environment: environment, prompt: prompt, directory: dir, summary: path}, nil
}

type prepared struct {
	runner                                     agentrunner.CommandRunner
	executable, repository, directory, summary string
	environment                                []string
	prompt                                     []byte
	mu                                         sync.Mutex
	used, closed                               bool
}

func (p *prepared) InputSHA256() string { return fmt.Sprintf("%x", sha256.Sum256(p.prompt)) }
func (p *prepared) Run(ctx context.Context, notify agentrunner.Notify) (agentrunner.ExecutionResult, error) {
	p.mu.Lock()
	if p.used || p.closed {
		p.mu.Unlock()
		return agentrunner.ExecutionResult{}, errors.New("prepared agent input is single-use")
	}
	p.used = true
	p.mu.Unlock()
	if notify == nil {
		notify = func(agentrunner.Notification) {}
	}
	stream := newEventStream(notify)
	command, err := p.runner.Run(ctx, agentrunner.Command{
		Executable: p.executable,
		Arguments:  []string{"exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--sandbox", "workspace-write", "--color", "never", "--json", "--output-last-message", p.summary, "-C", p.repository, "-"},
		Directory:  p.repository, Environment: append([]string(nil), p.environment...), Stdin: append([]byte(nil), p.prompt...),
		StdoutSink: stream, DiscardStderr: true,
		OnStarted: func() { notify(agentrunner.Notification{Started: true}) },
	})
	stream.finish()
	result := agentrunner.ExecutionResult{Status: generationprogress.Failed, CleanupComplete: !command.CleanupFailed && !command.WaitDelayed}
	// Only the separate file is authoritative for the final free-form message.
	summary, summaryErr := readSummary(p.summary)
	if summaryErr == nil {
		result.Summary = summary
	}
	if ctx.Err() != nil {
		result.Status = generationprogress.Outcome(ctx.Err())
		return result, ctx.Err()
	}
	if command.TimedOut || errors.Is(err, context.DeadlineExceeded) {
		result.Status = generationprogress.TimedOut
		return result, context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		result.Status = generationprogress.Cancelled
		return result, context.Canceled
	}
	if err != nil || command.ExitCode != 0 || command.WaitDelayed || command.CleanupFailed || command.StdoutTruncated || command.StderrTruncated || stream.failed {
		// Only fixed observations and numeric status, never runner error text
		// or raw streams. Do not guess rate-limit/sandbox/model causes.
		var reasons []string
		if command.ExitCode != 0 {
			reasons = append(reasons, fmt.Sprintf("agent CLI exited with %d", command.ExitCode))
		}
		if stream.failed {
			reasons = append(reasons, "agent reported a failed turn")
		}
		if command.CleanupFailed {
			reasons = append(reasons, "agent process cleanup failed")
		}
		if command.WaitDelayed {
			reasons = append(reasons, "agent output pipes did not close cleanly")
		}
		if command.StdoutTruncated || command.StderrTruncated {
			reasons = append(reasons, "agent output was truncated")
		}
		if err != nil {
			reasons = append(reasons, "agent process could not be started or observed cleanly")
		}
		return result, fmt.Errorf("%w: %s", agentrunner.ErrAgentFailed, strings.Join(reasons, "; "))
	}
	if summaryErr != nil {
		return result, errors.New("agent final summary is missing, unsafe, or exceeds 1 MiB")
	}
	result.Status = generationprogress.Completed
	return result, nil
}
func (p *prepared) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return os.RemoveAll(p.directory)
}

func readSummary(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > summaryLimit {
		return nil, errors.New("unsafe summary file")
	}
	f, err := openSummary(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("summary file changed during open")
	}
	content, err := io.ReadAll(io.LimitReader(f, summaryLimit+1))
	if err != nil {
		return nil, err
	}
	if len(content) > summaryLimit || len(strings.TrimSpace(string(content))) == 0 {
		return nil, errors.New("missing or oversized summary")
	}
	return content, nil
}

func environmentAllowlist(environ []string) []string {
	values := map[string]string{}
	for _, entry := range environ {
		if name, value, found := strings.Cut(entry, "="); found {
			values[name] = value
		}
	}
	allowed := []string{"PATH", "HOME", "CODEX_HOME", "TMPDIR", "TMP", "TEMP", "USER", "LOGNAME", "SHELL", "TERM", "COLORTERM", "LANG", "LC_ALL", "LC_CTYPE", "NO_COLOR", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy", "CODEX_CA_CERTIFICATE", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS"}
	result := make([]string, 0, len(allowed))
	for _, name := range allowed {
		if value, ok := values[name]; ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}
