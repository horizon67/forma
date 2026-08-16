package compiler

import (
	"fmt"
	"strconv"
	"strings"
)

type parser struct {
	tokens      []token
	current     int
	diagnostics []Diagnostic
}

func parse(source SourceFile) (*Program, []Diagnostic) {
	tokens, diagnostics := lex(source)
	p := &parser{tokens: tokens, diagnostics: diagnostics}
	program := p.parseProgram()
	return program, p.diagnostics
}

func (p *parser) parseProgram() *Program {
	program := &Program{}
	for !p.atEnd() {
		p.skipNewlines()
		if p.atEnd() {
			break
		}
		if !p.check(tokenIdent) {
			p.report(p.peek().Span, "F1001", "expected a top-level declaration", "start a declaration with `type`, `entity`, `action`, `page`, or `role`")
			before := p.current
			p.synchronizeLine()
			if p.current == before && !p.atEnd() {
				p.advance()
			}
			continue
		}
		switch p.peek().Value {
		case "type":
			if decl := p.parseTypeDecl(); decl != nil {
				program.Types = append(program.Types, decl)
			}
		case "entity":
			if decl := p.parseEntityDecl(); decl != nil {
				program.Entities = append(program.Entities, decl)
			}
		case "action":
			if decl := p.parseActionDecl(); decl != nil {
				program.Actions = append(program.Actions, decl)
			}
		case "page":
			if decl := p.parsePageDecl(); decl != nil {
				program.Pages = append(program.Pages, decl)
			}
		case "role":
			if decl := p.parseRoleDecl(); decl != nil {
				program.Roles = append(program.Roles, decl)
			}
		default:
			p.report(p.peek().Span, "F1001", fmt.Sprintf("unknown declaration `%s`", p.peek().Value), "expected `type`, `entity`, `action`, `page`, or `role`")
			p.synchronizeLine()
		}
	}
	return program
}

func (p *parser) parseTypeDecl() *TypeDecl {
	start := p.advance().Span
	name := p.consumeTypeName("after `type`")
	p.consume(tokenEqual, "`=` after the type name")
	first := p.consumeTypeName("as the type expression")
	decl := &TypeDecl{Name: name}
	if p.match(tokenPipe) {
		decl.Variants = append(decl.Variants, first)
		decl.Variants = append(decl.Variants, p.consumeTypeName("after `|`"))
		for p.match(tokenPipe) {
			decl.Variants = append(decl.Variants, p.consumeTypeName("after `|`"))
		}
	} else {
		decl.Base = &first
	}
	for !p.atLineEnd() {
		if !p.check(tokenIdent) {
			p.unexpected("a type modifier")
			p.synchronizeLine()
			break
		}
		modStart := p.advance()
		mod := TypeModifier{Kind: modStart.Value, Span: modStart.Span}
		switch mod.Kind {
		case "matches":
			value := p.consume(tokenRegex, "a regular expression after `matches`")
			mod.Value = value.Value
			mod.Span = mergeSpan(mod.Span, value.Span)
		case "min", "max":
			value := p.consume(tokenNumber, "a number after `"+mod.Kind+"`")
			mod.Value = value.Value
			mod.Span = mergeSpan(mod.Span, value.Span)
		default:
			p.report(mod.Span, "F1004", fmt.Sprintf("unknown type modifier `%s`", mod.Kind), "the v0 modifiers are `matches`, `min`, and `max`")
		}
		decl.Mods = append(decl.Mods, mod)
	}
	p.finishLine()
	decl.Span = mergeSpan(start, p.previous().Span)
	return decl
}

