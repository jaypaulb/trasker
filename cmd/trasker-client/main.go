// cmd/trasker-client/main.go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jaypaulb/trasker/internal/client/layout"
	"github.com/jaypaulb/trasker/internal/client/notify"
	"github.com/jaypaulb/trasker/internal/client/pomodoro"
	"github.com/jaypaulb/trasker/internal/client/presence"
	clientruntime "github.com/jaypaulb/trasker/internal/client/runtime"
	"github.com/jaypaulb/trasker/internal/client/setup"
	"github.com/jaypaulb/trasker/internal/client/store"
	syncpkg "github.com/jaypaulb/trasker/internal/client/sync"
	"github.com/jaypaulb/trasker/internal/client/tagger"
	"github.com/jaypaulb/trasker/internal/client/tracker"
	"github.com/jaypaulb/trasker/internal/client/tray"
	"github.com/jaypaulb/trasker/internal/client/webui"
)

// Build-time values stamped via ldflags.
// In production, these contain fixed-size sentinel strings that are patched
// (bytes.Replace) by the server at download time. The sentinels are 128 chars
// each so the binary size is stable regardless of real value length.
// For local dev, override with: go build -ldflags "-X main.serverURL=... -X main.apiKey=..."
var (
	serverURL = "http://localhost:8080"
	apiKey    = "dev-key"
	version   = "dev"
)

const (
	defaultWebPort   = 9746
	learnerThreshold = 5
)

