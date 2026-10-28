package osmulang

import (
	"fmt"
	"strings"
)

var (
	TypeVoid       = TypeRef{Name: "void"}
	TypeInt        = TypeRef{Name: "int"}
	TypeFloat      = TypeRef{Name: "float"}
	TypeBool       = TypeRef{Name: "bool"}
	TypeString     = TypeRef{Name: "string"}
	TypeVec2       = TypeRef{Name: "Vec2"}
	TypeVec3       = TypeRef{Name: "Vec3"}
	TypeColor      = TypeRef{Name: "Color"}
	TypeEntity     = TypeRef{Name: "Entity"}
	TypeEntityList = TypeRef{Name: "List", Elem: &TypeEntity}
	TypeUnknown    = TypeRef{Name: "unknown"}
)

// SymbolInfo stores metadata for IDE/LSP hover, completion, and go-to-definition.
type SymbolInfo struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"` // "component", "field", "method", "signal", "param", "var", "builtin"
	Type   TypeRef  `json:"type"`
	Detail string   `json:"detail"`
	DefPos Position `json:"def_pos"`
}

// CheckResult holds the output of parsing and type-checking a .osmu file.
type CheckResult struct {
	AST         *FileAST
	Diagnostics []Diagnostic
	Symbols     map[string]SymbolInfo
}

// HasErrors returns true if any diagnostic has severity "error".
func (r *CheckResult) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

// CompileAndCheck parses and statically type-checks a .osmu source string.
func CompileAndCheck(filename, src string) *CheckResult {
	ast, parseErrs := ParseFile(filename, src)
	tc := NewTypeChecker(ast, parseErrs)
	tc.Check()
	return &CheckResult{
		AST:         ast,
		Diagnostics: tc.Errors,
		Symbols:     tc.Symbols,
	}
}

// TypeChecker performs static type analysis on a FileAST.
type TypeChecker struct {
	ast          *FileAST
	Errors       []Diagnostic
	Symbols      map[string]SymbolInfo
	curComp      *ComponentDecl
	fields       map[string]TypeRef
	signals      map[string]*SignalDecl
	methods      map[string]*MethodDecl
	locals       map[string]TypeRef
	inQueryScope bool
}

// NewTypeChecker creates a static type checker for the given FileAST.
func NewTypeChecker(ast *FileAST, initialErrs []Diagnostic) *TypeChecker {
	tc := &TypeChecker{
		ast:     ast,
		Errors:  append([]Diagnostic(nil), initialErrs...),
		Symbols: make(map[string]SymbolInfo),
	}
	tc.registerBuiltins()
	return tc
}

