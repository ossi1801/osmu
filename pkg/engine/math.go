package engine

import (
	"fmt"
	"math"
)

// Vec2 represents a 2D vector.
type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func V2(x, y float64) Vec2 { return Vec2{X: x, Y: y} }

func (v Vec2) Add(o Vec2) Vec2      { return Vec2{v.X + o.X, v.Y + o.Y} }
func (v Vec2) Sub(o Vec2) Vec2      { return Vec2{v.X - o.X, v.Y - o.Y} }
func (v Vec2) Scale(s float64) Vec2 { return Vec2{v.X * s, v.Y * s} }
func (v Vec2) Dot(o Vec2) float64   { return v.X*o.X + v.Y*o.Y }
func (v Vec2) Len() float64         { return math.Hypot(v.X, v.Y) }
func (v Vec2) Dist(o Vec2) float64  { return v.Sub(o).Len() }
func (v Vec2) Norm() Vec2 {
	l := v.Len()
	if l < 1e-9 {
		return Vec2{}
	}
	return v.Scale(1.0 / l)
}
func (v Vec2) Lerp(o Vec2, t float64) Vec2 {
	return Vec2{v.X + (o.X-v.X)*t, v.Y + (o.Y-v.Y)*t}
}
func (v Vec2) String() string {
	return fmt.Sprintf("Vec2(%.2f, %.2f)", v.X, v.Y)
}

// Vec3 represents a 3D vector.
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func V3(x, y, z float64) Vec3 { return Vec3{X: x, Y: y, Z: z} }

func (v Vec3) Add(o Vec3) Vec3      { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) Sub(o Vec3) Vec3      { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v Vec3) Mul(o Vec3) Vec3      { return Vec3{v.X * o.X, v.Y * o.Y, v.Z * o.Z} }
func (v Vec3) Scale(s float64) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }
func (v Vec3) Dot(o Vec3) float64   { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		X: v.Y*o.Z - v.Z*o.Y,
		Y: v.Z*o.X - v.X*o.Z,
		Z: v.X*o.Y - v.Y*o.X,
	}
}
func (v Vec3) Len() float64        { return math.Sqrt(v.Dot(v)) }
func (v Vec3) Dist(o Vec3) float64 { return v.Sub(o).Len() }
func (v Vec3) Norm() Vec3 {
	l := v.Len()
	if l < 1e-9 {
		return Vec3{}
	}
	return v.Scale(1.0 / l)
}
func (v Vec3) Lerp(o Vec3, t float64) Vec3 {
	return Vec3{
		X: v.X + (o.X-v.X)*t,
		Y: v.Y + (o.Y-v.Y)*t,
		Z: v.Z + (o.Z-v.Z)*t,
	}
}
func (v Vec3) XY() Vec2 { return Vec2{v.X, v.Y} }
func (v Vec3) XZ() Vec2 { return Vec2{v.X, v.Z} }
func (v Vec3) String() string {
	return fmt.Sprintf("Vec3(%.2f, %.2f, %.2f)", v.X, v.Y, v.Z)
}

// Color represents an RGBA color with components in [0, 1].
type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

func RGBA(r, g, b, a float64) Color { return Color{R: r, G: g, B: b, A: a} }
func RGB(r, g, b float64) Color     { return Color{R: r, G: g, B: b, A: 1.0} }
func HexColor(hex uint32) Color {
	return Color{
		R: float64((hex>>16)&0xFF) / 255.0,
		G: float64((hex>>8)&0xFF) / 255.0,
		B: float64(hex&0xFF) / 255.0,
		A: 1.0,
	}
}

func (c Color) WithAlpha(a float64) Color {
	return Color{R: c.R, G: c.G, B: c.B, A: a}
}

func (c Color) Scale(s float64) Color {
	return Color{
		R: Clamp(c.R*s, 0, 1),
		G: Clamp(c.G*s, 0, 1),
		B: Clamp(c.B*s, 0, 1),
		A: c.A,
	}
}

func (c Color) String() string {
	return fmt.Sprintf("Color(%.2f, %.2f, %.2f, %.2f)", c.R, c.G, c.B, c.A)
}

// Mat4 is a column-major 4x4 matrix for 3D rendering and picking.
type Mat4 [16]float64

func IdentityMat4() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

func TranslateMat4(v Vec3) Mat4 {
	m := IdentityMat4()
	m[12] = v.X
	m[13] = v.Y
	m[14] = v.Z
	return m
}

func ScaleMat4(v Vec3) Mat4 {
	m := IdentityMat4()
	m[0] = v.X
	m[5] = v.Y
	m[10] = v.Z
	return m
}

