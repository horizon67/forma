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
		if err := b.addPreconditionFacts(item, nil); err != nil {
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
	var plan *actionBindingPlan
	if len(action.Preconditions) != 0 {
		resolved, err := b.resolveActionBindingPlan(entity, action)
		if err != nil {
			return err
		}
		plan = &resolved
	}
	for _, source := range action.Sources {
		b.add(transitionFact(entity, action, nil, source, true, plan))
	}
	sources := stringSet(action.Sources)
	for _, state := range entity.State.Values {
		if !sources[state] {
			b.add(transitionFact(entity, action, nil, state, false, nil))
		}
	}
	return nil
}

func (b *acceptanceBuilder) addTransitionSurfaceFacts(entity IREntity, action IRAction, ref IRActionRef) error {
	if entity.State == nil {
		return fmt.Errorf("build Acceptance Facts: action reference %s has no stateful entity", ref.ID)
	}
	var plan *actionBindingPlan
	if len(action.Preconditions) != 0 {
		resolved, err := b.resolveActionBindingPlan(entity, action)
		if err != nil {
			return err
		}
		plan = &resolved
	}
	for _, source := range action.Sources {
		b.add(transitionFact(entity, action, &ref, source, true, plan))
	}
	sources := stringSet(action.Sources)
	for _, state := range entity.State.Values {
		if !sources[state] {
			b.add(transitionFact(entity, action, &ref, state, false, nil))
		}
	}
	return nil
}

func transitionFact(entity IREntity, action IRAction, ref *IRActionRef, state string, accepted bool, plan *actionBindingPlan) AcceptanceFact {
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
	setup := &FactSetup{Subjects: []FactSubjectSetup{{
		Handle: "subject/action", Identity: entity.ID, State: &IRStateValueRef{State: entity.State.ID, Value: state},
	}}}
	inputValue := FactInput{Action: input}
	if accepted && plan != nil {
		setup = plan.setup(state, "")
		inputValue.Preconditions = plan.preconditionInputs(true)
		sources = append(sources, plan.sourceNodes()...)
	}
	reason := ""
	if !accepted {
		reason = "source-state-mismatch"
	}
	return AcceptanceFact{
		ID: factID(subject, "transition", caseName, "from", state), Kind: kind, Subject: subject,
		Setup: setup,
		Input: &inputValue,
		Expected: FactExpectation{
			Outcome: outcome, Reason: reason, Feedback: feedback, AppliedMutations: applied, Enforcement: "authoritative",
			Subjects: []FactSubjectExpectation{expectation},
		},
		SourceNodes: sources,
	}
}

func (b *acceptanceBuilder) addPreconditionFacts(action IRAction, ref *IRActionRef) error {
	if len(action.Preconditions) == 0 {
		return nil
	}
	entity, ok := b.entities[action.Entity]
	if !ok || entity.State == nil {
		return fmt.Errorf("build Acceptance Facts: Precondition action %s has no stateful entity", action.ID)
	}
	plan, err := b.resolveActionBindingPlan(entity, action)
	if err != nil {
		return err
	}
	for _, source := range action.Sources {
		for _, precondition := range plan.preconditions {
			b.add(preconditionUnsatisfiedFact(plan, precondition, ref, source))
			for _, relation := range plan.preconditionOnlyRelations() {
				used := false
				for _, leaf := range precondition.value.leaves {
					used = used || leaf.relation != nil && leaf.relation.ID == relation.field.ID
				}
				if used {
					b.add(preconditionValueUnavailableFact(plan, precondition, relation, ref, source))
				}
			}
		}
	}
	return nil
}

func preconditionUnsatisfiedFact(plan actionBindingPlan, precondition resolvedActionPrecondition, ref *IRActionRef, source string) AcceptanceFact {
	subject := plan.action.ID
	kind := "precondition-unsatisfied"
	actionInput := &FactActionInput{Action: plan.action.ID, Subject: "subject/action", Dispatches: 1}
	sources := plan.sourceNodes()
	var feedback []string
	if ref != nil {
		subject = ref.ID
		kind = "action-precondition-unsatisfied"
		actionInput.Reference = ref.ID
		sources = append(sources, ref.ID)
		feedback = []string{"invalid"}
	}
	setup := plan.setup(source, "")
	return AcceptanceFact{
		ID:   factID(subject, "precondition", precondition.precondition.Name, "unsatisfied", "from", source),
		Kind: kind, Subject: subject, Setup: setup,
		Input: &FactInput{Action: actionInput, Preconditions: plan.preconditionInputs(false)},
		Expected: FactExpectation{
			Outcome: "rejected", Reason: "precondition-unsatisfied", Feedback: feedback,
			Atomicity: "no-changes-committed", AppliedMutations: 0, Enforcement: "authoritative",
			Subjects: unchangedPlanSubjects(plan, setup, source),
		},
		SourceNodes: canonicalSemanticIDs(sources),
	}
}

