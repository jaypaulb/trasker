// Phase 7 layout snapshot downsampler.
//
// Runs hourly in-process: collapses raw rows aged > 7 days into 10-minute
// representatives, and tier='10min' rows aged > 37 days into 1-hour
// representatives. pg_cron is intentionally NOT used (not assumed installed
// on stock Postgres targets — see .planning/phases/07-layout-snapshots/07-RESEARCH.md
// Pitfall 4).
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jaypaulb/trasker/internal/server/store"
)

// downsamplerInitialDelay defers the first run until the server has settled.
// Prevents the downsampler from competing with startup-time DB connection
// establishment.
const downsamplerInitialDelay = 30 * time.Second

// downsamplerInterval is the cadence between downsampling sweeps.
// 1 hour matches the expected drift rate of the 7d / 37d retention boundaries
// — running more often would mostly find nothing to do.
const downsamplerInterval = 1 * time.Hour

// runLayoutDownsampler is intended to run as a long-lived goroutine started
// from main(). It blocks until ctx is cancelled.
//
// Errors are logged at WARN and never fatal: if the layout_snapshots table
// is missing, the downsampler simply logs the error each cycle. The user-
// facing 503 with operator instructions is emitted by the HTTP handlers,
// which is the correct surface for that signal.
func runLayoutDownsampler(ctx context.Context, s *store.Store, logger *slog.Logger) {
	// Initial settle delay, but bail early if the server is already shutting down.
	select {
	case <-ctx.Done():
		return
	case <-time.After(downsamplerInitialDelay):
	}

	ticker := time.NewTicker(downsamplerInterval)
	defer ticker.Stop()

	run := func() {
		now := time.Now().UTC()

		del10, err := s.DownsampleTo10Min(ctx, now)
		if err != nil {
			logger.Warn("layout downsample 10min failed", "error", err)
		} else if del10 > 0 {
			logger.Info("layout downsample 10min", "deleted", del10)
		}

		del1h, err := s.DownsampleTo1Hr(ctx, now)
		if err != nil {
			logger.Warn("layout downsample 1hr failed", "error", err)
		} else if del1h > 0 {
			logger.Info("layout downsample 1hr", "deleted", del1h)
		}
	}

	run() // initial run after settle delay
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
