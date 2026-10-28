package osmulang

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"osmu/pkg/engine"
)

// RuntimeWorld defines the scene interface needed by the .osmu VM.
type RuntimeWorld interface {
	Entities() []*engine.Entity
	EventBus() *engine.EventBus
	LogScript(msg string)
}

// InteractiveWorld extends RuntimeWorld with keyboard input and dynamic entity spawning.
type InteractiveWorld interface {
	RuntimeWorld
	InputAxisX() float64
	InputAxisY() float64
	IsKeyDown(key string) bool
	SpawnEntity(name, tag, mesh string, pos, vel engine.Vec3) *engine.Entity
}

// ComponentInstance is a live runtime instance of a .osmu ComponentDecl attached to an Entity.
type ComponentInstance struct {
	Decl   *ComponentDecl
	Entity *engine.Entity
	Fields map[string]interface{}
	World  RuntimeWorld
}

// NewComponentInstance creates and initializes a runtime component instance for an entity.
func NewComponentInstance(decl *ComponentDecl, ent *engine.Entity, world RuntimeWorld, overrides map[string]interface{}) *ComponentInstance {
	inst := &ComponentInstance{
		Decl:   decl,
		Entity: ent,
		Fields: make(map[string]interface{}),
		World:  world,
	}
	env := make(map[string]interface{})
	for _, f := range decl.Fields {
		if f.Init != nil {
			val, _ := inst.evalExpr(f.Init, env, nil)
			inst.Fields[f.Name] = val
		} else {
			inst.Fields[f.Name] = defaultValueForType(f.Type)
		}
		if overrides != nil {
			if ov, ok := overrides[f.Name]; ok {
				inst.Fields[f.Name] = ov
			}
		}
	}
	return inst
}

func defaultValueForType(t TypeRef) interface{} {
	switch t.Name {
	case "int":
		return 0
	case "float":
		return 0.0
	case "bool":
		return false
	case "string":
		return ""
	case "Vec2":
		return engine.V2(0, 0)
	case "Vec3":
		return engine.V3(0, 0, 0)
	case "Color":
		return engine.RGB(1, 1, 1)
	case "List":
		return []*engine.Entity{}
	default:
		return nil
	}
}

// CallMethod invokes a method on the component instance if it exists.
func (inst *ComponentInstance) CallMethod(name string, args ...interface{}) interface{} {
	for _, m := range inst.Decl.Methods {
		if m.Name == name {
			env := make(map[string]interface{}, len(m.Params)+4)
			for i, p := range m.Params {
				if i < len(args) {
					env[p.Name] = args[i]
				}
			}
			ret, _ := inst.execBlock(m.Body, env)
			return ret
		}
	}
	return nil
}

// HandleSignal invokes any matching `on SignalName(...)` blocks on this component.
func (inst *ComponentInstance) HandleSignal(sigName string, args ...interface{}) {
	for _, h := range inst.Decl.Handlers {
		if h.SignalName == sigName {
			env := make(map[string]interface{}, len(h.Params)+4)
			for i, p := range h.Params {
				if i < len(args) {
					env[p.Name] = args[i]
				}
			}
			inst.execBlock(h.Body, env)
		}
	}
}

func (inst *ComponentInstance) execBlock(b *BlockStmt, env map[string]interface{}) (interface{}, bool) {
	if b == nil {
		return nil, false
	}
	for _, s := range b.Stmts {
		val, returned := inst.execStmt(s, env)
		if returned {
			return val, true
		}
	}
	return nil, false
}

