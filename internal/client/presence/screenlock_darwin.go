//go:build darwin && cgo

// internal/client/presence/screenlock_darwin.go
package presence

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// We use a C callback approach: register for NSDistributedNotificationCenter
// and call back into Go when lock/unlock occurs.

extern void goScreenLockCallback(int locked);

static id lockObserver = nil;
static id unlockObserver = nil;

void registerScreenLockObservers() {
    NSDistributedNotificationCenter *center =
        [NSDistributedNotificationCenter defaultCenter];

    lockObserver = [center addObserverForName:@"com.apple.screenIsLocked"
                                       object:nil
                                        queue:[NSOperationQueue mainQueue]
                                   usingBlock:^(NSNotification *note) {
        goScreenLockCallback(1);
    }];

    unlockObserver = [center addObserverForName:@"com.apple.screenIsUnlocked"
                                         object:nil
                                          queue:[NSOperationQueue mainQueue]
                                     usingBlock:^(NSNotification *note) {
        goScreenLockCallback(0);
    }];
}

void unregisterScreenLockObservers() {
    NSDistributedNotificationCenter *center =
        [NSDistributedNotificationCenter defaultCenter];
    if (lockObserver != nil) {
        [center removeObserver:lockObserver];
        lockObserver = nil;
    }
    if (unlockObserver != nil) {
        [center removeObserver:unlockObserver];
        unlockObserver = nil;
    }
}
*/
import "C"

import (
	"context"
	"sync"
	"time"
)

// screenLockCallbackMu guards the global callback reference.
var screenLockCallbackMu sync.Mutex
var screenLockCallbackFn func(locked bool)

//export goScreenLockCallback
func goScreenLockCallback(locked C.int) {
	screenLockCallbackMu.Lock()
	cb := screenLockCallbackFn
	screenLockCallbackMu.Unlock()

	if cb != nil {
		cb(locked != 0)
	}
}

// DarwinScreenLockListener implements screen lock detection for macOS via
// NSDistributedNotificationCenter (com.apple.screenIsLocked / screenIsUnlocked).
type DarwinScreenLockListener struct {
	events chan StateChange
	done   chan struct{}
}

// NewScreenLockListener creates a macOS screen lock listener.
func NewScreenLockListener() (*DarwinScreenLockListener, error) {
	return &DarwinScreenLockListener{
		events: make(chan StateChange, 16),
		done:   make(chan struct{}),
	}, nil
}

// Start registers for screen lock/unlock notifications.
func (s *DarwinScreenLockListener) Start(ctx context.Context) error {
	screenLockCallbackMu.Lock()
	screenLockCallbackFn = func(locked bool) {
		state := Away
		if !locked {
			state = Tracking
		}
		select {
		case s.events <- StateChange{
			State:     state,
			Timestamp: time.Now().UTC(),
		}:
		case <-s.done:
		}
	}
	screenLockCallbackMu.Unlock()

	C.registerScreenLockObservers()
	return nil
}

// Events returns the channel of screen lock/unlock state changes.
func (s *DarwinScreenLockListener) Events() <-chan StateChange {
	return s.events
}

// Stop unregisters observers and cleans up.
func (s *DarwinScreenLockListener) Stop() {
	C.unregisterScreenLockObservers()

	screenLockCallbackMu.Lock()
	screenLockCallbackFn = nil
	screenLockCallbackMu.Unlock()

	close(s.done)
}
