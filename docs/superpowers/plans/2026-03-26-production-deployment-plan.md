# Production Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 4-container deployment with a 2-container setup (Go server with embedded SPA + autocert TLS, plus Postgres), shipped as a GHCR image with a one-file compose template.

**Architecture:** The Go server absorbs TLS (via `autocert`), SPA serving (via `go:embed`), and client binary distribution (from pre-compiled blobs baked in by CI). When `TRASKER_FQDN` is set, the server listens on :443/:80/:8080 with automatic Let's Encrypt. When unset, it falls back to plain HTTP on `:8080` (local dev).

**Tech Stack:** Go 1.25, `golang.org/x/crypto/acme/autocert`, `embed`, chi router, Docker multi-stage, GitHub Actions, GHCR

**Spec:** `docs/superpowers/specs/2026-03-26-production-deployment-design.md`

---

## File Structure

| File | Responsibility | Task |
|------|---------------|------|
| `internal/server/secrets/jwt.go` | Read-or-generate JWT secret from env/file | 1 |
| `internal/server/secrets/jwt_test.go` | Tests for JWT secret loading | 1 |
| `cmd/trasker-server/main.go` | Triple-listener autocert, admin env vars, JWT from file, FQDN-derived URLs | 2, 3 |
| `internal/server/webui/embed.go` | `//go:embed all:static` for server dashboard SPA | 4 |
| `internal/server/api/router.go` | SPA catch-all route + ENTRA_REDIRECT_URL derivation from FQDN | 4 |
| `internal/server/builder/builder.go` | Load pre-compiled binaries from disk, FQDN-derived server URL | 3 |
| `internal/server/api/build_handlers.go` | FQDN-based server URL for client binaries | 3 |
| `deploy/Dockerfile.server` | Production multi-stage: node + go + alpine, embed SPA, copy client bins | 5 |
| `deploy/docker-compose.production.yml` | User-facing production compose template | 6 |
| `.github/workflows/release.yml` | CI: build client bins, build+push image, create release | 8 |

---

### Task 1: JWT Secret Auto-Generation

**Files:**
- Create: `internal/server/secrets/jwt.go`
- Create: `internal/server/secrets/jwt_test.go`

**Context:** Currently `main.go:40` calls `mustEnvMulti("TRASKER_JWT_SECRET", "JWT_SECRET")` which hard-exits if neither is set. We need a function that checks env → file → generate, so the production compose can omit the JWT secret env var entirely.

- [x] **Step 1: Write the test file**

```go
// internal/server/secrets/jwt_test.go
package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerate_EnvVar(t *testing.T) {
	t.Setenv("TEST_JWT_SECRET", "env-secret-value-that-is-at-least-32-bytes!")
	secret, err := LoadOrGenerateJWTSecret("TEST_JWT_SECRET", filepath.Join(t.TempDir(), "jwt-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if secret != "env-secret-value-that-is-at-least-32-bytes!" {
		t.Fatalf("expected env value, got %q", secret)
	}
}

func TestLoadOrGenerate_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")
	os.WriteFile(path, []byte("file-secret-value"), 0600)

	secret, err := LoadOrGenerateJWTSecret("NONEXISTENT_VAR", path)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "file-secret-value" {
		t.Fatalf("expected file value, got %q", secret)
	}
}

func TestLoadOrGenerate_GeneratesNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")

	secret, err := LoadOrGenerateJWTSecret("NONEXISTENT_VAR", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64-char hex string, got len=%d", len(secret))
	}

	// File should exist now
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("file not created:", err)
	}
	if string(data) != secret {
		t.Fatal("file content doesn't match returned secret")
	}
}

func TestLoadOrGenerate_EnvTakesPrecedenceOverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")
	os.WriteFile(path, []byte("file-value"), 0600)

	t.Setenv("TEST_JWT_PREC", "env-wins")
	secret, err := LoadOrGenerateJWTSecret("TEST_JWT_PREC", path)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "env-wins" {
		t.Fatalf("expected env to win, got %q", secret)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go test ./internal/server/secrets/ -v`
Expected: FAIL — package does not exist yet

- [x] **Step 3: Write the implementation**