func (p *parser) parseEntityDecl() *EntityDecl {
	start := p.advance().Span
	name := p.consumeTypeName("after `entity`")
	decl := &EntityDecl{Name: name}
	if !p.beginBlock("an entity body") {
		p.synchronizeLine()
		return decl
	}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		if !p.check(tokenIdent) {
			p.unexpected("a field, state, or invariant declaration")
			p.synchronizeLine()
			continue
		}
		switch p.peek().Value {
		case "state":
			state := p.parseStateDecl()
			if decl.State != nil {
				p.report(state.Span, "F2201", fmt.Sprintf("entity `%s` declares more than one state", decl.Name.Text), "keep a single named state declaration")
			} else {
				decl.State = state
			}
		case "invariant":
			if next := p.peekAhead(1); next.Kind == tokenIdent && (startsLower(next.Value) || p.peekAhead(2).Kind == tokenColon) {
				decl.Invariants = append(decl.Invariants, p.parseInvariantDecl())
			} else {
				decl.Fields = append(decl.Fields, p.parseFieldDecl())
			}
		default:
			decl.Fields = append(decl.Fields, p.parseFieldDecl())
		}
	}
	end := p.consume(tokenRBrace, "`}` to close the entity")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseInvariantDecl() *InvariantDecl {
	start := p.advance().Span
	name := p.consumeName("after `invariant`")
	p.consume(tokenColon, "`:` after the invariant name")
	left := p.parseFieldExpression("as the left invariant operand")
	operator := p.consume(tokenLessEqual, "`<=` between invariant operands")
	right := p.parseFieldExpression("as the right invariant operand")
	if p.check(tokenLessEqual) {
		p.report(p.peek().Span, "F1003", "comparison operators cannot be chained", "declare two named invariants, one for each comparison; boolean `and` is not implemented yet")
		p.synchronizeLine()
	} else {
		p.finishLine()
	}
	expressionSpan := mergeSpan(left.Span, right.Span)
	predicate := &Expression{
		Kind: "binary",
		Binary: &BinaryExpression{
			Operator: "less-than-or-equal",
			Left:     left,
			Right:    right,
			Span:     expressionSpan,
		},
		Span: expressionSpan,
	}
	if operator.Kind == tokenInvalid {
		predicate.Binary.Operator = "invalid"
	}
	return &InvariantDecl{Name: name, Predicate: predicate, Span: mergeSpan(start, p.previous().Span)}
}

func (p *parser) parseFieldExpression(context string) *Expression {
	first := p.consumeName(context)
	path := []Name{first}
	for p.match(tokenDot) {
		path = append(path, p.consumeName("after `.` in the field path"))
	}
	span := first.Span
	if len(path) > 1 {
		span = mergeSpan(span, path[len(path)-1].Span)
	}
	return &Expression{Kind: "field", Field: &FieldExpression{Path: path, Span: span}, Span: span}
}

func (p *parser) parseFieldDecl() *FieldDecl {
	start := p.peek().Span
	name := p.consumeName("as the field name")
	typeRef := p.parseTypeRef()
	decl := &FieldDecl{Name: name, Type: typeRef}
	for !p.atLineEnd() {
		if !p.check(tokenIdent) {
			p.unexpected("a field modifier")
			p.synchronizeLine()
			break
		}
		modToken := p.advance()
		mod := FieldModifier{Kind: modToken.Value, Span: modToken.Span}
		switch mod.Kind {
		case "required", "unique", "readonly", "label":
		case "default":
			literal := p.parseLiteral()
			mod.Value = &literal
			mod.Span = mergeSpan(mod.Span, literal.Span)
		default:
			p.report(mod.Span, "F1004", fmt.Sprintf("unknown field modifier `%s`", mod.Kind), "the v0 modifiers are `required`, `unique`, `readonly`, `default`, and `label`")
		}
		decl.Mods = append(decl.Mods, mod)
	}
	p.finishLine()
	decl.Span = mergeSpan(start, p.previous().Span)
	return decl
}

func (p *parser) parseTypeRef() TypeRef {
	start := p.peek().Span
	if p.match(tokenLBracket) {
		name := p.consumeTypeName("inside the collection type")
		end := p.consume(tokenRBracket, "`]` after the collection type")
		return TypeRef{Name: name, Collection: true, Span: mergeSpan(start, end.Span)}
	}
	name := p.consumeTypeName("as the field type")
	return TypeRef{Name: name, Span: name.Span}
}

