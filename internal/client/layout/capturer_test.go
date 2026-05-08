package layout_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/layout"
	"github.com/jaypaulb/trasker/internal/client/presence"
)

// fakeEnumerator returns canned windows or a canned error per call.
// Calls counts how many times Enumerate was invoked.
type fakeEnumerator struct {
	mu      sync.Mutex
	results [][]layout.Window
	errs    []error
	calls   int
}

func (f *fakeEnumerator) Enumerate() ([]layout.Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	f.calls++
	if idx < len(f.errs) && f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	// repeat last result if available
	if len(f.results) > 0 {
		return f.results[len(f.results)-1], nil
	}
	return nil, nil
}

func (f *fakeEnumerator) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeEnumerator) Close() error { return nil }

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}))
}

// captureLogger captures all log output (including DEBUG) so assertions
// can match WARN messages.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, buf
}

func windowSetA() []layout.Window {
	return []layout.Window{
		{AppName: "Firefox", WindowTitle: "GitHub", X: 0, Y: 0, W: 1920, H: 1080},
	}
}

func windowSetB() []layout.Window {
	return []layout.Window{
		{AppName: "Code", WindowTitle: "main.go", X: 100, Y: 100, W: 1280, H: 800},
	}
}

// invokeTick runs the capturer's per-tick body directly. We use the
// public interval but call tick via Start+sleep — but to keep tests
// deterministic we use a thin wrapper that drives tick via a very short
// interval and a lock-bypass channel-less constructor.
//
// Implementation detail: NewCapturer's tick is unexported. To exercise
// it directly we drive Start with a tiny interval and wait for the
// ticker to fire N times.
func runCapturerForTicks(t *testing.T, c *layout.Capturer, ticks int, interval time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	// Wait long enough for `ticks` ticks plus a small safety margin.
	time.Sleep(time.Duration(ticks)*interval + interval/2)
	c.Stop()
}

// TestCapturer_SkipsUnchanged: same []Window twice; only one row
// inserted.
func TestCapturer_SkipsUnchanged(t *testing.T) {
	s := newLayoutTestStore(t)
	enum := &fakeEnumerator{
		results: [][]layout.Window{windowSetA(), windowSetA()},
	}
	tickInterval := 50 * time.Millisecond
	c := layout.NewCapturer(enum, s, nil, tickInterval, quietLogger())
	runCapturerForTicks(t, c, 2, tickInterval)

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("got %d rows, want 1 (change-detect should suppress duplicate)", len(pending))
	}
	if enum.Calls() < 2 {
		t.Errorf("enum.Calls() = %d, want at least 2", enum.Calls())
	}
}

// TestCapturer_WritesOnChange: A then B; both inserted with different
// hashes.
func TestCapturer_WritesOnChange(t *testing.T) {
	s := newLayoutTestStore(t)
	enum := &fakeEnumerator{
		results: [][]layout.Window{windowSetA(), windowSetB()},
	}
	tickInterval := 50 * time.Millisecond
	c := layout.NewCapturer(enum, s, nil, tickInterval, quietLogger())
	runCapturerForTicks(t, c, 2, tickInterval)

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("got %d rows, want 2", len(pending))
	}
	if pending[0].WindowsHash == pending[1].WindowsHash {
		t.Errorf("two different window sets produced identical hash: %s", pending[0].WindowsHash)
	}
}

// TestCapturer_SkipsWhenLocked: lockChan emits Away; capturer ticks;
// zero rows and zero enum calls.
func TestCapturer_SkipsWhenLocked(t *testing.T) {
	s := newLayoutTestStore(t)
	enum := &fakeEnumerator{
		results: [][]layout.Window{windowSetA()},
	}
	lockCh := make(chan presence.StateChange, 1)
	lockCh <- presence.StateChange{State: presence.Away, Timestamp: time.Now().UTC()}

	tickInterval := 50 * time.Millisecond
	c := layout.NewCapturer(enum, s, lockCh, tickInterval, quietLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	// Give the lock subscriber goroutine time to consume the event
	// BEFORE the first tick fires.
	time.Sleep(20 * time.Millisecond)
	// Now wait for several ticks under the locked state.
	time.Sleep(3 * tickInterval)
	c.Stop()

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("got %d rows, want 0 (capturer should skip when locked) — presence.Away gating broken", len(pending))
	}
	if enum.Calls() != 0 {
		t.Errorf("enum.Calls() = %d, want 0 (locked tick must not call Enumerate)", enum.Calls())
	}
}

// TestCapturer_FailSoft: enum returns error; capturer logs WARN; zero
// rows; next tick still runs (goroutine still alive).
func TestCapturer_FailSoft(t *testing.T) {
	s := newLayoutTestStore(t)
	enum := &fakeEnumerator{
		errs:    []error{errors.New("X11 boom"), nil},
		results: [][]layout.Window{nil, windowSetA()},
	}
	logger, buf := captureLogger()

	tickInterval := 50 * time.Millisecond
	c := layout.NewCapturer(enum, s, nil, tickInterval, logger)
	runCapturerForTicks(t, c, 3, tickInterval)

	if !strings.Contains(buf.String(), "layout enum failed") {
		t.Errorf("expected WARN log to contain 'layout enum failed'; got:\n%s", buf.String())
	}

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	// The first tick errored (no row), the second tick returned setA
	// (one row), subsequent ticks repeat setA (no new row by D-07).
	if len(pending) != 1 {
		t.Errorf("got %d rows, want 1 (error should not poison goroutine)", len(pending))
	}
}

// TestCapturer_NoLockListener: nil lockEvents = always-unlocked; rows
// still get written.
func TestCapturer_NoLockListener(t *testing.T) {
	s := newLayoutTestStore(t)
	enum := &fakeEnumerator{
		results: [][]layout.Window{windowSetA()},
	}
	tickInterval := 50 * time.Millisecond
	c := layout.NewCapturer(enum, s, nil, tickInterval, quietLogger())
	runCapturerForTicks(t, c, 2, tickInterval)

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("got %d rows, want 1 (nil lockEvents must not block capture)", len(pending))
	}
}
