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
			p.report(p.peek().Span, "F1001", "expected a top-level declaration", "start a declaration with `entry`, `type`, `entity`, `action`, `identity`, `page`, or `role`")
			before := p.current
			p.synchronizeLine()
			if p.current == before && !p.atEnd() {
				p.advance()
			}
			continue
		}
		switch p.peek().Value {
		case "entry":
			if decl := p.parseApplicationEntry(); decl != nil {
				program.Entries = append(program.Entries, decl)
			}
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
		case "identity":
			if decl := p.parseIdentityDecl(); decl != nil {
				program.Identities = append(program.Identities, decl)
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
			p.report(p.peek().Span, "F1001", fmt.Sprintf("unknown declaration `%s`", p.peek().Value), "expected `entry`, `type`, `entity`, `action`, `identity`, `page`, or `role`")
			p.synchronizeLine()
		}
	}
	return program
}

func (p *parser) parseApplicationEntry() *ApplicationEntryDecl {
	start := p.advance().Span
	page := p.consumeTypeName("after `entry`")
	p.finishLine()
	return &ApplicationEntryDecl{Page: page, Span: mergeSpan(start, page.Span)}
}

func (p *parser) parseIdentityDecl() *IdentityDecl {
	start := p.advance().Span
	name := p.consumeTypeName("after `identity`")
	forKeyword := p.consume(tokenIdent, "`for` after the identity name")
	if forKeyword.Value != "" && forKeyword.Value != "for" {
		p.report(forKeyword.Span, "F1002", fmt.Sprintf("expected `for`, found `%s`", forKeyword.Value), "bind the identity to its subject with `identity Name for Entity`")
	}
	subject := p.consumeTypeName("after `for`")
	decl := &IdentityDecl{Name: name, Subject: subject}
	if !p.beginBlock("an identity body") {
		p.synchronizeLine()
		return decl
	}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		if !p.check(tokenIdent) {
			p.unexpected("an identity member")
			p.synchronizeLine()
			continue
		}
		switch p.peek().Value {
		case "identifier":
			decl.Identifiers = append(decl.Identifiers, p.parseIdentityIdentifier())
		case "proof":
			decl.Proofs = append(decl.Proofs, p.parseIdentityProof())
		case "registration":
			item := p.parseIdentityRegistration()
			if decl.Registration != nil {
				p.report(item.Span, "F2701", "duplicate identity registration", "declare registration once per identity")
			} else {
				decl.Registration = item
			}
		case "verification":
			decl.Verifications = append(decl.Verifications, p.parseIdentityVerification())
		case "authentication":
			item := p.parseIdentityAuthentication()
			if decl.Authentication != nil {
				p.report(item.Span, "F2701", "duplicate identity authentication", "declare authentication once per identity")
			} else {
				decl.Authentication = item
			}
		case "ownership":
			decl.Ownerships = append(decl.Ownerships, p.parseIdentityOwnership())
		default:
			p.report(p.peek().Span, "F1001", fmt.Sprintf("unknown identity member `%s`", p.peek().Value), "expected `identifier`, `proof`, `registration`, `verification`, `authentication`, or `ownership`")
			p.synchronizeLine()
		}
	}
	end := p.consume(tokenRBrace, "`}` to close the identity")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseIdentityIdentifier() *IdentityIdentifierDecl {
	start := p.advance().Span
	name := p.consumeName("after `identifier`")
	from := p.consume(tokenIdent, "`from` after the identifier name")
	if from.Value != "" && from.Value != "from" {
		p.report(from.Span, "F1002", fmt.Sprintf("expected `from`, found `%s`", from.Value), "name the subject field explicitly")
	}
	field := p.consumeName("after `from`")
	decl := &IdentityIdentifierDecl{Name: name, Field: field}
	if !p.beginBlock("an identifier body") {
		p.synchronizeLine()
		return decl
	}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		keyword := p.consume(tokenIdent, "`canonicalize` in the identifier body")
		if keyword.Value != "canonicalize" {
			p.report(keyword.Span, "F1004", fmt.Sprintf("unknown identifier member `%s`", keyword.Value), "the first slice supports only `canonicalize`")
			p.synchronizeLine()
			continue
		}
		decl.Canonicalization = append(decl.Canonicalization, p.parseNameList()...)
		p.finishLine()
	}
	end := p.consume(tokenRBrace, "`}` to close the identifier")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseIdentityProof() *IdentityProofDecl {
	start := p.advance().Span
	decl := &IdentityProofDecl{Name: p.consumeName("after `proof`"), Kind: p.consumeName("as the proof kind")}
	if !p.beginBlock("a proof body") {
		p.synchronizeLine()
		return decl
	}
	seen := map[string]Span{}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		member := p.consume(tokenIdent, "a proof member")
		if previous, exists := seen[member.Value]; exists {
			p.report(member.Span, "F2005", fmt.Sprintf("duplicate proof member `%s`", member.Value), fmt.Sprintf("the first member is at line %d", previous.Start.Line))
		}
		seen[member.Value] = member.Span
		switch member.Value {
		case "minLength", "maxLength":
			number := p.consume(tokenNumber, "a positive integer after `"+member.Value+"`")
			value, _ := strconv.Atoi(number.Value)
			if member.Value == "minLength" {
				decl.MinLength = value
			} else {
				decl.MaxLength = value
			}
		case "lengthUnit":
			decl.LengthUnit = p.consumeName("after `lengthUnit`")
		case "preserveWhitespace":
			decl.PreserveWhitespace = true
		default:
			p.report(member.Span, "F1004", fmt.Sprintf("unknown proof member `%s`", member.Value), "the localPassword members are `minLength`, `maxLength`, `lengthUnit`, and `preserveWhitespace`")
		}
		p.finishLine()
	}
	end := p.consume(tokenRBrace, "`}` to close the proof")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseIdentityRegistration() *IdentityRegistrationDecl {
	start := p.advance().Span
	decl := &IdentityRegistrationDecl{Name: p.consumeName("after `registration`")}
	if !p.beginBlock("a registration body") {
		p.synchronizeLine()
		return decl
	}
	p.parseIdentityOperationBody("registration", func(member token) {
		switch member.Value {
		case "identifier":
			decl.Identifier = p.consumeName("after `identifier`")
		case "proof":
			decl.Proof = p.consumeName("after `proof`")
		case "attributes":
			decl.Attributes = p.parseNameList()
		case "initial":
			decl.InitialState = p.consumeName("as the state name after `initial`")
			decl.InitialValue = p.consumeTypeName("as the state value after `initial`")
		case "verification":
			decl.Verification = p.consumeName("after `verification`")
		case "existingIdentifier":
			decl.ExistingIdentifierOutcome = p.consumeName("after `existingIdentifier`")
		default:
			p.report(member.Span, "F1004", fmt.Sprintf("unknown registration member `%s`", member.Value), "use the closed registration member set")
		}
	})
	end := p.consume(tokenRBrace, "`}` to close the registration")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseIdentityVerification() *IdentityVerificationDecl {
	start := p.advance().Span
	decl := &IdentityVerificationDecl{Name: p.consumeName("after `verification`"), Kind: p.consumeName("as the verification kind")}
	if !p.beginBlock("a verification body") {
		p.synchronizeLine()
		return decl
	}
	p.parseIdentityOperationBody("verification", func(member token) {
		switch member.Value {
		case "verify":
			decl.VerifyOperation = p.consumeName("after `verify`")
		case "resend":
			decl.ResendOperation = p.consumeName("after `resend`")
		case "eligible":
			decl.EligibleState = p.consumeName("as the state name after `eligible`")
			decl.EligibleValue = p.consumeTypeName("as the state value after `eligible`")
		case "success":
			decl.SuccessEntity = p.consumeTypeName("as the action entity after `success`")
			p.consume(tokenDot, "`.` in the success action reference")
			decl.SuccessAction = p.consumeName("as the success action")
		case "lifetime":
			number := p.consume(tokenNumber, "an integer after `lifetime`")
			decl.LifetimeAmount, _ = strconv.Atoi(number.Value)
			decl.LifetimeUnit = p.consumeName("as the lifetime unit")
		case "maxUses":
			number := p.consume(tokenNumber, "an integer after `maxUses`")
			decl.MaxUses, _ = strconv.Atoi(number.Value)
		case "rotation":
			decl.Rotation = p.consumeName("after `rotation`")
		case "notice":
			decl.NoticeChannel = p.consumeName("as the notice channel")
			decl.NoticeEmission = p.consumeName("as the notice emission contract")
		case "deliveryFailure":
			decl.DeliveryFailure = p.consumeName("after `deliveryFailure`")
		case "resendDisclosure":
			decl.ResendDisclosure = p.consumeName("after `resendDisclosure`")
		default:
			p.report(member.Span, "F1004", fmt.Sprintf("unknown verification member `%s`", member.Value), "use the closed verification member set")
		}
	})
	end := p.consume(tokenRBrace, "`}` to close the verification")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseIdentityAuthentication() *IdentityAuthenticationDecl {
	start := p.advance().Span
	decl := &IdentityAuthenticationDecl{}
	if !p.beginBlock("an authentication body") {
		p.synchronizeLine()
		return decl
	}
	p.parseIdentityOperationBody("authentication", func(member token) {
		switch member.Value {
		case "identifier":
			decl.Identifier = p.consumeName("after `identifier`")
		case "proof":
			decl.Proof = p.consumeName("after `proof`")
		case "signin":
			decl.SignInOperation = p.consumeName("after `signin`")
		case "signout":
			decl.SignOutOperation = p.consumeName("after `signout`")
		case "eligible":
			decl.EligibleState = p.consumeName("as the state name after `eligible`")
			decl.EligibleValue = p.consumeTypeName("as the state value after `eligible`")
		case "failure":
			decl.FailureDisclosure = p.consumeName("after `failure`")
		default:
			p.report(member.Span, "F1004", fmt.Sprintf("unknown authentication member `%s`", member.Value), "use the closed authentication member set")
		}
	})
	end := p.consume(tokenRBrace, "`}` to close authentication")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseIdentityOperationBody(kind string, parseMember func(token)) {
	seen := map[string]Span{}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		member := p.consume(tokenIdent, "a "+kind+" member")
		if previous, exists := seen[member.Value]; exists {
			p.report(member.Span, "F2005", fmt.Sprintf("duplicate %s member `%s`", kind, member.Value), fmt.Sprintf("the first member is at line %d", previous.Start.Line))
		}
		seen[member.Value] = member.Span
		parseMember(member)
		p.finishLine()
	}
}

func (p *parser) parseIdentityOwnership() *IdentityOwnershipDecl {
	start := p.advance().Span
	name := p.consumeName("after `ownership`")
	p.finishLine()
	return &IdentityOwnershipDecl{Name: name, Span: mergeSpan(start, p.previous().Span)}
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
	for !p.atLineEnd() && !p.check(tokenLBrace) {
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
	if !p.check(tokenLBrace) {
		p.finishLine()
		decl.Span = mergeSpan(start, p.previous().Span)
		return decl
	}
	decl.HasBody = true
	if !p.beginBlock("an action body") {
		p.synchronizeLine()
		decl.Span = mergeSpan(start, p.previous().Span)
		return decl
	}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		if !p.check(tokenIdent) {
			p.unexpected("`changes` in the action body")
			p.synchronizeLine()
			continue
		}
		if p.peek().Value != "changes" {
			member := p.advance()
			p.report(member.Span, "F1004", fmt.Sprintf("unknown action member `%s`", member.Value), "the first action body slice supports only `changes`")
			p.synchronizeLine()
			continue
		}
		decl.Changes = append(decl.Changes, p.parseChangesDecl())
	}
	end := p.consume(tokenRBrace, "`}` to close the action")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseChangesDecl() *ChangesDecl {
	start := p.advance().Span
	decl := &ChangesDecl{}
	if !p.beginBlock("a changes block") {
		p.synchronizeLine()
		decl.Span = mergeSpan(start, p.previous().Span)
		return decl
	}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		if !p.check(tokenIdent) {
			p.unexpected("a change assignment")
			p.synchronizeLine()
			continue
		}
		targetExpression := p.parseFieldExpression("as the change target")
		p.consume(tokenEqual, "`=` after the change target")
		value := p.parseChangeValueExpression()
		decl.Assignments = append(decl.Assignments, &ChangeAssignmentDecl{
			Target: targetExpression.Field,
			Value:  value,
			Span:   mergeSpan(targetExpression.Span, value.Span),
		})
		p.finishLine()
	}
	end := p.consume(tokenRBrace, "`}` to close changes")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseChangeValueExpression() *Expression {
	left := p.parseFieldExpression("as the change value")
	for p.match(tokenPlus) {
		right := p.parseFieldExpression("after `+` in the change value")
		span := mergeSpan(left.Span, right.Span)
		left = &Expression{
			Kind: "binary",
			Binary: &BinaryExpression{
				Operator: "add",
				Left:     left,
				Right:    right,
				Span:     span,
			},
			Span: span,
		}
	}
	return left
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
			p.unexpected("`allow`, `require`, `interact`, `continue`, `list`, `detail`, or `form`")
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
		case "require":
			decl.Requirements = append(decl.Requirements, p.parseAccessRequirement())
		case "interact":
			decl.IdentityInteractions = append(decl.IdentityInteractions, p.parseIdentityInteraction())
		case "continue":
			decl.SurfaceTransitions = append(decl.SurfaceTransitions, p.parseSurfaceTransition())
		case "list":
			decl.Views = append(decl.Views, p.parseView(ViewList))
		case "detail":
			decl.Views = append(decl.Views, p.parseView(ViewDetail))
		case "form":
			decl.Views = append(decl.Views, p.parseView(ViewForm))
		default:
			p.report(p.peek().Span, "F1001", fmt.Sprintf("unknown page member `%s`", p.peek().Value), "expected `allow`, `require`, `interact`, `continue`, `list`, `detail`, or `form`")
			p.synchronizeLine()
		}
	}
	end := p.consume(tokenRBrace, "`}` to close the page")
	decl.Span = mergeSpan(start, end.Span)
	p.consumeOptionalNewline()
	return decl
}

func (p *parser) parseSurfaceTransition() *SurfaceTransitionDecl {
	start := p.advance().Span
	destination := p.consumeTypeName("after `continue`")
	p.finishLine()
	return &SurfaceTransitionDecl{Kind: "continue", Destination: destination, Span: mergeSpan(start, destination.Span)}
}

func (p *parser) parseAccessRequirement() *AccessRequirementDecl {
	start := p.advance().Span
	kind := p.consume(tokenIdent, "`authenticated` or `owner` after `require`")
	requirement := &AccessRequirementDecl{Kind: kind.Value}
	switch kind.Value {
	case "authenticated":
		requirement.Identity = p.consumeTypeName("after `require authenticated`")
	case "owner":
		requirement.Identity = p.consumeTypeName("as the ownership identity")
		p.consume(tokenDot, "`.` before the ownership name")
		requirement.Ownership = p.consumeName("as the ownership name")
		forKeyword := p.consume(tokenIdent, "`for` before the resource binding")
		if forKeyword.Value != "" && forKeyword.Value != "for" {
			p.report(forKeyword.Span, "F1002", fmt.Sprintf("expected `for`, found `%s`", forKeyword.Value), "bind ownership to a page parameter with `for binding`")
		}
		requirement.Binding = p.consumeName("after `for`")
	default:
		p.report(kind.Span, "F1004", fmt.Sprintf("unknown access requirement `%s`", kind.Value), "the Identity requirements are `authenticated` and `owner`")
	}
	p.finishLine()
	requirement.Span = mergeSpan(start, p.previous().Span)
	return requirement
}

func (p *parser) parseIdentityInteraction() *IdentityInteractionDecl {
	start := p.advance().Span
	decl := &IdentityInteractionDecl{Identity: p.consumeTypeName("after `interact`")}
	p.consume(tokenDot, "`.` before the identity operation")
	decl.Operation = p.consumeName("as the identity operation")
	if !p.beginBlock("an identity interaction body") {
		p.synchronizeLine()
		return decl
	}
	seen := map[string]Span{}
	for !p.atEnd() && !p.check(tokenRBrace) {
		p.skipNewlines()
		if p.check(tokenRBrace) || p.atEnd() {
			break
		}
		if p.check(tokenIdent) && p.peek().Value == "require" {
			decl.Requirements = append(decl.Requirements, p.parseAccessRequirement())
			continue
		}
		member := p.consume(tokenIdent, "an interaction member")
		if previous, exists := seen[member.Value]; exists {
			p.report(member.Span, "F2005", fmt.Sprintf("duplicate interaction member `%s`", member.Value), fmt.Sprintf("the first member is at line %d", previous.Start.Line))
		}
		seen[member.Value] = member.Span
		switch member.Value {
		case "fields":
			decl.Fields = p.parseNameList()
		case "identifier":
			name := p.consumeName("after `identifier`")
			decl.Identifier = &name
		case "proof":
			name := p.consumeName("after `proof`")
			decl.Proof = &name
		case "evidence":
			name := p.consumeName("after `evidence`")
			decl.Evidence = &name
		case "success":
			name := p.consumeTypeName("after `success`")
			decl.SuccessPage = &name
		case "stay":
			decl.Stay = true
		case "continue":
			name := p.consumeTypeName("after `continue`")
			decl.Continuation = &name
		case "feedback":
			decl.Feedback = p.parseNameList()
		default:
			p.report(member.Span, "F1004", fmt.Sprintf("unknown interaction member `%s`", member.Value), "use the closed identity interaction member set")
		}
		p.finishLine()
	}
	end := p.consume(tokenRBrace, "`}` to close the identity interaction")
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
		case "columns", "search", "filter":
			valid = true
			mod.Names = p.parseNameList()
		case "actions":
			valid = true
			mod.Names, mod.Destinations = p.parseActionRefList()
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
		switch mod.Kind {
		case "fields":
			valid = true
			mod.Names = p.parseNameList()
		case "actions":
			valid = true
			mod.Names, mod.Destinations = p.parseActionRefList()
		}
	case ViewForm:
		switch mod.Kind {
		case "fields":
			valid = true
			mod.Names = p.parseNameList()
		case "submit":
			valid = true
			name, destination := p.parseActionRef("after `submit`")
			mod.Names = []Name{name}
			mod.Destinations = []Name{destination}
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

// parseActionRefList reads `name [goto Page]` elements. Destinations stays
// index-aligned with the names so an omitted `goto` keeps a zero Name.
func (p *parser) parseActionRefList() ([]Name, []Name) {
	name, destination := p.parseActionRef("in the action list")
	names := []Name{name}
	destinations := []Name{destination}
	for p.match(tokenComma) {
		name, destination = p.parseActionRef("after `,`")
		names = append(names, name)
		destinations = append(destinations, destination)
	}
	return names, destinations
}

func (p *parser) parseActionRef(context string) (Name, Name) {
	name := p.consumeName(context)
	var destination Name
	if p.check(tokenIdent) && p.peek().Value == "goto" {
		p.advance()
		destination = p.consumeTypeName("after `goto`")
	}
	return name, destination
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
