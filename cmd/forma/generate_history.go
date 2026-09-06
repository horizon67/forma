package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/generationhistory"
)

func runManagedGeneration(options generateCommandOptions, result compiler.Result, selectors, paths []string, stdout, stderr io.Writer) (exit int) {
	fail := func(err error) int {
		fmt.Fprintf(stderr, "forma: %v\n", err)
		if generationSetupError(err) {
			return 2
		}
		return 1
	}
	previous, manifest, err := loadGenerationInputs(generationRequestOptions{previousPath: options.previousPath, manifestPath: options.manifestPath})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "forma: %v\n", err)
			return 2
		}
		return fail(err)
	}
	// Validate explicit inputs before Git/Codex lookup. Automatic baseline
	// selection is delayed until the containing worktree lock is held.
	plan, err := agentrequest.PlanGeneration(previous, result, manifest)
	if err != nil {
		return fail(err)
	}
	explicitPrevious := previous
	ctx, cancel := context.WithTimeout(context.Background(), alphaGenerationTimeout)
	defer cancel()
	git, err := absoluteExecutable("git")
	if err != nil {
		return fail(fmt.Errorf("%w: %v", errGitUnavailable, err))
	}
	preflight := agentrunner.RepositoryPreflight{Commands: agentrunner.OSCommandRunner{}, Locks: agentrunner.WorktreeLocker{}}
	state, err := preflight.Prepare(ctx, agentrunner.RepositoryPreflightOptions{Repository: options.repository, GitExecutable: git, AllowDirty: options.allowDirty})
	if err != nil {
		return fail(err)
	}
	defer func() {
		if err := state.Close(); err != nil {
			fmt.Fprintf(stderr, "forma: release Git worktree lock: %v\n", err)
			exit = 1
		}
	}()
	location, err := preflight.HistoryContext(ctx, state, git)
	if err != nil {
		return fail(err)
	}
	if location.Head != state.Head {
		return fail(errors.New("Git HEAD changed during history preflight"))
	}
	identity, err := generationhistory.NewIdentity(state.Worktree, state.Target, location.Branch, selectors)
	if err != nil {
		return fail(err)
	}
	historyDirectory := filepath.Join(location.GitDirectory, "forma")
	historyPath := filepath.Join(historyDirectory, "generation-history.json")
	fmt.Fprintf(stdout, "generation history: %s\n", historyPath)
	fmt.Fprintf(stdout, "generation history key: %s\n", identity.Key())
	history, err := generationhistory.Open(location.GitDirectory, state.Worktree)
	if err != nil {
		return fail(fmt.Errorf("generation history unavailable: %w; restore known-good metadata, or after review back up and move aside %s and %s (if present), then import a known reviewed --previous request; --previous alone cannot read invalid history; moving the catalog aside affects all applications in this worktree (see docs/generation-history.md)", err, historyPath, filepath.Join(historyDirectory, "generation-pending")))
	}
	record := history.Record(identity)
	if history.PendingKey != "" && history.PendingKey != identity.Key() {
		return fail(fmt.Errorf("another application or branch has an unfinished generation; recover it using its original source selectors and target first (pending key %s; history %s)", history.PendingKey, history.Path))
	}
	if previous == nil {
		if history.PendingKey != "" || record.Pending {
			return fail(errors.New("previous generation failed, was interrupted, or its history save did not finish; inspect the printed history record and partial repository edits, then recover with a known reviewed --previous request (see docs/generation-history.md); no automatic retry, no-op, repair, or full regeneration was performed"))
		}
		if record.Baseline != nil {
			baseline, err := record.Baseline.Decode()
			if err != nil {
				return fail(err)
			}
			ancestor, err := preflight.HistoryAncestor(ctx, state, git, record.Baseline.Head)
			if err != nil {
				return fail(err)
			}
			if !ancestor {
				return fail(errors.New("history HEAD is not an ancestor of this checkout; inspect the branch/reset and the printed history record, then explicitly supply a baseline confirmed to match this checkout via --previous to re-bind (see docs/generation-history.md)"))
			}
			previous = &baseline
		} else {
			if existing := history.TargetIdentities(state.Target); len(existing) != 0 {
				for _, other := range existing {
					fmt.Fprintf(stdout, "existing history key: %s (branch %q; source selectors %q)\n", other.Key(), other.Branch, other.Sources)
				}
				return fail(errors.New("target has history under different source selectors or branch; the current key has no baseline; inspect the existing records and explicitly supply a known reviewed --previous request to establish this application identity (see docs/generation-history.md)"))
			}
			if err := freshGenerationTarget(state.Target, paths, options.manifestPath); err != nil {
				return fail(err)
			}
		}
		plan, err = agentrequest.PlanGeneration(previous, result, manifest)
		if err != nil {
			return fail(err)
		}
	} else {
		fmt.Fprintln(stdout, "using explicit --previous instead of automatic history; this is a user-supplied baseline assertion, not proof of repository correctness")
		if history.PendingKey != "" || record.Pending {
			fmt.Fprintln(stdout, "explicit recovery overrides an unfinished attempt; its failure/interruption is not verification success")
		}
	}
	fmt.Fprintln(stdout, "history verification: unverified; human review: pending (tracked separately from execution completion)")
	if plan.NoOp {
		if options.previousPath != "" {
			fmt.Fprintln(stdout, "explicit no-op adoption: you are asserting that the target already implements the supplied Request; no application code was generated or verified; this is not initial generation")
			if err := history.Import(identity, *previous, state.Head); err != nil {
				return fail(fmt.Errorf("import baseline history: %w", err))
			}
			fmt.Fprintln(stdout, "explicit baseline imported into local history; no agent execution was recorded")
		}
		return printNoOpGeneration(state, plan.Baseline, stdout, stderr)
	}
	fmt.Fprintf(stdout, "generation request: %s\n", plan.Request.RequestedChange.Kind)
	if plan.Baseline != nil {
		fmt.Fprintf(stdout, "baseline request SHA-256: %s\n", plan.Baseline.RequestSHA256)
	}
	content, err := agentrequest.Marshal(plan.Request)
	if err != nil {
		return fail(err)
	}
	begun := false
	invocation := generateInvocation{Repository: state.Target, AllowDirty: options.allowDirty, Request: content,
		ReviewRequirements: plan.Request.ReviewRequirements, State: state, GitExecutable: git,
		BeforeExecute: func() error {
			if begun {
				return errors.New("generation candidate was already recorded")
			}
			if err := history.Begin(identity, plan.Request, explicitPrevious, state.Head); err != nil {
				return err
			}
			begun = true
			return nil
		},
	}
	generated, runErr := runGenerationAttempt(ctx, invocation, stdout, stderr)
	if !begun {
		if runErr != nil {
			return fail(runErr)
		}
		return fail(errors.New("generation returned without recording its candidate; no baseline was promoted"))
	}
	if runErr == nil && (!generated.FinalStatusKnown || generated.ImplementationPromptSHA256 == "" || generated.Target != state.Target || generated.Worktree != state.Worktree || generated.InitialHead != state.Head) {
		runErr = errors.New("generation did not return complete matching repository evidence")
	}
	// Re-check branch and HEAD even on cancellation using a bounded cleanup
	// context. An agent checkout/commit cannot silently promote a stale baseline.
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer checkCancel()
	after, err := preflight.HistoryContext(checkCtx, state, git)
	if err != nil {
		runErr = errors.Join(runErr, err)
	} else if after != location {
		runErr = errors.Join(runErr, errors.New("Git identity changed during generation; history was not promoted"))
	}
	if err := history.Finish(identity, runErr == nil, generated.ImplementationPromptSHA256); err != nil {
		return fail(errors.Join(runErr, fmt.Errorf("save generation result: %w; history remains pending, inspect the repository before explicit recovery", err)))
	}
	if runErr != nil {
		return fail(runErr)
	}
	fmt.Fprintln(stdout, "comparison baseline saved from the exact completed agent input; tests remain unverified and human review remains pending")
	return 0
}

