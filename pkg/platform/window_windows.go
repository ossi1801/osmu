//go:build windows

package platform

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	opengl32 = syscall.NewLazyDLL("opengl32.dll")

	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procShowWindow        = user32.NewProc("ShowWindow")
	procSetWindowTextW    = user32.NewProc("SetWindowTextW")
	procPeekMessageW      = user32.NewProc("PeekMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procGetDC             = user32.NewProc("GetDC")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procLoadCursorW       = user32.NewProc("LoadCursorW")
	procGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
	procChoosePixelFormat = gdi32.NewProc("ChoosePixelFormat")
	procSetPixelFormat    = gdi32.NewProc("SetPixelFormat")
	procSwapBuffers       = gdi32.NewProc("SwapBuffers")

	procWglCreateContext = opengl32.NewProc("wglCreateContext")
	procWglMakeCurrent   = opengl32.NewProc("wglMakeCurrent")
	procWglDeleteContext = opengl32.NewProc("wglDeleteContext")
	procGlViewport       = opengl32.NewProc("glViewport")
	procGlMatrixMode     = opengl32.NewProc("glMatrixMode")
	procGlLoadIdentity   = opengl32.NewProc("glLoadIdentity")
	procGlOrtho          = opengl32.NewProc("glOrtho")
	procGlClearColor     = opengl32.NewProc("glClearColor")
	procGlClear          = opengl32.NewProc("glClear")
	procGlEnable         = opengl32.NewProc("glEnable")
	procGlBlendFunc      = opengl32.NewProc("glBlendFunc")
	procGlBegin          = opengl32.NewProc("glBegin")
	procGlEnd            = opengl32.NewProc("glEnd")
	procGlColor4f        = opengl32.NewProc("glColor4f")
	procGlVertex2f       = opengl32.NewProc("glVertex2f")
	procGlLineWidth      = opengl32.NewProc("glLineWidth")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	swMaximize         = 3
	swShow             = 5
	swMinimize         = 6
	swRestore          = 9
	pmRemove           = 0x0001
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmClose            = 0x0010
	wmKeyDown          = 0x0100
	wmKeyUp            = 0x0101
	wmChar             = 0x0102
	wmMouseMove        = 0x0200
	wmLButtonDown      = 0x0201
	wmLButtonUp        = 0x0202
	wmRButtonDown      = 0x0204
	wmRButtonUp        = 0x0205
	wmMButtonDown      = 0x0207
	wmMButtonUp        = 0x0208
	wmMouseWheel       = 0x020A

	pfdDrawToWindow  = 0x00000004
	pfdSupportOpenGL = 0x00000020
	pfdDoubleBuffer  = 0x00000001

	glColorBufferBit   = 0x00004000
	glDepthBufferBit   = 0x00000100
	glProjection       = 0x1701
	glModelView        = 0x1700
	glBlend            = 0x0BE2
	glSrcAlpha         = 0x0302
	glOneMinusSrcAlpha = 0x0303
	glLines            = 0x0001
	glLineLoop         = 0x0002
	glTriangles        = 0x0004
	glTriangleFan      = 0x0006
	glQuads            = 0x0007
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type pixelFormatDescriptor struct {
	nSize           uint16
	nVersion        uint16
	dwFlags         uint32
	iPixelType      byte
	cColorBits      byte
	cRedBits        byte
	cRedShift       byte
	cGreenBits      byte
	cGreenShift     byte
	cBlueBits       byte
	cBlueShift      byte
	cAlphaBits      byte
	cAlphaShift     byte
	cAccumBits      byte
	cAccumRedBits   byte
	cAccumGreenBits byte
	cAccumBlueBits  byte
	cAccumAlphaBits byte
	cDepthBits      byte
	cStencilBits    byte
	cAuxBuffers     byte
	iLayerType      byte
	bReserved       byte
	dwLayerMask     uint32
	dwVisibleMask   uint32
	dwDamageMask    uint32
}

type msgWin32 struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

var activeWinInput *InputState

type win32Window struct {
	hwnd      uintptr
	hdc       uintptr
	hglrc     uintptr
	maximized bool
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	in := activeWinInput
	if in != nil {
		switch msg {
		case wmClose, wmDestroy:
			in.ShouldClose = true
			return 0
		case wmSize:
			w := int(lParam & 0xFFFF)
			h := int((lParam >> 16) & 0xFFFF)
			if w > 0 && h > 0 {
				in.Width = w
				in.Height = h
			}
		case wmMouseMove:
			nx := float64(int16(lParam & 0xFFFF))
			ny := float64(int16((lParam >> 16) & 0xFFFF))
			in.MouseDX += nx - in.MouseX
			in.MouseDY += ny - in.MouseY
			in.MouseX = nx
			in.MouseY = ny
		case wmLButtonDown:
			in.MouseDown[1] = true
			in.MouseClicked[1] = true
		case wmLButtonUp:
			in.MouseDown[1] = false
			in.MouseReleased[1] = true
		case wmMButtonDown:
			in.MouseDown[2] = true
			in.MouseClicked[2] = true
		case wmMButtonUp:
			in.MouseDown[2] = false
			in.MouseReleased[2] = true
		case wmRButtonDown:
			in.MouseDown[3] = true
			in.MouseClicked[3] = true
		case wmRButtonUp:
			in.MouseDown[3] = false
			in.MouseReleased[3] = true
		case wmMouseWheel:
			delta := int16((wParam >> 16) & 0xFFFF)
			if delta > 0 {
				in.ScrollY += 1.0
			} else if delta < 0 {
				in.ScrollY -= 1.0
			}
		case wmChar:
			if wParam >= 32 && wParam <= 126 {
				in.TypedText += string(rune(wParam))
			}
		case wmKeyDown:
			k := vkToName(wParam)
			if k != "" {
				in.KeysDown[k] = true
				in.KeysPressed[k] = true
			}
		case wmKeyUp:
			k := vkToName(wParam)
			if k != "" {
				in.KeysDown[k] = false
			}
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func vkToName(vk uintptr) string {
	switch vk {
	case 0x08:
		return "Backspace"
	case 0x0D:
		return "Enter"
	case 0x1B:
		return "Escape"
	case 0x20:
		return "Space"
	case 0x2E:
		return "Delete"
	case 0x25:
		return "Left"
	case 0x26:
		return "Up"
	case 0x27:
		return "Right"
	case 0x28:
		return "Down"
	default:
		if vk >= 'A' && vk <= 'Z' {
			return string(rune(vk))
		}
		return ""
	}
}

// NewWindow creates a native Windows Win32 + WGL OpenGL window.
func NewWindow(title string, width, height int) (Window, error) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16PtrFromString("OSMUWindowClass")
	winTitle, _ := syscall.UTF16PtrFromString(title)
	cursor, _, _ := procLoadCursorW.Call(0, 32512)

	wcx := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		style:         0x0023,
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInst,
		hCursor:       cursor,
		lpszClassName: clsName,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcx)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(winTitle)),
		wsOverlappedWindow|wsVisible,
		80, 60, uintptr(width), uintptr(height),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("failed to create Win32 window")
	}

	hdc, _, _ := procGetDC.Call(hwnd)
	pfd := pixelFormatDescriptor{
		nSize:      uint16(unsafe.Sizeof(pixelFormatDescriptor{})),
		nVersion:   1,
		dwFlags:    pfdDrawToWindow | pfdSupportOpenGL | pfdDoubleBuffer,
		iPixelType: 0,
		cColorBits: 32,
		cDepthBits: 24,
	}
	pf, _, _ := procChoosePixelFormat.Call(hdc, uintptr(unsafe.Pointer(&pfd)))
	procSetPixelFormat.Call(hdc, pf, uintptr(unsafe.Pointer(&pfd)))
	hglrc, _, _ := procWglCreateContext.Call(hdc)
	procWglMakeCurrent.Call(hdc, hglrc)
	procShowWindow.Call(hwnd, swShow)

	return &win32Window{hwnd: hwnd, hdc: hdc, hglrc: hglrc}, nil
}

