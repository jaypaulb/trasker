package api

import (
	"net/http"
	"time"

	"github.com/jaypaulb/trasker/internal/shared/version"
)

func healthHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		httpStatus := http.StatusOK

		// If store is available, check DB connectivity
		if deps.Store != nil {
			if err := deps.Store.Ping(r.Context()); err != nil {
				status = "degraded"
				httpStatus = http.StatusServiceUnavailable
				deps.Logger.Error("health check: database ping failed", "error", err)
			}
		}

		respondJSON(w, httpStatus, map[string]any{
			"status":    status,
			"version":   version.String(),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}
