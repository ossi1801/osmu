package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// QueryBuilder provides a minimal, fluent API for querying scene entities.
type QueryBuilder struct {
	entities   []*Entity
	predicates []func(*Entity) bool
	sortFn     func(a, b *Entity) bool
	limit      int
}

// NewQuery starts a query over a slice of entities.
func NewQuery(entities []*Entity) *QueryBuilder {
	active := make([]*Entity, 0, len(entities))
	for _, e := range entities {
		if e != nil && e.Active {
			active = append(active, e)
		}
	}
	return &QueryBuilder{
		entities: active,
	}
}

// IncludeInactive includes inactive entities in the query.
func (q *QueryBuilder) IncludeInactive(all []*Entity) *QueryBuilder {
	q.entities = all
	return q
}

// WithTag filters entities matching the specified tag.
func (q *QueryBuilder) WithTag(tag string) *QueryBuilder {
	if tag == "" || tag == "*" {
		return q
	}
	q.predicates = append(q.predicates, func(e *Entity) bool {
		return e.HasTag(tag)
	})
	return q
}

// NameContains filters entities whose name contains substr (case-insensitive).
func (q *QueryBuilder) NameContains(substr string) *QueryBuilder {
	sub := strings.ToLower(strings.TrimSpace(substr))
	if sub == "" {
		return q
	}
	q.predicates = append(q.predicates, func(e *Entity) bool {
		return strings.Contains(strings.ToLower(e.Name), sub)
	})
	return q
}

// WithinRadius filters entities within radius of origin.
func (q *QueryBuilder) WithinRadius(origin Vec3, radius float64) *QueryBuilder {
	if radius <= 0 {
		return q
	}
	q.predicates = append(q.predicates, func(e *Entity) bool {
		return e.Transform.Position.Dist(origin) <= radius
	})
	return q
}

// Where filters entities by comparing a property against a target value using op ("==", "!=", "<", "<=", ">", ">=").
func (q *QueryBuilder) Where(prop string, op string, target interface{}) *QueryBuilder {
	q.predicates = append(q.predicates, func(e *Entity) bool {
		val, ok := e.GetProperty(prop)
		if !ok {
			return false
		}
		return CompareValues(val, op, target)
	})
	return q
}

// Custom adds a custom predicate function.
func (q *QueryBuilder) Custom(fn func(*Entity) bool) *QueryBuilder {
	q.predicates = append(q.predicates, fn)
	return q
}

// OrderByDistance sorts matching entities by distance from origin.
func (q *QueryBuilder) OrderByDistance(origin Vec3, asc bool) *QueryBuilder {
	q.sortFn = func(a, b *Entity) bool {
		da := a.Transform.Position.Dist(origin)
		db := b.Transform.Position.Dist(origin)
		if asc {
			return da < db
		}
		return da > db
	}
	return q
}

// OrderByProperty sorts matching entities by a numeric or string property.
func (q *QueryBuilder) OrderByProperty(prop string, asc bool) *QueryBuilder {
	q.sortFn = func(a, b *Entity) bool {
		va, _ := a.GetProperty(prop)
		vb, _ := b.GetProperty(prop)
		fa, oka := ToFloat64(va)
		fb, okb := ToFloat64(vb)
		if oka && okb {
			if asc {
				return fa < fb
			}
			return fa > fb
		}
		sa := fmt.Sprint(va)
		sb := fmt.Sprint(vb)
		if asc {
			return sa < sb
		}
		return sa > sb
	}
	return q
}

// Limit caps the maximum number of returned entities.
func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limit = n
	return q
}

// All executes the query and returns all matching entities.
func (q *QueryBuilder) All() []*Entity {
	out := make([]*Entity, 0, len(q.entities))
	for _, e := range q.entities {
		if e == nil {
			continue
		}
		match := true
		for _, pred := range q.predicates {
			if !pred(e) {
				match = false
				break
			}
		}
		if match {
			out = append(out, e)
		}
	}
	if q.sortFn != nil {
		sort.SliceStable(out, func(i, j int) bool {
			return q.sortFn(out[i], out[j])
		})
	}
	if q.limit > 0 && len(out) > q.limit {
		out = out[:q.limit]
	}
	return out
}

// First returns the first matching entity or nil.
func (q *QueryBuilder) First() *Entity {
	res := q.Limit(1).All()
	if len(res) == 0 {
		return nil
	}
	return res[0]
}

// Count returns the number of matching entities.
func (q *QueryBuilder) Count() int {
	return len(q.All())
}

