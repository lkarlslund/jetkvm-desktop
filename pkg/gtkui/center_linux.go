package gtkui

/*
#cgo linux pkg-config: x11 gtk4
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <string.h>
#include <stdlib.h>

typedef struct _GdkSurface GdkSurface;
extern unsigned long gdk_x11_surface_get_xid(GdkSurface *surface);

static int read_workarea(Display *dpy, int *x, int *y, int *w, int *h) {
	Atom workareaAtom = XInternAtom(dpy, "_NET_WORKAREA", True);
	if (workareaAtom == None) return 0;
	Atom actualType;
	int actualFormat;
	unsigned long nitems, bytesAfter;
	unsigned char *prop = NULL;
	int rc = XGetWindowProperty(dpy, DefaultRootWindow(dpy), workareaAtom,
		0, 4, False, XA_CARDINAL,
		&actualType, &actualFormat, &nitems, &bytesAfter, &prop);
	if (rc != Success || !prop || nitems < 4) {
		if (prop) XFree(prop);
		return 0;
	}
	long *vals = (long *)prop;
	*x = (int)vals[0]; *y = (int)vals[1];
	*w = (int)vals[2]; *h = (int)vals[3];
	XFree(prop);
	return 1;
}

static void center_xid(unsigned long xid) {
	if (!xid) return;

	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) return;

	XWindowAttributes attr;
	if (!XGetWindowAttributes(dpy, xid, &attr)) {
		XCloseDisplay(dpy);
		return;
	}

	int wax = 0, way = 0;
	int waw = DisplayWidth(dpy, DefaultScreen(dpy));
	int wah = DisplayHeight(dpy, DefaultScreen(dpy));
	read_workarea(dpy, &wax, &way, &waw, &wah);

	int x = wax + (waw - attr.width) / 2;
	int y = way + (wah - attr.height) / 2;
	if (x < wax) x = wax;
	if (y < way) y = way;

	// _NET_MOVERESIZE_WINDOW on the CLIENT window. The WM interprets the
	// coordinates as the client area origin and adjusts its frame around it.
	// gravity=0 (current), source=1 (app), flags for x+y.
	Atom moveResize = XInternAtom(dpy, "_NET_MOVERESIZE_WINDOW", False);
	XEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.xclient.type = ClientMessage;
	ev.xclient.message_type = moveResize;
	ev.xclient.display = dpy;
	ev.xclient.window = xid;
	ev.xclient.format = 32;
	// gravity=StaticGravity(10), x(bit8), y(bit9), source=app(bit12)
	ev.xclient.data.l[0] = 10 | (1L << 8) | (1L << 9) | (1L << 12);
	ev.xclient.data.l[1] = x;
	ev.xclient.data.l[2] = y;
	ev.xclient.data.l[3] = 0;
	ev.xclient.data.l[4] = 0;
	XSendEvent(dpy, DefaultRootWindow(dpy), False,
		SubstructureRedirectMask | SubstructureNotifyMask, &ev);

	XFlush(dpy);
	XCloseDisplay(dpy);
}

static unsigned long get_surface_xid(GdkSurface *surface) {
	if (!surface) return 0;
	return gdk_x11_surface_get_xid(surface);
}

static int monitor_workarea(int *w, int *h) {
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) return 0;
	int x = 0, y = 0;
	int gw = DisplayWidth(dpy, DefaultScreen(dpy));
	int gh = DisplayHeight(dpy, DefaultScreen(dpy));
	read_workarea(dpy, &x, &y, &gw, &gh);
	*w = gw; *h = gh;
	XCloseDisplay(dpy);
	return 1;
}
*/
import "C"

import (
	"unsafe"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func centerWindow(win *gtk.ApplicationWindow) {
	var handler glib.SignalHandle
	handler = win.ConnectMap(func() {
		win.HandlerDisconnect(handler)

		doCenter := func() bool {
			surfacer := win.Surface()
			if surfacer == nil {
				return false
			}
			native := coreglib.InternObject(surfacer).Native()
			if native == 0 {
				return false
			}
			xid := C.get_surface_xid((*C.GdkSurface)(unsafe.Pointer(native)))
			if xid == 0 {
				return false
			}
			C.center_xid(xid)
			return false
		}

		// Fire multiple times after map. Cinnamon/Muffin applies its own
		// "smart" placement that may override ours; we re-center until
		// the WM is done with its initial placement.
		glib.TimeoutAdd(100, doCenter)
		glib.TimeoutAdd(300, doCenter)
		glib.TimeoutAdd(600, doCenter)
		glib.TimeoutAdd(1000, doCenter)
	})
}

func monitorWorkarea() (int, int) {
	var w, h C.int
	if C.monitor_workarea(&w, &h) == 0 {
		return 0, 0
	}
	return int(w), int(h)
}
