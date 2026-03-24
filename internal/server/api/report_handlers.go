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
		filters := parseReportFilters(r)

		summary, err := deps.Store.GetReportSummary(r.Context(), filters)
		if err != nil {
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
		filters := parseReportFilters(r)

		rows, err := deps.Store.GetReportExport(r.Context(), filters)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate export")
			return
		}

		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=trasker-export.csv")
		w.WriteHeader(http.StatusOK)

		writer := csv.NewWriter(w)
		defer writer.Flush()

		// Header row
		writer.Write([]string{"Email", "Name", "Tag", "Started", "Ended", "Duration (s)", "Duration (h)", "Notes", "App Summary", "Submitted"})

		for _, row := range rows {
			notes := ""
			if row.Notes != nil {
				notes = *row.Notes
			}
			appSummary := ""
			if row.AppSummary != nil {
				appSummary = *row.AppSummary
			}

			writer.Write([]string{
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
			})
		}
	}
}

func parseReportFilters(r *http.Request) store.ReportFilters {
	filters := store.ReportFilters{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		if uid, err := uuid.Parse(userIDStr); err == nil {
			filters.UserID = &uid
		}
	}
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if from, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filters.From = &from
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if to, err := time.Parse(time.RFC3339, toStr); err == nil {
			filters.To = &to
		}
	}

	return filters
}
