// cmd/trasker-server/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jaypaulb/trasker/internal/server/api"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/builder"
	"github.com/jaypaulb/trasker/internal/server/store"
	"golang.org/x/crypto/bcrypt"
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

	// Required config — check TRASKER_* prefix first (Docker), then unprefixed (local dev)
	dbURL := buildDatabaseURL()
	listenAddr := envOr("LISTEN_ADDR", ":8080")
	jwtSecret := mustEnvMulti("TRASKER_JWT_SECRET", "JWT_SECRET")

	// Optional OIDC config — check TRASKER_* prefix first, then unprefixed
	entraTenant := envOrMulti("TRASKER_ENTRA_TENANT", "ENTRA_TENANT_ID")
	entraClient := envOrMulti("TRASKER_ENTRA_CLIENT", "ENTRA_CLIENT_ID")
	entraSecret := envOrMulti("TRASKER_ENTRA_SECRET", "ENTRA_CLIENT_SECRET")
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

	// Bootstrap: create default admin if no users exist
	if err := bootstrapAdmin(ctx, s, logger); err != nil {
		return fmt.Errorf("bootstrapping admin: %w", err)
	}

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

	// Client builder (optional — only available when Go toolchain is installed)
	var clientBuilder *builder.Builder
	if _, err := exec.LookPath("go"); err == nil {
		// Find Go module root: look for go.mod starting from the executable's directory
		moduleRoot := findModuleRoot()
		if moduleRoot != "" {
			clientBuilder = builder.NewBuilder(moduleRoot, "./cmd/trasker-client")
			logger.Info("client builder available — pre-building binaries in background", "module_root", moduleRoot)

			// Pre-build all targets in the background so downloads are instant.
			go func() {
				if err := clientBuilder.PreBuild(ctx, logger); err != nil {
					logger.Error("pre-build failed — downloads will be unavailable", "error", err)
				}
			}()
		} else {
			logger.Warn("Go toolchain found but go.mod not found — client builds disabled")
		}
	} else {
		logger.Info("Go toolchain not found — client binary builds disabled")
	}

	// Router
	deps := &api.Dependencies{
		Store:      s,
		JWTIssuer:  jwtIssuer,
		OIDCConfig: oidcConfig,
		APIKeyAuth: apiKeyAuth,
		Builder:    clientBuilder,
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

// buildDatabaseURL returns a PostgreSQL connection string.
// It checks DATABASE_URL first, then falls back to constructing one from
// the individual TRASKER_DB_* env vars (as set by docker-compose).
func buildDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}

	host := os.Getenv("TRASKER_DB_HOST")
	port := os.Getenv("TRASKER_DB_PORT")
	name := os.Getenv("TRASKER_DB_NAME")
	user := os.Getenv("TRASKER_DB_USER")
	pass := os.Getenv("TRASKER_DB_PASSWORD")

	if host != "" && user != "" && name != "" {
		if port == "" {
			port = "5432"
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
	}

	fmt.Fprintf(os.Stderr, "required: either DATABASE_URL or TRASKER_DB_HOST/USER/NAME env vars\n")
	os.Exit(1)
	return ""
}

// mustEnvMulti checks multiple env var names in order, returning the first non-empty value.
// Exits if none are set.
func mustEnvMulti(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	fmt.Fprintf(os.Stderr, "required environment variable (one of %v) is not set\n", keys)
	os.Exit(1)
	return ""
}

// envOrMulti checks multiple env var names in order, returning the first non-empty value,
// or empty string if none are set.
func envOrMulti(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// bootstrapAdmin creates the default admin account if no users exist.
func bootstrapAdmin(ctx context.Context, s *store.Store, logger *slog.Logger) error {
	count, err := s.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(api.DefaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing default password: %w", err)
	}

	user, err := s.CreateLocalUser(ctx, store.CreateLocalUserParams{
		Email:               api.DefaultAdminEmail,
		DisplayName:         "Admin",
		PasswordHash:        string(hash),
		Role:                "admin",
		ForcePasswordChange: true,
	})
	if err != nil {
		return fmt.Errorf("creating bootstrap admin: %w", err)
	}

	logger.Info("bootstrap admin account created",
		"email", api.DefaultAdminEmail,
		"user_id", user.ID,
		"note", "default password must be changed on first login",
	)
	return nil
}

// findModuleRoot walks up from the current executable (or working directory)
// looking for a go.mod file, returning the directory that contains it.
// Returns empty string if not found.
func findModuleRoot() string {
	// Try working directory first (common in dev), then executable location
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	for _, start := range candidates {
		dir := start
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}
