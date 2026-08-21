package compiler

import "fmt"

func (c *checker) checkActionBody(action *ActionDecl, entity *EntityDecl) {
	if !action.HasBody {
		return
	}
	if len(action.Preconditions) == 0 && len(action.Changes) == 0 {
		c.error(action.Span, "F2810", fmt.Sprintf("action `%s.%s` has an empty body", entity.Name.Text, action.Name.Text), "declare one `precondition`, one non-empty `changes` block, or remove the action body")
		return
	}
	c.checkActionPreconditions(action, entity)
	c.checkActionChanges(action, entity)
}

func (c *checker) checkActionPreconditions(action *ActionDecl, entity *EntityDecl) {
	seen := map[string]Span{}
	for _, precondition := range action.Preconditions {
		if previous, ok := seen[precondition.Name.Text]; ok {
			c.error(precondition.Name.Span, "F2810", fmt.Sprintf("duplicate precondition `%s.%s.%s`", entity.Name.Text, action.Name.Text, precondition.Name.Text), fmt.Sprintf("the first declaration is at line %d", previous.Start.Line))
		} else {
			seen[precondition.Name.Text] = precondition.Name.Span
		}
	}
	if len(action.Preconditions) > 1 {
		c.error(action.Span, "F2810", fmt.Sprintf("action `%s.%s` has %d preconditions", entity.Name.Text, action.Name.Text, len(action.Preconditions)), "the first Action Precondition slice permits at most one named precondition")
		return
	}
	if len(action.Preconditions) == 0 {
		return
	}
	precondition := action.Preconditions[0]
	predicate := precondition.Predicate
	if predicate == nil || predicate.Kind != "binary" || predicate.Binary == nil || predicate.Binary.Operator == "invalid" {
		return
	}
	if predicate.Binary.Operator != "less-than-or-equal" || predicate.Binary.Left == nil || predicate.Binary.Right == nil {
		c.error(predicate.Span, "F2811", "precondition is outside the first predicate slice", "use one `<=` comparison with at most one binary `+` across both operands")
		return
	}
	if countExpressionOperator(predicate, "add") > 1 {
		c.error(predicate.Span, "F2811", "precondition contains more than one numeric addition", "use at most one binary `+` across both sides of `<=`")
		return
	}
	left, leftOK := c.resolvePreconditionValueExpression(entity, predicate.Binary.Left)
	right, rightOK := c.resolvePreconditionValueExpression(entity, predicate.Binary.Right)
	if !leftOK || !rightOK {
		return
	}
	if left.Name != right.Name || !isOrderedExpressionType(left) || !isOrderedExpressionType(right) {
		c.error(predicate.Span, "F2813", fmt.Sprintf("operator `<=` cannot compare `%s` and `%s` in a precondition", left.Name, right.Name), "compare required fields with the same nominal Int-, Decimal-, Date-, or DateTime-based type")
		return
	}
	c.expressionTypes[predicate] = "Bool"
}

func countExpressionOperator(expression *Expression, operator string) int {
	if expression == nil || expression.Binary == nil {
		return 0
	}
	count := 0
	if expression.Binary.Operator == operator {
		count++
	}
	return count + countExpressionOperator(expression.Binary.Left, operator) + countExpressionOperator(expression.Binary.Right, operator)
}

