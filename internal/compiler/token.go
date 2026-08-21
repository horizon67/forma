package compiler

type tokenKind uint8

const (
	tokenInvalid tokenKind = iota
	tokenEOF
	tokenNewline
	tokenIdent
	tokenNumber
	tokenString
	tokenRegex
	tokenLBrace
	tokenRBrace
	tokenLParen
	tokenRParen
	tokenLBracket
	tokenRBracket
	tokenComma
	tokenDot
	tokenColon
	tokenEqual
	tokenPipe
	tokenArrow
	tokenLessEqual
	tokenPlus
)

type token struct {
	Kind   tokenKind
	Lexeme string
	Value  string
	Span   Span
}

func (t token) display() string {
	if t.Kind == tokenEOF {
		return "end of file"
	}
	if t.Kind == tokenNewline {
		return "newline"
	}
	if t.Lexeme != "" {
		return "`" + t.Lexeme + "`"
	}
	return "token"
}
