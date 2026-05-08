package layout

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jaypaulb/trasker/internal/client/presence"
)

// Capturer drives the 60s window-set capture loop. Per Phase 7
// decisions D-07/D-08/D-09:
//   - skips writes when the hashed window-set is unchanged (D-07)
//   - skips the entire tick when the screen is locked (D-08)
//   - logs WARN and writes no row on enum error (D-09)
//
// The Capturer subscribes to a presence.StateChange channel and
// maintains an atomic.Bool. When the channel is nil (screenlock listener
// unavailable — e.g. headless) the Capturer treats the screen as always
// unlocked, per RESEARCH.md Pattern 3.
type Capturer struct {
	enum       Enumerator
	store      *Store
	lockEvents <-chan presence.StateChange
	interval   time.Duration
	logger     *slog.Logger

	locked   atomic.Bool
	lastHash string

	stopCh chan struct{}
}

// NewCapturer wires a capturer. interval is typically 60s in production;
// tests pass shorter values. logger may be nil — a default is provided.
func NewCapturer(
	enum Enumerator,
	store *Store,
	lockEvents <-chan presence.StateChange,
	interval time.Duration,
	logger *slog.Logger,
) *Capturer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Capturer{
		enum:       enum,
		store:      store,
		lockEvents: lockEvents,
		interval:   interval,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// Start spawns two goroutines: one bridges presence.StateChange events
// into the locked atomic.Bool, the other runs the 60s ticker.
func (c *Capturer) Start(ctx context.Context) {
	if c.lockEvents != nil {
		go c.runLockSubscriber(ctx)
	}
	go c.runTicker(ctx)
}

// Stop signals both goroutines to exit on their next iteration.
func (c *Capturer) Stop() {
	select {
	case <-c.stopCh:
		// already stopped
	default:
		close(c.stopCh)
	}
}

func (c *Capturer) runLockSubscriber(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case ev, ok := <-c.lockEvents:
			if !ok {
				return
			}
			// Only the screenlock-derived Away state gates capture
			// (RESEARCH.md A7). Other states (Paused/Checking) belong
			// to the deadman switch and must NOT block layout capture.
			c.locked.Store(ev.State == presence.Away)
		}
	}
}

func (c *Capturer) runTicker(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick is the per-interval body. Public for test direct-drive.
func (c *Capturer) tick(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// D-08: skip the tick entirely when locked.
	if c.locked.Load() {
		c.logger.Debug("layout capture: skipping tick (screen locked)")
		return
	}

	windows, err := c.enum.Enumerate()
	if err != nil {
		// D-09: fail-soft. Log WARN, write no row, retry next tick.
		c.logger.Warn("layout enum failed", "err", err)
		return
	}

	// D-07: change-detect via canonical hash.
	hash := HashWindows(windows)
	if hash == c.lastHash {
		return
	}

	jsonBytes, err := json.Marshal(windows)
	if err != nil {
		c.logger.Warn("layout capture: marshal windows failed", "err", err)
		return
	}

	id, err := c.store.InsertSnapshot(time.Now().UTC(), jsonBytes, hash)
	if err != nil {
		c.logger.Warn("layout capture: insert snapshot failed", "err", err)
		return
	}
	c.lastHash = hash
	// Hash + count only; never log raw window titles (T-7-02-05).
	c.logger.Debug("layout snapshot captured",
		"id", id,
		"hash", hash,
		"window_count", len(windows),
	)
}
