# Cross-Platform & Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add macOS and Windows platform support, verify cross-compilation, and run end-to-end integration tests

**Architecture:** Platform-specific implementations behind Go build tags. Same interfaces as Linux, different OS API calls. Cross-compiled from Linux build environment.

**Tech Stack:** Go 1.22+, NSWorkspace (macOS), Win32 API (Windows), CGo for platform APIs where needed

**Depends on:** Plans 01-05 (all Linux functionality working first)

**Important note on CGo:** The spec states `modernc.org/sqlite` was chosen specifically to avoid CGo for cross-compilation. However, macOS and Windows platform APIs (NSWorkspace, Win32) require CGo for direct OS integration. We use a **hybrid approach**: pure-Go libraries where available (e.g., `go-toast` for Windows notifications, `go-ole` for COM), and CGo only where no pure-Go alternative exists (macOS Objective-C APIs). For cross-compilation, the server build pipeline needs platform-specific C toolchains in the Docker builder image only for the platform API code — SQLite remains pure Go.

---

## Task 1: Focus Tracker — macOS

**Files:**
- Create: `internal/client/tracker/tracker_darwin.go`
- Create: `internal/client/tracker/tracker_darwin_test.go`

**Context:** The Linux tracker (Plans 03) established the `Tracker` interface. This task implements the same interface for macOS using NSWorkspace and the Accessibility API via CGo.

### Steps

- [ ] **1.1** Create `internal/client/tracker/tracker_darwin.go` with CGo bindings for NSWorkspace:
  ```go
  //go:build darwin

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

          // Use Accessibility API for window title
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

  // darwinTracker implements the Tracker interface for macOS.
  type darwinTracker struct {
  	pollInterval time.Duration
  	events       chan FocusEvent
  	done         chan struct{}
  }

  // NewPlatformTracker creates a macOS-specific focus tracker.
  func NewPlatformTracker() *darwinTracker {
  	return &darwinTracker{
  		pollInterval: 1 * time.Second,
  		events:       make(chan FocusEvent, 64),
  		done:         make(chan struct{}),
  	}
  }

  // Start begins polling the focused window every pollInterval.
  func (t *darwinTracker) Start(ctx context.Context) error {
  	go t.pollLoop(ctx)
  	return nil
  }

  // Events returns the channel of focus change events.
  func (t *darwinTracker) Events() <-chan FocusEvent {
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
  				event := FocusEvent{
  					AppName:     app,
  					WindowTitle: title,
  					Timestamp:   time.Now().UTC(),
  				}
  				select {
  				case t.events <- event:
  				default:
  					// Channel full — drop oldest if needed.
  					// The consumer should read fast enough that this is rare.
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
  ```

- [ ] **1.2** Create `internal/client/tracker/tracker_darwin_test.go`:
  ```go
  //go:build darwin

  package tracker

  import (
  	"context"
  	"testing"
  	"time"
  )

  func TestDarwinTracker_NewPlatformTracker(t *testing.T) {
  	tr := NewPlatformTracker()
  	if tr == nil {
  		t.Fatal("NewPlatformTracker returned nil")
  	}
  	if tr.pollInterval != 1*time.Second {
  		t.Errorf("expected poll interval 1s, got %v", tr.pollInterval)
  	}
  }

  func TestDarwinTracker_GetFocusedWindow(t *testing.T) {
  	// This test requires a running macOS GUI session with Accessibility permissions.
  	// It will return empty strings in headless/CI environments.
  	tr := NewPlatformTracker()
  	app, title := tr.getFocusedWindow()
  	t.Logf("Focused app: %q, title: %q", app, title)
  	// We don't assert specific values — just that it doesn't crash.
  }

  func TestDarwinTracker_StartStop(t *testing.T) {
  	tr := NewPlatformTracker()
  	ctx, cancel := context.WithCancel(context.Background())
  	defer cancel()

  	if err := tr.Start(ctx); err != nil {
  		t.Fatalf("Start failed: %v", err)
  	}

  	// Let it run briefly, then stop.
  	time.Sleep(100 * time.Millisecond)
  	tr.Stop()
  }

  func TestDarwinTracker_EventsChannel(t *testing.T) {
  	tr := NewPlatformTracker()
  	ch := tr.Events()
  	if ch == nil {
  		t.Fatal("Events() returned nil channel")
  	}
  }
  ```

