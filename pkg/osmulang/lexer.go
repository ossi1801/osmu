package osmulang

import (
	"fmt"
	"unicode"
)

// TokenKind identifies lexical tokens in .osmu files.
type TokenKind string

const (
	TokEOF        TokenKind = "EOF"
	TokIdent      TokenKind = "IDENT"
	TokInt        TokenKind = "INT"
	TokFloat      TokenKind = "FLOAT"
	TokString     TokenKind = "STRING"

	// Keywords (C#-inspired + OSMU events/queries)
	TokNamespace  TokenKind = "namespace"
	TokUsing      TokenKind = "using"
	TokComponent  TokenKind = "component"
	TokClass      TokenKind = "class"
	TokPublic     TokenKind = "public"
	TokPrivate    TokenKind = "private"
	TokSignal     TokenKind = "signal"
	TokEmit       TokenKind = "emit"
	TokOn         TokenKind = "on"
	TokQuery      TokenKind = "query"
	TokWhere      TokenKind = "where"
	TokOrderBy    TokenKind = "orderby"
	TokAsc        TokenKind = "asc"
	TokDesc       TokenKind = "desc"
	TokIf         TokenKind = "if"
	TokElse       TokenKind = "else"
	TokWhile      TokenKind = "while"
	TokFor        TokenKind = "for"
	TokForeach    TokenKind = "foreach"
	TokIn         TokenKind = "in"
	TokReturn     TokenKind = "return"
	TokVar        TokenKind = "var"
	TokNew        TokenKind = "new"
	TokTrue       TokenKind = "true"
	TokFalse      TokenKind = "false"
	TokThis       TokenKind = "this"

	// Punctuation & Operators
	TokLBrace     TokenKind = "{"
	TokRBrace     TokenKind = "}"
	TokLParen     TokenKind = "("
	TokRParen     TokenKind = ")"
	TokLBracket   TokenKind = "["
	TokRBracket   TokenKind = "]"
	TokSemi       TokenKind = ";"
	TokColon      TokenKind = ":"
	TokComma      TokenKind = ","
	TokDot        TokenKind = "."
	TokAssign     TokenKind = "="
	TokPlusAssign TokenKind = "+="
	TokSubAssign  TokenKind = "-="
	TokPlus       TokenKind = "+"
	TokMinus      TokenKind = "-"
	TokStar       TokenKind = "*"
	TokSlash      TokenKind = "/"
	TokEq         TokenKind = "=="
	TokNeq        TokenKind = "!="
	TokLt         TokenKind = "<"
	TokLte        TokenKind = "<="
	TokGt         TokenKind = ">"
	TokGte        TokenKind = ">="
	TokAnd        TokenKind = "&&"
	TokOr         TokenKind = "||"
	TokBang       TokenKind = "!"
)

var keywords = map[string]TokenKind{
	"namespace": TokNamespace,
	"using":     TokUsing,
	"component": TokComponent,
	"class":     TokClass,
	"public":    TokPublic,
	"private":   TokPrivate,
	"signal":    TokSignal,
	"emit":      TokEmit,
	"on":        TokOn,
	"query":     TokQuery,
	"where":     TokWhere,
	"orderby":   TokOrderBy,
	"asc":       TokAsc,
	"desc":      TokDesc,
	"if":        TokIf,
	"else":      TokElse,
	"while":     TokWhile,
	"for":       TokFor,
	"foreach":   TokForeach,
	"in":        TokIn,
	"return":    TokReturn,
	"var":       TokVar,
	"new":       TokNew,
	"true":      TokTrue,
	"false":     TokFalse,
	"this":      TokThis,
}

// Position represents a 1-indexed line and column in a .osmu source file.
type Position struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Offset int    `json:"offset"`
}

func (p Position) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Token represents a single lexical unit in .osmu source code.
type Token struct {
	Kind   TokenKind `json:"kind"`
	Lexeme string    `json:"lexeme"`
	Pos    Position  `json:"pos"`
}

