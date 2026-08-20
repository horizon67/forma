// Command orchestrate runs the controlled membership repair loop.
//
// The command is experimental orchestration, not part of the Forma language or
// its interchange formats. It owns the process boundary that the repair
// integrity probe left to a trusted caller:
//
//  1. before the retry starts, build the guard, feedback generator, and Forma
//     verifier into a private directory outside the repository;
//  2. take the immutable retry baseline with the prebuilt feedback binary;
//  3. measure the current failure;
//  4. start a fresh repair process for each bounded attempt;
//  5. run the prebuilt guard before the prebuilt feedback generator; and
//  6. accept success only when the prebuilt Forma verifier accepts it.
//
// Prebuilding all three tools matters. A prebuilt guard can reject changes to
// the measurement inputs, but running an agent-editable feedback generator or
// verifier after the guard would reintroduce a same-process bypass through any
// package those commands compile. The orchestrator never executes their source
// after the retry begins.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/horizon67/forma/internal/agentrequest"
)

const (
	experimentRoot             = "experiments/membership-agent-e2e"
	feedbackPath               = experimentRoot + "/target/generation-feedback.json"
	requestPath                = experimentRoot + "/generation-request.json"
	targetPath                 = experimentRoot + "/target"
	baselinePath               = "internal/agentrequest/testdata/admin.incremental.request.json"
	repairDecisionSchema       = "forma-experiment/repair-decision/v0alpha1"
	maxDecisionBytes           = 1 << 20
	maxDecisionSummaryBytes    = 1000
	maxDecisionEntries         = 50
	maxDecisionIDBytes         = 500
	maxDecisionDiagnostics     = 20
	maxDecisionDiagnosticBytes = 2000
)

type invocation struct {
	name string
	args []string
	dir  string
	env  []string
}

type commandRunner interface {
	run(invocation, io.Writer, io.Writer) error
}

type execRunner struct{}

func (execRunner) run(call invocation, stdout, stderr io.Writer) error {
	command := exec.Command(call.name, call.args...)
	command.Dir = call.dir
	command.Env = call.env
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

type trustedTools struct {
	guard    string
	feedback string
	forma    string
	snapshot string
	goPath   string
}

type orchestrator struct {
	root         string
	runner       commandRunner
	tools        trustedTools
	repair       []string
	decisionPath string
	stdout       io.Writer
	stderr       io.Writer
	baseEnv      []string
}

type loopActions struct {
	measure       func() (agentrequest.Feedback, error)
	repair        func(int, agentrequest.Feedback) (repairResult, error)
	verify        func() error
	publishIntent func(agentrequest.Feedback, repairDecision) error
}

type repairResult struct {
	intentGap *repairDecision
}

type repairDecision struct {
	Schema      string   `json:"schema"`
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	FactIDs     []string `json:"factIds"`
	IntentNodes []string `json:"intentNodes"`
	Diagnostics []string `json:"diagnostics"`
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("membership-repair-orchestrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "Forma repository root")
	maxAttempts := flags.Int("max-attempts", 2, "maximum fresh repair processes")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: membership-repair-orchestrate [-root DIR] [-max-attempts N] -- <repair-command> [args...]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "The repair command receives FORMA_RETRY_ATTEMPT, FORMA_RETRY_STAGE, FORMA_RETRY_FEEDBACK, FORMA_RETRY_REQUEST, FORMA_RETRY_TARGET, and FORMA_RETRY_DECISION.")
	}
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if *maxAttempts < 1 {
		fmt.Fprintln(stderr, "membership repair orchestrator: -max-attempts must be at least 1")
		return 2
	}
	repair := flags.Args()
	if len(repair) == 0 {
		flags.Usage()
		return 2
	}

	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator:", err)
		return 2
	}
	if _, err := os.Stat(filepath.Join(absoluteRoot, "go.mod")); err != nil {
		fmt.Fprintf(stderr, "membership repair orchestrator: %s is not the Forma repository root: %v\n", absoluteRoot, err)
		return 2
	}
	repair[0], err = resolveExecutable(repair[0], absoluteRoot)
	if err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator: resolve repair command:", err)
		return 2
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator: resolve Go toolchain:", err)
		return 2
	}
	goPath, err = filepath.Abs(goPath)
	if err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator: resolve Go toolchain:", err)
		return 2
	}

	trustedDirectory, err := os.MkdirTemp("", "forma-membership-repair-")
	if err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator: create trusted directory:", err)
		return 1
	}
	defer os.RemoveAll(trustedDirectory)
	decisionDirectory, err := os.MkdirTemp("", "forma-membership-handoff-")
	if err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator: create handoff directory:", err)
		return 1
	}
	defer os.RemoveAll(decisionDirectory)

	tools := trustedTools{
		guard:    filepath.Join(trustedDirectory, "retryguard"),
		feedback: filepath.Join(trustedDirectory, "feedback"),
		forma:    filepath.Join(trustedDirectory, "forma"),
		snapshot: filepath.Join(trustedDirectory, "retry-baseline.json"),
		goPath:   goPath,
	}
	instance := orchestrator{
		root: absoluteRoot, runner: execRunner{}, tools: tools, repair: repair,
		decisionPath: filepath.Join(decisionDirectory, "repair-decision.json"),
		stdout:       stdout, stderr: stderr, baseEnv: os.Environ(),
	}
	if err := instance.prepare(); err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator:", err)
		return 1
	}
	actions := loopActions{
		measure:       instance.measure,
		repair:        instance.runRepair,
		verify:        instance.verify,
		publishIntent: instance.publishIntentGap,
	}
	if err := runRetries(*maxAttempts, actions, stdout); err != nil {
		fmt.Fprintln(stderr, "membership repair orchestrator:", err)
		return 1
	}
	return 0
}

