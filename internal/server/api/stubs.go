package api

import "net/http"

// Stub handlers — replaced in subsequent tasks.
// Each returns an http.HandlerFunc so the router compiles.

func reportSummaryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func reportExportHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}