func RotateYMat4(deg float64) Mat4 {
	rad := deg * math.Pi / 180.0
	c, s := math.Cos(rad), math.Sin(rad)
	m := IdentityMat4()
	m[0] = c
	m[2] = -s
	m[8] = s
	m[10] = c
	return m
}

func RotateXMat4(deg float64) Mat4 {
	rad := deg * math.Pi / 180.0
	c, s := math.Cos(rad), math.Sin(rad)
	m := IdentityMat4()
	m[5] = c
	m[6] = s
	m[9] = -s
	m[10] = c
	return m
}

func RotateZMat4(deg float64) Mat4 {
	rad := deg * math.Pi / 180.0
	c, s := math.Cos(rad), math.Sin(rad)
	m := IdentityMat4()
	m[0] = c
	m[1] = s
	m[4] = -s
	m[5] = c
	return m
}

func (a Mat4) Mul(b Mat4) Mat4 {
	var out Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			sum := 0.0
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = sum
		}
	}
	return out
}

func (m Mat4) TransformPoint(v Vec3) (Vec3, float64) {
	x := m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12]
	y := m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13]
	z := m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14]
	w := m[3]*v.X + m[7]*v.Y + m[11]*v.Z + m[15]
	if math.Abs(w) > 1e-7 {
		return Vec3{x / w, y / w, z / w}, w
	}
	return Vec3{x, y, z}, w
}

func PerspectiveMat4(fovDeg, aspect, near, far float64) Mat4 {
	f := 1.0 / math.Tan((fovDeg*math.Pi/180.0)/2.0)
	var m Mat4
	m[0] = f / aspect
	m[5] = f
	m[10] = (far + near) / (near - far)
	m[11] = -1.0
	m[14] = (2.0 * far * near) / (near - far)
	return m
}

func LookAtMat4(eye, center, up Vec3) Mat4 {
	f := center.Sub(eye).Norm()
	s := f.Cross(up).Norm()
	u := s.Cross(f)

	m := IdentityMat4()
	m[0] = s.X
	m[4] = s.Y
	m[8] = s.Z
	m[1] = u.X
	m[5] = u.Y
	m[9] = u.Z
	m[2] = -f.X
	m[6] = -f.Y
	m[10] = -f.Z
	m[12] = -s.Dot(eye)
	m[13] = -u.Dot(eye)
	m[14] = f.Dot(eye)
	return m
}

// AABB represents an axis-aligned bounding box in 3D.
type AABB struct {
	Min Vec3
	Max Vec3
}

func (b AABB) Intersects(o AABB) bool {
	return b.Min.X <= o.Max.X && b.Max.X >= o.Min.X &&
		b.Min.Y <= o.Max.Y && b.Max.Y >= o.Min.Y &&
		b.Min.Z <= o.Max.Z && b.Max.Z >= o.Min.Z
}

// Ray represents a 3D ray with Origin and normalized Direction.
type Ray struct {
	Origin Vec3
	Dir    Vec3
}

// IntersectAABB returns (hit, distance) for ray-AABB intersection.
func (r Ray) IntersectAABB(box AABB) (bool, float64) {
	tmin := -math.MaxFloat64
	tmax := math.MaxFloat64

	axesOrig := [3]float64{r.Origin.X, r.Origin.Y, r.Origin.Z}
	axesDir := [3]float64{r.Dir.X, r.Dir.Y, r.Dir.Z}
	axesMin := [3]float64{box.Min.X, box.Min.Y, box.Min.Z}
	axesMax := [3]float64{box.Max.X, box.Max.Y, box.Max.Z}

	for i := 0; i < 3; i++ {
		if math.Abs(axesDir[i]) < 1e-8 {
			if axesOrig[i] < axesMin[i] || axesOrig[i] > axesMax[i] {
				return false, 0
			}
		} else {
			t1 := (axesMin[i] - axesOrig[i]) / axesDir[i]
			t2 := (axesMax[i] - axesOrig[i]) / axesDir[i]
			if t1 > t2 {
				t1, t2 = t2, t1
			}
			if t1 > tmin {
				tmin = t1
			}
			if t2 < tmax {
				tmax = t2
			}
			if tmin > tmax {
				return false, 0
			}
		}
	}
	if tmax < 0 {
		return false, 0
	}
	if tmin < 0 {
		return true, tmax
	}
	return true, tmin
}

// IntersectPlaneY intersects a ray with horizontal plane Y = planeY.
func (r Ray) IntersectPlaneY(planeY float64) (Vec3, bool) {
	if math.Abs(r.Dir.Y) < 1e-7 {
		return Vec3{}, false
	}
	t := (planeY - r.Origin.Y) / r.Dir.Y
	if t < 0 {
		return Vec3{}, false
	}
	return r.Origin.Add(r.Dir.Scale(t)), true
}

func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
