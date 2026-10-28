package platform

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
