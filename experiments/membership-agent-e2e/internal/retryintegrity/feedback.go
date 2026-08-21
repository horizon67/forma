package retryintegrity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/agentrequest"
)

// Retract withdraws the previous Generation Feedback.
//
// It runs before the tree is checked, not after. `forma verify` reads whatever
// file is on disk, so a retry that stops without withdrawing the previous run's
// succeeded feedback leaves an artifact that still verifies as 85/85 while the
// repair never happened. Withdrawing first means the only ways to end a retry
// are a fresh measurement, a blocked record, or no feedback at all.
func Retract(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("retract the previous feedback: %w", err)
	}
	return nil
}

// PublishBlocked records that the retry changed what the measurement means.
//
// The feedback carries no fact coverage. Nothing was observed: the gate runs
// before the test command, and dressing tampering up as a failed Acceptance
// Fact would claim a measurement that never happened.
func PublishBlocked(path, command string, violations []Violation) error {
	if len(violations) == 0 {
		return errors.New("publish blocked feedback: no violations to report")
	}
	feedback := agentrequest.Feedback{
		Schema: agentrequest.FeedbackSchema,
		// The gate inspects the repository; it never reaches edit, build or test.
		Stage:       "inspect",
		Status:      "blocked",
		Command:     command,
		Diagnostics: Diagnostics(violations),
		Summary:     BlockedSummary(violations),
	}
	encoded, err := json.MarshalIndent(feedback, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(encoded, '\n'))
}

// BlockedSummary states what moved, grouped by the guarantee it broke.
func BlockedSummary(violations []Violation) string {
	reasons := map[string]int{}
	for _, violation := range violations {
		reasons[violation.Reason]++
	}
	names := make([]string, 0, len(reasons))
	for reason := range reasons {
		names = append(names, reason)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for index, reason := range names {
		parts[index] = fmt.Sprintf("%d %s", reasons[reason], reason)
	}
	return fmt.Sprintf(
		"The retry changed %d path(s) it may not change (%s), so nothing was measured. "+
			"A repair must change the target implementation; the tests, the coverage map and the requested facts are the retry baseline. "+
			"No Acceptance Fact was observed, so this feedback reports no fact coverage and no policy coverage.",
		len(violations), strings.Join(parts, ", "),
	)
}

func writeAtomic(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".generation-feedback-*.json")
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
