// internal/server/api/report_handlers.go
package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/store"
)

func reportSummaryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, err := parseReportFilters(r)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		summary, err := deps.Store.GetReportSummary(r.Context(), filters)
		if err != nil {
			deps.Logger.Error("failed to generate report summary", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to generate summary")
			return
		}

		result := make([]map[string]any, len(summary))
		for i, row := range summary {
			result[i] = map[string]any{
				"tag":              row.Tag,
				"total_duration_s": row.TotalDurationS,
				"entry_count":      row.EntryCount,
				"total_hours":      fmt.Sprintf("%.1f", float64(row.TotalDurationS)/3600),
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}

func reportExportHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, err := parseReportFilters(r)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		rows, err := deps.Store.GetReportExport(r.Context(), filters)
		if err != nil {
			deps.Logger.Error("failed to generate CSV export", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to generate export")
			return
		}

		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=trasker-export.csv")
		w.WriteHeader(http.StatusOK)

		writer := csv.NewWriter(w)

		// Header row
		if err := writer.Write([]string{"Email", "Name", "Tag", "Started", "Ended", "Duration (s)", "Duration (h)", "Notes", "App Summary", "Submitted"}); err != nil {
			deps.Logger.Error("failed to write CSV header", "error", err)
			return
		}

		for _, row := range rows {
			notes := ""
			if row.Notes != nil {
				notes = *row.Notes
			}
			appSummary := ""
			if row.AppSummary != nil {
				appSummary = *row.AppSummary
			}

			if err := writer.Write([]string{
				row.UserEmail,
				row.DisplayName,
				row.Tag,
				row.StartedAt.Format(time.RFC3339),
				row.EndedAt.Format(time.RFC3339),
				fmt.Sprintf("%d", row.DurationS),
				fmt.Sprintf("%.1f", float64(row.DurationS)/3600),
				notes,
				appSummary,
				row.SubmittedAt.Format(time.RFC3339),
			}); err != nil {
				deps.Logger.Error("failed to write CSV row", "error", err)
				return
			}
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			deps.Logger.Error("CSV flush failed", "error", err)
		}
	}
}

func parseReportFilters(r *http.Request) (store.ReportFilters, error) {
	filters := store.ReportFilters{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		uid, err := uuid.Parse(userIDStr)
		if err != nil {
			return filters, fmt.Errorf("invalid user_id: must be a valid UUID")
		}
		filters.UserID = &uid
	}
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return filters, fmt.Errorf("invalid from: must be RFC3339 format")
		}
		filters.From = &from
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return filters, fmt.Errorf("invalid to: must be RFC3339 format")
		}
		filters.To = &to
	}

	return filters, nil
}