func (tc *TypeChecker) registerBuiltins() {
	builtins := []SymbolInfo{
		{Name: "Position", Kind: "property", Type: TypeVec3, Detail: "Vec3 Entity.Position — World 3D/2D position of the entity"},
		{Name: "Rotation", Kind: "property", Type: TypeVec3, Detail: "Vec3 Entity.Rotation — Euler angles in degrees"},
		{Name: "Scale", Kind: "property", Type: TypeVec3, Detail: "Vec3 Entity.Scale — Scale multiplier along X, Y, Z"},
		{Name: "Velocity", Kind: "property", Type: TypeVec3, Detail: "Vec3 Entity.Velocity — Linear velocity vector"},
		{Name: "Color", Kind: "property", Type: TypeColor, Detail: "Color Entity.Color — Render RGBA color"},
		{Name: "Health", Kind: "property", Type: TypeFloat, Detail: "float Entity.Health — Current health points"},
		{Name: "MaxHealth", Kind: "property", Type: TypeFloat, Detail: "float Entity.MaxHealth — Maximum health points"},
		{Name: "Speed", Kind: "property", Type: TypeFloat, Detail: "float Entity.Speed — Movement speed"},
		{Name: "Name", Kind: "property", Type: TypeString, Detail: "string Entity.Name — Display name in scene hierarchy"},
		{Name: "Tag", Kind: "property", Type: TypeString, Detail: "string Entity.Tag — Primary classification tag"},
		{Name: "Label", Kind: "property", Type: TypeString, Detail: "string Entity.Label — Overlay text label rendered on the entity"},
		{Name: "Active", Kind: "property", Type: TypeBool, Detail: "bool Entity.Active — Whether the entity is active in scene"},
		{Name: "Log", Kind: "builtin", Type: TypeVoid, Detail: "void Log(string message) — Prints a message to the OSMU console"},
		{Name: "Distance", Kind: "builtin", Type: TypeFloat, Detail: "float Distance(Vec3 a, Vec3 b) — Euclidean distance between two positions"},
		{Name: "DistanceTo", Kind: "builtin", Type: TypeFloat, Detail: "float DistanceTo(Vec3 target) — Distance from current entity to target in query"},
		{Name: "Vec3", Kind: "builtin", Type: TypeVec3, Detail: "Vec3 Vec3(float x, float y, float z) — Constructs a 3D vector"},
		{Name: "Vec2", Kind: "builtin", Type: TypeVec2, Detail: "Vec2 Vec2(float x, float y) — Constructs a 2D vector"},
		{Name: "Color", Kind: "builtin", Type: TypeColor, Detail: "Color Color(float r, float g, float b, float a) — Constructs an RGBA color"},
		{Name: "Destroy", Kind: "builtin", Type: TypeVoid, Detail: "void Destroy(Entity target) — Deactivates/removes an entity from the scene"},
		{Name: "InputAxisX", Kind: "builtin", Type: TypeFloat, Detail: "float InputAxisX() — Horizontal input axis (-1..1 from A/D or Left/Right keys)"},
		{Name: "InputAxisY", Kind: "builtin", Type: TypeFloat, Detail: "float InputAxisY() — Vertical input axis (-1..1 from S/W or Down/Up keys)"},
		{Name: "IsKeyDown", Kind: "builtin", Type: TypeBool, Detail: "bool IsKeyDown(string key) — Returns true while key is held down"},
		{Name: "Sin", Kind: "builtin", Type: TypeFloat, Detail: "float Sin(float rad) — Trigonometric sine"},
		{Name: "Cos", Kind: "builtin", Type: TypeFloat, Detail: "float Cos(float rad) — Trigonometric cosine"},
		{Name: "GlobalSignal", Kind: "signal", Type: TypeVoid, Detail: "signal GlobalSignal(string eventName, string payload) — Broadcasts a scene-wide signal"},
	}
	for _, b := range builtins {
		tc.Symbols[b.Name] = b
	}
}

func (tc *TypeChecker) errAt(pos Position, code, msg string) {
	tc.Errors = append(tc.Errors, Diagnostic{
		Pos:      pos,
		EndCol:   pos.Column + 8,
		Severity: "error",
		Code:     code,
		Message:  msg,
	})
}

func isValidType(t TypeRef) bool {
	switch t.Name {
	case "int", "float", "bool", "string", "void", "Vec2", "Vec3", "Color", "Entity":
		return true
	case "List":
		return t.Elem != nil && isValidType(*t.Elem)
	default:
		return false
	}
}

func isAssignable(dst, src TypeRef) bool {
	if dst.Name == "unknown" || src.Name == "unknown" {
		return true
	}
	if dst.Equals(src) {
		return true
	}
	// Implicit int -> float widening (like C#)
	if dst.Name == "float" && src.Name == "int" {
		return true
	}
	return false
}

func isNumeric(t TypeRef) bool {
	return t.Name == "int" || t.Name == "float"
}

// Check runs static type checking across all components in the file.
func (tc *TypeChecker) Check() {
	if tc.ast == nil {
		return
	}
	for _, comp := range tc.ast.Components {
		tc.checkComponent(comp)
	}
}