func resolveExecutable(name, root string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		if !filepath.IsAbs(name) {
			name = filepath.Join(root, name)
		}
		absolute, err := filepath.Abs(name)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(absolute); err != nil {
			return "", err
		} else if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%s is not executable", absolute)
		}
		return absolute, nil
	}
	resolved, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func (orchestrator orchestrator) prepare() error {
	builds := []struct {
		output      string
		packagePath string
	}{
		{orchestrator.tools.guard, "./experiments/membership-agent-e2e/cmd/retryguard"},
		{orchestrator.tools.feedback, "./experiments/membership-agent-e2e/cmd/feedback"},
		{orchestrator.tools.forma, "./cmd/forma"},
	}
	for _, build := range builds {
		if err := orchestrator.run(invocation{
			name: orchestrator.tools.goPath,
			args: []string{"build", "-o", build.output, build.packagePath},
			dir:  orchestrator.root,
			env:  orchestrator.baseEnv,
		}); err != nil {
			return fmt.Errorf("build trusted %s: %w", filepath.Base(build.output), err)
		}
	}
	if err := orchestrator.run(invocation{
		name: orchestrator.tools.feedback,
		args: []string{"-snapshot-out", orchestrator.tools.snapshot},
		dir:  orchestrator.root,
		env:  orchestrator.trustedToolEnv(),
	}); err != nil {
		return fmt.Errorf("take retry baseline: %w", err)
	}
	fmt.Fprintln(orchestrator.stdout, "trusted guard, feedback generator, and verifier built before retry")
	return nil
}

func (orchestrator orchestrator) measure() (agentrequest.Feedback, error) {
	feedback := filepath.Join(orchestrator.root, filepath.FromSlash(feedbackPath))
	guardErr := orchestrator.run(invocation{
		name: orchestrator.tools.guard,
		args: []string{
			"-root", orchestrator.root,
			"-snapshot", orchestrator.tools.snapshot,
			"-feedback", feedback,
		},
		dir: orchestrator.root,
		env: orchestrator.trustedToolEnv(),
	})
	if guardErr != nil {
		observed, readErr := readFeedback(feedback)
		if readErr == nil && observed.Status == "blocked" && observed.Stage == "inspect" {
			return observed, nil
		}
		if readErr != nil {
			return agentrequest.Feedback{}, fmt.Errorf("run trusted guard: %w; read its feedback: %v", guardErr, readErr)
		}
		return agentrequest.Feedback{}, fmt.Errorf("run trusted guard: %w; published unexpected %s/%s feedback", guardErr, observed.Stage, observed.Status)
	}

	var measurementOutput, measurementErrors bytes.Buffer
	measurementErr := orchestrator.runner.run(invocation{
		name: orchestrator.tools.feedback,
		dir:  orchestrator.root,
		env:  orchestrator.trustedToolEnv(),
	}, &measurementOutput, &measurementErrors)
	observed, readErr := readFeedback(feedback)
	if readErr != nil {
		details := boundedCommandOutput(measurementOutput.String(), measurementErrors.String())
		if measurementErr != nil {
			return agentrequest.Feedback{}, fmt.Errorf("run trusted feedback generator: %w; read feedback: %v%s", measurementErr, readErr, details)
		}
		return agentrequest.Feedback{}, fmt.Errorf("%w%s", readErr, details)
	}
	switch observed.Status {
	case "succeeded":
		if measurementErr != nil {
			return agentrequest.Feedback{}, fmt.Errorf("feedback says succeeded but generator failed: %w", measurementErr)
		}
	case "failed":
		if measurementErr == nil {
			return agentrequest.Feedback{}, errors.New("feedback says failed but generator exited successfully")
		}
	default:
		return agentrequest.Feedback{}, fmt.Errorf("feedback generator published unexpected %s/%s feedback", observed.Stage, observed.Status)
	}
	return observed, nil
}

