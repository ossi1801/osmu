package osmulang

import (
	"fmt"
)

// Parser parses a stream of Tokens into a FileAST and records syntax Diagnostics.
type Parser struct {
	file   string
	tokens []Token
	idx    int
	Errors []Diagnostic
}

// ParseFile lexes and parses a .osmu source string.
func ParseFile(filename, src string) (*FileAST, []Diagnostic) {
	lex := NewLexer(filename, src)
	toks := lex.Tokenize()
	p := &Parser{
		file:   filename,
		tokens: toks,
		Errors: append([]Diagnostic(nil), lex.Errors...),
	}
	ast := p.parseFile()
	return ast, p.Errors
}

func (p *Parser) cur() Token {
	if p.idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.idx]
}

func (p *Parser) peek(offset int) Token {
	i := p.idx + offset
	if i >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[i]
}

func (p *Parser) advance() Token {
	t := p.cur()
	if p.idx < len(p.tokens)-1 {
		p.idx++
	}
	return t
}

func (p *Parser) check(k TokenKind) bool {
	return p.cur().Kind == k
}

func (p *Parser) match(kinds ...TokenKind) bool {
	for _, k := range kinds {
		if p.check(k) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) expect(k TokenKind, msg string) Token {
	if p.check(k) {
		return p.advance()
	}
	t := p.cur()
	p.errorAt(t, "OSMU100", fmt.Sprintf("%s (got %q)", msg, t.Lexeme))
	return t
}

func (p *Parser) errorAt(t Token, code, msg string) {
	endCol := t.Pos.Column + len(t.Lexeme)
	if endCol <= t.Pos.Column {
		endCol = t.Pos.Column + 1
	}
	p.Errors = append(p.Errors, Diagnostic{
		Pos:      t.Pos,
		EndCol:   endCol,
		Severity: "error",
		Code:     code,
		Message:  msg,
	})
}

func (p *Parser) synchronizeMember() {
	for !p.check(TokEOF) {
		if p.cur().Kind == TokSemi {
			p.advance()
			return
		}
		switch p.cur().Kind {
		case TokPublic, TokPrivate, TokSignal, TokOn, TokLBracket, TokRBrace, TokComponent, TokClass:
			return
		}
		p.advance()
	}
}

func (p *Parser) synchronizeStmt() {
	for !p.check(TokEOF) {
		if p.cur().Kind == TokSemi {
			p.advance()
			return
		}
		switch p.cur().Kind {
		case TokIf, TokWhile, TokForeach, TokReturn, TokEmit, TokVar, TokRBrace:
			return
		}
		p.advance()
	}
}

func (p *Parser) parseFile() *FileAST {
	ast := &FileAST{
		Filename: p.file,
	}

	for !p.check(TokEOF) {
		if p.match(TokUsing) {
			nameTok := p.expect(TokIdent, "expected namespace identifier after 'using'")
			ast.Usings = append(ast.Usings, nameTok.Lexeme)
			p.expect(TokSemi, "expected ';' after using directive")
			continue
		}
		if p.match(TokNamespace) {
			nameTok := p.expect(TokIdent, "expected namespace name")
			ast.Namespace = nameTok.Lexeme
			if p.match(TokSemi) {
				continue
			}
			// Also support block-scoped namespace { ... }
			if p.match(TokLBrace) {
				for !p.check(TokRBrace) && !p.check(TokEOF) {
					if p.check(TokComponent) || p.check(TokClass) {
						ast.Components = append(ast.Components, p.parseComponent())
					} else {
						p.errorAt(p.cur(), "OSMU101", "expected 'component' or 'class' declaration")
						p.advance()
					}
				}
				p.expect(TokRBrace, "expected '}' to close namespace")
				continue
			}
		}
		if p.check(TokComponent) || p.check(TokClass) {
			ast.Components = append(ast.Components, p.parseComponent())
			continue
		}
		p.errorAt(p.cur(), "OSMU101", fmt.Sprintf("unexpected top-level token %q", p.cur().Lexeme))
		p.advance()
	}

	return ast
}

func (p *Parser) parseComponent() *ComponentDecl {
	kw := p.advance() // component or class
	nameTok := p.expect(TokIdent, "expected component name")
	baseType := "Entity"
	if p.match(TokColon) {
		bTok := p.expect(TokIdent, "expected base type name after ':'")
		baseType = bTok.Lexeme
	}
	p.expect(TokLBrace, "expected '{' to open component body")

	comp := &ComponentDecl{
		Name:     nameTok.Lexeme,
		BaseType: baseType,
		Pos:      kw.Pos,
	}

	for !p.check(TokRBrace) && !p.check(TokEOF) {
		isExported := false
		if p.match(TokLBracket) {
			attr := p.expect(TokIdent, "expected attribute name inside '[...]'")
			if attr.Lexeme == "Export" || attr.Lexeme == "SerializeField" {
				isExported = true
			}
			p.expect(TokRBracket, "expected ']' after attribute")
		}

		if p.match(TokSignal) {
			sig := p.parseSignalDecl()
			comp.Signals = append(comp.Signals, sig)
			continue
		}

		if p.match(TokOn) {
			handler := p.parseOnSignalDecl()
			comp.Handlers = append(comp.Handlers, handler)
			continue
		}

		isPublic := true
		if p.match(TokPublic) {
			isPublic = true
		} else if p.match(TokPrivate) {
			isPublic = false
		}

		if !p.check(TokIdent) {
			p.errorAt(p.cur(), "OSMU102", "expected member type, 'signal', or 'on' handler in component")
			p.synchronizeMember()
			continue
		}

		memberType := p.parseTypeRef()
		memberName := p.expect(TokIdent, "expected field or method name")

		// Method declaration if followed by '('
		if p.check(TokLParen) {
			params := p.parseParamList()
			body := p.parseBlock()
			comp.Methods = append(comp.Methods, &MethodDecl{
				Name:       memberName.Lexeme,
				ReturnType: memberType,
				Params:     params,
				Body:       body,
				Pos:        memberName.Pos,
			})
			continue
		}

		// Field declaration
		var initExpr Expr
		if p.match(TokAssign) {
			initExpr = p.parseExpr()
		}
		p.expect(TokSemi, "expected ';' after field declaration")
		comp.Fields = append(comp.Fields, &FieldDecl{
			Name:       memberName.Lexeme,
			Type:       memberType,
			IsPublic:   isPublic,
			IsExported: isExported,
			Init:       initExpr,
			Pos:        memberName.Pos,
		})
	}

	p.expect(TokRBrace, "expected '}' to close component")
	return comp
}

func (p *Parser) parseTypeRef() TypeRef {
	tok := p.expect(TokIdent, "expected type name")
	tr := TypeRef{
		Name: tok.Lexeme,
		Pos:  tok.Pos,
	}
	if p.match(TokLt) {
		elem := p.parseTypeRef()
		tr.Elem = &elem
		p.expect(TokGt, "expected '>' after generic type argument")
	}
	return tr
}

func (p *Parser) parseParamList() []Param {
	p.expect(TokLParen, "expected '('")
	var params []Param
	if !p.check(TokRParen) {
		for {
			pt := p.parseTypeRef()
			pn := p.expect(TokIdent, "expected parameter name")
			params = append(params, Param{
				Name: pn.Lexeme,
				Type: pt,
				Pos:  pn.Pos,
			})
			if !p.match(TokComma) {
				break
			}
		}
	}
	p.expect(TokRParen, "expected ')' after parameter list")
	return params
}

func (p *Parser) parseSignalDecl() *SignalDecl {
	nameTok := p.expect(TokIdent, "expected signal name")
	params := p.parseParamList()
	p.expect(TokSemi, "expected ';' after signal declaration")
	return &SignalDecl{
		Name:   nameTok.Lexeme,
		Params: params,
		Pos:    nameTok.Pos,
	}
}

func (p *Parser) parseOnSignalDecl() *OnSignalDecl {
	nameTok := p.expect(TokIdent, "expected signal name after 'on'")
	params := p.parseParamList()
	body := p.parseBlock()
	return &OnSignalDecl{
		SignalName: nameTok.Lexeme,
		Params:     params,
		Body:       body,
		Pos:        nameTok.Pos,
	}
}

func (p *Parser) parseBlock() *BlockStmt {
	open := p.expect(TokLBrace, "expected '{'")
	block := &BlockStmt{Pos: open.Pos}
	for !p.check(TokRBrace) && !p.check(TokEOF) {
		stmt := p.parseStmt()
		if stmt != nil {
			block.Stmts = append(block.Stmts, stmt)
		}
	}
	p.expect(TokRBrace, "expected '}'")
	return block
}

func (p *Parser) parseStmt() Stmt {
	cur := p.cur()

	switch cur.Kind {
	case TokLBrace:
		return p.parseBlock()

	case TokIf:
		p.advance()
		p.expect(TokLParen, "expected '(' after 'if'")
		cond := p.parseExpr()
		p.expect(TokRParen, "expected ')' after if condition")
		thenBody := p.parseBlock()
		var elseBody *BlockStmt
		if p.match(TokElse) {
			if p.check(TokIf) {
				nested := p.parseStmt()
				elseBody = &BlockStmt{Stmts: []Stmt{nested}, Pos: nested.StartPos()}
			} else {
				elseBody = p.parseBlock()
			}
		}
		return &IfStmt{Cond: cond, ThenBody: thenBody, ElseBody: elseBody, Pos: cur.Pos}

	case TokWhile:
		p.advance()
		p.expect(TokLParen, "expected '(' after 'while'")
		cond := p.parseExpr()
		p.expect(TokRParen, "expected ')' after while condition")
		body := p.parseBlock()
		return &WhileStmt{Cond: cond, Body: body, Pos: cur.Pos}

	case TokForeach:
		p.advance()
		p.expect(TokLParen, "expected '(' after 'foreach'")
		var vt TypeRef
		if p.match(TokVar) {
			vt = TypeRef{Name: "var", Pos: cur.Pos}
		} else {
			vt = p.parseTypeRef()
		}
		vName := p.expect(TokIdent, "expected loop variable name in foreach")
		p.expect(TokIn, "expected 'in' in foreach statement")
		coll := p.parseExpr()
		p.expect(TokRParen, "expected ')' after foreach header")
		body := p.parseBlock()
		return &ForeachStmt{
			VarName:    vName.Lexeme,
			VarType:    vt,
			Collection: coll,
			Body:       body,
			Pos:        cur.Pos,
		}

	case TokEmit:
		p.advance()
		sigName := p.expect(TokIdent, "expected signal name after 'emit'")
		p.expect(TokLParen, "expected '(' after signal name")
		var args []Expr
		if !p.check(TokRParen) {
			for {
				args = append(args, p.parseExpr())
				if !p.match(TokComma) {
					break
				}
			}
		}
		p.expect(TokRParen, "expected ')' after emit arguments")
		p.expect(TokSemi, "expected ';' after emit statement")
		return &EmitStmt{SignalName: sigName.Lexeme, Args: args, Pos: cur.Pos}

	case TokReturn:
		p.advance()
		var val Expr
		if !p.check(TokSemi) {
			val = p.parseExpr()
		}
		p.expect(TokSemi, "expected ';' after return statement")
		return &ReturnStmt{Value: val, Pos: cur.Pos}

	case TokVar:
		p.advance()
		nameTok := p.expect(TokIdent, "expected variable name after 'var'")
		p.expect(TokAssign, "expected '=' in 'var' declaration")
		initExpr := p.parseExpr()
		p.expect(TokSemi, "expected ';' after variable declaration")
		return &VarDeclStmt{
			Name: nameTok.Lexeme,
			Type: TypeRef{Name: "var", Pos: cur.Pos},
			Init: initExpr,
			Pos:  cur.Pos,
		}
	}

	// Check for explicit typed local variable declaration:
	// e.g. `float dist = ...;` or `List<Entity> enemies = ...;`
	if p.isTypedVarDeclLookahead() {
		vt := p.parseTypeRef()
		nameTok := p.expect(TokIdent, "expected variable name")
		var initExpr Expr
		if p.match(TokAssign) {
			initExpr = p.parseExpr()
		}
		p.expect(TokSemi, "expected ';' after variable declaration")
		return &VarDeclStmt{
			Name: nameTok.Lexeme,
			Type: vt,
			Init: initExpr,
			Pos:  vt.Pos,
		}
	}

	// Otherwise parse expression or assignment statement
	expr := p.parseExpr()
	if p.check(TokAssign) || p.check(TokPlusAssign) || p.check(TokSubAssign) {
		opTok := p.advance()
		rhs := p.parseExpr()
		p.expect(TokSemi, "expected ';' after assignment")
		return &AssignStmt{
			Target: expr,
			Op:     opTok.Lexeme,
			Value:  rhs,
			Pos:    opTok.Pos,
		}
	}

	p.expect(TokSemi, "expected ';' after statement")
	return &ExprStmt{Expr: expr, Pos: expr.StartPos()}
}

func (p *Parser) isTypedVarDeclLookahead() bool {
	if p.cur().Kind != TokIdent {
		return false
	}
	// Ident Ident (=|;)
	if p.peek(1).Kind == TokIdent && (p.peek(2).Kind == TokAssign || p.peek(2).Kind == TokSemi) {
		return true
	}
	// Ident < Ident > Ident (=|;)
	if p.peek(1).Kind == TokLt && p.peek(2).Kind == TokIdent && p.peek(3).Kind == TokGt && p.peek(4).Kind == TokIdent {
		return true
	}
	return false
}

// Expression parsing (precedence climbing)

func (p *Parser) parseExpr() Expr {
	if p.check(TokQuery) {
		return p.parseQueryExpr()
	}
	return p.parseLogicalOr()
}

func (p *Parser) parseQueryExpr() Expr {
	qTok := p.advance() // consume 'query'
	srcTok := p.expect(TokIdent, "expected query source (e.g. 'Entities') after 'query'")
	var whereExpr Expr
	if p.match(TokWhere) {
		whereExpr = p.parseLogicalOr()
	}
	var orderBy Expr
	asc := true
	if p.match(TokOrderBy) {
		orderBy = p.parsePostfix()
		if p.match(TokDesc) {
			asc = false
		} else {
			p.match(TokAsc)
		}
	}
	return &QueryExpr{
		Source:  srcTok.Lexeme,
		Where:   whereExpr,
		OrderBy: orderBy,
		Asc:     asc,
		Pos:     qTok.Pos,
	}
}

func (p *Parser) parseLogicalOr() Expr {
	left := p.parseLogicalAnd()
	for p.check(TokOr) {
		op := p.advance()
		right := p.parseLogicalAnd()
		left = &BinaryExpr{Left: left, Op: op.Lexeme, Right: right, Pos: op.Pos}
	}
	return left
}

func (p *Parser) parseLogicalAnd() Expr {
	left := p.parseEquality()
	for p.check(TokAnd) {
		op := p.advance()
		right := p.parseEquality()
		left = &BinaryExpr{Left: left, Op: op.Lexeme, Right: right, Pos: op.Pos}
	}
	return left
}

func (p *Parser) parseEquality() Expr {
	left := p.parseComparison()
	for p.check(TokEq) || p.check(TokNeq) {
		op := p.advance()
		right := p.parseComparison()
		left = &BinaryExpr{Left: left, Op: op.Lexeme, Right: right, Pos: op.Pos}
	}
	return left
}

func (p *Parser) parseComparison() Expr {
	left := p.parseTerm()
	for p.check(TokLt) || p.check(TokLte) || p.check(TokGt) || p.check(TokGte) {
		op := p.advance()
		right := p.parseTerm()
		left = &BinaryExpr{Left: left, Op: op.Lexeme, Right: right, Pos: op.Pos}
	}
	return left
}

func (p *Parser) parseTerm() Expr {
	left := p.parseFactor()
	for p.check(TokPlus) || p.check(TokMinus) {
		op := p.advance()
		right := p.parseFactor()
		left = &BinaryExpr{Left: left, Op: op.Lexeme, Right: right, Pos: op.Pos}
	}
	return left
}

func (p *Parser) parseFactor() Expr {
	left := p.parseUnary()
	for p.check(TokStar) || p.check(TokSlash) {
		op := p.advance()
		right := p.parseUnary()
		left = &BinaryExpr{Left: left, Op: op.Lexeme, Right: right, Pos: op.Pos}
	}
	return left
}

func (p *Parser) parseUnary() Expr {
	if p.check(TokMinus) || p.check(TokBang) {
		op := p.advance()
		operand := p.parseUnary()
		return &UnaryExpr{Op: op.Lexeme, Operand: operand, Pos: op.Pos}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Expr {
	expr := p.parsePrimary()
	for {
		if p.match(TokDot) {
			mem := p.expect(TokIdent, "expected property or method name after '.'")
			expr = &MemberExpr{Object: expr, Member: mem.Lexeme, Pos: mem.Pos}
			continue
		}
		if p.match(TokLParen) {
			var args []Expr
			if !p.check(TokRParen) {
				for {
					args = append(args, p.parseExpr())
					if !p.match(TokComma) {
						break
					}
				}
			}
			closeTok := p.expect(TokRParen, "expected ')' after arguments")
			expr = &CallExpr{Callee: expr, Args: args, Pos: closeTok.Pos}
			continue
		}
		if p.match(TokLBracket) {
			idx := p.parseExpr()
			closeTok := p.expect(TokRBracket, "expected ']' after index")
			expr = &IndexExpr{Object: expr, Index: idx, Pos: closeTok.Pos}
			continue
		}
		break
	}
	return expr
}

func (p *Parser) parsePrimary() Expr {
	cur := p.cur()
	switch cur.Kind {
	case TokInt, TokFloat, TokString, TokTrue, TokFalse:
		p.advance()
		return &LiteralExpr{Kind: cur.Kind, Value: cur.Lexeme, Pos: cur.Pos}
	case TokThis:
		p.advance()
		return &ThisExpr{Pos: cur.Pos}
	case TokNew:
		p.advance()
		typeTok := p.expect(TokIdent, "expected type name after 'new'")
		p.expect(TokLParen, "expected '(' after type name")
		var args []Expr
		if !p.check(TokRParen) {
			for {
				args = append(args, p.parseExpr())
				if !p.match(TokComma) {
					break
				}
			}
		}
		p.expect(TokRParen, "expected ')' after constructor arguments")
		return &CallExpr{
			Callee: &IdentExpr{Name: typeTok.Lexeme, Pos: typeTok.Pos},
			Args:   args,
			Pos:    cur.Pos,
		}
	case TokIdent:
		p.advance()
		return &IdentExpr{Name: cur.Lexeme, Pos: cur.Pos}
	case TokLParen:
		p.advance()
		inner := p.parseExpr()
		p.expect(TokRParen, "expected ')' after expression")
		return inner
	default:
		p.errorAt(cur, "OSMU103", fmt.Sprintf("expected expression, got %q", cur.Lexeme))
		p.advance()
		return &LiteralExpr{Kind: TokInt, Value: "0", Pos: cur.Pos}
	}
}