// A missing history is not evidence that an existing implementation is new.
// Permit only explicit input files and narrowly named repository scaffolding,
// even when Git ignores a file. Do not treat arbitrary clean/tracked code as new.
func freshGenerationTarget(target string, sources []string, manifest string) error {
	allowed := map[string]bool{}
	for _, name := range append(append([]string(nil), sources...), manifest) {
		if name == "" {
			continue
		}
		p, err := filepath.Abs(name)
		if err != nil {
			return err
		}
		p, err = filepath.EvalSymlinks(p)
		if err != nil {
			return err
		}
		allowed[p] = true
	}
	return filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == target {
			return nil
		}
		rel, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type().IsRegular() && (allowed[path] || generationScaffolding(rel)) {
			return nil
		}
		return fmt.Errorf("no generation history for this application, but target contains %s; refusing full regeneration of an existing target; for initial generation, use an empty target directory in a Git worktree with a commit; for adoption only, supply --previous with a Request confirmed to match the existing implementation; do not manufacture a baseline from today's specification to bypass this check (that can import a no-op without generating code)", rel)
	})
}

func generationScaffolding(relative string) bool {
	switch relative {
	case "README.md", ".gitignore", ".gitattributes", ".editorconfig", "LICENSE", "LICENSE.md", "LICENSE.txt", "CODEOWNERS", "Makefile", filepath.Join(".github", "CODEOWNERS"):
		return true
	}
	// A workflow configuration is scaffolding, but scripts/actions elsewhere
	// under .github are not implicitly treated as an unimplemented application.
	return filepath.Dir(relative) == filepath.Join(".github", "workflows") &&
		(filepath.Ext(relative) == ".yml" || filepath.Ext(relative) == ".yaml")
}
