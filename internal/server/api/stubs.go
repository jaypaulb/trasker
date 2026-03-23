package api

import "net/http"

// Stub handlers — replaced in subsequent tasks.
// Each returns an http.HandlerFunc so the router compiles.

// healthHandler stub — replaced in Task 13 with health.go.
func healthHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondJSON(w, http.StatusOK, map[string]string{"status": "ok"}) }
}

func deviceRegisterHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func deviceListHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func deviceUpdateHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func timesheetSubmitHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func timesheetListOwnHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func timesheetListTeamHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func authLoginHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func authRefreshHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func userListHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func userUpdateRoleHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func reportSummaryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func reportExportHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func adminDeleteEntryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func adminEditEntryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func adminRevokeKeyHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func adminAuditLogHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func adminGetSettingsHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}

func adminUpdateSettingsHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respondError(w, http.StatusNotImplemented, "not implemented") }
}