// CompareValues compares two arbitrary values using standard comparison operators.
func CompareValues(lhs interface{}, op string, rhs interface{}) bool {
	lf, lok := ToFloat64(lhs)
	rf, rok := ToFloat64(rhs)
	if lok && rok {
		switch op {
		case "==", "=":
			return lf == rf
		case "!=":
			return lf != rf
		case "<":
			return lf < rf
		case "<=":
			return lf <= rf
		case ">":
			return lf > rf
		case ">=":
			return lf >= rf
		}
	}
	ls := strings.Trim(fmt.Sprint(lhs), "\"'")
	rs := strings.Trim(fmt.Sprint(rhs), "\"'")
	switch op {
	case "==", "=":
		return strings.EqualFold(ls, rs)
	case "!=":
		return !strings.EqualFold(ls, rs)
	case "contains", "~=":
		return strings.Contains(strings.ToLower(ls), strings.ToLower(rs))
	}
	return false
}

// EvalQueryString parses and executes a human-friendly query string against entities.
// Examples:
//   - `Tag == "Enemy" && Health < 80 orderby Distance asc`
//   - `where Tag == "Coin" && Distance <= 10`
//   - `Enemy` (shorthand tag/name search)
func EvalQueryString(entities []*Entity, queryStr string, origin Vec3) ([]*Entity, error) {
	q := NewQuery(entities)
	raw := strings.TrimSpace(queryStr)
	if raw == "" || strings.EqualFold(raw, "all") || raw == "*" {
		return q.All(), nil
	}

	// Strip optional leading "select Entities" / "query Entities" / "where"
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"query entities", "select entities", "from entities"} {
		if strings.HasPrefix(lower, prefix) {
			raw = strings.TrimSpace(raw[len(prefix):])
			lower = strings.ToLower(raw)
		}
	}
	if strings.HasPrefix(lower, "where ") {
		raw = strings.TrimSpace(raw[6:])
		lower = strings.ToLower(raw)
	}

	// Split optional "orderby ..." clause
	var orderClause string
	if idx := strings.Index(lower, "orderby "); idx != -1 {
		orderClause = strings.TrimSpace(raw[idx+8:])
		raw = strings.TrimSpace(raw[:idx])
	}

	if raw != "" {
		// Split by && or "and"
		parts := splitConjunctions(raw)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			prop, op, valStr, ok := parseCondition(part)
			if !ok {
				// Treat single word as Tag or Name match
				word := strings.Trim(part, "\"'")
				q.Custom(func(e *Entity) bool {
					return e.HasTag(word) || strings.Contains(strings.ToLower(e.Name), strings.ToLower(word))
				})
				continue
			}
			val := parseLiteralValue(valStr)
			if strings.EqualFold(prop, "distance") || strings.HasPrefix(strings.ToLower(prop), "distanceto") {
				distTarget, ok := ToFloat64(val)
				if !ok {
					return nil, fmt.Errorf("distance comparison expects numeric value, got %q", valStr)
				}
				q.Custom(func(e *Entity) bool {
					d := e.Transform.Position.Dist(origin)
					return CompareValues(d, op, distTarget)
				})
			} else if strings.EqualFold(prop, "tag") && (op == "==" || op == "=") {
				q.WithTag(fmt.Sprint(val))
			} else {
				q.Where(prop, op, val)
			}
		}
	}

	if orderClause != "" {
		fields := strings.Fields(orderClause)
		if len(fields) > 0 {
			prop := fields[0]
			asc := true
			if len(fields) > 1 && strings.EqualFold(fields[1], "desc") {
				asc = false
			}
			if strings.EqualFold(prop, "distance") || strings.HasPrefix(strings.ToLower(prop), "distanceto") {
				q.OrderByDistance(origin, asc)
			} else {
				q.OrderByProperty(prop, asc)
			}
		}
	}

	return q.All(), nil
}

func splitConjunctions(s string) []string {
	replaced := strings.ReplaceAll(s, " AND ", " && ")
	replaced = strings.ReplaceAll(replaced, " and ", " && ")
	return strings.Split(replaced, "&&")
}

func parseCondition(cond string) (prop, op, val string, ok bool) {
	ops := []string{"==", "!=", "<=", ">=", "~=", "<", ">", "="}
	for _, candidate := range ops {
		if idx := strings.Index(cond, candidate); idx != -1 {
			prop = strings.TrimSpace(cond[:idx])
			op = candidate
			val = strings.TrimSpace(cond[idx+len(candidate):])
			if prop != "" && val != "" {
				return prop, op, val, true
			}
		}
	}
	return "", "", "", false
}

func parseLiteralValue(s string) interface{} {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		return s[1 : len(s)-1]
	}
	if strings.EqualFold(s, "true") {
		return true
	}
	if strings.EqualFold(s, "false") {
		return false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