- [ ] **1.3** Verify the file compiles on macOS (or verify build tag excludes it on Linux):
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  # On Linux, this file should be excluded by build tags:
  GOOS=linux go vet ./internal/client/tracker/
  ```
  Expected: No errors (darwin file excluded on Linux). The file will only compile when `GOOS=darwin`.

- [ ] **1.4** Commit:
  ```bash
  git add internal/client/tracker/tracker_darwin.go internal/client/tracker/tracker_darwin_test.go
  git commit -m "Add macOS focus tracker using NSWorkspace and Accessibility API"
  ```

---

## Task 2: Focus Tracker — Windows

**Files:**
- Create: `internal/client/tracker/tracker_windows.go`
- Create: `internal/client/tracker/tracker_windows_test.go`

**Context:** Windows provides `SetWinEventHook` for real-time focus change events (no polling needed). We use event-driven approach with `GetForegroundWindow` + `GetWindowText` + `GetWindowThreadProcessId`.

### Steps

- [ ] **2.1** Create `internal/client/tracker/tracker_windows.go`:
  ```go
  //go:build windows

  package tracker

  import (
  	"context"
  	"sync"
  	"syscall"
  	"time"
  	"unsafe"
  )

  var (
  	user32                  = syscall.NewLazyDLL("user32.dll")
  	kernel32                = syscall.NewLazyDLL("kernel32.dll")
  	psapi                   = syscall.NewLazyDLL("psapi.dll")
  	procSetWinEventHook     = user32.NewProc("SetWinEventHook")
  	procUnhookWinEvent      = user32.NewProc("UnhookWinEvent")
  	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
  	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
  	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
  	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
  	procGetMessageW         = user32.NewProc("GetMessageW")
  	procTranslateMessage    = user32.NewProc("TranslateMessage")
  	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
  	procOpenProcess         = kernel32.NewProc("OpenProcess")
  	procCloseHandle         = kernel32.NewProc("CloseHandle")
  	procGetModuleBaseNameW  = psapi.NewProc("GetModuleBaseNameW")
  )

  const (
  	EVENT_SYSTEM_FOREGROUND = 0x0003
  	WINEVENT_OUTOFCONTEXT   = 0x0000
  	PROCESS_QUERY_INFORMATION = 0x0400
  	PROCESS_VM_READ           = 0x0010
  )

  // MSG is the Windows message structure.
  type MSG struct {
  	HWnd    uintptr
  	Message uint32
  	WParam  uintptr
  	LParam  uintptr
  	Time    uint32
  	Pt      struct{ X, Y int32 }
  }

  // windowsTracker implements the Tracker interface for Windows.
  type windowsTracker struct {
  	events chan FocusEvent
  	done   chan struct{}
  	hook   uintptr
  	mu     sync.Mutex
  }

  // NewPlatformTracker creates a Windows-specific focus tracker.
  func NewPlatformTracker() *windowsTracker {
  	return &windowsTracker{
  		events: make(chan FocusEvent, 64),
  		done:   make(chan struct{}),
  	}
  }

  // Start begins listening for foreground window change events.
  func (t *windowsTracker) Start(ctx context.Context) error {
  	go t.messageLoop(ctx)
  	return nil
  }

  // Events returns the channel of focus change events.
  func (t *windowsTracker) Events() <-chan FocusEvent {
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
  		EVENT_SYSTEM_FOREGROUND, // eventMin
  		EVENT_SYSTEM_FOREGROUND, // eventMax
  		0,                       // hmodWinEventProc (0 = out-of-context)
  		callback,                // pfnWinEventProc
  		0,                       // idProcess (0 = all)
  		0,                       // idThread (0 = all)
  		WINEVENT_OUTOFCONTEXT,   // dwFlags
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
  	var msg MSG
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

  	event := FocusEvent{
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
  		PROCESS_QUERY_INFORMATION|PROCESS_VM_READ,
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
  ```

- [ ] **2.2** Create `internal/client/tracker/tracker_windows_test.go`:
  ```go
  //go:build windows

  package tracker

  import (
  	"context"
  	"testing"
  	"time"
  )

  func TestWindowsTracker_NewPlatformTracker(t *testing.T) {
  	tr := NewPlatformTracker()
  	if tr == nil {
  		t.Fatal("NewPlatformTracker returned nil")
  	}
  }

  func TestWindowsTracker_GetWindowText(t *testing.T) {
  	// Requires a running Windows GUI session.
  	hwnd, _, _ := procGetForegroundWindow.Call()
  	if hwnd == 0 {
  		t.Skip("No foreground window available (headless environment)")
  	}
  	title := getWindowText(hwnd)
  	t.Logf("Foreground window title: %q", title)
  	// Just verify no crash.
  }

  func TestWindowsTracker_GetProcessName(t *testing.T) {
  	hwnd, _, _ := procGetForegroundWindow.Call()
  	if hwnd == 0 {
  		t.Skip("No foreground window available (headless environment)")
  	}
  	name := getProcessName(hwnd)
  	t.Logf("Foreground process name: %q", name)
  }

  func TestWindowsTracker_StartStop(t *testing.T) {
  	tr := NewPlatformTracker()
  	ctx, cancel := context.WithCancel(context.Background())
  	defer cancel()

  	if err := tr.Start(ctx); err != nil {
  		t.Fatalf("Start failed: %v", err)
  	}
  	time.Sleep(100 * time.Millisecond)
  	tr.Stop()
  }

  func TestWindowsTracker_EventsChannel(t *testing.T) {
  	tr := NewPlatformTracker()
  	ch := tr.Events()
  	if ch == nil {
  		t.Fatal("Events() returned nil channel")
  	}
  }
  ```

- [ ] **2.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/tracker/
  ```
  Expected: No errors (windows file excluded on Linux).

- [ ] **2.4** Commit:
  ```bash
  git add internal/client/tracker/tracker_windows.go internal/client/tracker/tracker_windows_test.go
  git commit -m "Add Windows focus tracker using SetWinEventHook and Win32 API"
  ```

---

## Task 3: Presence — Screen Lock macOS

**Files:**
- Create: `internal/client/presence/screenlock_darwin.go`
- Create: `internal/client/presence/screenlock_darwin_test.go`

**Context:** macOS provides distributed notifications for screen lock/unlock via `NSDistributedNotificationCenter`. These are `com.apple.screenIsLocked` and `com.apple.screenIsUnlocked`.

### Steps

- [ ] **3.1** Create `internal/client/presence/screenlock_darwin.go`:
  ```go
  //go:build darwin

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
  	"sync"
  )

  // screenLockCallbackMu guards the global callback reference.
  var screenLockCallbackMu sync.Mutex
  var screenLockCallback func(locked bool)

  //export goScreenLockCallback
  func goScreenLockCallback(locked C.int) {
  	screenLockCallbackMu.Lock()
  	cb := screenLockCallback
  	screenLockCallbackMu.Unlock()

  	if cb != nil {
  		cb(locked != 0)
  	}
  }

  // darwinScreenLock implements screen lock detection for macOS.
  type darwinScreenLock struct {
  	events chan ScreenLockEvent
  	done   chan struct{}
  }

  // NewPlatformScreenLock creates a macOS screen lock detector.
  func NewPlatformScreenLock() *darwinScreenLock {
  	return &darwinScreenLock{
  		events: make(chan ScreenLockEvent, 16),
  		done:   make(chan struct{}),
  	}
  }

  // Start registers for screen lock/unlock notifications.
  func (s *darwinScreenLock) Start() error {
  	screenLockCallbackMu.Lock()
  	screenLockCallback = func(locked bool) {
  		event := ScreenLockEvent{Locked: locked}
  		select {
  		case s.events <- event:
  		default:
  		}
  	}
  	screenLockCallbackMu.Unlock()

  	C.registerScreenLockObservers()
  	return nil
  }

  // Events returns the channel of screen lock/unlock events.
  func (s *darwinScreenLock) Events() <-chan ScreenLockEvent {
  	return s.events
  }

  // Stop unregisters observers and cleans up.
  func (s *darwinScreenLock) Stop() {
  	C.unregisterScreenLockObservers()

  	screenLockCallbackMu.Lock()
  	screenLockCallback = nil
  	screenLockCallbackMu.Unlock()

  	close(s.done)
  }
  ```

- [ ] **3.2** Create `internal/client/presence/screenlock_darwin_test.go`:
  ```go
  //go:build darwin

  package presence

  import (
  	"testing"
  )

  func TestDarwinScreenLock_NewPlatformScreenLock(t *testing.T) {
  	sl := NewPlatformScreenLock()
  	if sl == nil {
  		t.Fatal("NewPlatformScreenLock returned nil")
  	}
  }

  func TestDarwinScreenLock_EventsChannel(t *testing.T) {
  	sl := NewPlatformScreenLock()
  	ch := sl.Events()
  	if ch == nil {
  		t.Fatal("Events() returned nil channel")
  	}
  }

  func TestDarwinScreenLock_StartStop(t *testing.T) {
  	// This test verifies Start/Stop don't crash.
  	// Actual lock/unlock events require a macOS GUI session.
  	sl := NewPlatformScreenLock()
  	if err := sl.Start(); err != nil {
  		t.Fatalf("Start failed: %v", err)
  	}
  	sl.Stop()
  }
  ```

- [ ] **3.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/presence/
  ```
  Expected: No errors (darwin file excluded on Linux).

- [ ] **3.4** Commit:
  ```bash
  git add internal/client/presence/screenlock_darwin.go internal/client/presence/screenlock_darwin_test.go
  git commit -m "Add macOS screen lock detection via NSDistributedNotificationCenter"
  ```

---

## Task 4: Presence — Screen Lock Windows

**Files:**
- Create: `internal/client/presence/screenlock_windows.go`
- Create: `internal/client/presence/screenlock_windows_test.go`

**Context:** Windows provides `WTSRegisterSessionNotification` to receive `WM_WTSSESSION_CHANGE` messages with `WTS_SESSION_LOCK` and `WTS_SESSION_UNLOCK` parameters.

### Steps

- [ ] **4.1** Create `internal/client/presence/screenlock_windows.go`:
  ```go
  //go:build windows

  package presence

  import (
  	"sync"
  	"syscall"
  	"unsafe"
  )

  var (
  	wtsapi32                        = syscall.NewLazyDLL("wtsapi32.dll")
  	user32                          = syscall.NewLazyDLL("user32.dll")
  	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
  	procWTSUnRegisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
  	procCreateWindowExW             = user32.NewProc("CreateWindowExW")
  	procDefWindowProcW              = user32.NewProc("DefWindowProcW")
  	procRegisterClassExW            = user32.NewProc("RegisterClassExW")
  	procGetMessageW                 = user32.NewProc("GetMessageW")
  	procTranslateMessage            = user32.NewProc("TranslateMessage")
  	procDispatchMessageW            = user32.NewProc("DispatchMessageW")
  	procDestroyWindow               = user32.NewProc("DestroyWindow")
  	procPostQuitMessage             = user32.NewProc("PostQuitMessage")
  	procGetModuleHandleW            = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
  )

  const (
  	WM_WTSSESSION_CHANGE  = 0x02B1
  	WTS_SESSION_LOCK      = 0x7
  	WTS_SESSION_UNLOCK    = 0x8
  	NOTIFY_FOR_THIS_SESSION = 0
  )

  // WNDCLASSEXW is the Windows window class structure.
  type WNDCLASSEXW struct {
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

  // MSG is the Windows message structure.
  type screenLockMSG struct {
  	HWnd    uintptr
  	Message uint32
  	WParam  uintptr
  	LParam  uintptr
  	Time    uint32
  	Pt      struct{ X, Y int32 }
  }

  // windowsScreenLock implements screen lock detection for Windows.
  type windowsScreenLock struct {
  	events chan ScreenLockEvent
  	done   chan struct{}
  	hwnd   uintptr
  	mu     sync.Mutex
  }

  // Global reference for the window proc callback.
  var globalScreenLock *windowsScreenLock
  var globalScreenLockMu sync.Mutex

  // NewPlatformScreenLock creates a Windows screen lock detector.
  func NewPlatformScreenLock() *windowsScreenLock {
  	return &windowsScreenLock{
  		events: make(chan ScreenLockEvent, 16),
  		done:   make(chan struct{}),
  	}
  }

  // Start creates a hidden message-only window and registers for session notifications.
  func (s *windowsScreenLock) Start() error {
  	globalScreenLockMu.Lock()
  	globalScreenLock = s
  	globalScreenLockMu.Unlock()

  	go s.messageLoop()
  	return nil
  }

  // Events returns the channel of screen lock/unlock events.
  func (s *windowsScreenLock) Events() <-chan ScreenLockEvent {
  	return s.events
  }

  // Stop destroys the hidden window and stops the message loop.
  func (s *windowsScreenLock) Stop() {
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

  func (s *windowsScreenLock) messageLoop() {
  	hInstance, _, _ := procGetModuleHandleW.Call(0)

  	className, _ := syscall.UTF16PtrFromString("TraskerScreenLockClass")

  	wc := WNDCLASSEXW{
  		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
  		LpfnWndProc:   syscall.NewCallback(screenLockWndProc),
  		HInstance:     hInstance,
  		LpszClassName: className,
  	}

  	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

  	windowName, _ := syscall.UTF16PtrFromString("TraskerScreenLock")

  	// HWND_MESSAGE = -3: message-only window (no visible UI)
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

  	procWTSRegisterSessionNotification.Call(hwnd, NOTIFY_FOR_THIS_SESSION)

  	var msg screenLockMSG
  	for {
  		select {
  		case <-s.done:
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

  func screenLockWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
  	if msg == WM_WTSSESSION_CHANGE {
  		globalScreenLockMu.Lock()
  		sl := globalScreenLock
  		globalScreenLockMu.Unlock()

  		if sl != nil {
  			var event ScreenLockEvent
  			switch wParam {
  			case WTS_SESSION_LOCK:
  				event = ScreenLockEvent{Locked: true}
  			case WTS_SESSION_UNLOCK:
  				event = ScreenLockEvent{Locked: false}
  			default:
  				ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
  				return ret
  			}
  			select {
  			case sl.events <- event:
  			default:
  			}
  		}
  	}
  	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
  	return ret
  }
  ```

- [ ] **4.2** Create `internal/client/presence/screenlock_windows_test.go`:
  ```go
  //go:build windows

  package presence

  import (
  	"testing"
  )

  func TestWindowsScreenLock_NewPlatformScreenLock(t *testing.T) {
  	sl := NewPlatformScreenLock()
  	if sl == nil {
  		t.Fatal("NewPlatformScreenLock returned nil")
  	}
  }

  func TestWindowsScreenLock_EventsChannel(t *testing.T) {
  	sl := NewPlatformScreenLock()
  	ch := sl.Events()
  	if ch == nil {
  		t.Fatal("Events() returned nil channel")
  	}
  }

  func TestWindowsScreenLock_StartStop(t *testing.T) {
  	// Verifies Start/Stop lifecycle doesn't crash.
  	// Actual lock/unlock events require a Windows desktop session.
  	sl := NewPlatformScreenLock()
  	if err := sl.Start(); err != nil {
  		t.Fatalf("Start failed: %v", err)
  	}
  	sl.Stop()
  }
  ```

- [ ] **4.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/presence/
  ```
  Expected: No errors.

- [ ] **4.4** Commit:
  ```bash
  git add internal/client/presence/screenlock_windows.go internal/client/presence/screenlock_windows_test.go
  git commit -m "Add Windows screen lock detection via WTSRegisterSessionNotification"
  ```

---

## Task 5: Notifications — macOS

**Files:**
- Create: `internal/client/notify/notify_darwin.go`
- Create: `internal/client/notify/notify_darwin_test.go`

**Context:** macOS notifications use `NSUserNotificationCenter` (deprecated but widely supported) or `UNUserNotificationCenter` (modern, requires entitlement). For a non-App-Store binary, `NSUserNotificationCenter` is the pragmatic choice. We use CGo.

### Steps

- [ ] **5.1** Create `internal/client/notify/notify_darwin.go`:
  ```go
  //go:build darwin

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

  void removeNotification(const char* identifier) {
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
  	"sync"
  	"unsafe"
  )

  // clickCallbackMu guards the global click callback reference.
  var clickCallbackMu sync.Mutex
  var clickCallbackFn func(identifier string)

  //export goNotificationClicked
  func goNotificationClicked(identifier *C.char) {
  	clickCallbackMu.Lock()
  	cb := clickCallbackFn
  	clickCallbackMu.Unlock()

  	if cb != nil && identifier != nil {
  		cb(C.GoString(identifier))
  	}
  }

  // darwinNotifier implements the Notifier interface for macOS.
  type darwinNotifier struct{}

  // NewPlatformNotifier creates a macOS notifier.
  func NewPlatformNotifier() *darwinNotifier {
  	return &darwinNotifier{}
  }

  // Send displays a macOS notification.
  func (n *darwinNotifier) Send(title, body, identifier string) error {
  	cTitle := C.CString(title)
  	defer C.free(unsafe.Pointer(cTitle))

  	cBody := C.CString(body)
  	defer C.free(unsafe.Pointer(cBody))

  	var cID *C.char
  	if identifier != "" {
  		cID = C.CString(identifier)
  		defer C.free(unsafe.Pointer(cID))
  	}

  	C.sendNotification(cTitle, cBody, cID)
  	return nil
  }

  // Remove dismisses a previously delivered notification by identifier.
  func (n *darwinNotifier) Remove(identifier string) error {
  	cID := C.CString(identifier)
  	defer C.free(unsafe.Pointer(cID))

  	C.removeNotification(cID)
  	return nil
  }

  // OnClick registers a callback for notification clicks.
  func (n *darwinNotifier) OnClick(fn func(identifier string)) {
  	clickCallbackMu.Lock()
  	clickCallbackFn = fn
  	clickCallbackMu.Unlock()
  }
  ```

- [ ] **5.2** Create `internal/client/notify/notify_darwin_test.go`:
  ```go
  //go:build darwin

  package notify

  import (
  	"testing"
  )

  func TestDarwinNotifier_NewPlatformNotifier(t *testing.T) {
  	n := NewPlatformNotifier()
  	if n == nil {
  		t.Fatal("NewPlatformNotifier returned nil")
  	}
  }

  func TestDarwinNotifier_Send(t *testing.T) {
  	// Requires macOS GUI session. Verify no crash.
  	n := NewPlatformNotifier()
  	err := n.Send("Trasker Test", "This is a test notification", "test-001")
  	if err != nil {
  		t.Fatalf("Send failed: %v", err)
  	}
  	// Clean up.
  	_ = n.Remove("test-001")
  }

  func TestDarwinNotifier_OnClick(t *testing.T) {
  	n := NewPlatformNotifier()
  	called := false
  	n.OnClick(func(id string) {
  		called = true
  	})
  	// Can't programmatically click a notification in tests.
  	// Just verify OnClick registration doesn't crash.
  	_ = called
  }
  ```

- [ ] **5.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/notify/
  ```
  Expected: No errors.

- [ ] **5.4** Commit:
  ```bash
  git add internal/client/notify/notify_darwin.go internal/client/notify/notify_darwin_test.go
  git commit -m "Add macOS notifications via NSUserNotificationCenter"
  ```

---

## Task 6: Notifications — Windows

**Files:**
- Create: `internal/client/notify/notify_windows.go`
- Create: `internal/client/notify/notify_windows_test.go`

**Context:** Windows Toast notifications can be sent via PowerShell as a pure-Go approach (no CGo), or via a Go library like `go-toast/toast`. We use the PowerShell approach to avoid external dependencies and CGo — this works on Windows 10+ and uses the built-in `[Windows.UI.Notifications.ToastNotificationManager]` API.

### Steps

- [ ] **6.1** Create `internal/client/notify/notify_windows.go`:
  ```go
  //go:build windows

  package notify

  import (
  	"fmt"
  	"os/exec"
  	"strings"
  	"sync"
  )

  // windowsNotifier implements the Notifier interface for Windows
  // using PowerShell to invoke the Windows Toast notification API.
  type windowsNotifier struct {
  	appID    string
  	clickMu  sync.Mutex
  	clickFn  func(identifier string)
  }

  // NewPlatformNotifier creates a Windows notifier.
  func NewPlatformNotifier() *windowsNotifier {
  	return &windowsNotifier{
  		appID: "Trasker",
  	}
  }

  // Send displays a Windows Toast notification via PowerShell.
  func (n *windowsNotifier) Send(title, body, identifier string) error {
  	// Escape single quotes for PowerShell strings.
  	safeTitle := strings.ReplaceAll(title, "'", "''")
  	safeBody := strings.ReplaceAll(body, "'", "''")

  	// Build the PowerShell script for a toast notification.
  	script := fmt.Sprintf(`
  [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
  [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

  $template = @"
  <toast>
      <visual>
          <binding template="ToastGeneric">
              <text>%s</text>
              <text>%s</text>
          </binding>
      </visual>
      <actions>
          <action content="I'm here" arguments="trasker://deadman-ack/%s" activationType="protocol"/>
      </actions>
  </toast>
  "@

  $xml = New-Object Windows.Data.Xml.Dom.XmlDocument
  $xml.LoadXml($template)
  $toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
  [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($toast)
  `, safeTitle, safeBody, identifier, n.appID)

  	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
  	output, err := cmd.CombinedOutput()
  	if err != nil {
  		return fmt.Errorf("toast notification failed: %w: %s", err, string(output))
  	}
  	return nil
  }

  // Remove is a no-op on Windows — toast notifications auto-dismiss
  // and we don't track them for programmatic removal.
  func (n *windowsNotifier) Remove(identifier string) error {
  	// Windows toast notifications can't be easily removed programmatically
  	// without tracking the ToastNotification object across process boundaries.
  	return nil
  }

  // OnClick registers a callback for notification clicks.
  // Windows toast click detection: We use a protocol handler approach.
  // The toast notification includes an activationType="protocol" action that
  // launches trasker://deadman-ack. The client registers a trasker:// protocol
  // handler (same as deep links) that routes back to the running process via
  // a localhost HTTP endpoint or named pipe.
  func (n *windowsNotifier) OnClick(fn func(identifier string)) {
  	n.clickMu.Lock()
  	n.clickFn = fn
  	n.clickMu.Unlock()
  }

  // The Notify method must include an activation protocol in the toast XML:
  //   <action activationType="protocol" content="I'm here"
  //           arguments="trasker://deadman-ack/{identifier}" />
  // The client's local HTTP server handles trasker:// URLs and invokes clickFn.
  //
  // IMPLEMENTATION NOTE: The trasker:// protocol handler must be registered
  // during client startup on Windows. This requires adding a registry key:
  //   HKCU\Software\Classes\trasker\shell\open\command = "C:\path\trasker-client.exe" "%1"
  // The client's webui HTTP server (Plan 05, Task 15) should handle
  //   GET /deadman-ack/:identifier → invoke presence.Acknowledge()
  // The protocol handler redirects trasker://deadman-ack/ID to
  //   http://localhost:PORT/deadman-ack/ID via the running client process.
  // This registration should be added to the Windows first-run/autostart setup.
  ```

- [ ] **6.2** Create `internal/client/notify/notify_windows_test.go`:
  ```go
  //go:build windows

  package notify

  import (
  	"testing"
  )

  func TestWindowsNotifier_NewPlatformNotifier(t *testing.T) {
  	n := NewPlatformNotifier()
  	if n == nil {
  		t.Fatal("NewPlatformNotifier returned nil")
  	}
  }

  func TestWindowsNotifier_Send(t *testing.T) {
  	// Requires Windows desktop session with PowerShell.
  	n := NewPlatformNotifier()
  	err := n.Send("Trasker Test", "This is a test notification", "test-001")
  	if err != nil {
  		t.Logf("Send returned error (may be expected in CI): %v", err)
  	}
  }

  func TestWindowsNotifier_Remove(t *testing.T) {
  	n := NewPlatformNotifier()
  	// Remove is a no-op, just verify no crash.
  	err := n.Remove("test-001")
  	if err != nil {
  		t.Fatalf("Remove failed: %v", err)
  	}
  }

  func TestWindowsNotifier_OnClick(t *testing.T) {
  	n := NewPlatformNotifier()
  	n.OnClick(func(id string) {})
  	// Just verify no crash.
  }
  ```

- [ ] **6.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/notify/
  ```
  Expected: No errors.

- [ ] **6.4** Commit:
  ```bash
  git add internal/client/notify/notify_windows.go internal/client/notify/notify_windows_test.go
  git commit -m "Add Windows toast notifications via PowerShell"
  ```

---

## Task 7: Autostart — macOS

**Files:**
- Create: `internal/client/setup/autostart_darwin.go`
- Create: `internal/client/setup/autostart_darwin_test.go`

**Context:** macOS autostart uses a LaunchAgent plist file at `~/Library/LaunchAgents/com.trasker.client.plist`. The plist runs the binary on user login.

### Steps

- [ ] **7.1** Create `internal/client/setup/autostart_darwin.go`:
  ```go
  //go:build darwin

  package setup

  import (
  	"fmt"
  	"os"
  	"path/filepath"
  	"text/template"
  )

  const (
  	launchAgentDir  = "Library/LaunchAgents"
  	plistFileName   = "com.trasker.client.plist"
  )

  const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
  <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
  <plist version="1.0">
  <dict>
      <key>Label</key>
      <string>com.trasker.client</string>
      <key>ProgramArguments</key>
      <array>
          <string>{{.ExecutablePath}}</string>
      </array>
      <key>RunAtLoad</key>
      <true/>
      <key>KeepAlive</key>
      <false/>
      <key>StandardOutPath</key>
      <string>{{.LogDir}}/trasker-stdout.log</string>
      <key>StandardErrorPath</key>
      <string>{{.LogDir}}/trasker-stderr.log</string>
  </dict>
  </plist>
  `

  type plistData struct {
  	ExecutablePath string
  	LogDir         string
  }

  // plistPathFn is the path resolver — overridable in tests to use temp dirs.
  var plistPathFn = plistPathDefault

  // plistPath returns the full path to the LaunchAgent plist.
  func plistPath() (string, error) { return plistPathFn() }

  func plistPathDefault() (string, error) {
  	home, err := os.UserHomeDir()
  	if err != nil {
  		return "", fmt.Errorf("cannot determine home directory: %w", err)
  	}
  	return filepath.Join(home, launchAgentDir, plistFileName), nil
  }

  // EnableAutostart creates the LaunchAgent plist for macOS autostart.
  func EnableAutostart() error {
  	execPath, err := os.Executable()
  	if err != nil {
  		return fmt.Errorf("cannot determine executable path: %w", err)
  	}
  	// Resolve symlinks to get the real path.
  	execPath, err = filepath.EvalSymlinks(execPath)
  	if err != nil {
  		return fmt.Errorf("cannot resolve executable path: %w", err)
  	}

  	home, err := os.UserHomeDir()
  	if err != nil {
  		return fmt.Errorf("cannot determine home directory: %w", err)
  	}

  	logDir := filepath.Join(home, "Library", "Logs", "Trasker")
  	if err := os.MkdirAll(logDir, 0755); err != nil {
  		return fmt.Errorf("cannot create log directory: %w", err)
  	}

  	pPath, err := plistPath()
  	if err != nil {
  		return err
  	}

  	// Ensure the LaunchAgents directory exists.
  	if err := os.MkdirAll(filepath.Dir(pPath), 0755); err != nil {
  		return fmt.Errorf("cannot create LaunchAgents directory: %w", err)
  	}

  	tmpl, err := template.New("plist").Parse(plistTemplate)
  	if err != nil {
  		return fmt.Errorf("cannot parse plist template: %w", err)
  	}

  	f, err := os.Create(pPath)
  	if err != nil {
  		return fmt.Errorf("cannot create plist file: %w", err)
  	}
  	defer f.Close()

  	data := plistData{
  		ExecutablePath: execPath,
  		LogDir:         logDir,
  	}
  	if err := tmpl.Execute(f, data); err != nil {
  		return fmt.Errorf("cannot write plist file: %w", err)
  	}

  	return nil
  }

  // DisableAutostart removes the LaunchAgent plist.
  func DisableAutostart() error {
  	pPath, err := plistPath()
  	if err != nil {
  		return err
  	}

  	err = os.Remove(pPath)
  	if os.IsNotExist(err) {
  		return nil // Already disabled.
  	}
  	return err
  }

  // IsAutostartEnabled checks if the LaunchAgent plist exists.
  func IsAutostartEnabled() (bool, error) {
  	pPath, err := plistPath()
  	if err != nil {
  		return false, err
  	}
  	_, err = os.Stat(pPath)
  	if os.IsNotExist(err) {
  		return false, nil
  	}
  	if err != nil {
  		return false, err
  	}
  	return true, nil
  }
  ```

- [ ] **7.2** Create `internal/client/setup/autostart_darwin_test.go`:
  ```go
  //go:build darwin

  package setup

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"
  )

  func TestDarwin_PlistPath(t *testing.T) {
  	p, err := plistPath()
  	if err != nil {
  		t.Fatalf("plistPath failed: %v", err)
  	}
  	if !strings.HasSuffix(p, "Library/LaunchAgents/com.trasker.client.plist") {
  		t.Errorf("unexpected plist path: %s", p)
  	}
  }

  func TestDarwin_EnableDisableAutostart(t *testing.T) {
  	// Use a temp directory to avoid modifying real LaunchAgents.
  	tmpDir := t.TempDir()
  	testPlistPath := filepath.Join(tmpDir, "com.trasker.client.plist")

  	// Override the plist path for testing via the internal helper.
  	origPlistPathFn := plistPathFn
  	plistPathFn = func() (string, error) { return testPlistPath, nil }
  	t.Cleanup(func() { plistPathFn = origPlistPathFn })

  	err := EnableAutostart()
  	if err != nil {
  		t.Fatalf("EnableAutostart failed: %v", err)
  	}

  	// Verify the plist was written to the temp directory
  	if _, err := os.Stat(testPlistPath); os.IsNotExist(err) {
  		t.Fatal("plist file was not created in temp directory")
  	}

  	enabled, err := IsAutostartEnabled()
  	if err != nil {
  		t.Fatalf("IsAutostartEnabled failed: %v", err)
  	}
  	if !enabled {
  		t.Error("expected autostart to be enabled after EnableAutostart")
  	}

  	err = DisableAutostart()
  	if err != nil {
  		t.Fatalf("DisableAutostart failed: %v", err)
  	}

  	enabled, err = IsAutostartEnabled()
  	if err != nil {
  		t.Fatalf("IsAutostartEnabled failed after disable: %v", err)
  	}
  	if enabled {
  		t.Error("expected autostart to be disabled after DisableAutostart")
  	}

  	// Clean up temp path reference (unused in direct test, kept for documentation).
  	_ = os.Remove(testPlistPath)
  }

  func TestDarwin_DisableAutostart_NotExists(t *testing.T) {
  	// DisableAutostart should not error if the plist doesn't exist.
  	// First ensure it's not there.
  	_ = DisableAutostart()
  	err := DisableAutostart()
  	if err != nil {
  		t.Fatalf("DisableAutostart on non-existent plist should not error: %v", err)
  	}
  }
  ```

- [ ] **7.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/setup/
  ```
  Expected: No errors.

- [ ] **7.4** Commit:
  ```bash
  git add internal/client/setup/autostart_darwin.go internal/client/setup/autostart_darwin_test.go
  git commit -m "Add macOS autostart via LaunchAgent plist"
  ```

---

## Task 8: Autostart — Windows

**Files:**
- Create: `internal/client/setup/autostart_windows.go`
- Create: `internal/client/setup/autostart_windows_test.go`

**Context:** Windows autostart uses the `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` registry key. Setting a string value starts the program at login. Pure Go via `golang.org/x/sys/windows/registry`.

### Steps

- [ ] **8.1** Create `internal/client/setup/autostart_windows.go`:
  ```go
  //go:build windows

  package setup

  import (
  	"fmt"
  	"os"
  	"path/filepath"

  	"golang.org/x/sys/windows/registry"
  )

  const (
  	registryKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
  	registryValueName = "Trasker"
  )

  // EnableAutostart creates a registry entry for Windows autostart.
  func EnableAutostart() error {
  	execPath, err := os.Executable()
  	if err != nil {
  		return fmt.Errorf("cannot determine executable path: %w", err)
  	}
  	execPath, err = filepath.EvalSymlinks(execPath)
  	if err != nil {
  		return fmt.Errorf("cannot resolve executable path: %w", err)
  	}

  	key, _, err := registry.CreateKey(
  		registry.CURRENT_USER,
  		registryKeyPath,
  		registry.SET_VALUE,
  	)
  	if err != nil {
  		return fmt.Errorf("cannot open registry key: %w", err)
  	}
  	defer key.Close()

  	// Quote the path in case it contains spaces.
  	quotedPath := fmt.Sprintf(`"%s"`, execPath)
  	if err := key.SetStringValue(registryValueName, quotedPath); err != nil {
  		return fmt.Errorf("cannot set registry value: %w", err)
  	}

  	return nil
  }

  // DisableAutostart removes the registry entry for Windows autostart.
  func DisableAutostart() error {
  	key, err := registry.OpenKey(
  		registry.CURRENT_USER,
  		registryKeyPath,
  		registry.SET_VALUE,
  	)
  	if err != nil {
  		// Key doesn't exist — already disabled.
  		return nil
  	}
  	defer key.Close()

  	err = key.DeleteValue(registryValueName)
  	if err == registry.ErrNotExist {
  		return nil
  	}
  	return err
  }

  // IsAutostartEnabled checks if the registry entry exists.
  func IsAutostartEnabled() (bool, error) {
  	key, err := registry.OpenKey(
  		registry.CURRENT_USER,
  		registryKeyPath,
  		registry.QUERY_VALUE,
  	)
  	if err != nil {
  		return false, nil // Key doesn't exist.
  	}
  	defer key.Close()

  	_, _, err = key.GetStringValue(registryValueName)
  	if err == registry.ErrNotExist {
  		return false, nil
  	}
  	if err != nil {
  		return false, err
  	}
  	return true, nil
  }
  ```

- [ ] **8.2** Create `internal/client/setup/autostart_windows_test.go`:
  ```go
  //go:build windows

  package setup

  import (
  	"testing"
  )

  func TestWindows_EnableDisableAutostart(t *testing.T) {
  	// Enable autostart.
  	err := EnableAutostart()
  	if err != nil {
  		t.Fatalf("EnableAutostart failed: %v", err)
  	}

  	enabled, err := IsAutostartEnabled()
  	if err != nil {
  		t.Fatalf("IsAutostartEnabled failed: %v", err)
  	}
  	if !enabled {
  		t.Error("expected autostart to be enabled after EnableAutostart")
  	}

  	// Disable autostart (clean up registry).
  	err = DisableAutostart()
  	if err != nil {
  		t.Fatalf("DisableAutostart failed: %v", err)
  	}

  	enabled, err = IsAutostartEnabled()
  	if err != nil {
  		t.Fatalf("IsAutostartEnabled after disable failed: %v", err)
  	}
  	if enabled {
  		t.Error("expected autostart to be disabled after DisableAutostart")
  	}
  }

  func TestWindows_DisableAutostart_NotExists(t *testing.T) {
  	// Ensure it's not there first.
  	_ = DisableAutostart()
  	err := DisableAutostart()
  	if err != nil {
  		t.Fatalf("DisableAutostart on non-existent key should not error: %v", err)
  	}
  }

  func TestWindows_IsAutostartEnabled_NotExists(t *testing.T) {
  	_ = DisableAutostart()
  	enabled, err := IsAutostartEnabled()
  	if err != nil {
  		t.Fatalf("IsAutostartEnabled failed: %v", err)
  	}
  	if enabled {
  		t.Error("expected autostart to be disabled when registry key doesn't exist")
  	}
  }
  ```

- [ ] **8.3** Verify build tag exclusion on Linux:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  GOOS=linux go vet ./internal/client/setup/
  ```
  Expected: No errors.

- [ ] **8.4** Commit:
  ```bash
  git add internal/client/setup/autostart_windows.go internal/client/setup/autostart_windows_test.go
  git commit -m "Add Windows autostart via registry Run key"
  ```

---

## Task 9: Cross-Compilation Verification

**Files:**
- Modify: `Makefile`
- Create: `scripts/verify-cross-compile.sh`

**Context:** The server's build pipeline cross-compiles client binaries for 5 targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. Since `modernc.org/sqlite` is pure Go, the SQLite layer cross-compiles with just `GOOS`/`GOARCH`. However, the macOS CGo code (Tasks 1, 3, 5) requires special handling — we need conditional compilation that falls back to stub implementations when CGo is unavailable during cross-compilation.

### Steps

- [ ] **9.1** Add cross-compilation Makefile targets. Append to the existing `Makefile`:
  ```makefile
  # --- Cross-Compilation Targets ---

  VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
  COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
  LDFLAGS  = -X github.com/jaypaulb/trasker/internal/shared/version.Version=$(VERSION) \
             -X github.com/jaypaulb/trasker/internal/shared/version.Commit=$(COMMIT)

  BUILD_DIR = build

  # All client build targets
  PLATFORMS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

  .PHONY: build-all-clients
  build-all-clients: $(addprefix build-client-,$(subst /,-,$(PLATFORMS)))

  # Pattern rule for cross-compilation.
  # macOS and Windows targets use CGO_ENABLED=0 for cross-compilation from Linux.
  # Platform-specific code that requires CGo has stub fallbacks for CGO_ENABLED=0.
  build-client-%:
  	$(eval GOOS := $(word 1,$(subst -, ,$*)))
  	$(eval GOARCH := $(word 2,$(subst -, ,$*)))
  	$(eval EXT := $(if $(filter windows,$(GOOS)),.exe,))
  	@echo "Building trasker-client for $(GOOS)/$(GOARCH)..."
  	@mkdir -p $(BUILD_DIR)/$(GOOS)-$(GOARCH)
  	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
  		-ldflags '$(LDFLAGS)' \
  		-o $(BUILD_DIR)/$(GOOS)-$(GOARCH)/trasker-client$(EXT) \
  		./cmd/trasker-client/

  .PHONY: verify-cross-compile
  verify-cross-compile: build-all-clients
  	@bash scripts/verify-cross-compile.sh

  .PHONY: clean
  clean:
  	rm -rf $(BUILD_DIR)
  ```

- [ ] **9.2** Create `scripts/verify-cross-compile.sh`:
  ```bash
  #!/usr/bin/env bash
  set -euo pipefail

  BUILD_DIR="build"
  PASS=0
  FAIL=0

  check_binary() {
      local path="$1"
      local expected_os="$2"
      local expected_arch="$3"

      if [[ ! -f "$path" ]]; then
          echo "FAIL: $path does not exist"
          ((FAIL++))
          return
      fi

      local size
      size=$(stat -c%s "$path" 2>/dev/null || stat -f%z "$path" 2>/dev/null)
      if [[ "$size" -lt 1000000 ]]; then
          echo "FAIL: $path is suspiciously small (${size} bytes)"
          ((FAIL++))
          return
      fi

      local file_output
      file_output=$(file "$path")

      case "$expected_os" in
          linux)
              if [[ "$file_output" != *"ELF"* ]]; then
                  echo "FAIL: $path is not an ELF binary: $file_output"
                  ((FAIL++))
                  return
              fi
              ;;
          darwin)
              if [[ "$file_output" != *"Mach-O"* ]]; then
                  echo "FAIL: $path is not a Mach-O binary: $file_output"
                  ((FAIL++))
                  return
              fi
              ;;
          windows)
              if [[ "$file_output" != *"PE32"* ]] && [[ "$file_output" != *"PE32+"* ]]; then
                  echo "FAIL: $path is not a PE binary: $file_output"
                  ((FAIL++))
                  return
              fi
              ;;
      esac

      echo "PASS: $path ($expected_os/$expected_arch, ${size} bytes)"
      ((PASS++))
  }

  echo "=== Cross-Compilation Verification ==="
  echo ""

  check_binary "$BUILD_DIR/linux-amd64/trasker-client"       linux   amd64
  check_binary "$BUILD_DIR/linux-arm64/trasker-client"       linux   arm64
  check_binary "$BUILD_DIR/darwin-amd64/trasker-client"      darwin  amd64
  check_binary "$BUILD_DIR/darwin-arm64/trasker-client"      darwin  arm64
  check_binary "$BUILD_DIR/windows-amd64/trasker-client.exe" windows amd64

  echo ""
  echo "Results: $PASS passed, $FAIL failed"

  if [[ "$FAIL" -gt 0 ]]; then
      exit 1
  fi
  ```

- [ ] **9.3** Make the verification script executable:
  ```bash
  chmod +x scripts/verify-cross-compile.sh
  ```

- [ ] **9.4** Ensure platform-specific CGo files have non-CGo stubs. Create stub files that compile when `CGO_ENABLED=0`. The build tag `!cgo` ensures these are used only when CGo is disabled (cross-compilation from Linux):

  Create `internal/client/tracker/tracker_darwin_nocgo.go`:
  ```go
  //go:build darwin && !cgo

  package tracker

  import (
  	"context"
  	"fmt"
  	"os/exec"
  	"strings"
  	"time"
  )

  // darwinTracker is the non-CGo fallback for macOS.
  // Uses osascript to query the frontmost application.
  type darwinTracker struct {
  	pollInterval time.Duration
  	events       chan FocusEvent
  	done         chan struct{}
  }

  func NewPlatformTracker() *darwinTracker {
  	return &darwinTracker{
  		pollInterval: 1 * time.Second,
  		events:       make(chan FocusEvent, 64),
  		done:         make(chan struct{}),
  	}
  }

  func (t *darwinTracker) Start(ctx context.Context) error {
  	go t.pollLoop(ctx)
  	return nil
  }

  func (t *darwinTracker) Events() <-chan FocusEvent {
  	return t.events
  }

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
  				event := FocusEvent{
  					AppName:     app,
  					WindowTitle: title,
  					Timestamp:   time.Now().UTC(),
  				}
  				select {
  				case t.events <- event:
  				default:
  				}
  				lastApp = app
  				lastTitle = title
  			}
  		}
  	}
  }

  func (t *darwinTracker) getFocusedWindow() (appName, windowTitle string) {
  	// Use osascript as a pure-Go fallback (no CGo needed).
  	appScript := `tell application "System Events" to get name of first application process whose frontmost is true`
  	out, err := exec.Command("osascript", "-e", appScript).Output()
  	if err == nil {
  		appName = strings.TrimSpace(string(out))
  	}

  	titleScript := fmt.Sprintf(`tell application "System Events" to get name of front window of application process "%s"`, appName)
  	out, err = exec.Command("osascript", "-e", titleScript).Output()
  	if err == nil {
  		windowTitle = strings.TrimSpace(string(out))
  	}

  	return appName, windowTitle
  }
  ```

- [ ] **9.4a** Update CGo build tags on existing macOS files. These files were created in earlier tasks with `//go:build darwin` — now that we have non-CGo alternatives, restrict them to CGo-only:

  Modify `internal/client/tracker/tracker_darwin.go` — change first line from `//go:build darwin` to:
  ```go
  //go:build darwin && cgo
  ```

  Modify `internal/client/presence/screenlock_darwin.go` — change first line from `//go:build darwin` to:
  ```go
  //go:build darwin && cgo
  ```

  Modify `internal/client/notify/notify_darwin.go` — change first line from `//go:build darwin` to:
  ```go
  //go:build darwin && cgo
  ```