func (p *parser) parseStateDecl() *StateDecl {
	start := p.advance().Span
	name := p.consumeName("after `state`")
	values := []Name{p.consumeTypeName("as a state value")}
	if !p.match(tokenPipe) {
		p.report(p.peek().Span, "F1002", "a state requires at least two values", "separate state values with `|`")
	} else {
		values = append(values, p.consumeTypeName("after `|`"))
	}
	for p.match(tokenPipe) {
		values = append(values, p.consumeTypeName("after `|`"))
	}
	initialKeyword := p.consume(tokenIdent, "`initial` after the state values")
	if initialKeyword.Value != "" && initialKeyword.Value != "initial" {
		p.report(initialKeyword.Span, "F1002", fmt.Sprintf("expected `initial`, found `%s`", initialKeyword.Value), "declare the initial state explicitly, for example `initial Pending`")
	}
	initial := p.consumeTypeName("after `initial`")
	p.finishLine()
	return &StateDecl{Name: name, Values: values, Initial: initial, Span: mergeSpan(start, p.previous().Span)}
}

func (p *parser) parseActionDecl() *ActionDecl {
	start := p.advance().Span
	entity := p.consumeTypeName("after `action`")
	p.consume(tokenDot, "`.` after the entity name")
	name := p.consumeName("as the action name")
	p.consume(tokenColon, "`:` after the action name")
	sources := []Name{p.consumeTypeName("as the source state")}
	for p.match(tokenPipe) {
		sources = append(sources, p.consumeTypeName("after `|`"))
	}
	p.consume(tokenArrow, "`->` after the source state")
	destination := p.consumeTypeName("as the destination state")
	decl := &ActionDecl{Entity: entity, Name: name, Sources: sources, Destination: destination}
	for !p.atLineEnd() {
		if !p.check(tokenIdent) {
			p.unexpected("an action modifier")
			p.synchronizeLine()
			break
		}
		modToken := p.advance()
		mod := ActionModifier{Kind: modToken.Value, Span: modToken.Span}
		switch mod.Kind {
		case "confirm":
		case "allow":
			mod.Names = p.parseNameList()
			if len(mod.Names) > 0 {
				mod.Span = mergeSpan(mod.Span, mod.Names[len(mod.Names)-1].Span)
			}
		case "goto":
			page := p.consumeTypeName("after `goto`")
			mod.Names = []Name{page}
			mod.Span = mergeSpan(mod.Span, page.Span)
		default:
			p.report(mod.Span, "F1004", fmt.Sprintf("unknown action modifier `%s`", mod.Kind), "the v0 modifiers are `confirm`, `allow`, and `goto`")
		}
		decl.Mods = append(decl.Mods, mod)
	}
	p.finishLine()
	decl.Span = mergeSpan(start, p.previous().Span)
	return decl
}

