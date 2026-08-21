package compiler

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type FactKindContract struct {
	Kind              string
	RequiresOperation bool
	MinDispatches     int
	RequiredCases     []string
}

var identityFactKindContracts = map[string]FactKindContract{
	"access-allowed":                   {Kind: "access-allowed"},
	"identity-inputs":                  {Kind: "identity-inputs"},
	"credential-non-projectable":       {Kind: "credential-non-projectable"},
	"registration-validation-rejected": {Kind: "registration-validation-rejected", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"invalid-name", "invalid-identifier", "invalid-credential"}},
	"secret-input-not-preserved":       {Kind: "secret-input-not-preserved", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"invalid-name", "invalid-identifier", "invalid-credential"}},
	"duplicate-identifier-rejected":    {Kind: "duplicate-identifier-rejected", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"exact", "canonical-equivalent"}},
	"registration-created":             {Kind: "registration-created", RequiresOperation: true, MinDispatches: 1},
	"credential-bound":                 {Kind: "credential-bound", RequiresOperation: true, MinDispatches: 1},
	"verification-issued":              {Kind: "verification-issued", RequiresOperation: true, MinDispatches: 1},
	"notice-emitted":                   {Kind: "notice-emitted", RequiresOperation: true, MinDispatches: 1},
	"navigation":                       {Kind: "navigation", RequiresOperation: true, MinDispatches: 1},
	"operation-at-most-once":           {Kind: "operation-at-most-once", RequiresOperation: true, MinDispatches: 2},
	"verification-accepted":            {Kind: "verification-accepted", RequiresOperation: true, MinDispatches: 1},
	"verification-consumed":            {Kind: "verification-consumed", RequiresOperation: true, MinDispatches: 1},
	"verification-rejected":            {Kind: "verification-rejected", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"invalid", "expired", "consumed"}},
	"verification-resent":              {Kind: "verification-resent", RequiresOperation: true, MinDispatches: 1},
	"verification-rotated":             {Kind: "verification-rotated", RequiresOperation: true, MinDispatches: 1},
	"enumeration-safe-outcome":         {Kind: "enumeration-safe-outcome", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"unknown", "active", "pending"}},
	"authentication-ineligible-state":  {Kind: "authentication-ineligible-state", RequiresOperation: true, MinDispatches: 1},
	"authentication-accepted":          {Kind: "authentication-accepted", RequiresOperation: true, MinDispatches: 1},
	"authentication-rejected":          {Kind: "authentication-rejected", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"unknown-identifier", "non-matching-credential"}},
	"session-terminated":               {Kind: "session-terminated", RequiresOperation: true, MinDispatches: 1},
	"access-denied":                    {Kind: "access-denied"},
	"ownership-allowed":                {Kind: "ownership-allowed"},
	"ownership-denied":                 {Kind: "ownership-denied"},
	"verification-expiry-boundary":     {Kind: "verification-expiry-boundary", RequiresOperation: true, MinDispatches: 1, RequiredCases: []string{"before-expiry", "at-expiry", "after-expiry"}},
	"delivery-failure-separated":       {Kind: "delivery-failure-separated", RequiresOperation: true, MinDispatches: 1},
}

// ValidateAcceptanceFacts checks compiler-owned invariant facts and every
// emitted Identity fact in addition to the existing admin-flow derivation. A
// particular program may emit only a supported subset of the Identity Fact
// kind registry.
func ValidateAcceptanceFacts(intent *ResolvedIntent, facts *AcceptanceFacts) error {
	if intent == nil || facts == nil {
		return fmt.Errorf("validate Acceptance Facts: intent and facts are required")
	}
	if facts.Version != AcceptanceFactsVersion || facts.IntentVersion != intent.Version {
		return fmt.Errorf("validate Acceptance Facts: schema versions do not match Resolved Intent")
	}
	semanticIDs, err := resolvedIntentSemanticIDs(intent)
	if err != nil {
		return fmt.Errorf("validate Acceptance Facts: %w", err)
	}
	invariants := map[SemanticID]IRInvariant{}
	for _, entity := range intent.Entities {
		for _, invariant := range entity.Invariants {
			invariants[invariant.ID] = invariant
		}
	}
	invariantMutationFacts, err := expectedInvariantMutationFacts(intent)
	if err != nil {
		return err
	}
	invariantMutationByID := make(map[SemanticID]invariantMutationFact, len(invariantMutationFacts))
	for _, expected := range invariantMutationFacts {
		invariantMutationByID[expected.ID] = expected
	}
	seenFacts := map[SemanticID]bool{}
	for _, fact := range facts.Facts {
		if fact.ID == "" || seenFacts[fact.ID] {
			return fmt.Errorf("validate Acceptance Facts: fact has an empty or duplicate ID %s", fact.ID)
		}
		seenFacts[fact.ID] = true
		if err := validateAcceptanceFactReferences(intent, fact, semanticIDs); err != nil {
			return err
		}
		if !semanticIDs[fact.Subject] {
			return fmt.Errorf("validate Acceptance Facts: fact %s has missing subject %s", fact.ID, fact.Subject)
		}
		if len(fact.SourceNodes) == 0 || !containsSemanticID(fact.SourceNodes, fact.Subject) {
			return fmt.Errorf("validate Acceptance Facts: fact %s sourceNodes omit its subject", fact.ID)
		}
		if !reflect.DeepEqual(fact.SourceNodes, canonicalSemanticIDs(fact.SourceNodes)) {
			return fmt.Errorf("validate Acceptance Facts: fact %s sourceNodes are not canonical", fact.ID)
		}
		for _, source := range fact.SourceNodes {
			if !semanticIDs[source] {
				return fmt.Errorf("validate Acceptance Facts: fact %s references missing source node %s", fact.ID, source)
			}
		}
		if isIdentityFact(fact) {
			if len(intent.Identities) == 0 {
				return fmt.Errorf("validate Acceptance Facts: Identity fact %s has no Identity intent", fact.ID)
			}
			if err := ValidateFactSetup(fact); err != nil {
				return err
			}
		}
		if fact.Input != nil && fact.Input.Action != nil {
			if err := validateActionFactHandles(intent, fact); err != nil {
				return err
			}
		}
		invariantKind := fact.Kind == "invariant-satisfied" || fact.Kind == "invariant-violated" ||
			fact.Kind == "invariant-validation-rejected"
		invariantPayload := fact.Input != nil && fact.Input.Predicate != nil
		if invariantKind != invariantPayload {
			return fmt.Errorf("validate Acceptance Facts: fact %s has a mismatched invariant kind and predicate payload", fact.ID)
		}
		if fact.Kind == "invariant-satisfied" || fact.Kind == "invariant-violated" {
			if err := validateInvariantFact(fact, invariants); err != nil {
				return err
			}
		} else if fact.Kind == "invariant-validation-rejected" {
			expected, ok := invariantMutationByID[fact.ID]
			if !ok {
				return fmt.Errorf("validate Acceptance Facts: invariant mutation fact %s is not derived from Resolved Intent", fact.ID)
			}
			if err := validateInvariantMutationFact(fact, expected); err != nil {
				return err
			}
		}
	}
	for _, invariant := range invariants {
		for _, caseName := range []string{"satisfied", "violated"} {
			id := factID(invariant.ID, "evaluation", caseName)
			if !seenFacts[id] {
				return fmt.Errorf("validate Acceptance Facts: missing invariant evaluation fact %s", id)
			}
		}
	}
	for _, expected := range invariantMutationFacts {
		if !seenFacts[expected.ID] {
			return fmt.Errorf("validate Acceptance Facts: missing invariant mutation fact %s", expected.ID)
		}
	}
	if err := validateVerificationRejectionReachability(facts.Facts); err != nil {
		return err
	}
	canonical, err := deriveActionContractFacts(intent)
	if err != nil {
		return err
	}
	return validateCanonicalActionFacts(facts, canonical)
}