```go
// internal/server/secrets/jwt.go
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateJWTSecret returns a JWT secret using this precedence:
//  1. Environment variable (if envKey is set and non-empty)
//  2. Contents of filePath (if file exists)
//  3. Generate 32 random bytes, hex-encode, write to filePath, return
func LoadOrGenerateJWTSecret(envKey, filePath string) (string, error) {
	// 1. Env var wins
	if val := os.Getenv(envKey); val != "" {
		return val, nil
	}

	// 2. Existing file
	if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
		return string(data), nil
	}

	// 3. Generate new
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating random secret: %w", err)
	}
	secret := hex.EncodeToString(buf)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		return "", fmt.Errorf("creating secret directory: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(secret), 0600); err != nil {
		return "", fmt.Errorf("writing secret file: %w", err)
	}

	return secret, nil
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go test ./internal/server/secrets/ -v`
Expected: All 4 tests PASS

- [x] **Step 5: Commit**

```bash
git add internal/server/secrets/jwt.go internal/server/secrets/jwt_test.go
git commit -m "feat: add JWT secret auto-generation from env/file/random"
```

---

### Task 2: Autocert TLS + Triple-Listener in main.go

**Files:**
- Modify: `cmd/trasker-server/main.go`

**Context:** Currently `main.go` creates a single `http.Server` on `LISTEN_ADDR` (:8080). When `TRASKER_FQDN` is set, we need three listeners: `:443` (autocert TLS), `:80` (ACME challenges + HTTP→HTTPS redirect), `:8080` (internal plain HTTP for health checks and load balancer access). When FQDN is unset, keep the existing single-server on `LISTEN_ADDR`. Also wire in JWT secret from Task 1, update admin bootstrap to read env vars, and derive `ENTRA_REDIRECT_URL` from FQDN.

**Reference:** Current `run()` function is at lines 33-170 of `cmd/trasker-server/main.go`.

- [x] **Step 1: Add autocert import and FQDN detection**

In `cmd/trasker-server/main.go`, add to imports:

```go
"crypto/tls"
"golang.org/x/crypto/acme/autocert"
"github.com/jaypaulb/trasker/internal/server/secrets"
```

- [x] **Step 2: Replace JWT secret loading**

Replace line 40:
```go
jwtSecret := mustEnvMulti("TRASKER_JWT_SECRET", "JWT_SECRET")
```

With (checking both env var names for backwards compat, then falling back to file/auto-generate):
```go
// JWT secret: env var → file → auto-generate
jwtSecret := os.Getenv("TRASKER_JWT_SECRET")
if jwtSecret == "" {
    jwtSecret = os.Getenv("JWT_SECRET")
}
if jwtSecret == "" {
    var err error
    jwtSecret, err = secrets.LoadOrGenerateJWTSecret("", "/data/jwt-secret")
    if err != nil {
        return fmt.Errorf("loading JWT secret: %w", err)
    }
    logger.Info("JWT secret loaded from /data/jwt-secret (auto-generated if first boot)")
}
```

- [x] **Step 3: Update admin bootstrap to read env vars**

Replace the `bootstrapAdmin` function (lines 229-261) with:

```go
func bootstrapAdmin(ctx context.Context, s *store.Store, logger *slog.Logger) error {
	count, err := s.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}
	if count > 0 {
		return nil
	}

	// Check env vars for custom admin credentials
	adminEmail := os.Getenv("TRASKER_ADMIN_EMAIL")
	adminPassword := os.Getenv("TRASKER_ADMIN_PASSWORD")
	forceChange := true

	if adminEmail != "" && adminPassword != "" {
		// Custom credentials from env — no forced password change
		forceChange = false
	} else if adminEmail == "" && adminPassword == "" {
		// No env vars — use defaults
		adminEmail = api.DefaultAdminEmail
		adminPassword = api.DefaultAdminPassword
	} else {
		// One set, one missing — config error
		logger.Warn("TRASKER_ADMIN_EMAIL and TRASKER_ADMIN_PASSWORD must both be set or both be empty — skipping admin bootstrap")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}

	user, err := s.CreateLocalUser(ctx, store.CreateLocalUserParams{
		Email:               adminEmail,
		DisplayName:         "Admin",
		PasswordHash:        string(hash),
		Role:                "admin",
		ForcePasswordChange: forceChange,
	})
	if err != nil {
		return fmt.Errorf("creating bootstrap admin: %w", err)
	}

	logger.Info("bootstrap admin account created",
		"email", adminEmail,
		"user_id", user.ID,
		"force_password_change", forceChange,
	)
	return nil
}
```

