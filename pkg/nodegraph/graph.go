package nodegraph

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"osmu/pkg/engine"
)

// NodeCategory classifies visual nodes for color-coding and menu grouping.
type NodeCategory string

const (
	CatEvent  NodeCategory = "Event"
	CatQuery  NodeCategory = "Query"
	CatAction NodeCategory = "Action"
	CatLogic  NodeCategory = "Logic"
)

// SocketKind defines the data or flow type carried by a node socket.
type SocketKind string

const (
	SocketFlow       SocketKind = "Flow"
	SocketEntityList SocketKind = "EntityList"
	SocketEntity     SocketKind = "Entity"
	SocketFloat      SocketKind = "Float"
	SocketBool       SocketKind = "Bool"
)

// Socket is an input or output pin on a visual node.
type Socket struct {
	Name string     `json:"name"`
	Kind SocketKind `json:"kind"`
}

// Node is a Blender-style visual block in the OSMU Node Graph editor.
type Node struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Title      string            `json:"title"`
	Category   NodeCategory      `json:"category"`
	X          float64           `json:"x"`
	Y          float64           `json:"y"`
	Width      float64           `json:"width"`
	Height     float64           `json:"height"`
	Inputs     []Socket          `json:"inputs"`
	Outputs    []Socket          `json:"outputs"`
	Params     map[string]string `json:"params"`
	LastFired  time.Time         `json:"-"`
	LastResult string            `json:"-"`
}

// Link connects an output socket of one Node to an input socket of another Node.
type Link struct {
	ID         string     `json:"id"`
	FromNode   string     `json:"from_node"`
	FromSocket string     `json:"from_socket"`
	ToNode     string     `json:"to_node"`
	ToSocket   string     `json:"to_socket"`
	Kind       SocketKind `json:"kind"`
	LastFired  time.Time  `json:"-"`
}

// Graph holds the visual node network and executes event/query/action flows.
type Graph struct {
	Nodes       []*Node `json:"nodes"`
	Links       []*Link `json:"links"`
	AutoGroup   bool    `json:"auto_group"`
	GroupThresh float64 `json:"group_thresh"`
}

// NewGraph creates an initialized node graph with automatic wire bundling enabled.
func NewGraph() *Graph {
	return &Graph{
		AutoGroup:   true,
		GroupThresh: 95.0,
	}
}

// NodeTemplate describes a creatable node preset for the editor palette.
type NodeTemplate struct {
	Type     string
	Title    string
	Category NodeCategory
	Inputs   []Socket
	Outputs  []Socket
	Defaults map[string]string
}

