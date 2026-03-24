// internal/client/notify/dispatcher_test.go
package notify

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// mockNotifier records all notifications for testing.
type mockNotifier struct {
	mu            sync.Mutex
	notifications []mockNotification
}

type mockNotification struct {
	Title    string
	Body     string
	HasClick bool
}

func (m *mockNotifier) Notify(title, body string, onClick ClickAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, mockNotification{
		Title:    title,
		Body:     body,
		HasClick: onClick != nil,
	})
	// Simulate user click if handler provided
	if onClick != nil {
		go onClick()
	}
	return nil
}

func (m *mockNotifier) Close() error { return nil }

func (m *mockNotifier) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func (m *mockNotifier) last() mockNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifications[len(m.notifications)-1]
}

func TestDispatcher_DeadmanCheck_Clicked(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	result := d.DeadmanCheck(5 * time.Second)

	// Mock auto-clicks, so we should get true
	select {
	case clicked := <-result:
		if !clicked {
			t.Error("expected clicked=true")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected result")
	}

	if mock.count() != 1 {
		t.Errorf("expected 1 notification, got %d", mock.count())
	}
}

func TestDispatcher_TrackingOffNag(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	resumed := false
	d.TrackingOffNag(func() { resumed = true })

	time.Sleep(50 * time.Millisecond) // let goroutine run

	if !resumed {
		t.Error("expected resume callback to fire")
	}
	n := mock.last()
	if n.Title != "Trasker is paused" {
		t.Errorf("unexpected title: %q", n.Title)
	}
}

func TestDispatcher_PomodoroTransitions(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	d.PomodoroTransition(EventPomodoroBreak)
	d.PomodoroTransition(EventPomodoroWork)
	d.PomodoroTransition(EventPomodoroDone)

	if mock.count() != 3 {
		t.Errorf("expected 3 notifications, got %d", mock.count())
	}
}

func TestDispatcher_LongFocusReminder(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	d.LongFocusReminder("VS Code", 90*time.Minute)

	if mock.count() != 1 {
		t.Errorf("expected 1 notification, got %d", mock.count())
	}
	n := mock.last()
	if n.Title != "Trasker — Long Focus" {
		t.Errorf("unexpected title: %q", n.Title)
	}
}
