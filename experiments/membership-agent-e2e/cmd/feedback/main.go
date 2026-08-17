// Command feedback maps every Acceptance Fact in the Stage D Generation Request
// to the repository test that observes it, and writes the Generation Feedback.
// The mapping is explicit: a fact with no entry fails the build rather than
// being silently dropped.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
	"github.com/horizon67/forma/internal/implementationpolicy"
)

const (
	adminTests      = "internal/web/server_test.go#"
	memberTests     = "internal/web/membership_e2e_test.go#"
	storeTests      = "internal/store/store_test.go#"
	identityTests   = "internal/identity/identity_test.go#"
	submissionTests = "internal/web/submission_test.go#"
	executableTests = "cmd/server/main_test.go#"
)

// coverage lists the tests that observe each fact. Keys are fact IDs with the
// leading "fact/" removed.
var coverage = map[string][]string{
	// --- Admin surfaces, unchanged from the baseline application ---
	"entity/User/field/team/relation/resolved":                             {adminTests + "TestRelationUsesTeamLabel"},
	"page/Users/view/list/User/access/allowed/admin":                       {adminTests + "TestAdminList"},
	"page/Users/view/list/User/access/denied/anonymous":                    {adminTests + "TestAccessDenied", executableTests + "TestExecutableServesBothSurfaces"},
	"page/Users/view/list/User/records/visible":                            {adminTests + "TestAdminList"},
	"page/Users/view/list/User/fields":                                     {adminTests + "TestAdminList"},
	"page/Users/view/list/User/actions":                                    {adminTests + "TestAdminList"},
	"page/Users/view/list/User/feedback":                                   {adminTests + "TestListEmptyAndFailure"},
	"page/Users/view/list/User/search":                                     {adminTests + "TestListQueryCapabilities"},
	"page/Users/view/list/User/sort":                                       {adminTests + "TestListQueryCapabilities"},
	"page/Users/view/list/User/filter/plan":                                {adminTests + "TestListQueryCapabilities"},
	"page/Users/view/list/User/filter/status":                              {adminTests + "TestListQueryCapabilities"},
	"page/Users/view/list/User/filter/team":                                {adminTests + "TestListQueryCapabilities"},
	"page/Users/view/list/User/page-boundary":                              {adminTests + "TestListPagination"},
	"page/Users/view/list/User/action/view/access/allowed/admin":           {adminTests + "TestAdminList"},
	"page/Users/view/list/User/action/view/access/denied/anonymous":        {adminTests + "TestAccessDenied"},
	"page/Users/view/list/User/action/view/navigation":                     {adminTests + "TestAdminList", adminTests + "TestUserDetail"},
	"page/Users/view/list/User/action/edit/access/allowed/admin":           {adminTests + "TestAdminList"},
	"page/Users/view/list/User/action/edit/access/denied/anonymous":        {adminTests + "TestAccessDenied"},
	"page/Users/view/list/User/action/edit/navigation":                     {adminTests + "TestAdminList", adminTests + "TestEditUser"},
	"page/UserDetail/view/detail/User/access/allowed/admin":                {adminTests + "TestUserDetail"},
	"page/UserDetail/view/detail/User/access/denied/anonymous":             {adminTests + "TestAccessDenied"},
	"page/UserDetail/view/detail/User/records/visible":                     {adminTests + "TestUserDetail"},
	"page/UserDetail/view/detail/User/fields":                              {adminTests + "TestUserDetail"},
	"page/UserDetail/view/detail/User/actions":                             {adminTests + "TestUserDetail"},
	"page/UserDetail/view/detail/User/feedback":                            {adminTests + "TestDetailEmptyAndFailure"},
	"page/UserDetail/view/detail/User/action/edit/access/allowed/admin":    {adminTests + "TestUserDetail"},
	"page/UserDetail/view/detail/User/action/edit/access/denied/anonymous": {adminTests + "TestAccessDenied"},
	"page/UserDetail/view/detail/User/action/edit/navigation":              {adminTests + "TestUserDetail", adminTests + "TestEditUser"},
	"page/UserEdit/view/form/edit/User/access/allowed/admin":               {adminTests + "TestEditUser"},
	"page/UserEdit/view/form/edit/User/access/denied/anonymous":            {adminTests + "TestAccessDenied"},
	"page/UserEdit/view/form/edit/User/fields":                             {adminTests + "TestEditUser"},
	"page/UserEdit/view/form/edit/User/feedback":                           {adminTests + "TestEditValidationAndFailure"},
	"page/UserEdit/view/form/edit/User/submit/access/allowed/admin":        {adminTests + "TestEditUser"},
	"page/UserEdit/view/form/edit/User/submit/access/denied/anonymous":     {adminTests + "TestAccessDenied"},
	"page/UserEdit/view/form/edit/User/submit/mutation/accepted":           {adminTests + "TestEditUser"},
	"page/UserEdit/view/form/edit/User/submit/mutation/at-most-once":       {adminTests + "TestEditIsAppliedAtMostOnce"},
	"page/UserEdit/view/form/edit/User/submit/navigation":                  {adminTests + "TestEditUser"},
	"page/UserEdit/view/form/edit/User/submit/validation/required/name":    {adminTests + "TestEditValidationAndFailure"},
	"page/UserEdit/view/form/edit/User/submit/validation/required/email":   {adminTests + "TestEditValidationAndFailure"},
	"page/UserEdit/view/form/edit/User/submit/validation/required/plan":    {adminTests + "TestEditValidationAndFailure"},
	"page/UserEdit/view/form/edit/User/submit/validation/closed-set/plan":  {adminTests + "TestEditValidationAndFailure"},
	"page/UserEdit/view/form/edit/User/submit/validation/unique/email": {
		adminTests + "TestEditValidationAndFailure", storeTests + "TestUpdateUserEnforcesUniqueEmail",
	},
	"page/UserEdit/view/form/edit/User/submit/validation/matches/email/constraint/type/Email/constraint/matches": {
		adminTests + "TestEditValidationAndFailure",
	},

	// --- Membership surfaces added by this experiment ---
	"page/SignUp/identity/register/UserAccount/access/allowed/anonymous": {
		memberTests + "TestMembershipRoutesAreReachableThroughTheShippedHandler",
		executableTests + "TestExecutableServesBothSurfaces",
	},
	"page/SignUp/identity/register/UserAccount/inputs":     {memberTests + "TestSignUpFormOffersExactlyTheDeclaredInputs"},
	"page/SignUp/identity/register/UserAccount/navigation": {memberTests + "TestRepeatedSignUpDispatchAppliesOnce"},
	"page/SignUp/identity/register/UserAccount/validation/preserve-input": {
		memberTests + "TestSignUpValidationKeepsInputAndNeverEchoesTheCredential",
	},
	"identity/UserAccount/credential/password/non-projectable": {
		memberTests + "TestSignUpValidationKeepsInputAndNeverEchoesTheCredential",
		memberTests + "TestSignInFailuresShareOneMessage",
	},
	"identity/UserAccount/operation/register/subject/created":      {memberTests + "TestSignUpCreatesAPendingMemberAndOneNotice"},
	"identity/UserAccount/operation/register/credential/bound":     {memberTests + "TestSignUpCreatesAPendingMemberAndOneNotice", memberTests + "TestSessionOwnershipIsEnforcedOnTheServer"},
	"identity/UserAccount/operation/register/verification/issued":  {memberTests + "TestSignUpCreatesAPendingMemberAndOneNotice"},
	"identity/UserAccount/operation/register/notice/emitted":       {memberTests + "TestSignUpCreatesAPendingMemberAndOneNotice"},
	"identity/UserAccount/operation/register/at-most-once":         {memberTests + "TestRepeatedSignUpDispatchAppliesOnce"},
	"identity/UserAccount/operation/register/validation/rejected":  {memberTests + "TestRegistrationRejectsEveryDeclaredInvalidCase"},
	"identity/UserAccount/operation/register/identifier/duplicate": {memberTests + "TestDuplicateIdentifierCoversExactAndCanonicalForms"},
	"identity/UserAccount/operation/resend/accepted":               {memberTests + "TestResendRotatesEvidenceAndRepeatsApplyOnce"},
	"identity/UserAccount/operation/resend/evidence/rotated":       {memberTests + "TestResendRotatesEvidenceAndRepeatsApplyOnce"},
	"identity/UserAccount/operation/resend/at-most-once":           {memberTests + "TestResendRotatesEvidenceAndRepeatsApplyOnce"},
	"page/CheckEmail/identity/resend/UserAccount/disclosure/uniform": {
		memberTests + "TestResendDisclosureIsUniformForEveryAccountState",
	},
	"identity/UserAccount/operation/verify/accepted": {memberTests + "TestVerificationAppliesOnceAndDoesNotSignIn"},
	"identity/UserAccount/operation/verify/evidence/consumed": {
		memberTests + "TestSuccessfulVerificationLeavesTheEvidenceConsumed",
		memberTests + "TestVerificationRejectionCoversEveryDeclaredCase",
	},
	"identity/UserAccount/operation/verify/evidence/rejected": {memberTests + "TestVerificationRejectionCoversEveryDeclaredCase"},
	"identity/UserAccount/operation/verify/expiry/boundary":   {memberTests + "TestExpiryBoundaryIsCheckedOnBothSides", identityTests + "TestEvidenceExpiryIsAClockRelation"},
	"page/VerifyEmail/identity/verify/UserAccount/navigation": {memberTests + "TestVerificationAppliesOnceAndDoesNotSignIn"},
	"identity/UserAccount/verification/email/notice/delivery/failure": {
		memberTests + "TestDeliveryFailureKeepsTheAccountRecoverable",
	},
	"identity/UserAccount/operation/signin/accepted":            {memberTests + "TestSessionOwnershipIsEnforcedOnTheServer"},
	"identity/UserAccount/operation/signin/state/ineligible":    {memberTests + "TestPendingMemberCannotSignIn"},
	"identity/UserAccount/operation/signin/rejected/generic":    {memberTests + "TestSignInFailuresShareOneMessage"},
	"identity/UserAccount/operation/signout/session/terminated": {memberTests + "TestSignOutClosesBothProtectedSurfaces"},
	"identity/UserAccount/ownership/self/access/allowed/self":   {memberTests + "TestSessionOwnershipIsEnforcedOnTheServer"},
	"identity/UserAccount/ownership/self/access/denied/other-subject": {
		memberTests + "TestSessionOwnershipIsEnforcedOnTheServer",
	},
	"identity/UserAccount/ownership/self/access/denied/anonymous": {
		memberTests + "TestSessionOwnershipIsEnforcedOnTheServer", memberTests + "TestSignOutClosesBothProtectedSurfaces",
	},
	"page/Profile/view/detail/User/records/visible": {memberTests + "TestSessionOwnershipIsEnforcedOnTheServer"},
	"page/Profile/view/detail/User/fields":          {memberTests + "TestProfileShowsExactlyTheDeclaredFields"},
	"page/Profile/view/detail/User/feedback": {
		memberTests + "TestProfileReportsEmptyWhenTheRecordIsGone",
		memberTests + "TestProfileReportsFailureWhenTheRecordCannotBeRead",
	},
	"page/ProfileEdit/view/form/edit/User/fields":                       {memberTests + "TestProfileEditOffersExactlyTheDeclaredFields"},
	"page/ProfileEdit/view/form/edit/User/submit/mutation/accepted":     {memberTests + "TestProfileEditAppliesOneMutationForARepeatedDispatch"},
	"page/ProfileEdit/view/form/edit/User/submit/mutation/at-most-once": {memberTests + "TestProfileEditAppliesOneMutationForARepeatedDispatch", submissionTests + "TestGuardReplaysTheCompletedOutcome"},
	"page/ProfileEdit/view/form/edit/User/submit/navigation":            {memberTests + "TestProfileEditAppliesOneMutationForARepeatedDispatch"},
	"page/ProfileEdit/view/form/edit/User/submit/validation/required/name": {
		memberTests + "TestProfileEditValidationKeepsInputAndIssuesAFreshToken",
	},
	"page/ProfileEdit/view/form/edit/User/feedback": {
		memberTests + "TestProfileEditValidationKeepsInputAndIssuesAFreshToken",
		memberTests + "TestProfileEditReportsFailureWhenTheSaveCannotCommit",
	},
}