// Diagnostic represents a syntax or static type error with source location and severity.
type Diagnostic struct {
	Pos      Position `json:"pos"`
	EndCol   int      `json:"end_col"`
	Severity string   `json:"severity"` // "error" or "warning"
	Code     string   `json:"code"`
	Message  string   `json:"message"`
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s [%s]: %s", d.Pos.String(), d.Severity, d.Code, d.Message)
}

// Lexer tokenizes .osmu source text.
type Lexer struct {
	file   string
	src    []rune
	idx    int
	line   int
	col    int
	Errors []Diagnostic
}

// NewLexer creates a Lexer for a .osmu source string.
func NewLexer(file, src string) *Lexer {
	return &Lexer{
		file: file,
		src:  []rune(src),
		line: 1,
		col:  1,
	}
}

// Tokenize scans all tokens from the input source.
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.nextToken()
		tokens = append(tokens, tok)
		if tok.Kind == TokEOF {
			break
		}
	}
	return tokens
}

func (l *Lexer) peek(offset int) rune {
	if l.idx+offset >= len(l.src) {
		return 0
	}
	return l.src[l.idx+offset]
}

func (l *Lexer) advance() rune {
	if l.idx >= len(l.src) {
		return 0
	}
	ch := l.src[l.idx]
	l.idx++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) pos() Position {
	return Position{
		File:   l.file,
		Line:   l.line,
		Column: l.col,
		Offset: l.idx,
	}
}

func (l *Lexer) skipWhitespaceAndComments() {
	for l.idx < len(l.src) {
		ch := l.peek(0)
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		// Single-line comment //
		if ch == '/' && l.peek(1) == '/' {
			for l.idx < len(l.src) && l.peek(0) != '\n' {
				l.advance()
			}
			continue
		}
		// Block comment /* ... */
		if ch == '/' && l.peek(1) == '*' {
			l.advance()
			l.advance()
			for l.idx < len(l.src) && !(l.peek(0) == '*' && l.peek(1) == '/') {
				l.advance()
			}
			if l.idx < len(l.src) {
				l.advance()
				l.advance()
			}
			continue
		}
		break
	}
}

