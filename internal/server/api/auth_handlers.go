package api

import (
	"errors"
	"net/http"

	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/jaypaulb/trasker/internal/shared/models"
)

func authLoginHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.OIDCConfig == nil || deps.OIDCConfig.Provider == nil {
			respondError(w, http.StatusServiceUnavailable, "OIDC not configured")
			return
		}

		var req struct {
			Code string `json:"code"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Code == "" {
			respondError(w, http.StatusBadRequest, "authorization code is required")
			return
		}

		oauth2Token, err := deps.OIDCConfig.OAuth2Config.Exchange(r.Context(), req.Code)
		if err != nil {
			deps.Logger.Error("failed to exchange authorization code", "error", err)
			respondError(w, http.StatusUnauthorized, "failed to exchange authorization code")
			return
		}

		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			respondError(w, http.StatusInternalServerError, "no id_token in response")
			return
		}

		idToken, err := deps.OIDCConfig.Verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			deps.Logger.Error("failed to verify ID token", "error", err)
			respondError(w, http.StatusUnauthorized, "failed to verify ID token")
			return
		}

		var claims map[string]any
		if err := idToken.Claims(&claims); err != nil {
			deps.Logger.Error("failed to extract claims", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to extract claims")
			return
		}

		userInfo, err := auth.ExtractUserInfo(claims)
		if err != nil {
			deps.Logger.Error("failed to extract user info", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to extract user info")
			return
		}

		user, err := deps.Store.GetUserByEntraOID(r.Context(), userInfo.EntraOID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			deps.Logger.Error("failed to look up user by entra OID", "error", err)
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			user, err = deps.Store.CreateUser(r.Context(), store.CreateUserParams{
				EntraOID:    userInfo.EntraOID,
				Email:       userInfo.Email,
				DisplayName: userInfo.DisplayName,
			})
			if err != nil {
				deps.Logger.Error("failed to create user", "error", err)
				respondError(w, http.StatusInternalServerError, "failed to create user")
				return
			}
		}

		token, err := deps.JWTIssuer.Issue(user.ID, user.Email, models.Role(user.Role))
		if err != nil {
			deps.Logger.Error("failed to issue token", "error", err, "user_id", user.ID)
			respondError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"token": token,
			"user": map[string]any{
				"id":           user.ID,
				"email":        user.Email,
				"display_name": user.DisplayName,
				"role":         user.Role,
			},
		})
	}
}

func authRefreshHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		role, _ := auth.UserRoleFrom(r.Context())
		email := ""

		if deps.Store != nil {
			user, err := deps.Store.GetUserByID(r.Context(), userID)
			if err != nil {
				deps.Logger.Error("failed to look up user for refresh", "error", err, "user_id", userID)
				respondError(w, http.StatusInternalServerError, "failed to look up user")
				return
			}
			email = user.Email
			role = models.Role(user.Role)
		}

		token, err := deps.JWTIssuer.Issue(userID, email, role)
		if err != nil {
			deps.Logger.Error("failed to issue refresh token", "error", err, "user_id", userID)
			respondError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"token": token})
	}
}
