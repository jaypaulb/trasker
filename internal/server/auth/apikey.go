package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// APIKeyRecord holds the data needed to validate an API key.
type APIKeyRecord struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	KeyHash   string
	Role      string
	ExpiresAt time.Time
	Revoked   bool
}

// APIKeyLookup is the interface the middleware needs from the store.
type APIKeyLookup interface {
	ListAllActiveAPIKeys(ctx context.Context) ([]APIKeyRecord, error)
	UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
}

// APIKeyMiddleware returns middleware that authenticates requests via API key.
// The key is expected in the Authorization header as "Bearer <key>".
// Because API keys are bcrypt-hashed, we must compare against all active keys.
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

			keys, err := store.ListAllActiveAPIKeys(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
				return
			}

			var matched *APIKeyRecord
			for i := range keys {
				if err := bcrypt.CompareHashAndPassword([]byte(keys[i].KeyHash), []byte(plainKey)); err == nil {
					matched = &keys[i]
					break
				}
			}

			if matched == nil {
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

			// Update last_used_at (fire-and-forget, don't block the request)
			go store.UpdateAPIKeyLastUsed(context.Background(), matched.ID)

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
	json.NewEncoder(w).Encode(v)
}