func validateCanonicalActionFacts(actual *AcceptanceFacts, canonical []AcceptanceFact) error {
	want := make(map[SemanticID]AcceptanceFact)
	for _, fact := range canonical {
		want[fact.ID] = fact
	}
	seen := make(map[SemanticID]bool, len(want))
	for _, fact := range actual.Facts {
		if !isActionContractFact(fact) {
			continue
		}
		seen[fact.ID] = true
		expected, ok := want[fact.ID]
		if !ok {
			return fmt.Errorf("validate Acceptance Facts: fact %s is not derived from Resolved Intent", fact.ID)
		}
		if !reflect.DeepEqual(fact, expected) {
			return fmt.Errorf("validate Acceptance Facts: fact %s differs from its canonical derivation", fact.ID)
		}
	}
	for id := range want {
		if !seen[id] {
			return fmt.Errorf("validate Acceptance Facts: missing compiler-derived fact %s", id)
		}
	}
	return nil
}

func isActionContractFact(fact AcceptanceFact) bool {
	return fact.Kind == "action-observable-feedback" || fact.Input != nil && fact.Input.Action != nil
}

func validateActionFactHandles(intent *ResolvedIntent, fact AcceptanceFact) error {
	if err := validateFactSetupHandles(fact.Setup); err != nil {
		return fmt.Errorf("validate action Fact %s: %w", fact.ID, err)
	}
	action := fact.Input.Action
	if action.Subject == "" || !setupHasSubject(fact.Setup, action.Subject) {
		return fmt.Errorf("validate action Fact %s: action subject handle %q is not established", fact.ID, action.Subject)
	}
	for _, subject := range fact.Expected.Subjects {
		if !setupHasSubject(fact.Setup, subject.Handle) {
			return fmt.Errorf("validate action Fact %s: expected subject handle %q is not established", fact.ID, subject.Handle)
		}
		for _, field := range subject.Fields {
			if err := validateFactExpressionExpectation(intent, fact, field); err != nil {
				return fmt.Errorf("validate action Fact %s: field expectation for %s is not closed", fact.ID, field.Field)
			}
		}
	}
	return nil
}

func validateFactExpressionExpectation(intent *ResolvedIntent, fact AcceptanceFact, field FactFieldExpectation) error {
	if field.Stored != "expression-result" || field.Expression == nil || field.Expression.Evaluation != "pre-state" {
		return fmt.Errorf("missing expression-result contract")
	}
	var resolvedExpression *IRExpression
	for _, action := range intent.Actions {
		if action.ID != fact.Input.Action.Action {
			continue
		}
		for _, change := range action.Changes {
			if change.Target.Field == field.Field {
				resolvedExpression = &change.Value
				break
			}
		}
	}
	if resolvedExpression == nil || !reflect.DeepEqual(field.Expression.Tree, *resolvedExpression) {
		return fmt.Errorf("expression tree differs from Resolved Intent")
	}
	leaves := expressionFieldReferenceNodes(field.Expression.Tree)
	if len(leaves) == 0 || len(leaves) != len(field.Expression.Bindings) {
		return fmt.Errorf("expression leaf bindings are incomplete")
	}
	for index, leaf := range leaves {
		binding := field.Expression.Bindings[index]
		if binding.Node != leaf.ID || !setupHasSubject(fact.Setup, binding.Subject) {
			return fmt.Errorf("expression leaf %s has a non-canonical subject binding", leaf.ID)
		}
		wantSubject := "subject/action"
		if len(leaf.RelationPath) == 1 {
			wantSubject = ""
			if fact.Setup != nil {
				for _, relation := range fact.Setup.Relations {
					if relation.Field == leaf.RelationPath[0] && relation.Condition == "resolved" {
						wantSubject = relation.Target
						break
					}
				}
			}
		}
		if binding.Subject != wantSubject || wantSubject == "" {
			return fmt.Errorf("expression leaf %s does not follow its runtime relation binding", leaf.ID)
		}
	}
	return nil
}

