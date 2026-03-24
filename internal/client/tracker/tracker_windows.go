//go:build windows

// internal/client/tracker/tracker_windows.go
package tracker

import (
	"context"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	psapi                        = syscall.NewLazyDLL("psapi.dll")
	procSetWinEventHook          = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent           = user32.NewProc("UnhookWinEvent")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procGetModuleBaseNameW       = psapi.NewProc("GetModuleBaseNameW")
)

const (
	eventSystemForeground   = 0x0003
	winEventOutOfContext    = 0x0000
	processQueryInformation = 0x0400
	processVMRead           = 0x0010
)

// winMSG is the Windows message structure.
type winMSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// windowsTracker implements the Tracker interface for Windows using
// SetWinEventHook for real-time foreground window change events.
type windowsTracker struct {
	events chan FocusChange
	done   chan struct{}
	hook   uintptr
	mu     sync.Mutex
}

// NewPlatformTracker creates a Windows-specific focus tracker.
func NewPlatformTracker() (Tracker, error) {
	return &windowsTracker{
		events: make(chan FocusChange, 64),
		done:   make(chan struct{}),
	}, nil
}

// Start begins listening for foreground window change events.
func (t *windowsTracker) Start(ctx context.Context) error {
	go t.messageLoop(ctx)
	return nil
}

// Events returns the channel of focus change events.
func (t *windowsTracker) Events() <-chan FocusChange {
	return t.events
}

// Stop unhooks the event listener and signals the message loop to exit.
func (t *windowsTracker) Stop() {
	t.mu.Lock()
	if t.hook != 0 {
		procUnhookWinEvent.Call(t.hook)
		t.hook = 0
	}
	t.mu.Unlock()
	close(t.done)
}

func (t *windowsTracker) messageLoop(ctx context.Context) {
	// The callback must be set up on the same thread as the message loop.
	callback := syscall.NewCallback(t.winEventProc)

	hook, _, _ := procSetWinEventHook.Call(
		eventSystemForeground, // eventMin
		eventSystemForeground, // eventMax
		0,                     // hmodWinEventProc (0 = out-of-context)
		callback,              // pfnWinEventProc
		0,                     // idProcess (0 = all)
		0,                     // idThread (0 = all)
		winEventOutOfContext,  // dwFlags
	)

	t.mu.Lock()
	t.hook = hook
	t.mu.Unlock()

	if hook == 0 {
		return
	}

	// Also emit the current foreground window immediately.
	t.emitCurrentFocus()

	// Run Windows message loop — required for SetWinEventHook callbacks.
	var msg winMSG
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		default:
		}

		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if ret == 0 { // WM_QUIT
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (t *windowsTracker) winEventProc(
	hWinEventHook uintptr,
	event uint32,
	hwnd uintptr,
	idObject int32,
	idChild int32,
	idEventThread uint32,
	dwmsEventTime uint32,
) uintptr {
	t.emitCurrentFocus()
	return 0
}

func (t *windowsTracker) emitCurrentFocus() {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return
	}

	title := getWindowText(hwnd)
	appName := getProcessName(hwnd)

	event := FocusChange{
		AppName:     appName,
		WindowTitle: title,
		Timestamp:   time.Now().UTC(),
	}

	select {
	case t.events <- event:
	default:
	}
}

func getWindowText(hwnd uintptr) string {
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return ""
	}
	buf := make([]uint16, length+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), length+1)
	return syscall.UTF16ToString(buf)
}

func getProcessName(hwnd uintptr) string {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}

	handle, _, _ := procOpenProcess.Call(
		processQueryInformation|processVMRead,
		0, uintptr(pid),
	)
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)

	var buf [256]uint16
	ret, _, _ := procGetModuleBaseNameW.Call(
		handle,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		256,
	)
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:ret])
}
