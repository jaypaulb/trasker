//go:build linux && cgo

// internal/client/layout/enum_linux_x11.go
// X11 window enumerator using EWMH _NET_CLIENT_LIST_STACKING with an
// XQueryTree fallback. Per-window geometry uses XGetWindowAttributes
// AND XTranslateCoordinates: XGetWindowAttributes returns parent-relative
// coords (07-RESEARCH.md Pitfall 1), so we ALWAYS translate to root.
package layout

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <string.h>

// enumWindows fills out_wids with up to max top-level windows.
// Tries _NET_CLIENT_LIST_STACKING first; falls back to XQueryTree(root).
// Returns the count, or -1 on hard failure.
static int enumWindows(Display *dpy,
                       Window *out_wids, int max,
                       Atom net_client_list_stacking) {
    Window root = DefaultRootWindow(dpy);

    if (net_client_list_stacking != None) {
        Atom actual_type;
        int actual_format;
        unsigned long nitems, bytes_after;
        unsigned char *prop = NULL;
        if (XGetWindowProperty(dpy, root, net_client_list_stacking, 0, 1024, False,
                               XA_WINDOW, &actual_type, &actual_format,
                               &nitems, &bytes_after, &prop) == Success && prop) {
            Window *wids = (Window *)prop;
            int n = (int)nitems;
            if (n > max) n = max;
            for (int i = 0; i < n; i++) out_wids[i] = wids[i];
            XFree(prop);
            return n;
        }
        if (prop) XFree(prop);
    }

    // Fallback: XQueryTree(root). On reparenting WMs this returns frame
    // windows rather than application windows — accept the degradation,
    // EWMH should normally succeed.
    Window root2, parent;
    Window *children = NULL;
    unsigned int nchildren = 0;
    if (XQueryTree(dpy, root, &root2, &parent, &children, &nchildren) == 0) {
        return -1;
    }
    int n = (int)nchildren;
    if (n > max) n = max;
    for (int i = 0; i < n; i++) out_wids[i] = children[i];
    if (children) XFree(children);
    return n;
}

// getGeom fetches root-absolute window geometry. Returns:
//   0 on success
//   -1 if XGetWindowAttributes fails
//   -2 if window is not viewable (caller skips)
//   -3 if XTranslateCoordinates fails
// Per Pitfall 1: XGetWindowAttributes coords are parent-relative, so we
// ALWAYS combine with XTranslateCoordinates(w, root).
static int getGeom(Display *dpy, Window w,
                   int *x, int *y, int *width, int *height) {
    XWindowAttributes attrs;
    if (!XGetWindowAttributes(dpy, w, &attrs)) return -1;
    if (attrs.map_state != IsViewable) return -2;
    Window child;
    if (!XTranslateCoordinates(dpy, w, DefaultRootWindow(dpy),
                               0, 0, x, y, &child)) return -3;
    *width = attrs.width;
    *height = attrs.height;
    return 0;
}

// getClassHint copies WM_CLASS res_name into out (NUL-terminated). Empty
// string on failure. res_class is ignored — Phase 7 follows the focus
// tracker convention of using res_name as the user-meaningful identifier.
static void getClassHint(Display *dpy, Window w, char *out, int len) {
    out[0] = '\0';
    XClassHint class_hint;
    if (XGetClassHint(dpy, w, &class_hint)) {
        if (class_hint.res_name) {
            strncpy(out, class_hint.res_name, len - 1);
            out[len - 1] = '\0';
            XFree(class_hint.res_name);
        }
        if (class_hint.res_class) {
            XFree(class_hint.res_class);
        }
    }
}