func (inst *ComponentInstance) execStmt(s Stmt, env map[string]interface{}) (interface{}, bool) {
	switch stmt := s.(type) {
	case *BlockStmt:
		return inst.execBlock(stmt, env)

	case *VarDeclStmt:
		var val interface{}
		if stmt.Init != nil {
			val, _ = inst.evalExpr(stmt.Init, env, nil)
		} else {
			val = defaultValueForType(stmt.Type)
		}
		env[stmt.Name] = val
		return nil, false

	case *AssignStmt:
		rhs, _ := inst.evalExpr(stmt.Value, env, nil)
		inst.assignTarget(stmt.Target, stmt.Op, rhs, env)
		return nil, false

	case *IfStmt:
		cond, _ := inst.evalExpr(stmt.Cond, env, nil)
		if toBool(cond) {
			return inst.execBlock(stmt.ThenBody, env)
		} else if stmt.ElseBody != nil {
			return inst.execBlock(stmt.ElseBody, env)
		}
		return nil, false

	case *WhileStmt:
		steps := 0
		for steps < 1000 {
			steps++
			cond, _ := inst.evalExpr(stmt.Cond, env, nil)
			if !toBool(cond) {
				break
			}
			if val, ret := inst.execBlock(stmt.Body, env); ret {
				return val, true
			}
		}
		return nil, false

	case *ForeachStmt:
		coll, _ := inst.evalExpr(stmt.Collection, env, nil)
		if list, ok := coll.([]*engine.Entity); ok {
			for _, item := range list {
				env[stmt.VarName] = item
				if val, ret := inst.execBlock(stmt.Body, env); ret {
					return val, true
				}
			}
		}
		return nil, false

	case *EmitStmt:
		var evalArgs []interface{}
		for _, a := range stmt.Args {
			v, _ := inst.evalExpr(a, env, nil)
			evalArgs = append(evalArgs, v)
		}
		sigName := stmt.SignalName
		if sigName == "GlobalSignal" && len(evalArgs) > 0 {
			sigName = fmt.Sprint(evalArgs[0])
			evalArgs = evalArgs[1:]
		}
		// Trigger local `on Signal` handlers immediately + broadcast on scene EventBus
		inst.HandleSignal(sigName, evalArgs...)
		if inst.World != nil && inst.World.EventBus() != nil {
			srcID := ""
			if inst.Entity != nil {
				srcID = inst.Entity.ID
			}
			inst.World.EventBus().Emit(sigName, srcID, evalArgs...)
		}
		return nil, false

	case *ReturnStmt:
		if stmt.Value == nil {
			return nil, true
		}
		v, _ := inst.evalExpr(stmt.Value, env, nil)
		return v, true

	case *ExprStmt:
		inst.evalExpr(stmt.Expr, env, nil)
		return nil, false
	}
	return nil, false
}

func (inst *ComponentInstance) assignTarget(target Expr, op string, rhs interface{}, env map[string]interface{}) {
	combine := func(cur, delta interface{}) interface{} {
		if op == "=" {
			return delta
		}
		if v1, ok := cur.(engine.Vec3); ok {
			if v2, ok2 := delta.(engine.Vec3); ok2 {
				if op == "+=" {
					return v1.Add(v2)
				}
				return v1.Sub(v2)
			}
		}
		if v1, ok := cur.(engine.Vec2); ok {
			if v2, ok2 := delta.(engine.Vec2); ok2 {
				if op == "+=" {
					return v1.Add(v2)
				}
				return v1.Sub(v2)
			}
		}
		f1, _ := engine.ToFloat64(cur)
		f2, _ := engine.ToFloat64(delta)
		if op == "+=" {
			return f1 + f2
		}
		return f1 - f2
	}

	switch t := target.(type) {
	case *IdentExpr:
		if _, ok := env[t.Name]; ok {
			env[t.Name] = combine(env[t.Name], rhs)
			return
		}
		if _, ok := inst.Fields[t.Name]; ok {
			inst.Fields[t.Name] = combine(inst.Fields[t.Name], rhs)
			return
		}
		if inst.Entity != nil {
			cur, _ := inst.Entity.GetProperty(t.Name)
			inst.Entity.SetProperty(t.Name, combine(cur, rhs))
		}

	case *MemberExpr:
		// Check if assigning to `this.field` or `entity.Property` or `entity.Position.X`
		if _, isThis := t.Object.(*ThisExpr); isThis {
			if _, ok := inst.Fields[t.Member]; ok {
				inst.Fields[t.Member] = combine(inst.Fields[t.Member], rhs)
				return
			}
			if inst.Entity != nil {
				cur, _ := inst.Entity.GetProperty(t.Member)
				inst.Entity.SetProperty(t.Member, combine(cur, rhs))
			}
			return
		}
		objVal, _ := inst.evalExpr(t.Object, env, nil)
		if ent, ok := objVal.(*engine.Entity); ok && ent != nil {
			cur, _ := ent.GetProperty(t.Member)
			ent.SetProperty(t.Member, combine(cur, rhs))
			return
		}
		// Support `this.Position.X = ...`
		if innerMem, ok := t.Object.(*MemberExpr); ok {
			ownerVal, _ := inst.evalExpr(innerMem.Object, env, nil)
			if ent, ok := ownerVal.(*engine.Entity); ok && ent != nil {
				v, ok := ent.GetProperty(innerMem.Member)
				if ok {
					if vec, isV3 := v.(engine.Vec3); isV3 {
						switch strings.ToUpper(t.Member) {
						case "X":
							vec.X, _ = engine.ToFloat64(combine(vec.X, rhs))
						case "Y":
							vec.Y, _ = engine.ToFloat64(combine(vec.Y, rhs))
						case "Z":
							vec.Z, _ = engine.ToFloat64(combine(vec.Z, rhs))
						}
						ent.SetProperty(innerMem.Member, vec)
					}
				}
			}
		}
	}
}

