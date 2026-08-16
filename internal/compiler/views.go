package compiler

import (
	"fmt"
	"sort"
)

func (c *checker) indexViews() {
	for _, page := range c.program.Pages {
		if c.pages[page.Name.Text] != page {
			continue
		}
		var parameterEntity *EntityDecl
		if page.Param != nil {
			parameterEntity = c.entities[page.Param.Type.Text]
			if parameterEntity == nil {
				c.error(page.Param.Type.Span, "F2003", fmt.Sprintf("unknown page parameter entity `%s`", page.Param.Type.Text), "use an entity type for the page parameter")
			}
		}
		seenViewIDs := map[SemanticID]Span{}
		for _, view := range page.Views {
			info := &viewInfo{Page: page, View: view}
			switch view.Kind {
			case ViewList:
				if !startsUpper(view.Subject.Text) {
					c.error(view.Subject.Span, "F2401", "a list subject must be an entity name", "use `list Entity`")
					continue
				}
				info.Entity = c.entities[view.Subject.Text]
				if info.Entity == nil {
					c.error(view.Subject.Span, "F2003", fmt.Sprintf("unknown list entity `%s`", view.Subject.Text), "declare the entity before listing it")
					continue
				}
				c.lists[info.Entity.Name.Text] = append(c.lists[info.Entity.Name.Text], info)
			case ViewDetail:
				if page.Param == nil || parameterEntity == nil || view.Subject.Text != page.Param.Name.Text {
					c.error(view.Subject.Span, "F2401", fmt.Sprintf("detail subject `%s` is not this page's entity binding", view.Subject.Text), "declare a page parameter and pass that binding to `detail`")
					continue
				}
				info.Entity = parameterEntity
				info.Mode = "read"
				c.details[info.Entity.Name.Text] = append(c.details[info.Entity.Name.Text], info)
			case ViewForm:
				if startsUpper(view.Subject.Text) {
					info.Entity = c.entities[view.Subject.Text]
					if info.Entity == nil {
						c.error(view.Subject.Span, "F2003", fmt.Sprintf("unknown form entity `%s`", view.Subject.Text), "declare the entity before creating it")
						continue
					}
					info.Mode = "create"
					c.createForm[info.Entity.Name.Text] = append(c.createForm[info.Entity.Name.Text], info)
				} else {
					if page.Param == nil || parameterEntity == nil || view.Subject.Text != page.Param.Name.Text {
						c.error(view.Subject.Span, "F2401", fmt.Sprintf("form subject `%s` is not this page's entity binding", view.Subject.Text), "use `form Entity` to create or `form binding` to edit")
						continue
					}
					info.Entity = parameterEntity
					info.Mode = "edit"
					c.editForm[info.Entity.Name.Text] = append(c.editForm[info.Entity.Name.Text], info)
				}
			}
			c.viewInfo[view] = info
			id := viewSemanticID(info)
			if previous, exists := seenViewIDs[id]; exists {
				c.error(view.Subject.Span, "F2001", fmt.Sprintf("duplicate semantic view `%s`", id), fmt.Sprintf("the first view is at line %d; a page needs one canonical view for each kind, mode, and entity", previous.Start.Line))
			} else {
				seenViewIDs[id] = view.Span
			}
		}
	}
}

func (c *checker) checkPages() {
	for _, page := range c.program.Pages {
		if c.pages[page.Name.Text] != page {
			continue
		}
		c.checkRoles(page.Allows)
		for _, view := range page.Views {
			info := c.viewInfo[view]
			if info == nil || info.Entity == nil {
				continue
			}
			mods := c.viewModifierMap(view)
			switch view.Kind {
			case ViewList:
				c.checkList(info, mods)
			case ViewDetail:
				c.checkDetail(info, mods)
			case ViewForm:
				c.checkForm(info, mods)
			}
		}
	}
}

func (c *checker) viewModifierMap(view *ViewDecl) map[string]ViewModifier {
	mods := map[string]ViewModifier{}
	for _, mod := range view.Mods {
		if previous, exists := mods[mod.Kind]; exists {
			c.duplicateModifier(mod.Kind, mod.Span, previous.Span)
			continue
		}
		mods[mod.Kind] = mod
	}
	return mods
}