func boundedCommandOutput(stdout, stderr string) string {
	output := strings.TrimSpace(stderr + "\n" + stdout)
	if output == "" {
		return ""
	}
	const limit = 4000
	if len(output) > limit {
		start := len(output) - limit
		for start < len(output) && !utf8.RuneStart(output[start]) {
			start++
		}
		output = "…" + output[start:]
	}
	return "\ncommand output:\n" + output
}

func (orchestrator orchestrator) runRepair(attempt int, feedback agentrequest.Feedback) (repairResult, error) {
	feedbackFile := filepath.Join(orchestrator.root, filepath.FromSlash(feedbackPath))
	trustedFeedback, err := os.ReadFile(feedbackFile)
	if err != nil {
		return repairResult{}, fmt.Errorf("retain failed feedback before repair attempt %d: %w", attempt, err)
	}
	restoreRejectedDecision := func(decisionErr error) (repairResult, error) {
		if restoreErr := writeAtomic(feedbackFile, trustedFeedback); restoreErr != nil {
			return repairResult{}, fmt.Errorf("%w; restore trusted failed feedback: %v", decisionErr, restoreErr)
		}
		return repairResult{}, decisionErr
	}
	before, err := snapshotRepository(orchestrator.root)
	if err != nil {
		return repairResult{}, fmt.Errorf("snapshot repository before repair attempt %d: %w", attempt, err)
	}
	if err := os.Remove(orchestrator.decisionPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return repairResult{}, fmt.Errorf("remove stale repair decision: %w", err)
	}
	environment := append([]string(nil), orchestrator.baseEnv...)
	environment = replaceEnvironment(environment, "FORMA_RETRY_ATTEMPT", strconv.Itoa(attempt))
	environment = replaceEnvironment(environment, "FORMA_RETRY_FEEDBACK", feedbackFile)
	environment = replaceEnvironment(environment, "FORMA_RETRY_REQUEST", filepath.Join(orchestrator.root, filepath.FromSlash(requestPath)))
	environment = replaceEnvironment(environment, "FORMA_RETRY_TARGET", filepath.Join(orchestrator.root, filepath.FromSlash(targetPath)))
	environment = replaceEnvironment(environment, "FORMA_RETRY_STAGE", feedback.Stage)
	environment = replaceEnvironment(environment, "FORMA_RETRY_DECISION", orchestrator.decisionPath)
	if err := orchestrator.run(invocation{
		name: orchestrator.repair[0],
		args: orchestrator.repair[1:],
		dir:  orchestrator.root,
		env:  environment,
	}); err != nil {
		// The repair process may have partially edited or removed the feedback
		// before it stopped. Put back the exact trusted measurement it started
		// from so a transport failure cannot replace the human handoff artifact.
		if restoreErr := writeAtomic(feedbackFile, trustedFeedback); restoreErr != nil {
			return repairResult{}, fmt.Errorf("repair command stopped on attempt %d: %w; restore failed feedback: %v", attempt, err, restoreErr)
		}
		return repairResult{}, fmt.Errorf("repair command stopped on attempt %d; the failed feedback remains for human inspection: %w", attempt, err)
	}
	decision, found, err := readRepairDecision(orchestrator.decisionPath, feedback, filepath.Join(orchestrator.root, filepath.FromSlash(requestPath)))
	if err != nil {
		return restoreRejectedDecision(fmt.Errorf("validate repair decision on attempt %d: %w", attempt, err))
	}
	if !found {
		return repairResult{}, nil
	}
	after, err := snapshotRepository(orchestrator.root)
	if err != nil {
		return restoreRejectedDecision(fmt.Errorf("snapshot repository after intent-gap decision: %w", err))
	}
	if changes := compareSnapshots(before, after); len(changes) != 0 {
		return restoreRejectedDecision(fmt.Errorf("intent-gap decision left repository changes: %s", strings.Join(changes, ", ")))
	}
	return repairResult{intentGap: &decision}, nil
}

