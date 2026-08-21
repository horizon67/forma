package compiler

import "fmt"

func (c *checker) checkActionChanges(action *ActionDecl, entity *EntityDecl) {
	if !action.HasBody {
		return
	}
	if len(action.Changes) != 1 {
		c.error(action.Span, "F2801", fmt.Sprintf("action `%s.%s` body has %d changes blocks", entity.Name.Text, action.Name.Text, len(action.Changes)), "declare exactly one non-empty `changes` block or remove the action body")
		return
	}
	block := action.Changes[0]
	if len(block.Assignments) != 1 {
		c.error(block.Span, "F2802", fmt.Sprintf("changes block has %d assignments", len(block.Assignments)), "the first Changes slice requires exactly one assignment")
		return
	}
	assignment := block.Assignments[0]
	targetEntity, relations, targetField, ok := c.resolveChangeTarget(entity, assignment)
	if !ok {
		return
	}
	valueEntity, valueRelations, valueField, ok := c.resolveChangeValue(entity, assignment.Value)
	if !ok {
		return
	}
	if targetField.Type.Name.Text != valueField.Type.Name.Text {
		c.error(assignment.Span, "F2806", fmt.Sprintf("change assigns `%s` to `%s`", valueField.Type.Name.Text, targetField.Type.Name.Text), "assign a required self field with the same nominal scalar type as the target")
		return
	}
	c.resolvedChanges[assignment] = resolvedChange{
		ActionEntity:       entity,
		TargetEntity:       targetEntity,
		TargetRelationPath: relations,
		TargetField:        targetField,
		ValueEntity:        valueEntity,
		ValueRelationPath:  valueRelations,
		ValueField:         valueField,
	}
}

func (c *checker) resolveChangeTarget(actionEntity *EntityDecl, assignment *ChangeAssignmentDecl) (*EntityDecl, []*FieldDecl, *FieldDecl, bool) {
	if assignment == nil || assignment.Target == nil || len(assignment.Target.Path) == 0 {
		return nil, nil, nil, false
	}
	path := assignment.Target.Path
	if len(path) > 2 {
		c.error(assignment.Target.Span, "F2803", "change target traverses more than one relation", "target a self field or one required to-one relation followed by a stored scalar field")
		return nil, nil, nil, false
	}
	if len(path) == 1 {
		field, isState := declaredField(actionEntity, path[0].Text)
		if field == nil {
			kind := "field"
			if isState {
				kind = "state"
			}
			c.error(path[0].Span, "F2804", fmt.Sprintf("change target `%s.%s` is not a mutable stored scalar %s", actionEntity.Name.Text, path[0].Text, kind), "target a stored scalar field; state is owned by the action transition")
			return nil, nil, nil, false
		}
		if !c.mutableChangeScalar(field) {
			c.error(path[0].Span, "F2804", fmt.Sprintf("change target `%s.%s` is not a mutable stored scalar field", actionEntity.Name.Text, path[0].Text), "remove readonly, relation, collection, or non-scalar targets")
			return nil, nil, nil, false
		}
		return actionEntity, nil, field, true
	}
	relation, isState := declaredField(actionEntity, path[0].Text)
	if relation == nil {
		kind := "field"
		if isState {
			kind = "state"
		}
		c.error(path[0].Span, "F2803", fmt.Sprintf("change target binding `%s.%s` is not a required to-one relation %s", actionEntity.Name.Text, path[0].Text, kind), "use one required to-one relation from the action entity")
		return nil, nil, nil, false
	}
	resolvedRelation := c.resolveType(relation.Type.Name.Text, relation.Type.Name.Span)
	if relation.Type.Collection || resolvedRelation.Kind != "entity" || !hasFieldModifier(relation, "required") {
		c.error(path[0].Span, "F2803", fmt.Sprintf("change target binding `%s.%s` is not a required to-one relation", actionEntity.Name.Text, path[0].Text), "use one required to-one relation from the action entity")
		return nil, nil, nil, false
	}
	targetEntity := c.entities[resolvedRelation.Name]
	if targetEntity == nil {
		return nil, nil, nil, false
	}
	target, isState := declaredField(targetEntity, path[1].Text)
	if target == nil {
		kind := "field"
		if isState {
			kind = "state"
		}
		c.error(path[1].Span, "F2804", fmt.Sprintf("change target `%s.%s` is not a mutable stored scalar %s", targetEntity.Name.Text, path[1].Text, kind), "target a stored scalar field; state is owned by its own action transition")
		return nil, nil, nil, false
	}
	if !c.mutableChangeScalar(target) {
		c.error(path[1].Span, "F2804", fmt.Sprintf("change target `%s.%s` is not a mutable stored scalar field", targetEntity.Name.Text, path[1].Text), "remove readonly, relation, collection, or non-scalar targets")
		return nil, nil, nil, false
	}
	return targetEntity, []*FieldDecl{relation}, target, true
}