func (p *parser) parsePageDecl() *PageDecl {
	start := p.advance().Span
	name := p.consumeTypeName("after `page`")
	decl := &PageDecl{Name: name}
	if p.match(tokenLParen) {
		paramName := p.consumeName("as the page parameter")
		paramType := p.consumeTypeName("as the page parameter type")
		end := p.consume(tokenRParen, "`)` after the page parameter")
		decl.Param = &Parameter{Name: paramName, Type: paramType, Span: mergeSpan(paramName.Span, end.Span)}
	}
	if !p.beginBlock("a page body") {
		p.synchronizeLine()
		return decl
	}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		if !p.check(tokenIdent) {
			p.unexpected("`allow`, `list`, `detail`, or `form`")
			p.synchronizeLine()
			continue
		}
		switch p.peek().Value {
		case "allow":
			allowToken := p.advance()
			if len(decl.Allows) != 0 {
				p.report(allowToken.Span, "F2005", "duplicate page `allow` clause", "combine the roles into one comma-separated `allow` clause")
			}
			decl.Allows = append(decl.Allows, p.parseNameList()...)
			p.finishLine()
		case "list":
			decl.Views = append(decl.Views, p.parseView(ViewList))
		case "detail":
			decl.Views = append(decl.Views, p.parseView(ViewDetail))
		case "form":
			decl.Views = append(decl.Views, p.parseView(ViewForm))
		default:
			p.report(p.peek().Span, "F1001", fmt.Sprintf("unknown page member `%s`", p.peek().Value), "expected `allow`, `list`, `detail`, or `form`")
			p.synchronizeLine()
		}
	}
	end := p.consume(tokenRBrace, "`}` to close the page")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseView(kind ViewKind) *ViewDecl {
	start := p.advance().Span
	subjectToken := p.consume(tokenIdent, "a view subject")
	subject := Name{Text: subjectToken.Value, Span: subjectToken.Span}
	decl := &ViewDecl{Kind: kind, Subject: subject}
	if p.match(tokenNewline) {
		decl.Span = mergeSpan(start, subject.Span)
		return decl
	}
	if !p.match(tokenLBrace) {
		p.report(p.peek().Span, "F1002", "expected a newline or `{` after the view subject", "start a modifier block with `{` or end the declaration")
		p.synchronizeLine()
		decl.Span = mergeSpan(start, p.previous().Span)
		return decl
	}
	p.consume(tokenNewline, "a newline after `{`")
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		decl.Mods = append(decl.Mods, p.parseViewModifier(kind))
	}
	end := p.consume(tokenRBrace, "`}` to close the view")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseViewModifier(kind ViewKind) ViewModifier {
	if !p.check(tokenIdent) {
		p.unexpected("a view modifier")
		p.synchronizeLine()
		return ViewModifier{Span: p.previous().Span}
	}
	modToken := p.advance()
	mod := ViewModifier{Kind: modToken.Value, Span: modToken.Span}
	valid := false
	switch kind {
	case ViewList:
		switch mod.Kind {
		case "columns", "search", "filter", "actions":
			valid = true
			mod.Names = p.parseNameList()
		case "sort":
			valid = true
			mod.Names = []Name{p.consumeName("after `sort`")}
			if p.check(tokenIdent) && (p.peek().Value == "asc" || p.peek().Value == "desc") {
				mod.Direction = p.advance().Value
			}
		case "paginate":
			valid = true
			number := p.consume(tokenNumber, "a positive integer after `paginate`")
			mod.PageSize, _ = strconv.Atoi(number.Value)
			mod.Span = mergeSpan(mod.Span, number.Span)
		}
	case ViewDetail:
		if mod.Kind == "fields" || mod.Kind == "actions" {
			valid = true
			mod.Names = p.parseNameList()
		}
	case ViewForm:
		switch mod.Kind {
		case "fields":
			valid = true
			mod.Names = p.parseNameList()
		case "submit":
			valid = true
			mod.Names = []Name{p.consumeName("after `submit`")}
		}
	}
	if !valid {
		p.report(mod.Span, "F1004", fmt.Sprintf("unknown %s modifier `%s`", kind, mod.Kind), "use a modifier from the closed v0 set")
	}
	if len(mod.Names) > 0 {
		mod.Span = mergeSpan(mod.Span, mod.Names[len(mod.Names)-1].Span)
	}
	p.finishLine()
	return mod
}

func (p *parser) parseRoleDecl() *RoleDecl {
	start := p.advance().Span
	name := p.consumeName("after `role`")
	p.finishLine()
	return &RoleDecl{Name: name, Span: mergeSpan(start, p.previous().Span)}
}

func (p *parser) parseNameList() []Name {
	names := []Name{p.consumeName("in the name list")}
	for p.match(tokenComma) {
		names = append(names, p.consumeName("after `,`"))
	}
	return names
}

func (p *parser) parseLiteral() Literal {
	t := p.peek()
	switch t.Kind {
	case tokenString:
		p.advance()
		return Literal{Kind: "string", Value: t.Value, Span: t.Span}
	case tokenNumber:
		p.advance()
		kind := "int"
		if strings.Contains(t.Value, ".") {
			kind = "decimal"
		}
		return Literal{Kind: kind, Value: t.Value, Span: t.Span}
	case tokenIdent:
		p.advance()
		kind := "enum"
		if t.Value == "true" || t.Value == "false" {
			kind = "bool"
		}
		return Literal{Kind: kind, Value: t.Value, Span: t.Span}
	default:
		p.report(t.Span, "F1002", "expected a literal value", "use a string, number, boolean, or union value")
		if !p.atLineEnd() {
			p.advance()
		}
		return Literal{Kind: "invalid", Span: t.Span}
	}
}

