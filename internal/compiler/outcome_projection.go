package compiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const OutcomeProjectionVersion = "forma/outcome-projection/v0alpha4"

// OutcomeProjection is a deterministic review view over observable Acceptance
// Facts. It splits multi-case facts into rows but does not add outcomes that
// are absent from the facts.
type OutcomeProjection struct {
	Version       string         `json:"version"`
	IntentVersion string         `json:"intentVersion"`
	FactsVersion  string         `json:"factsVersion"`
	Groups        []OutcomeGroup `json:"groups"`
}

type OutcomeGroup struct {
	ID    SemanticID   `json:"id"`
	Label string       `json:"label"`
	Rows  []OutcomeRow `json:"rows"`
}

type OutcomeRow struct {
	ID              SemanticID                   `json:"id"`
	FactID          SemanticID                   `json:"factId"`
	Kind            string                       `json:"kind"`
	Case            string                       `json:"case"`
	Principal       *FactPrincipal               `json:"principal,omitempty"`
	Input           *FactInput                   `json:"input,omitempty"`
	CaseInput       *IdentityFactCase            `json:"caseInput,omitempty"`
	Expected        FactExpectation              `json:"expected"`
	CaseExpectation *IdentityFactCaseExpectation `json:"caseExpectation,omitempty"`
	SourceNodes     []SemanticID                 `json:"sourceNodes"`
}

// BuildOutcomeProjection derives outcome rows from compiler-owned Acceptance
// Facts. Structural facts such as field lists and sort definitions stay out of
// this view; observable results, feedback, navigation, and explicit negative
// guarantees remain.
func BuildOutcomeProjection(intent *ResolvedIntent, sourceMap *SourceMap) (*OutcomeProjection, error) {
	if err := ValidateSourceMapCoverage(intent, sourceMap); err != nil {
		return nil, fmt.Errorf("build Outcome Projection: %w", err)
	}
	facts, err := BuildAcceptanceFacts(intent)
	if err != nil {
		return nil, fmt.Errorf("build Outcome Projection: %w", err)
	}

	interactionOperations := map[SemanticID]SemanticID{}
	for _, page := range intent.Pages {
		for _, interaction := range page.IdentityInteractions {
			interactionOperations[interaction.ID] = interaction.Operation
		}
	}
	groups := map[SemanticID]*OutcomeGroup{}
	for _, fact := range facts.Facts {
		if !isOutcomeFact(fact) {
			continue
		}
		groupID := outcomeGroupID(fact, interactionOperations)
		group := groups[groupID]
		if group == nil {
			group = &OutcomeGroup{ID: groupID, Label: outcomeSemanticLabel(groupID)}
			groups[groupID] = group
		}
		for _, row := range outcomeRowsForFact(fact) {
			group.Rows = append(group.Rows, row)
		}
	}

	projection := &OutcomeProjection{
		Version: OutcomeProjectionVersion, IntentVersion: intent.Version, FactsVersion: facts.Version,
	}
	for _, group := range groups {
		sort.Slice(group.Rows, func(i, j int) bool { return group.Rows[i].ID < group.Rows[j].ID })
		projection.Groups = append(projection.Groups, *group)
	}
	sort.Slice(projection.Groups, func(i, j int) bool { return projection.Groups[i].ID < projection.Groups[j].ID })
	if err := validateOutcomeProjection(projection, sourceMap); err != nil {
		return nil, fmt.Errorf("build Outcome Projection: %w", err)
	}
	return projection, nil
}

func isOutcomeFact(fact AcceptanceFact) bool {
	// The default entry is an application structure fact, not the result of an
	// operation or user-triggered edge. Flow renders it as its own root node.
	if fact.Kind == "application-entry" {
		return false
	}
	expected := fact.Expected
	if expected.Outcome != "" || expected.Reason != "" || expected.Dispatch != "" || len(expected.Subjects) > 0 || len(expected.Feedback) > 0 || expected.AppliedMutations > 0 ||
		expected.Enforcement != "" || expected.Atomicity != "" || expected.Stored != "" || len(expected.PreserveInput) > 0 || expected.Navigation != nil {
		return true
	}
	identity := expected.Identity
	if identity == nil {
		return false
	}
	return identity.Outcome != "" || identity.Subject != nil || identity.Credential != nil ||
		identity.Evidence != nil || identity.Notice != nil || identity.Session != nil ||
		identity.Disclosure != "" || identity.Navigation != nil || len(identity.PreserveFields) > 0 ||
		len(identity.ExcludedCredentials) > 0 || len(identity.Surfaces) > 0 ||
		identity.AppliedOperations > 0 || len(identity.Cases) > 0
}