func main() {
	// Strip null-byte padding from binary-patched sentinel values.
	serverURL = strings.TrimRight(serverURL, "\x00")
	apiKey = strings.TrimRight(apiKey, "\x00")
	version = strings.TrimRight(version, "\x00")

	// Subcommand dispatch (status, open, quit, install-autostart, version).
	// On a recognized subcommand, dispatch() calls os.Exit and never returns.
	// Bare invocation (no args) falls through to the daemon path below.
	if dispatch(os.Args, version) {
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger.Info("trasker client starting", "version", version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// First-run setup
	db, err := setupDatabase(ctx, logger)
	if err != nil {
		logger.Error("setup failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load config
	var deviceID string
	var trackingOn int
	err = db.QueryRow(`SELECT device_id, tracking_on FROM config WHERE id = 1`).Scan(&deviceID, &trackingOn)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// --- Initialize components ---

	// Tagger
	tagApplier := func(eventID, tagID, ruleID int64) error {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO event_tags (event_id, tag_id, source) VALUES (?, ?, 'auto_rule')`,
			eventID, tagID,
		)
		return err
	}
	tg, err := tagger.NewTagger(db, tagApplier, logger, learnerThreshold)
	if err != nil {
		logger.Error("failed to init tagger", "error", err)
		os.Exit(1)
	}

	// Pomodoro
	pomodoroTimer, err := pomodoro.NewTimer(pomodoro.DefaultConfig())
	if err != nil {
		logger.Error("failed to init pomodoro timer", "error", err)
		os.Exit(1)
	}

	// Sync client + queue
	syncClient := syncpkg.NewClient(serverURL, apiKey)
	syncQueue := syncpkg.NewQueue(db, syncClient, deviceID, logger)

	// Register device with server (non-blocking, best effort)
	go func() {
		hostname, _ := os.Hostname()
		err := syncClient.RegisterDevice(ctx, syncpkg.DeviceRegistration{
			ClientDeviceID: deviceID,
			OS:             runtime.GOOS,
			Hostname:       hostname,
		})
		if err != nil {
			logger.Warn("device registration failed (will retry)", "error", err)
		} else {
			logger.Info("device registered with server")
		}
	}()

	// Start submission queue
	syncQueue.Start(ctx)

	// Web UI
	webServer := webui.NewServer(db, defaultWebPort, logger)
	if err := webServer.Start(); err != nil {
		logger.Error("failed to start web server", "error", err)
		os.Exit(1)
	}
	logger.Info("dashboard available at", "url", webServer.URL())

	// --- Runtime state files (Phase 10) ---
	// Pidfile + URL file under XDG_STATE_HOME/trasker. Surfaced via
	// `trasker-client status / open / quit`. Stale-pid → overwrite.
	// ErrAlreadyRunning → another live daemon → bail to avoid double-start.
	if err := clientruntime.WriteState(webServer.Port()); err != nil {
		if errors.Is(err, clientruntime.ErrAlreadyRunning) {
			logger.Error("another trasker-client daemon is already running",
				"hint", "use `trasker-client quit` to stop it, or check your tray")
			os.Exit(1)
		}
		// Non-fatal: a missing state file just means status/open/quit
		// can't find us. Tracking and sync still work. Log and continue.
		logger.Warn("failed to write runtime state", "error", err)
	} else {
		// Best-effort cleanup on exit — also done in the shutdown block
		// below for clean exits, but a deferred Clear catches panics.
		defer func() {
			if err := clientruntime.Clear(); err != nil {
				logger.Warn("failed to clear runtime state on exit", "error", err)
			}
		}()
	}

	// Status snapshot — live values surfaced via /api/status.
	// Updated by goroutines below; reads are atomic.Pointer / atomic
	// loads so the HTTP handler doesn't block on long-running work.
	statusSnap := newStatusSnapshot(db)
	webServer.SetStatusProvider(statusSnap)

	// Always open browser and send notification on startup.
	// The system tray is a bonus if the desktop supports it.
	go openDashboardAndNotify(webServer.URL(), logger)

	// Tagger suggestion listener
	go func() {
		for suggestion := range tg.Suggestions() {
			logger.Info("new tag rule suggestion",
				"tag", suggestion.TagName,
				"app", suggestion.AppPattern,
				"title", suggestion.TitlePattern,
				"observed", suggestion.ObservedN,
			)
		}
	}()

	// Pomodoro state change listener
	go func() {
		for change := range pomodoroTimer.Changes() {
			logger.Info("pomodoro state change",
				"from", change.From,
				"to", change.To,
			)
		}
	}()

	// --- Focus Tracker ---
	// Tracks which window has focus and stores events in SQLite.
	// On non-CGo builds (cross-compiled), X11 tracking is unavailable;
	// only Wayland (gdbus-based) works. The client runs without tracking
	// if the platform tracker can't be created.
	var focusTracker tracker.Tracker
	if trackingOn == 1 {
		ft, err := tracker.NewPlatformTracker()
		if err != nil {
			logger.Warn("focus tracking unavailable", "error", err)
		} else {
			focusTracker = ft
			if err := focusTracker.Start(ctx); err != nil {
				logger.Warn("focus tracker failed to start", "error", err)
				focusTracker = nil
			} else {
				logger.Info("focus tracker started")
			}
		}
	} else {
		logger.Info("tracking is disabled in config")
	}

	// --- Presence Detector ---
	// Monitors screen lock (D-Bus) and deadman's switch to detect idle/away.
	deadman := presence.DefaultDeadman()
	deadman.Start(ctx)

	var screenLock presence.ScreenLockMonitor
	if sl, err := presence.NewScreenLockListener(); err != nil {
		logger.Warn("screen lock detection unavailable", "error", err)
	} else {
		if err := sl.Start(ctx); err != nil {
			logger.Warn("screen lock listener failed to start", "error", err)
		} else {
			screenLock = sl
			logger.Info("screen lock listener started")
		}
	}

	// Seed status defaults: tracking ON at startup, screen unlocked.
	// These are corrected by the listeners below as events arrive.
	statusSnap.setPresence("TRACKING")
	statusSnap.setScreenLock("UNLOCKED")

	// Presence state listener — pauses/resumes tracking based on lock state.
	//
	// The screenlock listener has a single Events() channel; layout.Capturer
	// also needs lock events (D-08). We fan out here so each downstream
	// consumer gets every event. layoutLockCh is the layout-side mirror.
	var layoutLockCh chan presence.StateChange
	if screenLock != nil {
		layoutLockCh = make(chan presence.StateChange, 4)
		go func() {
			defer close(layoutLockCh)
			for change := range screenLock.Events() {
				// Fan out to the layout capturer first (non-blocking — its
				// buffer absorbs bursts; if it's somehow full we'd rather
				// drop a stale lock-state ping than block focus tracking).
				select {
				case layoutLockCh <- change:
				default:
				}
				logger.Info("screen lock state change", "state", change.State.String())
				// Mirror lock state to the status snapshot so /api/status
				// reflects current desktop state.
				if change.State == presence.Away {
					statusSnap.setScreenLock("LOCKED")
				} else {
					statusSnap.setScreenLock("UNLOCKED")
				}
				if change.State == presence.Away {
					// Close current focus event when screen locks
					closeCurrentFocusEvent(db, logger)
				}
			}
		}()
	}

	// Deadman state listener
	go func() {
		for change := range deadman.States() {
			logger.Info("presence state change", "state", change.State.String())
			statusSnap.setPresence(change.State.String())
			if change.State == presence.Paused {
				closeCurrentFocusEvent(db, logger)
			}
		}
	}()

	// Focus event recorder — stores events in SQLite, feeds to tagger
	if focusTracker != nil {
		go func() {
			for event := range focusTracker.Events() {
				// Reset deadman timer on any focus change
				deadman.Reset()

				// Store focus event
				now := event.Timestamp.Format(time.RFC3339)
				// Close previous open event
				closeCurrentFocusEvent(db, logger)

				result, err := db.Exec(
					`INSERT INTO focus_events (app_name, window_title, started_at, is_idle, created_at)
					 VALUES (?, ?, ?, 0, ?)`,
					event.AppName, event.WindowTitle, now, now,
				)
				if err != nil {
					logger.Error("failed to store focus event", "error", err)
					continue
				}
				eventID, _ := result.LastInsertId()

				// Auto-tag via tagger engine
				tg.ProcessFocusEvent(ctx, tagger.FocusEvent{
					ID:          eventID,
					AppName:     event.AppName,
					WindowTitle: event.WindowTitle,
				})
			}
		}()
	}

	// --- Layout Snapshots (Phase 7) ---
	// Capture the open-window set every 60s when changed, sync to the
	// server every 5 minutes, prune local rows > 7 days every 6 hours.
	// Linux X11 only in v1 — see .planning/phases/07-layout-snapshots/07-CONTEXT.md D-04.
	// Gracefully no-ops when enumerator is unavailable (CGO_ENABLED=0,
	// macOS, Windows, no DISPLAY) — focus tracking + sync continue normally.

	layoutStore := layout.NewStore(db)

	var (
		layoutCapturer *layout.Capturer
		layoutSyncer   *layout.Syncer
	)
	if layoutEnum, err := layout.NewPlatformEnumerator(); err != nil {
		// ErrUnsupported on CGO_ENABLED=0 builds, macOS, Windows, or
		// no-DISPLAY environments. Intentional — focus tracking, presence,
		// sync of focus_events, and submit-flow remain unaffected.
		logger.Warn("layout snapshots unavailable on this build/platform; continuing without",
			"error", err)
	} else {
		layoutCapturer = layout.NewCapturer(
			layoutEnum,
			layoutStore,
			layoutLockCh, // nil-safe: Capturer treats nil as always-unlocked
			60*time.Second,
			logger,
		)
		layoutCapturer.Start(ctx)

		layoutSyncer = layout.NewSyncer(layout.SyncerConfig{
			Store:      layoutStore,
			DB:         db,
			DeviceID:   deviceID,
			ServerURL:  serverURL,
			APIKey:     apiKey,
			HTTPClient: http.DefaultClient,
			Interval:   5 * time.Minute,
			Logger:     logger,
			BatchSize:  100,
		})
		layoutSyncer.Start(ctx)

		go layout.RunPrune(ctx, layoutStore, 7*24*time.Hour, 6*time.Hour, logger)

		logger.Info("layout snapshots wired (capturer 60s, syncer 5m, prune 6h/7d)")
	}

	// --- System Tray (best-effort) ---
	// The tray is a bonus — the web dashboard is the primary UI.
	// On environments without a system tray (headless, no D-Bus, Wayland-only),
	// the app keeps running via web UI + sync.

	trayActions := &clientTrayActions{
		db:            db,
		webURL:        webServer.URL(),
		pomodoroTimer: pomodoroTimer,
		cancel:        cancel,
		logger:        logger,
	}

	sysTray := tray.New(trayActions)
	trayActions.tray = sysTray

	// Run tray in a goroutine — it may block forever even if the icon never appears.
	go sysTray.Run()

	// Main goroutine blocks on shutdown signal or quit from web UI.
	select {
	case <-sigCh:
		logger.Info("received shutdown signal")
	case <-webServer.QuitCh():
		logger.Info("quit requested from web UI")
	}

	cancel()

	if focusTracker != nil {
		focusTracker.Stop()
	}
	deadman.Stop()
	if screenLock != nil {
		screenLock.Stop()
	}
	if layoutCapturer != nil {
		layoutCapturer.Stop()
	}
	if layoutSyncer != nil {
		layoutSyncer.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	webServer.Stop(shutdownCtx)
	syncQueue.Stop()
	sysTray.Quit()

	// Clean-exit removal of runtime state files. The deferred Clear
	// above is the panic-safety net; this is the normal path.
	if err := clientruntime.Clear(); err != nil {
		logger.Warn("failed to clear runtime state", "error", err)
	}

	logger.Info("trasker client stopped")
}

// --- StatusSnapshot ---
//
// statusSnapshot implements webui.StatusProvider. The presence and
// lock fields are written from event-loop goroutines; the layout +
// sync timestamp queries hit SQLite each call but are bounded —
// /api/status is consumed by `trasker-client status` (interactive)
// not a tight polling loop.
//
// Atomic.Pointer keeps the writers and the HTTP handler off each
// other's locks. The DB is read-only here (the daemon owns the
// writes elsewhere).
type statusSnapshot struct {
	db          *sql.DB
	presence    atomic.Pointer[string]
	screenLock  atomic.Pointer[string]
}

func newStatusSnapshot(db *sql.DB) *statusSnapshot {
	return &statusSnapshot{db: db}
}

func (s *statusSnapshot) setPresence(state string) {
	v := state
	s.presence.Store(&v)
}

func (s *statusSnapshot) setScreenLock(state string) {
	v := state
	s.screenLock.Store(&v)
}

// PresenceState returns the most recent presence state name, or "" if
// no event has been seen yet.
func (s *statusSnapshot) PresenceState() string {
	if p := s.presence.Load(); p != nil {
		return *p
	}
	return ""
}

// ScreenLockState returns "LOCKED" / "UNLOCKED" / "" (unknown).
func (s *statusSnapshot) ScreenLockState() string {
	if p := s.screenLock.Load(); p != nil {
		return *p
	}
	return ""
}

// LastLayoutSnapshot returns MAX(captured_at) from layout_snapshots.
// Zero Time when the table is empty (layout disabled or no captures yet).
func (s *statusSnapshot) LastLayoutSnapshot() time.Time {
	var raw sql.NullString
	err := s.db.QueryRow(
		`SELECT MAX(captured_at) FROM layout_snapshots`).Scan(&raw)
	if err != nil || !raw.Valid {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

// LastServerSync returns the most recent successful sync time across
// focus-event submissions (submissions.submitted_at where status =
// 'sent') and layout snapshots (layout_snapshots.synced_at).
//
// Zero Time when nothing has ever been synced — e.g. fresh install
// running offline.
func (s *statusSnapshot) LastServerSync() time.Time {
	var bestStr sql.NullString

	// Focus-event submissions: the queue marks status='sent' on
	// successful upload; submitted_at is the wall-clock time the
	// row was queued, which is close enough for "last sync".
	var subTime sql.NullString
	if err := s.db.QueryRow(
		`SELECT MAX(submitted_at) FROM submissions WHERE status = 'sent'`,
	).Scan(&subTime); err == nil && subTime.Valid {
		bestStr = subTime
	}

	// Layout snapshots: synced_at is set by layout.Syncer on success.
	var layoutTime sql.NullString
	if err := s.db.QueryRow(
		`SELECT MAX(synced_at) FROM layout_snapshots WHERE synced_at IS NOT NULL`,
	).Scan(&layoutTime); err == nil && layoutTime.Valid {
		// Pick whichever is later.
		if !bestStr.Valid || layoutTime.String > bestStr.String {
			bestStr = layoutTime
		}
	}

	if !bestStr.Valid {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, bestStr.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

func setupDatabase(ctx context.Context, logger *slog.Logger) (*sql.DB, error) {
	isFirst, err := setup.IsFirstRun()
	if err != nil {
		return nil, fmt.Errorf("check first run: %w", err)
	}

	if isFirst {
		logger.Info("first run detected, initializing...")
		fr := setup.NewFirstRun(setup.Config{
			ServerURL: serverURL,
			APIKey:    apiKey,
		}, logger)
		db, err := fr.Run(ctx)
		if err != nil {
			return nil, err
		}
		// Open browser to dashboard
		go func() {
			setup.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d", defaultWebPort))
		}()
		return db, nil
	}

	// Existing install — just open DB
	dbPath, err := setup.DBPath()
	if err != nil {
		return nil, err
	}
	return sql.Open("sqlite", dbPath)
}

// clientTrayActions implements tray.Actions.
type clientTrayActions struct {
	db            *sql.DB
	webURL        string
	pomodoroTimer *pomodoro.Timer
	cancel        context.CancelFunc
	logger        *slog.Logger
	tray          *tray.Tray
}

func (a *clientTrayActions) OpenDashboard() {
	if err := setup.OpenBrowser(a.webURL); err != nil {
		a.logger.Error("failed to open browser", "error", err)
	}
}

func (a *clientTrayActions) StartPomodoro() {
	if err := a.pomodoroTimer.Start(nil); err != nil {
		a.logger.Warn("pomodoro start failed", "error", err)
	}
}

func (a *clientTrayActions) StopPomodoro() {
	a.pomodoroTimer.Cancel()
}

func (a *clientTrayActions) IsTrackingOn() bool {
	var on int
	a.db.QueryRow(`SELECT tracking_on FROM config WHERE id = 1`).Scan(&on)
	return on == 1
}

func (a *clientTrayActions) SetTrackingOn(on bool) {
	val := 0
	if on {
		val = 1
	}
	a.db.Exec(`UPDATE config SET tracking_on = ? WHERE id = 1`, val)
	a.logger.Info("tracking toggled", "on", on)
}

func (a *clientTrayActions) IsAutostartOn() bool {
	var on int
	a.db.QueryRow(`SELECT autostart FROM config WHERE id = 1`).Scan(&on)
	return on == 1
}

func (a *clientTrayActions) SetAutostartOn(on bool) {
	val := 0
	if on {
		val = 1
	}
	a.db.Exec(`UPDATE config SET autostart = ? WHERE id = 1`, val)

	// Create/remove autostart entry
	execPath, _ := os.Executable()
	autostart := setup.NewAutostart(execPath)
	if on {
		autostart.Enable()
	} else {
		autostart.Disable()
	}
	a.logger.Info("autostart toggled", "on", on)
}

func (a *clientTrayActions) Quit() {
	a.logger.Info("quit requested from tray")
	a.cancel()
}

// openDashboardAndNotify opens the browser to the web dashboard and sends a
// desktop notification. Called on every startup — the browser is the primary UI.
func openDashboardAndNotify(webURL string, logger *slog.Logger) {
	// Open browser to dashboard
	if err := setup.OpenBrowser(webURL); err != nil {
		logger.Warn("failed to open browser", "error", err)
	} else {
		logger.Info("opened browser to dashboard", "url", webURL)
	}

	// Send desktop notification (best-effort — notification service may also be unavailable)
	notifier, err := notify.New()
	if err != nil {
		logger.Warn("desktop notifications unavailable", "error", err)
		return
	}
	err = notifier.Notify(
		"Trasker is running",
		fmt.Sprintf("Dashboard: %s — use Ctrl+C in terminal to stop", webURL),
		func() { setup.OpenBrowser(webURL) },
	)
	if err != nil {
		logger.Warn("failed to send notification", "error", err)
	}
	// Don't close notifier — keep it alive so click callbacks work.
}

// closeCurrentFocusEvent closes the most recent open focus event (one without an ended_at).
// Called when focus changes, screen locks, or deadman fires.
//
// Delegates to store.CloseLatestOpenFocusEvent which uses a subquery form of
// UPDATE compatible with the pure-Go modernc.org/sqlite driver (which lacks
// SQLITE_ENABLE_UPDATE_DELETE_LIMIT, so `UPDATE ... ORDER BY ... LIMIT` is
// rejected as a syntax error).
func closeCurrentFocusEvent(db *sql.DB, logger *slog.Logger) {
	n, err := store.CloseLatestOpenFocusEvent(db, time.Now())
	if err != nil {
		logger.Warn("failed to close focus event", "error", err)
		return
	}
	if n > 0 {
		logger.Debug("closed focus event")
	}
}