// BuiltinNodeTemplates returns the catalog of visual nodes available in the editor.
func BuiltinNodeTemplates() []NodeTemplate {
	return []NodeTemplate{
		{
			Type:     "OnStart",
			Title:    "On Scene Start",
			Category: CatEvent,
			Outputs:  []Socket{{Name: "Out", Kind: SocketFlow}},
			Defaults: map[string]string{},
		},
		{
			Type:     "OnUpdate",
			Title:    "On Frame Update",
			Category: CatEvent,
			Outputs:  []Socket{{Name: "Tick", Kind: SocketFlow}, {Name: "Dt", Kind: SocketFloat}},
			Defaults: map[string]string{"Interval": "0.0"},
		},
		{
			Type:     "OnSignal",
			Title:    "On Signal Received",
			Category: CatEvent,
			Outputs:  []Socket{{Name: "Trigger", Kind: SocketFlow}, {Name: "Source", Kind: SocketEntity}},
			Defaults: map[string]string{"Signal": "TargetLocked"},
		},
		{
			Type:     "OnCollision",
			Title:    "On Tag Collision",
			Category: CatEvent,
			Outputs:  []Socket{{Name: "Hit", Kind: SocketFlow}, {Name: "Self", Kind: SocketEntity}, {Name: "Other", Kind: SocketEntity}},
			Defaults: map[string]string{"TagA": "Player", "TagB": "Coin"},
		},
		{
			Type:     "QueryEntities",
			Title:    "Query Entities",
			Category: CatQuery,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}},
			Outputs:  []Socket{{Name: "Found", Kind: SocketFlow}, {Name: "Entities", Kind: SocketEntityList}, {Name: "Nearest", Kind: SocketEntity}},
			Defaults: map[string]string{"Query": "Tag == \"Enemy\" && Health > 0 orderby Distance asc"},
		},
		{
			Type:     "FilterLowHealth",
			Title:    "Filter By Property",
			Category: CatQuery,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "Entities", Kind: SocketEntityList}},
			Outputs:  []Socket{{Name: "Match", Kind: SocketFlow}, {Name: "Filtered", Kind: SocketEntityList}},
			Defaults: map[string]string{"Property": "Health", "Op": "<=", "Value": "50"},
		},
		{
			Type:     "EmitSignal",
			Title:    "Emit Signal",
			Category: CatAction,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "Target", Kind: SocketEntity}},
			Outputs:  []Socket{{Name: "Done", Kind: SocketFlow}},
			Defaults: map[string]string{"Signal": "AlarmTriggered", "Arg": "1"},
		},
		{
			Type:     "ModifyProperty",
			Title:    "Modify Property",
			Category: CatAction,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "Targets", Kind: SocketEntityList}},
			Outputs:  []Socket{{Name: "Done", Kind: SocketFlow}},
			Defaults: map[string]string{"TargetTag": "Player", "Property": "Score", "Mode": "Add", "Value": "10"},
		},
		{
			Type:     "MoveTowards",
			Title:    "Move Towards Target",
			Category: CatAction,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "Actors", Kind: SocketEntityList}, {Name: "Goal", Kind: SocketEntity}},
			Outputs:  []Socket{{Name: "Done", Kind: SocketFlow}},
			Defaults: map[string]string{"ActorTag": "Enemy", "GoalTag": "Player", "Speed": "3.2"},
		},
		{
			Type:     "SpawnEntity",
			Title:    "Spawn / Pulse Effect",
			Category: CatAction,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "AtEntity", Kind: SocketEntity}},
			Outputs:  []Socket{{Name: "Spawned", Kind: SocketEntity}},
			Defaults: map[string]string{"Name": "EnergyPulse", "Tag": "Effect", "Mesh": "Sphere"},
		},
		{
			Type:     "DestroyEntity",
			Title:    "Destroy Entity",
			Category: CatAction,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "Target", Kind: SocketEntity}},
			Outputs:  []Socket{{Name: "Done", Kind: SocketFlow}},
			Defaults: map[string]string{},
		},
		{
			Type:     "BranchCount",
			Title:    "Branch If Any Found",
			Category: CatLogic,
			Inputs:   []Socket{{Name: "In", Kind: SocketFlow}, {Name: "Entities", Kind: SocketEntityList}},
			Outputs:  []Socket{{Name: "NonEmpty", Kind: SocketFlow}, {Name: "Empty", Kind: SocketFlow}},
			Defaults: map[string]string{"MinCount": "1"},
		},
	}
}

// CreateNodeFromTemplate instantiates a Node from a template name at (x, y).
func CreateNodeFromTemplate(id, templateType string, x, y float64) *Node {
	for _, tpl := range BuiltinNodeTemplates() {
		if tpl.Type == templateType {
			params := make(map[string]string, len(tpl.Defaults))
			for k, v := range tpl.Defaults {
				params[k] = v
			}
			rows := len(tpl.Inputs)
			if len(tpl.Outputs) > rows {
				rows = len(tpl.Outputs)
			}
			height := 56.0 + float64(rows)*20.0 + float64(len(params))*16.0
			return &Node{
				ID:       id,
				Type:     tpl.Type,
				Title:    tpl.Title,
				Category: tpl.Category,
				X:        x,
				Y:        y,
				Width:    180.0,
				Height:   height,
				Inputs:   append([]Socket(nil), tpl.Inputs...),
				Outputs:  append([]Socket(nil), tpl.Outputs...),
				Params:   params,
			}
		}
	}
	return &Node{
		ID:       id,
		Type:     templateType,
		Title:    templateType,
		Category: CatLogic,
		X:        x,
		Y:        y,
		Width:    185,
		Height:   90,
		Params:   map[string]string{},
	}
}

// FindNode returns the node with the matching ID.
func (g *Graph) FindNode(id string) *Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// AddLink connects two sockets if not already connected.
func (g *Graph) AddLink(fromNode, fromSock, toNode, toSock string, kind SocketKind) *Link {
	if fromNode == toNode {
		return nil
	}
	for _, l := range g.Links {
		if l.FromNode == fromNode && l.FromSocket == fromSock && l.ToNode == toNode && l.ToSocket == toSock {
			return l
		}
	}
	link := &Link{
		ID:         fmt.Sprintf("link_%d", len(g.Links)+1),
		FromNode:   fromNode,
		FromSocket: fromSock,
		ToNode:     toNode,
		ToSocket:   toSock,
		Kind:       kind,
	}
	g.Links = append(g.Links, link)
	return link
}

