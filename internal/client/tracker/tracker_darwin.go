//go:build darwin && cgo

// internal/client/tracker/tracker_darwin.go
package tracker

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

typedef struct {
    const char* app_name;
    const char* window_title;
    int pid;
} FocusInfo;

FocusInfo getFocusedWindow() {
    FocusInfo info = {NULL, NULL, 0};

    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) {
            return info;
        }

        info.pid = (int)app.processIdentifier;
        NSString *name = app.localizedName;
        if (name != nil) {
            info.app_name = strdup([name UTF8String]);
        }

        // Use Accessibility API for window title.
        AXUIElementRef appElement = AXUIElementCreateApplication(app.processIdentifier);
        if (appElement != NULL) {
            AXUIElementRef focusedWindow = NULL;
            AXError err = AXUIElementCopyAttributeValue(
                appElement,
                kAXFocusedWindowAttribute,
                (CFTypeRef *)&focusedWindow
            );
            if (err == kAXErrorSuccess && focusedWindow != NULL) {
                CFStringRef title = NULL;
                AXError titleErr = AXUIElementCopyAttributeValue(
                    focusedWindow,
                    kAXTitleAttribute,
                    (CFTypeRef *)&title
                );
                if (titleErr == kAXErrorSuccess && title != NULL) {
                    NSString *titleStr = (__bridge NSString *)title;
                    info.window_title = strdup([titleStr UTF8String]);
                    CFRelease(title);
                }
                CFRelease(focusedWindow);
            }
            CFRelease(appElement);
        }
    }
    return info;
}

void freeFocusInfo(FocusInfo info) {
    if (info.app_name != NULL) free((void*)info.app_name);
    if (info.window_title != NULL) free((void*)info.window_title);
}
*/
import "C"

import (
	"context"
	"time"
)

// darwinTracker implements the Tracker interface for macOS via NSWorkspace
// and the Accessibility API.
type darwinTracker struct {
	pollInterval time.Duration
	events       chan FocusChange
	done         chan struct{}
}

// NewPlatformTracker creates a macOS-specific focus tracker.
func NewPlatformTracker() (Tracker, error) {
	return &darwinTracker{
		pollInterval: 1 * time.Second,
		events:       make(chan FocusChange, 64),
		done:         make(chan struct{}),
	}, nil
}

// Start begins polling the focused window every pollInterval.
func (t *darwinTracker) Start(ctx context.Context) error {
	go t.pollLoop(ctx)
	return nil
}

// Events returns the channel of focus change events.
func (t *darwinTracker) Events() <-chan FocusChange {
	return t.events
}

// Stop signals the poll loop to exit.
func (t *darwinTracker) Stop() {
	close(t.done)
}

func (t *darwinTracker) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	var lastApp, lastTitle string

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		case <-ticker.C:
			app, title := t.getFocusedWindow()
			if app != lastApp || title != lastTitle {
				event := FocusChange{
					AppName:     app,
					WindowTitle: title,
					Timestamp:   time.Now().UTC(),
				}
				select {
				case t.events <- event:
				default:
					// Channel full — drop oldest if needed.
				}
				lastApp = app
				lastTitle = title
			}
		}
	}
}

func (t *darwinTracker) getFocusedWindow() (appName, windowTitle string) {
	info := C.getFocusedWindow()
	defer C.freeFocusInfo(info)

	if info.app_name != nil {
		appName = C.GoString(info.app_name)
	}
	if info.window_title != nil {
		windowTitle = C.GoString(info.window_title)
	}
	return appName, windowTitle
}
