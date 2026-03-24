// cmd/trasker-server/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jaypaulb/trasker/internal/server/api"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Required config
	dbURL := mustEnv("DATABASE_URL")
	listenAddr := envOr("LISTEN_ADDR", ":8080")
	jwtSecret := mustEnv("JWT_SECRET")

	// Optional OIDC config
	entraTenant := os.Getenv("ENTRA_TENANT_ID")
	entraClient := os.Getenv("ENTRA_CLIENT_ID")
	entraSecret := os.Getenv("ENTRA_CLIENT_SECRET")
	entraRedirect := os.Getenv("ENTRA_REDIRECT_URL")

	// Database
	pool, err := store.ConnectPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	s, err := store.New(ctx, pool)
	if err != nil {
		return fmt.Errorf("initializing store: %w", err)
	}
	logger.Info("database connected")

	// JWT
	jwtIssuer, err := auth.NewJWTIssuer(jwtSecret, 15*time.Minute)
	if err != nil {
		return fmt.Errorf("initializing JWT issuer: %w", err)
	}

	// OIDC (optional — server works without it for API-key-only mode)
	var oidcConfig *auth.OIDCConfig
	if entraTenant != "" && entraClient != "" {
		oidcConfig, err = auth.NewOIDCConfig(auth.OIDCParams{
			TenantID:     entraTenant,
			ClientID:     entraClient,
			ClientSecret: entraSecret,
			RedirectURL:  entraRedirect,
		})
		if err != nil {
			return fmt.Errorf("initializing OIDC config: %w", err)
		}

		if err := oidcConfig.InitProvider(ctx, entraTenant); err != nil {
			logger.Warn("OIDC provider init failed — dashboard login will be unavailable", "error", err)
			oidcConfig = nil
		} else {
			logger.Info("OIDC provider initialized", "tenant", entraTenant)
		}
	}

	// API key adapter
	apiKeyAuth := api.NewStoreAPIKeyAdapter(s)

	// Router
	deps := &api.Dependencies{
		Store:      s,
		JWTIssuer:  jwtIssuer,
		OIDCConfig: oidcConfig,
		APIKeyAuth: apiKeyAuth,
		Logger:     logger,
	}
	router := api.NewRouter(deps)

	// Server
	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return val
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