// RemoveLinksForNode removes all links attached to nodeID.
func (g *Graph) RemoveLinksForNode(nodeID string) {
	kept := g.Links[:0]
	for _, l := range g.Links {
		if l.FromNode != nodeID && l.ToNode != nodeID {
			kept = append(kept, l)
		}
	}
	g.Links = kept
}

// SocketPos calculates the exact canvas position (x, y) of an input or output socket on a node.
func (n *Node) SocketPos(socketName string, isOutput bool) (float64, float64) {
	if isOutput {
		for i, s := range n.Outputs {
			if s.Name == socketName {
				return n.X + n.Width, n.Y + 42.0 + float64(i)*22.0
			}
		}
		return n.X + n.Width, n.Y + 42.0
	}
	for i, s := range n.Inputs {
		if s.Name == socketName {
			return n.X, n.Y + 42.0 + float64(i)*22.0
		}
	}
	return n.X, n.Y + 42.0
}

// SceneContext provides the interface that the NodeGraph runtime uses to query and mutate the scene.
type SceneContext interface {
	Entities() []*engine.Entity
	FindEntityByID(id string) *engine.Entity
	EventBus() *engine.EventBus
	DeltaTime() float64
}

// ExecuteEvent triggers all matching Event nodes in the graph and propagates flows.
func (g *Graph) ExecuteEvent(ctx SceneContext, eventType string, payload map[string]interface{}) {
	now := time.Now()
	for _, n := range g.Nodes {
		if n.Type != eventType {
			continue
		}
		switch eventType {
		case "OnStart", "OnUpdate":
			n.LastFired = now
			g.propagateFrom(ctx, n, "Out", payload, 0)
			g.propagateFrom(ctx, n, "Tick", payload, 0)
		case "OnSignal":
			sig, _ := payload["Signal"].(string)
			want := n.Params["Signal"]
			if want == "" || want == "*" || strings.EqualFold(sig, want) {
				n.LastFired = now
				n.LastResult = sig
				g.propagateFrom(ctx, n, "Trigger", payload, 0)
			}
		case "OnCollision":
			a, _ := payload["Self"].(*engine.Entity)
			b, _ := payload["Other"].(*engine.Entity)
			if a == nil || b == nil {
				continue
			}
			tagA := n.Params["TagA"]
			tagB := n.Params["TagB"]
			if (tagA == "" || a.HasTag(tagA)) && (tagB == "" || b.HasTag(tagB)) {
				n.LastFired = now
				n.LastResult = fmt.Sprintf("%s <-> %s", a.Name, b.Name)
				g.propagateFrom(ctx, n, "Hit", payload, 0)
			}
		}
	}
}

func (g *Graph) propagateFrom(ctx SceneContext, src *Node, outSocket string, state map[string]interface{}, depth int) {
	if depth > 16 {
		return
	}
	now := time.Now()
	for _, link := range g.Links {
		if link.FromNode == src.ID && (link.FromSocket == outSocket || outSocket == "*") {
			link.LastFired = now
			dst := g.FindNode(link.ToNode)
			if dst != nil {
				g.evalNode(ctx, dst, state, depth+1)
			}
		}
	}
}