type invariantMutationFact struct {
	ID          SemanticID
	View        IRView
	Submit      IRSubmitIntent
	Invariant   IRInvariant
	FormFields  []SemanticID
	InputFields []SemanticID
}

func expectedInvariantMutationFacts(intent *ResolvedIntent) ([]invariantMutationFact, error) {
	entities := make(map[string]IREntity, len(intent.Entities))
	for _, entity := range intent.Entities {
		entities[entity.Name] = entity
	}
	var result []invariantMutationFact
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			if view.Submit == nil {
				continue
			}
			entity, ok := entities[view.Entity]
			if !ok {
				return nil, fmt.Errorf("validate Acceptance Facts: view %s references missing entity %s", view.ID, view.Entity)
			}
			formFields, err := fieldIDs(entity, view.Fields)
			if err != nil {
				return nil, fmt.Errorf("validate Acceptance Facts for %s: %w", view.ID, err)
			}
			for _, invariant := range entity.Invariants {
				inputFields := intersectSemanticIDs(formFields, invariantFieldReferences(invariant))
				if len(inputFields) == 0 {
					continue
				}
				result = append(result, invariantMutationFact{
					ID: invariantValidationFactID(view.Submit.ID, invariant.ID), View: view,
					Submit: *view.Submit, Invariant: invariant, FormFields: formFields, InputFields: inputFields,
				})
			}
		}
	}
	return result, nil
}

func validateInvariantMutationFact(fact AcceptanceFact, expected invariantMutationFact) error {
	wantInput := FactInput{
		Fields: expected.InputFields,
		Predicate: &FactPredicateInput{
			Expression: expected.Invariant.Predicate, Evaluation: "post-state",
			OtherRequirements: "satisfied", Result: false,
		},
	}
	wantExpectation := FactExpectation{
		Outcome: "rejected", Feedback: []string{"invalid"}, Enforcement: "authoritative",
		Atomicity: "no-changes-committed", Stored: "unchanged", PreserveInput: expected.FormFields,
	}
	sources := append([]SemanticID{expected.View.ID, expected.Submit.ID}, expected.FormFields...)
	sources = append(sources, invariantFactSourceNodes(expected.Invariant)...)
	if fact.Subject != expected.Submit.ID || fact.Principal != nil || fact.Setup != nil ||
		fact.Input == nil || !reflect.DeepEqual(*fact.Input, wantInput) {
		return fmt.Errorf("validate Acceptance Facts: invariant mutation fact %s has non-canonical input or subject", fact.ID)
	}
	if !reflect.DeepEqual(fact.Expected, wantExpectation) {
		return fmt.Errorf("validate Acceptance Facts: invariant mutation fact %s has a non-canonical expectation", fact.ID)
	}
	if !reflect.DeepEqual(fact.SourceNodes, canonicalSemanticIDs(sources)) {
		return fmt.Errorf("validate Acceptance Facts: invariant mutation fact %s has incomplete mutation provenance", fact.ID)
	}
	return nil
}

func validateInvariantFact(fact AcceptanceFact, invariants map[SemanticID]IRInvariant) error {
	invariant, ok := invariants[fact.Subject]
	if !ok {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s has no invariant subject", fact.ID)
	}
	if fact.Input == nil || fact.Input.Predicate == nil {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s has no predicate input", fact.ID)
	}
	input := fact.Input.Predicate
	if input.Evaluation != "post-state" || input.OtherRequirements != "satisfied" || !reflect.DeepEqual(input.Expression, invariant.Predicate) {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s differs from its resolved post-state predicate", fact.ID)
	}
	caseName := "satisfied"
	wantKind := "invariant-satisfied"
	wantExpectation := FactExpectation{
		Outcome: "accepted", Enforcement: "authoritative", Atomicity: "all-changes-committed",
	}
	if !input.Result {
		caseName = "violated"
		wantKind = "invariant-violated"
		wantExpectation = FactExpectation{
			Outcome: "rejected", Enforcement: "authoritative", Atomicity: "no-changes-committed",
		}
	}
	wantInput := FactInput{Predicate: &FactPredicateInput{
		Expression: invariant.Predicate, Evaluation: "post-state",
		OtherRequirements: "satisfied", Result: input.Result,
	}}
	if !reflect.DeepEqual(*fact.Input, wantInput) {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s has a non-canonical predicate input", fact.ID)
	}
	if fact.ID != factID(invariant.ID, "evaluation", caseName) || fact.Kind != wantKind {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s has non-canonical identity or kind", fact.ID)
	}
	if fact.Principal != nil || fact.Setup != nil || !reflect.DeepEqual(fact.Expected, wantExpectation) {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s has a non-canonical expectation", fact.ID)
	}
	if !reflect.DeepEqual(fact.SourceNodes, invariantFactSourceNodes(invariant)) {
		return fmt.Errorf("validate Acceptance Facts: invariant fact %s has incomplete predicate provenance", fact.ID)
	}
	return nil
}

