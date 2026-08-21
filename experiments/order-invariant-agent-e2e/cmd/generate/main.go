// Command generate builds the full Generation Request used by the order and
// inventory E2E experiment. It does not generate target application code.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/horizon67/forma/internal/agentrequest"
	"github.com/horizon67/forma/internal/compiler"
)

const (
	expectedFacts   = 275
	expectedReviews = 3
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "order-invariant-agent-e2e generate:", err)
		os.Exit(1)
	}
}

func run() error {
	root := filepath.Join("..", "..")
	if _, err := os.Stat("go.mod"); err == nil {
		root = "."
	}
	requestContent, coverageContent, err := buildArtifacts(root)
	if err != nil {
		return err
	}
	experiment := filepath.Join(root, "experiments", "order-invariant-agent-e2e")
	if err := os.WriteFile(filepath.Join(experiment, "generation-request.json"), requestContent, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(experiment, "coverage.json"), coverageContent, 0o644); err != nil {
		return err
	}
	fmt.Printf("generated %d facts and %d review requirement\n", expectedFacts, expectedReviews)
	return nil
}

func buildArtifacts(root string) ([]byte, []byte, error) {
	sourcePath := filepath.Join(root, "experiments", "order-invariant-agent-e2e", "app.forma")
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read Forma source: %w", err)
	}
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("experiments/order-invariant-agent-e2e/app.forma", string(content)),
	})
	if len(result.Diagnostics) != 0 {
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintln(os.Stderr, diagnostic.Error())
		}
		return nil, nil, fmt.Errorf("Forma source has %d diagnostics", len(result.Diagnostics))
	}
	request, err := agentrequest.BuildFull(result)
	if err != nil {
		return nil, nil, fmt.Errorf("build full request: %w", err)
	}
	if got := len(request.AcceptanceFacts.Facts); got != expectedFacts {
		return nil, nil, fmt.Errorf("Acceptance Facts = %d, want %d", got, expectedFacts)
	}
	if got := len(request.ReviewRequirements.Requirements); got != expectedReviews {
		return nil, nil, fmt.Errorf("Review Requirements = %d, want %d", got, expectedReviews)
	}
	encoded, err := agentrequest.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	coverage := make(map[compiler.SemanticID][]string, len(request.AcceptanceFacts.Facts))
	for _, fact := range request.AcceptanceFacts.Facts {
		references, err := coverageForFact(fact)
		if err != nil {
			return nil, nil, err
		}
		coverage[fact.ID] = references
	}
	coverageContent, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return append(encoded, '\n'), append(coverageContent, '\n'), nil
}

const (
	storeTests = "internal/store/store_test.go#"
	webTests   = "internal/web/server_test.go#"
)

