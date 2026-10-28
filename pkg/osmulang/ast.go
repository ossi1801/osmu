package osmulang

// TypeRef represents a static type annotation in .osmu (e.g., float, int, string, Entity, Vec3, List<Entity>).
type TypeRef struct {
	Name    string   `json:"name"`
	Elem    *TypeRef `json:"elem,omitempty"`
	Pos     Position `json:"pos"`
}

func (t TypeRef) String() string {
	if t.Elem != nil {
		return t.Name + "<" + t.Elem.String() + ">"
	}
	return t.Name
}

func (t TypeRef) Equals(o TypeRef) bool {
	if t.Name != o.Name {
		return false
	}
	if t.Elem == nil && o.Elem == nil {
		return true
	}
	if t.Elem != nil && o.Elem != nil {
		return t.Elem.Equals(*o.Elem)
	}
	return false
}

// FileAST represents a parsed .osmu source file.
type FileAST struct {
	Filename   string           `json:"filename"`
	Namespace  string           `json:"namespace"`
	Usings     []string         `json:"usings,omitempty"`
	Components []*ComponentDecl `json:"components"`
}

// ComponentDecl represents a `component Name : Entity { ... }` or `class Name : Entity { ... }` declaration.
type ComponentDecl struct {
	Name      string          `json:"name"`
	BaseType  string          `json:"base_type"`
	Pos       Position        `json:"pos"`
	Fields    []*FieldDecl    `json:"fields"`
	Signals   []*SignalDecl   `json:"signals"`
	Methods   []*MethodDecl   `json:"methods"`
	Handlers  []*OnSignalDecl `json:"handlers"`
}

// FieldDecl represents a strongly-typed field on a component, optionally with [Export].
type FieldDecl struct {
	Name       string   `json:"name"`
	Type       TypeRef  `json:"type"`
	IsPublic   bool     `json:"is_public"`
	IsExported bool     `json:"is_exported"`
	Init       Expr     `json:"init,omitempty"`
	Pos        Position `json:"pos"`
}

// Param represents a typed parameter in a method or signal.
type Param struct {
	Name string   `json:"name"`
	Type TypeRef  `json:"type"`
	Pos  Position `json:"pos"`
}

// SignalDecl represents `signal Name(Type1 arg1, Type2 arg2);`
type SignalDecl struct {
	Name   string   `json:"name"`
	Params []Param  `json:"params"`
	Pos    Position `json:"pos"`
}

// MethodDecl represents `public ReturnType Name(Params...) { ... }`
type MethodDecl struct {
	Name       string     `json:"name"`
	ReturnType TypeRef    `json:"return_type"`
	Params     []Param    `json:"params"`
	Body       *BlockStmt `json:"body"`
	Pos        Position   `json:"pos"`
}

// OnSignalDecl represents `on SignalName(Params...) { ... }`
type OnSignalDecl struct {
	SignalName string     `json:"signal_name"`
	Params     []Param    `json:"params"`
	Body       *BlockStmt `json:"body"`
	Pos        Position   `json:"pos"`
}

// Stmt is the interface implemented by all statement AST nodes.
type Stmt interface {
	stmtNode()
	StartPos() Position
}

type BlockStmt struct {
	Stmts []Stmt   `json:"stmts"`
	Pos   Position `json:"pos"`
}

func (*BlockStmt) stmtNode()            {}
func (s *BlockStmt) StartPos() Position { return s.Pos }

type VarDeclStmt struct {
	Name       string   `json:"name"`
	Type       TypeRef  `json:"type"` // Type.Name == "var" if inferred
	Init       Expr     `json:"init"`
	Pos        Position `json:"pos"`
}

func (*VarDeclStmt) stmtNode()            {}
func (s *VarDeclStmt) StartPos() Position { return s.Pos }

type AssignStmt struct {
	Target Expr     `json:"target"`
	Op     string   `json:"op"` // "=", "+=", "-="
	Value  Expr     `json:"value"`
	Pos    Position `json:"pos"`
}

func (*AssignStmt) stmtNode()            {}
func (s *AssignStmt) StartPos() Position { return s.Pos }

type IfStmt struct {
	Cond     Expr       `json:"cond"`
	ThenBody *BlockStmt `json:"then_body"`
	ElseBody *BlockStmt `json:"else_body,omitempty"`
	Pos      Position   `json:"pos"`
}