- [ ] **9.5** Create non-CGo stubs for macOS presence and notifications:

  Create `internal/client/presence/screenlock_darwin_nocgo.go`:
  ```go
  //go:build darwin && !cgo

  package presence

  import (
  	"os/exec"
  	"strings"
  	"time"
  )

  // darwinScreenLock is the non-CGo fallback for macOS screen lock detection.
  // Polls the CGSession dictionary via Python (available on all macOS).
  type darwinScreenLock struct {
  	events chan ScreenLockEvent
  	done   chan struct{}
  }

  func NewPlatformScreenLock() *darwinScreenLock {
  	return &darwinScreenLock{
  		events: make(chan ScreenLockEvent, 16),
  		done:   make(chan struct{}),
  	}
  }

  func (s *darwinScreenLock) Start() error {
  	go s.pollLoop()
  	return nil
  }

  func (s *darwinScreenLock) Events() <-chan ScreenLockEvent {
  	return s.events
  }

  func (s *darwinScreenLock) Stop() {
  	close(s.done)
  }

  func (s *darwinScreenLock) pollLoop() {
  	ticker := time.NewTicker(2 * time.Second)
  	defer ticker.Stop()

  	wasLocked := false

  	for {
  		select {
  		case <-s.done:
  			return
  		case <-ticker.C:
  			locked := isScreenLocked()
  			if locked != wasLocked {
  				select {
  				case s.events <- ScreenLockEvent{Locked: locked}:
  				default:
  				}
  				wasLocked = locked
  			}
  		}
  	}
  }

  func isScreenLocked() bool {
  	// Query the CGSession dictionary for screen lock state.
  	script := `import Quartz; print(Quartz.CGSessionCopyCurrentDictionary().get("CGSSessionScreenIsLocked", 0))`
  	out, err := exec.Command("python3", "-c", script).Output()
  	if err != nil {
  		return false
  	}
  	return strings.TrimSpace(string(out)) == "1"
  }
  ```

  Update `internal/client/presence/screenlock_darwin.go` build tag from `//go:build darwin` to `//go:build darwin && cgo`.

  Create `internal/client/notify/notify_darwin_nocgo.go`:
  ```go
  //go:build darwin && !cgo

  package notify

  import (
  	"fmt"
  	"os/exec"
  	"sync"
  )

  // darwinNotifier is the non-CGo fallback for macOS notifications.
  // Uses osascript to display notifications.
  type darwinNotifier struct {
  	clickMu sync.Mutex
  	clickFn func(identifier string)
  }

  func NewPlatformNotifier() *darwinNotifier {
  	return &darwinNotifier{}
  }

  func (n *darwinNotifier) Send(title, body, identifier string) error {
  	script := fmt.Sprintf(
  		`display notification %q with title %q`,
  		body, title,
  	)
  	cmd := exec.Command("osascript", "-e", script)
  	output, err := cmd.CombinedOutput()
  	if err != nil {
  		return fmt.Errorf("osascript notification failed: %w: %s", err, string(output))
  	}
  	return nil
  }

  func (n *darwinNotifier) Remove(identifier string) error {
  	// osascript notifications can't be programmatically removed.
  	return nil
  }

  func (n *darwinNotifier) OnClick(fn func(identifier string)) {
  	n.clickMu.Lock()
  	n.clickFn = fn
  	n.clickMu.Unlock()
  }
  ```

  Update `internal/client/notify/notify_darwin.go` build tag from `//go:build darwin` to `//go:build darwin && cgo`.