func outcomeGroupID(fact AcceptanceFact, interactions map[SemanticID]SemanticID) SemanticID {
	if fact.Input != nil && fact.Input.Identity != nil {
		input := fact.Input.Identity
		if input.Operation != "" {
			return input.Operation
		}
		if operation := interactions[input.Interaction]; operation != "" {
			return operation
		}
	}
	return fact.Subject
}

func outcomeRowsForFact(fact AcceptanceFact) []OutcomeRow {
	expected := fact.Expected
	var cases []string
	caseInputs := map[string]IdentityFactCase{}
	caseExpectations := map[string]IdentityFactCaseExpectation{}
	if expected.Identity != nil {
		identityExpected := *expected.Identity
		for _, item := range identityExpected.Cases {
			cases = append(cases, item.Kind)
			caseExpectations[item.Kind] = item
		}
		identityExpected.Cases = nil
		expected.Identity = &identityExpected
	}
	input := fact.Input
	if input != nil {
		inputCopy := *input
		input = &inputCopy
	}
	if input != nil && input.Identity != nil {
		identityInput := *input.Identity
		for _, item := range identityInput.Cases {
			caseInputs[item.Kind] = item
			if _, ok := caseExpectations[item.Kind]; !ok {
				cases = append(cases, item.Kind)
			}
		}
		identityInput.Cases = nil
		input.Identity = &identityInput
	}
	if len(cases) == 0 {
		cases = []string{outcomeFactCase(fact)}
	}
	cases = outcomeCanonicalStrings(cases)

	rows := make([]OutcomeRow, 0, len(cases))
	for _, caseName := range cases {
		rowID := semanticID("projection", "outcomes", "row", string(fact.ID))
		if len(cases) > 1 {
			rowID = semanticID(string(rowID), "case", caseName)
		}
		row := OutcomeRow{
			ID:     rowID,
			FactID: fact.ID, Kind: fact.Kind, Case: caseName, Principal: cloneFactPrincipal(fact.Principal), Input: input, Expected: expected,
			SourceNodes: append([]SemanticID(nil), fact.SourceNodes...),
		}
		if item, ok := caseInputs[caseName]; ok {
			copy := item
			row.CaseInput = &copy
		}
		if item, ok := caseExpectations[caseName]; ok {
			copy := item
			row.CaseExpectation = &copy
		}
		rows = append(rows, row)
	}
	return rows
}

func cloneFactPrincipal(value *FactPrincipal) *FactPrincipal {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Roles = append([]string(nil), value.Roles...)
	return &copy
}

func outcomeFactCase(fact AcceptanceFact) string {
	if fact.Input != nil && fact.Input.Predicate != nil {
		if fact.Input.Predicate.Result {
			return "predicate true"
		}
		return "predicate false"
	}
	if fact.Input != nil && fact.Input.Violation != nil {
		violation := fact.Input.Violation
		return violation.Kind + " " + outcomeSemanticLabel(violation.Field)
	}
	if fact.Principal != nil {
		principal := fact.Principal
		switch {
		case len(principal.Roles) > 0:
			return "roles(" + strings.Join(principal.Roles, ",") + ")"
		case principal.Kind != "":
			return principal.Kind
		}
	}
	return fact.Kind
}

func outcomeCanonicalStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validateOutcomeProjection(projection *OutcomeProjection, sourceMap *SourceMap) error {
	if projection == nil {
		return fmt.Errorf("nil projection")
	}
	if projection.Version != OutcomeProjectionVersion || projection.IntentVersion != sourceMap.IntentVersion || projection.FactsVersion != AcceptanceFactsVersion {
		return fmt.Errorf("schema versions do not match Source Map and Acceptance Facts")
	}
	sourceNodes := map[SemanticID]bool{}
	for _, entry := range sourceMap.Entries {
		sourceNodes[entry.NodeID] = true
	}
	groups := map[SemanticID]bool{}
	rows := map[SemanticID]bool{}
	for _, group := range projection.Groups {
		if group.ID == "" || group.Label == "" || groups[group.ID] || len(group.Rows) == 0 {
			return fmt.Errorf("outcome group has an empty or duplicate identity")
		}
		groups[group.ID] = true
		for _, row := range group.Rows {
			if row.ID == "" || strings.ContainsAny(string(row.ID), " \t\r\n") || row.FactID == "" || row.Kind == "" || row.Case == "" || rows[row.ID] {
				return fmt.Errorf("outcome row has an empty or duplicate identity")
			}
			rows[row.ID] = true
			if len(row.SourceNodes) == 0 {
				return fmt.Errorf("outcome row %s has no source provenance", row.ID)
			}
			if row.CaseInput != nil && row.CaseInput.Kind != row.Case {
				return fmt.Errorf("outcome row %s has a mismatched case input", row.ID)
			}
			if row.CaseExpectation != nil && row.CaseExpectation.Kind != row.Case {
				return fmt.Errorf("outcome row %s has a mismatched case expectation", row.ID)
			}
			for _, node := range row.SourceNodes {
				if !sourceNodes[node] {
					return fmt.Errorf("outcome row %s source node %s has no Source Map entry", row.ID, node)
				}
			}
			expected, prohibited := summarizeOutcomeRow(row)
			if len(expected) == 0 && len(prohibited) == 0 {
				return fmt.Errorf("outcome row %s has no observable result", row.ID)
			}
		}
	}
	return nil
}

// FormatOutcomeProjection renders case-oriented review text. Explicit zero,
// absent, and excluded expectations are separated as must-not guarantees;
// their inverses are never inferred.
func FormatOutcomeProjection(projection *OutcomeProjection) (string, error) {
	if projection == nil {
		return "", fmt.Errorf("format Outcome Projection: nil projection")
	}
	groups := append([]OutcomeGroup(nil), projection.Groups...)
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	rowCount := 0
	for index := range groups {
		groups[index].Rows = append([]OutcomeRow(nil), groups[index].Rows...)
		sort.Slice(groups[index].Rows, func(i, j int) bool { return groups[index].Rows[i].ID < groups[index].Rows[j].ID })
		rowCount += len(groups[index].Rows)
	}

	var output strings.Builder
	fmt.Fprintf(&output, "outcome projection %s\n", projection.Version)
	fmt.Fprintf(&output, "intent %s\n", projection.IntentVersion)
	fmt.Fprintf(&output, "facts %s\n", projection.FactsVersion)
	fmt.Fprintf(&output, "rows %d\n", rowCount)
	for _, group := range groups {
		fmt.Fprintf(&output, "\ngroup %s [%s]\n", group.Label, group.ID)
		for _, row := range group.Rows {
			expected, prohibited := summarizeOutcomeRow(row)
			fmt.Fprintf(&output, "  case %s / %s\n", row.Case, row.Kind)
			fmt.Fprintf(&output, "    expect: %s\n", outcomeSummary(expected, "(no positive result declared)"))
			fmt.Fprintf(&output, "    must not: %s\n", outcomeSummary(prohibited, "(none declared)"))
			fmt.Fprintf(&output, "    fact: %s\n", row.FactID)
		}
	}
	return output.String(), nil
}

