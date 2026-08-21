package compiler

import (
	"fmt"
	"reflect"
)

// validateInvariantSemantics protects the closed first expression slice after
// parsing. Resolved Intent can cross a process boundary, so the request
// validator must not rely on the source checker having been the only producer.
func validateInvariantSemantics(intent *ResolvedIntent) error {
	types := make(map[string]IRType, len(intent.Types))
	for _, item := range intent.Types {
		types[item.Name] = item
	}
	for _, entity := range intent.Entities {
		fields := make(map[SemanticID]IRField, len(entity.Fields))
		for _, field := range entity.Fields {
			fields[field.ID] = field
		}
		for _, invariant := range entity.Invariants {
			if invariant.Name == "" || invariant.ID != invariantID(entity.Name, invariant.Name) {
				return fmt.Errorf("validate Resolved Intent: invariant %s has non-canonical identity", invariant.ID)
			}
			predicate := invariant.Predicate
			wantPredicateID := semanticID(string(invariant.ID), "expression")
			if predicate.Left == nil || predicate.Right == nil {
				return fmt.Errorf("validate Resolved Intent: invariant %s is not the supported self-only <= predicate", invariant.ID)
			}
			canonicalPredicate := IRExpression{
				ID: wantPredicateID, Kind: "binary-expression", ResultType: "Bool", Operator: "less-than-or-equal",
				Left: predicate.Left, Right: predicate.Right,
			}
			if !reflect.DeepEqual(predicate, canonicalPredicate) {
				return fmt.Errorf("validate Resolved Intent: invariant %s is not the supported self-only <= predicate", invariant.ID)
			}
			left, err := validateInvariantFieldReference(entity, fields, *predicate.Left, semanticID(string(wantPredicateID), "left"))
			if err != nil {
				return err
			}
			right, err := validateInvariantFieldReference(entity, fields, *predicate.Right, semanticID(string(wantPredicateID), "right"))
			if err != nil {
				return err
			}
			if left.Type != right.Type || !isOrderedIRScalar(left.Type, types) {
				return fmt.Errorf("validate Resolved Intent: invariant %s operands are not the same ordered scalar type", invariant.ID)
			}
		}
	}
	return nil
}

func validateInvariantFieldReference(entity IREntity, fields map[SemanticID]IRField, expression IRExpression, wantID SemanticID) (IRField, error) {
	canonical := IRExpression{
		ID: wantID, Kind: "field-reference", ResultType: expression.ResultType, Binding: "self", Field: expression.Field,
	}
	if expression.Field == "" || !reflect.DeepEqual(expression, canonical) {
		return IRField{}, fmt.Errorf("validate Resolved Intent: invariant expression %s is not a canonical self field reference", expression.ID)
	}
	field, ok := fields[expression.Field]
	if !ok || field.ID != semanticID(string(entity.ID), "field", field.Name) {
		return IRField{}, fmt.Errorf("validate Resolved Intent: invariant expression %s references a field outside %s", expression.ID, entity.ID)
	}
	if expression.ResultType != field.Type || !field.Required || field.Collection || field.Relation != nil {
		return IRField{}, fmt.Errorf("validate Resolved Intent: invariant expression %s does not reference a required local scalar field", expression.ID)
	}
	return field, nil
}

func isOrderedIRScalar(name string, types map[string]IRType) bool {
	seen := map[string]bool{}
	for !seen[name] {
		seen[name] = true
		switch name {
		case "Int", "Decimal", "Date", "DateTime":
			return true
		case "String", "Bool":
			return false
		}
		item, ok := types[name]
		if !ok || item.Kind != "scalar" || item.Base == "" {
			return false
		}
		name = item.Base
	}
	return false
}
