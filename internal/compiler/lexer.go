package compiler

import (
	"strconv"
)

type lexer struct {
	source      SourceFile
	offset      int
	line        int
	column      int
	tokens      []token
	diagnostics []Diagnostic
}

func lex(source SourceFile) ([]token, []Diagnostic) {
	l := &lexer{source: source, line: 1, column: 1}
	for !l.atEnd() {
		l.scanToken()
	}
	pos := l.position()
	l.tokens = append(l.tokens, token{Kind: tokenEOF, Span: Span{File: source.Path, Start: pos, End: pos}})
	return l.tokens, l.diagnostics
}

func (l *lexer) scanToken() {
	start := l.position()
	c := l.peek(0)
	switch c {
	case ' ', '\t', '\f':
		l.advance()
	case '\r':
		l.advance()
		if l.peek(0) == '\n' {
			l.advance()
		}
		l.emit(tokenNewline, start, "")
		l.line++
		l.column = 1
	case '\n':
		l.advance()
		l.emit(tokenNewline, start, "")
		l.line++
		l.column = 1
	case '/':
		if l.peek(1) == '/' {
			l.skipComment()
		} else {
			l.scanRegex(start)
		}
	case '{':
		l.single(tokenLBrace, start)
	case '}':
		l.single(tokenRBrace, start)
	case '(':
		l.single(tokenLParen, start)
	case ')':
		l.single(tokenRParen, start)
	case '[':
		l.single(tokenLBracket, start)
	case ']':
		l.single(tokenRBracket, start)
	case ',':
		l.single(tokenComma, start)
	case '.':
		l.single(tokenDot, start)
	case ':':
		l.single(tokenColon, start)
	case '=':
		l.single(tokenEqual, start)
	case '+':
		l.single(tokenPlus, start)
	case '|':
		l.single(tokenPipe, start)
	case '<':
		if l.peek(1) == '=' {
			l.advance()
			l.advance()
			l.emit(tokenLessEqual, start, "")
		} else {
			l.invalid(start, "unexpected `<`", "the first expression slice supports only the `<=` comparison")
			l.advance()
		}
	case '-':
		if l.peek(1) == '>' {
			l.advance()
			l.advance()
			l.emit(tokenArrow, start, "")
		} else if isDigit(l.peek(1)) {
			l.scanNumber(start)
		} else {
			l.invalid(start, "unexpected `-`", "use `->` for a transition or follow `-` with a number")
			l.advance()
		}
	case '"':
		l.scanString(start)
	default:
		if isLetter(c) {
			l.scanIdentifier(start)
		} else if isDigit(c) {
			l.scanNumber(start)
		} else {
			l.invalid(start, "invalid character", "remove the character or replace it with a Forma token")
			l.advance()
		}
	}
}

func (l *lexer) scanIdentifier(start Position) {
	startOffset := l.offset
	for isLetter(l.peek(0)) || isDigit(l.peek(0)) || l.peek(0) == '_' {
		l.advance()
	}
	l.tokens = append(l.tokens, token{
		Kind: tokenIdent, Lexeme: l.source.Text[startOffset:l.offset],
		Value: l.source.Text[startOffset:l.offset], Span: l.span(start),
	})
}

func (l *lexer) scanNumber(start Position) {
	startOffset := l.offset
	if l.peek(0) == '-' {
		l.advance()
	}
	for isDigit(l.peek(0)) {
		l.advance()
	}
	if l.peek(0) == '.' && isDigit(l.peek(1)) {
		l.advance()
		for isDigit(l.peek(0)) {
			l.advance()
		}
	}
	text := l.source.Text[startOffset:l.offset]
	l.tokens = append(l.tokens, token{Kind: tokenNumber, Lexeme: text, Value: text, Span: l.span(start)})
}

func (l *lexer) scanString(start Position) {
	startOffset := l.offset
	l.advance()
	escaped := false
	for !l.atEnd() {
		c := l.peek(0)
		if c == '\n' || c == '\r' {
			break
		}
		l.advance()
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			raw := l.source.Text[startOffset:l.offset]
			value, err := strconv.Unquote(raw)
			if err != nil {
				value = raw[1 : len(raw)-1]
			}
			l.tokens = append(l.tokens, token{Kind: tokenString, Lexeme: raw, Value: value, Span: l.span(start)})
			return
		}
	}
	l.diagnostics = append(l.diagnostics, Diagnostic{
		Code: "F0002", Message: "unterminated string literal", Hint: "close the string with `\"` on the same line", Span: l.span(start),
	})
}

func (l *lexer) scanRegex(start Position) {
	startOffset := l.offset
	l.advance()
	escaped := false
	for !l.atEnd() {
		c := l.peek(0)
		if c == '\n' || c == '\r' {
			break
		}
		l.advance()
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '/' {
			raw := l.source.Text[startOffset:l.offset]
			l.tokens = append(l.tokens, token{Kind: tokenRegex, Lexeme: raw, Value: raw[1 : len(raw)-1], Span: l.span(start)})
			return
		}
	}
	l.diagnostics = append(l.diagnostics, Diagnostic{
		Code: "F0003", Message: "unterminated regular expression", Hint: "close the regular expression with `/` on the same line", Span: l.span(start),
	})
}

func (l *lexer) skipComment() {
	for !l.atEnd() && l.peek(0) != '\n' && l.peek(0) != '\r' {
		l.advance()
	}
}

func (l *lexer) single(kind tokenKind, start Position) {
	l.advance()
	l.emit(kind, start, "")
}

func (l *lexer) emit(kind tokenKind, start Position, value string) {
	l.tokens = append(l.tokens, token{
		Kind: kind, Lexeme: l.source.Text[start.Offset:l.offset], Value: value, Span: l.span(start),
	})
}

func (l *lexer) invalid(start Position, message, hint string) {
	l.diagnostics = append(l.diagnostics, Diagnostic{Code: "F0001", Message: message, Hint: hint, Span: l.span(start)})
}

func (l *lexer) span(start Position) Span {
	return Span{File: l.source.Path, Start: start, End: l.position()}
}

func (l *lexer) position() Position {
	return Position{Offset: l.offset, Line: l.line, Column: l.column}
}

func (l *lexer) advance() byte {
	c := l.source.Text[l.offset]
	l.offset++
	l.column++
	return c
}

func (l *lexer) peek(ahead int) byte {
	index := l.offset + ahead
	if index >= len(l.source.Text) {
		return 0
	}
	return l.source.Text[index]
}

func (l *lexer) atEnd() bool { return l.offset >= len(l.source.Text) }

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