func (g *Graph) evalNode(ctx SceneContext, n *Node, state map[string]interface{}, depth int) {
	now := time.Now()
	n.LastFired = now
	nextState := make(map[string]interface{}, len(state)+4)
	for k, v := range state {
		nextState[k] = v
	}

	switch n.Type {
	case "QueryEntities":
		qStr := n.Params["Query"]
		origin := engine.V3(0, 0, 0)
		if srcEnt, ok := state["Source"].(*engine.Entity); ok && srcEnt != nil {
			origin = srcEnt.Transform.Position
		}
		found, err := engine.EvalQueryString(ctx.Entities(), qStr, origin)
		if err == nil {
			nextState["Entities"] = found
			n.LastResult = fmt.Sprintf("%d matched", len(found))
			if len(found) > 0 {
				nextState["Nearest"] = found[0]
				nextState["Target"] = found[0]
			}
			g.propagateFrom(ctx, n, "Found", nextState, depth)
		}

	case "FilterLowHealth":
		prop := n.Params["Property"]
		if prop == "" {
			prop = "Health"
		}
		op := n.Params["Op"]
		if op == "" {
			op = "<="
		}
		val, _ := strconv.ParseFloat(n.Params["Value"], 64)
		list, _ := state["Entities"].([]*engine.Entity)
		if list == nil {
			list = ctx.Entities()
		}
		filtered := engine.NewQuery(list).Where(prop, op, val).All()
		nextState["Entities"] = filtered
		n.LastResult = fmt.Sprintf("%d matched", len(filtered))
		if len(filtered) > 0 {
			g.propagateFrom(ctx, n, "Match", nextState, depth)
		}

	case "BranchCount":
		minCount, _ := strconv.Atoi(n.Params["MinCount"])
		if minCount <= 0 {
			minCount = 1
		}
		list, _ := state["Entities"].([]*engine.Entity)
		n.LastResult = fmt.Sprintf("count=%d", len(list))
		if len(list) >= minCount {
			g.propagateFrom(ctx, n, "NonEmpty", nextState, depth)
		} else {
			g.propagateFrom(ctx, n, "Empty", nextState, depth)
		}

	case "EmitSignal":
		sig := n.Params["Signal"]
		arg := n.Params["Arg"]
		if sig != "" && ctx.EventBus() != nil {
			srcID := "NodeGraph"
			if ent, ok := state["Target"].(*engine.Entity); ok && ent != nil {
				srcID = ent.ID
			}
			ctx.EventBus().Emit(sig, srcID, arg)
			n.LastResult = "emit " + sig
		}
		g.propagateFrom(ctx, n, "Done", nextState, depth)

	case "ModifyProperty":
		prop := n.Params["Property"]
		mode := n.Params["Mode"]
		val, _ := strconv.ParseFloat(n.Params["Value"], 64)
		targetTag := n.Params["TargetTag"]

		var targets []*engine.Entity
		if list, ok := state["Entities"].([]*engine.Entity); ok && len(list) > 0 && targetTag == "" {
			targets = list
		} else if targetTag != "" {
			targets = engine.NewQuery(ctx.Entities()).WithTag(targetTag).All()
		} else if single, ok := state["Self"].(*engine.Entity); ok && single != nil {
			targets = []*engine.Entity{single}
		}

		for _, t := range targets {
			cur := t.GetFloat(prop)
			var updated float64
			if strings.EqualFold(mode, "Set") {
				updated = val
			} else {
				updated = cur + val
			}
			t.SetProperty(prop, updated)
		}
		n.LastResult = fmt.Sprintf("%s -> %d ent", prop, len(targets))
		g.propagateFrom(ctx, n, "Done", nextState, depth)

	case "MoveTowards":
		actorTag := n.Params["ActorTag"]
		goalTag := n.Params["GoalTag"]
		speed, err := strconv.ParseFloat(n.Params["Speed"], 64)
		if err != nil || speed == 0 {
			speed = 3.0
		}
		var actors []*engine.Entity
		if list, ok := state["Entities"].([]*engine.Entity); ok && len(list) > 0 {
			actors = list
		} else if actorTag != "" {
			actors = engine.NewQuery(ctx.Entities()).WithTag(actorTag).All()
		}
		goal := engine.NewQuery(ctx.Entities()).WithTag(goalTag).First()
		if goal != nil {
			dt := ctx.DeltaTime()
			for _, a := range actors {
				if a.ID == goal.ID {
					continue
				}
				dir := goal.Transform.Position.Sub(a.Transform.Position)
				if dir.Len() > 0.25 {
					step := dir.Norm().Scale(speed * dt)
					a.Transform.Position = a.Transform.Position.Add(step)
					a.LookAt(goal.Transform.Position)
				}
			}
			n.LastResult = fmt.Sprintf("moved %d -> %s", len(actors), goal.Name)
		}
		g.propagateFrom(ctx, n, "Done", nextState, depth)

	case "DestroyEntity":
		if other, ok := state["Other"].(*engine.Entity); ok && other != nil {
			other.Active = false
			n.LastResult = "destroyed " + other.Name
		} else if target, ok := state["Target"].(*engine.Entity); ok && target != nil {
			target.Active = false
			n.LastResult = "destroyed " + target.Name
		}
		g.propagateFrom(ctx, n, "Done", nextState, depth)
	}
}
