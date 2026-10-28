package engine

import (
	"fmt"
	"math"
	"strings"
)

// MeshType defines the 2D or 3D visual primitive for an Entity.
type MeshType string

const (
	MeshCube     MeshType = "Cube"
	MeshSphere   MeshType = "Sphere"
	MeshCylinder MeshType = "Cylinder"
	MeshPyramid  MeshType = "Pyramid"
	MeshPlane    MeshType = "Plane"
	MeshSprite2D MeshType = "Sprite2D"
	MeshCircle2D MeshType = "Circle2D"
	MeshRing2D   MeshType = "Ring2D"
	MeshNone     MeshType = "None"
)

// Transform stores 3D/2D position, Euler rotation (degrees), and scale.
type Transform struct {
	Position Vec3 `json:"position"`
	Rotation Vec3 `json:"rotation"`
	Scale    Vec3 `json:"scale"`
}

// RenderComponent controls how an Entity is drawn in 2D and 3D viewports.
type RenderComponent struct {
	Visible   bool     `json:"visible"`
	Mesh      MeshType `json:"mesh"`
	Color     Color    `json:"color"`
	Emissive  float64  `json:"emissive"`
	Wireframe bool     `json:"wireframe"`
	Label     string   `json:"label,omitempty"`
}

// PhysicsComponent handles velocity, collision bounds, and trigger flags.
type PhysicsComponent struct {
	Enabled    bool    `json:"enabled"`
	IsStatic   bool    `json:"is_static"`
	IsTrigger  bool    `json:"is_trigger"`
	Velocity   Vec3    `json:"velocity"`
	AngularVel Vec3    `json:"angular_vel"`
	Radius     float64 `json:"radius"`
	Mass       float64 `json:"mass"`
	UseGravity bool    `json:"use_gravity"`
}

// LightComponent represents a directional or point light in 3D scenes.
type LightComponent struct {
	Enabled   bool    `json:"enabled"`
	IsPoint   bool    `json:"is_point"`
	Color     Color   `json:"color"`
	Intensity float64 `json:"intensity"`
	Range     float64 `json:"range"`
}

