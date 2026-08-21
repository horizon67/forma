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
		c.checkIdentityPage(page)
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
		for index, name := range mod.Names {
			c.resolveActionRef(info, name, modDestination(mod, index), true)
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
		for index, name := range mod.Names {
			c.resolveActionRef(info, name, modDestination(mod, index), true)
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

func (c *checker) resolveActionRef(info *viewInfo, name Name, destination Name, report bool) (ref IRActionRef) {
	id := semanticID(string(viewSemanticID(info)), "action", name.Text)
	ref = IRActionRef{ID: id, Name: name.Text}
	var domainAction *ActionDecl
	defer func() {
		ref.Access = c.actionRefAccess(id, info, ref, domainAction)
	}()
	if _, standard := standardActions[name.Text]; standard {
		ref.Kind = "standard"
		if name.Text == "delete" {
			ref.InteractionStates = []string{"failure"}
		}
		switch name.Text {
		case "create":
			if info.View.Kind != ViewList {
				if report {
					c.error(name.Span, "F2501", "`create` is a collection action and can only appear on a list", "move `create` to the entity list")
				}
				return ref
			}
			// The chosen create form owns the post-write navigation through its
			// own submit intent, so the reference does not carry a success page.
			ref.TargetPage = c.resolveDestination(name, destination, "create form", name.Text, info, c.createForm[info.Entity.Name.Text], report)
		case "view":
			ref.TargetPage = c.resolveDestination(name, destination, "detail", name.Text, info, c.details[info.Entity.Name.Text], report)
		case "edit":
			ref.TargetPage = c.resolveDestination(name, destination, "edit form", name.Text, info, c.editForm[info.Entity.Name.Text], report)
		case "delete":
			if info.View.Kind == ViewList {
				if destination.Text != "" && report {
					c.namedDestinationNotAllowed(destination, "`delete` on a list returns to that list")
				}
				ref.SuccessPage = info.Page.Name.Text
			} else {
				ref.SuccessPage = c.resolveDestination(name, destination, "list", name.Text, info, c.lists[info.Entity.Name.Text], report)
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
	ref.Action = actionID(action.Entity.Text, action.Name.Text)
	ref.InteractionStates = []string{"invalid", "failure"}
	if destination.Text != "" && report {
		// Domain action navigation is declared once at the top level so the
		// reference cannot introduce a second, possibly disagreeing, record.
		c.error(destination.Span, "F2503",
			fmt.Sprintf("domain action `%s` cannot name a destination at the reference", name.Text),
			fmt.Sprintf("declare `goto` on `action %s.%s` instead", info.Entity.Name.Text, name.Text))
	}
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
	destination := submitDestination(info.View)
	switch {
	case destination.Text != "":
		if page := c.resolveDestination(info.View.Subject, destination, "detail", "submit "+info.Mode, info, details, report); page != "" {
			success.Kind = "page"
			success.Page = page
		}
	case len(details) == 1:
		success.Kind = "page"
		success.Page = details[0].Page.Name.Text
	case len(details) > 1:
		c.destinationCountError(info.View.Subject.Span, fmt.Sprintf("submit `%s`", info.Mode),
			"success detail", len(details), destinationHint("submit "+info.Mode, "detail", len(details)), report)
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
	seenPages := map[SemanticID]bool{}
	seenRequirements := map[string]bool{}
	for pageIndex, pageName := range pageNames {
		page := c.pages[pageName]
		if page == nil {
			continue
		}
		source := pageID(page.Name.Text)
		if seenPages[source] {
			continue
		}
		seenPages[source] = true
		if len(page.Allows) > 0 {
			requirement := IRAccessRequirement{Source: source, Kind: "roles", AnyOf: namesToStrings(page.Allows)}
			access.AllOf = append(access.AllOf, requirement)
			seenRequirements[accessRequirementKey(requirement)] = true
		}
		// Identity requirements protect the operation at its source page. The
		// destination page enforces its own access when it is queried; copying
		// destination ownership would also refer to a different page binding.
		if pageIndex == 0 {
			for _, requirement := range c.pageSemanticRequirements(page) {
				key := accessRequirementKey(requirement)
				if !seenRequirements[key] {
					access.AllOf = append(access.AllOf, requirement)
					seenRequirements[key] = true
				}
			}
		}
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
			requirement := IRAccessRequirement{Source: source, Kind: "roles", AnyOf: allows}
			key := accessRequirementKey(requirement)
			if !seenRequirements[key] {
				access.AllOf = append(access.AllOf, requirement)
			}
		}
	}
	sort.Slice(access.AllOf, func(i, j int) bool { return access.AllOf[i].Source < access.AllOf[j].Source })
	return access
}

func accessRequirementKey(requirement IRAccessRequirement) string {
	return fmt.Sprintf("%s|%s|%v|%s|%s|%s", requirement.Source, requirement.Kind, requirement.AnyOf, requirement.Identity, requirement.Ownership, requirement.ResourceBinding)
}

// resolveDestination applies the navigation destination rules. A named `goto`
// must be one of the candidates; an unnamed reference resolves only when the
// candidate is unique. Access is never used to choose between candidates.
func (c *checker) resolveDestination(name Name, destination Name, kind string, reference string, info *viewInfo, candidates []*viewInfo, report bool) string {
	if destination.Text != "" {
		for _, candidate := range candidates {
			if candidate.Page.Name.Text == destination.Text {
				return destination.Text
			}
		}
		if report {
			c.error(destination.Span, "F2502",
				fmt.Sprintf("`goto %s` is not a %s for `%s`", destination.Text, kind, info.Entity.Name.Text),
				fmt.Sprintf("name a page that declares this %s", kind))
		}
		return ""
	}
	if len(candidates) != 1 {
		c.destinationCountError(name.Span, fmt.Sprintf("action `%s`", name.Text), kind, len(candidates),
			destinationHint(reference, kind, len(candidates)), report)
		return ""
	}
	return candidates[0].Page.Name.Text
}

func (c *checker) namedDestinationNotAllowed(destination Name, because string) {
	c.error(destination.Span, "F2503",
		fmt.Sprintf("`goto %s` cannot be named here", destination.Text), because)
}

func modDestination(mod ViewModifier, index int) Name {
	if index < len(mod.Destinations) {
		return mod.Destinations[index]
	}
	return Name{}
}

func submitDestination(view *ViewDecl) Name {
	for _, mod := range view.Mods {
		if mod.Kind == "submit" && len(mod.Destinations) > 0 {
			return mod.Destinations[0]
		}
	}
	return Name{}
}

// destinationCountError names the construct that failed to resolve. A form
// submit is identified by its action, not by the view binding, so the subject
// label is supplied by the caller.
func (c *checker) destinationCountError(at Span, subject string, kind string, count int, hint string, report bool) {
	if !report {
		return
	}
	c.error(at, "F2501",
		fmt.Sprintf("%s resolves to %d %s destinations", subject, count, kind), hint)
}

// destinationHint separates the three remedies. A missing candidate cannot be
// fixed by naming one, and a form submit is named by its action, not by the
// view binding used in the diagnostic.
func destinationHint(reference string, kind string, count int) string {
	if count == 0 {
		switch kind {
		case "detail":
			return "declare a page with `detail <binding>` for this entity"
		case "create form":
			return "declare a page with `form Entity` for this entity"
		case "edit form":
			return "declare a page with `form <binding>` for this entity"
		case "list":
			return "declare a page with `list Entity` for this entity"
		default:
			return "declare the matching view for this entity"
		}
	}
	return fmt.Sprintf("name the destination with `%s goto <Page>`", reference)
}

func hasFieldModifier(field *FieldDecl, kind string) bool {
	for _, mod := range field.Mods {
		if mod.Kind == kind {
			return true
		}
	}
	return false
}
