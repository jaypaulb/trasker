// internal/client/presence/deadman_test.go
package presence_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/presence"
)

func TestDeadman_FiresAfterInterval(t *testing.T) {
	// Use very short intervals for testing
	dm, err := presence.NewDeadman(
		[]time.Duration{50 * time.Millisecond, 100 * time.Millisecond},
		200 * time.Millisecond, // countdown duration
	)
	if err != nil {
		t.Fatalf("NewDeadman: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm.Start(ctx)

	// Should fire CHECKING after ~50ms
	select {
	case sc := <-dm.States():
		if sc.State != presence.Checking {
			t.Errorf("first fire: State = %v, want CHECKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for CHECKING state")
	}

	dm.Stop()
}

func TestDeadman_AcknowledgeEscalates(t *testing.T) {
	dm, err := presence.NewDeadman(
		[]time.Duration{50 * time.Millisecond, 100 * time.Millisecond},
		5 * time.Second, // long countdown so it doesn't expire
	)
	if err != nil {
		t.Fatalf("NewDeadman: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm.Start(ctx)

	// Wait for first CHECKING
	select {
	case sc := <-dm.States():
		if sc.State != presence.Checking {
			t.Fatalf("State = %v, want CHECKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for first CHECKING")
	}

	// Acknowledge — should move back to TRACKING
	dm.Acknowledge()

	select {
	case sc := <-dm.States():
		if sc.State != presence.Tracking {
			t.Fatalf("after ack: State = %v, want TRACKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for TRACKING after ack")
	}

	// Next CHECKING should come after 100ms (escalated interval)
	select {
	case sc := <-dm.States():
		if sc.State != presence.Checking {
			t.Fatalf("second fire: State = %v, want CHECKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for second CHECKING")
	}

	dm.Stop()
}

func TestDeadman_CountdownExpiresToPaused(t *testing.T) {
	dm, err := presence.NewDeadman(
		[]time.Duration{50 * time.Millisecond},
		100 * time.Millisecond, // short countdown
	)
	if err != nil {
		t.Fatalf("NewDeadman: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm.Start(ctx)

	// Wait for CHECKING
	select {
	case sc := <-dm.States():
		if sc.State != presence.Checking {
			t.Fatalf("State = %v, want CHECKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for CHECKING")
	}

	// Don't acknowledge — should expire to PAUSED after ~100ms
	select {
	case sc := <-dm.States():
		if sc.State != presence.Paused {
			t.Fatalf("after expiry: State = %v, want PAUSED", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for PAUSED")
	}

	dm.Stop()
}

func TestDeadman_ResetOnFocusChange(t *testing.T) {
	dm, err := presence.NewDeadman(
		[]time.Duration{100 * time.Millisecond, 200 * time.Millisecond},
		5 * time.Second,
	)
	if err != nil {
		t.Fatalf("NewDeadman: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dm.Start(ctx)

	// Wait for first CHECKING at 100ms
	select {
	case sc := <-dm.States():
		if sc.State != presence.Checking {
			t.Fatalf("State = %v, want CHECKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out")
	}

	// Acknowledge to escalate
	dm.Acknowledge()
	<-dm.States() // consume TRACKING

	// Now reset (simulating focus change) — should go back to tier 0 (100ms)
	dm.Reset()

	// Expect TRACKING from reset
	select {
	case sc := <-dm.States():
		if sc.State != presence.Tracking {
			t.Fatalf("after reset: State = %v, want TRACKING", sc.State)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for TRACKING after reset")
	}

	// Next CHECKING should come after ~100ms (tier 0, not 200ms)
	start := time.Now()
	select {
	case sc := <-dm.States():
		elapsed := time.Since(start)
		if sc.State != presence.Checking {
			t.Fatalf("State = %v, want CHECKING", sc.State)
		}
		// Should be close to 100ms, not 200ms
		if elapsed > 180*time.Millisecond {
			t.Errorf("next check after %v, expected ~100ms (tier 0 reset)", elapsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out")
	}

	dm.Stop()
}

func TestNewDeadman_RejectsInvalidConfig(t *testing.T) {
	// Empty intervals
	_, err := presence.NewDeadman(nil, 90*time.Second)
	if err == nil {
		t.Error("expected error for nil intervals")
	}

	_, err = presence.NewDeadman([]time.Duration{}, 90*time.Second)
	if err == nil {
		t.Error("expected error for empty intervals")
	}

	// Zero duration interval
	_, err = presence.NewDeadman([]time.Duration{0}, 90*time.Second)
	if err == nil {
		t.Error("expected error for zero interval")
	}

	// Negative interval
	_, err = presence.NewDeadman([]time.Duration{-1 * time.Second}, 90*time.Second)
	if err == nil {
		t.Error("expected error for negative interval")
	}

	// Zero countdown
	_, err = presence.NewDeadman([]time.Duration{30 * time.Second}, 0)
	if err == nil {
		t.Error("expected error for zero countdown")
	}

	// Valid config should work
	_, err = presence.NewDeadman([]time.Duration{30 * time.Second}, 90*time.Second)
	if err != nil {
		t.Errorf("unexpected error for valid config: %v", err)
	}
}
