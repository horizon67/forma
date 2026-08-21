package compiler

import (
	"fmt"
	"reflect"
)

// validateActionSemantics protects both the existing transition contract and
// the closed first Changes slice after Resolved Intent crosses a process
// boundary.
func validateActionSemantics(intent *ResolvedIntent) error {
	entities := make(map[string]IREntity, len(intent.Entities))
	fields := map[SemanticID]IRField{}
	fieldOwners := map[SemanticID]SemanticID{}
	for _, entity := range intent.Entities {
		entities[entity.Name] = entity
		for _, field := range entity.Fields {
			fields[field.ID] = field
			fieldOwners[field.ID] = entity.ID
		}
	}
	types := make(map[string]IRType, len(intent.Types))
	for _, item := range intent.Types {
		types[item.Name] = item
	}
	for _, action := range intent.Actions {
		if action.ID != actionID(action.Entity, action.Name) {
			return fmt.Errorf("validate Resolved Intent: action %s has non-canonical identity", action.ID)
		}
		entity, ok := entities[action.Entity]
		if !ok || entity.State == nil {
			return fmt.Errorf("validate Resolved Intent: action %s has no stateful entity", action.ID)
		}
		stateValues := map[string]bool{}
		for _, value := range entity.State.Values {
			stateValues[value] = true
		}
		seenSources := map[string]bool{}
		for _, source := range action.Sources {
			if !stateValues[source] || seenSources[source] || source == action.Destination {
				return fmt.Errorf("validate Resolved Intent: action %s has a non-canonical source state", action.ID)
			}
			seenSources[source] = true
		}
		if len(action.Sources) == 0 || !stateValues[action.Destination] {
			return fmt.Errorf("validate Resolved Intent: action %s has a non-canonical destination state", action.ID)
		}
		if len(action.Changes) == 0 {
			if action.Atomicity != "" {
				return fmt.Errorf("validate Resolved Intent: action %s declares atomicity without Changes", action.ID)
			}
			continue
		}
		if len(action.Changes) != 1 || action.Atomicity != "all-or-nothing" {
			return fmt.Errorf("validate Resolved Intent: action %s is outside the first Changes slice", action.ID)
		}
		change := action.Changes[0]
		if change.Evaluation != "pre-state" || change.Target.Binding != "self" || len(change.Target.RelationPath) > 1 {
			return fmt.Errorf("validate Resolved Intent: change %s has non-canonical evaluation or target binding", change.ID)
		}
		targetField, ok := fields[change.Target.Field]
		if !ok || targetField.Collection || targetField.Relation != nil || targetField.Readonly || !isIRScalar(targetField.Type, types) {
			return fmt.Errorf("validate Resolved Intent: change %s does not target a mutable stored scalar field", change.ID)
		}
		targetPath := make([]string, 0, len(change.Target.RelationPath)+1)
		if len(change.Target.RelationPath) == 0 {
			if fieldOwners[targetField.ID] != entity.ID {
				return fmt.Errorf("validate Resolved Intent: change %s self target is outside %s", change.ID, entity.ID)
			}
		} else {
			relation, ok := fields[change.Target.RelationPath[0]]
			if !ok || fieldOwners[relation.ID] != entity.ID || relation.Collection || !relation.Required || relation.Relation == nil {
				return fmt.Errorf("validate Resolved Intent: change %s target path is not one required to-one relation", change.ID)
			}
			if fieldOwners[targetField.ID] != entityID(relation.Relation.Entity) {
				return fmt.Errorf("validate Resolved Intent: change %s target field does not belong to its relation entity", change.ID)
			}
			targetPath = append(targetPath, relation.Name)
		}
		targetPath = append(targetPath, targetField.Name)
		wantChangeID := actionChangeID(action.ID, targetPath)
		if change.ID != wantChangeID || change.Target.ID != semanticID(string(wantChangeID), "target") {
			return fmt.Errorf("validate Resolved Intent: change %s has non-canonical identity", change.ID)
		}
		valueField, err := validateChangeValueExpression(entity, fields, fieldOwners, change.Value, semanticID(string(wantChangeID), "value"), types)
		if err != nil {
			return err
		}
		if valueField.Type != targetField.Type {
			return fmt.Errorf("validate Resolved Intent: change %s assigns a different nominal type", change.ID)
		}
	}
	return nil
}

func validateChangeValueExpression(
	entity IREntity,
	fields map[SemanticID]IRField,
	fieldOwners map[SemanticID]SemanticID,
	expression IRExpression,
	wantID SemanticID,
	types map[string]IRType,
) (IRField, error) {
	if expression.ID != wantID || expression.Kind != "field-reference" || expression.Binding != "self" ||
		expression.Field == "" || expression.Operator != "" || expression.Left != nil || expression.Right != nil {
		return IRField{}, fmt.Errorf("validate Resolved Intent: change value %s is not a canonical self field reference", expression.ID)
	}
	field, ok := fields[expression.Field]
	if !ok || fieldOwners[field.ID] != entity.ID || !field.Required || field.Collection || field.Relation != nil ||
		!isIRScalar(field.Type, types) || expression.ResultType != field.Type {
		return IRField{}, fmt.Errorf("validate Resolved Intent: change value %s does not reference a required self scalar field", expression.ID)
	}
	return field, nil
}

func isIRScalar(name string, types map[string]IRType) bool {
	if _, ok := builtinTypes[name]; ok {
		return true
	}
	item, ok := types[name]
	return ok && item.Kind == "scalar"
}

func validateActionReferences(intent *ResolvedIntent) error {
	actions := make(map[SemanticID]IRAction, len(intent.Actions))
	for _, action := range intent.Actions {
		actions[action.ID] = action
	}
	for _, page := range intent.Pages {
		for _, view := range page.Views {
			for _, ref := range view.Actions {
				switch ref.Kind {
				case "transition":
					action, ok := actions[ref.Action]
					if !ok || action.Entity != view.Entity || action.Name != ref.Name || !reflect.DeepEqual(ref.InteractionStates, []string{"invalid", "failure"}) {
						return fmt.Errorf("validate Resolved Intent: action reference %s has a non-canonical transition binding or feedback", ref.ID)
					}
				case "standard":
					if ref.Action != "" {
						return fmt.Errorf("validate Resolved Intent: standard action reference %s binds a domain action", ref.ID)
					}
					wantStates := []string(nil)
					if ref.Name == "delete" {
						wantStates = []string{"failure"}
					}
					if _, ok := standardActions[ref.Name]; !ok || !reflect.DeepEqual(ref.InteractionStates, wantStates) {
						return fmt.Errorf("validate Resolved Intent: standard action reference %s has non-canonical feedback", ref.ID)
					}
				default:
					return fmt.Errorf("validate Resolved Intent: action reference %s has unsupported kind %q", ref.ID, ref.Kind)
				}
			}
		}
	}
	return nil
}