func coverageForFact(fact compiler.AcceptanceFact) ([]string, error) {
	subject := string(fact.Subject)
	switch fact.Kind {
	case "access-allowed", "access-denied":
		name, ok := accessTest(subject)
		if !ok {
			break
		}
		return []string{webTests + name}, nil
	case "relation-resolved":
		return []string{storeTests + "TestRelationsResolveToEntitiesAndLabels"}, nil
	case "invariant-satisfied", "invariant-violated":
		return []string{storeTests + "TestStockInvariantAcceptsValidAndRejectsInvalidPostStates"}, nil
	case "invariant-validation-rejected":
		return []string{
			storeTests + "TestStockValidationAndInvariantRejectAtomically",
			webTests + "TestStockItemEditSurfaceValidationInvariantAndNavigation",
		}, nil
	case "transition-accepted", "transition-source-rejected":
		if strings.Contains(subject, "StockReservation") {
			return []string{storeTests + "TestStockReservationCommitIsAtomicAcrossReservationAndStock"}, nil
		}
		name, ok := orderActionTest(subject, "Store")
		if !ok {
			break
		}
		return []string{storeTests + name}, nil
	case "action-transition-accepted", "action-transition-source-rejected":
		if strings.Contains(subject, "StockReservation") {
			return []string{webTests + "TestReservationCommitSurfaceObservesEveryAtomicOutcome"}, nil
		}
		name, ok := orderActionTest(subject, "HTTP")
		if !ok {
			break
		}
		return []string{webTests + name}, nil
	case "confirmation-required":
		if strings.Contains(subject, "StockReservation") {
			return []string{webTests + "TestReservationCommitConfirmationAndCrossEntityAuthorization"}, nil
		}
		return []string{webTests + "TestOrderActionConfirmationAcceptsOnceAndDeclinesWithoutDispatch"}, nil
	case "changes-accepted", "changes-invariant-rejected", "changes-target-unavailable":
		return []string{storeTests + "TestStockReservationCommitIsAtomicAcrossReservationAndStock"}, nil
	case "action-changes-accepted", "action-changes-invariant-rejected", "action-changes-target-unavailable":
		return []string{webTests + "TestReservationCommitSurfaceObservesEveryAtomicOutcome"}, nil
	case "action-observable-feedback":
		if strings.Contains(subject, "StockReservation") {
			return []string{
				webTests + "TestReservationCommitSurfaceObservesEveryAtomicOutcome",
				webTests + "TestReservationCommitConfirmationAndCrossEntityAuthorization",
			}, nil
		}
		if strings.Contains(subject, "/delete") {
			return []string{webTests + "TestOrderActionConfirmationAcceptsOnceAndDeclinesWithoutDispatch"}, nil
		}
		name, ok := orderActionTest(subject, "HTTP")
		if !ok {
			break
		}
		return []string{webTests + name}, nil
	case "list-search", "list-filter", "list-sort", "list-page-boundary":
		if strings.Contains(subject, "page/Orders/") {
			return []string{
				webTests + "TestOrdersListQueryHTTPBehavior",
				storeTests + "TestOrderQuerySearchFiltersSortAndPageBoundary",
			}, nil
		}
		if strings.Contains(subject, "page/StockItems/") {
			return []string{
				webTests + "TestStockItemsListQueryHTTPBehavior",
				storeTests + "TestStockQuerySearchFilterSortAndPageBoundary",
			}, nil
		}
	case "mutation-accepted", "mutation-at-most-once":
		if strings.Contains(subject, "StockItem") {
			return []string{
				webTests + "TestStockItemEditSurfaceValidationInvariantAndNavigation",
				storeTests + "TestStockMutationIsAcceptedAndAppliedAtMostOnce",
			}, nil
		}
		if strings.Contains(subject, "OrderCreate") {
			return []string{
				webTests + "TestOrderCreateSurfaceValidationMutationAndNavigation",
				storeTests + "TestOrderMutationsAreAcceptedAndAppliedAtMostOnce",
			}, nil
		}
		if strings.Contains(subject, "OrderEdit") {
			return []string{
				webTests + "TestOrderEditSurfaceValidationMutationAndNavigation",
				storeTests + "TestOrderMutationsAreAcceptedAndAppliedAtMostOnce",
			}, nil
		}
	case "validation-rejected":
		if strings.Contains(subject, "StockItem") {
			return []string{
				storeTests + "TestStockValidationAndInvariantRejectAtomically",
				webTests + "TestStockItemEditSurfaceValidationInvariantAndNavigation",
			}, nil
		}
		if strings.Contains(subject, "OrderCreate") {
			return []string{
				storeTests + "TestOrderValidationRejectsRequiredAndUnique",
				webTests + "TestOrderCreateSurfaceValidationMutationAndNavigation",
			}, nil
		}
		if strings.Contains(subject, "OrderEdit") {
			return []string{
				storeTests + "TestOrderValidationRejectsRequiredAndUnique",
				webTests + "TestOrderEditSurfaceValidationMutationAndNavigation",
			}, nil
		}
	case "navigation":
		if strings.Contains(subject, "StockReservation") {
			return []string{webTests + "TestReservationCommitSurfaceObservesEveryAtomicOutcome"}, nil
		}
		if strings.Contains(subject, "OrderCreate") {
			return []string{webTests + "TestOrderCreateSurfaceValidationMutationAndNavigation"}, nil
		}
		if strings.Contains(subject, "OrderEdit") {
			return []string{webTests + "TestOrderEditSurfaceValidationMutationAndNavigation"}, nil
		}
		if strings.Contains(subject, "page/Orders/") || strings.Contains(subject, "page/OrderDetail/") {
			return []string{webTests + "TestOrderActionNavigationFromListAndDetail"}, nil
		}
		if strings.Contains(subject, "StockItemEdit") {
			return []string{webTests + "TestStockItemEditSurfaceValidationInvariantAndNavigation"}, nil
		}
		if strings.Contains(subject, "page/StockItems/") || strings.Contains(subject, "page/StockItemDetail/") {
			return []string{webTests + "TestStockActionNavigationFromListAndDetail"}, nil
		}
	case "observable-feedback", "records-visible", "view-actions", "view-fields":
		name, ok := surfaceTest(subject)
		if ok {
			return []string{webTests + name}, nil
		}
	}
	return nil, fmt.Errorf("no repository test covers %s (%s, subject %s)", fact.ID, fact.Kind, fact.Subject)
}