func (inst *ComponentInstance) evalExpr(e Expr, env map[string]interface{}, queryCandidate *engine.Entity) (interface{}, error) {
	if e == nil {
		return nil, nil
	}
	switch expr := e.(type) {
	case *LiteralExpr:
		switch expr.Kind {
		case TokInt:
			n, _ := strconv.Atoi(expr.Value)
			return n, nil
		case TokFloat:
			f, _ := strconv.ParseFloat(expr.Value, 64)
			return f, nil
		case TokString:
			return expr.Value, nil
		case TokTrue:
			return true, nil
		case TokFalse:
			return false, nil
		}
		return nil, nil

	case *ThisExpr:
		return inst.Entity, nil

	case *IdentExpr:
		// If inside a `query Entities where ...` expression, check candidate entity properties first!
		if queryCandidate != nil {
			if v, ok := queryCandidate.GetProperty(expr.Name); ok {
				return v, nil
			}
		}
		if v, ok := env[expr.Name]; ok {
			return v, nil
		}
		if v, ok := inst.Fields[expr.Name]; ok {
			return v, nil
		}
		if inst.Entity != nil {
			if v, ok := inst.Entity.GetProperty(expr.Name); ok {
				return v, nil
			}
		}
		return nil, nil

	case *UnaryExpr:
		v, _ := inst.evalExpr(expr.Operand, env, queryCandidate)
		if expr.Op == "!" {
			return !toBool(v), nil
		}
		if expr.Op == "-" {
			if vec, ok := v.(engine.Vec3); ok {
				return vec.Scale(-1), nil
			}
			f, _ := engine.ToFloat64(v)
			return -f, nil
		}
		return v, nil

	case *BinaryExpr:
		if expr.Op == "&&" {
			lv, _ := inst.evalExpr(expr.Left, env, queryCandidate)
			if !toBool(lv) {
				return false, nil
			}
			rv, _ := inst.evalExpr(expr.Right, env, queryCandidate)
			return toBool(rv), nil
		}
		if expr.Op == "||" {
			lv, _ := inst.evalExpr(expr.Left, env, queryCandidate)
			if toBool(lv) {
				return true, nil
			}
			rv, _ := inst.evalExpr(expr.Right, env, queryCandidate)
			return toBool(rv), nil
		}
		lv, _ := inst.evalExpr(expr.Left, env, queryCandidate)
		rv, _ := inst.evalExpr(expr.Right, env, queryCandidate)
		return evalBinaryOp(lv, expr.Op, rv), nil

	case *MemberExpr:
		if _, isThis := expr.Object.(*ThisExpr); isThis {
			if v, ok := inst.Fields[expr.Member]; ok {
				return v, nil
			}
			if inst.Entity != nil {
				if v, ok := inst.Entity.GetProperty(expr.Member); ok {
					return v, nil
				}
			}
		}
		obj, _ := inst.evalExpr(expr.Object, env, queryCandidate)
		return evalMemberAccess(obj, expr.Member), nil

	case *IndexExpr:
		obj, _ := inst.evalExpr(expr.Object, env, queryCandidate)
		idxVal, _ := inst.evalExpr(expr.Index, env, queryCandidate)
		idxFloat, _ := engine.ToFloat64(idxVal)
		idx := int(idxFloat)
		if list, ok := obj.([]*engine.Entity); ok {
			if idx >= 0 && idx < len(list) {
				return list[idx], nil
			}
		}
		return nil, nil

	case *CallExpr:
		return inst.evalCall(expr, env, queryCandidate)

	case *QueryExpr:
		if inst.World == nil {
			return []*engine.Entity{}, nil
		}
		var matched []*engine.Entity
		for _, cand := range inst.World.Entities() {
			if cand == nil || !cand.Active {
				continue
			}
			if expr.Where != nil {
				condVal, _ := inst.evalExpr(expr.Where, env, cand)
				if !toBool(condVal) {
					continue
				}
			}
			matched = append(matched, cand)
		}
		if expr.OrderBy != nil && len(matched) > 1 {
			sort.SliceStable(matched, func(i, j int) bool {
				ki, _ := inst.evalExpr(expr.OrderBy, env, matched[i])
				kj, _ := inst.evalExpr(expr.OrderBy, env, matched[j])
				fi, oki := engine.ToFloat64(ki)
				fj, okj := engine.ToFloat64(kj)
				if oki && okj {
					if expr.Asc {
						return fi < fj
					}
					return fi > fj
				}
				si := fmt.Sprint(ki)
				sj := fmt.Sprint(kj)
				if expr.Asc {
					return si < sj
				}
				return si > sj
			})
		}
		return matched, nil
	}
	return nil, nil
}