// validateVerificationRejectionReachability pins the consumed rejection case to
// the state the successful verification actually reaches. Consumed evidence
// exists only because that transition already ran, so the case must start and
// end in the accepted fact's destination state; merely differing from the
// pre-verification state would still allow an unreachable one.
func validateVerificationRejectionReachability(facts []AcceptanceFact) error {
	success := map[SemanticID]*IRStateValueRef{}
	for _, fact := range facts {
		if fact.Kind != "verification-accepted" || fact.Expected.Identity == nil || fact.Expected.Identity.Subject == nil {
			continue
		}
		success[fact.Subject] = fact.Expected.Identity.Subject.State
	}
	for _, fact := range facts {
		if fact.Kind != "verification-rejected" || fact.Input == nil ||
			fact.Input.Identity == nil || fact.Expected.Identity == nil {
			continue
		}
		reached, ok := success[fact.Subject]
		if !ok || reached == nil {
			return fmt.Errorf(
				"validate Acceptance Facts: fact %s has no accepted verification declaring the state a consumed case starts from", fact.ID)
		}
		for _, item := range fact.Input.Identity.Cases {
			if item.Kind != "consumed" {
				continue
			}
			if !stateRefEquals(setupSubjectStateRef(item.Setup), reached) {
				return fmt.Errorf(
					"validate Acceptance Facts: fact %s consumed case starts in %s, but a successful verification reaches %s",
					fact.ID, describeState(setupSubjectStateRef(item.Setup)), describeState(reached))
			}
		}
		for _, expectation := range fact.Expected.Identity.Cases {
			if expectation.Kind != "consumed" {
				continue
			}
			if !stateRefEquals(expectation.SubjectState, reached) {
				return fmt.Errorf(
					"validate Acceptance Facts: fact %s consumed case expects %s, but a rejection must leave the subject in %s",
					fact.ID, describeState(expectation.SubjectState), describeState(reached))
			}
		}
	}
	return nil
}

func setupSubjectStateRef(setup *FactSetup) *IRStateValueRef {
	if setup == nil {
		return nil
	}
	for _, subject := range setup.Subjects {
		if subject.State != nil {
			return subject.State
		}
	}
	return nil
}

func stateRefEquals(left, right *IRStateValueRef) bool {
	if left == nil || right == nil {
		return false
	}
	return left.State == right.State && left.Value == right.Value
}

func describeState(ref *IRStateValueRef) string {
	if ref == nil {
		return "no state"
	}
	return string(ref.State) + "=" + ref.Value
}

func isIdentityFact(fact AcceptanceFact) bool {
	return fact.Input != nil && fact.Input.Identity != nil || fact.Expected.Identity != nil
}

// ValidateFactSetup enforces the closed handle vocabulary and prevents the
// compiler-produced setup from pre-installing the outcome under test.
func ValidateFactSetup(fact AcceptanceFact) error {
	if fact.Input == nil || fact.Input.Identity == nil || fact.Expected.Identity == nil {
		return fmt.Errorf("validate Identity Fact %s: input and expectation payloads are required", fact.ID)
	}
	contract, ok := identityFactKindContracts[fact.Kind]
	if !ok {
		return fmt.Errorf("validate Identity Fact %s: kind %q has no contract", fact.ID, fact.Kind)
	}
	input := fact.Input.Identity
	if contract.RequiresOperation && input.Operation == "" {
		return fmt.Errorf("validate Identity Fact %s: operation is required", fact.ID)
	}
	if len(input.Observe) == 0 {
		return fmt.Errorf("validate Identity Fact %s: observation boundary is required", fact.ID)
	}
	if !reflect.DeepEqual(input.Observe, canonicalStrings(input.Observe)) {
		return fmt.Errorf("validate Identity Fact %s: observations are not canonical", fact.ID)
	}
	for _, observation := range input.Observe {
		if !identityFactObservations[observation] {
			return fmt.Errorf("validate Identity Fact %s: unsupported observation %q", fact.ID, observation)
		}
	}
	if len(contract.RequiredCases) == 0 {
		if len(input.Cases) != 0 {
			return fmt.Errorf("validate Identity Fact %s: kind %s does not accept cases", fact.ID, fact.Kind)
		}
		if contract.MinDispatches > 0 && input.Dispatches < contract.MinDispatches {
			return fmt.Errorf("validate Identity Fact %s: requires at least %d dispatches", fact.ID, contract.MinDispatches)
		}
	} else {
		caseKinds := make([]string, len(input.Cases))
		for index, item := range input.Cases {
			caseKinds[index] = item.Kind
			if item.Dispatches < contract.MinDispatches {
				return fmt.Errorf("validate Identity Fact %s case %s: requires at least %d dispatches", fact.ID, item.Kind, contract.MinDispatches)
			}
		}
		if !reflect.DeepEqual(caseKinds, contract.RequiredCases) {
			return fmt.Errorf("validate Identity Fact %s: cases %v differ from contract %v", fact.ID, caseKinds, contract.RequiredCases)
		}
		if len(fact.Expected.Identity.Cases) != len(contract.RequiredCases) {
			return fmt.Errorf("validate Identity Fact %s: expectation cases are incomplete", fact.ID)
		}
		for index, expected := range fact.Expected.Identity.Cases {
			if expected.Kind != contract.RequiredCases[index] {
				return fmt.Errorf("validate Identity Fact %s: expectation cases are not canonical", fact.ID)
			}
		}
	}
	if err := validateFactSetupHandles(fact.Setup); err != nil {
		return fmt.Errorf("validate Identity Fact %s: %w", fact.ID, err)
	}
	for _, item := range input.Cases {
		if err := validateFactSetupHandles(item.Setup); err != nil {
			return fmt.Errorf("validate Identity Fact %s case %s: %w", fact.ID, item.Kind, err)
		}
		if err := validateIdentityCaseHandles(item); err != nil {
			return fmt.Errorf("validate Identity Fact %s case %s: %w", fact.ID, item.Kind, err)
		}
	}
	if err := validateIdentityInputHandles(*input, fact.Setup); err != nil {
		return fmt.Errorf("validate Identity Fact %s: %w", fact.ID, err)
	}
	if err := validatePrincipalSetup(fact.Principal, fact.Setup); err != nil {
		return fmt.Errorf("validate Identity Fact %s: %w", fact.ID, err)
	}
	if credential := fact.Expected.Identity.Credential; credential != nil && !oneOf(credential.Condition, "absent", "satisfies-policy", "unchanged") {
		return fmt.Errorf("validate Identity Fact %s: unsupported credential expectation %q", fact.ID, credential.Condition)
	}
	if evidence := fact.Expected.Identity.Evidence; evidence != nil {
		if evidence.Condition != "" && !oneOf(evidence.Condition, "issued", "consumed", "superseded") {
			return fmt.Errorf("validate Identity Fact %s: unsupported evidence expectation %q", fact.ID, evidence.Condition)
		}
		if negativeCount(evidence.Count) || negativeCount(evidence.Added) {
			return fmt.Errorf("validate Identity Fact %s: evidence counts cannot be negative", fact.ID)
		}
	}
	if session := fact.Expected.Identity.Session; session != nil && !oneOf(session.Condition, "active", "absent", "terminated") {
		return fmt.Errorf("validate Identity Fact %s: unsupported session expectation %q", fact.ID, session.Condition)
	}
	if notice := fact.Expected.Identity.Notice; notice != nil {
		if notice.Delivery != "" && !oneOf(notice.Delivery, "succeeded", "failed") {
			return fmt.Errorf("validate Identity Fact %s: unsupported delivery expectation %q", fact.ID, notice.Delivery)
		}
		if negativeCount(notice.Count) || negativeCount(notice.Added) {
			return fmt.Errorf("validate Identity Fact %s: notice counts cannot be negative", fact.ID)
		}
	}
	return validateFactNonSelfFulfillment(fact)
}

