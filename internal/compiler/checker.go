package compiler

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
)

var builtinTypes = map[string]struct{}{
	"String": {}, "Int": {}, "Decimal": {}, "Bool": {}, "Date": {}, "DateTime": {},
}

var standardActions = map[string]struct{}{
	"create": {}, "view": {}, "edit": {}, "delete": {},
}

type resolvedType struct {
	Name     string
	Kind     string // scalar, union, entity, invalid
	Base     string
	Variants []string
}

type resolvedExpressionType struct {
	Name  string
	Kind  string
	Base  string
	Valid bool
}

type viewInfo struct {
	Page   *PageDecl
	View   *ViewDecl
	Entity *EntityDecl
	Mode   string
}

type checker struct {
	program *Program

	types    map[string]*TypeDecl
	entities map[string]*EntityDecl
	pages    map[string]*PageDecl
	roles    map[string]*RoleDecl
	actions  map[string]*ActionDecl

	resolvedTypes    map[string]resolvedType
	resolving        map[string]bool
	cycleReported    map[string]bool
	entityLabels     map[string]string
	expressionFields map[*Expression]*FieldDecl
	expressionTypes  map[*Expression]string

	viewInfo   map[*ViewDecl]*viewInfo
	createForm map[string][]*viewInfo
	editForm   map[string][]*viewInfo
	details    map[string][]*viewInfo
	lists      map[string][]*viewInfo

	diagnostics []Diagnostic
}

func check(program *Program) (*ResolvedIntent, *SourceMap, []Diagnostic) {
	c := &checker{
		program: program, types: map[string]*TypeDecl{}, entities: map[string]*EntityDecl{},
		pages: map[string]*PageDecl{}, roles: map[string]*RoleDecl{}, actions: map[string]*ActionDecl{},
		resolvedTypes: map[string]resolvedType{}, resolving: map[string]bool{}, cycleReported: map[string]bool{},
		entityLabels: map[string]string{}, expressionFields: map[*Expression]*FieldDecl{},
		expressionTypes: map[*Expression]string{}, viewInfo: map[*ViewDecl]*viewInfo{},
		createForm: map[string][]*viewInfo{}, editForm: map[string][]*viewInfo{},
		details: map[string][]*viewInfo{}, lists: map[string][]*viewInfo{},
	}
	c.collectDeclarations()
	c.checkTypes()
	c.checkEntities()
	c.indexViews()
	c.checkActions()
	c.checkPages()
	SortDiagnostics(c.diagnostics)
	if len(c.diagnostics) > 0 {
		return nil, nil, c.diagnostics
	}
	ir, sourceMap := c.buildIntent()
	return ir, sourceMap, nil
}

func (c *checker) collectDeclarations() {
	for _, decl := range c.program.Types {
		if _, builtin := builtinTypes[decl.Name.Text]; builtin {
			c.error(decl.Name.Span, "F2001", fmt.Sprintf("type `%s` redeclares a built-in type", decl.Name.Text), "choose a different domain type name")
			continue
		}
		if previous, exists := c.types[decl.Name.Text]; exists {
			c.duplicate("type", decl.Name, previous.Name.Span)
			continue
		}
		c.types[decl.Name.Text] = decl
	}
	for _, decl := range c.program.Entities {
		if _, builtin := builtinTypes[decl.Name.Text]; builtin {
			c.error(decl.Name.Span, "F2001", fmt.Sprintf("entity `%s` conflicts with a built-in type", decl.Name.Text), "choose a different entity name")
			continue
		}
		if previous, exists := c.types[decl.Name.Text]; exists {
			c.duplicate("entity", decl.Name, previous.Name.Span)
			continue
		}
		if previous, exists := c.entities[decl.Name.Text]; exists {
			c.duplicate("entity", decl.Name, previous.Name.Span)
			continue
		}
		c.entities[decl.Name.Text] = decl
	}
	for _, decl := range c.program.Pages {
		if previous, exists := c.pages[decl.Name.Text]; exists {
			c.duplicate("page", decl.Name, previous.Name.Span)
			continue
		}
		c.pages[decl.Name.Text] = decl
	}
	for _, decl := range c.program.Roles {
		if previous, exists := c.roles[decl.Name.Text]; exists {
			c.duplicate("role", decl.Name, previous.Name.Span)
			continue
		}
		c.roles[decl.Name.Text] = decl
	}
	for _, decl := range c.program.Actions {
		key := actionKey(decl.Entity.Text, decl.Name.Text)
		if previous, exists := c.actions[key]; exists {
			c.error(decl.Name.Span, "F2001", fmt.Sprintf("duplicate action `%s.%s`", decl.Entity.Text, decl.Name.Text), fmt.Sprintf("the first declaration is at %s:%d", previous.Name.Span.File, previous.Name.Span.Start.Line))
			continue
		}
		c.actions[key] = decl
	}
}