func (c *checker) checkList(info *viewInfo, mods map[string]ViewModifier) {
	for _, kind := range []string{"columns", "filter"} {
		if mod, ok := mods[kind]; ok {
			c.checkUniqueNames(mod.Names, kind)
			for _, name := range mod.Names {
				field, isState := c.findField(info.Entity, name)
				if field == nil && !isState {
					c.unknownField(info.Entity, name)
					continue
				}
				if kind == "filter" && field != nil && field.Type.Collection {
					c.error(name.Span, "F2406", "collection relationships cannot be filtered in v0", "filter by a scalar, state, or to-one relationship")
					continue
				}
				if field != nil {
					c.requireRelationLabel(field, name.Span, kind)
				}
			}
		}
	}
	if mod, ok := mods["search"]; ok {
		c.checkUniqueNames(mod.Names, "search")
		for _, name := range mod.Names {
			field, isState := c.findField(info.Entity, name)
			if field == nil {
				if !isState {
					c.unknownField(info.Entity, name)
				} else {
					c.error(name.Span, "F2403", "state cannot be searched", "use `filter` for exact state selection")
				}
				continue
			}
			resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
			if field.Type.Collection || resolved.Kind != "scalar" || resolved.Base != "String" {
				c.error(name.Span, "F2403", fmt.Sprintf("field `%s` is not String-based and cannot be searched", name.Text), "search only String-based scalar fields")
			}
		}
	}
	if mod, ok := mods["sort"]; ok && len(mod.Names) > 0 {
		name := mod.Names[0]
		field, isState := c.findField(info.Entity, name)
		if field == nil {
			if !isState {
				c.unknownField(info.Entity, name)
			} else {
				c.error(name.Span, "F2404", "state cannot be used for sort in v0", "sort by a scalar field")
			}
		} else {
			resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
			if field.Type.Collection || resolved.Kind != "scalar" {
				c.error(name.Span, "F2404", fmt.Sprintf("field `%s` is not a sortable scalar", name.Text), "sort by a built-in or named scalar field")
			}
		}
	}
	if mod, ok := mods["paginate"]; ok && mod.PageSize <= 0 {
		c.error(mod.Span, "F2407", "pagination size must be a positive integer", "use `paginate N` where N is greater than zero")
	}
	if mod, ok := mods["actions"]; ok {
		c.checkUniqueNames(mod.Names, "actions")
		for _, name := range mod.Names {
			c.resolveActionRef(info, name, true)
		}
	}
}

func (c *checker) checkDetail(info *viewInfo, mods map[string]ViewModifier) {
	if mod, ok := mods["fields"]; ok {
		c.checkUniqueNames(mod.Names, "fields")
		for _, name := range mod.Names {
			field, isState := c.findField(info.Entity, name)
			if field == nil && !isState {
				c.unknownField(info.Entity, name)
				continue
			}
			if field != nil {
				c.requireRelationLabel(field, name.Span, "detail")
			}
		}
	}
	if mod, ok := mods["actions"]; ok {
		c.checkUniqueNames(mod.Names, "actions")
		for _, name := range mod.Names {
			c.resolveActionRef(info, name, true)
		}
	}
}

func (c *checker) checkForm(info *viewInfo, mods map[string]ViewModifier) {
	fields := []*FieldDecl{}
	if mod, ok := mods["fields"]; ok {
		c.checkUniqueNames(mod.Names, "fields")
		for _, name := range mod.Names {
			field, isState := c.findField(info.Entity, name)
			if field == nil {
				if !isState {
					c.unknownField(info.Entity, name)
				} else {
					c.error(name.Span, "F2405", "state cannot be edited by a form", "change state through a declared domain action")
				}
				continue
			}
			fields = append(fields, field)
			c.checkFormField(field, name.Span)
		}
	} else {
		for _, field := range info.Entity.Fields {
			if !hasFieldModifier(field, "readonly") {
				fields = append(fields, field)
				c.checkFormField(field, field.Name.Span)
			}
		}
	}
	if info.Mode == "create" {
		selected := map[string]bool{}
		for _, field := range fields {
			selected[field.Name.Text] = true
		}
		for _, field := range info.Entity.Fields {
			if hasFieldModifier(field, "required") && !hasFieldModifier(field, "readonly") && !hasFieldModifier(field, "default") && !selected[field.Name.Text] {
				c.error(info.View.Span, "F2405", fmt.Sprintf("create form omits required field `%s`", field.Name.Text), "add the field or give it a default")
			}
		}
	}
	if mod, ok := mods["submit"]; ok && len(mod.Names) > 0 {
		expected := info.Mode
		if mod.Names[0].Text != expected {
			c.error(mod.Names[0].Span, "F2408", fmt.Sprintf("%s form must submit `%s`, not `%s`", info.Mode, expected, mod.Names[0].Text), fmt.Sprintf("write `submit %s`", expected))
		}
	}
	c.resolveSubmitIntent(info, true)
}