func (tc *TypeChecker) checkComponent(comp *ComponentDecl) {
	tc.curComp = comp
	tc.fields = make(map[string]TypeRef)
	tc.signals = make(map[string]*SignalDecl)
	tc.methods = make(map[string]*MethodDecl)

	tc.Symbols[comp.Name] = SymbolInfo{
		Name:   comp.Name,
		Kind:   "component",
		Type:   TypeEntity,
		Detail: fmt.Sprintf("component %s : %s", comp.Name, comp.BaseType),
		DefPos: comp.Pos,
	}

	if comp.BaseType != "Entity" && comp.BaseType != "Behavior" && comp.BaseType != "Node2D" && comp.BaseType != "Node3D" {
		tc.errAt(comp.Pos, "OSMU200", fmt.Sprintf("unknown base type %q (expected Entity, Behavior, Node2D, or Node3D)", comp.BaseType))
	}

	// Pass 1: Register fields, signals, and methods
	for _, f := range comp.Fields {
		if !isValidType(f.Type) || f.Type.Name == "void" {
			tc.errAt(f.Pos, "OSMU201", fmt.Sprintf("invalid field type %q for field %q", f.Type.String(), f.Name))
		}
		if _, exists := tc.fields[f.Name]; exists {
			tc.errAt(f.Pos, "OSMU202", fmt.Sprintf("duplicate field %q in component %s", f.Name, comp.Name))
		}
		tc.fields[f.Name] = f.Type
		prefix := "private "
		if f.IsPublic {
			prefix = "public "
		}
		if f.IsExported {
			prefix = "[Export] " + prefix
		}
		tc.Symbols[f.Name] = SymbolInfo{
			Name:   f.Name,
			Kind:   "field",
			Type:   f.Type,
			Detail: fmt.Sprintf("%s%s %s.%s", prefix, f.Type.String(), comp.Name, f.Name),
			DefPos: f.Pos,
		}
	}

	for _, s := range comp.Signals {
		for _, p := range s.Params {
			if !isValidType(p.Type) || p.Type.Name == "void" {
				tc.errAt(p.Pos, "OSMU203", fmt.Sprintf("invalid parameter type %q in signal %q", p.Type.String(), s.Name))
			}
		}
		tc.signals[s.Name] = s
		tc.Symbols[s.Name] = SymbolInfo{
			Name:   s.Name,
			Kind:   "signal",
			Type:   TypeVoid,
			Detail: fmt.Sprintf("signal %s(%s)", s.Name, formatParams(s.Params)),
			DefPos: s.Pos,
		}
	}

	for _, m := range comp.Methods {
		if !isValidType(m.ReturnType) {
			tc.errAt(m.Pos, "OSMU204", fmt.Sprintf("unknown return type %q on method %q", m.ReturnType.String(), m.Name))
		}
		for _, p := range m.Params {
			if !isValidType(p.Type) || p.Type.Name == "void" {
				tc.errAt(p.Pos, "OSMU205", fmt.Sprintf("invalid parameter type %q in method %q", p.Type.String(), m.Name))
			}
		}
		tc.methods[m.Name] = m
		tc.Symbols[m.Name] = SymbolInfo{
			Name:   m.Name,
			Kind:   "method",
			Type:   m.ReturnType,
			Detail: fmt.Sprintf("%s %s.%s(%s)", m.ReturnType.String(), comp.Name, m.Name, formatParams(m.Params)),
			DefPos: m.Pos,
		}
	}

	// Pass 2: Check field initializers
	for _, f := range comp.Fields {
		if f.Init != nil {
			initType := tc.checkExpr(f.Init)
			if !isAssignable(f.Type, initType) {
				tc.errAt(f.Pos, "OSMU206", fmt.Sprintf("cannot assign initial value of type %q to field %q of type %q", initType.String(), f.Name, f.Type.String()))
			}
		}
	}

	// Pass 3: Check method bodies
	for _, m := range comp.Methods {
		tc.locals = make(map[string]TypeRef)
		for _, p := range m.Params {
			tc.locals[p.Name] = p.Type
			tc.Symbols[p.Name] = SymbolInfo{
				Name:   p.Name,
				Kind:   "param",
				Type:   p.Type,
				Detail: fmt.Sprintf("(parameter) %s %s", p.Type.String(), p.Name),
				DefPos: p.Pos,
			}
		}
		tc.checkBlock(m.Body, m.ReturnType)
	}

	// Pass 4: Check 'on Signal(...)' handlers
	for _, h := range comp.Handlers {
		if sig, ok := tc.signals[h.SignalName]; ok {
			if len(sig.Params) != len(h.Params) {
				tc.errAt(h.Pos, "OSMU207", fmt.Sprintf("signal handler 'on %s' expects %d parameters, got %d", h.SignalName, len(sig.Params), len(h.Params)))
			} else {
				for i, p := range h.Params {
					if !isAssignable(sig.Params[i].Type, p.Type) {
						tc.errAt(p.Pos, "OSMU208", fmt.Sprintf("parameter %q of 'on %s' has type %q, expected %q", p.Name, h.SignalName, p.Type.String(), sig.Params[i].Type.String()))
					}
				}
			}
		}
		tc.locals = make(map[string]TypeRef)
		for _, p := range h.Params {
			tc.locals[p.Name] = p.Type
			tc.Symbols[p.Name] = SymbolInfo{
				Name:   p.Name,
				Kind:   "param",
				Type:   p.Type,
				Detail: fmt.Sprintf("(signal param) %s %s", p.Type.String(), p.Name),
				DefPos: p.Pos,
			}
		}
		tc.checkBlock(h.Body, TypeVoid)
	}
}

