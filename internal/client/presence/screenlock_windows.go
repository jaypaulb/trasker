//go:build windows

// internal/client/presence/screenlock_windows.go
package presence

import (
	"context"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	wtsapi32                             = syscall.NewLazyDLL("wtsapi32.dll")
	user32Win                            = syscall.NewLazyDLL("user32.dll")
	kernel32Win                          = syscall.NewLazyDLL("kernel32.dll")
	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
	procCreateWindowExW                  = user32Win.NewProc("CreateWindowExW")
	procDefWindowProcW                   = user32Win.NewProc("DefWindowProcW")
	procRegisterClassExW                 = user32Win.NewProc("RegisterClassExW")
	procGetMessageWLock                  = user32Win.NewProc("GetMessageW")
	procTranslateMessageLock             = user32Win.NewProc("TranslateMessage")
	procDispatchMessageWLock             = user32Win.NewProc("DispatchMessageW")
	procDestroyWindow                    = user32Win.NewProc("DestroyWindow")
	procGetModuleHandleW                 = kernel32Win.NewProc("GetModuleHandleW")
)

const (
	wmWTSSessionChange    = 0x02B1
	wtsSessionLock        = 0x7
	wtsSessionUnlock      = 0x8
	notifyForThisSession  = 0
)

// wndClassExW is the Windows window class structure.
type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// screenLockMSG is the Windows message structure used in the lock message loop.
type screenLockMSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// WindowsScreenLockListener implements screen lock detection for Windows via
// WTSRegisterSessionNotification and a hidden message-only window.
type WindowsScreenLockListener struct {
	events chan StateChange
	done   chan struct{}
	hwnd   uintptr
	mu     sync.Mutex
}

// Global reference for the window proc callback.
var globalScreenLock *WindowsScreenLockListener
var globalScreenLockMu sync.Mutex

// NewScreenLockListener creates a Windows screen lock listener.
func NewScreenLockListener() (*WindowsScreenLockListener, error) {
	return &WindowsScreenLockListener{
		events: make(chan StateChange, 16),
		done:   make(chan struct{}),
	}, nil
}

// Start creates a hidden message-only window and registers for session notifications.
func (s *WindowsScreenLockListener) Start(ctx context.Context) error {
	globalScreenLockMu.Lock()
	globalScreenLock = s
	globalScreenLockMu.Unlock()

	go s.messageLoop()
	return nil
}

// Events returns the channel of screen lock/unlock state changes.
func (s *WindowsScreenLockListener) Events() <-chan StateChange {
	return s.events
}

// Stop destroys the hidden window and stops the message loop.
func (s *WindowsScreenLockListener) Stop() {
	s.mu.Lock()
	if s.hwnd != 0 {
		procWTSUnRegisterSessionNotification.Call(s.hwnd)
		procDestroyWindow.Call(s.hwnd)
		s.hwnd = 0
	}
	s.mu.Unlock()

	globalScreenLockMu.Lock()
	globalScreenLock = nil
	globalScreenLockMu.Unlock()

	close(s.done)
}

func (s *WindowsScreenLockListener) messageLoop() {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("TraskerScreenLockClass")

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   syscall.NewCallback(screenLockWndProc),
		HInstance:     hInstance,
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	windowName, _ := syscall.UTF16PtrFromString("TraskerScreenLock")

	// HWND_MESSAGE = -3 (0xFFFFFFFFFFFFFFFD): message-only window (no visible UI)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0, 0, 0, 0, 0,
		uintptr(0xFFFFFFFFFFFFFFFD), // HWND_MESSAGE
		0, hInstance, 0,
	)

	s.mu.Lock()
	s.hwnd = hwnd
	s.mu.Unlock()

	if hwnd == 0 {
		return
	}

	procWTSRegisterSessionNotification.Call(hwnd, notifyForThisSession)

	var msg screenLockMSG
	for {
		select {
		case <-s.done:
			return
		default:
		}

		ret, _, _ := procGetMessageWLock.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if ret == 0 { // WM_QUIT
			return
		}
		procTranslateMessageLock.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageWLock.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func screenLockWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	if msg == wmWTSSessionChange {
		globalScreenLockMu.Lock()
		sl := globalScreenLock
		globalScreenLockMu.Unlock()

		if sl != nil {
			var state State
			switch wParam {
			case wtsSessionLock:
				state = Away
			case wtsSessionUnlock:
				state = Tracking
			default:
				ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
				return ret
			}
			select {
			case sl.events <- StateChange{
				State:     state,
				Timestamp: time.Now().UTC(),
			}:
			default:
			}
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}
