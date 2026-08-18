// Command retryguard is the authoritative retry gate.
//
// It exists separately from the feedback generator for one reason: a check that
// shares a process with agent-editable code has already lost. A file added to
// the generator's package is compiled into the generator, and its init runs
// before the check does — early enough to rewrite the coverage map, or the
// baseline file the check is about to read.
//
// So the intended use is to build this binary from the trusted tree, before the
// retry starts, and run the built binary during the retry:
//
//	go build -o /trusted/retryguard ./experiments/membership-agent-e2e/cmd/retryguard
//	go run ./experiments/membership-agent-e2e/cmd/feedback -snapshot-out /trusted/retry-baseline.json
//	# ... the retry edits the repository ...
//	/trusted/retryguard -snapshot /trusted/retry-baseline.json \
//	  -feedback experiments/membership-agent-e2e/target/generation-feedback.json \
//	  && go run ./experiments/membership-agent-e2e/cmd/feedback
//
// The `&&` is part of the gate. A rejected retry must not go on to run the
// generator, because the generator is exactly the code the retry may have
// edited.
//
// This binary owns the feedback file for the duration of the check. It
// withdraws the previous feedback before looking at anything, and on a
// violation it publishes the blocked record itself, so neither step depends on
// code the retry could have changed. Stopping here can therefore never leave an
// earlier succeeded feedback in place for `forma verify` to accept.
//
// The binary imports only the integrity package and reads only the snapshot, so
// nothing the retry adds to the repository is linked into this check. That is
// what makes it a gate rather than a courtesy. The generator's own -snapshot
// flag runs the same comparison and is useful while developing, but it cannot
// make this guarantee about itself.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/horizon67/forma/experiments/membership-agent-e2e/internal/retryintegrity"
)

// command records how the gate was invoked, so a blocked feedback says what
// produced it.
const command = "retryguard -snapshot <retry-baseline.json> -feedback <generation-feedback.json>"

func main() {
	root := flag.String("root", ".", "repository root the baseline paths are relative to")
	snapshot := flag.String("snapshot", "", "retry baseline taken before the retry started")
	feedback := flag.String("feedback", "", "Generation Feedback this gate owns for the duration of the check")
	flag.Parse()
	os.Exit(run(*root, *snapshot, *feedback, os.Stdout, os.Stderr))
}

func run(root, snapshotPath, feedbackPath string, stdout, stderr io.Writer) int {
	if snapshotPath == "" || feedbackPath == "" {
		fmt.Fprintln(stderr, "retryguard: -snapshot and -feedback are both required")
		return 2
	}
	// Withdraw first. If the check or the publication below fails, the retry
	// still cannot end on a stale succeeded artifact.
	if err := retryintegrity.Retract(feedbackPath); err != nil {
		fmt.Fprintln(stderr, "retryguard:", err)
		return 2
	}
	violations, err := check(root, snapshotPath)
	if err != nil {
		fmt.Fprintln(stderr, "retryguard:", err)
		return 2
	}
	if len(violations) == 0 {
		fmt.Fprintln(stdout, "retry baseline intact")
		return 0
	}
	if err := retryintegrity.PublishBlocked(feedbackPath, command, violations); err != nil {
		fmt.Fprintln(stderr, "retryguard:", err)
		return 2
	}
	for _, line := range retryintegrity.Diagnostics(violations) {
		fmt.Fprintln(stderr, line)
	}
	fmt.Fprintln(stderr, "retryguard: published blocked Generation Feedback; do not run the generator for this retry")
	return 1
}

func check(root, snapshotPath string) ([]retryintegrity.Violation, error) {
	content, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, err
	}
	var snapshot retryintegrity.Snapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return nil, err
	}
	return retryintegrity.Check(root, snapshot)
}