func (c *checker) checkTypes() {
	for _, decl := range c.program.Types {
		if c.types[decl.Name.Text] != decl {
			continue
		}
		resolved := c.resolveType(decl.Name.Text, decl.Name.Span)
		seenMods := map[string]Span{}
		var minValue, maxValue *big.Rat
		for _, mod := range decl.Mods {
			if previous, exists := seenMods[mod.Kind]; exists {
				c.duplicateModifier(mod.Kind, mod.Span, previous)
				continue
			}
			seenMods[mod.Kind] = mod.Span
			if len(decl.Variants) > 0 {
				c.error(mod.Span, "F2101", fmt.Sprintf("union type `%s` cannot use `%s`", decl.Name.Text, mod.Kind), "remove scalar constraints from the union")
				continue
			}
			switch mod.Kind {
			case "matches":
				if resolved.Base != "String" {
					c.error(mod.Span, "F2101", "`matches` requires a String-based type", "use `matches` only with a type derived from `String`")
				}
				if _, err := regexp.Compile(mod.Value); err != nil {
					c.error(mod.Span, "F2101", fmt.Sprintf("invalid regular expression: %v", err), "fix the expression between `/` delimiters")
				}
			case "min", "max":
				if resolved.Base != "Int" && resolved.Base != "Decimal" {
					c.error(mod.Span, "F2101", "numeric bounds require an Int- or Decimal-based type", "remove the bound or change the base type")
				}
				value, ok := new(big.Rat).SetString(mod.Value)
				if !ok {
					c.error(mod.Span, "F2101", fmt.Sprintf("invalid numeric bound `%s`", mod.Value), "use an integer or decimal number")
					continue
				}
				if mod.Kind == "min" {
					minValue = value
				} else {
					maxValue = value
				}
			}
		}
		if minValue != nil && maxValue != nil && minValue.Cmp(maxValue) > 0 {
			c.error(decl.Name.Span, "F2101", fmt.Sprintf("type `%s` has min greater than max", decl.Name.Text), "make `min` less than or equal to `max`")
		}
		if len(decl.Variants) > 0 {
			seen := map[string]Span{}
			for _, variant := range decl.Variants {
				if previous, exists := seen[variant.Text]; exists {
					c.error(variant.Span, "F2101", fmt.Sprintf("duplicate union value `%s`", variant.Text), fmt.Sprintf("the first value is at line %d", previous.Start.Line))
				} else {
					seen[variant.Text] = variant.Span
				}
			}
		}
	}
}

func (c *checker) resolveType(name string, at Span) resolvedType {
	if _, ok := builtinTypes[name]; ok {
		return resolvedType{Name: name, Kind: "scalar", Base: name}
	}
	if _, ok := c.entities[name]; ok {
		return resolvedType{Name: name, Kind: "entity"}
	}
	if resolved, ok := c.resolvedTypes[name]; ok {
		return resolved
	}
	decl, ok := c.types[name]
	if !ok {
		c.error(at, "F2002", fmt.Sprintf("unknown type `%s`", name), "declare the type or use a v0 built-in type")
		return resolvedType{Name: name, Kind: "invalid"}
	}
	if c.resolving[name] {
		if !c.cycleReported[name] {
			c.error(decl.Name.Span, "F2102", fmt.Sprintf("cyclic type definition involving `%s`", name), "make named scalar types ultimately derive from a built-in type")
			c.cycleReported[name] = true
		}
		return resolvedType{Name: name, Kind: "invalid"}
	}
	c.resolving[name] = true
	var result resolvedType
	if len(decl.Variants) > 0 {
		result = resolvedType{Name: name, Kind: "union"}
		for _, variant := range decl.Variants {
			result.Variants = append(result.Variants, variant.Text)
		}
	} else if decl.Base != nil {
		base := c.resolveType(decl.Base.Text, decl.Base.Span)
		if base.Kind == "entity" {
			c.error(decl.Base.Span, "F2101", fmt.Sprintf("type `%s` cannot derive from entity `%s`", name, decl.Base.Text), "use the entity directly as a field type")
			result = resolvedType{Name: name, Kind: "invalid"}
		} else {
			result = resolvedType{Name: name, Kind: base.Kind, Base: base.Base, Variants: append([]string(nil), base.Variants...)}
		}
	} else {
		result = resolvedType{Name: name, Kind: "invalid"}
	}
	delete(c.resolving, name)
	c.resolvedTypes[name] = result
	return result
}

