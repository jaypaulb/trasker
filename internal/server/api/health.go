package api

import (
	"net/http"
	"time"
)

func healthHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"

		// If store is available, check DB connectivity
		if deps.Store != nil {
			if err := deps.Store.Ping(r.Context()); err != nil {
				status = "degraded"
			}
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"status":    status,
			"version":   "dev", // replaced by version package in production
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}