func (*IfStmt) stmtNode()            {}
func (s *IfStmt) StartPos() Position { return s.Pos }

type WhileStmt struct {
	Cond Expr       `json:"cond"`
	Body *BlockStmt `json:"body"`
	Pos  Position   `json:"pos"`
}

func (*WhileStmt) stmtNode()            {}
func (s *WhileStmt) StartPos() Position { return s.Pos }

type ForeachStmt struct {
	VarName    string     `json:"var_name"`
	VarType    TypeRef    `json:"var_type"`
	Collection Expr       `json:"collection"`
	Body       *BlockStmt `json:"body"`
	Pos        Position   `json:"pos"`
}

func (*ForeachStmt) stmtNode()            {}
func (s *ForeachStmt) StartPos() Position { return s.Pos }

type EmitStmt struct {
	SignalName string   `json:"signal_name"`
	Args       []Expr   `json:"args"`
	Pos        Position `json:"pos"`
}

func (*EmitStmt) stmtNode()            {}
func (s *EmitStmt) StartPos() Position { return s.Pos }

type ReturnStmt struct {
	Value Expr     `json:"value,omitempty"`
	Pos   Position `json:"pos"`
}

func (*ReturnStmt) stmtNode()            {}
func (s *ReturnStmt) StartPos() Position { return s.Pos }

type ExprStmt struct {
	Expr Expr     `json:"expr"`
	Pos  Position `json:"pos"`
}

func (*ExprStmt) stmtNode()            {}
func (s *ExprStmt) StartPos() Position { return s.Pos }

// Expr is the interface implemented by all expression AST nodes.
type Expr interface {
	exprNode()
	StartPos() Position
}

type IdentExpr struct {
	Name string   `json:"name"`
	Pos  Position `json:"pos"`
}

func (*IdentExpr) exprNode()            {}
func (e *IdentExpr) StartPos() Position { return e.Pos }

type ThisExpr struct {
	Pos Position `json:"pos"`
}

func (*ThisExpr) exprNode()            {}
func (e *ThisExpr) StartPos() Position { return e.Pos }

type LiteralExpr struct {
	Kind  TokenKind `json:"kind"` // TokInt, TokFloat, TokString, TokTrue, TokFalse
	Value string    `json:"value"`
	Pos   Position  `json:"pos"`
}

func (*LiteralExpr) exprNode()            {}
func (e *LiteralExpr) StartPos() Position { return e.Pos }

type UnaryExpr struct {
	Op      string   `json:"op"`
	Operand Expr     `json:"operand"`
	Pos     Position `json:"pos"`
}

func (*UnaryExpr) exprNode()            {}
func (e *UnaryExpr) StartPos() Position { return e.Pos }

type BinaryExpr struct {
	Left  Expr     `json:"left"`
	Op    string   `json:"op"`
	Right Expr     `json:"right"`
	Pos   Position `json:"pos"`
}

func (*BinaryExpr) exprNode()            {}
func (e *BinaryExpr) StartPos() Position { return e.Pos }

type MemberExpr struct {
	Object Expr     `json:"object"`
	Member string   `json:"member"`
	Pos    Position `json:"pos"`
}

func (*MemberExpr) exprNode()            {}
func (e *MemberExpr) StartPos() Position { return e.Pos }

type IndexExpr struct {
	Object Expr     `json:"object"`
	Index  Expr     `json:"index"`
	Pos    Position `json:"pos"`
}

func (*IndexExpr) exprNode()            {}
func (e *IndexExpr) StartPos() Position { return e.Pos }

type CallExpr struct {
	Callee Expr     `json:"callee"`
	Args   []Expr   `json:"args"`
	Pos    Position `json:"pos"`
}

func (*CallExpr) exprNode()            {}
func (e *CallExpr) StartPos() Position { return e.Pos }

// QueryExpr represents first-class data querying in .osmu:
// `query Entities where Tag == "Enemy" && Health > 0.0 orderby DistanceTo(this.Position) asc`
type QueryExpr struct {
	Source  string   `json:"source"` // "Entities"
	Where   Expr     `json:"where,omitempty"`
	OrderBy Expr     `json:"order_by,omitempty"`
	Asc     bool     `json:"asc"`
	Pos     Position `json:"pos"`
}

func (*QueryExpr) exprNode()            {}
func (e *QueryExpr) StartPos() Position { return e.Pos }