func (c *checker) checkFormField(field *FieldDecl, at Span) {
	if hasFieldModifier(field, "readonly") {
		c.error(at, "F2405", fmt.Sprintf("readonly field `%s` cannot appear in a form", field.Name.Text), "remove the field from the form")
	}
	c.requireRelationLabel(field, at, "form")
}

func (c *checker) requireRelationLabel(field *FieldDecl, at Span, context string) {
	resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
	if resolved.Kind != "entity" {
		return
	}
	if c.entityLabels[resolved.Name] == "" {
		c.error(at, "F2203", fmt.Sprintf("relationship `%s` presents `%s` without a label field", field.Name.Text, resolved.Name), fmt.Sprintf("mark one required String-based field on `%s` with `label` for %s", resolved.Name, context))
	}
}

func (c *checker) findField(entity *EntityDecl, name Name) (*FieldDecl, bool) {
	for _, field := range entity.Fields {
		if field.Name.Text == name.Text {
			return field, false
		}
	}
	return nil, entity.State != nil && entity.State.Name.Text == name.Text
}

func (c *checker) unknownField(entity *EntityDecl, name Name) {
	c.error(name.Span, "F2402", fmt.Sprintf("unknown field or state `%s.%s`", entity.Name.Text, name.Text), "use a field or named state declared by the entity")
}

func (c *checker) checkUniqueNames(names []Name, context string) {
	seen := map[string]Span{}
	for _, name := range names {
		if previous, exists := seen[name.Text]; exists {
			c.error(name.Span, "F2005", fmt.Sprintf("duplicate `%s` in %s", name.Text, context), fmt.Sprintf("the first occurrence is at line %d", previous.Start.Line))
		}
		seen[name.Text] = name.Span
	}
}

func (c *checker) resolveActionRef(info *viewInfo, name Name, report bool) (ref IRActionRef) {
	id := semanticID(string(viewSemanticID(info)), "action", name.Text)
	ref = IRActionRef{ID: id, Name: name.Text}
	var domainAction *ActionDecl
	defer func() {
		ref.Access = c.actionRefAccess(id, info, ref, domainAction)
	}()
	if _, standard := standardActions[name.Text]; standard {
		ref.Kind = "standard"
		switch name.Text {
		case "create":
			if info.View.Kind != ViewList {
				if report {
					c.error(name.Span, "F2501", "`create` is a collection action and can only appear on a list", "move `create` to the entity list")
				}
				return ref
			}
			ref.TargetPage = c.uniqueDestination(name, "create form", c.createForm[info.Entity.Name.Text], report)
			if len(c.details[info.Entity.Name.Text]) == 1 {
				ref.SuccessPage = c.details[info.Entity.Name.Text][0].Page.Name.Text
			} else if len(c.details[info.Entity.Name.Text]) > 1 {
				c.destinationCountError(name, "create success detail", len(c.details[info.Entity.Name.Text]), report)
			} else {
				ref.SuccessPage = info.Page.Name.Text
			}
		case "view":
			ref.TargetPage = c.uniqueDestination(name, "detail", c.details[info.Entity.Name.Text], report)
		case "edit":
			ref.TargetPage = c.uniqueDestination(name, "edit form", c.editForm[info.Entity.Name.Text], report)
			if len(c.details[info.Entity.Name.Text]) == 1 {
				ref.SuccessPage = c.details[info.Entity.Name.Text][0].Page.Name.Text
			} else if len(c.details[info.Entity.Name.Text]) > 1 {
				c.destinationCountError(name, "edit success detail", len(c.details[info.Entity.Name.Text]), report)
			} else {
				ref.SuccessPage = info.Page.Name.Text
			}
		case "delete":
			if info.View.Kind == ViewList {
				ref.SuccessPage = info.Page.Name.Text
			} else {
				ref.SuccessPage = c.uniqueDestination(name, "delete return list", c.lists[info.Entity.Name.Text], report)
			}
		}
		return ref
	}
	action := c.actions[actionKey(info.Entity.Name.Text, name.Text)]
	if action == nil {
		if report {
			c.error(name.Span, "F2501", fmt.Sprintf("unknown action `%s.%s`", info.Entity.Name.Text, name.Text), "declare the domain action or use a standard action")
		}
		return ref
	}
	domainAction = action
	ref.Kind = "transition"
	ref.SuccessPage = info.Page.Name.Text
	for _, mod := range action.Mods {
		if mod.Kind == "goto" && len(mod.Names) > 0 {
			ref.SuccessPage = mod.Names[0].Text
		}
	}
	return ref
}

