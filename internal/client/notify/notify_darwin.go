//go:build darwin && cgo

// internal/client/notify/notify_darwin.go
package notify

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Using NSUserNotificationCenter — works for non-App-Store binaries.
// Deprecated since macOS 11 but still functional through macOS 14+.
// If Apple removes it entirely, migrate to UNUserNotificationCenter.

extern void goNotificationClicked(const char* identifier);

@interface TraskerNotificationDelegate : NSObject <NSUserNotificationCenterDelegate>
@end

@implementation TraskerNotificationDelegate

- (void)userNotificationCenter:(NSUserNotificationCenter *)center
       didActivateNotification:(NSUserNotification *)notification {
    NSString *identifier = notification.identifier;
    if (identifier != nil) {
        goNotificationClicked([identifier UTF8String]);
    }
    [center removeDeliveredNotification:notification];
}

// Always show notification even when app is frontmost.
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center
     shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}

@end

static TraskerNotificationDelegate *notifDelegate = nil;

void initNotificationDelegate() {
    if (notifDelegate == nil) {
        notifDelegate = [[TraskerNotificationDelegate alloc] init];
        [[NSUserNotificationCenter defaultUserNotificationCenter] setDelegate:notifDelegate];
    }
}

void sendNotification(const char* title, const char* body, const char* identifier) {
    @autoreleasepool {
        initNotificationDelegate();

        NSUserNotification *notif = [[NSUserNotification alloc] init];
        notif.title = [NSString stringWithUTF8String:title];
        notif.informativeText = [NSString stringWithUTF8String:body];
        if (identifier != NULL) {
            notif.identifier = [NSString stringWithUTF8String:identifier];
        }

        [[NSUserNotificationCenter defaultUserNotificationCenter]
            deliverNotification:notif];
    }
}

void removeNotificationByID(const char* identifier) {
    @autoreleasepool {
        NSUserNotificationCenter *center =
            [NSUserNotificationCenter defaultUserNotificationCenter];
        for (NSUserNotification *notif in center.deliveredNotifications) {
            if ([notif.identifier isEqualToString:
                [NSString stringWithUTF8String:identifier]]) {
                [center removeDeliveredNotification:notif];
                break;
            }
        }
    }
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// clickCallbackMu guards the global click callback map.
var clickCallbackMu sync.Mutex
var clickCallbackMap = make(map[string]ClickAction)
var notifCounter uint64

//export goNotificationClicked
func goNotificationClicked(identifier *C.char) {
	if identifier == nil {
		return
	}
	id := C.GoString(identifier)

	clickCallbackMu.Lock()
	cb, exists := clickCallbackMap[id]
	if exists {
		delete(clickCallbackMap, id)
	}
	clickCallbackMu.Unlock()

	if exists && cb != nil {
		go cb()
	}
}

// DarwinNotifier implements the Notifier interface for macOS using
// NSUserNotificationCenter.
type DarwinNotifier struct{}

// NewDarwinNotifier creates a macOS notifier.
func NewDarwinNotifier() (*DarwinNotifier, error) {
	return &DarwinNotifier{}, nil
}

// Notify sends a macOS desktop notification. onClick is called if the user
// clicks the notification (may be nil).
func (n *DarwinNotifier) Notify(title, body string, onClick ClickAction) error {
	clickCallbackMu.Lock()
	notifCounter++
	id := fmt.Sprintf("trasker-%d", notifCounter)
	if onClick != nil {
		clickCallbackMap[id] = onClick
	}
	clickCallbackMu.Unlock()

	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))

	cBody := C.CString(body)
	defer C.free(unsafe.Pointer(cBody))

	cID := C.CString(id)
	defer C.free(unsafe.Pointer(cID))

	C.sendNotification(cTitle, cBody, cID)
	return nil
}

// Close is a no-op for the Darwin notifier (no persistent resources).
func (n *DarwinNotifier) Close() error {
	return nil
}
