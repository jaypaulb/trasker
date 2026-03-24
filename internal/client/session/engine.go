// internal/client/session/engine.go
package session

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jaypaulb/trasker/internal/client/presence"
	"github.com/jaypaulb/trasker/internal/client/store"
	"github.com/jaypaulb/trasker/internal/client/tracker"
)

// Engine orchestrates focus tracking, presence detection, and storage.
// It wires together a Tracker, a PresenceDetector, and a Store.
type Engine struct {
	store    *store.Store
	tracker  tracker.Tracker
	presence presence.PresenceDetector

	currentEventID int64
	paused         bool
	mu             sync.Mutex
	wg             sync.WaitGroup
}

// NewEngine creates a new session engine.
func NewEngine(s *store.Store, t tracker.Tracker, p presence.PresenceDetector) *Engine {
	return &Engine{
		store:    s,
		tracker:  t,
		presence: p,
	}
}

// Start begins the session engine. It starts the tracker and presence
// detector, then runs the main event loop in a goroutine.
func (e *Engine) Start(ctx context.Context) {
	if err := e.tracker.Start(ctx); err != nil {
		log.Printf("tracker start error: %v", err)
	}
	if err := e.presence.Start(ctx); err != nil {
		log.Printf("presence start error: %v", err)
	}

	e.wg.Add(1)
	go e.run(ctx)
}

// Wait blocks until the engine's event loop has exited.
func (e *Engine) Wait() {
	e.wg.Wait()
}

func (e *Engine) run(ctx context.Context) {
	defer e.wg.Done()

	focusEvents := e.tracker.Events()
	presenceStates := e.presence.States()

	for {
		select {
		case <-ctx.Done():
			e.endCurrentEvent()
			return

		case fc, ok := <-focusEvents:
			if !ok {
				return
			}
			e.handleFocusChange(fc)

		case sc, ok := <-presenceStates:
			if !ok {
				return
			}
			e.handlePresenceChange(sc)
		}
	}
}

func (e *Engine) handleFocusChange(fc tracker.FocusChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.paused {
		return // Ignore focus events while paused or away
	}

	// End the current event
	if e.currentEventID > 0 {
		if err := e.store.EndFocusEvent(e.currentEventID, fc.Timestamp); err != nil {
			log.Printf("end focus event %d: %v", e.currentEventID, err)
		}
	}

	// Insert the new event
	id, err := e.store.InsertFocusEvent(fc.AppName, fc.WindowTitle, fc.Timestamp)
	if err != nil {
		log.Printf("insert focus event: %v", err)
		return
	}
	e.currentEventID = id

	// Notify presence detector of focus change (resets deadman's switch)
	e.presence.ResetOnFocusChange()
}

func (e *Engine) handlePresenceChange(sc presence.StateChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch sc.State {
	case presence.Away:
		// Screen locked — end current event and mark as idle boundary
		e.endCurrentEventLocked()
		e.paused = true

	case presence.Paused:
		// Deadman's switch expired — end current event
		e.endCurrentEventLocked()
		e.paused = true

	case presence.Tracking:
		// Resumed — ready to accept focus events again
		e.paused = false

	case presence.Checking:
		// Deadman's switch fired — the UI layer will handle the notification.
		// Engine doesn't need to act on CHECKING; it continues tracking until
		// PAUSED or acknowledgment.
	}
}

func (e *Engine) endCurrentEvent() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.endCurrentEventLocked()
}

func (e *Engine) endCurrentEventLocked() {
	if e.currentEventID > 0 {
		now := time.Now().UTC()
		if err := e.store.EndFocusEvent(e.currentEventID, now); err != nil {
			log.Printf("end focus event %d: %v", e.currentEventID, err)
		}
		e.currentEventID = 0
	}
}
