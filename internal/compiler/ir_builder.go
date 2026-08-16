package compiler

import "sort"

func (c *checker) buildIR() (*SemanticIR, *SourceMap) {
	ir := &SemanticIR{Version: SemanticIRVersion}
	sourceMap := newSourceMapBuilder()
	for _, role := range c.program.Roles {
		if c.roles[role.Name.Text] == role {
			id := roleID(role.Name.Text)
			ir.Roles = append(ir.Roles, IRRole{ID: id, Name: role.Name.Text})
			sourceMap.add(id, "role", role.Span)
		}
	}
	for _, decl := range c.program.Types {
		if c.types[decl.Name.Text] != decl {
			continue
		}
		resolved := c.resolveType(decl.Name.Text, decl.Name.Span)
		id := typeID(decl.Name.Text)
		item := IRType{ID: id, Name: decl.Name.Text, Kind: resolved.Kind, Base: resolved.Base}
		sourceMap.add(id, "type", decl.Span)
		if resolved.Kind == "union" {
			item.Variants = append([]string(nil), resolved.Variants...)
		}
		for _, mod := range decl.Mods {
			constraintID := semanticID(string(id), "constraint", mod.Kind)
			item.Constraints = append(item.Constraints, IRConstraint{ID: constraintID, Kind: mod.Kind, Value: mod.Value})
			sourceMap.add(constraintID, "constraint", mod.Span)
		}
		sort.Slice(item.Constraints, func(i, j int) bool { return item.Constraints[i].ID < item.Constraints[j].ID })
		ir.Types = append(ir.Types, item)
	}
	for _, entity := range c.program.Entities {
		if c.entities[entity.Name.Text] != entity {
			continue
		}
		id := entityID(entity.Name.Text)
		item := IREntity{ID: id, Name: entity.Name.Text, Label: c.entityLabels[entity.Name.Text]}
		sourceMap.add(id, "entity", entity.Span)
		for _, field := range entity.Fields {
			fieldID := semanticID(string(id), "field", field.Name.Text)
			fieldIR := IRField{ID: fieldID, Name: field.Name.Text, Type: field.Type.Name.Text, Collection: field.Type.Collection}
			sourceMap.add(fieldID, "field", field.Span)
			for _, mod := range field.Mods {
				switch mod.Kind {
				case "required":
					fieldIR.Required = true
				case "unique":
					fieldIR.Unique = true
				case "readonly":
					fieldIR.Readonly = true
				case "label":
					fieldIR.Label = true
				case "default":
					if mod.Value != nil {
						fieldIR.Default = &IRLiteral{Kind: mod.Value.Kind, Value: mod.Value.Value}
					}
				}
			}
			resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
			if resolved.Kind == "entity" {
				relationID := semanticID(string(fieldID), "relation")
				fieldIR.Relation = &IRRelation{ID: relationID, Entity: resolved.Name, Label: c.entityLabels[resolved.Name]}
				sourceMap.add(relationID, "relation", field.Type.Span)
			}
			item.Fields = append(item.Fields, fieldIR)
		}
		if entity.State != nil {
			stateID := semanticID(string(id), "state", entity.State.Name.Text)
			state := &IRState{
				ID: stateID, Name: entity.State.Name.Text, Initial: entity.State.Initial.Text,
				Values: namesToStrings(entity.State.Values),
			}
			item.State = state
			sourceMap.add(stateID, "state", entity.State.Span)
		}
		ir.Entities = append(ir.Entities, item)
	}
	for _, action := range c.program.Actions {
		if c.actions[actionKey(action.Entity.Text, action.Name.Text)] != action {
			continue
		}
		id := actionID(action.Entity.Text, action.Name.Text)
		item := IRAction{
			ID: id, Entity: action.Entity.Text, Name: action.Name.Text, Sources: namesToStrings(action.Sources), Destination: action.Destination.Text,
		}
		sourceMap.add(id, "action", action.Span)
		for _, mod := range action.Mods {
			switch mod.Kind {
			case "confirm":
				item.Confirm = true
			case "allow":
				item.Allows = namesToStrings(mod.Names)
			case "goto":
				if len(mod.Names) > 0 {
					item.Goto = mod.Names[0].Text
				}
			}
		}
		ir.Actions = append(ir.Actions, item)
	}
	for _, page := range c.program.Pages {
		if c.pages[page.Name.Text] != page {
			continue
		}
		id := pageID(page.Name.Text)
		item := IRPage{ID: id, Name: page.Name.Text, Allows: namesToStrings(page.Allows)}
		sourceMap.add(id, "page", page.Span)
		if page.Param != nil {
			parameterID := semanticID(string(id), "parameter")
			item.Param = &IRParameter{ID: parameterID, Name: page.Param.Name.Text, Entity: page.Param.Type.Text}
			sourceMap.add(parameterID, "parameter", page.Param.Span)
		}
		for _, view := range page.Views {
			info := c.viewInfo[view]
			if info == nil || info.Entity == nil {
				continue
			}
			item.Views = append(item.Views, c.buildViewIR(info, sourceMap))
		}
		ir.Pages = append(ir.Pages, item)
	}
	sort.Slice(ir.Roles, func(i, j int) bool { return ir.Roles[i].ID < ir.Roles[j].ID })
	sort.Slice(ir.Types, func(i, j int) bool { return ir.Types[i].ID < ir.Types[j].ID })
	sort.Slice(ir.Entities, func(i, j int) bool { return ir.Entities[i].ID < ir.Entities[j].ID })
	sort.Slice(ir.Actions, func(i, j int) bool { return ir.Actions[i].ID < ir.Actions[j].ID })
	sort.Slice(ir.Pages, func(i, j int) bool { return ir.Pages[i].ID < ir.Pages[j].ID })
	return ir, sourceMap.build()
}