// getWindowName copies the window title into out. Tries _NET_WM_NAME
// (UTF-8) first, falls back to WM_NAME via XFetchName. Empty string on
// failure. NUL-terminated.
static void getWindowName(Display *dpy, Window w, char *out, int len,
                          Atom net_wm_name, Atom utf8_string) {
    out[0] = '\0';
    if (net_wm_name != None && utf8_string != None) {
        Atom actual_type;
        int actual_format;
        unsigned long nitems, bytes_after;
        unsigned char *prop = NULL;
        if (XGetWindowProperty(dpy, w, net_wm_name, 0, 1024, False,
                               utf8_string, &actual_type, &actual_format,
                               &nitems, &bytes_after, &prop) == Success && prop) {
            strncpy(out, (char *)prop, len - 1);
            out[len - 1] = '\0';
            XFree(prop);
            return;
        }
        if (prop) XFree(prop);
    }
    char *wm_name = NULL;
    if (XFetchName(dpy, w, &wm_name) && wm_name) {
        strncpy(out, wm_name, len - 1);
        out[len - 1] = '\0';
        XFree(wm_name);
    }
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// maxWindows is the per-tick cap. 256 is generous (Jaypaul has never had
// >100 windows; 256 covers extreme tiling configs).
const maxWindows = 256

// X11Enumerator implements Enumerator on Linux+cgo. It opens the X11
// display once and interns atoms once at construction; Enumerate() is the
// hot path called every 60s.
type X11Enumerator struct {
	display               *C.Display
	netClientListStacking C.Atom
	netWMName             C.Atom
	utf8String            C.Atom
}

// NewX11Enumerator opens the X11 display and interns EWMH atoms.
// Returns an error if DISPLAY is unset or the display cannot be opened.
func NewX11Enumerator() (*X11Enumerator, error) {
	dpy := C.XOpenDisplay(nil)
	if dpy == nil {
		return nil, fmt.Errorf("layout: cannot open X11 display (is DISPLAY set?)")
	}

	cName1 := C.CString("_NET_CLIENT_LIST_STACKING")
	defer C.free(unsafe.Pointer(cName1))
	cName2 := C.CString("_NET_WM_NAME")
	defer C.free(unsafe.Pointer(cName2))
	cName3 := C.CString("UTF8_STRING")
	defer C.free(unsafe.Pointer(cName3))

	return &X11Enumerator{
		display:               dpy,
		netClientListStacking: C.XInternAtom(dpy, cName1, C.True),
		netWMName:             C.XInternAtom(dpy, cName2, C.True),
		utf8String:            C.XInternAtom(dpy, cName3, C.True),
	}, nil
}

// NewPlatformEnumerator constructs an X11 enumerator on Linux+cgo. This
// is the symbol the daemon main wires; on no-cgo Linux / macOS / Windows
// the same name returns ErrUnsupported (see enum_*_nocgo.go etc.).
func NewPlatformEnumerator() (Enumerator, error) {
	return NewX11Enumerator()
}

// Enumerate returns the current set of viewable top-level windows.
func (e *X11Enumerator) Enumerate() ([]Window, error) {
	var wids [maxWindows]C.Window
	n := int(C.enumWindows(e.display, &wids[0], maxWindows, e.netClientListStacking))
	if n < 0 {
		return nil, fmt.Errorf("layout: window enumeration failed")
	}

	const bufSize = 512
	out := make([]Window, 0, n)
	for i := 0; i < n; i++ {
		var x, y, w, h C.int
		rc := C.getGeom(e.display, wids[i], &x, &y, &w, &h)
		if rc != 0 {
			// rc == -2 (not viewable) is the common case (popup menus,
			// IME windows, etc.). Treat all non-zero returns as "skip".
			continue
		}

		var appBuf [bufSize]C.char
		C.getClassHint(e.display, wids[i],
			(*C.char)(unsafe.Pointer(&appBuf[0])), C.int(bufSize))

		var titleBuf [bufSize]C.char
		C.getWindowName(e.display, wids[i],
			(*C.char)(unsafe.Pointer(&titleBuf[0])), C.int(bufSize),
			e.netWMName, e.utf8String)

		out = append(out, Window{
			AppName:     C.GoString((*C.char)(unsafe.Pointer(&appBuf[0]))),
			WindowTitle: C.GoString((*C.char)(unsafe.Pointer(&titleBuf[0]))),
			X:           int(x),
			Y:           int(y),
			W:           int(w),
			H:           int(h),
		})
	}
	return out, nil
}

// Close releases the X11 display.
func (e *X11Enumerator) Close() error {
	if e.display != nil {
		C.XCloseDisplay(e.display)
		e.display = nil
	}
	return nil
}
