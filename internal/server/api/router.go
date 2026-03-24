package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/jaypaulb/trasker/internal/shared/models"
)

// Dependencies holds all the dependencies the API handlers need.
type Dependencies struct {
	Store      *store.Store
	JWTIssuer  *auth.JWTIssuer
	OIDCConfig *auth.OIDCConfig
	APIKeyAuth auth.APIKeyLookup
	Logger     *slog.Logger
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

		// Client API endpoints (API key auth)
		if deps.APIKeyAuth != nil {
			r.Group(func(r chi.Router) {
				r.Use(auth.APIKeyMiddleware(deps.APIKeyAuth))

				r.Post("/devices", deviceRegisterHandler(deps))
				r.Get("/devices", deviceListHandler(deps))
				r.Patch("/devices/{id}", deviceUpdateHandler(deps))

				r.Post("/timesheets", timesheetSubmitHandler(deps))
				r.Get("/timesheets", timesheetListOwnHandler(deps))
			})
		}

		// Dashboard endpoints (JWT auth)
		if deps.JWTIssuer != nil {
			r.Group(func(r chi.Router) {
				r.Use(auth.JWTMiddleware(deps.JWTIssuer))

				// Auth
				r.Post("/auth/refresh", authRefreshHandler(deps))

				// Timesheets (dashboard view)
				r.Get("/timesheets/team", auth.RequireRole(models.RoleManager, models.RoleAdmin)(http.HandlerFunc(timesheetListTeamHandler(deps))).ServeHTTP)

				// Users (admin)
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

				// Admin
				r.Group(func(r chi.Router) {
					r.Use(auth.RequireRole(models.RoleAdmin))
					r.Delete("/admin/entries/{id}", adminDeleteEntryHandler(deps))
					r.Patch("/admin/entries/{id}", adminEditEntryHandler(deps))
					r.Post("/admin/keys/{id}/revoke", adminRevokeKeyHandler(deps))
					r.Get("/admin/audit", adminAuditLogHandler(deps))
					r.Get("/admin/settings", adminGetSettingsHandler(deps))
					r.Patch("/admin/settings", adminUpdateSettingsHandler(deps))
				})
			})

			// OIDC callback (no auth required)
			r.Post("/auth/login", authLoginHandler(deps))
		}
	})

	return r
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