func readRepairDecision(path string, feedback agentrequest.Feedback, requestFile string) (repairDecision, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return repairDecision{}, false, nil
	}
	if err != nil {
		return repairDecision{}, false, err
	}
	if !info.Mode().IsRegular() {
		return repairDecision{}, false, errors.New("repair decision is not a regular file")
	}
	if info.Size() > maxDecisionBytes {
		return repairDecision{}, false, fmt.Errorf("repair decision exceeds %d bytes", maxDecisionBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return repairDecision{}, false, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxDecisionBytes+1))
	if err != nil {
		return repairDecision{}, false, err
	}
	if len(content) > maxDecisionBytes {
		return repairDecision{}, false, fmt.Errorf("repair decision exceeds %d bytes", maxDecisionBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var decision repairDecision
	if err := decoder.Decode(&decision); err != nil {
		return repairDecision{}, false, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return repairDecision{}, false, errors.New("repair decision contains more than one JSON value")
		}
		return repairDecision{}, false, err
	}
	if decision.Schema != repairDecisionSchema {
		return repairDecision{}, false, fmt.Errorf("schema %q is not %q", decision.Schema, repairDecisionSchema)
	}
	if decision.Status != "intent-gap" {
		return repairDecision{}, false, fmt.Errorf("status %q is not intent-gap", decision.Status)
	}
	if strings.TrimSpace(decision.Summary) == "" || len(decision.Summary) > maxDecisionSummaryBytes {
		return repairDecision{}, false, fmt.Errorf("summary must contain 1 to %d bytes", maxDecisionSummaryBytes)
	}
	if err := validateDecisionStrings("diagnostics", decision.Diagnostics, 1, maxDecisionDiagnostics, maxDecisionDiagnosticBytes); err != nil {
		return repairDecision{}, false, err
	}
	if err := validateDecisionStrings("factIds", decision.FactIDs, 1, maxDecisionEntries, maxDecisionIDBytes); err != nil {
		return repairDecision{}, false, err
	}
	if err := validateDecisionStrings("intentNodes", decision.IntentNodes, 1, maxDecisionEntries, maxDecisionIDBytes); err != nil {
		return repairDecision{}, false, err
	}

	requestContent, err := os.ReadFile(requestFile)
	if err != nil {
		return repairDecision{}, false, err
	}
	request, err := agentrequest.UnmarshalRequest(requestContent)
	if err != nil {
		return repairDecision{}, false, err
	}
	requestedFacts := map[string]bool{}
	for _, fact := range request.AcceptanceFacts.Facts {
		requestedFacts[string(fact.ID)] = true
	}
	observedFailures := map[string]bool{}
	for _, coverage := range feedback.FactCoverage {
		if coverage.Result == "failed" {
			observedFailures[string(coverage.FactID)] = true
		}
	}
	if len(observedFailures) == 0 {
		return repairDecision{}, false, fmt.Errorf(
			"an intent gap needs a rejected Acceptance Fact; the %s measurement has none", feedback.Stage,
		)
	}
	for _, id := range decision.FactIDs {
		if !requestedFacts[id] {
			return repairDecision{}, false, fmt.Errorf("factId %s is not in the Generation Request", id)
		}
		if !observedFailures[id] {
			return repairDecision{}, false, fmt.Errorf("factId %s was not failed in the trusted feedback", id)
		}
	}
	requestNodes := map[string]bool{}
	for _, entry := range request.SourceMap.Entries {
		requestNodes[string(entry.NodeID)] = true
	}
	related := map[string]bool{}
	for _, id := range feedback.RelatedIntentNodes {
		related[string(id)] = true
	}
	overlapsFailure := false
	for _, id := range decision.IntentNodes {
		if !requestNodes[id] {
			return repairDecision{}, false, fmt.Errorf("intentNode %s is not in the Source Map", id)
		}
		if related[id] {
			overlapsFailure = true
		}
	}
	if !overlapsFailure {
		return repairDecision{}, false, errors.New("intentNodes do not overlap the failed feedback relatedIntentNodes")
	}
	sort.Strings(decision.FactIDs)
	sort.Strings(decision.IntentNodes)
	return decision, true, nil
}

