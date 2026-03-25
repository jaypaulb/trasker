//go:build linux && cgo

// internal/client/tracker/tracker_linux_x11.go
package tracker

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <string.h>

// getActiveWindowInfo retrieves the app name (WM_CLASS) and window title (_NET_WM_NAME or WM_NAME)
// of the currently focused window. Returns 0 on success, -1 on failure.
static int getActiveWindowInfo(Display *dpy, char *app_name, int app_len, char *win_title, int title_len) {
    Window focused;
    int revert;
    XGetInputFocus(dpy, &focused, &revert);
    if (focused == None || focused == PointerRoot) {
        return -1;
    }

    // Get WM_CLASS for app name
    XClassHint class_hint;
    if (XGetClassHint(dpy, focused, &class_hint)) {
        if (class_hint.res_name) {
            strncpy(app_name, class_hint.res_name, app_len - 1);
            app_name[app_len - 1] = '\0';
            XFree(class_hint.res_name);
        }
        if (class_hint.res_class) {
            XFree(class_hint.res_class);
        }
    }

    // Try _NET_WM_NAME first (UTF-8), fall back to WM_NAME
    Atom net_wm_name = XInternAtom(dpy, "_NET_WM_NAME", True);
    Atom utf8_string = XInternAtom(dpy, "UTF8_STRING", True);
    if (net_wm_name != None && utf8_string != None) {
        Atom actual_type;
        int actual_format;
        unsigned long nitems, bytes_after;
        unsigned char *prop = NULL;
        if (XGetWindowProperty(dpy, focused, net_wm_name, 0, 1024, False,
                               utf8_string, &actual_type, &actual_format,
                               &nitems, &bytes_after, &prop) == Success && prop) {
            strncpy(win_title, (char *)prop, title_len - 1);
            win_title[title_len - 1] = '\0';
            XFree(prop);
            return 0;
        }
    }

    // Fallback: WM_NAME
    char *wm_name = NULL;
    if (XFetchName(dpy, focused, &wm_name) && wm_name) {
        strncpy(win_title, wm_name, title_len - 1);
        win_title[title_len - 1] = '\0';
        XFree(wm_name);
    }

    return 0;
}
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"
)

// X11Tracker implements the Tracker interface for X11 sessions.
// It polls the focused window every second and emits FocusChange events
// when the active window changes.
type X11Tracker struct {
	display *C.Display
	events  chan FocusChange
	cancel  context.CancelFunc

	lastApp   string
	lastTitle string
}

// NewX11Tracker creates a new X11 focus tracker.
// Returns an error if it cannot connect to the X11 display.
func NewX11Tracker() (*X11Tracker, error) {
	dpy := C.XOpenDisplay(nil)
	if dpy == nil {
		return nil, fmt.Errorf("cannot open X11 display (is DISPLAY set?)")
	}
	return &X11Tracker{
		display: dpy,
		events:  make(chan FocusChange, 32),
	}, nil
}

// Start begins polling the active window every 1 second.
func (t *X11Tracker) Start(ctx context.Context) error {
	ctx, t.cancel = context.WithCancel(ctx)
	go t.poll(ctx)
	return nil
}

// Stop ceases polling and closes the events channel.
func (t *X11Tracker) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	C.XCloseDisplay(t.display)
	close(t.events)
}

// Events returns the channel of focus change events.
func (t *X11Tracker) Events() <-chan FocusChange {
	return t.events
}

func (t *X11Tracker) poll(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app, title := t.getActive()
			if app != t.lastApp || title != t.lastTitle {
				t.lastApp = app
				t.lastTitle = title
				t.events <- FocusChange{
					AppName:     app,
					WindowTitle: title,
					Timestamp:   time.Now().UTC(),
				}
			}
		}
	}
}

func (t *X11Tracker) getActive() (string, string) {
	const bufSize = 512
	var appBuf [bufSize]C.char
	var titleBuf [bufSize]C.char

	ret := C.getActiveWindowInfo(
		t.display,
		(*C.char)(unsafe.Pointer(&appBuf[0])), C.int(bufSize),
		(*C.char)(unsafe.Pointer(&titleBuf[0])), C.int(bufSize),
	)
	if ret != 0 {
		return "", ""
	}

	return C.GoString((*C.char)(unsafe.Pointer(&appBuf[0]))),
		C.GoString((*C.char)(unsafe.Pointer(&titleBuf[0])))
}
