//go:build linux

package platform

/*
#cgo LDFLAGS: -lX11 -lGL -lm
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xatom.h>
#include <X11/keysym.h>
#include <GL/gl.h>
#include <GL/glx.h>

typedef struct {
    unsigned long flags;
    unsigned long functions;
    unsigned long decorations;
    long input_mode;
    unsigned long status;
} MotifWmHints;

typedef struct {
    Display* dpy;
    Window win;
    Window root;
    int screen;
    GLXContext ctx;
    Atom wmDelete;
    int width;
    int height;
    int maximized;
} OsmuX11Window;

static void osmu_apply_wm_hints(Display* dpy, Window win, int w, int h) {
    // 1. Standard XSizeHints (minimum size so WMs allow resizing and maximization)
    // Note: Do NOT set PMaxSize or max_width/max_height, as setting maximum size
    // constraints causes many window managers (Mutter, XFWM, etc.) to disable or hide
    // the Maximize button on the window frame.
    XSizeHints* sh = XAllocSizeHints();
    if (sh) {
        sh->flags = PSize | PMinSize;
        sh->width = w;
        sh->height = h;
        sh->min_width = 640;
        sh->min_height = 420;
        XSetWMNormalHints(dpy, win, sh);
        XFree(sh);
    }

    // 2. XWMHints (input = True, initial_state = NormalState)
    XWMHints* wmh = XAllocWMHints();
    if (wmh) {
        wmh->flags = InputHint | StateHint;
        wmh->input = True;
        wmh->initial_state = NormalState;
        XSetWMHints(dpy, win, wmh);
        XFree(wmh);
    }

    // 3. Motif hints requesting all decorations and all window functions (minimize, maximize, resize, close)
    // In Motif specification:
    //   flags: (1L << 0) [MWM_HINTS_FUNCTIONS] | (1L << 1) [MWM_HINTS_DECORATIONS]
    //   functions: 1L [MWM_FUNC_ALL] enables all window functions (resize, move, minimize, maximize, close).
    //   decorations: 1L [MWM_DECOR_ALL] enables all decorations (title, border, resize handles, menu, minimize, maximize).
    // Note: When MWM_DECOR_ALL is set, any additional decoration bits are treated by Motif as SUBTRACTIONS (exclusions).
    // Therefore, MWM_DECOR_ALL alone (1L) requests all decorations without excluding minimize or maximize.
    Atom motifAtom = XInternAtom(dpy, "_MOTIF_WM_HINTS", False);
    if (motifAtom != None) {
        MotifWmHints hints;
        memset(&hints, 0, sizeof(hints));
        hints.flags = (1L << 0) | (1L << 1); // MWM_HINTS_FUNCTIONS | MWM_HINTS_DECORATIONS
        hints.functions = 1L;                // MWM_FUNC_ALL
        hints.decorations = 1L;              // MWM_DECOR_ALL
        XChangeProperty(dpy, win, motifAtom, motifAtom, 32, PropModeReplace,
                        (unsigned char*)&hints, 5);
    }

    // 4. EWMH _NET_WM_WINDOW_TYPE = _NET_WM_WINDOW_TYPE_NORMAL
    Atom wmType = XInternAtom(dpy, "_NET_WM_WINDOW_TYPE", False);
    Atom wmTypeNormal = XInternAtom(dpy, "_NET_WM_WINDOW_TYPE_NORMAL", False);
    if (wmType != None && wmTypeNormal != None) {
        XChangeProperty(dpy, win, wmType, XA_ATOM, 32, PropModeReplace,
                        (unsigned char*)&wmTypeNormal, 1);
    }
}

static OsmuX11Window* osmu_create_window(const char* title, int w, int h) {
    Display* dpy = XOpenDisplay(NULL);
    if (!dpy) {
        return NULL;
    }
    int screen = DefaultScreen(dpy);
    int attribs[] = {
        GLX_RGBA,
        GLX_DEPTH_SIZE, 24,
        GLX_DOUBLEBUFFER,
        None
    };
    XVisualInfo* vi = glXChooseVisual(dpy, screen, attribs);
    if (!vi) {
        XCloseDisplay(dpy);
        return NULL;
    }
    Window root = RootWindow(dpy, vi->screen);
    XSetWindowAttributes swa;
    memset(&swa, 0, sizeof(swa));
    swa.colormap = XCreateColormap(dpy, root, vi->visual, AllocNone);
    swa.event_mask = ExposureMask | KeyPressMask | KeyReleaseMask |
                     ButtonPressMask | ButtonReleaseMask | PointerMotionMask |
                     StructureNotifyMask;

    Window win = XCreateWindow(
        dpy, root, 60, 40, w, h, 0,
        vi->depth, InputOutput, vi->visual,
        CWColormap | CWEventMask, &swa
    );
    XStoreName(dpy, win, title);

    Atom netWmName = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom utf8Str = XInternAtom(dpy, "UTF8_STRING", False);
    if (netWmName != None && utf8Str != None) {
        XChangeProperty(dpy, win, netWmName, utf8Str, 8, PropModeReplace,
                        (const unsigned char*)title, strlen(title));
    }

    osmu_apply_wm_hints(dpy, win, w, h);

    Atom wmDelete = XInternAtom(dpy, "WM_DELETE_WINDOW", False);
    XSetWMProtocols(dpy, win, &wmDelete, 1);

    XMapWindow(dpy, win);
    GLXContext ctx = glXCreateContext(dpy, vi, NULL, GL_TRUE);
    glXMakeCurrent(dpy, win, ctx);
    XFree(vi);

    glEnable(GL_BLEND);
    glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA);

    OsmuX11Window* xw = (OsmuX11Window*)calloc(1, sizeof(OsmuX11Window));
    xw->dpy = dpy;
    xw->win = win;
    xw->root = root;
    xw->screen = screen;
    xw->ctx = ctx;
    xw->wmDelete = wmDelete;
    xw->width = w;
    xw->height = h;
    return xw;
}

static void osmu_minimize(OsmuX11Window* xw) {
    if (!xw) return;
    XIconifyWindow(xw->dpy, xw->win, xw->screen);
    XFlush(xw->dpy);
}

static void osmu_toggle_maximize(OsmuX11Window* xw) {
    if (!xw) return;
    Atom wmState = XInternAtom(xw->dpy, "_NET_WM_STATE", False);
    Atom maxHorz = XInternAtom(xw->dpy, "_NET_WM_STATE_MAXIMIZED_HORZ", False);
    Atom maxVert = XInternAtom(xw->dpy, "_NET_WM_STATE_MAXIMIZED_VERT", False);
    if (wmState == None) return;

    XEvent xev;
    memset(&xev, 0, sizeof(xev));
    xev.type = ClientMessage;
    xev.xclient.window = xw->win;
    xev.xclient.message_type = wmState;
    xev.xclient.format = 32;
    xev.xclient.data.l[0] = 2; // _NET_WM_STATE_TOGGLE
    xev.xclient.data.l[1] = maxHorz;
    xev.xclient.data.l[2] = maxVert;
    xev.xclient.data.l[3] = 1;

    XSendEvent(xw->dpy, xw->root, False,
               SubstructureRedirectMask | SubstructureNotifyMask, &xev);
    xw->maximized = !xw->maximized;
    XFlush(xw->dpy);
}

static void osmu_set_title(OsmuX11Window* xw, const char* title) {
    if (!xw) return;
    XStoreName(xw->dpy, xw->win, title);
    Atom netWmName = XInternAtom(xw->dpy, "_NET_WM_NAME", False);
    Atom utf8Str = XInternAtom(xw->dpy, "UTF8_STRING", False);
    if (netWmName != None && utf8Str != None) {
        XChangeProperty(xw->dpy, xw->win, netWmName, utf8Str, 8, PropModeReplace,
                        (const unsigned char*)title, strlen(title));
    }
    XFlush(xw->dpy);
}

static void osmu_begin_frame(OsmuX11Window* xw, int w, int h) {
    if (!xw) return;
    glViewport(0, 0, w, h);
    glMatrixMode(GL_PROJECTION);
    glLoadIdentity();
    glOrtho(0, w, h, 0, -1, 1);
    glMatrixMode(GL_MODELVIEW);
    glLoadIdentity();
    glDisable(GL_DEPTH_TEST);
    glEnable(GL_BLEND);
    glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA);
}

static void osmu_clear(float r, float g, float b, float a) {
    glClearColor(r, g, b, a);
    glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);
}

static void osmu_draw_rect(float x, float y, float w, float h, float r, float g, float b, float a) {
    glColor4f(r, g, b, a);
    glBegin(GL_QUADS);
    glVertex2f(x, y);
    glVertex2f(x + w, y);
    glVertex2f(x + w, y + h);
    glVertex2f(x, y + h);
    glEnd();
}

static void osmu_draw_triangle(float x1, float y1, float x2, float y2, float x3, float y3, float r, float g, float b, float a) {
    glColor4f(r, g, b, a);
    glBegin(GL_TRIANGLES);
    glVertex2f(x1, y1);
    glVertex2f(x2, y2);
    glVertex2f(x3, y3);
    glEnd();
}

static void osmu_draw_rect_outline(float x, float y, float w, float h, float thickness, float r, float g, float b, float a) {
    glLineWidth(thickness);
    glColor4f(r, g, b, a);
    glBegin(GL_LINE_LOOP);
    glVertex2f(x, y);
    glVertex2f(x + w, y);
    glVertex2f(x + w, y + h);
    glVertex2f(x, y + h);
    glEnd();
}

static void osmu_draw_line(float x1, float y1, float x2, float y2, float thickness, float r, float g, float b, float a) {
    glLineWidth(thickness);
    glColor4f(r, g, b, a);
    glBegin(GL_LINES);
    glVertex2f(x1, y1);
    glVertex2f(x2, y2);
    glEnd();
}

static void osmu_draw_circle(float cx, float cy, float rad, float r, float g, float b, float a) {
    glColor4f(r, g, b, a);
    glBegin(GL_TRIANGLE_FAN);
    glVertex2f(cx, cy);
    for (int i = 0; i <= 20; i++) {
        float ang = (float)i * 6.2831853f / 20.0f;
        glVertex2f(cx + cosf(ang) * rad, cy + sinf(ang) * rad);
    }
    glEnd();
}

static void osmu_swap(OsmuX11Window* xw) {
    if (!xw) return;
    glXSwapBuffers(xw->dpy, xw->win);
}

static void osmu_close(OsmuX11Window* xw) {
    if (!xw) return;
    glXMakeCurrent(xw->dpy, None, NULL);
    glXDestroyContext(xw->dpy, xw->ctx);
    XDestroyWindow(xw->dpy, xw->win);
    XCloseDisplay(xw->dpy);
    free(xw);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type linuxWindow struct {
	xw *C.OsmuX11Window
}

// NewWindow creates a native Linux X11 + OpenGL window with full WM decorations and min/max buttons.
func NewWindow(title string, width, height int) (Window, error) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	xw := C.osmu_create_window(cTitle, C.int(width), C.int(height))
	if xw == nil {
		return nil, fmt.Errorf("could not open X11 display (headless environment or DISPLAY not set)")
	}
	return &linuxWindow{xw: xw}, nil
}

func (w *linuxWindow) Minimize() {
	if w.xw != nil {
		C.osmu_minimize(w.xw)
	}
}

func (w *linuxWindow) ToggleMaximize() {
	if w.xw != nil {
		C.osmu_toggle_maximize(w.xw)
	}
}

func (w *linuxWindow) SetTitle(title string) {
	if w.xw != nil {
		cTitle := C.CString(title)
		defer C.free(unsafe.Pointer(cTitle))
		C.osmu_set_title(w.xw, cTitle)
	}
}

func (w *linuxWindow) PollEvents(in *InputState) {
	if w.xw == nil {
		in.ShouldClose = true
		return
	}
	in.IsMaximized = w.xw.maximized != 0
	var ev C.XEvent
	for C.XPending(w.xw.dpy) > 0 {
		C.XNextEvent(w.xw.dpy, &ev)
		evType := *(*C.int)(unsafe.Pointer(&ev))
		switch evType {
		case C.ClientMessage:
			cm := (*C.XClientMessageEvent)(unsafe.Pointer(&ev))
			if C.Atom(cm.data[0]) == w.xw.wmDelete {
				in.ShouldClose = true
			}
		case C.ConfigureNotify:
			ce := (*C.XConfigureEvent)(unsafe.Pointer(&ev))
			if int(ce.width) > 0 && int(ce.height) > 0 {
				in.Width = int(ce.width)
				in.Height = int(ce.height)
			}
		case C.MotionNotify:
			me := (*C.XMotionEvent)(unsafe.Pointer(&ev))
			nx, ny := float64(me.x), float64(me.y)
			in.MouseDX += nx - in.MouseX
			in.MouseDY += ny - in.MouseY
			in.MouseX = nx
			in.MouseY = ny
		case C.ButtonPress:
			be := (*C.XButtonEvent)(unsafe.Pointer(&ev))
			btn := int(be.button)
			if btn >= 1 && btn <= 3 {
				in.MouseDown[btn] = true
				in.MouseClicked[btn] = true
			} else if btn == 4 {
				in.ScrollY += 1.0
			} else if btn == 5 {
				in.ScrollY -= 1.0
			}
		case C.ButtonRelease:
			be := (*C.XButtonEvent)(unsafe.Pointer(&ev))
			btn := int(be.button)
			if btn >= 1 && btn <= 3 {
				in.MouseDown[btn] = false
				in.MouseReleased[btn] = true
			}
		case C.KeyPress:
			ke := (*C.XKeyEvent)(unsafe.Pointer(&ev))
			var buf [16]C.char
			var sym C.KeySym
			n := C.XLookupString(ke, &buf[0], 15, &sym, nil)
			keyName := keysymName(sym)
			if keyName != "" {
				in.KeysDown[keyName] = true
				in.KeysPressed[keyName] = true
			}
			if n > 0 {
				ch := byte(buf[0])
				if ch >= 32 && ch <= 126 {
					in.TypedText += string(ch)
				}
			}
		case C.KeyRelease:
			ke := (*C.XKeyEvent)(unsafe.Pointer(&ev))
			sym := C.XLookupKeysym(ke, 0)
			keyName := keysymName(sym)
			if keyName != "" {
				in.KeysDown[keyName] = false
			}
		}
	}
}

func keysymName(sym C.KeySym) string {
	switch sym {
	case C.XK_BackSpace:
		return "Backspace"
	case C.XK_Return, C.XK_KP_Enter:
		return "Enter"
	case C.XK_Escape:
		return "Escape"
	case C.XK_Delete:
		return "Delete"
	case C.XK_space:
		return "Space"
	case C.XK_Tab:
		return "Tab"
	case C.XK_Left:
		return "Left"
	case C.XK_Right:
		return "Right"
	case C.XK_Up:
		return "Up"
	case C.XK_Down:
		return "Down"
	default:
		if sym >= 'a' && sym <= 'z' {
			return string(rune(sym - 'a' + 'A'))
		}
		if sym >= 'A' && sym <= 'Z' {
			return string(rune(sym))
		}
		return ""
	}
}

func (w *linuxWindow) DrawOps(width, height int, ops []DrawCmd) {
	if w.xw == nil {
		return
	}
	C.osmu_begin_frame(w.xw, C.int(width), C.int(height))
	for _, cmd := range ops {
		switch cmd.Kind {
		case CmdClear:
			C.osmu_clear(C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
		case CmdRect:
			C.osmu_draw_rect(C.float(cmd.X), C.float(cmd.Y), C.float(cmd.W), C.float(cmd.H),
				C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
		case CmdTriangle:
			C.osmu_draw_triangle(C.float(cmd.X), C.float(cmd.Y), C.float(cmd.X2), C.float(cmd.Y2), C.float(cmd.X3), C.float(cmd.Y3),
				C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
		case CmdRectOutline:
			th := cmd.Thickness
			if th <= 0 {
				th = 1.0
			}
			C.osmu_draw_rect_outline(C.float(cmd.X), C.float(cmd.Y), C.float(cmd.W), C.float(cmd.H),
				C.float(th), C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
		case CmdLine:
			th := cmd.Thickness
			if th <= 0 {
				th = 1.5
			}
			C.osmu_draw_line(C.float(cmd.X), C.float(cmd.Y), C.float(cmd.X2), C.float(cmd.Y2),
				C.float(th), C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
		case CmdPolyLine:
			th := cmd.Thickness
			if th <= 0 {
				th = 2.0
			}
			for i := 0; i < len(cmd.Points)-1; i++ {
				p1 := cmd.Points[i]
				p2 := cmd.Points[i+1]
				C.osmu_draw_line(C.float(p1[0]), C.float(p1[1]), C.float(p2[0]), C.float(p2[1]),
					C.float(th), C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
			}
		case CmdCircle:
			C.osmu_draw_circle(C.float(cmd.X), C.float(cmd.Y), C.float(cmd.Radius),
				C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
		case CmdText:
			drawBitmapTextGL(cmd)
		}
	}
}

func drawBitmapTextGL(cmd DrawCmd) {
	scale := cmd.Scale
	if scale <= 0 {
		scale = 1.5
	}
	cx := cmd.X
	cy := cmd.Y
	for i := 0; i < len(cmd.Text); i++ {
		ch := cmd.Text[i]
		glyph, ok := Font5x7[ch]
		if !ok {
			glyph = Font5x7['?']
		}
		for col := 0; col < 5; col++ {
			colBits := glyph[col]
			if colBits == 0 {
				continue
			}
			for row := 0; row < 7; row++ {
				if (colBits & (1 << row)) != 0 {
					px := cx + float64(col)*scale
					py := cy + float64(row)*scale
					C.osmu_draw_rect(C.float(px), C.float(py), C.float(scale), C.float(scale),
						C.float(cmd.R), C.float(cmd.G), C.float(cmd.B), C.float(cmd.A))
				}
			}
		}
		cx += 6.0 * scale
	}
}

func (w *linuxWindow) SwapBuffers() {
	C.osmu_swap(w.xw)
}

func (w *linuxWindow) Close() {
	C.osmu_close(w.xw)
	w.xw = nil
}