func (tc *TypeChecker) checkBlock(b *BlockStmt, expectedReturn TypeRef) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		tc.checkStmt(s, expectedReturn)
	}
}

func (tc *TypeChecker) checkStmt(s Stmt, expectedReturn TypeRef) {
	switch stmt := s.(type) {
	case *BlockStmt:
		tc.checkBlock(stmt, expectedReturn)

	case *VarDeclStmt:
		var initType TypeRef
		if stmt.Init != nil {
			initType = tc.checkExpr(stmt.Init)
		}
		resolved := stmt.Type
		if resolved.Name == "var" {
			if initType.Name == "" || initType.Name == "void" {
				tc.errAt(stmt.Pos, "OSMU209", fmt.Sprintf("cannot infer type for 'var %s' from void initializer", stmt.Name))
				resolved = TypeUnknown
			} else {
				resolved = initType
			}
		} else {
			if !isValidType(resolved) || resolved.Name == "void" {
				tc.errAt(stmt.Pos, "OSMU210", fmt.Sprintf("invalid variable type %q for %q", resolved.String(), stmt.Name))
			}
			if stmt.Init != nil && !isAssignable(resolved, initType) {
				tc.errAt(stmt.Pos, "OSMU211", fmt.Sprintf("type mismatch: cannot assign %q to variable %q of type %q", initType.String(), stmt.Name, resolved.String()))
			}
		}
		tc.locals[stmt.Name] = resolved
		tc.Symbols[stmt.Name] = SymbolInfo{
			Name:   stmt.Name,
			Kind:   "var",
			Type:   resolved,
			Detail: fmt.Sprintf("(local variable) %s %s", resolved.String(), stmt.Name),
			DefPos: stmt.Pos,
		}

	case *AssignStmt:
		lhsType := tc.checkExpr(stmt.Target)
		rhsType := tc.checkExpr(stmt.Value)
		if stmt.Op == "+=" || stmt.Op == "-=" {
			if !(isNumeric(lhsType) && isNumeric(rhsType)) && !(lhsType.Name == "Vec3" && rhsType.Name == "Vec3") && !(lhsType.Name == "Vec2" && rhsType.Name == "Vec2") {
				tc.errAt(stmt.Pos, "OSMU212", fmt.Sprintf("operator %q cannot be applied to %q and %q", stmt.Op, lhsType.String(), rhsType.String()))
			}
		} else if !isAssignable(lhsType, rhsType) {
			tc.errAt(stmt.Pos, "OSMU213", fmt.Sprintf("type mismatch: cannot assign %q to target of type %q", rhsType.String(), lhsType.String()))
		}

	case *IfStmt:
		condType := tc.checkExpr(stmt.Cond)
		if condType.Name != "bool" && condType.Name != "unknown" {
			tc.errAt(stmt.Pos, "OSMU214", fmt.Sprintf("'if' condition must be of type 'bool', got %q", condType.String()))
		}
		tc.checkBlock(stmt.ThenBody, expectedReturn)
		if stmt.ElseBody != nil {
			tc.checkBlock(stmt.ElseBody, expectedReturn)
		}

	case *WhileStmt:
		condType := tc.checkExpr(stmt.Cond)
		if condType.Name != "bool" && condType.Name != "unknown" {
			tc.errAt(stmt.Pos, "OSMU215", fmt.Sprintf("'while' condition must be of type 'bool', got %q", condType.String()))
		}
		tc.checkBlock(stmt.Body, expectedReturn)

	case *ForeachStmt:
		collType := tc.checkExpr(stmt.Collection)
		elemType := TypeEntity
		if collType.Name == "List" && collType.Elem != nil {
			elemType = *collType.Elem
		} else if collType.Name != "unknown" {
			tc.errAt(stmt.Pos, "OSMU216", fmt.Sprintf("foreach requires a List<T> collection, got %q", collType.String()))
		}
		varType := stmt.VarType
		if varType.Name == "var" {
			varType = elemType
		} else if !isAssignable(varType, elemType) {
			tc.errAt(stmt.Pos, "OSMU217", fmt.Sprintf("foreach loop variable %q has type %q, but collection contains %q", stmt.VarName, varType.String(), elemType.String()))
		}
		tc.locals[stmt.VarName] = varType
		tc.Symbols[stmt.VarName] = SymbolInfo{
			Name:   stmt.VarName,
			Kind:   "var",
			Type:   varType,
			Detail: fmt.Sprintf("(foreach variable) %s %s", varType.String(), stmt.VarName),
			DefPos: stmt.Pos,
		}
		tc.checkBlock(stmt.Body, expectedReturn)

	case *EmitStmt:
		if stmt.SignalName == "GlobalSignal" {
			for _, a := range stmt.Args {
				tc.checkExpr(a)
			}
			return
		}
		sig, ok := tc.signals[stmt.SignalName]
		if !ok {
			tc.errAt(stmt.Pos, "OSMU218", fmt.Sprintf("undeclared signal %q (declare with 'signal %s(...);' or use 'GlobalSignal')", stmt.SignalName, stmt.SignalName))
			for _, a := range stmt.Args {
				tc.checkExpr(a)
			}
			return
		}
		if len(stmt.Args) != len(sig.Params) {
			tc.errAt(stmt.Pos, "OSMU219", fmt.Sprintf("signal %q expects %d arguments, got %d", sig.Name, len(sig.Params), len(stmt.Args)))
		}
		for i, a := range stmt.Args {
			at := tc.checkExpr(a)
			if i < len(sig.Params) && !isAssignable(sig.Params[i].Type, at) {
				tc.errAt(a.StartPos(), "OSMU220", fmt.Sprintf("argument %d of 'emit %s' has type %q, expected %q", i+1, sig.Name, at.String(), sig.Params[i].Type.String()))
			}
		}

	case *ReturnStmt:
		if expectedReturn.Name == "void" {
			if stmt.Value != nil {
				tc.errAt(stmt.Pos, "OSMU221", "void method cannot return a value")
			}
			return
		}
		if stmt.Value == nil {
			tc.errAt(stmt.Pos, "OSMU222", fmt.Sprintf("method expects return value of type %q", expectedReturn.String()))
			return
		}
		rt := tc.checkExpr(stmt.Value)
		if !isAssignable(expectedReturn, rt) {
			tc.errAt(stmt.Pos, "OSMU223", fmt.Sprintf("cannot return value of type %q from method returning %q", rt.String(), expectedReturn.String()))
		}

	case *ExprStmt:
		tc.checkExpr(stmt.Expr)
	}
}

