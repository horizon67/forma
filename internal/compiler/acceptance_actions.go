package compiler

import (
	"fmt"
	"sort"
)

func deriveActionContractFacts(intent *ResolvedIntent) ([]AcceptanceFact, error) {
	b := acceptanceBuilder{
		intent: intent, types: map[string]IRType{}, entities: map[string]IREntity{},
		actions: map[SemanticID]IRAction{}, pages: map[string]IRPage{},
	}
	for _, item := range intent.Types {
		b.types[item.Name] = item
	}
	for _, item := range intent.Entities {
		b.entities[item.Name] = item
	}
	for _, item := range intent.Actions {
		b.actions[item.ID] = item
		if err := b.addTransitionFacts(item); err != nil {
			return nil, err
		}
		if err := b.addChangeFacts(item, nil); err != nil {
			return nil, err
		}
	}
	for _, item := range intent.Pages {
		b.pages[item.Name] = item
	}
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			entity, ok := b.entities[view.Entity]
			if !ok {
				return nil, fmt.Errorf("derive action Acceptance Facts: view %s references missing entity %s", view.ID, view.Entity)
			}
			for _, ref := range view.Actions {
				if err := b.addActionFacts(page, view, entity, ref); err != nil {
					return nil, err
				}
			}
		}
	}
	result := b.facts[:0]
	for _, fact := range b.facts {
		if isActionContractFact(fact) {
			result = append(result, fact)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (b *acceptanceBuilder) addTransitionFacts(action IRAction) error {
	entity, ok := b.entities[action.Entity]
	if !ok || entity.State == nil {
		return fmt.Errorf("build Acceptance Facts: action %s has no stateful entity", action.ID)
	}
	for _, source := range action.Sources {
		b.add(transitionFact(entity, action, nil, source, true))
	}
	sources := stringSet(action.Sources)
	for _, state := range entity.State.Values {
		if !sources[state] {
			b.add(transitionFact(entity, action, nil, state, false))
		}
	}
	return nil
}

func (b *acceptanceBuilder) addTransitionSurfaceFacts(entity IREntity, action IRAction, ref IRActionRef) error {
	if entity.State == nil {
		return fmt.Errorf("build Acceptance Facts: action reference %s has no stateful entity", ref.ID)
	}
	for _, source := range action.Sources {
		b.add(transitionFact(entity, action, &ref, source, true))
	}
	sources := stringSet(action.Sources)
	for _, state := range entity.State.Values {
		if !sources[state] {
			b.add(transitionFact(entity, action, &ref, state, false))
		}
	}
	return nil
}

func transitionFact(entity IREntity, action IRAction, ref *IRActionRef, state string, accepted bool) AcceptanceFact {
	subject := action.ID
	kind := "transition-source-rejected"
	caseName := "rejected"
	outcome := "rejected"
	expectedState := state
	applied := 0
	var feedback []string
	if accepted {
		kind = "transition-accepted"
		caseName = "accepted"
		outcome = "accepted"
		expectedState = action.Destination
		applied = 1
	}
	input := &FactActionInput{Action: action.ID, Subject: "subject/action", Dispatches: 1}
	sources := []SemanticID{action.ID, entity.ID, entity.State.ID}
	if ref != nil {
		subject = ref.ID
		kind = "action-" + kind
		input.Reference = ref.ID
		sources = append(sources, ref.ID)
		if !accepted {
			feedback = []string{"invalid"}
		}
	}
	expectation := FactSubjectExpectation{
		Handle: "subject/action", State: &IRStateValueRef{State: entity.State.ID, Value: expectedState},
	}
	if !accepted {
		expectation.Unchanged = true
	}
	return AcceptanceFact{
		ID: factID(subject, "transition", caseName, "from", state), Kind: kind, Subject: subject,
		Setup: &FactSetup{Subjects: []FactSubjectSetup{{
			Handle: "subject/action", Identity: entity.ID, State: &IRStateValueRef{State: entity.State.ID, Value: state},
		}}},
		Input: &FactInput{Action: input},
		Expected: FactExpectation{
			Outcome: outcome, Feedback: feedback, AppliedMutations: applied, Enforcement: "authoritative",
			Subjects: []FactSubjectExpectation{expectation},
		},
		SourceNodes: sources,
	}
}

func (b *acceptanceBuilder) addConfirmationFacts(entity IREntity, action *IRAction, ref IRActionRef) {
	setup := FactSubjectSetup{Handle: "subject/action", Identity: entity.ID}
	sources := []SemanticID{ref.ID, entity.ID}
	var actionID SemanticID
	if action != nil {
		actionID = action.ID
		sources = append(sources, action.ID)
		if entity.State != nil && len(action.Sources) > 0 {
			setup.State = &IRStateValueRef{State: entity.State.ID, Value: action.Sources[0]}
			sources = append(sources, entity.State.ID)
		}
	}
	for _, item := range []struct {
		confirmation string
		outcome      string
		dispatch     string
		unchanged    bool
	}{
		{confirmation: "accepted", outcome: "accepted", dispatch: "once"},
		{confirmation: "declined", outcome: "cancelled", dispatch: "none", unchanged: true},
	} {
		expected := FactExpectation{Outcome: item.outcome, Dispatch: item.dispatch}
		if item.unchanged {
			expected.Subjects = []FactSubjectExpectation{{Handle: "subject/action", State: cloneStateRef(setup.State), Unchanged: true}}
		}
		b.add(AcceptanceFact{
			ID: factID(ref.ID, "confirmation", item.confirmation), Kind: "confirmation-required", Subject: ref.ID,
			Setup: &FactSetup{Subjects: []FactSubjectSetup{setup}},
			Input: &FactInput{Action: &FactActionInput{
				Action: actionID, Reference: ref.ID, Subject: "subject/action", Confirmation: item.confirmation,
			}},
			Expected: expected, SourceNodes: sources,
		})
	}
}

func (b *acceptanceBuilder) addChangeFacts(action IRAction, ref *IRActionRef) error {
	if len(action.Changes) == 0 {
		return nil
	}
	if len(action.Changes) != 1 {
		return fmt.Errorf("build Acceptance Facts: action %s is outside the first Changes slice", action.ID)
	}
	entity, ok := b.entities[action.Entity]
	if !ok || entity.State == nil {
		return fmt.Errorf("build Acceptance Facts: Changes action %s has no stateful entity", action.ID)
	}
	change := action.Changes[0]
	targetEntity, targetField, relationField, err := b.resolveChangeTarget(entity, change)
	if err != nil {
		return err
	}
	for _, source := range action.Sources {
		b.add(changeAcceptedFact(entity, targetEntity, targetField, relationField, action, change, ref, source))
		for _, invariant := range targetEntity.Invariants {
			if !containsSemanticID(invariantFieldReferences(invariant), targetField.ID) {
				continue
			}
			b.add(changeInvariantRejectedFact(entity, targetEntity, relationField, action, change, invariant, ref, source))
		}
		if relationField != nil {
			b.add(changeTargetUnavailableFact(entity, *relationField, action, change, ref, source))
		}
	}
	return nil
}

func (b *acceptanceBuilder) resolveChangeTarget(entity IREntity, change IRActionChange) (IREntity, IRField, *IRField, error) {
	targetEntity := entity
	var relationField *IRField
	if len(change.Target.RelationPath) == 1 {
		field, ok := findIRFieldByID(entity, change.Target.RelationPath[0])
		if !ok || field.Relation == nil {
			return IREntity{}, IRField{}, nil, fmt.Errorf("build Acceptance Facts: change %s has invalid relation target", change.ID)
		}
		relationField = &field
		targetEntity = b.entities[field.Relation.Entity]
	}
	targetField, ok := findIRFieldByID(targetEntity, change.Target.Field)
	if !ok {
		return IREntity{}, IRField{}, nil, fmt.Errorf("build Acceptance Facts: change %s has missing target field", change.ID)
	}
	return targetEntity, targetField, relationField, nil
}

func changeAcceptedFact(
	entity, targetEntity IREntity,
	targetField IRField,
	relationField *IRField,
	action IRAction,
	change IRActionChange,
	ref *IRActionRef,
	source string,
) AcceptanceFact {
	subject, kind, input, sources := changeFactBase(entity, targetEntity, relationField, action, change, ref, source)
	return AcceptanceFact{
		ID: factID(subject, "changes", "accepted", "from", source), Kind: kind + "accepted", Subject: subject,
		Setup: input.setup, Input: &FactInput{Action: input.action, Invariants: invariantInputs(targetEntity, input.targetHandle, "")},
		Expected: FactExpectation{
			Outcome: "accepted", Atomicity: "all-changes-committed", AppliedMutations: 2,
			Enforcement: "authoritative", Subjects: changeCommittedSubjects(entity, targetField, action, change, input.targetHandle),
		},
		SourceNodes: append(sources, invariantSources(targetEntity)...),
	}
}

func changeInvariantRejectedFact(
	entity, targetEntity IREntity,
	relationField *IRField,
	action IRAction,
	change IRActionChange,
	violated IRInvariant,
	ref *IRActionRef,
	source string,
) AcceptanceFact {
	subject, kind, input, sources := changeFactBase(entity, targetEntity, relationField, action, change, ref, source)
	var feedback []string
	if ref != nil {
		feedback = []string{"invalid"}
	}
	return AcceptanceFact{
		ID:   factID(subject, "changes", "invariant", string(violated.ID), "rejected", "from", source),
		Kind: kind + "invariant-rejected", Subject: subject,
		Setup: input.setup, Input: &FactInput{Action: input.action, Invariants: invariantInputs(targetEntity, input.targetHandle, violated.ID)},
		Expected: FactExpectation{
			Outcome: "rejected", Reason: "invariant-violated", Violated: violated.ID, Feedback: feedback,
			Atomicity: "no-changes-committed", AppliedMutations: 0, Enforcement: "authoritative",
			Subjects: unchangedChangeSubjects(entity, action, source, input.targetHandle),
		},
		SourceNodes: append(sources, invariantSources(targetEntity)...),
	}
}

func changeTargetUnavailableFact(entity IREntity, relation IRField, action IRAction, change IRActionChange, ref *IRActionRef, source string) AcceptanceFact {
	subject := action.ID
	kind := "changes-target-unavailable"
	actionInput := &FactActionInput{Action: action.ID, Subject: "subject/action", Dispatches: 1}
	sources := []SemanticID{entity.ID, entity.State.ID, action.ID, change.ID, change.Target.ID, change.Value.ID, relation.ID}
	var feedback []string
	if ref != nil {
		subject = ref.ID
		kind = "action-changes-target-unavailable"
		actionInput.Reference = ref.ID
		sources = append(sources, ref.ID)
		feedback = []string{"failure"}
	}
	return AcceptanceFact{
		ID: factID(subject, "changes", "target-unavailable", "from", source), Kind: kind, Subject: subject,
		Setup: &FactSetup{
			Subjects:  []FactSubjectSetup{{Handle: "subject/action", Identity: entity.ID, State: &IRStateValueRef{State: entity.State.ID, Value: source}}},
			Relations: []FactRelationSetup{{Source: "subject/action", Field: relation.ID, Condition: "target-unavailable"}},
		},
		Input: &FactInput{Action: actionInput},
		Expected: FactExpectation{
			Outcome: "rejected", Reason: "target-unavailable", Feedback: feedback,
			Atomicity: "no-changes-committed", AppliedMutations: 0, Enforcement: "authoritative",
			Subjects: []FactSubjectExpectation{{Handle: "subject/action", State: &IRStateValueRef{State: entity.State.ID, Value: source}, Unchanged: true}},
		},
		SourceNodes: sources,
	}
}

type changeFactInput struct {
	setup        *FactSetup
	action       *FactActionInput
	targetHandle string
}

func changeFactBase(entity, targetEntity IREntity, relationField *IRField, action IRAction, change IRActionChange, ref *IRActionRef, source string) (SemanticID, string, changeFactInput, []SemanticID) {
	subject := action.ID
	kind := "changes-"
	actionInput := &FactActionInput{Action: action.ID, Subject: "subject/action", Dispatches: 1}
	sources := []SemanticID{entity.ID, entity.State.ID, action.ID, change.ID, change.Target.ID, change.Value.ID, change.Value.Field, change.Target.Field}
	setup := &FactSetup{Subjects: []FactSubjectSetup{{
		Handle: "subject/action", Identity: entity.ID, State: &IRStateValueRef{State: entity.State.ID, Value: source},
	}}}
	targetHandle := "subject/action"
	if relationField != nil {
		targetHandle = "subject/target"
		setup.Subjects = append(setup.Subjects, FactSubjectSetup{Handle: "subject/target", Identity: targetEntity.ID})
		setup.Relations = []FactRelationSetup{{Source: "subject/action", Field: relationField.ID, Target: "subject/target", Condition: "resolved"}}
		sources = append(sources, relationField.ID, targetEntity.ID)
	}
	if ref != nil {
		subject = ref.ID
		kind = "action-changes-"
		actionInput.Reference = ref.ID
		sources = append(sources, ref.ID)
	}
	return subject, kind, changeFactInput{setup: setup, action: actionInput, targetHandle: targetHandle}, sources
}

func changeCommittedSubjects(entity IREntity, targetField IRField, action IRAction, change IRActionChange, targetHandle string) []FactSubjectExpectation {
	subject := FactSubjectExpectation{
		Handle: "subject/action", State: &IRStateValueRef{State: entity.State.ID, Value: action.Destination},
	}
	field := FactFieldExpectation{
		Field: targetField.ID, Stored: "value-source", ValueSubject: "subject/action", ValueField: change.Value.Field,
	}
	if targetHandle == "subject/action" {
		subject.Fields = []FactFieldExpectation{field}
		return []FactSubjectExpectation{subject}
	}
	return []FactSubjectExpectation{
		subject,
		{Handle: targetHandle, Fields: []FactFieldExpectation{field}},
	}
}

func unchangedChangeSubjects(entity IREntity, action IRAction, source, targetHandle string) []FactSubjectExpectation {
	result := []FactSubjectExpectation{{
		Handle: "subject/action", State: &IRStateValueRef{State: entity.State.ID, Value: source}, Unchanged: true,
	}}
	if targetHandle != "subject/action" {
		result = append(result, FactSubjectExpectation{Handle: targetHandle, Unchanged: true})
	}
	return result
}

func invariantInputs(entity IREntity, subject string, violated SemanticID) []FactInvariantInput {
	result := make([]FactInvariantInput, 0, len(entity.Invariants))
	for _, invariant := range entity.Invariants {
		result = append(result, FactInvariantInput{
			Invariant: invariant.ID, Subject: subject, Expression: cloneIRExpression(invariant.Predicate),
			Evaluation: "post-state", OtherRequirements: "satisfied", Result: invariant.ID != violated,
		})
	}
	return result
}

func invariantSources(entity IREntity) []SemanticID {
	var result []SemanticID
	for _, invariant := range entity.Invariants {
		result = append(result, invariantFactSourceNodes(invariant)...)
	}
	return result
}

func findIRFieldByID(entity IREntity, id SemanticID) (IRField, bool) {
	for _, field := range entity.Fields {
		if field.ID == id {
			return field, true
		}
	}
	return IRField{}, false
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func cloneStateRef(value *IRStateValueRef) *IRStateValueRef {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
