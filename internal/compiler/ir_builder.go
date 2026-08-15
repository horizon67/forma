package compiler

func (c *checker) buildIR() *SemanticIR {
	ir := &SemanticIR{Version: "forma/v0.3"}
	for _, role := range c.program.Roles {
		if c.roles[role.Name.Text] == role {
			ir.Roles = append(ir.Roles, role.Name.Text)
		}
	}
	for _, decl := range c.program.Types {
		if c.types[decl.Name.Text] != decl {
			continue
		}
		resolved := c.resolveType(decl.Name.Text, decl.Name.Span)
		item := IRType{Name: decl.Name.Text, Kind: resolved.Kind, Base: resolved.Base}
		if resolved.Kind == "union" {
			item.Variants = append([]string(nil), resolved.Variants...)
		}
		for _, mod := range decl.Mods {
			item.Constraints = append(item.Constraints, IRConstraint{Kind: mod.Kind, Value: mod.Value})
		}
		ir.Types = append(ir.Types, item)
	}
	for _, entity := range c.program.Entities {
		if c.entities[entity.Name.Text] != entity {
			continue
		}
		item := IREntity{Name: entity.Name.Text, Label: c.entityLabels[entity.Name.Text]}
		for _, field := range entity.Fields {
			fieldIR := IRField{Name: field.Name.Text, Type: field.Type.Name.Text, Collection: field.Type.Collection}
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
				fieldIR.Relation = &IRRelation{Entity: resolved.Name, Label: c.entityLabels[resolved.Name]}
			}
			item.Fields = append(item.Fields, fieldIR)
		}
		if entity.State != nil {
			state := &IRState{
				Name: entity.State.Name.Text, Initial: entity.State.Initial.Text,
				Values: namesToStrings(entity.State.Values),
			}
			item.State = state
		}
		ir.Entities = append(ir.Entities, item)
	}
	for _, action := range c.program.Actions {
		if c.actions[actionKey(action.Entity.Text, action.Name.Text)] != action {
			continue
		}
		item := IRAction{
			Entity: action.Entity.Text, Name: action.Name.Text, Sources: namesToStrings(action.Sources), Destination: action.Destination.Text,
		}
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
		item := IRPage{Name: page.Name.Text, Allows: namesToStrings(page.Allows)}
		if page.Param != nil {
			item.Param = &IRParameter{Name: page.Param.Name.Text, Entity: page.Param.Type.Text}
		}
		for _, view := range page.Views {
			info := c.viewInfo[view]
			if info == nil || info.Entity == nil {
				continue
			}
			item.Views = append(item.Views, c.buildViewIR(info))
		}
		ir.Pages = append(ir.Pages, item)
	}
	return ir
}

func (c *checker) buildViewIR(info *viewInfo) IRView {
	view := IRView{Kind: string(info.View.Kind), Entity: info.Entity.Name.Text, Mode: info.Mode}
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
			view.Relations = c.relationChoices(info.Entity, mod.Names)
		}
		if mod, ok := mods["sort"]; ok && len(mod.Names) > 0 {
			direction := mod.Direction
			if direction == "" {
				direction = "asc"
			}
			view.Sort = &IRSort{Field: mod.Names[0].Text, Direction: direction, TieBreak: "identity"}
		}
		if mod, ok := mods["paginate"]; ok {
			view.PageSize = mod.PageSize
		}
		if mod, ok := mods["actions"]; ok {
			for _, name := range mod.Names {
				view.Actions = append(view.Actions, c.resolveActionRef(info, name, false))
			}
		}
	case ViewDetail:
		view.InteractionStates = []string{"loading", "ready", "empty", "failure"}
		if mod, ok := mods["fields"]; ok {
			view.Fields = namesToStrings(mod.Names)
		}
		if mod, ok := mods["actions"]; ok {
			for _, name := range mod.Names {
				view.Actions = append(view.Actions, c.resolveActionRef(info, name, false))
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
		view.Relations = c.relationChoicesFromStrings(info.Entity, view.Fields)
	}
	return view
}

func (c *checker) relationChoices(entity *EntityDecl, names []Name) []IRChoice {
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name.Text)
	}
	return c.relationChoicesFromStrings(entity, values)
}

func (c *checker) relationChoicesFromStrings(entity *EntityDecl, names []string) []IRChoice {
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
				choices = append(choices, IRChoice{Field: name, Entity: resolved.Name, Label: c.entityLabels[resolved.Name]})
			}
		}
	}
	return choices
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