func (w *win32Window) Minimize() {
	procShowWindow.Call(w.hwnd, swMinimize)
}

func (w *win32Window) ToggleMaximize() {
	w.maximized = !w.maximized
	if w.maximized {
		procShowWindow.Call(w.hwnd, swMaximize)
	} else {
		procShowWindow.Call(w.hwnd, swRestore)
	}
}

func (w *win32Window) SetTitle(title string) {
	t, _ := syscall.UTF16PtrFromString(title)
	procSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(t)))
}

func f32bits(f float64) uintptr {
	return uintptr(math.Float32bits(float32(f)))
}

func f64bits(f float64) (uintptr, uintptr) {
	u := math.Float64bits(f)
	return uintptr(u & 0xFFFFFFFF), uintptr(u >> 32)
}

func (w *win32Window) PollEvents(in *InputState) {
	in.IsMaximized = w.maximized
	activeWinInput = in
	var m msgWin32
	for {
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (w *win32Window) DrawOps(width, height int, ops []DrawCmd) {
	procGlViewport.Call(0, 0, uintptr(width), uintptr(height))
	procGlMatrixMode.Call(glProjection)
	procGlLoadIdentity.Call()
	l0, h0 := f64bits(0)
	lw, hw := f64bits(float64(width))
	lh, hh := f64bits(float64(height))
	ln, hn := f64bits(-1)
	lf, hf := f64bits(1)
	procGlOrtho.Call(l0, h0, lw, hw, lh, hh, l0, h0, ln, hn, lf, hf)
	procGlMatrixMode.Call(glModelView)
	procGlLoadIdentity.Call()
	procGlEnable.Call(glBlend)
	procGlBlendFunc.Call(glSrcAlpha, glOneMinusSrcAlpha)

	for _, cmd := range ops {
		switch cmd.Kind {
		case CmdClear:
			procGlClearColor.Call(f32bits(cmd.R), f32bits(cmd.G), f32bits(cmd.B), f32bits(cmd.A))
			procGlClear.Call(glColorBufferBit | glDepthBufferBit)
		case CmdRect:
			winDrawRect(cmd.X, cmd.Y, cmd.W, cmd.H, cmd.R, cmd.G, cmd.B, cmd.A)
		case CmdTriangle:
			procGlColor4f.Call(f32bits(cmd.R), f32bits(cmd.G), f32bits(cmd.B), f32bits(cmd.A))
			procGlBegin.Call(glTriangles)
			procGlVertex2f.Call(f32bits(cmd.X), f32bits(cmd.Y))
			procGlVertex2f.Call(f32bits(cmd.X2), f32bits(cmd.Y2))
			procGlVertex2f.Call(f32bits(cmd.X3), f32bits(cmd.Y3))
			procGlEnd.Call()
		case CmdRectOutline:
			procGlLineWidth.Call(f32bits(math.Max(1.0, cmd.Thickness)))
			procGlColor4f.Call(f32bits(cmd.R), f32bits(cmd.G), f32bits(cmd.B), f32bits(cmd.A))
			procGlBegin.Call(glLineLoop)
			procGlVertex2f.Call(f32bits(cmd.X), f32bits(cmd.Y))
			procGlVertex2f.Call(f32bits(cmd.X+cmd.W), f32bits(cmd.Y))
			procGlVertex2f.Call(f32bits(cmd.X+cmd.W), f32bits(cmd.Y+cmd.H))
			procGlVertex2f.Call(f32bits(cmd.X), f32bits(cmd.Y+cmd.H))
			procGlEnd.Call()
		case CmdLine:
			procGlLineWidth.Call(f32bits(math.Max(1.5, cmd.Thickness)))
			procGlColor4f.Call(f32bits(cmd.R), f32bits(cmd.G), f32bits(cmd.B), f32bits(cmd.A))
			procGlBegin.Call(glLines)
			procGlVertex2f.Call(f32bits(cmd.X), f32bits(cmd.Y))
			procGlVertex2f.Call(f32bits(cmd.X2), f32bits(cmd.Y2))
			procGlEnd.Call()
		case CmdPolyLine:
			procGlLineWidth.Call(f32bits(math.Max(1.5, cmd.Thickness)))
			procGlColor4f.Call(f32bits(cmd.R), f32bits(cmd.G), f32bits(cmd.B), f32bits(cmd.A))
			procGlBegin.Call(glLines)
			for i := 0; i < len(cmd.Points)-1; i++ {
				p1 := cmd.Points[i]
				p2 := cmd.Points[i+1]
				procGlVertex2f.Call(f32bits(p1[0]), f32bits(p1[1]))
				procGlVertex2f.Call(f32bits(p2[0]), f32bits(p2[1]))
			}
			procGlEnd.Call()
		case CmdCircle:
			procGlColor4f.Call(f32bits(cmd.R), f32bits(cmd.G), f32bits(cmd.B), f32bits(cmd.A))
			procGlBegin.Call(glTriangleFan)
			procGlVertex2f.Call(f32bits(cmd.X), f32bits(cmd.Y))
			for i := 0; i <= 20; i++ {
				ang := float64(i) * 2 * math.Pi / 20.0
				procGlVertex2f.Call(f32bits(cmd.X+math.Cos(ang)*cmd.Radius), f32bits(cmd.Y+math.Sin(ang)*cmd.Radius))
			}
			procGlEnd.Call()
		case CmdText:
			scale := cmd.Scale
			if scale <= 0 {
				scale = 1.5
			}
			cx := cmd.X
			cy := cmd.Y
			for i := 0; i < len(cmd.Text); i++ {
				glyph, ok := Font5x7[cmd.Text[i]]
				if !ok {
					glyph = Font5x7['?']
				}
				for col := 0; col < 5; col++ {
					bits := glyph[col]
					if bits == 0 {
						continue
					}
					for row := 0; row < 7; row++ {
						if (bits & (1 << row)) != 0 {
							winDrawRect(cx+float64(col)*scale, cy+float64(row)*scale, scale, scale, cmd.R, cmd.G, cmd.B, cmd.A)
						}
					}
				}
				cx += 6.0 * scale
			}
		}
	}
}

func winDrawRect(x, y, w, h, r, g, b, a float64) {
	procGlColor4f.Call(f32bits(r), f32bits(g), f32bits(b), f32bits(a))
	procGlBegin.Call(glQuads)
	procGlVertex2f.Call(f32bits(x), f32bits(y))
	procGlVertex2f.Call(f32bits(x+w), f32bits(y))
	procGlVertex2f.Call(f32bits(x+w), f32bits(y+h))
	procGlVertex2f.Call(f32bits(x), f32bits(y+h))
	procGlEnd.Call()
}

func (w *win32Window) SwapBuffers() {
	procSwapBuffers.Call(w.hdc)
}

func (w *win32Window) Close() {
	procWglMakeCurrent.Call(0, 0)
	procWglDeleteContext.Call(w.hglrc)
	procDestroyWindow.Call(w.hwnd)
}