- [x] **Step 4: Read FQDN env var for use in listener selection and URL derivation**

After the existing env var reads near line 46, add:

```go
fqdn := os.Getenv("TRASKER_FQDN")
```

**Note:** `ENTRA_REDIRECT_URL` derivation from FQDN is handled in `router.go`'s `entraRedirectURL()` function (Task 4 Step 4). Do NOT duplicate it here in `main.go`.

- [x] **Step 5: Replace single-listener with triple-listener (autocert)**

Replace the server creation and startup block (lines 129-169) with:

```go
// Router
deps := &api.Dependencies{
    Store:      s,
    JWTIssuer:  jwtIssuer,
    OIDCConfig: oidcConfig,
    APIKeyAuth: apiKeyAuth,
    Builder:    clientBuilder,
    Logger:     logger,
    FQDN:      fqdn,
}
router := api.NewRouter(deps)

// Graceful shutdown
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

if fqdn != "" {
    // Production: autocert TLS on :443 + ACME/redirect on :80 + internal on :8080
    certDir := "/data/certs"
    if err := os.MkdirAll(certDir, 0700); err != nil {
        return fmt.Errorf("creating cert cache dir: %w", err)
    }

    m := &autocert.Manager{
        Cache:      autocert.DirCache(certDir),
        Prompt:     autocert.AcceptTOS,
        HostPolicy: autocert.HostWhitelist(fqdn),
    }

    // Listener 1: TLS on :443
    tlsSrv := &http.Server{
        Addr:         ":443",
        Handler:      router,
        TLSConfig:    &tls.Config{GetCertificate: m.GetCertificate},
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // Listener 2: ACME challenges + HTTP→HTTPS redirect on :80
    httpSrv := &http.Server{
        Addr:         ":80",
        Handler:      m.HTTPHandler(nil), // nil = default redirect to HTTPS
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 5 * time.Second,
    }

    // Listener 3: Internal plain HTTP on :8080 for health checks and load balancer.
    // Hardcoded to :8080 in TLS mode (not LISTEN_ADDR) because the compose
    // health check and docs rely on this port being predictable.
    internalSrv := &http.Server{
        Addr:         ":8080",
        Handler:      router,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    errCh := make(chan error, 3)
    go func() {
        logger.Info("TLS server starting", "addr", ":443", "fqdn", fqdn)
        if err := tlsSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
            errCh <- fmt.Errorf("TLS server: %w", err)
        }
    }()
    go func() {
        logger.Info("HTTP redirect server starting", "addr", ":80")
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errCh <- fmt.Errorf("HTTP server: %w", err)
        }
    }()
    go func() {
        logger.Info("internal HTTP server starting", "addr", ":8080")
        if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errCh <- fmt.Errorf("internal server: %w", err)
        }
    }()

    select {
    case sig := <-sigCh:
        logger.Info("received signal, shutting down", "signal", sig)
    case err := <-errCh:
        return err
    }

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutdownCancel()

    // Shut down all three servers
    var shutdownErr error
    for name, srv := range map[string]*http.Server{
        "TLS": tlsSrv, "HTTP": httpSrv, "internal": internalSrv,
    } {
        if err := srv.Shutdown(shutdownCtx); err != nil {
            if shutdownErr != nil {
                shutdownErr = fmt.Errorf("%v; %s shutdown: %w", shutdownErr, name, err)
            } else {
                shutdownErr = fmt.Errorf("%s shutdown: %w", name, err)
            }
        }
    }

    if shutdownErr != nil {
        return shutdownErr
    }
} else {
    // Development: plain HTTP on LISTEN_ADDR
    srv := &http.Server{
        Addr:         listenAddr,
        Handler:      router,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    errCh := make(chan error, 1)
    go func() {
        logger.Info("server starting (plain HTTP)", "addr", listenAddr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errCh <- err
        }
        close(errCh)
    }()

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
}

logger.Info("server stopped")
return nil
```

- [x] **Step 6: Verify compilation**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go build ./cmd/trasker-server/`
Expected: Compiles with no errors

- [x] **Step 7: Run existing tests**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go test ./internal/server/... -count=1`
Expected: All existing tests pass

- [x] **Step 8: Commit**

```bash
git add cmd/trasker-server/main.go
git commit -m "feat: add autocert TLS, JWT auto-generation, admin env vars"
```