func (tc *TypeChecker) checkExpr(e Expr) TypeRef {
	if e == nil {
		return TypeUnknown
	}
	switch expr := e.(type) {
	case *LiteralExpr:
		switch expr.Kind {
		case TokInt:
			return TypeInt
		case TokFloat:
			return TypeFloat
		case TokString:
			return TypeString
		case TokTrue, TokFalse:
			return TypeBool
		}
		return TypeUnknown

	case *ThisExpr:
		return TypeEntity

	case *IdentExpr:
		if t, ok := tc.locals[expr.Name]; ok {
			return t
		}
		if t, ok := tc.fields[expr.Name]; ok {
			return t
		}
		if propType, ok := entityPropertyType(expr.Name); ok {
			return propType
		}
		tc.errAt(expr.Pos, "OSMU230", fmt.Sprintf("undefined identifier %q", expr.Name))
		return TypeUnknown

	case *UnaryExpr:
		ot := tc.checkExpr(expr.Operand)
		if expr.Op == "-" {
			if isNumeric(ot) || ot.Name == "Vec3" || ot.Name == "Vec2" {
				return ot
			}
			tc.errAt(expr.Pos, "OSMU231", fmt.Sprintf("unary '-' requires numeric or vector operand, got %q", ot.String()))
			return TypeUnknown
		}
		if expr.Op == "!" {
			if ot.Name != "bool" && ot.Name != "unknown" {
				tc.errAt(expr.Pos, "OSMU232", fmt.Sprintf("unary '!' requires bool operand, got %q", ot.String()))
			}
			return TypeBool
		}
		return TypeUnknown

	case *BinaryExpr:
		lt := tc.checkExpr(expr.Left)
		rt := tc.checkExpr(expr.Right)
		if lt.Name == "unknown" || rt.Name == "unknown" {
			switch expr.Op {
			case "==", "!=", "<", "<=", ">", ">=", "&&", "||":
				return TypeBool
			default:
				return TypeUnknown
			}
		}
		switch expr.Op {
		case "&&", "||":
			if lt.Name != "bool" || rt.Name != "bool" {
				tc.errAt(expr.Pos, "OSMU233", fmt.Sprintf("operator %q requires bool operands, got %q and %q", expr.Op, lt.String(), rt.String()))
			}
			return TypeBool
		case "==", "!=":
			if !isAssignable(lt, rt) && !isAssignable(rt, lt) {
				tc.errAt(expr.Pos, "OSMU234", fmt.Sprintf("cannot compare incompatible types %q and %q", lt.String(), rt.String()))
			}
			return TypeBool
		case "<", "<=", ">", ">=":
			if !isNumeric(lt) || !isNumeric(rt) {
				tc.errAt(expr.Pos, "OSMU235", fmt.Sprintf("comparison operator %q requires numeric operands, got %q and %q", expr.Op, lt.String(), rt.String()))
			}
			return TypeBool
		case "+":
			// String concatenation support like C#
			if lt.Name == "string" || rt.Name == "string" {
				return TypeString
			}
			if (lt.Name == "Vec3" && rt.Name == "Vec3") || (lt.Name == "Vec2" && rt.Name == "Vec2") {
				return lt
			}
			if isNumeric(lt) && isNumeric(rt) {
				if lt.Name == "float" || rt.Name == "float" {
					return TypeFloat
				}
				return TypeInt
			}
			tc.errAt(expr.Pos, "OSMU236", fmt.Sprintf("operator '+' cannot be applied to %q and %q", lt.String(), rt.String()))
			return TypeUnknown
		case "-":
			if (lt.Name == "Vec3" && rt.Name == "Vec3") || (lt.Name == "Vec2" && rt.Name == "Vec2") {
				return lt
			}
			if isNumeric(lt) && isNumeric(rt) {
				if lt.Name == "float" || rt.Name == "float" {
					return TypeFloat
				}
				return TypeInt
			}
			tc.errAt(expr.Pos, "OSMU237", fmt.Sprintf("operator '-' cannot be applied to %q and %q", lt.String(), rt.String()))
			return TypeUnknown
		case "*", "/":
			if (lt.Name == "Vec3" || lt.Name == "Vec2") && isNumeric(rt) {
				return lt
			}
			if isNumeric(lt) && isNumeric(rt) {
				if lt.Name == "float" || rt.Name == "float" {
					return TypeFloat
				}
				return TypeInt
			}
			tc.errAt(expr.Pos, "OSMU238", fmt.Sprintf("operator %q cannot be applied to %q and %q", expr.Op, lt.String(), rt.String()))
			return TypeUnknown
		}
		return TypeUnknown

	case *MemberExpr:
		objType := tc.checkExpr(expr.Object)
		return tc.checkMember(objType, expr.Member, expr.Pos)

	case *IndexExpr:
		objType := tc.checkExpr(expr.Object)
		idxType := tc.checkExpr(expr.Index)
		if idxType.Name != "int" && idxType.Name != "unknown" {
			tc.errAt(expr.Pos, "OSMU239", fmt.Sprintf("list index must be of type 'int', got %q", idxType.String()))
		}
		if objType.Name == "List" && objType.Elem != nil {
			return *objType.Elem
		}
		if objType.Name != "unknown" {
			tc.errAt(expr.Pos, "OSMU240", fmt.Sprintf("cannot index value of type %q", objType.String()))
		}
		return TypeUnknown

	case *CallExpr:
		return tc.checkCall(expr)

	case *QueryExpr:
		if expr.Source != "Entities" {
			tc.errAt(expr.Pos, "OSMU241", fmt.Sprintf("unknown query source %q (expected 'Entities')", expr.Source))
		}
		prevQuery := tc.inQueryScope
		tc.inQueryScope = true
		if expr.Where != nil {
			wt := tc.checkExpr(expr.Where)
			if wt.Name != "bool" && wt.Name != "unknown" {
				tc.errAt(expr.Where.StartPos(), "OSMU242", fmt.Sprintf("query 'where' clause must evaluate to 'bool', got %q", wt.String()))
			}
		}
		if expr.OrderBy != nil {
			ot := tc.checkExpr(expr.OrderBy)
			if !isNumeric(ot) && ot.Name != "string" && ot.Name != "unknown" {
				tc.errAt(expr.OrderBy.StartPos(), "OSMU243", fmt.Sprintf("query 'orderby' key must be numeric or string, got %q", ot.String()))
			}
		}
		tc.inQueryScope = prevQuery
		return TypeEntityList
	}
	return TypeUnknown
}