func (c *checker) resolvePreconditionValueExpression(entity *EntityDecl, expression *Expression) (resolvedExpressionType, bool) {
	if expression == nil {
		return resolvedExpressionType{}, false
	}
	if expression.Kind == "field" {
		return c.resolvePreconditionField(entity, expression)
	}
	if expression.Kind != "binary" || expression.Binary == nil || expression.Binary.Operator != "add" ||
		expression.Binary.Left == nil || expression.Binary.Left.Kind != "field" ||
		expression.Binary.Right == nil || expression.Binary.Right.Kind != "field" {
		c.error(expression.Span, "F2811", "precondition operand is outside the first numeric expression slice", "use a required field reference or exactly two required field references joined by one `+`")
		return resolvedExpressionType{}, false
	}
	left, leftOK := c.resolvePreconditionField(entity, expression.Binary.Left)
	right, rightOK := c.resolvePreconditionField(entity, expression.Binary.Right)
	if !leftOK || !rightOK {
		return resolvedExpressionType{}, false
	}
	if left.Name != right.Name || left.Kind != "scalar" || right.Kind != "scalar" ||
		(left.Base != "Int" && left.Base != "Decimal") || right.Base != left.Base {
		c.error(expression.Span, "F2814", fmt.Sprintf("operator `+` cannot add `%s` and `%s` in a precondition", left.Name, right.Name), "add required fields with the same nominal Int- or Decimal-based type")
		return resolvedExpressionType{}, false
	}
	if !c.additionTypeIsClosedFor(left.Name, expression.Span, "F2814") {
		return resolvedExpressionType{}, false
	}
	c.expressionTypes[expression] = left.Name
	return left, true
}

func (c *checker) resolvePreconditionField(entity *EntityDecl, expression *Expression) (resolvedExpressionType, bool) {
	if expression == nil || expression.Field == nil || len(expression.Field.Path) == 0 {
		return resolvedExpressionType{}, false
	}
	path := expression.Field.Path
	if len(path) > 2 {
		c.error(expression.Span, "F2812", "precondition field traverses more than one relation", "use a required self scalar or one required to-one relation followed by a required scalar field")
		return resolvedExpressionType{}, false
	}
	owner := entity
	var relations []*FieldDecl
	if len(path) == 2 {
		relation, isState := declaredField(entity, path[0].Text)
		if relation == nil {
			kind := "field"
			if isState {
				kind = "state"
			}
			c.error(path[0].Span, "F2812", fmt.Sprintf("precondition binding `%s.%s` is not a required to-one relation %s", entity.Name.Text, path[0].Text, kind), "use one required to-one relation from the action entity")
			return resolvedExpressionType{}, false
		}
		resolvedRelation := c.resolveType(relation.Type.Name.Text, relation.Type.Name.Span)
		if relation.Type.Collection || resolvedRelation.Kind != "entity" || !hasFieldModifier(relation, "required") {
			c.error(path[0].Span, "F2812", fmt.Sprintf("precondition binding `%s.%s` is not a required to-one relation", entity.Name.Text, path[0].Text), "use one required to-one relation from the action entity")
			return resolvedExpressionType{}, false
		}
		owner = c.entities[resolvedRelation.Name]
		if owner == nil {
			return resolvedExpressionType{}, false
		}
		relations = []*FieldDecl{relation}
	}
	name := path[len(path)-1]
	field, isState := declaredField(owner, name.Text)
	if field == nil {
		kind := "field"
		if isState {
			kind = "state"
		}
		c.error(name.Span, "F2812", fmt.Sprintf("precondition value `%s.%s` is not a required scalar %s", owner.Name.Text, name.Text, kind), "use a required scalar field on self or one required to-one relation")
		return resolvedExpressionType{}, false
	}
	resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
	if field.Type.Collection || resolved.Kind != "scalar" || !hasFieldModifier(field, "required") {
		c.error(name.Span, "F2812", fmt.Sprintf("precondition value `%s.%s` is not a required scalar field", owner.Name.Text, name.Text), "use a required scalar field on self or one required to-one relation")
		return resolvedExpressionType{}, false
	}
	c.expressionFields[expression] = field
	c.expressionFieldOwners[expression] = owner
	c.expressionRelationPaths[expression] = relations
	c.expressionTypes[expression] = field.Type.Name.Text
	return resolvedExpressionType{Name: field.Type.Name.Text, Kind: resolved.Kind, Base: resolved.Base, Valid: true}, true
}