---

### Task 3: Builder — Load Pre-compiled Binaries from Disk

**Files:**
- Modify: `internal/server/builder/builder.go`
- Modify: `internal/server/api/build_handlers.go`
- Modify: `internal/server/api/router.go`
- Modify: `cmd/trasker-server/main.go`

**Context:** Currently `PreBuild()` compiles all 5 targets using `go build` and stores them in memory. For production (no Go toolchain), we need a `LoadFromDir()` method that reads pre-compiled binaries from `/app/clients/` on disk. The builder init in `main.go` needs to try `LoadFromDir` first, fall back to `PreBuild` if Go toolchain is available.

- [x] **Step 1: Add NewEmptyBuilder and LoadFromDir to builder.go**

Add after the `NewBuilder` function (line 108):

```go
// ClientBinDir is the default directory for pre-compiled client binaries
// baked into the container image by CI.
const ClientBinDir = "/app/clients"

// NewEmptyBuilder creates a Builder with no source directory
// (for loading pre-compiled binaries only).
func NewEmptyBuilder() *Builder {
	return &Builder{
		cache: make(map[string][]byte),
	}
}

// LoadFromDir loads pre-compiled generic binaries from a directory on disk.
// Files must be named trasker-client-{os}-{arch}[.exe] and contain sentinel
// strings. Returns the number of targets loaded. If the directory doesn't
// exist or is empty, returns 0 (not an error — caller should fall back to
// PreBuild or Build).
func (b *Builder) LoadFromDir(dir string, logger *slog.Logger) int {
	loaded := 0
	for _, target := range SupportedTargets {
		path := filepath.Join(dir, target.BinaryName())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Verify sentinel is present
		if !bytes.Contains(data, []byte(SentinelAPIKey)) {
			logger.Warn("pre-compiled binary missing sentinel, skipping",
				"target", target.String(), "path", path)
			continue
		}

		b.mu.Lock()
		b.cache[target.String()] = data
		b.mu.Unlock()

		logger.Info("loaded pre-compiled client binary",
			"target", target.String(),
			"size_mb", fmt.Sprintf("%.1f", float64(len(data))/(1024*1024)),
		)
		loaded++
	}
	return loaded
}
```

- [x] **Step 2: Update main.go builder initialization**

Replace the builder init block in `main.go` (lines 96-116) with:

```go
// Client builder: try pre-compiled binaries first (production),
// fall back to runtime compilation (development with Go toolchain).
var clientBuilder *builder.Builder

// 1. Try loading pre-compiled binaries from disk (CI-built, baked into image)
b := builder.NewEmptyBuilder()
loaded := b.LoadFromDir(builder.ClientBinDir, logger)
if loaded > 0 {
    clientBuilder = b
    logger.Info("loaded pre-compiled client binaries", "count", loaded, "total", len(builder.SupportedTargets))
} else if _, err := exec.LookPath("go"); err == nil {
    // 2. Fall back to runtime compilation (dev mode with Go toolchain)
    moduleRoot := findModuleRoot()
    if moduleRoot != "" {
        clientBuilder = builder.NewBuilder(moduleRoot, "./cmd/trasker-client")
        logger.Info("client builder available — pre-building binaries in background", "module_root", moduleRoot)
        go func() {
            if err := clientBuilder.PreBuild(ctx, logger); err != nil {
                logger.Error("pre-build failed — downloads will be unavailable", "error", err)
            }
        }()
    } else {
        logger.Warn("Go toolchain found but go.mod not found — client builds disabled")
    }
} else {
    logger.Info("no pre-compiled binaries and no Go toolchain — client downloads disabled")
}
```

- [x] **Step 3: Add FQDN to Dependencies and update build_handlers.go server URL**

> **Deviation note:** The `FQDN` field was added to `Dependencies` during Task 2 (Rule 3 — needed for compilation when `deps.FQDN: fqdn` was wired). This step's `build_handlers.go` change was completed here.

Add to `internal/server/api/router.go` `Dependencies` struct:

```go
FQDN string // If set, used for client binary server URL (https://<FQDN>)
```

Update `build_handlers.go` lines 74-83 to derive server URL from FQDN:

```go
// Derive server URL
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
```

- [x] **Step 4: Verify compilation**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go build ./cmd/trasker-server/`
Expected: Compiles

- [x] **Step 5: Run tests**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go test ./internal/server/... -count=1`
Expected: All tests pass