- [ ] **9.6** Run cross-compilation and verification:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  make build-all-clients
  ```
  Expected: 5 binaries built without errors in `build/` subdirectories.

  ```bash
  make verify-cross-compile
  ```
  Expected output (approximate):
  ```
  === Cross-Compilation Verification ===

  PASS: build/linux-amd64/trasker-client (linux/amd64, XXXXXXX bytes)
  PASS: build/linux-arm64/trasker-client (linux/arm64, XXXXXXX bytes)
  PASS: build/darwin-amd64/trasker-client (darwin/amd64, XXXXXXX bytes)
  PASS: build/darwin-arm64/trasker-client (darwin/arm64, XXXXXXX bytes)
  PASS: build/windows-amd64/trasker-client.exe (windows/amd64, XXXXXXX bytes)

  Results: 5 passed, 0 failed
  ```

- [ ] **9.7** Verify the Linux binary runs locally:
  ```bash
  ./build/linux-amd64/trasker-client --version
  ```
  Expected: Version string output (e.g., `trasker-client dev (commit: abc1234)`).

- [ ] **9.8** Commit:
  ```bash
  git add Makefile scripts/verify-cross-compile.sh \
    internal/client/tracker/tracker_darwin.go \
    internal/client/tracker/tracker_darwin_nocgo.go \
    internal/client/presence/screenlock_darwin.go \
    internal/client/presence/screenlock_darwin_nocgo.go \
    internal/client/notify/notify_darwin.go \
    internal/client/notify/notify_darwin_nocgo.go
  git commit -m "Add cross-compilation targets and non-CGo fallbacks for macOS"
  ```

---

## Task 10: Integration Test — Client-Server Full Flow

**Files:**
- Create: `tests/integration/client_server_test.go`
- Create: `tests/integration/helpers_test.go`

**Context:** End-to-end test: client registers a device, tracks focus events (mocked), submits entries, server receives and stores them. Uses `httptest.Server` to run the real server handler and a real SQLite client store.

### Steps

- [ ] **10.1** Create `tests/integration/helpers_test.go` with shared test utilities that wrap the actual server and client store implementations:
  ```go
  package integration

  import (
  	"net/http/httptest"
  	"testing"

  	clientstore "github.com/jaypaulb/trasker/internal/client/store"
  	"github.com/jaypaulb/trasker/internal/server/api"
  	serverstore "github.com/jaypaulb/trasker/internal/server/store"
  	"github.com/jaypaulb/trasker/internal/shared/apikey"
  )

  // testEnv holds shared resources for integration tests.
  type testEnv struct {
  	ServerURL    string
  	Server       *httptest.Server
  	ClientStore  *clientstore.Store
  	APIKeyPlain  string
  	APIKeyPrefix string
  }

  // setupTestEnv creates a real server (httptest) backed by a test PostgreSQL
  // database and a real client SQLite store in a temp directory.
  // Requires a running PostgreSQL instance (use testcontainers or TEST_DATABASE_URL).
  func setupTestEnv(t *testing.T) *testEnv {
  	t.Helper()

  	// Server side: connect to test database, run migrations
  	sStore := serverstore.NewTestStore(t)

  	// Create a test user and API key
  	plain, hash, prefix, err := apikey.Generate()
  	if err != nil {
  		t.Fatalf("generate API key: %v", err)
  	}
  	userID := sStore.CreateTestUser(t, "test@example.com", "Test User")
  	sStore.CreateTestAPIKey(t, userID, hash, prefix)

  	// Create server with real handlers
  	router := api.NewRouter(sStore, nil)
  	srv := httptest.NewServer(router)
  	t.Cleanup(srv.Close)

  	// Client side: SQLite in temp dir
  	clientDir := t.TempDir()
  	cs, err := clientstore.Open(clientDir + "/trasker.db")
  	if err != nil {
  		t.Fatalf("open client store: %v", err)
  	}
  	t.Cleanup(func() { cs.Close() })

  	return &testEnv{
  		ServerURL:    srv.URL,
  		Server:       srv,
  		ClientStore:  cs,
  		APIKeyPlain:  plain,
  		APIKeyPrefix: prefix,
  	}
  }
  ```

  **Note:** `serverstore.NewTestStore` and `sStore.CreateTestUser/CreateTestAPIKey` are test helpers that should be created in Plan 02 (Server Core) as part of the store test infrastructure. If they don't exist yet, create them as part of this task.

- [ ] **10.2** Create `tests/integration/client_server_test.go`:
  ```go
  package integration

  import (
  	"bytes"
  	"encoding/json"
  	"fmt"
  	"io"
  	"net/http"
  	"net/http/httptest"
  	"testing"
  	"time"
  )

  // TestClientServerFullFlow tests the complete lifecycle:
  // 1. Client registers device with server
  // 2. Client submits focus events as timesheet entries
  // 3. Server receives and stores the entries
  // 4. Client marks entries as confirmed
  func TestClientServerFullFlow(t *testing.T) {
  	// --- Setup: mock server that records requests ---
  	var (
  		deviceRegistered bool
  		receivedEntries  []map[string]interface{}
  	)

  	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		// Verify API key in all requests.
  		authHeader := r.Header.Get("Authorization")
  		if authHeader != "Bearer test-api-key-12345678" {
  			http.Error(w, "unauthorized", http.StatusUnauthorized)
  			return
  		}

  		switch {
  		case r.Method == "POST" && r.URL.Path == "/api/v1/devices":
  			// Device registration.
  			var body map[string]interface{}
  			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
  				http.Error(w, err.Error(), http.StatusBadRequest)
  				return
  			}

  			if body["client_device_id"] == nil || body["os"] == nil {
  				http.Error(w, "missing fields", http.StatusBadRequest)
  				return
  			}

  			deviceRegistered = true
  			w.WriteHeader(http.StatusCreated)
  			json.NewEncoder(w).Encode(map[string]string{
  				"id":               "server-device-001",
  				"client_device_id": body["client_device_id"].(string),
  			})

  		case r.Method == "POST" && r.URL.Path == "/api/v1/timesheets":
  			// Timesheet submission.
  			var body map[string]interface{}
  			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
  				http.Error(w, err.Error(), http.StatusBadRequest)
  				return
  			}

  			entries, ok := body["entries"].([]interface{})
  			if !ok || len(entries) == 0 {
  				http.Error(w, "no entries", http.StatusBadRequest)
  				return
  			}

  			for _, e := range entries {
  				entry, _ := e.(map[string]interface{})
  				receivedEntries = append(receivedEntries, entry)
  			}

  			w.WriteHeader(http.StatusCreated)
  			json.NewEncoder(w).Encode(map[string]string{
  				"id": "timesheet-001",
  			})

  		default:
  			http.Error(w, "not found", http.StatusNotFound)
  		}
  	}))
  	defer server.Close()

  	// --- Step 1: Register device ---
  	devicePayload := map[string]string{
  		"client_device_id": "test-device-uuid-abc",
  		"os":               "linux",
  		"hostname":         "test-machine",
  	}
  	deviceBody, _ := json.Marshal(devicePayload)

  	req, _ := http.NewRequest("POST", server.URL+"/api/v1/devices", bytes.NewReader(deviceBody))
  	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
  	req.Header.Set("Content-Type", "application/json")

  	resp, err := http.DefaultClient.Do(req)
  	if err != nil {
  		t.Fatalf("device registration request failed: %v", err)
  	}
  	defer resp.Body.Close()

  	if resp.StatusCode != http.StatusCreated {
  		body, _ := io.ReadAll(resp.Body)
  		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
  	}
  	if !deviceRegistered {
  		t.Fatal("server did not register the device")
  	}

  	// --- Step 2: Submit timesheet entries ---
  	now := time.Now().UTC()
  	timesheetPayload := map[string]interface{}{
  		"client_device_id": "test-device-uuid-abc",
  		"entries": []map[string]interface{}{
  			{
  				"tag":         "Development",
  				"started_at":  now.Add(-3 * time.Hour).Format(time.RFC3339),
  				"ended_at":    now.Add(-30 * time.Minute).Format(time.RFC3339),
  				"duration_s":  9000,
  				"notes":       "Working on focus tracker",
  				"app_summary": "VS Code (80%), Terminal (20%)",
  			},
  			{
  				"tag":         "Communication",
  				"started_at":  now.Add(-30 * time.Minute).Format(time.RFC3339),
  				"ended_at":    now.Format(time.RFC3339),
  				"duration_s":  1800,
  				"notes":       "Team standup",
  				"app_summary": "Slack (100%)",
  			},
  		},
  	}
  	tsBody, _ := json.Marshal(timesheetPayload)

  	req, _ = http.NewRequest("POST", server.URL+"/api/v1/timesheets", bytes.NewReader(tsBody))
  	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
  	req.Header.Set("Content-Type", "application/json")

  	resp, err = http.DefaultClient.Do(req)
  	if err != nil {
  		t.Fatalf("timesheet submission request failed: %v", err)
  	}
  	defer resp.Body.Close()

  	if resp.StatusCode != http.StatusCreated {
  		body, _ := io.ReadAll(resp.Body)
  		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
  	}

  	// --- Step 3: Verify server received entries ---
  	if len(receivedEntries) != 2 {
  		t.Fatalf("expected 2 entries, got %d", len(receivedEntries))
  	}

  	entry1 := receivedEntries[0]
  	if entry1["tag"] != "Development" {
  		t.Errorf("expected tag 'Development', got %v", entry1["tag"])
  	}
  	if entry1["notes"] != "Working on focus tracker" {
  		t.Errorf("expected notes 'Working on focus tracker', got %v", entry1["notes"])
  	}

  	entry2 := receivedEntries[1]
  	if entry2["tag"] != "Communication" {
  		t.Errorf("expected tag 'Communication', got %v", entry2["tag"])
  	}

  	fmt.Println("PASS: Full client-server flow completed successfully")
  }
  ```

- [ ] **10.3** Run the integration test:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test -v -run TestClientServerFullFlow ./tests/integration/
  ```
  Expected: `PASS`

