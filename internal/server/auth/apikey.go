package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/shared/apikey"
	"github.com/jaypaulb/trasker/internal/shared/models"
	"golang.org/x/crypto/bcrypt"
)

// APIKeyRecord holds the data needed to validate an API key.
type APIKeyRecord struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	KeyHash   string
	Role      models.Role
	ExpiresAt time.Time
	Revoked   bool
}

// APIKeyLookup is the interface the middleware needs from the store.
type APIKeyLookup interface {
	GetActiveAPIKeyByPrefix(ctx context.Context, prefix string) (*APIKeyRecord, error)
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
}

// APIKeyMiddleware returns middleware that authenticates requests via API key.
// The key is expected in the Authorization header as "Bearer <key>".
// Uses the key prefix to narrow lookup to a single candidate before bcrypt comparison.
func APIKeyMiddleware(store APIKeyLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing Authorization header"})
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid Authorization header format"})
				return
			}

			plainKey := parts[1]

			// Extract prefix to narrow lookup to a single candidate
			prefix := apikey.Prefix(plainKey)
			if prefix == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid API key format"})
				return
			}

			matched, err := store.GetActiveAPIKeyByPrefix(r.Context(), prefix)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid API key"})
				return
			}

			// Single bcrypt comparison against the matched candidate
			if err := bcrypt.CompareHashAndPassword([]byte(matched.KeyHash), []byte(plainKey)); err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid API key"})
				return
			}

			if matched.Revoked {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "API key has been revoked"})
				return
			}

			if time.Now().After(matched.ExpiresAt) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error":   "API key has expired",
					"message": "Please download a new client from your Trasker dashboard",
				})
				return
			}

			// Update last_used_at with bounded context and error logging
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			go func() {
				defer updateCancel()
				if err := store.UpdateAPIKeyLastUsed(updateCtx, matched.ID); err != nil {
					slog.Warn("failed to update API key last_used_at", "key_id", matched.ID, "error", err)
				}
			}()

			// Set context values
			ctx := r.Context()
			ctx = WithUserID(ctx, matched.UserID)
			ctx = WithUserRole(ctx, matched.Role)
			ctx = WithAPIKeyID(ctx, matched.ID)
			ctx = WithAuthType(ctx, AuthTypeAPIKey)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