- [x] **Step 6: Commit**

```bash
git add internal/server/builder/builder.go internal/server/api/build_handlers.go internal/server/api/router.go cmd/trasker-server/main.go
git commit -m "feat: load pre-compiled client binaries from disk, add FQDN-based URL"
```

---

### Task 4: Server-Side SPA Embedding

**Files:**
- Create: `internal/server/webui/embed.go`
- Modify: `internal/server/api/router.go`

**Context:** The server currently serves only `/api/v1/*` routes. In production, nginx served the dashboard SPA. We need to embed `web/server-ui/build/` into the server binary and serve it as a catch-all. The client SPA embed at `internal/client/webui/embed.go` is the pattern to follow.

**Important:** The `static/` directory must exist (even empty) for `go:embed` to compile. In CI, the Dockerfile copies the SPA build output there. For local dev without a build, create an empty placeholder.

- [x] **Step 1: Create embed.go**

```go
// internal/server/webui/embed.go
package webui

import "embed"

// Assets holds the built dashboard SPA files (web/server-ui/build/).
// The Dockerfile copies build output into static/ before go build.
//
//go:embed all:static
var Assets embed.FS
```

- [x] **Step 2: Create placeholder static directory**

Run: `mkdir -p /home/jaypaulb/Projects/gh/trasker/internal/server/webui/static && touch /home/jaypaulb/Projects/gh/trasker/internal/server/webui/static/.gitkeep`

This ensures `go build` works even without a SPA build. The `.gitkeep` is committed so the directory exists in source.

- [x] **Step 3: Add SPA serving to router.go**

At the end of `NewRouter`, after the `/api/v1` route block (after line 149), add:

```go
// Serve embedded dashboard SPA for all non-API routes.
// The SPA handles client-side routing — unknown paths get index.html.
spaFS, err := fs.Sub(webui.Assets, "static")
if err != nil {
    // Should never happen — embedded FS is compile-time
    panic(fmt.Sprintf("embedded SPA filesystem: %v", err))
}
fileServer := http.FileServer(http.FS(spaFS))

r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
    // Try to serve the file directly. If it doesn't exist, serve index.html
    // (SPA client-side routing).
    path := r.URL.Path
    if path == "/" {
        path = "/index.html"
    }

    // Check if the file exists in the embedded FS
    f, err := spaFS.Open(strings.TrimPrefix(path, "/"))
    if err == nil {
        f.Close()
        fileServer.ServeHTTP(w, r)
        return
    }

    // File doesn't exist — serve index.html for SPA routing
    r.URL.Path = "/"
    fileServer.ServeHTTP(w, r)
})
```

Add imports to router.go:

```go
"io/fs"
"strings"

"github.com/jaypaulb/trasker/internal/server/webui"
```

- [x] **Step 4: Update entraRedirectURL to use FQDN**

Replace `entraRedirectURL()` function (lines 154-160) with:

```go
func entraRedirectURL() string {
	if u := os.Getenv("ENTRA_REDIRECT_URL"); u != "" {
		return u
	}
	if fqdn := os.Getenv("TRASKER_FQDN"); fqdn != "" {
		return fmt.Sprintf("https://%s/auth/callback", fqdn)
	}
	return "http://localhost:5173/auth/callback"
}
```

**Note:** This function in `router.go` is the single canonical location for ENTRA_REDIRECT_URL derivation. Task 2 Step 4 explicitly defers to this function — do not duplicate FQDN derivation logic in `main.go`.

- [x] **Step 5: Verify compilation**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go build ./cmd/trasker-server/`
Expected: Compiles (SPA directory has just .gitkeep — that's fine, embed includes it)

- [x] **Step 6: Commit**

```bash
git add internal/server/webui/embed.go internal/server/webui/static/.gitkeep internal/server/api/router.go
git commit -m "feat: embed and serve dashboard SPA from server binary"
```

---

### Task 5: Production Dockerfile

**Files:**
- Modify: `deploy/Dockerfile.server`

**Context:** Rewrite as a proper multi-stage build: node → go → alpine runtime. The SPA build output goes into `internal/server/webui/static/` before `go build` so embed picks it up. Pre-compiled client binaries are copied from `deploy/clients/` (CI places them there before building the image).

- [x] **Step 1: Rewrite Dockerfile.server**

```dockerfile
# ============================================
# Stage 1: Build the dashboard SPA
# ============================================
FROM node:22-slim AS spa-builder

