// internal/client/webui/server.go
package webui

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// StatusProvider exposes live runtime metrics for the /api/status
// endpoint. The daemon registers a provider after wiring its components;
// the API uses nil-safe accessors so the endpoint still works (with
// "unavailable" fields) before registration completes.
//
// All fields are best-effort snapshots — implementations should be
// goroutine-safe and non-blocking. The status endpoint is hit by
// `trasker-client status` and must not stall on slow DB queries.
type StatusProvider interface {
	// PresenceState returns a human-readable presence state name
	// ("TRACKING", "AWAY", "PAUSED", "CHECKING") or "" if unknown.
	PresenceState() string
	// ScreenLockState returns "LOCKED", "UNLOCKED", or "" if unknown.
	ScreenLockState() string
	// LastLayoutSnapshot returns the captured_at of the most recent
	// layout snapshot, or zero Time if none / layout disabled.
	LastLayoutSnapshot() time.Time
	// LastServerSync returns the wall-clock time of the most recent
	// successful server sync (focus events or layout snapshots),
	// or zero Time if none.
	LastServerSync() time.Time
}

// Server serves the local web dashboard and REST API.
type Server struct {
	db       *sql.DB
	logger   *slog.Logger
	mux      *http.ServeMux
	srv      *http.Server
	port     int
	listener net.Listener
	quitCh   chan struct{} // closed when quit is requested via API
	status   StatusProvider
	started  time.Time // server start time, used for uptime in /api/status
}

// NewServer creates a new webui server.
func NewServer(db *sql.DB, port int, logger *slog.Logger) *Server {
	s := &Server{
		db:     db,
		logger: logger,
		mux:    http.NewServeMux(),
		port:   port,
		quitCh: make(chan struct{}),
	}
	s.registerRoutes()
	return s
}

// Port returns the port the server is listening on.
// Useful when port 0 is used for testing.
func (s *Server) Port() int {
	if s.listener != nil {
		return s.listener.Addr().(*net.TCPAddr).Port
	}
	return s.port
}

// SetStatusProvider registers the live-metrics source used by
// /api/status. May be called before or after Start. Pass nil to
// detach (the endpoint then reports "unavailable" for those fields).
func (s *Server) SetStatusProvider(p StatusProvider) {
	s.status = p
}

// Start begins serving. Non-blocking.
func (s *Server) Start() error {
	var err error
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("webui: listen: %w", err)
	}
	s.started = time.Now()

	s.srv = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("webui server error", "error", err)
		}
	}()

	s.logger.Info("webui server started", "addr", s.listener.Addr().String())
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// URL returns the full local URL to access the dashboard.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Port())
}

// QuitCh returns a channel that is closed when quit is requested via the API.
func (s *Server) QuitCh() <-chan struct{} {
	return s.quitCh
}

// StartedAt returns the wall-clock time the server started serving.
// Used by /api/status to compute uptime. Returns zero Time before Start().
func (s *Server) StartedAt() time.Time {
	return s.started
}

// Status returns the registered StatusProvider, or nil if none.
// Used internally by the /api/status handler; exposed for tests.
func (s *Server) Status() StatusProvider {
	return s.status
}

// RequestQuit signals that the client should shut down (called by the quit API endpoint).
func (s *Server) RequestQuit() {
	select {
	case <-s.quitCh:
		// Already closed
	default:
		close(s.quitCh)
	}
}

func (s *Server) registerRoutes() {
	// Serve SPA static files
	staticFS, err := fs.Sub(Assets, "static")
	if err != nil {
		s.logger.Warn("webui: no embedded assets, SPA will not be served", "error", err)
	} else {
		s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}

	// API routes registered in api.go
	api := NewAPI(s.db, s.logger, s)
	api.RegisterRoutes(s.mux)
}