func (c *checker) resolveSubmitIntent(info *viewInfo, report bool) IRSubmitIntent {
	viewID := viewSemanticID(info)
	id := semanticID(string(viewID), "submit")
	success := IRNavigationIntent{ID: semanticID(string(id), "success")}

	details := c.details[info.Entity.Name.Text]
	switch {
	case len(details) == 1:
		success.Kind = "page"
		success.Page = details[0].Page.Name.Text
	case len(details) > 1:
		name := info.View.Subject
		c.destinationCountError(name, info.Mode+" success detail", len(details), report)
	case len(c.lists[info.Entity.Name.Text]) > 0:
		success.Kind = "caller-list"
		success.FallbackPage = info.Page.Name.Text
	default:
		success.Kind = "same-context"
		success.Page = info.Page.Name.Text
	}

	pageNames := []string{info.Page.Name.Text}
	if success.Kind == "page" && success.Page != "" {
		pageNames = append(pageNames, success.Page)
	}

	return IRSubmitIntent{
		ID:      id,
		Action:  info.Mode,
		Success: success,
		Access:  c.composeAccess(semanticID(string(id), "access"), pageNames, nil),
	}
}

func (c *checker) actionRefAccess(id SemanticID, info *viewInfo, ref IRActionRef, action *ActionDecl) IRAccess {
	pageNames := []string{info.Page.Name.Text}
	if ref.TargetPage != "" {
		pageNames = append(pageNames, ref.TargetPage)
	} else if ref.SuccessPage != "" {
		pageNames = append(pageNames, ref.SuccessPage)
	}
	return c.composeAccess(semanticID(string(id), "access"), pageNames, action)
}

func (c *checker) composeAccess(id SemanticID, pageNames []string, action *ActionDecl) IRAccess {
	access := IRAccess{ID: id, AllOf: []IRAccessRequirement{}}
	seen := map[SemanticID]bool{}
	for _, pageName := range pageNames {
		page := c.pages[pageName]
		if page == nil || len(page.Allows) == 0 {
			continue
		}
		source := pageID(page.Name.Text)
		if seen[source] {
			continue
		}
		seen[source] = true
		access.AllOf = append(access.AllOf, IRAccessRequirement{Source: source, AnyOf: namesToStrings(page.Allows)})
	}
	if action != nil {
		var allows []string
		for _, mod := range action.Mods {
			if mod.Kind == "allow" {
				allows = namesToStrings(mod.Names)
				break
			}
		}
		if len(allows) > 0 {
			source := actionID(action.Entity.Text, action.Name.Text)
			if !seen[source] {
				access.AllOf = append(access.AllOf, IRAccessRequirement{Source: source, AnyOf: allows})
			}
		}
	}
	sort.Slice(access.AllOf, func(i, j int) bool { return access.AllOf[i].Source < access.AllOf[j].Source })
	return access
}

func (c *checker) uniqueDestination(name Name, kind string, candidates []*viewInfo, report bool) string {
	if len(candidates) != 1 {
		c.destinationCountError(name, kind, len(candidates), report)
		return ""
	}
	return candidates[0].Page.Name.Text
}

func (c *checker) destinationCountError(name Name, kind string, count int, report bool) {
	if !report {
		return
	}
	c.error(name.Span, "F2501", fmt.Sprintf("action `%s` resolves to %d %s destinations", name.Text, count, kind), "declare exactly one matching destination")
}

func hasFieldModifier(field *FieldDecl, kind string) bool {
	for _, mod := range field.Mods {
		if mod.Kind == kind {
			return true
		}
	}
	return false
}