func validateDecisionStrings(name string, values []string, minimum, maximum, maxLength int) error {
	if len(values) < minimum || len(values) > maximum {
		return fmt.Errorf("%s must contain %d to %d entries", name, minimum, maximum)
	}
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > maxLength {
			return fmt.Errorf("%s entries must contain 1 to %d bytes", name, maxLength)
		}
		if seen[value] {
			return fmt.Errorf("%s repeats %q", name, value)
		}
		seen[value] = true
	}
	return nil
}

type repositorySnapshot map[string]string

func snapshotRepository(root string) (repositorySnapshot, error) {
	snapshot := repositorySnapshot{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if path != root && ignoredSnapshotDirectory(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredSnapshotFile(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest := sha256.New()
		if _, err := io.WriteString(digest, info.Mode().String()+"\x00"); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(digest, "symlink\x00"+target); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(digest, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		} else {
			return nil
		}
		snapshot[relative] = hex.EncodeToString(digest.Sum(nil))
		return nil
	})
	return snapshot, err
}

func ignoredSnapshotDirectory(relative string) bool {
	// These are the directory entries currently ignored by the repository root
	// .gitignore, plus .git itself. A test keeps this list synchronized.
	switch relative {
	case ".git", ".forma-build", ".claude/skills":
		return true
	default:
		return false
	}
}

func ignoredSnapshotFile(relative string) bool {
	// Generation Feedback is the loop output. The other paths are the file
	// entries currently ignored by the repository root .gitignore.
	if relative == feedbackPath || relative == "forma" || relative == "coverage.out" {
		return true
	}
	return filepath.Base(relative) == ".DS_Store"
}

func compareSnapshots(before, after repositorySnapshot) []string {
	changes := []string{}
	for path, want := range before {
		got, ok := after[path]
		switch {
		case !ok:
			changes = append(changes, "missing "+path)
		case got != want:
			changes = append(changes, "modified "+path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changes = append(changes, "added "+path)
		}
	}
	sort.Strings(changes)
	if len(changes) > 10 {
		changes = append(changes[:10], fmt.Sprintf("and %d more", len(changes)-10))
	}
	return changes
}

func writeAtomic(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".repair-feedback-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (orchestrator orchestrator) verify() error {
	if err := orchestrator.run(invocation{
		name: orchestrator.tools.forma,
		args: []string{
			"verify",
			"--repository", targetPath,
			"--baseline", baselinePath,
			requestPath,
			feedbackPath,
		},
		dir: orchestrator.root,
		env: orchestrator.trustedToolEnv(),
	}); err != nil {
		return fmt.Errorf("trusted verifier rejected the retry: %w", err)
	}
	return nil
}

func (orchestrator orchestrator) publishIntentGap(measurement agentrequest.Feedback, decision repairDecision) error {
	requestFile := filepath.Join(orchestrator.root, filepath.FromSlash(requestPath))
	requestContent, err := os.ReadFile(requestFile)
	if err != nil {
		return err
	}
	request, err := agentrequest.UnmarshalRequest(requestContent)
	if err != nil {
		return err
	}
	baselineContent, err := os.ReadFile(filepath.Join(orchestrator.root, filepath.FromSlash(baselinePath)))
	if err != nil {
		return err
	}
	baseline, err := agentrequest.UnmarshalRequest(baselineContent)
	if err != nil {
		return err
	}

	selectedNodes := map[string]bool{}
	for _, id := range decision.IntentNodes {
		selectedNodes[id] = true
	}
	measurement.RelatedIntentNodes = nil
	for _, entry := range request.SourceMap.Entries {
		if selectedNodes[string(entry.NodeID)] {
			measurement.RelatedIntentNodes = append(measurement.RelatedIntentNodes, entry.NodeID)
		}
	}
	// Preserve the trusted measurement stage: the test ran and rejected a Fact.
	// `blocked` describes the new outcome without pretending the measurement
	// stopped before build or test.
	measurement.Status = "blocked"
	measurement.PolicyCoverage = nil
	measurement.Diagnostics = append([]string{
		"intent-gap candidate: the repair process left the repository unchanged and the trusted measurement remained failed",
	}, decision.Diagnostics...)
	measurement.Summary = fmt.Sprintf(
		"Human decision required for a Forma intent gap: %s A fresh trusted measurement still failed, and the repair process left the repository unchanged. Affected Acceptance Facts: %s.",
		strings.TrimSpace(decision.Summary), strings.Join(decision.FactIDs, ", "),
	)
	if err := agentrequest.ValidateCompletion(request, &baseline, measurement, filepath.Join(orchestrator.root, filepath.FromSlash(targetPath))); err != nil {
		return fmt.Errorf("validate intent-gap handoff: %w", err)
	}
	encoded, err := json.MarshalIndent(measurement, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(orchestrator.root, filepath.FromSlash(feedbackPath)), append(encoded, '\n')); err != nil {
		return err
	}
	fmt.Fprintf(orchestrator.stdout, "published %s/blocked Generation Feedback for human intent-gap review\n", measurement.Stage)
	return nil
}

func (orchestrator orchestrator) run(call invocation) error {
	return orchestrator.runner.run(call, orchestrator.stdout, orchestrator.stderr)
}

func (orchestrator orchestrator) trustedToolEnv() []string {
	environment := append([]string(nil), orchestrator.baseEnv...)
	// The feedback binary launches `go test`. Resolve Go before the retry and
	// give the measurement process a PATH that cannot prefer an executable the
	// repair added to the repository.
	trustedPath := filepath.Dir(orchestrator.tools.goPath) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin"
	return replaceEnvironment(environment, "PATH", trustedPath)
}

func replaceEnvironment(environment []string, key, value string) []string {
	prefix := key + "="
	filtered := environment[:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}

func readFeedback(path string) (agentrequest.Feedback, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return agentrequest.Feedback{}, fmt.Errorf("read Generation Feedback: %w", err)
	}
	feedback, err := agentrequest.UnmarshalFeedback(content)
	if err != nil {
		return agentrequest.Feedback{}, err
	}
	return feedback, nil
}

func runRetries(maxAttempts int, actions loopActions, stdout io.Writer) error {
	feedback, err := actions.measure()
	if err != nil {
		return fmt.Errorf("measure initial repository state: %w", err)
	}
	fmt.Fprintf(stdout, "initial measurement: %s/%s\n", feedback.Stage, feedback.Status)
	switch feedback.Status {
	case "succeeded":
		if err := actions.verify(); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "repair loop already satisfied; no repair process started")
		return nil
	case "blocked":
		return fmt.Errorf("retry baseline was already blocked before a repair started: %s", feedback.Summary)
	case "failed":
		// Continue into the first repair attempt.
	default:
		return fmt.Errorf("initial measurement has unsupported status %q", feedback.Status)
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		fmt.Fprintf(stdout, "repair attempt %d/%d starts from %s failure\n", attempt, maxAttempts, feedback.Stage)
		result, err := actions.repair(attempt, feedback)
		if err != nil {
			return err
		}
		feedback, err = actions.measure()
		if err != nil {
			return fmt.Errorf("measure repair attempt %d: %w", attempt, err)
		}
		fmt.Fprintf(stdout, "repair attempt %d measurement: %s/%s\n", attempt, feedback.Stage, feedback.Status)
		if result.intentGap != nil {
			if feedback.Status != "failed" {
				return fmt.Errorf("repair attempt %d declared an intent gap, but the trusted measurement is %s/%s", attempt, feedback.Stage, feedback.Status)
			}
			if err := actions.publishIntent(feedback, *result.intentGap); err != nil {
				return fmt.Errorf("publish intent-gap handoff for attempt %d: %w", attempt, err)
			}
			return fmt.Errorf("repair attempt %d found a Forma intent gap; blocked feedback requires human review", attempt)
		}
		switch feedback.Status {
		case "succeeded":
			if err := actions.verify(); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "repair loop succeeded after %d attempt(s)\n", attempt)
			return nil
		case "blocked":
			return fmt.Errorf("repair attempt %d changed the retry baseline: %s", attempt, feedback.Summary)
		case "failed":
			// Start another fresh repair process if the bound permits it.
		default:
			return fmt.Errorf("repair attempt %d has unsupported status %q", attempt, feedback.Status)
		}
	}
	return fmt.Errorf("repair remained failed after %d attempt(s); the latest Generation Feedback remains for human inspection", maxAttempts)
}