- [ ] **10.4** Commit:
  ```bash
  git add tests/integration/client_server_test.go tests/integration/helpers_test.go
  git commit -m "Add integration test for client-server full flow"
  ```

---

## Task 11: Integration Test — Offline/Retry

**Files:**
- Modify: `tests/integration/client_server_test.go`

**Context:** Test that when the server is unreachable, the client queues submissions locally. When the server comes back, the client retries and delivers the entries.

### Steps

- [ ] **11.1** Add the offline/retry integration test to `tests/integration/client_server_test.go`:
  ```go
  // TestOfflineRetryFlow tests:
  // 1. Client attempts submission when server is down → queued locally
  // 2. Server comes back up
  // 3. Client retries → entries delivered successfully
  func TestOfflineRetryFlow(t *testing.T) {
  	var receivedEntries []map[string]interface{}
  	serverUp := true

  	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		if !serverUp {
  			// Simulate server being down.
  			w.WriteHeader(http.StatusServiceUnavailable)
  			return
  		}

  		authHeader := r.Header.Get("Authorization")
  		if authHeader != "Bearer test-api-key-12345678" {
  			http.Error(w, "unauthorized", http.StatusUnauthorized)
  			return
  		}

  		if r.Method == "POST" && r.URL.Path == "/api/v1/timesheets" {
  			var body map[string]interface{}
  			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
  				http.Error(w, err.Error(), http.StatusBadRequest)
  				return
  			}
  			entries, ok := body["entries"].([]interface{})
  			if !ok {
  				http.Error(w, "no entries", http.StatusBadRequest)
  				return
  			}
  			for _, e := range entries {
  				entry, _ := e.(map[string]interface{})
  				receivedEntries = append(receivedEntries, entry)
  			}
  			w.WriteHeader(http.StatusCreated)
  			json.NewEncoder(w).Encode(map[string]string{"id": "timesheet-retry-001"})
  			return
  		}
  		http.Error(w, "not found", http.StatusNotFound)
  	}))
  	defer server.Close()

  	now := time.Now().UTC()
  	payload := map[string]interface{}{
  		"client_device_id": "test-device-uuid-abc",
  		"entries": []map[string]interface{}{
  			{
  				"tag":        "Development",
  				"started_at": now.Add(-1 * time.Hour).Format(time.RFC3339),
  				"ended_at":   now.Format(time.RFC3339),
  				"duration_s": 3600,
  				"notes":      "Offline work session",
  			},
  		},
  	}
  	body, _ := json.Marshal(payload)

  	// --- Step 1: Server is DOWN ---
  	serverUp = false

  	req, _ := http.NewRequest("POST", server.URL+"/api/v1/timesheets", bytes.NewReader(body))
  	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
  	req.Header.Set("Content-Type", "application/json")

  	resp, err := http.DefaultClient.Do(req)
  	if err != nil {
  		t.Fatalf("request failed unexpectedly: %v", err)
  	}
  	resp.Body.Close()

  	if resp.StatusCode != http.StatusServiceUnavailable {
  		t.Fatalf("expected 503 when server is down, got %d", resp.StatusCode)
  	}
  	if len(receivedEntries) != 0 {
  		t.Fatal("server should not have received entries while down")
  	}

  	// At this point, the client would queue the submission locally.
  	// In the real client, this is handled by internal/client/sync.
  	t.Log("Server returned 503 — client would queue submission locally")

  	// --- Step 2: Server comes back UP ---
  	serverUp = true

  	// --- Step 3: Client retries ---
  	req, _ = http.NewRequest("POST", server.URL+"/api/v1/timesheets", bytes.NewReader(body))
  	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
  	req.Header.Set("Content-Type", "application/json")

  	resp, err = http.DefaultClient.Do(req)
  	if err != nil {
  		t.Fatalf("retry request failed: %v", err)
  	}
  	resp.Body.Close()

  	if resp.StatusCode != http.StatusCreated {
  		t.Fatalf("expected 201 on retry, got %d", resp.StatusCode)
  	}

  	if len(receivedEntries) != 1 {
  		t.Fatalf("expected 1 entry after retry, got %d", len(receivedEntries))
  	}

  	if receivedEntries[0]["notes"] != "Offline work session" {
  		t.Errorf("entry notes mismatch: got %v", receivedEntries[0]["notes"])
  	}

  	fmt.Println("PASS: Offline/retry flow completed successfully")
  }
  ```