func preconditionValueUnavailableFact(plan actionBindingPlan, precondition resolvedActionPrecondition, relation actionRelationBinding, ref *IRActionRef, source string) AcceptanceFact {
	subject := plan.action.ID
	kind := "precondition-value-unavailable"
	actionInput := &FactActionInput{Action: plan.action.ID, Subject: "subject/action", Dispatches: 1}
	sources := append(plan.sourceNodes(), relation.field.ID, relation.entity.ID)
	var feedback []string
	if ref != nil {
		subject = ref.ID
		kind = "action-precondition-value-unavailable"
		actionInput.Reference = ref.ID
		sources = append(sources, ref.ID)
		feedback = []string{"failure"}
	}
	setup := plan.setup(source, relation.field.ID)
	return AcceptanceFact{
		ID:   factID(subject, "precondition", precondition.precondition.Name, "value-unavailable", "via", relation.field.Name, "from", source),
		Kind: kind, Subject: subject, Setup: setup,
		Input: &FactInput{Action: actionInput},
		Expected: FactExpectation{
			Outcome: "rejected", Reason: "value-unavailable", Feedback: feedback,
			Atomicity: "no-changes-committed", AppliedMutations: 0, Enforcement: "authoritative",
			Subjects: unchangedPlanSubjects(plan, setup, source),
		},
		SourceNodes: canonicalSemanticIDs(sources),
	}
}

func unchangedPlanSubjects(plan actionBindingPlan, setup *FactSetup, source string) []FactSubjectExpectation {
	result := make([]FactSubjectExpectation, 0, len(setup.Subjects))
	for _, subject := range setup.Subjects {
		expectation := FactSubjectExpectation{Handle: subject.Handle, Unchanged: true}
		if subject.Handle == "subject/action" {
			expectation.State = &IRStateValueRef{State: plan.entity.State.ID, Value: source}
		}
		result = append(result, expectation)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Handle < result[j].Handle })
	return result
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
	plan, err := b.resolveActionBindingPlan(entity, action)
	if err != nil {
		return err
	}
	change := action.Changes[0]
	targetEntity, targetField, relationField := plan.targetEntity, *plan.targetField, plan.targetRelation
	value := *plan.changeValue
	for _, source := range action.Sources {
		b.add(changeAcceptedFact(entity, targetEntity, targetField, relationField, value, action, change, ref, source, plan))
		for _, invariant := range targetEntity.Invariants {
			if !containsSemanticID(invariantFieldReferences(invariant), targetField.ID) {
				continue
			}
			b.add(changeInvariantRejectedFact(entity, targetEntity, relationField, value, action, change, invariant, ref, source, plan))
		}
		if relationField != nil {
			b.add(changeTargetUnavailableFact(entity, targetEntity, *relationField, value, action, change, ref, source, plan))
		}
		for _, relation := range distinctChangeValueRelations(value.leaves, relationField) {
			b.add(changeValueUnavailableFact(entity, targetEntity, relationField, value, relation, action, change, ref, source, plan))
		}
	}
	return nil
}

type resolvedChangeValue struct {
	expression IRExpression
	leaves     []changeValueLeaf
}

type resolvedActionPrecondition struct {
	precondition IRActionPrecondition
	value        resolvedChangeValue
}

type actionRelationBinding struct {
	field  IRField
	entity IREntity
	handle string
	owner  string
}

type actionBindingPlan struct {
	entity         IREntity
	action         IRAction
	targetEntity   IREntity
	targetField    *IRField
	targetRelation *IRField
	changeValue    *resolvedChangeValue
	preconditions  []resolvedActionPrecondition
	relations      []actionRelationBinding
}