WORKDIR /spa
COPY web/server-ui/package.json web/server-ui/package-lock.json ./
RUN npm ci
COPY web/server-ui/ ./
RUN npm run build

# ============================================
# Stage 2: Build the Go server binary
# ============================================
FROM golang:1.25-bookworm AS go-builder

WORKDIR /src

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Copy SPA build output into server embed directory
COPY --from=spa-builder /spa/build/ ./internal/server/webui/static/

# Build server binary (static, no CGo)
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /out/trasker-server \
    ./cmd/trasker-server

# ============================================
# Stage 3: Runtime
# ============================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1001 trasker && \
    adduser -u 1001 -G trasker -D trasker

WORKDIR /app

COPY --from=go-builder /out/trasker-server .

# Copy migrations directly from build context
COPY migrations/ ./migrations/

# Copy pre-compiled client binaries (CI places them in deploy/clients/)
COPY deploy/clients/ /app/clients/

# Data directory for JWT secret + cert cache (volume-mounted)
RUN mkdir -p /data/certs && chown -R trasker:trasker /data

USER trasker

EXPOSE 443 80 8080

ENTRYPOINT ["./trasker-server"]
```

- [x] **Step 2: Create deploy/clients/.gitkeep**

Run: `mkdir -p /home/jaypaulb/Projects/gh/trasker/deploy/clients && touch /home/jaypaulb/Projects/gh/trasker/deploy/clients/.gitkeep`

This ensures the `COPY deploy/clients/` directive succeeds even without CI binaries present.

- [x] **Step 3: Verify Docker build (without CI binaries — should still work)**

Run: `cd /home/jaypaulb/Projects/gh/trasker && docker build -f deploy/Dockerfile.server -t trasker-test .`
Expected: Builds successfully. Client binaries dir will be empty (only .gitkeep) — server logs "no pre-compiled binaries" at startup, which is correct for dev.

- [x] **Step 4: Commit**

```bash
git add deploy/Dockerfile.server deploy/clients/.gitkeep
git commit -m "feat: production multi-stage Dockerfile with embedded SPA"
```

---

### Task 6: Production Compose Template

**Files:**
- Create: `deploy/docker-compose.production.yml`

- [x] **Step 1: Create the compose file**

```yaml
# Trasker — Production deployment
# ================================
# 1. Edit the values marked <CHANGE_ME> below
# 2. Run:  docker compose -f docker-compose.production.yml up -d
# 3. Visit https://<your-fqdn> — TLS certificate is acquired automatically
#
# Requirements:
#   - Ports 80 and 443 must be open and reachable from the internet
#   - DNS A record pointing your FQDN to this server's IP address

services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: trasker
      POSTGRES_USER: trasker
      POSTGRES_PASSWORD: <CHANGE_ME>
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U trasker"]
      interval: 3s
      timeout: 2s
      retries: 10

  trasker:
    image: ghcr.io/jaypaulb/trasker:latest
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - trasker_data:/data
    environment:
      # --- REQUIRED: Edit these ---
      TRASKER_FQDN: <CHANGE_ME>               # Your server's public hostname (e.g. trasker.example.com)
      TRASKER_ADMIN_EMAIL: <CHANGE_ME>         # Initial admin account email
      TRASKER_ADMIN_PASSWORD: <CHANGE_ME>      # Initial admin account password
      TRASKER_DB_HOST: postgres
      TRASKER_DB_NAME: trasker
      TRASKER_DB_USER: trasker
      TRASKER_DB_PASSWORD: <CHANGE_ME>         # Must match POSTGRES_PASSWORD above
      # --- Optional: Azure AD SSO ---
      # TRASKER_ENTRA_TENANT: <tenant-id>
      # TRASKER_ENTRA_CLIENT: <client-id>
      # TRASKER_ENTRA_SECRET: <client-secret>
      # ENTRA_REDIRECT_URL: https://<your-fqdn>/auth/callback
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/api/v1/health"]
      interval: 10s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
  trasker_data:
