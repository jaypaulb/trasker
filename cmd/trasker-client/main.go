// cmd/trasker-client/main.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	_ "modernc.org/sqlite"

	"github.com/jaypaulb/trasker/internal/client/notify"
	"github.com/jaypaulb/trasker/internal/client/pomodoro"
	"github.com/jaypaulb/trasker/internal/client/setup"
	syncpkg "github.com/jaypaulb/trasker/internal/client/sync"
	"github.com/jaypaulb/trasker/internal/client/tagger"
	"github.com/jaypaulb/trasker/internal/client/tray"
	"github.com/jaypaulb/trasker/internal/client/webui"
)

// Build-time values set via ldflags.
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
	pomodoroTimer := pomodoro.NewTimer(pomodoro.DefaultConfig())

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

	// TODO: Wire focus tracker and presence detector from Plan 03
	// The session engine from Plan 03 would feed focus events to:
	// - tg.ProcessFocusEvent(ctx, event)   for auto-tagging
	// - notification dispatcher             for deadman/nag
	// These are wired once Plan 03 components are available.

	// --- System Tray ---
	// Tray must run on the main goroutine (macOS requirement).
	// All other work happens in goroutines above.

	trayActions := &clientTrayActions{
		db:            db,
		webURL:        webServer.URL(),
		pomodoroTimer: pomodoroTimer,
		cancel:        cancel,
		logger:        logger,
	}

	// Run tray in a goroutine if needed, or on main thread
	// On macOS this must be on the main thread. On Linux it can be a goroutine.
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
		webServer.Stop(ctx)
		syncQueue.Stop()
		trayActions.tray.Quit()
	}()

	sysTray := tray.New(trayActions)
	trayActions.tray = sysTray
	sysTray.Run() // blocks until quit

	logger.Info("trasker client stopped")
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

	// Notification integration placeholder
	notifier notify.Notifier
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