func (tc *TypeChecker) checkMember(objType TypeRef, member string, pos Position) TypeRef {
	switch objType.Name {
	case "Entity":
		// If accessing `this.fieldName` inside component, check component fields first
		if ft, ok := tc.fields[member]; ok {
			return ft
		}
		if pt, ok := entityPropertyType(member); ok {
			return pt
		}
		tc.errAt(pos, "OSMU244", fmt.Sprintf("Entity has no property or field named %q", member))
		return TypeUnknown
	case "Vec3":
		switch member {
		case "X", "Y", "Z", "x", "y", "z":
			return TypeFloat
		}
		tc.errAt(pos, "OSMU245", fmt.Sprintf("Vec3 has no member %q (expected X, Y, or Z)", member))
		return TypeUnknown
	case "Vec2":
		switch member {
		case "X", "Y", "x", "y":
			return TypeFloat
		}
		tc.errAt(pos, "OSMU246", fmt.Sprintf("Vec2 has no member %q (expected X or Y)", member))
		return TypeUnknown
	case "Color":
		switch member {
		case "R", "G", "B", "A", "r", "g", "b", "a":
			return TypeFloat
		}
		tc.errAt(pos, "OSMU247", fmt.Sprintf("Color has no member %q (expected R, G, B, or A)", member))
		return TypeUnknown
	case "List":
		if member == "Count" || member == "Length" {
			return TypeInt
		}
		tc.errAt(pos, "OSMU248", fmt.Sprintf("List<%s> has no property %q (did you mean '.Count'?)", objType.Elem.String(), member))
		return TypeUnknown
	case "unknown":
		return TypeUnknown
	default:
		tc.errAt(pos, "OSMU249", fmt.Sprintf("type %q has no member %q", objType.String(), member))
		return TypeUnknown
	}
}

