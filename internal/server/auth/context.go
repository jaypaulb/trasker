package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	ctxKeyUserID   contextKey = "auth_user_id"
	ctxKeyUserRole contextKey = "auth_user_role"
	ctxKeyAPIKeyID contextKey = "auth_apikey_id"
	ctxKeyAuthType contextKey = "auth_type"
)

// AuthType indicates how the request was authenticated.
type AuthType string

const (
	AuthTypeAPIKey AuthType = "apikey"
	AuthTypeJWT    AuthType = "jwt"
)

// WithUserID stores the authenticated user ID in context.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, id)
}

// UserIDFrom retrieves the authenticated user ID from context.
func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKeyUserID).(uuid.UUID)
	return id, ok
}

// WithUserRole stores the user role in context.
func WithUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxKeyUserRole, role)
}

// UserRoleFrom retrieves the user role from context.
func UserRoleFrom(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(ctxKeyUserRole).(string)
	return role, ok
}

// WithAPIKeyID stores the API key ID in context.
func WithAPIKeyID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKeyAPIKeyID, id)
}

// APIKeyIDFrom retrieves the API key ID from context.
func APIKeyIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKeyAPIKeyID).(uuid.UUID)
	return id, ok
}

// WithAuthType stores the authentication type in context.
func WithAuthType(ctx context.Context, t AuthType) context.Context {
	return context.WithValue(ctx, ctxKeyAuthType, t)
}

// AuthTypeFrom retrieves the authentication type from context.
func AuthTypeFrom(ctx context.Context) (AuthType, bool) {
	t, ok := ctx.Value(ctxKeyAuthType).(AuthType)
	return t, ok
}