func (l *Lexer) nextToken() Token {
	l.skipWhitespaceAndComments()
	p := l.pos()
	if l.idx >= len(l.src) {
		return Token{Kind: TokEOF, Lexeme: "", Pos: p}
	}

	ch := l.peek(0)

	// Identifiers and keywords
	if unicode.IsLetter(ch) || ch == '_' {
		start := l.idx
		for l.idx < len(l.src) && (unicode.IsLetter(l.peek(0)) || unicode.IsDigit(l.peek(0)) || l.peek(0) == '_') {
			l.advance()
		}
		word := string(l.src[start:l.idx])
		if kw, ok := keywords[word]; ok {
			return Token{Kind: kw, Lexeme: word, Pos: p}
		}
		return Token{Kind: TokIdent, Lexeme: word, Pos: p}
	}

	// Numbers (int or float, with optional trailing 'f' like C#)
	if unicode.IsDigit(ch) {
		start := l.idx
		isFloat := false
		for l.idx < len(l.src) && unicode.IsDigit(l.peek(0)) {
			l.advance()
		}
		if l.peek(0) == '.' && unicode.IsDigit(l.peek(1)) {
			isFloat = true
			l.advance()
			for l.idx < len(l.src) && unicode.IsDigit(l.peek(0)) {
				l.advance()
			}
		}
		numStr := string(l.src[start:l.idx])
		if l.peek(0) == 'f' || l.peek(0) == 'F' {
			isFloat = true
			l.advance()
		}
		if isFloat {
			return Token{Kind: TokFloat, Lexeme: numStr, Pos: p}
		}
		return Token{Kind: TokInt, Lexeme: numStr, Pos: p}
	}

	// String literal
	if ch == '"' {
		l.advance() // consume opening quote
		var buf []rune
		for l.idx < len(l.src) && l.peek(0) != '"' && l.peek(0) != '\n' {
			c := l.advance()
			if c == '\\' && l.idx < len(l.src) {
				esc := l.advance()
				switch esc {
				case 'n':
					buf = append(buf, '\n')
				case 't':
					buf = append(buf, '\t')
				case '"':
					buf = append(buf, '"')
				case '\\':
					buf = append(buf, '\\')
				default:
					buf = append(buf, esc)
				}
			} else {
				buf = append(buf, c)
			}
		}
		if l.peek(0) == '"' {
			l.advance()
		} else {
			l.Errors = append(l.Errors, Diagnostic{
				Pos:      p,
				EndCol:   l.col,
				Severity: "error",
				Code:     "OSMU001",
				Message:  "unterminated string literal",
			})
		}
		return Token{Kind: TokString, Lexeme: string(buf), Pos: p}
	}

	// Two-character operators
	l.advance()
	next := l.peek(0)
	switch ch {
	case '=':
		if next == '=' {
			l.advance()
			return Token{Kind: TokEq, Lexeme: "==", Pos: p}
		}
		return Token{Kind: TokAssign, Lexeme: "=", Pos: p}
	case '!':
		if next == '=' {
			l.advance()
			return Token{Kind: TokNeq, Lexeme: "!=", Pos: p}
		}
		return Token{Kind: TokBang, Lexeme: "!", Pos: p}
	case '<':
		if next == '=' {
			l.advance()
			return Token{Kind: TokLte, Lexeme: "<=", Pos: p}
		}
		return Token{Kind: TokLt, Lexeme: "<", Pos: p}
	case '>':
		if next == '=' {
			l.advance()
			return Token{Kind: TokGte, Lexeme: ">=", Pos: p}
		}
		return Token{Kind: TokGt, Lexeme: ">", Pos: p}
	case '&':
		if next == '&' {
			l.advance()
			return Token{Kind: TokAnd, Lexeme: "&&", Pos: p}
		}
	case '|':
		if next == '|' {
			l.advance()
			return Token{Kind: TokOr, Lexeme: "||", Pos: p}
		}
	case '+':
		if next == '=' {
			l.advance()
			return Token{Kind: TokPlusAssign, Lexeme: "+=", Pos: p}
		}
		return Token{Kind: TokPlus, Lexeme: "+", Pos: p}
	case '-':
		if next == '=' {
			l.advance()
			return Token{Kind: TokSubAssign, Lexeme: "-=", Pos: p}
		}
		return Token{Kind: TokMinus, Lexeme: "-", Pos: p}
	case '*':
		return Token{Kind: TokStar, Lexeme: "*", Pos: p}
	case '/':
		return Token{Kind: TokSlash, Lexeme: "/", Pos: p}
	case '{':
		return Token{Kind: TokLBrace, Lexeme: "{", Pos: p}
	case '}':
		return Token{Kind: TokRBrace, Lexeme: "}", Pos: p}
	case '(':
		return Token{Kind: TokLParen, Lexeme: "(", Pos: p}
	case ')':
		return Token{Kind: TokRParen, Lexeme: ")", Pos: p}
	case '[':
		return Token{Kind: TokLBracket, Lexeme: "[", Pos: p}
	case ']':
		return Token{Kind: TokRBracket, Lexeme: "]", Pos: p}
	case ';':
		return Token{Kind: TokSemi, Lexeme: ";", Pos: p}
	case ':':
		return Token{Kind: TokColon, Lexeme: ":", Pos: p}
	case ',':
		return Token{Kind: TokComma, Lexeme: ",", Pos: p}
	case '.':
		return Token{Kind: TokDot, Lexeme: ".", Pos: p}
	}

	l.Errors = append(l.Errors, Diagnostic{
		Pos:      p,
		EndCol:   p.Column + 1,
		Severity: "error",
		Code:     "OSMU002",
		Message:  fmt.Sprintf("unexpected character %q", ch),
	})
	return l.nextToken()
}
