package compiler

import (
	"fmt"
	"reflect"
)

func validatePreconditionSemantics(intent *ResolvedIntent) error {
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
		if len(action.Preconditions) > 1 {
			return fmt.Errorf("validate Resolved Intent: action %s is outside the first Action Precondition slice", action.ID)
		}
		for _, precondition := range action.Preconditions {
			wantID := actionPreconditionID(action.ID, precondition.Name)
			if precondition.Name == "" || precondition.ID != wantID || precondition.Evaluation != "pre-state" {
				return fmt.Errorf("validate Resolved Intent: precondition %s has non-canonical identity or evaluation", precondition.ID)
			}
			predicate := precondition.Predicate
			wantPredicateID := semanticID(string(wantID), "expression")
			canonical := IRExpression{
				ID: wantPredicateID, Kind: "binary-expression", ResultType: "Bool", Operator: "less-than-or-equal",
				Left: predicate.Left, Right: predicate.Right,
			}
			if predicate.Left == nil || predicate.Right == nil || !reflect.DeepEqual(predicate, canonical) ||
				countIRExpressionOperator(predicate, "add") > 1 {
				return fmt.Errorf("validate Resolved Intent: precondition %s is outside the first <= predicate slice", precondition.ID)
			}
			entity, ok := entities[action.Entity]
			if !ok {
				return fmt.Errorf("validate Resolved Intent: precondition %s has no action entity", precondition.ID)
			}
			leftType, err := validatePreconditionValueExpression(entity, fields, fieldOwners, *predicate.Left, semanticID(string(wantPredicateID), "left"), types)
			if err != nil {
				return err
			}
			rightType, err := validatePreconditionValueExpression(entity, fields, fieldOwners, *predicate.Right, semanticID(string(wantPredicateID), "right"), types)
			if err != nil {
				return err
			}
			if leftType != rightType || !isOrderedIRScalar(leftType, types) {
				return fmt.Errorf("validate Resolved Intent: precondition %s operands are not the same ordered scalar type", precondition.ID)
			}
		}
	}
	return nil
}

func validatePreconditionValueExpression(
	entity IREntity,
	fields map[SemanticID]IRField,
	fieldOwners map[SemanticID]SemanticID,
	expression IRExpression,
	wantID SemanticID,
	types map[string]IRType,
) (string, error) {
	if expression.Kind == "binary-expression" {
		if expression.Operator != "add" || expression.Left == nil || expression.Right == nil ||
			expression.Left.Kind != "field-reference" || expression.Right.Kind != "field-reference" {
			return "", fmt.Errorf("validate Resolved Intent: precondition expression %s is outside the first numeric expression slice", expression.ID)
		}
		leftType, err := validatePreconditionValueExpression(entity, fields, fieldOwners, *expression.Left, semanticID(string(wantID), "left"), types)
		if err != nil {
			return "", err
		}
		rightType, err := validatePreconditionValueExpression(entity, fields, fieldOwners, *expression.Right, semanticID(string(wantID), "right"), types)
		if err != nil {
			return "", err
		}
		canonical := IRExpression{
			ID: wantID, Kind: "binary-expression", ResultType: leftType, Operator: "add",
			Left: expression.Left, Right: expression.Right,
		}
		if leftType != rightType || !isIRNumericType(leftType, types) || !reflect.DeepEqual(expression, canonical) {
			return "", fmt.Errorf("validate Resolved Intent: precondition expression %s is not a canonical nominal numeric addition", expression.ID)
		}
		if err := validateAdditionIRType(leftType, types); err != nil {
			return "", fmt.Errorf("validate Resolved Intent: precondition expression %s: %w", expression.ID, err)
		}
		return leftType, nil
	}
	canonical := IRExpression{
		ID: wantID, Kind: "field-reference", ResultType: expression.ResultType, Binding: "self",
		RelationPath: append([]SemanticID(nil), expression.RelationPath...), Field: expression.Field,
	}
	if expression.Field == "" || len(expression.RelationPath) > 1 || !reflect.DeepEqual(expression, canonical) {
		return "", fmt.Errorf("validate Resolved Intent: precondition expression %s is not a canonical field reference", expression.ID)
	}
	owner := entity.ID
	if len(expression.RelationPath) == 1 {
		relation, ok := fields[expression.RelationPath[0]]
		if !ok || fieldOwners[relation.ID] != entity.ID || relation.Collection || !relation.Required || relation.Relation == nil {
			return "", fmt.Errorf("validate Resolved Intent: precondition expression %s does not traverse one required to-one relation", expression.ID)
		}
		owner = entityID(relation.Relation.Entity)
	}
	field, ok := fields[expression.Field]
	if !ok || fieldOwners[field.ID] != owner || !field.Required || field.Collection || field.Relation != nil ||
		!isIRScalar(field.Type, types) || expression.ResultType != field.Type {
		return "", fmt.Errorf("validate Resolved Intent: precondition expression %s does not reference a required scalar field on its resolved owner", expression.ID)
	}
	return field.Type, nil
}

func countIRExpressionOperator(expression IRExpression, operator string) int {
	count := 0
	if expression.Operator == operator {
		count++
	}
	if expression.Left != nil {
		count += countIRExpressionOperator(*expression.Left, operator)
	}
	if expression.Right != nil {
		count += countIRExpressionOperator(*expression.Right, operator)
	}
	return count
}