func summarizeOutcomeRow(row OutcomeRow) ([]string, []string) {
	var expected, prohibited []string
	addExpected := func(value string) { expected = appendUniqueString(expected, value) }
	addProhibited := func(value string) { prohibited = appendUniqueString(prohibited, value) }
	value := row.Expected
	commonOutcome := value.Outcome
	if value.Outcome != "" {
		addExpected("outcome=" + value.Outcome)
	}
	if value.Reason != "" {
		addExpected("reason=" + value.Reason)
	}
	if value.Violated != "" {
		addExpected("violated=" + outcomeSemanticLabel(value.Violated))
	}
	if value.Dispatch == "none" {
		addProhibited("action dispatched")
	} else if value.Dispatch != "" {
		addExpected("dispatch=" + value.Dispatch)
	}
	if len(value.Feedback) > 0 {
		addExpected("feedback=" + strings.Join(value.Feedback, ","))
	}
	if value.AppliedMutations > 0 {
		addExpected("applied-mutations=" + strconv.Itoa(value.AppliedMutations))
	}
	if value.Enforcement != "" {
		addExpected("enforcement=" + value.Enforcement)
	}
	if value.Atomicity != "" {
		if value.Atomicity == "no-changes-committed" {
			addProhibited("changes committed")
		} else {
			addExpected("atomicity=" + value.Atomicity)
		}
	}
	if value.Stored == "unchanged" {
		addProhibited("stored data changed")
	} else if value.Stored != "" {
		addExpected("stored=" + value.Stored)
	}
	if len(value.PreserveInput) > 0 {
		addExpected("preserve-input=" + outcomeSemanticLabels(value.PreserveInput))
	}
	for _, subject := range value.Subjects {
		prefix := subject.Handle
		if subject.State != nil {
			addExpected(prefix + ".state=" + outcomeStateLabel(*subject.State))
		}
		if subject.Unchanged {
			addExpected(prefix + " unchanged")
		}
		for _, field := range subject.Fields {
			if field.Expression != nil {
				addExpected(prefix + "." + outcomeFieldLabel(field.Field) + "=" + outcomeExpressionSummary(*field.Expression))
			}
		}
	}
	appendNavigationSummary(value.Navigation, addExpected)

	if identity := value.Identity; identity != nil {
		if identity.Outcome != "" {
			commonOutcome = identity.Outcome
			addExpected("outcome=" + identity.Outcome)
		}
		if subject := identity.Subject; subject != nil {
			appendCountSummary("subject", subject.Count, addExpected, addProhibited)
			if subject.State != nil {
				addExpected("subject.state=" + outcomeStateLabel(*subject.State))
			}
			if subject.Unchanged {
				addExpected("subject unchanged")
			}
		}
		if credential := identity.Credential; credential != nil {
			label := outcomeSemanticLabel(credential.Credential)
			if credential.Condition == "absent" {
				addProhibited("credential present=" + label)
			} else {
				addExpected("credential=" + label + ":" + credential.Condition)
			}
		}
		if evidence := identity.Evidence; evidence != nil {
			appendCountSummary("evidence", evidence.Count, addExpected, addProhibited)
			appendAddedSummary("evidence", evidence.Added, addExpected, addProhibited)
			if evidence.Condition != "" {
				addExpected("evidence.condition=" + evidence.Condition)
			}
			if evidence.MaxUses > 0 {
				addExpected("evidence.max-uses=" + strconv.Itoa(evidence.MaxUses))
			}
			if evidence.Lifetime != nil {
				addExpected("evidence.lifetime=" + strconv.Itoa(evidence.Lifetime.Amount) + "-" + evidence.Lifetime.Unit)
			}
			if evidence.Rotation != "" {
				addExpected("evidence.rotation=" + evidence.Rotation)
			}
		}
		if notice := identity.Notice; notice != nil {
			appendCountSummary("notice", notice.Count, addExpected, addProhibited)
			appendAddedSummary("notice", notice.Added, addExpected, addProhibited)
			if notice.Emission != "" {
				addExpected("notice.emission=" + notice.Emission)
			}
			if notice.Delivery != "" {
				addExpected("notice.delivery=" + notice.Delivery)
			}
		}
		if session := identity.Session; session != nil {
			if session.Condition == "absent" {
				addProhibited("session present")
			} else {
				addExpected("session=" + session.Condition)
			}
		}
		if identity.Disclosure != "" {
			addExpected("disclosure=" + identity.Disclosure)
		}
		appendNavigationSummary(identity.Navigation, addExpected)
		if len(identity.PreserveFields) > 0 {
			addExpected("preserve-fields=" + outcomeSemanticLabels(identity.PreserveFields))
		}
		for _, credential := range identity.ExcludedCredentials {
			addProhibited("credential included=" + outcomeSemanticLabel(credential))
		}
		if len(identity.Surfaces) > 0 {
			addExpected("surfaces=" + outcomeSemanticLabels(identity.Surfaces))
		}
		if identity.AppliedOperations > 0 {
			addExpected("applied-operations=" + strconv.Itoa(identity.AppliedOperations))
		}
	}
	if item := row.CaseExpectation; item != nil {
		if item.Outcome != "" {
			label := "outcome="
			if commonOutcome != "" && commonOutcome != item.Outcome {
				label = "case-outcome="
			}
			addExpected(label + item.Outcome)
		}
		if item.SubjectState != nil {
			addExpected("subject.state=" + outcomeStateLabel(*item.SubjectState))
		}
		if item.EvidenceCondition != "" {
			addExpected("evidence.condition=" + item.EvidenceCondition)
		}
		if item.SessionCondition == "absent" {
			addProhibited("session present")
		} else if item.SessionCondition != "" {
			addExpected("session=" + item.SessionCondition)
		}
		if item.Disclosure != "" {
			addExpected("disclosure=" + item.Disclosure)
		}
	}
	return expected, prohibited
}

