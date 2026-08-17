// Command generate produces the Stage D Generation Request from the real
// membership Forma source, using the applied historical admin request as the
// incremental baseline. It never edits JSON by hand.
package main

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

// historicalAdminBlob pins the applied v0alpha2 admin request. The bytes in the
// working tree must match the immutable git blob so the lineage is anchored to
// what was actually applied to the admin target.
const historicalAdminBlob = "5751ecf85e9b7be2665aa91854ee5b69798e81a3"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "membership-agent-e2e generate:", err)
		os.Exit(1)
	}
}

func run() error {
	root := filepath.Join("..", "..")
	if _, err := os.Stat("go.mod"); err == nil {
		root = "."
	}
	baselinePath := filepath.Join(root, "internal", "agentrequest", "testdata", "admin.incremental.request.json")
	sourcePath := filepath.Join(root, "experiments", "membership-agent-e2e", "app.forma")

	baselineContent, err := os.ReadFile(baselinePath)
	if err != nil {
		return fmt.Errorf("read historical baseline: %w", err)
	}
	if got := gitBlobID(baselineContent); got != historicalAdminBlob {
		return fmt.Errorf("historical baseline blob %s does not match the applied request %s", got, historicalAdminBlob)
	}
	baseline, err := agentrequest.UnmarshalRequest(baselineContent)
	if err != nil {
		return fmt.Errorf("decode historical baseline: %w", err)
	}

	sourceContent, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read Forma source: %w", err)
	}
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("experiments/membership-agent-e2e/app.forma", string(sourceContent)),
	})
	if len(result.Diagnostics) != 0 {
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintln(os.Stderr, diagnostic.Error())
		}
		return fmt.Errorf("Forma source has %d diagnostics", len(result.Diagnostics))
	}

	request, err := agentrequest.BuildIncremental(baseline, result, nil)
	if err != nil {
		return fmt.Errorf("build incremental request: %w", err)
	}
	if err := agentrequest.ValidateIncrementalBaseline(request, baseline); err != nil {
		return fmt.Errorf("validate lineage: %w", err)
	}

	// Pin the measured Stage D boundary so a later language or builder change
	// cannot move the experiment result without someone noticing.
	if err := assertMeasuredLineage(request); err != nil {
		return err
	}

	change := request.RequestedChange
	fmt.Printf("facts %d (changed %d, unchanged %d)\n",
		len(request.AcceptanceFacts.Facts), len(change.FactChanges), change.UnchangedFacts)
	fmt.Printf("intent nodes changed %d, unchanged %d\n", len(change.IntentChanges), change.UnchangedIntentNodes)
	fmt.Printf("review requirements %d (changed %d, unchanged %d)\n",
		len(request.ReviewRequirements.Requirements), len(change.ReviewRequirementChanges), change.UnchangedReviewRequirements)

	encoded, err := agentrequest.Marshal(request)
	if err != nil {
		return err
	}
	out := filepath.Join(root, "experiments", "membership-agent-e2e", "generation-request.json")
	return os.WriteFile(out, append(encoded, '\n'), 0o644)
}

const (
	expectedFacts                    = 81
	expectedFactChanges              = 38
	expectedUnchangedFacts           = 43
	expectedReviewRequirementChanges = 3
)

func assertMeasuredLineage(request agentrequest.Request) error {
	change := request.RequestedChange
	if got := len(request.AcceptanceFacts.Facts); got != expectedFacts {
		return fmt.Errorf("Acceptance Facts = %d, want the measured %d", got, expectedFacts)
	}
	if got := len(change.FactChanges); got != expectedFactChanges {
		return fmt.Errorf("fact changes = %d, want the measured %d", got, expectedFactChanges)
	}
	for _, item := range change.FactChanges {
		if item.Kind != "added" {
			return fmt.Errorf("fact %s is %s; Identity must not change or remove an admin fact", item.NodeID, item.Kind)
		}
	}
	if change.UnchangedFacts != expectedUnchangedFacts {
		return fmt.Errorf("unchanged facts = %d, want the measured %d admin facts", change.UnchangedFacts, expectedUnchangedFacts)
	}
	if got := len(change.ReviewRequirementChanges); got != expectedReviewRequirementChanges {
		return fmt.Errorf("review requirement changes = %d, want the measured %d", got, expectedReviewRequirementChanges)
	}
	return nil
}

func gitBlobID(content []byte) string {
	hash := sha1.New()
	fmt.Fprintf(hash, "blob %d%c", len(content), 0)
	hash.Write(content)
	return fmt.Sprintf("%x", hash.Sum(nil))
}