```

**Note:** The health check hits `localhost:8080` which is the internal plain HTTP listener started in Task 2 (the third listener in TLS mode). Port 8080 is not published in `ports:` — it's internal to the container only.

- [x] **Step 2: Commit**

```bash
git add deploy/docker-compose.production.yml
git commit -m "feat: add production compose template for single-command deployment"
```

---

### Task 7: Clean Up Removed Files

**Files:**
- Remove: `deploy/docker-compose.yml` (old 4-container production compose)
- Remove: `deploy/caddy/` (Caddyfile)
- Remove: `deploy/nginx/` (nginx config)
- Remove: `deploy/Dockerfile.builder` (replaced by CI)
- Remove: `deploy/builder-entrypoint.sh` (replaced by CI)
- Remove: `deploy/.env.example` (replaced by compose template comments)

- [x] **Step 1: Remove old files**

Run:
```bash
cd /home/jaypaulb/Projects/gh/trasker
git rm deploy/docker-compose.yml
git rm -r deploy/caddy/
git rm -r deploy/nginx/
git rm deploy/Dockerfile.builder
git rm deploy/builder-entrypoint.sh
git rm deploy/.env.example
```

**Note:** Some of these files may not exist. Use `git rm --ignore-unmatch` or check existence first. Only remove files that exist.

- [x] **Step 2: Verify nothing references removed files**

Run: `grep -r "Dockerfile.builder\|builder-entrypoint\|Caddyfile\|nginx/default" --include="*.go" --include="*.yml" --include="*.yaml" --include="*.md" /home/jaypaulb/Projects/gh/trasker/`
Expected: No references in active code (spec/plan docs are fine)

- [x] **Step 3: Commit**

```bash
git commit -m "chore: remove Caddy, nginx, and builder sidecar (replaced by autocert + embedded SPA + CI builds)"
```

---

### Task 8: GitHub Actions CI Pipeline

**Files:**
- Create: `.github/workflows/release.yml`

**Context:** Triggered by pushing a `v*` tag. Builds client binaries with sentinel placeholder strings, builds+pushes Docker image to GHCR, creates GitHub Release with compose file attached.

**Critical: Sentinel strings.** The CI must compile client binaries with the exact sentinel values from `internal/server/builder/builder.go`. These are the constants `SentinelServerURL`, `SentinelAPIKey`, and `SentinelVersion`, each exactly 128 bytes. The ldflags format (from `builder.go:123-126`) is:

```
-X main.apiKey=<SentinelAPIKey> -X main.serverURL=<SentinelServerURL> -X main.version=<SentinelVersion>
```

The exact values (from `builder.go:21-29`):
- `SentinelServerURL` = `TRASKER_SENTINEL_SERVER_URL_____` + 96 underscores (128 total)
- `SentinelAPIKey` = `TRASKER_SENTINEL_API_KEY________` + 96 underscores (128 total)
- `SentinelVersion` = `TRASKER_SENTINEL_VERSION________` + 96 underscores (128 total)

- [x] **Step 1: Create the workflow file**

```yaml
# .github/workflows/release.yml
# Source of truth for sentinel values: internal/server/builder/builder.go
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write    # Create releases
  packages: write    # Push to GHCR

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}
  # Sentinel strings — must match internal/server/builder/builder.go exactly (128 bytes each).
  # If builder.go sentinels change, these MUST be updated to match.
  SENTINEL_SERVER_URL: "TRASKER_SENTINEL_SERVER_URL_____________________________________________________________________________________________________"
  SENTINEL_API_KEY:    "TRASKER_SENTINEL_API_KEY________________________________________________________________________________________________________"
  SENTINEL_VERSION:    "TRASKER_SENTINEL_VERSION________________________________________________________________________________________________________"

