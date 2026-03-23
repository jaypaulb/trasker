package api

import "net/http"

// Stub handlers — replaced in subsequent tasks.
// Each returns an http.HandlerFunc so the router compiles.

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