// ScriptAttachment binds a .osmu component script to an Entity with exported field overrides.
type ScriptAttachment struct {
	ScriptPath    string                 `json:"script_path"`
	ComponentName string                 `json:"component_name"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
}

// Entity is a node in the OSMU Scene Graph / ECS store.
type Entity struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Tag        string                 `json:"tag"`
	Tags       []string               `json:"tags,omitempty"`
	ParentID   string                 `json:"parent_id,omitempty"`
	Active     bool                   `json:"active"`
	Layer      int                    `json:"layer"`
	Health     float64                `json:"health"`
	MaxHealth  float64                `json:"max_health"`
	Speed      float64                `json:"speed"`
	Transform  Transform              `json:"transform"`
	Render     RenderComponent        `json:"render"`
	Physics    PhysicsComponent       `json:"physics"`
	Light      *LightComponent        `json:"light,omitempty"`
	Scripts    []ScriptAttachment     `json:"scripts,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// NewEntity creates a default initialized Entity.
func NewEntity(id, name, tag string, mesh MeshType, pos Vec3, col Color) *Entity {
	return &Entity{
		ID:        id,
		Name:      name,
		Tag:       tag,
		Tags:      []string{tag},
		Active:    true,
		Layer:     0,
		Health:    100.0,
		MaxHealth: 100.0,
		Speed:     5.0,
		Transform: Transform{
			Position: pos,
			Rotation: V3(0, 0, 0),
			Scale:    V3(1, 1, 1),
		},
		Render: RenderComponent{
			Visible: true,
			Mesh:    mesh,
			Color:   col,
		},
		Physics: PhysicsComponent{
			Enabled:  true,
			IsStatic: false,
			Radius:   0.6,
			Mass:     1.0,
		},
		Properties: make(map[string]interface{}),
	}
}

func (e *Entity) String() string {
	if e == nil {
		return "Entity(nil)"
	}
	return fmt.Sprintf("Entity(%s)", e.Name)
}

// HasTag checks if the entity matches a tag (case-insensitive or exact).
func (e *Entity) HasTag(tag string) bool {
	if strings.EqualFold(e.Tag, tag) {
		return true
	}
	for _, t := range e.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// WorldAABB computes the world-space axis-aligned bounding box of the entity.
func (e *Entity) WorldAABB() AABB {
	half := Vec3{
		X: math.Max(0.25, math.Abs(e.Transform.Scale.X)*0.5),
		Y: math.Max(0.25, math.Abs(e.Transform.Scale.Y)*0.5),
		Z: math.Max(0.25, math.Abs(e.Transform.Scale.Z)*0.5),
	}
	if e.Render.Mesh == MeshPlane {
		half.Y = 0.1
	}
	return AABB{
		Min: e.Transform.Position.Sub(half),
		Max: e.Transform.Position.Add(half),
	}
}

// LookAt rotates the Entity on the Y axis (3D) and Z axis (2D) to face target.
func (e *Entity) LookAt(target Vec3) {
	d := target.Sub(e.Transform.Position)
	if d.Len() < 1e-6 {
		return
	}
	yaw := math.Atan2(d.X, d.Z) * 180.0 / math.Pi
	e.Transform.Rotation.Y = yaw
	angle2D := math.Atan2(d.Y, d.X) * 180.0 / math.Pi
	e.Transform.Rotation.Z = angle2D
}

// GetProperty retrieves either a built-in field or a custom dynamic property.
func (e *Entity) GetProperty(key string) (interface{}, bool) {
	switch strings.ToLower(key) {
	case "id":
		return e.ID, true
	case "name":
		return e.Name, true
	case "tag":
		return e.Tag, true
	case "active":
		return e.Active, true
	case "layer":
		return e.Layer, true
	case "health":
		return e.Health, true
	case "maxhealth":
		return e.MaxHealth, true
	case "speed":
		return e.Speed, true
	case "position":
		return e.Transform.Position, true
	case "rotation":
		return e.Transform.Rotation, true
	case "scale":
		return e.Transform.Scale, true
	case "velocity":
		return e.Physics.Velocity, true
	case "color":
		return e.Render.Color, true
	case "label":
		return e.Render.Label, true
	case "x":
		return e.Transform.Position.X, true
	case "y":
		return e.Transform.Position.Y, true
	case "z":
		return e.Transform.Position.Z, true
	}
	if e.Properties != nil {
		if v, ok := e.Properties[key]; ok {
			return v, true
		}
		for k, v := range e.Properties {
			if strings.EqualFold(k, key) {
				return v, true
			}
		}
	}
	return nil, false
}

// SetProperty sets either a built-in field or a custom dynamic property.
func (e *Entity) SetProperty(key string, val interface{}) {
	switch strings.ToLower(key) {
	case "name":
		e.Name = fmt.Sprint(val)
		return
	case "tag":
		e.Tag = fmt.Sprint(val)
		return
	case "label":
		e.Render.Label = fmt.Sprint(val)
		return
	case "layer":
		if f, ok := ToFloat64(val); ok {
			e.Layer = int(f)
		}
		return
	case "active":
		if b, ok := val.(bool); ok {
			e.Active = b
		}
		return
	case "health":
		if f, ok := ToFloat64(val); ok {
			e.Health = f
		}
		return
	case "maxhealth":
		if f, ok := ToFloat64(val); ok {
			e.MaxHealth = f
		}
		return
	case "speed":
		if f, ok := ToFloat64(val); ok {
			e.Speed = f
		}
		return
	case "position":
		if v, ok := val.(Vec3); ok {
			e.Transform.Position = v
		}
		return
	case "rotation":
		if v, ok := val.(Vec3); ok {
			e.Transform.Rotation = v
		}
		return
	case "scale":
		if v, ok := val.(Vec3); ok {
			e.Transform.Scale = v
		}
		return
	case "velocity":
		if v, ok := val.(Vec3); ok {
			e.Physics.Velocity = v
		}
		return
	case "color":
		if c, ok := val.(Color); ok {
			e.Render.Color = c
		}
		return
	}
	if e.Properties == nil {
		e.Properties = make(map[string]interface{})
	}
	e.Properties[key] = val
}

// GetFloat returns a numeric property as float64.
func (e *Entity) GetFloat(key string) float64 {
	v, ok := e.GetProperty(key)
	if !ok {
		return 0
	}
	f, _ := ToFloat64(v)
	return f
}

// ToFloat64 converts numeric values to float64.
func ToFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