func (c *checker) checkEntities() {
	for _, entity := range c.program.Entities {
		if c.entities[entity.Name.Text] != entity {
			continue
		}
		fields := map[string]*FieldDecl{}
		labelCount := 0
		for _, field := range entity.Fields {
			if previous, exists := fields[field.Name.Text]; exists {
				c.error(field.Name.Span, "F2201", fmt.Sprintf("duplicate field `%s.%s`", entity.Name.Text, field.Name.Text), fmt.Sprintf("the first field is at line %d", previous.Name.Span.Start.Line))
				continue
			}
			fields[field.Name.Text] = field
			resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
			if field.Type.Collection && resolved.Kind != "entity" && resolved.Kind != "invalid" {
				c.error(field.Type.Span, "F2204", "collection fields must contain an entity type", "use `[Entity]` for a to-many relationship")
			}
			mods := fieldModifierMap(c, field)
			if field.Type.Collection {
				for _, forbidden := range []string{"required", "unique", "default"} {
					if mod, ok := mods[forbidden]; ok {
						c.error(mod.Span, "F2204", fmt.Sprintf("collection field `%s` cannot use `%s`", field.Name.Text, forbidden), "collections are present and empty by default in v0")
					}
				}
			}
			if label, ok := mods["label"]; ok {
				labelCount++
				if field.Type.Collection || resolved.Kind != "scalar" || resolved.Base != "String" {
					c.error(label.Span, "F2202", "`label` requires a String-based scalar field", "mark a required String-based field as the entity label")
				}
				if _, required := mods["required"]; !required {
					c.error(label.Span, "F2202", "`label` field must also be `required`", "add `required` to the label field")
				}
				c.entityLabels[entity.Name.Text] = field.Name.Text
			}
			if defaultMod, ok := mods["default"]; ok && defaultMod.Value != nil {
				c.checkDefault(field, resolved, *defaultMod.Value)
			}
		}
		if labelCount > 1 {
			c.error(entity.Name.Span, "F2202", fmt.Sprintf("entity `%s` has more than one label field", entity.Name.Text), "keep exactly one human-readable label field")
		}
		if entity.State != nil {
			if previous, exists := fields[entity.State.Name.Text]; exists {
				c.error(entity.State.Name.Span, "F2201", fmt.Sprintf("state `%s` conflicts with field `%s`", entity.State.Name.Text, previous.Name.Text), "rename the state or field")
			}
			seenValues := map[string]Span{}
			for _, value := range entity.State.Values {
				if previous, exists := seenValues[value.Text]; exists {
					c.error(value.Span, "F2201", fmt.Sprintf("duplicate state value `%s`", value.Text), fmt.Sprintf("the first value is at line %d", previous.Start.Line))
				} else {
					seenValues[value.Text] = value.Span
				}
			}
			if _, exists := seenValues[entity.State.Initial.Text]; !exists && entity.State.Initial.Text != "" {
				c.error(entity.State.Initial.Span, "F2201", fmt.Sprintf("initial state `%s` is not declared by `%s`", entity.State.Initial.Text, entity.State.Name.Text), "use one of the values declared before `initial`")
			}
		}
		seenInvariants := map[string]*InvariantDecl{}
		for _, invariant := range entity.Invariants {
			if previous, exists := seenInvariants[invariant.Name.Text]; exists {
				c.error(invariant.Name.Span, "F2601", fmt.Sprintf("duplicate invariant `%s.%s`", entity.Name.Text, invariant.Name.Text), fmt.Sprintf("the first invariant is at line %d", previous.Name.Span.Start.Line))
				continue
			}
			seenInvariants[invariant.Name.Text] = invariant
			c.checkInvariant(entity, invariant, fields)
		}
	}
}

func (c *checker) checkInvariant(entity *EntityDecl, invariant *InvariantDecl, fields map[string]*FieldDecl) {
	if invariant.Predicate == nil || invariant.Predicate.Binary == nil {
		return
	}
	binary := invariant.Predicate.Binary
	if binary.Operator == "invalid" {
		return
	}
	left := c.checkInvariantOperand(entity, binary.Left, fields)
	right := c.checkInvariantOperand(entity, binary.Right, fields)
	if !left.Valid || !right.Valid {
		return
	}
	if left.Name != right.Name {
		c.error(invariant.Predicate.Span, "F2605", fmt.Sprintf("operator `<=` cannot compare `%s` and `%s`", left.Name, right.Name), "compare fields with the same nominal ordered scalar type")
		return
	}
	if !isOrderedExpressionType(left) || !isOrderedExpressionType(right) {
		c.error(invariant.Predicate.Span, "F2605", fmt.Sprintf("operator `<=` requires ordered scalar fields, not `%s`", left.Name), "use required fields with the same Int-, Decimal-, Date-, or DateTime-based type")
		return
	}
	c.expressionTypes[invariant.Predicate] = "Bool"
}