func outcomeExpressionSummary(expectation FactExpressionExpectation) string {
	bindings := make(map[SemanticID]string, len(expectation.Bindings))
	for _, binding := range expectation.Bindings {
		bindings[binding.Node] = binding.Subject
	}
	var render func(IRExpression) string
	render = func(expression IRExpression) string {
		if expression.Kind == "field-reference" {
			return bindings[expression.ID] + "." + outcomeFieldLabel(expression.Field)
		}
		if expression.Kind == "binary-expression" && expression.Left != nil && expression.Right != nil {
			return expression.Operator + "(" + render(*expression.Left) + "," + render(*expression.Right) + ")"
		}
		return outcomeSemanticLabel(expression.ID)
	}
	return render(expectation.Tree)
}

func outcomeFieldLabel(id SemanticID) string {
	parts := strings.Split(string(id), "/")
	if len(parts) == 0 {
		return string(id)
	}
	return parts[len(parts)-1]
}

func appendCountSummary(name string, count *int, addExpected, addProhibited func(string)) {
	if count == nil {
		return
	}
	if *count == 0 {
		addProhibited(name + " present")
		return
	}
	addExpected(name + ".count=" + strconv.Itoa(*count))
}

func appendAddedSummary(name string, count *int, addExpected, addProhibited func(string)) {
	if count == nil {
		return
	}
	if *count == 0 {
		addProhibited(name + " added")
		return
	}
	addExpected(name + ".added=" + strconv.Itoa(*count))
}

func appendNavigationSummary(navigation *FactNavigation, add func(string)) {
	if navigation == nil {
		return
	}
	if navigation.TargetPage != "" {
		add("navigation.target=" + outcomeSemanticLabel(navigation.TargetPage))
	}
	if navigation.SuccessKind != "" {
		value := navigation.SuccessKind
		if navigation.SuccessPage != "" {
			value += ":" + outcomeSemanticLabel(navigation.SuccessPage)
		}
		add("navigation.success=" + value)
	}
	if navigation.FallbackPage != "" {
		add("navigation.fallback=" + outcomeSemanticLabel(navigation.FallbackPage))
	}
	if navigation.ContinuationPage != "" {
		add("navigation.continue=" + outcomeSemanticLabel(navigation.ContinuationPage))
	}
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func outcomeSummary(values []string, empty string) string {
	if len(values) == 0 {
		return empty
	}
	return strings.Join(values, "; ")
}

func outcomeSemanticLabels(values []SemanticID) string {
	labels := make([]string, len(values))
	for index, value := range values {
		labels[index] = outcomeSemanticLabel(value)
	}
	return strings.Join(labels, ",")
}

func outcomeStateLabel(value IRStateValueRef) string {
	return outcomeSemanticLabel(value.State) + ":" + value.Value
}

func outcomeSemanticLabel(id SemanticID) string {
	parts := strings.Split(string(id), "/")
	if len(parts) == 0 {
		return string(id)
	}
	switch parts[0] {
	case "identity":
		if len(parts) >= 4 {
			return parts[1] + "." + strings.Join(parts[3:], ".")
		}
	case "entity":
		if len(parts) >= 4 {
			return parts[1] + "." + strings.Join(parts[3:], ".")
		}
		if len(parts) >= 2 {
			return parts[1]
		}
	case "page":
		if len(parts) == 2 {
			return parts[1]
		}
		for index, part := range parts {
			if part == "action" && index > 0 && index+1 < len(parts) {
				return parts[1] + ":" + parts[index-1] + "." + parts[index+1]
			}
		}
		if len(parts) >= 5 && parts[2] == "identity" {
			return parts[1] + ":" + parts[4] + "." + parts[3]
		}
		if len(parts) >= 6 && parts[2] == "view" && parts[3] == "form" {
			label := parts[1] + ":" + parts[5] + "." + parts[4]
			if parts[len(parts)-1] == "submit" {
				label += " submit"
			} else {
				label += " form"
			}
			return label
		}
		if len(parts) >= 5 && parts[2] == "view" {
			return parts[1] + ":" + parts[4] + "." + parts[3]
		}
		return parts[1] + ":" + strings.Join(parts[2:], ".")
	case "action":
		if len(parts) >= 3 {
			return parts[1] + "." + parts[2]
		}
	case "type":
		if len(parts) >= 2 {
			return parts[1] + "." + strings.Join(parts[2:], ".")
		}
	}
	return string(id)
}