func (tc *TypeChecker) checkCall(call *CallExpr) TypeRef {
	var argTypes []TypeRef
	for _, a := range call.Args {
		argTypes = append(argTypes, tc.checkExpr(a))
	}

	// Direct function call: e.g. Log(...), Vec3(...), Distance(...), DistanceTo(...), Destroy(...)
	if ident, ok := call.Callee.(*IdentExpr); ok {
		switch ident.Name {
		case "Log":
			if len(argTypes) != 1 {
				tc.errAt(call.Pos, "OSMU250", "Log() expects 1 argument")
			}
			return TypeVoid
		case "Vec3":
			if len(argTypes) != 3 {
				tc.errAt(call.Pos, "OSMU251", "Vec3(x, y, z) expects 3 numeric arguments")
			} else {
				for i, at := range argTypes {
					if !isNumeric(at) && at.Name != "unknown" {
						tc.errAt(call.Args[i].StartPos(), "OSMU251", fmt.Sprintf("Vec3 argument %d must be numeric, got %q", i+1, at.String()))
					}
				}
			}
			return TypeVec3
		case "Vec2":
			if len(argTypes) != 2 {
				tc.errAt(call.Pos, "OSMU252", "Vec2(x, y) expects 2 numeric arguments")
			}
			return TypeVec2
		case "Color":
			if len(argTypes) != 3 && len(argTypes) != 4 {
				tc.errAt(call.Pos, "OSMU253", "Color(r, g, b[, a]) expects 3 or 4 numeric arguments")
			}
			return TypeColor
		case "Distance":
			if len(argTypes) != 2 || (argTypes[0].Name != "Vec3" && argTypes[0].Name != "unknown") || (argTypes[1].Name != "Vec3" && argTypes[1].Name != "unknown") {
				tc.errAt(call.Pos, "OSMU254", "Distance(Vec3 a, Vec3 b) expects two Vec3 arguments")
			}
			return TypeFloat
		case "DistanceTo":
			if len(argTypes) != 1 || (argTypes[0].Name != "Vec3" && argTypes[0].Name != "unknown") {
				tc.errAt(call.Pos, "OSMU255", "DistanceTo(Vec3 pos) expects 1 Vec3 argument")
			}
			return TypeFloat
		case "Destroy":
			if len(argTypes) != 1 || (argTypes[0].Name != "Entity" && argTypes[0].Name != "unknown") {
				tc.errAt(call.Pos, "OSMU256", "Destroy(Entity target) expects 1 Entity argument")
			}
			return TypeVoid
		case "Clamp":
			if len(argTypes) != 3 {
				tc.errAt(call.Pos, "OSMU257", "Clamp(val, min, max) expects 3 numeric arguments")
			}
			return TypeFloat
		case "InputAxisX", "InputAxisY":
			return TypeFloat
		case "IsKeyDown":
			if len(argTypes) != 1 || (argTypes[0].Name != "string" && argTypes[0].Name != "unknown") {
				tc.errAt(call.Pos, "OSMU257", "IsKeyDown(string key) expects 1 string argument")
			}
			return TypeBool
		case "Sin", "Cos":
			if len(argTypes) != 1 {
				tc.errAt(call.Pos, "OSMU257", ident.Name+"(float angle) expects 1 numeric argument")
			}
			return TypeFloat
		case "Spawn":
			if len(argTypes) != 5 {
				tc.errAt(call.Pos, "OSMU257", "Spawn(name, tag, mesh, pos, vel) expects 5 arguments")
			}
			return TypeEntity
		}

		// Component method call without `this.`
		if m, ok := tc.methods[ident.Name]; ok {
			tc.verifyArgs(call, m.Name, m.Params, argTypes)
			return m.ReturnType
		}

		tc.errAt(ident.Pos, "OSMU258", fmt.Sprintf("undefined function or method %q", ident.Name))
		return TypeUnknown
	}

	// Member method call: e.g. `this.LookAt(pos)` or `entity.HasTag("Enemy")`
	if mem, ok := call.Callee.(*MemberExpr); ok {
		objType := tc.checkExpr(mem.Object)
		if objType.Name == "Entity" {
			switch mem.Member {
			case "LookAt":
				if len(argTypes) != 1 || (argTypes[0].Name != "Vec3" && argTypes[0].Name != "unknown") {
					tc.errAt(call.Pos, "OSMU259", "Entity.LookAt(Vec3 target) expects 1 Vec3 argument")
				}
				return TypeVoid
			case "HasTag":
				if len(argTypes) != 1 || (argTypes[0].Name != "string" && argTypes[0].Name != "unknown") {
					tc.errAt(call.Pos, "OSMU260", "Entity.HasTag(string tag) expects 1 string argument")
				}
				return TypeBool
			case "Move":
				if len(argTypes) != 1 || (argTypes[0].Name != "Vec3" && argTypes[0].Name != "unknown") {
					tc.errAt(call.Pos, "OSMU261", "Entity.Move(Vec3 delta) expects 1 Vec3 argument")
				}
				return TypeVoid
			}
			if m, ok := tc.methods[mem.Member]; ok {
				tc.verifyArgs(call, m.Name, m.Params, argTypes)
				return m.ReturnType
			}
		}
		tc.errAt(mem.Pos, "OSMU262", fmt.Sprintf("type %q has no method %q", objType.String(), mem.Member))
		return TypeUnknown
	}

	return TypeUnknown
}

func (tc *TypeChecker) verifyArgs(call *CallExpr, name string, params []Param, got []TypeRef) {
	if len(got) != len(params) {
		tc.errAt(call.Pos, "OSMU263", fmt.Sprintf("method %q expects %d arguments, got %d", name, len(params), len(got)))
		return
	}
	for i, p := range params {
		if !isAssignable(p.Type, got[i]) {
			tc.errAt(call.Args[i].StartPos(), "OSMU264", fmt.Sprintf("argument %d of %q has type %q, expected %q", i+1, name, got[i].String(), p.Type.String()))
		}
	}
}

func entityPropertyType(name string) (TypeRef, bool) {
	switch name {
	case "Position", "Rotation", "Scale", "Velocity":
		return TypeVec3, true
	case "Color":
		return TypeColor, true
	case "Health", "MaxHealth", "Speed", "Score", "Damage", "Energy":
		return TypeFloat, true
	case "Name", "Tag", "ID", "Label":
		return TypeString, true
	case "Active":
		return TypeBool, true
	case "Layer":
		return TypeInt, true
	default:
		return TypeUnknown, false
	}
}

func formatParams(params []Param) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, p.Type.String()+" "+p.Name)
	}
	return strings.Join(parts, ", ")
}