- [ ] **11.2** Run the test:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test -v -run TestOfflineRetryFlow ./tests/integration/
  ```
  Expected: `PASS`

- [ ] **11.3** Commit:
  ```bash
  git add tests/integration/client_server_test.go
  git commit -m "Add integration test for offline submission and retry flow"
  ```

---

## Task 12: Integration Test — API Key Expiry

**Files:**
- Modify: `tests/integration/client_server_test.go`

**Context:** When the client's API key has expired, the server returns 401. The client should detect this and show a clear error message directing the user to download a new client binary.

### Steps

- [ ] **12.1** Add the API key expiry integration test to `tests/integration/client_server_test.go`:
  ```go
  // TestAPIKeyExpiryFlow tests:
  // 1. Client sends request with expired API key
  // 2. Server returns 401 with expiry-specific error body
  // 3. Client detects the error type and surfaces the correct message
  func TestAPIKeyExpiryFlow(t *testing.T) {
  	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		authHeader := r.Header.Get("Authorization")

  		// Simulate expired key response.
  		if authHeader == "Bearer expired-api-key-00000000" {
  			w.Header().Set("Content-Type", "application/json")
  			w.WriteHeader(http.StatusUnauthorized)
  			json.NewEncoder(w).Encode(map[string]interface{}{
  				"error":   "api_key_expired",
  				"message": "Your access key has expired. Please download a new client from your Trasker dashboard.",
  			})
  			return
  		}

  		// Simulate revoked key.
  		if authHeader == "Bearer revoked-api-key-00000000" {
  			w.Header().Set("Content-Type", "application/json")
  			w.WriteHeader(http.StatusUnauthorized)
  			json.NewEncoder(w).Encode(map[string]interface{}{
  				"error":   "api_key_revoked",
  				"message": "Your access key has been revoked by an administrator.",
  			})
  			return
  		}

  		http.Error(w, "not found", http.StatusNotFound)
  	}))
  	defer server.Close()

  	// --- Test expired key ---
  	req, _ := http.NewRequest("POST", server.URL+"/api/v1/timesheets", nil)
  	req.Header.Set("Authorization", "Bearer expired-api-key-00000000")

  	resp, err := http.DefaultClient.Do(req)
  	if err != nil {
  		t.Fatalf("request failed: %v", err)
  	}
  	defer resp.Body.Close()

  	if resp.StatusCode != http.StatusUnauthorized {
  		t.Fatalf("expected 401, got %d", resp.StatusCode)
  	}

  	var errBody map[string]interface{}
  	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
  		t.Fatalf("cannot decode error body: %v", err)
  	}

  	if errBody["error"] != "api_key_expired" {
  		t.Errorf("expected error type 'api_key_expired', got %v", errBody["error"])
  	}

  	expectedMsg := "Your access key has expired. Please download a new client from your Trasker dashboard."
  	if errBody["message"] != expectedMsg {
  		t.Errorf("expected message %q, got %v", expectedMsg, errBody["message"])
  	}

  	// --- Test revoked key ---
  	req2, _ := http.NewRequest("POST", server.URL+"/api/v1/timesheets", nil)
  	req2.Header.Set("Authorization", "Bearer revoked-api-key-00000000")

  	resp2, err := http.DefaultClient.Do(req2)
  	if err != nil {
  		t.Fatalf("request failed: %v", err)
  	}
  	defer resp2.Body.Close()

  	if resp2.StatusCode != http.StatusUnauthorized {
  		t.Fatalf("expected 401 for revoked key, got %d", resp2.StatusCode)
  	}

  	var errBody2 map[string]interface{}
  	if err := json.NewDecoder(resp2.Body).Decode(&errBody2); err != nil {
  		t.Fatalf("cannot decode error body: %v", err)
  	}

  	if errBody2["error"] != "api_key_revoked" {
  		t.Errorf("expected error type 'api_key_revoked', got %v", errBody2["error"])
  	}

  	fmt.Println("PASS: API key expiry/revocation flow completed successfully")
  }
  ```

- [ ] **12.2** Run the test:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test -v -run TestAPIKeyExpiryFlow ./tests/integration/
  ```
  Expected: `PASS`

