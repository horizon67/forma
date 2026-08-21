package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/horizon67/forma/internal/implementationpolicy"
)

const (
	adminTests         = "internal/web/server_test.go#"
	memberTests        = "internal/web/membership_e2e_test.go#"
	storeTests         = "internal/store/store_test.go#"
	identityTests      = "internal/identity/identity_test.go#"
	identityStoreTests = "internal/store/identity_test.go#"
	submissionTests    = "internal/web/submission_test.go#"
	executableTests    = "cmd/server/main_test.go#"
)

// coverage lists the tests that observe each fact. Keys are fact IDs with the
// leading "fact/" removed.
var coverage = map[string][]string{
	"action/User/activate/transition/accepted/from/Pending":   {identityStoreTests + "TestActivationTransitionAcceptsPendingAndRejectsEveryOtherState"},
	"action/User/activate/transition/rejected/from/Active":    {identityStoreTests + "TestActivationTransitionAcceptsPendingAndRejectsEveryOtherState"},
	"action/User/activate/transition/rejected/from/Confirmed": {identityStoreTests + "TestActivationTransitionAcceptsPendingAndRejectsEveryOtherState"},
	"action/User/activate/transition/rejected/from/Suspended": {identityStoreTests + "TestActivationTransitionAcceptsPendingAndRejectsEveryOtherState"},
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

const coverageFingerprintLocked = "5413385c879192430e4fce3f4ff0e3763afbf7fd357a3b57257394eed4b56e07"

func coverageFingerprint() string {
	keys := make([]string, 0, len(coverage))
	for key := range coverage {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		refs := append([]string(nil), coverage[key]...)
		sort.Strings(refs)
		fmt.Fprintf(hash, "%s\t%s\n", key, strings.Join(refs, ","))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
