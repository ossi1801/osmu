package platform

import "math"

// MouseButton identifies mouse buttons.
type MouseButton int

const (
	MouseLeft   MouseButton = 1
	MouseMiddle MouseButton = 2
	MouseRight  MouseButton = 3
)

// InputState captures frame-by-frame mouse, keyboard, and window dimensions.
type InputState struct {
	Width         int
	Height        int
	MouseX        float64
	MouseY        float64
	MouseDX       float64
	MouseDY       float64
	ScrollY       float64
	MouseDown     [4]bool
	MouseClicked  [4]bool
	MouseReleased [4]bool
	KeysDown      map[string]bool
	KeysPressed   map[string]bool
	TypedText     string
	ShouldClose   bool
	IsMaximized   bool
}

// NewInputState creates an initialized InputState.
func NewInputState(w, h int) *InputState {
	return &InputState{
		Width:       w,
		Height:      h,
		KeysDown:    make(map[string]bool),
		KeysPressed: make(map[string]bool),
	}
}

// ResetFrameDeltas clears per-frame edge triggers (clicks, scroll, deltas, typed text).
func (in *InputState) ResetFrameDeltas() {
	in.MouseDX = 0
	in.MouseDY = 0
	in.ScrollY = 0
	in.MouseClicked = [4]bool{}
	in.MouseReleased = [4]bool{}
	for k := range in.KeysPressed {
		delete(in.KeysPressed, k)
	}
	in.TypedText = ""
}

// Window abstracts a cross-platform OpenGL window (Linux X11/GLX, Windows Win32/WGL, macOS).
type Window interface {
	PollEvents(in *InputState)
	SwapBuffers()
	Close()
	Minimize()
	ToggleMaximize()
	SetTitle(title string)
	DrawOps(width, height int, ops []DrawCmd)
}

// CmdKind identifies a hardware-accelerated OpenGL drawing primitive.
type CmdKind int

const (
	CmdClear CmdKind = iota
	CmdRect
	CmdRectOutline
	CmdLine
	CmdPolyLine
	CmdCircle
	CmdTriangle
	CmdText
)

// DrawCmd is a platform-agnostic draw command executed by the native OpenGL renderer.
type DrawCmd struct {
	Kind       CmdKind
	X, Y       float64
	W, H       float64
	X2, Y2     float64
	X3, Y3     float64
	Radius     float64
	Thickness  float64
	Points     [][2]float64
	R, G, B, A float64
	Text       string
	Scale      float64
}

var circleCos [21]float32
var circleSin [21]float32

func init() {
	for i := 0; i <= 20; i++ {
		ang := float64(i) * 2.0 * math.Pi / 20.0
		circleCos[i] = float32(math.Cos(ang))
		circleSin[i] = float32(math.Sin(ang))
	}
}