func (b *acceptanceBuilder) resolveActionBindingPlan(entity IREntity, action IRAction) (actionBindingPlan, error) {
	plan := actionBindingPlan{entity: entity, action: action, targetEntity: entity}
	relations := map[SemanticID]actionRelationBinding{}
	if len(action.Changes) == 1 {
		change := action.Changes[0]
		targetEntity, targetField, targetRelation, err := b.resolveChangeTarget(entity, change)
		if err != nil {
			return actionBindingPlan{}, err
		}
		value, err := b.resolveChangeValueExpression(entity, change.Value)
		if err != nil {
			return actionBindingPlan{}, err
		}
		plan.targetEntity = targetEntity
		plan.targetField = &targetField
		plan.targetRelation = targetRelation
		plan.changeValue = &value
		if targetRelation != nil {
			relations[targetRelation.ID] = actionRelationBinding{field: *targetRelation, entity: targetEntity, handle: "subject/target", owner: "target"}
		}
		for _, leaf := range value.leaves {
			if leaf.relation == nil {
				continue
			}
			if _, exists := relations[leaf.relation.ID]; !exists {
				relations[leaf.relation.ID] = actionRelationBinding{
					field: *leaf.relation, entity: leaf.entity, handle: "subject/value/" + leaf.relation.Name, owner: "value",
				}
			}
		}
	}
	for _, precondition := range action.Preconditions {
		value, err := b.resolveChangeValueExpression(entity, precondition.Predicate)
		if err != nil {
			return actionBindingPlan{}, fmt.Errorf("build Acceptance Facts: precondition %s: %w", precondition.ID, err)
		}
		plan.preconditions = append(plan.preconditions, resolvedActionPrecondition{precondition: precondition, value: value})
		for _, leaf := range value.leaves {
			if leaf.relation == nil {
				continue
			}
			if _, exists := relations[leaf.relation.ID]; !exists {
				relations[leaf.relation.ID] = actionRelationBinding{
					field: *leaf.relation, entity: leaf.entity, handle: "subject/precondition/" + leaf.relation.Name, owner: "precondition",
				}
			}
		}
	}
	for _, relation := range relations {
		plan.relations = append(plan.relations, relation)
	}
	sort.Slice(plan.relations, func(i, j int) bool { return plan.relations[i].field.ID < plan.relations[j].field.ID })
	return plan, nil
}

func (plan actionBindingPlan) setup(source string, unavailable SemanticID) *FactSetup {
	setup := &FactSetup{Subjects: []FactSubjectSetup{{
		Handle: "subject/action", Identity: plan.entity.ID,
		State: &IRStateValueRef{State: plan.entity.State.ID, Value: source},
	}}}
	for _, relation := range plan.relations {
		item := FactRelationSetup{Source: "subject/action", Field: relation.field.ID, Target: relation.handle, Condition: "resolved"}
		if relation.field.ID == unavailable {
			item.Target = ""
			if relation.owner == "target" {
				item.Condition = "target-unavailable"
			} else {
				item.Condition = "value-unavailable"
			}
		} else {
			setup.Subjects = append(setup.Subjects, FactSubjectSetup{Handle: relation.handle, Identity: relation.entity.ID})
		}
		setup.Relations = append(setup.Relations, item)
	}
	canonicalizeFactSetup(setup)
	return setup
}

func (plan actionBindingPlan) handleForExpression(expression IRExpression) string {
	if len(expression.RelationPath) == 0 {
		return "subject/action"
	}
	for _, relation := range plan.relations {
		if relation.field.ID == expression.RelationPath[0] {
			return relation.handle
		}
	}
	return ""
}

func (plan actionBindingPlan) expressionBindings(value resolvedChangeValue) []FactExpressionBinding {
	bindings := make([]FactExpressionBinding, 0, len(value.leaves))
	for _, leaf := range value.leaves {
		bindings = append(bindings, FactExpressionBinding{Node: leaf.expression.ID, Subject: plan.handleForExpression(leaf.expression)})
	}
	return bindings
}

func (plan actionBindingPlan) preconditionInputs(result bool) []FactActionPreconditionInput {
	inputs := make([]FactActionPreconditionInput, 0, len(plan.preconditions))
	for _, precondition := range plan.preconditions {
		inputs = append(inputs, FactActionPreconditionInput{
			Precondition: precondition.precondition.ID, Subject: "subject/action",
			Expression: cloneIRExpression(precondition.precondition.Predicate),
			Bindings:   plan.expressionBindings(precondition.value), Evaluation: "pre-state", Result: result,
		})
	}
	return inputs
}

