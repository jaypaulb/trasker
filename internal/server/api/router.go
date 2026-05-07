package api

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/builder"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/jaypaulb/trasker/internal/server/webui"
	"github.com/jaypaulb/trasker/internal/shared/models"
)

// Dependencies holds all the dependencies the API handlers need.
type Dependencies struct {
	Store      *store.Store
	JWTIssuer  *auth.JWTIssuer
	OIDCConfig *auth.OIDCConfig
	APIKeyAuth auth.APIKeyLookup
	Builder    *builder.Builder
	Logger     *slog.Logger
	// FQDN is the public hostname when running behind autocert TLS. When set,
	// it is used to derive absolute URLs (client binary server URL, OIDC
	// redirect URL). Empty in local-dev / plain-HTTP mode.
	FQDN string
}

// initOIDCFromSettings lazily initializes OIDC from the org_settings table.
// This allows admins to configure OIDC via the UI without restarting the server.
func (d *Dependencies) initOIDCFromSettings(ctx context.Context) error {
	if d.Store == nil {
		return fmt.Errorf("no store available")
	}
	settings, err := d.Store.GetOrgSettings(ctx)
	if err != nil {
		return fmt.Errorf("reading org settings: %w", err)
	}
	if settings.EntraTenant == "" || settings.EntraClient == "" {
		return fmt.Errorf("entra tenant and client must be configured in admin settings")
	}
	if settings.EntraSecret == "" {
		return fmt.Errorf("entra secret must be configured in admin settings")
	}

	oidcCfg, err := auth.NewOIDCConfig(auth.OIDCParams{
		TenantID:     settings.EntraTenant,
		ClientID:     settings.EntraClient,
		ClientSecret: settings.EntraSecret,
		RedirectURL:  entraRedirectURL(),
	})
	if err != nil {
		return fmt.Errorf("creating OIDC config: %w", err)
	}
	if err := oidcCfg.InitProvider(ctx, settings.EntraTenant); err != nil {
		return fmt.Errorf("initializing OIDC provider: %w", err)
	}

	d.OIDCConfig = oidcCfg
	d.Logger.Info("OIDC initialized from org_settings", "tenant", settings.EntraTenant)
	return nil
}

// NewRouter creates the Chi router with all routes and middleware.
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	if deps.Logger != nil {
		r.Use(structuredLogger(deps.Logger))
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Public endpoints
		r.Get("/health", healthHandler(deps))
		r.Get("/auth/config", authConfigHandler(deps))
		r.Post("/auth/local-login", localLoginHandler(deps))

		// Client API endpoints (API key auth)
		if deps.APIKeyAuth != nil {
			r.Group(func(r chi.Router) {
				r.Use(auth.APIKeyMiddleware(deps.APIKeyAuth))

				r.Post("/devices", deviceRegisterHandler(deps))
				r.Get("/devices", deviceListHandler(deps))
				r.Patch("/devices/{id}", deviceUpdateHandler(deps))

				r.Post("/timesheets", timesheetSubmitHandler(deps))
				r.Get("/timesheets", timesheetListOwnHandler(deps))

				// Layout snapshots ingest (Phase 7)
				r.Post("/layout-snapshots", layoutIngestHandler(deps))
			})
		}

		// Dashboard endpoints (JWT auth)
		if deps.JWTIssuer != nil {
			r.Group(func(r chi.Router) {
				r.Use(auth.JWTMiddleware(deps.JWTIssuer))

				// Auth
				r.Post("/auth/refresh", authRefreshHandler(deps))
				r.Post("/auth/change-password", changePasswordHandler(deps))

				// Dashboard data (own)
				r.Get("/timesheets", timesheetListOwnHandler(deps))
				r.Get("/devices", deviceListHandler(deps))

				// Layout snapshots dashboard reads (Phase 7)
				r.Get("/layout-snapshots", layoutAtHandler(deps))
				r.Get("/layout-snapshots/timeline", layoutTimelineHandler(deps))

				// Timesheets (team view, manager+)
				r.Get("/timesheets/team", auth.RequireRole(models.RoleManager, models.RoleAdmin)(http.HandlerFunc(timesheetListTeamHandler(deps))).ServeHTTP)

				// Users
				r.Get("/users/team", auth.RequireRole(models.RoleManager, models.RoleAdmin)(http.HandlerFunc(userListHandler(deps))).ServeHTTP)
				r.Group(func(r chi.Router) {
					r.Use(auth.RequireRole(models.RoleAdmin))
					r.Get("/users", userListHandler(deps))
					r.Patch("/users/{id}", userUpdateRoleHandler(deps))
				})

				// Reports (manager+)
				r.Group(func(r chi.Router) {
					r.Use(auth.RequireRole(models.RoleManager, models.RoleAdmin))
					r.Get("/reports/summary", reportSummaryHandler(deps))
					r.Get("/reports/export", reportExportHandler(deps))
				})

				// Build (any authenticated user can download a client)
				r.Post("/build/download", buildDownloadHandler(deps))
				r.Get("/build/status", buildStatusHandler(deps))

				// Admin
				r.Group(func(r chi.Router) {
					r.Use(auth.RequireRole(models.RoleAdmin))
					r.Delete("/admin/entries/{id}", adminDeleteEntryHandler(deps))
					r.Patch("/admin/entries/{id}", adminEditEntryHandler(deps))
					r.Post("/admin/keys/{id}/revoke", adminRevokeKeyHandler(deps))
					r.Get("/admin/audit", adminAuditLogHandler(deps))
					r.Get("/admin/keys", adminListKeysHandler(deps))
					r.Get("/admin/settings", adminGetSettingsHandler(deps))
					r.Patch("/admin/settings", adminUpdateSettingsHandler(deps))
				})
			})

			// OIDC callback (no auth required)
			r.Post("/auth/login", authLoginHandler(deps))
		}
	})

	// Serve embedded dashboard SPA for all non-API routes.
	// The SPA handles client-side routing — unknown paths get index.html.
	spaFS, err := fs.Sub(webui.Assets, "static")
	if err != nil {
		// Should never happen — embedded FS is compile-time
		panic(fmt.Sprintf("embedded SPA filesystem: %v", err))
	}
	fileServer := http.FileServer(http.FS(spaFS))

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly. If it doesn't exist, fall through to
		// index.html (SPA client-side routing).
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		f, err := spaFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// File doesn't exist — serve index.html for SPA routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	return r
}

// entraRedirectURL returns the OIDC redirect URL.
// Precedence: explicit ENTRA_REDIRECT_URL > derived from TRASKER_FQDN >
// localhost dev default.
func entraRedirectURL() string {
	if u := os.Getenv("ENTRA_REDIRECT_URL"); u != "" {
		return u
	}
	if fqdn := os.Getenv("TRASKER_FQDN"); fqdn != "" {
		return fmt.Sprintf("https://%s/auth/callback", fqdn)
	}
	return "http://localhost:5173/auth/callback"
}

// structuredLogger is a simple slog-based request logger middleware.
func structuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", r.RemoteAddr,
			)
		})
	}
}