func (c *checker) checkInvariantOperand(entity *EntityDecl, expression *Expression, fields map[string]*FieldDecl) resolvedExpressionType {
	if expression == nil || expression.Field == nil || len(expression.Field.Path) == 0 {
		return resolvedExpressionType{}
	}
	if len(expression.Field.Path) != 1 {
		c.error(expression.Span, "F2603", "relation traversal is not allowed in an invariant", "the first invariant slice may reference only fields declared directly by the same entity")
		return resolvedExpressionType{}
	}
	name := expression.Field.Path[0]
	field, exists := fields[name.Text]
	if !exists {
		if entity.State != nil && entity.State.Name.Text == name.Text {
			return resolvedExpressionType{Name: entity.State.Name.Text, Kind: "state", Valid: true}
		}
		c.error(name.Span, "F2602", fmt.Sprintf("unknown field or state `%s.%s`", entity.Name.Text, name.Text), "use a field or named state declared directly by the entity")
		return resolvedExpressionType{}
	}
	resolved := c.resolveType(field.Type.Name.Text, field.Type.Name.Span)
	if resolved.Kind == "invalid" {
		return resolvedExpressionType{}
	}
	if field.Type.Collection {
		c.error(name.Span, "F2605", fmt.Sprintf("field `%s.%s` is a collection and cannot be used with `<=`", entity.Name.Text, name.Text), "compare fields with the same ordered scalar type")
		return resolvedExpressionType{}
	}
	if resolved.Kind != "scalar" {
		c.error(name.Span, "F2605", fmt.Sprintf("field `%s.%s` has non-scalar type `%s` and cannot be used with `<=`", entity.Name.Text, name.Text, field.Type.Name.Text), "compare fields with the same ordered scalar type")
		return resolvedExpressionType{}
	}
	if !hasFieldModifier(field, "required") {
		c.error(name.Span, "F2604", fmt.Sprintf("optional field `%s.%s` cannot be used in an invariant expression", entity.Name.Text, name.Text), "mark the field `required` or wait for explicit absence handling")
		return resolvedExpressionType{}
	}
	c.expressionFields[expression] = field
	c.expressionTypes[expression] = field.Type.Name.Text
	return resolvedExpressionType{Name: field.Type.Name.Text, Kind: resolved.Kind, Base: resolved.Base, Valid: true}
}

func isOrderedExpressionType(value resolvedExpressionType) bool {
	if !value.Valid || value.Kind != "scalar" {
		return false
	}
	switch value.Base {
	case "Int", "Decimal", "Date", "DateTime":
		return true
	default:
		return false
	}
}

func fieldModifierMap(c *checker, field *FieldDecl) map[string]FieldModifier {
	mods := map[string]FieldModifier{}
	for _, mod := range field.Mods {
		if previous, exists := mods[mod.Kind]; exists {
			c.duplicateModifier(mod.Kind, mod.Span, previous.Span)
			continue
		}
		mods[mod.Kind] = mod
	}
	return mods
}

func (c *checker) checkDefault(field *FieldDecl, resolved resolvedType, literal Literal) {
	if field.Type.Collection || resolved.Kind == "entity" {
		c.error(literal.Span, "F2205", "relationship fields cannot have a literal default", "remove the default from the relationship")
		return
	}
	if resolved.Base == "Date" || resolved.Base == "DateTime" {
		c.error(literal.Span, "F2205", fmt.Sprintf("%s fields do not have a v0 default literal", resolved.Base), "remove the default; Date and DateTime literals are deferred beyond v0")
		return
	}
	valid := false
	switch resolved.Kind {
	case "union":
		if literal.Kind == "enum" {
			for _, variant := range resolved.Variants {
				if variant == literal.Value {
					valid = true
					break
				}
			}
		}
	case "scalar":
		switch resolved.Base {
		case "String":
			valid = literal.Kind == "string"
		case "Int":
			valid = literal.Kind == "int"
		case "Decimal":
			valid = literal.Kind == "int" || literal.Kind == "decimal"
		case "Bool":
			valid = literal.Kind == "bool"
		}
	}
	if !valid {
		c.error(literal.Span, "F2205", fmt.Sprintf("default `%s` does not match field type `%s`", literal.Value, field.Type.Name.Text), "use a literal compatible with the field type")
	}
}