func (p *parser) beginBlock(description string) bool {
	if !p.match(tokenLBrace) {
		p.report(p.peek().Span, "F1002", "expected `{` to start "+description, "add `{` and place the body on following lines")
		return false
	}
	p.consume(tokenNewline, "a newline after `{`")
	return true
}

func (p *parser) finishLine() {
	if p.match(tokenNewline) || p.atEnd() {
		return
	}
	p.report(p.peek().Span, "F1002", fmt.Sprintf("unexpected %s at end of declaration", p.peek().display()), "end the declaration with a newline")
	p.synchronizeLine()
}

func (p *parser) synchronizeLine() {
	for !p.atEnd() && !p.check(tokenNewline) && !p.check(tokenRBrace) {
		p.advance()
	}
	p.match(tokenNewline)
}

func (p *parser) skipNewlines() {
	for p.match(tokenNewline) {
	}
}

func (p *parser) consumeOptionalNewline() { p.match(tokenNewline) }

func (p *parser) atLineEnd() bool {
	return p.check(tokenNewline) || p.check(tokenRBrace) || p.atEnd()
}

func (p *parser) consumeTypeName(context string) Name {
	t := p.consume(tokenIdent, "an UpperCamelCase name "+context)
	if t.Value != "" && !startsUpper(t.Value) {
		p.report(t.Span, "F1101", fmt.Sprintf("`%s` must start with an uppercase letter", t.Value), "use UpperCamelCase for types, entities, pages, and state values")
	}
	return Name{Text: t.Value, Span: t.Span}
}

func (p *parser) consumeName(context string) Name {
	t := p.consume(tokenIdent, "a lowerCamelCase name "+context)
	if t.Value != "" && !startsLower(t.Value) {
		p.report(t.Span, "F1102", fmt.Sprintf("`%s` must start with a lowercase letter", t.Value), "use lowerCamelCase for fields, actions, roles, and bindings")
	}
	return Name{Text: t.Value, Span: t.Span}
}

func (p *parser) consume(kind tokenKind, expected string) token {
	if p.check(kind) {
		return p.advance()
	}
	t := p.peek()
	p.report(t.Span, "F1002", fmt.Sprintf("expected %s, found %s", expected, t.display()), "check the v0 grammar near this location")
	return token{Kind: tokenInvalid, Span: t.Span}
}

func (p *parser) unexpected(expected string) {
	p.report(p.peek().Span, "F1002", fmt.Sprintf("expected %s, found %s", expected, p.peek().display()), "check the v0 grammar near this location")
}

func (p *parser) report(span Span, code, message, hint string) {
	p.diagnostics = append(p.diagnostics, Diagnostic{Code: code, Message: message, Hint: hint, Span: span})
}

func (p *parser) match(kinds ...tokenKind) bool {
	for _, kind := range kinds {
		if p.check(kind) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *parser) check(kind tokenKind) bool {
	return p.peek().Kind == kind
}

func (p *parser) advance() token {
	if !p.atEnd() {
		p.current++
	}
	return p.previous()
}

func (p *parser) atEnd() bool { return p.peek().Kind == tokenEOF }

func (p *parser) peek() token { return p.tokens[p.current] }

func (p *parser) peekAhead(ahead int) token {
	index := p.current + ahead
	if index >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[index]
}

func (p *parser) previous() token {
	if p.current == 0 {
		return p.tokens[0]
	}
	return p.tokens[p.current-1]
}

func startsUpper(value string) bool { return value != "" && value[0] >= 'A' && value[0] <= 'Z' }
func startsLower(value string) bool { return value != "" && value[0] >= 'a' && value[0] <= 'z' }