// policyEvidence records where each declared implementation policy is visible.
var policyEvidence = map[string]implementationpolicy.Coverage{
	"implementation/server-rendering": {
		PolicyID: "implementation/server-rendering", Status: "satisfied",
		Evidence: []string{"internal/web/server.go", "internal/web/templates.go"},
	},
	"implementation/persistence": {
		PolicyID: "implementation/persistence", Status: "deviated",
		Reason: "This experiment keeps the in-memory store it inherited so the membership flow can be measured without introducing a database.",
	},
	"implementation/router": {
		PolicyID: "implementation/router", Status: "satisfied",
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "membership-agent-e2e feedback:", err)
		os.Exit(1)
	}
}

func run() error {
	root := "."
	if _, err := os.Stat("go.mod"); err != nil {
		root = filepath.Join("..", "..")
	}
	out := filepath.Join(root, "experiments", "membership-agent-e2e", "target", "generation-feedback.json")
	// The feedback is a measurement, not a document. Retract the previous one
	// before this run starts: if anything below fails, the tree must be left
	// with no feedback at all rather than with the last successful record,
	// which `forma verify` would still accept as current.
	if err := os.Remove(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("retract the previous feedback: %w", err)
	}
	requestPath := filepath.Join(root, "experiments", "membership-agent-e2e", "generation-request.json")
	content, err := os.ReadFile(requestPath)
	if err != nil {
		return fmt.Errorf("read Generation Request: %w", err)
	}
	request, err := agentrequest.UnmarshalRequest(content)
	if err != nil {
		return fmt.Errorf("decode Generation Request: %w", err)
	}

	// The feedback claims every fact passed, so the suite that observes them has
	// to run here. Writing "passed" without executing the command named in the
	// feedback would make the record nominal rather than measured.
	targetDir := filepath.Join(root, "experiments", "membership-agent-e2e", "target")
	if err := runTargetTests(targetDir); err != nil {
		return err
	}

	var missing, extra []string
	seen := map[string]bool{}
	feedback := agentrequest.Feedback{
		Schema: agentrequest.FeedbackSchema, Stage: "test", Status: "succeeded",
		Command: "cd experiments/membership-agent-e2e/target && go test ./...",
		Summary: "Email-verified membership added to the admin application; every Acceptance Fact is observed by a repository test.",
	}
	for _, fact := range request.AcceptanceFacts.Facts {
		key := strings.TrimPrefix(string(fact.ID), "fact/")
		references, ok := coverage[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		seen[key] = true
		feedback.FactCoverage = append(feedback.FactCoverage, agentrequest.FactCoverage{
			FactID: fact.ID, TestReferences: references, Result: "passed",
		})
	}
	for key := range coverage {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		return fmt.Errorf("fact coverage mismatch: unmapped=%v unreachable=%v", missing, extra)
	}
	sort.Slice(feedback.FactCoverage, func(i, j int) bool {
		return feedback.FactCoverage[i].FactID < feedback.FactCoverage[j].FactID
	})

	if request.ImplementationPolicy != nil {
		for _, policy := range request.ImplementationPolicy.Policies {
			entry, ok := policyEvidence[policy.ID]
			if !ok {
				return fmt.Errorf("no evidence recorded for implementation policy %s", policy.ID)
			}
			feedback.PolicyCoverage = append(feedback.PolicyCoverage, entry)
		}
		sort.Slice(feedback.PolicyCoverage, func(i, j int) bool {
			return feedback.PolicyCoverage[i].PolicyID < feedback.PolicyCoverage[j].PolicyID
		})
	}

	for _, node := range changedIntentNodes(request) {
		feedback.RelatedIntentNodes = append(feedback.RelatedIntentNodes, node)
	}

	encoded, err := json.MarshalIndent(feedback, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(out, append(encoded, '\n')); err != nil {
		return err
	}
	fmt.Printf("mapped %d facts and %d policies\n", len(feedback.FactCoverage), len(feedback.PolicyCoverage))
	return nil
}

// writeAtomic publishes the feedback with a single rename, so the file either
// holds a complete record of a passing run or does not exist.
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

// runTargetTests executes the repository suite and fails the generator when it
// does not pass, so a broken target cannot produce a passing feedback.
func runTargetTests(directory string) error {
	command := exec.Command("go", "test", "-count=1", "./...")
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("target tests failed, refusing to record passing coverage:\n%s", output)
	}
	fmt.Print(string(output))
	return nil
}

func changedIntentNodes(request agentrequest.Request) []compiler.SemanticID {
	nodes := append([]compiler.SemanticID(nil), request.RequestedChange.IntentNodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	return nodes
}
