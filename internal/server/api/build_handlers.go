package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/builder"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/jaypaulb/trasker/internal/shared/apikey"
	"github.com/jaypaulb/trasker/internal/shared/version"
)

func buildDownloadHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Builder == nil {
			respondError(w, http.StatusServiceUnavailable, "build service not available (Go toolchain not installed on server)")
			return
		}

		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}

		var req struct {
			TargetOS   string `json:"target_os"`
			TargetArch string `json:"target_arch"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		target, err := builder.ValidateTarget(req.TargetOS, req.TargetArch)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Check if pre-built binary is cached
		if !deps.Builder.IsCached(target) {
			respondError(w, http.StatusServiceUnavailable, "client binaries are still being built — try again in a minute")
			return
		}

		// Generate a new API key for this download
		plainKey, keyHash, keyPrefix, err := apikey.Generate()
		if err != nil {
			deps.Logger.Error("failed to generate API key", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to generate API key")
			return
		}

		// Determine key expiry from org settings (fall back to 90 days)
		expiryDays := 90
		if settings, err := deps.Store.GetOrgSettings(r.Context()); err == nil && settings.KeyExpiryDays > 0 {
			expiryDays = settings.KeyExpiryDays
		}

		if _, err := deps.Store.CreateAPIKey(r.Context(), store.CreateAPIKeyParams{
			UserID:    userID,
			KeyHash:   keyHash,
			KeyPrefix: keyPrefix,
			ExpiresAt: time.Now().Add(time.Duration(expiryDays) * 24 * time.Hour),
		}); err != nil {
			deps.Logger.Error("failed to store API key", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to create API key")
			return
		}

		// Derive server URL: prefer TRASKER_FQDN (canonical public hostname),
		// otherwise fall back to the incoming request.
		var serverURL string
		if deps.FQDN != "" {
			serverURL = fmt.Sprintf("https://%s", deps.FQDN)
		} else {
			scheme := "https"
			if r.TLS == nil {
				if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
					scheme = fwd
				} else {
					scheme = "http"
				}
			}
			serverURL = fmt.Sprintf("%s://%s", scheme, r.Host)
		}

		// Patch the cached binary with real values (~50ms)
		binaryData, err := deps.Builder.Patch(builder.PatchRequest{
			Target:    target,
			APIKey:    plainKey,
			ServerURL: serverURL,
			Version:   version.Version,
		})
		if err != nil {
			deps.Logger.Error("binary patch failed", "error", err, "target", target.String())
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("patch failed: %v", err))
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, target.BinaryName()))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(binaryData)))
		w.WriteHeader(http.StatusOK)
		w.Write(binaryData)
	}
}

func buildStatusHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Builder == nil {
			respondJSON(w, http.StatusOK, map[string]any{
				"available": false,
				"reason":    "Go toolchain not installed on server",
				"targets":   []any{},
			})
			return
		}

		cacheStatus := deps.Builder.CacheStatus()
		targets := make([]map[string]any, len(builder.SupportedTargets))
		allCached := true
		for i, t := range builder.SupportedTargets {
			cached := cacheStatus[t.String()]
			if !cached {
				allCached = false
			}
			targets[i] = map[string]any{
				"os":     t.OS,
				"arch":   t.Arch,
				"name":   t.BinaryName(),
				"cached": cached,
			}
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"available": allCached,
			"targets":   targets,
		})
	}
}