func orderActionTest(subject, boundary string) (string, bool) {
	for _, action := range []string{"submit", "approve", "reject", "ship"} {
		if strings.HasSuffix(subject, "/"+action) {
			return "TestOrder" + strings.ToUpper(action[:1]) + action[1:] + "TransitionFactsAt" + boundary + "Boundary", true
		}
	}
	return "", false
}

func accessTest(subject string) (string, bool) {
	result := map[string]string{
		"page/Orders/view/list/Order":                                "TestOrdersHTTPAccess",
		"page/Orders/view/list/Order/action/create":                  "TestOrderCreateActionHTTPAccess",
		"page/OrderCreate/view/form/create/Order":                    "TestOrderCreateFormHTTPAccess",
		"page/OrderCreate/view/form/create/Order/submit":             "TestOrderCreateSubmitHTTPAccess",
		"page/Orders/view/list/Order/action/view":                    "TestOrderDetailActionHTTPAccess",
		"page/OrderDetail/view/detail/Order":                         "TestOrderDetailHTTPAccess",
		"page/Orders/view/list/Order/action/edit":                    "TestOrderEditActionsHTTPAccess",
		"page/OrderDetail/view/detail/Order/action/edit":             "TestOrderEditActionsHTTPAccess",
		"page/OrderEdit/view/form/edit/Order":                        "TestOrderEditFormHTTPAccess",
		"page/OrderEdit/view/form/edit/Order/submit":                 "TestOrderEditSubmitHTTPAccess",
		"page/Orders/view/list/Order/action/delete":                  "TestOrderDeleteActionsHTTPAccess",
		"page/OrderDetail/view/detail/Order/action/delete":           "TestOrderDeleteActionsHTTPAccess",
		"page/Orders/view/list/Order/action/submit":                  "TestOrderSubmitActionsHTTPAccess",
		"page/OrderDetail/view/detail/Order/action/submit":           "TestOrderSubmitActionsHTTPAccess",
		"page/Orders/view/list/Order/action/approve":                 "TestOrderApproveActionsHTTPAccess",
		"page/OrderDetail/view/detail/Order/action/approve":          "TestOrderApproveActionsHTTPAccess",
		"page/Orders/view/list/Order/action/reject":                  "TestOrderRejectActionsHTTPAccess",
		"page/OrderDetail/view/detail/Order/action/reject":           "TestOrderRejectActionsHTTPAccess",
		"page/Orders/view/list/Order/action/ship":                    "TestOrderShipActionsHTTPAccess",
		"page/OrderDetail/view/detail/Order/action/ship":             "TestOrderShipActionsHTTPAccess",
		"page/StockItems/view/list/StockItem":                        "TestStockItemsHTTPAccess",
		"page/StockItems/view/list/StockItem/action/view":            "TestStockItemDetailActionHTTPAccess",
		"page/StockItemDetail/view/detail/StockItem":                 "TestStockItemDetailHTTPAccess",
		"page/StockItems/view/list/StockItem/action/edit":            "TestStockItemEditActionsHTTPAccess",
		"page/StockItemDetail/view/detail/StockItem/action/edit":     "TestStockItemEditActionsHTTPAccess",
		"page/StockItemEdit/view/form/edit/StockItem":                "TestStockItemEditFormHTTPAccess",
		"page/StockItemEdit/view/form/edit/StockItem/submit":         "TestStockItemEditSubmitHTTPAccess",
		"page/Reservations/view/list/StockReservation":               "TestReservationsSurfaceFieldsActionsFeedbackAndAccess",
		"page/Reservations/view/list/StockReservation/action/commit": "TestReservationCommitConfirmationAndCrossEntityAuthorization",
	}[subject]
	return result, result != ""
}

func surfaceTest(subject string) (string, bool) {
	for marker, test := range map[string]string{
		"page/Orders/":          "TestOrdersSurfaceListsFieldsActionsAndObservableFeedback",
		"page/OrderCreate/":     "TestOrderCreateSurfaceValidationMutationAndNavigation",
		"page/OrderDetail/":     "TestOrderDetailSurfaceFieldsActionsEmptyAndFailure",
		"page/OrderEdit/":       "TestOrderEditSurfaceValidationMutationAndNavigation",
		"page/StockItems/":      "TestStockItemsSurfaceListsFieldsActionsAndObservableFeedback",
		"page/StockItemDetail/": "TestStockItemDetailSurfaceFieldsActionsEmptyAndFailure",
		"page/StockItemEdit/":   "TestStockItemEditSurfaceValidationInvariantAndNavigation",
		"page/Reservations/":    "TestReservationsSurfaceFieldsActionsFeedbackAndAccess",
	} {
		if strings.Contains(subject, marker) {
			return test, true
		}
	}
	return "", false
}