func (c *checker) resolveChangeValue(entity *EntityDecl, expression *Expression) (*EntityDecl, []*FieldDecl, *FieldDecl, bool) {
	if expression == nil || expression.Field == nil || len(expression.Field.Path) == 0 {
		return nil, nil, nil, false
	}
	path := expression.Field.Path
	if len(path) > 2 {
		c.error(expression.Span, "F2805", "change value traverses more than one relation", "use a required self scalar or one required to-one relation followed by a required scalar field")
		return nil, nil, nil, false
	}
	valueEntity := entity
	var relations []*FieldDecl
	if len(path) == 2 {
		relation, isState := declaredField(entity, path[0].Text)
		if relation == nil {
			kind := "field"
			if isState {
				kind = "state"
			}
			c.error(path[0].Span, "F2805", fmt.Sprintf("change value binding `%s.%s` is not a required to-one relation %s", entity.Name.Text, path[0].Text, kind), "use one required to-one relation from the action entity")
			return nil, nil, nil, false
		}
		resolvedRelation := c.resolveType(relation.Type.Name.Text, relation.Type.Name.Span)
		if relation.Type.Collection || resolvedRelation.Kind != "entity" || !hasFieldModifier(relation, "required") {
			c.error(path[0].Span, "F2805", fmt.Sprintf("change value binding `%s.%s` is not a required to-one relation", entity.Name.Text, path[0].Text), "use one required to-one relation from the action entity")
			return nil, nil, nil, false
		}
		valueEntity = c.entities[resolvedRelation.Name]
		if valueEntity == nil {
			return nil, nil, nil, false
		}
		relations = []*FieldDecl{relation}
	}
	name := path[len(path)-1]
	field, isState := declaredField(valueEntity, name.Text)
	if field == nil {
		kind := "field"
		if isState {
			kind = "state"
		}
		c.error(name.Span, "F2805", fmt.Sprintf("change value `%s.%s` is not a required scalar %s", valueEntity.Name.Text, name.Text, kind), "use a required scalar field on self or one required to-one relation")
		return nil, nil, nil, false
	}
	resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
	if field.Type.Collection || resolved.Kind != "scalar" || !hasFieldModifier(field, "required") {
		c.error(name.Span, "F2805", fmt.Sprintf("change value `%s.%s` is not a required scalar field", valueEntity.Name.Text, name.Text), "use a required scalar field on self or one required to-one relation")
		return nil, nil, nil, false
	}
	c.expressionFields[expression] = field
	c.expressionFieldOwners[expression] = valueEntity
	c.expressionRelationPaths[expression] = relations
	c.expressionTypes[expression] = field.Type.Name.Text
	return valueEntity, relations, field, true
}

func (c *checker) mutableChangeScalar(field *FieldDecl) bool {
	resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
	return !field.Type.Collection && resolved.Kind == "scalar" && !hasFieldModifier(field, "readonly")
}

func declaredField(entity *EntityDecl, name string) (*FieldDecl, bool) {
	for _, field := range entity.Fields {
		if field.Name.Text == name {
			return field, false
		}
	}
	return nil, entity.State != nil && entity.State.Name.Text == name
}