// TessellateDrawCmds converts a slice of DrawCmds into a flat interleaved triangle vertex buffer
// with 6 float32s per vertex: [x, y, r, g, b, a].
// If ndc is true, vertex coordinates are transformed to OpenGL Normalized Device Coordinates [-1, 1].
func TessellateDrawCmds(width, height int, ops []DrawCmd, dst []float32, ndc bool) []float32 {
	dst = dst[:0]
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 760
	}
	fw := float32(width)
	fh := float32(height)

	pushQuad := func(x, y, w, h, r, g, b, a float32) {
		x2 := x + w
		y2 := y + h
		if ndc {
			x = (x/fw)*2.0 - 1.0
			x2 = (x2/fw)*2.0 - 1.0
			y = 1.0 - (y/fh)*2.0
			y2 = 1.0 - (y2/fh)*2.0
		}
		dst = append(dst,
			x, y, r, g, b, a,
			x2, y, r, g, b, a,
			x2, y2, r, g, b, a,
			x, y, r, g, b, a,
			x2, y2, r, g, b, a,
			x, y2, r, g, b, a,
		)
	}

	pushTri := func(x1, y1, x2, y2, x3, y3, r, g, b, a float32) {
		if ndc {
			x1 = (x1/fw)*2.0 - 1.0
			y1 = 1.0 - (y1/fh)*2.0
			x2 = (x2/fw)*2.0 - 1.0
			y2 = 1.0 - (y2/fh)*2.0
			x3 = (x3/fw)*2.0 - 1.0
			y3 = 1.0 - (y3/fh)*2.0
		}
		dst = append(dst,
			x1, y1, r, g, b, a,
			x2, y2, r, g, b, a,
			x3, y3, r, g, b, a,
		)
	}

	pushLine := func(x1, y1, x2, y2, th, r, g, b, a float32) {
		dx := x2 - x1
		dy := y2 - y1
		length := float32(math.Hypot(float64(dx), float64(dy)))
		if length < 0.001 {
			return
		}
		half := th * 0.5
		nx := (-dy / length) * half
		ny := (dx / length) * half
		ax, ay := x1+nx, y1+ny
		bx, by := x2+nx, y2+ny
		cx, cy := x2-nx, y2-ny
		dxp, dyp := x1-nx, y1-ny
		pushTri(ax, ay, bx, by, cx, cy, r, g, b, a)
		pushTri(ax, ay, cx, cy, dxp, dyp, r, g, b, a)
	}

	for _, cmd := range ops {
		r := float32(cmd.R)
		g := float32(cmd.G)
		b := float32(cmd.B)
		a := float32(cmd.A)

		switch cmd.Kind {
		case CmdClear:
			dst = dst[:0]
			pushQuad(0, 0, fw, fh, r, g, b, 1.0)
		case CmdRect:
			pushQuad(float32(cmd.X), float32(cmd.Y), float32(cmd.W), float32(cmd.H), r, g, b, a)
		case CmdTriangle:
			pushTri(
				float32(cmd.X), float32(cmd.Y),
				float32(cmd.X2), float32(cmd.Y2),
				float32(cmd.X3), float32(cmd.Y3),
				r, g, b, a,
			)
		case CmdRectOutline:
			th := float32(cmd.Thickness)
			if th <= 0 {
				th = 1.0
			}
			x := float32(cmd.X)
			y := float32(cmd.Y)
			w := float32(cmd.W)
			h := float32(cmd.H)
			pushQuad(x, y, w, th, r, g, b, a)
			pushQuad(x, y+h-th, w, th, r, g, b, a)
			pushQuad(x, y+th, th, h-2*th, r, g, b, a)
			pushQuad(x+w-th, y+th, th, h-2*th, r, g, b, a)
		case CmdLine:
			th := float32(cmd.Thickness)
			if th <= 0 {
				th = 1.5
			}
			pushLine(float32(cmd.X), float32(cmd.Y), float32(cmd.X2), float32(cmd.Y2), th, r, g, b, a)
		case CmdPolyLine:
			th := float32(cmd.Thickness)
			if th <= 0 {
				th = 2.0
			}
			for i := 0; i < len(cmd.Points)-1; i++ {
				p1 := cmd.Points[i]
				p2 := cmd.Points[i+1]
				pushLine(float32(p1[0]), float32(p1[1]), float32(p2[0]), float32(p2[1]), th, r, g, b, a)
			}
		case CmdCircle:
			cx := float32(cmd.X)
			cy := float32(cmd.Y)
			rad := float32(cmd.Radius)
			for i := 0; i < 20; i++ {
				x1 := cx + circleCos[i]*rad
				y1 := cy + circleSin[i]*rad
				x2 := cx + circleCos[i+1]*rad
				y2 := cy + circleSin[i+1]*rad
				pushTri(cx, cy, x1, y1, x2, y2, r, g, b, a)
			}
		case CmdText:
			scale := float32(cmd.Scale)
			if scale <= 0 {
				scale = 1.5
			}
			cx := float32(cmd.X)
			cy := float32(cmd.Y)
			step := 6.0 * scale
			for i := 0; i < len(cmd.Text); i++ {
				ch := cmd.Text[i]
				if ch == ' ' {
					cx += step
					continue
				}
				glyph, ok := Font5x7[ch]
				if !ok {
					glyph = Font5x7['?']
				}
				for col := 0; col < 5; col++ {
					colBits := glyph[col]
					if colBits == 0 {
						continue
					}
					px := cx + float32(col)*scale
					row := 0
					for row < 7 {
						if (colBits & (1 << row)) == 0 {
							row++
							continue
						}
						startRow := row
						for row < 7 && (colBits&(1<<row)) != 0 {
							row++
						}
						py := cy + float32(startRow)*scale
						ph := float32(row-startRow) * scale
						pushQuad(px, py, scale, ph, r, g, b, a)
					}
				}
				cx += step
			}
		}
	}
	return dst
}