func (inst *ComponentInstance) evalCall(call *CallExpr, env map[string]interface{}, queryCandidate *engine.Entity) (interface{}, error) {
	var args []interface{}
	for _, a := range call.Args {
		v, _ := inst.evalExpr(a, env, queryCandidate)
		args = append(args, v)
	}

	if ident, ok := call.Callee.(*IdentExpr); ok {
		switch ident.Name {
		case "Log":
			if len(args) > 0 && inst.World != nil {
				inst.World.LogScript(fmt.Sprint(args[0]))
			}
			return nil, nil
		case "Vec3":
			if len(args) >= 3 {
				x, _ := engine.ToFloat64(args[0])
				y, _ := engine.ToFloat64(args[1])
				z, _ := engine.ToFloat64(args[2])
				return engine.V3(x, y, z), nil
			}
			return engine.V3(0, 0, 0), nil
		case "Vec2":
			if len(args) >= 2 {
				x, _ := engine.ToFloat64(args[0])
				y, _ := engine.ToFloat64(args[1])
				return engine.V2(x, y), nil
			}
			return engine.V2(0, 0), nil
		case "Color":
			if len(args) >= 3 {
				r, _ := engine.ToFloat64(args[0])
				g, _ := engine.ToFloat64(args[1])
				b, _ := engine.ToFloat64(args[2])
				a := 1.0
				if len(args) >= 4 {
					a, _ = engine.ToFloat64(args[3])
				}
				return engine.RGBA(r, g, b, a), nil
			}
			return engine.RGB(1, 1, 1), nil
		case "Distance":
			if len(args) >= 2 {
				a, ok1 := args[0].(engine.Vec3)
				b, ok2 := args[1].(engine.Vec3)
				if ok1 && ok2 {
					return a.Dist(b), nil
				}
			}
			return 0.0, nil
		case "DistanceTo":
			if len(args) >= 1 {
				target, ok := args[0].(engine.Vec3)
				if ok {
					subject := inst.Entity
					if queryCandidate != nil {
						subject = queryCandidate
					}
					if subject != nil {
						return subject.Transform.Position.Dist(target), nil
					}
				}
			}
			return 0.0, nil
		case "Destroy":
			if len(args) >= 1 {
				if ent, ok := args[0].(*engine.Entity); ok && ent != nil {
					ent.Active = false
				}
			}
			return nil, nil
		case "Clamp":
			if len(args) >= 3 {
				v, _ := engine.ToFloat64(args[0])
				lo, _ := engine.ToFloat64(args[1])
				hi, _ := engine.ToFloat64(args[2])
				return engine.Clamp(v, lo, hi), nil
			}
			return 0.0, nil
		case "InputAxisX":
			if iw, ok := inst.World.(InteractiveWorld); ok {
				return iw.InputAxisX(), nil
			}
			return 0.0, nil
		case "InputAxisY":
			if iw, ok := inst.World.(InteractiveWorld); ok {
				return iw.InputAxisY(), nil
			}
			return 0.0, nil
		case "IsKeyDown":
			if len(args) >= 1 {
				if iw, ok := inst.World.(InteractiveWorld); ok {
					return iw.IsKeyDown(fmt.Sprint(args[0])), nil
				}
			}
			return false, nil
		case "Sin":
			if len(args) >= 1 {
				v, _ := engine.ToFloat64(args[0])
				return math.Sin(v), nil
			}
			return 0.0, nil
		case "Cos":
			if len(args) >= 1 {
				v, _ := engine.ToFloat64(args[0])
				return math.Cos(v), nil
			}
			return 1.0, nil
		case "Spawn":
			if len(args) >= 5 {
				if iw, ok := inst.World.(InteractiveWorld); ok {
					pos, _ := args[3].(engine.Vec3)
					vel, _ := args[4].(engine.Vec3)
					return iw.SpawnEntity(fmt.Sprint(args[0]), fmt.Sprint(args[1]), fmt.Sprint(args[2]), pos, vel), nil
				}
			}
			return nil, nil
		}
		return inst.CallMethod(ident.Name, args...), nil
	}

	if mem, ok := call.Callee.(*MemberExpr); ok {
		obj, _ := inst.evalExpr(mem.Object, env, queryCandidate)
		if ent, ok := obj.(*engine.Entity); ok && ent != nil {
			switch mem.Member {
			case "LookAt":
				if len(args) >= 1 {
					if target, ok := args[0].(engine.Vec3); ok {
						ent.LookAt(target)
					}
				}
				return nil, nil
			case "HasTag":
				if len(args) >= 1 {
					return ent.HasTag(fmt.Sprint(args[0])), nil
				}
				return false, nil
			case "Move":
				if len(args) >= 1 {
					if delta, ok := args[0].(engine.Vec3); ok {
						ent.Transform.Position = ent.Transform.Position.Add(delta)
					}
				}
				return nil, nil
			}
			if ent == inst.Entity {
				return inst.CallMethod(mem.Member, args...), nil
			}
		}
	}

	return nil, nil
}