func (plan actionBindingPlan) sourceNodes() []SemanticID {
	sources := []SemanticID{plan.entity.ID, plan.entity.State.ID, plan.action.ID}
	for _, relation := range plan.relations {
		sources = append(sources, relation.field.ID, relation.entity.ID)
	}
	for _, precondition := range plan.preconditions {
		sources = append(sources, precondition.precondition.ID)
		sources = append(sources, expressionSemanticIDs(precondition.precondition.Predicate)...)
		for _, leaf := range precondition.value.leaves {
			sources = append(sources, leaf.entity.ID)
		}
	}
	return canonicalSemanticIDs(sources)
}

func (plan actionBindingPlan) preconditionOnlyRelations() []actionRelationBinding {
	var result []actionRelationBinding
	for _, relation := range plan.relations {
		if relation.owner == "precondition" {
			result = append(result, relation)
		}
	}
	return result
}

type changeValueLeaf struct {
	expression IRExpression
	entity     IREntity
	field      IRField
	relation   *IRField
}

func (b *acceptanceBuilder) resolveChangeValueExpression(entity IREntity, expression IRExpression) (resolvedChangeValue, error) {
	result := resolvedChangeValue{expression: cloneIRExpression(expression)}
	for _, node := range expressionFieldReferenceNodes(expression) {
		valueEntity := entity
		var relationField *IRField
		if len(node.RelationPath) == 1 {
			field, ok := findIRFieldByID(entity, node.RelationPath[0])
			if !ok || field.Relation == nil {
				return resolvedChangeValue{}, fmt.Errorf("build Acceptance Facts: change value %s has invalid relation value", node.ID)
			}
			relationField = &field
			valueEntity = b.entities[field.Relation.Entity]
		}
		valueField, ok := findIRFieldByID(valueEntity, node.Field)
		if !ok {
			return resolvedChangeValue{}, fmt.Errorf("build Acceptance Facts: change value %s has missing value field", node.ID)
		}
		result.leaves = append(result.leaves, changeValueLeaf{expression: node, entity: valueEntity, field: valueField, relation: relationField})
	}
	if len(result.leaves) == 0 {
		return resolvedChangeValue{}, fmt.Errorf("build Acceptance Facts: change value %s has no field references", expression.ID)
	}
	return result, nil
}

func distinctChangeValueRelations(leaves []changeValueLeaf, targetRelation *IRField) []changeValueLeaf {
	byID := map[SemanticID]changeValueLeaf{}
	for _, leaf := range leaves {
		if leaf.relation == nil || targetRelation != nil && leaf.relation.ID == targetRelation.ID {
			continue
		}
		byID[leaf.relation.ID] = leaf
	}
	result := make([]changeValueLeaf, 0, len(byID))
	for _, leaf := range byID {
		result = append(result, leaf)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].relation.ID < result[j].relation.ID })
	return result
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
	value resolvedChangeValue,
	action IRAction,
	change IRActionChange,
	ref *IRActionRef,
	source string,
	plan actionBindingPlan,
) AcceptanceFact {
	subject, kind, input, sources := changeFactBase(entity, targetEntity, relationField, value, action, change, ref, source, "", plan)
	return AcceptanceFact{
		ID: factID(subject, "changes", "accepted", "from", source), Kind: kind + "accepted", Subject: subject,
		Setup: input.setup, Input: &FactInput{Action: input.action, Preconditions: plan.preconditionInputs(true), Invariants: invariantInputs(targetEntity, input.targetHandle, "")},
		Expected: FactExpectation{
			Outcome: "accepted", Atomicity: "all-changes-committed", AppliedMutations: 2,
			Enforcement: "authoritative", Subjects: changeCommittedSubjects(entity, targetField, value, action, input.targetHandle, input.bindings),
		},
		SourceNodes: append(sources, invariantSources(targetEntity)...),
	}
}

func changeInvariantRejectedFact(
	entity, targetEntity IREntity,
	relationField *IRField,
	value resolvedChangeValue,
	action IRAction,
	change IRActionChange,
	violated IRInvariant,
	ref *IRActionRef,
	source string,
	plan actionBindingPlan,
) AcceptanceFact {
	subject, kind, input, sources := changeFactBase(entity, targetEntity, relationField, value, action, change, ref, source, "", plan)
	var feedback []string
	if ref != nil {
		feedback = []string{"invalid"}
	}
	return AcceptanceFact{
		ID:   factID(subject, "changes", "invariant", string(violated.ID), "rejected", "from", source),
		Kind: kind + "invariant-rejected", Subject: subject,
		Setup: input.setup, Input: &FactInput{Action: input.action, Preconditions: plan.preconditionInputs(true), Invariants: invariantInputs(targetEntity, input.targetHandle, violated.ID)},
		Expected: FactExpectation{
			Outcome: "rejected", Reason: "invariant-violated", Violated: violated.ID, Feedback: feedback,
			Atomicity: "no-changes-committed", AppliedMutations: 0, Enforcement: "authoritative",
			Subjects: unchangedChangeSubjects(entity, action, source, input.targetHandle),
		},
		SourceNodes: append(sources, invariantSources(targetEntity)...),
	}
}