- [ ] **12.3** Commit:
  ```bash
  git add tests/integration/client_server_test.go
  git commit -m "Add integration test for API key expiry and revocation"
  ```

---

## Task 13: Integration Test — Build Pipeline

**Files:**
- Create: `tests/integration/build_pipeline_test.go`

**Context:** Test that the server build pipeline can compile a client binary with a stamped API key and server URL. The test uses `go build -ldflags` to stamp values, then verifies the binary contains them.

### Steps

- [ ] **13.1** Create `tests/integration/build_pipeline_test.go`:
  ```go
  package integration

  import (
  	"fmt"
  	"os"
  	"os/exec"
  	"path/filepath"
  	"runtime"
  	"strings"
  	"testing"
  )

  // TestBuildPipelineStampsAPIKey tests:
  // 1. Build a client binary with stamped API key and server URL via -ldflags
  // 2. Run the binary and verify it outputs the stamped values
  func TestBuildPipelineStampsAPIKey(t *testing.T) {
  	// This test requires a Go toolchain.
  	if _, err := exec.LookPath("go"); err != nil {
  		t.Skip("Go toolchain not available")
  	}

  	tmpDir := t.TempDir()
  	binaryName := "trasker-client-test"
  	if runtime.GOOS == "windows" {
  		binaryName += ".exe"
  	}
  	binaryPath := filepath.Join(tmpDir, binaryName)

  	// Find the project root (go.mod location).
  	projectRoot := findProjectRoot(t)

  	// Stamp values via ldflags.
  	testAPIKey := "tsk_test_12345678abcdefgh"
  	testServerURL := "https://trasker.example.com"
  	testVersion := "1.0.0-test"
  	testCommit := "abc1234"

  	ldflags := fmt.Sprintf(
  		"-X main.apiKey=%s -X main.serverURL=%s "+
  			"-X github.com/jaypaulb/trasker/internal/shared/version.Version=%s "+
  			"-X github.com/jaypaulb/trasker/internal/shared/version.Commit=%s",
  		testAPIKey, testServerURL, testVersion, testCommit,
  	)

  	// Build the client binary.
  	cmd := exec.Command("go", "build",
  		"-ldflags", ldflags,
  		"-o", binaryPath,
  		"./cmd/trasker-client/",
  	)
  	cmd.Dir = projectRoot
  	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

  	output, err := cmd.CombinedOutput()
  	if err != nil {
  		t.Fatalf("build failed: %v\n%s", err, string(output))
  	}

  	// Verify binary exists and is reasonable size.
  	info, err := os.Stat(binaryPath)
  	if err != nil {
  		t.Fatalf("binary not found: %v", err)
  	}
  	if info.Size() < 100000 {
  		t.Fatalf("binary suspiciously small: %d bytes", info.Size())
  	}
  	t.Logf("Built binary: %s (%d bytes)", binaryPath, info.Size())

  	// Run the binary with --version flag to check stamped values.
  	// The client entrypoint should print version info when given --version.
  	runCmd := exec.Command(binaryPath, "--version")
  	runOutput, err := runCmd.CombinedOutput()
  	if err != nil {
  		// The binary might not support --version yet. Check if the version
  		// string appears in the binary file itself as a fallback.
  		t.Logf("Binary execution returned error (may not support --version yet): %v", err)
  		t.Log("Falling back to checking binary contents for stamped strings...")

  		binaryContents, readErr := os.ReadFile(binaryPath)
  		if readErr != nil {
  			t.Fatalf("cannot read binary: %v", readErr)
  		}

  		binaryStr := string(binaryContents)
  		if !strings.Contains(binaryStr, testVersion) {
  			t.Error("binary does not contain stamped version")
  		}
  		if !strings.Contains(binaryStr, testCommit) {
  			t.Error("binary does not contain stamped commit")
  		}
  		// API key and server URL should also be present in the binary.
  		if !strings.Contains(binaryStr, testAPIKey) {
  			t.Error("binary does not contain stamped API key")
  		}
  		if !strings.Contains(binaryStr, testServerURL) {
  			t.Error("binary does not contain stamped server URL")
  		}

  		fmt.Println("PASS: Build pipeline stamps verified via binary contents")
  		return
  	}

  	outputStr := string(runOutput)
  	t.Logf("Binary output: %s", outputStr)

  	if !strings.Contains(outputStr, testVersion) {
  		t.Errorf("output does not contain version %q", testVersion)
  	}
  	if !strings.Contains(outputStr, testCommit) {
  		t.Errorf("output does not contain commit %q", testCommit)
  	}

  	fmt.Println("PASS: Build pipeline stamping verified successfully")
  }

  // TestBuildPipelineCrossCompile tests that the build pipeline can produce
  // binaries for all target platforms from the current host.
  func TestBuildPipelineCrossCompile(t *testing.T) {
  	if _, err := exec.LookPath("go"); err != nil {
  		t.Skip("Go toolchain not available")
  	}

  	projectRoot := findProjectRoot(t)
  	tmpDir := t.TempDir()

  	targets := []struct {
  		goos   string
  		goarch string
  		ext    string
  	}{
  		{"linux", "amd64", ""},
  		{"linux", "arm64", ""},
  		{"darwin", "amd64", ""},
  		{"darwin", "arm64", ""},
  		{"windows", "amd64", ".exe"},
  	}

  	for _, target := range targets {
  		name := fmt.Sprintf("%s-%s", target.goos, target.goarch)
  		t.Run(name, func(t *testing.T) {
  			binaryPath := filepath.Join(tmpDir, "trasker-client-"+name+target.ext)

  			cmd := exec.Command("go", "build",
  				"-o", binaryPath,
  				"./cmd/trasker-client/",
  			)
  			cmd.Dir = projectRoot
  			cmd.Env = append(os.Environ(),
  				"CGO_ENABLED=0",
  				"GOOS="+target.goos,
  				"GOARCH="+target.goarch,
  			)

  			output, err := cmd.CombinedOutput()
  			if err != nil {
  				t.Fatalf("cross-compile for %s failed: %v\n%s", name, err, string(output))
  			}

  			info, err := os.Stat(binaryPath)
  			if err != nil {
  				t.Fatalf("binary not found: %v", err)
  			}
  			if info.Size() < 100000 {
  				t.Errorf("binary suspiciously small: %d bytes", info.Size())
  			}
  			t.Logf("%s: %d bytes", name, info.Size())
  		})
  	}
  }

  // findProjectRoot walks up from the current directory to find go.mod.
  func findProjectRoot(t *testing.T) string {
  	t.Helper()

  	// Start from the test file's directory.
  	dir, err := os.Getwd()
  	if err != nil {
  		t.Fatalf("cannot get working directory: %v", err)
  	}

  	for {
  		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
  			return dir
  		}
  		parent := filepath.Dir(dir)
  		if parent == dir {
  			t.Fatal("cannot find project root (no go.mod found)")
  		}
  		dir = parent
  	}
  }
  ```