func evalMemberAccess(obj interface{}, member string) interface{} {
	switch v := obj.(type) {
	case *engine.Entity:
		if v == nil {
			return nil
		}
		prop, _ := v.GetProperty(member)
		return prop
	case engine.Vec3:
		switch strings.ToUpper(member) {
		case "X":
			return v.X
		case "Y":
			return v.Y
		case "Z":
			return v.Z
		}
	case engine.Vec2:
		switch strings.ToUpper(member) {
		case "X":
			return v.X
		case "Y":
			return v.Y
		}
	case engine.Color:
		switch strings.ToUpper(member) {
		case "R":
			return v.R
		case "G":
			return v.G
		case "B":
			return v.B
		case "A":
			return v.A
		}
	case []*engine.Entity:
		if member == "Count" || member == "Length" {
			return len(v)
		}
	}
	return nil
}

func evalBinaryOp(lhs interface{}, op string, rhs interface{}) interface{} {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return engine.CompareValues(lhs, op, rhs)
	case "+":
		if _, isStr := lhs.(string); isStr {
			return fmt.Sprint(lhs) + fmt.Sprint(rhs)
		}
		if _, isStr := rhs.(string); isStr {
			return fmt.Sprint(lhs) + fmt.Sprint(rhs)
		}
		if v1, ok := lhs.(engine.Vec3); ok {
			if v2, ok2 := rhs.(engine.Vec3); ok2 {
				return v1.Add(v2)
			}
		}
		if v1, ok := lhs.(engine.Vec2); ok {
			if v2, ok2 := rhs.(engine.Vec2); ok2 {
				return v1.Add(v2)
			}
		}
		f1, _ := engine.ToFloat64(lhs)
		f2, _ := engine.ToFloat64(rhs)
		return f1 + f2
	case "-":
		if v1, ok := lhs.(engine.Vec3); ok {
			if v2, ok2 := rhs.(engine.Vec3); ok2 {
				return v1.Sub(v2)
			}
		}
		if v1, ok := lhs.(engine.Vec2); ok {
			if v2, ok2 := rhs.(engine.Vec2); ok2 {
				return v1.Sub(v2)
			}
		}
		f1, _ := engine.ToFloat64(lhs)
		f2, _ := engine.ToFloat64(rhs)
		return f1 - f2
	case "*":
		if v1, ok := lhs.(engine.Vec3); ok {
			s, _ := engine.ToFloat64(rhs)
			return v1.Scale(s)
		}
		if v1, ok := lhs.(engine.Vec2); ok {
			s, _ := engine.ToFloat64(rhs)
			return v1.Scale(s)
		}
		f1, _ := engine.ToFloat64(lhs)
		f2, _ := engine.ToFloat64(rhs)
		return f1 * f2
	case "/":
		f2, _ := engine.ToFloat64(rhs)
		if f2 == 0 {
			return 0.0
		}
		if v1, ok := lhs.(engine.Vec3); ok {
			return v1.Scale(1.0 / f2)
		}
		f1, _ := engine.ToFloat64(lhs)
		return f1 / f2
	}
	return nil
}

func toBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if f, ok := engine.ToFloat64(v); ok {
		return f != 0
	}
	return v != nil
}
