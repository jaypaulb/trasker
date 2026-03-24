package auth

import (
	"net/http"
)

// RequireRole returns middleware that ensures the authenticated user has one of the specified roles.
// Must be chained AFTER an auth middleware (JWT or API key) that sets the role in context.
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := UserRoleFrom(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no authentication context"})
				return
			}

			if !allowed[role] {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "insufficient permissions",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
