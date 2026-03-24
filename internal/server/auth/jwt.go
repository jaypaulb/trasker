package auth

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTClaims holds the custom claims in a Trasker JWT.
type JWTClaims struct {
	UserID uuid.UUID `json:"uid"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// JWTIssuer handles creating and validating JWTs.
type JWTIssuer struct {
	secret   []byte
	duration time.Duration
}

// NewJWTIssuer creates a new JWT issuer with the given HMAC secret and token duration.
func NewJWTIssuer(secret string, duration time.Duration) (*JWTIssuer, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &JWTIssuer{
		secret:   []byte(secret),
		duration: duration,
	}, nil
}

// Issue creates a signed JWT for the given user.
func (j *JWTIssuer) Issue(userID uuid.UUID, email, role string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.duration)),
			Issuer:    "trasker",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

// Validate parses and validates a JWT, returning its claims.
func (j *JWTIssuer) Validate(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("jwt: unexpected signing method %v", token.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwt: validation failed: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("jwt: invalid token claims")
	}

	return claims, nil
}

// JWTMiddleware returns middleware that authenticates requests via JWT.
// The token is expected in the Authorization header as "Bearer <token>".
func JWTMiddleware(issuer *JWTIssuer) func(http.Handler) http.Handler {
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

			claims, err := issuer.Validate(parts[1])
			if err != nil {
				slog.Warn("JWT validation failed", "error", err)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
				return
			}

			ctx := r.Context()
			ctx = WithUserID(ctx, claims.UserID)
			ctx = WithUserRole(ctx, claims.Role)
			ctx = WithAuthType(ctx, AuthTypeJWT)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