var identityFactObservations = map[string]bool{
	"access": true, "resolved-inputs": true, "resolved-intent-projections": true, "artifact-projections": true,
	"subject-count": true, "credential-binding-count": true, "evidence-count": true, "notice-emission-count": true,
	"redisplayed-inputs": true, "authentication-result": true, "navigation": true, "continuation": true,
	"subject-state": true, "evidence-condition": true, "user-visible-outcome": true, "session-count": true,
	"session-condition": true, "protected-access": true, "view-access": true, "edit-access": true,
	"delivery-result": true, "resend-availability": true,
}

func validateFactKindContractCoverage(kinds map[string]bool) error {
	var missing, extra []string
	for kind := range kinds {
		if _, ok := identityFactKindContracts[kind]; !ok {
			missing = append(missing, kind)
		}
	}
	for kind := range identityFactKindContracts {
		if !kinds[kind] {
			extra = append(extra, kind)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		return fmt.Errorf("validate Acceptance Facts: Identity kind registry mismatch: undefined=%v unreachable=%v", missing, extra)
	}
	return nil
}

func validateAcceptanceFactReferences(intent *ResolvedIntent, fact AcceptanceFact, semanticIDs map[SemanticID]bool) error {
	var visit func(reflect.Value, bool) error
	visit = func(value reflect.Value, rootFact bool) error {
		if !value.IsValid() {
			return nil
		}
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return nil
			}
			return visit(value.Elem(), false)
		}
		switch value.Kind() {
		case reflect.Struct:
			typeOf := value.Type()
			for index := 0; index < value.NumField(); index++ {
				fieldInfo := typeOf.Field(index)
				field := value.Field(index)
				if rootFact && fieldInfo.Name == "ID" {
					continue
				}
				if field.Type() == reflect.TypeOf(SemanticID("")) {
					id := field.Interface().(SemanticID)
					if id != "" && !semanticIDs[id] {
						return fmt.Errorf("validate Acceptance Facts: fact %s references missing semantic node %s", fact.ID, id)
					}
					continue
				}
				if err := visit(field, false); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for index := 0; index < value.Len(); index++ {
				if err := visit(value.Index(index), false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(reflect.ValueOf(fact), true); err != nil {
		return err
	}
	states := map[SemanticID]IRState{}
	for _, entity := range intent.Entities {
		if entity.State != nil {
			states[entity.State.ID] = *entity.State
		}
	}
	validateState := func(ref *IRStateValueRef) error {
		if ref == nil {
			return nil
		}
		state, ok := states[ref.State]
		if !ok || !oneOf(ref.Value, state.Values...) {
			return fmt.Errorf("validate Acceptance Facts: fact %s has invalid state value %s:%s", fact.ID, ref.State, ref.Value)
		}
		return nil
	}
	setups := []*FactSetup{fact.Setup}
	if fact.Input != nil && fact.Input.Identity != nil {
		for _, item := range fact.Input.Identity.Cases {
			setups = append(setups, item.Setup)
		}
	}
	for _, setup := range setups {
		if setup == nil {
			continue
		}
		for _, subject := range setup.Subjects {
			if err := validateState(subject.State); err != nil {
				return err
			}
		}
	}
	if fact.Expected.Identity != nil {
		if fact.Expected.Identity.Subject != nil {
			if err := validateState(fact.Expected.Identity.Subject.State); err != nil {
				return err
			}
		}
		for _, item := range fact.Expected.Identity.Cases {
			if err := validateState(item.SubjectState); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFactSetupHandles(setup *FactSetup) error {
	if setup == nil {
		return nil
	}
	if !sort.SliceIsSorted(setup.Subjects, func(i, j int) bool { return setup.Subjects[i].Handle < setup.Subjects[j].Handle }) ||
		!sort.SliceIsSorted(setup.Relations, func(i, j int) bool {
			left, right := setup.Relations[i], setup.Relations[j]
			if left.Source != right.Source {
				return left.Source < right.Source
			}
			return left.Field < right.Field
		}) ||
		!sort.SliceIsSorted(setup.Evidence, func(i, j int) bool { return setup.Evidence[i].Handle < setup.Evidence[j].Handle }) ||
		!sort.SliceIsSorted(setup.Sessions, func(i, j int) bool { return setup.Sessions[i].Handle < setup.Sessions[j].Handle }) {
		return fmt.Errorf("setup handles are not canonical")
	}
	subjects := map[string]bool{}
	handles := map[string]bool{}
	for _, subject := range setup.Subjects {
		if !validSubjectHandle(subject.Handle) || handles[subject.Handle] || subject.Identity == "" {
			return fmt.Errorf("invalid or duplicate subject handle %q", subject.Handle)
		}
		handles[subject.Handle] = true
		subjects[subject.Handle] = true
		if !sort.SliceIsSorted(subject.Credentials, func(i, j int) bool { return subject.Credentials[i].Handle < subject.Credentials[j].Handle }) {
			return fmt.Errorf("credential handles for %s are not canonical", subject.Handle)
		}
		for _, credential := range subject.Credentials {
			if !strings.HasPrefix(credential.Handle, subject.Handle+"/credential/") || handles[credential.Handle] || credential.Credential == "" || credential.Condition != "satisfies-policy" {
				return fmt.Errorf("invalid credential binding handle %q", credential.Handle)
			}
			handles[credential.Handle] = true
		}
	}
	for _, relation := range setup.Relations {
		if !subjects[relation.Source] || relation.Field == "" || !oneOf(relation.Condition, "resolved", "target-unavailable", "value-unavailable") {
			return fmt.Errorf("relation setup has invalid source or condition")
		}
		if relation.Condition == "resolved" {
			if !subjects[relation.Target] {
				return fmt.Errorf("resolved relation target %q is not established", relation.Target)
			}
		} else if relation.Target != "" {
			return fmt.Errorf("unavailable relation unexpectedly establishes target %q", relation.Target)
		}
	}
	evidence := map[string]bool{}
	for _, item := range setup.Evidence {
		if !subjects[item.Subject] || !strings.HasPrefix(item.Handle, item.Subject+"/evidence/") || handles[item.Handle] || item.Verification == "" || !oneOf(item.Condition, "issued", "consumed", "superseded") {
			return fmt.Errorf("invalid evidence handle %q", item.Handle)
		}
		handles[item.Handle] = true
		evidence[item.Handle] = true
	}
	for _, item := range setup.Sessions {
		if !subjects[item.Subject] || !strings.HasPrefix(item.Handle, item.Subject+"/session/") || handles[item.Handle] || item.Session == "" || !oneOf(item.Condition, "active", "terminated") {
			return fmt.Errorf("invalid session handle %q", item.Handle)
		}
		handles[item.Handle] = true
	}
	if setup.Clock != nil {
		if !evidence[setup.Clock.Evidence] || !oneOf(setup.Clock.Relation, "before-expiry", "at-expiry", "after-expiry") {
			return fmt.Errorf("clock must reference issued semantic evidence with a closed relation")
		}
	}
	if setup.Delivery != nil && (setup.Delivery.Notice == "" || !oneOf(setup.Delivery.Condition, "succeeds", "fails")) {
		return fmt.Errorf("delivery setup has an invalid condition")
	}
	return nil
}

func validateIdentityInputHandles(input IdentityFactInput, setup *FactSetup) error {
	if input.Subject != "" && !setupHasSubject(setup, input.Subject) && input.Subject != "subject/created" {
		return fmt.Errorf("input subject handle %q is not established", input.Subject)
	}
	if input.Identifier != nil && !validIdentifierInput(*input.Identifier, setup) {
		return fmt.Errorf("invalid identifier input handle %q", input.Identifier.Handle)
	}
	if input.Credential != nil && !validCredentialInput(*input.Credential, setup) {
		return fmt.Errorf("invalid credential input binding %q", input.Credential.Binding)
	}
	if input.Evidence != "" && !setupHasEvidence(setup, input.Evidence) && input.Evidence != "input/evidence/invalid" {
		return fmt.Errorf("input evidence handle %q is not established", input.Evidence)
	}
	if input.Session != "" && !setupHasSession(setup, input.Session) {
		return fmt.Errorf("input session handle %q is not established", input.Session)
	}
	if input.Resource != "" && !setupHasSubject(setup, input.Resource) {
		return fmt.Errorf("resource handle %q is not established", input.Resource)
	}
	if input.Delivery != "" && !oneOf(input.Delivery, "succeeds", "fails") {
		return fmt.Errorf("invalid delivery input %q", input.Delivery)
	}
	return nil
}

func validateIdentityCaseHandles(item IdentityFactCase) error {
	if item.Identifier != nil && !validIdentifierInput(*item.Identifier, item.Setup) {
		return fmt.Errorf("invalid identifier input handle %q", item.Identifier.Handle)
	}
	if item.Credential != nil && !validCredentialInput(*item.Credential, item.Setup) {
		return fmt.Errorf("invalid credential input binding %q", item.Credential.Binding)
	}
	if item.Evidence != "" && !setupHasEvidence(item.Setup, item.Evidence) && item.Evidence != "input/evidence/invalid" {
		return fmt.Errorf("evidence handle %q is not established", item.Evidence)
	}
	if item.Session != "" && !setupHasSession(item.Setup, item.Session) {
		return fmt.Errorf("session handle %q is not established", item.Session)
	}
	if item.Resource != "" && !setupHasSubject(item.Setup, item.Resource) {
		return fmt.Errorf("resource handle %q is not established", item.Resource)
	}
	if item.Clock != "" && (item.Setup == nil || item.Setup.Clock == nil || item.Clock != item.Setup.Clock.Relation) {
		return fmt.Errorf("clock input is not established by case setup")
	}
	if item.Delivery != "" && (item.Setup == nil || item.Setup.Delivery == nil || item.Delivery != item.Setup.Delivery.Condition) {
		return fmt.Errorf("delivery input is not established by case setup")
	}
	return nil
}

func validIdentifierInput(input FactIdentifierInput, setup *FactSetup) bool {
	if input.Identifier == "" || !oneOf(input.Relation, "matching", "exact", "canonical-equivalent", "unknown", "invalid") {
		return false
	}
	if strings.HasPrefix(input.Handle, "input/identifier/") {
		return oneOf(input.Handle, "input/identifier/invalid", "input/identifier/unknown", "input/identifier/canonical-equivalent")
	}
	parts := strings.Split(input.Handle, "/")
	return len(parts) == 4 && parts[0] == "subject" && parts[2] == "identifier" && setupHasSubject(setup, strings.Join(parts[:2], "/"))
}

func validCredentialInput(input FactCredentialInput, setup *FactSetup) bool {
	if input.Credential == "" || !oneOf(input.Relation, "matching", "non-matching", "violates-policy") {
		return false
	}
	if input.Binding == "input/credential/invalid" {
		return input.Relation == "violates-policy"
	}
	return setupHasCredential(setup, input.Binding)
}

func validatePrincipalSetup(principal *FactPrincipal, setup *FactSetup) error {
	if principal == nil || principal.Kind == "roles" {
		return nil
	}
	switch principal.Kind {
	case "anonymous":
		if principal.Identity != "" || principal.Subject != "" || principal.Session != "" {
			return fmt.Errorf("anonymous principal contains Identity state")
		}
	case "authenticated":
		if principal.Identity == "" || !setupHasSubject(setup, principal.Subject) || !setupHasSession(setup, principal.Session) {
			return fmt.Errorf("authenticated principal is not established by setup")
		}
	default:
		return fmt.Errorf("unsupported principal kind %q", principal.Kind)
	}
	return nil
}

func validateFactNonSelfFulfillment(fact AcceptanceFact) error {
	setups := []*FactSetup{fact.Setup}
	for _, item := range fact.Input.Identity.Cases {
		setups = append(setups, item.Setup)
	}
	switch fact.Kind {
	case "registration-validation-rejected", "secret-input-not-preserved", "registration-created", "credential-bound", "verification-issued", "notice-emitted":
		if anyNonEmptySetup(setups) {
			return fmt.Errorf("validate Identity Fact %s: setup pre-installs a fresh registration result", fact.ID)
		}
	case "duplicate-identifier-rejected":
		// The prior registration has to be the state a registration actually
		// leaves behind: one subject holding one credential and one issued
		// evidence. Demanding a subject with no evidence would describe a state
		// no run can reach, and demanding two subjects would pre-install the
		// outcome. Self-fulfillment is instead ruled out below by requiring the
		// expectation to be stated as growth from that starting point.
		for _, setup := range setups[1:] {
			if setup == nil || len(setup.Subjects) != 1 || len(setup.Subjects[0].Credentials) != 1 || len(setup.Sessions) != 0 || setup.Delivery != nil {
				return fmt.Errorf("validate Identity Fact %s: duplicate case setup is not a single credentialed subject", fact.ID)
			}
			if len(setup.Evidence) != 1 || setup.Evidence[0].Subject != setup.Subjects[0].Handle || setup.Evidence[0].Condition != "issued" {
				return fmt.Errorf("validate Identity Fact %s: duplicate case setup omits the evidence the prior registration issued", fact.ID)
			}
		}
		expected := fact.Expected.Identity
		if expected.Evidence == nil || expected.Evidence.Added == nil || *expected.Evidence.Added != 0 {
			return fmt.Errorf("validate Identity Fact %s: duplicate rejection must expect no evidence added to the existing registration", fact.ID)
		}
		if expected.Notice == nil || expected.Notice.Added == nil || *expected.Notice.Added != 0 {
			return fmt.Errorf("validate Identity Fact %s: duplicate rejection must expect no notice added to the existing registration", fact.ID)
		}
	case "operation-at-most-once":
		if fact.Input.Identity.Dispatches < 2 || fact.Expected.Identity.AppliedOperations != 1 {
			return fmt.Errorf("validate Identity Fact %s: at-most-once requires repeated dispatch and one applied operation", fact.ID)
		}
	case "verification-accepted":
		if !setupHasState(fact.Setup, "Pending") || setupHasState(fact.Setup, "Active") || !setupHasEvidenceCondition(fact.Setup, "issued") || setupHasEvidenceCondition(fact.Setup, "consumed") || !setupHasClock(fact.Setup, "before-expiry") {
			return fmt.Errorf("validate Identity Fact %s: verification success setup pre-installs or omits its precondition", fact.ID)
		}
	case "verification-consumed":
		if !setupHasEvidenceCondition(fact.Setup, "issued") || setupHasEvidenceCondition(fact.Setup, "consumed") {
			return fmt.Errorf("validate Identity Fact %s: consumed evidence is pre-installed", fact.ID)
		}
	case "verification-rejected":
		if len(fact.Input.Identity.Cases) != 3 {
			return fmt.Errorf("validate Identity Fact %s: rejection cases are incomplete", fact.ID)
		}
	case "verification-resent":
		evidence := fact.Expected.Identity.Evidence
		notice := fact.Expected.Identity.Notice
		if !setupHasState(fact.Setup, "Pending") || len(fact.Setup.Evidence) != 1 || !setupHasEvidenceCondition(fact.Setup, "issued") || setupHasEvidenceCondition(fact.Setup, "superseded") || fact.Setup.Delivery != nil {
			return fmt.Errorf("validate Identity Fact %s: resend lacks exactly one prior issued evidence", fact.ID)
		}
		if evidence == nil || !countEquals(evidence.Count, 2) || !countEquals(evidence.Added, 1) || notice == nil || !countEquals(notice.Added, 1) {
			return fmt.Errorf("validate Identity Fact %s: resend must add one evidence and one notice emission", fact.ID)
		}
	case "verification-rotated":
		if !setupHasEvidenceCondition(fact.Setup, "issued") || setupHasEvidenceCondition(fact.Setup, "superseded") {
			return fmt.Errorf("validate Identity Fact %s: rotated evidence is pre-installed", fact.ID)
		}
	case "authentication-ineligible-state":
		if !setupHasState(fact.Setup, "Pending") || setupHasSessionCondition(fact.Setup, "active") {
			return fmt.Errorf("validate Identity Fact %s: ineligible authentication result is pre-installed", fact.ID)
		}
	case "authentication-accepted":
		if !setupHasState(fact.Setup, "Active") || setupHasSessionCondition(fact.Setup, "active") {
			return fmt.Errorf("validate Identity Fact %s: authenticated session is pre-installed", fact.ID)
		}
	case "authentication-rejected":
		for _, setup := range setups {
			if setupHasSessionCondition(setup, "active") {
				return fmt.Errorf("validate Identity Fact %s: rejected authentication has a pre-installed session", fact.ID)
			}
		}
	case "session-terminated":
		if !setupHasSessionCondition(fact.Setup, "active") || setupHasSessionCondition(fact.Setup, "terminated") {
			return fmt.Errorf("validate Identity Fact %s: terminated session is pre-installed", fact.ID)
		}
	case "access-denied":
		if fact.Principal == nil || fact.Principal.Kind != "anonymous" || !setupHasAnySubject(fact.Setup) || len(fact.Setup.Sessions) != 0 {
			return fmt.Errorf("validate Identity Fact %s: anonymous denial must use an existing resource without a session", fact.ID)
		}
	case "ownership-allowed", "ownership-denied":
		if fact.Principal == nil || fact.Principal.Kind != "authenticated" || !setupHasSessionCondition(fact.Setup, "active") {
			return fmt.Errorf("validate Identity Fact %s: ownership case lacks an authenticated precondition", fact.ID)
		}
	case "verification-expiry-boundary":
		for _, item := range fact.Input.Identity.Cases {
			if !setupHasEvidenceCondition(item.Setup, "issued") || setupHasEvidenceCondition(item.Setup, "consumed") || !setupHasClock(item.Setup, item.Kind) {
				return fmt.Errorf("validate Identity Fact %s: expiry case %s is self-fulfilled or incomplete", fact.ID, item.Kind)
			}
		}
	case "delivery-failure-separated":
		if fact.Setup == nil || fact.Setup.Delivery == nil || fact.Setup.Delivery.Condition != "fails" || setupHasState(fact.Setup, "Active") {
			return fmt.Errorf("validate Identity Fact %s: delivery failure precondition is incomplete", fact.ID)
		}
	}
	return nil
}

func validSubjectHandle(handle string) bool {
	parts := strings.Split(handle, "/")
	if len(parts) == 2 {
		return parts[0] == "subject" && validSubjectHandlePart(parts[1])
	}
	return len(parts) == 3 && parts[0] == "subject" && parts[1] == "value" && validSubjectHandlePart(parts[2])
}

func validSubjectHandlePart(part string) bool {
	return part != "" && !strings.ContainsAny(part, "@:. ")
}

func setupHasSubject(setup *FactSetup, handle string) bool {
	if setup == nil {
		return false
	}
	for _, item := range setup.Subjects {
		if item.Handle == handle {
			return true
		}
	}
	return false
}

func setupHasAnySubject(setup *FactSetup) bool {
	return setup != nil && len(setup.Subjects) != 0
}

func setupHasCredential(setup *FactSetup, handle string) bool {
	if setup == nil {
		return false
	}
	for _, subject := range setup.Subjects {
		for _, credential := range subject.Credentials {
			if credential.Handle == handle {
				return true
			}
		}
	}
	return false
}

func setupHasEvidence(setup *FactSetup, handle string) bool {
	if setup == nil {
		return false
	}
	for _, item := range setup.Evidence {
		if item.Handle == handle {
			return true
		}
	}
	return false
}

func setupHasSession(setup *FactSetup, handle string) bool {
	if setup == nil {
		return false
	}
	for _, item := range setup.Sessions {
		if item.Handle == handle {
			return true
		}
	}
	return false
}

func setupHasState(setup *FactSetup, value string) bool {
	if setup == nil {
		return false
	}
	for _, subject := range setup.Subjects {
		if subject.State != nil && subject.State.Value == value {
			return true
		}
	}
	return false
}

func setupHasEvidenceCondition(setup *FactSetup, condition string) bool {
	if setup == nil {
		return false
	}
	for _, item := range setup.Evidence {
		if item.Condition == condition {
			return true
		}
	}
	return false
}

func setupHasSessionCondition(setup *FactSetup, condition string) bool {
	if setup == nil {
		return false
	}
	for _, item := range setup.Sessions {
		if item.Condition == condition {
			return true
		}
	}
	return false
}

func setupHasClock(setup *FactSetup, relation string) bool {
	return setup != nil && setup.Clock != nil && setup.Clock.Relation == relation
}

func anyNonEmptySetup(setups []*FactSetup) bool {
	for _, setup := range setups {
		if setup != nil && (len(setup.Subjects) != 0 || len(setup.Evidence) != 0 || len(setup.Sessions) != 0 || setup.Clock != nil || setup.Delivery != nil) {
			return true
		}
	}
	return false
}

func containsSemanticID(values []SemanticID, expected SemanticID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func countEquals(value *int, expected int) bool {
	return value != nil && *value == expected
}

func negativeCount(value *int) bool {
	return value != nil && *value < 0
}
