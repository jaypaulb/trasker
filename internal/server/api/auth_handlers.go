package api

import (
	"net/http"

	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
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
			respondError(w, http.StatusUnauthorized, "failed to verify ID token")
			return
		}

		var claims map[string]any
		if err := idToken.Claims(&claims); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to extract claims")
			return
		}

		userInfo, err := auth.ExtractUserInfo(claims)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to extract user info")
			return
		}

		user, err := deps.Store.GetUserByEntraOID(r.Context(), userInfo.EntraOID)
		if err != nil {
			user, err = deps.Store.CreateUser(r.Context(), store.CreateUserParams{
				EntraOID:    userInfo.EntraOID,
				Email:       userInfo.Email,
				DisplayName: userInfo.DisplayName,
			})
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to create user")
				return
			}
		}

		token, err := deps.JWTIssuer.Issue(user.ID, user.Email, user.Role)
		if err != nil {
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
				respondError(w, http.StatusInternalServerError, "failed to look up user")
				return
			}
			email = user.Email
			role = user.Role
		}

		token, err := deps.JWTIssuer.Issue(userID, email, role)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"token": token})
	}
}
