package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

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