func (c *checker) checkActions() {
	for _, action := range c.program.Actions {
		if c.actions[actionKey(action.Entity.Text, action.Name.Text)] != action {
			continue
		}
		entity, ok := c.entities[action.Entity.Text]
		if !ok {
			c.error(action.Entity.Span, "F2003", fmt.Sprintf("unknown action entity `%s`", action.Entity.Text), "declare the entity before defining its actions")
			continue
		}
		if _, collision := standardActions[action.Name.Text]; collision {
			c.error(action.Name.Span, "F2302", fmt.Sprintf("domain action `%s` conflicts with a standard action", action.Name.Text), "choose a name other than create, view, edit, or delete")
		}
		if entity.State == nil {
			c.error(action.Span, "F2301", fmt.Sprintf("action `%s.%s` requires an entity state", entity.Name.Text, action.Name.Text), "add one named state to the entity")
			continue
		}
		values := map[string]struct{}{}
		for _, value := range entity.State.Values {
			values[value.Text] = struct{}{}
		}
		seenSources := map[string]Span{}
		for _, source := range action.Sources {
			if _, exists := values[source.Text]; !exists {
				c.error(source.Span, "F2301", fmt.Sprintf("unknown source state `%s` for `%s`", source.Text, entity.Name.Text), "use a value declared by the entity state")
			}
			if previous, exists := seenSources[source.Text]; exists {
				c.error(source.Span, "F2301", fmt.Sprintf("duplicate source state `%s`", source.Text), fmt.Sprintf("the first source is at line %d", previous.Start.Line))
			}
			seenSources[source.Text] = source.Span
			if source.Text == action.Destination.Text {
				c.error(action.Destination.Span, "F2301", "transition source and destination must differ", "choose a different destination state")
			}
		}
		if _, exists := values[action.Destination.Text]; !exists {
			c.error(action.Destination.Span, "F2301", fmt.Sprintf("unknown destination state `%s` for `%s`", action.Destination.Text, entity.Name.Text), "use a value declared by the entity state")
		}
		seenMods := map[string]Span{}
		for _, mod := range action.Mods {
			if previous, exists := seenMods[mod.Kind]; exists {
				c.duplicateModifier(mod.Kind, mod.Span, previous)
				continue
			}
			seenMods[mod.Kind] = mod.Span
			switch mod.Kind {
			case "allow":
				c.checkRoles(mod.Names)
			case "goto":
				if len(mod.Names) == 0 {
					continue
				}
				page, exists := c.pages[mod.Names[0].Text]
				if !exists {
					c.error(mod.Names[0].Span, "F2003", fmt.Sprintf("unknown page `%s`", mod.Names[0].Text), "declare the page or remove `goto`")
				} else if page.Param != nil && page.Param.Type.Text != entity.Name.Text {
					c.error(mod.Names[0].Span, "F2503", fmt.Sprintf("page `%s` requires `%s`, not `%s`", page.Name.Text, page.Param.Type.Text, entity.Name.Text), "choose a page that accepts the action entity")
				}
			}
		}
	}
}

func (c *checker) checkRoles(names []Name) {
	seen := map[string]Span{}
	for _, name := range names {
		if previous, exists := seen[name.Text]; exists {
			c.error(name.Span, "F2005", fmt.Sprintf("duplicate role `%s`", name.Text), fmt.Sprintf("the first role is at line %d", previous.Start.Line))
		}
		seen[name.Text] = name.Span
		if _, exists := c.roles[name.Text]; !exists {
			c.error(name.Span, "F2003", fmt.Sprintf("unknown role `%s`", name.Text), "declare the role before using it in `allow`")
		}
	}
}

func (c *checker) duplicate(kind string, name Name, previous Span) {
	c.error(name.Span, "F2001", fmt.Sprintf("duplicate %s `%s`", kind, name.Text), fmt.Sprintf("the first declaration is at %s:%d", previous.File, previous.Start.Line))
}

func (c *checker) duplicateModifier(kind string, span, previous Span) {
	c.error(span, "F2005", fmt.Sprintf("duplicate `%s` modifier", kind), fmt.Sprintf("the first modifier is at line %d", previous.Start.Line))
}

func (c *checker) error(span Span, code, message, hint string) {
	c.diagnostics = append(c.diagnostics, Diagnostic{Code: code, Message: message, Hint: hint, Span: span})
}

func actionKey(entity, action string) string { return entity + "." + action }

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
