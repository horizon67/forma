package codex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationprogress"
)

type commandFunc func(context.Context, agentrunner.Command) (agentrunner.CommandResult, error)

func (f commandFunc) Run(ctx context.Context, c agentrunner.Command) (agentrunner.CommandResult, error) {
	return f(ctx, c)
}

func TestCodexImmutableInputPrivateSummaryAndSafeNotifications(t *testing.T) {
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"requestedChange":{"kind":"full"}}`)
	original := append([]byte(nil), request...)
	var commands []agentrunner.Command
	var summaryPath string
	backend := Backend{LookPath: func(name string) (string, error) {
		if name != "codex" {
			t.Fatal(name)
		}
		return "/absolute/codex", nil
	}, Environment: []string{"HOME=/trusted/home", "OPENAI_API_KEY=SECRET"}, Commands: commandFunc(func(_ context.Context, c agentrunner.Command) (agentrunner.CommandResult, error) {
		commands = append(commands, c)
		if len(commands) == 1 {
			if !reflect.DeepEqual(c.Arguments, []string{"login", "status"}) {
				t.Fatal(c.Arguments)
			}
			return agentrunner.CommandResult{}, nil
		}
		for i, arg := range c.Arguments {
			if arg == "--output-last-message" {
				summaryPath = c.Arguments[i+1]
			}
		}
		if summaryPath == "" || strings.HasPrefix(summaryPath, target+string(filepath.Separator)) {
			t.Fatal("summary is not outside target")
		}
		info, err := os.Stat(filepath.Dir(summaryPath))
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("directory privacy: %v %v", info, err)
		}
		file, err := os.Stat(summaryPath)
		if err != nil || file.Mode().Perm() != 0600 {
			t.Fatal("summary privacy")
		}
		if !reflect.DeepEqual(c.Environment, []string{"HOME=/trusted/home"}) || !c.DiscardStderr || c.StdoutSink == nil {
			t.Fatalf("unsafe invocation: %#v", c)
		}
		c.OnStarted()
		_, _ = c.StdoutSink.Write([]byte(`{"type":"item.started","item":{"type":"command_execution","command":"SECRET"}}` + "\n"))
		_, _ = c.StdoutSink.Write([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"SECRET NOT THE SUMMARY"}}` + "\n"))
		if err := os.WriteFile(summaryPath, []byte("Final file summary."), 0600); err != nil {
			t.Fatal(err)
		}
		return agentrunner.CommandResult{}, nil
	})}
	p, err := backend.Prepare(context.Background(), agentrunner.Input{Repository: target, Request: request})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	request[0] = 'X' // Caller changes cannot alter prepared stdin or its digest.
	var notifications []agentrunner.Notification
	result, err := p.Run(context.Background(), func(n agentrunner.Notification) { notifications = append(notifications, n) })
	if err != nil || result.Status != generationprogress.Completed || !result.CleanupComplete || string(result.Summary) != "Final file summary." {
		t.Fatalf("result: %#v %v", result, err)
	}
	if len(commands) != 2 || len(notifications) != 3 || !notifications[0].Started || notifications[1].Activity != generationprogress.Command {
		t.Fatalf("protocol: %#v", notifications)
	}
	wantArgs := []string{"exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--sandbox", "workspace-write", "--color", "never", "--json", "--output-last-message", summaryPath, "-C", target, "-"}
	if !reflect.DeepEqual(commands[1].Arguments, wantArgs) || !bytes.Contains(commands[1].Stdin, original) || p.InputSHA256() != fmt.Sprintf("%x", sha256.Sum256(commands[1].Stdin)) {
		t.Fatal("immutable invocation changed")
	}
	if _, err := p.Run(context.Background(), nil); err == nil {
		t.Fatal("prepared input executed twice")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(summaryPath)); !os.IsNotExist(err) {
		t.Fatal("private directory retained")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("adapter wrote target inputs")
	}
}

func TestCodexAuthenticationNeverPublishesRawOutputOrErrors(t *testing.T) {
	for _, mode := range []string{"exit", "error", "truncated", "cleanup"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			b := Backend{LookPath: func(string) (string, error) { return "/absolute/codex", nil }, Commands: commandFunc(func(context.Context, agentrunner.Command) (agentrunner.CommandResult, error) {
				calls++
				r := agentrunner.CommandResult{Stdout: []byte("SECRET"), Stderr: []byte("SECRET")}
				switch mode {
				case "exit":
					r.ExitCode = 1
				case "error":
					return r, errors.New("SECRET")
				case "truncated":
					r.StdoutTruncated = true
				case "cleanup":
					r.CleanupFailed = true
				}
				return r, nil
			})}
			p, err := b.Prepare(context.Background(), agentrunner.Input{Repository: t.TempDir(), Request: []byte(`{"requestedChange":{"kind":"full"}}`)})
			if p != nil || !errors.Is(err, agentrunner.ErrAuthentication) || strings.Contains(err.Error(), "SECRET") || calls != 1 {
				t.Fatalf("unsafe auth failure %v", err)
			}
		})
	}
}