jobs:
  build-clients:
    name: Build client binaries
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
          - goos: windows
            goarch: amd64
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Verify sentinel length
        run: |
          # Safety check: sentinels must be exactly 128 bytes
          [ ${#SENTINEL_SERVER_URL} -eq 128 ] || { echo "SENTINEL_SERVER_URL is ${#SENTINEL_SERVER_URL} bytes, expected 128"; exit 1; }
          [ ${#SENTINEL_API_KEY} -eq 128 ] || { echo "SENTINEL_API_KEY is ${#SENTINEL_API_KEY} bytes, expected 128"; exit 1; }
          [ ${#SENTINEL_VERSION} -eq 128 ] || { echo "SENTINEL_VERSION is ${#SENTINEL_VERSION} bytes, expected 128"; exit 1; }
      - name: Build client binary
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: '0'
        run: |
          BINARY_NAME="trasker-client-${{ matrix.goos }}-${{ matrix.goarch }}"
          if [ "${{ matrix.goos }}" = "windows" ]; then
            BINARY_NAME="${BINARY_NAME}.exe"
          fi
          go build \
            -ldflags="-s -w -X main.apiKey=${SENTINEL_API_KEY} -X main.serverURL=${SENTINEL_SERVER_URL} -X main.version=${SENTINEL_VERSION}" \
            -trimpath \
            -o "deploy/clients/${BINARY_NAME}" \
            ./cmd/trasker-client
      - name: Upload client binary
        uses: actions/upload-artifact@v4
        with:
          name: client-${{ matrix.goos }}-${{ matrix.goarch }}
          path: deploy/clients/

  build-and-push:
    name: Build and push container image
    runs-on: ubuntu-latest
    needs: build-clients
    steps:
      - uses: actions/checkout@v4

      - name: Download all client binaries
        uses: actions/download-artifact@v4
        with:
          path: deploy/clients/
          merge-multiple: true

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=semver,pattern={{version}}
            type=raw,value=latest

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/Dockerfile.server
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}

  release:
    name: Create GitHub Release
    runs-on: ubuntu-latest
    needs: build-and-push
    steps:
      - uses: actions/checkout@v4
      - name: Create release
        uses: softprops/action-gh-release@v2
        with:
          files: deploy/docker-compose.production.yml
          body: |
            ## Quick Start

            1. Download `docker-compose.production.yml`
            2. Edit the `<CHANGE_ME>` values (FQDN, admin credentials, database password)
            3. Run: `docker compose -f docker-compose.production.yml up -d`
            4. Visit `https://<your-fqdn>` — TLS certificate is acquired automatically

            See [README](https://github.com/${{ github.repository }}#deployment) for full documentation.
          generate_release_notes: true
```

- [x] **Step 2: Verify workflow syntax**

Run: `cd /home/jaypaulb/Projects/gh/trasker && python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml')); print('valid')"`

- [x] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow — build clients, push to GHCR, create release"
```

---

### Task 9: Integration Test — Local Docker Build

**No new files.** This is a verification task.

- [x] **Step 1: Build the production image locally**

Run:
```bash
cd /home/jaypaulb/Projects/gh/trasker
docker build -f deploy/Dockerfile.server -t trasker-prod-test .
```
Expected: Multi-stage build completes. SPA embedded. Server binary built.

- [x] **Step 2: Run with local dev compose to verify backwards compatibility**

> **Deviation note:** Used the **production** image (`trasker-test:latest`)
> rather than `docker-compose.local.yml` for the smoke test, because
> `Dockerfile.server-local` does not embed the dashboard SPA into the server
> binary (the local dev workflow runs the SPA via `npm run dev` separately).
> Verifying step 3 (`/` returning SPA HTML) requires the production image.
> Local-dev image was rebuilt to confirm it still compiles. Smoke stack
> mounted `deploy/initdb/` into the postgres container for schema bootstrap
> (production compose users get this from the GHCR migration step on a real
> deploy).

Smoke stack run:
```
trasker:                          postgres:
  image: trasker-test:latest        volumes:
  ports: 18080:8080                   - deploy/initdb -> /docker-entrypoint-initdb.d
  env: TRASKER_DB_*                 healthcheck: pg_isready
```
Result: `GET /api/v1/health` → `{"status":"ok","timestamp":"...","version":"dev (unknown)"}`

- [x] **Step 3: Verify SPA is served from server**

Run: `curl -s http://localhost:18080/ | head -10`
Result: SvelteKit dashboard `index.html` served (200) — embedded SPA works.

- [x] **Step 4: Run all Go tests**

Run: `cd /home/jaypaulb/Projects/gh/trasker && go test -p 1 ./internal/server/... -count=1`
Result: All server packages pass (api, auth, builder, secrets, store, webui).

> **Pre-existing issue (out of scope):** `internal/client/webui` test setup
> fails with `pattern all:static: no matching files found` because the
> client SPA build output (`web/client-ui` → `internal/client/webui/static/`)
> is gitignored and not present locally. This existed before Phase 8 and
> is not caused by any Phase 8 change. Logged in `08-SUMMARY.md` under
> Deferred Issues.

- [x] **Step 5: Commit any final fixes if needed**