func changeValueUnavailableFact(
	entity, targetEntity IREntity,
	targetRelation *IRField,
	value resolvedChangeValue,
	unavailable changeValueLeaf,
	action IRAction,
	change IRActionChange,
	ref *IRActionRef,
	source string,
	plan actionBindingPlan,
) AcceptanceFact {
	subject, kind, input, sources := changeFactBase(entity, targetEntity, targetRelation, value, action, change, ref, source, unavailable.relation.ID, plan)
	var feedback []string
	if ref != nil {
		feedback = []string{"failure"}
	}
	return AcceptanceFact{
		ID: factID(subject, "changes", "value-unavailable", "via", unavailable.relation.Name, "from", source), Kind: kind + "value-unavailable", Subject: subject,
		Setup: input.setup, Input: &FactInput{Action: input.action},
		Expected: FactExpectation{
			Outcome: "rejected", Reason: "value-unavailable", Feedback: feedback,
			Atomicity: "no-changes-committed", AppliedMutations: 0, Enforcement: "authoritative",
			Subjects: unchangedChangeSubjects(entity, action, source, input.targetHandle),
		},
		SourceNodes: sources,
	}
}

func changeTargetUnavailableFact(
	entity, targetEntity IREntity,
	relation IRField,
	value resolvedChangeValue,
	action IRAction,
	change IRActionChange,
	ref *IRActionRef,
	source string,
	plan actionBindingPlan,
) AcceptanceFact {
	subject, kind, input, sources := changeFactBase(entity, targetEntity, &relation, value, action, change, ref, source, relation.ID, plan)
	var feedback []string
	if ref != nil {
		feedback = []string{"failure"}
	}
	return AcceptanceFact{
		ID: factID(subject, "changes", "target-unavailable", "from", source), Kind: kind + "target-unavailable", Subject: subject,
		Setup: input.setup,
		Input: &FactInput{Action: input.action},
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
	bindings     []FactExpressionBinding
}

func changeFactBase(
	entity, targetEntity IREntity,
	targetRelation *IRField,
	value resolvedChangeValue,
	action IRAction,
	change IRActionChange,
	ref *IRActionRef,
	source string,
	unavailableRelation SemanticID,
	plan actionBindingPlan,
) (SemanticID, string, changeFactInput, []SemanticID) {
	subject := action.ID
	kind := "changes-"
	actionInput := &FactActionInput{Action: action.ID, Subject: "subject/action", Dispatches: 1}
	sources := []SemanticID{entity.ID, entity.State.ID, action.ID, change.ID, change.Target.ID, change.Target.Field}
	sources = append(sources, expressionSemanticIDs(value.expression)...)
	setup := plan.setup(source, unavailableRelation)
	targetHandle := "subject/action"
	if targetRelation != nil {
		targetHandle = "subject/target"
		sources = append(sources, targetRelation.ID, targetEntity.ID)
	}
	bindings := plan.expressionBindings(value)
	sources = append(sources, plan.sourceNodes()...)
	if ref != nil {
		subject = ref.ID
		kind = "action-changes-"
		actionInput.Reference = ref.ID
		sources = append(sources, ref.ID)
	}
	return subject, kind, changeFactInput{setup: setup, action: actionInput, targetHandle: targetHandle, bindings: bindings}, canonicalSemanticIDs(sources)
}

func changeCommittedSubjects(entity IREntity, targetField IRField, value resolvedChangeValue, action IRAction, targetHandle string, bindings []FactExpressionBinding) []FactSubjectExpectation {
	subject := FactSubjectExpectation{
		Handle: "subject/action", State: &IRStateValueRef{State: entity.State.ID, Value: action.Destination},
	}
	field := FactFieldExpectation{
		Field: targetField.ID, Stored: "expression-result", Expression: &FactExpressionExpectation{
			Tree: cloneIRExpression(value.expression), Evaluation: "pre-state", Bindings: append([]FactExpressionBinding(nil), bindings...),
		},
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