func TestTargetLocalTemporaryDirectoryIsRejectedBeforeCreation(t *testing.T) {
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", target)
	b := Backend{LookPath: func(string) (string, error) { return "/absolute/codex", nil }, Commands: commandFunc(func(context.Context, agentrunner.Command) (agentrunner.CommandResult, error) {
		return agentrunner.CommandResult{}, nil
	})}
	p, err := b.Prepare(context.Background(), agentrunner.Input{Repository: target, Request: []byte(`{"requestedChange":{"kind":"full"}}`)})
	if p != nil || err == nil {
		if p != nil {
			_ = p.Close()
		}
		t.Fatal("target-local summary accepted")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("preparation wrote the target")
	}
}

func TestSummaryRejectsMissingSymlinkDirectoryEmptyAndOversize(t *testing.T) {
	for _, kind := range []string{"missing", "symlink", "directory", "empty", "oversize", "regular"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary")
			switch kind {
			case "symlink":
				target := filepath.Join(t.TempDir(), "secret")
				if err := os.WriteFile(target, []byte("SECRET"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "empty", "oversize", "regular":
				content := []byte{}
				if kind == "oversize" {
					content = bytes.Repeat([]byte("x"), summaryLimit+1)
				}
				if kind == "regular" {
					content = []byte("ok")
				}
				if err := os.WriteFile(path, content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			content, err := readSummary(path)
			if (err == nil) != (kind == "regular") || bytes.Contains(content, []byte("SECRET")) {
				t.Fatalf("unsafe %s: %q %v", kind, content, err)
			}
		})
	}
}

func TestCodexExecutionFailsClosedWithoutFinalEvidence(t *testing.T) {
	for _, mode := range []string{"missing summary", "exit", "raw error", "cleanup", "pipes", "cancel", "timeout", "turn failed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := Backend{LookPath: func(string) (string, error) { return "/absolute/codex", nil }, Commands: commandFunc(func(_ context.Context, c agentrunner.Command) (agentrunner.CommandResult, error) {
				if c.Arguments[0] == "login" {
					return agentrunner.CommandResult{}, nil
				}
				for i, arg := range c.Arguments {
					if arg == "--output-last-message" && mode != "missing summary" {
						if err := os.WriteFile(c.Arguments[i+1], []byte("final"), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				r := agentrunner.CommandResult{}
				switch mode {
				case "exit":
					r.ExitCode = 7
				case "raw error":
					return r, errors.New("SECRET")
				case "cleanup":
					r.CleanupFailed = true
				case "pipes":
					r.WaitDelayed = true
				case "cancel":
					cancel()
					return r, ctx.Err()
				case "timeout":
					r.TimedOut = true
					return r, context.DeadlineExceeded
				case "turn failed":
					_, _ = c.StdoutSink.Write([]byte(`{"type":"turn.failed","error":{"message":"SECRET"}}` + "\n"))
				}
				return r, nil
			})}
			p, err := b.Prepare(ctx, agentrunner.Input{Repository: t.TempDir(), Request: []byte(`{"requestedChange":{"kind":"full"}}`)})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			r, err := p.Run(ctx, nil)
			if err == nil || r.Status == generationprogress.Completed || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("unsafe result %#v %v", r, err)
			}
			want := map[string]string{"exit": "agent CLI exited with 7", "raw error": "could not be started or observed cleanly", "cleanup": "agent process cleanup failed", "pipes": "agent output pipes did not close cleanly", "turn failed": "agent reported a failed turn"}[mode]
			if want != "" && (!errors.Is(err, agentrunner.ErrAgentFailed) || !strings.Contains(err.Error(), want)) {
				t.Fatalf("safe failure detail missing: %v", err)
			}
		})
	}
}

func TestPromptsPreserveFullAndIncrementalContracts(t *testing.T) {
	for _, kind := range []string{"full", "incremental"} {
		request := []byte(`{"requestedChange":{"kind":"` + kind + `"}}`)
		content, err := implementationPrompt(request)
		if err != nil {
			t.Fatal(err)
		}
		prompt := string(content)
		required := []string{"Do not create Generation Feedback", "Do not modify .forma source files", "expected.enforcement=authoritative", "the application's public boundary that presents or invokes the subject must enforce it", "An anonymous principal means no authenticated identity and no roles", "Never turn missing identity, session, or role state into an allowed default role", "Never describe unrun or skipped checks as verification success", "BEGIN FORMA GENERATION REQUEST JSON\n" + string(request) + "\nEND FORMA GENERATION REQUEST JSON"}
		if kind == "full" {
			required = append(required, "Implementation scope:", "Implement and test each Acceptance Fact at its named subject boundary.")
		} else {
			required = append(required, "Apply only the incremental update", "policyChanges", "conventionChanges", "explicitly lists added/removed advisory text", "A removed convention only withdraws that advice", "does not require the opposite behavior or authorize code deletion", "including unchanged Facts", "zero-diff completion is valid", "Do not perform unrelated", "without fixing them")
			if strings.Contains(prompt, "Implement every requested intent node") {
				t.Fatal("full scope leaked")
			}
		}
		for _, s := range required {
			if !strings.Contains(prompt, s) {
				t.Fatalf("%s prompt missing %q", kind, s)
			}
		}
	}
}