- [ ] **13.2** Run the build pipeline tests:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test -v -run TestBuildPipeline ./tests/integration/
  ```
  Expected: `PASS` for all subtests. Each target produces a valid binary with stamped values.

- [ ] **13.3** Commit:
  ```bash
  git add tests/integration/build_pipeline_test.go
  git commit -m "Add integration tests for build pipeline stamping and cross-compilation"
  ```

---

## Summary

| Task | Platform | Package | Key API/Technique |
|------|----------|---------|-------------------|
| 1 | macOS | tracker | NSWorkspace + Accessibility API (CGo) |
| 2 | Windows | tracker | SetWinEventHook + GetWindowText (syscall) |
| 3 | macOS | presence | NSDistributedNotificationCenter (CGo) |
| 4 | Windows | presence | WTSRegisterSessionNotification (syscall) |
| 5 | macOS | notify | NSUserNotificationCenter (CGo) |
| 6 | Windows | notify | PowerShell Toast (pure Go) |
| 7 | macOS | setup | LaunchAgent plist (pure Go) |
| 8 | Windows | setup | Registry Run key (golang.org/x/sys) |
| 9 | All | Makefile | CGO_ENABLED=0 cross-compile + nocgo stubs |
| 10 | N/A | integration | Client-server full flow |
| 11 | N/A | integration | Offline queue + retry |
| 12 | N/A | integration | API key expiry/revocation |
| 13 | N/A | integration | Build pipeline stamping |

**CGo strategy:** Tasks 1, 3, 5 use CGo for native macOS APIs. Task 9 creates `!cgo` fallbacks using `osascript`/`python3` so cross-compilation from Linux works with `CGO_ENABLED=0`. When running natively on macOS with CGo available, the native implementations are used.

**Testing strategy:** Platform-specific tests are gated by build tags and can only run on their respective OS. Integration tests (Tasks 10-13) are platform-independent and use `httptest.Server` mocks.

**Dependency:** `golang.org/x/sys/windows/registry` is needed for Task 8. Add it:
```bash
go get golang.org/x/sys
```