func (c *checker) buildViewIR(info *viewInfo, sourceMap *sourceMapBuilder) IRView {
	id := viewSemanticID(info)
	view := IRView{ID: id, Kind: string(info.View.Kind), Entity: info.Entity.Name.Text, Mode: info.Mode}
	sourceMap.add(id, string(info.View.Kind), info.View.Span)
	if info.Page.Param != nil && info.View.Subject.Text == info.Page.Param.Name.Text {
		view.Binding = info.Page.Param.Name.Text
	}
	mods := map[string]ViewModifier{}
	for _, mod := range info.View.Mods {
		if _, exists := mods[mod.Kind]; !exists {
			mods[mod.Kind] = mod
		}
	}
	switch info.View.Kind {
	case ViewList:
		view.InteractionStates = []string{"loading", "ready", "empty", "failure"}
		if mod, ok := mods["columns"]; ok {
			view.Fields = namesToStrings(mod.Names)
		}
		if mod, ok := mods["search"]; ok {
			view.Search = namesToStrings(mod.Names)
		}
		if mod, ok := mods["filter"]; ok {
			view.Filters = namesToStrings(mod.Names)
			view.Relations = c.relationChoices(id, info.Entity, mod.Names)
		}
		if mod, ok := mods["sort"]; ok && len(mod.Names) > 0 {
			direction := mod.Direction
			if direction == "" {
				direction = "asc"
			}
			sortID := semanticID(string(id), "sort")
			view.Sort = &IRSort{ID: sortID, Field: mod.Names[0].Text, Direction: direction, TieBreak: "identity"}
			sourceMap.add(sortID, "sort", mod.Span)
		}
		if mod, ok := mods["paginate"]; ok {
			view.PageSize = mod.PageSize
		}
		if mod, ok := mods["actions"]; ok {
			for _, name := range mod.Names {
				ref := c.resolveActionRef(info, name, false)
				view.Actions = append(view.Actions, ref)
				sourceMap.add(ref.ID, "action-reference", name.Span)
				sourceMap.add(ref.Access.ID, "access", name.Span)
			}
		}
	case ViewDetail:
		view.InteractionStates = []string{"loading", "ready", "empty", "failure"}
		if mod, ok := mods["fields"]; ok {
			view.Fields = namesToStrings(mod.Names)
		}
		if mod, ok := mods["actions"]; ok {
			for _, name := range mod.Names {
				ref := c.resolveActionRef(info, name, false)
				view.Actions = append(view.Actions, ref)
				sourceMap.add(ref.ID, "action-reference", name.Span)
				sourceMap.add(ref.Access.ID, "access", name.Span)
			}
		}
	case ViewForm:
		view.InteractionStates = []string{"ready", "invalid", "pending", "failure"}
		if mod, ok := mods["fields"]; ok {
			view.Fields = namesToStrings(mod.Names)
		} else {
			for _, field := range info.Entity.Fields {
				if !hasFieldModifier(field, "readonly") {
					view.Fields = append(view.Fields, field.Name.Text)
				}
			}
		}
		view.Relations = c.relationChoicesFromStrings(id, info.Entity, view.Fields)
		submit := c.resolveSubmitIntent(info, false)
		view.Submit = &submit
		submitSpan := info.View.Span
		if mod, ok := mods["submit"]; ok {
			submitSpan = mod.Span
		}
		sourceMap.add(submit.ID, "submit", submitSpan)
		sourceMap.add(submit.Success.ID, "navigation", submitSpan)
		sourceMap.add(submit.Access.ID, "access", submitSpan)
	}
	for _, relation := range view.Relations {
		sourceMap.add(relation.ID, "relation-choice", c.fieldSpan(info.Entity, relation.Field))
	}
	return view
}

func (c *checker) relationChoices(parentID SemanticID, entity *EntityDecl, names []Name) []IRChoice {
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name.Text)
	}
	return c.relationChoicesFromStrings(parentID, entity, values)
}

func (c *checker) relationChoicesFromStrings(parentID SemanticID, entity *EntityDecl, names []string) []IRChoice {
	var choices []IRChoice
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		for _, field := range entity.Fields {
			if field.Name.Text != name {
				continue
			}
			resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
			if resolved.Kind == "entity" {
				id := semanticID(string(parentID), "relation-choice", name)
				choices = append(choices, IRChoice{ID: id, Field: name, Entity: resolved.Name, Label: c.entityLabels[resolved.Name]})
			}
		}
	}
	return choices
}

func (c *checker) fieldSpan(entity *EntityDecl, name string) Span {
	for _, field := range entity.Fields {
		if field.Name.Text == name {
			return field.Name.Span
		}
	}
	return entity.Span
}

func namesToStrings(names []Name) []string {
	if len(names) == 0 {
		return nil
	}
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name.Text)
	}
	return values
}
